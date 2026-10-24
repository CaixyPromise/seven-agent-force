# Wire 类型索引

来源：当前 Go 类型与 JSON tag。此页是字段级参考，类型名的包路径保留用于区分同名 DTO；不是要求客户端使用这些 Go 包。HTTP `response.Success` 对 int64/uint64 做字符串归一化；原生 JSON、消息、manifest 不经过该归一化，遵循 `json:",string"` 与自定义序列化。原生 JSON 列不等于每个字段都可直接从请求提交。

`*T` 可为 null；slice 在原生 JSON 可为 null；`omitempty` 是省略零值规则，不代表业务可选。未声明结构的 `RawMessage`／`map[string]any` 仍然是开放 JSON，需读对应协议语义。嵌入对象按未命名 JSON 嵌入规则展开；自定义 MarshalJSON 见 [公共约定](common.md)。

<a id="t-61ef6f9c66cc0319"></a>

## `cmd/seven-worker-cli.agentNotification`

[seven-agent-force-server/cmd/seven-worker-cli/interaction.go:25](../../../seven-agent-force-server/cmd/seven-worker-cli/interaction.go#L25)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `roundKey` | string | string | Go: `string` |
| `frame` | integer | integer | [`internal/worker/agentprotocol.Frame`](wire-types.md#t-d24d5e7adfa2aa26)；Go: `internal/worker/agentprotocol.Frame` |

<a id="t-b00156971165c1d2"></a>

## `cmd/seven-worker-cli.agentObservation`

[seven-agent-force-server/cmd/seven-worker-cli/agent_observations.go:16](../../../seven-agent-force-server/cmd/seven-worker-cli/agent_observations.go#L16)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `method` | string | string | Go: `string` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-9bb4c67471d2b20b"></a>

## `cmd/seven-worker-cli.attemptControlRequest`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:198](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L198)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `command` | string | string | Go: `string` |
| `taskId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `eventType` | string | string | 零值可省略；Go: `string` |
| `hookEvent` | string | string | 零值可省略；Go: `string` |
| `invocationId` | string | string | 零值可省略；Go: `string` |
| `configurationDigest` | string | string | 零值可省略；Go: `string` |
| `hookName` | string | string | 零值可省略；Go: `string` |
| `packageReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `packageVersion` | string | string | 零值可省略；Go: `string` |
| `packageDigest` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `diagnostic` | string | string | 零值可省略；Go: `string` |
| `durationMs` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `runPermitId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `runPermitToken` | string | string | 零值可省略；Go: `string` |
| `runNonce` | string | string | 零值可省略；Go: `string` |
| `expiresAt` | string | string | 零值可省略；Go: `string` |
| `previousOwnerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reattachPermitId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reattachPermitToken` | string | string | 零值可省略；Go: `string` |
| `leaseExpiresAt` | string | string | 零值可省略；Go: `string` |
| `leaseReceiptDigest` | string | string | 零值可省略；Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `cycleNumber` | integer | integer | 零值可省略；Go: `int` |
| `materialization` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `directive` | string | string | 零值可省略；Go: `string` |
| `decision` | string | string | 零值可省略；Go: `string` |
| `message` | string | string | 零值可省略；Go: `string` |
| `managedFeedback` | string | string | 零值可省略；Go: `string` |
| `resumeNonce` | string | string | 零值可省略；Go: `string` |
| `currentRound` | string | string | 零值可省略；Go: `string` |
| `sessionGeneration` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `method` | string | string | 零值可省略；Go: `string` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `after` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `through` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-a67eeab77f936175"></a>

## `cmd/seven-worker-cli.attemptSpec`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:40](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L40)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `authorityExpiresAt` | string | string | 零值可省略；Go: `string` |
| `instruction` | string | string | Go: `string` |
| `workspace` | string | string | Go: `string` |
| `output` | string | string | Go: `string` |
| `state` | string | string | Go: `string` |
| `runtime` | string | string | Go: `string` |
| `temporary` | string | string | Go: `string` |
| `hooks` | [`cmd/seven-worker-cli.hookSpec`](wire-types.md#t-e3b92a7fc9904be6) 或 null | [`cmd/seven-worker-cli.hookSpec`](wire-types.md#t-e3b92a7fc9904be6) 或 null | 零值可省略；[`cmd/seven-worker-cli.hookSpec`](wire-types.md#t-e3b92a7fc9904be6)；Go: `*cmd/seven-worker-cli.hookSpec` |
| `resultProcessor` | [`cmd/seven-worker-cli.resultProcessorSpec`](wire-types.md#t-7d19a4d7e3f58fab) 或 null | [`cmd/seven-worker-cli.resultProcessorSpec`](wire-types.md#t-7d19a4d7e3f58fab) 或 null | 零值可省略；[`cmd/seven-worker-cli.resultProcessorSpec`](wire-types.md#t-7d19a4d7e3f58fab)；Go: `*cmd/seven-worker-cli.resultProcessorSpec` |
| `provider` | [`cmd/seven-worker-cli.providerFacadeSpec`](wire-types.md#t-85898dcbaae7975b) 或 null | [`cmd/seven-worker-cli.providerFacadeSpec`](wire-types.md#t-85898dcbaae7975b) 或 null | 零值可省略；[`cmd/seven-worker-cli.providerFacadeSpec`](wire-types.md#t-85898dcbaae7975b)；Go: `*cmd/seven-worker-cli.providerFacadeSpec` |
| `continuation` | [`cmd/seven-worker-cli.continuationSpec`](wire-types.md#t-438f7039ab59e411) 或 null | [`cmd/seven-worker-cli.continuationSpec`](wire-types.md#t-438f7039ab59e411) 或 null | 零值可省略；[`cmd/seven-worker-cli.continuationSpec`](wire-types.md#t-438f7039ab59e411)；Go: `*cmd/seven-worker-cli.continuationSpec` |
| `reviewCorrection` | [`cmd/seven-worker-cli.reviewCorrectionSpec`](wire-types.md#t-44c3b2999dfa6837) 或 null | [`cmd/seven-worker-cli.reviewCorrectionSpec`](wire-types.md#t-44c3b2999dfa6837) 或 null | 零值可省略；[`cmd/seven-worker-cli.reviewCorrectionSpec`](wire-types.md#t-44c3b2999dfa6837)；Go: `*cmd/seven-worker-cli.reviewCorrectionSpec` |
| `interactionResponse` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `interactionQuestionRequest` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `operationToolResult` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `allowedAdapterTools` | array<string> | array<string> | 零值可省略；Go: `[]string` |

<a id="t-a4cc0060baccff0f"></a>

## `cmd/seven-worker-cli.codexReviewInput`

[seven-agent-force-server/cmd/seven-worker-cli/codex_review.go:52](../../../seven-agent-force-server/cmd/seven-worker-cli/codex_review.go#L52)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `reviewRunId` | string | string | Go: `string` |
| `taskId` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `reviewContract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `requirements` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `provider` | [`cmd/seven-worker-cli.codexReviewProviderIdentity`](wire-types.md#t-78628e2e60777266) 或 null | [`cmd/seven-worker-cli.codexReviewProviderIdentity`](wire-types.md#t-78628e2e60777266) 或 null | [`cmd/seven-worker-cli.codexReviewProviderIdentity`](wire-types.md#t-78628e2e60777266)；Go: `*cmd/seven-worker-cli.codexReviewProviderIdentity` |

<a id="t-78628e2e60777266"></a>

## `cmd/seven-worker-cli.codexReviewProviderIdentity`

[seven-agent-force-server/cmd/seven-worker-cli/codex_review.go:44](../../../seven-agent-force-server/cmd/seven-worker-cli/codex_review.go#L44)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string | string | Go: `string` |
| `credentialVersion` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `model` | string | string | Go: `string` |
| `structuredContract` | string | string | Go: `string` |

<a id="t-438f7039ab59e411"></a>

## `cmd/seven-worker-cli.continuationSpec`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:74](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L74)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `targetTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sessionLineageDigest` | string | string | Go: `string` |
| `resumeNonce` | string | string | Go: `string` |
| `resumeNonceDigest` | string | string | Go: `string` |

<a id="t-e46cd0b7f29a5e84"></a>

## `cmd/seven-worker-cli.controlStreamRequest`

[seven-agent-force-server/cmd/seven-worker-cli/control_stream.go:21](../../../seven-agent-force-server/cmd/seven-worker-cli/control_stream.go#L21)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string | string | Go: `string` |
| `request` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-6fb87698c5029896"></a>

## `cmd/seven-worker-cli.evaluationCandidateSnapshot`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:1331](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L1331)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `cycleNumber` | integer | integer | Go: `int` |
| `candidateFile` | string | string | Go: `string` |
| `contractSha256` | string | string | Go: `string` |
| `sessionDigest` | string | string | 零值可省略；Go: `string` |

<a id="t-d010c4bbb53920a1"></a>

## `cmd/seven-worker-cli.evaluationMaterialization`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:1325](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L1325)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `variables` | object<string, string> | object<string, string> | Go: `map[string]string` |
| `expiresAt` | string | string | Go: `string` |

<a id="t-5f3d22a27487b19b"></a>

## `cmd/seven-worker-cli.hookConfigurationSpec`

[seven-agent-force-server/cmd/seven-worker-cli/hook.go:30](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go#L30)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `configurationDigest` | string | string | Go: `string` |
| `hooks` | array<[`cmd/seven-worker-cli.hookPackageSpec`](wire-types.md#t-95d456a15d7eb94b)> | array<[`cmd/seven-worker-cli.hookPackageSpec`](wire-types.md#t-95d456a15d7eb94b)> | [`cmd/seven-worker-cli.hookPackageSpec`](wire-types.md#t-95d456a15d7eb94b)；Go: `[]cmd/seven-worker-cli.hookPackageSpec` |

<a id="t-5c125511739da7ac"></a>

## `cmd/seven-worker-cli.hookDecision`

[seven-agent-force-server/cmd/seven-worker-cli/hook.go:58](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go#L58)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `decision` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `operatorMessage` | string | string | Go: `string` |
| `evidence` | array<object<string, 任意 JSON 值>> | array<object<string, 任意 JSON 值>> | Go: `[]map[string]any` |

<a id="t-5f8fc10a27f35a0a"></a>

## `cmd/seven-worker-cli.hookEntrypoint`

[seven-agent-force-server/cmd/seven-worker-cli/hook.go:52](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go#L52)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `event` | string | string | Go: `string` |
| `path` | string | string | Go: `string` |
| `timeoutMs` | integer | integer | Go: `int` |
| `failureMode` | string | string | Go: `string` |

<a id="t-fd9783cd81b9cb67"></a>

## `cmd/seven-worker-cli.hookManifest`

[seven-agent-force-server/cmd/seven-worker-cli/hook.go:45](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go#L45)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | string | string | Go: `string` |
| `runtime` | object（见 Go 类型或匿名结构索引） | object（见 Go 类型或匿名结构索引） | [`inline.493caa9b0777`](wire-types.md#t-64485de401e84131)；Go: `struct{Kind string "json:\"kind\""}` |
| `entrypoints` | array<[`cmd/seven-worker-cli.hookEntrypoint`](wire-types.md#t-5f8fc10a27f35a0a)> | array<[`cmd/seven-worker-cli.hookEntrypoint`](wire-types.md#t-5f8fc10a27f35a0a)> | [`cmd/seven-worker-cli.hookEntrypoint`](wire-types.md#t-5f8fc10a27f35a0a)；Go: `[]cmd/seven-worker-cli.hookEntrypoint` |

<a id="t-95d456a15d7eb94b"></a>

## `cmd/seven-worker-cli.hookPackageSpec`

[seven-agent-force-server/cmd/seven-worker-cli/hook.go:35](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go#L35)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `releaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `packageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |
| `digest` | string | string | Go: `string` |
| `runtimeKind` | string | string | Go: `string` |
| `root` | string | string | Go: `string` |
| `manifest` | [`cmd/seven-worker-cli.hookManifest`](wire-types.md#t-fd9783cd81b9cb67) | [`cmd/seven-worker-cli.hookManifest`](wire-types.md#t-fd9783cd81b9cb67) | [`cmd/seven-worker-cli.hookManifest`](wire-types.md#t-fd9783cd81b9cb67)；Go: `cmd/seven-worker-cli.hookManifest` |

<a id="t-e3b92a7fc9904be6"></a>

## `cmd/seven-worker-cli.hookSpec`

[seven-agent-force-server/cmd/seven-worker-cli/hook.go:25](../../../seven-agent-force-server/cmd/seven-worker-cli/hook.go#L25)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `configurationDigest` | string | string | Go: `string` |
| `root` | string | string | Go: `string` |
| `spec` | string | string | Go: `string` |

<a id="t-85898dcbaae7975b"></a>

## `cmd/seven-worker-cli.providerFacadeSpec`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:108](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L108)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `model` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `listen` | string | string | Go: `string` |
| `proxyBaseUrl` | string | string | Go: `string` |

<a id="t-ac064a8eb2503f09"></a>

## `cmd/seven-worker-cli.resultDeliverSpec`

[seven-agent-force-server/cmd/seven-worker-cli/result_deliver.go:18](../../../seven-agent-force-server/cmd/seven-worker-cli/result_deliver.go#L18)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `deliveryAttemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `processorDigest` | string | string | Go: `string` |
| `processorRoot` | string | string | Go: `string` |
| `resultRoot` | string | string | Go: `string` |
| `runtimeRoot` | string | string | Go: `string` |
| `resultPath` | string | string | Go: `string` |
| `deliveryContext` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `materializationPath` | string | string | Go: `string` |

<a id="t-12c1732c8eb767cd"></a>

## `cmd/seven-worker-cli.resultFixtureSpec`

[seven-agent-force-server/cmd/seven-worker-cli/result_fixture.go:35](../../../seven-agent-force-server/cmd/seven-worker-cli/result_fixture.go#L35)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `runId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionDigest` | string | string | Go: `string` |
| `processorRoot` | string | string | Go: `string` |
| `workspaceRoot` | string | string | Go: `string` |
| `runtimeRoot` | string | string | Go: `string` |
| `resultPath` | string | string | Go: `string` |
| `evaluationContext` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `configOverrides` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `runDeliver` | boolean | boolean | 零值可省略；Go: `bool` |

<a id="t-584d4d32b2c93290"></a>

## `cmd/seven-worker-cli.resultProcessorImageIdentity`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:100](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L100)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `pythonRuntimeIdentity` | string | string | Go: `string` |
| `bootstrapSha256` | string | string | Go: `string` |
| `sdkSha256` | string | string | Go: `string` |
| `moduleIndexSha256` | string | string | Go: `string` |
| `moduleRoots` | array<string> | array<string> | Go: `[]string` |

<a id="t-7d19a4d7e3f58fab"></a>

## `cmd/seven-worker-cli.resultProcessorSpec`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:86](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L86)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `releaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `digest` | string | string | Go: `string` |
| `root` | string | string | Go: `string` |
| `bootstrap` | string | string | Go: `string` |
| `runtime` | string | string | Go: `string` |
| `pythonRuntimeIdentity` | string | string | Go: `string` |
| `bootstrapDigest` | string | string | Go: `string` |
| `sdkDigest` | string | string | Go: `string` |
| `moduleIndexDigest` | string | string | Go: `string` |
| `parameters` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `maxEvaluationCycles` | integer | integer | Go: `int` |

<a id="t-44c3b2999dfa6837"></a>

## `cmd/seven-worker-cli.reviewCorrectionSpec`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:65](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L65)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceReviewRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionKind` | string | string | Go: `string` |
| `verdict` | string | string | 零值可省略；Go: `string` |
| `reportPath` | string | string | 零值可省略；Go: `string` |
| `reportDigest` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |

<a id="t-c66a74ff682af4bb"></a>

## `cmd/seven-worker-cli.supervisorState`

[seven-agent-force-server/cmd/seven-worker-cli/main.go:128](../../../seven-agent-force-server/cmd/seven-worker-cli/main.go#L128)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `state` | string | string | Go: `string` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `startedAt` | string | string | Go: `string` |
| `harnessStartedAt` | string | string | 零值可省略；Go: `string` |
| `harnessPid` | integer | integer | 零值可省略；Go: `int` |
| `runPermitId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `runPermitDigest` | string | string | 零值可省略；Go: `string` |
| `runNonceDigest` | string | string | 零值可省略；Go: `string` |
| `authorityExpiresAt` | string | string | 零值可省略；Go: `string` |
| `orphanDeadline` | string | string | 零值可省略；Go: `string` |
| `reattachPermitId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reattachPermitDigest` | string | string | 零值可省略；Go: `string` |
| `lastJournalSequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `evaluationCycleId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `evaluationCycle` | integer | integer | 零值可省略；Go: `int` |
| `evaluationReadyAt` | string | string | 零值可省略；Go: `string` |
| `evaluationDigest` | string | string | 零值可省略；Go: `string` |
| `evaluationCandidate` | string | string | 零值可省略；Go: `string` |
| `evaluationCandidateDigest` | string | string | 零值可省略；Go: `string` |
| `evaluationContinuationDigest` | string | string | 零值可省略；Go: `string` |
| `sessionDigest` | string | string | 零值可省略；Go: `string` |
| `round` | integer | integer | 零值可省略；Go: `int` |

<a id="t-530db2c02836da26"></a>

## `inline.00f046ab8adb`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `messageKey` | string | string | Go: `string` |
| `responseId` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |

<a id="t-149143e12cdb87b6"></a>

## `inline.030988320bde`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `receipt` | integer | integer | [`internal/app/workforce/interaction/facade.Receipt`](wire-types.md#t-72ef9cc8b517430e)；Go: `internal/app/workforce/interaction/facade.Receipt` |
| `operation` | integer | integer | 零值可省略；[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)；Go: `internal/app/workforce/executionaccess/facade.Receipt` |
| `deliveryIntent` | integer | integer | 零值可省略；[`internal/app/workforce/delivery/facade.IntentReceipt`](wire-types.md#t-c029d6a1620fc30f)；Go: `internal/app/workforce/delivery/facade.IntentReceipt` |
| `immediateToolResult` | integer | integer | 零值可省略；[`internal/app/workforce/delivery/facade.ImmediateToolResult`](wire-types.md#t-510c99b41bf507eb)；Go: `internal/app/workforce/delivery/facade.ImmediateToolResult` |
| `frameSha256` | string | string | Go: `string` |

<a id="t-27bbabc2a5537467"></a>

## `inline.0b483ba1663f`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-3ef2787367843da1"></a>

## `inline.1385a0115560`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ack` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultAckRequest`](wire-types.md#t-e21dea01113fc815)；Go: `internal/app/workforce/executionaccess/facade.ResultAckRequest` |

<a id="t-1551860725c00813"></a>

## `inline.20d3df089fa8`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |

<a id="t-7b2b3f54f8647549"></a>

## `inline.21e068903bf9`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `enabled` | boolean | boolean | Go: `bool` |
| `events` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.FactType`](wire-types.md#t-26bedcb14f61a59b)；Go: `[]internal/app/workforce/tasknotification/domain.FactType` |
| `channels` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.ChannelSelection`](wire-types.md#t-5e9beaea2948de73)；Go: `[]internal/app/workforce/tasknotification/domain.ChannelSelection` |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-e7dcd469f447bb70"></a>

## `inline.354b28be148f`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `action` | integer | integer | [`internal/app/workforce/delivery/domain.Action`](wire-types.md#t-0b32913a8ffe942c)；Go: `internal/app/workforce/delivery/domain.Action` |
| `resultCommit` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-c51f7ae28dcdf0d7"></a>

## `inline.3df6d954d9ae`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ack` | integer | integer | [`internal/app/workforce/interaction/facade.AckRequest`](wire-types.md#t-f78ccd181810d5f2)；Go: `internal/app/workforce/interaction/facade.AckRequest` |

<a id="t-64485de401e84131"></a>

## `inline.493caa9b0777`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `kind` | string | string | Go: `string` |

<a id="t-68f9249de067355a"></a>

## `inline.4e1df174b955`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `start` | integer | integer | [`internal/app/workforce/executionaccess/facade.StartRequest`](wire-types.md#t-e87acbc7a9d6bc90)；Go: `internal/app/workforce/executionaccess/facade.StartRequest` |

<a id="t-12e661996f64a8ce"></a>

## `inline.4e6fb214e99b`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `bundle` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |
| `overlay` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |
| `session` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |
| `manifest` | integer | integer | [`internal/app/workforce/taskdispatch/controller.suspendedBlob`](wire-types.md#t-4f6d5d8287bd17d0)；Go: `internal/app/workforce/taskdispatch/controller.suspendedBlob` |

<a id="t-637fe0a8d5702fd1"></a>

## `inline.4ff1e598aa48`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `watermark` | string(decimal) 或 null | string(decimal) 或 null | JSON 引号编码；Go: `*int64` |

<a id="t-25a82a7db0c8134f"></a>

## `inline.520298938742`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `frame` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-a4d674c6c5585443"></a>

## `inline.5436381527b9`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-f5c24f61ad33639c"></a>

## `inline.560164efb36e`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `action` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-16d919fb1b8985b6"></a>

## `inline.5933edbe2477`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `processorReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-2f50884469a4850f"></a>

## `inline.60f289d86afe`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `version` | string | string | Go: `string` |

<a id="t-0fffb04ef22296a8"></a>

## `inline.733e0327e9df`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-7cef4645a5c4b0d0"></a>

## `inline.7487eca6ecbe`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedRevision` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `manifest` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)；Go: `internal/app/workforce/resultprocessor/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/scriptasset/domain.SourceFile`](wire-types.md#t-17354ae5b0a2291f)；Go: `[]internal/app/workforce/resultprocessor/domain.SourceFile` |

<a id="t-4e9f84b2e76718ce"></a>

## `inline.774ca3c5731e`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `evidence` | integer | integer | [`internal/app/workforce/completioncheck/facade.Evidence`](wire-types.md#t-c31697233475837a)；Go: `internal/app/workforce/completioncheck/facade.Evidence` |

<a id="t-a4d013714b0f75d1"></a>

## `inline.881abaa51f03`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `min` | integer | integer | Go: `int` |
| `max` | integer | integer | Go: `int` |

<a id="t-0e924ed22d3b395e"></a>

## `inline.955a70a39e85`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `protocol` | string | string | Go: `string` |
| `baseUrl` | string | string | Go: `string` |
| `apiKey` | string | string | Go: `string` |

<a id="t-72ff9a926568a75e"></a>

## `inline.9a7d2f28f426`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-f7ac3943f3cc2fef"></a>

## `inline.9dc7092d6c55`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `renew` | integer | integer | [`internal/app/workforce/executionaccess/facade.RenewRequest`](wire-types.md#t-173b66c0501009d7)；Go: `internal/app/workforce/executionaccess/facade.RenewRequest` |

<a id="t-17aac48f12b81238"></a>

## `inline.a259c218dba4`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `phase` | string | string | Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `digest` | string | string | Go: `string` |

<a id="t-678f1c5438f3c6f7"></a>

## `inline.a4cf61e687be`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |

<a id="t-f166cb0eba8e94e7"></a>

## `inline.b424e42d4232`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |

<a id="t-93dd0725050150d8"></a>

## `inline.c6dba04949c2`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedRevision` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `manifest` | integer | integer | [`internal/app/workforce/hook/domain.Manifest`](wire-types.md#t-e0fc1343c8a7c48b)；Go: `internal/app/workforce/hook/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/hook/domain.SourceFile`](wire-types.md#t-1d69f951596ffc41)；Go: `[]internal/app/workforce/hook/domain.SourceFile` |

<a id="t-3bd3d2aa151c7a73"></a>

## `inline.c7c6d9b4185d`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `limit` | integer | integer | Go: `int` |

<a id="t-b16411908307d2b4"></a>

## `inline.cd5ad4c08fb9`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `questions` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Question`](wire-types.md#t-5c2e63b06a84f495)；Go: `[]internal/app/workforce/interaction/domain.Question` |
| `isBlocking` | boolean | boolean | Go: `bool` |

<a id="t-5567296190fec4a8"></a>

## `inline.d1bde174e73e`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `sessionDigest` | string | string | Go: `string` |
| `currentRound` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `suspendControlKey` | string | string | Go: `string` |

<a id="t-5c7d0e08f094b695"></a>

## `inline.eb6a51ff5193`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `receipt` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultReceiptRequest`](wire-types.md#t-45900bc030591739)；Go: `internal/app/workforce/executionaccess/facade.ResultReceiptRequest` |

<a id="t-a2bca8d4ce980fed"></a>

## `inline.ecd6ca204369`

源码类型（由 Go 编译器导出）

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `result` | integer | integer | [`internal/app/workforce/executionaccess/facade.ResultRequest`](wire-types.md#t-57e2a75e70cf2ed5)；Go: `internal/app/workforce/executionaccess/facade.ResultRequest` |

<a id="t-2b93a282f225a50e"></a>

## `internal/app/notification/facade.GroupRouteOption`

[seven-agent-force-server/internal/app/notification/facade/types.go:33](../../../seven-agent-force-server/internal/app/notification/facade/types.go#L33)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `provider` | string | string | Go: `string` |
| `mentionReady` | boolean | boolean | Go: `bool` |

<a id="t-1b30371acce7d610"></a>

## `internal/app/notification/facade.PublicationChannelStatus`

[seven-agent-force-server/internal/app/notification/facade/types.go:40](../../../seven-agent-force-server/internal/app/notification/facade/types.go#L40)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `channelKind` | string | string | Go: `string` |
| `displayName` | string | string | Go: `string` |
| `state` | string | string | Go: `string` |
| `mentionState` | string | string | Go: `string` |
| `safeReasonCode` | string | string | 零值可省略；Go: `string` |
| `acceptedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-2cea9f9f740879ba"></a>

## `internal/app/workforce/artifact/domain.FinalizationClaim`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:23](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L23)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `runNumber` | integer | integer | Go: `int` |
| `leaseToken` | string | string | Go: `string` |
| `leaseExpiresAt` | string | string | Go: `string` |
| `workItem` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `attempt` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `completion` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `evaluationCycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultContract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `checks` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `events` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-7afe4f6afad0abf6"></a>

## `internal/app/workforce/artifact/domain.FinalizationView`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:94](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L94)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `runNumber` | integer | integer | Go: `int` |
| `maxRuns` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerInstanceId` | string | string | 零值可省略；Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `manifestDigest` | string | string | 零值可省略；Go: `string` |
| `spoolIdentity` | string | string | 零值可省略；Go: `string` |
| `nextRetryAt` | string | string | 零值可省略；Go: `string` |
| `errorCode` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `frozenAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |
| `artifactUploadAttempts` | array<integer> | array<integer> | [`internal/app/workforce/artifact/domain.UploadView`](wire-types.md#t-4a71cd3513962d40)；Go: `[]internal/app/workforce/artifact/domain.UploadView` |

<a id="t-3061dadeee2262bb"></a>

## `internal/app/workforce/artifact/domain.FreezeResult`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:56](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L56)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `manifestDigest` | string | string | Go: `string` |
| `spoolIdentity` | string | string | Go: `string` |
| `artifacts` | array<integer> | array<integer> | [`internal/app/workforce/artifact/domain.FrozenArtifact`](wire-types.md#t-3db91d36a23267b3)；Go: `[]internal/app/workforce/artifact/domain.FrozenArtifact` |
| `reviewEvidence` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/artifact/domain.ReviewEvidence`](wire-types.md#t-2a36c397d93b5179)；Go: `[]internal/app/workforce/artifact/domain.ReviewEvidence` |

<a id="t-3db91d36a23267b3"></a>

## `internal/app/workforce/artifact/domain.FrozenArtifact`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:46](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L46)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `type` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `sourcePath` | string | string | 零值可省略；Go: `string` |
| `mediaType` | string | string | Go: `string` |
| `path` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |

<a id="t-2a36c397d93b5179"></a>

## `internal/app/workforce/artifact/domain.ReviewEvidence`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:68](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L68)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `path` | string | string | Go: `string` |
| `lineCount` | integer | integer | Go: `int` |

<a id="t-7f30109101ed9a1b"></a>

## `internal/app/workforce/artifact/domain.UploadClaim`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:73](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L73)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `finalizationRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `materializationRunId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `artifactType` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `objectKey` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |
| `mediaType` | string | string | Go: `string` |
| `attemptNumber` | integer | integer | Go: `int` |
| `leaseToken` | string | string | Go: `string` |
| `leaseExpiresAt` | string | string | Go: `string` |
| `spoolIdentity` | string | string | Go: `string` |
| `spoolRelativePath` | string | string | Go: `string` |
| `spoolKind` | string | string | Go: `string` |

<a id="t-4a71cd3513962d40"></a>

## `internal/app/workforce/artifact/domain.UploadView`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:117](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L117)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `finalizationRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `materializationRunId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `artifactType` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `objectKey` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |
| `mediaType` | string | string | Go: `string` |
| `attemptNumber` | integer | integer | Go: `int` |
| `maxAttempts` | integer | integer | Go: `int` |
| `status` | string | string | Go: `string` |
| `nextRetryAt` | string | string | 零值可省略；Go: `string` |
| `errorCode` | string | string | 零值可省略；Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `uploadedAt` | string | string | 零值可省略；Go: `string` |
| `verifiedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-9b5dad55bac0c0ad"></a>

## `internal/app/workforce/artifact/domain.WorkMessage`

[seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go:14](../../../seven-agent-force-server/internal/app/workforce/artifact/domain/postprocess.go#L14)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `kind` | string | string | Go: `string` |
| `messageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-f7cee4a5c35aaec2"></a>

## `internal/app/workforce/completioncheck/application.Decision`

[seven-agent-force-server/internal/app/workforce/completioncheck/application/service.go:21](../../../seven-agent-force-server/internal/app/workforce/completioncheck/application/service.go#L21)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `action` | string | string | Go: `string` |
| `resultId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reason` | string | string | Go: `string` |
| `repairScope` | string | string | 零值可省略；Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |

<a id="t-4d15795a3963cf88"></a>

## `internal/app/workforce/completioncheck/application.DecisionResult`

[seven-agent-force-server/internal/app/workforce/completioncheck/application/service.go:33](../../../seven-agent-force-server/internal/app/workforce/completioncheck/application/service.go#L33)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `decisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `replayed` | boolean | boolean | Go: `bool` |

<a id="t-6f9904ce39e62bbb"></a>

## `internal/app/workforce/completioncheck/application.Summary`

[seven-agent-force-server/internal/app/workforce/completioncheck/application/service.go:9](../../../seven-agent-force-server/internal/app/workforce/completioncheck/application/service.go#L9)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `input` | integer 或 null | integer 或 null | [`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `digest` | string | string | Go: `string` |
| `baseline` | integer 或 null | integer 或 null | [`internal/app/workforce/completioncheck/facade.Evidence`](wire-types.md#t-c31697233475837a)；Go: `*internal/app/workforce/completioncheck/facade.Evidence` |
| `currentResult` | integer 或 null | integer 或 null | [`internal/app/workforce/completioncheck/facade.Evidence`](wire-types.md#t-c31697233475837a)；Go: `*internal/app/workforce/completioncheck/facade.Evidence` |
| `availableActions` | array<string> | array<string> | Go: `[]string` |
| `checkpointReady` | boolean | boolean | Go: `bool` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceCheckpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `expectedCurrentTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-977737ba165b2ce9"></a>

## `internal/app/workforce/completioncheck/domain.Comparison`

[seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go:228](../../../seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go#L228)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `disposition` | integer | integer | [`internal/app/workforce/completioncheck/domain.Disposition`](wire-types.md#t-3e25c1e9ecd88ebb)；Go: `internal/app/workforce/completioncheck/domain.Disposition` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-3e25c1e9ecd88ebb"></a>

## `internal/app/workforce/completioncheck/domain.Disposition`

[seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go:217](../../../seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go#L217)

底层类型：`string`。
声明的常量值：`"ASK_USER"`, `"BLOCKED"`, `"NO_CHECK"`, `"PROCEED"`, `"PROCEED_WITH_LIMITATIONS"`, `"REPAIR"`。
<a id="t-47c6a1ac1dbda59f"></a>

## `internal/app/workforce/completioncheck/domain.Failure`

[seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go:185](../../../seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go#L185)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `key` | string | string | Go: `string` |
| `path` | string | string | Go: `string` |
| `fingerprint` | string | string | Go: `string` |

<a id="t-d1b1494300121a97"></a>

## `internal/app/workforce/completioncheck/domain.Input`

[seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go:28](../../../seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go#L28)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `mode` | string | string | Go: `string` |
| `command` | string | string | 零值可省略；Go: `string` |
| `script` | string | string | 零值可省略；Go: `string` |
| `instructions` | string | string | 零值可省略；Go: `string` |

<a id="t-bc73b95621f12283"></a>

## `internal/app/workforce/completioncheck/domain.Observation`

[seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go:190](../../../seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go#L190)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `definitionDigest` | string | string | 零值可省略；Go: `string` |
| `diagnosticDigest` | string | string | 零值可省略；Go: `string` |
| `diagnosticKnown` | boolean | boolean | 零值可省略；Go: `bool` |
| `diagnosticTruncated` | boolean | boolean | 零值可省略；Go: `bool` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `exitCode` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `workspaceDigest` | string | string | 零值可省略；Go: `string` |
| `testCountKnown` | boolean | boolean | 零值可省略；Go: `bool` |
| `testCount` | integer | integer | 零值可省略；Go: `int` |
| `checkDigest` | string | string | Go: `string` |
| `status` | integer | integer | [`internal/app/workforce/completioncheck/domain.Status`](wire-types.md#t-77c0fcb055790deb)；Go: `internal/app/workforce/completioncheck/domain.Status` |
| `failures` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/completioncheck/domain.Failure`](wire-types.md#t-47c6a1ac1dbda59f)；Go: `[]internal/app/workforce/completioncheck/domain.Failure` |

<a id="t-77c0fcb055790deb"></a>

## `internal/app/workforce/completioncheck/domain.Status`

[seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go:174](../../../seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go#L174)

底层类型：`string`。
声明的常量值：`"FAILED"`, `"NO_TESTS"`, `"NOT_CONFIGURED"`, `"PASSED"`, `"TIMED_OUT"`, `"UNAVAILABLE"`。
<a id="t-3bba89e7039da21c"></a>

## `internal/app/workforce/completioncheck/domain.wireInput`

[seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go:36](../../../seven-agent-force-server/internal/app/workforce/completioncheck/domain/model.go#L36)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `mode` | string | string | Go: `string` |
| `command` | string | string | 零值可省略；Go: `string` |
| `script` | string | string | 零值可省略；Go: `string` |
| `instructions` | string | string | 零值可省略；Go: `string` |
| `referenceAssetIds` | array<string> | array<string> | 零值可省略；Go: `[]string` |

<a id="t-0847d4e3d1066114"></a>

## `internal/app/workforce/completioncheck/facade.ApplyPendingResult`

[seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go:93](../../../seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go#L93)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `applied` | boolean | boolean | Go: `bool` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-c31697233475837a"></a>

## `internal/app/workforce/completioncheck/facade.Evidence`

[seven-agent-force-server/internal/app/workforce/completioncheck/facade/contracts.go:53](../../../seven-agent-force-server/internal/app/workforce/completioncheck/facade/contracts.go#L53)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `evaluationCycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `phase` | string | string | Go: `string` |
| `checkDigest` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `summary` | string | string | Go: `string` |
| `observation` | integer | integer | [`internal/app/workforce/completioncheck/domain.Observation`](wire-types.md#t-bc73b95621f12283)；Go: `internal/app/workforce/completioncheck/facade.Observation` |
| `sourceCommit` | string | string | Go: `string` |
| `sourceIdentityDigest` | string | string | Go: `string` |

<a id="t-0fbcc46c1346111a"></a>

## `internal/app/workforce/completioncheck/facade.Reference`

[seven-agent-force-server/internal/app/workforce/completioncheck/facade/snapshot.go:10](../../../seven-agent-force-server/internal/app/workforce/completioncheck/facade/snapshot.go#L10)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `referenceAssetId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `contentSha256` | string | string | Go: `string` |
| `objectSha256` | string | string | Go: `string` |
| `sizeBytes` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-970a7c1811e86335"></a>

## `internal/app/workforce/completioncheck/facade.Revision`

[seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go:30](../../../seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go#L30)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceTaskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionNumber` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `baseDigest` | string | string | Go: `string` |
| `completionCheck` | integer | integer | [`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `internal/app/workforce/completioncheck/facade.Input` |
| `checkDigest` | string | string | Go: `string` |
| `sourceBaselineResultId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `appliedTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `appliedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `replayed` | boolean | boolean | Go: `bool` |

<a id="t-86af92c22fc7f700"></a>

## `internal/app/workforce/completioncheck/facade.RevisionRequest`

[seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go:25](../../../seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go#L25)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | [`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |

<a id="t-7bf43f955ad9986e"></a>

## `internal/app/workforce/completioncheck/facade.RevisionView`

[seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go:19](../../../seven-agent-force-server/internal/app/workforce/completioncheck/facade/revision.go#L19)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `completionCheck` | integer | integer | [`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `internal/app/workforce/completioncheck/facade.Input` |
| `pending` | integer 或 null | integer 或 null | [`internal/app/workforce/completioncheck/facade.Revision`](wire-types.md#t-970a7c1811e86335)；Go: `*internal/app/workforce/completioncheck/facade.Revision` |
| `allowedActions` | array<string> | array<string> | Go: `[]string` |

<a id="t-ed211d1296f091ae"></a>

## `internal/app/workforce/completioncheck/facade.Snapshot`

[seven-agent-force-server/internal/app/workforce/completioncheck/facade/contracts.go:18](../../../seven-agent-force-server/internal/app/workforce/completioncheck/facade/contracts.go#L18)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `input` | integer | integer | [`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `internal/app/workforce/completioncheck/facade.Input` |
| `digest` | string | string | Go: `string` |

<a id="t-d29ba38818d96a5d"></a>

## `internal/app/workforce/continuation/application.ApproveRequest`

[seven-agent-force-server/internal/app/workforce/continuation/application/service.go:24](../../../seven-agent-force-server/internal/app/workforce/continuation/application/service.go#L24)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reason` | string | string | Go: `string` |

<a id="t-f9cd7c61259b9fda"></a>

## `internal/app/workforce/continuation/application.CheckpointKeyMaterialization`

[seven-agent-force-server/internal/app/workforce/continuation/application/service.go:80](../../../seven-agent-force-server/internal/app/workforce/continuation/application/service.go#L80)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `algorithm` | string | string | Go: `string` |
| `plainKeyBase64` | string | string | Go: `string` |
| `envelope` | integer | integer | [`internal/app/workforce/continuation/application.KeyEnvelope`](wire-types.md#t-d48d8f31993825ed)；Go: `internal/app/workforce/continuation/application.KeyEnvelope` |
| `aadIdentity` | string | string | Go: `string` |
| `interactionRestoreInput` | integer 或 null | integer 或 null | 零值可省略；[`internal/worker/checkpoint.RestoreInput`](wire-types.md#t-3591d56911635dbc)；Go: `*internal/worker/checkpoint.RestoreInput` |

<a id="t-d645a0c43f5ed65f"></a>

## `internal/app/workforce/continuation/application.CreateMaterializationRequest`

[seven-agent-force-server/internal/app/workforce/continuation/application/service.go:31](../../../seven-agent-force-server/internal/app/workforce/continuation/application/service.go#L31)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `outputId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-d13a95e1451be261"></a>

## `internal/app/workforce/continuation/application.CreateNextTurnRequest`

[seven-agent-force-server/internal/app/workforce/continuation/application/service.go:14](../../../seven-agent-force-server/internal/app/workforce/continuation/application/service.go#L14)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceCheckpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `expectedCurrentTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instruction` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-2c1a696e868b765c"></a>

## `internal/app/workforce/continuation/application.InteractionCaptureRequest`

[seven-agent-force-server/internal/app/workforce/continuation/application/interaction_capture.go:10](../../../seven-agent-force-server/internal/app/workforce/continuation/application/interaction_capture.go#L10)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |

<a id="t-ce8408ef649d1ae3"></a>

## `internal/app/workforce/continuation/application.InteractionEncryptedBlob`

[seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go:51](../../../seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go#L51)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `objectKey` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |

<a id="t-401cea0472aa9fd0"></a>

## `internal/app/workforce/continuation/application.InteractionReservation`

[seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go:35](../../../seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go#L35)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `identity` | string | string | Go: `string` |
| `snapshotDigest` | string | string | Go: `string` |
| `sessionDigest` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `automaticRevisionCount` | integer | integer | Go: `int` |
| `envelope` | integer | integer | [`internal/app/workforce/continuation/application.KeyEnvelope`](wire-types.md#t-d48d8f31993825ed)；Go: `internal/app/workforce/continuation/application.KeyEnvelope` |
| `currentRound` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `suspendControlKeySha256` | string | string | Go: `string` |

<a id="t-3dcee10cb63ef331"></a>

## `internal/app/workforce/continuation/application.InteractionReservationResult`

[seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go:46](../../../seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go#L46)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `disposition` | string | string | Go: `string` |
| `reservation` | integer | integer | [`internal/app/workforce/continuation/application.InteractionReservation`](wire-types.md#t-401cea0472aa9fd0)；Go: `internal/app/workforce/continuation/application.InteractionReservation` |
| `key` | integer | integer | [`internal/app/workforce/continuation/application.CheckpointKeyMaterialization`](wire-types.md#t-f9cd7c61259b9fda)；Go: `internal/app/workforce/continuation/application.CheckpointKeyMaterialization` |

<a id="t-d48d8f31993825ed"></a>

## `internal/app/workforce/continuation/application.KeyEnvelope`

[seven-agent-force-server/internal/app/workforce/continuation/application/service.go:59](../../../seven-agent-force-server/internal/app/workforce/continuation/application/service.go#L59)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `ciphertext` | string | string | Go: `string` |
| `edek` | string | string | Go: `string` |
| `keyId` | string | string | Go: `string` |

<a id="t-dbe3c865f65d55ab"></a>

## `internal/app/workforce/continuation/application.PublishInteractionRequest`

[seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go:56](../../../seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go#L56)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `bundle` | integer | integer | [`internal/app/workforce/continuation/application.InteractionEncryptedBlob`](wire-types.md#t-ce8408ef649d1ae3)；Go: `internal/app/workforce/continuation/application.InteractionEncryptedBlob` |
| `overlay` | integer | integer | [`internal/app/workforce/continuation/application.InteractionEncryptedBlob`](wire-types.md#t-ce8408ef649d1ae3)；Go: `internal/app/workforce/continuation/application.InteractionEncryptedBlob` |
| `session` | integer | integer | [`internal/app/workforce/continuation/application.InteractionEncryptedBlob`](wire-types.md#t-ce8408ef649d1ae3)；Go: `internal/app/workforce/continuation/application.InteractionEncryptedBlob` |
| `manifest` | integer | integer | [`internal/app/workforce/continuation/application.InteractionEncryptedBlob`](wire-types.md#t-ce8408ef649d1ae3)；Go: `internal/app/workforce/continuation/application.InteractionEncryptedBlob` |

<a id="t-6f67c6542c30165e"></a>

## `internal/app/workforce/continuation/application.ReserveInteractionRequest`

[seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go:28](../../../seven-agent-force-server/internal/app/workforce/continuation/application/interaction_publish.go#L28)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sessionDigest` | string | string | Go: `string` |
| `currentRound` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `suspendControlKey` | string | string | Go: `string` |

<a id="t-33457f63eb133ea3"></a>

## `internal/app/workforce/continuation/domain.Checkpoint`

[seven-agent-force-server/internal/app/workforce/continuation/domain/model.go:121](../../../seven-agent-force-server/internal/app/workforce/continuation/domain/model.go#L121)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `decision` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `checkpointIdentity` | string | string | Go: `string` |
| `manifestSha256` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |
| `approvedBy` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `approvedAt` | string | string | 零值可省略；Go: `string` |
| `approvalReason` | string | string | 零值可省略；Go: `string` |
| `consumedByTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `consumedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-7b9980726c2bd212"></a>

## `internal/app/workforce/continuation/domain.MaterializationClaim`

[seven-agent-force-server/internal/app/workforce/continuation/domain/model.go:205](../../../seven-agent-force-server/internal/app/workforce/continuation/domain/model.go#L205)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `runNumber` | integer | integer | Go: `int` |
| `kind` | string | string | Go: `string` |
| `outputId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `outputName` | string | string | 零值可省略；Go: `string` |
| `outputSourcePath` | string | string | 零值可省略；Go: `string` |
| `outputKind` | string | string | 零值可省略；Go: `string` |
| `archiveFormat` | string | string | 零值可省略；Go: `string` |
| `selectionSpec` | string | string | 零值可省略；Go: `string` |
| `expectedBaseCommit` | string | string | 零值可省略；Go: `string` |
| `expectedResultCommit` | string | string | 零值可省略；Go: `string` |
| `expectedResultTree` | string | string | 零值可省略；Go: `string` |
| `restore` | integer | integer | [`internal/app/workforce/continuation/domain.RestoreContract`](wire-types.md#t-2a3056bf3dba231b)；Go: `internal/app/workforce/continuation/domain.RestoreContract` |
| `plainKeyBase64` | string | string | Go: `string` |
| `leaseToken` | string | string | Go: `string` |
| `leaseExpiresAt` | string | string | Go: `string` |

<a id="t-d1654b86d786a887"></a>

## `internal/app/workforce/continuation/domain.MaterializationFreeze`

[seven-agent-force-server/internal/app/workforce/continuation/domain/model.go:228](../../../seven-agent-force-server/internal/app/workforce/continuation/domain/model.go#L228)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `mediaType` | string | string | Go: `string` |
| `path` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |
| `spoolIdentity` | string | string | Go: `string` |
| `selectionManifest` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `selectionManifestDigest` | string | string | 零值可省略；Go: `string` |
| `selectedFileCount` | integer | integer | 零值可省略；Go: `int` |

<a id="t-88e8ed267999e10b"></a>

## `internal/app/workforce/continuation/domain.MaterializationMessage`

[seven-agent-force-server/internal/app/workforce/continuation/domain/model.go:196](../../../seven-agent-force-server/internal/app/workforce/continuation/domain/model.go#L196)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `kind` | string | string | Go: `string` |
| `messageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-e10445ca6ec9a028"></a>

## `internal/app/workforce/continuation/domain.MaterializationView`

[seven-agent-force-server/internal/app/workforce/continuation/domain/model.go:176](../../../seven-agent-force-server/internal/app/workforce/continuation/domain/model.go#L176)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceCheckpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `outputId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `runNumber` | integer | integer | Go: `int` |
| `maxRuns` | integer | integer | Go: `int` |
| `artifactId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `artifactSha256` | string | string | 零值可省略；Go: `string` |
| `artifactSizeBytes` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |
| `nextRetryAt` | string | string | 零值可省略；Go: `string` |

<a id="t-2a3056bf3dba231b"></a>

## `internal/app/workforce/continuation/domain.RestoreContract`

[seven-agent-force-server/internal/app/workforce/continuation/domain/model.go:138](../../../seven-agent-force-server/internal/app/workforce/continuation/domain/model.go#L138)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `formatVersion` | integer | integer | Go: `int` |
| `restoreRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `sourceTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `targetTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workspaceObjectKey` | string | string | Go: `string` |
| `workspaceContentSha256` | string | string | Go: `string` |
| `workspaceObjectSha256` | string | string | Go: `string` |
| `workspaceSizeBytes` | integer | string(decimal) | Go: `int64` |
| `repositoryBundleObjectKey` | string | string | Go: `string` |
| `repositoryBundleContentSha256` | string | string | Go: `string` |
| `repositoryBundleObjectSha256` | string | string | Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | Go: `int64` |
| `workspaceOverlayObjectKey` | string | string | Go: `string` |
| `workspaceOverlayContentSha256` | string | string | Go: `string` |
| `workspaceOverlayObjectSha256` | string | string | Go: `string` |
| `workspaceOverlaySizeBytes` | integer | string(decimal) | Go: `int64` |
| `selectionDigest` | string | string | Go: `string` |
| `sessionObjectKey` | string | string | Go: `string` |
| `sessionContentSha256` | string | string | Go: `string` |
| `sessionObjectSha256` | string | string | Go: `string` |
| `sessionSizeBytes` | integer | string(decimal) | Go: `int64` |
| `manifestObjectKey` | string | string | Go: `string` |
| `manifestContentSha256` | string | string | Go: `string` |
| `manifestObjectSha256` | string | string | Go: `string` |
| `manifestSizeBytes` | integer | string(decimal) | Go: `int64` |
| `taskExecutionSnapshotDigest` | string | string | Go: `string` |
| `expectedBaseCommit` | string | string | Go: `string` |
| `expectedResultCommit` | string | string | Go: `string` |
| `expectedResultTree` | string | string | Go: `string` |
| `sessionLineageDigest` | string | string | Go: `string` |
| `encryptionAlgorithm` | string | string | Go: `string` |
| `encryptionKeyId` | string | string | Go: `string` |
| `wrappedDataKey` | string | string | Go: `string` |

<a id="t-504ca07e3c7de7c9"></a>

## `internal/app/workforce/continuation/domain.Turn`

[seven-agent-force-server/internal/app/workforce/continuation/domain/model.go:101](../../../seven-agent-force-server/internal/app/workforce/continuation/domain/model.go#L101)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `turnNumber` | integer | integer | Go: `int` |
| `instruction` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `sourceTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `sourceCheckpointId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `executionSnapshotDigest` | string | string | Go: `string` |
| `continuationLineageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `createdBy` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultCommit` | string | string | 零值可省略；Go: `string` |
| `resultTree` | string | string | 零值可省略；Go: `string` |
| `resultSummary` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-103450ff23116498"></a>

## `internal/app/workforce/delivery/application.AttemptView`

[seven-agent-force-server/internal/app/workforce/delivery/application/service.go:86](../../../seven-agent-force-server/internal/app/workforce/delivery/application/service.go#L86)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `requestId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptNumber` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerInstanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `phase` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `remoteBefore` | string | string | 零值可省略；Go: `string` |
| `remoteAfter` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `verifiedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-926bdefce296b43c"></a>

## `internal/app/workforce/delivery/application.DeliveryClaim`

[seven-agent-force-server/internal/app/workforce/delivery/application/service.go:155](../../../seven-agent-force-server/internal/app/workforce/delivery/application/service.go#L155)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `requestId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `actionType` | string | string | Go: `string` |
| `action` | integer | integer | [`internal/app/workforce/delivery/domain.Action`](wire-types.md#t-0b32913a8ffe942c)；Go: `internal/app/workforce/delivery/domain.Action` |
| `processorReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `processorDigest` | string | string | Go: `string` |
| `processorArchiveObjectKey` | string | string | Go: `string` |
| `harnessImageDigest` | string | string | Go: `string` |
| `attemptNumber` | integer | integer | Go: `int` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `leaseExpiresAt` | string | string | Go: `string` |
| `plainCheckpointKeyBase64` | string | string | Go: `string` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `repositoryBundleObjectKey` | string | string | Go: `string` |
| `repositoryBundleContentSha256` | string | string | Go: `string` |
| `repositoryBundleObjectSha256` | string | string | Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | Go: `int64` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `selectionDigest` | string | string | Go: `string` |
| `expectedBaseCommit` | string | string | Go: `string` |
| `deliveryCommit` | string | string | Go: `string` |
| `commitTimestamp` | string | string | Go: `string` |
| `baseBranch` | string | string | Go: `string` |
| `destinationBranch` | string | string | Go: `string` |
| `target` | integer | integer | [`internal/app/workforce/delivery/domain.Target`](wire-types.md#t-0afa87070e3e8dfd)；Go: `internal/app/workforce/delivery/domain.Target` |
| `reconciliationOnly` | boolean | boolean | 零值可省略；Go: `bool` |

<a id="t-8554e566709ed2aa"></a>

## `internal/app/workforce/delivery/application.DeliveryCredential`

[seven-agent-force-server/internal/app/workforce/delivery/application/service.go:127](../../../seven-agent-force-server/internal/app/workforce/delivery/application/service.go#L127)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `username` | string | string | 零值可省略；Go: `string` |
| `password` | string | string | 零值可省略；Go: `string` |
| `privateKey` | string | string | 零值可省略；Go: `string` |
| `knownHosts` | string | string | 零值可省略；Go: `string` |
| `expiresAt` | string | string | Go: `string` |

<a id="t-1a585eac31214972"></a>

## `internal/app/workforce/delivery/application.DeliveryResult`

[seven-agent-force-server/internal/app/workforce/delivery/application/service.go:193](../../../seven-agent-force-server/internal/app/workforce/delivery/application/service.go#L193)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `remoteBefore` | string | string | Go: `string` |
| `remoteAfter` | string | string | Go: `string` |
| `readable` | boolean | boolean | Go: `bool` |
| `pushStarted` | boolean | boolean | Go: `bool` |

<a id="t-71652dd9dd9b2e1f"></a>

## `internal/app/workforce/delivery/application.PrepareResult`

[seven-agent-force-server/internal/app/workforce/delivery/application/service.go:189](../../../seven-agent-force-server/internal/app/workforce/delivery/application/service.go#L189)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `pushAuthorization` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |

<a id="t-8b3f058d77519df7"></a>

## `internal/app/workforce/delivery/application.RequestView`

[seven-agent-force-server/internal/app/workforce/delivery/application/service.go:54](../../../seven-agent-force-server/internal/app/workforce/delivery/application/service.go#L54)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `actionType` | string | string | Go: `string` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `baseBranch` | string | string | Go: `string` |
| `destinationBranch` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `selectionDigest` | string | string | Go: `string` |
| `deliveryCommit` | string | string | Go: `string` |
| `approvalMode` | string | string | Go: `string` |
| `approvedBy` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `approvedAt` | string | string | 零值可省略；Go: `string` |
| `status` | string | string | Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | Go: `string` |
| `review` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/delivery/application.ReviewView`](wire-types.md#t-4affc308a4b503af)；Go: `*internal/app/workforce/delivery/application.ReviewView` |

<a id="t-e125ce21366e091f"></a>

## `internal/app/workforce/delivery/application.ReviewReconciliationResult`

[seven-agent-force-server/internal/app/workforce/delivery/application/review.go:59](../../../seven-agent-force-server/internal/app/workforce/delivery/application/review.go#L59)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |

<a id="t-4affc308a4b503af"></a>

## `internal/app/workforce/delivery/application.ReviewView`

[seven-agent-force-server/internal/app/workforce/delivery/application/service.go:77](../../../seven-agent-force-server/internal/app/workforce/delivery/application/service.go#L77)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `providerType` | string | string | Go: `string` |
| `providerNumber` | string | string | 零值可省略；Go: `string` |
| `webUrl` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |

<a id="t-0b32913a8ffe942c"></a>

## `internal/app/workforce/delivery/domain.Action`

[seven-agent-force-server/internal/app/workforce/delivery/domain/model.go:132](../../../seven-agent-force-server/internal/app/workforce/delivery/domain/model.go#L132)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `type` | string | string | Go: `string` |
| `when` | string | string | Go: `string` |
| `targetId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `baseBranch` | string | string | 零值可省略；Go: `string` |
| `branch` | string | string | 零值可省略；Go: `string` |
| `operation` | string | string | 零值可省略；Go: `string` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |

<a id="t-f1aac88c550d6e15"></a>

## `internal/app/workforce/delivery/domain.EgressPolicy`

[seven-agent-force-server/internal/app/workforce/delivery/domain/model.go:72](../../../seven-agent-force-server/internal/app/workforce/delivery/domain/model.go#L72)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `allowPrivateNetwork` | boolean | boolean | Go: `bool` |
| `allowedHosts` | array<string> | array<string> | Go: `[]string` |
| `allowedCidrs` | array<string> | array<string> | Go: `[]string` |
| `allowedPorts` | array<integer> | array<integer> | Go: `[]int` |

<a id="t-b89dd47cf7d38740"></a>

## `internal/app/workforce/delivery/domain.Message`

[seven-agent-force-server/internal/app/workforce/delivery/domain/model.go:211](../../../seven-agent-force-server/internal/app/workforce/delivery/domain/model.go#L211)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `kind` | string | string | Go: `string` |
| `messageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-0afa87070e3e8dfd"></a>

## `internal/app/workforce/delivery/domain.Target`

[seven-agent-force-server/internal/app/workforce/delivery/domain/model.go:42](../../../seven-agent-force-server/internal/app/workforce/delivery/domain/model.go#L42)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `providerType` | string | string | 零值可省略；Go: `string` |
| `transport` | string | string | Go: `string` |
| `host` | string | string | 零值可省略；Go: `string` |
| `port` | integer | integer | 零值可省略；Go: `int` |
| `repositoryPath` | string | string | Go: `string` |
| `webRepositoryUrl` | string | string | 零值可省略；Go: `string` |
| `credentialRef` | string | string | 零值可省略；Go: `string` |
| `credentialVersion` | string | string | 零值可省略；Go: `string` |
| `sshKnownHostFingerprint` | string | string | 零值可省略；Go: `string` |
| `tlsPolicy` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `egress` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `egressPolicyDigest` | string | string | Go: `string` |
| `commitAuthorName` | string | string | Go: `string` |
| `commitAuthorEmail` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `configDigest` | string | string | Go: `string` |
| `lastProbeStatus` | string | string | 零值可省略；Go: `string` |
| `lastProbeReasonCode` | string | string | 零值可省略；Go: `string` |
| `lastProbedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-510c99b41bf507eb"></a>

## `internal/app/workforce/delivery/facade.ImmediateToolResult`

[seven-agent-force-server/internal/app/workforce/delivery/facade/tool.go:58](../../../seven-agent-force-server/internal/app/workforce/delivery/facade/tool.go#L58)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `RoundKey` | string | string | Go: `string` |
| `LocalToolCallKey` | string | string | Go: `string` |
| `NativeToolCallID` | string | string | Go: `string` |
| `IntentID` | integer | string(decimal) | Go: `int64` |
| `Status` | string | string | Go: `string` |
| `Summary` | string | string | Go: `string` |
| `ResultDigest` | string | string | Go: `string` |
| `ReasonCode` | string | string | Go: `string` |

<a id="t-c029d6a1620fc30f"></a>

## `internal/app/workforce/delivery/facade.IntentReceipt`

[seven-agent-force-server/internal/app/workforce/delivery/facade/tool.go:49](../../../seven-agent-force-server/internal/app/workforce/delivery/facade/tool.go#L49)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `intentId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `deliveryMode` | string | string | Go: `string` |
| `deliveryAuthorization` | string | string | Go: `string` |
| `replayed` | boolean | boolean | Go: `bool` |

<a id="t-3d838b862a576aca"></a>

## `internal/app/workforce/executionaccess/domain.Descriptor`

[seven-agent-force-server/internal/app/workforce/executionaccess/domain/model.go:98](../../../seven-agent-force-server/internal/app/workforce/executionaccess/domain/model.go#L98)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `purpose` | string | string | Go: `string` |
| `command` | string | string | Go: `string` |
| `cwd` | string | string | Go: `string` |
| `targets` | array<integer> | array<integer> | [`internal/app/workforce/executionaccess/domain.Target`](wire-types.md#t-2bf396eb201653fa)；Go: `[]internal/app/workforce/executionaccess/domain.Target` |

<a id="t-2bf396eb201653fa"></a>

## `internal/app/workforce/executionaccess/domain.Target`

[seven-agent-force-server/internal/app/workforce/executionaccess/domain/model.go:90](../../../seven-agent-force-server/internal/app/workforce/executionaccess/domain/model.go#L90)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `host` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `port` | integer | integer | Go: `int` |

<a id="t-3bc3f33391ceb263"></a>

## `internal/app/workforce/executionaccess/facade.Claim`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:245](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L245)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operation` | integer | integer | [`internal/app/workforce/executionaccess/facade.OperationView`](wire-types.md#t-5a1a374de13328c7)；Go: `internal/app/workforce/executionaccess/facade.OperationView` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `leaseExpiresAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-1ce509dc6c1a2270"></a>

## `internal/app/workforce/executionaccess/facade.ClaimRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:139](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L139)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-1b3600b4245f2c82"></a>

## `internal/app/workforce/executionaccess/facade.CloseRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:176](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L176)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |

<a id="t-dcbfaad20ae4affa"></a>

## `internal/app/workforce/executionaccess/facade.DecisionRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:127](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L127)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `action` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-5b0d7211fb5465e6"></a>

## `internal/app/workforce/executionaccess/facade.GrantOperationView`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:236](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L236)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `status` | string | string | Go: `string` |
| `resultSummary` | string | string | 零值可省略；Go: `string` |
| `resultDeliveryStatus` | string | string | Go: `string` |
| `exitCode` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `agentContinued` | boolean | boolean | Go: `bool` |
| `finishedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |

<a id="t-6ba560fbbed25f2c"></a>

## `internal/app/workforce/executionaccess/facade.GrantView`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:221](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L221)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceDecisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `targets` | array<integer> | array<integer> | [`internal/app/workforce/executionaccess/domain.Target`](wire-types.md#t-2bf396eb201653fa)；Go: `[]internal/app/workforce/executionaccess/domain.Target` |
| `status` | string | string | Go: `string` |
| `actorUserId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `purpose` | string | string | Go: `string` |
| `lastOperation` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/executionaccess/facade.GrantOperationView`](wire-types.md#t-5b0d7211fb5465e6)；Go: `*internal/app/workforce/executionaccess/facade.GrantOperationView` |
| `expiresAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-5a1a374de13328c7"></a>

## `internal/app/workforce/executionaccess/facade.OperationView`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:198](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L198)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `purpose` | string | string | Go: `string` |
| `targets` | array<integer> | array<integer> | [`internal/app/workforce/executionaccess/domain.Target`](wire-types.md#t-2bf396eb201653fa)；Go: `[]internal/app/workforce/executionaccess/domain.Target` |
| `cwd` | string | string | Go: `string` |
| `command` | string | string | Go: `string` |
| `resultDeliveryStatus` | string | string | Go: `string` |
| `agentContinued` | boolean | boolean | Go: `bool` |
| `descriptorDigest` | string | string | 零值可省略；Go: `string` |
| `resultDigest` | string | string | 零值可省略；Go: `string` |
| `resultSummary` | string | string | 零值可省略；Go: `string` |
| `exitCode` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `expiresAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-c972565d54b97cae"></a>

## `internal/app/workforce/executionaccess/facade.PendingSuspendOperation`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:251](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L251)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `nativeToolCallId` | string | string | Go: `string` |

<a id="t-80b439993ea624bb"></a>

## `internal/app/workforce/executionaccess/facade.PublishRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:118](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L118)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `nativeToolCallId` | string | string | Go: `string` |
| `toolName` | string | string | Go: `string` |
| `argumentsDigest` | string | string | Go: `string` |
| `arguments` | integer | integer | [`internal/app/workforce/executionaccess/domain.Descriptor`](wire-types.md#t-3d838b862a576aca)；Go: `internal/app/workforce/executionaccess/domain.Descriptor` |

<a id="t-ea2d168ef71fda6e"></a>

## `internal/app/workforce/executionaccess/facade.Receipt`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:187](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L187)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `grantId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `resultDigest` | string | string | 零值可省略；Go: `string` |
| `replayed` | boolean | boolean | Go: `bool` |

<a id="t-173b66c0501009d7"></a>

## `internal/app/workforce/executionaccess/facade.RenewRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:151](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L151)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-e21dea01113fc815"></a>

## `internal/app/workforce/executionaccess/facade.ResultAckRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:181](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L181)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `nativeResolved` | boolean | boolean | Go: `bool` |
| `agentContinued` | boolean | boolean | Go: `bool` |

<a id="t-45900bc030591739"></a>

## `internal/app/workforce/executionaccess/facade.ResultReceiptRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:170](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L170)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultDigest` | string | string | Go: `string` |

<a id="t-57e2a75e70cf2ed5"></a>

## `internal/app/workforce/executionaccess/facade.ResultRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:156](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L156)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `resultDigest` | string | string | Go: `string` |
| `exitCode` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `summary` | string | string | Go: `string` |
| `output` | string | string | Go: `string` |

<a id="t-a97139764dd69d1f"></a>

## `internal/app/workforce/executionaccess/facade.RevokeRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:134](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L134)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-e87acbc7a9d6bc90"></a>

## `internal/app/workforce/executionaccess/facade.StartRequest`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:144](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L144)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `containerId` | string | string | Go: `string` |
| `relayIdentity` | string | string | Go: `string` |

<a id="t-fc29e584943866df"></a>

## `internal/app/workforce/executionaccess/facade.ToolDefinition`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:19](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L19)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `inputSchema` | object<string, 任意 JSON 值> | object<string, 任意 JSON 值> | Go: `map[string]any` |

<a id="t-9097658de2992bc0"></a>

## `internal/app/workforce/executionaccess/facade.ToolResult`

[seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go:258](../../../seven-agent-force-server/internal/app/workforce/executionaccess/facade/contracts.go#L258)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `nativeToolCallId` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `exitCode` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `summary` | string | string | 零值可省略；Go: `string` |
| `output` | string | string | 零值可省略；Go: `string` |
| `resultDigest` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |

<a id="t-d3776436024e6c09"></a>

## `internal/app/workforce/executionrouting/facade.RuntimeOption`

[seven-agent-force-server/internal/app/workforce/executionrouting/facade/contracts.go:42](../../../seven-agent-force-server/internal/app/workforce/executionrouting/facade/contracts.go#L42)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `harnessType` | string | string | Go: `string` |
| `harnessVersion` | string | string | Go: `string` |
| `adapterVersion` | string | string | Go: `string` |
| `imageRef` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `providerProtocols` | array<string> | array<string> | Go: `[]string` |
| `environmentCapabilities` | array<string> | array<string> | Go: `[]string` |
| `executionKinds` | array<string> | array<string> | Go: `[]string` |
| `hookEvents` | array<string> | array<string> | Go: `[]string` |
| `supportsIndependentReview` | boolean | boolean | Go: `bool` |
| `waitingTaskCount` | integer | integer | Go: `int` |
| `availableSlots` | integer | integer | Go: `int` |
| `recommended` | boolean | boolean | Go: `bool` |
| `workers` | array<integer> | array<integer> | [`internal/app/workforce/executionrouting/facade.RuntimeWorkerOption`](wire-types.md#t-e7e04519a7a2fc0c)；Go: `[]internal/app/workforce/executionrouting/facade.RuntimeWorkerOption` |

<a id="t-ba843f200612eb8f"></a>

## `internal/app/workforce/executionrouting/facade.RuntimeOptions`

[seven-agent-force-server/internal/app/workforce/executionrouting/facade/contracts.go:63](../../../seven-agent-force-server/internal/app/workforce/executionrouting/facade/contracts.go#L63)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `items` | array<integer> | array<integer> | [`internal/app/workforce/executionrouting/facade.RuntimeOption`](wire-types.md#t-d3776436024e6c09)；Go: `[]internal/app/workforce/executionrouting/facade.RuntimeOption` |

<a id="t-e7e04519a7a2fc0c"></a>

## `internal/app/workforce/executionrouting/facade.RuntimeWorkerOption`

[seven-agent-force-server/internal/app/workforce/executionrouting/facade/contracts.go:32](../../../seven-agent-force-server/internal/app/workforce/executionrouting/facade/contracts.go#L32)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `state` | string | string | Go: `string` |
| `reservedAttempts` | integer | integer | Go: `int` |
| `startingAttempts` | integer | integer | Go: `int` |
| `runningAttempts` | integer | integer | Go: `int` |
| `availableAttempts` | integer | integer | Go: `int` |

<a id="t-50d7a0f71c7be8a2"></a>

## `internal/app/workforce/hook/application.CreatePackageCommand`

[seven-agent-force-server/internal/app/workforce/hook/application/service.go:34](../../../seven-agent-force-server/internal/app/workforce/hook/application/service.go#L34)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `category` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/hook/domain.Manifest`](wire-types.md#t-e0fc1343c8a7c48b)；Go: `internal/app/workforce/hook/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/hook/domain.SourceFile`](wire-types.md#t-1d69f951596ffc41)；Go: `[]internal/app/workforce/hook/domain.SourceFile` |

<a id="t-0f4bfabd9d4f9616"></a>

## `internal/app/workforce/hook/application.TestCommand`

[seven-agent-force-server/internal/app/workforce/hook/application/service.go:99](../../../seven-agent-force-server/internal/app/workforce/hook/application/service.go#L99)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `event` | string | string | Go: `string` |
| `fixture` | object<string, 任意 JSON 值> | object<string, 任意 JSON 值> | Go: `map[string]any` |

<a id="t-8864a7d84d8c394a"></a>

## `internal/app/workforce/hook/application.TestResult`

[seven-agent-force-server/internal/app/workforce/hook/application/service.go:103](../../../seven-agent-force-server/internal/app/workforce/hook/application/service.go#L103)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `schemaVersion` | integer | integer | Go: `int` |
| `decision` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `operatorMessage` | string | string | Go: `string` |
| `evidence` | array<object<string, 任意 JSON 值>> | array<object<string, 任意 JSON 值>> | Go: `[]map[string]any` |
| `diagnostics` | string | string | Go: `string` |
| `durationMs` | integer | string(decimal) | Go: `int64` |

<a id="t-73a8d26485c0c77a"></a>

## `internal/app/workforce/hook/domain.Entrypoint`

[seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go:43](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go#L43)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `event` | string | string | Go: `string` |
| `path` | string | string | Go: `string` |
| `timeoutMs` | integer | integer | Go: `int` |
| `failureMode` | string | string | Go: `string` |

<a id="t-e1448099ac2a7abc"></a>

## `internal/app/workforce/hook/domain.HookResult`

[seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go:72](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go#L72)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `decision` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `operatorMessage` | string | string | Go: `string` |
| `evidence` | array<object<string, 任意 JSON 值>> | array<object<string, 任意 JSON 值>> | Go: `[]map[string]any` |

<a id="t-97b9075404ac64a6"></a>

## `internal/app/workforce/hook/domain.Limits`

[seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go:50](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go#L50)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `stdinBytes` | integer | string(decimal) | Go: `int64` |
| `stdoutBytes` | integer | string(decimal) | Go: `int64` |
| `stderrBytes` | integer | string(decimal) | Go: `int64` |

<a id="t-e0fc1343c8a7c48b"></a>

## `internal/app/workforce/hook/domain.Manifest`

[seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go:56](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go#L56)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `version` | string | string | 零值可省略；Go: `string` |
| `trustClass` | string | string | Go: `string` |
| `runtime` | integer | integer | [`internal/app/workforce/hook/domain.Runtime`](wire-types.md#t-7032c4d20c895cc6)；Go: `internal/app/workforce/hook/domain.Runtime` |
| `entrypoints` | array<integer> | array<integer> | [`internal/app/workforce/hook/domain.Entrypoint`](wire-types.md#t-73a8d26485c0c77a)；Go: `[]internal/app/workforce/hook/domain.Entrypoint` |
| `limits` | integer | integer | [`internal/app/workforce/hook/domain.Limits`](wire-types.md#t-97b9075404ac64a6)；Go: `internal/app/workforce/hook/domain.Limits` |

<a id="t-7032c4d20c895cc6"></a>

## `internal/app/workforce/hook/domain.Runtime`

[seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go:38](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go#L38)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `kind` | string | string | Go: `string` |
| `requirement` | string | string | 零值可省略；Go: `string` |

<a id="t-1d69f951596ffc41"></a>

## `internal/app/workforce/hook/domain.SourceFile`

[seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go:66](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go#L66)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `path` | string | string | Go: `string` |
| `content` | string | string | Go: `string` |
| `mode` | integer | string(decimal) | 零值可省略；Go: `int64` |

<a id="t-87d6c7ecb648bdbd"></a>

## `internal/app/workforce/hook/domain.Validation`

[seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go:80](../../../seven-agent-force-server/internal/app/workforce/hook/domain/protocol.go#L80)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `valid` | boolean | boolean | Go: `bool` |
| `reasonCode` | string | string | Go: `string` |
| `errors` | array<string> | array<string> | Go: `[]string` |
| `manifestSha256` | string | string | 零值可省略；Go: `string` |
| `sourceSha256` | string | string | 零值可省略；Go: `string` |

<a id="t-71a75838e8fcb76f"></a>

## `internal/app/workforce/hook/infrastructure.AuditEvent`

[seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go:58](../../../seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go#L58)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `eventType` | string | string | Go: `string` |
| `resourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `actor` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `details` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `createdAt` | string | string | Go: `string` |

<a id="t-9f2ee9c0fc8badec"></a>

## `internal/app/workforce/hook/infrastructure.HarnessChannel`

[seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go:69](../../../seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go#L69)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `alias` | string | string | Go: `string` |
| `harnessReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `harnessType` | string | string | Go: `string` |
| `harnessVersion` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `supervisorVersion` | string | string | Go: `string` |

<a id="t-b2b94d9743d02662"></a>

## `internal/app/workforce/hook/infrastructure.Package`

[seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go:29](../../../seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go#L29)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `category` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `trustClass` | string | string | Go: `string` |
| `revisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revision` | integer | string(decimal) | Go: `int64` |
| `revisionStatus` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/hook/domain.Manifest`](wire-types.md#t-e0fc1343c8a7c48b)；Go: `internal/app/workforce/hook/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/hook/domain.SourceFile`](wire-types.md#t-1d69f951596ffc41)；Go: `[]internal/app/workforce/hook/domain.SourceFile` |
| `validation` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `latestVersion` | string | string | 零值可省略；Go: `string` |
| `updatedAt` | string | string | Go: `string` |

<a id="t-e2d455e6a41bf699"></a>

## `internal/app/workforce/hook/infrastructure.Release`

[seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go:46](../../../seven-agent-force-server/internal/app/workforce/hook/infrastructure/store.go#L46)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `packageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string | string | Go: `string` |
| `runtimeKind` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/hook/domain.Manifest`](wire-types.md#t-e0fc1343c8a7c48b)；Go: `internal/app/workforce/hook/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/hook/domain.SourceFile`](wire-types.md#t-1d69f951596ffc41)；Go: `[]internal/app/workforce/hook/domain.SourceFile` |
| `archiveSha256` | string | string | Go: `string` |
| `archiveSizeBytes` | integer | string(decimal) | Go: `int64` |
| `publishedAt` | string | string | Go: `string` |

<a id="t-db8a278ae107eba0"></a>

## `internal/app/workforce/imageadmission/facade.IndependentReviewImageOption`

[seven-agent-force-server/internal/app/workforce/imageadmission/facade/independent_review.go:25](../../../seven-agent-force-server/internal/app/workforce/imageadmission/facade/independent_review.go#L25)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `digest` | string | string | Go: `string` |
| `displayName` | string | string | Go: `string` |
| `capabilities` | array<string> | array<string> | Go: `[]string` |
| `available` | boolean | boolean | Go: `bool` |

<a id="t-96c911e27b1bea72"></a>

## `internal/app/workforce/imageadmission/facade.IndependentReviewImagePage`

[seven-agent-force-server/internal/app/workforce/imageadmission/facade/independent_review.go:32](../../../seven-agent-force-server/internal/app/workforce/imageadmission/facade/independent_review.go#L32)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `items` | array<integer> | array<integer> | [`internal/app/workforce/imageadmission/facade.IndependentReviewImageOption`](wire-types.md#t-db8a278ae107eba0)；Go: `[]internal/app/workforce/imageadmission/facade.IndependentReviewImageOption` |
| `nextCursor` | string | string | 零值可省略；Go: `string` |

<a id="t-b482691f81865689"></a>

## `internal/app/workforce/interaction/domain.Answer`

[seven-agent-force-server/internal/app/workforce/interaction/domain/model.go:51](../../../seven-agent-force-server/internal/app/workforce/interaction/domain/model.go#L51)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `questionKey` | string | string | Go: `string` |
| `selectedOptionKeys` | array<string> | array<string> | Go: `[]string` |
| `text` | string | string | Go: `string` |

<a id="t-e93f6f780725fb07"></a>

## `internal/app/workforce/interaction/domain.Conversation`

[seven-agent-force-server/internal/app/workforce/interaction/domain/model.go:96](../../../seven-agent-force-server/internal/app/workforce/interaction/domain/model.go#L96)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `items` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Item`](wire-types.md#t-f32fa9166a2bd4ba)；Go: `[]internal/app/workforce/interaction/domain.Item` |
| `nextCursor` | string 或 null | string 或 null | Go: `*string` |
| `watermark` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `allowedActions` | array<string> | array<string> | Go: `[]string` |

<a id="t-7860a3364778775b"></a>

## `internal/app/workforce/interaction/domain.Interaction`

[seven-agent-force-server/internal/app/workforce/interaction/domain/model.go:56](../../../seven-agent-force-server/internal/app/workforce/interaction/domain/model.go#L56)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `localRequestKey` | string | string | Go: `string` |
| `roundKey` | string | string | 零值可省略；Go: `string` |
| `kind` | string | string | Go: `string` |
| `state` | string | string | Go: `string` |
| `deliveryState` | string | string | Go: `string` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `questions` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/interaction/domain.Question`](wire-types.md#t-5c2e63b06a84f495)；Go: `[]internal/app/workforce/interaction/domain.Question` |
| `isBlocking` | boolean | boolean | Go: `bool` |
| `text` | string | string | 零值可省略；Go: `string` |
| `responseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `answers` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/interaction/domain.Answer`](wire-types.md#t-b482691f81865689)；Go: `[]internal/app/workforce/interaction/domain.Answer` |
| `resumeTaskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `expiresAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-f32fa9166a2bd4ba"></a>

## `internal/app/workforce/interaction/domain.Item`

[seven-agent-force-server/internal/app/workforce/interaction/domain/model.go:86](../../../seven-agent-force-server/internal/app/workforce/interaction/domain/model.go#L86)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `type` | string | string | Go: `string` |
| `body` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-d804ae316192a1ae"></a>

## `internal/app/workforce/interaction/domain.Option`

[seven-agent-force-server/internal/app/workforce/interaction/domain/model.go:38](../../../seven-agent-force-server/internal/app/workforce/interaction/domain/model.go#L38)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `key` | string | string | Go: `string` |
| `label` | string | string | Go: `string` |

<a id="t-5c2e63b06a84f495"></a>

## `internal/app/workforce/interaction/domain.Question`

[seven-agent-force-server/internal/app/workforce/interaction/domain/model.go:42](../../../seven-agent-force-server/internal/app/workforce/interaction/domain/model.go#L42)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `key` | string | string | Go: `string` |
| `title` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `kind` | string | string | Go: `string` |
| `required` | boolean | boolean | Go: `bool` |
| `allowFreeText` | boolean | boolean | Go: `bool` |
| `options` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Option`](wire-types.md#t-d804ae316192a1ae)；Go: `[]internal/app/workforce/interaction/domain.Option` |

<a id="t-f78ccd181810d5f2"></a>

## `internal/app/workforce/interaction/facade.AckRequest`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:78](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L78)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `responseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-7b335ae1558c40fe"></a>

## `internal/app/workforce/interaction/facade.CloseRequest`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:113](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L113)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localRequestKey` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |

<a id="t-8e269ecf7e1da3cf"></a>

## `internal/app/workforce/interaction/facade.Delivery`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:93](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L93)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `coldResume` | boolean | boolean | 零值可省略；Go: `bool` |
| `localRequestKey` | string | string | Go: `string` |
| `roundKey` | string | string | 零值可省略；Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `responseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `answers` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/interaction/domain.Answer`](wire-types.md#t-b482691f81865689)；Go: `[]internal/app/workforce/interaction/domain.Answer` |
| `text` | string | string | 零值可省略；Go: `string` |

<a id="t-8a288654937fb4f8"></a>

## `internal/app/workforce/interaction/facade.MessageRequest`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:74](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L74)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `text` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-580203605653fbea"></a>

## `internal/app/workforce/interaction/facade.PendingQuestion`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:118](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L118)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `localRequestKey` | string | string | Go: `string` |
| `roundKey` | string | string | Go: `string` |
| `isBlocking` | boolean | boolean | Go: `bool` |
| `expiresAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-34ae7deae7e1a364"></a>

## `internal/app/workforce/interaction/facade.PublishEventRequest`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:173](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L173)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `localRequestKey` | string | string | Go: `string` |
| `kind` | string | string | Go: `string` |
| `text` | string | string | Go: `string` |

<a id="t-9b7bdcc957ddc524"></a>

## `internal/app/workforce/interaction/facade.PublishRequest`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:63](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L63)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | 零值可省略；Go: `string` |
| `localRequestKey` | string | string | Go: `string` |
| `questions` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Question`](wire-types.md#t-5c2e63b06a84f495)；Go: `[]internal/app/workforce/interaction/domain.Question` |
| `isBlocking` | boolean | boolean | Go: `bool` |

<a id="t-907dc4b4ec578277"></a>

## `internal/app/workforce/interaction/facade.QuestionPage`

[seven-agent-force-server/internal/app/workforce/interaction/facade/questions.go:6](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/questions.go#L6)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `items` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Item`](wire-types.md#t-f32fa9166a2bd4ba)；Go: `[]internal/app/workforce/interaction/domain.Item` |
| `nextCursor` | string 或 null | string 或 null | Go: `*string` |

<a id="t-72ef9cc8b517430e"></a>

## `internal/app/workforce/interaction/facade.Receipt`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:82](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L82)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `responseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `saved` | boolean | boolean | Go: `bool` |
| `delivered` | boolean | boolean | Go: `bool` |
| `executionContinued` | boolean | boolean | Go: `bool` |
| `replayed` | boolean | boolean | Go: `bool` |

<a id="t-151c76a6558793b8"></a>

## `internal/app/workforce/interaction/facade.ResponseRequest`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:69](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L69)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `answers` | array<integer> | array<integer> | [`internal/app/workforce/interaction/domain.Answer`](wire-types.md#t-b482691f81865689)；Go: `[]internal/app/workforce/interaction/domain.Answer` |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-4f02cea657e923a1"></a>

## `internal/app/workforce/interaction/facade.TaskSummary`

[seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go:149](../../../seven-agent-force-server/internal/app/workforce/interaction/facade/contracts.go#L149)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `pendingActionCount` | integer | integer | Go: `int` |
| `hasUnreadUpdates` | boolean | boolean | Go: `bool` |
| `activitySummary` | string | string | Go: `string` |
| `firstPendingActionKind` | string | string | 零值可省略；Go: `string` |
| `firstPendingActionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-937480b1b06b587d"></a>

## `internal/app/workforce/openintegration/application.APIKeyRotation`

[seven-agent-force-server/internal/app/workforce/openintegration/application/service.go:64](../../../seven-agent-force-server/internal/app/workforce/openintegration/application/service.go#L64)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `applicationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialVersion` | integer | integer | Go: `int` |
| `apiKey` | string | string | Go: `string` |
| `credentialHint` | string | string | Go: `string` |
| `previousCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `previousValidUntil` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-69d3652dc248495d"></a>

## `internal/app/workforce/openintegration/application.CreateApplicationRequest`

[seven-agent-force-server/internal/app/workforce/openintegration/application/service.go:49](../../../seven-agent-force-server/internal/app/workforce/openintegration/application/service.go#L49)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `webhookUrl` | string | string | Go: `string` |

<a id="t-ec45d8dc446df3d2"></a>

## `internal/app/workforce/openintegration/application.CreatedApplication`

[seven-agent-force-server/internal/app/workforce/openintegration/application/service.go:54](../../../seven-agent-force-server/internal/app/workforce/openintegration/application/service.go#L54)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `application` | integer | integer | [`internal/app/workforce/openintegration/domain.Application`](wire-types.md#t-07bad8b17300354e)；Go: `internal/app/workforce/openintegration/domain.Application` |
| `apiKey` | string | string | Go: `string` |
| `apiCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `apiCredentialVersion` | integer | integer | Go: `int` |
| `signingSecret` | string | string | 零值可省略；Go: `string` |
| `signingCredentialId` | string(decimal) 或 null | string(decimal) 或 null | 零值可省略；JSON 引号编码；Go: `*int64` |
| `signingCredentialVersion` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |

<a id="t-7ec6ae3c73899e4c"></a>

## `internal/app/workforce/openintegration/application.SigningSecretRotation`

[seven-agent-force-server/internal/app/workforce/openintegration/application/service.go:75](../../../seven-agent-force-server/internal/app/workforce/openintegration/application/service.go#L75)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `applicationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `signingCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `signingCredentialVersion` | integer | integer | Go: `int` |
| `signingSecret` | string | string | Go: `string` |
| `signingSecretHint` | string | string | Go: `string` |
| `previousCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `previousValidUntil` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-07bad8b17300354e"></a>

## `internal/app/workforce/openintegration/domain.Application`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:28](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L28)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `webhookUrl` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `apiCredential` | integer | integer | [`internal/app/workforce/openintegration/domain.CredentialSummary`](wire-types.md#t-de84ef1dcdf69379)；Go: `internal/app/workforce/openintegration/domain.CredentialSummary` |
| `signingCredential` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/openintegration/domain.CredentialSummary`](wire-types.md#t-de84ef1dcdf69379)；Go: `*internal/app/workforce/openintegration/domain.CredentialSummary` |
| `apiKeyGrace` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/openintegration/domain.CredentialGraceSummary`](wire-types.md#t-d2b21c2f0d68ede6)；Go: `*internal/app/workforce/openintegration/domain.CredentialGraceSummary` |
| `signingSecretGrace` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/openintegration/domain.CredentialGraceSummary`](wire-types.md#t-d2b21c2f0d68ede6)；Go: `*internal/app/workforce/openintegration/domain.CredentialGraceSummary` |
| `latestDeliveryStatus` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `lastUsedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |

<a id="t-a9f026f9297558d7"></a>

## `internal/app/workforce/openintegration/domain.CreateOpenTaskRequest`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/task_submission.go:21](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/task_submission.go#L21)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | 零值可省略；Go: `string` |
| `instruction` | string | string | Go: `string` |
| `tags` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `referenceAssetIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `additionalCompletionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewEnabled` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `deliveryPreference` | string | string | 零值可省略；Go: `string` |
| `modelSelection` | integer | integer | [`internal/app/workforce/task/facade.ModelSelection`](wire-types.md#t-c395a518b87a2f05)；Go: `internal/app/workforce/task/facade.ModelSelection` |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionTimeoutSeconds` | integer | integer | 零值可省略；Go: `int` |
| `externalCorrelationId` | string | string | Go: `string` |

<a id="t-d2b21c2f0d68ede6"></a>

## `internal/app/workforce/openintegration/domain.CredentialGraceSummary`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:22](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L22)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | integer | integer | Go: `int` |
| `validUntil` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-de84ef1dcdf69379"></a>

## `internal/app/workforce/openintegration/domain.CredentialSummary`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:16](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L16)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | integer | integer | Go: `int` |
| `hint` | string | string | Go: `string` |

<a id="t-0d9bb0a0809d94fd"></a>

## `internal/app/workforce/openintegration/domain.Delivery`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:148](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L148)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `deliveryId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `eventId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `eventType` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `attemptCount` | integer | integer | Go: `int` |
| `lastHttpStatus` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `nextAttemptAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-1196cc6ebf2da3dd"></a>

## `internal/app/workforce/openintegration/domain.EventData`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:140](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L140)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `resultSummary` | string | string | Go: `string` |

<a id="t-6b662db50ad63d8f"></a>

## `internal/app/workforce/openintegration/domain.EventLinks`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:130](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L130)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `task` | string | string | Go: `string` |
| `outputs` | string | string | Go: `string` |

<a id="t-e46c9d5c3b65ecfc"></a>

## `internal/app/workforce/openintegration/domain.EventSubject`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:135](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L135)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `type` | string | string | Go: `string` |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-8c5176b8aadb7e02"></a>

## `internal/app/workforce/openintegration/domain.ModelOption`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:188](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L188)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `model` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `label` | string | string | Go: `string` |

<a id="t-9ac1da81b7add025"></a>

## `internal/app/workforce/openintegration/domain.ModelOptions`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:194](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L194)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `items` | array<integer> | array<integer> | [`internal/app/workforce/openintegration/domain.ModelOption`](wire-types.md#t-8c5176b8aadb7e02)；Go: `[]internal/app/workforce/openintegration/domain.ModelOption` |

<a id="t-b6ca673c5d930605"></a>

## `internal/app/workforce/openintegration/domain.OpenCapabilitiesView`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:55](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L55)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `applicationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskSubmission` | integer | integer | [`internal/app/workforce/openintegration/domain.OpenTaskSubmissionCapability`](wire-types.md#t-fcf475b6258d5d34)；Go: `internal/app/workforce/openintegration/domain.OpenTaskSubmissionCapability` |

<a id="t-2b91130460c2d155"></a>

## `internal/app/workforce/openintegration/domain.OpenTaskOutput`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:100](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L100)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `outputId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `outputKind` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `mediaType` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `links` | integer | integer | [`internal/app/workforce/openintegration/domain.OutputLinks`](wire-types.md#t-3de4e21917370a7d)；Go: `internal/app/workforce/openintegration/domain.OutputLinks` |

<a id="t-927ddc5de2f63f68"></a>

## `internal/app/workforce/openintegration/domain.OpenTaskOutputsView`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:94](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L94)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `outputs` | array<integer> | array<integer> | [`internal/app/workforce/openintegration/domain.OpenTaskOutput`](wire-types.md#t-2b91130460c2d155)；Go: `[]internal/app/workforce/openintegration/domain.OpenTaskOutput` |

<a id="t-fcf475b6258d5d34"></a>

## `internal/app/workforce/openintegration/domain.OpenTaskSubmissionCapability`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:61](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L61)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `supportsSelectionDiscovery` | boolean | boolean | Go: `bool` |
| `requiresModelSelection` | boolean | boolean | Go: `bool` |
| `requiresRuntimeReleaseId` | boolean | boolean | Go: `bool` |
| `requiresProjectId` | boolean | boolean | Go: `bool` |
| `supportsReferenceAssets` | boolean | boolean | Go: `bool` |
| `supportsAdditionalCompletionCheck` | boolean | boolean | Go: `bool` |
| `supportsReviewToggle` | boolean | boolean | Go: `bool` |
| `supportsDeliveryPreference` | boolean | boolean | Go: `bool` |
| `supportsOutputs` | boolean | boolean | Go: `bool` |

<a id="t-834edc0113f0df5e"></a>

## `internal/app/workforce/openintegration/domain.OpenTaskView`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:73](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L73)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `replayed` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `externalCorrelationId` | string | string | Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `resultSummary` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | 零值可省略；Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |
| `links` | integer | integer | [`internal/app/workforce/openintegration/domain.TaskLinks`](wire-types.md#t-7adefca890a9ce38)；Go: `internal/app/workforce/openintegration/domain.TaskLinks` |

<a id="t-3de4e21917370a7d"></a>

## `internal/app/workforce/openintegration/domain.OutputLinks`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:111](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L111)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `download` | string | string | Go: `string` |

<a id="t-2bfe2160c458e778"></a>

## `internal/app/workforce/openintegration/domain.PublicEventEnvelope`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:115](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L115)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `eventId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `deliveryId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `eventType` | string | string | Go: `string` |
| `occurredAt` | string | string | Go: `string` |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `applicationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `correlationId` | string | string | Go: `string` |
| `subject` | integer | integer | [`internal/app/workforce/openintegration/domain.EventSubject`](wire-types.md#t-e46c9d5c3b65ecfc)；Go: `internal/app/workforce/openintegration/domain.EventSubject` |
| `data` | integer | integer | [`internal/app/workforce/openintegration/domain.EventData`](wire-types.md#t-1196cc6ebf2da3dd)；Go: `internal/app/workforce/openintegration/domain.EventData` |
| `links` | integer | integer | [`internal/app/workforce/openintegration/domain.EventLinks`](wire-types.md#t-6b662db50ad63d8f)；Go: `internal/app/workforce/openintegration/domain.EventLinks` |

<a id="t-5ff5be88eb43a56d"></a>

## `internal/app/workforce/openintegration/domain.RuntimeOption`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:199](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L199)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `recommended` | boolean | boolean | Go: `bool` |

<a id="t-1e4182a0d7935aca"></a>

## `internal/app/workforce/openintegration/domain.RuntimeOptions`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:204](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L204)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `items` | array<integer> | array<integer> | [`internal/app/workforce/openintegration/domain.RuntimeOption`](wire-types.md#t-5ff5be88eb43a56d)；Go: `[]internal/app/workforce/openintegration/domain.RuntimeOption` |

<a id="t-7adefca890a9ce38"></a>

## `internal/app/workforce/openintegration/domain.TaskLinks`

[seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go:89](../../../seven-agent-force-server/internal/app/workforce/openintegration/domain/model.go#L89)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operator` | string | string | 零值可省略；Go: `string` |
| `self` | string | string | Go: `string` |

<a id="t-c325b275f15ded1a"></a>

## `internal/app/workforce/project/application.DeliveryPolicy`

[seven-agent-force-server/internal/app/workforce/project/application/service.go:68](../../../seven-agent-force-server/internal/app/workforce/project/application/service.go#L68)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `mode` | string | string | Go: `string` |
| `authorization` | string | string | 零值可省略；Go: `string` |
| `branchPattern` | string | string | 零值可省略；Go: `string` |

<a id="t-ed1e365e6decc277"></a>

## `internal/app/workforce/project/application.NotificationPolicy`

[seven-agent-force-server/internal/app/workforce/project/application/service.go:74](../../../seven-agent-force-server/internal/app/workforce/project/application/service.go#L74)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `enabled` | boolean | boolean | Go: `bool` |
| `events` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `groupRouteId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-88b16e49b05c31f3"></a>

## `internal/app/workforce/project/application.RepositorySelection`

[seven-agent-force-server/internal/app/workforce/project/application/service.go:42](../../../seven-agent-force-server/internal/app/workforce/project/application/service.go#L42)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cloneUrl` | string | string | 零值可省略；Go: `string` |
| `defaultRefName` | string | string | Go: `string` |

<a id="t-9ef6a23d3ee49ef3"></a>

## `internal/app/workforce/project/application.UpsertRequest`

[seven-agent-force-server/internal/app/workforce/project/application/service.go:20](../../../seven-agent-force-server/internal/app/workforce/project/application/service.go#L20)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectKey` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `description` | string | string | 零值可省略；Go: `string` |
| `departmentId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `repository` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/project/application.RepositorySelection`](wire-types.md#t-88b16e49b05c31f3)；Go: `*internal/app/workforce/project/application.RepositorySelection` |
| `techStack` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `agentGuidance` | string | string | 零值可省略；Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewPrompt` | string | string | 零值可省略；Go: `string` |
| `hookReleaseIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `delivery` | integer | integer | [`internal/app/workforce/project/application.DeliveryPolicy`](wire-types.md#t-c325b275f15ded1a)；Go: `internal/app/workforce/project/application.DeliveryPolicy` |
| `notification` | integer | integer | [`internal/app/workforce/project/application.NotificationPolicy`](wire-types.md#t-ed1e365e6decc277)；Go: `internal/app/workforce/project/application.NotificationPolicy` |

<a id="t-07321946ab193e0f"></a>

## `internal/app/workforce/project/domain.HookRelease`

[seven-agent-force-server/internal/app/workforce/project/domain/model.go:76](../../../seven-agent-force-server/internal/app/workforce/project/domain/model.go#L76)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `releaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `hookId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |
| `events` | array<string> | array<string> | Go: `[]string` |
| `failureMode` | string | string | Go: `string` |

<a id="t-c7a80f6c44726b86"></a>

## `internal/app/workforce/project/domain.Project`

[seven-agent-force-server/internal/app/workforce/project/domain/model.go:21](../../../seven-agent-force-server/internal/app/workforce/project/domain/model.go#L21)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectKey` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `description` | string | string | 零值可省略；Go: `string` |
| `departmentId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `versionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `repositoryProvider` | string | string | 零值可省略；Go: `string` |
| `repositoryDisplayUrl` | string | string | 零值可省略；Go: `string` |
| `repositoryWebUrl` | string | string | 零值可省略；Go: `string` |
| `repositorySourceState` | string | string | 零值可省略；Go: `string` |
| `repositoryCapabilities` | integer | integer | 零值可省略；[`internal/app/workforce/project/domain.RepositoryCapabilities`](wire-types.md#t-561defb6f7c4d40e)；Go: `internal/app/workforce/project/domain.RepositoryCapabilities` |
| `repositoryReasonCode` | string | string | 零值可省略；Go: `string` |
| `repositoryLastCheckedAt` | string | string | 零值可省略；Go: `string` |
| `defaultRefName` | string | string | 零值可省略；Go: `string` |
| `departmentName` | string | string | 零值可省略；Go: `string` |
| `techStack` | array<string> | array<string> | Go: `[]string` |
| `agentGuidance` | string | string | 零值可省略；Go: `string` |
| `completionCheck` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `completionCheckDigest` | string | string | 零值可省略；Go: `string` |
| `resultProcessorReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `resultProcessorParameters` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `reviewPrompt` | string | string | 零值可省略；Go: `string` |
| `hookReleaseIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `hooks` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/project/domain.HookRelease`](wire-types.md#t-07321946ab193e0f)；Go: `[]internal/app/workforce/project/domain.HookRelease` |
| `deliveryMode` | string | string | Go: `string` |
| `deliveryAuthorization` | string | string | 零值可省略；Go: `string` |
| `resultBranchPattern` | string | string | 零值可省略；Go: `string` |
| `notificationEnabled` | boolean | boolean | Go: `bool` |
| `notificationEvents` | array<string> | array<string> | Go: `[]string` |
| `notificationGroupRouteId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `notificationGroupRouteName` | string | string | 零值可省略；Go: `string` |
| `taskCount` | integer | integer | Go: `int` |
| `activeTaskCount` | integer | integer | Go: `int` |
| `latestTaskAt` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | Go: `string` |
| `updatedAt` | string | string | Go: `string` |
| `archivedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-561defb6f7c4d40e"></a>

## `internal/app/workforce/project/domain.RepositoryCapabilities`

[seven-agent-force-server/internal/app/workforce/project/domain/repository.go:52](../../../seven-agent-force-server/internal/app/workforce/project/domain/repository.go#L52)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `read` | boolean | boolean | Go: `bool` |
| `push` | boolean | boolean | Go: `bool` |
| `draftReview` | boolean | boolean | Go: `bool` |

<a id="t-9783417562f6a21c"></a>

## `internal/app/workforce/project/facade.DeliveryRepositoryBinding`

[seven-agent-force-server/internal/app/workforce/project/facade/delivery_repository.go:7](../../../seven-agent-force-server/internal/app/workforce/project/facade/delivery_repository.go#L7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `ProjectID` | integer | string(decimal) | Go: `int64` |
| `VersionID` | integer | string(decimal) | Go: `int64` |
| `CredentialVersion` | integer | string(decimal) | Go: `int64` |
| `Current` | boolean | boolean | Go: `bool` |
| `ProviderType` | string | string | Go: `string` |
| `Scheme` | string | string | Go: `string` |
| `Host` | string | string | Go: `string` |
| `RepositoryPath` | string | string | Go: `string` |
| `WebURL` | string | string | Go: `string` |
| `ConfigDigest` | string | string | Go: `string` |
| `Port` | integer | integer | Go: `int` |
| `Push` | boolean | boolean | Go: `bool` |
| `DraftReview` | boolean | boolean | Go: `bool` |

<a id="t-8ba3f2e78e482030"></a>

## `internal/app/workforce/project/facade.ExecutionRequirements`

[seven-agent-force-server/internal/app/workforce/project/facade/contracts.go:25](../../../seven-agent-force-server/internal/app/workforce/project/facade/contracts.go#L25)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `executionKind` | string | string | Go: `string` |
| `adapterCapabilities` | array<string> | array<string> | Go: `[]string` |
| `environmentCapabilities` | array<string> | array<string> | Go: `[]string` |
| `allowedPlatformTools` | array<string> | array<string> | Go: `[]string` |
| `resourceClass` | string | string | Go: `string` |
| `requiresNativeResume` | boolean | boolean | Go: `bool` |
| `requiresStructuredOutput` | boolean | boolean | Go: `bool` |

<a id="t-e15055ab60508a60"></a>

## `internal/app/workforce/project/facade.FrozenGitPlan`

[seven-agent-force-server/internal/app/workforce/project/facade/contracts.go:45](../../../seven-agent-force-server/internal/app/workforce/project/facade/contracts.go#L45)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `projectId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `repositoryConfigDigest` | string | string | 零值可省略；Go: `string` |
| `repositoryCredentialVersion` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `repositoryDisplayUrl` | string | string | 零值可省略；Go: `string` |
| `repositoryProviderType` | string | string | 零值可省略；Go: `string` |
| `repositoryWebUrl` | string | string | 零值可省略；Go: `string` |
| `sourceRevisionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `defaultRefName` | string | string | 零值可省略；Go: `string` |
| `reviewBaseBranch` | string | string | 零值可省略；Go: `string` |
| `resultBranchPattern` | string | string | 零值可省略；Go: `string` |
| `deliveryMode` | string | string | Go: `string` |
| `deliveryAuthorization` | string | string | 零值可省略；Go: `string` |

<a id="t-ba172821bf070de9"></a>

## `internal/app/workforce/project/facade.FrozenTaskSpec`

[seven-agent-force-server/internal/app/workforce/project/facade/contracts.go:76](../../../seven-agent-force-server/internal/app/workforce/project/facade/contracts.go#L76)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionRequirements` | integer | integer | [`internal/app/workforce/project/facade.ExecutionRequirements`](wire-types.md#t-8ba3f2e78e482030)；Go: `internal/app/workforce/project/facade.ExecutionRequirements` |
| `executionRequirementsDigest` | string | string | Go: `string` |
| `projectName` | string | string | Go: `string` |
| `projectKind` | string | string | Go: `string` |
| `instruction` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `tags` | array<string> | array<string> | Go: `[]string` |
| `referenceAssetIds` | integer | integer | [`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `workspaceSourceRevisionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `resultProcessorReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `resultProcessorParameters` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `reviewEnabled` | boolean | boolean | Go: `bool` |
| `reviewPrompt` | string | string | 零值可省略；Go: `string` |
| `gitPlan` | integer | integer | [`internal/app/workforce/project/facade.FrozenGitPlan`](wire-types.md#t-e15055ab60508a60)；Go: `internal/app/workforce/project/facade.FrozenGitPlan` |
| `notification` | integer | integer | [`internal/app/workforce/project/facade.NotificationPolicy`](wire-types.md#t-aa6d80f578d23853)；Go: `internal/app/workforce/project/facade.NotificationPolicy` |

<a id="t-aa6d80f578d23853"></a>

## `internal/app/workforce/project/facade.NotificationPolicy`

[seven-agent-force-server/internal/app/workforce/project/facade/contracts.go:69](../../../seven-agent-force-server/internal/app/workforce/project/facade/contracts.go#L69)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `enabled` | boolean | boolean | Go: `bool` |
| `events` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `groupRouteId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-d419acabb6232ecd"></a>

## `internal/app/workforce/project/facade.TaskAdditions`

[seven-agent-force-server/internal/app/workforce/project/facade/contracts.go:35](../../../seven-agent-force-server/internal/app/workforce/project/facade/contracts.go#L35)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `instruction` | string | string | Go: `string` |
| `name` | string | string | 零值可省略；Go: `string` |
| `tags` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `referenceAssetIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `additionalCompletionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewEnabled` | boolean | boolean | Go: `bool` |
| `deliveryPreference` | string | string | 零值可省略；Go: `string` |

<a id="t-913df7d71f4540bb"></a>

## `internal/app/workforce/provider/controller.createRequest`

[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:33](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L33)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `baseUrl` | string | string | Go: `string` |
| `apiKey` | string | string | Go: `string` |
| `models` | array<string> | array<string> | Go: `[]string` |

<a id="t-3f18a7682be27178"></a>

## `internal/app/workforce/provider/controller.updateRequest`

[seven-agent-force-server/internal/app/workforce/provider/controller/http.go:86](../../../seven-agent-force-server/internal/app/workforce/provider/controller/http.go#L86)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expectedVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `baseUrl` | string | string | Go: `string` |
| `apiKey` | string | string | Go: `string` |
| `models` | array<string> | array<string> | Go: `[]string` |

<a id="t-f5c86ea3d871b06b"></a>

## `internal/app/workforce/provider/domain.DeleteResult`

[seven-agent-force-server/internal/app/workforce/provider/domain/model.go:110](../../../seven-agent-force-server/internal/app/workforce/provider/domain/model.go#L110)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `deleted` | boolean | boolean | Go: `bool` |
| `retainedHistory` | boolean | boolean | Go: `bool` |

<a id="t-e747154f7cf1874e"></a>

## `internal/app/workforce/provider/domain.ModelOption`

[seven-agent-force-server/internal/app/workforce/provider/domain/model.go:102](../../../seven-agent-force-server/internal/app/workforce/provider/domain/model.go#L102)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialName` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `model` | string | string | Go: `string` |
| `label` | string | string | Go: `string` |

<a id="t-b5458b91c4dd21d4"></a>

## `internal/app/workforce/provider/domain.RuntimeSelection`

[seven-agent-force-server/internal/app/workforce/provider/domain/model.go:117](../../../seven-agent-force-server/internal/app/workforce/provider/domain/model.go#L117)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `modelCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `modelProtocol` | string | string | Go: `string` |
| `primaryModel` | string | string | Go: `string` |

<a id="t-fb6ac0d0d6ffab6e"></a>

## `internal/app/workforce/provider/domain.View`

[seven-agent-force-server/internal/app/workforce/provider/domain/model.go:44](../../../seven-agent-force-server/internal/app/workforce/provider/domain/model.go#L44)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `baseUrl` | string | string | Go: `string` |
| `models` | array<string> | array<string> | Go: `[]string` |
| `credentialConfigured` | boolean | boolean | Go: `bool` |
| `secretHint` | string | string | 零值可省略；Go: `string` |
| `status` | string | string | Go: `string` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `lastDiscoveryStatus` | string | string | 零值可省略；Go: `string` |
| `lastDiscoveryReasonCode` | string | string | 零值可省略；Go: `string` |
| `lastDiscoveredAt` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | 零值可省略；Go: `string` |
| `updatedAt` | string | string | 零值可省略；Go: `string` |
| `invocationCount` | integer | string(decimal) | Go: `int64` |
| `inputTokens` | integer | string(decimal) | Go: `int64` |
| `cachedInputTokens` | integer | string(decimal) | Go: `int64` |
| `outputTokens` | integer | string(decimal) | Go: `int64` |
| `totalTokens` | integer | string(decimal) | Go: `int64` |

<a id="t-b9f4af0479712f32"></a>

## `internal/app/workforce/providerinvocation/domain.Analytics`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:201](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L201)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `days` | integer | integer | Go: `int` |
| `summary` | integer | integer | [`internal/app/workforce/providerinvocation/domain.AnalyticsSummary`](wire-types.md#t-e538227bab5f5d1c)；Go: `internal/app/workforce/providerinvocation/domain.AnalyticsSummary` |
| `timeline` | array<integer> | array<integer> | [`internal/app/workforce/providerinvocation/domain.AnalyticsPoint`](wire-types.md#t-e3ed821feae8cf87)；Go: `[]internal/app/workforce/providerinvocation/domain.AnalyticsPoint` |
| `modelDistribution` | array<integer> | array<integer> | [`internal/app/workforce/providerinvocation/domain.ModelDistribution`](wire-types.md#t-a0257b2b1bb25efb)；Go: `[]internal/app/workforce/providerinvocation/domain.ModelDistribution` |
| `cacheHitRate` | number 或 null | number 或 null | Go: `*float64` |
| `p95DurationMs` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `comparison` | integer | integer | [`internal/app/workforce/providerinvocation/domain.AnalyticsComparison`](wire-types.md#t-2b953d4c44c49002)；Go: `internal/app/workforce/providerinvocation/domain.AnalyticsComparison` |

<a id="t-2b953d4c44c49002"></a>

## `internal/app/workforce/providerinvocation/domain.AnalyticsComparison`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:196](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L196)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `invocationPercent` | number 或 null | number 或 null | Go: `*float64` |
| `tokenPercent` | number 或 null | number 或 null | Go: `*float64` |

<a id="t-e3ed821feae8cf87"></a>

## `internal/app/workforce/providerinvocation/domain.AnalyticsPoint`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:185](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L185)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `date` | string | string | Go: `string` |
| `invocationCount` | integer | string(decimal) | Go: `int64` |
| `totalTokens` | integer | string(decimal) | Go: `int64` |

<a id="t-e538227bab5f5d1c"></a>

## `internal/app/workforce/providerinvocation/domain.AnalyticsSummary`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:178](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L178)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `activeCredentialCount` | integer | string(decimal) | Go: `int64` |
| `modelCount` | integer | string(decimal) | Go: `int64` |
| `invocationCount` | integer | string(decimal) | Go: `int64` |
| `totalTokens` | integer | string(decimal) | Go: `int64` |

<a id="t-e32db8160852adc6"></a>

## `internal/app/workforce/providerinvocation/domain.ExecutionKind`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:22](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L22)

底层类型：`string`。
声明的常量值：`"AGENT_ATTEMPT"`, `"REVIEW_RUN"`。
<a id="t-a0257b2b1bb25efb"></a>

## `internal/app/workforce/providerinvocation/domain.ModelDistribution`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:191](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L191)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `model` | string | string | Go: `string` |
| `invocationCount` | integer | string(decimal) | Go: `int64` |

<a id="t-7437168545b1a638"></a>

## `internal/app/workforce/providerinvocation/domain.TaskView`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:159](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L159)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskName` | string | string | Go: `string` |
| `attemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `requestedModel` | string | string | Go: `string` |
| `responseModel` | string | string | 零值可省略；Go: `string` |
| `isPrimaryModel` | boolean | boolean | Go: `bool` |
| `operation` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `statusCode` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `durationMs` | integer | string(decimal) | Go: `int64` |
| `firstByteMs` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |
| `usage` | integer | integer | [`internal/app/workforce/providerinvocation/domain.Usage`](wire-types.md#t-b4f34060597381a3)；Go: `internal/app/workforce/providerinvocation/domain.Usage` |
| `errorCode` | string | string | 零值可省略；Go: `string` |
| `startedAt` | string | string | Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-b4f34060597381a3"></a>

## `internal/app/workforce/providerinvocation/domain.Usage`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:106](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L106)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `inputTokens` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |
| `cachedInputTokens` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |
| `cacheCreationInputTokens` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |
| `outputTokens` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |
| `reasoningOutputTokens` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |
| `totalTokens` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |

<a id="t-e248bb8b954296bd"></a>

## `internal/app/workforce/providerinvocation/domain.View`

[seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go:128](../../../seven-agent-force-server/internal/app/workforce/providerinvocation/domain/model.go#L128)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionKind` | integer | integer | [`internal/app/workforce/providerinvocation/domain.ExecutionKind`](wire-types.md#t-e32db8160852adc6)；Go: `internal/app/workforce/providerinvocation/domain.ExecutionKind` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskName` | string | string | Go: `string` |
| `attemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewRunId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewRunAttemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerInstanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `modelCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `protocol` | string | string | Go: `string` |
| `primaryModel` | string | string | Go: `string` |
| `requestedModel` | string | string | Go: `string` |
| `responseModel` | string | string | 零值可省略；Go: `string` |
| `isPrimaryModel` | boolean | boolean | Go: `bool` |
| `operation` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `statusCode` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `durationMs` | integer | string(decimal) | Go: `int64` |
| `firstByteMs` | integer 或 null | string(decimal) 或 null | 零值可省略；Go: `*int64` |
| `upstreamRequestId` | string | string | 零值可省略；Go: `string` |
| `usage` | integer | integer | [`internal/app/workforce/providerinvocation/domain.Usage`](wire-types.md#t-b4f34060597381a3)；Go: `internal/app/workforce/providerinvocation/domain.Usage` |
| `errorCode` | string | string | 零值可省略；Go: `string` |
| `startedAt` | string | string | Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-c1f80d48eff67ece"></a>

## `internal/app/workforce/resultprocessor/application.CreateDraftCommand`

[seven-agent-force-server/internal/app/workforce/resultprocessor/application/service.go:100](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/application/service.go#L100)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `category` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)；Go: `internal/app/workforce/resultprocessor/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/scriptasset/domain.SourceFile`](wire-types.md#t-17354ae5b0a2291f)；Go: `[]internal/app/workforce/resultprocessor/domain.SourceFile` |

<a id="t-f979b627604d1fa0"></a>

## `internal/app/workforce/resultprocessor/application.FixtureCommand`

[seven-agent-force-server/internal/app/workforce/resultprocessor/application/service.go:166](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/application/service.go#L166)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `harnessReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `evaluationContext` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `workspaceFiles` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `configOverrides` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `expectedDecision` | string | string | 零值可省略；Go: `string` |

<a id="t-a03369a4beb894de"></a>

## `internal/app/workforce/resultprocessor/domain.Action`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:124](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L124)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `type` | string | string | Go: `string` |
| `when` | string | string | 零值可省略；Go: `string` |
| `targetId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `baseBranch` | string | string | 零值可省略；Go: `string` |
| `branch` | string | string | 零值可省略；Go: `string` |
| `operation` | string | string | 零值可省略；Go: `string` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |

<a id="t-521c12e7dfbd299f"></a>

## `internal/app/workforce/resultprocessor/domain.Check`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:95](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L95)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `message` | string | string | 零值可省略；Go: `string` |

<a id="t-1f818af5eaa17765"></a>

## `internal/app/workforce/resultprocessor/domain.ContinuationSelection`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:139](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L139)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `includes` | array<string> | array<string> | 零值可省略；Go: `[]string` |

<a id="t-5fb3543446d84b98"></a>

## `internal/app/workforce/resultprocessor/domain.EnvironmentField`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:60](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L60)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `type` | string | string | Go: `string` |
| `phase` | string | string | Go: `string` |
| `required` | boolean | boolean | 零值可省略；Go: `bool` |
| `taskOverrideAllowed` | boolean | boolean | 零值可省略；Go: `bool` |
| `defaultValue` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `secretRef` | string | string | 零值可省略；Go: `string` |

<a id="t-e6768d74cc8f1cb8"></a>

## `internal/app/workforce/resultprocessor/domain.Finding`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:102](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L102)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `code` | string | string | Go: `string` |
| `message` | string | string | Go: `string` |
| `path` | string | string | 零值可省略；Go: `string` |
| `severity` | string | string | 零值可省略；Go: `string` |

<a id="t-b70580e25014303e"></a>

## `internal/app/workforce/resultprocessor/domain.Manifest`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:41](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L41)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `version` | string | string | 零值可省略；Go: `string` |
| `category` | string | string | 零值可省略；Go: `string` |
| `runtime` | integer | integer | [`internal/app/workforce/resultprocessor/domain.PythonRuntime`](wire-types.md#t-c81d1d3ee7159925)；Go: `internal/app/workforce/resultprocessor/domain.PythonRuntime` |
| `parametersSchema` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `environment` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/domain.EnvironmentField`](wire-types.md#t-5fb3543446d84b98)；Go: `[]internal/app/workforce/resultprocessor/domain.EnvironmentField` |
| `maxEvaluationCycles` | integer | integer | 零值可省略；Go: `int` |
| `continuationPolicy` | string | string | 零值可省略；Go: `string` |
| `deliveryPolicy` | string | string | 零值可省略；Go: `string` |
| `capabilities` | array<string> | array<string> | 零值可省略；Go: `[]string` |

<a id="t-56b3a5c78a2ba7e2"></a>

## `internal/app/workforce/resultprocessor/domain.Output`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:115](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L115)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `kind` | string | string | Go: `string` |
| `path` | string | string | 零值可省略；Go: `string` |
| `selection` | integer | integer | 零值可省略；[`internal/app/workforce/resultprocessor/domain.PathSelection`](wire-types.md#t-94312405ecaa78a1)；Go: `internal/app/workforce/resultprocessor/domain.PathSelection` |
| `archiveFormat` | string | string | 零值可省略；Go: `string` |
| `storage` | string | string | Go: `string` |

<a id="t-94312405ecaa78a1"></a>

## `internal/app/workforce/resultprocessor/domain.PathSelection`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:134](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L134)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `includes` | array<string> | array<string> | Go: `[]string` |
| `excludes` | array<string> | array<string> | 零值可省略；Go: `[]string` |

<a id="t-c81d1d3ee7159925"></a>

## `internal/app/workforce/resultprocessor/domain.PythonRuntime`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:55](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L55)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `kind` | string | string | Go: `string` |
| `requirement` | string | string | 零值可省略；Go: `string` |

<a id="t-fae29394436754e8"></a>

## `internal/app/workforce/resultprocessor/domain.Question`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:109](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L109)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string | string | Go: `string` |
| `question` | string | string | Go: `string` |
| `required` | boolean | boolean | 零值可省略；Go: `bool` |

<a id="t-26bc168af80dc83c"></a>

## `internal/app/workforce/resultprocessor/domain.ResultContract`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:83](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L83)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `decision` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `message` | string | string | Go: `string` |
| `checks` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/domain.Check`](wire-types.md#t-521c12e7dfbd299f)；Go: `[]internal/app/workforce/resultprocessor/domain.Check` |
| `findings` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/domain.Finding`](wire-types.md#t-e6768d74cc8f1cb8)；Go: `[]internal/app/workforce/resultprocessor/domain.Finding` |
| `questions` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/domain.Question`](wire-types.md#t-fae29394436754e8)；Go: `[]internal/app/workforce/resultprocessor/domain.Question` |
| `outputs` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/domain.Output`](wire-types.md#t-56b3a5c78a2ba7e2)；Go: `[]internal/app/workforce/resultprocessor/domain.Output` |
| `actions` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/domain.Action`](wire-types.md#t-a03369a4beb894de)；Go: `[]internal/app/workforce/resultprocessor/domain.Action` |
| `continuation` | integer | integer | 零值可省略；[`internal/app/workforce/resultprocessor/domain.ContinuationSelection`](wire-types.md#t-1f818af5eaa17765)；Go: `internal/app/workforce/resultprocessor/domain.ContinuationSelection` |

<a id="t-5986f44670d787cd"></a>

## `internal/app/workforce/resultprocessor/domain.Validation`

[seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go:72](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/domain/protocol.go#L72)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `valid` | boolean | boolean | Go: `bool` |
| `reasonCode` | string | string | Go: `string` |
| `errors` | array<string> | array<string> | Go: `[]string` |
| `manifestSha256` | string | string | 零值可省略；Go: `string` |
| `sourceSha256` | string | string | 零值可省略；Go: `string` |

<a id="t-8b21b5d551e70b86"></a>

## `internal/app/workforce/resultprocessor/facade.ContinuationCheckpointCandidate`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:110](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L110)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `formatVersion` | integer | integer | Go: `int` |
| `checkpointIdentity` | string | string | Go: `string` |
| `evaluationCycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `decision` | string | string | Go: `string` |
| `workspaceObjectKey` | string | string | Go: `string` |
| `workspaceSha256` | string | string | Go: `string` |
| `workspaceContentSha256` | string | string | Go: `string` |
| `workspaceObjectSha256` | string | string | Go: `string` |
| `workspaceSizeBytes` | integer | string(decimal) | Go: `int64` |
| `repositoryBundleObjectKey` | string | string | 零值可省略；Go: `string` |
| `repositoryBundleContentSha256` | string | string | 零值可省略；Go: `string` |
| `repositoryBundleObjectSha256` | string | string | 零值可省略；Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `workspaceOverlayObjectKey` | string | string | 零值可省略；Go: `string` |
| `workspaceOverlayContentSha256` | string | string | 零值可省略；Go: `string` |
| `workspaceOverlayObjectSha256` | string | string | 零值可省略；Go: `string` |
| `workspaceOverlaySizeBytes` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `selectionDigest` | string | string | 零值可省略；Go: `string` |
| `gitignoreDigest` | string | string | 零值可省略；Go: `string` |
| `selectionPolicyDigest` | string | string | 零值可省略；Go: `string` |
| `sessionObjectKey` | string | string | Go: `string` |
| `sessionSha256` | string | string | Go: `string` |
| `sessionContentSha256` | string | string | Go: `string` |
| `sessionObjectSha256` | string | string | Go: `string` |
| `sessionSizeBytes` | integer | string(decimal) | Go: `int64` |
| `manifestObjectKey` | string | string | Go: `string` |
| `manifestSha256` | string | string | Go: `string` |
| `manifestContentSha256` | string | string | Go: `string` |
| `manifestObjectSha256` | string | string | Go: `string` |
| `manifestSizeBytes` | integer | string(decimal) | Go: `int64` |
| `taskExecutionSnapshotDigest` | string | string | Go: `string` |
| `baseCommit` | string | string | 零值可省略；Go: `string` |
| `resultCommit` | string | string | 零值可省略；Go: `string` |
| `resultTree` | string | string | 零值可省略；Go: `string` |
| `sessionLineageDigest` | string | string | Go: `string` |
| `encryptionAlgorithm` | string | string | Go: `string` |
| `encryptionKeyId` | string | string | Go: `string` |
| `wrappedDataKey` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |

<a id="t-12df377b44feb960"></a>

## `internal/app/workforce/resultprocessor/facade.ContinuationCheckpointView`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:154](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L154)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `evaluationCycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `decision` | string | string | Go: `string` |
| `manifestSha256` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |
| `createdAt` | string | string | Go: `string` |
| `availableAt` | string | string | 零值可省略；Go: `string` |

<a id="t-1cb3eba0f7bf13fb"></a>

## `internal/app/workforce/resultprocessor/facade.EvaluationCandidate`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:42](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L42)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `cycleNumber` | integer | integer | Go: `int` |
| `sessionDigest` | string | string | 零值可省略；Go: `string` |
| `candidateTreeDigest` | string | string | 零值可省略；Go: `string` |
| `findingsDigest` | string | string | 零值可省略；Go: `string` |
| `contract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `contractDigest` | string | string | Go: `string` |
| `processorReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `processorDigest` | string | string | Go: `string` |
| `evaluationStartedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-005d2358dcdb1f9c"></a>

## `internal/app/workforce/resultprocessor/facade.EvaluationDirective`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:69](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L69)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `contract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `contractDigest` | string | string | Go: `string` |
| `directive` | string | string | Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `decision` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `message` | string | string | 零值可省略；Go: `string` |
| `managedFeedback` | string | string | 零值可省略；Go: `string` |
| `resumeNonce` | string | string | 零值可省略；Go: `string` |
| `remainingCycles` | integer | integer | 零值可省略；Go: `int` |
| `checkpointId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-80207b14faf14de1"></a>

## `internal/app/workforce/resultprocessor/facade.EvaluationReplay`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/evaluation_replay.go:12](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/evaluation_replay.go#L12)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `found` | boolean | boolean | Go: `bool` |
| `directive` | integer | integer | [`internal/app/workforce/resultprocessor/facade.EvaluationDirective`](wire-types.md#t-005d2358dcdb1f9c)；Go: `internal/app/workforce/resultprocessor/facade.EvaluationDirective` |

<a id="t-47ef88aef1917589"></a>

## `internal/app/workforce/resultprocessor/facade.EvaluationReplayIdentity`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/evaluation_replay.go:5](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/evaluation_replay.go#L5)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `cycleNumber` | integer | integer | Go: `int` |
| `sessionDigest` | string | string | Go: `string` |

<a id="t-f50dc8464c810ee6"></a>

## `internal/app/workforce/resultprocessor/facade.EvaluationView`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:85](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L85)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `cycleNumber` | integer | integer | Go: `int` |
| `sessionDigest` | string | string | 零值可省略；Go: `string` |
| `candidateTreeDigest` | string | string | 零值可省略；Go: `string` |
| `findingsDigest` | string | string | 零值可省略；Go: `string` |
| `contractDigest` | string | string | Go: `string` |
| `decision` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `message` | string | string | 零值可省略；Go: `string` |
| `contract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `status` | string | string | Go: `string` |
| `startedAt` | string | string | Go: `string` |
| `persistedAt` | string | string | 零值可省略；Go: `string` |
| `completedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-c77b5bf41ecd7639"></a>

## `internal/app/workforce/resultprocessor/facade.Execution`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/types.go:10](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/types.go#L10)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `releaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `digest` | string | string | Go: `string` |
| `archiveObjectKey` | string | string | Go: `string` |
| `manifest` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `environmentConfigDigest` | string | string | Go: `string` |
| `environment` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `secretVersionIdentities` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `parameters` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `maxEvaluationCycles` | integer | integer | Go: `int` |
| `continuationPolicy` | string | string | Go: `string` |
| `deliveryPolicy` | string | string | Go: `string` |

<a id="t-b487b9d57000cbe9"></a>

## `internal/app/workforce/resultprocessor/facade.FixtureExecution`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:188](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L188)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `harnessImageDigest` | string | string | Go: `string` |
| `revisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionDigest` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)；Go: `internal/app/workforce/resultprocessor/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/scriptasset/domain.SourceFile`](wire-types.md#t-17354ae5b0a2291f)；Go: `[]internal/app/workforce/resultprocessor/domain.SourceFile` |
| `evaluationContext` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `workspaceFiles` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `configOverrides` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `expectedDecision` | string | string | 零值可省略；Go: `string` |

<a id="t-6ecead615b31bf3d"></a>

## `internal/app/workforce/resultprocessor/facade.FixtureResult`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:205](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L205)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `contract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `contractDigest` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `diagnostics` | string | string | 零值可省略；Go: `string` |

<a id="t-b9ccd3d72900bdf6"></a>

## `internal/app/workforce/resultprocessor/facade.MachineIdentity`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:20](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L20)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-af6eb918fd2fdb5d"></a>

## `internal/app/workforce/resultprocessor/facade.Materialization`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:31](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L31)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `variables` | object<string, string> | object<string, string> | Go: `map[string]string` |
| `expiresAt` | string | string | Go: `string` |

<a id="t-66337263924c2a96"></a>

## `internal/app/workforce/resultprocessor/facade.ResultOutputView`

[seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go:169](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/facade/runtime.go#L169)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `evaluationCycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `outputKind` | string | string | Go: `string` |
| `archiveFormat` | string | string | 零值可省略；Go: `string` |
| `storage` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `artifactId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `sha256` | string | string | 零值可省略；Go: `string` |
| `sizeBytes` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `createdAt` | string | string | Go: `string` |

<a id="t-897a7ccb60a1e5f0"></a>

## `internal/app/workforce/resultprocessor/infrastructure.EnvironmentBinding`

[seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go:99](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go#L99)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `processorReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `type` | string | string | Go: `string` |
| `phase` | string | string | Go: `string` |
| `config` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `secretRef` | string | string | 零值可省略；Go: `string` |
| `secretVersion` | string | string | 零值可省略；Go: `string` |
| `taskOverrideAllowed` | boolean | boolean | Go: `bool` |
| `networkPolicy` | string | string | Go: `string` |

<a id="t-a2fa5e5df2f0a955"></a>

## `internal/app/workforce/resultprocessor/infrastructure.FixtureRun`

[seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go:113](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go#L113)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `harnessReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `harnessImageDigest` | string | string | Go: `string` |
| `evaluationContext` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `workspaceFiles` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `configOverrides` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `expectedDecision` | string | string | 零值可省略；Go: `string` |
| `status` | string | string | Go: `string` |
| `workerId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `workerInstanceId` | string | string | 零值可省略；Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultContract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `resultContractSha256` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `diagnostics` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-43d18e8c30c06b56"></a>

## `internal/app/workforce/resultprocessor/infrastructure.Processor`

[seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go:84](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go#L84)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `scopeType` | string | string | Go: `string` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `category` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `defaultBound` | boolean | boolean | Go: `bool` |
| `latestRelease` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/resultprocessor/infrastructure.Release`](wire-types.md#t-aefd670a358cb98d)；Go: `*internal/app/workforce/resultprocessor/infrastructure.Release` |
| `currentDraft` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/resultprocessor/infrastructure.Revision`](wire-types.md#t-14a66f1cbd666fcc)；Go: `*internal/app/workforce/resultprocessor/infrastructure.Revision` |
| `releases` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/infrastructure.Release`](wire-types.md#t-aefd670a358cb98d)；Go: `[]internal/app/workforce/resultprocessor/infrastructure.Release` |
| `createdAt` | string | string | Go: `string` |
| `updatedAt` | string | string | Go: `string` |

<a id="t-aefd670a358cb98d"></a>

## `internal/app/workforce/resultprocessor/infrastructure.Release`

[seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go:68](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go#L68)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `processorId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `scopeType` | string | string | Go: `string` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `version` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)；Go: `internal/app/workforce/resultprocessor/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/scriptasset/domain.SourceFile`](wire-types.md#t-17354ae5b0a2291f)；Go: `[]internal/app/workforce/resultprocessor/domain.SourceFile` |
| `archiveObjectKey` | string | string | Go: `string` |
| `archiveSha256` | string | string | Go: `string` |
| `archiveSizeBytes` | integer | string(decimal) | Go: `int64` |
| `publishedAt` | string | string | Go: `string` |
| `environmentBindings` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/resultprocessor/infrastructure.EnvironmentBinding`](wire-types.md#t-897a7ccb60a1e5f0)；Go: `[]internal/app/workforce/resultprocessor/infrastructure.EnvironmentBinding` |

<a id="t-14a66f1cbd666fcc"></a>

## `internal/app/workforce/resultprocessor/infrastructure.Revision`

[seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go:53](../../../seven-agent-force-server/internal/app/workforce/resultprocessor/infrastructure/store.go#L53)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `processorId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `scopeType` | string | string | Go: `string` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionNumber` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `draftVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `manifest` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Manifest`](wire-types.md#t-b70580e25014303e)；Go: `internal/app/workforce/resultprocessor/domain.Manifest` |
| `files` | array<integer> | array<integer> | [`internal/app/workforce/scriptasset/domain.SourceFile`](wire-types.md#t-17354ae5b0a2291f)；Go: `[]internal/app/workforce/resultprocessor/domain.SourceFile` |
| `validation` | integer | integer | [`internal/app/workforce/resultprocessor/domain.Validation`](wire-types.md#t-5986f44670d787cd)；Go: `internal/app/workforce/resultprocessor/domain.Validation` |
| `createdAt` | string | string | Go: `string` |
| `updatedAt` | string | string | Go: `string` |

<a id="t-629e091bbdeb27cd"></a>

## `internal/app/workforce/reviewrun/controller.decisionRequest`

[seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go:99](../../../seven-agent-force-server/internal/app/workforce/reviewrun/controller/http.go#L99)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `action` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |
| `revisionGuidance` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-9c051df92d406a86"></a>

## `internal/app/workforce/reviewrun/domain.AssignmentMessage`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:192](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L192)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `kind` | string | string | Go: `string` |
| `messageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reviewRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `expiresAt` | string | string | Go: `string` |

<a id="t-298d71178d4a89bd"></a>

## `internal/app/workforce/reviewrun/domain.CandidateReport`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go:29](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go#L29)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | string | string | Go: `string` |
| `verdict` | string | string | Go: `string` |
| `score` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `summary` | string | string | Go: `string` |
| `findings` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/reviewrun/domain.Finding`](wire-types.md#t-57e1f7e0d6ae7b0b)；Go: `[]internal/app/workforce/reviewrun/domain.Finding` |
| `reviewTests` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/reviewrun/domain.ReviewTest`](wire-types.md#t-405bb61949b8e8b2)；Go: `[]internal/app/workforce/reviewrun/domain.ReviewTest` |
| `limitations` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/reviewrun/domain.Limitation`](wire-types.md#t-c69f2bf7d3a0511c)；Go: `[]internal/app/workforce/reviewrun/domain.Limitation` |

<a id="t-b564f5c782b4694d"></a>

## `internal/app/workforce/reviewrun/domain.Claim`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:204](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L204)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `completionCheckDigest` | string | string | 零值可省略；Go: `string` |
| `reviewRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reviewRunAttemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `finalizationRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `reviewerImageDigest` | string | string | Go: `string` |
| `modelCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `modelProtocol` | string | string | Go: `string` |
| `primaryModel` | string | string | Go: `string` |
| `providerStructuredOutputContract` | string | string | Go: `string` |
| `providerProxyImageDigest` | string | string | 零值可省略；Go: `string` |
| `systemContract` | string | string | Go: `string` |
| `reportSchemaVersion` | string | string | Go: `string` |
| `hardTimeoutSeconds` | integer | integer | Go: `int` |
| `reviewContract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `requirements` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `reviewLeaseExpiresAt` | string | string | Go: `string` |
| `executionDeadlineAt` | string | string | Go: `string` |

<a id="t-cbc896f3816ba825"></a>

## `internal/app/workforce/reviewrun/domain.CompletionCheckSummary`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:146](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L146)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `instructions` | string | string | 零值可省略；Go: `string` |
| `reviewPrompt` | string | string | 零值可省略；Go: `string` |

<a id="t-fe844e75ea3ad2e1"></a>

## `internal/app/workforce/reviewrun/domain.Evidence`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go:49](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go#L49)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `path` | string | string | Go: `string` |
| `line` | integer | integer | Go: `int` |

<a id="t-57e1f7e0d6ae7b0b"></a>

## `internal/app/workforce/reviewrun/domain.Finding`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go:39](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go#L39)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string | string | Go: `string` |
| `severity` | string | string | Go: `string` |
| `category` | string | string | Go: `string` |
| `title` | string | string | Go: `string` |
| `message` | string | string | Go: `string` |
| `evidence` | integer | integer | [`internal/app/workforce/reviewrun/domain.Evidence`](wire-types.md#t-fe844e75ea3ad2e1)；Go: `internal/app/workforce/reviewrun/domain.Evidence` |
| `suggestedDirection` | string | string | 零值可省略；Go: `string` |

<a id="t-1215a71f93c8a0dc"></a>

## `internal/app/workforce/reviewrun/domain.HumanDecisionSummary`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:111](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L111)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `action` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |
| `revisionGuidance` | string | string | 零值可省略；Go: `string` |
| `aiVerdict` | string | string | 零值可省略；Go: `string` |
| `reportDigest` | string | string | 零值可省略；Go: `string` |
| `qualityOutcome` | string | string | 零值可省略；Go: `string` |
| `resultingTaskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `decidedAt` | string | string | Go: `string` |

<a id="t-93cfbc887d87f40f"></a>

## `internal/app/workforce/reviewrun/domain.LaunchPermit`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:232](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L232)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `permitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `permitToken` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-c69f2bf7d3a0511c"></a>

## `internal/app/workforce/reviewrun/domain.Limitation`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go:61](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go#L61)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `code` | string | string | Go: `string` |
| `summary` | string | string | Go: `string` |

<a id="t-405bb61949b8e8b2"></a>

## `internal/app/workforce/reviewrun/domain.ReviewTest`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go:54](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go#L54)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `command` | string | string | Go: `string` |
| `exitCode` | integer | integer | Go: `int` |
| `status` | string | string | Go: `string` |
| `summary` | string | string | Go: `string` |

<a id="t-26a153c3d3c98dac"></a>

## `internal/app/workforce/reviewrun/domain.RunPage`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:151](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L151)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `items` | array<integer> | array<integer> | [`internal/app/workforce/reviewrun/domain.RunView`](wire-types.md#t-2177d7b784d4451b)；Go: `[]internal/app/workforce/reviewrun/domain.RunView` |
| `nextCursor` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-2177d7b784d4451b"></a>

## `internal/app/workforce/reviewrun/domain.RunView`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:123](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L123)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `completionCheckDigest` | string | string | 零值可省略；Go: `string` |
| `completionCheckSummary` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/reviewrun/domain.CompletionCheckSummary`](wire-types.md#t-cbc896f3816ba825)；Go: `*internal/app/workforce/reviewrun/domain.CompletionCheckSummary` |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `verdict` | string | string | 零值可省略；Go: `string` |
| `qualityOutcome` | string | string | 零值可省略；Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `report` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/reviewrun/domain.TaskReport`](wire-types.md#t-6714fb7fac0f58cb)；Go: `*internal/app/workforce/reviewrun/domain.TaskReport` |
| `automaticRevisionNumber` | integer | integer | Go: `int` |
| `noProgressStreak` | integer | integer | Go: `int` |
| `stopReason` | string | string | 零值可省略；Go: `string` |
| `revisionKind` | string | string | 零值可省略；Go: `string` |
| `successorTaskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `humanDecision` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/reviewrun/domain.HumanDecisionSummary`](wire-types.md#t-1215a71f93c8a0dc)；Go: `*internal/app/workforce/reviewrun/domain.HumanDecisionSummary` |
| `createdAt` | string | string | Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-a492b9360e866af6"></a>

## `internal/app/workforce/reviewrun/domain.Summary`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:83](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L83)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `required` | boolean | boolean | Go: `bool` |
| `status` | string | string | 零值可省略；Go: `string` |

<a id="t-6714fb7fac0f58cb"></a>

## `internal/app/workforce/reviewrun/domain.TaskReport`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go:92](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/model.go#L92)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `required` | boolean | boolean | Go: `bool` |
| `status` | string | string | 零值可省略；Go: `string` |
| `verdict` | string | string | 零值可省略；Go: `string` |
| `score` | integer 或 null | integer 或 null | 零值可省略；Go: `*int` |
| `summary` | string | string | 零值可省略；Go: `string` |
| `findings` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/reviewrun/domain.Finding`](wire-types.md#t-57e1f7e0d6ae7b0b)；Go: `[]internal/app/workforce/reviewrun/domain.Finding` |
| `reviewTests` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/reviewrun/domain.ReviewTest`](wire-types.md#t-405bb61949b8e8b2)；Go: `[]internal/app/workforce/reviewrun/domain.ReviewTest` |
| `limitations` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/reviewrun/domain.Limitation`](wire-types.md#t-c69f2bf7d3a0511c)；Go: `[]internal/app/workforce/reviewrun/domain.Limitation` |
| `reportDigest` | string | string | 零值可省略；Go: `string` |
| `qualityOutcome` | string | string | 零值可省略；Go: `string` |
| `automaticRevisionCount` | integer | integer | Go: `int` |
| `automaticRevisionLimit` | integer | integer | Go: `int` |
| `noProgressStreak` | integer | integer | Go: `int` |
| `stopReason` | string | string | 零值可省略；Go: `string` |
| `availableActions` | array<string> | array<string> | Go: `[]string` |
| `revisionGuidanceRequired` | boolean | boolean | Go: `bool` |

<a id="t-8db36876e421763a"></a>

## `internal/app/workforce/reviewrun/domain.UsageFacts`

[seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go:68](../../../seven-agent-force-server/internal/app/workforce/reviewrun/domain/report.go#L68)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `requests` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `inputTokens` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `outputTokens` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `durationMillis` | integer | string(decimal) | 零值可省略；Go: `int64` |

<a id="t-7d46c6abb89ba2d3"></a>

## `internal/app/workforce/reviewrun/facade.HumanDecisionResult`

[seven-agent-force-server/internal/app/workforce/reviewrun/facade/contracts.go:157](../../../seven-agent-force-server/internal/app/workforce/reviewrun/facade/contracts.go#L157)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reviewRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `action` | string | string | Go: `string` |
| `aiVerdict` | string | string | 零值可省略；Go: `string` |
| `reportDigest` | string | string | 零值可省略；Go: `string` |
| `qualityOutcome` | string | string | 零值可省略；Go: `string` |
| `resultingTaskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `decidedAt` | string | string | Go: `string` |
| `revisionGuidance` | string | string | 零值可省略；Go: `string` |

<a id="t-71c24cb95da0a225"></a>

## `internal/app/workforce/reviewrun/facade.ReportResult`

[seven-agent-force-server/internal/app/workforce/reviewrun/facade/contracts.go:125](../../../seven-agent-force-server/internal/app/workforce/reviewrun/facade/contracts.go#L125)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `Status` | string | string | Go: `string` |
| `Verdict` | string | string | Go: `string` |
| `ReportDigest` | string | string | Go: `string` |

<a id="t-17354ae5b0a2291f"></a>

## `internal/app/workforce/scriptasset/domain.SourceFile`

[seven-agent-force-server/internal/app/workforce/scriptasset/domain/asset.go:35](../../../seven-agent-force-server/internal/app/workforce/scriptasset/domain/asset.go#L35)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `path` | string | string | Go: `string` |
| `content` | string | string | Go: `string` |
| `mode` | integer | string(decimal) | 零值可省略；Go: `int64` |

<a id="t-cb967617226e781d"></a>

## `internal/app/workforce/scriptasset/domain.Validation`

[seven-agent-force-server/internal/app/workforce/scriptasset/domain/asset.go:43](../../../seven-agent-force-server/internal/app/workforce/scriptasset/domain/asset.go#L43)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `valid` | boolean | boolean | Go: `bool` |
| `reasonCode` | string | string | Go: `string` |
| `errors` | array<string> | array<string> | Go: `[]string` |
| `sourceSha256` | string | string | 零值可省略；Go: `string` |

<a id="t-857519a45b47ace4"></a>

## `internal/app/workforce/shared/gittransport.AccessBinding`

[seven-agent-force-server/internal/app/workforce/shared/gittransport/model.go:52](../../../seven-agent-force-server/internal/app/workforce/shared/gittransport/model.go#L52)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `capability` | string | string | Go: `string` |
| `credentialRef` | string | string | 零值可省略；Go: `string` |
| `credentialVersion` | string | string | 零值可省略；Go: `string` |
| `status` | string | string | Go: `string` |
| `lastProbeReasonCode` | string | string | 零值可省略；Go: `string` |
| `lastProbedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |

<a id="t-9419f97d1b4bcd0e"></a>

## `internal/app/workforce/shared/gittransport.EgressPolicy`

[seven-agent-force-server/internal/app/workforce/shared/gittransport/model.go:61](../../../seven-agent-force-server/internal/app/workforce/shared/gittransport/model.go#L61)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `allowPrivateNetwork` | boolean | boolean | Go: `bool` |
| `allowedHosts` | array<string> | array<string> | Go: `[]string` |
| `allowedCidrs` | array<string> | array<string> | Go: `[]string` |
| `allowedPorts` | array<integer> | array<integer> | Go: `[]int` |

<a id="t-a3e01d6fa2995af1"></a>

## `internal/app/workforce/shared/gittransport.RepositoryTarget`

[seven-agent-force-server/internal/app/workforce/shared/gittransport/model.go:104](../../../seven-agent-force-server/internal/app/workforce/shared/gittransport/model.go#L104)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `providerType` | string | string | Go: `string` |
| `transport` | string | string | Go: `string` |
| `host` | string | string | 零值可省略；Go: `string` |
| `port` | integer | integer | 零值可省略；Go: `int` |
| `repositoryPath` | string | string | Go: `string` |
| `webRepositoryUrl` | string | string | 零值可省略；Go: `string` |
| `read` | integer | integer | [`internal/app/workforce/shared/gittransport.AccessBinding`](wire-types.md#t-857519a45b47ace4)；Go: `internal/app/workforce/shared/gittransport.AccessBinding` |
| `push` | integer | integer | [`internal/app/workforce/shared/gittransport.AccessBinding`](wire-types.md#t-857519a45b47ace4)；Go: `internal/app/workforce/shared/gittransport.AccessBinding` |
| `review` | integer | integer | [`internal/app/workforce/shared/gittransport.AccessBinding`](wire-types.md#t-857519a45b47ace4)；Go: `internal/app/workforce/shared/gittransport.AccessBinding` |
| `sshKnownHostFingerprint` | string | string | 零值可省略；Go: `string` |
| `tlsPolicy` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `egress` | integer | integer | [`internal/app/workforce/shared/gittransport.EgressPolicy`](wire-types.md#t-9419f97d1b4bcd0e)；Go: `internal/app/workforce/shared/gittransport.EgressPolicy` |
| `egressPolicyDigest` | string | string | Go: `string` |
| `commitAuthorName` | string | string | Go: `string` |
| `commitAuthorEmail` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `configDigest` | string | string | Go: `string` |
| `disabledAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `disabledBy` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `disabledReason` | string | string | 零值可省略；Go: `string` |
| `decommissionedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `decommissionedBy` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `decommissionReason` | string | string | 零值可省略；Go: `string` |
| `lifecycleGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-aa20383d22defc63"></a>

## `internal/app/workforce/shared/transport.Int64Slice`

[seven-agent-force-server/internal/app/workforce/shared/transport/int64.go:13](../../../seven-agent-force-server/internal/app/workforce/shared/transport/int64.go#L13)

底层类型：`[]int64`。
<a id="t-5a2b56cbd1e127e7"></a>

## `internal/app/workforce/task/application.MetadataRequest`

[seven-agent-force-server/internal/app/workforce/task/application/service.go:675](../../../seven-agent-force-server/internal/app/workforce/task/application/service.go#L675)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `tags` | array<string> | array<string> | Go: `[]string` |
| `expectedMetadataVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-7c59d8eb54e5bb24"></a>

## `internal/app/workforce/task/controller.independentReviewSummary`

[seven-agent-force-server/internal/app/workforce/task/controller/http.go:68](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L68)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `required` | boolean | boolean | Go: `bool` |
| `status` | string | string | 零值可省略；Go: `string` |

<a id="t-d866baf604f3d5b9"></a>

## `internal/app/workforce/task/controller.taskResponse`

[seven-agent-force-server/internal/app/workforce/task/controller/http.go:73](../../../seven-agent-force-server/internal/app/workforce/task/controller/http.go#L73)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `selectedRuntimeReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `selectedRuntime` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.SelectedRuntimeView`](wire-types.md#t-09e0b38cc1c3d2a1)；Go: `*internal/app/workforce/task/domain.SelectedRuntimeView` |
| `executionRequirementsDigest` | string | string | 零值可省略；Go: `string` |
| `name` | string | string | Go: `string` |
| `tags` | array<string> | array<string> | Go: `[]string` |
| `createdBy` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `metadataVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `inputKind` | string | string | 零值可省略；Go: `string` |
| `referenceCount` | integer | integer | 零值可省略；Go: `int` |
| `repositoryUrl` | string | string | Go: `string` |
| `repositoryRef` | string | string | Go: `string` |
| `resolvedBaseCommit` | string | string | 零值可省略；Go: `string` |
| `instruction` | string | string | Go: `string` |
| `resultProcessorReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `currentTurnNumber` | integer | integer | 零值可省略；Go: `int` |
| `reviewEnabled` | boolean | boolean | Go: `bool` |
| `modelSelection` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.ModelSelectionView`](wire-types.md#t-3cfef8c6731f17fd)；Go: `*internal/app/workforce/task/domain.ModelSelectionView` |
| `status` | integer | integer | [`internal/app/workforce/task/domain.TaskStatus`](wire-types.md#t-00f23df6481c09b6)；Go: `internal/app/workforce/task/domain.TaskStatus` |
| `priority` | integer | integer | Go: `int` |
| `executionTimeoutSeconds` | integer | integer | Go: `int` |
| `executionActiveMilliseconds` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `lastProgressAt` | string | string | 零值可省略；Go: `string` |
| `resultSummary` | string | string | 零值可省略；Go: `string` |
| `latestAttemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `createdAt` | string | string | Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `execution` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.ExecutionView`](wire-types.md#t-69bfdb28cfe61d52)；Go: `*internal/app/workforce/task/domain.ExecutionView` |
| `cancelRequestId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `cancelGeneration` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `cancelRequestedBy` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `cancelRequestedAt` | string | string | 零值可省略；Go: `string` |
| `cancelReason` | string | string | 零值可省略；Go: `string` |
| `independentReview` | integer | integer | [`internal/app/workforce/task/controller.independentReviewSummary`](wire-types.md#t-7c59d8eb54e5bb24)；Go: `internal/app/workforce/task/controller.independentReviewSummary` |
| `pendingActionCount` | integer | integer | Go: `int` |
| `hasUnreadUpdates` | boolean | boolean | Go: `bool` |
| `activitySummary` | string | string | 零值可省略；Go: `string` |
| `firstPendingActionKind` | string | string | 零值可省略；Go: `string` |
| `firstPendingActionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-954080f0a094f86c"></a>

## `internal/app/workforce/task/domain.Artifact`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:275](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L275)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `type` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |
| `mediaType` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `metadata` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `createdAt` | string | string | Go: `string` |
| `downloadUrl` | string | string | Go: `string` |

<a id="t-9c2cda27ed812dc1"></a>

## `internal/app/workforce/task/domain.Attempt`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:506](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L506)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `leaseExpiresAt` | string | string | 零值可省略；Go: `string` |
| `workspacePath` | string | string | Go: `string` |
| `executionNodeId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `assignmentId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `workerInstanceId` | string | string | 零值可省略；Go: `string` |
| `hostBootId` | string | string | 零值可省略；Go: `string` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `expectedContainerName` | string | string | 零值可省略；Go: `string` |
| `runtimeLabels` | object<string, string> | object<string, string> | 零值可省略；Go: `map[string]string` |

<a id="t-12559205cdc785ad"></a>

## `internal/app/workforce/task/domain.BacklogReasonCount`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:157](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L157)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reasonCode` | string | string | Go: `string` |
| `count` | integer | integer | Go: `int` |

<a id="t-68e1c55e3001858a"></a>

## `internal/app/workforce/task/domain.BacklogSummary`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:162](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L162)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `queuedTasks` | integer | integer | Go: `int` |
| `oldestWaitSeconds` | integer | string(decimal) | Go: `int64` |
| `reasons` | array<integer> | array<integer> | [`internal/app/workforce/task/domain.BacklogReasonCount`](wire-types.md#t-12559205cdc785ad)；Go: `[]internal/app/workforce/task/domain.BacklogReasonCount` |
| `compatibleWorkers` | integer | integer | Go: `int` |
| `availableWorkers` | integer | integer | Go: `int` |

<a id="t-f89855766a823f57"></a>

## `internal/app/workforce/task/domain.ContinuationRestore`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:424](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L424)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `formatVersion` | integer | integer | Go: `int` |
| `restoreRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |
| `sourceTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `targetTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workspaceObjectKey` | string | string | Go: `string` |
| `workspaceContentSha256` | string | string | Go: `string` |
| `workspaceObjectSha256` | string | string | Go: `string` |
| `workspaceSizeBytes` | integer | string(decimal) | Go: `int64` |
| `repositoryBundleObjectKey` | string | string | Go: `string` |
| `repositoryBundleContentSha256` | string | string | Go: `string` |
| `repositoryBundleObjectSha256` | string | string | Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | Go: `int64` |
| `workspaceOverlayObjectKey` | string | string | Go: `string` |
| `workspaceOverlayContentSha256` | string | string | Go: `string` |
| `workspaceOverlayObjectSha256` | string | string | Go: `string` |
| `workspaceOverlaySizeBytes` | integer | string(decimal) | Go: `int64` |
| `selectionDigest` | string | string | Go: `string` |
| `sessionObjectKey` | string | string | Go: `string` |
| `sessionContentSha256` | string | string | Go: `string` |
| `sessionObjectSha256` | string | string | Go: `string` |
| `sessionSizeBytes` | integer | string(decimal) | Go: `int64` |
| `manifestObjectKey` | string | string | Go: `string` |
| `manifestContentSha256` | string | string | Go: `string` |
| `manifestObjectSha256` | string | string | Go: `string` |
| `manifestSizeBytes` | integer | string(decimal) | Go: `int64` |
| `taskExecutionSnapshotDigest` | string | string | Go: `string` |
| `expectedBaseCommit` | string | string | Go: `string` |
| `expectedResultCommit` | string | string | Go: `string` |
| `expectedResultTree` | string | string | Go: `string` |
| `sessionLineageDigest` | string | string | Go: `string` |
| `resumeNonce` | string | string | Go: `string` |
| `resumeNonceDigest` | string | string | Go: `string` |
| `encryptionAlgorithm` | string | string | 零值可省略；Go: `string` |
| `encryptionKeyId` | string | string | 零值可省略；Go: `string` |

<a id="t-2b0251d7dcf14955"></a>

## `internal/app/workforce/task/domain.Event`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:240](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L240)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `type` | string | string | Go: `string` |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `createdAt` | string | string | Go: `string` |

<a id="t-e4e060f9023f9ef1"></a>

## `internal/app/workforce/task/domain.ExecutionSnapshot`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:200](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L200)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `completionCheckInputDigest` | string | string | 零值可省略；Go: `string` |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `completionCheckDigest` | string | string | 零值可省略；Go: `string` |
| `completionCheckReferences` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/completioncheck/facade.Reference`](wire-types.md#t-0fbcc46c1346111a)；Go: `[]internal/app/workforce/task/domain.CompletionCheckReference` |
| `harnessReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `harnessImageDigest` | string | string | Go: `string` |
| `hookConfigurationDigest` | string | string | 零值可省略；Go: `string` |
| `packageReleases` | array<integer> | array<integer> | [`internal/app/workforce/task/domain.HookPackageReleaseSnapshot`](wire-types.md#t-29ed34406ad73388)；Go: `[]internal/app/workforce/task/domain.HookPackageReleaseSnapshot` |
| `dispatcherVersion` | string | string | 零值可省略；Go: `string` |
| `dispatcherSha256` | string | string | 零值可省略；Go: `string` |
| `adapterVersion` | string | string | 零值可省略；Go: `string` |
| `providerProxyImageDigest` | string | string | 零值可省略；Go: `string` |
| `providerProxyVersion` | string | string | 零值可省略；Go: `string` |
| `modelCredentialId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `credentialVersion` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `modelProtocol` | string | string | 零值可省略；Go: `string` |
| `primaryModel` | string | string | 零值可省略；Go: `string` |
| `localProviderFacadeVersion` | string | string | 零值可省略；Go: `string` |
| `localProviderFacadeDigest` | string | string | 零值可省略；Go: `string` |
| `providerGatewayIdentity` | string | string | 零值可省略；Go: `string` |
| `providerGatewayContractDigest` | string | string | 零值可省略；Go: `string` |
| `providerAdapterIdentity` | string | string | 零值可省略；Go: `string` |
| `executionDeadlineSeconds` | integer | integer | Go: `int` |
| `resultProcessorReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `resultProcessorDigest` | string | string | 零值可省略；Go: `string` |
| `pythonRuntimeIdentity` | string | string | 零值可省略；Go: `string` |
| `bootstrapDigest` | string | string | 零值可省略；Go: `string` |
| `sdkDigest` | string | string | 零值可省略；Go: `string` |
| `moduleIndexDigest` | string | string | 零值可省略；Go: `string` |
| `environmentConfigDigest` | string | string | 零值可省略；Go: `string` |
| `resultProcessorEnvironment` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `secretVersionIdentities` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `resultProcessorParameters` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `maxEvaluationCycles` | integer | integer | 零值可省略；Go: `int` |
| `continuationPolicy` | string | string | 零值可省略；Go: `string` |
| `deliveryPolicy` | string | string | 零值可省略；Go: `string` |
| `isolationProfile` | string | string | Go: `string` |

<a id="t-69bfdb28cfe61d52"></a>

## `internal/app/workforce/task/domain.ExecutionView`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:84](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L84)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `nodeKey` | string | string | Go: `string` |
| `nodeStatus` | string | string | Go: `string` |
| `queueWaitReason` | string | string | 零值可省略；Go: `string` |
| `executionRequirementDigest` | string | string | Go: `string` |
| `assignmentId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `assignmentStatus` | string | string | 零值可省略；Go: `string` |
| `workerId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `workerInstanceId` | string | string | 零值可省略；Go: `string` |
| `hostBootId` | string | string | 零值可省略；Go: `string` |
| `attemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `containerName` | string | string | 零值可省略；Go: `string` |
| `containerId` | string | string | 零值可省略；Go: `string` |
| `executionIdentityDigest` | string | string | 零值可省略；Go: `string` |
| `runtimeStatus` | string | string | 零值可省略；Go: `string` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `executionLeaseExpiresAt` | string | string | 零值可省略；Go: `string` |
| `containerCreatedAt` | string | string | 零值可省略；Go: `string` |
| `supervisorReadyAt` | string | string | 零值可省略；Go: `string` |
| `runPermitIssuedAt` | string | string | 零值可省略；Go: `string` |
| `harnessStartedAt` | string | string | 零值可省略；Go: `string` |
| `deliveryAckedAt` | string | string | 零值可省略；Go: `string` |
| `attemptStatus` | string | string | 零值可省略；Go: `string` |
| `attemptNumber` | integer | integer | 零值可省略；Go: `int` |
| `maxAttempts` | integer | integer | Go: `int` |
| `retryCount` | integer | integer | Go: `int` |
| `nextRetryAt` | string | string | 零值可省略；Go: `string` |
| `lastRetryReasonCode` | string | string | 零值可省略；Go: `string` |
| `recoveryPreviousStatus` | string | string | 零值可省略；Go: `string` |
| `recoveryStartedAt` | string | string | 零值可省略；Go: `string` |
| `reattachDeadline` | string | string | 零值可省略；Go: `string` |
| `pendingWorkerInstanceId` | string | string | 零值可省略；Go: `string` |
| `pendingOwnerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reattachedAt` | string | string | 零值可省略；Go: `string` |
| `watchdogDeadline` | string | string | 零值可省略；Go: `string` |
| `terminalReasonCode` | string | string | 零值可省略；Go: `string` |
| `resolved` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.ResolvedExecutionView`](wire-types.md#t-6374b04d7ddf9ecb)；Go: `*internal/app/workforce/task/domain.ResolvedExecutionView` |

<a id="t-29ed34406ad73388"></a>

## `internal/app/workforce/task/domain.HookPackageReleaseSnapshot`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:170](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L170)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `packageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |
| `digest` | string | string | Go: `string` |
| `runtimeKind` | string | string | Go: `string` |
| `archiveObjectKey` | string | string | 零值可省略；Go: `string` |

<a id="t-3cfef8c6731f17fd"></a>

## `internal/app/workforce/task/domain.ModelSelectionView`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:71](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L71)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialName` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `primaryModel` | string | string | Go: `string` |

<a id="t-6374b04d7ddf9ecb"></a>

## `internal/app/workforce/task/domain.ResolvedExecutionView`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:129](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L129)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `runtimeAlias` | string | string | Go: `string` |
| `runtimeImageDigest` | string | string | Go: `string` |
| `adapterId` | string | string | Go: `string` |
| `adapterVersion` | string | string | Go: `string` |
| `adapterProtocol` | string | string | Go: `string` |
| `adapterCapabilities` | array<string> | array<string> | Go: `[]string` |
| `hookConfigurationDigest` | string | string | 零值可省略；Go: `string` |
| `hooks` | array<integer> | array<integer> | [`internal/app/workforce/task/domain.ResolvedHookView`](wire-types.md#t-e1da3417267cad14)；Go: `[]internal/app/workforce/task/domain.ResolvedHookView` |
| `modelCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `modelCredentialName` | string | string | Go: `string` |
| `modelProtocol` | string | string | Go: `string` |
| `primaryModel` | string | string | Go: `string` |
| `credentialVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerImageContract` | string | string | Go: `string` |
| `isolationProfile` | string | string | Go: `string` |
| `selectionDigest` | string | string | Go: `string` |
| `executionDeadlineSeconds` | integer | integer | Go: `int` |

<a id="t-e1da3417267cad14"></a>

## `internal/app/workforce/task/domain.ResolvedHookView`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:150](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L150)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `releaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `hookId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |

<a id="t-598578e5760a7bda"></a>

## `internal/app/workforce/task/domain.ReviewCorrection`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:342](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L342)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceReviewRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `revisionKind` | string | string | Go: `string` |
| `verdict` | string | string | 零值可省略；Go: `string` |
| `reportSchemaVersion` | string | string | 零值可省略；Go: `string` |
| `reportDigest` | string | string | 零值可省略；Go: `string` |
| `report` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |

<a id="t-09e0b38cc1c3d2a1"></a>

## `internal/app/workforce/task/domain.SelectedRuntimeView`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:78](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L78)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |

<a id="t-d8ef1e519b19e98a"></a>

## `internal/app/workforce/task/domain.Task`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:11](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L11)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `selectedRuntimeReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `selectedRuntime` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.SelectedRuntimeView`](wire-types.md#t-09e0b38cc1c3d2a1)；Go: `*internal/app/workforce/task/domain.SelectedRuntimeView` |
| `executionRequirementsDigest` | string | string | 零值可省略；Go: `string` |
| `name` | string | string | Go: `string` |
| `tags` | array<string> | array<string> | Go: `[]string` |
| `createdBy` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `metadataVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `inputKind` | string | string | 零值可省略；Go: `string` |
| `referenceCount` | integer | integer | 零值可省略；Go: `int` |
| `repositoryUrl` | string | string | Go: `string` |
| `repositoryRef` | string | string | Go: `string` |
| `resolvedBaseCommit` | string | string | 零值可省略；Go: `string` |
| `instruction` | string | string | Go: `string` |
| `resultProcessorReleaseId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `currentTurnNumber` | integer | integer | 零值可省略；Go: `int` |
| `reviewEnabled` | boolean | boolean | Go: `bool` |
| `modelSelection` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.ModelSelectionView`](wire-types.md#t-3cfef8c6731f17fd)；Go: `*internal/app/workforce/task/domain.ModelSelectionView` |
| `status` | integer | integer | [`internal/app/workforce/task/domain.TaskStatus`](wire-types.md#t-00f23df6481c09b6)；Go: `internal/app/workforce/task/domain.TaskStatus` |
| `priority` | integer | integer | Go: `int` |
| `executionTimeoutSeconds` | integer | integer | Go: `int` |
| `executionActiveMilliseconds` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `lastProgressAt` | string | string | 零值可省略；Go: `string` |
| `resultSummary` | string | string | 零值可省略；Go: `string` |
| `latestAttemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `createdAt` | string | string | Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `execution` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.ExecutionView`](wire-types.md#t-69bfdb28cfe61d52)；Go: `*internal/app/workforce/task/domain.ExecutionView` |
| `cancelRequestId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `cancelGeneration` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `cancelRequestedBy` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `cancelRequestedAt` | string | string | 零值可省略；Go: `string` |
| `cancelReason` | string | string | 零值可省略；Go: `string` |

<a id="t-5fa6794d69c32fe2"></a>

## `internal/app/workforce/task/domain.TaskInputContract`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:351](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L351)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceKind` | string | string | Go: `string` |
| `snapshotDigest` | string | string | Go: `string` |
| `workspaceSourceRevisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceCommit` | string | string | Go: `string` |
| `sourceTree` | string | string | Go: `string` |
| `sourceIdentityDigest` | string | string | Go: `string` |
| `repositoryBundleObjectKey` | string | string | Go: `string` |
| `repositoryBundleContentSha256` | string | string | Go: `string` |
| `repositoryBundleObjectSha256` | string | string | Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | Go: `int64` |
| `references` | array<integer> | array<integer> | [`internal/app/workforce/task/domain.TaskInputReference`](wire-types.md#t-a107318e76208c3f)；Go: `[]internal/app/workforce/task/domain.TaskInputReference` |

<a id="t-a107318e76208c3f"></a>

## `internal/app/workforce/task/domain.TaskInputReference`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:390](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L390)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `displayName` | string | string | Go: `string` |
| `contentType` | string | string | Go: `string` |
| `contentSha256` | string | string | Go: `string` |
| `objectKey` | string | string | Go: `string` |
| `objectSha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |

<a id="t-00f23df6481c09b6"></a>

## `internal/app/workforce/task/domain.TaskStatus`

[seven-agent-force-server/internal/app/workforce/task/domain/status.go:5](../../../seven-agent-force-server/internal/app/workforce/task/domain/status.go#L5)

底层类型：`string`。
声明的常量值：`"CANCELLED"`, `"CANCELLING"`, `"CREATED"`, `"FAILED"`, `"INTERACTION_SUSPENDED"`, `"MANUAL_REVIEW"`, `"PACKAGING"`, `"PREPARING"`, `"QUEUED"`, `"RETRY_WAIT"`, `"REVIEWING"`, `"RUNNING"`, `"SUCCEEDED"`, `"TIMED_OUT"`, `"UPLOADING"`, `"VALIDATING"`。
<a id="t-375fdc24e6a36933"></a>

## `internal/app/workforce/task/domain.WorkItem`

[seven-agent-force-server/internal/app/workforce/task/domain/model.go:291](../../../seven-agent-force-server/internal/app/workforce/task/domain/model.go#L291)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `completionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `completionCheckDigest` | string | string | 零值可省略；Go: `string` |
| `completionCheckInputDigest` | string | string | 零值可省略；Go: `string` |
| `completionCheckReferences` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/completioncheck/facade.Reference`](wire-types.md#t-0fbcc46c1346111a)；Go: `[]internal/app/workforce/completioncheck/facade.Reference` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskInput` | integer 或 null | integer 或 null | [`internal/app/workforce/task/domain.TaskInputContract`](wire-types.md#t-5fa6794d69c32fe2)；Go: `*internal/app/workforce/task/domain.TaskInputContract` |
| `instruction` | string | string | Go: `string` |
| `taskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `resultProcessorSelectionMode` | string | string | 零值可省略；Go: `string` |
| `resultProcessorParameters` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `resultProcessorArchiveObjectKey` | string | string | 零值可省略；Go: `string` |
| `validationCommands` | array<string> | array<string> | Go: `[]string` |
| `harnessAlias` | string | string | Go: `string` |
| `harnessReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `harnessImage` | string | string | Go: `string` |
| `harnessImageDigest` | string | string | Go: `string` |
| `executionSnapshot` | integer | integer | [`internal/app/workforce/task/domain.ExecutionSnapshot`](wire-types.md#t-e4e060f9023f9ef1)；Go: `internal/app/workforce/task/domain.ExecutionSnapshot` |
| `taskExecutionSnapshotDigest` | string | string | Go: `string` |
| `modelCredentialName` | string | string | 零值可省略；Go: `string` |
| `primaryModel` | string | string | 零值可省略；Go: `string` |
| `resolvedBaseCommit` | string | string | 零值可省略；Go: `string` |
| `harnessTimeoutSeconds` | integer | integer | Go: `int` |
| `executionActiveMilliseconds` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `continuationRestore` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.ContinuationRestore`](wire-types.md#t-f89855766a823f57)；Go: `*internal/app/workforce/task/domain.ContinuationRestore` |
| `reviewCorrection` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/task/domain.ReviewCorrection`](wire-types.md#t-598578e5760a7bda)；Go: `*internal/app/workforce/task/domain.ReviewCorrection` |
| `allowedAdapterTools` | array<string> | array<string> | 零值可省略；Go: `[]string` |

<a id="t-cbb2f5e0ec1e69ca"></a>

## `internal/app/workforce/task/facade.IndependentReviewReference`

[seven-agent-force-server/internal/app/workforce/task/facade/independent_review.go:19](../../../seven-agent-force-server/internal/app/workforce/task/facade/independent_review.go#L19)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `referenceAssetId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `contentSha256` | string | string | Go: `string` |
| `sizeBytes` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-c395a518b87a2f05"></a>

## `internal/app/workforce/task/facade.ModelSelection`

[seven-agent-force-server/internal/app/workforce/task/facade/submission.go:50](../../../seven-agent-force-server/internal/app/workforce/task/facade/submission.go#L50)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `model` | string | string | Go: `string` |

<a id="t-1529f512453fbef3"></a>

## `internal/app/workforce/task/facade.ProjectTaskRequest`

[seven-agent-force-server/internal/app/workforce/task/facade/submission.go:37](../../../seven-agent-force-server/internal/app/workforce/task/facade/submission.go#L37)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `instruction` | string | string | Go: `string` |
| `name` | string | string | 零值可省略；Go: `string` |
| `tags` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `referenceAssetIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `additionalCompletionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewEnabled` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `deliveryPreference` | string | string | 零值可省略；Go: `string` |
| `modelSelection` | integer | integer | [`internal/app/workforce/task/facade.ModelSelection`](wire-types.md#t-c395a518b87a2f05)；Go: `internal/app/workforce/task/facade.ModelSelection` |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionTimeoutSeconds` | integer | integer | 零值可省略；Go: `int` |

<a id="t-373da65ba27b4995"></a>

## `internal/app/workforce/task/facade.WorkspaceGCEligibility`

[seven-agent-force-server/internal/app/workforce/task/facade/workspace_retention.go:12](../../../seven-agent-force-server/internal/app/workforce/task/facade/workspace_retention.go#L12)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `terminal` | boolean | boolean | Go: `bool` |
| `terminalAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | 零值可省略；Go: `time.Time` |
| `activeReferences` | boolean | boolean | Go: `bool` |
| `portableStateAvailable` | boolean | boolean | Go: `bool` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-0512f13eb3962ee0"></a>

## `internal/app/workforce/task/infrastructure.CancelResult`

[seven-agent-force-server/internal/app/workforce/task/infrastructure/store.go:1049](../../../seven-agent-force-server/internal/app/workforce/task/infrastructure/store.go#L1049)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cancelRequestId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `previousStatus` | string | string | Go: `string` |
| `currentStatus` | string | string | Go: `string` |
| `effectiveStage` | string | string | Go: `string` |

<a id="t-a7bc36d4611fdc43"></a>

## `internal/app/workforce/task/infrastructure.CheckEvidence`

[seven-agent-force-server/internal/app/workforce/task/infrastructure/store.go:1504](../../../seven-agent-force-server/internal/app/workforce/task/infrastructure/store.go#L1504)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `ordinal` | integer | integer | Go: `int` |
| `name` | string | string | Go: `string` |
| `commandDigest` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `exitCode` | integer | integer | Go: `int` |
| `timeoutSeconds` | integer | integer | Go: `int` |
| `output` | string | string | Go: `string` |

<a id="t-bac1fd2af5521ba2"></a>

## `internal/app/workforce/taskanalytics/application.ActivityPoint`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:128](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L128)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `at` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `created` | integer | string(decimal) | Go: `int64` |
| `terminal` | integer | string(decimal) | Go: `int64` |
| `succeeded` | integer | string(decimal) | Go: `int64` |
| `failed` | integer | string(decimal) | Go: `int64` |
| `backlog` | integer | string(decimal) | Go: `int64` |

<a id="t-e58de886a2348d60"></a>

## `internal/app/workforce/taskanalytics/application.Count`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:44](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L44)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `code` | string | string | Go: `string` |
| `count` | integer | string(decimal) | Go: `int64` |

<a id="t-ca99ab0c4eea12d9"></a>

## `internal/app/workforce/taskanalytics/application.DurationPoint`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:37](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L37)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `at` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `sampleCount` | integer | string(decimal) | Go: `int64` |
| `p50Seconds` | number 或 null | number 或 null | Go: `*float64` |
| `p90Seconds` | number 或 null | number 或 null | Go: `*float64` |

<a id="t-1d4c38bc70bd0bd1"></a>

## `internal/app/workforce/taskanalytics/application.ModelUsagePoint`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:97](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L97)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `at` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `invocationCount` | integer | string(decimal) | Go: `int64` |
| `inputTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `cachedInputTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `cacheCreationInputTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `outputTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `reasoningOutputTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `totalTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `cacheHitRate` | number 或 null | number 或 null | Go: `*float64` |

<a id="t-828e42c932531540"></a>

## `internal/app/workforce/taskanalytics/application.ProjectAnalytics`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:109](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L109)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `from` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `to` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `summary` | integer | integer | [`internal/app/workforce/taskanalytics/application.ProjectSummary`](wire-types.md#t-2b898ee369eea48c)；Go: `internal/app/workforce/taskanalytics/application.ProjectSummary` |
| `taskTrend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.TaskTrendPoint`](wire-types.md#t-ea8d82360184db92)；Go: `[]internal/app/workforce/taskanalytics/application.TaskTrendPoint` |
| `stageHealth` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.StageHealth`](wire-types.md#t-f557f736f9677d21)；Go: `[]internal/app/workforce/taskanalytics/application.StageHealth` |
| `durationTrend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.DurationPoint`](wire-types.md#t-ca99ab0c4eea12d9)；Go: `[]internal/app/workforce/taskanalytics/application.DurationPoint` |
| `reviewOutcomes` | integer | integer | [`internal/app/workforce/taskanalytics/application.ReviewOutcomes`](wire-types.md#t-c0373c9e4a4e02d6)；Go: `internal/app/workforce/taskanalytics/application.ReviewOutcomes` |
| `waitingReasons` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.Count`](wire-types.md#t-e58de886a2348d60)；Go: `[]internal/app/workforce/taskanalytics/application.Count` |
| `modelUsageTrend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.ModelUsagePoint`](wire-types.md#t-1d4c38bc70bd0bd1)；Go: `[]internal/app/workforce/taskanalytics/application.ModelUsagePoint` |

<a id="t-ab511446700a0b29"></a>

## `internal/app/workforce/taskanalytics/application.ProjectHealth`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:56](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L56)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `succeeded` | integer | string(decimal) | Go: `int64` |
| `failed` | integer | string(decimal) | Go: `int64` |
| `active` | integer | string(decimal) | Go: `int64` |
| `attention` | integer | string(decimal) | Go: `int64` |
| `latestActivity` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-4edb0c12b0489139"></a>

## `internal/app/workforce/taskanalytics/application.ProjectListAnalytics`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:66](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L66)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `from` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `to` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `summary` | integer | integer | [`internal/app/workforce/taskanalytics/application.ProjectListSummary`](wire-types.md#t-39d10066d4e19aea)；Go: `internal/app/workforce/taskanalytics/application.ProjectListSummary` |
| `taskTrend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.TaskTrendPoint`](wire-types.md#t-ea8d82360184db92)；Go: `[]internal/app/workforce/taskanalytics/application.TaskTrendPoint` |
| `projectHealth` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.ProjectHealth`](wire-types.md#t-ab511446700a0b29)；Go: `[]internal/app/workforce/taskanalytics/application.ProjectHealth` |

<a id="t-39d10066d4e19aea"></a>

## `internal/app/workforce/taskanalytics/application.ProjectListSummary`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:49](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L49)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `activeProjectCount` | integer | string(decimal) | Go: `int64` |
| `createdTaskCount` | integer | string(decimal) | Go: `int64` |
| `succeededTaskCount` | integer | string(decimal) | Go: `int64` |
| `attentionProjectCount` | integer | string(decimal) | Go: `int64` |

<a id="t-2b898ee369eea48c"></a>

## `internal/app/workforce/taskanalytics/application.ProjectSummary`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:74](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L74)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `createdTaskCount` | integer | string(decimal) | Go: `int64` |
| `successRate` | number 或 null | number 或 null | Go: `*float64` |
| `successfulDeliveryCount` | integer | string(decimal) | Go: `int64` |
| `attentionTaskCount` | integer | string(decimal) | Go: `int64` |

<a id="t-c0373c9e4a4e02d6"></a>

## `internal/app/workforce/taskanalytics/application.ReviewOutcomes`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:88](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L88)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `disabled` | integer | string(decimal) | Go: `int64` |
| `directPassed` | integer | string(decimal) | Go: `int64` |
| `passedAfterRevision` | integer | string(decimal) | Go: `int64` |
| `acceptedRisk` | integer | string(decimal) | Go: `int64` |
| `rejected` | integer | string(decimal) | Go: `int64` |
| `processing` | integer | string(decimal) | Go: `int64` |

<a id="t-f557f736f9677d21"></a>

## `internal/app/workforce/taskanalytics/application.StageHealth`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:81](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L81)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `code` | string | string | Go: `string` |
| `eligibleCount` | integer | string(decimal) | Go: `int64` |
| `reachedCount` | integer | string(decimal) | Go: `int64` |
| `failedCount` | integer | string(decimal) | Go: `int64` |

<a id="t-78953998aaecf7d5"></a>

## `internal/app/workforce/taskanalytics/application.TaskAnalytics`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:137](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L137)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `from` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `to` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `summary` | integer | integer | [`internal/app/workforce/taskanalytics/application.TaskSummary`](wire-types.md#t-3b98b8f8bab976e2)；Go: `internal/app/workforce/taskanalytics/application.TaskSummary` |
| `activityTrend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.ActivityPoint`](wire-types.md#t-bac1fd2af5521ba2)；Go: `[]internal/app/workforce/taskanalytics/application.ActivityPoint` |
| `statusCounts` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.Count`](wire-types.md#t-e58de886a2348d60)；Go: `[]internal/app/workforce/taskanalytics/application.Count` |
| `waitingAgeBuckets` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.Count`](wire-types.md#t-e58de886a2348d60)；Go: `[]internal/app/workforce/taskanalytics/application.Count` |
| `waitingReasons` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.Count`](wire-types.md#t-e58de886a2348d60)；Go: `[]internal/app/workforce/taskanalytics/application.Count` |
| `durationTrend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.DurationPoint`](wire-types.md#t-ca99ab0c4eea12d9)；Go: `[]internal/app/workforce/taskanalytics/application.DurationPoint` |

<a id="t-3b98b8f8bab976e2"></a>

## `internal/app/workforce/taskanalytics/application.TaskSummary`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:121](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L121)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `attention` | integer | string(decimal) | Go: `int64` |
| `running` | integer | string(decimal) | Go: `int64` |
| `waiting` | integer | string(decimal) | Go: `int64` |
| `succeeded` | integer | string(decimal) | Go: `int64` |

<a id="t-ea8d82360184db92"></a>

## `internal/app/workforce/taskanalytics/application.TaskTrendPoint`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:29](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L29)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `at` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `created` | integer | string(decimal) | Go: `int64` |
| `succeeded` | integer | string(decimal) | Go: `int64` |
| `failed` | integer | string(decimal) | Go: `int64` |
| `cancelled` | integer | string(decimal) | Go: `int64` |

<a id="t-e1913dc24067b81a"></a>

## `internal/app/workforce/taskanalytics/application.Workbench`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:224](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L224)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `from` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `to` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `summary` | integer | integer | [`internal/app/workforce/taskanalytics/application.WorkbenchSummary`](wire-types.md#t-7a327c1b281baf78)；Go: `internal/app/workforce/taskanalytics/application.WorkbenchSummary` |
| `activeTasks` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.WorkbenchTask`](wire-types.md#t-3ef8e53432458479)；Go: `[]internal/app/workforce/taskanalytics/application.WorkbenchTask` |
| `attentionItems` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.WorkbenchAttentionItem`](wire-types.md#t-403c2da16253f901)；Go: `[]internal/app/workforce/taskanalytics/application.WorkbenchAttentionItem` |
| `activityTrend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.WorkbenchActivityPoint`](wire-types.md#t-ff7a3fb5d64b6b4e)；Go: `[]internal/app/workforce/taskanalytics/application.WorkbenchActivityPoint` |
| `resources` | integer | integer | [`internal/app/workforce/taskanalytics/application.WorkbenchResources`](wire-types.md#t-ad993c1131a4ee56)；Go: `internal/app/workforce/taskanalytics/application.WorkbenchResources` |
| `modelUsage` | integer 或 null | integer 或 null | [`internal/app/workforce/taskanalytics/application.WorkbenchModelUsage`](wire-types.md#t-435c45ab5f6394e1)；Go: `*internal/app/workforce/taskanalytics/application.WorkbenchModelUsage` |
| `recentDeliveries` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.WorkbenchDelivery`](wire-types.md#t-9dfd57f193d2f56a)；Go: `[]internal/app/workforce/taskanalytics/application.WorkbenchDelivery` |

<a id="t-ff7a3fb5d64b6b4e"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchActivityPoint`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:180](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L180)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `at` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `created` | integer | string(decimal) | Go: `int64` |
| `completed` | integer | string(decimal) | Go: `int64` |
| `backlog` | integer | string(decimal) | Go: `int64` |

<a id="t-403c2da16253f901"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchAttentionItem`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:169](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L169)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `objectId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `taskName` | string | string | Go: `string` |
| `projectName` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `objectKind` | string | string | Go: `string` |
| `occurredAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-9dfd57f193d2f56a"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchDelivery`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:214](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L214)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `deliveryId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskName` | string | string | Go: `string` |
| `projectName` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-435c45ab5f6394e1"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchModelUsage`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:207](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L207)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `invocationCount` | integer | string(decimal) | Go: `int64` |
| `totalTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |
| `cacheHitRate` | number 或 null | number 或 null | Go: `*float64` |
| `trend` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.WorkbenchModelUsagePoint`](wire-types.md#t-21d2a7a6aff121d6)；Go: `[]internal/app/workforce/taskanalytics/application.WorkbenchModelUsagePoint` |

<a id="t-21d2a7a6aff121d6"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchModelUsagePoint`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:202](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L202)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `at` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `totalTokens` | integer 或 null | string(decimal) 或 null | Go: `*int64` |

<a id="t-ad993c1131a4ee56"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchResources`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:194](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L194)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `onlineWorkerCount` | integer | string(decimal) | Go: `int64` |
| `runningSlots` | integer | string(decimal) | Go: `int64` |
| `maximumSlots` | integer | string(decimal) | Go: `int64` |
| `queuedTasks` | integer | string(decimal) | Go: `int64` |
| `workers` | array<integer> | array<integer> | [`internal/app/workforce/taskanalytics/application.WorkbenchWorker`](wire-types.md#t-9c780752f702860f)；Go: `[]internal/app/workforce/taskanalytics/application.WorkbenchWorker` |

<a id="t-7a327c1b281baf78"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchSummary`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:148](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L148)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `attentionCount` | integer | string(decimal) | Go: `int64` |
| `runningTaskCount` | integer | string(decimal) | Go: `int64` |
| `queuedTaskCount` | integer | string(decimal) | Go: `int64` |
| `completedTodayCount` | integer | string(decimal) | Go: `int64` |
| `todaySuccessRate` | number 或 null | number 或 null | Go: `*float64` |

<a id="t-3ef8e53432458479"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchTask`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:156](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L156)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `projectName` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `runtimeName` | string | string | Go: `string` |
| `progressSummary` | string | string | Go: `string` |
| `waitReasonCode` | string | string | 零值可省略；Go: `string` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `startedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |

<a id="t-9c780752f702860f"></a>

## `internal/app/workforce/taskanalytics/application.WorkbenchWorker`

[seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go:187](../../../seven-agent-force-server/internal/app/workforce/taskanalytics/application/service.go#L187)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `runningSlots` | integer | string(decimal) | Go: `int64` |
| `maximumSlots` | integer | string(decimal) | Go: `int64` |

<a id="t-0547425de299ad84"></a>

## `internal/app/workforce/taskdispatch/controller.artifactInput`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1293](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1293)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `type` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `objectKey` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |
| `mediaType` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `metadata` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-8d5e3c6aea609842"></a>

## `internal/app/workforce/taskdispatch/controller.artifactsRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1304](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1304)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `artifacts` | array<integer> | array<integer> | [`internal/app/workforce/taskdispatch/controller.artifactInput`](wire-types.md#t-0547425de299ad84)；Go: `[]internal/app/workforce/taskdispatch/controller.artifactInput` |

<a id="t-6c1fa5628a43d246"></a>

## `internal/app/workforce/taskdispatch/controller.checkInput`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1102](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1102)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `ordinal` | integer | integer | Go: `int` |
| `name` | string | string | Go: `string` |
| `command` | string | string | Go: `string` |
| `exitCode` | integer | integer | Go: `int` |
| `output` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |

<a id="t-db0aa9590a571a14"></a>

## `internal/app/workforce/taskdispatch/controller.checksRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1110](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1110)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `checks` | array<integer> | array<integer> | [`internal/app/workforce/taskdispatch/controller.checkInput`](wire-types.md#t-6c1fa5628a43d246)；Go: `[]internal/app/workforce/taskdispatch/controller.checkInput` |

<a id="t-d54e5c07e8750b63"></a>

## `internal/app/workforce/taskdispatch/controller.claimRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:705](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L705)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `executionRequirementDigest` | string | string | Go: `string` |

<a id="t-31d74f94537ad213"></a>

## `internal/app/workforce/taskdispatch/controller.completionRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1079](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1079)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `completion` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-845b6de22a9314ca"></a>

## `internal/app/workforce/taskdispatch/controller.containerCreatedRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:906](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L906)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `containerId` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |

<a id="t-4da93b7acdfbd0f7"></a>

## `internal/app/workforce/taskdispatch/controller.continuationCheckpointRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1190](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1190)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `checkpoint` | integer | integer | [`internal/app/workforce/resultprocessor/facade.ContinuationCheckpointCandidate`](wire-types.md#t-8b21b5d551e70b86)；Go: `internal/app/workforce/resultprocessor/facade.ContinuationCheckpointCandidate` |

<a id="t-810dc39cb5ca3b4d"></a>

## `internal/app/workforce/taskdispatch/controller.continuationKeyRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:512](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L512)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `checkpointIdentity` | string | string | Go: `string` |

<a id="t-c1dacba809dcd927"></a>

## `internal/app/workforce/taskdispatch/controller.continuationRestoreKeyRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:531](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L531)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |

<a id="t-c0332ca759674163"></a>

## `internal/app/workforce/taskdispatch/controller.continuationRestoreResultRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:550](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L550)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `restoreRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultDigest` | string | string | Go: `string` |
| `baseCommit` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |

<a id="t-5e692640557e4821"></a>

## `internal/app/workforce/taskdispatch/controller.deliveryCredentialRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:324](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L324)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |

<a id="t-29272c288d8dc514"></a>

## `internal/app/workforce/taskdispatch/controller.deliveryFailureRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:398](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L398)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-f67ba4457583ee07"></a>

## `internal/app/workforce/taskdispatch/controller.deliveryPrepareRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:342](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L342)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `deliveryCommit` | string | string | Go: `string` |

<a id="t-5cc6d556aab025e5"></a>

## `internal/app/workforce/taskdispatch/controller.deliveryReconciliationRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:380](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L380)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/delivery/application.DeliveryResult`](wire-types.md#t-1a585eac31214972)；Go: `internal/app/workforce/delivery/application.DeliveryResult` |

<a id="t-4eab194eab49d1ad"></a>

## `internal/app/workforce/taskdispatch/controller.deliveryResultRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:361](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L361)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `pushAuthorization` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/delivery/application.DeliveryResult`](wire-types.md#t-1a585eac31214972)；Go: `internal/app/workforce/delivery/application.DeliveryResult` |

<a id="t-06c94ac1879faa14"></a>

## `internal/app/workforce/taskdispatch/controller.evaluationReplayRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/evaluation_replay.go:11](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/evaluation_replay.go#L11)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `pending` | integer | integer | [`internal/app/workforce/resultprocessor/facade.EvaluationReplayIdentity`](wire-types.md#t-47ef88aef1917589)；Go: `internal/app/workforce/resultprocessor/facade.EvaluationReplayIdentity` |

<a id="t-35b7bf49020c3244"></a>

## `internal/app/workforce/taskdispatch/controller.factsRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1045](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1045)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `facts` | array<integer> | array<integer> | [`internal/app/workforce/taskdispatch/domain.Fact`](wire-types.md#t-a555a5c104bf8ed2)；Go: `[]internal/app/workforce/taskdispatch/domain.Fact` |

<a id="t-4ec29bce16336dcf"></a>

## `internal/app/workforce/taskdispatch/controller.fixtureClaimRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1216](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1216)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-f09e06a198710518"></a>

## `internal/app/workforce/taskdispatch/controller.fixtureResultRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1235](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1235)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `result` | integer | integer | [`internal/app/workforce/resultprocessor/facade.FixtureResult`](wire-types.md#t-6ecead615b31bf3d)；Go: `internal/app/workforce/resultprocessor/facade.FixtureResult` |

<a id="t-1c48e0532b4a52ca"></a>

## `internal/app/workforce/taskdispatch/controller.freezeRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:610](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L610)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/artifact/domain.FreezeResult`](wire-types.md#t-3061dadeee2262bb)；Go: `internal/app/workforce/artifact/domain.FreezeResult` |

<a id="t-d9a37d495d1bce8b"></a>

## `internal/app/workforce/taskdispatch/controller.harnessStartedRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:957](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L957)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `started` | integer | integer | [`internal/app/workforce/taskdispatch/domain.HarnessStarted`](wire-types.md#t-76278493a6e091ef)；Go: `internal/app/workforce/taskdispatch/domain.HarnessStarted` |

<a id="t-9a19597dbb49b5bf"></a>

## `internal/app/workforce/taskdispatch/controller.interactionEnvelope`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go:35](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction.go#L35)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-28897326f63cbcd9"></a>

## `internal/app/workforce/taskdispatch/controller.leaseRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:887](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L887)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `activeElapsedMillis` | string(decimal) 或 null | string(decimal) 或 null | 零值可省略；JSON 引号编码；Go: `*int64` |

<a id="t-9b0fcbed42d61fd0"></a>

## `internal/app/workforce/taskdispatch/controller.materializationFailureRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:493](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L493)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-31128fe5d65e34bf"></a>

## `internal/app/workforce/taskdispatch/controller.materializationFreezeRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:474](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L474)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/continuation/domain.MaterializationFreeze`](wire-types.md#t-d1654b86d786a887)；Go: `internal/app/workforce/continuation/domain.MaterializationFreeze` |

<a id="t-b4f3adddbc06c6db"></a>

## `internal/app/workforce/taskdispatch/controller.outcomeRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1334](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1334)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `outcome` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `message` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `summary` | string | string | Go: `string` |

<a id="t-b6d513fdac3291f3"></a>

## `internal/app/workforce/taskdispatch/controller.postprocessClaimRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:595](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L595)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-1b152dc2c3cf52a6"></a>

## `internal/app/workforce/taskdispatch/controller.postprocessFailureRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:628](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L628)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-4e33385067ea8aee"></a>

## `internal/app/workforce/taskdispatch/controller.processorMaterializationRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1143](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1143)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `cycleId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `cycleNumber` | integer | integer | Go: `int` |

<a id="t-d7e2b508f5d14f2c"></a>

## `internal/app/workforce/taskdispatch/controller.providerProxyReadyRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:577](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L577)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `ready` | integer | integer | [`internal/app/workforce/taskdispatch/domain.ProviderProxyReady`](wire-types.md#t-1de3f534590336a9)；Go: `internal/app/workforce/taskdispatch/domain.ProviderProxyReady` |

<a id="t-a27d1078ad478922"></a>

## `internal/app/workforce/taskdispatch/controller.reattachAckRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1007](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1007)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `ack` | integer | integer | [`internal/app/workforce/taskdispatch/domain.ReattachAck`](wire-types.md#t-494b89344ead4de3)；Go: `internal/app/workforce/taskdispatch/domain.ReattachAck` |

<a id="t-081ea713c9382f3e"></a>

## `internal/app/workforce/taskdispatch/controller.reattachPermitRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:989](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L989)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `evidence` | integer | integer | [`internal/app/workforce/taskdispatch/domain.RuntimeEvidence`](wire-types.md#t-f633aecbece0f37e)；Go: `internal/app/workforce/taskdispatch/domain.RuntimeEvidence` |

<a id="t-062cbf0baaa0d899"></a>

## `internal/app/workforce/taskdispatch/controller.releaseAssignmentRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:688](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L688)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-aa7d61165d0822fd"></a>

## `internal/app/workforce/taskdispatch/controller.resultEvaluationRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1167](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1167)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `candidate` | integer | integer | [`internal/app/workforce/resultprocessor/facade.EvaluationCandidate`](wire-types.md#t-1cb3eba0f7bf13fb)；Go: `internal/app/workforce/resultprocessor/facade.EvaluationCandidate` |

<a id="t-6e96a09c4683d1a0"></a>

## `internal/app/workforce/taskdispatch/controller.reviewClaimRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:723](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L723)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-64c215f56cb17d2f"></a>

## `internal/app/workforce/taskdispatch/controller.reviewContainerRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:807](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L807)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `permitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `permitToken` | string | string | Go: `string` |
| `containerId` | string | string | Go: `string` |
| `containerName` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |

<a id="t-ba0ac47d3cef95ac"></a>

## `internal/app/workforce/taskdispatch/controller.reviewFailureRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:861](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L861)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-8086fb2559c7abb7"></a>

## `internal/app/workforce/taskdispatch/controller.reviewLeaseRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:767](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L767)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |

<a id="t-9f3a430337ecee42"></a>

## `internal/app/workforce/taskdispatch/controller.reviewRecoveryRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:742](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L742)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `containerId` | string | string | Go: `string` |
| `containerName` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |

<a id="t-9213912d5cdba281"></a>

## `internal/app/workforce/taskdispatch/controller.reviewReportRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:835](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L835)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `permitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `permitToken` | string | string | Go: `string` |
| `report` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `usage` | integer | integer | [`internal/app/workforce/reviewrun/domain.UsageFacts`](wire-types.md#t-8db36876e421763a)；Go: `internal/app/workforce/reviewrun/facade.UsageFacts` |

<a id="t-e051a0d57383510e"></a>

## `internal/app/workforce/taskdispatch/controller.runtimeLossRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:1026](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L1026)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reasonCode` | string | string | Go: `string` |
| `evidenceDigest` | string | string | 零值可省略；Go: `string` |

<a id="t-404867bd00b530df"></a>

## `internal/app/workforce/taskdispatch/controller.sourceIngestionFailureRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:279](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L279)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-6917fe13dd6d9b6c"></a>

## `internal/app/workforce/taskdispatch/controller.sourceIngestionLeaseRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:224](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L224)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |

<a id="t-cb33383fd6a0a2ec"></a>

## `internal/app/workforce/taskdispatch/controller.sourceIngestionObservationRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:242](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L242)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `resolvedCommit` | string | string | Go: `string` |

<a id="t-03ffd4bcb5caba8d"></a>

## `internal/app/workforce/taskdispatch/controller.sourceIngestionResultRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:260](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L260)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `result` | integer | integer | [`internal/app/workforce/workspaceinput/application.SourceIngestionResult`](wire-types.md#t-39631ebb08f1ced3)；Go: `internal/app/workforce/workspaceinput/application.SourceIngestionResult` |

<a id="t-ca09ca061ad0cab6"></a>

## `internal/app/workforce/taskdispatch/controller.supervisorReadyRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:926](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L926)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `ready` | integer | integer | [`internal/app/workforce/taskdispatch/domain.SupervisorReady`](wire-types.md#t-6f962b67246eb397)；Go: `internal/app/workforce/taskdispatch/domain.SupervisorReady` |

<a id="t-3aa0e2a6dc976a74"></a>

## `internal/app/workforce/taskdispatch/controller.suspendEnvelope`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go:20](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go#L20)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `checkpointIdentity` | string | string | Go: `string` |

<a id="t-4f6d5d8287bd17d0"></a>

## `internal/app/workforce/taskdispatch/controller.suspendedBlob`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go:70](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/interaction_suspend.go#L70)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `objectKey` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-bd4b422284a3bcdb"></a>

## `internal/app/workforce/taskdispatch/controller.uploadVerificationRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:659](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L659)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |

<a id="t-baeb18a3c002cfb4"></a>

## `internal/app/workforce/taskdispatch/controller.workspaceGCEligibilityRequest`

[seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go:175](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/controller/machine.go#L175)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-8353cfb14d51a576"></a>

## `internal/app/workforce/taskdispatch/domain.AssignmentMessage`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:13](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L13)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `kind` | string | string | Go: `string` |
| `messageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `assignmentId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionRequirementDigest` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |

<a id="t-b34c0b987fd65a4d"></a>

## `internal/app/workforce/taskdispatch/domain.DeliveryDirective`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:36](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L36)

底层类型：`string`。
声明的常量值：`"ACK_ALREADY_RUNNING"`, `"ACK_TERMINAL"`, `"RECONCILE_LIVE_RUNTIME"`, `"REJECT_STALE_ASSIGNMENT"`, `"RESUME_AUTHORIZED_RUNTIME"`, `"START_NEW_RUNTIME"`。
<a id="t-13223d8862bd16fa"></a>

## `internal/app/workforce/taskdispatch/domain.ExecutionContract`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:47](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L47)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `directive` | integer | integer | [`internal/app/workforce/taskdispatch/domain.DeliveryDirective`](wire-types.md#t-b34c0b987fd65a4d)；Go: `internal/app/workforce/taskdispatch/domain.DeliveryDirective` |
| `assignmentId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionRequirementDigest` | string | string | Go: `string` |
| `attempt` | integer | integer | [`internal/app/workforce/task/domain.Attempt`](wire-types.md#t-9c2cda27ed812dc1)；Go: `internal/app/workforce/task/domain.Attempt` |
| `workItem` | integer | integer | [`internal/app/workforce/task/domain.WorkItem`](wire-types.md#t-375fdc24e6a36933)；Go: `internal/app/workforce/task/domain.WorkItem` |
| `attemptLeaseToken` | string | string | Go: `string` |
| `attemptLeaseExpiresAt` | string | string | Go: `string` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-a555a5c104bf8ed2"></a>

## `internal/app/workforce/taskdispatch/domain.Fact`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:165](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L165)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `factId` | string | string | Go: `string` |
| `producerSequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `eventType` | string | string | Go: `string` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `payloadDigest` | string | string | Go: `string` |

<a id="t-76278493a6e091ef"></a>

## `internal/app/workforce/taskdispatch/domain.HarnessStarted`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:157](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L157)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runPermitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `runPermitDigest` | string | string | Go: `string` |
| `runNonceDigest` | string | string | Go: `string` |
| `harnessPid` | integer | integer | Go: `int` |
| `startedAt` | string | string | Go: `string` |

<a id="t-bdb4c8321ddbbcac"></a>

## `internal/app/workforce/taskdispatch/domain.InteractionReplayBinding`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/interaction_replay.go:7](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/interaction_replay.go#L7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reconciliationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `oldOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `newOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `containerId` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `roundKey` | string | string | Go: `string` |

<a id="t-37e759f82846b697"></a>

## `internal/app/workforce/taskdispatch/domain.LaunchPermit`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:86](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L86)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `permitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `permitToken` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |
| `expectedContainerName` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |
| `labels` | object<string, string> | object<string, string> | Go: `map[string]string` |

<a id="t-484ce7e7dbd22e9b"></a>

## `internal/app/workforce/taskdispatch/domain.MachineEnvelope`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:26](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L26)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-6fea488d5eed1d14"></a>

## `internal/app/workforce/taskdispatch/domain.ProviderProxyEvidence`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:94](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L94)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `containerId` | string | string | Go: `string` |
| `containerName` | string | string | Go: `string` |
| `labelsDigest` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `controlState` | string | string | Go: `string` |

<a id="t-1de3f534590336a9"></a>

## `internal/app/workforce/taskdispatch/domain.ProviderProxyReady`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:149](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L149)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `containerId` | string | string | Go: `string` |
| `containerName` | string | string | Go: `string` |
| `imageDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |
| `controlState` | string | string | Go: `string` |

<a id="t-96d0a4a8267b3a77"></a>

## `internal/app/workforce/taskdispatch/domain.ProviderProxyReattachAck`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:253](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L253)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reattachPermitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reattachPermitDigest` | string | string | Go: `string` |
| `oldOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `newOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `controlState` | string | string | Go: `string` |

<a id="t-494b89344ead4de3"></a>

## `internal/app/workforce/taskdispatch/domain.ReattachAck`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:237](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L237)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `containerId` | string | string | 零值可省略；Go: `string` |
| `executionIdentityDigest` | string | string | 零值可省略；Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `roundKey` | string | string | 零值可省略；Go: `string` |
| `reattachPermitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reattachPermitDigest` | string | string | Go: `string` |
| `oldOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `newOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `supervisorState` | string | string | Go: `string` |
| `harnessPid` | integer | integer | Go: `int` |
| `runNonceDigest` | string | string | 零值可省略；Go: `string` |
| `lastJournalSequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `providerProxy` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/taskdispatch/domain.ProviderProxyReattachAck`](wire-types.md#t-96d0a4a8267b3a77)；Go: `*internal/app/workforce/taskdispatch/domain.ProviderProxyReattachAck` |

<a id="t-dbb03c3ccc46b41a"></a>

## `internal/app/workforce/taskdispatch/domain.ReattachMode`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:169](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L169)

底层类型：`string`。
声明的常量值：`"FOLLOW_RUNNING_RUNTIME"`, `"RESUME_GATED_RUNTIME"`。
<a id="t-7507316b8fb7a560"></a>

## `internal/app/workforce/taskdispatch/domain.ReattachPermit`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:220](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L220)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `interactionReplayRequested` | boolean | boolean | 零值可省略；Go: `bool` |
| `containerId` | string | string | 零值可省略；Go: `string` |
| `executionIdentityDigest` | string | string | 零值可省略；Go: `string` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `roundKey` | string | string | 零值可省略；Go: `string` |
| `reattachPermitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reattachPermitToken` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |
| `oldOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `newOwnerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionLeaseToken` | string | string | Go: `string` |
| `executionLeaseExpiresAt` | string | string | Go: `string` |
| `mode` | integer | integer | [`internal/app/workforce/taskdispatch/domain.ReattachMode`](wire-types.md#t-dbb03c3ccc46b41a)；Go: `internal/app/workforce/taskdispatch/domain.ReattachMode` |
| `startNeeded` | boolean | boolean | Go: `bool` |

<a id="t-84cc8dde8d0708e0"></a>

## `internal/app/workforce/taskdispatch/domain.ReattachResult`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:261](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L261)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `interactionReplayBinding` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/taskdispatch/domain.InteractionReplayBinding`](wire-types.md#t-bdb4c8321ddbbcac)；Go: `*internal/app/workforce/taskdispatch/domain.InteractionReplayBinding` |
| `contract` | integer | integer | [`internal/app/workforce/taskdispatch/domain.ExecutionContract`](wire-types.md#t-13223d8862bd16fa)；Go: `internal/app/workforce/taskdispatch/domain.ExecutionContract` |
| `runtimeState` | string | string | Go: `string` |
| `mode` | integer | integer | [`internal/app/workforce/taskdispatch/domain.ReattachMode`](wire-types.md#t-dbb03c3ccc46b41a)；Go: `internal/app/workforce/taskdispatch/domain.ReattachMode` |
| `startNeeded` | boolean | boolean | Go: `bool` |

<a id="t-1e2763af6d727d92"></a>

## `internal/app/workforce/taskdispatch/domain.RecoverableRuntime`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:198](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L198)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `assignmentId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `hostBootId` | string | string | Go: `string` |
| `expectedContainerName` | string | string | Go: `string` |
| `containerId` | string | string | Go: `string` |
| `runtimeStatus` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |
| `harnessImageDigest` | string | string | Go: `string` |
| `hookConfigurationDigest` | string | string | Go: `string` |
| `providerBacked` | boolean | boolean | Go: `bool` |
| `providerProxyContainerId` | string | string | 零值可省略；Go: `string` |
| `providerProxyContainerName` | string | string | 零值可省略；Go: `string` |
| `providerProxyImageDigest` | string | string | 零值可省略；Go: `string` |
| `providerProxyExecutionIdentityDigest` | string | string | 零值可省略；Go: `string` |
| `reattachDeadline` | string | string | Go: `string` |

<a id="t-32066c176c444c32"></a>

## `internal/app/workforce/taskdispatch/domain.RunPermit`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:95](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L95)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runPermitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `runPermitToken` | string | string | Go: `string` |
| `runNonce` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-f633aecbece0f37e"></a>

## `internal/app/workforce/taskdispatch/domain.RuntimeEvidence`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:104](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L104)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `interactionReplayRequested` | boolean | boolean | 零值可省略；Go: `bool` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `roundKey` | string | string | 零值可省略；Go: `string` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `assignmentId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `hostBootId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `containerId` | string | string | Go: `string` |
| `containerName` | string | string | Go: `string` |
| `labelsDigest` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |
| `harnessImageDigest` | string | string | Go: `string` |
| `hookConfigurationDigest` | string | string | Go: `string` |
| `supervisorProtocol` | string | string | Go: `string` |
| `supervisorState` | string | string | Go: `string` |
| `harnessPid` | integer | integer | Go: `int` |
| `runNonceDigest` | string | string | 零值可省略；Go: `string` |
| `lastJournalSequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `providerProxy` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/taskdispatch/domain.ProviderProxyEvidence`](wire-types.md#t-6fea488d5eed1d14)；Go: `*internal/app/workforce/taskdispatch/domain.ProviderProxyEvidence` |

<a id="t-88932fa7ff616165"></a>

## `internal/app/workforce/taskdispatch/domain.RuntimeLossResult`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go:269](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/recovery.go#L269)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `disposition` | string | string | Go: `string` |
| `attemptCount` | integer | integer | Go: `int` |
| `maxAttempts` | integer | integer | Go: `int` |
| `nextGeneration` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-6f962b67246eb397"></a>

## `internal/app/workforce/taskdispatch/domain.SupervisorReady`

[seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go:140](../../../seven-agent-force-server/internal/app/workforce/taskdispatch/domain/model.go#L140)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `containerId` | string | string | Go: `string` |
| `controlProtocol` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |

<a id="t-5b8c756dd4374cc1"></a>

## `internal/app/workforce/tasknotification/application.ChannelReadiness`

[seven-agent-force-server/internal/app/workforce/tasknotification/application/service.go:51](../../../seven-agent-force-server/internal/app/workforce/tasknotification/application/service.go#L51)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `available` | boolean | boolean | Go: `bool` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `directReady` | boolean | boolean | 零值可省略；Go: `bool` |
| `mentionReady` | boolean | boolean | 零值可省略；Go: `bool` |

<a id="t-23fae19b8d25418f"></a>

## `internal/app/workforce/tasknotification/application.Readiness`

[seven-agent-force-server/internal/app/workforce/tasknotification/application/service.go:43](../../../seven-agent-force-server/internal/app/workforce/tasknotification/application/service.go#L43)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `inApp` | integer | integer | [`internal/app/workforce/tasknotification/application.ChannelReadiness`](wire-types.md#t-5b8c756dd4374cc1)；Go: `internal/app/workforce/tasknotification/application.ChannelReadiness` |
| `email` | integer | integer | [`internal/app/workforce/tasknotification/application.ChannelReadiness`](wire-types.md#t-5b8c756dd4374cc1)；Go: `internal/app/workforce/tasknotification/application.ChannelReadiness` |
| `feishu` | integer | integer | [`internal/app/workforce/tasknotification/application.ChannelReadiness`](wire-types.md#t-5b8c756dd4374cc1)；Go: `internal/app/workforce/tasknotification/application.ChannelReadiness` |
| `wecom` | integer | integer | [`internal/app/workforce/tasknotification/application.ChannelReadiness`](wire-types.md#t-5b8c756dd4374cc1)；Go: `internal/app/workforce/tasknotification/application.ChannelReadiness` |
| `groupRoutes` | array<integer> | array<integer> | [`internal/app/notification/facade.GroupRouteOption`](wire-types.md#t-2b93a282f225a50e)；Go: `[]internal/app/notification/facade.GroupRouteOption` |

<a id="t-fadf230ce1eeaaff"></a>

## `internal/app/workforce/tasknotification/application.RecordView`

[seven-agent-force-server/internal/app/workforce/tasknotification/application/service.go:157](../../../seven-agent-force-server/internal/app/workforce/tasknotification/application/service.go#L157)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceType` | integer | integer | [`internal/app/workforce/tasknotification/domain.SourceType`](wire-types.md#t-9244ad1bcaec6afb)；Go: `internal/app/workforce/tasknotification/domain.SourceType` |
| `sourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceOccurrence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `factIdentityVersion` | integer | integer | Go: `int` |
| `factType` | integer | integer | [`internal/app/workforce/tasknotification/domain.FactType`](wire-types.md#t-26bedcb14f61a59b)；Go: `internal/app/workforce/tasknotification/domain.FactType` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `focus` | integer | integer | [`internal/app/workforce/tasknotification/domain.Focus`](wire-types.md#t-4b29b30cc0283804)；Go: `internal/app/workforce/tasknotification/domain.Focus` |
| `subscriptionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `subscriberUserId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `subscriptionVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskNameSnapshot` | string | string | Go: `string` |
| `turnNumber` | integer | integer | Go: `int` |
| `summarySnapshot` | string | string | 零值可省略；Go: `string` |
| `titleSnapshot` | string | string | Go: `string` |
| `contentSnapshot` | string | string | Go: `string` |
| `publicationContractVersion` | integer | integer | Go: `int` |
| `sceneCode` | string | string | 零值可省略；Go: `string` |
| `deepLink` | string | string | Go: `string` |
| `idempotencyIdentity` | string | string | Go: `string` |
| `requestDigest` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `publishAttempts` | integer | integer | Go: `int` |
| `nextRetryAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `notificationReceiptIdentity` | string | string | 零值可省略；Go: `string` |
| `warningCode` | string | string | 零值可省略；Go: `string` |
| `lastErrorCode` | string | string | 零值可省略；Go: `string` |
| `occurredAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `publishedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `channels` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/tasknotification/domain.RecordChannel`](wire-types.md#t-eb18126e6f91c514)；Go: `[]internal/app/workforce/tasknotification/domain.RecordChannel` |
| `recipients` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/tasknotification/domain.RecordRecipient`](wire-types.md#t-a97c4290e3bf7600)；Go: `[]internal/app/workforce/tasknotification/domain.RecordRecipient` |
| `deliveries` | array<integer> | array<integer> | [`internal/app/notification/facade.PublicationChannelStatus`](wire-types.md#t-1b30371acce7d610)；Go: `[]internal/app/notification/facade.PublicationChannelStatus` |

<a id="t-5e9beaea2948de73"></a>

## `internal/app/workforce/tasknotification/domain.ChannelSelection`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:88](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L88)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `kind` | integer | integer | [`internal/app/workforce/tasknotification/domain.ChannelType`](wire-types.md#t-d6a4b66b3f28d2fe)；Go: `internal/app/workforce/tasknotification/domain.ChannelType` |
| `routeId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-d6a4b66b3f28d2fe"></a>

## `internal/app/workforce/tasknotification/domain.ChannelType`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:50](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L50)

底层类型：`string`。
声明的常量值：`"EMAIL_USER"`, `"FEISHU_DIRECT"`, `"GROUP_ROUTE"`, `"IN_APP"`, `"WECOM_DIRECT"`。
<a id="t-26bedcb14f61a59b"></a>

## `internal/app/workforce/tasknotification/domain.FactType`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:16](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L16)

底层类型：`string`。
声明的常量值：`"TASK_ACTION_REQUIRED"`, `"TASK_COMPLETED"`, `"DELIVERY_COMPLETED"`, `"TASK_INTERRUPTED"`。
<a id="t-4b29b30cc0283804"></a>

## `internal/app/workforce/tasknotification/domain.Focus`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:41](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L41)

底层类型：`string`。
声明的常量值：`"continuation"`, `"delivery"`, `"evaluation"`, `"result"`。
<a id="t-3952e6f2e856a89f"></a>

## `internal/app/workforce/tasknotification/domain.OutboxMessage`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:653](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L653)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `recordId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-dac05fbfed121120"></a>

## `internal/app/workforce/tasknotification/domain.PersonalChannelPolicy`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:470](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L470)

底层类型：`string`。
声明的常量值：`"TASK_SUBSCRIPTION"`, `"USER_PREFERENCE"`。
<a id="t-d0a9f86d4f44b977"></a>

## `internal/app/workforce/tasknotification/domain.RecipientRole`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:469](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L469)

底层类型：`string`。
声明的常量值：`"ATTENTION"`, `"SUBSCRIBER"`。
<a id="t-d4e23f57e8bc0a68"></a>

## `internal/app/workforce/tasknotification/domain.Record`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:427](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L427)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceType` | integer | integer | [`internal/app/workforce/tasknotification/domain.SourceType`](wire-types.md#t-9244ad1bcaec6afb)；Go: `internal/app/workforce/tasknotification/domain.SourceType` |
| `sourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceOccurrence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `factIdentityVersion` | integer | integer | Go: `int` |
| `factType` | integer | integer | [`internal/app/workforce/tasknotification/domain.FactType`](wire-types.md#t-26bedcb14f61a59b)；Go: `internal/app/workforce/tasknotification/domain.FactType` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `focus` | integer | integer | [`internal/app/workforce/tasknotification/domain.Focus`](wire-types.md#t-4b29b30cc0283804)；Go: `internal/app/workforce/tasknotification/domain.Focus` |
| `subscriptionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `subscriberUserId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `subscriptionVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskNameSnapshot` | string | string | Go: `string` |
| `turnNumber` | integer | integer | Go: `int` |
| `summarySnapshot` | string | string | 零值可省略；Go: `string` |
| `titleSnapshot` | string | string | Go: `string` |
| `contentSnapshot` | string | string | Go: `string` |
| `publicationContractVersion` | integer | integer | Go: `int` |
| `sceneCode` | string | string | 零值可省略；Go: `string` |
| `deepLink` | string | string | Go: `string` |
| `idempotencyIdentity` | string | string | Go: `string` |
| `requestDigest` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `publishAttempts` | integer | integer | Go: `int` |
| `nextRetryAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `notificationReceiptIdentity` | string | string | 零值可省略；Go: `string` |
| `warningCode` | string | string | 零值可省略；Go: `string` |
| `lastErrorCode` | string | string | 零值可省略；Go: `string` |
| `occurredAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `publishedAt` | string(RFC3339/RFC3339Nano) 或 null | string(RFC3339/RFC3339Nano) 或 null | 零值可省略；Go: `*time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `channels` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/tasknotification/domain.RecordChannel`](wire-types.md#t-eb18126e6f91c514)；Go: `[]internal/app/workforce/tasknotification/domain.RecordChannel` |
| `recipients` | array<integer> | array<integer> | 零值可省略；[`internal/app/workforce/tasknotification/domain.RecordRecipient`](wire-types.md#t-a97c4290e3bf7600)；Go: `[]internal/app/workforce/tasknotification/domain.RecordRecipient` |

<a id="t-eb18126e6f91c514"></a>

## `internal/app/workforce/tasknotification/domain.RecordChannel`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:486](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L486)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `kind` | integer | integer | [`internal/app/workforce/tasknotification/domain.ChannelType`](wire-types.md#t-d6a4b66b3f28d2fe)；Go: `internal/app/workforce/tasknotification/domain.ChannelType` |
| `routeId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `mentionUserId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-a97c4290e3bf7600"></a>

## `internal/app/workforce/tasknotification/domain.RecordRecipient`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:479](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L479)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `userId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `role` | integer | integer | [`internal/app/workforce/tasknotification/domain.RecipientRole`](wire-types.md#t-d0a9f86d4f44b977)；Go: `internal/app/workforce/tasknotification/domain.RecipientRole` |
| `personalChannelPolicy` | integer | integer | [`internal/app/workforce/tasknotification/domain.PersonalChannelPolicy`](wire-types.md#t-dac05fbfed121120)；Go: `internal/app/workforce/tasknotification/domain.PersonalChannelPolicy` |
| `inAppRequired` | boolean | boolean | Go: `bool` |

<a id="t-9244ad1bcaec6afb"></a>

## `internal/app/workforce/tasknotification/domain.SourceType`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:25](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L25)

底层类型：`string`。
声明的常量值：`"ARTIFACT_UPLOAD"`, `"CONTINUATION_CHECKPOINT"`, `"DELIVERY_REQUEST"`, `"FINALIZATION_RUN"`, `"INDEPENDENT_REVIEW_RUN"`, `"RESULT_EVALUATION"`, `"REVIEW_OPERATION"`, `"RUNTIME_RECOVERY"`, `"TASK_ATTEMPT"`, `"TASK_CANCEL"`, `"TASK_TIMEOUT"`。
<a id="t-7bf91db006b3fa53"></a>

## `internal/app/workforce/tasknotification/domain.Subscription`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:123](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L123)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `subscriberUserId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `events` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.FactType`](wire-types.md#t-26bedcb14f61a59b)；Go: `[]internal/app/workforce/tasknotification/domain.FactType` |
| `channels` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.ChannelSelection`](wire-types.md#t-5e9beaea2948de73)；Go: `[]internal/app/workforce/tasknotification/domain.ChannelSelection` |
| `enabled` | boolean | boolean | Go: `bool` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `createdBy` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `updatedBy` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `createdAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `updatedAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |

<a id="t-2dbe3fb5c95de1cc"></a>

## `internal/app/workforce/tasknotification/domain.SubscriptionCommand`

[seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go:138](../../../seven-agent-force-server/internal/app/workforce/tasknotification/domain/model.go#L138)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `enabled` | boolean | boolean | Go: `bool` |
| `events` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.FactType`](wire-types.md#t-26bedcb14f61a59b)；Go: `[]internal/app/workforce/tasknotification/domain.FactType` |
| `channels` | array<integer> | array<integer> | [`internal/app/workforce/tasknotification/domain.ChannelSelection`](wire-types.md#t-5e9beaea2948de73)；Go: `[]internal/app/workforce/tasknotification/domain.ChannelSelection` |

<a id="t-b793ce995a2b8fd3"></a>

## `internal/app/workforce/workerregistry/application.Credential`

[seven-agent-force-server/internal/app/workforce/workerregistry/application/service.go:80](../../../seven-agent-force-server/internal/app/workforce/workerregistry/application/service.go#L80)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `token` | string | string | Go: `string` |
| `tokenHint` | string | string | Go: `string` |

<a id="t-52a772e122b0f397"></a>

## `internal/app/workforce/workerregistry/application.RegisterResult`

[seven-agent-force-server/internal/app/workforce/workerregistry/application/service.go:161](../../../seven-agent-force-server/internal/app/workforce/workerregistry/application/service.go#L161)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `registrationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `desiredState` | string | string | Go: `string` |
| `heartbeatIntervalSeconds` | integer | integer | Go: `int` |
| `staleAfterSeconds` | integer | integer | Go: `int` |
| `assignmentLeaseSeconds` | integer | integer | Go: `int` |
| `reconcileRequired` | boolean | boolean | Go: `bool` |
| `workerState` | string | string | Go: `string` |
| `reconciliation` | integer | integer | [`internal/app/workforce/workerregistry/domain.Reconciliation`](wire-types.md#t-753781e520cdac37)；Go: `internal/app/workforce/workerregistry/domain.Reconciliation` |
| `inventoryVersion` | integer | integer | Go: `int` |
| `queue` | integer | integer | [`internal/app/workforce/workerregistry/domain.QueueContract`](wire-types.md#t-8a71731bec37af4d)；Go: `internal/app/workforce/workerregistry/domain.QueueContract` |
| `serverTime` | string | string | Go: `string` |

<a id="t-f447032854da6cec"></a>

## `internal/app/workforce/workerregistry/controller.capacityRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:148](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L148)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-75871c0454c97f97"></a>

## `internal/app/workforce/workerregistry/controller.createRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:63](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L63)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `maxActiveAttempts` | integer | integer | Go: `int` |

<a id="t-75f9438cae55fa76"></a>

## `internal/app/workforce/workerregistry/controller.decommissionRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:188](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L188)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-37e4b010b6048064"></a>

## `internal/app/workforce/workerregistry/controller.deregisterRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:299](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L299)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reason` | string | string | Go: `string` |

<a id="t-07d2399bfaeacd7f"></a>

## `internal/app/workforce/workerregistry/controller.desiredRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:130](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L130)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `desiredState` | string | string | Go: `string` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-1ff25250e69ce4f6"></a>

## `internal/app/workforce/workerregistry/controller.heartbeatRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:251](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L251)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `consumerState` | string | string | Go: `string` |
| `dockerReady` | boolean | boolean | Go: `bool` |
| `activeAttempts` | integer | integer | Go: `int` |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `hostname` | string | string | Go: `string` |
| `reportedIp` | string | string | Go: `string` |
| `primaryInterface` | string | string | Go: `string` |
| `primaryMac` | string | string | Go: `string` |
| `interfaces` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `resourceSnapshot` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-4b5ac93677efc845"></a>

## `internal/app/workforce/workerregistry/controller.inventoryRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:279](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L279)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `inventoryVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `inventoryDigest` | string | string | Go: `string` |
| `images` | array<integer> | array<integer> | [`internal/app/workforce/workerregistry/domain.Image`](wire-types.md#t-7bd66bcc2721dfe0)；Go: `[]internal/app/workforce/workerregistry/domain.Image` |

<a id="t-6b2ff7feac576128"></a>

## `internal/app/workforce/workerregistry/controller.machineEnvelope`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:213](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L213)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |

<a id="t-6bd03c013abd6a55"></a>

## `internal/app/workforce/workerregistry/controller.registerRequest`

[seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go:221](../../../seven-agent-force-server/internal/app/workforce/workerregistry/controller/http.go#L221)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `hostBootId` | string | string | Go: `string` |
| `requestId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `startedAt` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |
| `revision` | string | string | Go: `string` |
| `buildDate` | string | string | Go: `string` |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `maxActiveReviews` | integer | integer | Go: `int` |
| `maxTotalContainers` | integer | integer | Go: `int` |
| `capabilities` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-13121e6a2f024643"></a>

## `internal/app/workforce/workerregistry/domain.Assignment`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:125](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L125)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerInstanceId` | string | string | Go: `string` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionRequirementDigest` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `messageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `createdAt` | string | string | Go: `string` |
| `claimedAt` | string | string | 零值可省略；Go: `string` |
| `completedAt` | string | string | 零值可省略；Go: `string` |

<a id="t-7bd66bcc2721dfe0"></a>

## `internal/app/workforce/workerregistry/domain.Image`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:140](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L140)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `imageDigest` | string | string | Go: `string` |
| `imageRef` | string | string | Go: `string` |
| `role` | string | string | Go: `string` |
| `contractVersion` | string | string | Go: `string` |
| `manifestDigest` | string | string | Go: `string` |
| `manifest` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `capabilities` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `schedulingScope` | string | string | Go: `string` |
| `schedulingPriority` | integer | integer | Go: `int` |
| `environmentCapabilities` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `executionKinds` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `schedulingDigest` | string | string | Go: `string` |
| `admissionStatus` | string | string | Go: `string` |
| `admissionEvidence` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `inventoryVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `presenceStatus` | string | string | Go: `string` |
| `missingSince` | string | string | 零值可省略；Go: `string` |
| `driftReason` | string | string | 零值可省略；Go: `string` |

<a id="t-458e972d0e354721"></a>

## `internal/app/workforce/workerregistry/domain.MetricPoint`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:9](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L9)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `at` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `cpuUsagePercent` | number 或 null | number 或 null | Go: `*float64` |
| `memoryUsagePercent` | number 或 null | number 或 null | Go: `*float64` |
| `workspaceDiskUsagePercent` | number 或 null | number 或 null | Go: `*float64` |
| `activeAttempts` | integer | integer | Go: `int` |
| `maxActiveAttempts` | integer | integer | Go: `int` |

<a id="t-f6b5cf4af9913eb8"></a>

## `internal/app/workforce/workerregistry/domain.MetricSeries`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:18](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L18)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `from` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `to` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | Go: `time.Time` |
| `stepSeconds` | integer | integer | Go: `int` |
| `points` | array<integer> | array<integer> | [`internal/app/workforce/workerregistry/domain.MetricPoint`](wire-types.md#t-458e972d0e354721)；Go: `[]internal/app/workforce/workerregistry/domain.MetricPoint` |

<a id="t-8a71731bec37af4d"></a>

## `internal/app/workforce/workerregistry/domain.QueueContract`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:41](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L41)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `exchange` | string | string | Go: `string` |
| `queue` | string | string | Go: `string` |
| `routingKey` | string | string | Go: `string` |
| `deadQueue` | string | string | Go: `string` |
| `messageSchema` | string | string | Go: `string` |
| `maxPrefetch` | integer | integer | Go: `int` |
| `postprocessExchange` | string | string | Go: `string` |
| `postprocessQueue` | string | string | Go: `string` |
| `postprocessRoutingKey` | string | string | Go: `string` |
| `postprocessDeadQueue` | string | string | Go: `string` |

<a id="t-753781e520cdac37"></a>

## `internal/app/workforce/workerregistry/domain.Reconciliation`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:55](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L55)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `required` | boolean | boolean | Go: `bool` |
| `deadline` | string | string | 零值可省略；Go: `string` |
| `runtimes` | array<integer> | array<integer> | [`internal/app/workforce/workerregistry/domain.RecoverableRuntime`](wire-types.md#t-29cbb5cbf1655254)；Go: `[]internal/app/workforce/workerregistry/domain.RecoverableRuntime` |

<a id="t-29cbb5cbf1655254"></a>

## `internal/app/workforce/workerregistry/domain.RecoverableRuntime`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:61](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L61)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `cancellationRequested` | boolean | boolean | Go: `bool` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionNodeId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `assignmentId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `hostBootId` | string | string | Go: `string` |
| `expectedContainerName` | string | string | Go: `string` |
| `containerId` | string | string | Go: `string` |
| `runtimeStatus` | string | string | Go: `string` |
| `executionIdentityDigest` | string | string | Go: `string` |
| `harnessImageDigest` | string | string | Go: `string` |
| `hookConfigurationDigest` | string | string | Go: `string` |
| `providerBacked` | boolean | boolean | Go: `bool` |
| `providerProxyContainerId` | string | string | 零值可省略；Go: `string` |
| `providerProxyContainerName` | string | string | 零值可省略；Go: `string` |
| `providerProxyImageDigest` | string | string | 零值可省略；Go: `string` |
| `providerProxyExecutionIdentityDigest` | string | string | 零值可省略；Go: `string` |
| `reattachDeadline` | string | string | Go: `string` |

<a id="t-d7835ff39836369f"></a>

## `internal/app/workforce/workerregistry/domain.Worker`

[seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go:84](../../../seven-agent-force-server/internal/app/workforce/workerregistry/domain/model.go#L84)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `desiredState` | string | string | Go: `string` |
| `state` | string | string | Go: `string` |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `reportedMaxActiveAttempts` | integer | integer | Go: `int` |
| `currentInstanceId` | string | string | 零值可省略；Go: `string` |
| `hostBootId` | string | string | 零值可省略；Go: `string` |
| `workerVersion` | string | string | 零值可省略；Go: `string` |
| `workerRevision` | string | string | 零值可省略；Go: `string` |
| `workerBuildDate` | string | string | 零值可省略；Go: `string` |
| `tokenHint` | string | string | Go: `string` |
| `observedRemoteIp` | string | string | 零值可省略；Go: `string` |
| `reportedIp` | string | string | 零值可省略；Go: `string` |
| `hostname` | string | string | 零值可省略；Go: `string` |
| `primaryInterface` | string | string | 零值可省略；Go: `string` |
| `primaryMac` | string | string | 零值可省略；Go: `string` |
| `consumerState` | string | string | 零值可省略；Go: `string` |
| `sessionState` | string | string | 零值可省略；Go: `string` |
| `reconciliationDeadline` | string | string | 零值可省略；Go: `string` |
| `dockerReady` | boolean | boolean | Go: `bool` |
| `activeAttempts` | integer | integer | Go: `int` |
| `reservedAttempts` | integer | integer | Go: `int` |
| `startingAttempts` | integer | integer | Go: `int` |
| `runningAttempts` | integer | integer | Go: `int` |
| `availableAttempts` | integer | integer | Go: `int` |
| `inventoryRefreshGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `inventoryVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `decommissionedAt` | string | string | 零值可省略；Go: `string` |
| `lastHeartbeatAt` | string | string | 零值可省略；Go: `string` |
| `resourceSnapshot` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `interfaces` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | 零值可省略；Go: `encoding/json.RawMessage` |
| `imageCount` | integer | integer | Go: `int` |
| `pendingFinalizations` | integer | integer | Go: `int` |
| `pendingArtifactUploads` | integer | integer | Go: `int` |
| `version` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `createdAt` | string | string | Go: `string` |

<a id="t-13e8fe72e31c31d9"></a>

## `internal/app/workforce/workspaceinput/application.GitPlanView`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:131](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L131)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceProjectId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewBaseSourceRevisionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `sourceRefType` | string | string | 零值可省略；Go: `string` |
| `sourceRefName` | string | string | 零值可省略；Go: `string` |
| `sourceCommit` | string | string | Go: `string` |
| `reviewBaseBranch` | string | string | 零值可省略；Go: `string` |
| `reviewBaseCommit` | string | string | 零值可省略；Go: `string` |
| `resultBranch` | string | string | 零值可省略；Go: `string` |
| `deliveryMode` | string | string | Go: `string` |
| `deliveryAuthorization` | string | string | Go: `string` |
| `fullProjectImport` | boolean | boolean | Go: `bool` |
| `previousDeliveryCommit` | string | string | 零值可省略；Go: `string` |

<a id="t-80cab33cc323b47c"></a>

## `internal/app/workforce/workspaceinput/application.IngestionRun`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:48](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L48)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workspaceSourceRevisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `attemptNumber` | integer | integer | Go: `int` |
| `maxAttempts` | integer | integer | Go: `int` |

<a id="t-016e0a8320d13fcf"></a>

## `internal/app/workforce/workspaceinput/application.ReferenceAsset`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:106](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L106)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `displayName` | string | string | Go: `string` |
| `contentType` | string | string | Go: `string` |
| `contentSha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |
| `status` | string | string | Go: `string` |

<a id="t-21e5ab0031c0bd5a"></a>

## `internal/app/workforce/workspaceinput/application.ReferenceRequest`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:161](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L161)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `fileId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `displayName` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-d795b7ce16e84109"></a>

## `internal/app/workforce/workspaceinput/application.SourceCredential`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:66](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L66)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `Username` | string | string | Go: `string` |
| `Password` | string | string | Go: `string` |
| `PrivateKey` | string | string | Go: `string` |
| `KnownHosts` | string | string | Go: `string` |
| `ExpiresAt` | string | string | Go: `string` |

<a id="t-eca9027d7a67f4c6"></a>

## `internal/app/workforce/workspaceinput/application.SourceIngestionClaim`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:76](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L76)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workspaceSourceRevisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `attemptNumber` | integer | integer | Go: `int` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `leaseToken` | string | string | Go: `string` |
| `leaseExpiresAt` | string | string | Go: `string` |
| `repository` | integer | integer | 零值可省略；[`internal/app/workforce/shared/gittransport.RepositoryTarget`](wire-types.md#t-a3e01d6fa2995af1)；Go: `internal/app/workforce/shared/gittransport.RepositoryTarget` |
| `requestedRefType` | string | string | 零值可省略；Go: `string` |
| `requestedRefName` | string | string | 零值可省略；Go: `string` |
| `frozenCommit` | string | string | 零值可省略；Go: `string` |
| `inputObjectKey` | string | string | 零值可省略；Go: `string` |
| `inputContentSha256` | string | string | 零值可省略；Go: `string` |
| `inputObjectSha256` | string | string | 零值可省略；Go: `string` |
| `inputSizeBytes` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `bundleObjectKey` | string | string | Go: `string` |

<a id="t-39631ebb08f1ced3"></a>

## `internal/app/workforce/workspaceinput/application.SourceIngestionResult`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:95](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L95)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `resolvedCommit` | string | string | Go: `string` |
| `resolvedTree` | string | string | Go: `string` |
| `repositoryBundleContentSha256` | string | string | Go: `string` |
| `repositoryBundleObjectSha256` | string | string | Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | Go: `int64` |

<a id="t-3d5781a76d6bd975"></a>

## `internal/app/workforce/workspaceinput/application.SourceRevision`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:28](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L28)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `kind` | string | string | Go: `string` |
| `projectId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `repositoryConfigDigest` | string | string | 零值可省略；Go: `string` |
| `refType` | string | string | 零值可省略；Go: `string` |
| `refName` | string | string | 零值可省略；Go: `string` |
| `resolvedCommit` | string | string | 零值可省略；Go: `string` |
| `resolvedTree` | string | string | 零值可省略；Go: `string` |
| `repositoryBundleContentSha256` | string | string | 零值可省略；Go: `string` |
| `repositoryBundleObjectSha256` | string | string | 零值可省略；Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | 零值可省略；Go: `int64` |
| `sourceIdentityDigest` | string | string | 零值可省略；Go: `string` |
| `status` | string | string | Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |

<a id="t-83357101ce2099a0"></a>

## `internal/app/workforce/workspaceinput/application.TaskInputView`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:122](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L122)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceKind` | string | string | Go: `string` |
| `workspaceSource` | integer | integer | [`internal/app/workforce/workspaceinput/application.SourceRevision`](wire-types.md#t-3d5781a76d6bd975)；Go: `internal/app/workforce/workspaceinput/application.SourceRevision` |
| `references` | array<integer> | array<integer> | [`internal/app/workforce/workspaceinput/application.ReferenceAsset`](wire-types.md#t-016e0a8320d13fcf)；Go: `[]internal/app/workforce/workspaceinput/application.ReferenceAsset` |
| `inputSnapshotDigest` | string | string | Go: `string` |
| `repositoryBundleSizeBytes` | integer | string(decimal) | Go: `int64` |

<a id="t-204186287995edce"></a>

## `internal/app/workforce/workspaceinput/application.UploadSourceRequest`

[seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go:153](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/application/service.go#L153)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `fileId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceKind` | string | string | Go: `string` |
| `idempotencyKey` | string | string | Go: `string` |

<a id="t-f31c258a6fac6f88"></a>

## `internal/app/workforce/workspaceinput/domain.EmptyWorkspaceIdentity`

[seven-agent-force-server/internal/app/workforce/workspaceinput/domain/empty_workspace.go:18](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/domain/empty_workspace.go#L18)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `contractVersion` | integer | integer | Go: `int` |
| `commit` | string | string | Go: `string` |
| `tree` | string | string | Go: `string` |
| `identityDigest` | string | string | Go: `string` |

<a id="t-b74585a09ab20b9e"></a>

## `internal/app/workforce/workspaceinput/domain.GitPlan`

[seven-agent-force-server/internal/app/workforce/workspaceinput/domain/model.go:50](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/domain/model.go#L50)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `deliveryMode` | string | string | Go: `string` |
| `deliveryAuthorization` | string | string | Go: `string` |
| `targetConfigDigest` | string | string | 零值可省略；Go: `string` |
| `pushAuthorizationDigest` | string | string | 零值可省略；Go: `string` |
| `reviewAuthorizationDigest` | string | string | 零值可省略；Go: `string` |
| `projectId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectVersionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `repositoryConfigDigest` | string | string | 零值可省略；Go: `string` |
| `reviewBaseSourceRevisionId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `reviewBaseBranch` | string | string | 零值可省略；Go: `string` |
| `reviewBaseCommit` | string | string | 零值可省略；Go: `string` |
| `resultBranch` | string | string | 零值可省略；Go: `string` |
| `fullProjectImport` | boolean | boolean | Go: `bool` |

<a id="t-fb0895871c599452"></a>

## `internal/app/workforce/workspaceinput/domain.SourceIngestionMessage`

[seven-agent-force-server/internal/app/workforce/workspaceinput/domain/model.go:82](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/domain/model.go#L82)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `kind` | string | string | Go: `string` |
| `messageId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resourceId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-31745ba0fac22ac8"></a>

## `internal/app/workforce/workspaceinput/domain.TaskInputSnapshot`

[seven-agent-force-server/internal/app/workforce/workspaceinput/domain/model.go:70](../../../seven-agent-force-server/internal/app/workforce/workspaceinput/domain/model.go#L70)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sourceKind` | string | string | Go: `string` |
| `workspaceSourceRevisionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `sourceCommit` | string | string | Go: `string` |
| `sourceTree` | string | string | Go: `string` |
| `sourceIdentityDigest` | string | string | Go: `string` |
| `referenceAssetIds` | integer | integer | [`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `referenceIdentityDigests` | array<string> | array<string> | Go: `[]string` |
| `gitPlan` | integer | integer | [`internal/app/workforce/workspaceinput/domain.GitPlan`](wire-types.md#t-b74585a09ab20b9e)；Go: `internal/app/workforce/workspaceinput/domain.GitPlan` |
| `digest` | string | string | Go: `string` |

<a id="t-71363295fb0f30e7"></a>

## `internal/mcpserver.Capabilities`

[seven-agent-force-server/internal/mcpserver/types.go:20](../../../seven-agent-force-server/internal/mcpserver/types.go#L20)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `applicationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskSubmission` | integer | integer | [`internal/mcpserver.TaskSubmissionCapability`](wire-types.md#t-bcc8bac7d62ca54b)；Go: `internal/mcpserver.TaskSubmissionCapability` |

<a id="t-e5c3532a38ddb868"></a>

## `internal/mcpserver.CreateTaskRequest`

[seven-agent-force-server/internal/mcpserver/types.go:38](../../../seven-agent-force-server/internal/mcpserver/types.go#L38)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `tags` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `referenceAssetIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `additionalCompletionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewEnabled` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `deliveryPreference` | string | string | 零值可省略；Go: `string` |
| `modelSelection` | integer | integer | [`internal/app/workforce/task/facade.ModelSelection`](wire-types.md#t-c395a518b87a2f05)；Go: `internal/app/workforce/task/facade.ModelSelection` |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionTimeoutSeconds` | integer | integer | 零值可省略；Go: `int` |
| `name` | string | string | Go: `string` |
| `instruction` | string | string | Go: `string` |
| `externalCorrelationId` | string | string | Go: `string` |

<a id="t-2665c3e6708cae0a"></a>

## `internal/mcpserver.ModelOption`

[seven-agent-force-server/internal/mcpserver/types.go:128](../../../seven-agent-force-server/internal/mcpserver/types.go#L128)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `credentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `model` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `label` | string | string | Go: `string` |

<a id="t-90d898068cb591bd"></a>

## `internal/mcpserver.ModelOptions`

[seven-agent-force-server/internal/mcpserver/types.go:134](../../../seven-agent-force-server/internal/mcpserver/types.go#L134)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `projectId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `items` | array<integer> | array<integer> | [`internal/mcpserver.ModelOption`](wire-types.md#t-2665c3e6708cae0a)；Go: `[]internal/mcpserver.ModelOption` |

<a id="t-694cfcd74397dbf3"></a>

## `internal/mcpserver.Output`

[seven-agent-force-server/internal/mcpserver/types.go:76](../../../seven-agent-force-server/internal/mcpserver/types.go#L76)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `outputId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `outputKind` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |
| `mediaType` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `links` | integer | integer | 零值可省略；[`internal/mcpserver.OutputLinks`](wire-types.md#t-317fe4e79ffe3511)；Go: `internal/mcpserver.OutputLinks` |

<a id="t-317fe4e79ffe3511"></a>

## `internal/mcpserver.OutputLinks`

[seven-agent-force-server/internal/mcpserver/types.go:87](../../../seven-agent-force-server/internal/mcpserver/types.go#L87)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `download` | string | string | 零值可省略；Go: `string` |

<a id="t-ddd7beba76fb6faf"></a>

## `internal/mcpserver.RuntimeOption`

[seven-agent-force-server/internal/mcpserver/types.go:139](../../../seven-agent-force-server/internal/mcpserver/types.go#L139)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `name` | string | string | Go: `string` |
| `recommended` | boolean | boolean | Go: `bool` |

<a id="t-d2b99925dc49df42"></a>

## `internal/mcpserver.RuntimeOptions`

[seven-agent-force-server/internal/mcpserver/types.go:144](../../../seven-agent-force-server/internal/mcpserver/types.go#L144)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | Go: `string` |
| `projectVersionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `items` | array<integer> | array<integer> | [`internal/mcpserver.RuntimeOption`](wire-types.md#t-ddd7beba76fb6faf)；Go: `[]internal/mcpserver.RuntimeOption` |

<a id="t-430e586a68a040e7"></a>

## `internal/mcpserver.Task`

[seven-agent-force-server/internal/mcpserver/types.go:55](../../../seven-agent-force-server/internal/mcpserver/types.go#L55)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schema` | string | string | 零值可省略；Go: `string` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `projectId` | string(decimal) | string(decimal) | 零值可省略；JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `replayed` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `externalCorrelationId` | string | string | 零值可省略；Go: `string` |
| `reasonCode` | string | string | 零值可省略；Go: `string` |
| `resultSummary` | string | string | 零值可省略；Go: `string` |
| `createdAt` | string | string | 零值可省略；Go: `string` |
| `startedAt` | string | string | 零值可省略；Go: `string` |
| `finishedAt` | string | string | 零值可省略；Go: `string` |
| `links` | integer | integer | 零值可省略；[`internal/mcpserver.TaskLinks`](wire-types.md#t-b740635148cf2e4a)；Go: `internal/mcpserver.TaskLinks` |

<a id="t-b740635148cf2e4a"></a>

## `internal/mcpserver.TaskLinks`

[seven-agent-force-server/internal/mcpserver/types.go:71](../../../seven-agent-force-server/internal/mcpserver/types.go#L71)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `operator` | string | string | 零值可省略；Go: `string` |
| `self` | string | string | 零值可省略；Go: `string` |

<a id="t-bcc8bac7d62ca54b"></a>

## `internal/mcpserver.TaskSubmissionCapability`

[seven-agent-force-server/internal/mcpserver/types.go:26](../../../seven-agent-force-server/internal/mcpserver/types.go#L26)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `supportsSelectionDiscovery` | boolean | boolean | Go: `bool` |
| `requiresModelSelection` | boolean | boolean | Go: `bool` |
| `requiresRuntimeReleaseId` | boolean | boolean | Go: `bool` |
| `requiresProjectId` | boolean | boolean | Go: `bool` |
| `supportsReferenceAssets` | boolean | boolean | Go: `bool` |
| `supportsAdditionalCompletionCheck` | boolean | boolean | Go: `bool` |
| `supportsReviewToggle` | boolean | boolean | Go: `bool` |
| `supportsDeliveryPreference` | boolean | boolean | Go: `bool` |
| `supportsOutputs` | boolean | boolean | Go: `bool` |

<a id="t-77d081eb69f4687e"></a>

## `internal/mcpserver.createTaskInput`

[seven-agent-force-server/internal/mcpserver/tools.go:40](../../../seven-agent-force-server/internal/mcpserver/tools.go#L40)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `tags` | array<string> | array<string> | 零值可省略；Go: `[]string` |
| `referenceAssetIds` | integer | integer | 零值可省略；[`internal/app/workforce/shared/transport.Int64Slice`](wire-types.md#t-aa20383d22defc63)；Go: `internal/app/workforce/shared/transport.Int64Slice` |
| `additionalCompletionCheck` | integer 或 null | integer 或 null | 零值可省略；[`internal/app/workforce/completioncheck/domain.Input`](wire-types.md#t-d1b1494300121a97)；Go: `*internal/app/workforce/completioncheck/facade.Input` |
| `reviewEnabled` | boolean 或 null | boolean 或 null | 零值可省略；Go: `*bool` |
| `deliveryPreference` | string | string | 零值可省略；Go: `string` |
| `modelSelection` | integer | integer | [`internal/app/workforce/task/facade.ModelSelection`](wire-types.md#t-c395a518b87a2f05)；Go: `internal/app/workforce/task/facade.ModelSelection` |
| `runtimeReleaseId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `executionTimeoutSeconds` | integer | integer | 零值可省略；Go: `int` |
| `idempotencyKey` | string | string | Go: `string` |
| `projectId` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `instruction` | string | string | Go: `string` |
| `externalCorrelationId` | string | string | Go: `string` |

<a id="t-7c1211a38cd9b147"></a>

## `internal/mcpserver.frameworkEnvelope`

[seven-agent-force-server/internal/mcpserver/gateway_http.go:75](../../../seven-agent-force-server/internal/mcpserver/gateway_http.go#L75)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `code` | integer | integer | Go: `int` |
| `data` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `message` | string | string | Go: `string` |

<a id="t-4064316b3372c93c"></a>

## `internal/mcpserver.listOutputsOutput`

[seven-agent-force-server/internal/mcpserver/tools.go:66](../../../seven-agent-force-server/internal/mcpserver/tools.go#L66)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string | string | Go: `string` |
| `outputs` | array<integer> | array<integer> | [`internal/mcpserver.Output`](wire-types.md#t-694cfcd74397dbf3)；Go: `[]internal/mcpserver.Output` |

<a id="t-7b42b59aacc5828a"></a>

## `internal/mcpserver.modelOptionsInput`

[seven-agent-force-server/internal/mcpserver/tools.go:90](../../../seven-agent-force-server/internal/mcpserver/tools.go#L90)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `projectId` | string | string | Go: `string` |

<a id="t-be9f91088b2c8389"></a>

## `internal/mcpserver.readOutputInput`

[seven-agent-force-server/internal/mcpserver/tools.go:61](../../../seven-agent-force-server/internal/mcpserver/tools.go#L61)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string | string | Go: `string` |
| `outputId` | string | string | Go: `string` |

<a id="t-f399bd89c9960c3a"></a>

## `internal/mcpserver.runtimeOptionsInput`

[seven-agent-force-server/internal/mcpserver/tools.go:93](../../../seven-agent-force-server/internal/mcpserver/tools.go#L93)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `projectId` | string | string | Go: `string` |
| `modelCredentialId` | string | string | Go: `string` |

<a id="t-9feb4a22c85451b7"></a>

## `internal/mcpserver.taskIDInput`

[seven-agent-force-server/internal/mcpserver/tools.go:57](../../../seven-agent-force-server/internal/mcpserver/tools.go#L57)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string | string | Go: `string` |

<a id="t-af0908b303b3a808"></a>

## `internal/mcpserver.toolError`

[seven-agent-force-server/internal/mcpserver/tools.go:84](../../../seven-agent-force-server/internal/mcpserver/tools.go#L84)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `code` | string | string | Go: `string` |
| `message` | string | string | Go: `string` |
| `retryable` | boolean | boolean | Go: `bool` |

<a id="t-e0cdcf44ff3a23f7"></a>

## `internal/mcpserver.toolErrorView`

[seven-agent-force-server/internal/mcpserver/tools.go:80](../../../seven-agent-force-server/internal/mcpserver/tools.go#L80)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `error` | integer | integer | [`internal/mcpserver.toolError`](wire-types.md#t-af0908b303b3a808)；Go: `internal/mcpserver.toolError` |

<a id="t-56a5232a7f60c494"></a>

## `internal/mcpserver.verifiedOutput`

[seven-agent-force-server/internal/mcpserver/tools.go:71](../../../seven-agent-force-server/internal/mcpserver/tools.go#L71)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `outputId` | string | string | Go: `string` |
| `name` | string | string | Go: `string` |
| `mediaType` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | string | string | Go: `string` |
| `verified` | boolean | boolean | Go: `bool` |

<a id="t-fcf4c64e306f0ca2"></a>

## `internal/worker/agentprotocol.Answer`

[seven-agent-force-server/internal/worker/agentprotocol/interaction.go:32](../../../seven-agent-force-server/internal/worker/agentprotocol/interaction.go#L32)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `questionKey` | string | string | Go: `string` |
| `selectedOptionKeys` | array<string> | array<string> | Go: `[]string` |
| `text` | string | string | Go: `string` |

<a id="t-d91b3e5da1bcd10d"></a>

## `internal/worker/agentprotocol.Descriptor`

[seven-agent-force-server/internal/worker/agentprotocol/manifest.go:16](../../../seven-agent-force-server/internal/worker/agentprotocol/manifest.go#L16)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `id` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |
| `executable` | string | string | Go: `string` |
| `args` | array<string> | array<string> | Go: `[]string` |
| `binarySha256` | string | string | Go: `string` |
| `sourceSha256` | string | string | Go: `string` |
| `protocol` | object（见 Go 类型或匿名结构索引） | object（见 Go 类型或匿名结构索引） | [`inline.881abaa51f03`](wire-types.md#t-a4d013714b0f75d1)；Go: `struct{Name string "json:\"name\""; Min int "json:\"min\""; Max int "json:\"max\""}` |
| `transport` | string | string | Go: `string` |
| `capabilities` | array<string> | array<string> | Go: `[]string` |

<a id="t-d24d5e7adfa2aa26"></a>

## `internal/worker/agentprotocol.Frame`

[seven-agent-force-server/internal/worker/agentprotocol/codec.go:34](../../../seven-agent-force-server/internal/worker/agentprotocol/codec.go#L34)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `protocol` | string | string | Go: `string` |
| `method` | string | string | Go: `string` |
| `key` | string | string | Go: `string` |
| `replyTo` | string | string | 零值可省略；Go: `string` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-33bb83cc546c6027"></a>

## `internal/worker/agentprotocol.GitDeliveryArguments`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:132](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L132)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `summary` | string | string | Go: `string` |

<a id="t-d14aff88b8a22611"></a>

## `internal/worker/agentprotocol.InteractionAnswer`

[seven-agent-force-server/internal/worker/agentprotocol/interaction.go:37](../../../seven-agent-force-server/internal/worker/agentprotocol/interaction.go#L37)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `responseId` | string | string | Go: `string` |
| `answers` | array<integer> | array<integer> | [`internal/worker/agentprotocol.Answer`](wire-types.md#t-fcf4c64e306f0ca2)；Go: `[]internal/worker/agentprotocol.Answer` |

<a id="t-80da06bbb01ca8b8"></a>

## `internal/worker/agentprotocol.NativeResult`

[seven-agent-force-server/internal/worker/agentprotocol/process.go:53](../../../seven-agent-force-server/internal/worker/agentprotocol/process.go#L53)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `outcome` | string | string | Go: `string` |
| `summary` | string | string | Go: `string` |
| `sessionReference` | string | string | Go: `string` |

<a id="t-053887636e577d5a"></a>

## `internal/worker/agentprotocol.NetworkCommandArguments`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:125](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L125)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `purpose` | string | string | Go: `string` |
| `command` | string | string | Go: `string` |
| `cwd` | string | string | Go: `string` |
| `targets` | array<integer> | array<integer> | [`internal/worker/agentprotocol.ToolTarget`](wire-types.md#t-2d3f2704bc315e56)；Go: `[]internal/worker/agentprotocol.ToolTarget` |

<a id="t-10a8dcd2333cb078"></a>

## `internal/worker/agentprotocol.Question`

[seven-agent-force-server/internal/worker/agentprotocol/interaction.go:17](../../../seven-agent-force-server/internal/worker/agentprotocol/interaction.go#L17)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `key` | string | string | Go: `string` |
| `title` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `kind` | string | string | Go: `string` |
| `required` | boolean | boolean | Go: `bool` |
| `options` | array<integer> | array<integer> | [`internal/worker/agentprotocol.QuestionOption`](wire-types.md#t-cfbc2390a29a0738)；Go: `[]internal/worker/agentprotocol.QuestionOption` |
| `allowFreeText` | boolean | boolean | Go: `bool` |

<a id="t-cfbc2390a29a0738"></a>

## `internal/worker/agentprotocol.QuestionOption`

[seven-agent-force-server/internal/worker/agentprotocol/interaction.go:13](../../../seven-agent-force-server/internal/worker/agentprotocol/interaction.go#L13)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `key` | string | string | Go: `string` |
| `label` | string | string | Go: `string` |

<a id="t-ce23f8df4dac6f0c"></a>

## `internal/worker/agentprotocol.QuestionRequest`

[seven-agent-force-server/internal/worker/agentprotocol/interaction.go:26](../../../seven-agent-force-server/internal/worker/agentprotocol/interaction.go#L26)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `interactionKey` | string | string | Go: `string` |
| `questions` | array<integer> | array<integer> | [`internal/worker/agentprotocol.Question`](wire-types.md#t-10a8dcd2333cb078)；Go: `[]internal/worker/agentprotocol.Question` |
| `isBlocking` | boolean | boolean | Go: `bool` |

<a id="t-d0c093d94cb7b7ee"></a>

## `internal/worker/agentprotocol.ToolCatalogPayload`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:49](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L49)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `toolNames` | array<string> | array<string> | Go: `[]string` |
| `catalogDigest` | string | string | Go: `string` |

<a id="t-7befa62321d6299b"></a>

## `internal/worker/agentprotocol.ToolClosedPayload`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:165](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L165)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `reason` | string | string | Go: `string` |

<a id="t-90b6a8b0a4403eee"></a>

## `internal/worker/agentprotocol.ToolDefinition`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:43](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L43)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `description` | string | string | Go: `string` |
| `inputSchema` | object<string, 任意 JSON 值> | object<string, 任意 JSON 值> | Go: `map[string]any` |

<a id="t-01d1ae55d45642aa"></a>

## `internal/worker/agentprotocol.ToolReceiptPayload`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:158](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L158)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |

<a id="t-85fdfa303a772fac"></a>

## `internal/worker/agentprotocol.ToolRequestPayload`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:136](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L136)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `toolName` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `nativeToolCallId` | string | string | Go: `string` |
| `roundKey` | string | string | Go: `string` |
| `arguments` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `argumentsDigest` | string | string | Go: `string` |

<a id="t-cfc84c0f97e87dcc"></a>

## `internal/worker/agentprotocol.ToolResultPayload`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:145](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L145)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `localToolCallKey` | string | string | Go: `string` |
| `nativeToolCallId` | string | string | Go: `string` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `status` | string | string | Go: `string` |
| `exitCode` | integer 或 null | integer 或 null | Go: `*int` |
| `summary` | string | string | Go: `string` |
| `output` | string | string | Go: `string` |
| `resultDigest` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-2d3f2704bc315e56"></a>

## `internal/worker/agentprotocol.ToolTarget`

[seven-agent-force-server/internal/worker/agentprotocol/tool.go:37](../../../seven-agent-force-server/internal/worker/agentprotocol/tool.go#L37)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `host` | string | string | Go: `string` |
| `protocol` | string | string | Go: `string` |
| `port` | integer | integer | Go: `int` |

<a id="t-40f6f165edbcb421"></a>

## `internal/worker/agentprotocol.UserMessage`

[seven-agent-force-server/internal/worker/agentprotocol/interaction.go:43](../../../seven-agent-force-server/internal/worker/agentprotocol/interaction.go#L43)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `roundKey` | string | string | Go: `string` |
| `messageKey` | string | string | Go: `string` |
| `text` | string | string | Go: `string` |

<a id="t-881f50a14f016af2"></a>

## `internal/worker/checkpoint.Blob`

[seven-agent-force-server/internal/worker/checkpoint/interaction.go:37](../../../seven-agent-force-server/internal/worker/checkpoint/interaction.go#L37)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | integer | string(decimal) | Go: `int64` |

<a id="t-f82b130a8108fec8"></a>

## `internal/worker/checkpoint.ExecutionBudget`

[seven-agent-force-server/internal/worker/checkpoint/budget.go:7](../../../seven-agent-force-server/internal/worker/checkpoint/budget.go#L7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `version` | integer | integer | Go: `int` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `activeLimitMillis` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `activeElapsedMillis` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionWaitingMillis` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `absoluteDeadlineUnixMillis` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-59e9b969a09a01e6"></a>

## `internal/worker/checkpoint.Manifest`

[seven-agent-force-server/internal/worker/checkpoint/interaction.go:41](../../../seven-agent-force-server/internal/worker/checkpoint/interaction.go#L41)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `format` | string | string | Go: `string` |
| `purpose` | string | string | Go: `string` |
| `decision` | string | string | Go: `string` |
| `checkpointIdentity` | string | string | Go: `string` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `interactionId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `operationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `adapterSessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `automaticRevisionCount` | integer | integer | Go: `int` |
| `sessionDigest` | string | string | Go: `string` |
| `taskExecutionSnapshotDigest` | string | string | Go: `string` |
| `baseCommit` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `selectionDigest` | string | string | Go: `string` |
| `repositoryBundle` | integer | integer | [`internal/worker/checkpoint.Blob`](wire-types.md#t-881f50a14f016af2)；Go: `internal/worker/checkpoint.Blob` |
| `workspaceOverlay` | integer | integer | [`internal/worker/checkpoint.Blob`](wire-types.md#t-881f50a14f016af2)；Go: `internal/worker/checkpoint.Blob` |
| `session` | integer | integer | [`internal/worker/checkpoint.Blob`](wire-types.md#t-881f50a14f016af2)；Go: `internal/worker/checkpoint.Blob` |
| `executionBudget` | integer | integer | [`internal/worker/checkpoint.ExecutionBudget`](wire-types.md#t-f82b130a8108fec8)；Go: `internal/worker/checkpoint.ExecutionBudget` |
| `currentRound` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `suspendControlKeySha256` | string | string | Go: `string` |

<a id="t-3591d56911635dbc"></a>

## `internal/worker/checkpoint.RestoreInput`

[seven-agent-force-server/internal/worker/checkpoint/restore.go:20](../../../seven-agent-force-server/internal/worker/checkpoint/restore.go#L20)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `manifest` | integer | integer | [`internal/worker/checkpoint.Manifest`](wire-types.md#t-59e9b969a09a01e6)；Go: `internal/worker/checkpoint.Manifest` |
| `interactionResponse` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `questionRequest` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `executionBudget` | integer | integer | [`internal/worker/checkpoint.ExecutionBudget`](wire-types.md#t-f82b130a8108fec8)；Go: `internal/worker/checkpoint.ExecutionBudget` |

<a id="t-a834afa641aa7cce"></a>

## `internal/worker/control.CheckInput`

[seven-agent-force-server/internal/worker/control/client.go:483](../../../seven-agent-force-server/internal/worker/control/client.go#L483)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `ordinal` | integer | integer | Go: `int` |
| `name` | string | string | Go: `string` |
| `command` | string | string | Go: `string` |
| `exitCode` | integer | integer | Go: `int` |
| `output` | string | string | Go: `string` |
| `status` | string | string | Go: `string` |

<a id="t-9849f8affb9248be"></a>

## `internal/worker/control.HeartbeatInput`

[seven-agent-force-server/internal/worker/control/client.go:103](../../../seven-agent-force-server/internal/worker/control/client.go#L103)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `consumerState` | string | string | Go: `string` |
| `dockerReady` | boolean | boolean | Go: `bool` |
| `activeAttempts` | integer | integer | Go: `int` |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `hostname` | string | string | Go: `string` |
| `reportedIp` | string | string | Go: `string` |
| `primaryInterface` | string | string | Go: `string` |
| `primaryMac` | string | string | Go: `string` |
| `interfaces` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `resourceSnapshot` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-da1d69a3e051f55d"></a>

## `internal/worker/control.HeartbeatResult`

[seven-agent-force-server/internal/worker/control/client.go:117](../../../seven-agent-force-server/internal/worker/control/client.go#L117)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `desiredState` | string | string | Go: `string` |
| `derivedState` | string | string | Go: `string` |
| `consumerDirective` | string | string | Go: `string` |
| `refreshInventoryGeneration` | integer | integer | Go: `int` |

<a id="t-05b9a19422596c51"></a>

## `internal/worker/control.InteractionBlob`

[seven-agent-force-server/internal/worker/control/interaction_suspend.go:34](../../../seven-agent-force-server/internal/worker/control/interaction_suspend.go#L34)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `objectKey` | string | string | Go: `string` |
| `sha256` | string | string | Go: `string` |
| `sizeBytes` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-a6f3caee63b4d206"></a>

## `internal/worker/control.InteractionPublication`

[seven-agent-force-server/internal/worker/control/interaction.go:14](../../../seven-agent-force-server/internal/worker/control/interaction.go#L14)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `receipt` | integer | integer | [`internal/app/workforce/interaction/facade.Receipt`](wire-types.md#t-72ef9cc8b517430e)；Go: `internal/app/workforce/interaction/facade.Receipt` |
| `operation` | integer | integer | 零值可省略；[`internal/app/workforce/executionaccess/facade.Receipt`](wire-types.md#t-ea2d168ef71fda6e)；Go: `internal/app/workforce/executionaccess/facade.Receipt` |
| `deliveryIntent` | integer | integer | 零值可省略；[`internal/app/workforce/delivery/facade.IntentReceipt`](wire-types.md#t-c029d6a1620fc30f)；Go: `internal/app/workforce/delivery/facade.IntentReceipt` |
| `immediateToolResult` | integer | integer | 零值可省略；[`internal/app/workforce/delivery/facade.ImmediateToolResult`](wire-types.md#t-510c99b41bf507eb)；Go: `internal/app/workforce/delivery/facade.ImmediateToolResult` |
| `frameSha256` | string | string | Go: `string` |

<a id="t-3bb0a8037b3f13f4"></a>

## `internal/worker/control.LeaseRenewal`

[seven-agent-force-server/internal/worker/control/client.go:317](../../../seven-agent-force-server/internal/worker/control/client.go#L317)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `expiresAt` | string | string | Go: `string` |
| `receiptDigest` | string | string | Go: `string` |

<a id="t-d41213dc3a010a86"></a>

## `internal/worker/control.Outcome`

[seven-agent-force-server/internal/worker/control/client.go:526](../../../seven-agent-force-server/internal/worker/control/client.go#L526)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `outcome` | string | string | Go: `string` |
| `reasonCode` | string | string | Go: `string` |
| `message` | string | string | Go: `string` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `summary` | string | string | Go: `string` |

<a id="t-06313444ba34c684"></a>

## `internal/worker/control.RegisterInput`

[seven-agent-force-server/internal/worker/control/client.go:59](../../../seven-agent-force-server/internal/worker/control/client.go#L59)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `startedAt` | string | string | Go: `string` |
| `version` | string | string | Go: `string` |
| `revision` | string | string | Go: `string` |
| `buildDate` | string | string | Go: `string` |
| `maxActiveAttempts` | integer | integer | Go: `int` |
| `maxActiveReviews` | integer | integer | Go: `int` |
| `maxTotalContainers` | integer | integer | Go: `int` |
| `capabilities` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |

<a id="t-c8099c20f44e2fac"></a>

## `internal/worker/control.RegisterResult`

[seven-agent-force-server/internal/worker/control/client.go:70](../../../seven-agent-force-server/internal/worker/control/client.go#L70)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `registrationId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `workerId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `instanceId` | string | string | Go: `string` |
| `desiredState` | string | string | Go: `string` |
| `heartbeatIntervalSeconds` | integer | integer | Go: `int` |
| `staleAfterSeconds` | integer | integer | Go: `int` |
| `assignmentLeaseSeconds` | integer | integer | Go: `int` |
| `reconcileRequired` | boolean | boolean | Go: `bool` |
| `workerState` | string | string | Go: `string` |
| `reconciliation` | integer | integer | [`internal/app/workforce/workerregistry/domain.Reconciliation`](wire-types.md#t-753781e520cdac37)；Go: `internal/app/workforce/workerregistry/domain.Reconciliation` |
| `inventoryVersion` | integer | integer | Go: `int` |
| `queue` | integer | integer | [`internal/app/workforce/workerregistry/domain.QueueContract`](wire-types.md#t-8a71731bec37af4d)；Go: `internal/app/workforce/workerregistry/domain.QueueContract` |

<a id="t-65c378b5e5c7e4fe"></a>

## `internal/worker/control.ReviewClaim`

[seven-agent-force-server/internal/worker/control/client.go:135](../../../seven-agent-force-server/internal/worker/control/client.go#L135)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `reviewRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reviewRunAttemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `tenantId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `taskTurnId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `finalizationRunId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `resultCommit` | string | string | Go: `string` |
| `resultTree` | string | string | Go: `string` |
| `reviewerImageDigest` | string | string | Go: `string` |
| `modelCredentialId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `credentialVersion` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `modelProtocol` | string | string | Go: `string` |
| `primaryModel` | string | string | Go: `string` |
| `providerStructuredOutputContract` | string | string | Go: `string` |
| `providerProxyImageDigest` | string | string | 零值可省略；Go: `string` |
| `systemContract` | string | string | Go: `string` |
| `reportSchemaVersion` | string | string | Go: `string` |
| `hardTimeoutSeconds` | integer | integer | Go: `int` |
| `reviewContract` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `requirements` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `reviewLeaseToken` | string | string | Go: `string` |
| `reviewLeaseExpiresAt` | string | string | Go: `string` |
| `executionDeadlineAt` | string | string | Go: `string` |

<a id="t-633832d1c1537ca0"></a>

## `internal/worker/control.ReviewLaunchPermit`

[seven-agent-force-server/internal/worker/control/client.go:162](../../../seven-agent-force-server/internal/worker/control/client.go#L162)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `permitId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `permitToken` | string | string | Go: `string` |
| `expiresAt` | string | string | Go: `string` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-d5edca2e59f4a3cf"></a>

## `internal/worker/control.ReviewReportResult`

[seven-agent-force-server/internal/worker/control/client.go:169](../../../seven-agent-force-server/internal/worker/control/client.go#L169)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `status` | string | string | Go: `string` |
| `verdict` | string | string | Go: `string` |
| `reportDigest` | string | string | Go: `string` |

<a id="t-04ce0702e9e1dbc1"></a>

## `internal/worker/control.WorkspaceGCEligibility`

[seven-agent-force-server/internal/worker/control/client.go:126](../../../seven-agent-force-server/internal/worker/control/client.go#L126)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `terminal` | boolean | boolean | Go: `bool` |
| `terminalAt` | string(RFC3339/RFC3339Nano) | string(RFC3339/RFC3339Nano) | 零值可省略；Go: `time.Time` |
| `activeReferences` | boolean | boolean | Go: `bool` |
| `portableStateAvailable` | boolean | boolean | Go: `bool` |
| `reasonCode` | string | string | Go: `string` |

<a id="t-49d89421311635e2"></a>

## `internal/worker/control.resultEnvelope`

[seven-agent-force-server/internal/worker/control/client.go:685](../../../seven-agent-force-server/internal/worker/control/client.go#L685)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `code` | integer | integer | Go: `int` |
| `data` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
| `message` | string | string | Go: `string` |

<a id="t-e16cb163a9a1d389"></a>

## `internal/worker/hostinfo.Interface`

[seven-agent-force-server/internal/worker/hostinfo/hostinfo.go:25](../../../seven-agent-force-server/internal/worker/hostinfo/hostinfo.go#L25)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `name` | string | string | Go: `string` |
| `addresses` | array<string> | array<string> | Go: `[]string` |
| `mac` | string | string | 零值可省略；Go: `string` |

<a id="t-b3ea5dabe4146b23"></a>

## `internal/worker/hostinfo.Resources`

[seven-agent-force-server/internal/worker/hostinfo/hostinfo.go:31](../../../seven-agent-force-server/internal/worker/hostinfo/hostinfo.go#L31)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `logicalCpuCores` | integer | integer | Go: `int` |
| `cpuUsagePercent` | number | number | Go: `float64` |
| `cpuUsageAvailable` | boolean | boolean | Go: `bool` |
| `load1` | number | number | Go: `float64` |
| `memoryTotalBytes` | integer | string(decimal) | Go: `uint64` |
| `memoryUsedBytes` | integer | string(decimal) | Go: `uint64` |
| `memoryAvailableBytes` | integer | string(decimal) | Go: `uint64` |
| `memoryUsagePercent` | number | number | Go: `float64` |
| `workspaceDiskTotalBytes` | integer | string(decimal) | Go: `uint64` |
| `workspaceDiskUsedBytes` | integer | string(decimal) | Go: `uint64` |
| `workspaceDiskAvailableBytes` | integer | string(decimal) | Go: `uint64` |
| `workspaceDiskUsagePercent` | number | number | Go: `float64` |

<a id="t-077d315815713470"></a>

## `internal/worker/interactionspool.Entry`

[seven-agent-force-server/internal/worker/interactionspool/spool.go:53](../../../seven-agent-force-server/internal/worker/interactionspool/spool.go#L53)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `receipt` | integer | integer | [`internal/worker/interactionspool.Receipt`](wire-types.md#t-fb6cde0cacbe79c1)；Go: `internal/worker/interactionspool.Receipt` |
| `frame` | string(base64) | string(base64) | Go: `[]byte` |

<a id="t-ef9b08a98f657bfb"></a>

## `internal/worker/interactionspool.Identity`

[seven-agent-force-server/internal/worker/interactionspool/spool.go:36](../../../seven-agent-force-server/internal/worker/interactionspool/spool.go#L36)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `taskId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `attemptId` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `generation` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `ownerEpoch` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `currentRound` | string | string | Go: `string` |
| `sessionGeneration` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |

<a id="t-242509bb98b207ce"></a>

## `internal/worker/interactionspool.Key`

[seven-agent-force-server/internal/worker/interactionspool/spool.go:45](../../../seven-agent-force-server/internal/worker/interactionspool/spool.go#L45)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `identity` | integer | integer | [`internal/worker/interactionspool.Identity`](wire-types.md#t-ef9b08a98f657bfb)；Go: `internal/worker/interactionspool.Identity` |
| `occurrence` | string | string | Go: `string` |

<a id="t-fb6cde0cacbe79c1"></a>

## `internal/worker/interactionspool.Receipt`

[seven-agent-force-server/internal/worker/interactionspool/spool.go:49](../../../seven-agent-force-server/internal/worker/interactionspool/spool.go#L49)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `key` | integer | integer | [`internal/worker/interactionspool.Key`](wire-types.md#t-242509bb98b207ce)；Go: `internal/worker/interactionspool.Key` |
| `sha256` | string | string | Go: `string` |

<a id="t-7c41880a71ca27cf"></a>

## `internal/worker/supervisorwire.JournalEntry`

[seven-agent-force-server/internal/worker/supervisorwire/journal.go:7](../../../seven-agent-force-server/internal/worker/supervisorwire/journal.go#L7)

| 字段 | 请求／原生 JSON 类型 | HTTP `data` 类型 | 编码与嵌套类型 |
| --- | --- | --- | --- |
| `schemaVersion` | integer | integer | Go: `int` |
| `sequence` | string(decimal) | string(decimal) | JSON 引号编码；Go: `int64` |
| `factId` | string | string | Go: `string` |
| `type` | string | string | Go: `string` |
| `time` | string | string | Go: `string` |
| `payloadDigest` | string | string | Go: `string` |
| `payload` | `encoding/json.RawMessage` | `encoding/json.RawMessage` | Go: `encoding/json.RawMessage` |
