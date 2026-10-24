# 公共传输、鉴权与错误约定

## 身份与用途

| 表面 | 身份 | 作用范围 |
| --- | --- | --- |
| `/api/open/workforce/v1/*` 和 MCP | `Authorization: Bearer <Application API Key>` | 按 Application 和其 tenant 校验 Project、Task、Output |
| `/api/workforce/*` | Framework 登录得到的用户 access token、组织范围、RBAC | 用户／组织可见对象；独立权限包装器 |
| `/api/internal/workforce/v1/*` | Worker Bearer credential，加 instance/boot/lease/epoch | 注册 Worker 和当前执行所有权；不同于用户 token |
| 出站 Webhook | `X-Seven-Signature` HMAC | 接收方验签、去重；不使用回调 URL 中的密钥 |
| Runtime 内 stdio/UDS | 容器内执行契约、当前 round、租约和 permit | Adapter 报告事实，授权由 Control Plane 决定 |

API Key、用户 token、Worker token 不能互换。Application 停用后其 API Key 返回 `401 APPLICATION_DISABLED`。列表与读取不会自动获得修改权限。tenant_id 来自已鉴权 OrganizationScope，客户端不能通过 JSON 自选组织身份。

## 类型与格式

| 类型 | 传输表示 | 说明 |
| --- | --- | --- |
| 实体 ID、relation ID、epoch/generation 等 int64 | decimal **string** | Web 不能转换成 JavaScript Number；Open 的正 ID 必须 canonical 且 ≤ 9223372036854775807 |
| int/float/bool | JSON number/boolean | `schemaVersion`、端口、普通计数等遵循 DTO，不把所有数字都转成字符串 |
| time.Time | RFC3339/RFC3339Nano string | UTC 时间语义；具体 string 字段要读该方法定义 |
| `[]T` / `*T` | array / object 或 null | 可空与业务可选是两件事 |
| `json.RawMessage` | 任意 JSON 值 | 已知稳定结构会另列；上游透传和运行快照不是随意可写数据 |
| Go `[]byte` | base64 string | 二进制下载为 HTTP body，不是 base64 JSON |
| digest/SHA/nonce/opaque native ID | string | 不作为数据库数值 ID 解析 |

`json:",string"` 强制 JSON 引号。HTTP `response.Success` 还归一化 int64/uint64；原生 JSONL、AMQP、manifest 不经过 HTTP 归一化。实现了 MarshalJSON 的对象和 RawMessage 可能自行控制编码，优先看实际 wire 定义。

`omitempty` 只代表零值可能被省略。字段是否必填、空数组是否允许、期望版本与状态前置条件，由各 application/domain 校验决定，不能直接用 Go 零值推出可选。严格 JSON 解码不一定在所有 operator 方法一致实施；Open、MCP 和 Agent 的限制见各指南。

任务详情的 `taskResponse.MarshalJSON` 展开 Task，并主动去掉 executionSnapshot、旧 Harness 选择信息和内部 instruction 控制内容；类型索引记录声明，**不承诺内部 Task 的全部字段都公开**。外部集成应使用专门的 OpenTaskView。

## HTTP 包装与失败

普通 JSON 成功包装：

```json
{"code":0,"data":{},"message":"ok","traceId":"optional-trace"}
```

| 字段 | 类型 | 意义 |
| --- | --- | --- |
| `code` | integer | Framework 数值业务码；成功为 0 |
| `data` | JSON value | 成功 DTO，或错误详情 |
| `message` | string | 人可读信息；不作为程序判断依据 |
| `traceId` | string，可能省略 | 排查关联标识 |
| `data.reasonCode` | string，存在时读取 | 稳定业务原因；与顶层数值 code 不同 |

Open 创建返回 **202**，其余通常 200；状态操作和错误可能有独立 status。下载、SSE、Provider 成功透传是例外。鉴权／网关层拒绝可能并非相同 JSON 包装，客户端同时检查 HTTP status、Content-Type 和业务 code。

400 修正输入；401/403 修正身份或权限；404 不凭此推断跨租户对象不存在；409 读取当前版本／状态并确认冲突；429 按服务提示限流；5xx 按具体 retryable 语义处理。**写请求结果未知时先只读回查**。只有协议明确支持幂等时才用原 key 与原 payload 重放；不要换 key 来绕开冲突或重复 Git 交付。

HTTP 方法参考中的错误列表来自入口与同包辅助方法，域、数据库和网关还可返回更多错误；列表不是 exhaustive enum。Open 的稳定错误映射单独列于 [接入指南](open-rest.md)。

源码：[response](../../../seven-agent-force-server/internal/common/response/result.go)、[JSON 归一化](../../../seven-agent-force-server/internal/common/json/compat.go)、[模块挂载](../../../seven-agent-force-server/internal/app/workforce/module.go)。
