package infrastructure

import (
	"strings"
	"testing"
)

func TestHubControlPostgresRendererKeepsSnakeTablesBare(t *testing.T) {
	query := `INSERT INTO sys_federated_node_connection_command (nodeCode, connectionVersion, terminalState, updatedAt)
VALUES (?, ?, ?, ?) ON CONFLICT (nodeCode, connectionVersion)
DO UPDATE SET terminalState=EXCLUDED.terminalState, updatedAt=EXCLUDED.updatedAt`
	got := hubControlPostgresRenderer.RenderPostgres(query)
	for _, want := range []string{
		`INSERT INTO sys_federated_node_connection_command`, `("nodeCode", "connectionVersion", "terminalState", "updatedAt")`,
		`EXCLUDED."terminalState"`, `EXCLUDED."updatedAt"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered query missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, `"sys_federated_node_connection_command"`) {
		t.Fatalf("lower snake table should not be quoted: %s", got)
	}
}
