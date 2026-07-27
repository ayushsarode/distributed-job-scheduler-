package entity

import (
	"time"

	"github.com/google/uuid"
)

type OutboundCallDocument struct {
	ID           uuid.UUID
	DocumentName string
	DocumentUrl  string
	DocumentType string
	IsDeleted    bool
	CreatedBy    string
	TenantID     uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
}