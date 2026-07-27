package sarvam

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/types"
	serviceTypes "exiro.ai/application/service/types"
	"github.com/rs/zerolog"
)

const (
	sarvamTTSEndpoint = "https://api.sarvam.ai/text-to-speech"
	defaultModel      = "bulbul:v2"
	defaultSpeaker    = "anushka"
)

type SarvamClient struct {
	apiKey string
	logger zerolog.Logger
}

var _ types.TTSHandler = (*SarvamClient)(nil)

func NewSarvamClient(ctx context.Context) *SarvamClient {
	apiKey := os.Getenv("SARVAM_API_KEY")
	if apiKey == "" {
		zerolog.Ctx(ctx).Fatal().Msg("SARVAM_API_KEY NOT FOUND IN ENV")
	}

	logger := *zerolog.Ctx(ctx)
	logger.Info().Msg("Successfully created Sarvam TTS client")

	return &SarvamClient{
		apiKey: apiKey,
		logger: logger,
	}
}

type sarvamTTSRequest struct {
	Inputs              []string `json:"inputs"`
	TargetLanguageCode  string   `json:"target_language_code"`
	Speaker             string   `json:"speaker"`
	Pitch               float64  `json:"pitch"`
	Pace                float64  `json:"pace"`
	Loudness            float64  `json:"loudness"`
	SpeechSampleRate    int      `json:"speech_sample_rate"`
	EnablePreprocessing bool     `json:"enable_preprocessing"`
	Model               string   `json:"model"`
	OutputAudioCodec    string   `json:"output_audio_codec,omitempty"`
}

type sarvamTTSResponse struct {
	RequestID string   `json:"request_id"`
	Audios    []string `json:"audios"`
}

// GenerateAudio implements types.TTSHandler.
func (s *SarvamClient) GenerateAudio(ctx context.Context, message string, encoding pb.AudioEncoding, language serviceTypes.AgentLanguage, sampleRate int) ([]byte, error) {
	req, err := s.createTTSRequest(ctx, message, encoding, language, sampleRate)
	if err != nil {
		return nil, err
	}

	return s.executeTTSRequest(ctx, req)
}

func (s *SarvamClient) createTTSRequest(ctx context.Context, message string, encoding pb.AudioEncoding, language serviceTypes.AgentLanguage, sampleRate int) (*http.Request, error) {
	var codec string
	switch encoding {
	case pb.AudioEncoding_AUDIO_ENCODING_MP3:
		codec = "mp3"
	case pb.AudioEncoding_AUDIO_ENCODING_MULAW:
		codec = "mulaw"
	case pb.AudioEncoding_AUDIO_ENCODING_LINEAR16:
		codec = "linear16"
	case pb.AudioEncoding_AUDIO_ENCODING_OGG_OPUS:
		codec = "opus"
	case pb.AudioEncoding_AUDIO_ENCODING_UNKNOWN:
		codec = "mp3"
	default:
		codec = "mp3"
	}

	if sampleRate == 0 {
		sampleRate = 24000
	}

	reqBody := sarvamTTSRequest{
		Inputs:              []string{message},
		TargetLanguageCode:  s.getLanguageCode(language),
		Speaker:             defaultSpeaker,
		Pitch:               0.0,
		Pace:                1.0,
		Loudness:            1.0,
		SpeechSampleRate:    sampleRate,
		EnablePreprocessing: true,
		Model:               defaultModel,
		OutputAudioCodec:    codec,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sarvamTTSEndpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Subscription-Key", s.apiKey)

	return req, nil
}

func (s *SarvamClient) executeTTSRequest(ctx context.Context, req *http.Request) ([]byte, error) {
	resp, err := http.DefaultClient.Do(req) //nolint:gosec
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.logger.Error().Ctx(ctx).Err(closeErr).Msg("sarvam: failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, xerrors.InternalError(ctx, errors.New("sarvam: TTS request failed with status "+resp.Status+": "+string(body)))
	}

	var ttsResp sarvamTTSResponse
	if err := json.NewDecoder(resp.Body).Decode(&ttsResp); err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	if len(ttsResp.Audios) == 0 {
		return nil, xerrors.InternalError(ctx, errors.New("sarvam: no audio returned in response"))
	}

	audioData, err := base64.StdEncoding.DecodeString(ttsResp.Audios[0])
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	return audioData, nil
}

func (s *SarvamClient) getLanguageCode(language serviceTypes.AgentLanguage) string {
	switch language {
	case serviceTypes.AgentLanguageEnglish:
		return "en-IN"
	case serviceTypes.AgentLanguageHindi:
		return "hi-IN"
	default:
		s.logger.Warn().Msgf("sarvam: unsupported language %v, defaulting to en-IN", language)
		return "en-IN"
	}
}