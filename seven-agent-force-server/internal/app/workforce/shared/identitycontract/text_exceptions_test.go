package identitycontract

import "testing"

func TestTextIdentityExceptionsHaveReasonsAndCannotBeMutatedByCallers(t *testing.T) {
	exceptions := TextIdentityExceptions()
	if len(exceptions) == 0 {
		t.Fatal("TEXT identity exception registry is empty")
	}
	for column, reason := range exceptions {
		if column == "" || reason == "" {
			t.Fatalf("TEXT identity exception must name its column and reason: %q=%q", column, reason)
		}
	}
	delete(exceptions, "workforce_tasks.external_correlation_id")
	if !IsTextIdentityException("workforce_tasks.external_correlation_id") {
		t.Fatal("caller mutated the repository-wide TEXT identity exception registry")
	}
}
