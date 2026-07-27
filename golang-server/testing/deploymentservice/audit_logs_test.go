package deploymentservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	servicetypes "exiro.ai/application/service/types"
	"github.com/google/uuid"
)

func (s *DeploymentServiceSuite) TestCreateAgent_WritesAuditLog() {
	var reqData pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &reqData)
	reqData.GetAgentPromptRequest().AgentName = "audit-create-agent-" + uuid.NewString()

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")
	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Msg.GetAgentId())

	agentID := resp.Msg.GetAgentId()
	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionAgentCreated,
		servicetypes.AuditResourceTypeAgent,
		agentID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionAgentCreated, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeAgent, log.GetResourceType())
	s.Equal(agentID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *DeploymentServiceSuite) TestUpdateAgent_WritesAuditLog() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	createReq.GetAgentPromptRequest().AgentName = "audit-update-agent-" + uuid.NewString()
	agentID := s.createAgentViaAPI(&createReq)

	ctx := s.T().Context()
	updateReq := connect.NewRequest(&pb.UpdateAgentRequest{
		AgentId:   agentID,
		AgentName: "audit-updated-name-" + uuid.NewString(),
		CreateAgentRequest: &pb.CreateAgentRequest{
			DeploymentType: pb.DeploymentType_DEPLOYMENT_TYPE_RT,
			CreateAgentRequest: &pb.CreateAgentRequest_AgentPromptRequest{
				AgentPromptRequest: &pb.AgentPromptRequest{
					AgentName: "audit-updated-name",
					Prompt:    "Updated prompt for audit test.",
					Model:     pb.AgentModel_AGENTMODEL_GPT_REALTIME,
				},
			},
		},
	})
	updateReq.Header().Set("Authorization", "Bearer test-token")
	_, err := s.deployClient.UpdateAgent(ctx, updateReq)
	s.Require().NoError(err)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionAgentUpdated,
		servicetypes.AuditResourceTypeAgent,
		agentID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionAgentUpdated, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeAgent, log.GetResourceType())
	s.Equal(agentID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *DeploymentServiceSuite) TestDeployAgent_WritesAuditLog() {
	var createReq pb.CreateAgentRequest
	s.LoadTestData(serviceName, "request/create_agent_prompt_realtime.json", &createReq)
	createReq.GetAgentPromptRequest().AgentName = "audit-deploy-agent-" + uuid.NewString()
	agentID := s.createAgentViaAPI(&createReq)

	ctx := s.T().Context()
	s.deployAgentViaAPI(agentID)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionAgentDeployed,
		servicetypes.AuditResourceTypeAgent,
		agentID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionAgentDeployed, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeAgent, log.GetResourceType())
	s.Equal(agentID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}
