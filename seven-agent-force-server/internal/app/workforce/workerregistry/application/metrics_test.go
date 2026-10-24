package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
)

type metricRepository struct {
	Repository
	sample        MetricSample
	recordErr     error
	queryWorkerID *int64
	queryStep     int
	queryFrom     time.Time
	queryTo       time.Time
}

func (r *metricRepository) Heartbeat(context.Context, HeartbeatCommand) error { return nil }
func (r *metricRepository) RecordMetricSample(_ context.Context, sample MetricSample) error {
	r.sample = sample
	return r.recordErr
}
func (r *metricRepository) Get(context.Context, int64) (domain.Worker, []domain.Image, error) {
	return domain.Worker{ID: 41, DesiredState: domain.DesiredEnabled, State: domain.StateOnline}, nil, nil
}
func (r *metricRepository) QueryMetricSeries(_ context.Context, workerID *int64, from, to time.Time, step int) ([]domain.MetricPoint, error) {
	r.queryWorkerID, r.queryStep, r.queryFrom, r.queryTo = workerID, step, from, to
	return []domain.MetricPoint{}, nil
}
func (r *metricRepository) DeleteMetricSamplesBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func validMetricHeartbeat() HeartbeatCommand {
	return HeartbeatCommand{WorkerID: 41, InstanceID: "instance", HostBootID: "boot", Sequence: 1, ActiveAttempts: 1, MaxActiveAttempts: 4, DockerReady: true, Interfaces: json.RawMessage(`[]`), ResourceSnapshot: json.RawMessage(`{"logicalCpuCores":8,"cpuUsagePercent":36.2,"cpuUsageAvailable":true,"load1":1.25,"memoryUsedBytes":800,"memoryTotalBytes":1000,"memoryUsagePercent":80,"workspaceDiskUsedBytes":500,"workspaceDiskTotalBytes":1000,"workspaceDiskUsagePercent":50}`)}
}

func TestHeartbeatRecordsOneSafeMetricSampleAfterAuthoritativeUpdate(t *testing.T) {
	repo := &metricRepository{}
	service := New(repo, nil)
	if _, err := service.Heartbeat(context.Background(), validMetricHeartbeat()); err != nil {
		t.Fatal(err)
	}
	if repo.sample.WorkerID != 41 || repo.sample.LogicalCPUCores != 8 || repo.sample.CPUUsagePercent == nil || *repo.sample.CPUUsagePercent != 36.2 || repo.sample.ActiveAttempts != 1 || !repo.sample.DockerReady {
		t.Fatalf("unexpected sample: %+v", repo.sample)
	}
}

func TestHeartbeatMetricFailureDoesNotRejectHeartbeat(t *testing.T) {
	repo := &metricRepository{recordErr: errors.New("metrics unavailable")}
	service := New(repo, nil)
	var observed error
	service.SetMetricFailureHandler(func(err error) { observed = err })
	if _, err := service.Heartbeat(context.Background(), validMetricHeartbeat()); err != nil {
		t.Fatal(err)
	}
	if observed == nil {
		t.Fatal("metric failure was not reported")
	}
}

func TestHeartbeatStoresUnavailableCPUAsNull(t *testing.T) {
	command := validMetricHeartbeat()
	command.ResourceSnapshot = json.RawMessage(`{"logicalCpuCores":8,"cpuUsagePercent":0,"cpuUsageAvailable":false,"load1":0,"memoryUsedBytes":800,"memoryTotalBytes":1000,"memoryUsagePercent":80,"workspaceDiskUsedBytes":500,"workspaceDiskTotalBytes":1000,"workspaceDiskUsagePercent":50}`)
	repo := &metricRepository{}
	service := New(repo, nil)
	if _, err := service.Heartbeat(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	if repo.sample.CPUUsagePercent != nil {
		t.Fatal("unavailable CPU must be stored as NULL")
	}
}

func TestMetricRangesUseFixedAlignedSteps(t *testing.T) {
	for rangeName, expected := range map[string]struct {
		step     int
		duration time.Duration
	}{
		"1h": {60, time.Hour}, "6h": {120, 6 * time.Hour}, "24h": {300, 24 * time.Hour},
		"7d": {1800, 7 * 24 * time.Hour}, "30d": {7200, 30 * 24 * time.Hour},
	} {
		repo := &metricRepository{}
		service := New(repo, nil)
		if _, err := service.Metrics(context.Background(), nil, rangeName); err != nil {
			t.Fatal(err)
		}
		if repo.queryStep != expected.step {
			t.Fatalf("range %s used %d, want %d", rangeName, repo.queryStep, expected.step)
		}
		if repo.queryFrom.Unix()%int64(expected.step) != 0 || repo.queryTo.Unix()%int64(expected.step) != 0 {
			t.Fatalf("range %s is not aligned to %d-second buckets: %s - %s", rangeName, expected.step, repo.queryFrom, repo.queryTo)
		}
		if repo.queryTo.Sub(repo.queryFrom) != expected.duration {
			t.Fatalf("range %s duration is %s, want %s", rangeName, repo.queryTo.Sub(repo.queryFrom), expected.duration)
		}
	}
	if _, err := New(&metricRepository{}, nil).Metrics(context.Background(), nil, "2d"); !errors.Is(err, ErrMetricRangeInvalid) {
		t.Fatalf("unexpected invalid range result: %v", err)
	}
}
