package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

const PurposeInteractionSuspend = "INTERACTION_SUSPEND"
const InteractionSuspended = "INTERACTION_SUSPENDED"

var ErrInteractionSuspend = errors.New("INTERACTION_SUSPEND_UNAVAILABLE")
var ErrInteractionSuspendAnswerWon = errors.New("INTERACTION_SUSPEND_ANSWER_WON")
var ErrInteractionSuspendInProgress = errors.New("INTERACTION_SUSPEND_IN_PROGRESS")

// Independent namespace: an interaction occurrence is not an evaluation cycle.
func InteractionCheckpointIdentity(taskID, turnID, attemptID, generation, interactionID, sessionGeneration, round int64, snapshotDigest, controlSHA256 string) string {
	// Reservation precedes native stop. The descriptor can be absent until
	// suspend completes; encrypted verified manifest freezes it at publication.
	sum := sha256.Sum256([]byte(fmt.Sprintf("seven.interaction-suspend/v2\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d\x00%s\x00%s", taskID, turnID, attemptID, generation, interactionID, sessionGeneration, round, snapshotDigest, controlSHA256)))
	return hex.EncodeToString(sum[:])
}

// OperationCheckpointIdentity deliberately uses a different namespace from a
// user question. An execution Operation is an authorization/execution fact and
// must never be materialized as a synthetic interaction row.
func OperationCheckpointIdentity(taskID, turnID, attemptID, generation, operationID, sessionGeneration, round int64, snapshotDigest, controlSHA256 string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("seven.operation-suspend/v1\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d\x00%s\x00%s", taskID, turnID, attemptID, generation, operationID, sessionGeneration, round, snapshotDigest, controlSHA256)))
	return hex.EncodeToString(sum[:])
}

// InteractionSuspend is private continuation authority, never a result decision.
// It confers no artifact, delivery, approval or accepted-risk authority.
type InteractionSuspend struct {
	Source                                   Source
	Purpose, Phase, TaskStatus, TurnStatus   string
	TaskID, TurnID, AttemptID, InteractionID int64
	OperationID                              int64
	CurrentTurnID, CurrentAttemptID          int64
	FormatVersion                            int
	Cancelled                                bool
	SnapshotDigest, CheckpointSnapshotDigest string
	SessionDigest                            string
	Instruction                              string
	AttemptStatus, RuntimeStatus             string
	AuthoritySealed                          bool
}

func (s InteractionSuspend) validBase(taskID, turnID, attemptID int64, now time.Time) bool {
	return s.TaskID == taskID && s.TurnID == turnID && s.AttemptID == attemptID &&
		s.CurrentTurnID == turnID && s.CurrentAttemptID == attemptID && !s.Cancelled && s.TaskStatus == InteractionSuspended && s.TurnStatus == InteractionSuspended && s.AttemptStatus == InteractionSuspended && s.RuntimeStatus == InteractionSuspended && s.AuthoritySealed &&
		s.Purpose == PurposeInteractionSuspend && s.Phase == "SUSPENDED" && s.Source.CheckpointDecision == "" &&
		s.FormatVersion == 2 && s.Source.CheckpointStatus == "AVAILABLE" && s.Source.ConsumedByTurnID == 0 &&
		s.Source.CheckpointExpires.After(now) && s.SnapshotDigest != "" && s.SnapshotDigest == s.CheckpointSnapshotDigest && s.SessionDigest != ""
}

func (s InteractionSuspend) ValidateResume(taskID, turnID, attemptID, interactionID int64, now time.Time) error {
	if interactionID <= 0 || s.InteractionID != interactionID || s.OperationID != 0 || !s.validBase(taskID, turnID, attemptID, now) {
		return ErrInteractionSuspend
	}
	return nil
}

func (s InteractionSuspend) ValidateOperationResume(taskID, turnID, attemptID, operationID int64, now time.Time) error {
	if operationID <= 0 || s.OperationID != operationID || s.InteractionID != 0 || !s.validBase(taskID, turnID, attemptID, now) {
		return ErrInteractionSuspend
	}
	return nil
}
