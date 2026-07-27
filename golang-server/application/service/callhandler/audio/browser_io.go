package audio

import (
	"context"
	"fmt"
	"sync"

	"exiro.ai/application/models/pb"
	"exiro.ai/application/voicekit"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

const (
	defaultSampleRate = 16000
	audioChanBuffer   = 64
	eventsChanBuffer  = 8
)

// compile-time interface check.
var _ voicekit.AudioIO = (*BrowserAudioIO)(nil)

type BrowserConfig struct {
	InputEncoding  pb.AudioEncoding
	OutputEncoding pb.AudioEncoding
	SampleRate     int
}

// DefaultBrowserConfig returns hardcoded values from ws_handler and service.go.
func DefaultBrowserConfig() BrowserConfig {
	return BrowserConfig{
		InputEncoding:  pb.AudioEncoding_AUDIO_ENCODING_LINEAR16,
		OutputEncoding: pb.AudioEncoding_AUDIO_ENCODING_MP3,
		SampleRate:     defaultSampleRate,
	}
}

type BrowserAudioIO struct {
	conn      *websocket.Conn
	cfg       BrowserConfig
	logger    *zerolog.Logger
	audioCh   chan voicekit.AudioChunk
	eventsCh  chan voicekit.TransportEvent
	markCh    chan string
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
	writeMu   sync.Mutex // guards conn.WriteMessage across SendAudio / SendClear.
}

func NewBrowserAudioIO(
	ctx context.Context,
	conn *websocket.Conn,
	cfg BrowserConfig,
) *BrowserAudioIO {
	markCh := make(chan string)
	close(markCh) // closed immediately — prevents deadlock, outbound calls only.

	b := &BrowserAudioIO{
		conn:     conn,
		cfg:      cfg,
		logger:   zerolog.Ctx(ctx),
		audioCh:  make(chan voicekit.AudioChunk, audioChanBuffer),
		eventsCh: make(chan voicekit.TransportEvent, eventsChanBuffer),
		markCh:   markCh,
		done:     make(chan struct{}),
	}

	return b
}

// Start begins reading from the WebSocket transport continuously.
// Must be called explicitly by the handler before forwardUserAudio.
func (b *BrowserAudioIO) Start(ctx context.Context) {
	go b.readLoop(ctx)
}

func (b *BrowserAudioIO) SendAudio(_ context.Context, audio []byte, _ voicekit.AudioMetadata) error {
	msg := &pb.AgentCallEvent{
		Event: &pb.AgentCallEvent_Audio{
			Audio: &pb.Audio{
				Audio:    audio,
				Encoding: b.cfg.OutputEncoding, // MP3 — from DefaultBrowserConfig().
			},
		},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("browser_io: SendAudio: failed to marshal proto: %w", err)
	}

	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	if err := b.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("browser_io: SendAudio: failed to write to WebSocket: %w", err)
	}

	return nil
}

func (b *BrowserAudioIO) ReceiveAudio() <-chan voicekit.AudioChunk {
	return b.audioCh
}

// SendClear signals the browser to stop playing any audio when the user interrupts.
func (b *BrowserAudioIO) SendClear(_ context.Context) error {
	msg := &pb.AgentCallEvent{
		Event: &pb.AgentCallEvent_Clear{},
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("browser_io: SendClear: failed to marshal proto: %w", err)
	}

	b.writeMu.Lock()
	defer b.writeMu.Unlock()

	if err := b.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("browser_io: SendClear: failed to write to WebSocket: %w", err)
	}

	return nil
}

// MarkEvents returns a closed channel — outbound calls only, not relevant for browser transport.
// A closed channel is returned instead of nil to prevent any range/select from deadlocking.
func (b *BrowserAudioIO) MarkEvents() <-chan string {
	return b.markCh
}

// Events returns a channel of transport-level errors from the browser to voicekit.
func (b *BrowserAudioIO) Events() <-chan voicekit.TransportEvent {
	return b.eventsCh
}

// Close is safe to call from multiple goroutines.
func (b *BrowserAudioIO) Close() error {
	b.closeOnce.Do(func() {
		close(b.done)
		b.closeErr = b.conn.Close()
	})
	return b.closeErr
}

// readLoop is extracted from readUserMessage in service.go.
func (b *BrowserAudioIO) readLoop(ctx context.Context) {
	defer close(b.audioCh)

	for {
		if isDone(ctx, b.done) {
			return
		}

		_, raw, err := b.conn.ReadMessage()
		if err != nil {
			b.logger.Error().Err(err).Msg("browser_io: WebSocket read error, ending session")
			select {
			case b.eventsCh <- voicekit.TransportEvent{Error: err}:
			default:
			}
			return
		}

		if !b.handleRawMessage(ctx, raw) {
			return
		}
	}
}

func isDone(ctx context.Context, done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func (b *BrowserAudioIO) handleRawMessage(ctx context.Context, raw []byte) bool {
	protoMsg := &pb.AgentCallEvent{}
	if err := proto.Unmarshal(raw, protoMsg); err != nil {
		b.logger.Error().Err(err).Msg("browser_io: failed to unmarshal AgentCallEvent, skipping frame")
		return true
	}

	audio, ok := protoMsg.GetEvent().(*pb.AgentCallEvent_Audio)
	if !ok {
		b.logger.Warn().
			Str("event_type", fmt.Sprintf("%T", protoMsg.GetEvent())).
			Msg("browser_io: unknown event type received, ignoring frame")
		return true
	}

	audioBytes := audio.Audio.GetAudio()
	if len(audioBytes) == 0 {
		return true
	}

	select {
	case b.audioCh <- voicekit.AudioChunk{Data: audioBytes}:
		return true
	case <-b.done:
		return false
	case <-ctx.Done():
		return false
	}
}
