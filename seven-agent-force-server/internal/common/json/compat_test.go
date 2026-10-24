package json

import (
	stdjson "encoding/json"
	"math"
	"strings"
	"testing"
)

type customWireID int64

var customWireIDUnmarshalCalls int

func (id *customWireID) UnmarshalJSON(data []byte) error {
	customWireIDUnmarshalCalls++
	if string(data) != `"custom"` {
		return stdjson.Unmarshal(data, (*int64)(id))
	}
	*id = 77
	return nil
}

func TestUnmarshalAcceptsQuotedAndNumericInt64Values(t *testing.T) {
	type nested struct {
		OwnerID int64 `json:"ownerId"`
	}
	type payload struct {
		ID       int64            `json:"id"`
		Counter  uint64           `json:"counter"`
		RoleIDs  []int64          `json:"roleIds"`
		ByCode   map[string]int64 `json:"byCode"`
		Nested   *nested          `json:"nested"`
		TextCode string           `json:"textCode"`
	}

	var actual payload
	err := Unmarshal([]byte(`{
		"id":"9007199254740993",
		"counter":"18446744073709551615",
		"roleIds":["9007199254740995",7],
		"byCode":{"root":"9007199254740997"},
		"nested":{"ownerId":"9007199254740999"},
		"textCode":"9007199254741001"
	}`), &actual)
	if err != nil {
		t.Fatalf("unmarshal compatible int64 payload: %v", err)
	}
	if actual.ID != 9007199254740993 || actual.Counter != math.MaxUint64 {
		t.Fatalf("unexpected scalar values: %#v", actual)
	}
	if len(actual.RoleIDs) != 2 || actual.RoleIDs[0] != 9007199254740995 || actual.RoleIDs[1] != 7 {
		t.Fatalf("unexpected role ids: %#v", actual.RoleIDs)
	}
	if actual.ByCode["root"] != 9007199254740997 || actual.Nested == nil || actual.Nested.OwnerID != 9007199254740999 {
		t.Fatalf("unexpected nested values: %#v", actual)
	}
	if actual.TextCode != "9007199254741001" {
		t.Fatalf("numeric text must remain a string: %q", actual.TextCode)
	}
}

func TestUnmarshalRejectsInvalidQuotedInt64Values(t *testing.T) {
	type payload struct {
		ID int64 `json:"id"`
	}

	for _, raw := range []string{
		`{"id":""}`,
		`{"id":"1.5"}`,
		`{"id":"1e3"}`,
		`{"id":"9223372036854775808"}`,
		`{"id":"not-a-number"}`,
	} {
		var actual payload
		if err := Unmarshal([]byte(raw), &actual); err == nil {
			t.Fatalf("expected invalid int64 payload to fail: %s", raw)
		}
	}
}

func TestUnmarshalPreservesStringTaggedAndRawJSONFields(t *testing.T) {
	type payload struct {
		StringTagged int64              `json:"stringTagged,string"`
		Raw          stdjson.RawMessage `json:"raw"`
		Custom       customWireID       `json:"custom"`
		ID           int64              `json:"id"`
	}

	var actual payload
	customWireIDUnmarshalCalls = 0
	err := Unmarshal([]byte(`{"stringTagged":"42","raw":{"id":"9007199254740993"},"custom":"custom","id":"9007199254740995"}`), &actual)
	if err != nil {
		t.Fatalf("unmarshal fields with standard semantics: %v", err)
	}
	if actual.StringTagged != 42 || string(actual.Raw) != `{"id":"9007199254740993"}` || actual.Custom != 77 || actual.ID != 9007199254740995 {
		t.Fatalf("unexpected preserved values: %#v", actual)
	}
	if customWireIDUnmarshalCalls != 1 {
		t.Fatalf("custom JSON unmarshaler must run once, got %d calls", customWireIDUnmarshalCalls)
	}
}

func TestNormalizeForJSONConvertsInt64ToString(t *testing.T) {
	type payload struct {
		ID      int64          `json:"id"`
		Counter uint64         `json:"counter"`
		Items   []int64        `json:"items"`
		Nested  map[string]any `json:"nested"`
	}

	normalized := NormalizeForJSON(payload{
		ID:      100000000000000001,
		Counter: 9,
		Items:   []int64{7, 8},
		Nested: map[string]any{
			"value": int64(12),
		},
	}).(map[string]any)

	if normalized["id"] != "100000000000000001" {
		t.Fatalf("unexpected id: %#v", normalized["id"])
	}
	if normalized["counter"] != "9" {
		t.Fatalf("unexpected counter: %#v", normalized["counter"])
	}
	items := normalized["items"].([]any)
	if items[0] != "7" || items[1] != "8" {
		t.Fatalf("unexpected items: %#v", items)
	}
	nested := normalized["nested"].(map[string]any)
	if nested["value"] != "12" {
		t.Fatalf("unexpected nested value: %#v", nested["value"])
	}
}

func TestMaskSensitiveFieldsMasksAndClips(t *testing.T) {
	sanitized := MaskSensitiveFields(map[string]any{
		"password":     "super-secret",
		"refreshToken": "token-value",
		"profile": map[string]any{
			"displayName": "abcdefghijklmnopqrstuvwxyz",
		},
	}, []string{"password", "token"}, 8).(map[string]any)

	if sanitized["password"] != "******" {
		t.Fatalf("password not masked: %#v", sanitized["password"])
	}
	if sanitized["refreshToken"] != "******" {
		t.Fatalf("refreshToken not masked: %#v", sanitized["refreshToken"])
	}
	profile := sanitized["profile"].(map[string]any)
	if profile["displayName"] != "abcdefgh...(truncated)" {
		t.Fatalf("displayName not clipped: %#v", profile["displayName"])
	}
}

func TestMaskSensitiveFieldsMasksManagementBearer(t *testing.T) {
	sanitized := MaskSensitiveFields(map[string]any{
		"managementBearer": "node-management-bearer",
	}, nil, 512).(map[string]any)

	if sanitized["managementBearer"] != "******" {
		t.Fatalf("managementBearer not masked: %#v", sanitized["managementBearer"])
	}
}

func TestMaskSensitiveFieldsMasksSensitiveConfigWithoutHidingKey(t *testing.T) {
	sanitized := MaskSensitiveFields(map[string]any{
		"configKey":   "payment.gateway.secret",
		"configValue": "plain-secret-updated",
		"isSensitive": 1,
		"items": []any{
			map[string]any{"key": "API_TOKEN", "value": "raw-token"},
			map[string]any{"key": "DISPLAY_NAME", "value": "public-name"},
		},
	}, []string{"password"}, 512).(map[string]any)

	if sanitized["configKey"] != "payment.gateway.secret" {
		t.Fatalf("configKey should remain visible, got %#v", sanitized["configKey"])
	}
	if sanitized["configValue"] != "******" {
		t.Fatalf("configValue not masked: %#v", sanitized["configValue"])
	}
	if sanitized["isSensitive"] != "******" {
		t.Fatalf("isSensitive flag not masked: %#v", sanitized["isSensitive"])
	}
	items := sanitized["items"].([]any)
	secretItem := items[0].(map[string]any)
	if secretItem["key"] != "API_TOKEN" || secretItem["value"] != "******" {
		t.Fatalf("sensitive key/value pair not masked correctly: %#v", secretItem)
	}
	publicItem := items[1].(map[string]any)
	if publicItem["key"] != "DISPLAY_NAME" || publicItem["value"] != "public-name" {
		t.Fatalf("public key/value pair should remain visible: %#v", publicItem)
	}
}

func TestMaskSensitiveTextMasksConfigAssignments(t *testing.T) {
	masked := MaskSensitiveText(
		`payload configKey=payment.gateway configValue=plain-secret isSensitive=1 {"configValue":"json-secret","isSensitive":true}`,
		nil,
		512,
	)
	for _, leaked := range []string{"plain-secret", "json-secret", "isSensitive=1", `"isSensitive":true`} {
		if strings.Contains(masked, leaked) {
			t.Fatalf("sensitive text leaked %q in %s", leaked, masked)
		}
	}
	if !strings.Contains(masked, "configKey=payment.gateway") {
		t.Fatalf("configKey should remain visible in %s", masked)
	}
}

func TestMaskSensitiveTextMasksOAuthProtocolSecrets(t *testing.T) {
	masked := MaskSensitiveText(
		`callback authCode=plain-code state=plain-state oauthState=oauth-state oidcNonce=oidc-nonce codeVerifier=code-verifier {"authorizationCode":"json-code","oauth_state":"json-state","nonce":"json-nonce","code_verifier":"json-verifier"}`,
		nil,
		2048,
	)
	for _, leaked := range []string{
		"plain-code",
		"plain-state",
		"oauth-state",
		"oidc-nonce",
		"code-verifier",
		"json-code",
		"json-state",
		"json-nonce",
		"json-verifier",
	} {
		if strings.Contains(masked, leaked) {
			t.Fatalf("OAuth protocol secret leaked %q in %s", leaked, masked)
		}
	}
}

func TestClipLargePayload(t *testing.T) {
	payload := []byte("1234567890")
	clipped, truncated := ClipLargePayload(payload, 4)
	if !truncated {
		t.Fatal("expected payload to be truncated")
	}
	if string(clipped) != "1234" {
		t.Fatalf("unexpected clipped payload: %s", string(clipped))
	}
}
