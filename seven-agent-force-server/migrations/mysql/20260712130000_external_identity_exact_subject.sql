-- +goose Up
SET @task7ExactSQL = IF(
  EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND COLUMN_NAME = 'providerSubjectDigest'),
  'SELECT 1',
  'ALTER TABLE sysExternalUserIdentity ADD COLUMN providerSubjectDigest BINARY(32) GENERATED ALWAYS AS (UNHEX(SHA2(CONCAT(providerCode, CHAR(0), externalSubject), 256))) STORED AFTER externalSubject'
);
PREPARE task7ExactStatement FROM @task7ExactSQL;
EXECUTE task7ExactStatement;
DEALLOCATE PREPARE task7ExactStatement;

SET @task7ExactSQL = IF(
  EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND COLUMN_NAME = 'externalIdentityDigest'),
  'SELECT 1',
  'ALTER TABLE sysExternalUserIdentity ADD COLUMN externalIdentityDigest BINARY(32) GENERATED ALWAYS AS (UNHEX(SHA2(CONCAT(externalIssuer, CHAR(0), externalSubject), 256))) STORED AFTER externalIssuerDigest'
);
PREPARE task7ExactStatement FROM @task7ExactSQL;
EXECUTE task7ExactStatement;
DEALLOCATE PREPARE task7ExactStatement;

SET @task7ExactSQL = IF(
  EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND INDEX_NAME = 'uk_sysExternalUserIdentity_subject_deleted'),
  'ALTER TABLE sysExternalUserIdentity DROP INDEX uk_sysExternalUserIdentity_subject_deleted',
  'SELECT 1'
);
PREPARE task7ExactStatement FROM @task7ExactSQL;
EXECUTE task7ExactStatement;
DEALLOCATE PREPARE task7ExactStatement;

SET @task7ExactSQL = IF(
  EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND INDEX_NAME = 'uk_sysExternalUserIdentity_issuer_subject_deleted'),
  'ALTER TABLE sysExternalUserIdentity DROP INDEX uk_sysExternalUserIdentity_issuer_subject_deleted',
  'SELECT 1'
);
PREPARE task7ExactStatement FROM @task7ExactSQL;
EXECUTE task7ExactStatement;
DEALLOCATE PREPARE task7ExactStatement;

SET @task7ExactSQL = IF(
  EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND INDEX_NAME = 'uk_sysExternalUserIdentity_provider_subject_digest_deleted'),
  'SELECT 1',
  'ALTER TABLE sysExternalUserIdentity ADD UNIQUE KEY uk_sysExternalUserIdentity_provider_subject_digest_deleted (providerSubjectDigest, isDeleted)'
);
PREPARE task7ExactStatement FROM @task7ExactSQL;
EXECUTE task7ExactStatement;
DEALLOCATE PREPARE task7ExactStatement;

SET @task7ExactSQL = IF(
  EXISTS(SELECT 1 FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalUserIdentity' AND INDEX_NAME = 'uk_sysExternalUserIdentity_issuer_subject_deleted'),
  'SELECT 1',
  'ALTER TABLE sysExternalUserIdentity ADD UNIQUE KEY uk_sysExternalUserIdentity_issuer_subject_deleted (externalIdentityDigest, isDeleted)'
);
PREPARE task7ExactStatement FROM @task7ExactSQL;
EXECUTE task7ExactStatement;
DEALLOCATE PREPARE task7ExactStatement;

SET @task7ExactSQL = IF(
  EXISTS(SELECT 1 FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'sysExternalOAuthLoginState' AND COLUMN_NAME = 'providerConfigDigest'),
  'SELECT 1',
  'ALTER TABLE sysExternalOAuthLoginState ADD COLUMN providerConfigDigest CHAR(64) NULL COMMENT ''托管Provider启动配置摘要'' AFTER issuer'
);
PREPARE task7ExactStatement FROM @task7ExactSQL;
EXECUTE task7ExactStatement;
DEALLOCATE PREPARE task7ExactStatement;

DROP PROCEDURE IF EXISTS task7ExactIdentityRollbackPreflight;
-- +goose StatementBegin
CREATE PROCEDURE task7ExactIdentityRollbackPreflight()
BEGIN
  IF EXISTS(
    SELECT 1
    FROM sysExternalUserIdentity
    GROUP BY providerCode, externalSubject, isDeleted
    HAVING COUNT(*) > 1
  ) OR EXISTS(
    SELECT 1
    FROM sysExternalUserIdentity
    WHERE externalIssuerDigest IS NOT NULL
    GROUP BY externalIssuerDigest, externalSubject, isDeleted
    HAVING COUNT(*) > 1
  ) THEN
    SIGNAL SQLSTATE '45000'
      SET MESSAGE_TEXT = 'Task 7 rollback blocked: resolve case-insensitive external identity conflicts before retry';
  END IF;
END;
-- +goose StatementEnd

-- +goose Down
CALL task7ExactIdentityRollbackPreflight();
DROP PROCEDURE IF EXISTS task7ExactIdentityRollbackPreflight;

ALTER TABLE sysExternalOAuthLoginState DROP COLUMN providerConfigDigest;
ALTER TABLE sysExternalUserIdentity DROP INDEX uk_sysExternalUserIdentity_issuer_subject_deleted;
ALTER TABLE sysExternalUserIdentity DROP INDEX uk_sysExternalUserIdentity_provider_subject_digest_deleted;
ALTER TABLE sysExternalUserIdentity ADD UNIQUE KEY uk_sysExternalUserIdentity_subject_deleted (providerCode, externalSubject, isDeleted);
ALTER TABLE sysExternalUserIdentity ADD UNIQUE KEY uk_sysExternalUserIdentity_issuer_subject_deleted (externalIssuerDigest, externalSubject, isDeleted);
ALTER TABLE sysExternalUserIdentity DROP COLUMN externalIdentityDigest;
ALTER TABLE sysExternalUserIdentity DROP COLUMN providerSubjectDigest;
