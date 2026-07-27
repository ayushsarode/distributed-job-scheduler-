package workflowservice

import (
	"time"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Verifies that DeleteWorkflow successfully deletes a workflow.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_Success() {
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	workflowID := s.createTestWorkflow(
		agentID,
		"Workflow "+uuid.Must(uuid.NewV7()).String(),
		"Workflow to delete",
	)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteWorkflowRequest{
		WorkflowId: workflowID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.workflowClient.DeleteWorkflow(ctx, req)
	s.Require().NoError(err, "DeleteWorkflow should succeed")

	// Verify workflow no longer exists
	getReq := connect.NewRequest(&pb.GetWorkflowRequest{
		WorkflowId: workflowID,
	})
	getReq.Header().Set("Authorization", "Bearer test-token")

	_, err = s.workflowClient.GetWorkflow(ctx, getReq)
	s.Require().Error(err, "Deleted workflow should not be retrievable")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeNotFound, connectErr.Code())
}

// Verifies that DeleteWorkflow fails when workflow_id is empty.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_EmptyWorkflowID() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteWorkflowRequest{
		WorkflowId: "",
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.DeleteWorkflow(ctx, req)
	s.Require().Error(err)
	s.Require().Nil(resp)

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeInvalidArgument, connectErr.Code())
}

// Verifies that DeleteWorkflow fails for invalid workflow_id format.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_InvalidWorkflowIDFormat() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteWorkflowRequest{
		WorkflowId: "not-a-uuid",
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.DeleteWorkflow(ctx, req)
	s.Require().Error(err)
	s.Require().Nil(resp)

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeInvalidArgument, connectErr.Code())
}

// Verifies that DeleteWorkflow returns NotFound for non-existent workflow.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_NonExistentWorkflow() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteWorkflowRequest{
		WorkflowId: uuid.Must(uuid.NewV7()).String(),
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.DeleteWorkflow(ctx, req)
	s.Require().Error(err)
	s.Require().Nil(resp)

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeNotFound, connectErr.Code())
}

// Verifies that deleting a workflow makes it inaccessible.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_WorkflowNoLongerAccessible() {
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	workflowID := s.createTestWorkflow(
		agentID,
		"Workflow "+uuid.Must(uuid.NewV7()).String(),
		"Workflow to delete",
	)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteWorkflowRequest{
		WorkflowId: workflowID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.workflowClient.DeleteWorkflow(ctx, req)
	s.Require().NoError(err)

	// Verify cannot fetch
	_, err = s.workflowClient.GetWorkflow(ctx, connect.NewRequest(
		&pb.GetWorkflowRequest{WorkflowId: workflowID},
	))
	s.Require().Error(err)
}

// ============================================================================
// Call Job Status Blocking Tests
// ============================================================================

// Verifies that DeleteWorkflow succeeds when call job is in PENDING status.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_JobInPendingStatus_Allowed() {
	ctx := s.T().Context()

	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	workflowID := s.createTestWorkflow(agentID, "PendingJob-Workflow-"+uuid.Must(uuid.NewV7()).String(), "Description")

	// Create call job (PENDING by default)
	documentID := s.uploadCSVDocument(testCSVContent)
	var reqData pb.CreateCallJobRequest
	s.LoadTestData("outboundcallservice", "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID
	reqData.WorkflowId = workflowID

	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")
	_, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().NoError(err, "CreateCallJob should succeed")

	// Delete should succeed - PENDING jobs don't block deletion
	delReq := connect.NewRequest(&pb.DeleteWorkflowRequest{WorkflowId: workflowID})
	delReq.Header().Set("Authorization", "Bearer test-token")
	_, err = s.workflowClient.DeleteWorkflow(ctx, delReq)
	s.Require().NoError(err, "PENDING jobs should not block deletion")
}

// Verifies that DeleteWorkflow is blocked when call job is in PROCESSING status.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_JobInProcessingStatus_Blocked() {
	ctx := s.T().Context()

	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	workflowID := s.createTestWorkflow(agentID, "ProcessingJob-Workflow-"+uuid.Must(uuid.NewV7()).String(), "Description")

	// Insert job with PROCESSING status (transient state during materialization)
	s.insertCallJobWithStatus(ctx, "processing", workflowID, "ProcessingJob")

	// Delete should fail
	req := connect.NewRequest(&pb.DeleteWorkflowRequest{WorkflowId: workflowID})
	req.Header().Set("Authorization", "Bearer test-token")
	_, err := s.workflowClient.DeleteWorkflow(ctx, req)

	s.Require().Error(err, "PROCESSING jobs should block deletion")
	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeFailedPrecondition, connectErr.Code())
}

// Verifies that DeleteWorkflow is blocked when call job is in READY status.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_JobInReadyStatus_Blocked() {
	ctx := s.T().Context()

	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	workflowID := s.createTestWorkflow(agentID, "ReadyJob-Workflow-"+uuid.Must(uuid.NewV7()).String(), "Description")

	s.publishWorkflowViaAPI(workflowID)

	// Create and materialize job to READY status
	documentID := s.uploadCSVDocument(testCSVContent)
	var reqData pb.CreateCallJobRequest
	s.LoadTestData("outboundcallservice", "request/create_call_job.json", &reqData)
	reqData.DocumentId = documentID
	reqData.WorkflowId = workflowID

	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")
	resp, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().NoError(err, "CreateCallJob should succeed")
	jobID := resp.Msg.GetId()

	err = s.materializeCallJobViaAPI(jobID)
	s.Require().NoError(err, "MaterializeJob should succeed")

	// Wait for READY status
	s.Require().Eventually(func() bool {
		job := s.getCallJobViaAPI(jobID)
		return job.GetStatus() == pb.CallJobStatus_CALL_JOB_STATUS_READY
	}, 10*time.Second, 500*time.Millisecond, "Job should reach READY status")

	// Delete should fail
	delReq := connect.NewRequest(&pb.DeleteWorkflowRequest{WorkflowId: workflowID})
	delReq.Header().Set("Authorization", "Bearer test-token")
	_, err = s.workflowClient.DeleteWorkflow(ctx, delReq)

	s.Require().Error(err, "READY jobs should block deletion")
	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeFailedPrecondition, connectErr.Code())
}

// Verifies that DeleteWorkflow is blocked when call job is in RUNNING status.
func (s *WorkflowServiceSuite) TestDeleteWorkflow_JobInRunningStatus_Blocked() {
	ctx := s.T().Context()

	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	workflowID := s.createTestWorkflow(agentID, "RunningJob-Workflow-"+uuid.Must(uuid.NewV7()).String(), "Description")

	// Insert job with RUNNING status (no API to start job)
	s.insertCallJobWithStatus(ctx, "running", workflowID, "RunningJob")

	// Delete should fail
	req := connect.NewRequest(&pb.DeleteWorkflowRequest{WorkflowId: workflowID})
	req.Header().Set("Authorization", "Bearer test-token")
	_, err := s.workflowClient.DeleteWorkflow(ctx, req)

	s.Require().Error(err, "RUNNING jobs should block deletion")
	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeFailedPrecondition, connectErr.Code())
}
