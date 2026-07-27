package transcriptionstorage

import (
	"errors"
	"testing"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestHandleStaleSession_Success(t *testing.T) {
	f := setupTest(t)

	f.mockRepo.EXPECT().GetSessionMeta(gomock.Any(), "session-123").
		Return(&entity.MetadataDynamo{TenantID: "tenant-123"}, nil)
	f.mockRepo.EXPECT().ClaimSession(gomock.Any(), "session-123").Return(nil)
	f.mockRepo.EXPECT().FetchSessionItems(gomock.Any(), "session-123").
		Return(entity.MetaItem{Status: "AGGREGATING"}, []entity.SegmentItem{}, nil)
	f.mockS3.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("etag", nil)
	f.mockRepo.EXPECT().FinalizeSession(gomock.Any(), "session-123", gomock.Any(), "COMPLETED_STALE", float64(0)).
		Return(nil)

	f.createService()

	err := f.service.handleStaleSession(f.ctx, "session-123")

	require.NoError(t, err)
}

func TestHandleStaleSession_GetMetaError(t *testing.T) {
	f := setupTest(t)

	repoError := xerrors.InternalError(f.ctx, errors.New("meta error"))

	f.mockRepo.EXPECT().GetSessionMeta(gomock.Any(), "session-123").
		Return(nil, repoError)

	f.createService()

	err := f.service.handleStaleSession(f.ctx, "session-123")

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, repoError, err)
}

func TestHandleStaleSession_NoMeta(t *testing.T) {
	f := setupTest(t)

	f.mockRepo.EXPECT().GetSessionMeta(gomock.Any(), "session-123").
		Return(nil, nil)

	f.createService()

	err := f.service.handleStaleSession(f.ctx, "session-123")

	require.NoError(t, err)
}

func TestHandleStaleSession_NoTenantID(t *testing.T) {
	f := setupTest(t)

	f.mockRepo.EXPECT().GetSessionMeta(gomock.Any(), "session-123").
		Return(&entity.MetadataDynamo{TenantID: ""}, nil)

	f.createService()

	err := f.service.handleStaleSession(f.ctx, "session-123")

	require.NoError(t, err)
}

func TestSweepOnce_ProcessesStaleSessions(t *testing.T) {
	f := setupTest(t)

	f.mockRepo.EXPECT().Query(gomock.Any(), "IN_PROGRESS", gomock.Any()).
		Return([]string{"stale-1"}, nil)
	f.mockRepo.EXPECT().Query(gomock.Any(), "AGGREGATING", gomock.Any()).
		Return([]string{}, nil)

	f.mockRepo.EXPECT().GetSessionMeta(gomock.Any(), "stale-1").
		Return(&entity.MetadataDynamo{TenantID: "tenant-123"}, nil)
	f.mockRepo.EXPECT().ClaimSession(gomock.Any(), "stale-1").Return(nil)
	f.mockRepo.EXPECT().FetchSessionItems(gomock.Any(), "stale-1").
		Return(entity.MetaItem{}, []entity.SegmentItem{}, nil)
	f.mockS3.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("etag", nil)
	f.mockRepo.EXPECT().FinalizeSession(gomock.Any(), "stale-1", gomock.Any(), "COMPLETED_STALE", float64(0)).Return(nil)

	f.createService()

	f.service.sweepOnce(f.ctx, 30*time.Minute)
}

func TestSweepOnce_QueryErrorContinues(t *testing.T) {
	f := setupTest(t)

	f.mockRepo.EXPECT().Query(gomock.Any(), "IN_PROGRESS", gomock.Any()).
		Return(nil, errors.New("DynamoDB error"))
	f.mockRepo.EXPECT().Query(gomock.Any(), "AGGREGATING", gomock.Any()).
		Return([]string{}, nil)

	f.createService()

	f.service.sweepOnce(f.ctx, 30*time.Minute)
}

func TestSweepOnce_HandleErrorContinues(t *testing.T) {
	f := setupTest(t)

	f.mockRepo.EXPECT().Query(gomock.Any(), "IN_PROGRESS", gomock.Any()).
		Return([]string{"stale-1", "stale-2"}, nil)
	f.mockRepo.EXPECT().Query(gomock.Any(), "AGGREGATING", gomock.Any()).
		Return([]string{}, nil)

	f.mockRepo.EXPECT().GetSessionMeta(gomock.Any(), "stale-1").
		Return(nil, errors.New("error"))

	f.mockRepo.EXPECT().GetSessionMeta(gomock.Any(), "stale-2").
		Return(&entity.MetadataDynamo{TenantID: "tenant-123"}, nil)
	f.mockRepo.EXPECT().ClaimSession(gomock.Any(), "stale-2").Return(nil)
	f.mockRepo.EXPECT().FetchSessionItems(gomock.Any(), "stale-2").
		Return(entity.MetaItem{}, []entity.SegmentItem{}, nil)
	f.mockS3.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("etag", nil)
	f.mockRepo.EXPECT().FinalizeSession(gomock.Any(), "stale-2", gomock.Any(), "COMPLETED_STALE", float64(0)).Return(nil)

	f.createService()

	f.service.sweepOnce(f.ctx, 30*time.Minute)
}
