// In openai/options.go
package openai

import "exiro.ai/application/service/internal/types"

const (
	defaultInputSampleRate  = 8000
	defaultOutputSampleRate = 24000
)

// WithAudioConfig sets the audio configuration.
func WithAudioConfig(config types.AudioConfig) types.RealtimeConnectOption {
	return func(opts *types.RealtimeConnectOptions) {
		opts.AudioConfig = config
	}
}

// WithSampleRates sets input and output sample rates.
func WithSampleRates(inputRate, outputRate int) types.RealtimeConnectOption {
	return func(opts *types.RealtimeConnectOptions) {
		opts.AudioConfig.InputSampleRate = inputRate
		opts.AudioConfig.OutputSampleRate = outputRate
	}
}

func WithAudioFormat(audioFormat string) types.RealtimeConnectOption {
	return func(opts *types.RealtimeConnectOptions) {
		opts.AudioConfig.AudioFormat = audioFormat
	}
}

func DefaultAudioConfig() types.AudioConfig {
	return types.AudioConfig{
		InputSampleRate:  defaultInputSampleRate,
		OutputSampleRate: defaultOutputSampleRate,
		AudioFormat:      "pcm16",
	}
}