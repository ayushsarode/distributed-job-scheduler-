package transcriptionstorage

import (
	"errors"
	"testing"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDetermineTargetStatus(t *testing.T) {
	f := setupTest(t)
	f.createService()

	tests := []struct {
		name     string
		isStale  bool
		expected string
	}{
		{"normal completion", false, "COMPLETED"},
		{"stale completion", true, "COMPLETED_STALE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := f.service.determineTargetStatus(tt.isStale)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildCallDocument(t *testing.T) {
	f := setupTest(t)
	f.createService()

	req := aggregationRequest{
		SessionID:    "session-123",
		TenantID:     "tenant-456",
		DurationSecs: 120.5,
		IsStale:      false,
	}

	meta := entity.MetaItem{
		Status: "IN_PROGRESS",
	}

	segments := []entity.SegmentItem{
		{Sequence: 1, Text: "Hello"},
		{Sequence: 2, Text: "World"},
	}

	doc := f.service.buildCallDocument(req, meta, segments, "COMPLETED")

	assert.Equal(t, "session-123", doc.SessionID)
	assert.Equal(t, "tenant-456", doc.TenantID)
	assert.Equal(t, "COMPLETED", doc.Status)
	assert.Equal(t, "COMPLETED", doc.Meta.Status)
	assert.Len(t, doc.Segments, 2)
}

func TestUploadCallDocumentToS3_Success(t *testing.T) {
	f := setupTest(t)

	callDoc := callDocument{
		SessionID: "session-123",
		TenantID:  "tenant-456",
		Status:    "COMPLETED",
	}

	f.mockS3.EXPECT().
		PutObject(gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Eq("session-123.json"), gomock.Eq("application/json")).
		Return("etag-123", nil)

	f.createService()

	s3Key, err := f.service.uploadCallDocumentToS3(f.ctx, callDoc)

	require.NoError(t, err)
	assert.Equal(t, "session-123.json", s3Key)
}

func TestUploadCallDocumentToS3_S3Error(t *testing.T) {
	f := setupTest(t)

	callDoc := callDocument{SessionID: "session-123"}

	f.mockS3.EXPECT().
		PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("S3 connection failed"))

	f.createService()

	result, err := f.service.uploadCallDocumentToS3(f.ctx, callDoc)

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Empty(t, result)
}

func TestAggregateSession_FullSuccess(t *testing.T) {
	f := setupTest(t)

	req := aggregationRequest{
		SessionID:    "session-123",
		TenantID:     "tenant-456",
		DurationSecs: 125.5,
		IsStale:      false,
	}

	f.mockRepo.EXPECT().
		ClaimSession(gomock.Any(), "session-123").
		Return(nil)

	f.mockRepo.EXPECT().
		FetchSessionItems(gomock.Any(), "session-123").
		Return(entity.MetaItem{Status: "AGGREGATING"}, []entity.SegmentItem{{Text: "Hello"}}, nil)

	f.mockS3.EXPECT().
		PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("etag", nil)

	f.mockRepo.EXPECT().
		FinalizeSession(gomock.Any(), "session-123", "session-123.json", "COMPLETED", 125.5).
		Return(nil)

	f.createService()

	err := f.service.aggregateSession(f.ctx, req)

	require.NoError(t, err)
}

func TestAggregateSession_ClaimFails(t *testing.T) {
	f := setupTest(t)

	req := aggregationRequest{SessionID: "session-123"}

	repoError := xerrors.InternalError(f.ctx, errors.New("claim failed"))

	f.mockRepo.EXPECT().
		ClaimSession(gomock.Any(), "session-123").
		Return(repoError)

	f.createService()

	err := f.service.aggregateSession(f.ctx, req)

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, repoError, err)
}

func TestAggregateSession_FetchFails(t *testing.T) {
	f := setupTest(t)

	req := aggregationRequest{SessionID: "session-123"}

	repoError := xerrors.InternalError(f.ctx, errors.New("fetch failed"))

	f.mockRepo.EXPECT().ClaimSession(gomock.Any(), "session-123").Return(nil)
	f.mockRepo.EXPECT().FetchSessionItems(gomock.Any(), "session-123").
		Return(entity.MetaItem{}, nil, repoError)

	f.createService()

	err := f.service.aggregateSession(f.ctx, req)

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, repoError, err)
}

func TestAggregateSession_S3UploadFails(t *testing.T) {
	f := setupTest(t)

	req := aggregationRequest{SessionID: "session-123"}

	f.mockRepo.EXPECT().ClaimSession(gomock.Any(), "session-123").Return(nil)
	f.mockRepo.EXPECT().FetchSessionItems(gomock.Any(), "session-123").
		Return(entity.MetaItem{}, []entity.SegmentItem{}, nil)

	f.mockS3.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("S3 error"))

	f.createService()

	err := f.service.aggregateSession(f.ctx, req)

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
}

func TestAggregateSession_FinalizeFails(t *testing.T) {
	f := setupTest(t)

	req := aggregationRequest{SessionID: "session-123"}

	repoError := xerrors.InternalError(f.ctx, errors.New("finalize failed"))

	f.mockRepo.EXPECT().ClaimSession(gomock.Any(), "session-123").Return(nil)
	f.mockRepo.EXPECT().FetchSessionItems(gomock.Any(), "session-123").
		Return(entity.MetaItem{}, []entity.SegmentItem{}, nil)
	f.mockS3.EXPECT().PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("etag", nil)
	f.mockRepo.EXPECT().FinalizeSession(gomock.Any(), "session-123", gomock.Any(), gomock.Any(), gomock.Any()).
		Return(repoError)

	f.createService()

	err := f.service.aggregateSession(f.ctx, req)

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, repoError, err)
}

func TestGetTranscriptionMetadata_Success(t *testing.T) {
	f := setupTest(t)

	sessionID := "session-123"
	tenantID := uuid.Must(uuid.NewV7())

	ctx := auth.SetTenant(f.ctx, tenantID)

	metaDynamo := &entity.MetadataDynamo{
		TenantID: tenantID.String(),
		S3Key:    "session-123.json",
	}

	f.mockRepo.EXPECT().
		GetSessionMeta(gomock.Any(), sessionID).
		Return(metaDynamo, nil)

	f.mockS3.EXPECT().
		GetSignedURL(gomock.Any(), gomock.Any(), "session-123.json", gomock.Any()).
		Return("https://signed-url", nil)

	f.createService()

	metadata, err := f.service.GetTranscriptionMetadata(ctx, sessionID)

	require.NoError(t, err)
	assert.Equal(t, "https://signed-url", metadata.DownloadURL)
}

func TestGetTranscriptionMetadata_NotFound(t *testing.T) {
	f := setupTest(t)

	sessionID := "session-123"
	tenantID := uuid.Must(uuid.NewV7())

	ctx := auth.SetTenant(f.ctx, tenantID)

	f.mockRepo.EXPECT().
		GetSessionMeta(gomock.Any(), sessionID).
		Return(nil, nil)

	f.createService()

	metadata, err := f.service.GetTranscriptionMetadata(ctx, sessionID)

	require.Error(t, err)
	assert.True(t, xerrors.IsNotFoundError(err))
	assert.Empty(t, metadata.DownloadURL)
}

func TestGetTranscriptionMetadata_RepositoryError(t *testing.T) {
	f := setupTest(t)

	sessionID := "session-123"
	tenantID := uuid.Must(uuid.NewV7())

	ctx := auth.SetTenant(f.ctx, tenantID)

	repoError := xerrors.InternalError(f.ctx, errors.New("database error"))

	f.mockRepo.EXPECT().
		GetSessionMeta(gomock.Any(), sessionID).
		Return(nil, repoError)

	f.createService()

	metadata, err := f.service.GetTranscriptionMetadata(ctx, sessionID)

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Equal(t, repoError, err)
	assert.Empty(t, metadata.DownloadURL)
}

func TestGetTranscriptionMetadata_Unauthorized(t *testing.T) {
	f := setupTest(t)

	sessionID := "session-123"
	requestingTenant := uuid.Must(uuid.NewV7())
	ownerTenant := uuid.Must(uuid.NewV7())

	ctx := auth.SetTenant(f.ctx, requestingTenant)

	metaDynamo := &entity.MetadataDynamo{
		TenantID: ownerTenant.String(),
	}

	f.mockRepo.EXPECT().
		GetSessionMeta(gomock.Any(), sessionID).
		Return(metaDynamo, nil)

	f.createService()

	metadata, err := f.service.GetTranscriptionMetadata(ctx, sessionID)

	require.Error(t, err)
	assert.True(t, xerrors.IsInternalError(err))
	assert.Empty(t, metadata.DownloadURL)
}

func TestGetTranscriptionMetadata_NoS3Key(t *testing.T) {
	f := setupTest(t)

	sessionID := "session-123"
	tenantID := uuid.Must(uuid.NewV7())

	ctx := auth.SetTenant(f.ctx, tenantID)

	metaDynamo := &entity.MetadataDynamo{
		TenantID: tenantID.String(),
		S3Key:    "",
	}

	f.mockRepo.EXPECT().
		GetSessionMeta(gomock.Any(), sessionID).
		Return(metaDynamo, nil)

	f.createService()

	metadata, err := f.service.GetTranscriptionMetadata(ctx, sessionID)

	require.NoError(t, err)
	assert.Empty(t, metadata.DownloadURL)
}
