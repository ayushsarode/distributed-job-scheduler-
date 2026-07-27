package deadletterservice

import (
	"testing"

	"github.com/ayushsarode/distributed-job-scheduler/testing/base"
	"github.com/stretchr/testify/suite"
)

type DeadLetterServiceSuite struct {
	base.IntegrationSuite
}

func TestDeadLetterServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(DeadLetterServiceSuite))
}
