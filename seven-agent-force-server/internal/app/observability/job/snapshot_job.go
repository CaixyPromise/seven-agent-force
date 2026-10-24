package job

import (
	"context"
	"fmt"
	"time"
)

type SnapshotService interface {
	RefreshSnapshots(ctx context.Context) error
}

type SnapshotJob struct {
	service SnapshotService
	spec    string
}

func NewSnapshotJob(service SnapshotService, intervalMs int64) *SnapshotJob {
	if intervalMs <= 0 {
		intervalMs = 300000
	}
	seconds := int(intervalMs / int64(time.Second/time.Millisecond))
	if seconds <= 0 {
		seconds = 300
	}
	return &SnapshotJob{
		service: service,
		spec:    fmt.Sprintf("@every %ds", seconds),
	}
}

func (j *SnapshotJob) Name() string {
	return "observability_metrics_snapshot"
}

func (j *SnapshotJob) Spec() string {
	return j.spec
}

func (j *SnapshotJob) Run(ctx context.Context) error {
	if j == nil || j.service == nil {
		return nil
	}
	return j.service.RefreshSnapshots(ctx)
}
