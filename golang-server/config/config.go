package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type AgentServiceConfig struct {
	// URL for the Python/LangGraph agent service (gRPC)
	// Used by lg_ prefixed agents. ws_ and rt_ agents use OPENAI_API_KEY.
	URL string `mapstructure:"url" yaml:"url" validate:"required"`
}

type TTSConfig struct {
	Provider string `mapstructure:"provider" validate:"required,oneof=elevenlabs deepgram google sarvam"`
}

type STTConfig struct {
	Provider string `mapstructure:"provider" validate:"required,oneof=deepgram sarvam"`
}

type RealtimeConfig struct {
	Provider string `mapstructure:"provider" validate:"required,oneof=openai claude"`
}

type OutboundCallingConfig struct {
	Enabled                  bool   `mapstructure:"enabled"`
	Provider                 string `mapstructure:"provider" validate:"required_if=Enabled true,oneof=twilio exotel"`
	BaseURL                  string `mapstructure:"base_url" validate:"required_if=Enabled true"`                 // Base URL for webhooks and WebSocket connections
	ExotelFromPhoneNumber    string `mapstructure:"exotel_from_phone_number" validate:"required_if=Enabled true"` // Phone number to make calls from
	TwilioFromPhoneNumber    string `mapstructure:"twilio_from_phone_number" validate:"required_if=Enabled true"` // Phone number to make calls from
	MaxCallWaitTimeMinutes   int    `mapstructure:"max_call_wait_time_minutes" default:"5"`                       // Maximum time to wait for a call to complete
	PollIntervalSeconds      int    `mapstructure:"poll_interval_seconds" default:"5"`                            // How often to check call status
	DelayBetweenCallsSeconds int    `mapstructure:"delay_between_calls_seconds" default:"2"`                      // Delay between sequential calls
	CampaignDocumentBucket   string `mapstructure:"campaign_document_bucket" validate:"required_if=Enabled true"` // S3 bucket for campaign documents
	ExotelVoicebotAppId      string `mapstructure:"exotel_voicebot_app_id" validate:"required_if=Enabled true"`
}

type SweeperConfig struct {
	SweepIntervalMinutes  int `mapstructure:"sweep_interval_minutes" validate:"min=0"`
	StaleThresholdMinutes int `mapstructure:"stale_threshold_minutes" validate:"min=0"`
}

// Default values for sweeper configuration.
const (
	DefaultSweepIntervalMinutes  = 10
	DefaultStaleThresholdMinutes = 60
)

type TranscriptionStorageConfig struct {
	TranscriptionTableName string        `mapstructure:"transcriptions_table_name" validate:"required"`
	AgentKVStoreQueueName  string        `mapstructure:"agent_kv_store_queue_name" validate:"required"`
	AgentKVStoreTableName  string        `mapstructure:"agent_kv_store_table_name" validate:"required"`
	UseLocalStack          bool          `mapstructure:"use_localstack"`
	TranscriptsQueueName   string        `mapstructure:"transcripts_queue_name" validate:"required"`
	S3BucketName           string        `mapstructure:"s3_bucket_name" validate:"required"`
	Sweeper                SweeperConfig `mapstructure:"sweeper"`
}

type CredentialServiceConfig struct {
	AWSKMSKeyId string `mapstructure:"aws_kms_key_id"`
}

type Config struct {
	ProductionMode bool   `mapstructure:"production_mode"`
	TestMode       bool   `mapstructure:"test_mode"`
	TestUserID     string `mapstructure:"test_user_id"`

	HttpPort           string `mapstructure:"http_port" validate:"required"`
	DatabaseURL        string `mapstructure:"database_url" validate:"required"`
	DBOSDatabaseURL    string `mapstructure:"dbos_database_url" validate:"required"`
	ObjectStoreType    string `mapstructure:"object_store_type" validate:"required,oneof=S3 LocalStack GCS"`
	MessageQueueType   string `mapstructure:"message_queue_type" validate:"required,oneof=SQS LocalStack"`
	ConsumerQueueType  string `mapstructure:"message_queue_type" validate:"required,oneof=SQS LocalStack"`
	LocalStackEndpoint string `mapstructure:"localstack_endpoint" validate:"required_if=ObjectStoreType LocalStack"`
	DocumentsBucket    string `mapstructure:"documents_bucket" validate:"required"`

	// DynamoDB
	TranscriptionStorage TranscriptionStorageConfig `mapstructure:"transcription_storage" validate:"required"`

	// AWS S3 Configuration
	AWSRegion          string `mapstructure:"aws_region" validate:"required_if=ObjectStoreType S3"`
	AWSAccessKeyID     string `mapstructure:"aws_access_key_id"`     // Optional when using IAM roles
	AWSSecretAccessKey string `mapstructure:"aws_secret_access_key"` // Optional when using IAM roles

	// AgentService Configuration
	AgentService AgentServiceConfig `mapstructure:"agent_service" validate:"required"`

	// TTS Configuration
	TTS TTSConfig `mapstructure:"tts" validate:"required"`

	STT STTConfig `mapstructure:"stt" validate:"required"`

	// Realtime Configuration
	Realtime RealtimeConfig `mapstructure:"realtime" validate:"required"`

	// Outbound Calling Configuration
	OutboundCalling OutboundCallingConfig `mapstructure:"outbound_calling"`

	AgentDeploymentSQSQueueName      string `mapstructure:"agent_deployment_sqs_queue_name"`
	AgentDeploymentCheckSQSQueueName string `mapstructure:"agent_deployment_check_queue_name"`

	// Credential Configuration
	CredentialService CredentialServiceConfig `mapstructure:"credential_service" validate:"required_if=ProductionMode true"`
}

// ObjectStore Types.
const (
	S3         = "S3"
	LocalStack = "LocalStack"
	GCS        = "GCS"
	DynamoDB   = "DynamoDB"
)

// Realtime types.
const (
	OpenAI = "openai"
	Claude = "Claude"
)

type configContextKey struct{}

var ctxKey = configContextKey{}

// setDefaultValues applies default values to config fields that are not set.
func setDefaultValues(config *Config) {
	// Apply default values for sweeper config if not set
	if config.TranscriptionStorage.Sweeper.SweepIntervalMinutes <= 0 {
		config.TranscriptionStorage.Sweeper.SweepIntervalMinutes = DefaultSweepIntervalMinutes
	}
	if config.TranscriptionStorage.Sweeper.StaleThresholdMinutes <= 0 {
		config.TranscriptionStorage.Sweeper.StaleThresholdMinutes = DefaultStaleThresholdMinutes
	}
}

func validateConfig(config *Config) error {
	validate := validator.New()

	if err := validate.Struct(config); err != nil {
		// Type assert err to use validator.ValidationErrors
		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			errorMessages := make([]string, 0, len(validationErrors))
			for _, e := range validationErrors {
				errorMessages = append(errorMessages, formatValidationError(e))
			}
			return fmt.Errorf("config validation failed: %s", strings.Join(errorMessages, "; "))
		}
		return fmt.Errorf("config validation failed: %w", err)
	}

	return nil
}

func formatValidationError(e validator.FieldError) string {
	field := e.Namespace() // Full path like "Config.TTS.Provider"
	// Remove the "Config." prefix for cleaner output
	field = strings.TrimPrefix(field, "Config.")

	currentValue := e.Value()
	tag := e.Tag()
	param := e.Param()

	switch tag {
	case "oneof":
		allowedValues := strings.ReplaceAll(param, " ", ", ")
		return fmt.Sprintf("field '%s': got '%v', must be one of [%s]", field, currentValue, allowedValues)
	case "required":
		return fmt.Sprintf("field '%s': required but not set", field)
	case "required_if":
		return fmt.Sprintf("field '%s': required (condition: %s) but got '%v'", field, param, currentValue)
	case "min":
		return fmt.Sprintf("field '%s': got '%v', must be at least %s", field, currentValue, param)
	case "max":
		return fmt.Sprintf("field '%s': got '%v', must be at most %s", field, currentValue, param)
	default:
		if param != "" {
			return fmt.Sprintf("field '%s': got '%v', failed validation '%s=%s'", field, currentValue, tag, param)
		}
		return fmt.Sprintf("field '%s': got '%v', failed validation '%s'", field, currentValue, tag)
	}
}

func New() (*Config, error) {
	var config Config
	v := viper.New()
	v.SetConfigFile("config.yaml")
	v.AddConfigPath(".")
	v.AutomaticEnv()
	v.SetEnvPrefix("")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	err := v.Unmarshal(&config)
	if err != nil {
		return nil, err
	}

	setDefaultValues(&config)

	err = validateConfig(&config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func WithContext(ctx context.Context, cfg *Config) context.Context {
	return context.WithValue(ctx, ctxKey, cfg)
}

func Ctx(ctx context.Context) *Config {
	cfg := ctx.Value(ctxKey)
	return cfg.(*Config)
}
