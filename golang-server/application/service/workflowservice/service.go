package workflowservice

import (
	"context"
	"time"

	"errors"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type WorkflowService struct {
	WorkflowRepository          repositoryTypes.WorkflowRepository
	DeploymentAgentStatsService types.DeploymentAgentStatsService
	transactionHandler          repositoryTypes.TransationHandler
	logger                      *zerolog.Logger
}

func NewService(
	ctx context.Context,
	workflowRepository repositoryTypes.WorkflowRepository,
	deploymentAgentStatsService types.DeploymentAgentStatsService,
	transactionHandler repositoryTypes.TransationHandler,
) *WorkflowService {
	assert.NotNil(workflowRepository)
	assert.NotNil(deploymentAgentStatsService)

	return &WorkflowService{
		WorkflowRepository:          workflowRepository,
		DeploymentAgentStatsService: deploymentAgentStatsService,
		transactionHandler:          transactionHandler,
		logger:                      zerolog.Ctx(ctx),
	}
}

var _ types.WorkflowService = &WorkflowService{}

var activeCallJobStatuses = []entity.CallJobStatus{
	entity.CallJobProcessing,
	entity.CallJobReady,
	entity.CallJobRunning,
}

func (s *WorkflowService) CreateWorkflow(ctx context.Context, workflow entity.Workflow) (entity.Workflow, error) {
	workflow.Status = entity.WorkflowDraft

	workflow.ID = uuid.Must(uuid.NewV7())
	workflow.TenantID = auth.MustGetTenant(ctx)
	workflow.CreatedBy = auth.MustGetUser(ctx)

	workflow.CreatedAt = time.Now()
	workflow.UpdatedAt = time.Now()

	err := s.WorkflowRepository.CreateWorkflow(ctx, workflow)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflow.ID.String()).Msg("Failed to create workflow in database")
		return entity.Workflow{}, err
	}

	return workflow, nil
}

func (s *WorkflowService) GetWorkflow(ctx context.Context, workflowId uuid.UUID) (entity.Workflow, error) {
	tenantID := auth.MustGetTenant(ctx)
	workflow, err := s.WorkflowRepository.GetWorkflow(ctx, workflowId, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("unable to get workflow")
		return entity.Workflow{}, err
	}

	return workflow, nil
}

func (s *WorkflowService) INTERNALGetWorkflow(ctx context.Context, workflowId uuid.UUID) (entity.Workflow, error) {
	workflow, err := s.WorkflowRepository.INTERNALGetWorkflow(ctx, workflowId)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("unable to internally get workflow")
		return entity.Workflow{}, err
	}

	return workflow, nil
}

func (s *WorkflowService) UpdateWorkflow(ctx context.Context, workflowId uuid.UUID, name string, description string, agentId string) error {
	tenantID := auth.MustGetTenant(ctx)

	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		workflow, err := s.WorkflowRepository.GetWorkflow(ctx, workflowId, tenantID)
		if err != nil {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow not found")
			return err
		}

		if workflow.Status == entity.WorkflowPublished {
			return xerrors.PreconditionFailedError(
				ctx,
				errors.New("published workflows cannot be updated"),
			)
		}

		workflow.Name = name
		workflow.Description = description
		workflow.Agent_id = agentId
		workflow.UpdatedAt = time.Now()

		return s.WorkflowRepository.UpdateWorkflow(ctx, workflow, tenantID)
	})
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Failed to Update Workflow")
		return err
	}

	return nil
}

func (s *WorkflowService) PublishWorkflow(ctx context.Context, workflowId uuid.UUID) error {
	tenantID := auth.MustGetTenant(ctx)

	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		workflow, err := s.WorkflowRepository.GetWorkflow(ctx, workflowId, tenantID)
		if err != nil {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow not found")
			return err
		}

		if workflow.Status == entity.WorkflowPublished {
			s.logger.Info().Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow already published")
			return nil
		}

		agent, err := s.DeploymentAgentStatsService.GetAgent(ctx, workflow.Agent_id)
		if err != nil {
			s.logger.Err(err).Ctx(ctx).Str("agent_id", agent.ID).Msg("Agent not found")
			return err
		}

		if agent.DeploymentStatus != entity.DeploymentStatusSuccess {
			s.logger.Err(err).Ctx(ctx).Str("agent_id", agent.ID).Msg("Agent not deployed")
			return xerrors.PreconditionFailedError(ctx, errors.New("agent not deployed"))
		}

		workflow.Status = entity.WorkflowPublished
		workflow.UpdatedAt = time.Now()

		return s.WorkflowRepository.UpdateWorkflow(ctx, workflow, tenantID)
	})
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Failed to Publish Workflow")
		return err
	}

	return nil
}

func (s *WorkflowService) DeactivateWorkflow(ctx context.Context, workflowId uuid.UUID) error {
	tenantID := auth.MustGetTenant(ctx)

	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		workflow, err := s.WorkflowRepository.GetWorkflow(ctx, workflowId, tenantID)
		if err != nil {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow not found")
			return err
		}

		activeCount, err := s.getWorkflowActiveCallJobCount(ctx, workflowId)
		if err != nil {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow not found")
			return err
		}

		if activeCount > 0 {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow already in use")
			return xerrors.PreconditionFailedError(ctx, errors.New("workflow already in use"))
		}

		workflow.Status = entity.WorkflowInActive
		workflow.UpdatedAt = time.Now()

		return s.WorkflowRepository.UpdateWorkflow(ctx, workflow, tenantID)
	})
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Failed to Deactivate Workflow")
		return err
	}

	return nil
}

func (s *WorkflowService) DeleteWorkflow(ctx context.Context, workflowId uuid.UUID) error {
	tenantID := auth.MustGetTenant(ctx)

	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		workflow, err := s.WorkflowRepository.GetWorkflow(ctx, workflowId, tenantID)
		if err != nil {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow not found")
			return err
		}

		activeCount, err := s.getWorkflowActiveCallJobCount(ctx, workflow.ID)
		if err != nil {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Failed to get active call job count")
			return err
		}

		if activeCount > 0 {
			s.logger.Err(err).Ctx(ctx).Str("workflow_id", workflowId.String()).Msg("Workflow has active call jobs")
			return xerrors.PreconditionFailedError(ctx, errors.New("workflow has active call jobs"))
		}

		return s.WorkflowRepository.DeleteWorkflow(ctx, workflowId, tenantID)
	})
	if err != nil {
		s.logger.Err(err).
			Ctx(ctx).
			Str("workflow_id", workflowId.String()).
			Msg("Failed to delete workflow")
		return err
	}

	return nil
}

func (s *WorkflowService) ListWorkflows(ctx context.Context, statuses []entity.WorkflowStatus, limit int32, offset int32) ([]entity.Workflow, int32, error) {
	tenantID := auth.MustGetTenant(ctx)

	workflows, err := s.WorkflowRepository.ListWorkflows(ctx, statuses, limit, offset, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to list call jobs")
		return nil, 0, err
	}

	totalCount, err := s.WorkflowRepository.GetWorkflowCount(ctx, statuses, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to get workflow count")
		return nil, 0, err
	}

	return workflows, totalCount, nil
}

func (s *WorkflowService) getWorkflowActiveCallJobCount(ctx context.Context, workflowId uuid.UUID) (int32, error) {
	tenantID := auth.MustGetTenant(ctx)

	activeCount, err := s.WorkflowRepository.GetWorkflowCallJobCountByStatuses(ctx, workflowId, activeCallJobStatuses, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to list call jobs")
		return 0, err
	}

	return activeCount, nil
}

func (s *WorkflowService) GetWorkflowWithCallJobCount(ctx context.Context, workflowId uuid.UUID) (types.WorkflowWithCallJobCount, error) {
	tenantID := auth.MustGetTenant(ctx)

	activeCount, err := s.WorkflowRepository.GetWorkflowCallJobCountByStatuses(ctx, workflowId, activeCallJobStatuses, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to list call jobs")
		return types.WorkflowWithCallJobCount{}, err
	}

	totalCount, err := s.WorkflowRepository.GetWorkflowCallJobCount(ctx, workflowId, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to list call jobs")
		return types.WorkflowWithCallJobCount{}, err
	}

	return types.WorkflowWithCallJobCount{
		ActiveCallJobCount: activeCount,
		TotalCallJobCount:  totalCount,
	}, nil
}
