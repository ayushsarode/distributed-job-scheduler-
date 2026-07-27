package sessionmanager

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"exiro.ai/application/service/types"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

func TestManager_Get_NotFound(t *testing.T) {
	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{})
	handle, ok := m.Get("nonexistent")
	if ok || handle != nil {
		t.Errorf("Get(nonexistent) = (%v, %v), want (nil, false)", handle, ok)
	}
}

func TestManager_Delete_NotFound(t *testing.T) {
	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{})
	err := m.Delete("nonexistent")
	if err != nil {
		t.Errorf("Delete(nonexistent) = %v, want nil", err)
	}
}

func TestManager_Close_NotFound(t *testing.T) {
	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{})
	err := m.Close("nonexistent")
	if err != nil {
		t.Errorf("Close(nonexistent) = %v, want nil", err)
	}
}

func TestManager_GetOrCreate_ReturnsExistingSession(t *testing.T) {
	var upgrader websocket.Upgrader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	dialFunc := func(ctx context.Context, _ string, _ http.Header) (*websocket.Conn, error) {
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return conn, err
	}

	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{DialFunc: dialFunc})

	params := CreateSessionParams{
		SessionID:    "s1",
		Model:        "gpt-4o-mini",
		Instructions: "test",
		AgentID:      "a1",
		APIKey:       "test-key",
	}

	h1, err := m.GetOrCreate(context.Background(), params)
	if err != nil {
		t.Fatalf("GetOrCreate = %v", err)
	}
	if h1 == nil {
		t.Fatal("GetOrCreate returned nil handle")
	}

	h2, err := m.GetOrCreate(context.Background(), params)
	if err != nil {
		t.Fatalf("second GetOrCreate = %v", err)
	}
	if h2 == nil {
		t.Fatal("second GetOrCreate returned nil handle")
	}

	if h1.SessionID() != h2.SessionID() {
		t.Errorf("session IDs differ: %s vs %s", h1.SessionID(), h2.SessionID())
	}

	// Cleanup
	_ = m.Delete("s1")
}

//nolint:funlen // test setup
func TestManager_ConcurrentGetOrCreate_SameSessionID(t *testing.T) {
	var upgrader websocket.Upgrader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	dialFunc := func(ctx context.Context, _ string, _ http.Header) (*websocket.Conn, error) {
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return conn, err
	}

	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{DialFunc: dialFunc})

	params := CreateSessionParams{
		SessionID:    "concurrent-s1",
		Model:        "gpt-4o-mini",
		Instructions: "test",
		AgentID:      "a1",
		APIKey:       "test-key",
	}

	var wg sync.WaitGroup
	handles := make([]*SessionHandle, 10)
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			h, err := m.GetOrCreate(context.Background(), params)
			if err != nil {
				t.Errorf("GetOrCreate[%d] = %v", idx, err)
				return
			}
			handles[idx] = h
		}(i)
	}
	wg.Wait()

	for i, h := range handles {
		if h == nil {
			t.Errorf("handle[%d] is nil", i)
			continue
		}
		if h.SessionID() != "concurrent-s1" {
			t.Errorf("handle[%d].SessionID() = %s, want concurrent-s1", i, h.SessionID())
		}
	}

	_ = m.Delete("concurrent-s1")
}

func TestManager_Delete_ClosesSession(t *testing.T) {
	var upgrader websocket.Upgrader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	dialFunc := func(ctx context.Context, _ string, _ http.Header) (*websocket.Conn, error) {
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return conn, err
	}

	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{DialFunc: dialFunc})

	params := CreateSessionParams{
		SessionID:    "del-s1",
		Model:        "gpt-4o-mini",
		Instructions: "test",
		AgentID:      "a1",
		APIKey:       "test-key",
	}

	h, err := m.GetOrCreate(context.Background(), params)
	if err != nil {
		t.Fatalf("GetOrCreate = %v", err)
	}

	if err := m.Delete("del-s1"); err != nil {
		t.Errorf("Delete = %v", err)
	}

	_, ok := m.Get("del-s1")
	if ok {
		t.Error("Get after Delete returned true")
	}

	err = h.WriteEnvelope(map[string]any{"type": "test"})
	if err == nil {
		t.Error("WriteEnvelope after Delete should fail")
	}
}

//nolint:cyclop,funlen // multiple assertions
func TestSessionHandle_State(t *testing.T) {
	var upgrader websocket.Upgrader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	dialFunc := func(ctx context.Context, _ string, _ http.Header) (*websocket.Conn, error) {
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return conn, err
	}

	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{DialFunc: dialFunc})

	params := CreateSessionParams{
		SessionID:    "state-s1",
		Model:        "gpt-4o-mini",
		Instructions: "instr",
		AgentID:      "agent1",
		APIKey:       "test-key",
	}

	h, err := m.GetOrCreate(context.Background(), params)
	if err != nil {
		t.Fatalf("GetOrCreate = %v", err)
	}
	defer func() { _ = m.Delete("state-s1") }()

	if h.Model() != "gpt-4o-mini" {
		t.Errorf("Model() = %s", h.Model())
	}
	if h.Instructions() != "instr" {
		t.Errorf("Instructions() = %s", h.Instructions())
	}
	if h.Language() != types.AgentLanguageEnglish {
		t.Errorf("Language() = %v", h.Language())
	}

	h.SetLanguage(types.AgentLanguageHindi)
	if h.Language() != types.AgentLanguageHindi {
		t.Errorf("after SetLanguage: Language() = %v", h.Language())
	}

	h.SetPrevResponseID("resp_123")
	if h.PrevResponseID() != "resp_123" {
		t.Errorf("PrevResponseID() = %s", h.PrevResponseID())
	}

	h.SetPendingCutCall(true, "Goodbye!")
	if !h.PendingCutCall() || h.CutCallMessage() != "Goodbye!" {
		t.Errorf("PendingCutCall=%v, CutCallMessage=%s", h.PendingCutCall(), h.CutCallMessage())
	}
}

func TestManager_Sweeper_ClosesStaleSessions(t *testing.T) {
	var upgrader websocket.Upgrader
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:]
	dialFunc := func(ctx context.Context, _ string, _ http.Header) (*websocket.Conn, error) {
		conn, resp, err := websocket.DefaultDialer.DialContext(ctx, wsURL, nil)
		if resp != nil {
			_ = resp.Body.Close()
		}
		return conn, err
	}

	logger := zerolog.Nop()
	m := NewManager(&logger, ManagerConfig{
		DialFunc:            dialFunc,
		StaleSessionTimeout: 50 * time.Millisecond,
	})

	params := CreateSessionParams{
		SessionID:    "stale-s1",
		Model:        "gpt-4o-mini",
		Instructions: "test",
		AgentID:      "a1",
		APIKey:       "test-key",
	}

	h, err := m.GetOrCreate(context.Background(), params)
	if err != nil {
		t.Fatalf("GetOrCreate = %v", err)
	}
	if h == nil {
		t.Fatal("handle is nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.StartSweeper(ctx)

	time.Sleep(100 * time.Millisecond)
	m.sweepStaleSessions(ctx)

	_, ok := m.Get("stale-s1")
	if ok {
		t.Error("stale session should have been closed by sweeper")
	}

	cancel()
	m.Stop()
}
