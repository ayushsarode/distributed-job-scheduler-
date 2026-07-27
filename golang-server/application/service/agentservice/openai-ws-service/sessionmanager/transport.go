package sessionmanager

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/openai/openai-go/responses"
)

const (
	writeWait = 10 * time.Second
	pongWait  = 60 * time.Second
	readLimit = 1 << 20
)

// writeEnvelope sends a response.create envelope to the WebSocket.
func writeEnvelope(conn *websocket.Conn, envelope map[string]any) error {
	_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
	return conn.WriteJSON(envelope)
}

// readEnvelope reads the next stream event from the WebSocket into the SDK type.
func readEnvelope(conn *websocket.Conn) (responses.ResponseStreamEventUnion, error) {
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	var event responses.ResponseStreamEventUnion
	if err := conn.ReadJSON(&event); err != nil {
		return responses.ResponseStreamEventUnion{}, err
	}
	return event, nil
}
