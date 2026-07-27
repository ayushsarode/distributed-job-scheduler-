package callhandler

import (
	"context"
	"time"

	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"github.com/gorilla/websocket"
)

// deleteSession removes a session from the activeSessions map.
func deleteSession(agentId, sessionId string) {
	activeSessionsMu.Lock()
	defer activeSessionsMu.Unlock()
	if sessions, exists := activeSessions[agentId]; exists {
		delete(sessions, sessionId)
	}
}

// Helper method to close websocket connection with proper error handling.
func (s *Service) closeWSWithError(c *websocket.Conn, code int, message string) {
	deadline := time.Now().Add(time.Second)
	err := c.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(code, message),
		deadline)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to send close message")
	}
}

// Updated cleanupResources to handle nil components.
func (s *Service) cleanupResources(
	ctx context.Context,
	c *websocket.Conn,
	stt clientTypes.STTClient,
	agentService types.AgentService,
	agentId string,
	sessionId string,
) {
	if c != nil {
		if err := c.Close(); err != nil {
			s.logger.Error().Err(err).Msg("Failed to close WebSocket connection")
		}
	}

	if stt != nil {
		stt.Disconnect(ctx)
	}

	if agentService != nil {
		if err := agentService.DisconnectAgent(ctx, agentId, sessionId); err != nil {
			s.logger.Error().Err(err).Str("agent_id", agentId).Str("session_id", sessionId).Msg("Failed to disconnect agent")
		}
	}

	// Clean up session from active sessions
	deleteSession(agentId, sessionId)
}
