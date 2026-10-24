# Migrations

`seven-agent-force-server` uses `goose` for schema migrations.

Rules for this directory:

- One table set has one migration owner. Do not let Java Liquibase and Go goose
  both mutate the same table set.
- Use SQL migrations by default. Only use Go migrations when SQL is genuinely
  insufficient.
- Baseline existing schemas before handing migration ownership to Go.
- Keep migration directories separated by driver.

Directory layout:

- `migrations/mysql`
- `migrations/postgres`

## SSO Provider v1

The SSO Provider v1 migrations create the OIDC runtime tables and seed the
SSO client admin menu/permissions. The schema intentionally uses camelCase
column names to match the Go sqlx mappings.

Common commands:

```bash
make migrate-status
make migrate-up
make migrate-down
make migrate-create NAME=add_sso_client
go run ./cmd/migrate inspect
go run ./cmd/migrate bootstrap
```

Bootstrap config:

```yaml
datasource:
  bootstrap:
    enabled: false
    mode: manual # manual | startup | both
    migrationsDir: ""
    versionTable: goose_db_version
    changeOwner: goose
    baselineVersion: ""
    allowLegacySync: false
```

Behavior:

- `inspect` only detects schema state and prints the recommended action.
- `bootstrap` applies the schema-state orchestrator:
  - `EMPTY_SCHEMA`: requires `baselineVersion`, runs `up-to` then `up`
  - `MANAGED_SCHEMA`: runs `up`
  - `LEGACY_UNMANAGED_SCHEMA`: requires `allowLegacySync=true`, writes baseline version records, then runs `up`
- Startup auto-bootstrap is disabled by default and only runs when
  `datasource.bootstrap.enabled=true` and `mode=startup|both`.
