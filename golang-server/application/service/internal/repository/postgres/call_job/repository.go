package call_job

import (
	"context"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
	sb "github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
)

type Repository struct {
	logger *zerolog.Logger
	db     *gen.Queries
	pool   *pgxpool.Pool
}

func NewRepository(ctx context.Context) *Repository {
	pool := postgres.Ctx(ctx)

	return &Repository{
		logger: zerolog.Ctx(ctx),
		db:     gen.New(postgres.Ctx(ctx)),
		pool:   pool,
	}
}

// getQueries returns the appropriate Queries instance - either with transaction or the default one.
func (r *Repository) getQueries(ctx context.Context) *gen.Queries {
	if tx, ok := transaction.TxFromContext(ctx); ok {
		return r.db.WithTx(tx)
	}
	return r.db
}

func (r *Repository) CreateCallJob(ctx context.Context, callJob entity.CallJob) error {
	pgCallJob := r.mapToPostgresCallJob(ctx, callJob)

	_, err := r.db.InsertCallJob(ctx, gen.InsertCallJobParams{
		ID:                     pgCallJob.ID,
		Name:                   pgCallJob.Name,
		WorkflowID:             pgCallJob.WorkflowID,
		DocumentID:             pgCallJob.DocumentID,
		DocumentType:           pgCallJob.DocumentType,
		PrefferedLanguage:      pgCallJob.PrefferedLanguage,
		MaxRetries:             pgCallJob.MaxRetries,
		RetryDelay:             pgCallJob.RetryDelay,
		OutboundCallProviderID: pgCallJob.OutboundCallProviderID,
		IsMaterialised:         pgCallJob.IsMaterialised,
		CreatedBy:              pgCallJob.CreatedBy,
		Status:                 pgCallJob.Status,
		TenantID:               pgCallJob.TenantID,
	})

	if err != nil {
		return r.mapPgError(ctx, err, pgCallJob.ID.String(), "call job")
	}

	return nil
}

func (r *Repository) GetCallJob(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (entity.CallJob, error) {
	row, err := r.db.GetCallJob(ctx, gen.GetCallJobParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return entity.CallJob{}, r.mapPgError(ctx, err, id.String(), "call job")
	}
	callJob, err := r.mapToCallJobEntity(ctx, row)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map call job entity")
		return entity.CallJob{}, xerrors.InternalError(ctx, err)
	}
	return callJob, nil
}

func (r *Repository) ListCallJobs(
	ctx context.Context,
	statuses []entity.CallJobStatus,
	workflowId uuid.UUID,
	filterDateBegin time.Time,
	filterDateEnd time.Time,
	limit int32,
	offset int32,
	tenantID uuid.UUID,
) ([]entity.CallJob, error) {
	pool := r.pool
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()
	sbBuilder.Select(
		"id",
		"name",
		"workflow_id",
		"status",
		"document_id",
		"document_type",
		"COALESCE(preffered_language, '') AS preffered_language",
		"COALESCE(max_retries, 0) AS max_retries",                              // COALESCE to compensate for NULLs, treating them as 0
		"COALESCE(retry_delay, 0) AS retry_delay",                              // COALESCE to compensate for NULLs, treating them as 0
		"COALESCE(outbound_call_provider_id, '') AS outbound_call_provider_id", // COALESCE to compensate for NULLs, treating them as empty string
		"created_by",
		"tenant_id",
		"is_materialised",
		"created_at",
		"updated_at",
	).From("call_jobs")
	applyCallJobStatusFilters(sbBuilder, tenantID, statuses)
	applyCallJobFilters(sbBuilder, workflowId, filterDateBegin, filterDateEnd)
	sbBuilder.OrderBy("id DESC")

	applyPagination(sbBuilder, limit, offset)

	query, args := sbBuilder.Build()

	r.logger.Debug().Ctx(ctx).Str("query", query).Interface("args", args).Int32("limit", limit).Int32("offset", offset).Msg("Executing ListCallJobs query")
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Str("query", query).Msg("Failed to execute query")
		return nil, r.mapPgError(ctx, err, "calljob", "")
	}
	defer rows.Close()

	callJobs := make([]entity.CallJob, 0)
	rowCount := 0
	for rows.Next() {
		rowCount++
		cj, err := scanCallJobRow(rows)
		if err != nil {
			r.logger.Error().Ctx(ctx).Err(err).Int("row_number", rowCount).Msg("Failed to scan calljob row")
			return nil, r.mapPgError(ctx, err, "calljob", "")
		}
		callJobs = append(callJobs, cj)
	}
	if err := rows.Err(); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Error iterating rows")
		return nil, r.mapPgError(ctx, err, "calljob", "")
	}

	r.logger.Debug().Ctx(ctx).Int("count", len(callJobs)).Msg("Successfully listed call jobs")
	return callJobs, nil
}

func scanCallJobRow(rows pgx.Rows) (entity.CallJob, error) {
	var cj entity.CallJob
	var statusStr string

	err := rows.Scan(
		&cj.ID,
		&cj.Name,
		&cj.Workflow_id,
		&statusStr,
		&cj.Document_id,
		&cj.Document_type,
		&cj.Preffered_language,
		&cj.Max_retries,
		&cj.Retry_delay,
		&cj.Outbound_call_provider_id,
		&cj.User,
		&cj.TenantID,
		&cj.Materialized,
		&cj.CreatedAt,
		&cj.UpdatedAt,
	)
	if err != nil {
		return entity.CallJob{}, err
	}

	cj.Status, err = entity.ParseCallJobStatus(statusStr)
	if err != nil {
		return entity.CallJob{}, err
	}

	return cj, nil
}

func applyCallJobStatusFilters(
	builder *sb.SelectBuilder,
	tenantID uuid.UUID,
	statuses []entity.CallJobStatus,
) {
	builder.Where(builder.Equal("tenant_id", tenantID))

	if len(statuses) > 0 {
		statusStrings := make([]any, len(statuses))
		for i, status := range statuses {
			statusStrings[i] = status.String()
		}
		builder.Where(builder.In("status", statusStrings...))
	}
}

func applyCallJobFilters(
	builder *sb.SelectBuilder,
	workflowId uuid.UUID,
	filterDateBegin time.Time,
	filterDateEnd time.Time,
) {
	if workflowId != uuid.Nil {
		builder.Where(builder.Equal("workflow_id", workflowId))
	}

	if !filterDateBegin.IsZero() {
		builder.Where(builder.GreaterEqualThan("created_at", filterDateBegin))
	}

	if !filterDateEnd.IsZero() {
		builder.Where(builder.LessEqualThan("created_at", filterDateEnd))
	}
}

func applyPagination(builder *sb.SelectBuilder, limit int32, offset int32) {
	const maxLimit = 50
	effectiveLimit := limit

	if effectiveLimit <= 0 || effectiveLimit > maxLimit {
		effectiveLimit = maxLimit
	}

	builder.Limit(int(effectiveLimit))

	if offset > 0 {
		builder.Offset(int(offset))
	}
}

func (r *Repository) GetCallJobCount(ctx context.Context, statuses []entity.CallJobStatus, workflowId uuid.UUID, filterDateBegin time.Time, filterDateEnd time.Time, tenantID uuid.UUID,
) (int32, error) {
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()
	sbBuilder.
		Select("COUNT(*)").
		From("call_jobs")

	// apply filters
	applyCallJobStatusFilters(sbBuilder, tenantID, statuses)
	applyCallJobFilters(sbBuilder, workflowId, filterDateBegin, filterDateEnd)

	query, args := sbBuilder.Build()

	var count int32
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		r.logger.Error().Ctx(ctx).
			Err(err).
			Str("query", query).
			Msg("Failed to get call job count")
		return 0, r.mapPgError(ctx, err, "calljob", "")
	}

	return count, nil
}

func (r *Repository) GetJobItemsCount(ctx context.Context, jobID uuid.UUID, tenantID uuid.UUID,
) (int32, error) {
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()
	sbBuilder.
		Select("COUNT(*)").
		From("job_item").
		Where(sbBuilder.Equal("job_id", jobID)).
		Where(sbBuilder.Equal("tenant_id", tenantID))

	query, args := sbBuilder.Build()

	var count int32
	err := r.pool.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		r.logger.Error().Ctx(ctx).
			Err(err).
			Str("query", query).
			Msg("Failed to get job items count")
		return 0, r.mapPgError(ctx, err, "", "job items")
	}

	return count, nil
}

func (r *Repository) DeleteCallJob(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error {
	err := r.db.DeleteCallJob(ctx, gen.DeleteCallJobParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		return r.mapPgError(ctx, err, id.String(), "call job")
	}

	return nil
}

func (r *Repository) UpdateCallJob(ctx context.Context, callJob entity.CallJob) error {
	pgCallJob := r.mapToPostgresCallJob(ctx, callJob)

	err := r.db.UpdateCallJob(ctx, gen.UpdateCallJobParams{
		ID:                     pgCallJob.ID,
		Name:                   pgCallJob.Name,
		WorkflowID:             pgCallJob.WorkflowID,
		DocumentID:             pgCallJob.DocumentID,
		DocumentType:           pgCallJob.DocumentType,
		PrefferedLanguage:      pgCallJob.PrefferedLanguage,
		MaxRetries:             pgCallJob.MaxRetries,
		RetryDelay:             pgCallJob.RetryDelay,
		OutboundCallProviderID: pgCallJob.OutboundCallProviderID,
		TenantID:               pgCallJob.TenantID,
		Status:                 callJob.Status.String(),
		IsMaterialised:         callJob.Materialized,
		UpdatedAt:              pgCallJob.UpdatedAt,
	})
	if err != nil {
		return r.mapPgError(ctx, err, pgCallJob.ID.String(), "call job")
	}
	return nil
}

func (r *Repository) UpdateCallJobStatus(ctx context.Context, jobId uuid.UUID, status entity.CallJobStatus, tenantID uuid.UUID) error {
	err := r.db.UpdateCallJobStatus(ctx, gen.UpdateCallJobStatusParams{
		ID:       jobId,
		Status:   status.String(),
		TenantID: tenantID,
	})
	if err != nil {
		return r.mapPgError(ctx, err, jobId.String(), "call job")
	}
	return nil
}

func (r *Repository) InsertJobItem(ctx context.Context, row entity.JobItem) error {
	pg_row := r.mapToPostgresJobItem(ctx, row)

	queries := r.getQueries(ctx)
	err := queries.InsertJobItem(ctx, gen.InsertJobItemParams{
		ID:           pg_row.ID,
		PhoneNo:      pg_row.PhoneNo,
		AgentContext: pg_row.AgentContext,
		CallStatus:   pg_row.CallStatus,
		JobID:        pg_row.JobID,
		JobData:      pg_row.JobData,
		CreatedBy:    pg_row.CreatedBy,
		TenantID:     pg_row.TenantID,
	})

	if err != nil {
		return r.mapPgError(ctx, err, pg_row.ID.String(), "job item")
	}

	return nil
}

func (r *Repository) INTERNALGetJobItem(ctx context.Context, jobItemId uuid.UUID) (entity.JobItem, error) {
	jobItem, err := r.getQueries(ctx).INTERNALGetJobItemById(ctx, jobItemId)
	if err != nil {
		return entity.JobItem{}, r.mapPgError(ctx, err, jobItemId.String(), "job item")
	}
	return r.mapToJobItemEntity(ctx, jobItem)
}

func (r *Repository) GetJobItems(
	ctx context.Context,
	jobID uuid.UUID,
	limit int32,
	offset int32,
	tenantID uuid.UUID,
) ([]entity.JobItem, error) {
	pool := r.pool
	sbBuilder := sb.PostgreSQL.NewSelectBuilder()
	sbBuilder.Select(
		"id",
		"phone_no",
		"agent_context",
		"job_id",
		"COALESCE(job_data, '') AS job_data", // COALESCE for nullable fields
		"COALESCE(external_call_id, '') AS external_call_id",
		"COALESCE(call_status, 'unknown') AS call_status",
		"call_started_at",
		"call_ended_at",
		"call_duration",
		"call_error_message",
		"created_by",
		"tenant_id",
		"updated_at",
	).From("job_item").Where(sbBuilder.Equal("job_id", jobID)).Where(sbBuilder.Equal("tenant_id", tenantID)).
		OrderBy("call_started_at ASC")

	applyPagination(sbBuilder, limit, offset)

	query, args := sbBuilder.Build()
	r.logger.Debug().Ctx(ctx).Str("query", query).Interface("args", args).Msg("Executing GetJobItems query")

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Str("query", query).Msg("Failed to execute GetJobItems query")
		return nil, r.mapPgError(ctx, err, "", "jobItems")
	}
	defer rows.Close()

	items := make([]entity.JobItem, 0)
	rowCount := 0
	for rows.Next() {
		rowCount++
		job, err := scanJobItemRow(rows)
		if err != nil {
			r.logger.Error().Ctx(ctx).Err(err).Int("row_number", rowCount).Msg("Failed to scan job item row")
			return nil, r.mapPgError(ctx, err, "", "jobItems")
		}
		items = append(items, job)
	}

	if err := rows.Err(); err != nil {
		r.logger.Error().Ctx(ctx).Err(err).Msg("Error iterating job item rows")
		return nil, r.mapPgError(ctx, err, "", "jobItems")
	}

	r.logger.Debug().Ctx(ctx).Int("count", len(items)).Msg("Successfully fetched job items")
	return items, nil
}

func scanJobItemRow(rows pgx.Rows) (entity.JobItem, error) {
	var job entity.JobItem

	var externalCallID string
	var statusStr string

	err := rows.Scan(
		&job.ID,
		&job.PhoneNumber,
		&job.AgentContext,
		&job.JobID,
		&job.JobData,
		&externalCallID,
		&statusStr,
		&job.CallStartedAt,
		&job.CallEndedAt,
		&job.CallDuration,
		&job.CallErrorMessage,
		&job.CreatedBy,
		&job.TenantID,
		&job.UpdatedAt,
	)
	if err != nil {
		return entity.JobItem{}, err
	}

	// normalize empty string → nil
	if externalCallID != "" {
		job.ExternalCallID = &externalCallID
	}

	job.Status, err = entity.ParseCallStatus(statusStr)
	if err != nil {
		return entity.JobItem{}, err
	}

	return job, nil
}

func (r *Repository) UpdateJobItem(ctx context.Context, row entity.JobItem) error {
	var callStartedAt pgtype.Timestamp // callStartedAt
	if row.CallStartedAt != nil {
		callStartedAt = pgtype.Timestamp{Time: *row.CallStartedAt, Valid: true}
	} else {
		callStartedAt = pgtype.Timestamp{Valid: false}
	}
	var callEndedAt pgtype.Timestamp // callEndedAt
	if row.CallEndedAt != nil {
		callEndedAt = pgtype.Timestamp{Time: *row.CallEndedAt, Valid: true}
	} else {
		callEndedAt = pgtype.Timestamp{Valid: false}
	}
	var callDuration pgtype.Int4 // callDuration
	if row.CallDuration != nil {
		callDuration = pgtype.Int4{Int32: *row.CallDuration, Valid: true}
	} else {
		callDuration = pgtype.Int4{Valid: false}
	}
	var callErrorMessage pgtype.Text // callErrorMessage
	if row.CallErrorMessage != nil {
		callErrorMessage = pgtype.Text{String: *row.CallErrorMessage, Valid: true}
	} else {
		callErrorMessage = pgtype.Text{Valid: false}
	}
	var jobData pgtype.Text // jobData
	if row.JobData != "" {
		jobData = pgtype.Text{String: row.JobData, Valid: true}
	} else {
		jobData = pgtype.Text{Valid: false}
	}
	var externalCallID pgtype.Text // externalCallID
	if row.ExternalCallID != nil {
		externalCallID = pgtype.Text{String: *row.ExternalCallID, Valid: true}
	} else {
		externalCallID = pgtype.Text{Valid: false}
	}
	var callStatus pgtype.Text // callStatus
	if row.Status != entity.CallJobItemStatusUnknown {
		callStatus = pgtype.Text{String: row.Status.String(), Valid: true}
	} else {
		callStatus = pgtype.Text{Valid: false}
	}

	err := r.getQueries(ctx).UpdateJobItem(ctx, gen.UpdateJobItemParams{
		ID:               row.ID,
		TenantID:         row.TenantID,
		CallStatus:       callStatus,
		CallStartedAt:    callStartedAt,
		CallEndedAt:      callEndedAt,
		CallDuration:     callDuration,
		CallErrorMessage: callErrorMessage,
		JobData:          jobData,
		ExternalCallID:   externalCallID,
		UpdatedAt:        pgtype.Timestamp{Time: row.UpdatedAt, Valid: !row.UpdatedAt.IsZero()},
	})
	if err != nil {
		return r.mapPgError(ctx, err, row.ID.String(), "job item")
	}
	return nil
}

// GetJobItemByExternalCallId retrieves a job item by its external call ID.
func (r *Repository) GetJobItemByExternalCallId(ctx context.Context, externalCallId string) (entity.JobItem, error) {
	jobItem, err := r.db.GetJobItemByExternalCallId(ctx, pgtype.Text{String: externalCallId, Valid: true})
	if err != nil {
		return entity.JobItem{}, r.mapPgError(ctx, err, externalCallId, "job item")
	}
	return r.mapToJobItemEntity(ctx, jobItem)
}

func (r *Repository) UpdateJobItemCallStatus(ctx context.Context, jobItemId uuid.UUID, statusUpdate entity.CallJobItemStatus) error {
	err := r.db.UpdateJobItemCallStatus(ctx, gen.UpdateJobItemCallStatusParams{
		ID:         jobItemId,
		CallStatus: pgtype.Text{String: statusUpdate.String(), Valid: true},
	})
	if err != nil {
		return r.mapPgError(ctx, err, jobItemId.String(), "job item")
	}
	return nil
}

func (r *Repository) GetJobItemTenantById(ctx context.Context, jobItemId uuid.UUID) (uuid.UUID, error) {
	tenantID, err := r.db.GetJobItemTenantById(ctx, jobItemId)
	if err != nil {
		return uuid.Nil, r.mapPgError(ctx, err, jobItemId.String(), "job item")
	}
	return tenantID, nil
}

func (r *Repository) GetJobItemsByIDs(ctx context.Context, jobID uuid.UUID, itemIDs []uuid.UUID, tenantID uuid.UUID) ([]entity.JobItem, error) {
	rows, err := r.db.GetJobItemsByIDs(ctx, gen.GetJobItemsByIDsParams{
		SourceJobID: jobID,
		JobItemIds:  itemIDs,
		TenantID:    tenantID,
	})
	if err != nil {
		return nil, r.mapPgError(ctx, err, jobID.String(), "job items")
	}

	jobItems := make([]entity.JobItem, 0, len(rows))
	for _, row := range rows {
		mappedItem, err := r.mapToJobItemEntity(ctx, row)
		if err != nil {
			r.logger.Error().Ctx(ctx).Err(err).Msg("Failed to map job item entity")
			return nil, xerrors.InternalError(ctx, err)
		}
		jobItems = append(jobItems, mappedItem)
	}

	return jobItems, nil
}

func (r *Repository) DeleteJobItem(ctx context.Context, id uuid.UUID, job_id uuid.UUID, tenantID uuid.UUID) error {
	err := r.db.DeleteJobItem(ctx, gen.DeleteJobItemParams{
		ID:       id,
		JobID:    job_id,
		TenantID: tenantID,
	})
	if err != nil {
		return r.mapPgError(ctx, err, id.String(), "job item")
	}
	return nil
}
