package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// credentialPostgresIdentifiers is the reviewed static set of camelCase
// identifiers used by credential repository SQL. Runtime values remain bound
// parameters and must never be added to this allowlist.
var credentialPostgresIdentifiers = []string{
	"createTime", "creatorId", "credentialKey", "credentialPayloadJson",
	"credentialType", "invalidatedAt", "isDeleted", "lastUsedAt",
	"metadataJson", "mustChangePassword", "passwordChangedAt", "secretCiphertext",
	"secretHash", "updateTime", "updaterId", "userId", "verifiedAt",
}

var credentialPostgresRenderer = store.MustNewPostgresRenderer(
	credentialPostgresIdentifiers,
	"isDeleted",
)

func (r *UserCredentialRepository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = credentialPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}
