package text

import (
	"testing"
)

func TestSegmentText_ShortText(t *testing.T) {
	segmenter := NewSegmenter()
	text := "Hello world!"

	segments := segmenter.SegmentText(text)

	if len(segments) != 1 {
		t.Errorf("Expected 1 segment for short text, got %d", len(segments))
	}

	if segments[0] != text {
		t.Errorf("Expected segment to be '%s', got '%s'", text, segments[0])
	}
}

func TestSegmentText_LongTextWithSentences(t *testing.T) {
	segmenter := NewSegmenter()
	text := "This is the first sentence. This is the second sentence that is quite long and should be in its own segment. This is the third sentence. And this is the fourth sentence that completes our test."

	segments := segmenter.SegmentText(text)

	if len(segments) == 0 {
		t.Error("Expected at least 1 segment, got 0")
	}

	// Verify all segments are within reasonable length bounds
	for i, segment := range segments {
		if len(segment) > MaxSegmentLength {
			t.Errorf("Segment %d exceeds max length: %d > %d", i, len(segment), MaxSegmentLength)
		}
	}
}

func TestSegmentText_EmptyText(t *testing.T) {
	segmenter := NewSegmenter()
	text := ""

	segments := segmenter.SegmentText(text)

	if len(segments) != 0 {
		t.Errorf("Expected 0 segments for empty text, got %d", len(segments))
	}
}

func TestSegmentText_TextWithoutSentenceBoundaries(t *testing.T) {
	segmenter := NewSegmenter()
	// Create a long text without sentence boundaries to test clause-based splitting
	text := "This is a very long text that does not have proper sentence boundaries, it just keeps going with commas, semicolons; and colons: making it challenging to segment properly, but our algorithm should handle it gracefully by falling back to clause-based segmentation, which should work reasonably well for most cases"

	segments := segmenter.SegmentText(text)

	if len(segments) == 0 {
		t.Error("Expected at least 1 segment, got 0")
	}

	// Verify segments are reasonable
	for i, segment := range segments {
		if len(segment) > MaxSegmentLength {
			t.Errorf("Segment %d exceeds max length: %d > %d", i, len(segment), MaxSegmentLength)
		}
	}
}
