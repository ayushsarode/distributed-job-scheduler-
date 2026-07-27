package text

import (
	"regexp"
	"strings"
)

// Configuration constants for text segmentation.
const (
	// MinSegmentLength is the minimum length for a text segment to be processed separately.
	MinSegmentLength = 50
	// MaxSegmentLength is the maximum length for a text segment to prevent overly long audio generation.
	MaxSegmentLength = 300
	// SmallTextThreshold defines what we consider "small text" that shouldn't be segmented.
	SmallTextThreshold = 100
)

// Segmenter handles text segmentation operations.
type Segmenter struct {
	sentenceRegex *regexp.Regexp
	clauseRegex   *regexp.Regexp
}

// NewSegmenter creates a new text segmenter with default sentence boundary regex.
func NewSegmenter() *Segmenter {
	// 1. Sentence endings: [.!?।]+\s+
	//    - Matches one or more sentence-ending punctuation marks followed by whitespace
	//    - Includes period (.), exclamation (!), question (?), and Hindi full stop (।)
	// 2. Double line breaks (paragraph breaks): \n\s*\n
	//    - Matches newline, optional whitespace, then another newline
	//    - Handles paragraph breaks and formatted text like poetry/markdown
	//
	return &Segmenter{
		sentenceRegex: regexp.MustCompile(`(?:[.!?।]+\s+)|(?:\n\s*\n)`),
		// Clause regex: [,;:]\s+
		// - Matches comma (,), semicolon (;), or colon (:) followed by one or more whitespace
		// - Used as fallback when sentence boundaries aren't clear
		// - Helps split long text at natural pause points like clauses and phrases
		clauseRegex: regexp.MustCompile(`[,;:]\s+`),
	}
}

// SegmentText intelligently breaks text into smaller segments for parallel audio generation
// It prioritizes natural language boundaries while respecting size constraints.
func (s *Segmenter) SegmentText(text string) []string {
	// Clean and normalize the text first
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{}
	}

	// Handle edge case: very short text doesn't need segmentation
	if len(text) <= SmallTextThreshold {
		return []string{text}
	}

	// First, try to split by sentence boundaries (periods, exclamation marks, question marks)
	sentences := s.sentenceRegex.Split(text, -1)
	segments := s.buildSegmentsFromSentences(text, sentences)

	// If no good sentence boundaries found, fall back to clause-based splitting
	if len(segments) <= 1 && len(text) > MaxSegmentLength {
		return s.segmentByClausesAndPhrases(text)
	}

	return s.validateAndOptimizeSegments(segments)
}

func (s *Segmenter) buildSegmentsFromSentences(text string, sentences []string) []string {
	var segments []string
	currentSegment := ""

	for i, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		// Add sentence ending punctuation back (except for the last sentence)
		sentence = s.addPunctuationBack(text, sentences, i, sentence)

		potentialSegment := currentSegment
		if potentialSegment != "" {
			potentialSegment += " "
		}
		potentialSegment += sentence

		// If adding this sentence would exceed max length, finalize current segment
		if len(potentialSegment) > MaxSegmentLength && currentSegment != "" {
			segments = append(segments, strings.TrimSpace(currentSegment))
			currentSegment = sentence
		} else {
			currentSegment = potentialSegment
		}

		// If current segment is long enough and we have more content, finalize it
		if len(currentSegment) >= MinSegmentLength && i < len(sentences)-1 {
			segments = append(segments, strings.TrimSpace(currentSegment))
			currentSegment = ""
		}
	}

	// Add any remaining content
	if currentSegment != "" {
		segments = append(segments, strings.TrimSpace(currentSegment))
	}

	return segments
}

func (s *Segmenter) addPunctuationBack(text string, sentences []string, i int, sentence string) string {
	if i < len(sentences)-1 {
		// Find the original punctuation that was used to split
		remainingText := text[len(strings.Join(sentences[:i+1], "")):]
		if match := s.sentenceRegex.FindString(remainingText); match != "" {
			sentence += strings.TrimSpace(match)
		}
	}
	return sentence
}

// segmentByClausesAndPhrases handles text that doesn't have clear sentence boundaries.
func (s *Segmenter) segmentByClausesAndPhrases(text string) []string {
	// Split by common clause separators using pre-compiled regex
	clauses := s.clauseRegex.Split(text, -1)

	var segments []string
	currentSegment := ""

	for i, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}

		// Add punctuation back
		if i < len(clauses)-1 {
			clause += ","
		}

		potentialSegment := currentSegment
		if potentialSegment != "" {
			potentialSegment += " "
		}
		potentialSegment += clause

		if len(potentialSegment) > MaxSegmentLength && currentSegment != "" {
			segments = append(segments, strings.TrimSpace(currentSegment))
			currentSegment = clause
		} else {
			currentSegment = potentialSegment
		}
	}

	if currentSegment != "" {
		segments = append(segments, strings.TrimSpace(currentSegment))
	}

	return segments
}

// validateAndOptimizeSegments ensures segments meet quality criteria.
func (s *Segmenter) validateAndOptimizeSegments(segments []string) []string {
	optimized := make([]string, 0, len(segments))

	for i, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		// Merge very short segments with adjacent ones
		if len(segment) < MinSegmentLength && i < len(segments)-1 {
			nextSegment := strings.TrimSpace(segments[i+1])
			if len(segment+" "+nextSegment) <= MaxSegmentLength {
				segments[i+1] = segment + " " + nextSegment
				continue
			}
		}

		optimized = append(optimized, segment)
	}

	return optimized
}
