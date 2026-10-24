package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// externalLoginPostgresIdentifiers is the reviewed static identifier set used
// by this repository. Lower snake_case physical table names are deliberately
// omitted because PostgreSQL accepts them unquoted.
var externalLoginPostgresIdentifiers = []string{
	"accessExpiresAt", "accountAutoCreateEnabled", "authorizationEndpoint", "avatarUrl",
	"bindEnabled", "bindUserId", "capabilityCode", "clientId", "clientSecretCiphertext",
	"clientSecretEdek", "clientSecretWrapKeyRef", "codeVerifierCiphertext", "codeVerifierEdek",
	"codeVerifierWrapKeyRef", "connectionVersion", "consumedAt", "createTime", "createdAt", "creatorId",
	"displayEnabled", "displayName", "emailAutoBindEnabled", "emailVerified", "expiresAt",
	"externalEmail", "externalIdentityDigest", "externalIssuer", "externalLogin", "externalSubject",
	"firstLinkedAt", "icon", "identityId", "isDeleted", "issuer", "jwksUri", "lastLoginAt",
	"lastRefreshAt", "lastVerifiedAt", "loginEnabled", "loginIp", "loginTransactionId",
	"metadataJson", "methodKey", "nonceHash", "platformCode", "profileJson", "protocolType",
	"providerCode", "providerConfigDigest", "providerName", "provisioningAuthorityId", "redirectAfterLogin", "requestHash",
	"redirectUri", "refreshExpiresAt", "requiredScopesJson", "revokedAt", "scopeHash", "scopeJson",
	"scopesJson", "sortOrder", "stateHash", "stateId", "status", "tokenEndpoint", "tokenPurpose",
	"tokenSetCiphertext", "tokenSetEdek", "tokenSetWrapKeyRef", "traceId", "updateTime", "updaterId",
	"userAgent", "userId", "userinfoEndpoint", "version",
}

var externalLoginPostgresRenderer = store.MustNewPostgresRenderer(externalLoginPostgresIdentifiers)
