package outboundcallservice

import (
	"context"
	"encoding/gob"
	"fmt"

	"strings"
	"time"

	"exiro.ai/application/models/pb"

	agentutils "exiro.ai/application/service/agentservice/agentutils"
	"exiro.ai/application/service/outboundcallservice/callProvider/exotel"
	"exiro.ai/application/service/outboundcallservice/callProvider/twilio"
	"exiro.ai/application/service/types/entity"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/google/uuid"
)

func init() {
	gob.Register(&pb.CredentialMetadata_Twilio{})
	gob.Register(&pb.CredentialMetadata_Exotel{})
	gob.Register(&pb.Credential_Twilio{})
	gob.Register(&pb.Credential_Exotel{})
}

const (
	callCompletionTopic           = "call-completion"
	defaultStepRetries            = 3
	defaultMaxWaitMinutes         = 5
	defaultJobItemsPageSize int32 = 10
	initialOffset           int32 = 0
)

// makeJobItemWorkflowID generates a consistent workflow ID for a job item.
func makeJobItemWorkflowID(jobItemId uuid.UUID) string {
	return "process-item-" + jobItemId.String()
}

// makeJobWorkflowID generates a consistent workflow ID for a job.
func makeJobWorkflowID(jobId uuid.UUID) string {
	return "trigger-job-" + jobId.String()
}

func (s *Service) triggerCallJobWorkflow(ctx dbos.DBOSContext, callJob entity.CallJob) (bool, error) {
	s.logger.Info().Ctx(ctx).Str("job_id", callJob.ID.String()).Msg("Starting trigger call job workflow")

	// Step 1: Update job status to Running
	if err := s.updateJobStatusToRunning(ctx, callJob); err != nil {
		return false, err
	}

	// Step 2: Get all job items
	jobItems, err := s.getJobItemsForJob(ctx, callJob, defaultJobItemsPageSize, initialOffset)
	if err != nil {
		return false, err
	}

	s.logger.Info().
		Ctx(ctx).
		Str("job_id", callJob.ID.String()).
		Int("total_items", len(jobItems)).
		Msg("Processing job items sequentially as child workflows")

	// Step 3: Process each item sequentially
	successCount, failureCount := s.processJobItemsSequentially(ctx, callJob, jobItems)

	// Step 4: Update final job status based on results
	if err := s.updateFinalJobStatus(ctx, callJob, successCount, failureCount, len(jobItems)); err != nil {
		return false, err
	}

	s.logger.Info().
		Ctx(ctx).
		Str("job_id", callJob.ID.String()).
		Int("success_count", successCount).
		Int("failure_count", failureCount).
		Msg("Trigger call job workflow completed successfully")

	return true, nil
}

// updateJobStatusToRunning updates the job status to Running.
func (s *Service) updateJobStatusToRunning(ctx dbos.DBOSContext, callJob entity.CallJob) error {
	_, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		return true, s.CallJobRepository.UpdateCallJobStatus(stepCtx, callJob.ID, entity.CallJobRunning, callJob.TenantID)
	}, dbos.WithStepName("update_job_running"), dbos.WithStepMaxRetries(defaultStepRetries))
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("job_id", callJob.ID.String()).Msg("Failed to update job status to Running")
		return fmt.Errorf("failed to update job status to Running: %w", err)
	}
	return nil
}

// getJobItemsForJob retrieves all job items for a job.
func (s *Service) getJobItemsForJob(ctx dbos.DBOSContext, callJob entity.CallJob, limit int32, offset int32) ([]entity.JobItem, error) {
	var allItems []entity.JobItem

	for {
		items, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) ([]entity.JobItem, error) {
			return s.CallJobRepository.GetJobItems(
				stepCtx,
				callJob.ID,
				limit,
				offset,
				callJob.TenantID,
			)
		},
			dbos.WithStepName("get_job_items"),
			dbos.WithStepMaxRetries(defaultStepRetries),
		)
		if err != nil {
			s.logger.
				Error().
				Ctx(ctx).
				Err(err).
				Str("job_id", callJob.ID.String()).
				Msg("Failed to get job items")

			return nil, fmt.Errorf("failed to get job items: %w", err)
		}

		// No more items left
		if len(items) == 0 {
			break
		}

		allItems = append(allItems, items...)

		// If we got fewer than pageSize, this was the last page
		if len(items) < int(limit) {
			break
		}

		offset += limit
	}

	return allItems, nil
}

// processJobItemsSequentially processes job items one by one.
func (s *Service) processJobItemsSequentially(ctx dbos.DBOSContext, callJob entity.CallJob, jobItems []entity.JobItem) (int, int) {
	successCount := 0
	failureCount := 0

	for i, item := range jobItems {
		if s.processJobItem(ctx, callJob, item, i, len(jobItems)) {
			successCount++
		} else {
			failureCount++
		}
	}

	s.logger.Info().
		Ctx(ctx).
		Str("job_id", callJob.ID.String()).
		Int("success_count", successCount).
		Int("failure_count", failureCount).
		Int("total_items", len(jobItems)).
		Msg("All job item workflows completed")

	return successCount, failureCount
}

// executeJobItemWorkflow starts and waits for a job item workflow to complete.
func (s *Service) executeJobItemWorkflow(ctx dbos.DBOSContext, callJob entity.CallJob, item entity.JobItem, workflowID string) (bool, error) {
	handle, err := dbos.RunWorkflow(
		ctx,
		s.processJobItemWorkflow,
		struct {
			JobItem entity.JobItem
			CallJob entity.CallJob
		}{
			JobItem: item,
			CallJob: callJob,
		},
		dbos.WithWorkflowID(workflowID),
	)
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_id", callJob.ID.String()).
			Str("job_item_id", item.ID.String()).
			Str("workflow_id", workflowID).
			Msg("Failed to start job item workflow")
		return false, err
	}

	s.logger.Debug().
		Ctx(ctx).
		Str("job_id", callJob.ID.String()).
		Str("job_item_id", item.ID.String()).
		Str("workflow_id", workflowID).
		Msg("Job item workflow started, waiting for completion")

	// Wait for jobItem workflow to complete before starting the next one
	result, err := handle.GetResult()
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_id", callJob.ID.String()).
			Str("job_item_id", item.ID.String()).
			Msg("Job item workflow failed")
		return false, err
	}

	return result, nil
}

// processJobItem processes a single job item and returns true if successful.
func (s *Service) processJobItem(ctx dbos.DBOSContext, callJob entity.CallJob, item entity.JobItem, index, total int) bool {
	workflowID := makeJobItemWorkflowID(item.ID)

	s.logger.Info().
		Ctx(ctx).
		Str("job_id", callJob.ID.String()).
		Str("job_item_id", item.ID.String()).
		Int("item_index", index+1).
		Int("total_items", total).
		Msg("Starting job item workflow")

	result, err := s.executeJobItemWorkflow(ctx, callJob, item, workflowID)
	if err != nil {
		return false
	}

	if result {
		s.logger.Info().
			Ctx(ctx).
			Str("job_id", callJob.ID.String()).
			Str("job_item_id", item.ID.String()).
			Int("item_index", index+1).
			Int("total_items", total).
			Msg("Job item workflow completed successfully")
		return true
	}

	s.logger.Warn().
		Ctx(ctx).
		Str("job_id", callJob.ID.String()).
		Str("job_item_id", item.ID.String()).
		Msg("Job item workflow returned false")
	return false
}

// updateFinalJobStatus updates the final status of the job.
func (s *Service) updateFinalJobStatus(ctx dbos.DBOSContext, callJob entity.CallJob, successCount, failureCount, _ int) error {
	_, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		// Determine final status
		finalStatus := s.determineFinalStatus(successCount, failureCount)
		return true, s.CallJobRepository.UpdateCallJobStatus(stepCtx, callJob.ID, finalStatus, callJob.TenantID)
	}, dbos.WithStepName("update_final_status"), dbos.WithStepMaxRetries(defaultStepRetries))

	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).Str("job_id", callJob.ID.String()).Msg("Failed to update final job status")
		return fmt.Errorf("failed to update final job status: %w", err)
	}
	return nil
}

func (s *Service) determineFinalStatus(successCount, failureCount int) entity.CallJobStatus {
	switch {
	case failureCount == 0:
		return entity.CallJobCompleted
	case successCount == 0:
		return entity.CallJobFailed
	default:
		// Partial success - some succeeded, some failed
		return entity.CallJobCompleted
	}
}

func (s *Service) materialzeCallJobWorkflow(ctx dbos.DBOSContext, callJob entity.CallJob) (bool, error) {
	// Get the job
	job_data, err := s.getCallJobForMaterialization(ctx, callJob)
	if err != nil {
		return false, err
	}

	// Skip if already materialized
	if job_data.Materialized {
		s.logger.Info().Ctx(ctx).Str("job_id", callJob.ID.String()).Msg("Job already materialized, skipping")
		return false, nil
	}

	// Get document entity
	doc_data, err := s.getDocumentForMaterialization(ctx, job_data)
	if err != nil {
		s.markJobAsFailedOnError(ctx, job_data, "mark_job_failed_get_doc")
		return false, fmt.Errorf("fetching document: %w", err)
	}

	// Parse CSV and insert data
	if err := s.parseAndInsertCSVData(ctx, callJob, job_data, doc_data); err != nil {
		s.markJobAsFailedOnError(ctx, job_data, "mark_job_failed_parse")
		return false, err
	}

	// Set materialized flag to true
	if err := s.markJobAsMaterialized(ctx, callJob); err != nil {
		return false, fmt.Errorf("marking job as materialized: %w", err)
	}

	s.logger.Info().Ctx(ctx).Str("job_id", job_data.ID.String()).Msg("Successfully completed job materialization workflow")
	return true, nil
}

// getCallJobForMaterialization retrieves the call job.
func (s *Service) getCallJobForMaterialization(ctx dbos.DBOSContext, callJob entity.CallJob) (entity.CallJob, error) {
	job_data, err := dbos.RunAsStep(ctx, func(ctx context.Context) (entity.CallJob, error) {
		return s.CallJobRepository.GetCallJob(ctx, callJob.ID, callJob.TenantID)
	}, dbos.WithStepName("get_call_job"), dbos.WithStepMaxRetries(defaultStepRetries))
	if err != nil {
		return entity.CallJob{}, err
	}
	return job_data, nil
}

// getDocumentForMaterialization retrieves the document.
func (s *Service) getDocumentForMaterialization(ctx dbos.DBOSContext, job_data entity.CallJob) (entity.OutboundCallDocument, error) {
	doc_data, err := dbos.RunAsStep(ctx, func(ctx context.Context) (entity.OutboundCallDocument, error) {
		return s.outboundCallDocumentRepository.GetOutboundCallDocumentRepository(ctx, job_data.Document_id, job_data.TenantID)
	}, dbos.WithStepName("get_document"), dbos.WithStepMaxRetries(defaultStepRetries))
	return doc_data, err
}

// markJobAsFailedOnError marks job as failed when error occurs.
func (s *Service) markJobAsFailedOnError(ctx dbos.DBOSContext, job_data entity.CallJob, stepName string) {
	_, markErr := dbos.RunAsStep(ctx, func(ctx context.Context) (bool, error) {
		return true, s.markJobAsFailed(ctx, job_data)
	}, dbos.WithStepName(stepName))
	if markErr != nil {
		s.logger.Error().Ctx(ctx).Err(markErr).Str("job_id", job_data.ID.String()).Msg("Failed to mark job as failed")
	}
}

// parseAndInsertCSVData parses CSV and inserts job items.
func (s *Service) parseAndInsertCSVData(ctx dbos.DBOSContext, callJob entity.CallJob, job_data entity.CallJob, doc_data entity.OutboundCallDocument) error {
	_, err := dbos.RunAsStep(ctx, func(ctx context.Context) (bool, error) {
		url := doc_data.DocumentUrl

		_, afterScheme, found := strings.Cut(url, "://")
		if !found {
			s.logger.Error().Ctx(ctx).Str("document_id", doc_data.ID.String()).Str("url", url).Msg("Document URL is not a valid object store URL")
			return false, fmt.Errorf("invalid object store URL format: %s", url)
		}

		bucket, objectKey, found := strings.Cut(afterScheme, "/")
		if !found {
			s.logger.Error().Ctx(ctx).Str("document_id", doc_data.ID.String()).Str("url", url).Msg("Document URL does not contain bucket and key")
			return false, fmt.Errorf("invalid object store URL format: %s", url)
		}

		file, err := s.objectStore.GetObject(ctx, bucket, objectKey)
		if err != nil {
			s.logger.Error().Ctx(ctx).Err(err).Str("document_id", doc_data.ID.String()).Str("bucket", bucket).Str("object_key", objectKey).Msg("Failed to fetch document from S3")
			return false, fmt.Errorf("fetching object %s: %w", url, err)
		}

		csv_data, err := s.parseCSV(file)
		if err != nil {
			s.logger.Error().Ctx(ctx).Err(err).Str("document_id", doc_data.ID.String()).Msg("Failed to parse CSV document")
			return false, fmt.Errorf("parsing CSV for document %s: %w", doc_data.ID.String(), err)
		}

		// Insert all rows in a single transaction
		err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
			for _, row := range csv_data {
				row.ID = uuid.Must(uuid.NewV7())
				row.Status = entity.CallJobItemStatusPending
				row.JobID = callJob.ID
				row.CreatedBy = job_data.User
				row.TenantID = job_data.TenantID
				err := s.CallJobRepository.InsertJobItem(ctx, row)
				if err != nil {
					s.logger.Error().Ctx(ctx).Err(err).Str("row_id", row.ID.String()).Str("phone_number", row.PhoneNumber).Msg("Failed to insert job item into database")
					return fmt.Errorf("inserting csv row %s: %w", row.ID, err)
				}
			}
			return nil
		})
		if err != nil {
			return false, fmt.Errorf("inserting csv rows: %w", err)
		}

		s.logger.Info().Ctx(ctx).Str("job_id", job_data.ID.String()).Int("rows_inserted", len(csv_data)).Msg("Successfully inserted job items")
		return true, nil
	}, dbos.WithStepName("parse_and_insert_csv"), dbos.WithStepMaxRetries(defaultStepRetries))
	return err
}

// markJobAsMaterialized marks the job as materialized.
func (s *Service) markJobAsMaterialized(ctx dbos.DBOSContext, callJob entity.CallJob) error {
	_, err := dbos.RunAsStep(ctx, func(ctx context.Context) (bool, error) {
		job, err := s.CallJobRepository.GetCallJob(ctx, callJob.ID, callJob.TenantID)
		if err != nil {
			return false, err
		}
		job.Materialized = true
		job.Status = entity.CallJobReady
		job.UpdatedAt = time.Now()
		err = s.CallJobRepository.UpdateCallJob(ctx, job)
		if err != nil {
			return false, err
		}
		return true, nil
	}, dbos.WithStepName("mark_materialized"), dbos.WithStepMaxRetries(defaultStepRetries))
	return err
}

// processJobItemWorkflow processes a single job item through its lifecycle.
func (s *Service) processJobItemWorkflow(ctx dbos.DBOSContext, input struct {
	JobItem entity.JobItem
	CallJob entity.CallJob
},
) (bool, error) {
	jobItem := input.JobItem
	callJob := input.CallJob

	s.logger.Info().
		Ctx(ctx).
		Str("job_item_id", jobItem.ID.String()).
		Str("job_id", callJob.ID.String()).
		Str("phone_number", jobItem.PhoneNumber).
		Msg("Starting job item workflow")

	// Step 1: Prepare job item
	if err := s.prepareJobItem(ctx, jobItem, callJob); err != nil {
		return false, err
	}

	// Step 2: Make the call
	sessionId := jobItem.ID.String()
	callId, err := s.initiateCall(ctx, jobItem, callJob, sessionId)
	if err != nil {
		s.markCallAsFailed(ctx, jobItem)
		return false, fmt.Errorf("failed to start call: %w", err)
	}

	// Step 3: Update job item with external call ID
	if err := s.updateJobItemCallId(ctx, jobItem, callId); err != nil {
		return false, fmt.Errorf("failed to update job item with external call ID: %w", err)
	}

	s.logger.Info().
		Ctx(ctx).
		Str("job_item_id", jobItem.ID.String()).
		Str("call_id", callId).
		Str("session_id", sessionId).
		Msg("Call started, waiting for completion notification")

	// Step 4: Wait for completion notification
	if err := s.waitForCallCompletion(ctx, jobItem); err != nil {
		return false, err
	}

	return true, nil
}

// prepareJobItem prepares the job item for calling.
func (s *Service) prepareJobItem(ctx dbos.DBOSContext, jobItem entity.JobItem, callJob entity.CallJob) error {
	_, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		if err := s.CallJobRepository.UpdateCallJobStatus(stepCtx, jobItem.ID, entity.CallJobRunning, callJob.TenantID); err != nil {
			return false, fmt.Errorf("failed to update job item status to queued: %w", err)
		}

		// For now we are using the job item ID as the session ID.
		// This has to be changed when we will implement multiple sessions per job item.
		sessionId := jobItem.ID.String()

		workflow, err := s.workflowService.INTERNALGetWorkflow(stepCtx, callJob.Workflow_id)
		if err != nil {
			s.logger.Error().Err(err).
				Str("workflow_id", callJob.Workflow_id.String()).
				Msg("Failed to get workflow")
			return false, fmt.Errorf("failed to get workflow: %w", err)
		}

		// Avoid calling UpdateContext for RT agents (handled by RealtimeAgentClient)
		if !agentutils.IsRTAgent(workflow.Agent_id) {
			// Update agent context for LG and WS agents
			if err := s.agentService.UpdateContext(stepCtx, workflow.Agent_id, sessionId, jobItem.AgentContext); err != nil {
				s.logger.Error().Err(err).
					Str("job_item_id", jobItem.ID.String()).
					Msg("Failed to update agent context")
				return false, fmt.Errorf("failed to update agent context: %w", err)
			}
		}

		return true, nil
	}, dbos.WithStepName("prepare_job_item"), dbos.WithStepMaxRetries(defaultStepRetries))
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItem.ID.String()).
			Msg("Failed to prepare job item")
		return err
	}
	return nil
}

// initiateCall makes the actual call via the provider.
func (s *Service) initiateCall(ctx dbos.DBOSContext, jobItem entity.JobItem, callJob entity.CallJob, sessionId string) (string, error) {
	fromNumber := s.providerConfig.GetFromPhoneNumber(ctx)

	callId, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (string, error) {
		credential, err := s.getCredentialForCall(stepCtx, callJob)
		if err != nil {
			return "", err
		}

		credMap := getCredMap(credential)
		return s.makeCallWithProvider(stepCtx, credential, fromNumber, jobItem, callJob, sessionId, credMap)
	}, dbos.WithStepName("make_call"), dbos.WithStepMaxRetries(defaultStepRetries))

	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItem.ID.String()).
			Str("from_number", fromNumber).
			Str("to_number", jobItem.PhoneNumber).
			Msg("Failed to initiate call")
		return "", err
	}

	return callId, nil
}

// getCredentialForCall retrieves the credential for making a call.
func (s *Service) getCredentialForCall(ctx context.Context, callJob entity.CallJob) (entity.Credential, error) {
	if callJob.Outbound_call_provider_id == "" {
		return entity.Credential{}, nil
	}

	credential, err := s.credentialService.GetDecryptedCredential(ctx, uuid.MustParse(callJob.Outbound_call_provider_id))
	if err != nil {
		s.logger.Error().Err(err).
			Str("call_job_id", callJob.ID.String()).
			Msg("Failed to get decrypted credential")
		return entity.Credential{}, fmt.Errorf("failed to get decrypted credential: %w", err)
	}

	// if credential.Type == entity.CredentialTypeUnspecified {
	// 	s.logger.Error().
	// 		Str("call_job_id", callJob.ID.String()).
	// 		Msg("Unknown credential type")
	// 	return entity.Credential{}, errors.New("unknown credential type")
	// }

	return credential, nil
}

// makeCallWithProvider makes a call using the appropriate provider.
func (s *Service) makeCallWithProvider(ctx context.Context, credential entity.Credential, fromNumber string, jobItem entity.JobItem, callJob entity.CallJob, sessionId string, credMap map[string]string) (string, error) {
	workflow, err := s.workflowService.INTERNALGetWorkflow(ctx, callJob.Workflow_id)
	if err != nil {
		s.logger.Error().Err(err).
			Str("workflow_id", callJob.Workflow_id.String()).
			Msg("Failed to get workflow")
		return "", fmt.Errorf("failed to get workflow: %w", err)
	}

	switch credential.Type {
	case entity.CredentialTypeTwilio:
		return twilio.NewTwilioHandler(s.logger).MakeCall(
			ctx,
			credential.CredentialMetadata.GetTwilio().GetFromPhoneNumber(),
			jobItem.PhoneNumber,
			workflow.Agent_id,
			sessionId,
			jobItem.ID.String(),
			credMap,
		)
	case entity.CredentialTypeExotel:
		return exotel.NewExotelHandler(s.logger).MakeCall(
			ctx,
			credential.CredentialMetadata.GetExotel().GetFromPhoneNumber(),
			jobItem.PhoneNumber,
			workflow.Agent_id,
			sessionId,
			jobItem.ID.String(),
			credMap,
		)
	case entity.CredentialTypeUnspecified:
		fallthrough
	default:
		// Use default call provider when credential type is unspecified or unknown
		return s.callProvider.MakeCall(
			ctx,
			fromNumber,
			jobItem.PhoneNumber,
			workflow.Agent_id,
			sessionId,
			jobItem.ID.String(),
			credMap,
		)
	}
}

// markCallAsFailed marks a call as failed.
func (s *Service) markCallAsFailed(ctx dbos.DBOSContext, jobItem entity.JobItem) {
	_, markErr := dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		return true, s.CallJobRepository.UpdateJobItemCallStatus(stepCtx, jobItem.ID, entity.CallJobItemStatusFailed)
	}, dbos.WithStepName("mark_call_failed"))
	if markErr != nil {
		s.logger.Error().Ctx(ctx).Err(markErr).Str("job_item_id", jobItem.ID.String()).Msg("Failed to mark call as failed")
	}
}

// updateJobItemCallId updates the job item with external call ID.
func (s *Service) updateJobItemCallId(ctx dbos.DBOSContext, jobItem entity.JobItem, callId string) error {
	_, err := dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
		return true, s.updateJobItemWithExternalCallId(stepCtx, jobItem.ID, callId)
	}, dbos.WithStepName("update_job_item_call_id"), dbos.WithStepMaxRetries(defaultStepRetries))
	if err != nil {
		s.logger.Error().Ctx(ctx).Err(err).
			Str("job_item_id", jobItem.ID.String()).
			Str("call_id", callId).
			Msg("Failed to update job item with external call ID")
		return err
	}
	return nil
}

// waitForCallCompletion waits for the call to complete.
func (s *Service) waitForCallCompletion(ctx dbos.DBOSContext, jobItem entity.JobItem) error {
	maxWaitMinutes := s.outboundCallConfig.MaxCallWaitTimeMinutes
	if maxWaitMinutes <= 0 {
		maxWaitMinutes = defaultMaxWaitMinutes
	}
	timeout := time.Duration(maxWaitMinutes) * time.Minute

	statusUpdate, err := dbos.Recv[entity.CallJobItemCallEndDetails](ctx, callCompletionTopic, timeout)
	if err != nil {
		s.logger.Warn().Ctx(ctx).Err(err).
			Str("job_item_id", jobItem.ID.String()).
			Dur("timeout", timeout).
			Msg("Timeout or error waiting for call completion")

		// On timeout, mark as failed
		_, markErr := dbos.RunAsStep(ctx, func(stepCtx context.Context) (bool, error) {
			return true, s.CallJobRepository.UpdateJobItemCallStatus(stepCtx, jobItem.ID, entity.CallJobItemStatusFailed)
		}, dbos.WithStepName("mark_timeout_failed"))
		if markErr != nil {
			s.logger.Error().Ctx(ctx).Err(markErr).Str("job_item_id", jobItem.ID.String()).Msg("Failed to mark timeout as failed")
		}

		return fmt.Errorf("timeout waiting for call completion: %w", err)
	}

	s.logger.Info().
		Ctx(ctx).
		Str("job_item_id", jobItem.ID.String()).
		Str("final_status", statusUpdate.Status.String()).
		Msg("Received call completion notification")

	return nil
}

func getCredMap(c entity.Credential) map[string]string {
	m := make(map[string]string)
	switch c.Type {
	case entity.CredentialTypeTwilio:
		if tw := c.Credential.GetTwilio(); tw != nil {
			m["account_sid"] = tw.GetAccountSid()
			m["auth_token"] = tw.GetAuthToken()
		}
	case entity.CredentialTypeExotel:
		if ex := c.Credential.GetExotel(); ex != nil {
			m["account_sid"] = ex.GetAccountSid()
			m["api_token"] = ex.GetApiToken()
			m["api_key"] = ex.GetApiKey()
		}
	case entity.CredentialTypeUnspecified:
		return nil
	default:
		return nil
	}

	return m
}
