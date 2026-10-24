# Hook Package 协议

schemaVersion:string **seven.hook-package/v1**。Hook 是平台准入的可信资产，trustClass 固定 PLATFORM_TRUSTED；在已批准 Runtime 内执行。runtime.kind为shell或node；node requirement支持当前固定范围 `>=22 <23`，不在执行时安装依赖。

## 事件与调用

| event:string | 调用时机 |
| --- | --- |
| attempt.prepare | Attempt准备 |
| attempt.propose | CompletionProposal提出 |
| attempt.end | Attempt结束 |
| codex.PreToolUse / codex.PostToolUse | Codex原生工具调用前／后 |
| claude.PreToolUse / claude.PostToolUse | Claude原生工具调用前／后 |

统一调用：根据manifest.entrypoints找到event→执行path→stdin传入UTF-8 JSON envelope→stdout读取单个HookResult JSON。入口方法不是HTTP。stdin envelope 字段：schemaVersion:integer=1、event:string、taskId/attemptId:string、configurationDigest:string、payload:JSON object。原生事件的input来自固定Adapter桥接；保留结构为JSON，不能当作任意授权字段。

## Manifest、输入与输出

完整Manifest：[`internal/app/workforce/hook/domain.Manifest`](wire-types.md#t-e0fc1343c8a7c48b)；runtime、entrypoints、limits类型可在索引逐层查看。

| 字段 | 类型／约束 |
| --- | --- |
| schemaVersion/name/trustClass | string；前两者必需，trustClass固定 |
| version | optional string |
| runtime | object{kind:string,requirement?:string} |
| entrypoints | array{event:string,path:string,timeoutMs:integer,failureMode:string}；同事件不得重复 |
| entrypoints[].timeoutMs | 1–30000；0按默认5000处理 |
| entrypoints[].failureMode | BLOCK / OBSERVE |
| limits | object{stdinBytes:integer,stdoutBytes:integer,stderrBytes:integer}；manifest原生整数 |
| SourceFile | {path:string,content:string,mode?:integer} |

HookResult：[`internal/app/workforce/hook/domain.HookResult`](wire-types.md#t-e1448099ac2a7abc)，schemaVersion:integer固定1；decision:string ALLOW/DENY；reasonCode:string非空；operatorMessage:string；evidence:array<object<string,JSON>>。不能以进程exit0替代合法Result，也不能将Hook ALLOW视为Git/网络的用户授权。

输入与输出最大1 MiB，stderr512 KiB，包archive≤8 MiB、每文件≤2 MiB、≤256文件。路径需相对、安全、对应已冻结文件；结果严格拒绝未知字段／多段JSON。BLOCK失败关闭当前受控动作；OBSERVE保留观察结果，按平台运行规则处理。审计通道失败可导致原生动作拒绝，不能绕过审计。

源码：[Manifest/result](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go)、[实际dispatch与native envelope](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go)。管理发布相关HTTP方法见 [operator](http-operator.md)。
