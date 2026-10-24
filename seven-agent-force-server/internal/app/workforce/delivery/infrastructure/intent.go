package infrastructure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	deliveryfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
)

type gitDeliveryArguments struct {
	Summary string `json:"summary"`
}

func normalizeIntentArguments(raw json.RawMessage) (gitDeliveryArguments, string, error) {
	var value gitDeliveryArguments
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil {
		return value, "", domain.ErrIntentInvalid
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return value, "", domain.ErrIntentInvalid
	}
	var err error
	value.Summary, err = domain.NormalizeDeliverySummary(value.Summary)
	if err != nil {
		return value, "", err
	}
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return value, hex.EncodeToString(sum[:]), nil
}

func intentReceipt(id, version int64, status, mode, authorization string, replayed bool) deliveryfacade.IntentReceipt {
	return deliveryfacade.IntentReceipt{IntentID: id, Version: version, Status: status, DeliveryMode: mode, DeliveryAuthorization: authorization, Replayed: replayed}
}

func intentToolResult(req deliveryfacade.PublishIntentRequest, receipt deliveryfacade.IntentReceipt) deliveryfacade.ImmediateToolResult {
	body, _ := json.Marshal(map[string]string{"intentId": strings.TrimSpace(int64Text(receipt.IntentID)), "status": deliveryfacade.IntentScheduled, "deliveryMode": receipt.DeliveryMode, "authorization": receipt.DeliveryAuthorization})
	sum := sha256.Sum256(body)
	return deliveryfacade.ImmediateToolResult{RoundKey: req.RoundKey, LocalToolCallKey: req.LocalToolCallKey, NativeToolCallID: req.NativeToolCallID, IntentID: receipt.IntentID, Status: deliveryfacade.IntentScheduled, Summary: "Git delivery intent saved and scheduled after required gates.", ResultDigest: hex.EncodeToString(sum[:])}
}

func int64Text(value int64) string { return strconv.FormatInt(value, 10) }

func (s *Store) PublishIntent(ctx context.Context, machine deliveryfacade.Machine, request deliveryfacade.PublishIntentRequest) (deliveryfacade.IntentReceipt, deliveryfacade.ImmediateToolResult, error) {
	var receipt deliveryfacade.IntentReceipt
	args, digest, err := normalizeIntentArguments(request.Arguments)
	if err != nil || request.ArgumentsDigest != digest || !domain.ValidAdapterKey(request.RoundKey) || !domain.ValidAdapterKey(request.LocalToolCallKey) || !domain.ValidAdapterKey(request.NativeToolCallID) || s == nil || s.db == nil || s.toolAuthority == nil {
		return receipt, deliveryfacade.ImmediateToolResult{}, domain.ErrIntentInvalid
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return receipt, deliveryfacade.ImmediateToolResult{}, err
	}
	defer tx.Rollback(ctx)
	target, err := s.toolAuthority.ValidateDeliveryTool(ctx, tx, machine)
	if err != nil || target.TaskTerminal || target.TaskCancelled {
		return receipt, deliveryfacade.ImmediateToolResult{}, domain.ErrIntentNotAvailable
	}
	var planDigest, mode, authorization string
	err = tx.QueryRow(ctx, `SELECT plan_digest,delivery_mode,delivery_authorization FROM workforce_task_git_plans WHERE tenant_id=$1 AND task_id=$2 AND task_turn_id=$3 AND project_id IS NOT NULL AND delivery_mode IN ('NEW_BRANCH','NEW_BRANCH_AND_DRAFT_REVIEW') AND status='ACTIVE' FOR SHARE`, target.TenantID, target.TaskID, target.TaskTurnID).Scan(&planDigest, &mode, &authorization)
	if err != nil || (authorization != "AUTO_AFTER_GATES" && authorization != "ASK_AFTER_GATES") {
		return receipt, deliveryfacade.ImmediateToolResult{}, domain.ErrIntentNotAvailable
	}
	var id, version, oldAttempt, oldGeneration int64
	var oldRound, oldLocal, oldNative, oldDigest, status string
	err = tx.QueryRow(ctx, `SELECT id,version,attempt_id,adapter_session_generation,round_key,local_tool_call_key,native_tool_call_id,summary_digest,status FROM workforce_delivery_intents WHERE task_turn_id=$1 FOR UPDATE`, target.TaskTurnID).Scan(&id, &version, &oldAttempt, &oldGeneration, &oldRound, &oldLocal, &oldNative, &oldDigest, &status)
	if err == nil {
		if oldAttempt == target.AttemptID && oldGeneration == target.AdapterSessionGeneration && oldLocal == request.LocalToolCallKey && (oldRound != request.RoundKey || oldNative != request.NativeToolCallID || oldDigest != digest) {
			return receipt, deliveryfacade.ImmediateToolResult{}, domain.ErrDecisionConflict
		}
		receipt = intentReceipt(id, version, status, mode, authorization, true)
		if err = tx.Commit(ctx); err != nil {
			return deliveryfacade.IntentReceipt{}, deliveryfacade.ImmediateToolResult{}, err
		}
		return receipt, intentToolResult(request, receipt), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return receipt, deliveryfacade.ImmediateToolResult{}, err
	}
	id = s.idGen.NextID()
	_, err = tx.Exec(ctx, `INSERT INTO workforce_delivery_intents (id,tenant_id,task_id,task_turn_id,attempt_id,adapter_session_generation,local_tool_call_key,native_tool_call_id,round_key,summary,summary_digest,git_plan_digest,delivery_mode,delivery_authorization,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,'WAITING_FINALIZATION')`, id, target.TenantID, target.TaskID, target.TaskTurnID, target.AttemptID, target.AdapterSessionGeneration, request.LocalToolCallKey, request.NativeToolCallID, request.RoundKey, args.Summary, digest, planDigest, mode, authorization)
	if err != nil {
		return receipt, deliveryfacade.ImmediateToolResult{}, err
	}
	receipt = intentReceipt(id, 1, "WAITING_FINALIZATION", mode, authorization, false)
	if err = s.projectDelivery(ctx, tx, target.TenantID, target.TaskTurnID); err != nil {
		return deliveryfacade.IntentReceipt{}, deliveryfacade.ImmediateToolResult{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return deliveryfacade.IntentReceipt{}, deliveryfacade.ImmediateToolResult{}, err
	}
	return receipt, intentToolResult(request, receipt), nil
}

func (s *Store) AckIntent(ctx context.Context, machine deliveryfacade.Machine, request deliveryfacade.AckIntentRequest) (deliveryfacade.IntentReceipt, error) {
	if request.IntentID <= 0 || s == nil || s.toolAuthority == nil {
		return deliveryfacade.IntentReceipt{}, domain.ErrIntentInvalid
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return deliveryfacade.IntentReceipt{}, err
	}
	defer tx.Rollback(ctx)
	target, err := s.toolAuthority.ValidateDeliveryTool(ctx, tx, machine)
	if err != nil {
		return deliveryfacade.IntentReceipt{}, domain.ErrIntentNotAvailable
	}
	var r deliveryfacade.IntentReceipt
	err = tx.QueryRow(ctx, `UPDATE workforce_delivery_intents SET adapter_received_at=COALESCE(adapter_received_at,CURRENT_TIMESTAMP),version=version+CASE WHEN adapter_received_at IS NULL THEN 1 ELSE 0 END,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND tenant_id=$2 AND task_id=$3 AND attempt_id=$4 AND adapter_session_generation=$5 RETURNING id,version,status,delivery_mode,delivery_authorization`, request.IntentID, target.TenantID, target.TaskID, target.AttemptID, target.AdapterSessionGeneration).Scan(&r.IntentID, &r.Version, &r.Status, &r.DeliveryMode, &r.DeliveryAuthorization)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, domain.ErrIntentNotAvailable
	}
	if err != nil {
		return r, err
	}
	err = tx.Commit(ctx)
	return r, err
}

func (s *Store) CloseIntent(ctx context.Context, machine deliveryfacade.Machine, request deliveryfacade.CloseIntentRequest) (deliveryfacade.IntentReceipt, error) {
	if !domain.ValidAdapterKey(request.RoundKey) || !domain.ValidAdapterKey(request.LocalToolCallKey) || s == nil || s.toolAuthority == nil {
		return deliveryfacade.IntentReceipt{}, domain.ErrIntentInvalid
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return deliveryfacade.IntentReceipt{}, err
	}
	defer tx.Rollback(ctx)
	target, err := s.toolAuthority.ValidateDeliveryTool(ctx, tx, machine)
	if err != nil {
		return deliveryfacade.IntentReceipt{}, domain.ErrIntentNotAvailable
	}
	var r deliveryfacade.IntentReceipt
	err = tx.QueryRow(ctx, `UPDATE workforce_delivery_intents SET closed_at=COALESCE(closed_at,CURRENT_TIMESTAMP),updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND task_id=$2 AND attempt_id=$3 AND adapter_session_generation=$4 AND round_key=$5 AND local_tool_call_key=$6 RETURNING id,version,status,delivery_mode,delivery_authorization`, target.TenantID, target.TaskID, target.AttemptID, target.AdapterSessionGeneration, request.RoundKey, request.LocalToolCallKey).Scan(&r.IntentID, &r.Version, &r.Status, &r.DeliveryMode, &r.DeliveryAuthorization)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, domain.ErrIntentNotAvailable
	}
	if err != nil {
		return r, err
	}
	err = tx.Commit(ctx)
	return r, err
}
