package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

func deliveryHarnessDigestTx(ctx context.Context, tx pgx.Tx, requestID int64) (string, error) {
	var assignmentID int64
	var attemptSelectionDigest string
	var attemptSnapshot []byte
	if err := tx.QueryRow(ctx, `SELECT attempt.assignment_id,attempt.assignment_selection_digest,attempt.resolved_execution_snapshot_json
		FROM workforce_delivery_requests request
		JOIN workforce_continuation_checkpoints checkpoint ON checkpoint.id=request.checkpoint_id AND checkpoint.task_id=request.task_id AND checkpoint.task_turn_id=request.task_turn_id
		JOIN workforce_task_attempts attempt ON attempt.id=checkpoint.attempt_id AND attempt.task_id=request.task_id AND attempt.task_turn_id=request.task_turn_id
		WHERE request.id=$1`, requestID).Scan(&assignmentID, &attemptSelectionDigest, &attemptSnapshot); err != nil {
		return "", err
	}
	var assignmentSelectionDigest string
	var assignmentSnapshot []byte
	if err := tx.QueryRow(ctx, `SELECT selection_digest,resolved_execution_snapshot_json FROM workforce_task_assignments WHERE id=$1`, assignmentID).Scan(&assignmentSelectionDigest, &assignmentSnapshot); err != nil {
		return "", err
	}
	if attemptSelectionDigest != assignmentSelectionDigest || !bytes.Equal(attemptSnapshot, assignmentSnapshot) {
		return "", domain.ErrIdentityConflict
	}
	var identity struct {
		HarnessImageDigest string `json:"harnessImageDigest"`
	}
	if json.Unmarshal(attemptSnapshot, &identity) != nil || strings.TrimSpace(identity.HarnessImageDigest) == "" {
		return "", domain.ErrIdentityConflict
	}
	return identity.HarnessImageDigest, nil
}
