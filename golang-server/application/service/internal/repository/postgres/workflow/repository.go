package workflow

import (
	"context"

	xerrors "exiro.ai/application/errors"
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
)

type Repository struct {
	logger *zerolog.Logger
	db     *gen.Queries
	pool   *pgxpool.Pool
}

var _ (types.WorkflowRepository) = (*Repository)(nil)

func NewRepository(ctx context.Context) *Repository {
	pool := postgres.Ctx(ctx)

	return &Repository{
		logger: zerolog.Ctx(ctx),
		db:     gen.New(postgres.Ctx(ctx)),
		pool:   pool,
	}
}

// getQueries returns the appropriate Queries instance - either with transaction or the default one.
func (r *Repository) getQueries(ctx context.Context) *gen.Queries {
	if tx, ok := transaction.TxFromContext(ctx); ok {
		return r.db.WithTx(tx)
	}
	return r.db
}

func (r *Repository) CreateWorkflow(ctx context.Context, workflow entity.Workflow) error {
	pgWorkflow := r.mapToPostgresWorkflow(ctx, workflow)

	_, err := r.db.InsertWorkflow(ctx, gen.InsertWorkflowParams{
		ID:          pgWorkflow.ID,
		Name:        pgWorkflow.Name,
		Description: pgWorkflow.Description,
		AgentID:     pgWorkflow.AgentID,
		CreatedBy:   pgWorkflow.CreatedBy,
		Status:      pgWorkflow.Status,
		TenantID:    pgWorkflow.TenantID,
		CreatedAt:   pgWorkflow.CreatedAt,
		UpdatedAt:   pgWorkflow.UpdatedAt,
	})

	if err != nil {
		return r.mapPgError(ctx, err, pgWorkflow.ID.String())
	}

	return nil
}

func (r *Repository) GetWorkflow(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (entity.Workflow, error) {
	row, err := r.db.GetWorkflow(ctx, gen.GetWorkflowParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return entity.Workflow{}, r.mapPgError(ctx, err, id.String())
	}

	workflow, err := r.mapToWorkflowEntity(ctx, row)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map workflow entity")
		return entity.Workflow{}, xerrors.InternalError(ctx, err)
	}

	return workflow, nil
}

func (r *Repository) INTERNALGetWorkflow(ctx context.Context, workflowId uuid.UUID) (entity.Workflow, error) {
	queries := r.getQueries(ctx)

	workflow, err := queries.INTERNALGetWorkflow(ctx, workflowId)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map workflow entity")
		return entity.Workflow{}, xerrors.InternalError(ctx, err)
	}

	return r.mapToWorkflowEntity(ctx, workflow)
}

func (r *Repository) UpdateWorkflow(ctx context.Context, workflow entity.Workflow, tenantID uuid.UUID) error {
	pgWorkflow := r.mapToPostgresWorkflow(ctx, workflow)

	err := r.db.UpdateWorkflow(ctx, gen.UpdateWorkflowParams{
		Name:        pgWorkflow.Name,
		Description: pgWorkflow.Description,
		AgentID:     pgWorkflow.AgentID,
		CreatedBy:   pgWorkflow.CreatedBy,
		Status:      pgWorkflow.Status,
		ID:          pgWorkflow.ID,
		TenantID:    tenantID,
		CreatedAt:   pgWorkflow.CreatedAt,
		UpdatedAt:   pgWorkflow.UpdatedAt,
	})
	if err != nil {
		return r.mapPgError(ctx, err, pgWorkflow.ID.String())
	}
	return nil
}

func (r *Repository) DeleteWorkflow(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	err := r.db.DeleteWorkflow(ctx, gen.DeleteWorkflowParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return r.mapPgError(ctx, err, id.String())
	}

	return nil
}

func (r *Repository) ListWorkflows(
	ctx context.Context,
	statuses []entity.WorkflowStatus,
	limit int32,
	offset int32,
	tenantID uuid.UUID,
) ([]entity.Workflow, error) {
	pool := r.pool

	sbBuilder := sb.PostgreSQL.NewSelectBuilder()
	sbBuilder.
		Select(
			"id",
			"name",
			"description",
			"agent_id",
			"status",
			"created_by",
			"tenant_id",
			"created_at",
			"updated_at",
		).
		From("workflows")
	// apply filters
	applyWorkflowFilters(sbBuilder, tenantID, statuses)
	sbBuilder.OrderBy("id DESC")

	// add pagination
	applyPagination(sbBuilder, limit, offset)

	query, args := sbBuilder.Build()

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return nil, r.mapPgError(ctx, err, "")
	}
	defer rows.Close()

	workflows := make([]entity.Workflow, 0)
	rowCount := 0
	for rows.Next() {
		rowCount++
		wf, err := scanWorkflowRow(rows)
		if err != nil {
			r.logger.Error().Ctx(ctx).
				Err(err).
				Int("row_number", rowCount).
				Msg("Failed to scan workflow row")
			return nil, r.mapPgError(ctx, err, "")
		}
		workflows = append(workflows, wf)
	}
	if err := rows.Err(); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Error iterating rows")
		return nil, r.mapPgError(ctx, err, "")
	}
	return workflows, nil
}

// get active and total count of calljobs for a workflow.
func (r *Repository) GetWorkflowCallJobCount(ctx context.Context, workflowId uuid.UUID, tenantID uuid.UUID) (int32, error) {
	count, err := r.getQueries(ctx).GetWorkflowCallJobCount(ctx, gen.GetWorkflowCallJobCountParams{
		WorkflowID: workflowId.String(),
		TenantID:   tenantID,
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to get calljobs count from workflowId")
		return 0, r.mapPgError(ctx, err, "")
	}

	return count, nil
}

// get count of calljobs for a workflow by given statuses.
func (r *Repository) GetWorkflowCallJobCountByStatuses(ctx context.Context, workflowId uuid.UUID, statuses []entity.CallJobStatus, tenantID uuid.UUID) (int32, error) {
	statusesStrings := make([]string, len(statuses))
	for i, s := range statuses {
		statusesStrings[i] = s.String()
	}
	count, err := r.getQueries(ctx).GetWorkflowCallJobCountByStatuses(ctx, gen.GetWorkflowCallJobCountByStatusesParams{
		WorkflowID: workflowId.String(),
		TenantID:   tenantID,
		Statuses:   statusesStrings,
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to get calljobs count from workflowId by statuses")
		return 0, r.mapPgError(ctx, err, "")
	}
	return count, nil
}

// get published and total workflows for a given agent.
func (r *Repository) HasPublishedWorkflowsForAgent(ctx context.Context, agentId string, tenantID uuid.UUID) (bool, error) {
	hasPublished, err := r.getQueries(ctx).HasPublishedWorkflowsForAgent(ctx, gen.HasPublishedWorkflowsForAgentParams{
		Column1:  agentId,
		TenantID: tenantID,
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to get calljobs count from workflowId by statuses")
		return false, r.mapPgError(ctx, err, "")
	}
	return hasPublished, nil
}

func scanWorkflowRow(rows pgx.Rows) (entity.Workflow, error) {
	var wf entity.Workflow
	var statusStr string

	err := rows.Scan(
		&wf.ID,
		&wf.Name,
		&wf.Description,
		&wf.Agent_id,
		&statusStr,
		&wf.CreatedBy,
		&wf.TenantID,
		&wf.CreatedAt,
		&wf.UpdatedAt,
	)
	if err != nil {
		return entity.Workflow{}, err
	}

	wf.Status, err = entity.ParseWorkflowStatus(statusStr)
	if err != nil {
		return entity.Workflow{}, err
	}

	return wf, nil
}

func applyWorkflowFilters(builder *sb.SelectBuilder, tenantID uuid.UUID, statuses []entity.WorkflowStatus) {
	builder.Where(builder.Equal("tenant_id", tenantID))

	// add status filter if statuses are provided
	if len(statuses) > 0 {
		statusStrings := make([]any, len(statuses))
		for i, status := range statuses {
			statusStrings[i] = status.String()
		}
		builder.Where(builder.In("status", statusStrings...))
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

func (r *Repository) GetWorkflowCount(
	ctx context.Context,
	statuses []entity.WorkflowStatus,
	tenantID uuid.UUID,
) (int32, error) {
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()
	sbBuilder.
		Select("COUNT(*)").
		From("workflows")

	// apply filters
	applyWorkflowFilters(sbBuilder, tenantID, statuses)

	query, args := sbBuilder.Build()

	var count int32
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		r.logger.Error().Ctx(ctx).
			Err(err).
			Str("query", query).
			Msg("Failed to get workflow count")
		return 0, r.mapPgError(ctx, err, "")
	}

	return count, nil
}
