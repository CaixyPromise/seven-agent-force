package store

import (
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestPostgresRendererPreservesCamelCaseSchemaNames(t *testing.T) {
	db := sqlx.NewDb(nil, "postgres")
	query := `INSERT INTO sysNotificationRecipient (notificationId, userId, mailboxVersion) VALUES (?, ?, ?)`
	renderer := MustNewPostgresRenderer([]string{
		"sysNotificationRecipient", "notificationId", "userId", "mailboxVersion",
	})
	quoted := renderer.Render(db, query)
	for _, expected := range []string{
		`"sysNotificationRecipient"`,
		`"notificationId"`,
		`"userId"`,
		`"mailboxVersion"`,
	} {
		if !strings.Contains(quoted, expected) {
			t.Fatalf("quoted SQL missing %s: %s", expected, quoted)
		}
	}
	if strings.Contains(quoted, `"sysNotification"Recipient`) {
		t.Fatalf("long table name was partially quoted: %s", quoted)
	}
}

func TestPostgresRendererLeavesMySQLQueryUntouched(t *testing.T) {
	db := sqlx.NewDb(nil, "mysql")
	query := `SELECT notificationId FROM sysNotification`
	renderer := MustNewPostgresRenderer([]string{"sysNotification", "notificationId"})
	if got := renderer.Render(db, query); got != query {
		t.Fatalf("MySQL query changed: %s", got)
	}
}

func TestPostgresRendererDoesNotRewriteSQLDataOrComments(t *testing.T) {
	query := `SELECT notificationId
FROM sysNotification
WHERE payloadJson = '{"notificationId":"literal"}'
  AND eventKey = 'notificationId'
  -- notificationId in an operator note
  /* notificationId in a block note */`
	got := MustNewPostgresRenderer([]string{"sysNotification", "notificationId"}).RenderPostgres(query)
	for _, preserved := range []string{
		`'{"notificationId":"literal"}'`,
		`'notificationId'`,
		`-- notificationId in an operator note`,
		`/* notificationId in a block note */`,
	} {
		if !strings.Contains(got, preserved) {
			t.Fatalf("SQL data/comment was rewritten; missing %q in %s", preserved, got)
		}
	}
	if !strings.HasPrefix(got, `SELECT "notificationId"`+"\n"+`FROM "sysNotification"`) {
		t.Fatalf("reviewed SQL identifiers were not quoted: %s", got)
	}
}

func TestPostgresRendererMatchesWholeTokensOnly(t *testing.T) {
	query := `SELECT notificationId, notificationIdentifier FROM sysNotificationArchive`
	got := MustNewPostgresRenderer([]string{"sysNotification", "notificationId"}).RenderPostgres(query)
	want := `SELECT "notificationId", notificationIdentifier FROM sysNotificationArchive`
	if got != want {
		t.Fatalf("RenderPostgres() = %q, want %q", got, want)
	}
}

func TestPostgresRendererPreservesDollarQuotedBodiesAndConvertsReviewedBooleans(t *testing.T) {
	renderer := MustNewPostgresRenderer(
		[]string{"isDeleted", "payloadJson"},
		"isDeleted",
	)
	query := `SELECT payloadJson FROM sys_file_info
WHERE isDeleted = 0
  AND payloadJson = $body${"isDeleted":1}$body$`
	got := renderer.RenderPostgres(query)
	want := `SELECT "payloadJson" FROM sys_file_info
WHERE "isDeleted" = FALSE
  AND "payloadJson" = $body${"isDeleted":1}$body$`
	if got != want {
		t.Fatalf("RenderPostgres() = %q, want %q", got, want)
	}
}

func TestPostgresRendererUsesStandardConformingStringBoundaries(t *testing.T) {
	renderer := MustNewPostgresRenderer([]string{"sysNotification", "notificationId"})
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{
			name:  "standard string does not treat backslash as quote escape",
			query: `SELECT notificationId FROM sysNotification WHERE note = 'trailing\' AND notificationId = 1`,
			want:  `SELECT "notificationId" FROM "sysNotification" WHERE note = 'trailing\' AND "notificationId" = 1`,
		},
		{
			name:  "escape string retains backslash quote semantics",
			query: `SELECT notificationId FROM sysNotification WHERE note = E'quoted\'value' AND notificationId = 1`,
			want:  `SELECT "notificationId" FROM "sysNotification" WHERE note = E'quoted\'value' AND "notificationId" = 1`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := renderer.RenderPostgres(test.query); got != test.want {
				t.Fatalf("RenderPostgres() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPostgresRendererRejectsUnsafeAllowlistEntries(t *testing.T) {
	if _, err := NewPostgresRenderer([]string{"safeId", `unsafe"; DROP TABLE sys_user; --`}); err == nil {
		t.Fatal("expected unsafe identifier allowlist entry to be rejected")
	}
	if _, err := NewPostgresRenderer([]string{"isDeleted"}, "unreviewedFlag"); err == nil {
		t.Fatal("expected boolean column outside identifier allowlist to be rejected")
	}
}
