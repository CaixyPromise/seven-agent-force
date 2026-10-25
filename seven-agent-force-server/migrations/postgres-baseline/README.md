# PostgreSQL clean-install baseline

For a brand-new empty PostgreSQL database only. This baseline represents source schema/system seeds through version `20260719110000` and registers the covered Goose versions inside the migration transaction. Then apply the regular upgrade chain.

From the Server source directory, with a private PostgreSQL configuration:

```sh
go run ./cmd/seven-agent-force-server migrate --config-dir /path/to/config --dir migrations/postgres-baseline up
go run ./cmd/seven-agent-force-server migrate --config-dir /path/to/config up
go run ./cmd/seven-agent-force-server migrate --config-dir /path/to/config status
```

Release packages provide `bin/seven-agent-force-server` and both migration directories. PostgreSQL does not use the MySQL-only inspect/bootstrap path. See docs/database-schema.md in the source checkout or release package.

Never run this baseline on an existing business database. Existing installations use regular forward migrations. Down is intentionally rejected. The baseline omits pg_dump's PG17+ transaction_timeout setting so the same clean-install path works on PostgreSQL 16 as well as 18; it does not copy a live database's rows or roles.
