package infrastructure

import (
	"os"
	"strings"
	"testing"
)

func TestMySQLDictBaselineMigrationCreatesRuntimeSchemaAndGenderSeed(t *testing.T) {
	const path = "../../../../../migrations/mysql/20260719100000_system_dict_baseline.sql"
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	text := string(payload)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS sys_dict_type",
		"CREATE TABLE IF NOT EXISTS sys_dict_item",
		"dictTypeId",
		"requiredLogin",
		"dictCode = 'gender'",
		"JSON_OBJECT",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
	if strings.Contains(text, "DROP TABLE") {
		t.Fatal("baseline repair down migration must not drop pre-existing dictionary tables")
	}
}
