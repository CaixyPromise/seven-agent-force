# AgentAdapter 操作维护手册

适用当前统一 Adapter 运行链；本文为操作流程，不声明当前任何本地进程仍在线。源码接入见[开发指南](agent-adapter-developer-guide.md)。

## 1. 维护前确认对象

协议字段和消费者边界见[接口参考](agent-adapter-interface-reference.md)，尤其第8节：Go已支持当前进程内的匹配重复回执和已完成结果重放，但不能替代跨进程恢复。重连使用现有ExecutionSession，不通过重发start排障。

先确定目标环境、Task/Attempt 或 ReviewRun、Worker、镜像 digest 和操作权限。只读确认 API 健康、Worker 在线/能力、Docker 可达和现有资源 journal。不要混用其他环境的端口、数据库或凭据。

运行入口为 `seven-worker-cli supervise`；Supervisor 从已准入 manifest 加载 Adapter。Worker 经 `control --stream` 连接私有 UDS，后者只转接，不是另一个业务服务。无需为每个容器配置 HTTP/gRPC 端口或开放 Docker socket。

## 2. 发布、升级与回退

1. 固定 Adapter、SDK、原生 CLI 和依赖版本，完成本地协议/取消/恢复测试。
2. 构建镜像，保存源码版本、构建工具、精确镜像 digest、manifest 及源码/SDK 索引摘要。
3. 按[现有运行手册](mvp-runbook.md)完成镜像准入、Worker 镜像就绪和 Harness 配置；能力声明必须与实际 ready 一致。
4. 用隔离 Fake 任务验证启动、活动、检查、结果读取及清理，再将新配置用于新任务。真实 Provider 测试需要额外授权和有界预算。
5. 回退时让新任务选回已验证且满足当前协议的精确镜像；不能改写运行中或历史 Task 的 frozen digest，也不能退回不支持新协议的旧镜像作为 fallback。

SDK/Processor 升级应同步 bootstrap、内置模板和 runtime/module 身份。历史 Release 保持不变；若不兼容，应新发布匹配资产，而非替换历史内容。不要仅修改镜像 tag 或手填摘要修复准入。

## 3. 常见症状与处理

| 症状 | 优先核对 | 安全处理 |
| --- | --- | --- |
| 入口未启动 | manifest executable/args、执行权限、入口及源码摘要 | 修复构建并重新准入；不从 Task 注入启动命令 |
| ModuleNotFoundError | `-I` 下包是否安装、受保护路径是否显式加入、依赖是否打包 | 按开发指南修镜像；不导入仓库路径、不关闭隔离来掩盖问题 |
| 握手不匹配 | 协议范围、ready 能力、SDK与CLI组合 | 拒绝启动并修声明或实现；不假装支持在线交互 |
| 非法帧/活动缺失 | stdout 污染、帧尺寸、安全投影、缓冲缺口 | 限制并过滤原生事件；不得把控制通道变成无限日志流 |
| Worker 断线但容器仍运行 | Server owner/session、lease、Supervisor round/stage | 使用既有 ExecutionSession 恢复；不再次 start Agent |
| 原生进程丢失 | 权威状态、原生 Session、可靠 Checkpoint | 走现有恢复或失败路径，不将连接重试当进程恢复 |
| WAIT_USER 确认丢失 | 当前 cycle、EVALUATION_PENDING_ACK、Server 已保存决定 | 查询并重放原决定；不在 REVIEWING 下重新物化或重跑检查 |
| 任务结束但容器残留 | 精确资源 journal、删除错误、Docker 可达性、实际归属 | 保留占用并重试；不能以业务成功代替清理成功 |
| Worker 不再接任务 | 实际资源占用、清理待办、Docker未知状态、镜像能力 | 先恢复可证明的容量，不调高上限或清空 journal 绕过保护 |
| 网络 Operation 一直等待 | Operation version/expiry、Task 权限、是否已有精确 Grant | 由聊天卡决定；不得直接改数据库为 READY 或在 Agent 容器里开放公网 |
| 网络 Operation 结果未知 | Worker 私有 `started.json/result.json`、Server terminal receipt、descriptor/result digest | 同 digest readback；缺少可证明结果时保持 `OUTCOME_UNKNOWN`，禁止自动重跑 |
| Grant 已撤销但 runner 仍在 | Operation Attempt lease、renew fencing、journal 中 runner/gateway/network | 撤销会取消关联 Operation；Worker 下次 renew 必须终止执行并精确清理，不能等命令自然结束 |

不要将以上诊断标签视为承诺存在的 HTTP 错误码。用户页面使用现有安全错误映射；运维诊断只记录安全原因，不输出完整环境或原始 Provider 内容。

## 4. 等待、重连、取消与重启

- 控制连接丢失：先由 ExecutionSession 重新读取身份、round 和阶段，关闭旧读写任务后继续观察；没有权限重新发起同一原生执行。
- authority/watchdog 失效：中止执行，不能靠无限重连延长授权期限。
- `EVALUATION_PENDING_ACK`：先查权威决定，再重放；若需提交已有候选，保持当前 cycle 幂等。不重新领取秘密材料。
- WAIT_USER：只有 Checkpoint 可靠保存才可宣称可恢复等待；等待不保留无执行用途的容器。
- 取消：通过现有平台取消入口；旧 owner、过期 lease 和已取消执行的恢复应被拒绝。容器结束与业务取消分别核对。
- Worker 重启：先按 journal/精确标签盘点，再由 Server 决定恢复或清理。不要启动第二个竞争执行或手删活跃 journal。

当前本地证据包括控制断线重连、WAIT_USER 丢 ACK 恢复、删除失败重试和 Worker 重启后的终止资源清理；不是任意崩溃时刻或跨主机恢复的完整证明。当前活动缓冲有界，观察缺口必须明确，不是持久化聊天记录。

## 5. 资源释放核对清单

业务状态成立后，仍需逐项核对：

1. 用实际运行身份关联 Agent/Reviewer、Provider proxy、检查容器和本次专属临时资源；不接受容器任意指定删除路径。
2. 检查 journal 中创建意图与 Docker 实际对象，处理创建成功但尚未登记 ID 的窗口。
3. 清理采用独立 30 秒 context，先停止并删除容器，再处理无引用的专属临时卷、网络及文件。
4. 每一步确认实际不存在；不存在视为幂等成功。删除失败保留 CLEANUP_PENDING、安全原因、退避重试及容量占用。
5. Docker 不可达或归属不明时 fail closed。只有确认容器不存在后核对释放实际占用。
6. 回读结果、Review、Artifact、Checkpoint/Continuation 和 Delivery 仍引用的资料，确认未误删。
7. 验证报告分别列出“业务完成”“临时执行资源已释放”“按保留策略继续保留的资料”。

禁止全局 prune、广域目录删除、删除共享卷、清空 journal 假装恢复容量。需要人工删除时先核定精确目标及引用，再另行取得授权。既有 Custom Delivery 直接执行路径尚未全部纳入本批 journal，不能据此宣称所有遗留资源已被统一管理。

## 6. 受控网络 Operation 运维

模型调用与项目外网使用两个隔离的数据面：`seven-provider-proxy` 只把当前任务固定模型密钥的 OpenAI/Anthropic 请求转发到固定上游，绝不执行 shell；网络 Operation 使用 Worker 启动的短生命周期 `target-egress-gateway`。不要为了排障把模型代理改成任意 URL 代理，也不要给 runner 增加默认 bridge 网络。

运行链：

```text
tool.request → Server Operation/Decision/Grant
→ Worker claim → 准备 runner 与精确 target relay
→ Server RUNNING ACK → 启动 runner
→ Worker 本地 readback → Server 保存结果
→ tool.result → Adapter/native call → 后续公开活动
```

关键观察：

1. `RUNNING` 之前，Server 保存的 container/relay identity 必须来自 Docker 实际创建结果，不得使用预测名称。
2. runner 只在 operation internal network，使用批准 hostname 到 relay 内网 IP 的确定映射，并禁用普通 DNS；relay 另接 outbound network并固定解析后的公网 IP:port。
3. runner 不挂载 Adapter/Provider socket、Docker socket、Worker 私有 journal/result root或平台凭据。完整命令只在加密 descriptor 和授权详情中可读。
4. 同一 Attempt 只允许一个 workspace-writing Operation。Task Grant 只覆盖精确 target 子集；命令变化仍产生新 Operation。
5. Grant 撤销、Task取消、owner/lease变化通过每秒 renew fencing终止 runner和已建立relay连接。确认容器和网络不存在后才释放容量。
6. Worker提交结果响应丢失时，使用 `operationId + operationAttemptId + resultDigest` 查询 terminal receipt；确认同一结果后清理，不再次执行。已经启动但没有可信 result文件时持久化 `OUTCOME_UNKNOWN`。

本地 Docker 验收应同时证明批准 hostname 可达，未批准 DNS、错误端口、host alias、IP直连、私网和metadata不可达，并检查 runner无法看到Worker私有结果根。测试结束后按Operation精确标签核对 runner、gateway、internal/outbound network和结果目录均不存在。

## 7. 授权 Git 交付运维

`request_git_delivery` 与 `run_command_with_network_access` 共用 Adapter 工具信封，但不共用业务执行器。前者只保存 Delivery Intent，并立即返回 `SCHEDULED`；Git 写入仍由既有 DeliveryRequest/DeliveryAttempt 在可信结果冻结和门控通过后执行。不要让 Agent 容器、Adapter 或网络 Operation runner 持有 Git credential，也不要把 Provider Proxy 或项目网络 relay 改成 Git 出口。

运行链：

```text
request_git_delivery → Delivery Intent/SCHEDULED
→ Agent 结束 → Finalizer/检查/Review
→ 唯一 DeliveryRequest
→ AUTO_FROM_TASK_PLAN 或用户 DELIVER/SKIP_THIS_RESULT
→ non-force push → remote ref readback
→ 可选 Draft PR → PR readback
```

运维排障和真实验收必须按 [授权 Git 交付运行手册](authorized-git-delivery-runbook.md) 执行。尤其区分 Task 成功、Intent 已保存、DeliveryRequest 已授权、push 已返回、remote ref 已核验和 Draft PR 已核验；任何较早事实都不能替代较晚事实。真实交付应在目标环境验证。

## 8. 变更后的最小验证

| 改动 | 必须补的证据 |
| --- | --- |
| 文档 | 链接/命令核对及 diff whitespace 检查 |
| Adapter 事件映射 | 固定原生事件 fixture、安全投影、协议方向与尺寸 |
| SDK/打包/入口 | Python测试、多文件导入与身份校验、真实镜像握手 |
| 控制连接/恢复 | 不重复启动、取消、旧owner、丢ACK重放、后台循环退出 |
| 资源收尾 | 删除失败占用不释放、重试后实际不存在、结果资料仍可读 |

先跑受影响 focused/race 测试，再按变更范围运行仓库 Gate；关键数据库测试显式设置隔离 DSN 并确认未跳过。无 schema 变化不重复 alignment。模型参数构造或本地 help 检查不能替代真实模型业务；不得为文档维护隐式调用 Provider。

维护记录至少包含源码基线、变更范围、安全身份、命令退出码、业务与资源观察、未执行项。禁止记录密码、Token、TOTP、完整环境、原始 Provider body 或内部推理。不自动提交、推送或部署。
