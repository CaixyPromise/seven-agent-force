package infrastructure

import "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"

// filePostgresIdentifiers is a source-controlled schema allowlist. It contains
// only file-domain columns referenced by repository SQL; request values and
// request-selected fields must never be added here.
var filePostgresIdentifiers = []string{
	"strategyName", "providerType", "isDefault", "isEnabled", "runState",
	"configCiphertext", "configEdek", "wrapKeyRef", "healthCheckUrl",
	"healthStatus", "lastHealthCheck", "failureCount", "totalRequests",
	"failureRateThreshold", "windowStartTime", "createTime", "updateTime",
	"isDeleted",

	"fileInnerName", "fileSize", "fileSha256", "fileCrc32c", "hashAlgorithm",
	"contentType", "fileMetadata", "thumbnailData", "storageStrategyId",
	"storagePath", "scanStatus", "integrityStatus", "integrityCheckedAt",
	"deletedTime",

	"fileId", "userId", "scopeId", "displayName", "bizType", "bizId",
	"visitUrl", "accessLevel", "visitStrategy", "accessScope",

	"credentialId", "credentialVersion", "fileName", "objectKeyStaging",
	"objectKeyClean", "uploadMode", "multipartUploadId", "partSize",
	"totalParts", "expectedSize", "expectedSha256", "expectedCrc32c",
	"actualSize", "serverCrc32c", "failureCategory", "failureReason",
	"bindingToken", "bindingChannel", "expireAt", "protectedUntil",
	"credentialExpireAt", "revokedAt", "userIp",

	"uploadId", "uploadTaskId", "chunkSize", "totalChunks", "uploadedChunks",
	"chunkSha256Map", "tempStoragePath", "cloudUploadId", "partETagsMap",
	"expireTime",

	"taskType", "taskParams", "pipelineId", "nodeId", "idempotencyKey",
	"dedupKey", "replayToken", "dependsOn", "retryCount", "maxRetry",
	"errorMsg", "resultData", "mqMessageId", "nextRetryTime", "startTime",
	"finishTime", "taskId", "startedAt", "finishedAt",

	"attemptCount", "nextRetryTime", "lastError",
}

var filePostgresRenderer = store.MustNewPostgresRenderer(
	filePostgresIdentifiers,
	"isDefault",
	"isEnabled",
	"isDeleted",
)

func prepareRepositoryQuery(query string, postgres bool) string {
	if !postgres {
		return query
	}
	return filePostgresRenderer.RenderPostgres(query)
}
