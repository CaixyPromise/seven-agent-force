# Supervisor 控制、journal 与 Worker Image 协议

Attempt 容器不开放 HTTP/gRPC 服务。Worker 在已授权容器内经 Unix Domain Socket 调用 Supervisor，传输为 UTF-8 单行 JSON。控制方法执行前仍验证 task/attempt/generation/ownerEpoch、permit、nonce、lease 等当前 authority。

## 请求与 framing

one-shot：attemptControlRequest JSON + newline；stream：`{id:string,request:attemptControlRequest}` + newline，返回带同一 id 的关联响应。id 是1–64字符 correlation，不是数据库ID；单请求上限56 KiB，stream framing/request/response上限64 KiB，最多8个并行工作槽。状态 JSON 标识 `seven.supervisor-control/v2`。

请求公共字段 schemaVersion:integer、command:string；其余参数必须按命令提供，不把全部 optional 字段同时传入。完整 request 字段：[`cmd/seven-worker-cli.attemptControlRequest`](wire-types.md#t-9bb4c67471d2b20b)。

## 控制方法

| command | 主要参数（除 schemaVersion/command） | 回执／用途 |
| --- | --- | --- |
| status / state / diagnostics / evaluation-state | 当前 socket 内上下文 | {ok:boolean,command:string,state:object} |
| interrupt | taskId/attemptId/generation/ownerEpoch:string | 身份一致才cancel；ok:boolean或reasonCode:string |
| start-harness | 当前身份；runPermitId:string、runPermitToken/runNonce/expiresAt:string | {ok,command,harnessStarted:object}；需要有效运行许可 |
| confirm-running | 当前身份、run permit 绑定信息 | {ok,command} |
| adopt-owner | 当前身份、previousOwnerEpoch:string、reattachPermitId:string、reattachPermitToken/leaseExpiresAt/leaseReceiptDigest:string | {ok,command,reattachAck:object} |
| renew-authority | 当前身份、租约与owner epoch相关字段 | {ok,command} |
| run-evaluation | cycleId:string、cycleNumber:integer、materialization:JSON，加当前身份 | {ok,command,candidate:object} |
| continue-evaluation | cycleId:string、directive/decision/message/managedFeedback/resumeNonce:string，当前身份 | {ok,command} |
| hook-event | eventType/hookEvent/invocationId/configurationDigest/hookName/packageVersion/packageDigest/reasonCode/diagnostic:string、packageReleaseId:string、durationMs:integer、当前身份 | {ok,command,invocationId:string} |
| notifications | after:string(int64)，当前 round/identity | 本轮 Agent notification page；不是业务状态可信来源 |
| notifications-received | through:string(int64)，当前 round/identity | 确认 notification 已接收 |
| agent-control | method:string、payload:object、currentRound:string、sessionGeneration:string，当前identity | 向Adapter发控制；写入成功不代表收到执行回执 |
| interaction-ask / interaction-capture | payload:object、当前 round/identity | image CLI提问与捕获；依据实现读取关联回执 |
| interaction-suspend-commit / interaction-suspend-abort | currentRound:string、sessionGeneration:string、当前identity | 完成／中止冷挂起提交 |

上表给出分组用途；请求类型表声明全部字段，**命令的必填／状态校验以同名 dispatcher 方法为准**，不是每个命令都接受整个 request 的任意组合。失败 `{ok:false,reasonCode?:string,error?:string}`；ok:true 仅该控制方法被接受，业务终态由 Server 核验。

## Journal、notification 与 image probe

journal 单行记录：schemaVersion:integer、sequence:string、factId:string、type:string、time:string(RFC3339Nano)、payloadDigest:string、payload:JSON。完整 [`internal/worker/supervisorwire.JournalEntry`](wire-types.md#t-7c41880a71ca27cf)。以 sequence/factId/digest 去重；恢复时不可重放过期授权，只能读事实并重新取得当前所有权。

Supervisor stdout event：`{schemaVersion:1,type:string,time:string,payload:JSON}`。Agent notification 是 [`cmd/seven-worker-cli.agentNotification`](wire-types.md#t-61ef6f9c66cc0319)，frame 内仍为 Agent Adapter envelope。

`seven-worker-cli image probe --json` 返回 worker image 元数据，协议标识 **seven.worker-image/v1**；准入应核对镜像digest、工具链、Adapter manifest/source digest与协议支持，不凭浮动tag作为生产身份。命令详情见 [CLI](cli.md)。Supervisor 提出的 completion/evaluation candidate 不等于可信结果；Finalizer 在 Harness 外重算 ResultCommit、文件与哈希。

源码：[dispatcher](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go)、[stream](../../../seven-agent-force-server/cmd/seven-worker-cli/control_stream.go)、[interaction control](../../../seven-agent-force-server/cmd/seven-worker-cli/interaction.go)、[journal](../../../seven-agent-force-server/internal/worker/supervisorwire/journal.go)。
