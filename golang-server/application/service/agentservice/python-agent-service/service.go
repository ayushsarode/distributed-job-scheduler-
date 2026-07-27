// write grpc code to connect to python agent service
package pythonagentservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	xerrors "exiro.ai/application/errors"
	agentpb "exiro.ai/application/models/protos"
	"exiro.ai/application/service/types"
	"exiro.ai/config"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const AgentCleanupTimeout = 5 * time.Second

type AgentService struct {
	logger *zerolog.Logger
	conn   *grpc.ClientConn
	client agentpb.AgentServiceClient
}

func NewAgentService(ctx context.Context) (*AgentService, error) {
	logger := zerolog.Ctx(ctx)

	conn, err := grpc.NewClient(config.Ctx(ctx).AgentService.URL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("error connecting to Python Agent Service: %w", err)
	}

	client := agentpb.NewAgentServiceClient(conn)
	return &AgentService{
		logger: logger,
		conn:   conn,
		client: client,
	}, nil
}

var _ types.AgentService = &AgentService{}

func (s *AgentService) ValidateAgent(ctx context.Context, agentId string) error {
	exists, err := s.client.ValidateAgentId(ctx, &agentpb.ValidateAgentIdRequest{Agentid: agentId})
	if err != nil {
		return fmt.Errorf("error validating agent ID: %w", err)
	}
	if !exists.GetValid() {
		return fmt.Errorf("invalid agent ID: %s", agentId)
	}
	return nil
}

func (s *AgentService) DisconnectAgent(ctx context.Context, agentId string, sessionId string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), AgentCleanupTimeout)
	defer cancel()

	_, err := s.client.DisconnectAgent(cleanupCtx, &agentpb.DisconnectAgentRequest{
		Sessionid: sessionId,
		Agentid:   agentId,
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("sessionId", sessionId).Msg("Error calling DisconnectAgent")
		return nil // Don't return error as this is cleanup
	}

	return nil
}

func (s *AgentService) Invoke(ctx context.Context, agentID string, sessionId string, messages []types.AgentMessage) (types.AgentResponse, error) {
	pbMessages := make([]*agentpb.MessageRequest, len(messages))
	for i, msg := range messages {
		switch msg.Role {
		case types.MessageRoleSystem:
			pbMessages[i] = &agentpb.MessageRequest{
				Message: &agentpb.MessageRequest_SystemMessage{
					SystemMessage: &agentpb.SystemMessageRequest{
						Content: msg.Content,
					},
				},
			}
		case types.MessageRoleUser:
			pbMessages[i] = &agentpb.MessageRequest{
				Message: &agentpb.MessageRequest_HumanMessage{
					HumanMessage: &agentpb.HumanMessageRequest{
						Content: msg.Content,
					},
				},
			}
		default:
			err := xerrors.InternalError(ctx, fmt.Errorf("unknown message role: %v", msg.Role))
			s.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map agent message role")
			return types.AgentResponse{}, err
		}
	}

	stream, err := s.client.InvokeStream(ctx, &agentpb.InvokeRequest{
		Messages:  pbMessages,
		Sessionid: sessionId,
		Agentid:   agentID,
	})
	if err != nil {
		err = xerrors.InternalError(ctx, fmt.Errorf("error starting invoke stream: %w", err))
		s.logger.Error().Ctx(ctx).Err(err).Msg("Failed to start invoke stream")
		return types.AgentResponse{}, err
	}

	return s.processInvokeStream(ctx, stream)
}

func (s *AgentService) processInvokeStream(ctx context.Context, stream agentpb.AgentService_InvokeStreamClient) (types.AgentResponse, error) {
	responseAccumulator := ""
	var config *agentpb.SystemConfig

	for {
		select {
		case <-ctx.Done():
			return types.AgentResponse{}, ctx.Err()
		default:
			resp, err := stream.Recv()
			if err != nil {
				return s.handleStreamError(err, responseAccumulator, config)
			}
			if resp != nil {
				responseAccumulator += resp.GetResponse()
				if resp.GetSystemConfig() != nil {
					config = resp.GetSystemConfig()
				}
			}
		}
	}
}

func (s *AgentService) handleStreamError(err error, responseAccumulator string, config *agentpb.SystemConfig) (types.AgentResponse, error) {
	if errors.Is(err, io.EOF) {
		return types.AgentResponse{
			Message:          responseAccumulator,
			Language:         s.getLanguage(config),
			CutCall:          s.getCutCall(config),
			Cut_call_message: s.getCutCallMessage(config),
		}, nil
	}

	statusErr, ok := status.FromError(err)
	if ok && (statusErr.Code() == codes.Unavailable || statusErr.Code() == codes.Canceled) {
		return types.AgentResponse{}, fmt.Errorf("stream error: %w", err)
	}

	return types.AgentResponse{}, fmt.Errorf("error receiving response: %w", err)
}

func (s *AgentService) UpdateContext(ctx context.Context, agentID, sessionID, contextText string) error {
	// _, err := s.client.UpdateContext(ctx, agentID, sessionID, contextText)
	_, err := s.client.UpdateAgentContext(ctx, &agentpb.UpdateAgentContextRequest{
		Agentid:   agentID,
		Sessionid: sessionID,
		Context:   contextText,
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("Unable to update context")
		return err
	}
	return nil
}

func (s *AgentService) getLanguage(config *agentpb.SystemConfig) types.AgentLanguage {
	if config == nil {
		return types.AgentLanguageEnglish
	}

	switch config.GetLanguage() {
	case agentpb.Language_LANGUAGE_ENGLISH:
		return types.AgentLanguageEnglish
	case agentpb.Language_LANGUAGE_HINDI:
		return types.AgentLanguageHindi
	case agentpb.Language_LANGUAGE_UNKNOWN:
		return types.AgentLanguageEnglish
	default:
		return types.AgentLanguageEnglish
	}
}

func (s *AgentService) getCutCall(config *agentpb.SystemConfig) bool {
	return config != nil && config.GetCutCall()
}

func (s *AgentService) getCutCallMessage(config *agentpb.SystemConfig) string {
	if config != nil && config.GetCutCall() {
		return config.GetCutCallMessage()
	}
	return ""
}
