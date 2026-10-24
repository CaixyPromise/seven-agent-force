package infrastructure

import (
	"encoding/json"
	"testing"
)

func TestRequireAdmittedProcessorRuntime(t *testing.T) {
	digest := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	evidence, err := json.Marshal(map[string]any{"probe": map[string]any{"resultProcessor": map[string]any{
		"pythonRuntimeIdentity": digest,
		"bootstrapSha256":       digest,
		"sdkSha256":             digest,
		"moduleIndexSha256":     digest,
		"moduleRoots":           []string{"/usr/local/lib/python3.11/site-packages"},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := requireAdmittedProcessorRuntime(evidence); err != nil {
		t.Fatalf("valid admitted processor runtime rejected: %v", err)
	}
	for name, body := range map[string][]byte{
		"missing":       []byte(`{"probe":{"resultProcessor":{}}}`),
		"bad-digest":    []byte(`{"probe":{"resultProcessor":{"pythonRuntimeIdentity":"not-a-digest","bootstrapSha256":"not-a-digest","sdkSha256":"not-a-digest","moduleIndexSha256":"not-a-digest","moduleRoots":["/sdk"]}}}`),
		"missing-roots": []byte(`{"probe":{"resultProcessor":{"pythonRuntimeIdentity":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","bootstrapSha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","sdkSha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","moduleIndexSha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}}}`),
	} {
		t.Run(name, func(t *testing.T) {
			if err := requireAdmittedProcessorRuntime(body); err == nil {
				t.Fatalf("invalid evidence %s was accepted", name)
			}
		})
	}
}
