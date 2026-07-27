package credential

import (
	"context"
	"errors"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
	sb "github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
)

type Repository struct {
	logger *zerolog.Logger
	db     *gen.Queries
	pool   *pgxpool.Pool
}

func NewRepository(ctx context.Context) *Repository {
	pool := postgres.Ctx(ctx)

	return &Repository{
		logger: zerolog.Ctx(ctx),
		db:     gen.New(postgres.Ctx(ctx)),
		pool:   pool,
	}
}

var _ (types.CredentialRepository) = (*Repository)(nil)

func (r *Repository) getQueries(ctx context.Context) *gen.Queries {
	if tx, ok := transaction.TxFromContext(ctx); ok {
		return r.db.WithTx(tx)
	}
	return r.db
}

func (r *Repository) CreateCredential(ctx context.Context, credentialEncrypted entity.CredentialEncrypted) (entity.CredentialEncrypted, error) {
	queries := r.getQueries(ctx)
	pgCredential := r.mapToPostgresCredential(ctx, credentialEncrypted)

	insertedCredential, err := queries.InsertCredential(ctx, gen.InsertCredentialParams(pgCredential))
	if err != nil {
		return entity.CredentialEncrypted{}, r.mapPgError(ctx, err, credentialEncrypted.ID.String())
	}

	credential, err := r.mapToCredentialEntity(ctx, insertedCredential)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map credential to entity")
		return entity.CredentialEncrypted{}, xerrors.InternalError(ctx, errors.New("internal error occurred"))
	}

	return credential, nil
}

func (r *Repository) UpdateCredential(ctx context.Context, credentialEncrypted entity.CredentialEncrypted) (entity.CredentialEncrypted, error) {
	queries := r.getQueries(ctx)
	pgCredential := r.mapToPostgresCredential(ctx, credentialEncrypted)

	updatedCredential, err := queries.UpdateCredentials(ctx, gen.UpdateCredentialsParams{
		ID:               pgCredential.ID,
		TenantID:         pgCredential.TenantID,
		CredentialName:   pgCredential.CredentialName,
		CredentialType:   pgCredential.CredentialType,
		EncryptedPayload: pgCredential.EncryptedPayload,
		EncryptedDataKey: pgCredential.EncryptedDataKey,
		Nonce:            pgCredential.Nonce,
		CreatedBy:        pgCredential.CreatedBy,
		CreatedAt:        pgCredential.CreatedAt,
		UpdatedAt:        pgCredential.UpdatedAt,
	})
	if err != nil {
		return entity.CredentialEncrypted{}, r.mapPgError(ctx, err, credentialEncrypted.ID.String())
	}

	credential, err := r.mapToCredentialEntity(ctx, updatedCredential)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map credential to entity")
		return entity.CredentialEncrypted{}, xerrors.InternalError(ctx, errors.New("internal error occurred"))
	}

	return credential, nil
}

func (r *Repository) GetCredentialEncrypted(ctx context.Context, credentialID uuid.UUID, tenantID uuid.UUID) (entity.CredentialEncrypted, error) {
	queries := r.getQueries(ctx)
	credential, err := queries.GetCredentialEncrypted(ctx, gen.GetCredentialEncryptedParams{
		ID:       credentialID,
		TenantID: tenantID,
	})
	if err != nil {
		return entity.CredentialEncrypted{}, r.mapPgError(ctx, err, credentialID.String())
	}
	return r.mapToCredentialEntity(ctx, credential)
}

func (r *Repository) INTERNALgetCredential(ctx context.Context, credentialID uuid.UUID) (entity.CredentialEncrypted, error) {
	queries := r.getQueries(ctx)
	credential, err := queries.INTERNALgetCredential(ctx, credentialID)
	if err != nil {
		return entity.CredentialEncrypted{}, r.mapPgError(ctx, err, credentialID.String())
	}
	return r.mapToCredentialEntity(ctx, credential)
}

func (r *Repository) DeleteCredential(ctx context.Context, credentialID uuid.UUID, tenantID uuid.UUID) error {
	queries := r.getQueries(ctx)
	err := queries.DeleteCredential(ctx, gen.DeleteCredentialParams{
		ID:       credentialID,
		TenantID: tenantID,
	})
	if err != nil {
		return r.mapPgError(ctx, err, credentialID.String())
	}
	return nil
}

func (r *Repository) ListCredentials(ctx context.Context, types []entity.CredentialType, limit int32, offset int32, tenantID uuid.UUID) ([]entity.CredentialEncrypted, error) {
	pool := r.pool
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()

	sbBuilder.
		Select(
			"id",
			"credential_name",
			"credential_type",
			"credential_metadata",
			"encrypted_payload",
			"encrypted_data_key",
			"nonce",
			"created_by",
			"created_at",
			"updated_at",
			"tenant_id",
		).From("credentials")

	applyCredentialFilters(sbBuilder, tenantID, types)

	sbBuilder.OrderBy("id DESC")

	applyPagination(sbBuilder, limit, offset)

	query, args := sbBuilder.Build()

	r.logger.Debug().Ctx(ctx).Str("query", query).Interface("args", args).Int32("limit", limit).Int32("offset", offset).Msg("Executing ListCredentials query")

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Str("query", query).Msg("Failed to execute ListCredentials query")
		return nil, r.mapPgError(ctx, err, "")
	}
	defer rows.Close()

	credentials := make([]entity.CredentialEncrypted, 0)
	rowCount := 0
	for rows.Next() {
		rowCount++
		cred, err := scanCredentialRow(rows)
		if err != nil {
			r.logger.Error().Ctx(ctx).Err(err).Int("row_number", rowCount).Msg("Failed to scan credential row")
			return nil, r.mapPgError(ctx, err, "")
		}
		credentials = append(credentials, cred)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Error iterating credential rows")
		return nil, r.mapPgError(ctx, err, "")
	}

	r.logger.Debug().Ctx(ctx).Int("count", len(credentials)).Msg("Successfully listed credentials")

	return credentials, nil
}

func scanCredentialRow(
	rows pgx.Rows,
) (entity.CredentialEncrypted, error) {
	var cr entity.CredentialEncrypted
	var credentialTypeStr string
	var credentialMetadata []byte

	err := rows.Scan(
		&cr.ID,
		&cr.CredentialName,
		&credentialTypeStr,
		&credentialMetadata,
		&cr.EncryptedPayload,
		&cr.EncryptedDataKey,
		&cr.Nonce,
		&cr.CreatedBy,
		&cr.CreatedAt,
		&cr.UpdatedAt,
		&cr.TenantID,
	)
	if err != nil {
		return entity.CredentialEncrypted{}, err
	}

	cr.Type, err = entity.ParseCredentialType(credentialTypeStr)
	if err != nil {
		return entity.CredentialEncrypted{}, err
	}

	req := &pb.CredentialMetadata{}
	if err := protojson.Unmarshal(credentialMetadata, req); err != nil {
		return entity.CredentialEncrypted{}, err
	}
	cr.CredentialMetadata = req

	return cr, nil
}

func applyCredentialFilters(builder *sb.SelectBuilder, tenantID uuid.UUID, types []entity.CredentialType) {
	builder.Where(builder.Equal("tenant_id", tenantID))

	// add status filter if statuses are provided
	if len(types) > 0 {
		typeStrings := make([]any, len(types))
		for i, t := range types {
			typeStrings[i] = t.String()
		}

		builder.Where(builder.In("credential_type", typeStrings...))
	}
}

func applyPagination(builder *sb.SelectBuilder, limit int32, offset int32) {
	const maxLimit = 50
	effectiveLimit := limit

	if effectiveLimit <= 0 || effectiveLimit > maxLimit {
		effectiveLimit = maxLimit
	}

	builder.Limit(int(effectiveLimit))

	if offset > 0 {
		builder.Offset(int(offset))
	}
}
