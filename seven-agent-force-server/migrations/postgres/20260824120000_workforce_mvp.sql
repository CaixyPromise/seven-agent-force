-- +goose Up

CREATE TABLE workforce_principals (
    id BIGINT PRIMARY KEY,
    principal_type VARCHAR(16) NOT NULL,
    user_id BIGINT,
    worker_id BIGINT,
    service_code VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	CONSTRAINT workforce_principals_type_ck CHECK (principal_type IN ('USER','WORKER','SERVICE')),
	CONSTRAINT workforce_principals_shape_ck CHECK (
        (principal_type='USER' AND user_id IS NOT NULL AND worker_id IS NULL AND service_code IS NULL) OR
        (principal_type='WORKER' AND user_id IS NULL AND worker_id IS NOT NULL AND service_code IS NULL) OR
        (principal_type='SERVICE' AND user_id IS NULL AND worker_id IS NULL AND service_code IS NOT NULL AND service_code<>'')
    )
);
CREATE UNIQUE INDEX workforce_principals_user_uidx ON workforce_principals (user_id) WHERE principal_type='USER';
CREATE UNIQUE INDEX workforce_principals_worker_uidx ON workforce_principals (worker_id) WHERE principal_type='WORKER';
CREATE UNIQUE INDEX workforce_principals_service_uidx ON workforce_principals (service_code) WHERE principal_type='SERVICE';

-- Read-only compatibility index for immutable pre-Snowflake evidence and old
-- GET links. New entities never write a legacy identity.
CREATE TABLE workforce_legacy_identity_map (
    entity_type VARCHAR(64) NOT NULL,
    legacy_identity VARCHAR(256) NOT NULL,
    entity_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (entity_type,legacy_identity),
    UNIQUE (entity_type,entity_id)
);

CREATE TABLE workforce_harness_releases (
    id BIGINT PRIMARY KEY,
    alias VARCHAR(128) NOT NULL UNIQUE,
    image_ref VARCHAR(1024) NOT NULL,
    image_digest VARCHAR(71) NOT NULL,
    contract_version VARCHAR(64) NOT NULL,
    supervisor_version VARCHAR(64) NOT NULL,
    harness_type VARCHAR(64) NOT NULL,
    harness_version VARCHAR(64) NOT NULL,
    capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    scheduling_scope VARCHAR(16) NOT NULL DEFAULT 'PRODUCTION'
        CONSTRAINT workforce_harness_releases_scheduling_scope_ck
        CHECK (scheduling_scope IN ('PRODUCTION','TEST_ONLY')),
    scheduling_priority INTEGER NOT NULL DEFAULT 100
        CONSTRAINT workforce_harness_releases_scheduling_priority_ck
        CHECK (scheduling_priority BETWEEN -100000 AND 100000),
    provider_protocols_json JSONB NOT NULL DEFAULT '[]'::jsonb
        CONSTRAINT workforce_harness_releases_provider_protocols_ck
        CHECK (jsonb_typeof(provider_protocols_json)='array'),
    environment_capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb
        CONSTRAINT workforce_harness_releases_environment_capabilities_ck
        CHECK (jsonb_typeof(environment_capabilities_json)='array'),
    hook_events_json JSONB NOT NULL DEFAULT '[]'::jsonb
        CONSTRAINT workforce_harness_releases_hook_events_ck
        CHECK (jsonb_typeof(hook_events_json)='array'),
    execution_kinds_json JSONB NOT NULL DEFAULT '["AGENT_ATTEMPT","REVIEW_RUN"]'::jsonb
        CONSTRAINT workforce_harness_releases_execution_kinds_ck
        CHECK (
            jsonb_typeof(execution_kinds_json)='array'
            AND jsonb_array_length(execution_kinds_json)>0
            AND execution_kinds_json <@ '["AGENT_ATTEMPT","REVIEW_RUN"]'::jsonb
        ),
    scheduling_digest VARCHAR(64) NOT NULL DEFAULT ''
        CONSTRAINT workforce_harness_releases_scheduling_digest_ck
        CHECK (scheduling_digest='' OR scheduling_digest ~ '^[0-9a-f]{64}$'),
    manifest_json JSONB NOT NULL DEFAULT '{}'::jsonb
        CONSTRAINT workforce_harness_releases_manifest_ck
        CHECK (jsonb_typeof(manifest_json)='object'),
    admission_evidence_json JSONB NOT NULL,
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
COMMENT ON COLUMN workforce_harness_releases.provider_protocols_json IS '镜像支持的模型协议';
COMMENT ON COLUMN workforce_harness_releases.hook_events_json IS '镜像支持的 Hook 事件';
COMMENT ON COLUMN workforce_harness_releases.manifest_json IS '准入通过的镜像清单';

CREATE TABLE workforce_image_candidates (
    id BIGINT PRIMARY KEY,
    requested_ref VARCHAR(1024) NOT NULL,
    resolved_digest VARCHAR(71),
    requested_harness_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    rejection_code VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_image_admission_runs (
    id BIGINT PRIMARY KEY,
    candidate_id BIGINT NOT NULL,
    worker_id BIGINT NOT NULL,
    image_digest VARCHAR(71) NOT NULL,
    contract_version VARCHAR(64),
    supervisor_version VARCHAR(64),
    harness_type VARCHAR(64),
    harness_version VARCHAR(64),
    declared_manifest_json JSONB,
    observed_probe_json JSONB,
    checks_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    verdict VARCHAR(32) NOT NULL,
    error_code VARCHAR(128),
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMPTZ
);

CREATE TABLE workforce_tasks (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
	created_by BIGINT NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    repository_url VARCHAR(2048) NOT NULL,
    repository_ref VARCHAR(256) NOT NULL,
    resolved_base_commit VARCHAR(64),
    task_instruction TEXT NOT NULL,
    validation_commands_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(32) NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 1,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    result_summary TEXT,
    latest_attempt_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    queued_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 0,
    UNIQUE (tenant_id, created_by, idempotency_key)
);

CREATE TABLE workforce_task_attempts (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    attempt_no INTEGER NOT NULL,
    generation INTEGER NOT NULL DEFAULT 1,
    worker_id BIGINT NOT NULL,
    container_id VARCHAR(128),
    workspace_path VARCHAR(2048) NOT NULL,
    status VARCHAR(32) NOT NULL,
    lease_token VARCHAR(256) NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    last_heartbeat_at TIMESTAMPTZ,
    last_progress_at TIMESTAMPTZ,
    runtime_driver VARCHAR(64) NOT NULL,
    runtime_image_digest VARCHAR(71),
    image_contract_version VARCHAR(64),
    supervisor_protocol_version INTEGER,
    base_commit VARCHAR(64),
    result_commit VARCHAR(64),
    result_tree VARCHAR(64),
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMPTZ,
    exit_code INTEGER,
    error_code VARCHAR(128),
    error_message TEXT,
    runtime_metrics_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (task_id, attempt_no)
);

CREATE TABLE workforce_harness_sessions (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    provider VARCHAR(64) NOT NULL,
    provider_session_id VARCHAR(256),
    status VARCHAR(32) NOT NULL,
    resume_count INTEGER NOT NULL DEFAULT 0,
    cli_version VARCHAR(64) NOT NULL,
    adapter_version VARCHAR(64) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    stopped_at TIMESTAMPTZ
);

CREATE TABLE workforce_completion_proposals (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    harness_session_id BIGINT NOT NULL,
    schema_version INTEGER NOT NULL,
    raw_json JSONB NOT NULL,
    validation_status VARCHAR(32) NOT NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_task_events (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL,
    attempt_id BIGINT,
    event_type VARCHAR(128) NOT NULL,
    sequence INTEGER NOT NULL,
    payload_json JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE NULLS NOT DISTINCT (task_id, attempt_id, sequence)
);

CREATE TABLE workforce_artifacts (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    artifact_type VARCHAR(64) NOT NULL,
    name VARCHAR(512) NOT NULL,
    object_key VARCHAR(1024) NOT NULL UNIQUE,
    sha256 VARCHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL,
    media_type VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_check_runs (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    ordinal INTEGER NOT NULL,
    name VARCHAR(255) NOT NULL,
    command_digest VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    exit_code INTEGER,
    timeout_seconds INTEGER NOT NULL,
    output TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE (attempt_id, ordinal)
);

CREATE TABLE workforce_outbox_events (
    id BIGINT PRIMARY KEY,
    aggregate_type VARCHAR(64) NOT NULL,
    aggregate_id BIGINT NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    payload_json JSONB NOT NULL,
    status VARCHAR(32) NOT NULL,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMPTZ,
    last_error TEXT
);

CREATE INDEX workforce_tasks_status_created_idx ON workforce_tasks (status, priority DESC, created_at);
CREATE INDEX workforce_task_events_task_cursor_idx ON workforce_task_events (task_id, id);
CREATE INDEX workforce_outbox_events_publish_idx ON workforce_outbox_events (status, next_retry_at, created_at);
CREATE INDEX workforce_task_attempts_lease_idx ON workforce_task_attempts (status, lease_expires_at);
CREATE INDEX workforce_artifacts_task_created_idx ON workforce_artifacts (task_id, created_at);

COMMENT ON TABLE workforce_principals IS '执行主体';
COMMENT ON TABLE workforce_legacy_identity_map IS '旧标识与内部 ID 的映射';
COMMENT ON TABLE workforce_harness_releases IS 'Agent 运行环境发布版本';
COMMENT ON TABLE workforce_image_candidates IS '待验证的运行镜像';
COMMENT ON TABLE workforce_image_admission_runs IS '运行镜像验证记录';
COMMENT ON TABLE workforce_tasks IS '开发任务';
COMMENT ON TABLE workforce_task_attempts IS '任务执行尝试';
COMMENT ON TABLE workforce_harness_sessions IS 'Agent 会话';
COMMENT ON TABLE workforce_completion_proposals IS 'Agent 提交的完成提议';
COMMENT ON TABLE workforce_task_events IS '任务事件';
COMMENT ON COLUMN workforce_task_events.sequence IS '任务内事件序号';
COMMENT ON TABLE workforce_artifacts IS '任务产物';
COMMENT ON TABLE workforce_check_runs IS '完成检查记录';
COMMENT ON TABLE workforce_outbox_events IS '待发布事件';
COMMENT ON COLUMN workforce_tasks.version IS '任务版本号';
COMMENT ON COLUMN workforce_task_attempts.generation IS '执行尝试代次';
COMMENT ON COLUMN workforce_artifacts.size_bytes IS '产物字节数';

-- +goose Down

DROP TABLE IF EXISTS workforce_outbox_events;
DROP TABLE IF EXISTS workforce_check_runs;
DROP TABLE IF EXISTS workforce_artifacts;
DROP TABLE IF EXISTS workforce_task_events;
DROP TABLE IF EXISTS workforce_completion_proposals;
DROP TABLE IF EXISTS workforce_harness_sessions;
DROP TABLE IF EXISTS workforce_task_attempts;
DROP TABLE IF EXISTS workforce_tasks;
DROP TABLE IF EXISTS workforce_image_admission_runs;
DROP TABLE IF EXISTS workforce_image_candidates;
DROP TABLE IF EXISTS workforce_harness_releases;
DROP TABLE IF EXISTS workforce_legacy_identity_map;
DROP TABLE IF EXISTS workforce_principals;
