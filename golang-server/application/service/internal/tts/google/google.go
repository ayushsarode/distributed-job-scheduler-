package google

import (
	"context"
	"os"

	texttospeech "cloud.google.com/go/texttospeech/apiv1"
	texttospeechpb "cloud.google.com/go/texttospeech/apiv1/texttospeechpb"
	"exiro.ai/application/assert"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/types"
	serviceTypes "exiro.ai/application/service/types"
	"github.com/ccoveille/go-safecast"
	"github.com/rs/zerolog"
	"google.golang.org/api/option"
)

type googleTTSClient struct {
	client *texttospeech.Client
	logger zerolog.Logger
}

func NewGoogleClient(ctx context.Context) types.TTSHandler {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	assert.True(apiKey != "", "GOOGLE_API_KEY is not set")
	client, err := texttospeech.NewClient(ctx, option.WithAPIKey(apiKey))
	assert.NoError(err, "Failed to create Google TTS client")

	logger := *zerolog.Ctx(ctx)
	logger.Info().Msg("Successfully created Google TTS client")
	return &googleTTSClient{client: client, logger: logger}
}

func (g *googleTTSClient) GenerateAudio(ctx context.Context, message string, encoding pb.AudioEncoding, language serviceTypes.AgentLanguage, sampleRate int) ([]byte, error) {
	audioConfig, err := g.getAudioConfig(ctx, encoding, sampleRate)
	if err != nil {
		return nil, err
	}
	req := &texttospeechpb.SynthesizeSpeechRequest{
		Input: &texttospeechpb.SynthesisInput{
			InputSource: &texttospeechpb.SynthesisInput_Text{Text: message},
		},
		Voice:       g.getVoiceSelectionParams(language),
		AudioConfig: audioConfig,
	}

	resp, err := g.client.SynthesizeSpeech(ctx, req)
	if err != nil {
		g.logger.Error().Ctx(ctx).Err(err).Msg("Failed to synthesize speech")
		return nil, xerrors.InternalError(ctx, err)
	}

	g.logger.Info().Ctx(ctx).Msg("Successfully synthesized speech")
	return resp.GetAudioContent(), nil
}

func (g *googleTTSClient) getAudioConfig(ctx context.Context, encoding pb.AudioEncoding, sampleRate int) (*texttospeechpb.AudioConfig, error) {
	switch encoding {
	case pb.AudioEncoding_AUDIO_ENCODING_UNKNOWN:
		g.logger.Warn().Msgf("Unsupported audio encoding: %s, defaulting to MP3", encoding.String())
		return &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		}, nil
	case pb.AudioEncoding_AUDIO_ENCODING_MP3:
		return &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		}, nil
	case pb.AudioEncoding_AUDIO_ENCODING_MULAW:
		rate, err := safecast.Convert[int32](sampleRate)
		if err != nil {
			g.logger.Error().Err(err).Int("sample_rate", sampleRate).Msg("Invalid sample rate for MULAW")
			return nil, xerrors.BadRequestError(ctx, err)
		}
		return &texttospeechpb.AudioConfig{
			AudioEncoding:   texttospeechpb.AudioEncoding_MULAW,
			SampleRateHertz: rate,
		}, nil
	case pb.AudioEncoding_AUDIO_ENCODING_LINEAR16:
		rate, err := safecast.Convert[int32](sampleRate)
		if err != nil {
			g.logger.Error().Err(err).Int("sample_rate", sampleRate).Msg("Invalid sample rate for LINEAR16")
			return nil, xerrors.BadRequestError(ctx, err)
		}
		return &texttospeechpb.AudioConfig{
			AudioEncoding:   texttospeechpb.AudioEncoding_LINEAR16,
			SampleRateHertz: rate,
		}, nil
	case pb.AudioEncoding_AUDIO_ENCODING_OGG_OPUS:
		g.logger.Warn().Msgf("Unsupported audio encoding: %s, defaulting to MP3", encoding.String())
		return &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		}, nil
	default:
		g.logger.Warn().Msgf("Unsupported audio encoding: %s, defaulting to MP3", encoding.String())
		return &texttospeechpb.AudioConfig{
			AudioEncoding: texttospeechpb.AudioEncoding_MP3,
		}, nil
	}
}

func (g *googleTTSClient) getVoiceSelectionParams(language serviceTypes.AgentLanguage) *texttospeechpb.VoiceSelectionParams {
	switch language {
	case serviceTypes.AgentLanguageEnglish:
		g.logger.Info().Msg("Using English voice")
		return &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "en-IN",
			Name:         "en-IN-Chirp3-HD-Callirrhoe",
		}
	case serviceTypes.AgentLanguageHindi:
		g.logger.Info().Msg("Using Hindi voice")
		return &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "hi-IN",
			Name:         "hi-IN-Chirp3-HD-Achernar",
		}
	default:
		g.logger.Warn().Msgf("Unsupported language: %v, defaulting to English", language)
		return &texttospeechpb.VoiceSelectionParams{
			LanguageCode: "en-IN",
			Name:         "en-IN-Chirp3-HD-Callirrhoe",
		}
	}
}
