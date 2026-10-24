package schemaalignment

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func TestProjectFirstHardCutPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(schemaAlignmentTestDSNEnv))
	if dsn == "" {
		t.Skip("set SCHEMA_ALIGNMENT_TEST_POSTGRES_DSN to a dedicated schema-alignment PostgreSQL database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	requireDedicatedSchemaAlignmentDatabase(t, ctx, pool)

	pgxConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := stdlib.OpenDB(*pgxConfig)
	defer sqlDB.Close()

	serverRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	baselineDir := filepath.Join(serverRoot, "migrations", "postgres-baseline")
	migrationsDir := filepath.Join(serverRoot, "migrations", "postgres")

	t.Run("empty legacy schema aligns repeatedly", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		configureProjectFirstGoose(t)
		applyProjectFirstBaselineAndPredecessors(t, ctx, sqlDB, baselineDir, migrationsDir)
		if err = goose.UpToContext(ctx, sqlDB, migrationsDir, 20260825130000); err != nil {
			t.Fatalf("apply Project-first hard cut: %v", err)
		}
		if err = goose.UpToContext(ctx, sqlDB, migrationsDir, 20260825130000); err != nil {
			t.Fatalf("repeat Project-first alignment: %v", err)
		}
		for pass := 1; pass <= 2; pass++ {
			if _, err = pool.Exec(ctx, `SELECT workforce_assert_project_hard_cut_ready()`); err != nil {
				t.Fatalf("repeat empty hard-cut preflight pass %d: %v", pass, err)
			}
		}

		for _, table := range []string{"workforce_projects", "workforce_project_versions", "workforce_task_assignments"} {
			var relation *string
			if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1)::text`, table).Scan(&relation); err != nil {
				t.Fatal(err)
			}
			if relation == nil {
				t.Errorf("fresh Project-first migration did not create %s", table)
			}
		}
		for _, table := range []string{
			"workforce_result_processor_templates",
			"workforce_result_processor_template_releases",
			"workforce_task_execution_snapshots",
			"workforce_review_rubrics",
			"workforce_review_rubric_revisions",
			"workforce_review_rubric_releases",
			"workforce_review_profiles",
			"workforce_hook_bundles",
			"workforce_hook_bundle_releases",
			"workforce_hook_bindings",
			"workforce_hook_execution_profiles",
			"workforce_golden_verification_runs",
			"workforce_integrated_verification_runs",
		} {
			var relation *string
			if err = pool.QueryRow(ctx, `SELECT to_regclass('public.'||$1)::text`, table).Scan(&relation); err != nil {
				t.Fatal(err)
			}
			if relation != nil {
				t.Errorf("removed Project-first relation remains: %s", table)
			}
		}

		assertProjectFirstColumns(t, ctx, pool)

		if err = goose.DownContext(ctx, sqlDB, migrationsDir); err != nil {
			t.Fatalf("roll back Project-first hard cut: %v", err)
		}
		var projectTable *string
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.workforce_projects')::text`).Scan(&projectTable); err != nil {
			t.Fatal(err)
		}
		if projectTable != nil {
			t.Fatalf("workforce_projects remains after Down as %q", *projectTable)
		}
		if err = goose.UpToContext(ctx, sqlDB, migrationsDir, 20260825130000); err != nil {
			t.Fatalf("reapply Project-first hard cut after Down: %v", err)
		}
		assertProjectFirstColumns(t, ctx, pool)
		if err = goose.UpContext(ctx, sqlDB, migrationsDir); err != nil {
			t.Fatalf("apply migrations after Project-first hard cut: %v", err)
		}
		ids := &alignmentTestIDGenerator{next: 8_130_000_000_000_000_000}
		for pass := 1; pass <= 2; pass++ {
			report, alignErr := Align(ctx, pool, ids)
			if alignErr != nil {
				t.Fatalf("repeat Project-first Schema Alignment pass %d: %v", pass, alignErr)
			}
			if report.State != "ALIGNED" || report.RemainingTextIDs != 0 {
				t.Fatalf("alignment pass %d report=%#v, want ALIGNED with no TEXT identities", pass, report)
			}
			if verifyErr := Verify(ctx, pool); verifyErr != nil {
				t.Fatalf("verify Project-first schema pass %d: %v", pass, verifyErr)
			}
		}
	})

	t.Run("nonempty legacy business schema requires reset", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		configureProjectFirstGoose(t)
		applyProjectFirstBaselineAndPredecessors(t, ctx, sqlDB, baselineDir, migrationsDir)
		if _, err = pool.Exec(ctx, `INSERT INTO workforce_tasks
			(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status)
			VALUES (8100000000000000001,8100000000000000002,8100000000000000003,'legacy-task','legacy-hash','','','legacy task','SUCCEEDED')`); err != nil {
			t.Fatalf("install nonempty legacy Task: %v", err)
		}
		err = goose.UpToContext(ctx, sqlDB, migrationsDir, 20260825130000)
		if err == nil || !strings.Contains(err.Error(), "PROJECT_HARD_CUT_RESET_REQUIRED") {
			t.Fatalf("nonempty Project hard cut error=%v, want PROJECT_HARD_CUT_RESET_REQUIRED", err)
		}
		var projectTable *string
		if err = pool.QueryRow(ctx, `SELECT to_regclass('public.workforce_projects')::text`).Scan(&projectTable); err != nil {
			t.Fatal(err)
		}
		if projectTable != nil {
			t.Fatalf("failed hard cut was not atomic; workforce_projects remains as %q", *projectTable)
		}
	})

	t.Run("empty retired Hook products align in place", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		configureProjectFirstGoose(t)
		if err = goose.UpContext(ctx, sqlDB, baselineDir); err != nil {
			t.Fatalf("apply clean baseline: %v", err)
		}
		if err = goose.UpContext(ctx, sqlDB, migrationsDir); err != nil {
			t.Fatalf("apply current migrations: %v", err)
		}
		if _, err = pool.Exec(ctx, `
			ALTER TABLE workforce_project_versions DROP COLUMN hook_release_ids;
			ALTER TABLE workforce_task_assignments DROP COLUMN hook_configuration_digest;
			ALTER TABLE workforce_task_assignments ADD COLUMN managed_hook_release_id BIGINT;
			ALTER TABLE workforce_attempt_runtimes DROP COLUMN hook_configuration_digest;
			ALTER TABLE workforce_attempt_runtimes ADD COLUMN hook_bundle_digest TEXT NOT NULL DEFAULT '';
			CREATE TABLE workforce_hook_bundles (id BIGINT PRIMARY KEY);
			CREATE TABLE workforce_hook_bundle_releases (id BIGINT PRIMARY KEY);
			CREATE TABLE workforce_hook_bindings (id BIGINT PRIMARY KEY);
			CREATE TABLE workforce_hook_execution_profiles (id BIGINT PRIMARY KEY);
			CREATE TABLE workforce_golden_verification_runs (id BIGINT PRIMARY KEY);
			CREATE TABLE workforce_integrated_verification_runs (id BIGINT PRIMARY KEY);
		`); err != nil {
			t.Fatalf("install empty retired Hook schema: %v", err)
		}

		ids := &alignmentTestIDGenerator{next: 8_140_000_000_000_000_000}
		for pass := 1; pass <= 2; pass++ {
			if _, err = Align(ctx, pool, ids); err != nil {
				t.Fatalf("align retired Hook schema pass %d: %v", pass, err)
			}
		}
		if err = Verify(ctx, pool); err != nil {
			t.Fatalf("verify aligned direct Project Hook schema: %v", err)
		}
	})

	t.Run("populated retired Hook product requires reset", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		configureProjectFirstGoose(t)
		if err = goose.UpContext(ctx, sqlDB, baselineDir); err != nil {
			t.Fatalf("apply clean baseline: %v", err)
		}
		if err = goose.UpContext(ctx, sqlDB, migrationsDir); err != nil {
			t.Fatalf("apply current migrations: %v", err)
		}
		if _, err = pool.Exec(ctx, `CREATE TABLE workforce_hook_bundles (id BIGINT PRIMARY KEY); INSERT INTO workforce_hook_bundles(id) VALUES (1)`); err != nil {
			t.Fatalf("install populated retired Hook schema: %v", err)
		}
		_, err = Align(ctx, pool, &alignmentTestIDGenerator{next: 8_150_000_000_000_000_000})
		if err == nil || !strings.Contains(err.Error(), "PROJECT_HOOK_HARD_CUT_RESET_REQUIRED") {
			t.Fatalf("populated retired Hook product error=%v, want PROJECT_HOOK_HARD_CUT_RESET_REQUIRED", err)
		}
		var retained int64
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM workforce_hook_bundles`).Scan(&retained); err != nil {
			t.Fatal(err)
		}
		if retained != 1 {
			t.Fatalf("failed Hook hard cut was not atomic; retained rows=%d", retained)
		}
	})

	t.Run("legacy Provider data converts to model credentials repeatedly", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		configureProjectFirstGoose(t)
		if err = goose.UpContext(ctx, sqlDB, baselineDir); err != nil {
			t.Fatalf("apply clean baseline: %v", err)
		}
		if err = goose.UpContext(ctx, sqlDB, migrationsDir); err != nil {
			t.Fatalf("apply current migrations: %v", err)
		}
		if _, err = pool.Exec(ctx, `
DROP TABLE workforce_model_invocations;
DROP TABLE workforce_model_credentials;
CREATE TABLE workforce_provider_profiles (
 id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, alias TEXT NOT NULL, name TEXT NOT NULL,
 protocol TEXT NOT NULL, base_url TEXT NOT NULL, default_model TEXT NOT NULL,
 allowed_models_json JSONB NOT NULL DEFAULT '[]'::jsonb, capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
 request_timeout_millis BIGINT NOT NULL DEFAULT 300000, config_digest TEXT NOT NULL DEFAULT '',
 secret_ciphertext TEXT NOT NULL, secret_edek TEXT NOT NULL, secret_wrap_key_ref TEXT NOT NULL,
 secret_hint TEXT NOT NULL, status TEXT NOT NULL, version BIGINT NOT NULL DEFAULT 1,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 UNIQUE(tenant_id,alias), CONSTRAINT workforce_provider_profiles_protocol_check CHECK(protocol='OPENAI_RESPONSES'));
CREATE TABLE workforce_provider_invocations (
 invocation_id TEXT PRIMARY KEY, tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL,
 execution_kind TEXT NOT NULL DEFAULT 'AGENT_ATTEMPT', attempt_id BIGINT, review_run_id BIGINT,
 review_run_attempt_id BIGINT, generation BIGINT NOT NULL, owner_epoch BIGINT NOT NULL,
 worker_id BIGINT NOT NULL, worker_instance_id TEXT NOT NULL, host_boot_id TEXT NOT NULL,
 provider_profile_id BIGINT NOT NULL, provider_config_digest TEXT NOT NULL, model TEXT NOT NULL,
 operation TEXT NOT NULL, request_body_sha256 TEXT NOT NULL, nonce_sha256 TEXT NOT NULL,
 status TEXT NOT NULL, upstream_status_code INTEGER, upstream_request_id TEXT, headers_at TIMESTAMPTZ,
 first_byte_at TIMESTAMPTZ, finished_at TIMESTAMPTZ, duration_milliseconds BIGINT,
 input_tokens BIGINT, output_tokens BIGINT, total_tokens BIGINT, error_code TEXT,
 started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 CONSTRAINT workforce_provider_invocations_execution_identity_ck CHECK(
  execution_kind='AGENT_ATTEMPT' AND attempt_id IS NOT NULL AND review_run_id IS NULL
  AND review_run_attempt_id IS NULL AND generation>0 AND owner_epoch>0),
 CONSTRAINT workforce_provider_invocations_operation_check CHECK(operation IN ('responses.create','responses.compact')),
 UNIQUE(worker_id,nonce_sha256));
INSERT INTO workforce_provider_profiles
 (id,tenant_id,alias,name,protocol,base_url,default_model,allowed_models_json,config_digest,
  secret_ciphertext,secret_edek,secret_wrap_key_ref,secret_hint,status)
VALUES (8150000000000000001,8150000000000000002,'legacy','Legacy API','OPENAI_RESPONSES','https://models.example/v1',
 'gpt-main','["gpt-main","gpt-helper","gpt-main"]','aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
 'Y2lwaGVy','ZWRlaw==','wrap','abcd','ACTIVE');
INSERT INTO workforce_provider_invocations
 (invocation_id,tenant_id,task_id,attempt_id,generation,owner_epoch,worker_id,worker_instance_id,host_boot_id,
  provider_profile_id,provider_config_digest,model,operation,request_body_sha256,nonce_sha256,status,input_tokens,output_tokens,total_tokens)
VALUES ('pinv_legacy',8150000000000000002,8150000000000000003,8150000000000000004,1,1,8150000000000000005,
 'worker-instance','boot',8150000000000000001,'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
 'gpt-helper','responses.create','bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',
	'cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc','SUCCEEDED',10,2,12);
INSERT INTO sys_user (id,"userAccount","nickName")
VALUES (8150000000000000006,'legacy-model-owner','Legacy Model Owner');
INSERT INTO workforce_principals (id,principal_type,user_id)
VALUES (8150000000000000010,'USER',8150000000000000006);
INSERT INTO workforce_result_processors
 (id,scope_type,tenant_id,name,created_by_principal_id)
VALUES (8150000000000000011,'TENANT',8150000000000000002,'Legacy model processor',8150000000000000010);
INSERT INTO workforce_result_processor_revisions
 (id,processor_id,scope_type,tenant_id,revision_number,status,manifest_json,files_json,created_by_principal_id)
VALUES (8150000000000000012,8150000000000000011,'TENANT',8150000000000000002,1,'RELEASED','{}','[]',8150000000000000010);
INSERT INTO workforce_result_processor_releases
 (id,processor_id,scope_type,tenant_id,revision_id,version,manifest_json,files_json,archive_object_key,
  archive_sha256,archive_size_bytes,published_by_principal_id)
VALUES (8150000000000000007,8150000000000000011,'TENANT',8150000000000000002,8150000000000000012,'1.0.0',
 '{}','[]','legacy-model-processor','abababababababababababababababababababababababababababababababab',1,
 8150000000000000010);
INSERT INTO workforce_projects
 (id,tenant_id,project_key,status,current_version_id,created_by_principal_id,updated_by_principal_id,
  create_idempotency_key,create_request_digest)
VALUES (8150000000000000008,8150000000000000002,'legacy-model-project','ACTIVE',8150000000000000009,
	8150000000000000010,8150000000000000010,'legacy-model-project',
 'ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff');
INSERT INTO workforce_project_versions
 (id,tenant_id,project_id,name,kind,completion_check_digest,result_processor_release_id,review_prompt,
  delivery_mode,delivery_authorization,created_by_principal_id)
VALUES (8150000000000000009,8150000000000000002,8150000000000000008,'Legacy model project','GENERAL',
 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',8150000000000000007,'','NONE','NONE',
	8150000000000000010);
INSERT INTO workforce_tasks
 (id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,
  result_processor_release_id,result_processor_parameters_json,project_id,project_version_id,review_enabled,
  model_credential_id,primary_model,execution_requirements_json,execution_requirements_digest,version)
VALUES (8150000000000000003,8150000000000000002,8150000000000000006,'legacy-model-task',
 'dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd','','','legacy model task','RUNNING',
 8150000000000000007,'{}',8150000000000000008,8150000000000000009,false,8150000000000000001,'gpt-main','{}',
 'eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee',1);`); err != nil {
			t.Fatalf("install legacy Provider data: %v", err)
		}
		ids := &alignmentTestIDGenerator{next: 8_160_000_000_000_000_000}
		if _, err = Align(ctx, pool, ids); err == nil || !strings.Contains(err.Error(), "MODEL_CREDENTIAL_HARD_CUT_ACTIVE_EXECUTION") {
			t.Fatalf("active Task model hard cut error=%v", err)
		}
		if _, err = pool.Exec(ctx, `UPDATE workforce_tasks SET status='SUCCEEDED' WHERE id=8150000000000000003`); err != nil {
			t.Fatal(err)
		}
		for pass := 1; pass <= 2; pass++ {
			if _, err = Align(ctx, pool, ids); err != nil {
				t.Fatalf("align model credential hard cut pass %d: %v", pass, err)
			}
		}
		var protocol string
		var models []byte
		if err = pool.QueryRow(ctx, `SELECT protocol,models_json FROM workforce_model_credentials WHERE id=8150000000000000001`).Scan(&protocol, &models); err != nil {
			t.Fatal(err)
		}
		if protocol != "OPENAI" || string(models) != `["gpt-main", "gpt-helper"]` {
			t.Fatalf("converted credential protocol=%q models=%s", protocol, models)
		}
		var invocationID int64
		var credentialVersion int
		var requestedModel, primaryModel, invocationProtocol string
		if err = pool.QueryRow(ctx, `SELECT id,credential_version,requested_model,primary_model,protocol FROM workforce_model_invocations WHERE protocol_invocation_key='pinv_legacy'`).Scan(&invocationID, &credentialVersion, &requestedModel, &primaryModel, &invocationProtocol); err != nil {
			t.Fatal(err)
		}
		if invocationID <= 0 || credentialVersion != 1 || requestedModel != "gpt-helper" || primaryModel != "gpt-helper" || invocationProtocol != "OPENAI" {
			t.Fatalf("converted invocation id=%d version=%d requested=%q primary=%q protocol=%q", invocationID, credentialVersion, requestedModel, primaryModel, invocationProtocol)
		}
	})
}

func configureProjectFirstGoose(t *testing.T) {
	t.Helper()
	goose.SetTableName(goose.DefaultTablename)
	goose.SetVerbose(false)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
}

func applyProjectFirstBaselineAndPredecessors(t *testing.T, ctx context.Context, sqlDB *sql.DB, baselineDir, migrationsDir string) {
	t.Helper()
	if err := goose.UpContext(ctx, sqlDB, baselineDir); err != nil {
		t.Fatalf("apply clean baseline: %v", err)
	}
	if err := goose.UpToContext(ctx, sqlDB, migrationsDir, 20260825112000); err != nil {
		t.Fatalf("apply pre-Project migrations: %v", err)
	}
}

func assertProjectFirstColumns(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	want := map[string][]string{
		"workforce_projects": {
			"current_version_id", "current_source_state", "current_source_revision_id", "created_by_principal_id", "updated_by_principal_id",
		},
		"workforce_project_versions": {
			"project_id", "review_prompt", "hook_release_ids", "repository_credential_ciphertext", "delivery_authorized_by_principal_id",
		},
		"workforce_tasks": {
			"project_id", "project_version_id", "review_enabled",
		},
		"workforce_model_credentials": {
			"protocol", "base_url", "models_json", "secret_version", "last_discovery_status",
		},
		"workforce_worker_images": {
			"scheduling_scope", "scheduling_priority", "environment_capabilities_json", "execution_kinds_json",
		},
		"workforce_task_assignments": {
			"agent_runtime_release_id", "adapter_identity", "model_credential_id", "resolved_execution_snapshot_json", "selection_digest",
		},
		"workforce_task_attempts": {
			"assignment_selection_digest", "resolved_execution_snapshot_json",
		},
	}
	for table, columns := range want {
		for _, column := range columns {
			var exists bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema='public' AND table_name=$1 AND column_name=$2
			)`, table, column).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if !exists {
				t.Errorf("missing %s.%s", table, column)
			}
		}
	}
}
