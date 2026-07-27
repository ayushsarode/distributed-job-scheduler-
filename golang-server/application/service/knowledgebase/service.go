package knowledgebase

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/internal/types"
	serviceTypes "exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/config"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Service struct {
	objectStore             types.ObjectStore
	knowledgeBaseRepository types.KnoledgeBaseRepository
	auditService            serviceTypes.AuditService
	transactionHandler      types.TransationHandler
	logger                  *zerolog.Logger
}

var _ (serviceTypes.KnowledgeBaseService) = (*Service)(nil)

func (s *Service) ChangeDocumentStatus(ctx context.Context, documentId string, isActive bool) error {
	if documentId == "" {
		s.logger.Error().Msg("documentId is empty")
		return xerrors.BadRequestError(ctx, errors.New("documentId cannot be empty"))
	}

	tenantID := auth.MustGetTenant(ctx)

	document, err := s.knowledgeBaseRepository.GetDocumentById(ctx, tenantID, documentId)
	if err != nil {
		s.logger.Err(err).
			Str("documentId", documentId).
			Msg("failed to fetch document details")
		return fmt.Errorf("failed to fetch document details for document %s: %w", documentId, err)
	}

	document.IsActive = isActive
	document.UpdatedAt = time.Now()

	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := s.knowledgeBaseRepository.UpdateDocument(ctx, document); err != nil {
			return err
		}
		return s.auditService.Log(ctx, serviceTypes.AuditEvent{
			Action:       serviceTypes.AuditActionDocumentStatusChanged,
			ResourceType: serviceTypes.AuditResourceTypeDocument,
			ResourceID:   documentId,
		})
	})
	if err != nil {
		s.logger.Err(err).
			Str("documentId", documentId).
			Bool("newStatus", document.IsActive).
			Msg("failed to change document status")
		return fmt.Errorf("failed to change status for document %s: %w", documentId, err)
	}

	s.logger.Debug().
		Str("documentId", documentId).
		Bool("newStatus", document.IsActive).
		Msg("Document status changed successfully")

	return nil
}

func (s *Service) GetDocumentById(ctx context.Context, documentId string) (entity.Document, error) {
	tenantID := auth.MustGetTenant(ctx)
	return s.knowledgeBaseRepository.GetDocumentById(ctx, tenantID, documentId)
}

func (s *Service) UpdateDocument(ctx context.Context, document entity.Document) error {
	return s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := s.knowledgeBaseRepository.UpdateDocument(ctx, document); err != nil {
			return err
		}
		return s.auditService.Log(ctx, serviceTypes.AuditEvent{
			Action:       serviceTypes.AuditActionDocumentUpdated,
			ResourceType: serviceTypes.AuditResourceTypeDocument,
			ResourceID:   document.ID.String(),
		})
	})
}

// ListDocuments implements types.KnowledgeBaseService.
func (s *Service) ListDocuments(ctx context.Context, limit int32, offset int32) ([]entity.Document, error) {
	tenantID := auth.MustGetTenant(ctx)
	docs, err := s.knowledgeBaseRepository.ListDocuments(ctx, tenantID, limit, offset)
	if err != nil {
		s.logger.Err(err).Msg("unable to list documents")

		// TODO: Use xerrors package to wrap errors.
		return nil, err
	}
	return docs, nil
}

// UploadDocument implements types.KnowledgeBaseService.
func (s *Service) UploadDocument(ctx context.Context, file io.Reader, documentType string, fileName string) error {
	tenantID := auth.MustGetTenant(ctx)
	userID := auth.MustGetUser(ctx)
	objectKey := fmt.Sprintf("%s/documents/%s", userID, fileName)
	contentType, err := s.getContentType(ctx, documentType)
	if err != nil {
		s.logger.Err(err).Msg("unable to get content type")
		return err
	}
	documentUrl, err := s.objectStore.PutObject(ctx, file, config.Ctx(ctx).DocumentsBucket, objectKey, contentType)
	if err != nil {
		s.logger.Err(err).Msg("unable to upload document")
		return err
	}

	documentID := uuid.Must(uuid.NewV7())
	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		_, err = s.knowledgeBaseRepository.InsertDocument(ctx, entity.Document{
			ID:           documentID,
			DocumentName: fileName,
			DocumentUrl:  documentUrl,
			CreatedBy:    userID,
			TenantID:     tenantID,
		})
		if err != nil {
			return err
		}
		return s.auditService.Log(ctx, serviceTypes.AuditEvent{
			Action:       serviceTypes.AuditActionDocumentCreated,
			ResourceType: serviceTypes.AuditResourceTypeDocument,
			ResourceID:   documentID.String(),
		})
	})
	if err != nil {
		s.logger.Err(err).Msg("unable to insert document")
		return err
	}

	return nil
}

func (s *Service) DeleteDocument(ctx context.Context, documentId string) error {
	tenantID := auth.MustGetTenant(ctx)

	document, err := s.knowledgeBaseRepository.GetDocumentById(ctx, tenantID, documentId)
	if err != nil {
		s.logger.Err(err).
			Str("documentId", documentId).
			Msg("failed to fetch document details")
		return fmt.Errorf("failed to fetch document details for document %s: %w", documentId, err)
	}

	document.IsDeleted = true
	document.UpdatedAt = time.Now()

	err = s.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		if err := s.knowledgeBaseRepository.UpdateDocument(ctx, document); err != nil {
			return err
		}
		return s.auditService.Log(ctx, serviceTypes.AuditEvent{
			Action:       serviceTypes.AuditActionDocumentDeleted,
			ResourceType: serviceTypes.AuditResourceTypeDocument,
			ResourceID:   documentId,
		})
	})
	if err != nil {
		s.logger.Err(err).
			Str("documentId", documentId).
			Bool("isDeleted", document.IsDeleted).
			Msg("failed to delete document")
		return fmt.Errorf("failed to delete document %s: %w", documentId, err)
	}

	s.logger.Debug().
		Str("documentId", documentId).
		Bool("isDeleted", document.IsDeleted).
		Msg("Document deleted successfully")

	return nil
}

func (s *Service) getContentType(_ context.Context, fileType string) (string, error) {
	switch fileType {
	case "pdf":
		return "application/pdf", nil
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document", nil
	case "doc":
		return "application/msword", nil
	case "txt":
		return "text/plain", nil
	default:

		// TODO: Use xerrors package to wrap errors.
		return "", fmt.Errorf("unsupported file type: %s", fileType)
	}
}

func NewService(
	ctx context.Context,
	objectStore types.ObjectStore,
	knowledgeBaseRepository types.KnoledgeBaseRepository,
	auditService serviceTypes.AuditService,
	transactionHandler types.TransationHandler,
) *Service {
	assert.NotNil(objectStore)
	assert.NotNil(knowledgeBaseRepository)
	assert.NotNil(auditService)
	assert.NotNil(transactionHandler)

	return &Service{
		objectStore:             objectStore,
		knowledgeBaseRepository: knowledgeBaseRepository,
		auditService:            auditService,
		transactionHandler:      transactionHandler,
		logger:                  zerolog.Ctx(ctx),
	}
}
