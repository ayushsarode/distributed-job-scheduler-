package agent

import (
	"context"
	"errors"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"google.golang.org/protobuf/encoding/protojson"
)

func (r *Repository) mapToAgentEntity(_ context.Context, agent gen.Agent) (entity.Agent, error) {
	createAgentRequest := &pb.CreateAgentRequest{}

	err := protojson.Unmarshal(agent.CreateAgentRequestJson, createAgentRequest)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to unmarshal create agent request")
		return entity.Agent{}, err
	}

	deploymentStatus, err := entity.ParseDeploymentStatus(agent.DeploymentStatus)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to parse deployment status")
		return entity.Agent{}, err
	}
	agentEntity := entity.Agent{
		ID:                 agent.ID,
		AgentName:          agent.Name,
		CreateAgentRequest: createAgentRequest,
		CreatedAt:          agent.CreatedAt.Time,
		UpdatedAt:          agent.UpdatedAt.Time,
		CreatedBy:          agent.CreatedBy,
		TenantID:           agent.TenantID,
		DeploymentStatus:   deploymentStatus,
	}

	return agentEntity, nil
}

func (r *Repository) mapToPostgresAgent(_ context.Context, agent entity.Agent) (gen.Agent, error) {
	requestJson, err := protojson.Marshal(agent.CreateAgentRequest)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to marshal create agent request")
		return gen.Agent{}, err
	}

	agentRecord := gen.Agent{
		ID:                     agent.ID,
		Name:                   agent.AgentName,
		CreateAgentRequestJson: requestJson,
		CreatedAt:              pgtype.Timestamp{Time: agent.CreatedAt, Valid: true},
		UpdatedAt:              pgtype.Timestamp{Time: agent.UpdatedAt, Valid: true},
		CreatedBy:              agent.CreatedBy,
		TenantID:               agent.TenantID,
		DeploymentStatus:       agent.DeploymentStatus.String(),
	}

	return agentRecord, nil
}

const (
	pgUniqueViolation = "23505"
)

func (r *Repository) mapPgError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return xerrors.NotFoundError(ctx, errors.New("agent not found"))
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		if pgErr.Code == pgUniqueViolation {
			return xerrors.ConflictError(ctx, errors.New("agent already exists"))
		}
	}

	r.logger.Error().Err(err).Msg("Database error for agent")
	return xerrors.InternalError(ctx, errors.New("failed to process agent"))
}
