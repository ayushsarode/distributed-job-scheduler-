package usermanagement

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
)

type UserRepository struct {
	db     *gen.Queries
	logger *zerolog.Logger
}

// getQueries returns the appropriate Queries instance - either with transaction or the default one.
func (u *UserRepository) getQueries(ctx context.Context) *gen.Queries {
	if tx, ok := transaction.TxFromContext(ctx); ok {
		return u.db.WithTx(tx)
	}
	return u.db
}

var _ (types.UserManagementRepository) = (*UserRepository)(nil)

func (u *UserRepository) RegisterUser(ctx context.Context, user entity.User) (string, error) {
	// Inserting the new User into db
	queries := u.getQueries(ctx)
	_, err := queries.RegisterUser(ctx, gen.RegisterUserParams{
		UserID:    user.UserID,
		FirstName: pgtype.Text{String: user.FirstName, Valid: true},
		LastName:  pgtype.Text{String: user.LastName, Valid: true},
		TenantID:  user.TenantID,
	})
	if err != nil {
		u.logger.Error().Ctx(ctx).Err(err).Msg("Failed to insert user into database")
		return "", err
	}
	u.logger.Info().Ctx(ctx).Msgf("User Registered: %+v", user)
	return user.UserID, nil
}

func (u *UserRepository) CreateTenant(ctx context.Context, tenant entity.Tenant) (entity.Tenant, error) {
	queries := u.getQueries(ctx)
	result, err := queries.InsertTenant(ctx, gen.InsertTenantParams{
		ID:        tenant.ID,
		Name:      tenant.Name,
		CreatedAt: pgtype.Timestamp{Time: time.Now(), Valid: true},
		UpdatedAt: pgtype.Timestamp{Time: time.Now(), Valid: true},
		IsActive:  tenant.IsActive,
	})
	if err != nil {
		u.logger.Error().Ctx(ctx).Err(err).Msg("Failed to create tenant")
		return entity.Tenant{}, xerrors.InternalError(ctx, errors.New("failed to create tenant"))
	}
	u.logger.Info().Ctx(ctx).Msgf("Tenant created: %+v", result)
	return entity.Tenant{
		ID:        result.ID,
		Name:      result.Name,
		CreatedAt: result.CreatedAt.Time,
		UpdatedAt: result.UpdatedAt.Time,
		IsActive:  result.IsActive,
	}, nil
}

func (u *UserRepository) GetUserTenantID(ctx context.Context, userID string) (uuid.UUID, error) {
	queries := u.getQueries(ctx)
	tenantID, err := queries.GetUserTenantID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return uuid.Nil, xerrors.NotFoundError(ctx, errors.New("tenant not found"))
		}
		u.logger.Error().Ctx(ctx).Err(err).Msgf("Failed to get tenant ID for user %s", userID)
		return uuid.Nil, xerrors.InternalError(ctx, fmt.Errorf("failed to get tenant ID for user %s", userID))
	}
	return tenantID, nil
}

func NewRepository(ctx context.Context) *UserRepository {
	return &UserRepository{
		logger: zerolog.Ctx(ctx),
		db:     gen.New(postgres.Ctx(ctx)),
	}
}
