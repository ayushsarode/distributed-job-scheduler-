package realtime

import (
	"context"
	"fmt"

	"exiro.ai/application/service/internal/realtime/openai"
	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/config"
)

func NewRealtimeService(ctx context.Context) (clientTypes.RealtimeAgentService, error) {
	switch config.Ctx(ctx).Realtime.Provider {
	case config.OpenAI:
		return openai.NewOpenAIRealtimeService(ctx), nil

	default:
		return nil, fmt.Errorf("invalid realtime provider: %s", config.Ctx(ctx).Realtime.Provider)
	}
}
