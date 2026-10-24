package schemaalignment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	completioninfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/infrastructure"
	continuationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/infrastructure"
	interactioninfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/infrastructure"
)

func TestBatch3MigrationMatchesModuleSchema(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "postgres", "20260825130000_workforce_worker_fleet_mvp.sql"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(body), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatal("expected one Down boundary")
	}
	for name, schema := range map[string]string{"conversation": interactioninfra.Schema, "suspension": continuationinfra.InteractionSchemaSQL, "check revisions": completioninfra.RevisionSchemaSQL} {
		if !strings.Contains(parts[0], strings.TrimSpace(schema)) {
			t.Errorf("unpublished migration differs from %s schema", name)
		}
	}
	for _, table := range []string{"workforce_agent_interactions", "workforce_agent_interaction_responses", "workforce_task_conversation_items", "workforce_task_conversation_read_states", "workforce_completion_check_revisions"} {
		if !strings.Contains(parts[1], "DROP TABLE IF EXISTS "+table+";") {
			t.Errorf("Down missing %s", table)
		}
	}
}
