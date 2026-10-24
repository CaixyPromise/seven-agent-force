package application

import (
	"context"
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

type fairnessRepository struct {
	tenants  []int64
	assigned map[int64]bool
	calls    [][]int64
}

func (r *fairnessRepository) DispatchOne(_ context.Context, excluded []int64) (int64, bool, error) {
	r.calls = append(r.calls, append([]int64(nil), excluded...))
	for _, tenant := range r.tenants {
		found := false
		for _, value := range excluded {
			found = found || value == tenant
		}
		if !found {
			return tenant, r.assigned[tenant], nil
		}
	}
	return 0, false, nil
}
func (*fairnessRepository) ReconcileExpired(context.Context, int) (int, error) { return 0, nil }
func (*fairnessRepository) Claim(context.Context, domain.MachineEnvelope, int64, string) (domain.ExecutionContract, error) {
	return domain.ExecutionContract{}, nil
}
func (*fairnessRepository) ReleaseAssignment(context.Context, domain.MachineEnvelope, int64, string) error {
	return nil
}
func (*fairnessRepository) AcquireLaunchPermit(context.Context, domain.MachineEnvelope, int64, string) (domain.LaunchPermit, bool, error) {
	return domain.LaunchPermit{}, false, nil
}
func (*fairnessRepository) RecordContainerCreated(context.Context, domain.MachineEnvelope, int64, string, string, string, string) error {
	return nil
}
func (*fairnessRepository) RecordProviderProxyReady(context.Context, domain.MachineEnvelope, int64, string, domain.ProviderProxyReady) error {
	return nil
}
func (*fairnessRepository) RecordSupervisorReady(context.Context, domain.MachineEnvelope, int64, string, domain.SupervisorReady) error {
	return nil
}
func (*fairnessRepository) AcquireRunPermit(context.Context, domain.MachineEnvelope, int64, string) (domain.RunPermit, error) {
	return domain.RunPermit{}, nil
}
func (*fairnessRepository) RecordHarnessStarted(context.Context, domain.MachineEnvelope, int64, string, domain.HarnessStarted) error {
	return nil
}
func (*fairnessRepository) RenewExecutionLease(context.Context, domain.MachineEnvelope, int64, string, *int64) (string, error) {
	return "", nil
}
func (*fairnessRepository) AcquireReattachPermit(context.Context, domain.MachineEnvelope, int64, domain.RuntimeEvidence) (domain.ReattachPermit, error) {
	return domain.ReattachPermit{}, nil
}
func (*fairnessRepository) CommitReattach(context.Context, domain.MachineEnvelope, int64, string, domain.ReattachAck) (domain.ReattachResult, error) {
	return domain.ReattachResult{}, nil
}
func (*fairnessRepository) ReportRuntimeLoss(context.Context, domain.MachineEnvelope, int64, string, string) (domain.RuntimeLossResult, error) {
	return domain.RuntimeLossResult{}, nil
}
func (*fairnessRepository) ReconcileRuntimeLosses(context.Context, int) (int, error) { return 0, nil }
func (*fairnessRepository) ReconcileTimeouts(context.Context, int) (int, error)      { return 0, nil }

func TestDispatchAvailableVisitsEachTenantOncePerSweep(t *testing.T) {
	repo := &fairnessRepository{tenants: []int64{101, 102, 103}, assigned: map[int64]bool{101: true, 102: false, 103: true}}
	count, err := New(repo).DispatchAvailable(context.Background(), 32)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("assigned count=%d want=2", count)
	}
	want := [][]int64{nil, {101}, {101, 102}, {101, 102, 103}}
	if !reflect.DeepEqual(repo.calls, want) {
		t.Fatalf("fairness exclusions=%v want=%v", repo.calls, want)
	}
}
