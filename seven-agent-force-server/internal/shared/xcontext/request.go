package xcontext

import (
	"net"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

const (
	ResponseCodeKey    = "response_code"
	ResponseMessageKey = "response_message"
	ResponseCauseKey   = "response_cause"
)

type RequestMeta struct {
	TraceID       string
	Method        string
	Path          string
	RawQuery      string
	ClientIP      string
	UserAgent     string
	ContentType   string
	ContentLength int
}

func BuildRequestMeta(c *app.RequestContext) RequestMeta {
	return RequestMeta{
		TraceID:       EnsureTraceID(c),
		Method:        string(c.Method()),
		Path:          string(c.Path()),
		RawQuery:      string(c.Request.URI().QueryString()),
		ClientIP:      ResolveClientIP(c),
		UserAgent:     string(c.UserAgent()),
		ContentType:   string(c.Request.Header.ContentType()),
		ContentLength: c.Request.Header.ContentLength(),
	}
}

func ResolveClientIP(c *app.RequestContext) string {
	if c == nil {
		return ""
	}

	for _, header := range []string{
		"X-Forwarded-For",
		"Proxy-Client-IP",
		"WL-Proxy-Client-IP",
		"X-Real-IP",
	} {
		value := normalizeIP(c.Request.Header.Get(header))
		if value != "" {
			return value
		}
	}

	if remoteAddr := c.RemoteAddr(); remoteAddr != nil {
		host, _, err := net.SplitHostPort(remoteAddr.String())
		if err == nil {
			return normalizeLoopback(host)
		}
		return normalizeLoopback(strings.TrimSpace(remoteAddr.String()))
	}
	return ""
}

func SetResponseError(c *app.RequestContext, code int, message string) {
	if c == nil {
		return
	}
	c.Set(ResponseCodeKey, code)
	c.Set(ResponseMessageKey, message)
}

func ResponseError(c *app.RequestContext) (int, string) {
	if c == nil {
		return 0, ""
	}
	if code, ok := c.Get(ResponseCodeKey); ok {
		if typed, ok := code.(int); ok {
			return typed, c.GetString(ResponseMessageKey)
		}
	}
	return 0, ""
}

func SetResponseErrorCause(c *app.RequestContext, cause string) {
	if c == nil || strings.TrimSpace(cause) == "" {
		return
	}
	c.Set(ResponseCauseKey, strings.TrimSpace(cause))
}

func ResponseErrorCause(c *app.RequestContext) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.GetString(ResponseCauseKey))
}

func normalizeIP(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "unknown") {
		return ""
	}
	if strings.Contains(trimmed, ",") {
		parts := strings.Split(trimmed, ",")
		for _, part := range parts {
			candidate := normalizeIP(part)
			if candidate != "" {
				return candidate
			}
		}
		return ""
	}
	return normalizeLoopback(trimmed)
}

func normalizeLoopback(value string) string {
	trimmed := strings.Trim(strings.TrimSpace(value), "[]")
	if trimmed == "::1" || trimmed == "0:0:0:0:0:0:0:1" {
		return "127.0.0.1"
	}
	return trimmed
}
