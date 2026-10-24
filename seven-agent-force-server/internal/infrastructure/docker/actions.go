package docker

import (
	"context"
	"fmt"
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const (
	containerActionStart   = "start"
	containerActionStop    = "stop"
	containerActionRestart = "restart"
	containerActionDelete  = "delete"
	containerActionLogs    = "logs"
	containerActionStats   = "stats"
	containerActionInspect = "inspect"

	composeActionUp       = "up"
	composeActionDown     = "down"
	composeActionRestart  = "restart"
	composeActionLogs     = "logs"
	composeActionPS       = "ps"
	composeActionPreview  = "preview"
	composeActionValidate = "validate"
	composeActionEdit     = "edit"
)

func containerAvailableActions(state string) []string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "running", "restarting", "paused":
		return []string{containerActionStop, containerActionRestart, containerActionLogs, containerActionStats, containerActionInspect}
	case "exited", "created", "dead":
		return []string{containerActionStart, containerActionLogs, containerActionInspect, containerActionDelete}
	default:
		return []string{containerActionLogs, containerActionInspect}
	}
}

func composeAvailableActions(source ComposeProjectSource, status ComposeProjectStatus, containerCount int, hasComposeSpec bool, active bool) []string {
	actions := []string{}
	if source == ComposeProjectSourceManaged && hasComposeSpec {
		actions = append(actions, composeActionPreview, composeActionValidate, composeActionEdit)
	}
	if containerCount > 0 {
		actions = append(actions, composeActionLogs, composeActionPS)
	}
	if active {
		return uniqueActions(actions)
	}
	if source != ComposeProjectSourceManaged || !hasComposeSpec {
		return uniqueActions(actions)
	}
	switch status {
	case ComposeProjectStatusRunning, ComposeProjectStatusDegraded:
		actions = append(actions, composeActionDown, composeActionRestart)
	case ComposeProjectStatusStopped, ComposeProjectStatusUnknown:
		actions = append(actions, composeActionUp)
	default:
		actions = append(actions, composeActionUp)
	}
	return uniqueActions(actions)
}

func uniqueActions(actions []string) []string {
	seen := make(map[string]struct{}, len(actions))
	result := make([]string, 0, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		if _, ok := seen[action]; ok {
			continue
		}
		seen[action] = struct{}{}
		result = append(result, action)
	}
	return result
}

func hasAction(actions []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	for _, action := range actions {
		if action == requested {
			return true
		}
	}
	return false
}

func containerActionForOperation(operationType string) string {
	switch operationType {
	case OperationTypeContainerStart:
		return containerActionStart
	case OperationTypeContainerStop:
		return containerActionStop
	case OperationTypeContainerRestart:
		return containerActionRestart
	case OperationTypeContainerDelete:
		return containerActionDelete
	case OperationTypeContainerLogs:
		return containerActionLogs
	default:
		return ""
	}
}

func composeActionForOperation(operationType string) string {
	switch operationType {
	case OperationTypeComposeUp:
		return composeActionUp
	case OperationTypeComposeDown:
		return composeActionDown
	case OperationTypeComposeRestart:
		return composeActionRestart
	case OperationTypeComposeLogs:
		return composeActionLogs
	case OperationTypeComposeValidate:
		return composeActionValidate
	default:
		return ""
	}
}

func activeOperationSummary(op *OperationVO) *OperationVO {
	if op == nil {
		return nil
	}
	if op.Status != OperationStatusPending && op.Status != OperationStatusRunning {
		return nil
	}
	copy := *op
	return &copy
}

func actionNotAllowed(targetType, targetID, currentState, requested string, available []string) error {
	message := fmt.Sprintf("Docker %s 当前状态不允许执行 %s", targetType, requested)
	return apperrors.ParamsWithDetails(message, ActionNotAllowedVO{
		TargetType:       targetType,
		TargetID:         targetID,
		CurrentState:     currentState,
		RequestedAction:  requested,
		AvailableActions: available,
		Message:          message,
	})
}

func (s *service) ValidateContainerOperation(ctx context.Context, id, operationType string) error {
	requested := containerActionForOperation(operationType)
	if requested == "" {
		return nil
	}
	_, _, summary, err := s.resolveContainer(ctx, id)
	if err != nil {
		return err
	}
	actions := containerAvailableActions(string(summary.State))
	if !hasAction(actions, requested) {
		return actionNotAllowed("container", stripSHA(summary.ID), string(summary.State), requested, actions)
	}
	return nil
}

func (s *service) ValidateComposeProjectOperation(ctx context.Context, projectID, operationType string) error {
	requested := composeActionForOperation(operationType)
	if requested == "" {
		return nil
	}
	return s.validateComposeProjectAction(ctx, projectID, requested)
}

func (s *service) validateComposeProjectAction(ctx context.Context, projectID, requested string) error {
	row, err := s.requireManagedComposeProject(ctx, projectID)
	if err != nil {
		return err
	}
	containers, err := s.allContainerViews(ctx)
	if err != nil {
		return err
	}
	containers = matchComposeContainers(row.ProjectName, containers)
	status := composeStatus(containers)
	hasSpec := strings.TrimSpace(row.ComposeYaml.String) != "" || strings.TrimSpace(row.ComposeFilePath.String) != ""
	active := false
	if latest, _ := s.latestOperationForCompose(ctx, row.ProjectID, row.ProjectName, ""); latest != nil {
		active = activeOperationSummary(latest) != nil
	}
	actions := composeAvailableActions(ComposeProjectSourceManaged, status, len(containers), hasSpec, active)
	if !hasAction(actions, requested) {
		return actionNotAllowed("compose", row.ProjectID, string(status), requested, actions)
	}
	return nil
}
