# Seven Agent Force architecture and trust contracts

## Components and ownership

| Component | Responsibility |
| --- | --- |
| Server | Authenticated tenant scope, task state, scheduling authority, review, result acceptance and delivery authorization |
| Web | Operator interaction, safe state projection and authorized user inputs |
| Worker | Host execution, image discovery and admission checks, fenced ownership, capacity and resource journals |
| Supervisor | Protected image entrypoint, Adapter protocol, process control, checkpoint and result-processor coordination |
| Agent Adapter | Native CLI mapping, session identity, progress, input delivery and process cleanup |
| Result Processor | Evaluate the frozen task outputs and propose a bounded result contract |
| Finalizer | Independently recompute result identity, files and hashes outside Harness trust |

The active Go module and Web console include the required foundation services. Separate reference checkouts are not required. See [secondary development](../development.md), [SDK development](../sdk-development.md) and the [protocol index](protocols/README.md).

## Identity and state authority

Internal entity and relation IDs are BIGINT/int64; JSON, URLs and queues encode them as decimal strings. Tenant identity comes from authenticated organization scope. Worker and Attempt credentials cannot be substituted with Application API Keys or browser user tokens.

Task, TaskTurn, Attempt, ReviewRun and DeliveryAttempt have independent lifecycle authority. Agent output is observation. CompletionProposal is not Task success; a passing Processor result still requires platform validation. A successful task does not imply Git delivery succeeded. Frozen Runtime, input and native-session identity must be preserved for existing executions.

## Execution boundary

The Control Plane does not expose a Docker socket through API handlers or Attempt containers. Attempts expose no control HTTP or gRPC listener. Worker-to-Supervisor control uses private Unix-domain communication with bounded, correlated commands and receipts. Same-host reattach and cold restore require the corresponding trusted session, generation, owner epoch and checkpoint evidence; resource existence alone never establishes adopt authority.

Worker image discovery reads the supported local image envelope. An arbitrary discovered Docker image is not automatically an eligible Runtime: admission, image/protocol compatibility, published immutable Runtime identity and Worker capability govern scheduling. Production uses immutable image references. See [Worker](protocols/worker-amqp.md), [Supervisor](protocols/supervisor.md) and [Adapter](protocols/agent-adapter.md).

## Adapter and input lifecycle

An image owns its protected Adapter executable and arguments; repository/task input cannot replace the launcher. The Adapter reports actual native process start and retains native session reference for resume. Capability negotiation limits questions, messages, tools, suspension and receipts.

Question response identity, round correlation, duplicate handling, cancellation, closure and late-input isolation are protocol concerns. Persisted input is not evidence of native consumption; receipt and saved/restored state remain separate facts. A cold restored answer must match its original complete question and trusted response. See [interaction contract](../../deploy/workforce/runtime-sdk/INTERACTION.md) and [interface reference](agent-adapter-interface-reference.md).

## Result, review and delivery

The task-frozen Processor evaluates outputs and may propose PASS, REVISE, WAIT_USER, REJECT or MANUAL_REVIEW. Proposed paths, hashes, output contracts and bounded actions remain untrusted until independently validated. Finalizer does not trust files merely because a Harness declared them complete.

Packaging, Review and Delivery retries must not rerun the Agent. Checkpoint continuation uses its authorized lifecycle; it is not a generic replacement for post-processing retry. Git push and remote request creation belong to platform-authorized DeliveryAttempt handling. Unknown remote outcomes require readback before another side effect. See [Processor](protocols/result-processor.md), [Git delivery](protocols/git-delivery.md) and [delivery operations](authorized-git-delivery-runbook.md).

## Provider and external access

Open REST and MCP use Application identity scoped by tenant and application. Provider gateway credentials remain lease- and execution-scoped; raw model/provider payloads must not become operator logs or error bodies. Runtime-reported tool names do not grant network or Git permission. See [Open REST](protocols/open-rest.md), [MCP](protocols/mcp.md), [Provider Gateway](protocols/provider-gateway.md) and [controlled network](controlled-network-operations.md).

## Extension and verification

For each change identify producer, consumer, validation, defaults, configuration and authorization. Update interface references alongside affected code. Test pure contracts locally, database semantics against disposable integration databases and visible behavior through actual pages. Synthetic tests, image execution, real Provider calls and deployment qualify different boundaries; report each separately.
