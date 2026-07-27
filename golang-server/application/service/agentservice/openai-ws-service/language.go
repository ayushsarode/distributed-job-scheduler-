package openaiwsservice

import (
	"errors"

	"exiro.ai/application/service/types"
)

// ParseLanguage converts a string from the set_language tool to AgentLanguage.
func ParseLanguage(s string) (types.AgentLanguage, error) {
	switch s {
	case "english":
		return types.AgentLanguageEnglish, nil
	case "hindi":
		return types.AgentLanguageHindi, nil
	default:
		return types.AgentLanguageEnglish, errors.New("unsupported language: " + s)
	}
}

// ValidateLanguage returns true if the string is a supported language value.
func ValidateLanguage(s string) bool {
	return s == "english" || s == "hindi"
}
