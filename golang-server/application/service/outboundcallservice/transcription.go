package outboundcallservice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"exiro.ai/application/models/pb"
	appcontext "exiro.ai/application/xcontext"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	TranscriptionChannelBufferSize = 100
	TranscriptionTimeout           = 10 * time.Minute
)

// startTranscriptionSession creates a channel for the session and starts the worker.
func (s *Service) startTranscriptionSession(ctx context.Context, sessionID string, ownerID string) chan<- transcriptionEvent {
	transcriptionCh := make(chan transcriptionEvent, TranscriptionChannelBufferSize) // buffered channel

	// Use the global for transcription worker that won't be cancelled when call ends
	// This ensures the worker can process the call end event even if the call context is cancelled
	transcriptionCtx := appcontext.GetAppContext()

	go s.transcriptionWorker(transcriptionCtx, sessionID, transcriptionCh)

	s.logger.Info().Ctx(ctx).Str("session_id", sessionID).Str("owner_id", ownerID).Msg("Started transcription session")
	return transcriptionCh
}

// transcriptionWorker processes transcription events for a session in order.
func (s *Service) transcriptionWorker(ctx context.Context, sessionID string, eventCh <-chan transcriptionEvent) {
	defer func() {
		s.logger.Debug().Ctx(ctx).Str("session_id", sessionID).Msg("Transcription worker stopped")
	}()

	var sequence uint64 = 0
	// Add timeout to prevent worker from running indefinitely
	timeout := time.NewTimer(TranscriptionTimeout) // Max call duration timeout
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			s.logger.Warn().Ctx(ctx).Str("session_id", sessionID).Msg("Transcription worker timeout - force stopping")
			return

		case event, ok := <-eventCh:
			if !ok {
				// Channel closed - transcription session ended
				// Call end event will be sent by Twilio webhook when call status is "completed"
				s.logger.Debug().Ctx(ctx).Str("session_id", sessionID).Msg("Transcription channel closed, worker exiting")
				return
			}

			// Send segment event
			sequence++
			s.sendSegmentToQueue(ctx, event.sessionID, event.ownerID, sequence, event.text, event.speaker, event.language)

			// Reset timeout on each event
			timeout.Reset(TranscriptionTimeout)
		}
	}
}

// calculateDeduplicationID generates a deduplication ID based on session ID and message content.
func (s *Service) calculateDeduplicationID(sessionID string, msgBytes []byte) string {
	hash := sha256.New()
	hash.Write([]byte(sessionID))
	hash.Write(msgBytes)
	return hex.EncodeToString(hash.Sum(nil))
}

// sendSegmentToQueue sends a segment event to the transcription queue.
func (s *Service) sendSegmentToQueue(ctx context.Context, sessionID string, ownerID string, sequence uint64, text string, speaker pb.Speaker, language string) {
	segmentId := uuid.New().String()
	event := &pb.TranscriptEvent{
		SessionId: sessionID,
		SegmentId: segmentId,
		TenantId:  ownerID,
		EventPayload: &pb.TranscriptEvent_Segment{
			Segment: &pb.Segment{
				Sequence:     sequence,
				Speaker:      speaker,
				Text:         text,
				Timestamp:    timestamppb.New(time.Now()), // TODO: Take input instead of current time
				LanguageCode: language,
			},
		},
	}

	msgBytes, err := protojson.Marshal(event)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionID).Msg("Failed to marshal transcription segment event")
		return
	}

	dedupID := s.calculateDeduplicationID(sessionID, msgBytes)
	if err := s.transcriptionQueue.PublishStringMessage(ctx, string(msgBytes), sessionID, dedupID); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionID).Msg("Failed to send transcription segment event")
	} else {
		s.logger.Debug().Ctx(ctx).Str("session_id", sessionID).Str("segment_id", segmentId).Uint64("sequence", sequence).Msg("Successfully sent transcription segment event")
	}
}

// sendCallEndToQueue sends a call end event to the transcription queue.
func (s *Service) sendCallEndToQueue(ctx context.Context, sessionID string, ownerID string, _ uint64, durationSeconds float32) {
	segmentId := uuid.New().String()
	event := &pb.TranscriptEvent{
		SessionId: sessionID,
		SegmentId: segmentId,
		TenantId:  ownerID,
		EventPayload: &pb.TranscriptEvent_EndOfCall{
			EndOfCall: &pb.CallEnd{
				FinalTimestamp:  timestamppb.New(time.Now()),
				DurationSeconds: durationSeconds,
			},
		},
	}

	msgBytes, err := protojson.Marshal(event)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionID).Msg("Failed to marshal call end event")
		return
	}

	dedupID := s.calculateDeduplicationID(sessionID, msgBytes)
	if err := s.transcriptionQueue.PublishStringMessage(ctx, string(msgBytes), sessionID, dedupID); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionID).Msg("Failed to send call end event")
	} else {
		s.logger.Info().Ctx(ctx).Str("session_id", sessionID).Str("segment_id", segmentId).Float32("duration", durationSeconds).Msg("Successfully sent call end event")
	}
}
