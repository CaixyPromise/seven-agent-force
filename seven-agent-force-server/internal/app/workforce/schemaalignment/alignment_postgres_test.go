package schemaalignment

import (
	"context"
	"database/sql"
	"fmt"
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

const (
	schemaAlignmentTestDSNEnv = "SCHEMA_ALIGNMENT_TEST_POSTGRES_DSN"
	schemaAlignmentTestDBName = "seven_workforce_schema_alignment_test"
)

func TestSchemaAlignmentFreshAlignedAndLegacyPostgres(t *testing.T) {
	// Project-first is an intentional hard cut: populated pre-Project Workforce
	// schemas are rejected with PROJECT_HARD_CUT_RESET_REQUIRED instead of being
	// repaired field by field. Keep this matrix as executable historical
	// documentation; the current fresh/repeated/Down-Up/Verify contract lives in
	// TestProjectFirstHardCutPostgres.
	t.Skip("superseded by the Project-first hard-cut migration contract")
	dsn := strings.TrimSpace(os.Getenv(schemaAlignmentTestDSNEnv))
	if dsn == "" {
		t.Skip("set SCHEMA_ALIGNMENT_TEST_POSTGRES_DSN to the dedicated seven_workforce_schema_alignment_test PostgreSQL database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("open Schema Alignment test pool: %v", err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		t.Fatalf("ping Schema Alignment test database: %v", err)
	}
	requireDedicatedSchemaAlignmentDatabase(t, ctx, pool)

	pgxConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse Schema Alignment test DSN: %v", err)
	}
	sqlDB := stdlib.OpenDB(*pgxConfig)
	defer sqlDB.Close()

	t.Run("fresh migration is aligned", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		assertAlignedIntegrationCreatorContract(t, ctx, pool)
	})
	t.Run("pre-native-tool-call Batch4 schema is aligned", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_execution_operations DROP COLUMN native_tool_call_id`); err != nil {
			t.Fatal(err)
		}
		ids := &alignmentTestIDGenerator{next: 8_048_000_000_000_000_000}
		for pass := 0; pass < 2; pass++ {
			if _, err = Align(ctx, pool, ids); err != nil {
				t.Fatal(err)
			}
			if err = Verify(ctx, pool); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("Batch4 Git delivery schema is aligned repeatedly", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `
DROP TABLE workforce_delivery_decisions,workforce_delivery_intents;
DROP INDEX workforce_delivery_requests_id_version_uidx;
ALTER TABLE workforce_delivery_requests DROP COLUMN version;
ALTER TABLE workforce_task_git_plans
  DROP COLUMN delivery_authorization,
  DROP COLUMN delivery_authorized_by_principal_id,
  DROP COLUMN delivery_authorized_at;`); err != nil {
			t.Fatalf("install Batch 4 Git delivery schema: %v", err)
		}
		ids := &alignmentTestIDGenerator{next: 8_048_500_000_000_000_000}
		for pass := 0; pass < 2; pass++ {
			if _, err = Align(ctx, pool, ids); err != nil {
				t.Fatalf("align Batch 4 Git delivery schema pass %d: %v", pass+1, err)
			}
			if err = Verify(ctx, pool); err != nil {
				t.Fatalf("verify Batch 4 Git delivery schema pass %d: %v", pass+1, err)
			}
		}
	})
	t.Run("batch3 absent interaction schema is added without changing result checkpoints", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		// Empty, run-owned database: remove the Batch3/4 additions to model the
		// preceding published checkpoint; all original Result columns stay.
		_, err := pool.Exec(ctx, `
DROP TABLE workforce_execution_operation_attempts,workforce_task_network_grants,workforce_execution_operation_decisions,workforce_execution_operations;
DROP TABLE workforce_completion_check_revisions,workforce_task_conversation_read_states,workforce_task_conversation_items,workforce_agent_interaction_responses,workforce_agent_interactions;
ALTER TABLE workforce_attempt_runtimes DROP COLUMN adapter_session_generation;
ALTER TABLE workforce_continuation_restore_runs DROP CONSTRAINT workforce_restore_interaction_budget_ck;
ALTER TABLE workforce_continuation_restore_runs DROP COLUMN interaction_execution_budget_json;
ALTER TABLE workforce_task_turns DROP CONSTRAINT workforce_turn_interaction_response_ck;
ALTER TABLE workforce_task_turns DROP COLUMN source_interaction_response_id,DROP COLUMN interaction_response_json;
DROP INDEX workforce_turn_execution_operation_once_idx;
ALTER TABLE workforce_task_turns DROP CONSTRAINT workforce_task_turns_source_execution_operation_ck;
ALTER TABLE workforce_task_turns DROP COLUMN source_execution_operation_id;
DROP INDEX workforce_turn_source_checkpoint_once_idx;
ALTER TABLE workforce_continuation_checkpoints DROP CONSTRAINT workforce_checkpoint_purpose_ck,DROP CONSTRAINT workforce_checkpoint_interaction_manifest_ck;
ALTER TABLE workforce_continuation_checkpoints DROP COLUMN purpose,DROP COLUMN interaction_id,DROP COLUMN interaction_phase,DROP COLUMN adapter_session_generation,DROP COLUMN automatic_revision_count,DROP COLUMN interaction_round,DROP COLUMN suspend_control_key_sha256,DROP COLUMN interaction_manifest_json;
ALTER TABLE workforce_continuation_checkpoints ALTER COLUMN evaluation_cycle_id SET NOT NULL;`)
		if err != nil {
			t.Fatal(err)
		}
		ids := &alignmentTestIDGenerator{next: 8_049_000_000_000_000_000}
		for pass := 0; pass < 2; pass++ {
			if _, err = Align(ctx, pool, ids); err != nil {
				t.Fatal(err)
			}
			if err = Verify(ctx, pool); err != nil {
				t.Fatal(err)
			}
		}
	})

	t.Run("legacy system identity subject is protected atomically", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `INSERT INTO sys_external_user_identity
			(id,"providerCode","externalIssuer","externalSubject","userId",status,"creatorId","updaterId","isDeleted")
			VALUES (8091,'github','https://github.com','provider-subject-42',101,0,101,101,0)`); err != nil {
			t.Fatalf("install plaintext SYSTEM identity fixture: %v", err)
		}
		protector := alignmentTestSubjectProtector{}
		report, err := AlignWithSubjectProtection(ctx, pool, &alignmentTestIDGenerator{next: 8_050_000_000_000_000_000}, protector)
		if err != nil {
			t.Fatalf("protect legacy SYSTEM identity: %v", err)
		}
		if report.State != "ALIGNED" {
			t.Fatalf("protected alignment report=%+v", report)
		}
		var plaintext *string
		var ciphertext, edek, wrapKeyRef, digest, digestKeyRef string
		if err = pool.QueryRow(ctx, `SELECT "externalSubject",subject_ciphertext,subject_edek,subject_wrap_key_ref,subject_digest,subject_digest_key_ref
			FROM sys_external_user_identity WHERE id=8091`).Scan(&plaintext, &ciphertext, &edek, &wrapKeyRef, &digest, &digestKeyRef); err != nil {
			t.Fatalf("read protected SYSTEM identity: %v", err)
		}
		if plaintext != nil || ciphertext != "cipher:provider-subject-42" || edek != "edek" || wrapKeyRef != "wrap-key" || digest != "digest:github:provider-subject-42" || digestKeyRef != "digest-key" {
			t.Fatalf("unexpected protected SYSTEM identity plaintext=%v cipher=%q edek=%q wrap=%q digest=%q digestKey=%q", plaintext, ciphertext, edek, wrapKeyRef, digest, digestKeyRef)
		}
	})

	t.Run("aligned migration is idempotent", func(t *testing.T) {
		report, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_100_000_000_000_000_000})
		if err != nil {
			t.Fatalf("align current schema: %v", err)
		}
		if report.State != "ALIGNED" || report.ConvertedColumns != 0 || report.MappedIdentities != 0 {
			t.Fatalf("aligned schema must be a no-op, report=%+v", report)
		}
		assertAlignedIntegrationCreatorContract(t, ctx, pool)
	})

	t.Run("semantic probe is isolated from existing admitted Task Harnesses", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `INSERT INTO workforce_harness_releases
(id,alias,image_ref,image_digest,contract_version,supervisor_version,harness_type,harness_version,capabilities_json,admission_evidence_json,status)
VALUES (8100000000000000100,'alignment-existing-harness','alignment-existing@sha256:eeee',
        $1,'seven.worker-image/v1','fixture','fixture','fixture',
        '["harness.codex.review","review.structured-output"]'::jsonb,'{"fixture":true}'::jsonb,'ACTIVE')`, "sha256:"+strings.Repeat("e", 64)); err != nil {
			t.Fatalf("install existing admitted Harness: %v", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_101_000_000_000_000_000}); err != nil {
			t.Fatalf("alignment semantic probe depended on an empty Harness catalog: %v", err)
		}
		if err := Verify(ctx, pool); err != nil {
			t.Fatalf("verify schema with existing admitted Harness: %v", err)
		}
	})

	t.Run("stale projection constraint and index are normalized", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_task_events DROP CONSTRAINT workforce_task_events_open_projection_status_ck;
ALTER TABLE workforce_task_events ADD CONSTRAINT workforce_task_events_open_projection_status_ck
  CHECK (open_integration_projection_status IN ('PENDING','PROJECTED','SKIPPED_NO_WEBHOOK'));
DROP INDEX idx_workforce_task_events_open_projection_pending;
CREATE INDEX idx_workforce_task_events_open_projection_pending
  ON workforce_task_events(task_id) WHERE open_integration_projection_status='PENDING';
INSERT INTO workforce_task_events(task_id,event_type,sequence,payload_json,open_integration_projection_status)
VALUES (8890,'task.succeeded',1,'{}'::jsonb,'SKIPPED_NO_WEBHOOK')`); err != nil {
			t.Fatalf("install stale projection schema: %v", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_105_000_000_000_000_000}); err != nil {
			t.Fatalf("align stale projection schema: %v", err)
		}
		var status string
		if err := pool.QueryRow(ctx, `SELECT open_integration_projection_status FROM workforce_task_events WHERE task_id=8890`).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "SKIPPED" {
			t.Fatalf("normalized projection status=%q, want SKIPPED", status)
		}
		if err := Verify(ctx, pool); err != nil {
			t.Fatalf("verify normalized projection schema: %v", err)
		}
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
	})

	t.Run("oversized webhook URL fails alignment without truncation", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_integration_applications ALTER COLUMN webhook_url TYPE TEXT`); err != nil {
			t.Fatalf("relax webhook URL fixture column: %v", err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO workforce_integration_applications(id,tenant_id,name,status,webhook_url,created_by)
VALUES (8891,1,'Oversized Webhook','ENABLED',$1,101)`, strings.Repeat("x", 2049)); err != nil {
			t.Fatalf("install oversized webhook fixture: %v", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_110_000_000_000_000_000}); err == nil || !strings.Contains(err.Error(), "longer than 2048") {
			t.Fatalf("oversized webhook URL must fail closed, err=%v", err)
		}
		var stored string
		if err := pool.QueryRow(ctx, `SELECT webhook_url FROM workforce_integration_applications WHERE id=8891`).Scan(&stored); err != nil {
			t.Fatalf("read oversized webhook after rollback: %v", err)
		}
		if len(stored) != 2049 {
			t.Fatalf("oversized webhook was truncated to %d bytes", len(stored))
		}
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
	})

	t.Run("partial credential backfill mismatch fails closed", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_integration_applications
  ADD COLUMN credential_locator TEXT,
  ADD COLUMN credential_digest TEXT,
  ADD COLUMN credential_hint TEXT,
  ADD COLUMN signing_secret_ciphertext TEXT,
  ADD COLUMN signing_secret_edek TEXT,
  ADD COLUMN signing_secret_wrap_key_ref TEXT,
  ADD COLUMN signing_secret_hint TEXT;
INSERT INTO workforce_integration_applications(
  id,tenant_id,name,status,webhook_url,created_by,credential_locator,credential_digest,credential_hint,
  signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint
) VALUES (8901,1,'Partial Alignment','ENABLED','https://example.com/hook',101,'locator-good','digest-good','hint-good','cipher-good','edek-good','wrap-good','sign-good');
INSERT INTO workforce_integration_api_credentials(
  id,tenant_id,integration_application_id,credential_locator,credential_digest,credential_hint,version,status,created_by
) VALUES (8902,1,8901,'locator-wrong','digest-wrong','hint-wrong',1,'CURRENT',101);
INSERT INTO workforce_integration_signing_credentials(
  id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,created_by
) VALUES (8903,1,8901,'cipher-wrong','edek-wrong','wrap-wrong','sign-wrong',1,'CURRENT',101)`); err != nil {
			t.Fatalf("install partial Credential alignment fixture: %v", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_125_000_000_000_000_000}); err == nil || !strings.Contains(err.Error(), "content mismatch") {
			t.Fatalf("partial Credential mismatch must fail closed, err=%v", err)
		}
		var legacyColumnCount int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
WHERE table_schema='public' AND table_name='workforce_integration_applications'
  AND column_name IN ('credential_locator','credential_digest','credential_hint','signing_secret_ciphertext','signing_secret_edek','signing_secret_wrap_key_ref','signing_secret_hint')`).Scan(&legacyColumnCount); err != nil {
			t.Fatalf("verify legacy columns after rollback: %v", err)
		}
		if legacyColumnCount != 7 {
			t.Fatalf("legacy Credential columns were not preserved after failed alignment: %d", legacyColumnCount)
		}
		if _, err := pool.Exec(ctx, `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=8901;
DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=8901;
DELETE FROM workforce_integration_applications WHERE id=8901;
ALTER TABLE workforce_integration_applications
  DROP COLUMN credential_locator,
  DROP COLUMN credential_digest,
  DROP COLUMN credential_hint,
  DROP COLUMN signing_secret_ciphertext,
  DROP COLUMN signing_secret_edek,
  DROP COLUMN signing_secret_wrap_key_ref,
  DROP COLUMN signing_secret_hint`); err != nil {
			t.Fatalf("clean partial Credential alignment fixture: %v", err)
		}
	})

	t.Run("previous BIGINT alignment is repaired without losing external identities", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `
ALTER TABLE workforce_webhook_delivery_attempts DROP COLUMN grace_signing_credential_id;
ALTER TABLE workforce_webhook_delivery_attempts DROP COLUMN signing_credential_id;
DROP TABLE workforce_integration_signing_credentials;
DROP TABLE workforce_integration_api_credentials;
ALTER TABLE workforce_integration_applications
  ADD COLUMN credential_locator TEXT,
  ADD COLUMN credential_digest TEXT,
  ADD COLUMN credential_hint TEXT,
  ADD COLUMN signing_secret_ciphertext TEXT,
  ADD COLUMN signing_secret_edek TEXT,
  ADD COLUMN signing_secret_wrap_key_ref TEXT,
  ADD COLUMN signing_secret_hint TEXT;
INSERT INTO workforce_principals(id,principal_type,user_id) VALUES (9101,'USER',101);
INSERT INTO workforce_legacy_identity_map(entity_type,legacy_identity,entity_id) VALUES ('GLOBAL','crd_legacy',9201);
INSERT INTO workforce_integration_applications(
  id,tenant_id,name,status,credential_locator,credential_digest,credential_hint,
  webhook_url,signing_secret_ciphertext,signing_secret_edek,
  signing_secret_wrap_key_ref,signing_secret_hint,created_by
) VALUES (
  9001,1,'Legacy Integration','ENABLED','crd_legacy','digest','hint',
  'https://example.com/webhook','ciphertext','edek','key-ref','secret-hint',101
);
ALTER TABLE workforce_integration_applications RENAME COLUMN created_by TO created_by_principal_id;
UPDATE workforce_integration_applications SET created_by_principal_id=9101 WHERE id=9001;
UPDATE workforce_integration_applications SET credential_locator='9201' WHERE id=9001;
ALTER TABLE workforce_integration_applications RENAME COLUMN credential_locator TO credential_id;
ALTER TABLE workforce_integration_applications ALTER COLUMN credential_id TYPE BIGINT USING credential_id::BIGINT;
ALTER TABLE workforce_tasks ALTER COLUMN external_correlation_id TYPE BIGINT USING NULLIF(external_correlation_id,'')::BIGINT;
ALTER TABLE workforce_task_attempts ALTER COLUMN worker_instance_id TYPE BIGINT USING NULLIF(worker_instance_id,'')::BIGINT;
ALTER TABLE workforce_task_attempts ALTER COLUMN host_boot_id TYPE BIGINT USING NULLIF(host_boot_id,'')::BIGINT;
`); err != nil {
			t.Fatalf("install previous BIGINT alignment fixture: %v", err)
		}
		state, _, err := Inspect(ctx, pool)
		if err != nil {
			t.Fatalf("inspect previous BIGINT alignment: %v", err)
		}
		if state != "COMPATIBILITY_LEGACY" {
			t.Fatalf("previous BIGINT alignment state=%s, want COMPATIBILITY_LEGACY", state)
		}
		report, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_150_000_000_000_000_000})
		if err != nil {
			t.Fatalf("repair previous BIGINT alignment: %v", err)
		}
		if report.State != "ALIGNED" {
			t.Fatalf("previous BIGINT alignment was not repaired, report=%+v", report)
		}
		assertAlignedIntegrationCreatorContract(t, ctx, pool)
		var creator int64
		var locator string
		if err = pool.QueryRow(ctx, `SELECT a.created_by,c.credential_locator
FROM workforce_integration_applications a JOIN workforce_integration_api_credentials c ON c.integration_application_id=a.id
WHERE a.id=9001 AND c.status='CURRENT'`).Scan(&creator, &locator); err != nil {
			t.Fatalf("read repaired Integration Application: %v", err)
		}
		if creator != 101 || locator != "crd_legacy" {
			t.Fatalf("repaired Integration Application creator=%d locator=%q, want 101 and crd_legacy", creator, locator)
		}
		var correlationType string
		if err = pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_tasks' AND column_name='external_correlation_id'`).Scan(&correlationType); err != nil {
			t.Fatalf("read repaired external correlation type: %v", err)
		}
		if correlationType != "text" {
			t.Fatalf("external correlation type=%s, want text", correlationType)
		}
	})

	t.Run("legacy migration preserves numeric Integration Application creator", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyPreWorkforceSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, legacySchemaAlignmentFixture); err != nil {
			t.Fatalf("install legacy Schema Alignment fixture: %v", err)
		}
		state, _, err := Inspect(ctx, pool)
		if err != nil {
			t.Fatalf("inspect legacy schema: %v", err)
		}
		if state != "LEGACY" {
			t.Fatalf("legacy fixture state=%s, want LEGACY", state)
		}
		report, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_200_000_000_000_000_000})
		if err != nil {
			t.Fatalf("align legacy schema: %v", err)
		}
		if report.State != "ALIGNED" || report.ConvertedColumns == 0 {
			t.Fatalf("legacy schema was not converted, report=%+v", report)
		}
		state, remaining, err := Inspect(ctx, pool)
		if err != nil || state != "ALIGNED" || len(remaining) != 0 {
			t.Fatalf("legacy identity alignment state=%s remaining=%v err=%v", state, remaining, err)
		}
		var creator int64
		if err = pool.QueryRow(ctx, `SELECT created_by FROM workforce_integration_applications WHERE id=9001`).Scan(&creator); err != nil {
			t.Fatalf("read preserved Integration Application creator: %v", err)
		}
		if creator != 101 {
			t.Fatalf("Integration Application created_by=%d, want Framework user 101", creator)
		}
		for _, table := range []string{
			"workforce_review_rubrics",
			"workforce_review_rubric_revisions",
			"workforce_review_rubric_releases",
			"workforce_review_policy_bootstrap_records",
			"workforce_review_profiles",
			"workforce_review_runs",
			"workforce_review_run_attempts",
		} {
			var partialTable *string
			if err = pool.QueryRow(ctx, `SELECT to_regclass('public.' || $1)::text`, table).Scan(&partialTable); err != nil {
				t.Fatalf("inspect pre-Fleet %s table: %v", table, err)
			}
			if partialTable != nil {
				t.Fatalf("identity alignment must not pre-create %s before its authoritative migration: %q", table, *partialTable)
			}
		}
	})

	t.Run("malformed Task notification schema fails closed", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_task_notification_records ALTER COLUMN source_id TYPE TEXT USING source_id::TEXT`); err != nil {
			t.Fatalf("install malformed Task notification fixture: %v", err)
		}
		if err := verifyTaskNotificationSchemaShape(ctx, pool); err == nil || !strings.Contains(err.Error(), "Task notification schema") {
			t.Fatalf("Verify error=%v, want Task notification schema drift", err)
		}
	})

	t.Run("same-name malformed ReviewRun task cursor index is repaired exactly", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `
DROP INDEX workforce_review_runs_task_read_idx;
CREATE INDEX workforce_review_runs_task_read_idx
  ON workforce_review_runs(tenant_id,task_id,created_at DESC,id DESC)`); err != nil {
			t.Fatalf("install malformed ReviewRun task read index: %v", err)
		}
		if err := verifyReviewRunSchemaShape(ctx, pool); err == nil || !strings.Contains(err.Error(), "task read index") {
			t.Fatalf("Verify error=%v, want exact ReviewRun task read index drift", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_350_000_000_000_000_000}); err != nil {
			t.Fatalf("repair malformed ReviewRun task read index: %v", err)
		}
		matched, exists, err := reviewRunTaskReadIndexMatches(ctx, pool)
		if err != nil || !exists || !matched {
			t.Fatalf("repaired ReviewRun task read index matched=%t exists=%t err=%v", matched, exists, err)
		}
		var definition string
		if err = pool.QueryRow(ctx, `SELECT pg_get_indexdef('workforce_review_runs_task_read_idx'::regclass)`).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(definition, "(tenant_id, task_id, id DESC)") || strings.Contains(definition, "created_at") {
			t.Fatalf("unexpected repaired ReviewRun task read index: %s", definition)
		}
	})

	t.Run("same-name malformed Task notification index fails alignment closed", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `
DROP INDEX workforce_task_notification_records_dispatch_idx;
CREATE INDEX workforce_task_notification_records_dispatch_idx
  ON workforce_task_notification_records(task_id,status)`); err != nil {
			t.Fatalf("install malformed Task notification index: %v", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_300_000_000_000_000_000}); err == nil || !strings.Contains(err.Error(), "Task notification indexes") {
			t.Fatalf("Align error=%v, want exact Task notification index drift", err)
		}
		var definition string
		if err := pool.QueryRow(ctx, `SELECT pg_get_indexdef('workforce_task_notification_records_dispatch_idx'::regclass)`).Scan(&definition); err != nil {
			t.Fatalf("read malformed index after rollback: %v", err)
		}
		if !strings.Contains(definition, "(task_id, status)") {
			t.Fatalf("malformed index unexpectedly rewritten despite rollback: %s", definition)
		}
	})

	t.Run("missing Task notification check fails alignment closed", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_task_notification_records DROP CONSTRAINT workforce_task_notification_records_focus_check`); err != nil {
			t.Fatalf("drop Task notification focus check: %v", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_400_000_000_000_000_000}); err == nil || !strings.Contains(err.Error(), "Task notification checks") {
			t.Fatalf("Align error=%v, want missing Task notification check drift", err)
		}
	})

	t.Run("human revision guidance length drift is detected and repaired", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_human_review_decisions ALTER COLUMN revision_guidance TYPE VARCHAR(8000)`); err != nil {
			t.Fatalf("install guidance length drift: %v", err)
		}
		if err := Verify(ctx, pool); err == nil || !strings.Contains(err.Error(), "guidance length") {
			t.Fatalf("Verify error=%v, want guidance length drift", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_410_000_000_000_000_000}); err != nil {
			t.Fatalf("repair guidance length drift: %v", err)
		}
		if err := Verify(ctx, pool); err != nil {
			t.Fatalf("verify repaired guidance length: %v", err)
		}
	})

	t.Run("human revision guidance constraint drift is detected and repaired", func(t *testing.T) {
		resetSchemaAlignmentDatabase(t, ctx, pool)
		applyFreshSchemaAlignmentMigrations(t, ctx, sqlDB)
		if _, err := pool.Exec(ctx, `ALTER TABLE workforce_human_review_decisions DROP CONSTRAINT workforce_human_review_decisions_guidance_ck, ADD CONSTRAINT workforce_human_review_decisions_guidance_ck CHECK (TRUE)`); err != nil {
			t.Fatalf("install guidance constraint drift: %v", err)
		}
		if err := Verify(ctx, pool); err == nil || !strings.Contains(err.Error(), "guidance constraint") {
			t.Fatalf("Verify error=%v, want guidance constraint drift", err)
		}
		if _, err := Align(ctx, pool, &alignmentTestIDGenerator{next: 8_420_000_000_000_000_000}); err != nil {
			t.Fatalf("repair guidance constraint drift: %v", err)
		}
		if err := Verify(ctx, pool); err != nil {
			t.Fatalf("verify repaired guidance constraint: %v", err)
		}
	})
}

type alignmentTestIDGenerator struct{ next int64 }

func (g *alignmentTestIDGenerator) NextID() int64 {
	g.next++
	return g.next
}

type alignmentTestSubjectProtector struct{}

func (alignmentTestSubjectProtector) EncryptSubject(_ context.Context, plaintext string) (ProtectedSubject, error) {
	return ProtectedSubject{Ciphertext: "cipher:" + plaintext, EDEK: "edek", WrapKeyRef: "wrap-key"}, nil
}

func (alignmentTestSubjectProtector) DigestSystemSubject(_ context.Context, provider, _ string, subject string) (string, string, error) {
	return "digest:" + provider + ":" + subject, "digest-key", nil
}

func requireDedicatedSchemaAlignmentDatabase(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var database string
	if err := pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		t.Fatalf("read Schema Alignment test database name: %v", err)
	}
	// A run-specific dedicated database avoids destroying another developer's
	// retained alignment evidence. The destructive test namespace stays narrow.
	if database != schemaAlignmentTestDBName && !strings.HasPrefix(database, schemaAlignmentTestDBName+"_") {
		t.Fatalf("refusing destructive Schema Alignment test on database %q; want %q", database, schemaAlignmentTestDBName)
	}
}

func resetSchemaAlignmentDatabase(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset dedicated Schema Alignment test database: %v", err)
	}
}

func applyFreshSchemaAlignmentMigrations(t *testing.T, ctx context.Context, sqlDB *sql.DB) {
	t.Helper()
	serverRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve server root: %v", err)
	}
	goose.SetTableName(goose.DefaultTablename)
	goose.SetVerbose(false)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	if err = goose.UpContext(ctx, sqlDB, filepath.Join(serverRoot, "migrations", "postgres-baseline")); err != nil {
		t.Fatalf("apply fresh PostgreSQL baseline: %v", err)
	}
	if err = goose.UpContext(ctx, sqlDB, filepath.Join(serverRoot, "migrations", "postgres")); err != nil {
		t.Fatalf("migrate fresh Schema Alignment database: %v", err)
	}
}

func applyPreWorkforceSchemaAlignmentMigrations(t *testing.T, ctx context.Context, sqlDB *sql.DB) {
	t.Helper()
	serverRoot, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve server root: %v", err)
	}
	goose.SetTableName(goose.DefaultTablename)
	goose.SetVerbose(false)
	if err = goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	if err = goose.UpContext(ctx, sqlDB, filepath.Join(serverRoot, "migrations", "postgres-baseline")); err != nil {
		t.Fatalf("apply pre-Workforce PostgreSQL baseline: %v", err)
	}
	if err = goose.UpToContext(ctx, sqlDB, filepath.Join(serverRoot, "migrations", "postgres"), 20260810110000); err != nil {
		t.Fatalf("apply pre-Workforce PostgreSQL migrations: %v", err)
	}
}

func assertAlignedIntegrationCreatorContract(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	state, columns, err := Inspect(ctx, pool)
	if err != nil {
		t.Fatalf("inspect aligned schema: %v", err)
	}
	if state != "ALIGNED" || len(columns) != 0 {
		t.Fatalf("schema state=%s non-allowlisted-text-identities=%v, want ALIGNED", state, columns)
	}
	if err = Verify(ctx, pool); err != nil {
		t.Fatalf("verify aligned schema: %v", err)
	}
	stats, err := ReadCatalogStats(ctx, pool)
	if err != nil {
		t.Fatalf("read aligned identity catalog: %v", err)
	}
	if stats.BigintIdentityColumns == 0 || stats.TextIdentityExceptions > len(governedTextIdentityExceptions()) || stats.ForeignKeys != 0 {
		t.Fatalf("unexpected aligned identity catalog: %+v", stats)
	}
	t.Logf("identity catalog: bigint=%d governed-text-exceptions=%d foreign-keys=%d", stats.BigintIdentityColumns, stats.TextIdentityExceptions, stats.ForeignKeys)
	var dataType string
	if err = pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_integration_applications' AND column_name='created_by'`).Scan(&dataType); err != nil {
		t.Fatalf("read Integration Application created_by type: %v", err)
	}
	if dataType != "bigint" {
		t.Fatalf("Integration Application created_by type=%s, want bigint", dataType)
	}
	var webhookType, webhookNullable string
	var webhookMaxLength int
	var webhookDefault *string
	if err = pool.QueryRow(ctx, `SELECT data_type,character_maximum_length,is_nullable,column_default
FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_integration_applications' AND column_name='webhook_url'`).Scan(&webhookType, &webhookMaxLength, &webhookNullable, &webhookDefault); err != nil {
		t.Fatalf("read Integration Application webhook URL contract: %v", err)
	}
	if webhookType != "character varying" || webhookMaxLength != 2048 || webhookNullable != "NO" || webhookDefault == nil || !strings.Contains(*webhookDefault, "''") {
		t.Fatalf("unexpected webhook URL contract: type=%s max=%d nullable=%s default=%v", webhookType, webhookMaxLength, webhookNullable, webhookDefault)
	}
	var principalColumn bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_integration_applications' AND column_name='created_by_principal_id')`).Scan(&principalColumn); err != nil {
		t.Fatalf("inspect Integration Application principal column: %v", err)
	}
	if principalColumn {
		t.Fatal("Integration Application must not gain created_by_principal_id")
	}
	for _, legacyColumn := range []string{"credential_locator", "credential_digest", "credential_hint", "signing_secret_ciphertext", "signing_secret_edek", "signing_secret_wrap_key_ref", "signing_secret_hint"} {
		var exists bool
		if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_integration_applications' AND column_name=$1)`, legacyColumn).Scan(&exists); err != nil {
			t.Fatalf("inspect legacy Application column %s: %v", legacyColumn, err)
		}
		if exists {
			t.Fatalf("legacy Application credential column %s remains after alignment", legacyColumn)
		}
	}
	for _, credentialTable := range []string{"workforce_integration_api_credentials", "workforce_integration_signing_credentials"} {
		for _, identityColumn := range []string{"id", "tenant_id", "integration_application_id", "created_by"} {
			if err = pool.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, credentialTable, identityColumn).Scan(&dataType); err != nil {
				t.Fatalf("read %s.%s type: %v", credentialTable, identityColumn, err)
			}
			if dataType != "bigint" {
				t.Fatalf("%s.%s type=%s, want bigint", credentialTable, identityColumn, dataType)
			}
		}
	}
	for _, column := range []string{"worker_instance_id", "host_boot_id"} {
		if err = pool.QueryRow(ctx, `SELECT COALESCE((SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_task_attempts' AND column_name=$1),'absent')`, column).Scan(&dataType); err != nil {
			t.Fatalf("read Task Attempt %s type: %v", column, err)
		}
		if dataType == "absent" {
			continue
		}
		if dataType != "text" {
			t.Fatalf("Task Attempt %s type=%s, want text process identity", column, dataType)
		}
	}
}

var legacySchemaAlignmentFixture = fmt.Sprintf(`
INSERT INTO sys_org(id,code,name) VALUES (1,'LEGACY_FIXTURE','Legacy fixture');
INSERT INTO sys_user(id,"userAccount","nickName") VALUES (101,'owner','Owner');

-- Schema Alignment's actionable-notification convergence is additive over
-- Framework-owned base relations. The legacy fixture must model those
-- pre-existing relations even though their newer actionable columns are absent.
-- The pre-Workforce migration set now includes a newer Framework shape for
-- these relations. Recreate the deliberately legacy fixture shape so this
-- test continues to exercise the additive alignment path instead of silently
-- inheriting the modern columns and constraints from that baseline.
DROP TABLE IF EXISTS sys_notification_delivery CASCADE;
DROP TABLE IF EXISTS sys_notification_recipient CASCADE;
DROP TABLE IF EXISTS sys_notification CASCADE;
DROP TABLE IF EXISTS sys_notification_template_revision CASCADE;
CREATE TABLE sys_notification_template_revision (id BIGINT PRIMARY KEY);
CREATE TABLE sys_notification (id BIGINT PRIMARY KEY);
CREATE TABLE sys_notification_recipient (id BIGINT PRIMARY KEY);
CREATE TABLE sys_notification_delivery (id BIGINT PRIMARY KEY);

CREATE TABLE workforce_tasks (
  id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  created_by BIGINT NOT NULL,
  integration_application_id BIGINT,
  status TEXT NOT NULL,
  execution_snapshot_json JSONB
);
INSERT INTO workforce_tasks(id,tenant_id,created_by,integration_application_id,status) VALUES ('tsk_legacy','org:1',101,9001,'SUCCEEDED');

-- This is the pre-Slice-2 execution snapshot relation from an existing
-- Workforce database. The fixture intentionally retains its legacy textual
-- relation keys so alignment still has to converge them before review fields
-- are added.
CREATE TABLE workforce_task_execution_snapshots (
  task_id TEXT PRIMARY KEY,
  tenant_id TEXT NOT NULL,
  harness_release_id TEXT NOT NULL,
  harness_image_digest TEXT NOT NULL,
  hook_configuration_digest TEXT,
  package_releases_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  dispatcher_version TEXT,
  dispatcher_sha256 TEXT,
  adapter_version TEXT,
  isolation_profile TEXT NOT NULL,
  snapshot_json JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE workforce_task_events (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  event_type TEXT NOT NULL
);

CREATE TABLE workforce_public_events (
  id TEXT PRIMARY KEY,
  source_task_event_id TEXT NOT NULL UNIQUE
);

-- Task notification relationship verification is additive over the Worker
-- Fleet and Workforce MVP relations. Keep these legacy identities textual so
-- this fixture also proves their conversion instead of bypassing alignment.
CREATE TABLE workforce_task_turns (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL,
  tenant_id TEXT NOT NULL,
  created_by BIGINT NOT NULL
);
INSERT INTO workforce_task_turns(id,task_id,tenant_id,created_by)
VALUES ('trn_legacy','tsk_legacy','org:1',101);

CREATE TABLE workforce_outbox_events (
  id TEXT PRIMARY KEY,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL
);

CREATE TABLE workforce_task_attempts (id TEXT PRIMARY KEY, task_id TEXT, container_id TEXT, status TEXT NOT NULL);
CREATE TABLE workforce_attempt_runtimes (id TEXT PRIMARY KEY, task_id TEXT, status TEXT NOT NULL);
CREATE TABLE workforce_delivery_attempts (id TEXT PRIMARY KEY, task_id TEXT, phase TEXT, push_started_at TIMESTAMPTZ, status TEXT NOT NULL);

CREATE TABLE workforce_result_materialization_runs (
  id TEXT PRIMARY KEY,
  task_turn_id TEXT,
  kind TEXT NOT NULL,
  input_identity_digest TEXT NOT NULL,
  output_id TEXT
);

CREATE TABLE workforce_integration_applications (
  id BIGINT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  name TEXT NOT NULL DEFAULT 'legacy',
  status TEXT NOT NULL DEFAULT 'ENABLED',
  credential_locator TEXT NOT NULL,
  credential_digest TEXT NOT NULL DEFAULT 'digest',
  credential_hint TEXT NOT NULL DEFAULT 'hint',
  webhook_url TEXT NOT NULL DEFAULT 'https://example.com/hook',
  signing_secret_ciphertext TEXT NOT NULL DEFAULT 'ciphertext',
  signing_secret_edek TEXT NOT NULL DEFAULT 'edek',
  signing_secret_wrap_key_ref TEXT NOT NULL DEFAULT 'key-ref',
  signing_secret_hint TEXT NOT NULL DEFAULT 'signing-hint',
  created_by BIGINT NOT NULL
  ,created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
  ,updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
  ,last_used_at TIMESTAMPTZ
);
INSERT INTO workforce_integration_applications(id,tenant_id,credential_locator,created_by)
VALUES (9001,1,'credential-legacy',101);

%s
`, legacySystemScopeTablesSQL())

func legacySystemScopeTablesSQL() string {
	var builder strings.Builder
	for _, table := range []string{
		"workforce_hook_packages",
		"workforce_hook_package_revisions",
		"workforce_hook_package_releases",
		"workforce_result_processor_templates",
		"workforce_result_processor_template_releases",
		"workforce_result_processors",
		"workforce_result_processor_revisions",
		"workforce_result_processor_releases",
	} {
		fmt.Fprintf(&builder, "CREATE TABLE %s (id TEXT PRIMARY KEY, tenant_id TEXT);\n", table)
	}
	return builder.String()
}
