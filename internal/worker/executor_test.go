package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/ayushsarode/distributed-job-scheduler/internal/models"
	workermocks "github.com/ayushsarode/distributed-job-scheduler/internal/worker/mocks"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestExecutor_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	payload := json.RawMessage(`{"name":"daily-report"}`)
	job := &models.Job{
		ID:      uuid.New(),
		Type:    "report",
		Payload: payload,
	}
	runner := workermocks.NewMockJobRunner(ctrl)
	reporter := workermocks.NewMockResultReporter(ctrl)

	runner.EXPECT().
		Run(gomock.Any(), payload).
		Return(nil).
		Times(1)
	reporter.EXPECT().
		ReportResult(gomock.Any(), job, true, "").
		Return("ok", nil).
		Times(1)

	executor := NewExecutor(reporter, nil, map[string]JobRunner{
		"report": runner,
	}, zerolog.Nop())

	executor.execute(context.Background(), job)
}

func TestExecutor_Execute_RunnerError(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	job := &models.Job{
		ID:      uuid.New(),
		Type:    "report",
		Payload: json.RawMessage(`{}`),
	}
	runner := workermocks.NewMockJobRunner(ctrl)
	reporter := workermocks.NewMockResultReporter(ctrl)

	runner.EXPECT().
		Run(gomock.Any(), job.Payload).
		Return(errors.New("runner failed")).
		Times(1)
	reporter.EXPECT().
		ReportResult(gomock.Any(), job, false, "runner failed").
		Return("ok", nil).
		Times(1)

	executor := NewExecutor(reporter, nil, map[string]JobRunner{
		"report": runner,
	}, zerolog.Nop())

	executor.execute(context.Background(), job)
}

func TestExecutor_Execute_NoRunner(t *testing.T) {
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	job := &models.Job{
		ID:   uuid.New(),
		Type: "unknown",
	}
	reporter := workermocks.NewMockResultReporter(ctrl)

	reporter.EXPECT().
		ReportResult(gomock.Any(), job, false, "no runner registered for job type").
		Return("ok", nil).
		Times(1)

	executor := NewExecutor(reporter, nil, map[string]JobRunner{}, zerolog.Nop())

	executor.execute(context.Background(), job)
}

func TestExecutor_RunningJobs(t *testing.T) {
	executor := NewExecutor(nil, nil, nil, zerolog.Nop())

	assert.Equal(t, 0, executor.RunningJobs())
}
