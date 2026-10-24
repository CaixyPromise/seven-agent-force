package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	check "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/jackc/pgx/v5"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// This checks the actual public Store entry point, not merely its replay
// helper. It is a source guard regression, not a database integration test.
func TestPersistEvaluationOuterGuardAllowsOnlyExistingReviewingReplay(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "runtime.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string]string{}
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		var text bytes.Buffer
		if err := printer.Fprint(&text, fset, fn.Body); err != nil {
			t.Fatal(err)
		}
		bodies[fn.Name.Name] = text.String()
	}
	persist := bodies["PersistEvaluation"]
	guard := strings.Index(persist, `scope.TaskStatus != "RUNNING" && scope.TaskStatus != "VALIDATING" && scope.TaskStatus != "REVIEWING"`)
	lookup := strings.Index(persist, "SELECT contract_digest,decision,id,reason_code,message")
	replay := strings.Index(persist, "s.directiveForPersistedCycle(")
	newCycleGuard := strings.Index(persist, `if scope.TaskStatus == "REVIEWING"`)
	insert := strings.Index(persist, "INSERT INTO workforce_result_evaluation_cycles")
	if guard < 0 || lookup <= guard || replay <= lookup || newCycleGuard <= replay || insert <= newCycleGuard {
		t.Fatal("PersistEvaluation must allow REVIEWING through exact replay but reject before creating a cycle")
	}
	if !strings.Contains(persist[lookup:replay], "existingDigest != digest") {
		t.Fatal("replay must reject a different contract")
	}
	materialize := bodies["MaterializeEvaluationEnvironment"]
	if !strings.Contains(materialize, `scope.TaskStatus != "RUNNING" && scope.TaskStatus != "VALIDATING"`) || strings.Contains(materialize, `scope.TaskStatus != "REVIEWING"`) {
		t.Fatal("unrelated environment materialization guard was widened")
	}
}

type persistedContractRow struct {
	body   json.RawMessage
	digest string
}

func (r persistedContractRow) Scan(dest ...any) error {
	*dest[0].(*json.RawMessage) = r.body
	*dest[1].(*string) = r.digest
	return nil
}

type persistedContractTx struct {
	pgx.Tx
	row persistedContractRow
}

func (tx persistedContractTx) QueryRow(context.Context, string, ...any) pgx.Row { return tx.row }
func TestUnavailableCheckPassToWaitUserReplayReturnsPersistedContract(t *testing.T) {
	input := check.Input{Mode: "COMMAND", Command: "missing-command"}
	digest, _ := check.Digest(input)
	raw, _ := json.Marshal(input)
	s := &Store{}
	reader := &checkReader{t: t, before: check.Observation{CheckDigest: digest, Status: "UNAVAILABLE"}, after: check.Observation{CheckDigest: digest, Status: "UNAVAILABLE"}}
	s.SetCompletionCheckEvidenceReader(reader)
	original := processor.ResultContract{Decision: "PASS", ReasonCode: "PROCESSOR_OK", Message: "candidate", Continuation: processor.ContinuationSelection{Includes: []string{"result.txt"}}}
	final, e := s.completionCheckGate(context.Background(), checkTx{row: checkRow{raw: raw, digest: digest}}, evaluationScope{TenantID: 1, TaskID: 2, TurnID: 3}, 4, 5, original)
	if e != nil || final.Decision != "WAIT_USER" || original.Decision != "PASS" {
		t.Fatalf("conversion %+v %v", final, e)
	}
	body, _ := json.Marshal(final)
	finalDigest, _ := processor.ContractDigest(final)
	for i := 0; i < 2; i++ {
		d, e := s.directiveForPersistedCycle(context.Background(), persistedContractTx{row: persistedContractRow{body, finalDigest}}, 5, final.Decision, final.ReasonCode, final.Message, 2)
		if e != nil || d.Decision != "WAIT_USER" || d.Directive != facade.DirectiveWaitForUser || d.ContractDigest != finalDigest || string(d.Contract) != string(body) {
			t.Fatalf("replay %+v %v", d, e)
		}
	}
}
