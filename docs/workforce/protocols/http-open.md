# Open REST 方法参考

来源：随附独立源码快照（结构经检查）。共 **7** 个方法。外部系统使用 Application API Key。接入步骤、必填与约束见 [Open REST 指南](open-rest.md)。

此页逐项列出方法、URL、路径／查询／Header／表单参数、请求体字段、响应类型和实现位置。嵌套对象点进类型索引；`omitempty` 仅是编码规则，业务必填与状态前置条件由对应应用／域校验决定。各接口并不统一采用 PATCH 或自动幂等。

| 方法 | 路径 | Handler |
| --- | --- | --- |
| GET | [`/api/open/workforce/v1/capabilities`](#route-1) | `getOpenCapabilities` |
| GET | [`/api/open/workforce/v1/projects/:projectId/model-options`](#route-2) | `listModelOptions` |
| GET | [`/api/open/workforce/v1/projects/:projectId/runtime-options`](#route-3) | `listRuntimeOptions` |
| POST | [`/api/open/workforce/v1/projects/:projectId/tasks`](#route-4) | `createOpenTask` |
| GET | [`/api/open/workforce/v1/tasks/:id`](#route-5) | `getOpenTask` |
| GET | [`/api/open/workforce/v1/tasks/:id/outputs`](#route-6) | `listOpenTaskOutputs` |
| GET | [`/api/open/workforce/v1/tasks/:id/outputs/:outputId/download`](#route-7) | `downloadOpenTaskOutput` |

<a id="route-1"></a>

## GET `/api/open/workforce/v1/capabilities`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:53](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L53)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:44](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L44)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/domain.OpenCapabilitiesView`](wire-types.md#t-b6ca673c5d930605)。

<a id="route-2"></a>

## GET `/api/open/workforce/v1/projects/:projectId/model-options`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:477](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L477)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:46](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L46)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `projectId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/domain.ModelOptions`](wire-types.md#t-9ac1da81b7add025)。

<a id="route-3"></a>

## GET `/api/open/workforce/v1/projects/:projectId/runtime-options`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:495](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L495)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:47](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L47)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| query | `modelCredentialId` | string |
| path | `projectId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/domain.RuntimeOptions`](wire-types.md#t-1e4182a0d7935aca)。

<a id="route-4"></a>

## POST `/api/open/workforce/v1/projects/:projectId/tasks`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:62](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L62)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:45](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L45)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| header | `Idempotency-Key` | string |
| path | `projectId` | string |

### 请求体

[`internal/app/workforce/openintegration/domain.CreateOpenTaskRequest`](wire-types.md#t-a9f026f9297558d7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | 零值可省略；Go: `string` |
| `instruction` | string | string | Go: `string` |
| `tags` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `referenceAssetIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `additionalCompletionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewEnabled` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `deliveryPreference` | string | string | 零值可省略；Go: `string` |
| `modelSelection` | integer | integer | [`internal/app/workforce/task/facade.ModelSelection`](wire-types.md#t-c395a518b87a2f05)；Go: `internal/app/workforce/task/facade.ModelSelection` |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionTimeoutSeconds` | integer | integer | 零值可省略；Go: `int` |
| `externalCorrelationId` | string | string | Go: `string` |


### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/domain.OpenTaskView`](wire-types.md#t-834edc0113f0df5e)。

<a id="route-5"></a>

## GET `/api/open/workforce/v1/tasks/:id`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:101](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L101)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:48](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L48)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/domain.OpenTaskView`](wire-types.md#t-834edc0113f0df5e)。

<a id="route-6"></a>

## GET `/api/open/workforce/v1/tasks/:id/outputs`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:120](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L120)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:49](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L49)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `id` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

默认包装为 `Result.data`；`data` 类型：[`internal/app/workforce/openintegration/domain.OpenTaskOutputsView`](wire-types.md#t-927ddc5de2f63f68)。

<a id="route-7"></a>

## GET `/api/open/workforce/v1/tasks/:id/outputs/:outputId/download`

实现：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:139](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L139)。注册：[seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go:50](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go#L50)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| path | `id` | string |
| path | `outputId` | string |

### 请求体

没有已声明的业务 JSON DTO；按照实现使用空请求体。状态操作仍需满足当前业务状态及权限。

### 响应

成功为二进制文件流；错误在开始发送之前返回 HTTP 错误，开始之后可能中止流。请核对大小与 SHA256。
