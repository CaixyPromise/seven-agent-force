package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExecutionSnapshotAcceptsLosslessModelCredentialIdentity(t *testing.T) {
	var snapshot ExecutionSnapshot
	if err := json.Unmarshal([]byte(`{"harnessReleaseId":"1005","modelCredentialId":"2001","credentialVersion":"3","modelProtocol":"OPENAI","primaryModel":"gpt-test","isolationProfile":"seven.worker-image/v1"}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ModelCredentialID != 2001 || snapshot.CredentialVersion != 3 || snapshot.ModelProtocol != "OPENAI" || snapshot.PrimaryModel != "gpt-test" {
		t.Fatalf("model identity=%+v", snapshot)
	}
}

func TestExecutionSnapshotContainsNoReviewPolicy(t *testing.T) {
	raw, err := json.Marshal(ExecutionSnapshot{HarnessReleaseID: 1005, IsolationProfile: "seven.worker-image/v1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"reviewProfile", "rubric", "reviewFocus", "reviewSandbox"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("execution snapshot contains %q: %s", forbidden, raw)
		}
	}
}
