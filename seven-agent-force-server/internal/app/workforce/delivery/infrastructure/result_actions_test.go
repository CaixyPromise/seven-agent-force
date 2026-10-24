package infrastructure

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

func TestActionsForFrozenGitPlanDerivesOnlyConfiguredGitDelivery(t *testing.T) {
	t.Parallel()

	derived := actionsForFrozenGitPlan(nil, "NEW_BRANCH_AND_DRAFT_REVIEW")
	if len(derived) != 1 || derived[0].Type != domain.ActionGitPush || derived[0].When != domain.ApprovalAfter {
		t.Fatalf("derived actions = %#v", derived)
	}
	if got := actionsForFrozenGitPlan(nil, "NONE"); len(got) != 0 {
		t.Fatalf("NONE actions = %#v", got)
	}

	explicit := domain.Action{Type: domain.ActionGitPush, When: domain.ApprovalManual, TargetID: 1793999986}
	got := actionsForFrozenGitPlan([]domain.Action{explicit}, "NEW_BRANCH")
	if len(got) != 1 || !reflect.DeepEqual(got[0], explicit) {
		t.Fatalf("explicit action was changed: %#v", got)
	}
}

func TestResultActionContractDecodesJSONBoundaryIDs(t *testing.T) {
	t.Parallel()

	var contract struct {
		Actions []domain.Action `json:"actions"`
	}
	if err := json.Unmarshal([]byte(`{"actions":[{"type":"GIT_PUSH","when":"AFTER_APPROVAL","targetId":"1793999986","baseBranch":"main","branch":"seven/task"}]}`), &contract); err != nil {
		t.Fatalf("decode persisted action contract: %v", err)
	}
	if len(contract.Actions) != 1 || contract.Actions[0].TargetID != 1793999986 || contract.Actions[0].Type != domain.ActionGitPush {
		t.Fatalf("decoded actions = %#v", contract.Actions)
	}
}
