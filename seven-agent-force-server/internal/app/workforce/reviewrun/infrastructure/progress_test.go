package infrastructure

import (
	"testing"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
)

func TestMaterialProgressIgnoresFormattingOnlyTreeChanges(t *testing.T) {
	findings := []string{"WARNING\x00CORRECTNESS\x00main.go\x0010"}
	if hasMaterialProgress(0, 0, findings, "tree-after-format", 60, 0, 0, findings, "tree-before-format", 60) {
		t.Fatal("tree-only formatting change was treated as material progress")
	}
}

func TestMaterialProgressRecognizesFindingAndTestImprovement(t *testing.T) {
	previous := []string{"BLOCKING\x00SECURITY\x00main.go\x0010", "WARNING\x00CORRECTNESS\x00main.go\x0020"}
	if !hasMaterialProgress(0, 2, previous[1:], "same-tree", 40, 1, 2, previous, "same-tree", 40) {
		t.Fatal("strict actionable finding reduction was not material progress")
	}
	if !hasMaterialProgress(1, 1, previous, "same-tree", 40, 1, 2, previous, "same-tree", 40) {
		t.Fatal("deterministic failure reduction was not material progress")
	}
}

func TestMaterialProgressRequiresNonWorseEvidenceForScoreAndTree(t *testing.T) {
	findings := []string{"WARNING\x00CORRECTNESS\x00main.go\x0020"}
	if !hasMaterialProgress(0, 0, findings, "new-tree", 80, 0, 0, findings, "old-tree", 60) {
		t.Fatal("higher canonical score with changed result tree and non-worse evidence was not progress")
	}
	if hasMaterialProgress(1, 0, findings, "new-tree", 80, 0, 0, findings, "old-tree", 60) {
		t.Fatal("worse blocking evidence was hidden by a score increase")
	}
}

func TestRevisionStopPolicyUsesFrozenBudgetAndNoProgress(t *testing.T) {
	for _, test := range []struct {
		name  string
		facts progressFacts
		limit int
		want  string
	}{
		{name: "below budget", facts: progressFacts{AutomaticRevisionNumber: 2, NoProgressStreak: 1}, limit: 3},
		{name: "frozen limit exhausted", facts: progressFacts{AutomaticRevisionNumber: 3}, limit: 3, want: reasonRevisionLimit},
		{name: "no progress", facts: progressFacts{AutomaticRevisionNumber: 1, NoProgressStreak: 2}, limit: 3, want: reasonNoMaterialProgress},
		{name: "human cycle never reopens automatic loop", facts: progressFacts{CurrentRevisionKind: continuationfacade.RevisionHumanAuthorized}, limit: 3, want: reasonRevisionLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := revisionStopReason(test.facts, test.limit); got != test.want {
				t.Fatalf("revisionStopReason()=%q want %q", got, test.want)
			}
		})
	}
}
