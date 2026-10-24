package handler

import (
	"context"
	"io"
	"net/http"

	obsfacade "github.com/CaixyPromise/seven-agent/force/internal/app/observability/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/httpx"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/adaptor"
)

type Service interface {
	GetOverview(ctx context.Context, platformKey, rangeKey string) (*obsfacade.OverviewVO, error)
	PageLogs(ctx context.Context, request adminfacade.RuntimeLogQueryDTO) (*adminfacade.PageResult[adminfacade.RuntimeLogLineDTO], error)
	StreamLogs(ctx context.Context, request adminfacade.RuntimeLogStreamRequestDTO, userID int64) (io.ReadCloser, error)
}

type Handler struct {
	service Service
}

func NewHandler(service Service) *Handler {
	return &Handler{service: service}
}

func (c *Handler) Overview(ctx context.Context, reqCtx *app.RequestContext) {
	platform := string(reqCtx.QueryArgs().Peek("platform"))
	rangeKey := string(reqCtx.QueryArgs().Peek("range"))
	result, err := c.service.GetOverview(ctx, platform, rangeKey)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, result)
}

func (c *Handler) LogPage(ctx context.Context, reqCtx *app.RequestContext) {
	var request adminfacade.RuntimeLogQueryDTO
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	page, err := c.service.PageLogs(ctx, request)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	response.Success(reqCtx, page)
}

func (c *Handler) LogStream(ctx context.Context, reqCtx *app.RequestContext) {
	currentUserID, ok := securitycontext.CurrentUserID(reqCtx)
	if !ok {
		response.Error(reqCtx, apperrors.Unauthorized("未登录"))
		return
	}
	var request adminfacade.RuntimeLogStreamRequestDTO
	if err := httpx.Bind(reqCtx, &request); err != nil {
		response.Error(reqCtx, err)
		return
	}
	stream, err := c.service.StreamLogs(ctx, request, currentUserID)
	if err != nil {
		response.Error(reqCtx, err)
		return
	}
	adaptor.HertzHandler(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer stream.Close()
		flusher, ok := writer.(http.Flusher)
		if !ok {
			http.Error(writer, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		writer.Header().Set("Cache-Control", "no-cache")
		writer.Header().Set("Connection", "keep-alive")
		writer.WriteHeader(http.StatusOK)
		flusher.Flush()

		buffer := make([]byte, 4096)
		for {
			n, readErr := stream.Read(buffer)
			if n > 0 {
				if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
					return
				}
				flusher.Flush()
			}
			if readErr != nil {
				return
			}
		}
	}))(ctx, reqCtx)
}
