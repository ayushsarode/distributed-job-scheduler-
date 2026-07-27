package stt

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/internal/stt/deepgram"
	"exiro.ai/application/service/internal/stt/sarvam"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/config"
	"github.com/rs/zerolog"
)

func NewSTTService(ctx context.Context) types.STTService {
	provider := config.Ctx(ctx).STT.Provider
	switch provider {
	case "deepgram":
		zerolog.Ctx(ctx).Info().Msg("Using Deepgram STT")
		return deepgram.NewService(ctx)
	case "sarvam":
		zerolog.Ctx(ctx).Info().Msg("Using Sarvam STT")
		return sarvam.NewService(ctx)
	default:
		assert.Fail("unsupported STT provider: " + provider)
		return nil
	}
}
