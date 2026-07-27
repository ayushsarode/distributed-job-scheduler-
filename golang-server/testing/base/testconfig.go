package base

// insertTestData inserts the db with base test tenant and user.
func (s *IntegrationSuite) insertTestData() {
	ctx := s.T().Context()

	_, err := s.DBPool.Exec(ctx, `
		INSERT INTO tenants (id, name, created_at, updated_at, is_active)
		VALUES ($1, $2, NOW(), NOW(), TRUE)
		ON CONFLICT (id) DO NOTHING
	`, s.TestTenantID, "test-tenant-"+s.TestTenantID.String())
	s.Require().NoError(err, "Failed to insert test tenant")

	_, err = s.DBPool.Exec(ctx, `
		INSERT INTO users (user_id, first_name, last_name, tenant_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO NOTHING
	`, s.TestUserID, "Test", "User", s.TestTenantID)
	s.Require().NoError(err, "Failed to insert test user")

	s.T().Logf("Seeded test data: tenant=%s, user=%s", s.TestTenantID, s.TestUserID)
}
