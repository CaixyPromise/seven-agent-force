# Authorized Git delivery runbook

Scope: authorized Git delivery operations.

This runbook covers the AI Native `request_git_delivery` integration with the existing DeliveryRequest/DeliveryAttempt implementation. It does not replace the Delivery runbook or create a second Git executor.

## 1. Operator-visible contract

The user chooses an existing Repository Connection, immutable base revision, result branch and one delivery mode when creating the Task:

- `NONE`: retain platform result only; no Git authorization is accepted.
- `NEW_BRANCH`: non-force push and remote-ref readback.
- `NEW_BRANCH_AND_DRAFT_REVIEW`: the same push/readback followed by Draft PR/MR creation and readback.

For Git modes, authorization is either:

- `AUTO_AFTER_GATES`: allowed only when the creator has `workforce:delivery:approve`; the Server freezes the authorizing principal and Git Plan identity.
- `ASK_AFTER_GATES`: show one decision card only after the ResultCommit is frozen and completion-check/Independent-Review gates permit delivery.

The Agent cannot change repository, remote, base, branch, mode, force policy, credential or Review policy. `request_git_delivery({summary})` saves an Intent and returns `SCHEDULED`; it never performs push directly. A frozen Git Plan may still produce the same DeliveryRequest if the Agent does not call the tool.

## 2. Preflight before any real remote write

Record only safe identities and never copy credentials into commands, logs or this report:

1. Pin the source commit and exact Worker/Adapter image digests.
2. Confirm the Server, Web, Worker, PostgreSQL, RabbitMQ and Docker environment is the intended isolated acceptance environment.
3. Confirm the Repository Connection is the dedicated acceptance connection and has verified READ/PUSH/CREATE_REVIEW capabilities as required.
4. Resolve and record the exact repository display identity, immutable base commit, unique `codex/` result branch and `NEW_BRANCH_AND_DRAFT_REVIEW` mode.
5. Confirm non-force policy, Draft PR only, no auto-merge and the planned branch-retention policy.
6. Confirm the explicit real Provider budget and that the configured `local-api` profile is the intended one. Do not silently substitute another Provider or Git repository.
7. Inspect the remote first. If the proposed branch or matching open PR already exists with an unrelated commit, stop instead of overwriting or inventing another target.

The completed bounded acceptance used:

```text
Repository: CaixyPromise/seven-workforce-batch5-acceptance
Branch:     codex/batch5-live-20260912
Mode:       NEW_BRANCH_AND_DRAFT_REVIEW
PR:         Draft; never auto-merge
Force:      prohibited
Budget:     at most 25 Provider invocations
Retention:  keep the acceptance branch and Draft PR as evidence
```

If the connection or repository is unavailable, record the real acceptance as `NOT RUN` or `BLOCKED`; do not use another remote without explicit authorization.

## 3. Expected authoritative sequence

```text
tool.request(request_git_delivery)
→ Delivery Intent persisted
→ tool.result(SCHEDULED) returned to Adapter
→ Agent finishes
→ Finalizer freezes ResultCommit/ResultTree
→ completion check passes
→ Independent Review passes or reaches an authorized existing human closure
→ exactly one DeliveryRequest links to the Intent/default action
→ exact AUTO_FROM_TASK_PLAN or DELIVER decision
→ DeliveryAttempt performs non-force push
→ remote ref equals DeliveryCommit
→ Draft PR operation runs once
→ PR readback matches source branch, base branch and provider identity
→ conversation card shows the final safe result
→ temporary Git credentials, runner, gateway, networks and directories are cleaned
```

Each arrow is a separate fact. In particular, Task success is not Delivery success; `SCHEDULED` is not approval; push exit code zero is not remote readback; branch readback is not Draft PR readback.

运行态诊断时先区分三层事实：执行合同中的 `allowedAdapterTools` 证明平台确实下发；conversation 中的 `Platform tools ready: ...` 证明原生会话接受了冻结目录；非 `platform-default` 的 Delivery Intent 及其 `adapter_received_at` 证明 Codex 真实调用工具并收到 `SCHEDULED`。缺少后两项时，平台默认 Git Plan 仍可能成功交付，因此不能仅凭远端 PR 倒推出 Dynamic Tool 已工作。

DeliveryRequest 或 Draft PR operation 到达权威终态后，投影器必须幂等收敛关联 Intent：完整 Draft PR readback 成功为 `DELIVERED`；确定失败为 `DELIVERY_FAILED`；结果无法确认保持 `OUTCOME_UNKNOWN`；用户跳过和取消分别为 `SKIPPED`、`CANCELLED`。若交付卡已成功而 Intent 仍停留在 `WAITING_*` 或 `DELIVERY_RUNNING`，这是投影收敛故障，需要重跑同一 request projection/readback；不得直接改表制造一致。

## 4. User decisions and retries

- `DELIVER` binds the exact DeliveryRequest version and frozen target. It authorizes both push and its configured Draft PR step; do not ask again between those steps.
- `SKIP_THIS_RESULT` closes only the current result's request and clears its pending action. It does not delete the Task result or change Task success.
- Reopen only through the task-scoped restart endpoint with the current ResultCommit. The Server must resolve the frozen target and reject a stale or already-delivered result.
- A stale card refreshes the current request; changing the idempotency key must never be used to defeat version or decision conflict.
- A provably pre-write failure may create a new DeliveryAttempt after explicit user retry.
- `OUTCOME_UNKNOWN` requires remote readback first. If readback cannot prove whether a push or PR was created, keep the uncertain state and do not show an ordinary retry action.
- If the branch already points to DeliveryCommit, converge to success without pushing again. If push succeeded and PR remains incomplete, resume only the PR operation.

## 5. Authorization and fencing checks

At Task creation and again before external write, verify tenant, Task, current result, Repository Connection, config/credential authorization version, Git Plan digest, base commit, destination branch and current permission. Revoked user access, reauthorized connection, changed target identity, cancelled Task or digest mismatch must prevent automatic authorization consumption and return to an explicit safe decision or terminal rejection.

Browser requests may carry only Task/DeliveryRequest identity, expected version, decision, reason and idempotency key. They must never supply remote, repository, branch, refspec, force flag or credential. Cross-tenant reads and actions return the existing not-found projection.

## 6. Resource and secret cleanup

After terminal readback, inspect by exact Task/Delivery identity:

1. Agent and Reviewer containers are already subject to their own lifecycle; Delivery retry must not rerun either.
2. Delivery runner and target-fixed egress gateway containers no longer exist.
3. Delivery-only internal/outbound networks and temporary result directories no longer exist.
4. HTTPS AskPass files, SSH keys, known-hosts material and any other task-scoped credential copies are gone.
5. Cleanup journal has no pending entry for this Delivery; capacity is released only after resource non-existence is confirmed.
6. Artifact, immutable Checkpoint, Delivery readback and conversation history remain readable according to retention policy.

Do not use global Docker prune, broad directory deletion or database edits to manufacture a clean result. Cleanup failure does not rewrite a successful Task or verified remote Delivery; it remains an operational failure with retained capacity and retry state.

## 7. Evidence ledger

Record the following in the target-environment verification report without secrets or raw Provider/Git bodies:

- source checkpoint/worktree identity and exact image digests;
- Task, Turn, Attempt, Intent, DeliveryRequest/Attempt and PR-operation safe IDs;
- Git Plan authorization mode and decision type;
- ResultCommit, DeliveryCommit, remote ref and PR/MR URL/provider identity;
- actual Provider invocation count;
- gate commands, exit codes and non-skipped database tests;
- browser action/card observations;
- remote branch and Draft PR readback;
- exact temporary-resource cleanup observation;
- limitations and one final `VERDICT: PASS|FAIL|PARTIAL`.

Record target-environment verification independently from source and unit checks.
