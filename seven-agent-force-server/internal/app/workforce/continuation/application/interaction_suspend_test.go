package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

// Embedding intentionally panics on Begin/Commit/Rollback: caller owns tx.
type interactionTestTx struct{ pgx.Tx }
type interactionResumeRepo struct {
	source    domain.InteractionSuspend
	tx        pgx.Tx
	persisted bool
	next      CreateNextTurnRequest
}

type operationResumeRepo struct{ interactionResumeRepo }

func (r *operationResumeRepo) LoadOperationResumeTx(_ context.Context, tx pgx.Tx, _ OperationResumeRequest) (domain.InteractionSuspend, error) {
	r.tx = tx
	return r.source, nil
}
func (r *operationResumeRepo) PersistOperationResumeTx(_ context.Context, tx pgx.Tx, _ OperationResumeRequest, n CreateNextTurnRequest) (OperationResumeResult, error) {
	if tx != r.tx {
		panic("transaction escaped")
	}
	r.persisted = true
	r.next = n
	return OperationResumeResult{TaskTurnID: 10, SourceCheckpointID: n.SourceCheckpointID}, nil
}

func (r *interactionResumeRepo) LoadInteractionResumeTx(_ context.Context, tx pgx.Tx, _ InteractionResumeRequest) (domain.InteractionSuspend, error) {
	r.tx = tx
	return r.source, nil
}

func TestOperationResumeConsumesCheckpointInDecisionTransaction(t *testing.T) {
	tx := &interactionTestTx{}
	repo := &operationResumeRepo{interactionResumeRepo: interactionResumeRepo{source: domain.InteractionSuspend{TaskID: 2, TurnID: 3, AttemptID: 4, OperationID: 5, CurrentTurnID: 3, CurrentAttemptID: 4, Purpose: domain.PurposeInteractionSuspend, Phase: "SUSPENDED", TaskStatus: domain.InteractionSuspended, TurnStatus: domain.InteractionSuspended, AttemptStatus: domain.InteractionSuspended, RuntimeStatus: domain.InteractionSuspended, FormatVersion: 2, SnapshotDigest: "fixed", CheckpointSnapshotDigest: "fixed", SessionDigest: "lineage", Instruction: "Preserve the original development goal.", AuthoritySealed: true, Source: domain.Source{CheckpointStatus: "AVAILABLE", CheckpointExpires: time.Now().Add(time.Hour)}}}}
	request := OperationResumeRequest{TenantID: 1, TaskID: 2, SourceTaskTurnID: 3, SourceAttemptID: 4, OperationID: 5, CheckpointID: 6, DecisionID: 7, CreatorUserID: 8, Action: "ALLOW_ONCE"}
	result, err := NewOperationResume(repo).ResumeOperationTx(context.Background(), tx, request)
	if err != nil || result.TaskTurnID != 10 || !repo.persisted || repo.next.IdempotencyKey != "operation:5:decision:7" || repo.next.Instruction != repo.source.Instruction {
		t.Fatalf("result=%+v err=%v next=%+v", result, err, repo.next)
	}
	repo.persisted = false
	repo.source.Cancelled = true
	if _, err = NewOperationResume(repo).ResumeOperationTx(context.Background(), tx, request); err != domain.ErrInteractionSuspend || repo.persisted {
		t.Fatalf("cancelled operation resumed: %v", err)
	}
}
func (r *interactionResumeRepo) PersistInteractionResumeTx(_ context.Context, tx pgx.Tx, _ InteractionResumeRequest, n CreateNextTurnRequest) (InteractionResumeResult, error) {
	if tx != r.tx {
		panic("transaction escaped")
	}
	r.persisted = true
	r.next = n
	return InteractionResumeResult{TaskTurnID: 9, SourceCheckpointID: n.SourceCheckpointID}, nil
}

func TestInteractionResumeUsesCallerTransactionAndRejectsIncompleteCapture(t *testing.T) {
	r := InteractionResumeRequest{TenantID: 1, TaskID: 2, SourceTaskTurnID: 3, SourceAttemptID: 4, InteractionID: 5, CheckpointID: 6, AnswerID: 7, CreatorUserID: 8, Instruction: " saved answer "}
	r.InteractionResponse = []byte(`{"interactionKey":"fake:question","responseId":"7","answers":[]}`)
	repo := &interactionResumeRepo{source: domain.InteractionSuspend{TaskID: 2, TurnID: 3, AttemptID: 4, InteractionID: 5, CurrentTurnID: 3, CurrentAttemptID: 4, Purpose: domain.PurposeInteractionSuspend, Phase: "SUSPENDED", TaskStatus: "RUNNING", TurnStatus: "ACTIVE", FormatVersion: 2, SnapshotDigest: "fixed", CheckpointSnapshotDigest: "fixed", SessionDigest: "lineage", Source: domain.Source{CheckpointStatus: "AVAILABLE", CheckpointExpires: time.Now().Add(time.Hour)}}}
	repo.source.TaskStatus = domain.InteractionSuspended
	repo.source.TurnStatus = domain.InteractionSuspended
	repo.source.AttemptStatus = domain.InteractionSuspended
	repo.source.RuntimeStatus = domain.InteractionSuspended
	repo.source.AuthoritySealed = true
	repo.source.Instruction = "Preserve the original development goal."
	tx := &interactionTestTx{}
	result, err := NewInteractionResume(repo).ResumeInteractionTx(context.Background(), tx, r)
	if err != nil || result.TaskTurnID != 9 || !repo.persisted || repo.next.Instruction != repo.source.Instruction || repo.next.IdempotencyKey != "interaction:5:answer:7" {
		t.Fatalf("result=%+v err=%v next=%+v", result, err, repo.next)
	}
	repo.persisted = false
	repo.source.Phase = "CAPTURING"
	if _, err = NewInteractionResume(repo).ResumeInteractionTx(context.Background(), tx, r); err != domain.ErrInteractionSuspend || repo.persisted {
		t.Fatalf("capture accepted %v", err)
	}
	if _, err = NewInteractionResume(repo).ResumeInteractionTx(context.Background(), nil, r); err != domain.ErrInteractionSuspend {
		t.Fatalf("nil tx accepted %v", err)
	}
}
