package application

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"strings"
)

type Summary struct {
	ReasonCode            string           `json:"reasonCode,omitempty"`
	Input                 *facade.Input    `json:"input"`
	Digest                string           `json:"digest"`
	Baseline              *facade.Evidence `json:"baseline"`
	CurrentResult         *facade.Evidence `json:"currentResult"`
	AvailableActions      []string         `json:"availableActions"`
	CheckpointReady       bool             `json:"checkpointReady"`
	Version               int64            `json:"version,string"`
	SourceCheckpointID    int64            `json:"sourceCheckpointId,string"`
	ExpectedCurrentTurnID int64            `json:"expectedCurrentTurnId,string"`
}
type Decision struct {
	TenantID        int64         `json:"-"`
	TaskID          int64         `json:"-"`
	ActorID         int64         `json:"-"`
	Action          string        `json:"action"`
	ResultID        int64         `json:"resultId,string"`
	ExpectedVersion int64         `json:"expectedVersion,string"`
	Reason          string        `json:"reason"`
	RepairScope     string        `json:"repairScope,omitempty"`
	IdempotencyKey  string        `json:"idempotencyKey"`
	CompletionCheck *facade.Input `json:"completionCheck,omitempty"`
}
type DecisionResult struct {
	DecisionID int64 `json:"decisionId,string"`
	TaskTurnID int64 `json:"taskTurnId,string"`
	Replayed   bool  `json:"replayed"`
}
type Repository interface {
	Summary(context.Context, int64, int64) (Summary, error)
	Decide(context.Context, Decision) (DecisionResult, error)
}
type Service struct{ repository Repository }

func New(repository Repository) *Service { return &Service{repository: repository} }
func (s *Service) Summary(ctx context.Context, tenant, task int64) (Summary, error) {
	if tenant <= 0 || task <= 0 {
		return Summary{}, facade.ErrInvalid
	}
	return s.repository.Summary(ctx, tenant, task)
}
func NormalizeDecision(d Decision) (Decision, error) {
	d.Reason = strings.TrimSpace(d.Reason)
	d.RepairScope = strings.TrimSpace(d.RepairScope)
	d.IdempotencyKey = strings.TrimSpace(d.IdempotencyKey)
	if d.TenantID <= 0 || d.TaskID <= 0 || d.ActorID <= 0 || d.ResultID <= 0 || d.ExpectedVersion <= 0 || d.Reason == "" || len(d.Reason) > 2000 || len(d.RepairScope) > 20000 || d.IdempotencyKey == "" || len(d.IdempotencyKey) > 128 {
		return Decision{}, facade.ErrInvalid
	}
	switch d.Action {
	case facade.ContinueWithLimitations, facade.AuthorizeRepair:
		if d.CompletionCheck != nil {
			return Decision{}, facade.ErrInvalid
		}
		if (d.Action == facade.AuthorizeRepair) != (d.RepairScope != "") {
			return Decision{}, facade.ErrInvalid
		}
	case facade.AddVerification:
		snapshot, err := facade.Freeze(d.CompletionCheck)
		if err != nil || snapshot == nil || snapshot.Input.Mode == "NONE" || d.RepairScope != "" {
			return Decision{}, facade.ErrInvalid
		}
		d.CompletionCheck = &snapshot.Input
	default:
		return Decision{}, facade.ErrInvalid
	}
	return d, nil
}
func (s *Service) Decide(ctx context.Context, d Decision) (DecisionResult, error) {
	normalized, err := NormalizeDecision(d)
	if err != nil {
		return DecisionResult{}, err
	}
	return s.repository.Decide(ctx, normalized)
}
