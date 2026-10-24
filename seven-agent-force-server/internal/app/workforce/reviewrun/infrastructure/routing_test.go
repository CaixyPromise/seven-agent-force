package infrastructure

import (
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
)

func TestRunIdentityUsesFixedReviewContract(t *testing.T) {
	input := facade.FinalizationInput{TenantID: 1, TaskID: 2, TaskTurnID: 3, AttemptID: 4, FinalizationRunID: 5, ResultCommit: "commit", ResultTree: "tree"}
	identity, err := runIdentity(input, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	if identity.HardTimeoutSeconds != 3600 || identity.TransientRetryLimit != 3 || identity.AutomaticRevisionLimit != 3 {
		t.Fatalf("runtime constants changed: %+v", identity)
	}
}
