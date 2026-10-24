package infrastructure

import (
	"strings"
	"testing"
)

func TestAdminPostgresRendererQuotesOperationLogColumns(t *testing.T) {
	t.Parallel()

	rendered := adminPostgresRenderer.RenderPostgres(`
INSERT INTO sys_operation_log (userId, operationType, operationTime, isDeleted)
VALUES (?, ?, ?, ?)`)
	for _, expected := range []string{`"userId"`, `"operationType"`, `"operationTime"`, `"isDeleted"`} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("rendered SQL missing %s: %s", expected, rendered)
		}
	}
}

func TestAdminPostgresRendererConvertsDeletedPredicate(t *testing.T) {
	t.Parallel()

	rendered := adminPostgresRenderer.RenderPostgres(`SELECT id FROM sys_operation_log WHERE isDeleted = 0`)
	if !strings.Contains(rendered, `"isDeleted" = FALSE`) {
		t.Fatalf("rendered SQL did not convert boolean predicate: %s", rendered)
	}
}
