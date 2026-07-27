package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (g *grpcApi) CreateWorkflow(ctx context.Context, req *connect.Request[pb.CreateWorkflowRequest]) (*connect.Response[pb.CreateWorkflowResponse], error) {
	// Validate name is not empty
	if req.Msg.GetName() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("name is required"))
	}

	// Validate agent_id is not empty
	if req.Msg.GetAgentId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("agent_id is required"))
	}

	workflow := entity.Workflow{
		Name:        req.Msg.GetName(),
		Agent_id:    req.Msg.GetAgentId(),
		Description: req.Msg.GetDescription(),
	}
	result, err := g.workflowService.CreateWorkflow(ctx, workflow)
	if err != nil {
		return nil, err
	}
	resp := &pb.CreateWorkflowResponse{
		WorkflowId: result.ID.String(),
	}
	return connect.NewResponse(resp), nil
}

func (g *grpcApi) GetWorkflow(ctx context.Context, req *connect.Request[pb.GetWorkflowRequest]) (*connect.Response[pb.GetWorkflowResponse], error) {
	if req.Msg.GetWorkflowId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("workflow_id is required"))
	}

	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("invalid workflow ID format"))
	}

	workflow, err := g.workflowService.GetWorkflow(ctx, workflowId)
	if err != nil {
		return nil, err
	}

	callJobCount, err := g.workflowService.GetWorkflowWithCallJobCount(ctx, workflowId)
	if err != nil {
		return nil, err
	}

	resp := convertToWorkflowProto(workflow)
	resp.ActiveCampaignsCount = callJobCount.ActiveCallJobCount
	resp.TotalCalls = callJobCount.TotalCallJobCount

	return connect.NewResponse(&resp), nil
}

func (g *grpcApi) ListWorkflows(ctx context.Context, req *connect.Request[pb.ListWorkflowsRequest]) (*connect.Response[pb.ListWorkflowsResponse], error) {
	statusFilter := g.parseWorkflowStatusFilter(ctx, req.Msg.GetStatuses())

	workflows, totalCount, err := g.workflowService.ListWorkflows(ctx, statusFilter, req.Msg.GetLimit(), req.Msg.GetOffset())
	if err != nil {
		return nil, err
	}

	workflowList := g.convertToListWorkflowProto(workflows)

	resp := &pb.ListWorkflowsResponse{
		Workflows:  workflowList,
		TotalCount: totalCount,
	}
	return connect.NewResponse(resp), nil
}

func (g *grpcApi) UpdateWorkflow(ctx context.Context, req *connect.Request[pb.UpdateWorkflowRequest]) (*connect.Response[emptypb.Empty], error) {
	if req.Msg.GetWorkflowId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("workflow_id is required"))
	}

	if req.Msg.GetAgentId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("agent_id is required"))
	}

	if req.Msg.GetName() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("name is required"))
	}

	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("invalid workflow ID format"))
	}

	err = g.workflowService.UpdateWorkflow(ctx, workflowId, req.Msg.GetName(), req.Msg.GetDescription(), req.Msg.GetAgentId())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) PublishWorkflow(ctx context.Context, req *connect.Request[pb.PublishWorkflowRequest]) (*connect.Response[emptypb.Empty], error) {
	if req.Msg.GetWorkflowId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("workflow_id is required"))
	}

	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("invalid workflow ID format"))
	}

	err = g.workflowService.PublishWorkflow(ctx, workflowId)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) DeactivateWorkflow(ctx context.Context, req *connect.Request[pb.DeactivateWorkflowRequest]) (*connect.Response[emptypb.Empty], error) {
	if req.Msg.GetWorkflowId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("workflow_id is required"))
	}

	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("invalid workflow ID format"))
	}

	err = g.workflowService.DeactivateWorkflow(ctx, workflowId)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) DeleteWorkflow(ctx context.Context, req *connect.Request[pb.DeleteWorkflowRequest]) (*connect.Response[emptypb.Empty], error) {
	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, err)
	}

	err = g.workflowService.DeleteWorkflow(ctx, workflowId)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) parseWorkflowStatusFilter(_ context.Context, statuses []pb.WorkflowStatus) []entity.WorkflowStatus {
	if len(statuses) == 0 {
		return []entity.WorkflowStatus{}
	}

	statusFilter := make([]entity.WorkflowStatus, 0, len(statuses))

	for _, statusItem := range statuses {
		statusFilter = append(statusFilter, mapToEntityWorkflowStatus(statusItem))
	}

	return statusFilter
}

func convertToWorkflowProto(workflow entity.Workflow) pb.GetWorkflowResponse {
	return pb.GetWorkflowResponse{
		Id:          workflow.ID.String(),
		Name:        workflow.Name,
		Description: workflow.Description,
		AgentId:     workflow.Agent_id,
		Status:      mapToWorkflowStatus(workflow.Status),
		CreatedBy:   workflow.CreatedBy,
		CreatedAt:   timestamppb.New(workflow.CreatedAt),
		UpdatedAt:   timestamppb.New(workflow.UpdatedAt),
	}
}

func (g *grpcApi) convertToListWorkflowProto(workflow []entity.Workflow) []*pb.ListWorkFlowResponseItem {
	workflowList := make([]*pb.ListWorkFlowResponseItem, 0, len(workflow))
	for _, workflow := range workflow {
		protoWorkflow := pb.ListWorkFlowResponseItem{
			Id:          workflow.ID.String(),
			Name:        workflow.Name,
			Description: workflow.Description,
			AgentId:     workflow.Agent_id,
			Status:      mapToWorkflowStatus(workflow.Status),
			CreatedBy:   workflow.CreatedBy,
			CreatedAt:   timestamppb.New(workflow.CreatedAt),
			UpdatedAt:   timestamppb.New(workflow.UpdatedAt),
		}

		workflowList = append(workflowList, &protoWorkflow)
	}
	return workflowList
}
