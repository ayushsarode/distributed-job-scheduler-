package workflowservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"github.com/google/uuid"
)

// Verifies that ListWorkflows with status filter returns only matching statuses.
func (s *WorkflowServiceSuite) TestListWorkflows_FilterByStatus_NoResults() {
	ctx := s.T().Context()
	s.clearWorkflows(ctx)

	// Create a workflow (starts as DRAFT status)
	agentID := s.createAgentViaAPI()
	s.createTestWorkflow(agentID, "NoResult-Test-"+uuid.NewString(), "Test")

	// Filter by INACTIVE status - should return empty since we only created DRAFT
	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{
		Statuses: []pb.WorkflowStatus{pb.WorkflowStatus_WORKFLOW_STATUS_INACTIVE},
	})
	s.Require().NoError(err)

	s.Empty(resp.GetWorkflows(), "Filtering by INACTIVE should return empty when only DRAFT exists")
	s.Equal(0, int(resp.GetTotalCount()))
}

// Verifies that ListWorkflows returns all workflows when empty status filter is provided.
func (s *WorkflowServiceSuite) TestListWorkflows_EmptyStatusFilter() {
	agentID := s.createAgentViaAPI()

	workflow1ID := s.createTestWorkflow(agentID, "Empty Filter Test 1 "+uuid.NewString(), "Test 1")
	workflow2ID := s.createTestWorkflow(agentID, "Empty Filter Test 2 "+uuid.NewString(), "Test 2")

	// Pass empty status array (should return all workflows)
	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{
		Statuses: []pb.WorkflowStatus{},
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	workflows := resp.GetWorkflows()
	s.GreaterOrEqual(len(workflows), 2, "Should return at least 2 workflows")
	s.Equal(len(workflows), int(resp.GetTotalCount()), "Total count should match workflows returned")

	workflowIDs := make(map[string]bool)
	for _, w := range workflows {
		workflowIDs[w.GetId()] = true
	}
	s.True(workflowIDs[workflow1ID], "Workflow 1 should be in the list")
	s.True(workflowIDs[workflow2ID], "Workflow 2 should be in the list")
}

// Verifies that total_count field is always populated correctly.
func (s *WorkflowServiceSuite) TestListWorkflows_TotalCountAccuracy() {
	agentID := s.createAgentViaAPI()

	prefix := uuid.NewString()[:8]
	s.createTestWorkflow(agentID, "Count-1-"+prefix, "Test")
	s.createTestWorkflow(agentID, "Count-2-"+prefix, "Test")
	s.createTestWorkflow(agentID, "Count-3-"+prefix, "Test")

	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{})
	s.Require().NoError(err)

	s.GreaterOrEqual(int(resp.GetTotalCount()), 3)
	s.Require().GreaterOrEqual(
		int(resp.GetTotalCount()),
		len(resp.GetWorkflows()),
		"total_count must be >= returned workflows length",
	)
}

// Verifies that ListWorkflows correctly filters when combining draft and published statuses.
func (s *WorkflowServiceSuite) TestListWorkflows_StatusFiltering_ExclusiveBehavior() {
	agentID := s.createAgentViaAPI()

	w1 := s.createTestWorkflow(agentID, "Draft "+uuid.NewString(), "Draft")
	w2 := s.createTestWorkflow(agentID, "Published "+uuid.NewString(), "Published")
	w3 := s.createTestWorkflow(agentID, "Inactive "+uuid.NewString(), "Inactive")

	s.deployAgentViaAPI(agentID)
	s.publishWorkflowViaAPI(w2)
	s.deactivateWorkflowViaAPI(w3)

	// Filter by DRAFT only
	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{
		Statuses: []pb.WorkflowStatus{
			pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT,
		},
	})
	s.Require().NoError(err)

	assertAllStatuses(s, resp.GetWorkflows(), pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT)

	ids := collectWorkflowIDs(resp.GetWorkflows())
	s.Contains(ids, w1)
	s.NotContains(ids, w2)
	s.NotContains(ids, w3)

	// Filter by PUBLISHED only
	resp, err = s.listWorkflows(&pb.ListWorkflowsRequest{
		Statuses: []pb.WorkflowStatus{
			pb.WorkflowStatus_WORKFLOW_STATUS_PUBLISHED,
		},
	})
	s.Require().NoError(err)

	assertAllStatuses(s, resp.GetWorkflows(), pb.WorkflowStatus_WORKFLOW_STATUS_PUBLISHED)

	ids = collectWorkflowIDs(resp.GetWorkflows())
	s.Contains(ids, w2)
	s.NotContains(ids, w1)
	s.NotContains(ids, w3)

	// Filter by INACTIVE only
	resp, err = s.listWorkflows(&pb.ListWorkflowsRequest{
		Statuses: []pb.WorkflowStatus{
			pb.WorkflowStatus_WORKFLOW_STATUS_INACTIVE,
		},
	})
	s.Require().NoError(err)

	assertAllStatuses(s, resp.GetWorkflows(), pb.WorkflowStatus_WORKFLOW_STATUS_INACTIVE)

	ids = collectWorkflowIDs(resp.GetWorkflows())
	s.Contains(ids, w3)
	s.NotContains(ids, w1)
	s.NotContains(ids, w2)
}

// Verifies that ListWorkflows respects the limit parameter and returns at most limit workflows.
func (s *WorkflowServiceSuite) TestListWorkflows_Pagination_Limit() {
	agentID := s.createAgentViaAPI()

	prefix := uuid.NewString()[:8]
	for i := range 5 {
		s.createTestWorkflow(agentID, "Page-"+prefix+"-"+string(rune('A'+i)), "Test")
	}

	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{
		Limit:  2,
		Offset: 0,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	s.LessOrEqual(len(resp.GetWorkflows()), 2, "Should return at most 2 workflows")
	s.GreaterOrEqual(int(resp.GetTotalCount()), 5, "TotalCount should reflect all matching workflows, not just the page")
}

// Verifies that ListWorkflows offset advances the page window correctly.
func (s *WorkflowServiceSuite) TestListWorkflows_Pagination_Offset() {
	agentID := s.createAgentViaAPI()

	for range 4 {
		s.createTestWorkflow(agentID, "Offset-"+uuid.NewString(), "Test")
	}

	first, err := s.listWorkflows(&pb.ListWorkflowsRequest{Limit: 2, Offset: 0})
	s.Require().NoError(err)

	second, err := s.listWorkflows(&pb.ListWorkflowsRequest{Limit: 2, Offset: 2})
	s.Require().NoError(err)

	s.Equal(first.GetTotalCount(), second.GetTotalCount())

	firstIDs := collectWorkflowIDs(first.GetWorkflows())
	for _, w := range second.GetWorkflows() {
		_, exists := firstIDs[w.GetId()]
		s.False(exists, "workflow appeared in both pages")
	}
}

// Verifies that offset beyond total count returns an empty list without error.
func (s *WorkflowServiceSuite) TestListWorkflows_Pagination_OffsetBeyondTotal() {
	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{
		Limit:  10,
		Offset: 999999,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	s.Empty(resp.GetWorkflows(), "Should return no workflows when offset exceeds total")
}

func (s *WorkflowServiceSuite) TestListWorkflows_NoFilters() {
	agentID := s.createAgentViaAPI()

	w1 := s.createTestWorkflow(agentID, "Alpha-"+uuid.NewString(), "First")
	w2 := s.createTestWorkflow(agentID, "Beta-"+uuid.NewString(), "Second")
	w3 := s.createTestWorkflow(agentID, "Gamma-"+uuid.NewString(), "Third")

	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{})
	s.Require().NoError(err)

	s.Require().GreaterOrEqual(int(resp.GetTotalCount()), 3)

	ids := collectWorkflowIDs(resp.GetWorkflows())
	s.Contains(ids, w1)
	s.Contains(ids, w2)
	s.Contains(ids, w3)
}

func (s *WorkflowServiceSuite) TestListWorkflows_Filter_Draft() {
	agentID := s.createAgentViaAPI()

	// Create workflows with different statuses
	draftID := s.createTestWorkflow(agentID, "Draft-"+uuid.NewString(), "Draft")
	publishedID := s.createTestWorkflow(agentID, "Published-"+uuid.NewString(), "Published")
	inactiveID := s.createTestWorkflow(agentID, "Inactive-"+uuid.NewString(), "Inactive")

	// Change statuses of the non-DRAFT workflows
	s.deployAgentViaAPI(agentID)
	s.publishWorkflowViaAPI(publishedID)
	s.deactivateWorkflowViaAPI(inactiveID)

	// Filter by DRAFT status only
	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{
		Statuses: []pb.WorkflowStatus{pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT},
	})
	s.Require().NoError(err)

	assertAllStatuses(s, resp.GetWorkflows(), pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT)

	ids := collectWorkflowIDs(resp.GetWorkflows())

	// Verify DRAFT workflow IS in the results
	s.Contains(ids, draftID, "DRAFT workflow should be in filtered list")

	// Verify non-DRAFT workflows are NOT in the results
	s.NotContains(ids, publishedID, "PUBLISHED workflow should NOT be in DRAFT filtered list")
	s.NotContains(ids, inactiveID, "INACTIVE workflow should NOT be in DRAFT filtered list")
}

func (s *WorkflowServiceSuite) TestListWorkflows_Filter_MultipleStatuses() {
	agentID := s.createAgentViaAPI()

	w1 := s.createTestWorkflow(agentID, "Draft "+uuid.NewString(), "Draft")
	w2 := s.createTestWorkflow(agentID, "Published "+uuid.NewString(), "Published")
	w3 := s.createTestWorkflow(agentID, "Inactive "+uuid.NewString(), "Inactive")

	s.deployAgentViaAPI(agentID)
	s.publishWorkflowViaAPI(w2)
	s.deactivateWorkflowViaAPI(w3)

	resp, err := s.listWorkflows(&pb.ListWorkflowsRequest{
		Statuses: []pb.WorkflowStatus{
			pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT,
			pb.WorkflowStatus_WORKFLOW_STATUS_PUBLISHED,
		},
	})
	s.Require().NoError(err)

	assertAllStatuses(
		s,
		resp.GetWorkflows(),
		pb.WorkflowStatus_WORKFLOW_STATUS_DRAFT,
		pb.WorkflowStatus_WORKFLOW_STATUS_PUBLISHED,
	)

	ids := collectWorkflowIDs(resp.GetWorkflows())
	s.Contains(ids, w1)
	s.Contains(ids, w2)
}

func (s *WorkflowServiceSuite) listWorkflows(
	req *pb.ListWorkflowsRequest,
) (*pb.ListWorkflowsResponse, error) {
	s.T().Helper()

	ctx := s.T().Context()
	cr := connect.NewRequest(req)
	cr.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.ListWorkflows(ctx, cr)
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func assertAllStatuses(
	s *WorkflowServiceSuite,
	workflows []*pb.ListWorkFlowResponseItem,
	allowed ...pb.WorkflowStatus,
) {
	s.T().Helper()

	for _, w := range workflows {
		s.Contains(allowed, w.GetStatus(),
			"unexpected workflow status: %v", w.GetStatus())
	}
}

func collectWorkflowIDs(workflows []*pb.ListWorkFlowResponseItem) map[string]struct{} {
	ids := make(map[string]struct{}, len(workflows))
	for _, w := range workflows {
		ids[w.GetId()] = struct{}{}
	}
	return ids
}

// Verifies that ListWorkflows with zero limit uses the default limit of 50.
func (s *WorkflowServiceSuite) TestListWorkflows_ZeroLimit_UsesDefault() {
	agentID := s.createAgentViaAPI()

	// Create 55 workflows to exceed the default limit of 50
	for range 55 {
		s.createTestWorkflow(agentID, "ZeroLimit-"+uuid.NewString(), "Test")
	}

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListWorkflowsRequest{Limit: 0, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.ListWorkflows(ctx, req)
	s.Require().NoError(err, "Zero limit should succeed using default")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetWorkflows()), 50, "Zero limit should use default of 50")
}

// Verifies that ListWorkflows with negative offset is clamped to 0.
func (s *WorkflowServiceSuite) TestListWorkflows_NegativeOffset_ClampedToZero() {
	agentID := s.createAgentViaAPI()

	// Create multiple workflows to ensure we have data
	createdIDs := make([]string, 5)
	for i := range 5 {
		createdIDs[i] = s.createTestWorkflow(agentID, "NegOffset-"+uuid.NewString(), "Test")
	}

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListWorkflowsRequest{Limit: 10, Offset: -10})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.ListWorkflows(ctx, req)
	s.Require().NoError(err, "Negative offset should be clamped to 0 and succeed")
	s.Require().NotNil(resp)

	// Should return results from the beginning (offset clamped to 0)
	ids := collectWorkflowIDs(resp.Msg.GetWorkflows())
	for _, id := range createdIDs {
		s.Contains(ids, id, "Negative offset should be clamped to 0 and include created workflows")
	}
}

// Verifies that ListWorkflows with negative limit uses the default limit of 50.
func (s *WorkflowServiceSuite) TestListWorkflows_NegativeLimit_UsesDefault() {
	agentID := s.createAgentViaAPI()

	// Create 55 workflows to exceed the default limit of 50
	for range 55 {
		s.createTestWorkflow(agentID, "NegLimit-"+uuid.NewString(), "Test")
	}

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListWorkflowsRequest{Limit: -10, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.ListWorkflows(ctx, req)
	s.Require().NoError(err, "Negative limit should succeed using default")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetWorkflows()), 50, "Negative limit should use default of 50")
}

// Verifies that ListWorkflows with high limit is clamped to max of 50.
func (s *WorkflowServiceSuite) TestListWorkflows_HighLimit_ClampedToMax() {
	agentID := s.createAgentViaAPI()

	// Create 55 workflows to exceed the max limit of 50
	for range 55 {
		s.createTestWorkflow(agentID, "HighLimit-"+uuid.NewString(), "Test")
	}

	ctx := s.T().Context()
	req := connect.NewRequest(&pb.ListWorkflowsRequest{Limit: 9999, Offset: 0})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.workflowClient.ListWorkflows(ctx, req)
	s.Require().NoError(err, "High limit should be clamped to 50 and succeed")
	s.Require().NotNil(resp)
	s.LessOrEqual(len(resp.Msg.GetWorkflows()), 50, "High limit should be clamped to max 50")
}
