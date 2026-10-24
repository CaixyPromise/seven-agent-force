# Image-owned Codex App Server AgentAdapter

This package is the Codex-specific implementation of `seven.agent-adapter/v1`.
The platform starts only the admitted `/opt/seven/adapter/entrypoint`; the image
owns `adapter.py`, `app_server.py`, and the pinned Codex CLI **0.147.0**. The
image build indexes every file below `/opt/seven/adapter`, so changing or
omitting either Python module changes admission identity.

The Adapter is a translation boundary, not a business-policy component. Task,
authorization, completion-check, Review, Finalizer, Artifact, and Delivery
authority remain in Server/Worker code.

## Runtime mapping

One Adapter session owns one persistent `codex app-server --stdio` child:

| Seven Adapter operation | Codex App Server operation |
| --- | --- |
| `initialize` | start child, `initialize`, `initialized` |
| `execution.start` | `thread/start`, then `turn/start` |
| `execution.resume` | `thread/resume`, then a new `turn/start` |
| `user.message` | `turn/steer` with the current `expectedTurnId` |
| `execution.interrupt` / suspend | `turn/interrupt`, then bounded process cleanup |
| native `requestUserInput` | `interaction.request`, then one correlated native response |
| native request resolution / turn end | `interaction.closed` |
| native turn completion | `execution.finished` |

`turn/started` and `turn/completed` define a round. App Server process liveness
does not. A Worker control reconnection reattaches to the Supervisor and never
resends `execution.start`.

Native thread, turn, item, and JSON-RPC IDs stay private to this package. The
Adapter maps them to platform round and interaction keys. A hot answer resolves
the original native request exactly once. A cold resume uses the persisted
thread ID plus the Server-authoritative saved question/answer as explicit new-
turn context; it never reuses an old JSON-RPC request ID.

For a checkpoint restored into a new Attempt namespace, the Adapter supplies
the unique matching rollout under the current `CODEX_HOME/sessions` through
the pinned CLI's `thread/resume.path`. Codex's own index may still contain the
preceding Attempt's absolute path. The Adapter does not rewrite that index or
mount prior state, and checks the returned thread ID before starting a turn.
Ambiguous rollout files and paths escaping the restored state fail closed.

## Input contracts

AGENT start receives the Supervisor-owned shape below. Unknown fields,
noncanonical IDs, unsafe paths, model overrides, executables, arguments,
environment values, provider URLs, and credentials are rejected.

```json
{
  "mode": "AGENT",
  "taskId": "123",
  "attemptId": "456",
  "instruction": "Supervisor-prepared instruction",
  "workspace": "/workspace/123/attempts/456/repo",
  "output": "/workspace/123/attempts/456/output",
  "state": "/workspace/123/attempts/456/state",
  "temporary": "/workspace/123/attempts/456/tmp",
  "provider": {"model": "frozen-model"},
  "feedback": "",
  "reviewCorrection": null
}
```

Resume uses the same shape, the prior private thread ID in
`session_reference`, and nonempty `feedback`. An interaction-suspend resume may
also contain `interactionQuestionRequest` and `interactionResponse`; these are
validated, correlated, and added as context, not treated as authorization.

REVIEW starts an independent ephemeral thread:

```json
{
  "mode": "REVIEW",
  "workspace": "/scratch/repo",
  "model": "frozen-model",
  "prompt": "Go-prepared bounded review prompt",
  "output_schema": "/tmp/seven-review-report-v1.schema.json",
  "output_path": "/report/review-report.json"
}
```

The output schema is passed through `turn/start.outputSchema`. The final native
assistant message is written to the trusted preparation's report path, but
remains an untrusted candidate until the Server canonicalizer accepts it.
Reviewer sessions do not expose questions or user steering.

AGENT explicitly enables `features.default_mode_request_user_input=true` for
the pinned Codex 0.147.0, whose Default-mode native questions are disabled by
default. It keeps the coding collaboration mode and forwards native questions
through the existing interaction bridge, including their native blocking flag.
REVIEW does not enable this feature. A user message asking a question is not an
interaction request and must not be converted into a synthetic question card.

## Provider and filesystem isolation

The native executable and configuration are fixed. The command uses
`app-server --stdio --strict-config`, a clean `HOME/CODEX_HOME`, frozen model,
and provider `seven` or `seven-review`. Both providers point only to the existing
loopback facade at `http://127.0.0.1:8787/v1`. AGENT receives a non-secret
placeholder credential; REVIEW receives no provider credential. Real provider
keys remain decrypted only by the Server adapter.

The App Server child stays in the Adapter/Supervisor process group. Normal
shutdown terminates and reaps it; cancellation or container cleanup terminates
the whole supervised group. No App Server control port is exposed.

## Observable events and interaction limits

- completed public assistant messages become bounded `message` events;
- plans become bounded `plan` events;
- command, file, MCP, and web-search lifecycle become fixed-title `activity`;
- reasoning, raw tool output, Provider bodies, usage payloads, credentials, and
  unknown native fields are not forwarded;
- a native request maps at most three questions and eight options per question;
- at most eight unresolved interactions may exist in one round;
- secret-input questions and native permission/file/command approvals are
  rejected because this Adapter does not implement operation authorization;
- nonblocking questions remain answerable while the turn proceeds; turn end or
  native resolution closes their existing card;
- an Adapter receipt means received, not applied, authorized, or successful.

Wire frames are limited to 1 MiB and public text to the shared SDK bounds. The
App Server request table is bounded, writes are serialized, stderr is discarded
with a fixed cap, and protocol failures cross the Adapter boundary only as stable
machine codes.

## Offline development and tests

The deterministic `tests/mock_codex.py` speaks the 0.147.0 JSON-RPC shape over
real pipes. It covers start/resume, activity, blocking and nonblocking questions,
native close, steer, structured REVIEW output, secret-question rejection, and
child cleanup without calling a Provider.

```sh
PYTHONPATH=deploy/workforce/runtime-sdk/src \
python3 -B -m unittest discover \
  -s deploy/workforce/adapters/codex/tests -p 'test_*.py'
```

Image acceptance additionally verifies `codex --version`, the generated source
index, manifest capability equality, and the exact immutable image digest. A
successful offline test is not evidence of a real Codex model call.
