package infrastructure

import (
	"strings"
	"testing"
)

func TestPlatformPostgresRendererQuotesReviewedIdentifiers(t *testing.T) {
	query := `
SELECT lm.platformCode, lm.providerCode, p.loginEnabled
FROM sys_platform_login_method lm
JOIN sys_external_login_provider p ON p.providerCode = lm.providerCode
WHERE lm.platformCode IN (?) AND lm.isDeleted = 0 AND p.isDeleted = 0`
	got := platformPostgresRenderer.RenderPostgres(query)
	for _, want := range []string{
		`lm."platformCode"`, `lm."providerCode"`, `p."loginEnabled"`,
		`FROM sys_platform_login_method lm`, `JOIN sys_external_login_provider p`,
		`lm."isDeleted" = 0`, `p."isDeleted" = 0`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered query missing %q:\n%s", want, got)
		}
	}
	for _, lowerSnakeTable := range []string{"sys_platform_login_method", "sys_external_login_provider"} {
		if strings.Contains(got, `"`+lowerSnakeTable+`"`) {
			t.Fatalf("lower snake table should not require PostgreSQL quoting: %s", got)
		}
	}
}

func TestPlatformPostgresRendererCoversReplaceStatements(t *testing.T) {
	queries := []string{
		`DELETE FROM sys_platform_login_method WHERE platformCode = ? AND isDeleted = 1`,
		`INSERT INTO sys_platform_source_rule (platformCode, matchType, matchValue, metadataJson, creatorId, createTime, isDeleted) VALUES (?, ?, ?, ?, ?, NOW(), 0)`,
		`UPDATE sys_platform_default_role SET isDeleted = 1, updaterId = ?, updateTime = NOW() WHERE platformCode = ? AND isDeleted = 0`,
	}
	for _, query := range queries {
		got := platformPostgresRenderer.RenderPostgres(query)
		for _, unquoted := range []string{
			"(platformCode", " platformCode =", " isDeleted =", " SET isDeleted",
		} {
			if strings.Contains(got, unquoted) {
				t.Fatalf("renderer left reviewed identifier %q unquoted:\n%s", unquoted, got)
			}
		}
	}
}
