package callhandler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"exiro.ai/application/models/pb"
	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/utils/conc"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

const (
	sttResponseTimerDuration = 1 * time.Second
	ttsMaxConcurrent         = 5
)

// processSTTResponsesWithAgent handles the coordination between STT responses and agent processing
// It manages response accumulation, timing, and the interaction between STT and agent services.
func (s *Service) processSTTResponsesWithAgent(
	ctx context.Context,
	c *websocket.Conn,
	stt clientTypes.STTClient,
	agentId string,
	sessionId string,
	initialResponse string,
) error {
	accumulatedResponse := initialResponse
	timer := time.NewTimer(sttResponseTimerDuration)
	defer timer.Stop()

	agentResponseCh, agentErrorCh, agentCancel := s.startAgentProcessingPipeline(ctx, agentId, sessionId, accumulatedResponse)
	defer agentCancel()

	for {
		select {
		case err := <-agentErrorCh:
			s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Agent pipeline failed")
			return err

		case <-ctx.Done():
			return ctx.Err()

		case <-timer.C:
			// Timer expired, wait for agent response
			s.logger.Debug().Str("session_id", sessionId).Msg("Timer expired, waiting for agent response")
			select {
			case agentResponse := <-agentResponseCh:
				s.logger.Debug().Str("session_id", sessionId).Msgf("Sending Agent response to client: %+v", agentResponse)
				if err := s.generateAudioAndSendToUser(ctx, c, agentResponse.Message, agentResponse.Language); err != nil {
					s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Failed to handle agent response")
					return err
				}
				return nil
			case err := <-agentErrorCh:
				s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Agent pipeline failed")
				return err
			case <-ctx.Done():
				return ctx.Err()
			}

		case newResponse := <-stt.Responses():
			// New message received, cancel current pipeline and accumulate response
			s.logger.Debug().Str("session_id", sessionId).Msgf("New message received = [%s]", newResponse)
			agentCancel()
			accumulatedResponse += " " + newResponse
			s.logger.Debug().Str("session_id", sessionId).Msgf("Accumulated response: %s", accumulatedResponse)

			// Reset timer and start new pipeline
			timer.Reset(sttResponseTimerDuration)
			agentResponseCh, agentErrorCh, agentCancel = s.startAgentProcessingPipeline(ctx, agentId, sessionId, accumulatedResponse)
		}
	}
}

// startAgentProcessingPipeline initializes and starts the agent processing pipeline
// It returns channels for agent responses and errors, along with a cancel function.
func (s *Service) startAgentProcessingPipeline(ctx context.Context, agentId, sessionId, input string) (chan types.AgentResponse, chan error, context.CancelFunc) {
	agentCtx, agentCancel := context.WithCancel(ctx)
	agentResponseCh := make(chan types.AgentResponse, 1)
	agentErrorCh := make(chan error, 1)

	go func() {
		messages := []types.AgentMessage{
			{Role: types.MessageRoleUser, Content: input},
		}
		response, err := s.agentService.Invoke(agentCtx, agentId, sessionId, messages)
		if err != nil {
			agentErrorCh <- err
			return
		}
		agentResponseCh <- response
	}()

	return agentResponseCh, agentErrorCh, agentCancel
}

// audioSegment represents audio data with metadata.
type audioSegment struct {
	Text  string
	Audio []byte
}

func (s *Service) generateAudioAndSendToUser(
	ctx context.Context,
	c *websocket.Conn,
	response string,
	language types.AgentLanguage,
) error {
	s.logger.Info().Msgf("Processing agent response: %s", response)

	// Segment the text for parallel processing
	segments := s.textSegmenter.SegmentText(response)

	if len(segments) == 0 {
		s.logger.Warn().Msg("No segments to process after text segmentation")
		return nil
	}

	s.logger.Debug().Int("segment_count", len(segments)).Msg("Text segmented for parallel audio generation")

	if len(segments) == 1 {
		return s.generateAndSendSingleSegment(ctx, c, segments[0], language)
	}

	// Use parallel processing for multiple segments
	return s.generateAndSendMultipleSegments(ctx, c, segments, language)
}

// generateAndSendSingleSegment handles the simple case of single segment processing.
func (s *Service) generateAndSendSingleSegment(
	ctx context.Context,
	c *websocket.Conn,
	text string,
	language types.AgentLanguage,
) error {
	audioData, err := s.ttsHandler.GenerateAudio(ctx, text, pb.AudioEncoding_AUDIO_ENCODING_MP3, language, 0)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error generating audio from TTS service")
		return err
	}

	audioProtoMsg := &pb.AgentCallEvent{
		Event: &pb.AgentCallEvent_Audio{
			Audio: &pb.Audio{
				Audio:    audioData,
				Encoding: pb.AudioEncoding_AUDIO_ENCODING_MP3,
			},
		},
	}
	audioProtoBytes, _ := proto.Marshal(audioProtoMsg)

	if err := c.WriteMessage(websocket.BinaryMessage, audioProtoBytes); err != nil {
		s.logger.Error().Err(err).Msg("Error sending audio message to client")
		return err
	}

	return nil
}

// generateAndSendMultipleSegments handles parallel audio generation with immediate streaming.
func (s *Service) generateAndSendMultipleSegments(
	ctx context.Context,
	c *websocket.Conn,
	segments []string,
	language types.AgentLanguage,
) error {
	// Create executor with configuration
	config := conc.ExecutorConfig{
		MaxConcurrent: ttsMaxConcurrent,
		BufferSize:    len(segments),
		ErrorHandling: conc.StopOnError,   // Stop if any segment fails
		StreamingMode: conc.StreamInOrder, // Stream in original text order
	}

	executor := conc.NewExecutor[audioSegment](config)

	// Add audio generation functions for each segment
	for _, segment := range segments {
		// Capture variables for closure
		text := segment
		executor = executor.WithExecuteFunc(func(ctx context.Context) (audioSegment, error) {
			s.logger.Debug().Str("text", text).Msg("Generating audio for segment")

			audioData, err := s.ttsHandler.GenerateAudio(ctx, text, pb.AudioEncoding_AUDIO_ENCODING_MP3, language, 0)
			if err != nil {
				return audioSegment{}, fmt.Errorf("failed to generate audio: %w", err)
			}

			return audioSegment{Text: text, Audio: audioData}, nil
		})
	}

	// Execute all functions concurrently
	resultCh, err := executor.Run(ctx)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error starting concurrent execution")
		return fmt.Errorf("failed to start concurrent audio generation: %w", err)
	}

	// Stream results as they become available
	return s.streamAudioResults(ctx, c, resultCh)
}

// streamAudioResults streams audio results as they become available from the concurrent executor.
func (s *Service) streamAudioResults(
	_ context.Context,
	c *websocket.Conn,
	resultCh <-chan conc.TaskResult[audioSegment],
) error {
	totalBytes := 0
	segmentCount := 0

	for result := range resultCh {
		// Handle any errors from audio generation
		if result.Error != nil {
			s.logger.Error().Err(result.Error).
				Int("position", result.Position).
				Msg("Failed to generate audio for segment")
			return fmt.Errorf("failed to generate audio for segment %d: %w", result.Position, result.Error)
		}

		// Validate audio data
		if result.Result.Audio == nil {
			s.logger.Error().
				Int("position", result.Position).
				Msg("Audio segment is nil, skipping")
			continue
		}

		audioProtoMsg := &pb.AgentCallEvent{
			Event: &pb.AgentCallEvent_Audio{
				Audio: &pb.Audio{
					Audio:    result.Result.Audio,
					Encoding: pb.AudioEncoding_AUDIO_ENCODING_MP3,
				},
			},
		}
		audioProtoBytes, _ := proto.Marshal(audioProtoMsg)

		if err := c.WriteMessage(websocket.BinaryMessage, audioProtoBytes); err != nil {
			s.logger.Error().Err(err).
				Int("position", result.Position).
				Msg("Failed to send audio segment")
			return fmt.Errorf("failed to send audio segment %d: %w", result.Position, err)
		}

		totalBytes += len(result.Result.Audio)
		segmentCount++

		s.logger.Info().
			Int("position", result.Position).
			Int("audio_bytes", len(result.Result.Audio)).
			Str("text", result.Result.Text).
			Msg("Successfully sent audio segment immediately when ready")
	}

	s.logger.Info().
		Int("total_segments", segmentCount).
		Int("total_bytes", totalBytes).
		Msg("Successfully sent all audio segments with immediate streaming")

	return nil
}

// readUserMessages reads messages from the user and sends them to the stt service.
func (s *Service) readUserMessages(
	ctx context.Context,
	c *websocket.Conn,
	stt clientTypes.STTClient,
	agentId string,
	sessionId string,
) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := c.ReadMessage()
			if err != nil {
				s.logger.Error().Err(err).
					Str("session_id", sessionId).
					Str("agent_id", agentId).
					Msg("Failed to read message")
				return fmt.Errorf("failed to read message: %w", err)
			}

			protoMsg := &pb.AgentCallEvent{}
			if err := proto.Unmarshal(message, protoMsg); err != nil {
				s.logger.Error().Err(err).
					Str("session_id", sessionId).
					Str("agent_id", agentId).
					Msg("Failed to unmarshal message")
				return fmt.Errorf("failed to unmarshal message: %w", err)
			}

			switch protoMsg.GetEvent().(type) {
			case *pb.AgentCallEvent_Audio:
				if err := stt.SendAudio(ctx, protoMsg.GetAudio().GetAudio()); err != nil {
					s.logger.Error().Err(err).
						Str("session_id", sessionId).
						Str("agent_id", agentId).
						Msg("Failed to send audio")
					return fmt.Errorf("failed to send audio: %w", err)
				}
			default:
				return errors.New("unknown event type received")
			}
		}
	}
}
