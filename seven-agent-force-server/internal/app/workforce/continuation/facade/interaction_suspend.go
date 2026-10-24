package facade

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	workercheckpoint "github.com/CaixyPromise/seven-agent/force/internal/worker/checkpoint"
	"github.com/jackc/pgx/v5"
)

type InteractionResumeRequest = application.InteractionResumeRequest
type InteractionResumeResult = application.InteractionResumeResult
type InteractionSuspendOccurrence = application.InteractionSuspendOccurrence
type InteractionSuspendOccurrenceGate = application.InteractionSuspendOccurrenceGate
type OperationSuspendOccurrence = application.OperationSuspendOccurrence
type OperationSuspendOccurrenceGate = application.OperationSuspendOccurrenceGate
type ReserveInteractionRequest = application.ReserveInteractionRequest
type InteractionReservationResult = application.InteractionReservationResult
type PublishInteractionRequest = application.PublishInteractionRequest
type InteractionCaptureRequest = application.InteractionCaptureRequest
type InteractionEncryptedBlob = application.InteractionEncryptedBlob
type InteractionReservation = application.InteractionReservation

const InteractionSuspended = domain.InteractionSuspended

var ErrInteractionSuspendAnswerWon = domain.ErrInteractionSuspendAnswerWon
var ErrInteractionSuspendInProgress = domain.ErrInteractionSuspendInProgress

// Suspender is the purpose-specific machine HTTP integration seam. HTTP owns
// authenticated envelope decoding and calibrates Task/Turn from Attempt scope.
type Suspender interface {
	ReserveInteractionSuspend(context.Context, ReserveInteractionRequest) (InteractionReservationResult, error)
	PublishInteractionSuspend(context.Context, PublishInteractionRequest) error
}

var _ Suspender = (*application.InteractionSuspendService)(nil)

type InteractionRestoreInput = workercheckpoint.RestoreInput
type InteractionRestoreResolver interface {
	ResolveInteractionRestoreInput(context.Context, application.MachineAuthority, int64) (InteractionRestoreInput, error)
}

type InteractionResumeLookup struct{ TenantID, TaskID, AttemptID, InteractionID int64 }
type InteractionResumeTarget struct{ SourceTaskTurnID, CheckpointID int64 }
type InteractionResumeFinder interface {
	FindInteractionResumeTx(context.Context, pgx.Tx, InteractionResumeLookup) (*InteractionResumeTarget, error)
}

// InteractionResumer is scoped to an authorized, persisted cold answer. Missing
// implementation must fail closed. Caller owns transaction commit/rollback.
type InteractionResumer interface {
	ResumeInteractionTx(context.Context, pgx.Tx, InteractionResumeRequest) (InteractionResumeResult, error)
}

type OperationResumeRequest = application.OperationResumeRequest
type OperationResumeResult = application.OperationResumeResult
type OperationResumeLookup struct{ TenantID, TaskID, AttemptID, OperationID int64 }
type OperationResumeTarget struct{ SourceTaskTurnID, CheckpointID, ResumedTaskTurnID int64 }
type OperationResumeFinder interface {
	FindOperationResumeTx(context.Context, pgx.Tx, OperationResumeLookup) (*OperationResumeTarget, error)
}
type OperationResumer interface {
	ResumeOperationTx(context.Context, pgx.Tx, OperationResumeRequest) (OperationResumeResult, error)
}
