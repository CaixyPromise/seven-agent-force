package schemaalignment

import (
	"context"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/identitycontract"
	"github.com/jackc/pgx/v5"
)

// These unpublished runtime identities remain TEXT because they are owned by
// external Adapter or Worker process protocols, not by Workforce's entity
// model. Keeping the exceptions next to the unpublished alignment makes their
// scope explicit without treating opaque values as BIGINT relations.
var unpublishedSchemaTextIdentityExceptions = map[string]string{
	"workforce_execution_operation_attempts.worker_instance_id": "worker process-session identity",
	"workforce_execution_operation_attempts.host_boot_id":       "machine boot identity",
	"workforce_delivery_intents.native_tool_call_id":            "Adapter-native external Git delivery tool-call identity",
}

func governedTextIdentityExceptions() map[string]string {
	result := identitycontract.TextIdentityExceptions()
	for column, reason := range unpublishedSchemaTextIdentityExceptions {
		result[column] = reason
	}
	return result
}

func isGovernedTextIdentityException(column string) bool {
	if identitycontract.IsTextIdentityException(column) {
		return true
	}
	_, ok := unpublishedSchemaTextIdentityExceptions[column]
	return ok
}

func ensureGitDeliverySchema(ctx context.Context, tx pgx.Tx) error {
	statements := []string{
		`ALTER TABLE workforce_task_git_plans ADD COLUMN IF NOT EXISTS delivery_authorization VARCHAR(32) NOT NULL DEFAULT ''`,
		`ALTER TABLE workforce_task_git_plans ADD COLUMN IF NOT EXISTS delivery_authorized_by_principal_id BIGINT`,
		`ALTER TABLE workforce_task_git_plans ADD COLUMN IF NOT EXISTS delivery_authorized_at TIMESTAMPTZ`,
		`ALTER TABLE workforce_task_git_plans ADD COLUMN IF NOT EXISTS target_config_digest VARCHAR(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE workforce_task_git_plans ADD COLUMN IF NOT EXISTS push_authorization_digest VARCHAR(64) NOT NULL DEFAULT ''`,
		`ALTER TABLE workforce_task_git_plans ADD COLUMN IF NOT EXISTS review_authorization_digest VARCHAR(64) NOT NULL DEFAULT ''`,
		`UPDATE workforce_task_git_plans SET delivery_authorization=CASE WHEN delivery_mode='NONE' THEN '' ELSE 'ASK_AFTER_GATES' END,delivery_authorized_by_principal_id=NULL,delivery_authorized_at=NULL WHERE delivery_authorization NOT IN ('','AUTO_AFTER_GATES','ASK_AFTER_GATES') OR (delivery_mode='NONE' AND (delivery_authorization<>'' OR delivery_authorized_by_principal_id IS NOT NULL OR delivery_authorized_at IS NOT NULL)) OR (delivery_mode IN ('NEW_BRANCH','NEW_BRANCH_AND_DRAFT_REVIEW') AND delivery_authorization='') OR (delivery_authorization='AUTO_AFTER_GATES' AND (delivery_authorized_by_principal_id IS NULL OR delivery_authorized_at IS NULL)) OR (delivery_authorization='ASK_AFTER_GATES' AND (delivery_authorized_by_principal_id IS NOT NULL OR delivery_authorized_at IS NOT NULL))`,
		`UPDATE workforce_task_git_plans SET delivery_authorization='ASK_AFTER_GATES',delivery_authorized_by_principal_id=NULL,delivery_authorized_at=NULL WHERE delivery_authorization='AUTO_AFTER_GATES' AND (target_config_digest !~ '^[0-9a-f]{64}$' OR push_authorization_digest !~ '^[0-9a-f]{64}$' OR (delivery_mode='NEW_BRANCH' AND review_authorization_digest<>'') OR (delivery_mode='NEW_BRANCH_AND_DRAFT_REVIEW' AND review_authorization_digest !~ '^[0-9a-f]{64}$'))`,
		`UPDATE workforce_task_git_plans SET target_config_digest='',push_authorization_digest='',review_authorization_digest='' WHERE delivery_mode='NONE'`,
		`ALTER TABLE workforce_task_git_plans DROP CONSTRAINT IF EXISTS workforce_task_git_plans_delivery_authorization_ck`,
		`ALTER TABLE workforce_task_git_plans ADD CONSTRAINT workforce_task_git_plans_delivery_authorization_ck CHECK ((delivery_mode='NONE' AND delivery_authorization='' AND delivery_authorized_by_principal_id IS NULL AND delivery_authorized_at IS NULL AND target_config_digest='' AND push_authorization_digest='' AND review_authorization_digest='') OR (delivery_mode IN ('NEW_BRANCH','NEW_BRANCH_AND_DRAFT_REVIEW') AND ((delivery_authorization='ASK_AFTER_GATES' AND delivery_authorized_by_principal_id IS NULL AND delivery_authorized_at IS NULL AND (target_config_digest='' OR target_config_digest ~ '^[0-9a-f]{64}$') AND (push_authorization_digest='' OR push_authorization_digest ~ '^[0-9a-f]{64}$') AND (review_authorization_digest='' OR review_authorization_digest ~ '^[0-9a-f]{64}$')) OR (delivery_authorization='AUTO_AFTER_GATES' AND delivery_authorized_by_principal_id>0 AND delivery_authorized_at IS NOT NULL AND target_config_digest ~ '^[0-9a-f]{64}$' AND push_authorization_digest ~ '^[0-9a-f]{64}$' AND ((delivery_mode='NEW_BRANCH' AND review_authorization_digest='') OR (delivery_mode='NEW_BRANCH_AND_DRAFT_REVIEW' AND review_authorization_digest ~ '^[0-9a-f]{64}$'))))))`,
		`ALTER TABLE workforce_delivery_requests ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE workforce_delivery_requests ADD COLUMN IF NOT EXISTS reauthorized_project_version_id BIGINT`,
		`ALTER TABLE workforce_delivery_requests ADD COLUMN IF NOT EXISTS delivery_completed_notified_at TIMESTAMPTZ`,
		`DO $$ DECLARE item RECORD; BEGIN
			FOR item IN SELECT conname FROM pg_constraint WHERE conrelid='workforce_task_notification_records'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%fact_type%'
			LOOP EXECUTE format('ALTER TABLE workforce_task_notification_records DROP CONSTRAINT %I',item.conname); END LOOP;
			ALTER TABLE workforce_task_notification_records ADD CONSTRAINT workforce_task_notification_records_fact_type_check CHECK (fact_type IN ('TASK_ACTION_REQUIRED','TASK_INTERRUPTED','TASK_COMPLETED','DELIVERY_COMPLETED'));
		END $$`,
		`UPDATE workforce_delivery_requests SET version=1 WHERE version IS NULL OR version<=0`,
		`ALTER TABLE workforce_delivery_requests DROP CONSTRAINT IF EXISTS workforce_delivery_requests_version_ck`,
		`ALTER TABLE workforce_delivery_requests ADD CONSTRAINT workforce_delivery_requests_version_ck CHECK (version>0)`,
		`CREATE TABLE IF NOT EXISTS workforce_delivery_intents (
			id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL, task_turn_id BIGINT NOT NULL,
			attempt_id BIGINT NOT NULL, adapter_session_generation BIGINT NOT NULL, local_tool_call_key TEXT NOT NULL,
			native_tool_call_id TEXT NOT NULL, round_key TEXT NOT NULL, summary VARCHAR(2048) NOT NULL,
			summary_digest VARCHAR(64) NOT NULL, git_plan_digest VARCHAR(64) NOT NULL, delivery_mode TEXT NOT NULL,
			delivery_authorization VARCHAR(32) NOT NULL, delivery_request_id BIGINT, status VARCHAR(32) NOT NULL,
			version INTEGER NOT NULL DEFAULT 1, adapter_received_at TIMESTAMPTZ, closed_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (attempt_id,adapter_session_generation,local_tool_call_key), UNIQUE (task_turn_id))`,
		`CREATE TABLE IF NOT EXISTS workforce_delivery_decisions (
			id BIGINT PRIMARY KEY, tenant_id BIGINT NOT NULL, task_id BIGINT NOT NULL, delivery_request_id BIGINT NOT NULL,
			action VARCHAR(32) NOT NULL, expected_version INTEGER NOT NULL, principal_id BIGINT NOT NULL, idempotency_key VARCHAR(128) NOT NULL,
			request_digest VARCHAR(64) NOT NULL, authorization_digest VARCHAR(64) NOT NULL,
			reason VARCHAR(2048) NOT NULL DEFAULT '', applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (tenant_id,principal_id,idempotency_key))`,
		`ALTER TABLE workforce_delivery_intents DROP CONSTRAINT IF EXISTS workforce_delivery_intents_identity_ck`,
		`ALTER TABLE workforce_delivery_intents ADD CONSTRAINT workforce_delivery_intents_identity_ck CHECK (id>0 AND tenant_id>0 AND task_id>0 AND task_turn_id>0 AND attempt_id>0 AND adapter_session_generation>0 AND (delivery_request_id IS NULL OR delivery_request_id>0) AND version>0)`,
		`ALTER TABLE workforce_delivery_intents DROP CONSTRAINT IF EXISTS workforce_delivery_intents_tool_identity_ck`,
		`ALTER TABLE workforce_delivery_intents ADD CONSTRAINT workforce_delivery_intents_tool_identity_ck CHECK (octet_length(local_tool_call_key) BETWEEN 1 AND 128 AND octet_length(native_tool_call_id) BETWEEN 1 AND 512 AND octet_length(round_key) BETWEEN 1 AND 128)`,
		`ALTER TABLE workforce_delivery_intents DROP CONSTRAINT IF EXISTS workforce_delivery_intents_summary_ck`,
		`ALTER TABLE workforce_delivery_intents ADD CONSTRAINT workforce_delivery_intents_summary_ck CHECK (summary=btrim(summary) AND octet_length(summary) BETWEEN 1 AND 2048)`,
		`ALTER TABLE workforce_delivery_intents DROP CONSTRAINT IF EXISTS workforce_delivery_intents_digest_ck`,
		`ALTER TABLE workforce_delivery_intents ADD CONSTRAINT workforce_delivery_intents_digest_ck CHECK (summary_digest ~ '^[0-9a-f]{64}$' AND git_plan_digest ~ '^[0-9a-f]{64}$')`,
		`ALTER TABLE workforce_delivery_intents DROP CONSTRAINT IF EXISTS workforce_delivery_intents_mode_ck`,
		`ALTER TABLE workforce_delivery_intents ADD CONSTRAINT workforce_delivery_intents_mode_ck CHECK (delivery_mode IN ('NEW_BRANCH','NEW_BRANCH_AND_DRAFT_REVIEW'))`,
		`ALTER TABLE workforce_delivery_intents DROP CONSTRAINT IF EXISTS workforce_delivery_intents_authorization_ck`,
		`ALTER TABLE workforce_delivery_intents ADD CONSTRAINT workforce_delivery_intents_authorization_ck CHECK (delivery_authorization IN ('AUTO_AFTER_GATES','ASK_AFTER_GATES'))`,
		`ALTER TABLE workforce_delivery_intents DROP CONSTRAINT IF EXISTS workforce_delivery_intents_status_ck`,
		`WITH desired AS (
			SELECT i.id,CASE
				WHEN r.status IN ('WAITING_APPROVAL','MANUAL_ONLY') THEN 'WAITING_USER_DECISION'
				WHEN r.status IN ('READY','DISPATCHED','RUNNING','CONFIRMING','RETRY_WAIT','CANCELLING') THEN 'DELIVERY_RUNNING'
				WHEN r.status='OUTCOME_UNKNOWN' THEN 'OUTCOME_UNKNOWN'
				WHEN r.status IN ('FAILED','MANUAL_REVIEW','DENIED','EXPIRED') THEN 'DELIVERY_FAILED'
				WHEN r.status='CANCELLED' AND r.reason_code='USER_SKIPPED' THEN 'SKIPPED'
				WHEN r.status='CANCELLED' THEN 'CANCELLED'
				WHEN r.status='SUCCEEDED' AND i.delivery_mode<>'NEW_BRANCH_AND_DRAFT_REVIEW' THEN 'DELIVERED'
				WHEN r.status='SUCCEEDED' THEN 'DELIVERY_RUNNING'
				ELSE i.status END AS status
			FROM workforce_delivery_intents i
			JOIN workforce_delivery_requests r ON r.id=i.delivery_request_id)
		UPDATE workforce_delivery_intents i SET status=d.status,version=i.version+1,updated_at=CURRENT_TIMESTAMP
		FROM desired d WHERE i.id=d.id AND i.status<>d.status`,
		`WITH desired AS (
			SELECT i.id,CASE
				WHEN o.status='SUCCEEDED' THEN 'DELIVERED'
				WHEN o.status='OUTCOME_UNKNOWN' THEN 'OUTCOME_UNKNOWN'
				WHEN o.status IN ('FAILED','MANUAL_REVIEW','CANCELLED','DENIED','EXPIRED') THEN 'DELIVERY_FAILED'
				ELSE i.status END AS status
			FROM workforce_delivery_intents i
			LEFT JOIN LATERAL (SELECT status FROM workforce_delivery_operations WHERE delivery_request_id=i.delivery_request_id AND operation_kind='CREATE_PULL_REQUEST' ORDER BY id DESC LIMIT 1) o ON true
			WHERE i.delivery_mode='NEW_BRANCH_AND_DRAFT_REVIEW' AND i.status='DELIVERY_RUNNING')
		UPDATE workforce_delivery_intents i SET status=d.status,version=i.version+1,updated_at=CURRENT_TIMESTAMP
		FROM desired d WHERE i.id=d.id AND i.status<>d.status`,
		`ALTER TABLE workforce_delivery_intents ADD CONSTRAINT workforce_delivery_intents_status_ck CHECK (status IN ('SCHEDULED','WAITING_FINALIZATION','WAITING_CHECKS','WAITING_REVIEW','WAITING_USER_DECISION','DELIVERY_RUNNING','DELIVERED','DELIVERY_FAILED','OUTCOME_UNKNOWN','SKIPPED','CANCELLED'))`,
		`ALTER TABLE workforce_delivery_decisions DROP CONSTRAINT IF EXISTS workforce_delivery_decisions_identity_ck`,
		`ALTER TABLE workforce_delivery_decisions ADD CONSTRAINT workforce_delivery_decisions_identity_ck CHECK (id>0 AND tenant_id>0 AND task_id>0 AND delivery_request_id>0 AND expected_version>0 AND principal_id>0)`,
		`ALTER TABLE workforce_delivery_decisions DROP CONSTRAINT IF EXISTS workforce_delivery_decisions_action_ck`,
		`ALTER TABLE workforce_delivery_decisions ADD CONSTRAINT workforce_delivery_decisions_action_ck CHECK (action IN ('AUTO_FROM_TASK_PLAN','DELIVER','SKIP_THIS_RESULT','RESTART_DELIVERY'))`,
		`ALTER TABLE workforce_delivery_decisions DROP CONSTRAINT IF EXISTS workforce_delivery_decisions_idempotency_ck`,
		`ALTER TABLE workforce_delivery_decisions ADD CONSTRAINT workforce_delivery_decisions_idempotency_ck CHECK (octet_length(idempotency_key) BETWEEN 1 AND 128)`,
		`ALTER TABLE workforce_delivery_decisions DROP CONSTRAINT IF EXISTS workforce_delivery_decisions_digest_ck`,
		`ALTER TABLE workforce_delivery_decisions ADD CONSTRAINT workforce_delivery_decisions_digest_ck CHECK (request_digest ~ '^[0-9a-f]{64}$' AND authorization_digest ~ '^[0-9a-f]{64}$')`,
		`ALTER TABLE workforce_delivery_decisions DROP CONSTRAINT IF EXISTS workforce_delivery_decisions_reason_ck`,
		`ALTER TABLE workforce_delivery_decisions ADD CONSTRAINT workforce_delivery_decisions_reason_ck CHECK (octet_length(reason)<=2048)`,
		`CREATE INDEX IF NOT EXISTS workforce_delivery_intents_task_idx ON workforce_delivery_intents (tenant_id,task_id,status,id DESC)`,
		`CREATE INDEX IF NOT EXISTS workforce_delivery_intents_request_idx ON workforce_delivery_intents (delivery_request_id) WHERE delivery_request_id IS NOT NULL`,
		`CREATE INDEX IF NOT EXISTS workforce_delivery_decisions_request_idx ON workforce_delivery_decisions (delivery_request_id,id DESC)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS workforce_delivery_requests_id_version_uidx ON workforce_delivery_requests (id,version)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("align Batch 5 Git delivery schema: %w", err)
		}
	}
	return nil
}

func verifyGitDeliverySchema(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	identityColumns := map[string][]string{
		"workforce_delivery_intents":   {"id", "tenant_id", "task_id", "task_turn_id", "attempt_id", "delivery_request_id"},
		"workforce_delivery_decisions": {"id", "tenant_id", "task_id", "delivery_request_id", "principal_id"},
		"workforce_task_git_plans":     {"delivery_authorized_by_principal_id"},
		"workforce_delivery_requests":  {"reauthorized_project_version_id"},
	}
	for table, columns := range identityColumns {
		for _, column := range columns {
			var dataType string
			if err := q.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, column).Scan(&dataType); err != nil {
				return fmt.Errorf("Git delivery column %s.%s mismatch: %w", table, column, err)
			}
			if dataType != "bigint" {
				return fmt.Errorf("Git delivery column %s.%s type=%s, want bigint", table, column, dataType)
			}
		}
	}
	integerColumns := map[string][]string{
		"workforce_delivery_intents":   {"adapter_session_generation", "version"},
		"workforce_delivery_decisions": {"expected_version"},
		"workforce_delivery_requests":  {"version"},
	}
	for table, columns := range integerColumns {
		for _, column := range columns {
			var dataType string
			if err := q.QueryRow(ctx, `SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2`, table, column).Scan(&dataType); err != nil {
				return fmt.Errorf("Git delivery column %s.%s mismatch: %w", table, column, err)
			}
			if dataType != "integer" {
				return fmt.Errorf("Git delivery column %s.%s type=%s, want integer", table, column, dataType)
			}
		}
	}
	constraints := map[string][]string{
		"workforce_task_git_plans_delivery_authorization_ck": {"delivery_mode", "delivery_authorization", "AUTO_AFTER_GATES", "ASK_AFTER_GATES", "delivery_authorized_by_principal_id", "delivery_authorized_at", "target_config_digest", "push_authorization_digest", "review_authorization_digest", "0-9a-f", "64"},
		"workforce_delivery_requests_version_ck":             {"version", "> 0"},
		"workforce_delivery_intents_identity_ck":             {"adapter_session_generation", "delivery_request_id", "version"},
		"workforce_delivery_intents_tool_identity_ck":        {"local_tool_call_key", "native_tool_call_id", "round_key", "octet_length", "128", "512"},
		"workforce_delivery_intents_summary_ck":              {"summary", "btrim", "octet_length", "2048"},
		"workforce_delivery_intents_digest_ck":               {"summary_digest", "git_plan_digest", "0-9a-f", "64"},
		"workforce_delivery_intents_mode_ck":                 {"NEW_BRANCH", "NEW_BRANCH_AND_DRAFT_REVIEW"},
		"workforce_delivery_intents_authorization_ck":        {"AUTO_AFTER_GATES", "ASK_AFTER_GATES"},
		"workforce_delivery_intents_status_ck":               {"WAITING_FINALIZATION", "WAITING_USER_DECISION", "DELIVERY_RUNNING", "DELIVERED", "DELIVERY_FAILED", "OUTCOME_UNKNOWN", "SKIPPED", "CANCELLED"},
		"workforce_delivery_decisions_identity_ck":           {"delivery_request_id", "expected_version", "principal_id"},
		"workforce_delivery_decisions_action_ck":             {"AUTO_FROM_TASK_PLAN", "DELIVER", "SKIP_THIS_RESULT", "RESTART_DELIVERY"},
		"workforce_delivery_decisions_idempotency_ck":        {"idempotency_key", "octet_length", "128"},
		"workforce_delivery_decisions_digest_ck":             {"request_digest", "authorization_digest", "0-9a-f", "64"},
		"workforce_delivery_decisions_reason_ck":             {"reason", "octet_length", "2048"},
	}
	for name, tokens := range constraints {
		var definition string
		if err := q.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE connamespace='public'::regnamespace AND conname=$1 AND contype='c' AND convalidated`, name).Scan(&definition); err != nil {
			return fmt.Errorf("Git delivery constraint %s mismatch: %w", name, err)
		}
		for _, token := range tokens {
			if !strings.Contains(definition, token) {
				return fmt.Errorf("Git delivery constraint %s is missing %q", name, token)
			}
		}
	}
	indexes := map[string][]string{
		"workforce_delivery_intents_task_idx":         {"tenant_id", "task_id", "status", "id DESC"},
		"workforce_delivery_intents_request_idx":      {"delivery_request_id", "WHERE", "IS NOT NULL"},
		"workforce_delivery_decisions_request_idx":    {"delivery_request_id", "id DESC"},
		"workforce_delivery_requests_id_version_uidx": {"UNIQUE", "id", "version"},
	}
	for name, tokens := range indexes {
		var definition string
		if err := q.QueryRow(ctx, `SELECT pg_get_indexdef(i.oid) FROM pg_class i JOIN pg_index x ON x.indexrelid=i.oid WHERE i.relnamespace='public'::regnamespace AND i.relname=$1 AND x.indisvalid AND x.indisready`, name).Scan(&definition); err != nil {
			return fmt.Errorf("Git delivery index %s mismatch: %w", name, err)
		}
		for _, token := range tokens {
			if !strings.Contains(definition, token) {
				return fmt.Errorf("Git delivery index %s is missing %q", name, token)
			}
		}
	}
	checks := []string{
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_delivery_requests r LEFT JOIN workforce_project_versions v ON v.id=r.reauthorized_project_version_id WHERE r.reauthorized_project_version_id IS NOT NULL AND (v.id IS NULL OR v.tenant_id<>r.tenant_id OR v.project_id<>r.project_id OR r.reauthorized_at IS NULL OR r.reauthorized_by_principal_id IS NULL))`,
		`SELECT NOT EXISTS(SELECT 1 FROM pg_constraint WHERE contype='f' AND conrelid IN ('workforce_delivery_intents'::regclass,'workforce_delivery_decisions'::regclass,'workforce_delivery_requests'::regclass,'workforce_task_git_plans'::regclass))`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_delivery_intents i LEFT JOIN workforce_tasks t ON t.id=i.task_id LEFT JOIN workforce_task_turns tr ON tr.id=i.task_turn_id WHERE t.id IS NULL OR t.tenant_id<>i.tenant_id OR tr.id IS NULL OR tr.task_id<>i.task_id OR tr.tenant_id<>i.tenant_id)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_delivery_intents i LEFT JOIN workforce_task_attempts a ON a.id=i.attempt_id WHERE a.id IS NULL OR a.task_id<>i.task_id OR a.task_turn_id<>i.task_turn_id)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_delivery_intents i LEFT JOIN workforce_delivery_requests r ON r.id=i.delivery_request_id WHERE i.delivery_request_id IS NOT NULL AND (r.id IS NULL OR r.tenant_id<>i.tenant_id OR r.task_id<>i.task_id OR r.task_turn_id<>i.task_turn_id))`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_delivery_decisions d LEFT JOIN workforce_delivery_requests r ON r.id=d.delivery_request_id LEFT JOIN workforce_principals p ON p.id=d.principal_id WHERE r.id IS NULL OR r.tenant_id<>d.tenant_id OR r.task_id<>d.task_id OR p.id IS NULL OR p.principal_type<>'USER')`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_task_git_plans g LEFT JOIN workforce_principals p ON p.id=g.delivery_authorized_by_principal_id WHERE g.delivery_authorized_by_principal_id IS NOT NULL AND (p.id IS NULL OR p.principal_type<>'USER'))`,
	}
	for index, query := range checks {
		var ok bool
		if err := q.QueryRow(ctx, query).Scan(&ok); err != nil {
			return fmt.Errorf("verify Git delivery relationship %d: %w", index+1, err)
		}
		if !ok {
			return fmt.Errorf("Git delivery relationship integrity mismatch: check %d", index+1)
		}
	}
	return nil
}
