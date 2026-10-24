package infrastructure

import (
	"os"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestScalarFoundationMigrationsStayDualDatabaseAndAssetFree(t *testing.T) {
	paths := []string{
		"../../../../../migrations/mysql/20260730100000_scalar_configuration_foundation.sql",
		"../../../../../migrations/postgres/20260730100000_scalar_configuration_foundation.sql",
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := string(content)
		for _, required := range []string{"uiWidget", "validationJson", "exposure", "sensitivity", "schemaVersion", "version", "oldValueProtected", "sys_dict_type", "colorToken"} {
			if !strings.Contains(sql, required) {
				t.Fatalf("%s does not contain %q", path, required)
			}
		}
		for _, forbidden := range []string{"CONFIG_ASSET", "sys_file_reference", "fileId"} {
			if strings.Contains(sql, forbidden) {
				t.Fatalf("%s unexpectedly contains forbidden asset contract %q", path, forbidden)
			}
		}
	}
}

func TestScalarFoundationMigrationsUseEquivalentFiniteDictionaryTokens(t *testing.T) {
	paths := []string{
		"../../../../../migrations/mysql/20260730100000_scalar_configuration_foundation.sql",
		"../../../../../migrations/postgres/20260730100000_scalar_configuration_foundation.sql",
	}
	colorTokens := []string{"gray", "blue", "pink", "green", "orange", "red", "purple"}
	iconTokens := []string{"unknown", "male", "female", "check", "close", "info"}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := string(content)
		for _, token := range append(colorTokens, iconTokens...) {
			if !strings.Contains(sql, "'"+token+"'") {
				t.Fatalf("%s does not enforce finite token %q", path, token)
			}
		}
	}
}

func TestScalarFoundationMigrationsSkipMalformedLegacyDictionaryJSON(t *testing.T) {
	mysqlPayload, err := os.ReadFile("../../../../../migrations/mysql/20260730100000_scalar_configuration_foundation.sql")
	if err != nil {
		t.Fatalf("read MySQL scalar migration: %v", err)
	}
	postgresPayload, err := os.ReadFile("../../../../../migrations/postgres/20260730100000_scalar_configuration_foundation.sql")
	if err != nil {
		t.Fatalf("read PostgreSQL scalar migration: %v", err)
	}
	if !strings.Contains(string(mysqlPayload), "JSON_VALID(extJson)") {
		t.Fatal("MySQL scalar migration must skip malformed legacy extJson")
	}
	postgresSQL := string(postgresPayload)
	for _, required := range []string{
		"pg_temp.dc23_try_jsonb",
		`pg_temp.dc23_try_jsonb("extJson"::text)`,
		`BTRIM("extJson"::text)`,
		"EXCEPTION WHEN others THEN",
		"parsed.payload IS NOT NULL",
		"-- +goose StatementBegin",
		"-- +goose StatementEnd",
	} {
		if !strings.Contains(postgresSQL, required) {
			t.Fatalf("PostgreSQL scalar migration lacks safe legacy JSON handling %q", required)
		}
	}
}

func TestMySQLScalarFoundationDownRemovesEveryAddedColumnForReUp(t *testing.T) {
	payload, err := os.ReadFile("../../../../../migrations/mysql/20260730100000_scalar_configuration_foundation.sql")
	if err != nil {
		t.Fatalf("read MySQL scalar migration: %v", err)
	}
	sql := string(payload)
	parts := strings.Split(sql, "-- +goose Down")
	if len(parts) != 2 {
		t.Fatalf("expected exactly one Goose Down section")
	}
	down := parts[1]
	for _, column := range []string{
		"uiWidget", "validationJson", "exposure", "sensitivity", "schemaVersion", "version",
		"oldValueProtected", "newValueProtected", "valueType", "colorToken", "iconToken",
		"presentationVersion",
	} {
		if !strings.Contains(down, "DROP COLUMN "+column) {
			t.Fatalf("MySQL Down must remove %s so a subsequent Up cannot hit duplicate columns", column)
		}
	}
}

func TestScalarFoundationMigrationDirectoriesRemainGooseParseable(t *testing.T) {
	for _, path := range []string{
		"../../../../../migrations/mysql",
		"../../../../../migrations/postgres",
	} {
		migrations, err := goose.CollectMigrations(path, 0, goose.MaxVersion)
		if err != nil {
			t.Fatalf("collect goose migrations in %s: %v", path, err)
		}
		if len(migrations) == 0 {
			t.Fatalf("expected goose migrations in %s", path)
		}
	}
}
