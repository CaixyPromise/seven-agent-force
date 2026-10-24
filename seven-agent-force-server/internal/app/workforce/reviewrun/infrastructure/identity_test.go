package infrastructure

import (
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
)

func TestCompletionCheckBindsReviewOccurrenceIdentity(t *testing.T) {
	base := domain.RunIdentity{TenantID: 1, TaskID: 2, TaskTurnID: 3, FinalizationRunID: 4, AttemptID: 5,
		ResultCommit: "commit", ResultTree: "tree", HardTimeoutSeconds: 3600, TransientRetryLimit: 3, AutomaticRevisionLimit: 3}
	first := base
	first.CompletionCheckDigest = strings.Repeat("a", 64)
	second := first
	second.CompletionCheckDigest = strings.Repeat("b", 64)
	if sameIdentity(first, second) || digestIdentity(first) == digestIdentity(second) {
		t.Fatal("different completion check reused identity")
	}
	if !sameIdentity(first, first) || digestIdentity(first) == "" {
		t.Fatal("exact replay did not converge")
	}
}
