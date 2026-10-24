# 受控网络命令：设计、接口与运维合同

> 范围：当前受控网络访问接口与运维流程。

## 1. 它解决什么问题

Agent 的普通执行容器没有默认公网出口。需要访问依赖仓库时，支持 Dynamic Tool 的 Adapter 可以向平台提出一个具体请求：在仓库工作副本中运行一条命令，并只访问列出的 `host/protocol/port`。

用户只决定是否允许这一次，或允许当前任务后续访问同一目标集合。Agent 不获得 Worker、Docker、Provider 或平台凭据，也不能自行把工具请求变成授权。

本能力不复用模型调用的 `seven-provider-proxy`。模型调用只能使用 Task 绑定的模型密钥和固定上游地址；项目网络访问由 `executionaccess` 和目标固定出口执行。

## 2. 固定工具合同

工具名：`run_command_with_network_access`

```text
Run a shell command that requires outbound network access. Call this tool
whenever the task container cannot reach a required external host; do not
retry the command through the normal shell while this call is pending.
Provide the purpose, exact shell command, repository-relative working
directory, and every required host, protocol, and port. The platform may ask
the user to allow this operation once or for the rest of the task.
```

输入示例：

```json
{
  "purpose": "安装项目锁定的依赖，以便运行完成前检查",
  "command": "pnpm install --frozen-lockfile",
  "cwd": ".",
  "targets": [
    {"host": "registry.npmjs.org", "protocol": "https", "port": 443}
  ]
}
```

约束：

- `purpose`：1–2048 bytes；会进入安全聊天投影，因此拒绝明显凭据格式。
- `command`：1–16384 bytes 的完整 shell 字符串；只保存在加密 descriptor 中。
- `cwd`：仓库相对路径，最多 4096 bytes；规范化后不得逃逸工作区。
- `targets`：1–8 个；仅 `http`、`https` 和明确端口。
- host 必须是规范 DNS 名；拒绝通配符、IP 字面量、userinfo、URL path、重复 host、私网、loopback、link-local、metadata 和控制面目标。

工具定义由 Server 冻结，并取 Server、Execution Profile、精确镜像 manifest 和 Adapter `ready` 能力的交集。Task prompt、仓库文件和环境变量不能修改名称、描述或 schema。

## 3. 协议与可信边界

统一 `seven.agent-adapter/v1` 只增加工具信封：

```text
Adapter → Supervisor  tool.request
Supervisor → Adapter  tool.result
Adapter → Supervisor  tool.received
Adapter → Supervisor  tool.closed
```

occurrence identity 为 `(attemptId, adapterSessionGeneration, localToolCallKey)`。相同 identity 和参数 digest 幂等复用；相同 identity、不同 digest 冲突。Worker 重连只恢复观察和投递，不重发 `execution.start`，也不创建第二个 Operation。

Codex `0.147.0` Adapter 将冻结工具注册为 App Server `dynamicTools`，把 `item/tool/call` 映射为 `tool.request`。Dynamic Tool experimental capability 不可用时返回 `CODEX_DYNAMIC_TOOL_UNAVAILABLE`，不会退化成普通 shell approval。其他 Agent 可以在自己的镜像 Adapter 中产生相同协议消息；SDK 是实现辅助，不是接入前提。

以下事实分开保存：

```text
用户已决定
→ 命令已执行并保存结果
→ 结果已送达 Adapter / 原生调用已解决
→ Agent 在后续公开活动中继续
```

`execution.finished`、`tool.received` 或容器退出均不等于 Task 成功。Task 最终状态仍由完成前检查、Finalizer、必要 Review、Artifact 和 Delivery 门控决定。

## 4. 用户 API

所有 ID 和 version 均使用规范 decimal-string。接口沿用组织范围、Task 授权和现有 Cookie/Bearer 认证；跨租户资源统一返回 not found。错误响应只返回稳定 code，不包含数据库、Worker 或 Provider 原因。

### 4.1 读取 Operation

```http
GET /workforce/tasks/{taskId}/execution-operations/{operationId}
Permission: workforce:task:query
```

返回安全状态、purpose、targets、工作目录、完整命令、结果摘要及 digest。完整命令只通过这个 Task-authorized 详情接口读取，不进入普通 conversation item 或日志。

### 4.2 决定 Operation

```http
POST /workforce/tasks/{taskId}/execution-operations/{operationId}/decisions
Permission: workforce:task:continue
Content-Type: application/json
```

```json
{
  "expectedVersion": "3",
  "action": "ALLOW_ONCE",
  "reason": "",
  "idempotencyKey": "client-generated-key"
}
```

`action` 仅允许 `ALLOW_ONCE`、`ALLOW_FOR_TASK`、`DENY`。同幂等键和同正文返回既有决定，并以 `Idempotency-Replayed: true` 标识；同键不同正文返回 `EXECUTION_OPERATION_IDEMPOTENCY_CONFLICT`。并发或旧页面使用过期 version 返回 `EXECUTION_OPERATION_STALE`。

`ALLOW_FOR_TASK` 创建当前 Task 的精确目标集合 Grant。它只使后续请求在 `requested targets ⊆ active grant targets` 时自动进入执行；每条命令仍有独立 Operation、结果和审计记录。

### 4.3 查询和撤销 Task Grant

```http
GET  /workforce/tasks/{taskId}/network-grants
POST /workforce/tasks/{taskId}/network-grants/{grantId}/revoke
```

查询需要 `workforce:task:query`。撤销需要 `workforce:task:continue`，请求为：

```json
{
  "expectedVersion": "1",
  "idempotencyKey": "client-generated-key"
}
```

撤销后不再允许新的 claim；Worker 同步撤销并终止相关 relay/连接。Task 取消或终态也会关闭 Operation 并使 Task Grant 失效。

### 4.4 稳定错误码

```text
EXECUTION_OPERATION_INVALID
EXECUTION_OPERATION_NOT_FOUND
EXECUTION_OPERATION_STALE
EXECUTION_OPERATION_ACTION_NOT_ALLOWED
EXECUTION_OPERATION_IDEMPOTENCY_CONFLICT
EXECUTION_OPERATION_TARGET_DENIED
EXECUTION_OPERATION_OUTCOME_UNKNOWN
EXECUTION_OPERATION_FENCED
EXECUTION_OPERATION_UNAVAILABLE
TASK_NETWORK_GRANT_REVOKED
TASK_NETWORK_GRANT_EXPIRED
```

Web 必须按 code 显示安全反馈，不能渲染任意后端 `message`。

## 5. Worker machine API

以下接口只接受 Worker machine auth，浏览器、Adapter 和 Agent 均不能直接调用：

```text
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/claim-next
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/pending-suspend
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/{operationId}/start
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/{operationId}/renew
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/{operationId}/result
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/{operationId}/result-receipt
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/results/pull
POST /internal/workforce/v1/attempts/{attemptId}/execution-operations/{operationId}/result-ack
```

冷恢复使用同一业务 Operation 和以下专用路径：

```text
POST .../{operationId}/cold/claim
POST .../{operationId}/cold/start
POST .../{operationId}/cold/renew
POST .../{operationId}/cold/result
POST .../{operationId}/cold/result-receipt
POST .../{operationId}/cold/readback
POST .../{operationId}/cold/result-ack
```

每次调用都重新核对 Worker ID、instance、host boot、Attempt generation、owner epoch、执行 lease、Adapter session generation、Task/Turn/Attempt、Operation version、Task 取消/终态和 Grant 撤销状态。冷路径额外绑定已消费的 `INTERACTION_SUSPEND` Checkpoint 中唯一的 source Operation；它不是绕过正常 authority 的通用入口。

## 6. 执行与出口拓扑

```text
Codex / Agent Adapter
        │ tool.request
        ▼
Supervisor → Worker spool → Server executionaccess
                                │ 决定或已有 Task Grant
                                ▼
Operation runner ── internal network ── target-egress-gateway
      无默认公网路由                         │ 固定公网 IP:port
      无 Adapter/Provider/Docker socket       ▼
      无 Worker/Provider/Git secret      approved hostname
```

Worker 在 runner 创建前登记资源 journal。每个 approved host 创建短生命周期 relay，并把该次 DNS 解析得到的公网 IP 固定为实际 dial target。runner 通过 network alias 使用原 hostname，所以 TLS SNI 和证书验证仍由客户端完成；未批准的 redirect/CDN host 没有 alias 和出口。

Operation 同一时间至多一个写入当前 Attempt 工作区。Dynamic Tool 未返回前，Codex 原 turn 等待，避免 Agent 和 runner 并发修改同一工作区。

若 Operation 在当前工作区准备了顶层 `node_modules`，CURRENT 完成检查会将该目录作为只读依赖输入挂入自己的无网络私有工作区。检查代码和构建输出仍在 tmpfs 中执行，依赖不能回写；`node_modules`、pnpm symlink 和缓存均不会进入可信 ResultTree。BASELINE 不借用当前依赖，以免把修改后的运行条件伪装成原始基线。该首期能力只覆盖已选验收项目需要的 Node/pnpm 条件，不应描述为通用环境编排。

独立 Reviewer 仍只读取冻结 ResultCommit 与审查输入，不自动继承 Operation 的临时依赖。若 Reviewer 在自己的离线沙箱重跑依赖型命令，它可以如实报告环境限制；平台必须保留 canonical verdict，并通过现有修订或人工闭环处理，不能用可信完成检查结果改写 AI verdict。

## 7. 结果、恢复和清理

runner 先把受限结果写入 Worker 私有目录，Worker readback 后提交 Server。Agent 可见输出上限 64 KiB；Worker 本地临时输出上限 8 MiB。数据库保存加密结果、digest 和安全摘要，普通聊天仅显示安全投影。

提交响应丢失时，Worker 用同一 Operation、Attempt 和 result digest 调用 `result-receipt`。Server 已提交同 digest 时继续投递和清理；若容器可能执行但结果不可恢复，则落为 `OUTCOME_UNKNOWN`，禁止自动重跑。

等待用户超过热等待期限时，复用 `INTERACTION_SUSPEND`：保存 thread/workspace 后清理 Agent 和 Provider Proxy。用户决定后只创建一个后继 Turn，先执行或拒绝原 Operation，再用 standalone tool output 恢复 Codex thread；不复用旧 JSON-RPC request ID，也不创建第二张用户卡。

清理顺序为容器、专属卷、专属网络、临时目录。每步使用独立有界 context，确认资源不存在后才释放容量；失败保留 `CLEANUP_PENDING` 并重试。禁止全局 prune，Review、Artifact、Checkpoint 和 Delivery 引用资料按既有保留合同处理。

## 8. 开发和故障排查

Adapter 作者应使用 `seven-agent-force-runtime check` 验证 schema、方向、大小限制和重复消息，再把工具调用映射到原生 Agent。SDK 的 `context.request_tool(...)` 只发送协议消息并等待结果，不访问 Server、不执行命令、不授予网络。

排查顺序：

1. `tool.request` 是否通过 capability 交集和 descriptor 校验；
2. Operation 是否处于 `WAITING_APPROVAL`、`READY`、`DISPATCHED` 或 `RUNNING`；
3. 当前 Task Grant 是否精确覆盖 targets，是否已撤销或过期；
4. Worker authority、Operation attempt lease 和 resource journal 是否一致；
5. runner/relay 是否只在专属网络，实际 dial IP 是否为批准时冻结值；
6. Server 是否已保存 terminal result，Adapter 是否已 ACK native resolved，是否已有后续公开活动证明 Agent continued；
7. 终态后 runner、relay、network 和临时目录是否均已确认不存在。

不要把 Provider Proxy 请求、Agent 普通 shell 日志、conversation 文案或容器退出码当作 Operation 权威状态。
