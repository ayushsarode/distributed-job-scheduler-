package usermanagement

import (
	"context"
	"fmt"
	"strings"
	"time"

	"exiro.ai/application/assert"
	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
)

type Service struct {
	userManagement     repositoryTypes.UserManagementRepository
	transactionHandler repositoryTypes.TransationHandler
	auditService       types.AuditService
}

var _ types.UserManagementService = &Service{}

func (u *Service) RegisterUser(ctx context.Context, userInfo entity.User) error {
	// Create tenant (default name: user's name or email)
	tenantName := fmt.Sprintf("%s %s", userInfo.FirstName, userInfo.LastName)
	if strings.TrimSpace(tenantName) == "" {
		tenantName = auth.MustGetUser(ctx) // Fallback to userID if name is empty
	}
	return u.transactionHandler.WithTransaction(ctx, func(ctx context.Context) error {
		tenantID := uuid.Must(uuid.NewV7())
		_, err := u.userManagement.CreateTenant(ctx, entity.Tenant{
			ID:        tenantID,
			Name:      tenantName,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
			IsActive:  true,
		})
		if err != nil {
			return err
		}

		ctx = auth.SetTenant(ctx, tenantID)

		userID := auth.MustGetUser(ctx)
		_, err = u.userManagement.RegisterUser(ctx, entity.User{
			UserID:    userID,
			FirstName: userInfo.FirstName,
			LastName:  userInfo.LastName,
			TenantID:  tenantID,
		})
		if err != nil {
			return err
		}
		return u.auditService.Log(ctx, types.AuditEvent{
			Action:       types.AuditActionUserCreated,
			ResourceType: types.AuditResourceTypeUser,
			ResourceID:   userID,
		})
	})
}

func (u *Service) GetUserTenantID(ctx context.Context, userID string) (uuid.UUID, error) {
	tenantID, err := u.userManagement.GetUserTenantID(ctx, userID)
	if err != nil {
		// If NotFoundError is returned, we create a new tenant.
		if xerrors.IsNotFoundError(err) {
			err := u.RegisterUser(ctx, entity.User{
				UserID:    userID,
				FirstName: "",
				LastName:  "",
			})
			if err != nil {
				return uuid.Nil, err
			}
			return u.userManagement.GetUserTenantID(ctx, userID)
		}

		return uuid.Nil, err
	}
	return tenantID, nil
}

func NewService(ctx context.Context, userManagementClient repositoryTypes.UserManagementRepository, transactionHandler repositoryTypes.TransationHandler, auditService types.AuditService) *Service {
	assert.NotNil(auditService)
	return &Service{
		userManagement:     userManagementClient,
		transactionHandler: transactionHandler,
		auditService:       auditService,
	}
}
