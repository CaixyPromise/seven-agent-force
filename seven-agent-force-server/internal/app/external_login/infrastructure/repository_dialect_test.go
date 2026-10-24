package infrastructure

import (
	"strings"
	"testing"
)

func TestExternalLoginPostgresRendererKeepsSnakeTablesBare(t *testing.T) {
	query := `
SELECT p.providerCode, i.externalSubject
FROM sys_external_login_provider p
JOIN sys_external_user_identity i ON i.providerCode = p.providerCode
WHERE p.displayEnabled = 1 AND i.isDeleted = 0`
	got := externalLoginPostgresRenderer.RenderPostgres(query)
	for _, want := range []string{
		`p."providerCode"`, `i."externalSubject"`, `i."isDeleted"`,
		`FROM sys_external_login_provider p`, `JOIN sys_external_user_identity i`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered query missing %q:\n%s", want, got)
		}
	}
	for _, unexpected := range []string{`"sys_external_login_provider"`, `"sys_external_user_identity"`} {
		if strings.Contains(got, unexpected) {
			t.Fatalf("lower snake table should not be quoted: %s", got)
		}
	}
}
