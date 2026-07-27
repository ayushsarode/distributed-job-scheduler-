package elevenlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/types"
	serviceTypes "exiro.ai/application/service/types"
	"github.com/rs/zerolog"
)

func getOutputFormat(ctx context.Context, encoding pb.AudioEncoding, sampleRate int) (string, error) {
	if sampleRate == 0 {
		sampleRate = 44100
	}

	switch encoding {
	case pb.AudioEncoding_AUDIO_ENCODING_MP3:
		return fmt.Sprintf("mp3_%d_128", sampleRate), nil

	case pb.AudioEncoding_AUDIO_ENCODING_LINEAR16:
		return fmt.Sprintf("pcm_%d", sampleRate), nil

	case pb.AudioEncoding_AUDIO_ENCODING_UNKNOWN,
		pb.AudioEncoding_AUDIO_ENCODING_MULAW,
		pb.AudioEncoding_AUDIO_ENCODING_OGG_OPUS:
		err := errors.New("unsupported encoding: " + encoding.String())
		zerolog.Ctx(ctx).Error().Err(err).Str("encoding", encoding.String()).Msg("Unsupported audio encoding")
		return "", xerrors.BadRequestError(ctx, err)

	default:
		err := errors.New("unsupported encoding: " + encoding.String())
		zerolog.Ctx(ctx).Error().Err(err).Str("encoding", encoding.String()).Msg("Unsupported audio encoding")
		return "", xerrors.BadRequestError(ctx, err)
	}
}

type ElevenLabsClient struct {
	apiKey string
}

const (
	defaultVoiceSpeed = 0.90
)

// GenerateAudio implements types.TTSHandler.
func (e *ElevenLabsClient) GenerateAudio(ctx context.Context, message string, encoding pb.AudioEncoding, language serviceTypes.AgentLanguage, sampleRate int) ([]byte, error) {
	outputFormat, err := getOutputFormat(ctx, encoding, sampleRate)
	if err != nil {
		return nil, err
	}

	maxTokens := 40000
	if len(message) > maxTokens {
		return nil, xerrors.BadRequestError(ctx, errors.New("message is too long"))
	}

	req, err := e.createTTSRequest(ctx, message, language, outputFormat)
	if err != nil {
		return nil, err
	}

	return e.executeTTSRequest(ctx, req)
}

func (e *ElevenLabsClient) createTTSRequest(ctx context.Context, message string, language serviceTypes.AgentLanguage, outputFormat string) (*http.Request, error) {
	voiceId := "AZnzlk1XvdvUeBnXmlld"
	modelId := "eleven_flash_v2_5"
	language_code := e.getLanguage(language)

	uri := fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s/stream?output_format=%s", voiceId, outputFormat)

	body := map[string]any{
		"text":          message,
		"model_id":      modelId,
		"language_code": language_code,
		"voice_settings": map[string]any{
			"speed": defaultVoiceSpeed,
		},
	}

	jsonBody, _ := json.Marshal(body)

	headers := map[string]string{
		"xi-api-key":   e.apiKey,
		"Content-Type": "application/json",
	}

	req, err := http.NewRequestWithContext(ctx, "POST", uri, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

func (e *ElevenLabsClient) executeTTSRequest(ctx context.Context, req *http.Request) ([]byte, error) {
	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, xerrors.InternalError(ctx, errors.New("failed to generate audio: "+resp.Status))
	}

	audioData := bytes.NewBuffer(nil)
	_, err = io.Copy(audioData, resp.Body)
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	return audioData.Bytes(), nil
}

func (e *ElevenLabsClient) getLanguage(language serviceTypes.AgentLanguage) string {
	switch language {
	case serviceTypes.AgentLanguageEnglish:
		return "en"
	case serviceTypes.AgentLanguageHindi:
		return "hi"
	default:
		return "en"
	}
}

var _ types.TTSHandler = (*ElevenLabsClient)(nil)

func NewElevenLabsClient(ctx context.Context) *ElevenLabsClient {
	logger := zerolog.Ctx(ctx)
	ELEVENLABS_API_KEY := os.Getenv("ELEVENLABS_API_KEY")
	if ELEVENLABS_API_KEY == "" {
		logger.Fatal().Msg("ELEVENLABS_API_KEY NOT FOUND IN ENV")
	}
	return &ElevenLabsClient{apiKey: ELEVENLABS_API_KEY}
}
