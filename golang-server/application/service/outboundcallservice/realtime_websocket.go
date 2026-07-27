package outboundcallservice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/realtime/openai"
	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	AudioFormat       = "pcm16"
	ChannelBufferSize = 1
	FarewellDelay     = 5 * time.Second
)

// realtimeSessionContext holds all the context needed for a realtime WebSocket session.
type realtimeSessionContext struct {
	agentID        string
	sessionID      string
	jobItemID      uuid.UUID
	tenantID       uuid.UUID
	jobItem        entity.JobItem
	language       types.AgentLanguage
	instructions   string
	realtimeClient clientTypes.RealtimeAgentClient
}

// realtimeChannels holds all the channels used in the realtime event loop.
type realtimeChannels struct {
	userMessageError chan error
	streamSid        chan string
	mark             chan string
	cutCall          chan string
	transcription    chan<- transcriptionEvent
	audioOutput      <-chan []byte
	events           <-chan clientTypes.AgentEvent
}

// HandleRealtimeWSConnection handles WebSocket connection using OpenAI Realtime API.
func (s *Service) handleRealtimeWSConnection(ctx context.Context, conn *websocket.Conn, agentId string, sessionId string, jobItemId string, sampleRate int) error {
	// Validate inputs
	if err := s.validateRealtimeConnectionInputs(ctx, conn, agentId, sessionId, jobItemId); err != nil {
		return err
	}

	s.logger.Info().
		Str("agent_id", agentId).
		Str("session_id", sessionId).
		Str("job_item_id", jobItemId).
		Msg("Handling WebSocket connection with Realtime API")

	// Setup session context
	sessionCtx, err := s.setupRealtimeSessionContext(ctx, agentId, sessionId, jobItemId)
	if err != nil {
		return err
	}

	realtimeClient, err := s.initializeRealtimeClient(ctx, conn, sessionCtx, sampleRate)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := realtimeClient.Close(); closeErr != nil {
			s.logger.Warn().Err(closeErr).Msg("Failed to close realtime client")
		}
	}()

	sessionCtx.realtimeClient = realtimeClient

	channels := s.setupRealtimeChannels(ctx, conn, sessionCtx)

	// Wait for stream SID
	streamSid, err := s.waitForStreamSid(ctx, conn, agentId, channels.userMessageError, channels.streamSid)
	if err != nil {
		return err
	}

	return s.runRealtimeEventLoop(ctx, conn, sessionCtx, channels, streamSid)
}

// validateRealtimeConnectionInputs validates the input parameters for handleRealtimeWSConnection.
func (s *Service) validateRealtimeConnectionInputs(ctx context.Context, conn *websocket.Conn, agentId, sessionId, jobItemId string) error {
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

// setupRealtimeSessionContext gathers all required context for the realtime session.
func (s *Service) setupRealtimeSessionContext(ctx context.Context, agentId, sessionId, jobItemId string) (realtimeSessionContext, error) {
	var sessionCtx realtimeSessionContext
	sessionCtx.agentID = agentId
	sessionCtx.sessionID = sessionId

	jobItemUUID, err := uuid.Parse(jobItemId)
	if err != nil {
		s.logger.Error().Err(err).Str("job_item_id", jobItemId).Msg("Invalid job item ID format")
		return sessionCtx, xerrors.BadRequestError(ctx, fmt.Errorf("invalid job item ID format: %w", err))
	}
	sessionCtx.jobItemID = jobItemUUID

	tenantId, err := s.CallJobRepository.GetJobItemTenantById(ctx, jobItemUUID)
	if err != nil {
		s.logger.Error().Err(err).Str("job_item_id", jobItemId).Msg("Failed to get tenant ID for job item")
		return sessionCtx, xerrors.BadRequestError(ctx, fmt.Errorf("failed to get tenant ID: %w", err))
	}
	sessionCtx.tenantID = tenantId

	// Get job item
	jobItem, err := s.CallJobRepository.INTERNALGetJobItem(ctx, jobItemUUID)
	if err != nil {
		s.logger.Error().Err(err).Str("job_item_id", jobItemId).Msg("Failed to get job item for Realtime instructions")
		return sessionCtx, xerrors.BadRequestError(ctx, fmt.Errorf("failed to get job item: %w", err))
	}
	sessionCtx.jobItem = jobItem

	// Get language preference
	language, err := s.getLanguageFromJobItem(ctx, jobItem)
	if err != nil {
		s.logger.Error().Err(err).Str("agent_id", agentId).Str("job_item_id", jobItemId).Msg("Failed to get agent language")
	}
	sessionCtx.language = language

	// Get agent instructions
	instructions, err := s.getAgentInstructionsWithContext(ctx, agentId, jobItem)
	if err != nil {
		s.logger.Error().Err(err).Str("job_item_id", jobItemId).Msg("Failed to get agent instructions")
		return sessionCtx, xerrors.InternalError(ctx, fmt.Errorf("failed to get agent instructions: %w", err))
	}
	sessionCtx.instructions = instructions

	s.logger.Info().
		Str("agent_id", agentId).
		Str("session_id", sessionId).
		Int("instructions_length", len(instructions)).
		Msg("Loaded agent instructions for Realtime API")

	return sessionCtx, nil
}

// initializeRealtimeClient connects to the Realtime API and returns the client.
func (s *Service) initializeRealtimeClient(ctx context.Context, conn *websocket.Conn, sessionCtx realtimeSessionContext, sampleRate int) (clientTypes.RealtimeAgentClient, error) {
	realtimeClient, err := s.realTimeService.Connect(
		ctx,
		sessionCtx.agentID,
		sessionCtx.sessionID,
		sessionCtx.instructions,
		sessionCtx.language,
		openai.WithSampleRates(sampleRate, SampleRate24KHz),
		openai.WithAudioFormat(AudioFormat),
	)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to initialize Realtime client")
		if closeErr := conn.Close(); closeErr != nil {
			s.logger.Warn().Err(closeErr).Msg("Failed to close websocket connection")
		}
		return nil, xerrors.BadRequestError(ctx, fmt.Errorf("unable to connect to Realtime API: %w", err))
	}
	return realtimeClient, nil
}

// setupRealtimeChannels initializes all channels and starts background goroutines.
func (s *Service) setupRealtimeChannels(ctx context.Context, conn *websocket.Conn, sessionCtx realtimeSessionContext) realtimeChannels {
	channels := realtimeChannels{
		userMessageError: make(chan error, ChannelBufferSize),
		streamSid:        make(chan string, ChannelBufferSize),
		mark:             make(chan string, ChannelBufferSize),
		cutCall:          make(chan string, ChannelBufferSize),
		transcription:    s.startTranscriptionSession(ctx, sessionCtx.sessionID, sessionCtx.tenantID.String()),
		audioOutput:      sessionCtx.realtimeClient.AudioOutput(),
		events:           sessionCtx.realtimeClient.Events(),
	}

	// Start goroutine to read messages from Exotel
	go func() {
		if err := s.callProvider.ReadMessagesRealtime(ctx, conn, sessionCtx.realtimeClient, sessionCtx.agentID, channels.streamSid, channels.mark); err != nil {
			channels.userMessageError <- err
		}
	}()

	return channels
}

// waitForStreamSid waits for the streamSid from Exotel's start message.
func (s *Service) waitForStreamSid(ctx context.Context, conn *websocket.Conn, agentId string, errorChan <-chan error, streamSidChan <-chan string) (string, error) {
	select {
	case streamSid := <-streamSidChan:
		s.logger.Info().Str("agent_id", agentId).Str("stream_sid", streamSid).Msg("Received streamSid from Exotel")
		return streamSid, nil

	case err := <-errorChan:
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) || strings.Contains(err.Error(), "use of closed network connection") {
			s.logger.Info().Str("agent_id", agentId).Msg("WebSocket closed normally, exiting handler")
			return "", nil
		}
		s.logger.Error().Err(err).Str("agent_id", agentId).Msg("Error in websocket communication")
		if closeErr := conn.Close(); closeErr != nil {
			s.logger.Warn().Err(closeErr).Msg("Failed to close websocket connection")
		}
		return "", err

	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// runRealtimeEventLoop is the main event loop handling audio, events, and errors.
//
//nolint:cyclop
func (s *Service) runRealtimeEventLoop(ctx context.Context, conn *websocket.Conn, sessionCtx realtimeSessionContext, channels realtimeChannels, streamSid string) error {
	for {
		select {
		case <-ctx.Done():
			return s.handleContextDone(ctx, conn, sessionCtx.agentID)

		case err := <-channels.userMessageError:
			s.logger.Error().Err(err).Str("agent_id", sessionCtx.agentID).Msg("Error in websocket communication")
			return err

		case cutCallMsg := <-channels.cutCall:
			return s.handleCutCall(conn, sessionCtx.sessionID, cutCallMsg)

		case audio, ok := <-channels.audioOutput:
			if !ok {
				s.logger.Info().Str("session_id", sessionCtx.sessionID).Msg("Realtime audio output channel closed")
				return nil
			}
			if err := s.handleAudioOutput(ctx, conn, sessionCtx.sessionID, streamSid, audio); err != nil {
				return err
			}

		case event, ok := <-channels.events:
			if !ok {
				s.logger.Debug().Str("session_id", sessionCtx.sessionID).Msg("Realtime events channel closed")
				return nil
			}
			if err := s.handleRealtimeEvent(ctx, conn, sessionCtx, channels, streamSid, event); err != nil {
				return err
			}
		}
	}
}

// handleContextDone handles context cancellation or timeout.
func (s *Service) handleContextDone(ctx context.Context, conn *websocket.Conn, agentId string) error {
	if ctx.Err() == context.DeadlineExceeded {
		s.logger.Info().Str("agent_id", agentId).Msg("Session timeout reached")
		if closeErr := conn.Close(); closeErr != nil {
			s.logger.Warn().Err(closeErr).Msg("Failed to close websocket connection")
		}
		return xerrors.BadRequestError(ctx, errors.New("session timeout"))
	}
	return ctx.Err()
}

// handleCutCall handles the cut_call signal and closes the connection.
func (s *Service) handleCutCall(conn *websocket.Conn, sessionId, cutCallMsg string) error {
	s.logger.Info().
		Str("session_id", sessionId).
		Str("cut_call_message", cutCallMsg).
		Msg("Cut call signal received, ending call")

	if err := conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "Call ended by AI")); err != nil {
		s.logger.Warn().Err(err).Str("session_id", sessionId).Msg("Failed to send close message to Exotel, forcing close")
	}
	if closeErr := conn.Close(); closeErr != nil {
		s.logger.Warn().Err(closeErr).Msg("Failed to close websocket connection")
	}

	s.logger.Info().Str("session_id", sessionId).Msg("Call ended successfully via cut_call function")
	return nil
}

// handleAudioOutput sends audio from Realtime API to Exotel.
func (s *Service) handleAudioOutput(ctx context.Context, conn *websocket.Conn, sessionId, streamSid string, audio []byte) error {
	if len(audio) == 0 {
		s.logger.Debug().Str("session_id", sessionId).Msg("Received empty audio chunk, skipping")
		return nil
	}

	base64Audio := base64.StdEncoding.EncodeToString(audio)
	if err := s.callProvider.SendAgentAudioResponse(ctx, conn, streamSid, base64Audio, sessionId, len(audio)); err != nil {
		s.logger.Error().Err(err).Str("session_id", sessionId).Msg("Failed to send audio to Exotel")
		return err
	}

	s.logger.Debug().Str("session_id", sessionId).Int("audio_bytes", len(audio)).Msg("Forwarded audio to Exotel")
	return nil
}

// handleRealtimeEvent processes events from the Realtime API.
func (s *Service) handleRealtimeEvent(ctx context.Context, conn *websocket.Conn, sessionCtx realtimeSessionContext, channels realtimeChannels, streamSid string, event clientTypes.AgentEvent) error {
	switch event.Type {
	case entity.Unknown:
		s.logger.Debug().Str("session_id", sessionCtx.sessionID).Str("event_type", event.Type.String()).Msg("Ignored unknown Realtime event")

	case entity.SessionCreated:
		s.logger.Debug().Str("session_id", sessionCtx.sessionID).Msg("Realtime session created")

	case entity.AudioDelta:
		s.logger.Debug().Str("session_id", sessionCtx.sessionID).Str("event_type", event.Type.String()).Msg("Realtime event received")

	case entity.SpeechStarted:
		return s.handleSpeechStarted(ctx, conn, sessionCtx, streamSid)

	case entity.FunctionCall:
		return s.handleFunctionCallEvent(ctx, sessionCtx, channels.cutCall, event)

	case entity.TranscriptionCompleted:
		s.handleTranscriptionEvent(sessionCtx, channels.transcription, event, pb.Speaker_HUMAN)

	case entity.AudioTranscriptDone:
		s.handleTranscriptionEvent(sessionCtx, channels.transcription, event, pb.Speaker_AI_AGENT)

	case entity.Error:
		s.logger.Error().Str("session_id", sessionCtx.sessionID).Str("event_type", event.Type.String()).Msg("Error event received from Realtime API")

	default:
		s.logger.Debug().Str("session_id", sessionCtx.sessionID).Str("event_type", event.Type.String()).Msg("Unhandled Realtime event")
	}

	return nil
}

// handleSpeechStarted handles the SpeechStarted event (interrupting AI and clearing audio).
func (s *Service) handleSpeechStarted(ctx context.Context, conn *websocket.Conn, sessionCtx realtimeSessionContext, streamSid string) error {
	s.logger.Info().Str("session_id", sessionCtx.sessionID).Msg("User speech detected")

	// TODO: possible race, interrupt may be sent with no active response.
	if err := sessionCtx.realtimeClient.InterruptResponse(ctx); err != nil {
		s.logger.Error().Err(err).Str("session_id", sessionCtx.sessionID).Msg("Failed to interrupt AI response")
	}

	if err := s.callProvider.SendClearMessage(ctx, conn, streamSid); err != nil {
		s.logger.Error().Err(err).Str("session_id", sessionCtx.sessionID).Msg("Failed to send clear message to Exotel")
	} else {
		s.logger.Debug().Str("session_id", sessionCtx.sessionID).Msg("Sent clear message to Exotel")
	}

	return nil
}

// handleFunctionCallEvent processes function call events from the Realtime API.
func (s *Service) handleFunctionCallEvent(ctx context.Context, sessionCtx realtimeSessionContext, cutCallChan chan<- string, event clientTypes.AgentEvent) error {
	fnEvent, ok := event.Payload.(openai.FunctionCallEvent)
	if !ok {
		s.logger.Error().Str("session_id", sessionCtx.sessionID).Msg("Failed to parse function call event")
		return nil
	}

	s.logger.Info().
		Str("session_id", sessionCtx.sessionID).
		Str("call_id", fnEvent.CallID).
		Str("function", fnEvent.Name).
		Str("arguments", fnEvent.Arguments).
		Msg("Processing function call from Realtime API")

	result := s.handleRealtimeFunctionCall(ctx, fnEvent.Name, fnEvent.Arguments, sessionCtx.sessionID, cutCallChan, sessionCtx.realtimeClient)

	if err := sessionCtx.realtimeClient.SubmitFunctionResult(ctx, fnEvent.CallID, result); err != nil {
		s.logger.Error().Err(err).Str("session_id", sessionCtx.sessionID).Str("call_id", fnEvent.CallID).Msg("Failed to submit function result")
	}

	return nil
}

// handleTranscriptionEvent sends transcription events to the transcription service.
func (s *Service) handleTranscriptionEvent(sessionCtx realtimeSessionContext, transcriptionCh chan<- transcriptionEvent, event clientTypes.AgentEvent, speaker pb.Speaker) {
	transcript, ok := event.Payload.(openai.TranscriptionEvent)
	if !ok {
		s.logger.Error().Str("session_id", sessionCtx.sessionID).Msg("Failed to parse transcript event")
		return
	}

	detectedLang := detectLanguageFromText(transcript.Transcript)
	speakerStr := "USER"
	if speaker == pb.Speaker_AI_AGENT {
		speakerStr = "AI"
	}

	s.logger.Info().
		Str("session_id", sessionCtx.sessionID).
		Str("speaker", speakerStr).
		Str("transcript", transcript.Transcript).
		Str("detected_language", detectedLang).
		Msgf("%s transcript from Realtime API", speakerStr)

	transcriptionCh <- transcriptionEvent{
		sessionID: sessionCtx.sessionID,
		ownerID:   sessionCtx.tenantID.String(),
		text:      transcript.Transcript,
		speaker:   speaker,
		language:  detectedLang,
	}
}

// getAgentInstructionsWithContext builds Realtime system instructions from JobItem context.
func (s *Service) getAgentInstructionsWithContext(ctx context.Context, agentId string, jobItem entity.JobItem) (string, error) {
	agent, err := s.agentRepository.INTERNALGetAgent(ctx, agentId)
	if err != nil {
		s.logger.Error().Err(err).Str("agent_id", agentId).Msg("Failed to get agent")
		return "", err
	}

	agentPrompt := agent.CreateAgentRequest.GetAgentPromptRequest().GetPrompt()

	contextText := strings.TrimSpace(jobItem.AgentContext)

	instructions := fmt.Sprintf("%s\n\n%s", agentPrompt, contextText)

	s.logger.Info().
		Str("agent_id", agentId).
		Str("job_item_id", jobItem.ID.String()).
		Int("context_len", len(contextText)).
		Int("instructions_len", len(instructions)).
		Msg("Built Realtime instructions from campaign context")

	return instructions, nil
}

// getLanguageFromJobItem retrieves the language preference from the call job or CSV context.
func (s *Service) getLanguageFromJobItem(ctx context.Context, jobItem entity.JobItem) (types.AgentLanguage, error) {
	// Get the call job to retrieve preferred language
	callJob, err := s.CallJobRepository.GetCallJob(ctx, jobItem.JobID, jobItem.TenantID)
	if err != nil {
		s.logger.Warn().Err(err).Msg("Failed to get call job")
		return "", err
	}

	// Return preferred language from call job or default to English
	if callJob.Preffered_language != "" {
		lang := strings.ToLower(callJob.Preffered_language)
		if lang == "hindi" || lang == "hi" {
			return types.AgentLanguageHindi, nil
		}
		return types.AgentLanguageEnglish, nil
	}

	return types.AgentLanguageEnglish, nil
}

// handleRealtimeFunctionCall processes function calls from the Realtime API.
func (s *Service) handleRealtimeFunctionCall(ctx context.Context, funcName string, arguments string, sessionId string, cutCallChan chan<- string, realtime clientTypes.RealtimeAgentClient) string {
	s.logger.Info().
		Str("function", funcName).
		Str("arguments", arguments).
		Str("session_id", sessionId).
		Msg("Handling Realtime function call")

	switch funcName {
	case "cut_call":
		return s.handleCutCallFunction(arguments, sessionId, cutCallChan)
	case "set_language":
		return s.handleSetLanguageFunction(ctx, arguments, sessionId, realtime)
	default:
		s.logger.Warn().Str("function", funcName).Msg("Unknown function call received")
		return fmt.Sprintf(`{"success": false, "error": "Unknown function: %s"}`, funcName)
	}
}

func (s *Service) handleCutCallFunction(arguments string, sessionId string, cutCallChan chan<- string) string {
	var args struct {
		CutCallMessage string `json:"cut_call_message"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		s.logger.Error().Err(err).Msg("Failed to parse cut_call arguments")
		return `{"success": false, "error": "Invalid arguments"}`
	}

	s.logger.Info().
		Str("session_id", sessionId).
		Str("message", args.CutCallMessage).
		Msg("Cut call requested via Realtime function call")

	go func() {
		time.Sleep(FarewellDelay)
		select {
		case cutCallChan <- args.CutCallMessage:
			s.logger.Info().Str("session_id", sessionId).Msg("Sent cut call signal after farewell delay")
		default:
			s.logger.Warn().Str("session_id", sessionId).Msg("Cut call channel full")
		}
	}()

	return fmt.Sprintf(
		`{"success": true, "action": "speak_farewell", "farewell_message": "%s", "instruction": "Please say this farewell message to the customer now: %s"}`,
		args.CutCallMessage,
		args.CutCallMessage,
	)
}

func (s *Service) handleSetLanguageFunction(ctx context.Context, arguments string, sessionId string, realtime clientTypes.RealtimeAgentClient) string {
	var args struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		s.logger.Error().Err(err).Msg("Failed to parse set_language arguments")
		return `{"success": false, "error": "Invalid arguments"}`
	}

	s.logger.Info().
		Str("session_id", sessionId).
		Str("language", args.Language).
		Msg("Language change requested via Realtime function call")

	if err := realtime.UpdateLanguage(ctx, types.AgentLanguage(args.Language)); err != nil {
		s.logger.Error().Err(err).
			Str("session_id", sessionId).
			Str("language", args.Language).
			Msg("Failed to update language in Realtime session")
		return fmt.Sprintf(`{"success": false, "error": "Failed to update language: %s"}`, err.Error())
	}

	s.logger.Info().
		Str("session_id", sessionId).
		Str("language", args.Language).
		Msg("Successfully updated language in Realtime session")

	return fmt.Sprintf(
		`{"success": true, "language": "%s", "message": "Language updated to %s. Please continue the conversation in %s."}`,
		args.Language,
		args.Language,
		args.Language,
	)
}

func detectLanguageFromText(text string) string {
	for _, r := range text {
		if unicode.Is(unicode.Devanagari, r) {
			return "hi"
		}
	}
	return "en"
}
