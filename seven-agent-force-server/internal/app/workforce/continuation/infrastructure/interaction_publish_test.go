package infrastructure

import (
	"errors"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
)

func TestInteractionActiveLimitUsesAttemptSnapshotWhenPresent(t *testing.T) {
	got, err := interactionActiveLimitMillis([]byte(`{"executionDeadlineSeconds":900}`))
	if err != nil || got != 900_000 {
		t.Fatalf("limit = %d, err = %v", got, err)
	}
}

func TestInteractionActiveLimitIsUnlimitedWithoutAttemptSnapshot(t *testing.T) {
	got, err := interactionActiveLimitMillis(nil)
	if err != nil || got != 0 {
		t.Fatalf("limit = %d, err = %v", got, err)
	}
}

func TestInteractionActiveLimitFailsClosedForMissingOrInvalidIdentity(t *testing.T) {
	for _, test := range []struct {
		name    string
		attempt string
	}{
		{name: "missing Attempt deadline", attempt: `{}`},
		{name: "malformed Attempt snapshot", attempt: `[]`},
		{name: "negative Attempt deadline", attempt: `{"executionDeadlineSeconds":-1}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := interactionActiveLimitMillis([]byte(test.attempt))
			if !errors.Is(err, domain.ErrInteractionSuspend) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
