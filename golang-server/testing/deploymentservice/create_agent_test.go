package deploymentservice

import (
	"strings"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/go-cmp/cmp"
	"github.com/oklog/ulid/v2"
)

// Verifies that creating an agent with a valid template request returns a valid prefixed agent ID.
func (s *DeploymentServiceSuite) TestCreateAgent_TemplateRequest_Success() {
	var reqData pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_template.json", &reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed for a valid template request")
	s.Require().NotNil(resp)

	agentID := resp.Msg.GetAgentId()
	s.NotEmpty(agentID, "Agent ID should not be empty")

	hasValidSuffix := strings.HasSuffix(agentID, "_ws") ||
		strings.HasSuffix(agentID, "_rt") ||
		strings.HasSuffix(agentID, "_lg")
	s.True(hasValidSuffix, "Agent ID should have a valid suffix (_ws, _rt, or _lg)")

	var ulidPart string
	switch {
	case strings.HasSuffix(agentID, "_ws"):
		ulidPart, _ = strings.CutSuffix(agentID, "_ws")
	case strings.HasSuffix(agentID, "_rt"):
		ulidPart, _ = strings.CutSuffix(agentID, "_rt")
	case strings.HasSuffix(agentID, "_lg"):
		ulidPart, _ = strings.CutSuffix(agentID, "_lg")
	}
	_, err = ulid.Parse(ulidPart)
	s.NoError(err, "Part before suffix should be a valid ULID")
}

// Verifies that creating an agent with a valid prompt request returns a valid ULID agent ID with suffix.
func (s *DeploymentServiceSuite) TestCreateAgent_PromptRequest_Success() {
	var reqData pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed for a valid prompt request")
	s.Require().NotNil(resp)

	agentID := resp.Msg.GetAgentId()
	s.NotEmpty(agentID, "Agent ID should not be empty")

	hasValidSuffix := strings.HasSuffix(agentID, "_ws") ||
		strings.HasSuffix(agentID, "_rt") ||
		strings.HasSuffix(agentID, "_lg")
	s.True(hasValidSuffix, "Agent ID should have a valid suffix (_ws, _rt, or _lg)")

	var ulidPart string
	switch {
	case strings.HasSuffix(agentID, "_ws"):
		ulidPart, _ = strings.CutSuffix(agentID, "_ws")
	case strings.HasSuffix(agentID, "_rt"):
		ulidPart, _ = strings.CutSuffix(agentID, "_rt")
	case strings.HasSuffix(agentID, "_lg"):
		ulidPart, _ = strings.CutSuffix(agentID, "_lg")
	}
	_, err = ulid.Parse(ulidPart)
	s.NoError(err, "Part before suffix should be a valid ULID")
}

// Verifies that creating an agent with an empty request (no oneof variant set) returns CodeInvalidArgument.
func (s *DeploymentServiceSuite) TestCreateAgent_EmptyRequest() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.CreateAgentRequest{})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail for an empty request")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	wantMsg := "bad request: create_agent_request is required"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}

// TestCreateAgent_RTDeployment_GeneratesRTPrefixID verifies creating agent with RT deployment type returns rt_ prefixed ID.
func (s *DeploymentServiceSuite) TestCreateAgent_RTDeployment_GeneratesRTPrefixID() {
	req := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "RT Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
	}

	agentID := s.createAgentViaAPI(req)

	s.True(strings.HasSuffix(agentID, "_rt"), "Agent ID should end with '_rt' suffix for RT deployment")

	// Verify the part before suffix is a valid ULID
	ulidPart := strings.TrimSuffix(agentID, "_rt")
	_, err := ulid.Parse(ulidPart)
	s.NoError(err, "Part before suffix should be a valid ULID")
}

// TestCreateAgent_WSDeployment_GeneratesWSSuffixID verifies creating agent with WS deployment type returns _ws suffixed ID.
func (s *DeploymentServiceSuite) TestCreateAgent_WSDeployment_GeneratesWSPrefixID() {
	req := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "WS Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
	}

	agentID := s.createAgentViaAPI(req)

	s.True(strings.HasSuffix(agentID, "_ws"), "Agent ID should end with '_ws' suffix for WS deployment")

	// Verify the part before suffix is a valid ULID
	ulidPart := strings.TrimSuffix(agentID, "_ws")
	_, err := ulid.Parse(ulidPart)
	s.NoError(err, "Part before suffix should be a valid ULID")
}

// TestCreateAgent_LGDeployment_GeneratesLGSuffixID verifies creating agent with LG deployment type returns _lg suffixed ID.
func (s *DeploymentServiceSuite) TestCreateAgent_LGDeployment_GeneratesLGPrefixID() {
	req := &pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "LG Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
	}

	agentID := s.createAgentViaAPI(req)

	s.True(strings.HasSuffix(agentID, "_lg"), "Agent ID should end with '_lg' suffix for LG deployment")

	// Verify the part before suffix is a valid ULID
	ulidPart := strings.TrimSuffix(agentID, "_lg")
	_, err := ulid.Parse(ulidPart)
	s.NoError(err, "Part before suffix should be a valid ULID")
}

// TestCreateAgent_RTDeploymentWithNonRealtimeModel_ReturnsError verifies RT deployment with GPT_4OMINI model returns InvalidArgument error.
func (s *DeploymentServiceSuite) TestCreateAgent_RTDeploymentWithNonRealtimeModel_ReturnsError() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "RT Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI, // Wrong model for RT
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail for RT deployment with non-realtime model")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "RT deployment type requires GPT_REALTIME model", "Error message should indicate model mismatch")
}

// TestCreateAgent_NoDeploymentType_ReturnsError verifies omitting deployment_type returns error.
func (s *DeploymentServiceSuite) TestCreateAgent_NoDeploymentType_ReturnsError() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		// deployment_type is not set (defaults to UNKNOWN)
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail when deployment_type is not specified")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)
}

// TestCreateAgent_UnknownDeploymentType_ReturnsError verifies UNKNOWN deployment type returns error.
func (s *DeploymentServiceSuite) TestCreateAgent_RejectUnknownDeploymentType() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_GPT_4OMINI,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_UNKNOWN,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail for UNKNOWN deployment type")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "deployment_type must be explicitly specified", "Error message should indicate deployment type must be specified")
}

// TestCreateAgent_WSDeployment_UnspecifiedModel_ReturnsError verifies that creating a WS agent via AgentPromptRequest without specifying a model returns CodeInvalidArgument.
func (s *DeploymentServiceSuite) TestCreateAgent_WSDeployment_UnspecifiedModel_ReturnsError() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "WS Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_UNSPECIFIED,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_WS,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail when model is unspecified")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "model must be explicitly specified", "Error message should indicate model must be specified")
}

// TestCreateAgent_LGDeployment_UnspecifiedModel_ReturnsError verifies that creating an LG agent via AgentPromptRequest without specifying a model returns CodeInvalidArgument.
func (s *DeploymentServiceSuite) TestCreateAgent_LGDeployment_UnspecifiedModel_ReturnsError() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "LG Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_UNSPECIFIED,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_LG,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail when model is unspecified")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "model must be explicitly specified", "Error message should indicate model must be specified")
}

// TestCreateAgent_RTDeployment_UnspecifiedModel_ReturnsError verifies that creating an RT agent via AgentPromptRequest without specifying a model returns CodeInvalidArgument.
func (s *DeploymentServiceSuite) TestCreateAgent_RTDeployment_UnspecifiedModel_ReturnsError() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.CreateAgentRequest{
		CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
			AgentPromptRequest: &pb.AgentPromptRequest{
				AgentName: "RT Test Agent",
				Prompt:    "You are a helpful assistant",
				Model:     pb.AgentModel_AGENTMODEL_UNSPECIFIED,
			},
		},
		DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().Error(err, "CreateAgent should fail when model is unspecified")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should be InvalidArgument, diff: %s", diff)

	s.Contains(connectErr.Message(), "model must be explicitly specified", "Error message should indicate model must be specified")
}
