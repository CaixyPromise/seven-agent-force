package schemaalignment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	executionaccessinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/infrastructure"
)

func TestExecutionAccessSchemaUsesBigintWithoutForeignKeys(t *testing.T) {
	schema := strings.ToUpper(executionaccessinfra.Schema)
	for _, table := range []string{"WORKFORCE_EXECUTION_OPERATIONS", "WORKFORCE_EXECUTION_OPERATION_DECISIONS", "WORKFORCE_EXECUTION_OPERATION_ATTEMPTS", "WORKFORCE_TASK_NETWORK_GRANTS"} {
		if !strings.Contains(schema, "CREATE TABLE IF NOT EXISTS "+table) {
			t.Fatalf("missing %s", table)
		}
	}
	if strings.Contains(schema, "FOREIGN KEY") || strings.Contains(schema, "REFERENCES ") {
		t.Fatal("execution access schema must not create database foreign keys")
	}
}

func TestBatch4MigrationContainsExecutionAccessSchema(t *testing.T) {
	path := filepath.Join("..", "..", "..", "..", "migrations", "postgres", "20260825130000_workforce_worker_fleet_mvp.sql")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	migration := strings.ToUpper(string(body))
	for _, table := range []string{"WORKFORCE_EXECUTION_OPERATIONS", "WORKFORCE_EXECUTION_OPERATION_DECISIONS", "WORKFORCE_EXECUTION_OPERATION_ATTEMPTS", "WORKFORCE_TASK_NETWORK_GRANTS"} {
		if !strings.Contains(migration, "CREATE TABLE IF NOT EXISTS "+table) || !strings.Contains(migration, "DROP TABLE IF EXISTS "+table) {
			t.Fatalf("migration is missing up/down contract for %s", table)
		}
	}
	for _, contract := range []string{
		"NATIVE_TOOL_CALL_ID TEXT NOT NULL CONSTRAINT WORKFORCE_EXECUTION_OPERATIONS_NATIVE_TOOL_CALL_ID_CK",
		"SOURCE_EXECUTION_OPERATION_ID BIGINT",
		"WORKFORCE_TURN_EXECUTION_OPERATION_ONCE_IDX",
		"WHERE SOURCE_EXECUTION_OPERATION_ID IS NOT NULL",
		"DROP COLUMN IF EXISTS SOURCE_EXECUTION_OPERATION_ID",
	} {
		if !strings.Contains(migration, contract) {
			t.Fatalf("migration is missing cold Operation continuation contract %q", contract)
		}
	}
	start := strings.Index(migration, "-- A COLD OPERATION RESUME")
	end := strings.Index(migration, "-- +GOOSE DOWN")
	if start < 0 || end <= start {
		t.Fatal("cold Operation continuation migration block is missing")
	}
	continuationBlock := migration[start:end]
	if strings.Contains(continuationBlock, "FOREIGN KEY") || strings.Contains(continuationBlock, " REFERENCES ") {
		t.Fatal("cold Operation continuation must not add a database foreign key")
	}
}

func TestExecutionAccessContinuationAlignmentContractsAreExplicit(t *testing.T) {
	path := filepath.Join("execution_access.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToUpper(string(body))
	if strings.Contains(text, "P.TENANT_ID") {
		t.Fatal("execution access alignment assumes workforce_principals has a tenant_id column")
	}
	for _, contract := range []string{
		"ADD COLUMN IF NOT EXISTS NATIVE_TOOL_CALL_ID TEXT",
		"SET NATIVE_TOOL_CALL_ID=LOCAL_TOOL_CALL_KEY WHERE NATIVE_TOOL_CALL_ID IS NULL",
		"WORKFORCE_EXECUTION_OPERATIONS_NATIVE_TOOL_CALL_ID_CK",
		"ADD COLUMN IF NOT EXISTS SOURCE_EXECUTION_OPERATION_ID BIGINT",
		"WORKFORCE_TURN_EXECUTION_OPERATION_ONCE_IDX",
		"C.PURPOSE<>'INTERACTION_SUSPEND'",
		"C.OPERATION_ID<>TR.SOURCE_EXECUTION_OPERATION_ID",
		"C.CONSUMED_BY_TURN_ID<>TR.ID",
	} {
		if !strings.Contains(text, contract) {
			t.Fatalf("alignment is missing cold Operation contract %q", contract)
		}
	}
	legacyBody, err := os.ReadFile(filepath.Join("alignment_postgres_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.ToUpper(string(legacyBody))
	for _, contract := range []string{
		"DROP INDEX WORKFORCE_TURN_EXECUTION_OPERATION_ONCE_IDX",
		"DROP CONSTRAINT WORKFORCE_TASK_TURNS_SOURCE_EXECUTION_OPERATION_CK",
		"DROP COLUMN SOURCE_EXECUTION_OPERATION_ID",
	} {
		if !strings.Contains(legacy, contract) {
			t.Fatalf("legacy alignment fixture does not remove cold Operation contract %q", contract)
		}
	}
}
