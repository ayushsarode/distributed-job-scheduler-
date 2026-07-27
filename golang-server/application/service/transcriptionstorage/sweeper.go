package transcriptionstorage

import (
	"context"
	"time"

	"exiro.ai/config"
)

func (s *Service) startSweeper(ctx context.Context) {
	sweepInterval := time.Duration(config.Ctx(ctx).TranscriptionStorage.Sweeper.SweepIntervalMinutes) * time.Minute
	staleThreshold := time.Duration(config.Ctx(ctx).TranscriptionStorage.Sweeper.StaleThresholdMinutes) * time.Minute

	s.logger.Info().
		Dur("sweep_interval", sweepInterval).
		Dur("stale_threshold", staleThreshold).
		Msg("Starting sweeper...")

	go func() {
		ticker := time.NewTicker(sweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				s.logger.Info().Msg("Sweeper shutting down")
				return
			case <-ticker.C:
				s.sweepOnce(ctx, staleThreshold)
			}
		}
	}()
}

func (s *Service) sweepOnce(ctx context.Context, staleThreshold time.Duration) {
	statuses := []string{"IN_PROGRESS", "AGGREGATING"}
	cutoff := time.Now().Add(-staleThreshold)

	for _, st := range statuses {
		sessionIDs, err := s.transcriptionRepository.Query(ctx, st, cutoff)
		if err != nil {
			s.logger.Error().Err(err).Str("status", st).Msg("sweeper: query failed")
			continue
		}

		for _, sid := range sessionIDs {
			if err := s.handleStaleSession(ctx, sid); err != nil {
				s.logger.Error().Err(err).Str("session_id", sid).Msg("sweeper: handle failed")
			}
		}
	}
}

func (s *Service) handleStaleSession(ctx context.Context, sessionID string) error {
	s.logger.Info().Str("session_id", sessionID).Msg("Handling stale session")

	metaDynamo, err := s.transcriptionRepository.GetSessionMeta(ctx, sessionID)
	if err != nil {
		return err
	}
	if metaDynamo == nil {
		s.logger.Warn().Str("session_id", sessionID).Msg("No META found; skipping")
		return nil
	}

	if metaDynamo.TenantID == "" {
		s.logger.Warn().Str("session_id", sessionID).Msg("No tenant_id found; skipping")
		return nil
	}

	return s.aggregateSession(ctx, aggregationRequest{
		SessionID:    sessionID,
		TenantID:     metaDynamo.TenantID,
		DurationSecs: 0,
		IsStale:      true,
	})
}
