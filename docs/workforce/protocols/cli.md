# CLI 命令与参数

用户工具 `seven-workerctl` 使用Framework用户token调用operator API；镜像内 `seven-worker-cli` 使用冻结spec与Supervisor socket，二者权限与用途不同。以下只解析源码，不执行命令。

## seven-workerctl

| 命令 | 参数与类型 | 对应行为 |
| --- | --- | --- |
| version | 无 | 本地版本 |
| health | --server:string；--timeout:duration=5s | GET /api/healthz |
| task create | 参数表runTask，Project/模型/Runtime/instruction必需 | POST /api/workforce/projects/:id/tasks |
| task list | --limit:integer=50，加公共flags | GET tasks |
| task get / events / cancel | task-id:string位置参数，加公共flags | GET task / GET events / POST cancel |
| artifact list | task-id:string位置参数，加公共flags | GET artifacts |
| artifact download | --output:string必传；task-id/artifact-id:string位置参数 | 下载文件，落地0600 |
| help | 无 | usage |

公共flags为--server:string（源码默认http://127.0.0.1:9277，部署覆盖）、--token:string（Framework access token）、--timeout:duration=30s。优先用SEVEN_ACCESS_TOKEN注入，避免命令行明文。flag解析在首个位置参数停止，因此flags应放在task-id等位置参数之前；artifact download源码usage将--output放在最后不等于该排序可用。

## seven-worker-cli

| 命令 | 参数形态／类型 | 用途 |
| --- | --- | --- |
| version | 无 | image版本 |
| image probe --json | 固定形态 | seven.worker-image/v1准入探测 |
| supervise | --spec:string（或环境提供路径） | 运行冻结AttemptSpec |
| check | --input:string必传；其余见runCheck flags | 冻结completion check |
| review | --spec:string，见reviewMain | 独立Review执行 |
| result-fixture / result-deliver | --spec:string | 受保护fixture／delivery spec |
| hook dispatch | --event:string | 已批准Hook事件 |
| control | [--stream]，stdin JSON request | Supervisor UDS控制 |
| interaction ask --request FILE | FILE:string必需 | 提问；等待正常答案回执 |
| status / interrupt / diagnostics | 当前socket与冻结identity | Supervisor控制 |
| review-provider-relay | flags见fakeReviewProviderRelay | image测试／受控relay，不是外部用户接口 |
| delivery-egress-gateway / target-egress-gateway / delivery-egress-probe-target | flags见对应源码表 | egress隔离与测试辅助 |

spec路径、socket路径是string；里面承载JSON对象需遵守 [类型索引](wire-types.md) 与 [Supervisor](supervisor.md)。stdin/stdout与exit code的含义由具体命令决定，不能把进程成功当作Task成功；Result Processor候选与CompletionProposal仍需独立核验。

## 从源码提取的flag定义

下表按实际解析函数给出flag名、类型、源码默认表达式与描述。默认表达式中os.Getenv只表示环境名，不读取其值。相同函数内不同subcommand不一定接受同一组flag，按上方命令分组与源码case使用。

### `seven-workerctl.addCommon`

[seven-agent-force-server/cmd/seven-workerctl/main.go:69](../../../seven-agent-force-server/cmd/seven-workerctl/main.go#L69)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--server` | string | `defaultServerURL` | Control Plane base URL |
| `--token` | string | `""` | Framework access token (or SEVEN_ACCESS_TOKEN) |
| `--timeout` | duration | `30*time.Second` | request timeout |
### `seven-workerctl.runHealth`

[seven-agent-force-server/cmd/seven-workerctl/main.go:87](../../../seven-agent-force-server/cmd/seven-workerctl/main.go#L87)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--server` | string | `defaultServerURL` | Seven Worker Control Plane base URL |
| `--timeout` | duration | `5*time.Second` | request timeout |
### `seven-workerctl.runTask`

[seven-agent-force-server/cmd/seven-workerctl/main.go:98](../../../seven-agent-force-server/cmd/seven-workerctl/main.go#L98)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--project` | string | `""` | Workforce Project ID |
| `--model-credential` | string | `""` | model credential ID |
| `--model` | string | `""` | primary model from the credential catalog |
| `--runtime-release` | string | `""` | execution image release ID |
| `--execution-timeout-seconds` | integer | `0` | optional Agent execution limit in seconds; 0 means unlimited |
| `--instruction` | string | `""` | task instruction |
| `--name` | string | `""` | optional task name |
| `--completion-check-command` | string | `""` | optional additional completion-check command |
| `--review-enabled` | boolean | `true` | enable independent review for this task |
| `--delivery-preference` | string | `"PROJECT_DEFAULT"` | PROJECT_DEFAULT, ASK_AFTER_GATES, or SAVE_ONLY |
| `--idempotency-key` | string | `randomKey()` | idempotency key |
| `--limit` | integer | `50` | maximum tasks |
| `--tag` | repeatable string | `[]` | task tag (repeatable) |
| `--reference-asset` | repeatable string | `[]` | reference asset ID (repeatable) |
### `seven-workerctl.runArtifact`

[seven-agent-force-server/cmd/seven-workerctl/main.go:217](../../../seven-agent-force-server/cmd/seven-workerctl/main.go#L217)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--output` | string | `""` | output file path |
### `seven-worker-cli.runCheck`

[seven-agent-force-server/cmd/seven-worker-cli/check.go:17](../../../seven-agent-force-server/cmd/seven-worker-cli/check.go#L17)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--input` | string | `""` | frozen check input |
| `--repository` | string | `"/workspace"` | private writable check workspace |
| `--source` | string | `""` | read-only source to copy into private workspace |
| `--dependencies` | string | `""` | optional read-only prepared dependency directory |
| `--timeout` | duration | `5*time.Minute` | check time limit |
### `seven-worker-cli.deliveryEgressGateway`

[seven-agent-force-server/cmd/seven-worker-cli/delivery_egress_gateway.go:21](../../../seven-agent-force-server/cmd/seven-worker-cli/delivery_egress_gateway.go#L21)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--listen-port` | integer | `0` | original target port exposed to the runner |
| `--target-ip` | string | `""` | frozen authorized target IP |
| `--target-port` | integer | `0` | frozen authorized target port |
| `--deadline` | string | `""` | RFC3339 authority deadline |
### `seven-worker-cli.deliveryEgressProbeTarget`

[seven-agent-force-server/cmd/seven-worker-cli/delivery_egress_gateway.go:52](../../../seven-agent-force-server/cmd/seven-worker-cli/delivery_egress_gateway.go#L52)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--port` | integer | `0` | probe port |
### `seven-worker-cli.hookMain`

[seven-agent-force-server/cmd/seven-worker-cli/hook.go:66](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go#L66)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--event` | string | `""` | known Hook event |
### `seven-worker-cli.fakeReviewProviderRelay`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:469](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L469)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--socket` | string | `""` | Unix socket path |
| `--credential-id` | string | `""` | frozen model credential id |
| `--credential-version` | string | `""` | model credential secret version |
| `--protocol` | string | `""` | frozen provider protocol |
| `--model` | string | `""` | frozen model |
| `--contract` | string | `""` | frozen structured-output contract |
### `seven-worker-cli.supervise`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:604](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L604)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--spec` | string | `os.Getenv("SEVEN_ATTEMPT_SPEC")` | AttemptSpec JSON path |
### `seven-worker-cli.resultDeliver`

[seven-agent-force-server/cmd/seven-worker-cli/result_deliver.go:33](../../../seven-agent-force-server/cmd/seven-worker-cli/result_deliver.go#L33)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--spec` | string | `""` | protected delivery spec |
### `seven-worker-cli.resultFixture`

[seven-agent-force-server/cmd/seven-worker-cli/result_fixture.go:48](../../../seven-agent-force-server/cmd/seven-worker-cli/result_fixture.go#L48)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--spec` | string | `""` | private fixture spec |
### `seven-worker-cli.parseTargetEgressGatewayArgs`

[seven-agent-force-server/cmd/seven-worker-cli/target_egress_gateway.go:29](../../../seven-agent-force-server/cmd/seven-worker-cli/target_egress_gateway.go#L29)

| flag | 类型 | 默认表达式 | 说明 |
| --- | --- | --- | --- |
| `--listen-port` | integer | `0` | original target port exposed to the runner |
| `--target-ip` | string | `""` | frozen authorized target IP |
| `--target-port` | integer | `0` | frozen authorized target port |
| `--deadline` | string | `""` | RFC3339 authority deadline |
