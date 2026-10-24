package infrastructure

import (
	"os"
	"strings"
	"testing"
)

func TestRoleGrantRevisionMigrationsUseCamelCaseAndIdempotencyRecord(t *testing.T) {
	paths := []string{
		"../../../../migrations/mysql/20260719120000_role_grant_revision.sql",
		"../../../../migrations/postgres/20260719120000_role_grant_revision.sql",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read migration: %v", err)
			}
			text := string(payload)
			for _, required := range []string{"grantRevision", "sys_role_grant_request", "roleId", "idempotencyKey", "requestHash", "resultRevision"} {
				if !strings.Contains(text, required) {
					t.Fatalf("migration %s missing %q", path, required)
				}
			}
			for _, forbidden := range []string{"grant_revision", "role_id", "idempotency_key", "request_hash", "result_revision"} {
				if strings.Contains(text, forbidden) {
					t.Fatalf("migration %s contains non-camelCase field %q", path, forbidden)
				}
			}
		})
	}
}

func TestTemporaryPermissionReasonMigrationsAreEquivalent(t *testing.T) {
	paths := []string{
		"../../../../migrations/mysql/20260719121000_temporary_permission_reason.sql",
		"../../../../migrations/postgres/20260719121000_temporary_permission_reason.sql",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read migration: %v", err)
			}
			text := string(payload)
			for _, required := range []string{"sys_user_permission", "reason", "VARCHAR(500)", "-- +goose Up", "-- +goose Down"} {
				if !strings.Contains(text, required) {
					t.Fatalf("migration %s missing %q", path, required)
				}
			}
		})
	}
}
