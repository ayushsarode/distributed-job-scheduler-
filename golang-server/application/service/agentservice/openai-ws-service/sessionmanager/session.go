package sessionmanager

import (
	"sync"
	"time"

	"exiro.ai/application/service/types"
	"github.com/gorilla/websocket"
)

// session holds per-session WebSocket connection and mutable state.
// Guarded by mu for all access.
type session struct {
	mu             sync.Mutex
	conn           *websocket.Conn
	prevResponseID string
	model          string
	instructions   string
	language       types.AgentLanguage
	pendingCutCall bool
	cutCallMessage string
	agentID        string
	lastActiveAt   time.Time
	closed         bool
}

func newSession(conn *websocket.Conn, model, instructions, agentID string) *session {
	return &session{
		conn:         conn,
		model:        model,
		instructions: instructions,
		agentID:      agentID,
		language:     types.AgentLanguageEnglish,
		lastActiveAt: time.Now(),
	}
}

func (s *session) touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActiveAt = time.Now()
}

func (s *session) closeConn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	_ = s.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	_ = s.conn.Close()
}

func (s *session) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
