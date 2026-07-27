package outboundcallservice

import (
	"context"
	"fmt"

	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/agentservice/agentutils"
	"exiro.ai/application/service/outboundcallservice/callProvider"
	"exiro.ai/application/service/outboundcallservice/callProvider/exotel"
	"exiro.ai/application/service/outboundcallservice/callProvider/twilio"
	"exiro.ai/config"
	"github.com/rs/zerolog"
)

const (
	SampleRate8KHz  = 8000
	SampleRate16KHz = 16000
	SampleRate24KHz = 24000
)

const (
	SampleRateTelephony = SampleRate8KHz
	SampleRateRealtime  = SampleRate24KHz
)

type ProviderConfig struct {
	provider    string
	ttsProvider string
	sttProvider string
}

func NewProviderConfig(ctx context.Context) *ProviderConfig {
	return &ProviderConfig{
		provider:    config.Ctx(ctx).OutboundCalling.Provider,
		ttsProvider: config.Ctx(ctx).TTS.Provider,
		sttProvider: config.Ctx(ctx).STT.Provider,
	}
}

func (pc *ProviderConfig) GetSampleRate(agentID string) int {
	switch pc.provider {
	case "twilio":
		return SampleRate8KHz
	case "exotel":
		if agentutils.IsRTAgent(agentID) {
			return SampleRate24KHz
		}

		return pc.ttsProviderSampleRate()
	default:
		return SampleRate8KHz
	}
}

func (pc *ProviderConfig) ttsProviderSampleRate() int {
	switch pc.ttsProvider {
	case "deepgram":
		return SampleRate16KHz
	case "google":
		return SampleRate16KHz
	case "sarvam":
		return SampleRate16KHz
	case "elevenlabs":
		return SampleRate24KHz
	default:
		return SampleRate24KHz
	}
}

// NewTelephonyProvider creates a new telephony provider based on config.
func (pc *ProviderConfig) NewCallProvider(logger *zerolog.Logger) (callProvider.CallProvider, error) {
	switch pc.provider {
	case "twilio":
		return twilio.NewTwilioHandler(logger), nil
	case "exotel":
		return exotel.NewExotelHandler(logger), nil
	default:
		return nil, fmt.Errorf("unsupported call provider: %s", pc.provider)
	}
}

func (pc *ProviderConfig) GetAudioEncoding() pb.AudioEncoding {
	switch pc.provider {
	case "twilio":
		// Twilio uses μ-law (PCMU) 8kHz
		return pb.AudioEncoding_AUDIO_ENCODING_MULAW
	case "exotel":
		// Exotel uses Linear PCM16
		return pb.AudioEncoding_AUDIO_ENCODING_LINEAR16
	default:
		// Default to Linear PCM16
		return pb.AudioEncoding_AUDIO_ENCODING_LINEAR16
	}
}

func (pc *ProviderConfig) GetFromPhoneNumber(ctx context.Context) string {
	switch pc.provider {
	case "twilio":
		return config.Ctx(ctx).OutboundCalling.TwilioFromPhoneNumber
	case "exotel":
		return config.Ctx(ctx).OutboundCalling.ExotelFromPhoneNumber
	default:
		return ""
	}
}
