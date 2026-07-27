package sarvam

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"

	"exiro.ai/application/assert"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/types"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
)

const (
	sarvamWSEndpoint      = "wss://api.sarvam.ai/speech-to-text/ws"
	defaultModel          = "saaras:v3"
	defaultMode           = "transcribe"
	defaultLanguageCode   = "en-IN"
	audioInputBufferSize  = 100
	transcriptsBufferSize = 300
)

type SVClient struct {
	conn        *websocket.Conn
	audioInput  chan []byte
	transcripts chan string
	logger      zerolog.Logger
	cancel      context.CancelFunc
	closeOnce   sync.Once
	sampleRate  int
	encodingStr string
}

var _ types.STTClient = (*SVClient)(nil)

type Service struct {
	logger *zerolog.Logger
}

func NewService(ctx context.Context) *Service {
	return &Service{
		logger: zerolog.Ctx(ctx),
	}
}

func (s *Service) Connect(ctx context.Context, encoding pb.AudioEncoding, sampleRate int) (types.STTClient, error) {
	return newSVClient(ctx, encoding, sampleRate)
}

type wsAudioMessage struct {
	Audio wsAudioData `json:"audio"`
}

type wsAudioData struct {
	Data       string `json:"data"`
	SampleRate int    `json:"sample_rate"`
	Encoding   string `json:"encoding"`
}

type wsResponse struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type wsTranscriptData struct {
	Transcript   string `json:"transcript"`
	LanguageCode string `json:"language_code,omitempty"`
}

func newSVClient(ctx context.Context, encoding pb.AudioEncoding, sampleRate int) (*SVClient, error) {
	apiKey := os.Getenv("SARVAM_API_KEY")
	if apiKey == "" {
		zerolog.Ctx(ctx).Fatal().Msg("SARVAM_API_KEY NOT FOUND IN ENV")
	}

	logger := *zerolog.Ctx(ctx)

	wsURL := buildWSURL(encoding, sampleRate)

	header := http.Header{}
	header.Set("Api-Subscription-Key", apiKey)

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if resp != nil && resp.Body != nil {
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				logger.Error().Ctx(ctx).Err(closeErr).Msg("failed to close handshake response body")
			}
		}()
	}
	if err != nil {
		return nil, errors.New("sarvam stt: failed to connect: " + err.Error())
	}

	clientCtx, clientCancel := context.WithCancel(ctx)

	client := &SVClient{
		conn:        conn,
		audioInput:  make(chan []byte, audioInputBufferSize),
		transcripts: make(chan string, transcriptsBufferSize),
		logger:      logger,
		cancel:      clientCancel,
		sampleRate:  sampleRate,
		encodingStr: "audio/wav",
	}

	go client.readMessages(clientCtx)
	go client.streamAudio(clientCtx)

	logger.Info().Ctx(ctx).
		Int("sampleRate", sampleRate).
		Str("encoding", encoding.String()).
		Msg("Successfully created Sarvam STT client (WebSocket)")

	return client, nil
}

func buildWSURL(encoding pb.AudioEncoding, sampleRate int) string {
	u, err := url.Parse(sarvamWSEndpoint)
	assert.NoError(err, "sarvamWSEndpoint is a constant and must always parse")

	q := u.Query()
	q.Set("model", defaultModel)
	q.Set("mode", defaultMode)
	q.Set("language-code", defaultLanguageCode)
	q.Set("sample_rate", strconv.Itoa(sampleRate))
	q.Set("high_vad_sensitivity", "true")
	q.Set("input_audio_codec", audioCodecFromEncoding(encoding))
	u.RawQuery = q.Encode()

	return u.String()
}

func audioCodecFromEncoding(encoding pb.AudioEncoding) string {
	switch encoding {
	case pb.AudioEncoding_AUDIO_ENCODING_LINEAR16:
		return "pcm_s16le"
	case pb.AudioEncoding_AUDIO_ENCODING_MULAW:
		return "pcm_raw"
	case pb.AudioEncoding_AUDIO_ENCODING_UNKNOWN,
		pb.AudioEncoding_AUDIO_ENCODING_MP3,
		pb.AudioEncoding_AUDIO_ENCODING_OGG_OPUS:
		return "pcm_s16le"
	default:
		return "audio/wav"
	}
}

func (s *SVClient) streamAudio(ctx context.Context) {
	defer s.logger.Debug().Ctx(ctx).Msg("exiting streamAudio goroutine")

	for {
		select {
		case <-ctx.Done():
			return
		case audio, ok := <-s.audioInput:
			if !ok {
				return
			}
			msg := wsAudioMessage{
				Audio: wsAudioData{
					Data:       base64.StdEncoding.EncodeToString(audio),
					SampleRate: s.sampleRate,
					Encoding:   s.encodingStr,
				},
			}
			if err := s.conn.WriteJSON(msg); err != nil {
				s.logger.Error().Ctx(ctx).Err(err).Msg("failed to write audio message")
				return
			}
		}
	}
}

func (s *SVClient) readMessages(ctx context.Context) {
	defer s.logger.Debug().Ctx(ctx).Msg("exiting readMessages goroutine")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		_, rawMsg, err := s.conn.ReadMessage()
		if err != nil {
			s.handleReadError(ctx, err)
			return
		}

		var resp wsResponse
		if err := json.Unmarshal(rawMsg, &resp); err != nil {
			s.logger.Error().Ctx(ctx).Err(err).Str("raw", string(rawMsg)).Msg("failed to unmarshal response")
			continue
		}

		if err := s.handleWSResponse(ctx, resp); err != nil {
			s.logger.Debug().Ctx(ctx).Err(err).Msg("exiting read loop")
			return
		}
	}
}

func (s *SVClient) handleReadError(ctx context.Context, err error) {
	select {
	case <-ctx.Done():
		return
	default:
	}
	if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		s.logger.Debug().Ctx(ctx).Msg("websocket closed normally")
	} else {
		s.logger.Error().Ctx(ctx).Err(err).Msg("websocket read error")
	}
}

func (s *SVClient) handleWSResponse(ctx context.Context, resp wsResponse) error {
	switch resp.Type {
	case "data":
		return s.handleTranscript(ctx, resp.Data)
	case "error":
		s.logger.Error().Ctx(ctx).RawJSON("error", resp.Data).Msg("server error")
	case "events":
		s.logger.Debug().Ctx(ctx).RawJSON("event", resp.Data).Msg("vad event")
	default:
		s.logger.Debug().Ctx(ctx).Str("type", resp.Type).Msg("unhandled message type")
	}
	return nil
}

func (s *SVClient) handleTranscript(ctx context.Context, data json.RawMessage) error {
	var transcript wsTranscriptData
	if err := json.Unmarshal(data, &transcript); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("failed to unmarshal transcript data")
		return nil
	}
	if transcript.Transcript == "" {
		return nil
	}
	select {
	case s.transcripts <- transcript.Transcript:
		return nil
	case <-ctx.Done():
		return errors.New("sarvam stt: client done")
	}
}

func (s *SVClient) SendAudio(ctx context.Context, audio []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.audioInput <- audio:
		return nil
	}
}

func (s *SVClient) Responses() <-chan string {
	return s.transcripts
}

func (s *SVClient) Disconnect(_ context.Context) {
	s.closeOnce.Do(func() {
		s.logger.Debug().Msg("disconnecting SVClient")
		s.cancel()

		_ = s.conn.WriteMessage(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		)
		_ = s.conn.Close()

		go func() {
			for range s.audioInput {
			}
		}()
		close(s.audioInput)
		close(s.transcripts)
	})
}
