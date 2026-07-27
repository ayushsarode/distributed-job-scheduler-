package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
	"github.com/go-chi/chi/v5"
)

const (
	maxMultipartMemory = 10 << 20 // 10MB
)

type ExotelResolverResponse struct {
	URL string `json:"url"`
}

// exotelResolverHandler handles the dynamic URL resolution for Exotel Voicebot
// This endpoint is configured in the Exotel Voicebot Applet as the "Dynamic Endpoint"
// Route: GET/POST /public/exotel/resolve-voicebot.
type CustomFieldData struct {
	AgentId   string `json:"agentId"`
	SessionId string `json:"sessionId"`
	JobItemId string `json:"jobItemId"`
}

func (r *restApi) exotelResolverHandler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	r.logger.Debug().Ctx(ctx).Msgf("Processing Exotel resolver request: %s %s", req.Method, req.URL.Path)

	agentID, sessionID, jobItemID, err := r.parseCustomFieldParams(req)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to parse parameters")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := r.validateResolverParams(ctx, agentID, sessionID, jobItemID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	r.logger.Info().Ctx(ctx).
		Str("agent_id", agentID).
		Str("session_id", sessionID).
		Str("job_item_id", jobItemID).
		Str("remote_addr", req.RemoteAddr).
		Msg("Exotel resolver called")

	wsURL, err := r.buildWebSocketURL(ctx, agentID, sessionID, jobItemID)
	if err != nil {
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	r.logger.Info().Ctx(ctx).
		Str("ws_url", wsURL).
		Str("agent_id", agentID).
		Str("session_id", sessionID).
		Str("job_item_id", jobItemID).
		Msg("Returning WebSocket URL to Exotel")

	r.respondWithWebSocketURL(w, ctx, wsURL)
}

func (r *restApi) parseCustomFieldParams(req *http.Request) (string, string, string, error) {
	ctx := req.Context()
	customFieldRaw := req.URL.Query().Get("CustomField")
	if customFieldRaw == "" {
		return "", "", "", errors.New("customField parameter is required")
	}

	var customData CustomFieldData
	if err := json.Unmarshal([]byte(customFieldRaw), &customData); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Str("custom_field", customFieldRaw).Msg("Failed to unmarshal CustomField JSON")
		return "", "", "", errors.New("invalid CustomField format")
	}

	r.logger.Debug().Ctx(ctx).
		Str("custom_field_raw", customFieldRaw).
		Str("parsed_agent_id", customData.AgentId).
		Msg("Parsed params from CustomField")

	return customData.AgentId, customData.SessionId, customData.JobItemId, nil
}

func (r *restApi) validateResolverParams(ctx context.Context, agentID, sessionID, jobItemID string) error {
	if agentID == "" {
		r.logger.Error().Ctx(ctx).Msg("Agent ID is required in resolver request")
		return errors.New("agent ID is required")
	}
	if sessionID == "" {
		r.logger.Error().Ctx(ctx).Msg("Session ID is required in resolver request")
		return errors.New("session ID is required")
	}
	if jobItemID == "" {
		r.logger.Error().Ctx(ctx).Msg("Job Item ID is required in resolver request")
		return errors.New("job Item ID is required")
	}
	return nil
}

func (r *restApi) buildWebSocketURL(ctx context.Context, agentID, sessionID, jobItemID string) (string, error) {
	baseURL := r.baseUrl
	if baseURL == "" {
		r.logger.Error().Ctx(ctx).Msg("BASE_URL not configured")
		return "", errors.New("BASE_URL not configured")
	}
	sampleRate, err := r.outboundCallService.GetAudioSampleRateForAgent(ctx, agentID)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Str("agent_id", agentID).Msg("Failed to get sample rate for agent")
		return "", err
	}
	wsURL := fmt.Sprintf("wss://%s/public/exotel-handler/%s/%s/%s?sample-rate=%d",
		baseURL, agentID, sessionID, jobItemID, sampleRate)

	return wsURL, nil
}

func (r *restApi) respondWithWebSocketURL(w http.ResponseWriter, ctx context.Context, wsURL string) {
	w.Header().Set("Content-Type", "application/json")
	response := ExotelResolverResponse{
		URL: wsURL,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to encode resolver response")
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// exotelHandler handles the WebSocket connection for Exotel voice stream
// Route: GET /public/exotel-handler/{agent-id}/{session-id}/{job-item-id}?sample-rate={rate}.
func (r *restApi) exotelHandler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	r.logger.Debug().Ctx(ctx).Msgf("Processing Exotel WebSocket request: %s %s", req.Method, req.URL.Path)

	agentID := chi.URLParam(req, "agent-id")
	if agentID == "" {
		r.logger.Error().Ctx(ctx).Msg("Agent ID is required")
		http.Error(w, "Agent ID is required", http.StatusBadRequest)
		return
	}

	sessionID := chi.URLParam(req, "session-id")
	if sessionID == "" {
		r.logger.Error().Ctx(ctx).Msg("Session ID is required")
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	jobItemID := chi.URLParam(req, "job-item-id")
	if jobItemID == "" {
		r.logger.Error().Ctx(ctx).Msg("Job Item ID is required")
		http.Error(w, "Job Item ID is required", http.StatusBadRequest)
		return
	}

	r.logger.Info().Ctx(ctx).
		Str("agent_id", agentID).
		Str("session_id", sessionID).
		Str("job_item_id", jobItemID).
		Msg("Exotel WebSocket connection request")

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to upgrade connection to WebSocket")
		http.Error(w, "Failed to upgrade connection", http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to close websocket connection")
		}
	}()

	r.logger.Debug().Ctx(ctx).Msgf("Handling Exotel WebSocket connection for agent ID: %s", agentID)

	// Call the service handler
	err = r.outboundCallService.HandleWSConnection(req.Context(), conn, agentID, sessionID, jobItemID)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).
			Str("agent_id", agentID).
			Str("session_id", sessionID).
			Str("job_item_id", jobItemID).
			Msg("Failed to handle Exotel WebSocket connection")
		return
	}
}

// exotelStatusWebhook handles call status webhooks from Exotel
// Route: POST /public/exotel/status-callback/{job-item-id}/{session-id}.
func (r *restApi) exotelStatusWebhook(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	r.logger.Debug().Ctx(ctx).Msgf("Processing Exotel status webhook: %s", req.URL.Path)

	jobItemId, sessionId, err := r.extractWebhookParams(req)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to extract webhook parameters")
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	webhook, err := r.parseExotelWebhookData(req)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to parse webhook data")
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	err = r.processCallStatusWebhook(req.Context(), jobItemId, sessionId, webhook)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId).
			Str("session_id", sessionId).
			Str("call_sid", webhook.CallSid).
			Msg("Failed to process Exotel status webhook")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Respond with 200 OK to acknowledge receipt
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		r.logger.Error().Err(err).Ctx(ctx).Msg("Failed to write response")
	}
}

func (r *restApi) extractWebhookParams(req *http.Request) (string, string, error) {
	jobItemId := chi.URLParam(req, "job-item-id")
	if jobItemId == "" {
		return "", "", errors.New("job item ID is required")
	}

	sessionId := chi.URLParam(req, "session-id")
	if sessionId == "" {
		return "", "", errors.New("session ID is required")
	}

	return jobItemId, sessionId, nil
}

func (r *restApi) parseExotelWebhookData(req *http.Request) (CallStatusWebhook, error) {
	// Parse form data from webhook (Exotel sends form data)
	err := req.ParseMultipartForm(maxMultipartMemory)
	if err != nil {
		return CallStatusWebhook{}, err
	}

	var durationSeconds int

	// Convert duration to string for CallStatusWebhook
	durationStr := ""
	if durationSeconds > 0 {
		durationStr = strconv.Itoa(durationSeconds)
	}

	timestampUTC := r.convertExotelTimestampToUTC(req.FormValue("DateUpdated"))

	// Extract Exotel webhook data
	webhook := CallStatusWebhook{
		CallSid:      req.FormValue("CallSid"),
		CallStatus:   req.FormValue("Status"),
		Timestamp:    timestampUTC,
		CallDuration: durationStr,
	}

	return webhook, nil
}

func (r *restApi) convertExotelTimestampToUTC(dateUpdated string) string {
	if dateUpdated == "" {
		return ""
	}

	layout := "2006-01-02 15:04:05"
	istLocation, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		return dateUpdated
	}

	istTime, err := time.ParseInLocation(layout, dateUpdated, istLocation)
	if err != nil {
		return dateUpdated
	}

	return istTime.UTC().Format(layout)
}
