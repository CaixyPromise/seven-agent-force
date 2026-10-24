package schemaalignment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const projectFirstWorkerMigration = "20260825130000_workforce_worker_fleet_mvp.sql"

func readProjectFirstMigration(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "migrations", "postgres", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return strings.ToUpper(string(body))
}

func migrationUpAndDown(t *testing.T, body string) (string, string) {
	t.Helper()
	parts := strings.Split(body, "-- +GOOSE DOWN")
	if len(parts) != 2 {
		t.Fatalf("expected exactly one Goose Down boundary, got %d", len(parts)-1)
	}
	return parts[0], parts[1]
}

func TestProjectFirstMigrationUsesImmutableProjectVersions(t *testing.T) {
	worker := readProjectFirstMigration(t, projectFirstWorkerMigration)
	up, down := migrationUpAndDown(t, worker)

	for _, token := range []string{
		"CREATE TABLE WORKFORCE_PROJECTS",
		"PROJECT_KEY TEXT NOT NULL",
		"CURRENT_VERSION_ID BIGINT NOT NULL",
		"CREATE TABLE WORKFORCE_PROJECT_VERSIONS",
		"DEPARTMENT_ID BIGINT CONSTRAINT WORKFORCE_PROJECT_VERSIONS_DEPARTMENT_ID_CK CHECK (DEPARTMENT_ID IS NULL OR DEPARTMENT_ID>0)",
		"KIND VARCHAR(32) NOT NULL",
		"REPOSITORY_DISPLAY_URL TEXT",
		"REPOSITORY_CREDENTIAL_CIPHERTEXT BYTEA",
		"REPOSITORY_CONFIG_DIGEST VARCHAR(64)",
		"TECH_STACK_JSON JSONB NOT NULL",
		"COMPLETION_CHECK_DIGEST VARCHAR(64) NOT NULL",
		"RESULT_PROCESSOR_RELEASE_ID BIGINT NOT NULL",
		"REVIEW_PROMPT TEXT NOT NULL DEFAULT ''",
		"HOOK_RELEASE_IDS BIGINT[] NOT NULL DEFAULT '{}'",
		"DELIVERY_AUTHORIZATION VARCHAR(32) NOT NULL",
		"DELIVERY_AUTHORIZED_BY_PRINCIPAL_ID BIGINT",
		"NOTIFICATION_GROUP_ROUTE_ID BIGINT",
		"ADD COLUMN PROJECT_ID BIGINT NOT NULL",
		"ADD COLUMN PROJECT_VERSION_ID BIGINT NOT NULL",
		"ADD COLUMN REVIEW_ENABLED BOOLEAN NOT NULL DEFAULT TRUE",
		"ADD COLUMN SELECTED_RUNTIME_RELEASE_ID BIGINT",
		"ADD COLUMN EXECUTION_TIMEOUT_SECONDS INTEGER",
		"ADD COLUMN EXECUTION_ACTIVE_MILLISECONDS BIGINT NOT NULL DEFAULT 0",
		"CONSTRAINT WORKFORCE_TASKS_EXECUTION_TIMEOUT_SECONDS_CK CHECK (EXECUTION_TIMEOUT_SECONDS IS NULL OR EXECUTION_TIMEOUT_SECONDS>0)",
		"COMMENT ON COLUMN WORKFORCE_TASKS.EXECUTION_TIMEOUT_SECONDS",
		"WORKFORCE_PROJECT_VERSION_IMMUTABLE",
		"CREATE INDEX WORKFORCE_TASKS_PROJECT_CREATED_IDX",
	} {
		if !strings.Contains(up, token) {
			t.Errorf("Project-first migration Up is missing %q", token)
		}
	}

	projectStart := strings.Index(up, "CREATE TABLE WORKFORCE_PROJECTS")
	projectEnd := strings.Index(up[projectStart:], "CREATE TABLE WORKFORCE_TASK_INPUT_SNAPSHOTS")
	if projectStart < 0 || projectEnd < 0 {
		t.Fatal("cannot isolate workforce_projects DDL")
	}
	projectBlock := up[projectStart : projectStart+projectEnd]
	if strings.Contains(projectBlock, "FOREIGN KEY") || strings.Contains(projectBlock, "REFERENCES ") {
		t.Fatal("Project tables must use application-owned relationship validation, not database foreign keys")
	}

	for _, token := range []string{
		"DROP COLUMN IF EXISTS REVIEW_ENABLED",
		"DROP COLUMN IF EXISTS EXECUTION_TIMEOUT_SECONDS",
		"DROP COLUMN IF EXISTS EXECUTION_ACTIVE_MILLISECONDS",
		"DROP COLUMN IF EXISTS PROJECT_VERSION_ID",
		"DROP COLUMN IF EXISTS PROJECT_ID",
		"DROP TABLE IF EXISTS WORKFORCE_PROJECT_VERSIONS",
		"DROP TABLE IF EXISTS WORKFORCE_PROJECTS",
	} {
		if !strings.Contains(down, token) {
			t.Errorf("Project-first migration Down is missing %q", token)
		}
	}
}

func TestProjectHookHardCutRemovesBundleAndBindingProducts(t *testing.T) {
	migrationsDir := filepath.Join("..", "..", "..", "..", "migrations", "postgres")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	var combined strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		combined.Write(body)
	}
	upper := strings.ToUpper(combined.String())
	for _, forbidden := range []string{
		"CREATE TABLE WORKFORCE_HOOK_BUNDLES",
		"CREATE TABLE WORKFORCE_HOOK_BUNDLE_RELEASES",
		"CREATE TABLE WORKFORCE_HOOK_BINDINGS",
		"CREATE TABLE WORKFORCE_HOOK_EXECUTION_PROFILES",
		"CREATE TABLE WORKFORCE_GOLDEN_VERIFICATION_RUNS",
		"CREATE TABLE WORKFORCE_INTEGRATED_VERIFICATION_RUNS",
		"/WORKFORCE/HOOK-BUNDLES",
	} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("Hook product hard cut still contains %q", forbidden)
		}
	}
}

func TestProjectRepositoryHardCutRemovesRepositoryConnectionProduct(t *testing.T) {
	migrationsDir := filepath.Join("..", "..", "..", "..", "migrations", "postgres")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	var combined strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		combined.Write(body)
	}
	upper := strings.ToUpper(combined.String())
	for _, forbidden := range []string{
		"CREATE TABLE WORKFORCE_REPOSITORY_CONNECTIONS",
		"CREATE TABLE WORKFORCE_REPOSITORY_ACCESS_BINDINGS",
		"CREATE TABLE WORKFORCE_REPOSITORY_CREDENTIAL_EVENTS",
		"CREATE TABLE WORKFORCE_REPOSITORY_LIFECYCLE_EVENTS",
		"/WORKFORCE/REPOSITORIES",
		"REPOSITORY_CONNECTION_ID",
		"SOURCE_REPOSITORY_CONNECTION_ID",
		"TARGET_REPOSITORY_CONNECTION_ID",
	} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("repository product hard cut still contains %q", forbidden)
		}
	}
}

func TestProjectFirstMigrationMovesInfrastructureSelectionToAssignmentAndAttempt(t *testing.T) {
	base := readProjectFirstMigration(t, "20260824120000_workforce_mvp.sql")
	provider := readProjectFirstMigration(t, "20260824123000_workforce_provider.sql")
	hooks := readProjectFirstMigration(t, "20260824124000_workforce_managed_hooks.sql")
	worker := readProjectFirstMigration(t, projectFirstWorkerMigration)
	all := strings.Join([]string{base, provider, hooks, worker}, "\n")

	for _, token := range []string{
		"SCHEDULING_SCOPE VARCHAR(32) NOT NULL DEFAULT 'PRODUCTION'",
		"SCHEDULING_PRIORITY INTEGER NOT NULL DEFAULT 100",
		"ENVIRONMENT_CAPABILITIES_JSON JSONB NOT NULL",
		"EXECUTION_KINDS_JSON JSONB NOT NULL",
		"AGENT_RUNTIME_RELEASE_ID BIGINT NOT NULL",
		"ADAPTER_IDENTITY TEXT NOT NULL",
		"MODEL_CREDENTIAL_ID BIGINT NOT NULL",
		"CREDENTIAL_VERSION INTEGER NOT NULL",
		"MODEL_PROTOCOL VARCHAR(16) NOT NULL",
		"PRIMARY_MODEL VARCHAR(256) NOT NULL",
		"RESOLVED_EXECUTION_SNAPSHOT_JSON JSONB NOT NULL",
		"SELECTION_DIGEST VARCHAR(64) NOT NULL",
		"ADD COLUMN ASSIGNMENT_SELECTION_DIGEST VARCHAR(64) NOT NULL",
	} {
		if !strings.Contains(all, token) {
			t.Errorf("runtime/provider routing persistence is missing %q", token)
		}
	}

	for _, forbidden := range []string{
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN HARNESS_ALIAS",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN HARNESS_RELEASE_ID",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN PROVIDER_ALIAS",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN PROVIDER_MODEL",
		"ROUTING_PRIORITY INTEGER NOT NULL DEFAULT 100",
		"EXECUTION_ROLES_JSON JSONB NOT NULL",
		"MAX_CONCURRENT_INVOCATIONS INTEGER NOT NULL DEFAULT 2",
		"PROVIDER_PROFILE_ID BIGINT NOT NULL",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN HOOK_SELECTION_MODE",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN HOOK_BUNDLE_RELEASE_ID",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN EXECUTION_SNAPSHOT_JSON",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN HARNESS_TIMEOUT_SECONDS",
		"ALTER TABLE WORKFORCE_TASKS ADD COLUMN RESULT_PROCESSOR_SELECTION_MODE",
		"CREATE TABLE WORKFORCE_TASK_EXECUTION_SNAPSHOTS",
		"ALTER TABLE WORKFORCE_TASK_EXECUTION_SNAPSHOTS",
	} {
		if strings.Contains(all, forbidden) {
			t.Errorf("Project-first hard cut retains forbidden Task infrastructure selection %q", forbidden)
		}
	}
}

func TestModelCredentialMigrationUsesBoundedAndBinaryTypes(t *testing.T) {
	provider := readProjectFirstMigration(t, "20260824123000_workforce_provider.sql")
	up, _ := migrationUpAndDown(t, provider)

	for _, token := range []string{
		"ID BIGINT PRIMARY KEY",
		"SECRET_CIPHERTEXT BYTEA",
		"SECRET_EDEK BYTEA",
		"SECRET_VERSION INTEGER NOT NULL",
		"VERSION INTEGER NOT NULL",
		"PROTOCOL_INVOCATION_KEY VARCHAR(64) NOT NULL",
		"GENERATION INTEGER NOT NULL",
		"OWNER_EPOCH INTEGER NOT NULL",
		"CREDENTIAL_VERSION INTEGER NOT NULL",
		"REQUEST_BODY_SHA256 BYTEA NOT NULL",
		"NONCE_SHA256 BYTEA NOT NULL",
		"UPSTREAM_STATUS_CODE SMALLINT",
		"DURATION_MILLISECONDS INTEGER",
		"INPUT_TOKENS INTEGER",
		"TOTAL_TOKENS INTEGER",
		"STATUS IN ('ACTIVE', 'DISABLED', 'DELETED')",
		"WHERE STATUS<>'DELETED'",
	} {
		if !strings.Contains(up, token) {
			t.Errorf("model credential migration is missing %q", token)
		}
	}
	for _, forbidden := range []string{
		"INVOCATION_ID TEXT PRIMARY KEY",
		"SECRET_CIPHERTEXT TEXT",
		"SECRET_EDEK TEXT",
		"DURATION_MILLISECONDS BIGINT",
		"INPUT_TOKENS BIGINT",
		"TOTAL_TOKENS BIGINT",
	} {
		if strings.Contains(up, forbidden) {
			t.Errorf("model credential migration contains %q", forbidden)
		}
	}
}

func TestProjectFirstMigrationRemovesResultProcessorTemplateLayer(t *testing.T) {
	migrationsDir := filepath.Join("..", "..", "..", "..", "migrations", "postgres")
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	var combined strings.Builder
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(migrationsDir, entry.Name()))
		if readErr != nil {
			t.Fatal(readErr)
		}
		combined.Write(body)
	}
	registry, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "governance", "future_table_registry.csv"))
	if err != nil {
		t.Fatal(err)
	}
	combined.Write(registry)
	upper := strings.ToUpper(combined.String())
	for _, forbidden := range []string{
		"WORKFORCE_RESULT_PROCESSOR_TEMPLATES",
		"WORKFORCE_RESULT_PROCESSOR_TEMPLATE_RELEASES",
		"TEMPLATE_RELEASE_ID",
		"/WORKFORCE/RESULT-PROCESSOR-TEMPLATES",
	} {
		if strings.Contains(upper, forbidden) {
			t.Errorf("Result Processor Template layer still contains %q", forbidden)
		}
	}
	if !strings.Contains(upper, "WORKFORCE_PROJECTS,WF23,WORKFORCE") {
		t.Fatal("schema governance registry does not own workforce_projects")
	}
}

func TestProjectFirstNavigationUsesProjectScopedTaskEntryPoints(t *testing.T) {
	rbac := readProjectFirstMigration(t, "20260824121000_workforce_rbac_seed.sql")
	for _, required := range []string{
		"'/WORKFORCE/PROJECTS'",
		"'/WORKFORCE/TASKS'",
		"'/WORKFORCE/ACTIONS'",
		"'/WORKFORCE/PROJECTS/{PROJECTID}/TASKS'",
	} {
		if !strings.Contains(rbac, required) {
			t.Errorf("Project-first navigation or permission is missing %q", required)
		}
	}
	if strings.Contains(rbac, "'POST', '/WORKFORCE/TASKS'") {
		t.Fatal("legacy global Task creation permission remains in the unpublished migration")
	}
}

func TestProjectFirstMigrationRejectsNonemptyLegacyBusinessSchema(t *testing.T) {
	worker := readProjectFirstMigration(t, projectFirstWorkerMigration)
	up, down := migrationUpAndDown(t, worker)
	for _, token := range []string{
		"CREATE OR REPLACE FUNCTION WORKFORCE_ASSERT_PROJECT_HARD_CUT_READY()",
		"'WORKFORCE_TASKS'",
		"'WORKFORCE_TASK_ATTEMPTS'",
		"SELECT EXISTS (SELECT 1 FROM PUBLIC.%I LIMIT 1)",
		"MESSAGE = 'PROJECT_HARD_CUT_RESET_REQUIRED'",
		"SELECT WORKFORCE_ASSERT_PROJECT_HARD_CUT_READY();",
	} {
		if !strings.Contains(up, token) {
			t.Errorf("Project-first hard-cut preflight is missing %q", token)
		}
	}
	if !strings.Contains(down, "DROP FUNCTION IF EXISTS WORKFORCE_ASSERT_PROJECT_HARD_CUT_READY();") {
		t.Fatal("Project-first migration Down does not remove its preflight function")
	}
}
