package domain

import (
	"context"
	"time"
)

type Query struct {
	PlatformKey string
	RangeKey    string
	StartTime   time.Time
	EndTime     time.Time
	BucketSize  time.Duration
}

type RuntimeSnapshot struct {
	CapturedAt         time.Time `json:"capturedAt"`
	RequestCount       int64     `json:"requestCount"`
	ClientErrorCount   int64     `json:"clientErrorCount"`
	ServerErrorCount   int64     `json:"serverErrorCount"`
	P50LatencyMs       float64   `json:"p50LatencyMs"`
	P95LatencyMs       float64   `json:"p95LatencyMs"`
	P99LatencyMs       float64   `json:"p99LatencyMs"`
	SchedulerRuns      int64     `json:"schedulerRuns"`
	SchedulerFailures  int64     `json:"schedulerFailures"`
	DatasourceUp       bool      `json:"datasourceUp"`
	HeapUsedBytes      float64   `json:"heapUsedBytes"`
	HeapCommittedBytes float64   `json:"heapCommittedBytes"`
	HeapMaxBytes       float64   `json:"heapMaxBytes"`
	GCPauseCount       int64     `json:"gcPauseCount"`
	GCPauseTimeMs      float64   `json:"gcPauseTimeMs"`
	Goroutines         int64     `json:"goroutines"`
}

type SnapshotStore interface {
	Append(ctx context.Context, platformKey string, snapshot RuntimeSnapshot) error
	ListBetween(ctx context.Context, platformKey string, startTime, endTime time.Time) ([]RuntimeSnapshot, error)
	Latest(ctx context.Context, platformKey string) (*RuntimeSnapshot, error)
}

type RuntimeSnapshotProvider interface {
	Snapshot(ctx context.Context) RuntimeSnapshot
}

type DependencyHealthProvider interface {
	CacheHealth(ctx context.Context) CacheHealth
}

type CacheHealth struct {
	RedisEnabled    bool
	RedisMode       string
	RedisConfigured bool
	RedisPing       bool
}
