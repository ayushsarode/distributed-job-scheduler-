package credentialservice_test

//go:generate mockgen -destination=mocks/credential_mocks.go -package=mocks exiro.ai/application/service/internal/types CredentialRepository,EncryptionClient,TransationHandler
//go:generate mockgen -destination=mocks/audit_mocks.go -package=mocks exiro.ai/application/service/types AuditService

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/credentialservice"
	"exiro.ai/application/service/credentialservice/mocks"
	"exiro.ai/application/service/internal/types"
	servicetypes "exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"google.golang.org/protobuf/encoding/protojson"
)

func setupTest(t *testing.T) (
	context.Context,
	*credentialservice.CredentialService,
	*mocks.MockCredentialRepository,
	*mocks.MockEncryptionClient,
	*mocks.MockAuditService,
	*mocks.MockTransationHandler,
	uuid.UUID,
	string,
) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	logger := zerolog.Nop()
	ctx := logger.WithContext(context.Background())

	tenantID := uuid.Must(uuid.NewV7())
	userID := "test123"
	ctx = auth.SetTenant(ctx, tenantID)
	ctx = auth.SetUser(ctx, userID)

	mockRepo := mocks.NewMockCredentialRepository(ctrl)
	mockEncryption := mocks.NewMockEncryptionClient(ctrl)
	mockAudit := mocks.NewMockAuditService(ctrl)
	mockTransaction := mocks.NewMockTransationHandler(ctrl)

	service := credentialservice.NewCredentialService(ctx, mockRepo, mockEncryption, mockAudit, mockTransaction)

	return ctx, service, mockRepo, mockEncryption, mockAudit, mockTransaction, tenantID, userID
}

func createTestTwilioCredential() entity.Credential {
	return entity.Credential{
		Credential: &pb.Credential{
			Credential: &pb.Credential_Twilio{
				Twilio: &pb.TwilioCredential{
					AccountSid:      "AC123456789",
					AuthToken:       "secret-auth-token",
					FromPhoneNumber: "9909310929",
				},
			},
		},
		CredentialMetadata: &pb.CredentialMetadata{
			CredentialMetadata: &pb.CredentialMetadata_Twilio{
				Twilio: &pb.TwilioCredentialMetadata{
					AccountSid:      "AC123456789",
					FromPhoneNumber: "9909310929",
				},
			},
		},
	}
}

func createTestEncryptedData() *types.EncryptedData {
	return &types.EncryptedData{
		EncryptedPayload: []byte("encrypted-credential-data"),
		EncryptedDataKey: []byte("encrypted-key"),
		IV:               []byte("initialization-vector"),
	}
}

func buildExpectedCredential(
	id, tenantID uuid.UUID,
	userID, credentialName string,
	credType entity.CredentialType,
	credential entity.Credential,
	encryptedData *types.EncryptedData,
) entity.CredentialEncrypted {
	return entity.CredentialEncrypted{
		ID:                 id,
		TenantID:           tenantID,
		CredentialName:     credentialName,
		Type:               credType,
		CredentialMetadata: credential.CredentialMetadata,
		EncryptedPayload:   encryptedData.EncryptedPayload,
		EncryptedDataKey:   encryptedData.EncryptedDataKey,
		Nonce:              encryptedData.IV,
		CreatedBy:          userID,
	}
}

func TestSetCredential_Success(t *testing.T) {
	ctx, service, mockRepo, mockEncryption, mockAudit, mockTransaction, tenantID, userID := setupTest(t)

	credentialName := "twilio123"
	credType := entity.CredentialTypeTwilio
	credential := createTestTwilioCredential()
	encryptedData := createTestEncryptedData()

	mockEncryption.EXPECT().
		Encrypt(ctx, gomock.Any()).
		Return(encryptedData, nil).
		Times(1)

	expectedID := uuid.Must(uuid.NewV7())
	expectedResult := buildExpectedCredential(
		expectedID, tenantID, userID, credentialName,
		credType, credential, encryptedData,
	)

	mockTransaction.EXPECT().
		WithTransaction(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, callback func(context.Context) error) error {
			return callback(ctx)
		}).
		Times(1)

	mockRepo.EXPECT().
		CreateCredential(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, cred entity.CredentialEncrypted) (entity.CredentialEncrypted, error) {
			assert.Equal(t, credentialName, cred.CredentialName)
			assert.Equal(t, credType, cred.Type)
			assert.Equal(t, tenantID, cred.TenantID)
			assert.Equal(t, userID, cred.CreatedBy)
			return expectedResult, nil
		}).
		Times(1)

	mockAudit.EXPECT().
		Log(ctx, servicetypes.AuditEvent{
			Action:       servicetypes.AuditActionCredentialCreated,
			ResourceType: servicetypes.AuditResourceTypeCredential,
			ResourceID:   expectedID.String(),
		}).
		Return(nil).
		Times(1)

	result, err := service.SetCredential(ctx, credentialName, credential, credType)

	require.NoError(t, err)
	assert.Equal(t, expectedID, result.ID)
	assert.Equal(t, credentialName, result.CredentialName)
	assert.Equal(t, credType, result.Type)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, userID, result.CreatedBy)
	assert.NotEmpty(t, result.EncryptedPayload)
}

func TestSetCredential_EmptyName_ReturnsBadRequestError(t *testing.T) {
	ctx, service, _, _, _, _, _, _ := setupTest(t)

	credential := entity.Credential{
		Credential: &pb.Credential{
			Credential: &pb.Credential_Twilio{
				Twilio: &pb.TwilioCredential{
					AccountSid:      "AC123456789",
					AuthToken:       "secret-token",
					FromPhoneNumber: "1234567890",
				},
			},
		},
	}

	result, err := service.SetCredential(ctx, "", credential, entity.CredentialTypeTwilio)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "credential name cannot be empty")
	assert.Equal(t, entity.CredentialEncrypted{}, result)
}

func TestSetCredential_EncryptionFails_ReturnsInternalError(t *testing.T) {
	ctx, service, _, mockEncryption, _, _, _, _ := setupTest(t)

	credentialName := "test123"
	credential := entity.Credential{
		Credential: &pb.Credential{
			Credential: &pb.Credential_Twilio{
				Twilio: &pb.TwilioCredential{
					AccountSid:      "AC123456789",
					AuthToken:       "secret-auth-token",
					FromPhoneNumber: "9909310929",
				},
			},
		},
	}

	encryptionError := xerrors.InternalError(ctx, errors.New("failed to generate key"))

	mockEncryption.EXPECT().
		Encrypt(ctx, gomock.Any()).
		Return(nil, encryptionError).
		Times(1)

	result, err := service.SetCredential(ctx, credentialName, credential, entity.CredentialTypeTwilio)

	require.Error(t, err)
	assert.Equal(t, encryptionError, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, entity.CredentialEncrypted{}, result)
}

func TestSetCredential_RepositoryReturnsConflictError(t *testing.T) {
	ctx, service, mockRepo, mockEncryption, _, mockTransaction, _, _ := setupTest(t)

	credentialName := "duplicate-credential"
	credential := entity.Credential{
		Credential: &pb.Credential{
			Credential: &pb.Credential_Twilio{
				Twilio: &pb.TwilioCredential{
					AccountSid:      "AC123456789",
					AuthToken:       "secret-auth-token",
					FromPhoneNumber: "9909310929",
				},
			},
		},
	}

	encryptedData := &types.EncryptedData{
		EncryptedPayload: []byte("encrypted-data"),
		EncryptedDataKey: []byte("encrypted-key"),
		IV:               []byte("iv"),
	}

	mockEncryption.EXPECT().
		Encrypt(ctx, gomock.Any()).
		Return(encryptedData, nil).
		Times(1)

	mockTransaction.EXPECT().
		WithTransaction(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, callback func(context.Context) error) error {
			return callback(ctx)
		}).
		Times(1)

	repoError := xerrors.ConflictError(ctx, errors.New("credential already exists"))
	mockRepo.EXPECT().
		CreateCredential(ctx, gomock.Any()).
		Return(entity.CredentialEncrypted{}, repoError).
		Times(1)

	result, err := service.SetCredential(ctx, credentialName, credential, entity.CredentialTypeTwilio)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsConflictError(err))
	assert.Contains(t, err.Error(), "credential already exists")
	assert.Equal(t, entity.CredentialEncrypted{}, result)
}

func TestSetCredential_RepositoryReturnsInternalError(t *testing.T) {
	ctx, service, mockRepo, mockEncryption, _, mockTransaction, _, _ := setupTest(t)

	credentialName := "test123"
	credential := entity.Credential{
		Credential: &pb.Credential{
			Credential: &pb.Credential_Twilio{
				Twilio: &pb.TwilioCredential{
					AccountSid:      "AC123456789",
					AuthToken:       "secret-auth-token",
					FromPhoneNumber: "9909310929",
				},
			},
		},
	}

	encryptedData := &types.EncryptedData{
		EncryptedPayload: []byte("encrypted-data"),
		EncryptedDataKey: []byte("encrypted-key"),
		IV:               []byte("iv"),
	}

	mockEncryption.EXPECT().
		Encrypt(ctx, gomock.Any()).
		Return(encryptedData, nil).
		Times(1)

	mockTransaction.EXPECT().
		WithTransaction(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, callback func(context.Context) error) error {
			return callback(ctx)
		}).
		Times(1)

	repoError := xerrors.InternalError(ctx, errors.New("failed to process credential"))
	mockRepo.EXPECT().
		CreateCredential(ctx, gomock.Any()).
		Return(entity.CredentialEncrypted{}, repoError).
		Times(1)

	result, err := service.SetCredential(ctx, credentialName, credential, entity.CredentialTypeTwilio)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, entity.CredentialEncrypted{}, result)
}

func TestGetCredential_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, userID := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())
	expected := entity.CredentialEncrypted{
		ID:             credentialID,
		TenantID:       tenantID,
		CredentialName: "existing-credential",
		Type:           entity.CredentialTypeTwilio,
		CreatedBy:      userID,
		CreatedAt:      time.Now().Add(-24 * time.Hour),
		UpdatedAt:      time.Now(),
	}

	mockRepo.EXPECT().
		GetCredentialEncrypted(ctx, credentialID, tenantID).
		Return(expected, nil).
		Times(1)

	result, err := service.GetCredential(ctx, credentialID)

	require.NoError(t, err)
	assert.Equal(t, expected.ID, result.ID)
	assert.Equal(t, expected.CredentialName, result.CredentialName)
	assert.Equal(t, expected.Type, result.Type)
	assert.Equal(t, expected.TenantID, result.TenantID)
}

func TestGetCredential_RepositoryReturnsNotFoundError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())
	repoError := xerrors.NotFoundError(ctx, fmt.Errorf("credential with id %s not found", credentialID))

	mockRepo.EXPECT().
		GetCredentialEncrypted(ctx, credentialID, tenantID).
		Return(entity.CredentialEncrypted{}, repoError).
		Times(1)

	result, err := service.GetCredential(ctx, credentialID)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsNotFoundError(err))
	assert.Contains(t, err.Error(), "credential with id")
	assert.Equal(t, entity.CredentialEncrypted{}, result)
}

func TestGetCredential_RepositoryReturnsInternalError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())
	repoError := xerrors.InternalError(ctx, errors.New("failed to process credential"))

	mockRepo.EXPECT().
		GetCredentialEncrypted(ctx, credentialID, tenantID).
		Return(entity.CredentialEncrypted{}, repoError).
		Times(1)

	result, err := service.GetCredential(ctx, credentialID)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, entity.CredentialEncrypted{}, result)
}

func TestDeleteCredential_Success(t *testing.T) {
	ctx, service, mockRepo, _, mockAudit, mockTransaction, tenantID, _ := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())

	mockTransaction.EXPECT().
		WithTransaction(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, callback func(context.Context) error) error {
			return callback(ctx)
		}).
		Times(1)

	mockRepo.EXPECT().
		DeleteCredential(ctx, credentialID, tenantID).
		Return(nil).
		Times(1)

	mockAudit.EXPECT().
		Log(ctx, servicetypes.AuditEvent{
			Action:       servicetypes.AuditActionCredentialDeleted,
			ResourceType: servicetypes.AuditResourceTypeCredential,
			ResourceID:   credentialID.String(),
		}).
		Return(nil).
		Times(1)

	err := service.DeleteCredential(ctx, credentialID)

	require.NoError(t, err)
}

func TestDeleteCredential_RepositoryReturnsNotFoundError(t *testing.T) {
	ctx, service, mockRepo, _, _, mockTransaction, tenantID, _ := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())
	repoError := xerrors.NotFoundError(ctx, fmt.Errorf("credential with id %s not found", credentialID))

	mockTransaction.EXPECT().
		WithTransaction(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, callback func(context.Context) error) error {
			return callback(ctx)
		}).
		Times(1)

	mockRepo.EXPECT().
		DeleteCredential(ctx, credentialID, tenantID).
		Return(repoError).
		Times(1)

	err := service.DeleteCredential(ctx, credentialID)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsNotFoundError(err))
	assert.Contains(t, err.Error(), "credential with id")
}

func TestDeleteCredential_RepositoryReturnsInternalError(t *testing.T) {
	ctx, service, mockRepo, _, _, mockTransaction, tenantID, _ := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())
	repoError := xerrors.InternalError(ctx, errors.New("failed to process credential"))

	mockTransaction.EXPECT().
		WithTransaction(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, callback func(context.Context) error) error {
			return callback(ctx)
		}).
		Times(1)

	mockRepo.EXPECT().
		DeleteCredential(ctx, credentialID, tenantID).
		Return(repoError).
		Times(1)

	err := service.DeleteCredential(ctx, credentialID)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsInternalError(err))
}

func TestListCredentials_AllTypes_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	expected := []entity.CredentialEncrypted{
		{ID: uuid.Must(uuid.NewV7()), TenantID: tenantID, CredentialName: "twilio-prod", Type: entity.CredentialTypeTwilio},
		{ID: uuid.Must(uuid.NewV7()), TenantID: tenantID, CredentialName: "exotel-prod", Type: entity.CredentialTypeExotel},
	}

	mockRepo.EXPECT().
		ListCredentials(
			ctx,
			[]entity.CredentialType{},
			int32(50),
			int32(0),
			tenantID,
		).
		Return(expected, nil).
		Times(1)

	result, err := service.ListCredentials(
		ctx,
		[]entity.CredentialType{},
		50,
		0,
	)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestListCredentials_FilterByType_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	expected := []entity.CredentialEncrypted{
		{ID: uuid.Must(uuid.NewV7()), TenantID: tenantID, CredentialName: "twilio-prod", Type: entity.CredentialTypeTwilio},
	}

	mockRepo.EXPECT().
		ListCredentials(
			ctx,
			[]entity.CredentialType{entity.CredentialTypeTwilio},
			int32(10),
			int32(0),
			tenantID,
		).
		Return(expected, nil).
		Times(1)

	result, err := service.ListCredentials(
		ctx,
		[]entity.CredentialType{entity.CredentialTypeTwilio},
		10,
		0,
	)

	require.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, expected[0].ID, result[0].ID)
}

func TestListCredentials_EmptyList_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	mockRepo.EXPECT().
		ListCredentials(
			ctx,
			[]entity.CredentialType{},
			int32(20),
			int32(0),
			tenantID,
		).
		Return([]entity.CredentialEncrypted{}, nil).
		Times(1)

	result, err := service.ListCredentials(
		ctx,
		[]entity.CredentialType{},
		20,
		0,
	)

	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestListCredentials_RepositoryReturnsInternalError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	repoErr := xerrors.InternalError(ctx, errors.New("db failure"))

	mockRepo.EXPECT().
		ListCredentials(
			ctx,
			[]entity.CredentialType{},
			int32(50),
			int32(0),
			tenantID,
		).
		Return(nil, repoErr).
		Times(1)

	result, err := service.ListCredentials(
		ctx,
		[]entity.CredentialType{},
		50,
		0,
	)

	require.Error(t, err)
	assert.Equal(t, repoErr, err)
	assert.Nil(t, result)
}

func TestGetDecryptedCredential_Success(t *testing.T) {
	ctx, service, mockRepo, mockEncryption, _, _, tenantID, userID := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())

	credentialEncrypted := entity.CredentialEncrypted{
		ID:               credentialID,
		TenantID:         tenantID,
		CredentialName:   "test-credential",
		Type:             entity.CredentialTypeTwilio,
		EncryptedPayload: []byte("encrypted-payload"),
		EncryptedDataKey: []byte("encrypted-key"),
		Nonce:            []byte("nonce"),
		CredentialMetadata: &pb.CredentialMetadata{
			CredentialMetadata: &pb.CredentialMetadata_Twilio{
				Twilio: &pb.TwilioCredentialMetadata{
					AccountSid:      "AC123456789",
					FromPhoneNumber: "9909310929",
				},
			},
		},
		CreatedBy: userID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	mockRepo.EXPECT().
		INTERNALgetCredential(ctx, credentialID).
		Return(credentialEncrypted, nil).
		Times(1)

	credentialPb := &pb.Credential{
		Credential: &pb.Credential_Twilio{
			Twilio: &pb.TwilioCredential{
				AccountSid:      "AC123456789",
				AuthToken:       "secret-token",
				FromPhoneNumber: "9909310929",
			},
		},
	}

	decryptedJSON, _ := protojson.Marshal(credentialPb)

	mockEncryption.EXPECT().
		Decrypt(ctx, gomock.Any()).
		Return(decryptedJSON, nil).
		Times(1)

	result, err := service.GetDecryptedCredential(ctx, credentialID)

	require.NoError(t, err)
	assert.Equal(t, credentialID, result.ID)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, "test-credential", result.CredentialName)
	assert.Equal(t, entity.CredentialTypeTwilio, result.Type)
	assert.NotNil(t, result.Credential)
}

func TestGetDecryptedCredential_RepositoryReturnsNotFoundError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, _, _ := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())
	repoError := xerrors.NotFoundError(ctx, fmt.Errorf("credential with id %s not found", credentialID))

	mockRepo.EXPECT().
		INTERNALgetCredential(ctx, credentialID).
		Return(entity.CredentialEncrypted{}, repoError).
		Times(1)

	result, err := service.GetDecryptedCredential(ctx, credentialID)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsNotFoundError(err))
	assert.Contains(t, err.Error(), "credential with id")
	assert.Equal(t, entity.Credential{}, result)
}

func TestGetDecryptedCredential_RepositoryReturnsInternalError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, _, _ := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())
	repoError := xerrors.InternalError(ctx, errors.New("failed to process credential"))

	mockRepo.EXPECT().
		INTERNALgetCredential(ctx, credentialID).
		Return(entity.CredentialEncrypted{}, repoError).
		Times(1)

	result, err := service.GetDecryptedCredential(ctx, credentialID)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, entity.Credential{}, result)
}

func TestGetDecryptedCredential_DecryptionFails(t *testing.T) {
	ctx, service, mockRepo, mockEncryption, _, _, tenantID, userID := setupTest(t)

	credentialID := uuid.Must(uuid.NewV7())

	credentialEncrypted := entity.CredentialEncrypted{
		ID:               credentialID,
		TenantID:         tenantID,
		CredentialName:   "test-credential",
		Type:             entity.CredentialTypeTwilio,
		EncryptedPayload: []byte("encrypted-payload"),
		EncryptedDataKey: []byte("encrypted-key"),
		Nonce:            []byte("nonce"),
		CreatedBy:        userID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	mockRepo.EXPECT().
		INTERNALgetCredential(ctx, credentialID).
		Return(credentialEncrypted, nil).
		Times(1)

	decryptionError := xerrors.InternalError(ctx, errors.New("failed to decrypt data key"))

	mockEncryption.EXPECT().
		Decrypt(ctx, gomock.Any()).
		Return(nil, decryptionError).
		Times(1)

	result, err := service.GetDecryptedCredential(ctx, credentialID)

	require.Error(t, err)
	assert.Equal(t, decryptionError, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, entity.Credential{}, result)
}
