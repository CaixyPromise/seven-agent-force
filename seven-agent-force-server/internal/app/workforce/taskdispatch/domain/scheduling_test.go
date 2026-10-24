package domain

import (
	"testing"
	"time"
)

func TestEffectivePriorityAgesEveryFiveMinutesAndCaps(t *testing.T) {
	created := time.Date(2026, 8, 26, 1, 0, 0, 0, time.UTC)
	if got := EffectivePriority(7, created, created.Add(14*time.Minute+59*time.Second)); got != 9 {
		t.Fatalf("effective priority = %d, want 9", got)
	}
	if got := EffectivePriority(99, created, created.Add(10*time.Minute)); got != 100 {
		t.Fatalf("capped effective priority = %d, want 100", got)
	}
}
