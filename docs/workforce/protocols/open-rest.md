# Open REST 外部接入指南

协议前缀 `/api/open/workforce/v1`，HTTP + UTF-8 JSON，Application API Key 每次请求携带。功能开关 `workforce.openIntegration.enabled` 默认 false；部署方需明确启用并提供已授权 Project ID，当前没有公开的 Project 列表接口。

## 方法

| HTTP | 方法／路径 | 参数 | 返回 `data` |
| --- | --- | --- | --- |
| GET | `/capabilities` | 无业务参数 | OpenCapabilitiesView |
| GET | `/projects/:projectId/model-options` | projectId:string(ID) | ModelOptions，items 含 credentialId/model/protocol/label |
| GET | `/projects/:projectId/runtime-options` | projectId:string(ID)，query modelCredentialId:string(ID)，必传 | RuntimeOptions，items 含 runtimeReleaseId/name/recommended |
| POST | `/projects/:projectId/tasks` | projectId:string(ID)，Idempotency-Key:string，JSON 见下表 | 202 OpenTaskView，含 replayed |
| GET | `/tasks/:id` | id:string(Task ID) | OpenTaskView |
| GET | `/tasks/:id/outputs` | id:string(Task ID) | OpenTaskOutputsView |
| GET | `/tasks/:id/outputs/:outputId/download` | 两个 string(ID) | 原始二进制 body |

[逐方法字段与所有嵌套类型](http-open.md)。所有 ID 是无前导零、无符号、无空白的正十进制 string；上限为 signed int64。modelCredentialId 是凭据记录引用，不是 Provider API Key。

## 创建任务请求

Header：`Authorization: Bearer <APPLICATION_API_KEY>`、`Content-Type: application/json`、`Idempotency-Key: <CLIENT_KEY>`。请求 body 最大 **32 KiB**；未知字段、多段 JSON、非法 UTF-8 拒绝。

| 参数 | JSON 类型 | 必填／默认 | 约束与用途 |
| --- | --- | --- | --- |
| `instruction` | string | 必填 | trim 后 1–20000 Unicode 字符；仍受总 body 限制 |
| `modelSelection` | object | 必填 | 下面两个子字段 |
| `modelSelection.credentialId` | string(ID) | 必填 | 选择目录返回的凭据引用 |
| `modelSelection.model` | string | 必填 | trim 后非空，≤256 bytes，无控制字符 |
| `runtimeReleaseId` | string(ID) | 必填 | 已发布兼容 Runtime；创建时再次校验 |
| `externalCorrelationId` | string | 必填 | trim 后 1–128 字符，外部业务关联 |
| `name` | string | 可省略 | trim 后 ≤120 字符 |
| `tags` | string[] | 可省略 | ≤20 个，trim 后每项 1–64 字符且不重复 |
| `referenceAssetIds` | string(ID)[] | 可省略 | ≤50 项，资源需获授权 |
| `additionalCompletionCheck` | object | 可省略 | 追加完成检查；字段见 completioncheck.Input 类型索引 |
| `reviewEnabled` | boolean | 可省略 | 复用普通 Project task 的 Review 默认与校验 |
| `deliveryPreference` | string enum | PROJECT_DEFAULT | PROJECT_DEFAULT / ASK_AFTER_GATES / SAVE_ONLY |
| `executionTimeoutSeconds` | integer | 0 | 0 不加此执行时限；1–2147483647 设置秒数 |

示例只含示意 ID 与模型名；不要直接复制为真实配置：

```json
{
  "instruction":"生成本项目的开发说明。",
  "modelSelection":{"credentialId":"123","model":"configured-model-id"},
  "runtimeReleaseId":"456",
  "externalCorrelationId":"external-order-20261006-001",
  "deliveryPreference":"SAVE_ONLY"
}
```

模型目录选择模型与 credential → 根据相同 Project 和 credential 查询 Runtime → 提交任务 → 保存 taskId → 轮询 GET task → 列出 outputs → 下载并核对 sizeBytes 与 SHA256。推荐 Runtime 不会预留资源，提交时可能因版本、权限、兼容性变化被拒绝。**相同 Application、相同 key、相同规范请求返回原任务；相同 key 不同请求返回 409 IDEMPOTENCY_CONFLICT。** key trim 后非空且≤128 字符，不能拿 externalCorrelationId 代替幂等 key。

## 公共状态和输出

公共 Task schema `seven.workforce.open-task/v2`，capabilities `seven.workforce.open-capabilities/v2`，输出列表 `seven.workforce.open-task-outputs/v1`。详细 Task 字段在方法参考；`links.operator` 供有用户登录权限的人打开。

| 内部阶段 | 公共 status |
| --- | --- |
| CREATED / QUEUED / RETRY_WAIT | QUEUED |
| PREPARING / RUNNING / VALIDATING / PACKAGING / UPLOADING / CANCELLING / REVIEWING | RUNNING |
| INTERACTION_SUSPENDED / MANUAL_REVIEW | ACTION_REQUIRED（reasonCode 解释原因） |
| SUCCEEDED / CANCELLED / TIMED_OUT | 同名终态 |
| 其他失败状态 | 按 PublicStatus 映射返回 FAILED |

ACTION_REQUIRED 需要通过 operator 页面继续处理。当前 Open 和 MCP 没有回答、补充、取消、Review 决定、Git 授权方法。Task 执行状态不等于 Git 交付状态；不要仅靠 SUCCEEDED 宣称已 push/MR。

outputs 最多 64 项，只暴露已 AVAILABLE 的 IMMEDIATE 结果与允许的 Artifact 结果；不触发 ON_DEMAND materialization。`outputId`、`sizeBytes` 都是 decimal string。下载验证实际 bytes 与已声明 SHA256；HTTP 200 本身不是完整文件校验。MCP 仅读取有限文本，REST 可以下载二进制。

## 稳定错误

| HTTP | reasonCode |
| --- | --- |
| 400 | IDEMPOTENCY_KEY_REQUIRED、INVALID_REQUEST、TASK_RUNTIME_SELECTION_REQUIRED |
| 401 | UNAUTHORIZED、APPLICATION_DISABLED |
| 404 | PROJECT_NOT_FOUND、TASK_NOT_FOUND、OUTPUT_NOT_FOUND |
| 409 | PROJECT_ARCHIVED、PROJECT_SOURCE_NOT_READY、PROJECT_CONFIGURATION_INVALID、IDEMPOTENCY_CONFLICT、TASK_RUNTIME_SELECTION_UNAVAILABLE、TASK_RUNTIME_SELECTION_INCOMPATIBLE |
| 503 | OUTPUT_UNAVAILABLE、SELECTION_UNAVAILABLE |
| 500 | INTERNAL_ERROR |

入口还可拒绝 Content-Type、非法 JSON、body 超限或限流。错误保留 Framework 数值 code 与 `data.reasonCode`，参见 [公共约定](common.md)。幂等重放与轮询不会跳过权限校验；Key 轮换／应用停用后不能凭旧 Task ID 绕过身份。

源码：[路由](../../../seven-agent-force-server/internal/app/workforce/openintegration/controller/http.go)、[请求 DTO 与校验](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/task_submission.go)、[错误映射](../../../seven-agent-force-server/internal/app/workforce/openintegration/application/service.go)。
