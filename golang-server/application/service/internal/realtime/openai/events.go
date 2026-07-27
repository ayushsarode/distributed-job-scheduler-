package openai

import (
	"context"
	"encoding/base64"

	clientTypes "exiro.ai/application/service/internal/types"
	"exiro.ai/application/service/types/entity"
)

func mapOpenAIEvent(eventType string) (entity.RealtimeEventType, bool) {
	switch eventType {
	case "session.created":
		return entity.SessionCreated, true
	case "input_audio_buffer.speech_started":
		return entity.SpeechStarted, true
	case "response.audio.delta":
		return entity.AudioDelta, true
	case "response.audio_transcript.done":
		return entity.AudioTranscriptDone, true
	case "conversation.item.input_audio_transcription.completed":
		return entity.TranscriptionCompleted, true
	case "response.function_call_arguments.done":
		return entity.FunctionCall, true
	case "error":
		return entity.Error, true
	default:
		return entity.Unknown, false
	}
}

// Events returns non-audio events (function calls, transcripts, etc.)
func (c *OpenAIRealtimeClient) Events() <-chan clientTypes.AgentEvent {
	return c.events
}

func (c *OpenAIRealtimeClient) handleSessionCreated() {
	if !c.sessionCreated {
		c.sessionCreated = true
		close(c.sessionReady)
	}
}

func (c *OpenAIRealtimeClient) handleAudioDelta(ctx context.Context, event map[string]any) bool {
	delta, ok := event["delta"].(string)
	if !ok {
		return true
	}

	audioBytes, err := base64.StdEncoding.DecodeString(delta)
	if err != nil {
		return true
	}

	downsampled := resampleAudio(audioBytes, c.audioConfig.OutputSampleRate, c.audioConfig.InputSampleRate)
	select {
	case c.audioIn <- downsampled:
	case <-ctx.Done():
		return false
	default:
	}
	return true
}

func (c *OpenAIRealtimeClient) handleAudioTranscriptDone(ctx context.Context, event map[string]any) bool {
	transcript, ok := event["transcript"].(string)
	if !ok {
		c.logger.Error().Msg("Missing or invalid transcript in transcription event")
		return true
	}

	c.logger.Info().Str("speaker", "AI").Str("text", transcript).Msg("REALTIME SPEAKING")

	select {
	case c.events <- clientTypes.AgentEvent{
		Type:    entity.AudioTranscriptDone,
		Payload: TranscriptionEvent{Transcript: transcript},
	}:
	case <-ctx.Done():
		return false
	default:
	}
	return true
}

func (c *OpenAIRealtimeClient) handleSpeechStarted(ctx context.Context, eventType entity.RealtimeEventType, event map[string]any) bool {
	select {
	case c.events <- clientTypes.AgentEvent{Type: eventType, Payload: event}:
	case <-ctx.Done():
		return false
	default:
	}
	return true
}

func (c *OpenAIRealtimeClient) handleUserTranscription(ctx context.Context, event map[string]any) bool {
	transcript, ok := event["transcript"].(string)
	if !ok {
		c.logger.Error().Ctx(ctx).
			Msg("Missing or invalid transcript in transcription event")
		return true
	}

	c.logger.Debug().Ctx(ctx).
		Str("speaker", "USER").
		Str("text", transcript).
		Msg("User spoke")

	select {
	case c.events <- clientTypes.AgentEvent{
		Type:    entity.TranscriptionCompleted,
		Payload: TranscriptionEvent{Transcript: transcript},
	}:
	case <-ctx.Done():
		return false
	default:
	}
	return true
}

func (c *OpenAIRealtimeClient) handleFunctionCall(ctx context.Context, event map[string]any) bool {
	fnEvent, err := ParseFunctionCallEvent(ctx, event)
	if err != nil {
		c.logger.Error().Ctx(ctx).
			Err(err).
			Msg("Failed to parse function call event")
		return true //
	}

	c.logger.Info().Ctx(ctx).
		Str("function", fnEvent.Name).
		Str("call_id", fnEvent.CallID).
		Msg("Function call received from Realtime API")

	select {
	case c.events <- clientTypes.AgentEvent{
		Type:    entity.FunctionCall,
		Payload: fnEvent,
	}:
	case <-ctx.Done():
		return false
	default:
	}
	return true
}

func (c *OpenAIRealtimeClient) handleError(event map[string]any) {
	if errorData, ok := event["error"].(map[string]any); ok {
		c.logger.Error().Interface("error", errorData).Msg("Realtime API error")
	}
}

func (c *OpenAIRealtimeClient) handleEvent(ctx context.Context, eventType entity.RealtimeEventType, event map[string]any) bool {
	switch eventType {
	case entity.Unknown:
		return true

	case entity.SessionCreated:
		c.handleSessionCreated()

	case entity.AudioDelta:
		return c.handleAudioDelta(ctx, event)

	case entity.AudioTranscriptDone:
		return c.handleAudioTranscriptDone(ctx, event)

	case entity.SpeechStarted:
		return c.handleSpeechStarted(ctx, eventType, event)

	case entity.TranscriptionCompleted:
		c.handleUserTranscription(ctx, event)

	case entity.FunctionCall:
		return c.handleFunctionCall(ctx, event)

	case entity.Error:
		c.handleError(event)
	}

	return true
}
