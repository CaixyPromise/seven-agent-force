package facade

import (
	"strings"
	"testing"
)

func TestSafeSummaryDoesNotClaimTestsOrExposeRawDiagnostic(t *testing.T) {
	code := 127
	for _, o := range []Observation{{Status: "PASSED"}, {Status: "UNAVAILABLE", ReasonCode: "untrusted raw body secret", ExitCode: &code}, {Status: "FAILED", ExitCode: &code}} {
		summary := SafeSummary(o)
		if strings.Contains(summary, "secret") || strings.Contains(summary, "raw body") {
			t.Fatal(summary)
		}
		if o.Status == "PASSED" && !strings.Contains(summary, "测试数量未知") {
			t.Fatal("command success overstated as tests")
		}
	}
}
