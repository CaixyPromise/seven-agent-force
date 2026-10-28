# Serial development work items

A work item coordinates a planning Task and an implementation Task for one Project. The Agent produces a structured plan first; a user can edit it, request a new plan, or approve an exact revision. Approval records the decision and creates the implementation Task in the same transaction. Existing Task execution, result verification, independent Review and delivery remain responsible for their own lifecycle.

## Operator workflow

1. Open a Project and choose **先出方案再实施**. Enter the desired outcome, select a model and compatible Runtime, and choose a delivery preference.
2. The planning Task may ask questions. Open its task page to answer or provide additional context.
3. Review the generated plan. Edit it without a model call, request a new version with feedback, or approve it. Unresolved blocking questions prevent approval.
4. The implementation Task uses the approved plan and frozen Project version. Inspect its checks, independent Review and results in the Task page. A successful implementation and Git delivery are separate states.

The current Web form uses the same model and Runtime for both stages. The operator API accepts separate selections. Usage policy defaults to `OBSERVE`: known usage and calls are recorded when available; unknown usage is counted separately. This policy does not pause execution for cost, token or call limits, and does not estimate charges.

Planning runs in a separate writable workspace. The frozen Project source is available through a read-only input; a compatible Worker must declare `planningSource=v1`. Planning cannot request controlled external execution or Git delivery, and does not run the Project's implementation completion check or independent code Review. Projects with configured Hooks are currently rejected for planning until a compatible stage-aware Hook contract exists. The Project's existing checks, Review and delivery policies apply to implementation.

## Operator HTTP API

These endpoints require an authenticated operator session and an organization scope. They do not extend Open Application or MCP. Internal IDs and versions in JSON and URLs are decimal strings; never convert them to JavaScript numbers. Mutating requests other than cancellation require an `Idempotency-Key` header. Replaying the same key with changed input conflicts.

| Method and path | Request | Required permissions |
| --- | --- | --- |
| `POST /api/workforce/projects/{projectId}/work-items` | Create request below | `workforce:work-item:create` and `workforce:task:create` |
| `GET /api/workforce/projects/{projectId}/work-items` | No body; returns at most 100 items | `workforce:work-item:query` |
| `GET /api/workforce/work-items/{id}` | No body | `workforce:work-item:query` |
| `POST /api/workforce/work-items/{id}/plan-edits` | Version guard plus `plan: object` | `workforce:work-item:plan:approve` |
| `POST /api/workforce/work-items/{id}/plan-revisions` | Version guard plus `feedback: string` | `workforce:work-item:plan:approve` and `workforce:task:create` |
| `POST /api/workforce/work-items/{id}/plan-decisions` | Version guard plus `decision: "APPROVE"` | `workforce:work-item:plan:approve` and `workforce:task:create` |
| `POST /api/workforce/work-items/{id}/cancel` | `expectedVersion: string` | `workforce:work-item:cancel` |

A create body has `instruction: string`, optional `name: string`, `planSelection` and `implementationSelection`, optional `referenceAssetIds: string[]`, `deliveryPreference: "PROJECT_DEFAULT" | "SAVE_ONLY" | "ASK_AFTER_GATES"` (default `PROJECT_DEFAULT`), and `usagePolicy: { "mode": "OBSERVE" }` (default `OBSERVE`). Each selection contains `modelSelection: { credentialId: string, model: string }`, `runtimeReleaseId: string`, and optional `executionTimeoutSeconds: integer`.

The version guard is `{ "expectedVersion": "…", "planRevisionId": "…", "contentDigest": "…" }` from the current work item and plan revision. A stale guard yields a conflict; retrieve the latest work item before deciding again. Creating a new planning revision after a failed initial PLAN with no plan uses `planRevisionId: "0"` and an empty digest.

A plan uses schema `seven.force.development-plan/v1` with `title`, `summary`, `goals`, `nonGoals`, `approach`, `affectedAreas`, `steps`, `acceptanceCriteria`, `risks`, `openQuestions` and `markdown`. Areas have `path` and `reason`; steps have `key` and `description`; questions have `key`, `question`, `blocking` and optional `resolution`. The platform owns the frozen source identity; a plan cannot authorize its own source.

The current slice does not expose plan-history diffing, stage-specific usage breakdowns, budget alerts, automatic failed-implementation retry, or separate model selection in the Web form. Follow the linked Tasks for progress, interactions, checks, Review and results.
