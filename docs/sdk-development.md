# Python Runtime SDK 与 Adapter 开发指南

本指南分为 SDK 使用者和 SDK 贡献者两条路径。规范入口是 [Agent Adapter 合同](workforce/protocols/agent-adapter.md)、[接口参考](workforce/agent-adapter-interface-reference.md) 和 [方法/参数/类型参考](workforce/protocols/python-sdk.md)。这里的 SDK 用于镜像内执行协议，不是 Open REST HTTP 客户端；外部系统调用任务请看 [Open REST](workforce/protocols/open-rest.md) 和 [MCP](workforce/protocols/mcp.md)。

## 1. 安装与命名

Python 3.11+。发行包名为 `seven-agent-force-runtime-sdk`，导入名为 `seven_agent.force`，CLI 为 `seven-agent-force-runtime`。下列方式从源码安装，不假设已发布到 PyPI：

```sh
python3 -m venv .venv
.venv/bin/python -m pip install -e ./deploy/workforce/runtime-sdk
.venv/bin/seven-agent-force-runtime --help
```

构建依赖通过包源安装。也可以只为本地示例设置 `PYTHONPATH=deploy/workforce/runtime-sdk/src`，使用 `python3 -m seven_agent.force`。镜像隔离模式 Python `-I` 不读取 PYTHONPATH，不能用该开发方式代替镜像安装。

已有 `seven_workforce_runtime` 导入和旧 CLI 作为兼容入口保留。更名或删除兼容入口属于破坏性变更，需要独立设计和迁移说明。

## 2. 使用者：运行一个不调用模型的 Adapter

仓库提供 [local_adapter.py](examples/sdk/local_adapter.py) 和 [inputs.json](examples/sdk/inputs.json)。该 Adapter 只发送内存中的开始/消息/结束事件，不访问 Provider、Docker、Git 或平台 API。

```sh
.venv/bin/seven-agent-force-runtime inspect docs/examples/sdk/local_adapter.py
.venv/bin/seven-agent-force-runtime dev docs/examples/sdk/local_adapter.py
.venv/bin/seven-agent-force-runtime dev docs/examples/sdk/local_adapter.py   --fixture docs/examples/sdk/inputs.json
```

不带 fixture 的 dev 只握手，结果为 HANDSHAKE_ONLY；带此合成 fixture 才执行一次示例，结果为 LOCAL_FIXTURE_FINISHED。inspect/dev/build 会导入指定作者模块，因此只对可信本地代码使用。换成真实原生 Adapter 后，fixture 可能产生模型调用和费用，需要先确认授权。

`@agent.run` 注册 execution.start 处理器。request 是 RunRequest，context 负责发送事件，RoundResult 只代表本轮结果。真实 Adapter 必须先启动原生进程，再 `context.started(native_pid=...)`；native session_reference 要能对应后续恢复。示例的 synthetic session 不能作为原生会话恢复实现。

## 3. 打包单文件与项目

单文件资产构建使用一个新的输出目录：

```sh
.venv/bin/seven-agent-force-runtime build docs/examples/sdk/local_adapter.py   --output /tmp/force-example-assets-new   --image-module /opt/seven/adapter/main.py
```

输出 entrypoint、Python 源码、source-index.json 和 adapter-manifest.json。输出目录不能已存在；这些资产不是 Docker 镜像、完整镜像 manifest 或已准入的 Runtime。

多文件实现应使用标准 src/flat Python 包，见 [多文件开发与构建](workforce/agent-adapter-developer-guide.md)。实际 CLI 形式为：

```sh
.venv/bin/seven-agent-force-runtime inspect /path/to/adapter-project --module my_adapter
.venv/bin/seven-agent-force-runtime dev /path/to/adapter-project --module my_adapter
.venv/bin/seven-agent-force-runtime build /path/to/adapter-project   --module my_adapter --output /tmp/force-project-assets-new
```

作者模块名按项目实际包名替换。项目模式不再传 --image-module。构建会索引受支持的源码与声明 package-data，并封装 SDK；不替作者执行任意依赖安装或 build backend。最终镜像需安装原生 CLI、受保护 SDK 和正确入口，合并完整 manifest，计算身份并通过已有镜像准入和发布流程。生产 Runtime 使用不可变 digest。

## 4. 问题、工具、取消与恢复

- 问答及用户补充遵循 [交互合同](../deploy/workforce/runtime-sdk/INTERACTION.md)。capability、responseId、回执及关闭事件均由既有合同约束；一次执行仅允许一个未完成问题。恢复接收可信保存答案，不创建替代问题以伪造继续执行。
- 平台工具通过协商的工具信封调用；只有具体受支持工具可以执行。声明任意工具名不产生网络或 Git 授权。参考 [工具与方法合同](workforce/agent-adapter-interface-reference.md)。
- 处理中断时收尾子进程组、任务和读写协程，并按合同返回；异常、断流、重复开始、冲突输入与迟到回答要分开处理。断流后不能盲目重发 start。
- RoundResult.finished 不等于 Task SUCCEEDED；Worker/Server 继续核验 CompletionProposal、结果、Review 和 Delivery。不能在 Adapter 中越权完成 Git push 或可信状态写入。
- Result Processor 开发见 [Processor 合同](workforce/protocols/result-processor.md)。process(context) 返回平台待验证的结果合同；可选 deliver(context) 只能用于平台批准的相应交付类型。

## 5. 贡献者：修改 SDK 或线协议

| 修改面 | 对应源码/证据 |
| --- | --- |
| SDK 生命周期/装饰器/事件 | `deploy/workforce/runtime-sdk/src/seven_agent/force/agent.py` |
| 编解码与方向验证 | 同目录 `_protocol.py` |
| 交互与工具 | 同目录 `interaction.py`、`tools.py` |
| 项目打包及 CLI | 同目录 `_project.py`、`__main__.py` |
| Result Processor helpers | 同目录 `processor/` |
| 共享 schema/有效与无效帧 | `deploy/workforce/runtime-sdk/protocol/` |
| Go 线协议消费者 | `seven-agent-force-server/internal/worker/agentprotocol/` |
| 实际执行与装配 | `seven-agent-force-server/internal/worker/`，检索处理方法和 Supervisor 调用 |
| Python 生命周期与打包测试 | `deploy/workforce/runtime-sdk/tests/` |
| 原生转换示例 | `deploy/workforce/adapters/codex/`、`claude/`；合成执行器 `fake/` |

新增字段前明确：方向、JSON 类型、必填性、默认值、长度/帧上限、ID 编码、请求/响应关联、重复和未知字段处理。新增方法还需定义时序、能力协商、取消、错误、关闭和恢复行为。先确认旧消费者对新增字段是否兼容，再决定协议/能力版本策略，不能静默启用新权限。

生产者、Python codec、schema/frames、Go decoder 和真实业务消费者必须一致。新增一条 schema 不代表实际工具已接入。输入从未验证状态进入已验证状态的位置应清晰可见；保持 JSON ID 为字符串。不要以 SDK 宽松构造函数代替 Server 的业务校验。

## 6. 开发验证

从仓库根目录执行；这些测试使用合成输入或模拟原生进程，不调用真实模型：

```sh
PYTHONPATH=deploy/workforce/runtime-sdk/src   python3 -m unittest discover -s deploy/workforce/runtime-sdk/tests
python3 -m unittest discover -s deploy/workforce/adapters/codex/tests
python3 -m unittest discover -s deploy/workforce/adapters/claude/tests
python3 -m unittest discover -s deploy/workforce/adapters/fake/tests
cd seven-agent-force-server
go test ./internal/worker/agentprotocol/... -count=1
cd ..
python3 docs/workforce/protocols/tools/check.py
```

本机执行模拟子进程/Unix socket 测试可能需要相应沙箱权限。新增线协议要有共享有效/无效帧，生命周期变更要验证重复、冲突、中断、断连与恢复等实际影响；打包变更要检查文件索引、资源、路径逃逸和 digest 身份。更新 [协议生成说明](workforce/protocols/tools/README.md) 涉及的生成文档。

构建通过、单帧合法、模拟测试通过、Docker 镜像验证、平台调度与真实 Provider 任务是不同证据。只报告实际执行过的层级。对真实任务、镜像准入或发布有额外副作用时先遵循仓库授权要求。

项目 AI 开发引导见 [.agents SDK 开发 Skill](../.agents/skills/seven-force-sdk-development/SKILL.md)。
