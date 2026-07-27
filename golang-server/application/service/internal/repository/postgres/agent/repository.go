package agent

import (
	"context"
	"time"

	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
	sb "github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
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

// getQueries returns the appropriate Queries instance - either with transaction or the default one.
func (r *Repository) getQueries(ctx context.Context) *gen.Queries {
	if tx, ok := transaction.TxFromContext(ctx); ok {
		return r.db.WithTx(tx)
	}
	return r.db
}

var _ (types.AgentRepository) = (*Repository)(nil)

func (r *Repository) InsertAgent(ctx context.Context, agent entity.Agent) (entity.Agent, error) {
	postgresAgent, err := r.mapToPostgresAgent(ctx, agent)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map agent to postgres agent")
		return entity.Agent{}, err
	}

	queries := r.getQueries(ctx)
	insertedAgent, err := queries.InsertAgent(ctx, gen.InsertAgentParams{
		ID:                     agent.ID,
		Name:                   postgresAgent.Name,
		CreateAgentRequestJson: postgresAgent.CreateAgentRequestJson,
		CreatedAt:              postgresAgent.CreatedAt,
		UpdatedAt:              postgresAgent.UpdatedAt,
		CreatedBy:              postgresAgent.CreatedBy,
		DeploymentStatus:       postgresAgent.DeploymentStatus,
		TenantID:               agent.TenantID,
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to insert agent into database")
		return entity.Agent{}, r.mapPgError(ctx, err)
	}

	agent, err = r.mapToAgentEntity(ctx, insertedAgent) // re-assigning the agent to the returned agent
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map agent to entity")
		return entity.Agent{}, err
	}

	return agent, nil
}

func (r *Repository) GetAgent(ctx context.Context, agentID string, tenantID uuid.UUID) (entity.Agent, error) {
	queries := r.getQueries(ctx)
	agent, err := queries.GetAgent(ctx, gen.GetAgentParams{
		ID:       agentID,
		TenantID: tenantID,
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to get agent from database")
		return entity.Agent{}, r.mapPgError(ctx, err)
	}

	return r.mapToAgentEntity(ctx, agent)
}

func (r *Repository) INTERNALGetAgent(ctx context.Context, agentID string) (entity.Agent, error) {
	queries := r.getQueries(ctx)
	agent, err := queries.INTERNALGetAgent(ctx, agentID)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to get agent from database")
		return entity.Agent{}, r.mapPgError(ctx, err)
	}

	return r.mapToAgentEntity(ctx, agent)
}

func (r *Repository) ListAgents(ctx context.Context, tenantID uuid.UUID, statuses []entity.DeploymentStatus, limit int32, offset int32) ([]entity.Agent, error) {
	pool := r.pool
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()

	sbBuilder.Select(
		"id",
		"name",
		"create_agent_request_json",
		"created_by",
		"created_at",
		"updated_at",
		"tenant_id",
		"deployment_status",
	).From("agent")
	applyAgentFilters(sbBuilder, tenantID, statuses)
	sbBuilder.OrderBy("id DESC")

	applyPagination(sbBuilder, limit, offset)

	query, args := sbBuilder.Build()
	r.logger.Debug().Ctx(ctx).
		Str("query", query).
		Interface("args", args).
		Int32("limit", limit).
		Int32("offset", offset).
		Msg("Executing ListAgents query")

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		r.logger.Error().Ctx(ctx).
			Err(err).
			Str("query", query).
			Msg("Failed to execute query")
		return nil, r.mapPgError(ctx, err)
	}
	defer rows.Close()

	agents := make([]entity.Agent, 0)
	rowCount := 0
	for rows.Next() {
		rowCount++
		ag, err := scanAgentRow(rows)
		if err != nil {
			r.logger.Error().Ctx(ctx).Err(err).Int("row_number", rowCount).Msg("Failed to scan agent row")
			return nil, r.mapPgError(ctx, err)
		}
		agents = append(agents, ag)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Error iterating rows")
		return nil, r.mapPgError(ctx, err)
	}

	r.logger.Debug().Ctx(ctx).Int("count", len(agents)).Msg("Successfully listed agents")
	return agents, nil
}

func (r *Repository) GetAgentCount(
	ctx context.Context,
	statuses []entity.DeploymentStatus,
	tenantID uuid.UUID,
) (int32, error) {
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()
	sbBuilder.
		Select("COUNT(*)").
		From("agent")

	// apply filters
	applyAgentFilters(sbBuilder, tenantID, statuses)

	query, args := sbBuilder.Build()

	var count int32
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		r.logger.Error().Ctx(ctx).
			Err(err).
			Str("query", query).
			Msg("Failed to get agent count")
		return 0, r.mapPgError(ctx, err)
	}

	return count, nil
}

func (r *Repository) UpdateDeploymentStatus(
	ctx context.Context,
	agentId string,
	deploymentStatus entity.DeploymentStatus,
	updatedAt time.Time,
) error {
	queries := r.getQueries(ctx)
	err := queries.UpdateAgentDeploymentStatus(ctx, gen.UpdateAgentDeploymentStatusParams{
		DeploymentStatus: deploymentStatus.String(),
		ID:               agentId,
		UpdatedAt:        pgtype.Timestamp{Time: updatedAt, Valid: true},
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map agent to entity")
		return r.mapPgError(ctx, err)
	}
	return nil
}

func (r *Repository) UpdateAgent(
	ctx context.Context, agent entity.Agent,
) error {
	queries := r.getQueries(ctx)

	postgresAgent, mapErr := r.mapToPostgresAgent(ctx, agent)
	if mapErr != nil {
		r.logger.Error().Ctx(ctx).Err(mapErr).Msg("Failed to map agent to postgres agent")
		return mapErr
	}

	err := queries.UpdateAgent(ctx, gen.UpdateAgentParams{
		ID:                     agent.ID,
		Name:                   agent.AgentName,
		CreateAgentRequestJson: postgresAgent.CreateAgentRequestJson,
		DeploymentStatus:       agent.DeploymentStatus.String(),
		UpdatedAt:              postgresAgent.UpdatedAt,
		TenantID:               agent.TenantID,
	})
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map agent to entity")
		return r.mapPgError(ctx, err)
	}
	return nil
}

func scanAgentRow(rows pgx.Rows) (entity.Agent, error) {
	var ag entity.Agent

	var statusStr string
	var createAgentRequestBytes []byte

	err := rows.Scan(
		&ag.ID,
		&ag.AgentName,
		&createAgentRequestBytes,
		&ag.CreatedBy,
		&ag.CreatedAt,
		&ag.UpdatedAt,
		&ag.TenantID,
		&statusStr,
	)
	if err != nil {
		return entity.Agent{}, err
	}

	req := &pb.CreateAgentRequest{}
	if err := protojson.Unmarshal(createAgentRequestBytes, req); err != nil {
		return entity.Agent{}, err
	}
	ag.CreateAgentRequest = req

	ag.DeploymentStatus, err = entity.ParseDeploymentStatus(statusStr)
	if err != nil {
		return entity.Agent{}, err
	}

	return ag, nil
}

func applyAgentFilters(builder *sb.SelectBuilder, tenantID uuid.UUID, statuses []entity.DeploymentStatus) {
	builder.Where(builder.Equal("tenant_id", tenantID))

	// add status filter if statuses are provided
	if len(statuses) > 0 {
		statusStrings := make([]any, len(statuses))
		for i, status := range statuses {
			statusStrings[i] = status.String()
		}

		builder.Where(builder.In("deployment_status", statusStrings...))
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
