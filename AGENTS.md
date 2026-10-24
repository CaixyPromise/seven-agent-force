# Seven Agent Force repository instructions

## Scope, priority, and authorization

- These instructions apply to the repository unless a closer `AGENTS.md` provides narrower rules for its subtree. Treat files under test fixtures as data unless the task explicitly targets that fixture.
- Follow the user's stated outcome and scope. If a request conflicts with a frozen safety or identity rule below, stop and name the exact conflict instead of silently weakening either requirement.
- For review, diagnosis, or explanation, inspect and report without modifying files. A request to change or build authorizes reversible in-repository edits and proportionate local verification.
- Require explicit authorization before committing, pushing, merging, deploying, mutating shared or production data, calling a live third-party/provider service that has side effects or cost, changing credentials, or performing destructive cleanup. Never print or commit secrets, raw provider bodies, or private local configuration.
- Delegate only independent scopes when it materially improves speed or quality. Keep blocking decisions in the parent task, validate delegated results, and never treat pending work as completed evidence.

## Worktree hygiene

- Inspect Git status and relevant nested repositories before editing. Preserve unrelated tracked and untracked work.
- Limit edits and staging to the requested paths. Do not reset, clean, discard, or stash user changes unless the user explicitly asks for that exact operation.
- Prefer current source, tests, migrations, and runtime configuration over historical status prose. Preserve historical acceptance records as evidence of what was true when recorded.

## Project structure

- `seven-agent-force-server/` is the active server: the Framework Server plus Workforce modules, Worker daemon, internal `seven-worker-cli`, user-facing `seven-workerctl`, migrations, and runtime adapters. It exposes the single browser-facing API; do not create a parallel Workforce API.
- `seven-agent-force-web/` is the active operator console. It keeps the Framework setup, login, SSO, RBAC, menu, and administration behavior while adding Workforce API adapters, auto-scanned pages, and route metadata.
- `deploy/` contains local infrastructure and deployment assets. `docs/workforce/` contains architecture, ADRs, runbooks, status, and acceptance evidence.

## Architecture boundaries

Application modules live under `seven-agent-force-server/internal/app` and use these layers where needed:

- `controller`: HTTP entry points;
- `listener`: RabbitMQ entry points;
- `application`: use-case orchestration and transaction boundaries;
- `domain`: aggregates, value objects, policies, and state-transition rules;
- `infrastructure`: persistence and provider adapters;
- `facade`: stable cross-module contracts.

Entry points may call local application or facade packages. Application code coordinates domain behavior. Infrastructure implements ports owned by the domain or application side and must not own business state transitions. Cross-module calls use explicit contracts, never another module's private implementation.

## Frozen safety rules

- Control Plane code must not pass a Docker socket to API handlers or Attempt containers.
- Attempt containers expose no control HTTP or gRPC port.
- `seven-worker-cli complete` creates a CompletionProposal, never a trusted success result.
- Finalizer code recomputes ResultCommit, changed files, and hashes outside the Harness trust boundary.
- Packaging, ReviewRun, and future DeliveryAttempt retries must not rerun the Agent.
- Production configuration must use immutable runtime image references, never floating tags.

## Database identity rules

- Framework and Workforce internal database primary and relation keys use PostgreSQL `BIGINT` and Go `int64`.
- New Workforce entities use the injected Framework Snowflake `IDGen`; do not introduce another internal database ID generator.
- `tenant_id` uses authenticated `OrganizationScope.OrgID`; never persist `org:<id>` or another composed scope string as a tenant relation.
- Reuse Framework `sys_user.id`, `sys_org.id`, and other Framework `BIGINT` identities directly.
- Encode `int64` IDs as decimal strings at JSON, URL, RabbitMQ, and Web boundaries. Web code must not convert them to JavaScript `Number`.
- `TEXT` or UUID identity columns are limited to external-system identities, machine boot or process-session identities, container identities, protocol idempotency identities, and security tokens. Every exception needs a named reason and a table-and-column allowlist enforced by tests.
- Git SHAs, digests, object keys, nonces, leases, permits, tokens, and opaque Provider, Git, or Docker identities are not internal database entity IDs and retain their appropriate text or binary representation.
- Do not use `_system`, `0`, or `-1` as a tenant. System assets use an explicit scope type and nullable `tenant_id`.
- User, Worker, and Service actors use structured principals. Do not persist usernames, `system`, or `worker:<id>` in relation identity columns.
- While Workforce migrations remain unpublished, update the existing Workforce migrations instead of adding parallel migrations. Any existing local database needs an idempotent, transactional, and verifiable alignment path.
- Do not add database foreign keys. Application and domain validation, schema governance, and alignment verification enforce relationship integrity.

## Verification and evidence

- Run the smallest meaningful checks for the changed risk surface. Start with focused tests; use `make server-check` for broad server changes, `make web-check` for Web behavior or build changes, and `make check` for cross-cutting or release-sensitive changes.
- For documentation or instruction-only changes, validate structure, links, frontmatter where applicable, and `git diff --check`; application builds are unnecessary unless the text changes executable behavior.
- Do not rerun an unchanged passing check without a new failure, edit, or unresolved risk.
- Report evidence precisely: distinguish source changes, focused tests, full local checks, local integration, commit, push, deployment, structural smoke, and real business acceptance. Claim only the highest level actually observed.


## Public source scope

Only active Force source is included. Framework attribution is retained in NOTICE.
Use the documented checks; private reference baselines and acceptance records are not required.

## Development guidance

Use `.agents/skills/seven-force-development/SKILL.md` for Server/Web extensions and `.agents/skills/seven-force-sdk-development/SKILL.md` for SDK/Adapter work.
