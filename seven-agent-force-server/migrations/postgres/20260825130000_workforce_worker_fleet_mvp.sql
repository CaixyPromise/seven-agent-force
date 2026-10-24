-- +goose Up

-- P0.7 is still an unreleased development schema. Keep the final Workforce
-- operator permissions in this migration instead of adding a compatibility
-- migration. Application-level authorization continues to own the relation;
-- no database foreign keys are introduced.
-- +goose StatementBegin
DO $p07_operator_permissions$
BEGIN
  IF to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.ordinal), candidate.code, candidate.name,
           'API', candidate.method, candidate.path, 0, candidate.description,
           CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false
      FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082513000000000) AS id FROM sys_permission) base
      CROSS JOIN (VALUES
        (1, 'workforce:finalization:retry', '重试可信收尾', 'POST', '/workforce/finalization-runs/{id}/retry', '仅重试同一Agent执行结果的可信收尾'),
        (2, 'workforce:artifact-upload:retry', '重试产物上传', 'POST', '/workforce/artifact-upload-attempts/{id}/retry', '仅重试冻结制品的上传与回读校验'),
        (3, 'workforce:result-processor:list', '查看结果处理器列表', 'GET', '/workforce/result-processors', '查看当前租户的结果处理器'),
        (4, 'workforce:result-processor:query', '查看结果处理器详情', 'GET', '/workforce/result-processors/{id}', '查看草稿、发布版本与测试证据'),
        (5, 'workforce:result-processor:author', '编辑结果处理器', 'POST', '/workforce/result-processors', '创建、编辑、导入、校验和提交Fixture'),
        (6, 'workforce:result-processor:publish', '发布结果处理器', 'POST', '/workforce/result-processor-revisions/{id}/publish', '发布不可变结果处理器Release'),
        (7, 'workforce:result-processor:bind', '绑定结果处理器', 'PUT', '/workforce/result-processor-bindings/default', '设置租户默认结果处理器'),
        (8, 'workforce:result-processor:secret:bind', '绑定结果处理器秘密', 'PUT', '/workforce/result-processor-bindings/{id}/environment', '绑定EVALUATE阶段秘密引用'),
        (9, 'workforce:result-processor:network:bind', '绑定结果处理器网络', 'PUT', '/workforce/result-processor-bindings/{id}/environment', '配置结果处理器网络策略'),
        (10, 'workforce:result-processor:audit', '查看结果处理器审计', 'GET', '/workforce/tasks/{id}/evaluations', '查看评估、检查点与安全审计'),
		(11, 'workforce:task:continue', '继续任务下一轮', 'POST', '/workforce/tasks/{id}/turns', '从可信Checkpoint创建同一任务的下一轮'),
		(12, 'workforce:task:continuation:approve', '审批人工复核接续', 'POST', '/workforce/continuation-checkpoints/{id}/approvals', '审批MANUAL_REVIEW结果的后续接续'),
		(13, 'workforce:result-output:materialize', '生成按需输出', 'POST', '/workforce/tasks/{id}/result-materializations', '从可信Checkpoint生成声明输出或Patch/Bundle'),
		(14, 'workforce:result-output:retry', '重试按需输出', 'POST', '/workforce/result-materialization-runs/{id}/retry', '仅重试按需物化，不重新运行Agent'),
		(20, 'workforce:source:create', '准备任务代码', 'POST', '/workforce/workspace-sources/uploads', '从Git、目录或压缩包生成冻结代码版本'),
		(21, 'workforce:source:query', '查看任务代码准备状态', 'GET', '/workforce/workspace-source-revisions/{id}', '查看代码导入和固定版本证据'),
		(22, 'workforce:reference:create', '添加任务参考资料', 'POST', '/workforce/reference-assets', '将PDF、图片和需求文档作为只读任务资料'),
		(23, 'workforce:delivery:approve', '决定是否交付', 'POST', '/workforce/tasks/{taskId}/delivery-requests/{requestId}/decisions', '决定是否交付冻结结果'),
		(24, 'workforce:delivery:cancel', '取消交付', 'POST', '/workforce/delivery-requests/{id}/cancel', '取消交付并执行远端回读仲裁'),
		(25, 'workforce:delivery:retry', '重试交付', 'POST', '/workforce/delivery-requests/{id}/retry', '仅重试交付后处理'),
		(31, 'workforce:task:metadata:update', '编辑任务信息', 'PATCH', '/workforce/tasks/{id}/metadata', '修改任务名称和标签，不改变执行身份'),
		(32, 'workforce:task-notification:operate', '管理任务通知', 'PUT', '/workforce/tasks/{id}/notification-subscription', '管理本租户任务创建人的通知订阅'),
		(33, 'workforce:delivery:reconcile', '核对交付状态', 'POST', '/workforce/delivery-requests/{id}/reconcile', '只读回读远端证据并仲裁不确定交付'),
		(43, 'workforce:review:approve', '处置独立审查结果', 'POST', '/workforce/review-runs/{id}/decisions', '经二次验证接受风险、授权一次修订或取消任务')
      ) AS candidate(ordinal, code, name, method, path, description)
     WHERE NOT EXISTS (
       SELECT 1 FROM sys_permission existing
        WHERE existing.code = candidate.code AND existing."isDeleted" = false
     );
  END IF;

  IF to_regclass('public.sys_role_permission') IS NOT NULL THEN
    INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
    SELECT base.id + ROW_NUMBER() OVER (ORDER BY candidate.role_id, candidate.permission_id),
           candidate.role_id, candidate.permission_id, NULL, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id), 0), 2026082513000000100) AS id FROM sys_role_permission) base
      CROSS JOIN (
        SELECT role.id AS role_id, permission.id AS permission_id
          FROM sys_role role
          JOIN sys_permission permission
            ON permission.code IN ('workforce:finalization:retry', 'workforce:artifact-upload:retry',
                'workforce:result-processor:list', 'workforce:result-processor:query',
                'workforce:result-processor:author', 'workforce:result-processor:publish',
                'workforce:result-processor:bind', 'workforce:result-processor:secret:bind',
                'workforce:result-processor:network:bind', 'workforce:result-processor:audit',
				'workforce:task:continue','workforce:task:continuation:approve',
				'workforce:result-output:materialize','workforce:result-output:retry',
				'workforce:source:create',
				'workforce:source:query','workforce:reference:create','workforce:delivery:approve',
				'workforce:delivery:cancel','workforce:delivery:retry',
				'workforce:task:metadata:update',
				'workforce:task-notification:operate','workforce:delivery:reconcile',
				'workforce:review:approve')
           AND permission."isDeleted" = false
         WHERE role.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN')
           AND role."isDeleted" = false
           AND NOT EXISTS (
             SELECT 1 FROM sys_role_permission existing
              WHERE existing."roleId" = role.id AND existing."permissionId" = permission.id
           )
      ) candidate;
  END IF;
END
$p07_operator_permissions$;
-- +goose StatementEnd

-- Result Processor is an operator-facing Workforce capability.  Keep its
-- menu and permission attachment together with the unpublished Workforce
-- schema so a fresh Framework baseline gets a complete, navigable console.
-- The database is not used to enforce business relationships: this only
-- seeds Framework navigation/RBAC metadata when the Framework tables exist.
-- +goose StatementBegin
DO $p09_result_processor_menu$
BEGIN
  IF to_regclass('public.sys_menu') IS NOT NULL THEN
    INSERT INTO sys_menu (id,name,"parentId","sortOrder",path,component,icon,type,permission,
                          "isFrame","isCache",visible,hierarchy,level,status,
                          "creatorId","createTime","updaterId","updateTime","isDeleted",remark)
    SELECT base.id+candidate.ordinal,candidate.name,parent.id,candidate.sort_order,candidate.path,
           candidate.component,candidate.icon,'C',candidate.permission,true,true,true,
           CONCAT(COALESCE(NULLIF(parent.hierarchy,''),'/1'),'/',base.id+candidate.ordinal),
           parent.level+1,0,0,CURRENT_TIMESTAMP,0,CURRENT_TIMESTAMP,false,candidate.remark
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513000001000) id FROM sys_menu) base
      JOIN sys_menu parent ON parent.path='/system/workforce' AND parent."isDeleted"=false
      CROSS JOIN (VALUES
		(1,50,'结果处理','/workforce/result-processors','workforce/result-processors/index','FileSearchOutlined','workforce:result-processor:list','维护高级结果验收规则、Fixture 与环境绑定')
      ) candidate(ordinal,sort_order,name,path,component,icon,permission,remark)
     WHERE NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path=candidate.path AND existing."isDeleted"=false);
  END IF;

  IF to_regclass('public.sys_menu_permission') IS NOT NULL
     AND to_regclass('public.sys_permission') IS NOT NULL THEN
    INSERT INTO sys_menu_permission (id,"menuId","permissionId","creatorId","createTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY menu.id,permission.id),menu.id,permission.id,0,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513000001100) id FROM sys_menu_permission) base
	  JOIN sys_menu menu ON menu.path='/workforce/result-processors'
                        AND menu."isDeleted"=false
      CROSS JOIN sys_permission permission
	 WHERE (permission.code LIKE 'workforce:result-processor:%' OR permission.code LIKE 'workforce:delivery:%' OR permission.code LIKE 'workforce:source:%' OR permission.code LIKE 'workforce:reference:%') AND permission."isDeleted"=false
       AND NOT EXISTS (
         SELECT 1 FROM sys_menu_permission existing
          WHERE existing."menuId"=menu.id AND existing."permissionId"=permission.id
       );
  END IF;

  IF to_regclass('public.sys_role_menu') IS NOT NULL
     AND to_regclass('public.sys_role') IS NOT NULL
     AND to_regclass('public.sys_menu') IS NOT NULL THEN
    INSERT INTO sys_role_menu (id,"roleId","menuId","createTime","updateTime")
    SELECT base.id+ROW_NUMBER() OVER (ORDER BY role.id,menu.id),role.id,menu.id,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
      FROM (SELECT GREATEST(COALESCE(MAX(id),0),2026082513000001200) id FROM sys_role_menu) base
      CROSS JOIN sys_role role
	  JOIN sys_menu menu ON menu.path='/workforce/result-processors'
                        AND menu."isDeleted"=false
     WHERE role.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND role."isDeleted"=false
       AND NOT EXISTS (
         SELECT 1 FROM sys_role_menu existing
          WHERE existing."roleId"=role.id AND existing."menuId"=menu.id
       );
  END IF;
END
$p09_result_processor_menu$;
-- +goose StatementEnd


-- +goose StatementBegin
CREATE OR REPLACE FUNCTION workforce_assert_project_hard_cut_ready()
RETURNS VOID
LANGUAGE plpgsql
SET search_path = pg_catalog, public
AS $project_hard_cut$
DECLARE
  relation_name TEXT;
  has_business_rows BOOLEAN;
BEGIN
  FOREACH relation_name IN ARRAY ARRAY[
    'workforce_repository_connections',
    'workforce_repository_access_bindings',
    'workforce_repository_credential_events',
    'workforce_repository_lifecycle_events',
    'workforce_hook_bundles',
    'workforce_hook_bundle_releases',
    'workforce_hook_bindings',
    'workforce_hook_execution_profiles',
    'workforce_golden_verification_runs',
    'workforce_integrated_verification_runs'
  ] LOOP
    IF pg_catalog.to_regclass(pg_catalog.format('public.%I',relation_name)) IS NULL THEN
      CONTINUE;
    END IF;
    EXECUTE pg_catalog.format(
      'SELECT EXISTS (SELECT 1 FROM public.%I LIMIT 1)',
      relation_name
    ) INTO has_business_rows;
    IF has_business_rows THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'PROJECT_REPOSITORY_HARD_CUT_RESET_REQUIRED';
    END IF;
  END LOOP;

  FOREACH relation_name IN ARRAY ARRAY[
    'workforce_tasks',
    'workforce_task_attempts',
    'workforce_harness_sessions',
    'workforce_completion_proposals',
    'workforce_task_events',
    'workforce_artifacts',
    'workforce_check_runs',
    'workforce_model_invocations',
    'workforce_task_execution_snapshots',
    'workforce_review_runs',
    'workforce_delivery_requests'
  ] LOOP
    IF pg_catalog.to_regclass(pg_catalog.format('public.%I',relation_name)) IS NULL THEN
      CONTINUE;
    END IF;
    EXECUTE pg_catalog.format(
      'SELECT EXISTS (SELECT 1 FROM public.%I LIMIT 1)',
      relation_name
    ) INTO has_business_rows;
    IF has_business_rows THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'PROJECT_HARD_CUT_RESET_REQUIRED';
    END IF;
  END LOOP;
END
$project_hard_cut$;

SELECT workforce_assert_project_hard_cut_ready();
-- +goose StatementEnd

DROP TABLE IF EXISTS workforce_repository_credential_events;
DROP TABLE IF EXISTS workforce_repository_lifecycle_events;
DROP TABLE IF EXISTS workforce_repository_access_bindings;
DROP TABLE IF EXISTS workforce_repository_connections;
DROP TABLE IF EXISTS workforce_integrated_verification_runs;
DROP TABLE IF EXISTS workforce_golden_verification_runs;
DROP TABLE IF EXISTS workforce_hook_bindings;
DROP TABLE IF EXISTS workforce_hook_execution_profiles;
DROP TABLE IF EXISTS workforce_hook_bundle_releases;
DROP TABLE IF EXISTS workforce_hook_bundles;

CREATE TABLE workforce_workers (
    id BIGINT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    desired_state VARCHAR(32) NOT NULL DEFAULT 'ENABLED',
    max_active_attempts INTEGER NOT NULL DEFAULT 1,
    max_active_reviews INTEGER NOT NULL DEFAULT 1 CHECK (max_active_reviews > 0 AND max_active_reviews <= 64),
    max_total_containers INTEGER NOT NULL DEFAULT 2 CHECK (max_total_containers > 0 AND max_total_containers <= 128),
    current_instance_id TEXT,
    token_hint TEXT NOT NULL,
    inventory_refresh_generation INTEGER NOT NULL DEFAULT 1,
    decommissioned_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1,
    created_by_principal_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_worker_tokens (
    id BIGINT PRIMARY KEY,
    worker_id BIGINT NOT NULL,
    token_sha256 VARCHAR(64) NOT NULL UNIQUE,
    token_hint TEXT NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at TIMESTAMPTZ
);

CREATE TABLE workforce_worker_sessions (
    instance_id TEXT PRIMARY KEY,
    worker_id BIGINT NOT NULL,
    host_boot_id TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at TIMESTAMPTZ,
    end_reason VARCHAR(128),
    last_heartbeat_at TIMESTAMPTZ,
    heartbeat_sequence INTEGER NOT NULL DEFAULT 0,
    consumer_state VARCHAR(32) NOT NULL DEFAULT 'STARTING',
    docker_ready BOOLEAN NOT NULL DEFAULT FALSE,
    active_attempts INTEGER NOT NULL DEFAULT 0,
    max_active_attempts INTEGER NOT NULL DEFAULT 1,
    worker_version TEXT NOT NULL DEFAULT '',
    worker_revision TEXT NOT NULL DEFAULT '',
    worker_build_date TEXT NOT NULL DEFAULT '',
    observed_remote_ip TEXT NOT NULL DEFAULT '',
    reported_ip TEXT NOT NULL DEFAULT '',
    hostname TEXT NOT NULL DEFAULT '',
    primary_interface TEXT NOT NULL DEFAULT '',
    primary_mac TEXT NOT NULL DEFAULT '',
    interfaces_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    resource_snapshot_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb
    ,session_state VARCHAR(32) NOT NULL DEFAULT 'REGISTERED'
    ,reconciliation_deadline TIMESTAMPTZ
);

CREATE TABLE workforce_worker_metric_samples (
    worker_id BIGINT NOT NULL,
    sampled_at TIMESTAMPTZ NOT NULL,
    logical_cpu_cores SMALLINT NOT NULL CHECK (logical_cpu_cores > 0),
    cpu_usage_percent REAL CHECK (cpu_usage_percent BETWEEN 0 AND 100),
    load1 REAL CHECK (load1 >= 0),
    memory_used_bytes BIGINT NOT NULL CHECK (memory_used_bytes >= 0),
    memory_total_bytes BIGINT NOT NULL CHECK (memory_total_bytes >= memory_used_bytes),
    memory_usage_percent REAL NOT NULL CHECK (memory_usage_percent BETWEEN 0 AND 100),
    workspace_disk_used_bytes BIGINT NOT NULL CHECK (workspace_disk_used_bytes >= 0),
    workspace_disk_total_bytes BIGINT NOT NULL CHECK (workspace_disk_total_bytes >= workspace_disk_used_bytes),
    workspace_disk_usage_percent REAL NOT NULL CHECK (workspace_disk_usage_percent BETWEEN 0 AND 100),
    active_attempts SMALLINT NOT NULL CHECK (active_attempts >= 0),
    max_active_attempts SMALLINT NOT NULL CHECK (max_active_attempts > 0 AND active_attempts <= max_active_attempts),
    docker_ready BOOLEAN NOT NULL,
    PRIMARY KEY (worker_id, sampled_at)
);

CREATE INDEX workforce_worker_metric_samples_sampled_at_brin
    ON workforce_worker_metric_samples USING BRIN (sampled_at);

CREATE TABLE workforce_worker_images (
    worker_id BIGINT NOT NULL,
    instance_id TEXT NOT NULL,
    image_digest TEXT NOT NULL,
    image_ref TEXT NOT NULL DEFAULT '',
    role VARCHAR(32) NOT NULL,
    contract_version TEXT NOT NULL,
    manifest_digest VARCHAR(64) NOT NULL,
    manifest_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    scheduling_scope VARCHAR(32) NOT NULL DEFAULT 'PRODUCTION'
        CONSTRAINT workforce_worker_images_scheduling_scope_ck
        CHECK (scheduling_scope IN ('PRODUCTION','TEST_ONLY')),
    scheduling_priority INTEGER NOT NULL DEFAULT 100
        CONSTRAINT workforce_worker_images_scheduling_priority_ck
        CHECK (scheduling_priority BETWEEN -100000 AND 100000),
    environment_capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb
        CONSTRAINT workforce_worker_images_environment_capabilities_ck
        CHECK (jsonb_typeof(environment_capabilities_json)='array'),
    execution_kinds_json JSONB NOT NULL DEFAULT '["AGENT_ATTEMPT","REVIEW_RUN"]'::jsonb
        CONSTRAINT workforce_worker_images_execution_kinds_ck
        CHECK (
            jsonb_typeof(execution_kinds_json)='array'
            AND ((role='attempt' AND jsonb_array_length(execution_kinds_json)>0) OR (role<>'attempt' AND jsonb_array_length(execution_kinds_json)=0))
            AND execution_kinds_json <@ '["AGENT_ATTEMPT","REVIEW_RUN"]'::jsonb
        ),
    scheduling_digest VARCHAR(64) NOT NULL
        CONSTRAINT workforce_worker_images_scheduling_digest_ck
        CHECK (scheduling_digest ~ '^[0-9a-f]{64}$'),
    admission_status VARCHAR(32) NOT NULL,
    admission_evidence_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    presence_status VARCHAR(32) NOT NULL DEFAULT 'PRESENT',
    missing_since TIMESTAMPTZ,
    drift_reason TEXT NOT NULL DEFAULT '',
    inventory_version INTEGER NOT NULL,
    inventory_digest TEXT NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (worker_id, image_digest, role)
);

CREATE TABLE workforce_execution_nodes (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
    node_key TEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    generation INTEGER NOT NULL DEFAULT 1,
    execution_requirement_digest TEXT NOT NULL,
    wait_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    max_attempts INTEGER NOT NULL DEFAULT 3,
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    last_retry_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (task_turn_id, node_key)
);

CREATE TABLE workforce_task_assignments (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    execution_node_id BIGINT NOT NULL,
    worker_id BIGINT NOT NULL,
    worker_instance_id TEXT NOT NULL,
    generation INTEGER NOT NULL,
    execution_requirement_digest TEXT NOT NULL,
    agent_runtime_release_id BIGINT NOT NULL CHECK (agent_runtime_release_id > 0),
    runtime_image_ref TEXT NOT NULL CHECK (btrim(runtime_image_ref)<>''),
    runtime_image_digest TEXT NOT NULL CHECK (runtime_image_digest ~ '^sha256:[0-9a-f]{64}$'),
    adapter_identity TEXT NOT NULL CHECK (btrim(adapter_identity)<>''),
    adapter_version TEXT NOT NULL CHECK (btrim(adapter_version)<>''),
    adapter_protocol VARCHAR(64) NOT NULL CHECK (btrim(adapter_protocol)<>''),
    adapter_capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(adapter_capabilities_json)='array'),
    hook_configuration_digest VARCHAR(64) NOT NULL DEFAULT ''
        CHECK (hook_configuration_digest = '' OR hook_configuration_digest ~ '^[0-9a-f]{64}$'),
    model_credential_id BIGINT NOT NULL CHECK (model_credential_id > 0),
    credential_version INTEGER NOT NULL CHECK (credential_version > 0),
    model_protocol VARCHAR(16) NOT NULL CHECK (model_protocol IN ('OPENAI','ANTHROPIC')),
    primary_model VARCHAR(256) NOT NULL CHECK (btrim(primary_model)<>''),
    provider_proxy_identity TEXT NOT NULL CHECK (btrim(provider_proxy_identity)<>''),
    worker_image_contract TEXT NOT NULL CHECK (btrim(worker_image_contract)<>''),
    isolation_profile VARCHAR(64) NOT NULL CHECK (btrim(isolation_profile)<>''),
    resolved_execution_snapshot_json JSONB NOT NULL
        CHECK (jsonb_typeof(resolved_execution_snapshot_json)='object'),
    selection_digest VARCHAR(64) NOT NULL CHECK (selection_digest ~ '^[0-9a-f]{64}$'),
    lease_token_sha256 VARCHAR(64) NOT NULL,
    lease_expires_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(32) NOT NULL,
    message_id BIGINT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMPTZ,
    claimed_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ
    ,released_at TIMESTAMPTZ
    ,release_reason_code VARCHAR(128) NOT NULL DEFAULT ''
    ,delivery_disposition VARCHAR(32) NOT NULL DEFAULT 'UNSETTLED'
    ,delivery_acked_at TIMESTAMPTZ
    ,dead_lettered_at TIMESTAMPTZ
);

CREATE TABLE workforce_attempt_runtimes (
    attempt_id BIGINT PRIMARY KEY,
    execution_node_id BIGINT NOT NULL,
    assignment_id BIGINT NOT NULL UNIQUE,
    worker_id BIGINT NOT NULL,
    worker_instance_id TEXT NOT NULL,
    host_boot_id TEXT NOT NULL,
    generation INTEGER NOT NULL,
    owner_epoch INTEGER NOT NULL DEFAULT 1,
    execution_identity_digest VARCHAR(64) NOT NULL,
    expected_container_name TEXT NOT NULL,
    container_id TEXT,
	provider_proxy_container_id TEXT,
	provider_proxy_container_name TEXT,
	provider_proxy_image_digest TEXT NOT NULL DEFAULT '',
	provider_proxy_execution_identity_digest VARCHAR(64) NOT NULL DEFAULT '',
	provider_proxy_owner_epoch INTEGER,
	provider_proxy_control_state VARCHAR(32) NOT NULL DEFAULT '',
	provider_proxy_ready_at TIMESTAMPTZ,
    harness_image_digest TEXT NOT NULL,
    hook_configuration_digest TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL,
    launch_permit_id BIGINT,
    launch_permit_sha256 VARCHAR(64),
    launch_permit_expires_at TIMESTAMPTZ,
    container_created_at TIMESTAMPTZ,
    supervisor_ready_at TIMESTAMPTZ,
    harness_started_at TIMESTAMPTZ,
    harness_pid INTEGER,
    run_permit_id BIGINT,
    run_permit_sha256 VARCHAR(64),
    run_permit_expires_at TIMESTAMPTZ,
    run_permit_issued_at TIMESTAMPTZ,
    run_nonce_sha256 VARCHAR(64),
    run_nonce_accepted_at TIMESTAMPTZ,
    execution_lease_sha256 VARCHAR(64),
    execution_lease_expires_at TIMESTAMPTZ,
    execution_lease_renewed_at TIMESTAMPTZ,
    startup_deadline TIMESTAMPTZ,
    supervisor_protocol_version INTEGER NOT NULL DEFAULT 2,
    recovery_started_at TIMESTAMPTZ,
    recovery_previous_status VARCHAR(32),
    reattach_deadline TIMESTAMPTZ,
    reattach_permit_id BIGINT,
    reattach_permit_sha256 VARCHAR(64),
    reattach_permit_expires_at TIMESTAMPTZ,
    pending_worker_instance_id TEXT,
    pending_owner_epoch INTEGER,
    reattached_at TIMESTAMPTZ,
    authority_receipt_sha256 VARCHAR(64),
    watchdog_deadline TIMESTAMPTZ,
    terminating_at TIMESTAMPTZ,
    lost_at TIMESTAMPTZ,
    fenced_at TIMESTAMPTZ,
    terminal_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_runtime_reconciliation_runs (
    id BIGINT PRIMARY KEY,
    attempt_id BIGINT NOT NULL,
    execution_node_id BIGINT NOT NULL,
    worker_id BIGINT NOT NULL,
    old_instance_id TEXT NOT NULL,
    new_instance_id TEXT NOT NULL,
    host_boot_id TEXT NOT NULL,
    old_owner_epoch INTEGER NOT NULL,
    new_owner_epoch INTEGER NOT NULL,
    status VARCHAR(32) NOT NULL,
    runtime_evidence_sha256 VARCHAR(64) NOT NULL,
    supervisor_ack_sha256 VARCHAR(64),
	provider_proxy_ack_sha256 VARCHAR(64),
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    deadline TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ
);

CREATE TABLE workforce_finalization_runs (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
    execution_node_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    generation INTEGER NOT NULL,
    input_identity_digest VARCHAR(64) NOT NULL,
    completion_proposal_id BIGINT NOT NULL,
    result_commit TEXT NOT NULL DEFAULT '',
    result_tree TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL,
    run_number INTEGER NOT NULL DEFAULT 1,
    max_runs INTEGER NOT NULL DEFAULT 3,
    worker_id BIGINT NOT NULL,
    worker_instance_id TEXT NOT NULL,
    host_boot_id TEXT NOT NULL,
    lease_sha256 VARCHAR(64),
    lease_expires_at TIMESTAMPTZ,
    manifest_digest VARCHAR(64),
    manifest_json JSONB,
    spool_identity TEXT NOT NULL DEFAULT '',
    next_retry_at TIMESTAMPTZ,
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMPTZ,
    frozen_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_artifact_upload_attempts (
    id BIGINT PRIMARY KEY,
    finalization_run_id BIGINT,
    materialization_run_id BIGINT,
    task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    artifact_type VARCHAR(64) NOT NULL,
    artifact_name TEXT NOT NULL,
    object_key TEXT NOT NULL,
    sha256 VARCHAR(64) NOT NULL,
    size_bytes BIGINT NOT NULL,
    media_type VARCHAR(255) NOT NULL,
    attempt_number INTEGER NOT NULL DEFAULT 1,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    worker_id BIGINT NOT NULL,
    host_boot_id TEXT NOT NULL,
    status VARCHAR(32) NOT NULL,
    lease_sha256 VARCHAR(64),
    lease_expires_at TIMESTAMPTZ,
    next_retry_at TIMESTAMPTZ,
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMPTZ,
    uploaded_at TIMESTAMPTZ,
    verified_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_machine_idempotency_records (
    worker_id BIGINT NOT NULL,
    http_method VARCHAR(16) NOT NULL,
    normalized_route TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_digest VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    response_status INTEGER,
    safe_response_json JSONB,
    encrypted_response_blob BYTEA,
    resource_type VARCHAR(64) NOT NULL DEFAULT '',
    resource_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (worker_id, http_method, normalized_route, idempotency_key)
);

CREATE TABLE workforce_attempt_facts (
    attempt_id BIGINT NOT NULL,
    fact_id TEXT NOT NULL,
    producer_source TEXT NOT NULL DEFAULT 'WORKER' CHECK (producer_source IN ('WORKER','SUPERVISOR_JOURNAL')),
    producer_sequence INTEGER NOT NULL,
    event_type VARCHAR(128) NOT NULL,
    payload_sha256 VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (attempt_id, fact_id)
);
CREATE UNIQUE INDEX workforce_attempt_facts_source_sequence_uidx ON workforce_attempt_facts(attempt_id,producer_source,producer_sequence);

ALTER TABLE workforce_task_attempts ADD COLUMN assignment_id BIGINT;
ALTER TABLE workforce_task_attempts ADD COLUMN execution_node_id BIGINT;
ALTER TABLE workforce_task_attempts
ADD COLUMN worker_instance_id TEXT;
ALTER TABLE workforce_task_attempts
ADD COLUMN host_boot_id TEXT;
ALTER TABLE workforce_task_attempts ADD COLUMN owner_epoch INTEGER NOT NULL DEFAULT 1;
ALTER TABLE workforce_task_attempts ADD COLUMN retry_class VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE workforce_task_attempts ADD COLUMN retry_of_attempt_id BIGINT;
ALTER TABLE workforce_task_attempts ADD COLUMN execution_completed_at TIMESTAMPTZ;
ALTER TABLE workforce_task_attempts ADD COLUMN lost_at TIMESTAMPTZ;
ALTER TABLE workforce_task_attempts ADD COLUMN timeout_stage VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE workforce_task_attempts ADD COLUMN timed_out_at TIMESTAMPTZ;
ALTER TABLE workforce_task_attempts
    ADD COLUMN assignment_selection_digest VARCHAR(64) NOT NULL
        CONSTRAINT workforce_task_attempts_assignment_selection_digest_ck
        CHECK (assignment_selection_digest ~ '^[0-9a-f]{64}$'),
    ADD COLUMN resolved_execution_snapshot_json JSONB NOT NULL
        CONSTRAINT workforce_task_attempts_resolved_execution_snapshot_ck
        CHECK (jsonb_typeof(resolved_execution_snapshot_json)='object');

ALTER TABLE workforce_tasks ADD COLUMN cancel_request_id BIGINT;
ALTER TABLE workforce_tasks ADD COLUMN cancel_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE workforce_tasks ADD COLUMN cancel_requested_by_principal_id BIGINT;
ALTER TABLE workforce_tasks ADD COLUMN cancel_requested_at TIMESTAMPTZ;
ALTER TABLE workforce_tasks ADD COLUMN cancel_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE workforce_tasks ADD COLUMN quality_outcome TEXT NOT NULL DEFAULT '' CONSTRAINT workforce_tasks_quality_outcome_ck CHECK (quality_outcome IN ('','INDEPENDENT_REVIEW_PASSED','HUMAN_ACCEPTED'));

ALTER TABLE workforce_outbox_events ADD COLUMN exchange_name TEXT;
ALTER TABLE workforce_outbox_events ADD COLUMN routing_key TEXT;
ALTER TABLE workforce_outbox_events ADD COLUMN message_expiration_ms BIGINT;

UPDATE workforce_outbox_events
   SET status='SUPERSEDED', last_error='superseded by P0.7 targeted Assignment dispatch'
 WHERE event_type='task.queued' AND status='PENDING';

CREATE UNIQUE INDEX workforce_active_assignment_node_idx
    ON workforce_task_assignments (execution_node_id)
    WHERE status IN ('RESERVED','PUBLISHED','CLAIMED');
CREATE UNIQUE INDEX workforce_active_runtime_node_idx
    ON workforce_attempt_runtimes (execution_node_id)
    WHERE status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','TERMINATING','RECONCILIATION_REQUIRED');
CREATE UNIQUE INDEX workforce_active_reconciliation_attempt_idx
    ON workforce_runtime_reconciliation_runs (attempt_id)
    WHERE status IN ('PENDING','ADOPTING');
CREATE UNIQUE INDEX workforce_active_finalization_attempt_idx
    ON workforce_finalization_runs (attempt_id)
    WHERE status IN ('PENDING','RUNNING','RETRY_WAIT','FROZEN');
CREATE UNIQUE INDEX workforce_finalization_identity_idx
    ON workforce_finalization_runs (attempt_id, input_identity_digest);
CREATE UNIQUE INDEX workforce_verified_upload_identity_idx
    ON workforce_artifact_upload_attempts (finalization_run_id, artifact_type, sha256)
    WHERE status='VERIFIED';
CREATE UNIQUE INDEX workforce_verified_materialization_upload_idx
    ON workforce_artifact_upload_attempts (materialization_run_id, sha256)
    WHERE materialization_run_id IS NOT NULL AND status='VERIFIED';
CREATE INDEX workforce_finalization_retry_idx
    ON workforce_finalization_runs (status, next_retry_at, worker_id);
CREATE INDEX workforce_upload_retry_idx
    ON workforce_artifact_upload_attempts (status, next_retry_at, worker_id);
CREATE INDEX workforce_reconciliation_deadline_idx
    ON workforce_attempt_runtimes (status, reattach_deadline);
CREATE INDEX workforce_machine_idempotency_expiry_idx
    ON workforce_machine_idempotency_records (expires_at);
CREATE INDEX workforce_worker_sessions_fresh_idx ON workforce_worker_sessions (worker_id, last_heartbeat_at DESC);
CREATE INDEX workforce_worker_images_match_idx ON workforce_worker_images (image_digest, role, admission_status, worker_id);
CREATE INDEX workforce_worker_images_routing_idx
    ON workforce_worker_images (scheduling_scope,role,admission_status,presence_status,scheduling_priority,worker_id);
CREATE INDEX workforce_worker_images_environment_capabilities_gin_idx
    ON workforce_worker_images USING GIN (environment_capabilities_json);
CREATE INDEX workforce_execution_nodes_dispatch_idx ON workforce_execution_nodes (status, created_at);
CREATE INDEX workforce_assignments_worker_status_idx ON workforce_task_assignments (worker_id, status, created_at);
CREATE INDEX workforce_assignments_capacity_idx ON workforce_task_assignments (worker_id, status, claimed_at);
CREATE INDEX workforce_assignments_selection_idx ON workforce_task_assignments (selection_digest,execution_node_id,generation);
CREATE INDEX workforce_runtime_capacity_idx ON workforce_attempt_runtimes (worker_id, status, updated_at);
CREATE INDEX workforce_images_presence_idx ON workforce_worker_images (worker_id, presence_status, admission_status, image_digest);
CREATE INDEX workforce_tasks_fair_dispatch_idx ON workforce_tasks (tenant_id, status, priority DESC, created_at);
CREATE UNIQUE INDEX workforce_completion_attempt_once_idx ON workforce_completion_proposals (attempt_id);

-- P0.9 Result Processing Closure. Processors and immutable Releases remain;
-- the unpublished Template product layer was removed by the Project-first
-- hard cut. Projects bind directly to a released Processor.
CREATE TABLE workforce_result_processors (
    id BIGINT PRIMARY KEY,
    scope_type VARCHAR(32) NOT NULL CHECK (scope_type IN ('SYSTEM','TENANT')),
    tenant_id BIGINT,
    name TEXT NOT NULL,
    category VARCHAR(64) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id IS NOT NULL))
);
CREATE UNIQUE INDEX workforce_result_processors_system_name_uidx ON workforce_result_processors (name) WHERE scope_type='SYSTEM';
CREATE UNIQUE INDEX workforce_result_processors_tenant_name_uidx ON workforce_result_processors (tenant_id,name) WHERE scope_type='TENANT';

CREATE TABLE workforce_result_processor_revisions (
    id BIGINT PRIMARY KEY,
    processor_id BIGINT NOT NULL,
    scope_type VARCHAR(32) NOT NULL CHECK (scope_type IN ('SYSTEM','TENANT')),
    tenant_id BIGINT,
    revision_number INTEGER NOT NULL,
	    draft_version INTEGER NOT NULL DEFAULT 1,
    status VARCHAR(32) NOT NULL DEFAULT 'DRAFT',
    manifest_json JSONB NOT NULL,
    files_json JSONB NOT NULL,
    validation_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    validation_status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    created_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id IS NOT NULL)),
    UNIQUE (processor_id, revision_number)
);

CREATE TABLE workforce_result_processor_releases (
    id BIGINT PRIMARY KEY,
    processor_id BIGINT NOT NULL,
    scope_type VARCHAR(32) NOT NULL CHECK (scope_type IN ('SYSTEM','TENANT')),
    tenant_id BIGINT,
    revision_id BIGINT NOT NULL,
    version TEXT NOT NULL,
    manifest_json JSONB NOT NULL,
    files_json JSONB NOT NULL,
    archive_object_key TEXT NOT NULL,
    archive_sha256 VARCHAR(64) NOT NULL,
    archive_size_bytes BIGINT NOT NULL,
    published_by_principal_id BIGINT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id IS NOT NULL))
);
CREATE UNIQUE INDEX workforce_result_processor_releases_version_uidx ON workforce_result_processor_releases (processor_id,version);
CREATE UNIQUE INDEX workforce_result_processor_releases_digest_uidx ON workforce_result_processor_releases (processor_id,archive_sha256);

CREATE TABLE workforce_result_processor_bindings (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    binding_name TEXT NOT NULL,
    processor_release_id BIGINT NOT NULL,
    version INTEGER NOT NULL DEFAULT 1,
    changed_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, binding_name)
);

CREATE TABLE workforce_result_processor_environment_bindings (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    processor_release_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    type VARCHAR(32) NOT NULL,
    phase VARCHAR(32) NOT NULL DEFAULT 'EVALUATE',
    config_json JSONB,
    secret_ref TEXT NOT NULL DEFAULT '',
    secret_version TEXT NOT NULL DEFAULT '',
    task_override_allowed BOOLEAN NOT NULL DEFAULT FALSE,
    network_policy VARCHAR(32) NOT NULL DEFAULT 'NONE',
    created_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, processor_release_id, name)
);

CREATE TABLE workforce_result_processor_fixture_runs (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    revision_id BIGINT NOT NULL,
	-- Fixtures execute unpublished drafts.  Freeze source at request time so a
	-- later author edit cannot change queued test bytes.
	revision_manifest_json JSONB NOT NULL DEFAULT '{}'::jsonb,
	revision_files_json JSONB NOT NULL DEFAULT '[]'::jsonb,
	revision_digest TEXT NOT NULL DEFAULT '',
    harness_release_id BIGINT NOT NULL,
    harness_image_digest TEXT NOT NULL,
    evaluation_context_json JSONB NOT NULL,
    workspace_files_json JSONB NOT NULL,
    config_overrides_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    expected_decision VARCHAR(32) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    worker_id BIGINT,
    worker_instance_id TEXT,
    generation INTEGER NOT NULL DEFAULT 1,
    result_contract_json JSONB,
    result_contract_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    diagnostics TEXT NOT NULL DEFAULT '',
    created_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ
);

CREATE TABLE workforce_task_turns (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    tenant_id BIGINT NOT NULL,
    turn_number INTEGER NOT NULL,
	instruction TEXT NOT NULL,
	source_turn_id BIGINT,
	source_checkpoint_id BIGINT,
	execution_snapshot_digest TEXT NOT NULL,
	continuation_lineage_id BIGINT NOT NULL,
	created_by BIGINT NOT NULL,
	result_commit TEXT NOT NULL DEFAULT '',
	result_tree TEXT NOT NULL DEFAULT '',
	result_summary TEXT NOT NULL DEFAULT '',
	resume_nonce_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	idempotency_key TEXT,
	request_digest VARCHAR(64),
	review_revision_kind VARCHAR(32),
	source_review_run_id BIGINT,
	review_report_digest VARCHAR(64),
	revision_trigger_principal_id BIGINT,
    processor_release_id BIGINT NOT NULL,
    processor_digest VARCHAR(64) NOT NULL,
    environment_config_digest VARCHAR(64) NOT NULL DEFAULT '',
    environment_snapshot_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    secret_version_identities JSONB NOT NULL DEFAULT '[]'::jsonb,
    parameters_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    session_digest VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	UNIQUE (task_id, turn_number),
	CONSTRAINT workforce_task_turns_review_revision_ck CHECK (
		(review_revision_kind IS NULL AND source_review_run_id IS NULL AND review_report_digest IS NULL AND revision_trigger_principal_id IS NULL)
		OR
		(review_revision_kind='AUTOMATIC' AND source_review_run_id IS NOT NULL AND review_report_digest ~ '^[0-9a-f]{64}$' AND revision_trigger_principal_id IS NOT NULL)
		OR
		(review_revision_kind='HUMAN_AUTHORIZED' AND source_review_run_id IS NOT NULL AND (review_report_digest IS NULL OR review_report_digest ~ '^[0-9a-f]{64}$') AND revision_trigger_principal_id IS NOT NULL)
	)
);

CREATE TABLE workforce_result_evaluation_cycles (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    task_turn_id BIGINT NOT NULL,
    execution_node_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    cycle_number INTEGER NOT NULL,
    session_digest VARCHAR(64) NOT NULL DEFAULT '',
    candidate_tree_digest TEXT NOT NULL DEFAULT '',
    findings_digest VARCHAR(64) NOT NULL DEFAULT '',
    contract_digest VARCHAR(64) NOT NULL,
    decision VARCHAR(32) NOT NULL,
    reason_code VARCHAR(128) NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    contract_json JSONB NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PERSISTED',
    resume_nonce_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    persisted_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    UNIQUE (attempt_id, cycle_number)
);

CREATE TABLE workforce_continuation_checkpoints (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    task_turn_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    evaluation_cycle_id BIGINT NOT NULL,
    decision VARCHAR(32) NOT NULL,
    checkpoint_identity_sha256 VARCHAR(64) NOT NULL,
	format_version INTEGER NOT NULL DEFAULT 2,
	repository_bundle_object_key TEXT NOT NULL DEFAULT '',
	repository_bundle_content_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	repository_bundle_object_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	repository_bundle_size_bytes BIGINT NOT NULL DEFAULT 0,
	workspace_overlay_object_key TEXT NOT NULL DEFAULT '',
	workspace_overlay_content_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	workspace_overlay_object_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	workspace_overlay_size_bytes BIGINT NOT NULL DEFAULT 0,
	selection_digest VARCHAR(64) NOT NULL DEFAULT '',
	selection_policy_digest TEXT NOT NULL DEFAULT '',
	gitignore_digest VARCHAR(64) NOT NULL DEFAULT '',
    session_object_key TEXT NOT NULL,
    session_sha256 VARCHAR(64) NOT NULL,
    identity_manifest_object_key TEXT NOT NULL,
    manifest_sha256 VARCHAR(64) NOT NULL,
	task_execution_snapshot_digest TEXT NOT NULL DEFAULT '',
	base_commit TEXT NOT NULL DEFAULT '',
	result_commit TEXT NOT NULL DEFAULT '',
	result_tree TEXT NOT NULL DEFAULT '',
	session_lineage_digest TEXT NOT NULL DEFAULT '',
	session_content_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	session_object_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	session_size_bytes BIGINT NOT NULL DEFAULT 0,
	manifest_content_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	manifest_object_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	manifest_size_bytes BIGINT NOT NULL DEFAULT 0,
	encryption_algorithm TEXT NOT NULL DEFAULT '',
	encryption_key_id TEXT NOT NULL DEFAULT '',
	wrapped_data_key TEXT NOT NULL DEFAULT '',
	approved_by_principal_id BIGINT,
	approved_at TIMESTAMPTZ,
	version INTEGER NOT NULL DEFAULT 1,
	approval_reason TEXT NOT NULL DEFAULT '',
	consumed_by_turn_id BIGINT,
	consumed_at TIMESTAMPTZ,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    available_at TIMESTAMPTZ,
    expired_at TIMESTAMPTZ,
    UNIQUE (attempt_id, checkpoint_identity_sha256)
);

CREATE TABLE workforce_continuation_restore_runs (
	id BIGINT PRIMARY KEY,
	task_id BIGINT NOT NULL,
	source_turn_id BIGINT NOT NULL,
	target_turn_id BIGINT NOT NULL,
	execution_node_id BIGINT NOT NULL,
	attempt_id BIGINT NOT NULL,
	checkpoint_id BIGINT NOT NULL,
	worker_id BIGINT NOT NULL,
	worker_instance_id TEXT NOT NULL,
	host_boot_id TEXT NOT NULL,
	generation INTEGER NOT NULL,
	owner_epoch INTEGER NOT NULL,
	expected_identity_digest VARCHAR(64) NOT NULL,
	restored_identity_digest VARCHAR(64) NOT NULL DEFAULT '',
	status VARCHAR(32) NOT NULL,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at TIMESTAMPTZ,
	restored_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_result_materialization_runs (
	id BIGINT PRIMARY KEY,
	task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
	source_checkpoint_id BIGINT NOT NULL,
	output_id BIGINT,
	kind VARCHAR(32) NOT NULL,
	input_identity_digest VARCHAR(64) NOT NULL,
	worker_id BIGINT,
	worker_instance_id TEXT,
	host_boot_id TEXT,
	attempt_id BIGINT NOT NULL,
	generation INTEGER NOT NULL,
	lease_sha256 VARCHAR(64),
	lease_expires_at TIMESTAMPTZ,
	status VARCHAR(32) NOT NULL,
	run_number INTEGER NOT NULL DEFAULT 1,
	max_runs INTEGER NOT NULL DEFAULT 3,
	artifact_id BIGINT,
	artifact_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	artifact_size_bytes BIGINT NOT NULL DEFAULT 0,
	artifact_name TEXT NOT NULL DEFAULT '',
	artifact_media_type VARCHAR(255) NOT NULL DEFAULT '',
	spool_identity TEXT NOT NULL DEFAULT '',
	manifest_json JSONB,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	idempotency_key TEXT NOT NULL,
	request_digest VARCHAR(64) NOT NULL,
	created_by_principal_id BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	started_at TIMESTAMPTZ,
	finished_at TIMESTAMPTZ,
	next_retry_at TIMESTAMPTZ,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (task_id, idempotency_key)
);

CREATE TABLE workforce_result_outputs (
    id BIGINT PRIMARY KEY,
    task_id BIGINT NOT NULL,
    task_turn_id BIGINT NOT NULL,
    attempt_id BIGINT NOT NULL,
    evaluation_cycle_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    source_path TEXT NOT NULL,
	output_kind VARCHAR(32) NOT NULL DEFAULT 'PATH',
	archive_format VARCHAR(32) NOT NULL DEFAULT '',
	selection_spec_json JSONB NOT NULL DEFAULT '{}'::jsonb,
	selection_manifest_json JSONB NOT NULL DEFAULT '{}'::jsonb,
	selection_manifest_object_key TEXT NOT NULL DEFAULT '',
	selection_manifest_digest VARCHAR(64) NOT NULL DEFAULT '',
	selected_file_count INTEGER NOT NULL DEFAULT 0,
    storage VARCHAR(32) NOT NULL,
    artifact_id BIGINT,
    sha256 VARCHAR(64) NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'DECLARED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (evaluation_cycle_id, name)
);

CREATE TABLE workforce_workspace_sources (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	kind VARCHAR(32) NOT NULL,
	project_id BIGINT,
	project_version_id BIGINT,
	repository_config_digest VARCHAR(64) NOT NULL DEFAULT '',
	display_name TEXT NOT NULL,
	created_by_principal_id BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_workspace_source_revisions (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	workspace_source_id BIGINT NOT NULL,
	version INTEGER NOT NULL,
	kind VARCHAR(32) NOT NULL,
	project_id BIGINT,
	project_version_id BIGINT,
	repository_config_digest VARCHAR(64) NOT NULL DEFAULT '',
	ref_type VARCHAR(32) NOT NULL DEFAULT '',
	ref_name TEXT NOT NULL DEFAULT '',
	resolved_commit TEXT NOT NULL DEFAULT '',
	resolved_tree TEXT NOT NULL DEFAULT '',
	repository_bundle_object_key TEXT NOT NULL DEFAULT '',
	repository_bundle_content_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	repository_bundle_object_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	repository_bundle_size_bytes BIGINT NOT NULL DEFAULT 0,
	source_identity_digest VARCHAR(64) NOT NULL DEFAULT '',
	status VARCHAR(32) NOT NULL,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	resolved_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (workspace_source_id,version)
);

CREATE TABLE workforce_source_ingestion_runs (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	workspace_source_revision_id BIGINT,
	kind VARCHAR(32) NOT NULL,
	file_asset_handle TEXT NOT NULL DEFAULT '',
	input_object_key TEXT NOT NULL DEFAULT '',
	input_content_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	input_object_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	input_size_bytes BIGINT NOT NULL DEFAULT 0,
	project_id BIGINT,
	project_version_id BIGINT,
	repository_config_digest VARCHAR(64) NOT NULL DEFAULT '',
	requested_ref_type VARCHAR(32) NOT NULL DEFAULT '',
	requested_ref_name TEXT NOT NULL DEFAULT '',
	frozen_commit TEXT NOT NULL DEFAULT '',
	attempt_number INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL DEFAULT 3,
	worker_id BIGINT,
	worker_instance_id TEXT NOT NULL DEFAULT '',
	host_boot_id TEXT NOT NULL DEFAULT '',
	lease_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	lease_expires_at TIMESTAMPTZ,
	status VARCHAR(32) NOT NULL,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	next_retry_at TIMESTAMPTZ,
	idempotency_key TEXT NOT NULL,
	request_digest VARCHAR(64) NOT NULL,
	created_by_principal_id BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at TIMESTAMPTZ,
	UNIQUE (tenant_id,idempotency_key)
);

CREATE TABLE workforce_reference_assets (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	file_asset_handle TEXT NOT NULL,
	display_name TEXT NOT NULL,
	content_type VARCHAR(255) NOT NULL DEFAULT 'application/octet-stream',
	content_sha256 VARCHAR(64) NOT NULL,
	size_bytes BIGINT NOT NULL,
	object_key TEXT NOT NULL,
	object_sha256 VARCHAR(64) NOT NULL,
	status VARCHAR(32) NOT NULL,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	idempotency_key TEXT NOT NULL,
	request_digest VARCHAR(64) NOT NULL,
	created_by_principal_id BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (tenant_id,content_sha256,display_name),
	UNIQUE (tenant_id,idempotency_key)
);

-- Project is a stable identity. All mutable business configuration lives in
-- immutable ProjectVersion rows selected by current_version_id.
CREATE TABLE workforce_projects (
	id BIGINT PRIMARY KEY CONSTRAINT workforce_projects_id_ck CHECK (id>0),
	tenant_id BIGINT NOT NULL CONSTRAINT workforce_projects_tenant_id_ck CHECK (tenant_id>0),
	project_key TEXT NOT NULL CONSTRAINT workforce_projects_project_key_ck CHECK (btrim(project_key)<>''),
	status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE' CONSTRAINT workforce_projects_status_ck CHECK (status IN ('ACTIVE','ARCHIVED')),
	current_version_id BIGINT NOT NULL CONSTRAINT workforce_projects_current_version_id_ck CHECK (current_version_id>0),
	current_source_state VARCHAR(32) CONSTRAINT workforce_projects_source_state_ck CHECK (current_source_state IS NULL OR current_source_state IN ('PREPARING','READY','FAILED')),
	current_source_revision_id BIGINT CONSTRAINT workforce_projects_source_revision_id_ck CHECK (current_source_revision_id IS NULL OR current_source_revision_id>0),
	created_by_principal_id BIGINT NOT NULL CONSTRAINT workforce_projects_created_by_principal_id_ck CHECK (created_by_principal_id>0),
	updated_by_principal_id BIGINT NOT NULL CONSTRAINT workforce_projects_updated_by_principal_id_ck CHECK (updated_by_principal_id>0),
	create_idempotency_key TEXT NOT NULL CONSTRAINT workforce_projects_create_idempotency_key_ck CHECK (btrim(create_idempotency_key)<>''),
	create_request_digest VARCHAR(64) NOT NULL CONSTRAINT workforce_projects_create_request_digest_ck CHECK (create_request_digest ~ '^[0-9a-f]{64}$'),
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	archived_at TIMESTAMPTZ,
	CONSTRAINT workforce_projects_archive_shape_ck CHECK ((status='ACTIVE' AND archived_at IS NULL) OR (status='ARCHIVED' AND archived_at IS NOT NULL)),
	UNIQUE (tenant_id,project_key),
	UNIQUE (tenant_id,created_by_principal_id,create_idempotency_key)
);

CREATE TABLE workforce_project_versions (
	id BIGINT PRIMARY KEY CONSTRAINT workforce_project_versions_id_ck CHECK (id>0),
	tenant_id BIGINT NOT NULL CONSTRAINT workforce_project_versions_tenant_id_ck CHECK (tenant_id>0),
	project_id BIGINT NOT NULL CONSTRAINT workforce_project_versions_project_id_ck CHECK (project_id>0),
	name TEXT NOT NULL CONSTRAINT workforce_project_versions_name_ck CHECK (btrim(name)<>''),
	description TEXT NOT NULL DEFAULT '',
	department_id BIGINT CONSTRAINT workforce_project_versions_department_id_ck CHECK (department_id IS NULL OR department_id>0),
	kind VARCHAR(32) NOT NULL CONSTRAINT workforce_project_versions_kind_ck CHECK (kind IN ('CODEBASE','GENERAL')),
	repository_provider_type VARCHAR(32),
	repository_scheme VARCHAR(32),
	repository_host TEXT,
	repository_port INTEGER,
	repository_path TEXT,
	repository_display_url TEXT,
	repository_web_url TEXT,
	repository_credential_ciphertext BYTEA,
	repository_credential_edek BYTEA,
	repository_credential_wrap_key_ref TEXT,
	repository_credential_version INTEGER,
	repository_config_digest VARCHAR(64),
	repository_capabilities_json JSONB,
	repository_last_check_status VARCHAR(32),
	repository_last_check_reason_code VARCHAR(128),
	repository_last_checked_at TIMESTAMPTZ,
	default_ref_name TEXT,
	tech_stack_json JSONB NOT NULL DEFAULT '[]'::jsonb
		CONSTRAINT workforce_project_versions_tech_stack_ck CHECK (jsonb_typeof(tech_stack_json)='array'),
	agent_guidance TEXT NOT NULL DEFAULT '',
	completion_check_json JSONB NOT NULL DEFAULT '{"mode":"NONE"}'::jsonb
		CONSTRAINT workforce_project_versions_completion_check_ck CHECK (jsonb_typeof(completion_check_json)='object'),
	completion_check_digest VARCHAR(64) NOT NULL
		CONSTRAINT workforce_project_versions_completion_check_digest_ck CHECK (completion_check_digest ~ '^[0-9a-f]{64}$'),
	result_processor_release_id BIGINT NOT NULL
		CONSTRAINT workforce_project_versions_result_processor_release_id_ck CHECK (result_processor_release_id>0),
	result_processor_parameters_json JSONB NOT NULL DEFAULT '{}'::jsonb
		CONSTRAINT workforce_project_versions_result_processor_parameters_ck CHECK (jsonb_typeof(result_processor_parameters_json)='object'),
	review_prompt TEXT NOT NULL DEFAULT '',
	hook_release_ids BIGINT[] NOT NULL DEFAULT '{}',
	delivery_mode VARCHAR(32) NOT NULL DEFAULT 'NONE'
		CONSTRAINT workforce_project_versions_delivery_mode_ck CHECK (delivery_mode IN ('NONE','NEW_BRANCH','NEW_BRANCH_AND_DRAFT_REVIEW')),
	delivery_authorization VARCHAR(32) NOT NULL DEFAULT 'NONE'
		CONSTRAINT workforce_project_versions_delivery_authorization_ck CHECK (delivery_authorization IN ('NONE','AUTO_AFTER_GATES','ASK_AFTER_GATES')),
	delivery_authorized_by_principal_id BIGINT,
	result_branch_pattern TEXT NOT NULL DEFAULT '',
	notification_enabled BOOLEAN NOT NULL DEFAULT false,
	notification_events_json JSONB NOT NULL DEFAULT '[]'::jsonb
		CONSTRAINT workforce_project_versions_notification_events_ck CHECK (jsonb_typeof(notification_events_json)='array'),
	notification_group_route_id BIGINT,
	created_by_principal_id BIGINT NOT NULL CONSTRAINT workforce_project_versions_created_by_principal_id_ck CHECK (created_by_principal_id>0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	CONSTRAINT workforce_project_versions_repository_shape_ck CHECK (
		(kind='CODEBASE' AND repository_scheme IN ('http','https') AND btrim(repository_host)<>'' AND repository_port BETWEEN 1 AND 65535 AND btrim(repository_path)<>'' AND btrim(repository_display_url)<>'' AND ((repository_credential_ciphertext IS NULL AND repository_credential_edek IS NULL AND repository_credential_wrap_key_ref IS NULL AND repository_credential_version IS NULL) OR (octet_length(repository_credential_ciphertext)>0 AND octet_length(repository_credential_edek)>0 AND btrim(repository_credential_wrap_key_ref)<>'' AND repository_credential_version>0)) AND repository_config_digest ~ '^[0-9a-f]{64}$' AND btrim(default_ref_name)<>'')
		OR
		(kind='GENERAL' AND repository_provider_type IS NULL AND repository_scheme IS NULL AND repository_host IS NULL AND repository_port IS NULL AND repository_path IS NULL AND repository_display_url IS NULL AND repository_web_url IS NULL AND repository_credential_ciphertext IS NULL AND repository_credential_edek IS NULL AND repository_credential_wrap_key_ref IS NULL AND repository_credential_version IS NULL AND repository_config_digest IS NULL AND repository_capabilities_json IS NULL AND repository_last_check_status IS NULL AND repository_last_check_reason_code IS NULL AND repository_last_checked_at IS NULL AND default_ref_name IS NULL)
	),
	CONSTRAINT workforce_project_versions_delivery_shape_ck CHECK (
		(delivery_mode='NONE' AND delivery_authorization='NONE' AND delivery_authorized_by_principal_id IS NULL AND result_branch_pattern='')
		OR
		(kind='CODEBASE' AND delivery_mode<>'NONE' AND btrim(result_branch_pattern)<>'' AND (
			(delivery_authorization='AUTO_AFTER_GATES' AND delivery_authorized_by_principal_id>0)
			OR (delivery_authorization='ASK_AFTER_GATES' AND delivery_authorized_by_principal_id IS NULL)
		))
	),
	UNIQUE (tenant_id,project_id,id)
);

-- ProjectVersion is append-only. Project updates create a new row and only
-- move workforce_projects.current_version_id.
-- +goose StatementBegin
CREATE FUNCTION workforce_project_version_immutable() RETURNS trigger
LANGUAGE plpgsql
AS $workforce_project_version_immutable$
BEGIN
	RAISE EXCEPTION 'PROJECT_VERSION_IMMUTABLE';
END;
$workforce_project_version_immutable$;
-- +goose StatementEnd

CREATE TRIGGER workforce_project_versions_immutable_trg
BEFORE UPDATE OR DELETE ON workforce_project_versions
FOR EACH ROW EXECUTE FUNCTION workforce_project_version_immutable();

CREATE TABLE workforce_task_input_snapshots (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
	source_kind VARCHAR(32) NOT NULL,
	workspace_source_revision_id BIGINT,
	source_commit TEXT NOT NULL,
	source_tree TEXT NOT NULL,
	source_identity_digest VARCHAR(64) NOT NULL,
	repository_bundle_object_key TEXT NOT NULL,
	repository_bundle_content_sha256 VARCHAR(64) NOT NULL,
	repository_bundle_object_sha256 VARCHAR(64) NOT NULL,
	repository_bundle_size_bytes BIGINT NOT NULL,
	input_snapshot_digest TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (task_id,task_turn_id)
);

CREATE TABLE workforce_task_input_references (
	task_input_snapshot_id BIGINT NOT NULL,
	reference_asset_id BIGINT NOT NULL,
	reference_content_sha256 VARCHAR(64) NOT NULL,
	display_order INTEGER NOT NULL DEFAULT 0,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	PRIMARY KEY (task_input_snapshot_id,reference_asset_id)
);

CREATE TABLE workforce_task_git_plans (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
	project_id BIGINT,
	project_version_id BIGINT,
	repository_provider_type VARCHAR(32) NOT NULL DEFAULT '',
	repository_scheme VARCHAR(32) NOT NULL DEFAULT '',
	repository_host TEXT NOT NULL DEFAULT '',
	repository_port INTEGER NOT NULL DEFAULT 0,
	repository_path TEXT NOT NULL DEFAULT '',
	repository_display_url TEXT NOT NULL DEFAULT '',
	repository_web_url TEXT NOT NULL DEFAULT '',
	repository_config_digest VARCHAR(64) NOT NULL DEFAULT '',
	repository_credential_ciphertext BYTEA NOT NULL DEFAULT '\x',
	repository_credential_edek BYTEA NOT NULL DEFAULT '\x',
	repository_credential_wrap_key_ref TEXT NOT NULL DEFAULT '',
	repository_credential_version INTEGER NOT NULL DEFAULT 0,
	review_base_source_revision_id BIGINT,
	source_ref_type VARCHAR(32) NOT NULL DEFAULT '',
	source_ref_name TEXT NOT NULL DEFAULT '',
	source_commit TEXT NOT NULL,
	review_base_branch TEXT NOT NULL DEFAULT '',
	review_base_commit TEXT NOT NULL DEFAULT '',
	result_branch TEXT NOT NULL DEFAULT '',
	delivery_mode VARCHAR(32) NOT NULL,
	delivery_authorization VARCHAR(32) NOT NULL DEFAULT '',
	delivery_authorized_by_principal_id BIGINT,
	delivery_authorized_at TIMESTAMPTZ,
	target_config_digest VARCHAR(64) NOT NULL DEFAULT '',
	push_authorization_digest VARCHAR(64) NOT NULL DEFAULT '',
	review_authorization_digest VARCHAR(64) NOT NULL DEFAULT '',
	full_project_import BOOLEAN NOT NULL DEFAULT FALSE,
	previous_delivery_commit TEXT NOT NULL DEFAULT '',
	plan_digest TEXT NOT NULL,
	status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	CONSTRAINT workforce_task_git_plans_delivery_authorization_ck CHECK (
		(delivery_mode='NONE' AND delivery_authorization='' AND delivery_authorized_by_principal_id IS NULL AND delivery_authorized_at IS NULL AND target_config_digest='' AND push_authorization_digest='' AND review_authorization_digest='')
		OR
		(delivery_mode IN ('NEW_BRANCH','NEW_BRANCH_AND_DRAFT_REVIEW') AND (
			(delivery_authorization='ASK_AFTER_GATES' AND delivery_authorized_by_principal_id IS NULL AND delivery_authorized_at IS NULL AND (target_config_digest='' OR target_config_digest ~ '^[0-9a-f]{64}$') AND (push_authorization_digest='' OR push_authorization_digest ~ '^[0-9a-f]{64}$') AND (review_authorization_digest='' OR review_authorization_digest ~ '^[0-9a-f]{64}$'))
			OR
			(delivery_authorization='AUTO_AFTER_GATES' AND delivery_authorized_by_principal_id>0 AND delivery_authorized_at IS NOT NULL AND target_config_digest ~ '^[0-9a-f]{64}$' AND push_authorization_digest ~ '^[0-9a-f]{64}$' AND ((delivery_mode='NEW_BRANCH' AND review_authorization_digest='') OR (delivery_mode='NEW_BRANCH_AND_DRAFT_REVIEW' AND review_authorization_digest ~ '^[0-9a-f]{64}$')))
		))
	),
	UNIQUE (task_id,task_turn_id)
);

CREATE TABLE workforce_delivery_operations (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	delivery_request_id BIGINT NOT NULL,
	operation_kind VARCHAR(32) NOT NULL,
	status VARCHAR(32) NOT NULL,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	max_attempts INTEGER NOT NULL DEFAULT 3,
	authorization_version TEXT NOT NULL DEFAULT '',
	authorization_digest VARCHAR(64) NOT NULL DEFAULT '',
	reauthorized_by_principal_id BIGINT,
	reauthorized_at TIMESTAMPTZ,
	reauthorization_reason TEXT NOT NULL DEFAULT '',
	idempotency_key TEXT NOT NULL,
	request_digest VARCHAR(64) NOT NULL,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	next_retry_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at TIMESTAMPTZ,
	UNIQUE (delivery_request_id,operation_kind),
	UNIQUE (tenant_id,idempotency_key)
);

CREATE TABLE workforce_pull_request_publications (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
	delivery_request_id BIGINT NOT NULL,
	delivery_operation_id BIGINT NOT NULL,
	project_id BIGINT NOT NULL,
	provider_type VARCHAR(32) NOT NULL,
	provider_review_id TEXT NOT NULL DEFAULT '',
	provider_number TEXT NOT NULL DEFAULT '',
	status VARCHAR(32) NOT NULL,
	head_branch TEXT NOT NULL,
	base_branch TEXT NOT NULL,
	head_commit TEXT NOT NULL,
	frozen_base_commit TEXT NOT NULL,
	observed_base_commit TEXT NOT NULL DEFAULT '',
	web_url TEXT NOT NULL DEFAULT '',
	marker_digest TEXT NOT NULL,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (project_id,head_branch,base_branch)
);

CREATE TABLE workforce_delivery_requests (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
	checkpoint_id BIGINT NOT NULL,
	action_type VARCHAR(64) NOT NULL,
	action_json JSONB NOT NULL DEFAULT '{}'::jsonb,
	action_digest TEXT NOT NULL,
	processor_release_id BIGINT NOT NULL,
	processor_digest VARCHAR(64) NOT NULL,
	result_commit TEXT NOT NULL,
	result_tree TEXT NOT NULL,
	selection_digest VARCHAR(64) NOT NULL,
	project_id BIGINT NOT NULL,
	target_config_digest VARCHAR(64) NOT NULL,
	target_identity_digest VARCHAR(64) NOT NULL DEFAULT '',
	policy_digest TEXT NOT NULL,
	credential_version TEXT NOT NULL DEFAULT '',
	authorization_digest VARCHAR(64) NOT NULL DEFAULT '',
	reauthorized_by_principal_id BIGINT,
	reauthorized_at TIMESTAMPTZ,
	reauthorized_project_version_id BIGINT,
	reauthorization_reason TEXT NOT NULL DEFAULT '',
	base_branch TEXT NOT NULL,
	destination_branch TEXT NOT NULL,
	expected_base_commit TEXT NOT NULL,
	delivery_commit TEXT NOT NULL,
	commit_timestamp TIMESTAMPTZ NOT NULL,
	approval_mode VARCHAR(32) NOT NULL,
	approved_by_principal_id BIGINT,
	approved_at TIMESTAMPTZ,
	status VARCHAR(32) NOT NULL,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	version INTEGER NOT NULL DEFAULT 1 CONSTRAINT workforce_delivery_requests_version_ck CHECK (version>0),
	delivery_completed_notified_at TIMESTAMPTZ,
	idempotency_key TEXT NOT NULL,
	request_digest VARCHAR(64) NOT NULL,
	created_by_principal_id BIGINT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at TIMESTAMPTZ,
	UNIQUE (tenant_id,idempotency_key),
	UNIQUE (task_turn_id,action_digest)
);

CREATE TABLE workforce_delivery_intents (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	task_id BIGINT NOT NULL,
	task_turn_id BIGINT NOT NULL,
	attempt_id BIGINT NOT NULL,
	adapter_session_generation INTEGER NOT NULL,
	local_tool_call_key TEXT NOT NULL,
	native_tool_call_id TEXT NOT NULL,
	round_key TEXT NOT NULL,
	summary VARCHAR(2048) NOT NULL,
	summary_digest VARCHAR(64) NOT NULL,
	git_plan_digest VARCHAR(64) NOT NULL,
	delivery_mode VARCHAR(32) NOT NULL,
	delivery_authorization VARCHAR(32) NOT NULL,
	delivery_request_id BIGINT,
	status VARCHAR(32) NOT NULL,
	version INTEGER NOT NULL DEFAULT 1,
	adapter_received_at TIMESTAMPTZ,
	closed_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	CONSTRAINT workforce_delivery_intents_identity_ck CHECK (
		id>0 AND tenant_id>0 AND task_id>0 AND task_turn_id>0 AND attempt_id>0 AND adapter_session_generation>0
		AND (delivery_request_id IS NULL OR delivery_request_id>0) AND version>0
	),
	CONSTRAINT workforce_delivery_intents_tool_identity_ck CHECK (
		octet_length(local_tool_call_key) BETWEEN 1 AND 128
		AND octet_length(native_tool_call_id) BETWEEN 1 AND 512
		AND octet_length(round_key) BETWEEN 1 AND 128
	),
	CONSTRAINT workforce_delivery_intents_summary_ck CHECK (
		summary=btrim(summary) AND octet_length(summary) BETWEEN 1 AND 2048
	),
	CONSTRAINT workforce_delivery_intents_digest_ck CHECK (
		summary_digest ~ '^[0-9a-f]{64}$' AND git_plan_digest ~ '^[0-9a-f]{64}$'
	),
	CONSTRAINT workforce_delivery_intents_mode_ck CHECK (delivery_mode IN ('NEW_BRANCH','NEW_BRANCH_AND_DRAFT_REVIEW')),
	CONSTRAINT workforce_delivery_intents_authorization_ck CHECK (delivery_authorization IN ('AUTO_AFTER_GATES','ASK_AFTER_GATES')),
	CONSTRAINT workforce_delivery_intents_status_ck CHECK (status IN ('SCHEDULED','WAITING_FINALIZATION','WAITING_CHECKS','WAITING_REVIEW','WAITING_USER_DECISION','DELIVERY_RUNNING','DELIVERED','DELIVERY_FAILED','OUTCOME_UNKNOWN','SKIPPED','CANCELLED')),
	UNIQUE (attempt_id,adapter_session_generation,local_tool_call_key),
	UNIQUE (task_turn_id)
);

CREATE TABLE workforce_delivery_decisions (
	id BIGINT PRIMARY KEY,
	tenant_id BIGINT NOT NULL,
	task_id BIGINT NOT NULL,
	delivery_request_id BIGINT NOT NULL,
	action VARCHAR(32) NOT NULL,
	expected_version INTEGER NOT NULL,
	principal_id BIGINT NOT NULL,
	idempotency_key TEXT NOT NULL,
	request_digest VARCHAR(64) NOT NULL,
	authorization_digest VARCHAR(64) NOT NULL,
	reason VARCHAR(2048) NOT NULL DEFAULT '',
	applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	CONSTRAINT workforce_delivery_decisions_identity_ck CHECK (id>0 AND tenant_id>0 AND task_id>0 AND delivery_request_id>0 AND expected_version>0 AND principal_id>0),
	CONSTRAINT workforce_delivery_decisions_action_ck CHECK (action IN ('AUTO_FROM_TASK_PLAN','DELIVER','SKIP_THIS_RESULT','RESTART_DELIVERY')),
	CONSTRAINT workforce_delivery_decisions_idempotency_ck CHECK (octet_length(idempotency_key) BETWEEN 1 AND 128),
	CONSTRAINT workforce_delivery_decisions_digest_ck CHECK (request_digest ~ '^[0-9a-f]{64}$' AND authorization_digest ~ '^[0-9a-f]{64}$'),
	CONSTRAINT workforce_delivery_decisions_reason_ck CHECK (octet_length(reason)<=2048),
	UNIQUE (tenant_id,principal_id,idempotency_key)
);

CREATE TABLE workforce_delivery_attempts (
	id BIGINT PRIMARY KEY,
	request_id BIGINT NOT NULL,
	attempt_number INTEGER NOT NULL,
	worker_id BIGINT NOT NULL,
	worker_instance_id TEXT NOT NULL,
	host_boot_id TEXT NOT NULL,
	generation INTEGER NOT NULL DEFAULT 1,
	lease_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	lease_expires_at TIMESTAMPTZ,
	push_authorization_sha256 VARCHAR(64) NOT NULL DEFAULT '',
	push_authorization_expires_at TIMESTAMPTZ,
	phase VARCHAR(32) NOT NULL DEFAULT 'PENDING',
	status VARCHAR(32) NOT NULL,
	remote_before TEXT NOT NULL DEFAULT '',
	remote_after TEXT NOT NULL DEFAULT '',
	push_started_at TIMESTAMPTZ,
	verified_at TIMESTAMPTZ,
	reason_code VARCHAR(128) NOT NULL DEFAULT '',
	reconciliation_only BOOLEAN NOT NULL DEFAULT false,
	next_retry_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
	finished_at TIMESTAMPTZ,
	UNIQUE (request_id,attempt_number)
);

-- This is an unpublished schema. Fresh installs have no Task rows while the
-- migration chain is running, so historical TaskTurn/ExecutionNode creation
-- deliberately does not live in a migration. The local alignment command is
-- the only supported path for preserving and rewriting an existing text-ID
-- development database with Framework Snowflake identities.

ALTER TABLE workforce_tasks ADD COLUMN result_processor_release_id BIGINT NOT NULL
	CONSTRAINT workforce_tasks_result_processor_release_id_ck CHECK (result_processor_release_id>0);
ALTER TABLE workforce_tasks ADD COLUMN result_processor_parameters_json JSONB NOT NULL DEFAULT '{}'::jsonb
	CONSTRAINT workforce_tasks_result_processor_parameters_ck CHECK (jsonb_typeof(result_processor_parameters_json)='object');
ALTER TABLE workforce_tasks
	ADD COLUMN project_id BIGINT NOT NULL
		CONSTRAINT workforce_tasks_project_id_ck CHECK (project_id>0),
		ADD COLUMN project_version_id BIGINT NOT NULL
			CONSTRAINT workforce_tasks_project_version_id_ck CHECK (project_version_id>0),
		ADD COLUMN review_enabled BOOLEAN NOT NULL DEFAULT TRUE,
		ADD COLUMN model_credential_id BIGINT,
		ADD COLUMN primary_model VARCHAR(256),
		ADD COLUMN selected_runtime_release_id BIGINT,
		ADD COLUMN execution_timeout_seconds INTEGER,
		ADD COLUMN execution_active_milliseconds BIGINT NOT NULL DEFAULT 0,
	ADD COLUMN execution_requirements_json JSONB NOT NULL
		CONSTRAINT workforce_tasks_execution_requirements_ck CHECK (jsonb_typeof(execution_requirements_json)='object'),
	ADD COLUMN execution_requirements_digest VARCHAR(64) NOT NULL
		CONSTRAINT workforce_tasks_execution_requirements_digest_ck CHECK (execution_requirements_digest ~ '^[0-9a-f]{64}$');
ALTER TABLE workforce_tasks ADD CONSTRAINT workforce_tasks_model_selection_ck CHECK (
	(model_credential_id IS NULL AND primary_model IS NULL)
	OR (model_credential_id>0 AND btrim(primary_model)<>''));
ALTER TABLE workforce_tasks ADD CONSTRAINT workforce_tasks_selected_runtime_release_id_ck
	CHECK (selected_runtime_release_id IS NULL OR selected_runtime_release_id>0);
COMMENT ON COLUMN workforce_tasks.selected_runtime_release_id IS '任务选择的执行镜像版本';
ALTER TABLE workforce_tasks ADD CONSTRAINT workforce_tasks_execution_timeout_seconds_ck CHECK (execution_timeout_seconds IS NULL OR execution_timeout_seconds>0);
COMMENT ON COLUMN workforce_tasks.execution_timeout_seconds IS '任务设置的 Agent 执行时限，秒';
ALTER TABLE workforce_tasks ADD CONSTRAINT workforce_tasks_execution_active_milliseconds_ck CHECK (execution_active_milliseconds>=0);
COMMENT ON COLUMN workforce_tasks.execution_active_milliseconds IS '已确认的 Agent 活跃执行时间，毫秒';
ALTER TABLE workforce_tasks ADD COLUMN current_task_turn_id BIGINT;
ALTER TABLE workforce_tasks ADD COLUMN current_turn_number INTEGER NOT NULL DEFAULT 1;
ALTER TABLE workforce_tasks ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE workforce_tasks ADD COLUMN tags_json JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE workforce_tasks ADD COLUMN metadata_version INTEGER NOT NULL DEFAULT 1;
UPDATE workforce_tasks t
   SET name=LEFT(REGEXP_REPLACE(COALESCE(NULLIF(SPLIT_PART(t.task_instruction,E'\n',1),''),t.id::TEXT),'[[:space:]]+',' ','g'),80),
       tags_json=COALESCE(t.tags_json,'[]'::jsonb)
 WHERE t.name='';
ALTER TABLE workforce_task_attempts ADD COLUMN task_turn_id BIGINT;
CREATE INDEX workforce_result_processor_releases_lookup_idx ON workforce_result_processor_releases (tenant_id, id);
CREATE INDEX workforce_result_processor_fixture_dispatch_idx ON workforce_result_processor_fixture_runs (status, harness_image_digest, created_at);
CREATE INDEX workforce_task_turns_task_idx ON workforce_task_turns (task_id, turn_number);
CREATE INDEX workforce_projects_tenant_status_idx ON workforce_projects (tenant_id,status,id);
CREATE INDEX workforce_project_versions_project_idx ON workforce_project_versions (tenant_id,project_id,id DESC);
CREATE INDEX workforce_project_versions_department_idx ON workforce_project_versions (tenant_id,department_id,id DESC);
CREATE INDEX workforce_project_versions_repository_idx ON workforce_project_versions (tenant_id,repository_host,repository_path) WHERE kind='CODEBASE';
CREATE INDEX workforce_tasks_project_created_idx ON workforce_tasks (tenant_id,project_id,created_at DESC,id);
CREATE INDEX workforce_tasks_tenant_created_idx ON workforce_tasks (tenant_id,created_at DESC);
CREATE INDEX workforce_tasks_creator_created_idx ON workforce_tasks (tenant_id,created_by,created_at DESC);
CREATE INDEX workforce_tasks_tenant_project_finished_idx ON workforce_tasks (tenant_id,project_id,finished_at DESC,id) WHERE finished_at IS NOT NULL;
CREATE INDEX workforce_tasks_tags_gin_idx ON workforce_tasks USING GIN (tags_json);
CREATE UNIQUE INDEX workforce_task_turns_active_idx ON workforce_task_turns (task_id) WHERE status='ACTIVE';
CREATE UNIQUE INDEX workforce_task_turns_idempotency_idx ON workforce_task_turns (task_id,idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE UNIQUE INDEX workforce_task_turns_source_review_run_uidx ON workforce_task_turns (source_review_run_id) WHERE source_review_run_id IS NOT NULL;
CREATE UNIQUE INDEX workforce_checkpoint_consumed_once_idx ON workforce_continuation_checkpoints (id) WHERE consumed_by_turn_id IS NOT NULL;
CREATE UNIQUE INDEX workforce_restore_active_turn_idx ON workforce_continuation_restore_runs (target_turn_id) WHERE status IN ('PENDING','RUNNING');
CREATE UNIQUE INDEX workforce_materialization_identity_output_idx ON workforce_result_materialization_runs (task_turn_id,kind,input_identity_digest,output_id) WHERE output_id IS NOT NULL;
CREATE UNIQUE INDEX workforce_materialization_identity_platform_idx ON workforce_result_materialization_runs (task_turn_id,kind,input_identity_digest) WHERE output_id IS NULL;
CREATE INDEX workforce_evaluation_cycles_attempt_idx ON workforce_result_evaluation_cycles (attempt_id, cycle_number);
CREATE INDEX workforce_checkpoint_expiry_idx ON workforce_continuation_checkpoints (status, expires_at);
CREATE INDEX workforce_result_outputs_task_idx ON workforce_result_outputs (task_id, created_at);
CREATE UNIQUE INDEX workforce_running_evaluation_attempt_idx ON workforce_result_evaluation_cycles (attempt_id) WHERE status='RUNNING';
CREATE INDEX workforce_workspace_source_revisions_lookup_idx ON workforce_workspace_source_revisions (tenant_id,status,created_at DESC);
CREATE UNIQUE INDEX workforce_workspace_source_revisions_identity_uidx ON workforce_workspace_source_revisions (tenant_id,source_identity_digest) WHERE source_identity_digest <> '';
CREATE INDEX workforce_source_ingestion_dispatch_idx ON workforce_source_ingestion_runs (status,next_retry_at,created_at);
CREATE INDEX workforce_reference_assets_tenant_idx ON workforce_reference_assets (tenant_id,status,created_at DESC);
CREATE INDEX workforce_task_input_snapshots_task_idx ON workforce_task_input_snapshots (task_id,task_turn_id);
CREATE INDEX workforce_task_git_plans_task_idx ON workforce_task_git_plans (task_id,task_turn_id);
CREATE INDEX workforce_delivery_intents_task_idx ON workforce_delivery_intents (tenant_id,task_id,status,id DESC);
CREATE INDEX workforce_delivery_intents_request_idx ON workforce_delivery_intents (delivery_request_id) WHERE delivery_request_id IS NOT NULL;
CREATE INDEX workforce_delivery_decisions_request_idx ON workforce_delivery_decisions (delivery_request_id,id DESC);
CREATE UNIQUE INDEX workforce_delivery_requests_id_version_uidx ON workforce_delivery_requests (id,version);
CREATE INDEX workforce_delivery_operations_dispatch_idx ON workforce_delivery_operations (status,next_retry_at,created_at);
CREATE INDEX workforce_pull_request_publications_task_idx ON workforce_pull_request_publications (task_id,created_at DESC);
CREATE INDEX workforce_delivery_requests_task_idx ON workforce_delivery_requests (task_id,created_at DESC);
CREATE INDEX workforce_delivery_requests_tenant_project_created_idx ON workforce_delivery_requests (tenant_id,project_id,created_at DESC,id);
CREATE INDEX workforce_delivery_requests_dispatch_idx ON workforce_delivery_requests (status,created_at);
CREATE UNIQUE INDEX workforce_delivery_active_destination_idx ON workforce_delivery_requests (project_id,destination_branch) WHERE status IN ('READY','DISPATCHED','RUNNING','CONFIRMING','OUTCOME_UNKNOWN');
CREATE UNIQUE INDEX workforce_delivery_succeeded_attempt_idx ON workforce_delivery_attempts (request_id) WHERE status='SUCCEEDED';
CREATE INDEX workforce_delivery_attempt_retry_idx ON workforce_delivery_attempts (status,next_retry_at,worker_id);

CREATE TABLE workforce_task_notification_subscriptions (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    task_id BIGINT NOT NULL,
    subscriber_user_id BIGINT NOT NULL,
    events_json JSONB NOT NULL CHECK (jsonb_typeof(events_json)='array'),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_by BIGINT NOT NULL,
    updated_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id,task_id,subscriber_user_id)
);

CREATE TABLE workforce_task_notification_subscription_channels (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subscription_id BIGINT NOT NULL,
    channel_kind VARCHAR(32) NOT NULL CHECK (channel_kind IN ('IN_APP','EMAIL_USER','FEISHU_DIRECT','WECOM_DIRECT','GROUP_ROUTE')),
    group_route_id BIGINT,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((channel_kind='GROUP_ROUTE' AND group_route_id IS NOT NULL) OR (channel_kind<>'GROUP_ROUTE' AND group_route_id IS NULL)),
    UNIQUE (subscription_id,channel_kind,group_route_id)
);
CREATE UNIQUE INDEX workforce_task_notification_subscription_channel_direct_uidx
    ON workforce_task_notification_subscription_channels (subscription_id,channel_kind) WHERE group_route_id IS NULL;
CREATE INDEX workforce_task_notification_subscription_channels_route_idx
    ON workforce_task_notification_subscription_channels (tenant_id,group_route_id,subscription_id) WHERE group_route_id IS NOT NULL;

CREATE TABLE workforce_task_notification_records (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    task_id BIGINT NOT NULL,
    task_turn_id BIGINT NOT NULL,
    source_type VARCHAR(64) NOT NULL,
    source_id BIGINT NOT NULL,
    source_occurrence BIGINT NOT NULL DEFAULT 0 CHECK (source_occurrence >= 0),
    fact_identity_version INTEGER NOT NULL DEFAULT 1 CHECK (fact_identity_version IN (1,2)),
    fact_type VARCHAR(64) NOT NULL,
    reason_code VARCHAR(128) NOT NULL DEFAULT '',
    focus VARCHAR(32) NOT NULL,
    subscription_id BIGINT,
    subscriber_user_id BIGINT NOT NULL,
    subscription_version INTEGER CHECK (subscription_version > 0),
    task_name_snapshot TEXT NOT NULL,
    turn_number INTEGER NOT NULL CHECK (turn_number > 0),
    summary_snapshot TEXT NOT NULL DEFAULT '',
    title_snapshot TEXT NOT NULL,
    content_snapshot TEXT NOT NULL,
    publication_contract_version INTEGER NOT NULL DEFAULT 1 CHECK (publication_contract_version IN (1,2)),
    scene_code TEXT NOT NULL DEFAULT '',
    template_variables_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    deep_link TEXT NOT NULL,
    idempotency_identity TEXT NOT NULL,
    request_digest VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING',
    publish_attempts INTEGER NOT NULL DEFAULT 0 CHECK (publish_attempts >= 0),
    next_retry_at TIMESTAMPTZ,
    lease_token TEXT NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    notification_receipt_identity TEXT NOT NULL DEFAULT '',
    warning_code VARCHAR(128) NOT NULL DEFAULT '',
    last_error_code VARCHAR(128) NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (status IN ('PENDING','PROCESSING','RETRY_WAIT','PUBLISHED','SUPPRESSED','FAILED')),
    CHECK (fact_type IN ('TASK_ACTION_REQUIRED','TASK_INTERRUPTED','TASK_COMPLETED','DELIVERY_COMPLETED')),
    CHECK (source_type IN ('CONTINUATION_CHECKPOINT','FINALIZATION_RUN','ARTIFACT_UPLOAD','RESULT_EVALUATION','TASK_ATTEMPT','DELIVERY_REQUEST','REVIEW_OPERATION','INDEPENDENT_REVIEW_RUN','RUNTIME_RECOVERY','TASK_TIMEOUT','TASK_CANCEL')),
    CHECK (focus IN ('evaluation','result','continuation','delivery')),
    CHECK (status='SUPPRESSED' OR (subscription_id IS NOT NULL AND subscription_version IS NOT NULL))
);
CREATE UNIQUE INDEX workforce_task_notification_records_fact_v2_uidx
    ON workforce_task_notification_records (tenant_id,task_id,task_turn_id,fact_type,source_type,source_id,source_occurrence,subscriber_user_id);

CREATE TABLE workforce_task_notification_record_recipients (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    record_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    recipient_role VARCHAR(32) NOT NULL CHECK (recipient_role IN ('SUBSCRIBER','ATTENTION')),
    personal_channel_policy VARCHAR(32) NOT NULL CHECK (personal_channel_policy IN ('TASK_SUBSCRIPTION','USER_PREFERENCE')),
    in_app_required BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (record_id,user_id)
);
CREATE INDEX workforce_task_notification_record_recipients_user_idx
    ON workforce_task_notification_record_recipients (tenant_id,user_id,created_at DESC);

CREATE TABLE workforce_task_notification_record_channels (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    record_id BIGINT NOT NULL,
    channel_kind VARCHAR(32) NOT NULL CHECK (channel_kind IN ('IN_APP','EMAIL_USER','FEISHU_DIRECT','WECOM_DIRECT','GROUP_ROUTE')),
    group_route_id BIGINT,
    mention_user_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((channel_kind='GROUP_ROUTE' AND group_route_id IS NOT NULL) OR (channel_kind<>'GROUP_ROUTE' AND group_route_id IS NULL)),
    UNIQUE (record_id,channel_kind,group_route_id)
);
CREATE UNIQUE INDEX workforce_task_notification_record_channel_direct_uidx
    ON workforce_task_notification_record_channels (record_id,channel_kind) WHERE group_route_id IS NULL;
CREATE INDEX workforce_task_notification_record_channels_route_idx
    ON workforce_task_notification_record_channels (tenant_id,group_route_id,record_id) WHERE group_route_id IS NOT NULL;

CREATE INDEX workforce_task_notification_subscriptions_lookup_idx
    ON workforce_task_notification_subscriptions (tenant_id,task_id,enabled);
CREATE INDEX workforce_task_notification_records_task_idx
    ON workforce_task_notification_records (tenant_id,task_id,occurred_at DESC,id DESC);
CREATE INDEX workforce_task_notification_records_dispatch_idx
    ON workforce_task_notification_records (status,next_retry_at,lease_until,occurred_at)
    WHERE status IN ('PENDING','PROCESSING','RETRY_WAIT');

-- Independent Reviewer Slice 3: a ReviewRun is a durable control-plane
-- aggregate, not a Provider call or an Agent attempt.  No foreign keys are
-- used: cross-table integrity is checked by the application and alignment.
CREATE TABLE workforce_review_runs (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL CHECK (tenant_id > 0),
    task_id BIGINT NOT NULL CHECK (task_id > 0),
    task_turn_id BIGINT NOT NULL CHECK (task_turn_id > 0),
    attempt_id BIGINT NOT NULL CHECK (attempt_id > 0),
    finalization_run_id BIGINT NOT NULL CHECK (finalization_run_id > 0),
    result_commit TEXT NOT NULL CHECK (result_commit <> ''),
    result_tree TEXT NOT NULL CHECK (result_tree <> ''),
    runtime_release_id BIGINT,
    reviewer_image_digest TEXT NOT NULL DEFAULT '' CHECK (reviewer_image_digest='' OR reviewer_image_digest ~ '^sha256:[0-9a-f]{64}$'),
    adapter_identity TEXT NOT NULL DEFAULT '',
    adapter_version TEXT NOT NULL DEFAULT '',
    adapter_protocol VARCHAR(64) NOT NULL DEFAULT '',
    adapter_capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(adapter_capabilities_json)='array'),
    model_credential_id BIGINT,
    credential_version INTEGER,
    model_protocol VARCHAR(16) NOT NULL DEFAULT '',
    primary_model VARCHAR(256) NOT NULL DEFAULT '',
    provider_proxy_image_digest TEXT NOT NULL DEFAULT '' CHECK (provider_proxy_image_digest='' OR provider_proxy_image_digest ~ '^sha256:[0-9a-f]{64}$'),
    selection_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK (selection_digest='' OR selection_digest ~ '^[0-9a-f]{64}$'),
    hard_timeout_seconds INTEGER NOT NULL CHECK (hard_timeout_seconds BETWEEN 60 AND 14400),
    transient_retry_limit INTEGER NOT NULL CHECK (transient_retry_limit BETWEEN 0 AND 10),
    automatic_revision_limit INTEGER NOT NULL CHECK (automatic_revision_limit BETWEEN 0 AND 10),
    identity_digest VARCHAR(64) NOT NULL CHECK (identity_digest ~ '^[0-9a-f]{64}$'),
    status VARCHAR(32) NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','ASSIGNED','RUNNING','VALIDATING_REPORT','PASSED','REVISION_REQUIRED','REJECTED','RETRY_WAIT','MANUAL_REVIEW','CANCELLED')),
    worker_id BIGINT,
    worker_instance_id TEXT NOT NULL DEFAULT '',
    host_boot_id TEXT NOT NULL DEFAULT '',
    owner_epoch INTEGER NOT NULL DEFAULT 0 CHECK (owner_epoch >= 0),
    assignment_generation INTEGER NOT NULL DEFAULT 0 CHECK (assignment_generation >= 0),
    attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    lease_token_sha256 VARCHAR(64) NOT NULL DEFAULT '' CHECK (lease_token_sha256='' OR lease_token_sha256 ~ '^[0-9a-f]{64}$'),
    lease_expires_at TIMESTAMPTZ,
    execution_deadline_at TIMESTAMPTZ,
    launch_permit_id BIGINT,
    launch_permit_sha256 VARCHAR(64) NOT NULL DEFAULT '' CHECK (launch_permit_sha256='' OR launch_permit_sha256 ~ '^[0-9a-f]{64}$'),
    launch_permit_expires_at TIMESTAMPTZ,
    runtime_container_id TEXT NOT NULL DEFAULT '',
    runtime_container_name TEXT NOT NULL DEFAULT '',
    runtime_execution_identity_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK (runtime_execution_identity_digest='' OR runtime_execution_identity_digest ~ '^[0-9a-f]{64}$'),
    canonical_report_json JSONB,
    canonical_report_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK (canonical_report_digest='' OR canonical_report_digest ~ '^[0-9a-f]{64}$'),
    report_verdict VARCHAR(32) NOT NULL DEFAULT '' CHECK (report_verdict IN ('','PASS','REVISE','REJECT')),
    provider_usage_json JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(provider_usage_json)='object'),
    automatic_revision_number INTEGER NOT NULL DEFAULT 0,
    blocking_finding_count INTEGER NOT NULL DEFAULT 0,
    actionable_finding_digest TEXT NOT NULL DEFAULT '',
    deterministic_test_summary_digest TEXT NOT NULL DEFAULT '',
    deterministic_failure_count INTEGER NOT NULL DEFAULT 0,
    progress_signature TEXT NOT NULL DEFAULT '',
    no_progress_streak INTEGER NOT NULL DEFAULT 0,
    successor_task_turn_id BIGINT,
    closure_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    next_retry_at TIMESTAMPTZ,
    wait_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT workforce_review_runs_slice4_runtime_ck CHECK (
        (runtime_container_id='' AND runtime_container_name='' AND runtime_execution_identity_digest='')
        OR (runtime_container_id<>'' AND runtime_container_name<>'' AND runtime_execution_identity_digest ~ '^[0-9a-f]{64}$')
    ),
    CONSTRAINT workforce_review_runs_assignment_ck CHECK (
        ((runtime_release_id IS NULL AND reviewer_image_digest='' AND adapter_identity='' AND adapter_version='' AND adapter_protocol='' AND model_credential_id IS NULL AND credential_version IS NULL AND model_protocol='' AND primary_model='' AND provider_proxy_image_digest='' AND selection_digest='' AND worker_id IS NULL AND worker_instance_id='')
         OR (runtime_release_id>0 AND reviewer_image_digest ~ '^sha256:[0-9a-f]{64}$' AND adapter_identity<>'' AND adapter_version<>'' AND adapter_protocol<>'' AND model_credential_id>0 AND credential_version>0 AND model_protocol IN ('OPENAI','ANTHROPIC') AND primary_model<>'' AND provider_proxy_image_digest ~ '^sha256:[0-9a-f]{64}$' AND selection_digest ~ '^[0-9a-f]{64}$' AND worker_id>0 AND worker_instance_id<>''))
        AND (status NOT IN ('ASSIGNED','RUNNING','VALIDATING_REPORT') OR selection_digest<>'')
    ),
    CONSTRAINT workforce_review_runs_slice4_report_ck CHECK (
        (canonical_report_json IS NULL AND canonical_report_digest='' AND report_verdict='')
        OR (jsonb_typeof(canonical_report_json)='object' AND canonical_report_digest ~ '^[0-9a-f]{64}$' AND report_verdict IN ('PASS','REVISE','REJECT'))
    ),
    CONSTRAINT workforce_review_runs_slice5_progress_ck CHECK (
        automatic_revision_number>=0 AND blocking_finding_count>=0 AND deterministic_failure_count>=0 AND no_progress_streak>=0
        AND (actionable_finding_digest='' OR actionable_finding_digest ~ '^[0-9a-f]{64}$')
        AND (deterministic_test_summary_digest='' OR deterministic_test_summary_digest ~ '^[0-9a-f]{64}$')
        AND (progress_signature='' OR progress_signature ~ '^[0-9a-f]{64}$')
    ),
    UNIQUE (tenant_id,task_id,task_turn_id,finalization_run_id),
    UNIQUE (tenant_id,task_id,task_turn_id,finalization_run_id,result_commit,result_tree)
);

CREATE TABLE workforce_review_run_attempts (
    id BIGINT PRIMARY KEY,
    review_run_id BIGINT NOT NULL CHECK (review_run_id > 0),
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    runtime_release_id BIGINT NOT NULL CHECK (runtime_release_id > 0),
    reviewer_image_digest TEXT NOT NULL CHECK (reviewer_image_digest ~ '^sha256:[0-9a-f]{64}$'),
    adapter_identity TEXT NOT NULL CHECK (adapter_identity<>''),
    adapter_version TEXT NOT NULL CHECK (adapter_version<>''),
    adapter_protocol VARCHAR(64) NOT NULL CHECK (adapter_protocol<>''),
    adapter_capabilities_json JSONB NOT NULL CHECK (jsonb_typeof(adapter_capabilities_json)='array'),
    model_credential_id BIGINT NOT NULL CHECK (model_credential_id > 0),
    credential_version INTEGER NOT NULL CHECK (credential_version > 0),
    model_protocol VARCHAR(16) NOT NULL CHECK (model_protocol IN ('OPENAI','ANTHROPIC')),
    primary_model VARCHAR(256) NOT NULL CHECK (primary_model<>''),
    provider_proxy_image_digest TEXT NOT NULL CHECK (provider_proxy_image_digest ~ '^sha256:[0-9a-f]{64}$'),
    selection_digest VARCHAR(64) NOT NULL CHECK (selection_digest ~ '^[0-9a-f]{64}$'),
    worker_id BIGINT NOT NULL CHECK (worker_id > 0),
    worker_instance_id TEXT NOT NULL,
    host_boot_id TEXT NOT NULL DEFAULT '',
    owner_epoch INTEGER NOT NULL CHECK (owner_epoch > 0),
    status VARCHAR(32) NOT NULL CHECK (status IN ('ASSIGNED','RUNNING','VALIDATING_REPORT','COMPLETED','REJECTED','FAILED','EXPIRED','CANCELLED')),
    lease_token_sha256 VARCHAR(64) NOT NULL DEFAULT '' CHECK (lease_token_sha256='' OR lease_token_sha256 ~ '^[0-9a-f]{64}$'),
    lease_expires_at TIMESTAMPTZ,
    launch_permit_id BIGINT,
    launch_permit_sha256 VARCHAR(64) NOT NULL DEFAULT '' CHECK (launch_permit_sha256='' OR launch_permit_sha256 ~ '^[0-9a-f]{64}$'),
    launch_permit_expires_at TIMESTAMPTZ,
    container_id TEXT NOT NULL DEFAULT '',
    container_name TEXT NOT NULL DEFAULT '',
    image_digest TEXT NOT NULL DEFAULT '',
    execution_identity_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK (execution_identity_digest='' OR execution_identity_digest ~ '^[0-9a-f]{64}$'),
    error_code VARCHAR(128) NOT NULL DEFAULT '',
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    claimed_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT workforce_review_run_attempts_slice4_runtime_ck CHECK (
        (container_id='' AND container_name='' AND image_digest='' AND execution_identity_digest='')
        OR (container_id<>'' AND container_name<>'' AND image_digest ~ '^sha256:[0-9a-f]{64}$' AND execution_identity_digest ~ '^[0-9a-f]{64}$')
    ),
    UNIQUE (review_run_id,attempt_number)
);

CREATE TABLE workforce_human_review_decisions (
    id BIGINT PRIMARY KEY,
    tenant_id BIGINT NOT NULL CHECK (tenant_id > 0),
    task_id BIGINT NOT NULL CHECK (task_id > 0),
    task_turn_id BIGINT NOT NULL CHECK (task_turn_id > 0),
    review_run_id BIGINT NOT NULL CHECK (review_run_id > 0),
    action VARCHAR(32) NOT NULL CHECK (action IN ('ACCEPT_RISK','AUTHORIZE_ONE_REVISION','CANCEL_TASK')),
    idempotency_key TEXT NOT NULL CHECK (idempotency_key <> ''),
    request_digest VARCHAR(64) NOT NULL CHECK (request_digest ~ '^[0-9a-f]{64}$'),
    actor_principal_id BIGINT NOT NULL CHECK (actor_principal_id > 0),
    reason TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 2000),
    revision_guidance VARCHAR(4000),
    proof_identifier_sha256 VARCHAR(64) NOT NULL CHECK (proof_identifier_sha256 ~ '^[0-9a-f]{64}$'),
    challenge_identifier_sha256 VARCHAR(64) NOT NULL CHECK (challenge_identifier_sha256 ~ '^[0-9a-f]{64}$'),
    assurance_level VARCHAR(16) NOT NULL CHECK (assurance_level='AAL2'),
    authentication_methods_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(authentication_methods_json)='array'),
    ai_verdict VARCHAR(32) NOT NULL DEFAULT '' CHECK (ai_verdict IN ('','REVISE','REJECT')),
    canonical_report_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK (canonical_report_digest='' OR canonical_report_digest ~ '^[0-9a-f]{64}$'),
    status VARCHAR(32) NOT NULL DEFAULT 'APPLIED' CHECK (status='APPLIED'),
    resulting_task_turn_id BIGINT,
    quality_outcome VARCHAR(32) NOT NULL DEFAULT '' CHECK (quality_outcome IN ('','HUMAN_ACCEPTED')),
    decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id,review_run_id,idempotency_key),
    CONSTRAINT workforce_human_review_decisions_guidance_ck CHECK (
        (revision_guidance IS NULL OR octet_length(revision_guidance) BETWEEN 1 AND 4000)
        AND (action='AUTHORIZE_ONE_REVISION' OR revision_guidance IS NULL)
        AND (action<>'AUTHORIZE_ONE_REVISION' OR canonical_report_digest<>'' OR revision_guidance IS NOT NULL)
    )
);

CREATE INDEX workforce_review_runs_scheduler_idx ON workforce_review_runs (status,next_retry_at,created_at);
CREATE INDEX workforce_review_runs_worker_capacity_idx ON workforce_review_runs (worker_id,status,lease_expires_at);
CREATE INDEX workforce_review_runs_task_retention_idx ON workforce_review_runs (task_id,status,finalization_run_id);
CREATE INDEX workforce_review_runs_task_read_idx ON workforce_review_runs (tenant_id,task_id,id DESC);
CREATE INDEX workforce_review_run_attempts_run_owner_idx ON workforce_review_run_attempts (review_run_id,owner_epoch,status);
CREATE UNIQUE INDEX workforce_human_review_decisions_applied_run_uidx ON workforce_human_review_decisions (review_run_id) WHERE status='APPLIED';
CREATE INDEX workforce_human_review_decisions_task_read_idx ON workforce_human_review_decisions (tenant_id,task_id,id DESC);

ALTER TABLE workforce_review_runs ADD COLUMN IF NOT EXISTS completion_check_digest VARCHAR(64) NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS workforce_completion_check_cycle_authorizations (
 cycle_id BIGINT PRIMARY KEY CHECK(cycle_id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0),
 task_id BIGINT NOT NULL CHECK(task_id>0),task_turn_id BIGINT NOT NULL CHECK(task_turn_id>0),
 attempt_id BIGINT NOT NULL CHECK(attempt_id>0),cycle_number INTEGER NOT NULL CHECK(cycle_number>0),
 UNIQUE(attempt_id,cycle_number)
);
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
CREATE INDEX IF NOT EXISTS workforce_completion_check_results_tenant_task_created_idx ON workforce_completion_check_results(tenant_id,task_id,created_at DESC,id);
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
);

-- Batch3 persistent Task conversation and native interaction.
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

ALTER TABLE workforce_attempt_runtimes ADD COLUMN adapter_session_generation INTEGER CHECK(adapter_session_generation > 0);

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

-- Batch3 additive task-scoped completion check revisions.
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

-- Batch4 controlled network operations. Full descriptors and tool output are
-- encrypted; conversation projections contain only safe purpose/status data.
CREATE TABLE IF NOT EXISTS workforce_execution_operations (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 task_turn_id BIGINT NOT NULL CHECK(task_turn_id>0), attempt_id BIGINT NOT NULL CHECK(attempt_id>0),
 adapter_session_generation INTEGER NOT NULL CHECK(adapter_session_generation>0), authorization_grant_id BIGINT CHECK(authorization_grant_id>0),
 local_tool_call_key VARCHAR(128) NOT NULL CHECK(octet_length(local_tool_call_key) BETWEEN 1 AND 128),
 native_tool_call_id TEXT NOT NULL CONSTRAINT workforce_execution_operations_native_tool_call_id_ck CHECK(octet_length(native_tool_call_id) BETWEEN 1 AND 512 AND native_tool_call_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$'),
 round_key VARCHAR(128) NOT NULL CHECK(octet_length(round_key) BETWEEN 1 AND 128), operation_kind VARCHAR(32) NOT NULL CHECK(operation_kind='NETWORK_COMMAND'),
 descriptor_ciphertext BYTEA NOT NULL, descriptor_edek BYTEA NOT NULL, descriptor_key_ref VARCHAR(256) NOT NULL,
 descriptor_digest VARCHAR(64) NOT NULL CHECK(descriptor_digest ~ '^[0-9a-f]{64}$'),
 purpose VARCHAR(2048) NOT NULL CHECK(octet_length(purpose) BETWEEN 1 AND 2048),
 targets_json JSONB NOT NULL CHECK(jsonb_typeof(targets_json)='array' AND jsonb_array_length(targets_json) BETWEEN 1 AND 8),
 status VARCHAR(32) NOT NULL CHECK(status IN ('WAITING_APPROVAL','READY','DISPATCHED','RUNNING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN','DENIED','EXPIRED','CANCELLED')),
 version INTEGER NOT NULL CHECK(version>0), result_delivery_status VARCHAR(32) NOT NULL DEFAULT '' CHECK(result_delivery_status IN ('','SAVED','DELIVERED_TO_ADAPTER','NATIVE_CALL_RESOLVED')),
 agent_continued BOOLEAN NOT NULL DEFAULT FALSE, result_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK(result_digest='' OR result_digest ~ '^[0-9a-f]{64}$'),
 result_summary TEXT NOT NULL DEFAULT '' CHECK(octet_length(result_summary)<=2048), exit_code INTEGER,
 result_output_ciphertext BYTEA, result_output_edek BYTEA, result_output_key_ref VARCHAR(256),
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ,
 UNIQUE(attempt_id,adapter_session_generation,local_tool_call_key), CHECK(expires_at>created_at),
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
 request_digest VARCHAR(64) NOT NULL CHECK(request_digest ~ '^[0-9a-f]{64}$'), reason VARCHAR(2000) NOT NULL DEFAULT '' CHECK(octet_length(reason)<=2000),
 status VARCHAR(16) NOT NULL CHECK(status='APPLIED'), created_at TIMESTAMPTZ NOT NULL,
 UNIQUE(tenant_id,task_id,actor_principal_id,idempotency_key)
);

CREATE TABLE IF NOT EXISTS workforce_task_network_grants (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 source_decision_id BIGINT NOT NULL UNIQUE CHECK(source_decision_id>0), actor_principal_id BIGINT NOT NULL CHECK(actor_principal_id>0),
 targets_json JSONB NOT NULL CHECK(jsonb_typeof(targets_json)='array' AND jsonb_array_length(targets_json) BETWEEN 1 AND 8),
 targets_digest VARCHAR(64) NOT NULL CHECK(targets_digest ~ '^[0-9a-f]{64}$'), status VARCHAR(16) NOT NULL CHECK(status IN ('ACTIVE','REVOKED','EXPIRED')),
 version INTEGER NOT NULL CHECK(version>0), revoke_idempotency_key VARCHAR(128), revoke_request_digest VARCHAR(64),
 expires_at TIMESTAMPTZ NOT NULL, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ,
 CHECK(expires_at>created_at), CHECK((revoke_idempotency_key IS NULL AND revoke_request_digest IS NULL) OR (octet_length(revoke_idempotency_key) BETWEEN 1 AND 128 AND revoke_request_digest ~ '^[0-9a-f]{64}$'))
);
CREATE INDEX IF NOT EXISTS workforce_task_network_grant_task_idx ON workforce_task_network_grants(tenant_id,task_id,status,id DESC);

CREATE TABLE IF NOT EXISTS workforce_execution_operation_attempts (
 id BIGINT PRIMARY KEY CHECK(id>0), tenant_id BIGINT NOT NULL CHECK(tenant_id>0), task_id BIGINT NOT NULL CHECK(task_id>0),
 operation_id BIGINT NOT NULL CHECK(operation_id>0), attempt_number INTEGER NOT NULL CHECK(attempt_number>0),
 worker_id BIGINT NOT NULL CHECK(worker_id>0), worker_instance_id VARCHAR(128) NOT NULL, host_boot_id VARCHAR(128) NOT NULL,
 owner_epoch INTEGER NOT NULL CHECK(owner_epoch>0), lease_digest VARCHAR(64) NOT NULL CHECK(lease_digest ~ '^[0-9a-f]{64}$'), lease_expires_at TIMESTAMPTZ NOT NULL,
 status VARCHAR(32) NOT NULL CHECK(status IN ('DISPATCHED','RUNNING','SUCCEEDED','FAILED','OUTCOME_UNKNOWN')),
 container_id TEXT NOT NULL DEFAULT '', relay_identity TEXT NOT NULL DEFAULT '', result_digest VARCHAR(64) NOT NULL DEFAULT '' CHECK(result_digest='' OR result_digest ~ '^[0-9a-f]{64}$'),
 exit_code INTEGER, created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, started_at TIMESTAMPTZ, finished_at TIMESTAMPTZ,
 UNIQUE(operation_id,attempt_number)
);
CREATE UNIQUE INDEX IF NOT EXISTS workforce_execution_operation_one_active_attempt_idx ON workforce_execution_operation_attempts(operation_id) WHERE status IN ('DISPATCHED','RUNNING');

-- A cold Operation resume creates at most one successor Turn. Relationship
-- integrity is enforced by Workforce governance rather than database FKs.
ALTER TABLE workforce_task_turns DROP CONSTRAINT IF EXISTS workforce_task_turns_source_execution_operation_ck;
ALTER TABLE workforce_task_turns ADD CONSTRAINT workforce_task_turns_source_execution_operation_ck CHECK (
 source_execution_operation_id IS NULL OR (source_execution_operation_id>0 AND source_checkpoint_id IS NOT NULL)
);

COMMENT ON TABLE workforce_workers IS 'Worker 配置';
COMMENT ON COLUMN workforce_workers.max_active_attempts IS 'Server 允许该 Worker 同时执行的最大任务数';
COMMENT ON TABLE workforce_worker_tokens IS 'Worker 登录凭据';
COMMENT ON TABLE workforce_worker_sessions IS 'Worker 在线会话';
COMMENT ON COLUMN workforce_worker_sessions.end_reason IS '会话结束原因';
COMMENT ON COLUMN workforce_worker_sessions.max_active_attempts IS 'Worker 当前进程上报的最大任务能力';
COMMENT ON TABLE workforce_worker_metric_samples IS 'Worker 每分钟资源与任务负载样本';
COMMENT ON COLUMN workforce_worker_metric_samples.worker_id IS 'Worker 标识';
COMMENT ON COLUMN workforce_worker_metric_samples.sampled_at IS 'Server 记录的采样分钟';
COMMENT ON COLUMN workforce_worker_metric_samples.logical_cpu_cores IS '逻辑 CPU 核数';
COMMENT ON COLUMN workforce_worker_metric_samples.cpu_usage_percent IS 'CPU 使用率，无法采集时为空';
COMMENT ON COLUMN workforce_worker_metric_samples.load1 IS '一分钟系统负载';
COMMENT ON COLUMN workforce_worker_metric_samples.memory_used_bytes IS '已使用内存字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.memory_total_bytes IS '内存总字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.memory_usage_percent IS '内存使用率';
COMMENT ON COLUMN workforce_worker_metric_samples.workspace_disk_used_bytes IS '工作区磁盘已使用字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.workspace_disk_total_bytes IS '工作区磁盘总字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.workspace_disk_usage_percent IS '工作区磁盘使用率';
COMMENT ON COLUMN workforce_worker_metric_samples.active_attempts IS '采样时正在占用的任务槽位';
COMMENT ON COLUMN workforce_worker_metric_samples.max_active_attempts IS '采样时可用的任务槽位总数';
COMMENT ON COLUMN workforce_worker_metric_samples.docker_ready IS '采样时 Docker 是否可用';
COMMENT ON TABLE workforce_worker_images IS 'Worker 镜像库存';
COMMENT ON COLUMN workforce_worker_images.scheduling_scope IS '镜像调度范围';
COMMENT ON TABLE workforce_execution_nodes IS '待调度执行节点';
COMMENT ON TABLE workforce_task_assignments IS '任务调度结果';
COMMENT ON COLUMN workforce_task_assignments.adapter_protocol IS 'Agent 适配协议';
COMMENT ON COLUMN workforce_task_assignments.isolation_profile IS '运行隔离级别';
COMMENT ON TABLE workforce_attempt_runtimes IS '任务运行实例';
COMMENT ON COLUMN workforce_attempt_runtimes.provider_proxy_control_state IS '模型代理控制状态';
COMMENT ON COLUMN workforce_attempt_runtimes.recovery_previous_status IS '恢复前运行状态';
COMMENT ON TABLE workforce_runtime_reconciliation_runs IS '运行实例恢复记录';
COMMENT ON TABLE workforce_finalization_runs IS '任务结果冻结记录';
COMMENT ON TABLE workforce_artifact_upload_attempts IS '产物上传尝试';
COMMENT ON COLUMN workforce_artifact_upload_attempts.artifact_type IS '产物类型';
COMMENT ON COLUMN workforce_artifact_upload_attempts.media_type IS '产物媒体类型';
COMMENT ON TABLE workforce_machine_idempotency_records IS 'Worker 幂等记录';
COMMENT ON COLUMN workforce_machine_idempotency_records.http_method IS 'HTTP 请求方法';
COMMENT ON COLUMN workforce_machine_idempotency_records.resource_type IS '业务资源类型';
COMMENT ON TABLE workforce_attempt_facts IS '执行尝试事实';
COMMENT ON TABLE workforce_result_processors IS '结果处理器';
COMMENT ON COLUMN workforce_result_processors.category IS '处理器分类';
COMMENT ON TABLE workforce_result_processor_revisions IS '结果处理器草稿版本';
COMMENT ON TABLE workforce_result_processor_releases IS '结果处理器发布版本';
COMMENT ON TABLE workforce_result_processor_bindings IS '结果处理器绑定';
COMMENT ON TABLE workforce_result_processor_environment_bindings IS '结果处理器环境配置';
COMMENT ON TABLE workforce_result_processor_fixture_runs IS '结果处理器测试记录';
COMMENT ON COLUMN workforce_result_processor_fixture_runs.expected_decision IS '预期处理结果';
COMMENT ON TABLE workforce_task_turns IS '任务执行轮次';
COMMENT ON COLUMN workforce_task_turns.review_revision_kind IS '审查修订方式';
COMMENT ON TABLE workforce_result_evaluation_cycles IS '结果评估轮次';
COMMENT ON TABLE workforce_continuation_checkpoints IS '任务接续检查点';
COMMENT ON TABLE workforce_continuation_restore_runs IS '任务接续恢复记录';
COMMENT ON TABLE workforce_result_materialization_runs IS '结果生成记录';
COMMENT ON COLUMN workforce_result_materialization_runs.artifact_media_type IS '结果媒体类型';
COMMENT ON TABLE workforce_result_outputs IS '任务结果输出';
COMMENT ON COLUMN workforce_result_outputs.output_kind IS '结果输出类型';
COMMENT ON COLUMN workforce_result_outputs.archive_format IS '结果归档格式';
COMMENT ON COLUMN workforce_result_outputs.storage IS '结果存储方式';
COMMENT ON TABLE workforce_workspace_sources IS '项目代码来源';
COMMENT ON TABLE workforce_workspace_source_revisions IS '冻结代码版本';
COMMENT ON TABLE workforce_source_ingestion_runs IS '代码准备记录';
COMMENT ON COLUMN workforce_source_ingestion_runs.requested_ref_type IS '请求的 Git 引用类型';
COMMENT ON TABLE workforce_reference_assets IS '任务参考资料';
COMMENT ON COLUMN workforce_reference_assets.content_type IS '资料媒体类型';
COMMENT ON TABLE workforce_projects IS '项目';
COMMENT ON TABLE workforce_project_versions IS '项目配置版本';
COMMENT ON TABLE workforce_task_input_snapshots IS '任务输入快照';
COMMENT ON COLUMN workforce_task_input_snapshots.source_kind IS '任务代码来源';
COMMENT ON TABLE workforce_task_input_references IS '任务引用资料';
COMMENT ON TABLE workforce_task_git_plans IS '任务 Git 交付计划';
COMMENT ON TABLE workforce_delivery_operations IS 'Git 交付步骤';
COMMENT ON TABLE workforce_pull_request_publications IS '评审请求发布记录';
COMMENT ON COLUMN workforce_pull_request_publications.provider_type IS 'Git 服务类型';
COMMENT ON TABLE workforce_delivery_requests IS 'Git 交付请求';
COMMENT ON COLUMN workforce_delivery_requests.action_type IS '交付动作类型';
COMMENT ON COLUMN workforce_delivery_requests.approval_mode IS '交付批准方式';
COMMENT ON TABLE workforce_delivery_intents IS 'Agent 交付意图';
COMMENT ON TABLE workforce_delivery_decisions IS 'Git 交付决定';
COMMENT ON COLUMN workforce_delivery_decisions.action IS '交付决定';
COMMENT ON TABLE workforce_delivery_attempts IS 'Git 交付尝试';
COMMENT ON TABLE workforce_task_notification_subscriptions IS '任务通知订阅';
COMMENT ON TABLE workforce_task_notification_subscription_channels IS '任务通知渠道';
COMMENT ON COLUMN workforce_task_notification_subscription_channels.channel_kind IS '通知渠道类型';
COMMENT ON TABLE workforce_task_notification_records IS '任务通知记录';
COMMENT ON COLUMN workforce_task_notification_records.source_type IS '通知事实来源';
COMMENT ON COLUMN workforce_task_notification_records.fact_type IS '通知事实类型';
COMMENT ON COLUMN workforce_task_notification_records.focus IS '通知关注区域';
COMMENT ON COLUMN workforce_task_notification_records.warning_code IS '通知警告码';
COMMENT ON COLUMN workforce_task_notification_records.last_error_code IS '最近错误码';
COMMENT ON TABLE workforce_task_notification_record_recipients IS '任务通知接收人';
COMMENT ON COLUMN workforce_task_notification_record_recipients.recipient_role IS '接收人角色';
COMMENT ON COLUMN workforce_task_notification_record_recipients.personal_channel_policy IS '个人渠道策略';
COMMENT ON TABLE workforce_task_notification_record_channels IS '任务通知发送渠道';
COMMENT ON COLUMN workforce_task_notification_record_channels.channel_kind IS '发送渠道类型';
COMMENT ON TABLE workforce_review_runs IS '独立审查';
COMMENT ON COLUMN workforce_review_runs.adapter_protocol IS '审查适配协议';
COMMENT ON COLUMN workforce_review_runs.report_verdict IS '审查结论';
COMMENT ON COLUMN workforce_review_runs.closure_reason_code IS '审查结束原因码';
COMMENT ON TABLE workforce_review_run_attempts IS '独立审查执行尝试';
COMMENT ON COLUMN workforce_review_run_attempts.adapter_protocol IS '审查适配协议';
COMMENT ON TABLE workforce_human_review_decisions IS '人工审查决定';
COMMENT ON COLUMN workforce_human_review_decisions.action IS '人工处理动作';
COMMENT ON COLUMN workforce_human_review_decisions.assurance_level IS '身份验证级别';
COMMENT ON COLUMN workforce_human_review_decisions.ai_verdict IS 'AI 审查结论';
COMMENT ON COLUMN workforce_human_review_decisions.quality_outcome IS '最终质量结论';
COMMENT ON TABLE workforce_completion_check_cycle_authorizations IS '完成检查轮次授权';
COMMENT ON TABLE workforce_completion_check_results IS '完成检查结果';
COMMENT ON TABLE workforce_completion_check_decisions IS '完成检查决定';
COMMENT ON TABLE workforce_agent_interactions IS 'Agent 交互请求';
COMMENT ON TABLE workforce_agent_interaction_responses IS 'Agent 交互回答';
COMMENT ON TABLE workforce_task_conversation_items IS '任务对话条目';
COMMENT ON TABLE workforce_task_conversation_read_states IS '任务对话已读位置';
COMMENT ON TABLE workforce_completion_check_revisions IS '完成检查修订';
COMMENT ON TABLE workforce_execution_operations IS '受控网络操作';
COMMENT ON TABLE workforce_execution_operation_decisions IS '受控网络操作决定';
COMMENT ON TABLE workforce_task_network_grants IS '任务网络授权';
COMMENT ON TABLE workforce_execution_operation_attempts IS '受控网络操作尝试';
COMMENT ON COLUMN workforce_project_versions.repository_credential_ciphertext IS '项目 Git 凭据密文';
COMMENT ON COLUMN workforce_project_versions.repository_credential_edek IS '项目 Git 凭据的数据密钥';
COMMENT ON COLUMN workforce_task_git_plans.repository_credential_ciphertext IS '任务冻结的 Git 凭据密文';
COMMENT ON COLUMN workforce_task_git_plans.repository_credential_edek IS '任务冻结的 Git 凭据数据密钥';
COMMENT ON COLUMN workforce_execution_operations.descriptor_ciphertext IS '网络操作参数密文';
COMMENT ON COLUMN workforce_execution_operations.descriptor_edek IS '网络操作参数的数据密钥';
COMMENT ON COLUMN workforce_execution_operations.result_output_ciphertext IS '网络操作结果密文';
COMMENT ON COLUMN workforce_execution_operations.result_output_edek IS '网络操作结果的数据密钥';
COMMENT ON COLUMN workforce_attempt_runtimes.owner_epoch IS '运行实例所有权代次';
COMMENT ON COLUMN workforce_review_runs.owner_epoch IS '审查所有权代次';
COMMENT ON COLUMN workforce_review_run_attempts.owner_epoch IS '审查尝试所有权代次';

-- +goose Down
DROP INDEX IF EXISTS workforce_turn_execution_operation_once_idx;
ALTER TABLE workforce_task_turns
 DROP CONSTRAINT IF EXISTS workforce_task_turns_source_execution_operation_ck,
 DROP COLUMN IF EXISTS source_execution_operation_id;
DROP TABLE IF EXISTS workforce_execution_operation_attempts;
DROP TABLE IF EXISTS workforce_task_network_grants;
DROP TABLE IF EXISTS workforce_execution_operation_decisions;
DROP TABLE IF EXISTS workforce_execution_operations;
DROP TABLE IF EXISTS workforce_completion_check_revisions;
DROP TABLE IF EXISTS workforce_task_conversation_read_states;
DROP TABLE IF EXISTS workforce_task_conversation_items;
DROP TABLE IF EXISTS workforce_agent_interaction_responses;
DROP TABLE IF EXISTS workforce_agent_interactions;
DROP TABLE IF EXISTS workforce_completion_check_cycle_authorizations;
DROP TABLE IF EXISTS workforce_completion_check_decisions;
DROP TABLE IF EXISTS workforce_completion_check_results;

DELETE FROM sys_role_menu
 WHERE "menuId" IN (SELECT id FROM sys_menu WHERE path IN ('/workforce/review','/workforce/review/profiles','/workforce/review/rubrics'));
DELETE FROM sys_menu_permission
 WHERE "menuId" IN (SELECT id FROM sys_menu WHERE path IN ('/workforce/review/profiles','/workforce/review/rubrics'));
DELETE FROM sys_menu
 WHERE path IN ('/workforce/review','/workforce/review/profiles','/workforce/review/rubrics');

DROP INDEX IF EXISTS workforce_human_review_decisions_task_read_idx;
DROP INDEX IF EXISTS workforce_human_review_decisions_applied_run_uidx;
DROP TABLE IF EXISTS workforce_human_review_decisions;
DROP INDEX IF EXISTS workforce_review_run_attempts_run_owner_idx;
DROP INDEX IF EXISTS workforce_review_runs_task_read_idx;
DROP INDEX IF EXISTS workforce_review_runs_task_retention_idx;
DROP INDEX IF EXISTS workforce_review_runs_worker_capacity_idx;
DROP INDEX IF EXISTS workforce_review_runs_scheduler_idx;
DROP TABLE IF EXISTS workforce_review_run_attempts;
DROP TABLE IF EXISTS workforce_review_runs;
DROP INDEX IF EXISTS workforce_task_turns_source_review_run_uidx;
ALTER TABLE workforce_task_turns
    DROP COLUMN IF EXISTS revision_trigger_principal_id,
    DROP COLUMN IF EXISTS review_report_digest,
    DROP COLUMN IF EXISTS source_review_run_id,
    DROP COLUMN IF EXISTS review_revision_kind;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS quality_outcome;
ALTER TABLE workforce_workers
    DROP COLUMN IF EXISTS max_total_containers,
    DROP COLUMN IF EXISTS max_active_reviews;

DROP INDEX IF EXISTS workforce_task_notification_records_dispatch_idx;
DROP INDEX IF EXISTS workforce_task_notification_records_task_idx;
DROP INDEX IF EXISTS workforce_task_notification_subscriptions_lookup_idx;
DROP TABLE IF EXISTS workforce_task_notification_record_channels;
DROP TABLE IF EXISTS workforce_task_notification_record_recipients;
DROP TABLE IF EXISTS workforce_task_notification_records;
DROP TABLE IF EXISTS workforce_task_notification_subscription_channels;
DROP TABLE IF EXISTS workforce_task_notification_subscriptions;
DROP INDEX IF EXISTS workforce_delivery_attempt_retry_idx;
DROP INDEX IF EXISTS workforce_delivery_succeeded_attempt_idx;
DROP INDEX IF EXISTS workforce_delivery_active_destination_idx;
DROP INDEX IF EXISTS workforce_delivery_requests_dispatch_idx;
DROP INDEX IF EXISTS workforce_delivery_requests_task_idx;
DROP INDEX IF EXISTS workforce_delivery_requests_tenant_project_created_idx;
DROP INDEX IF EXISTS workforce_completion_check_results_tenant_task_created_idx;
DROP TABLE IF EXISTS workforce_delivery_attempts;
DROP TABLE IF EXISTS workforce_delivery_decisions;
DROP TABLE IF EXISTS workforce_delivery_intents;
DROP TABLE IF EXISTS workforce_delivery_requests;
DROP TABLE IF EXISTS workforce_pull_request_publications;
DROP TABLE IF EXISTS workforce_delivery_operations;
DROP TABLE IF EXISTS workforce_task_git_plans;
DROP TABLE IF EXISTS workforce_task_input_references;
DROP TABLE IF EXISTS workforce_task_input_snapshots;
DROP TABLE IF EXISTS workforce_reference_assets;
DROP TABLE IF EXISTS workforce_source_ingestion_runs;
DROP TABLE IF EXISTS workforce_workspace_source_revisions;
DROP TABLE IF EXISTS workforce_workspace_sources;
DROP INDEX IF EXISTS workforce_running_evaluation_attempt_idx;
DROP INDEX IF EXISTS workforce_result_outputs_task_idx;
DROP INDEX IF EXISTS workforce_checkpoint_expiry_idx;
DROP INDEX IF EXISTS workforce_evaluation_cycles_attempt_idx;
DROP INDEX IF EXISTS workforce_task_turns_task_idx;
DROP INDEX IF EXISTS workforce_task_turns_active_idx;
DROP INDEX IF EXISTS workforce_task_turns_idempotency_idx;
DROP INDEX IF EXISTS workforce_tasks_tags_gin_idx;
DROP INDEX IF EXISTS workforce_tasks_creator_created_idx;
DROP INDEX IF EXISTS workforce_tasks_tenant_created_idx;
DROP INDEX IF EXISTS workforce_tasks_project_created_idx;
DROP INDEX IF EXISTS workforce_tasks_tenant_project_finished_idx;
DROP INDEX IF EXISTS workforce_project_versions_repository_idx;
DROP INDEX IF EXISTS workforce_project_versions_department_idx;
DROP INDEX IF EXISTS workforce_project_versions_project_idx;
DROP INDEX IF EXISTS workforce_projects_tenant_status_idx;
DROP INDEX IF EXISTS workforce_checkpoint_consumed_once_idx;
DROP INDEX IF EXISTS workforce_restore_active_turn_idx;
DROP INDEX IF EXISTS workforce_materialization_identity_idx;
DROP INDEX IF EXISTS workforce_result_processor_fixture_dispatch_idx;
DROP INDEX IF EXISTS workforce_result_processor_releases_lookup_idx;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS task_turn_id;
ALTER TABLE workforce_execution_nodes DROP COLUMN IF EXISTS task_turn_id;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS result_processor_parameters_json;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS execution_requirements_digest;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS execution_requirements_json;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS review_enabled;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS primary_model;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS selected_runtime_release_id;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS execution_timeout_seconds;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS execution_active_milliseconds;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS model_credential_id;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS project_version_id;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS project_id;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS metadata_version;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS tags_json;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS name;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS current_turn_number;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS current_task_turn_id;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS result_processor_release_id;
DROP TRIGGER IF EXISTS workforce_project_versions_immutable_trg ON workforce_project_versions;
DROP FUNCTION IF EXISTS workforce_project_version_immutable();
DROP TABLE IF EXISTS workforce_project_versions;
DROP TABLE IF EXISTS workforce_projects;
DROP TABLE IF EXISTS workforce_result_outputs;
DROP TABLE IF EXISTS workforce_result_materialization_runs;
DROP TABLE IF EXISTS workforce_continuation_restore_runs;
DROP TABLE IF EXISTS workforce_continuation_checkpoints;
DROP TABLE IF EXISTS workforce_result_evaluation_cycles;
DROP TABLE IF EXISTS workforce_task_turns;
DROP TABLE IF EXISTS workforce_result_processor_fixture_runs;
DROP TABLE IF EXISTS workforce_result_processor_environment_bindings;
DROP TABLE IF EXISTS workforce_result_processor_bindings;
DROP TABLE IF EXISTS workforce_result_processor_releases;
DROP TABLE IF EXISTS workforce_result_processor_revisions;
DROP TABLE IF EXISTS workforce_result_processors;
DROP INDEX IF EXISTS workforce_assignments_worker_status_idx;
DROP INDEX IF EXISTS workforce_machine_idempotency_expiry_idx;
DROP INDEX IF EXISTS workforce_reconciliation_deadline_idx;
DROP INDEX IF EXISTS workforce_upload_retry_idx;
DROP INDEX IF EXISTS workforce_finalization_retry_idx;
DROP INDEX IF EXISTS workforce_verified_upload_identity_idx;
DROP INDEX IF EXISTS workforce_finalization_identity_idx;
DROP INDEX IF EXISTS workforce_active_finalization_attempt_idx;
DROP INDEX IF EXISTS workforce_active_reconciliation_attempt_idx;
DROP INDEX IF EXISTS workforce_tasks_fair_dispatch_idx;
DROP INDEX IF EXISTS workforce_images_presence_idx;
DROP INDEX IF EXISTS workforce_runtime_capacity_idx;
DROP INDEX IF EXISTS workforce_assignments_capacity_idx;
DROP INDEX IF EXISTS workforce_completion_attempt_once_idx;
DROP INDEX IF EXISTS workforce_execution_nodes_dispatch_idx;
DROP INDEX IF EXISTS workforce_worker_images_match_idx;
DROP INDEX IF EXISTS workforce_worker_sessions_fresh_idx;
DROP INDEX IF EXISTS workforce_active_runtime_node_idx;
DROP INDEX IF EXISTS workforce_active_assignment_node_idx;
ALTER TABLE workforce_outbox_events DROP COLUMN IF EXISTS message_expiration_ms;
ALTER TABLE workforce_outbox_events DROP COLUMN IF EXISTS routing_key;
ALTER TABLE workforce_outbox_events DROP COLUMN IF EXISTS exchange_name;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS owner_epoch;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS resolved_execution_snapshot_json;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS assignment_selection_digest;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS timeout_stage;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS timed_out_at;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS lost_at;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS execution_completed_at;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS retry_of_attempt_id;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS retry_class;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS cancel_reason;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS cancel_requested_at;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS cancel_requested_by_principal_id;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS cancel_generation;
ALTER TABLE workforce_tasks DROP COLUMN IF EXISTS cancel_request_id;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS host_boot_id;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS worker_instance_id;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS execution_node_id;
ALTER TABLE workforce_task_attempts DROP COLUMN IF EXISTS assignment_id;
DROP TABLE IF EXISTS workforce_attempt_facts;
DROP TABLE IF EXISTS workforce_machine_idempotency_records;
DROP TABLE IF EXISTS workforce_artifact_upload_attempts;
DROP TABLE IF EXISTS workforce_finalization_runs;
DROP TABLE IF EXISTS workforce_runtime_reconciliation_runs;
DROP TABLE IF EXISTS workforce_attempt_runtimes;
DROP TABLE IF EXISTS workforce_task_assignments;
DROP TABLE IF EXISTS workforce_execution_nodes;
DROP TABLE IF EXISTS workforce_worker_images;
DROP TABLE IF EXISTS workforce_worker_metric_samples;
DROP TABLE IF EXISTS workforce_worker_sessions;
DROP TABLE IF EXISTS workforce_worker_tokens;
DROP TABLE IF EXISTS workforce_workers;
DROP FUNCTION IF EXISTS workforce_assert_project_hard_cut_ready();
