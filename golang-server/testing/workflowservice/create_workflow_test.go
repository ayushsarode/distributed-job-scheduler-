package workflowservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/go-cmp/cmp"
)

func (s *WorkflowServiceSuite) TestCreateWorkflow_Success() {
	agentID := s.createAgentViaAPI()
	s.NotEmpty(agentID, "Agent ID should not be empty")

	var reqData pb.CreateWorkflowRequest
	s.LoadTestData(serviceName, "request/create_workflow.json", &reqData)
	reqData.AgentId = agentID

	workflowId := s.createWorkflowViaAPI(&reqData)

	s.NotEmpty(workflowId, "Workflow ID should not be empty")
}

func (s *WorkflowServiceSuite) TestCreateWorkflow_InvalidRequest_EmptyName() {
	var reqData pb.CreateWorkflowRequest
	s.LoadTestData(serviceName, "request/create_workflow_invalid_empty_name.json", &reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.CreateWorkflow(ctx, req)
	s.Require().Error(err, "CreateWorkflow request should fail with invalid input")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should match, diff: %s", diff)

	wantMsg := "bad request: name is required"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

func (s *WorkflowServiceSuite) TestCreateWorkflow_InvalidRequest_EmptyAgentId() {
	var reqData pb.CreateWorkflowRequest
	s.LoadTestData(serviceName, "request/create_workflow_invalid_empty_agent_id.json", &reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.CreateWorkflow(ctx, req)
	s.Require().Error(err, "CreateWorkflow request should fail with invalid input")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should match, diff: %s", diff)

	// The error message will be about empty agent_id
	wantMsg := "bad request: agent_id is required"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}
