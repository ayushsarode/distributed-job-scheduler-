package workflowservice

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/models/pb/pbconnect"
	"exiro.ai/testing/base"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
)

const serviceName = "workflowservice"

// Sample CSV content for testing call jobs.
var testCSVContent = []byte(`phone_number,agent_context
+12345678901,Test context 1
+12345678902,Test context 2
`)

type WorkflowServiceSuite struct {
	base.IntegrationSuite

	workflowClient pbconnect.WorkflowServiceClient
	agentClient    pbconnect.DeployServiceClient
	outboundClient pbconnect.OutboundCallDocumentServiceClient
}

func TestWorkflowServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(WorkflowServiceSuite))
}

func (s *WorkflowServiceSuite) SetupSuite() {
	s.IntegrationSuite.SetupSuite()

	s.workflowClient = pbconnect.NewWorkflowServiceClient(
		s.HTTPClient,
		s.ServerURL,
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(uf connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer test-token")
				return uf(ctx, req)
			}
		})),
	)

	s.agentClient = pbconnect.NewDeployServiceClient(
		s.HTTPClient,
		s.ServerURL,
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(uf connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer test-token")
				return uf(ctx, req)
			}
		})),
	)

	s.outboundClient = pbconnect.NewOutboundCallDocumentServiceClient(
		s.HTTPClient,
		s.ServerURL,
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(uf connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer test-token")
				return uf(ctx, req)
			}
		})),
	)
}

// createWorkflowViaAPI is a test helper that creates an agent and returns the agent ID.
func (s *WorkflowServiceSuite) createWorkflowViaAPI(request *pb.CreateWorkflowRequest) string {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(request)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.CreateWorkflow(ctx, req)
	s.Require().NoError(err, "CreateWorkflow should succeed")
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Msg.GetWorkflowId(), "Workflow ID should not be empty")

	return resp.Msg.GetWorkflowId()
}

// getWorkflowViaAPI is a test helper that retrieves an agent by ID.
func (s *WorkflowServiceSuite) getWorkflowViaAPI(workflowId string) *pb.GetWorkflowResponse {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetWorkflowRequest{WorkflowId: workflowId})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.GetWorkflow(ctx, req)
	s.Require().NoError(err, "GetWorkflow should succeed")
	s.Require().NotNil(resp)

	return resp.Msg
}

// publishWorkflowViaAPI is a test helper that retrieves an agent by ID.
func (s *WorkflowServiceSuite) publishWorkflowViaAPI(workflowId string) {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.PublishWorkflowRequest{WorkflowId: workflowId})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.PublishWorkflow(ctx, req)
	s.Require().NoError(err, "GetWorkflow should succeed")
	s.Require().NotNil(resp)
}

// deactivateWorkflowViaAPI is a test helper that retrieves an agent by ID.
func (s *WorkflowServiceSuite) deactivateWorkflowViaAPI(workflowId string) {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeactivateWorkflowRequest{WorkflowId: workflowId})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.DeactivateWorkflow(ctx, req)
	s.Require().NoError(err, "GetWorkflow should succeed")
	s.Require().NotNil(resp)
}

// createAgentViaAPI is a test helper that creates an agent and returns the agent ID.
func (s *WorkflowServiceSuite) createAgentViaAPI() string {
	s.T().Helper()

	ctx := s.T().Context()

	var agentReq pb.CreateAgentRequest
	s.LoadTestData("deploymentservice", "request/create_agent_prompt_realtime.json", &agentReq)

	req := connect.NewRequest(&agentReq)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.agentClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed")
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Msg.GetAgentId(), "Agent ID should not be empty")

	return resp.Msg.GetAgentId()
}

// deployAgentViaAPI is a test helper that creates an agent and returns the agent ID.
func (s *WorkflowServiceSuite) deployAgentViaAPI(agentID string) {
	s.T().Helper()

	ctx := s.T().Context()

	req := connect.NewRequest(&pb.DeployAgentRequest{
		AgentId: agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.agentClient.DeployAgent(ctx, req)
	s.Require().NoError(err, "DeployAgent should succeed")

	agent := s.getAgentViaAPI(agentID)
	s.Equal(
		pb.DeploymentStatus_DEPLOYMENT_STATUS_SUCCESS,
		agent.GetDeploymentStatus(),
		"Agent should be deployed successfully",
	)
}

// getAgentViaAPI is a test helper that retrieves an agent by ID.
func (s *WorkflowServiceSuite) getAgentViaAPI(agentID string) *pb.GetAgentResponse {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetAgentRequest{AgentId: agentID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.agentClient.GetAgent(ctx, req)
	s.Require().NoError(err, "GetAgent should succeed")
	s.Require().NotNil(resp)

	return resp.Msg
}

// clearWorkflows deletes all workflows for the test tenant.
func (s *WorkflowServiceSuite) clearWorkflows(ctx context.Context) {
	s.T().Helper()
	_, err := s.DBPool.Exec(ctx, "DELETE FROM workflows WHERE tenant_id = $1", s.TestTenantID)
	s.Require().NoError(err, "Failed to clear workflows")
}

// ============================================================================
// Call Job Helper Functions
// ============================================================================

// uploadCSVDocument uploads a CSV document for call jobs.
func (s *WorkflowServiceSuite) uploadCSVDocument(csvContent []byte) string {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.UploadOutboundCallDocumentRequest{
		FileName:    "test_contacts.csv",
		FileType:    "text/csv",
		FileContent: csvContent,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.UploadOutboundCallDocument(ctx, req)
	s.Require().NoError(err, "UploadOutboundCallDocument should succeed")
	return resp.Msg.GetDocumentId()
}

// materializeCallJobViaAPI calls MaterializeJob API.
func (s *WorkflowServiceSuite) materializeCallJobViaAPI(jobID string) error {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.MaterializeJobRequest{JobId: jobID})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.MaterializeJob(ctx, req)
	return err
}

// getCallJobViaAPI retrieves a call job by ID.
func (s *WorkflowServiceSuite) getCallJobViaAPI(jobID string) *pb.GetCallJobResponse {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.GetCallJobRequest{Id: jobID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.GetCallJob(ctx, req)
	s.Require().NoError(err, "GetCallJob should succeed")
	return resp.Msg
}

// insertCallJobWithStatus inserts a call job with specific status (for statuses not achievable via API).
func (s *WorkflowServiceSuite) insertCallJobWithStatus(ctx context.Context, status string, workflowID string, name string) {
	s.T().Helper()
	jobID := uuid.Must(uuid.NewV7())
	documentID := uuid.Must(uuid.NewV7()).String()

	_, err := s.DBPool.Exec(ctx, `
		INSERT INTO call_jobs (
			id, name, workflow_id, document_id, document_type,
			preffered_language, max_retries, retry_delay, outbound_call_provider_id,
			is_materialised, created_by, tenant_id, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, jobID, name, workflowID, documentID, "csv", "en", 3, 60, nil, status == "ready", s.TestUserID, s.TestTenantID, status)
	s.Require().NoError(err, "Failed to insert call job with status")
}

// createTestWorkflow creates a workflow via the API and returns its ID.
func (s *WorkflowServiceSuite) createTestWorkflow(agentID, name, description string) string {
	s.T().Helper()

	var reqData pb.CreateWorkflowRequest
	s.LoadTestData(serviceName, "request/create_workflow.json", &reqData)
	reqData.AgentId = agentID
	reqData.Name = name
	reqData.Description = description

	return s.createWorkflowViaAPI(&reqData)
}
