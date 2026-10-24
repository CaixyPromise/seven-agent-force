package domain

import (
	"errors"
	"testing"
)

func TestFenceViolationPreservesSentinelAndSafeStage(t *testing.T) {
	err := NewFenceViolation(FenceStagePermit)
	if !errors.Is(err, ErrFenceRejected) {
		t.Fatal("fence violation no longer matches the stable sentinel")
	}
	if got := FenceViolationStage(err); got != FenceStagePermit {
		t.Fatalf("stage = %q, want %q", got, FenceStagePermit)
	}
	if got := err.Error(); got != ErrFenceRejected.Error() {
		t.Fatalf("error disclosed predicate details: %q", got)
	}
}
