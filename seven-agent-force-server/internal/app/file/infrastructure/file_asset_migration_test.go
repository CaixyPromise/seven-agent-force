package infrastructure

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileAssetCredentialMigrationsAreEquivalentAndFailClosed(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	mysqlPayload, err := os.ReadFile(filepath.Join(root, "migrations", "mysql", "20260730120000_file_asset_credentials.sql"))
	if err != nil {
		t.Fatalf("read MySQL migration: %v", err)
	}
	postgresPayload, err := os.ReadFile(filepath.Join(root, "migrations", "postgres", "20260730120000_file_asset_credentials.sql"))
	if err != nil {
		t.Fatalf("read PostgreSQL migration: %v", err)
	}

	for _, token := range []string{
		"scopeId",
		"credentialId",
		"credentialVersion",
		"protectedUntil",
		"credentialExpireAt",
		"revokedAt",
		"uploadTaskId",
		"uk_file_reference_active_slot",
		"idx_upload_credential_authority",
	} {
		if !strings.Contains(string(mysqlPayload), token) {
			t.Errorf("MySQL migration missing %q", token)
		}
		if !strings.Contains(string(postgresPayload), token) {
			t.Errorf("PostgreSQL migration missing %q", token)
		}
	}
	if !strings.Contains(string(mysqlPayload), "credentialVersion INT NOT NULL DEFAULT 0") {
		t.Fatal("MySQL historical tasks must default to authority version zero")
	}
	if !strings.Contains(string(postgresPayload), `"credentialVersion" integer NOT NULL DEFAULT 0`) {
		t.Fatal("PostgreSQL historical tasks must default to authority version zero")
	}
	if !strings.Contains(string(postgresPayload), `WHERE "isDeleted" = false`) {
		t.Fatal("PostgreSQL active reference uniqueness must be partial")
	}
	if !strings.Contains(string(mysqlPayload), "ELSE NULL") {
		t.Fatal("MySQL active reference uniqueness must allow multiple deleted history rows")
	}
	mysqlDown := strings.SplitN(string(mysqlPayload), "-- +goose Down", 2)
	if len(mysqlDown) != 2 || !strings.Contains(mysqlDown[1], "CASE WHEN isDeleted = 0 THEN bizId ELSE NULL END") {
		t.Fatal("MySQL rollback uniqueness must preserve multiple deleted replacement rows")
	}
	if strings.Contains(mysqlDown[1], "UNIQUE KEY uk_user_biz_active (userId, bizType, bizId, isDeleted)") {
		t.Fatal("MySQL rollback must not restore history-incompatible uniqueness")
	}
	postgresDown := strings.SplitN(string(postgresPayload), "-- +goose Down", 2)
	if len(postgresDown) != 2 || !strings.Contains(postgresDown[1], `WHERE "isDeleted" = false`) {
		t.Fatal("PostgreSQL rollback uniqueness must remain active-only")
	}
}

func TestUploadBindingChannelExpansionMigrationsAreEquivalentAndForwardOnly(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	mysqlPayload, err := os.ReadFile(filepath.Join(root, "migrations", "mysql", "20260731130000_expand_upload_binding_channel.sql"))
	if err != nil {
		t.Fatalf("read MySQL upload binding channel migration: %v", err)
	}
	postgresPayload, err := os.ReadFile(filepath.Join(root, "migrations", "postgres", "20260731130000_expand_upload_binding_channel.sql"))
	if err != nil {
		t.Fatalf("read PostgreSQL upload binding channel migration: %v", err)
	}

	for dialect, payload := range map[string]string{
		"mysql":    string(mysqlPayload),
		"postgres": string(postgresPayload),
	} {
		sections := strings.SplitN(payload, "-- +goose Down", 2)
		if len(sections) != 2 {
			t.Fatalf("%s migration must declare an explicit Down section", dialect)
		}
		if !strings.Contains(strings.ToLower(sections[1]), "forward-only") {
			t.Fatalf("%s destructive Down must fail forward-only", dialect)
		}
		upper := strings.ToUpper(sections[0])
		for _, forbidden := range []string{"CREATE TABLE", "FOREIGN KEY", "RENAME TABLE", "RENAME COLUMN", "INSERT INTO", "UPDATE "} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("%s protected-batch expansion must not contain %q", dialect, forbidden)
			}
		}
	}
	if !strings.Contains(string(mysqlPayload), "ALTER TABLE sys_upload_task\n  MODIFY COLUMN bindingChannel VARCHAR(64)") {
		t.Fatal("MySQL expansion must widen the existing bindingChannel in place to VARCHAR(64)")
	}
	if !strings.Contains(string(postgresPayload), "ALTER TABLE \"sys_upload_task\"\n  ALTER COLUMN \"bindingChannel\" TYPE character varying(64)") {
		t.Fatal("PostgreSQL expansion must widen the existing bindingChannel in place to character varying(64)")
	}
}
