package repository

import (
	"context"

	"exiro.ai/application/service/internal/repository/dynamodb/agentkv"
	"exiro.ai/application/service/internal/repository/dynamodb/transcriptionstorage"
	agent "exiro.ai/application/service/internal/repository/postgres/agent"
	"exiro.ai/application/service/internal/repository/postgres/audit"
	"exiro.ai/application/service/internal/repository/postgres/call_job"
	"exiro.ai/application/service/internal/repository/postgres/credential"
	"exiro.ai/application/service/internal/repository/postgres/knowledgebase"
	outboundcalldocument "exiro.ai/application/service/internal/repository/postgres/outbound_call_document"
	"exiro.ai/application/service/internal/repository/postgres/transaction"
	"exiro.ai/application/service/internal/repository/postgres/usermanagement"
	"exiro.ai/application/service/internal/repository/postgres/workflow"
	"exiro.ai/application/service/internal/types"
)

func TransactionHandler(ctx context.Context) types.TransationHandler {
	return transaction.NewTransactionHandler(ctx)
}

func NewKnowledgeBaseRepository(ctx context.Context) types.KnoledgeBaseRepository {
	return knowledgebase.NewRepository(ctx)
}

func NewAgentRepository(ctx context.Context) types.AgentRepository {
	return agent.NewRepository(ctx)
}

func NewUserManagementRepository(ctx context.Context) types.UserManagementRepository {
	return usermanagement.NewRepository(ctx)
}

func NewOutboundCallDocumentRepository(ctx context.Context) types.OutboundCallDocumentRepository {
	return outboundcalldocument.NewRepository(ctx)
}

func NewOutBoundCallJobRepository(ctx context.Context) types.CallJobRepository {
	return call_job.NewRepository(ctx)
}

func NewTranscriptionStorageRepository(ctx context.Context) types.TranscriptionRepository {
	return transcriptionstorage.NewTranscriptionRepository(ctx)
}

func NewAgentKVRepository(ctx context.Context) types.AgentKVRepository {
	return agentkv.NewAgentKVRepository(ctx)
}

func NewCredentialRepository(ctx context.Context) types.CredentialRepository {
	return credential.NewRepository(ctx)
}

func NewWorkflowRepository(ctx context.Context) types.WorkflowRepository {
	return workflow.NewRepository(ctx)
}

func NewAuditRepository(ctx context.Context) types.AuditRepository {
	return audit.NewRepository(ctx)
}
