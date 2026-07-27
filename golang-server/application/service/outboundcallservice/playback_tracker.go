package outboundcallservice

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// playbackTracker manages the state of audio segments sent to the telephony provider
// and tracks which segments have actually been played to the user.
//
// When an agent generates a long text response, it is segmented into smaller chunks.
// Each chunk is converted to audio, sent to the telephony provider, and marked with
// an index. The telephony provider asynchronously replies with these marks as playback
// completes.
//
// If the user interrupts (barge-in) before the audio finishes playing, playbackTracker
// is used to determine which text segments the user actually heard and which they didn't.
// This allows the system to inform the LLM about the interruption so it doesn't assume
// the user heard the entire response.
type playbackTracker struct {
	segments           []string
	lastDeliveredIndex atomic.Int32
	mu                 sync.RWMutex
}

func newPlaybackTracker() *playbackTracker {
	t := &playbackTracker{}
	t.lastDeliveredIndex.Store(-1) // -1 means nothing delivered yet
	return t
}

func (t *playbackTracker) addSegment(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.segments = append(t.segments, text)
}

func (t *playbackTracker) updateLastDelivered(index int) {
	t.mu.RLock()
	length := len(t.segments)
	t.mu.RUnlock()

	// Bounds check to avoid invalid indices
	if index < 0 || index >= length {
		return
	}

	for {
		current := t.lastDeliveredIndex.Load()
		//nolint:gosec // Safe to convert since index is bounded by array length which fits in int32
		if int32(index) <= current {
			break // Already at or past this index
		}
		//nolint:gosec // Safe to convert since index is bounded by array length which fits in int32
		if t.lastDeliveredIndex.CompareAndSwap(current, int32(index)) {
			break
		}
	}
}

func (t *playbackTracker) getInterruptionContext() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	lastIdx := int(t.lastDeliveredIndex.Load())

	// No segments tracked
	if len(t.segments) == 0 {
		return ""
	}

	// If everything was played, no interruption context needed
	if lastIdx >= len(t.segments)-1 {
		return ""
	}

	// Nothing confirmed as delivered yet — user may not have heard any of it
	if lastIdx == -1 {
		allText := strings.Join(t.segments, " ")
		return fmt.Sprintf("[SYSTEM NOTE: Your previous response was interrupted by the user before they could hear all of it. No part of your response was confirmed as delivered. Your full response was: %q. The user may not have heard any of it. Keep this in mind when responding.]", allText)
	}

	// Partial delivery — some segments confirmed
	delivered := strings.Join(t.segments[:lastIdx+1], " ")
	undelivered := strings.Join(t.segments[lastIdx+1:], " ")

	return fmt.Sprintf("[SYSTEM NOTE: Your previous response was interrupted by the user before they could hear all of it. They heard approximately: %q. They likely did NOT hear: %q. Keep this in mind when responding.]", delivered, undelivered)
}

func (t *playbackTracker) clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.segments = t.segments[:0] // Retain allocated memory but clear the slice
	t.lastDeliveredIndex.Store(-1)
}