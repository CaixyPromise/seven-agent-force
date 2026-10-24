package imageadmission

import "testing"

func TestFleetHarnessAliasIsStableAndDigestAddressed(t *testing.T) {
	manifest := `{"harnessType":"Fake Harness","harnessVersion":"0.2.0-fleet-b"}`
	got, err := fleetHarnessAlias(manifest, "sha256:11c553ee6eb3dbf50770e992d9061b09f397c6de1b1d80dae5913728e80784b9")
	if err != nil {
		t.Fatal(err)
	}
	if want := "fleet-fake-harness-0-2-0-fleet-b-11c553ee6eb3"; got != want {
		t.Fatalf("alias=%q want %q", got, want)
	}
}

func TestFleetHarnessAliasRejectsMalformedIdentity(t *testing.T) {
	for _, test := range []struct {
		name, manifest, digest string
	}{
		{name: "manifest", manifest: `{}`, digest: "sha256:123456789012"},
		{name: "digest", manifest: `{"harnessType":"fake","harnessVersion":"1"}`, digest: "sha256:short"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := fleetHarnessAlias(test.manifest, test.digest); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
