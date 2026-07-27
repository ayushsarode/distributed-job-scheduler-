package twilio

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	xerrors "exiro.ai/application/errors"
	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/outboundcallservice/callProvider"
	"exiro.ai/config"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/twilio/twilio-go"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"
)

const (
	twilioAPITimeout = 10 * time.Second
)

var _ callProvider.CallProvider = (*twilioHandler)(nil)

// twilioHandler encapsulates all Twilio WebSocket message creation and sending logic.
type twilioHandler struct {
	logger *zerolog.Logger
}

// newTwilioHandler creates a new twilioHandler instance.
func NewTwilioHandler(logger *zerolog.Logger) *twilioHandler {
	return &twilioHandler{
		logger: logger,
	}
}

// SendMediaMessage sends a media message to Twilio with base64 encoded audio.
func (th *twilioHandler) SendMediaMessage(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string) error {
	message := twilioOutboundMediaMessage{
		Event:     "media",
		StreamSid: streamSid,
		Media: twilioOutboundMediaPayload{
			Payload: base64AudioPayload,
		},
	}

	return th.SendMessage(ctx, conn, message, "media message")
}

// SendMarkMessage sends a mark message to Twilio for tracking audio playback completion.
func (th *twilioHandler) SendMarkMessage(ctx context.Context, conn *websocket.Conn, streamSid, markName string) error {
	message := twilioOutboundMarkMessage{
		Event:     "mark",
		StreamSid: streamSid,
		Mark: twilioOutboundMarkData{
			Name: markName,
		},
	}

	return th.SendMessage(ctx, conn, message, "mark message")
}

// SendClearMessage sends a clear message to Twilio to stop any queued audio.
func (th *twilioHandler) SendClearMessage(ctx context.Context, conn *websocket.Conn, streamSid string) error {
	message := twilioClearMessage{
		Event:     "clear",
		StreamSid: streamSid,
	}

	return th.SendMessage(ctx, conn, message, "clear message")
}

// sendMessage is a generic method to marshal and send any message to Twilio.
func (th *twilioHandler) SendMessage(ctx context.Context, conn *websocket.Conn, message any, messageType string) error {
	// Marshal the message to JSON
	messageJSON, err := json.Marshal(message)
	if err != nil {
		th.logger.Error().Ctx(ctx).Err(err).Str("message_type", messageType).Msg("Failed to marshal Twilio message to JSON")
		return fmt.Errorf("failed to marshal %s: %w", messageType, err)
	}

	// Send the JSON message via WebSocket as text message (Twilio expects JSON as text)
	if err := conn.WriteMessage(websocket.TextMessage, messageJSON); err != nil {
		th.logger.Error().Ctx(ctx).Err(err).Str("message_type", messageType).Msg("Failed to send Twilio message to WebSocket")
		return fmt.Errorf("failed to send %s: %w", messageType, err)
	}

	th.logger.Debug().Ctx(ctx).Str("message_type", messageType).Msg("Successfully sent Twilio message")
	return nil
}

// SendStopMessage sends a stop message to Twilio to terminate the call stream.
func (th *twilioHandler) SendStopMessage(ctx context.Context, conn *websocket.Conn, streamSid string, sessionId string) error {
	message := map[string]any{
		"event":     "stop",
		"streamSid": streamSid,
	}

	th.logger.Info().
		Ctx(ctx).
		Str("session_id", sessionId).
		Str("stream_sid", streamSid).
		Msg("Sending stop signal to Twilio to terminate call")

	return th.SendMessage(ctx, conn, message, "stop signal")
}

func (th *twilioHandler) SendAgentAudioResponse(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string, sessionId string, audioBytes int) error {
	// Send the media message
	if err := th.SendMediaMessage(ctx, conn, streamSid, base64AudioPayload); err != nil {
		return fmt.Errorf("failed to send agent audio response: %w", err)
	}

	th.logger.Info().
		Ctx(ctx).
		Str("session_id", sessionId).
		Str("stream_sid", streamSid).
		Int("audio_bytes", audioBytes).
		Msg("Successfully sent agent audio response to Twilio")

	// Optionally send a mark message to track when this audio finishes playing
	// markName := fmt.Sprintf("agent-response-%s", sessionId)
	// if err := th.SendMarkMessage(ctx, conn, streamSid, markName); err != nil {
	// 	th.logger.Warn().Err(err).Str("session_id", sessionId).Msg("Failed to send mark message, but continuing")
	// 	// Don't return error for mark message failure as it's not critical
	// }

	return nil
}

// SendMediaWithClear sends a clear message followed by a media message
// This is useful when you want to interrupt any currently playing audio.
func (th *twilioHandler) SendMediaWithClear(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string) error {
	// First send clear message to stop any queued audio
	if err := th.SendClearMessage(ctx, conn, streamSid); err != nil {
		th.logger.Warn().Ctx(ctx).Err(err).Str("stream_sid", streamSid).Msg("Failed to send clear message before media, but continuing")
		// Don't return error for clear message failure as it's not critical
	}

	// Then send the new media
	return th.SendMediaMessage(ctx, conn, streamSid, base64AudioPayload)
}

// getTwilioCredentials extracts Twilio credentials from the provided map or environment variables.
func (th *twilioHandler) getTwilioCredentials(credential map[string]string) (string, string, error) {
	var accountSid, authToken string

	if credential != nil {
		accountSid = credential["account_sid"]
		authToken = credential["auth_token"]
	} else {
		accountSid = os.Getenv("TWILIO_ACCOUNT_SID")
		authToken = os.Getenv("TWILIO_AUTH_TOKEN")
	}

	if accountSid == "" || authToken == "" {
		return "", "", errors.New("twilio credentials not configured")
	}

	return accountSid, authToken, nil
}

// getBaseURL retrieves the base URL from config or environment variables.
func (th *twilioHandler) getBaseURL(ctx context.Context) string {
	baseURL := config.Ctx(ctx).OutboundCalling.BaseURL
	if baseURL == "" {
		baseURL = os.Getenv("BASE_URL")
		if baseURL == "" {
			baseURL = "seemingly-fit-python.ngrok-free.app"
			th.logger.Warn().Ctx(ctx).Str("base_url", baseURL).Msg("Using fallback BASE_URL for development")
		}
	}
	return baseURL
}

// buildCallParams builds and configures Twilio call parameters.
func (th *twilioHandler) buildCallParams(ctx context.Context, fromNumber, toNumber, agentId, sessionId, jobItemId, baseURL string) *openapi.CreateCallParams {
	params := &openapi.CreateCallParams{}
	params.SetTo(toNumber)
	params.SetFrom(fromNumber)

	websocketURL := fmt.Sprintf("wss://%s/public/twilio-handler/%s/%s/%s", baseURL, agentId, sessionId, jobItemId)
	twiml := fmt.Sprintf(`<Response><Connect><Stream url="%s" /></Connect></Response>`, websocketURL)
	params.SetTwiml(twiml)

	th.logger.Debug().Ctx(ctx).Str("websocket_url", websocketURL).Str("twiml", twiml).Msg("Generated TwiML for call")

	statusCallbackURL := fmt.Sprintf("https://%s/public/call-status-webhook/%s/%s", baseURL, jobItemId, sessionId)
	params.SetStatusCallback(statusCallbackURL)
	params.SetStatusCallbackMethod("POST")
	params.SetStatusCallbackEvent([]string{
		"initiated", "ringing", "answered", "in-progress",
		"completed", "failed", "busy", "no-answer", "canceled",
	})

	return params
}

// MakeCall initiates an outbound call through Twilio REST API.
func (th *twilioHandler) MakeCall(ctx context.Context, fromNumber, toNumber, agentId, sessionId, jobItemId string, credential map[string]string) (string, error) {
	th.logger.Info().
		Ctx(ctx).
		Str("from", fromNumber).
		Str("to", toNumber).
		Str("agent_id", agentId).
		Str("session_id", sessionId).
		Str("job_item_id", jobItemId).
		Msg("Initiating Twilio outbound call")

	accountSid, authToken, err := th.getTwilioCredentials(credential)
	if err != nil {
		th.logger.Error().Ctx(ctx).Msg("Twilio credentials not configured")
		return "", xerrors.InternalError(ctx, err)
	}

	baseURL := th.getBaseURL(ctx)

	client := twilio.NewRestClientWithParams(twilio.ClientParams{
		Username: accountSid,
		Password: authToken,
	})

	params := th.buildCallParams(ctx, fromNumber, toNumber, agentId, sessionId, jobItemId, baseURL)

	resp, err := client.Api.CreateCall(params)
	if err != nil {
		th.logger.Error().Ctx(ctx).Err(err).
			Str("from", fromNumber).
			Str("to", toNumber).
			Msg("Failed to initiate Twilio call")
		return "", xerrors.InternalError(ctx, fmt.Errorf("failed to start Twilio call: %w", err))
	}

	callSid := ""
	if resp.Sid != nil {
		callSid = *resp.Sid
	}

	statusCallbackURL := fmt.Sprintf("https://%s/public/call-status-webhook/%s/%s", baseURL, jobItemId, sessionId)
	th.logger.Info().
		Ctx(ctx).
		Str("from", fromNumber).
		Str("to", toNumber).
		Str("call_sid", callSid).
		Str("agent_id", agentId).
		Str("session_id", sessionId).
		Str("job_item_id", jobItemId).
		Str("status_callback_url", statusCallbackURL).
		Msg("Twilio call initiated successfully")

	return callSid, nil
}

// readWebSocketMessage reads a single message from the WebSocket connection.
func (th *twilioHandler) readWebSocketMessage(ctx context.Context, c *websocket.Conn, agentId string) ([]byte, error) {
	_, message, err := c.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(err, websocket.CloseNormalClosure) ||
			strings.Contains(err.Error(), "use of closed network connection") {
			th.logger.Info().Ctx(ctx).Str("agent_id", agentId).Msg("WebSocket closed, exiting Twilio reader")
			return nil, nil
		}
		th.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentId).Msg("Failed to read Twilio message")
		return nil, fmt.Errorf("failed to read Twilio message: %w", err)
	}
	return message, nil
}

// processMessage processes a single Twilio message based on its event type.
//nolint:cyclop
func (th *twilioHandler) processMessage(ctx context.Context, message []byte, stt clientTypes.STTClient, agentId string, streamSidChan chan<- string, markChan chan<- string) (bool, error) {
	var baseMsg twilioInboundMessage
	if err := json.Unmarshal(message, &baseMsg); err != nil {
		th.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentId).Msg("Failed to unmarshal Twilio message")
		return false, fmt.Errorf("failed to unmarshal Twilio message: %w", err)
	}

	switch baseMsg.Event {
	case "connected":
		return false, th.HandleConnectedMessage(ctx, message, agentId)
	case "start":
		streamSid, err := th.HandleStartMessage(ctx, message, agentId)
		if err != nil {
			return false, err
		}
		select {
		case streamSidChan <- streamSid:
			th.logger.Debug().Ctx(ctx).Str("agent_id", agentId).Str("stream_sid", streamSid).Msg("Extracted streamSid from Twilio start message")
		case <-ctx.Done():
			return false, ctx.Err()
		}
		return false, nil
	case "media":
		return false, th.HandleMediaMessage(ctx, message, stt, agentId)
	case "stop":
		if err := th.HandleStopMessage(ctx, message, agentId); err != nil {
			return false, err
		}
		th.logger.Info().Ctx(ctx).Str("agent_id", agentId).Msg("Twilio stream stopped")
		return true, nil
	case "dtmf":
		return false, th.HandleDTMFMessage(ctx, message, agentId)
	case "mark":
		return false, th.HandleMarkMessage(ctx, message, agentId, markChan)
	default:
		th.logger.Warn().Ctx(ctx).Str("agent_id", agentId).Str("event", baseMsg.Event).Msg("Received unknown Twilio message type")
		return false, nil
	}
}

// ReadTwilioMessages reads and processes incoming Twilio WebSocket messages.
func (th *twilioHandler) ReadMessages(
	ctx context.Context,
	c *websocket.Conn,
	stt clientTypes.STTClient,
	agentId string,
	streamSidChan chan<- string,
	markChan chan<- string,
) error {
	th.logger.Debug().Ctx(ctx).Str("agent_id", agentId).Msg("Starting to read Twilio messages")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			message, err := th.readWebSocketMessage(ctx, c, agentId)
			if err != nil {
				return err
			}
			if message == nil {
				return nil
			}

			shouldStop, err := th.processMessage(ctx, message, stt, agentId, streamSidChan, markChan)
			if err != nil {
				return err
			}
			if shouldStop {
				return nil
			}
		}
	}
}

// ReadMessagesRealtime reads and processes incoming Twilio WebSocket messages for Realtime API mode
// stub implementation as Twilio is typically used with STT+TTS mode.
func (th *twilioHandler) ReadMessagesRealtime(
	ctx context.Context,
	c *websocket.Conn,
	realtime clientTypes.RealtimeAgentClient,
	agentId string,
	streamSidChan chan<- string,
	markChan chan<- string,
) error {
	// For now, return an error as Twilio Realtime mode is not implemented
	return errors.New("realtime API mode is not supported for Twilio provider")
}

// handleConnectedMessage processes Twilio's connected message.
func (th *twilioHandler) HandleConnectedMessage(ctx context.Context, message []byte, agentId string) error {
	var connectedMsg twilioConnectedMessage
	if err := json.Unmarshal(message, &connectedMsg); err != nil {
		return fmt.Errorf("failed to unmarshal connected message: %w", err)
	}

	th.logger.Info().
		Ctx(ctx).
		Str("agent_id", agentId).
		Str("protocol", connectedMsg.Protocol).
		Str("version", connectedMsg.Version).
		Msg("Twilio WebSocket connected")

	return nil
}

// handleStartMessage processes Twilio's start message and extracts streamSid.
func (th *twilioHandler) HandleStartMessage(ctx context.Context, message []byte, agentId string) (string, error) {
	var startMsg twilioStartMessage
	if err := json.Unmarshal(message, &startMsg); err != nil {
		return "", fmt.Errorf("failed to unmarshal start message: %w", err)
	}

	th.logger.Info().
		Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", startMsg.StreamSid).
		Str("call_sid", startMsg.Start.CallSid).
		Str("account_sid", startMsg.Start.AccountSid).
		Strs("tracks", startMsg.Start.Tracks).
		Str("encoding", startMsg.Start.MediaFormat.Encoding).
		Int("sample_rate", startMsg.Start.MediaFormat.SampleRate).
		Msg("Twilio stream started")

	return startMsg.StreamSid, nil
}

// handleMediaMessage processes incoming media messages from Twilio.
func (th *twilioHandler) HandleMediaMessage(ctx context.Context, message []byte, stt clientTypes.STTClient, agentId string) error {
	var mediaMsg twilioInboundMediaMessage
	if err := json.Unmarshal(message, &mediaMsg); err != nil {
		return fmt.Errorf("failed to unmarshal media message: %w", err)
	}

	// Decode the base64 audio payload
	audioBytes, err := base64.StdEncoding.DecodeString(mediaMsg.Media.Payload)
	if err != nil {
		th.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentId).Msg("Failed to decode audio payload")
		return fmt.Errorf("failed to decode audio payload: %w", err)
	}

	// Send audio to STT handler
	if err := stt.SendAudio(ctx, audioBytes); err != nil {
		th.logger.Error().Ctx(ctx).Err(err).
			Str("agent_id", agentId).
			Msg("Failed to send audio to STT")
		return fmt.Errorf("failed to send audio to STT: %w", err)
	}

	return nil
}

// handleStopMessage processes Twilio's stop message.
func (th *twilioHandler) HandleStopMessage(ctx context.Context, message []byte, agentId string) error {
	var stopMsg twilioStopMessage
	if err := json.Unmarshal(message, &stopMsg); err != nil {
		return fmt.Errorf("failed to unmarshal stop message: %w", err)
	}

	th.logger.Info().
		Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", stopMsg.StreamSid).
		Str("call_sid", stopMsg.Stop.CallSid).
		Str("account_sid", stopMsg.Stop.AccountSid).
		Msg("Twilio stream stopped")

	return nil
}

// handleDTMFMessage processes DTMF messages from Twilio.
func (th *twilioHandler) HandleDTMFMessage(ctx context.Context, message []byte, agentId string) error {
	var dtmfMsg twilioDTMFMessage
	if err := json.Unmarshal(message, &dtmfMsg); err != nil {
		return fmt.Errorf("failed to unmarshal DTMF message: %w", err)
	}

	th.logger.Info().
		Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", dtmfMsg.StreamSid).
		Str("digit", dtmfMsg.DTMF.Digit).
		Str("track", dtmfMsg.DTMF.Track).
		Msg("Received DTMF from Twilio")

	// TODO: Handle DTMF input if needed for your application
	return nil
}

// handleMarkMessage processes mark messages from Twilio.
func (th *twilioHandler) HandleMarkMessage(ctx context.Context, message []byte, agentId string, markChan chan<- string) error {
	var markMsg twilioInboundMarkMessage
	if err := json.Unmarshal(message, &markMsg); err != nil {
		return fmt.Errorf("failed to unmarshal mark message: %w", err)
	}

	th.logger.Debug().
		Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", markMsg.StreamSid).
		Str("mark_name", markMsg.Mark.Name).
		Msg("Received mark confirmation from Twilio")

	if markChan != nil {
		select {
		case markChan <- markMsg.Mark.Name:
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return nil
}

// fetchTwilioCallData makes HTTP request to Twilio API to fetch call details.
func (th *twilioHandler) fetchTwilioCallData(ctx context.Context, accountSid, authToken, callSid string) (*http.Response, error) {
	url := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s/Calls/%s.json", accountSid, callSid)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, xerrors.InternalError(ctx, fmt.Errorf("failed to create request: %w", err))
	}

	req.SetBasicAuth(accountSid, authToken)

	client := &http.Client{Timeout: twilioAPITimeout}
	resp, err := client.Do(req) //nolint:gosec

	if err != nil {
		return nil, xerrors.InternalError(ctx, fmt.Errorf("failed to make request to Twilio: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			th.logger.Warn().Err(closeErr).Ctx(ctx).Msg("Failed to close response body after error status")
		}
		return nil, xerrors.InternalError(ctx, fmt.Errorf("twilio API error: Status %d", resp.StatusCode))
	}

	return resp, nil
}

// parseTwilioCallResponse parses the Twilio API response into CallDetails.
func (th *twilioHandler) parseTwilioCallResponse(ctx context.Context, resp *http.Response) (*callProvider.CallDetails, error) {
	var twilioCall struct {
		Sid           string `json:"sid"`
		Status        string `json:"status"`
		StartTime     string `json:"start_time"`
		EndTime       string `json:"end_time"`
		Duration      string `json:"duration"`
		From          string `json:"from"`
		To            string `json:"to"`
		Price         string `json:"price"`
		PriceUnit     string `json:"price_unit"`
		Direction     string `json:"direction"`
		AnsweredBy    string `json:"answered_by"`
		ForwardedFrom string `json:"forwarded_from"`
		CallerName    string `json:"caller_name"`
		Uri           string `json:"uri"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&twilioCall); err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to decode response"))
	}

	layout := time.RFC1123Z
	startTime, _ := time.Parse(layout, twilioCall.StartTime)
	endTime, _ := time.Parse(layout, twilioCall.EndTime)

	durationSec := 0
	if twilioCall.Duration != "" {
		if val, err := strconv.Atoi(twilioCall.Duration); err == nil {
			durationSec = val
		}
	}

	return &callProvider.CallDetails{
		Sid:       twilioCall.Sid,
		Status:    twilioCall.Status,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  durationSec,
		From:      twilioCall.From,
		To:        twilioCall.To,
	}, nil
}

func (th *twilioHandler) GetCallDetails(ctx context.Context, callSid string, creds map[string]string) (*callProvider.CallDetails, error) {
	accountSid, authToken, err := th.getTwilioCredentials(creds)
	if err != nil {
		th.logger.Error().Ctx(ctx).Msg("Twilio credentials not configured for GetCallDetails")
		return nil, xerrors.InternalError(ctx, err)
	}

	resp, err := th.fetchTwilioCallData(ctx, accountSid, authToken, callSid)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			th.logger.Error().Ctx(ctx).
				Err(err).
				Str("call_sid", callSid).
				Msg("Failed to close Twilio GetCallDetails response body")
		}
	}()

	return th.parseTwilioCallResponse(ctx, resp)
}
