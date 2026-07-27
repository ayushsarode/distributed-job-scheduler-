package transcriptionstorage

//go:generate mockgen -destination=mocks/mock_repository.go -package=mocks exiro.ai/application/service/internal/types TranscriptionRepository,AgentKVRepository
//go:generate mockgen -destination=mocks/mock_queue.go -package=mocks exiro.ai/application/service/internal/types FIFOMessageQueue,FIFOMessageConsumer
//go:generate mockgen -destination=mocks/mock_objectstore.go -package=mocks exiro.ai/application/service/internal/types ObjectStore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/transcriptionstorage/mocks"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type testDependencies struct {
	ctrl *gomock.Controller
	ctx  context.Context

	mockRepo     *mocks.MockTranscriptionRepository
	mockAgentKV  *mocks.MockAgentKVRepository
	mockS3       *mocks.MockObjectStore
	mockQueue    *mocks.MockFIFOMessageQueue
	mockConsumer *mocks.MockFIFOMessageConsumer

	service *Service

	sessionID string
	tenantID  string
	now       time.Time
}

const (
	sweepMinutes = 15
	saleMinutes  = 30
)

// NewTestContext creates a context with test config.
func NewTranscriptionTestContext() context.Context {
	ctx := context.Background()
	testConfig := &config.Config{
		TranscriptionStorage: config.TranscriptionStorageConfig{
			TranscriptionTableName: "test-table",
			AgentKVStoreTableName:  "test-agent-kv",
			S3BucketName:           "test-bucket",
			Sweeper: config.SweeperConfig{
				SweepIntervalMinutes:  sweepMinutes,
				StaleThresholdMinutes: saleMinutes,
			},
		},
	}
	ctx = config.WithContext(ctx, testConfig)

	logger := zerolog.Nop()
	ctx = logger.WithContext(ctx)

	return ctx
}

func setupTest(t *testing.T) *testDependencies {
	ctrl := gomock.NewController(t)

	return &testDependencies{
		ctrl:         ctrl,
		ctx:          NewTranscriptionTestContext(),
		mockRepo:     mocks.NewMockTranscriptionRepository(ctrl),
		mockAgentKV:  mocks.NewMockAgentKVRepository(ctrl),
		mockS3:       mocks.NewMockObjectStore(ctrl),
		mockQueue:    mocks.NewMockFIFOMessageQueue(ctrl),
		mockConsumer: mocks.NewMockFIFOMessageConsumer(ctrl),
		sessionID:    "test-session-123",
		tenantID:     "test-tenant-456",
		now:          time.Now(),
	}
}

func (f *testDependencies) createService() {
	f.service = &Service{
		messageQueue:            f.mockQueue,
		consumer:                f.mockConsumer,
		transcriptionRepository: f.mockRepo,
		agentKVRepository:       f.mockAgentKV,
		objectStore:             f.mockS3,
		logger:                  zerolog.Ctx(f.ctx),
	}
}

func (f *testDependencies) createSegmentEvent(sequence uint64, text string) *pb.TranscriptEvent {
	sec := int64(sequence) // #nosec G115
	return &pb.TranscriptEvent{
		SessionId: f.sessionID,
		TenantId:  f.tenantID,
		SegmentId: fmt.Sprintf("seg-%d", sequence),
		EventPayload: &pb.TranscriptEvent_Segment{
			Segment: &pb.Segment{
				Sequence:     sequence,
				Speaker:      pb.Speaker_AI_AGENT,
				Text:         text,
				Timestamp:    timestamppb.New(f.now.Add(time.Duration(sec) * time.Second)),
				LanguageCode: "en-US",
			},
		},
	}
}

func (f *testDependencies) createEndOfCallEvent(duration float32) *pb.TranscriptEvent {
	return &pb.TranscriptEvent{
		SessionId: f.sessionID,
		TenantId:  f.tenantID,
		EventPayload: &pb.TranscriptEvent_EndOfCall{
			EndOfCall: &pb.CallEnd{
				DurationSeconds: duration,
			},
		},
	}
}

func TestHandleTranscriptEvent_SegmentSuccess(t *testing.T) {
	f := setupTest(t)

	event := f.createSegmentEvent(1, "Hello world")
	messageBody := `{"session_id":"test-session-123","segment_id":"seg-1"}`

	f.mockRepo.EXPECT().
		InsertSegment(gomock.Any(), event, gomock.Any()).
		Return(nil)

	f.createService()

	err := f.service.handleTranscriptEvent(f.ctx, event, messageBody)

	require.NoError(t, err)
}

func TestHandleTranscriptEvent_DuplicateSegmentIsSwallowed(t *testing.T) {
	f := setupTest(t)

	event := f.createSegmentEvent(1, "Hello")
	messageBody := `{"session_id":"session-123","segment_id":"seg-1"}`

	f.mockRepo.EXPECT().
		InsertSegment(gomock.Any(), event, gomock.Any()).
		Return(&dynamodbTypes.TransactionCanceledException{
			Message: aws.String("ConditionalCheckFailed"),
		})

	f.createService()

	err := f.service.handleTranscriptEvent(f.ctx, event, messageBody)

	require.NoError(t, err, "ConditionalCheckFailed must be swallowed")
}

func TestHandleTranscriptEvent_DynamoError(t *testing.T) {
	f := setupTest(t)

	event := f.createSegmentEvent(1, "Hello")
	messageBody := `{"session_id":"session-123"}`

	repoError := xerrors.InternalError(f.ctx, errors.New("internal error"))
	f.mockRepo.EXPECT().
		InsertSegment(gomock.Any(), event, gomock.Any()).
		Return(repoError)

	f.createService()

	err := f.service.handleTranscriptEvent(f.ctx, event, messageBody)

	require.Error(t, err)
	assert.Equal(t, repoError, err)
	assert.True(t, xerrors.IsInternalError(err))
}

func TestHandleTranscriptEvent_EndOfCallCallsHandler(t *testing.T) {
	f := setupTest(t)

	event := f.createEndOfCallEvent(125.5)
	messageBody := `{"session_id":"session-123"}`

	// Claim
	f.mockRepo.EXPECT().
		ClaimSession(gomock.Any(), f.sessionID).
		Return(nil)

	// Fetch
	f.mockRepo.EXPECT().
		FetchSessionItems(gomock.Any(), f.sessionID).
		Return(entity.MetaItem{Status: "AGGREGATING"}, []entity.SegmentItem{{Text: "Hello"}}, nil)

	// Upload S3
	f.mockS3.EXPECT().
		PutObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(f.sessionID+".json"), gomock.Any()).
		Return("etag-123", nil)

	// Finalize
	f.mockRepo.EXPECT().
		FinalizeSession(gomock.Any(), f.sessionID, f.sessionID+".json", "COMPLETED", float64(125.5)).
		Return(nil)

	f.createService()

	err := f.service.handleTranscriptEvent(f.ctx, event, messageBody)

	require.NoError(t, err)
}

func TestHandleTranscriptEvent_UnknownPayloadReturnsError(t *testing.T) {
	f := setupTest(t)

	event := &pb.TranscriptEvent{
		SessionId:    "session-123",
		TenantId:     "tenant-abc",
		EventPayload: nil,
	}
	messageBody := `{"session_id":"session-123"}`

	f.createService()

	err := f.service.handleTranscriptEvent(f.ctx, event, messageBody)

	require.Error(t, err)
	require.Equal(t, "unknown event payload type", err.Error())
}
