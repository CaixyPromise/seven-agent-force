# Python interaction contract

This slice implements the Python SDK and fixed Fake fixture. Native Coding Agent
adapters, Supervisor/Worker transport, persistent interaction storage, Web UI,
image admission and deployment are independent integrations. These frames are
untrusted observations and delivery acknowledgements, never approval grants,
trusted completion, or proof that a checkpoint is recoverable.

## Wire contract

Every frame is one UTF-8 JSON object followed by a newline:

```json
{"protocol":"seven.agent-adapter/v1","method":"interaction.respond","key":"answer:1","payload":{"roundKey":"round:1","interactionKey":"question:1","responseId":"101","answers":[{"questionKey":"result","selectedOptionKeys":["alpha"],"text":""}]}}
```

Envelope fields are `protocol`, `method`, `key`, `payload`, and optional `replyTo`.
All fields listed below are required, including empty arrays, empty description/text,
and explicit booleans. Additional fields and nulls are rejected.

| Method | Direction | Exact payload fields |
| --- | --- | --- |
| `interaction.request` | Adapter → host | `{roundKey, interactionKey, questions: Question[]}` |
| `interaction.respond` | Host → Adapter | `{roundKey, interactionKey, responseId, answers: InteractionAnswer[]}` |
| `interaction.received` | Adapter → host | `{roundKey, interactionKey, responseId, status}` |
| `user.message` | Host → Adapter | `{roundKey, messageKey, text}` |
| `message.received` | Adapter → host | `{roundKey, messageKey, status}` |
| `execution.suspend` | Host → Adapter | `{roundKey}` |
| `execution.suspended` | Adapter → host | `{roundKey, sessionReference}` |

`Question` is exactly:

```json
{"key":"result","title":"Choose a result","description":"One fixed fixture result","kind":"SINGLE_CHOICE","required":true,"allowFreeText":false,"options":[{"key":"alpha","label":"Alpha"},{"key":"beta","label":"Beta"}]}
```

`InteractionAnswer` is exactly:

```json
{"questionKey":"result","selectedOptionKeys":["alpha"],"text":""}
```

`kind` is `SINGLE_CHOICE`, `MULTIPLE_CHOICE`, or `TEXT`. `TEXT` has no options
and accepts text regardless of `allowFreeText`; the flag controls additional text
for choice questions. Choice questions have 1–8 unique options. `SINGLE_CHOICE`
accepts at most one selection; `MULTIPLE_CHOICE` accepts at most eight. All selected
keys must occur in that question. A required question must be present and have a
selection or nonblank text. Optional questions may be omitted or answered with
`selectedOptionKeys: []` and `text: ""`. An all-optional request accepts `answers: []`.
Unknown/duplicate question keys or duplicate selected keys are invalid.

Limits match the backend question model:

| Value | Bound |
| --- | --- |
| Questions per request | 1–3, unique keys |
| Answers per delivery | 0–3; required/optional rules checked against the pending request |
| Options per question / selected keys per answer | 0–8, with kind-specific rules above |
| Question title / option label | Nonblank, at most 512 UTF-8 bytes |
| Question description | At most 4,096 UTF-8 bytes |
| Question/option keys | Nonblank, at most 128 UTF-8 bytes, no surrounding whitespace or NUL |
| Complete answers array | At most 16,384 bytes using compact Go `json.Marshal`-compatible UTF-8 JSON, including HTML and U+2028/U+2029 escaping |
| User message text | Nonblank, at most 16,384 UTF-8 bytes |
| Suspension session reference | Nonblank, at most 4,096 UTF-8 bytes |
| Envelope key/replyTo, roundKey, interactionKey, messageKey | `[A-Za-z0-9._:-]{1,128}` |
| responseId | Positive int64 decimal **string**, no leading zeros, maximum `9223372036854775807` |

Text fields reject NUL. Existing frame limit (1 MiB), duplicate JSON key rejection,
finite-number validation, nesting limit and direction allowlists still apply.
[JSON Schema](protocol/agent-adapter-v1.schema.json) documents structure; byte limits,
unique keys, pending-question rules, direction and runtime capability gates require
the codec/session validator. [Shared fixtures](protocol/frames.json) include both
directions and invalid interaction payloads.

## Correlation and capabilities

The host maps `interactionKey` to its persisted `localRequestKey`. It retains
Task/Attempt/session-generation/owner authority outside these SDK payloads.
`responseId` identifies the saved backend response; the SDK echoes it, never
generates a database identity. `interaction.request`, `execution.started`, and
other round-originated observations use `replyTo` equal to the accepted
`execution.start`/`execution.resume` envelope key. Receipts and
`execution.suspended` use `replyTo` equal to their incoming control envelope key.

| Registration | Advertised optional capabilities |
| --- | --- |
| `AgentAdapter(..., capabilities=("interaction.request",))` | `interaction.request`, `interaction.respond`, `interaction.received`, `interaction.closed` |
| `@agent.message` async handler | `user.message`, `message.received` |
| `@agent.suspend` async handler plus `@agent.resume` | `execution.suspend`, `execution.suspended` |

Unknown constructor capabilities are rejected. Message and suspend support cannot
be enabled merely by passing strings: the handlers must exist. Absent capabilities
cause supported-shape down controls to return `ADAPTER_METHOD_UNSUPPORTED` without
invoking application code. Unknown method names likewise leave control reading
available. Malformed known frames fail closed with a sanitized error and session
closure; a capability declaration does not bypass wire validation.

## Python API

The typed objects are exported from `seven_agent.force.agent` and defined in
`seven_agent.force.interaction`:

```python
QuestionOption(key: str, label: str)
Question(key: str, title: str, description: str = "", kind: str = "TEXT",
         required: bool = True, allow_free_text: bool = True,
         options: tuple[QuestionOption, ...] = ())
QuestionRequest(interaction_key: str, questions: tuple[Question, ...])
InteractionAnswer(question_key: str, selected_option_keys: tuple[str, ...] = (),
                  text: str = "")
UserMessage(round_key: str, message_key: str, text: str)
InteractionResponse(interaction_key: str, response_id: str,
                    answers: tuple[InteractionAnswer, ...])
DeliveryStatus.RECEIVED | DeliveryStatus.REJECTED | DeliveryStatus.UNKNOWN
```

Python defaults are materialized explicitly on the wire. `await ctx.ask(request)`
returns `tuple[InteractionAnswer, ...]` after local receipt of a valid answer. Call
`await ctx.started()` first. The coroutine supports task cancellation and
`asyncio.wait_for`/`asyncio.timeout`. Interruption or EOF cancels the running round,
its outstanding asks and cooperative message/suspend handlers. One execution may
have only one unresolved interaction request (containing up to three questions).
A concurrent different-key ask fails with `ADAPTER_INTERACTION_BUSY` **before**
emitting a frame or adding history. Identical same-key callers share the existing
future; changed questions conflict. After a received answer resolves the pending
request, another key may be asked. Cancelling a local wait does not remove the
host's unresolved card: the key remains consumed and a second request is still
busy until the round ends. No local timeout is an implicit host cancellation.

### Cold answer receipt

```python
await ctx.started()
answers = await ctx.receive_restored_answer(question_request, expected_response)
```

`question_request` is the complete typed `QuestionRequest`; `expected_response`
is a typed `InteractionResponse` with the original business `response_id` and
answers. `QuestionRequest.from_wire(value)` parses the complete frozen shared
request, preserving every Question field and enforcing the normal request rules.
`InteractionResponse.wire()` shares the normal downstream codec, bounds,
positive decimal int64 ID rules and typed answer validation. The API validates
the answers against the complete question semantics and registers the single
pending question in the **current** round without emitting `interaction.request`.
It does not fabricate a receipt or mark `ctx.received_response()` as accepted.

Only normal `interaction.respond` carrying that round, interaction key, exact
response ID and canonical JSON answers resolves this cancellable wait. Different
rounds are rejected; a valid but different response ID/answer returns
`ADAPTER_IDEMPOTENCY_CONFLICT` and leaves the expected delivery pending. The normal
SDK `interaction.received` uses the incoming control's `replyTo`, current round,
original response ID and `status: "RECEIVED"`. Replays reuse that receipt and do
not reexecute the agent. This API has the same optional capability and lifecycle
gates as `ask`; it introduces no additional wire method or manifest capability.

The host/Supervisor must independently register the original full QuestionRequest
from trusted persisted interaction data, correlate delivery to the new native
round/current Turn, and retain the old business response identity. The private
restore snapshot is an expected-value contract, **not** an automatic local receipt.
Do not create a new user-visible question to make the old answer deliverable.

`ctx.received_response(interaction_key)` returns an immutable `InteractionResponse`
snapshot or `None`. The SDK records it atomically when accepting an answer, before
receipt transport and before the ask coroutine necessarily runs again. A native
suspend handler can therefore persist an already accepted answer at its final
safe point without racing coroutine scheduling. This getter itself does not
persist anything or claim that native execution has applied the answer.

```python
from seven_agent.force.agent import (
    AgentAdapter, QuestionRequest, Question, QuestionOption, RoundResult,
)

agent = AgentAdapter(name="question-fixture", version="3",
                     capabilities=("interaction.request",))

@agent.run
async def run(request, ctx):
    await ctx.started()  # This example executes inside the Python process.
    answers = await ctx.ask(QuestionRequest("result-question", (
        Question("result", "Which result?", kind="SINGLE_CHOICE",
                 allow_free_text=False,
                 options=(QuestionOption("alpha", "Alpha"),
                          QuestionOption("beta", "Beta"))),
    )))
    await ctx.emit_message("Received " + answers[0].selected_option_keys[0])
    return RoundResult.finished(summary="Question fixture finished")
```

`@agent.message` receives `(UserMessage, ExecutionContext)` and returns an explicit
`DeliveryStatus`. `RECEIVED` means the handler reports local consumption;
`REJECTED` means the handler explicitly declined it; `UNKNOWN` means consumption
cannot be established. Handler exceptions, cancellation and invalid return types
produce `UNKNOWN` without exposing exception text. The SDK never reruns that
delivery automatically. The handler must report status honestly and implement
native cancellation cleanup.

`@agent.suspend` receives `(RunRequest, ExecutionContext)` with the current round
key and empty inputs. It must quiesce native work and persist the real native
session before returning its nonblank session-reference string. The SDK then
cancels the run coroutine and pending asks, cancels other controls for that round,
and emits only `execution.suspended`, **not** `execution.finished`. A handler
exception, empty reference or round that already ended yields
`ADAPTER_SUSPEND_FAILED`; it never fabricates a reference or a recoverability flag.
A resume handler is required, but its existence alone cannot prove restoration.
The host still owns verified checkpoint capture and cleanup. Failed suspension is
replayable as failure; interruption remains available to stop a still-active round.
From acceptance of `execution.suspend` until its handler finishes, the SDK fences
new answers, user messages and asks with `ADAPTER_ROUND_SUSPENDING`. Already accepted
deliveries remain replayable and interruption stays available. This prevents an
answer arriving after the native handler's final session snapshot. A failed handler
reopens new input; successful suspension closes the round.
Before `execution.finished` or `execution.suspended`, the SDK cancels and drains
the round's outstanding control tasks, including their receipt writes, for up to
five seconds. The suspend task excludes itself from this drain. An unconfirmed
receipt produces `ADAPTER_CONTROL_RECEIPT_UNCONFIRMED` and prevents terminal success
and new-round execution in that session; the host must reconcile the saved delivery.

## Delivery replay and lifecycle

`interaction.respond` resolves the existing ask future; it never calls run/resume.
The answer is accepted atomically before awaiting receipt transport. Logical delivery
identity is `(roundKey, interactionKey)` for answers and `(roundKey, messageKey)`
for messages. The complete method/payload digest, including `responseId`, is bound
to that identity. Matching retries, including fresh envelope keys, replay the
receipt. Changed response IDs/answers/text return `ADAPTER_IDEMPOTENCY_CONFLICT`.
Pending message/suspend retries share the original operation. Unknown, expired,
cancelled or wrong-round questions cannot receive answers. Matching accepted-answer
retries remain replayable after round completion.

Repeated `ctx.ask` with the same key and identical questions shares the existing
future/result; changed questions conflict. After a native round ends, replaying its
start key/round replays its terminal frame; it cannot execute again. Suspension
is one keyed operation per round; duplicates replay the same suspended observation
or failure. A new round requires a new round key. Replay history is process-local;
it is not durable exactly-once execution across an Adapter restart. The host must
fence old-session delivery rather than replay an old answer into a new ask.

The session retains at most 256 rounds, 256 ask keys per round, 2,048 incoming
envelope keys and 2,048 logical control deliveries. Limits fail closed with
`ADAPTER_SESSION_LIMIT`; keys are not evicted and reused. Stdin reading runs
independently of execution and optional async control handlers; a blocked handler
cannot prevent receipt of an interrupt. Handlers must cooperate with cancellation
and must not block the event loop with synchronous work. Output drain is bounded
to five seconds; the host remains responsible for hard deadline enforcement.

## Local verification

From the repository root:

```sh
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=deploy/workforce/runtime-sdk/src python3 -m unittest discover -s deploy/workforce/runtime-sdk/tests -v
PYTHONDONTWRITEBYTECODE=1 PYTHONPATH=deploy/workforce/runtime-sdk/src python3 -m unittest discover -s deploy/workforce/adapters/fake/tests -v
```

Tests use real child-process stdin/stdout for questions, answer receipt/replay,
conflicts, cancellation, unknown/disabled capabilities, blocked message handlers,
suspension and Fake session preservation. The fixed Fake fixture additionally
requires a supplement, drains pending message delivery at its final safe point,
and consumes a private host-injected `inputs.interactionResponse` on fresh-process
resume while preserving its original session ID. They make no model calls. See the
[Fake fixture documentation](../adapters/fake/README.md) for its fixed entrypoint.
