# PostgreSQL storage contracts and upgrades

Storage types follow the value's producer, consumer and validation rules. `TEXT` remains appropriate for unbounded content and opaque external metadata. It is not a substitute for an internal identity: internal entity and relation IDs remain `BIGINT`, and external identity exceptions remain governed by the identity contract tests.

## Type policy

| Value | Contract |
| --- | --- |
| Internal identities and relations | PostgreSQL `BIGINT`, Go `int64`; decimal strings at external boundaries |
| SHA256 and OCI digests | Bounded storage matching the digest representation; retain existing format validation |
| State, scope, protocol and action codes | Module-defined enum values with the module's explicit storage width |
| Idempotency keys and externally accepted names | Bounds must accommodate the actual application validator; never truncate during an upgrade |
| Opaque sessions, filesystem paths, Git refs, image/version metadata, artifact display names | Retain `TEXT` where the producer/consumer contract has no matching length limit |
| Hook optimistic revision counter | `BIGINT`, matching the Hook Store and HTTP expected-revision `int64` contract |
| Frozen assignment credential version and model protocol | Required snapshot inputs; do not infer historical values from the current credential |
| HTTP response status | `SMALLINT` storage; protocol-specific response checks remain owned by the relevant module |

A change from `TEXT` to `VARCHAR(n)` is a real storage constraint, not a performance optimization. PostgreSQL explicit casts to `VARCHAR(n)` can truncate; the upgrade uses preflight length validation and assignment conversion instead. Default expressions differing only by `::text` versus `::character varying` are compared by value semantics, not raw SQL spelling.

## One policy for installation and legacy alignment

`internal/app/workforce/schemacontract/columns.sql` defines the explicit table/column storage allowlist. `guards.sql` supplies current model/runtime JSON, snapshot, digest, protocol and counter checks, access paths and readiness diagnostics. The forward migration `20261025130000_workforce_storage_contract.sql` contains the same policy; a Go test prevents migration/alignment divergence.

Full current development schemas use that shared policy through the explicit alignment command. Partial historical development schemas remain owned by their earlier migrations. Normal API startup does not run alignment or modify schema.

Fresh installation uses the baseline and the full upgrade chain described in [database initialization](database-schema.md). An existing installation uses only pending forward migrations. Never replay the baseline or forge migration ledger entries.

## Read-only preflight

From a source checkout matching the target release, supply the target through private libpq environment/service configuration:

```sh
python3 scripts/database/check-storage-contract.py --output /path/to/new-preflight.json
```

The utility forces read-only sessions, limits statement/lock waits, checks 280 explicit column targets and 25 module checks, and emits only violation counts with column/constraint names. It never prints credentials or row values. Exit 0 means no data blockers were found; exit 2 means review the counts. A connection/schema/query error stops the check without printing raw database errors. The report is private operational evidence and must not be committed.

A successful preflight does not authorize upgrading the database and does not eliminate races with new writes. The migration validates again while holding table locks. Coordinate maintenance, backup and dispatch suspension before an approved upgrade; this migration can acquire exclusive locks and rewrite tables, so assess duration on a representative dataset. Use the actual migration runner and retain normal transaction and version ownership.

## Failure and recovery

- Oversized bounded content, integer overflow, missing required frozen inputs or invalid module data block the upgrade. The transaction rolls back; values are not shortened, deleted, synthesized or filled from mutable current credentials.
- Diagnose blockers using authorized, private inspection. Resolve invalid data against original immutable evidence and business policy before retrying. A missing frozen value is not permission to rerun an Agent.
- Existing semantically equivalent index/constraint names can remain. The upgrade adds missing checks and access paths; it does not require destructive index cleanup merely for naming consistency.
- The migration is forward-only. After commit, recover through the approved backup/restore plan or a reviewed forward repair, rather than a destructive Down.

## Verification

Unit tests run without a database:

```sh
python3 -m unittest discover -s scripts/database -p 'test_*.py'
```

PostgreSQL regression tests are destructive **only inside a disposable test database**. Create `force_storage_contract_test_regression` in an isolated instance, configure libpq for that database, then opt in explicitly:

```sh
FORCE_STORAGE_CONTRACT_TEST=1 python3 -m unittest discover -s scripts/database -p 'test_*.py'
```

The suite refuses any other database name and checks length failures, integer overflow, missing frozen snapshots, rollback, long unbounded values, Unicode, 64-bit revisions, module guards and repeated application. Separately validate the complete baseline/upgrade chain and a schema-only copy of the old structure. Do not substitute these synthetic tests for production-data upgrade rehearsal or application acceptance.
