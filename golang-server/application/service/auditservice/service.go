package auditservice

import (
	"context"
	"time"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type Service struct {
	repo   repositoryTypes.AuditRepository
	logger *zerolog.Logger
}

var _ types.AuditService = &Service{}

func NewService(ctx context.Context, repo repositoryTypes.AuditRepository) *Service {
	assert.NotNil(repo)

	return &Service{
		repo:   repo,
		logger: zerolog.Ctx(ctx),
	}
}

func (s *Service) Log(ctx context.Context, event types.AuditEvent) error {
	userID := auth.MustGetUser(ctx)
	tenantID := auth.MustGetTenant(ctx)

	return s.repo.InsertAuditLog(ctx, entity.AuditLog{
		ID:           uuid.Must(uuid.NewV7()),
		TenantID:     tenantID,
		UserID:       userID,
		Action:       event.Action,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		CreatedAt:    time.Now(),
	})
}

func (s *Service) ListAuditLogs(ctx context.Context, query entity.AuditLogQuery) ([]entity.AuditLog, error) {
	tenantID := auth.MustGetTenant(ctx)
	return s.repo.ListAuditLogs(ctx, tenantID, query)
}
