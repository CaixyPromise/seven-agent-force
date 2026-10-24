# Project-first 项目版本与能力调度

Scope: project routing, immutable versions and task input inheritance.

## 1. 产品模型

普通用户只维护项目和任务目标：

```text
Project
  -> current_version_id
  -> immutable ProjectVersion
       仓库 / 技术栈 / Agent 指引 / 完成检查
       独立审查提示词 / Git 交付 / 通知

Task
  project_id / project_version_id / instruction / review_enabled
  model_credential_id / primary_model
  selected_runtime_release_id

Assignment / Attempt
  Task 选择的 Runtime Release / 平台分配的 Worker / Hook
  冻结的模型密钥、协议与主模型
```

ProjectVersion 是内部不可变配置版本。项目编辑会新建一个 Version 并切换 `current_version_id`；没有 Draft、Release、发布或回滚流程。旧 Task 始终读取创建时锁定的 `project_version_id`，不复制通用 Project snapshot。

Project 不绑定模型密钥、模型、Runtime、镜像或 Worker。ProjectVersion 可以明确绑定有序 Hook Release；Task 自动继承该版本的 Hook，并在创建时选择一个模型密钥、主模型和不可变执行镜像版本。Assignment 只在持有所选镜像精确 digest 的 Worker 中分配运行节点；同一 Agent Session 的模型密钥、主模型、Runtime 和 Hook 身份保持稳定。

## 2. 用户操作

### 创建代码项目

项目负责人提供：

- 项目名称、说明和归属；
- 带访问令牌的 HTTP(S) Git URL 与可选默认分支；
- 技术栈和 Agent 项目指引；
- 完成检查；
- 一段独立审查提示词；
- Git 交付与通知规则。

Server 解析 URL 后只保留安全地址，并加密保存凭据。源码准备完成前不能创建新 Task。`GENERAL` Project 使用空工作区，不配置 Git Delivery。

### 发起任务

任务主输入是目标，并必须选择一个可用的“模型密钥 + 主模型”和一个“执行镜像”。镜像候选卡片一次展示准入能力、持有镜像的 Worker、实时容量和排队数量；页面自动选中推荐项，用户可以改选镜像，但不能指定 Worker。其他可选项限于只读资料、增强完成检查、交付偏好和“是否启用独立审查”。`reviewEnabled` 默认开启；关闭后本次不创建 ReviewRun。

兼容镜像的推荐顺序依次按调度优先级、可用槽位和等待任务数确定；这些条件相同时，优先较新的 Release（较大的 Snowflake ID），避免修复版本发布后仍默认选中旧版本。旧版本仍可显式选择，已创建任务继续使用其冻结的 Release ID，不因推荐顺序变化而迁移。

任务不能提交额外审查提示词、API Key、Base URL、协议、Worker、Hook 或 Git 目标。模型目录只约束创建时的选项，不是 Coding CLI 辅助调用的运行时白名单。项目技术栈只用于说明，不参与镜像筛选，也不扫描仓库推断技术栈。

### 更新项目

更新请求携带 `expectedVersionId`。Server 锁定 Project、验证当前版本、新建不可变 ProjectVersion，再切换 `current_version_id`。仓库未变化时复用源码；仓库变化时重新准备。更新只影响新 Task。

## 3. 浏览器合同

Project 更新示例：

```json
{
  "expectedVersionId": "8000000000000000004",
  "name": "Seven Agent Force",
  "reviewPrompt": "重点检查回归风险、权限边界和数据一致性。"
}
```

Project 响应使用 `versionId` 表示当前不可变版本。Task 只能在 Project 下创建：

```http
POST /workforce/projects/{projectId}/tasks
Idempotency-Key: <uuid>
```

```json
{
  "instruction": "修复分页边界问题，并补充回归测试",
  "modelSelection": {
    "credentialId": "8000000000000000005",
    "model": "gpt-5.6-luna"
  },
  "runtimeReleaseId": "8000000000000000006",
  "reviewEnabled": true,
  "deliveryPreference": "PROJECT_DEFAULT"
}
```

省略 `reviewEnabled` 时 Server 规范化为 `true`。`modelSelection` 与 `runtimeReleaseId` 必填；Server 校验镜像处于生产准入状态、支持当前模型协议、Agent 执行和 Project Hooks。当前 Worker 满载不阻止创建，Task 会锁定所选 Runtime Release 后排队。

候选由单个聚合接口返回，不按卡片发起额外请求，也不后台轮询：

```http
GET /workforce/projects/{projectId}/runtime-options?modelCredentialId={credentialId}
```

所有内部 BIGINT ID 在 JSON、URL 和 CLI 边界使用十进制字符串。

## 4. 独立审查

ProjectVersion 只提供 `review_prompt`。平台固定审查角色、安全边界、证据要求和 `review-report/v1` 报告协议；最长时间 60 分钟、瞬时重试 3 次、自动修订最多 3 次。

ReviewRun 创建规则：

- `Task.review_enabled=false`：不创建 ReviewRun，完成检查通过后继续结果与 Delivery；
- `Task.review_enabled=true`：为同一 Finalization 创建唯一 ReviewRun；
- Reviewer 输入读取 Task 锁定的 ProjectVersion 提示词、任务目标、冻结差异、完成检查和 Artifact 证据；
- 项目后续更新不影响旧 Task、修订 Turn 或运行中的 Review；
- Delivery 仍由 Review 的权威质量结果门控。

Review 使用 Task 的同一模型密钥和同一主模型。Reviewer Runtime 优先复用完成 Agent Attempt 的镜像；该镜像不支持 `REVIEW_RUN` 时，路由器选择另一同协议兼容 Runtime。平台校验 `review-report/v1` 结果，不通过模型目录能力标签预判模型是否能输出报告。

## 5. 执行镜像与 Worker 调度

Task 固定模型密钥、主模型和 Runtime Release。`executionrouting.AssignmentResolver` 不再替用户更换 Runtime；它校验所选 Release 的协议、Adapter、Hook 和准入事实，然后只在持有精确 image digest 的在线 Worker 中按容量、磁盘、最近分配时间和稳定 ID 选择节点。

暂时无资源时 Task 保持排队：

```text
WAITING_SELECTED_RUNTIME
WAITING_MODEL_CREDENTIAL
WAITING_WORKER_CAPACITY
WORKER_DISK_PRESSURE
```

所选镜像重新上线或容量恢复后 Dispatcher 自动启动；即使另一镜像空闲，也不会切换 Runtime Release。Continuation、重试和恢复继续使用相同 Release 与 digest，可以迁移到持有同一 digest 的其他 Worker。独立审查优先复用所选镜像；不支持 `REVIEW_RUN` 时才选择同协议 Reviewer Runtime。

## 6. 数据与安全边界

`workforce_projects.current_version_id` 指向 `workforce_project_versions.id`；Task 只保存 `project_id` 与 `project_version_id`。ProjectVersion 创建后不可更新。

仓库 Token、模型 API Key 和 Git credential 不进入普通 JSON、日志、对话、Agent、Adapter、Worker 或 Provider Proxy。模型调用由 Server Gateway 按 Task 身份解密当前密钥版本；Source ingestion 与 Delivery 通过 machine authority 临时物化 Git 凭据。Finalizer、检查、Review、远端 readback 和资源清理继续由各自权威模块负责。

旧 Review Profile/Rubric 表、菜单、接口和权限已硬切删除。旧非空开发库不做猜测迁移，按 hard-cut reset 规则重新初始化；升级时验证已有数据兼容性。

## 7. 证据
