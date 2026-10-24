# Web／管理 REST 方法参考

来源：随附独立源码快照（结构经检查）。共 **134** 个方法。使用 Framework 用户身份、权限与组织范围。权限列为空表示包装器未给出字面权限，不能解释为匿名开放。详见 [公共约定](common.md)。

此页逐项列出方法、URL、路径／查询／Header／表单参数、请求体字段、响应类型和实现位置。嵌套对象点进类型索引；`omitempty` 仅是编码规则，业务必填与状态前置条件由对应应用／域校验决定。各接口并不统一采用 PATCH 或自动幂等。

| 方法 | 路径 | Handler |
| --- | --- | --- |
| GET | [`/api/workforce/artifact-upload-attempts/:uploadId`](#route-1) | `uploadAttempt` |
| POST | [`/api/workforce/artifact-upload-attempts/:uploadId/retry`](#route-2) | `retryUpload` |
| GET | [`/api/workforce/backlog`](#route-3) | `backlog` |
| GET | [`/api/workforce/continuation-checkpoints/:checkpointId`](#route-4) | `getCheckpoint` |
| POST | [`/api/workforce/continuation-checkpoints/:checkpointId/approvals`](#route-5) | `approve` |
| GET | [`/api/workforce/delivery-attempts/:id`](#route-6) | `getAttempt` |
| GET | [`/api/workforce/delivery-requests/:id`](#route-7) | `getRequest` |
| POST | [`/api/workforce/delivery-requests/:id/cancel`](#route-8) | `cancel` |
| POST | [`/api/workforce/delivery-requests/:id/reauthorizations`](#route-9) | `reauthorizeRequest` |
| POST | [`/api/workforce/delivery-requests/:id/reconcile`](#route-10) | `reconcile` |
| POST | [`/api/workforce/delivery-requests/:id/retry`](#route-11) | `retry` |
| GET | [`/api/workforce/finalization-runs/:runId`](#route-12) | `finalizationRun` |
| POST | [`/api/workforce/finalization-runs/:runId/retry`](#route-13) | `retryFinalization` |
| GET | [`/api/workforce/harness-channels`](#route-14) | `listHarnessChannels` |
| GET | [`/api/workforce/hook-audit`](#route-15) | `listAudit` |
| GET | [`/api/workforce/hook-package-releases`](#route-16) | `listReleases` |
| GET | [`/api/workforce/hook-packages`](#route-17) | `listPackages` |
| POST | [`/api/workforce/hook-packages`](#route-18) | `createPackage` |
| GET | [`/api/workforce/hook-packages/:id`](#route-19) | `getPackage` |
| POST | [`/api/workforce/hook-packages/:id/revisions`](#route-20) | `createRevision` |
| PUT | [`/api/workforce/hook-revisions/:id`](#route-21) | `updateRevision` |
| POST | [`/api/workforce/hook-revisions/:id/archive-import`](#route-22) | `importArchive` |
| POST | [`/api/workforce/hook-revisions/:id/publish`](#route-23) | `publish` |
| POST | [`/api/workforce/hook-revisions/:id/test-runs`](#route-24) | `test` |
| POST | [`/api/workforce/hook-revisions/:id/validate`](#route-25) | `validate` |
| GET | [`/api/workforce/model-credentials`](#route-26) | `list` |
| POST | [`/api/workforce/model-credentials`](#route-27) | `create` |
| DELETE | [`/api/workforce/model-credentials/:id`](#route-28) | `delete` |
| PUT | [`/api/workforce/model-credentials/:id`](#route-29) | `update` |
| GET | [`/api/workforce/model-credentials/:id/invocations`](#route-30) | `listCredential` |
| PUT | [`/api/workforce/model-credentials/:id/status`](#route-31) | `status` |
| GET | [`/api/workforce/model-credentials/analytics`](#route-32) | `analytics` |
| POST | [`/api/workforce/model-credentials/models:discover`](#route-33) | `discover` |
| GET | [`/api/workforce/model-invocations/:id`](#route-34) | `getInvocation` |
| GET | [`/api/workforce/model-options`](#route-35) | `options` |
| GET | [`/api/workforce/open-integrations`](#route-36) | `listApplications` |
| POST | [`/api/workforce/open-integrations`](#route-37) | `createApplication` |
| POST | [`/api/workforce/open-integrations/:id/api-key/rotate`](#route-38) | `rotateAPIKey` |
| GET | [`/api/workforce/open-integrations/:id/deliveries`](#route-39) | `listDeliveries` |
| POST | [`/api/workforce/open-integrations/:id/disable`](#route-40) | `disableApplication` |
| POST | [`/api/workforce/open-integrations/:id/enable`](#route-41) | `enableApplication` |
| POST | [`/api/workforce/open-integrations/:id/signing-secret/rotate`](#route-42) | `rotateSigningSecret` |
| GET | [`/api/workforce/projects`](#route-43) | `list` |
| POST | [`/api/workforce/projects`](#route-44) | `create` |
| GET | [`/api/workforce/projects/:projectId`](#route-45) | `get` |
| PUT | [`/api/workforce/projects/:projectId`](#route-46) | `update` |
| GET | [`/api/workforce/projects/:projectId/analytics`](#route-47) | `project` |
| POST | [`/api/workforce/projects/:projectId/archive`](#route-48) | `archive` |
| POST | [`/api/workforce/projects/:projectId/repository/retry`](#route-49) | `retryRepository` |
| GET | [`/api/workforce/projects/:projectId/runtime-options`](#route-50) | `runtimeOptions` |
| GET | [`/api/workforce/projects/:projectId/tasks`](#route-51) | `listProjectTasks` |
| POST | [`/api/workforce/projects/:projectId/tasks`](#route-52) | `createProjectTask` |
| GET | [`/api/workforce/projects/analytics`](#route-53) | `projects` |
| POST | [`/api/workforce/reference-assets`](#route-54) | `reference` |
| GET | [`/api/workforce/reference-assets/:id`](#route-55) | `referenceView` |
| GET | [`/api/workforce/result-materialization-runs/:runId`](#route-56) | `getMaterialization` |
| POST | [`/api/workforce/result-materialization-runs/:runId/retry`](#route-57) | `retryMaterialization` |
| PUT | [`/api/workforce/result-processor-bindings/:id/environment`](#route-58) | `setEnvironment` |
| PUT | [`/api/workforce/result-processor-bindings/default`](#route-59) | `setDefault` |
| GET | [`/api/workforce/result-processor-fixture-runs/:id`](#route-60) | `getFixture` |
| PUT | [`/api/workforce/result-processor-revisions/:id`](#route-61) | `updateRevision` |
| POST | [`/api/workforce/result-processor-revisions/:id/archive-import`](#route-62) | `importArchive` |
| POST | [`/api/workforce/result-processor-revisions/:id/fixture-runs`](#route-63) | `createFixture` |
| POST | [`/api/workforce/result-processor-revisions/:id/publish`](#route-64) | `publish` |
| POST | [`/api/workforce/result-processor-revisions/:id/validate`](#route-65) | `validate` |
| GET | [`/api/workforce/result-processors`](#route-66) | `listProcessors` |
| POST | [`/api/workforce/result-processors`](#route-67) | `createProcessor` |
| GET | [`/api/workforce/result-processors/:id`](#route-68) | `getProcessor` |
| POST | [`/api/workforce/result-processors/:id/revisions`](#route-69) | `createRevision` |
| POST | [`/api/workforce/review-operations/:id/reauthorizations`](#route-70) | `reauthorizeReview` |
| POST | [`/api/workforce/review-operations/:id/reconcile`](#route-71) | `reconcileReview` |
| GET | [`/api/workforce/review-runs/:reviewRunId`](#route-72) | `runDetail` |
| POST | [`/api/workforce/review-runs/:reviewRunId/decisions`](#route-73) | `decide` |
| GET | [`/api/workforce/source-ingestion-runs/:id`](#route-74) | `ingestionRun` |
| GET | [`/api/workforce/task-notifications/readiness`](#route-75) | `readiness` |
| GET | [`/api/workforce/task-turns/:turnId`](#route-76) | `getTurn` |
| GET | [`/api/workforce/tasks`](#route-77) | `list` |
| GET | [`/api/workforce/tasks/:id`](#route-78) | `get` |
| GET | [`/api/workforce/tasks/:id/artifacts`](#route-79) | `listArtifacts` |
| GET | [`/api/workforce/tasks/:id/artifacts/:artifactId/download`](#route-80) | `downloadArtifact` |
| POST | [`/api/workforce/tasks/:id/cancel`](#route-81) | `cancel` |
| GET | [`/api/workforce/tasks/:id/continuations`](#route-82) | `continuations` |
| GET | [`/api/workforce/tasks/:id/evaluations`](#route-83) | `evaluations` |
| GET | [`/api/workforce/tasks/:id/events`](#route-84) | `events` |
| GET | [`/api/workforce/tasks/:id/events/stream`](#route-85) | `eventStream` |
| GET | [`/api/workforce/tasks/:id/finalization-runs`](#route-86) | `finalizationRuns` |
| GET | [`/api/workforce/tasks/:id/git-plan`](#route-87) | `gitPlan` |
| GET | [`/api/workforce/tasks/:id/independent-review`](#route-88) | `independentReview` |
| GET | [`/api/workforce/tasks/:id/input`](#route-89) | `taskInput` |
| PATCH | [`/api/workforce/tasks/:id/metadata`](#route-90) | `updateMetadata` |
| GET | [`/api/workforce/tasks/:id/model-invocations`](#route-91) | `listTask` |
| GET | [`/api/workforce/tasks/:id/recovery`](#route-92) | `recovery` |
| GET | [`/api/workforce/tasks/:id/result-outputs`](#route-93) | `resultOutputs` |
| GET | [`/api/workforce/tasks/:taskId/completion-check`](#route-94) | `get` |
| POST | [`/api/workforce/tasks/:taskId/completion-check-decisions`](#route-95) | `decide` |
| GET | [`/api/workforce/tasks/:taskId/completion-check-revisions`](#route-96) | `revisionView` |
| POST | [`/api/workforce/tasks/:taskId/completion-check-revisions`](#route-97) | `requestRevision` |
| GET | [`/api/workforce/tasks/:taskId/completion-check-revisions/pending`](#route-98) | `pendingRevision` |
| GET | [`/api/workforce/tasks/:taskId/conversation`](#route-99) | `conversation` |
| GET | [`/api/workforce/tasks/:taskId/conversation/:itemId`](#route-100) | `item` |
| POST | [`/api/workforce/tasks/:taskId/conversation/read`](#route-101) | `read` |
| GET | [`/api/workforce/tasks/:taskId/deliveries`](#route-102) | `listRequests` |
| POST | [`/api/workforce/tasks/:taskId/delivery-requests`](#route-103) | `createRequest` |
| POST | [`/api/workforce/tasks/:taskId/delivery-requests/:requestId/decisions`](#route-104) | `decide` |
| GET | [`/api/workforce/tasks/:taskId/execution-operations/:operationId`](#route-105) | `get` |
| POST | [`/api/workforce/tasks/:taskId/execution-operations/:operationId/decisions`](#route-106) | `decide` |
| GET | [`/api/workforce/tasks/:taskId/independent-review/runs`](#route-107) | `listRuns` |
| GET | [`/api/workforce/tasks/:taskId/interactions`](#route-108) | `questions` |
| POST | [`/api/workforce/tasks/:taskId/interactions/:interactionId/responses`](#route-109) | `respond` |
| POST | [`/api/workforce/tasks/:taskId/messages`](#route-110) | `message` |
| GET | [`/api/workforce/tasks/:taskId/network-grants`](#route-111) | `grants` |
| POST | [`/api/workforce/tasks/:taskId/network-grants/:grantId/revoke`](#route-112) | `revoke` |
| GET | [`/api/workforce/tasks/:taskId/notification-records`](#route-113) | `listRecords` |
| GET | [`/api/workforce/tasks/:taskId/notification-subscription`](#route-114) | `getSubscription` |
| PUT | [`/api/workforce/tasks/:taskId/notification-subscription`](#route-115) | `updateSubscription` |
| GET | [`/api/workforce/tasks/:taskId/result-materializations`](#route-116) | `listMaterializations` |
| POST | [`/api/workforce/tasks/:taskId/result-materializations`](#route-117) | `createMaterialization` |
| GET | [`/api/workforce/tasks/:taskId/turns`](#route-118) | `listTurns` |
| POST | [`/api/workforce/tasks/:taskId/turns`](#route-119) | `createTurn` |
| GET | [`/api/workforce/tasks/analytics`](#route-120) | `tasks` |
| GET | [`/api/workforce/workbench`](#route-121) | `workbench` |
| GET | [`/api/workforce/workers`](#route-122) | `list` |
| POST | [`/api/workforce/workers`](#route-123) | `create` |
| GET | [`/api/workforce/workers/:id`](#route-124) | `get` |
| GET | [`/api/workforce/workers/:id/assignments`](#route-125) | `assignments` |
| PUT | [`/api/workforce/workers/:id/capacity`](#route-126) | `capacity` |
| POST | [`/api/workforce/workers/:id/decommission`](#route-127) | `decommission` |
| PUT | [`/api/workforce/workers/:id/desired-state`](#route-128) | `desiredState` |
| POST | [`/api/workforce/workers/:id/image-inventory-refreshes`](#route-129) | `refreshInventory` |
| GET | [`/api/workforce/workers/:id/metrics`](#route-130) | `workerMetrics` |
| POST | [`/api/workforce/workers/:id/token-rotations`](#route-131) | `rotate` |
| GET | [`/api/workforce/workers/metrics`](#route-132) | `metrics` |
| GET | [`/api/workforce/workspace-source-revisions/:id`](#route-133) | `sourceRevision` |
| POST | [`/api/workforce/workspace-sources/uploads`](#route-134) | `uploadSource` |

<a id="route-1"></a>

## GET `/api/workforce/artifact-upload-attempts/:uploadId`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:390](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L390)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:168](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L168)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `uploadId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/artifact/domain.UploadView`](wire-types.md#t-4a71cd3513962d40)。

入口／同包辅助方法中可定位的 reasonCode：`POSTPROCESS_UNAVAILABLE`, `INVALID_ID`, `POSTPROCESS_NOT_FOUND`, `POSTPROCESS_RETRY_CONFLICT`, `DATABASE_ERROR`, `NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-2"></a>

## POST `/api/workforce/artifact-upload-attempts/:uploadId/retry`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:440](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L440)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:169](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L169)。

权限：`workforce:artifact-upload:retry`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `uploadId` | string |

### 请求体

[`inline.a4cf61e687be`](wire-types.md#t-678f1c5438f3c6f7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/artifact/domain.UploadView`](wire-types.md#t-4a71cd3513962d40)。

入口／同包辅助方法中可定位的 reasonCode：`POSTPROCESS_UNAVAILABLE`, `INVALID_ID`, `POSTPROCESS_NOT_FOUND`, `POSTPROCESS_RETRY_CONFLICT`, `DATABASE_ERROR`, `NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-3"></a>

## GET `/api/workforce/backlog`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:512](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L512)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:152](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L152)。

权限：`workforce:task:list`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/task/domain.BacklogSummary`](wire-types.md#t-68e1c55e3001858a)。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-4"></a>

## GET `/api/workforce/continuation-checkpoints/:checkpointId`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:145](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L145)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L34)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `checkpointId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`int` / [`internal/app/workforce/continuation/domain.Checkpoint`](wire-types.md#t-33457f63eb133ea3)。

<a id="route-5"></a>

## POST `/api/workforce/continuation-checkpoints/:checkpointId/approvals`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:109](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L109)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L35)。

权限：`workforce:task:continuation:approve`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `checkpointId` | string |

### 请求体

[`internal/app/workforce/continuation/application.ApproveRequest`](wire-types.md#t-d29ba38818d96a5d)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`int` / [`internal/app/workforce/continuation/domain.Checkpoint`](wire-types.md#t-33457f63eb133ea3)。

<a id="route-6"></a>

## GET `/api/workforce/delivery-attempts/:id`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:47](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L47)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:37](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L37)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.AttemptView`](wire-types.md#t-103450ff23116498)。

<a id="route-7"></a>

## GET `/api/workforce/delivery-requests/:id`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:43](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L43)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:30](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L30)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-8"></a>

## POST `/api/workforce/delivery-requests/:id/cancel`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:118](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L118)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L32)。

权限：`workforce:delivery:cancel`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-9"></a>

## POST `/api/workforce/delivery-requests/:id/reauthorizations`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:97](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L97)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:31](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L31)。

权限：`workforce:delivery:approve`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.9a7d2f28f426`](wire-types.md#t-72ff9a926568a75e)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-10"></a>

## POST `/api/workforce/delivery-requests/:id/reconcile`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:126](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L126)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L34)。

权限：`workforce:delivery:reconcile`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-11"></a>

## POST `/api/workforce/delivery-requests/:id/retry`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:122](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L122)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L33)。

权限：`workforce:delivery:retry`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-12"></a>

## GET `/api/workforce/finalization-runs/:runId`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:368](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L368)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:166](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L166)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `runId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/artifact/domain.FinalizationView`](wire-types.md#t-7afe4f6afad0abf6)。

入口／同包辅助方法中可定位的 reasonCode：`POSTPROCESS_UNAVAILABLE`, `INVALID_ID`, `POSTPROCESS_NOT_FOUND`, `POSTPROCESS_RETRY_CONFLICT`, `DATABASE_ERROR`, `NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-13"></a>

## POST `/api/workforce/finalization-runs/:runId/retry`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:412](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L412)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:167](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L167)。

权限：`workforce:finalization:retry`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `runId` | string |

### 请求体

[`inline.a4cf61e687be`](wire-types.md#t-678f1c5438f3c6f7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/artifact/domain.FinalizationView`](wire-types.md#t-7afe4f6afad0abf6)。

入口／同包辅助方法中可定位的 reasonCode：`POSTPROCESS_UNAVAILABLE`, `INVALID_ID`, `POSTPROCESS_NOT_FOUND`, `POSTPROCESS_RETRY_CONFLICT`, `DATABASE_ERROR`, `NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-14"></a>

## GET `/api/workforce/harness-channels`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L41)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:38](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L38)。

权限：`workforce:result-processor:author`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/hook/infrastructure.HarnessChannel`](wire-types.md#t-9f2ee9c0fc8badec)>。

<a id="route-15"></a>

## GET `/api/workforce/hook-audit`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:153](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L153)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:37](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L37)。

权限：`workforce:hook-audit:list`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/hook/infrastructure.AuditEvent`](wire-types.md#t-71a75838e8fcb76f)>。

<a id="route-16"></a>

## GET `/api/workforce/hook-package-releases`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:145](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L145)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:36](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L36)。

权限：`workforce:hook-package:list`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/hook/infrastructure.Release`](wire-types.md#t-e2d455e6a41bf699)>。

<a id="route-17"></a>

## GET `/api/workforce/hook-packages`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:53](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L53)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:27](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L27)。

权限：`workforce:hook-package:list`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/hook/infrastructure.Package`](wire-types.md#t-b2b94d9743d02662)>。

<a id="route-18"></a>

## POST `/api/workforce/hook-packages`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:69](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L69)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:28](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L28)。

权限：`workforce:hook-package:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/hook/application.CreatePackageCommand`](wire-types.md#t-50d7a0f71c7be8a2)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `category` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/hook/domain.Manifest`](wire-types.md#t-e0fc1343c8a7c48b)；Go: `internal/app/workforce/hook/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/hook/domain.SourceFile`](wire-types.md#t-1d69f951596ffc41)；Go: `[]internal/app/workforce/hook/domain.SourceFile` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/infrastructure.Package`](wire-types.md#t-b2b94d9743d02662)。

<a id="route-19"></a>

## GET `/api/workforce/hook-packages/:id`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:61](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L61)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:29](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L29)。

权限：`workforce:hook-package:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/infrastructure.Package`](wire-types.md#t-b2b94d9743d02662)。

<a id="route-20"></a>

## POST `/api/workforce/hook-packages/:id/revisions`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:45](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L45)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:30](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L30)。

权限：`workforce:hook-package:update`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/infrastructure.Package`](wire-types.md#t-b2b94d9743d02662)。

<a id="route-21"></a>

## PUT `/api/workforce/hook-revisions/:id`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:81](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L81)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:31](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L31)。

权限：`workforce:hook-package:update`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.c6dba04949c2`](wire-types.md#t-93dd0725050150d8)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedRevision` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `manifest` | integer | integer | [`internal/app/workforce/hook/domain.Manifest`](wire-types.md#t-e0fc1343c8a7c48b)；Go: `internal/app/workforce/hook/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/hook/domain.SourceFile`](wire-types.md#t-1d69f951596ffc41)；Go: `[]internal/app/workforce/hook/domain.SourceFile` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/infrastructure.Package`](wire-types.md#t-b2b94d9743d02662)。

<a id="route-22"></a>

## POST `/api/workforce/hook-revisions/:id/archive-import`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:97](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L97)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L32)。

权限：`workforce:hook-package:update`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `expectedRevision` | string |
| path | `id` | string |

### 请求体

原始二进制 tar.gz archive body（不是 JSON 或 multipart）；最大 8 MiB。query expectedRevision:string(decimal int64) 用于版本检查，具体必填校验见该方法源码。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/infrastructure.Package`](wire-types.md#t-b2b94d9743d02662)。

<a id="route-23"></a>

## POST `/api/workforce/hook-revisions/:id/publish`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:131](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L131)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L35)。

权限：`workforce:hook-package:publish`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.60f289d86afe`](wire-types.md#t-2f50884469a4850f)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `version` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/infrastructure.Release`](wire-types.md#t-e2d455e6a41bf699)。

<a id="route-24"></a>

## POST `/api/workforce/hook-revisions/:id/test-runs`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:119](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L119)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L34)。

权限：`workforce:hook-package:test`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/hook/application.TestCommand`](wire-types.md#t-0f4bfabd9d4f9616)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `event` | string | string | Go: `string` |
| `fixture` | object<string, 任意 JSON 值> | object<string, 任意 JSON 值> | Go: `map[string]any` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/application.TestResult`](wire-types.md#t-8864a7d84d8c394a)。

<a id="route-25"></a>

## POST `/api/workforce/hook-revisions/:id/validate`

实现：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:111](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L111)。注册：[seven-agent-force-server/internal/app/workforce/hook/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/hook/controller/http.go#L33)。

权限：`workforce:hook-package:validate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/hook/domain.Validation`](wire-types.md#t-87d6c7ecb648bdbd)。

<a id="route-26"></a>

## GET `/api/workforce/model-credentials`

实现：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:59](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L59)。注册：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:24](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L24)。

权限：`workforce:model-credential:list`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/provider/domain.View`](wire-types.md#t-fb6ac0d0d6ffab6e)>。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `MODEL_CREDENTIAL_NOT_FOUND`, `MODEL_CREDENTIAL_PROTOCOL_IN_USE`, `MODEL_DISCOVERY_AUTHENTICATION_FAILED`, `MODEL_DISCOVERY_UNAVAILABLE`, `MODEL_CREDENTIAL_INVALID`, `MODEL_CREDENTIAL_INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-27"></a>

## POST `/api/workforce/model-credentials`

实现：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L41)。注册：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:25](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L25)。

权限：`workforce:model-credential:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/provider/controller.createRequest`](wire-types.md#t-913df7d71f4540bb)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `baseUrl` | string | string | Go: `string` |
| `apiKey` | string | string | Go: `string` |
| `models` | array<string> | array<string> | Go: `[]string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/provider/domain.View`](wire-types.md#t-fb6ac0d0d6ffab6e)。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `MODEL_CREDENTIAL_NOT_FOUND`, `MODEL_CREDENTIAL_PROTOCOL_IN_USE`, `MODEL_DISCOVERY_AUTHENTICATION_FAILED`, `MODEL_DISCOVERY_UNAVAILABLE`, `MODEL_CREDENTIAL_INVALID`, `MODEL_CREDENTIAL_INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-28"></a>

## DELETE `/api/workforce/model-credentials/:id`

实现：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:134](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L134)。注册：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:28](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L28)。

权限：`workforce:model-credential:delete`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.0b483ba1663f`](wire-types.md#t-27bbabc2a5537467)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/provider/domain.DeleteResult`](wire-types.md#t-f5c86ea3d871b06b)。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `MODEL_CREDENTIAL_NOT_FOUND`, `MODEL_CREDENTIAL_PROTOCOL_IN_USE`, `MODEL_DISCOVERY_AUTHENTICATION_FAILED`, `MODEL_DISCOVERY_UNAVAILABLE`, `MODEL_CREDENTIAL_INVALID`, `MODEL_CREDENTIAL_INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-29"></a>

## PUT `/api/workforce/model-credentials/:id`

实现：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:95](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L95)。注册：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:26](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L26)。

权限：`workforce:model-credential:update`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/provider/controller.updateRequest`](wire-types.md#t-3f18a7682be27178)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `baseUrl` | string | string | Go: `string` |
| `apiKey` | string | string | Go: `string` |
| `models` | array<string> | array<string> | Go: `[]string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/provider/domain.View`](wire-types.md#t-fb6ac0d0d6ffab6e)。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `MODEL_CREDENTIAL_NOT_FOUND`, `MODEL_CREDENTIAL_PROTOCOL_IN_USE`, `MODEL_DISCOVERY_AUTHENTICATION_FAILED`, `MODEL_DISCOVERY_UNAVAILABLE`, `MODEL_CREDENTIAL_INVALID`, `MODEL_CREDENTIAL_INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-30"></a>

## GET `/api/workforce/model-credentials/:id/invocations`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:84](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L84)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:51](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L51)。

权限：`workforce:model-credential:usage`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `limit` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/providerinvocation/domain.View`](wire-types.md#t-e248bb8b954296bd)>。

<a id="route-31"></a>

## PUT `/api/workforce/model-credentials/:id/status`

实现：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:113](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L113)。注册：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:27](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L27)。

权限：`workforce:model-credential:status`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.20d3df089fa8`](wire-types.md#t-1551860725c00813)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/provider/domain.View`](wire-types.md#t-fb6ac0d0d6ffab6e)。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `MODEL_CREDENTIAL_NOT_FOUND`, `MODEL_CREDENTIAL_PROTOCOL_IN_USE`, `MODEL_DISCOVERY_AUTHENTICATION_FAILED`, `MODEL_DISCOVERY_UNAVAILABLE`, `MODEL_CREDENTIAL_INVALID`, `MODEL_CREDENTIAL_INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-32"></a>

## GET `/api/workforce/model-credentials/analytics`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:56](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L56)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:50](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L50)。

权限：`workforce:model-credential:usage`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `days` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/providerinvocation/domain.Analytics`](wire-types.md#t-b9f4af0479712f32)。

<a id="route-33"></a>

## POST `/api/workforce/model-credentials/models:discover`

实现：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:154](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L154)。注册：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:29](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L29)。

权限：`workforce:model-credential:update`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`inline.955a70a39e85`](wire-types.md#t-0e924ed22d3b395e)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `protocol` | string | string | Go: `string` |
| `baseUrl` | string | string | Go: `string` |
| `apiKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `models` | array<string>；Go `[]string` |

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `MODEL_CREDENTIAL_NOT_FOUND`, `MODEL_CREDENTIAL_PROTOCOL_IN_USE`, `MODEL_DISCOVERY_AUTHENTICATION_FAILED`, `MODEL_DISCOVERY_UNAVAILABLE`, `MODEL_CREDENTIAL_INVALID`, `MODEL_CREDENTIAL_INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-34"></a>

## GET `/api/workforce/model-invocations/:id`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:120](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L120)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:53](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L53)。

权限：`workforce:model-credential:usage`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/providerinvocation/domain.View`](wire-types.md#t-e248bb8b954296bd)。

<a id="route-35"></a>

## GET `/api/workforce/model-options`

实现：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:72](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L72)。注册：[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:30](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L30)。

权限：`workforce:task:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/provider/domain.ModelOption`](wire-types.md#t-e747154f7cf1874e)>。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `MODEL_CREDENTIAL_NOT_FOUND`, `MODEL_CREDENTIAL_PROTOCOL_IN_USE`, `MODEL_DISCOVERY_AUTHENTICATION_FAILED`, `MODEL_DISCOVERY_UNAVAILABLE`, `MODEL_CREDENTIAL_INVALID`, `MODEL_CREDENTIAL_INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-36"></a>

## GET `/api/workforce/open-integrations`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:256](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L256)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:37](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L37)。

权限：`workforce:open-integration:read`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/openintegration/domain.Application`](wire-types.md#t-07bad8b17300354e)>。

<a id="route-37"></a>

## POST `/api/workforce/open-integrations`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:270](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L270)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:38](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L38)。

权限：`workforce:open-integration:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/openintegration/application.CreateApplicationRequest`](wire-types.md#t-69d3652dc248495d)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `webhookUrl` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/application.CreatedApplication`](wire-types.md#t-ec45d8dc446df3d2)。

<a id="route-38"></a>

## POST `/api/workforce/open-integrations/:id/api-key/rotate`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:307](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L307)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L41)。

权限：`workforce:open-integration:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/application.APIKeyRotation`](wire-types.md#t-937480b1b06b587d)。

<a id="route-39"></a>

## GET `/api/workforce/open-integrations/:id/deliveries`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:404](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L404)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:43](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L43)。

权限：`workforce:open-integration:read`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `limit` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/openintegration/domain.Delivery`](wire-types.md#t-0d9bb0a0809d94fd)>。

<a id="route-40"></a>

## POST `/api/workforce/open-integrations/:id/disable`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:303](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L303)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:40](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L40)。

权限：`workforce:open-integration:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]string`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `status` | string；Go `string` |

<a id="route-41"></a>

## POST `/api/workforce/open-integrations/:id/enable`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:299](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L299)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:39](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L39)。

权限：`workforce:open-integration:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]string`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `status` | string；Go `string` |

<a id="route-42"></a>

## POST `/api/workforce/open-integrations/:id/signing-secret/rotate`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:324](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L324)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:42](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L42)。

权限：`workforce:open-integration:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/application.SigningSecretRotation`](wire-types.md#t-7ec6ae3c73899e4c)。

<a id="route-43"></a>

## GET `/api/workforce/projects`

实现：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:101](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L101)。注册：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:39](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L39)。

权限：`workforce:task:list`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `limit` | string |
| query | `query` | string |
| query | `status` | string |
| query | `cursor` | string(decimal ID) |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`untyped nil` / `map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `items` | array<object<string, 任意 JSON 值>>；Go `[]map[string]any` |
| `nextCursor` | 任意 JSON 值；Go `any` |

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `PROJECT_ACTOR_REQUIRED`, `INVALID_ID`, `PROJECT_NOT_FOUND`, `PROJECT_VERSION_CONFLICT`, `PROJECT_IDEMPOTENCY_CONFLICT`, `PROJECT_ARCHIVED`, `PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED`, `PROJECT_REPOSITORY_URL_INVALID`, `PROJECT_REPOSITORY_CREDENTIAL_REQUIRED`, `PROJECT_REPOSITORY_AUTHENTICATION_FAILED`, `PROJECT_REPOSITORY_NOT_FOUND`, `PROJECT_REPOSITORY_REF_NOT_FOUND`, `PROJECT_REPOSITORY_PUSH_UNAVAILABLE`, `PROJECT_REPOSITORY_REVIEW_UNAVAILABLE`, `PROJECT_REPOSITORY_NOT_READY`, `PROJECT_REPOSITORY_PREPARATION_FAILED`, `PROJECT_CONFIGURATION_INVALID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-44"></a>

## POST `/api/workforce/projects`

实现：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:58](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L58)。注册：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:40](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L40)。

权限：`workforce:task:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/project/application.UpsertRequest`](wire-types.md#t-9ef6a23d3ee49ef3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectKey` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `description` | string | string | 零值可省略；Go: `string` |
| `departmentId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `repository` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/project/application.RepositorySelection`](wire-types.md#t-88b16e49b05c31f3)；Go: `*internal/app/workforce/project/application.RepositorySelection` |
| `techStack` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `agentGuidance` | string | string | 零值可省略；Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewPrompt` | string | string | 零值可省略；Go: `string` |
| `hookReleaseIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `delivery` | integer | integer | [`internal/app/workforce/project/application.DeliveryPolicy`](wire-types.md#t-c325b275f15ded1a)；Go: `internal/app/workforce/project/application.DeliveryPolicy` |
| `notification` | integer | integer | [`internal/app/workforce/project/application.NotificationPolicy`](wire-types.md#t-ed1e365e6decc277)；Go: `internal/app/workforce/project/application.NotificationPolicy` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `PROJECT_ACTOR_REQUIRED`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `PROJECT_NOT_FOUND`, `PROJECT_VERSION_CONFLICT`, `PROJECT_IDEMPOTENCY_CONFLICT`, `PROJECT_ARCHIVED`, `PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED`, `PROJECT_REPOSITORY_URL_INVALID`, `PROJECT_REPOSITORY_CREDENTIAL_REQUIRED`, `PROJECT_REPOSITORY_AUTHENTICATION_FAILED`, `PROJECT_REPOSITORY_NOT_FOUND`, `PROJECT_REPOSITORY_REF_NOT_FOUND`, `PROJECT_REPOSITORY_PUSH_UNAVAILABLE`, `PROJECT_REPOSITORY_REVIEW_UNAVAILABLE`, `PROJECT_REPOSITORY_NOT_READY`, `PROJECT_REPOSITORY_PREPARATION_FAILED`, `PROJECT_CONFIGURATION_INVALID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-45"></a>

## GET `/api/workforce/projects/:projectId`

实现：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:89](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L89)。注册：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L41)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `projectId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `PROJECT_ACTOR_REQUIRED`, `INVALID_ID`, `PROJECT_NOT_FOUND`, `PROJECT_VERSION_CONFLICT`, `PROJECT_IDEMPOTENCY_CONFLICT`, `PROJECT_ARCHIVED`, `PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED`, `PROJECT_REPOSITORY_URL_INVALID`, `PROJECT_REPOSITORY_CREDENTIAL_REQUIRED`, `PROJECT_REPOSITORY_AUTHENTICATION_FAILED`, `PROJECT_REPOSITORY_NOT_FOUND`, `PROJECT_REPOSITORY_REF_NOT_FOUND`, `PROJECT_REPOSITORY_PUSH_UNAVAILABLE`, `PROJECT_REPOSITORY_REVIEW_UNAVAILABLE`, `PROJECT_REPOSITORY_NOT_READY`, `PROJECT_REPOSITORY_PREPARATION_FAILED`, `PROJECT_CONFIGURATION_INVALID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-46"></a>

## PUT `/api/workforce/projects/:projectId`

实现：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:72](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L72)。注册：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:42](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L42)。

权限：`workforce:task:metadata:update`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `projectId` | string |

### 请求体

[`internal/app/workforce/project/application.UpsertRequest`](wire-types.md#t-9ef6a23d3ee49ef3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectKey` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `description` | string | string | 零值可省略；Go: `string` |
| `departmentId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `repository` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/project/application.RepositorySelection`](wire-types.md#t-88b16e49b05c31f3)；Go: `*internal/app/workforce/project/application.RepositorySelection` |
| `techStack` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `agentGuidance` | string | string | 零值可省略；Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewPrompt` | string | string | 零值可省略；Go: `string` |
| `hookReleaseIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `delivery` | integer | integer | [`internal/app/workforce/project/application.DeliveryPolicy`](wire-types.md#t-c325b275f15ded1a)；Go: `internal/app/workforce/project/application.DeliveryPolicy` |
| `notification` | integer | integer | [`internal/app/workforce/project/application.NotificationPolicy`](wire-types.md#t-ed1e365e6decc277)；Go: `internal/app/workforce/project/application.NotificationPolicy` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `PROJECT_ACTOR_REQUIRED`, `INVALID_ID`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `PROJECT_NOT_FOUND`, `PROJECT_VERSION_CONFLICT`, `PROJECT_IDEMPOTENCY_CONFLICT`, `PROJECT_ARCHIVED`, `PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED`, `PROJECT_REPOSITORY_URL_INVALID`, `PROJECT_REPOSITORY_CREDENTIAL_REQUIRED`, `PROJECT_REPOSITORY_AUTHENTICATION_FAILED`, `PROJECT_REPOSITORY_NOT_FOUND`, `PROJECT_REPOSITORY_REF_NOT_FOUND`, `PROJECT_REPOSITORY_PUSH_UNAVAILABLE`, `PROJECT_REPOSITORY_REVIEW_UNAVAILABLE`, `PROJECT_REPOSITORY_NOT_READY`, `PROJECT_REPOSITORY_PREPARATION_FAILED`, `PROJECT_CONFIGURATION_INVALID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-47"></a>

## GET `/api/workforce/projects/:projectId/analytics`

实现：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:67](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L67)。注册：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:28](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L28)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `range` | string |
| path | `projectId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/taskanalytics/application.ProjectAnalytics`](wire-types.md#t-828e42c932531540)。

<a id="route-48"></a>

## POST `/api/workforce/projects/:projectId/archive`

实现：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:128](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L128)。注册：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:43](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L43)。

权限：`workforce:task:metadata:update`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `projectId` | string |

### 请求体

[`inline.733e0327e9df`](wire-types.md#t-0fffb04ef22296a8)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `PROJECT_ACTOR_REQUIRED`, `INVALID_ID`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `PROJECT_NOT_FOUND`, `PROJECT_VERSION_CONFLICT`, `PROJECT_IDEMPOTENCY_CONFLICT`, `PROJECT_ARCHIVED`, `PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED`, `PROJECT_REPOSITORY_URL_INVALID`, `PROJECT_REPOSITORY_CREDENTIAL_REQUIRED`, `PROJECT_REPOSITORY_AUTHENTICATION_FAILED`, `PROJECT_REPOSITORY_NOT_FOUND`, `PROJECT_REPOSITORY_REF_NOT_FOUND`, `PROJECT_REPOSITORY_PUSH_UNAVAILABLE`, `PROJECT_REPOSITORY_REVIEW_UNAVAILABLE`, `PROJECT_REPOSITORY_NOT_READY`, `PROJECT_REPOSITORY_PREPARATION_FAILED`, `PROJECT_CONFIGURATION_INVALID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-49"></a>

## POST `/api/workforce/projects/:projectId/repository/retry`

实现：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:46](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L46)。注册：[seven-agent-force-server/internal/app/workforce/project/controller/http.go:44](../../../seven-agent-force-server/internal/app/workforce/project/controller/http.go#L44)。

权限：`workforce:source:create`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `projectId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `PROJECT_ACTOR_REQUIRED`, `INVALID_ID`, `PROJECT_NOT_FOUND`, `PROJECT_VERSION_CONFLICT`, `PROJECT_IDEMPOTENCY_CONFLICT`, `PROJECT_ARCHIVED`, `PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED`, `PROJECT_REPOSITORY_URL_INVALID`, `PROJECT_REPOSITORY_CREDENTIAL_REQUIRED`, `PROJECT_REPOSITORY_AUTHENTICATION_FAILED`, `PROJECT_REPOSITORY_NOT_FOUND`, `PROJECT_REPOSITORY_REF_NOT_FOUND`, `PROJECT_REPOSITORY_PUSH_UNAVAILABLE`, `PROJECT_REPOSITORY_REVIEW_UNAVAILABLE`, `PROJECT_REPOSITORY_NOT_READY`, `PROJECT_REPOSITORY_PREPARATION_FAILED`, `PROJECT_CONFIGURATION_INVALID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-50"></a>

## GET `/api/workforce/projects/:projectId/runtime-options`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:172](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L172)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:149](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L149)。

权限：`workforce:task:create`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `modelCredentialId` | string |
| path | `projectId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionrouting/facade.RuntimeOptions`](wire-types.md#t-ba843f200612eb8f)。

入口／同包辅助方法中可定位的 reasonCode：`INVALID_ID`, `TASK_RUNTIME_SELECTION_REQUIRED`, `TASK_RUNTIME_SELECTION_UNAVAILABLE`, `TASK_RUNTIME_SELECTION_INCOMPATIBLE`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-51"></a>

## GET `/api/workforce/projects/:projectId/tasks`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:204](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L204)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:150](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L150)。

权限：`workforce:task:list`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `query` | string |
| query | `view` | string |
| query | `limit` | string |
| query | `cursor` | string |
| path | `projectId` | string |
| query | `from` | string(RFC3339 或 YYYY-MM-DD) |
| query | `to` | string(RFC3339 或 YYYY-MM-DD) |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `items` | array<integer>；Go `[]internal/app/workforce/task/controller.taskResponse` |
| `nextCursor` | 任意 JSON 值；Go `any` |

入口／同包辅助方法中可定位的 reasonCode：`INVALID_ID`, `TASK_LIST_FILTER_INVALID`, `DATABASE_ERROR`, `INTERACTION_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-52"></a>

## POST `/api/workforce/projects/:projectId/tasks`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:232](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L232)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:148](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L148)。

权限：`workforce:task:create`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Idempotency-Key` | string |
| path | `projectId` | string |

### 请求体

[`internal/app/workforce/task/facade.ProjectTaskRequest`](wire-types.md#t-1529f512453fbef3)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `instruction` | string | string | Go: `string` |
| `name` | string | string | 零值可省略；Go: `string` |
| `tags` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `referenceAssetIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `additionalCompletionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewEnabled` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `deliveryPreference` | string | string | 零值可省略；Go: `string` |
| `modelSelection` | integer | integer | [`internal/app/workforce/task/facade.ModelSelection`](wire-types.md#t-c395a518b87a2f05)；Go: `internal/app/workforce/task/facade.ModelSelection` |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionTimeoutSeconds` | integer | integer | 零值可省略；Go: `int` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `task` | integer；Go `internal/app/workforce/task/controller.taskResponse` |
| `replayed` | boolean；Go `bool` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_ID`, `INVALID_JSON`, `TENANT_SCOPE_REQUIRED`, `TASK_CREATOR_IDENTITY_REQUIRED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-53"></a>

## GET `/api/workforce/projects/analytics`

实现：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:50](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L50)。注册：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:27](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L27)。

权限：`workforce:task:list`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `range` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/taskanalytics/application.ProjectListAnalytics`](wire-types.md#t-4edb0c12b0489139)。

<a id="route-54"></a>

## POST `/api/workforce/reference-assets`

实现：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:67](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L67)。注册：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:30](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L30)。

权限：`workforce:reference:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/workspaceinput/application.ReferenceRequest`](wire-types.md#t-21e5ab0031c0bd5a)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `fileId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `displayName` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.ReferenceAsset`](wire-types.md#t-016e0a8320d13fcf)。

<a id="route-55"></a>

## GET `/api/workforce/reference-assets/:id`

实现：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:79](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L79)。注册：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:31](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L31)。

权限：`workforce:source:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.ReferenceAsset`](wire-types.md#t-016e0a8320d13fcf)。

<a id="route-56"></a>

## GET `/api/workforce/result-materialization-runs/:runId`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:57](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L57)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:38](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L38)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `runId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`int` / [`internal/app/workforce/continuation/domain.MaterializationView`](wire-types.md#t-e10445ca6ec9a028)。

<a id="route-57"></a>

## POST `/api/workforce/result-materialization-runs/:runId/retry`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:75](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L75)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:39](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L39)。

权限：`workforce:result-output:retry`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `runId` | string |

### 请求体

[`inline.a4cf61e687be`](wire-types.md#t-678f1c5438f3c6f7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`int` / `map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `accepted` | boolean；Go `bool` |

<a id="route-58"></a>

## PUT `/api/workforce/result-processor-bindings/:id/environment`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:188](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L188)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:44](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L44)。

权限：`workforce:result-processor:secret:bind`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/resultprocessor/infrastructure.EnvironmentBinding`](wire-types.md#t-897a7ccb60a1e5f0)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `processorReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `type` | string | string | Go: `string` |
| `phase` | string | string | Go: `string` |
| `config` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `secretRef` | string | string | 零值可省略；Go: `string` |
| `secretVersion` | string | string | 零值可省略；Go: `string` |
| `taskOverrideAllowed` | boolean | boolean | Go: `bool` |
| `networkPolicy` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.EnvironmentBinding`](wire-types.md#t-897a7ccb60a1e5f0)。

<a id="route-59"></a>

## PUT `/api/workforce/result-processor-bindings/default`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:173](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L173)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:43](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L43)。

权限：`workforce:result-processor:bind`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`inline.5933edbe2477`](wire-types.md#t-16d919fb1b8985b6)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `processorReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `name` | string；Go `string` |
| `processorReleaseId` | string；Go `string` |

<a id="route-60"></a>

## GET `/api/workforce/result-processor-fixture-runs/:id`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:149](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L149)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L41)。

权限：`workforce:result-processor:audit`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.FixtureRun`](wire-types.md#t-a2fa5e5df2f0a955)。

<a id="route-61"></a>

## PUT `/api/workforce/result-processor-revisions/:id`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:86](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L86)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:37](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L37)。

权限：`workforce:result-processor:author`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.7487eca6ecbe`](wire-types.md#t-7cef4645a5c4b0d0)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedRevision` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `manifest` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)；Go: `internal/app/workforce/resultprocessor/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/scriptasset/domain.SourceFile`](wire-types.md#t-17354ae5b0a2291f)；Go: `[]internal/app/workforce/resultprocessor/domain.SourceFile` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.Revision`](wire-types.md#t-14a66f1cbd666fcc)。

<a id="route-62"></a>

## POST `/api/workforce/result-processor-revisions/:id/archive-import`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:103](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L103)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:38](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L38)。

权限：`workforce:result-processor:author`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `expectedRevision` | string |
| path | `id` | string |

### 请求体

原始二进制 tar.gz archive body（不是 JSON 或 multipart）；最大 8 MiB。query expectedRevision:string(decimal int64) 用于版本检查，具体必填校验见该方法源码。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.Revision`](wire-types.md#t-14a66f1cbd666fcc)。

<a id="route-63"></a>

## POST `/api/workforce/result-processor-revisions/:id/fixture-runs`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:131](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L131)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:40](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L40)。

权限：`workforce:result-processor:author`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/resultprocessor/application.FixtureCommand`](wire-types.md#t-f979b627604d1fa0)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `harnessReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `evaluationContext` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `workspaceFiles` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `configOverrides` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `expectedDecision` | string | string | 零值可省略；Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`untyped nil` / `map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `runId` | string；Go `string` |
| `status` | string；Go `string` |

<a id="route-64"></a>

## POST `/api/workforce/result-processor-revisions/:id/publish`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:158](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L158)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:42](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L42)。

权限：`workforce:result-processor:publish`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.60f289d86afe`](wire-types.md#t-2f50884469a4850f)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `version` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.Release`](wire-types.md#t-aefd670a358cb98d)。

<a id="route-65"></a>

## POST `/api/workforce/result-processor-revisions/:id/validate`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:122](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L122)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:39](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L39)。

权限：`workforce:result-processor:author`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/domain.Validation`](wire-types.md#t-5986f44670d787cd)。

<a id="route-66"></a>

## GET `/api/workforce/result-processors`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:46](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L46)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L33)。

权限：`workforce:result-processor:list`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/resultprocessor/infrastructure.Processor`](wire-types.md#t-43d18e8c30c06b56)>。

<a id="route-67"></a>

## POST `/api/workforce/result-processors`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:55](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L55)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L34)。

权限：`workforce:result-processor:author`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/resultprocessor/application.CreateDraftCommand`](wire-types.md#t-c1f80d48eff67ece)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `category` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)；Go: `internal/app/workforce/resultprocessor/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/scriptasset/domain.SourceFile`](wire-types.md#t-17354ae5b0a2291f)；Go: `[]internal/app/workforce/resultprocessor/domain.SourceFile` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.Processor`](wire-types.md#t-43d18e8c30c06b56)。

<a id="route-68"></a>

## GET `/api/workforce/result-processors/:id`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:68](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L68)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L35)。

权限：`workforce:result-processor:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.Processor`](wire-types.md#t-43d18e8c30c06b56)。

<a id="route-69"></a>

## POST `/api/workforce/result-processors/:id/revisions`

实现：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:77](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L77)。注册：[seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go:36](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/controller/http.go#L36)。

权限：`workforce:result-processor:author`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/resultprocessor/infrastructure.Revision`](wire-types.md#t-14a66f1cbd666fcc)。

<a id="route-70"></a>

## POST `/api/workforce/review-operations/:id/reauthorizations`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:108](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L108)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:36](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L36)。

权限：`workforce:delivery:approve`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.a4cf61e687be`](wire-types.md#t-678f1c5438f3c6f7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-71"></a>

## POST `/api/workforce/review-operations/:id/reconcile`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:130](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L130)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L35)。

权限：`workforce:delivery:reconcile`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.ReviewReconciliationResult`](wire-types.md#t-e125ce21366e091f)。

<a id="route-72"></a>

## GET `/api/workforce/review-runs/:reviewRunId`

实现：[seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go:79](../../../seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go#L79)。注册：[seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go#L41)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `reviewRunId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/reviewrun/domain.RunView`](wire-types.md#t-2177d7b784d4451b)。

入口／同包辅助方法中可定位的 reasonCode：`INDEPENDENT_REVIEW_NOT_FOUND`, `INDEPENDENT_REVIEW_CONTEXT_UNAVAILABLE`, `INDEPENDENT_REVIEW_REASON_INVALID`, `INDEPENDENT_REVIEW_GUIDANCE_INVALID`, `INDEPENDENT_REVIEW_GUIDANCE_REQUIRED`, `INDEPENDENT_REVIEW_IDEMPOTENCY_CONFLICT`, `INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE`, `INDEPENDENT_REVIEW_STALE`, `INDEPENDENT_REVIEW_DETERMINISTIC_AUTHORITY_INVALID`, `INDEPENDENT_REVIEW_ACTION_NOT_ALLOWED`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-73"></a>

## POST `/api/workforce/review-runs/:reviewRunId/decisions`

实现：[seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go:106](../../../seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go#L106)。注册：[seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go:39](../../../seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go#L39)。

权限：`workforce:review:approve`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `X-Device-Id` | string |
| header | `X-Tenant-Id` | string |
| header | `Flow-Nonce` | string |
| header | `Proof-Token` | string |
| path | `reviewRunId` | string |

### 请求体

[`internal/app/workforce/reviewrun/controller.decisionRequest`](wire-types.md#t-629e091bbdeb27cd)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `action` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |
| `revisionGuidance` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/reviewrun/facade.HumanDecisionResult`](wire-types.md#t-7d46c6abb89ba2d3)。

入口／同包辅助方法中可定位的 reasonCode：`INDEPENDENT_REVIEW_NOT_FOUND`, `INDEPENDENT_REVIEW_REASON_INVALID`, `INDEPENDENT_REVIEW_GUIDANCE_INVALID`, `INDEPENDENT_REVIEW_CONTEXT_UNAVAILABLE`, `INDEPENDENT_REVIEW_GUIDANCE_REQUIRED`, `INDEPENDENT_REVIEW_IDEMPOTENCY_CONFLICT`, `INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE`, `INDEPENDENT_REVIEW_STALE`, `INDEPENDENT_REVIEW_DETERMINISTIC_AUTHORITY_INVALID`, `INDEPENDENT_REVIEW_ACTION_NOT_ALLOWED`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-74"></a>

## GET `/api/workforce/source-ingestion-runs/:id`

实现：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:62](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L62)。注册：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:29](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L29)。

权限：`workforce:source:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.IngestionRun`](wire-types.md#t-80cab33cc323b47c)。

<a id="route-75"></a>

## GET `/api/workforce/task-notifications/readiness`

实现：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:37](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L37)。注册：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:31](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L31)。

权限：`workforce:task:query`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/tasknotification/application.Readiness`](wire-types.md#t-23fae19b8d25418f)。

入口／同包辅助方法中可定位的 reasonCode：`TASK_NOTIFICATION_RECIPIENT_INVALID`, `TENANT_SCOPE_REQUIRED`, `TASK_NOTIFICATION_NOT_FOUND`, `TASK_NOTIFICATION_SUBSCRIPTION_CONFLICT`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-76"></a>

## GET `/api/workforce/task-turns/:turnId`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:135](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L135)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L33)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `turnId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`int` / [`internal/app/workforce/continuation/domain.Turn`](wire-types.md#t-504ca07e3c7de7c9)。

<a id="route-77"></a>

## GET `/api/workforce/tasks`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:589](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L589)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:151](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L151)。

权限：`workforce:task:list`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `query` | string |
| query | `view` | string |
| query | `limit` | string |
| query | `cursor` | string |
| query | `from` | string(RFC3339 或 YYYY-MM-DD) |
| query | `to` | string(RFC3339 或 YYYY-MM-DD) |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `items` | array<integer>；Go `[]internal/app/workforce/task/controller.taskResponse` |
| `nextCursor` | 任意 JSON 值；Go `any` |

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `TASK_LIST_FILTER_INVALID`, `DATABASE_ERROR`, `INTERACTION_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-78"></a>

## GET `/api/workforce/tasks/:id`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:670](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L670)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:153](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L153)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/task/controller.taskResponse`](wire-types.md#t-d866baf604f3d5b9)。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `INTERACTION_SUMMARY_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-79"></a>

## GET `/api/workforce/tasks/:id/artifacts`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:793](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L793)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:159](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L159)。

权限：`workforce:artifact:list`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/task/domain.Artifact`](wire-types.md#t-954080f0a094f86c)>。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `INVALID_ID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-80"></a>

## GET `/api/workforce/tasks/:id/artifacts/:artifactId/download`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:811](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L811)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:160](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L160)。

权限：`workforce:artifact:download`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |
| path | `artifactId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

成功为二进制文件流；错误在开始发送之前返回 HTTP 错误，开始之后可能中止流。请核对大小与 SHA256。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `INVALID_ID`, `ARTIFACT_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-81"></a>

## POST `/api/workforce/tasks/:id/cancel`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:701](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L701)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:156](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L156)。

权限：`workforce:task:cancel`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`inline.a4cf61e687be`](wire-types.md#t-678f1c5438f3c6f7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `task` | integer；Go `internal/app/workforce/task/controller.taskResponse` |
| `cancel` | integer；Go `internal/app/workforce/task/infrastructure.CancelResult` |

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `INVALID_JSON`, `INVALID_ID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-82"></a>

## GET `/api/workforce/tasks/:id/continuations`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:317](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L317)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:163](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L163)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/resultprocessor/facade.ContinuationCheckpointView`](wire-types.md#t-12df377b44feb960)>。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `RESULT_PROCESSOR_UNAVAILABLE`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-83"></a>

## GET `/api/workforce/tasks/:id/evaluations`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:299](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L299)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:162](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L162)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/resultprocessor/facade.EvaluationView`](wire-types.md#t-f50dc8464c810ee6)>。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `RESULT_PROCESSOR_UNAVAILABLE`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-84"></a>

## GET `/api/workforce/tasks/:id/events`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:728](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L728)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:157](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L157)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `after` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/task/domain.Event`](wire-types.md#t-2b0251d7dcf14955)>。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `INVALID_ID`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-85"></a>

## GET `/api/workforce/tasks/:id/events/stream`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:747](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L747)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:158](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L158)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `after` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

`text/event-stream`，见 [SSE 协议](sse.md)。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-86"></a>

## GET `/api/workforce/tasks/:id/finalization-runs`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:354](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L354)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:165](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L165)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

`data` 为后处理提供方的 finalization run 数组；提供方未配置时为 `[]`。入口以 `any` 声明，实际类型见实现，不保证静态统一 DTO。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-87"></a>

## GET `/api/workforce/tasks/:id/git-plan`

实现：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:40](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L40)。注册：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L33)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.GitPlanView`](wire-types.md#t-13e8fe72e31c31d9)。

<a id="route-88"></a>

## GET `/api/workforce/tasks/:id/independent-review`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:687](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L687)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:154](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L154)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`internal/app/workforce/reviewrun/facade.TaskReport`。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `INDEPENDENT_REVIEW_UNAVAILABLE`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-89"></a>

## GET `/api/workforce/tasks/:id/input`

实现：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L35)。注册：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L32)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.TaskInputView`](wire-types.md#t-83357101ce2099a0)。

<a id="route-90"></a>

## PATCH `/api/workforce/tasks/:id/metadata`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:550](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L550)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:155](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L155)。

权限：`workforce:task:metadata:update`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/task/application.MetadataRequest`](wire-types.md#t-5a2b56cbd1e127e7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `tags` | array<string> | array<string> | Go: `[]string` |
| `expectedMetadataVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/task/controller.taskResponse`](wire-types.md#t-d866baf604f3d5b9)。

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-91"></a>

## GET `/api/workforce/tasks/:id/model-invocations`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:90](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L90)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:52](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L52)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `limit` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/providerinvocation/domain.TaskView`](wire-types.md#t-7437168545b1a638)>。

<a id="route-92"></a>

## GET `/api/workforce/tasks/:id/recovery`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:335](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L335)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:161](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L161)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `taskId` | string；Go `string` |
| `status` | integer；Go `internal/app/workforce/task/domain.TaskStatus` |
| `execution` | integer 或 null；Go `*internal/app/workforce/task/domain.ExecutionView` |
| `cancelRequestId` | string；Go `string` |
| `cancelGeneration` | string；Go `string` |
| `cancelRequestedBy` | string；Go `string` |
| `cancelRequestedAt` | string；Go `string` |
| `cancelReason` | string；Go `string` |
| `finalizationRuns` | 任意 JSON 值；Go `any` |

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-93"></a>

## GET `/api/workforce/tasks/:id/result-outputs`

实现：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:281](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L281)。注册：[seven-agent-force-server/internal/app/workforce/task/controller/http.go:164](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L164)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/resultprocessor/facade.ResultOutputView`](wire-types.md#t-66337263924c2a96)>。

入口／同包辅助方法中可定位的 reasonCode：`NOT_FOUND`, `INVALID_STATE`, `INTERNAL_ERROR`, `RESULT_PROCESSOR_UNAVAILABLE`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-94"></a>

## GET `/api/workforce/tasks/:taskId/completion-check`

实现：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go:53](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go#L53)。注册：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go#L32)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/application.Summary`](wire-types.md#t-6f9904ce39e62bbb)。

<a id="route-95"></a>

## POST `/api/workforce/tasks/:taskId/completion-check-decisions`

实现：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go:66](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go#L66)。注册：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go#L33)。

权限：`workforce:task:continue`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

[`internal/app/workforce/completioncheck/application.Decision`](wire-types.md#t-f7cee4a5c35aaec2)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `action` | string | string | Go: `string` |
| `resultId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reason` | string | string | Go: `string` |
| `repairScope` | string | string | 零值可省略；Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/application.DecisionResult`](wire-types.md#t-4d15795a3963cf88)。

<a id="route-96"></a>

## GET `/api/workforce/tasks/:taskId/completion-check-revisions`

实现：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/revision.go:93](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/revision.go#L93)。注册：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go:36](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go#L36)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/facade.RevisionView`](wire-types.md#t-7bf43f955ad9986e)。

<a id="route-97"></a>

## POST `/api/workforce/tasks/:taskId/completion-check-revisions`

实现：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/revision.go:52](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/revision.go#L52)。注册：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go#L34)。

权限：`workforce:task:continue`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

`encoding/json.RawMessage`

类型：`encoding/json.RawMessage`。

[`internal/app/workforce/completioncheck/facade.RevisionRequest`](wire-types.md#t-86af92c22fc7f700)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | [`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/facade.Revision`](wire-types.md#t-970a7c1811e86335)。

<a id="route-98"></a>

## GET `/api/workforce/tasks/:taskId/completion-check-revisions/pending`

实现：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/revision.go:75](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/revision.go#L75)。注册：[seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/completioncheck/controller/http.go#L35)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/completioncheck/facade.Revision`](wire-types.md#t-970a7c1811e86335) 或 null。

<a id="route-99"></a>

## GET `/api/workforce/tasks/:taskId/conversation`

实现：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:149](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L149)。注册：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:31](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L31)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `cursor` | string |
| query | `limit` | string |
| query | `before` | string |
| query | `after` | string |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/interaction/domain.Conversation`](wire-types.md#t-e93f6f780725fb07)。

<a id="route-100"></a>

## GET `/api/workforce/tasks/:taskId/conversation/:itemId`

实现：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:187](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L187)。注册：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L33)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |
| path | `itemId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/interaction/domain.Item`](wire-types.md#t-f32fa9166a2bd4ba)。

<a id="route-101"></a>

## POST `/api/workforce/tasks/:taskId/conversation/read`

实现：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:248](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L248)。注册：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:36](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L36)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

[`inline.4ff1e598aa48`](wire-types.md#t-637fe0a8d5702fd1)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `watermark` | string(decimal) 或 null | string(decimal) 或 null | JSON 引号编码；Go: `*int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]bool`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `saved` | boolean；Go `bool` |

<a id="route-102"></a>

## GET `/api/workforce/tasks/:taskId/deliveries`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:39](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L39)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:27](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L27)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)>。

<a id="route-103"></a>

## POST `/api/workforce/tasks/:taskId/delivery-requests`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:51](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L51)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:28](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L28)。

权限：`workforce:delivery-target:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

[`inline.354b28be148f`](wire-types.md#t-e7dcd469f447bb70)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `action` | integer | integer | [`internal/app/workforce/delivery/domain.Action`](wire-types.md#t-0b32913a8ffe942c)；Go: `internal/app/workforce/delivery/domain.Action` |
| `resultCommit` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-104"></a>

## POST `/api/workforce/tasks/:taskId/delivery-requests/:requestId/decisions`

实现：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:81](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L81)。注册：[seven-agent-force-server/internal/app/workforce/delivery/controller/http.go:29](../../../seven-agent-force-server/internal/app/workforce/delivery/controller/http.go#L29)。

权限：`workforce:delivery:approve`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |
| path | `requestId` | string |

### 请求体

[`inline.560164efb36e`](wire-types.md#t-f5c24f61ad33639c)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `action` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/delivery/application.RequestView`](wire-types.md#t-8b3f058d77519df7)。

<a id="route-105"></a>

## GET `/api/workforce/tasks/:taskId/execution-operations/:operationId`

实现：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:38](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L38)。注册：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L32)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |
| path | `operationId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.OperationView`](wire-types.md#t-5a1a374de13328c7)。

<a id="route-106"></a>

## POST `/api/workforce/tasks/:taskId/execution-operations/:operationId/decisions`

实现：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:47](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L47)。注册：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L33)。

权限：`workforce:task:continue`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |
| path | `operationId` | string |

### 请求体

[`internal/app/workforce/executionaccess/facade.DecisionRequest`](wire-types.md#t-dcbfaad20ae4affa)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `action` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

<a id="route-107"></a>

## GET `/api/workforce/tasks/:taskId/independent-review/runs`

实现：[seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go:44](../../../seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go#L44)。注册：[seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go:40](../../../seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go#L40)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `cursor` | string |
| query | `limit` | string |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/reviewrun/domain.RunPage`](wire-types.md#t-26a153c3d3c98dac)。

入口／同包辅助方法中可定位的 reasonCode：`INDEPENDENT_REVIEW_NOT_FOUND`, `INVALID_CURSOR`, `INVALID_LIMIT`, `INDEPENDENT_REVIEW_CONTEXT_UNAVAILABLE`, `INDEPENDENT_REVIEW_REASON_INVALID`, `INDEPENDENT_REVIEW_GUIDANCE_INVALID`, `INDEPENDENT_REVIEW_GUIDANCE_REQUIRED`, `INDEPENDENT_REVIEW_IDEMPOTENCY_CONFLICT`, `INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE`, `INDEPENDENT_REVIEW_STALE`, `INDEPENDENT_REVIEW_DETERMINISTIC_AUTHORITY_INVALID`, `INDEPENDENT_REVIEW_ACTION_NOT_ALLOWED`, `INTERNAL_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-108"></a>

## GET `/api/workforce/tasks/:taskId/interactions`

实现：[seven-agent-force-server/internal/app/workforce/interaction/controller/questions.go:11](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/questions.go#L11)。注册：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L32)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `cursor` | string |
| query | `limit` | string |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/interaction/facade.QuestionPage`](wire-types.md#t-907dc4b4ec578277)。

<a id="route-109"></a>

## POST `/api/workforce/tasks/:taskId/interactions/:interactionId/responses`

实现：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:205](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L205)。注册：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L34)。

权限：`workforce:task:continue`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |
| path | `interactionId` | string |

### 请求体

[`internal/app/workforce/interaction/facade.ResponseRequest`](wire-types.md#t-151c76a6558793b8)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `answers` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Answer`](wire-types.md#t-b482691f81865689)；Go: `[]internal/app/workforce/interaction/domain.Answer` |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/interaction/facade.Receipt`](wire-types.md#t-72ef9cc8b517430e)。

<a id="route-110"></a>

## POST `/api/workforce/tasks/:taskId/messages`

实现：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:229](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L229)。注册：[seven-agent-force-server/internal/app/workforce/interaction/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go#L35)。

权限：`workforce:task:continue`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

[`internal/app/workforce/interaction/facade.MessageRequest`](wire-types.md#t-8a288654937fb4f8)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `text` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/interaction/facade.Receipt`](wire-types.md#t-72ef9cc8b517430e)。

<a id="route-111"></a>

## GET `/api/workforce/tasks/:taskId/network-grants`

实现：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:64](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L64)。注册：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L34)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/executionaccess/facade.GrantView`](wire-types.md#t-6ba560fbbed25f2c)>。

<a id="route-112"></a>

## POST `/api/workforce/tasks/:taskId/network-grants/:grantId/revoke`

实现：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:73](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L73)。注册：[seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go:35](../../../seven-agent-force-server/internal/app/workforce/executionaccess/controller/http.go#L35)。

权限：`workforce:task:continue`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |
| path | `grantId` | string |

### 请求体

[`internal/app/workforce/executionaccess/facade.RevokeRequest`](wire-types.md#t-a97139764dd69d1f)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)。

<a id="route-113"></a>

## GET `/api/workforce/tasks/:taskId/notification-records`

实现：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:93](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L93)。注册：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:34](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L34)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/tasknotification/application.RecordView`](wire-types.md#t-fadf230ce1eeaaff)>。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `TASK_NOTIFICATION_RECIPIENT_INVALID`, `TASK_NOTIFICATION_FACT_INVALID`, `TASK_NOTIFICATION_NOT_FOUND`, `TASK_NOTIFICATION_SUBSCRIPTION_CONFLICT`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-114"></a>

## GET `/api/workforce/tasks/:taskId/notification-subscription`

实现：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:52](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L52)。注册：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L32)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/tasknotification/domain.Subscription`](wire-types.md#t-7bf91db006b3fa53)。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `TASK_NOTIFICATION_RECIPIENT_INVALID`, `TASK_NOTIFICATION_FACT_INVALID`, `TASK_NOTIFICATION_NOT_FOUND`, `TASK_NOTIFICATION_SUBSCRIPTION_CONFLICT`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-115"></a>

## PUT `/api/workforce/tasks/:taskId/notification-subscription`

实现：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:61](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L61)。注册：[seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/tasknotification/controller/http.go#L33)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

[`inline.21e068903bf9`](wire-types.md#t-7b2b3f54f8647549)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `enabled` | boolean | boolean | Go: `bool` |
| `events` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.FactType`](wire-types.md#t-26bedcb14f61a59b)；Go: `[]internal/app/workforce/tasknotification/domain.FactType` |
| `channels` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.ChannelSelection`](wire-types.md#t-5e9beaea2948de73)；Go: `[]internal/app/workforce/tasknotification/domain.ChannelSelection` |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/tasknotification/domain.Subscription`](wire-types.md#t-7bf91db006b3fa53)。

入口／同包辅助方法中可定位的 reasonCode：`TENANT_SCOPE_REQUIRED`, `TASK_NOTIFICATION_RECIPIENT_INVALID`, `TASK_NOTIFICATION_FACT_INVALID`, `REQUEST_TOO_LARGE`, `INVALID_JSON`, `TASK_NOTIFICATION_NOT_FOUND`, `TASK_NOTIFICATION_SUBSCRIPTION_CONFLICT`, `DATABASE_ERROR`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-116"></a>

## GET `/api/workforce/tasks/:taskId/result-materializations`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:66](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L66)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:37](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L37)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`int` / array<[`internal/app/workforce/continuation/domain.MaterializationView`](wire-types.md#t-e10445ca6ec9a028)>。

<a id="route-117"></a>

## POST `/api/workforce/tasks/:taskId/result-materializations`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:42](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L42)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:36](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L36)。

权限：`workforce:result-output:materialize`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

[`internal/app/workforce/continuation/application.CreateMaterializationRequest`](wire-types.md#t-d645a0c43f5ed65f)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `outputId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`int` / [`internal/app/workforce/continuation/domain.MaterializationView`](wire-types.md#t-e10445ca6ec9a028)。

<a id="route-118"></a>

## GET `/api/workforce/tasks/:taskId/turns`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:123](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L123)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:31](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L31)。

权限：`workforce:task:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`int` / array<[`internal/app/workforce/continuation/domain.Turn`](wire-types.md#t-504ca07e3c7de7c9)>。

<a id="route-119"></a>

## POST `/api/workforce/tasks/:taskId/turns`

实现：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:89](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L89)。注册：[seven-agent-force-server/internal/app/workforce/continuation/controller/http.go:32](../../../seven-agent-force-server/internal/app/workforce/continuation/controller/http.go#L32)。

权限：`workforce:task:continue`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `taskId` | string |

### 请求体

[`internal/app/workforce/continuation/application.CreateNextTurnRequest`](wire-types.md#t-d13a95e1451be261)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceCheckpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `expectedCurrentTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instruction` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`int` / [`internal/app/workforce/continuation/domain.Turn`](wire-types.md#t-504ca07e3c7de7c9)。

<a id="route-120"></a>

## GET `/api/workforce/tasks/analytics`

实现：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:89](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L89)。注册：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:29](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L29)。

权限：`workforce:task:list`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `range` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/taskanalytics/application.TaskAnalytics`](wire-types.md#t-78953998aaecf7d5)。

<a id="route-121"></a>

## GET `/api/workforce/workbench`

实现：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L33)。注册：[seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go:30](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/controller/http.go#L30)。

权限：`workforce:task:list`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `range` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/taskanalytics/application.Workbench`](wire-types.md#t-e1913dc24067b81a)。

<a id="route-122"></a>

## GET `/api/workforce/workers`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:86](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L86)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:37](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L37)。

权限：`workforce:worker:list`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/workerregistry/domain.Worker`](wire-types.md#t-d7835ff39836369f)>。

入口／同包辅助方法中可定位的 reasonCode：`WORKER_LIST_FAILED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-123"></a>

## POST `/api/workforce/workers`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:69](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L69)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:36](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L36)。

权限：`workforce:worker:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/workerregistry/controller.createRequest`](wire-types.md#t-75871c0454c97f97)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `maxActiveAttempts` | integer | integer | Go: `int` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `worker` | integer；Go `internal/app/workforce/workerregistry/domain.Worker` |
| `credential` | integer；Go `internal/app/workforce/workerregistry/application.Credential` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `INVALID_WORKER`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-124"></a>

## GET `/api/workforce/workers/:id`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:117](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L117)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:39](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L39)。

权限：`workforce:worker:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `worker` | integer；Go `internal/app/workforce/workerregistry/domain.Worker` |
| `images` | array<integer>；Go `[]internal/app/workforce/workerregistry/domain.Image` |
| `queue` | integer；Go `internal/app/workforce/workerregistry/domain.QueueContract` |
| `reconciliation` | integer；Go `internal/app/workforce/workerregistry/domain.Reconciliation` |

入口／同包辅助方法中可定位的 reasonCode：`WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-125"></a>

## GET `/api/workforce/workers/:id/assignments`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:204](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L204)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:46](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L46)。

权限：`workforce:worker:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：array<[`internal/app/workforce/workerregistry/domain.Assignment`](wire-types.md#t-13121e6a2f024643)>。

入口／同包辅助方法中可定位的 reasonCode：`WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-126"></a>

## PUT `/api/workforce/workers/:id/capacity`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:153](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L153)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:42](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L42)。

权限：`workforce:worker:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/workerregistry/controller.capacityRequest`](wire-types.md#t-f447032854da6cec)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `worker` | integer；Go `internal/app/workforce/workerregistry/domain.Worker` |
| `images` | array<integer>；Go `[]internal/app/workforce/workerregistry/domain.Image` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-127"></a>

## POST `/api/workforce/workers/:id/decommission`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:192](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L192)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:45](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L45)。

权限：`workforce:worker:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/workerregistry/controller.decommissionRequest`](wire-types.md#t-75f9438cae55fa76)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `decommissioned` | boolean；Go `bool` |
| `purgedMessages` | integer；Go `int` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-128"></a>

## PUT `/api/workforce/workers/:id/desired-state`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:135](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L135)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L41)。

权限：`workforce:worker:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

[`internal/app/workforce/workerregistry/controller.desiredRequest`](wire-types.md#t-07d2399bfaeacd7f)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `desiredState` | string | string | Go: `string` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `worker` | integer；Go `internal/app/workforce/workerregistry/domain.Worker` |
| `images` | array<integer>；Go `[]internal/app/workforce/workerregistry/domain.Image` |

入口／同包辅助方法中可定位的 reasonCode：`REQUEST_TOO_LARGE`, `INVALID_JSON`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-129"></a>

## POST `/api/workforce/workers/:id/image-inventory-refreshes`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:179](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L179)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:44](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L44)。

权限：`workforce:worker:operate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `refreshInventoryGeneration` | string(decimal)；Go `int64` |

入口／同包辅助方法中可定位的 reasonCode：`WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-130"></a>

## GET `/api/workforce/workers/:id/metrics`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:97](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L97)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:40](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L40)。

权限：`workforce:worker:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `range` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workerregistry/domain.MetricSeries`](wire-types.md#t-f6b5cf4af9913eb8)。

入口／同包辅助方法中可定位的 reasonCode：`WORKER_NOT_FOUND`, `WORKER_METRIC_RANGE_INVALID`, `WORKER_UNAUTHORIZED`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-131"></a>

## POST `/api/workforce/workers/:id/token-rotations`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:170](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L170)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:43](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L43)。

权限：`workforce:worker:token:rotate`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `credential` | integer；Go `internal/app/workforce/workerregistry/application.Credential` |

入口／同包辅助方法中可定位的 reasonCode：`WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-132"></a>

## GET `/api/workforce/workers/metrics`

实现：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:94](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L94)。注册：[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:38](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L38)。

权限：`workforce:worker:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| query | `range` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workerregistry/domain.MetricSeries`](wire-types.md#t-f6b5cf4af9913eb8)。

入口／同包辅助方法中可定位的 reasonCode：`WORKER_METRIC_RANGE_INVALID`, `WORKER_UNAUTHORIZED`, `WORKER_NOT_FOUND`, `STALE_WORKER_INSTANCE`, `WORKER_NOT_DRAINED`, `WORKER_HAS_ACTIVE_PROVIDER_RUNTIME`, `WORKER_CAPACITY_EXCEEDS_REPORTED_LIMIT`, `WORKER_CAPACITY_INVALID`, `WORKER_STATE_CONFLICT`, `WORKER_REQUEST_REJECTED`。这不是跨层完整枚举；重试前读 [公共错误约定](common.md)。

<a id="route-133"></a>

## GET `/api/workforce/workspace-source-revisions/:id`

实现：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:57](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L57)。注册：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:28](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L28)。

权限：`workforce:source:query`。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/workspaceinput/application.SourceRevision`](wire-types.md#t-3d5781a76d6bd975)。

<a id="route-134"></a>

## POST `/api/workforce/workspace-sources/uploads`

实现：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:45](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L45)。注册：[seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go:27](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/controller/http.go#L27)。

权限：`workforce:source:create`。

路径／查询／表单：未声明额外参数；鉴权 Header 见指南。

### 请求体

[`internal/app/workforce/workspaceinput/application.UploadSourceRequest`](wire-types.md#t-204186287995edce)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `fileId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceKind` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：`map[string]any`。

直接构造的 `data` 字段：

| 字段 | Go 类型与 HTTP 传输类型 |
| --- | --- |
| `sourceRevision` | integer；Go `internal/app/workforce/workspaceinput/application.SourceRevision` |
| `ingestionRun` | integer；Go `internal/app/workforce/workspaceinput/application.IngestionRun` |
