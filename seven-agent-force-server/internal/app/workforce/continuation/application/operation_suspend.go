package application

import (
	"context"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/jackc/pgx/v5"
)

// OperationResumeRequest is produced only after executionaccess has persisted
// one authoritative user decision in the caller-owned transaction.
type OperationResumeRequest struct {
	TenantID, TaskID, SourceTaskTurnID, SourceAttemptID  int64
	OperationID, CheckpointID, DecisionID, CreatorUserID int64
	Action                                               string
}

type OperationResumeResult struct {
	TaskTurnID, SourceCheckpointID int64
	Replayed                       bool
}

type OperationResumeRepository interface {
	LoadOperationResumeTx(context.Context, pgx.Tx, OperationResumeRequest) (domain.InteractionSuspend, error)
	PersistOperationResumeTx(context.Context, pgx.Tx, OperationResumeRequest, CreateNextTurnRequest) (OperationResumeResult, error)
}

type OperationResumeService struct{ repository OperationResumeRepository }

func NewOperationResume(repository OperationResumeRepository) *OperationResumeService {
	return &OperationResumeService{repository: repository}
}

// ResumeOperationTx neither starts nor commits a transaction. The decision,
// checkpoint consumption and successor Turn are one atomic state change.
func (s *OperationResumeService) ResumeOperationTx(ctx context.Context, tx pgx.Tx, r OperationResumeRequest) (OperationResumeResult, error) {
	if s == nil || s.repository == nil || tx == nil || r.TenantID <= 0 || r.TaskID <= 0 || r.SourceTaskTurnID <= 0 || r.SourceAttemptID <= 0 || r.OperationID <= 0 || r.CheckpointID <= 0 || r.DecisionID <= 0 || r.CreatorUserID <= 0 || (r.Action != "ALLOW_ONCE" && r.Action != "ALLOW_FOR_TASK" && r.Action != "DENY") {
		return OperationResumeResult{}, domain.ErrInteractionSuspend
	}
	source, err := s.repository.LoadOperationResumeTx(ctx, tx, r)
	if err != nil {
		return OperationResumeResult{}, err
	}
	if err = source.ValidateOperationResume(r.TaskID, r.SourceTaskTurnID, r.SourceAttemptID, r.OperationID, time.Now().UTC()); err != nil {
		return OperationResumeResult{}, err
	}
	if err = domain.ValidateInstruction(source.Instruction); err != nil {
		return OperationResumeResult{}, err
	}
	return s.repository.PersistOperationResumeTx(ctx, tx, r, CreateNextTurnRequest{TaskID: r.TaskID, TenantID: r.TenantID, CreatorUserID: r.CreatorUserID, SourceCheckpointID: r.CheckpointID, ExpectedCurrentTurnID: r.SourceTaskTurnID, Instruction: source.Instruction, IdempotencyKey: fmt.Sprintf("operation:%d:decision:%d", r.OperationID, r.DecisionID)})
}
