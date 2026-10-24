-- +goose Up

ALTER TABLE sys_upload_task
  ADD COLUMN scopeId VARCHAR(128) DEFAULT NULL AFTER userId,
  ADD COLUMN credentialId VARCHAR(64) DEFAULT NULL AFTER scopeId,
  ADD COLUMN credentialVersion INT NOT NULL DEFAULT 0 AFTER credentialId,
  ADD COLUMN protectedUntil DATETIME DEFAULT NULL AFTER expireAt,
  ADD COLUMN credentialExpireAt DATETIME DEFAULT NULL AFTER protectedUntil,
  ADD COLUMN revokedAt DATETIME DEFAULT NULL AFTER credentialExpireAt,
  ADD UNIQUE KEY uk_upload_credential_id (credentialId),
  ADD KEY idx_upload_credential_authority (userId, scopeId, fileId, status, credentialVersion, credentialExpireAt),
  ADD KEY idx_upload_credential_protection (fileId, status, protectedUntil, revokedAt);

ALTER TABLE sys_file_chunk_upload
  ADD COLUMN scopeId VARCHAR(128) DEFAULT NULL AFTER userId,
  ADD COLUMN uploadTaskId VARCHAR(64) DEFAULT NULL AFTER uploadId,
  ADD KEY idx_chunk_upload_task (uploadTaskId),
  ADD KEY idx_chunk_scope_user_status (scopeId, userId, status, expireTime);

ALTER TABLE sys_file_reference
  ADD COLUMN scopeId VARCHAR(128) DEFAULT NULL AFTER userId,
  ADD COLUMN activeBizId BIGINT
    GENERATED ALWAYS AS (CASE WHEN isDeleted = 0 THEN bizId ELSE NULL END) STORED,
  DROP INDEX uk_user_biz_active,
  ADD UNIQUE KEY uk_file_reference_active_slot (userId, scopeId, bizType, activeBizId),
  ADD KEY idx_file_reference_scope_file (scopeId, fileId, isDeleted);

-- +goose Down

ALTER TABLE sys_file_reference
  DROP INDEX idx_file_reference_scope_file,
  DROP INDEX uk_file_reference_active_slot,
  DROP COLUMN activeBizId,
  DROP COLUMN scopeId;

CREATE UNIQUE INDEX uk_user_biz_active
  ON sys_file_reference (userId, bizType, ((CASE WHEN isDeleted = 0 THEN bizId ELSE NULL END)));

ALTER TABLE sys_file_chunk_upload
  DROP INDEX idx_chunk_scope_user_status,
  DROP INDEX idx_chunk_upload_task,
  DROP COLUMN uploadTaskId,
  DROP COLUMN scopeId;

ALTER TABLE sys_upload_task
  DROP INDEX idx_upload_credential_protection,
  DROP INDEX idx_upload_credential_authority,
  DROP INDEX uk_upload_credential_id,
  DROP COLUMN revokedAt,
  DROP COLUMN credentialExpireAt,
  DROP COLUMN protectedUntil,
  DROP COLUMN credentialVersion,
  DROP COLUMN credentialId,
  DROP COLUMN scopeId;
