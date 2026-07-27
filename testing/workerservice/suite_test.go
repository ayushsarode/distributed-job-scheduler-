package workerservice

import (
	"testing"

	"github.com/ayushsarode/distributed-job-scheduler/testing/base"
	"github.com/stretchr/testify/suite"
)

type WorkerServiceSuite struct {
	base.IntegrationSuite
}

func TestWorkerServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(WorkerServiceSuite))
}
