package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

func TestRefreshServiceCoalescesAndDoesNotDirtyOnRollback(t *testing.T) {
	ports := &refreshTestOutbox{}
	generation := &refreshTestGeneration{}
	service := NewRefreshService(refreshTestTx{}, ports, generation, refreshTestFanout{}, refreshTestGate{})
	if result, err := service.Refresh(context.Background()); err != nil || result.State != "PENDING" || ports.appended != 1 || generation.dirty != 1 {
		t.Fatalf("first refresh result=%+v err=%v appended=%d dirty=%d", result, err, ports.appended, generation.dirty)
	}
	ports.active = &cachepolicy.RefreshOperation{EventID: "existing"}
	if result, err := service.Refresh(context.Background()); err != nil || result.State != "PENDING" || ports.appended != 1 || generation.dirty != 1 {
		t.Fatalf("active operation did not coalesce: result=%+v err=%v appended=%d dirty=%d", result, err, ports.appended, generation.dirty)
	}

	rollback := NewRefreshService(refreshFailingTx{}, &refreshTestOutbox{}, generation, refreshTestFanout{}, refreshTestGate{})
	if _, err := rollback.Refresh(context.Background()); err == nil || generation.dirty != 1 {
		t.Fatalf("rollback refresh dirtied local cache or hid error: err=%v dirty=%d", err, generation.dirty)
	}
}

func TestRefreshServiceHonorsOneMinuteCooldown(t *testing.T) {
	ports := &refreshTestOutbox{latest: &cachepolicy.RefreshOperation{EventID: "done", CompletedAt: time.Now().UTC().Add(-time.Second)}}
	generation := &refreshTestGeneration{}
	service := NewRefreshService(refreshTestTx{}, ports, generation, refreshTestFanout{}, refreshTestGate{})
	if result, err := service.Refresh(context.Background()); err != nil || result.State != "COOLDOWN" || ports.appended != 0 || generation.dirty != 0 {
		t.Fatalf("cooldown result=%+v err=%v appended=%d dirty=%d", result, err, ports.appended, generation.dirty)
	}
}

func TestRefreshServiceDisabledRequestDoesNotCreateAnOutboxEvent(t *testing.T) {
	ports := &refreshTestOutbox{}
	generation := &refreshTestGeneration{}
	service := NewRefreshService(refreshTestTx{}, ports, generation, refreshTestFanout{}, refreshTestGate{})
	service.SetRequestEnabled(false)

	result, err := service.Refresh(context.Background())
	if err != nil || result.State != "DISABLED" {
		t.Fatalf("disabled refresh result=%+v err=%v", result, err)
	}
	if ports.appended != 0 || generation.dirty != 0 {
		t.Fatalf("disabled refresh created side effects: appended=%d dirty=%d", ports.appended, generation.dirty)
	}
}

func TestRefreshServiceDisabledRequestStillHandlesDurableV3Fanout(t *testing.T) {
	generation := &refreshTestGeneration{}
	refresh := NewRefreshService(refreshTestTx{}, &refreshTestOutbox{}, generation, refreshTestFanout{}, refreshTestGate{})
	refresh.SetRequestEnabled(false)
	service := &Service{}
	service.BindRefresh(refresh)
	event, err := cachepolicy.NewCacheRefreshEnvelope("refresh-after-rollout")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.HandleRefreshFanout(context.Background(), event); err != nil {
		t.Fatalf("disabled request gate rejected durable V3 fanout: %v", err)
	}
	if generation.evicted != 1 {
		t.Fatalf("V3 fanout evictions=%d, want 1", generation.evicted)
	}
}

type refreshTestTx struct{}

func (refreshTestTx) Enabled() bool { return true }
func (refreshTestTx) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type refreshFailingTx struct{}

func (refreshFailingTx) Enabled() bool { return true }
func (refreshFailingTx) WithinTransaction(context.Context, func(context.Context) error) error {
	return errors.New("rollback")
}

type refreshTestLease struct{}

func (refreshTestLease) Trusted() bool { return true }
func (refreshTestLease) Release()      {}

type refreshTestGate struct{}

func (refreshTestGate) AcquireRefreshMutation(context.Context) (cachepolicy.FreshnessLease, error) {
	return refreshTestLease{}, nil
}

type refreshTestOutbox struct {
	active, latest *cachepolicy.RefreshOperation
	appended       int
}

func (o *refreshTestOutbox) AppendRefresh(context.Context, cachepolicy.CacheRefreshEnvelope) error {
	o.appended++
	return nil
}
func (o *refreshTestOutbox) ListRefreshReady(context.Context, int) ([]cachepolicy.OutboxEvent, error) {
	return nil, nil
}
func (o *refreshTestOutbox) ListRefreshUnknown(context.Context, int) ([]cachepolicy.OutboxEvent, error) {
	return nil, nil
}
func (o *refreshTestOutbox) FindActiveRefresh(context.Context) (*cachepolicy.RefreshOperation, error) {
	return o.active, nil
}
func (o *refreshTestOutbox) FindLatestCompletedRefresh(context.Context) (*cachepolicy.RefreshOperation, error) {
	return o.latest, nil
}
func (*refreshTestOutbox) Claim(context.Context, int64, string, string) (*cachepolicy.Lease, bool, error) {
	return nil, false, nil
}
func (*refreshTestOutbox) Mark(context.Context, int64, string, string, string, string, int, *time.Time) (bool, error) {
	return true, nil
}

type refreshTestGeneration struct {
	dirty   int
	evicted int
}

func (*refreshTestGeneration) AdvanceGlobalRefresh(context.Context, string) (bool, error) {
	return true, nil
}
func (g *refreshTestGeneration) MarkGlobalRefreshDirty(string)    { g.dirty++ }
func (g *refreshTestGeneration) EvictAllGovernedLocal(string)     { g.evicted++ }
func (*refreshTestGeneration) SetGlobalRefreshFanoutHealthy(bool) {}

type refreshTestFanout struct{}

func (refreshTestFanout) Enabled() bool { return true }
func (refreshTestFanout) PublishRefresh(context.Context, cachepolicy.CacheRefreshEnvelope) error {
	return nil
}
