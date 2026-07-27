package outboundcallservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Verifies that CopyCallJob successfully copies a job with valid workflow_id.
func (s *OutboundCallServiceSuite) TestCopyCallJob_Success() {
	ctx := s.T().Context()

	// Create a source job with items
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	sourceJobID := s.createCallJobViaAPI(&reqData)

	// Insert job items into the source job
	itemIDs := s.insertJobItems(ctx, sourceJobID, 3)

	// Create a workflow for the copy target
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	targetWorkflowID := s.createTestWorkflow(agentID)

	// Copy the job
	req := connect.NewRequest(&pb.CopyCallJobRequest{
		SourceJobId:            sourceJobID,
		Name:                   "Copied Job " + uuid.NewString(),
		OutboundCallProviderId: uuid.NewString(),
		WorkflowId:             targetWorkflowID,
		PreferedLanguage:       "en",
		JobItemIds:             itemIDs,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.CopyCallJob(ctx, req)
	s.Require().NoError(err, "CopyCallJob should succeed")
	s.Require().NotNil(resp)
	s.NotEmpty(resp.Msg.GetNewJobId(), "Copied job ID should not be empty")
	s.Equal(int32(3), resp.Msg.GetItemsCopied(), "Should copy 3 items")

	// Verify the copied job has the correct workflow_id
	copiedJob := s.getCallJobViaAPI(resp.Msg.GetNewJobId())
	s.Equal(targetWorkflowID, copiedJob.GetWorkflowId(), "Copied job should have target workflow_id")
}

// Verifies that CopyCallJob with invalid workflow_id format returns BadRequest.
func (s *OutboundCallServiceSuite) TestCopyCallJob_InvalidWorkflowIdFormat_BadRequest() {
	ctx := s.T().Context()

	// Create a source job with items
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	sourceJobID := s.createCallJobViaAPI(&reqData)

	// Insert job items
	itemIDs := s.insertJobItems(ctx, sourceJobID, 2)

	// Try to copy with invalid workflow_id format
	req := connect.NewRequest(&pb.CopyCallJobRequest{
		SourceJobId:            sourceJobID,
		Name:                   "Copied Job " + uuid.NewString(),
		OutboundCallProviderId: uuid.NewString(),
		WorkflowId:             "invalid-uuid-format",
		PreferedLanguage:       "en",
		JobItemIds:             itemIDs,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.CopyCallJob(ctx, req)
	s.Require().Error(err, "CopyCallJob with invalid workflow_id format should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// Verifies that CopyCallJob with non-existent source job returns NotFound.
func (s *OutboundCallServiceSuite) TestCopyCallJob_SourceJobNotFound() {
	ctx := s.T().Context()

	// Create a workflow for the copy target
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	targetWorkflowID := s.createTestWorkflow(agentID)

	// Try to copy from non-existent source job
	req := connect.NewRequest(&pb.CopyCallJobRequest{
		SourceJobId:            uuid.NewString(), // Valid UUID but doesn't exist
		Name:                   "Copied Job " + uuid.NewString(),
		OutboundCallProviderId: uuid.NewString(),
		WorkflowId:             targetWorkflowID,
		PreferedLanguage:       "en",
		JobItemIds:             []string{uuid.NewString()},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.CopyCallJob(ctx, req)
	s.Require().Error(err, "CopyCallJob with non-existent source job should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
}

// Verifies that CopyCallJob with empty job_item_ids returns BadRequest.
func (s *OutboundCallServiceSuite) TestCopyCallJob_EmptyJobItemIds_BadRequest() {
	ctx := s.T().Context()

	// Create a source job
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	sourceJobID := s.createCallJobViaAPI(&reqData)

	// Create a workflow for the copy target
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	targetWorkflowID := s.createTestWorkflow(agentID)

	// Try to copy with empty job_item_ids
	req := connect.NewRequest(&pb.CopyCallJobRequest{
		SourceJobId:            sourceJobID,
		Name:                   "Copied Job " + uuid.NewString(),
		OutboundCallProviderId: uuid.NewString(),
		WorkflowId:             targetWorkflowID,
		PreferedLanguage:       "en",
		JobItemIds:             []string{}, // Empty
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.CopyCallJob(ctx, req)
	s.Require().Error(err, "CopyCallJob with empty job_item_ids should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// Verifies that CopyCallJob with non-existent workflow returns NotFound error.
func (s *OutboundCallServiceSuite) TestCopyCallJob_NonExistentWorkflow_NotFound() {
	ctx := s.T().Context()

	// Create a source job with items
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	sourceJobID := s.createCallJobViaAPI(&reqData)

	// Insert job items
	itemIDs := s.insertJobItems(ctx, sourceJobID, 2)

	// Use a non-existent workflow_id
	nonExistentWorkflowID := uuid.NewString()

	// Copy should fail with non-existent workflow
	req := connect.NewRequest(&pb.CopyCallJobRequest{
		SourceJobId:            sourceJobID,
		Name:                   "Copied Job " + uuid.NewString(),
		OutboundCallProviderId: uuid.NewString(),
		WorkflowId:             nonExistentWorkflowID,
		PreferedLanguage:       "en",
		JobItemIds:             itemIDs,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.CopyCallJob(ctx, req)
	s.Require().Error(err, "CopyCallJob with non-existent workflow should fail")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
}
