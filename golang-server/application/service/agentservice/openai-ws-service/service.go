package openaiwsservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"exiro.ai/application/assert"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/agentservice/openai-ws-service/sessionmanager"
	"exiro.ai/application/service/internal/types"
	servicetypes "exiro.ai/application/service/types"
	"github.com/openai/openai-go/responses"
	"github.com/rs/zerolog"
)



// AgentService implements servicetypes.AgentService using the OpenAI Responses API WebSocket.
type AgentService struct {
	logger         *zerolog.Logger
	agentRepo      types.AgentRepository
	sessionManager *sessionmanager.Manager
	apiKey         string
}

var _ servicetypes.AgentService = (*AgentService)(nil)

// NewAgentService creates a new OpenAI WebSocket AgentService.
func NewAgentService(ctx context.Context, agentRepo types.AgentRepository) (*AgentService, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	assert.NotEmpty(apiKey, "OPENAI_API_KEY is required")

	logger := zerolog.Ctx(ctx)
	const staleSessionMinutes = 30
	mgr := sessionmanager.NewManager(logger, sessionmanager.ManagerConfig{
		StaleSessionTimeout: staleSessionMinutes * time.Minute,
	})

	mgr.StartSweeper(ctx)

	return &AgentService{
		logger:         logger,
		agentRepo:      agentRepo,
		sessionManager: mgr,
		apiKey:         apiKey,
	}, nil
}

func (s *AgentService) cleanupSession(sessionID string) {
	_ = s.sessionManager.Close(sessionID)
}

// ValidateAgent checks that the agent exists.
func (s *AgentService) ValidateAgent(ctx context.Context, agentId string) error {
	_, err := s.agentRepo.INTERNALGetAgent(ctx, agentId)
	if err != nil {
		return xerrors.NotFoundError(ctx, fmt.Errorf("agent %s: %w", agentId, err))
	}
	return nil
}

// DisconnectAgent closes the session and removes it from the registry.
func (s *AgentService) DisconnectAgent(ctx context.Context, agentId string, sessionId string) error {
	s.cleanupSession(sessionId)
	return nil
}

// Invoke sends messages to the model and returns the response.
// It establishes a session, sends the input, then delegates to invokeLoop for streaming.
func (s *AgentService) Invoke(ctx context.Context, agentID string, sessionId string, messages []servicetypes.AgentMessage) (servicetypes.AgentResponse, error) {
	handle, err := s.getOrCreateSession(ctx, agentID, sessionId)
	if err != nil {
		return servicetypes.AgentResponse{}, err
	}

	input := BuildInputItems(messages)
	prevID := handle.PrevResponseID()
	gen := true
	envelope, err := BuildResponseCreateEnvelope(ResponseCreateOpts{
		Model:              handle.Model(),
		Instructions:       handle.Instructions(),
		Input:              input,
		PreviousResponseID: prevID,
		Generate:           &gen,
	})
	if err != nil {
		s.cleanupSession(sessionId)
		return servicetypes.AgentResponse{}, xerrors.InternalError(ctx, fmt.Errorf("build invoke envelope: %w", err))
	}
	if err := handle.WriteEnvelope(envelope); err != nil {
		s.cleanupSession(sessionId)
		return servicetypes.AgentResponse{}, xerrors.InternalError(ctx, fmt.Errorf("write invoke request: %w", err))
	}

	return s.invokeLoop(ctx, handle, gen)
}

// invokeLoop processes streaming responses until completion or error.
// Handles function calls by executing them and sending results back, continuing until done.
func (s *AgentService) invokeLoop(ctx context.Context, handle *sessionmanager.SessionHandle, gen bool) (servicetypes.AgentResponse, error) {
	for {
		result, err := s.processStream(ctx, handle)
		if err != nil {
			if result != nil && (result.ErrorCode == "previous_response_not_found" || result.ErrorCode == "websocket_connection_limit_reached") {
				s.cleanupSession(handle.SessionID())
			}
			return servicetypes.AgentResponse{}, err
		}

		if result.FunctionCall != nil {
			if err := s.handleFunctionCallInLoop(ctx, handle, result, gen); err != nil {
				return servicetypes.AgentResponse{}, err
			}
			continue
		}

		if result.Completed {
			if result.ResponseID != "" {
				handle.SetPrevResponseID(result.ResponseID)
			}
			return servicetypes.AgentResponse{
				Message:          result.Message,
				Language:         handle.Language(),
				CutCall:          handle.PendingCutCall(),
				Cut_call_message: handle.CutCallMessage(),
			}, nil
		}
	}
}

// handleFunctionCallInLoop executes a function call and sends its output back to the model.
func (s *AgentService) handleFunctionCallInLoop(ctx context.Context, handle *sessionmanager.SessionHandle, result *StreamResult, gen bool) error {
	output, err := s.handleFunctionCall(ctx, handle, result.FunctionCall)
	if err != nil {
		s.cleanupSession(handle.SessionID())
		return err
	}
	if err := s.sendFunctionCallOutput(ctx, handle, result, output, gen); err != nil {
		s.cleanupSession(handle.SessionID())
		return err
	}
	return nil
}

func (s *AgentService) sendFunctionCallOutput(ctx context.Context, handle *sessionmanager.SessionHandle, result *StreamResult, output string, gen bool) error {
	fcInput := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfFunctionCallOutput(result.FunctionCall.CallID, output),
	}
	envelope, err := BuildResponseCreateEnvelope(ResponseCreateOpts{
		Model:              handle.Model(),
		Instructions:       handle.Instructions(),
		Input:              fcInput,
		PreviousResponseID: result.ResponseID,
		Generate:           &gen,
	})
	if err != nil {
		return xerrors.InternalError(ctx, fmt.Errorf("build function output envelope: %w", err))
	}
	if result.ResponseID != "" {
		handle.SetPrevResponseID(result.ResponseID)
	}
	if err := handle.WriteEnvelope(envelope); err != nil {
		return xerrors.InternalError(ctx, fmt.Errorf("write function output: %w", err))
	}
	return nil
}

func (s *AgentService) handleFunctionCall(_ context.Context, handle *sessionmanager.SessionHandle, fc *FunctionCallEvent) (string, error) {
	switch fc.Name {
	case "set_language":
		return s.handleSetLanguage(handle, fc.Arguments)
	case "cut_call":
		return s.handleCutCall(handle, fc.Arguments)
	default:
		return `{"success":false,"error":"unknown tool"}`, nil
	}
}

func (s *AgentService) handleSetLanguage(handle *sessionmanager.SessionHandle, args string) (string, error) {
	var payload struct {
		Language string `json:"language"`
	}
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return `{"success":false,"error":"invalid arguments"}`, err
	}
	lang, err := ParseLanguage(payload.Language)
	if err != nil {
		return `{"success":false,"error":"` + err.Error() + `"}`, err
	}
	handle.SetLanguage(lang)
	return `{"success":true}`, nil
}

func (s *AgentService) handleCutCall(handle *sessionmanager.SessionHandle, args string) (string, error) {
	var payload struct {
		CutCallMessage string `json:"cut_call_message"`
	}
	if err := json.Unmarshal([]byte(args), &payload); err != nil {
		return `{"success":false,"error":"invalid arguments"}`, err
	}
	if payload.CutCallMessage == "" {
		return `{"success":false,"error":"cut_call_message is required"}`, nil
	}
	handle.SetPendingCutCall(true, payload.CutCallMessage)
	return `{"success":true}`, nil
}

// UpdateContext sends context as a system message with generate: false.
func (s *AgentService) UpdateContext(ctx context.Context, agentID, sessionID, contextText string) error {
	handle, err := s.getOrCreateSession(ctx, agentID, sessionID)
	if err != nil {
		return err
	}

	input := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(contextText, responses.EasyInputMessageRoleSystem),
	}
	prevID := handle.PrevResponseID()
	gen := false
	envelope, err := BuildResponseCreateEnvelope(ResponseCreateOpts{
		Model:              handle.Model(),
		Instructions:       handle.Instructions(),
		Input:              input,
		PreviousResponseID: prevID,
		Generate:           &gen,
	})
	if err != nil {
		return xerrors.InternalError(ctx, fmt.Errorf("build update context envelope: %w", err))
	}
	if err := handle.WriteEnvelope(envelope); err != nil {
		s.cleanupSession(sessionID)
		return xerrors.InternalError(ctx, fmt.Errorf("write update context: %w", err))
	}

	result, err := s.processStream(ctx, handle)
	if err != nil {
		if result != nil && (result.ErrorCode == "previous_response_not_found" || result.ErrorCode == "websocket_connection_limit_reached") {
			s.cleanupSession(sessionID)
		}
		return err
	}
	if result.ResponseID != "" {
		handle.SetPrevResponseID(result.ResponseID)
	}
	return nil
}

func (s *AgentService) getOrCreateSession(ctx context.Context, agentID, sessionID string) (*sessionmanager.SessionHandle, error) {
	if handle, ok := s.sessionManager.Get(sessionID); ok {
		return handle, nil
	}

	agent, err := s.agentRepo.INTERNALGetAgent(ctx, agentID)
	if err != nil {
		return nil, xerrors.NotFoundError(ctx, fmt.Errorf("agent %s: %w", agentID, err))
	}

	agentModel, err := agent.GetModel()
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, fmt.Errorf("invalid agent model: %w", err))
	}
	modelStr, err := mapAgentModelToOpenAI(agentModel)
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, err)
	}

	instructions := ""
	if req := agent.CreateAgentRequest.GetAgentPromptRequest(); req != nil {
		instructions = req.GetPrompt()
	}

	handle, err := s.sessionManager.GetOrCreate(ctx, sessionmanager.CreateSessionParams{
		SessionID:    sessionID,
		Model:        modelStr,
		Instructions: instructions,
		AgentID:      agentID,
		APIKey:       s.apiKey,
	})
	if err != nil {
		return nil, xerrors.InternalError(ctx, fmt.Errorf("create session: %w", err))
	}
	return handle, nil
}
