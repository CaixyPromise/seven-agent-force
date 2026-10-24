package schemaalignment

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWorkforceMigrationsUseBoundedColumnTypesAndComments(t *testing.T) {
	migrations, err := filepath.Glob(filepath.Join("..", "..", "..", "..", "migrations", "postgres", "*workforce*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) == 0 {
		t.Fatal("Workforce migrations not found")
	}

	for _, migration := range migrations {
		body, readErr := os.ReadFile(migration)
		if readErr != nil {
			t.Fatal(readErr)
		}
		sql := string(body)
		upper := strings.ToUpper(sql)

		assertNoMigrationMatch(t, migration, upper, regexp.MustCompile(`\b[A-Z0-9_]*(?:CIPHERTEXT|EDEK)\s+TEXT\b`), "密文和数据密钥必须使用 BYTEA")
		assertNoMigrationMatch(t, migration, upper, regexp.MustCompile(`\b(?:VERSION|GENERATION|CANCEL_GENERATION|METADATA_VERSION|SEQUENCE|OWNER_EPOCH|PROVIDER_PROXY_OWNER_EPOCH|PENDING_OWNER_EPOCH|OLD_OWNER_EPOCH|NEW_OWNER_EPOCH|INVENTORY_REFRESH_GENERATION|HEARTBEAT_SEQUENCE|INVENTORY_VERSION|PRODUCER_SEQUENCE|REVISION_NUMBER|DRAFT_VERSION|EXPECTED_VERSION|ADAPTER_SESSION_GENERATION|ASSIGNMENT_GENERATION|REPOSITORY_CREDENTIAL_VERSION|SUBSCRIPTION_VERSION|CHANGE_SEQUENCE|WATERMARK|SOURCE_VERSION|DURATION_MS|DURATION_MILLIS|DURATION_MILLISECONDS)\s+BIGINT\b`), "有界数值必须使用 INTEGER")
		assertNoMigrationMatch(t, migration, upper, regexp.MustCompile(`\b(?:STATUS|[A-Z0-9_]+_STATUS|KIND|[A-Z0-9_]+_KIND|TYPE|[A-Z0-9_]+_TYPE|MODE|[A-Z0-9_]+_MODE|PROTOCOL|[A-Z0-9_]+_PROTOCOL|SCOPE_TYPE|DESIRED_STATE|CONSUMER_STATE|SESSION_STATE|ROLE|PHASE|NETWORK_POLICY|ADMISSION_STATUS|PRESENCE_STATUS|DELIVERY_DISPOSITION|RETRY_CLASS|TIMEOUT_STAGE|VALIDATION_STATUS|DECISION|VERDICT|SOURCE_REF_TYPE|REPOSITORY_PROVIDER_TYPE|REPOSITORY_SCHEME|DELIVERY_MODE|DELIVERY_AUTHORIZATION|OPERATION_KIND|RESULT_DELIVERY_STATUS|REASON_CODE|[A-Z0-9_]+_REASON_CODE|ERROR_CODE|[A-Z0-9_]+_ERROR_CODE|LAST_FAILURE_CLASS|FAILURE_CLASS|REJECTION_CODE)\s+TEXT\b`), "枚举和原因码必须使用有界 VARCHAR")

		createdTables := regexp.MustCompile(`(?m)CREATE TABLE(?: IF NOT EXISTS)?\s+([A-Z0-9_]+)`).FindAllStringSubmatch(upper, -1)
		for _, match := range createdTables {
			comment := "COMMENT ON TABLE " + match[1] + " IS"
			if !strings.Contains(upper, comment) {
				t.Errorf("%s: %s 缺少表注释", filepath.Base(migration), strings.ToLower(match[1]))
			}
		}
	}
}

// Occurrences can refer to native Question Snowflake IDs as well as counters.
func TestNotificationOccurrenceMigrationPreservesSnowflakeIdentity(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "postgres", "20260825130000_workforce_worker_fleet_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "source_occurrence BIGINT NOT NULL") {
		t.Fatal("notification occurrences must hold Snowflake identities")
	}
	if !strings.Contains(taskNotificationOccurrenceAlignmentSQL, "TYPE BIGINT") {
		t.Fatal("existing databases need repeatable widening")
	}
}

func assertNoMigrationMatch(t *testing.T, migration, sql string, pattern *regexp.Regexp, message string) {
	t.Helper()
	if match := pattern.FindString(sql); match != "" {
		t.Errorf("%s: %s：%s", filepath.Base(migration), message, strings.ToLower(match))
	}
}

func TestDeliveryCredentialBindingUsesGovernedProjectVersionReference(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "postgres", "20260825130000_workforce_worker_fleet_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "reauthorized_project_version_id BIGINT") {
		t.Fatal("binding needs a BIGINT ProjectVersion reference")
	}
	b, err = os.ReadFile("git_delivery.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ADD COLUMN IF NOT EXISTS reauthorized_project_version_id BIGINT") {
		t.Fatal("existing databases need idempotent alignment")
	}
}
