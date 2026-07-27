package outboundcallservice

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
)

func (s *Service) CreateCallJob(ctx context.Context, callJob entity.CallJob) (entity.CallJob, error) {
	s.logger.Debug().Ctx(ctx).Msg("Creating call job")

	if callJob.Name == "" {
		s.logger.Error().Ctx(ctx).Msg("Missing name in call job")
		return entity.CallJob{}, xerrors.BadRequestError(ctx, errors.New("name is required"))
	}
	if callJob.Workflow_id == uuid.Nil {
		s.logger.Error().Ctx(ctx).Msg("Missing workflow_id in call job")
		return entity.CallJob{}, xerrors.BadRequestError(ctx, errors.New("workflow_id is required"))
	}
	if callJob.Document_id == "" {
		s.logger.Error().Ctx(ctx).Msg("Missing document_id in call job")
		return entity.CallJob{}, xerrors.BadRequestError(ctx, errors.New("document_id is required"))
	}
	if callJob.Document_type == "" {
		s.logger.Error().Ctx(ctx).Msg("Missing document_type in call job")
		return entity.CallJob{}, xerrors.BadRequestError(ctx, errors.New("document_type is required"))
	}

	// check if work exists or not
	_, err := s.workflowService.GetWorkflow(ctx, callJob.Workflow_id)
	if err != nil {
		return entity.CallJob{}, err
	}

	callJob.Status = entity.CallJobPending
	callJob.ID = uuid.Must(uuid.NewV7())
	callJob.User = auth.MustGetUser(ctx)
	callJob.TenantID = auth.MustGetTenant(ctx)
	callJob.CreatedAt = time.Now()
	callJob.UpdatedAt = time.Now()

	s.logger.Debug().Ctx(ctx).Str("job_id", callJob.ID.String()).Str("user", callJob.User).Msg("Call job fields validated")

	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := s.CallJobRepository.CreateCallJob(ctx, callJob); err != nil {
			s.logger.Err(err).Ctx(ctx).Str("job_id", callJob.ID.String()).Msg("Failed to create call job in database")
			return err
		}
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCallJobCreated,
			ResourceType: types.AuditResourceTypeCallJob,
			ResourceID:   callJob.ID.String(),
		})
	})
	if err != nil {
		return entity.CallJob{}, err
	}

	s.logger.Info().Ctx(ctx).Str("job_id", callJob.ID.String()).Msg("Call job created successfully")
	return callJob, nil
}

func (s *Service) GetCallJob(ctx context.Context, id uuid.UUID) (entity.CallJob, error) {
	tenantID := auth.MustGetTenant(ctx)
	callJob, err := s.CallJobRepository.GetCallJob(ctx, id, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("job_id", id.String()).Msg("Call job not found")
		return entity.CallJob{}, err
	}
	return callJob, nil
}

// TODO: filtering should happen at database itself, refact this later.
func (s *Service) ListCallJobs(
	ctx context.Context,
	statuses []entity.CallJobStatus,
	workflowId uuid.UUID,
	filterDateBegin time.Time,
	filterDateEnd time.Time,
	limit int32,
	offset int32,
) ([]entity.CallJob, int32, error) {
	tenantID := auth.MustGetTenant(ctx)

	callJobs, err := s.CallJobRepository.ListCallJobs(ctx, statuses, workflowId, filterDateBegin, filterDateEnd, limit, offset, tenantID)
	if err != nil {
		if xerrors.IsNotFoundError(err) {
			return nil, 0, xerrors.NotFoundError(ctx, errors.New("call jobs not found"))
		}
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to list call jobs")
		return nil, 0, err
	}

	totalCount, err := s.CallJobRepository.GetCallJobCount(ctx, statuses, workflowId, filterDateBegin, filterDateEnd, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("tenant_id", tenantID.String()).Msg("Failed to get call job count")
		return nil, 0, err
	}

	return callJobs, totalCount, nil
}

func (s *Service) DeleteCallJob(ctx context.Context, id uuid.UUID) error {
	tenantID := auth.MustGetTenant(ctx)
	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := s.CallJobRepository.DeleteCallJob(ctx, id, tenantID); err != nil {
			s.logger.Err(err).Ctx(ctx).Str("job_id", id.String()).Msg("Failed to delete call job")
			return err
		}
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCallJobDeleted,
			ResourceType: types.AuditResourceTypeCallJob,
			ResourceID:   id.String(),
		})
	})
	if err != nil {
		return err
	}

	s.logger.Info().Ctx(ctx).Str("job_id", id.String()).Msg("Call job deleted successfully")
	return nil
}

func (s *Service) ListJobItems(ctx context.Context, jobId uuid.UUID, limit int32, offset int32) ([]entity.JobItem, int32, error) {
	tenantID := auth.MustGetTenant(ctx)

	// First verify the job exists and belongs to the tenant
	callJob, err := s.CallJobRepository.GetCallJob(ctx, jobId, tenantID)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("job_id", jobId.String()).Msg("Failed to get call job")
		return nil, 0, err
	}
	s.logger.Debug().Ctx(ctx).Str("call_job", callJob.ID.String()).Msgf("Call job: %+v", callJob)

	// Get the total count of job items
	totalCount, err := s.CallJobRepository.GetJobItemsCount(ctx, jobId, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("job_id", jobId.String()).Msg("Failed to get job items count")
		return nil, 0, xerrors.InternalError(ctx, errors.New("failed to get job items count"))
	}

	// Get all job items for this job
	allJobItems, err := s.CallJobRepository.GetJobItems(ctx, jobId, limit, offset, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("job_id", jobId.String()).Msg("Failed to get job items")
		return nil, 0, xerrors.InternalError(ctx, errors.New("failed to get job items"))
	}

	return allJobItems, totalCount, nil
}

// updateJobItemCallEndDetails updates the call status and related fields for a job item.
func (s *Service) updateJobItemCallEndDetails(ctx context.Context, jobItemId uuid.UUID, statusUpdate entity.CallJobItemCallEndDetails) error {
	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		job, err := s.CallJobRepository.INTERNALGetJobItem(ctx, jobItemId)
		if err != nil {
			return err
		}
		job.Status = statusUpdate.Status
		job.CallStartedAt = statusUpdate.CallStartedAt
		job.CallEndedAt = statusUpdate.CallEndedAt
		job.CallDuration = statusUpdate.CallDuration
		job.CallErrorMessage = statusUpdate.CallErrorMessage
		job.UpdatedAt = time.Now()
		return s.CallJobRepository.UpdateJobItem(ctx, job)
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("job_item_id", jobItemId.String()).Msg("failed to update job item call status")
		return err
	}
	return nil
}

func (s *Service) ProcessCallStatusWebhook(ctx context.Context, callSid string, jobItemId uuid.UUID, sessionId string, callStatus string, callDuration string, timestamp string, errorMessage string) error {
	callDuration = s.ensureCallDuration(ctx, callSid, jobItemId, callDuration)

	statusUpdate := s.buildCallStatusUpdate(callStatus, callDuration, timestamp, errorMessage)
	s.logger.Info().
		Ctx(ctx).
		Str("job_item_id", jobItemId.String()).
		Str("call_status", callStatus).
		Str("mapped_status", statusUpdate.Status.String()).
		Msg("Processing call status webhook")

	if err := s.updateJobItemCallEndDetails(ctx, jobItemId, statusUpdate); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId.String()).
			Msg("Failed to update job item call status")
		return err
	}

	if !s.isTerminalStatus(callStatus) {
		s.logger.Debug().
			Ctx(ctx).
			Str("job_item_id", jobItemId.String()).
			Str("call_status", callStatus).
			Msg("Non-terminal status processed")
		return nil
	}

	s.handleTerminalStatus(ctx, jobItemId, sessionId, callStatus, statusUpdate)

	s.logger.Info().
		Ctx(ctx).
		Str("job_item_id", jobItemId.String()).
		Str("call_status", callStatus).
		Msg("Successfully processed terminal call status webhook")

	return nil
}

func (s *Service) ensureCallDuration(ctx context.Context, callSid string, jobItemId uuid.UUID, callDuration string) string {
	if callDuration == "" && callSid != "" {
		callDetails, err := s.GetCallDetails(ctx, callSid, jobItemId)
		if err != nil {
			s.logger.Warn().Err(err).Str("call_sid", callSid).Msg("Failed to fetch call details for duration, will proceed without it")
		} else if callDetails.Duration > 0 {
			callDuration = strconv.Itoa(callDetails.Duration)
			s.logger.Info().
				Str("call_sid", callSid).
				Str("duration", callDuration).
				Msg("Fetched duration from call provider")
		}
	}
	return callDuration
}

func (s *Service) isTerminalStatus(callStatus string) bool {
	return callStatus == entity.CallJobItemStatusCompleted.String() ||
		callStatus == entity.CallJobItemStatusFailed.String() ||
		callStatus == entity.CallJobItemStatusBusy.String() ||
		callStatus == entity.CallJobItemStatusNoAnswer.String() ||
		callStatus == entity.CallJobItemStatusCanceled.String()
}

func (s *Service) handleTerminalStatus(ctx context.Context, jobItemId uuid.UUID, sessionId string, callStatus string, statusUpdate entity.CallJobItemCallEndDetails) {
	if err := s.notifyJobItemCompletion(ctx, jobItemId, statusUpdate); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId.String()).
			Str("call_status", callStatus).
			Msg("Failed to notify workflow of call completion")
	}

	if callStatus == entity.CallJobItemStatusCompleted.String() {
		s.handleCompletedCall(ctx, jobItemId, sessionId, statusUpdate)
	}
}

func (s *Service) handleCompletedCall(ctx context.Context, jobItemId uuid.UUID, sessionId string, statusUpdate entity.CallJobItemCallEndDetails) {
	var durationSeconds float32 = 0
	if statusUpdate.CallDuration != nil {
		durationSeconds = float32(*statusUpdate.CallDuration)
	}

	if err := s.sendCallEndEvent(ctx, jobItemId, sessionId, durationSeconds); err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId.String()).
			Msg("Failed to send call end event to transcription service")
	}
}

// buildCallStatusUpdate constructs a CallStatusUpdate from webhook data.
func (s *Service) buildCallStatusUpdate(callStatus string, callDuration string, timestamp string, errorMessage string) entity.CallJobItemCallEndDetails {
	var statusUpdate entity.CallJobItemCallEndDetails

	// Parse timestamp - try multiple formats for different providers
	parsedTime := s.parseTimestamp(timestamp)
	if parsedTime != nil {
		s.assignTimestampToStatus(&statusUpdate, callStatus, *parsedTime, callDuration)
	}

	// Parse call duration
	if callDuration != "" {
		if duration, err := strconv.ParseInt(callDuration, 10, 32); err == nil {
			durationInt32 := int32(duration)
			statusUpdate.CallDuration = &durationInt32
		}
	}

	// Add error message if present
	if errorMessage != "" {
		statusUpdate.CallErrorMessage = &errorMessage
	}

	// Convert provider status to our generic CallStatus enum
	statusUpdate.Status = entity.ParseCallStatusFromProvider(callStatus)

	return statusUpdate
}

// parseTimestamp attempts to parse timestamp using multiple formats.
func (s *Service) parseTimestamp(timestamp string) *time.Time {
	if timestamp == "" {
		return nil
	}

	formats := []string{
		"Mon, 02 Jan 2006 15:04:05 -0700", // Twilio format
		"2006-01-02 15:04:05",             // Exotel format
	}

	for _, format := range formats {
		parsedTime, err := time.Parse(format, timestamp)
		if err == nil {
			return &parsedTime
		}
	}

	s.logger.Warn().
		Str("timestamp", timestamp).
		Msg("Failed to parse timestamp - tried all known formats")

	return nil
}

// assignTimestampToStatus assigns parsed timestamp to appropriate status field.
func (s *Service) assignTimestampToStatus(statusUpdate *entity.CallJobItemCallEndDetails, callStatus string, parsedTime time.Time, callDuration string) {
	switch callStatus {
	case entity.DeploymentStatusInProgress.String(), entity.CallJobItemStatusAnswered.String():
		statusUpdate.CallStartedAt = &parsedTime
	case entity.CallJobItemStatusCompleted.String(), entity.CallJobItemStatusFailed.String(), entity.CallJobItemStatusBusy.String(), entity.CallJobItemStatusNoAnswer.String(), entity.CallJobItemStatusCanceled.String():
		statusUpdate.CallEndedAt = &parsedTime

		// For completed calls, estimate start time from duration if available
		if callDuration != "" {
			if duration, err := strconv.ParseInt(callDuration, 10, 32); err == nil && duration > 0 {
				estimatedStartTime := parsedTime.Add(-time.Duration(duration) * time.Second)
				statusUpdate.CallStartedAt = &estimatedStartTime
			}
		}
	}
}

// sendCallEndEvent sends a call end event to the transcription service for a completed call.
func (s *Service) sendCallEndEvent(ctx context.Context, jobItemId uuid.UUID, sessionId string, durationSeconds float32) error {
	// Get tenant ID for the job item
	tenantId, err := s.CallJobRepository.GetJobItemTenantById(ctx, jobItemId)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItemId.String()).
			Str("session_id", sessionId).
			Msg("Failed to get tenant ID for call end event")
		return fmt.Errorf("failed to get tenant ID: %w", err)
	}

	s.sendCallEndToQueue(ctx, sessionId, tenantId.String(), 1, durationSeconds)

	s.logger.Info().
		Ctx(ctx).
		Str("job_item_id", jobItemId.String()).
		Str("session_id", sessionId).
		Str("tenant_id", tenantId.String()).
		Float32("duration_seconds", durationSeconds).
		Msg("Successfully sent call end event to transcription service")

	return nil
}

// updateJobItemWithExternalCallId updates the job item with external call ID when a call is initiated.
func (s *Service) updateJobItemWithExternalCallId(ctx context.Context, jobItemId uuid.UUID, externalCallId string) error {
	return s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		jobItem, err := s.CallJobRepository.INTERNALGetJobItem(ctx, jobItemId)
		if err != nil {
			return err
		}
		jobItem.ExternalCallID = &externalCallId
		jobItem.Status = entity.CallJobItemStatusQueued
		jobItem.UpdatedAt = time.Now()
		return s.CallJobRepository.UpdateJobItem(ctx, jobItem)
	})
}

// markJobAsFailed marks a job as failed.
func (s *Service) markJobAsFailed(ctx context.Context, job entity.CallJob) error {
	err := s.CallJobRepository.UpdateCallJobStatus(ctx, job.ID, entity.CallJobFailed, job.TenantID)
	if err != nil {
		return err
	}
	return nil
}

// CopyCallJob copies an existing call job and a specific list of job items into a new job.
func (s *Service) CopyCallJob(ctx context.Context, sourceJobID string, name string, providerId uuid.UUID, workflowId uuid.UUID, preferedLanguage string, jobItemIDs []string) (string, int32, error) {
	tenantID := auth.MustGetTenant(ctx)
	userID := auth.MustGetUser(ctx)

	if len(jobItemIDs) == 0 {
		return "", 0, xerrors.BadRequestError(ctx, errors.New("job_item_ids must not be empty"))
	}

	// Parse and validate source job
	sourceJobUUID, sourceJob, err := s.validateAndFetchSourceJob(ctx, sourceJobID, tenantID)
	if err != nil {
		return "", 0, err
	}

	// Parse and validate job item IDs
	itemUUIDs, err := s.parseJobItemIDs(ctx, jobItemIDs)
	if err != nil {
		return "", 0, err
	}

	// Validate job items belong to source job
	matchedItems, err := s.validateJobItems(ctx, sourceJobUUID, itemUUIDs, sourceJob.TenantID, jobItemIDs)
	if err != nil {
		return "", 0, err
	}

	// check if work exists or not
	workflow, err := s.workflowService.GetWorkflow(ctx, workflowId)
	if err != nil {
		return "", 0, err
	}

	// Create new job and copy items
	return s.createJobAndCopyItems(ctx, sourceJob, matchedItems, name, providerId, workflow.ID, preferedLanguage, userID, tenantID)
}

// validateAndFetchSourceJob validates and fetches the source job.
func (s *Service) validateAndFetchSourceJob(ctx context.Context, sourceJobID string, tenantID uuid.UUID) (uuid.UUID, entity.CallJob, error) {
	sourceJobUUID, err := uuid.Parse(sourceJobID)
	if err != nil {
		return uuid.Nil, entity.CallJob{}, xerrors.BadRequestError(ctx, errors.New("invalid source_job_id"))
	}

	sourceJob, err := s.CallJobRepository.GetCallJob(ctx, sourceJobUUID, tenantID)
	if err != nil {
		return uuid.Nil, entity.CallJob{}, xerrors.NotFoundError(ctx, errors.New("source job not found or unauthorized"))
	}

	return sourceJobUUID, sourceJob, nil
}

// parseJobItemIDs parses job item ID strings to UUIDs.
func (s *Service) parseJobItemIDs(ctx context.Context, jobItemIDs []string) ([]uuid.UUID, error) {
	itemUUIDs := make([]uuid.UUID, 0, len(jobItemIDs))
	for _, idStr := range jobItemIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, xerrors.BadRequestError(ctx, fmt.Errorf("invalid job item id: %s", idStr))
		}
		itemUUIDs = append(itemUUIDs, id)
	}
	return itemUUIDs, nil
}

// validateJobItems validates that job items belong to source job.
func (s *Service) validateJobItems(ctx context.Context, sourceJobUUID uuid.UUID, itemUUIDs []uuid.UUID, tenantID uuid.UUID, jobItemIDs []string) ([]entity.JobItem, error) {
	matchedItems, err := s.CallJobRepository.GetJobItemsByIDs(ctx, sourceJobUUID, itemUUIDs, tenantID)
	if err != nil {
		return nil, xerrors.BadRequestError(ctx, errors.New("failed to fetch job items"))
	}

	if len(matchedItems) != len(jobItemIDs) {
		s.logger.Error().Ctx(ctx).Err(err).Msg("some job_item_ids do not belong to source_job_id")
		return nil, xerrors.BadRequestError(ctx, errors.New("some job_item_ids do not belong to source_job_id"))
	}

	return matchedItems, nil
}

// createJobAndCopyItems creates a new job and copies items in a transaction.
func (s *Service) createJobAndCopyItems(ctx context.Context, sourceJob entity.CallJob, matchedItems []entity.JobItem, name string, providerId uuid.UUID, workflowId uuid.UUID, preferedLanguage string, userID string, tenantID uuid.UUID) (string, int32, error) {
	newJobID := uuid.Must(uuid.NewV7())
	var copiedCount int32

	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		// Create new job
		if err := s.createNewJob(ctx, newJobID, sourceJob, name, providerId, workflowId, preferedLanguage, userID, tenantID); err != nil {
			return err
		}

		// Copy job items
		copiedCount = s.copyJobItems(ctx, matchedItems, newJobID, userID, tenantID)
		if copiedCount < 0 {
			return xerrors.InternalError(ctx, errors.New("failed to copy job item"))
		}

		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCallJobCopied,
			ResourceType: types.AuditResourceTypeCallJob,
			ResourceID:   newJobID.String(),
		})
	})

	if err != nil {
		return "", copiedCount, err
	}

	return newJobID.String(), copiedCount, nil
}

// createNewJob creates a new job entity.
func (s *Service) createNewJob(ctx context.Context, newJobID uuid.UUID, sourceJob entity.CallJob, name string, providerId uuid.UUID, workflowId uuid.UUID, preferedLanguage string, userID string, tenantID uuid.UUID) error {
	providerIdStr := providerId.String()
	if providerId == uuid.Nil {
		providerIdStr = ""
	}

	newJob := entity.CallJob{
		ID:                        newJobID,
		Name:                      name,
		Workflow_id:               workflowId,
		Preffered_language:        preferedLanguage,
		Document_id:               sourceJob.Document_id,
		Document_type:             sourceJob.Document_type,
		Outbound_call_provider_id: providerIdStr,
		Status:                    entity.CallJobReady,
		Materialized:              true,
		User:                      userID,
		TenantID:                  tenantID,
		CreatedAt:                 time.Now(),
		UpdatedAt:                 time.Now(),
	}
	if err := s.CallJobRepository.CreateCallJob(ctx, newJob); err != nil {
		return xerrors.InternalError(ctx, errors.New("failed to create new job"))
	}
	return nil
}

// copyJobItems copies job items from source to new job.
func (s *Service) copyJobItems(ctx context.Context, matchedItems []entity.JobItem, newJobID uuid.UUID, userID string, tenantID uuid.UUID) int32 {
	var copiedCount int32
	for _, item := range matchedItems {
		newItem := entity.JobItem{
			ID:               uuid.Must(uuid.NewV7()),
			JobID:            newJobID,
			PhoneNumber:      item.PhoneNumber,
			AgentContext:     item.AgentContext,
			JobData:          item.JobData,
			CreatedBy:        userID,
			TenantID:         tenantID,
			ExternalCallID:   nil,
			Status:           entity.CallJobItemStatusInitiated,
			CallStartedAt:    nil,
			CallEndedAt:      nil,
			CallDuration:     nil,
			CallErrorMessage: nil,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		}
		if err := s.CallJobRepository.InsertJobItem(ctx, newItem); err != nil {
			return -1
		}
		copiedCount++
	}
	return copiedCount
}

func (s *Service) UpdateCallJobDetails(ctx context.Context, jobId string, name string, workflowId uuid.UUID) error {
	tenantID := auth.MustGetTenant(ctx)

	parsedJobId, err := uuid.Parse(jobId)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("Invalid Call Job ID")
		return xerrors.BadRequestError(ctx, errors.New("invalid job ID"))
	}

	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		callJob, err := s.CallJobRepository.GetCallJob(ctx, parsedJobId, tenantID)
		if err != nil {
			return err
		}

		if callJob.Status != entity.CallJobPending && callJob.Status != entity.CallJobReady {
			return xerrors.BadRequestError(ctx, errors.New("calljob not in Ready state"))
		}

		// check if work exists or not
		workflow, err := s.workflowService.GetWorkflow(ctx, workflowId)
		if err != nil {
			return err
		}

		callJob.Name = name
		callJob.Workflow_id = workflow.ID

		err = s.CallJobRepository.UpdateCallJob(ctx, callJob)
		if err != nil {
			return err
		}
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCallJobUpdated,
			ResourceType: types.AuditResourceTypeCallJob,
			ResourceID:   parsedJobId.String(),
		})
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("Failed to Update Call Job")
		return err
	}

	return nil
}

// AddJobItems just returns an empty response for now.
func (s *Service) AddJobItems(ctx context.Context, jobId string, items []entity.NewJobItem) error {
	tenantID := auth.MustGetTenant(ctx)
	userID := auth.MustGetUser(ctx)

	parsedCallJobId, err := uuid.Parse(jobId)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("Invalid Job ID")
		return xerrors.BadRequestError(ctx, errors.New("invalid job ID"))
	}

	callJob, err := s.CallJobRepository.GetCallJob(ctx, parsedCallJobId, tenantID)
	if err != nil {
		return err
	}

	if callJob.Status != entity.CallJobReady {
		return xerrors.BadRequestError(ctx, errors.New("calljob not in Ready state"))
	}

	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		for _, row := range items {
			var jobItem entity.JobItem
			jobItem.ID = uuid.Must(uuid.NewV7())
			jobItem.Status = entity.CallJobItemStatusInitiated
			jobItem.JobID = parsedCallJobId
			jobItem.CreatedBy = userID
			jobItem.TenantID = tenantID
			jobItem.PhoneNumber = row.PhoneNumber
			jobItem.AgentContext = row.AgentContext
			err := s.CallJobRepository.InsertJobItem(ctx, jobItem)
			if err != nil {
				s.logger.Error().Ctx(ctx).Err(err).Str("row_id", jobItem.ID.String()).Str("phone_number", jobItem.PhoneNumber).Msg("Failed to insert job item into database")
				return xerrors.InternalError(ctx, fmt.Errorf("inserting csv row %s: %w", jobItem.ID, err))
			}
		}
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCallJobItemsAdded,
			ResourceType: types.AuditResourceTypeCallJob,
			ResourceID:   parsedCallJobId.String(),
		})
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("Failed to Add Job Items")
		return xerrors.BadRequestError(ctx, err)
	}

	return nil
}

// RemoveJobItems just returns an empty response for now.
func (s *Service) RemoveJobItems(ctx context.Context, jobId string, jobItemIds []string) error {
	tenantID := auth.MustGetTenant(ctx)

	parsedCallJobId, err := uuid.Parse(jobId)
	if err != nil {
		return xerrors.BadRequestError(ctx, errors.New("invalid call job ID"))
	}

	callJob, err := s.CallJobRepository.GetCallJob(ctx, parsedCallJobId, tenantID)
	if err != nil {
		return err
	}

	if callJob.Status != entity.CallJobReady {
		return xerrors.BadRequestError(ctx, errors.New("calljob not in Ready state"))
	}

	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		for _, item := range jobItemIds {
			parsedItemId, err := uuid.Parse(item)
			if err != nil {
				return xerrors.BadRequestError(ctx, errors.New("invalid job ID"))
			}

			err = s.CallJobRepository.DeleteJobItem(ctx, parsedItemId, parsedCallJobId, tenantID)
			if err != nil {
				return err
			}
		}
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionCallJobItemsRemoved,
			ResourceType: types.AuditResourceTypeCallJob,
			ResourceID:   parsedCallJobId.String(),
		})
	})
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Msg("Failed to Remove Job Items")
		return xerrors.BadRequestError(ctx, err)
	}

	return nil
}
