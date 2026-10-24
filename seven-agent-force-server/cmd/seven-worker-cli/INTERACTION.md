# Supervisor interaction integration

The Supervisor uses the admitted image Adapter through
[`agentprotocol.Process.SendControl`](../../internal/worker/agentprotocol/control.go).
`Round` owns stdout; `SendControl(ctx, method, payload) (controlKey, error)`
serializes stdin without acquiring the round lock. Its result means **written**,
not received, applied, authorized, or successfully completed. Round observations
correlate control replies by envelope key, method, round, logical interaction or
message key, and response ID. Startup acknowledgment cannot substitute for a
control receipt or native startup. Reviewer rounds reject interaction frames.
At a terminal frame, new sends are fenced and already-written control receipts
are drained for up to five seconds. Missing/invalid late receipts produce
`ADAPTER_CONTROL_RECEIPT_UNCONFIRMED`; the CLI holds PAUSING for parent reconciliation
instead of presenting an ambiguous delivery as completion.

The exact closed payloads, question kinds, limits and capability groups match
the [Python SDK contract](../../../deploy/workforce/runtime-sdk/INTERACTION.md).
`interaction.respond` and `interaction.received` both require the backend's
positive int64 decimal-string `responseId`. Question fields include `description`,
`kind`, `required`, `allowFreeText` and options; answers use `selectedOptionKeys`.
Additional authority or Task fields in these payloads are rejected.

## Worker calls

All Worker commands use the existing schema-2 owner envelope:

```json
{"schemaVersion":2,"taskId":"1","attemptId":"2","generation":"3","ownerEpoch":"4","command":"notifications","after":"0"}
```

Persistent transport wraps each call as `{ "id": "call-key", "request": ... }`
and each response as `{ "id": "call-key", "response": ... }`. Notifications are
**nonblocking correlated polls**. The CLI does not send unsolicited events.
The bridge `control --stream` only copies bounded frames; it owns no journal.
Concurrent dispatch is bounded to eight calls; interrupt bypasses this queue.

`notifications` returns exactly:

```json
{
  "ok": true,
  "notifications": [{
    "sequence": "1",
    "sessionGeneration": "1",
    "roundKey": "round:1",
    "frame": {
      "protocol": "seven.agent-adapter/v1",
      "method": "interaction.received",
      "key": "adapter:3",
      "replyTo": "supervisor:3",
      "payload": {"roundKey":"round:1","interactionKey":"question:1","responseId":"101","status":"RECEIVED"}
    }
  }],
  "through": "1",
  "hasMore": false,
  "activityGap": false,
  "activityDroppedThrough": "0"
}
```

Read/poll never removes frames. After private Worker spool persistence, send
`notifications-received` with `through` as a decimal string. Its response is
`{ok:true,through:"1"}`. ACK is monotonic and idempotent, with current-owner
validation. A cursor ahead of the produced sequence is rejected. A poll behind
the acknowledged prefix starts at that prefix. Pages are bounded to 48 KiB;
`hasMore` indicates remaining pages. The critical window holds up to 128 events
and 4 MiB; an oversized event or saturation fails closed with an overflow snapshot
and cancellation rather than silently evicting unpersisted questions/receipts.

Safe message/plan/activity frames are merged into the same notification polls,
using one sequence with critical events, then follow the Worker spool and Server
PublishEvent path. They occupy a separate lossy eight-event notification window
and cannot evict critical frames. An existing eight-event state view remains too.
`activityGap` reports activity coalescing after the effective requested/acknowledged
cursor; `activityDroppedThrough` is the highest evicted activity sequence. It can
refer to a later page and must never advance the consumer cursor: only `through`
does that. Retained frames are sorted, and gaps are intentional activity loss,
not permission to skip critical events. ACK removes retained frames from both
windows after spool persistence. Full projected activity frames stay below the
64 KiB protocol-frame boundary. Activity is not synchronously logged by the round
reader, so a slow stdout consumer cannot block it on activity logging.
Questions retain every typed field; credential redaction only changes display
title, description and option-label text.

`agent-control` adds `currentRound`, decimal-string `sessionGeneration`, `method`
and exact SDK `payload`. A successful write returns
`{ok:true,controlKey:"supervisor:3",status:"WRITTEN"}`. The subsequent correlated
notification establishes local delivery. `RECEIVED` never means platform
authorization or an independently verified application of a message. Native
handlers own safe-point application; a new execution activity is a separate fact.
During `PAUSING`, controls return `SUPERVISOR_PAUSING`; the parent retains the
Server's SAVED delivery. No queue, successor Task, or resume authority is invented.

`state` exposes `currentRound`, `sessionGeneration`, `interactionActive`,
`pendingNotifications` (decimal-string count), `notificationSequence`,
`notificationReceivedThrough`, `notificationOverflow`, `pendingInteractions`
(logical keys), and `controlReceiptCount` (decimal-string count). Full frames are
read through notifications instead of expanding the state response indefinitely.

## Hot waiting and capture

An execution admits one unresolved question request at a time. That request may
contain up to three questions. A concurrent different local ask returns
`ADAPTER_INTERACTION_BUSY` before notification persistence; same-key identical
retries reuse the existing request. The SDK enforces the same pre-frame rule.
Supervisor defensively does not admit a second unsolicited native request and
exposes `interactionAdmissionError`; it preserves the first request and running
Task. No multi-question-request cold-resume support is claimed.

Cold restore uses a Server-validated private pair:
`interactionQuestionRequest:{interactionKey,questions}` and
`interactionResponse:{interactionKey,responseId,answers}`. These fields are not
accepted from public WorkItem JSON. The Worker validates and copies them only
from its authenticated restore-key response after checkpoint verification.
On the first resumed round the Supervisor validates the complete question and
answer maps, binds the current round, and registers one pending request without
publishing `interaction.request`. It does not mark the saved response accepted
or resolved and emits no receipt from private input alone. A normal
`interaction.respond` must match that saved response ID and canonical answers;
only the correlated native `interaction.received` resolves delivery. Later
correction rounds do not re-register or resend the cold pair. Neither native
frames nor local `interaction ask` can replace the expected saved answer.

`activeElapsedMillis` and `interactionWaitingMillis` are decimal-string cumulative
durations. Outstanding questions pause active accounting; accepted local answers
or correlated RECEIVED answer receipts resume it when all questions are resolved.
Neither duration grows while the round is inactive or PAUSING. Accounting persists
across rounds in the same Supervisor. These are local observations; the parent
owns trusted active timeout enforcement and a separate absolute resource cap.
Lease/watchdog enforcement continues during waiting and capture.

Hot wait defaults to ten minutes. The operator/image environment variable
`SEVEN_INTERACTION_HOT_WAIT` accepts a positive Go duration up to 24 hours. No Task
field controls it. Clock/timer interfaces are injected in deterministic tests.
Expiration enters `PAUSING_PREPARE`, preserving the native process and unresolved
ask. State exposes a stable `suspendControlKey`. The trusted Worker first reserves
the question on Server using the current round, session generation and this key.
Only `RESERVED` permits `interaction-suspend-commit`; `ANSWER_WON` instead calls
`interaction-suspend-abort`, keeps the pump alive and delivers the saved answer in
the same round. Both commands use the existing identity envelope plus
`currentRound`, decimal-string `sessionGeneration` and
`payload:{"suspendControlKey":"suspend:1:1"}`. Repeating the same decision is
idempotent; the opposite decision is rejected. Commit sends the existing SDK
`execution.suspend`, entering `PAUSING`; no new SDK wire method is introduced.
Direct `agent-control` execution.suspend now only prepares. Unsupported/failed
suspension makes no capture-ready claim. An unconfirmed suspend cannot become
evaluation or completion. Abort does not immediately re-arm the expired timer
against the same pending question; bounded Worker active/absolute caps remain.

After a correlated native `execution.suspended`, the Supervisor closes/reaps
the Adapter, saves the private `session.json`, enters `INTERACTION_SUSPENDED`,
and holds without returning from the harness loop. It runs no Result Processor,
completion proposal or end hook while held. Use `interaction-capture` with the
normal owner envelope plus exact `currentRound`/`sessionGeneration`:

```json
{
  "ok": true,
  "capture": {
    "purpose": "INTERACTION_SUSPEND",
    "attemptId": "2",
    "currentRound": "round:1",
    "roundKey": "round:1",
    "sessionGeneration": "1",
    "adapterSessionGeneration": "1",
    "sessionDigest": "sha256-of-private-reference",
    "suspensionReferenceDigest": "sha256-of-private-reference",
    "sessionArchiveFile": "session.json",
    "adapterStopped": true,
    "quiescenceHeld": true,
    "suspendControlKey": "supervisor:3",
    "captureReadyAt": "2026-09-09T00:00:00Z"
  }
}
```

Capture is a query/coordination contract, not archive creation or trusted
quiescence proof. The native PID and container assertions remain untrusted.
The Worker must independently hold its stopped/paused workspace guard throughout
packing, encryption, upload and readback. The Worker supplies the persisted
InteractionID, checkpoint purpose and evaluationCycleID=0. It captures both
`session.json` and the admitted native session files, then confirms reliable
checkpoint publication before interrupt/cleanup. The parent Runtime returns its
dedicated Suspend outcome. The CLI does not exit normally just because an Adapter
claims suspension. Archive failure exposes `captureError` and stays PAUSING.

Required parent wiring is the session notification/spool pump, active-budget
accounting, `InteractionSuspend(ctx, channel, evidence)`, trusted capture guard,
checkpoint publication, and dedicated Suspend outcome. Those implementations
are outside this CLI slice.

## In-runtime question command

`seven-worker-cli interaction ask --request FILE` uses the current
`SEVEN_CONTROL_SOCKET`. FILE contains only `{interactionKey,questions}` in the
SDK question shape. The Supervisor supplies the current round/session and rejects
caller-selected Task, Attempt, owner or session identities. The command waits on
the same runtime and prints the accepted `{roundKey,interactionKey,responseId,answers}`.
Matching retries replay the local answer; changed response IDs/answers conflict.
Local receipt is retained before waking the caller. This path also requires the
admitted question capability and does not enable Reviewer interaction.

Focused verification:

```sh
go test -race ./internal/worker/agentprotocol ./cmd/seven-worker-cli
```

The tests invoke the Python SDK through real stdio, compare the shared frame
fixtures, exercise concurrent/duplicate controls, spoofed receipts, Reviewer
rejection, UDS questions, spool ACK behavior, fake-clock waits and held capture.
They require Python 3 but make no provider/model calls.
