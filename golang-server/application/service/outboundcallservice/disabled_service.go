package outboundcallservice

import (
	"context"
	"errors"
	"time"

	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types"
	"exiro.ai/application/service/types/entity"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// DisabledOutboundCallService is a no-op implementation when outbound calling is disabled.
type DisabledOutboundCallService struct{}

var _ types.OutboundCallService = &DisabledOutboundCallService{}

func (d *DisabledOutboundCallService) UploadOutboundCallDocument(ctx context.Context, filename string, fileType string, fileContent []byte) (string, error) {
	return "", errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) ListOutboundCallDocuments(ctx context.Context, limit int32, offset int32) ([]entity.OutboundCallDocument, error) {
	return nil, errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) GetOutboundCallDocument(ctx context.Context, documentId string, tenantID uuid.UUID) (entity.OutboundCallDocument, error) {
	return entity.OutboundCallDocument{}, errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) DeleteOutboundCallDocument(ctx context.Context, documentId string, tenantID uuid.UUID) (string, error) {
	return "", errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) CreateCallJob(ctx context.Context, callJob entity.CallJob) (entity.CallJob, error) {
	return entity.CallJob{}, errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) GetCallJob(ctx context.Context, id uuid.UUID) (entity.CallJob, error) {
	return entity.CallJob{}, errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) ListCallJobs(ctx context.Context, statuses []entity.CallJobStatus, workflowId uuid.UUID, filterDateBegin time.Time, filterDateEnd time.Time, limit int32, offset int32) ([]entity.CallJob, int32, error) {
	return nil, 0, errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) DeleteCallJob(ctx context.Context, id uuid.UUID) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) UpdateCallJob(ctx context.Context, callJob entity.CallJob) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) MaterialiseJob(ctx context.Context, jobid uuid.UUID) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) StartMaterialiseJobWorker(ctx context.Context) {
	// Do nothing - service is disabled
}

func (d *DisabledOutboundCallService) TriggerJob(ctx context.Context, jobId uuid.UUID) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) ListJobItems(ctx context.Context, jobId uuid.UUID, limit int32, offset int32) ([]entity.JobItem, int32, error) {
	return nil, 0, errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) StartTriggerJobWorker(ctx context.Context) {
	// Do nothing - service is disabled
}

func (d *DisabledOutboundCallService) SendCompletedMessage(ctx context.Context, agentID, sessionID, jobItemID string) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) HandleWSConnection(ctx context.Context, conn *websocket.Conn, agentID, sessionID, jobItemID string) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) ProcessCallStatusWebhook(ctx context.Context, callSid string, jobItemId uuid.UUID, sessionId string, callStatus string, callDuration string, timestamp string, errorMessage string) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) ProcessExotelStatusWebhook(ctx context.Context, jobItemID uuid.UUID, sessionId string, status string, duration string, dateUpdated string) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) UpdateCallJobDetails(ctx context.Context, jobId string, name string, workflowId uuid.UUID) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) UpdateJobItemWithExternalCallId(ctx context.Context, jobItemId uuid.UUID, externalCallId string) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) CopyCallJob(ctx context.Context, sourceJobID string, name string, providerId uuid.UUID, workflowId uuid.UUID, preferedLanguage string, jobItemIDs []string) (string, int32, error) {
	return "", 0, errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) AddJobItems(ctx context.Context, jobId string, items []entity.NewJobItem) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) RemoveJobItems(ctx context.Context, jobId string, jobItemIds []string) error {
	return errors.New("outbound calling service is disabled")
}

func (d *DisabledOutboundCallService) GetCallDetails(ctx context.Context, callSid string, jobItemId uuid.UUID) (entity.CallDetails, error) {
	return entity.CallDetails{}, xerrors.InternalError(ctx, errors.New("outbound calling service is disabled"))
}

func (d *DisabledOutboundCallService) GetAudioSampleRateForAgent(ctx context.Context, agentID string) (int, error) {
	return 0, errors.New("outbound calling service is disabled")
}
