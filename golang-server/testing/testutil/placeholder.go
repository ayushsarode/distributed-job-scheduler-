package testutil

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var placeholderRegex = regexp.MustCompile(`<([^:]+):([^>]+)>`)

const resolvePatternLength = 3

// PlaceholderResolver resolves placeholder patterns like <uuid:fieldName> in test data.
type PlaceholderResolver struct {
	cache      map[string]string
	testUserID string
}

func NewPlaceholderResolver() *PlaceholderResolver {
	return &PlaceholderResolver{cache: make(map[string]string)}
}

func (r *PlaceholderResolver) SetTestUserID(testUserID string) {
	r.testUserID = testUserID
}

func (r *PlaceholderResolver) Clear() {
	clear(r.cache)
}

// Resolve resolves all placeholder patterns in a string.
func (r *PlaceholderResolver) Resolve(input string) (string, error) {
	var lastErr error
	result := placeholderRegex.ReplaceAllStringFunc(input, func(match string) string {
		if value, exists := r.cache[match]; exists {
			return value
		}

		matches := placeholderRegex.FindStringSubmatch(match)
		if len(matches) != resolvePatternLength {
			lastErr = fmt.Errorf("invalid pattern: %s", match)
			return match
		}

		value, err := r.generate(matches[1], matches[2])
		if err != nil {
			lastErr = err
			return match
		}

		r.cache[match] = value
		return value
	})
	return result, lastErr
}

func (r *PlaceholderResolver) generate(resolverType, key string) (string, error) {
	switch resolverType {
	case "uuid":
		return uuid.Must(uuid.NewV7()).String(), nil
	case "smallCase":
		return r.generateSmallCase(key), nil
	case "testUserId":
		return r.testUserID, nil
	default:
		return "", fmt.Errorf("unknown resolver type: %s", resolverType)
	}
}

func (r *PlaceholderResolver) generateSmallCase(_ string) string {
	return strings.ReplaceAll(uuid.Must(uuid.NewV7()).String(), "-", "")[:10]
}
