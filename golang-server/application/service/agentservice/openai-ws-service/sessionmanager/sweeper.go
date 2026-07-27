package sessionmanager

import (
	"context"
	"time"
)

const sweepInterval = 2 * time.Minute

// sweepLoop periodically closes sessions idle beyond the configured timeout.
func (m *Manager) sweepLoop(ctx context.Context) {
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.sweepStaleSessions(ctx)
		}
	}
}

func (m *Manager) sweepStaleSessions(_ context.Context) {
	cutoff := time.Now().Add(-m.staleAfter)
	var toClose []string

	m.sessions.Range(func(key, value any) bool {
		sessionID := key.(string)
		s := value.(*session)
		s.mu.Lock()
		lastActive := s.lastActiveAt
		closed := s.closed
		s.mu.Unlock()

		if !closed && lastActive.Before(cutoff) {
			toClose = append(toClose, sessionID)
		}
		return true
	})

	for _, sessionID := range toClose {
		m.logger.Debug().
			Str("session_id", sessionID).
			Msg("sweeper closing stale session")
		_ = m.Delete(sessionID)
	}
}
