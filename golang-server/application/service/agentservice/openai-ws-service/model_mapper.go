package openaiwsservice

import (
	"errors"
	"fmt"

	"exiro.ai/application/models/pb"
)

// mapAgentModelToOpenAI converts the protobuf AgentModel enum to an OpenAI model string.
func mapAgentModelToOpenAI(m pb.AgentModel) (string, error) {
	switch m {
	case pb.AgentModel_AGENTMODEL_UNSPECIFIED:
		return "",  errors.New("unsupported agent model for OpenAI WebSocket mode: AGENTMODEL_UNSPECIFIED")
	case pb.AgentModel_AGENTMODEL_GPT_REALTIME:
		return "", errors.New("unsupported agent model for OpenAI WebSocket mode: GPT_REALTIME is not applicable to WebSocket text mode")
	case pb.AgentModel_AGENTMODEL_GPT_4OMINI:
		return "gpt-4o-mini", nil
	default:
		return "", fmt.Errorf("unsupported agent model for OpenAI WebSocket mode: unknown model %v", m)
	}
}
