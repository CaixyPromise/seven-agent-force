package infrastructure

// SchemaSQL is shared by the unpublished migration alignment path.
const SchemaSQL = `
CREATE TABLE IF NOT EXISTS workforce_completion_check_cycle_authorizations (
 cycle_id BIGINT PRIMARY KEY CHECK(cycle_id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),
 task_id BIGINT NOT NULL CHECK(task_id>0),task_turn_id BIGINT NOT NULL CHECK(task_turn_id>0),
 attempt_id BIGINT NOT NULL CHECK(attempt_id>0),cycle_number INTEGER NOT NULL CHECK(cycle_number>0),
 UNIQUE(attempt_id,cycle_number)
);
ALTER TABLE workforce_review_runs ADD COLUMN IF NOT EXISTS completion_check_digest VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE workforce_review_runs DROP CONSTRAINT IF EXISTS workforce_review_runs_completion_check_digest_ck;
ALTER TABLE workforce_review_runs ADD CONSTRAINT workforce_review_runs_completion_check_digest_ck CHECK (completion_check_digest='' OR completion_check_digest ~ '^[0-9a-f]{64}$');
ALTER TABLE workforce_task_turns ADD COLUMN IF NOT EXISTS completion_check_json JSONB;
ALTER TABLE workforce_task_turns ADD COLUMN IF NOT EXISTS completion_check_input_digest VARCHAR(64);
ALTER TABLE workforce_task_turns ADD COLUMN IF NOT EXISTS completion_check_baseline_result_id BIGINT;
ALTER TABLE workforce_task_turns ADD COLUMN IF NOT EXISTS completion_check_digest VARCHAR(64);
ALTER TABLE workforce_task_turns ADD COLUMN IF NOT EXISTS completion_check_references_json JSONB;
CREATE TABLE IF NOT EXISTS workforce_completion_check_results (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 task_turn_id BIGINT NOT NULL CHECK(task_turn_id>0), attempt_id BIGINT NOT NULL CHECK(attempt_id>0),
 evaluation_cycle_id BIGINT,
 phase VARCHAR(16) NOT NULL CHECK(phase IN ('BASELINE','CURRENT')),
 check_digest VARCHAR(64) NOT NULL CHECK(check_digest ~ '^[0-9a-f]{64}$'),
 status VARCHAR(32) NOT NULL CHECK(status IN ('PASSED','FAILED','UNAVAILABLE','NO_TESTS','NOT_CONFIGURED','TIMED_OUT')),
 summary VARCHAR(4000) NOT NULL DEFAULT '', observation_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CHECK((phase='BASELINE' AND evaluation_cycle_id IS NULL) OR (phase='CURRENT' AND evaluation_cycle_id>0))
);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_completion_check_result_occurrence ON workforce_completion_check_results(tenant_id,attempt_id,phase,COALESCE(evaluation_cycle_id,0),check_digest);
ALTER TABLE workforce_completion_check_results ADD COLUMN IF NOT EXISTS source_commit VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE workforce_completion_check_results ADD COLUMN IF NOT EXISTS source_identity_digest VARCHAR(64) NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS workforce_completion_check_decisions (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 source_task_turn_id BIGINT NOT NULL CHECK(source_task_turn_id>0), task_turn_id BIGINT NOT NULL CHECK(task_turn_id>0),
 checkpoint_id BIGINT NOT NULL CHECK(checkpoint_id>0), actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 action VARCHAR(32) NOT NULL CHECK(action IN ('CONTINUE_WITH_LIMITATIONS','AUTHORIZE_REPAIR','ADD_VERIFICATION')),
 result_id BIGINT NOT NULL CHECK(result_id>0), baseline_result_id BIGINT NOT NULL CHECK(baseline_result_id>0),
 reason VARCHAR(2000) NOT NULL, repair_scope VARCHAR(20000) NOT NULL DEFAULT '', idempotency_key VARCHAR(128) NOT NULL, request_digest VARCHAR(64) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(tenant_id,task_id,idempotency_key), UNIQUE(tenant_id,checkpoint_id)
);`
