package infrastructure

// Schema is an additive fragment for the parent-owned unpublished migration and
// alignment path. This module never executes it in production. No foreign keys.
// local_request_key and idempotency_key are protocol idempotency identities.
const Schema = `
CREATE TABLE IF NOT EXISTS workforce_agent_interactions (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),
 task_id BIGINT NOT NULL CHECK(task_id>0), task_turn_id BIGINT CHECK(task_turn_id>0), attempt_id BIGINT CHECK(attempt_id>0),
 adapter_session_generation INTEGER CHECK(adapter_session_generation>0),
 local_request_key VARCHAR(128) NOT NULL CHECK(octet_length(local_request_key) BETWEEN 1 AND 128),
 actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 kind VARCHAR(32) NOT NULL CHECK(kind IN ('QUESTION','USER_MESSAGE')),
 is_blocking BOOLEAN NOT NULL DEFAULT TRUE,
 state VARCHAR(16) NOT NULL CHECK(state IN ('OPEN','RESOLVED','EXPIRED','CANCELLED','SUPERSEDED')),
 delivery_state VARCHAR(16) NOT NULL CHECK(delivery_state IN ('NONE','SAVED','DELIVERED')),
 version INTEGER NOT NULL CHECK(version>0), request_digest VARCHAR(64) NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 body_json JSONB NOT NULL CHECK(jsonb_typeof(body_json)='object' AND octet_length(body_json::text)<=65536), created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 expires_at TIMESTAMPTZ NOT NULL DEFAULT (CURRENT_TIMESTAMP + INTERVAL '24 hours'),
 CHECK(expires_at>created_at),
 CHECK(kind<>'QUESTION' OR (task_turn_id IS NOT NULL AND attempt_id IS NOT NULL AND adapter_session_generation IS NOT NULL)),
 UNIQUE(attempt_id,adapter_session_generation,local_request_key)
);
ALTER TABLE workforce_agent_interactions ADD COLUMN IF NOT EXISTS is_blocking BOOLEAN NOT NULL DEFAULT TRUE;
UPDATE workforce_agent_interactions
SET is_blocking=TRUE,
    body_json=jsonb_set(body_json, '{isBlocking}', 'true'::jsonb, true)
WHERE kind='QUESTION' AND NOT (body_json ? 'isBlocking');
CREATE INDEX IF NOT EXISTS workforce_interaction_task_idx ON workforce_agent_interactions(tenant_id,task_id,id);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_interaction_message_key_idx ON workforce_agent_interactions(tenant_id,task_id,actor_principal_id,local_request_key) WHERE kind='USER_MESSAGE';
CREATE TABLE IF NOT EXISTS workforce_agent_interaction_responses (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),
 task_id BIGINT NOT NULL CHECK(task_id>0), interaction_id BIGINT NOT NULL UNIQUE,
 actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 idempotency_key VARCHAR(128) NOT NULL CHECK(octet_length(idempotency_key) BETWEEN 1 AND 128),
 request_digest VARCHAR(64) NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'), answers_json JSONB NOT NULL CHECK(jsonb_typeof(answers_json)='array' AND octet_length(answers_json::text)<=18432),
 created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,task_id,actor_principal_id,idempotency_key)
);
CREATE TABLE IF NOT EXISTS workforce_task_conversation_items (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),
 task_id BIGINT NOT NULL CHECK(task_id>0), interaction_id BIGINT UNIQUE CHECK(interaction_id>0),
 version INTEGER NOT NULL CHECK(version>0), type VARCHAR(32) NOT NULL,
 source_kind VARCHAR(32) NOT NULL, source_key VARCHAR(256) NOT NULL,
 request_digest VARCHAR(64) NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 body_json JSONB NOT NULL CHECK(jsonb_typeof(body_json)='object' AND octet_length(body_json::text)<=65536), created_at TIMESTAMPTZ NOT NULL,
 change_sequence INTEGER NOT NULL CHECK(change_sequence>0),
 UNIQUE(task_id,change_sequence), UNIQUE(tenant_id,task_id,source_kind,source_key)
);
CREATE INDEX IF NOT EXISTS workforce_conversation_changes_idx ON workforce_task_conversation_items(tenant_id,task_id,change_sequence);
CREATE TABLE IF NOT EXISTS workforce_task_conversation_read_states (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),
 task_id BIGINT NOT NULL CHECK(task_id>0), actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 watermark INTEGER NOT NULL CHECK(watermark>=0), updated_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,task_id,actor_principal_id)
);
`
