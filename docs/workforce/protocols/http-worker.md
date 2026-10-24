# Worker Machine HTTP 方法参考

来源：随附独立源码快照（结构经检查）。共 **86** 个方法。只供 Worker／Control Plane 使用 Worker Bearer credential；不接受 Application API Key 代替 Worker 身份。详见 [Worker 与队列指南](worker-amqp.md)。

此页逐项列出方法、URL、路径／查询／Header／表单参数、请求体字段、响应类型和实现位置。嵌套对象点进类型索引；`omitempty` 仅是编码规则，业务必填与状态前置条件由对应应用／域校验决定。各接口并不统一采用 PATCH 或自动幂等。

| 方法 | 路径 | Handler |
| --- | --- | --- |
| POST | [`/api/internal/workforce/v1/artifact-upload-attempts/:uploadId/claims`](#route-1) | `claimUpload` |
| POST | [`/api/internal/workforce/v1/artifact-upload-attempts/:uploadId/failures`](#route-2) | `failUpload` |
| POST | [`/api/internal/workforce/v1/artifact-upload-attempts/:uploadId/verifications`](#route-3) | `verifyUpload` |
| POST | [`/api/internal/workforce/v1/assignments/:assignmentId/claim`](#route-4) | `claim` |
| POST | [`/api/internal/workforce/v1/assignments/:assignmentId/release`](#route-5) | `releaseAssignment` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/artifacts/batch`](#route-6) | `artifacts` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/checks/batch`](#route-7) | `checks` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/completion-check-results`](#route-8) | `recordCompletionCheck` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/completion-check-results/read`](#route-9) | `readCompletionCheck` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/completion-check-revisions/apply`](#route-10) | `applyCompletionRevision` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/completion-proposals`](#route-11) | `completion` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/containers`](#route-12) | `containerCreated` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/continuation-checkpoint-keys`](#route-13) | `continuationCheckpointKey` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/continuation-checkpoints`](#route-14) | `continuationCheckpoint` |
| GET | [`/api/internal/workforce/v1/attempts/:attemptId/control`](#route-15) | `control` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/events/batch`](#route-16) | `facts` |
| GET | [`/api/internal/workforce/v1/attempts/:attemptId/evidence`](#route-17) | `evidence` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/claim`](#route-18) | `claimColdExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/readback`](#route-19) | `readbackColdExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/renew`](#route-20) | `renewColdExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result`](#route-21) | `resultColdExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result-ack`](#route-22) | `ackColdExecutionOperationResult` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result-receipt`](#route-23) | `readColdExecutionOperationResultReceipt` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/start`](#route-24) | `startColdExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/renew`](#route-25) | `renewExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result`](#route-26) | `resultExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-ack`](#route-27) | `ackExecutionOperationResult` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-receipt`](#route-28) | `readExecutionOperationResultReceipt` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/start`](#route-29) | `startExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/claim-next`](#route-30) | `claimNextExecutionOperation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/pending-suspend`](#route-31) | `pendingSuspendExecutionOperations` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/execution-operations/results/pull`](#route-32) | `pullExecutionOperationResults` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/harness-started`](#route-33) | `harnessStarted` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/interactions/ack`](#route-34) | `ackInteraction` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/interactions/pending`](#route-35) | `pendingInteractions` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/interactions/publish`](#route-36) | `publishInteraction` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/interactions/pull`](#route-37) | `pullInteractions` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/interactions/suspend/publish`](#route-38) | `publishInteractionSuspend` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/interactions/suspend/reserve`](#route-39) | `reserveInteractionSuspend` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/journal-facts/batch`](#route-40) | `journalFacts` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/launch-permits`](#route-41) | `launch` |
| PUT | [`/api/internal/workforce/v1/attempts/:attemptId/lease`](#route-42) | `renewLease` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/outcome`](#route-43) | `outcome` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/processor-materializations`](#route-44) | `processorMaterialization` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/provider-proxy-ready`](#route-45) | `providerProxyReady` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/reattach-acks`](#route-46) | `reattachAck` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/reattach-permits`](#route-47) | `reattachPermit` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/result-evaluations`](#route-48) | `resultEvaluation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/result-evaluations/read`](#route-49) | `readEvaluation` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/run-permits`](#route-50) | `runPermit` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/runtime-losses`](#route-51) | `runtimeLoss` |
| POST | [`/api/internal/workforce/v1/attempts/:attemptId/supervisor-ready`](#route-52) | `supervisorReady` |
| POST | [`/api/internal/workforce/v1/continuation-restores/:restoreRunId/key`](#route-53) | `continuationRestoreKey` |
| POST | [`/api/internal/workforce/v1/continuation-restores/:restoreRunId/results`](#route-54) | `continuationRestoreResult` |
| POST | [`/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/claims`](#route-55) | `claimDelivery` |
| POST | [`/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/credential-materializations`](#route-56) | `deliveryCredential` |
| POST | [`/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/failures`](#route-57) | `failDelivery` |
| POST | [`/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/observations`](#route-58) | `prepareDelivery` |
| POST | [`/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/reconciliations`](#route-59) | `reconcileDelivery` |
| POST | [`/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/results`](#route-60) | `resultDelivery` |
| POST | [`/api/internal/workforce/v1/finalization-runs/:runId/claims`](#route-61) | `claimFinalization` |
| POST | [`/api/internal/workforce/v1/finalization-runs/:runId/failures`](#route-62) | `failFinalization` |
| POST | [`/api/internal/workforce/v1/finalization-runs/:runId/freeze`](#route-63) | `freezeFinalization` |
| POST | [`/api/internal/workforce/v1/result-materialization-runs/:runId/claims`](#route-64) | `claimMaterialization` |
| POST | [`/api/internal/workforce/v1/result-materialization-runs/:runId/failures`](#route-65) | `failMaterialization` |
| POST | [`/api/internal/workforce/v1/result-materialization-runs/:runId/freeze`](#route-66) | `freezeMaterialization` |
| POST | [`/api/internal/workforce/v1/result-processor-fixture-runs/:runId/claims`](#route-67) | `fixtureClaim` |
| POST | [`/api/internal/workforce/v1/result-processor-fixture-runs/:runId/results`](#route-68) | `fixtureResult` |
| POST | [`/api/internal/workforce/v1/review-runs/:runId/claims`](#route-69) | `claimReviewRun` |
| POST | [`/api/internal/workforce/v1/review-runs/:runId/containers`](#route-70) | `reviewContainerCreated` |
| POST | [`/api/internal/workforce/v1/review-runs/:runId/failures`](#route-71) | `failReviewRun` |
| POST | [`/api/internal/workforce/v1/review-runs/:runId/launch-permits`](#route-72) | `reviewLaunchPermit` |
| PUT | [`/api/internal/workforce/v1/review-runs/:runId/lease`](#route-73) | `renewReviewLease` |
| POST | [`/api/internal/workforce/v1/review-runs/:runId/recover`](#route-74) | `recoverReviewRun` |
| POST | [`/api/internal/workforce/v1/review-runs/:runId/reports`](#route-75) | `submitReviewReport` |
| POST | [`/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/claims`](#route-76) | `claimSourceIngestion` |
| POST | [`/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/credential-materializations`](#route-77) | `sourceIngestionCredential` |
| POST | [`/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/failures`](#route-78) | `failSourceIngestion` |
| POST | [`/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/observations`](#route-79) | `observeSourceIngestion` |
| POST | [`/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/results`](#route-80) | `resultSourceIngestion` |
| POST | [`/api/internal/workforce/v1/tasks/:taskId/workspace-gc-eligibility`](#route-81) | `workspaceGCEligibility` |
| POST | [`/api/internal/workforce/v1/workers/:workerId/deregister`](#route-82) | `deregister` |
| POST | [`/api/internal/workforce/v1/workers/:workerId/heartbeat`](#route-83) | `heartbeat` |
| PUT | [`/api/internal/workforce/v1/workers/:workerId/image-inventory`](#route-84) | `inventory` |
| GET | [`/api/internal/workforce/v1/workers/:workerId/reconciliation`](#route-85) | `reconciliation` |
| POST | [`/api/internal/workforce/v1/workers/:workerId/register`](#route-86) | `register` |

<a id="route-1"></a>

## POST `/api/internal/workforce/v1/artifact-upload-attempts/:uploadId/claims`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:646](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L646)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:159](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L159)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `uploadId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.postprocessClaimRequest`](wire-types.md#t-b6d513fdac3291f3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/artifact/domain.UploadClaim`](wire-types.md#t-7f30109101ed9a1b)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `POSTPROCESS_WORK_NOT_FOUND`, `POSTPROCESS_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-2"></a>

## POST `/api/internal/workforce/v1/artifact-upload-attempts/:uploadId/failures`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:676](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L676)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:161](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L161)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `uploadId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.postprocessFailureRequest`](wire-types.md#t-1b152dc2c3cf52a6)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `POSTPROCESS_WORK_NOT_FOUND`, `POSTPROCESS_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-3"></a>

## POST `/api/internal/workforce/v1/artifact-upload-attempts/:uploadId/verifications`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:664](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L664)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:160](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L160)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `uploadId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.uploadVerificationRequest`](wire-types.md#t-bd4b422284a3bcdb)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `verified` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `POSTPROCESS_WORK_NOT_FOUND`, `POSTPROCESS_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-4"></a>

## POST `/api/internal/workforce/v1/assignments/:assignmentId/claim`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:710](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L710)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:113](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L113)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `assignmentId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.claimRequest`](wire-types.md#t-d54e5c07e8750b63)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `executionRequirementDigest` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `contract` | integer；Go `internal/app/workforce/taskdispatch/domain.ExecutionContract` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-5"></a>

## POST `/api/internal/workforce/v1/assignments/:assignmentId/release`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:693](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L693)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:122](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L122)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `assignmentId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.releaseAssignmentRequest`](wire-types.md#t-062cbf0baaa0d899)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reasonCode` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `released` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-6"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/artifacts/batch`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1310](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1310)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:154](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L154)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.artifactsRequest`](wire-types.md#t-8d5e3c6aea609842)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `artifacts` | array<integer> | array<integer> | [`internal/app/workforce/taskdispatch/controller.artifactInput`](wire-types.md#t-0547425de299ad84)；Go: `[]internal/app/workforce/taskdispatch/controller.artifactInput` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `ARTIFACT_IDENTITY_INVALID`, `ARTIFACT_PERSIST_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-7"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/checks/batch`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1116](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1116)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:137](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L137)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.checksRequest`](wire-types.md#t-db0aa9590a571a14)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `checks` | array<integer> | array<integer> | [`internal/app/workforce/taskdispatch/controller.checkInput`](wire-types.md#t-6c1fa5628a43d246)；Go: `[]internal/app/workforce/taskdispatch/controller.checkInput` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `CHECK_BATCH_INVALID`, `CHECK_OUTPUT_TOO_LARGE`, `CHECK_PERSIST_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-8"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/completion-check-results`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/completion_check.go:17](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/completion_check.go#L17)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:142](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L142)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.774ca3c5731e`](wire-types.md#t-4e9f84b2e76718ce)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `evidence` | integer | integer | [`internal/app/workforce/completioncheck/facade.Evidence`](wire-types.md#t-c31697233475837a)；Go: `internal/app/workforce/completioncheck/facade.Evidence` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/facade.Evidence`](wire-types.md#t-c31697233475837a)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `COMPLETION_CHECK_UNAVAILABLE`, `COMPLETION_CHECK_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-9"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/completion-check-results/read`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/completion_check.go:37](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/completion_check.go#L37)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:143](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L143)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.a259c218dba4`](wire-types.md#t-17aac48f12b81238)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `phase` | string | string | Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `digest` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/facade.Evidence`](wire-types.md#t-c31697233475837a) 或 null。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `COMPLETION_CHECK_UNAVAILABLE`, `COMPLETION_CHECK_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-10"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/completion-check-revisions/apply`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/completion_revision.go:15](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/completion_revision.go#L15)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:109](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L109)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.5436381527b9`](wire-types.md#t-a4d674c6c5585443)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/facade.ApplyPendingResult`](wire-types.md#t-0847d4e3d1066114)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `COMPLETION_CHECK_REVISION_UNAVAILABLE`, `COMPLETION_CHECK_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-11"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/completion-proposals`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1085](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1085)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:136](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L136)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.completionRequest`](wire-types.md#t-31d74f94537ad213)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `completion` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `validationStatus` | string；Go `string` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `COMPLETION_INVALID`, `COMPLETION_PERSIST_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-12"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/containers`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:914](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L914)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:124](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L124)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.containerCreatedRequest`](wire-types.md#t-845b6de22a9314ca)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `containerId` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-13"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/continuation-checkpoint-keys`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:518](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L518)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:145](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L145)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.continuationKeyRequest`](wire-types.md#t-810dc39cb5ca3b4d)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `checkpointIdentity` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/continuation/application.CheckpointKeyMaterialization`](wire-types.md#t-f9cd7c61259b9fda)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-14"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/continuation-checkpoints`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1199](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1199)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:144](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L144)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.continuationCheckpointRequest`](wire-types.md#t-4da93b7acdfbd0f7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `checkpoint` | integer | integer | [`internal/app/workforce/resultprocessor/facade.ContinuationCheckpointCandidate`](wire-types.md#t-8b21b5d551e70b86)；Go: `internal/app/workforce/resultprocessor/facade.ContinuationCheckpointCandidate` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/facade.ContinuationCheckpointView`](wire-types.md#t-12df377b44feb960)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `RESULT_PROCESSOR_UNAVAILABLE`, `RESULT_PROCESSOR_NOT_FOUND`, `RESULT_PROCESSOR_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-15"></a>

## GET `/api/internal/workforce/v1/attempts/:attemptId/control`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1279](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1279)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:133](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L133)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `X-Seven-Generation` | string |
| header | `X-Seven-Owner-Epoch` | string |
| header | `X-Seven-Worker-ID` | string |
| header | `X-Seven-Request-ID` | string |
| header | `X-Seven-Worker-Instance` | string |
| header | `X-Seven-Host-Boot-ID` | string |
| header | `X-Seven-Attempt-Lease` | string |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

`int64`

类型：`int64`。


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `cancelling` | boolean；Go `bool` |
| `serverTime` | string；Go `string` |

入口／同包辅助方法中可定位的 reasonCode：`INVALID_MACHINE_ENVELOPE`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `CONTROL_READ_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-16"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/events/batch`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1051](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1051)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:134](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L134)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.factsRequest`](wire-types.md#t-35b7bf49020c3244)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `facts` | array<integer> | array<integer> | [`internal/app/workforce/taskdispatch/domain.Fact`](wire-types.md#t-a555a5c104bf8ed2)；Go: `[]internal/app/workforce/taskdispatch/domain.Fact` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `FACT_BATCH_INVALID`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-17"></a>

## GET `/api/internal/workforce/v1/attempts/:attemptId/evidence`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1261](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1261)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:153](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L153)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `X-Seven-Generation` | string |
| header | `X-Seven-Owner-Epoch` | string |
| header | `X-Seven-Worker-ID` | string |
| header | `X-Seven-Request-ID` | string |
| header | `X-Seven-Worker-Instance` | string |
| header | `X-Seven-Host-Boot-ID` | string |
| header | `X-Seven-Attempt-Lease` | string |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

`int64`

类型：`int64`。


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `checks` | array<integer>；Go `[]internal/app/workforce/task/infrastructure.CheckEvidence` |
| `events` | array<integer>；Go `[]internal/app/workforce/task/domain.Event` |

入口／同包辅助方法中可定位的 reasonCode：`INVALID_MACHINE_ENVELOPE`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `EVIDENCE_READ_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-18"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/claim`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:46](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L46)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:24](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L24)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.interactionEnvelope`](wire-types.md#t-9a19597dbb49b5bf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Claim`](wire-types.md#t-3bc3f33391ceb263)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-19"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/readback`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:147](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L147)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:29](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L29)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.interactionEnvelope`](wire-types.md#t-9a19597dbb49b5bf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.ToolResult`](wire-types.md#t-9097658de2992bc0)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-20"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/renew`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:84](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L84)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:26](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L26)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.9dc7092d6c55`](wire-types.md#t-f7ac3943f3cc2fef)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `renew` | integer | integer | [`internal/app/workforce/executionaccess/facade.RenewRequest`](wire-types.md#t-173b66c0501009d7)；Go: `internal/app/workforce/executionaccess/facade.RenewRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-21"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:105](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L105)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:27](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L27)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.ecd6ca204369`](wire-types.md#t-a2bca8d4ce980fed)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `result` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultRequest`](wire-types.md#t-57e2a75e70cf2ed5)；Go: `internal/app/workforce/executionaccess/facade.ResultRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-22"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result-ack`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:164](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L164)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:30](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L30)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.1385a0115560`](wire-types.md#t-3ef2787367843da1)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ack` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultAckRequest`](wire-types.md#t-e21dea01113fc815)；Go: `internal/app/workforce/executionaccess/facade.ResultAckRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-23"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/result-receipt`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:126](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L126)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:28](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L28)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.eb6a51ff5193`](wire-types.md#t-5c7d0e08f094b695)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `receipt` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultReceiptRequest`](wire-types.md#t-45900bc030591739)；Go: `internal/app/workforce/executionaccess/facade.ResultReceiptRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-24"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/cold/start`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:63](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L63)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:25](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L25)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.4e1df174b955`](wire-types.md#t-68f9249de067355a)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `start` | integer | integer | [`internal/app/workforce/executionaccess/facade.StartRequest`](wire-types.md#t-e87acbc7a9d6bc90)；Go: `internal/app/workforce/executionaccess/facade.StartRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-25"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/renew`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:277](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L277)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:19](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L19)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.9dc7092d6c55`](wire-types.md#t-f7ac3943f3cc2fef)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `renew` | integer | integer | [`internal/app/workforce/executionaccess/facade.RenewRequest`](wire-types.md#t-173b66c0501009d7)；Go: `internal/app/workforce/executionaccess/facade.RenewRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-26"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:298](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L298)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:20](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L20)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.ecd6ca204369`](wire-types.md#t-a2bca8d4ce980fed)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `result` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultRequest`](wire-types.md#t-57e2a75e70cf2ed5)；Go: `internal/app/workforce/executionaccess/facade.ResultRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-27"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-ack`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:360](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L360)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:23](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L23)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.1385a0115560`](wire-types.md#t-3ef2787367843da1)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ack` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultAckRequest`](wire-types.md#t-e21dea01113fc815)；Go: `internal/app/workforce/executionaccess/facade.ResultAckRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-28"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/result-receipt`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:319](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L319)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:21](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L21)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.eb6a51ff5193`](wire-types.md#t-5c7d0e08f094b695)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `receipt` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultReceiptRequest`](wire-types.md#t-45900bc030591739)；Go: `internal/app/workforce/executionaccess/facade.ResultReceiptRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-29"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/:operationId/start`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:256](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L256)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:18](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L18)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |
| path | `operationId` | string |

### 请求体

[`inline.4e1df174b955`](wire-types.md#t-68f9249de067355a)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `start` | integer | integer | [`internal/app/workforce/executionaccess/facade.StartRequest`](wire-types.md#t-e87acbc7a9d6bc90)；Go: `internal/app/workforce/executionaccess/facade.StartRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-30"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/claim-next`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:239](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L239)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:16](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L16)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.interactionEnvelope`](wire-types.md#t-9a19597dbb49b5bf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Claim`](wire-types.md#t-3bc3f33391ceb263)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-31"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/pending-suspend`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:185](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L185)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:17](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L17)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.interactionEnvelope`](wire-types.md#t-9a19597dbb49b5bf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/executionaccess/facade.PendingSuspendOperation`](wire-types.md#t-c972565d54b97cae)>。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-32"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/execution-operations/results/pull`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:340](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L340)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go:22](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/execution_access.go#L22)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.c7c6d9b4185d`](wire-types.md#t-3bd3d2aa151c7a73)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `limit` | integer | integer | Go: `int` |


### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/executionaccess/facade.ToolResult`](wire-types.md#t-9097658de2992bc0)>。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-33"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/harness-started`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:963](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L963)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:128](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L128)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.harnessStartedRequest`](wire-types.md#t-d9a37d495d1bce8b)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `started` | integer | integer | [`internal/app/workforce/taskdispatch/domain.HarnessStarted`](wire-types.md#t-76278493a6e091ef)；Go: `internal/app/workforce/taskdispatch/domain.HarnessStarted` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-34"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/interactions/ack`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:284](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L284)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:32](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L32)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.3df6d954d9ae`](wire-types.md#t-c51f7ae28dcdf0d7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ack` | integer | integer | [`internal/app/workforce/interaction/facade.AckRequest`](wire-types.md#t-f78ccd181810d5f2)；Go: `internal/app/workforce/interaction/facade.AckRequest` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/interaction/facade.Receipt`](wire-types.md#t-72ef9cc8b517430e)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `INTERACTION_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-35"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/interactions/pending`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_pending.go:9](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_pending.go#L9)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:29](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L29)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.interactionEnvelope`](wire-types.md#t-9a19597dbb49b5bf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/interaction/facade.PendingQuestion`](wire-types.md#t-580203605653fbea)>。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `INTERACTION_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-36"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/interactions/publish`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:86](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L86)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:30](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L30)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.520298938742`](wire-types.md#t-25a82a7db0c8134f)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `frame` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

[`inline.cd5ad4c08fb9`](wire-types.md#t-b16411908307d2b4)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `questions` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Question`](wire-types.md#t-5c2e63b06a84f495)；Go: `[]internal/app/workforce/interaction/domain.Question` |
| `isBlocking` | boolean | boolean | Go: `bool` |

[`inline.b424e42d4232`](wire-types.md#t-f166cb0eba8e94e7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |

[`inline.00f046ab8adb`](wire-types.md#t-530db2c02836da26)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `messageKey` | string | string | Go: `string` |
| `responseId` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |

[`internal/worker/agentprotocol.ToolCatalogPayload`](wire-types.md#t-d0c093d94cb7b7ee)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `toolNames` | array<string> | array<string> | Go: `[]string` |
| `catalogDigest` | string | string | Go: `string` |

[`internal/worker/agentprotocol.ToolRequestPayload`](wire-types.md#t-85fdfa303a772fac)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `toolName` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `nativeToolCallId` | string | string | Go: `string` |
| `roundKey` | string | string | Go: `string` |
| `arguments` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `argumentsDigest` | string | string | Go: `string` |

[`internal/worker/agentprotocol.NetworkCommandArguments`](wire-types.md#t-053887636e577d5a)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `purpose` | string | string | Go: `string` |
| `command` | string | string | Go: `string` |
| `cwd` | string | string | Go: `string` |
| `targets` | array<integer> | array<integer> | [`internal/worker/agentprotocol.ToolTarget`](wire-types.md#t-2d3f2704bc315e56)；Go: `[]internal/worker/agentprotocol.ToolTarget` |

[`internal/worker/agentprotocol.ToolReceiptPayload`](wire-types.md#t-01d1ae55d45642aa)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |

[`internal/worker/agentprotocol.ToolClosedPayload`](wire-types.md#t-7befa62321d6299b)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`inline.030988320bde`](wire-types.md#t-149143e12cdb87b6)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `INTERACTION_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-37"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/interactions/pull`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:272](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L272)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:31](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L31)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.interactionEnvelope`](wire-types.md#t-9a19597dbb49b5bf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/interaction/facade.Delivery`](wire-types.md#t-8e269ecf7e1da3cf)>。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `INTERACTION_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-38"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/interactions/suspend/publish`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go:79](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go#L79)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go:17](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go#L17)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.4e6fb214e99b`](wire-types.md#t-12e661996f64a8ce)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `bundle` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |
| `overlay` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |
| `session` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |
| `manifest` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]bool`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `saved` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `INTERACTION_UNAVAILABLE`, `INTERACTION_FENCED`, `INTERACTION_SUSPEND_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-39"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/interactions/suspend/reserve`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go:48](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go#L48)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go:16](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go#L16)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`inline.d1bde174e73e`](wire-types.md#t-5567296190fec4a8)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `sessionDigest` | string | string | Go: `string` |
| `currentRound` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `suspendControlKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/continuation/application.InteractionReservationResult`](wire-types.md#t-3dcee10cb63ef331)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `INTERACTION_UNAVAILABLE`, `INTERACTION_FENCED`, `INTERACTION_SUSPEND_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-40"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/journal-facts/batch`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1055](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1055)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:135](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L135)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.factsRequest`](wire-types.md#t-35b7bf49020c3244)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `facts` | array<integer> | array<integer> | [`internal/app/workforce/taskdispatch/domain.Fact`](wire-types.md#t-a555a5c104bf8ed2)；Go: `[]internal/app/workforce/taskdispatch/domain.Fact` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `FACT_BATCH_INVALID`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-41"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/launch-permits`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:893](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L893)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:123](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L123)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.leaseRequest`](wire-types.md#t-28897326f63cbcd9)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `activeElapsedMillis` | string(decimal) 或 null | string(decimal) 或 null | 零值可省略；JSON 引号编码；Go: `*int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `permit` | integer；Go `internal/app/workforce/taskdispatch/domain.LaunchPermit` |
| `replayed` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-42"></a>

## PUT `/api/internal/workforce/v1/attempts/:attemptId/lease`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:975](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L975)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:129](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L129)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.leaseRequest`](wire-types.md#t-28897326f63cbcd9)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `activeElapsedMillis` | string(decimal) 或 null | string(decimal) 或 null | 零值可省略；JSON 引号编码；Go: `*int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `expiresAt` | string；Go `string` |
| `receiptDigest` | string；Go `string` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-43"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/outcome`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1345](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1345)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:155](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L155)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.outcomeRequest`](wire-types.md#t-b4f3adddbc06c6db)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `outcome` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `message` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `summary` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `OUTCOME_INVALID`, `OUTCOME_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-44"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/processor-materializations`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1150](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1150)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:138](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L138)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.processorMaterializationRequest`](wire-types.md#t-4e33385067ea8aee)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `cycleNumber` | integer | integer | Go: `int` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/facade.Materialization`](wire-types.md#t-af6eb918fd2fdb5d)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `RESULT_PROCESSOR_UNAVAILABLE`, `RESULT_PROCESSOR_NOT_FOUND`, `RESULT_PROCESSOR_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-45"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/provider-proxy-ready`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:583](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L583)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:125](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L125)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.providerProxyReadyRequest`](wire-types.md#t-d7e2b508f5d14f2c)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `ready` | integer | integer | [`internal/app/workforce/taskdispatch/domain.ProviderProxyReady`](wire-types.md#t-1de3f534590336a9)；Go: `internal/app/workforce/taskdispatch/domain.ProviderProxyReady` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-46"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/reattach-acks`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1013](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1013)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:131](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L131)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reattachAckRequest`](wire-types.md#t-a27d1078ad478922)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `ack` | integer | integer | [`internal/app/workforce/taskdispatch/domain.ReattachAck`](wire-types.md#t-494b89344ead4de3)；Go: `internal/app/workforce/taskdispatch/domain.ReattachAck` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/taskdispatch/domain.ReattachResult`](wire-types.md#t-84cc8dde8d0708e0)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-47"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/reattach-permits`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:994](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L994)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:130](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L130)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reattachPermitRequest`](wire-types.md#t-081ea713c9382f3e)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `evidence` | integer | integer | [`internal/app/workforce/taskdispatch/domain.RuntimeEvidence`](wire-types.md#t-f633aecbece0f37e)；Go: `internal/app/workforce/taskdispatch/domain.RuntimeEvidence` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `permit` | integer；Go `internal/app/workforce/taskdispatch/domain.ReattachPermit` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-48"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/result-evaluations`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1173](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1173)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:139](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L139)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.resultEvaluationRequest`](wire-types.md#t-aa7d61165d0822fd)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `candidate` | integer | integer | [`internal/app/workforce/resultprocessor/facade.EvaluationCandidate`](wire-types.md#t-1cb3eba0f7bf13fb)；Go: `internal/app/workforce/resultprocessor/facade.EvaluationCandidate` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/facade.EvaluationDirective`](wire-types.md#t-005d2358dcdb1f9c)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `RESULT_PROCESSOR_UNAVAILABLE`, `RESULT_PROCESSOR_NOT_FOUND`, `RESULT_PROCESSOR_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-49"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/result-evaluations/read`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/evaluation_replay.go:17](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/evaluation_replay.go#L17)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:141](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L141)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.evaluationReplayRequest`](wire-types.md#t-06c94ac1879faa14)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `pending` | integer | integer | [`internal/app/workforce/resultprocessor/facade.EvaluationReplayIdentity`](wire-types.md#t-47ef88aef1917589)；Go: `internal/app/workforce/resultprocessor/facade.EvaluationReplayIdentity` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/facade.EvaluationReplay`](wire-types.md#t-80207b14faf14de1)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ATTEMPT_IDENTITY_CONFLICT`, `RESULT_PROCESSOR_UNAVAILABLE`, `RESULT_PROCESSOR_NOT_FOUND`, `RESULT_PROCESSOR_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-50"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/run-permits`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:944](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L944)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:127](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L127)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.leaseRequest`](wire-types.md#t-28897326f63cbcd9)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `activeElapsedMillis` | string(decimal) 或 null | string(decimal) 或 null | 零值可省略；JSON 引号编码；Go: `*int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `permit` | integer；Go `internal/app/workforce/taskdispatch/domain.RunPermit` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-51"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/runtime-losses`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1032](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1032)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:132](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L132)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.runtimeLossRequest`](wire-types.md#t-e051a0d57383510e)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reasonCode` | string | string | Go: `string` |
| `evidenceDigest` | string | string | 零值可省略；Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/taskdispatch/domain.RuntimeLossResult`](wire-types.md#t-88932fa7ff616165)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-52"></a>

## POST `/api/internal/workforce/v1/attempts/:attemptId/supervisor-ready`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:932](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L932)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:126](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L126)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `attemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.supervisorReadyRequest`](wire-types.md#t-ca09ca061ad0cab6)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `ready` | integer | integer | [`internal/app/workforce/taskdispatch/domain.SupervisorReady`](wire-types.md#t-6f962b67246eb397)；Go: `internal/app/workforce/taskdispatch/domain.SupervisorReady` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-53"></a>

## POST `/api/internal/workforce/v1/continuation-restores/:restoreRunId/key`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:537](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L537)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:146](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L146)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `restoreRunId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.continuationRestoreKeyRequest`](wire-types.md#t-c1dacba809dcd927)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/continuation/application.CheckpointKeyMaterialization`](wire-types.md#t-f9cd7c61259b9fda)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-54"></a>

## POST `/api/internal/workforce/v1/continuation-restores/:restoreRunId/results`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:561](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L561)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:147](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L147)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `restoreRunId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.continuationRestoreResultRequest`](wire-types.md#t-c0332ca759674163)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `restoreRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultDigest` | string | string | Go: `string` |
| `baseCommit` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `ASSIGNMENT_NOT_FOUND`, `ASSIGNMENT_NOT_CLAIMABLE`, `STALE_WORKER_INSTANCE`, `EXECUTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-55"></a>

## POST `/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/claims`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:311](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L311)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:162](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L162)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `deliveryAttemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.postprocessClaimRequest`](wire-types.md#t-b6d513fdac3291f3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.DeliveryClaim`](wire-types.md#t-926bdefce296b43c)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-56"></a>

## POST `/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/credential-materializations`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:329](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L329)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:163](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L163)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `deliveryAttemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.deliveryCredentialRequest`](wire-types.md#t-5e692640557e4821)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.DeliveryCredential`](wire-types.md#t-8554e566709ed2aa)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-57"></a>

## POST `/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/failures`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:404](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L404)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:167](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L167)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `deliveryAttemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.deliveryFailureRequest`](wire-types.md#t-29272c288d8dc514)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-58"></a>

## POST `/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/observations`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:348](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L348)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:164](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L164)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `deliveryAttemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.deliveryPrepareRequest`](wire-types.md#t-f67ba4457583ee07)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `deliveryCommit` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.PrepareResult`](wire-types.md#t-71652dd9dd9b2e1f)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-59"></a>

## POST `/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/reconciliations`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:386](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L386)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:166](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L166)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `deliveryAttemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.deliveryReconciliationRequest`](wire-types.md#t-5cc6d556aab025e5)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/delivery/application.DeliveryResult`](wire-types.md#t-1a585eac31214972)；Go: `internal/app/workforce/delivery/application.DeliveryResult` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-60"></a>

## POST `/api/internal/workforce/v1/delivery-attempts/:deliveryAttemptId/results`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:368](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L368)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:165](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L165)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `deliveryAttemptId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.deliveryResultRequest`](wire-types.md#t-4eab194eab49d1ad)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `pushAuthorization` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/delivery/application.DeliveryResult`](wire-types.md#t-1a585eac31214972)；Go: `internal/app/workforce/delivery/application.DeliveryResult` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-61"></a>

## POST `/api/internal/workforce/v1/finalization-runs/:runId/claims`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:597](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L597)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:156](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L156)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.postprocessClaimRequest`](wire-types.md#t-b6d513fdac3291f3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/artifact/domain.FinalizationClaim`](wire-types.md#t-2cea9f9f740879ba)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `POSTPROCESS_WORK_NOT_FOUND`, `POSTPROCESS_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-62"></a>

## POST `/api/internal/workforce/v1/finalization-runs/:runId/failures`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:634](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L634)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:158](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L158)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.postprocessFailureRequest`](wire-types.md#t-1b152dc2c3cf52a6)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `POSTPROCESS_WORK_NOT_FOUND`, `POSTPROCESS_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-63"></a>

## POST `/api/internal/workforce/v1/finalization-runs/:runId/freeze`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:616](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L616)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:157](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L157)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.freezeRequest`](wire-types.md#t-1c48e0532b4a52ca)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/artifact/domain.FreezeResult`](wire-types.md#t-3061dadeee2262bb)；Go: `internal/app/workforce/artifact/domain.FreezeResult` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `frozen` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `POSTPROCESS_WORK_NOT_FOUND`, `POSTPROCESS_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-64"></a>

## POST `/api/internal/workforce/v1/result-materialization-runs/:runId/claims`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:461](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L461)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:148](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L148)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.postprocessClaimRequest`](wire-types.md#t-b6d513fdac3291f3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/continuation/domain.MaterializationClaim`](wire-types.md#t-7b9980726c2bd212)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `CONTINUATION_RESOURCE_NOT_FOUND`, `CONTINUATION_PERSIST_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-65"></a>

## POST `/api/internal/workforce/v1/result-materialization-runs/:runId/failures`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:499](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L499)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:150](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L150)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.materializationFailureRequest`](wire-types.md#t-9b0fcbed42d61fd0)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `CONTINUATION_RESOURCE_NOT_FOUND`, `CONTINUATION_PERSIST_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-66"></a>

## POST `/api/internal/workforce/v1/result-materialization-runs/:runId/freeze`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:480](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L480)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:149](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L149)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.materializationFreezeRequest`](wire-types.md#t-31128fe5d65e34bf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/continuation/domain.MaterializationFreeze`](wire-types.md#t-d1654b86d786a887)；Go: `internal/app/workforce/continuation/domain.MaterializationFreeze` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `frozen` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `CONTINUATION_RESOURCE_NOT_FOUND`, `CONTINUATION_PERSIST_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-67"></a>

## POST `/api/internal/workforce/v1/result-processor-fixture-runs/:runId/claims`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1218](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1218)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:151](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L151)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.fixtureClaimRequest`](wire-types.md#t-4ec29bce16336dcf)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/facade.FixtureExecution`](wire-types.md#t-b487b9d57000cbe9)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `RESULT_PROCESSOR_UNAVAILABLE`, `RESULT_PROCESSOR_NOT_FOUND`, `RESULT_PROCESSOR_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-68"></a>

## POST `/api/internal/workforce/v1/result-processor-fixture-runs/:runId/results`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1240](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1240)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:152](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L152)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.fixtureResultRequest`](wire-types.md#t-f09e06a198710518)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `result` | integer | integer | [`internal/app/workforce/resultprocessor/facade.FixtureResult`](wire-types.md#t-6ecead615b31bf3d)；Go: `internal/app/workforce/resultprocessor/facade.FixtureResult` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.FixtureRun`](wire-types.md#t-a2fa5e5df2f0a955)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `RESULT_PROCESSOR_UNAVAILABLE`, `RESULT_PROCESSOR_NOT_FOUND`, `RESULT_PROCESSOR_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-69"></a>

## POST `/api/internal/workforce/v1/review-runs/:runId/claims`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:725](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L725)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:114](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L114)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reviewClaimRequest`](wire-types.md#t-6e96a09c4683d1a0)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `claim` | integer；Go `internal/app/workforce/reviewrun/facade.Claim` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `REVIEW_RUN_CONTROL_UNAVAILABLE`, `REVIEW_RUN_NOT_FOUND`, `REVIEW_REPORT_REJECTED`, `REVIEW_RUN_CONTROL_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-70"></a>

## POST `/api/internal/workforce/v1/review-runs/:runId/containers`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:818](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L818)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:118](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L118)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reviewContainerRequest`](wire-types.md#t-64c215f56cb17d2f)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `permitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `permitToken` | string | string | Go: `string` |
| `containerId` | string | string | Go: `string` |
| `containerName` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `REVIEW_RUN_CONTROL_UNAVAILABLE`, `REVIEW_RUN_NOT_FOUND`, `REVIEW_REPORT_REJECTED`, `REVIEW_RUN_CONTROL_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-71"></a>

## POST `/api/internal/workforce/v1/review-runs/:runId/failures`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:867](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L867)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:120](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L120)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reviewFailureRequest`](wire-types.md#t-ba0ac47d3cef95ac)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `REVIEW_RUN_CONTROL_UNAVAILABLE`, `REVIEW_RUN_NOT_FOUND`, `REVIEW_REPORT_REJECTED`, `REVIEW_RUN_CONTROL_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-72"></a>

## POST `/api/internal/workforce/v1/review-runs/:runId/launch-permits`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:790](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L790)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:117](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L117)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reviewLeaseRequest`](wire-types.md#t-8086fb2559c7abb7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `permit` | integer；Go `internal/app/workforce/reviewrun/facade.LaunchPermit` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `REVIEW_RUN_CONTROL_UNAVAILABLE`, `REVIEW_RUN_NOT_FOUND`, `REVIEW_REPORT_REJECTED`, `REVIEW_RUN_CONTROL_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-73"></a>

## PUT `/api/internal/workforce/v1/review-runs/:runId/lease`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:772](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L772)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:116](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L116)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reviewLeaseRequest`](wire-types.md#t-8086fb2559c7abb7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `expiresAt` | string；Go `string` |
| `receiptDigest` | string；Go `string` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `REVIEW_RUN_CONTROL_UNAVAILABLE`, `REVIEW_RUN_NOT_FOUND`, `REVIEW_REPORT_REJECTED`, `REVIEW_RUN_CONTROL_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-74"></a>

## POST `/api/internal/workforce/v1/review-runs/:runId/recover`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:750](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L750)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:115](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L115)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reviewRecoveryRequest`](wire-types.md#t-9f3a430337ecee42)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `containerId` | string | string | Go: `string` |
| `containerName` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `claim` | integer；Go `internal/app/workforce/reviewrun/facade.Claim` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `REVIEW_RUN_CONTROL_UNAVAILABLE`, `REVIEW_RUN_NOT_FOUND`, `REVIEW_REPORT_REJECTED`, `REVIEW_RUN_CONTROL_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-75"></a>

## POST `/api/internal/workforce/v1/review-runs/:runId/reports`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:844](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L844)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:119](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L119)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `runId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.reviewReportRequest`](wire-types.md#t-9213912d5cdba281)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `permitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `permitToken` | string | string | Go: `string` |
| `report` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `usage` | integer | integer | [`internal/app/workforce/reviewrun/domain.UsageFacts`](wire-types.md#t-8db36876e421763a)；Go: `internal/app/workforce/reviewrun/facade.UsageFacts` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `result` | integer；Go `internal/app/workforce/reviewrun/facade.ReportResult` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `REVIEW_RUN_CONTROL_UNAVAILABLE`, `REVIEW_RUN_NOT_FOUND`, `REVIEW_REPORT_REJECTED`, `REVIEW_RUN_CONTROL_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-76"></a>

## POST `/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/claims`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:211](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L211)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:168](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L168)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `sourceIngestionRunId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.postprocessClaimRequest`](wire-types.md#t-b6d513fdac3291f3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.SourceIngestionClaim`](wire-types.md#t-eca9027d7a67f4c6)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `SOURCE_INGESTION_NOT_FOUND`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-77"></a>

## POST `/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/credential-materializations`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:229](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L229)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:169](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L169)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `sourceIngestionRunId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.sourceIngestionLeaseRequest`](wire-types.md#t-6917fe13dd6d9b6c)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.SourceCredential`](wire-types.md#t-d795b7ce16e84109)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `SOURCE_INGESTION_NOT_FOUND`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-78"></a>

## POST `/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/failures`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:285](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L285)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:172](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L172)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `sourceIngestionRunId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.sourceIngestionFailureRequest`](wire-types.md#t-404867bd00b530df)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `recorded` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `SOURCE_INGESTION_NOT_FOUND`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-79"></a>

## POST `/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/observations`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:248](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L248)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:170](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L170)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `sourceIngestionRunId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.sourceIngestionObservationRequest`](wire-types.md#t-cb33383fd6a0a2ec)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `resolvedCommit` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `resolvedCommit` | string；Go `string` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `SOURCE_INGESTION_NOT_FOUND`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-80"></a>

## POST `/api/internal/workforce/v1/source-ingestion-runs/:sourceIngestionRunId/results`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:266](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L266)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:171](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L171)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `sourceIngestionRunId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.sourceIngestionResultRequest`](wire-types.md#t-03ffd4bcb5caba8d)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/workspaceinput/application.SourceIngestionResult`](wire-types.md#t-39631ebb08f1ced3)；Go: `internal/app/workforce/workspaceinput/application.SourceIngestionResult` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.SourceRevision`](wire-types.md#t-3d5781a76d6bd975)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `SOURCE_INGESTION_NOT_FOUND`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-81"></a>

## POST `/api/internal/workforce/v1/tasks/:taskId/workspace-gc-eligibility`

实现：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:182](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L182)。注册：[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:121](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L121)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `taskId` | string |

### 请求体

[`internal/app/workforce/taskdispatch/controller.workspaceGCEligibilityRequest`](wire-types.md#t-baeb18a3c002cfb4)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/task/facade.WorkspaceGCEligibility`](wire-types.md#t-373da65ba27b4995)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `WORKSPACE_RETENTION_TASK_ID_INVALID`, `WORKSPACE_RETENTION_AUTHORITY_UNAVAILABLE`, `WORKSPACE_RETENTION_IDENTITY_CONFLICT`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-82"></a>

## POST `/api/internal/workforce/v1/workers/:workerId/deregister`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:304](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L304)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:59](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L59)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `workerId` | string |

### 请求体

[`internal/app/workforce/workerregistry/controller.deregisterRequest`](wire-types.md#t-37e4b010b6048064)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `INVALID_MACHINE_ENVELOPE`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-83"></a>

## POST `/api/internal/workforce/v1/workers/:workerId/heartbeat`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:266](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L266)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:57](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L57)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `workerId` | string |

### 请求体

[`internal/app/workforce/workerregistry/controller.heartbeatRequest`](wire-types.md#t-1ff25250e69ce4f6)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `consumerState` | string | string | Go: `string` |
| `dockerReady` | boolean | boolean | Go: `bool` |
| `activeAttempts` | integer | integer | Go: `int` |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `hostname` | string | string | Go: `string` |
| `reportedIp` | string | string | Go: `string` |
| `primaryInterface` | string | string | Go: `string` |
| `primaryMac` | string | string | Go: `string` |
| `interfaces` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `resourceSnapshot` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |
| `serverTime` | string；Go `string` |
| `desiredState` | string；Go `string` |
| `derivedState` | string；Go `string` |
| `consumerDirective` | string；Go `string` |
| `refreshInventoryGeneration` | integer；Go `int` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `INVALID_MACHINE_ENVELOPE`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-84"></a>

## PUT `/api/internal/workforce/v1/workers/:workerId/image-inventory`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:286](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L286)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:58](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L58)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `workerId` | string |

### 请求体

[`internal/app/workforce/workerregistry/controller.inventoryRequest`](wire-types.md#t-4b5ac93677efc845)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `inventoryVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `inventoryDigest` | string | string | Go: `string` |
| `images` | array<integer> | array<integer> | [`internal/app/workforce/workerregistry/domain.Image`](wire-types.md#t-7bd66bcc2721dfe0)；Go: `[]internal/app/workforce/workerregistry/domain.Image` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `ack` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `INVALID_MACHINE_ENVELOPE`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-85"></a>

## GET `/api/internal/workforce/v1/workers/:workerId/reconciliation`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:316](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L316)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:60](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L60)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `X-Seven-Worker-Instance` | string |
| header | `X-Seven-Host-Boot-ID` | string |
| header | `X-Seven-Request-ID` | string |
| path | `workerId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workerregistry/domain.Reconciliation`](wire-types.md#t-753781e520cdac37)。

入口／同包辅助方法中可定位的 reasonCode：`INVALID_MACHINE_ENVELOPE`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-86"></a>

## POST `/api/internal/workforce/v1/workers/:workerId/register`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:233](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L233)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:56](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L56)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `workerId` | string |

### 请求体

[`internal/app/workforce/workerregistry/controller.registerRequest`](wire-types.md#t-6bd03c013abd6a55)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `startedAt` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |
| `revision` | string | string | Go: `string` |
| `buildDate` | string | string | Go: `string` |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `maxActiveReviews` | integer | integer | Go: `int` |
| `maxTotalContainers` | integer | integer | Go: `int` |
| `capabilities` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workerregistry/application.RegisterResult`](wire-types.md#t-52a772e122b0f397)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `INVALID_MACHINE_ENVELOPE`, `INVALID_STARTED_AT`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。
