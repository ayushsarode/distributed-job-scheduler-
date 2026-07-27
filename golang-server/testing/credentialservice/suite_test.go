package credentialservice

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/models/pb/pbconnect"
	"exiro.ai/testing/base"
	"github.com/stretchr/testify/suite"
)

const serviceName = "credentialservice"

type CredentialServiceSuite struct {
	base.IntegrationSuite

	credentialClient pbconnect.CredentialServiceClient
	auditClient      pbconnect.AuditServiceClient
}

func TestCredentialServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(CredentialServiceSuite))
}

func (s *CredentialServiceSuite) SetupSuite() {
	s.IntegrationSuite.SetupSuite()

	s.credentialClient = pbconnect.NewCredentialServiceClient(
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
func (s *CredentialServiceSuite) getLatestAuditLog(ctx context.Context, action, resourceType, resourceID string) *pb.AuditLogEntry {
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

// clearCredentials deletes all credentials for the test tenant.
func (s *CredentialServiceSuite) clearCredentials(ctx context.Context) {
	s.T().Helper()
	_, err := s.DBPool.Exec(ctx, "DELETE FROM credentials WHERE tenant_id = $1", s.TestTenantID)
	s.Require().NoError(err, "Failed to clear credentials")
}

