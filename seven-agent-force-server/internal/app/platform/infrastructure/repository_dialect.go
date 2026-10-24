package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// platformPostgresIdentifiers is the reviewed, static set of quoted camelCase
// identifiers used by platform repository SQL. Runtime values remain parameters
// and must never be added to this allowlist. Lower snake_case physical tables
// are intentionally omitted because PostgreSQL accepts them unquoted.
var platformPostgresIdentifiers = []string{
	"platformCode", "platformName", "platformType", "defaultRedirectUrl",
	"allowAutoRegister", "allowFormRegister", "isDefault", "defaultDeptId",
	"brandJson", "settingsJson", "creatorId", "updaterId", "createTime",
	"updateTime", "isDeleted",

	"clientId", "matchType", "matchValue", "metadataJson", "methodType",
	"providerCode", "displayName", "sortOrder", "displayEnabled",
	"loginEnabled", "roleId", "autoAssignEnabled", "systemKey", "menuId",
	"permissionId",
}

var platformPostgresRenderer = store.MustNewPostgresRenderer(platformPostgresIdentifiers)
