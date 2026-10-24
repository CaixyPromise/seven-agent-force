package scheduler

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	jobregistry "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/job/registry"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"go.uber.org/zap"
)

type testJob struct {
	name  string
	spec  string
	count *atomic.Int64
}

func (j testJob) Name() string { return j.name }
func (j testJob) Spec() string { return j.spec }
func (j testJob) Run(ctx context.Context) error {
	_ = ctx
	j.count.Add(1)
	return nil
}

func TestSchedulerRunsJob(t *testing.T) {
	reg := jobregistry.New()
	service, err := New(config.SchedulerConfig{
		Enabled:  true,
		Timezone: "Asia/Shanghai",
		Lock: config.SchedulerLockConfig{
			Enabled: false,
			TTL:     time.Second,
		},
	}, zap.NewNop(), reg, nil, nil)
	if err != nil {
		t.Fatalf("new scheduler: %v", err)
	}
	var counter atomic.Int64
	if err := service.Register(testJob{name: "demo", spec: "*/1 * * * * *", count: &counter}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = service.Stop(context.Background()) }()
	time.Sleep(2200 * time.Millisecond)
	if counter.Load() == 0 {
		t.Fatal("expected scheduled job to run at least once")
	}
}
