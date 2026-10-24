package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
)

func TestRecoveryCodePrepareExposesSingleUseHints(t *testing.T) {
	provider := NewRecoveryCodeChallengeStepProvider(&fakeSubjectStore{})
	step := &domain.ChallengeStep{ChallengeType: domain.ChallengeTypeRecoveryCodeVerification}

	if err := provider.Prepare(context.Background(), &domain.ChallengeSession{}, step); err != nil {
		t.Fatalf("prepare recovery code step: %v", err)
	}
	if step.UserInterfaceHints["format"] != "XXXX-XXXX-XXXX" {
		t.Fatalf("unexpected format hint: %+v", step.UserInterfaceHints["format"])
	}
	if step.UserInterfaceHints["singleUse"] != true {
		t.Fatalf("unexpected singleUse hint: %+v", step.UserInterfaceHints["singleUse"])
	}
}

func TestRecoveryCodeEligibleRequiresAvailableCodes(t *testing.T) {
	provider := NewRecoveryCodeChallengeStepProvider(&fakeSubjectStore{recoveryCount: 1})

	eligible, err := provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{})
	if err != nil {
		t.Fatalf("check recovery code eligibility: %v", err)
	}
	if !eligible {
		t.Fatal("expected recovery code step to be eligible when codes are available")
	}
}

func TestRecoveryCodeEligibleRejectsMissingCodes(t *testing.T) {
	provider := NewRecoveryCodeChallengeStepProvider(&fakeSubjectStore{})

	eligible, err := provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{})
	if err != nil {
		t.Fatalf("check recovery code eligibility: %v", err)
	}
	if eligible {
		t.Fatal("expected recovery code step to be ineligible when no codes are available")
	}
}

func TestRecoveryCodeVerifyConsumesOnce(t *testing.T) {
	store := &fakeSubjectStore{consumeResults: []bool{true, false}}
	provider := NewRecoveryCodeChallengeStepProvider(store)
	session := &domain.ChallengeSession{SubjectIdentifier: "login:alice"}
	step := &domain.ChallengeStep{ChallengeType: domain.ChallengeTypeRecoveryCodeVerification}

	first, err := provider.Verify(context.Background(), session, step, map[string]any{"recoveryCode": "ABCD-EFGH-IJKL"})
	if err != nil {
		t.Fatalf("first consume: %v", err)
	}
	replay, err := provider.Verify(context.Background(), session, step, map[string]any{"recoveryCode": "ABCD-EFGH-IJKL"})
	if err != nil {
		t.Fatalf("replay consume: %v", err)
	}
	if !first {
		t.Fatalf("expected first consume to pass")
	}
	if replay {
		t.Fatalf("expected replay consume to fail")
	}
}

func TestRecoveryCodeProviderDoesNotExposeSubmittedCode(t *testing.T) {
	store := &fakeSubjectStore{consumeResults: []bool{true}}
	provider := NewRecoveryCodeChallengeStepProvider(store)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		SessionContext: map[string]any{
			"existing": "metadata",
		},
	}
	step := &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeRecoveryCodeVerification,
		UserInterfaceHints: map[string]any{
			"format": "XXXX-XXXX-XXXX",
		},
	}

	ok, err := provider.Verify(
		context.Background(),
		session,
		step,
		map[string]any{"recoveryCode": "ABCD-EFGH-IJKL"},
	)
	if err != nil {
		t.Fatalf("verify recovery code: %v", err)
	}
	if !ok {
		t.Fatal("expected recovery code to pass")
	}

	serialized, err := json.Marshal(map[string]any{
		"sessionContext":    session.SessionContext,
		"userInterfaceHint": step.UserInterfaceHints,
	})
	if err != nil {
		t.Fatalf("serialize recovery code leak surface: %v", err)
	}
	for _, forbidden := range []string{"ABCD-EFGH-IJKL", "recoveryCode"} {
		if strings.Contains(string(serialized), forbidden) {
			t.Fatalf("recovery code provider exposed submitted code material: %s", serialized)
		}
	}
}

func TestRecoveryCodeVerifyRejectsBlankCode(t *testing.T) {
	provider := NewRecoveryCodeChallengeStepProvider(&fakeSubjectStore{})
	ok, err := provider.Verify(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{}, map[string]any{"recoveryCode": "  "})
	if err != nil {
		t.Fatalf("verify blank recovery code: %v", err)
	}
	if ok {
		t.Fatalf("expected blank recovery code to fail")
	}
}

func TestRecoveryCodeVerifyHandlesNilSession(t *testing.T) {
	provider := NewRecoveryCodeChallengeStepProvider(&fakeSubjectStore{})
	ok, err := provider.Verify(context.Background(), nil, &domain.ChallengeStep{}, map[string]any{"recoveryCode": "ABCD-EFGH-IJKL"})
	if err == nil {
		t.Fatalf("expected nil session to return params error")
	}
	if ok {
		t.Fatalf("expected nil session to fail")
	}
}
