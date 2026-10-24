package infrastructure

import (
	"regexp"
	"strconv"
	"testing"
)

func TestCredentialReferenceChecksBindEverySQLParameter(t *testing.T) {
	checks := credentialReferenceChecks(11, 22)
	if len(checks) != 5 {
		t.Fatalf("reference check count = %d, want 5", len(checks))
	}

	placeholder := regexp.MustCompile(`\$(\d+)`)
	for index, check := range checks {
		matches := placeholder.FindAllStringSubmatch(check.query, -1)
		maximum := 0
		for _, match := range matches {
			value, err := strconv.Atoi(match[1])
			if err != nil {
				t.Fatalf("check %d has invalid placeholder %q: %v", index, match[0], err)
			}
			if value > maximum {
				maximum = value
			}
		}
		if maximum != len(check.args) {
			t.Fatalf("check %d binds %d arguments for SQL requiring %d", index, len(check.args), maximum)
		}
	}
}
