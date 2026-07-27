package call_job

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) mapToCallJobEntity(_ context.Context, callJob gen.CallJob) (entity.CallJob, error) {
	var preferredLanguage string
	if callJob.PrefferedLanguage.Valid {
		preferredLanguage = callJob.PrefferedLanguage.String
	}

	var maxRetries int32
	if callJob.MaxRetries.Valid {
		maxRetries = callJob.MaxRetries.Int32
	}

	var retryDelay int32
	if callJob.RetryDelay.Valid {
		retryDelay = callJob.RetryDelay.Int32
	}
	status, err := entity.ParseCallJobStatus(callJob.Status)
	if err != nil {
		return entity.CallJob{}, err
	}

	workflowId, err := uuid.Parse(callJob.WorkflowID)
	if err != nil {
		r.logger.Error().Err(err).Msg("Failed to map to callJobEntity")
		return entity.CallJob{}, err
	}

	return entity.CallJob{
		ID:                        callJob.ID,
		User:                      callJob.CreatedBy,
		Name:                      callJob.Name,
		Workflow_id:               workflowId,
		Document_id:               callJob.DocumentID,
		Document_type:             callJob.DocumentType,
		Preffered_language:        preferredLanguage,
		Max_retries:               maxRetries,
		Retry_delay:               retryDelay,
		Outbound_call_provider_id: callJob.OutboundCallProviderID.String,
		Status:                    status,
		Materialized:              callJob.IsMaterialised,
		TenantID:                  callJob.TenantID,
		CreatedAt:                 callJob.CreatedAt.Time,
		UpdatedAt:                 callJob.UpdatedAt.Time,
	}, nil
}

func (r *Repository) mapToPostgresCallJob(_ context.Context, callJob entity.CallJob) gen.CallJob {
	var preferredLanguage pgtype.Text
	if callJob.Preffered_language != "" {
		preferredLanguage = pgtype.Text{String: callJob.Preffered_language, Valid: true}
	} else {
		preferredLanguage = pgtype.Text{Valid: false}
	}

	var maxRetries pgtype.Int4
	if callJob.Max_retries != 0 {
		maxRetries = pgtype.Int4{Int32: callJob.Max_retries, Valid: true}
	} else {
		maxRetries = pgtype.Int4{Valid: false}
	}

	var retryDelay pgtype.Int4
	if callJob.Retry_delay != 0 {
		retryDelay = pgtype.Int4{Int32: callJob.Retry_delay, Valid: true}
	} else {
		retryDelay = pgtype.Int4{Valid: false}
	}

	var outboundCallProviderId pgtype.Text
	if callJob.Outbound_call_provider_id != "" {
		outboundCallProviderId = pgtype.Text{String: callJob.Outbound_call_provider_id, Valid: true}
	} else {
		outboundCallProviderId = pgtype.Text{Valid: false}
	}

	return gen.CallJob{
		ID:                     callJob.ID,
		Name:                   callJob.Name,
		WorkflowID:             callJob.Workflow_id.String(),
		DocumentID:             callJob.Document_id,
		DocumentType:           callJob.Document_type,
		PrefferedLanguage:      preferredLanguage,
		MaxRetries:             maxRetries,
		RetryDelay:             retryDelay,
		OutboundCallProviderID: outboundCallProviderId,
		CreatedBy:              callJob.User,
		TenantID:               callJob.TenantID,
		Status:                 callJob.Status.String(),
		IsMaterialised:         callJob.Materialized,
		CreatedAt:              pgtype.Timestamp{Time: callJob.CreatedAt, Valid: !callJob.CreatedAt.IsZero()},
		UpdatedAt:              pgtype.Timestamp{Time: callJob.UpdatedAt, Valid: !callJob.UpdatedAt.IsZero()},
	}
}

func (r *Repository) mapToPostgresJobItem(_ context.Context, row entity.JobItem) gen.InsertJobItemParams {
	var job_data pgtype.Text
	if row.JobData != "" {
		job_data = pgtype.Text{String: row.JobData, Valid: true}
	} else {
		job_data = pgtype.Text{Valid: false}
	}
	return gen.InsertJobItemParams{
		ID:           row.ID,
		PhoneNo:      row.PhoneNumber,
		AgentContext: row.AgentContext,
		CallStatus:   pgtype.Text{String: row.Status.String(), Valid: true},
		JobID:        row.JobID,
		JobData:      job_data,
		CreatedBy:    row.CreatedBy,
		TenantID:     row.TenantID,
	}
}

func (r *Repository) mapToJobItemEntity(_ context.Context, jobItem gen.JobItem) (entity.JobItem, error) {
	var externalCallId *string
	if jobItem.ExternalCallID.Valid {
		externalCallId = &jobItem.ExternalCallID.String
	}

	// Map from database CallStatus to entity CallStatus
	var status entity.CallJobItemStatus
	status = entity.CallJobItemStatusUnknown // default
	if jobItem.CallStatus.Valid {
		status_parsed, err := entity.ParseCallStatus(jobItem.CallStatus.String)
		if err != nil {
			r.logger.Error().Err(err).Msg("Failed to parse call status")
			return entity.JobItem{}, err
		}
		status = status_parsed
	}

	var callStartedAt *time.Time
	if jobItem.CallStartedAt.Valid {
		callStartedAt = &jobItem.CallStartedAt.Time
	}

	var callEndedAt *time.Time
	if jobItem.CallEndedAt.Valid {
		callEndedAt = &jobItem.CallEndedAt.Time
	}

	var callDuration *int32
	if jobItem.CallDuration.Valid {
		callDuration = &jobItem.CallDuration.Int32
	}

	var callErrorMessage *string
	if jobItem.CallErrorMessage.Valid {
		callErrorMessage = &jobItem.CallErrorMessage.String
	}

	return entity.JobItem{
		ID:               jobItem.ID,
		PhoneNumber:      jobItem.PhoneNo,
		AgentContext:     jobItem.AgentContext,
		JobID:            jobItem.JobID,
		JobData:          jobItem.JobData.String,
		ExternalCallID:   externalCallId,
		Status:           status,
		CreatedBy:        jobItem.CreatedBy,
		TenantID:         jobItem.TenantID,
		CallStartedAt:    callStartedAt,
		CallEndedAt:      callEndedAt,
		CallDuration:     callDuration,
		CallErrorMessage: callErrorMessage,
		UpdatedAt:        jobItem.UpdatedAt.Time,
	}, nil
}

// Postgres error codes.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

func (r *Repository) mapPgError(
	ctx context.Context,
	err error,
	resourceID string,
	resourceType string,
) error {
	if err == nil {
		return nil
	}

	// 1. No rows found (SELECT / DELETE)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return xerrors.NotFoundError(
			ctx,
			fmt.Errorf("%s with id %s not found", resourceType, resourceID),
		)
	}

	// 2. PostgreSQL-specific errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			return xerrors.ConflictError(
				ctx,
				fmt.Errorf("%s already exists", resourceType),
			)

		case pgForeignKeyViolation:
			return xerrors.BadRequestError(
				ctx,
				fmt.Errorf("invalid %s reference", resourceType),
			)
		}
	}

	// 3. Fallback: unexpected database error
	r.logger.Error().
		Ctx(ctx).
		Err(err).
		Str("resource_id", resourceID).
		Str("resource_type", resourceType).
		Msg("Unhandled postgres error")

	return xerrors.InternalError(
		ctx,
		fmt.Errorf("failed to process %s", resourceType),
	)
}
