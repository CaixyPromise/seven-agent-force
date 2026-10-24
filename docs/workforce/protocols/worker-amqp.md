# Worker Machine HTTP 与 AMQP

## 身份和调用顺序

Worker 注册 → 获取 QueueContract → 拉取消息 → claim 当前 assignment → 申请 launch/run permit → 上报 runtime事实 → 租约续期 → 提出 completion → Server 核验和后处理。消息只是派发提示，消费消息本身不授予执行权。

HTTP 使用 `/api/internal/workforce/v1`、Worker Bearer credential、每次机器请求的 MachineEnvelope；不是 Application Open API。实现客户端构造 `New(baseURL:string, token:string, workerID:int64, instanceID:string, hostBootID:string)`，token 至少32字符，请求timeout30s。客户端负责将 int64 编码为 string。

MachineEnvelope 的 schemaVersion:integer、workerId:string(ID)、instanceId:string、hostBootId:string、requestId:string、generation:string(int64)、ownerEpoch:string(int64，视方法需要)。generation/ownerEpoch 是并发所有权栅栏，不得自行加一绕开拒绝。GET reconciliation 使用 X-Seven-Worker-Instance:string、X-Seven-Host-Boot-ID:string、X-Seven-Request-ID:string。

全部 **86** 个方法及请求、响应、字段见 [机器 HTTP 参考](http-worker.md)，按路径分组涵盖 Worker 注册／heartbeat／image inventory／deregister／reconciliation，以及 assignment claim、runtime事实、租约／reattach、completion、interaction／execution access、evaluation、checkpoint／restore、fixture、finalization、artifact upload、ReviewRun、DeliveryAttempt、source ingestion、result materialization。不同 wrapper 的幂等 TTL 不同，不应给所有POST套统一客户端重试规则。

## QueueContract 与拓扑

服务端 register 响应返回当前 Worker 的 QueueContract。消费者使用此返回值，而不是自己猜队列名。

| 属性 | 默认契约 |
| --- | --- |
| Task exchange | seven.workforce.tasks，durable direct |
| Task queue | seven.workforce.worker.<workerId>.tasks |
| Task routing key | worker.<workerId> |
| Task dead queue | seven.workforce.worker.<workerId>.dead |
| messageSchema | seven.task-assignment/v1 |
| maxPrefetch | 默认1，配置容量时上限64 |
| Postprocess exchange | seven.workforce.postprocess |
| Postprocess queue | seven.workforce.worker.<workerId>.postprocess |
| Postprocess routing key | postprocess.<workerId> |
| Postprocess dead queue | seven.workforce.worker.<workerId>.postprocess.dead |

Task queue TTL2分钟、max-length32；postprocess TTL10分钟、max-length128；溢出reject-publish-dlx。Outbox publisher 提供 message ID 与有界 expiration。消息确认必须依据 Worker handler 的当前 authority 和处理结果；不能因为投递成功就更新 Task 成功。

## 消息方法与字段

AMQP 消息 body 为 JSON，不使用 HTTP Result 包装。schemaVersion 为 integer（当前1）；所有实体 ID、generation、ownerEpoch 为 decimal string；kind 是 string。各类型字段表见下面链接。

| kind / schema | 消费用途 | body 类型 |
| --- | --- | --- |
| seven.task-assignment/v1 | Agent assignment | [`internal/app/workforce/taskdispatch/domain.AssignmentMessage`](wire-types.md#t-8353cfb14d51a576) |
| seven.review-run-assignment/v1 | 独立 ReviewRun | [`internal/app/workforce/reviewrun/domain.AssignmentMessage`](wire-types.md#t-9c051df92d406a86) |
| seven.finalization-run/v1 | 核验与冻结结果 | [`internal/app/workforce/artifact/domain.WorkMessage`](wire-types.md#t-9b5dad55bac0c0ad) |
| seven.artifact-upload-attempt/v1 | 上传已冻结结果 | [`internal/app/workforce/artifact/domain.WorkMessage`](wire-types.md#t-9b5dad55bac0c0ad) |
| seven.result-materialization-run/v1 | checkpoint restore／结果按需物化 | [`internal/app/workforce/continuation/domain.MaterializationMessage`](wire-types.md#t-88e8ed267999e10b) |
| seven.delivery-attempt | Git 交付 | [`internal/app/workforce/delivery/domain.Message`](wire-types.md#t-b89dd47cf7d38740) |
| seven.source-ingestion-run | 导入 Project 源码 | [`internal/app/workforce/workspaceinput/domain.SourceIngestionMessage`](wire-types.md#t-fb0895871c599452) |
| seven.result-processor-fixture-run | Processor 发布 fixture | schemaVersion:integer、kind:string、messageId/workerId/runId/generation:string |

普通 postprocess WorkMessage 共同字段：schemaVersion:integer、kind:string、messageId/workerId/resourceId/generation:string。Fixture 用 runId 替代 resourceId。Task assignment 另有 assignmentId/taskId/executionNodeId/workerId/generation:string、executionRequirementDigest:string、expiresAt:string；Review assignment 另有 reviewRunId/workerId/ownerEpoch:string、expiresAt:string。不要将不同消息的 resourceId、assignmentId、runId 混用。

后处理失败重试使用原冻结输入，**不能重跑 Agent**；相同队列上不同 kind 应分派到现有 handler。未知结果先从机器 HTTP 查回当前 lease/claim/receipt，然后决定 ack、拒绝或恢复。

源码：[机器客户端](../../../seven-agent-force-server/internal/worker/control/client.go)、[QueueContract](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go)、[拓扑挂载](../../../seven-agent-force-server/internal/app/workforce/module.go)、[Worker 消费入口](../../../seven-agent-force-server/cmd/seven-worker/main.go)。
