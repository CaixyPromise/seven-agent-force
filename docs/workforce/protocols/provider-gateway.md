# Provider Invocation 与模型协议

Workforce当前支持 configured OPENAI / ANTHROPIC 凭据协议；Runtime向平台网关转发，对应 Responses 或 Messages。凭据由平台注入受控proxy边界，Attempt请求不携带真实Provider Key。网关仅公开以下内部操作，不代表完整支持所有OpenAI/Anthropic产品接口。

## 四个HTTP方法

| POST路径 | 对应上游操作 | body／返回 |
| --- | --- | --- |
| /api/internal/workforce/v1/model-invocations/responses | Responses create | 原始JSON；JSON或SSE响应 |
| /api/internal/workforce/v1/model-invocations/responses/compact | Responses compact | 原始JSON与响应 |
| /api/internal/workforce/v1/model-invocations/messages | Messages create | 原始JSON；JSON或SSE响应 |
| /api/internal/workforce/v1/model-invocations/messages/count_tokens | Messages count tokens | 原始JSON与响应 |

字段如model:string、stream:boolean、input/messages等按各固定操作的上游协议透传，网关不声明覆盖上游所有参数。Body原始bytes参与digest。当前没有Workforce自己的ChatCompletions、embedding、image生成REST入口。选择目录返回protocol只表示执行模型协议能力，具体model须来自已配置目录。

## 所有权Header参数

| Header | 类型／必填条件 |
| --- | --- |
| Authorization | Bearer Worker credential，必填 |
| X-Seven-Execution-Kind | string，AGENT_ATTEMPT默认；或REVIEW_RUN |
| X-Seven-Worker-ID / X-Seven-Task-ID / X-Seven-Owner-Epoch | 正decimal string，必填 |
| X-Seven-Worker-Instance / X-Seven-Host-Boot-ID | string，必填当前机器身份 |
| X-Seven-Timestamp | string(RFC3339Nano)，必填 |
| X-Seven-Invocation-ID / X-Seven-Nonce | string，调用幂等／重放标识 |
| X-Seven-Body-SHA256 | string，body摘要；可带sha256:前缀 |
| X-Seven-Attempt-ID / X-Seven-Generation / X-Seven-Attempt-Lease | string，AGENT_ATTEMPT所需 |
| X-Seven-Review-Run-ID / X-Seven-Review-Run-Attempt-ID / X-Seven-Review-Run-Lease | string，REVIEW_RUN所需 |
| X-Seven-Review-Contract | string，Review structured output contract标识 |

Review身份不能混入ordinaryAttempt三个Header。网关验证Worker、冻结模型快照、租约、时间、nonce、digest与执行kind，不由客户端自由选Provider凭据。方法级参考见 [HTTP Provider](http-provider.md)。

成功返回上游status和BodyStream，保留筛选后的Header，并添加X-Seven-Invocation-ID:string；不套Framework成功包装。应用层拒绝写标准错误，reasonCode包含WORKER_UNAUTHORIZED、PROVIDER_INVOCATION_IDENTITY_INVALID、PROVIDER_EXECUTION_KIND_INVALID、PROVIDER_TIMESTAMP_INVALID、PROVIDER_REVIEW_RUN_IDENTITY_MIXED等。不能保存／输出raw Provider body作为日志，排查使用Invocation标识与安全错误类别。

源码：[网关](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/controller/http.go)、[模型调用域](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go)、[proxy](../../../seven-agent-force-server/cmd/seven-provider-proxy/main.go)。
