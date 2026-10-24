# Result Processor 协议

Processor 发布为不可变 Python 资产，manifest `seven-result-processor.json`，入口 `processor.py`；schemaVersion:string 固定 **seven.result-processor**，不是七层任意 shell 脚本协议。服务端校验 + image fixture 通过后才准入。

## 入口方法与参数

| 方法 | 参数类型 | 返回类型／用途 |
| --- | --- | --- |
| `process(context)`，允许 async | ProcessorContext | dict，符合 ResultContract |
| `deliver(context)`，允许 async，可选 | ProcessorContext | dict{actions:array}，只作为 delivery fixture 验证入口 |
| `context.process.run(argv, *, cwd=None, timeout_seconds=120)` | argv:list[str]，1–64项，每项1–4096字符；cwd:str\|None，repo内相对路径；timeout_seconds:int，1–300 | CommandResult |
| SDK `pass_result / ask_agent / wait_user / reject / manual_review` | 关键字参数，详见SDK签名 | dict ResultContract |
| SDK `workspace_path / archive_output / get_env` | 详见SDK参数表 | 输出声明或environment值 |

ProcessorContext 字段：task_id/attempt_id/cycle_id:str（decimal ID）；cycle_number:int 正数；parameters:dict[str,Any] 默认{}；workspace:Workspace{repo:str}；environment:dict[str,str]；process:ProcessRunner。CommandResult：argv:list[str]、returncode:int、stdout:str、stderr:str、timed_out:bool=false。stdout/stderr 截断到256 KiB。完整 [Python SDK](python-sdk.md)。

process 返回对象由**受保护 bootstrap**写入指定 JSON candidate 文件；stdout/stderr 是诊断，不能作为可信业务协议。SDK 的未注解 checks/findings/outputs/actions 参数实际承载下面对应 list 项结构，省略或None表示不提供，不是任意可信结果。

## Manifest 与结果字段

Manifest 全字段与嵌套对象：[`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)。必需schemaVersion、name、runtime.kind=python；runtime.requirement允许 `>=3.11 <3.14`；maxEvaluationCycles默认3、范围1–5；continuationPolicy默认且仅CHECKPOINT_REQUIRED；deliveryPolicy默认OPTIONAL，可DECLARED。parametersSchema为JSON Schema，environment声明每个字段name/type/phase/required/taskOverrideAllowed/defaultValue/secretRef；secretRef是引用，不写真实密钥。

ResultContract 全字段：[`internal/app/workforce/resultprocessor/domain.ResultContract`](wire-types.md#t-26bc168af80dc83c)。

| 字段 | 类型与用途 |
| --- | --- |
| decision / reasonCode / message | string；decision：PASS / REVISE / WAIT_USER / REJECT / MANUAL_REVIEW |
| checks | array{name:string,status:string,reasonCode?:string,message?:string} |
| findings | array{code:string,message:string,path?:string,severity?:string} |
| questions | array{id:string,question:string,required?:boolean} |
| outputs | array{name:string,kind:string,path?:string,selection?:{includes:string[],excludes?:string[]},archiveFormat?:string,storage:string} |
| actions | array{type:string,when?:string,targetId:string(ID),baseBranch?:string,branch?:string,operation?:string,payload?:JSON} |
| continuation | object{includes?:string[]} |

输出声明与文件路径受平台校验，storage支持已有IMMEDIATE/ON_DEMAND策略；archive_output 默认format ZIP、storage ON_DEMAND。ResultContract 最大1 MiB。PASS 仍须外部 Finalizer 核验；REVISE 触发有界续跑评估；WAIT_USER需要用户处理。后处理重试不重新执行 Agent。生产Git交付由平台授权的DeliveryAttempt实现，deliver(context)不能直接绕过授权push。

源码：[契约与校验](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go)、[bootstrap](../../../deploy/workforce/result-runtime/bootstrap.py)、[SDK helpers](../../../deploy/workforce/runtime-sdk/src/seven_agent/force/processor/__init__.py)。
