package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// hubControlPostgresIdentifiers is the reviewed static identifier set used by
// the federation persistence adapter. Lower snake_case physical table names
// are deliberately omitted because PostgreSQL accepts them unquoted.
var hubControlPostgresIdentifiers = []string{
	"capabilitiesJson", "connectionRequestHash", "connectionStatus", "connectionVersion",
	"createdAt", "discoveryType", "hubIssuer", "isDeleted", "issuerLockedAt", "lastConnectionError",
	"lastConnectionTraceId", "lastHealthyAt", "managementBaseUrl", "managementBearerCiphertext",
	"managementBearerEdek", "managementBearerWrapKeyRef", "nodeCode", "nodeName", "oidcClientId",
	"oidcClientSecretCiphertext", "oidcClientSecretEdek", "oidcClientSecretWrapKeyRef", "requestHash",
	"serviceName", "targetRevision", "terminalState", "updatedAt",
}

var hubControlPostgresRenderer = store.MustNewPostgresRenderer(hubControlPostgresIdentifiers)
