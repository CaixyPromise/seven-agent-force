package application

import "testing"

func TestPermissionCodeSetAllowsScopedWildcard(t *testing.T) {
	held := []string{"system:*"}

	if !permissionCodeSetAllows(held, "system:user:list") {
		t.Fatalf("system:* should allow granting concrete system permission")
	}
	if !permissionCodeSetAllows(held, "system:user:*") {
		t.Fatalf("system:* should allow granting narrower scoped wildcard")
	}
	if permissionCodeSetAllows(held, "*") {
		t.Fatalf("system:* must not allow granting global wildcard")
	}
	if permissionCodeSetAllows(held, "admin:user:list") {
		t.Fatalf("system:* must not allow granting another permission scope")
	}
}

func TestPermissionCodeSetAllowsExactOnlyDoesNotExpand(t *testing.T) {
	held := []string{"system:user:list"}

	if !permissionCodeSetAllows(held, "system:user:list") {
		t.Fatalf("exact permission should allow granting itself")
	}
	if permissionCodeSetAllows(held, "system:user:*") {
		t.Fatalf("exact permission must not allow granting wildcard")
	}
	if permissionCodeSetAllows(held, "system:user:edit") {
		t.Fatalf("exact permission must not allow granting sibling permission")
	}
}

func TestNormalizePermissionCodes(t *testing.T) {
	values := normalizePermissionCodes([]string{" system:user:list ", "", "system:user:list", "*"})
	if len(values) != 2 || values[0] != "system:user:list" || values[1] != "*" {
		t.Fatalf("unexpected normalized values: %#v", values)
	}
}
