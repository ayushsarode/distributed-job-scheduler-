package callhandler

import (
	"context"
	"fmt"

	"exiro.ai/application/models/pb"
	clientTypes "exiro.ai/application/service/internal/types"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// initializeClients initializes the STT and Agent clients.
func (s *Service) initializeClients(
	ctx context.Context,
	c *websocket.Conn,
) (clientTypes.STTClient, error) {
	stt, err := s.sttService.Connect(ctx, defaultEncoding, defaultSampleRate)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to create STT client")
		if closeErr := c.Close(); closeErr != nil {
			s.logger.Error().Err(closeErr).Msg("Failed to close websocket connection")
		}
		return nil, fmt.Errorf("unable to connect to STT service: %w", err)
	}
	s.logger.Info().Msg("Client connected")
	return stt, nil
}

func (s *Service) sendClearedMessage(_ context.Context, c *websocket.Conn) error {
	protoMsg := &pb.AgentCallEvent{
		Event: &pb.AgentCallEvent_Clear{},
	}
	protoBytes, _ := proto.Marshal(protoMsg)
	if err := c.WriteMessage(websocket.BinaryMessage, protoBytes); err != nil {
		s.logger.Error().Err(err).Msg("Error sending cleared message to client")
		return err
	}
	return nil
}
