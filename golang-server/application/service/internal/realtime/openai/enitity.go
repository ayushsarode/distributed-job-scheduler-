package openai

import (
	"context"
	"errors"

	xerrors "exiro.ai/application/errors"
)

type FunctionCallEvent struct {
	CallID    string
	Name      string
	Arguments string
}

type TranscriptionEvent struct {
	Transcript string
}

// ParseFunctionCallEvent parses and validates a function call event from the Realtime API.
func ParseFunctionCallEvent(ctx context.Context, event map[string]any) (FunctionCallEvent, error) {
	if event == nil {
		return FunctionCallEvent{}, xerrors.BadRequestError(ctx, errors.New("function call event payload is nil"))
	}

	callID, _ := event["call_id"].(string)
	if callID == "" {
		return FunctionCallEvent{}, xerrors.BadRequestError(ctx, errors.New("missing or empty call_id in function call event"))
	}

	name, _ := event["name"].(string)
	if name == "" {
		return FunctionCallEvent{}, xerrors.BadRequestError(ctx, errors.New("missing or empty name in function call event"))
	}

	args, _ := event["arguments"].(string)
	if args == "" {
		return FunctionCallEvent{}, xerrors.BadRequestError(ctx, errors.New("missing or empty arguments in function call event"))
	}

	return FunctionCallEvent{
		CallID:    callID,
		Name:      name,
		Arguments: args,
	}, nil
}
