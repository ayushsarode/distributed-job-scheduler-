package outboundcallservice

import (
	"context"
	"os"

	"exiro.ai/application/assert"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	repositoryTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/outboundcallservice/callProvider"
	"exiro.ai/application/service/outboundcallservice/callProvider/exotel"
	"exiro.ai/application/service/outboundcallservice/callProvider/twilio"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/config"
	"exiro.ai/infra/workflow/xdbos"
	"exiro.ai/utils/text"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/sourcegraph/conc/pool"
)

const (
	materializeJobQueueName      = "materialize_job_queue"
	callJobQueueName             = "call_job_queue"
	MaterializeWorkerConcurrency = 5
	CallJobWorkerConcurrency     = 100
)

// transcriptionEvent represents an event to be sent to transcription service.
type transcriptionEvent struct {
	sessionID string
	ownerID   string
	text      string
	speaker   pb.Speaker
	language  string
}

type Service struct {
	CallJobRepository              repositoryTypes.CallJobRepository
	objectStore                    repositoryTypes.ObjectStore
	outboundCallDocumentRepository repositoryTypes.OutboundCallDocumentRepository
	logger                         *zerolog.Logger
	jobPool                        *pool.Pool
	transactionHandler             repositoryTypes.TransationHandler
	agentRepository                repositoryTypes.AgentRepository
	sttService                     repositoryTypes.STTService
	realTimeService                repositoryTypes.RealtimeAgentService
	ttsHandler                     repositoryTypes.TTSHandler
	SQSMessageQueue                repositoryTypes.SQSMessageQueue
	SQSMessageConsumer             repositoryTypes.SQSMessageConsumer
	agentService                   types.AgentService
	callProvider                   callProvider.CallProvider
	textSegmenter                  *text.Segmenter
	transcriptionQueue             repositoryTypes.FIFOMessageQueue
	materializeJobQueue            dbos.WorkflowQueue
	callJobQueue                   dbos.WorkflowQueue
	providerConfig                 *ProviderConfig
	credentialService              types.CredentialService
	workflowService                types.WorkflowService
	outboundCallConfig             config.OutboundCallingConfig
	auditService                   types.AuditService
}

var _ types.OutboundCallService = &Service{}

func NewService(
	ctx context.Context,
	objectStore repositoryTypes.ObjectStore,
	outboundCallDocumentRepository repositoryTypes.OutboundCallDocumentRepository,
	outboundCallJobRepository repositoryTypes.CallJobRepository,
	jobPool *pool.Pool,
	transactionHandler repositoryTypes.TransationHandler,
	agentRepository repositoryTypes.AgentRepository,
	sttService repositoryTypes.STTService,
	realTimeService repositoryTypes.RealtimeAgentService,
	ttsHandler repositoryTypes.TTSHandler,
	agentService types.AgentService,
	transcriptionQueue repositoryTypes.FIFOMessageQueue,
	providerConfig *ProviderConfig,
	credentialService types.CredentialService,
	workflowService types.WorkflowService,
	outboundCallConfig config.OutboundCallingConfig,
	auditService types.AuditService,
) *Service {
	assert.NotEmpty(os.Getenv("TWILIO_ACCOUNT_SID"), "TWILIO_ACCOUNT_SID is not set")
	assert.NotEmpty(os.Getenv("TWILIO_AUTH_TOKEN"), "TWILIO_AUTH_TOKEN is not set")
	assert.NotNil(auditService)

	logger := zerolog.Ctx(ctx)
	cp := newCallProvider(providerConfig, logger)

	s := &Service{
		objectStore:                    objectStore,
		outboundCallDocumentRepository: outboundCallDocumentRepository,
		CallJobRepository:              outboundCallJobRepository,
		logger:                         logger,
		jobPool:                        jobPool,
		transactionHandler:             transactionHandler,
		agentRepository:                agentRepository,
		sttService:                     sttService,
		realTimeService:                realTimeService,
		ttsHandler:                     ttsHandler,
		agentService:                   agentService,
		callProvider:                   cp,
		textSegmenter:                  text.NewSegmenter(),
		transcriptionQueue:             transcriptionQueue,
		materializeJobQueue: dbos.NewWorkflowQueue(
			xdbos.Ctx(ctx),
			materializeJobQueueName,
			dbos.WithWorkerConcurrency(MaterializeWorkerConcurrency), // Process up to materialize 5 jobs concurrently per worker
		),
		callJobQueue: dbos.NewWorkflowQueue(
			xdbos.Ctx(ctx),
			callJobQueueName,
			dbos.WithWorkerConcurrency(CallJobWorkerConcurrency), // 100 call jobs can run concurrently per worker
		),
		providerConfig:     providerConfig,
		credentialService:  credentialService,
		workflowService:    workflowService,
		outboundCallConfig: outboundCallConfig,
		auditService:       auditService,
	}

	s.registerWorkflows(ctx)

	return s
}

func newCallProvider(pc *ProviderConfig, logger *zerolog.Logger) callProvider.CallProvider {
	cp, err := pc.NewCallProvider(logger)
	assert.NoError(err, "Failed to create call provider")
	return cp
}

func (s *Service) registerWorkflows(ctx context.Context) {
	dbos.RegisterWorkflow(xdbos.Ctx(ctx),
		s.materialzeCallJobWorkflow,
		dbos.WithWorkflowName("Materialize Call Job"),
	)

	dbos.RegisterWorkflow(xdbos.Ctx(ctx),
		s.triggerCallJobWorkflow,
		dbos.WithWorkflowName("Trigger Call Job"),
	)

	dbos.RegisterWorkflow(xdbos.Ctx(ctx),
		s.processJobItemWorkflow,
		dbos.WithWorkflowName("Process Job Item"),
	)
}

func (s *Service) GetCallDetails(ctx context.Context, callSid string, jobItemId uuid.UUID) (entity.CallDetails, error) {
	jobItem, err := s.CallJobRepository.INTERNALGetJobItem(ctx, jobItemId)
	if err != nil {
		s.logger.Error().Err(err).Str("job_item_id", jobItemId.String()).Msg("Failed to get job item for call details")
		return entity.CallDetails{}, err
	}

	callJob, err := s.CallJobRepository.GetCallJob(ctx, jobItem.JobID, jobItem.TenantID)
	if err != nil {
		s.logger.Error().Err(err).Str("job_id", jobItem.JobID.String()).Msg("Failed to get call job for call details")
		return entity.CallDetails{}, err
	}

	credential, credMap, err := s.getCredentialAndMap(ctx, callJob)
	if err != nil {
		return entity.CallDetails{}, err
	}

	details, err := s.fetchCallDetailsFromProvider(ctx, callSid, credential.Type, credMap)
	if err != nil {
		return entity.CallDetails{}, err
	}

	return entity.CallDetails{
		Sid:          details.Sid,
		Status:       details.Status,
		StartTime:    details.StartTime,
		EndTime:      details.EndTime,
		Duration:     details.Duration,
		From:         details.From,
		To:           details.To,
		RecordingUrl: details.RecordingUrl,
	}, nil
}

func (s *Service) getCredentialAndMap(ctx context.Context, callJob entity.CallJob) (entity.Credential, map[string]string, error) {
	var credential entity.Credential
	var err error

	if callJob.Outbound_call_provider_id != "" {
		credential, err = s.credentialService.GetDecryptedCredential(ctx, uuid.MustParse(callJob.Outbound_call_provider_id))
		if err != nil {
			s.logger.Error().Err(err).
				Str("call_job_id", callJob.ID.String()).
				Str("credential_id", callJob.Outbound_call_provider_id).
				Msg("Failed to get decrypted credential")
			return entity.Credential{}, nil, xerrors.InternalError(ctx, err)
		}
	}

	var credMap map[string]string
	if credential.Type != entity.CredentialTypeUnspecified {
		credMap = getCredMap(credential)
	}

	return credential, credMap, nil
}

func (s *Service) fetchCallDetailsFromProvider(ctx context.Context, callSid string, credType entity.CredentialType, credMap map[string]string) (*callProvider.CallDetails, error) {
	switch credType {
	case entity.CredentialTypeTwilio:
		return twilio.NewTwilioHandler(s.logger).GetCallDetails(ctx, callSid, credMap)
	case entity.CredentialTypeExotel:
		return exotel.NewExotelHandler(s.logger).GetCallDetails(ctx, callSid, credMap)
	case entity.CredentialTypeUnspecified:
		return s.callProvider.GetCallDetails(ctx, callSid, credMap)
	default:
		return s.callProvider.GetCallDetails(ctx, callSid, credMap)
	}
}

func (s *Service) GetAudioSampleRateForAgent(ctx context.Context, agentID string) (int, error) {
	return s.providerConfig.GetSampleRate(agentID), nil
}
