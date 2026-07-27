package api

import (
	"net/http"

	"exiro.ai/application/auth"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

const websocketBufferSize = 1024

var upgrader = websocket.Upgrader{
	ReadBufferSize:  websocketBufferSize,
	WriteBufferSize: websocketBufferSize,
	CheckOrigin: func(r *http.Request) bool {
		return true // Note: In production, you should implement proper origin checking
	},
}

// Handler for /public/call/{agent-id}/{session-id}.
func (r *restApi) callHandler(w http.ResponseWriter, req *http.Request) {
	agentID := chi.URLParam(req, "agent-id")
	sessionID := chi.URLParam(req, "session-id")
	if sessionID == "" {
		r.logger.Error().Msg("Session ID is required")
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}
	req = req.WithContext(auth.SetSessionId(req.Context(), sessionID))

	r.logger.Info().Msgf("CallHandler: agentID=%s, sessionID=%s", agentID, sessionID)

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to upgrade connection to WebSocket")
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			r.logger.Error().Err(err).Msg("Failed to close websocket connection")
		}
	}()

	// Call the service handler
	err = r.callHandlerService.HandleWSConnection(req.Context(), conn, agentID, sessionID)
	if err != nil {
		r.logger.Error().Err(err).
			Str("agentID", agentID).
			Str("sessionID", sessionID).
			Msg("Error handling call")
		return
	}
}
