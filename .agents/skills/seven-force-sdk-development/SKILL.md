---
name: seven-force-sdk-development
description: Develop Seven Agent Force Python Runtime SDK, Agent Adapters or Result Processors, including local examples, packaging, protocol compatibility and Python/Go consumer tests. Use for SDK usage and contributor changes, not external Open REST client development.
---

# Seven Agent Force SDK and Adapter development

Read root AGENTS.md and [SDK development guide](../../../docs/sdk-development.md). Decide whether the request concerns an SDK user writing an Adapter/Processor or a contributor changing SDK/protocol behavior. Do not assume a public PyPI release. Canonical import is seven_agent.force; preserve declared legacy compatibility aliases unless removal is explicitly requested.

## Ground the implementation

Start at deploy/workforce/runtime-sdk/src/seven_agent/force, protocol/, tests/ and pyproject.toml. Check the actual CLI --help before giving packaging commands. Read docs/workforce/agent-adapter-interface-reference.md for payload, ordering and platform requirements; method signatures alone do not prove business compliance. Read Server internal/worker/agentprotocol for the Go consumer and locate Supervisor handling before extending a method.

For a local example use docs/examples/sdk/local_adapter.py: it performs no Provider, Docker, Git or platform API calls. inspect/dev/build import author code; only load trusted files. dev without --fixture performs handshake only. An arbitrary Adapter fixture may start a native Agent and incur cost; this skill does not authorize that.

## Implement safely

Use typed request/response contracts and real lifecycle semantics. Emit started only after the actual native process starts, except for explicitly labeled in-process synthetic examples. Persist/restore native session references where required. Handle cancellation, subprocess cleanup, duplicate/conflicting input, request/reply correlation and late input isolation. Adapter completion is a proposal, never trusted Task success.

A new protocol field or method requires producer, Python codec, shared schema/frames, Go decoder/consumer, capability negotiation and compatibility analysis. Update methods/types documentation and packaging/source identity behavior together. Reject unsupported capabilities rather than silently widening authority. Processor output remains subject to platform validation; do not implement unauthorized Git push or bypass Finalizer.

## Validate and hand off

Run focused SDK and Adapter tests, shared Go agentprotocol tests for wire changes, and protocol docs checks. Use local mocked processes, not a real Provider, unless separately authorized. Check both valid/invalid frames and ordering/replay/cancel/restore paths appropriate to the change. Confirm source and built assets in separate temporary output directories; build emits assets, not a Docker image or admitted Runtime. Keep broad release checks proportionate. Report any unrun native CLI, Docker, platform integration and provider scenarios distinctly.
