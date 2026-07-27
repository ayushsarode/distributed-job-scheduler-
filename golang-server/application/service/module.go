package service

import (
	"context"

	"exiro.ai/application/assert"
	"exiro.ai/application/service/agentservice"
	"exiro.ai/application/service/auditservice"
	"exiro.ai/application/service/callhandler"
	"exiro.ai/application/service/credentialservice"
	"exiro.ai/application/service/deploymentservice"
	deploymentstatsservice "exiro.ai/application/service/deploymentservice/stats"
	"exiro.ai/application/service/internal/awskms"
	"exiro.ai/application/service/internal/messagequeue"
	"exiro.ai/application/service/internal/objectstore"
	"exiro.ai/application/service/internal/realtime"
	"exiro.ai/application/service/internal/repository"
	"exiro.ai/application/service/internal/repository/postgres/transaction"
	stt "exiro.ai/application/service/internal/stt"
	"exiro.ai/application/service/internal/tts"
	"exiro.ai/application/service/knowledgebase"
	"exiro.ai/application/service/outboundcallservice"
	"exiro.ai/application/service/transcriptionstorage"
	servicetypes "exiro.ai/application/service/types"
	usermanagement "exiro.ai/application/service/usermanagementservice"
	workflowservice "exiro.ai/application/service/workflowservice"
	workflowstatsservice "exiro.ai/application/service/workflowservice/stats"
	"exiro.ai/config"
	"github.com/sourcegraph/conc/pool"
)

const maxGoroutines = 10

func NewCallHandlerService(ctx context.Context) servicetypes.CallHandlerService {
	return callhandler.NewService(
		ctx,
		stt.NewSTTService(ctx),
		tts.NewTTSHandler(ctx),
		agentservice.NewAgentService(ctx, repository.NewAgentRepository(ctx)),
	)
}

func NewDeploymentService(ctx context.Context) servicetypes.DeploymentService {
	WorkflowStatsService := NewWorkflowStatsService(ctx)

	return deploymentservice.NewDeployService(ctx,
		repository.NewAgentRepository(ctx),
		messagequeue.NewAgentDeploymentMessageQueue(ctx),
		messagequeue.NewAgentDeploymentStatusConsumer(ctx),
		repository.TransactionHandler(ctx),
		WorkflowStatsService,
		NewAuditService(ctx),
	)
}

func NewKnowledgeBaseService(ctx context.Context) servicetypes.KnowledgeBaseService {
	objectStore, err := objectstore.NewObjectStore(ctx, config.Ctx(ctx).DocumentsBucket)
	assert.NoError(err)
	return knowledgebase.NewService(ctx, objectStore, repository.NewKnowledgeBaseRepository(ctx), NewAuditService(ctx), repository.TransactionHandler(ctx))
}

func NewUserManagementService(ctx context.Context) servicetypes.UserManagementService {
	return usermanagement.NewService(
		ctx,
		repository.NewUserManagementRepository(ctx),
		transaction.NewTransactionHandler(ctx),
		NewAuditService(ctx),
	)
}

func NewOutboundCallService(ctx context.Context) servicetypes.OutboundCallService {
	// Check if outbound calling is enabled
	if !config.Ctx(ctx).OutboundCalling.Enabled {
		return &outboundcallservice.DisabledOutboundCallService{}
	}

	outboundCallConfig := config.Ctx(ctx).OutboundCalling
	providerConfig := outboundcallservice.NewProviderConfig(ctx)
	objectStore, err := objectstore.NewObjectStore(ctx, config.Ctx(ctx).OutboundCalling.CampaignDocumentBucket)
	jobPool := pool.New().WithMaxGoroutines(maxGoroutines)
	assert.NoError(err)

	realtimeService, err := realtime.NewRealtimeService(ctx)
	assert.NoError(err)

	workflowService := NewWorkflowManagementService(ctx)

	return outboundcallservice.NewService(
		ctx,
		objectStore,
		repository.NewOutboundCallDocumentRepository(ctx),
		repository.NewOutBoundCallJobRepository(ctx),
		jobPool,
		repository.TransactionHandler(ctx),
		repository.NewAgentRepository(ctx), // TODO: use agent service instead here
		stt.NewSTTService(ctx),
		realtimeService,
		tts.NewTTSHandler(ctx),
		agentservice.NewAgentService(ctx, repository.NewAgentRepository(ctx)),
		messagequeue.NewTranscriptionFIFOMessageQueue(ctx),
		providerConfig,
		NewCredentialService(ctx),
		workflowService,
		outboundCallConfig,
		NewAuditService(ctx),
	)
}

func NewTranscriptionService(ctx context.Context) servicetypes.TranscriptionStorageService {
	objectStore, err := objectstore.NewObjectStore(ctx, config.Ctx(ctx).TranscriptionStorage.S3BucketName)
	assert.NoError(err)

	svc := transcriptionstorage.New(
		ctx,
		messagequeue.NewTranscriptionFIFOMessageQueue(ctx),
		messagequeue.NewTranscriptionFIFOMessageConsumer(ctx),
		objectStore,
		repository.NewTranscriptionStorageRepository(ctx),
		repository.NewAgentKVRepository(ctx))

	return svc
}

func NewCredentialService(ctx context.Context) servicetypes.CredentialService {
	encryptionClient := awskms.NewEncryptionClient(ctx)

	return credentialservice.NewCredentialService(
		ctx,
		repository.NewCredentialRepository(ctx),
		encryptionClient,
		NewAuditService(ctx),
		repository.TransactionHandler(ctx),
	)
}

func NewWorkflowManagementService(ctx context.Context) servicetypes.WorkflowService {
	DeploymentAgentStatsSerivce := NewDeployAgentStatsService(ctx)

	return workflowservice.NewService(ctx, repository.NewWorkflowRepository(ctx), DeploymentAgentStatsSerivce, repository.TransactionHandler(ctx))
}

func NewDeployAgentStatsService(ctx context.Context) servicetypes.DeploymentAgentStatsService {
	return deploymentstatsservice.NewDeployAgentStatsService(ctx, repository.NewAgentRepository(ctx))
}

func NewWorkflowStatsService(ctx context.Context) servicetypes.WorkflowStatsService {
	return workflowstatsservice.NewService(ctx, repository.NewWorkflowRepository(ctx), repository.NewAgentRepository(ctx))
}

func NewAuditService(ctx context.Context) servicetypes.AuditService {
	return auditservice.NewService(ctx, repository.NewAuditRepository(ctx))
}
