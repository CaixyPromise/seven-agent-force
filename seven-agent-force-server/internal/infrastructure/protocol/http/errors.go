package http

import (
	"fmt"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xpanic"
	"github.com/cloudwego/hertz/pkg/app"
	"go.uber.org/zap"
)

func writeAppError(c *app.RequestContext, err error) {
	response.Error(c, err)
}

func handleRecovered(log *zap.Logger, c *app.RequestContext, recovered any) {
	err := fmt.Errorf("%v", recovered)
	if appErr, ok := recovered.(error); ok {
		err = appErr
	}

	if isClientDisconnect(err) {
		log.Debug("request_client_disconnect",
			zap.String("method", string(c.Method())),
			zap.String("path", string(c.Path())),
			zap.String("client_ip", xcontext.ResolveClientIP(c)),
			zap.Error(err),
		)
		return
	}

	if isStreamCommitted(c) {
		log.Debug("request_stream_write_aborted",
			zap.String("method", string(c.Method())),
			zap.String("path", string(c.Path())),
			zap.String("client_ip", xcontext.ResolveClientIP(c)),
			zap.Any("panic", recovered),
		)
		return
	}

	log.Error("request_panic_recovered",
		zap.Any("panic", recovered),
		zap.String("method", string(c.Method())),
		zap.String("path", string(c.Path())),
		zap.String("client_ip", xcontext.ResolveClientIP(c)),
		zap.String("stack", xpanic.Stack()),
	)
	writeAppError(c, apperrors.System("系统内部异常"))
}
