package workflowservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Verifies that UpdateWorkflow successfully updates all fields of a workflow.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_Success() {
	agentID := s.createAgentViaAPI()

	originalName := "Original Workflow " + uuid.Must(uuid.NewV7()).String()
	originalDesc := "Original description"
	workflowID := s.createTestWorkflow(agentID, originalName, originalDesc)

	// Create a second agent for update
	agent2ID := s.createAgentViaAPI()

	updatedName := "Updated Workflow " + uuid.Must(uuid.NewV7()).String()
	updatedDesc := "Updated description"

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        updatedName,
		Description: updatedDesc,
		AgentId:     agent2ID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().NoError(err, "UpdateWorkflow should succeed")
	s.Require().NotNil(resp)

	workflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(workflowID, workflow.GetId(), "Workflow ID should remain the same")
	s.Equal(updatedName, workflow.GetName(), "Workflow name should be updated")
	s.Equal(updatedDesc, workflow.GetDescription(), "Workflow description should be updated")
	s.Equal(agent2ID, workflow.GetAgentId(), "Agent ID should be updated")
	s.NotNil(workflow.GetUpdatedAt(), "Updated timestamp should be set")
}

// Verifies that UpdateWorkflow only updates the name.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_UpdateNameOnly() {
	agentID := s.createAgentViaAPI()

	originalName := "Original Name " + uuid.Must(uuid.NewV7()).String()
	originalDesc := "Original description"
	workflowID := s.createTestWorkflow(agentID, originalName, originalDesc)

	updatedName := "Updated Name " + uuid.Must(uuid.NewV7()).String()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        updatedName,
		Description: originalDesc,
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().NoError(err, "UpdateWorkflow should succeed")
	s.Require().NotNil(resp)

	workflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(updatedName, workflow.GetName(), "Workflow name should be updated")
	s.Equal(originalDesc, workflow.GetDescription(), "Description should remain unchanged")
	s.Equal(agentID, workflow.GetAgentId(), "Agent ID should remain unchanged")
}

// Verifies that UpdateWorkflow only updates the description.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_UpdateDescriptionOnly() {
	agentID := s.createAgentViaAPI()

	originalName := "Original Name " + uuid.Must(uuid.NewV7()).String()
	originalDesc := "Original description"
	workflowID := s.createTestWorkflow(agentID, originalName, originalDesc)

	updatedDesc := "This is the updated description for testing"

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        originalName,
		Description: updatedDesc,
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().NoError(err, "UpdateWorkflow should succeed")
	s.Require().NotNil(resp)

	workflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(originalName, workflow.GetName(), "Workflow name should remain unchanged")
	s.Equal(updatedDesc, workflow.GetDescription(), "Description should be updated")
	s.Equal(agentID, workflow.GetAgentId(), "Agent ID should remain unchanged")
}

// Verifies that UpdateWorkflow only updates the agent ID.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_UpdateAgentIdOnly() {
	agent1ID := s.createAgentViaAPI()
	agent2ID := s.createAgentViaAPI()

	originalName := "Original Name " + uuid.Must(uuid.NewV7()).String()
	originalDesc := "Original description"
	workflowID := s.createTestWorkflow(agent1ID, originalName, originalDesc)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        originalName,
		Description: originalDesc,
		AgentId:     agent2ID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().NoError(err, "UpdateWorkflow should succeed")
	s.Require().NotNil(resp)

	workflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(originalName, workflow.GetName(), "Workflow name should remain unchanged")
	s.Equal(originalDesc, workflow.GetDescription(), "Description should remain unchanged")
	s.Equal(agent2ID, workflow.GetAgentId(), "Agent ID should be updated")
}

// Verifies that UpdateWorkflow returns CodeInvalidArgument when workflow_id is empty.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_EmptyWorkflowID() {
	agentID := s.createAgentViaAPI()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  "", // Empty workflow ID
		Name:        "Some Name",
		Description: "Some description",
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().Error(err, "UpdateWorkflow should fail for empty workflow_id")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument")
	s.Contains(connectErr.Message(), "workflow_id is required", "Error message should mention workflow_id")
}

// Verifies that UpdateWorkflow returns CodeInvalidArgument when agent_id is empty.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_EmptyAgentID() {
	agentID := s.createAgentViaAPI()

	workflowID := s.createTestWorkflow(agentID, "Test "+uuid.Must(uuid.NewV7()).String(), "Test")

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        "Updated Name",
		Description: "Updated description",
		AgentId:     "", // Empty agent ID
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().Error(err, "UpdateWorkflow should fail for empty agent_id")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument")
	s.Contains(connectErr.Message(), "agent_id is required", "Error message should mention agent_id")
}

// Verifies that UpdateWorkflow returns CodeNotFound for a non-existent workflow.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_NonExistentWorkflow() {
	agentID := s.createAgentViaAPI()

	// Use a valid UUID that doesn't exist
	nonExistentID := uuid.Must(uuid.NewV7()).String()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  nonExistentID,
		Name:        "Updated Name",
		Description: "Updated description",
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().Error(err, "UpdateWorkflow should fail for non-existent workflow")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
	s.Contains(connectErr.Message(), "not found", "Error message should indicate not found")
}

// Verifies that UpdateWorkflow allows updating with empty name.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_EmptyName() {
	agentID := s.createAgentViaAPI()

	originalName := "Original Name " + uuid.Must(uuid.NewV7()).String()
	workflowID := s.createTestWorkflow(agentID, originalName, "Test description")

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        "", // Empty name
		Description: "Updated description",
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.workflowClient.UpdateWorkflow(ctx, req)

	s.Require().Error(err, "UpdateWorkflow should fail for empty name")
	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)
	s.Equal(connect.CodeInvalidArgument, connectErr.Code())
}

// Verifies that UpdateWorkflow is allowed when workflow is in DRAFT state.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_DraftState_AllowsUpdate() {
	agentID := s.createAgentViaAPI()

	workflowID := s.createTestWorkflow(
		agentID,
		"Draft Workflow "+uuid.Must(uuid.NewV7()).String(),
		"Draft description",
	)

	updatedName := "Updated Draft Name"

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        updatedName,
		Description: "Updated description",
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)
	s.Require().NoError(err, "Update should succeed for draft workflow")
	s.Require().NotNil(resp)

	workflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(updatedName, workflow.GetName())
}

// Verifies that UpdateWorkflow is allowed when workflow is in INACTIVE state.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_InactiveState_AllowsUpdate() {
	agentID := s.createAgentViaAPI()

	workflowID := s.createTestWorkflow(
		agentID,
		"Inactive Workflow "+uuid.Must(uuid.NewV7()).String(),
		"Inactive description",
	)

	s.deactivateWorkflowViaAPI(workflowID)

	updatedName := "Updated Inactive Name"

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        updatedName,
		Description: "Updated description",
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)

	s.Require().NoError(err, "Update should succeed for inactive workflow")
	s.Require().NotNil(resp)

	workflow := s.getWorkflowViaAPI(workflowID)
	s.Equal(updatedName, workflow.GetName())
}

// Verifies that UpdateWorkflow fails with PreconditionFailed when workflow is PUBLISHED.
func (s *WorkflowServiceSuite) TestUpdateWorkflow_PublishedState_PreconditionFailed() {
	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)

	workflowID := s.createTestWorkflow(
		agentID,
		"Published Workflow "+uuid.Must(uuid.NewV7()).String(),
		"Published description",
	)

	s.publishWorkflowViaAPI(workflowID)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateWorkflowRequest{
		WorkflowId:  workflowID,
		Name:        "Illegal Update",
		Description: "Should fail",
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.UpdateWorkflow(ctx, req)

	s.Require().Error(err, "Update should fail for published workflow")
	s.Require().Nil(resp)

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr)

	s.Equal(
		connect.CodeFailedPrecondition,
		connectErr.Code(),
		"Published workflows should not be updatable",
	)
}
