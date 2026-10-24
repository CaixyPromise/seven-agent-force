package domain

// This file implements the intentionally small Parameters Schema subset used
// by Result Processors.  It is not a partial implementation of every JSON
// Schema dialect: accepting arbitrary keywords would make the editor promise
// validation behavior the Worker cannot reproduce.  The supported subset is
// enough for Task Create forms and is validated identically before a frozen
// TaskTurn is created.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const maxParameterSchemaDepth = 8

var supportedParameterSchemaKeywords = map[string]struct{}{
	"type": {}, "properties": {}, "required": {}, "additionalProperties": {}, "enum": {},
	"items": {}, "minItems": {}, "maxItems": {}, "minLength": {}, "maxLength": {},
	"pattern": {}, "minimum": {}, "maximum": {}, "title": {}, "description": {}, "default": {},
}

// ValidateParametersSchema validates authoring-time syntax for the restricted
// schema subset.  A missing schema means the immutable Task parameters must
// still be an object but imposes no named-field restrictions.
func ValidateParametersSchema(schema json.RawMessage) error {
	if len(bytes.TrimSpace(schema)) == 0 || bytes.Equal(bytes.TrimSpace(schema), []byte("null")) {
		return nil
	}
	root, err := decodeParameterSchema(schema, "parametersSchema", 0)
	if err != nil {
		return err
	}
	if rawType, exists := root["type"]; exists {
		types, err := decodeSchemaTypes(rawType, "parametersSchema.type")
		if err != nil {
			return err
		}
		if !types["object"] {
			return fmt.Errorf("parametersSchema.type must include object")
		}
	}
	return nil
}

// ValidateParameters checks a Task Create parameter object against the exact
// immutable Processor manifest.  Values are decoded with UseNumber so numeric
// comparisons do not silently round IDs or integer bounds through float64.
func ValidateParameters(schema, parameters json.RawMessage) error {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(parameters))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("resultProcessorParameters must be valid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("resultProcessorParameters contains trailing JSON")
	}
	if _, ok := value.(map[string]any); !ok {
		return fmt.Errorf("resultProcessorParameters must be a JSON object")
	}
	if err := ValidateParametersSchema(schema); err != nil {
		return err
	}
	if len(bytes.TrimSpace(schema)) == 0 || bytes.Equal(bytes.TrimSpace(schema), []byte("null")) {
		return nil
	}
	root, err := decodeParameterSchema(schema, "parametersSchema", 0)
	if err != nil {
		return err
	}
	return validateParameterValue(root, value, "resultProcessorParameters", 0)
}

func decodeParameterSchema(raw json.RawMessage, location string, depth int) (map[string]json.RawMessage, error) {
	if depth > maxParameterSchemaDepth {
		return nil, fmt.Errorf("%s exceeds maximum schema depth", location)
	}
	var object map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", location)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return nil, fmt.Errorf("%s contains trailing JSON", location)
	}
	for keyword := range object {
		if _, supported := supportedParameterSchemaKeywords[keyword]; !supported {
			return nil, fmt.Errorf("%s uses unsupported keyword %q", location, keyword)
		}
	}
	if rawType, exists := object["type"]; exists {
		if _, err := decodeSchemaTypes(rawType, location+".type"); err != nil {
			return nil, err
		}
	}
	if rawProperties, exists := object["properties"]; exists {
		var properties map[string]json.RawMessage
		if err := json.Unmarshal(rawProperties, &properties); err != nil || properties == nil {
			return nil, fmt.Errorf("%s.properties must be a JSON object", location)
		}
		for name, child := range properties {
			if strings.TrimSpace(name) == "" || len(name) > 128 {
				return nil, fmt.Errorf("%s.properties contains an invalid name", location)
			}
			if _, err := decodeParameterSchema(child, location+".properties."+name, depth+1); err != nil {
				return nil, err
			}
		}
	}
	if rawRequired, exists := object["required"]; exists {
		var fields []string
		if err := json.Unmarshal(rawRequired, &fields); err != nil {
			return nil, fmt.Errorf("%s.required must be a string array", location)
		}
		seen := make(map[string]struct{}, len(fields))
		for _, field := range fields {
			if strings.TrimSpace(field) == "" || len(field) > 128 {
				return nil, fmt.Errorf("%s.required contains an invalid name", location)
			}
			if _, duplicate := seen[field]; duplicate {
				return nil, fmt.Errorf("%s.required contains duplicate %q", location, field)
			}
			seen[field] = struct{}{}
		}
	}
	if rawAdditional, exists := object["additionalProperties"]; exists {
		var allowed bool
		if err := json.Unmarshal(rawAdditional, &allowed); err != nil {
			return nil, fmt.Errorf("%s.additionalProperties must be boolean", location)
		}
	}
	if rawItems, exists := object["items"]; exists {
		if _, err := decodeParameterSchema(rawItems, location+".items", depth+1); err != nil {
			return nil, err
		}
	}
	for _, keyword := range []string{"minItems", "maxItems", "minLength", "maxLength"} {
		if rawValue, exists := object[keyword]; exists {
			value, err := decodeNonNegativeInteger(rawValue)
			if err != nil || value > 1<<20 {
				return nil, fmt.Errorf("%s.%s must be a bounded non-negative integer", location, keyword)
			}
		}
	}
	if rawPattern, exists := object["pattern"]; exists {
		var expression string
		if err := json.Unmarshal(rawPattern, &expression); err != nil || len(expression) > 1024 {
			return nil, fmt.Errorf("%s.pattern must be a string of at most 1024 characters", location)
		}
		if _, err := regexp.Compile(expression); err != nil {
			return nil, fmt.Errorf("%s.pattern is invalid", location)
		}
	}
	for _, keyword := range []string{"minimum", "maximum"} {
		if rawValue, exists := object[keyword]; exists {
			if _, err := decodeSchemaNumber(rawValue); err != nil {
				return nil, fmt.Errorf("%s.%s must be a finite number", location, keyword)
			}
		}
	}
	if rawEnum, exists := object["enum"]; exists {
		var values []json.RawMessage
		if err := json.Unmarshal(rawEnum, &values); err != nil || len(values) == 0 || len(values) > 256 {
			return nil, fmt.Errorf("%s.enum must be a non-empty array of at most 256 values", location)
		}
		for _, value := range values {
			if !json.Valid(value) {
				return nil, fmt.Errorf("%s.enum contains invalid JSON", location)
			}
		}
	}
	return object, nil
}

func validateParameterValue(schema map[string]json.RawMessage, value any, location string, depth int) error {
	if depth > maxParameterSchemaDepth {
		return fmt.Errorf("%s exceeds maximum value depth", location)
	}
	if rawTypes, exists := schema["type"]; exists {
		types, err := decodeSchemaTypes(rawTypes, location+".type")
		if err != nil {
			return err
		}
		if !matchesParameterType(types, value) {
			return fmt.Errorf("%s does not match declared type", location)
		}
	}
	if rawEnum, exists := schema["enum"]; exists && !matchesParameterEnum(rawEnum, value) {
		return fmt.Errorf("%s is not one of the declared enum values", location)
	}
	if stringValue, ok := value.(string); ok {
		if rawMinimum, exists := schema["minLength"]; exists {
			minimum, _ := decodeNonNegativeInteger(rawMinimum)
			if len([]rune(stringValue)) < minimum {
				return fmt.Errorf("%s is shorter than minLength", location)
			}
		}
		if rawMaximum, exists := schema["maxLength"]; exists {
			maximum, _ := decodeNonNegativeInteger(rawMaximum)
			if len([]rune(stringValue)) > maximum {
				return fmt.Errorf("%s exceeds maxLength", location)
			}
		}
		if rawPattern, exists := schema["pattern"]; exists {
			var expression string
			_ = json.Unmarshal(rawPattern, &expression)
			matched, _ := regexp.MatchString(expression, stringValue)
			if !matched {
				return fmt.Errorf("%s does not match pattern", location)
			}
		}
	}
	if number, ok := value.(json.Number); ok {
		parsed, err := strconv.ParseFloat(number.String(), 64)
		if err != nil || math.IsInf(parsed, 0) || math.IsNaN(parsed) {
			return fmt.Errorf("%s is not a finite number", location)
		}
		if rawMinimum, exists := schema["minimum"]; exists {
			minimum, _ := decodeSchemaNumber(rawMinimum)
			if parsed < minimum {
				return fmt.Errorf("%s is below minimum", location)
			}
		}
		if rawMaximum, exists := schema["maximum"]; exists {
			maximum, _ := decodeSchemaNumber(rawMaximum)
			if parsed > maximum {
				return fmt.Errorf("%s exceeds maximum", location)
			}
		}
	}
	if array, ok := value.([]any); ok {
		if rawMinimum, exists := schema["minItems"]; exists {
			minimum, _ := decodeNonNegativeInteger(rawMinimum)
			if len(array) < minimum {
				return fmt.Errorf("%s has fewer than minItems", location)
			}
		}
		if rawMaximum, exists := schema["maxItems"]; exists {
			maximum, _ := decodeNonNegativeInteger(rawMaximum)
			if len(array) > maximum {
				return fmt.Errorf("%s exceeds maxItems", location)
			}
		}
		if rawItems, exists := schema["items"]; exists {
			child, _ := decodeParameterSchema(rawItems, location+".items", depth+1)
			for index, item := range array {
				if err := validateParameterValue(child, item, location+"["+strconv.Itoa(index)+"]", depth+1); err != nil {
					return err
				}
			}
		}
	}
	if object, ok := value.(map[string]any); ok {
		properties := map[string]json.RawMessage{}
		if rawProperties, exists := schema["properties"]; exists {
			_ = json.Unmarshal(rawProperties, &properties)
		}
		if rawRequired, exists := schema["required"]; exists {
			var required []string
			_ = json.Unmarshal(rawRequired, &required)
			for _, field := range required {
				if _, exists := object[field]; !exists {
					return fmt.Errorf("%s.%s is required", location, field)
				}
			}
		}
		additional := true
		if rawAdditional, exists := schema["additionalProperties"]; exists {
			_ = json.Unmarshal(rawAdditional, &additional)
		}
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			childRaw, declared := properties[key]
			if !declared {
				if !additional {
					return fmt.Errorf("%s.%s is not allowed", location, key)
				}
				continue
			}
			child, _ := decodeParameterSchema(childRaw, location+"."+key, depth+1)
			if err := validateParameterValue(child, object[key], location+"."+key, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}

func decodeSchemaTypes(raw json.RawMessage, location string) (map[string]bool, error) {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		return validatedSchemaTypes([]string{one}, location)
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil || len(many) == 0 {
		return nil, fmt.Errorf("%s must be a type string or non-empty type array", location)
	}
	return validatedSchemaTypes(many, location)
}

func validatedSchemaTypes(values []string, location string) (map[string]bool, error) {
	allowed := map[string]struct{}{"string": {}, "number": {}, "integer": {}, "boolean": {}, "array": {}, "object": {}, "null": {}}
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if _, exists := allowed[value]; !exists || result[value] {
			return nil, fmt.Errorf("%s contains unsupported or duplicate type", location)
		}
		result[value] = true
	}
	return result, nil
}

func matchesParameterType(types map[string]bool, value any) bool {
	if value == nil {
		return types["null"]
	}
	switch typed := value.(type) {
	case string:
		return types["string"]
	case bool:
		return types["boolean"]
	case []any:
		return types["array"]
	case map[string]any:
		return types["object"]
	case json.Number:
		if types["number"] {
			return true
		}
		if !types["integer"] {
			return false
		}
		_, err := strconv.ParseInt(typed.String(), 10, 64)
		return err == nil
	default:
		return false
	}
}

func matchesParameterEnum(raw json.RawMessage, value any) bool {
	var options []any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&options); err != nil {
		return false
	}
	actual, err := canonicalParameterJSON(value)
	if err != nil {
		return false
	}
	for _, option := range options {
		encoded, err := canonicalParameterJSON(option)
		if err == nil && bytes.Equal(encoded, actual) {
			return true
		}
	}
	return false
}

func canonicalParameterJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func decodeNonNegativeInteger(raw json.RawMessage) (int, error) {
	var value json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return 0, err
	}
	parsed, err := strconv.ParseInt(value.String(), 10, 64)
	if err != nil || parsed < 0 || parsed > math.MaxInt {
		return 0, fmt.Errorf("not a bounded non-negative integer")
	}
	return int(parsed), nil
}

func decodeSchemaNumber(raw json.RawMessage) (float64, error) {
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(number.String(), 64)
	if err != nil || math.IsInf(value, 0) || math.IsNaN(value) {
		return 0, fmt.Errorf("not finite")
	}
	return value, nil
}
