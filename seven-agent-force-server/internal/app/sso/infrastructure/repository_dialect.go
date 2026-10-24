package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// ssoPostgresIdentifiers is the reviewed identifier set used by repository SQL.
// Runtime values remain bound parameters and must never be added here.
var ssoPostgresIdentifiers = []string{
	"accessTokenTtlSec", "activeRedirectCount", "activeSecretCount", "amrJson",
	"clientAuthMethod", "clientId", "clientName", "clientType", "codeChallenge",
	"codeChallengeMethod", "consumedAt", "createTime", "creatorId", "currentTokenHash",
	"detailJson", "deviceId", "eventType", "expiresAt", "externalIdentityId",
	"externalProviderCode", "familyId", "grantedAt", "grantTypesJson", "isDeleted",
	"lastAccessAt", "loginAt", "loginIp", "loginMethod", "metadataJson", "nonce",
	"platformCode", "postLogoutRedirectUri", "previousTokenHash", "reasonCode", "redirectUri",
	"refreshTokenTtlSec", "requireConsent", "requirePkce", "reuseDetected", "revokedAt",
	"rotatedAt", "scopesJson", "secretHash", "secretHint", "sessionId", "tenantId",
	"traceId", "trustedFirstParty", "updateTime", "updaterId", "userAgent", "userId",
}

var ssoPostgresRenderer = store.MustNewPostgresRenderer(ssoPostgresIdentifiers)
