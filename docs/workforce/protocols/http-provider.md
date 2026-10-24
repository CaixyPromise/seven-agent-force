# Provider Invocation HTTP 方法参考

来源：随附独立源码快照（结构经检查）。共 **4** 个方法。只供受租约约束的 Agent Attempt／ReviewRun。详见 [模型网关指南](provider-gateway.md)。

此页逐项列出方法、URL、路径／查询／Header／表单参数、请求体字段、响应类型和实现位置。嵌套对象点进类型索引；`omitempty` 仅是编码规则，业务必填与状态前置条件由对应应用／域校验决定。各接口并不统一采用 PATCH 或自动幂等。

| 方法 | 路径 | Handler |
| --- | --- | --- |
| POST | [`/api/internal/workforce/v1/model-invocations/messages`](#route-1) | `invoke` |
| POST | [`/api/internal/workforce/v1/model-invocations/messages/count_tokens`](#route-2) | `invoke` |
| POST | [`/api/internal/workforce/v1/model-invocations/responses`](#route-3) | `invoke` |
| POST | [`/api/internal/workforce/v1/model-invocations/responses/compact`](#route-4) | `invoke` |

<a id="route-1"></a>

## POST `/api/internal/workforce/v1/model-invocations/messages`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:139](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L139)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:42](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L42)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| header | `X-Seven-Execution-Kind` | string |
| header | `X-Seven-Timestamp` | string |
| header | `Accept` | string |
| header | `anthropic-version` | string |
| header | `anthropic-beta` | string |

### 请求体

原始 Provider JSON object（根据 URL 对应 Responses 或 Messages 操作），网关按字节计算 SHA256；字段兼容性由已配置 Provider 决定。

### 响应

原始上游 HTTP status、JSON 或 SSE body；成功路径不套 `{code,data,message}`。网关拒绝／失败格式见指南。

<a id="route-2"></a>

## POST `/api/internal/workforce/v1/model-invocations/messages/count_tokens`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:139](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L139)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:43](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L43)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| header | `X-Seven-Execution-Kind` | string |
| header | `X-Seven-Timestamp` | string |
| header | `Accept` | string |
| header | `anthropic-version` | string |
| header | `anthropic-beta` | string |

### 请求体

原始 Provider JSON object（根据 URL 对应 Responses 或 Messages 操作），网关按字节计算 SHA256；字段兼容性由已配置 Provider 决定。

### 响应

原始上游 HTTP status、JSON 或 SSE body；成功路径不套 `{code,data,message}`。网关拒绝／失败格式见指南。

<a id="route-3"></a>

## POST `/api/internal/workforce/v1/model-invocations/responses`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:139](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L139)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:40](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L40)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| header | `X-Seven-Execution-Kind` | string |
| header | `X-Seven-Timestamp` | string |
| header | `Accept` | string |
| header | `anthropic-version` | string |
| header | `anthropic-beta` | string |

### 请求体

原始 Provider JSON object（根据 URL 对应 Responses 或 Messages 操作），网关按字节计算 SHA256；字段兼容性由已配置 Provider 决定。

### 响应

原始上游 HTTP status、JSON 或 SSE body；成功路径不套 `{code,data,message}`。网关拒绝／失败格式见指南。

<a id="route-4"></a>

## POST `/api/internal/workforce/v1/model-invocations/responses/compact`

实现：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:139](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L139)。注册：[seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go:41](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go#L41)。

鉴权：遵循本页协议身份与公共约定；本方法未提取到字面 RBAC 权限。

| 位置 | 参数 | 传输类型 |
| --- | --- | --- |
| header | `Authorization` | string |
| header | `X-Seven-Execution-Kind` | string |
| header | `X-Seven-Timestamp` | string |
| header | `Accept` | string |
| header | `anthropic-version` | string |
| header | `anthropic-beta` | string |

### 请求体

原始 Provider JSON object（根据 URL 对应 Responses 或 Messages 操作），网关按字节计算 SHA256；字段兼容性由已配置 Provider 决定。

### 响应

原始上游 HTTP status、JSON 或 SSE body；成功路径不套 `{code,data,message}`。网关拒绝／失败格式见指南。
