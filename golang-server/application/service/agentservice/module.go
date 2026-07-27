package agentservice

import (
	"context"

	"exiro.ai/application/assert"
	openaiwsservice "exiro.ai/application/service/agentservice/openai-ws-service"
	pythonagentservice "exiro.ai/application/service/agentservice/python-agent-service"
	repositoryTypes "exiro.ai/application/service/internal/types"
	servicetypes "exiro.ai/application/service/types"
)

func NewAgentService(ctx context.Context, agentRepo repositoryTypes.AgentRepository) servicetypes.AgentService {
	assert.NotNil(agentRepo, "agentRepo is required")
	lgService, err := pythonagentservice.NewAgentService(ctx)
	assert.NoError(err, "failed to create LG agent service")
	wsService, err := openaiwsservice.NewAgentService(ctx, agentRepo)
	assert.NoError(err, "failed to create WS agent service")

	return NewRoutingAgentService(lgService, wsService)
}
