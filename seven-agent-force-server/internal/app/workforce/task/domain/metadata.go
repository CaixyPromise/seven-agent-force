package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrTaskNameInvalid      = errors.New("TASK_NAME_INVALID")
	ErrTaskTagInvalid       = errors.New("TASK_TAG_INVALID")
	ErrTaskTagLimitExceeded = errors.New("TASK_TAG_LIMIT_EXCEEDED")
	ErrTaskMetadataConflict = errors.New("TASK_METADATA_CONFLICT")
)

// NormalizeTaskMetadata is the single authority for Task display metadata.
// Display metadata deliberately remains outside every execution identity.
func NormalizeTaskMetadata(name, instruction string, tags []string) (string, []string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = suggestedTaskName(instruction)
	}
	if utf8.RuneCountInString(name) == 0 || utf8.RuneCountInString(name) > 120 || hasControlOrLineBreak(name) {
		return "", nil, ErrTaskNameInvalid
	}
	if len(tags) > 10 {
		return "", nil, ErrTaskTagLimitExceeded
	}
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(norm.NFKC.String(tag)))
		if tag == "" || utf8.RuneCountInString(tag) > 32 || hasControlOrLineBreak(tag) {
			return "", nil, ErrTaskTagInvalid
		}
		if _, exists := seen[tag]; exists {
			return "", nil, ErrTaskTagInvalid
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return name, normalized, nil
}

func suggestedTaskName(instruction string) string {
	for _, line := range strings.Split(strings.ReplaceAll(instruction, "\r\n", "\n"), "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > 80 {
			runes = runes[:80]
		}
		return string(runes)
	}
	return ""
}

func hasControlOrLineBreak(value string) bool {
	for _, r := range value {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}
