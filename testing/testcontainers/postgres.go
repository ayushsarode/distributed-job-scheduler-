package testcontainers

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	postgresImage     = "postgres:16"
	postgresPort      = "5432/tcp"
	testDBName        = "scheduler_test"
	testDBUser        = "scheduler"
	testDBPassword    = "scheduler"
	containerDeadline = 60 * time.Second
)

type PostgresContainer struct {
	Container tc.Container
	Pool      *pgxpool.Pool
	DSN       string
	Host      string
	Port      string
}

func SetupPostgres(ctx context.Context) (*PostgresContainer, func(context.Context) error, error) {
	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: tc.ContainerRequest{
			Image:        postgresImage,
			ExposedPorts: []string{postgresPort},
			Env: map[string]string{
				"POSTGRES_DB":       testDBName,
				"POSTGRES_USER":     testDBUser,
				"POSTGRES_PASSWORD": testDBPassword,
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(containerDeadline),
		},
		Started: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start postgres container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("get postgres host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "5432")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("get postgres port: %w", err)
	}

	port := mappedPort.Port()
	dsn := fmt.Sprintf("postgres://%s:%s@%s/%s?sslmode=disable",
		testDBUser,
		testDBPassword,
		net.JoinHostPort(host, port),
		testDBName,
	)

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("connect postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("ping postgres: %w", err)
	}

	postgres := &PostgresContainer{
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

	return postgres, cleanup, nil
}
