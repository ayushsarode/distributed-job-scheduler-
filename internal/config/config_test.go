package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_UsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/jobs")
	t.Setenv("HTTP_PORT", "")
	t.Setenv("KAFKA_BROKERS", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("API_KEY", "")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "postgres://user:pass@localhost:5432/jobs", cfg.PostgresDSN)
	assert.Equal(t, 8080, cfg.HTTPPort)
	assert.Equal(t, []string{"localhost:9092"}, cfg.KafkaBrokers)
	assert.Equal(t, "localhost:6379", cfg.RedisAddr)
}

func TestLoad_ParsesEnvOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("KAFKA_BROKERS", "kafka-1:9092,kafka-2:9092")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("API_KEY", "secret")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.HTTPPort)
	assert.Equal(t, []string{"kafka-1:9092", "kafka-2:9092"}, cfg.KafkaBrokers)
	assert.Equal(t, "redis:6379", cfg.RedisAddr)
	assert.Equal(t, "secret", cfg.APIKey)
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DATABASE_URL is required")
}

func TestLoad_InvalidHTTPPort(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db")
	t.Setenv("HTTP_PORT", "not-a-port")

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid HTTP_PORT")
}
