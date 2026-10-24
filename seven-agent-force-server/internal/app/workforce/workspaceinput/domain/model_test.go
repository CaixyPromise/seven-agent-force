package domain

import "testing"

func TestFreezeTaskInputRequiresAvailableSourceAndReferences(t *testing.T) {
	source := SourceRevisionView{ID: 3487660911, TenantID: 2310864210, Status: SourceAvailable, SourceCommit: "0123456789012345678901234567890123456789", SourceTree: "abcdefabcdefabcdefabcdefabcdefabcdefabcd", SourceIdentityDigest: digest64('a')}
	refs := []ReferenceAssetView{{ID: 1763542435, TenantID: 2310864210, Status: ReferenceAvailable, ContentSHA256: digest64('b'), SizeBytes: 123}}
	plan := GitPlan{DeliveryMode: DeliveryNewBranchAndDraftReview, DeliveryAuthorization: DeliveryAuthorizationAsk, TargetConfigDigest: digest64('c'), PushAuthorizationDigest: digest64('d'), ReviewAuthorizationDigest: digest64('e'), ProjectID: 175719119, ProjectVersionID: 1, RepositoryConfigDigest: digest64('f'), ReviewBaseBranch: "main", ReviewBaseCommit: "1111111111111111111111111111111111111111", ResultBranch: "seven/tsk_1-t1"}
	frozen, err := FreezeTaskInput(2310864210, source, refs, plan)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.Digest == "" || frozen.WorkspaceSourceRevisionID != source.ID || len(frozen.ReferenceAssetIDs) != 1 {
		t.Fatalf("unexpected frozen input: %+v", frozen)
	}

	source.Status = SourcePending
	if _, err = FreezeTaskInput(2310864210, source, refs, plan); err == nil {
		t.Fatal("pending source revision must be rejected")
	}
	source.Status = SourceAvailable
	refs[0].TenantID = 1002
	if _, err = FreezeTaskInput(2310864210, source, refs, plan); err == nil {
		t.Fatal("cross-tenant reference asset must be rejected")
	}
}

func TestGitPlanRejectsBaseBranchAndUnsafeResultRef(t *testing.T) {
	policy := RepositoryPolicy{ReviewSupported: true}
	valid := GitPlan{DeliveryMode: DeliveryNewBranchAndDraftReview, DeliveryAuthorization: DeliveryAuthorizationAsk, TargetConfigDigest: digest64('c'), PushAuthorizationDigest: digest64('d'), ReviewAuthorizationDigest: digest64('e'), ProjectID: 175719119, ProjectVersionID: 1, RepositoryConfigDigest: digest64('f'), ReviewBaseBranch: "main", ReviewBaseCommit: "1111111111111111111111111111111111111111", ResultBranch: "seven/tsk_1-t1"}
	if err := ValidateGitPlan(valid, policy); err != nil {
		t.Fatalf("valid plan rejected: %v", err)
	}
	for _, branch := range []string{"main", "refs/heads/evil", "seven/../main", "seven/bad name"} {
		candidate := valid
		candidate.ResultBranch = branch
		if err := ValidateGitPlan(candidate, policy); err == nil {
			t.Fatalf("unsafe result branch accepted: %q", branch)
		}
	}
}

func TestFrozenTargetBaseRevisionParticipatesInTaskInputIdentity(t *testing.T) {
	source := SourceRevisionView{ID: 3487660911, TenantID: 2310864210, Status: SourceAvailable, SourceCommit: "0123456789012345678901234567890123456789", SourceTree: "abcdefabcdefabcdefabcdefabcdefabcdefabcd", SourceIdentityDigest: digest64('a')}
	plan := GitPlan{DeliveryMode: DeliveryNewBranch, DeliveryAuthorization: DeliveryAuthorizationAsk, TargetConfigDigest: digest64('c'), PushAuthorizationDigest: digest64('d'), ProjectID: 175719119, ProjectVersionID: 1, RepositoryConfigDigest: digest64('f'), ReviewBaseSourceRevisionID: 3219911616, ReviewBaseBranch: "main", ReviewBaseCommit: "1111111111111111111111111111111111111111", ResultBranch: "seven/tsk_1-t1"}
	first, err := FreezeTaskInput(2310864210, source, nil, plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.ReviewBaseSourceRevisionID = 1003
	second, err := FreezeTaskInput(2310864210, source, nil, plan)
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == second.Digest {
		t.Fatal("target base source revision must be part of frozen task input identity")
	}
	if err = ValidateGitPlan(GitPlan{DeliveryMode: DeliveryNone, ReviewBaseSourceRevisionID: 3219911616}, RepositoryPolicy{}); err == nil {
		t.Fatal("delivery NONE must reject a hidden target base revision")
	}
}

func digest64(ch byte) string {
	value := make([]byte, 64)
	for i := range value {
		value[i] = ch
	}
	return string(value)
}
