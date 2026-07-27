package jobservice

import (
	"testing"

	"github.com/ayushsarode/distributed-job-scheduler/testing/base"
	"github.com/stretchr/testify/suite"
)

type JobServiceSuite struct {
	base.IntegrationSuite
}

func TestJobServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(JobServiceSuite))
}
