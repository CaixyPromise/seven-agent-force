package infrastructure

import "testing"

func TestResolvedHookViewsIgnoreRetiredSnapshotFields(t *testing.T) {
	raw := []byte(`{
  "completionCheck": {
    "mode": "COMMAND",
    "command": "true",
    "reviewFocus": "retired project review text"
  },
  "packageReleases": [
    {
      "id": "101",
      "packageId": "201",
      "name": "Sensitive file guard",
      "version": "2.0.0"
    }
  ]
}`)
	hooks, err := resolvedHookViews(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 || hooks[0].ReleaseID != 101 || hooks[0].HookID != 201 || hooks[0].Name != "Sensitive file guard" || hooks[0].Version != "2.0.0" {
		t.Fatalf("hooks=%+v", hooks)
	}
}

func TestResolvedHookViewsRejectMalformedSnapshot(t *testing.T) {
	if _, err := resolvedHookViews([]byte(`{"packageReleases":`)); err == nil {
		t.Fatal("expected malformed snapshot to fail")
	}
}
