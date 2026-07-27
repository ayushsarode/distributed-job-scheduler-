package openaiwsservice

import (
	"context"
	"fmt"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/agentservice/openai-ws-service/sessionmanager"
	"github.com/openai/openai-go/shared/constant"
)

// Event type constants from openai-go for WebSocket stream handling.
var (
	eventOutputTextDelta            = string(constant.ValueOf[constant.ResponseOutputTextDelta]())
	eventResponseCreated            = string(constant.ValueOf[constant.ResponseCreated]())
	eventResponseCompleted          = string(constant.ValueOf[constant.ResponseCompleted]())
	eventOutputItemAdded            = string(constant.ValueOf[constant.ResponseOutputItemAdded]())
	eventFunctionCallArgumentsDelta = string(constant.ValueOf[constant.ResponseFunctionCallArgumentsDelta]())
	eventFunctionCallArgumentsDone  = string(constant.ValueOf[constant.ResponseFunctionCallArgumentsDone]())
	eventError                      = string(constant.ValueOf[constant.Error]())
)

// StreamResult holds the accumulated result from processing the response stream.
type StreamResult struct {
	Message      string
	ResponseID   string
	FunctionCall *FunctionCallEvent
	Completed    bool
	Error        error
	ErrorCode    string
}

// FunctionCallEvent represents a function call from the model.
type FunctionCallEvent struct {
	CallID    string
	Name      string
	Arguments string
}

// processStream reads events from the session handle and accumulates text/response ID.
// Returns when response.completed, a function call requiring handling, or an error.
//nolint:cyclop // orchestration flow with multiple branches
func (s *AgentService) processStream(ctx context.Context, handle *sessionmanager.SessionHandle) (*StreamResult, error) {
	result := &StreamResult{}
	var pendingFC *FunctionCallEvent

	for {
		select {
		case <-ctx.Done():
			return nil, xerrors.InternalError(ctx, ctx.Err())
		default:
		}

		event, err := handle.ReadEnvelope()
		if err != nil {
			return result, xerrors.InternalError(ctx, fmt.Errorf("read envelope: %w", err))
		}

		switch event.Type {
		case eventOutputTextDelta:
			result.Message += event.Delta.OfString

		case eventResponseCreated:
			if event.Response.ID != "" {
				result.ResponseID = event.Response.ID
			}

		case eventResponseCompleted:
			if event.Response.ID != "" && result.ResponseID == "" {
				result.ResponseID = event.Response.ID
			}
			result.Completed = true
			// Return after response.completed so the API has the response (including any function call) in cache
			// before we send a continuation with function_call_output.
			return result, nil

		case eventOutputItemAdded:
			if event.Item.Type == string(constant.ValueOf[constant.FunctionCall]()) {
				pendingFC = &FunctionCallEvent{
					CallID: event.Item.CallID,
					Name:   event.Item.Name,
				}
			}

		case eventFunctionCallArgumentsDelta:
			if pendingFC != nil {
				pendingFC.Arguments += event.Delta.OfString
			}

		case eventFunctionCallArgumentsDone:
			// If we don't have a pending function call from output_item.added, create one
			if pendingFC == nil && event.ItemID != "" {
				pendingFC = &FunctionCallEvent{CallID: event.ItemID}
			}
			if pendingFC != nil {
				pendingFC.Arguments = event.Arguments
				result.FunctionCall = pendingFC
			}

		case eventError:
			result.ErrorCode = event.Code
			result.Error = fmt.Errorf("openai error %s: %s", event.Code, event.Message)
			return result, xerrors.InternalError(ctx, result.Error)
		}
	}
}
