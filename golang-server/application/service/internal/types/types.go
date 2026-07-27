package types

import (
	"context"
	"io"
	"time"

	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"

	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"
)

type STTService interface {
	Connect(ctx context.Context, encoding pb.AudioEncoding, sampleRate int) (STTClient, error)
}

type STTClient interface {
	SendAudio(ctx context.Context, audio []byte) error
	Responses() <-chan string
	Disconnect(ctx context.Context)
}

type TTSHandler interface {
	GenerateAudio(ctx context.Context, message string, encoding pb.AudioEncoding, language types.AgentLanguage, sampleRate int) ([]byte, error)
}

// ObjectStore interface is used for connecting to any Object Store. S3, GCS etc.
type ObjectStore interface {
	// PutObject uploads the objectBody to the bucket with objectKey and contentType.
	// Returns the URL of the object.
	PutObject(ctx context.Context, objectBody io.Reader, bucket, objectKey, contentType string) (string, error)
	GetObject(ctx context.Context, bucket string, objectKey string) (io.ReadCloser, error)
	GetSignedURL(ctx context.Context, bucket, objectKey string, expires time.Duration) (string, error)
}

type EncryptionClient interface {
	Encrypt(ctx context.Context, data []byte) (*EncryptedData, error)
	Decrypt(ctx context.Context, encryptedData *EncryptedData) ([]byte, error)
}

type MessageQueue interface {
	PublishBytesMessage(ctx context.Context, message []byte) error
	PublishStringMessage(ctx context.Context, message string) error
}

// For producer.
type SQSMessageQueue interface {
	PublishStringMessage(ctx context.Context, message string, messageGroupID string) error
}

type MessageConsumer interface {
	ConsumeStringMessage(ctx context.Context, callback func(ctx context.Context, message string) error) error
}

// For consumer.
type SQSMessageConsumer interface {
	ConsumeStringMessage(ctx context.Context, callback func(ctx context.Context, message sqsTypes.Message) (bool, error)) error
}

// FIFO Queue interfaces for ordered message processing.
type FIFOMessageQueue interface {
	PublishStringMessage(ctx context.Context, message string, messageGroupID string, deduplicationID string) error
}

type FIFOMessageConsumer interface {
	ConsumeStringMessage(ctx context.Context, callback func(ctx context.Context, message string, messageGroupID string) error) error
}

type TranscriptionRepository interface {
	InsertSegment(ctx context.Context, event *pb.TranscriptEvent, segment *pb.Segment) error
	ClaimSession(ctx context.Context, sessionID string) error
	GetSessionMeta(ctx context.Context, sessionID string) (*entity.MetadataDynamo, error)
	FinalizeSession(ctx context.Context, sessionID, s3Key, status string, duration float64) error
	FetchSessionItems(ctx context.Context, sessionID string) (entity.MetaItem, []entity.SegmentItem, error)
	Query(ctx context.Context, status string, olderThan time.Time) ([]string, error)
}

type AgentKVRepository interface {
	QuerySessionKV(ctx context.Context, sessionID string, maxItems int32) ([]entity.AgentKVItem, error)
}

type KVMessageQueue interface {
	PublishStringMessage(ctx context.Context, msg string) error
}

type KnoledgeBaseRepository interface {
	InsertDocument(ctx context.Context, document entity.Document) (entity.Document, error)
	ListDocuments(ctx context.Context, tenantID uuid.UUID, limit int32, offset int32) ([]entity.Document, error)
	GetDocumentById(ctx context.Context, tenantID uuid.UUID, documentId string) (entity.Document, error)
	UpdateDocument(ctx context.Context, document entity.Document) error
}

type AgentRepository interface {
	InsertAgent(ctx context.Context, agent entity.Agent) (entity.Agent, error)
	GetAgent(ctx context.Context, agentId string, tenantID uuid.UUID) (entity.Agent, error)
	ListAgents(ctx context.Context, tenantID uuid.UUID, statuses []entity.DeploymentStatus, limit int32, offset int32) ([]entity.Agent, error)
	INTERNALGetAgent(ctx context.Context, agentID string) (entity.Agent, error)
	// TODO: This method is used for both internal and external use so tennant id cannot be added for now.
	// TODO: Create a separate method for internal use, and add tenant id to the method.
	UpdateDeploymentStatus(ctx context.Context, agentId string, deploymentStatus entity.DeploymentStatus, updatedAt time.Time) error
	UpdateAgent(ctx context.Context, agent entity.Agent) error
	GetAgentCount(ctx context.Context, statuses []entity.DeploymentStatus, tenantID uuid.UUID) (int32, error)
}

type UserManagementRepository interface {
	RegisterUser(ctx context.Context, user entity.User) (string, error)
	CreateTenant(ctx context.Context, tenant entity.Tenant) (entity.Tenant, error)
	GetUserTenantID(ctx context.Context, userID string) (uuid.UUID, error)
}

type TransationHandler interface {
	WithTransaction(ctx context.Context, callback func(ctx context.Context) error) error
}

type OutboundCallDocumentRepository interface {
	UploadOutboundCallDocumentRepository(ctx context.Context, document entity.OutboundCallDocument) (entity.OutboundCallDocument, error)
	ListOutboundCallDocumentsRepository(ctx context.Context, tenantID uuid.UUID, limit int32, offset int32) ([]entity.OutboundCallDocument, error)
	GetOutboundCallDocumentRepository(ctx context.Context, id string, tenantID uuid.UUID) (entity.OutboundCallDocument, error)
	DeleteOutboundCallDocumentRepository(ctx context.Context, id string, tenantID uuid.UUID) error
}

type CallJobRepository interface {
	CreateCallJob(ctx context.Context, callJob entity.CallJob) error
	GetCallJob(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) (entity.CallJob, error)
	ListCallJobs(ctx context.Context, statuses []entity.CallJobStatus, workflowId uuid.UUID, filterDateBegin time.Time, filterDateEnd time.Time, limit int32, offset int32, tenantID uuid.UUID) ([]entity.CallJob, error)
	DeleteCallJob(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error
	UpdateCallJob(ctx context.Context, callJob entity.CallJob) error
	UpdateCallJobStatus(ctx context.Context, jobId uuid.UUID, status entity.CallJobStatus, tenantID uuid.UUID) error
	InsertJobItem(ctx context.Context, row entity.JobItem) error
	INTERNALGetJobItem(ctx context.Context, jobItemId uuid.UUID) (entity.JobItem, error)
	GetJobItems(ctx context.Context, jobId uuid.UUID, limit int32, offset int32, tenantID uuid.UUID) ([]entity.JobItem, error)
	UpdateJobItem(ctx context.Context, row entity.JobItem) error
	UpdateJobItemCallStatus(ctx context.Context, jobItemId uuid.UUID, statusUpdate entity.CallJobItemStatus) error
	GetJobItemTenantById(ctx context.Context, jobItemId uuid.UUID) (uuid.UUID, error)
	GetJobItemsByIDs(ctx context.Context, jobID uuid.UUID, itemIDs []uuid.UUID, tenantID uuid.UUID) ([]entity.JobItem, error)
	DeleteJobItem(ctx context.Context, id uuid.UUID, jobId uuid.UUID, tenantID uuid.UUID) error
	GetCallJobCount(ctx context.Context, statuses []entity.CallJobStatus, workflowId uuid.UUID, filterDateBegin time.Time, filterDateEnd time.Time, tenantID uuid.UUID) (int32, error)
	GetJobItemsCount(ctx context.Context, jobID uuid.UUID, tenantID uuid.UUID) (int32, error)
}

type RealtimeAgentService interface {
	Connect(ctx context.Context, agentID string, sessionID string, instructions string, language types.AgentLanguage, opts ...RealtimeConnectOption) (RealtimeAgentClient, error)
}

type RealtimeAgentClient interface {
	// SendAudio sends a chunk of audio (from the telephony provider) to the model.
	SendAudio(ctx context.Context, audioChunk []byte) error

	// AudioOutput returns a channel that streams audio responses from the model.
	AudioOutput() <-chan []byte

	// Events returns a channel for control events (e.g., transcriptions, call termination).
	Events() <-chan AgentEvent

	UpdateLanguage(ctx context.Context, language types.AgentLanguage) error

	InterruptResponse(ctx context.Context) error

	SubmitFunctionResult(ctx context.Context, callId string, output string) error
	// Close terminates the session and cleans up resources.
	Close() error
}

type AgentEvent struct {
	Type    entity.RealtimeEventType // Event type: "transcription", "end_call", "transfer", "error"
	Payload any                      // Event-specific data
}

type AudioConfig struct {
	InputSampleRate  int
	OutputSampleRate int
	AudioFormat      string
}

type RealtimeConnectOption func(*RealtimeConnectOptions)

type RealtimeConnectOptions struct {
	AudioConfig AudioConfig
}

type CredentialRepository interface {
	CreateCredential(ctx context.Context, credentialEncrypted entity.CredentialEncrypted) (entity.CredentialEncrypted, error)
	UpdateCredential(ctx context.Context, credentialEncrypted entity.CredentialEncrypted) (entity.CredentialEncrypted, error)
	GetCredentialEncrypted(ctx context.Context, credentialID uuid.UUID, tenantID uuid.UUID) (entity.CredentialEncrypted, error)
	INTERNALgetCredential(ctx context.Context, credentialID uuid.UUID) (entity.CredentialEncrypted, error)
	DeleteCredential(ctx context.Context, credentialID uuid.UUID, tenantID uuid.UUID) error
	ListCredentials(ctx context.Context, types []entity.CredentialType, limit int32, offset int32, tenantID uuid.UUID) ([]entity.CredentialEncrypted, error)
}

// EncryptedData holds the result of encryption.
type EncryptedData struct {
	EncryptedPayload []byte
	EncryptedDataKey []byte
	IV               []byte
}

type WorkflowRepository interface {
	CreateWorkflow(ctx context.Context, workflow entity.Workflow) error
	GetWorkflow(ctx context.Context, workflowId uuid.UUID, tenantID uuid.UUID) (entity.Workflow, error)
	INTERNALGetWorkflow(ctx context.Context, workflowId uuid.UUID) (entity.Workflow, error)
	UpdateWorkflow(ctx context.Context, workflow entity.Workflow, tenantID uuid.UUID) error
	DeleteWorkflow(ctx context.Context, id uuid.UUID, tenantID uuid.UUID) error
	ListWorkflows(ctx context.Context, statuses []entity.WorkflowStatus, limit int32, offset int32, tenantID uuid.UUID) ([]entity.Workflow, error)
	GetWorkflowCallJobCount(ctx context.Context, workflowId uuid.UUID, tenantID uuid.UUID) (int32, error)
	GetWorkflowCallJobCountByStatuses(ctx context.Context, workflowId uuid.UUID, statuses []entity.CallJobStatus, tenantID uuid.UUID) (int32, error)
	HasPublishedWorkflowsForAgent(ctx context.Context, agentId string, tenantID uuid.UUID) (bool, error)
	GetWorkflowCount(ctx context.Context, statuses []entity.WorkflowStatus, tenantID uuid.UUID) (int32, error)
}

type AuditRepository interface {
	InsertAuditLog(ctx context.Context, log entity.AuditLog) error
	ListAuditLogs(ctx context.Context, tenantID uuid.UUID, params entity.AuditLogQuery) ([]entity.AuditLog, error)
}
