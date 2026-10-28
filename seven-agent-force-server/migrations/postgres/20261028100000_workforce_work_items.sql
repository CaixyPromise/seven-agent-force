-- +goose Up
-- Independent workflow state only. Existing Task/Review/Delivery remain authoritative.
ALTER TABLE workforce_tasks ADD COLUMN work_item_input_json JSONB;
ALTER TABLE workforce_tasks ADD CONSTRAINT workforce_tasks_work_item_input_ck CHECK (work_item_input_json IS NULL OR (jsonb_typeof(work_item_input_json)='object' AND octet_length(work_item_input_json::text)<=262144));
CREATE TABLE workforce_work_items (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, project_id BIGINT NOT NULL,
 project_version_id BIGINT NOT NULL, created_by_principal_id BIGINT NOT NULL,
 status VARCHAR(32) NOT NULL CHECK(status IN ('PLANNING','AWAITING_PLAN_APPROVAL','IMPLEMENTING','NEEDS_ATTENTION','SUCCEEDED','CANCELLING','CANCELLED')),
 version INTEGER NOT NULL CHECK(version>0), data_json JSONB NOT NULL, request_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX workforce_work_items_project_idx ON workforce_work_items(tenant_id,project_id,id DESC);
CREATE INDEX workforce_work_items_active_idx ON workforce_work_items(updated_at,id) WHERE status IN ('PLANNING','IMPLEMENTING','CANCELLING','AWAITING_PLAN_APPROVAL');
CREATE TABLE workforce_work_item_stages (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, work_item_id BIGINT NOT NULL,
 stage_key VARCHAR(16) NOT NULL CHECK(stage_key IN ('PLAN','IMPLEMENT')), position INTEGER NOT NULL,
 status VARCHAR(32) NOT NULL, current_run_id BIGINT, UNIQUE(work_item_id,stage_key)
);
CREATE TABLE workforce_work_item_stage_runs (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, work_item_id BIGINT NOT NULL,
 stage_key VARCHAR(16) NOT NULL CHECK(stage_key IN ('PLAN','IMPLEMENT')),
 run_number INTEGER NOT NULL CHECK(run_number>0),
 task_id BIGINT NOT NULL UNIQUE, status VARCHAR(32) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(work_item_id,stage_key,run_number)
);
CREATE TABLE workforce_work_item_plan_revisions (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, work_item_id BIGINT NOT NULL,
 revision INTEGER NOT NULL CHECK(revision>0), source_task_id BIGINT NOT NULL, source_output_id BIGINT NOT NULL,
 content_digest VARCHAR(64) NOT NULL CHECK(content_digest ~ '^[0-9a-f]{64}$'), data_json JSONB NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE(work_item_id,revision)
);
CREATE TABLE workforce_work_item_decisions (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, work_item_id BIGINT NOT NULL, actor_principal_id BIGINT NOT NULL,
 decision_kind VARCHAR(32) NOT NULL, plan_revision_id BIGINT, plan_content_digest VARCHAR(64),
 scope_key VARCHAR(32) NOT NULL, request_key VARCHAR(128) NOT NULL, request_digest VARCHAR(64) NOT NULL,
 response_json JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(tenant_id,actor_principal_id,scope_key,request_key)
);
CREATE UNIQUE INDEX workforce_work_item_one_plan_approval_idx ON workforce_work_item_decisions(work_item_id,plan_revision_id) WHERE decision_kind='APPROVE_PLAN';
CREATE TABLE workforce_work_item_events (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, work_item_id BIGINT NOT NULL,
 kind VARCHAR(64) NOT NULL, data_json JSONB NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX workforce_work_item_events_stream_idx ON workforce_work_item_events(tenant_id,work_item_id,id);
CREATE TABLE workforce_work_item_usage_snapshots (
 work_item_id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, usage_json JSONB NOT NULL,
 observed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
COMMENT ON TABLE workforce_work_items IS 'Coordinates a project work item and its frozen configuration.';
COMMENT ON TABLE workforce_work_item_stages IS 'Tracks the ordered planning and implementation stages of a work item.';
COMMENT ON TABLE workforce_work_item_stage_runs IS 'Links each work item stage run to its underlying task.';
COMMENT ON TABLE workforce_work_item_plan_revisions IS 'Stores immutable development plan revisions and their source identity.';
COMMENT ON TABLE workforce_work_item_decisions IS 'Records idempotent human decisions for a work item.';
COMMENT ON TABLE workforce_work_item_events IS 'Records work item lifecycle events.';
COMMENT ON TABLE workforce_work_item_usage_snapshots IS 'Stores observed usage aggregated for a work item.';
-- +goose Down
-- Forward-only: preserve decisions and referenced execution history.
SELECT 1;
