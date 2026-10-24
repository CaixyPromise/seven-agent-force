package response

import (
	"net/http"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	jsoncompat "github.com/CaixyPromise/seven-agent/force/internal/common/json"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
)

type Result struct {
	Code    int    `json:"code"`
	Data    any    `json:"data"`
	Message string `json:"message"`
	TraceID string `json:"traceId,omitempty"`
}

func Success(c *app.RequestContext, data any) {
	SuccessWithStatus(c, http.StatusOK, data)
}

// SuccessWithStatus writes Framework's success envelope with an explicit HTTP
// status while preserving its JSON compatibility normalization.
func SuccessWithStatus(c *app.RequestContext, status int, data any) {
	write(c, status, Result{
		Code:    apperrors.CodeSuccess,
		Data:    jsoncompat.NormalizeForJSON(data),
		Message: "ok",
		TraceID: xcontext.TraceID(c),
	})
}

func Error(c *app.RequestContext, err error) {
	ErrorWithStatus(c, 0, err)
}

// ErrorWithStatus is reserved for APIs whose public contract requires a
// transport-level status in addition to Framework's stable business code.
// A zero status preserves the Framework-wide status mapping.
func ErrorWithStatus(c *app.RequestContext, status int, err error) {
	appErr := apperrors.From(err)
	xcontext.SetResponseError(c, appErr.Code(), appErr.Message())
	if appErr.Code() == apperrors.CodeSystemError {
		if cause := strings.TrimSpace(err.Error()); cause != "" && cause != appErr.Message() {
			xcontext.SetResponseErrorCause(c, cause)
		}
	}
	if status == 0 {
		status = apperrors.HTTPStatus(appErr)
	}
	write(c, status, Result{
		Code:    appErr.Code(),
		Data:    jsoncompat.NormalizeForJSON(appErr.Details()),
		Message: appErr.Message(),
		TraceID: xcontext.TraceID(c),
	})
}

// ErrorWithReason writes the standard numeric Framework envelope while keeping
// a stable, machine-readable business reason in data.reasonCode. Callers may
// add safe details, but cannot replace the reason supplied here.
func ErrorWithReason(c *app.RequestContext, status int, err error, reasonCode string, details map[string]any) {
	appErr := apperrors.From(err)
	safeDetails := make(map[string]any, len(details)+1)
	for key, value := range details {
		safeDetails[key] = value
	}
	safeDetails["reasonCode"] = strings.TrimSpace(reasonCode)
	ErrorWithStatus(c, status, appErr.WithDetails(safeDetails))
}

func write(c *app.RequestContext, status int, body Result) {
	c.JSON(status, body)
}
