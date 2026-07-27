package deploymentservice

import (
	"time"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

// Verifies that DeployAgent returns CodeInvalidArgument when agent_id is empty.
func (s *DeploymentServiceSuite) TestDeployAgent_EmptyAgentID() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeployAgentRequest{AgentId: ""})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.DeployAgent(ctx, req)
	s.Require().Error(err, "DeployAgent should fail for empty agent_id")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	wantMsg := "bad request: agent_id is required"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

func (s *DeploymentServiceSuite) TestDeployAgent_NonExistentAgent() {
	nonExistentID := uuid.Must(uuid.NewV7()).String()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeployAgentRequest{AgentId: nonExistentID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.DeployAgent(ctx, req)
	s.Require().Error(err, "DeployAgent should fail for non-existent agent")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeNotFound
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be NotFound, diff: %s", diff)

	wantMsg := "not found: agent not found"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// TestDeployAgent_RTAgent_DeployedImmediately verifies rt_ prefixed agent deploys immediately without SQS.
func (s *DeploymentServiceSuite) TestDeployAgent_RTAgent_DeployedImmediately() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Deploy the realtime agent
	s.deployAgentViaAPI(agentID)

	// RT agents should be deployed immediately (no external service needed)
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS, agent.GetDeploymentStatus(), "RT agent should be deployed immediately after DeployAgent")
}

// TestDeployAgent_WSAgent_DeployedImmediately verifies ws_ prefixed agent deploys immediately without SQS.
func (s *DeploymentServiceSuite) TestDeployAgent_WSAgent_DeployedImmediately() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_4omini.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Deploy the WS agent
	s.deployAgentViaAPI(agentID)

	// WS agents should be deployed immediately (no external service needed)
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS, agent.GetDeploymentStatus(), "WS agent should be deployed immediately after DeployAgent")
}

// TestDeployAgent_LGAgent_DeployedViaSQS verifies lg_ prefixed agent goes to InProgress status (SQS flow).
func (s *DeploymentServiceSuite) TestDeployAgent_LGAgent_DeployedViaSQS() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_lg.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Deploy the LG agent
	s.deployAgentViaAPI(agentID)

	// LG agents require external deployment via SQS, so should enter InProgress state
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_IN_PROGRESS, agent.GetDeploymentStatus(), "LG agent should enter IN_PROGRESS status (SQS deployment flow)")
}

// Verifies that deploying a template agent (with WS deployment type) sets status to Success immediately.
func (s *DeploymentServiceSuite) TestDeployAgent_TemplateAgent_DeployedImmediately() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_template.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Deploy the template agent
	s.deployAgentViaAPI(agentID)

	// Template agents with WS deployment type should be deployed immediately
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS, agent.GetDeploymentStatus(), "Template agent with WS deployment should be deployed immediately")
}

// Verifies that an LG agent becomes deployed after receiving a success status via SQS.
func (s *DeploymentServiceSuite) TestDeployAgent_Pipeline_Success() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_lg.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Deploy the agent (status becomes InProgress)
	s.deployAgentViaAPI(agentID)

	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_IN_PROGRESS, agent.GetDeploymentStatus(), "LG agent should be in InProgress state initially")

	// Simulate the external deployment service reporting success
	ctx := s.T().Context()
	err := s.sendDeploymentStatusMessage(
		ctx,
		s.Cfg.AgentDeploymentCheckSQSQueueName,
		agentID,
		true, // deployed successfully
	)
	s.Require().NoError(err, "Sending deployment status message should succeed")

	// Poll GetAgent until is_deployed becomes true (poller processes the SQS message)
	s.Eventually(func() bool {
		agent := s.getAgentViaAPI(agentID)
		isDeployed := agent.GetDeploymentStatus() == pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS
		return isDeployed
	}, 10*time.Second, 500*time.Millisecond, "LG agent should become deployed after SQS success callback")
}

// Verifies that an LG agent remains not deployed after receiving a failure status via SQS.
func (s *DeploymentServiceSuite) TestDeployAgent_Pipeline_Failure() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_lg.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Deploy the agent (status becomes InProgress)
	s.deployAgentViaAPI(agentID)

	// Simulate the external deployment service reporting failure
	ctx := s.T().Context()
	err := s.sendDeploymentStatusMessage(
		ctx,
		s.Cfg.AgentDeploymentCheckSQSQueueName,
		agentID,
		false, // deployment failed
	)
	s.Require().NoError(err, "Sending deployment status message should succeed")

	// Wait for the poller to process the message
	time.Sleep(3 * time.Second)

	// Agent should still not be deployed (deployment failed)
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_FAILED, agent.GetDeploymentStatus(), "LG agent should be marked as failed after SQS failure callback")
}

// TestDeployAgent_InvalidPrefix_ReturnsError verifies deploying agent with unknown prefix returns error.
func (s *DeploymentServiceSuite) TestDeployAgent_InvalidPrefix_ReturnsError() {
	// Create an agent with invalid prefix that doesn't match any known type
	invalidAgentID := "xx_01KKQJHPRV9SH3ZX42G8G84XHX"

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeployAgentRequest{AgentId: invalidAgentID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.DeployAgent(ctx, req)
	s.Require().Error(err, "DeployAgent should fail for invalid prefix")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeNotFound
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be NotFound (agent doesn't exist), diff: %s", diff)
}

// TestDeployAgent_NoPrefix_ReturnsError verifies deploying agent with no prefix returns error.
func (s *DeploymentServiceSuite) TestDeployAgent_NoPrefix_ReturnsError() {
	// Create an agent with no prefix (raw ULID)
	noPrefixAgentID := "01KKQJHPRV9SH3ZX42G8G84XHX"

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeployAgentRequest{AgentId: noPrefixAgentID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.DeployAgent(ctx, req)
	s.Require().Error(err, "DeployAgent should fail for no prefix")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeNotFound
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be NotFound (agent doesn't exist), diff: %s", diff)
}
