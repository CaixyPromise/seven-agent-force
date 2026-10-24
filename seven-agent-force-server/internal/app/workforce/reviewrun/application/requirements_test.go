package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

type inputReader struct {
	input taskfacade.IndependentReviewInput
}

func (r inputReader) IndependentReviewInput(context.Context, int64, int64, int64) (taskfacade.IndependentReviewInput, error) {
	return r.input, nil
}

func TestHydrateClaimUsesTaskProjectVersionInput(t *testing.T) {
	for _, tc := range []struct {
		name, claimDigest, inputDigest string
		reject                         bool
	}{
		{"legacy", "", "", false},
		{"matching", strings.Repeat("a", 64), strings.Repeat("a", 64), false},
		{"changed", strings.Repeat("a", 64), strings.Repeat("b", 64), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewService(nil)
			s.BindExecutionInputs(inputReader{taskfacade.IndependentReviewInput{TenantID: 1, TaskID: 2, TaskTurnID: 3, Instruction: "inspect", ReviewPrompt: "check regressions", CompletionCheckDigest: tc.inputDigest}})
			claim, err := s.hydrateClaim(context.Background(), facade.Claim{TenantID: 1, TaskID: 2, TaskTurnID: 3, CompletionCheckDigest: tc.claimDigest})
			if tc.reject {
				if !errors.Is(err, domain.ErrFenceRejected) {
					t.Fatalf("want fence rejection, got %v", err)
				}
				return
			}
			if err != nil || !json.Valid(claim.Requirements) {
				t.Fatalf("claim=%+v error=%v", claim, err)
			}
		})
	}
}

func TestRequirementsContainOnlyProjectPromptAndSafeEvidence(t *testing.T) {
	input := taskfacade.IndependentReviewInput{
		Instruction: "original task", CompletionCheckDigest: strings.Repeat("a", 64), Instructions: "check expected behavior", ReviewPrompt: "regressions",
		References: []taskfacade.IndependentReviewReference{{ReferenceAssetID: 9007199254740993, Name: "reference.pdf", ContentSHA256: strings.Repeat("b", 64), SizeBytes: 123}},
	}
	raw, err := reviewRequirements(input)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]json.RawMessage
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if string(value["reviewPrompt"]) != `"regressions"` || value["reviewFocus"] != nil {
		t.Fatalf("unexpected projection: %s", raw)
	}
	var refs []map[string]any
	if err = json.Unmarshal(value["references"], &refs); err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0]["referenceAssetId"] != "9007199254740993" || refs[0]["readOnly"] != true || refs[0]["containerPath"] != "/review/references/9007199254740993/content" {
		t.Fatalf("unsafe reference projection: %s", raw)
	}
	input.ReviewPrompt = strings.Repeat("x", 16*1024+1)
	if _, err = reviewRequirements(input); err == nil {
		t.Fatal("oversized prompt accepted")
	}
}
