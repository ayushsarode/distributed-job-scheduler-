package transcriptionstorage

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestGetSessionKVData_EmptySessionID(t *testing.T) {
	f := setupTest(t)
	tenantID := uuid.New()
	ctx := auth.SetTenant(f.ctx, tenantID)
	f.createService()

	pairs, err := f.service.GetSessionKVData(ctx, "  ")
	require.Error(t, err)
	assert.True(t, xerrors.IsBadRequestError(err))
	assert.Nil(t, pairs)
}

func TestGetSessionKVData_EmptyPairs(t *testing.T) {
	f := setupTest(t)
	sessionID := "session-123"
	tenantID := uuid.New()
	ctx := auth.SetTenant(f.ctx, tenantID)

	f.mockAgentKV.EXPECT().QuerySessionKV(gomock.Any(), sessionID, int32(101)).Return(nil, nil)
	f.createService()

	pairs, err := f.service.GetSessionKVData(ctx, sessionID)
	require.NoError(t, err)
	assert.Empty(t, pairs)
}

func TestGetSessionKVData_QueryKVDynamoError(t *testing.T) {
	f := setupTest(t)
	sessionID := "session-123"
	tenantID := uuid.New()
	ctx := auth.SetTenant(f.ctx, tenantID)

	repoErr := xerrors.InternalError(f.ctx, errors.New("dynamo timeout"))
	f.mockAgentKV.EXPECT().QuerySessionKV(gomock.Any(), sessionID, int32(101)).Return(nil, repoErr)
	f.createService()

	pairs, err := f.service.GetSessionKVData(ctx, sessionID)
	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Nil(t, pairs)
}

func TestGetSessionKVData_Success(t *testing.T) {
	f := setupTest(t)
	sessionID := "session-123"
	tenantID := uuid.New()
	ctx := auth.SetTenant(f.ctx, tenantID)

	f.mockAgentKV.EXPECT().QuerySessionKV(gomock.Any(), sessionID, int32(101)).Return([]entity.AgentKVItem{
		{SessionID: sessionID, Key: "payment_date", Value: "2026-04-15", TenantID: tenantID.String(), CreatedAt: time.Unix(100, 0), UpdatedAt: time.Unix(200, 0)},
	}, nil)
	f.createService()

	pairs, err := f.service.GetSessionKVData(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, pairs, 1)
	assert.Equal(t, "payment_date", pairs[0].Key)
	assert.Equal(t, "2026-04-15", pairs[0].Value)
	assert.Equal(t, time.Unix(100, 0).UTC(), pairs[0].CreatedAt)
	assert.Equal(t, time.Unix(200, 0).UTC(), pairs[0].UpdatedAt)
}

func TestGetSessionKVData_TenantMismatch_Unauthorized(t *testing.T) {
	f := setupTest(t)
	sessionID := "session-123"
	tenantID := uuid.New()
	ctx := auth.SetTenant(f.ctx, tenantID)

	f.mockAgentKV.EXPECT().QuerySessionKV(gomock.Any(), sessionID, int32(101)).Return([]entity.AgentKVItem{
		{SessionID: sessionID, Key: "payment_date", Value: "2026-04-15", TenantID: uuid.New().String(), CreatedAt: time.Unix(100, 0), UpdatedAt: time.Unix(200, 0)},
	}, nil)
	f.createService()

	pairs, err := f.service.GetSessionKVData(ctx, sessionID)
	require.Error(t, err)
	assert.True(t, xerrors.IsPermissionDeniedError(err))
	assert.Nil(t, pairs)
}

func TestGetSessionKVData_Truncated(t *testing.T) {
	f := setupTest(t)
	sessionID := "session-123"
	tenantID := uuid.New()
	ctx := auth.SetTenant(f.ctx, tenantID)

	// Build 101 rows all owned by the requesting tenant.
	rows := make([]entity.AgentKVItem, 101)
	for i := range rows {
		rows[i] = entity.AgentKVItem{
			SessionID: sessionID,
			Key:       fmt.Sprintf("key_%d", i),
			Value:     "v",
			TenantID:  tenantID.String(),
			CreatedAt: time.Unix(int64(i), 0),
			UpdatedAt: time.Unix(int64(i), 0),
		}
	}

	f.mockAgentKV.EXPECT().QuerySessionKV(gomock.Any(), sessionID, int32(101)).Return(rows, nil)
	f.createService()

	pairs, err := f.service.GetSessionKVData(ctx, sessionID)
	require.NoError(t, err)
	// Service truncates to 100.
	assert.Len(t, pairs, 100)
}
