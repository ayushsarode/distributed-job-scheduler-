package agentutils

import (
	"context"
	"strings"
	"testing"

	"exiro.ai/application/models/pb"
	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateAgentID_RTDeployment_ReturnsRTSuffix verifies RT deployment type generates ID with "_rt" suffix.
func TestGenerateAgentID_RTDeployment_ReturnsRTSuffix(t *testing.T) {
	id, err := GenerateAgentID(context.Background(), pb.DeploymentType_DEPLOYMENT_TYPE_RT, pb.AgentModel_AGENTMODEL_GPT_REALTIME)

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(id, "_rt"), "ID should end with '_rt' suffix")

	ulidPart := strings.TrimSuffix(id, "_rt")
	_, err = ulid.Parse(ulidPart)
	assert.NoError(t, err, "Part before suffix should be a valid ULID")
}

// TestGenerateAgentID_WSDeployment_ReturnsWSSuffix verifies WS deployment type generates ID with "_ws" suffix.
func TestGenerateAgentID_WSDeployment_ReturnsWSSuffix(t *testing.T) {
	id, err := GenerateAgentID(context.Background(), pb.DeploymentType_DEPLOYMENT_TYPE_WS, pb.AgentModel_AGENTMODEL_GPT_4OMINI)

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(id, "_ws"), "ID should end with '_ws' suffix")

	ulidPart := strings.TrimSuffix(id, "_ws")
	_, err = ulid.Parse(ulidPart)
	assert.NoError(t, err, "Part before suffix should be a valid ULID")
}

// TestGenerateAgentID_LGDeployment_ReturnsLGSuffix verifies LG deployment type generates ID with "_lg" suffix.
func TestGenerateAgentID_LGDeployment_ReturnsLGSuffix(t *testing.T) {
	id, err := GenerateAgentID(context.Background(), pb.DeploymentType_DEPLOYMENT_TYPE_LG, pb.AgentModel_AGENTMODEL_GPT_4OMINI)

	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(id, "_lg"), "ID should end with '_lg' suffix")

	ulidPart := strings.TrimSuffix(id, "_lg")
	_, err = ulid.Parse(ulidPart)
	assert.NoError(t, err, "Part before suffix should be a valid ULID")
}

// TestGenerateAgentID_UnknownDeployment_ReturnsError verifies UNKNOWN deployment type returns error.
func TestGenerateAgentID_UnknownDeployment_ReturnsError(t *testing.T) {
	id, err := GenerateAgentID(context.Background(), pb.DeploymentType_DEPLOYMENT_TYPE_UNKNOWN, pb.AgentModel_AGENTMODEL_GPT_4OMINI)

	require.Error(t, err)
	assert.Empty(t, id)
	assert.Contains(t, err.Error(), "deployment_type must be explicitly specified", "Error should indicate deployment type must be specified")
}

// TestGenerateAgentID_RTWithNonRealtimeModel_ReturnsError verifies RT deployment with non-GPT_REALTIME model returns validation error.
func TestGenerateAgentID_RTWithNonRealtimeModel_ReturnsError(t *testing.T) {
	id, err := GenerateAgentID(context.Background(), pb.DeploymentType_DEPLOYMENT_TYPE_RT, pb.AgentModel_AGENTMODEL_GPT_4OMINI)

	require.Error(t, err)
	assert.Empty(t, id)
	assert.Contains(t, err.Error(), "RT deployment type requires GPT_REALTIME model", "Error should indicate model mismatch")
}

// TestGenerateAgentID_RTWithRealtimeModel_Succeeds verifies RT deployment with GPT_REALTIME model succeeds.
func TestGenerateAgentID_RTWithRealtimeModel_Succeeds(t *testing.T) {
	id, err := GenerateAgentID(context.Background(), pb.DeploymentType_DEPLOYMENT_TYPE_RT, pb.AgentModel_AGENTMODEL_GPT_REALTIME)

	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.True(t, strings.HasSuffix(id, "_rt"))
}

// TestIsLGAgent verifies IsLGAgent correctly identifies _lg suffix.
func TestIsLGAgent(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		expected bool
	}{
		{
			name:     "WithLGSuffix_ReturnsTrue",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_lg",
			expected: true,
		},
		{
			name:     "WithWSSuffix_ReturnsFalse",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_ws",
			expected: false,
		},
		{
			name:     "WithRTSuffix_ReturnsFalse",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_rt",
			expected: false,
		},
		{
			name:     "WithEmptyString_ReturnsFalse",
			agentID:  "",
			expected: false,
		},
		{
			name:     "WithShortString_ReturnsFalse",
			agentID:  "lg",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsLGAgent(tt.agentID))
		})
	}
}

// TestIsWSAgent verifies IsWSAgent correctly identifies _ws suffix.
func TestIsWSAgent(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		expected bool
	}{
		{
			name:     "WithWSSuffix_ReturnsTrue",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_ws",
			expected: true,
		},
		{
			name:     "WithLGSuffix_ReturnsFalse",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_lg",
			expected: false,
		},
		{
			name:     "WithRTSuffix_ReturnsFalse",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_rt",
			expected: false,
		},
		{
			name:     "WithEmptyString_ReturnsFalse",
			agentID:  "",
			expected: false,
		},
		{
			name:     "WithShortString_ReturnsFalse",
			agentID:  "ws",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsWSAgent(tt.agentID))
		})
	}
}

// TestIsRTAgent verifies IsRTAgent correctly identifies _rt suffix.
func TestIsRTAgent(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		expected bool
	}{
		{
			name:     "WithRTSuffix_ReturnsTrue",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_rt",
			expected: true,
		},
		{
			name:     "WithLGSuffix_ReturnsFalse",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_lg",
			expected: false,
		},
		{
			name:     "WithWSSuffix_ReturnsFalse",
			agentID:  "01KKQJHPRV9SH3ZX42G8G84XHX_ws",
			expected: false,
		},
		{
			name:     "WithEmptyString_ReturnsFalse",
			agentID:  "",
			expected: false,
		},
		{
			name:     "WithShortString_ReturnsFalse",
			agentID:  "rt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsRTAgent(tt.agentID))
		})
	}
}
