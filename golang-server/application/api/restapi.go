package api

import (
	"context"

	"exiro.ai/application/service/types"
	"exiro.ai/config"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type restApi struct {
	knowledgeBaseService        types.KnowledgeBaseService
	callHandlerService          types.CallHandlerService
	outboundCallService         types.OutboundCallService
	transcriptionStorageService types.TranscriptionStorageService
	logger                      *zerolog.Logger
	baseUrl                     string
}

func newRestApi(
	ctx context.Context,
	callHandlerService types.CallHandlerService,
	knowledgeBaseService types.KnowledgeBaseService,
	outboundcallservice types.OutboundCallService,
	transcriptionStorageService types.TranscriptionStorageService,
) restApi {
	return restApi{
		logger:                      zerolog.Ctx(ctx),
		knowledgeBaseService:        knowledgeBaseService,
		callHandlerService:          callHandlerService,
		outboundCallService:         outboundcallservice,
		transcriptionStorageService: transcriptionStorageService,
		baseUrl:                     config.Ctx(ctx).OutboundCalling.BaseURL,
	}
}

func (r *restApi) RegisterRoutes(ctx context.Context, mux *chi.Mux) {
	mux.Post("/upload-file", r.UploadFileHandler)
	mux.HandleFunc("/public/twilio-handler/{agent-id}/{session-id}/{job-item-id}", r.twilioHandler)
	mux.HandleFunc("/public/call/{agent-id}/{session-id}", r.callHandler)
	mux.Post("/public/call-status-webhook/{job-item-id}/{session-id}", r.callStatusWebhook)
	mux.HandleFunc("/public/exotel-handler/{agent-id}/{session-id}/{job-item-id}", r.exotelHandler)
	mux.Post("/public/exotel/status-callback/{job-item-id}/{session-id}", r.exotelStatusWebhook)
	mux.Get("/public/exotel/resolve-voicebot", r.exotelResolverHandler)
	mux.Post("/public/exotel/resolve-voicebot", r.exotelResolverHandler)
	if !config.Ctx(ctx).ProductionMode {
		// Test Utility. Not enabled in production.
		mux.Post("/transcriptionstorage/test/push-sqs", r.TestPushSQSHandler)
	}
}
