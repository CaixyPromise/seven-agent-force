# PostgreSQL clean-install baseline

This directory is only for a brand-new, empty PostgreSQL database. The snapshot
contains the complete schema and seed state through version `20260719110000`.
It deliberately excludes Goose's own version table.
During the snapshot transaction it records every historical PostgreSQL
migration represented by the snapshot. The regular migration directory contains
a no-op marker at the same cutoff version, so switching back to the regular
upgrade chain does not require `-allow-missing`.

Run the clean baseline first:

```bash
go run ./cmd/migrate -dir migrations/postgres-baseline up
```

Then run the regular upgrade chain so migrations newer than the snapshot are
applied:

```bash
go run ./cmd/migrate -dir migrations/postgres up
```

Never run the baseline directory against an existing database. Existing
databases keep using `migrations/postgres` and their current Goose version
records. The snapshot migration is forward-only; its Down section does not drop
application tables or seed data.
