package facade

import (
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"strings"
	"testing"
)

func TestEvaluationPolicies(t *testing.T) {
	for _, tc := range []struct {
		before, after   domain.Status
		candidate, want string
	}{
		{domain.Passed, domain.Failed, "PASS", "REVISE"},
		{domain.Failed, domain.Failed, "REVISE", "WAIT_USER"},
		{domain.Unavailable, domain.NoTests, "PASS", "WAIT_USER"},
		{"", "", "PASS", "WAIT_USER"},
		{domain.Passed, domain.Passed, "REJECT", "REJECT"},
		{domain.Passed, domain.Passed, "PASS", "PASS"},
	} {
		digest := strings.Repeat("a", 64)
		result := GateEvaluation(processor.ResultContract{Decision: tc.candidate}, domain.Observation{CheckDigest: digest, Status: tc.before}, domain.Observation{CheckDigest: digest, Status: tc.after}, domain.ComparisonContext{})
		if result.Decision != tc.want {
			t.Fatalf("%+v got %s", tc, result.Decision)
		}
	}
}
