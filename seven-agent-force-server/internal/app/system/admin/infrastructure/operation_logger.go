package infrastructure

import (
	"context"
	stdjson "encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type OperationLogger struct {
	service adminfacade.OperationLogFacade
	cfg     config.RequestLoggingConfig
}

func NewOperationLogger(service adminfacade.OperationLogFacade, cfg config.RequestLoggingConfig) *OperationLogger {
	if service == nil {
		return nil
	}
	return &OperationLogger{service: service, cfg: cfg}
}

func (l *OperationLogger) Wrap(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if l == nil || l.service == nil {
		return handler
	}
	return func(ctx context.Context, reqCtx *app.RequestContext) {
		startedAt := time.Now().UTC()
		entry := adminfacade.OperationLogEntry{
			OperationType: spec.Operation,
			OperationDesc: strings.TrimSpace(spec.Description),
			MethodName:    strings.TrimSpace(string(reqCtx.Path())),
			RequestMethod: strings.TrimSpace(string(reqCtx.Method())),
			RequestURL:    buildRequestURL(reqCtx),
			TraceID:       xcontext.TraceID(reqCtx),
			RequestIP:     xcontext.ResolveClientIP(reqCtx),
			UserAgent:     strings.TrimSpace(string(reqCtx.UserAgent())),
			OperationTime: startedAt,
			Status:        1,
		}
		if spec.OmitQuery {
			entry.RequestURL = strings.TrimSpace(string(reqCtx.Path()))
		}
		if entry.OperationDesc == "" {
			entry.OperationDesc = spec.Operation.Description()
		}
		if user := securitycontext.Get(reqCtx); user != nil && !user.IsAnonymous {
			entry.UserID = user.UserID
			entry.UserName = strings.TrimSpace(user.Username)
			entry.NickName = strings.TrimSpace(user.Nickname)
		}
		entry.Browser, entry.OS = detectUserAgent(entry.UserAgent)
		if spec.IncludeParams {
			entry.RequestParams = l.captureRequestParams(reqCtx)
			if entry.UserName == "" && spec.Operation == adminfacade.OperationTypeUserLogin {
				entry.UserName = extractUserAccount(entry.RequestParams)
			}
		}
		for _, enricher := range spec.Enrichers {
			if enricher != nil {
				enricher.Enrich(ctx, reqCtx, &entry)
			}
		}

		defer func() {
			entry.ExecutionTime = time.Since(startedAt).Milliseconds()
			if recovered := recover(); recovered != nil {
				entry.Status = 0
				entry.ErrorMsg = fmt.Sprint(recovered)
				if spec.IncludeResult {
					entry.ResponseResult = l.captureResponseResult(reqCtx)
				}
				for _, enricher := range spec.CompletionEnrichers {
					if enricher != nil {
						enricher.Enrich(ctx, reqCtx, &entry)
					}
				}
				entry.RequestParams = appendStepUpProofAuditMetadata(entry.RequestParams, reqCtx, l.cfg)
				entry.RequestParams = appendLoginPunishmentAuditMetadata(entry.RequestParams, reqCtx, l.cfg)
				l.service.SaveLogAsync(context.Background(), entry)
				panic(recovered)
			}
			if code, message := xcontext.ResponseError(reqCtx); code != 0 {
				entry.Status = 0
				entry.ErrorMsg = message
			} else if statusCode := reqCtx.Response.StatusCode(); statusCode >= 400 {
				entry.Status = 0
				entry.ErrorMsg = apperrors.From(apperrors.Operation(fmt.Sprintf("HTTP %d", statusCode))).Message()
			}
			applyLoginPunishmentAuditStatus(reqCtx, &entry)
			if spec.IncludeResult {
				entry.ResponseResult = l.captureResponseResult(reqCtx)
			}
			for _, enricher := range spec.CompletionEnrichers {
				if enricher != nil {
					enricher.Enrich(ctx, reqCtx, &entry)
				}
			}
			entry.RequestParams = appendStepUpProofAuditMetadata(entry.RequestParams, reqCtx, l.cfg)
			entry.RequestParams = appendLoginPunishmentAuditMetadata(entry.RequestParams, reqCtx, l.cfg)
			l.service.SaveLogAsync(context.Background(), entry)
		}()

		handler(ctx, reqCtx)
	}
}

func applyLoginPunishmentAuditStatus(reqCtx *app.RequestContext, entry *adminfacade.OperationLogEntry) {
	if entry == nil || entry.OperationType != adminfacade.OperationTypeUserLogin {
		return
	}
	audit, ok := securitycontext.GetLoginPunishmentAudit(reqCtx)
	if !ok {
		return
	}
	if strings.EqualFold(audit.Outcome, "authenticated") {
		return
	}
	entry.Status = 0
	if strings.TrimSpace(entry.ErrorMsg) == "" && strings.TrimSpace(audit.Outcome) != "" {
		entry.ErrorMsg = "login outcome: " + strings.TrimSpace(audit.Outcome)
	}
}

func appendStepUpProofAuditMetadata(existing string, reqCtx *app.RequestContext, cfg config.RequestLoggingConfig) string {
	metadata, ok := securitycontext.StepUpProofAuditMetadata(reqCtx)
	if !ok {
		return existing
	}
	payload := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		if err := stdjson.Unmarshal([]byte(existing), &payload); err != nil {
			payload["raw"] = maskInlineSecrets(existing, cfg, "raw")
		}
	}
	payload["stepUpProof"] = sanitizePayload(metadata, cfg)
	return marshalPayload(payload)
}

func appendLoginPunishmentAuditMetadata(existing string, reqCtx *app.RequestContext, cfg config.RequestLoggingConfig) string {
	metadata, ok := securitycontext.LoginPunishmentAuditMetadata(reqCtx)
	if !ok {
		return existing
	}
	payload := map[string]any{}
	if strings.TrimSpace(existing) != "" {
		if err := stdjson.Unmarshal([]byte(existing), &payload); err != nil {
			payload["raw"] = maskInlineSecrets(existing, cfg, "raw")
		}
	}
	payload["loginPunishment"] = sanitizePayload(metadata, cfg)
	return marshalPayload(payload)
}

func (l *OperationLogger) captureRequestParams(reqCtx *app.RequestContext) string {
	if reqCtx == nil {
		return ""
	}
	payload := map[string]any{}
	if reqCtx.QueryArgs().Len() > 0 {
		payload["query"] = sanitizePayload(argsToMap(reqCtx.QueryArgs()), l.cfg)
	}
	contentType := strings.ToLower(strings.TrimSpace(string(reqCtx.Request.Header.ContentType())))
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		if body := strings.TrimSpace(string(reqCtx.Request.Body())); body != "" {
			payload["body"] = sanitizeJSONOrRaw([]byte(body), l.cfg)
		}
	case strings.HasPrefix(contentType, "application/x-www-form-urlencoded"):
		if reqCtx.PostArgs().Len() > 0 {
			payload["body"] = sanitizePayload(argsToMap(reqCtx.PostArgs()), l.cfg)
		}
	case contentType != "" && !shouldSkipContentType(contentType, l.cfg.SkipContentTypes):
		if body := strings.TrimSpace(string(reqCtx.Request.Body())); body != "" {
			payload["body"] = sanitizeJSONOrRaw([]byte(body), l.cfg)
		}
	}
	return marshalPayload(payload)
}

func (l *OperationLogger) captureResponseResult(reqCtx *app.RequestContext) string {
	if reqCtx == nil {
		return ""
	}
	contentType := strings.ToLower(strings.TrimSpace(string(reqCtx.Response.Header.ContentType())))
	if strings.Contains(contentType, "text/event-stream") || strings.Contains(contentType, "application/octet-stream") {
		return ""
	}
	if disposition := strings.TrimSpace(string(reqCtx.Response.Header.Peek("Content-Disposition"))); disposition != "" {
		return ""
	}
	body := reqCtx.Response.Body()
	if len(body) == 0 {
		return ""
	}
	return marshalPayload(map[string]any{
		"status": reqCtx.Response.StatusCode(),
		"body":   sanitizeJSONOrRaw(body, l.cfg),
	})
}

func sanitizeJSONOrRaw(raw []byte, cfg config.RequestLoggingConfig) any {
	if cfg.MaxBodyBytes > 0 && len(raw) > cfg.MaxBodyBytes {
		marker := "malformed_json_omitted"
		if stdjson.Valid(raw) {
			marker = "valid_json_omitted"
		}
		return map[string]any{"raw": marker, "truncated": true}
	}
	var parsed any
	if err := stdjson.Unmarshal(raw, &parsed); err == nil {
		return sanitizePayload(parsed, cfg)
	}
	_, truncated := jsoncompat.ClipLargePayload(raw, cfg.MaxBodyBytes)
	return map[string]any{
		"raw":       "malformed_json_omitted",
		"truncated": truncated,
	}
}

func sanitizePayload(value any, cfg config.RequestLoggingConfig) any {
	return sanitizeOperationPayload(jsoncompat.NormalizeForJSON(value), cfg, "")
}

func sanitizeOperationPayload(value any, cfg config.RequestLoggingConfig, fieldName string) any {
	if shouldMaskOperationField(fieldName, cfg.MaskedFields) {
		return "******"
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = sanitizeOperationPayload(child, cfg, key)
		}
		if keyName, ok := stringFromAny(result["key"]); ok && isMaskedFieldName(keyName, cfg.MaskedFields) {
			result["value"] = "******"
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for i := range typed {
			result[i] = sanitizeOperationPayload(typed[i], cfg, fieldName)
		}
		return result
	case string:
		return maskInlineSecrets(typed, cfg, fieldName)
	default:
		return value
	}
}

func stringFromAny(value any) (string, bool) {
	typed, ok := value.(string)
	return typed, ok
}

func isMaskedFieldName(name string, maskedFields []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if isOperationVisibleField(normalized) {
		return false
	}
	if normalized == "" {
		return false
	}
	for _, marker := range operationLogSensitiveMarkers(maskedFields) {
		if marker != "" && strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func shouldMaskOperationField(name string, maskedFields []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" || isOperationVisibleField(normalized) {
		return false
	}
	switch normalized {
	case "code", "authorization_code", "issensitive":
		return true
	}
	return isMaskedFieldName(normalized, maskedFields)
}

func isOperationVisibleField(normalized string) bool {
	return normalized == "configkey" || normalized == "config_key" || normalized == "key"
}

func operationLogSensitiveMarkers(maskedFields []string) []string {
	result := []string{
		"password",
		"passwd",
		"pwd",
		"token",
		"secret",
		"credential",
		"captcha",
		"code_verifier",
		"codeverifier",
		"access_token",
		"refreshtoken",
		"refresh_token",
		"id_token",
		"otp",
		"one_time_password",
		"onetimepassword",
		"signature",
		"clientdata",
		"client_data",
		"authenticatordata",
		"authenticator_data",
		"assertion",
		"authorization",
		"bearer",
		"code",
		"authcode",
		"auth_code",
		"authorizationcode",
		"authorization_code",
		"private_key",
		"apikey",
		"api_key",
		"access_key",
		"configvalue",
		"config_value",
		"sessionref",
		"sessionrefs",
		"session_ref",
		"session_refs",
	}
	for _, item := range maskedFields {
		normalized := strings.ToLower(strings.TrimSpace(item))
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func maskInlineSecrets(value string, cfg config.RequestLoggingConfig, fieldName string) string {
	masked := value
	if isMaskedFieldName(fieldName, cfg.MaskedFields) {
		return "******"
	}
	for _, marker := range operationLogSensitiveMarkers(cfg.MaskedFields) {
		if marker == "" {
			continue
		}
		patterns := []*regexp.Regexp{
			regexp.MustCompile(`(?i)("` + regexp.QuoteMeta(marker) + `"\s*:\s*)("[^"]*"|'[^']*'|[0-9]+|true|false|null|[^\s,;}\]]+)`),
			regexp.MustCompile(`(?i)(^|[{\s,;])(` + regexp.QuoteMeta(marker) + `\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;}\]]+)`),
			regexp.MustCompile(`(?i)(^|[{\s,;])(` + regexp.QuoteMeta(marker) + `\s+)([^\s,;}\]]+)`),
		}
		if !isExactOperationLogTextMarker(marker) {
			patterns = append(patterns,
				regexp.MustCompile(`(?i)("[A-Za-z0-9_.-]*`+regexp.QuoteMeta(marker)+`[A-Za-z0-9_.-]*"\s*:\s*)("[^"]*"|'[^']*'|[0-9]+|true|false|null|[^\s,;}\]]+)`),
				regexp.MustCompile(`(?i)(^|[{\s,;])([A-Za-z0-9_.-]*`+regexp.QuoteMeta(marker)+`[A-Za-z0-9_.-]*\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;}\]]+)`),
			)
		}
		for _, pattern := range patterns {
			masked = pattern.ReplaceAllStringFunc(masked, func(match string) string {
				groups := pattern.FindStringSubmatch(match)
				switch len(groups) {
				case 3:
					return groups[1] + `"******"`
				case 4:
					return groups[1] + groups[2] + `******`
				default:
					return match
				}
			})
		}
	}
	return masked
}

func isExactOperationLogTextMarker(marker string) bool {
	switch strings.ToLower(strings.TrimSpace(marker)) {
	case "code", "authcode", "auth_code", "authorizationcode", "authorization_code":
		return true
	default:
		return false
	}
}

func marshalPayload(value any) string {
	if value == nil {
		return ""
	}
	typed, ok := value.(map[string]any)
	if ok && len(typed) == 0 {
		return ""
	}
	raw, err := stdjson.Marshal(jsoncompat.NormalizeForJSON(value))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
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

func shouldSkipContentType(contentType string, skipContentTypes []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(contentType))
	for _, item := range skipContentTypes {
		if candidate := strings.ToLower(strings.TrimSpace(item)); candidate != "" && strings.HasPrefix(normalized, candidate) {
			return true
		}
	}
	return false
}

func buildRequestURL(reqCtx *app.RequestContext) string {
	if reqCtx == nil {
		return ""
	}
	path := strings.TrimSpace(string(reqCtx.Path()))
	query := strings.TrimSpace(string(reqCtx.Request.URI().QueryString()))
	if query == "" {
		return path
	}
	return path + "?" + query
}

func detectUserAgent(userAgent string) (string, string) {
	value := strings.ToLower(strings.TrimSpace(userAgent))
	browser := "Unknown"
	switch {
	case strings.Contains(value, "edg/"):
		browser = "Edge"
	case strings.Contains(value, "chrome/"):
		browser = "Chrome"
	case strings.Contains(value, "firefox/"):
		browser = "Firefox"
	case strings.Contains(value, "safari/") && !strings.Contains(value, "chrome/"):
		browser = "Safari"
	}
	os := "Unknown"
	switch {
	case strings.Contains(value, "windows"):
		os = "Windows"
	case strings.Contains(value, "mac os x"):
		os = "macOS"
	case strings.Contains(value, "android"):
		os = "Android"
	case strings.Contains(value, "iphone"), strings.Contains(value, "ipad"), strings.Contains(value, "ios"):
		os = "iOS"
	case strings.Contains(value, "linux"):
		os = "Linux"
	}
	return browser, os
}

func extractUserAccount(requestParams string) string {
	if strings.TrimSpace(requestParams) == "" {
		return ""
	}
	var payload map[string]any
	if err := stdjson.Unmarshal([]byte(requestParams), &payload); err != nil {
		return ""
	}
	body, ok := payload["body"].(map[string]any)
	if !ok {
		return ""
	}
	if value := strings.TrimSpace(toString(body["userAccount"])); value != "" {
		return value
	}
	return strings.TrimSpace(toString(body["username"]))
}

func toString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}
