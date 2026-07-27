package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	connectcors "connectrpc.com/cors"
	"exiro.ai/application/auth"
	"exiro.ai/application/service/types"
	"exiro.ai/config"
	"exiro.ai/infra/workflow/xdbos"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/traceid"
	"github.com/rs/cors"
	"github.com/rs/zerolog"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

const (
	serverReadHeaderTimeout = 10 * time.Second
	serverReadTimeout       = 30 * time.Second
	serverWriteTimeout      = 30 * time.Second
	serverIdleTimeout       = 2 * time.Minute
)

type Server struct {
	logger                *zerolog.Logger
	callHandler           types.CallHandlerService
	grpcApi               grpcApi
	restApi               restApi
	tokenVerifier         auth.TokenVerifier
	usermanagementservice types.UserManagementService
}

func NewServer(
	ctx context.Context,
	callService types.CallHandlerService,
	knowledgeBaseService types.KnowledgeBaseService,
	deploymentService types.DeploymentService,
	usermanagementservice types.UserManagementService,
	outboundCallService types.OutboundCallService,
	transcriptionStorageService types.TranscriptionStorageService,
	credentialService types.CredentialService,
	workflowService types.WorkflowService,
	auditService types.AuditService,
	tokenVerifier auth.TokenVerifier,
) *Server {
	return &Server{
		logger:      zerolog.Ctx(ctx),
		callHandler: callService,

		grpcApi: newGrpcApi(ctx,
			callService,
			knowledgeBaseService,
			deploymentService,
			usermanagementservice,
			outboundCallService,
			transcriptionStorageService,
			credentialService,
			workflowService,
			auditService),

		restApi: newRestApi(ctx,
			callService,
			knowledgeBaseService,
			outboundCallService,
			transcriptionStorageService),

		tokenVerifier:         tokenVerifier,
		usermanagementservice: usermanagementservice,
	}
}

func (s *Server) setupRouter() *chi.Mux {
	mux := chi.NewRouter()
	mux.Use(middleware.Logger)
	mux.Use(traceid.Middleware)
	return mux
}

func (s *Server) setupCORS() *cors.Cors {
	return cors.New(cors.Options{
		AllowedOrigins:   []string{"http://localhost:3000", "https://exiro.ai", "https://www.exiro.ai"},
		AllowedMethods:   connectcors.AllowedMethods(),
		AllowedHeaders:   append(connectcors.AllowedHeaders(), "grpc-status", "grpc-message", "Authorization"),
		ExposedHeaders:   append(connectcors.ExposedHeaders(), "grpc-status", "grpc-message", "Authorization"),
		AllowCredentials: true,
	})
}

func (s *Server) setupMiddleware(mux *chi.Mux) {
	mux.Use(s.traceLogMiddleware(s.logger))

	mux.Use(s.newAuthMiddleware([]string{
		"/public",
		"/health",
	}, s.tokenVerifier))

	mux.Use(s.newTenantMiddleware([]string{
		"/public",
		"/health",
		"/grpc.exiro.ai.UserManagementService/RegisterUser",
	}, s.usermanagementservice))
}

func (s *Server) registerRoutes(ctx context.Context, mux *chi.Mux) {
	RegisterHealthRoutes(mux)
	s.grpcApi.RegisterRoutes(mux)
	s.restApi.RegisterRoutes(ctx, mux)
}

func (s *Server) startBackgroundWorkers(ctx context.Context) {
	go func() {
		if err := s.grpcApi.transcriptionstorageService.StartIngestWorker(ctx); err != nil {
			s.logger.Error().Err(err).Msg("Transcription ingest worker exited with error")
		}
	}()
	go func() {
		if err := s.grpcApi.deploymentService.DeploymentStatusPoller(ctx); err != nil {
			s.logger.Error().Err(err).Msg("Deployment status poller exited with error")
		}
	}()
	if err := dbos.Launch(xdbos.Ctx(ctx)); err != nil {
		s.logger.Error().Err(err).Msg("Failed to launch DBOS")
	}
}

func (s *Server) createHTTPHandler(mux *chi.Mux) http.Handler {
	corsMiddleware := s.setupCORS()
	handler := corsMiddleware.Handler(mux)
	return h2c.NewHandler(handler, &http2.Server{})
}

func (s *Server) Start(ctx context.Context) error {
	mux := s.setupRouter()
	s.setupMiddleware(mux)
	s.registerRoutes(ctx, mux)
	s.startBackgroundWorkers(ctx)
	handler := s.createHTTPHandler(mux)
	s.logger.Info().Msgf("Starting server on %s", config.Ctx(ctx).HttpPort)
	server := &http.Server{
		Addr:              fmt.Sprintf(":%v", config.Ctx(ctx).HttpPort),
		Handler:           handler,
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverReadTimeout,
		WriteTimeout:      serverWriteTimeout,
		IdleTimeout:       serverIdleTimeout,
	}
	return server.ListenAndServe()
}
