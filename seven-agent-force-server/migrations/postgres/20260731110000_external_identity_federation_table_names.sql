-- +goose Up
-- B2 renames external identity and federation physical tables in place. Apply
-- it before starting application code that uses the lower snake_case names;
-- no legacy-name process may remain live afterward.
ALTER TABLE "sysExternalLoginProvider" RENAME TO sys_external_login_provider;
ALTER TABLE "sysExternalManagedProviderCommand" RENAME TO sys_external_managed_provider_command;
ALTER TABLE "sysExternalOAuthLoginState" RENAME TO sys_external_oauth_login_state;
ALTER TABLE "sysExternalOAuthToken" RENAME TO sys_external_oauth_token;
ALTER TABLE "sysExternalProviderMethod" RENAME TO sys_external_provider_method;
ALTER TABLE "sysExternalUserIdentity" RENAME TO sys_external_user_identity;
ALTER TABLE "sysFederatedNode" RENAME TO sys_federated_node;
ALTER TABLE "sysFederatedNodeConnectionCommand" RENAME TO sys_federated_node_connection_command;

-- Gate 3 deepens the existing external-login boundary with organization-owned
-- enterprise application credentials and grant-scoped personal identities.
-- Existing generic GitHub/Google/OIDC rows become encrypted SYSTEM identities;
-- the controlled alignment path protects their subject before clearing the
-- legacy plaintext column.
ALTER TABLE sys_external_user_identity
    ALTER COLUMN "externalSubject" DROP NOT NULL,
    ADD COLUMN IF NOT EXISTS scope_type VARCHAR(32) NOT NULL DEFAULT 'SYSTEM',
    ADD COLUMN IF NOT EXISTS organization_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS enterprise_connection_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS subject_kind VARCHAR(64) NULL,
    ADD COLUMN IF NOT EXISTS subject_ciphertext TEXT NULL,
    ADD COLUMN IF NOT EXISTS subject_edek TEXT NULL,
    ADD COLUMN IF NOT EXISTS subject_wrap_key_ref VARCHAR(128) NULL,
    ADD COLUMN IF NOT EXISTS subject_digest VARCHAR(128) NULL,
    ADD COLUMN IF NOT EXISTS subject_digest_key_ref VARCHAR(128) NULL,
    ADD COLUMN IF NOT EXISTS display_hint VARCHAR(128) NULL,
    ADD COLUMN IF NOT EXISTS identity_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS verified_at TIMESTAMPTZ NULL,
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMPTZ NULL;

ALTER TABLE sys_external_user_identity
    ADD CONSTRAINT chk_external_identity_scope
    CHECK (
        (scope_type = 'SYSTEM' AND organization_id IS NULL AND enterprise_connection_id IS NULL)
        OR
        (scope_type = 'ORGANIZATION' AND organization_id IS NOT NULL AND enterprise_connection_id IS NOT NULL)
    );

CREATE UNIQUE INDEX IF NOT EXISTS uk_external_identity_enterprise_subject_active
    ON sys_external_user_identity (organization_id, enterprise_connection_id, subject_digest)
    WHERE scope_type = 'ORGANIZATION' AND status = 0 AND "isDeleted" = 0;
CREATE UNIQUE INDEX IF NOT EXISTS uk_external_identity_enterprise_user_active
    ON sys_external_user_identity (organization_id, enterprise_connection_id, "userId")
    WHERE scope_type = 'ORGANIZATION' AND status = 0 AND "isDeleted" = 0;
CREATE UNIQUE INDEX IF NOT EXISTS uk_external_identity_system_subject_active
    ON sys_external_user_identity ("providerCode", COALESCE("externalIssuer", ''), subject_digest)
    WHERE scope_type = 'SYSTEM' AND status = 0 AND "isDeleted" = 0;

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_connection (
    id BIGINT PRIMARY KEY,
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
    binding_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    notification_direct_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    notification_mention_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    login_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    protocol_capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    config_digest VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    last_probe_status VARCHAR(32) NOT NULL DEFAULT 'NOT_PROBED',
    last_probe_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    last_probe_at TIMESTAMPTZ NULL,
    version BIGINT NOT NULL DEFAULT 1,
    creator_id BIGINT NOT NULL,
    updater_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_enterprise_connection_provider CHECK (provider_type IN ('FEISHU','WECOM')),
    CONSTRAINT chk_enterprise_connection_role CHECK (connection_role = 'PRIMARY'),
    CONSTRAINT chk_enterprise_connection_status CHECK (status IN ('ACTIVE','DISABLED')),
    CONSTRAINT chk_enterprise_connection_positive_ids CHECK (id > 0 AND organization_id > 0 AND creator_id > 0 AND updater_id > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_connection_primary
    ON sys_enterprise_identity_connection (organization_id, provider_type)
    WHERE connection_role = 'PRIMARY';
CREATE INDEX IF NOT EXISTS idx_enterprise_connection_status
    ON sys_enterprise_identity_connection (organization_id, status, provider_type);

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_grant (
    id BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    external_identity_id BIGINT NOT NULL,
    grant_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    version BIGINT NOT NULL DEFAULT 1,
    granted_by BIGINT NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL,
    revoked_by BIGINT NULL,
    revoked_at TIMESTAMPTZ NULL,
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_enterprise_grant_type CHECK (grant_type IN ('IDENTITY_LINK','NOTIFICATION_DIRECT','NOTIFICATION_MENTION','LOGIN')),
    CONSTRAINT chk_enterprise_grant_status CHECK (status IN ('ACTIVE','REVOKED')),
    CONSTRAINT chk_enterprise_grant_positive_ids CHECK (id > 0 AND organization_id > 0 AND external_identity_id > 0 AND granted_by > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_identity_grant
    ON sys_enterprise_identity_grant (organization_id, external_identity_id, grant_type);
CREATE INDEX IF NOT EXISTS idx_enterprise_identity_grant_active
    ON sys_enterprise_identity_grant (organization_id, grant_type, status, external_identity_id);

CREATE TABLE IF NOT EXISTS sys_enterprise_identity_authorization (
    id BIGINT PRIMARY KEY,
    authorization_identity VARCHAR(128) NOT NULL,
    purpose VARCHAR(32) NOT NULL,
    organization_id BIGINT NOT NULL,
    connection_id BIGINT NOT NULL,
    connection_version BIGINT NOT NULL,
    connection_config_digest VARCHAR(128) NOT NULL,
    bind_user_id BIGINT NULL,
    requested_grants_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    requested_grants_digest VARCHAR(128) NOT NULL,
    state_hash VARCHAR(128) NOT NULL,
    nonce_hash VARCHAR(128) NOT NULL DEFAULT '',
    code_verifier_ciphertext TEXT NULL,
    code_verifier_edek TEXT NULL,
    code_verifier_wrap_key_ref VARCHAR(128) NULL,
    redirect_after VARCHAR(1024) NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    result_identity_id BIGINT NULL,
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    consumed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_enterprise_authorization_purpose CHECK (purpose IN ('NOTIFICATION_BIND','LOGIN')),
    CONSTRAINT chk_enterprise_authorization_status CHECK (status IN ('ACTIVE','PROCESSING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN')),
    CONSTRAINT chk_enterprise_authorization_positive_ids CHECK (id > 0 AND organization_id > 0 AND connection_id > 0)
);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_authorization_identity
    ON sys_enterprise_identity_authorization (authorization_identity);
CREATE UNIQUE INDEX IF NOT EXISTS uk_enterprise_authorization_state
    ON sys_enterprise_identity_authorization (state_hash);
CREATE INDEX IF NOT EXISTS idx_enterprise_authorization_active
    ON sys_enterprise_identity_authorization (organization_id, status, expires_at);

-- +goose Down
-- B2 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
-- +goose StatementBegin
DO $b2$
BEGIN
    RAISE EXCEPTION 'B2 external identity and federation table rename is forward-only';
END
$b2$;
-- +goose StatementEnd
