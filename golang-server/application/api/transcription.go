package api

import (
	"context"
	"io"
	"net/http"

	"connectrpc.com/connect"
	"exiro.ai/application/models/pb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (r *restApi) TestPushSQSHandler(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}

	var event pb.TranscriptEvent
	if err := protojson.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid request body (protojson unmarshal failed): "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := r.transcriptionStorageService.TestPushSQS(req.Context(), &event); err != nil {
		http.Error(w, "Failed to push message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("Message pushed to SQS"))
}

// GetTranscriptionMetadata implements the gRPC endpoint for fetching transcript metadata and signed URL.
func (g *grpcApi) GetTranscriptionMetadata(ctx context.Context, req *connect.Request[pb.GetTranscriptionMetadataRequest]) (*connect.Response[pb.GetTranscriptionMetadataResponse], error) {
	metadata, err := g.transcriptionstorageService.GetTranscriptionMetadata(ctx, req.Msg.GetSessionId())
	if err != nil {
		return nil, err
	}

	resp := &pb.GetTranscriptionMetadataResponse{
		SessionId:       req.Msg.GetSessionId(),
		Status:          pb.Status(pb.Status_value[metadata.Status]),
		DownloadUrl:     metadata.DownloadURL,
		DurationSeconds: float32(metadata.DurationSecs),
		CreatedAt:       timestamppb.New(metadata.CreatedAt),
	}
	return connect.NewResponse(resp), nil
}

// GetSessionKVData implements agent KV read for a session.
func (g *grpcApi) GetSessionKVData(ctx context.Context, req *connect.Request[pb.GetSessionKVDataRequest]) (*connect.Response[pb.GetSessionKVDataResponse], error) {
	pairs, err := g.transcriptionstorageService.GetSessionKVData(ctx, req.Msg.GetSessionId())
	if err != nil {
		return nil, err
	}

	resp := &pb.GetSessionKVDataResponse{}
	for _, p := range pairs {
		resp.KvPairs = append(resp.KvPairs, &pb.KVPair{
			Key:       p.Key,
			Value:     p.Value,
			CreatedAt: timestamppb.New(p.CreatedAt),
			UpdatedAt: timestamppb.New(p.UpdatedAt),
		})
	}
	return connect.NewResponse(resp), nil
}
