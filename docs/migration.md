# Database Migration

Seven Framework maintains parallel migration histories for MySQL and PostgreSQL. It uses Goose and protects the configured version table, baselines, forward-only migrations, and database-name safety checks already present in the service.

## Initialize and inspect

Configure exactly one datasource and run:

```bash
seven-agent-force-server migrate --home /opt/seven-agent-force inspect
seven-agent-force-server migrate --home /opt/seven-agent-force bootstrap
seven-agent-force-server migrate --home /opt/seven-agent-force status
```

Bootstrap must be explicitly enabled in `datasource.bootstrap` and should first be exercised against a disposable database. For an empty database, follow the inspection recommendation. For an existing database, record the current version before upgrading.

## Upgrade and rollback

```bash
seven-agent-force-server migrate --home /opt/seven-agent-force up
seven-agent-force-server migrate --home /opt/seven-agent-force version
seven-agent-force-server migrate --home /opt/seven-agent-force down
```

Some migrations are intentionally forward-only and reject destructive `down` execution. That rejection is part of the compatibility contract. Never force a version-table value or bypass a database-name guard to make a migration appear successful.

## Verification

Test both clean initialization and upgrade from the supported prior version in dedicated MySQL and PostgreSQL databases. Validate the exact connected database name before reset, inspect key tables and indexes afterward, and keep development and production DSNs out of test commands and logs.
