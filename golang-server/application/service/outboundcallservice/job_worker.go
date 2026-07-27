package outboundcallservice

import (
	"context"
	"errors"
	"fmt"

	"exiro.ai/application/auth"
	"exiro.ai/application/service/types"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/application/xcontext"
	"exiro.ai/infra/workflow/xdbos"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/google/uuid"
)

// MaterialiseJob queues a job for materialization.
func (s *Service) MaterialiseJob(ctx context.Context, jobid uuid.UUID) error {
	jobEntity, err := s.CallJobRepository.GetCallJob(ctx, jobid, auth.MustGetTenant(ctx))
	if err != nil {
		return err
	}

	// Validate that the workflow exists and is published
	workflow, err := s.workflowService.GetWorkflow(ctx, jobEntity.Workflow_id)
	if err != nil {
		return err
	}

	// Workflow must be published before materialization
	if workflow.Status != entity.WorkflowPublished {
		return xerrors.PreconditionFailedError(ctx, errors.New("workflow must be published before materializing job"))
	}

	// Use job ID as workflow ID for idempotency
	workflowID := "materialize-job-" + jobid.String()

	_, err = dbos.RunWorkflow(
		xdbos.Ctx(xcontext.GetAppContext()),
		s.materialzeCallJobWorkflow,
		jobEntity,
		dbos.WithQueue(s.materializeJobQueue.Name),
		dbos.WithWorkflowID(workflowID),
	)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_id", jobEntity.ID.String()).
			Str("workflow_id", workflowID).
			Msg("Failed to enqueue materialize workflow")
		return fmt.Errorf("failed to enqueue materialize workflow: %w", err)
	}

	s.logger.Info().
		Ctx(ctx).
		Str("job_id", jobEntity.ID.String()).
		Str("workflow_id", workflowID).
		Msg("Successfully enqueued materialize workflow")

	return nil
}

// TriggerJob queues a job for execution.
func (s *Service) TriggerJob(ctx context.Context, jobId uuid.UUID) error {
	jobEntity, err := s.CallJobRepository.GetCallJob(ctx, jobId, auth.MustGetTenant(ctx))
	if err != nil {
		return fmt.Errorf("call job not found: %w", err)
	}
	// Validate that the job is in Ready state before starting
	if jobEntity.Status != entity.CallJobReady {
		s.logger.Warn().
			Ctx(ctx).
			Str("job_id", jobEntity.ID.String()).
			Str("current_status", jobEntity.Status.String()).
			Msg("Attempted to start job that is not in Ready state")
		return fmt.Errorf("job must be in Ready state to be started, current status: %s", jobEntity.Status.String())
	}

	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCallJobTriggered,
			ResourceType: types.AuditResourceTypeCallJob,
			ResourceID:   jobId.String(),
		})
	})
	if err != nil {
		s.logger.Err(err).
			Ctx(ctx).
			Str("job_id", jobEntity.ID.String()).
			Msg("Failed to log audit event for trigger call job")
		return err
	}

	// workflow ID for idempotency
	workflowID := makeJobWorkflowID(jobId)
	_, err = dbos.RunWorkflow(
		xdbos.Ctx(xcontext.GetAppContext()),
		s.triggerCallJobWorkflow,
		jobEntity,
		dbos.WithQueue(s.callJobQueue.Name),
		dbos.WithWorkflowID(workflowID),
	)
	if err != nil {
		s.logger.Err(err).
			Ctx(ctx).
			Str("job_id", jobEntity.ID.String()).
			Str("workflow_id", workflowID).
			Msg("Failed to trigger call job")
		return fmt.Errorf("failed to enqueue trigger call workflow: %w", err)
	}
	s.logger.Info().
		Ctx(ctx).
		Str("job_id", jobEntity.ID.String()).
		Str("workflow_id", workflowID).
		Msg("Successfully enqueued trigger call job workflow")
	return nil
}

// notifyJobItemCompletion notifies a job item workflow that the call has completed.
// This is called by the webhook handler when receiving call status updates from Twilio.
func (s *Service) notifyJobItemCompletion(ctx context.Context, jobItemId uuid.UUID, statusUpdate entity.CallJobItemCallEndDetails) error {
	// Generate the workflow ID for this job item
	workflowID := makeJobItemWorkflowID(jobItemId)
	// Send completion notification to the workflow using DBOS messaging
	err := dbos.Send(
		xdbos.Ctx(xcontext.GetAppContext()),
		workflowID,
		statusUpdate,
		callCompletionTopic,
	)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId.String()).
			Str("workflow_id", workflowID).
			Msg("Failed to send completion notification to workflow")
		return fmt.Errorf("failed to send completion notification: %w", err)
	}
	s.logger.Info().
		Ctx(ctx).
		Str("job_item_id", jobItemId.String()).
		Str("workflow_id", workflowID).
		Str("status", statusUpdate.Status.String()).
		Msg("Successfully sent completion notification to workflow")
	return nil
}
