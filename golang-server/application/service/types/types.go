package types

import (
	"context"
	"io"
	"time"

	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type CallHandlerService interface {
	CreateSession(ctx context.Context, agentId string) (string, error)

	// HandleWSConnection takes *websocket.Conn to handle Call Connection.
	// This is a blocking call
	HandleWSConnection(ctx context.Context, conn *websocket.Conn, agentId string, sessionId string) error
}

type MessageRole int

const (
	MessageRoleUser MessageRole = iota
	MessageRoleSystem
)

type AgentMessage struct {
	Role    MessageRole
	Content string
}

type AgentService interface {
	ValidateAgent(ctx context.Context, agentId string) error
	DisconnectAgent(ctx context.Context, agentId string, sessionId string) error
	Invoke(ctx context.Context, agentID string, sessionId string, messages []AgentMessage) (AgentResponse, error)
	UpdateContext(ctx context.Context, agentID, sessionID, contextText string) error
}

type AgentLanguage string

const (
	AgentLanguageEnglish AgentLanguage = "en"
	AgentLanguageHindi   AgentLanguage = "hi"
)

type AgentResponse struct {
	Message          string
	Language         AgentLanguage
	CutCall          bool
	Cut_call_message string
}

// TODO: Since domain of AgentService and DeploymentService are same (Agent), we can use a single service for both.
// TODO: Merge AgentService and DeploymentService into a single service.
type DeploymentService interface {
	CreateAgent(ctx context.Context, request *pb.CreateAgentRequest) (entity.Agent, error)
	GetAgent(ctx context.Context, agentId string) (entity.Agent, error)
	ListAgents(ctx context.Context, statuses []entity.DeploymentStatus, limit int32, offset int32) ([]entity.Agent, int32, error)
	DeployAgent(ctx context.Context, agentId string) error
	DeploymentStatusPoller(ctx context.Context) error
	UpdateAgent(ctx context.Context, agentId string, agentName string, request *pb.UpdateAgentRequest) error
}

// TODO: Merge AgentService and DeploymentService into a single service.
type DeploymentAgentStatsService interface {
	GetAgent(ctx context.Context, agentId string) (entity.Agent, error)
}

type KnowledgeBaseService interface {
	UploadDocument(ctx context.Context, file io.Reader, documentType string, documentName string) error
	ListDocuments(ctx context.Context, limit, offset int32) ([]entity.Document, error)
	DeleteDocument(ctx context.Context, documentId string) error

	ChangeDocumentStatus(ctx context.Context, documentId string, isActive bool) error
	GetDocumentById(ctx context.Context, documentId string) (entity.Document, error)
	UpdateDocument(ctx context.Context, document entity.Document) error
}

type UserManagementService interface {
	RegisterUser(ctx context.Context, userInfo entity.User) error
	GetUserTenantID(ctx context.Context, userID string) (uuid.UUID, error)
}

type OutboundCallService interface {
	UploadOutboundCallDocument(ctx context.Context, filename string, fileType string, fileContent []byte) (string, error)
	ListOutboundCallDocuments(ctx context.Context, limit int32, offset int32) ([]entity.OutboundCallDocument, error)
	GetOutboundCallDocument(ctx context.Context, documentId string, tenantID uuid.UUID) (entity.OutboundCallDocument, error)
	DeleteOutboundCallDocument(ctx context.Context, documentId string, tenantID uuid.UUID) (string, error)
	CreateCallJob(ctx context.Context, callJob entity.CallJob) (entity.CallJob, error)
	GetCallJob(ctx context.Context, id uuid.UUID) (entity.CallJob, error)
	ListCallJobs(ctx context.Context, statuses []entity.CallJobStatus, workflowId uuid.UUID, filterDateBegin time.Time, filterDateEnd time.Time, limit int32, offset int32) ([]entity.CallJob, int32, error) // calljobs, totalCount, error
	DeleteCallJob(ctx context.Context, id uuid.UUID) error
	MaterialiseJob(ctx context.Context, jobid uuid.UUID) error
	TriggerJob(ctx context.Context, jobId uuid.UUID) error
	ListJobItems(ctx context.Context, jobId uuid.UUID, limit int32, offset int32) ([]entity.JobItem, int32, error) // jobItems, totalCount, error
	HandleWSConnection(ctx context.Context, conn *websocket.Conn, agentID, sessionID, jobItemID string) error
	ProcessCallStatusWebhook(ctx context.Context, callSid string, jobItemId uuid.UUID, sessionId string, callStatus string, callDuration string, timestamp string, errorMessage string) error
	CopyCallJob(ctx context.Context, sourceJobID string, name string, providerId uuid.UUID, workflowId uuid.UUID, preferedLanguage string, jobItemIDs []string) (string, int32, error) // newCalljobId, copiedCount, error
	UpdateCallJobDetails(ctx context.Context, jobId string, name string, workflowId uuid.UUID) error
	AddJobItems(ctx context.Context, jobId string, items []entity.NewJobItem) error
	RemoveJobItems(ctx context.Context, jobId string, jobItemIds []string) error
	GetCallDetails(ctx context.Context, callSid string, jobItemId uuid.UUID) (entity.CallDetails, error)
	GetAudioSampleRateForAgent(ctx context.Context, agentID string) (int, error)
}

type TranscriptionStorageService interface {
	TestPushSQS(ctx context.Context, event *pb.TranscriptEvent) error
	StartIngestWorker(ctx context.Context) error
	GetTranscriptionMetadata(ctx context.Context, sessionID string) (entity.TranscriptionMetadata, error)
	GetSessionKVData(ctx context.Context, sessionID string) ([]entity.AgentKVItem, error)
}

type CredentialService interface {
	SetCredential(ctx context.Context, credentialName string, credential entity.Credential, credentialType entity.CredentialType) (entity.CredentialEncrypted, error)
	GetCredential(ctx context.Context, credentialId uuid.UUID) (entity.CredentialEncrypted, error)
	GetDecryptedCredential(ctx context.Context, credentialId uuid.UUID) (entity.Credential, error)
	DeleteCredential(ctx context.Context, credentialId uuid.UUID) error
	ListCredentials(ctx context.Context, credentialType []entity.CredentialType, limit int32, offset int32) ([]entity.CredentialEncrypted, error)
}

type WorkflowWithCallJobCount struct {
	ActiveCallJobCount int32 `json:"activeCallJobCount"`
	TotalCallJobCount  int32 `json:"totalCallJobCount"`
}

type WorkflowService interface {
	CreateWorkflow(ctx context.Context, workflow entity.Workflow) (entity.Workflow, error)
	GetWorkflow(ctx context.Context, workflowId uuid.UUID) (entity.Workflow, error)
	INTERNALGetWorkflow(ctx context.Context, workflowId uuid.UUID) (entity.Workflow, error)
	UpdateWorkflow(ctx context.Context, workflowId uuid.UUID, name string, description string, agentId string) error
	PublishWorkflow(ctx context.Context, workflowId uuid.UUID) error
	DeactivateWorkflow(ctx context.Context, workflowId uuid.UUID) error
	DeleteWorkflow(ctx context.Context, workflowId uuid.UUID) error
	ListWorkflows(ctx context.Context, statuses []entity.WorkflowStatus, limit int32, offset int32) ([]entity.Workflow, int32, error)
	GetWorkflowWithCallJobCount(ctx context.Context, workflowId uuid.UUID) (WorkflowWithCallJobCount, error)
}

type WorkflowStatsService interface {
	HasPublishedWorkflowsForAgent(ctx context.Context, agentId string) (bool, error)
}

// Audit resource types.
const (
	AuditResourceTypeAgent                = "agent"
	AuditResourceTypeDocument             = "document"
	AuditResourceTypeCredential           = "credential"
	AuditResourceTypeOutboundCallDocument = "outbound_call_document"
	AuditResourceTypeCallJob              = "call_job"
	AuditResourceTypeUser                 = "user"
)

// Audit event actions.
const (
	AuditActionAgentCreated  = "agent.created"
	AuditActionAgentUpdated  = "agent.updated"
	AuditActionAgentDeployed = "agent.deployed"

	AuditActionDocumentCreated       = "document.created"
	AuditActionDocumentDeleted       = "document.deleted"
	AuditActionDocumentStatusChanged = "document.status_changed"
	AuditActionDocumentUpdated       = "document.updated"

	AuditActionCredentialCreated = "credential.created" //nolint:gosec
	AuditActionCredentialDeleted = "credential.deleted" //nolint:gosec

	AuditActionOutboundCallDocCreated = "outbound_call_document.created"
	AuditActionOutboundCallDocDeleted = "outbound_call_document.deleted"

	AuditActionCallJobCreated      = "call_job.created"
	AuditActionCallJobDeleted      = "call_job.deleted"
	AuditActionCallJobCopied       = "call_job.copied"
	AuditActionCallJobUpdated      = "call_job.updated"
	AuditActionCallJobTriggered    = "call_job.triggered"
	AuditActionCallJobItemsAdded   = "call_job.items_added"
	AuditActionCallJobItemsRemoved = "call_job.items_removed"

	AuditActionUserCreated = "user.created"
)

type AuditEvent struct {
	Action       string
	ResourceType string
	ResourceID   string
}

type AuditService interface {
	Log(ctx context.Context, event AuditEvent) error
	ListAuditLogs(ctx context.Context, query entity.AuditLogQuery) ([]entity.AuditLog, error)
}
