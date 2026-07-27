package openai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

const (
	sessionWaitTimeoutSeconds = 10
	audioInBufferSize         = 100
	eventsBufferSize          = 50
	silenceThreshold          = 0.8
	prefixPaddingMs           = 250
	silenceDurationMs         = 500
	sessionTemperature        = 0.8

	SessionTimeoutShort = 5 * time.Second
	SessionTimeoutLong  = 10 * time.Second
)

type OpenAIRealtimeService struct {
	logger *zerolog.Logger
}

func NewOpenAIRealtimeService(ctx context.Context) clientTypes.RealtimeAgentService {
	return &OpenAIRealtimeService{
		logger: zerolog.Ctx(ctx),
	}
}

// RealtimeClient handles the OpenAI Realtime API WebSocket connection.
type OpenAIRealtimeClient struct {
	conn           *websocket.Conn
	logger         *zerolog.Logger
	audioIn        chan []byte
	events         chan clientTypes.AgentEvent
	writeMu        sync.Mutex
	closed         bool
	sessionReady   chan struct{}
	sessionCreated bool
	audioConfig    clientTypes.AudioConfig
}

// NewRealtimeClient creates a new OpenAI Realtime API client.
func (s *OpenAIRealtimeService) Connect(ctx context.Context, agentId, sessionId string, instructions string, language types.AgentLanguage, opts ...clientTypes.RealtimeConnectOption) (clientTypes.RealtimeAgentClient, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY not set")
	}

	options := &clientTypes.RealtimeConnectOptions{
		AudioConfig: DefaultAudioConfig(),
	}

	// Apply options
	for _, opt := range opts {
		opt(options)
	}

	logger := zerolog.Ctx(ctx)
	model := "gpt-4o-realtime-preview-2025-06-03"

	logger.Info().Ctx(ctx).
		Str("agent_id", agentId).
		Str("session_id", sessionId).
		Str("model", model).
		Msg("Connecting to OpenAI Realtime API")

	conn, err := s.establishWebSocketConnection(ctx, logger, apiKey, model)
	if err != nil {
		return nil, err
	}

	client := s.createRealtimeClient(ctx, conn, logger, options.AudioConfig)

	if err := s.initializeAndStartSession(ctx, client, conn, logger, instructions, language, sessionId); err != nil {
		return nil, err
	}

	logger.Info().Ctx(ctx).Msg("OpenAI Realtime API client initialized")
	return client, nil
}

func (s *OpenAIRealtimeService) establishWebSocketConnection(ctx context.Context, logger *zerolog.Logger, apiKey, model string) (*websocket.Conn, error) {
	url := "wss://api.openai.com/v1/realtime?model=" + model
	header := http.Header{}
	header.Set("Authorization", "Bearer "+apiKey)
	header.Set("Openai-Beta", "realtime=v1")

	conn, resp, err := websocket.DefaultDialer.Dial(url, header)
	if resp != nil && resp.Body != nil {
		defer func() {
			if err := resp.Body.Close(); err != nil {
				s.logger.Error().Ctx(ctx).Err(err).Msg("Failed to close response body")
			}
		}()
	}
	if err != nil {
		if resp != nil {
			body, _ := io.ReadAll(resp.Body)
			logger.Error().Ctx(ctx).
				Int("status_code", resp.StatusCode).
				Str("response_body", string(body)).
				Err(err).
				Msg("Failed to connect to OpenAI Realtime API")
		} else {
			logger.Error().Ctx(ctx).
				Err(err).
				Msg("Failed to connect to OpenAI Realtime API (no response)")
		}
		return nil, fmt.Errorf("failed to connect to OpenAI Realtime API: %w", err)
	}

	logger.Info().Ctx(ctx).
		Str("remote_addr", conn.RemoteAddr().String()).
		Msg("Successfully connected to OpenAI Realtime API WebSocket")

	return conn, nil
}

func (s *OpenAIRealtimeService) createRealtimeClient(_ context.Context, conn *websocket.Conn, logger *zerolog.Logger, audioConfig clientTypes.AudioConfig) *OpenAIRealtimeClient {
	return &OpenAIRealtimeClient{
		conn:           conn,
		logger:         logger,
		audioIn:        make(chan []byte, audioInBufferSize),
		events:         make(chan clientTypes.AgentEvent, eventsBufferSize),
		closed:         false,
		sessionReady:   make(chan struct{}),
		sessionCreated: false,
		audioConfig:    audioConfig,
	}
}

func (s *OpenAIRealtimeService) initializeAndStartSession(ctx context.Context, client *OpenAIRealtimeClient, conn *websocket.Conn, logger *zerolog.Logger, instructions string, language types.AgentLanguage, sessionId string) error {
	logger.Info().Ctx(ctx).Msg("Sending session configuration to Realtime API")
	if err := client.initializeSession(ctx, instructions, language); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn().Ctx(ctx).Err(closeErr).Msg("Failed to close websocket connection")
		}
		logger.Error().Ctx(ctx).Err(err).Msg("Failed to initialize Realtime API session")
		return fmt.Errorf("failed to initialize session: %w", err)
	}

	logger.Info().Ctx(ctx).Msg("Session configuration sent to Realtime API, starting message handler")

	go func() {
		if err := client.readMessages(ctx); err != nil {
			logger.Error().Ctx(ctx).Err(err).Msg("Realtime API message reader exited with error")
		}
	}()

	logger.Info().Ctx(ctx).Str("session_id", sessionId).Msg("Waiting for Realtime API session confirmation...")
	if err := client.waitForSession(ctx, sessionWaitTimeoutSeconds*time.Second); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn().Ctx(ctx).Err(closeErr).Msg("Failed to close websocket connection")
		}
		logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Failed to wait for Realtime API session")
		return fmt.Errorf("failed to wait for session: %w", err)
	}

	logger.Info().Ctx(ctx).Str("session_id", sessionId).Msg("Triggering initial greeting from Realtime API")
	if err := client.triggerInitialResponse(ctx); err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Warn().Ctx(ctx).Err(closeErr).Msg("Failed to close websocket connection")
		}
		logger.Error().Ctx(ctx).Err(err).Str("session_id", sessionId).Msg("Failed to trigger initial response")
		return fmt.Errorf("failed to trigger initial response: %w", err)
	}

	return nil
}

func (c *OpenAIRealtimeClient) initializeSession(ctx context.Context, instructions string, language types.AgentLanguage) error {
	openAILanguage := c.mapToOpenAILanguageCode(language)

	inputAudioTranscription := map[string]any{
		"model": "whisper-1",
	}
	if openAILanguage != "" {
		inputAudioTranscription["language"] = openAILanguage
	}

	config := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"modalities":                []string{"audio", "text"},
			"instructions":              instructions,
			"voice":                     "cedar",
			"input_audio_format":        c.audioConfig.AudioFormat,
			"output_audio_format":       c.audioConfig.AudioFormat,
			"input_audio_transcription": inputAudioTranscription,
			"turn_detection": map[string]any{
				"type":                "server_vad",
				"threshold":           silenceThreshold,
				"prefix_padding_ms":   prefixPaddingMs,
				"silence_duration_ms": silenceDurationMs,
			},
			"temperature": sessionTemperature,
			"tools":       getTools(),
			"tool_choice": "auto",
		},
	}

	c.logger.Info().Ctx(ctx).
		Str("language", string(language)).
		Str("openai_language", openAILanguage).
		Msg("Configuring Realtime API session")

	c.writeMu.Lock()
	err := c.conn.WriteJSON(config)
	c.writeMu.Unlock()
	if err != nil {
		c.logger.Error().Ctx(ctx).Err(err).Msg("Failed to write session config")
		return err
	}
	return nil
}

// SendAudio sends PCM16 audio at 8kHz, upsamples to 24kHz for OpenAI.
func (c *OpenAIRealtimeClient) SendAudio(ctx context.Context, audio []byte) error {
	if c.closed {
		return errors.New("client is closed")
	}

	if !c.sessionCreated {
		select {
		case <-c.sessionReady:
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(SessionTimeoutShort):
			c.logger.Warn().Ctx(ctx).Msg("Timeout waiting for session")
		}
	}

	upsampledAudio := resampleAudio(audio, c.audioConfig.InputSampleRate, c.audioConfig.OutputSampleRate)
	base64Audio := base64.StdEncoding.EncodeToString(upsampledAudio)

	event := map[string]any{
		"type":  "input_audio_buffer.append",
		"audio": base64Audio,
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		c.writeMu.Lock()
		err := c.conn.WriteJSON(event)
		c.writeMu.Unlock()
		return err
	}
}

// AudioOutput returns audio from OpenAI (downsampled to 8kHz).
func (c *OpenAIRealtimeClient) AudioOutput() <-chan []byte {
	return c.audioIn
}

func (c *OpenAIRealtimeClient) readMessages(ctx context.Context) error {
	defer func() {
		close(c.audioIn)
		close(c.events)
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if shouldStopReading := c.isExpectedCloseError(ctx, err); shouldStopReading {
					return nil
				}
				return fmt.Errorf("failed to read WebSocket message: %w", err)
			}

			var event map[string]any
			if err := json.Unmarshal(message, &event); err != nil {
				return fmt.Errorf("failed to unmarshal realtime event: %w", err)
			}

			eventTypeStr, ok := event["type"].(string)
			if !ok || eventTypeStr == "" {
				return errors.New("realtime event missing type field")
			}

			eventType, _ := mapOpenAIEvent(eventTypeStr)
			if !c.handleEvent(ctx, eventType, event) {
				return nil
			}
		}
	}
}

func (c *OpenAIRealtimeClient) isExpectedCloseError(ctx context.Context, err error) bool {
	if c.closed {
		return true
	}
	if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		c.logger.Info().Ctx(ctx).Msg("Realtime API connection closed normally")
		return true
	}
	return false
}

// SubmitFunctionResult submits the result of a function call back to the Realtime API
// output is the result of the function call (will be JSON serialized).
func (c *OpenAIRealtimeClient) SubmitFunctionResult(ctx context.Context, callId string, output string) error {
	if c.closed {
		return errors.New("client is closed")
	}

	// create a conversation item with the function output
	createItemEvent := map[string]any{
		"type": "conversation.item.create",
		"item": map[string]any{
			"type":    "function_call_output",
			"call_id": callId,
			"output":  output,
		},
	}

	c.logger.Info().Ctx(ctx).
		Str("call_id", callId).
		Str("output", output).
		Msg("Submitting function result to Realtime API")

	c.writeMu.Lock()
	err := c.conn.WriteJSON(createItemEvent)
	c.writeMu.Unlock()
	if err != nil {
		c.logger.Error().Ctx(ctx).Err(err).Msg("Failed to create function output item")
		return err
	}

	responseEvent := map[string]any{
		"type": "response.create",
	}

	c.writeMu.Lock()
	err = c.conn.WriteJSON(responseEvent)
	c.writeMu.Unlock()
	if err != nil {
		c.logger.Error().Ctx(ctx).Err(err).Msg("Failed to trigger response after function call")
		return err
	}

	c.logger.Debug().Ctx(ctx).Msg("Successfully submitted function result")
	return nil
}

// TriggerInitialResponse triggers the AI to start speaking.
func (c *OpenAIRealtimeClient) triggerInitialResponse(ctx context.Context) error {
	if c.closed {
		return errors.New("client is closed")
	}

	select {
	case <-c.sessionReady:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(SessionTimeoutLong):
		return errors.New("timeout waiting for session")
	}

	event := map[string]any{
		"type": "response.create",
		"response": map[string]any{
			"modalities": []string{"audio", "text"},
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(event)
	c.writeMu.Unlock()
	return err
}

func (c *OpenAIRealtimeClient) waitForSession(ctx context.Context, timeout time.Duration) error {
	select {
	case <-c.sessionReady:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(timeout):
		return errors.New("timeout waiting for session")
	}
}

// InterruptResponse cancels the current response (barge-in).
func (c *OpenAIRealtimeClient) InterruptResponse(ctx context.Context) error {
	if c.closed {
		return errors.New("client is closed")
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(map[string]any{"type": "response.cancel"})
	c.writeMu.Unlock()
	return err
}

// UpdateLanguage updates the transcription language mid-call.
func (c *OpenAIRealtimeClient) UpdateLanguage(ctx context.Context, language types.AgentLanguage) error {
	if c.closed {
		return errors.New("client is closed")
	}

	openAILanguage := c.mapToOpenAILanguageCode(language)
	config := map[string]any{
		"type": "session.update",
		"session": map[string]any{
			"input_audio_transcription": map[string]any{
				"model":    "whisper-1",
				"language": openAILanguage,
			},
		},
	}

	c.writeMu.Lock()
	err := c.conn.WriteJSON(config)
	c.writeMu.Unlock()
	return err
}

func (c *OpenAIRealtimeClient) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func (c *OpenAIRealtimeClient) mapToOpenAILanguageCode(language types.AgentLanguage) string {
	switch language {
	case types.AgentLanguageEnglish:
		return "en"
	case types.AgentLanguageHindi:
		return "hi"
	default:
		return "en"
	}
}
