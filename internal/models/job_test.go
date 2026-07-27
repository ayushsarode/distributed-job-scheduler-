package models

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewJob_Defaults(t *testing.T) {
	payload := json.RawMessage(`{"task":"email"}`)

	job := NewJob("send-email", payload, 7)

	require.NotEqual(t, uuid.Nil, job.ID)
	assert.Equal(t, JobStatusQueued, job.Status)
	assert.Equal(t, "send-email", job.Type)
	assert.JSONEq(t, string(payload), string(job.Payload))
	assert.Equal(t, int16(7), job.Priority)
	assert.Equal(t, 0, job.Attempts)
	assert.Nil(t, job.WorkerID)
}
