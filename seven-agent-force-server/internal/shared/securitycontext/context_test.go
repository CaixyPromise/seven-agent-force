package securitycontext

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestHasRoleDoesNotTreatAuthorizationRootAsEveryRole(t *testing.T) {
	reqCtx := app.NewContext(0)
	Set(reqCtx, &UserContext{
		UserID:      1001,
		Roles:       []string{"CUSTOM_ROOT"},
		IsAdmin:     true,
		IsAnonymous: false,
	})

	if !HasRole(reqCtx, "CUSTOM_ROOT") {
		t.Fatal("expected the assigned role code to match")
	}
	if HasRole(reqCtx, "UNASSIGNED_ROLE") {
		t.Fatal("authorization root must not match an unassigned role code")
	}
}

func TestPermissionMatches(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		required  string
		want      bool
	}{
		{name: "exact", candidate: "system:user:list", required: "system:user:list", want: true},
		{name: "global wildcard", candidate: "*", required: "system:user:list", want: true},
		{name: "prefix wildcard", candidate: "system:user:*", required: "system:user:list", want: true},
		{name: "different prefix", candidate: "system:role:*", required: "system:user:list", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PermissionMatches(tt.candidate, tt.required); got != tt.want {
				t.Fatalf("PermissionMatches(%q, %q) = %v, want %v", tt.candidate, tt.required, got, tt.want)
			}
		})
	}
}
