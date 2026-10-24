package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// authorizationPostgresIdentifiers is the reviewed, static set of camelCase
// identifiers used by authorization repository SQL. Runtime values remain
// bound parameters and must never be added to this allowlist.
var authorizationPostgresIdentifiers = []string{
	"bootstrapKey", "createTime", "creatorId", "dataScope", "deptId",
	"expireTime", "featureCode", "grantRevision", "grantedBy",
	"idempotencyKey", "impactedUserCount", "initializedAt", "isCache",
	"isDeleted", "isFrame", "isPrimary", "menuId", "operatorId", "orgId",
	"nickName", "parentId", "permissionId", "postId", "requestHash", "resourceType",
	"resultRevision", "roleId", "rootRoleCode", "rootRoleId", "sortOrder",
	"systemKey", "updateTime", "updaterId", "userAccount", "userGender", "userId",
}

var authorizationPostgresRenderer = store.MustNewPostgresRenderer(
	authorizationPostgresIdentifiers,
	"isDeleted",
)

func (r *Repository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = authorizationPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}

func (r *Repository) temporaryPermissionDeadlineSQL() string {
	if r.postgres {
		return "NOW() + INTERVAL '24 hours'"
	}
	return "DATE_ADD(NOW(), INTERVAL 24 HOUR)"
}

func (r *Repository) boolValue(value bool) any {
	if r.postgres {
		return value
	}
	if value {
		return 1
	}
	return 0
}
