# 出站 Webhook 协议

Application 可配置 Webhook URL；MCP-only 调用方可以不配置。平台向已验证的 URL 发 HTTP POST，payload 固定公共投影，不包含 executionSnapshot、Provider body、凭据或 Agent 私有历史。应用停用后待发送回调暂停。

## Header、验签与字段

| Header | 类型 | 必填／语义 |
| --- | --- | --- |
| Content-Type | string | application/json |
| X-Seven-Event-ID | string(ID) | eventId，用于去重 |
| X-Seven-Delivery-ID | string(ID) | deliveryId，发送记录标识 |
| X-Seven-Timestamp | string(decimal) | UTC Unix 秒 |
| X-Seven-Signature | string | 一个或多个 `v2=credentialId.hexSignature`，逗号分隔 |

签名公式：`hex(HMAC-SHA256(signingSecret, timestamp + "." + rawBody))`。使用收到的**原始 body bytes**，不要 JSON 解析后重新序列化；credentialId:string 用于选择签名凭据版本。轮换宽限期可同时携带 current 与 grace 签名。API Key 宽限15分钟；签名secret宽限24小时。接收方选可信版本、常量时间比较、按自身策略检查timestamp时间窗口，并校验Header IDs 与body一致。

body schema 固定 `seven.workforce.public-event/v1`：

| 字段 | 类型 |
| --- | --- |
| schema | string |
| eventId / deliveryId / sequence / tenantId / applicationId | decimal string |
| eventType / occurredAt / correlationId | string，occurredAt 为时间 |
| subject | object{type:string,id:string(ID)} |
| data | object{taskId:string,taskTurnId:string,status:string,reasonCode:string,resultSummary:string} |
| links | object{task:string,outputs:string} |

事件类型由公共事件投影产生，Task status 使用 [Open 公共状态](open-rest.md)。接收方以 eventId 做持久去重；deliveryId 是发送记录，不用它代替业务事件去重。sequence 可以帮助识别顺序，不能假设网络投递严格有序。收到 ACTION_REQUIRED 时可引导用户打开 operator 链接。

## 接收响应与重试

2xx 视为成功。408、425、429、5xx 可重试；其余4xx终止。最多6次尝试，间隔10s、1m、5m、30m、2h；Retry-After 支持有界解释，最大2小时。接收方先可靠保存事件再快速返回成功；重复事件应幂等返回2xx。签名失败不能把事件写入业务处理队列。

源码：[公共 Envelope](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go)、[签名与发送](../../../seven-agent-force-server/internal/app/workforce/openintegration/application/webhook.go)。
