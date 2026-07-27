package credentialservice

import (
	"context"
	"errors"
	"time"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
)

type CredentialService struct {
	CredentialRepository repositoryTypes.CredentialRepository
	encryptionClient     repositoryTypes.EncryptionClient
	auditService         types.AuditService
	transactionHandler   repositoryTypes.TransationHandler
	logger               *zerolog.Logger
}

func NewCredentialService(
	ctx context.Context,
	credentialRepository repositoryTypes.CredentialRepository,
	encryptionClient repositoryTypes.EncryptionClient,
	auditService types.AuditService,
	transactionHandler repositoryTypes.TransationHandler,
) *CredentialService {
	assert.NotNil(credentialRepository)
	assert.NotNil(encryptionClient)
	assert.NotNil(auditService)
	assert.NotNil(transactionHandler)

	return &CredentialService{
		CredentialRepository: credentialRepository,
		encryptionClient:     encryptionClient,
		auditService:         auditService,
		transactionHandler:   transactionHandler,
		logger:               zerolog.Ctx(ctx),
	}
}

var _ types.CredentialService = &CredentialService{}

func (s *CredentialService) SetCredential(ctx context.Context, credentialName string, credential entity.Credential, credentialType entity.CredentialType) (entity.CredentialEncrypted, error) {
	tenantId := auth.MustGetTenant(ctx)
	userId := auth.MustGetUser(ctx)

	if credentialName == "" {
		s.logger.Error().Ctx(ctx).Msg("credential name cannot be empty")
		return entity.CredentialEncrypted{}, xerrors.BadRequestError(ctx, errors.New("credential name cannot be empty"))
	}

	// Marshal credential to JSON
	credentialJSON, err := protojson.Marshal(credential.Credential)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("failed to marshal credential to JSON")
		return entity.CredentialEncrypted{}, xerrors.InternalError(ctx, errors.New("failed to marshal credential"))
	}

	// Encrypt the credential
	encryptedData, err := s.encryptionClient.Encrypt(ctx, credentialJSON)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("failed to encrypt credential")
		return entity.CredentialEncrypted{}, xerrors.InternalError(ctx, errors.New("failed to encrypt credential"))
	}

	now := time.Now()
	credentialEncrypted := entity.CredentialEncrypted{
		ID:                 uuid.Must(uuid.NewV7()),
		TenantID:           tenantId,
		CredentialName:     credentialName,
		Type:               credentialType,
		CredentialMetadata: credential.CredentialMetadata,
		EncryptedPayload:   encryptedData.EncryptedPayload,
		EncryptedDataKey:   encryptedData.EncryptedDataKey,
		Nonce:              encryptedData.IV,
		CreatedBy:          userId,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	var result entity.CredentialEncrypted
	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		var txErr error
		result, txErr = s.CredentialRepository.CreateCredential(ctx, credentialEncrypted)
		if txErr != nil {
			return txErr
		}

		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCredentialCreated,
			ResourceType: types.AuditResourceTypeCredential,
			ResourceID:   result.ID.String(),
		})
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("failed to create credential")
		return entity.CredentialEncrypted{}, err
	}

	s.logger.Info().Ctx(ctx).Str("credential_id", result.ID.String()).Str("credential_name", credentialName).Msg("Credential created successfully")
	return result, nil
}

func (s *CredentialService) GetCredential(ctx context.Context, credentialId uuid.UUID) (entity.CredentialEncrypted, error) {
	tenantID := auth.MustGetTenant(ctx)

	// Get the credential from repository
	credential, err := s.CredentialRepository.GetCredentialEncrypted(ctx, credentialId, tenantID)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("credential_id", credentialId.String()).Msg("failed to get credential")
		return entity.CredentialEncrypted{}, err
	}

	s.logger.Debug().Ctx(ctx).Str("credential_id", credentialId.String()).Msg("Retrieved credential successfully")

	return credential, nil
}

func (s *CredentialService) GetDecryptedCredential(ctx context.Context, credentialId uuid.UUID) (entity.Credential, error) {
	credentialEncrypted, err := s.CredentialRepository.INTERNALgetCredential(ctx, credentialId)
	if err != nil {
		s.logger.Error().Err(err).Str("credential_id", credentialId.String()).Msg("failed to get credential")
		return entity.Credential{}, err
	}

	// decrypt
	encryptedData := &repositoryTypes.EncryptedData{
		EncryptedPayload: credentialEncrypted.EncryptedPayload,
		EncryptedDataKey: credentialEncrypted.EncryptedDataKey,
		IV:               credentialEncrypted.Nonce,
	}

	decryptedJSON, err := s.encryptionClient.Decrypt(ctx, encryptedData)
	if err != nil {
		s.logger.Error().Err(err).Str("credential_id", credentialId.String()).Msg("failed to decrypt credential")
		return entity.Credential{}, xerrors.InternalError(ctx, errors.New("failed to decrypt credential"))
	}

	// Unmarshal the decrypted JSON to pb.Credential
	credentialPb := &pb.Credential{}
	if err := protojson.Unmarshal(decryptedJSON, credentialPb); err != nil {
		s.logger.Error().Err(err).Str("credential_id", credentialId.String()).Msg("failed to unmarshal credential")
		return entity.Credential{}, xerrors.InternalError(ctx, errors.New("failed to unmarshal credential"))
	}

	return entity.Credential{
		ID:                 credentialEncrypted.ID,
		TenantID:           credentialEncrypted.TenantID,
		CredentialName:     credentialEncrypted.CredentialName,
		Type:               credentialEncrypted.Type,
		CredentialMetadata: credentialEncrypted.CredentialMetadata,
		Credential:         credentialPb,
		CreatedBy:          credentialEncrypted.CreatedBy,
		CreatedAt:          credentialEncrypted.CreatedAt,
		UpdatedAt:          credentialEncrypted.UpdatedAt,
	}, nil
}

func (s *CredentialService) DeleteCredential(ctx context.Context, credentialId uuid.UUID) error {
	tenantID := auth.MustGetTenant(ctx)

	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := s.CredentialRepository.DeleteCredential(ctx, credentialId, tenantID); err != nil {
			return err
		}

		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCredentialDeleted,
			ResourceType: types.AuditResourceTypeCredential,
			ResourceID:   credentialId.String(),
		})
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("credential_id", credentialId.String()).Msg("failed to delete credential")
		return err
	}

	s.logger.Info().Ctx(ctx).Str("credential_id", credentialId.String()).Msg("Credential deleted successfully")
	return nil
}

func (s *CredentialService) ListCredentials(ctx context.Context, types []entity.CredentialType, limit int32, offset int32) ([]entity.CredentialEncrypted, error) {
	tenantID := auth.MustGetTenant(ctx)

	credentials, err := s.CredentialRepository.ListCredentials(ctx, types, limit, offset, tenantID)
	if err != nil {
		s.logger.Error().Ctx(ctx).Msg("failed to list credentials")
		return nil, err
	}

	s.logger.Debug().Ctx(ctx).Int("count", len(credentials)).Msg("Listed credentials successfully")

	return credentials, nil
}
