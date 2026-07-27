package tts

import (
	"context"

	"exiro.ai/application/service/internal/tts/deepgram"
	"exiro.ai/application/service/internal/tts/elevenlabs"
	"exiro.ai/application/service/internal/tts/google"
	"exiro.ai/application/service/internal/tts/sarvam"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/config"
	"github.com/rs/zerolog"
)

func NewTTSHandler(ctx context.Context) types.TTSHandler {
	switch config.Ctx(ctx).TTS.Provider {
	case "elevenlabs":
		zerolog.Ctx(ctx).Info().Msg("Using ElevenLabs TTS")
		return elevenlabs.NewElevenLabsClient(ctx)
	case "deepgram":
		zerolog.Ctx(ctx).Info().Msg("Using Deepgram TTS")
		return deepgram.NewDGClient(ctx)
	case "google":
		zerolog.Ctx(ctx).Info().Msg("Using Google TTS")
		return google.NewGoogleClient(ctx)
	case "sarvam":
		zerolog.Ctx(ctx).Info().Msg("Using Sarvam TTS")
		return sarvam.NewSarvamClient(ctx)
	default:
		panic("unsupported TTS provider: " + config.Ctx(ctx).TTS.Provider)
	}
}
