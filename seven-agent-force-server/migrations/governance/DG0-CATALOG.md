# Database storage governance catalog

This is the current source-governance guide. Machine-readable contracts are [table_registry.csv](table_registry.csv), [operational_sql_manifest.csv](operational_sql_manifest.csv). The catalog filename is retained for existing source references.

Internal entity and relation identities use BIGINT/int64. External tokens, process/container identities and opaque protocol identifiers use their declared types; exceptions require explicit governance evidence. Tenant relationships use authenticated organization identity. No database foreign keys are added.

Operational SQL must use its declared dialect and registered table identity. Inventory commands, jobs, listeners and database-client scripts; local-only commands omitted from a source distribution must not remain as stale inventory rows. Keep the corresponding static tests enabled.

Before a schema change, verify publication status and the actual migration/table registry. For unpublished Workforce migrations follow root AGENTS.md. A local alignment path must be idempotent, transactional and verifiable. Do not copy/dual-write tables or mutate an existing database merely to satisfy a static test.

Validate repository/Facade reads and writes, relation checks, transaction boundaries, scope isolation and forward-only recovery where applicable. Database integration requires explicitly selected disposable infrastructure; skipped tests do not establish integration success. Original private execution reports are not part of this source guide.
