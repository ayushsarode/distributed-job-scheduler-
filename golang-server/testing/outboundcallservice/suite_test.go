package outboundcallservice

import (
	"context"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"exiro.ai/application/models/pb"
	"exiro.ai/application/models/pb/pbconnect"
	"exiro.ai/testing/base"
)

const serviceName = "outboundcallservice"

type OutboundCallServiceSuite struct {
	base.IntegrationSuite

	outboundClient pbconnect.OutboundCallDocumentServiceClient
	auditClient    pbconnect.AuditServiceClient
	workflowClient pbconnect.WorkflowServiceClient
	agentClient    pbconnect.DeployServiceClient
}

func TestOutboundCallServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(OutboundCallServiceSuite))
}

func (s *OutboundCallServiceSuite) SetupSuite() {
	s.IntegrationSuite.SetupSuite()

	authInterceptor := connect.UnaryInterceptorFunc(func(uf connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			req.Header().Set("Authorization", "Bearer test-token")
			return uf(ctx, req)
		}
	})

	s.outboundClient = pbconnect.NewOutboundCallDocumentServiceClient(
		s.HTTPClient,
		s.ServerURL,
		connect.WithInterceptors(authInterceptor),
	)

	s.workflowClient = pbconnect.NewWorkflowServiceClient(
		s.HTTPClient,
		s.ServerURL,
		connect.WithInterceptors(authInterceptor),
	)

	s.agentClient = pbconnect.NewDeployServiceClient(
		s.HTTPClient,
		s.ServerURL,
		connect.WithInterceptors(authInterceptor),
	)

	s.auditClient = pbconnect.NewAuditServiceClient(
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

// getLatestAuditLog returns the first matching audit log or nil.
func (s *OutboundCallServiceSuite) getLatestAuditLog(ctx context.Context, action, resourceType, resourceID string) *pb.AuditLogEntry {
	s.T().Helper()
	req := connect.NewRequest(&pb.ListAuditLogsRequest{
		Limit:        50,
		ResourceType: resourceType,
		UserId:       s.TestUserID,
	})
	req.Header().Set("Authorization", "Bearer test-token")
	resp, err := s.auditClient.ListAuditLogs(ctx, req)
	s.Require().NoError(err, "ListAuditLogs should succeed")

	for _, log := range resp.Msg.GetAuditLogs() {
		if log.GetAction() == action && log.GetResourceId() == resourceID {
			return log
		}
	}
	return nil
}

// createCallJobViaAPI is a test helper that creates a call job and returns the job ID.
func (s *OutboundCallServiceSuite) createCallJobViaAPI(request *pb.CreateCallJobRequest) string {
	s.T().Helper()

	jobID, _ := s.createCallJobWithAgentViaAPI(request)
	return jobID
}

// createCallJobWithAgentViaAPI creates a call job and returns both job ID and agent ID.
// Use this when you need to reuse the same agent for subsequent operations.
func (s *OutboundCallServiceSuite) createCallJobWithAgentViaAPI(request *pb.CreateCallJobRequest) (string, string) {
	s.T().Helper()

	ctx := s.T().Context()

	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	request.WorkflowId = s.createTestWorkflow(agentID)

	req := connect.NewRequest(request)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.CreateCallJob(ctx, req)
	s.Require().NoError(err, "CreateCallJob should succeed")
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Msg.GetId(), "CreateCallJob response id should not be empty")

	return resp.Msg.GetId(), agentID
}

// getCallJobViaAPI is a test helper that retrieves a call job by ID.
func (s *OutboundCallServiceSuite) getCallJobViaAPI(jobID string) *pb.GetCallJobResponse {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetCallJobRequest{Id: jobID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.GetCallJob(ctx, req)
	s.Require().NoError(err, "GetCallJob should succeed")
	s.Require().NotNil(resp)

	return resp.Msg
}

// insertCallJobWithStatus inserts a call job directly into the database with a specific status.
// This is used for testing status filters where the status cannot be achieved via API in integration tests.
func (s *OutboundCallServiceSuite) insertCallJobWithStatus(ctx context.Context, status string, name string) string {
	s.T().Helper()

	jobID := uuid.Must(uuid.NewV7())
	documentID := uuid.Must(uuid.NewV7()).String()

	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	workflowID := s.createTestWorkflow(agentID)

	_, err := s.DBPool.Exec(ctx, `
		INSERT INTO call_jobs (
			id, name, document_id, document_type, workflow_id,
			preffered_language, max_retries, retry_delay, outbound_call_provider_id,
			is_materialised, created_by, tenant_id, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, jobID, name, documentID, "csv", workflowID, "en", 3, 60, nil, false, s.TestUserID, s.TestTenantID, status)
	s.Require().NoError(err, "Failed to insert call job with status")

	return jobID.String()
}

// insertJobItems inserts job items directly into the database.
// This is an exception to the normal practice of using API calls to set up test data.
// Job items cannot be created via API, they are created internally by the system.
func (s *OutboundCallServiceSuite) insertJobItems(ctx context.Context, jobID string, count int) []string {
	s.T().Helper()

	ids := make([]string, count)
	for i := range count {
		itemID := uuid.Must(uuid.NewV7())
		phoneNo := fmt.Sprintf("+1234567890%d", i)
		_, err := s.DBPool.Exec(ctx, `
			INSERT INTO job_item (
				id, phone_no, agent_context, call_status, job_id, job_data, created_by, tenant_id
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, itemID, phoneNo, "", "pending", jobID, "", s.TestUserID, s.TestTenantID)
		s.Require().NoError(err, "Failed to insert job item")

		ids[i] = itemID.String()
	}
	return ids
}

// createTestWorkflow creates a published workflow using API calls.
// Requires agentID to be a real deployed agent.
func (s *OutboundCallServiceSuite) createTestWorkflow(agentID string) string {
	s.T().Helper()
	s.Require().NotEmpty(agentID, "agentID is required for createTestWorkflow")

	workflowID := s.createDraftWorkflowViaAPI(agentID)
	s.publishWorkflowViaAPI(workflowID)
	return workflowID
}

// insertCallJobWithReadyStatus inserts a call job directly into the database with READY status.
// This is used for testing AddJobItems and RemoveJobItems which require jobs to be in READY state.
func (s *OutboundCallServiceSuite) insertCallJobWithReadyStatus(ctx context.Context, name string) string {
	s.T().Helper()

	jobID := uuid.Must(uuid.NewV7())
	documentID := uuid.Must(uuid.NewV7()).String()

	agentID := s.createAgentViaAPI()
	s.deployAgentViaAPI(agentID)
	workflowID := s.createTestWorkflow(agentID)

	_, err := s.DBPool.Exec(ctx, `
		INSERT INTO call_jobs (
			id, name, document_id, document_type, workflow_id,
			preffered_language, max_retries, retry_delay, outbound_call_provider_id,
			is_materialised, created_by, tenant_id, status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, jobID, name, documentID, "csv", workflowID, "en", 3, 60, nil, true, s.TestUserID, s.TestTenantID, "ready")
	s.Require().NoError(err, "Failed to insert call job with ready status")

	return jobID.String()
}

// ============================================================================
// API-Based Helper Functions
// ============================================================================

// createAgentViaAPI creates a realtime agent and returns the agent ID.
func (s *OutboundCallServiceSuite) createAgentViaAPI() string {
	s.T().Helper()
	ctx := s.T().Context()

	var agentReq pb.CreateAgentRequest
	s.LoadTestData("deploymentservice", "request/create_agent_prompt_realtime.json", &agentReq)

	req := connect.NewRequest(&agentReq)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.agentClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed")
	return resp.Msg.GetAgentId()
}

// deployAgentViaAPI deploys an agent.
func (s *OutboundCallServiceSuite) deployAgentViaAPI(agentID string) {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.DeployAgentRequest{AgentId: agentID})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.agentClient.DeployAgent(ctx, req)
	s.Require().NoError(err, "DeployAgent should succeed")
}

// createDraftWorkflowViaAPI creates a workflow in draft state. Returns workflow ID.
func (s *OutboundCallServiceSuite) createDraftWorkflowViaAPI(agentID string) string {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.CreateWorkflowRequest{
		Name:        "Test Workflow " + uuid.NewString(),
		Description: "Test workflow for integration tests",
		AgentId:     agentID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.CreateWorkflow(ctx, req)
	s.Require().NoError(err, "CreateWorkflow should succeed")
	return resp.Msg.GetWorkflowId()
}

// publishWorkflowViaAPI publishes a workflow.
func (s *OutboundCallServiceSuite) publishWorkflowViaAPI(workflowID string) {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.PublishWorkflowRequest{WorkflowId: workflowID})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.workflowClient.PublishWorkflow(ctx, req)
	s.Require().NoError(err, "PublishWorkflow should succeed")
}

// uploadCSVDocument uploads a CSV document. Returns document ID.
func (s *OutboundCallServiceSuite) uploadCSVDocument(csvContent []byte) string {
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

// updateCallJobWorkflowViaAPI updates the workflow_id of a job.
func (s *OutboundCallServiceSuite) updateCallJobWorkflowViaAPI(jobID string, workflowID string) error {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.UpdateCallJobDetailsRequest{
		JobId:      jobID,
		WorkflowId: workflowID,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.UpdateCallJobDetails(ctx, req)
	return err
}

// materializeCallJobViaAPI calls MaterializeJob API. Returns error.
func (s *OutboundCallServiceSuite) materializeCallJobViaAPI(jobID string) error {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.MaterializeJobRequest{JobId: jobID})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.MaterializeJob(ctx, req)
	return err
}

// listJobItemsViaAPI returns job items using ListJobItems API.
func (s *OutboundCallServiceSuite) listJobItemsViaAPI(jobID string) *pb.ListJobItemsResponse {
	s.T().Helper()
	ctx := s.T().Context()

	req := connect.NewRequest(&pb.ListJobItemsRequest{JobId: jobID, Limit: 1000, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "ListJobItems should succeed")
	return resp.Msg
}
