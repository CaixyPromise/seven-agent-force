package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeHumanReasonRejectsUnsafeInput(t *testing.T) {
	for _, value := range []string{
		"",
		"   ",
		"contains\na control split",
		"Authorization: Bearer top-secret",
		"api_key=top-secret",
		"token: top-secret",
		strings.Repeat("a", 2001),
	} {
		if _, err := NormalizeHumanReason(value); !errors.Is(err, ErrHumanReasonInvalid) {
			t.Fatalf("NormalizeHumanReason(%q) error=%v, want reason invalid", value, err)
		}
	}
	got, err := NormalizeHumanReason("  Reviewed remaining risk.  ")
	if err != nil || got != "Reviewed remaining risk." {
		t.Fatalf("safe reason=%q err=%v", got, err)
	}
}

func TestHumanDecisionOperationBindingContainsOnlyDigests(t *testing.T) {
	binding := HumanDecisionOperationBinding(123, HumanActionAuthorizeRevision, "private operator reason", "private correction guidance", "private-idempotency-key")
	if !strings.HasPrefix(binding, "workforce:review-run:123|action:AUTHORIZE_ONE_REVISION|reason:") {
		t.Fatalf("unexpected binding: %s", binding)
	}
	if strings.Contains(binding, "private operator reason") || strings.Contains(binding, "private correction guidance") || strings.Contains(binding, "private-idempotency-key") {
		t.Fatalf("operation binding exposed request material: %s", binding)
	}
	parts := strings.Split(binding, "|")
	if len(parts) != 5 || len(strings.TrimPrefix(parts[2], "reason:")) != 64 || len(strings.TrimPrefix(parts[3], "guidance:")) != 64 || len(strings.TrimPrefix(parts[4], "idempotency:")) != 64 {
		t.Fatalf("operation binding does not contain stable sha256 identities: %s", binding)
	}
}

func TestNormalizeRevisionGuidance(t *testing.T) {
	if got, err := NormalizeRevisionGuidance(HumanActionAuthorizeRevision, "  keep API compatibility  "); err != nil || got != "keep API compatibility" {
		t.Fatalf("guidance=%q err=%v", got, err)
	}
	if got, err := NormalizeRevisionGuidance(HumanActionAuthorizeRevision, "  "); err != nil || got != "" {
		t.Fatalf("optional guidance=%q err=%v", got, err)
	}
	for _, value := range []string{"token=secret-value", "line one\nline two", strings.Repeat("界", 1334)} {
		if _, err := NormalizeRevisionGuidance(HumanActionAuthorizeRevision, value); !errors.Is(err, ErrHumanGuidanceInvalid) {
			t.Fatalf("unsafe guidance accepted: %q err=%v", value, err)
		}
	}
	if _, err := NormalizeRevisionGuidance(HumanActionAcceptRisk, "do more work"); !errors.Is(err, ErrHumanGuidanceInvalid) {
		t.Fatalf("non-revision guidance error=%v", err)
	}
	if got, err := NormalizeRevisionGuidance(HumanActionCancelTask, ""); err != nil || got != "" {
		t.Fatalf("empty non-revision guidance=%q err=%v", got, err)
	}
}
