package exotel

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
)

var _ callProvider.CallProvider = (*exotelHandler)(nil)

const (
	maxCustomParamsLength = 256
	httpClientTimeout     = 10 * time.Second
)

type exotelCredentials struct {
	AccountSid string
	ApiToken   string //nolint:gosec
	ApiKey     string //nolint:gosec
}

type exotelHandler struct {
	logger *zerolog.Logger
}

type exotelCallResponse struct {
	XMLName xml.Name       `xml:"TwilioResponse"`
	Call    exotelCallData `xml:"Call"`
}

type exotelCallData struct {
	Sid    string `xml:"Sid"`
	Status string `xml:"Status"`
}

type exotelCallDetails struct {
	XMLName xml.Name       `xml:"TwilioResponse"`
	Call    exotelCallInfo `xml:"Call"`
}

type exotelCallInfo struct {
	Sid          string `xml:"Sid"`
	Status       string `xml:"Status"`
	StartTime    string `xml:"StartTime"`
	EndTime      string `xml:"EndTime"`
	Duration     string `xml:"Duration"` // in seconds
	From         string `xml:"From"`
	To           string `xml:"To"`
	RecordingUrl string `xml:"RecordingUrl"`
}

func NewExotelHandler(logger *zerolog.Logger) *exotelHandler {
	return &exotelHandler{
		logger: logger,
	}
}

func (eh *exotelHandler) SendMediaMessage(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string) error {
	message := exotelOutboundMediaMessage{
		Event:     "media",
		StreamSid: streamSid,
		Media: exotelOutboundMediaPayload{
			Payload: base64AudioPayload,
		},
	}

	return eh.SendMessage(ctx, conn, message, "media message")
}

func (eh *exotelHandler) SendMarkMessage(ctx context.Context, conn *websocket.Conn, streamSid, markName string) error {
	message := exotelOutboundMarkMessage{
		Event:     "mark",
		StreamSid: streamSid,
		Mark: exotelOutboundMarkData{
			Name: markName,
		},
	}

	return eh.SendMessage(ctx, conn, message, "mark message")
}

func (eh *exotelHandler) SendClearMessage(ctx context.Context, conn *websocket.Conn, streamSid string) error {
	message := exotelClearMessage{
		Event:     "clear",
		StreamSid: streamSid,
	}

	return eh.SendMessage(ctx, conn, message, "clear message")
}

func (eh *exotelHandler) SendMessage(ctx context.Context, conn *websocket.Conn, message any, messageType string) error {
	messageJSON, err := json.Marshal(message)
	if err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Str("message_type", messageType).Msg("Failed to marshal Exotel message to JSON")
		return fmt.Errorf("failed to marshal %s: %w", messageType, err)
	}

	if err := conn.WriteMessage(websocket.TextMessage, messageJSON); err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Str("message_type", messageType).Msg("Failed to send Exotel message to WebSocket")
		return fmt.Errorf("failed to send %s: %w", messageType, err)
	}

	eh.logger.Debug().Ctx(ctx).Str("message_type", messageType).Msg("Successfully sent Exotel message")
	return nil
}

func (eh *exotelHandler) SendAgentAudioResponse(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string, sessionId string, audioBytes int) error {
	if err := eh.SendMediaMessage(ctx, conn, streamSid, base64AudioPayload); err != nil {
		return fmt.Errorf("failed to send agent audio response: %w", err)
	}

	eh.logger.Info().
		Ctx(ctx).
		Str("session_id", sessionId).
		Str("stream_sid", streamSid).
		Int("audio_bytes", audioBytes).
		Msg("Successfully sent agent audio response to Exotel")

	return nil
}

func (eh *exotelHandler) SendMediaWithClear(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string) error {
	if err := eh.SendClearMessage(ctx, conn, streamSid); err != nil {
		eh.logger.Warn().Ctx(ctx).Err(err).Str("stream_sid", streamSid).Msg("Failed to send clear message before media, but continuing")
	}

	return eh.SendMediaMessage(ctx, conn, streamSid, base64AudioPayload)
}

func (eh *exotelHandler) loadCredentials(credential map[string]string) (*exotelCredentials, error) {
	var accountSid, apiToken, apiKey string

	if credential != nil {
		accountSid = credential["account_sid"]
		apiToken = credential["api_token"]
		apiKey = credential["api_key"]
	} else {
		accountSid = os.Getenv("EXOTEL_ACCOUNT_SID")
		apiToken = os.Getenv("EXOTEL_API_TOKEN")
		apiKey = os.Getenv("EXOTEL_API_KEY")
	}

	if accountSid == "" || apiKey == "" || apiToken == "" {
		return nil, errors.New("exotel credentials not configured")
	}

	return &exotelCredentials{
		AccountSid: accountSid,
		ApiToken:   apiToken,
		ApiKey:     apiKey,
	}, nil
}

func (eh *exotelHandler) buildCustomParams(ctx context.Context, agentId, sessionId, jobItemId string) (string, error) {
	customData := map[string]string{
		"agentId":   agentId,
		"sessionId": sessionId,
		"jobItemId": jobItemId,
	}
	customFieldJSON, err := json.Marshal(customData)
	if err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Msg("Failed to marshal custom field data")
		return "", fmt.Errorf("failed to marshal custom field: %w", err)
	}
	customParamsStr := string(customFieldJSON)
	if len(customParamsStr) > maxCustomParamsLength {
		eh.logger.Warn().Ctx(ctx).Int("length", len(customParamsStr)).Msg("Custom parameters exceed 256 character limit, truncating")
		customParamsStr = customParamsStr[:maxCustomParamsLength]
	}
	return customParamsStr, nil
}

func (eh *exotelHandler) getBaseURL(ctx context.Context) string {
	appBaseURL := config.Ctx(ctx).OutboundCalling.BaseURL
	if appBaseURL == "" {
		appBaseURL = os.Getenv("BASE_URL")
		if appBaseURL == "" {
			appBaseURL = "seemingly-fit-python.ngrok-free.app"
			eh.logger.Warn().Ctx(ctx).Str("base_url", appBaseURL).Msg("Using fallback BASE_URL for development")
		}
	}
	return appBaseURL
}

func (eh *exotelHandler) buildCallURLs(ctx context.Context, accountSid, appBaseURL, jobItemId, sessionId string) (string, string, error) {
	voicebotAppId := config.Ctx(ctx).OutboundCalling.ExotelVoicebotAppId
	if voicebotAppId == "" {
		return "", "", errors.New("exotel voicebot app ID not configured")
	}

	exomlURL := fmt.Sprintf("http://my.exotel.com/%s/exoml/start_voice/%s", accountSid, voicebotAppId)
	statusCallbackURL := fmt.Sprintf("https://%s/public/exotel/status-callback/%s/%s", appBaseURL, jobItemId, sessionId)

	return exomlURL, statusCallbackURL, nil
}

func (eh *exotelHandler) buildCallFormData(toNumber, fromNumber, exomlURL, statusCallbackURL, customParamsStr string) url.Values {
	formData := url.Values{}
	formData.Set("From", toNumber)
	formData.Set("CallerId", fromNumber)
	formData.Set("Url", exomlURL)
	formData.Set("StatusCallback", statusCallbackURL)
	formData.Set("CallType", "trans")
	formData.Set("TimeLimit", "14400")
	formData.Set("CustomField", customParamsStr)
	return formData
}

func (eh *exotelHandler) executeCallRequest(ctx context.Context, accountSid, formDataEncoded string, creds *exotelCredentials) ([]byte, error) {
	exotelAPIBaseURL := "https://api.exotel.com/v1/Accounts"
	apiURL := fmt.Sprintf("%s/%s/Calls/connect", exotelAPIBaseURL, accountSid)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(formDataEncoded))
	if err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Msg("Failed to create HTTP request")
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(creds.ApiKey, creds.ApiToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	eh.logger.Debug().Ctx(ctx).Str("api_url", apiURL).Str("form_data", formDataEncoded).Msg("Sending Exotel API request")

	client := &http.Client{}
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to make Exotel API call: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			eh.logger.Error().Ctx(ctx).Err(err).Msg("Failed to close Exotel API response body")
		}
	}()

	body, _ := io.ReadAll(resp.Body)
	eh.logger.Debug().Ctx(ctx).Int("status", resp.StatusCode).Str("body", string(body)).Msg("Exotel API raw response")

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		eh.logger.Error().Ctx(ctx).Int("status", resp.StatusCode).Str("body", string(body)).Msg("Exotel API returned error")
		return nil, fmt.Errorf("exotel API error (status %d): %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (eh *exotelHandler) parseCallResponse(ctx context.Context, body []byte) (string, string, error) {
	var result exotelCallResponse
	trimmedBody := []byte(strings.TrimSpace(string(body)))
	if err := xml.Unmarshal(trimmedBody, &result); err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Str("body", string(body)).Msg("Failed to parse Exotel XML response")
		return "", "", fmt.Errorf("failed to parse Exotel response: %w", err)
	}

	if result.Call.Sid == "" {
		eh.logger.Error().Ctx(ctx).Str("body", string(body)).Msg("Exotel response missing Call SID")
		return "", "", errors.New("exotel response missing call SID")
	}

	return result.Call.Sid, result.Call.Status, nil
}

func (eh *exotelHandler) MakeCall(
	ctx context.Context,
	fromNumber, toNumber, agentId, sessionId, jobItemId string,
	credential map[string]string,
) (string, error) {
	eh.logger.Info().Ctx(ctx).Str("from", fromNumber).Str("to", toNumber).
		Str("agent_id", agentId).Str("session_id", sessionId).Str("job_item_id", jobItemId).
		Msg("Initiating Exotel outbound call")

	creds, err := eh.loadCredentials(credential)
	if err != nil {
		eh.logger.Error().Ctx(ctx).Msg("Exotel credentials not configured")
		return "", err
	}
	eh.logger.Debug().Ctx(ctx).Str("account_sid", creds.AccountSid).Msg("Loaded Account SID")

	customParamsStr, err := eh.buildCustomParams(ctx, agentId, sessionId, jobItemId)
	if err != nil {
		return "", err
	}

	appBaseURL := eh.getBaseURL(ctx)
	exomlURL, statusCallbackURL, err := eh.buildCallURLs(ctx, creds.AccountSid, appBaseURL, jobItemId, sessionId)
	if err != nil {
		eh.logger.Error().Ctx(ctx).Msg("Exotel Voicebot App ID not configured")
		return "", err
	}

	formData := eh.buildCallFormData(toNumber, fromNumber, exomlURL, statusCallbackURL, customParamsStr)
	formDataEncoded := formData.Encode()

	eh.logger.Debug().Ctx(ctx).Str("exoml_url", exomlURL).Str("status_callback_url", statusCallbackURL).Msg("Built URLs")

	body, err := eh.executeCallRequest(ctx, creds.AccountSid, formDataEncoded, creds)
	if err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Str("from", fromNumber).Str("to", toNumber).Msg("Failed to make Exotel API call")
		return "", err
	}

	callSid, callStatus, err := eh.parseCallResponse(ctx, body)
	if err != nil {
		return "", err
	}

	eh.logger.Info().Ctx(ctx).Str("from", fromNumber).Str("to", toNumber).
		Str("call_sid", callSid).Str("status", callStatus).Msg("Exotel call initiated successfully")

	return callSid, nil
}

func (eh *exotelHandler) fetchCallDetails(ctx context.Context, accountSid, callSid, apiKey, apiToken string) ([]byte, error) {
	url := fmt.Sprintf("https://api.exotel.com/v1/Accounts/%s/Calls/%s", accountSid, callSid)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	req.SetBasicAuth(apiKey, apiToken)

	client := &http.Client{Timeout: httpClientTimeout}
	resp, err := client.Do(req) //nolint:gosec
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			eh.logger.Error().Ctx(ctx).Err(err).Msg("Failed to close Exotel API response body")
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, xerrors.InternalError(ctx, fmt.Errorf("exotel API error: Status %d", resp.StatusCode))
	}

	return body, nil
}

func (eh *exotelHandler) parseCallDetailsXML(ctx context.Context, body []byte) (*exotelCallDetails, error) {
	var callDetails exotelCallDetails
	if err := xml.Unmarshal(body, &callDetails); err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}
	return &callDetails, nil
}

func (eh *exotelHandler) calculateDuration(callDetails *exotelCallDetails, startTime, endTime time.Time) int {
	var durationSeconds int
	if callDetails.Call.Duration != "" {
		if d, err := strconv.Atoi(callDetails.Call.Duration); err == nil {
			durationSeconds = d
		}
	}

	if durationSeconds == 0 && !startTime.IsZero() && !endTime.IsZero() {
		durationSeconds = int(endTime.Sub(startTime).Seconds())
	}

	return durationSeconds
}

func (eh *exotelHandler) parseTimestamps(callDetails *exotelCallDetails) (time.Time, time.Time) {
	layout := "2006-01-02 15:04:05"
	var startTime, endTime time.Time

	if callDetails.Call.StartTime != "" {
		startTime, _ = time.Parse(layout, callDetails.Call.StartTime)
	}
	if callDetails.Call.EndTime != "" {
		endTime, _ = time.Parse(layout, callDetails.Call.EndTime)
	}

	istLocation, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return startTime, endTime
	}

	if callDetails.Call.StartTime != "" {
		if t, err := time.ParseInLocation(layout, callDetails.Call.StartTime, istLocation); err == nil {
			startTime = t.UTC()
		}
	}
	if callDetails.Call.EndTime != "" {
		if t, err := time.ParseInLocation(layout, callDetails.Call.EndTime, istLocation); err == nil {
			endTime = t.UTC()
		}
	}

	return startTime, endTime
}

func (eh *exotelHandler) GetCallDetails(
	ctx context.Context,
	callSid string,
	creds map[string]string,
) (*callProvider.CallDetails, error) {
	credentials, err := eh.loadCredentials(creds)
	if err != nil {
		return nil, xerrors.InternalError(ctx, errors.New("failed to marshal credential"))
	}

	body, err := eh.fetchCallDetails(ctx, credentials.AccountSid, callSid, credentials.ApiKey, credentials.ApiToken)
	if err != nil {
		return nil, err
	}

	callDetails, err := eh.parseCallDetailsXML(ctx, body)
	if err != nil {
		return nil, err
	}

	startTime, endTime := eh.parseTimestamps(callDetails)
	durationSeconds := eh.calculateDuration(callDetails, startTime, endTime)

	eh.logger.Info().Ctx(ctx).
		Str("call_sid", callDetails.Call.Sid).
		Int("duration_seconds", durationSeconds).
		Time("start_time", startTime).
		Time("end_time", endTime).
		Msg("Final call duration calculated")

	return &callProvider.CallDetails{
		Sid:          callDetails.Call.Sid,
		Status:       callDetails.Call.Status,
		StartTime:    startTime,
		EndTime:      endTime,
		Duration:     durationSeconds,
		From:         callDetails.Call.From,
		To:           callDetails.Call.To,
		RecordingUrl: callDetails.Call.RecordingUrl,
	}, nil
}

func (eh *exotelHandler) ReadMessages(
	ctx context.Context,
	c *websocket.Conn,
	stt clientTypes.STTClient,
	agentId string,
	streamSidChan chan<- string,
	markChan chan<- string,
) error {
	return eh.readMessages(ctx, c, agentId, streamSidChan, markChan, "Starting to read Exotel messages", func(ctx context.Context, message []byte) error {
		return eh.HandleMediaMessage(ctx, message, stt, agentId)
	})
}

// Extract the switch statement logic into a separate helper.
//
//nolint:cyclop
func (eh *exotelHandler) processExotelMessage(
	ctx context.Context,
	baseMsg exotelInboundMessage,
	message []byte,
	agentId string,
	streamSidChan chan<- string,
	markChan chan<- string,
	handleMedia func(ctx context.Context, message []byte) error,
) error {
	switch baseMsg.Event {
	case "connected":
		if err := eh.HandleConnectedMessage(ctx, message, agentId); err != nil {
			return err
		}

	case "start":
		streamSid, err := eh.HandleStartMessage(ctx, message, agentId)
		if err != nil {
			return err
		}
		select {
		case streamSidChan <- streamSid:
			eh.logger.Debug().Ctx(ctx).Str("agent_id", agentId).Str("stream_sid", streamSid).Msg("Extracted streamSid from Exotel start message")
		case <-ctx.Done():
			return ctx.Err()
		}

	case "media":
		if err := handleMedia(ctx, message); err != nil {
			return err
		}

	case "stop":
		if err := eh.HandleStopMessage(ctx, message, agentId); err != nil {
			return err
		}
		eh.logger.Info().Ctx(ctx).Str("agent_id", agentId).Msg("Exotel stream stopped")
		return errors.New("stream_stopped") // Signal to exit loop

	case "dtmf":
		if err := eh.HandleDTMFMessage(ctx, message, agentId); err != nil {
			return err
		}

	case "mark":
		if err := eh.HandleMarkMessage(ctx, message, agentId, markChan); err != nil {
			return err
		}

	default:
		eh.logger.Warn().Ctx(ctx).
			Str("agent_id", agentId).
			Str("event", baseMsg.Event).
			Msg("Received unknown Exotel message type")
	}

	return nil
}

func (eh *exotelHandler) readMessages(
	ctx context.Context,
	c *websocket.Conn,
	agentId string,
	streamSidChan chan<- string,
	markChan chan<- string,
	startMsg string,
	handleMedia func(ctx context.Context, message []byte) error,
) error {
	eh.logger.Debug().Ctx(ctx).Str("agent_id", agentId).Msg(startMsg)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, message, err := c.ReadMessage()
			if err != nil {
				if eh.isExpectedCloseError(ctx, err, agentId) {
					return nil
				}
				eh.logger.Error().Ctx(ctx).Err(err).
					Str("agent_id", agentId).
					Msg("Failed to read Exotel message")
				return fmt.Errorf("failed to read Exotel message: %w", err)
			}

			var baseMsg exotelInboundMessage
			if err := json.Unmarshal(message, &baseMsg); err != nil {
				eh.logger.Error().Ctx(ctx).Err(err).
					Str("agent_id", agentId).
					Msg("Failed to unmarshal Exotel message")
				return fmt.Errorf("failed to unmarshal Exotel message: %w", err)
			}

			err = eh.processExotelMessage(ctx, baseMsg, message, agentId, streamSidChan, markChan, handleMedia)
			if err != nil && err.Error() == "stream_stopped" {
				return nil // Stop event received
			}
			if err != nil {
				return err
			}
		}
	}
}

func (eh *exotelHandler) isExpectedCloseError(ctx context.Context, err error, agentId string) bool {
	if websocket.IsCloseError(err, websocket.CloseNormalClosure) ||
		strings.Contains(err.Error(), "use of closed network connection") {
		eh.logger.Info().Ctx(ctx).Str("agent_id", agentId).Msg("WebSocket closed, exiting Exotel reader")
		return true
	}
	return false
}

func (eh *exotelHandler) HandleConnectedMessage(ctx context.Context, message []byte, agentId string) error {
	var connectedMsg exotelConnectedMessage
	if err := json.Unmarshal(message, &connectedMsg); err != nil {
		return fmt.Errorf("failed to unmarshal connected message: %w", err)
	}

	eh.logger.Info().Ctx(ctx).Str("agent_id", agentId).Msg("Exotel WebSocket connected")

	return nil
}

func (eh *exotelHandler) HandleStartMessage(ctx context.Context, message []byte, agentId string) (string, error) {
	var startMsg exotelStartMessage
	if err := json.Unmarshal(message, &startMsg); err != nil {
		return "", fmt.Errorf("failed to unmarshal start message: %w", err)
	}

	eh.logger.Info().Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", startMsg.StreamSid).
		Str("call_sid", startMsg.Start.CallSid).
		Interface("custom_params", startMsg.Start.CustomParameters).
		Msg("Exotel stream started")

	return startMsg.StreamSid, nil
}

func (eh *exotelHandler) decodeMediaPayload(ctx context.Context, message []byte, agentId string) ([]byte, error) {
	var mediaMsg exotelInboundMediaMessage
	if err := json.Unmarshal(message, &mediaMsg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal media message: %w", err)
	}

	audioBytes, err := base64.StdEncoding.DecodeString(mediaMsg.Media.Payload)
	if err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentId).Msg("Failed to decode audio payload")
		return nil, fmt.Errorf("failed to decode audio payload: %w", err)
	}

	return audioBytes, nil
}

func (eh *exotelHandler) HandleMediaMessage(ctx context.Context, message []byte, stt clientTypes.STTClient, agentId string) error {
	audioBytes, err := eh.decodeMediaPayload(ctx, message, agentId)
	if err != nil {
		return err
	}

	if err := stt.SendAudio(ctx, audioBytes); err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentId).Msg("Failed to send audio to STT")
		return fmt.Errorf("failed to send audio to STT: %w", err)
	}

	return nil
}

func (eh *exotelHandler) ReadMessagesRealtime(
	ctx context.Context,
	c *websocket.Conn,
	realtime clientTypes.RealtimeAgentClient,
	agentId string,
	streamSidChan chan<- string,
	markChan chan<- string,
) error {
	return eh.readMessages(ctx, c, agentId, streamSidChan, markChan, "Starting to read Exotel messages (Realtime mode)", func(ctx context.Context, message []byte) error {
		return eh.HandleMediaMessageRealtime(ctx, message, realtime, agentId)
	})
}

func (eh *exotelHandler) HandleMediaMessageRealtime(ctx context.Context, message []byte, realtime clientTypes.RealtimeAgentClient, agentId string) error {
	audioBytes, err := eh.decodeMediaPayload(ctx, message, agentId)
	if err != nil {
		return err
	}

	if err := realtime.SendAudio(ctx, audioBytes); err != nil {
		eh.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentId).Msg("Failed to send audio to Realtime API")
		return fmt.Errorf("failed to send audio to Realtime API: %w", err)
	}

	return nil
}

func (eh *exotelHandler) HandleStopMessage(ctx context.Context, message []byte, agentId string) error {
	var stopMsg exotelStopMessage
	if err := json.Unmarshal(message, &stopMsg); err != nil {
		return fmt.Errorf("failed to unmarshal stop message: %w", err)
	}

	eh.logger.Info().Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", stopMsg.StreamSid).
		Str("call_sid", stopMsg.Stop.CallSid).
		Msg("Exotel stream stopped")

	return nil
}

func (eh *exotelHandler) HandleDTMFMessage(ctx context.Context, message []byte, agentId string) error {
	var dtmfMsg exotelDTMFMessage
	if err := json.Unmarshal(message, &dtmfMsg); err != nil {
		return fmt.Errorf("failed to unmarshal DTMF message: %w", err)
	}

	eh.logger.Info().Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", dtmfMsg.StreamSid).
		Str("digit", dtmfMsg.DTMF.Digit).
		Str("track", dtmfMsg.DTMF.Duration).
		Msg("Received DTMF from Twilio")

	return nil
}

func (eh *exotelHandler) HandleMarkMessage(ctx context.Context, message []byte, agentId string, markChan chan<- string) error {
	var markMsg exotelInboundMarkMessage
	if err := json.Unmarshal(message, &markMsg); err != nil {
		return fmt.Errorf("failed to unmarshal mark message: %w", err)
	}

	eh.logger.Debug().Ctx(ctx).
		Str("agent_id", agentId).
		Str("stream_sid", markMsg.StreamSid).
		Str("mark_name", markMsg.Mark.Name).
		Msg("Received mark confirmation from Exotel")

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
