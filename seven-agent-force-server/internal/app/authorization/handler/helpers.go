package handler

import (
	"strconv"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/cloudwego/hertz/pkg/app"
)

func parsePathInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	if reqCtx == nil {
		return 0, apperrors.Params("路径参数错误")
	}
	value := string(reqCtx.Param(key))
	return parseStringInt64(value)
}

func parseQueryInt64(reqCtx *app.RequestContext, key string) (int64, error) {
	if reqCtx == nil {
		return 0, apperrors.Params("查询参数错误")
	}
	return parseStringInt64(string(reqCtx.Query(key)))
}

func parseQueryTime(reqCtx *app.RequestContext, key string) (*time.Time, error) {
	if reqCtx == nil {
		return nil, apperrors.Params("查询参数错误")
	}
	value := strings.TrimSpace(string(reqCtx.Query(key)))
	if value == "" {
		return nil, apperrors.Params("查询参数错误")
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			utc := parsed.UTC()
			return &utc, nil
		}
	}
	return nil, apperrors.Params("时间参数格式错误")
}

func parseStringInt64(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, apperrors.Params("路径参数错误")
	}
	return parsed, nil
}

func contextParamError(message string) error {
	return apperrors.Params(message)
}
