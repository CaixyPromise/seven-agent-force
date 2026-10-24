package infrastructure

import (
	"strings"
	"testing"
)

func TestCredentialPostgresRendererCoversRepositoryIdentifiersAndBooleans(t *testing.T) {
	query := `SELECT userId, credentialType, mustChangePassword, isDeleted
FROM sys_user_credential
WHERE userId = ? AND credentialType = ? AND isDeleted = 0
ORDER BY createTime ASC`

	got := credentialPostgresRenderer.RenderPostgres(query)
	for _, want := range []string{
		`"userId"`, `"credentialType"`, `"mustChangePassword"`,
		`"isDeleted" = FALSE`, `"createTime"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("PostgreSQL credential query missing %q:\n%s", want, got)
		}
	}
}

func TestCredentialBooleanValuesAreDialectSafe(t *testing.T) {
	if got := (&UserCredentialRepository{postgres: true}).boolValue(true); got != true {
		t.Fatalf("PostgreSQL true value=%#v", got)
	}
	if got := (&UserCredentialRepository{}).boolValue(true); got != 1 {
		t.Fatalf("MySQL true value=%#v", got)
	}
	if got := (&UserCredentialRepository{}).boolValue(false); got != 0 {
		t.Fatalf("MySQL false value=%#v", got)
	}
}
