package entity

import "time"

type TranscriptionMetadata struct {
	TenantID     string
	Status       string
	DownloadURL  string
	DurationSecs float64
	S3Key        string
	CreatedAt    time.Time
	LastUpdated  time.Time
}

// segmentItem represents a single transcription segment stored in DynamoDB.
type SegmentItem struct {
	SessionId    string    `json:"session_id" dynamodbav:"session_id"`
	SortKey      string    `json:"sort_key" dynamodbav:"sort_key"`
	SegmentId    string    `json:"segment_id" dynamodbav:"segment_id"`
	Sequence     uint64    `json:"sequence" dynamodbav:"sequence"`
	Speaker      string    `json:"speaker" dynamodbav:"speaker"`
	Text         string    `json:"text" dynamodbav:"text"`
	Timestamp    time.Time `json:"timestamp" dynamodbav:"ts,unixtime"`
	LanguageCode string    `json:"language" dynamodbav:"language"`
}

type MetaKey struct {
	SessionID string `dynamodbav:"session_id"`
	SortKey   string `dynamodbav:"sort_key"`
}

type MetadataDynamo struct {
	TenantID     string    `dynamodbav:"owner_id"`
	Status       string    `dynamodbav:"status"`
	DownloadURL  string    `dynamodbav:"download_url"`
	DurationSecs float64   `dynamodbav:"duration_secs"`
	S3Key        string    `dynamodbav:"s3_key"`
	CreatedAt    time.Time `dynamodbav:"created_at,unixtime"`
	LastUpdated  time.Time `dynamodbav:"last_updated,unixtime"`
}

// toDomainMetadata converts metadataDynamo to the public entity.TranscriptionMetadata.
func (d *MetadataDynamo) ToDomainMetadata() TranscriptionMetadata {
	return TranscriptionMetadata{
		TenantID:     d.TenantID,
		Status:       d.Status,
		DownloadURL:  d.DownloadURL,
		DurationSecs: d.DurationSecs,
		S3Key:        d.S3Key,
		CreatedAt:    d.CreatedAt,
		LastUpdated:  d.LastUpdated,
	}
}

// metaItem represents metadata about a transcription session.
type MetaItem struct {
	TenantID     string    `json:"owner_id"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	LastUpdated  time.Time `json:"last_updated"`
	DurationSecs float64   `json:"duration_secs,omitempty"`
}
