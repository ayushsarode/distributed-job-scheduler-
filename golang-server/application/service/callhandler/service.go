package callhandler

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/utils/text"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

const (
	callHandlerCleanupTimeout = 5 * time.Second
	defaultSampleRate         = 16000
	defaultEncoding           = pb.AudioEncoding_AUDIO_ENCODING_LINEAR16
)

type Service struct {
	sttService    clientTypes.STTService
	ttsHandler    clientTypes.TTSHandler
	logger        *zerolog.Logger
	agentService  types.AgentService
	textSegmenter *text.Segmenter
}

var _ types.CallHandlerService = (*Service)(nil)

// NewService creates a new Service instance.
func NewService(
	ctx context.Context,
	sttService clientTypes.STTService,
	ttsHandler clientTypes.TTSHandler,
	agentService types.AgentService,
) *Service {
	return &Service{
		sttService:    sttService,
		ttsHandler:    ttsHandler,
		logger:        zerolog.Ctx(ctx),
		agentService:  agentService,
		textSegmenter: text.NewSegmenter(),
	}
}

var (
	maxSessionsPerAgent = 10
	sessionTimeout      = 10 * time.Minute

	// TODO: Use session repository to store sessions.
	activeSessionsMu sync.RWMutex
	activeSessions   = make(map[string]map[string]struct{}) // map[agentId]map[sessionId]struct{}
)

func (s *Service) CreateSession(ctx context.Context, agentId string) (string, error) {
	if err := s.agentService.ValidateAgent(ctx, agentId); err != nil {
		s.logger.Error().Err(err).Str("agent_id", agentId).Msg("Error validating agent")
		return "", err
	}

	activeSessionsMu.Lock() // TODO: This is a temporary solution to avoid race conditions.
	defer activeSessionsMu.Unlock()

	// Initialize sessions map for agent if it doesn't exist
	if _, exists := activeSessions[agentId]; !exists {
		activeSessions[agentId] = make(map[string]struct{})
	}

	// Check if max sessions limit is reached
	if len(activeSessions[agentId]) >= maxSessionsPerAgent {
		return "", xerrors.BadRequestError(ctx, fmt.Errorf("maximum number of concurrent sessions reached for agent %s", agentId))
	}

	// Generate new session ID
	sessionId := uuid.New().String()
	activeSessions[agentId][sessionId] = struct{}{}

	s.logger.Info().
		Str("agent_id", agentId).
		Str("session_id", sessionId).
		Int("active_sessions", len(activeSessions[agentId])).
		Msg("Created new session")

	return sessionId, nil
}

// HandleWSConnection handles the WebSocket connection.
func (s *Service) HandleWSConnection(ctx context.Context, c *websocket.Conn, agentId string, sessionId string) error {
	// Create a cancellable context with timeout
	ctx, cancel := context.WithTimeout(ctx, sessionTimeout)
	defer cancel()

	// Defer cleanup with timeout context for graceful shutdown
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), callHandlerCleanupTimeout)
		defer cleanupCancel()
		s.cleanupResources(cleanupCtx, c, nil, s.agentService, agentId, sessionId)
	}()

	// Validate that the session exists and is active
	if err := s.validateSession(ctx, c, agentId, sessionId); err != nil {
		return err
	}

	// Initialize STT clients
	stt, err := s.initializeClients(ctx, c)
	if err != nil {
		s.closeWSWithError(c, websocket.CloseInternalServerErr, "Failed to initialize services")
		return err
	}

	userMessageErrorChannel := make(chan error, 1)

	// Start message reading in a goroutine
	go func() {
		if err := s.readUserMessages(ctx, c, stt, agentId, sessionId); err != nil {
			userMessageErrorChannel <- err
		}
	}()

	/*
		TODO:
		 1. Handle session timeout
		 2. Remind user if they are not talking for a while
	*/

	err = s.greetUser(ctx, c, agentId, sessionId)
	if err != nil {
		s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Error greeting user")
		s.closeWSWithError(c, websocket.CloseInternalServerErr, "Failed to greet user")
		return err
	}

	return s.handleSessionLoop(ctx, c, stt, agentId, sessionId, userMessageErrorChannel)
}

func (s *Service) validateSession(ctx context.Context, c *websocket.Conn, agentId string, sessionId string) error {
	activeSessionsMu.RLock()
	sessions, exists := activeSessions[agentId]
	if !exists || sessions == nil {
		activeSessionsMu.RUnlock()
		s.logger.Error().Str("session_id", sessionId).Msg("No active sessions found for agent")
		s.closeWSWithError(c, websocket.CloseInvalidFramePayloadData, "Invalid session")
		return xerrors.BadRequestError(ctx, errors.New("invalid session"))
	}

	_, sessionExists := sessions[sessionId]
	activeSessionsMu.RUnlock()

	if !sessionExists {
		s.logger.Error().Str("session_id", sessionId).Msg("Session ID not found in active sessions")
		s.closeWSWithError(c, websocket.CloseInvalidFramePayloadData, "Invalid session")
		return xerrors.BadRequestError(ctx, errors.New("invalid session"))
	}

	return nil
}

func (s *Service) handleSessionLoop(
	ctx context.Context,
	c *websocket.Conn,
	stt clientTypes.STTClient,
	agentId string,
	sessionId string,
	userMessageErrorChannel chan error,
) error {
	for {
		select {
		case <-ctx.Done():
			return s.handleContextDone(ctx, c, sessionId)
		case err := <-userMessageErrorChannel:
			s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Error in websocket communication")
			s.closeWSWithError(c, websocket.CloseInternalServerErr, "Internal error occurred")
			return err
		case response := <-stt.Responses():
			if err := s.handleSTTResponse(ctx, c, stt, agentId, sessionId, response); err != nil {
				return err
			}
		}
	}
}

func (s *Service) handleContextDone(ctx context.Context, c *websocket.Conn, sessionId string) error {
	if ctx.Err() == context.DeadlineExceeded {
		s.logger.Info().Str("session_id", sessionId).Msg("Session timeout reached")
		if err := c.Close(); err != nil {
			s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Failed to close websocket connection")
		}
		return xerrors.BadRequestError(ctx, errors.New("session timeout"))
	}
	s.logger.Info().Str("session_id", sessionId).Msg("Context cancelled")
	s.closeWSWithError(c, websocket.CloseNormalClosure, "Session terminated")
	return ctx.Err()
}

func (s *Service) handleSTTResponse(
	ctx context.Context,
	c *websocket.Conn,
	stt clientTypes.STTClient,
	agentId string,
	sessionId string,
	response string,
) error {
	s.logger.Debug().Str("session_id", sessionId).Msgf("Received STT response: %s", response)

	s.logger.Debug().Str("session_id", sessionId).Msg("Sending cleared message to client")
	if err := s.sendClearedMessage(ctx, c); err != nil {
		s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Error sending cleared message to client")
		return err
	}

	return s.processSTTResponsesWithAgent(ctx, c, stt, agentId, sessionId, response)
}

func (s *Service) greetUser(ctx context.Context, c *websocket.Conn, agentId string, sessionId string) error {
	// TODO: Check if this requires special handling from agent-service
	messages := []types.AgentMessage{
		{Role: types.MessageRoleUser, Content: "Hello"},
	}
	response, err := s.agentService.Invoke(ctx, agentId, sessionId, messages)
	if err != nil {
		return err
	}

	return s.generateAudioAndSendToUser(ctx, c, response.Message, response.Language)
}
