package api

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"exiro.ai/application/assert"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/models/pb/pbconnect"
	"exiro.ai/application/service/types"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type grpcApi struct {
	logger                      zerolog.Logger
	knowledgeBaseService        types.KnowledgeBaseService
	callHandlerService          types.CallHandlerService
	deploymentService           types.DeploymentService
	usermanagementservice       types.UserManagementService
	outboundCallDocumentService types.OutboundCallService
	transcriptionstorageService types.TranscriptionStorageService
	credentialService           types.CredentialService
	workflowService             types.WorkflowService
	auditService                types.AuditService
}

func (g *grpcApi) RegisterRoutes(mux *chi.Mux) {
	// path, handler := pbconnect.NewCollectionsHandler(g)
	interceptors := connect.WithInterceptors(xerrors.NewErrorInterceptor())

	path, handler := pbconnect.NewKnowledgeBaseHandler(g, interceptors)
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewDeployServiceHandler(g, interceptors) // TODO: Rename to DeploymentServiceHandler
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewCallServiceHandler(g, interceptors)
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewUserManagementServiceHandler(g, interceptors)
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewOutboundCallDocumentServiceHandler(g, interceptors)
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewTranscriptionRetrievalServiceHandler(g, interceptors)
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewCredentialServiceHandler(g, interceptors)
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewWorkflowServiceHandler(g, interceptors)
	mux.Handle(path+"*", handler)

	path, handler = pbconnect.NewAuditServiceHandler(g, interceptors)
	mux.Handle(path+"*", handler)
}

// TODO: move to different package.
// CreateSession implements pbconnect.CallServiceHandler.
func (g *grpcApi) CreateSession(
	ctx context.Context,
	req *connect.Request[pb.CreateSessionRequest],
) (*connect.Response[pb.CreateSessionResponse], error) {
	// TODO: add protovalider
	if req.Msg.GetAgentId() == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("agent_id is required"))
	}
	sessionId, err := g.callHandlerService.CreateSession(ctx, req.Msg.GetAgentId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.CreateSessionResponse{
		SessionId: sessionId,
	}), nil
}

// ListDocuments implements pbconnect.KnowledgeBaseHandler.
func (g *grpcApi) ListDocuments(
	ctx context.Context,
	req *connect.Request[pb.ListDocumentRequest],
) (*connect.Response[pb.ListDocumentResponse], error) {
	docs, err := g.knowledgeBaseService.ListDocuments(ctx, req.Msg.GetLimit(), req.Msg.GetOffset())
	if err != nil {
		return nil, err
	}
	resp := &pb.ListDocumentResponse{}
	for _, doc := range docs {
		resp.Documents = append(resp.Documents, &pb.Document{
			Id:           doc.ID.String(),
			DocumentName: doc.DocumentName,
			IsActive:     doc.IsActive,
			CreatedAt:    timestamppb.New(doc.CreatedAt),
			UpdatedAt:    timestamppb.New(doc.UpdatedAt),
		})
	}
	return connect.NewResponse(resp), nil
}

func (g *grpcApi) ChangeDocumentStatus(
	ctx context.Context,
	req *connect.Request[pb.ChangeDocumentStatusRequest],
) (*connect.Response[emptypb.Empty], error) {
	err := g.knowledgeBaseService.ChangeDocumentStatus(ctx, req.Msg.GetDocumentId(), req.Msg.GetIsActive())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func (g *grpcApi) DeleteDocument(ctx context.Context, req *connect.Request[pb.DeleteDocumentRequest]) (*connect.Response[emptypb.Empty], error) {
	err := g.knowledgeBaseService.DeleteDocument(ctx, req.Msg.GetDocumentId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&emptypb.Empty{}), nil
}

func newGrpcApi(
	ctx context.Context,
	callHandlerService types.CallHandlerService,
	knowledgeBaseService types.KnowledgeBaseService,
	deploymentService types.DeploymentService,
	usermanagementservice types.UserManagementService,
	outboundCallDocumentService types.OutboundCallService,
	transcriptionStorageService types.TranscriptionStorageService,
	credentialService types.CredentialService,
	workflowService types.WorkflowService,
	auditService types.AuditService,
) grpcApi {
	assert.NotNil(knowledgeBaseService)
	assert.NotNil(callHandlerService)
	assert.NotNil(deploymentService)
	assert.NotNil(outboundCallDocumentService)
	assert.NotNil(transcriptionStorageService)
	assert.NotNil(credentialService)
	assert.NotNil(workflowService)
	assert.NotNil(auditService)
	return grpcApi{
		logger:                      *zerolog.Ctx(ctx),
		knowledgeBaseService:        knowledgeBaseService,
		callHandlerService:          callHandlerService,
		deploymentService:           deploymentService,
		usermanagementservice:       usermanagementservice,
		outboundCallDocumentService: outboundCallDocumentService,
		transcriptionstorageService: transcriptionStorageService,
		credentialService:           credentialService,
		workflowService:             workflowService,
		auditService:                auditService,
	}
}
