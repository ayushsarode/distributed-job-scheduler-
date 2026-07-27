package outboundcallservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// ============================================================================
// CreateCallJob Tests
// ============================================================================

// Verifies that CreateCallJob successfully creates a job with a valid workflow_id.
func (s *OutboundCallServiceSuite) TestCreateCallJob_Success() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)
	s.NotEmpty(jobID, "Call job ID should not be empty")

	// Verify the job was created with the correct workflow_id
	job := s.getCallJobViaAPI(jobID)
	s.NotEmpty(job.GetWorkflowId(), "Workflow ID should be set")
	s.Equal(reqData.GetName(), job.GetName(), "Job name should match")
}

// Verifies that CreateCallJob with invalid workflow_id format returns BadRequest.
func (s *OutboundCallServiceSuite) TestCreateCallJob_InvalidWorkflowIdFormat_BadRequest() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.WorkflowId = "invalid-uuid-format"

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().Error(err, "CreateCallJob with invalid workflow_id format should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// Verifies that CreateCallJob with non-existent workflow_id returns NotFound.
func (s *OutboundCallServiceSuite) TestCreateCallJob_NonExistentWorkflowId_NotFound() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.WorkflowId = uuid.NewString() // Valid UUID but doesn't exist

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().Error(err, "CreateCallJob with non-existent workflow_id should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
}

// ============================================================================
// GetCallJob Tests
// ============================================================================

// Verifies that GetCallJob returns the job with workflow_id populated.
func (s *OutboundCallServiceSuite) TestGetCallJob_Success_VerifyWorkflowId() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	job := s.getCallJobViaAPI(jobID)
	s.NotEmpty(job.GetId(), "Job ID should not be empty")
	s.NotEmpty(job.GetName(), "Job name should not be empty")
	s.NotEmpty(job.GetWorkflowId(), "Workflow ID should not be empty")
	s.Equal(reqData.GetName(), job.GetName(), "Job name should match request")
}

// Verifies that GetCallJob with non-existent ID returns NotFound.
func (s *OutboundCallServiceSuite) TestGetCallJob_NotFound() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetCallJobRequest{Id: uuid.NewString()})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.GetCallJob(ctx, req)
	s.Require().Error(err, "GetCallJob with non-existent ID should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
}

// Verifies that GetCallJob with invalid ID format returns an error.
func (s *OutboundCallServiceSuite) TestGetCallJob_InvalidId_BadRequest() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetCallJobRequest{Id: "invalid-job-id"})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.GetCallJob(ctx, req)
	s.Require().Error(err, "GetCallJob with invalid ID should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.NotEmpty(connectErr.Code(), "Error code should be present")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// ============================================================================
// DeleteCallJob Tests
// ============================================================================

// Verifies that DeleteCallJob successfully deletes a job.
func (s *OutboundCallServiceSuite) TestDeleteCallJob_Success() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// Delete the job
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteCallJobRequest{Id: jobID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.DeleteCallJob(ctx, req)
	s.Require().NoError(err, "DeleteCallJob should succeed")
	s.Require().NotNil(resp)

	// Verify the job no longer exists
	getReq := connect.NewRequest(&pb.GetCallJobRequest{Id: jobID})
	getReq.Header().Set("Authorization", "Bearer test-token")

	_, err = s.outboundClient.GetCallJob(ctx, getReq)
	s.Require().Error(err, "GetCallJob should fail after deletion")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound after deletion")
}

// Verifies that DeleteCallJob with non-existent ID behavior.
func (s *OutboundCallServiceSuite) TestDeleteCallJob_NotFound() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteCallJobRequest{Id: uuid.NewString()})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.DeleteCallJob(ctx, req)
	s.Require().NoError(err, "DeleteCallJob should succeed")
}

// Verifies that DeleteCallJob with invalid ID format returns BadRequest.
func (s *OutboundCallServiceSuite) TestDeleteCallJob_InvalidId_BadRequest() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeleteCallJobRequest{Id: "invalid-job-id"})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.DeleteCallJob(ctx, req)
	s.Require().Error(err, "DeleteCallJob with invalid ID should return error")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}
