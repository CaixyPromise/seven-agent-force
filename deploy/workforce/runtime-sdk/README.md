# Seven Agent Force runtime SDK

Canonical import: `seven_agent.force`. Distribution: `seven-agent-force-runtime-sdk`.
CLI: `seven-agent-force-runtime` (or `python -m seven_agent.force`).
The previous `seven_workforce_runtime` import and `seven-workforce-runtime` CLI remain compatible.

## Python SDK and synthetic Adapter

Optional Python tools for image authors. The wire protocol, not this package,
is the integration requirement. No platform API, credentials or model client
is included. Adapter output is untrusted observation, never Task success.

[interaction contract](INTERACTION.md) is the exact Python/Fake reference
for capability-gated questions, answer receipts, user messages and suspension.
The interaction API enforces one unresolved question request per execution.
Cold restore uses `ctx.receive_restored_answer()` with the complete trusted
`QuestionRequest` and expected `InteractionResponse`; normal downstream delivery
is required before a receipt or continuation, without creating a new question.
It includes the frozen backend/Web question fields, `selectedOptionKeys`,
`responseId`, async cancellation, bounded delivery and replay semantics. Native
adapters and downstream platform integration qualify independently.

See the [developer and multi-file integration guide](../../../docs/workforce/agent-adapter-developer-guide.md)
and [operations runbook](../../../docs/workforce/agent-adapter-operations.md).
The [interface reference](../../../docs/workforce/agent-adapter-interface-reference.md)
documents method payloads, Python signatures, AGENT/REVIEW inputs, ordering,
errors and process-local replay boundaries.
Project-directory packaging is available through `inspect/dev/build PROJECT --module PACKAGE`.
It supports a regular src/flat Python package and declared setuptools package-data,
bundles the stdlib-only SDK, and indexes all packaged code/resources. No dependency
installation or build backend runs. The single-file builder remains a separate mode.

```python
from seven_agent.force.agent import AgentAdapter, RoundResult

agent = AgentAdapter(name="example", version="1")

@agent.run
async def run(request, context):
    # Start the native process first, then report its actual start.
    await context.started()
    await context.emit_message("Local fixture executed.")
    return RoundResult.finished(summary="Native round ended.",
                                session_reference="local-fixture-session")

if __name__ == "__main__":
    agent.serve()
```

`started()` above describes a local fixture only. Real adapters must call it
after native startup. Implement cancellation-safe child process cleanup; the
outer Supervisor remains responsible for deadline enforcement and escalation.
The example session reference is for a local fixture. A Coding Agent adapter
must return its real stable native session reference and register `@agent.resume`
to restore it; the platform never treats a fabricated reference as a checkpoint.

## Local checks

From the repository root:

```sh
PYTHONPATH=deploy/workforce/runtime-sdk/src python3 -m unittest discover -s deploy/workforce/runtime-sdk/tests -v
```

The development commands are `inspect SOURCE`, `dev SOURCE [--fixture INPUT.json]`,
`check TRANSCRIPT --direction up|down`, and
`build SOURCE --output DIRECTORY --image-module /opt/seven/adapter/main.py`.
For projects use `inspect/dev PROJECT --module my_adapter` and
`build PROJECT --module my_adapter --output DIRECTORY` instead of `--image-module`.
The module must export one AgentAdapter (directly or through the package's guarded
`__main__.py`); modules must not call serve or models at import time.
`inspect`, `dev`, and `build` execute the supplied Python module: use trusted
author code only. `dev` simulates the Supervisor handshake without starting an
Agent; an explicit local fixture opts into a bounded execution. It never
simulates platform authorization. `check` validates frames, not full lifecycle
conformance. `build` emits assets, not an admitted image. The image must install
this package into system site-packages or add only its protected, indexed
image path explicitly. Python `-I` ignores `PYTHONPATH` and the script directory.

## Verification scope

Shared protocol frames and Python lifecycle tests exercise codec behavior, handshake, duplicate input, cancellation and packaging. Platform integration, native CLI, Docker and real Provider behavior need separate validation in the target environment. See [SDK development](../../../docs/sdk-development.md).
