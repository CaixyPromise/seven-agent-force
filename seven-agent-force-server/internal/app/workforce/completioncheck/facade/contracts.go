// Package facade owns the stable completion-check integration contracts.
package facade

import (
	"context"
	"errors"
	"fmt"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	"github.com/jackc/pgx/v5"
)

type Input = domain.Input
type Observation = domain.Observation

func Normalize(input Input) (Input, error) { return domain.Normalize(input) }
func Digest(input Input) (string, error)   { return domain.Digest(input) }

type Snapshot struct {
	Input  Input  `json:"input"`
	Digest string `json:"digest"`
}

func Freeze(input *Input) (*Snapshot, error) {
	if input == nil {
		return nil, nil
	}
	normalized, err := Normalize(*input)
	if err != nil {
		return nil, err
	}
	digest, err := Digest(normalized)
	if err != nil {
		return nil, err
	}
	return &Snapshot{Input: normalized, Digest: digest}, nil
}

var ErrInvalid = errors.New("COMPLETION_CHECK_INVALID")
var ErrConflict = errors.New("COMPLETION_CHECK_CONFLICT")
var ErrNotFound = errors.New("COMPLETION_CHECK_NOT_FOUND")
var ErrStale = fmt.Errorf("%w: COMPLETION_CHECK_STALE", ErrConflict)
var ErrActionNotAllowed = fmt.Errorf("%w: COMPLETION_CHECK_ACTION_NOT_ALLOWED", ErrConflict)
var ErrIdempotencyConflict = fmt.Errorf("%w: COMPLETION_CHECK_IDEMPOTENCY_CONFLICT", ErrConflict)

const (
	ContinueWithLimitations = "CONTINUE_WITH_LIMITATIONS"
	AuthorizeRepair         = "AUTHORIZE_REPAIR"
	AddVerification         = "ADD_VERIFICATION"
)

// Evidence is accepted only from a trusted, already fenced Worker transaction.
// It describes optional checks; it never changes mandatory gate outcomes.
type Evidence struct {
	ID                   int64       `json:"id,string"`
	TenantID             int64       `json:"-"`
	TaskID               int64       `json:"taskId,string"`
	TaskTurnID           int64       `json:"taskTurnId,string"`
	AttemptID            int64       `json:"attemptId,string"`
	EvaluationCycleID    int64       `json:"evaluationCycleId,string"`
	Phase                string      `json:"phase"`
	CheckDigest          string      `json:"checkDigest"`
	Status               string      `json:"status"`
	Summary              string      `json:"summary"`
	Observation          Observation `json:"observation"`
	SourceCommit         string      `json:"sourceCommit"`
	SourceIdentityDigest string      `json:"sourceIdentityDigest"`
}

type EvidenceWriter interface {
	RecordEvidence(context.Context, pgx.Tx, Evidence) (Evidence, bool, error)
}

type EvidenceReader interface {
	LoadComparison(context.Context, pgx.Tx, int64, int64, int64, int64, int64, string) (Observation, Observation, error)
}
type LimitationsReader interface {
	AcceptedLimitations(context.Context, pgx.Tx, int64, int64, int64, string, Observation) (bool, error)
}

type MachineAuthority struct {
	WorkerID   int64
	InstanceID string
	HostBootID string
	Generation int64
	OwnerEpoch int64
}
type MachineControl interface {
	RecordMachineEvidence(context.Context, MachineAuthority, int64, string, Evidence) (Evidence, error)
	ReadMachineEvidence(context.Context, MachineAuthority, int64, string, string, int64, string) (*Evidence, error)
}
