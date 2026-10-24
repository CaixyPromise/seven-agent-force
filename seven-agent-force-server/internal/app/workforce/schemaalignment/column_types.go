package schemaalignment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// alignWorkforceColumnTypes applies the current unpublished schema's storage
// rules to an existing development database.
func alignWorkforceColumnTypes(ctx context.Context, tx pgx.Tx) error {
	if err := alignTaskNotificationOccurrence(ctx, tx); err != nil {
		return err
	}
	const legacyTaskNotificationDispatchIndex = "CREATE INDEX workforce_task_notification_records_dispatch_idx ON public.workforce_task_notification_records USING btree (status, next_retry_at, lease_until, occurred_at) WHERE (status = ANY (ARRAY['PENDING'::text, 'PROCESSING'::text, 'RETRY_WAIT'::text]))"
	var rebuildTaskNotificationDispatchIndex bool
	if err := tx.QueryRow(ctx, `SELECT COALESCE(pg_get_indexdef(to_regclass('public.workforce_task_notification_records_dispatch_idx'))=$1,false)`, legacyTaskNotificationDispatchIndex).Scan(&rebuildTaskNotificationDispatchIndex); err != nil {
		return fmt.Errorf("inspect legacy Task notification dispatch index: %w", err)
	}
	var rebuildTaskNotificationChecks bool
	if err := tx.QueryRow(ctx, `WITH expected(name,definition) AS (VALUES
		('workforce_task_notification_records_status_check','CHECK ((status = ANY (ARRAY[''PENDING''::text, ''PROCESSING''::text, ''RETRY_WAIT''::text, ''PUBLISHED''::text, ''SUPPRESSED''::text, ''FAILED''::text])))'),
		('workforce_task_notification_records_fact_type_check','CHECK ((fact_type = ANY (ARRAY[''TASK_ACTION_REQUIRED''::text, ''TASK_INTERRUPTED''::text, ''TASK_COMPLETED''::text, ''DELIVERY_COMPLETED''::text])))'),
		('workforce_task_notification_records_source_type_check','CHECK ((source_type = ANY (ARRAY[''CONTINUATION_CHECKPOINT''::text, ''FINALIZATION_RUN''::text, ''ARTIFACT_UPLOAD''::text, ''RESULT_EVALUATION''::text, ''TASK_ATTEMPT''::text, ''DELIVERY_REQUEST''::text, ''REVIEW_OPERATION''::text, ''INDEPENDENT_REVIEW_RUN''::text, ''RUNTIME_RECOVERY''::text, ''TASK_TIMEOUT''::text, ''TASK_CANCEL''::text])))'),
		('workforce_task_notification_records_focus_check','CHECK ((focus = ANY (ARRAY[''evaluation''::text, ''result''::text, ''continuation''::text, ''delivery''::text])))'),
		('workforce_task_notification_records_check','CHECK (((status = ''SUPPRESSED''::text) OR ((subscription_id IS NOT NULL) AND (subscription_version IS NOT NULL))))')
	)
	SELECT count(*)=5
	FROM expected e
	JOIN pg_constraint c ON c.conrelid=to_regclass('public.workforce_task_notification_records') AND c.conname=e.name
	WHERE pg_get_constraintdef(c.oid)=e.definition`).Scan(&rebuildTaskNotificationChecks); err != nil {
		return fmt.Errorf("inspect legacy Task notification checks: %w", err)
	}

	statements := []string{
		`DO $types$
		DECLARE column_record record;
		BEGIN
			FOR column_record IN
				SELECT table_name,column_name
				FROM information_schema.columns
				WHERE table_schema='public'
				  AND table_name LIKE 'workforce\_%' ESCAPE '\'
				  AND data_type='bigint'
				  AND column_name=ANY(ARRAY[
					'version','generation','owner_epoch','provider_proxy_owner_epoch','pending_owner_epoch',
					'old_owner_epoch','new_owner_epoch','inventory_refresh_generation','heartbeat_sequence',
					'inventory_version','producer_sequence','revision_number','draft_version','expected_version',
					'adapter_session_generation','assignment_generation','repository_credential_version',
					'subscription_version','change_sequence','watermark','source_version',
					'cancel_generation','metadata_version','sequence','duration_ms','duration_millis','duration_milliseconds'
				  ])
			LOOP
				EXECUTE format('ALTER TABLE %I.%I ALTER COLUMN %I TYPE INTEGER USING %I::INTEGER',
					'public',column_record.table_name,column_record.column_name,column_record.column_name);
			END LOOP;
		END
		$types$`,
		`DO $types$
		DECLARE column_record record; target_type text;
		BEGIN
			FOR column_record IN
				SELECT table_name,column_name
				FROM information_schema.columns
				WHERE table_schema='public'
				  AND table_name LIKE 'workforce\_%' ESCAPE '\'
				  AND data_type='text'
				  AND column_name=ANY(ARRAY[
					'status','current_source_state','repository_last_check_status','recovery_previous_status','kind','type','mode','protocol','adapter_protocol','scope_type','desired_state','consumer_state',
					'session_state','role','phase','network_policy','admission_status','presence_status',
					'delivery_disposition','retry_class','timeout_stage','validation_status','decision','verdict',
					'source_ref_type','source_type','source_kind','output_kind','artifact_type','resource_type','event_type','ref_type','requested_ref_type','provider_type','action_type','fact_type','channel_kind','review_revision_kind','repository_provider_type','repository_scheme','delivery_mode',
					'delivery_authorization','operation_kind','result_delivery_status','reason_code','error_code',
					'wait_reason_code','last_retry_reason_code','release_reason_code','terminal_reason_code','repository_last_check_reason_code','closure_reason_code','last_error_code',
					'last_failure_class','failure_class','rejection_code','approval_mode','end_reason','scheduling_scope','isolation_profile','provider_proxy_control_state','http_method','category','expected_decision','archive_format','storage','action','focus','recipient_role','personal_channel_policy','report_verdict','assurance_level','ai_verdict','quality_outcome','warning_code'
				  ])
			LOOP
				target_type := CASE WHEN column_record.column_name LIKE '%reason_code' OR column_record.column_name LIKE '%error_code' OR column_record.column_name IN ('failure_class','last_failure_class','rejection_code','warning_code','end_reason') THEN 'VARCHAR(128)' ELSE 'VARCHAR(64)' END;
				EXECUTE format('ALTER TABLE %I.%I ALTER COLUMN %I TYPE %s USING %I::%s',
					'public',column_record.table_name,column_record.column_name,target_type,column_record.column_name,target_type);
			END LOOP;
		END
		$types$`,
		`DO $types$
		DECLARE column_record record;
		BEGIN
			FOR column_record IN
				SELECT table_name,column_name
				FROM information_schema.columns
				WHERE table_schema='public'
				  AND table_name LIKE 'workforce\_%' ESCAPE '\'
				  AND data_type='text'
				  AND column_name=ANY(ARRAY['media_type','artifact_media_type','content_type'])
			LOOP
				EXECUTE format('ALTER TABLE %I.%I ALTER COLUMN %I TYPE VARCHAR(255) USING %I::VARCHAR(255)',
					'public',column_record.table_name,column_record.column_name,column_record.column_name);
			END LOOP;
		END
		$types$`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("align Workforce column types: %w", err)
		}
	}
	if rebuildTaskNotificationDispatchIndex {
		if _, err := tx.Exec(ctx, `DROP INDEX workforce_task_notification_records_dispatch_idx;
			CREATE INDEX workforce_task_notification_records_dispatch_idx
			ON workforce_task_notification_records (status,next_retry_at,lease_until,occurred_at)
			WHERE status IN ('PENDING','PROCESSING','RETRY_WAIT')`); err != nil {
			return fmt.Errorf("rebuild Task notification dispatch index after status conversion: %w", err)
		}
	}
	if rebuildTaskNotificationChecks {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_task_notification_records
			DROP CONSTRAINT workforce_task_notification_records_status_check,
			DROP CONSTRAINT workforce_task_notification_records_fact_type_check,
			DROP CONSTRAINT workforce_task_notification_records_source_type_check,
			DROP CONSTRAINT workforce_task_notification_records_focus_check,
			DROP CONSTRAINT workforce_task_notification_records_check,
			ADD CONSTRAINT workforce_task_notification_records_status_check CHECK (status IN ('PENDING','PROCESSING','RETRY_WAIT','PUBLISHED','SUPPRESSED','FAILED')),
			ADD CONSTRAINT workforce_task_notification_records_fact_type_check CHECK (fact_type IN ('TASK_ACTION_REQUIRED','TASK_INTERRUPTED','TASK_COMPLETED','DELIVERY_COMPLETED')),
			ADD CONSTRAINT workforce_task_notification_records_source_type_check CHECK (source_type IN ('CONTINUATION_CHECKPOINT','FINALIZATION_RUN','ARTIFACT_UPLOAD','RESULT_EVALUATION','TASK_ATTEMPT','DELIVERY_REQUEST','REVIEW_OPERATION','INDEPENDENT_REVIEW_RUN','RUNTIME_RECOVERY','TASK_TIMEOUT','TASK_CANCEL')),
			ADD CONSTRAINT workforce_task_notification_records_focus_check CHECK (focus IN ('evaluation','result','continuation','delivery')),
			ADD CONSTRAINT workforce_task_notification_records_check CHECK (status='SUPPRESSED' OR (subscription_id IS NOT NULL AND subscription_version IS NOT NULL))`); err != nil {
			return fmt.Errorf("rebuild Task notification checks after status conversion: %w", err)
		}
	}

	projectVersionsPresent, err := relationExists(ctx, tx, "workforce_project_versions")
	if err != nil {
		return err
	}
	if projectVersionsPresent {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_project_versions DROP CONSTRAINT IF EXISTS workforce_project_versions_repository_shape_ck`); err != nil {
			return fmt.Errorf("drop ProjectVersion repository constraint before credential conversion: %w", err)
		}
	}
	taskGitPlansPresent, err := relationExists(ctx, tx, "workforce_task_git_plans")
	if err != nil {
		return err
	}
	if taskGitPlansPresent {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_task_git_plans
			ALTER COLUMN repository_credential_ciphertext DROP DEFAULT,
			ALTER COLUMN repository_credential_edek DROP DEFAULT`); err != nil {
			return fmt.Errorf("drop Task Git credential defaults before conversion: %w", err)
		}
	}

	binaryColumns := []struct{ table, column string }{
		{"workforce_integration_signing_credentials", "signing_secret_ciphertext"},
		{"workforce_integration_signing_credentials", "signing_secret_edek"},
		{"workforce_project_versions", "repository_credential_ciphertext"},
		{"workforce_project_versions", "repository_credential_edek"},
		{"workforce_task_git_plans", "repository_credential_ciphertext"},
		{"workforce_task_git_plans", "repository_credential_edek"},
		{"workforce_execution_operations", "descriptor_ciphertext"},
		{"workforce_execution_operations", "descriptor_edek"},
		{"workforce_execution_operations", "result_output_ciphertext"},
		{"workforce_execution_operations", "result_output_edek"},
	}
	for _, column := range binaryColumns {
		if err := alterColumnType(ctx, tx, column.table, column.column, "BYTEA", "bytea", fmt.Sprintf("decode(%s, 'base64')", pgx.Identifier{column.column}.Sanitize())); err != nil {
			return fmt.Errorf("align %s.%s: %w", column.table, column.column, err)
		}
	}
	if taskGitPlansPresent {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_task_git_plans
			ALTER COLUMN repository_credential_ciphertext SET DEFAULT '\x'::bytea,
			ALTER COLUMN repository_credential_edek SET DEFAULT '\x'::bytea`); err != nil {
			return fmt.Errorf("restore Task Git credential defaults after conversion: %w", err)
		}
	}
	if projectVersionsPresent {
		if _, err := tx.Exec(ctx, `ALTER TABLE workforce_project_versions
			ADD CONSTRAINT workforce_project_versions_repository_shape_ck CHECK (
				(kind='CODEBASE' AND repository_scheme IN ('http','https') AND btrim(repository_host)<>'' AND repository_port BETWEEN 1 AND 65535 AND btrim(repository_path)<>'' AND btrim(repository_display_url)<>'' AND ((repository_credential_ciphertext IS NULL AND repository_credential_edek IS NULL AND repository_credential_wrap_key_ref IS NULL AND repository_credential_version IS NULL) OR (octet_length(repository_credential_ciphertext)>0 AND octet_length(repository_credential_edek)>0 AND btrim(repository_credential_wrap_key_ref)<>'' AND repository_credential_version>0)) AND repository_config_digest ~ '^[0-9a-f]{64}$' AND btrim(default_ref_name)<>'')
				OR
				(kind='GENERAL' AND repository_provider_type IS NULL AND repository_scheme IS NULL AND repository_host IS NULL AND repository_port IS NULL AND repository_path IS NULL AND repository_display_url IS NULL AND repository_web_url IS NULL AND repository_credential_ciphertext IS NULL AND repository_credential_edek IS NULL AND repository_credential_wrap_key_ref IS NULL AND repository_credential_version IS NULL AND repository_config_digest IS NULL AND repository_capabilities_json IS NULL AND repository_last_check_status IS NULL AND repository_last_check_reason_code IS NULL AND repository_last_checked_at IS NULL AND default_ref_name IS NULL)
			)`); err != nil {
			return fmt.Errorf("restore ProjectVersion repository constraint after credential conversion: %w", err)
		}
	}
	return nil
}

// A notification occurrence may identify a native Question or Delivery Intent,
// not just an event sequence. Keep the exact Snowflake identity as BIGINT.
// The caller owns the transaction; repeated alignment preserves existing rows.
func alignTaskNotificationOccurrence(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, taskNotificationOccurrenceAlignmentSQL)
	if err != nil {
		return fmt.Errorf("align Task notification occurrence identity: %w", err)
	}
	return nil
}

const taskNotificationOccurrenceAlignmentSQL = `DO $occurrence$
DECLARE existing_type text;
BEGIN
	SELECT data_type INTO existing_type FROM information_schema.columns
	WHERE table_schema=current_schema() AND table_name='workforce_task_notification_records' AND column_name='source_occurrence';
	IF existing_type='integer' THEN
		ALTER TABLE workforce_task_notification_records ALTER COLUMN source_occurrence TYPE BIGINT USING source_occurrence::BIGINT;
	ELSIF existing_type IS NOT NULL AND existing_type<>'bigint' THEN
		RAISE EXCEPTION 'Task notification occurrence identity type is unsupported';
	END IF;
END
$occurrence$`
