package deploymentstatsservice

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
)

type DeployAgentStatsService struct {
	AgentRepository repositoryTypes.AgentRepository
}

func NewDeployAgentStatsService(
	ctx context.Context,
	agentRepository repositoryTypes.AgentRepository,

) *DeployAgentStatsService {
	assert.NotNil(agentRepository)

	return &DeployAgentStatsService{
		AgentRepository: agentRepository,
	}
}

var _ types.DeploymentAgentStatsService = &DeployAgentStatsService{}

func (d *DeployAgentStatsService) GetAgent(ctx context.Context, agentId string) (entity.Agent, error) {
	tenantID := auth.MustGetTenant(ctx)

	agent, err := d.AgentRepository.GetAgent(ctx, agentId, tenantID)
	if err != nil {
		return entity.Agent{}, err
	}

	return agent, nil
}
