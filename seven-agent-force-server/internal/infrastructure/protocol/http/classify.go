package http

import (
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/cloudwego/hertz/pkg/app"
)

func isClientDisconnect(err error) bool {
	return apperrors.IsClientDisconnect(err)
}

func isStreamCommitted(c *app.RequestContext) bool {
	if c == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(string(c.Response.Header.ContentType())))
	return c.Response.IsBodyStream() ||
		c.Response.HasBodyBytes() ||
		strings.Contains(contentType, "text/event-stream")
}
