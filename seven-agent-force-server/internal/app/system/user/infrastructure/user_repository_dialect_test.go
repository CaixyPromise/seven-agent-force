package infrastructure

import (
	"strings"
	"testing"
)

func TestPostgresGenderProjectionKeepsNumericAdminContract(t *testing.T) {
	t.Parallel()

	repository := &Repository{postgres: true}
	rendered := userPostgresRenderer.RenderPostgres(repository.genderProjection("u.userGender"))
	if !strings.Contains(rendered, `CASE WHEN u."userGender" THEN 1 ELSE 0 END AS "userGender"`) {
		t.Fatalf("unexpected PostgreSQL gender projection: %s", rendered)
	}
}

func TestMySQLGenderProjectionUsesStoredNumericColumn(t *testing.T) {
	t.Parallel()

	repository := &Repository{}
	if got, want := repository.genderProjection("u.userGender"), "u.userGender"; got != want {
		t.Fatalf("genderProjection() = %q, want %q", got, want)
	}
}
