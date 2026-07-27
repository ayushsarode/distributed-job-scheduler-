package outboundcallservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// ============================================================================
// UpdateCallJobDetails Tests
// ============================================================================

// Verifies that UpdateCallJobDetails successfully updates a job with valid workflow_id.
func (s *OutboundCallServiceSuite) TestUpdateCallJobDetails_Success() {
	ctx := s.T().Context()

	// Create a job
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	// Create a new workflow for the update
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	newWorkflowID := s.createTestWorkflow(agentID)
	newName := "Updated Job " + uuid.NewString()

	// Update the job
	req := connect.NewRequest(&pb.UpdateCallJobDetailsRequest{
		JobId:      jobID,
		Name:       newName,
		WorkflowId: newWorkflowID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.UpdateCallJobDetails(ctx, req)
	s.Require().NoError(err, "UpdateCallJobDetails should succeed")
	s.Require().NotNil(resp)

	// Verify the name and workflow_id updates
	updatedJob := s.getCallJobViaAPI(jobID)
	s.Equal(newName, updatedJob.GetName(), "Job name should be updated")
	s.Equal(newWorkflowID, updatedJob.GetWorkflowId(), "Job workflow_id should be updated")
}

// Verifies that UpdateCallJobDetails with non-existent workflow_id returns error.
// The service validates workflow existence and returns an error when not found.
func (s *OutboundCallServiceSuite) TestUpdateCallJobDetails_NonExistentWorkflowId_Error() {
	ctx := s.T().Context()

	// Create a job
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	// Try to update with non-existent workflow_id
	req := connect.NewRequest(&pb.UpdateCallJobDetailsRequest{
		JobId:      jobID,
		Name:       "Updated Job",
		WorkflowId: uuid.NewString(), // Valid UUID but doesn't exist
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.UpdateCallJobDetails(ctx, req)
	s.Require().Error(err, "UpdateCallJobDetails with non-existent workflow_id should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	// The service wraps errors, so we expect either NotFound or InvalidArgument
	s.Contains([]connect.Code{connect.CodeNotFound, connect.CodeInvalidArgument}, connectErr.Code(),
		"Error code should be NotFound or InvalidArgument")
}

// Verifies that UpdateCallJobDetails with invalid workflow_id format returns BadRequest.
func (s *OutboundCallServiceSuite) TestUpdateCallJobDetails_InvalidWorkflowId_BadRequest() {
	ctx := s.T().Context()

	// Create a job
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	// Try to update with invalid workflow_id format
	req := connect.NewRequest(&pb.UpdateCallJobDetailsRequest{
		JobId:      jobID,
		Name:       "Updated Job",
		WorkflowId: "invalid-uuid-format",
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.UpdateCallJobDetails(ctx, req)
	s.Require().Error(err, "UpdateCallJobDetails with invalid workflow_id should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// Verifies that UpdateCallJobDetails on non-editable job returns BadRequest.
// Jobs that are not in PENDING or READY state cannot be updated.
func (s *OutboundCallServiceSuite) TestUpdateCallJobDetails_JobNotEditable_BadRequest() {
	ctx := s.T().Context()

	// Create a job with RUNNING status
	jobID := s.insertCallJobWithStatus(ctx, "running", "NonEditableJob-"+uuid.NewString())

	// Create a new workflow for the update
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	newWorkflowID := s.createTestWorkflow(agentID)

	// Try to update the non-editable job
	req := connect.NewRequest(&pb.UpdateCallJobDetailsRequest{
		JobId:      jobID,
		Name:       "Updated Job",
		WorkflowId: newWorkflowID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.UpdateCallJobDetails(ctx, req)
	s.Require().Error(err, "UpdateCallJobDetails on non-editable job should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// ============================================================================
// AddJobItems Tests
// ============================================================================

// Verifies that AddJobItems successfully adds items to a READY job.
func (s *OutboundCallServiceSuite) TestAddJobItems_Success() {
	ctx := s.T().Context()

	// Create a job with READY status
	jobID := s.insertCallJobWithReadyStatus(ctx, "ReadyJob-"+uuid.NewString())

	// Add job items
	req := connect.NewRequest(&pb.AddJobItemsRequest{
		JobId: jobID,
		Items: []*pb.NewJobItem{
			{
				PhoneNumber:  "+1234567890",
				AgentContext: `{"key": "value"}`,
			},
			{
				PhoneNumber:  "+0987654321",
				AgentContext: `{"key": "value2"}`,
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.AddJobItems(ctx, req)
	s.Require().NoError(err, "AddJobItems should succeed")
	s.Require().NotNil(resp)

	// Verify items were added by listing
	listReq := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  100,
		Offset: 0,
	})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.outboundClient.ListJobItems(ctx, listReq)
	s.Require().NoError(err)
	s.GreaterOrEqual(listResp.Msg.GetTotalCount(), int32(2), "Should have at least 2 items")
}

// Verifies that AddJobItems to a non-READY job returns BadRequest.
func (s *OutboundCallServiceSuite) TestAddJobItems_JobNotReady_BadRequest() {
	ctx := s.T().Context()

	// Create a job with PENDING status (not READY)
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	// Try to add items to PENDING job
	req := connect.NewRequest(&pb.AddJobItemsRequest{
		JobId: jobID,
		Items: []*pb.NewJobItem{
			{
				PhoneNumber:  "+1234567890",
				AgentContext: `{"key": "value"}`,
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.AddJobItems(ctx, req)
	s.Require().Error(err, "AddJobItems to non-READY job should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// ============================================================================
// RemoveJobItems Tests
// ============================================================================

// Verifies that RemoveJobItems successfully removes items from a READY job.
func (s *OutboundCallServiceSuite) TestRemoveJobItems_Success() {
	ctx := s.T().Context()

	// Create a job with READY status
	jobID := s.insertCallJobWithReadyStatus(ctx, "ReadyJob-"+uuid.NewString())

	// Insert job items
	itemIDs := s.insertJobItems(ctx, jobID, 3)

	// Remove one item
	req := connect.NewRequest(&pb.RemoveJobItemsRequest{
		JobId:      jobID,
		JobItemIds: []string{itemIDs[0]},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.RemoveJobItems(ctx, req)
	s.Require().NoError(err, "RemoveJobItems should succeed")
	s.Require().NotNil(resp)

	// Verify item was removed
	listReq := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  100,
		Offset: 0,
	})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.outboundClient.ListJobItems(ctx, listReq)
	s.Require().NoError(err)

	// Should have 2 items remaining
	s.Equal(int32(2), listResp.Msg.GetTotalCount(), "Should have 2 items remaining after removal")
}

// Verifies that RemoveJobItems from a non-READY job returns BadRequest.
func (s *OutboundCallServiceSuite) TestRemoveJobItems_JobNotReady_BadRequest() {
	ctx := s.T().Context()

	// Create a job with PENDING status
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	// Insert job items
	itemIDs := s.insertJobItems(ctx, jobID, 2)

	// Try to remove items from PENDING job
	req := connect.NewRequest(&pb.RemoveJobItemsRequest{
		JobId:      jobID,
		JobItemIds: []string{itemIDs[0]},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.RemoveJobItems(ctx, req)
	s.Require().Error(err, "RemoveJobItems from non-READY job should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}
