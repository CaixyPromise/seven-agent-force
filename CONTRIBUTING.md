# Contributing

## Development workflow

Create a focused branch, keep commits reviewable, and explain behavior, migration, and configuration changes in the pull request. Use concise commit prefixes such as `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, or `chore:`.

Before submitting changes, run:

```bash
make web-install
make check
```

Changes that affect concurrency, authorization, migrations, caching, files, notifications, or public API contracts also require focused regression tests and a race-enabled test for the affected Go packages.

## Architecture rules

Each Go application module uses `controller`, `job`, `listener`, `facade`, `application`, `domain`, and `infrastructure` boundaries. Entry points call only their local facade or application layer. Cross-module dependencies use API/facade contracts. Infrastructure must not import application, facade, controller, job, or listener packages. Core business rules belong in the domain layer.

Run the repository's Go tests before requesting review; they include DDD import-boundary checks.

## Database changes

Workforce currently targets PostgreSQL. Follow the repository AGENTS.md identity and unpublished-migration rules; Framework-wide changes must preserve their supported database coverage. Do not add database foreign keys. Preserve existing forward-only and version-table safeguards, and verify destructive operations only against explicitly named disposable databases.

## Security and privacy

Do not include real users, real credentials, private URLs, environment files, uploads, database dumps, internal acceptance evidence, or private deployment information. Report vulnerabilities through the private process in [SECURITY.md](SECURITY.md).

## Developer guides

- [Server/Web development](docs/development.md)
- [SDK/Adapter development](docs/sdk-development.md)
