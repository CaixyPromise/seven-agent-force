# MCP 2026-07-28 接入指南

当前锁定 Go SDK v1.7.0；提供 **无会话 Streamable HTTP**，默认 endpoint `/mcp`，仅 POST，响应为 JSON。协议版本 **2026-07-28**。不建立 Mcp-Session-Id；没有 GET 事件流、DELETE 会话、resources/prompts/sampling/elicitation。旧 initialize + notifications/initialized 流程不在此服务支持范围。

每次请求 `Authorization: Bearer <APPLICATION_API_KEY>`，同一把 Key 向 Workforce Open API 验证身份；不缓存客户 Key、不用服务器全局客户 Key。Host 必须在白名单，Origin 存在时必须匹配显式白名单。

## JSON-RPC 方法与公共参数

| JSON-RPC 方法 | params 类型／字段 | 返回 |
| --- | --- | --- |
| `server/discover` | object，含下方 `_meta` | serverInfo、capabilities、supportedVersions；无会话发现 |
| `tools/list` | object，`_meta`，SDK cursor 可选 string | tools[] 与 inputSchema |
| `tools/call` | object，`_meta`、name:string、arguments:object | CallToolResult |
| `ping` | object，`_meta` | 空成功结果；由 SDK 处理 |

JSON-RPC envelope：jsonrpc:string 固定 "2.0"；id:string 或 number（RPC correlation，**不是数据库 ID**）；method:string；params:object。客户端每个请求携带 `_meta`：

```json
{
  "jsonrpc":"2.0",
  "id":1,
  "method":"server/discover",
  "params":{
    "_meta":{
      "io.modelcontextprotocol/protocolVersion":"2026-07-28",
      "io.modelcontextprotocol/clientInfo":{"name":"your-client","version":"1.0"},
      "io.modelcontextprotocol/clientCapabilities":{}
    }
  }
}
```

`protocolVersion`:string；clientInfo:object{name:string,version:string}；clientCapabilities:object。HTTP 使用 Content-Type application/json、Accept application/json, text/event-stream；`Mcp-Protocol-Version: 2026-07-28` 必传；`Mcp-Method` 填本次RPC method，推荐由同版本SDK设置。缺少版本Header会被拒绝，不能只填_meta。此服务虽声明 HTTP transport，未打开 SSE 返回。

## 六个 Tools：arguments、返回与限制

所有工具 arguments 都是闭合 object，ID 必须 canonical 正十进制 string。最终权限与 Runtime 兼容性由 Open 服务再次校验。

| Tool | arguments（`!` 为必填） | structuredContent |
| --- | --- | --- |
| `list_model_options` | projectId:string(ID)! | ModelOptions{schema:string, projectId:string, items:ModelOption[]} |
| `list_runtime_options` | projectId:string(ID)!；modelCredentialId:string(ID)! | RuntimeOptions{schema:string, projectVersionId:string, items:RuntimeOption[]} |
| `create_task` | projectId:string(ID)!；idempotencyKey:string!；instruction:string!；externalCorrelationId:string!；modelSelection:object{credentialId:string(ID)!,model:string!}!；runtimeReleaseId:string(ID)! | Task（Open 公共视图） |
| `get_task` | taskId:string(ID)! | Task |
| `list_task_outputs` | taskId:string(ID)! | {taskId:string,outputs:Output[]} |
| `read_task_output` | taskId:string(ID)!；outputId:string(ID)! | verifiedOutput 元信息；文本通过 content 返回 |

create_task 还接受 name:string、tags:string[]、referenceAssetIds:string(ID)[]、additionalCompletionCheck:object、reviewEnabled:boolean、deliveryPreference:string、executionTimeoutSeconds:integer；语义与 [Open 创建参数表](open-rest.md)一致。idempotencyKey 映射 Open Idempotency-Key Header；projectId 映射 URL，不放进 Open JSON body。Task、ModelOption、RuntimeOption、Output 的逐字段定义见 [Wire 类型索引](wire-types.md)，`internal/mcpserver.*` 类型章节。

read_task_output 先读取可见输出列表验证归属，再核对大小、SHA256、UTF-8；仅接受 text/* 或 application/json，最多 **256 KiB**。verifiedOutput 为 `{outputId:string,name:string,mediaType:string,sha256:string,sizeBytes:string,verified:boolean}`；不要将这个 tool 当作任意 URL 下载器或二进制下载器。

示例 tools/call 的 params（附上同样的 _meta）：

```json
{"name":"get_task","arguments":{"taskId":"123"},"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"your-client","version":"1.0"},"io.modelcontextprotocol/clientCapabilities":{}}}
```

## 错误与资源边界

CallToolResult.content 为 SDK content block 数组；structuredContent 为成功 DTO。业务失败设置 isError:true，并返回 `{error:{code:string,message:string,retryable:boolean}}`。这是工具业务失败，与 JSON-RPC `{error:{code:integer,message:string,data:object}}` 不同。身份拒绝可能是 HTTP401/JSON-RPC -32000；限流 HTTP429/-32001，data.code RATE_LIMITED、retryable:true。解析／协议参数错误交 SDK 处理；不要重发格式错误。

默认最大 body 256 KiB、最大文本输出 256 KiB、全局并发64、入口并发64、每 Application 并发8；可收紧，不能将正文／文本上限扩大超过256 KiB。失败时依 retryable 和读回查策略处理，创建始终保留原 key 与原 payload。

源码：[Tools](../../../seven-agent-force-server/internal/mcpserver/tools.go)、[类型](../../../seven-agent-force-server/internal/mcpserver/types.go)、[HTTP transport](../../../seven-agent-force-server/internal/mcpserver/handler.go)、[原始协议样例测试](../../../seven-agent-force-server/internal/mcpserver/handler_test.go)。旧设计背景见 [既有 MCP 文档](../stateless-open-task-mcp-server.md)。
