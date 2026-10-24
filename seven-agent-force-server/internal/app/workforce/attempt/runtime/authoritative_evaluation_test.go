package runtime

import (
	"encoding/json"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func waitDirective(t *testing.T) facade.EvaluationDirective {
	t.Helper()
	c := processor.ResultContract{Decision: "WAIT_USER", ReasonCode: "CHECK_UNAVAILABLE", Message: "Please select verification", Continuation: processor.ContinuationSelection{Includes: []string{"result.txt"}}}
	b, _ := json.Marshal(c)
	digest, e := processor.ContractDigest(c)
	if e != nil {
		t.Fatal(e)
	}
	return facade.EvaluationDirective{CycleID: 5, Decision: c.Decision, Directive: facade.DirectiveWaitForUser, Contract: b, ContractDigest: digest}
}

func TestAuthoritativeWaitUserPreservesCandidateAndCheckpointPlanOnReplay(t *testing.T) {
	root := t.TempDir()
	a := domain.Attempt{ID: 3, Generation: 1}
	candidate := filepath.Join(root, "candidate.result.json")
	original := []byte(`{"decision":"PASS"}`)
	os.WriteFile(candidate, original, 0600)
	d := waitDirective(t)
	for i := 0; i < 2; i++ {
		if e := PersistAuthoritativeEvaluation(root, 1, a, 5, d); e != nil {
			t.Fatal(e)
		}
	}
	c, e := ReadAuthoritativeEvaluation(root, 1, a, 5, "WAIT_USER")
	if e != nil || c.Decision != "WAIT_USER" || len(c.Continuation.Includes) != 1 || c.Continuation.Includes[0] != "result.txt" {
		t.Fatalf("checkpoint plan %+v %v", c, e)
	}
	b, _ := os.ReadFile(candidate)
	if string(b) != string(original) {
		t.Fatal("candidate overwritten")
	}
	if _, e = ReadAuthoritativeEvaluation(root, 1, a, 5, "PASS"); e == nil {
		t.Fatal("decision mismatch accepted")
	}
	if _, e = ReadAuthoritativeEvaluation(root, 1, a, 6, "WAIT_USER"); e == nil {
		t.Fatal("missing authority fell back")
	}
	changed := d
	c.Message = "different persisted result"
	changed.Contract, _ = json.Marshal(c)
	changed.ContractDigest, _ = processor.ContractDigest(c)
	if e = PersistAuthoritativeEvaluation(root, 1, a, 5, changed); e == nil {
		t.Fatal("conflicting replay overwrote authoritative record")
	}
	path, _ := authoritativeEvaluationPath(root, 1, a, 5)
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("authoritative record must be Worker-private")
	}
	stored, _ := os.ReadFile(path)
	if e = os.WriteFile(path, []byte(strings.Replace(string(stored), "Please select verification", "tampered", 1)), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = ReadAuthoritativeEvaluation(root, 1, a, 5, "WAIT_USER"); e == nil {
		t.Fatal("tampered authoritative contract accepted")
	}
}

func TestAuthoritativeEvaluationRejectsWrongCycleDigestAndDecision(t *testing.T) {
	for _, kind := range []string{"cycle", "digest", "decision", "directive", "contract"} {
		t.Run(kind, func(t *testing.T) {
			d := waitDirective(t)
			switch kind {
			case "cycle":
				d.CycleID = 6
			case "digest":
				d.ContractDigest = strings.Repeat("0", 64)
			case "decision":
				d.Decision = "PASS"
			case "directive":
				d.Directive = facade.DirectiveProposeCompletion
			case "contract":
				d.Contract = nil
			}
			if e := PersistAuthoritativeEvaluation(t.TempDir(), 1, domain.Attempt{ID: 3, Generation: 1}, 5, d); e == nil {
				t.Fatal("invalid authority accepted")
			}
		})
	}
}
