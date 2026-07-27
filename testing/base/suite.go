package base

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ayushsarode/distributed-job-scheduler/internal/application"
	"github.com/ayushsarode/distributed-job-scheduler/internal/config"
	"github.com/ayushsarode/distributed-job-scheduler/internal/db"
	"github.com/ayushsarode/distributed-job-scheduler/testing/testcontainers"
	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/suite"
)

const (
	cleanupTimeout     = 30 * time.Second
	serverStartupDelay = 10 * time.Second
	serverPollInterval = 100 * time.Millisecond
	httpClientTimeout  = 10 * time.Second
)

type IntegrationSuite struct {
	suite.Suite

	Postgres *testcontainers.PostgresContainer
	Redis    *testcontainers.RedisContainer
	DB       *db.DB

	ServerCancel context.CancelFunc
	ServerDone   chan error
	ServerURL    string
	Client       *http.Client

	Cleanup []func(context.Context) error
}

func (s *IntegrationSuite) SetupSuite() {
	s.setupContainers()
	s.runMigrations()
	s.setupApp()
}

func (s *IntegrationSuite) TearDownSuite() {
	if s.ServerCancel != nil {
		s.ServerCancel()
		if s.ServerDone != nil {
			select {
			case err := <-s.ServerDone:
				s.Require().NoError(err)
			case <-time.After(cleanupTimeout):
				s.T().Log("server shutdown timed out")
			}
		}
	}
	if s.DB != nil {
		s.DB.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	for i := len(s.Cleanup) - 1; i >= 0; i-- {
		if err := s.Cleanup[i](ctx); err != nil {
			s.T().Logf("cleanup error: %v", err)
		}
	}
}

func (s *IntegrationSuite) SetupTest() {
	ctx := s.T().Context()
	_, err := s.DB.Pool.Exec(ctx, "TRUNCATE dead_letters, jobs, workers RESTART IDENTITY CASCADE")
	s.Require().NoError(err)

	redisClient := redis.NewClient(&redis.Options{Addr: s.Redis.Addr})
	defer redisClient.Close()
	s.Require().NoError(redisClient.FlushDB(ctx).Err())
}

func (s *IntegrationSuite) setupContainers() {
	ctx := s.T().Context()
	_ = os.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	postgresContainer, cleanupPostgres, err := testcontainers.SetupPostgres(ctx)
	s.Require().NoError(err)
	s.Postgres = postgresContainer
	s.Cleanup = append(s.Cleanup, cleanupPostgres)

	redisContainer, cleanupRedis, err := testcontainers.SetupRedis(ctx)
	s.Require().NoError(err)
	s.Redis = redisContainer
	s.Cleanup = append(s.Cleanup, cleanupRedis)
}

func (s *IntegrationSuite) runMigrations() {
	pgxCfg, err := pgx.ParseConfig(s.Postgres.DSN)
	s.Require().NoError(err)

	sqlDB := stdlib.OpenDB(*pgxCfg)
	defer sqlDB.Close()

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	s.Require().NoError(err)

	migrationsPath := filepath.Join(repoRoot(), "migrations")
	migrator, err := migrate.NewWithDatabaseInstance("file://"+migrationsPath, "postgres", driver)
	s.Require().NoError(err)
	defer migrator.Close()

	err = migrator.Up()
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		s.Require().NoError(err)
	}
}

func (s *IntegrationSuite) setupApp() {
	ctx := s.T().Context()
	database, err := db.New(ctx, db.Config{DSN: s.Postgres.DSN})
	s.Require().NoError(err)
	s.DB = database

	port, err := getFreePort()
	s.Require().NoError(err)

	serverCtx, cancel := context.WithCancel(context.Background())
	s.ServerCancel = cancel
	s.ServerDone = make(chan error, 1)

	cfg := &config.Config{
		PostgresDSN:  s.Postgres.DSN,
		HTTPPort:     port,
		KafkaBrokers: []string{"localhost:9092"},
		RedisAddr:    s.Redis.Addr,
		APIKey:       "",
	}

	go func() {
		s.ServerDone <- application.RunAPI(serverCtx, cfg, zerolog.Nop())
	}()

	s.ServerURL = fmt.Sprintf("http://localhost:%d", port)
	s.Client = &http.Client{Timeout: httpClientTimeout}
	s.waitForServer(s.ServerDone)
}

func (s *IntegrationSuite) waitForServer(serverErrCh <-chan error) {
	deadline := time.After(serverStartupDelay)
	ticker := time.NewTicker(serverPollInterval)
	defer ticker.Stop()

	for {
		select {
		case err := <-serverErrCh:
			s.Require().NoError(err)
			return
		case <-deadline:
			s.Require().Fail("server did not become ready within startup timeout")
			return
		case <-ticker.C:
			resp, err := s.Client.Get(s.ServerURL + "/health")
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return
				}
			}
		}
	}
}

func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("find free port: %w", err)
	}
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, fmt.Errorf("parse free port: %w", err)
	}

	var parsed int
	_, err = fmt.Sscanf(port, "%d", &parsed)
	if err != nil {
		return 0, fmt.Errorf("parse port number: %w", err)
	}
	return parsed, nil
}

func repoRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("cannot resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}
