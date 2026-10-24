package infrastructure

// InteractionSchemaSQL is copied by the parent into the unpublished migration
// and transactional alignment. This module never applies global schema itself.
const InteractionSchemaSQL = `
ALTER TABLE workforce_continuation_checkpoints
 ADD COLUMN IF NOT EXISTS purpose VARCHAR(32) NOT NULL DEFAULT 'RESULT',
 ADD COLUMN IF NOT EXISTS interaction_id BIGINT,
 ADD COLUMN IF NOT EXISTS operation_id BIGINT,
 ADD COLUMN IF NOT EXISTS interaction_phase VARCHAR(16),
 ADD COLUMN IF NOT EXISTS adapter_session_generation INTEGER,
 ADD COLUMN IF NOT EXISTS automatic_revision_count INTEGER,
 ADD COLUMN IF NOT EXISTS interaction_round BIGINT,
 ADD COLUMN IF NOT EXISTS suspend_control_key_sha256 VARCHAR(64),
 ADD COLUMN IF NOT EXISTS interaction_manifest_json JSONB;
ALTER TABLE workforce_continuation_checkpoints ALTER COLUMN evaluation_cycle_id DROP NOT NULL;
ALTER TABLE workforce_continuation_checkpoints DROP CONSTRAINT IF EXISTS workforce_checkpoint_interaction_manifest_ck;
-- Batch 3 persisted interaction manifests before operation-backed suspension
-- existed. Preserve their immutable fields and add the explicit zero identity
-- required by the unified suspend manifest contract.
UPDATE workforce_continuation_checkpoints
SET interaction_manifest_json=interaction_manifest_json || jsonb_build_object('operationId','0')
WHERE purpose='INTERACTION_SUSPEND' AND interaction_id IS NOT NULL
  AND interaction_manifest_json IS NOT NULL AND NOT (interaction_manifest_json ? 'operationId');
ALTER TABLE workforce_continuation_checkpoints DROP CONSTRAINT IF EXISTS workforce_checkpoint_purpose_ck;
ALTER TABLE workforce_continuation_checkpoints ADD CONSTRAINT workforce_checkpoint_purpose_ck CHECK (
 (purpose='RESULT' AND interaction_id IS NULL AND operation_id IS NULL AND interaction_phase IS NULL AND evaluation_cycle_id IS NOT NULL
  AND adapter_session_generation IS NULL AND automatic_revision_count IS NULL AND interaction_manifest_json IS NULL AND interaction_round IS NULL AND suspend_control_key_sha256 IS NULL)
 OR
 (purpose='INTERACTION_SUSPEND' AND ((interaction_id>0 AND operation_id IS NULL) OR (operation_id>0 AND interaction_id IS NULL)) AND evaluation_cycle_id IS NULL
  AND decision='' AND format_version=2 AND interaction_phase IS NOT NULL AND interaction_phase IN ('CAPTURING','SUSPENDED','RESUMED')
  AND adapter_session_generation>0 AND adapter_session_generation IS NOT NULL
  AND automatic_revision_count>=0 AND automatic_revision_count IS NOT NULL
  AND interaction_round>0 AND interaction_round IS NOT NULL AND suspend_control_key_sha256 IS NOT NULL AND suspend_control_key_sha256 ~ '^[a-f0-9]{64}$'
  AND approved_by_principal_id IS NULL AND approved_at IS NULL
  AND ((interaction_phase='CAPTURING' AND status IN ('PENDING','FAILED','INVALIDATED','EXPIRED') AND consumed_by_turn_id IS NULL AND consumed_at IS NULL AND interaction_manifest_json IS NULL)
    OR (interaction_phase='SUSPENDED' AND status IN ('AVAILABLE','FAILED','INVALIDATED','EXPIRED') AND available_at IS NOT NULL AND consumed_by_turn_id IS NULL AND consumed_at IS NULL AND interaction_manifest_json IS NOT NULL)
    OR (interaction_phase='RESUMED' AND status IN ('AVAILABLE','FAILED','INVALIDATED','EXPIRED') AND consumed_by_turn_id IS NOT NULL AND consumed_at IS NOT NULL AND interaction_manifest_json IS NOT NULL)))
);
ALTER TABLE workforce_continuation_checkpoints ADD CONSTRAINT workforce_checkpoint_interaction_manifest_ck CHECK (
 interaction_manifest_json IS NULL OR (jsonb_typeof(interaction_manifest_json)='object'
 AND octet_length(interaction_manifest_json::text)<=1048576
 AND interaction_manifest_json ?& ARRAY['schemaVersion','format','purpose','decision','checkpointIdentity','taskId','taskTurnId','attemptId','generation','interactionId','operationId','adapterSessionGeneration','automaticRevisionCount','sessionDigest','taskExecutionSnapshotDigest','baseCommit','resultCommit','resultTree','selectionDigest','repositoryBundle','workspaceOverlay','session','executionBudget','currentRound','suspendControlKeySha256']
 AND (interaction_manifest_json-ARRAY['schemaVersion','format','purpose','decision','checkpointIdentity','taskId','taskTurnId','attemptId','generation','interactionId','operationId','adapterSessionGeneration','automaticRevisionCount','sessionDigest','taskExecutionSnapshotDigest','baseCommit','resultCommit','resultTree','selectionDigest','repositoryBundle','workspaceOverlay','session','executionBudget','currentRound','suspendControlKeySha256'])='{}'::jsonb
 AND interaction_manifest_json->>'currentRound'=interaction_round::text
 AND interaction_manifest_json->>'suspendControlKeySha256'=suspend_control_key_sha256
 AND jsonb_typeof(interaction_manifest_json->'executionBudget')='object'
 AND (interaction_manifest_json->'executionBudget') ?& ARRAY['version','taskId','attemptId','generation','ownerEpoch','activeLimitMillis','activeElapsedMillis','interactionWaitingMillis','absoluteDeadlineUnixMillis']
 AND ((interaction_manifest_json->'executionBudget')-ARRAY['version','taskId','attemptId','generation','ownerEpoch','activeLimitMillis','activeElapsedMillis','interactionWaitingMillis','absoluteDeadlineUnixMillis'])='{}'::jsonb
 AND interaction_manifest_json->'executionBudget'->>'version'='1'
 AND interaction_manifest_json->'executionBudget'->>'taskId'=task_id::text
 AND interaction_manifest_json->'executionBudget'->>'attemptId'=attempt_id::text
 AND jsonb_typeof(interaction_manifest_json->'repositoryBundle')='object'
 AND jsonb_typeof(interaction_manifest_json->'workspaceOverlay')='object'
 AND jsonb_typeof(interaction_manifest_json->'session')='object'
 AND (interaction_manifest_json->'repositoryBundle') ?& ARRAY['sha256','sizeBytes']
 AND (interaction_manifest_json->'workspaceOverlay') ?& ARRAY['sha256','sizeBytes']
 AND (interaction_manifest_json->'session') ?& ARRAY['sha256','sizeBytes']
 AND jsonb_typeof(interaction_manifest_json->'repositoryBundle'->'sha256')='string'
 AND jsonb_typeof(interaction_manifest_json->'workspaceOverlay'->'sha256')='string'
 AND jsonb_typeof(interaction_manifest_json->'session'->'sha256')='string'
 AND jsonb_typeof(interaction_manifest_json->'repositoryBundle'->'sizeBytes')='number'
 AND jsonb_typeof(interaction_manifest_json->'workspaceOverlay'->'sizeBytes')='number'
 AND jsonb_typeof(interaction_manifest_json->'session'->'sizeBytes')='number'
 AND interaction_manifest_json->>'purpose'='INTERACTION_SUSPEND'
 AND interaction_manifest_json->>'decision'=''
 AND interaction_manifest_json->>'checkpointIdentity'=checkpoint_identity_sha256
 AND interaction_manifest_json->>'taskId'=task_id::text
 AND interaction_manifest_json->>'taskTurnId'=task_turn_id::text
 AND interaction_manifest_json->>'attemptId'=attempt_id::text
 AND interaction_manifest_json->>'interactionId'=COALESCE(interaction_id,0)::text
 AND interaction_manifest_json->>'operationId'=COALESCE(operation_id,0)::text
 AND interaction_manifest_json->>'adapterSessionGeneration'=adapter_session_generation::text
 AND interaction_manifest_json->>'automaticRevisionCount'=automatic_revision_count::text
 AND ((interaction_manifest_json->'repositoryBundle')-ARRAY['sha256','sizeBytes'])='{}'::jsonb
 AND ((interaction_manifest_json->'workspaceOverlay')-ARRAY['sha256','sizeBytes'])='{}'::jsonb
 AND ((interaction_manifest_json->'session')-ARRAY['sha256','sizeBytes'])='{}'::jsonb)
);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_checkpoint_interaction_once_idx ON workforce_continuation_checkpoints(interaction_id) WHERE interaction_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_checkpoint_operation_once_idx ON workforce_continuation_checkpoints(operation_id) WHERE operation_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_turn_source_checkpoint_once_idx ON workforce_task_turns(source_checkpoint_id) WHERE source_checkpoint_id IS NOT NULL;
ALTER TABLE workforce_task_turns ADD COLUMN IF NOT EXISTS source_interaction_response_id BIGINT, ADD COLUMN IF NOT EXISTS interaction_response_json JSONB, ADD COLUMN IF NOT EXISTS source_execution_operation_id BIGINT;
ALTER TABLE workforce_task_turns DROP CONSTRAINT IF EXISTS workforce_turn_interaction_response_ck;
ALTER TABLE workforce_task_turns ADD CONSTRAINT workforce_turn_interaction_response_ck CHECK (
 (source_interaction_response_id IS NULL AND interaction_response_json IS NULL) OR
 (source_interaction_response_id IS NOT NULL AND source_interaction_response_id>0 AND source_checkpoint_id IS NOT NULL
 AND interaction_response_json IS NOT NULL AND jsonb_typeof(interaction_response_json)='object' AND octet_length(interaction_response_json::text)<=16384
 AND interaction_response_json ?& ARRAY['interactionKey','responseId','answers']
 AND (interaction_response_json-ARRAY['interactionKey','responseId','answers'])='{}'::jsonb
 AND interaction_response_json->>'responseId'=source_interaction_response_id::text
 AND jsonb_typeof(interaction_response_json->'interactionKey')='string' AND jsonb_typeof(interaction_response_json->'answers')='array')
);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_turn_interaction_response_once_idx ON workforce_task_turns(source_interaction_response_id) WHERE source_interaction_response_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_turn_execution_operation_once_idx ON workforce_task_turns(source_execution_operation_id) WHERE source_execution_operation_id IS NOT NULL;
ALTER TABLE workforce_continuation_restore_runs ADD COLUMN IF NOT EXISTS interaction_execution_budget_json JSONB;
ALTER TABLE workforce_continuation_restore_runs DROP CONSTRAINT IF EXISTS workforce_restore_interaction_budget_ck;
ALTER TABLE workforce_continuation_restore_runs ADD CONSTRAINT workforce_restore_interaction_budget_ck CHECK (
 interaction_execution_budget_json IS NULL OR (
 jsonb_typeof(interaction_execution_budget_json)='object' AND octet_length(interaction_execution_budget_json::text)<=4096
 AND interaction_execution_budget_json ?& ARRAY['version','taskId','attemptId','generation','ownerEpoch','activeLimitMillis','activeElapsedMillis','interactionWaitingMillis','absoluteDeadlineUnixMillis']
 AND (interaction_execution_budget_json-ARRAY['version','taskId','attemptId','generation','ownerEpoch','activeLimitMillis','activeElapsedMillis','interactionWaitingMillis','absoluteDeadlineUnixMillis'])='{}'::jsonb
 AND interaction_execution_budget_json->>'version'='1'
 AND interaction_execution_budget_json->>'taskId'=task_id::text
 AND interaction_execution_budget_json->>'attemptId'=attempt_id::text));
`
