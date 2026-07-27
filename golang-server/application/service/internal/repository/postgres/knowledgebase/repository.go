package knowledgebase

import (
	"context"
	"fmt"

	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/rs/zerolog"
)

type Repository struct {
	logger *zerolog.Logger
	db     *gen.Queries
}

// getQueries returns the appropriate Queries instance - either with transaction or the default one.
func (r *Repository) getQueries(ctx context.Context) *gen.Queries {
	if tx, ok := transaction.TxFromContext(ctx); ok {
		return r.db.WithTx(tx)
	}
	return r.db
}

var _ (types.KnoledgeBaseRepository) = (*Repository)(nil)

// UpdateDocument implements types.KnoledgeBaseRepository.
func (r *Repository) UpdateDocument(ctx context.Context, document entity.Document) error {
	queries := r.getQueries(ctx)
	_, err := queries.UpdateDocument(ctx, gen.UpdateDocumentParams{
		ID:        document.ID,
		IsActive:  document.IsActive,
		IsIndexed: document.IsIndexed,
		UpdatedAt: pgtype.Timestamp{Time: document.UpdatedAt, Valid: true},
		TenantID:  document.TenantID,
		IsDeleted: document.IsDeleted,
	})
	if err != nil {
		r.logger.Err(err).
			Ctx(ctx).
			Str("documentID", document.ID.String()).
			Msg("unable to update document")
		// TODO: use xerrors
		return fmt.Errorf("unable to update document: %w", err)
	}
	return nil
}

// GetDocumentById implements types.KnoledgeBaseRepository.
func (r *Repository) GetDocumentById(ctx context.Context, tenantID uuid.UUID, documentId string) (entity.Document, error) {
	id, err := uuid.Parse(documentId)
	if err != nil {
		r.logger.Err(err).
			Ctx(ctx).
			Str("documentId", documentId).
			Msg("invalid document ID format")
		return entity.Document{}, fmt.Errorf("invalid document ID format: %w", err)
	}

	queries := r.getQueries(ctx)
	doc, err := queries.GetDocumentById(ctx, gen.GetDocumentByIdParams{
		ID:       id,
		TenantID: tenantID,
	})
	if err != nil {
		r.logger.Err(err).
			Ctx(ctx).
			Str("documentId", documentId).
			Str("tenantID", tenantID.String()).
			Msg("unable to get document by id")
		return entity.Document{}, fmt.Errorf("unable to get document by id: %w", err)
	}

	return r.mapToDocument(doc), nil
}

// ListDocuments implements types.KnoledgeBaseRepository.
func (r *Repository) ListDocuments(ctx context.Context, tenantID uuid.UUID, limit int32, offset int32) ([]entity.Document, error) {
	queries := r.getQueries(ctx)
	docs, err := queries.ListDocuments(ctx, gen.ListDocumentsParams{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		r.logger.Err(err).Ctx(ctx).Msg("unable to list documents")
		// TODO: use xerrors
		return nil, fmt.Errorf("unable to list documents: %w", err)
	}

	mappedDocs := make([]entity.Document, 0, len(docs))
	for _, doc := range docs {
		mappedDocs = append(mappedDocs, r.mapToDocument(doc))
	}

	return mappedDocs, nil
}

// InsertDocument implements types.KnoledgeBaseRepository.
func (r *Repository) InsertDocument(ctx context.Context, document entity.Document) (entity.Document, error) {
	queries := r.getQueries(ctx)
	doc, err := queries.InsertDocument(ctx, gen.InsertDocumentParams{
		ID:           document.ID,
		CreatedBy:    document.CreatedBy,
		IsActive:     document.IsActive,
		IsIndexed:    document.IsIndexed,
		DocumentUrl:  document.DocumentUrl,
		DocumentName: document.DocumentName,
		TenantID:     document.TenantID,
	})
	if err != nil {
		// TODO: use xerrors
		return entity.Document{}, fmt.Errorf("unable to insert document: %w", err)
	}

	return entity.Document{
		ID:           doc.ID,
		DocumentName: doc.DocumentName,
		DocumentUrl:  doc.DocumentUrl,
		IsIndexed:    doc.IsIndexed,
		IsActive:     doc.IsActive,
		CreatedBy:    doc.CreatedBy,
		CreatedAt:    doc.CreatedAt.Time,
		UpdatedAt:    doc.UpdatedAt.Time,
		TenantID:     doc.TenantID,
	}, nil
}

func NewRepository(ctx context.Context) *Repository {
	return &Repository{
		logger: zerolog.Ctx(ctx),
		db:     gen.New(postgres.Ctx(ctx)),
	}
}
