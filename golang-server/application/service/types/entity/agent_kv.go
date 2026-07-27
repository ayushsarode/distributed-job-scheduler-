package entity

import "time"

type AgentKVItem struct {
	SessionID string    `dynamodbav:"session_id"`
	Key       string    `dynamodbav:"key"`
	Value     string    `dynamodbav:"value"`
	TenantID  string    `dynamodbav:"tenant_id"`
	CreatedAt time.Time `dynamodbav:"created_at,unixtime"`
	UpdatedAt time.Time `dynamodbav:"updated_at,unixtime"`
}
