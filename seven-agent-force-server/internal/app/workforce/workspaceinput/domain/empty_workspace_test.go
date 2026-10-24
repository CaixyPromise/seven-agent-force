package domain

import (
	"errors"
	"reflect"
	"testing"
)

func TestCanonicalEmptyWorkspaceIdentityIsStable(t *testing.T) {
	first, second := CanonicalEmptyWorkspaceIdentity(), CanonicalEmptyWorkspaceIdentity()
	if first != second || !hexLen(first.Commit, 40) || first.Tree != EmptyWorkspaceTree || !hexLen(first.IdentityDigest, 64) {
		t.Fatalf("invalid deterministic identity: %#v %#v", first, second)
	}
}

func TestEmptyWorkspaceAcceptsIndependentReferences(t *testing.T) {
	first := ReferenceAssetView{ID: 2608563565, TenantID: 2310864210, Status: ReferenceAvailable, ContentSHA256: digest64('b'), SizeBytes: 20}
	second := ReferenceAssetView{ID: 41178327, TenantID: 2310864210, Status: ReferenceAvailable, ContentSHA256: digest64('a'), SizeBytes: 10}
	frozen, err := FreezeEmptyTaskInput(2310864210, []ReferenceAssetView{first, second}, GitPlan{DeliveryMode: DeliveryNone})
	if err != nil {
		t.Fatal(err)
	}
	if frozen.SourceKind != SourceKindEmptyWorkspace || frozen.WorkspaceSourceRevisionID != 0 {
		t.Fatalf("unexpected source identity: %+v", frozen)
	}
	if !reflect.DeepEqual([]int64(frozen.ReferenceAssetIDs), []int64{41178327, 2608563565}) {
		t.Fatalf("reference identity must be deterministic: %#v", frozen.ReferenceAssetIDs)
	}

	withoutReferences, err := FreezeEmptyTaskInput(2310864210, nil, GitPlan{DeliveryMode: DeliveryNone})
	if err != nil || len(withoutReferences.ReferenceAssetIDs) != 0 {
		t.Fatalf("attachments are optional: %+v err=%v", withoutReferences, err)
	}
}

func TestEmptyWorkspaceRejectsDeliveryAndUnsafeReferences(t *testing.T) {
	if _, err := FreezeEmptyTaskInput(2310864210, nil, GitPlan{DeliveryMode: DeliveryNewBranch}); !errors.Is(err, ErrTaskInputInvalid) {
		t.Fatalf("EMPTY plus delivery must fail closed, got %v", err)
	}
	invalid := ReferenceAssetView{ID: 1763542435, TenantID: 2310864210, Status: SourcePending, ContentSHA256: digest64('a'), SizeBytes: 1}
	if _, err := FreezeEmptyTaskInput(2310864210, []ReferenceAssetView{invalid}, GitPlan{DeliveryMode: DeliveryNone}); !errors.Is(err, ErrReferenceNotAvailable) {
		t.Fatalf("pending reference must be rejected, got %v", err)
	}
	invalid.Status, invalid.TenantID = ReferenceAvailable, 1002
	if _, err := FreezeEmptyTaskInput(2310864210, []ReferenceAssetView{invalid}, GitPlan{DeliveryMode: DeliveryNone}); !errors.Is(err, ErrReferenceNotAvailable) {
		t.Fatalf("cross-tenant reference must be rejected, got %v", err)
	}
}
