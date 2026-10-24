# AgentAdapter 开发与镜像接入指南

范围：当前统一 `seven.agent-adapter/v1`、Python SDK、镜像入口及平台工具映射。SDK 可选，协议是接入要求。源码检查、模拟测试和真实集成分别验证；本文不代表 SDK 已发布到公共 PyPI，也不替代真实模型、受控网络或远端 Git 验收。

## 1. 谁负责什么

逐字段消息合同、Python方法签名、AGENT/REVIEW输入、时序及错误处理见[接口参考与执行原理](agent-adapter-interface-reference.md)。开发者应先确认第1节必需实现，再按本文打包接入；仅看示例不足以证明业务合规。

| 所有者 | 职责 | 不负责 |
| --- | --- | --- |
| 镜像作者的 Adapter | 启动原生 Coding Agent、转换事件、原生 Session 恢复、中断子进程 | Task 成功、授权、修订额度、可信验收 |
| Supervisor | 入口校验、进程监督、通用协议、检查与 Processor 协调、完成提案 | 按品牌解析原生事件 |
| Worker | 执行身份、控制连接、恢复判断、资源记录与清理 | 从容器消息产生用户批准 |
| Server | 权威业务状态、权限、检查决定、Finalizer/Review/Delivery 门控 | 信任 Agent 自报成功 |

用户仍通过任务创建入口提出目标；无需配置 Adapter 源文件。镜像作者通过现有镜像准入和 Harness 发布配置接入，不新增 Adapter 管理服务。

## 2. 到底启动哪个文件

```text
Worker 启动容器：seven-worker-cli supervise
  → 读取 /opt/seven-worker/contract/manifest.json
  → 校验 adapter 描述、入口与相关代码身份
  → 执行 adapter.executable + args
  → 默认 /opt/seven/adapter/entrypoint
  → launcher 启动镜像作者指定的模块
  → agent.serve() 建立 stdio 会话
```

`entrypoint` 是实际可执行文件，不是等待扫描的目录。平台不搜索 `main.py`，不根据装饰器猜文件名。装饰器只在作者指定模块已加载后注册处理函数。Task、仓库及上传内容不能覆盖启动入口。其他语言可以直接提供可执行程序，无需 Python launcher。

## 3. 本地开发与单文件构建：当前可用

从仓库根目录执行；使用 Python 3.11 或更高版本，在本地虚拟环境安装源码包：

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -e ./deploy/workforce/runtime-sdk
.venv/bin/seven-agent-force-runtime inspect ./my_adapter.py
.venv/bin/seven-agent-force-runtime dev ./my_adapter.py
.venv/bin/seven-agent-force-runtime build ./my_adapter.py \
  --output ./adapter-assets \
  --image-module /opt/seven/adapter/main.py
```

发行名 `seven-agent-force-runtime-sdk`；导入名 `seven_agent.force`。输出目录应为本次新目录：工具使用独占创建，不能拿覆盖旧发布资产作为更新办法。

构建输出 `entrypoint`、`main.py`、`source-index.json`、`adapter-manifest.json`。launcher 内容为：

```sh
#!/bin/sh
exec /usr/bin/python3 -I /opt/seven/adapter/main.py
```

这些只是镜像资产，不是已构建或已准入镜像。镜像必须安装 SDK，合并 manifest 片段并完成参考镜像的运行时身份构建；不能把片段当完整 image manifest。

### 最小本地 Fake 示例

```python
from seven_agent.force.agent import AgentAdapter, RoundResult

agent = AgentAdapter(name="example", version="1")

@agent.run
async def run(request, context):
    await context.started()  # 这里只描述进程内 Fake；真实实现必须先启动原生进程。
    await context.emit_message("本地示例已执行。")
    return RoundResult.finished(summary="本轮执行结束。")

if __name__ == "__main__":
    agent.serve()
```

真实实现使用 `RunRequest`、`ResumeRequest`、`ExecutionContext` 和 `RoundResult`。仅在原生恢复真正可用时注册 `@agent.resume`，保存真实 Session 引用；不得编造可恢复身份。处理函数应感知 `context.cancelled`，收尾原生子进程。活动成功必须显式报告，`finished()` 不等于 Task 成功。

参考：[Fake Adapter](../../deploy/workforce/adapters/fake/adapter.py)、[Codex Adapter](../../deploy/workforce/adapters/codex/adapter.py)、[SDK 源码](../../deploy/workforce/runtime-sdk/src/seven_agent/force)。示例不是原生 CLI 调用模板，也不证明恢复和取消合规。

## 4. 多文件项目：项目构建入口已实现

推荐保持普通 Python 项目结构：

```text
my-adapter/
├─ pyproject.toml
└─ src/my_adapter/
   ├─ __init__.py
   ├─ __main__.py
   ├─ adapter.py
   ├─ native_client.py
   ├─ events.py
   └─ session.py
```

`__init__.py` 导出 `agent`；也可由指定模块或包的 `__main__.py` 导出。`__main__.py` 若自行调用 serve，须放在 `if __name__ == "__main__":` 保护下，避免 inspect 导入时启动通信。一个入口可以加载多个文件，平台不增加多个入口或业务分支。

```toml
[project]
name = "my-adapter"
version = "0.1.0"

[tool.setuptools.package-data]
my_adapter = ["data/*.json"]
```

```sh
seven-agent-force-runtime inspect ./my-adapter --module my_adapter
seven-agent-force-runtime dev ./my-adapter --module my_adapter
seven-agent-force-runtime build ./my-adapter --module my_adapter --output ./adapter-assets
```

三条命令共用发现规则：pyproject.toml 的 project 表、唯一 src 或 flat 布局的普通 Python 包、指定模块导出唯一 AgentAdapter（无导出时读取该包的 `__main__`）。不自动执行 build backend，不支持隐式 namespace-package 发现；相互冲突的布局拒绝。资源使用上述 setuptools package-data（含 `*` 包匹配），每个声明模式必须匹配文件，不读取 MANIFEST.in 或执行 setup.py。选定顶层包内所有 `.py` 和声明资源纳入构建；其他顶层包及第三方依赖由镜像作者固定，不自动安装。

产物包含选定包、标准库核心 SDK、runtime-entry.py、entrypoint、source-index.json 和 manifest 片段。entrypoint 执行 `python3 -I /opt/seven/adapter/runtime-entry.py`；启动文件仅把自身所在的受保护目录加入导入路径，加载已指定模块并调用 serve。不会借助 Task cwd 或 PYTHONPATH。完整代码/资源/SDK均进入摘要索引，平台实际校验可发现已索引文件遗漏或篡改。镜像仍需复制产物、合并完整manifest并完成现有准入与Processor运行时身份配置。

构建拒绝符号链接、越界资源模式、缺失资源、明显凭据文件及私钥标记；跳过缓存与虚拟环境。单文件最多1 MiB，索引最多2048项，总内容最多64 MiB。输出必须是新目录，禁止覆盖。secret检查不等于完整凭据扫描，作者仍须检查交付内容。构建/inspect会导入可信作者代码；工具不主动调用模型，但作者顶层代码不能有联网、模型调用或serve副作用。

**不使用项目构建工具时的手工镜像接入**：镜像作者自行复制完整包到 `/opt/seven/adapter/` 下，在该目录保留一个顶层启动文件。由于 `python -I` 不使用脚本目录和 `PYTHONPATH` 查找模块，启动文件必须像参考 Adapter 一样显式添加受保护的镜像路径，不能添加 Task 工作目录。例如：

```python
# /opt/seven/adapter/main.py；所有引用代码均由镜像构建固定。
import sys
sys.path.insert(0, "/opt/seven/adapter")
sys.path.insert(0, "/opt/seven-worker/result-runtime")
from my_adapter.adapter import agent
agent.serve()
```

复制 SDK 到参考镜像的 runtime 路径，完成 SDK/module index；复制完整 Adapter 包后，在镜像构建阶段调用仓库内 `build_image_adapter.py --source main.py`，该脚本递归建立 Adapter 文件索引，并更新完整 manifest 中的摘要。随后去除代码目录写权限。具体构建步骤以[参考 Dockerfile](../../deploy/workforce/images/codex-harness.Dockerfile)为准；脚本固定使用镜像内路径，不是宿主机通用打包命令。

注意：

- 不带 `--module` 的单文件 `build SOURCE --image-module ...` 仍只复制该源文件；多文件项目使用本节的项目入口，不混用两种参数。
- 构建前排除凭据、`.env`、虚拟环境、缓存、日志、测试输出和不需要的资源；不打包符号链接。当前构建助手不能被当作完整的恶意目录扫描器。
- 依赖由正常 Python 包管理及镜像构建流程固定；不在 Task 执行时联网安装 Adapter 依赖。包名列表不是依赖内容完整性证明，最终发布身份必须绑定精确镜像 digest。
- 索引生成后不得修改被索引内容；新增文件、修改 launcher、SDK 或依赖都需重建、重新准入。
- `python -I -m my_adapter` 是安装到受保护 site-packages 后可采用的 Python 入口形式，但当前构建工具不会自动生成这类项目安装和完整代码索引。不要只手改 launcher 而遗漏身份更新。

### 验收范围

项目示例包括 adapter/events/session 三个模块和 JSON 资源；真实隔离Python进程与Go消费者运行通过。现有入口校验实际拒绝删除或篡改模块、资源及SDK。另验证flat布局、确定性索引、导入缓存隔离、缺失资源、路径越界及重复构建拒绝。没有为此重建Docker镜像、重跑浏览器或调用真实Provider。

## 5. 映射平台工具

平台工具不是 Adapter 插件。Adapter 作者只负责把原生 Agent 的工具调用转换为统一 `tool.request`，并把平台返回的 `tool.result` 交回原生调用；授权、执行、网络出口和审计都在平台侧。当前源码定义两个直接命名的工具：网络命令和 Git 交付意图。Adapter 不能自行注册任意名称获得平台能力。

工具定义来自 `execution.start/resume` 的冻结 `availableTools`。原生会话确认接受这些定义后，Adapter 应调用 `context.tool_catalog_ready(...)` 回报排序后的安全工具名；SDK 计算目录 digest，Supervisor 再与冻结集合核对。该回报只帮助定位“平台未下发、Adapter 未注册、原生未调用”中的哪一段，不会为 Adapter 增加权限，也不能替代实际 `context.request_tool(...)`。

Python SDK 最小写法：

```python
from seven_agent.force.agent import AgentAdapter, ToolCallRequest

agent = AgentAdapter(
    name="my-agent",
    version="1.0.0",
    capabilities=("tool.request",),
)

# 在 @agent.run / @agent.resume handler 内：
result = await context.request_tool(ToolCallRequest(
    native_tool_call_id=native_call.id,
    local_tool_call_key="native:" + native_call.id,
    tool_name="run_command_with_network_access",
    arguments={
        "purpose": "安装锁定依赖，以运行项目检查",
        "command": "pnpm install --frozen-lockfile",
        "cwd": ".",
        "targets": [
            {"host": "registry.npmjs.org", "protocol": "https", "port": 443},
        ],
    },
))

await native_client.resolve_tool_call(native_call.id, result.wire())
await context.tool_received(result)
```

Git 交付意图使用同一接口，但 arguments 只能包含摘要：

```python
result = await context.request_tool(ToolCallRequest(
    native_tool_call_id=native_call.id,
    local_tool_call_key="native:" + native_call.id,
    tool_name="request_git_delivery",
    arguments={
        "summary": "实现与必需检查已经完成。",
    },
))

await native_client.resolve_tool_call(native_call.id, result.wire())
await context.tool_received(result)
```

返回的 `SCHEDULED` 只表示 Delivery Intent 已由平台可靠保存。Adapter 不轮询 Git 状态、不执行 push、不持有 credential，也不把远端目标加入 arguments；最终交付由 Finalizer 与 Delivery 模块在 Agent 轮结束后完成。若 Task 没有冻结 Git Plan，该工具不会出现在 Server 提供的工具集合中，Adapter 必须按不支持处理，不能退回原生 `git push`。

实际原生客户端必须保持自己的 request ID 映射，并在原生调用确实收到结果后才发 `tool.received`。如果原生结果状态不明，不得盲目重复调用；关闭或撤销时使用 `context.close_tool(...)`。不使用 SDK 的语言实现必须遵守同一线协议与 occurrence/digest 规则。

Codex 参考 Adapter 使用固定 `0.147.0` App Server experimental `dynamicTools`：冻结定义在 `thread/start` 注册，成功后回报 `tool.catalog`，`item/tool/call` 转为 `tool.request`，结果通过匹配的 JSON-RPC request 返回。能力不可用时 fail closed，不改用普通 shell approval。Fake Adapter 对网络工具和 Git 交付工具均使用固定 fixture，不访问 Provider/Git，也不允许 Task 文本选择工具调用。

## 6. 协议、调试与安全

已开放：initialize/ready、execution.start/resume、execution.started、message/plan/activity、execution.interrupt、control.ack、execution.finished，持久问题/补充，以及 `tool.request/result/received/closed`。当前源码识别 `run_command_with_network_access` 与 `request_git_delivery`；声明通用工具信封不等于任意名称都会获得执行能力。真实远端 Git 操作需在目标环境独立验证。

- 专用 stdin/stdout JSONL；日志不得写入协议 stdout。
- 帧上限 1 MiB，展示消息上限 64 KiB；严格方法方向、关联键及 decimal-string 实体 ID。
- 一个实例同一时间一轮执行；控制接收不等于执行完成。连接重建不能自行重新启动 Agent。
- 不透传原始 Provider body、内部推理、凭据和未过滤原生事件；不把 Processor SECRET 环境传给 Adapter。
- stdout 与镜像身份提供通信和代码来源约束，不把不可信容器变成授权方。批准、可信检查及最终结果仍由平台验证。

```sh
.venv/bin/seven-agent-force-runtime check ./transcript.jsonl --direction up
PYTHONPATH=deploy/workforce/runtime-sdk/src python3 -m unittest discover \
  -s deploy/workforce/runtime-sdk/tests -v
```

`inspect/dev/build` 会导入作者模块，只针对可信作者代码。`dev` 默认只握手，显式 `--fixture` 才执行；真实 Adapter 可能由此调用模型，必须另行授权。`check` 只验证消息帧，不等于顺序、去重、取消、恢复或业务验收。调试记录不得保存真实凭据或原始模型内容。

## 7. 交付与维护入口

顺序：本地测试 → 镜像构建 → 固定 digest → 现有 image admission → Harness 发布/配置 → 隔离 Fake Task → 状态与结果回读 → 资源清理核对。配置和操作命令沿用[本地运行手册](mvp-runbook.md)，不手写数据库记录跳过准入。

交付包应包含源码、依赖约定、入口说明、能力矩阵、测试命令、镜像/代码身份、已验证与未验证范围。升级只影响新选择的执行配置，不改写历史 Task 冻结身份或不可变 Processor Release。

运行排障见 [AgentAdapter 操作维护手册](agent-adapter-operations.md)。
