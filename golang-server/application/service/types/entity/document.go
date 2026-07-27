package entity

import (
	"time"

	"github.com/google/uuid"
)

// Document struct represents Document Table.
type Document struct {
	ID           uuid.UUID
	DocumentName string
	DocumentUrl  string
	IsIndexed    bool
	IsActive     bool
	IsDeleted    bool
	CreatedBy    string
	TenantID     uuid.UUID
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
