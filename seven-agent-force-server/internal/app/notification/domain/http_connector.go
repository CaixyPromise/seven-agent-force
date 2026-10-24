package domain

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	// HTTPConnectorMethodPOST is the only method allowed by the first connector
	// contract. A caller cannot switch it per delivery.
	HTTPConnectorMethodPOST = "POST"

	// HTTPConnectorAuthNone sends no connection credential.
	HTTPConnectorAuthNone = "NONE"
	// HTTPConnectorAuthBearer uses a connection-owned bearer secret in G5.2.
	HTTPConnectorAuthBearer = "BEARER"
	// HTTPConnectorAuthBasic uses a connection-owned basic credential in G5.2.
	HTTPConnectorAuthBasic = "BASIC"
	// HTTPConnectorAuthHMACSHA256 uses a connection-owned RFC 9421 key in G5.2.
	HTTPConnectorAuthHMACSHA256 = "HMAC_SHA256"
	// HTTPConnectorAuthMTLS is reserved for a later typed certificate-bundle
	// implementation. G5.2 deliberately rejects it: a normal string secret
	// cannot safely represent a client certificate, private key and trust policy.
	HTTPConnectorAuthMTLS = "MTLS"

	// HTTPConnectorSecretRefConnection identifies the channel's encrypted secret
	// slot. The configuration stores only this reference, never a secret value.
	HTTPConnectorSecretRefConnection = "CONNECTION_SECRET"
	// HTTPConnectorSecretRefClientCertificate identifies a future encrypted mTLS
	// credential slot. It remains a reference rather than a PEM value and is not
	// accepted by G5.2 configuration.
	HTTPConnectorSecretRefClientCertificate = "CONNECTION_CLIENT_CERTIFICATE"

	// HTTPConnectorFieldSubject is the rendered notification subject.
	HTTPConnectorFieldSubject = "SUBJECT"
	// HTTPConnectorFieldText is the rendered notification text.
	HTTPConnectorFieldText = "TEXT"
	// HTTPConnectorFieldEventKey is the semantic notification event key.
	HTTPConnectorFieldEventKey = "EVENT_KEY"
	// HTTPConnectorFieldCategory is the normalized notification category.
	HTTPConnectorFieldCategory = "CATEGORY"
	// HTTPConnectorFieldPriority is the normalized notification priority.
	HTTPConnectorFieldPriority = "PRIORITY"
	// HTTPConnectorFieldTraceID is the caller trace identifier.
	HTTPConnectorFieldTraceID = "TRACE_ID"
	// HTTPConnectorFieldDeepLink is an already-sanitized internal deep link.
	HTTPConnectorFieldDeepLink = "DEEP_LINK"
	// HTTPConnectorFieldActionURL is generated only by Notification Center from
	// the trusted public base URL and an already validated internal deep link.
	HTTPConnectorFieldActionURL = "ACTION_URL"

	// HTTPConnectorIdempotencyHeader is generated from the durable delivery key;
	// neither the operator nor a business caller may replace its name or value.
	HTTPConnectorIdempotencyHeader = "Idempotency-Key"

	httpConnectorMaxFieldMappings = 16
	httpConnectorMaxHeaders       = 8
	httpConnectorMinTimeoutMillis = 1000
	httpConnectorMaxTimeoutMillis = 30000
)

// HTTPConnectorConfig is the persisted, operator-owned static request shape.
// The URL, method, authentication reference and mapping belong to the
// connection; business callers can never supply a raw request body or a URL.
type HTTPConnectorConfig struct {
	EndpointURL         string                      `json:"endpointUrl"`
	EgressPolicyRef     string                      `json:"egressPolicyRef,omitempty"`
	Method              string                      `json:"method"`
	Authentication      HTTPConnectorAuthentication `json:"authentication"`
	FieldMappings       []HTTPConnectorFieldMapping `json:"fieldMappings"`
	HeaderAllowlist     []string                    `json:"headerAllowlist,omitempty"`
	IdempotencyHeader   string                      `json:"idempotencyHeader"`
	TimeoutMilliseconds int                         `json:"timeoutMilliseconds"`
	SuccessStatusCodes  []int                       `json:"successStatusCodes,omitempty"`
}

// HTTPConnectorAuthentication declares a supported credential mode and a
// reference to connection-owned encrypted material. It never stores a secret
// itself.
type HTTPConnectorAuthentication struct {
	Mode      string `json:"mode"`
	SecretRef string `json:"secretRef,omitempty"`
}

// HTTPConnectorFieldMapping maps one small semantic notification field to a
// top-level JSON property. Expressions, nested paths and raw body fragments
// are deliberately absent from this contract.
type HTTPConnectorFieldMapping struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// ParseHTTPConnectorConfig decodes the persisted representation with unknown
// fields rejected so it cannot become a script/plugin/raw-body escape hatch.
func ParseHTTPConnectorConfig(raw string) (HTTPConnectorConfig, error) {
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	var config HTTPConnectorConfig
	if err := decoder.Decode(&config); err != nil {
		return HTTPConnectorConfig{}, fmt.Errorf("parse HTTP connector configuration: %w", err)
	}
	if err := requireNoAdditionalJSONValue(decoder); err != nil {
		return HTTPConnectorConfig{}, err
	}
	return NormalizeHTTPConnectorConfig(config)
}

// EncodeHTTPConnectorConfig validates and serializes the only accepted
// internal storage shape. It is intentionally separate from free-form
// ConfigJSON request input.
func EncodeHTTPConnectorConfig(config HTTPConnectorConfig) (string, error) {
	normalized, err := NormalizeHTTPConnectorConfig(config)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode HTTP connector configuration: %w", err)
	}
	return string(encoded), nil
}

// NormalizeHTTPConnectorConfig validates the static G5.1 contract before a
// channel can persist it. Actual sending remains the G5.2 driver's job.
func NormalizeHTTPConnectorConfig(config HTTPConnectorConfig) (HTTPConnectorConfig, error) {
	config.EndpointURL = strings.TrimSpace(config.EndpointURL)
	if err := validateHTTPConnectorEndpoint(config.EndpointURL); err != nil {
		return HTTPConnectorConfig{}, err
	}
	config.EgressPolicyRef = strings.ToLower(strings.TrimSpace(config.EgressPolicyRef))
	if config.EgressPolicyRef != "" && !isHTTPConnectorIdentifier(config.EgressPolicyRef) {
		return HTTPConnectorConfig{}, fmt.Errorf("HTTP connector egress policy reference is invalid")
	}
	config.Method = strings.ToUpper(strings.TrimSpace(config.Method))
	if config.Method == "" {
		config.Method = HTTPConnectorMethodPOST
	}
	if config.Method != HTTPConnectorMethodPOST {
		return HTTPConnectorConfig{}, fmt.Errorf("HTTP connector method must be POST")
	}
	if err := normalizeHTTPConnectorAuthentication(&config.Authentication); err != nil {
		return HTTPConnectorConfig{}, err
	}
	if err := normalizeHTTPConnectorFieldMappings(&config.FieldMappings); err != nil {
		return HTTPConnectorConfig{}, err
	}
	if err := normalizeHTTPConnectorHeaderAllowlist(&config.HeaderAllowlist); err != nil {
		return HTTPConnectorConfig{}, err
	}
	config.IdempotencyHeader = strings.TrimSpace(config.IdempotencyHeader)
	if config.IdempotencyHeader == "" {
		config.IdempotencyHeader = HTTPConnectorIdempotencyHeader
	}
	if !strings.EqualFold(config.IdempotencyHeader, HTTPConnectorIdempotencyHeader) {
		return HTTPConnectorConfig{}, fmt.Errorf("HTTP connector idempotency header is fixed")
	}
	config.IdempotencyHeader = HTTPConnectorIdempotencyHeader
	if config.TimeoutMilliseconds < httpConnectorMinTimeoutMillis || config.TimeoutMilliseconds > httpConnectorMaxTimeoutMillis {
		return HTTPConnectorConfig{}, fmt.Errorf("HTTP connector timeout must be between %d and %d milliseconds", httpConnectorMinTimeoutMillis, httpConnectorMaxTimeoutMillis)
	}
	if err := normalizeHTTPConnectorSuccessCodes(&config.SuccessStatusCodes); err != nil {
		return HTTPConnectorConfig{}, err
	}
	return config, nil
}

func validateHTTPConnectorEndpoint(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || !parsed.IsAbs() || strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("HTTP connector endpoint URL is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, "https") || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" || strings.TrimSpace(parsed.Hostname()) == "" {
		return fmt.Errorf("HTTP connector endpoint must be an HTTPS URL without credentials, query or fragment")
	}
	return nil
}

func normalizeHTTPConnectorAuthentication(authentication *HTTPConnectorAuthentication) error {
	if authentication == nil {
		return fmt.Errorf("HTTP connector authentication is required")
	}
	authentication.Mode = strings.ToUpper(strings.TrimSpace(authentication.Mode))
	if authentication.Mode == "" {
		authentication.Mode = HTTPConnectorAuthNone
	}
	authentication.SecretRef = strings.ToUpper(strings.TrimSpace(authentication.SecretRef))
	switch authentication.Mode {
	case HTTPConnectorAuthNone:
		if authentication.SecretRef != "" {
			return fmt.Errorf("HTTP connector without authentication cannot reference a secret")
		}
	case HTTPConnectorAuthBearer, HTTPConnectorAuthBasic, HTTPConnectorAuthHMACSHA256:
		if authentication.SecretRef != HTTPConnectorSecretRefConnection {
			return fmt.Errorf("HTTP connector authentication must reference the connection secret")
		}
	case HTTPConnectorAuthMTLS:
		return fmt.Errorf("HTTP connector mTLS is unavailable until a versioned certificate bundle is implemented")
	default:
		return fmt.Errorf("HTTP connector authentication mode is unsupported")
	}
	return nil
}

func normalizeHTTPConnectorFieldMappings(mappings *[]HTTPConnectorFieldMapping) error {
	if mappings == nil || len(*mappings) == 0 || len(*mappings) > httpConnectorMaxFieldMappings {
		return fmt.Errorf("HTTP connector must declare between one and %d content field mappings", httpConnectorMaxFieldMappings)
	}
	seenTargets := make(map[string]struct{}, len(*mappings))
	for index := range *mappings {
		mapping := &(*mappings)[index]
		mapping.Source = strings.ToUpper(strings.TrimSpace(mapping.Source))
		if !validHTTPConnectorFieldSource(mapping.Source) {
			return fmt.Errorf("HTTP connector field source %q is unsupported", mapping.Source)
		}
		mapping.Target = strings.TrimSpace(mapping.Target)
		if !isHTTPConnectorIdentifier(mapping.Target) {
			return fmt.Errorf("HTTP connector field target %q is invalid", mapping.Target)
		}
		key := strings.ToLower(mapping.Target)
		if _, duplicate := seenTargets[key]; duplicate {
			return fmt.Errorf("HTTP connector field target %q is duplicated", mapping.Target)
		}
		seenTargets[key] = struct{}{}
	}
	return nil
}

func validHTTPConnectorFieldSource(source string) bool {
	switch source {
	case HTTPConnectorFieldSubject, HTTPConnectorFieldText, HTTPConnectorFieldEventKey, HTTPConnectorFieldCategory, HTTPConnectorFieldPriority, HTTPConnectorFieldTraceID, HTTPConnectorFieldDeepLink, HTTPConnectorFieldActionURL:
		return true
	default:
		return false
	}
}

func normalizeHTTPConnectorHeaderAllowlist(headers *[]string) error {
	if headers == nil || len(*headers) == 0 {
		return nil
	}
	if len(*headers) > httpConnectorMaxHeaders {
		return fmt.Errorf("HTTP connector header allowlist exceeds %d entries", httpConnectorMaxHeaders)
	}
	seen := make(map[string]struct{}, len(*headers))
	normalized := make([]string, 0, len(*headers))
	for _, rawHeader := range *headers {
		header := http.CanonicalHeaderKey(strings.TrimSpace(rawHeader))
		if !isSafeHTTPConnectorHeader(header) {
			return fmt.Errorf("HTTP connector header %q is not allowed", rawHeader)
		}
		key := strings.ToLower(header)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("HTTP connector header %q is duplicated", header)
		}
		seen[key] = struct{}{}
		normalized = append(normalized, header)
	}
	*headers = normalized
	return nil
}

func isSafeHTTPConnectorHeader(header string) bool {
	if header == "Accept" {
		return true
	}
	if !strings.HasPrefix(header, "X-Notification-") {
		return false
	}
	lower := strings.ToLower(header)
	for _, forbidden := range []string{"authorization", "proxy", "forwarded", "real-ip", "cookie", "token", "secret", "key", "credential"} {
		if strings.Contains(lower, forbidden) {
			return false
		}
	}
	return isHTTPConnectorHeaderToken(header)
}

func isHTTPConnectorHeaderToken(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizeHTTPConnectorSuccessCodes(codes *[]int) error {
	if codes == nil || len(*codes) == 0 {
		return nil // Empty means the fixed default: every 2xx status.
	}
	if len(*codes) > 16 {
		return fmt.Errorf("HTTP connector success status list exceeds 16 entries")
	}
	seen := make(map[int]struct{}, len(*codes))
	normalized := make([]int, 0, len(*codes))
	for _, code := range *codes {
		if code < http.StatusOK || code >= http.StatusMultipleChoices {
			return fmt.Errorf("HTTP connector success status %d must be 2xx", code)
		}
		if _, duplicate := seen[code]; duplicate {
			return fmt.Errorf("HTTP connector success status %d is duplicated", code)
		}
		seen[code] = struct{}{}
		normalized = append(normalized, code)
	}
	*codes = normalized
	return nil
}

func isHTTPConnectorIdentifier(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' || (character == '-' && index > 0 && index < len(value)-1) {
			continue
		}
		return false
	}
	return true
}

func requireNoAdditionalJSONValue(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("HTTP connector configuration must contain one JSON object")
		}
		return fmt.Errorf("parse HTTP connector configuration: %w", err)
	}
	return nil
}
