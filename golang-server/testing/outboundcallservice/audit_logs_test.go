package outboundcallservice

import (
	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	servicetypes "exiro.ai/application/service/types"
	"github.com/google/uuid"
)


func (s *OutboundCallServiceSuite) TestCreateCallJob_WritesAuditLog() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.Name = "audit-create-" + uuid.NewString()

	jobID := s.createCallJobViaAPI(&reqData)
	s.Require().NotEmpty(jobID)

	ctx := s.T().Context()
	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCallJobCreated,
		servicetypes.AuditResourceTypeCallJob,
		jobID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCallJobCreated, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCallJob, log.GetResourceType())
	s.Equal(jobID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestDeleteCallJob_WritesAuditLog() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.Name = "audit-delete-" + uuid.NewString()

	jobID := s.createCallJobViaAPI(&reqData)
	s.Require().NotEmpty(jobID)

	ctx := s.T().Context()
	delReq := connect.NewRequest(&pb.DeleteCallJobRequest{Id: jobID})
	delReq.Header().Set("Authorization", "Bearer test-token")
	delResp, err := s.outboundClient.DeleteCallJob(ctx, delReq)
	s.Require().NoError(err)
	s.Require().NotNil(delResp)
	s.True(delResp.Msg.GetStatus())

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCallJobDeleted,
		servicetypes.AuditResourceTypeCallJob,
		jobID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCallJobDeleted, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCallJob, log.GetResourceType())
	s.Equal(jobID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestUpdateCallJobDetails_WritesAuditLog() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.Name = "audit-" + uuid.NewString()

	jobID := s.createCallJobViaAPI(&reqData)
	s.Require().NotEmpty(jobID)

	ctx := s.T().Context()
	updateReq := connect.NewRequest(&pb.UpdateCallJobDetailsRequest{
		JobId:      jobID,
		Name:       "audit-updated-" + uuid.NewString(),
		WorkflowId: reqData.GetWorkflowId(),
	})
	updateReq.Header().Set("Authorization", "Bearer test-token")
	_, err := s.outboundClient.UpdateCallJobDetails(ctx, updateReq)
	s.Require().NoError(err)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCallJobUpdated,
		servicetypes.AuditResourceTypeCallJob,
		jobID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCallJobUpdated, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCallJob, log.GetResourceType())
	s.Equal(jobID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestCopyCallJob_WritesAuditLog() {
	var reqData pb.CreateCallJobRequest
	s.LoadTestData(serviceName, "request/create_call_job.json", &reqData)
	reqData.Name = "audit-copy-source-" + uuid.NewString()

	sourceJobID := s.createCallJobViaAPI(&reqData)
	s.Require().NotEmpty(sourceJobID)

	ctx := s.T().Context()
	itemIDs := s.insertJobItems(ctx, sourceJobID, 2)
	s.Require().Len(itemIDs, 2)

	copyReq := connect.NewRequest(&pb.CopyCallJobRequest{
		SourceJobId:            sourceJobID,
		Name:                   "audit-copied-job-" + uuid.NewString(),
		WorkflowId:             reqData.GetWorkflowId(),
		OutboundCallProviderId: uuid.NewString(),
		PreferedLanguage:       "en",
		JobItemIds:             itemIDs,
	})
	copyReq.Header().Set("Authorization", "Bearer test-token")
	copyResp, err := s.outboundClient.CopyCallJob(ctx, copyReq)
	s.Require().NoError(err)
	s.Require().NotNil(copyResp)

	newJobID := copyResp.Msg.GetNewJobId()
	s.Require().NotEmpty(newJobID)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCallJobCopied,
		servicetypes.AuditResourceTypeCallJob,
		newJobID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCallJobCopied, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCallJob, log.GetResourceType())
	s.Equal(newJobID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestAddJobItems_WritesAuditLog() {
	ctx := s.T().Context()
	jobID := s.insertCallJobWithStatus(ctx, "ready", "audit-add-items-"+uuid.NewString())

	addReq := connect.NewRequest(&pb.AddJobItemsRequest{
		JobId: jobID,
		Items: []*pb.NewJobItem{
			{PhoneNumber: "+19876543210", AgentContext: "audit test context"},
		},
	})
	addReq.Header().Set("Authorization", "Bearer test-token")
	_, err := s.outboundClient.AddJobItems(ctx, addReq)
	s.Require().NoError(err)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCallJobItemsAdded,
		servicetypes.AuditResourceTypeCallJob,
		jobID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCallJobItemsAdded, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCallJob, log.GetResourceType())
	s.Equal(jobID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestRemoveJobItems_WritesAuditLog() {
	ctx := s.T().Context()
	jobID := s.insertCallJobWithStatus(ctx, "ready", "audit-remove-items-"+uuid.NewString())
	itemIDs := s.insertJobItems(ctx, jobID, 1)

	removeReq := connect.NewRequest(&pb.RemoveJobItemsRequest{
		JobId:      jobID,
		JobItemIds: itemIDs,
	})
	removeReq.Header().Set("Authorization", "Bearer test-token")
	_, err := s.outboundClient.RemoveJobItems(ctx, removeReq)
	s.Require().NoError(err)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCallJobItemsRemoved,
		servicetypes.AuditResourceTypeCallJob,
		jobID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCallJobItemsRemoved, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCallJob, log.GetResourceType())
	s.Equal(jobID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestTriggerJob_WritesAuditLog() {
	ctx := s.T().Context()
	jobID := s.insertCallJobWithStatus(ctx, "ready", "audit-trigger-"+uuid.NewString())

	triggerReq := connect.NewRequest(&pb.TriggerJobRequest{JobId: jobID})
	triggerReq.Header().Set("Authorization", "Bearer test-token")
	_, _ = s.outboundClient.TriggerJob(ctx, triggerReq)

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionCallJobTriggered,
		servicetypes.AuditResourceTypeCallJob,
		jobID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionCallJobTriggered, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeCallJob, log.GetResourceType())
	s.Equal(jobID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestUploadOutboundCallDocument_WritesAuditLog() {
	ctx := s.T().Context()
	docName := "audit-upload-" + uuid.NewString() + ".csv"
	
	req := connect.NewRequest(&pb.UploadOutboundCallDocumentRequest{
		FileName:    docName,
		FileType:    "csv",
		FileContent: []byte("phone_number,agent_context\n+1234567890,test context"),
	})
	req.Header().Set("Authorization", "Bearer test-token")

	resp, err := s.outboundClient.UploadOutboundCallDocument(ctx, req)
	s.Require().NoError(err, "UploadOutboundCallDocument should succeed")
	s.Require().NotNil(resp)
	docID := resp.Msg.GetDocumentId()
	s.Require().NotEmpty(docID, "Document ID should not be empty")

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionOutboundCallDocCreated,
		servicetypes.AuditResourceTypeOutboundCallDocument,
		docID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionOutboundCallDocCreated, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeOutboundCallDocument, log.GetResourceType())
	s.Equal(docID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}

func (s *OutboundCallServiceSuite) TestDeleteOutboundCallDocument_WritesAuditLog() {
	ctx := s.T().Context()
	docName := "audit-delete-doc-" + uuid.NewString() + ".csv"
	
	uploadReq := connect.NewRequest(&pb.UploadOutboundCallDocumentRequest{
		FileName:    docName,
		FileType:    "csv",
		FileContent: []byte("phone_number,agent_context\n+1234567890,test context"),
	})
	uploadReq.Header().Set("Authorization", "Bearer test-token")

	uploadResp, err := s.outboundClient.UploadOutboundCallDocument(ctx, uploadReq)
	s.Require().NoError(err, "UploadOutboundCallDocument should succeed")
	s.Require().NotNil(uploadResp)
	docID := uploadResp.Msg.GetDocumentId()
	s.Require().NotEmpty(docID, "Document ID should not be empty")

	delReq := connect.NewRequest(&pb.DeleteOutboundCallDocumentRequest{DocumentId: docID})
	delReq.Header().Set("Authorization", "Bearer test-token")
	
	delResp, err := s.outboundClient.DeleteOutboundCallDocument(ctx, delReq)
	s.Require().NoError(err)
	s.Require().NotNil(delResp)
	s.NotEmpty(delResp.Msg.GetMessage())

	log := s.getLatestAuditLog(
		ctx,
		servicetypes.AuditActionOutboundCallDocDeleted,
		servicetypes.AuditResourceTypeOutboundCallDocument,
		docID,
	)
	s.Require().NotNil(log)
	s.Equal(servicetypes.AuditActionOutboundCallDocDeleted, log.GetAction())
	s.Equal(servicetypes.AuditResourceTypeOutboundCallDocument, log.GetResourceType())
	s.Equal(docID, log.GetResourceId())
	s.Equal(s.TestUserID, log.GetUserId())
	s.Equal(s.TestTenantID.String(), log.GetTenantId())
}
