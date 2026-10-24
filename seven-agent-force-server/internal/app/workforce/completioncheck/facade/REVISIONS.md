# Runtime completion-check revisions

## Parent-owned integration

```go
revisions := completionapp.NewRevisions(
    db, ids, completioninfra.NewRevisionStore(), principals,
    safePointAuthority, revisionTurnCreator,
)
completionHandler.WithRevisions(revisions)
```

`infrastructure.RevisionSchemaSQL` is the full additive schema fragment for the
parent-owned unpublished migration and alignment path. It creates only
`workforce_completion_check_revisions`, uses injected Snowflake/BIGINT identities,
a structured principal relation, bounded JSON/text/digests and no foreign keys.
No module/main/global migration was changed by this implementation.

## User request

GET `/workforce/tasks/:taskId/completion-check-revisions` returns the dedicated
`RevisionView`: `{expectedVersion: decimalString, completionCheck: Input,
pending: Revision|null, allowedActions: []|["REQUEST_REVISION"]}`. Input, version
and pending state are read in one Task-locked transaction. The action requires
continue permission, RUNNING, active Attempt, no pending revision, and both
safe-point authority and continuation creator dependencies injected. Query-only
users receive no mutation action. This is separate from conversation actions
and result-stage `ADD_VERIFICATION`. Missing handler wiring returns safe 503.

POST `/workforce/tasks/:taskId/completion-check-revisions` uses the existing
`workforce:task:continue` wrapper and authenticated organization/user context.
Body is `{expectedVersion:decimalString,idempotencyKey,completionCheck:Input}`.
expectedVersion is the current Task version exposed by completion-check summary.
The application requires RUNNING and an active source Attempt, locks the Task,
and creates at most one PENDING revision per Task. Identical request replay
returns the original revision; a different body with the same key conflicts.
A second pending request conflicts. Requesting a revision does not mutate the
current Task version, Turn input, processor, baseline or Worker state.

The complete proposed Input passes existing `Freeze` and `FrozenDigest` plus
additive validation. An existing Command or Script must remain exactly the same
after canonical normalization. Existing Instructions may only
grow at a newline boundary. Old selected reference IDs and frozen bytes must
remain; newly selected references must already be tenant-authorized members of
the original frozen Task input. A previously NONE executable may gain a Command
or Script. No-op requests and replacement/removal are rejected. This schema does
not claim arbitrary shell text replacement is additive.

Mandatory gates are not fields in this request; unknown fields are rejected.
The application cannot downgrade mandatory policy through optional check input.

GET `/workforce/tasks/:taskId/completion-check-revisions/pending` uses Task query
permission and returns the pending revision or null. Both routes use the existing
Framework response envelope. The revision carries decimal-string id/version,
revisionNumber/sourceVersion/sourceTaskTurnId, status, frozen completionCheck,
checkDigest/baseDigest, historical sourceBaselineResultId and appliedTurnId.
The private principal/lease/Attempt/reference object metadata is not returned.

## Pending read and safe-point application

`facade.PendingReader.PendingRevisionTx(ctx, tx, tenant, task, sourceTurn)` runs
inside an already authorized Task transaction. It never claims a safe point or
consumes a pending revision. Source Turn/check-digest drift fails closed.

`facade.RevisionApplier.ApplyRevisionTx(ctx, tx, RevisionSafePoint)` requires:

- matching tenant/Task/source Turn/Attempt/revision/version/checkpoint identities;
- `RevisionSafePointAuthority.ValidateRevisionSafePoint` that locks current Task
  first, then verifies current Worker/instance/boot/generation/owner/lease/cancel
  plus independently confirmed stop/quiescence and checkpoint binding;
- `continuation/facade.CompletionRevisionTurnCreator`, a purpose-specific
  transaction adapter that consumes a genuine RESULT checkpoint once, following
  a real Processor PASS or WAIT_USER after freezeCheckpoint and before Finalizer,
  and creates a real
  continuation Turn without inventing an interaction or treating Agent output
  as authority.

The application revalidates additive input and reference bytes at Apply, invokes
the continuation port, then reads the new current Turn back. Check digest,
original source commit/identity, execution snapshot, processor identity and task
instruction must match the expected frozen values. Old baseline rows and source
Turn remain untouched; the new Turn must clear its baseline-result pointer so
the changed check can evaluate against the same original source snapshot.
Only then does the application move PENDING to APPLIED and append the safe
`completion_check.revision.applied` Task event in the same transaction. Every
error requires caller rollback, including errors after a new Turn was created.

Missing safe-point authority or continuation implementation returns
`COMPLETION_CHECK_REVISION_UNAVAILABLE`; there is no no-op or in-memory backend.
The parent owns Worker safe point, runtime restore and module wiring. The concrete
authority and creator reuse genuine RESULT policy; they must not fabricate PASS,
borrow an interaction checkpoint, or relabel purposes to bypass validation.

## Verification and evidence boundary

```sh
COMPLETION_CHECK_REVISION_TEST_POSTGRES_DSN='postgres:///seven_workforce_batch3_20260909_qa?host=/tmp&sslmode=disable' \
  go test ./internal/app/workforce/completioncheck/... -run 'TestAdditiveRevision|TestRevision' -count=1
```

The integration test creates an isolated schema and runs RevisionSchemaSQL twice,
uses the real pgx repository, existing structured principal resolver and Task
eventlog, and cleans up its schema. Seven PostgreSQL cases cover pending
uniqueness/replay, concurrency, authorization/version/additive rejection,
reference fencing, baseline-preserving Apply, unsafe/missing authority and
atomic rollback after continuation failure or identity drift. Safe-point and
continuation implementations in this fixture are explicitly test-only SQL ports;
their success is not production checkpoint/quiescence/restore acceptance.
