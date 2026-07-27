package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	apphttp "github.com/ayushsarode/distributed-job-scheduler/internal/api/http"
	"github.com/ayushsarode/distributed-job-scheduler/internal/cache"
	"github.com/ayushsarode/distributed-job-scheduler/internal/config"
	"github.com/ayushsarode/distributed-job-scheduler/internal/db"
	"github.com/ayushsarode/distributed-job-scheduler/internal/metrics"
	"github.com/ayushsarode/distributed-job-scheduler/internal/repository"
	"github.com/rs/zerolog"
)

const shutdownTimeout = 30 * time.Second

func RunAPI(ctx context.Context, cfg *config.Config, log zerolog.Logger) error {
	database, err := db.New(ctx, db.Config{DSN: cfg.PostgresDSN})
	if err != nil {
		return fmt.Errorf("db connect: %w", err)
	}
	defer database.Close()

	metrics.Register()

	jobsRepo := repository.NewJobsRepo(database)
	deadLettersRepo := repository.NewDeadLettersRepo(database)
	workersRepo := repository.NewWorkerRepo(database)

	idem := cache.NewIdempotencyStore(cfg.RedisAddr)
	defer idem.Close()

	limiter := cache.NewRateLimiter(cfg.RedisAddr, 100, time.Minute)
	defer limiter.Close()

	statusCache := cache.NewStatusCache(cfg.RedisAddr)
	defer statusCache.Close()

	server := apphttp.NewServer(
		cfg.HTTPPort,
		jobsRepo,
		workersRepo,
		deadLettersRepo,
		idem,
		limiter,
		statusCache,
		cfg.APIKey,
		log,
	)

	serverErrCh := make(chan error, 1)
	go func() {
		log.Info().Int("port", cfg.HTTPPort).Msg("HTTP server listening")
		if err := server.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		log.Info().Msg("shutdown complete")
		return nil
	case err := <-serverErrCh:
		return err
	}
}
