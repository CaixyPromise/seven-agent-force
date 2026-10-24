package json

import (
	stdjson "encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

const maskedValue = "******"

var (
	jsonMarshalerType = reflect.TypeOf((*stdjson.Marshaler)(nil)).Elem()
	timeType          = reflect.TypeOf(time.Time{})
)

func Normalize(v any) any {
	return NormalizeForJSON(v)
}

func NormalizeForJSON(v any) any {
	return normalizeValue(reflect.ValueOf(v))
}

func MaskSensitiveFields(v any, maskedFields []string, maxFieldLength int) any {
	normalized := NormalizeForJSON(v)
	matchers := normalizeMatchers(maskedFields)
	return sanitizeValue(normalized, matchers, maxFieldLength, "")
}

func MaskSensitiveText(value string, maskedFields []string, maxFieldLength int) string {
	masked := value
	for _, marker := range normalizeMatchers(maskedFields) {
		if marker == "" || isVisibleSensitiveMetadataField(marker) {
			continue
		}
		masked = maskTextMarker(masked, marker)
	}
	return clipString(masked, maxFieldLength)
}

func ClipLargePayload(payload []byte, maxBytes int) ([]byte, bool) {
	if maxBytes <= 0 || len(payload) <= maxBytes {
		return payload, false
	}
	return payload[:maxBytes], true
}

func normalizeValue(value reflect.Value) any {
	if !value.IsValid() {
		return nil
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}

	if value.Type() == timeType {
		return value.Interface()
	}
	if value.CanInterface() && value.Type().Implements(jsonMarshalerType) {
		return value.Interface()
	}

	switch value.Kind() {
	case reflect.Int64:
		return strconvFormatInt(value.Int())
	case reflect.Uint64:
		return strconvFormatUint(value.Uint())
	case reflect.Slice, reflect.Array:
		result := make([]any, value.Len())
		for i := 0; i < value.Len(); i++ {
			result[i] = normalizeValue(value.Index(i))
		}
		return result
	case reflect.Map:
		result := make(map[string]any, value.Len())
		iter := value.MapRange()
		for iter.Next() {
			result[mapKeyString(iter.Key())] = normalizeValue(iter.Value())
		}
		return result
	case reflect.Struct:
		result := make(map[string]any, value.NumField())
		valueType := value.Type()
		for i := 0; i < value.NumField(); i++ {
			field := valueType.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name, omitEmpty, skip := jsonFieldName(field)
			if skip {
				continue
			}
			fieldValue := value.Field(i)
			if omitEmpty && isEmptyValue(fieldValue) {
				continue
			}
			result[name] = normalizeValue(fieldValue)
		}
		return result
	default:
		if value.CanInterface() {
			return value.Interface()
		}
		return nil
	}
}

func sanitizeValue(value any, maskedFields []string, maxFieldLength int, fieldName string) any {
	if fieldName != "" && isSensitiveField(fieldName, maskedFields) {
		return maskedValue
	}

	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		_, hasKeyValue := sensitiveKeyValueName(typed, maskedFields)
		for key, child := range typed {
			if hasKeyValue && strings.EqualFold(strings.TrimSpace(key), "value") {
				result[key] = maskedValue
				continue
			}
			result[key] = sanitizeValue(child, maskedFields, maxFieldLength, key)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i := range typed {
			result[i] = sanitizeValue(typed[i], maskedFields, maxFieldLength, fieldName)
		}
		return result
	case string:
		return MaskSensitiveText(typed, maskedFields, maxFieldLength)
	default:
		return value
	}
}

func normalizeMatchers(maskedFields []string) []string {
	defaults := []string{
		"password",
		"passwd",
		"pwd",
		"token",
		"secret",
		"credential",
		"authorization",
		"bearer",
		"cookie",
		"set-cookie",
		"clientsecret",
		"client_secret",
		"secretciphertext",
		"secret_ciphertext",
		"passwordhash",
		"password_hash",
		"privatekey",
		"private_key",
		"apikey",
		"api_key",
		"accesskey",
		"access_key",
		"configvalue",
		"config_value",
		"issensitive",
		"is_sensitive",
		"authorizationcode",
		"authorization_code",
		"authcode",
		"auth_code",
		"oauthcode",
		"oauth_code",
		"codeverifier",
		"code_verifier",
		"state",
		"oauthstate",
		"oauth_state",
		"externalstate",
		"external_state",
		"oidcstate",
		"oidc_state",
		"nonce",
		"oidcnonce",
		"oidc_nonce",
	}
	result := make([]string, 0, len(defaults)+len(maskedFields))
	seen := map[string]bool{}
	for _, field := range defaults {
		seen[field] = true
		result = append(result, field)
	}
	for _, field := range maskedFields {
		normalized := strings.ToLower(strings.TrimSpace(field))
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	return result
}

func isSensitiveField(field string, maskedFields []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(field))
	if isVisibleSensitiveMetadataField(normalized) {
		return false
	}
	for _, matcher := range maskedFields {
		if strings.Contains(normalized, matcher) {
			return true
		}
	}
	return false
}

func isVisibleSensitiveMetadataField(normalized string) bool {
	return normalized == "key" || normalized == "configkey" || normalized == "config_key"
}

func sensitiveKeyValueName(items map[string]any, maskedFields []string) (string, bool) {
	for _, keyField := range []string{"key", "configKey", "config_key", "name"} {
		raw, ok := items[keyField]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			continue
		}
		if isSensitiveField(value, maskedFields) {
			return value, true
		}
	}
	return "", false
}

func maskTextMarker(value, marker string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(^|[{\s,;])(` + regexp.QuoteMeta(marker) + `\s*[:=]\s*)("[^"]*"|'[^']*'|[^,\s;}\]]+)`),
		regexp.MustCompile(`(?i)("` + regexp.QuoteMeta(marker) + `"\s*:\s*)("[^"]*"|[0-9]+|true|false|null)`),
	}
	masked := value
	for _, pattern := range patterns {
		masked = pattern.ReplaceAllStringFunc(masked, func(match string) string {
			groups := pattern.FindStringSubmatch(match)
			if len(groups) == 4 {
				return groups[1] + groups[2] + "******"
			}
			if len(groups) == 3 {
				return groups[1] + "******"
			}
			return match
		})
	}
	return masked
}

func clipString(value string, maxFieldLength int) string {
	if maxFieldLength <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxFieldLength {
		return value
	}
	return string(runes[:maxFieldLength]) + "...(truncated)"
}

func jsonFieldName(field reflect.StructField) (string, bool, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, true
	}
	if tag == "" {
		return field.Name, false, false
	}
	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = field.Name
	}
	omitEmpty := false
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func isEmptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool:
		return !value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return value.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return value.IsNil()
	case reflect.Struct:
		return false
	default:
		return false
	}
}

func mapKeyString(key reflect.Value) string {
	if key.Kind() == reflect.String {
		return key.String()
	}
	if key.CanInterface() {
		return fmt.Sprint(key.Interface())
	}
	return ""
}

func strconvFormatInt(v int64) string {
	return fmt.Sprintf("%d", v)
}

func strconvFormatUint(v uint64) string {
	return fmt.Sprintf("%d", v)
}
