package agentservice

import (
	"context"
	"fmt"

	xerrors "exiro.ai/application/errors"
	agentutils "exiro.ai/application/service/agentservice/agentutils"
	servicetypes "exiro.ai/application/service/types"
)

type RoutingAgentService struct {
	lgService servicetypes.AgentService
	wsService servicetypes.AgentService
}

func NewRoutingAgentService(lgService, wsService servicetypes.AgentService) *RoutingAgentService {
	return &RoutingAgentService{
		lgService: lgService,
		wsService: wsService,
	}
}

// getService routes to the appropriate service based on agent ID suffix.
// _rt agents are handled directly by RealtimeAgentClient and should never reach this service.
func (s *RoutingAgentService) getService(ctx context.Context, agentID string) (servicetypes.AgentService, error) {
	if agentutils.IsLGAgent(agentID) {
		return s.lgService, nil
	}
	if agentutils.IsWSAgent(agentID) {
		return s.wsService, nil
	}
	return nil, xerrors.BadRequestError(ctx, fmt.Errorf("invalid agent ID suffix: %s (expected _lg or _ws; _rt agents bypass this service)", agentID))
}

func (s *RoutingAgentService) ValidateAgent(ctx context.Context, agentID string) error {
	service, err := s.getService(ctx, agentID)
	if err != nil {
		return err
	}
	return service.ValidateAgent(ctx, agentID)
}

func (s *RoutingAgentService) DisconnectAgent(ctx context.Context, agentID string, sessionID string) error {
	service, err := s.getService(ctx, agentID)
	if err != nil {
		return err
	}
	return service.DisconnectAgent(ctx, agentID, sessionID)
}

func (s *RoutingAgentService) Invoke(ctx context.Context, agentID string, sessionID string, messages []servicetypes.AgentMessage) (servicetypes.AgentResponse, error) {
	service, err := s.getService(ctx, agentID)
	if err != nil {
		return servicetypes.AgentResponse{}, err
	}
	return service.Invoke(ctx, agentID, sessionID, messages)
}

func (s *RoutingAgentService) UpdateContext(ctx context.Context, agentID, sessionID, contextText string) error {
	service, err := s.getService(ctx, agentID)
	if err != nil {
		return err
	}
	return service.UpdateContext(ctx, agentID, sessionID, contextText)
}
