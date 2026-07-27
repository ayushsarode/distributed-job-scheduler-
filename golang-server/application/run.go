package application

import (
	"fmt"

	"exiro.ai/application/api"
	"exiro.ai/application/auth"
	"exiro.ai/application/service"
	appcontext "exiro.ai/application/xcontext"
	"exiro.ai/config"
	"exiro.ai/infra/database/postgres/migrations"
)

func Run() error {
	cfg, err := config.New()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	return RunWithConfig(cfg)
}

func RunWithConfig(cfg *config.Config) error {
	// Use GetAppContextWithConfig to inject the test config
	ctx := appcontext.GetAppContextWithConfig(cfg)

	verifier, err := auth.NewTokenVerifier(ctx)
	if err != nil {
		return fmt.Errorf("failed to create token verifier: %w", err)
	}

	migrations.Migrate(ctx)

	server := api.NewServer(
		ctx,
		service.NewCallHandlerService(ctx),
		service.NewKnowledgeBaseService(ctx),
		service.NewDeploymentService(ctx),
		service.NewUserManagementService(ctx),
		service.NewOutboundCallService(ctx),
		service.NewTranscriptionService(ctx),
		service.NewCredentialService(ctx),
		service.NewWorkflowManagementService(ctx),
		service.NewAuditService(ctx),
		verifier,
	)

	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	return nil
}
