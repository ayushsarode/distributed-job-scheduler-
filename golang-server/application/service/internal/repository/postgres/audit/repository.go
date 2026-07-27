package audit

import (
	"context"
	"errors"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
	sb "github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type Repository struct {
	logger *zerolog.Logger
	db     *gen.Queries
	pool   *pgxpool.Pool
}

var _ types.AuditRepository = (*Repository)(nil)

func (r *Repository) getQueries(ctx context.Context) (*gen.Queries, error) {
	tx, ok := transaction.TxFromContext(ctx)
	if !ok {
		return nil, xerrors.InternalError(ctx, errors.New("audit: must be called within a transaction"))
	}
	return r.db.WithTx(tx), nil
}

func (r *Repository) InsertAuditLog(ctx context.Context, log entity.AuditLog) error {
	queries, err := r.getQueries(ctx)
	if err != nil {
		return err
	}

	_, err = queries.InsertAuditLog(ctx, gen.InsertAuditLogParams{
		ID:           log.ID,
		TenantID:     log.TenantID,
		UserID:       log.UserID,
		Action:       log.Action,
		ResourceType: log.ResourceType,
		ResourceID:   log.ResourceID,
		CreatedAt:    pgtype.Timestamp{Time: log.CreatedAt, Valid: true},
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to insert audit log")
		return xerrors.InternalError(ctx, errors.New("failed to insert audit log"))
	}

	return nil
}

func applyAuditLogFilters(builder *sb.SelectBuilder, tenantID uuid.UUID, params entity.AuditLogQuery) {
	builder.Where(builder.Equal("tenant_id", tenantID))

	if params.ResourceType != nil {
		builder.Where(builder.Equal("resource_type", *params.ResourceType))
	}
	if params.UserID != nil {
		builder.Where(builder.Equal("user_id", *params.UserID))
	}
	if params.StartTime != nil {
		builder.Where(builder.GE("created_at", *params.StartTime))
	}
	if params.EndTime != nil {
		builder.Where(builder.LE("created_at", *params.EndTime))
	}
}

func (r *Repository) ListAuditLogs(ctx context.Context, tenantID uuid.UUID, params entity.AuditLogQuery) ([]entity.AuditLog, error) {
	listBuilder := sb.PostgreSQL.NewSelectBuilder()
	listBuilder.
		Select("id", "tenant_id", "user_id", "action", "resource_type", "resource_id", "created_at").
		From("audit_logs")
	applyAuditLogFilters(listBuilder, tenantID, params)
	listBuilder.OrderBy("id DESC")

	if params.Limit > 0 {
		listBuilder.Limit(int(params.Limit))
	}
	if params.Offset > 0 {
		listBuilder.Offset(int(params.Offset))
	}

	listQuery, listArgs := listBuilder.Build()
	rows, err := r.pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to list audit logs")
		return nil, xerrors.InternalError(ctx, errors.New("failed to list audit logs"))
	}
	defer rows.Close()

	var logs []entity.AuditLog
	for rows.Next() {
		var log entity.AuditLog
		var createdAt pgtype.Timestamp
		err := rows.Scan(
			&log.ID,
			&log.TenantID,
			&log.UserID,
			&log.Action,
			&log.ResourceType,
			&log.ResourceID,
			&createdAt,
		)
		if err != nil {
			r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to scan audit log row")
			return nil, xerrors.InternalError(ctx, errors.New("failed to scan audit log"))
		}
		log.CreatedAt = createdAt.Time
		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Error iterating audit log rows")
		return nil, xerrors.InternalError(ctx, errors.New("failed to list audit logs"))
	}

	return logs, nil
}

func NewRepository(ctx context.Context) *Repository {
	return &Repository{
		logger: zerolog.Ctx(ctx),
		db:     gen.New(postgres.Ctx(ctx)),
		pool:   postgres.Ctx(ctx),
	}
}
