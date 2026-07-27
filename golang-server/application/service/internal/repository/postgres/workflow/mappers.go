package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

func (r *Repository) mapToWorkflowEntity(_ context.Context, workflow gen.Workflow) (entity.Workflow, error) {
	status, err := entity.ParseWorkflowStatus(workflow.Status)
	if err != nil {
		return entity.Workflow{}, err
	}

	return entity.Workflow{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Description: workflow.Description.String,
		Agent_id:    workflow.AgentID,
		Status:      status,
		CreatedBy:   workflow.CreatedBy,
		TenantID:    workflow.TenantID,
		CreatedAt:   workflow.CreatedAt.Time,
		UpdatedAt:   workflow.UpdatedAt.Time,
	}, nil
}

func (r *Repository) mapToPostgresWorkflow(_ context.Context, workflow entity.Workflow) gen.Workflow {
	var description pgtype.Text
	if workflow.Description != "" {
		description = pgtype.Text{String: workflow.Description, Valid: true}
	} else {
		description = pgtype.Text{Valid: false}
	}

	return gen.Workflow{
		ID:          workflow.ID,
		Name:        workflow.Name,
		Description: description,
		AgentID:     workflow.Agent_id,
		CreatedBy:   workflow.CreatedBy,
		TenantID:    workflow.TenantID,
		Status:      workflow.Status.String(),
		CreatedAt:   pgtype.Timestamp{Time: workflow.CreatedAt, Valid: !workflow.CreatedAt.IsZero()},
		UpdatedAt:   pgtype.Timestamp{Time: workflow.UpdatedAt, Valid: !workflow.UpdatedAt.IsZero()},
	}
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
) error {
	if err == nil {
		return nil
	}

	// 1. No rows found (SELECT / DELETE)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return xerrors.NotFoundError(
			ctx,
			fmt.Errorf("workflow with id %s not found", resourceID),
		)
	}

	// 2. PostgreSQL-specific errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			return xerrors.ConflictError(
				ctx,
				errors.New("workflow already exists"),
			)

		case pgForeignKeyViolation:
			return xerrors.BadRequestError(
				ctx,
				errors.New("invalid Workflow reference"),
			)
		}
	}

	// 3. Fallback: unexpected database error
	r.logger.Error().
		Ctx(ctx).
		Err(err).
		Str("resource_id", resourceID).
		Msg("Unhandled postgres error")

	return xerrors.InternalError(
		ctx,
		errors.New("failed to process Workflow"),
	)
}
