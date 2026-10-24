package domain

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
)

func TestNormalizeScalarMetadataUsesSafeHistoricalDefaults(t *testing.T) {
	exposure, err := NormalizeExposure("")
	if err != nil || exposure != ConfigExposureInternal {
		t.Fatalf("historical exposure = %q, %v; want INTERNAL", exposure, err)
	}
	sensitivity, err := NormalizeSensitivity("", 1)
	if err != nil || sensitivity != ConfigSensitivitySensitive {
		t.Fatalf("historical sensitive mapping = %q, %v; want SENSITIVE", sensitivity, err)
	}
}

func TestNormalizeValueTypeAllowsOnlyTypedServerOwnedAssets(t *testing.T) {
	for _, valueType := range []string{"IMAGE", "FILE"} {
		if _, err := NormalizeValueType(valueType); err != nil {
			t.Fatalf("NormalizeValueType(%q) should allow the typed asset contract: %v", valueType, err)
		}
	}
	for _, valueType := range []string{"URL", "CONFIG_ASSET", "FILE_ID", "BLOB"} {
		if _, err := NormalizeValueType(valueType); err == nil {
			t.Fatalf("NormalizeValueType(%q) unexpectedly exposed a raw asset authority", valueType)
		}
	}
	for _, value := range []string{
		"https://cdn.example/logo.png",
		"data:image/png;base64,AAAA",
		"blob:https://app.example/asset",
		"file:///tmp/logo.png",
		"/api/config-assets/01",
		"/api/config-assets/1?download=1",
		"/api/config-assets/1/extra",
	} {
		if _, _, err := CanonicalizeScalarValue(value, ConfigValueImage, nil); err == nil {
			t.Fatalf("IMAGE accepted non-canonical asset value %q", value)
		}
	}
	canonical, typed, err := CanonicalizeScalarValue("/api/config-assets/42", ConfigValueImage, nil)
	if err != nil || canonical != "/api/config-assets/42" || typed != "/api/config-assets/42" {
		t.Fatalf("IMAGE stable path was not preserved: canonical=%q typed=%#v err=%v", canonical, typed, err)
	}
}

func TestScalarValidationRejectsUnknownAndTrailingFields(t *testing.T) {
	for _, raw := range []string{
		`{"required":true,"script":"alert(1)"}`,
		`{"required":true} {"required":false}`,
	} {
		var validation ScalarValidation
		if err := sonic.Unmarshal([]byte(raw), &validation); err == nil {
			t.Fatalf("validation %q unexpectedly succeeded", raw)
		}
	}
}

func TestCanonicalizeScalarValueIsStrict(t *testing.T) {
	if _, _, err := CanonicalizeScalarValue(`{"ok":true} trailing`, ConfigValueJSON, nil); err == nil {
		t.Fatal("JSON with trailing data unexpectedly succeeded")
	}
	if _, _, err := CanonicalizeScalarValue(`not-json`, ConfigValueJSON, nil); err == nil {
		t.Fatal("invalid JSON unexpectedly fell back to a string")
	}
	if _, _, err := CanonicalizeScalarValue("1", ConfigValueBoolean, nil); err == nil {
		t.Fatal("numeric boolean unexpectedly succeeded")
	}
	deep := strings.Repeat(`{"a":`, maxScalarJSONDepth+1) + `true` + strings.Repeat(`}`, maxScalarJSONDepth+1)
	if _, _, err := CanonicalizeScalarValue(deep, ConfigValueJSON, nil); err == nil {
		t.Fatal("over-depth JSON unexpectedly succeeded")
	}
	_, typed, err := CanonicalizeScalarValue(`{"large":9007199254740993}`, ConfigValueJSON, nil)
	if err != nil {
		t.Fatalf("canonicalize precise JSON number: %v", err)
	}
	object, ok := typed.(map[string]any)
	if !ok {
		t.Fatalf("typed JSON = %T, want map[string]any", typed)
	}
	number, ok := object["large"].(json.Number)
	if !ok || number.String() != "9007199254740993" {
		t.Fatalf("Sonic JSON number lost precision: %#v (%T)", object["large"], object["large"])
	}
}

func TestCanReadConfigUsesOnePolicyForExternalAndInternalReads(t *testing.T) {
	group := &ConfigGroup{GroupCode: "runtime", Status: 1}
	item := &Config{
		ConfigKey:   "title",
		GroupCode:   "runtime",
		IsEnabled:   1,
		Exposure:    string(ConfigExposureAuthenticated),
		Sensitivity: string(ConfigSensitivityNormal),
	}
	if CanReadConfig(item, group, ConfigReadContext{Identity: ConfigReadAnonymous}, nil) {
		t.Fatal("anonymous read unexpectedly passed AUTHENTICATED exposure")
	}
	if !CanReadConfig(item, group, ConfigReadContext{
		Identity:  ConfigReadAuthenticated,
		AccountID: 42,
		ScopeID:   "org:7",
	}, nil) {
		t.Fatal("authenticated scoped read unexpectedly failed")
	}
	item.Sensitivity = string(ConfigSensitivitySensitive)
	if CanReadConfig(item, group, ConfigReadContext{
		Identity:  ConfigReadAuthenticated,
		AccountID: 42,
		ScopeID:   "org:7",
	}, nil) {
		t.Fatal("external sensitive read unexpectedly passed")
	}
	registration := &ConsumerRegistration{
		ConsumerID:        "app.shell",
		FullyQualifiedKey: "runtime.title",
		ScopeID:           "server:local",
		Purpose:           "render title",
		AllowedSecret:     ConfigSensitivitySensitive,
	}
	read := ConfigReadContext{
		Identity:      ConfigReadInternal,
		ConsumerID:    "app.shell",
		ScopeID:       "server:local",
		Purpose:       "render title",
		AllowedSecret: ConfigSensitivitySensitive,
	}
	if !CanReadConfig(item, group, read, registration) {
		t.Fatal("registered internal read unexpectedly failed")
	}
	read.Purpose = "different purpose"
	if CanReadConfig(item, group, read, registration) {
		t.Fatal("internal purpose mismatch unexpectedly passed")
	}
	read.Purpose = registration.Purpose
	registration.FullyQualifiedKey = "runtime.other"
	if CanReadConfig(item, group, read, registration) {
		t.Fatal("internal fully-qualified-key mismatch unexpectedly passed")
	}
}
