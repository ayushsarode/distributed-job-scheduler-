package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CreateAgent implements pbconnect.DeployServiceHandler.
func (g *grpcApi) CreateAgent(
	ctx context.Context,
	req *connect.Request[pb.CreateAgentRequest],
) (*connect.Response[pb.CreateAgentResponse], error) {
	if req.Msg.GetAgentTemplateRequest() == nil && req.Msg.GetAgentPromptRequest() == nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("create_agent_request is required"))
	}

	agent, err := g.deploymentService.CreateAgent(ctx, req.Msg)
	if err != nil {
		return nil, err
	}

	response := &pb.CreateAgentResponse{
		AgentId: agent.ID,
	}
	return connect.NewResponse(response), nil
}

// GetAgent implements pbconnect.DeployServiceHandler.
func (g *grpcApi) GetAgent(
	ctx context.Context,
	req *connect.Request[pb.GetAgentRequest],
) (*connect.Response[pb.GetAgentResponse], error) {
	if req.Msg.GetAgentId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("agent_id is required"))
	}

	agent, err := g.deploymentService.GetAgent(ctx, req.Msg.GetAgentId())
	if err != nil {
		return nil, err
	}

	response := &pb.GetAgentResponse{
		AgentId:          agent.ID,
		AgentName:        agent.AgentName,
		AgentInfo:        agent.CreateAgentRequest,
		DeploymentStatus: mapToDeploymentStatus(agent.DeploymentStatus),
		CreatedAt:        timestamppb.New(agent.CreatedAt),
		UpdatedAt:        timestamppb.New(agent.UpdatedAt),
		CreatedBy:        agent.CreatedBy,
	}

	return connect.NewResponse(response), nil
}

// ListAgents implements pbconnect.DeployServiceHandler.
func (g *grpcApi) ListAgents(
	ctx context.Context,
	req *connect.Request[pb.ListAgentRequest],
) (*connect.Response[pb.ListAgentResponse], error) {
	statusFilter := g.parseAgentStatusFilter(req.Msg.GetStatuses())

	agents, totalCount, err := g.deploymentService.ListAgents(ctx, statusFilter, req.Msg.GetLimit(), req.Msg.GetOffset())
	if err != nil {
		return nil, err
	}

	response := &pb.ListAgentResponse{
		Agents:     make([]*pb.ListAgentResponseItem, len(agents)),
		TotalCount: totalCount,
	}
	for i, agent := range agents {
		response.Agents[i] = &pb.ListAgentResponseItem{
			AgentId:          agent.ID,
			AgentName:        agent.AgentName,
			DeploymentStatus: mapToDeploymentStatus(agent.DeploymentStatus),
			CreatedAt:        timestamppb.New(agent.CreatedAt),
			UpdatedAt:        timestamppb.New(agent.UpdatedAt),
			CreatedBy:        agent.CreatedBy,
		}
	}

	return connect.NewResponse(response), nil
}

// DeployAgent implements pbconnect.DeployServiceHandler.
func (g *grpcApi) DeployAgent(
	ctx context.Context,
	req *connect.Request[pb.DeployAgentRequest],
) (*connect.Response[emptypb.Empty], error) {
	if req.Msg.GetAgentId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("agent_id is required"))
	}

	err := g.deploymentService.DeployAgent(ctx, req.Msg.GetAgentId())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) UpdateAgent(
	ctx context.Context,
	req *connect.Request[pb.UpdateAgentRequest],
) (*connect.Response[emptypb.Empty], error) {
	if req.Msg.GetAgentId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("agent_id is required"))
	}

	err := g.deploymentService.UpdateAgent(ctx, req.Msg.GetAgentId(), req.Msg.GetAgentName(), req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) parseAgentStatusFilter(statuses []pb.DeploymentStatus) []entity.DeploymentStatus {
	statusFilter := make([]entity.DeploymentStatus, 0, len(statuses))

	if len(statuses) >= 1 {
		for _, statusItem := range statuses {
			if statusItem != pb.DeploymentStatus_DEPLOYMENT_STATUS_UNKNOWN {
				parsedStatus := mapToEntityDeploymentStatus(statusItem)
				statusFilter = append(statusFilter, parsedStatus)
			}
		}
	}

	return statusFilter
}
