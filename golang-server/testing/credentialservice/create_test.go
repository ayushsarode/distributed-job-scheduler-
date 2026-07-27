package credentialservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func (s *CredentialServiceSuite) TestCreateTwilioCredential_Success() {
	var reqData pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential HTTP request should succeed")
	s.Require().NotNil(resp)

	result := resp.Msg

	s.NotEmpty(result.GetCredentialId(), "Credential ID should not be empty")
	s.NotNil(result.GetCreatedAt(), "Created timestamp should be set")
	s.NotNil(result.GetUpdatedAt(), "Updated timestamp should be set")

	var want pb.SetCredentialResponse
	s.LoadTestData(serviceName, "response/create_twilio_credential.json", &want)

	diff := cmp.Diff(&want, result, protocmp.Transform(), protocmp.IgnoreFields(&pb.SetCredentialResponse{}, "credential_id", "created_at", "updated_at"))
	s.Empty(diff, "Response should match expected, diff: %s", diff)
}

func (s *CredentialServiceSuite) TestCreateExotelCredential_Success() {
	var reqData pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_exotel_credential.json", &reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential HTTP request should succeed")
	s.Require().NotNil(resp)

	result := resp.Msg

	s.NotEmpty(result.GetCredentialId(), "Credential ID should not be empty")
	s.NotNil(result.GetCreatedAt(), "Created timestamp should be set")
	s.NotNil(result.GetUpdatedAt(), "Updated timestamp should be set")

	var want pb.SetCredentialResponse
	s.LoadTestData(serviceName, "response/create_exotel_credential.json", &want)

	diff := cmp.Diff(&want, result, protocmp.Transform(), protocmp.IgnoreFields(&pb.SetCredentialResponse{}, "credential_id", "created_at", "updated_at"))
	s.Empty(diff, "Response should match expected, diff: %s", diff)
}

func (s *CredentialServiceSuite) TestCreateTwilioCredential_InvalidRequest() {
	var reqData pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_invalid_twilio_credential.json", &reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&reqData)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().Error(err, "SetCredential HTTP request should fail with invalid input")
	s.Require().Nil(resp, "Response should be nil on failure")

	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")

	wantCode := connect.CodeInvalidArgument
	diff := cmp.Diff(wantCode, connectErr.Code())
	s.Empty(diff, "Error code should match, diff: %s", diff)

	wantMsg := "bad request: credential_name is required"
	diff = cmp.Diff(wantMsg, connectErr.Message())
	s.Empty(diff, "Error message should match, diff: %s", diff)
}
