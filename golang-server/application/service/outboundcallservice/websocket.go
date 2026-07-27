package outboundcallservice

import (
	"context"
	"errors"
	"fmt"
	"strings"

	xerrors "exiro.ai/application/errors"
	agentutils "exiro.ai/application/service/agentservice/agentutils"
	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// handleStandardWSConnection handles the stt/tts WebSocket connection.
func (s *Service) handleStandardWSConnection(ctx context.Context, conn *websocket.Conn, agentId string, sessionId string, jobItemId string) error {
	if err := s.validateStandardWSParams(ctx, conn, agentId, sessionId, jobItemId); err != nil {
		return err
	}

	s.logger.Info().Str("agent_id", agentId).Str("session_id", sessionId).Str("job_item_id", jobItemId).Msg("Handling Twilio WebSocket connection")

	sampleRate := s.providerConfig.GetSampleRate(agentId)

	tenantId, transcriptionCh, err := s.setupStandardTranscription(ctx, jobItemId, sessionId)
	if err != nil {
		return err
	}

	defer func() {
		s.logger.Debug().Str("session_id", sessionId).Msg("Transcription session closed")
	}()

	stt, err := s.initializeSTTClient(ctx, conn, sampleRate)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to initialize STT client")
		if closeErr := conn.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("Failed to close websocket connection")
		}
		return xerrors.BadRequestError(ctx, fmt.Errorf("unable to connect to STT service: %w", err))
	}

	userMessageErrorChannel, streamSidChannel, markChan, cutCallChan := s.setupStandardWSChannels(ctx, conn, stt, agentId)

	streamSid, err := s.waitForStandardStreamSid(ctx, agentId, streamSidChannel, userMessageErrorChannel, conn)
	if err != nil {
		return err
	}

	err = s.greetUser(ctx, conn, agentId, sessionId, streamSid, transcriptionCh, tenantId, sampleRate)
	if err != nil {
		s.logger.Error().Err(err).Str("agent_id", agentId).Msg("Error greeting user")
		return err
	}

	return s.processStandardWSMessages(ctx, conn, stt, agentId, sessionId, streamSid, tenantId, transcriptionCh, userMessageErrorChannel, markChan, cutCallChan, sampleRate)
}

func (s *Service) validateStandardWSParams(ctx context.Context, conn *websocket.Conn, agentId, sessionId, jobItemId string) error {
	if conn == nil {
		return xerrors.BadRequestError(ctx, errors.New("websocket connection is nil"))
	}
	if agentId == "" {
		return xerrors.BadRequestError(ctx, errors.New("agent ID cannot be empty"))
	}
	if sessionId == "" {
		return xerrors.BadRequestError(ctx, errors.New("session ID cannot be empty"))
	}
	if jobItemId == "" {
		return xerrors.BadRequestError(ctx, errors.New("job item ID cannot be empty"))
	}
	return nil
}

func (s *Service) setupStandardTranscription(ctx context.Context, jobItemId, sessionId string) (string, chan<- transcriptionEvent, error) {
	jobItemUUID, err := uuid.Parse(jobItemId)
	if err != nil {
		s.logger.Error().Err(err).Str("job_item_id", jobItemId).Msg("Invalid job item ID format")
		return "", nil, xerrors.BadRequestError(ctx, fmt.Errorf("invalid job item ID format: %w", err))
	}

	tenantId, err := s.CallJobRepository.GetJobItemTenantById(ctx, jobItemUUID)
	if err != nil {
		s.logger.Error().Err(err).Str("job_item_id", jobItemId).Msg("Failed to get tenant ID for job item")
		return "", nil, xerrors.BadRequestError(ctx, fmt.Errorf("failed to get tenant ID: %w", err))
	}

	transcriptionCh := s.startTranscriptionSession(ctx, sessionId, tenantId.String())
	return tenantId.String(), transcriptionCh, nil
}

func (s *Service) setupStandardWSChannels(ctx context.Context, conn *websocket.Conn, stt clientTypes.STTClient, agentId string) (chan error, chan string, chan string, chan types.AgentResponse) {
	userMessageErrorChannel := make(chan error, 1)
	streamSidChannel := make(chan string, 1)
	//nolint:mnd
	markChan := make(chan string, 20)
	cutCallChan := make(chan types.AgentResponse, 1)

	go func() {
		if err := s.callProvider.ReadMessages(ctx, conn, stt, agentId, streamSidChannel, markChan); err != nil {
			userMessageErrorChannel <- err
		}
	}()

	return userMessageErrorChannel, streamSidChannel, markChan, cutCallChan
}

func (s *Service) waitForStandardStreamSid(ctx context.Context, agentId string, streamSidChannel chan string, userMessageErrorChannel chan error, conn *websocket.Conn) (string, error) {
	select {
	case streamSid := <-streamSidChannel:
		s.logger.Info().Str("agent_id", agentId).Str("stream_sid", streamSid).Msg("Received streamSid from Twilio")
		return streamSid, nil
	case err := <-userMessageErrorChannel:
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) || strings.Contains(err.Error(), "use of closed network connection") {
			s.logger.Info().Str("agent_id", agentId).Msg("WebSocket closed normally, exiting handler")
			return "", nil
		}
		s.logger.Error().Err(err).Str("agent_id", agentId).Msg("Error in websocket communication")
		if closeErr := conn.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("Failed to close websocket connection")
		}
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *Service) processStandardWSMessages(ctx context.Context, conn *websocket.Conn, stt clientTypes.STTClient, agentId, sessionId, streamSid, tenantId string, transcriptionCh chan<- transcriptionEvent, userMessageErrorChannel chan error, markChan chan string, cutCallChan chan types.AgentResponse, sampleRate int) error {
	tracker := newPlaybackTracker()

	for {
		select {
		case <-ctx.Done():
			return s.handleStandardContextDone(ctx, conn, agentId)
		case err := <-userMessageErrorChannel:
			s.logger.Error().Err(err).Str("agent_id", agentId).Msg("Error in websocket communication")
			return err
		case mark := <-markChan:
			s.processMarkMessage(mark, tracker)
		case response := <-stt.Responses():
			if err := s.handleUserResponse(ctx, conn, stt, agentId, sessionId, streamSid, tenantId, response, transcriptionCh, cutCallChan, sampleRate, tracker); err != nil {
				return err
			}
		case <-cutCallChan:
			s.logger.Info().Str("session_id", sessionId).Msg("Received cut call signal — initiating graceful disconnect")
			return s.cutCall(ctx, conn, sessionId, streamSid, agentId, markChan)
		}
	}
}

func (s *Service) handleStandardContextDone(ctx context.Context, conn *websocket.Conn, agentId string) error {
	if ctx.Err() == context.DeadlineExceeded {
		s.logger.Info().Str("agent_id", agentId).Msg("Session timeout reached")
		if err := conn.Close(); err != nil {
			s.logger.Error().Err(err).Msg("Failed to close websocket connection")
		}
	}
	return ctx.Err()
}

func (s *Service) processMarkMessage(mark string, tracker *playbackTracker) {
	if strings.HasPrefix(mark, "audio-seg-") {
		var index int
		if _, err := fmt.Sscanf(mark, "audio-seg-%d", &index); err == nil {
			tracker.updateLastDelivered(index)
		}
	}
}

func (s *Service) handleUserResponse(
	ctx context.Context,
	conn *websocket.Conn,
	stt clientTypes.STTClient,
	agentId, sessionId, streamSid, tenantId, response string,
	transcriptionCh chan<- transcriptionEvent,
	cutCallChan chan types.AgentResponse,
	sampleRate int,
	tracker *playbackTracker,
) error {
	s.logger.Debug().Str("agent_id", agentId).Msgf("Received STT response: %s", response)

	interruptedContext := tracker.getInterruptionContext()
	tracker.clear()

	s.logger.Debug().Str("agent_id", agentId).Msg("Sending cleared message to client")

	if err := s.callProvider.SendClearMessage(ctx, conn, streamSid); err != nil {
		s.logger.Error().Err(err).Str("agent_id", agentId).Msg("Error sending cleared message to client")
		return err
	}

	return s.processSTTResponsesWithAgent(ctx, conn, stt, agentId, sessionId, streamSid, response, transcriptionCh, tenantId, cutCallChan, sampleRate, interruptedContext, tracker)
}

func (s *Service) initializeSTTClient(
	ctx context.Context,
	c *websocket.Conn,
	sampleRate int,
) (clientTypes.STTClient, error) {
	stt, err := s.sttService.Connect(ctx, s.providerConfig.GetAudioEncoding(), sampleRate)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to create STT client")
		if closeErr := c.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("Failed to close websocket connection")
		}
		return nil, fmt.Errorf("unable to connect to STT service: %w", err)
	}
	s.logger.Debug().Msg("STT client initialized successfully")
	return stt, nil
}

func (s *Service) HandleWSConnection(
	ctx context.Context,
	conn *websocket.Conn,
	agentID, sessionID, jobItemID string,
) error {
	if agentutils.IsRTAgent(agentID) {
		s.logger.Info().
			Str("agent_id", agentID).
			Str("session_id", sessionID).
			Msg("Routing to Realtime API WS handler")
		sampleRate := s.providerConfig.GetSampleRate(agentID)
		return s.handleRealtimeWSConnection(ctx, conn, agentID, sessionID, jobItemID, sampleRate)
	}

	s.logger.Info().
		Str("agent_id", agentID).
		Str("session_id", sessionID).
		Msg("Routing to standard STT+TTS WS handler")
	return s.handleStandardWSConnection(ctx, conn, agentID, sessionID, jobItemID)
}
