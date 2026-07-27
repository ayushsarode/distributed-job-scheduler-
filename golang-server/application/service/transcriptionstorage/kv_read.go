package transcriptionstorage

import (
	"context"
	"errors"
	"strings"

	"exiro.ai/application/auth"
	xerrors "exiro.ai/application/errors"
	"exiro.ai/application/service/types/entity"
)

func (s *Service) GetSessionKVData(ctx context.Context, sessionID string) ([]entity.AgentKVItem, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, xerrors.BadRequestError(ctx, errors.New("session_id is required"))
	}

	tenantID := auth.MustGetTenant(ctx)

	// query up to 101 items. if we get 101 back, we know the result was truncated.
	// TODO: implement proper pagination; for now the top 100 are returned.
	const maxKVItems = 100
	rows, err := s.agentKVRepository.QuerySessionKV(ctx, sessionID, int32(maxKVItems+1))
	if err != nil {
		s.logger.Error().
			Ctx(ctx).
			Err(err).
			Str("session_id", sessionID).
			Str("tenant_id", tenantID.String()).
			Msg("GetSessionKVData: failed to query KV items")
		return nil, err
	}

	if len(rows) > maxKVItems {
		s.logger.Warn().
			Ctx(ctx).
			Str("session_id", sessionID).
			Int("limit", maxKVItems).
			Msg("KV result truncated: session has more than 100 KV items")
		rows = rows[:maxKVItems]
	}

	out := make([]entity.AgentKVItem, 0, len(rows))
	for _, row := range rows {
		if row.TenantID != tenantID.String() {
			s.logger.Warn().
				Ctx(ctx).
				Str("session_id", sessionID).
				Str("requesting_tenant_id", tenantID.String()).
				Str("owner_tenant_id", row.TenantID).
				Msg("Authorization failed: tenant does not own this KV session data")
			return nil, xerrors.PermissionDeniedError(ctx, errors.New("tenant does not own this session data"))
		}

		row.CreatedAt = row.CreatedAt.UTC()
		row.UpdatedAt = row.UpdatedAt.UTC()
		out = append(out, row)
	}

	return out, nil
}
