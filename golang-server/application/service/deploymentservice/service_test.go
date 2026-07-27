package deploymentservice

//go:generate mockgen -destination=mocks/agent_repository_mocks.go -package=mocks exiro.ai/application/service/internal/types AgentRepository
//go:generate mockgen -destination=mocks/message_queue_mocks.go -package=mocks exiro.ai/application/service/internal/types MessageQueue
//go:generate mockgen -destination=mocks/message_consumer_mocks.go -package=mocks exiro.ai/application/service/internal/types MessageConsumer
//go:generate mockgen -destination=mocks/transaction_handler_mocks.go -package=mocks exiro.ai/application/service/internal/types TransationHandler
//go:generate mockgen -destination=mocks/workflow_stats_mocks.go -package=mocks exiro.ai/application/service/types WorkflowStatsService
//go:generate mockgen -destination=mocks/audit_service_mocks.go -package=mocks exiro.ai/application/service/types AuditService

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/deploymentservice/mocks"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
)

type MockTransactionHandler struct{}

func (m *MockTransactionHandler) WithTransaction(ctx context.Context, callback func(ctx context.Context) error) error {
	return callback(ctx)
}

type MockAuditService struct{}

func (m *MockAuditService) Log(ctx context.Context, event types.AuditEvent) error {
	return nil
}

func (m *MockAuditService) ListAuditLogs(ctx context.Context, query entity.AuditLogQuery) ([]entity.AuditLog, error) {
	return []entity.AuditLog{}, nil
}

func setupTest(t *testing.T) (
	context.Context,
	*DeployService,
	*mocks.MockAgentRepository,
	*mocks.MockMessageQueue,
	*mocks.MockWorkflowStatsService,
	uuid.UUID,
	string,
) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	logger := zerolog.Nop()
	ctx := logger.WithContext(context.Background())

	tenantID := uuid.Must(uuid.NewV7())
	userID := "test-user-123"
	ctx = auth.SetTenant(ctx, tenantID)
	ctx = auth.SetUser(ctx, userID)

	mockRepo := mocks.NewMockAgentRepository(ctrl)
	mockMQ := mocks.NewMockMessageQueue(ctrl)
	mockWorkflowStats := mocks.NewMockWorkflowStatsService(ctrl)
	mockAuditService := mocks.NewMockAuditService(ctrl)

	// Allow any audit log calls by default — tests that care can override
	mockAuditService.EXPECT().
		Log(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	service := &DeployService{
		AgentRepository:      mockRepo,
		MessageQueue:         mockMQ,
		logger:               &logger,
		transactionHandler:   &MockTransactionHandler{},
		WorkflowStatsService: mockWorkflowStats,
		auditService:         mockAuditService,
	}

	return ctx, service, mockRepo, mockMQ, mockWorkflowStats, tenantID, userID
}

// CreateAgent.
func TestCreateAgent_Success_TemplateRequest(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, userID := setupTest(t)

	request := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
			AgentTemplateRequest: &pb.AgentTemplateRequest{
				AgentName:  "Test Template Agent",
				TemplateId: "template-123",
				Params:     map[string]string{"key": "value"},
			},
		},
	}

	mockRepo.EXPECT().
		InsertAgent(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, agent entity.Agent) (entity.Agent, error) {
			assert.Equal(t, "Test Template Agent", agent.AgentName)
			assert.Equal(t, entity.DeploymentStatusNotDeployed, agent.DeploymentStatus)
			assert.Equal(t, tenantID, agent.TenantID)
			assert.Equal(t, userID, agent.CreatedBy)
			assert.NotEqual(t, uuid.Nil, agent.ID)
			assert.False(t, agent.CreatedAt.IsZero())
			assert.False(t, agent.UpdatedAt.IsZero())
			assert.True(t, proto.Equal(request, agent.CreateAgentRequest))
			return agent, nil
		}).
		Times(1)

	result, err := service.CreateAgent(ctx, request)

	require.NoError(t, err)
	assert.Equal(t, "Test Template Agent", result.AgentName)
	assert.Equal(t, entity.DeploymentStatusNotDeployed, result.DeploymentStatus)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, userID, result.CreatedBy)
	assert.NotEqual(t, uuid.Nil, result.ID)
}

func TestCreateAgent_Success_PromptRequest(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, userID := setupTest(t)

	request := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "Test Prompt Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
			},
		},
	}

	mockRepo.EXPECT().
		InsertAgent(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, agent entity.Agent) (entity.Agent, error) {
			assert.Equal(t, "Test Prompt Agent", agent.AgentName)
			assert.Equal(t, entity.DeploymentStatusNotDeployed, agent.DeploymentStatus)
			assert.Equal(t, tenantID, agent.TenantID)
			assert.Equal(t, userID, agent.CreatedBy)
			assert.True(t, proto.Equal(request, agent.CreateAgentRequest))
			return agent, nil
		}).
		Times(1)

	result, err := service.CreateAgent(ctx, request)

	require.NoError(t, err)
	assert.Equal(t, "Test Prompt Agent", result.AgentName)
	assert.Equal(t, entity.DeploymentStatusNotDeployed, result.DeploymentStatus)
}

func TestCreateAgent_InvalidRequestType(t *testing.T) {
	ctx, service, _, _, _, _, _ := setupTest(t)

	// Create a request with nil inner request (invalid)
	request := &pb.CreateAgentRequest{
		CreateAgentRequest: nil,
	}

	_, err := service.CreateAgent(ctx, request)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid agent request type")
}

func TestCreateAgent_UnknownDeploymentType(t *testing.T) {
	ctx, service, _, _, _, _, _ := setupTest(t)

	request := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_UNKNOWN,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
	}

	_, err := service.CreateAgent(ctx, request)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "deployment_type must be explicitly specified")
}

func TestCreateAgent_RTDeploymentWithIncompatibleModel(t *testing.T) {
	ctx, service, _, _, _, _, _ := setupTest(t)

	request := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "Test RT Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI, // Not GPT_REALTIME
			},
		},
	}

	_, err := service.CreateAgent(ctx, request)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "RT deployment type requires GPT_REALTIME model")
}

func TestCreateAgent_RepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, _ := setupTest(t)

	request := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
			AgentTemplateRequest: &pb.AgentTemplateRequest{
				AgentName:  "Test Agent",
				TemplateId: "template-123",
			},
		},
	}

	expectedErr := xerrors.InternalError(ctx, errors.New("database connection failed"))

	mockRepo.EXPECT().
		InsertAgent(ctx, gomock.Any()).
		Return(entity.Agent{}, expectedErr).
		Times(1)

	_, err := service.CreateAgent(ctx, request)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.True(t, xerrors.IsInternalError(err))
}

// GetAgent.
func TestGetAgent_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	expected := entity.Agent{
		ID:               agentID,
		AgentName:        "Test Agent",
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusSuccess,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(expected, nil).
		Times(1)

	result, err := service.GetAgent(ctx, agentID)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestGetAgent_NotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	expectedErr := xerrors.NotFoundError(ctx, errors.New("agent not found"))

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(entity.Agent{}, expectedErr).
		Times(1)

	_, err := service.GetAgent(ctx, agentID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// ListAgents.
func TestListAgents_Success_NoFilter(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	expected := []entity.Agent{
		{ID: ulid.Make().String() + "_lg", AgentName: "Agent 1", TenantID: tenantID, DeploymentStatus: entity.DeploymentStatusNotDeployed},
		{ID: ulid.Make().String() + "_ws", AgentName: "Agent 2", TenantID: tenantID, DeploymentStatus: entity.DeploymentStatusSuccess},
	}

	mockRepo.EXPECT().
		ListAgents(ctx, tenantID, []entity.DeploymentStatus(nil), int32(10), int32(0)).
		Return(expected, nil).
		Times(1)

	mockRepo.EXPECT().
		GetAgentCount(ctx, []entity.DeploymentStatus(nil), tenantID).
		Return(int32(2), nil).
		Times(1)

	result, totalCount, err := service.ListAgents(ctx, nil, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int32(2), totalCount)
	assert.ElementsMatch(t, expected, result)
}

func TestListAgents_Success_WithStatusFilter(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	statuses := []entity.DeploymentStatus{entity.DeploymentStatusSuccess}
	expected := []entity.Agent{
		{ID: ulid.Make().String() + "_rt", AgentName: "Deployed Agent", TenantID: tenantID, DeploymentStatus: entity.DeploymentStatusSuccess},
	}

	mockRepo.EXPECT().
		ListAgents(ctx, tenantID, statuses, int32(10), int32(0)).
		Return(expected, nil).
		Times(1)

	mockRepo.EXPECT().
		GetAgentCount(ctx, statuses, tenantID).
		Return(int32(1), nil).
		Times(1)

	result, totalCount, err := service.ListAgents(ctx, statuses, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int32(1), totalCount)
	assert.Len(t, result, 1)
	assert.Equal(t, entity.DeploymentStatusSuccess, result[0].DeploymentStatus)
}

func TestListAgents_RepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	expectedErr := xerrors.InternalError(ctx, errors.New("database error"))

	mockRepo.EXPECT().
		ListAgents(ctx, tenantID, []entity.DeploymentStatus(nil), int32(10), int32(0)).
		Return(nil, expectedErr).
		Times(1)

	_, _, err := service.ListAgents(ctx, nil, 10, 0)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestListAgents_CountRepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	expectedErr := xerrors.InternalError(ctx, errors.New("count database error"))

	mockRepo.EXPECT().
		ListAgents(ctx, tenantID, []entity.DeploymentStatus(nil), int32(10), int32(0)).
		Return([]entity.Agent{}, nil).
		Times(1)

	mockRepo.EXPECT().
		GetAgentCount(ctx, []entity.DeploymentStatus(nil), tenantID).
		Return(int32(0), expectedErr).
		Times(1)

	_, _, err := service.ListAgents(ctx, nil, 10, 0)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// DeployAgent.
func TestDeployAgent_Success_RealtimeAgent(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_rt"
	agent := entity.Agent{
		ID:               agentID,
		AgentName:        "Realtime Agent",
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusNotDeployed,
		CreateAgentRequest: &pb.CreateAgentRequest{
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "Realtime Agent",
					Prompt:    "Test prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(agent, nil).
		Times(1)

	// Realtime agents deploy immediately to Success status
	mockRepo.EXPECT().
		UpdateDeploymentStatus(ctx, agentID, entity.DeploymentStatusSuccess, gomock.Any()).
		Return(nil).
		Times(1)

	err := service.DeployAgent(ctx, agentID)

	require.NoError(t, err)
}

func TestDeployAgent_Success_WSAgent(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_ws"
	agent := entity.Agent{
		ID:               agentID,
		AgentName:        "WS Agent",
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusNotDeployed,
		CreateAgentRequest: &pb.CreateAgentRequest{
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "WS Agent",
					Prompt:    "Test prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
				},
			},
		},
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(agent, nil).
		Times(1)

	// WS agents deploy immediately to Success status
	mockRepo.EXPECT().
		UpdateDeploymentStatus(ctx, agentID, entity.DeploymentStatusSuccess, gomock.Any()).
		Return(nil).
		Times(1)

	err := service.DeployAgent(ctx, agentID)

	require.NoError(t, err)
}

func TestDeployAgent_Success_TemplateAgent(t *testing.T) {
	ctx, service, mockRepo, mockMQ, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	agent := entity.Agent{
		ID:               agentID,
		AgentName:        "Template Agent",
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusNotDeployed,
		CreateAgentRequest: &pb.CreateAgentRequest{
			CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
				AgentTemplateRequest: &pb.AgentTemplateRequest{
					AgentName:  "Template Agent",
					TemplateId: "template-123",
					Params:     map[string]string{"key": "value"},
				},
			},
		},
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(agent, nil).
		Times(1)

	// Template agents go to InProgress and publish message
	mockRepo.EXPECT().
		UpdateDeploymentStatus(ctx, agentID, entity.DeploymentStatusInProgress, gomock.Any()).
		Return(nil).
		Times(1)

	mockMQ.EXPECT().
		PublishStringMessage(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, msg string) error {
			assert.Contains(t, msg, agentID)
			assert.Contains(t, msg, "template-123")
			return nil
		}).
		Times(1)

	err := service.DeployAgent(ctx, agentID)

	require.NoError(t, err)
}

func TestDeployAgent_Success_PromptAgentNonRealtime(t *testing.T) {
	ctx, service, mockRepo, mockMQ, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	agent := entity.Agent{
		ID:               agentID,
		AgentName:        "Prompt Agent",
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusNotDeployed,
		CreateAgentRequest: &pb.CreateAgentRequest{
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "Prompt Agent",
					Prompt:    "Test prompt",
					Model:     pb.AgentModel_AGENTMODEL_UNSPECIFIED, // Not realtime
				},
			},
		},
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(agent, nil).
		Times(1)

	// LG agents go to InProgress and publish message
	mockRepo.EXPECT().
		UpdateDeploymentStatus(ctx, agentID, entity.DeploymentStatusInProgress, gomock.Any()).
		Return(nil).
		Times(1)

	mockMQ.EXPECT().
		PublishStringMessage(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, msg string) error {
			assert.Contains(t, msg, agentID)
			return nil
		}).
		Times(1)

	err := service.DeployAgent(ctx, agentID)

	require.NoError(t, err)
}

func TestDeployAgent_AgentNotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	expectedErr := xerrors.NotFoundError(ctx, errors.New("agent not found"))

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(entity.Agent{}, expectedErr).
		Times(1)

	err := service.DeployAgent(ctx, agentID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestDeployAgent_UpdateStatusError(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_rt"
	agent := entity.Agent{
		ID:               agentID,
		AgentName:        "Realtime Agent",
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusNotDeployed,
		CreateAgentRequest: &pb.CreateAgentRequest{
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					Model: pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	}

	expectedErr := xerrors.InternalError(ctx, errors.New("update failed"))

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(agent, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateDeploymentStatus(ctx, agentID, entity.DeploymentStatusSuccess, gomock.Any()).
		Return(expectedErr).
		Times(1)

	err := service.DeployAgent(ctx, agentID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestDeployAgent_MessageQueueError(t *testing.T) {
	ctx, service, mockRepo, mockMQ, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	agent := entity.Agent{
		ID:               agentID,
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusNotDeployed,
		CreateAgentRequest: &pb.CreateAgentRequest{
			CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
				AgentTemplateRequest: &pb.AgentTemplateRequest{
					AgentName:  "Template Agent",
					TemplateId: "template-123",
				},
			},
		},
	}

	expectedErr := errors.New("message queue publish failed")

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(agent, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateDeploymentStatus(ctx, agentID, entity.DeploymentStatusInProgress, gomock.Any()).
		Return(nil).
		Times(1)

	mockMQ.EXPECT().
		PublishStringMessage(ctx, gomock.Any()).
		Return(expectedErr).
		Times(1)

	err := service.DeployAgent(ctx, agentID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// UpdateAgent.
func TestUpdateAgent_Success_ConfigChanged(t *testing.T) {
	ctx, service, mockRepo, _, mockWorkflowStats, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	oldRequest := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
			AgentTemplateRequest: &pb.AgentTemplateRequest{
				AgentName:  "Old Name",
				TemplateId: "template-123",
			},
		},
	}

	newRequest := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
			AgentTemplateRequest: &pb.AgentTemplateRequest{
				AgentName:  "New Name",
				TemplateId: "template-456", // Different config
			},
		},
	}

	existing := entity.Agent{
		ID:                 agentID,
		AgentName:          "Old Name",
		TenantID:           tenantID,
		DeploymentStatus:   entity.DeploymentStatusSuccess,
		CreateAgentRequest: oldRequest,
	}

	updateRequest := &pb.UpdateAgentRequest{
		AgentId:            agentID,
		AgentName:          "New Name",
		CreateAgentRequest: newRequest,
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(existing, nil).
		Times(1)

	mockWorkflowStats.EXPECT().
		HasPublishedWorkflowsForAgent(ctx, agentID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateAgent(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, agent entity.Agent) error {
			assert.Equal(t, "New Name", agent.AgentName)
			assert.Equal(t, entity.DeploymentStatusNotDeployed, agent.DeploymentStatus) // Reset due to config change
			assert.False(t, agent.UpdatedAt.IsZero())
			return nil
		}).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "New Name", updateRequest)

	require.NoError(t, err)
}

func TestUpdateAgent_Success_ConfigUnchanged(t *testing.T) {
	ctx, service, mockRepo, _, mockWorkflowStats, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	sameRequest := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
			AgentTemplateRequest: &pb.AgentTemplateRequest{
				AgentName:  "Agent Name",
				TemplateId: "template-123",
			},
		},
	}

	existing := entity.Agent{
		ID:                 agentID,
		AgentName:          "Old Agent Name",
		TenantID:           tenantID,
		DeploymentStatus:   entity.DeploymentStatusSuccess,
		CreateAgentRequest: sameRequest,
	}

	updateRequest := &pb.UpdateAgentRequest{
		AgentId:            agentID,
		AgentName:          "New Agent Name",
		CreateAgentRequest: sameRequest, // Same config
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(existing, nil).
		Times(1)

	mockWorkflowStats.EXPECT().
		HasPublishedWorkflowsForAgent(ctx, agentID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateAgent(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, agent entity.Agent) error {
			assert.Equal(t, "New Agent Name", agent.AgentName)
			assert.Equal(t, entity.DeploymentStatusSuccess, agent.DeploymentStatus) // Status unchanged
			return nil
		}).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "New Agent Name", updateRequest)

	require.NoError(t, err)
}

func TestUpdateAgent_AgentNotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	expectedErr := xerrors.NotFoundError(ctx, errors.New("agent not found"))

	updateRequest := &pb.UpdateAgentRequest{
		AgentId: agentID,
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(entity.Agent{}, expectedErr).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "Name", updateRequest)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestUpdateAgent_HasPublishedWorkflows(t *testing.T) {
	ctx, service, mockRepo, _, mockWorkflowStats, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	existing := entity.Agent{
		ID:               agentID,
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusSuccess,
	}

	updateRequest := &pb.UpdateAgentRequest{
		AgentId: agentID,
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(existing, nil).
		Times(1)

	mockWorkflowStats.EXPECT().
		HasPublishedWorkflowsForAgent(ctx, agentID).
		Return(true, nil).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "Name", updateRequest)

	require.Error(t, err)
	assert.True(t, xerrors.IsPreconditionFailedError(err))
	assert.Contains(t, err.Error(), "cannot update agent while being used in published workflows")
}

func TestUpdateAgent_DeploymentInProgress(t *testing.T) {
	ctx, service, mockRepo, _, mockWorkflowStats, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	existing := entity.Agent{
		ID:               agentID,
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusInProgress,
	}

	updateRequest := &pb.UpdateAgentRequest{
		AgentId: agentID,
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(existing, nil).
		Times(1)

	mockWorkflowStats.EXPECT().
		HasPublishedWorkflowsForAgent(ctx, agentID).
		Return(false, nil).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "Name", updateRequest)

	require.Error(t, err)
	assert.True(t, xerrors.IsPreconditionFailedError(err))
	assert.Contains(t, err.Error(), "cannot update agent while deployment is in progress")
}

func TestUpdateAgent_WorkflowStatsError(t *testing.T) {
	ctx, service, mockRepo, _, mockWorkflowStats, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	existing := entity.Agent{
		ID:               agentID,
		TenantID:         tenantID,
		DeploymentStatus: entity.DeploymentStatusSuccess,
	}

	expectedErr := xerrors.InternalError(ctx, errors.New("workflow stats error"))

	updateRequest := &pb.UpdateAgentRequest{
		AgentId: agentID,
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(existing, nil).
		Times(1)

	mockWorkflowStats.EXPECT().
		HasPublishedWorkflowsForAgent(ctx, agentID).
		Return(false, expectedErr).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "Name", updateRequest)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestUpdateAgent_RepositoryUpdateError(t *testing.T) {
	ctx, service, mockRepo, _, mockWorkflowStats, tenantID, _ := setupTest(t)

	agentID := ulid.Make().String() + "_lg"
	request := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentTemplateRequest{
			AgentTemplateRequest: &pb.AgentTemplateRequest{
				AgentName:  "Agent Name",
				TemplateId: "template-123",
			},
		},
	}

	existing := entity.Agent{
		ID:                 agentID,
		TenantID:           tenantID,
		DeploymentStatus:   entity.DeploymentStatusSuccess,
		CreateAgentRequest: request,
	}

	expectedErr := xerrors.InternalError(ctx, errors.New("update failed"))

	updateRequest := &pb.UpdateAgentRequest{
		AgentId:            agentID,
		CreateAgentRequest: request,
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(existing, nil).
		Times(1)

	mockWorkflowStats.EXPECT().
		HasPublishedWorkflowsForAgent(ctx, agentID).
		Return(false, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateAgent(ctx, gomock.Any()).
		Return(expectedErr).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "Name", updateRequest)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestUpdateAgent_IncompatibleSuffixChange(t *testing.T) {
	ctx, service, mockRepo, _, mockWorkflowStats, tenantID, _ := setupTest(t)

	// Existing agent has _lg suffix
	agentID := ulid.Make().String() + "_lg"
	oldRequest := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "LG Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
	}

	existing := entity.Agent{
		ID:                 agentID,
		AgentName:          "LG Agent",
		TenantID:           tenantID,
		DeploymentStatus:   entity.DeploymentStatusSuccess,
		CreateAgentRequest: oldRequest,
	}

	// Try to update to WS deployment type (would change suffix from _lg to _ws)
	newRequest := &pb.CreateAgentRequest{
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "WS Agent",
				Prompt:    "You are a different assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
	}

	updateRequest := &pb.UpdateAgentRequest{
		AgentId:            agentID,
		AgentName:          "WS Agent",
		CreateAgentRequest: newRequest,
	}

	mockRepo.EXPECT().
		GetAgent(ctx, agentID, tenantID).
		Return(existing, nil).
		Times(1)

	mockWorkflowStats.EXPECT().
		HasPublishedWorkflowsForAgent(ctx, agentID).
		Return(false, nil).
		Times(1)

	err := service.UpdateAgent(ctx, agentID, "WS Agent", updateRequest)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "cannot change deployment type or model in a way that changes agent suffix")
}
