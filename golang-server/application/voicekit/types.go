package voicekit

import "context"

type AudioChunk struct {
	Data []byte
}

type AudioMetadata struct {
	Mark string
}

type TransportEvent struct {
	Error error
}

type AudioIO interface {
	Start(ctx context.Context)

	SendAudio(ctx context.Context, audio []byte, metadata AudioMetadata) error

	ReceiveAudio() <-chan AudioChunk

	SendClear(ctx context.Context) error

	MarkEvents() <-chan string

	Events() <-chan TransportEvent

	Close() error
}
