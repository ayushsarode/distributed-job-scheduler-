package workflowservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Verifies that PublishWorkflow successfully publishes a draft workflow.
func (s *WorkflowServiceSuite) TestPublishWorkflow_Success() {
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	workflowName := "Draft Workflow " + uuid.Must(uuid.NewV7()).String()
	workflowID := s.createTestWorkflow(agentID, workflowName, "Draft workflow to be published")

	workflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT, workflow.GetStatus(), "Workflow should initially be in DRAFT status")

	// Publish the workflow
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.PublishWorkflowRequest{
		WorkflowId: workflowID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.PublishWorkflow(ctx, req)
	s.Require().NoError(err, "PublishWorkflow should succeed")
	s.Require().NotNil(resp)

	publishedWorkflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(pb.WorkflowStatus_WORKFLOW_STATUS_PUBLISHED, publishedWorkflow.GetStatus(), "Workflow should be in PUBLISHED status")
	s.Equal(workflowID, publishedWorkflow.GetId(), "Workflow ID should remain the same")
	s.Equal(workflowName, publishedWorkflow.GetName(), "Workflow name should remain the same")
}

// Verifies that PublishWorkflow returns CodeInvalidArgument when workflow_id is empty.
func (s *WorkflowServiceSuite) TestPublishWorkflow_EmptyWorkflowID() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.PublishWorkflowRequest{
		WorkflowId: "", // Empty workflow ID
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.PublishWorkflow(ctx, req)
	s.Require().Error(err, "PublishWorkflow should fail for empty workflow_id")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument")
	s.Contains(connectErr.Message(), "workflow_id is required", "Error message should mention workflow_id")
}

// Verifies that PublishWorkflow returns CodeInvalidArgument for invalid workflow_id format.
func (s *WorkflowServiceSuite) TestPublishWorkflow_InvalidWorkflowIDFormat() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.PublishWorkflowRequest{
		WorkflowId: "not-a-valid-uuid",
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.PublishWorkflow(ctx, req)
	s.Require().Error(err, "PublishWorkflow should fail for invalid workflow_id format")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument")
	s.Contains(connectErr.Message(), "invalid workflow ID format", "Error message should mention invalid format")
}

// Verifies that PublishWorkflow returns CodeNotFound for a non-existent workflow.
func (s *WorkflowServiceSuite) TestPublishWorkflow_NonExistentWorkflow() {
	nonExistentID := uuid.Must(uuid.NewV7()).String()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.PublishWorkflowRequest{
		WorkflowId: nonExistentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.PublishWorkflow(ctx, req)
	s.Require().Error(err, "PublishWorkflow should fail for non-existent workflow")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
	s.Contains(connectErr.Message(), "not found", "Error message should indicate not found")
}
