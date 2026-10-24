package infrastructure

// RevisionSchemaSQL is copied by the parent into the unpublished migration and
// alignment path. It neither changes current Turn inputs nor deletes evidence.
const RevisionSchemaSQL = `
CREATE TABLE IF NOT EXISTS workforce_completion_check_revisions (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),task_id BIGINT NOT NULL CHECK(task_id>0),
 source_task_turn_id BIGINT NOT NULL CHECK(source_task_turn_id>0),source_attempt_id BIGINT NOT NULL CHECK(source_attempt_id>0),
 source_version INTEGER NOT NULL CHECK(source_version>0), revision_number INTEGER NOT NULL CHECK(revision_number>0),
 version INTEGER NOT NULL CHECK(version>0),status VARCHAR(16) NOT NULL CHECK(status IN ('PENDING','APPLIED','CANCELLED','SUPERSEDED')),
 creator_user_id BIGINT NOT NULL CHECK(creator_user_id>0),actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 idempotency_key VARCHAR(128) NOT NULL CHECK(octet_length(idempotency_key) BETWEEN 1 AND 128),
 request_digest VARCHAR(64) NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'),
 base_input_json JSONB NOT NULL CHECK(jsonb_typeof(base_input_json)='object' AND octet_length(base_input_json::text)<=131072),
 base_digest VARCHAR(64) NOT NULL CHECK(base_digest ~ '^[0-9a-f]{64}$'),
 input_json JSONB NOT NULL CHECK(jsonb_typeof(input_json)='object' AND octet_length(input_json::text)<=131072),
 input_digest VARCHAR(64) NOT NULL CHECK(input_digest ~ '^[0-9a-f]{64}$'),
 check_digest VARCHAR(64) NOT NULL CHECK(check_digest ~ '^[0-9a-f]{64}$'),
 references_json JSONB NOT NULL CHECK(jsonb_typeof(references_json)='array' AND octet_length(references_json::text)<=65536),
 source_baseline_result_id BIGINT CHECK(source_baseline_result_id>0),
 source_commit VARCHAR(64) NOT NULL,source_identity_digest VARCHAR(64) NOT NULL CHECK(source_identity_digest ~ '^[0-9a-f]{64}$'),
 execution_snapshot_digest VARCHAR(64) NOT NULL CHECK(execution_snapshot_digest ~ '^[0-9a-f]{64}$'),
 processor_release_id BIGINT NOT NULL CHECK(processor_release_id>0),processor_digest VARCHAR(64) NOT NULL CHECK(processor_digest ~ '^[0-9a-f]{64}$'),
 applied_turn_id BIGINT CHECK(applied_turn_id>0),created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,applied_at TIMESTAMPTZ,
 CHECK((status='APPLIED')=(applied_turn_id IS NOT NULL AND applied_at IS NOT NULL)),
 UNIQUE(tenant_id,task_id,creator_user_id,idempotency_key),UNIQUE(tenant_id,task_id,revision_number),UNIQUE(applied_turn_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_completion_revision_pending_uq ON workforce_completion_check_revisions(tenant_id,task_id) WHERE status='PENDING';
`
