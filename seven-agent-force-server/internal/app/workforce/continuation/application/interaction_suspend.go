package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/jackc/pgx/v5"
)

// These are server-owned inputs. An answer is persisted and authorized by the
// interaction application before entering this same transaction.
type InteractionResumeRequest struct {
	TenantID, TaskID, SourceTaskTurnID, SourceAttemptID  int64
	InteractionID, CheckpointID, AnswerID, CreatorUserID int64
	Instruction                                          string
	InteractionResponse                                  json.RawMessage
}

type InteractionResumeResult struct {
	TaskTurnID, SourceCheckpointID int64
	Replayed                       bool
}

type InteractionResumeRepository interface {
	// Locks Task first, then source Turn/checkpoint; cancel serializes on Task.
	LoadInteractionResumeTx(context.Context, pgx.Tx, InteractionResumeRequest) (domain.InteractionSuspend, error)
	PersistInteractionResumeTx(context.Context, pgx.Tx, InteractionResumeRequest, CreateNextTurnRequest) (InteractionResumeResult, error)
}

type InteractionResumeService struct{ repository InteractionResumeRepository }

func NewInteractionResume(repository InteractionResumeRepository) *InteractionResumeService {
	return &InteractionResumeService{repository: repository}
}

// ResumeInteractionTx never begins/commits the caller's answer transaction.
// Every error requires the caller to roll it back. Short questions must use the
// existing Attempt's answer delivery path and never call this cold-resume seam.
func (s *InteractionResumeService) ResumeInteractionTx(ctx context.Context, tx pgx.Tx, r InteractionResumeRequest) (InteractionResumeResult, error) {
	r.Instruction = strings.TrimSpace(r.Instruction)
	if s == nil || s.repository == nil || tx == nil || r.TenantID <= 0 || r.TaskID <= 0 || r.SourceTaskTurnID <= 0 || r.SourceAttemptID <= 0 || r.InteractionID <= 0 || r.CheckpointID <= 0 || r.AnswerID <= 0 || r.CreatorUserID <= 0 {
		return InteractionResumeResult{}, domain.ErrInteractionSuspend
	}
	if err := domain.ValidateInstruction(r.Instruction); err != nil {
		return InteractionResumeResult{}, err
	}
	var response struct {
		InteractionKey string          `json:"interactionKey"`
		ResponseID     int64           `json:"responseId,string"`
		Answers        json.RawMessage `json:"answers"`
	}
	if len(r.InteractionResponse) > 16*1024 || json.Unmarshal(r.InteractionResponse, &response) != nil || response.InteractionKey == "" || len(response.InteractionKey) > 256 || response.ResponseID != r.AnswerID || len(response.Answers) == 0 {
		return InteractionResumeResult{}, domain.ErrInteractionSuspend
	}
	source, err := s.repository.LoadInteractionResumeTx(ctx, tx, r)
	if err != nil {
		return InteractionResumeResult{}, err
	}
	if err = source.ValidateResume(r.TaskID, r.SourceTaskTurnID, r.SourceAttemptID, r.InteractionID, time.Now().UTC()); err != nil {
		return InteractionResumeResult{}, err
	}
	// The saved answer has its own typed restore input and delivery receipt.
	// Resuming a pending question must retain the user's existing goal.
	if err = domain.ValidateInstruction(source.Instruction); err != nil {
		return InteractionResumeResult{}, err
	}
	return s.repository.PersistInteractionResumeTx(ctx, tx, r, CreateNextTurnRequest{
		TaskID: r.TaskID, TenantID: r.TenantID, CreatorUserID: r.CreatorUserID,
		SourceCheckpointID: r.CheckpointID, ExpectedCurrentTurnID: r.SourceTaskTurnID,
		Instruction: source.Instruction, IdempotencyKey: fmt.Sprintf("interaction:%d:answer:%d", r.InteractionID, r.AnswerID),
	})
}
