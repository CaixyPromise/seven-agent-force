package errors

import "testing"

func TestPermissionDeniedIncludesOnlySafeStableDetails(t *testing.T) {
	err := PermissionDenied(" system:user:update ")
	if err.Code() != CodeForbidden {
		t.Fatalf("unexpected code: %d", err.Code())
	}
	details, ok := err.Details().(map[string]string)
	if !ok {
		t.Fatalf("unexpected details type: %T", err.Details())
	}
	if details["requiredPermission"] != "system:user:update" || details["reasonCode"] != "PERMISSION_NOT_GRANTED" {
		t.Fatalf("unexpected details: %#v", details)
	}
	if len(details) != 2 {
		t.Fatalf("permission denial leaked extra details: %#v", details)
	}
}
