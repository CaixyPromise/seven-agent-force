# Deterministic Fake Reviewer fixtures

Slice 4 uses the existing `fake-harness.Dockerfile` to produce immutable local
Reviewer fixtures.  Each image is built with exactly one
`SEVEN_FAKE_REVIEW_SCENARIO` build argument; the scenario is copied into the
read-only image contract during image creation and is not accepted from a Task,
environment variable, Provider response, or Worker message.

| Scenario | Expected Worker/Server behavior |
| --- | --- |
| `PASS` | Writes a valid PASS candidate report. |
| `REVISE` | Writes a valid REVISE candidate report. |
| `REJECT` | Writes a valid REJECT candidate report. |
| `INVALID` | Writes a contradictory PASS/blocking report; Server transitions to manual review. |
| `MALFORMED_JSON` | Writes malformed JSON; the Server accepts no verdict and records only the safe protocol-failure reason. |
| `INVALID_ENUM` | Writes a JSON report with an unsupported verdict enum; the Server rejects it as a protocol failure. |
| `OVERSIZED_REPORT` | Writes a report larger than the bounded candidate limit; the Server rejects it before decoding/persistence. |
| `PASS_WITH_BLOCKING_FINDING` | Writes a contradictory PASS with a blocking finding; canonicalization rejects it. |
| `PATH_TRAVERSAL` | Writes an evidence path containing `..`; canonicalization rejects it. |
| `SYMLINK_ESCAPE` | Writes an otherwise-shaped evidence path; the runtime Git-tree check rejects symlink/non-regular entries. |
| `INVALID_LINE_RANGE` | Writes evidence beyond the frozen blob's line count; the runtime rejects the candidate before verdict handling. |
| `UNABLE_TO_REPRODUCE_PASS` | Writes PASS with an `UNABLE_TO_REPRODUCE` test; canonicalization rejects the contradictory verdict. |
| `TIMEOUT` | Does not terminate; Worker timeout is a transient failure. |
| `CONTAINER_TIMEOUT` | Alias of the bounded container timeout fixture used by the Slice 7 fault matrix. |
| `RESPONSE_LOSS` | Terminates without a report; Worker classifies it as a transient failure. |
| `PROVIDER_TIMEOUT` | Remains alive until the Worker deadline, exercising the Provider-timeout/reconciliation boundary without a real Provider. |
| `PROVIDER_RESPONSE_LOSS` | Terminates without a report, exercising response-loss handling without a real Provider. |
| `CONTAINER_EXIT` | Exits with a fixed non-zero error; the Worker records a transient container failure. |
| `PROVIDER_PASS` | Calls the fixed `OPENAI_RESPONSES` request through only the run-scoped, network-none Unix-socket Fake Provider relay sidecar, then writes a valid PASS report. |
| `CORRECTION_CYCLE` | Returns REVISE until a successor ResultCommit contains the fixed marker written by FakeHarness only after a valid server-authorized `reviewCorrection`; the same image digest then returns PASS. |
| `FINDING_IMPROVEMENT` | Emits a fixed REVISE report with an improved finding fact for progress/no-progress matrix fixtures. |
| `TEST_IMPROVEMENT` | Emits a fixed REVISE report with improved deterministic-test evidence for progress matrix fixtures. |
| `NO_PROGRESS` | Emits a fixed REVISE report with unchanged material evidence for no-progress exhaustion fixtures. |
| `FORMATTING_ONLY_CHANGE` | Emits the same material review facts despite a result-formatting-only fixture, so the Server keeps the no-progress streak. |

Build each required fixture under its own local tag, inspect its **image digest**,
and register only that `sha256:<digest>` through the existing image-admission
workflow.  A tag is a local build convenience, never a Review Profile identity.
