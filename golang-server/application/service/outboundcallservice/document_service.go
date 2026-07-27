package outboundcallservice

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
)

func (s *Service) UploadOutboundCallDocument(ctx context.Context, filename string, fileType string, fileContent []byte) (string, error) {
	if filename == "" {
		s.logger.Error().Ctx(ctx).Msg("filename is empty")
		return "", xerrors.BadRequestError(ctx, errors.New("filename cannot be empty"))
	}

	if fileContent == nil {
		s.logger.Error().Ctx(ctx).Msg("fileContent is empty")
		return "", xerrors.BadRequestError(ctx, errors.New("fileContent cannot be empty"))
	}

	documentType := fileType
	if documentType == "" {
		s.logger.Error().Ctx(ctx).Msg("documentType is empty")
		return "", xerrors.BadRequestError(ctx, errors.New("documentType cannot be empty"))
	}

	user := auth.MustGetUser(ctx)
	tenantID := auth.MustGetTenant(ctx)
	documentID := uuid.Must(uuid.NewV7())

	ext := filepath.Ext(filename)
	objectKey := fmt.Sprintf("%s/documents/%s%s", user, documentID, ext)

	s.logger.Debug().Ctx(ctx).Str("document_type", documentType).Msg("Uploading document")

	// Convert []byte to io.Reader by creating a bytes.Reader
	fileReader := bytes.NewReader(fileContent)

	documentUrl, err := s.objectStore.PutObject(ctx, fileReader, s.outboundCallConfig.CampaignDocumentBucket, objectKey, documentType)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Msg("unable to upload document")
		return "", xerrors.BadRequestError(ctx, err)
	}

	// Create the entity document
	newDocument := entity.OutboundCallDocument{
		ID:           documentID,
		DocumentName: filename,
		DocumentType: documentType,
		DocumentUrl:  documentUrl,
		CreatedBy:    user,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		TenantID:     tenantID,
	}

	document, err := s.saveDocumentWithAudit(ctx, newDocument)
	if err != nil {
		return "", xerrors.BadRequestError(ctx, err)
	}

	s.logger.Info().Ctx(ctx).Str("document_id", document.ID.String()).Str("filename", filename).Msg("Document uploaded successfully")
	return document.ID.String(), nil
}

func (s *Service) saveDocumentWithAudit(ctx context.Context, newDocument entity.OutboundCallDocument) (entity.OutboundCallDocument, error) {
	var document entity.OutboundCallDocument
	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		var txErr error
		document, txErr = s.outboundCallDocumentRepository.UploadOutboundCallDocumentRepository(ctx, newDocument)
		if txErr != nil {
			s.logger.Err(txErr).Ctx(ctx).Msg("unable to save document metadata")
			return txErr
		}
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionOutboundCallDocCreated,
			ResourceType: types.AuditResourceTypeOutboundCallDocument,
			ResourceID:   document.ID.String(),
		})
	})
	return document, err
}

func (s *Service) ListOutboundCallDocuments(ctx context.Context, limit int32, offset int32) ([]entity.OutboundCallDocument, error) {
	tenantID := auth.MustGetTenant(ctx)

	if limit <= 0 {
		s.logger.Error().Ctx(ctx).Msg("limit should be greater than 0")
		return nil, xerrors.BadRequestError(ctx, errors.New("limit should be greater than 0"))
	}

	if offset < 0 {
		s.logger.Error().Ctx(ctx).Msg("offset should be greater than or equal to 0")
		return nil, xerrors.BadRequestError(ctx, errors.New("offset should be greater than or equal to 0"))
	}

	documents, err := s.outboundCallDocumentRepository.ListOutboundCallDocumentsRepository(ctx, tenantID, limit, offset)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Msg("unable to list documents")
		return nil, xerrors.BadRequestError(ctx, err)
	}

	s.logger.Debug().Ctx(ctx).Int("count", len(documents)).Msg("Listed documents successfully")
	return documents, nil
}

func (s *Service) GetOutboundCallDocument(ctx context.Context, documentId string, tenantID uuid.UUID) (entity.OutboundCallDocument, error) {
	if documentId == "" {
		s.logger.Error().Ctx(ctx).Msg("id is empty")
		return entity.OutboundCallDocument{}, xerrors.BadRequestError(ctx, errors.New("id cannot be empty"))
	}

	document, err := s.outboundCallDocumentRepository.GetOutboundCallDocumentRepository(ctx, documentId, tenantID)
	if err != nil {
		s.logger.Err(err).Ctx(ctx).Str("document_id", documentId).Msg("unable to get document")
		return entity.OutboundCallDocument{}, xerrors.BadRequestError(ctx, err)
	}

	return entity.OutboundCallDocument{
		ID:           document.ID,
		DocumentName: document.DocumentName,
		DocumentType: document.DocumentType,
		CreatedBy:    document.CreatedBy,
		TenantID:     document.TenantID,
		CreatedAt:    document.CreatedAt,
		UpdatedAt:    document.UpdatedAt,
	}, nil
}

func (s *Service) DeleteOutboundCallDocument(ctx context.Context, documentId string, tenantID uuid.UUID) (string, error) {
	if documentId == "" {
		s.logger.Error().Ctx(ctx).Msg("id is empty")
		return "", xerrors.BadRequestError(ctx, errors.New("id cannot be empty"))
	}

	err := s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := s.outboundCallDocumentRepository.DeleteOutboundCallDocumentRepository(ctx, documentId, tenantID); err != nil {
			s.logger.Err(err).Ctx(ctx).Str("document_id", documentId).Msg("unable to delete document")
			return err
		}
		return s.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionOutboundCallDocDeleted,
			ResourceType: types.AuditResourceTypeOutboundCallDocument,
			ResourceID:   documentId,
		})
	})
	if err != nil {
		return "", xerrors.BadRequestError(ctx, err)
	}

	s.logger.Info().Ctx(ctx).Str("document_id", documentId).Msg("Document deleted successfully")
	return "Document deleted successfully", nil
}
