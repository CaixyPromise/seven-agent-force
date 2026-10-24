# Agent Adapter v1：stdio JSONL

协议 `seven.agent-adapter/v1`。Supervisor 和镜像自带Adapter使用stdin/stdout双向UTF-8 JSONL。每个frame一物理行且以newline结束；不能将日志写到stdout混入协议，诊断使用stderr。单帧≤1 MiB；message/plan/activity的compact payload≤64 KiB；拒绝重复JSON key、未知envelope字段、尾随JSON、非有限数与过深嵌套。

## Frame与方法参数

| 字段 | 类型 | 必填与规则 |
| --- | --- | --- |
| protocol | string | 必填，固定seven.agent-adapter/v1 |
| method | string | 必填，下面25个之一，按方向校验 |
| key | string | 必填，`[A-Za-z0-9._:-]{1,128}` |
| replyTo | string | 可省略；存在时非空合法key，指向原请求 |
| payload | object | 必填，按method定义 |

`down`：Supervisor→Adapter；`up`：Adapter→Supervisor。下面列出字段级payload或完整类型链接，所有键以wire命名为准。普通执行／展示payload按SDK生成结构记录；互动和工具payload受更严格闭合字段校验。

| 方向 | method | payload参数与类型 | 语义／回执 |
| --- | --- | --- | --- |
| down | initialize | {}，初始化参数当前为空 | ready |
| down | execution.start | roundKey:string；inputs:object（镜像Adapter冻结输入） | execution.started → execution.finished |
| down | execution.resume | roundKey:string；inputs:object；sessionReference:string(非空≤4096) | execution.started → execution.finished |
| down | execution.interrupt | roundKey:string | control.ack；本轮停止／finished |
| down | interaction.respond | [`internal/worker/agentprotocol.InteractionAnswer`](wire-types.md#t-d14aff88b8a22611) | interaction.received |
| down | user.message | [`internal/worker/agentprotocol.UserMessage`](wire-types.md#t-40f6f165edbcb421) | message.received |
| down | execution.suspend | roundKey:string | execution.suspended |
| down | tool.result | [`internal/worker/agentprotocol.ToolResultPayload`](wire-types.md#t-cfc84c0f97e87dcc) | tool.received |
| up | ready | name:string；version:string；capabilities:string[] | 与initialize的key关联 |
| up | execution.started | roundKey:string；nativePid:positive integer | 报告真正启动；不能只报告进程存活 |
| up | execution.finished | roundKey:string；outcome:string enum；summary:string；sessionReference:string | 本轮结束；不是Task可信成功 |
| up | message | roundKey:string；text:string | 展示消息 |
| up | plan | roundKey:string；steps:string[] | 展示计划 |
| up | activity | roundKey:string；activityKey:string；title:string；status:string enum | 展示活动 |
| up | control.ack | roundKey:string | 控制接收回执 |
| up | error | code:string | 安全错误码，不输出异常原文／秘密 |
| up | interaction.request | [`internal/worker/agentprotocol.QuestionRequest`](wire-types.md#t-ce23f8df4dac6f0c) | 挂起或提示回答请求 |
| up | interaction.received | roundKey/interactionKey/responseId:string；status:string enum | RECEIVED / REJECTED / UNKNOWN |
| up | interaction.closed | roundKey/interactionKey/reason:string | 原生请求关闭 |
| up | message.received | roundKey/messageKey:string；status:string enum | RECEIVED / REJECTED / UNKNOWN |
| up | execution.suspended | roundKey:string；sessionReference:string | 保存可恢复会话 |
| up | tool.catalog | [`internal/worker/agentprotocol.ToolCatalogPayload`](wire-types.md#t-d0c093d94cb7b7ee) | 确认已加载安全工具目录 |
| up | tool.request | [`internal/worker/agentprotocol.ToolRequestPayload`](wire-types.md#t-85fdfa303a772fac) | 请求平台动作，不等于批准 |
| up | tool.received | [`internal/worker/agentprotocol.ToolReceiptPayload`](wire-types.md#t-01d1ae55d45642aa) | 操作结果收到状态 |
| up | tool.closed | [`internal/worker/agentprotocol.ToolClosedPayload`](wire-types.md#t-7befa62321d6299b) | 本轮工具等待关闭 |

RoundOutcome：FINISHED / FAILED / CANCELLED；ActivityStatus的值见SDK enum声明。DeliveryStatus：RECEIVED / REJECTED / UNKNOWN。interaction.closed.reason：NATIVE_REQUEST_CLEARED / TURN_COMPLETED / TURN_INTERRUPTED。详细dataclass、回调类型和方法参数见 [Python SDK](python-sdk.md)。

## 问题、答案与平台工具

QuestionRequest：roundKey/interactionKey:string，questions:Question[]（1–3），isBlocking:boolean，全部字段都需存在。Question：key/title/description/kind:string、required/allowFreeText:boolean、options:QuestionOption[]；kind为TEXT/SINGLE_CHOICE/MULTIPLE_CHOICE。title≤512 bytes，description≤4096，options≤8；TEXT无options，选择型需options；数组不可用null代替。QuestionOption为key/label:string。

答案为roundKey/interactionKey/responseId:string、answers:Answer[]；responseId为正decimal int64 string。Answer：questionKey:string、selectedOptionKeys:string[]、text:string；text≤16 KiB；required/单选/允许自由文本规则与原问题一致。迟到回答不能投给新round。同一delivery key不同payload返回ADAPTER_IDEMPOTENCY_CONFLICT；UNKNOWN不能声称原生Agent已收到。

平台内置工具仅以下两种：

| toolName | arguments闭合object（均必传） | 校验 |
| --- | --- | --- |
| run_command_with_network_access | purpose:string、command:string、cwd:string、targets:ToolTarget[] | purpose1–2048 bytes；command1–16384；cwd为安全repo相对路径≤4096；targets1–8 |
| request_git_delivery | summary:string | 1–2048 bytes；提出交付，不直接push |

ToolTarget：host:string（合法hostname≤253，不直接IP）、protocol:string枚举http/https、port:integer1–65535。工具请求还携带roundKey/localToolCallKey/nativeToolCallId/toolName:string、arguments:object、argumentsDigest:string；nativeToolCallId是opaque原生标识≤512，不是内部数据库ID。ToolResult包含operationId:string(ID)、status:string、exitCode:integer或null、summary/output/resultDigest/reasonCode:string。目录、args与结果digest需按已有canonical实现计算，不能跨语言自行改变JSON规范。

capabilities只说明Adapter能处理的方法，不授予工具权限。平台冻结allowlist决定availableTools；授权、网络隔离、Git计划、lease/current round由Server/Worker执行。

## 镜像Manifest与开发顺序

Descriptor包含id/version/executable:string、args:string[]、binarySha256/sourceSha256:string、protocol:object{name:string,min:integer,max:integer}、transport:string=stdio、capabilities:string[]。源码清单和digest需与已准入不可变镜像一致。executable是`/opt/seven/adapter/`下绝对安全路径；binary/source摘要64位小写hex；args≤8；capabilities≤16。execution.start+execution.interrupt必须支持；interaction、message、tool、suspend相关能力按完整组准入，不允许只支持请求不支持回执。

开发时先实现SDK handler，声明能力，正确调用started、发送消息和回执，再处理取消／冷恢复；frame有效只证明格式，不证明操作获准。执行结束需排空受控回执；未确认回执不能被finished覆盖。实际completion由Supervisor提出，Finalizer独立核验。

仓库JSON Schema资产目前没有列全interaction.closed，且部分payload字段描述早于当前codec。此文档以Go codec和Python validator为准；不能只依据旧schema枚举拒绝合法现有method。这是已发现的描述差异，本次未修改运行时实现。

源码：[codec](../../../seven-agent-force-server/internal/worker/agentprotocol/codec.go)、[interaction](../../../seven-agent-force-server/internal/worker/agentprotocol/interaction.go)、[tool](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go)、[manifest](../../../seven-agent-force-server/internal/worker/agentprotocol/manifest.go)、[SDK](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py)、[现有schema](../../../deploy/workforce/runtime-sdk/protocol/agent-adapter-v1.schema.json)。
