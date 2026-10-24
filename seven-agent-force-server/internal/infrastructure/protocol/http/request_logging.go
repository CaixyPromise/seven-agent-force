package http

import (
	"crypto/sha256"
	"encoding/hex"
	stdjson "encoding/json"
	"mime/multipart"
	"net/url"
	"regexp"
	"strings"

	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/cloudwego/hertz/pkg/app"
)

var requestLogAlwaysMaskedFields = []string{
	"captchaCode",
	"otpCode",
	"totpCode",
	"oneTimePassword",
	"emailCode",
	"recoveryCode",
	"credentialIdentifier",
	"clientDataJSON",
	"authenticatorData",
	"signature",
	"userHandle",
	"flowNonce",
	"operationBinding",
	"configValue",
	"isSensitive",
	"clientSecret",
	"managementBearer",
	"keyword",
	"sessionRef",
	"sessionRefs",
}

var requestLogExactMaskedFields = map[string]struct{}{
	"cloneurl":           {},
	"clone_url":          {},
	"code":               {},
	"authcode":           {},
	"auth_code":          {},
	"authorizationcode":  {},
	"authorization_code": {},
	"codeverifier":       {},
	"code_verifier":      {},
	"state":              {},
	"oauthstate":         {},
	"oauth_state":        {},
	"externalstate":      {},
	"external_state":     {},
	"oidcstate":          {},
	"oidc_state":         {},
	"nonce":              {},
	"oidcnonce":          {},
	"oidc_nonce":         {},
}

var requestLogExactMaskedTextFields = []string{
	"cloneurl",
	"clone_url",
	"code",
	"authcode",
	"auth_code",
	"authorizationcode",
	"authorization_code",
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

func buildRequestPayload(c *app.RequestContext, cfg config.RequestLoggingConfig) any {
	cfg = withRequestLogSensitiveBaseline(cfg)
	payload := map[string]any{}

	if cfg.IncludeQuery && c.QueryArgs().Len() > 0 {
		payload["query"] = finalizePayload(argsToMap(c.QueryArgs()), c.QueryArgs().QueryString(), cfg)
	}
	if isSensitiveDictMutationRequest(c) {
		body := c.Request.Body()
		sum := sha256.Sum256(body)
		payload["body"] = map[string]any{
			"kind":           "sensitive_dict_mutation",
			"content_length": len(body),
			"sha256":         hex.EncodeToString(sum[:]),
		}
		return payload
	}
	if isProviderInvocationRequest(c) {
		body := c.Request.Body()
		sum := sha256.Sum256(body)
		payload["body"] = map[string]any{
			"kind":           "provider_invocation",
			"content_length": len(body),
			"sha256":         hex.EncodeToString(sum[:]),
		}
		return payload
	}
	if isOpenTaskCreateRequest(c) {
		body := c.Request.Body()
		sum := sha256.Sum256(body)
		payload["body"] = map[string]any{
			"kind":           "open_task_submission",
			"content_length": len(body),
			"sha256":         hex.EncodeToString(sum[:]),
		}
		return payload
	}

	contentType := strings.ToLower(strings.TrimSpace(string(c.Request.Header.ContentType())))
	if contentType == "" {
		if len(payload) == 0 {
			return nil
		}
		return payload
	}

	if isSkippedContentType(contentType, cfg.SkipContentTypes) {
		if summary := summarizeSkippedBody(c, contentType, cfg); summary != nil {
			payload["body"] = summary
		}
		if len(payload) == 0 {
			return nil
		}
		return payload
	}

	switch {
	case strings.HasPrefix(contentType, "application/json"):
		body := c.Request.Body()
		if bodyPayload := summarizeJSONBody(body, cfg); bodyPayload != nil {
			payload["body"] = bodyPayload
		}
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		if c.PostArgs().Len() > 0 {
			payload["body"] = finalizePayload(argsToMap(c.PostArgs()), c.PostArgs().QueryString(), cfg)
		}
	default:
		body := c.Request.Body()
		if len(body) > 0 {
			payload["body"] = summarizeRawBody(body, cfg)
		}
	}

	if len(payload) == 0 {
		return nil
	}
	return payload
}

func isOpenTaskCreateRequest(c *app.RequestContext) bool {
	if c == nil || !strings.EqualFold(strings.TrimSpace(string(c.Method())), "POST") {
		return false
	}
	path := "/" + strings.Trim(strings.TrimSpace(string(c.Path())), "/")
	return path == "/api/open/workforce/v1/tasks" || path == "/open/workforce/v1/tasks"
}

func isSensitiveDictMutationRequest(c *app.RequestContext) bool {
	if c == nil || !strings.EqualFold(strings.TrimSpace(string(c.Method())), "POST") {
		return false
	}
	path := strings.Trim(strings.TrimSpace(string(c.Path())), "/")
	parts := strings.Split(path, "/")
	for index, part := range parts {
		if part != "dict" {
			continue
		}
		dictPath := parts[index:]
		if len(dictPath) >= 2 && dictPath[1] == "items" {
			if len(dictPath) == 3 && (dictPath[2] == "update" || dictPath[2] == "delete" ||
				dictPath[2] == "status" || dictPath[2] == "sort") {
				return true
			}
		}
		if len(dictPath) == 3 && dictPath[2] == "items" {
			return true
		}
		if len(dictPath) == 5 && dictPath[2] == "items" && dictPath[4] == "move" {
			return true
		}
	}
	return false
}

func isProviderInvocationRequest(c *app.RequestContext) bool {
	if c == nil || !strings.EqualFold(strings.TrimSpace(string(c.Method())), "POST") {
		return false
	}
	path := "/" + strings.Trim(strings.TrimSpace(string(c.Path())), "/")
	return path == "/api/internal/workforce/v1/provider-invocations/responses" ||
		path == "/api/internal/workforce/v1/provider-invocations/responses/compact"
}

func maskRawQuery(rawQuery string, cfg config.RequestLoggingConfig) string {
	cfg = withRequestLogSensitiveBaseline(cfg)
	if rawQuery == "" {
		return ""
	}
	parts := strings.Split(rawQuery, "&")
	for index, part := range parts {
		if part == "" {
			continue
		}
		key, value, hasValue := strings.Cut(part, "=")
		decodedKey, err := url.QueryUnescape(key)
		if err != nil {
			decodedKey = key
		}
		if isRequestLogExactMaskedField(decodedKey) || isSensitiveQueryKey(decodedKey, cfg) {
			parts[index] = key + "=******"
			continue
		}
		if hasValue {
			parts[index] = key + "=" + jsoncompat.MaskSensitiveText(value, cfg.MaskedFields, cfg.MaxFieldLength)
		}
	}
	return strings.Join(parts, "&")
}

func isSensitiveQueryKey(key string, cfg config.RequestLoggingConfig) bool {
	masked := jsoncompat.MaskSensitiveFields(map[string]any{key: "probe-value"}, cfg.MaskedFields, cfg.MaxFieldLength)
	items, ok := masked.(map[string]any)
	if !ok {
		return false
	}
	return items[key] == "******"
}

func argsToMap(args interface{ VisitAll(func(key, value []byte)) }) map[string]any {
	result := map[string]any{}
	args.VisitAll(func(key, value []byte) {
		name := string(key)
		current, exists := result[name]
		if !exists {
			result[name] = string(value)
			return
		}
		switch typed := current.(type) {
		case []any:
			result[name] = append(typed, string(value))
		default:
			result[name] = []any{typed, string(value)}
		}
	})
	return result
}

func summarizeJSONBody(body []byte, cfg config.RequestLoggingConfig) any {
	if len(body) == 0 {
		return nil
	}

	var parsed any
	if err := stdjson.Unmarshal(body, &parsed); err != nil {
		return summarizeRawBody(body, cfg)
	}
	return finalizePayload(parsed, body, cfg)
}

func summarizeRawBody(body []byte, cfg config.RequestLoggingConfig) map[string]any {
	clipped, truncated := jsoncompat.ClipLargePayload(body, cfg.MaxBodyBytes)
	raw := maskRequestLogExactText(string(clipped))
	return map[string]any{
		"raw":            jsoncompat.MaskSensitiveText(raw, cfg.MaskedFields, cfg.MaxFieldLength),
		"truncated":      truncated,
		"content_length": len(body),
	}
}

func finalizePayload(value any, raw []byte, cfg config.RequestLoggingConfig) any {
	sanitized := jsoncompat.MaskSensitiveFields(maskRequestLogExactFields(value), cfg.MaskedFields, cfg.MaxFieldLength)
	if maxBytes := cfg.MaxBodyBytes; maxBytes > 0 {
		if encoded, err := stdjson.Marshal(jsoncompat.NormalizeForJSON(sanitized)); err == nil && len(encoded) > maxBytes {
			clipped, truncated := jsoncompat.ClipLargePayload(encoded, maxBytes)
			return map[string]any{
				"raw":            truncateField(string(clipped), cfg.MaxFieldLength),
				"truncated":      truncated,
				"content_length": len(raw),
			}
		}
	}
	return sanitized
}

func summarizeSkippedBody(c *app.RequestContext, contentType string, cfg config.RequestLoggingConfig) any {
	summary := map[string]any{
		"kind":           "binary",
		"content_type":   contentType,
		"content_length": c.Request.Header.ContentLength(),
	}

	if strings.HasPrefix(contentType, "multipart/form-data") {
		form, err := c.MultipartForm()
		if err != nil {
			summary["parse_error"] = err.Error()
			return summary
		}
		fields := map[string]any{}
		for key, values := range form.Value {
			if len(values) == 1 {
				fields[key] = values[0]
				continue
			}
			items := make([]any, len(values))
			for i := range values {
				items[i] = values[i]
			}
			fields[key] = items
		}
		summary["fields"] = jsoncompat.MaskSensitiveFields(maskRequestLogExactFields(fields), cfg.MaskedFields, cfg.MaxFieldLength)
		summary["files"] = summarizeFiles(form.File)
		summary["kind"] = "multipart"
	}
	return summary
}

func summarizeFiles(files map[string][]*multipart.FileHeader) any {
	result := make([]map[string]any, 0)
	for field, headers := range files {
		for _, header := range headers {
			result = append(result, map[string]any{
				"field":    field,
				"filename": header.Filename,
				"size":     header.Size,
			})
		}
	}
	return jsoncompat.NormalizeForJSON(result)
}

func isSkippedContentType(contentType string, skipContentTypes []string) bool {
	for _, item := range skipContentTypes {
		if strings.HasPrefix(contentType, strings.ToLower(strings.TrimSpace(item))) {
			return true
		}
	}
	return false
}

func truncateField(value string, maxFieldLength int) string {
	return jsoncompat.MaskSensitiveText(value, nil, maxFieldLength)
}

func maskRequestLogExactFields(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isRequestLogExactMaskedField(key) {
				result[key] = "******"
				continue
			}
			result[key] = maskRequestLogExactFields(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			result[index] = maskRequestLogExactFields(item)
		}
		return result
	default:
		return value
	}
}

func maskRequestLogExactText(value string) string {
	masked := value
	for _, field := range requestLogExactMaskedTextFields {
		marker := regexp.QuoteMeta(field)
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`(?i)(^|[{\s,;?&])(` + marker + `\s*[:=]\s*)("[^"]*"|'[^']*'|[^,\s;&}\]]+)`),
			regexp.MustCompile(`(?i)("` + marker + `"\s*:\s*)("[^"]*"|[0-9]+|true|false|null)`),
		}
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
	}
	return masked
}

func isRequestLogExactMaskedField(name string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	_, ok := requestLogExactMaskedFields[normalized]
	return ok
}

func withRequestLogSensitiveBaseline(cfg config.RequestLoggingConfig) config.RequestLoggingConfig {
	seen := make(map[string]struct{}, len(cfg.MaskedFields)+len(requestLogAlwaysMaskedFields))
	for _, item := range cfg.MaskedFields {
		key := strings.ToLower(strings.TrimSpace(item))
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, item := range requestLogAlwaysMaskedFields {
		key := strings.ToLower(strings.TrimSpace(item))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		cfg.MaskedFields = append(cfg.MaskedFields, item)
		seen[key] = struct{}{}
	}
	return cfg
}
