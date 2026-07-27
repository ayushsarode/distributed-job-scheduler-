package transcriptionstorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/config"
	dynamodbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	SignedURLExpiration = 5 * time.Minute
)

type Service struct {
	messageQueue             repositoryTypes.FIFOMessageQueue
	consumer                 repositoryTypes.FIFOMessageConsumer
	transcriptionRepository  repositoryTypes.TranscriptionRepository
	agentKVRepository        repositoryTypes.AgentKVRepository
	objectStore              repositoryTypes.ObjectStore
	transcriptionsBucketName string
	logger                   *zerolog.Logger
}

var _ types.TranscriptionStorageService = (*Service)(nil)

func New(ctx context.Context,
	messageQueue repositoryTypes.FIFOMessageQueue,
	consumer repositoryTypes.FIFOMessageConsumer,
	objectStore repositoryTypes.ObjectStore,
	transcriptionRepository repositoryTypes.TranscriptionRepository,
	agentKVRepository repositoryTypes.AgentKVRepository,
) *Service {
	return &Service{
		messageQueue:             messageQueue,
		consumer:                 consumer,
		transcriptionRepository:  transcriptionRepository,
		agentKVRepository:        agentKVRepository,
		objectStore:              objectStore,
		transcriptionsBucketName: config.Ctx(ctx).TranscriptionStorage.S3BucketName,
		logger:                   zerolog.Ctx(ctx),
	}
}

func (s *Service) TestPushSQS(ctx context.Context, event *pb.TranscriptEvent) error {
	s.logger.Debug().Msgf("Pushing test message to FIFO SQS %#v", event)

	if event.GetSegmentId() == "" {
		return xerrors.InternalError(ctx, errors.New("sessionId is required"))
	}

	msgBytes, err := protojson.Marshal(event)
	if err != nil {
		return xerrors.InternalError(ctx, err)
	}

	// sessionid+content+sequence, and make sha256. Ans send that as a deduplication id

	hash := sha256.New()
	hash.Write([]byte(event.GetSessionId()))
	hash.Write(msgBytes)
	dedupID := hex.EncodeToString(hash.Sum(nil))

	return s.messageQueue.PublishStringMessage(ctx, string(msgBytes), event.GetSessionId(), dedupID)
}

func (s *Service) StartIngestWorker(ctx context.Context) error {
	s.logger.Info().Msg("Starting Transcription Ingest Worker...")
	// Start the ingest worker
	go func() {
		err := s.consumer.ConsumeStringMessage(ctx, func(ctx context.Context, messageBody string, messageGroupID string) error {
			s.logger.Debug().Msgf("Received message from FIFO queue with group ID: %s", messageGroupID)
			event := &pb.TranscriptEvent{}
			if err := protojson.Unmarshal([]byte(messageBody), event); err != nil {
				return xerrors.InternalError(ctx, err)
			}
			// Validate required fields
			if event.GetSessionId() == "" || event.GetTenantId() == "" {
				return xerrors.InternalError(ctx, errors.New("missing required fields: session_id or tenant_id"))
			}

			return s.handleTranscriptEvent(ctx, event, messageBody)
		})
		if err != nil {
			s.logger.Error().Err(err).Msg("Consumer error")
		}
	}()
	s.startSweeper(ctx)
	s.logger.Info().Msg("Transcription workers (ingest + sweeper) started in background")
	return nil
}

func (s *Service) handleTranscriptEvent(ctx context.Context, event *pb.TranscriptEvent, messageBody string) error {
	switch payload := event.GetEventPayload().(type) {
	case *pb.TranscriptEvent_Segment:
		s.logger.Info().Msgf("processSegmentEvent message body %v", messageBody)
		err := s.processSegmentEvent(ctx, event, payload.Segment)
		if err != nil {
			// Swallow duplicate-insert errors so SQS doesn't retry
			var tce *dynamodbTypes.TransactionCanceledException
			if errors.As(err, &tce) && strings.Contains(err.Error(), "ConditionalCheckFailed") {
				s.logger.Warn().Msgf("Duplicate segment detected, skipping. Message: %v, err: %v", messageBody, err)
				return nil
			}
			s.logger.Error().Err(err).Msg("Failed to process segment")
			return err
		}
		return nil
	case *pb.TranscriptEvent_EndOfCall:
		return s.processEndOfCallEvent(ctx, event)
	default:
		return errors.New("unknown event payload type")
	}
}

func (s *Service) processSegmentEvent(ctx context.Context, event *pb.TranscriptEvent, segment *pb.Segment) error {
	err := s.transcriptionRepository.InsertSegment(ctx, event, segment)
	if err != nil {
		return err
	}

	s.logger.Debug().
		Str("session_id", event.GetSessionId()).
		Str("segment_id", event.GetSegmentId()).
		Uint64("sequence", segment.GetSequence()).
		Msg("Successfully processed segment event")

	return nil
}

// generateSignedURL generates a presigned S3 URL for downloading the transcript.
func (s *Service) generateSignedURL(ctx context.Context, s3Key string) (string, error) {
	// Use the objectStore's PresignGetObject method
	return s.objectStore.GetSignedURL(ctx, s.transcriptionsBucketName, s3Key, SignedURLExpiration)
}
