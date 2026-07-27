package outboundcallservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Verifies that ListJobItems returns job items for a call job.
// A newly created job has no items, so this tests the empty list case.
func (s *OutboundCallServiceSuite) TestListJobItems_EmptyList() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)
	s.NotEmpty(jobID, "Call job ID should not be empty")

	// List job items - should return empty list for new job
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  100,
		Offset: 0,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "ListJobItems should succeed")
	s.Require().NotNil(resp)

	jobItems := resp.Msg.GetJobItems()
	s.Empty(jobItems, "Newly created job should have no items")

	// Verify total count is 0
	s.Equal(int32(0), resp.Msg.GetTotalCount(), "Total count should be 0 for empty job")
}

// Verifies that ListJobItems with limit returns at most limit items.
func (s *OutboundCallServiceSuite) TestListJobItems_Pagination_Limit() {
	// Create a call job and insert multiple job items
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)
	s.NotEmpty(jobID, "Call job ID should not be empty")

	ctx := s.T().Context()
	s.insertJobItems(ctx, jobID, 5)

	// List with limit=2
	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  2,
		Offset: 0,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "ListJobItems with limit=2 should succeed")
	s.Require().NotNil(resp)

	jobItems := resp.Msg.GetJobItems()
	s.LessOrEqual(len(jobItems), 2, "Should return at most 2 items with limit=2")
	s.GreaterOrEqual(resp.Msg.GetTotalCount(), int32(5), "TotalCount should reflect all matching items")
}

// Verifies that ListJobItems pagination offset advances the page window correctly.
func (s *OutboundCallServiceSuite) TestListJobItems_Pagination_Offset() {
	// Create a call job and insert multiple job items
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	ctx := s.T().Context()
	s.insertJobItems(ctx, jobID, 4)

	// Get first page
	first, err := s.outboundClient.ListJobItems(ctx, connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  2,
		Offset: 0,
	}))
	s.Require().NoError(err, "First page request should succeed")
	s.Require().NotNil(first)

	// Get second page
	second, err := s.outboundClient.ListJobItems(ctx, connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  2,
		Offset: 2,
	}))
	s.Require().NoError(err, "Second page request should succeed")

	// Total count should be the same for both pages
	s.Equal(first.Msg.GetTotalCount(), second.Msg.GetTotalCount(), "Total count should be consistent across pages")

	// Pages should have different results
	firstIDs := make(map[string]bool)
	for _, item := range first.Msg.GetJobItems() {
		firstIDs[item.GetId()] = true
	}

	for _, item := range second.Msg.GetJobItems() {
		s.False(firstIDs[item.GetId()], "Job item %s should not appear in both pages", item.GetId())
	}
}

// Verifies that ListJobItems with a high offset returns an empty list.
func (s *OutboundCallServiceSuite) TestListJobItems_HighOffset_EmptyResult() {
	// Create a call job using request fixture
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)

	jobID := s.createCallJobViaAPI(&reqData)

	// List with high offset
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  10,
		Offset: 999999,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "ListJobItems with high offset should succeed")
	s.Require().NotNil(resp)

	jobItems := resp.Msg.GetJobItems()
	s.Empty(jobItems, "High offset should return no job items")
}

// Verifies that ListJobItems with invalid job_id returns BadRequest error.
func (s *OutboundCallServiceSuite) TestListJobItems_InvalidJobID_BadRequest() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  "invalid-job-id",
		Limit:  10,
		Offset: 0,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().Error(err, "ListJobItems with invalid job_id should return error")

	// Verify it's a BadRequest error (connect.CodeInvalidArgument)
	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeInvalidArgument, connectErr.Code(), "Error code should be InvalidArgument/BadRequest")
}

// Verifies that ListJobItems with zero limit uses the default limit of 50.
func (s *OutboundCallServiceSuite) TestListJobItems_ZeroLimit_UsesDefault() {
	// Create a call job and insert 55 job items to exceed the default limit of 50
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	ctx := s.T().Context()
	s.insertJobItems(ctx, jobID, 55)

	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  0,
		Offset: 0,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "Zero limit should succeed using default")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetJobItems()), 50, "Zero limit should use default of 50")
}

// Verifies that ListJobItems with negative limit uses the default limit of 50.
func (s *OutboundCallServiceSuite) TestListJobItems_NegativeLimit_UsesDefault() {
	// Create a call job and insert 55 job items to exceed the default limit of 50
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	ctx := s.T().Context()
	s.insertJobItems(ctx, jobID, 55)

	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  -10,
		Offset: 0,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "Negative limit should succeed using default")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetJobItems()), 50, "Negative limit should use default of 50")
}

// Verifies that ListJobItems with high limit is clamped to max of 50.
func (s *OutboundCallServiceSuite) TestListJobItems_HighLimit_ClampedToMax() {
	// Create a call job and insert 55 job items to exceed the max limit of 50
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	ctx := s.T().Context()
	s.insertJobItems(ctx, jobID, 55)

	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  9999,
		Offset: 0,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "High limit should be clamped to 50 and succeed")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetJobItems()), 50, "High limit should be clamped to max 50")
}

// Verifies that ListJobItems with negative offset is clamped to 0.
func (s *OutboundCallServiceSuite) TestListJobItems_NegativeOffset_ClampedToZero() {
	// Create a call job and insert job items to ensure we have data
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	jobID := s.createCallJobViaAPI(&reqData)

	ctx := s.T().Context()
	createdIDs := s.insertJobItems(ctx, jobID, 5)

	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  jobID,
		Limit:  10,
		Offset: -10,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().NoError(err, "Negative offset should be clamped to 0 and succeed")
	s.Require().NotNil(resp)

	// Should return results from the beginning (offset clamped to 0)
	foundIDs := make(map[string]bool)
	for _, item := range resp.Msg.GetJobItems() {
		foundIDs[item.GetId()] = true
	}
	for _, id := range createdIDs {
		s.True(foundIDs[id], "Negative offset should be clamped to 0 and include created job item %s", id)
	}
}

// Verifies that ListJobItems with non-existent job_id returns NotFound error.
// Job items are a sub-resource of call jobs, so querying for items of a non-existent
// job should return a not found error rather than an empty list.
func (s *OutboundCallServiceSuite) TestListJobItems_NonExistentJobID_NotFound() {
	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListJobItemsRequest{
		JobId:  uuid.NewString(), // Valid UUID but doesn't exist
		Limit:  10,
		Offset: 0,
	})
	req.Header().Set("Authorization", "Bearer test-token")

	_, err := s.outboundClient.ListJobItems(ctx, req)
	s.Require().Error(err, "ListJobItems with non-existent job_id should return error")

	// Verify it's a NotFound error
	var connectErr *connect.Error
	s.Require().ErrorAs(err, &connectErr, "Error should be a connect.Error")
	s.Equal(connect.CodeNotFound, connectErr.Code(), "Error code should be NotFound")
}
