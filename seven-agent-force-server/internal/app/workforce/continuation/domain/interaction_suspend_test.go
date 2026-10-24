package domain

import (
	"strings"
	"testing"
	"time"
)

func TestInteractionSuspendPurposeAndAuthority(t *testing.T) {
	now := time.Now().UTC()
	valid := InteractionSuspend{TaskID: 1, TurnID: 2, AttemptID: 3, InteractionID: 4, CurrentTurnID: 2, CurrentAttemptID: 3, Purpose: PurposeInteractionSuspend, Phase: "SUSPENDED", TaskStatus: "RUNNING", TurnStatus: TurnActive, FormatVersion: 2, SnapshotDigest: "frozen", CheckpointSnapshotDigest: "frozen", SessionDigest: "private-lineage", Source: Source{CheckpointStatus: "AVAILABLE", CheckpointExpires: now.Add(time.Hour)}}
	valid.TaskStatus = InteractionSuspended
	valid.TurnStatus = InteractionSuspended
	valid.AttemptStatus = InteractionSuspended
	valid.RuntimeStatus = InteractionSuspended
	valid.AuthoritySealed = true
	if err := valid.ValidateResume(1, 2, 3, 4, now); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*InteractionSuspend){
		"unsealed authority":  func(s *InteractionSuspend) { s.AuthoritySealed = false },
		"live attempt":        func(s *InteractionSuspend) { s.AttemptStatus = "RUNNING" },
		"live runtime":        func(s *InteractionSuspend) { s.RuntimeStatus = "RUNNING" },
		"cancel":              func(s *InteractionSuspend) { s.Cancelled = true },
		"task replaced":       func(s *InteractionSuspend) { s.CurrentTurnID++ },
		"attempt replaced":    func(s *InteractionSuspend) { s.CurrentAttemptID++ },
		"wrong occurrence":    func(s *InteractionSuspend) { s.InteractionID++ },
		"capture incomplete":  func(s *InteractionSuspend) { s.Phase = "CAPTURING" },
		"readback incomplete": func(s *InteractionSuspend) { s.Source.CheckpointStatus = "PENDING" },
		"consumed":            func(s *InteractionSuspend) { s.Source.ConsumedByTurnID = 9 },
		"expired":             func(s *InteractionSuspend) { s.Source.CheckpointExpires = now },
		"v1":                  func(s *InteractionSuspend) { s.FormatVersion = 1 },
		"result purpose":      func(s *InteractionSuspend) { s.Purpose = "RESULT" },
		"PASS laundering":     func(s *InteractionSuspend) { s.Source.CheckpointDecision = DecisionPass },
		"WAIT laundering":     func(s *InteractionSuspend) { s.Source.CheckpointDecision = DecisionWaitUser },
		"identity drift":      func(s *InteractionSuspend) { s.CheckpointSnapshotDigest = "new" },
		"session missing":     func(s *InteractionSuspend) { s.SessionDigest = "" },
		"completed task":      func(s *InteractionSuspend) { s.TaskStatus = "SUCCEEDED" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			change(&candidate)
			if err := candidate.ValidateResume(1, 2, 3, 4, now); err != ErrInteractionSuspend {
				t.Fatalf("got %v", err)
			}
		})
	}
	// The ordinary result continuation validator never accepts the new purpose.
	source := valid.Source
	source.CurrentTurnID = 2
	source.CurrentTurnStatus = TurnActive
	source.CheckpointDecision = PurposeInteractionSuspend
	if err := source.ValidateCreate(2, now); err != ErrContinuationNotAllowed {
		t.Fatalf("ordinary continuation accepted suspend: %v", err)
	}
}

func TestOperationSuspendHasIndependentCheckpointIdentityAndResumeAuthority(t *testing.T) {
	now := time.Now().UTC()
	interactionIdentity := InteractionCheckpointIdentity(1, 2, 3, 4, 9, 5, 6, "snapshot", strings.Repeat("a", 64))
	operationIdentity := OperationCheckpointIdentity(1, 2, 3, 4, 9, 5, 6, "snapshot", strings.Repeat("a", 64))
	if interactionIdentity == operationIdentity {
		t.Fatal("operation checkpoint was aliased to a question identity")
	}
	valid := InteractionSuspend{TaskID: 1, TurnID: 2, AttemptID: 3, OperationID: 9, CurrentTurnID: 2, CurrentAttemptID: 3, Purpose: PurposeInteractionSuspend, Phase: "SUSPENDED", TaskStatus: InteractionSuspended, TurnStatus: InteractionSuspended, AttemptStatus: InteractionSuspended, RuntimeStatus: InteractionSuspended, FormatVersion: 2, SnapshotDigest: "snapshot", CheckpointSnapshotDigest: "snapshot", SessionDigest: "lineage", AuthoritySealed: true, Source: Source{CheckpointStatus: "AVAILABLE", CheckpointExpires: now.Add(time.Hour)}}
	if err := valid.ValidateOperationResume(1, 2, 3, 9, now); err != nil {
		t.Fatal(err)
	}
	valid.InteractionID = 8
	if err := valid.ValidateOperationResume(1, 2, 3, 9, now); err != ErrInteractionSuspend {
		t.Fatalf("mixed question/operation target accepted: %v", err)
	}
}
