package outboundcalldocument

import (
	"context"
	"fmt"

	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/database/postgres/gen"
	"github.com/google/uuid"
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

var _ (types.OutboundCallDocumentRepository) = (*Repository)(nil)

func (r *Repository) UploadOutboundCallDocumentRepository(
	ctx context.Context,
	document entity.OutboundCallDocument,
) (entity.OutboundCallDocument, error) {
	r.logger.Debug().
		Str("document_id", document.ID.String()).
		Str("document_name", document.DocumentName).
		Str("tenant_id", document.TenantID.String()).
		Msg("Uploading outbound call document to repository")

	queries := r.getQueries(ctx)
	doc, err := queries.UploadOutboundCallDocument(ctx, gen.UploadOutboundCallDocumentParams{
		ID:           document.ID,
		DocumentName: document.DocumentName,
		DocumentUrl:  document.DocumentUrl,
		DocumentType: document.DocumentType,
		CreatedBy:    document.CreatedBy,
		TenantID:     document.TenantID,
	})
	if err != nil {
		// TODO: use xerrors
		return entity.OutboundCallDocument{}, fmt.Errorf("unable to insert document: %w", err)
	}

	return entity.OutboundCallDocument{
		ID:           doc.ID,
		DocumentName: doc.DocumentName,
		DocumentUrl:  doc.DocumentUrl,
		DocumentType: doc.DocumentType,
		CreatedBy:    doc.CreatedBy,
		TenantID:     doc.TenantID,
	}, nil
}

func (r *Repository) ListOutboundCallDocumentsRepository(ctx context.Context, tenantID uuid.UUID, limit int32, offset int32) ([]entity.OutboundCallDocument, error) {
		r.logger.Debug().
		Str("tenant_id", tenantID.String()).
		Int32("limit", limit).
		Int32("offset", offset).
		Msg("Listing outbound call documents from repository")

	queries := r.getQueries(ctx)
	docs, err := queries.ListOutboundCallDocuments(ctx, gen.ListOutboundCallDocumentsParams{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to list documents: %w", err)
	}

	outboundCallDocuments := make([]entity.OutboundCallDocument, len(docs))
	for i, doc := range docs {
		outboundCallDocuments[i] = entity.OutboundCallDocument{
			ID:           doc.ID,
			DocumentName: doc.DocumentName,
			DocumentUrl:  doc.DocumentUrl,
			DocumentType: doc.DocumentType,
			CreatedBy:    doc.CreatedBy,
			TenantID:     doc.TenantID,
		}
	}

	return outboundCallDocuments, nil
}

func (r *Repository) GetOutboundCallDocumentRepository(ctx context.Context, id string, tenantID uuid.UUID) (entity.OutboundCallDocument, error) {
	r.logger.Debug().
		Str("document_id", id).
		Str("tenant_id", tenantID.String()).
		Msg("Getting outbound call document from repository")

	documentId, err := uuid.Parse(id)
	if err != nil {
		return entity.OutboundCallDocument{}, fmt.Errorf("invalid document ID format: %w", err)
	}

	queries := r.getQueries(ctx)
	doc, err := queries.GetOutboundCallDocument(ctx, gen.GetOutboundCallDocumentParams{
		ID:       documentId,
		TenantID: tenantID,
	})
	if err != nil {
		return entity.OutboundCallDocument{}, fmt.Errorf("unable to get document: %w", err)
	}

	return entity.OutboundCallDocument{
		ID:           doc.ID,
		DocumentName: doc.DocumentName,
		DocumentUrl:  doc.DocumentUrl,
		DocumentType: doc.DocumentType,
		CreatedBy:    doc.CreatedBy,
		TenantID:     doc.TenantID,
	}, nil
}

func (r *Repository) DeleteOutboundCallDocumentRepository(ctx context.Context, id string, tenantID uuid.UUID) error {
	r.logger.Debug().
		Str("document_id", id).
		Str("tenant_id", tenantID.String()).
		Msg("Deleting outbound call document from repository")
	documentId, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid document ID format: %w", err)
	}
	queries := r.getQueries(ctx)
	_, err = queries.DeleteOutboundCallDocument(ctx, gen.DeleteOutboundCallDocumentParams{
		ID:       documentId,
		TenantID: tenantID,
	})
	if err != nil {
		return fmt.Errorf("unable to delete document: %w", err)
	}
	return nil
}

func NewRepository(ctx context.Context) *Repository {
	return &Repository{
		logger: zerolog.Ctx(ctx),
		db:     gen.New(postgres.Ctx(ctx)),
	}
}
