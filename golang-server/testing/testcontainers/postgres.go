package testcontainers

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	// Database configuration.
	TestDBName   = "test_db"
	TestUser     = "postgres"
	TestPassword = "test_password"

	// Container configuration.
	PostgresImage     = "postgres:17"
	PostgresPort      = "5432/tcp"
	StartupTimeout    = 60 * time.Second
	LogWaitOccurrence = 2
)

// PostgresContainer wraps the testcontainer and provides connection info.
type PostgresContainer struct {
	Container testcontainers.Container
	Pool      *pgxpool.Pool
	DSN       string
	Host      string
	Port      string
}

// SetupPostgresForTestMain starts a PostgreSQL container for test suite.
func SetupPostgresForTestMain(ctx context.Context) (*PostgresContainer, func(context.Context) error, error) {
	container, err := startPostgresContainer(ctx)
	if err != nil {
		return nil, nil, err
	}

	host, port, err := getContainerHostPort(ctx, container)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, err
	}

	dsn := buildDSN(host, port)
	pool, err := createAndVerifyPool(ctx, dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, err
	}

	pc := &PostgresContainer{
		Container: container,
		Pool:      pool,
		DSN:       dsn,
		Host:      host,
		Port:      port,
	}

	cleanup := func(ctx context.Context) error {
		pool.Close()
		return container.Terminate(ctx)
	}

	return pc, cleanup, nil
}

func startPostgresContainer(ctx context.Context) (testcontainers.Container, error) {
	req := testcontainers.ContainerRequest{
		Image:        PostgresImage,
		ExposedPorts: []string{PostgresPort},
		Env: map[string]string{
			"POSTGRES_USER":     TestUser,
			"POSTGRES_PASSWORD": TestPassword,
			"POSTGRES_DB":       TestDBName,
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(LogWaitOccurrence).
			WithStartupTimeout(StartupTimeout),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start PostgreSQL container: %w", err)
	}

	return container, nil
}

func getContainerHostPort(ctx context.Context, container testcontainers.Container) (string, string, error) {
	host, err := container.Host(ctx)
	if err != nil {
		return "", "", fmt.Errorf("get PostgreSQL host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return "", "", fmt.Errorf("get PostgreSQL port: %w", err)
	}

	return host, port.Port(), nil
}

func buildDSN(host, port string) string {
	hostPort := net.JoinHostPort(host, port)
	return fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		TestUser, TestPassword, hostPort, TestDBName)
}

func createAndVerifyPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}

	return pool, nil
}

// Terminate stops the container and closes connections.
func (pc *PostgresContainer) Terminate(ctx context.Context) error {
	if pc == nil {
		return nil
	}
	if pc.Pool != nil {
		pc.Pool.Close()
	}
	if pc.Container != nil {
		return pc.Container.Terminate(ctx)
	}
	return nil
}
