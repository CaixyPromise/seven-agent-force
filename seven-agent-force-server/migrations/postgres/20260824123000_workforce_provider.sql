-- +goose Up

CREATE TABLE workforce_model_credentials (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    name VARCHAR(100) NOT NULL,
    protocol VARCHAR(16) NOT NULL,
    base_url VARCHAR(2048) NOT NULL,
    secret_ciphertext BYTEA,
    secret_edek BYTEA,
    secret_wrap_key_ref VARCHAR(256),
    secret_hint VARCHAR(16) NOT NULL DEFAULT '',
    secret_version INTEGER NOT NULL DEFAULT 1,
    models_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    status VARCHAR(16) NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    last_discovery_status VARCHAR(32) NOT NULL DEFAULT '',
    last_discovery_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    last_discovered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (protocol IN ('OPENAI', 'ANTHROPIC')),
    CHECK (status IN ('ACTIVE', 'DISABLED', 'DELETED')),
    CHECK (secret_version > 0),
    CHECK (version > 0),
    CHECK (
        (status='DELETED' AND secret_ciphertext IS NULL AND secret_edek IS NULL AND secret_wrap_key_ref IS NULL AND secret_hint='')
        OR
        (status IN ('ACTIVE','DISABLED') AND secret_ciphertext IS NOT NULL AND secret_edek IS NOT NULL AND secret_wrap_key_ref IS NOT NULL AND octet_length(secret_ciphertext) BETWEEN 1 AND 16384 AND octet_length(secret_edek) BETWEEN 1 AND 4096 AND btrim(secret_wrap_key_ref)<>'')
    ),
    CHECK (jsonb_typeof(models_json) = 'array'),
    CHECK (jsonb_array_length(models_json) <= 5000)
);

COMMENT ON TABLE workforce_model_credentials IS '模型密钥配置';
COMMENT ON COLUMN workforce_model_credentials.id IS '模型密钥编号';
COMMENT ON COLUMN workforce_model_credentials.tenant_id IS '所属组织编号';
COMMENT ON COLUMN workforce_model_credentials.name IS '模型密钥名称';
COMMENT ON COLUMN workforce_model_credentials.protocol IS '接口协议';
COMMENT ON COLUMN workforce_model_credentials.base_url IS '服务基础地址';
COMMENT ON COLUMN workforce_model_credentials.secret_ciphertext IS 'API Key 密文';
COMMENT ON COLUMN workforce_model_credentials.secret_edek IS '加密后的数据密钥';
COMMENT ON COLUMN workforce_model_credentials.secret_wrap_key_ref IS '主密钥引用';
COMMENT ON COLUMN workforce_model_credentials.secret_hint IS '密钥尾号提示';
COMMENT ON COLUMN workforce_model_credentials.secret_version IS '密钥版本';
COMMENT ON COLUMN workforce_model_credentials.models_json IS '可选模型列表';
COMMENT ON COLUMN workforce_model_credentials.status IS '使用状态';
COMMENT ON COLUMN workforce_model_credentials.version IS '配置版本';
COMMENT ON COLUMN workforce_model_credentials.last_discovery_status IS '模型发现状态';
COMMENT ON COLUMN workforce_model_credentials.last_discovery_reason_code IS '模型发现结果码';
COMMENT ON COLUMN workforce_model_credentials.last_discovered_at IS '模型发现时间';
COMMENT ON COLUMN workforce_model_credentials.created_at IS '创建时间';
COMMENT ON COLUMN workforce_model_credentials.updated_at IS '更新时间';

CREATE TABLE workforce_model_invocations (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    task_id BIGINT NOT NULL,
    protocol_invocation_key VARCHAR(64) NOT NULL,
    execution_kind VARCHAR(32) NOT NULL DEFAULT 'AGENT_ATTEMPT',
    attempt_id BIGINT,
    review_run_id BIGINT,
    review_run_attempt_id BIGINT,
    generation INTEGER NOT NULL,
    owner_epoch INTEGER NOT NULL,
    worker_id BIGINT NOT NULL,
    worker_instance_id VARCHAR(128) NOT NULL,
    host_boot_id VARCHAR(128) NOT NULL,
    model_credential_id BIGINT NOT NULL,
    credential_version INTEGER NOT NULL,
    protocol VARCHAR(16) NOT NULL,
    primary_model VARCHAR(256) NOT NULL,
    requested_model VARCHAR(256) NOT NULL,
    response_model VARCHAR(256),
    operation VARCHAR(32) NOT NULL,
    is_primary_model BOOLEAN NOT NULL,
    request_body_sha256 BYTEA NOT NULL,
    nonce_sha256 BYTEA NOT NULL,
    status VARCHAR(32) NOT NULL,
    upstream_status_code SMALLINT,
    upstream_request_id VARCHAR(256),
    headers_at TIMESTAMPTZ,
    first_byte_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration_milliseconds INTEGER,
    input_tokens INTEGER,
    cached_input_tokens INTEGER,
    cache_creation_input_tokens INTEGER,
    output_tokens INTEGER,
    reasoning_output_tokens INTEGER,
    total_tokens INTEGER,
    usage_json JSONB,
    error_code VARCHAR(128),
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT workforce_model_invocations_execution_kind_ck
        CHECK (execution_kind IN ('AGENT_ATTEMPT','REVIEW_RUN')),
    CONSTRAINT workforce_model_invocations_owner_epoch_ck CHECK (owner_epoch > 0),
    CONSTRAINT workforce_model_invocations_execution_identity_ck CHECK (
        (execution_kind='AGENT_ATTEMPT' AND attempt_id IS NOT NULL AND review_run_id IS NULL AND review_run_attempt_id IS NULL AND generation > 0)
        OR (execution_kind='REVIEW_RUN' AND attempt_id IS NULL AND review_run_id IS NOT NULL AND review_run_attempt_id IS NOT NULL AND generation = 0)
    ),
    CHECK (protocol IN ('OPENAI', 'ANTHROPIC')),
    CHECK (credential_version > 0),
    CHECK (octet_length(request_body_sha256) = 32),
    CHECK (octet_length(nonce_sha256) = 32),
    CHECK (operation IN ('responses.create','responses.compact','messages.create','messages.count_tokens')),
    CHECK (status IN ('AUTHORIZED','STREAMING','SUCCEEDED','FAILED','CANCELLED','OUTCOME_UNKNOWN')),
    CHECK (upstream_status_code IS NULL OR upstream_status_code BETWEEN 100 AND 599),
    CHECK (duration_milliseconds IS NULL OR duration_milliseconds >= 0),
    CHECK (input_tokens IS NULL OR input_tokens >= 0),
    CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
    CHECK (cache_creation_input_tokens IS NULL OR cache_creation_input_tokens >= 0),
    CHECK (output_tokens IS NULL OR output_tokens >= 0),
    CHECK (reasoning_output_tokens IS NULL OR reasoning_output_tokens >= 0),
    CHECK (total_tokens IS NULL OR total_tokens >= 0),
    CONSTRAINT workforce_model_invocations_protocol_key_uk
        UNIQUE (tenant_id, protocol_invocation_key),
    UNIQUE (worker_id, nonce_sha256)
);

COMMENT ON TABLE workforce_model_invocations IS '模型调用记录';
COMMENT ON COLUMN workforce_model_invocations.id IS '调用编号';
COMMENT ON COLUMN workforce_model_invocations.tenant_id IS '所属组织编号';
COMMENT ON COLUMN workforce_model_invocations.task_id IS '所属任务编号';
COMMENT ON COLUMN workforce_model_invocations.protocol_invocation_key IS '请求唯一标识';
COMMENT ON COLUMN workforce_model_invocations.execution_kind IS '执行类型';
COMMENT ON COLUMN workforce_model_invocations.attempt_id IS '任务执行编号';
COMMENT ON COLUMN workforce_model_invocations.review_run_id IS '独立审查编号';
COMMENT ON COLUMN workforce_model_invocations.review_run_attempt_id IS '独立审查执行编号';
COMMENT ON COLUMN workforce_model_invocations.generation IS '任务执行代次';
COMMENT ON COLUMN workforce_model_invocations.owner_epoch IS '执行权版本';
COMMENT ON COLUMN workforce_model_invocations.worker_id IS 'Worker 编号';
COMMENT ON COLUMN workforce_model_invocations.worker_instance_id IS 'Worker 进程标识';
COMMENT ON COLUMN workforce_model_invocations.host_boot_id IS '主机启动标识';
COMMENT ON COLUMN workforce_model_invocations.model_credential_id IS '模型密钥编号';
COMMENT ON COLUMN workforce_model_invocations.credential_version IS '密钥版本';
COMMENT ON COLUMN workforce_model_invocations.protocol IS '接口协议';
COMMENT ON COLUMN workforce_model_invocations.primary_model IS '任务主模型';
COMMENT ON COLUMN workforce_model_invocations.requested_model IS '请求模型';
COMMENT ON COLUMN workforce_model_invocations.response_model IS '响应模型';
COMMENT ON COLUMN workforce_model_invocations.operation IS '调用操作';
COMMENT ON COLUMN workforce_model_invocations.is_primary_model IS '是否使用任务主模型';
COMMENT ON COLUMN workforce_model_invocations.request_body_sha256 IS '请求内容摘要';
COMMENT ON COLUMN workforce_model_invocations.nonce_sha256 IS '请求随机数摘要';
COMMENT ON COLUMN workforce_model_invocations.status IS '调用状态';
COMMENT ON COLUMN workforce_model_invocations.upstream_status_code IS '上游 HTTP 状态码';
COMMENT ON COLUMN workforce_model_invocations.upstream_request_id IS '上游请求编号';
COMMENT ON COLUMN workforce_model_invocations.headers_at IS '响应头到达时间';
COMMENT ON COLUMN workforce_model_invocations.first_byte_at IS '首字节到达时间';
COMMENT ON COLUMN workforce_model_invocations.finished_at IS '完成时间';
COMMENT ON COLUMN workforce_model_invocations.duration_milliseconds IS '调用耗时（毫秒）';
COMMENT ON COLUMN workforce_model_invocations.input_tokens IS '输入 Token 数量';
COMMENT ON COLUMN workforce_model_invocations.cached_input_tokens IS '缓存命中 Token 数量';
COMMENT ON COLUMN workforce_model_invocations.cache_creation_input_tokens IS '缓存写入 Token 数量';
COMMENT ON COLUMN workforce_model_invocations.output_tokens IS '输出 Token 数量';
COMMENT ON COLUMN workforce_model_invocations.reasoning_output_tokens IS '推理 Token 数量';
COMMENT ON COLUMN workforce_model_invocations.total_tokens IS '总 Token 数量';
COMMENT ON COLUMN workforce_model_invocations.usage_json IS '补充用量数据';
COMMENT ON COLUMN workforce_model_invocations.error_code IS '错误码';
COMMENT ON COLUMN workforce_model_invocations.started_at IS '开始时间';

CREATE INDEX workforce_model_credentials_tenant_status_idx
    ON workforce_model_credentials (tenant_id, status, name);
CREATE UNIQUE INDEX workforce_model_credentials_tenant_name_uidx
    ON workforce_model_credentials (tenant_id, name) WHERE status<>'DELETED';
CREATE INDEX workforce_model_invocations_task_idx
    ON workforce_model_invocations (tenant_id, task_id, started_at DESC);
CREATE INDEX workforce_model_invocations_credential_idx
    ON workforce_model_invocations (tenant_id, model_credential_id, started_at DESC);
CREATE INDEX workforce_model_invocations_active_idx
    ON workforce_model_invocations (tenant_id, attempt_id, status)
    WHERE status IN ('AUTHORIZED','STREAMING');
CREATE INDEX workforce_model_invocations_review_run_active_idx
    ON workforce_model_invocations (tenant_id, review_run_id, review_run_attempt_id, status)
    WHERE execution_kind='REVIEW_RUN' AND status IN ('AUTHORIZED','STREAMING');

-- +goose Down

DROP TABLE IF EXISTS workforce_model_invocations;
DROP TABLE IF EXISTS workforce_model_credentials;
