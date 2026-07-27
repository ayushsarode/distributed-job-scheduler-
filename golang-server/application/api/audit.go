package api

import (
	"context"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"exiro.ai/application/service/types/entity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (g *grpcApi) ListAuditLogs(
	ctx context.Context,
	req *connect.Request[pb.ListAuditLogsRequest],
) (*connect.Response[pb.ListAuditLogsResponse], error) {
	limit := req.Msg.GetLimit()
	offset := req.Msg.GetOffset()
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	query := entity.AuditLogQuery{Limit: limit, Offset: offset}
	if s := req.Msg.GetResourceType(); s != "" {
		query.ResourceType = &s
	}
	if s := req.Msg.GetUserId(); s != "" {
		query.UserID = &s
	}
	if t := req.Msg.GetStartTime(); t != nil {
		ts := t.AsTime()
		query.StartTime = &ts
	}
	if t := req.Msg.GetEndTime(); t != nil {
		ts := t.AsTime()
		query.EndTime = &ts
	}

	logs, err := g.auditService.ListAuditLogs(ctx, query)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListAuditLogsResponse{}
	for _, audit := range logs {
		resp.AuditLogs = append(resp.AuditLogs, &pb.AuditLogEntry{
			Id:           audit.ID.String(),
			TenantId:     audit.TenantID.String(),
			UserId:       audit.UserID,
			Action:       audit.Action,
			ResourceType: audit.ResourceType,
			ResourceId:   audit.ResourceID,
			CreatedAt:    timestamppb.New(audit.CreatedAt),
		})
	}

	return connect.NewResponse(resp), nil
}

