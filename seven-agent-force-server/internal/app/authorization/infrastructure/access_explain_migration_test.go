package infrastructure

import (
	"os"
	"strings"
	"testing"
)

func TestAccessExplainPermissionMigrationsUseStableRootIdentity(t *testing.T) {
	paths := []string{
		"../../../../migrations/mysql/20260718150000_access_explain_permissions.sql",
		"../../../../migrations/postgres/20260718150000_access_explain_permissions.sql",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read migration: %v", err)
			}
			text := string(payload)
			for _, required := range []string{"1900301062", "1900301063", "system:user:access:query", "system:user:access:explain", "AUTHORIZATION_ROOT", "systemKey"} {
				if !strings.Contains(text, required) {
					t.Fatalf("migration %s missing %q", path, required)
				}
			}
			if strings.Contains(text, "r.code = 'SUPER_ADMIN'") || strings.Contains(text, `r."code" = 'SUPER_ADMIN'`) {
				t.Fatalf("migration %s must not identify the root by configurable code", path)
			}
		})
	}
}
