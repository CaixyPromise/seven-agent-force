package errors

import (
	"errors"
	"net/http"
	"strings"
)

const (
	CodeSuccess            = 0
	CodeParamsError        = 40000
	CodeNotLogin           = 40100
	CodeNoAuth             = 40101
	CodeChallengeRequired  = 40120
	CodeForbidden          = 40300
	CodeDataScopeDenied    = 40310
	CodeNotFound           = 40400
	CodeObjectStateInvalid = 40900
	CodeRateLimited        = 42900
	CodeSystemError        = 50000
	CodeOperateError       = 50001
	CodeServiceUnavailable = 50300
)

type Kind string

const (
	KindParams             Kind = "params_error"
	KindAuth               Kind = "auth"
	KindForbidden          Kind = "forbidden"
	KindDataScopeDenied    Kind = "data_scope_denied"
	KindMethodNotAllowed   Kind = "method_not_allowed"
	KindNotFound           Kind = "not_found"
	KindObjectState        Kind = "object_state_invalid"
	KindRateLimited        Kind = "rate_limited"
	KindSystem             Kind = "system"
	KindServiceUnavailable Kind = "service_unavailable"
	KindOperation          Kind = "operation_failed"
	KindChallengeThrottle  Kind = "challenge_throttled"
)

type Error interface {
	error
	Code() int
	Message() string
	Kind() Kind
	Details() any
}

type AppError struct {
	code    int
	kind    Kind
	message string
	details any
	cause   error
}

func New(code int, kind Kind, message string) *AppError {
	return &AppError{
		code:    code,
		kind:    kind,
		message: message,
	}
}

func Wrap(code int, kind Kind, message string, cause error) *AppError {
	return &AppError{
		code:    code,
		kind:    kind,
		message: message,
		cause:   cause,
	}
}

func Params(message string) *AppError {
	if message == "" {
		message = "请求参数错误"
	}
	return New(CodeParamsError, KindParams, message)
}

func ParamsWithDetails(message string, details any) *AppError {
	return Params(message).WithDetails(details)
}

func Unauthorized(message string) *AppError {
	if message == "" {
		message = "未登录"
	}
	return New(CodeNotLogin, KindAuth, message)
}

func ChallengeRequired(message string, details any) *AppError {
	if message == "" {
		message = "需要额外挑战验证"
	}
	return New(CodeChallengeRequired, KindAuth, message).WithDetails(details)
}

func Forbidden(message string) *AppError {
	if message == "" {
		message = "无权限"
	}
	return New(CodeForbidden, KindForbidden, message)
}

func PermissionDenied(permission string) *AppError {
	return Forbidden("无权限").WithDetails(map[string]string{
		"requiredPermission": strings.TrimSpace(permission),
		"reasonCode":         "PERMISSION_NOT_GRANTED",
	})
}

func DataScopeDenied(message string) *AppError {
	if message == "" {
		message = "数据范围不足"
	}
	return New(CodeDataScopeDenied, KindDataScopeDenied, message)
}

func MethodNotAllowed(message string) *AppError {
	if message == "" {
		message = "请求方法不被允许"
	}
	return New(CodeForbidden, KindMethodNotAllowed, message)
}

func NotFound(message string) *AppError {
	if message == "" {
		message = "请求路径不存在"
	}
	return New(CodeNotFound, KindNotFound, message)
}

func System(message string) *AppError {
	if message == "" {
		message = "系统内部异常"
	}
	return New(CodeSystemError, KindSystem, message)
}

func ServiceUnavailable(message string) *AppError {
	if message == "" {
		message = "服务暂时不可用"
	}
	return New(CodeServiceUnavailable, KindServiceUnavailable, message)
}

func Operation(message string) *AppError {
	if message == "" {
		message = "操作失败"
	}
	return New(CodeOperateError, KindOperation, message)
}

func ChallengeThrottled(message string) *AppError {
	if message == "" {
		message = "挑战触发过于频繁，请稍后再试"
	}
	return New(CodeRateLimited, KindChallengeThrottle, message)
}

func RateLimited(message string) *AppError {
	if message == "" {
		message = "请求过于频繁，请稍后再试"
	}
	return New(CodeRateLimited, KindRateLimited, message)
}

func ObjectState(message string) *AppError {
	if message == "" {
		message = "当前状态不允许操作"
	}
	return New(CodeObjectStateInvalid, KindObjectState, message)
}

func (e *AppError) WithDetails(details any) *AppError {
	if e == nil {
		return nil
	}
	e.details = details
	return e
}

func (e *AppError) Error() string {
	return e.message
}

func (e *AppError) Code() int {
	return e.code
}

func (e *AppError) Message() string {
	return e.message
}

func (e *AppError) Kind() Kind {
	return e.kind
}

func (e *AppError) Details() any {
	return e.details
}

func (e *AppError) Unwrap() error {
	return e.cause
}

func From(err error) *AppError {
	if err == nil {
		return nil
	}
	if appErr, ok := err.(*AppError); ok {
		return appErr
	}
	var coded Error
	if errors.As(err, &coded) {
		return &AppError{
			code:    coded.Code(),
			kind:    coded.Kind(),
			message: coded.Message(),
			details: coded.Details(),
			cause:   err,
		}
	}
	return Wrap(CodeSystemError, KindSystem, "系统内部异常", err)
}

func HTTPStatus(err error) int {
	appErr := From(err)
	if appErr == nil {
		return http.StatusOK
	}
	switch appErr.Kind() {
	case KindNotFound:
		return http.StatusNotFound
	case KindMethodNotAllowed:
		return http.StatusMethodNotAllowed
	case KindRateLimited, KindChallengeThrottle:
		return http.StatusTooManyRequests
	case KindServiceUnavailable:
		return http.StatusServiceUnavailable
	default:
		return http.StatusOK
	}
}

func IsClientDisconnect(err error) bool {
	if err == nil {
		return false
	}
	for current := err; current != nil; current = errors.Unwrap(current) {
		message := strings.ToLower(strings.TrimSpace(current.Error()))
		if strings.Contains(message, "broken pipe") ||
			strings.Contains(message, "connection reset by peer") ||
			strings.Contains(message, "asyncrequestnotusableexception") ||
			strings.Contains(message, "client disconnected") ||
			strings.Contains(message, "client abort") ||
			strings.Contains(message, "use of closed network connection") {
			return true
		}
	}
	return false
}
