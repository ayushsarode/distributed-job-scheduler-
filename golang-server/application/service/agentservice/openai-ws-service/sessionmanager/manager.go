package sessionmanager

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"exiro.ai/application/service/types"
	"github.com/gorilla/websocket"
	"github.com/openai/openai-go/responses"
	"github.com/rs/zerolog"
)

var errSessionClosed = errors.New("session closed")

const (
	defaultStaleSessionTimeout = 30 * time.Minute
	openAIResponsesWSURL       = "wss://api.openai.com/v1/responses"
)

// CreateSessionParams holds parameters for creating a new session.
type CreateSessionParams struct {
	SessionID    string
	Model        string
	Instructions string
	AgentID      string
	APIKey       string //nolint:gosec // env var name, not a secret value
}

// SessionHandle provides access to a session for the business layer.
// All methods are safe for concurrent use.
type SessionHandle struct {
	sessionID string
	session   *session
	logger    *zerolog.Logger
}

// PrevResponseID returns the previous response ID for chaining.
func (h *SessionHandle) PrevResponseID() string {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return h.session.prevResponseID
}

// SetPrevResponseID sets the previous response ID.
func (h *SessionHandle) SetPrevResponseID(id string) {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	h.session.prevResponseID = id
	h.session.lastActiveAt = time.Now()
}

// Language returns the current session language.
func (h *SessionHandle) Language() types.AgentLanguage {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return h.session.language
}

// SetLanguage sets the session language.
func (h *SessionHandle) SetLanguage(l types.AgentLanguage) {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	h.session.language = l
	h.session.lastActiveAt = time.Now()
}

// PendingCutCall returns whether a cut call is pending.
func (h *SessionHandle) PendingCutCall() bool {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return h.session.pendingCutCall
}

// SetPendingCutCall sets the pending cut call state.
func (h *SessionHandle) SetPendingCutCall(pending bool, message string) {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	h.session.pendingCutCall = pending
	h.session.cutCallMessage = message
	h.session.lastActiveAt = time.Now()
}

// CutCallMessage returns the cut call message.
func (h *SessionHandle) CutCallMessage() string {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return h.session.cutCallMessage
}

// Model returns the model string.
func (h *SessionHandle) Model() string {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return h.session.model
}

// Instructions returns the agent instructions.
func (h *SessionHandle) Instructions() string {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	return h.session.instructions
}

// Touch updates the last active timestamp.
func (h *SessionHandle) Touch() {
	h.session.touch()
}

// WriteEnvelope sends a response.create envelope. Serializes writes per session.
func (h *SessionHandle) WriteEnvelope(envelope map[string]any) error {
	h.session.mu.Lock()
	defer h.session.mu.Unlock()
	if h.session.closed {
		return errSessionClosed
	}
	return writeEnvelope(h.session.conn, envelope)
}

// ReadEnvelope reads the next stream event from the WebSocket.
func (h *SessionHandle) ReadEnvelope() (responses.ResponseStreamEventUnion, error) {
	h.session.mu.Lock()
	if h.session.closed {
		h.session.mu.Unlock()
		return responses.ResponseStreamEventUnion{}, errSessionClosed
	}
	conn := h.session.conn
	h.session.mu.Unlock()

	event, err := readEnvelope(conn)
	if err != nil {
		return responses.ResponseStreamEventUnion{}, err
	}
	h.session.touch()
	return event, nil
}

// SessionID returns the session ID.
func (h *SessionHandle) SessionID() string {
	return h.sessionID
}

// Manager manages WebSocket sessions for the OpenAI Responses API.
type Manager struct {
	sessions   sync.Map // map[string]*session
	logger     *zerolog.Logger
	staleAfter time.Duration
	stopCh     chan struct{}
	stopped    sync.Once
	dialFunc   DialFunc
}

// DialFunc connects to a WebSocket. Used for testing when a custom implementation is needed.
type DialFunc func(ctx context.Context, url string, header http.Header) (*websocket.Conn, error)

// ManagerConfig holds configuration for the Manager.
type ManagerConfig struct {
	StaleSessionTimeout time.Duration
	DialFunc            DialFunc // if set, used instead of default dial (for testing)
}

// NewManager creates a new session manager.
func NewManager(logger *zerolog.Logger, cfg ManagerConfig) *Manager {
	staleAfter := cfg.StaleSessionTimeout
	if staleAfter <= 0 {
		staleAfter = defaultStaleSessionTimeout
	}
	dialFunc := cfg.DialFunc
	if dialFunc == nil {
		dialFunc = defaultDial
	}
	return &Manager{
		logger:     logger,
		staleAfter: staleAfter,
		stopCh:     make(chan struct{}),
		dialFunc:   dialFunc,
	}
}

func defaultDial(ctx context.Context, u string, header http.Header) (*websocket.Conn, error) {
	conn, resp, err := websocket.DefaultDialer.DialContext(ctx, u, header)
	if resp != nil {
		_ = resp.Body.Close()
	}
	return conn, err
}

// Get returns an existing session by ID, or (nil, false) if not found.
func (m *Manager) Get(sessionID string) (*SessionHandle, bool) {
	v, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil, false
	}
	s := v.(*session)
	if s.isClosed() {
		m.sessions.Delete(sessionID)
		return nil, false
	}
	return &SessionHandle{sessionID: sessionID, session: s, logger: m.logger}, true
}

// GetOrCreate returns an existing session or creates a new one.
func (m *Manager) GetOrCreate(ctx context.Context, params CreateSessionParams) (*SessionHandle, error) {
	if handle, ok := m.Get(params.SessionID); ok {
		return handle, nil
	}

	conn, err := m.dialOpenAIWebSocket(ctx, params.APIKey)
	if err != nil {
		return nil, err
	}

	conn.SetReadLimit(readLimit)
	s := newSession(conn, params.Model, params.Instructions, params.AgentID)

	// Use LoadOrStore to handle concurrent GetOrCreate for the same sessionID.
	actual, loaded := m.sessions.LoadOrStore(params.SessionID, s)
	if loaded {
		s.closeConn()
		return &SessionHandle{sessionID: params.SessionID, session: actual.(*session), logger: m.logger}, nil
	}

	m.logger.Debug().
		Str("session_id", params.SessionID).
		Str("agent_id", params.AgentID).
		Msg("created new OpenAI WebSocket session")

	return &SessionHandle{sessionID: params.SessionID, session: s, logger: m.logger}, nil
}

// Delete removes the session from the registry and closes the connection.
func (m *Manager) Delete(sessionID string) error {
	v, loaded := m.sessions.LoadAndDelete(sessionID)
	if !loaded {
		return nil
	}
	s := v.(*session)
	s.closeConn()
	m.logger.Debug().Str("session_id", sessionID).Msg("deleted OpenAI WebSocket session")
	return nil
}

// Close is an alias for Delete for idempotent cleanup.
func (m *Manager) Close(sessionID string) error {
	return m.Delete(sessionID)
}

// StartSweeper starts the background stale-session cleanup loop.
func (m *Manager) StartSweeper(ctx context.Context) {
	go m.sweepLoop(ctx)
}

// Stop stops the sweeper.
func (m *Manager) Stop() {
	m.stopped.Do(func() { close(m.stopCh) })
}

func (m *Manager) dialOpenAIWebSocket(ctx context.Context, apiKey string) (*websocket.Conn, error) {
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+apiKey)
	return m.dialFunc(ctx, openAIResponsesWSURL, header)
}
