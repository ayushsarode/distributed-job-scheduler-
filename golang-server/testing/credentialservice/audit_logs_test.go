package credentialservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	servicetypes "exiro.ai/application/service/types"
	"github.com/google/uuid"
)

func (s *CredentialServiceSuite) TestSetCredential_WritesAuditLog() {
	var reqData pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &reqData)
	reqData.CredentialName = "audit-set-cred-" + uuid.NewString()

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")
	resp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Msg.GetCredentialId())

	credentialID := resp.Msg.GetCredentialId()
	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCredentialCreated,
		servicetypes.AuditResourceTypeCredential,
		credentialID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCredentialCreated, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCredential, log.GetResourceType())
	s.Equal(credentialID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *CredentialServiceSuite) TestDeleteCredential_WritesAuditLog() {
	var reqData pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &reqData)
	reqData.CredentialName = "audit-del-cred-" + uuid.NewString()

	ctx := s.T().Context()
	setReq := connect.NewRequest(&reqData)
	setReq.Header().Set("Authorization", "Bearer test-token")
	setResp, err := s.credentialClient.SetCredential(ctx, setReq)
	s.Require().NoError(err)
	s.Require().NotNil(setResp)
	credentialID := setResp.Msg.GetCredentialId()
	s.Require().NotEmpty(credentialID)

	delReq := connect.NewRequest(&pb.DeleteCredentialRequest{CredentialId: credentialID})
	delReq.Header().Set("Authorization", "Bearer test-token")
	_, err = s.credentialClient.DeleteCredential(ctx, delReq)
	s.Require().NoError(err)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCredentialDeleted,
		servicetypes.AuditResourceTypeCredential,
		credentialID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCredentialDeleted, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCredential, log.GetResourceType())
	s.Equal(credentialID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}
