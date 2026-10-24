-- +goose Up

CREATE TABLE IF NOT EXISTS docker_operation_integrity_guard (
  "operationId" bigint NOT NULL,
  "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT pk_docker_operation_integrity_guard PRIMARY KEY ("operationId")
);

INSERT INTO docker_operation_integrity_guard ("operationId", "createTime")
SELECT id, CURRENT_TIMESTAMP FROM docker_operation
ON CONFLICT ("operationId") DO NOTHING;

INSERT INTO docker_operation_integrity_guard ("operationId", "createTime")
SELECT DISTINCT "operationId", CURRENT_TIMESTAMP FROM docker_operation_event
ON CONFLICT ("operationId") DO NOTHING;

ALTER TABLE docker_operation_event
  ADD COLUMN IF NOT EXISTS "integrityStatus" character varying(16) NOT NULL DEFAULT 'ACTIVE',
  ADD COLUMN IF NOT EXISTS "diagnosticId" character varying(191),
  ADD COLUMN IF NOT EXISTS "integrityVersion" bigint NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS "diagnosedAt" timestamp with time zone,
  ADD COLUMN IF NOT EXISTS "integrityScope" character varying(64),
  ADD COLUMN IF NOT EXISTS "integrityRelationshipType" character varying(128),
  ADD COLUMN IF NOT EXISTS "integrityReason" character varying(128);

-- +goose StatementBegin
DO $dg3$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_docker_operation_event_integrity_status'
      AND conrelid = 'docker_operation_event'::regclass
  ) THEN
    ALTER TABLE docker_operation_event
      ADD CONSTRAINT chk_docker_operation_event_integrity_status
        CHECK ("integrityStatus" IN ('ACTIVE', 'QUARANTINED'));
  END IF;
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'chk_docker_operation_event_diagnostic_metadata'
      AND conrelid = 'docker_operation_event'::regclass
  ) THEN
    ALTER TABLE docker_operation_event
      ADD CONSTRAINT chk_docker_operation_event_diagnostic_metadata
        CHECK (
          ("integrityStatus" = 'ACTIVE' AND "diagnosticId" IS NULL AND "integrityScope" IS NULL AND "integrityRelationshipType" IS NULL AND "integrityReason" IS NULL)
          OR
          ("integrityStatus" = 'QUARANTINED' AND "diagnosticId" IS NOT NULL AND "integrityScope" IS NOT NULL AND "integrityRelationshipType" IS NOT NULL AND "integrityReason" IS NOT NULL)
        );
  END IF;
END
$dg3$;
-- +goose StatementEnd

CREATE UNIQUE INDEX IF NOT EXISTS uk_docker_operation_event_diagnostic
  ON docker_operation_event ("diagnosticId")
  WHERE "diagnosticId" IS NOT NULL;

CREATE TABLE IF NOT EXISTS docker_operation_event_orphan_audit (
  id bigint NOT NULL,
  "diagnosticId" character varying(191) NOT NULL,
  "eventId" bigint NOT NULL,
  "operationId" bigint NOT NULL,
  sequence bigint NOT NULL,
  "expectedIntegrityVersion" bigint NOT NULL,
  action character varying(32) NOT NULL,
  result character varying(32) NOT NULL,
  "actorUserId" bigint NOT NULL,
  "actorUsername" character varying(128) NOT NULL,
  reason character varying(512) NOT NULL,
  "createTime" timestamp with time zone NOT NULL DEFAULT CURRENT_TIMESTAMP,
  CONSTRAINT pk_docker_operation_event_orphan_audit PRIMARY KEY (id),
  CONSTRAINT chk_docker_operation_orphan_audit_action CHECK (action IN ('DELETE')),
  CONSTRAINT chk_docker_operation_orphan_audit_result
    CHECK (result IN ('DELETED', 'ALREADY_DONE', 'PARENT_PRESENT'))
);

ALTER TABLE docker_operation_event_orphan_audit
  ADD COLUMN IF NOT EXISTS "expectedIntegrityVersion" bigint;
UPDATE docker_operation_event_orphan_audit
SET "expectedIntegrityVersion" = 0
WHERE "expectedIntegrityVersion" IS NULL;
ALTER TABLE docker_operation_event_orphan_audit
  ALTER COLUMN "expectedIntegrityVersion" SET NOT NULL;

-- The existing fk_docker_operation_event_operation is intentionally retained
-- until MySQL and PostgreSQL concurrency/restart/recovery evidence passes.

-- +goose Down

-- DG3 integrity enforcement is forward-only. Removing the audit trail, lock
-- namespace, and diagnostic metadata would make an interrupted deployment less
-- safe. Repair the forward precondition and resume instead.
-- +goose StatementBegin
DO $dg3_down$
BEGIN
  RAISE EXCEPTION 'DG3 operation integrity migration is forward-only';
END
$dg3_down$;
-- +goose StatementEnd
