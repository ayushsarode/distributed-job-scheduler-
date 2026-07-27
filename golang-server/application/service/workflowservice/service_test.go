package workflowservice

//go:generate mockgen -destination=mocks/workflow_mocks.go -package=mocks exiro.ai/application/service/internal/types WorkflowRepository
//go:generate mockgen -destination=mocks/agent_mocks.go -package=mocks exiro.ai/application/service/types DeploymentAgentStatsService

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/application/service/workflowservice/mocks"
)

type MockTransactionHandler struct{}

func (m *MockTransactionHandler) WithTransaction(ctx context.Context, callback func(ctx context.Context) error) error {
	return callback(ctx)
}

func setupTest(t *testing.T) (
	context.Context,
	*WorkflowService,
	*mocks.MockWorkflowRepository,
	*mocks.MockDeploymentAgentStatsService,
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

	mockRepo := mocks.NewMockWorkflowRepository(ctrl)
	mockAgentStatsService := mocks.NewMockDeploymentAgentStatsService(ctrl)

	service := &WorkflowService{
		WorkflowRepository:          mockRepo,
		DeploymentAgentStatsService: mockAgentStatsService,
		transactionHandler:          &MockTransactionHandler{},
		logger:                      &logger,
	}

	return ctx, service, mockRepo, mockAgentStatsService, tenantID, userID
}

// ---------------------------------------------------------------------------
// CreateWorkflow
// ---------------------------------------------------------------------------

func TestCreateWorkflow_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, userID := setupTest(t)

	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"
	input := entity.Workflow{
		Name:        "Test Workflow",
		Description: "Test Description",
		Agent_id:    agentID,
	}

	mockRepo.EXPECT().
		CreateWorkflow(ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, wf entity.Workflow) error {
			assert.Equal(t, input.Name, wf.Name)
			assert.Equal(t, input.Description, wf.Description)
			assert.Equal(t, input.Agent_id, wf.Agent_id)
			assert.Equal(t, entity.WorkflowDraft, wf.Status)
			assert.Equal(t, tenantID, wf.TenantID)
			assert.Equal(t, userID, wf.CreatedBy)
			assert.NotEqual(t, uuid.Nil, wf.ID)
			assert.False(t, wf.CreatedAt.IsZero())
			assert.False(t, wf.UpdatedAt.IsZero())
			return nil
		}).
		Times(1)

	result, err := service.CreateWorkflow(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, input.Name, result.Name)
	assert.Equal(t, entity.WorkflowDraft, result.Status)
	assert.Equal(t, tenantID, result.TenantID)
	assert.Equal(t, userID, result.CreatedBy)
	assert.NotEqual(t, uuid.Nil, result.ID)
}

func TestCreateWorkflow_RepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, _, _ := setupTest(t)

	input := entity.Workflow{
		Name:     "Test Workflow",
		Agent_id: "01KKQJHPRV9SH3ZX42G8G84XHX_ws",
	}

	repoErr := xerrors.InternalError(ctx, errors.New("database connection failed"))

	mockRepo.EXPECT().
		CreateWorkflow(ctx, gomock.Any()).
		Return(repoErr).
		Times(1)

	_, err := service.CreateWorkflow(ctx, input)

	require.Error(t, err)
	assert.Equal(t, repoErr, err)
	assert.True(t, xerrors.IsInternalError(err))
}

// ---------------------------------------------------------------------------
// GetWorkflow
// ---------------------------------------------------------------------------

func TestGetWorkflow_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expected := entity.Workflow{
		ID:       workflowID,
		Name:     "Test Workflow",
		TenantID: tenantID,
		Status:   entity.WorkflowDraft,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(expected, nil).
		Times(1)

	result, err := service.GetWorkflow(ctx, workflowID)

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestGetWorkflow_NotFound(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.NotFoundError(ctx, errors.New("not found"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(entity.Workflow{}, expectedErr).
		Times(1)

	_, err := service.GetWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// ---------------------------------------------------------------------------
// UpdateWorkflow
// ---------------------------------------------------------------------------

func TestUpdateWorkflow_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"
	newName := "Updated Workflow Name"
	newDescription := "Updated Description"

	existing := entity.Workflow{
		ID:          workflowID,
		Name:        "Old Name",
		Description: "Old Description",
		TenantID:    tenantID,
		Status:      entity.WorkflowDraft,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateWorkflow(ctx, gomock.Any(), tenantID).
		DoAndReturn(func(_ context.Context, wf entity.Workflow, _ uuid.UUID) error {
			assert.Equal(t, newName, wf.Name)
			assert.Equal(t, newDescription, wf.Description)
			assert.Equal(t, agentID, wf.Agent_id)
			assert.False(t, wf.UpdatedAt.IsZero())
			return nil
		}).
		Times(1)

	err := service.UpdateWorkflow(ctx, workflowID, newName, newDescription, agentID)
	require.NoError(t, err)
}

func TestUpdateWorkflow_Success_PublishedStatus(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"

	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	err := service.UpdateWorkflow(ctx, workflowID, "name", "desc", agentID)

	require.Error(t, err)
	assert.True(t, xerrors.IsPreconditionFailedError(err))
}

func TestUpdateWorkflow_WorkflowAlreadyInUse(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)
	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}
	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)
	err := service.UpdateWorkflow(ctx, workflowID, "name", "desc", "01KKQJHPRV9SH3ZX42G8G84XHX_ws")
	require.Error(t, err)
	assert.True(t, xerrors.IsPreconditionFailedError(err))
}

func TestUpdateWorkflow_WorkflowNotFound(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)
	workflowID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.NotFoundError(ctx, errors.New("not found"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(entity.Workflow{}, expectedErr).
		Times(1)

	err := service.UpdateWorkflow(ctx, workflowID, "name", "desc", "01KKQJHPRV9SH3ZX42G8G84XHX_ws")

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestUpdateWorkflow_RepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"
	existing := entity.Workflow{ID: workflowID, TenantID: tenantID, Status: entity.WorkflowDraft}
	expectedErr := xerrors.InternalError(ctx, errors.New("update failed"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateWorkflow(ctx, gomock.Any(), tenantID).
		Return(expectedErr).
		Times(1)

	err := service.UpdateWorkflow(ctx, workflowID, "New Name", "New Desc", agentID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// ---------------------------------------------------------------------------
// PublishWorkflow
// ---------------------------------------------------------------------------

func TestPublishWorkflow_Success(t *testing.T) {
	ctx, service, mockRepo, mockAgentStats, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"

	existing := entity.Workflow{
		ID:       workflowID,
		Name:     "Test Workflow",
		TenantID: tenantID,
		Status:   entity.WorkflowDraft,
		Agent_id: agentID,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	// Service calls DeploymentAgentStatsService.GetAgent with the agent ID as a string
	mockAgentStats.EXPECT().
		GetAgent(ctx, agentID).
		Return(entity.Agent{
			ID:               agentID,
			DeploymentStatus: entity.DeploymentStatusSuccess,
		}, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateWorkflow(ctx, gomock.Any(), tenantID).
		DoAndReturn(func(_ context.Context, wf entity.Workflow, _ uuid.UUID) error {
			assert.Equal(t, entity.WorkflowPublished, wf.Status)
			assert.False(t, wf.UpdatedAt.IsZero())
			return nil
		}).
		Times(1)

	err := service.PublishWorkflow(ctx, workflowID)
	require.NoError(t, err)
}

func TestPublishWorkflow_AlreadyPublished_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished, // Already published
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	// Should NOT call GetAgent or UpdateWorkflow when already published

	err := service.PublishWorkflow(ctx, workflowID)
	require.NoError(t, err)
}

func TestPublishWorkflow_WorkflowNotFound(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.NotFoundError(ctx, errors.New("not found"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(entity.Workflow{}, expectedErr).
		Times(1)

	err := service.PublishWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestPublishWorkflow_AgentNotFound(t *testing.T) {
	ctx, service, mockRepo, mockAgentStats, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"

	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowDraft,
		Agent_id: agentID,
	}

	expectedErr := xerrors.NotFoundError(ctx, errors.New("agent not found"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockAgentStats.EXPECT().
		GetAgent(ctx, agentID).
		Return(entity.Agent{}, expectedErr).
		Times(1)

	err := service.PublishWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestPublishWorkflow_AgentNotDeployed(t *testing.T) {
	ctx, service, mockRepo, mockAgentStats, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"

	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowDraft,
		Agent_id: agentID,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockAgentStats.EXPECT().
		GetAgent(ctx, agentID).
		Return(entity.Agent{
			ID:               agentID,
			DeploymentStatus: entity.DeploymentStatusFailed, // not successful
		}, nil).
		Times(1)

	err := service.PublishWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.True(t, xerrors.IsPreconditionFailedError(err))
}

func TestPublishWorkflow_UpdateRepositoryError(t *testing.T) {
	ctx, service, mockRepo, mockAgentStats, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"

	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowDraft,
		Agent_id: agentID,
	}

	expectedErr := xerrors.InternalError(ctx, errors.New("publish failed"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockAgentStats.EXPECT().
		GetAgent(ctx, agentID).
		Return(entity.Agent{
			ID:               agentID,
			DeploymentStatus: entity.DeploymentStatusSuccess,
		}, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateWorkflow(ctx, gomock.Any(), tenantID).
		Return(expectedErr).
		Times(1)

	err := service.PublishWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// ---------------------------------------------------------------------------
// DeactivateWorkflow
// ---------------------------------------------------------------------------

func TestDeactivateWorkflow_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(
			ctx,
			workflowID,
			activeCallJobStatuses,
			tenantID,
		).
		Return(int32(0), nil). // ← MUST return zero active jobs
		Times(1)

	mockRepo.EXPECT().
		UpdateWorkflow(ctx, gomock.Any(), tenantID).
		DoAndReturn(func(_ context.Context, wf entity.Workflow, _ uuid.UUID) error {
			assert.Equal(t, entity.WorkflowInActive, wf.Status)
			return nil
		}).
		Times(1)

	err := service.DeactivateWorkflow(ctx, workflowID)
	require.NoError(t, err)
}

func TestDeactivateWorkflow_WorkflowNotFound(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.NotFoundError(ctx, errors.New("not found"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(entity.Workflow{}, expectedErr).
		Times(1)

	err := service.DeactivateWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestDeactivateWorkflow_HasActiveJobs_ReturnsError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(
			ctx,
			workflowID,
			activeCallJobStatuses,
			tenantID,
		).
		Return(int32(3), nil). // Has active jobs
		Times(1)

	err := service.DeactivateWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.True(t, xerrors.IsPreconditionFailedError(err))
}

func TestDeactivateWorkflow_GetActiveCountError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}
	expectedErr := xerrors.InternalError(ctx, errors.New("db error"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(
			ctx,
			workflowID,
			activeCallJobStatuses,
			tenantID,
		).
		Return(int32(0), expectedErr).
		Times(1)

	err := service.DeactivateWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestDeactivateWorkflow_UpdateRepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}
	expectedErr := xerrors.InternalError(ctx, errors.New("update failed"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(
			ctx,
			workflowID,
			activeCallJobStatuses,
			tenantID,
		).
		Return(int32(0), nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateWorkflow(ctx, gomock.Any(), tenantID).
		Return(expectedErr).
		Times(1)

	err := service.DeactivateWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// ---------------------------------------------------------------------------
// DeleteWorkflow
// ---------------------------------------------------------------------------

func TestDeleteWorkflow_DraftStatus_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowDraft,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	// Even draft workflows need to check for active jobs (edge case: jobs inserted directly)
	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(int32(0), nil).
		Times(1)

	mockRepo.EXPECT().
		DeleteWorkflow(ctx, workflowID, tenantID).
		Return(nil).
		Times(1)

	err := service.DeleteWorkflow(ctx, workflowID)
	require.NoError(t, err)
}

func TestDeleteWorkflow_InactiveStatus_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowInActive,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	// Inactive workflows can still have active jobs from when they were published
	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(int32(0), nil).
		Times(1)

	mockRepo.EXPECT().
		DeleteWorkflow(ctx, workflowID, tenantID).
		Return(nil).
		Times(1)

	err := service.DeleteWorkflow(ctx, workflowID)
	require.NoError(t, err)
}

func TestDeleteWorkflow_PublishedStatus_NoActiveJobs_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(int32(0), nil).
		Times(1)

	mockRepo.EXPECT().
		DeleteWorkflow(ctx, workflowID, tenantID).
		Return(nil).
		Times(1)

	err := service.DeleteWorkflow(ctx, workflowID)
	require.NoError(t, err)
}

func TestDeleteWorkflow_PublishedStatus_ActiveJobs_ReturnsError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowPublished,
	}

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(int32(5), nil).
		Times(1)

	err := service.DeleteWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.True(t, xerrors.IsPreconditionFailedError(err))
}

func TestDeleteWorkflow_WorkflowNotFound(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.NotFoundError(ctx, errors.New("not found"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(entity.Workflow{}, expectedErr).
		Times(1)

	err := service.DeleteWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestDeleteWorkflow_RepositoryDeleteError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	existing := entity.Workflow{
		ID:       workflowID,
		TenantID: tenantID,
		Status:   entity.WorkflowDraft,
	}
	expectedErr := xerrors.InternalError(ctx, errors.New("delete failed"))

	mockRepo.EXPECT().
		GetWorkflow(ctx, workflowID, tenantID).
		Return(existing, nil).
		Times(1)

	// Always check for active jobs first
	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(int32(0), nil).
		Times(1)

	mockRepo.EXPECT().
		DeleteWorkflow(ctx, workflowID, tenantID).
		Return(expectedErr).
		Times(1)

	err := service.DeleteWorkflow(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// ---------------------------------------------------------------------------
// ListWorkflows
// ---------------------------------------------------------------------------

func TestListWorkflows_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	expected := []entity.Workflow{
		{ID: uuid.Must(uuid.NewV7()), Name: "Workflow 1", TenantID: tenantID, Status: entity.WorkflowDraft},
		{ID: uuid.Must(uuid.NewV7()), Name: "Workflow 2", TenantID: tenantID, Status: entity.WorkflowPublished},
	}

	mockRepo.EXPECT().
		ListWorkflows(ctx, []entity.WorkflowStatus(nil), int32(10), int32(0), tenantID).
		Return(expected, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCount(ctx, []entity.WorkflowStatus(nil), tenantID).
		Return(int32(2), nil).
		Times(1)

	result, totalCount, err := service.ListWorkflows(ctx, nil, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int32(2), totalCount)
	assert.ElementsMatch(t, expected, result)
}

func TestListWorkflows_FilterByStatus(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	draft := entity.Workflow{ID: uuid.Must(uuid.NewV7()), Name: "Workflow 1", Status: entity.WorkflowDraft, TenantID: tenantID}
	filteredStatuses := []entity.WorkflowStatus{entity.WorkflowDraft}

	mockRepo.EXPECT().
		ListWorkflows(ctx, filteredStatuses, int32(10), int32(0), tenantID).
		Return([]entity.Workflow{draft}, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCount(ctx, filteredStatuses, tenantID).
		Return(int32(1), nil).
		Times(1)

	result, totalCount, err := service.ListWorkflows(ctx, filteredStatuses, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int32(1), totalCount)
	assert.Len(t, result, 1)
	assert.Equal(t, draft.ID, result[0].ID)
	assert.Equal(t, entity.WorkflowDraft, result[0].Status)
}

func TestListWorkflows_MultipleStatusFilter(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	wf1 := entity.Workflow{ID: uuid.Must(uuid.NewV7()), Name: "Workflow 1", Status: entity.WorkflowDraft, TenantID: tenantID}
	wf2 := entity.Workflow{ID: uuid.Must(uuid.NewV7()), Name: "Workflow 2", Status: entity.WorkflowPublished, TenantID: tenantID}
	statuses := []entity.WorkflowStatus{entity.WorkflowDraft, entity.WorkflowPublished}

	mockRepo.EXPECT().
		ListWorkflows(ctx, statuses, int32(10), int32(0), tenantID).
		Return([]entity.Workflow{wf1, wf2}, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCount(ctx, statuses, tenantID).
		Return(int32(2), nil).
		Times(1)

	result, totalCount, err := service.ListWorkflows(ctx, statuses, 10, 0)

	require.NoError(t, err)
	assert.Equal(t, int32(2), totalCount)

	statusMap := make(map[entity.WorkflowStatus]bool)
	for _, wf := range result {
		statusMap[wf.Status] = true
	}
	assert.True(t, statusMap[entity.WorkflowDraft], "Should contain DRAFT workflow")
	assert.True(t, statusMap[entity.WorkflowPublished], "Should contain PUBLISHED workflow")
}

func TestListWorkflows_RepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	expectedErr := xerrors.InternalError(ctx, errors.New("db error"))

	mockRepo.EXPECT().
		ListWorkflows(ctx, []entity.WorkflowStatus(nil), int32(10), int32(0), tenantID).
		Return(nil, expectedErr).
		Times(1)

	_, _, err := service.ListWorkflows(ctx, nil, 10, 0)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestListWorkflows_CountRepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	expectedErr := xerrors.InternalError(ctx, errors.New("count db error"))

	mockRepo.EXPECT().
		ListWorkflows(ctx, []entity.WorkflowStatus(nil), int32(10), int32(0), tenantID).
		Return([]entity.Workflow{}, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCount(ctx, []entity.WorkflowStatus(nil), tenantID).
		Return(int32(0), expectedErr).
		Times(1)

	_, _, err := service.ListWorkflows(ctx, nil, 10, 0)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

// ---------------------------------------------------------------------------
// GetWorkflowCallJobCount
// ---------------------------------------------------------------------------

func TestGetWorkflowCallJobCount_Success(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expectedActiveCount := int32(42)
	expectedTotalCount := int32(69)

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(expectedActiveCount, nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCount(ctx, workflowID, tenantID).
		Return(expectedTotalCount, nil).
		Times(1)

	count, err := service.GetWorkflowWithCallJobCount(ctx, workflowID)

	require.NoError(t, err)
	assert.Equal(t, expectedActiveCount, count.ActiveCallJobCount)
	assert.Equal(t, expectedTotalCount, count.TotalCallJobCount)
}

func TestGetWorkflowCallJobCount_ActiveCountRepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.InternalError(ctx, errors.New("database error"))

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(int32(0), expectedErr).
		Times(1)

	count, err := service.GetWorkflowWithCallJobCount(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, int32(0), count.ActiveCallJobCount)
	assert.Equal(t, int32(0), count.TotalCallJobCount)
	assert.Equal(t, expectedErr, err)
}

func TestGetWorkflowCallJobCount_TotalCountRepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, tenantID, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.InternalError(ctx, errors.New("database error"))

	mockRepo.EXPECT().
		GetWorkflowCallJobCountByStatuses(ctx, workflowID, activeCallJobStatuses, tenantID).
		Return(int32(5), nil).
		Times(1)

	mockRepo.EXPECT().
		GetWorkflowCallJobCount(ctx, workflowID, tenantID).
		Return(int32(0), expectedErr).
		Times(1)

	count, err := service.GetWorkflowWithCallJobCount(ctx, workflowID)

	require.Error(t, err)
	assert.Equal(t, int32(0), count.ActiveCallJobCount)
	assert.Equal(t, int32(0), count.TotalCallJobCount)
	assert.Equal(t, expectedErr, err)
}
