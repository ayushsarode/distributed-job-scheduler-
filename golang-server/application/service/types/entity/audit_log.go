package entity

import (
	"time"
	"github.com/google/uuid"
)

type AuditLog struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	UserID       string
	Action       string
	ResourceType string
	ResourceID   string
	CreatedAt    time.Time
}

type AuditLogQuery struct {
	ResourceType *string
	UserID       *string
	StartTime    *time.Time
	EndTime      *time.Time
	Limit        int32
	Offset       int32
}
