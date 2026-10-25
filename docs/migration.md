# Database migration

Workforce uses PostgreSQL. See the [database schema and initialization guide](database-schema.md) for the complete clean-install and schema-only export procedure.

## PostgreSQL clean install

Use the real `seven-agent-force-server migrate` subcommand. Apply `migrations/postgres-baseline` once to a new empty database, then `migrations/postgres`. Never apply the baseline to an existing business database. The source form is `go run ./cmd/seven-agent-force-server migrate` from the Server directory.

`inspect/bootstrap` currently belongs to the MySQL bootstrap implementation; do not use it to initialize PostgreSQL. Configure the PostgreSQL datasource explicitly, use baseline/up/status/version, and finish normal application initialization afterward.

## Existing installations

Back up and verify the selected database before applying forward upgrades. Keep migration/version-table ownership with Goose and stop incompatible dispatch before schema changes. Do not force version records, rerun a baseline or use destructive Down to bypass a failed precondition.

The baseline includes only source-controlled system seed definitions. The separate `schema/postgres.sql` contains no rows or version ledger and cannot replace baseline plus upgrade migrations.

## Other database support

Framework-derived MySQL migrations are retained for their supported foundation modules. They are not a complete Workforce installation option. Follow the actual module/driver contract; do not infer support from the presence of a SQL directory.
