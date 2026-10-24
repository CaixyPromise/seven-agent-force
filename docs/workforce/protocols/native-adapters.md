# 原生Codex／Claude Adapter映射

以下描述仓库内Adapter实际使用的原生操作；不把整个第三方SDK当作Workforce公开API。外部调用者仍使用Open/MCP，镜像开发者实现 [Agent Adapter协议](agent-adapter.md)。当前Codex CLI锁定0.147.0，Claude Code锁定2.1.273；升级需同步原生协议、镜像descriptor与fixture。

## Codex App Server JSON-RPC

子进程stdin/stdout JSON-RPC，method:string、id:RPC关联标识、params:object。Client timeout默认30s。此处initialize属于原生Codex协议，与Workforce MCP server/discover是两个独立表面。

| 原生method | params字段与类型（当前传入） | 返回／消费 |
| --- | --- | --- |
| initialize | clientInfo:{name:string,version:string}；capabilities:{experimentalApi:boolean=true} | 初始化response，随后notifications initialized |
| initialized（notification） | {} | 无response |
| thread/start | cwd/model/modelProvider/approvalPolicy/sandbox:string；Review可ephemeral:boolean；Agent可dynamicTools:ToolDefinition[] | thread:{id:string} |
| thread/resume | threadId:string；继承上述配置；path?:string（恢复rollout） | thread:{id:string}，须匹配恢复reference |
| thread/inject_items | threadId:string；items:object[]（冻结operation结果注入） | 未知结果禁止重复注入／续轮 |
| turn/start | threadId:string；input:array{text输入}；outputSchema?:object | turn:{id:string}，后续等turn/completed |
| turn/steer | threadId:string；expectedTurnId:string；input:array{text输入} | 成功映射message RECEIVED，失败UNKNOWN |
| turn/interrupt | threadId:string；turnId:string | 等turn完成后冷挂起／中断 |
| item/tool/requestUserInput（server request） | threadId/turnId等原生绑定；questions:object[] | 转QuestionRequest；答复原生answers对象 |
| item/tool/call（server request） | 原生callId/工具name:string、arguments:object及绑定信息 | 转ToolCallRequest，回传平台ToolResult映射 |

text输入由`_text_input`构造，operation注入由`_operation_injection_item`构造；具体子字段、问题映射和原生结果格式均以 [app_server.py](../../../deploy/workforce/adapters/codex/app_server.py)对应构造器为准。原生thread/turn/call ID是opaque string，不转数据库int64。

AGENT显式启用features.default_mode_request_user_input，REVIEW使用结构化output schema且不开放AGENT交互能力。原生进程存活不等于round完成，实际以turn事件驱动。冻结操作注入结果未知时先停并回查，避免重复动作。

## Claude Code stream-json 与桥接

Claude执行使用锁定CLI的stream-json，消费session_id/sessionId:string、message.content:object[]、result:string、structured_output/structuredOutput:object；运行模式、model、workspace、prompt、availableTools、outputSchema由prepare校验。恢复通过已持久化session reference，消息／挂起的支持取决于已准入能力，不自动宣称与Codex完全相同。

镜像内UDS工具桥接request为闭合object `{nativeCallId:string,name:string,arguments:object}`，单行≤128 KiB；response `{ok:boolean,value:JSON}`。ask-user工具questions含key/title/description:string、options:array{key,label:string}和interactionKey:string；转SDK QuestionRequest。平台工具名称必须来自冻结目录，再转ToolCallRequest；response成功只是桥接结果，要保留tool_received的独立回执。

源码：[Claude Adapter](../../../deploy/workforce/adapters/claude/adapter.py)、[Codex Adapter](../../../deploy/workforce/adapters/codex/adapter.py)。原生SDK完整上游字段不在本库稳定契约中，本页记录已实现映射，不能用它假定兼容任意新版本原生客户端。
