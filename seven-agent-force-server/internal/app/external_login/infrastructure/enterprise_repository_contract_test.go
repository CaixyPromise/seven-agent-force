package infrastructure

import (
	"strings"
	"testing"
)

func TestEnterpriseIdentityInsertHasOneExpressionPerTargetColumn(t *testing.T) {
	parts := strings.Split(insertEnterpriseIdentitySQL, ") VALUES (")
	if len(parts) != 2 {
		t.Fatalf("unexpected enterprise identity insert shape")
	}
	columns := strings.Split(strings.TrimPrefix(parts[0], "INSERT INTO sys_external_user_identity ("), ",")
	values := strings.Split(strings.TrimSuffix(parts[1], ")"), ",")
	if len(columns) != len(values) {
		t.Fatalf("enterprise identity insert has %d columns and %d expressions", len(columns), len(values))
	}
}
