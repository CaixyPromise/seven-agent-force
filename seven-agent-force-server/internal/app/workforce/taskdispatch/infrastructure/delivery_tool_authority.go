package infrastructure

import (
	"context"

	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	delivery "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/facade"
	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	dispatch "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/facade"
	"github.com/jackc/pgx/v5"
)

// deliveryToolAuthority adapts the established Attempt/Worker fencing rule.
// It deliberately adds no second interpretation of owner, lease, or session.
type deliveryToolAuthority struct{ base dispatch.InteractionAuthority }

func NewDeliveryToolAuthority(base dispatch.InteractionAuthority) delivery.MachineAuthority {
	return &deliveryToolAuthority{base: base}
}

func (a *deliveryToolAuthority) ValidateDeliveryTool(ctx context.Context, tx pgx.Tx, m delivery.Machine) (delivery.Target, error) {
	t, err := a.base.Validate(ctx, tx, interaction.Machine{
		TenantID: m.TenantID, TaskID: m.TaskID, TaskTurnID: m.TaskTurnID,
		WorkerID: m.WorkerID, InstanceID: m.InstanceID, HostBootID: m.HostBootID,
		AttemptID: m.AttemptID, Generation: m.Generation, OwnerEpoch: m.OwnerEpoch,
		AdapterSessionGeneration: m.AdapterSessionGeneration, ExecutionLease: m.ExecutionLease,
	})
	if err != nil {
		return delivery.Target{}, err
	}
	if !containsCapability(t.Capabilities, "tool.request") || !containsCapability(t.Capabilities, "tool.result") {
		return delivery.Target{}, deliverydomain.ErrIntentNotAvailable
	}
	return delivery.Target{
		TenantID: t.TenantID, TaskID: t.TaskID, TaskTurnID: t.TaskTurnID,
		AttemptID: t.AttemptID, AdapterSessionGeneration: t.AdapterSessionGeneration,
		WorkerID: m.WorkerID, OwnerEpoch: m.OwnerEpoch,
		Capabilities: append([]string(nil), t.Capabilities...),
	}, nil
}
