package base

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"exiro.ai/application"
	"exiro.ai/config"
	"exiro.ai/testing/testcontainers"
	"exiro.ai/testing/testutil"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/suite"
)

const (
	serverShutdownDelay = 500 * time.Millisecond
	cleanupTimeout      = 30 * time.Second
	serverStartupDelay  = 15 * time.Second
	httpClientTimeout   = 30 * time.Second
	serverPollInterval  = 250 * time.Millisecond
)

// base test suite with shared container and server setup.
type IntegrationSuite struct {
	suite.Suite

	ServerCancel context.CancelFunc
	TestTenantID uuid.UUID
	TestUserID   string
	DBPool       *pgxpool.Pool

	PostgresContainer   *testcontainers.PostgresContainer
	LocalstackContainer *testcontainers.LocalStackContainer
	Cleanup             []func(context.Context) error

	ServerURL   string
	HTTPClient  *http.Client
	Placeholder *testutil.PlaceholderResolver
	Cfg         *config.Config
}

func (s *IntegrationSuite) SetupSuite() {
	s.setRequiredEnvVars()

	s.Placeholder = testutil.NewPlaceholderResolver()
	s.TestTenantID = uuid.Must(uuid.NewV7())
	s.TestUserID = "test-user-" + uuid.Must(uuid.NewV7()).String()
	s.Placeholder.SetTestUserID(s.TestUserID)

	err := os.Chdir("../..")
	s.Require().NoError(err, "Failed to change to root directory")

	s.setupContainers()
	s.setupDatabase()
	s.startServer()
}

// setRequiredEnvVars sets dummy values for environment variables required by the application.
// Values are only set if not already defined.
func (s *IntegrationSuite) setRequiredEnvVars() {
	envDefaults := map[string]string{
		"DEEPGRAM_KEY":          "test-dummy-key",
		"GOOGLE_API_KEY":        "test-dummy-key",
		"SARVAM_API_KEY":        "test-dummy-key",
		"ELEVENLABS_API_KEY":    "test-dummy-key",
		"OPENAI_API_KEY":        "test-dummy-key",
		"TWILIO_ACCOUNT_SID":    "test-dummy-sid",
		"TWILIO_AUTH_TOKEN":     "test-dummy-token",
		"EXOTEL_ACCOUNT_SID":    "test-dummy-sid",
		"EXOTEL_API_TOKEN":      "test-dummy-token",
		"EXOTEL_API_KEY":        "test-dummy-key",
		"AWS_ACCESS_KEY_ID":     "test",
		"AWS_SECRET_ACCESS_KEY": "test",
		"FIREBASE_PROJECT_ID":   "test-project",
	}

	for key, defaultValue := range envDefaults {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, defaultValue); err != nil {
				s.T().Fatalf("Failed to set env var %s: %v", key, err)
			}
		}
	}
}

// SetupTest runs before each test method, resetting the placeholder cache
// so every test gets fresh generated values (UUIDs, etc.).
func (s *IntegrationSuite) SetupTest() {
	s.Placeholder.Clear()
}

func (s *IntegrationSuite) TearDownSuite() {
	if s.ServerCancel != nil {
		s.ServerCancel()
		time.Sleep(serverShutdownDelay)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	for i := len(s.Cleanup) - 1; i >= 0; i-- {
		if err := s.Cleanup[i](ctx); err != nil {
			s.T().Logf("Cleanup error: %v", err)
		}
	}
}

func (s *IntegrationSuite) setupContainers() {
	ctx := s.T().Context()

	postgresContainer, cleanupPostgres, err := testcontainers.SetupPostgresForTestMain(ctx)
	s.Require().NoError(err)
	s.PostgresContainer = postgresContainer
	s.Cleanup = append(s.Cleanup, cleanupPostgres)

	localstackContainer, cleanupLocalStack, err := testcontainers.SetupLocalStackForTestMain(ctx)
	s.Require().NoError(err)
	s.LocalstackContainer = localstackContainer
	s.Cleanup = append(s.Cleanup, cleanupLocalStack)

	s.T().Logf("Containers ready - PostgreSQL: %s, LocalStack: %s",
		postgresContainer.DSN, localstackContainer.Endpoint)
}

func (s *IntegrationSuite) setupDatabase() {
	ctx := s.T().Context()
	var err error
	s.DBPool, err = pgxpool.New(ctx, s.PostgresContainer.DSN)
	s.Require().NoError(err)
	s.Cleanup = append(s.Cleanup, func(ctx context.Context) error {
		if s.DBPool != nil {
			s.DBPool.Close()
		}
		return nil
	})
}

func (s *IntegrationSuite) startServer() {
	cfg, err := config.New()
	s.Require().NoError(err, "Failed to load config")

	cfg.DBOSDatabaseURL = s.PostgresContainer.DSN
	cfg.DatabaseURL = s.PostgresContainer.DSN
	cfg.LocalStackEndpoint = s.LocalstackContainer.Endpoint
	cfg.TestMode = true
	cfg.TestUserID = s.TestUserID

	// Use a dynamically allocated free port to avoid "address already in use" errors
	// when a previous test run left a server on the default port.
	port, err := getFreePort()
	s.Require().NoError(err, "Failed to find a free port")
	cfg.HttpPort = port
	s.Cfg = cfg

	serverErrCh := make(chan error, 1)
	go func() {
		if err := application.RunWithConfig(cfg); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrCh <- err
		}
	}()

	// Wait for the server to become ready by polling the port
	s.ServerURL = "http://localhost:" + cfg.HttpPort
	s.waitForServer(serverErrCh)

	s.HTTPClient = &http.Client{Timeout: httpClientTimeout}

	s.T().Logf("Server started at %s (TestMode: %v)", s.ServerURL, cfg.TestMode)

	s.insertTestData()
}

// getFreePort asks the OS for an available port by briefly listening on :0.
func getFreePort() (string, error) {
	//nolint:gosec,noctx // Binding to all interfaces is intentional for test port discovery.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return "", fmt.Errorf("find free port: %w", err)
	}
	defer l.Close() //nolint:errcheck
	_, port, err := net.SplitHostPort(l.Addr().String())
	if err != nil {
		return "", fmt.Errorf("parse free port: %w", err)
	}
	return port, nil
}

// waitForServer polls the server URL until it responds or the startup timeout is reached.
func (s *IntegrationSuite) waitForServer(serverErrCh <-chan error) {
	deadline := time.After(serverStartupDelay)
	ticker := time.NewTicker(serverPollInterval)
	defer ticker.Stop()

	for {
		select {
		case err := <-serverErrCh:
			s.Require().NoError(err, "Server failed to start")
			return
		case <-deadline:
			s.Require().Fail("Server did not become ready within the startup timeout")
			return
		case <-ticker.C:
			//nolint:noctx // Simple health-check poll in test setup; context not needed.
			resp, err := http.Get(s.ServerURL + "/")
			if err == nil {
				_ = resp.Body.Close()
				return
			}
		}
	}
}

func (s *IntegrationSuite) LoadTestData(serviceName, filename string, msg proto.Message) {
	s.T().Helper()

	path := filepath.Join("testing", serviceName, "testdata", filename)

	//nolint:gosec
	data, err := os.ReadFile(path)
	s.Require().NoError(err, "Failed to read test data file: %s", path)

	resolvedData, err := s.Placeholder.Resolve(string(data))
	s.Require().NoError(err, "Failed to resolve placeholders in: %s", path)

	err = protojson.Unmarshal([]byte(resolvedData), msg)
	s.Require().NoError(err, "Failed to unmarshal test data: %s", path)
}
