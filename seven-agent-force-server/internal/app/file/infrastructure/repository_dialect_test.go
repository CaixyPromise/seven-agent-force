package infrastructure

import (
	"strings"
	"testing"
)

func TestPrepareRepositoryQueryQuotesPostgresCamelCaseAndBooleans(t *testing.T) {
	source := `SELECT fileId, scopeId FROM sys_file_reference WHERE userId=? AND isDeleted=0 AND isEnabled = 1`
	got := prepareRepositoryQuery(source, true)
	want := `SELECT "fileId", "scopeId" FROM sys_file_reference WHERE "userId"=? AND "isDeleted"=FALSE AND "isEnabled" = TRUE`
	if got != want {
		t.Fatalf("prepareRepositoryQuery() = %q, want %q", got, want)
	}
	if mysql := prepareRepositoryQuery(source, false); mysql != source {
		t.Fatalf("MySQL query changed: %q", mysql)
	}
}

func TestPrepareRepositoryQueryPreservesHostileValuesAndComments(t *testing.T) {
	source := `SELECT fileId
FROM sys_file_reference
WHERE fileMetadata = '{"fileId":1,"isDeleted":0}'
  AND displayName = 'fileId isDeleted=0'
  -- fileId isDeleted=0
  /* scopeId isEnabled=1 */`
	got := prepareRepositoryQuery(source, true)
	for _, preserved := range []string{
		`'{"fileId":1,"isDeleted":0}'`,
		`'fileId isDeleted=0'`,
		`-- fileId isDeleted=0`,
		`/* scopeId isEnabled=1 */`,
	} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("SQL data/comment was rewritten; missing %q in %s", preserved, got)
		}
	}
	if !strings.HasPrefix(got, `SELECT "fileId"`+"\n"+`FROM sys_file_reference`) {
		t.Fatalf("reviewed SQL identifier was not quoted: %s", got)
	}
}

func TestDatabaseFlagScansMySQLAndPostgresValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "postgres-true", value: true, want: true},
		{name: "postgres-false", value: false, want: false},
		{name: "mysql-one", value: int64(1), want: true},
		{name: "mysql-zero", value: int64(0), want: false},
		{name: "text-true", value: []byte("true"), want: true},
		{name: "text-zero", value: []byte("0"), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var flag dbFlag
			if err := flag.Scan(test.value); err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if flag.Bool() != test.want {
				t.Fatalf("Bool() = %t, want %t", flag.Bool(), test.want)
			}
		})
	}
}
