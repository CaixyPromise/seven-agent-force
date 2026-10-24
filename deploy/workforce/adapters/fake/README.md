# Image-owned Fake AgentAdapter

Image-owned sidecar using `seven_agent.force.agent.AgentAdapter` with fixed answer, supplement and suspension fixtures.
The build selects a fixed fixture entrypoint. Provider, database, image
admission and service deployment qualify independently. Cold resume consumes the
private, host-authorized `inputs.interactionResponse` contract documented below.

## Fixed image entries

`batch3_combined.py` is a fixed dual-role image entry: AGENT
delegates to the existing answer/supplement/suspend fixture, while REVIEW
delegates to `batch3_review.py` and `fixtures/batch3-review.json`. REVIEW checks
only `batch3-result.txt` (`Fixed answer: alpha\n` or `Fixed answer: beta\n`) and
nonempty UTF-8 `batch3-supplements.txt`. Missing, invalid or unsafe files produce
REVISE; both valid checks produce a candidate PASS. It never reads prompt or
environment to select verdict, never asks/steers, never changes result files,
and never copies supplement contents into its report.

The dual-role image retains the existing real CLI Review gateway and frozen
Provider/relay contract. The deterministic REVIEW algorithm does not call that
interface. This is distinct from removing gateway capabilities or weakening
production image admission. Existing Task/Profile snapshots remain frozen.

| Entrypoint | Asset | Role and behavior |
| --- | --- | --- |
| `combined.py` | `fixtures/batch1.json` + `fixtures/batch1-review.json` | AGENT / BATCH1_COMPLETION and REVIEW / BATCH1_RESULT; integration entrypoint |
| `batch1.py` | `fixtures/batch1.json` | AGENT / BATCH1_COMPLETION |
| `correction.py` | `fixtures/correction.json` | AGENT / CORRECTION_CYCLE |
| `review.py` | `fixtures/review.json` | REVIEW / CORRECTION_CYCLE |
| `batch3.py` | `fixtures/batch3.json` | AGENT / BATCH3_SAFE_POINT; fixed Alpha/Beta answer + supplement, suspend/cold resume |

`batch3.py` imports the independent `interaction.py` implementation. It advertises
questions, user messages, suspension and their paired receipts, plus ordinary
start/resume/interrupt capabilities. The fixed fixture requires an Alpha/Beta
answer **and at least one supplement** before writing `batch3-result.txt` and
`batch3-supplements.txt`. Neither Task instruction nor runtime environment can
change this behavior. The message handler enqueues text and returns `RECEIVED`
only after the execution loop or final suspend safe point has persisted it.
This receipt remains a local delivery fact, not a platform application-status event.
Message keys/text are deduplicated in the persisted session; changed text is
rejected, and at most 32 messages / 64 KiB of serialized supplement records are
retained. Completion waits for safe-point message handlers to return.

The suspend handler holds the same mutation lock as the execution loop, saves
already accepted answers and pending supplements, marks the session `SUSPENDED`,
and stops further writes before returning the original session reference. The
SDK emits `execution.suspended` only. A cancelled ask is not a completed result.
Repeated answers/messages/suspend controls do not repeat completed mutations.
The immutable SDK `ctx.received_response()` snapshot closes the race where an
answer has been accepted but the awaiting ask coroutine has not yet been scheduled.

The native file is **`state/codex/fake-session.json`**, inside the checkpoint
collector's `codex/` subtree. The Adapter does not write the Supervisor-owned
`state/session.json`. The native file is private (`0600`) under a private (`0700`)
directory, rejects links, and has this exact shape:

```text
{
  schemaVersion: 1, kind: "fake-session",
  sessionId, sessionDigest, round, taskId, fixtureDigest,
  question: {interactionKey, questions},
  answer: null | {interactionKey, responseId, answers},
  supplements: [{messageKey, text}],
  phase: "ACTIVE" | "SUSPENDED" | "FINISHED",
  resultWritten: boolean
}
```

`sessionDigest` is SHA-256 of the original `sessionId`; `round` counts this native
fixture's executions, not a platform revision budget. Restore validates the exact
fixture/task/question/session identity and preserves the session ID even in a
fresh process and different Attempt. The host independently carries session
generation and automatic revision budget in its checkpoint identity.

When the answer is saved **after** suspension, the host injects the authorized
saved response into `execution.resume.payload.inputs.interactionResponse` and the
original complete request into `inputs.interactionQuestionRequest`:

```json
{
  "interactionQuestionRequest": {
    "interactionKey": "fake:question",
    "questions": [{"key":"result","title":"Which fixed result should the fixture record?","description":"Choose Alpha or Beta and send a supplement. The fixture preserves both in the same session.","kind":"SINGLE_CHOICE","required":true,"allowFreeText":false,"options":[{"key":"alpha","label":"Alpha"},{"key":"beta","label":"Beta"}]}]
  },
  "interactionResponse": {"interactionKey":"fake:question","responseId":"101","answers":[{"questionKey":"result","selectedOptionKeys":["beta"],"text":""}]}
}
```

The example shows the complete field contract; actual question text must match
the frozen fixture and original persisted request exactly. Both fields are
required together on cold resume. The complete question is parsed with shared
`QuestionRequest.from_wire`, using Server-projected trusted restore data; it is
compared with both the fixed fixture and native session question before use.
These fields are accepted on resume only. The answer must match that question;
if an answer was already saved in the native session, conflicting input is rejected
before writing session bytes. On cold input the run handler calls the SDK's
`await ctx.receive_restored_answer(complete_question, expected_response)` before
continuing or returning an already-written result. It emits no new question and
does not acknowledge the private input automatically. The host must send the
matching normal `interaction.respond` into the **new** round; only then does the
SDK emit `interaction.received` (same original response ID, `RECEIVED`, `replyTo`
the down control) and unlock continuation in the original session. A different
ID or valid but different answer conflicts without writing result artifacts.
Supervisor pending state must also be restored from trusted original question
data; native session JSON is not a replacement for host authority.
Missing answer input with
no answer in the session keeps the fixed question pending and reissues it in the
new round. A resumed finished fixture returns its existing result without
rewriting output files. Session bytes alone are not a trusted checkpoint.
The entry uses the same protected image import paths as `combined.py`; it is not
selected by task text, model, feedback or environment. No model client is used.
The [SDK interaction contract](../../runtime-sdk/INTERACTION.md) defines all exact
payloads, `responseId` correlation and question/answer validation rules.

### Fixed interaction-fixture image build

[fake-harness.Dockerfile](../../images/fake-harness.Dockerfile) accepts only the
build-time argument `SEVEN_FAKE_BATCH=batch2|batch3` (default `batch2`). Example:

```sh
docker build -f deploy/workforce/images/fake-harness.Dockerfile \
  --build-arg SEVEN_FAKE_BATCH=batch3 -t seven-fake-batch3:local .
```

The interaction-fixture build copies the separate [manifest](../../images/fake-harness-batch3-manifest.json)
and binds `/opt/seven/adapter/entrypoint` to `python3 -I /opt/seven/adapter/batch3.py`.
The runtime does not read this build argument. Unknown build values fail closed.
This specialized image declares `executionKinds: ["AGENT_ATTEMPT"]`; it does not
claim Reviewer behavior. The default combined image retains the combined entry.
The complete copied native modules (`adapter.py`, `combined.py`, `batch3.py`,
`interaction.py`) and all fixture resources enter `source-index.json`; the launcher
and source index digests bind the manifest. The SDK package index also includes
its interaction module. Tests compare the real ready capabilities/version/name
with the interaction-fixture manifest and verify copied modules/resources and index digests.

Local tests exercise real pipes, safe-point receipts, buffered answer/suspend and
message/suspend races, original-process exit, restored session in a new Attempt,
saved-answer consumption, replay, conflicting cold input, message bounds, isolated
imports and link rejection with network operations blocked. They do not build or
admit a Docker image, capture an encrypted host checkpoint, or prove the platform
HTTP/Worker cold-answer wiring; those require platform integration tests.

Each entry embeds its asset SHA-256 and loads only those adjacent assets. Image
packaging must freeze the entrypoint, shared `adapter.py`, fixtures and SDK in
protected image paths and bind them to the admitted immutable image digest.
The production launcher is `python3 -I /opt/seven/adapter/combined.py`.
Before importing the SDK or shared adapter, this entrypoint explicitly prepends
`/opt/seven-worker/result-runtime` and `/opt/seven/adapter` to `sys.path`; it does
not derive import paths from cwd, `__file__`, or `PYTHONPATH`.
The planned manifest `nativeSource` digest is a platform integration concern;
these local asset checks do not substitute for image admission. Invoke the
fixed entrypoint with the installed SDK available on the protected Python path.
Never select the entrypoint, asset or expected digest from Task data or environment.
`combined.py` is the manifest target for the combined image requiring both execution
kinds. Its run handler routes only by standard `mode` to the two fixed handlers;
resume accepts AGENT only. Specialized entries remain available and require their
fixed role. No environment variables,
instruction markers, model names, feedback text or requested verdict select a
scenario. Unknown top-level fields (including `scenario` and `verdict`) fail closed.

## Shared requests and native results

AGENT inputs: `mode`, `taskId`, `attemptId`, `instruction`, `workspace`, `output`,
`state`, `temporary`, `provider: {model}`, `feedback`, and optional/null
`reviewCorrection`. IDs are positive int64 decimal strings. Feedback may be null
or bounded text. Review correction uses the existing Go object:
`sourceReviewRunId`, `revisionKind` (`AUTOMATIC` or `HUMAN_AUTHORIZED`), with
optional `verdict`, `reportPath`, `reportDigest`, `reasonCode` strings. Main must
validate and authorize that contract before passing it; Fake's shape checks
are not authorization. Correction text/verdict is never a scenario selector.

REVIEW inputs: `mode`, `workspace`, `model`, `prompt`, `output_schema`,
`output_path`. Go owns preparing the isolated result checkout, prompt and schema.
The standard optional fields `resultCommit`, `resultTree`, `requirements_path` and
`review_contract_path` are accepted but not required. Commit/tree values are bounded strings;
requirements/contract paths are validated as absolute paths. They do not select assets
or verdicts and are not interpreted as proof of checkout identity.
Fake validates the schema path's shape but does not execute or load it; its
fixed report shape is image code. It writes a candidate report of at most 128 KiB
to the prepared `output_path` in an existing directory outside the repository.
The filename is supplied by the shared contract. Go must constrain this supplied
path to its report mount. No extra `output` or scenario field is required.
No clone, checkout, provider call or review command runs inside Fake. Consequently
the report's `reviewTests` describes only marker inspection, rather than copying
the old Go fixture's checkout-success claim.

The Python process itself runs the native fake work. It calls
`context.started(native_pid=os.getpid())`, emits fixed STARTED/SUCCEEDED activity,
and returns `RoundResult.finished(summary=..., session_reference=...)`.
SDK `execution.finished` carries only round key, native outcome, summary and
session reference; it has no authoritative verdict, changed files or hashes.
Candidate REVIEW verdicts exist only inside the report for downstream validation.

## Main-chain behavior

The fixture validates the existing read-only, regular, bounded
`<task root>/input/attempts/<attemptId>/completion-check.json`: closed schema,
no references, normalized Go-compatible definition digest, and the exact
path/digest citation in the instruction. Workspace must be
`<task root>/attempts/<attemptId>/repo`. It never executes the supplied check.
First start writes `value.txt = bad\n`. Resume writes `good\n` and the exact Go
`batch1-result.txt` contents, leaving the check entrypoint untouched.

Correction Agent ports the fixed revise-then-pass behavior without selecting it
from instruction text: first start writes `SEVEN_WORKER_DRAFT.md`; resume writes
`SEVEN_WORKER_RESULT.md`. A supplied, supervisor-validated correction contract
writes the result plus `SEVEN_WORKER_REVIEW_CORRECTION.md` with the Go marker text,
including when a correction is started in a fresh Agent session.
Correction Reviewer emits the Go REVISE/60 finding when the marker is absent and
PASS/100 when it is a regular file. Marker contents are not authority; no marker
authenticity is asserted by this adapter. Symlink/special/hardlinked markers fail
closed instead of following them.

The combined Reviewer instead inspects the exact bytes of `value.txt`
(`good\n`) and `batch1-result.txt` (the fixed result record). Both must match for a
PASS/100 candidate; missing or incorrect contents produce REVISE/60 with FAILED
reviewTests. Unsafe file types fail execution without a new candidate. It needs no
correction marker, avoiding the combined/correction-marker revision loop. This is
fixed fixture validation, not an unconditional PASS asset or a trusted platform
verdict. Optional generic review metadata does not override the byte checks.

AGENT persists `state/fake-session.json` with mode 0600 under an existing private
state directory (0700). The bounded token `fake-session-<sha256>` is the raw
native reference, derived from initial task/attempt identity; it is returned only
through the SDK session field, never messages/activities. The archive stores its
digest, round, task and fixture identity. Resume requires that exact reference and
archive, including after copying private state into a new attempt for the same
task. A fresh start never overwrites an existing session. This private archive is
not the old Go supervisor archive; the parent owns capturing/restoring it and
lineage authorization. File replacement is atomic and private, directory traversal
does not follow symlinks, and existing nonregular/hardlinked write targets are
rejected. Multi-file work plus session update is not a transactional filesystem
commit; a crash can leave partial workspace work for the supervisor to handle.

## Local evidence and exclusions

Run from the repository root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s deploy/workforce/adapters/fake/tests -v
```

Tests use temporary workspaces and fixed assets; the only command executed is a
test-authored local `sh ./check.sh` to demonstrate the actual failure/fix. Tests
cover candidate reports, same/restored sessions, SDK wire results and sanitized
failure, fixed roles, environment/input selector resistance, digest/input limits,
and symlink/hardlink/path/private-state rejection. No external verifier is used.
The isolated-import test runs a real `python3 -I` subprocess with hostile cwd and
PYTHONPATH modules, mapping the two fixed image mount paths to temporary copies
through a test-only import finder. It verifies SDK/shared-module origins and
adapter initialization. It does not claim actual Docker mount/launcher acceptance.

The synthetic fixtures validate their declared behavior only. Native CLI, Docker image execution, platform authorization and real Provider behavior require separate integration checks.

## Execution observations

The observed-fixture revision emits bounded observations for actual work using
the existing SDK protocol: `fake:prepare` STARTED, runtime fixture read/hash and
typed-question validation, then COMPLETED, a safe message and a two-step plan.
Only after receiving both answer and supplement does `fake:record` STARTED;
SUCCEEDED follows successful writes of both result files and the original
session. Write failure produces a failed execution, never a success activity.
Waiting loops emit no periodic events, duplicate answers do not repeat the
record activity, and an already-finished resume does not claim new record work.
These are Adapter work observations, not trusted Server completion evidence.
Use the task-frozen image identity when comparing supported observations.
