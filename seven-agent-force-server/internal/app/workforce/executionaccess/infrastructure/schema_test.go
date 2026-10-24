package infrastructure

import (
	"strings"
	"testing"
)

func TestSchemaUsesBigintRelationsWithoutForeignKeys(t *testing.T) {
	upper := strings.ToUpper(Schema)
	if strings.Contains(upper, "FOREIGN KEY") || strings.Contains(upper, "REFERENCES ") {
		t.Fatal("database foreign key introduced")
	}
	for _, column := range []string{"tenant_id BIGINT", "task_id BIGINT", "task_turn_id BIGINT", "attempt_id BIGINT", "operation_id BIGINT", "worker_id BIGINT", "actor_principal_id BIGINT"} {
		if !strings.Contains(Schema, column) {
			t.Fatalf("missing BIGINT relation %q", column)
		}
	}
	if strings.Contains(Schema, "command ") || strings.Contains(Schema, "cwd ") {
		t.Fatal("full descriptor field stored outside encrypted envelope")
	}
}

func TestSchemaHasOccurrenceDecisionAndActiveAttemptUniqueness(t *testing.T) {
	for _, contract := range []string{
		"UNIQUE(attempt_id,adapter_session_generation,local_tool_call_key)",
		"operation_id BIGINT NOT NULL UNIQUE",
		"workforce_execution_operation_one_active_attempt_idx",
		"revoke_request_digest",
		"agent_continued BOOLEAN NOT NULL DEFAULT FALSE",
		"result_output_ciphertext BYTEA",
		"native_tool_call_id TEXT NOT NULL CONSTRAINT workforce_execution_operations_native_tool_call_id_ck",
		"octet_length(native_tool_call_id) BETWEEN 1 AND 512",
		"native_tool_call_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$'",
	} {
		if !strings.Contains(Schema, contract) {
			t.Fatalf("missing schema contract %q", contract)
		}
	}
}

func TestClaimNextLocksFirstReadyOperationInCurrentExecutionScope(t *testing.T) {
	for _, contract := range []string{"tenant_id=$1", "task_id=$2", "attempt_id=$3", "adapter_session_generation=$4", "status='READY'", "ORDER BY id LIMIT 1", "FOR UPDATE SKIP LOCKED"} {
		if !strings.Contains(nextReadyOperationClause, contract) {
			t.Fatalf("claim-next query missing %q", contract)
		}
	}
}

func TestRecoverableClaimIncludesRunningAttemptForResultReadback(t *testing.T) {
	// A Worker may have executed the command and lost the Server response. The
	// same live owner must reclaim RUNNING so its durable result.json is read,
	// never execute the shell command a second time.
	for _, contract := range []string{"o.status IN ('DISPATCHED','RUNNING')", "a.status IN ('DISPATCHED','RUNNING')", "a.worker_instance_id=$7", "a.host_boot_id=$8", "a.lease_digest=$9", "a.lease_expires_at>CURRENT_TIMESTAMP"} {
		if !strings.Contains(recoverableClaimQuery, contract) {
			t.Fatalf("recoverable claim query missing %q", contract)
		}
	}
}

func TestExpiredClaimQueryFencesUnknownExecution(t *testing.T) {
	for _, contract := range []string{"o.status IN ('DISPATCHED','RUNNING')", "a.status IN ('DISPATCHED','RUNNING')", "a.lease_expires_at<=$1", "FOR UPDATE OF o,a SKIP LOCKED"} {
		if !strings.Contains(expiredClaimOperationQuery, contract) {
			t.Fatalf("expired claim query missing %q", contract)
		}
	}
}
