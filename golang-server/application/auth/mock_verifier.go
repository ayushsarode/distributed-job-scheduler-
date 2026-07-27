package auth

type MockTokenVerifier struct {
	userID string
}

func NewMockTokenVerifier(userID string) TokenVerifier {
	return &MockTokenVerifier{userID: userID}
}

func (m *MockTokenVerifier) Verify(token string) (string, error) {
	return m.userID, nil
}
