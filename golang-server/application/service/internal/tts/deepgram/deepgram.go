package deepgram

import (
	"context"
	"os"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/types"
	serviceTypes "exiro.ai/application/service/types"
	api "github.com/deepgram/deepgram-go-sdk/pkg/api/speak/v1/rest"
	interfaces "github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/pkg/client/speak"
	"github.com/rs/zerolog"
)

type DgClient struct {
	client *api.Client
	logger *zerolog.Logger
}

var _ types.TTSHandler = (*DgClient)(nil)

func NewDGClient(ctx context.Context) *DgClient {
	DEEPGRAM_KEY := os.Getenv("DEEPGRAM_KEY")
	if DEEPGRAM_KEY == "" {
		zerolog.Ctx(ctx).Fatal().Msg("DEEPGRAM KEY NOT FOUND IN ENV")
	}

	options := &interfaces.ClientOptions{}
	client := api.New(client.NewREST(DEEPGRAM_KEY, options))

	return &DgClient{
		client: client,
		logger: zerolog.Ctx(ctx),
	}
}

// GenerateAudio implements types.TTSHandler.
func (d *DgClient) GenerateAudio(ctx context.Context, message string, encoding pb.AudioEncoding, language serviceTypes.AgentLanguage, sampleRate int) ([]byte, error) {
	var encodingStr string
	var container string
	switch encoding {
	case pb.AudioEncoding_AUDIO_ENCODING_UNKNOWN:
		encodingStr = "mp3"
	case pb.AudioEncoding_AUDIO_ENCODING_MP3:
		encodingStr = "mp3"
	case pb.AudioEncoding_AUDIO_ENCODING_MULAW:
		encodingStr = "mulaw"
		container = "none"
	case pb.AudioEncoding_AUDIO_ENCODING_LINEAR16:
		encodingStr = "linear16"
		container = "none"
	case pb.AudioEncoding_AUDIO_ENCODING_OGG_OPUS:
		encodingStr = "ogg-opus"
		
	default:
		encodingStr = "mp3"
	}

	speakOptions := interfaces.SpeakOptions{
		Model:      d.getLanguage(language),
		Encoding:   encodingStr,
		Container:  container,
		SampleRate: sampleRate,
	}
	var buf interfaces.RawResponse

	res, err := d.client.ToStream(ctx, message, &speakOptions, &buf)
	d.logger.Debug().Ctx(ctx).Msgf("Speak response: %+v", res)
	if err != nil {
		return nil, xerrors.InternalError(ctx, err)
	}

	return buf.Bytes(), nil
}

func (d *DgClient) getLanguage(language serviceTypes.AgentLanguage) string {
	switch language {
	case serviceTypes.AgentLanguageEnglish:
		return "aura-asteria-en"
	case serviceTypes.AgentLanguageHindi:
		return "aura-asteria-hi"
	default:
		return "aura-asteria-en"
	}
}
