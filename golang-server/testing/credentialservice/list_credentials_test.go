package credentialservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Verifies that ListCredentials returns created credentials with correct fields.
func (s *CredentialServiceSuite) TestListCredentials_Success() {
	// Create a Twilio credential
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)

	ctx := s.T().Context()
	req := connect.NewRequest(&twilioReq)
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential should succeed")
	s.Require().NotNil(resp)

	// List credentials
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{Limit: 100, Offset: 0})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "ListCredentials should succeed")
	s.Require().NotNil(listResp)

	credentials := listResp.Msg.GetCredentials()
	s.GreaterOrEqual(len(credentials), 1, "Should have at least 1 credential")

	// Verify credential fields
	found := false
	for _, cred := range credentials {
		if cred.GetCredentialId() == resp.Msg.GetCredentialId() {
			found = true
			s.NotEmpty(cred.GetCredentialName(), "Credential name should not be empty")
			s.NotNil(cred.GetCreatedAt(), "Created timestamp should be set")
			s.NotNil(cred.GetUpdatedAt(), "Updated timestamp should be set")
			break
		}
	}
	s.True(found, "Created credential should be in the list")
}

// Verifies that ListCredentials can filter by credential type.
func (s *CredentialServiceSuite) TestListCredentials_FilterByType() {
	ctx := s.T().Context()

	// Create a Twilio credential
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)
	twilioReq.CredentialName = "Filter-Twilio-" + uuid.NewString()

	req := connect.NewRequest(&twilioReq)
	req.Header().Set("Authorization", "Bearer test-token")

	twilioResp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential for Twilio should succeed")
	twilioID := twilioResp.Msg.GetCredentialId()

	// Create an Exotel credential (should NOT appear in TWILIO filter results)
	var exotelReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_exotel_credential.json", &exotelReq)
	exotelReq.CredentialName = "Filter-Exotel-" + uuid.NewString()

	req = connect.NewRequest(&exotelReq)
	req.Header().Set("Authorization", "Bearer test-token")

	exotelResp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential for Exotel should succeed")
	exotelID := exotelResp.Msg.GetCredentialId()

	// List credentials filtering by TWILIO type
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{
		Limit:  100,
		Offset: 0,
		Type:   []pb.CredentialType{pb.CredentialType_TWILIO},
	})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "ListCredentials with type filter should succeed")
	s.Require().NotNil(listResp)

	// All returned credentials should be TWILIO type
	for _, cred := range listResp.Msg.GetCredentials() {
		s.Equal(pb.CredentialType_TWILIO, cred.GetType(), "Credential should be TWILIO type")
	}

	// Verify the TWILIO credential IS in the results
	credIDs := make(map[string]bool)
	for _, cred := range listResp.Msg.GetCredentials() {
		credIDs[cred.GetCredentialId()] = true
	}
	s.True(credIDs[twilioID], "TWILIO credential should be in filtered list")

	// Verify the EXOTEL credential is NOT in the results
	s.False(credIDs[exotelID], "EXOTEL credential should NOT be in TWILIO filtered list")
}

// Verifies that ListCredentials with limit returns at most limit credentials.
func (s *CredentialServiceSuite) TestListCredentials_Pagination_Limit() {
	// Create multiple credentials
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)

	ctx := s.T().Context()
	for range 5 {
		twilioReq.CredentialName = "Limit-Test-" + uuid.NewString()
		req := connect.NewRequest(&twilioReq)
		req.Header().Set("Authorization", "Bearer test-token")

		_, err := s.credentialClient.SetCredential(ctx, req)
		s.Require().NoError(err, "SetCredential should succeed")
	}

	// List with limit=2
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{Limit: 2, Offset: 0})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "ListCredentials with limit=2 should succeed")
	s.Require().NotNil(listResp)

	credentials := listResp.Msg.GetCredentials()
	s.LessOrEqual(len(credentials), 2, "Should return at most 2 credentials with limit=2")
}

// Verifies that ListCredentials with a high offset returns an empty list.
func (s *CredentialServiceSuite) TestListCredentials_HighOffset_EmptyResult() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCredentialsRequest{Limit: 10, Offset: 999999})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.credentialClient.ListCredentials(ctx, req)
	s.Require().NoError(err, "ListCredentials with high offset should succeed")
	s.Require().NotNil(resp)

	credentials := resp.Msg.GetCredentials()
	s.Empty(credentials, "High offset should return no credentials")
}

// Verifies that ListCredentials pagination offset advances the page window correctly.
func (s *CredentialServiceSuite) TestListCredentials_Pagination_Offset() {
	// Create multiple credentials with unique names
	for i := range 4 {
		var twilioReq pb.SetCredentialRequest
		s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)
		twilioReq.CredentialName = "Offset-Test-" + uuid.NewString() + "-" + string(rune('A'+i))

		ctx := s.T().Context()
		req := connect.NewRequest(&twilioReq)
		req.Header().Set("Authorization", "Bearer test-token")

		_, err := s.credentialClient.SetCredential(ctx, req)
		s.Require().NoError(err, "SetCredential should succeed")
	}

	ctx := s.T().Context()

	// Get first page
	first, err := s.credentialClient.ListCredentials(ctx, 
		connect.NewRequest(&pb.ListCredentialsRequest{Limit: 2, Offset: 0}),
	)
	s.Require().NoError(err, "First page request should succeed")

	// Get second page
	second, err := s.credentialClient.ListCredentials(ctx, connect.NewRequest(&pb.ListCredentialsRequest{Limit: 2, Offset: 2}))
	s.Require().NoError(err, "Second page request should succeed")

	// Pages should have different results
	firstIDs := make(map[string]bool)
	for _, cred := range first.Msg.GetCredentials() {
		firstIDs[cred.GetCredentialId()] = true
	}

	for _, cred := range second.Msg.GetCredentials() {
		s.False(firstIDs[cred.GetCredentialId()], "Credential %s should not appear in both pages", cred.GetCredentialId())
	}
}

// Verifies that ListCredentials can filter by multiple credential types.
func (s *CredentialServiceSuite) TestListCredentials_FilterByMultipleTypes() {
	ctx := s.T().Context()

	// Create a Twilio credential
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)
	twilioReq.CredentialName = "MultiType-Twilio-" + uuid.NewString()

	req := connect.NewRequest(&twilioReq)
	req.Header().Set("Authorization", "Bearer test-token")

	twilioResp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential for Twilio should succeed")
	twilioID := twilioResp.Msg.GetCredentialId()

	// Create an Exotel credential
	var exotelReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_exotel_credential.json", &exotelReq)
	exotelReq.CredentialName = "MultiType-Exotel-" + uuid.NewString()

	req = connect.NewRequest(&exotelReq)
	req.Header().Set("Authorization", "Bearer test-token")

	exotelResp, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential for Exotel should succeed")
	exotelID := exotelResp.Msg.GetCredentialId()

	// List credentials filtering by multiple types (TWILIO and EXOTEL)
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{
		Limit:  100,
		Offset: 0,
		Type:   []pb.CredentialType{pb.CredentialType_TWILIO, pb.CredentialType_EXOTEL},
	})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "ListCredentials with multiple type filter should succeed")
	s.Require().NotNil(listResp)

	// All returned credentials should be either TWILIO or EXOTEL type
	for _, cred := range listResp.Msg.GetCredentials() {
		s.Contains([]pb.CredentialType{pb.CredentialType_TWILIO, pb.CredentialType_EXOTEL}, cred.GetType(),
			"Credential should be TWILIO or EXOTEL type")
	}

	// Verify both TWILIO and EXOTEL credentials are in the results
	credIDs := make(map[string]bool)
	for _, cred := range listResp.Msg.GetCredentials() {
		credIDs[cred.GetCredentialId()] = true
	}
	s.True(credIDs[twilioID], "TWILIO credential should be in filtered list")
	s.True(credIDs[exotelID], "EXOTEL credential should be in filtered list")
}

// Verifies that ListCredentials with zero limit uses the default limit of 50.
func (s *CredentialServiceSuite) TestListCredentials_ZeroLimit_UsesDefault() {
	// Create 55 credentials to exceed the default limit of 50
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)

	ctx := s.T().Context()
	for range 55 {
		twilioReq.CredentialName = "ZeroLimit-" + uuid.NewString()
		req := connect.NewRequest(&twilioReq)
		req.Header().Set("Authorization", "Bearer test-token")
		_, err := s.credentialClient.SetCredential(ctx, req)
		s.Require().NoError(err, "SetCredential should succeed")
	}

	// List with zero limit
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{Limit: 0, Offset: 0})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "Zero limit should succeed using default")
	s.Require().NotNil(listResp)
	s.LessOrEqual(len(listResp.Msg.GetCredentials()), 50, "Zero limit should use default of 50")
}

// Verifies that ListCredentials with negative offset is clamped to 0.
func (s *CredentialServiceSuite) TestListCredentials_NegativeOffset_ClampedToZero() {
	// Create multiple credentials to ensure we have data
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)

	ctx := s.T().Context()
	createdIDs := make([]string, 5)
	for i := range 5 {
		twilioReq.CredentialName = "NegOffset-" + uuid.NewString()
		req := connect.NewRequest(&twilioReq)
		req.Header().Set("Authorization", "Bearer test-token")
		setResp, err := s.credentialClient.SetCredential(ctx, req)
		s.Require().NoError(err, "SetCredential should succeed")
		createdIDs[i] = setResp.Msg.GetCredentialId()
	}

	// List with negative offset
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{Limit: 10, Offset: -10})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "Negative offset should be clamped to 0 and succeed")
	s.Require().NotNil(listResp)

	// Should return results from the beginning (offset clamped to 0)
	foundIDs := make(map[string]bool)
	for _, cred := range listResp.Msg.GetCredentials() {
		foundIDs[cred.GetCredentialId()] = true
	}
	for _, id := range createdIDs {
		s.True(foundIDs[id], "Negative offset should be clamped to 0 and include created credential %s", id)
	}
}

// Verifies that ListCredentials with negative limit uses the default limit of 50.
func (s *CredentialServiceSuite) TestListCredentials_NegativeLimit_UsesDefault() {
	// Create 55 credentials to exceed the default limit of 50
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)

	ctx := s.T().Context()
	for range 55 {
		twilioReq.CredentialName = "NegLimit-" + uuid.NewString()
		req := connect.NewRequest(&twilioReq)
		req.Header().Set("Authorization", "Bearer test-token")
		_, err := s.credentialClient.SetCredential(ctx, req)
		s.Require().NoError(err, "SetCredential should succeed")
	}

	// List with negative limit
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{Limit: -10, Offset: 0})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "Negative limit should succeed using default")
	s.Require().NotNil(listResp)
	s.LessOrEqual(len(listResp.Msg.GetCredentials()), 50, "Negative limit should use default of 50")
}

// Verifies that ListCredentials with high limit is clamped to max of 50.
func (s *CredentialServiceSuite) TestListCredentials_HighLimit_ClampedToMax() {
	// Create 55 credentials to exceed the max limit of 50
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)

	ctx := s.T().Context()
	for range 55 {
		twilioReq.CredentialName = "HighLimit-" + uuid.NewString()
		req := connect.NewRequest(&twilioReq)
		req.Header().Set("Authorization", "Bearer test-token")
		_, err := s.credentialClient.SetCredential(ctx, req)
		s.Require().NoError(err, "SetCredential should succeed")
	}

	// List with high limit
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{Limit: 9999, Offset: 0})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "High limit should be clamped to 50 and succeed")
	s.Require().NotNil(listResp)
	s.LessOrEqual(len(listResp.Msg.GetCredentials()), 50, "High limit should be clamped to max 50")
}

// Verifies that ListCredentials with type filter returns only matching types.
func (s *CredentialServiceSuite) TestListCredentials_FilterByType_NoMatches() {
	ctx := s.T().Context()
	s.clearCredentials(ctx)

	// Create a Twilio credential
	var twilioReq pb.SetCredentialRequest
	s.LoadTestData(serviceName, "request/create_twilio_credential.json", &twilioReq)
	twilioReq.CredentialName = "NoMatch-Test-" + uuid.NewString()

	req := connect.NewRequest(&twilioReq)
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.credentialClient.SetCredential(ctx, req)
	s.Require().NoError(err, "SetCredential should succeed")

	// Filter by EXOTEL type - should return empty since we only created TWILIO
	listReq := connect.NewRequest(&pb.ListCredentialsRequest{
		Limit:  100,
		Offset: 0,
		Type:   []pb.CredentialType{pb.CredentialType_EXOTEL},
	})
	listReq.Header().Set("Authorization", "Bearer test-token")

	listResp, err := s.credentialClient.ListCredentials(ctx, listReq)
	s.Require().NoError(err, "ListCredentials with EXOTEL type filter should succeed")
	s.Require().NotNil(listResp)

	// Should return empty list since no EXOTEL credentials were created
	s.Empty(listResp.Msg.GetCredentials(), "Filtering by EXOTEL should return empty when only TWILIO exists")
}
