-- +goose Up
-- B2 renames external identity and federation physical tables in place. Apply
-- it before starting application code that uses the lower snake_case names;
-- no legacy-name process may remain live afterward.
RENAME TABLE
    sysExternalLoginProvider TO sys_external_login_provider,
    sysExternalManagedProviderCommand TO sys_external_managed_provider_command,
    sysExternalOAuthLoginState TO sys_external_oauth_login_state,
    sysExternalOAuthToken TO sys_external_oauth_token,
    sysExternalProviderMethod TO sys_external_provider_method,
    sysExternalUserIdentity TO sys_external_user_identity,
    sysFederatedNode TO sys_federated_node,
    sysFederatedNodeConnectionCommand TO sys_federated_node_connection_command;

ALTER TABLE sys_external_user_identity
    MODIFY COLUMN externalSubject VARCHAR(255) NULL COMMENT '系统身份兼容主体；企业身份始终使用加密 Subject',
    ADD COLUMN scope_type VARCHAR(32) NOT NULL DEFAULT 'SYSTEM',
    ADD COLUMN organization_id BIGINT NULL,
    ADD COLUMN enterprise_connection_id BIGINT NULL,
    ADD COLUMN subject_kind VARCHAR(64) NULL,
    ADD COLUMN subject_ciphertext TEXT NULL,
    ADD COLUMN subject_edek TEXT NULL,
    ADD COLUMN subject_wrap_key_ref VARCHAR(128) NULL,
    ADD COLUMN subject_digest VARCHAR(128) NULL,
    ADD COLUMN subject_digest_key_ref VARCHAR(128) NULL,
    ADD COLUMN display_hint VARCHAR(128) NULL,
    ADD COLUMN identity_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN verified_at DATETIME NULL,
    ADD COLUMN revoked_at DATETIME NULL,
    ADD CONSTRAINT chk_external_identity_scope CHECK (
        (scope_type = 'SYSTEM' AND organization_id IS NULL AND enterprise_connection_id IS NULL)
        OR
        (scope_type = 'ORGANIZATION' AND organization_id IS NOT NULL AND enterprise_connection_id IS NOT NULL)
    ),
    ADD UNIQUE KEY uk_external_identity_enterprise_subject_active (organization_id, enterprise_connection_id, subject_digest, status, isDeleted),
    ADD UNIQUE KEY uk_external_identity_enterprise_user_active (organization_id, enterprise_connection_id, userId, status, isDeleted),
    ADD UNIQUE KEY uk_external_identity_system_subject_active (providerCode, externalIssuer, subject_digest, status, isDeleted);

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_connection (
    id BIGINT NOT NULL,
    organization_id BIGINT NOT NULL,
    provider_type VARCHAR(32) NOT NULL,
    connection_role VARCHAR(32) NOT NULL DEFAULT 'PRIMARY',
    display_name VARCHAR(128) NOT NULL,
    app_id VARCHAR(255) NULL,
    corp_id VARCHAR(255) NULL,
    agent_id VARCHAR(255) NULL,
    endpoint_profile VARCHAR(32) NOT NULL DEFAULT 'OFFICIAL_CN',
    client_secret_ciphertext TEXT NOT NULL,
    client_secret_edek TEXT NOT NULL,
    client_secret_wrap_key_ref VARCHAR(128) NOT NULL,
    credential_version BIGINT NOT NULL DEFAULT 1,
    binding_enabled TINYINT(1) NOT NULL DEFAULT 1,
    notification_direct_enabled TINYINT(1) NOT NULL DEFAULT 1,
    notification_mention_enabled TINYINT(1) NOT NULL DEFAULT 1,
    login_enabled TINYINT(1) NOT NULL DEFAULT 0,
    protocol_capabilities_json JSON NOT NULL,
    config_digest VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    last_probe_status VARCHAR(32) NOT NULL DEFAULT 'NOT_PROBED',
    last_probe_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    last_probe_at DATETIME NULL,
    version BIGINT NOT NULL DEFAULT 1,
    creator_id BIGINT NOT NULL,
    updater_id BIGINT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_enterprise_connection_primary (organization_id, provider_type),
    KEY idx_enterprise_connection_status (organization_id, status, provider_type),
    CONSTRAINT chk_enterprise_connection_provider CHECK (provider_type IN ('FEISHU','WECOM')),
    CONSTRAINT chk_enterprise_connection_role CHECK (connection_role = 'PRIMARY'),
    CONSTRAINT chk_enterprise_connection_status CHECK (status IN ('ACTIVE','DISABLED')),
    CONSTRAINT chk_enterprise_connection_positive_ids CHECK (id > 0 AND organization_id > 0 AND creator_id > 0 AND updater_id > 0)
) COMMENT='组织企业身份连接';

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_grant (
    id BIGINT NOT NULL,
    organization_id BIGINT NOT NULL,
    external_identity_id BIGINT NOT NULL,
    grant_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    version BIGINT NOT NULL DEFAULT 1,
    granted_by BIGINT NOT NULL,
    granted_at DATETIME NOT NULL,
    revoked_by BIGINT NULL,
    revoked_at DATETIME NULL,
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_enterprise_identity_grant (organization_id, external_identity_id, grant_type),
    KEY idx_enterprise_identity_grant_active (organization_id, grant_type, status, external_identity_id),
    CONSTRAINT chk_enterprise_grant_type CHECK (grant_type IN ('IDENTITY_LINK','NOTIFICATION_DIRECT','NOTIFICATION_MENTION','LOGIN')),
    CONSTRAINT chk_enterprise_grant_status CHECK (status IN ('ACTIVE','REVOKED')),
    CONSTRAINT chk_enterprise_grant_positive_ids CHECK (id > 0 AND organization_id > 0 AND external_identity_id > 0 AND granted_by > 0)
) COMMENT='企业身份独立授权';

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_authorization (
    id BIGINT NOT NULL,
    authorization_identity VARCHAR(128) NOT NULL,
    purpose VARCHAR(32) NOT NULL,
    organization_id BIGINT NOT NULL,
    connection_id BIGINT NOT NULL,
    connection_version BIGINT NOT NULL,
    connection_config_digest VARCHAR(128) NOT NULL,
    bind_user_id BIGINT NULL,
    requested_grants_json JSON NOT NULL,
    requested_grants_digest VARCHAR(128) NOT NULL,
    state_hash VARCHAR(128) NOT NULL,
    nonce_hash VARCHAR(128) NOT NULL DEFAULT '',
    code_verifier_ciphertext TEXT NULL,
    code_verifier_edek TEXT NULL,
    code_verifier_wrap_key_ref VARCHAR(128) NULL,
    redirect_after VARCHAR(1024) NOT NULL DEFAULT '',
    expires_at DATETIME NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    result_identity_id BIGINT NULL,
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    consumed_at DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uk_enterprise_authorization_identity (authorization_identity),
    UNIQUE KEY uk_enterprise_authorization_state (state_hash),
    KEY idx_enterprise_authorization_active (organization_id, status, expires_at),
    CONSTRAINT chk_enterprise_authorization_purpose CHECK (purpose IN ('NOTIFICATION_BIND','LOGIN')),
    CONSTRAINT chk_enterprise_authorization_status CHECK (status IN ('ACTIVE','PROCESSING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN')),
    CONSTRAINT chk_enterprise_authorization_positive_ids CHECK (id > 0 AND organization_id > 0 AND connection_id > 0)
) COMMENT='企业OAuth授权事务';

-- +goose Down
-- B2 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'B2 external identity and federation table rename is forward-only';
