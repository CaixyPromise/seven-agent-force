package infrastructure

import (
	"regexp"
	"strings"

	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
)

type RuntimeLogMaskingSupport struct {
	maskedFields   []string
	maxFieldLength int
}

func NewRuntimeLogMaskingSupport(maskedFields []string, maxFieldLength int) *RuntimeLogMaskingSupport {
	normalized := make([]string, 0, len(maskedFields))
	for _, item := range maskedFields {
		value := strings.ToLower(strings.TrimSpace(item))
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return &RuntimeLogMaskingSupport{
		maskedFields:   normalized,
		maxFieldLength: maxFieldLength,
	}
}

func (s *RuntimeLogMaskingSupport) MaskMessage(message string) string {
	if s == nil {
		return message
	}
	masked := message
	for _, field := range s.maskedFields {
		masked = maskLogField(masked, field)
	}
	return jsoncompat.MaskSensitiveText(masked, s.maskedFields, s.maxFieldLength)
}

func maskLogField(message, field string) string {
	if field == "" || message == "" {
		return message
	}
	quoted := regexp.MustCompile(`(?i)("` + regexp.QuoteMeta(field) + `"\s*:\s*")([^"]*)(")`)
	message = quoted.ReplaceAllString(message, `${1}******${3}`)
	raw := regexp.MustCompile(`(?i)(` + regexp.QuoteMeta(field) + `\s*[=:]\s*)([^,\s;]+)`)
	return raw.ReplaceAllString(message, `${1}******`)
}
