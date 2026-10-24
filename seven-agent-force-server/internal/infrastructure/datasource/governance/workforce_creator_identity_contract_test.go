package governance

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWorkforceTaskCreatorIdentityIsNumericAndCreatorScoped(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	base, err := os.ReadFile(filepath.Join(root, "migrations", "postgres", "20260824120000_workforce_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	closure, err := os.ReadFile(filepath.Join(root, "migrations", "postgres", "20260825130000_workforce_worker_fleet_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(base), "created_by BIGINT NOT NULL") || !strings.Contains(string(base), "UNIQUE (tenant_id, created_by, idempotency_key)") {
		t.Fatal("fresh Task schema must use numeric ownership and creator-scoped idempotency")
	}
	if !strings.Contains(string(closure), "CREATE TABLE workforce_task_turns") || !strings.Contains(string(closure), "created_by BIGINT NOT NULL") {
		t.Fatal("fresh TaskTurn schema must use numeric ownership")
	}
	openIntegration, err := os.ReadFile(filepath.Join(root, "migrations", "postgres", "20260830100000_workforce_open_task_integration_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(openIntegration), "CREATE TABLE workforce_integration_applications") || !strings.Contains(string(openIntegration), "created_by BIGINT NOT NULL") {
		t.Fatal("Integration Application ownership must use the same numeric Framework user identity")
	}
	if !strings.Contains(string(openIntegration), "webhook_url VARCHAR(2048) NOT NULL DEFAULT ''") || strings.Contains(string(openIntegration), "webhook_url TEXT") {
		t.Fatal("Integration Application webhook URL must be an optional bounded VARCHAR(2048)")
	}
	if strings.Contains(string(openIntegration), "created_by_principal_id") {
		t.Fatal("Integration Application ownership must not be rewritten as a Workforce principal")
	}
}

func TestWorkforceFreshSchemaMatchesIdentityAlignmentContract(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
	base, err := os.ReadFile(filepath.Join(root, "migrations", "postgres", "20260824120000_workforce_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, constraint := range []string{"CONSTRAINT workforce_principals_type_ck", "CONSTRAINT workforce_principals_shape_ck"} {
		if !strings.Contains(string(base), constraint) {
			t.Fatalf("fresh Workforce principal schema is missing %s", constraint)
		}
	}
	closure, err := os.ReadFile(filepath.Join(root, "migrations", "postgres", "20260825130000_workforce_worker_fleet_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(closure), "CREATE TABLE workforce_task_input_snapshots")
	if start < 0 {
		t.Fatal("fresh schema is missing workforce_task_input_snapshots")
	}
	end := strings.Index(string(closure)[start:], ");")
	if end < 0 {
		t.Fatal("workforce_task_input_snapshots definition is incomplete")
	}
	definition := string(closure)[start : start+end]
	if !strings.Contains(definition, "workspace_source_revision_id BIGINT,") || strings.Contains(definition, "workspace_source_revision_id BIGINT NOT NULL") {
		t.Fatal("EMPTY_WORKSPACE must retain a nullable BIGINT workspace source revision relation")
	}
}
