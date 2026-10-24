package facade

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// CompletionRevisionTurnCreator is a dedicated safe-point continuation seam.
// Implementations must validate a genuine RESULT checkpoint produced after a
// real Processor PASS or WAIT_USER, independently stopped before Finalizer,
// current Task/Turn/Attempt, cancellation and lease/owner fences in tx; atomically
// consume it and create one normal continuation Turn. Never borrow an interaction
// checkpoint by inventing a question. Preserve execution/processor identity,
// mandatory gates and ORIGINAL source input snapshot. Retain historical baseline
// evidence, but clear the new Turn's baseline pointer for the changed check digest.
// The caller owns tx; a returned Turn is independently read back before commit.
type CompletionRevisionTurnCreator interface {
	CreateCompletionRevisionTurnTx(context.Context, pgx.Tx, CompletionRevisionTurnRequest) (CompletionRevisionTurnResult, error)
}
type CompletionRevisionTurnRequest struct {
	TenantID, TaskID, SourceTaskTurnID, SourceAttemptID, CheckpointID, RevisionID, CreatorUserID int64
	OriginalBaselineResultID                                                                     int64
	SourceCommit, SourceIdentityDigest, ExecutionSnapshotDigest                                  string
	InputJSON, ReferencesJSON                                                                    json.RawMessage
	InputDigest, CheckDigest                                                                     string
}
type CompletionRevisionTurnResult struct{ TaskTurnID int64 }
