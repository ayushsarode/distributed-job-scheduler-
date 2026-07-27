package xcontext

import (
	"context"
	"sync"

	"exiro.ai/application/assert"
	"exiro.ai/application/logger"
	"exiro.ai/config"
	"exiro.ai/infra/database/postgres"
	"exiro.ai/infra/workflow/xdbos"
)

var (
	appCtx     context.Context
	appCtxOnce sync.Once
)

// GetAppContext returns a global application context that lives for the lifetime of the app.
// This context includes all necessary dependencies like database connection and logger.
func GetAppContext() context.Context {
	appCtxOnce.Do(func() {
		cfg, err := config.New()
		assert.NoError(err, "failed to load config")

		appCtx = buildContext(cfg)
	})
	return appCtx
}

func GetAppContextWithConfig(cfg *config.Config) context.Context {
	appCtxOnce.Do(func() {
		appCtx = buildContext(cfg)
	})
	return appCtx
}

func buildContext(cfg *config.Config) context.Context {
	ctx := context.Background()
	ctx = config.WithContext(ctx, cfg)
	ctx = postgres.WithContext(ctx)
	ctx = logger.NewLogger(ctx).WithContext(ctx)
	ctx = xdbos.WithContext(ctx, cfg.DBOSDatabaseURL)
	return ctx
}
