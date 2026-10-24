-- +goose Up

CREATE TABLE IF NOT EXISTS docker_operation_integrity_guard (
  operationId BIGINT NOT NULL COMMENT 'Docker operation lock namespace',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Guard creation time',
  PRIMARY KEY (operationId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Docker operation application-integrity lock guards';

INSERT IGNORE INTO docker_operation_integrity_guard (operationId, createTime)
SELECT id, NOW() FROM docker_operation;

INSERT IGNORE INTO docker_operation_integrity_guard (operationId, createTime)
SELECT DISTINCT operationId, NOW() FROM docker_operation_event;

-- MySQL does not support ADD COLUMN IF NOT EXISTS. Each atomic DDL unit is
-- independently gated so a retry also recovers a stop between two units.
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name = 'integrityStatus') = 0,
  'ALTER TABLE docker_operation_event ADD COLUMN integrityStatus VARCHAR(16) NOT NULL DEFAULT ''ACTIVE'' COMMENT ''ACTIVE or QUARANTINED''', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name = 'diagnosticId') = 0,
  'ALTER TABLE docker_operation_event ADD COLUMN diagnosticId VARCHAR(191) DEFAULT NULL COMMENT ''Stable orphan diagnostic identity''', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name = 'integrityVersion') = 0,
  'ALTER TABLE docker_operation_event ADD COLUMN integrityVersion BIGINT NOT NULL DEFAULT 0 COMMENT ''Optimistic orphan cleanup version''', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name = 'diagnosedAt') = 0,
  'ALTER TABLE docker_operation_event ADD COLUMN diagnosedAt DATETIME DEFAULT NULL COMMENT ''Latest orphan diagnosis time''', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name = 'integrityScope') = 0,
  'ALTER TABLE docker_operation_event ADD COLUMN integrityScope VARCHAR(64) DEFAULT NULL COMMENT ''Bounded diagnostic scope''', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name = 'integrityRelationshipType') = 0,
  'ALTER TABLE docker_operation_event ADD COLUMN integrityRelationshipType VARCHAR(128) DEFAULT NULL COMMENT ''Broken relationship type''', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND column_name = 'integrityReason') = 0,
  'ALTER TABLE docker_operation_event ADD COLUMN integrityReason VARCHAR(128) DEFAULT NULL COMMENT ''Diagnostic reason code''', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;

SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'docker_operation_event' AND constraint_name = 'chk_docker_operation_event_integrity_status') = 0,
  'ALTER TABLE docker_operation_event ADD CONSTRAINT chk_docker_operation_event_integrity_status CHECK (integrityStatus IN (''ACTIVE'', ''QUARANTINED''))', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'docker_operation_event' AND constraint_name = 'chk_docker_operation_event_diagnostic_metadata') = 0,
  'ALTER TABLE docker_operation_event ADD CONSTRAINT chk_docker_operation_event_diagnostic_metadata CHECK ((integrityStatus = ''ACTIVE'' AND diagnosticId IS NULL AND integrityScope IS NULL AND integrityRelationshipType IS NULL AND integrityReason IS NULL) OR (integrityStatus = ''QUARANTINED'' AND diagnosticId IS NOT NULL AND integrityScope IS NOT NULL AND integrityRelationshipType IS NOT NULL AND integrityReason IS NOT NULL))', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event' AND index_name = 'uk_docker_operation_event_diagnostic') = 0,
  'ALTER TABLE docker_operation_event ADD UNIQUE KEY uk_docker_operation_event_diagnostic (diagnosticId)', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;

CREATE TABLE IF NOT EXISTS docker_operation_event_orphan_audit (
  id BIGINT NOT NULL COMMENT 'Audit id and idempotency identity',
  diagnosticId VARCHAR(191) NOT NULL COMMENT 'Stable diagnosed orphan identity',
  eventId BIGINT NOT NULL COMMENT 'Diagnosed event id',
  operationId BIGINT NOT NULL COMMENT 'Missing parent operation id',
  sequence BIGINT NOT NULL COMMENT 'Diagnosed event sequence',
  expectedIntegrityVersion BIGINT NOT NULL COMMENT 'Exact quarantined event version authorized by this audit',
  action VARCHAR(32) NOT NULL COMMENT 'Authorized cleanup action',
  result VARCHAR(32) NOT NULL COMMENT 'Bounded cleanup result',
  actorUserId BIGINT NOT NULL COMMENT 'Authorized actor id',
  actorUsername VARCHAR(128) NOT NULL COMMENT 'Authorized actor name',
  reason VARCHAR(512) NOT NULL COMMENT 'Cleanup reason',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT 'Audit creation time',
  PRIMARY KEY (id),
  CONSTRAINT chk_docker_operation_orphan_audit_action CHECK (action IN ('DELETE')),
  CONSTRAINT chk_docker_operation_orphan_audit_result
    CHECK (result IN ('DELETED', 'ALREADY_DONE', 'PARENT_PRESENT'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Audited Docker operation orphan cleanup outcomes';

SET @dg3_sql = IF((SELECT COUNT(1) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'docker_operation_event_orphan_audit' AND column_name = 'expectedIntegrityVersion') = 0,
  'ALTER TABLE docker_operation_event_orphan_audit ADD COLUMN expectedIntegrityVersion BIGINT NULL COMMENT ''Exact version; 0 means unknown legacy audit'' AFTER sequence', 'SELECT 1');
PREPARE dg3_stmt FROM @dg3_sql;
EXECUTE dg3_stmt;
DEALLOCATE PREPARE dg3_stmt;
UPDATE docker_operation_event_orphan_audit
SET expectedIntegrityVersion = 0
WHERE expectedIntegrityVersion IS NULL;
ALTER TABLE docker_operation_event_orphan_audit
  MODIFY COLUMN expectedIntegrityVersion BIGINT NOT NULL COMMENT 'Exact version; 0 means unknown legacy audit';

-- The existing fk_docker_operation_event_operation is intentionally retained
-- until MySQL and PostgreSQL concurrency/restart/recovery evidence passes.

-- +goose Down

-- DG3 integrity enforcement is forward-only. Removing the audit trail, lock
-- namespace, and diagnostic metadata would make an interrupted deployment less
-- safe. Repair the forward precondition and resume instead.
SIGNAL SQLSTATE '45000'
  SET MESSAGE_TEXT = 'DG3 operation integrity migration is forward-only';
