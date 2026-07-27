package entity

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type WorkflowStatus int32

const (
	WorkflowUnknown WorkflowStatus = iota

	WorkflowDraft
	WorkflowPublished
	WorkflowInActive
)

func (s WorkflowStatus) String() string {
	switch s {
	case WorkflowDraft:
		return "draft"
	case WorkflowPublished:
		return "published"
	case WorkflowInActive:
		return "inactive"
	case WorkflowUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func ParseWorkflowStatus(status string) (WorkflowStatus, error) {
	switch status {
	case "draft":
		return WorkflowDraft, nil
	case "published":
		return WorkflowPublished, nil
	case "inactive":
		return WorkflowInActive, nil
	case "unknown":
		return WorkflowUnknown, nil
	default:
		return WorkflowStatus(-1), errors.New("invalid WorkflowStatus: " + status)
	}
}

type Workflow struct {
	ID          uuid.UUID
	Name        string
	Description string
	Agent_id    string
	Status      WorkflowStatus
	CreatedBy   string
	TenantID    uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type WorkflowMetadata struct {
	ActiveCallJobCount int32
	TotalCallJobs      int32
}
