package provider

import (
	"fmt"
	"strings"
)

func payloadString(payload map[string]any, keys ...string) string {
	if payload == nil {
		return ""
	}
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		if text := valueString(value); text != "" {
			return text
		}
	}
	return ""
}

func valueString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}
