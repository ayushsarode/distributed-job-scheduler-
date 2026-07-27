package deploymentservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
)

// --- Validation Tests ---

// Verifies that UpdateAgent returns CodeInvalidArgument when agent_id is empty.
func (s *DeploymentServiceSuite) TestUpdateAgent_EmptyAgentID() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{AgentId: ""})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail for empty agent_id")
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

func (s *DeploymentServiceSuite) TestUpdateAgent_NonExistentAgent() {
	nonExistentID := uuid.Must(uuid.NewV7()).String()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   nonExistentID,
		AgentName: "new-name",
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail for non-existent agent")
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

// --- Basic Success ---

// Verifies that updating the name of a non-deployed agent succeeds and is in NotDeployed status.
func (s *DeploymentServiceSuite) TestUpdateAgent_NameOnNonDeployedAgent() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Update the agent name
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "renamed-agent",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "renamed-agent",
					Prompt:    "You are a helpful assistant that answers questions.",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().NoError(err, "UpdateAgent should succeed for a non-deployed agent")

	// Verify the update
	agent := s.getAgentViaAPI(agentID)

	s.Equal("renamed-agent", agent.GetAgentName(), "Agent name should be updated")
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED, agent.GetDeploymentStatus(), "Non-deployed agent should remain not deployed after update")
}

// --- Deployment State Edge Cases ---

// Verifies that updating an agent while deployment is in progress is rejected.
func (s *DeploymentServiceSuite) TestUpdateAgent_RejectWhileDeploymentInProgress() {
	// Create and deploy an LG agent (deployment goes to external service via SQS, status=InProgress)
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_lg.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)
	s.deployAgentViaAPI(agentID)

	// Verify it's not yet deployed (InProgress)
	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_IN_PROGRESS, agent.GetDeploymentStatus(), "Agent should be in progress, not yet deployed")

	// Attempt to update while deployment is in progress -- should be rejected
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "should-not-update",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "should-not-update",
					Prompt:    "New prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should be rejected while deployment is in progress")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeFailedPrecondition
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be FailedPrecondition, diff: %s", diff)

	// Verify the agent config was NOT changed
	agentAfter := s.getAgentViaAPI(agentID)
	s.Equal("test-lg-agent", agentAfter.GetAgentName(), "Agent name should not have changed")
}

// Verifies that updating a deployed realtime agent's prompt resets deployment status to NotDeployed.
func (s *DeploymentServiceSuite) TestUpdateAgent_DeployedRealtimeToNonRealtime_ResetsDeployment() {
	// Create and deploy a realtime agent (deployed immediately)
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)
	s.deployAgentViaAPI(agentID)

	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS, agent.GetDeploymentStatus(), "Realtime agent should be deployed")

	// Update the prompt (RT deployment must keep REALTIME model)
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "updated-rt-agent",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "updated-rt-agent",
					Prompt:    "Updated prompt for realtime agent",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().NoError(err, "UpdateAgent should succeed for a deployed agent")

	// Deployment status should be reset to NotDeployed (requires re-deployment)
	agentAfter := s.getAgentViaAPI(agentID)

	s.Equal("updated-rt-agent", agentAfter.GetAgentName(), "Agent name should be updated")
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED, agentAfter.GetDeploymentStatus(), "Changing config should reset deployment status to NotDeployed")
}

// Verifies that a WS agent accepting GPT_REALTIME model is rejected.
func (s *DeploymentServiceSuite) TestUpdateAgent_WSAgentWithRealtimeModel_ShouldReject() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_4omini.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "ws-with-realtime",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "ws-with-realtime",
					Prompt:    "Some prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail when using REALTIME model with WS deployment type")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	wantMsg := "bad request: GPT_REALTIME model requires RT deployment type"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// Verifies that updating only the name of a deployed agent resets it's status to NotDeployed.
func (s *DeploymentServiceSuite) TestUpdateAgent_DeployedAgent_NameOnlyChange_ResetsDeployment() {
	// Create and deploy a realtime agent (deployed immediately)
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)
	s.deployAgentViaAPI(agentID)

	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS, agent.GetDeploymentStatus(), "Realtime agent should be deployed")

	// Update only the name, keeping the same model
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "just-renamed",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "just-renamed",
					Prompt:    "You are a helpful assistant that answers questions.",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().NoError(err, "UpdateAgent should succeed")

	// Even for a name-only change, deployment status should be reset (config may have changed)
	agentAfter := s.getAgentViaAPI(agentID)

	s.Equal("just-renamed", agentAfter.GetAgentName(), "Agent name should be updated")
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED, agentAfter.GetDeploymentStatus(), "Any config change on a deployed agent should reset deployment status")
}

// Verifies that updating a non-deployed agent succeeds without affecting status.
func (s *DeploymentServiceSuite) TestUpdateAgent_NonDeployedAgent_FreelyUpdated() {
	// Create a realtime agent (do NOT deploy it)
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	agent := s.getAgentViaAPI(agentID)
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED, agent.GetDeploymentStatus(), "Agent should not be deployed")

	// Update the agent prompt
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "freely-updated",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "freely-updated",
					Prompt:    "Changed prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().NoError(err, "UpdateAgent should succeed for a non-deployed agent")

	// Verify update applied and status stays NotDeployed
	agentAfter := s.getAgentViaAPI(agentID)

	s.Equal("freely-updated", agentAfter.GetAgentName(), "Agent name should be updated")
	s.Equal(pb.DeploymentStatus_DEPLOYMENT_STATUS_NOT_DEPLOYED, agentAfter.GetDeploymentStatus(), "Non-deployed agent should remain not deployed")
}

// --- Prefix Compatibility Validation Tests ---

// Verifies that changing an LG agent's deployment type to WS is rejected with an appropriate error.
func (s *DeploymentServiceSuite) TestUpdateAgent_RejectCrossTypeUpdate_LGtoWS() {
	// Create an LG agent
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_lg.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Attempt to update to WS deployment type (would change _lg suffix to _ws)
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "switched-to-ws",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "switched-to-ws",
					Prompt:    "New prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail when changing deployment type from LG to WS")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	wantMsg := "bad request: cannot change deployment type or model in a way that changes agent suffix"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// Verifies that changing a WS agent to RT deployment type is rejected.
func (s *DeploymentServiceSuite) TestUpdateAgent_RejectCrossTypeUpdate_WStoRT() {
	// Create a WS agent with 4o-mini model
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_4omini.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Attempt to update to RT deployment type (would change _ws suffix to _rt)
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "switched-to-rt",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "switched-to-rt",
					Prompt:    "New prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail when changing deployment type from WS to RT")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	wantMsg := "bad request: cannot change deployment type or model in a way that changes agent suffix"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// Verifies that updating an RT agent with a non-REALTIME model is rejected.
func (s *DeploymentServiceSuite) TestUpdateAgent_RejectRTAgentWithWrongModel() {
	// Create an RT agent with REALTIME model
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Attempt to update to 4o-mini model (invalid for RT deployment)
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "rt-with-wrong-model",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "rt-with-wrong-model",
					Prompt:    "New prompt",
					Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail when using non-REALTIME model with RT deployment type")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	wantMsg := "bad request: RT deployment type requires GPT_REALTIME model"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// TestUpdateAgent_WSAgent_UnspecifiedModel_ReturnsError verifies that updating a WS agent with an unspecified model returns CodeInvalidArgument.
func (s *DeploymentServiceSuite) TestUpdateAgent_WSAgent_UnspecifiedModel_ReturnsError() {
	// Create a WS agent with a valid model
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_4omini.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Attempt to update with unspecified model
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "ws-with-unspecified-model",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "ws-with-unspecified-model",
					Prompt:    "New prompt",
					Model:     pb.AgentModel_AGENTMODEL_UNSPECIFIED,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail when model is unspecified")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "model must be explicitly specified", "Error message should indicate model must be specified")
}

// TestUpdateAgent_LGAgent_UnspecifiedModel_ReturnsError verifies that updating an LG agent with an unspecified model returns CodeInvalidArgument.
func (s *DeploymentServiceSuite) TestUpdateAgent_LGAgent_UnspecifiedModel_ReturnsError() {
	// Create an LG agent with a valid model
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_lg.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Attempt to update with unspecified model
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "lg-with-unspecified-model",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "lg-with-unspecified-model",
					Prompt:    "New prompt",
					Model:     pb.AgentModel_AGENTMODEL_UNSPECIFIED,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail when model is unspecified")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "model must be explicitly specified", "Error message should indicate model must be specified")
}

// TestUpdateAgent_RTAgent_UnspecifiedModel_ReturnsError verifies that updating an RT agent with an unspecified model returns CodeInvalidArgument.
func (s *DeploymentServiceSuite) TestUpdateAgent_RTAgent_UnspecifiedModel_ReturnsError() {
	// Create an RT agent with a valid model
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	agentID := s.createAgentViaAPI(&createReq)

	// Attempt to update with unspecified model
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "rt-with-unspecified-model",
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "rt-with-unspecified-model",
					Prompt:    "New prompt",
					Model:     pb.AgentModel_AGENTMODEL_UNSPECIFIED,
				},
			},
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.UpdateAgent(ctx, req)
	s.Require().Error(err, "UpdateAgent should fail when model is unspecified")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "model must be explicitly specified", "Error message should indicate model must be specified")
}
