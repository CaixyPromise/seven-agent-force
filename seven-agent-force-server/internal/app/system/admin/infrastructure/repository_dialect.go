package infrastructure

import (
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
)

type adminDialectExecutor struct {
	store.SQLX
	postgres bool
}

func (e adminDialectExecutor) Rebind(query string) string {
	if e.postgres {
		query = adminPostgresRenderer.RenderPostgres(query)
	}
	return e.SQLX.Rebind(query)
}

// All identifiers are static repository columns. Keeping this list explicit
// avoids treating runtime input as SQL while supporting the quoted camelCase
// columns used by the Framework PostgreSQL baseline.
var adminPostgresRenderer = store.MustNewPostgresRenderer([]string{
	"userId", "userName", "nickName", "operationType", "operationDesc",
	"methodName", "requestMethod", "requestUrl", "traceId", "requestParams",
	"responseResult", "requestIp", "requestLocation", "userAgent", "operationTime",
	"executionTime", "errorMsg", "createTime", "updateTime", "isDeleted",
}, "isDeleted")
