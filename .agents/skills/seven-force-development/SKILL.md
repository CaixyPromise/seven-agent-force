---
name: seven-force-development
description: Implement or review Seven Agent Force Server and Web extensions using existing modules, authenticated scope, string IDs, API contracts and route conventions. Use for project secondary development; use seven-force-sdk-development for Runtime SDK or Adapter changes.
---

# Seven Agent Force secondary development

Read root AGENTS.md and [development guide](../../../docs/development.md). Work in the active Server/Web directories; preserve unrelated work. Skill invocation authorizes only the user's requested change, not commits, pushes, migrations, deployments or live Provider calls.

## Find the existing path

Trace one concrete user action through page, API adapter/normalizer, mounted controller, application transaction, domain policy and infrastructure. For a project feature, start with Server internal/app/workforce/project/, internal/app/workforce/module.go, Web src/api/projectController.ts, projectContract.ts and src/app/workforce/projects/page.tsx. Verify current paths before editing. Cross-module access uses facade contracts.

Identify authenticated actor, organization scope, permission, idempotency/revision, frozen input and error presentation before adding fields or actions. Do not invent route names, DTOs or state transitions from a UI mockup. Keep int64 entity IDs as decimal strings in browser/JSON/queue boundaries.

## Implement across the contract

Follow the guide's project-search walkthrough as a navigation exercise, not a requirement to modify unrelated project functionality. Put state-transition rules in domain/application; controllers decode and authorize, infrastructure persists, Web normalizers validate boundary data. Register new dependencies and routes in the real module assembly. Auto-scanned page presence alone does not establish permission/menu visibility.

For UI behavior, cover loading, empty, denied, failed and refresh states; action availability comes from server authority. Prefer existing components. Keep task completion, review and Git delivery status distinct. Do not use new task execution to test a display-only change.

## Verify and hand off

Start with affected domain/controller/normalizer tests. Follow AGENTS.md for broader checks; use real rendered evidence for requested visual acceptance and label unrun browser paths. Database integration uses explicitly configured disposable environments, never implicit local business credentials. Update public guides and protocol references when contracts change. Report exact changed paths, checks, and unresolved behavior; do not present source/build success as live task acceptance.
