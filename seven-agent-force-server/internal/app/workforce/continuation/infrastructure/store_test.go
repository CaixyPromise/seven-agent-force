package infrastructure

import (
	"strings"
	"testing"
)

func TestContinuationInputSnapshotCopyPreservesSourceKind(t *testing.T) {
	normalized := strings.Join(strings.Fields(continuationInputSnapshotCopySQL), " ")
	if !strings.Contains(normalized, "task_turn_id,source_kind,workspace_source_revision_id") {
		t.Fatalf("continuation snapshot insert must own source_kind: %s", normalized)
	}
	if !strings.Contains(normalized, "$2,source_kind,workspace_source_revision_id") {
		t.Fatalf("continuation snapshot select must copy source_kind: %s", normalized)
	}
}
