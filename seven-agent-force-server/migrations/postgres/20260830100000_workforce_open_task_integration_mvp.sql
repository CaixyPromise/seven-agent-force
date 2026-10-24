-- +goose Up

ALTER TABLE workforce_tasks
    ADD COLUMN submission_channel VARCHAR(32) NOT NULL DEFAULT 'WEB',
    ADD COLUMN integration_application_id BIGINT,
    ADD COLUMN scenario_key VARCHAR(128),
    ADD COLUMN scenario_version INTEGER,
    ADD COLUMN scenario_digest VARCHAR(64),
    ADD COLUMN external_correlation_id VARCHAR(256);

ALTER TABLE workforce_task_events
    ADD COLUMN open_integration_projection_status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    ADD CONSTRAINT workforce_task_events_open_projection_status_ck
        CHECK (open_integration_projection_status IN ('PENDING','PROJECTED','SKIPPED'));

CREATE INDEX idx_workforce_task_events_open_projection_pending
    ON workforce_task_events (id)
    WHERE open_integration_projection_status = 'PENDING'
      AND event_type IN ('task.succeeded','task.failed','task.cancelled','task.timed_out');

CREATE INDEX idx_workforce_tasks_open_origin
    ON workforce_tasks (tenant_id, integration_application_id, created_at DESC)
    WHERE integration_application_id IS NOT NULL;

CREATE TABLE workforce_integration_applications (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ENABLED',
    webhook_url VARCHAR(2048) NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_workforce_integration_applications_tenant
    ON workforce_integration_applications (tenant_id, created_at DESC);

CREATE TABLE workforce_integration_api_credentials (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    integration_application_id BIGINT NOT NULL,
    credential_locator VARCHAR(256) NOT NULL UNIQUE,
    credential_digest VARCHAR(64) NOT NULL,
    credential_hint VARCHAR(64) NOT NULL,
    version INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    valid_until TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    UNIQUE (integration_application_id, version),
    CHECK (version > 0),
    CHECK (status IN ('CURRENT','GRACE','REVOKED'))
);

CREATE UNIQUE INDEX idx_workforce_integration_api_credentials_current
    ON workforce_integration_api_credentials (integration_application_id)
    WHERE status = 'CURRENT';
CREATE INDEX idx_workforce_integration_api_credentials_application
    ON workforce_integration_api_credentials (tenant_id, integration_application_id, status, version DESC);

CREATE TABLE workforce_integration_signing_credentials (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    integration_application_id BIGINT NOT NULL,
    signing_secret_ciphertext BYTEA NOT NULL,
    signing_secret_edek BYTEA NOT NULL,
    signing_secret_wrap_key_ref VARCHAR(256) NOT NULL,
    signing_secret_hint VARCHAR(64) NOT NULL,
    version INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    valid_until TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at TIMESTAMPTZ,
    UNIQUE (integration_application_id, version),
    CHECK (version > 0),
    CHECK (status IN ('CURRENT','GRACE','REVOKED'))
);

CREATE UNIQUE INDEX idx_workforce_integration_signing_credentials_current
    ON workforce_integration_signing_credentials (integration_application_id)
    WHERE status = 'CURRENT';
CREATE INDEX idx_workforce_integration_signing_credentials_application
    ON workforce_integration_signing_credentials (tenant_id, integration_application_id, status, version DESC);

CREATE TABLE workforce_public_events (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    integration_application_id BIGINT NOT NULL,
    source_task_event_id BIGINT NOT NULL UNIQUE,
    task_id BIGINT NOT NULL,
    task_turn_id BIGINT NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    sequence INTEGER NOT NULL,
    correlation_id VARCHAR(256) NOT NULL,
    payload_json JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_workforce_public_events_application
    ON workforce_public_events (tenant_id, integration_application_id, created_at DESC);

CREATE TABLE workforce_webhook_deliveries (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    integration_application_id BIGINT NOT NULL,
    public_event_id BIGINT NOT NULL UNIQUE,
    task_id BIGINT NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    lease_owner VARCHAR(256),
    lease_expires_at TIMESTAMPTZ,
    last_http_status SMALLINT,
    last_failure_class VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at TIMESTAMPTZ
);

CREATE INDEX idx_workforce_webhook_deliveries_claim
    ON workforce_webhook_deliveries (status, next_attempt_at, lease_expires_at);
CREATE INDEX idx_workforce_webhook_deliveries_application
    ON workforce_webhook_deliveries (tenant_id, integration_application_id, updated_at DESC);

CREATE TABLE workforce_webhook_delivery_attempts (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    integration_application_id BIGINT NOT NULL,
    delivery_id BIGINT NOT NULL,
    signing_credential_id BIGINT NOT NULL,
    grace_signing_credential_id BIGINT,
    attempt_number INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    http_status SMALLINT,
    failure_class VARCHAR(128) NOT NULL DEFAULT '',
    duration_millis INTEGER NOT NULL DEFAULT 0,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (delivery_id, attempt_number)
);

CREATE INDEX idx_workforce_webhook_delivery_attempts_delivery
    ON workforce_webhook_delivery_attempts (delivery_id, attempt_number DESC);

COMMENT ON TABLE workforce_integration_applications IS '开放接入应用';
COMMENT ON TABLE workforce_integration_api_credentials IS '开放接入 API 凭据';
COMMENT ON TABLE workforce_integration_signing_credentials IS 'Webhook 签名凭据';
COMMENT ON TABLE workforce_public_events IS '开放接入事件';
COMMENT ON COLUMN workforce_public_events.sequence IS '应用内事件序号';
COMMENT ON TABLE workforce_webhook_deliveries IS 'Webhook 投递任务';
COMMENT ON TABLE workforce_webhook_delivery_attempts IS 'Webhook 投递尝试';
COMMENT ON COLUMN workforce_integration_signing_credentials.signing_secret_ciphertext IS '签名密钥密文';
COMMENT ON COLUMN workforce_integration_signing_credentials.signing_secret_edek IS '加密后的数据密钥';
COMMENT ON COLUMN workforce_webhook_delivery_attempts.duration_millis IS '投递耗时，单位毫秒';

-- +goose Down

DROP TABLE IF EXISTS workforce_webhook_delivery_attempts;
DROP TABLE IF EXISTS workforce_webhook_deliveries;
DROP TABLE IF EXISTS workforce_public_events;
DROP TABLE IF EXISTS workforce_integration_signing_credentials;
DROP TABLE IF EXISTS workforce_integration_api_credentials;
DROP TABLE IF EXISTS workforce_integration_applications;
DROP INDEX IF EXISTS idx_workforce_task_events_open_projection_pending;
ALTER TABLE workforce_task_events
    DROP COLUMN IF EXISTS open_integration_projection_status;
DROP INDEX IF EXISTS idx_workforce_tasks_open_origin;
ALTER TABLE workforce_tasks
    DROP COLUMN IF EXISTS external_correlation_id,
    DROP COLUMN IF EXISTS scenario_digest,
    DROP COLUMN IF EXISTS scenario_version,
    DROP COLUMN IF EXISTS scenario_key,
    DROP COLUMN IF EXISTS integration_application_id,
    DROP COLUMN IF EXISTS submission_channel;
