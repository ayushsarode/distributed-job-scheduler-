package callProvider

import (
	"context"
	"time"

	clientTypes "exiro.ai/application/service/internal/types"
	"github.com/gorilla/websocket"
)

type CallProvider interface {
	// MakeCall initiates an outbound call
	MakeCall(
		ctx context.Context,
		fromNumber, toNumber, agentId, sessionId, jobItemId string,
		creds map[string]string,
	) (string, error)

	// ReadMessages reads and processes incoming WebSocket messages from the provider
	ReadMessages(
		ctx context.Context,
		conn *websocket.Conn,
		stt clientTypes.STTClient,
		agentId string,
		streamSidChan chan<- string,
		markChan chan<- string,
	) error

	// ReadMessagesRealtime reads and processes incoming WebSocket messages for Realtime API mode
	ReadMessagesRealtime(
		ctx context.Context,
		conn *websocket.Conn,
		realtime clientTypes.RealtimeAgentClient,
		agentId string,
		streamSidChan chan<- string,
		markChan chan<- string,
	) error

	SendAgentAudioResponse(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string, sessionId string, audioBytes int) error

	SendMediaMessage(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string) error
	SendMarkMessage(ctx context.Context, conn *websocket.Conn, streamSid, markName string) error
	SendClearMessage(ctx context.Context, conn *websocket.Conn, streamSid string) error
	SendMessage(ctx context.Context, conn *websocket.Conn, message any, messageType string) error
	SendMediaWithClear(ctx context.Context, conn *websocket.Conn, streamSid string, base64AudioPayload string) error
	HandleConnectedMessage(ctx context.Context, message []byte, agentId string) error
	HandleStartMessage(ctx context.Context, message []byte, agentId string) (string, error)
	HandleMediaMessage(ctx context.Context, message []byte, stt clientTypes.STTClient, agentId string) error
	HandleStopMessage(ctx context.Context, message []byte, agentId string) error
	HandleDTMFMessage(ctx context.Context, message []byte, agentId string) error
	HandleMarkMessage(ctx context.Context, message []byte, agentId string, markChan chan<- string) error
	GetCallDetails(ctx context.Context, callSid string, creds map[string]string) (*CallDetails, error)
}

type CallDetails struct {
	Sid          string
	Status       string
	StartTime    time.Time
	EndTime      time.Time
	Duration     int // seconds
	From         string
	To           string
	RecordingUrl string
}
