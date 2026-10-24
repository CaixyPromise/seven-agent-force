package json

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
)

var (
	jsonUnmarshalerType = reflect.TypeOf((*stdjson.Unmarshaler)(nil)).Elem()
	textUnmarshalerType = reflect.TypeOf((*interface{ UnmarshalText([]byte) error })(nil)).Elem()
)

// Unmarshal applies the business JSON wire contract before decoding into target.
// Quoted base-10 values are accepted for int64 and uint64 fields while native
// JSON numbers continue to use the standard encoding/json semantics.
func Unmarshal(data []byte, target any) error {
	normalized, changed, err := normalizeQuotedInt64(data, reflect.TypeOf(target), false)
	if err != nil {
		return err
	}
	if changed {
		data = normalized
	}
	return sonic.Unmarshal(data, target)
}

func normalizeQuotedInt64(data []byte, targetType reflect.Type, stringTagged bool) ([]byte, bool, error) {
	if targetType == nil || preservesCustomJSON(targetType) {
		return data, false, nil
	}
	for targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
		if preservesCustomJSON(targetType) {
			return data, false, nil
		}
	}

	trimmed := bytes.TrimSpace(data)
	if bytes.Equal(trimmed, []byte("null")) {
		return data, false, nil
	}

	switch targetType.Kind() {
	case reflect.Int64:
		if stringTagged {
			return data, false, nil
		}
		return normalizeQuotedSignedInteger(trimmed, targetType)
	case reflect.Uint64:
		if stringTagged {
			return data, false, nil
		}
		return normalizeQuotedUnsignedInteger(trimmed, targetType)
	case reflect.Struct:
		return normalizeStructInt64(trimmed, targetType)
	case reflect.Slice, reflect.Array:
		return normalizeSliceInt64(trimmed, targetType.Elem())
	case reflect.Map:
		if targetType.Key().Kind() != reflect.String {
			return data, false, nil
		}
		return normalizeMapInt64(trimmed, targetType.Elem())
	default:
		return data, false, nil
	}
}

func normalizeQuotedSignedInteger(data []byte, targetType reflect.Type) ([]byte, bool, error) {
	if len(data) == 0 || data[0] != '"' {
		return data, false, nil
	}
	var text string
	if err := stdjson.Unmarshal(data, &text); err != nil {
		return nil, false, err
	}
	if text == "" || text != strings.TrimSpace(text) || strings.HasPrefix(text, "+") {
		return nil, false, fmt.Errorf("json: invalid quoted %s value %q", targetType, text)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return nil, false, fmt.Errorf("json: invalid quoted %s value %q: %w", targetType, text, err)
	}
	return []byte(strconv.FormatInt(value, 10)), true, nil
}

func normalizeQuotedUnsignedInteger(data []byte, targetType reflect.Type) ([]byte, bool, error) {
	if len(data) == 0 || data[0] != '"' {
		return data, false, nil
	}
	var text string
	if err := stdjson.Unmarshal(data, &text); err != nil {
		return nil, false, err
	}
	if text == "" || text != strings.TrimSpace(text) || strings.HasPrefix(text, "+") || strings.HasPrefix(text, "-") {
		return nil, false, fmt.Errorf("json: invalid quoted %s value %q", targetType, text)
	}
	value, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		return nil, false, fmt.Errorf("json: invalid quoted %s value %q: %w", targetType, text, err)
	}
	return []byte(strconv.FormatUint(value, 10)), true, nil
}

func normalizeStructInt64(data []byte, targetType reflect.Type) ([]byte, bool, error) {
	var fields map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(data, &fields); err != nil {
		return data, false, nil
	}
	fieldTypes := collectJSONFieldTypes(targetType)
	changed := false
	for name, raw := range fields {
		field, ok := fieldTypes[name]
		if !ok {
			field, ok = fieldTypes[strings.ToLower(name)]
		}
		if !ok {
			continue
		}
		normalized, fieldChanged, err := normalizeQuotedInt64(raw, field.targetType, field.stringTagged)
		if err != nil {
			return nil, false, fmt.Errorf("json field %q: %w", name, err)
		}
		if fieldChanged {
			fields[name] = normalized
			changed = true
		}
	}
	if !changed {
		return data, false, nil
	}
	normalized, err := stdjson.Marshal(fields)
	return normalized, true, err
}

func normalizeSliceInt64(data []byte, elementType reflect.Type) ([]byte, bool, error) {
	var items []stdjson.RawMessage
	if err := stdjson.Unmarshal(data, &items); err != nil {
		return data, false, nil
	}
	changed := false
	for index, raw := range items {
		normalized, itemChanged, err := normalizeQuotedInt64(raw, elementType, false)
		if err != nil {
			return nil, false, fmt.Errorf("json array item %d: %w", index, err)
		}
		if itemChanged {
			items[index] = normalized
			changed = true
		}
	}
	if !changed {
		return data, false, nil
	}
	normalized, err := stdjson.Marshal(items)
	return normalized, true, err
}

func normalizeMapInt64(data []byte, elementType reflect.Type) ([]byte, bool, error) {
	var items map[string]stdjson.RawMessage
	if err := stdjson.Unmarshal(data, &items); err != nil {
		return data, false, nil
	}
	changed := false
	for key, raw := range items {
		normalized, itemChanged, err := normalizeQuotedInt64(raw, elementType, false)
		if err != nil {
			return nil, false, fmt.Errorf("json map value %q: %w", key, err)
		}
		if itemChanged {
			items[key] = normalized
			changed = true
		}
	}
	if !changed {
		return data, false, nil
	}
	normalized, err := stdjson.Marshal(items)
	return normalized, true, err
}

type jsonFieldType struct {
	targetType   reflect.Type
	stringTagged bool
}

func collectJSONFieldTypes(targetType reflect.Type) map[string]jsonFieldType {
	result := make(map[string]jsonFieldType)
	for _, field := range reflect.VisibleFields(targetType) {
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		parts := strings.Split(tag, ",")
		if len(parts) > 0 && parts[0] == "-" {
			continue
		}
		explicitName := len(parts) > 0 && parts[0] != ""
		if field.Anonymous && !explicitName && isStructType(field.Type) {
			continue
		}
		name := field.Name
		if explicitName {
			name = parts[0]
		}
		metadata := jsonFieldType{targetType: field.Type, stringTagged: containsTagOption(parts[1:], "string")}
		result[name] = metadata
		result[strings.ToLower(name)] = metadata
	}
	return result
}

func containsTagOption(options []string, expected string) bool {
	for _, option := range options {
		if option == expected {
			return true
		}
	}
	return false
}

func isStructType(targetType reflect.Type) bool {
	for targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	return targetType.Kind() == reflect.Struct
}

func preservesCustomJSON(targetType reflect.Type) bool {
	if targetType.Implements(jsonUnmarshalerType) || targetType.Implements(textUnmarshalerType) {
		return true
	}
	return targetType.Kind() != reflect.Pointer && (reflect.PointerTo(targetType).Implements(jsonUnmarshalerType) || reflect.PointerTo(targetType).Implements(textUnmarshalerType))
}
