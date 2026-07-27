package deploymentservice

import (
	"context"
	"errors"
	"fmt"
	"time"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	agentServicePb "exiro.ai/application/models/protos"
	"exiro.ai/application/service/agentservice/agentutils"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type DeployService struct {
	AgentRepository      repositoryTypes.AgentRepository
	MessageQueue         repositoryTypes.MessageQueue
	logger               *zerolog.Logger
	consumer             repositoryTypes.MessageConsumer
	transactionHandler   repositoryTypes.TransationHandler
	WorkflowStatsService types.WorkflowStatsService
	auditService         types.AuditService
}

func NewDeployService(
	ctx context.Context,
	agentRepository repositoryTypes.AgentRepository,
	messageQueue repositoryTypes.MessageQueue,
	consumer repositoryTypes.MessageConsumer,
	transactionHandler repositoryTypes.TransationHandler,
	workflowStatsService types.WorkflowStatsService,
	auditService types.AuditService,
) *DeployService {
	assert.NotNil(agentRepository)
	assert.NotNil(messageQueue)
	assert.NotNil(auditService)

	return &DeployService{
		AgentRepository:      agentRepository,
		MessageQueue:         messageQueue,
		logger:               zerolog.Ctx(ctx),
		consumer:             consumer,
		transactionHandler:   transactionHandler,
		WorkflowStatsService: workflowStatsService,
		auditService:         auditService,
	}
}

var _ types.DeploymentService = &DeployService{}

func (d *DeployService) CreateAgent(ctx context.Context, request *pb.CreateAgentRequest) (entity.Agent, error) {
	user := auth.MustGetUser(ctx)
	tenantID := auth.MustGetTenant(ctx)
	currentTime := time.Now()

	var agentName string
	var model pb.AgentModel
	switch request.GetCreateAgentRequest().(type) {
	case *pb.CreateAgentRequest_AgentTemplateRequest:
		agentName = request.GetAgentTemplateRequest().GetAgentName()
		model = pb.AgentModel_AGENTMODEL_GPT_4OMINI // Default for templates
	case *pb.CreateAgentRequest_AgentPromptRequest:
		agentName = request.GetAgentPromptRequest().GetAgentName()
		model = request.GetAgentPromptRequest().GetModel()
		if model == pb.AgentModel_AGENTMODEL_UNSPECIFIED {
			return entity.Agent{}, xerrors.BadRequestError(ctx, errors.New("model must be explicitly specified"))
		}
	default:
		return entity.Agent{}, errors.New("invalid agent request type")
	}

	agentID, err := agentutils.GenerateAgentID(ctx, request.GetDeploymentType(), model)
	if err != nil {
		return entity.Agent{}, err
	}

	agent := entity.Agent{
		ID:                 agentID,
		AgentName:          agentName,
		CreateAgentRequest: request,
		CreatedAt:          currentTime,
		UpdatedAt:          currentTime,
		DeploymentStatus:   entity.DeploymentStatusNotDeployed,
		CreatedBy:          user,
		TenantID:           tenantID,
	}

	err = d.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		var txErr error
		agent, txErr = d.AgentRepository.InsertAgent(ctx, agent)
		if txErr != nil {
			return txErr
		}

		return d.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionAgentCreated,
			ResourceType: types.AuditResourceTypeAgent,
			ResourceID:   agent.ID,
		})
	})
	if err != nil {
		return entity.Agent{}, err
	}

	return agent, nil
}

func (d *DeployService) GetAgent(ctx context.Context, agentId string) (entity.Agent, error) {
	tenantID := auth.MustGetTenant(ctx)

	agent, err := d.AgentRepository.GetAgent(ctx, agentId, tenantID)
	if err != nil {
		return entity.Agent{}, err
	}

	return agent, nil
}

func (d *DeployService) ListAgents(ctx context.Context, statuses []entity.DeploymentStatus, limit int32, offset int32) ([]entity.Agent, int32, error) {
	tenantID := auth.MustGetTenant(ctx)

	agents, err := d.AgentRepository.ListAgents(ctx, tenantID, statuses, limit, offset)
	if err != nil {
		return nil, 0, err
	}

	totalCount, err := d.AgentRepository.GetAgentCount(ctx, statuses, tenantID)
	if err != nil {
		return nil, 0, err
	}

	return agents, totalCount, nil
}

func (d *DeployService) DeployAgent(ctx context.Context, agentId string) error {
	tenantID := auth.MustGetTenant(ctx)

	agent, err := d.AgentRepository.GetAgent(ctx, agentId, tenantID)
	if err != nil {
		return err
	}

	// If it's a realtime or websocket agent, deploy immediately
	if agentutils.IsRTAgent(agentId) || agentutils.IsWSAgent(agentId) {
		err := d.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
			if err := d.AgentRepository.UpdateDeploymentStatus(ctx, agent.ID, entity.DeploymentStatusSuccess, time.Now()); err != nil {
				return err
			}
			return d.auditService.Log(ctx, types.AuditEvent{
				Action:       types.AuditActionAgentDeployed,
				ResourceType: types.AuditResourceTypeAgent,
				ResourceID:   agent.ID,
			})
		})
		return err
	}

	deployAgentMessage, err := d.createDeployMessage(agentId, agent)
	if err != nil {
		return err
	}

	messagePayload, err := protojson.Marshal(deployAgentMessage)
	if err != nil {
		return fmt.Errorf("failed to marshal deploy agent message: %w", err)
	}
	// Todo: Handle in transaction using outbox pattern
	err = d.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := d.AgentRepository.UpdateDeploymentStatus(ctx, agent.ID, entity.DeploymentStatusInProgress, time.Now()); err != nil {
			return fmt.Errorf("failed to update deployment status: %w", err)
		}
		return d.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionAgentDeployed,
			ResourceType: types.AuditResourceTypeAgent,
			ResourceID:   agent.ID,
		})
	})
	if err != nil {
		return err
	}
	return d.MessageQueue.PublishStringMessage(ctx, string(messagePayload))
}

func (d *DeployService) DeploymentStatusPoller(ctx context.Context) error {
	return d.consumer.ConsumeStringMessage(ctx, func(ctx context.Context, messageBody string) error {
		var data agentServicePb.AgentDeploymentUpdate
		if err := protojson.Unmarshal([]byte(messageBody), &data); err != nil {
			d.logger.Error().Ctx(ctx).Err(err).Msg("Failed to unmarshal message body")
			return err
		}

		agentID := data.GetAgentid()
		status := entity.DeploymentStatusFailed
		if data.GetDeployed() {
			status = entity.DeploymentStatusSuccess
		}
		updatedTime := time.Now()
		if err := d.AgentRepository.UpdateDeploymentStatus(ctx, agentID, status, updatedTime); err != nil {
			d.logger.Error().Ctx(ctx).Err(err).Str("agentID", agentID).Msg("Failed to update deployment status for agent")
		}

		d.logger.Info().Ctx(ctx).Str("agentID", agentID).Str("status", status.String()).Msg("Updated deployment status for agent")
		return nil
	})
}

func (d *DeployService) UpdateAgent(ctx context.Context, agentId string, agentName string, request *pb.UpdateAgentRequest) error {
	tenantID := auth.MustGetTenant(ctx)

	agent, err := d.AgentRepository.GetAgent(ctx, agentId, tenantID)
	if err != nil {
		return err
	}

	// Validate update preconditions
	if err := d.validateUpdatePreconditions(ctx, agentId, agent); err != nil {
		return err
	}

	oldRequest := agent.CreateAgentRequest
	newRequest := request.GetCreateAgentRequest()

	// Extract model and validate prefix compatibility
	newModel, err := extractModelFromRequest(ctx, newRequest)
	if err != nil {
		return err
	}
	if err := agentutils.ValidateAgentSuffixCompatibility(ctx, agentId, newRequest.GetDeploymentType(), newModel); err != nil {
		return err
	}

	configChanged := !proto.Equal(oldRequest, newRequest)

	// update
	agent.AgentName = agentName
	agent.CreateAgentRequest = newRequest
	agent.UpdatedAt = time.Now()

	// reset deployment status if configuration changed
	if configChanged && agent.DeploymentStatus == entity.DeploymentStatusSuccess {
		agent.DeploymentStatus = entity.DeploymentStatusNotDeployed
	}

	// update agent repository
	err = d.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := d.AgentRepository.UpdateAgent(ctx, agent); err != nil {
			return err
		}
		return d.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionAgentUpdated,
			ResourceType: types.AuditResourceTypeAgent,
			ResourceID:   agent.ID,
		})
	})
	if err != nil {
		return err
	}

	return nil
}

// createDeployMessage creates the appropriate deploy message based on agent type.
func (d *DeployService) createDeployMessage(agentId string, agent entity.Agent) (*agentServicePb.DeployAgentMessage, error) {
	switch agent.CreateAgentRequest.GetCreateAgentRequest().(type) {
	case *pb.CreateAgentRequest_AgentTemplateRequest:
		return &agentServicePb.DeployAgentMessage{
			Payload: &agentServicePb.DeployAgentMessage_DeployAgentFromTemplate{
				DeployAgentFromTemplate: &agentServicePb.DeployAgentFromTemplate{
					AgentId:    agentId,
					TemplateId: agent.CreateAgentRequest.GetAgentTemplateRequest().GetTemplateId(),
					Params:     agent.CreateAgentRequest.GetAgentTemplateRequest().GetParams(),
				},
			},
		}, nil
	case *pb.CreateAgentRequest_AgentPromptRequest:
		return &agentServicePb.DeployAgentMessage{
			Payload: &agentServicePb.DeployAgentMessage_DeployAgentFromPrompt{
				DeployAgentFromPrompt: &agentServicePb.DeployAgentFromPrompt{
					AgentId: agentId,
					Prompt:  agent.CreateAgentRequest.GetAgentPromptRequest().GetPrompt(),
				},
			},
		}, nil
	default:
		return nil, errors.New("invalid agent request type")
	}
}

// validateUpdatePreconditions validates whether an agent can be updated.
func (d *DeployService) validateUpdatePreconditions(ctx context.Context, agentID string, agent entity.Agent) error {
	hasPublishedWorkflows, err := d.WorkflowStatsService.HasPublishedWorkflowsForAgent(ctx, agentID)
	if err != nil {
		return err
	}

	if hasPublishedWorkflows {
		return xerrors.PreconditionFailedError(ctx, errors.New("cannot update agent while being used in published workflows"))
	}

	if agent.DeploymentStatus == entity.DeploymentStatusInProgress {
		return xerrors.PreconditionFailedError(ctx, errors.New("cannot update agent while deployment is in progress"))
	}

	return nil
}

// extractModelFromRequest extracts the model from a CreateAgentRequest.
func extractModelFromRequest(ctx context.Context, request *pb.CreateAgentRequest) (pb.AgentModel, error) {
	switch request.GetCreateAgentRequest().(type) {
	case *pb.CreateAgentRequest_AgentTemplateRequest:
		return pb.AgentModel_AGENTMODEL_GPT_4OMINI, nil // Default for templates
	case *pb.CreateAgentRequest_AgentPromptRequest:
		model := request.GetAgentPromptRequest().GetModel()
		if model == pb.AgentModel_AGENTMODEL_UNSPECIFIED {
			return pb.AgentModel_AGENTMODEL_UNSPECIFIED, xerrors.BadRequestError(ctx, errors.New("model must be explicitly specified"))
		}
		return model, nil
	default:
		return pb.AgentModel_AGENTMODEL_UNSPECIFIED, xerrors.BadRequestError(ctx, errors.New("invalid agent request type"))
	}
}
