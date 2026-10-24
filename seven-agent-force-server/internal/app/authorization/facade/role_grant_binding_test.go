package facade

import "testing"

func TestRoleGrantRequestHashBindsReason(t *testing.T) {
	request := RoleGrantBundleRequest{ExpectedRevision: 3, DataScope: 5, Reason: "first reason"}
	first, err := RoleGrantRequestHash(request)
	if err != nil {
		t.Fatalf("hash first request: %v", err)
	}
	request.Reason = "second reason"
	second, err := RoleGrantRequestHash(request)
	if err != nil {
		t.Fatalf("hash second request: %v", err)
	}
	if first == second {
		t.Fatalf("authorization reason must participate in the idempotency and step-up binding")
	}
}
