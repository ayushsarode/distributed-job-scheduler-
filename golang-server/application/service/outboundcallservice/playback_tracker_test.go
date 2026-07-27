package outboundcallservice

import (
	"testing"
)

func TestPlaybackTracker_getInterruptionContext(t *testing.T) {
	tests := []struct {
		name     string
		segments []string
		lastIdx  int
		want     string
	}{
		{
			name:     "empty segments",
			segments: []string{},
			lastIdx:  -1,
			want:     "",
		},
		{
			name:     "no segments delivered",
			segments: []string{"segment 1", "segment 2"},
			lastIdx:  -1,
			want:     `[SYSTEM NOTE: Your previous response was interrupted by the user before they could hear all of it. No part of your response was confirmed as delivered. Your full response was: "segment 1 segment 2". The user may not have heard any of it. Keep this in mind when responding.]`,
		},
		{
			name:     "single segment no delivery confirmation",
			segments: []string{"Hello, how can I help?"},
			lastIdx:  -1,
			want:     `[SYSTEM NOTE: Your previous response was interrupted by the user before they could hear all of it. No part of your response was confirmed as delivered. Your full response was: "Hello, how can I help?". The user may not have heard any of it. Keep this in mind when responding.]`,
		},
		{
			name:     "all segments delivered",
			segments: []string{"segment 1", "segment 2"},
			lastIdx:  1,
			want:     "",
		},
		{
			name:     "partial delivery",
			segments: []string{"Hello there.", "My name is Claude.", "How can I help you today?"},
			lastIdx:  0,
			want:     "[SYSTEM NOTE: Your previous response was interrupted by the user before they could hear all of it. They heard approximately: \"Hello there.\". They likely did NOT hear: \"My name is Claude. How can I help you today?\". Keep this in mind when responding.]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracker := newPlaybackTracker()
			for _, seg := range tt.segments {
				tracker.addSegment(seg)
			}
			tracker.updateLastDelivered(tt.lastIdx)

			if got := tracker.getInterruptionContext(); got != tt.want {
				t.Errorf("playbackTracker.getInterruptionContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPlaybackTracker_clear(t *testing.T) {
	tracker := newPlaybackTracker()
	tracker.addSegment("segment 1")
	tracker.addSegment("segment 2")
	tracker.updateLastDelivered(0)

	if len(tracker.segments) != 2 {
		t.Errorf("Expected 2 segments, got %d", len(tracker.segments))
	}
	if tracker.lastDeliveredIndex.Load() != 0 {
		t.Errorf("Expected lastDeliveredIndex to be 0, got %d", tracker.lastDeliveredIndex.Load())
	}

	tracker.clear()

	if len(tracker.segments) != 0 {
		t.Errorf("Expected 0 segments after clear, got %d", len(tracker.segments))
	}
	if tracker.lastDeliveredIndex.Load() != -1 {
		t.Errorf("Expected lastDeliveredIndex to be -1 after clear, got %d", tracker.lastDeliveredIndex.Load())
	}
}
