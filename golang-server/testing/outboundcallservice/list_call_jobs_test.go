package outboundcallservice

import (
	"time"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Verifies that ListCallJobs returns created call jobs with correct fields.
func (s *OutboundCallServiceSuite) TestListCallJobs_Success() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)
	s.NotEmpty(jobID, "Call job ID should not be empty")

	// List call jobs with a high limit to capture all
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{Limit: 100, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs should succeed")
	s.Require().NotNil(resp)

	jobs := resp.Msg.GetCallJobs()
	s.GreaterOrEqual(len(jobs), 1, "Should have at least 1 call job")

	// Verify the created job is present in the list
	found := false
	for _, job := range jobs {
		if job.GetId() == jobID {
			found = true
			s.NotEmpty(job.GetName(), "Job name should not be empty")
			s.NotEmpty(job.GetWorkflowId(), "Workflow ID should not be empty")
			s.NotNil(job.GetCreatedAt(), "Created timestamp should be set")
			break
		}
	}
	s.True(found, "Created call job should be in the list")

	// Verify total count is returned
	s.GreaterOrEqual(resp.Msg.GetTotalCount(), int32(1), "Total count should be at least 1")
}

// Verifies that ListCallJobs with limit returns at most limit jobs.
func (s *OutboundCallServiceSuite) TestListCallJobs_Pagination_Limit() {
	// Create multiple call jobs
	for i := range 5 {
		var reqData pb.CreateCallJobRequest
		s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
		reqData.Name = "Limit-Test-" + uuid.NewString() + "-" + string(rune('A'+i))
		_ = s.createCallJobViaAPI(&reqData)
	}

	// List with limit=2
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{Limit: 2, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with limit=2 should succeed")
	s.Require().NotNil(resp)

	jobs := resp.Msg.GetCallJobs()
	s.LessOrEqual(len(jobs), 2, "Should return at most 2 jobs with limit=2")
	s.GreaterOrEqual(resp.Msg.GetTotalCount(), int32(5), "TotalCount should reflect all matching jobs")
}

// Verifies that ListCallJobs can filter by status.
func (s *OutboundCallServiceSuite) TestListCallJobs_FilterByStatus() {
	ctx := s.T().Context()

	// Create a call job using request fixture (starts as PENDING)
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.Name = "Filter-Pending-" + uuid.NewString()
	pendingID := s.createCallJobViaAPI(&reqData)

	// Create jobs with other statuses via DB (they should NOT appear in PENDING filter results)
	completedID := s.insertCallJobWithStatus(ctx, "completed", "Filter-Completed-"+uuid.NewString())
	failedID := s.insertCallJobWithStatus(ctx, "failed", "Filter-Failed-"+uuid.NewString())

	// List jobs filtering by PENDING status
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:    100,
		Offset:   0,
		Statuses: []pb.CallJobStatus{pb.CallJobStatus_CALL_JOB_STATUS_PENDING},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with status filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have PENDING status
	for _, job := range resp.Msg.GetCallJobs() {
		s.Equal(pb.CallJobStatus_CALL_JOB_STATUS_PENDING, job.GetStatus(), "Job should have PENDING status")
	}

	// Collect all job IDs in the response
	jobIDs := make(map[string]bool)
	for _, job := range resp.Msg.GetCallJobs() {
		jobIDs[job.GetId()] = true
	}

	// Verify the PENDING job IS in the results
	s.True(jobIDs[pendingID], "PENDING job should be in filtered list")

	// Verify jobs with other statuses are NOT in the results
	s.False(jobIDs[completedID], "COMPLETED job should NOT be in PENDING filtered list")
	s.False(jobIDs[failedID], "FAILED job should NOT be in PENDING filtered list")
}

// Verifies that ListCallJobs can filter by PROCESSING, READY, and RUNNING statuses.
// NOTE: We insert records directly into the database because call jobs cannot "run"
// in integration tests. This is an exception to the normal practice of using API calls
// to set up test data.
func (s *OutboundCallServiceSuite) TestListCallJobs_FilterByStatus_ProcessingReadyRunning() {
	ctx := s.T().Context()

	// Create call jobs directly in the database with specific statuses
	// that cannot be achieved via API in integration tests
	processingJobID := s.insertCallJobWithStatus(ctx, "processing", "test-job-processing-"+uuid.NewString())
	readyJobID := s.insertCallJobWithStatus(ctx, "ready", "test-job-ready-"+uuid.NewString())
	runningJobID := s.insertCallJobWithStatus(ctx, "running", "test-job-running-"+uuid.NewString())

	// Filter by PROCESSING status only
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:    100,
		Offset:   0,
		Statuses: []pb.CallJobStatus{pb.CallJobStatus_CALL_JOB_STATUS_PROCESSING},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with PROCESSING filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have PROCESSING status
	for _, job := range resp.Msg.GetCallJobs() {
		s.Equal(pb.CallJobStatus_CALL_JOB_STATUS_PROCESSING, job.GetStatus(), "All jobs should have PROCESSING status")
	}

	// Verify the processing job is in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), processingJobID, "Processing job should be in filtered list")

	// Filter by READY status only
	req = connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:    100,
		Offset:   0,
		Statuses: []pb.CallJobStatus{pb.CallJobStatus_CALL_JOB_STATUS_READY},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err = s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with READY filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have READY status
	for _, job := range resp.Msg.GetCallJobs() {
		s.Equal(pb.CallJobStatus_CALL_JOB_STATUS_READY, job.GetStatus(), "All jobs should have READY status")
	}

	// Verify the ready job is in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), readyJobID, "Ready job should be in filtered list")

	// Filter by RUNNING status only
	req = connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:    100,
		Offset:   0,
		Statuses: []pb.CallJobStatus{pb.CallJobStatus_CALL_JOB_STATUS_RUNNING},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err = s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with RUNNING filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have RUNNING status
	for _, job := range resp.Msg.GetCallJobs() {
		s.Equal(pb.CallJobStatus_CALL_JOB_STATUS_RUNNING, job.GetStatus(), "All jobs should have RUNNING status")
	}

	// Verify the running job is in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), runningJobID, "Running job should be in filtered list")
}

// Verifies that ListCallJobs can filter by COMPLETED and FAILED statuses.
// NOTE: We insert records directly into the database because call jobs cannot "run"
// in integration tests. This is an exception to the normal practice of using API calls
// to set up test data.
func (s *OutboundCallServiceSuite) TestListCallJobs_FilterByStatus_CompletedAndFailed() {
	ctx := s.T().Context()

	// Create call jobs directly in the database with specific statuses
	// that cannot be achieved via API in integration tests
	completedJobID := s.insertCallJobWithStatus(ctx, "completed", "test-job-completed-"+uuid.NewString())
	failedJobID := s.insertCallJobWithStatus(ctx, "failed", "test-job-failed-"+uuid.NewString())

	// Filter by COMPLETED status only
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:    100,
		Offset:   0,
		Statuses: []pb.CallJobStatus{pb.CallJobStatus_CALL_JOB_STATUS_COMPLETED},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with COMPLETED filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have COMPLETED status
	for _, job := range resp.Msg.GetCallJobs() {
		s.Equal(pb.CallJobStatus_CALL_JOB_STATUS_COMPLETED, job.GetStatus(), "All jobs should have COMPLETED status")
	}

	// Verify the completed job is in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), completedJobID, "Completed job should be in filtered list")

	// Filter by FAILED status only
	req = connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:    100,
		Offset:   0,
		Statuses: []pb.CallJobStatus{pb.CallJobStatus_CALL_JOB_STATUS_FAILED},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err = s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with FAILED filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have FAILED status
	for _, job := range resp.Msg.GetCallJobs() {
		s.Equal(pb.CallJobStatus_CALL_JOB_STATUS_FAILED, job.GetStatus(), "All jobs should have FAILED status")
	}

	// Verify the failed job is in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), failedJobID, "Failed job should be in filtered list")
}

// Verifies that ListCallJobs can filter by multiple statuses.
// NOTE: We insert records directly into the database because call jobs cannot "run"
// in integration tests. This is an exception to the normal practice of using API calls
// to set up test data.
func (s *OutboundCallServiceSuite) TestListCallJobs_FilterByStatus_MultipleStatuses() {
	ctx := s.T().Context()

	// Create call jobs directly in the database with different statuses
	processingJobID := s.insertCallJobWithStatus(ctx, "processing", "test-job-multi-processing-"+uuid.NewString())
	completedJobID := s.insertCallJobWithStatus(ctx, "completed", "test-job-multi-completed-"+uuid.NewString())
	failedJobID := s.insertCallJobWithStatus(ctx, "failed", "test-job-multi-failed-"+uuid.NewString())

	// Filter by PROCESSING, COMPLETED, and FAILED statuses
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:  100,
		Offset: 0,
		Statuses: []pb.CallJobStatus{
			pb.CallJobStatus_CALL_JOB_STATUS_PROCESSING,
			pb.CallJobStatus_CALL_JOB_STATUS_COMPLETED,
			pb.CallJobStatus_CALL_JOB_STATUS_FAILED,
		},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with multiple status filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have one of the filtered statuses
	for _, job := range resp.Msg.GetCallJobs() {
		s.Contains(
			[]pb.CallJobStatus{
				pb.CallJobStatus_CALL_JOB_STATUS_PROCESSING,
				pb.CallJobStatus_CALL_JOB_STATUS_COMPLETED,
				pb.CallJobStatus_CALL_JOB_STATUS_FAILED,
			},
			job.GetStatus(),
			"Job should have one of the filtered statuses",
		)
	}

	// Verify all jobs are in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), processingJobID, "Processing job should be in filtered list")
	s.assertJobInList(resp.Msg.GetCallJobs(), completedJobID, "Completed job should be in filtered list")
	s.assertJobInList(resp.Msg.GetCallJobs(), failedJobID, "Failed job should be in filtered list")
}

// assertJobInList verifies that a job with the given ID exists in the job list.
func (s *OutboundCallServiceSuite) assertJobInList(jobs []*pb.GetCallJobResponse, jobID string, message string) {
	s.T().Helper()
	for _, job := range jobs {
		if job.GetId() == jobID {
			return
		}
	}
	s.Fail(message)
}

// Verifies that ListCallJobs with a high offset returns an empty list.
func (s *OutboundCallServiceSuite) TestListCallJobs_HighOffset_EmptyResult() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{Limit: 10, Offset: 999999})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with high offset should succeed")
	s.Require().NotNil(resp)

	jobs := resp.Msg.GetCallJobs()
	s.Empty(jobs, "High offset should return no call jobs")
}

// Verifies that ListCallJobs pagination offset advances the page window correctly.
func (s *OutboundCallServiceSuite) TestListCallJobs_Pagination_Offset() {
	// Create multiple call jobs using request fixtures
	for i := range 4 {
		var reqData pb.CreateCallJobRequest
		s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
		reqData.Name = "Offset-Test-" + uuid.NewString() + "-" + string(rune('A'+i))
		_ = s.createCallJobViaAPI(&reqData)
	}

	// Get first page
	ctx := s.T().Context()
	first, err := s.outboundClient.ListCallJobs(ctx, connect.NewRequest(&pb.ListCallJobsRequest{Limit: 2, Offset: 0}))
	s.Require().NotNil(first, "First page request should not be nil")
	s.Require().NoError(err, "First page request should succeed")

	// Get second page
	second, err := s.outboundClient.ListCallJobs(ctx, connect.NewRequest(&pb.ListCallJobsRequest{Limit: 2, Offset: 2}))
	s.Require().NoError(err, "Second page request should succeed")

	// Total count should be the same for both pages
	s.Equal(first.Msg.GetTotalCount(), second.Msg.GetTotalCount(), "Total count should be consistent across pages")

	// Pages should have different results
	firstIDs := make(map[string]bool)
	for _, job := range first.Msg.GetCallJobs() {
		firstIDs[job.GetId()] = true
	}

	for _, job := range second.Msg.GetCallJobs() {
		s.False(firstIDs[job.GetId()], "Job %s should not appear in both pages", job.GetId())
	}
}

// Verifies that ListCallJobs can filter by workflow_id.
func (s *OutboundCallServiceSuite) TestListCallJobs_FilterByWorkflowID() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// Get the job to find its workflow_id
	job := s.getCallJobViaAPI(jobID)
	workflowId := job.GetWorkflowId()
	s.NotEmpty(workflowId, "Workflow ID should not be empty")

	// List jobs filtering by workflow_id
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:      100,
		Offset:     0,
		WorkflowId: workflowId,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with workflow_id filter should succeed")
	s.Require().NotNil(resp)

	// All returned jobs should have the specified workflow_id
	for _, j := range resp.Msg.GetCallJobs() {
		s.Equal(workflowId, j.GetWorkflowId(), "Job should have the filtered workflow_id")
	}

	// The created job should be in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), jobID, "Created job should be in filtered list")
}

// Verifies that ListCallJobs can filter by date range.
func (s *OutboundCallServiceSuite) TestListCallJobs_FilterByDateRange() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// Get the job to find its created_at timestamp
	job := s.getCallJobViaAPI(jobID)
	createdAt := job.GetCreatedAt()
	s.Require().NotNil(createdAt, "Created timestamp should be set")

	// Create a date range filter around the creation time
	startTime := createdAt.AsTime().Add(-24 * time.Hour)
	endTime := createdAt.AsTime().Add(24 * time.Hour)

	// List jobs filtering by date range
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:           100,
		Offset:          0,
		FilterDateBegin: timestamppb.New(startTime),
		FilterDateEnd:   timestamppb.New(endTime),
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with date range filter should succeed")
	s.Require().NotNil(resp)

	// The created job should be in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), jobID, "Created job should be in date range filtered list")
}

// Verifies that ListCallJobs with zero limit uses the default limit of 50.
func (s *OutboundCallServiceSuite) TestListCallJobs_ZeroLimit_UsesDefault() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{Limit: 0, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with zero limit should succeed (uses default 50)")
	s.Require().NotNil(resp)

	// Should return at most 50 results (default limit)
	s.LessOrEqual(len(resp.Msg.GetCallJobs()), 50, "Zero limit should use default limit of 50")
}

// Verifies that ListCallJobs with negative offset clamps to 0.
func (s *OutboundCallServiceSuite) TestListCallJobs_NegativeOffset_ClampedToZero() {
	// Create a call job to ensure there's data
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	_ = s.createCallJobViaAPI(&reqData)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{Limit: 100, Offset: -10})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with negative offset should succeed (clamped to 0)")
	s.Require().NotNil(resp)

	// Should return results from the beginning (offset clamped to 0)
	s.GreaterOrEqual(len(resp.Msg.GetCallJobs()), 1, "Negative offset should be clamped to 0 and return results")
}

// Verifies that ListCallJobs with empty status filter returns all jobs.
func (s *OutboundCallServiceSuite) TestListCallJobs_EmptyStatusFilter_ReturnsAll() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// List jobs with empty status filter
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:    100,
		Offset:   0,
		Statuses: []pb.CallJobStatus{},
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with empty status filter should succeed")
	s.Require().NotNil(resp)

	// The created job should be in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), jobID, "Created job should be returned with empty status filter")
}

// Verifies that ListCallJobs with negative limit uses the default limit of 50.
func (s *OutboundCallServiceSuite) TestListCallJobs_NegativeLimit_UsesDefault() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{Limit: -10, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "Negative limit should succeed using default")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetCallJobs()), 50, "Negative limit should use default of 50")
}

// Verifies that ListCallJobs with high limit is clamped to max of 50.
func (s *OutboundCallServiceSuite) TestListCallJobs_HighLimit_ClampedToMax() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{Limit: 9999, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "High limit should be clamped to 50 and succeed")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetCallJobs()), 50, "High limit should be clamped to max 50")
}

// Verifies that ListCallJobs with invalid workflow_id returns BadRequest error.
func (s *OutboundCallServiceSuite) TestListCallJobs_InvalidWorkflowID_BadRequest() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:      100,
		Offset:     0,
		WorkflowId: "invalid-workflow-id",
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().Error(err, "ListCallJobs with invalid workflow_id should return error")

	// Verify it's a BadRequest error (connect.CodeInvalidArgument)
	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// Verifies that ListCallJobs with non-existent workflow_id returns empty list.
func (s *OutboundCallServiceSuite) TestListCallJobs_NonExistentWorkflowID_EmptyResult() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:      100,
		Offset:     0,
		WorkflowId: uuid.NewString(), // Valid UUID but doesn't exist
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with non-existent workflow_id should succeed")
	s.Require().NotNil(resp)
	s.Empty(resp.Msg.GetCallJobs(), "Non-existent workflow_id should return empty list")
}

// Verifies that ListCallJobs with end date before start date handles the range correctly.
func (s *OutboundCallServiceSuite) TestListCallJobs_DateRange_EndBeforeStart() {
	ctx := s.T().Context()

	// End date is before start date (invalid range)
	startTime := time.Now()
	endTime := startTime.Add(-24 * time.Hour) // End is 24 hours before start

	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:           100,
		Offset:          0,
		FilterDateBegin: timestamppb.New(startTime),
		FilterDateEnd:   timestamppb.New(endTime),
	})
	req.Header().Set("Authorization", "Bearer test-token")

	// The API should either return empty or handle gracefully
	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with inverted date range should succeed")
	s.Require().NotNil(resp)
	// With inverted range, no jobs should match
	s.Empty(resp.Msg.GetCallJobs(), "Inverted date range should return no jobs")
}

// Verifies that ListCallJobs with only start date filters from that point forward.
func (s *OutboundCallServiceSuite) TestListCallJobs_DateRange_OnlyStartDate() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// Get the job to find its created_at timestamp
	job := s.getCallJobViaAPI(jobID)
	createdAt := job.GetCreatedAt()
	s.Require().NotNil(createdAt, "Created timestamp should be set")

	// Use only start date (1 day before creation)
	startTime := createdAt.AsTime().Add(-24 * time.Hour)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:           100,
		Offset:          0,
		FilterDateBegin: timestamppb.New(startTime),
		// No FilterDateEnd
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with only start date should succeed")
	s.Require().NotNil(resp)

	// The created job should be in the list (created after start date)
	s.assertJobInList(resp.Msg.GetCallJobs(), jobID, "Created job should be in list when only start date provided")
}

// Verifies that ListCallJobs with only end date filters up to that point.
func (s *OutboundCallServiceSuite) TestListCallJobs_DateRange_OnlyEndDate() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// Get the job to find its created_at timestamp
	job := s.getCallJobViaAPI(jobID)
	createdAt := job.GetCreatedAt()
	s.Require().NotNil(createdAt, "Created timestamp should be set")

	// Use only end date (1 day after creation)
	endTime := createdAt.AsTime().Add(24 * time.Hour)

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:         100,
		Offset:        0,
		FilterDateEnd: timestamppb.New(endTime),
		// No FilterDateBegin
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with only end date should succeed")
	s.Require().NotNil(resp)

	// The created job should be in the list (created before end date)
	s.assertJobInList(resp.Msg.GetCallJobs(), jobID, "Created job should be in list when only end date provided")
}

// Verifies that ListCallJobs can combine multiple filters (status, workflow_id, date range).
func (s *OutboundCallServiceSuite) TestListCallJobs_CombinedFilters() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// Get the job to find its workflow_id and created_at
	job := s.getCallJobViaAPI(jobID)
	workflowId := job.GetWorkflowId()
	createdAt := job.GetCreatedAt()
	s.Require().NotNil(createdAt, "Created timestamp should be set")

	// Create date range around the creation time
	startTime := createdAt.AsTime().Add(-24 * time.Hour)
	endTime := createdAt.AsTime().Add(24 * time.Hour)

	// List jobs with combined filters
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListCallJobsRequest{
		Limit:           100,
		Offset:          0,
		WorkflowId:      workflowId,
		Statuses:        []pb.CallJobStatus{pb.CallJobStatus_CALL_JOB_STATUS_PENDING},
		FilterDateBegin: timestamppb.New(startTime),
		FilterDateEnd:   timestamppb.New(endTime),
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListCallJobs(ctx, req)
	s.Require().NoError(err, "ListCallJobs with combined filters should succeed")
	s.Require().NotNil(resp)

	// The created job should be in the list
	s.assertJobInList(resp.Msg.GetCallJobs(), jobID, "Created job should be in combined filtered list")

	// All returned jobs should match all filters
	for _, j := range resp.Msg.GetCallJobs() {
		s.Equal(workflowId, j.GetWorkflowId(), "Job should have the filtered workflow_id")
		s.Equal(pb.CallJobStatus_CALL_JOB_STATUS_PENDING, j.GetStatus(), "Job should have PENDING status")
	}
}
