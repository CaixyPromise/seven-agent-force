# AgentAdapter 接口参考与执行原理

适用范围：当前统一 Adapter 源码合同。本文区分线协议、Python SDK 行为与 Go 执行器的实际要求；新增工具或能力需要在实际消费端验证，接口说明不代表真实业务已通过。构建步骤见[开发指南](agent-adapter-developer-guide.md)，恢复与资源操作见[维护手册](agent-adapter-operations.md)。

## 1. 接入者必须实现什么

| 必须交付 | 要求 |
| --- | --- |
| 镜像入口 | manifest 指定的可执行程序，支持专用 stdin/stdout JSONL；代码和入口固定在准入镜像 |
| 握手 | 收到 initialize 后返回匹配 manifest 的 ready，不启动原生 Agent |
| 执行 | 收到 execution.start 后启动原生程序，报告真实 started 和一次 finished |
| 活动 | 将需要展示的原生事件转换为安全 message/plan/activity，不透传完整原生 JSON |
| 中断 | 执行中仍读控制消息，响应 interrupt，停止受监督子进程 |
| Session | AGENT 模式返回真实、稳定的 Session 引用；原生支持恢复时实现 resume |
| REVIEW（若声明支持） | 按提供的工作区、项目审查要求、schema 和报告路径执行；输出候选报告文件 |
| 平台工具（若声明支持） | 将原生工具调用映射为 `tool.request`，等待并校验 `tool.result`，回报原生调用是否收到/关闭；不得在 Adapter 内自行批准或执行 |

Adapter 不需要实现平台 HTTP API、Worker 注册、租约数据库或 TaskTurn 创建。不允许自行产生用户批准、可信 PASS 或资源删除目标。不使用 SDK 的实现者必须自行完成严格编解码、并发读写和子进程收尾。

## 2. 连接和消息为什么这样流转

```text
Server：业务权威、执行身份与结果门控
  ↕ Worker machine API（Adapter 不直接调用）
Worker ExecutionSession
  ↕ docker exec -i … seven-worker-cli control --stream
容器私有 UDS → Supervisor
  ↕ 专用 stdin/stdout JSONL
镜像 AgentAdapter
  ↕ 作者实现的 CLI/进程/原生协议
Coding Agent
```

Supervisor 校验镜像描述和运行时身份，创建 Adapter 子进程及独立管道；Python serve 的读循环独立于异步 handler，写入经锁串行化。Worker 的观察连接断开不等于 Adapter stdin 断开：前者可重连原 Supervisor，后者意味着 Adapter 会话结束，需要收尾，不能自行重跑 Agent。

这些管道限制消息的来源与传输路径，但不证明容器内容可信。镜像身份、平台 owner/lease/permit 校验及进程监督承担不同职责；Task 完成还要经过检查、Finalizer、canonicalizer 等外部权威验证。不能把 stdio 或原生 PID 自报当安全批准。

## 3. 消息外壳

一行一个 UTF-8 JSON 对象，末尾必须有 LF；字符串中的换行使用 JSON 转义。无 HTTP、无 JSON-RPC id/result 包装。

```json
{"protocol":"seven.agent-adapter/v1","method":"execution.start","key":"supervisor:2","payload":{"roundKey":"round:1","inputs":{"mode":"AGENT"}}}
```

此例仅演示外壳，业务输入需补齐第5节；不能直接用空输入启动真实任务。

| 字段 | 必填 | 类型及语义 |
| --- | --- | --- |
| protocol | 是 | 固定 seven.agent-adapter/v1 |
| method | 是 | 第4节方法之一，按方向校验 |
| key | 是 | 本条消息关联键，匹配 `[A-Za-z0-9._:-]{1,128}` |
| replyTo | 否 | 回复的下行 key；ready、finished 必须按下述关联 |
| payload | 是 | JSON object，具体字段由方法定义 |

顶层未知字段、重复 JSON key、NaN/Infinity、错误方向、非法 UTF-8、超大帧和无换行帧不得触发执行。帧上限 1 MiB（包括 LF）；message/plan/activity 的整个 payload 经规范编码后上限 64 KiB，不只是 text 字段。

`taskId/taskTurnId/attemptId/reviewRunId/reviewRunAttemptId` 出现时必须为正 int64 十进制字符串，无前导零，最大 9223372036854775807。PID 是正整数，不是实体 ID。roundKey 是当前 Adapter 会话内的 opaque 字符串（SDK 检查非空且最多128字符），不是 TaskTurn ID。协议 payload 的未知业务字段并非都由外壳 codec 拒绝；不能仅靠 envelope schema 证明业务输入合规。

## 4. 方法接口

down 表示 Supervisor → Adapter；up 表示 Adapter → Supervisor。下表为作者应生成的有效 payload；编码器并非对每个方法都提供完整字段级校验。

| 方法/方向 | payload | 何时及返回要求 |
| --- | --- | --- |
| initialize / down | 当前为 `{}` | 每个新进程会话一次；当前没有动态协商模型、权限或超时字段 |
| ready / up | name:string、version:string、capabilities:string[] | replyTo=initialize.key；name/version/能力集合必须匹配 manifest |
| execution.start / down | roundKey:string、inputs:object | 新一轮；不能在握手时提前启动 |
| execution.resume / down | start字段 + sessionReference:string | 使用原生会话恢复；缺少恢复handler返回unsupported，不能静默改成新执行 |
| execution.started / up | roundKey、nativePid:正整数 | 原生进程实际启动后一次；SDK默认无replyTo，Go按round关联 |
| message / up | roundKey、text:string | 安全展示文本；不承担审批或结果证明 |
| plan / up | roundKey、steps:string[] | 当前列表型计划，不是平台任务状态机 |
| activity / up | roundKey、activityKey:string、title:string、status:enum | 同活动用稳定activityKey；状态见SDK表 |
| execution.interrupt / down | roundKey | 只中断当前运行轮次 |
| control.ack / up | roundKey | replyTo=控制或重复请求key；只表示接收，不是终态；当前Go限制见第8节 |
| execution.finished / up | roundKey、outcome、summary:string、sessionReference:string | replyTo=start/resume.key；当前轮必须先收到started |
| error / up | code:string | 安全错误码；可有replyTo；不能携带原始异常正文 |
| tool.catalog / up | roundKey、toolNames、catalogDigest | Adapter 在原生会话成功接受本轮冻结工具定义后回报安全目录；名称必须排序、去重且与 Supervisor 冻结集合完全一致。它是可观察证据，不授予新能力 |
| tool.request / up | roundKey、toolName、localToolCallKey、nativeToolCallId、arguments、argumentsDigest | 请求当前执行明确开放的平台工具；当前源码识别 `run_command_with_network_access` 与 `request_git_delivery`，replyTo=start/resume key |
| tool.result / down | roundKey、localToolCallKey、nativeToolCallId、operationId、status及安全结果 | Server 已保存的权威 Operation 结果；同一结果可重投但不能重复推进原生调用 |
| tool.received / up | roundKey、localToolCallKey、operationId、status=`RECEIVED` | Adapter 已将结果交给匹配的原生调用；不等于 Agent 已继续或 Task 成功 |
| tool.closed / up | roundKey、localToolCallKey、reason | 原生调用已撤销、失效或关闭；不能借此伪造 Operation 终态 |

ready 能力最多16项。SDK自动生成 `activity`、`execution.interrupt`、`execution.start`，注册 resume 后增加 `execution.resume`。manifest `adapter.id` 对应 ready.name，`adapter.version` 对应 ready.version；集合必须精确一致，不能多报未实现能力。

工具能力是四项原子组：`tool.request`、`tool.result`、`tool.received`、`tool.closed`。Python SDK 的 Adapter 作者只声明可选能力 `tool.request`，SDK 会展开完整组；镜像 manifest 和 ready 必须声明展开后的同一集合。平台最终开放工具取 Server、Execution Profile、精确镜像 manifest 与 ready 的交集，Task prompt、仓库文件和环境变量不能增加工具。

`availableTools` 是经过认证的 Server→Worker 执行合同字段，不是公开 Task 输入。Worker 必须把该字段原样交给 Supervisor；Adapter 只有在 App Server `thread/start` 或 `thread/resume` 成功接受冻结定义后才能发送 `tool.catalog`。Supervisor 校验目录与本轮冻结集合完全一致，Server 只持久化安全工具名，不保存原生 Provider body。目录事件不能替代实际 `tool.request`：只有带原生 occurrence 的请求和后续 `tool.received` 才证明某个工具被真实调用并获得结果。

### 4.1 `run_command_with_network_access`

本批冻结的工具名称、描述和 JSON Schema 由平台提供，Adapter 只能按原生 API 映射，不能改名或改 schema。arguments：

```json
{
  "purpose": "安装锁定依赖，以便运行完成前检查",
  "command": "pnpm install --frozen-lockfile",
  "cwd": ".",
  "targets": [
    {"host": "registry.npmjs.org", "protocol": "https", "port": 443}
  ]
}
```

`purpose` 最多 2 KiB，`command` 是最多 16 KiB 的完整 shell 字符串，`cwd` 必须是仓库相对路径；targets 为 1–8 个精确 `host/protocol/port`。首版只接受 http/https、DNS hostname 和明确端口，不接受通配符、IP 字面量、userinfo、URL/path、localhost、控制面或私网目标。为保持 runner 的确定性 hostname 映射，同一请求不得重复 hostname，即使端口不同；需要多个端口时分成新的 Operation。

`argumentsDigest` 是规范化 arguments 的 SHA-256。Operation occurrence 为 `(attemptId, adapterSessionGeneration, localToolCallKey)`；同 identity 同 digest 复用，不同 digest 冲突。Dynamic Tool 返回只含 Operation ID、状态、exit code、安全摘要、有界脱敏输出和结果 digest。`DENIED` 与 `OUTCOME_UNKNOWN` 都是可交给 Agent 的真实结果；后者禁止平台自动重跑命令。

### 4.2 `request_git_delivery`

该工具只表达“当前工作完成后，请平台按 Task 已冻结的 Git Plan 交付”，不执行 `git push`，也不等待真实 Delivery 完成。工具只有在 Server、Execution Profile、精确镜像 manifest 与 Adapter ready 的交集允许，且当前 Task 已冻结有效 Git Plan 时才会出现在 `availableTools`。

```json
{
  "summary": "分页边界问题已经修复，回归检查已完成。"
}
```

- `summary` 必填，trim 后为非空 UTF-8，最多 2 KiB；未知字段拒绝。
- 不接受 repository、remote、branch、mode、credential、refspec、force、Review 或 Delivery policy。
- occurrence 继续使用 `(attemptId, adapterSessionGeneration, localToolCallKey)`；同 occurrence 同摘要 digest 复用，不同 digest 冲突。
- Server 保存 Intent 后返回 `tool.result.status=SCHEDULED` 和安全 Intent identity。该结果只证明意图已持久化，不证明 Finalizer、检查、Review、push、PR 或 readback 成功。
- Adapter 将 `SCHEDULED` 回答给原生调用并发送 `tool.received`；后续真实 Delivery 状态进入持久 conversation/Delivery API，不把 Agent 原生请求保持到 push 完成为止。
- 没有 Agent 调用时，用户创建 Task 时冻结的 Git Plan 仍可由 Finalizer 生成同一个 DeliveryRequest。工具不是用户明确交付要求的唯一触发器。

## 5. execution inputs：现有 Go 生产者合同

inputs 是对象，不是一套强类型SDK参数类。作者按 mode 读取，路径只使用平台提供的容器内路径，不允许 Task 自行提供宿主路径或入口。身份字段用于关联，不是授权凭据。

### AGENT

| 字段 | 类型 | 意义 |
| --- | --- | --- |
| mode | string | AGENT |
| taskId / attemptId | decimal-string | 当前实体身份 |
| instruction | string | 当前任务指令，保留平台生成的修订指导 |
| workspace | string | 当前工作副本，原生CLI cwd |
| output | string | 当前输出目录 |
| state | string | 本次状态/Session资料位置，不是平台业务数据库 |
| temporary | string | 临时目录 |
| provider | object | 当前只有 model:string；无 Provider key |
| feedback | string | Supervisor提供的同会话修订反馈；初轮可空 |
| reviewCorrection | object或null | 只读审查报告关联；字段如下 |

reviewCorrection 字段：sourceReviewRunId（decimal-string）、revisionKind（AUTOMATIC/HUMAN_AUTHORIZED）、可选 verdict、reportPath、reportDigest、reasonCode。有报告时使用已校验的只读路径与摘要；无报告时不能伪造报告。不要把整个对象拼入日志，也不要自行替换报告或冻结政策。

Go 当前AGENT完成还要求 sessionReference 非空、长度不超过512字节，同一运行链已有session时必须相同。SDK允许构造空引用，但它不足以通过这个业务入口。因此本地无Session示例仅用于SDK，不是可交付AGENT实现。

### REVIEW

| 字段 | 类型 | 意义 |
| --- | --- | --- |
| mode | string | REVIEW |
| workspace | string | 已从冻结结果派生的审查工作副本，通常 /scratch/repo |
| model | string | 冻结模型；注意不是 AGENT 的 provider.model |
| prompt | string | 平台固定的报告与审查要求 |
| output_schema | string | 容器内报告schema文件路径，不是JSON对象 |
| output_path | string | 候选报告文件，通常 /report/review-report.json |
| resultCommit / resultTree | string | 冻结Git身份 |
| requirements_path / review_contract_path | string | 已物化的任务/项目要求与平台固定审查合同路径 |

当前REVIEW inputs并不包含taskId/reviewRunId；roundKey为 `review:<runId>`，不要假设全部Agent字段都会提供。Reviewer必须实际读取 Task 锁定的 ProjectVersion 审查提示词、任务要求和平台合同，生成报告文件，不能只在summary里说已审查。当前候选文件须为非空普通文件且不超过256 KiB；Server另行执行完整canonical校验。REVIEW这条调用不要求返回Agent Session引用，也不通过当前Round回调向普通页面转发活动。

AGENT和REVIEW共用传输，不表示业务输入字段已经统一。字段改动要同步Go生产者、参考Adapter及测试；开发者不能凭命名推测字段。

## 6. Python API 参考

导入路径 `seven_agent.force.agent`；SDK核心只使用标准库。

| API | 参数/返回 | 实现者责任 |
| --- | --- | --- |
| AgentAdapter(name=…, version=…) | 非空名称及版本 | 与镜像manifest一致 |
| @agent.run | async handler(request, context) → RoundResult | 必需，恰好两个位置参数；同类handler只能注册一次 |
| @agent.resume | 同上，request为ResumeRequest | 可选，真实恢复原生session；不静默重建 |
| RunRequest | frozen dataclass：round_key:str、inputs:dict | frozen不意味着内部dict递归不可变，不修改权威输入 |
| ResumeRequest | 继承RunRequest，新增session_reference:str | SDK接收非空引用且最多4096字符；实际AGENT完成限制更严见上 |
| ExecutionContext.round_key | str | 本轮关联 |
| ExecutionContext.cancelled | asyncio.Event | 取消时被set；同时handler task会被cancel |
| await context.started(native_pid=pid) | 正整数PID；缺省为Adapter自身PID | 真实子进程先spawn再报告；缺省仅适合进程内执行，SDK防重复上报 |
| await context.emit_message(text) | str | 脱敏、限长文本 |
| await context.emit_plan(steps) | list[str] | 有界计划 |
| await context.emit_activity(key=…, title=…, status=…) | ActivityStatus枚举 | 不接受字符串代替SDK枚举；显式描述成功或失败 |
| RoundResult(outcome, summary="", session_reference="") | RoundOutcome枚举 | 仅本轮结果，不含Task PASS、批准或Artifact字段 |
| RoundResult.finished(summary=…, session_reference=…) | outcome=FINISHED | AGENT返回真实稳定session；不代表测试通过 |
| agent.capabilities | 排序后的list[str] | SDK按注册handler派生 |
| agent.serve() | 同步进程入口 | 使用stdin/stdout并运行事件循环；不得先输出欢迎语 |
| await agent.serve_streams(reader, writer) | 测试/嵌入入口 | 调用方负责创建和关闭流，不等于平台连接注册 |

RoundOutcome：FINISHED、CANCELLED、FAILED。ActivityStatus：STARTED、COMPLETED、SUCCEEDED、FAILED；COMPLETED只表达活动结束，不应展示为检查成功。框架不会根据handler正常返回推断活动SUCCEEDED。

取消会引发asyncio.CancelledError，必须在finally清理子进程。不能仅设置一个布尔变量然后留下shell；SDK不是通用原生进程管理器。普通handler异常折叠成 FAILED / ADAPTER_EXECUTION_FAILED，不返回异常字符串。若handler未报告started便完成，SDK将其视为失败，Go也拒绝无started的完成序列。

## 7. 一条完整线协议示例

以下独立JSON行用于说明有效关联；nativePid和Session必须替换成真实事实。上下文路径仅为容器内示例。

```jsonl
{"protocol":"seven.agent-adapter/v1","method":"initialize","key":"s:1","payload":{}}
{"protocol":"seven.agent-adapter/v1","method":"ready","key":"a:1","replyTo":"s:1","payload":{"name":"example","version":"1","capabilities":["activity","execution.interrupt","execution.start"]}}
{"protocol":"seven.agent-adapter/v1","method":"execution.start","key":"s:2","payload":{"roundKey":"round:1","inputs":{"mode":"AGENT","taskId":"123","attemptId":"456","instruction":"完成本地目标","workspace":"/workspace/123/attempts/456/repo","output":"/workspace/123/attempts/456/output","state":"/workspace/123/attempts/456/state","temporary":"/workspace/123/attempts/456/tmp","provider":{"model":""},"feedback":"","reviewCorrection":null}}}
{"protocol":"seven.agent-adapter/v1","method":"execution.started","key":"a:2","payload":{"roundKey":"round:1","nativePid":42}}
{"protocol":"seven.agent-adapter/v1","method":"activity","key":"a:3","payload":{"roundKey":"round:1","activityKey":"check:1","title":"执行项目检查","status":"STARTED"}}
{"protocol":"seven.agent-adapter/v1","method":"activity","key":"a:4","payload":{"roundKey":"round:1","activityKey":"check:1","title":"项目检查命令退出成功","status":"SUCCEEDED"}}
{"protocol":"seven.agent-adapter/v1","method":"execution.finished","key":"a:5","replyTo":"s:2","payload":{"roundKey":"round:1","outcome":"FINISHED","summary":"本轮完成，等待平台检查。","sessionReference":"native-session-example"}}
```

下一轮由Supervisor决定：有session时发送 execution.resume，使用新的roundKey、输入及原sessionReference。不是Adapter自己循环创建TaskTurn。完成后仍执行平台检查；检查失败可在同一原生Session修订，检查通过才进入既有完成链。

## 8. 去重、顺序和恢复：不要超出当前实现承诺

Python SDK以规范编码的method+payload计算请求digest：同key不同内容冲突；同round不同内容也冲突。同round执行中重投返回control.ack，完成后返回缓存finished；最多2048个请求key、256个round，不驱逐旧round后重新执行。它们都是进程内记录，进程丢失不会保留，不能称跨进程exactly-once。initialize重复返回ALREADY_INITIALIZED，不是重放ready。

**当前Go消费者**：接受与当前请求key和round匹配的control.ack，但不把它当started或finished。同一Process最多记录256个round的输入摘要和已观察完成结果；同round不同method/输入/session直接冲突。只有本Process此前已收到真实started并完成的轮次，才允许接受SDK无新started的缓存finished，且结果必须与之前一致；不再次触发启动回调。无历史的finished、错误replyTo、重复started和改变的缓存结果都拒绝。Go↔Python真实管道已覆盖运行中同key重复请求及完成后同round重放。正常Worker恢复仍重连现有Supervisor观察连接，不自动重发start；进程丢失不使用这份内存记录承诺恢复。

中断也有同样区别：SDK对interrupt设置取消事件、取消handler并发ack，ack与finished不承诺固定顺序；Go在自身context取消时发送有界interrupt然后Close，不等待“ack即完成”。不得把接收确认记录成清理完成。

当前Go握手和写入限时5秒；Close先等待5秒，随后进程组SIGTERM，再等2秒后SIGKILL。SDK写入drain限时5秒，流结束取消handler并有界等待。业务deadline/lease由外层运行时负责，不从Adapter任意参数扩展。进程组之外的资源仍须Worker精确journal清理。

WAIT_USER丢ACK恢复属于Worker↔Server Evaluation合同：查已有权威决定，恢复记录并重放确认，不是重新发Adapter start/resume，更不是开放REVIEWING下任意检查物化。

## 9. 错误参考

| 产生方 | code | 意义/处理 |
| --- | --- | --- |
| codec/SDK | ADAPTER_PROTOCOL_INVALID、ADAPTER_FRAME_TOO_LARGE、ADAPTER_MESSAGE_TOO_LARGE | 帧或展示payload非法；修编码/大小，不重放原始内容到日志 |
| SDK | ADAPTER_METHOD_UNSUPPORTED | 方法方向不支持或未注册；不触发执行；SDK可继续接收，Go执行轮中error仍会结束当前会话 |
| SDK | ADAPTER_IDEMPOTENCY_CONFLICT | 请求key/round内容变更；不能覆盖旧执行 |
| SDK | ADAPTER_ALREADY_INITIALIZED、ADAPTER_NOT_INITIALIZED | 握手顺序错误 |
| SDK | ADAPTER_ROUND_INVALID、ADAPTER_ROUND_NOT_RUNNING | 无效round或中断非运行轮 |
| SDK | ADAPTER_INPUT_INVALID、ADAPTER_BUSY、ADAPTER_SESSION_LIMIT | 输入错误、已有执行、内存历史上限 |
| SDK | ADAPTER_EXECUTION_FAILED | handler异常的安全折叠，不包含原始异常 |
| Go | ADAPTER_HANDSHAKE_FAILED、ADAPTER_CAPABILITY_MISMATCH | 握手或manifest身份不匹配 |
| Go | ADAPTER_ROUND_MISMATCH、ADAPTER_NATIVE_START_INVALID、ADAPTER_REPLAY_MISMATCH | round错配、重复started/无效PID、缓存结果改变 |
| Go AGENT | ADAPTER_ROUND_NOT_FINISHED、ADAPTER_SESSION_UNAVAILABLE、ADAPTER_SESSION_CHANGED、ADAPTER_SESSION_ARCHIVE_FAILED | 本轮失败或原生Session不满足业务要求 |

这些是内部协议/运行时原因，不是普通用户HTTP错误码承诺。Go会折叠部分上行error，运维不得假设全部Python细分码均会进入Server持久化。ADAPTER_NATIVE_NOT_STARTED是SDK内部校验原因，通常被折叠为执行失败，不承诺原样返回。

## 10. Processor 子包与 Agent 接口不可混用

`seven_agent.force.processor` 提供普通dict构造函数；由独立Processor bootstrap执行process(context)，不是Agent stdio方法。

| 函数 | 必填关键字/可选参数 | 返回用途 |
| --- | --- | --- |
| pass_result | 可选reason_code="OK", message="", checks, findings, outputs, actions | PASS候选Result Contract |
| ask_agent | reason_code, message；可选checks, findings | REVISE候选 |
| wait_user | reason_code, message；可选questions, findings, outputs | WAIT_USER候选，非在线Agent提问API |
| reject | reason_code, message；可选checks, findings | REJECT候选 |
| manual_review | reason_code, message；可选checks, findings, questions, outputs | MANUAL_REVIEW候选 |
| workspace_path | name, path；storage="IMMEDIATE" | PATH输出声明 |
| archive_output | name, files；excludes=None, format="ZIP", storage="ON_DEMAND" | 平台从冻结结果生成归档的选择声明 |
| get_env | name, default_value=None | 获取当前Processor获准的环境绑定，不授予新权限 |

Result Contract字段为decision、reasonCode、message及对应可选集合，具体业务校验仍由平台执行。调用helper不直接保存数据库、不批准用户请求、不取得SECRET，也不能把Processor环境复制进Agent。完整语义沿用[架构与合同](architecture-and-contracts.md)。

## 11. 源码定位与接口维护规则

- [线协议schema](../../deploy/workforce/runtime-sdk/protocol/agent-adapter-v1.schema.json)：当前仅外壳，不是全部payload和状态机schema。
- [Python codec](../../deploy/workforce/runtime-sdk/src/seven_agent/force/_protocol.py)、[SDK](../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py)：编码、字段限制、调度与错误。
- [Go消费者](../../seven-agent-force-server/internal/worker/agentprotocol/process.go)：真实握手、关联、顺序、取消与关闭。
- [Agent输入生产者](../../seven-agent-force-server/cmd/seven-worker-cli/agent_adapter.go)、[Review输入生产者](../../seven-agent-force-server/cmd/seven-worker-cli/review_adapter.go)：两种inputs和完成条件。
- [Processor helpers](../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py)：独立结果构造接口。

新增方法或字段时同步生产者、Go/Python校验、消费者、fixture、镜像声明和本文；不能只加SDK方法或manifest能力。必须覆盖错误方向、字段边界、真实时序、取消、重复请求与安全日志。inspect/dev/build的项目 `--module` 接口已实现，支持范围见开发指南，不是通用Python构建后端。当前源码开放持久问题/补充、`run_command_with_network_access` 与 `request_git_delivery`；真实 Git 写入与原生 Dynamic Tool 调用分别保留证据，不能用平台默认交付冒充原生调用。任意插件注册、凭据注入和项目预览仍未开放。
