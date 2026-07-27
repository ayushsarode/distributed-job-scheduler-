package deploymentservice

import (
	"strings"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

// Verifies that GetAgent returns all expected fields for a newly created template agent.
func (s *DeploymentServiceSuite) TestGetAgent_Success() {
	// Create an agent first
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_template.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Get the agent
	agent := s.getAgentViaAPI(agentID)

	s.Equal(agentID, agent.GetAgentId(), "Agent ID should match")
	s.Equal("test-template-agent", agent.GetAgentName(), "Agent name should match")
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED, agent.GetDeploymentStatus(), "Newly created agent should not be deployed")
	s.NotNil(agent.GetCreatedAt(), "Created timestamp should be set")
	s.NotNil(agent.GetUpdatedAt(), "Updated timestamp should be set")
	s.NotEmpty(agent.GetCreatedBy(), "Created by should be set")

	// Verify the agent_info matches the original create request
	var want pb.GetAgentResponse
	s.LoadTestData(serviceName, "response/get_agent_template_not_deployed.json", &want)

	diff := cmp.Diff(&want, agent,
		protocmp.Transform(),
		protocmp.IgnoreFields(&pb.GetAgentResponse{}, "agent_id", "agent_info", "created_at", "updated_at"),
	)
	s.Empty(diff, "Response fields should match expected, diff: %s", diff)

	// Verify agent_info is present and contains the template request
	s.NotNil(agent.GetAgentInfo(), "Agent info should be present")
	s.NotNil(agent.GetAgentInfo().GetAgentTemplateRequest(), "Template request should be present in agent info")
}

// Verifies that GetAgent returns CodeInvalidArgument when agent_id is empty.
func (s *DeploymentServiceSuite) TestGetAgent_EmptyAgentID() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetAgentRequest{AgentId: ""})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.GetAgent(ctx, req)
	s.Require().Error(err, "GetAgent should fail for empty agent_id")
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

// Verifies that GetAgent returns CodeNotFound for a valid ULID that doesn't exist in the database.
func (s *DeploymentServiceSuite) TestGetAgent_NonExistentAgent() {
	nonExistentID := "01ZZZZZZZZZZZZZZZZZZZZZZZ9_ws"

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetAgentRequest{AgentId: nonExistentID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.GetAgent(ctx, req)
	s.Require().Error(err, "GetAgent should fail for non-existent agent")
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

// TestGetAgent_RTSuffixedID_ReturnsCorrectAgent verifies GetAgent works with _rt suffixed ID.
func (s *DeploymentServiceSuite) TestGetAgent_RTSuffixedID_ReturnsCorrectAgent() {
	// Create an RT agent
	createReq := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "RT Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
	}
	agentID := s.createAgentViaAPI(createReq)

	// Verify it's an RT agent
	s.True(strings.HasSuffix(agentID, "_rt"), "Agent ID should have _rt suffix")

	// Get the agent
	agent := s.getAgentViaAPI(agentID)

	s.Equal(agentID, agent.GetAgentId(), "Agent ID should match")
	s.Equal("RT Test Agent", agent.GetAgentName(), "Agent name should match")
	s.NotNil(agent.GetAgentInfo(), "Agent info should be present")
}

// TestGetAgent_WSSuffixedID_ReturnsCorrectAgent verifies GetAgent works with _ws suffixed ID.
func (s *DeploymentServiceSuite) TestGetAgent_WSSuffixedID_ReturnsCorrectAgent() {
	// Create a WS agent
	createReq := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "WS Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
	}
	agentID := s.createAgentViaAPI(createReq)

	// Verify it's a WS agent
	s.True(strings.HasSuffix(agentID, "_ws"), "Agent ID should have _ws suffix")

	// Get the agent
	agent := s.getAgentViaAPI(agentID)

	s.Equal(agentID, agent.GetAgentId(), "Agent ID should match")
	s.Equal("WS Test Agent", agent.GetAgentName(), "Agent name should match")
	s.NotNil(agent.GetAgentInfo(), "Agent info should be present")
}

// TestGetAgent_LGSuffixedID_ReturnsCorrectAgent verifies GetAgent works with _lg suffixed ID.
func (s *DeploymentServiceSuite) TestGetAgent_LGSuffixedID_ReturnsCorrectAgent() {
	// Create an LG agent
	createReq := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "LG Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
	}
	agentID := s.createAgentViaAPI(createReq)

	// Verify it's an LG agent
	s.True(strings.HasSuffix(agentID, "_lg"), "Agent ID should have _lg suffix")

	// Get the agent
	agent := s.getAgentViaAPI(agentID)

	s.Equal(agentID, agent.GetAgentId(), "Agent ID should match")
	s.Equal("LG Test Agent", agent.GetAgentName(), "Agent name should match")
	s.NotNil(agent.GetAgentInfo(), "Agent info should be present")
}
