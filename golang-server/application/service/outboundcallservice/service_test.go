package outboundcallservice

//go:generate mockgen -destination=mocks/outbound_call_mocks.go -package=mocks exiro.ai/application/service/internal/types CallJobRepository
//go:generate mockgen -destination=mocks/audit_mocks.go -package=mocks exiro.ai/application/service/types AuditService
//go:generate mockgen -destination=mocks/workflow_service_mocks.go -package=mocks exiro.ai/application/service/types WorkflowService

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/outboundcallservice/mocks"
	servicetypes "exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
)

type MockTransactionHandler struct{}

func (m *MockTransactionHandler) WithTransaction(ctx context.Context, callback func(ctx context.Context) error) error {
	return callback(ctx)
}

// MockFIFOMessageQueue for testing.
type MockFIFOMessageQueue struct {
	ctrl     *gomock.Controller
	recorder *MockFIFOMessageQueueMockRecorder
}

type MockFIFOMessageQueueMockRecorder struct {
	mock *MockFIFOMessageQueue
}

func NewMockFIFOMessageQueue(ctrl *gomock.Controller) *MockFIFOMessageQueue {
	mock := &MockFIFOMessageQueue{ctrl: ctrl}
	mock.recorder = &MockFIFOMessageQueueMockRecorder{mock}
	return mock
}

func (m *MockFIFOMessageQueue) EXPECT() *MockFIFOMessageQueueMockRecorder {
	return m.recorder
}

func (m *MockFIFOMessageQueue) PublishStringMessage(ctx context.Context, message string, messageGroupID string, deduplicationID string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PublishStringMessage", ctx, message, messageGroupID, deduplicationID)
	ret0, _ := ret[0].(error)
	return ret0
}

func (mr *MockFIFOMessageQueueMockRecorder) PublishStringMessage(
	ctx, message, messageGroupID, deduplicationID any,
) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(
		mr.mock,
		"PublishStringMessage",
		reflect.TypeFor[func(context.Context, any, any, any) error](),
		ctx,
		message,
		messageGroupID,
		deduplicationID,
	)
}

func setupTest(t *testing.T) (
	context.Context,
	*Service,
	*mocks.MockCallJobRepository,
	*mocks.MockWorkflowService,
	*MockFIFOMessageQueue,
	*mocks.MockAuditService,
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

	mockRepo := mocks.NewMockCallJobRepository(ctrl)
	mockWorkflowService := mocks.NewMockWorkflowService(ctrl)
	mockQueue := NewMockFIFOMessageQueue(ctrl)
	mockAudit := mocks.NewMockAuditService(ctrl)

	service := &Service{
		CallJobRepository:  mockRepo,
		workflowService:    mockWorkflowService,
		logger:             &logger,
		transactionHandler: &MockTransactionHandler{},
		transcriptionQueue: mockQueue,
		auditService:       mockAudit,
		// Add mock queues to avoid nil pointer panics
		materializeJobQueue: dbos.WorkflowQueue{},
		callJobQueue:        dbos.WorkflowQueue{},
	}

	return ctx, service, mockRepo, mockWorkflowService, mockQueue, mockAudit, tenantID, userID
}

func TestCreateCallJob_Success(t *testing.T) {
	ctx, service, mockRepo, mockWorkflowService, _, mockAudit, tenantID, userID := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"
	callJob := entity.CallJob{
		Name:          "Test Call Job",
		Workflow_id:   workflowID,
		Document_id:   "test-document-123",
		Document_type: "csv",
	}

	mockWorkflowService.EXPECT().
		GetWorkflow(ctx, workflowID).
		Return(entity.Workflow{
			ID:       workflowID,
			Agent_id: agentID,
		}, nil).
		Times(1)

	mockRepo.EXPECT().
		CreateCallJob(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, job entity.CallJob) error {
			assert.Equal(t, workflowID, job.Workflow_id)
			assert.Equal(t, tenantID, job.TenantID)
			assert.Equal(t, userID, job.User)
			assert.Equal(t, entity.CallJobPending, job.Status)
			return nil
		}).
		Times(1)

	mockAudit.EXPECT().
		Log(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, event servicetypes.AuditEvent) error {
			assert.Equal(t, servicetypes.AuditActionCallJobCreated, event.Action)
			assert.Equal(t, servicetypes.AuditResourceTypeCallJob, event.ResourceType)
			return nil
		}).
		Return(nil).
		Times(1)

	result, err := service.CreateCallJob(ctx, callJob)
	require.NoError(t, err)
	assert.NotEmpty(t, result.ID)
}

func TestCreateCallJob_EmptyName_ReturnsBadRequestError(t *testing.T) {
	ctx, service, _, _, _, _, _, _ := setupTest(t)

	workflowId := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{
		Name:          "", // Empty name
		Workflow_id:   workflowId,
		Document_id:   "test-document-123",
		Document_type: "csv",
	}

	callJobResult, err := service.CreateCallJob(ctx, callJob)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "name is required")
	assert.Empty(t, callJobResult.ID)
}

func TestCreateCallJob_NilWorkflowId_ReturnsBadRequestError(t *testing.T) {
	ctx, service, _, _, _, _, _, _ := setupTest(t)

	callJob := entity.CallJob{
		Name:          "Test Call Job",
		Workflow_id:   uuid.Nil, // Nil workflow_id
		Document_id:   "test-document-123",
		Document_type: "csv",
	}

	callJobResult, err := service.CreateCallJob(ctx, callJob)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "workflow_id is required")
	assert.Empty(t, callJobResult.ID)
}

func TestCreateCallJob_EmptyDocumentId_ReturnsBadRequestError(t *testing.T) {
	ctx, service, _, _, _, _, _, _ := setupTest(t)

	workflowId := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{
		Name:          "Test Call Job",
		Workflow_id:   workflowId,
		Document_id:   "", // Empty document_id
		Document_type: "csv",
	}

	callJobResult, err := service.CreateCallJob(ctx, callJob)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "document_id is required")
	assert.Empty(t, callJobResult.ID)
}

func TestCreateCallJob_EmptyDocumentType_ReturnsBadRequestError(t *testing.T) {
	ctx, service, _, _, _, _, _, _ := setupTest(t)

	workflowId := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{
		Name:          "Test Call Job",
		Workflow_id:   workflowId,
		Document_id:   "test-document-123",
		Document_type: "", // Empty document_type
	}

	callJobResult, err := service.CreateCallJob(ctx, callJob)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "document_type is required")
	assert.Empty(t, callJobResult.ID)
}

func TestCreateCallJob_RepositoryReturnsInternalError(t *testing.T) {
	ctx, service, mockRepo, mockWorkflowService, _, _, _, _ := setupTest(t)

	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"
	callJob := entity.CallJob{
		Name:          "Test Call Job",
		Workflow_id:   workflowID,
		Document_id:   "test-document-123",
		Document_type: "csv",
	}

	mockWorkflowService.EXPECT().
		GetWorkflow(ctx, workflowID).
		Return(entity.Workflow{
			ID:       workflowID,
			Agent_id: agentID,
		}, nil).
		Times(1)

	repoError := xerrors.InternalError(ctx, errors.New("database connection failed"))

	mockRepo.EXPECT().
		CreateCallJob(ctx, gomock.Any()).
		Return(repoError).
		Times(1)

	callJobResult, err := service.CreateCallJob(ctx, callJob)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Empty(t, callJobResult.ID)
}

func TestGetCallJob_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	callJobId := uuid.Must(uuid.NewV7())
	expectedJob := entity.CallJob{
		ID:       callJobId,
		Name:     "Test Job",
		TenantID: tenantID,
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, callJobId, tenantID).
		Return(expectedJob, nil).
		Times(1)

	result, err := service.GetCallJob(ctx, callJobId)

	require.NoError(t, err)
	assert.Equal(t, expectedJob, result)
}

func TestGetCallJob_NotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.NotFoundError(ctx, errors.New("not found"))

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(entity.CallJob{}, expectedErr).
		Times(1)

	_, err := service.GetCallJob(ctx, jobID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestListCallJobs_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	expectedJobs := []entity.CallJob{
		{ID: uuid.Must(uuid.NewV7()), Name: "Job 1", TenantID: tenantID},
		{ID: uuid.Must(uuid.NewV7()), Name: "Job 2", TenantID: tenantID},
	}

	limit := int32(50)
	offset := int32(0)

	mockRepo.EXPECT().
		ListCallJobs(
			ctx,
			nil,
			uuid.Nil,
			time.Time{},
			time.Time{},
			limit,
			offset,
			tenantID,
		).
		Return(expectedJobs, nil).
		Times(1)

	mockRepo.EXPECT().
		GetCallJobCount(
			ctx,
			nil,
			uuid.Nil,
			time.Time{},
			time.Time{},
			tenantID,
		).
		Return(int32(2), nil).
		Times(1)

	result, totalCount, err := service.ListCallJobs(
		ctx,
		nil,
		uuid.Nil,
		time.Time{},
		time.Time{},
		limit,
		offset,
	)

	require.NoError(t, err)
	assert.Equal(t, expectedJobs, result)
	assert.Equal(t, int32(len(expectedJobs)), totalCount)
}

func TestListCallJobs_RepoError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	expectedErr := xerrors.InternalError(ctx, errors.New("db error"))

	mockRepo.EXPECT().
		ListCallJobs(
			ctx,
			nil,
			uuid.Nil,
			time.Time{},
			time.Time{},
			int32(50),
			int32(0),
			tenantID,
		).
		Return(nil, expectedErr).
		Times(1)

	_, _, err := service.ListCallJobs(
		ctx,
		nil,
		uuid.Nil,
		time.Time{},
		time.Time{},
		50,
		0,
	)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestListCallJobs_NotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	notFoundErr := xerrors.NotFoundError(ctx, errors.New("not found"))

	mockRepo.EXPECT().
		ListCallJobs(
			ctx,
			nil,
			uuid.Nil,
			time.Time{},
			time.Time{},
			int32(50),
			int32(0),
			tenantID,
		).
		Return(nil, notFoundErr).
		Times(1)

	_, _, err := service.ListCallJobs(
		ctx,
		nil,
		uuid.Nil,
		time.Time{},
		time.Time{},
		50,
		0,
	)

	require.Error(t, err)
	assert.True(t, xerrors.IsNotFoundError(err))
}

func TestDeleteCallJob_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, mockAudit, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())

	mockRepo.EXPECT().
		DeleteCallJob(ctx, jobID, tenantID).
		Return(nil).
		Times(1)

	mockAudit.EXPECT().
		Log(ctx, servicetypes.AuditEvent{
			Action:       servicetypes.AuditActionCallJobDeleted,
			ResourceType: servicetypes.AuditResourceTypeCallJob,
			ResourceID:   jobID.String(),
		}).
		Return(nil).
		Times(1)

	err := service.DeleteCallJob(ctx, jobID)

	require.NoError(t, err)
}

func TestDeleteCallJob_RepositoryError(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	expectedErr := xerrors.InternalError(ctx, errors.New("delete failed"))

	mockRepo.EXPECT().
		DeleteCallJob(ctx, jobID, tenantID).
		Return(expectedErr).
		Times(1)

	err := service.DeleteCallJob(ctx, jobID)

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

//nolint:funlen
func TestCopyCallJob_Success(t *testing.T) {
	ctx, service, mockRepo, mockWorkflowService, _, mockAudit, tenantID, userID := setupTest(t)

	sourceJobID := uuid.Must(uuid.NewV7())
	workflowID := uuid.Must(uuid.NewV7())
	sourceJob := entity.CallJob{
		ID:                        sourceJobID,
		Name:                      "Source Job",
		TenantID:                  tenantID,
		Document_id:               "doc-123",
		Document_type:             "csv",
		Outbound_call_provider_id: "prov-123",
	}

	jobItemIDs := []string{uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()}
	jobItemUUIDs := []uuid.UUID{uuid.MustParse(jobItemIDs[0]), uuid.MustParse(jobItemIDs[1])}

	jobItems := []entity.JobItem{
		{ID: jobItemUUIDs[0], JobID: sourceJobID, PhoneNumber: "123"},
		{ID: jobItemUUIDs[1], JobID: sourceJobID, PhoneNumber: "456"},
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, sourceJobID, tenantID).
		Return(sourceJob, nil).
		Times(1)

	mockWorkflowService.EXPECT().
		GetWorkflow(ctx, workflowID).
		Return(entity.Workflow{
			ID:       workflowID,
			Agent_id: "01KKQJHPRV9SH3ZX42G8G84XHX_ws",
		}, nil).
		Times(1)

	mockRepo.EXPECT().
		GetJobItemsByIDs(ctx, sourceJobID, jobItemUUIDs, tenantID).
		Return(jobItems, nil).
		Times(1)

	mockRepo.EXPECT().
		CreateCallJob(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, job entity.CallJob) error {
			assert.Equal(t, "Copied Job", job.Name)
			assert.Equal(t, sourceJob.Document_id, job.Document_id)
			assert.Equal(t, userID, job.User)
			return nil
		}).
		Times(1)

	mockRepo.EXPECT().
		InsertJobItem(ctx, gomock.Any()).
		Times(2)

	mockAudit.EXPECT().
		Log(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, event servicetypes.AuditEvent) error {
			assert.Equal(t, servicetypes.AuditActionCallJobCopied, event.Action)
			assert.Equal(t, servicetypes.AuditResourceTypeCallJob, event.ResourceType)
			return nil
		}).
		Return(nil).
		Times(1)

	newJobID, count, err := service.CopyCallJob(ctx, sourceJobID.String(), "Copied Job", uuid.Must(uuid.NewV7()), workflowID, "en", jobItemIDs)

	require.NoError(t, err)
	assert.NotEmpty(t, newJobID)
	assert.Equal(t, int32(2), count)
}

//nolint:funlen
func TestCopyCallJob_EmptyOutboundProviderID(t *testing.T) {
	ctx, service, mockRepo, mockWorkflowService, _, mockAudit, tenantID, userID := setupTest(t)

	sourceJobID := uuid.Must(uuid.NewV7())
	workflowID := uuid.Must(uuid.NewV7())
	sourceJob := entity.CallJob{ID: sourceJobID, Name: "Source Job", TenantID: tenantID, Document_id: "doc-123", Document_type: "csv"}

	jobItemIDs := []string{uuid.Must(uuid.NewV7()).String()}
	jobItemUUIDs := []uuid.UUID{uuid.MustParse(jobItemIDs[0])}

	jobItems := []entity.JobItem{
		{ID: jobItemUUIDs[0], JobID: sourceJobID, PhoneNumber: "123"},
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, sourceJobID, tenantID).
		Return(sourceJob, nil).
		Times(1)

	mockWorkflowService.EXPECT().
		GetWorkflow(ctx, workflowID).
		Return(entity.Workflow{
			ID:       workflowID,
			Agent_id: "01KKQJHPRV9SH3ZX42G8G84XHX_ws",
		}, nil).
		Times(1)

	mockRepo.EXPECT().
		GetJobItemsByIDs(ctx, sourceJobID, jobItemUUIDs, tenantID).
		Return(jobItems, nil).
		Times(1)

	mockRepo.EXPECT().
		CreateCallJob(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, job entity.CallJob) error {
			assert.Equal(t, "Copied Job", job.Name)
			assert.Equal(t, sourceJob.Document_id, job.Document_id)
			assert.Equal(t, userID, job.User)
			assert.Empty(t, job.Outbound_call_provider_id)
			return nil
		}).
		Times(1)

	mockRepo.EXPECT().
		InsertJobItem(ctx, gomock.Any()).
		Times(1)

	mockAudit.EXPECT().
		Log(ctx, gomock.Any()).
		Return(nil).
		Times(1)

	newJobID, count, err := service.CopyCallJob(
		ctx,
		sourceJobID.String(),
		"Copied Job",
		uuid.Nil,
		workflowID,
		"en",
		jobItemIDs,
	)

	require.NoError(t, err)
	assert.NotEmpty(t, newJobID)
	assert.Equal(t, int32(1), count)
}

func TestCopyCallJob_InvalidSourceID(t *testing.T) {
	ctx, service, _, _, _, _, _, _ := setupTest(t)

	_, _, err := service.CopyCallJob(ctx, "invalid-uuid", "New Job", uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "en", []string{uuid.Must(uuid.NewV7()).String()})

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
}

func TestCopyCallJob_SourceJobNotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	sourceJobID := uuid.Must(uuid.NewV7())
	mockRepo.EXPECT().
		GetCallJob(ctx, sourceJobID, tenantID).
		Return(entity.CallJob{}, xerrors.NotFoundError(ctx, errors.New("not found"))).
		Times(1)

	_, _, err := service.CopyCallJob(ctx, sourceJobID.String(), "Copied Job", uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "en", []string{uuid.Must(uuid.NewV7()).String()})

	require.Error(t, err)
	assert.True(t, xerrors.IsNotFoundError(err))
}

func TestCopyCallJob_JobItemsMismatch(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	sourceJobID := uuid.Must(uuid.NewV7())
	sourceJob := entity.CallJob{ID: sourceJobID, TenantID: tenantID}

	jobItemIDs := []string{uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()}
	jobItemUUIDs := []uuid.UUID{uuid.MustParse(jobItemIDs[0]), uuid.MustParse(jobItemIDs[1])}

	// Return only one item, simulating mismatch/missing items
	jobItems := []entity.JobItem{
		{ID: jobItemUUIDs[0], JobID: sourceJobID, PhoneNumber: "123"},
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, sourceJobID, tenantID).
		Return(sourceJob, nil).
		Times(1)

	mockRepo.EXPECT().
		GetJobItemsByIDs(ctx, sourceJobID, jobItemUUIDs, tenantID).
		Return(jobItems, nil).
		Times(1)

	_, _, err := service.CopyCallJob(ctx, sourceJobID.String(), "Copied Job", uuid.Must(uuid.NewV7()), uuid.Must(uuid.NewV7()), "en", jobItemIDs)

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "some job_item_ids do not belong to source_job_id")
}

func TestUpdateCallJobDetails_Success(t *testing.T) {
	ctx, service, mockRepo, mockWorkflowService, _, mockAudit, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	workflowID := uuid.Must(uuid.NewV7())
	agentID := "01KKQJHPRV9SH3ZX42G8G84XHX_ws"
	jobName := "Updated Job Name"

	callJob := entity.CallJob{
		ID:       jobID,
		Name:     "Old Name",
		TenantID: tenantID,
		Status:   entity.CallJobReady,
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(callJob, nil).
		Times(1)

	mockWorkflowService.EXPECT().
		GetWorkflow(ctx, workflowID).
		Return(entity.Workflow{
			ID:       workflowID,
			Agent_id: agentID,
		}, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateCallJob(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, job entity.CallJob) error {
			assert.Equal(t, jobName, job.Name)
			return nil
		}).
		Times(1)

	mockAudit.EXPECT().
		Log(ctx, servicetypes.AuditEvent{
			Action:       servicetypes.AuditActionCallJobUpdated,
			ResourceType: servicetypes.AuditResourceTypeCallJob,
			ResourceID:   jobID.String(),
		}).
		Return(nil).
		Times(1)

	err := service.UpdateCallJobDetails(ctx, jobID.String(), jobName, workflowID)

	require.NoError(t, err)
}

func TestUpdateCallJobDetails_InvalidJobID(t *testing.T) {
	ctx, service, _, _, _, _, _, _ := setupTest(t)

	err := service.UpdateCallJobDetails(ctx, "invalid-uuid", "Name", uuid.Must(uuid.NewV7()))

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
}

func TestUpdateCallJobDetails_JobNotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(entity.CallJob{}, xerrors.NotFoundError(ctx, errors.New("not found"))).
		Times(1)

	err := service.UpdateCallJobDetails(ctx, jobID.String(), "New Name", uuid.Must(uuid.NewV7()))

	require.Error(t, err)
	assert.True(t, xerrors.IsNotFoundError(err))
}

func TestListJobItems_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{ID: jobID, TenantID: tenantID}

	jobItems := []entity.JobItem{
		{ID: uuid.Must(uuid.NewV7()), JobID: jobID, PhoneNumber: "123"},
		{ID: uuid.Must(uuid.NewV7()), JobID: jobID, PhoneNumber: "456"},
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(callJob, nil).
		Times(1)

	mockRepo.EXPECT().
		GetJobItemsCount(ctx, jobID, tenantID).
		Return(int32(2), nil).
		Times(1)

	mockRepo.EXPECT().
		GetJobItems(ctx, jobID, int32(10), int32(0), tenantID).
		Return(jobItems, nil).
		Times(1)

	result, total, err := service.ListJobItems(ctx, jobID, 10, 0)

	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, int32(2), total)
}

func TestListJobItems_Pagination(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{ID: jobID, TenantID: tenantID}

	// Repository returns ONLY paged results
	pagedItems := []entity.JobItem{
		{ID: uuid.Must(uuid.NewV7()), JobID: jobID, PhoneNumber: "num-2"},
		{ID: uuid.Must(uuid.NewV7()), JobID: jobID, PhoneNumber: "num-3"},
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(callJob, nil).
		Times(1)

	// Total count should be higher than the page size to properly test pagination
	mockRepo.EXPECT().
		GetJobItemsCount(ctx, jobID, tenantID).
		Return(int32(10), nil). // Total of 10 items
		Times(1)

	mockRepo.EXPECT().
		GetJobItems(ctx, jobID, int32(2), int32(2), tenantID).
		Return(pagedItems, nil).
		Times(1)

	result, total, err := service.ListJobItems(ctx, jobID, 2, 2)

	require.NoError(t, err)
	assert.Len(t, result, 2)          // 2 pages
	assert.Equal(t, int32(10), total) // total is 10
	assert.Equal(t, pagedItems[0].ID, result[0].ID)
	assert.Equal(t, pagedItems[1].ID, result[1].ID)
}

func TestListJobItems_JobNotFound(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(entity.CallJob{}, xerrors.NotFoundError(ctx, errors.New("not found"))).
		Times(1)

	_, _, err := service.ListJobItems(ctx, jobID, 10, 0)

	require.Error(t, err)
	assert.True(t, xerrors.IsNotFoundError(err))
}

func TestAddJobItems_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, mockAudit, tenantID, userID := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{
		ID:       jobID,
		TenantID: tenantID,
		Status:   entity.CallJobReady,
	}

	newItems := []entity.NewJobItem{
		{PhoneNumber: "123", AgentContext: "ctx1"},
		{PhoneNumber: "456", AgentContext: "ctx2"},
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(callJob, nil).
		Times(1)

	mockRepo.EXPECT().
		InsertJobItem(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, item entity.JobItem) error {
			assert.Equal(t, jobID, item.JobID)
			assert.Equal(t, tenantID, item.TenantID)
			assert.Equal(t, userID, item.CreatedBy)
			assert.Equal(t, entity.CallJobItemStatusInitiated, item.Status)
			return nil
		}).
		Times(2)

	mockAudit.EXPECT().
		Log(ctx, servicetypes.AuditEvent{
			Action:       servicetypes.AuditActionCallJobItemsAdded,
			ResourceType: servicetypes.AuditResourceTypeCallJob,
			ResourceID:   jobID.String(),
		}).
		Return(nil).
		Times(1)

	err := service.AddJobItems(ctx, jobID.String(), newItems)

	require.NoError(t, err)
}

func TestAddJobItems_JobNotReady(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{
		ID:       jobID,
		TenantID: tenantID,
		Status:   entity.CallJobPending, // Not Ready
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(callJob, nil).
		Times(1)

	err := service.AddJobItems(ctx, jobID.String(), []entity.NewJobItem{})

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Contains(t, err.Error(), "calljob not in Ready state")
}

func TestRemoveJobItems_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, mockAudit, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{
		ID:       jobID,
		TenantID: tenantID,
		Status:   entity.CallJobReady,
	}

	itemsToRemove := []string{uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String()}

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(callJob, nil).
		Times(1)

	mockRepo.EXPECT().
		DeleteJobItem(ctx, gomock.Any(), jobID, tenantID).
		Times(2)

	mockAudit.EXPECT().
		Log(ctx, servicetypes.AuditEvent{
			Action:       servicetypes.AuditActionCallJobItemsRemoved,
			ResourceType: servicetypes.AuditResourceTypeCallJob,
			ResourceID:   jobID.String(),
		}).
		Return(nil).
		Times(1)

	err := service.RemoveJobItems(ctx, jobID.String(), itemsToRemove)

	require.NoError(t, err)
}

func TestRemoveJobItems_JobNotReady(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)

	jobID := uuid.Must(uuid.NewV7())
	callJob := entity.CallJob{
		ID:       jobID,
		TenantID: tenantID,
		Status:   entity.CallJobPending,
	}

	mockRepo.EXPECT().
		GetCallJob(ctx, jobID, tenantID).
		Return(callJob, nil).
		Times(1)
	err := service.RemoveJobItems(ctx, jobID.String(), []string{uuid.Must(uuid.NewV7()).String()})

	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
}

func TestProcessCallStatusWebhook_Success(t *testing.T) {
	t.Skip("Uses DBOS workflow queues - requires integration test setup")

	ctx, service, mockRepo, _, mockQueue, _, tenantID, _ := setupTest(t)
	jobItemID := uuid.Must(uuid.NewV7())
	sessionID := "session-123"
	callSid := "call-123"
	duration := "120" // 2 minutes

	// 1. Transaction in updateJobItemCallEndDetails
	mockRepo.EXPECT().
		INTERNALGetJobItem(ctx, jobItemID).
		Return(entity.JobItem{ID: jobItemID, TenantID: tenantID}, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateJobItem(ctx, gomock.Any()).
		Return(nil).
		Times(1)

	// 2. sendCallEndEvent (only for completed status)
	mockRepo.EXPECT().
		GetJobItemTenantById(ctx, jobItemID).
		Return(tenantID, nil).
		Times(1)

	mockQueue.EXPECT().
		PublishStringMessage(ctx, gomock.Any(), sessionID, gomock.Any()).
		Return(nil).
		Times(1)

	err := service.ProcessCallStatusWebhook(ctx, callSid, jobItemID, sessionID, "completed", duration, "", "")
	require.NoError(t, err)
}

func TestProcessCallStatusWebhook_NonTerminal(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)
	jobItemID := uuid.Must(uuid.NewV7())
	callSid := "call-123"

	// Transaction in updateJobItemCallEndDetails
	mockRepo.EXPECT().
		INTERNALGetJobItem(ctx, jobItemID).
		Return(entity.JobItem{ID: jobItemID, TenantID: tenantID}, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateJobItem(ctx, gomock.Any()).
		Return(nil).
		Times(1)

	err := service.ProcessCallStatusWebhook(ctx, callSid, jobItemID, "session", "in-progress", "10", "", "")
	require.NoError(t, err)
}

func TestUpdateJobItemWithExternalCallId_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)
	jobItemID := uuid.Must(uuid.NewV7())
	externalID := "ext-123"

	mockRepo.EXPECT().
		INTERNALGetJobItem(ctx, jobItemID).
		Return(entity.JobItem{ID: jobItemID, TenantID: tenantID}, nil).
		Times(1)

	mockRepo.EXPECT().
		UpdateJobItem(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, item entity.JobItem) error {
			assert.Equal(t, externalID, *item.ExternalCallID)
			assert.Equal(t, entity.CallJobItemStatusQueued, item.Status)
			return nil
		}).
		Times(1)

	err := service.updateJobItemWithExternalCallId(ctx, jobItemID, externalID)
	require.NoError(t, err)
}

func TestMarkJobAsFailed_Success(t *testing.T) {
	ctx, service, mockRepo, _, _, _, tenantID, _ := setupTest(t)
	job := entity.CallJob{ID: uuid.Must(uuid.NewV7()), TenantID: tenantID}

	mockRepo.EXPECT().
		UpdateCallJobStatus(ctx, job.ID, entity.CallJobFailed, tenantID).
		Return(nil).
		Times(1)

	err := service.markJobAsFailed(ctx, job)
	require.NoError(t, err)
}

func TestBuildCallStatusUpdate_Success(t *testing.T) {
	logger := zerolog.Nop()
	service := &Service{logger: &logger}

	update := service.buildCallStatusUpdate("completed", "60", "", "")
	assert.Equal(t, entity.CallJobItemStatusCompleted, update.Status)
	assert.NotNil(t, update.CallDuration)
	assert.Equal(t, int32(60), *update.CallDuration)
}

func TestBuildCallStatusUpdate_Error(t *testing.T) {
	logger := zerolog.Nop()
	service := &Service{logger: &logger}

	update := service.buildCallStatusUpdate("failed", "", "", "some error")
	assert.Equal(t, entity.CallJobItemStatusFailed, update.Status)
	assert.NotNil(t, update.CallErrorMessage)
	assert.Equal(t, "some error", *update.CallErrorMessage)
}

func TestSendCallEndEvent_Success(t *testing.T) {
	ctx, service, mockRepo, _, mockQueue, _, tenantID, _ := setupTest(t)
	jobItemID := uuid.Must(uuid.NewV7())
	sessionID := "session-123"

	mockRepo.EXPECT().
		GetJobItemTenantById(ctx, jobItemID).
		Return(tenantID, nil).
		Times(1)

	mockQueue.EXPECT().
		PublishStringMessage(ctx, gomock.Any(), sessionID, gomock.Any()).
		Return(nil).
		Times(1)

	err := service.sendCallEndEvent(ctx, jobItemID, sessionID, 120.0)
	require.NoError(t, err)
}
