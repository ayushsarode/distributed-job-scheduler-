package testcontainers

import (
	"context"
	"fmt"
	"net"

	tc "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	redisImage = "redis:8.8.0-alpine"
	redisPort  = "6379/tcp"
)

type RedisContainer struct {
	Container tc.Container
	Addr      string
	Host      string
	Port      string
}

func SetupRedis(ctx context.Context) (*RedisContainer, func(context.Context) error, error) {
	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: tc.ContainerRequest{
			Image:        redisImage,
			ExposedPorts: []string{redisPort},
			WaitingFor: wait.ForListeningPort(redisPort).
				WithStartupTimeout(containerDeadline),
		},
		Started: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("start redis container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("get redis host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "6379")
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, fmt.Errorf("get redis port: %w", err)
	}

	port := mappedPort.Port()
	redis := &RedisContainer{
		Container: container,
		Addr:      net.JoinHostPort(host, port),
		Host:      host,
		Port:      port,
	}

	cleanup := func(ctx context.Context) error {
		return container.Terminate(ctx)
	}

	return redis, cleanup, nil
}
