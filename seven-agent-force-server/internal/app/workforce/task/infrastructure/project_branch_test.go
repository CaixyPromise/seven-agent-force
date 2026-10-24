package infrastructure

import (
	"testing"

	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

func TestExpandResultBranchUsesTheAllocatedTaskID(t *testing.T) {
	plan := workspaceinputdomain.GitPlan{
		DeliveryMode:          workspaceinputdomain.DeliveryNewBranch,
		DeliveryAuthorization: workspaceinputdomain.DeliveryAuthorizationAsk,
		TargetConfigDigest:    digest64ForProjectBranch('a'), PushAuthorizationDigest: digest64ForProjectBranch('b'),
		ProjectID: 11, ProjectVersionID: 1, RepositoryConfigDigest: digest64ForProjectBranch('c'), ReviewBaseSourceRevisionID: 12, ReviewBaseBranch: "main",
		ReviewBaseCommit: "1111111111111111111111111111111111111111", ResultBranch: "codex/{task-id}", FullProjectImport: true,
	}
	first, err := expandResultBranch(plan, 9007199254740993)
	if err != nil {
		t.Fatalf("expand first result branch: %v", err)
	}
	second, err := expandResultBranch(plan, 9007199254740994)
	if err != nil {
		t.Fatalf("expand second result branch: %v", err)
	}
	if first.ResultBranch != "codex/9007199254740993" || second.ResultBranch != "codex/9007199254740994" || first.ResultBranch == second.ResultBranch {
		t.Fatalf("task branches are not unique: %q %q", first.ResultBranch, second.ResultBranch)
	}
}

func digest64ForProjectBranch(value byte) string {
	result := make([]byte, 64)
	for i := range result {
		result[i] = value
	}
	return string(result)
}
