package workflowstatsservice

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"github.com/rs/zerolog"
)

type WorkflowStatsService struct {
	WorkflowRepository repositoryTypes.WorkflowRepository
	AgentRepository    repositoryTypes.AgentRepository
	logger             *zerolog.Logger
}

func NewService(
	ctx context.Context,
	workflowRepository repositoryTypes.WorkflowRepository,
	agentRepository repositoryTypes.AgentRepository,
) *WorkflowStatsService {
	assert.NotNil(workflowRepository)
	assert.NotNil(agentRepository)

	return &WorkflowStatsService{
		WorkflowRepository: workflowRepository,
		AgentRepository:    agentRepository,
		logger:             zerolog.Ctx(ctx),
	}
}

var _ types.WorkflowStatsService = &WorkflowStatsService{}

func (s *WorkflowStatsService) HasPublishedWorkflowsForAgent(ctx context.Context, agentId string) (bool, error) {
	tenantID := auth.MustGetTenant(ctx)

	hasPublishedWorkflow, err := s.WorkflowRepository.HasPublishedWorkflowsForAgent(ctx, agentId, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to list worflows by agent")
		return false, err
	}

	return hasPublishedWorkflow, nil
}
