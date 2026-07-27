package testcontainers

import (
	"context"
	"fmt"
	"net"
	"time"

	testcontainersgo "github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

const (
	defaultRegion          = "us-east-1"
	localstackStartTimeout = 60 * time.Second
)

// LocalStackContainer wraps the testcontainer and provides AWS clients.
type LocalStackContainer struct {
	Container testcontainersgo.Container
	Endpoint  string
}

func (lsc *LocalStackContainer) Terminate(ctx context.Context) error {
	if lsc == nil || lsc.Container == nil {
		return nil
	}
	return lsc.Container.Terminate(ctx)
}

func SetupLocalStackForTestMain(ctx context.Context) (*LocalStackContainer, func(context.Context) error, error) {
	container, err := startLocalStackContainer(ctx)
	if err != nil {
		return nil, nil, err
	}

	endpoint, err := getLocalStackEndpoint(ctx, container)
	if err != nil {
		_ = container.Terminate(ctx)
		return nil, nil, err
	}

	lsc := &LocalStackContainer{
		Container: container,
		Endpoint:  endpoint,
	}

	cleanup := func(ctx context.Context) error {
		return lsc.Terminate(ctx)
	}

	return lsc, cleanup, nil
}

func startLocalStackContainer(ctx context.Context) (testcontainersgo.Container, error) {
	req := testcontainersgo.ContainerRequest{
		Image:        "localstack/localstack:community-archive",
		ExposedPorts: []string{"4566/tcp"},
		Env: map[string]string{
			"SERVICES":              "dynamodb,s3,sqs,kms",
			"DEFAULT_REGION":        defaultRegion,
			"EAGER_SERVICE_LOADING": "1",
		},
		WaitingFor: wait.ForLog("Ready.").WithStartupTimeout(localstackStartTimeout),
	}

	container, err := testcontainersgo.GenericContainer(ctx, testcontainersgo.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("start LocalStack container: %w", err)
	}

	return container, nil
}

func getLocalStackEndpoint(ctx context.Context, container testcontainersgo.Container) (string, error) {
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("get LocalStack host: %w", err)
	}

	port, err := container.MappedPort(ctx, "4566")
	if err != nil {
		return "", fmt.Errorf("get LocalStack port: %w", err)
	}

	hostPort := net.JoinHostPort(host, port.Port())
	return "http://" + hostPort, nil
}
