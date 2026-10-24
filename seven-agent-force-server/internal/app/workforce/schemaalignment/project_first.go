package schemaalignment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

var retiredHookProductTables = []string{
	"workforce_integrated_verification_runs",
	"workforce_golden_verification_runs",
	"workforce_hook_bindings",
	"workforce_hook_execution_profiles",
	"workforce_hook_bundle_releases",
	"workforce_hook_bundles",
}

// ensureProjectHookHardCut converges development databases created before
// Projects selected immutable Hook releases directly. Populated retired Hook
// product tables are deliberately rejected: their Bundle/Binding identities
// cannot be assigned to a ProjectVersion without guessing user intent.
func ensureProjectHookHardCut(ctx context.Context, tx pgx.Tx) error {
	var projectVersionsPresent bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.workforce_project_versions') IS NOT NULL`).Scan(&projectVersionsPresent); err != nil {
		return fmt.Errorf("inspect Project Hook schema: %w", err)
	}
	if !projectVersionsPresent {
		return nil
	}

	for _, table := range retiredHookProductTables {
		var present bool
		if err := tx.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, table).Scan(&present); err != nil {
			return fmt.Errorf("inspect retired Hook table %s: %w", table, err)
		}
		if !present {
			continue
		}
		var populated bool
		query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM %s LIMIT 1)`, pgx.Identifier{table}.Sanitize())
		if err := tx.QueryRow(ctx, query).Scan(&populated); err != nil {
			return fmt.Errorf("inspect retired Hook table %s contents: %w", table, err)
		}
		if populated {
			return fmt.Errorf("PROJECT_HOOK_HARD_CUT_RESET_REQUIRED: retired Hook table %s contains business data", table)
		}
	}

	for _, table := range retiredHookProductTables {
		query := fmt.Sprintf(`DROP TABLE IF EXISTS %s`, pgx.Identifier{table}.Sanitize())
		if _, err := tx.Exec(ctx, query); err != nil {
			return fmt.Errorf("drop retired Hook table %s: %w", table, err)
		}
	}

	statements := []string{
		`ALTER TABLE workforce_project_versions ADD COLUMN IF NOT EXISTS hook_release_ids BIGINT[] NOT NULL DEFAULT '{}'`,
		`ALTER TABLE workforce_project_versions DROP COLUMN IF EXISTS runtime_requirements_json`,
		`ALTER TABLE workforce_tasks ADD COLUMN IF NOT EXISTS selected_runtime_release_id BIGINT`,
		`COMMENT ON COLUMN workforce_tasks.selected_runtime_release_id IS '任务选择的执行镜像版本'`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='workforce_tasks' AND column_name='execution_timeout_seconds') THEN ALTER TABLE workforce_tasks ADD COLUMN execution_timeout_seconds INTEGER; UPDATE workforce_tasks SET execution_timeout_seconds=600; UPDATE workforce_tasks t SET execution_timeout_seconds=source.timeout_seconds FROM (SELECT DISTINCT ON (a.task_id) a.task_id,(a.resolved_execution_snapshot_json->>'executionDeadlineSeconds')::INTEGER AS timeout_seconds FROM workforce_task_assignments a WHERE a.resolved_execution_snapshot_json ? 'executionDeadlineSeconds' ORDER BY a.task_id,a.created_at,a.id) source WHERE source.task_id=t.id AND source.timeout_seconds>0; END IF; END $$`,
		`COMMENT ON COLUMN workforce_tasks.execution_timeout_seconds IS '任务设置的 Agent 执行时限，秒'`,
		`ALTER TABLE workforce_tasks ADD COLUMN IF NOT EXISTS execution_active_milliseconds BIGINT NOT NULL DEFAULT 0`,
		`COMMENT ON COLUMN workforce_tasks.execution_active_milliseconds IS '已确认的 Agent 活跃执行时间，毫秒'`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='workforce_tasks_execution_active_milliseconds_ck' AND conrelid='workforce_tasks'::regclass) THEN ALTER TABLE workforce_tasks ADD CONSTRAINT workforce_tasks_execution_active_milliseconds_ck CHECK (execution_active_milliseconds>=0); END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='workforce_tasks_execution_timeout_seconds_ck' AND conrelid='workforce_tasks'::regclass) THEN ALTER TABLE workforce_tasks ADD CONSTRAINT workforce_tasks_execution_timeout_seconds_ck CHECK (execution_timeout_seconds IS NULL OR execution_timeout_seconds>0); END IF; END $$`,
		`ALTER TABLE workforce_harness_releases ADD COLUMN IF NOT EXISTS provider_protocols_json JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE workforce_harness_releases ADD COLUMN IF NOT EXISTS hook_events_json JSONB NOT NULL DEFAULT '[]'::jsonb`,
		`ALTER TABLE workforce_harness_releases ADD COLUMN IF NOT EXISTS manifest_json JSONB NOT NULL DEFAULT '{}'::jsonb`,
		`UPDATE workforce_harness_releases h SET provider_protocols_json=COALESCE(wi.manifest_json->'scheduling'->'providerProtocols','[]'::jsonb),hook_events_json=COALESCE(wi.manifest_json->'scheduling'->'hookEvents','[]'::jsonb),manifest_json=wi.manifest_json FROM (SELECT DISTINCT ON (image_digest) image_digest,manifest_json FROM workforce_worker_images WHERE role='attempt' AND admission_status='ADMITTED' ORDER BY image_digest,last_seen_at DESC) wi WHERE wi.image_digest=h.image_digest AND (h.provider_protocols_json='[]'::jsonb OR h.manifest_json='{}'::jsonb)`,
		`UPDATE workforce_tasks t SET selected_runtime_release_id=source.agent_runtime_release_id FROM (SELECT DISTINCT ON (a.task_id) a.task_id,a.agent_runtime_release_id FROM workforce_task_assignments a ORDER BY a.task_id,a.created_at,a.id) source WHERE source.task_id=t.id AND t.selected_runtime_release_id IS NULL`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("align direct Project Hook schema: %w", err)
		}
	}

	var activeWithoutRuntime int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_tasks WHERE selected_runtime_release_id IS NULL AND status NOT IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT')`).Scan(&activeWithoutRuntime); err != nil {
		return fmt.Errorf("inspect Task runtime selection: %w", err)
	}
	if activeWithoutRuntime != 0 {
		return errors.New("TASK_RUNTIME_SELECTION_HARD_CUT_ACTIVE_EXECUTION")
	}

	statements = []string{
		`ALTER TABLE workforce_task_assignments ADD COLUMN IF NOT EXISTS hook_configuration_digest VARCHAR(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE workforce_task_assignments DROP COLUMN IF EXISTS managed_hook_release_id`,
		`ALTER TABLE workforce_attempt_runtimes ADD COLUMN IF NOT EXISTS hook_configuration_digest TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE workforce_attempt_runtimes DROP COLUMN IF EXISTS hook_bundle_digest`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("align direct Project Hook schema: %w", err)
		}
	}

	retireRBACStatements := []string{
		`DELETE FROM sys_role_permission WHERE "permissionId" IN (SELECT id FROM sys_permission WHERE code LIKE 'workforce:hook-bundle:%')`,
		`UPDATE sys_permission SET "isDeleted"=true,"updateTime"=CURRENT_TIMESTAMP WHERE code LIKE 'workforce:hook-bundle:%' AND "isDeleted"=false`,
		`DELETE FROM sys_role_menu WHERE "menuId" IN (SELECT id FROM sys_menu WHERE path='/workforce/hook-bundles')`,
		`UPDATE sys_menu SET "isDeleted"=true,"updateTime"=CURRENT_TIMESTAMP WHERE path='/workforce/hook-bundles' AND "isDeleted"=false`,
		`UPDATE sys_menu SET name='Hook 管理',component='workforce/hooks/index',permission='workforce:hook-package:list',remark='Hook 编辑、测试和发布',"updateTime"=CURRENT_TIMESTAMP WHERE path='/workforce/hooks' AND "isDeleted"=false`,
	}
	for _, statement := range retireRBACStatements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("retire Hook Bundle RBAC: %w", err)
		}
	}

	return nil
}

func verifyProjectSchema(ctx context.Context, db DB) error {
	var relations bool
	if err := db.QueryRow(ctx, `SELECT to_regclass('public.workforce_projects') IS NOT NULL AND to_regclass('public.workforce_project_versions') IS NOT NULL`).Scan(&relations); err != nil {
		return fmt.Errorf("verify Project relations: %w", err)
	}
	if !relations {
		return errors.New("Project schema is not aligned")
	}
	for _, removed := range retiredHookProductTables {
		var exists bool
		if err := db.QueryRow(ctx, `SELECT to_regclass('public.'||$1) IS NOT NULL`, removed).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("Project Hook hard cut still contains %s", removed)
		}
	}
	var retiredHookProductRBAC int64
	if err := db.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM sys_permission WHERE code LIKE 'workforce:hook-bundle:%' AND "isDeleted"=false) +
		(SELECT count(*) FROM sys_menu WHERE path='/workforce/hook-bundles' AND "isDeleted"=false)`).Scan(&retiredHookProductRBAC); err != nil {
		return fmt.Errorf("verify retired Hook product RBAC: %w", err)
	}
	if retiredHookProductRBAC != 0 {
		return errors.New("Project Hook hard cut still exposes Hook Bundle RBAC")
	}

	expected := []struct{ table, column, dataType, nullable string }{
		{"workforce_projects", "id", "bigint", "NO"},
		{"workforce_projects", "tenant_id", "bigint", "NO"},
		{"workforce_projects", "current_version_id", "bigint", "NO"},
		{"workforce_projects", "current_source_state", "character varying", "YES"},
		{"workforce_project_versions", "id", "bigint", "NO"},
		{"workforce_project_versions", "project_id", "bigint", "NO"},
		{"workforce_project_versions", "department_id", "bigint", "YES"},
		{"workforce_project_versions", "repository_credential_ciphertext", "bytea", "YES"},
		{"workforce_project_versions", "repository_credential_version", "integer", "YES"},
		{"workforce_project_versions", "repository_config_digest", "character varying", "YES"},
		{"workforce_project_versions", "completion_check_json", "jsonb", "NO"},
		{"workforce_project_versions", "completion_check_digest", "character varying", "NO"},
		{"workforce_project_versions", "result_processor_release_id", "bigint", "NO"},
		{"workforce_project_versions", "review_prompt", "text", "NO"},
		{"workforce_project_versions", "hook_release_ids", "ARRAY", "NO"},
		{"workforce_project_versions", "created_by_principal_id", "bigint", "NO"},
		{"workforce_tasks", "project_version_id", "bigint", "NO"},
		{"workforce_tasks", "selected_runtime_release_id", "bigint", "YES"},
		{"workforce_tasks", "execution_timeout_seconds", "integer", "YES"},
		{"workforce_tasks", "execution_active_milliseconds", "bigint", "NO"},
		{"workforce_tasks", "review_enabled", "boolean", "NO"},
		{"workforce_harness_releases", "provider_protocols_json", "jsonb", "NO"},
		{"workforce_harness_releases", "hook_events_json", "jsonb", "NO"},
		{"workforce_harness_releases", "manifest_json", "jsonb", "NO"},
	}
	for _, item := range expected {
		var dataType, nullable string
		err := db.QueryRow(ctx, `SELECT data_type,is_nullable FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, item.table, item.column).Scan(&dataType, &nullable)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && (dataType != item.dataType || nullable != item.nullable) {
			return fmt.Errorf("Project schema has incompatible %s.%s", item.table, item.column)
		}
		if err != nil {
			return err
		}
	}

	for _, removed := range []struct{ table, column string }{
		{"workforce_projects", "config_version"},
		{"workforce_tasks", "project_config_version"},
		{"workforce_tasks", "project_snapshot_json"},
		{"workforce_tasks", "project_snapshot_digest"},
		{"workforce_project_versions", "runtime_requirements_json"},
	} {
		var exists bool
		if err := db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2)`, removed.table, removed.column).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("Project hard cut still contains %s.%s", removed.table, removed.column)
		}
	}

	checks := []struct{ label, query string }{
		{"current version", `SELECT count(*) FROM workforce_projects p LEFT JOIN workforce_project_versions v ON v.id=p.current_version_id AND v.project_id=p.id AND v.tenant_id=p.tenant_id WHERE v.id IS NULL`},
		{"Task version", `SELECT count(*) FROM workforce_tasks t LEFT JOIN workforce_project_versions v ON v.id=t.project_version_id AND v.project_id=t.project_id AND v.tenant_id=t.tenant_id WHERE v.id IS NULL`},
		{"Project creator", `SELECT count(*) FROM workforce_projects x LEFT JOIN workforce_principals p ON p.id=x.created_by_principal_id WHERE p.id IS NULL OR p.principal_type<>'USER'`},
		{"version creator", `SELECT count(*) FROM workforce_project_versions x LEFT JOIN workforce_principals p ON p.id=x.created_by_principal_id WHERE p.id IS NULL OR p.principal_type<>'USER'`},
		{"Result Processor Release", `SELECT count(*) FROM workforce_project_versions x LEFT JOIN workforce_result_processor_releases r ON r.id=x.result_processor_release_id WHERE r.id IS NULL OR NOT ((r.scope_type='SYSTEM' AND r.tenant_id IS NULL) OR r.tenant_id=x.tenant_id)`},
		{"Hook Release", `SELECT count(*) FROM workforce_project_versions v CROSS JOIN LATERAL unnest(v.hook_release_ids) WITH ORDINALITY selected(release_id,ordinality) LEFT JOIN workforce_hook_package_releases r ON r.id=selected.release_id WHERE r.id IS NULL OR NOT (r.tenant_id=v.tenant_id OR r.scope_type='SYSTEM' AND r.tenant_id IS NULL)`},
		{"Task Runtime Release", `SELECT count(*) FROM workforce_tasks t LEFT JOIN workforce_harness_releases r ON r.id=t.selected_runtime_release_id WHERE t.selected_runtime_release_id IS NOT NULL AND r.id IS NULL`},
		{"Assignment Runtime Release", `SELECT count(*) FROM workforce_task_assignments a JOIN workforce_tasks t ON t.id=a.task_id WHERE t.selected_runtime_release_id IS NOT NULL AND a.agent_runtime_release_id<>t.selected_runtime_release_id`},
	}
	for _, check := range checks {
		var invalid int64
		if err := db.QueryRow(ctx, check.query).Scan(&invalid); err != nil {
			return fmt.Errorf("verify Project %s: %w", check.label, err)
		}
		if invalid != 0 {
			return fmt.Errorf("found %d rows with an invalid %s", invalid, check.label)
		}
	}
	return nil
}
