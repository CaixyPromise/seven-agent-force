package infrastructure

import (
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"strings"
	"testing"
)

func TestEvidenceSafeProjectionRejectsContradictionsAndPaths(t *testing.T) {
	h := strings.Repeat("a", 64)
	base := `{"checkDigest":"` + h + `","status":"FAILED","workspaceDigest":"` + h + `","failures":[{"key":"TS2322","path":"src/parser.go","fingerprint":"` + h + `"}]}`
	for _, tc := range []struct{ name, raw string }{
		{"absolute path", strings.Replace(base, "src/parser.go", "/Users/private/secret", 1)},
		{"invalid definition digest", strings.Replace(base, `"status":`, `"definitionDigest":"not-a-digest","status":`, 1)},
		{"traversal", strings.Replace(base, "src/parser.go", "../private", 1)},
		{"raw error", strings.Replace(base, "TS2322", "token=credential", 1)},
		{"control", strings.Replace(base, "TS2322", `bad\nerror`, 1)},
		{"empty fingerprint", strings.Replace(base, `"fingerprint":"`+h+`"`, `"fingerprint":""`, 1)},
		{"passed failures", strings.Replace(base, "FAILED", "PASSED", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var o facade.Observation
			if err := json.Unmarshal([]byte(tc.raw), &o); err != nil {
				t.Fatal(err)
			}
			if _, err := normalizeEvidence(facade.Evidence{CheckDigest: h, Status: string(o.Status), Observation: o}); err == nil {
				t.Fatal("unsafe evidence accepted")
			}
		})
	}
	var o facade.Observation
	if err := json.Unmarshal([]byte(base), &o); err != nil {
		t.Fatal(err)
	}
	e, err := normalizeEvidence(facade.Evidence{CheckDigest: h, Status: "FAILED", Summary: "raw secret", Observation: o})
	if err != nil || strings.Contains(e.Summary, "secret") {
		t.Fatal("safe projection failed", err)
	}
	o.Failures = append(o.Failures, o.Failures[0])
	if _, err := normalizeEvidence(facade.Evidence{CheckDigest: h, Status: "FAILED", Observation: o}); err == nil {
		t.Fatal("duplicate fingerprint accepted")
	}
}
