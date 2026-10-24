package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTurnCreatorIdentityUsesLosslessUserID(t *testing.T) {
	body, err := json.Marshal(Turn{ID: 637649102, CreatedBy: 2065424359060983808})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"createdBy":"2065424359060983808"`) {
		t.Fatalf("creator id must be encoded losslessly, got %s", body)
	}
}

func TestSourceValidateCreate(t *testing.T) {
	now := time.Unix(100, 0)
	approved := now.Add(-time.Second)
	tests := []struct {
		name   string
		source Source
		want   error
	}{
		{name: "pass", source: Source{CurrentTurnID: 637649102, CurrentTurnStatus: TurnSucceeded, CheckpointDecision: DecisionPass, CheckpointStatus: "AVAILABLE", CheckpointExpires: now.Add(time.Hour)}},
		{name: "wait user", source: Source{CurrentTurnID: 637649102, CurrentTurnStatus: TurnReviewing, CheckpointDecision: DecisionWaitUser, CheckpointStatus: "AVAILABLE", CheckpointExpires: now.Add(time.Hour)}},
		{name: "manual approved", source: Source{CurrentTurnID: 637649102, CurrentTurnStatus: TurnManualReview, CheckpointDecision: DecisionManualReview, CheckpointStatus: "AVAILABLE", CheckpointExpires: now.Add(time.Hour), ApprovedAt: &approved}},
		{name: "manual requires approval", source: Source{CurrentTurnID: 637649102, CurrentTurnStatus: TurnManualReview, CheckpointDecision: DecisionManualReview, CheckpointStatus: "AVAILABLE", CheckpointExpires: now.Add(time.Hour)}, want: ErrManualApprovalRequired},
		{name: "expired", source: Source{CurrentTurnID: 637649102, CurrentTurnStatus: TurnSucceeded, CheckpointDecision: DecisionPass, CheckpointStatus: "AVAILABLE", CheckpointExpires: now}, want: ErrCheckpointExpired},
		{name: "consumed", source: Source{CurrentTurnID: 637649102, CurrentTurnStatus: TurnSucceeded, CheckpointDecision: DecisionPass, CheckpointStatus: "AVAILABLE", CheckpointExpires: now.Add(time.Hour), ConsumedByTurnID: 3171578228}, want: ErrTaskTurnAlreadyActive},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.source.ValidateCreate(637649102, now)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidateCreate() error=%v want=%v", err, tc.want)
			}
		})
	}
}

func TestSourceRejectsStaleCurrentTurn(t *testing.T) {
	source := Source{CurrentTurnID: 3171578228, CurrentTurnStatus: TurnSucceeded, CheckpointDecision: DecisionPass, CheckpointStatus: "AVAILABLE", CheckpointExpires: time.Now().Add(time.Hour)}
	if err := source.ValidateCreate(1001, time.Now()); !errors.Is(err, ErrTaskTurnConflict) {
		t.Fatalf("stale current turn accepted: %v", err)
	}
}
