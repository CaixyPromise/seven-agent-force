package application

import (
	"context"
	"encoding/base64"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/domain"
	"github.com/jackc/pgx/v5"
)

type InteractionCaptureRequest struct {
	MachineAuthority   `json:"-"`
	TenantID           int64  `json:"-"`
	TaskID             int64  `json:"taskId,string"`
	TaskTurnID         int64  `json:"taskTurnId,string"`
	InteractionID      int64  `json:"interactionId,string"`
	OperationID        int64  `json:"operationId,string"`
	CheckpointID       int64  `json:"checkpointId,string"`
	CheckpointIdentity string `json:"checkpointIdentity"`
}

type InteractionCaptureAuthority interface {
	AuthorizeInteractionCaptureTx(context.Context, pgx.Tx, InteractionCaptureRequest) error
}

// This is a distinct CAPTURING-purpose authority path. Ordinary result-cycle
// authorization must never be broadened to authorize interaction capture.
func (s *Service) IssueInteractionCheckpointKeyTx(ctx context.Context, tx pgx.Tx, r InteractionCaptureRequest) (CheckpointKeyMaterialization, error) {
	if s == nil {
		return CheckpointKeyMaterialization{}, domain.ErrInteractionSuspend
	}
	authority, ok := s.repository.(InteractionCaptureAuthority)
	if tx == nil || !ok || s.keys == nil || len(r.CheckpointIdentity) != 64 {
		return CheckpointKeyMaterialization{}, domain.ErrInteractionSuspend
	}
	if err := authority.AuthorizeInteractionCaptureTx(ctx, tx, r); err != nil {
		return CheckpointKeyMaterialization{}, err
	}
	loader, ok := s.repository.(interface {
		LoadInteractionReservationTx(context.Context, pgx.Tx, InteractionCaptureRequest) (InteractionReservation, error)
	})
	if !ok {
		return CheckpointKeyMaterialization{}, domain.ErrInteractionSuspend
	}
	record, err := loader.LoadInteractionReservationTx(ctx, tx, r)
	if err != nil {
		return CheckpointKeyMaterialization{}, err
	}
	key, err := s.keys.Unwrap(ctx, record.Envelope)
	if err != nil || len(key) != 32 {
		return CheckpointKeyMaterialization{}, domain.ErrInteractionSuspend
	}
	defer clear(key)
	return CheckpointKeyMaterialization{Algorithm: "AES-256-GCM-CHUNKED", PlainKeyBase64: base64.StdEncoding.EncodeToString(key), Envelope: record.Envelope, AADIdentity: r.CheckpointIdentity}, nil
}
