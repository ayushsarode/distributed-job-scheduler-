package deepgram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/internal/types"
	api "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/websocket/interfaces"
	interfaces "github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/pkg/client/listen"
	"github.com/rs/zerolog"
)

const (
	audioInputBufferSize  = 100
	transcriptsBufferSize = 300
)

type DgClient struct {
	audioInput  chan []byte
	transcripts chan string
	conn        *client.WSCallback
	logger      *zerolog.Logger
	done        chan struct{}
}

var _ types.STTClient = (*DgClient)(nil)

type Service struct {
	logger *zerolog.Logger
}

func NewService(ctx context.Context) *Service {
	return &Service{
		logger: zerolog.Ctx(ctx),
	}
}

func (s *Service) Connect(ctx context.Context, encoding pb.AudioEncoding, sampleRate int) (types.STTClient, error) {
	return newDGClient(ctx, encoding, sampleRate)
}

func getTranscriptionOptions(encoding pb.AudioEncoding, sampleRate int) *interfaces.LiveTranscriptionOptions {
	switch encoding {
	case pb.AudioEncoding_AUDIO_ENCODING_UNKNOWN:
		return &interfaces.LiveTranscriptionOptions{
			Model:    "nova-3",
			Language: "multi",
		}
	case pb.AudioEncoding_AUDIO_ENCODING_MULAW:
		return &interfaces.LiveTranscriptionOptions{
			Model:      "nova-3",
			Language:   "multi",
			Encoding:   "mulaw",
			SampleRate: sampleRate,
		}
	case pb.AudioEncoding_AUDIO_ENCODING_MP3:
		return &interfaces.LiveTranscriptionOptions{
			Model:    "nova-3",
			Language: "multi",
		}
	case pb.AudioEncoding_AUDIO_ENCODING_LINEAR16:
		return &interfaces.LiveTranscriptionOptions{
			Model:      "nova-3",
			Language:   "multi",
			Encoding:   "linear16",
			SampleRate: sampleRate,
		}
	case pb.AudioEncoding_AUDIO_ENCODING_OGG_OPUS:
		return &interfaces.LiveTranscriptionOptions{
			Model:      "nova-3",
			Language:   "multi",
			Encoding:   "ogg-opus",
			SampleRate: sampleRate,
		}
	default:
		return &interfaces.LiveTranscriptionOptions{
			Model:    "nova-3",
			Language: "multi",
		}
	}
}

func newDGClient(ctx context.Context, encoding pb.AudioEncoding, sampleRate int) (*DgClient, error) {
	DEEPGRAM_KEY := os.Getenv("DEEPGRAM_KEY")
	if DEEPGRAM_KEY == "" {
		zerolog.Ctx(ctx).Fatal().Msg("DEEPGRAM KEY NOT FOUND IN ENV")
	}

	dgClient := &DgClient{
		audioInput:  make(chan []byte, audioInputBufferSize),
		transcripts: make(chan string, transcriptsBufferSize),
		logger:      zerolog.Ctx(ctx),
		done:        make(chan struct{}),
	}

	cOptions := &interfaces.ClientOptions{
		EnableKeepAlive: true,
	}

	// @TODO: Make encoding configurable

	// tOptions := &interfaces.LiveTranscriptionOptions{
	// 	Model:      "nova-3",
	// 	Language:   "multi",
	// 	Encoding:   "mulaw",   // Twilio sends audio as mulaw
	// 	SampleRate: 8000,      // Twilio uses 8000 Hz sample rate
	// 	// Endpointing: "false",
	// }

	tOptions := getTranscriptionOptions(encoding, sampleRate)

	connection, err := client.NewWSUsingCallback(ctx, DEEPGRAM_KEY, cOptions, tOptions, dgClient)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to Deepgram: %w", err)
	}

	dgClient.conn = connection

	if !connection.Connect() {
		return nil, errors.New("unable to connect to Deepgram client")
	}

	// Start the streaming in a separate goroutine
	go dgClient.startStreaming()

	return dgClient, nil
}

// startStreaming begins the Deepgram streaming process.
func (d *DgClient) startStreaming() {
	defer d.logger.Debug().Msg("Exiting Deepgram streaming goroutine")
	err := d.conn.Stream(d)
	if err != nil {
		d.logger.Error().Err(err).Msg("Error in Deepgram stream")
	}
}

// SendAudio sends audio data to the STT service.
func (d *DgClient) SendAudio(ctx context.Context, audio []byte) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.done:
		return errors.New("DgClient is closed")
	case d.audioInput <- audio:
		return nil
	}
}

// Responses returns a read-only channel of transcription responses.
func (d *DgClient) Responses() <-chan string {
	return d.transcripts
}

// Disconnect terminates the connection and cleans up resources.
func (d *DgClient) Disconnect(ctx context.Context) {
	d.logger.Debug().Msg("Disconnecting DgClient")
	select {
	case <-d.done:
		// Already closed
		return
	default:
		close(d.done)
	}
	// Close channels
	close(d.audioInput)
	close(d.transcripts)
	if d.conn != nil {
		d.conn.Finish()
	}
}

// Read implements the io.Reader interface for Deepgram's streaming.
func (d *DgClient) Read(p []byte) (int, error) {
	select {
	case <-d.done:
		return 0, io.EOF
	case audio, ok := <-d.audioInput:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, audio)
		return n, nil
	}
}

// Implementing Deepgram Callback Methods.
func (d *DgClient) Open(or *api.OpenResponse) error {
	d.logger.Debug().Msg("Deepgram connection opened")
	return nil
}

func (d *DgClient) Close(cr *api.CloseResponse) error {
	d.logger.Debug().Msg("Deepgram connection closed")
	return nil
}

func (d *DgClient) Error(er *api.ErrorResponse) error {
	// TODO: return error to err channel
	d.logger.Error().Msgf("Deepgram error: %v", er)
	return nil
}

func (d *DgClient) Message(mr *api.MessageResponse) error {
	select {
	case <-d.done:
		return nil
	default:
	}

	if mr.IsFinal {
		transcript := mr.Channel.Alternatives[0].Transcript
		if transcript != "" {
			select {
			case d.transcripts <- transcript:
			case <-d.done:
				return nil
			}
		}
	}
	return nil
}

func (d *DgClient) Metadata(md *api.MetadataResponse) error {
	d.logger.Debug().Msgf("Metadata: %+v", md)
	return nil
}

// UnhandledEvent handles any unhandled events.
func (d *DgClient) UnhandledEvent(byData []byte) error {
	d.logger.Debug().Msgf("Unhandled event: %s", string(byData))
	return nil
}

// SpeechStarted handles the SpeechStarted event.
func (d *DgClient) SpeechStarted(ssr *api.SpeechStartedResponse) error {
	d.logger.Debug().Msg("Speech started")
	return nil
}

// UtteranceEnd handles the UtteranceEnd event.
func (d *DgClient) UtteranceEnd(ur *api.UtteranceEndResponse) error {
	d.logger.Debug().Msg("Utterance ended")
	return nil
}
