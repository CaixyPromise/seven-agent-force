# Workforce 协议开发与接入文档

这套文档描述随附源码的接口约定。结构覆盖与本地测试不等于生产环境或真实 Provider 验收。

## 按开发目的选择入口

| 开发目的 | 使用协议 | 方法与参数参考 |
| --- | --- | --- |
| 普通外部系统提交、查询和获取结果 | Open REST / HTTP JSON，Application API Key | [接入指南](open-rest.md)、[全部 7 个 HTTP 方法](http-open.md) |
| MCP AI 客户端调用同样的任务能力 | MCP 2026-07-28 / Streamable HTTP，无会话 | [6 个 Tools 与 JSON-RPC 方法](mcp.md) |
| 外部系统被动接收任务变化 | HTTP Webhook / HMAC-SHA256 v2 | [回调 Header、payload、验签与重试](webhook.md) |
| 开发 operator console、任务操作和资产管理 | Framework 用户鉴权的管理 REST | [全部 134 个 HTTP 方法](http-operator.md) |
| 实时获取任务事件 | HTTP SSE | [连接参数、事件与重连](sse.md) |
| 扩展 Worker 与 Control Plane | Worker Machine HTTP JSON + RabbitMQ/AMQP | [生命周期与消息](worker-amqp.md)、[全部 86 个机器 HTTP 方法](http-worker.md) |
| 扩展 Agent 执行环境／适配原生 Agent | stdio UTF-8 JSONL `seven.agent-adapter/v1` | [25 个双向方法](agent-adapter.md) |
| 编写 Python Adapter／Processor | Python Runtime SDK | [公开方法、参数、dataclass](python-sdk.md) |
| Worker 控制 Attempt 内 Supervisor | Unix Domain Socket JSONL + journal | [控制命令、请求与回执](supervisor.md) |
| 开发结果检查、问题和输出策略 | Python Result Processor / `seven.result-processor` | [process(context)、manifest 与返回契约](result-processor.md) |
| 开发可信执行 Hook | Hook package / `seven.hook-package/v1` | [7 个事件、manifest、stdin/stdout](hooks.md) |
| 模型转发与调用审计 | Provider Gateway / Responses 与 Messages | [4 个 HTTP 操作、身份 Header、透传](provider-gateway.md)、[HTTP 参考](http-provider.md) |
| 对接原生 Codex／Claude | 镜像内 Adapter 对原生 SDK 的映射 | [当前映射与扩展位置](native-adapters.md) |
| 任务通知、通道扩展 | 站内通知、SMTP、企业应用与HTTP Connector | [消息与Driver方法参数](notifications.md) |
| 企业身份／第三方登录扩展 | Framework继承的OAuth/OIDC与企业身份 | [Adapter方法与参数](identity.md) |
| Git 结果交付、自动化运维 | Git HTTP(S)/SSH + GitLab/GitHub/Gitea API；CLI | [Git 交付边界](git-delivery.md)、[CLI 参数](cli.md) |

## 阅读约定与覆盖

先读 [公共约定](common.md)。HTTP 参考页每个方法包含 URL、Handler、鉴权／权限、path/query/header/form 参数、请求体字段、响应类型、入口错误以及源码位置。嵌套结构和枚举见 [515 个 Go 类型索引](wire-types.md)；也提供 [机器可读接口清单](contracts.json)。Python SDK 与 Agent payload 是另一层协议，不要拿机器 HTTP DTO 直接作为 Adapter payload。

HTTP 合计 **231 个已挂载 Workforce 方法 = 7 Open + 134 operator + 86 Worker + 4 Provider**。计数按方法与路径对算，包含下载和 SSE，不包含继承自 Framework 的登录、SSO、RBAC、菜单、通用文件等接口。Framework 由原模块处理；Workforce 没有另设登录协议。旧 `executionaccess/controller/machine.go` 未被当前模块挂载，已排除，不能据其文件推断 API 可用。

不是所有对象都是对外稳定接口：Open／MCP 是外部接入面；operator、Worker、Supervisor 是内部产品与运行时契约，升级应同时核对生产者、消费者和持久化。Provider body 是透传 JSON，具体模型支持由配置决定。本项目没有对外发布 Attempt HTTP/gRPC 服务，也没有公开 Open Application 回答／取消任务的工具。

## 再生成与维护

[生成工具说明](tools/README.md)说明如何从源码重建路由、字段、类型和 SDK 签名。更新协议时同步改实际实现、对应指南、生成参考以及相关契约测试；不要手改生成页来掩盖实现差异。架构和开发入口见 [Workforce 文档总目录](../README.md)。
