# SQL and table governance registry

Use [table_registry.csv](table_registry.csv), [future_table_registry.csv](future_table_registry.csv) and [operational_sql_manifest.csv](operational_sql_manifest.csv) as the machine-readable source contracts. See [storage governance](DG0-CATALOG.md) for application identity and relationship rules.

New physical tables and rename targets must use registered lower snake_case names. Database FOREIGN KEY/REFERENCES declarations are forbidden; application transactions and validation own relationship integrity. Existing column identity is preserved. Static tests inspect source migrations and operational SQL declarations.

Schema alignment is a controlled forward operation: verify migration publication status, prepare an authorized backup/recovery path, apply the reviewed migration and validate existing reads/writes. Do not enable dual-write or a hidden compatibility table. Never infer permission to mutate a database from a passing source check.

## Command SQL dialect contracts

Every production Go source file below `cmd/` containing a raw SQL statement
must begin with exactly one source-controlled declaration:

```go
// sql-governance: postgres-capable
```

or:

```go
// sql-governance: mysql-only
```

`postgres-capable` commands must render all mixed-case identifiers through an
explicit reviewed renderer and rebind placeholders for the active provider.
`mysql-only` commands must reject a non-MySQL datasource before they create or
change any data. `TestDG1CommandRawSQLDeclaresDialectContract` enforces the
declaration for every non-test command source so new operational SQL cannot
silently bypass the dual-database contract.

## Operational SQL manifest

`operational_sql_manifest.csv` is the bounded inventory for raw SQL outside
repositories: production commands, future job/listener sources, and executable
`scripts/` or `script/` sources. Every SQL-bearing source must have exactly one
manifest row with a dialect and `registry` table contract. Go command/job/
listener sources also retain the source-level `sql-governance:` declaration;
the checker requires it to match the manifest.

The existing audit scripts are deliberately `mysql-only`: they drive local
MySQL federation fixtures and use MySQL syntax. They remain listed rather than
being silently treated as PostgreSQL-capable. Pure unit-test/assertion scripts
are excluded only when their name identifies them as tests and they do not
invoke a database client; a test script that invokes a database process remains
in the inventory.
