package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

const (
	OperationTypeContainerStart   = "CONTAINER_START"
	OperationTypeContainerStop    = "CONTAINER_STOP"
	OperationTypeContainerRestart = "CONTAINER_RESTART"
	OperationTypeContainerDelete  = "CONTAINER_DELETE"
	OperationTypeContainerCreate  = "CONTAINER_CREATE"
	OperationTypeContainerLogs    = "CONTAINER_LOGS_FOLLOW"
	OperationTypeImagePull        = "IMAGE_PULL"
	OperationTypeImageRemotePull  = "IMAGE_REMOTE_PULL"
	OperationTypeImageTag         = "IMAGE_TAG"
	OperationTypeImagePush        = "IMAGE_PUSH"
	OperationTypeImageDelete      = "IMAGE_DELETE"
	OperationTypeImageExport      = "IMAGE_EXPORT"
	OperationTypeComposeValidate  = "COMPOSE_VALIDATE"
	OperationTypeComposeUp        = "COMPOSE_UP"
	OperationTypeComposeDown      = "COMPOSE_DOWN"
	OperationTypeComposeRestart   = "COMPOSE_RESTART"
	OperationTypeComposeLogs      = "COMPOSE_LOGS"
	OperationTypeComposeExport    = "COMPOSE_EXPORT"
	OperationTypeImageCleanup     = "IMAGE_CLEANUP"
	OperationTypeContainerCleanup = "CONTAINER_CLEANUP"
	OperationTypeNetworkPrune     = "NETWORK_PRUNE"
	OperationTypeVolumePrune      = "VOLUME_PRUNE"
	OperationTypeRegistrySync     = "REGISTRY_SYNC"
	OperationTypeDaemonRestart    = "DAEMON_RESTART"
)

func (s *service) SubmitOperation(ctx context.Context, command OperationSubmitCommand) (*OperationAcceptedVO, error) {
	if s == nil || s.operations == nil {
		return nil, apperrors.Operation("Docker operation runtime 未配置 datasource")
	}
	if strings.TrimSpace(command.OperationType) == "" {
		return nil, apperrors.Params("operationType 不能为空")
	}
	decision := evaluatePolicy(s.cfg.Security, command.Actor, command.OperationType, command.Payload)
	payloadBytes, err := json.Marshal(command.Payload)
	if err != nil {
		return nil, apperrors.Params("Docker operation payload 无法序列化")
	}
	preview := sanitizePayloadJSON(payloadBytes, s.cfg.Security)
	secret, err := s.secret.EncryptBytes(ctx, payloadBytes)
	if err != nil {
		return nil, apperrors.Operation("加密 Docker operation payload 失败：" + err.Error())
	}
	timeout := command.Timeout
	if timeout <= 0 {
		timeout = s.cfg.Operation.DefaultTimeout
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	id := s.idGen.NextID()
	row := OperationRecord{
		ID:                       id,
		OperationType:            strings.TrimSpace(command.OperationType),
		TargetType:               strings.TrimSpace(command.TargetType),
		TargetID:                 nullString(command.TargetID),
		TargetName:               nullString(command.TargetName),
		Status:                   string(OperationStatusPending),
		ProgressPercent:          0,
		CurrentStage:             nullString("等待执行"),
		RequestPayloadPreview:    nullString(string(preview)),
		RequestPayloadCiphertext: nullString(secret.CiphertextB64),
		RequestPayloadEDEK:       nullString(secret.EDEKB64),
		RequestPayloadWrapKeyRef: nullString(secret.WrapKeyRef),
		ActorUserID:              nullInt64(command.Actor.UserID),
		ActorUsername:            nullString(command.Actor.Username),
		RetryOf:                  nullInt64(command.RetryOf),
		TimeoutAt:                nullTimeValue(time.Now().UTC().Add(timeout)),
	}
	if err := s.operations.InsertOperation(ctx, row); err != nil {
		return nil, err
	}
	_ = s.appendOperationEvent(ctx, id, OperationEventState, "等待执行", 0, "operation pending", nil)
	for _, warning := range decision.Warnings {
		_ = s.appendOperationEvent(ctx, id, OperationEventPolicy, "policy", 0, warning.Message, warning)
	}
	if !decision.safe() {
		for _, violation := range decision.Violations {
			_ = s.appendOperationEvent(ctx, id, OperationEventPolicy, "policy", 0, violation.Message, violation)
		}
		_ = s.operations.Finish(ctx, id, OperationStatusFailed, 0, "policy", "", policyErrorMessage(decision.Violations))
		return &OperationAcceptedVO{OperationID: id, OperationType: command.OperationType, TargetType: command.TargetType, TargetID: command.TargetID, TargetName: command.TargetName, Status: OperationStatusFailed}, nil
	}
	if !s.enqueueOperation(context.Background(), id) {
		message := "Docker operation queue is full"
		_ = s.operations.Finish(ctx, id, OperationStatusFailed, 0, "queue_full", "", message)
		_ = s.appendOperationEvent(ctx, id, OperationEventError, "queue_full", 0, message, nil)
		return &OperationAcceptedVO{OperationID: id, OperationType: command.OperationType, TargetType: command.TargetType, TargetID: command.TargetID, TargetName: command.TargetName, Status: OperationStatusFailed}, nil
	}
	return &OperationAcceptedVO{OperationID: id, OperationType: command.OperationType, TargetType: command.TargetType, TargetID: command.TargetID, TargetName: command.TargetName, Status: OperationStatusPending}, nil
}

func (s *service) enqueueOperation(ctx context.Context, operationID int64) bool {
	if s == nil {
		return false
	}
	if s.queueSem == nil {
		go s.runOperation(ctx, operationID)
		return true
	}
	select {
	case s.queueSem <- struct{}{}:
		go func() {
			defer func() { <-s.queueSem }()
			s.runOperation(ctx, operationID)
		}()
		return true
	default:
		return false
	}
}

func (s *service) runOperation(parent context.Context, operationID int64) {
	if s == nil || s.operations == nil {
		return
	}
	row, err := s.operations.GetOperation(parent, operationID)
	if err != nil || row == nil {
		return
	}
	if row.CancelRequested {
		_ = s.operations.Finish(context.Background(), operationID, OperationStatusCancelled, 0, "cancelled", "", "operation cancelled before start")
		_ = s.appendOperationEvent(context.Background(), operationID, OperationEventState, "cancelled", 0, "operation cancelled before start", nil)
		return
	}
	now := time.Now().UTC()
	if row.TimeoutAt.Valid && !row.TimeoutAt.Time.IsZero() && !now.Before(row.TimeoutAt.Time) {
		_ = s.operations.Finish(context.Background(), operationID, OperationStatusTimeout, 0, "timeout", "", "operation timeout before start")
		_ = s.appendOperationEvent(context.Background(), operationID, OperationEventError, "timeout", 0, "operation timeout before start", nil)
		return
	}
	timeout := s.cfg.Operation.DefaultTimeout
	if row.TimeoutAt.Valid && !row.TimeoutAt.Time.IsZero() {
		timeout = time.Until(row.TimeoutAt.Time)
	}
	if timeout <= 0 {
		if s.cfg.Operation.DefaultTimeout > 0 {
			timeout = s.cfg.Operation.DefaultTimeout
		} else {
			timeout = 10 * time.Minute
		}
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	s.cancelMu.Lock()
	s.cancels[operationID] = cancel
	s.cancelMu.Unlock()
	defer func() {
		cancel()
		s.cancelMu.Lock()
		delete(s.cancels, operationID)
		s.cancelMu.Unlock()
	}()

	select {
	case s.workerSem <- struct{}{}:
		defer func() { <-s.workerSem }()
	case <-ctx.Done():
		s.finishInterruptedOperation(operationID, ctx.Err(), "before worker acquired")
		return
	}

	row, err = s.operations.GetOperation(context.Background(), operationID)
	if err != nil || row == nil {
		return
	}
	if row.CancelRequested {
		_ = s.operations.Finish(context.Background(), operationID, OperationStatusCancelled, 0, "cancelled", "", "operation cancelled before start")
		_ = s.appendOperationEvent(context.Background(), operationID, OperationEventState, "cancelled", 0, "operation cancelled before start", nil)
		return
	}
	if row.TimeoutAt.Valid && !row.TimeoutAt.Time.IsZero() && !time.Now().UTC().Before(row.TimeoutAt.Time) {
		_ = s.operations.Finish(context.Background(), operationID, OperationStatusTimeout, 0, "timeout", "", "operation timeout before start")
		_ = s.appendOperationEvent(context.Background(), operationID, OperationEventError, "timeout", 0, "operation timeout before start", nil)
		return
	}
	if err := s.operations.MarkRunning(ctx, operationID, "执行中", 1); err != nil {
		if err == sql.ErrNoRows {
			latest, latestErr := s.operations.GetOperation(context.Background(), operationID)
			if latestErr == nil && latest != nil && latest.CancelRequested {
				_ = s.operations.Finish(context.Background(), operationID, OperationStatusCancelled, 0, "cancelled", "", "operation cancelled before start")
				_ = s.appendOperationEvent(context.Background(), operationID, OperationEventState, "cancelled", 0, "operation cancelled before start", nil)
			} else if latestErr == nil && latest != nil && latest.TimeoutAt.Valid && !time.Now().UTC().Before(latest.TimeoutAt.Time) {
				_ = s.operations.Finish(context.Background(), operationID, OperationStatusTimeout, 0, "timeout", "", "operation timeout before start")
				_ = s.appendOperationEvent(context.Background(), operationID, OperationEventError, "timeout", 0, "operation timeout before start", nil)
			}
		}
		return
	}
	_ = s.appendOperationEvent(ctx, operationID, OperationEventState, "running", 1, "operation running", nil)
	payload, err := s.decryptOperationPayload(ctx, row)
	if err != nil {
		s.failOperation(context.Background(), operationID, err)
		return
	}
	result, err := s.executeOperation(ctx, row.OperationType, payload, operationID)
	if ctx.Err() == context.Canceled {
		_ = s.operations.Finish(context.Background(), operationID, OperationStatusCancelled, 0, "cancelled", "", "operation cancelled")
		_ = s.appendOperationEvent(context.Background(), operationID, OperationEventState, "cancelled", 0, "operation cancelled", nil)
		return
	}
	if ctx.Err() == context.DeadlineExceeded {
		_ = s.operations.Finish(context.Background(), operationID, OperationStatusTimeout, 0, "timeout", "", "operation timeout")
		_ = s.appendOperationEvent(context.Background(), operationID, OperationEventError, "timeout", 0, "operation timeout", nil)
		return
	}
	if err != nil {
		s.failOperation(context.Background(), operationID, err)
		return
	}
	resultJSON := s.marshalSanitizedResult(result)
	_ = s.operations.Finish(context.Background(), operationID, OperationStatusSucceeded, 100, "完成", resultJSON, "")
	_ = s.appendOperationEvent(context.Background(), operationID, OperationEventResult, "完成", 100, "operation succeeded", result)
}

func (s *service) finishInterruptedOperation(operationID int64, err error, suffix string) {
	if err == context.DeadlineExceeded {
		message := strings.TrimSpace("operation timeout " + suffix)
		_ = s.operations.Finish(context.Background(), operationID, OperationStatusTimeout, 0, "timeout", "", message)
		_ = s.appendOperationEvent(context.Background(), operationID, OperationEventError, "timeout", 0, message, nil)
		return
	}
	message := strings.TrimSpace("operation cancelled " + suffix)
	_ = s.operations.Finish(context.Background(), operationID, OperationStatusCancelled, 0, "cancelled", "", message)
	_ = s.appendOperationEvent(context.Background(), operationID, OperationEventState, "cancelled", 0, message, nil)
}

func (s *service) executeOperation(ctx context.Context, operationType string, payload []byte, operationID int64) (any, error) {
	progress := func(stage string, percent int, message string) {
		_ = s.operations.UpdateProgress(ctx, operationID, stage, percent)
		_ = s.appendOperationEvent(ctx, operationID, OperationEventProgress, stage, percent, message, nil)
	}
	progress("准备执行", 5, operationType)
	switch operationType {
	case OperationTypeContainerStart:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.StartContainer(ctx, req.ID)
		return map[string]any{"success": ok}, err
	case OperationTypeContainerStop:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.StopContainer(ctx, req.ID)
		return map[string]any{"success": ok}, err
	case OperationTypeContainerRestart:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.RestartContainer(ctx, req.ID)
		return map[string]any{"success": ok}, err
	case OperationTypeContainerDelete:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.DeleteContainer(ctx, req.ID)
		return map[string]any{"success": ok}, err
	case OperationTypeContainerCreate:
		var req ContainerCreateRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		id, err := s.CreateContainerFromImage(ctx, req)
		return map[string]any{"containerId": id}, err
	case OperationTypeContainerLogs:
		var req struct {
			ID   string `json:"id"`
			Tail int    `json:"tail"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		logs, err := s.GetContainerLogs(ctx, req.ID, req.Tail)
		_ = s.appendOperationEvent(ctx, operationID, OperationEventLog, "logs", 80, s.maskOperationMessage(logs), nil)
		return map[string]any{"logs": logs}, err
	case OperationTypeImagePull:
		var req ImagePullCommand
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.PullImage(ctx, req)
		return map[string]any{"success": ok}, err
	case OperationTypeImageRemotePull:
		var req RemoteImagePullRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.PullRemoteImage(ctx, req)
		return map[string]any{"success": ok}, err
	case OperationTypeImageTag:
		var req ImageTagCommand
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.TagImage(ctx, req)
		return map[string]any{"success": ok}, err
	case OperationTypeImagePush:
		var req ImagePushCommand
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.PushImage(ctx, req)
		return map[string]any{"success": ok}, err
	case OperationTypeImageDelete:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		ok, err := s.DeleteImage(ctx, req.ID)
		return map[string]any{"success": ok}, err
	case OperationTypeImageExport:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return s.ExportImage(ctx, req.ID)
	case OperationTypeComposeValidate:
		var req ComposeUpRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("parse", 15, "compose request parsed")
		progress("policy", 35, "compose policy reported")
		progress("validate", 65, "running docker compose config")
		result, err := s.PreviewCompose(ctx, OperationActor{}, req)
		progress("collect-result", 90, "compose validate result collected")
		return result, err
	case OperationTypeComposeUp:
		var req ComposeUpRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("parse", 15, "compose request parsed")
		progress("policy", 35, "compose policy accepted")
		progress("validate", 55, "validating compose before up")
		progress("execute", 75, "running docker compose up")
		ok, err := s.UpCompose(ctx, req)
		progress("collect-result", 90, "compose up result collected")
		return map[string]any{"success": ok}, err
	case OperationTypeComposeDown:
		var req ComposeUpRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("parse", 15, "compose request parsed")
		progress("policy", 35, "compose policy accepted")
		progress("execute", 75, "running docker compose down")
		ok, err := s.DownCompose(ctx, req)
		progress("collect-result", 90, "compose down result collected")
		return map[string]any{"success": ok}, err
	case OperationTypeComposeRestart:
		var req ComposeUpRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("parse", 15, "compose request parsed")
		progress("policy", 35, "compose policy accepted")
		progress("execute", 75, "running docker compose restart")
		ok, err := s.RestartCompose(ctx, req)
		progress("collect-result", 90, "compose restart result collected")
		return map[string]any{"success": ok}, err
	case OperationTypeComposeLogs:
		var req struct {
			ProjectName     string `json:"projectName,omitempty"`
			ComposeYaml     string `json:"composeYaml"`
			WorkingDir      string `json:"workingDir,omitempty"`
			ComposeFilePath string `json:"composeFilePath,omitempty"`
			Tail            int    `json:"tail"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("parse", 15, "compose logs request parsed")
		progress("policy", 35, "compose policy accepted")
		progress("execute", 75, "running docker compose logs")
		logs, err := s.ComposeLogs(ctx, ComposeUpRequest{ProjectName: req.ProjectName, ComposeYaml: req.ComposeYaml, WorkingDir: req.WorkingDir, ComposeFilePath: req.ComposeFilePath}, req.Tail)
		_ = s.appendOperationEvent(ctx, operationID, OperationEventLog, "logs", 80, s.maskOperationMessage(logs), nil)
		progress("collect-result", 90, "compose logs result collected")
		return map[string]any{"logs": logs}, err
	case OperationTypeComposeExport:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("parse", 15, "compose export request parsed")
		progress("policy", 35, "compose policy accepted")
		progress("execute", 75, "reconstructing compose from inspect")
		result, err := s.ExportCompose(ctx, req.ID)
		progress("collect-result", 90, "compose export result collected")
		return result, err
	case OperationTypeImageCleanup:
		var req CleanupApplyRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return s.applyImageCleanup(ctx, req)
	case OperationTypeContainerCleanup:
		var req CleanupApplyRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return s.applyContainerCleanup(ctx, req)
	case OperationTypeNetworkPrune:
		var req CleanupApplyRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("execute", 75, "running docker network prune")
		return s.ApplyNetworkPrune(ctx, req)
	case OperationTypeVolumePrune:
		var req CleanupApplyRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		progress("execute", 75, "running docker volume prune")
		return s.ApplyVolumePrune(ctx, req)
	case OperationTypeRegistrySync:
		var req struct {
			RegistryID int64 `json:"registryId"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, err
		}
		return s.syncRegistry(ctx, req.RegistryID)
	case OperationTypeDaemonRestart:
		progress("execute", 75, "restarting docker daemon")
		ok, err := s.RestartDaemon(ctx)
		return map[string]any{"success": ok}, err
	default:
		return nil, apperrors.Params("未知 Docker operationType: " + operationType)
	}
}

func (s *service) ListOperations(ctx context.Context, current, size int64, status, operationType string) (*PageResult[OperationVO], error) {
	if s.operations == nil {
		return nil, apperrors.Operation("Docker operation runtime 未配置 datasource")
	}
	page, err := s.operations.ListOperations(ctx, current, size, status, operationType)
	if err != nil {
		return nil, err
	}
	result := &PageResult[OperationVO]{Current: page.Current, Size: page.Size, Total: page.Total, Records: make([]OperationVO, 0, len(page.Records))}
	for _, row := range page.Records {
		result.Records = append(result.Records, operationVO(row))
	}
	return result, nil
}

func (s *service) GetOperation(ctx context.Context, operationID int64) (*OperationVO, error) {
	row, err := s.getOperationRecord(ctx, operationID)
	if err != nil {
		return nil, err
	}
	vo := operationVO(*row)
	return &vo, nil
}

func (s *service) ListOperationEvents(ctx context.Context, operationID, afterSequence int64, limit int) ([]OperationEventVO, error) {
	if _, err := s.getOperationRecord(ctx, operationID); err != nil {
		return nil, err
	}
	rows, err := s.operations.ListEvents(ctx, operationID, afterSequence, limit)
	if err != nil {
		return nil, err
	}
	result := make([]OperationEventVO, 0, len(rows))
	for _, row := range rows {
		result = append(result, operationEventVO(row))
	}
	return result, nil
}

func (s *service) DiagnoseOperationEventOrphans(ctx context.Context, afterEventID int64, limit int, actor OperationActor) ([]OperationEventOrphanDiagnosticVO, error) {
	if s.operations == nil {
		return nil, apperrors.Operation("Docker operation runtime 未配置 datasource")
	}
	if !operationActorHasExplicitPermission(actor, OperationIntegrityDiagnosePermission) {
		return nil, apperrors.PermissionDenied(OperationIntegrityDiagnosePermission)
	}
	rows, err := s.operations.DiagnoseOrphanEvents(ctx, afterEventID, limit)
	if err != nil {
		return nil, err
	}
	result := make([]OperationEventOrphanDiagnosticVO, 0, len(rows))
	for _, row := range rows {
		if !row.DiagnosticID.Valid || !operationEventDiagnosticMetadataMatches(row) {
			return nil, apperrors.Operation("Docker operation orphan 诊断元数据不完整")
		}
		result = append(result, OperationEventOrphanDiagnosticVO{
			DiagnosticID:             row.DiagnosticID.String,
			EventID:                  row.EventID,
			OperationID:              row.OperationID,
			Sequence:                 row.Sequence,
			ExpectedIntegrityVersion: row.IntegrityVersion,
			Scope:                    row.IntegrityScope.String,
			RelationshipType:         row.IntegrityRelationshipType.String,
			Reason:                   row.IntegrityReason.String,
			OccurredAt:               row.OccurredAt,
		})
	}
	return result, nil
}

func (s *service) CleanupOperationEventOrphan(ctx context.Context, request OperationEventOrphanCleanupRequest, actor OperationActor) (OperationEventOrphanCleanupResult, error) {
	if s.operations == nil {
		return "", apperrors.Operation("Docker operation runtime 未配置 datasource")
	}
	if actor.UserID <= 0 || strings.TrimSpace(actor.Username) == "" {
		return "", apperrors.Params("Docker operation orphan cleanup 需要明确操作者")
	}
	if !operationActorHasExplicitPermission(actor, OperationIntegrityCleanupPermission) {
		return "", apperrors.PermissionDenied(OperationIntegrityCleanupPermission)
	}
	return s.operations.CleanupOrphanEvent(ctx, OperationEventOrphanCleanupCommand{
		AuditID:                  request.AuditID,
		DiagnosticID:             strings.TrimSpace(request.DiagnosticID),
		EventID:                  request.EventID,
		OperationID:              request.OperationID,
		Sequence:                 request.Sequence,
		ExpectedIntegrityVersion: request.ExpectedIntegrityVersion,
		ActorUserID:              actor.UserID,
		ActorUsername:            strings.TrimSpace(actor.Username),
		Reason:                   strings.TrimSpace(request.Reason),
	})
}

func operationActorHasExplicitPermission(actor OperationActor, required string) bool {
	for _, permission := range actor.Permissions {
		if strings.TrimSpace(permission) == required {
			return true
		}
	}
	return false
}

func (s *service) StreamOperation(ctx context.Context, operationID, afterSequence int64) (io.ReadCloser, error) {
	if _, err := s.getOperationRecord(ctx, operationID); err != nil {
		return nil, err
	}
	reader, writer := io.Pipe()
	go s.streamOperationLoop(ctx, writer, operationID, afterSequence)
	return reader, nil
}

func (s *service) CancelOperation(ctx context.Context, operationID int64, actor OperationActor) (bool, error) {
	_ = actor
	row, err := s.getOperationRecord(ctx, operationID)
	if err != nil {
		return false, err
	}
	if row == nil {
		return false, apperrors.NotFound("Docker operation 不存在")
	}
	if terminalStatus(OperationStatus(row.Status)) {
		return false, apperrors.ObjectState("Docker operation 已结束，无法取消")
	}
	if err := s.operations.RequestCancel(ctx, operationID); err != nil {
		return false, err
	}
	s.cancelMu.Lock()
	cancel := s.cancels[operationID]
	s.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	_ = s.appendOperationEvent(ctx, operationID, OperationEventState, "cancel", row.ProgressPercent, "cancel requested", nil)
	return true, nil
}

func (s *service) RetryOperation(ctx context.Context, operationID int64, actor OperationActor) (*OperationAcceptedVO, error) {
	row, err := s.getOperationRecord(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, apperrors.NotFound("Docker operation 不存在")
	}
	if !terminalStatus(OperationStatus(row.Status)) {
		return nil, apperrors.ObjectState("只有终态 Docker operation 可以重试")
	}
	payload, err := s.decryptOperationPayload(ctx, row)
	if err != nil {
		return nil, err
	}
	var payloadAny any
	if err := json.Unmarshal(payload, &payloadAny); err != nil {
		return nil, err
	}
	return s.SubmitOperation(ctx, OperationSubmitCommand{
		OperationType: row.OperationType,
		TargetType:    row.TargetType,
		TargetID:      row.TargetID.String,
		TargetName:    row.TargetName.String,
		Payload:       payloadAny,
		Actor:         actor,
		RetryOf:       row.ID,
	})
}

func (s *service) streamOperationLoop(ctx context.Context, writer *io.PipeWriter, operationID, afterSequence int64) {
	defer writer.Close()
	heartbeat := s.cfg.Operation.SSEHeartbeat
	if heartbeat <= 0 {
		heartbeat = 15 * time.Second
	}
	poll := s.cfg.Operation.PollInterval
	if poll <= 0 {
		poll = time.Second
	}
	ticker := time.NewTicker(poll)
	heartbeatTicker := time.NewTicker(heartbeat)
	defer ticker.Stop()
	defer heartbeatTicker.Stop()
	lastSeq := afterSequence
	for {
		events, err := s.ListOperationEvents(ctx, operationID, lastSeq, 200)
		if err == nil {
			for _, event := range events {
				if event.Sequence > lastSeq {
					lastSeq = event.Sequence
				}
				if err := writeDockerSSEEvent(writer, strings.ToLower(string(event.Type)), event); err != nil {
					return
				}
			}
		}
		if op, err := s.GetOperation(ctx, operationID); err == nil && op != nil && terminalStatus(op.Status) {
			_ = writeDockerSSEEvent(writer, "done", op)
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-heartbeatTicker.C:
			if err := writeDockerSSEEvent(writer, "heartbeat", map[string]any{"ts": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
				return
			}
		case <-ticker.C:
		}
	}
}

func (s *service) getOperationRecord(ctx context.Context, operationID int64) (*OperationRecord, error) {
	if s == nil || s.operations == nil {
		return nil, apperrors.Operation("Docker operation runtime 未配置 datasource")
	}
	if operationID <= 0 {
		return nil, apperrors.Params("operationId 不能为空")
	}
	row, err := s.operations.GetOperation(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, apperrors.NotFound("Docker operation 不存在")
	}
	return row, nil
}

func (s *service) decryptOperationPayload(ctx context.Context, row *OperationRecord) ([]byte, error) {
	if row == nil {
		return nil, apperrors.NotFound("Docker operation 不存在")
	}
	if s.secret == nil {
		return nil, apperrors.Operation("Docker operation payload 解密服务未配置")
	}
	return s.secret.DecryptBytes(ctx, secretvalueinfra.SecretValue{
		CiphertextB64: row.RequestPayloadCiphertext.String,
		EDEKB64:       row.RequestPayloadEDEK.String,
		WrapKeyRef:    row.RequestPayloadWrapKeyRef.String,
	})
}

func (s *service) failOperation(ctx context.Context, operationID int64, err error) {
	message := "operation failed"
	if err != nil {
		message = err.Error()
	}
	message = s.maskOperationMessage(message)
	_ = s.operations.Finish(ctx, operationID, OperationStatusFailed, 0, "失败", "", truncate(message, 1000))
	_ = s.appendOperationEvent(ctx, operationID, OperationEventError, "失败", 0, message, nil)
}

func (s *service) appendOperationEvent(ctx context.Context, operationID int64, eventType OperationEventType, stage string, percent int, message string, payload any) error {
	if s == nil || s.operations == nil || s.idGen == nil {
		return nil
	}
	payloadJSON := ""
	if payload != nil {
		if b, err := json.Marshal(payload); err == nil {
			payloadJSON = string(sanitizePayloadJSON(b, s.cfg.Security))
		}
	}
	err := s.operations.AppendEvent(ctx, OperationEventRecord{
		ID:          s.idGen.NextID(),
		OperationID: operationID,
		EventType:   string(eventType),
		Stage:       sql.NullString{String: stage, Valid: strings.TrimSpace(stage) != ""},
		Percent:     sql.NullInt64{Int64: int64(percent), Valid: percent >= 0},
		Message:     sql.NullString{String: truncate(s.maskOperationMessage(message), 2048), Valid: strings.TrimSpace(message) != ""},
		PayloadJSON: sql.NullString{String: payloadJSON, Valid: payloadJSON != ""},
	})
	if err == nil {
		_ = s.operations.TrimEvents(ctx, operationID, s.cfg.Operation.EventRetentionLimit)
	}
	return err
}

func (s *service) marshalSanitizedResult(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"message":%q}`, s.maskOperationMessage(err.Error()))
	}
	return string(sanitizePayloadJSON(data, s.cfg.Security))
}

func (s *service) maskOperationMessage(value string) string {
	return sanitizeTextBySensitiveKeys(value, s.cfg.Security)
}

func operationVO(row OperationRecord) OperationVO {
	return OperationVO{
		OperationID:   row.ID,
		OperationType: row.OperationType,
		TargetType:    row.TargetType,
		TargetID:      row.TargetID.String,
		TargetName:    row.TargetName.String,
		Status:        OperationStatus(row.Status),
		Progress:      row.ProgressPercent,
		CurrentStage:  row.CurrentStage.String,
		StartedAt:     nullableTimePtr(row.StartedAt),
		FinishedAt:    nullableTimePtr(row.FinishedAt),
		TimeoutAt:     nullableTimePtr(row.TimeoutAt),
		Actor:         OperationActor{UserID: row.ActorUserID.Int64, Username: row.ActorUsername.String},
		RetryOf:       row.RetryOf.Int64,
		ErrorSummary:  row.ErrorSummary.String,
		Result:        jsonMap(row.ResultJSON.String),
		CreateTime:    nullableTimePtr(row.CreateTime),
		UpdateTime:    nullableTimePtr(row.UpdateTime),
	}
}

func operationEventVO(row OperationEventRecord) OperationEventVO {
	var percent *int
	if row.Percent.Valid {
		value := int(row.Percent.Int64)
		percent = &value
	}
	return OperationEventVO{
		EventID:     row.ID,
		OperationID: row.OperationID,
		Sequence:    row.Sequence,
		Type:        OperationEventType(row.EventType),
		Stage:       row.Stage.String,
		Percent:     percent,
		Message:     row.Message.String,
		Payload:     jsonMap(row.PayloadJSON.String),
		OccurredAt:  row.OccurredAt,
	}
}

func nullableTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid || value.Time.IsZero() {
		return nil
	}
	copy := value.Time
	return &copy
}

func terminalStatus(status OperationStatus) bool {
	switch status {
	case OperationStatusSucceeded, OperationStatusFailed, OperationStatusCancelled, OperationStatusTimeout:
		return true
	default:
		return false
	}
}

func marshalResult(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"message":%q}`, err.Error())
	}
	return string(data)
}

func sanitizePayloadJSON(payload []byte, cfg config.DockerSecurityConfig) []byte {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return payload
	}
	masked := maskJSONValue(value, cfg)
	data, err := json.Marshal(masked)
	if err != nil {
		return payload
	}
	return data
}

func maskJSONValue(value any, cfg config.DockerSecurityConfig) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		if keyValue, ok := typed["key"].(string); ok && isSensitiveDockerKey(cfg, keyValue) {
			for key, item := range typed {
				if strings.EqualFold(key, "value") {
					result[key] = "******"
				} else if isVisiblePayloadMetadataField(key) {
					result[key] = item
				} else {
					result[key] = maskJSONValue(item, cfg)
				}
			}
			return result
		}
		for key, item := range typed {
			if isVisiblePayloadMetadataField(key) {
				result[key] = item
				continue
			}
			if isSensitiveDockerKey(cfg, key) {
				result[key] = "******"
				continue
			}
			result[key] = maskJSONValue(item, cfg)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			result = append(result, maskJSONValue(item, cfg))
		}
		return result
	case string:
		return sanitizeTextBySensitiveKeys(typed, cfg)
	default:
		return value
	}
}

func isVisiblePayloadMetadataField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "key" || normalized == "configkey" || normalized == "config_key"
}

func writeDockerSSEEvent(writer *io.PipeWriter, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte("event: " + strings.TrimSpace(event) + "\n" + "data: " + string(data) + "\n\n"))
	return err
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
