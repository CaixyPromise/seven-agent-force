# Seven Agent Force 二次开发指南

面向扩展 Server、Web 或业务模块的开发者。Runtime Adapter 和 Python SDK 的开发步骤见 [SDK 开发指南](sdk-development.md)，外部系统调用见 [协议索引](workforce/protocols/README.md)。

## 1. 环境与启动

使用 go.mod/go.work 指定的 Go 版本，Node.js 20.19+、pnpm 10；SDK 示例需要 Python 3.11+。先阅读 [配置](configuration.md) 和 [部署](deployment.md)。完整任务流依赖 PostgreSQL、Redis、RabbitMQ、对象存储和独立 Linux Docker Worker。仅启动 API/Web 不代表已经具备任务执行环境。

在仓库根目录运行离线源码检查：

```sh
make web-install
make check
node --test seven-agent-force-web/tests/*.test.mjs
```

安装依赖会访问包源。没有配置独立集成环境的数据库测试可能跳过；阅读测试输出，不能把跳过记作通过。

本地服务配置应从 `seven-agent-force-server/configs/application.example.yaml` 复制到自己管理的配置目录，按文档提供数据库、加密密钥和其他依赖。示例默认关闭多项集成，不是完整可运行的业务配置。使用正常初始化/登录流程，不手写用户、任务或权限数据库记录。

配置完成后，分别启动（以下是开发者操作说明，本指南本身不执行服务或迁移）：

```sh
make -C seven-agent-force-server run CONFIG_DIR=/absolute/path/to/local-config
pnpm --dir seven-agent-force-web dev
```

Web 默认 `http://127.0.0.1:5177`，代理 API 默认 `http://127.0.0.1:9277`；自定义 API 地址通过 `VITE_DEV_API_TARGET` 设置。与已有服务冲突时选择自己的端口和配置，不能直接替换他人的进程。生产部署另按部署文档处理。

## 2. 按业务路径定位代码

| 关注点 | 位置 |
| --- | --- |
| 业务模块 | `seven-agent-force-server/internal/app/workforce/` |
| 依赖装配和真实路由挂载 | `seven-agent-force-server/internal/app/workforce/module.go` |
| Worker 协议和执行 | `seven-agent-force-server/internal/worker/` |
| Web API/边界归一化 | `seven-agent-force-web/src/api/` |
| 页面 | `seven-agent-force-web/src/app/workforce/` |
| 自动路由与权限可见性 | `src/autoRoutes.tsx`、`src/router.tsx`、`src/lib/navigation/`（Web 目录下） |
| 数据迁移和 SQL 治理 | `seven-agent-force-server/migrations/` |

模块分层：controller/listener → application → domain；infrastructure 实现存储和外部适配。跨模块使用 facade，不直接访问另一模块私有存储。业务状态变更放在 domain/application，不能由数据库适配或浏览器自行决定。

## 3. 贯穿示例：扩展项目列表筛选

以现有搜索功能作为可追踪示例，再在自己的分支扩展所需筛选。这里不新增一个不存在的 API，也不要求直接修改业务数据。

1. 打开 [项目页面](../seven-agent-force-web/src/app/workforce/projects/page.tsx)。`search` 经过去空格和延迟后传给 `listProjects`；React Query key 同时包含状态和搜索值。新增筛选时，先规定字段语义、空值行为、分页边界，以及筛选改变后缓存如何隔离。
2. 查看 [Web 请求适配](../seven-agent-force-web/src/api/projectController.ts) 与 [项目合同](../seven-agent-force-web/src/api/projectContract.ts)。`GET /api/workforce/projects` 已传递 query、status、cursor、limit；DTO、序列化和响应 normalizer 应一起调整。不要给 int64 ID 调用 Number/parseInt。
3. 查看 [Controller](../seven-agent-force-server/internal/app/workforce/project/controller/http.go)、[应用服务](../seven-agent-force-server/internal/app/workforce/project/application/service.go) 和 [存储](../seven-agent-force-server/internal/app/workforce/project/infrastructure/store.go)。跟踪真实参数校验、认证组织范围、权限、分页和 SQL 方言。新增条件必须与租户约束同时生效；不能在 Web 端过滤当前一页后声称完成服务端筛选。
4. 若扩展响应字段，同步领域投影、响应 DTO、Web 类型和 normalizer；若扩展独立写操作，还需定义幂等键、版本冲突和错误展示。不要用一次读操作的示例绕过写操作治理。
5. 新增页面时遵循现有 page 自动扫描约定，同时核对菜单、权限和运行时功能策略。按钮隐藏不替代服务端鉴权。不要仅向 router.tsx 手写一条路径就认为页面接入完成。
6. 用合成输入验证：空筛选、非法状态、超长字符串、权限不足、不同租户、翻页以及大 ID 字符串。Web 验证加载/空列表/失败/刷新和筛选切换。只读筛选测试无需真实 Provider 或新建执行任务。

建议先运行相关小范围检查，再扩大：

```sh
cd seven-agent-force-server
go test ./internal/app/workforce/project/... -count=1
cd ..
node --test seven-agent-force-web/tests/*.test.mjs
make web-check
```

Server 行为改动较广时运行 `make server-check`，跨模块/发布相关改动按根目录规则运行 `make check`。页面视觉修改需要在实际页面验证桌面/窄屏及相关状态；编译或静态测试不能代替页面验证。

## 4. 数据与安全边界

内部实体/关系键使用 PostgreSQL BIGINT 与 Go int64，新增实体通过注入 IDGen 分配；JSON/URL/队列/Web 使用十进制字符串。tenant_id 来自认证组织范围，不从请求体自行信任。数据库不加外键，由应用验证关系与事务一致性。

新增迁移前先确认当前迁移是否已发布；遵循根 AGENTS.md 的未发布迁移策略。需要本地对齐时提供事务化、幂等、可验证路径；本文不授权对任意现有数据库执行迁移。不要将管理员密码、APIKey、DSN 或生产样例写入测试、截图和文档。

Agent 的完成消息不是可信成功；Finalizer 在 Harness 之外重算结果。任务成功、独立 Review 和 Git Delivery 是不同状态。后处理/审查/交付重试不能重新执行 Agent。

## 5. 交付开发变更

更新用户行为、错误提示、配置和受影响的接口文档。将单元/合同测试、数据库集成、浏览器验证、真实 Worker/Provider 验证分开报告；未执行项写明原因。提交应围绕完整可审阅行为，发布、迁移和推送遵循仓库授权规则。

项目 AI 开发引导位于 [.agents 二次开发 Skill](../.agents/skills/seven-force-development/SKILL.md)。它和本指南互相补充；Skill 不会自动部署或创建业务数据。
