# Independent Reviewer runbook

## Configuration

Review configuration lives on the current immutable ProjectVersion:

- Project: `reviewPrompt` only.
- Task: `reviewEnabled` only; omitted means enabled.
- Platform: fixed report schema, timeout, retry and revision limits.

There are no review-policy administration pages or bootstrap jobs. To change project review guidance, update the Project. New Tasks receive the new ProjectVersion ID; existing Tasks remain pinned to the old version.

## Normal operation

1. Confirm the Task's completion check and Finalizer succeeded.
2. Confirm `workforce_tasks.review_enabled`.
3. Resolve `workforce_tasks.project_version_id` and inspect the safe `review_prompt` from `workforce_project_versions`.
4. Confirm one ReviewRun exists for the finalization occurrence.
5. Confirm a compatible admitted runtime and a `REVIEW` Provider are available.
6. Follow ReviewRun status through assignment, execution, canonical validation and closure.

The preferred runtime is the Agent runtime used by the completed Turn. Routing falls back only when it is incompatible with review execution. The Provider is selected independently for the REVIEW role.

## Waiting states

- `WAITING_REVIEW_RUNTIME`: no compatible admitted runtime is currently available.
- `WAITING_REVIEW_PROVIDER`: no active REVIEW Provider satisfies structured output.
- `WAITING_REVIEWER_CAPACITY`: compatible Workers are at review capacity.
- `WORKER_DISK_PRESSURE`: compatible Workers need disk capacity.

These are recoverable scheduling states. Do not ask a Project user to choose infrastructure.

## Failure handling

- Duplicate finalization: verify the existing ReviewRun is reused.
- Worker restart: recover only the exact persisted container and owner identity; otherwise fence it.
- Provider outcome unknown: reconcile the original invocation; do not switch Providers and resend.
- Invalid report: keep the result gated and record the stable validation reason.
- Automatic revision exhausted or no material progress: enter the existing human-review closure path.
- Task cancellation: cancel active ReviewRuns, fence late results and retain historical reports.

## Diagnostic queries

Use IDs from the Task detail or audit view. Do not print Provider secrets or raw model bodies.

```sql
SELECT t.id, t.project_version_id, t.review_enabled, p.review_prompt
FROM workforce_tasks t
JOIN workforce_project_versions p
  ON p.id=t.project_version_id AND p.tenant_id=t.tenant_id
WHERE t.id=$1;

SELECT id,status,task_turn_id,finalization_run_id,result_commit,
       runtime_release_id,provider_profile_id,provider_model,wait_reason_code,error_code
FROM workforce_review_runs
WHERE task_id=$1
ORDER BY id DESC;
```

## Completion evidence

A review path is complete only when the canonical ReviewRun result, Task/Turn state and Artifact/Delivery gate agree. A model message, process exit, health endpoint or locally generated report alone is not acceptance evidence.
