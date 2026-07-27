package deploymentservice

import (
	"strings"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
)

// TestCreateAgent_RTDeployment_GeneratesRTSuffix verifies RT deployment type creates agent with "_rt" suffix.
func (s *DeploymentServiceSuite) TestCreateAgent_RTDeployment_GeneratesRTSuffix() {
	ctx := s.T().Context()

	request := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "RT Test Agent",
				Prompt:    "Test prompt for RT agent",
				Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
	}

	req := connect.NewRequest(request)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed")
	s.Require().NotNil(resp)

	agentID := resp.Msg.GetAgentId()
	s.NotEmpty(agentID, "Agent ID should not be empty")
	s.True(strings.HasSuffix(agentID, "_rt"), "Agent ID should have '_rt' suffix for RT deployment type")
}

// TestCreateAgent_WSDeployment_GeneratesWSSuffix verifies WS deployment type creates agent with "_ws" suffix.
func (s *DeploymentServiceSuite) TestCreateAgent_WSDeployment_GeneratesWSSuffix() {
	ctx := s.T().Context()

	request := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "WS Test Agent",
				Prompt:    "Test prompt for WS agent",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
	}

	req := connect.NewRequest(request)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed")
	s.Require().NotNil(resp)

	agentID := resp.Msg.GetAgentId()
	s.NotEmpty(agentID, "Agent ID should not be empty")
	s.True(strings.HasSuffix(agentID, "_ws"), "Agent ID should have '_ws' suffix for WS deployment type")
}

// TestCreateAgent_LGDeployment_GeneratesLGSuffix verifies LG deployment type creates agent with "_lg" suffix.
func (s *DeploymentServiceSuite) TestCreateAgent_LGDeployment_GeneratesLGSuffix() {
	ctx := s.T().Context()

	request := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "LG Test Agent",
				Prompt:    "Test prompt for LG agent",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
	}

	req := connect.NewRequest(request)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed")
	s.Require().NotNil(resp)

	agentID := resp.Msg.GetAgentId()
	s.NotEmpty(agentID, "Agent ID should not be empty")
	s.True(strings.HasSuffix(agentID, "_lg"), "Agent ID should have '_lg' suffix for LG deployment type")
}

// TestCreateAgent_RTWithNonRealtimeModel_ReturnsError verifies RT deployment with non-realtime model fails.
func (s *DeploymentServiceSuite) TestCreateAgent_RTWithNonRealtimeModel_ReturnsError() {
	ctx := s.T().Context()

	request := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "RT Test Agent with Wrong Model",
				Prompt:    "Test prompt",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI, // Wrong model for RT
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
	}

	req := connect.NewRequest(request)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail with wrong model for RT deployment")
	s.Nil(resp)
	s.Contains(err.Error(), "RT deployment type requires GPT_REALTIME model", "Error should indicate model mismatch")
}

// TestCreateAndDeployAgent_RTDeployment_DeploysSuccessfully verifies RT agent can be deployed immediately.
func (s *DeploymentServiceSuite) TestCreateAndDeployAgent_RTDeployment_DeploysSuccessfully() {
	// Create RT agent
	createRequest := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "RT Agent for Deployment Test",
				Prompt:    "Test prompt for deployment",
				Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
	}

	agentID := s.createAgentViaAPI(createRequest)
	s.True(strings.HasSuffix(agentID, "_rt"), "Agent ID should have '_rt' suffix")

	// Deploy the agent
	s.deployAgentViaAPI(agentID)

	// Verify deployment status is Success (RT agents deploy immediately)
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS, agent.GetDeploymentStatus(),
		"RT agent should have SUCCESS deployment status immediately after deploy")
}

// TestCreateAndDeployAgent_LGDeployment_SetsInProgressStatus verifies LG agent deployment status.
func (s *DeploymentServiceSuite) TestCreateAndDeployAgent_LGDeployment_SetsInProgressStatus() {
	// Create LG agent
	createRequest := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "LG Agent for Deployment Test",
				Prompt:    "Test prompt for deployment",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
	}

	agentID := s.createAgentViaAPI(createRequest)
	s.True(strings.HasSuffix(agentID, "_lg"), "Agent ID should have '_lg' suffix")

	// Deploy the agent
	s.deployAgentViaAPI(agentID)

	// Verify deployment status is InProgress (LG agents require async deployment)
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_IN_PROGRESS, agent.GetDeploymentStatus(),
		"LG agent should have IN_PROGRESS deployment status after deploy")
}
