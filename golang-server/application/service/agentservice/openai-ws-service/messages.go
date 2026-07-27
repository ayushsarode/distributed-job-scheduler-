package openaiwsservice

import (
	"encoding/json"

	"exiro.ai/application/service/types"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/responses"
)

// BuildInputItems converts AgentMessage slice to OpenAI Responses API input format.
func BuildInputItems(messages []types.AgentMessage) responses.ResponseInputParam {
	items := make(responses.ResponseInputParam, 0, len(messages))
	for _, m := range messages {
		role := responses.EasyInputMessageRoleUser
		if m.Role == types.MessageRoleSystem {
			role = responses.EasyInputMessageRoleSystem
		}
		items = append(items, responses.ResponseInputItemParamOfMessage(m.Content, role))
	}
	return items
}

// BuildResponseCreateEnvelope builds the WebSocket "response.create" envelope.
func BuildResponseCreateEnvelope(opts ResponseCreateOpts) (map[string]any, error) {
	inputUnion := responses.ResponseNewParamsInputUnion{}
	if len(opts.Input) > 0 {
		inputUnion.OfInputItemList = opts.Input
	}

	params := responses.ResponseNewParams{
		Model: opts.Model,
		Store: param.NewOpt(false),
		Input: inputUnion,
		Tools: getTools(),
	}
	if opts.PreviousResponseID != "" {
		params.PreviousResponseID = param.NewOpt(opts.PreviousResponseID)
	}
	if opts.Instructions != "" {
		params.Instructions = param.NewOpt(opts.Instructions)
	}

	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}
	envelope["type"] = "response.create"
	if opts.Generate != nil {
		envelope["generate"] = *opts.Generate
	}
	return envelope, nil
}

// ResponseCreateOpts holds options for building a response.create envelope.
type ResponseCreateOpts struct {
	Model              string
	Instructions       string
	Input              responses.ResponseInputParam
	PreviousResponseID string
	Generate           *bool
}
