package deploymentservice

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/models/pb/pbconnect"
	agentServicePb "exiro.ai/application/models/protos"
	"exiro.ai/testing/base"
	"exiro.ai/testing/testutil"
)

const serviceName = "deploymentservice"

type DeploymentServiceSuite struct {
	base.IntegrationSuite

	deployClient pbconnect.DeployServiceClient
	auditClient  pbconnect.AuditServiceClient
}

func TestDeploymentServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(DeploymentServiceSuite))
}

func (s *DeploymentServiceSuite) SetupSuite() {
	s.IntegrationSuite.SetupSuite()

	s.deployClient = pbconnect.NewDeployServiceClient(
		s.HTTPClient,
		s.ServerURL,
		connect.WithInterceptors(connect.UnaryInterceptorFunc(func(uf connect.UnaryFunc) connect.UnaryFunc {
			return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
				req.Header().Set("Authorization", "Bearer test-token")
				return uf(ctx, req)
			}
		})),
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
func (s *DeploymentServiceSuite) getLatestAuditLog(ctx context.Context, action, resourceType, resourceID string) *pb.AuditLogEntry {
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

// createAgentViaAPI is a test helper that creates an agent and returns the agent ID.
func (s *DeploymentServiceSuite) createAgentViaAPI(request *pb.CreateAgentRequest) string {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(request)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.CreateAgent(ctx, req)
	s.Require().NoError(err, "CreateAgent should succeed")
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Msg.GetAgentId(), "Agent ID should not be empty")

	return resp.Msg.GetAgentId()
}

// getAgentViaAPI is a test helper that retrieves an agent by ID.
func (s *DeploymentServiceSuite) getAgentViaAPI(agentID string) *pb.GetAgentResponse {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.GetAgentRequest{AgentId: agentID})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.deployClient.GetAgent(ctx, req)
	s.Require().NoError(err, "GetAgent should succeed")
	s.Require().NotNil(resp)

	return resp.Msg
}

// deployAgentViaAPI is a test helper that deploys an agent by ID.
func (s *DeploymentServiceSuite) deployAgentViaAPI(agentID string) {
	s.T().Helper()

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.DeployAgentRequest{AgentId: agentID})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.deployClient.DeployAgent(ctx, req)
	s.Require().NoError(err, "DeployAgent should succeed")
}

// sendDeploymentStatusMessage sends an AgentDeploymentUpdate message to the deployment
// status SQS queue on LocalStack, simulating the external deployment service callback.
func (s *DeploymentServiceSuite) sendDeploymentStatusMessage(ctx context.Context, queueName string, agentID string, deployed bool) error {
	s.T().Helper()

	msg := &agentServicePb.AgentDeploymentUpdate{
		Agentid:  agentID,
		Deployed: deployed,
	}

	payload, err := protojson.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal AgentDeploymentUpdate: %w", err)
	}

	return testutil.SendSQSMessage(ctx, s.LocalstackContainer.Endpoint, queueName, string(payload))
}

// insertAgentWithStatus inserts an agent directly into the database with a specific deployment status.
// This is used for testing status filters where the status cannot be achieved via API in integration tests.
func (s *DeploymentServiceSuite) insertAgentWithStatus(ctx context.Context, status string, name string) string {
	s.T().Helper()

	agentID := uuid.Must(uuid.NewV7())
	now := time.Now()

	_, err := s.DBPool.Exec(ctx, `
		INSERT INTO agent (id, create_agent_request_json, name, created_at, updated_at, created_by, tenant_id, deployment_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, agentID, "{}", name, now, now, s.TestUserID, s.TestTenantID, status)
	s.Require().NoError(err, "Failed to insert agent with status")

	return agentID.String()
}
