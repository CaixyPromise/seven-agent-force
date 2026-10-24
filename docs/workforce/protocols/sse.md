# 任务事件 SSE

GET `/api/workforce/tasks/:id/events/stream`，使用 Framework 用户身份与 workforce:task:query 权限。id:string 为 Task ID；query `after`:string(decimal event cursor)，首次可省略；服务端按 int64 解析，当前实现未用 Last-Event-ID Header 代替 query。

| 成功响应 | 类型／语义 |
| --- | --- |
| Content-Type | text/event-stream |
| Cache-Control | no-cache |
| X-Accel-Buffering | no |
| id 行 | decimal string，当前 event ID |
| event 行 | string，Task event.Type |
| data 行 | JSON Task Event 对象，按 domain.Event JSON tag |

```text
id: 123
event: task.changed
data: {"id":"123"}

```

这是 framing 示意，不是完整 Event payload。实际字段见 [类型索引](wire-types.md) 的 task/domain.Event；type 值由事件生产者决定。连接约25秒结束并发送 `: reconnect` 注释，内部500ms扫描；客户端记住最后收到的 event ID，以 `after` 重连并去重。先通过普通 GET task 读取当前状态，再增量消费，断线不代表任务失败。错误事件可能为 `event:error`、data `{code:"INVALID_ID"}` 或 `{code:"EVENT_STREAM_ERROR"}`。

GET `/api/workforce/tasks/:id/events?after=...` 是普通 JSON 列表；GET `/api/workforce/tasks/:taskId/conversation` 是普通 conversation 读取，**当前没有独立 conversation SSE 路由**。浏览器需要带 token 的 fetch stream 或同等获授权方式；原生 EventSource 自定义 Header 的限制由调用方处理，不能把 token 放在 URL。

源码：[Task 事件与流](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go)、[conversation](../../../seven-agent-force-server/internal/app/workforce/interaction/controller/http.go)。
