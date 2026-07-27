package entity

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type CallJobStatus int

const (
	CallJobPending CallJobStatus = iota
	CallJobProcessing
	CallJobReady
	CallJobRunning
	CallJobCompleted
	CallJobFailed
	CallJobUnknown
)

func (s CallJobStatus) String() string {
	switch s {
	case CallJobPending:
		return "pending"
	case CallJobProcessing:
		return "processing"
	case CallJobReady:
		return "ready"
	case CallJobRunning:
		return "running"
	case CallJobCompleted:
		return "completed"
	case CallJobFailed:
		return "failed"
	case CallJobUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

func ParseCallJobStatus(status string) (CallJobStatus, error) {
	switch status {
	case "pending":
		return CallJobPending, nil
	case "processing":
		return CallJobProcessing, nil
	case "ready":
		return CallJobReady, nil
	case "running":
		return CallJobRunning, nil
	case "completed":
		return CallJobCompleted, nil
	case "failed":
		return CallJobFailed, nil
	case "unknown":
		return CallJobUnknown, nil
	default:
		return CallJobStatus(-1), errors.New("invalid CallJobStatus: " + status)
	}
}

// CallJobItemStatus represents the generic status of any call (provider-agnostic).
type CallJobItemStatus int

const (
	CallJobItemStatusPending CallJobItemStatus = iota
	CallJobItemStatusInitiated
	CallJobItemStatusQueued
	CallJobItemStatusRinging
	CallJobItemStatusAnswered
	CallJobItemStatusCompleted
	CallJobItemStatusBusy
	CallJobItemStatusFailed
	CallJobItemStatusNoAnswer
	CallJobItemStatusCanceled
	CallJobItemStatusUnknown
)

//nolint:cyclop
func (s CallJobItemStatus) String() string {
	switch s {
	case CallJobItemStatusPending:
		return "pending"
	case CallJobItemStatusInitiated:
		return "initiated"
	case CallJobItemStatusQueued:
		return "queued"
	case CallJobItemStatusRinging:
		return "ringing"
	case CallJobItemStatusAnswered:
		return "answered"
	case CallJobItemStatusCompleted:
		return "completed"
	case CallJobItemStatusBusy:
		return "busy"
	case CallJobItemStatusFailed:
		return "failed"
	case CallJobItemStatusNoAnswer:
		return "no-answer"
	case CallJobItemStatusCanceled:
		return "canceled"
	case CallJobItemStatusUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

//nolint:cyclop
func ParseCallStatus(status string) (CallJobItemStatus, error) {
	switch status {
	case "pending":
		return CallJobItemStatusPending, nil
	case "initiated":
		return CallJobItemStatusInitiated, nil
	case "queued":
		return CallJobItemStatusQueued, nil
	case "ringing":
		return CallJobItemStatusRinging, nil
	case "answered":
		return CallJobItemStatusAnswered, nil
	case "completed":
		return CallJobItemStatusCompleted, nil
	case "busy":
		return CallJobItemStatusBusy, nil
	case "failed":
		return CallJobItemStatusFailed, nil
	case "no-answer":
		return CallJobItemStatusNoAnswer, nil
	case "canceled":
		return CallJobItemStatusCanceled, nil
	case "unknown":
		return CallJobItemStatusUnknown, nil
	default:
		return CallJobItemStatusUnknown, errors.New("invalid CallStatus: " + status)
	}
}

// ParseCallStatusFromProvider maps provider-specific call status to our generic CallStatus.
// This function handles the mapping from different call providers (Twilio, etc.) to our standard enum.
//
//nolint:cyclop
func ParseCallStatusFromProvider(providerStatus string) CallJobItemStatus {
	// Normalize to lowercase for consistent mapping
	switch providerStatus {
	case "initiated", "queued":
		return CallJobItemStatusInitiated
	case "ringing":
		return CallJobItemStatusRinging
	case "in-progress", "answered":
		return CallJobItemStatusAnswered
	case "completed":
		return CallJobItemStatusCompleted
	case "busy":
		return CallJobItemStatusBusy
	case "failed":
		return CallJobItemStatusFailed
	case "no-answer":
		return CallJobItemStatusNoAnswer
	case "canceled":
		return CallJobItemStatusCanceled
	case "unknown":
		return CallJobItemStatusUnknown
	default:
		// Default to initiated for unknown status
		return CallJobItemStatusUnknown
	}
}

// CallJobItemCallEndDetails represents the data needed to update a job item's call status.
type CallJobItemCallEndDetails struct {
	Status           CallJobItemStatus // The new call status
	CallStartedAt    *time.Time        // When the call was answered/started
	CallEndedAt      *time.Time        // When the call ended
	CallDuration     *int32            // Duration in seconds
	CallErrorMessage *string           // Error message if call failed
}

type CallJob struct {
	ID                        uuid.UUID
	Name                      string
	Workflow_id               uuid.UUID
	Document_id               string
	Document_type             string
	Materialized              bool
	Status                    CallJobStatus
	Preffered_language        string
	Max_retries               int32
	Retry_delay               int32
	Outbound_call_provider_id string
	User                      string
	TenantID                  uuid.UUID
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

type JobItem struct {
	ID           uuid.UUID
	PhoneNumber  string
	JobID        uuid.UUID
	AgentContext string
	JobData      string
	// Generic call status fields (provider-agnostic)
	ExternalCallID   *string           // Provider's call ID (e.g., Twilio SID)
	Status           CallJobItemStatus // Generic call status (renamed from CallStatus)
	CallStartedAt    *time.Time
	CallEndedAt      *time.Time
	CallDuration     *int32
	CallErrorMessage *string
	CreatedBy        string
	TenantID         uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type NewJobItem struct {
	PhoneNumber  string
	AgentContext string
}

type CallDetails struct {
	Sid          string
	Status       string
	StartTime    time.Time
	EndTime      time.Time
	Duration     int // seconds
	From         string
	To           string
	RecordingUrl string
}
