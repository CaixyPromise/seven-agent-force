package infrastructure

// Schema is an additive fragment for the parent-owned unpublished Workforce
// migration and alignment path. This package never executes it at startup.
// There are deliberately no database foreign keys.
const Schema = `
CREATE TABLE IF NOT EXISTS workforce_execution_operations (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),
 task_id BIGINT NOT NULL CHECK(task_id>0), task_turn_id BIGINT NOT NULL CHECK(task_turn_id>0),
 attempt_id BIGINT NOT NULL CHECK(attempt_id>0), adapter_session_generation INTEGER NOT NULL CHECK(adapter_session_generation>0),
 authorization_grant_id BIGINT CHECK(authorization_grant_id>0),
 local_tool_call_key VARCHAR(128) NOT NULL CHECK(octet_length(local_tool_call_key) BETWEEN 1 AND 128),
 native_tool_call_id TEXT NOT NULL CONSTRAINT workforce_execution_operations_native_tool_call_id_ck CHECK(octet_length(native_tool_call_id) BETWEEN 1 AND 512 AND native_tool_call_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$'),
 round_key VARCHAR(128) NOT NULL CHECK(octet_length(round_key) BETWEEN 1 AND 128),
 operation_kind VARCHAR(32) NOT NULL CHECK(operation_kind='NETWORK_COMMAND'),
 descriptor_ciphertext BYTEA NOT NULL, descriptor_edek BYTEA NOT NULL, descriptor_key_ref VARCHAR(256) NOT NULL,
 descriptor_digest VARCHAR(64) NOT NULL CHECK(descriptor_digest ~ '^[0-9a-f]{64}$'),
 purpose VARCHAR(2048) NOT NULL CHECK(octet_length(purpose) BETWEEN 1 AND 2048),
 targets_json JSONB NOT NULL CHECK(jsonb_typeof(targets_json)='array' AND jsonb_array_length(targets_json) BETWEEN 1 AND 8),
 status VARCHAR(32) NOT NULL CHECK(status IN ('WAITING_APPROVAL','READY','DISPATCHED','RUNNING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN','DENIED','EXPIRED','CANCELLED')),
 version INTEGER NOT NULL CHECK(version>0),
 result_delivery_status VARCHAR(32) NOT NULL DEFAULT '' CHECK(result_delivery_status IN ('','SAVED','DELIVERED_TO_ADAPTER','NATIVE_CALL_RESOLVED')),
	agent_continued BOOLEAN NOT NULL DEFAULT FALSE,
	result_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK(result_digest='' OR result_digest ~ '^[0-9a-f]{64}$'),
	result_summary TEXT NOT NULL DEFAULT '' CHECK(octet_length(result_summary)<=2048), exit_code INTEGER,
	result_output_ciphertext BYTEA, result_output_edek BYTEA, result_output_key_ref VARCHAR(256),
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
 started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ,
 UNIQUE(attempt_id,adapter_session_generation,local_tool_call_key),
	CHECK(expires_at>created_at),
	CHECK((result_output_ciphertext IS NULL AND result_output_edek IS NULL AND result_output_key_ref IS NULL) OR (result_output_ciphertext IS NOT NULL AND result_output_edek IS NOT NULL AND result_output_key_ref IS NOT NULL)),
 CHECK(status NOT IN ('SUCCEEDED','FAILED','OUTCOME_UNKNOWN','DENIED') OR finished_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS workforce_execution_operation_task_idx ON workforce_execution_operations(tenant_id,task_id,id DESC);
CREATE INDEX IF NOT EXISTS workforce_execution_operation_result_idx ON workforce_execution_operations(attempt_id,adapter_session_generation,result_delivery_status,id);

CREATE TABLE IF NOT EXISTS workforce_execution_operation_decisions (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 operation_id BIGINT NOT NULL UNIQUE CHECK(operation_id>0), actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 action VARCHAR(32) NOT NULL CHECK(action IN ('ALLOW_ONCE','ALLOW_FOR_TASK','DENY')),
 idempotency_key VARCHAR(128) NOT NULL CHECK(octet_length(idempotency_key) BETWEEN 1 AND 128),
 request_digest VARCHAR(64) NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 reason VARCHAR(2000) NOT NULL DEFAULT '' CHECK(octet_length(reason)<=2000), status VARCHAR(16) NOT NULL CHECK(status='APPLIED'),
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,task_id,actor_principal_id,idempotency_key)
);

CREATE TABLE IF NOT EXISTS workforce_task_network_grants (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 source_decision_id BIGINT NOT NULL UNIQUE CHECK(source_decision_id>0), actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 targets_json JSONB NOT NULL CHECK(jsonb_typeof(targets_json)='array' AND jsonb_array_length(targets_json) BETWEEN 1 AND 8),
 targets_digest VARCHAR(64) NOT NULL CHECK(targets_digest ~ '^[0-9a-f]{64}$'),
 status VARCHAR(16) NOT NULL CHECK(status IN ('ACTIVE','REVOKED','EXPIRED')), version INTEGER NOT NULL CHECK(version>0),
 revoke_idempotency_key VARCHAR(128), revoke_request_digest VARCHAR(64),
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ,
 CHECK(expires_at>created_at),
 CHECK((revoke_idempotency_key IS NULL AND revoke_request_digest IS NULL) OR (octet_length(revoke_idempotency_key) BETWEEN 1 AND 128 AND revoke_request_digest ~ '^[0-9a-f]{64}$'))
);
CREATE INDEX IF NOT EXISTS workforce_task_network_grant_task_idx ON workforce_task_network_grants(tenant_id,task_id,status,id DESC);

CREATE TABLE IF NOT EXISTS workforce_execution_operation_attempts (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 operation_id BIGINT NOT NULL CHECK(operation_id>0), attempt_number INTEGER NOT NULL CHECK(attempt_number>0),
 worker_id BIGINT NOT NULL CHECK(worker_id>0), worker_instance_id VARCHAR(128) NOT NULL, host_boot_id VARCHAR(128) NOT NULL,
 owner_epoch INTEGER NOT NULL CHECK(owner_epoch>0), lease_digest VARCHAR(64) NOT NULL CHECK(lease_digest ~ '^[0-9a-f]{64}$'), lease_expires_at TIMESTAMPTZ NOT NULL,
 status VARCHAR(32) NOT NULL CHECK(status IN ('DISPATCHED','RUNNING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN')),
 container_id TEXT NOT NULL DEFAULT '', relay_identity TEXT NOT NULL DEFAULT '', result_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK(result_digest='' OR result_digest ~ '^[0-9a-f]{64}$'), exit_code INTEGER,
 created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ,
 UNIQUE(operation_id,attempt_number)
);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_execution_operation_one_active_attempt_idx ON workforce_execution_operation_attempts(operation_id) WHERE status IN ('DISPATCHED','RUNNING');
`
