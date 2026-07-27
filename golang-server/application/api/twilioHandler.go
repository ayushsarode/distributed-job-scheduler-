package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Twilio handler: /public/twilio-handler?agent-id={agentID}/session-id={sessionID}/job-id={jobItemID}.
func (r *restApi) twilioHandler(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	r.logger.Debug().Ctx(ctx).Msgf("Processing Twilio request: %s %s", req.Method, req.URL.Path)
	agentID := chi.URLParam(req, "agent-id")
	if agentID == "" {
		r.logger.Error().Ctx(ctx).Msg("Agent ID is required")
		http.Error(w, "Session ID is required", http.StatusBadRequest)
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

	r.logger.Debug().Ctx(ctx).Msgf("Handling Twilio WebSocket connection for agent ID: %s", agentID)

	// Call the service handler
	err = r.outboundCallService.HandleWSConnection(req.Context(), conn, agentID, sessionID, jobItemID)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to handle Twilio WebSocket connection")
		return
	}
}

// CallStatusWebhook represents the webhook payload from any call provider.
type CallStatusWebhook struct {
	CallSid      string `form:"CallSid" json:"CallSid"`           // Provider's call ID
	CallStatus   string `form:"CallStatus" json:"CallStatus"`     // Provider's call status
	CallDuration string `form:"CallDuration" json:"CallDuration"` // Call duration
	From         string `form:"From" json:"From"`                 // From number
	To           string `form:"To" json:"To"`                     // To number
	Timestamp    string `form:"Timestamp" json:"Timestamp"`       // Timestamp
	ErrorCode    string `form:"ErrorCode" json:"ErrorCode"`       // Error code if any
	ErrorMessage string `form:"ErrorMessage" json:"ErrorMessage"` // Error message if any
}


// callStatusWebhook handles call status webhooks from any provider
// Route: POST /public/call-status-webhook/{job-item-id}/{session-id}.
func (r *restApi) callStatusWebhook(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	r.logger.Debug().Ctx(ctx).Msgf("Processing call status webhook: %s", req.URL.Path)

	jobItemId, sessionId, err := r.extractCallStatusWebhookParams(ctx, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = req.ParseForm()
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to parse webhook form data")
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	webhook := r.extractWebhookData(req)
	r.logWebhookReceived(ctx, jobItemId, sessionId, webhook)

	err = r.processCallStatusWebhook(req.Context(), jobItemId, sessionId, webhook)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId).
			Str("session_id", sessionId).
			Str("call_sid", webhook.CallSid).
			Msg("Failed to process call status webhook")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to write response")
	}
}

func (r *restApi) extractCallStatusWebhookParams(ctx context.Context, req *http.Request) (string, string, error) {
	jobItemId := chi.URLParam(req, "job-item-id")
	if jobItemId == "" {
		r.logger.Error().Ctx(ctx).Msg("Job Item ID is required")
		return "", "", errors.New("job Item ID is required")
	}

	sessionId := chi.URLParam(req, "session-id")
	if sessionId == "" {
		r.logger.Error().Ctx(ctx).Msg("Session ID is required")
		return "", "", errors.New("session ID is required")
	}

	return jobItemId, sessionId, nil
}

func (r *restApi) extractWebhookData(req *http.Request) CallStatusWebhook {
	return CallStatusWebhook{
		CallSid:      req.FormValue("CallSid"),
		CallStatus:   req.FormValue("CallStatus"),
		CallDuration: req.FormValue("CallDuration"),
		From:         req.FormValue("From"),
		To:           req.FormValue("To"),
		Timestamp:    req.FormValue("Timestamp"),
		ErrorCode:    req.FormValue("ErrorCode"),
		ErrorMessage: req.FormValue("ErrorMessage"),
	}
}

func (r *restApi) logWebhookReceived(ctx context.Context, jobItemId, sessionId string, webhook CallStatusWebhook) {
	r.logger.Info().Ctx(ctx).
		Str("job_item_id", jobItemId).
		Str("session_id", sessionId).
		Str("call_sid", webhook.CallSid).
		Str("call_status", webhook.CallStatus).
		Str("call_duration", webhook.CallDuration).
		Str("timestamp", webhook.Timestamp).
		Str("from", webhook.From).
		Str("to", webhook.To).
		Str("error_code", webhook.ErrorCode).
		Str("error_message", webhook.ErrorMessage).
		Msg("Received call status webhook")
}

// processCallStatusWebhook processes the call status webhook using job-item-id and session-id.
func (r *restApi) processCallStatusWebhook(ctx context.Context, jobItemId, sessionId string, webhook CallStatusWebhook) error {
	// Parse job item ID
	jobItemUUID, err := uuid.Parse(jobItemId)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId).
			Msg("Invalid job item ID format")
		return err
	}

	return r.outboundCallService.ProcessCallStatusWebhook(ctx, webhook.CallSid, jobItemUUID, sessionId, webhook.CallStatus, webhook.CallDuration, webhook.Timestamp, webhook.ErrorMessage)
}
