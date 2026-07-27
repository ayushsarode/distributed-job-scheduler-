package transcriptionstorage

import (
	"time"

	"exiro.ai/application/service/types/entity"
)

// aggregationRequest holds parameters for session aggregation.
type aggregationRequest struct {
	SessionID    string
	TenantID     string
	DurationSecs float64
	IsStale      bool
}

// callDocument represents the final transcript document uploaded to S3.
type callDocument struct {
	SessionID string               `json:"session_id"`
	TenantID  string               `json:"owner_id"`
	Meta      entity.MetaItem      `json:"meta"`
	Segments  []entity.SegmentItem `json:"segments"`
	EndedAt   time.Time            `json:"ended_at"`
	Status    string               `json:"status"`
}
