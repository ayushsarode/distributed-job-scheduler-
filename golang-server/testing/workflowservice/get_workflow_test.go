package workflowservice

import (
	"fmt"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

// Verifies that GetWorkflow returns all expected fields for a newly created workflow.
func (s *WorkflowServiceSuite) TestGetWorkflow_Success() {
	agentID := s.createAgentViaAPI()
	s.NotEmpty(agentID, "Agent ID should not be empty")

	var reqData pb.CreateWorkflowRequest
	s.LoadTestData(serviceName, "request/create_workflow.json", &reqData)
	reqData.AgentId = agentID
	reqData.Name = "Test Workflow " + uuid.Must(uuid.NewV7()).String()

	workflowId := s.createWorkflowViaAPI(&reqData)
	workflow := s.getWorkflowViaAPI(workflowId)

	s.Equal(workflowId, workflow.GetId(), "Workflow ID should match")
	s.Equal(reqData.GetName(), workflow.GetName(), "Workflow name should match")
	s.Equal("Test workflow for integration testing", workflow.GetDescription(), "Description should match")
	s.Equal(pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT, workflow.GetStatus(), "Newly created workflow should be in draft status")
	s.NotNil(workflow.GetCreatedAt(), "Created timestamp should be set")
	s.NotNil(workflow.GetUpdatedAt(), "Updated timestamp should be set")
	s.NotEmpty(workflow.GetCreatedBy(), "Created by should be set")
	s.Equal(int32(0), workflow.GetActiveCampaignsCount(), "New workflow should have 0 active campaigns")
	s.Equal(int32(0), workflow.GetTotalCalls(), "New workflow should have 0 total calls")
}

// Verifies that GetWorkflow returns CodeInvalidArgument when workflow_id is empty.
func (s *WorkflowServiceSuite) TestGetWorkflow_EmptyWorkflowID() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetWorkflowRequest{WorkflowId: ""})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.GetWorkflow(ctx, req)
	s.Require().Error(err, "GetWorkflow should fail for empty workflow_id")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	wantMsg := "bad request: workflow_id is required"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// Verifies that GetWorkflow returns CodeInvalidArgument for a malformed (non-UUID) workflow_id.
func (s *WorkflowServiceSuite) TestGetWorkflow_InvalidUUIDFormat() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetWorkflowRequest{WorkflowId: "not-a-valid-uuid"})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.GetWorkflow(ctx, req)
	s.Require().Error(err, "GetWorkflow should fail for invalid UUID format")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument for malformed UUID, diff: %s", diff)

	wantMsg := "bad request: invalid workflow ID format"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// Verifies that GetWorkflow returns CodeNotFound for a valid UUID that doesn't exist in the database.
func (s *WorkflowServiceSuite) TestGetWorkflow_NonExistentWorkflow() {
	nonExistentID := uuid.Must(uuid.NewV7()).String()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetWorkflowRequest{WorkflowId: nonExistentID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.GetWorkflow(ctx, req)
	s.Require().Error(err, "GetWorkflow should fail for non-existent workflow")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeNotFound
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be NotFound, diff: %s", diff)

	wantMsg := fmt.Sprintf("not found: workflow with id %s not found", nonExistentID)
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}
