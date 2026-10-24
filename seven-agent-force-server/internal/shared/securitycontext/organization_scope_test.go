package securitycontext

import "testing"

func TestResolveOrganizationScopeUsesPrimaryOrganization(t *testing.T) {
	scope, err := ResolveOrganizationScope(&UserContext{
		UserID:       101,
		PrimaryOrgID: 22,
		OrgIDs:       []int64{11, 22},
	})
	if err != nil {
		t.Fatalf("ResolveOrganizationScope() error = %v", err)
	}
	if scope.OrgID != 22 || scope.ScopeID != "org:22" || scope.Source != "primary-org" {
		t.Fatalf("unexpected primary organization scope: %+v", scope)
	}
}

func TestResolveOrganizationScopeAllowsAuditableSingleMembershipFallback(t *testing.T) {
	scope, err := ResolveOrganizationScope(&UserContext{
		UserID: 101,
		OrgIDs: []int64{33, 33, 0},
	})
	if err != nil {
		t.Fatalf("ResolveOrganizationScope() error = %v", err)
	}
	if scope.ScopeID != "org:33" || scope.Source != "single-org-fallback" {
		t.Fatalf("unexpected fallback organization scope: %+v", scope)
	}
}

func TestResolveOrganizationScopeRejectsAmbiguousOrInconsistentMembership(t *testing.T) {
	tests := []*UserContext{
		{UserID: 101},
		{UserID: 101, OrgIDs: []int64{11, 22}},
		{UserID: 101, PrimaryOrgID: 33, OrgIDs: []int64{11, 22}},
	}
	for _, user := range tests {
		if _, err := ResolveOrganizationScope(user); err == nil {
			t.Fatalf("ambiguous or inconsistent user must fail closed: %+v", user)
		}
	}
}
