# Interaction module integration

Cold resume carries only RESOLVED/SAVED USER_MESSAGE rows belonging to the source
Turn and source Attempt (or still unbound source-Turn messages) into the new Turn
inside the answer/resume transaction. IDs, local keys and content stay stable;
Attempt/session become NULL until the first new Pull binds them. DELIVERED rows
remain historical. Paused SEND_MESSAGE remains fail-closed in current authority.
SAVED is not proof of non-consumption: native receipt loss requires stable-ID
deduplication/restore-manifest preservation, not an exactly-once claim.

Pull also returns SAVED questions whose trusted `ResumeTaskTurnID` matches the
current authorized Turn, with `ColdResume=true`, current session and empty round.
Worker maps only these cold questions to its trusted current native round for
delivery and receipt matching. Original occurrence IDs/Attempt/session/round are
never rewritten. PublishReceipt validates the own-module cold association and
current machine authority; Ack uses `AcknowledgeCold` with that authorized Turn.
Neither a container-supplied old response ID nor an ordinary empty-round question
is sufficient authorization. No additional continuation SQL or schema is needed.

`Event.AttentionRequired` is true only for the first persisted QUESTION
occurrence (version 1). The parent EventRecorder must append its attention
occurrence in the same transaction as conversation invalidation. Replays,
answers, receipt updates and cancellation never request another attention.
This flag alone is not a notification-delivery acknowledgement.

`CloseAttemptTx` closes OPEN questions and RESOLVED/SAVED responses or messages
still awaiting native delivery for the exact attempt. It preserves SAVED versus
DELIVERED facts and excludes already consumed cold-resume responses. USER_MESSAGE
projection includes `status` so CANCELLED/SAVED is not displayed as delivered.
No table or schema changes are required for these lifecycle semantics.

The application owns pgx transactions and domain transitions. The PostgreSQL
repository only loads and persists caller-supplied state. The platform assembly owns global
migration/alignment, Task/dispatch authority, Machine authentication, runtime and
SSE wiring. `../infrastructure/schema.go` exports the complete four-table `Schema`
fragment.

```go
authority := dispatchinfra.NewInteractionAuthority(ids, principals)
service := interactionapp.New(pool, ids, interactioninfra.New(), authority, authority).
    WithEventRecorder(taskEvents).
    WithContinuation(continuationStore, continuationResumer).
    WithSummaryAuthority(authority)
handler := interactioncontroller.New(service)
handler.Mount(router, permissionWrapper)
machineHandler.SetInteractions(service)
```

All dependencies are explicit. Missing event, Machine or user authority fails
closed. Answer handling additionally requires a continuation finder, even for
short answers, because a missing implementation cannot prove no suspended
checkpoint exists. The finder and resumer execute in the answer transaction.

## Authority and lock order

`MachineAuthority.Validate(ctx, tx, Machine) (Target, error)` validates current
Worker instance/host boot, Attempt generation, owner epoch, unexpired execution
lease, current Task/Turn, cancel fences and adapter session. TenantID, TaskID and
TaskTurnID may be zero in the input: the authority derives them from the
authenticated Attempt. Nonzero inputs must match the derived target. Target must
have positive tenant/task/turn/attempt/session and structured Worker PrincipalID.

User authority receives authenticated `User{TenantID, TaskID, UserID,
CanContinue}`. The controller derives CanContinue from the security context; it
is never read from JSON. READ includes Task authorization; ANSWER and SEND_MESSAGE
require continuation mutation authorization. READ must return a resolved user
PrincipalID when serving MarkRead. Query-only users receive no allowedActions.

Target.Capabilities must come from the exact admitted image. Application checks
interaction.request/respond/received, user.message/message.received and activity
for the respective operations, and filters allowedActions against these same
capabilities. A missing capability list grants no interaction functionality.
Cold resume additionally carries the private InteractionResponse binding
`{interactionKey,responseId,answers}` to continuation; it is not a public item body.

Lock order is Task authority first, then the Task event advisory lock, then
interaction rows. All authority locks must last until the application commits.
The parent cancellation/supersession transaction can call
`CloseAttemptTx(ctx, tx, tenant, task, attempt, CANCELLED|SUPERSEDED)` after its Task
lock to update still-open question projections. The continuation occurrence gate
is `AuthorizeInteractionSuspendTx(ctx, tx, continuation.InteractionSuspendOccurrence)`.
It requires the exact open, unanswered, unexpired question and source session.

## Public routes and projections

- GET `/workforce/tasks/:taskId/conversation?before=<itemID>&limit=20` returns
  newest-first history, stable item-ID pagination, maximum 50. `cursor` is an alias
  for `before`, not a change cursor. With neither, the newest page is returned.
- GET the same path with `after=<sequence>` returns ascending changed items for
  stable-ID/version merge. An explicit `after=0` selects change replay.
  `after` cannot be mixed with `before`/`cursor`.
- GET `/workforce/tasks/:taskId/conversation/:itemId` returns one safe item.
- POST `/workforce/tasks/:taskId/interactions/:interactionId/responses` accepts
  `{expectedVersion:string,idempotencyKey,answers:[{questionKey,selectedOptionKeys,text}]}`.
- POST `/workforce/tasks/:taskId/messages` accepts `{text,idempotencyKey}`.
- POST `/workforce/tasks/:taskId/conversation/read` accepts `{watermark:string}`.

All browser responses use the existing Framework response envelope. Conversation
is `{items,nextCursor,watermark,allowedActions}`; nextCursor is null or a decimal
string. Item is `{id,version,type,body,createdAt}`, with string IDs/versions.
Question body is `{interactionId,status,version,questions,response?,deliveryStatus}`;
response is `{id,answers}`. Native keys, Attempt IDs and private session data are
excluded. Options and selectedOptionKeys serialize as arrays, including empty
arrays. USER_MESSAGE body is `{text,deliveryStatus}`; ACTIVITY and ASSISTANT_MESSAGE
body is `{text}`. There are no grants or operation controls.

Question status is OPEN/RESOLVED/EXPIRED/CANCELLED/SUPERSEDED. Delivery status is
NONE/SAVED/DELIVERED. Expiry is 24 hours after publication. Saving an answer or
message, pulling it, acknowledging delivery and observing execution continuation
are independent facts. These APIs never claim executionContinued=true.

`EventRecorder.RecordInteractionEvent(ctx, tx, Event) (sequence, error)` integrates
with the existing Task eventlog. Parent wiring records `conversation.updated`,
payload `{itemId,version,kind}` only. The event sequence is the change watermark.
History watermark is current at the read; paginated changes advance watermark
only through returned items. SSE metadata does not contain answer/message bodies.

## Machine methods and restart recovery

Publish persists a question under `(attempt_id, adapter_session_generation,
local_request_key)`, including the native roundKey. Pull returns at most 50 saved
deliveries and never marks them delivered. Delivery includes localRequestKey,
roundKey, adapterSessionGeneration, interactionId, responseId, kind, answers/text.
Question answers require a positive responseId; messages use zero. A queued user
message can have no Attempt; first authorized Pull binds it to the current
TaskTurn/Attempt/session transactionally. The Worker selects the current native
round for messages whose roundKey is empty, then verifies the corresponding
native receipt before Ack.

PublishReceipt resolves the persisted occurrence even after an earlier Ack, so a
restart between delivery ACK and spool removal can recover the exact identities.
It is not a delivery mutation. PublishEvent accepts ACTIVITY or ASSISTANT_MESSAGE
and a bounded text body; it cannot create authority or trusted execution evidence.
The parent Machine publish controller preserves authentication and returns the
input frame digest only after successful application processing. Suspended frames
are display observations; continuation owns checkpoint/stop/readback authority.

## Batch summaries

`TaskSummaries(ctx, tenant, user, taskIDs, canContinue)
(map[int64]facade.TaskSummary, error)` accepts at most 200 distinct positive IDs.
`SummaryAuthority.AuthorizeSummary` supplies authorized TaskIDs, ActiveTaskIDs and
the structured user PrincipalID in one batch. Repository aggregation reads
interaction-owned projections plus the current completion-check checkpoint with
tenant filters and no per-Task queries. Summary fields are PendingActionCount,
HasUnreadUpdates, ActivitySummary, FirstPendingActionKind and
FirstPendingActionID. The typed anchor distinguishes a conversation card from a
completion-check decision; a checkpoint ID is never presented as a conversation
item ID. Unexpired OPEN questions, waiting network operations and eligible
completion-check WAIT_USER checkpoints count only for continuation users.
MarkRead changes the read watermark; it never consumes pending actions.

## Verification

Run from `seven-agent-force-server`:

```sh
INTERACTION_TEST_POSTGRES_DSN='postgres:///seven_workforce_batch3_20260909_qa?host=/tmp&sslmode=disable' \
  go test ./internal/app/workforce/interaction/... -count=1
```

The integration suite creates a unique schema, executes the schema fragment twice,
uses the real pgx repository and Task eventlog, and drops only that test schema.
Its authority/continuation fixtures are explicitly test-only transaction ports;
they do not replace production dispatch authority or prove real checkpoint
restore. Without the DSN the single PostgreSQL integration gate is explicitly
NOT RUN. Production wiring, full migrations, runtime pump and business acceptance
remain independent parent-owned evidence.
