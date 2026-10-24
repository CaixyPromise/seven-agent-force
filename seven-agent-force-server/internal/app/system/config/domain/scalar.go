package domain

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

// scalarStrictJSON is the only parser for scalar-validation metadata. The
// allowlisted shape remains strict, strings are copied before they outlive the
// input buffer, and arbitrary JSON numbers keep their lexical precision.
var scalarStrictJSON = sonic.Config{
	CaseSensitive:         true,
	CopyString:            true,
	DisallowUnknownFields: true,
	UseNumber:             true,
	ValidateString:        true,
}.Froze()

// scalarJSON is used for user-supplied scalar JSON values. It intentionally
// preserves json.Number semantics through Sonic rather than silently turning
// large integer literals into float64.
var scalarJSON = sonic.Config{
	CaseSensitive:  true,
	CopyString:     true,
	UseNumber:      true,
	ValidateString: true,
}.Froze()

const (
	CurrentScalarSchemaVersion = 1
	maxScalarTextBytes         = 64 * 1024
	maxScalarJSONBytes         = 32 * 1024
	maxScalarJSONDepth         = 16
	maxMultiEnumItems          = 100
)

type ConfigValueType string

const (
	ConfigValueString    ConfigValueType = "STRING"
	ConfigValueText      ConfigValueType = "TEXT"
	ConfigValueInteger   ConfigValueType = "INTEGER"
	ConfigValueDecimal   ConfigValueType = "DECIMAL"
	ConfigValueBoolean   ConfigValueType = "BOOLEAN"
	ConfigValueEnum      ConfigValueType = "ENUM"
	ConfigValueMultiEnum ConfigValueType = "MULTI_ENUM"
	ConfigValueDate      ConfigValueType = "DATE"
	ConfigValueDateTime  ConfigValueType = "DATETIME"
	ConfigValueDuration  ConfigValueType = "DURATION"
	ConfigValueColor     ConfigValueType = "COLOR"
	ConfigValueJSON      ConfigValueType = "JSON"
	// IMAGE and FILE are persisted only as server-generated same-origin
	// CONFIG_ASSET paths. They are never arbitrary URLs or file identifiers.
	ConfigValueImage ConfigValueType = "IMAGE"
	ConfigValueFile  ConfigValueType = "FILE"
)

type ConfigUIWidget string

const (
	ConfigWidgetInput          ConfigUIWidget = "INPUT"
	ConfigWidgetTextarea       ConfigUIWidget = "TEXTAREA"
	ConfigWidgetInputNumber    ConfigUIWidget = "INPUT_NUMBER"
	ConfigWidgetSwitch         ConfigUIWidget = "SWITCH"
	ConfigWidgetSelect         ConfigUIWidget = "SELECT"
	ConfigWidgetMultiSelect    ConfigUIWidget = "MULTI_SELECT"
	ConfigWidgetDatePicker     ConfigUIWidget = "DATE_PICKER"
	ConfigWidgetDateTimePicker ConfigUIWidget = "DATETIME_PICKER"
	ConfigWidgetDurationInput  ConfigUIWidget = "DURATION_INPUT"
	ConfigWidgetColorPicker    ConfigUIWidget = "COLOR_PICKER"
	ConfigWidgetControlledJSON ConfigUIWidget = "CONTROLLED_JSON"
	ConfigWidgetImageUpload    ConfigUIWidget = "IMAGE_UPLOAD"
	ConfigWidgetFileUpload     ConfigUIWidget = "FILE_UPLOAD"
)

type ConfigExposure string

const (
	ConfigExposureInternal      ConfigExposure = "INTERNAL"
	ConfigExposureAuthenticated ConfigExposure = "AUTHENTICATED"
	ConfigExposurePublic        ConfigExposure = "PUBLIC"
)

type ConfigSensitivity string

const (
	ConfigSensitivityNormal    ConfigSensitivity = "NORMAL"
	ConfigSensitivitySensitive ConfigSensitivity = "SENSITIVE"
	ConfigSensitivitySecret    ConfigSensitivity = "SECRET"
)

type ScalarValidation struct {
	Required  bool     `json:"required,omitempty"`
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	MinValue  *float64 `json:"minValue,omitempty"`
	MaxValue  *float64 `json:"maxValue,omitempty"`
	Options   []string `json:"options,omitempty"`
	MaxItems  *int     `json:"maxItems,omitempty"`
}

func (v *ScalarValidation) UnmarshalJSON(data []byte) error {
	type validationAlias ScalarValidation
	decoder := scalarStrictJSON.NewDecoder(bytes.NewReader(data))
	var value validationAlias
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("validation contains trailing data")
	}
	*v = ScalarValidation(value)
	return nil
}

func NormalizeValueType(value string) (ConfigValueType, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "INT":
		normalized = string(ConfigValueInteger)
	case "BOOL":
		normalized = string(ConfigValueBoolean)
	case "ARRAY":
		normalized = string(ConfigValueMultiEnum)
	}
	switch ConfigValueType(normalized) {
	case ConfigValueString, ConfigValueText, ConfigValueInteger, ConfigValueDecimal,
		ConfigValueBoolean, ConfigValueEnum, ConfigValueMultiEnum, ConfigValueDate,
		ConfigValueDateTime, ConfigValueDuration, ConfigValueColor, ConfigValueJSON,
		ConfigValueImage, ConfigValueFile:
		return ConfigValueType(normalized), nil
	default:
		return "", fmt.Errorf("unsupported scalar value type %q", value)
	}
}

func DefaultWidget(valueType ConfigValueType) ConfigUIWidget {
	switch valueType {
	case ConfigValueText:
		return ConfigWidgetTextarea
	case ConfigValueInteger, ConfigValueDecimal:
		return ConfigWidgetInputNumber
	case ConfigValueBoolean:
		return ConfigWidgetSwitch
	case ConfigValueEnum:
		return ConfigWidgetSelect
	case ConfigValueMultiEnum:
		return ConfigWidgetMultiSelect
	case ConfigValueDate:
		return ConfigWidgetDatePicker
	case ConfigValueDateTime:
		return ConfigWidgetDateTimePicker
	case ConfigValueDuration:
		return ConfigWidgetDurationInput
	case ConfigValueColor:
		return ConfigWidgetColorPicker
	case ConfigValueJSON:
		return ConfigWidgetControlledJSON
	case ConfigValueImage:
		return ConfigWidgetImageUpload
	case ConfigValueFile:
		return ConfigWidgetFileUpload
	default:
		return ConfigWidgetInput
	}
}

func NormalizeWidget(value string, valueType ConfigValueType) (ConfigUIWidget, error) {
	if strings.TrimSpace(value) == "" {
		return DefaultWidget(valueType), nil
	}
	widget := ConfigUIWidget(strings.ToUpper(strings.TrimSpace(value)))
	allowed := map[ConfigValueType][]ConfigUIWidget{
		ConfigValueString:    {ConfigWidgetInput, ConfigWidgetTextarea},
		ConfigValueText:      {ConfigWidgetTextarea},
		ConfigValueInteger:   {ConfigWidgetInputNumber},
		ConfigValueDecimal:   {ConfigWidgetInputNumber},
		ConfigValueBoolean:   {ConfigWidgetSwitch},
		ConfigValueEnum:      {ConfigWidgetSelect},
		ConfigValueMultiEnum: {ConfigWidgetMultiSelect},
		ConfigValueDate:      {ConfigWidgetDatePicker},
		ConfigValueDateTime:  {ConfigWidgetDateTimePicker},
		ConfigValueDuration:  {ConfigWidgetDurationInput},
		ConfigValueColor:     {ConfigWidgetColorPicker},
		ConfigValueJSON:      {ConfigWidgetControlledJSON},
		ConfigValueImage:     {ConfigWidgetImageUpload},
		ConfigValueFile:      {ConfigWidgetFileUpload},
	}
	for _, candidate := range allowed[valueType] {
		if candidate == widget {
			return widget, nil
		}
	}
	return "", fmt.Errorf("widget %q is not compatible with %s", value, valueType)
}

func NormalizeExposure(value string) (ConfigExposure, error) {
	if strings.TrimSpace(value) == "" {
		return ConfigExposureInternal, nil
	}
	exposure := ConfigExposure(strings.ToUpper(strings.TrimSpace(value)))
	switch exposure {
	case ConfigExposureInternal, ConfigExposureAuthenticated, ConfigExposurePublic:
		return exposure, nil
	default:
		return "", fmt.Errorf("unsupported exposure %q", value)
	}
}

func NormalizeSensitivity(value string, legacySensitive int) (ConfigSensitivity, error) {
	if strings.TrimSpace(value) == "" {
		if legacySensitive == 1 {
			return ConfigSensitivitySensitive, nil
		}
		return ConfigSensitivityNormal, nil
	}
	sensitivity := ConfigSensitivity(strings.ToUpper(strings.TrimSpace(value)))
	switch sensitivity {
	case ConfigSensitivityNormal, ConfigSensitivitySensitive, ConfigSensitivitySecret:
		return sensitivity, nil
	default:
		return "", fmt.Errorf("unsupported sensitivity %q", value)
	}
}

func ValidateScalarMetadata(valueType ConfigValueType, widget ConfigUIWidget, validation *ScalarValidation, schemaVersion int) error {
	if schemaVersion != CurrentScalarSchemaVersion {
		return fmt.Errorf("unsupported schema version %d", schemaVersion)
	}
	if _, err := NormalizeWidget(string(widget), valueType); err != nil {
		return err
	}
	if validation == nil {
		return nil
	}
	if validation.MinLength != nil && *validation.MinLength < 0 {
		return fmt.Errorf("minLength must be non-negative")
	}
	if validation.MaxLength != nil && (*validation.MaxLength < 0 || *validation.MaxLength > maxScalarTextBytes) {
		return fmt.Errorf("maxLength is outside the supported range")
	}
	if validation.MinLength != nil && validation.MaxLength != nil && *validation.MinLength > *validation.MaxLength {
		return fmt.Errorf("minLength cannot exceed maxLength")
	}
	if validation.MinValue != nil && (math.IsNaN(*validation.MinValue) || math.IsInf(*validation.MinValue, 0)) {
		return fmt.Errorf("minValue must be finite")
	}
	if validation.MaxValue != nil && (math.IsNaN(*validation.MaxValue) || math.IsInf(*validation.MaxValue, 0)) {
		return fmt.Errorf("maxValue must be finite")
	}
	if validation.MinValue != nil && validation.MaxValue != nil && *validation.MinValue > *validation.MaxValue {
		return fmt.Errorf("minValue cannot exceed maxValue")
	}
	if validation.MaxItems != nil && (*validation.MaxItems < 1 || *validation.MaxItems > maxMultiEnumItems) {
		return fmt.Errorf("maxItems is outside the supported range")
	}
	if (valueType == ConfigValueEnum || valueType == ConfigValueMultiEnum) && len(normalizeScalarOptions(validation.Options)) == 0 {
		return fmt.Errorf("%s requires finite options", valueType)
	}
	if valueType != ConfigValueEnum && valueType != ConfigValueMultiEnum && len(validation.Options) > 0 {
		return fmt.Errorf("options are only valid for enum values")
	}
	if valueType == ConfigValueImage || valueType == ConfigValueFile {
		if validation.MinLength != nil || validation.MaxLength != nil || validation.MinValue != nil || validation.MaxValue != nil || validation.MaxItems != nil {
			return fmt.Errorf("%s only supports required validation", valueType)
		}
	}
	return nil
}

func CanonicalizeScalarValue(raw string, valueType ConfigValueType, validation *ScalarValidation) (string, any, error) {
	if len(raw) > maxScalarTextBytes {
		return "", nil, fmt.Errorf("scalar value exceeds maximum size")
	}
	trimmed := strings.TrimSpace(raw)
	if validation != nil && validation.Required && trimmed == "" {
		return "", nil, fmt.Errorf("value is required")
	}
	var canonical string
	var typed any
	switch valueType {
	case ConfigValueString, ConfigValueText:
		canonical, typed = raw, raw
	case ConfigValueImage, ConfigValueFile:
		if trimmed != "" && !IsConfigAssetStablePath(trimmed) {
			return "", nil, fmt.Errorf("asset value must be a server-generated same-origin config asset path")
		}
		canonical, typed = trimmed, trimmed
	case ConfigValueInteger:
		value, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return "", nil, fmt.Errorf("value must be an integer")
		}
		canonical, typed = strconv.FormatInt(value, 10), value
	case ConfigValueDecimal:
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return "", nil, fmt.Errorf("value must be a finite decimal")
		}
		canonical, typed = strconv.FormatFloat(value, 'f', -1, 64), strconv.FormatFloat(value, 'f', -1, 64)
	case ConfigValueBoolean:
		if trimmed != "true" && trimmed != "false" {
			return "", nil, fmt.Errorf("value must be true or false")
		}
		value, err := strconv.ParseBool(strings.ToLower(trimmed))
		if err != nil {
			return "", nil, fmt.Errorf("value must be true or false")
		}
		canonical, typed = strconv.FormatBool(value), value
	case ConfigValueEnum:
		if !containsScalarOption(validation, trimmed) {
			return "", nil, fmt.Errorf("value is not an allowed option")
		}
		canonical, typed = trimmed, trimmed
	case ConfigValueMultiEnum:
		var values []string
		if err := decodeStrictJSON(raw, &values); err != nil {
			return "", nil, fmt.Errorf("multi-enum value must be a JSON string array")
		}
		limit := maxMultiEnumItems
		if validation != nil && validation.MaxItems != nil {
			limit = *validation.MaxItems
		}
		if len(values) > limit {
			return "", nil, fmt.Errorf("multi-enum contains too many items")
		}
		seen := map[string]struct{}{}
		for index, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || !containsScalarOption(validation, value) {
				return "", nil, fmt.Errorf("multi-enum contains an invalid option")
			}
			if _, exists := seen[value]; exists {
				return "", nil, fmt.Errorf("multi-enum contains duplicate options")
			}
			seen[value] = struct{}{}
			values[index] = value
		}
		payload, _ := sonic.Marshal(values)
		canonical, typed = string(payload), values
	case ConfigValueDate:
		value, err := time.Parse("2006-01-02", trimmed)
		if err != nil {
			return "", nil, fmt.Errorf("value must use YYYY-MM-DD")
		}
		canonical, typed = value.Format("2006-01-02"), value.Format("2006-01-02")
	case ConfigValueDateTime:
		value, err := time.Parse(time.RFC3339, trimmed)
		if err != nil {
			return "", nil, fmt.Errorf("value must use RFC3339")
		}
		canonical, typed = value.UTC().Format(time.RFC3339), value.UTC().Format(time.RFC3339)
	case ConfigValueDuration:
		value, err := time.ParseDuration(trimmed)
		if err != nil || value < 0 {
			return "", nil, fmt.Errorf("value must be a non-negative Go duration")
		}
		canonical, typed = value.String(), value.String()
	case ConfigValueColor:
		if !isSafeColor(trimmed) {
			return "", nil, fmt.Errorf("value must be a hex color")
		}
		canonical, typed = strings.ToUpper(trimmed), strings.ToUpper(trimmed)
	case ConfigValueJSON:
		if len(raw) > maxScalarJSONBytes {
			return "", nil, fmt.Errorf("JSON value exceeds maximum size")
		}
		var value any
		if err := decodeStrictJSON(raw, &value); err != nil {
			return "", nil, fmt.Errorf("value must be strict JSON")
		}
		if jsonDepth(value) > maxScalarJSONDepth {
			return "", nil, fmt.Errorf("JSON value exceeds maximum depth")
		}
		payload, err := sonic.Marshal(value)
		if err != nil {
			return "", nil, fmt.Errorf("value must be strict JSON")
		}
		canonical, typed = string(payload), value
	default:
		return "", nil, fmt.Errorf("unsupported scalar value type %q", valueType)
	}
	if err := validateScalarBounds(canonical, typed, valueType, validation); err != nil {
		return "", nil, err
	}
	return canonical, typed, nil
}

func DecodeScalarValue(raw string, valueType string, validation *ScalarValidation) (any, error) {
	normalized, err := NormalizeValueType(valueType)
	if err != nil {
		return nil, err
	}
	_, typed, err := CanonicalizeScalarValue(raw, normalized, validation)
	return typed, err
}

// IsConfigAssetStablePath accepts only the server-owned presentation path.
// The strict decimal ID parser rejects query strings, fragments, absolute
// URLs, blob/data schemes, file paths, and traversal-like suffixes.
func IsConfigAssetStablePath(value string) bool {
	const prefix = "/api/config-assets/"
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	id := strings.TrimPrefix(value, prefix)
	if id == "" || strings.ContainsAny(id, "/?#\\") {
		return false
	}
	parsed, err := strconv.ParseInt(id, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == id
}

func normalizeScalarOptions(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func containsScalarOption(validation *ScalarValidation, value string) bool {
	if validation == nil {
		return false
	}
	for _, option := range normalizeScalarOptions(validation.Options) {
		if option == value {
			return true
		}
	}
	return false
}

func validateScalarBounds(canonical string, typed any, valueType ConfigValueType, validation *ScalarValidation) error {
	if validation == nil {
		return nil
	}
	length := len([]rune(canonical))
	if valueType == ConfigValueString || valueType == ConfigValueText {
		if validation.MinLength != nil && length < *validation.MinLength {
			return fmt.Errorf("value is shorter than minLength")
		}
		if validation.MaxLength != nil && length > *validation.MaxLength {
			return fmt.Errorf("value exceeds maxLength")
		}
	}
	var numeric float64
	var numericValue bool
	switch value := typed.(type) {
	case int64:
		numeric, numericValue = float64(value), true
	case string:
		if valueType == ConfigValueDecimal {
			numeric, _ = strconv.ParseFloat(value, 64)
			numericValue = true
		}
	}
	if numericValue {
		if validation.MinValue != nil && numeric < *validation.MinValue {
			return fmt.Errorf("value is below minValue")
		}
		if validation.MaxValue != nil && numeric > *validation.MaxValue {
			return fmt.Errorf("value exceeds maxValue")
		}
	}
	return nil
}

func decodeStrictJSON(raw string, target any) error {
	decoder := scalarJSON.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("multiple JSON values")
	}
	return nil
}

func jsonDepth(value any) int {
	switch typed := value.(type) {
	case []any:
		max := 1
		for _, child := range typed {
			if depth := 1 + jsonDepth(child); depth > max {
				max = depth
			}
		}
		return max
	case map[string]any:
		max := 1
		for _, child := range typed {
			if depth := 1 + jsonDepth(child); depth > max {
				max = depth
			}
		}
		return max
	default:
		return 1
	}
}

func isSafeColor(value string) bool {
	if len(value) != 7 || value[0] != '#' {
		return false
	}
	for _, char := range value[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

type ConfigReadIdentity string

const (
	ConfigReadAnonymous     ConfigReadIdentity = "ANONYMOUS"
	ConfigReadAuthenticated ConfigReadIdentity = "AUTHENTICATED"
	ConfigReadInternal      ConfigReadIdentity = "INTERNAL"
)

type ConfigReadContext struct {
	Identity      ConfigReadIdentity
	AccountID     int64
	ScopeID       string
	AuthzVersion  int64
	ConsumerID    string
	Purpose       string
	AllowedSecret ConfigSensitivity
}

type ConsumerRegistration struct {
	ConsumerID        string
	FullyQualifiedKey string
	ScopeID           string
	Purpose           string
	AllowedSecret     ConfigSensitivity
	Source            string
	ActualConsumer    string
	Activation        string
	CacheRule         string
}

func CanReadConfig(item *Config, group *ConfigGroup, read ConfigReadContext, registration *ConsumerRegistration) bool {
	if item == nil || item.IsEnabled != 1 || item.IsDeleted == 1 || group == nil || group.Status != 1 || group.IsDeleted == 1 {
		return false
	}
	exposure, err := NormalizeExposure(item.Exposure)
	if err != nil {
		return false
	}
	sensitivity, err := NormalizeSensitivity(item.Sensitivity, item.IsSensitive)
	if err != nil {
		return false
	}
	if read.Identity == ConfigReadInternal {
		if registration == nil || strings.TrimSpace(read.ConsumerID) == "" || strings.TrimSpace(read.ScopeID) == "" || strings.TrimSpace(read.Purpose) == "" {
			return false
		}
		if registration.ConsumerID != read.ConsumerID || registration.ScopeID != read.ScopeID || registration.Purpose != read.Purpose {
			return false
		}
		if registration.FullyQualifiedKey != item.FullyQualifiedKey(group) {
			return false
		}
		return sensitivityRank(sensitivity) <= sensitivityRank(registration.AllowedSecret) &&
			sensitivityRank(sensitivity) <= sensitivityRank(read.AllowedSecret)
	}
	if sensitivity != ConfigSensitivityNormal {
		return false
	}
	switch read.Identity {
	case ConfigReadAnonymous:
		return exposure == ConfigExposurePublic
	case ConfigReadAuthenticated:
		return read.AccountID > 0 && strings.TrimSpace(read.ScopeID) != "" &&
			(exposure == ConfigExposurePublic || exposure == ConfigExposureAuthenticated)
	default:
		return false
	}
}

func sensitivityRank(value ConfigSensitivity) int {
	switch value {
	case ConfigSensitivitySecret:
		return 2
	case ConfigSensitivitySensitive:
		return 1
	default:
		return 0
	}
}
