package credential

import (
	"context"
	"errors"
	"fmt"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"
)

func (r *Repository) mapToPostgresCredential(_ context.Context, credentialEncrypted entity.CredentialEncrypted) gen.Credential {
	var metadata []byte
	if credentialEncrypted.CredentialMetadata != nil {
		if b, err := protojson.Marshal(credentialEncrypted.CredentialMetadata); err == nil {
			metadata = b
		}
	}

	return gen.Credential{
		ID:                 credentialEncrypted.ID,
		TenantID:           credentialEncrypted.TenantID,
		CredentialName:     credentialEncrypted.CredentialName,
		CredentialType:     credentialEncrypted.Type.String(),
		CredentialMetadata: metadata,
		EncryptedPayload:   credentialEncrypted.EncryptedPayload,
		EncryptedDataKey:   credentialEncrypted.EncryptedDataKey,
		Nonce:              credentialEncrypted.Nonce,
		CreatedBy:          credentialEncrypted.CreatedBy,
		CreatedAt:          pgtype.Timestamp{Time: credentialEncrypted.CreatedAt, Valid: !credentialEncrypted.CreatedAt.IsZero()},
		UpdatedAt:          pgtype.Timestamp{Time: credentialEncrypted.UpdatedAt, Valid: !credentialEncrypted.UpdatedAt.IsZero()},
	}
}

func (r *Repository) mapToCredentialEntity(ctx context.Context, credential gen.Credential) (entity.CredentialEncrypted, error) {
	credentialType, err := entity.ParseCredentialType(credential.CredentialType)
	if err != nil {
		r.logger.Error().Err(err).Str("credentialType", credential.CredentialType).Msg("Failed to parse credential type")
		return entity.CredentialEncrypted{}, xerrors.InternalError(ctx, fmt.Errorf("invalid credential type in database for credential %s", credential.ID))
	}

	// Unmarshal credential metadata from JSON
	var credentialMetadata *pb.CredentialMetadata
	if len(credential.CredentialMetadata) > 0 {
		credentialMetadata = &pb.CredentialMetadata{}
		if err := protojson.Unmarshal(credential.CredentialMetadata, credentialMetadata); err != nil {
			r.logger.Error().Err(err).Msg("Failed to unmarshal credential metadata")
			// Don't fail the whole operation, just set metadata to nil
			credentialMetadata = nil
		}
	}

	credentialEntity := entity.CredentialEncrypted{
		ID:                 credential.ID,
		TenantID:           credential.TenantID,
		CredentialName:     credential.CredentialName,
		Type:               credentialType,
		CredentialMetadata: credentialMetadata,
		EncryptedPayload:   credential.EncryptedPayload,
		EncryptedDataKey:   credential.EncryptedDataKey,
		Nonce:              credential.Nonce,
		CreatedBy:          credential.CreatedBy,
		CreatedAt:          credential.CreatedAt.Time,
		UpdatedAt:          credential.UpdatedAt.Time,
	}

	return credentialEntity, nil
}

// TODO: Postgres -> xerrors mapping needs to be moved to a common place.
const (
	pgUniqueViolation = "23505"
)

func (r *Repository) mapPgError(ctx context.Context, err error, resourceID string) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return xerrors.NotFoundError(ctx, fmt.Errorf("credential with id %s not found", resourceID))
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == pgUniqueViolation {
			return xerrors.ConflictError(ctx, errors.New("credential already exists"))
		}
	}

	r.logger.Error().Err(err).Msg("Database error for credential")
	return xerrors.InternalError(ctx, errors.New("failed to process credential"))
}
