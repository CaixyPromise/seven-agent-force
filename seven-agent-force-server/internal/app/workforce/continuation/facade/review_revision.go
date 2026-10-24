// Package facade exposes the narrow transaction seam used to turn a trusted
// independent-review result into a normal continuation TaskTurn.
package facade

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	ErrReviewRevisionCheckpointUnavailable = errors.New("INDEPENDENT_REVIEW_CHECKPOINT_UNAVAILABLE")
	ErrReviewRevisionIdentityConflict      = errors.New("INDEPENDENT_REVIEW_REVISION_IDENTITY_CONFLICT")
)

const (
	RevisionAutomatic       = "AUTOMATIC"
	RevisionHumanAuthorized = "HUMAN_AUTHORIZED"
	// ReviewReportPathPlaceholder is replaced by the Worker with the exact
	// Attempt-private, read-only container path before the Harness starts.
	ReviewReportPathPlaceholder = "{{SEVEN_REVIEW_REPORT_PATH}}"
	ManagedCorrectionMarker     = "\n\n[System-managed independent review correction]\n"
)

// PublicInstruction removes the machine-managed correction contract from
// creator-facing Task and Turn responses. The complete instruction remains
// available only to the trusted dispatch path.
func PublicInstruction(instruction string) string {
	if index := strings.Index(instruction, ManagedCorrectionMarker); index >= 0 {
		return strings.TrimSpace(instruction[:index])
	}
	return strings.TrimSpace(instruction)
}

// ReviewRevisionTurnRequest contains only server-authoritative facts. The
// canonical report is never accepted from an HTTP caller through this seam.
type ReviewRevisionTurnRequest struct {
	TenantID          int64
	TaskID            int64
	SourceTaskTurnID  int64
	SourceAttemptID   int64
	SourceReviewRunID int64
	RevisionKind      string
	ReportVerdict     string
	ReportDigest      string
	CanonicalReport   json.RawMessage
	SafeReasonCode    string
	RevisionGuidance  string
	TriggerUserID     int64
}

type ReviewRevisionTurnResult struct {
	TaskTurnID         int64
	SourceCheckpointID int64
	TurnNumber         int
	Instruction        string
	Replayed           bool
}

// ReviewRevisionTurnCreator participates in the ReviewRun report/decision
// transaction. Implementations must not begin or commit a nested transaction.
type ReviewRevisionTurnCreator interface {
	CreateReviewRevisionTurn(context.Context, pgx.Tx, ReviewRevisionTurnRequest) (ReviewRevisionTurnResult, error)
}
