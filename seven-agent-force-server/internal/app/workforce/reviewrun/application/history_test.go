package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

type historicalInputReader struct {
	turns         map[int64]taskfacade.IndependentReviewInput
	requestedTurn int64
	err           error
}

func (r *historicalInputReader) IndependentReviewInput(_ context.Context, _, _, turnID int64) (taskfacade.IndependentReviewInput, error) {
	r.requestedTurn = turnID
	return r.turns[turnID], r.err
}

func TestHistoricalCompletionCheckUsesFrozenRunTurn(t *testing.T) {
	oldDigest, newDigest := strings.Repeat("a", 64), strings.Repeat("b", 64)
	reader := &historicalInputReader{turns: map[int64]taskfacade.IndependentReviewInput{
		3: {TenantID: 1, TaskID: 2, TaskTurnID: 3, CompletionCheckDigest: oldDigest, Instructions: "old criteria"},
		4: {TenantID: 1, TaskID: 2, TaskTurnID: 4, CompletionCheckDigest: newDigest, Instructions: "new criteria"},
	}}
	s := NewService(nil)
	s.BindExecutionInputs(reader)
	item, err := s.hydrateHistoricalRun(context.Background(), 1, domain.RunView{ID: 9, TaskID: 2, TaskTurnID: 3, CompletionCheckDigest: oldDigest})
	if err != nil || reader.requestedTurn != 3 || item.CompletionCheckSummary == nil || item.CompletionCheckSummary.Instructions != "old criteria" {
		t.Fatalf("wrong historical context: %+v %v", item, err)
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var summary map[string]any
	if err = json.Unmarshal(doc["completionCheckSummary"], &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary) != 1 || string(doc["completionCheckDigest"]) != `"`+oldDigest+`"` {
		t.Fatalf("wrong safe summary: %s", raw)
	}
	for _, forbidden := range []string{"script", "command", "provider", "lease", "objectKey", "new criteria", "referenceAssetIds"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("unsafe field %s", forbidden)
		}
	}
	reader.requestedTurn = 0
	legacy, err := s.hydrateHistoricalRun(context.Background(), 1, domain.RunView{TaskID: 2, TaskTurnID: 3})
	if err != nil || reader.requestedTurn != 0 || legacy.CompletionCheckSummary != nil {
		t.Fatal("legacy history acquired newer criteria")
	}
}

func TestHistoricalCompletionCheckRejectsInvalidContextWithoutEcho(t *testing.T) {
	const marker = "PRIVATE_RAW_CONTEXT"
	digest := strings.Repeat("a", 64)
	valid := taskfacade.IndependentReviewInput{TenantID: 1, TaskID: 2, TaskTurnID: 3, CompletionCheckDigest: digest, Instructions: "safe"}
	for _, mutate := range []func(*taskfacade.IndependentReviewInput){
		func(i *taskfacade.IndependentReviewInput) { i.TenantID = 8 },
		func(i *taskfacade.IndependentReviewInput) { i.TaskID = 8 },
		func(i *taskfacade.IndependentReviewInput) { i.TaskTurnID = 4 },
		func(i *taskfacade.IndependentReviewInput) { i.CompletionCheckDigest = strings.Repeat("b", 64) },
		func(i *taskfacade.IndependentReviewInput) { i.Instructions = marker + "\x00" },
		func(i *taskfacade.IndependentReviewInput) { i.ReviewPrompt = marker + string([]byte{255}) },
	} {
		input := valid
		mutate(&input)
		s := NewService(nil)
		s.BindExecutionInputs(inputReader{input})
		item, err := s.hydrateHistoricalRun(context.Background(), 1, domain.RunView{TaskID: 2, TaskTurnID: 3, CompletionCheckDigest: digest})
		if !errors.Is(err, domain.ErrInvalidReviewInput) || strings.Contains(err.Error(), marker) || item.CompletionCheckSummary != nil {
			t.Fatalf("invalid context leaked: %+v %v", item, err)
		}
	}
	s := NewService(nil)
	s.BindExecutionInputs(&historicalInputReader{err: errors.New(marker)})
	_, err := s.hydrateHistoricalRun(context.Background(), 1, domain.RunView{TaskID: 2, TaskTurnID: 3, CompletionCheckDigest: digest})
	if !errors.Is(err, domain.ErrInvalidReviewInput) || strings.Contains(err.Error(), marker) {
		t.Fatalf("reader error leaked: %v", err)
	}
}
