package outboundcallservice

import (
	"time"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Sample CSV content for testing.
var testCSVContent = []byte(`phone_number,agent_context
+12345678901,Test context 1
+12345678902,Test context 2
+12345678903,Test context 3
`)

// Async test constants.
const (
	asyncTimeout      = 10 * time.Second
	asyncPollInterval = 500 * time.Millisecond
)

// ============================================================================
// MaterializeCallJob Success Tests
// ============================================================================

// Verifies that MaterializeCallJob successfully materializes a job when workflow is PUBLISHED.
// Flow: Create agent → Deploy agent → Create draft workflow → Publish workflow → Create job → Materialize.
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_Success() {
	ctx := s.T().Context()

	// Setup: Create agent and deploy
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	// Setup: Create workflow and publish it (materialization requires PUBLISHED workflow)
	workflowID := s.createDraftWorkflowViaAPI(agentID)
	s.publishWorkflowViaAPI(workflowID)

	// Setup: Upload document
	documentID := s.uploadCSVDocument(testCSVContent)

	// Setup: Create call job with published workflow
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID
	reqData.WorkflowId = workflowID

	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")
	resp, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().NoError(err, "CreateCallJob should succeed")
	jobID := resp.Msg.GetId()

	// Execute: Materialize the job
	err = s.materializeCallJobViaAPI(jobID)
	s.Require().NoError(err, "MaterializeCallJob should succeed with published workflow")

	// Verify async completion
	s.Require().Eventually(func() bool {
		job := s.getCallJobViaAPI(jobID)
		return job.GetIsMaterialized() && job.GetStatus() == pb.CallJobStatus_CALL_JOB_STATUS_READY
	}, asyncTimeout, asyncPollInterval, "Job should be materialized with READY status")

	items := s.listJobItemsViaAPI(jobID)
	s.Equal(int32(3), items.GetTotalCount(), "Should have 3 job items from CSV")
}

// Verifies that MaterializeCallJob is idempotent when workflow is PUBLISHED.
// Flow: Create agent → Deploy agent → Create draft workflow → Publish workflow → Create job → Materialize twice.
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_AlreadyMaterialized_Idempotent() {
	ctx := s.T().Context()

	// Setup: Create agent and deploy
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	// Setup: Create workflow and publish it
	workflowID := s.createDraftWorkflowViaAPI(agentID)
	s.publishWorkflowViaAPI(workflowID)

	// Setup: Upload document
	documentID := s.uploadCSVDocument(testCSVContent)

	// Setup: Create call job with published workflow
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID
	reqData.WorkflowId = workflowID

	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")
	resp, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().NoError(err, "CreateCallJob should succeed")
	jobID := resp.Msg.GetId()

	// First materialization
	err = s.materializeCallJobViaAPI(jobID)
	s.Require().NoError(err, "First MaterializeCallJob should succeed")

	s.Require().Eventually(func() bool {
		return s.listJobItemsViaAPI(jobID).GetTotalCount() == 3
	}, asyncTimeout, asyncPollInterval, "First materialization should create 3 items")

	itemCount := s.listJobItemsViaAPI(jobID).GetTotalCount()

	// Second materialization (should be idempotent)
	err = s.materializeCallJobViaAPI(jobID)
	s.Require().NoError(err, "Second MaterializeCallJob should succeed (idempotent)")

	time.Sleep(2 * time.Second)
	s.Equal(itemCount, s.listJobItemsViaAPI(jobID).GetTotalCount(), "No duplicate items")
}

// ============================================================================
// Workflow Validation Tests
// ============================================================================

// Verifies that MaterializeCallJob fails when workflow_id is empty.
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_EmptyWorkflowId_Fails() {
	documentID := s.uploadCSVDocument(testCSVContent)

	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID
	jobID := s.createCallJobViaAPI(&reqData)

	// set workflow_id to empty - this should fail at the update level
	err := s.updateCallJobWorkflowViaAPI(jobID, "")
	s.Require().Error(err, "UpdateCallJobDetails with empty workflow_id should fail")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(),
		"Error should be InvalidArgument for empty workflow_id")
}

// Verifies that MaterializeCallJob fails when workflow is in draft state (via update).
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_DraftWorkflow_Fails() {
	// Setup: Create agent and deploy
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	// Setup: Create draft workflow (don't publish)
	draftWorkflowID := s.createDraftWorkflowViaAPI(agentID)

	// Setup: Create job with a published workflow first, then update to draft
	documentID := s.uploadCSVDocument(testCSVContent)
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID

	// Create with published workflow first
	reqData.WorkflowId = s.createTestWorkflow(agentID)
	jobID := s.createCallJobViaAPI(&reqData)

	// Update to draft workflow
	err := s.updateCallJobWorkflowViaAPI(jobID, draftWorkflowID)
	s.Require().NoError(err, "UpdateCallJobDetails should succeed")

	err = s.materializeCallJobViaAPI(jobID)
	s.Require().Error(err, "MaterializeCallJob with draft workflow should fail")
	// ...
}

// Verifies that MaterializeCallJob fails when job is created directly with a DRAFT workflow.
// Flow: Create agent → Deploy agent → Create draft workflow → DON'T publish → Create job → Materialize (should FAIL).
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_DraftWorkflow_DirectCreation_Fails() {
	ctx := s.T().Context()

	// Setup: Create agent and deploy
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	// Setup: Create workflow but DON'T publish (stays in DRAFT state)
	draftWorkflowID := s.createDraftWorkflowViaAPI(agentID)

	// Setup: Upload document
	documentID := s.uploadCSVDocument(testCSVContent)

	// Setup: Create call job with DRAFT workflow
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID
	reqData.WorkflowId = draftWorkflowID

	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")
	resp, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().NoError(err, "CreateCallJob should succeed even with draft workflow")
	jobID := resp.Msg.GetId()

	// Execute: Try to materialize with draft workflow - should FAIL
	err = s.materializeCallJobViaAPI(jobID)
	s.Require().Error(err, "MaterializeCallJob with draft workflow should fail")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeFailedPrecondition, connectErr.Code(),
		"Error should be FailedPrecondition for draft workflow")
}

// Verifies that MaterializeCallJob fails when workflow doesn't exist.
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_NonExistentWorkflow_Fails() {
	documentID := s.uploadCSVDocument(testCSVContent)

	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID
	jobID := s.createCallJobViaAPI(&reqData)

	// Set to non-existent workflow_id
	err := s.updateCallJobWorkflowViaAPI(jobID, uuid.NewString())
	s.Require().Error(err, "UpdateCallJobDetails with non-existent workflow should fail")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Contains([]connect.Code{connect.CodeNotFound, connect.CodeFailedPrecondition},
		connectErr.Code(), "Error should be NotFound or FailedPrecondition")
}

// ============================================================================
// Error Scenarios
// ============================================================================

// Verifies that MaterializeCallJob with non-existent job_id returns NotFound.
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_JobNotFound() {
	err := s.materializeCallJobViaAPI(uuid.NewString())
	s.Require().Error(err, "MaterializeCallJob with non-existent job_id should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
}

// Verifies that MaterializeCallJob with invalid job_id format returns BadRequest.
func (s *OutboundCallServiceSuite) TestMaterializeCallJob_InvalidJobIdFormat() {
	err := s.materializeCallJobViaAPI("invalid-uuid-format")
	s.Require().Error(err, "MaterializeCallJob with invalid job_id format should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument")
}
