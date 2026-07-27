package api

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (g *grpcApi) UploadOutboundCallDocument(ctx context.Context, req *connect.Request[pb.UploadOutboundCallDocumentRequest]) (*connect.Response[pb.UploadOutboundCallDocumentResponse], error) {
	documentId, err := g.outboundCallDocumentService.UploadOutboundCallDocument(ctx, req.Msg.GetFileName(), req.Msg.GetFileType(), req.Msg.GetFileContent())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.UploadOutboundCallDocumentResponse{
		DocumentId: documentId,
	}), nil
}

func (g *grpcApi) ListOutboundCallDocuments(ctx context.Context, req *connect.Request[pb.ListOutboundCallDocumentsRequest]) (*connect.Response[pb.ListOutboundCallDocumentsResponse], error) {
	limit := req.Msg.GetLimit()
	offset := req.Msg.GetOffset()
	response, err := g.outboundCallDocumentService.ListOutboundCallDocuments(ctx, limit, offset)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListOutboundCallDocumentsResponse{}
	for _, doc := range response {
		resp.Documents = append(resp.Documents, &pb.OutboundCallDocument{
			Id:        doc.ID.String(),
			FileName:  doc.DocumentName,
			CreatedAt: timestamppb.New(doc.CreatedAt),
			UpdatedAt: timestamppb.New(doc.UpdatedAt),
		})
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) GetOutboundCallDocument(ctx context.Context, req *connect.Request[pb.GetOutboundCallDocumentRequest]) (*connect.Response[pb.GetOutboundCallDocumentResponse], error) {
	documentId := req.Msg.GetDocumentId()
	tenantID := auth.MustGetTenant(ctx)

	response, err := g.outboundCallDocumentService.GetOutboundCallDocument(ctx, documentId, tenantID)
	if err != nil {
		return nil, err
	}

	resp := &pb.GetOutboundCallDocumentResponse{
		Document: &pb.OutboundCallDocument{
			Id:        response.ID.String(),
			FileName:  response.DocumentName,
			CreatedAt: timestamppb.New(response.CreatedAt),
			UpdatedAt: timestamppb.New(response.UpdatedAt),
		},
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) DeleteOutboundCallDocument(ctx context.Context, req *connect.Request[pb.DeleteOutboundCallDocumentRequest]) (*connect.Response[pb.DeleteOutboundCallDocumentResponse], error) {
	documentId := req.Msg.GetDocumentId()
	tenantID := auth.MustGetTenant(ctx)

	response, err := g.outboundCallDocumentService.DeleteOutboundCallDocument(ctx, documentId, tenantID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&pb.DeleteOutboundCallDocumentResponse{
		Message: response,
	}), nil
}

func (g *grpcApi) CreateCallJob(ctx context.Context, req *connect.Request[pb.CreateCallJobRequest]) (*connect.Response[pb.CreateCallJobResponse], error) {
	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, err)
	}

	callJob := entity.CallJob{
		Name:                      req.Msg.GetName(),
		Workflow_id:               workflowId,
		Document_id:               req.Msg.GetDocumentId(),
		Document_type:             req.Msg.GetDocumentType(),
		Preffered_language:        req.Msg.GetPreferedLanguage(),
		Max_retries:               req.Msg.GetMaxRetries(),
		Retry_delay:               req.Msg.GetRetryDelay(),
		Outbound_call_provider_id: req.Msg.GetOutboundCallProviderId(),
	}
	callJob, err = g.outboundCallDocumentService.CreateCallJob(ctx, callJob)
	if err != nil {
		return nil, err
	}
	resp := &pb.CreateCallJobResponse{
		Id: callJob.ID.String(),
	}
	return connect.NewResponse(resp), nil
}

func (g *grpcApi) GetCallJob(ctx context.Context, req *connect.Request[pb.GetCallJobRequest]) (*connect.Response[pb.GetCallJobResponse], error) {
	calljobId, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("invalid job ID format"))
	}

	callJob, err := g.outboundCallDocumentService.GetCallJob(ctx, calljobId)
	if err != nil {
		return nil, err
	}

	resp := &pb.GetCallJobResponse{
		Id:                     callJob.ID.String(),
		Name:                   callJob.Name,
		WorkflowId:             callJob.Workflow_id.String(),
		DocumentId:             callJob.Document_id,
		DocumentType:           callJob.Document_type,
		PreferedLanguage:       callJob.Preffered_language,
		MaxRetries:             callJob.Max_retries,
		RetryDelay:             callJob.Retry_delay,
		OutboundCallProviderId: callJob.Outbound_call_provider_id,
		IsMaterialized:         callJob.Materialized,
		Status:                 mapToCallJobStatus(callJob.Status),
		CreatedAt:              timestamppb.New(callJob.CreatedAt),
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) ListCallJobs(ctx context.Context, req *connect.Request[pb.ListCallJobsRequest]) (*connect.Response[pb.ListCallJobsResponse], error) {
	statusFilter := g.parseCallJobStatusFilter(req.Msg.GetStatuses())

	workflowFilter, err := g.parseAgentFilter(ctx, req.Msg.GetWorkflowId())
	if err != nil {
		return nil, err
	}

	callStartedAt, callEndedAt := g.parseDateFilters(req.Msg.GetFilterDateBegin(), req.Msg.GetFilterDateEnd())

	limit, offset := g.normalizeListParams(req.Msg.GetLimit(), req.Msg.GetOffset())

	callJobs, totalCount, err := g.outboundCallDocumentService.ListCallJobs(ctx, statusFilter, workflowFilter, callStartedAt, callEndedAt, limit, offset)
	if err != nil {
		return nil, err
	}

	jobList := g.convertCallJobsToProto(callJobs)

	resp := &pb.ListCallJobsResponse{
		CallJobs:   jobList,
		TotalCount: totalCount,
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) parseCallJobStatusFilter(statuses []pb.CallJobStatus) []entity.CallJobStatus {
	statusFilter := make([]entity.CallJobStatus, 0, len(statuses))

	if len(statuses) >= 1 {
		for _, statusItem := range statuses {
			if statusItem != pb.CallJobStatus_CALL_JOB_STATUS_UNKNOWN {
				parsedStatus := mapToEntityCallJobStatus(statusItem)
				statusFilter = append(statusFilter, parsedStatus)
			}
		}
	}

	return statusFilter
}

func (g *grpcApi) parseAgentFilter(ctx context.Context, agentId string) (uuid.UUID, error) {
	agentFilter := uuid.Nil
	if agentId != "" {
		parsedAgentId, err := uuid.Parse(agentId)
		if err != nil {
			return uuid.Nil, xerrors.BadRequestError(ctx, err)
		}
		agentFilter = parsedAgentId
	}
	return agentFilter, nil
}

func (g *grpcApi) parseDateFilters(filterDateBegin, filterDateEnd *timestamppb.Timestamp) (time.Time, time.Time) {
	callStartedAt := time.Time{}
	if filterDateBegin != nil {
		callStartedAt = filterDateBegin.AsTime()
	}

	callEndedAt := time.Time{}
	if filterDateEnd != nil {
		callEndedAt = filterDateEnd.AsTime()
	}

	return callStartedAt, callEndedAt
}

func (g *grpcApi) convertCallJobsToProto(callJobs []entity.CallJob) []*pb.GetCallJobResponse {
	jobList := make([]*pb.GetCallJobResponse, 0, len(callJobs))
	for _, job := range callJobs {
		jobList = append(jobList, &pb.GetCallJobResponse{
			Id:                     job.ID.String(),
			Name:                   job.Name,
			WorkflowId:             job.Workflow_id.String(),
			DocumentId:             job.Document_id,
			DocumentType:           job.Document_type,
			PreferedLanguage:       job.Preffered_language,
			MaxRetries:             job.Max_retries,
			RetryDelay:             job.Retry_delay,
			OutboundCallProviderId: job.Outbound_call_provider_id,
			Status:                 mapToCallJobStatus(job.Status),
			IsMaterialized:         job.Materialized,
			CreatedAt:              timestamppb.New(job.CreatedAt),
		})
	}
	return jobList
}

func (g *grpcApi) DeleteCallJob(ctx context.Context, req *connect.Request[pb.DeleteCallJobRequest]) (*connect.Response[pb.DeleteCallJobResponse], error) {
	uuId, err := uuid.Parse(req.Msg.GetId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, err)
	}
	err = g.outboundCallDocumentService.DeleteCallJob(ctx, uuId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.DeleteCallJobResponse{Status: true}), nil
}

func (g *grpcApi) MaterializeJob(ctx context.Context, req *connect.Request[pb.MaterializeJobRequest]) (*connect.Response[emptypb.Empty], error) {
	job_id, err := uuid.Parse(req.Msg.GetJobId())
	if err != nil {
		return connect.NewResponse(&emptypb.Empty{}), xerrors.BadRequestError(ctx, err)
	}
	err = g.outboundCallDocumentService.MaterialiseJob(ctx, job_id)
	return connect.NewResponse(&emptypb.Empty{}), err
}

func (g *grpcApi) TriggerJob(ctx context.Context, req *connect.Request[pb.TriggerJobRequest]) (*connect.Response[emptypb.Empty], error) {
	job_id, err := uuid.Parse(req.Msg.GetJobId())
	if err != nil {
		return connect.NewResponse(&emptypb.Empty{}), xerrors.BadRequestError(ctx, err)
	}
	err = g.outboundCallDocumentService.TriggerJob(ctx, job_id)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) ListJobItems(ctx context.Context, req *connect.Request[pb.ListJobItemsRequest]) (*connect.Response[pb.ListJobItemsResponse], error) {
	jobId, err := uuid.Parse(req.Msg.GetJobId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, err)
	}

	limit, offset := g.normalizeListParams(req.Msg.GetLimit(), req.Msg.GetOffset())

	jobItems, totalCount, err := g.outboundCallDocumentService.ListJobItems(ctx, jobId, limit, offset)
	if err != nil {
		return nil, err
	}

	protoJobItems := g.convertJobItemsToProto(jobItems)

	resp := &pb.ListJobItemsResponse{
		JobItems:   protoJobItems,
		TotalCount: totalCount,
	}

	return connect.NewResponse(resp), nil
}

func (g *grpcApi) normalizeListParams(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func (g *grpcApi) convertJobItemsToProto(jobItems []entity.JobItem) []*pb.JobItem {
	protoJobItems := make([]*pb.JobItem, 0, len(jobItems))
	for _, item := range jobItems {
		protoJobItems = append(protoJobItems, g.convertJobItemToProto(item))
	}
	return protoJobItems
}

func (g *grpcApi) convertJobItemToProto(item entity.JobItem) *pb.JobItem {
	protoItem := &pb.JobItem{
		Id:           item.ID.String(),
		PhoneNumber:  item.PhoneNumber,
		AgentContext: item.AgentContext,
		JobData:      item.JobData,
		CallStatus:   mapToCallJobItemStatus(item.Status),
		UpdatedAt:    timestamppb.New(item.UpdatedAt),
	}

	g.setOptionalCallFields(protoItem, item)

	return protoItem
}

func (g *grpcApi) setOptionalCallFields(protoItem *pb.JobItem, item entity.JobItem) {
	if item.ExternalCallID != nil {
		protoItem.ExternalCallId = *item.ExternalCallID
	}

	if item.CallStartedAt != nil {
		protoItem.CallStartedAt = timestamppb.New(*item.CallStartedAt)
	} else if item.CallEndedAt != nil && item.CallDuration != nil && *item.CallDuration > 0 {
		// If we have end time and duration but no start time, estimate start time
		estimatedStartTime := item.CallEndedAt.Add(-time.Duration(*item.CallDuration) * time.Second)
		protoItem.CallStartedAt = timestamppb.New(estimatedStartTime)
	}

	if item.CallEndedAt != nil {
		protoItem.CallEndedAt = timestamppb.New(*item.CallEndedAt)
	}

	if item.CallDuration != nil {
		protoItem.CallDuration = *item.CallDuration
	}

	if item.CallErrorMessage != nil {
		protoItem.CallErrorMessage = *item.CallErrorMessage
	}
}

func (g *grpcApi) CopyCallJob(
	ctx context.Context,
	req *connect.Request[pb.CopyCallJobRequest],
) (*connect.Response[pb.CopyCallJobResponse], error) {
	providerId := uuid.Nil

	if req.Msg.GetOutboundCallProviderId() != "" {
		parsedProviderId, err := uuid.Parse(req.Msg.GetOutboundCallProviderId())
		if err != nil {
			return nil, xerrors.BadRequestError(ctx, err)
		}

		providerId = parsedProviderId
	}

	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, err)
	}

	// Call into your domain service
	newJobID, copiedCount, err := g.outboundCallDocumentService.CopyCallJob(
		ctx,
		req.Msg.GetSourceJobId(),
		req.Msg.GetName(),
		providerId,
		workflowId,
		req.Msg.GetPreferedLanguage(),
		req.Msg.GetJobItemIds(),
	)
	if err != nil {
		return nil, err
	}

	// Build response
	return connect.NewResponse(&pb.CopyCallJobResponse{
		NewJobId:    newJobID,
		ItemsCopied: copiedCount,
	}), nil
}

// UpdateCallJobDetails implements pbconnect.OutboundCallDocumentServiceHandler.
func (g *grpcApi) UpdateCallJobDetails(ctx context.Context, req *connect.Request[pb.UpdateCallJobDetailsRequest]) (*connect.Response[emptypb.Empty], error) {
	workflowId, err := uuid.Parse(req.Msg.GetWorkflowId())
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, err)
	}

	err = g.outboundCallDocumentService.UpdateCallJobDetails(ctx, req.Msg.GetJobId(), req.Msg.GetName(), workflowId)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// AddJobItems implements pbconnect.OutboundCallDocumentServiceHandler.
func (g *grpcApi) AddJobItems(ctx context.Context, req *connect.Request[pb.AddJobItemsRequest]) (*connect.Response[emptypb.Empty], error) {
	entityItems := make([]entity.NewJobItem, 0, len(req.Msg.GetItems()))
	for _, pbItem := range req.Msg.GetItems() {
		entityItem := entity.NewJobItem{
			PhoneNumber:  pbItem.GetPhoneNumber(),
			AgentContext: pbItem.GetAgentContext(),
		}
		entityItems = append(entityItems, entityItem)
	}

	err := g.outboundCallDocumentService.AddJobItems(ctx, req.Msg.GetJobId(), entityItems)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// RemoveJobItems implements pbconnect.OutboundCallDocumentServiceHandler.
func (g *grpcApi) RemoveJobItems(ctx context.Context, req *connect.Request[pb.RemoveJobItemsRequest]) (*connect.Response[emptypb.Empty], error) {
	err := g.outboundCallDocumentService.RemoveJobItems(ctx, req.Msg.GetJobId(), req.Msg.GetJobItemIds())
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
