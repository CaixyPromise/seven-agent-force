package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// userPostgresIdentifiers is the reviewed set of camelCase columns used by
// system-user repository SQL. It is static source code, never request input.
var userPostgresIdentifiers = []string{
	"userAccount", "nickName", "creatorId", "updaterId", "userPhone",
	"userEmail", "userGender", "userAvatar", "userProfile", "unsealTime",
	"deletionTime", "createTime", "updateTime", "isDeleted", "statusVersion",
	"statusCommandHash",

	"userId", "roleId", "orgId", "deptId", "postId", "isPrimary",
	"parentId", "sortOrder", "leaderUserId",
}

var userPostgresRenderer = store.MustNewPostgresRenderer(
	userPostgresIdentifiers,
	"isDeleted",
	"isPrimary",
)
