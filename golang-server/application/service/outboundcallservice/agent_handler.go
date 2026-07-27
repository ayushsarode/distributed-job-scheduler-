package outboundcallservice

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"exiro.ai/application/models/pb"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/xcontext"
	"exiro.ai/utils/conc"
	"github.com/gorilla/websocket"
)

const (
	responseTimerDuration     = 800 * time.Millisecond
	MaxConcurrentExecutions    = 5
	CutCallDelay              = 3 * time.Second
	DisconnectTimeout         = 5 * time.Second
)

// processSTTResponsesWithAgent handles the coordination between STT responses and agent processing
// It manages response accumulation, timing, and the interaction between STT and agent services.
func (s *Service) processSTTResponsesWithAgent(
	ctx context.Context,
	conn *websocket.Conn,
	stt repositoryTypes.STTClient,
	agentId string,
	sessionId string,
	streamSid string,
	initialResponse string,
	transcriptionCh chan<- transcriptionEvent,
	ownerId string,
	cutCallChan chan<- types.AgentResponse,
	sampleRate int,
	interruptedContext string,
	tracker *playbackTracker,
) error {
	accumulatedResponse := initialResponse
	timer := time.NewTimer(responseTimerDuration)
	defer timer.Stop()

	// Send initial human response immediately
	s.sendHumanTranscription(transcriptionCh, sessionId, ownerId, initialResponse)

	messages := createMessageRequestForAgent(accumulatedResponse, interruptedContext)
	agentResponseCh, agentErrorCh, agentCancel := s.startAgentProcessingPipeline(ctx, agentId, sessionId, messages)
	defer agentCancel()

	for {
		select {
		case err := <-agentErrorCh:
			s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Agent pipeline failed")
			return err

		case <-ctx.Done():
			return ctx.Err()

		case <-timer.C:
			// Timer expired, wait for agent response
			if err := s.handleTimerExpiration(ctx, conn, sessionId, streamSid, ownerId, transcriptionCh, cutCallChan, agentResponseCh, agentErrorCh, sampleRate, tracker); err != nil {
				return err
			}
			return nil

		case newResponse := <-stt.Responses():
			// New message received, cancel current pipeline and accumulate response
			s.logger.Debug().Ctx(ctx).Str("session_id", sessionId).Msgf("New message received = [%s]", newResponse)
			agentCancel()

			// Send the new human message immediately
			s.sendHumanTranscription(transcriptionCh, sessionId, ownerId, newResponse)

			accumulatedResponse += " " + newResponse
			s.logger.Debug().Ctx(ctx).Str("session_id", sessionId).Msgf("Accumulated response: %s", accumulatedResponse)

			// Reset timer and start new pipeline
			// We only pass interruptedContext on the first pipeline start, if user keeps talking,
			// it's part of the same turn, but we still pass it since it hasn't been sent yet.
			timer.Reset(1 * time.Second)
			messages = createMessageRequestForAgent(accumulatedResponse, interruptedContext)
			agentResponseCh, agentErrorCh, agentCancel = s.startAgentProcessingPipeline(ctx, agentId, sessionId, messages)
		}
	}
}

func createMessageRequestForAgent(accumulatedResponse string, interruptedContext string) []types.AgentMessage {
	messages := []types.AgentMessage{
		{Role: types.MessageRoleUser, Content: accumulatedResponse},
	}
	if interruptedContext != "" {
		messages = append([]types.AgentMessage{{Role: types.MessageRoleSystem, Content: interruptedContext}}, messages...)
	}
	return messages
}

func (s *Service) sendHumanTranscription(transcriptionCh chan<- transcriptionEvent, sessionId, ownerId, text string) {
	transcriptionCh <- transcriptionEvent{
		sessionID: sessionId,
		ownerID:   ownerId,
		text:      text,
		speaker:   pb.Speaker_HUMAN,
		language:  "en", // TODO: This is hardcoded for now, we need to get the language from the STT response
	}
}

func (s *Service) handleTimerExpiration(
	ctx context.Context,
	conn *websocket.Conn,
	sessionId string,
	streamSid string,
	ownerId string,
	transcriptionCh chan<- transcriptionEvent,
	cutCallChan chan<- types.AgentResponse,
	agentResponseCh <-chan types.AgentResponse,
	agentErrorCh <-chan error,
	sampleRate int,
	tracker *playbackTracker,
) error {
	s.logger.Debug().Ctx(ctx).Str("session_id", sessionId).Msg("Timer expired, waiting for agent response")

	select {
	case agentResponse := <-agentResponseCh:
		s.logger.Debug().Ctx(ctx).Str("session_id", sessionId).Msgf("Agent response received: %s", agentResponse.Message)

		if err := s.handleAgentResponse(ctx, conn, agentResponse, sessionId, streamSid, sampleRate, tracker); err != nil {
			s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Failed to handle agent response")
			return err
		}

		// Send AI agent transcription segment after processing
		transcriptionCh <- transcriptionEvent{
			sessionID: sessionId,
			ownerID:   ownerId,
			text:      agentResponse.Message,
			speaker:   pb.Speaker_AI_AGENT,
			language:  string(agentResponse.Language),
		}

		if agentResponse.CutCall {
			cutCallChan <- agentResponse
		}
		return nil

	case err := <-agentErrorCh:
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Agent pipeline failed")
		return err

	case <-ctx.Done():
		return ctx.Err()
	}
}

// startAgentProcessingPipeline initializes and starts the agent processing pipeline
// It returns channels for agent responses and errors, along with a cancel function.
func (s *Service) startAgentProcessingPipeline(ctx context.Context, agentId, sessionId string, messages []types.AgentMessage) (chan types.AgentResponse, chan error, context.CancelFunc) {
	agentCtx, agentCancel := context.WithCancel(ctx)
	agentResponseCh := make(chan types.AgentResponse, 1)
	agentErrorCh := make(chan error, 1)

	go func() {
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

// handleAgentResponse processes the agent response for outbound calls with parallel audio generation
// It intelligently segments text, generates audio concurrently, and streams in order.
func (s *Service) handleAgentResponse(
	ctx context.Context,
	conn *websocket.Conn,
	response types.AgentResponse,
	sessionId string,
	streamSid string,
	sampleRate int,
	tracker *playbackTracker,
) error {
	s.logger.Info().Ctx(ctx).Str("session_id", sessionId).Str("stream_sid", streamSid).Msgf("Processing agent response: %s", response.Message)

	// Segment the text for parallel processing
	segments := s.textSegmenter.SegmentText(response.Message)

	if len(segments) == 0 {
		s.logger.Warn().Ctx(ctx).Str("session_id", sessionId).Msg("No segments to process after text segmentation")
		return nil
	}

	// Add segments to tracker if provided
	if tracker != nil {
		for _, seg := range segments {
			tracker.addSegment(seg)
		}
	}

	s.logger.Debug().Ctx(ctx).Str("session_id", sessionId).Int("segment_count", len(segments)).Msg("Text segmented for parallel audio generation")

	// For single segment or very short text, use the original synchronous approach
	if len(segments) == 1 {
		return s.generateAndSendSingleSegment(ctx, conn, segments[0], response.Language, sessionId, streamSid, sampleRate, tracker, 0)
	}

	// Use parallel processing for multiple segments
	return s.generateAndSendMultipleSegments(ctx, conn, segments, response.Language, sessionId, streamSid, sampleRate, tracker)
}

// generateAndSendSingleSegment handles the simple case of single segment processing.
func (s *Service) generateAndSendSingleSegment(
	ctx context.Context,
	conn *websocket.Conn,
	text string,
	language types.AgentLanguage,
	sessionId string,
	streamSid string,
	sampleRate int,
	tracker *playbackTracker,
	segmentIndex int,
) error {
	audioData, err := s.ttsHandler.GenerateAudio(ctx, text, s.providerConfig.GetAudioEncoding(), language, sampleRate)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Error generating mulaw audio from TTS service")
		return fmt.Errorf("failed to generate mulaw audio: %w", err)
	}

	base64AudioPayload := base64.StdEncoding.EncodeToString(audioData)

	if err := s.callProvider.SendAgentAudioResponse(ctx, conn, streamSid, base64AudioPayload, sessionId, len(audioData)); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Failed to send agent audio response via Twilio handler")
		return fmt.Errorf("failed to send agent audio response: %w", err)
	}

	// Send mark after audio segment is sent
	if tracker != nil {
		markName := fmt.Sprintf("audio-seg-%d", segmentIndex)
		if err := s.callProvider.SendMarkMessage(ctx, conn, streamSid, markName); err != nil {
			s.logger.Warn().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Failed to send segment mark message")
		}
	}

	return nil
}

// generateAndSendMultipleSegments handles parallel audio generation with immediate streaming.
func (s *Service) generateAndSendMultipleSegments(
	ctx context.Context,
	conn *websocket.Conn,
	segments []string,
	language types.AgentLanguage,
	sessionId string,
	streamSid string,
	sampleRate int,
	tracker *playbackTracker,
) error {
	// Create executor with configuration
	config := conc.ExecutorConfig{
		MaxConcurrent:    MaxConcurrentExecutions,
		BufferSize:       len(segments),
		ErrorHandling:    conc.StopOnError,   // Stop if any segment fails
		StreamingMode:    conc.StreamInOrder, // Stream in original text order
	}

	executor := conc.NewExecutor[audioSegment](config)

	// Add audio generation functions for each segment
	for _, segment := range segments {
		// Capture variables for closure
		text := segment
		executor = executor.WithExecuteFunc(func(ctx context.Context) (audioSegment, error) {
			s.logger.Debug().Ctx(ctx).Str("text", text).Msg("Generating audio for segment")

			audioData, err := s.ttsHandler.GenerateAudio(ctx, text, s.providerConfig.GetAudioEncoding(), language, sampleRate)
			if err != nil {
				return audioSegment{}, fmt.Errorf("failed to generate audio: %w", err)
			}

			return audioSegment{Text: text, Audio: audioData}, nil
		})
	}

	// Execute all functions concurrently
	resultCh, err := executor.Run(ctx)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Error starting concurrent execution")
		return fmt.Errorf("failed to start concurrent audio generation: %w", err)
	}

	// Stream results as they become available
	return s.streamAudioResults(ctx, conn, resultCh, sessionId, streamSid, tracker)
}

// streamAudioResults streams audio results as they become available from the concurrent executor.
func (s *Service) streamAudioResults(
	ctx context.Context,
	conn *websocket.Conn,
	resultCh <-chan conc.TaskResult[audioSegment],
	sessionId string,
	streamSid string,
	tracker *playbackTracker,
) error {
	totalBytes := 0
	segmentCount := 0

	for result := range resultCh {
		// Handle any errors from audio generation
		if result.Error != nil {
			s.logger.Error().Ctx(ctx).Err(result.Error).
				Str("session_id", sessionId).
				Int("position", result.Position).
				Msg("Failed to generate audio for segment")
			return fmt.Errorf("failed to generate audio for segment %d: %w", result.Position, result.Error)
		}

		// Validate audio data
		if result.Result.Audio == nil {
			s.logger.Error().
				Ctx(ctx).
				Str("session_id", sessionId).
				Int("position", result.Position).
				Msg("Audio segment is nil, skipping")
			continue
		}

		if err := s.sendAudioSegment(ctx, conn, streamSid, sessionId, result); err != nil {
			return err
		}

		// Send mark after audio segment is sent
		if tracker != nil {
			markName := fmt.Sprintf("audio-seg-%d", result.Position)
			if err := s.callProvider.SendMarkMessage(ctx, conn, streamSid, markName); err != nil {
				s.logger.Warn().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Failed to send segment mark message")
			}
		}

		totalBytes += len(result.Result.Audio)
		segmentCount++
	}

	s.logger.Info().
		Ctx(ctx).
		Str("session_id", sessionId).
		Str("stream_sid", streamSid).
		Int("total_segments", segmentCount).
		Int("total_bytes", totalBytes).
		Msg("Successfully sent all audio segments to Twilio with immediate streaming")

	return nil
}

func (s *Service) sendAudioSegment(
	ctx context.Context,
	conn *websocket.Conn,
	streamSid string,
	sessionId string,
	result conc.TaskResult[audioSegment],
) error {
	base64AudioPayload := base64.StdEncoding.EncodeToString(result.Result.Audio)

	if err := s.callProvider.SendAgentAudioResponse(
		ctx,
		conn,
		streamSid,
		base64AudioPayload,
		sessionId,
		len(result.Result.Audio),
	); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("session_id", sessionId).
			Int("position", result.Position).
			Msg("Failed to send audio segment via Twilio handler")
		return fmt.Errorf("failed to send audio segment %d: %w", result.Position, err)
	}

	s.logger.Info().
		Ctx(ctx).
		Str("session_id", sessionId).
		Int("position", result.Position).
		Int("audio_bytes", len(result.Result.Audio)).
		Str("text", result.Result.Text).
		Msg("Successfully sent audio segment immediately when ready")

	return nil
}

// greetUser greets the user by invoking the agent and sending the greeting audio through Twilio.
func (s *Service) greetUser(
	ctx context.Context,
	conn *websocket.Conn,
	agentId string,
	sessionId string,
	streamSid string,
	transcriptionCh chan<- transcriptionEvent,
	ownerId string,
	sampleRate int,
) error {
	// Should we add Hello to transcription?
	messages := []types.AgentMessage{
		{Role: types.MessageRoleUser, Content: "Hello"},
	}
	response, err := s.agentService.Invoke(ctx, agentId, sessionId, messages)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentId).Str("session_id", sessionId).Msg("Error getting greeting from agent")
		return fmt.Errorf("failed to get greeting from agent: %w", err)
	}

	s.logger.Info().Ctx(ctx).Str("agent_id", agentId).Str("session_id", sessionId).Msgf("Agent greeting: %s", response.Message)

	if err := s.handleAgentResponse(ctx, conn, response, sessionId, streamSid, sampleRate, nil); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Failed to handle greeting response")
		return fmt.Errorf("failed to send greeting: %w", err)
	}

	// Send AI agent greeting transcription segment after audio is sent
	transcriptionCh <- transcriptionEvent{
		sessionID: sessionId,
		ownerID:   ownerId,
		text:      response.Message,
		speaker:   pb.Speaker_AI_AGENT,
		language:  string(response.Language),
	}

	s.logger.Info().Ctx(ctx).Str("agent_id", agentId).Str("session_id", sessionId).Str("stream_sid", streamSid).Msg("Successfully sent greeting to user via Twilio")
	return nil
}

func (s *Service) cutCall(
	ctx context.Context,
	conn *websocket.Conn,
	sessionId string,
	streamSid string,
	agentId string,
	markChan <-chan string,
) error {
	s.logger.Info().Ctx(ctx).Str("session_id", sessionId).Msg("Cutting call - waiting for audio to finish")

	s.waitForAudioToFinish(ctx, conn, streamSid, sessionId, markChan)

	// TODO: Mark message should confirm that audio has finished playing but that was not the case in testing.
	// This is added as a temporary fix to ensure that the audio has finished playing.
	time.Sleep(CutCallDelay)

	disconnectCtx, cancel := context.WithTimeout(xcontext.GetAppContext(), DisconnectTimeout)
	defer cancel()

	if err := s.agentService.DisconnectAgent(disconnectCtx, agentId, sessionId); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("session_id", sessionId).
			Str("agent_id", agentId).
			Msg("Error disconnecting agent")
	}

	if conn != nil {
		if err := conn.Close(); err != nil {
			s.logger.Error().Ctx(ctx).Err(err).
				Str("session_id", sessionId).
				Msg("Error closing WebSocket connection")
		}
	}

	s.logger.Info().Ctx(ctx).Str("session_id", sessionId).Msg("Call cut successfully")
	return nil
}

func (s *Service) waitForAudioToFinish(
	ctx context.Context,
	conn *websocket.Conn,
	streamSid string,
	sessionId string,
	markChan <-chan string,
) {
	// Send mark to detect when audio finishes
	markName := "final-audio-" + sessionId

	if err := s.callProvider.SendMarkMessage(ctx, conn, streamSid, markName); err != nil {
		return
	}

	for {
		select {
		case receivedMark := <-markChan:
			if receivedMark == markName {
				s.logger.Info().Ctx(ctx).Msg("Audio finished playing")
				return
			}

		case <-ctx.Done():
			s.logger.Warn().Ctx(ctx).Msg("Context cancelled while waiting for audio")
			return
		}
	}
}