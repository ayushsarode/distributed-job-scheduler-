package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/ayushsarode/distributed-job-scheduler/internal/application"
	"github.com/ayushsarode/distributed-job-scheduler/internal/config"
	"github.com/ayushsarode/distributed-job-scheduler/internal/logger"
)

func main() {
	log := logger.New("scheduler-service")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("config load failed")
	}

	if err := application.RunAPI(ctx, cfg, log); err != nil {
		log.Fatal().Err(err).Msg("api server failed")
	}
}
