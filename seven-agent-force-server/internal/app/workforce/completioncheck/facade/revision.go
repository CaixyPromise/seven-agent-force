package facade

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"time"
)

var ErrRevisionUnavailable = errors.New("COMPLETION_CHECK_REVISION_UNAVAILABLE")

type RevisionUser struct {
	TenantID, TaskID, UserID int64
	CanContinue              bool
}

const RequestRevision = "REQUEST_REVISION"

type RevisionView struct {
	ExpectedVersion int64     `json:"expectedVersion,string"`
	CompletionCheck Input     `json:"completionCheck"`
	Pending         *Revision `json:"pending"`
	AllowedActions  []string  `json:"allowedActions"`
}
type RevisionRequest struct {
	ExpectedVersion int64  `json:"expectedVersion,string"`
	IdempotencyKey  string `json:"idempotencyKey"`
	CompletionCheck *Input `json:"completionCheck"`
}
type Revision struct {
	ID                                                          int64       `json:"id,string"`
	TenantID                                                    int64       `json:"-"`
	TaskID                                                      int64       `json:"taskId,string"`
	SourceTaskTurnID                                            int64       `json:"sourceTaskTurnId,string"`
	SourceAttemptID                                             int64       `json:"-"`
	SourceVersion                                               int64       `json:"sourceVersion,string"`
	RevisionNumber                                              int64       `json:"revisionNumber,string"`
	Version                                                     int64       `json:"version,string"`
	Status                                                      string      `json:"status"`
	CreatorUserID, PrincipalID                                  int64       `json:"-"`
	IdempotencyKey, RequestDigest                               string      `json:"-"`
	Base                                                        Input       `json:"-"`
	BaseDigest                                                  string      `json:"baseDigest"`
	Input                                                       Input       `json:"completionCheck"`
	References                                                  []Reference `json:"-"`
	InputDigest                                                 string      `json:"-"`
	CheckDigest                                                 string      `json:"checkDigest"`
	SourceBaselineResultID                                      int64       `json:"sourceBaselineResultId,string,omitempty"`
	SourceCommit, SourceIdentityDigest, ExecutionSnapshotDigest string      `json:"-"`
	ProcessorReleaseID                                          int64       `json:"-"`
	ProcessorDigest                                             string      `json:"-"`
	AppliedTurnID                                               int64       `json:"appliedTurnId,string,omitempty"`
	CreatedAt                                                   time.Time   `json:"createdAt"`
	AppliedAt                                                   *time.Time  `json:"appliedAt,omitempty"`
	Replayed                                                    bool        `json:"replayed"`
}

// PendingReader runs within an already Task-authorized caller transaction.
// Its read never consumes the revision or claims a runtime safe point.
type PendingReader interface {
	PendingRevisionTx(context.Context, pgx.Tx, int64, int64, int64) (*Revision, error)
}
type RevisionSafePoint struct {
	TenantID, TaskID, TaskTurnID, AttemptID, CheckpointID, RevisionID, ExpectedVersion int64
	Authority                                                                          MachineAuthority
	ExecutionLease                                                                     string
}

// ValidateRevisionSafePoint must lock current Task first and verify current
// Worker/instance/boot/generation/owner/lease/cancel plus independently confirmed
// stop/quiescence and exact checkpoint binding. Admission/output is not authority.
type RevisionSafePointAuthority interface {
	ValidateRevisionSafePoint(context.Context, pgx.Tx, RevisionSafePoint) error
}
type RevisionApplier interface {
	ApplyRevisionTx(context.Context, pgx.Tx, RevisionSafePoint) (Revision, error)
}

// ResolveRevisionSafePoint authorizes the authenticated machine and locks Task
// first, deriving all source/revision/checkpoint identities from persisted state.
// Nil means authorized but no pending or previously applied revision for this
// exact attempt/cycle. Lost-response replay resolves the APPLIED revision and its
// consumed checkpoint with ExpectedVersion=Version-1; fencing remains mandatory.
type RevisionSafePointResolver interface {
	ResolveRevisionSafePoint(context.Context, pgx.Tx, MachineAuthority, int64, string, int64) (*RevisionSafePoint, error)
}

// SafePointRevisionLookup is called only after caller-owned Task/machine
// authorization. It never reads another module's checkpoint/runtime tables.
type SafePointRevisionLookup interface {
	FindSafePointRevisionTx(context.Context, pgx.Tx, int64, int64, int64, int64) (*Revision, error)
}
type ApplyPendingResult struct {
	Applied    bool  `json:"applied"`
	TaskTurnID int64 `json:"taskTurnId,string"`
}
type RevisionMachineControl interface {
	ApplyPendingAtSafePoint(context.Context, MachineAuthority, int64, string, int64) (ApplyPendingResult, error)
}
type RevisionRequester interface {
	RequestRevision(context.Context, RevisionUser, RevisionRequest) (Revision, error)
}
