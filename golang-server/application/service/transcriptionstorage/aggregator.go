package transcriptionstorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"time"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"exiro.ai/config"
)

func (s *Service) aggregateSession(ctx context.Context, req aggregationRequest) error {
	if err := s.transcriptionRepository.ClaimSession(ctx, req.SessionID); err != nil {
		return err
	}

	meta, segments, err := s.transcriptionRepository.FetchSessionItems(ctx, req.SessionID)
	if err != nil {
		return err
	}

	targetStatus := s.determineTargetStatus(req.IsStale)
	callDoc := s.buildCallDocument(req, meta, segments, targetStatus)

	s3Key, err := s.uploadCallDocumentToS3(ctx, callDoc)
	if err != nil {
		return err
	}

	if err := s.transcriptionRepository.FinalizeSession(ctx, req.SessionID, s3Key, targetStatus, req.DurationSecs); err != nil {
		return err
	}

	return nil
}

func (s *Service) determineTargetStatus(isStale bool) string {
	if isStale {
		return "COMPLETED_STALE"
	}
	return "COMPLETED"
}

func (s *Service) buildCallDocument(req aggregationRequest, meta entity.MetaItem, segments []entity.SegmentItem, targetStatus string) callDocument {
	meta.Status = targetStatus

	return callDocument{
		SessionID: req.SessionID,
		TenantID:  req.TenantID,
		Meta:      meta,
		Segments:  segments,
		EndedAt:   time.Now().UTC(),
		Status:    targetStatus,
	}
}

func (s *Service) uploadCallDocumentToS3(ctx context.Context, callDoc callDocument) (string, error) {
	payload, err := json.MarshalIndent(callDoc, "", "  ")
	if err != nil {
		return "", xerrors.InternalError(ctx, err)
	}

	s3Key := callDoc.SessionID + ".json"
	if _, err := s.objectStore.PutObject(
		ctx,
		bytes.NewReader(payload),
		config.Ctx(ctx).TranscriptionStorage.S3BucketName,
		s3Key,
		"application/json",
	); err != nil {
		return "", xerrors.InternalError(ctx, err)
	}

	return s3Key, nil
}

// Called from CallEnd path.
func (s *Service) processEndOfCallEvent(ctx context.Context, event *pb.TranscriptEvent) error {
	endOfCall := event.GetEndOfCall()
	return s.aggregateSession(ctx, aggregationRequest{
		SessionID:    event.GetSessionId(),
		TenantID:     event.GetTenantId(),
		DurationSecs: float64(endOfCall.GetDurationSeconds()),
		IsStale:      false,
	})
}

// GetTranscriptionMetadata fetches transcript metadata and a signed S3 URL for download.
func (s *Service) GetTranscriptionMetadata(ctx context.Context, sessionID string) (entity.TranscriptionMetadata, error) {
	// Extract the authenticated user ID from context (set by auth middleware)
	tenantID := auth.MustGetTenant(ctx)

	metaDynamo, err := s.transcriptionRepository.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return entity.TranscriptionMetadata{}, err
	}
	if metaDynamo == nil {
		return entity.TranscriptionMetadata{}, xerrors.NotFoundError(ctx, errors.New("no transcript metadata found for session: "+sessionID))
	}

	// Authorization check: verify the requesting user owns this transcript
	if metaDynamo.TenantID != tenantID.String() {
		s.logger.Warn().
			Str("session_id", sessionID).
			Str("requesting_user_id", tenantID.String()).
			Str("tenant_id", metaDynamo.TenantID).
			Msg("Authorization failed: user does not own this transcript")
		return entity.TranscriptionMetadata{}, xerrors.InternalError(ctx, errors.New("permission denied: you do not have access to this transcript"))
	}

	// Convert to domain model
	metadata := metaDynamo.ToDomainMetadata()

	// Generate signed S3 download URL if s3_key exists
	if metadata.S3Key != "" {
		if downloadURL, err := s.generateSignedURL(ctx, metadata.S3Key); err == nil {
			metadata.DownloadURL = downloadURL
		}
	}

	return metadata, nil
}
