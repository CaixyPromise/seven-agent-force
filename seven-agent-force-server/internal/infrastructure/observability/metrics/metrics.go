package metrics

import (
	"context"
	"database/sql"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

type Recorder struct {
	registry   *prometheus.Registry
	datasource *sql.DB

	httpRequests       *prometheus.CounterVec
	httpDuration       *prometheus.HistogramVec
	cacheHits          prometheus.Counter
	cacheMisses        prometheus.Counter
	websocketActive    prometheus.Gauge
	schedulerRuns      *prometheus.CounterVec
	schedulerFailures  *prometheus.CounterVec
	schedulerDurations *prometheus.HistogramVec

	httpTotal       atomic.Int64
	http4xx         atomic.Int64
	http5xx         atomic.Int64
	schedulerTotal  atomic.Int64
	schedulerFailed atomic.Int64
	mu              sync.Mutex
	recentLatencies []float64
}

type Snapshot struct {
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

func NewRecorder(datasource *sql.DB, cacheMgr cacheinfra.Manager) *Recorder {
	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	recorder := &Recorder{
		registry:   registry,
		datasource: datasource,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_server_requests_total",
			Help: "Total HTTP requests.",
		}, []string{"method", "path", "status"}),
		httpDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_server_request_duration_ms",
			Help:    "HTTP request duration in milliseconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path"}),
		cacheHits: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total cache hits.",
		}),
		cacheMisses: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total cache misses.",
		}),
		websocketActive: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "websocket_connections_active",
			Help: "Current active websocket client connections.",
		}),
		schedulerRuns: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scheduler_job_runs_total",
			Help: "Total scheduler job runs.",
		}, []string{"job"}),
		schedulerFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "scheduler_job_failures_total",
			Help: "Total scheduler job failures.",
		}, []string{"job"}),
		schedulerDurations: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "scheduler_job_duration_ms",
			Help:    "Scheduler job duration in milliseconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"job"}),
	}
	registry.MustRegister(
		recorder.httpRequests,
		recorder.httpDuration,
		recorder.cacheHits,
		recorder.cacheMisses,
		recorder.websocketActive,
		recorder.schedulerRuns,
		recorder.schedulerFailures,
		recorder.schedulerDurations,
	)
	if datasource != nil {
		registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "datasource_ping_up",
			Help: "Datasource connectivity status.",
		}, func() float64 {
			if datasource.PingContext(context.Background()) == nil {
				return 1
			}
			return 0
		}))
	}
	registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "websocket_connections_active_runtime",
		Help: "Current runtime websocket connections.",
	}, func() float64 {
		return float64(websocket.ActiveConnections())
	}))
	_ = cacheMgr
	return recorder
}

func (r *Recorder) Registry() *prometheus.Registry {
	return r.registry
}

func (r *Recorder) RecordHTTPRequest(method, path string, status int, duration time.Duration) {
	r.httpRequests.WithLabelValues(method, path, prometheusLabelInt(status)).Inc()
	r.httpDuration.WithLabelValues(method, path).Observe(float64(duration.Milliseconds()))
	r.httpTotal.Add(1)
	if status >= 400 && status < 500 {
		r.http4xx.Add(1)
	}
	if status >= 500 {
		r.http5xx.Add(1)
	}
	r.mu.Lock()
	r.recentLatencies = append(r.recentLatencies, float64(duration.Milliseconds()))
	if len(r.recentLatencies) > 2048 {
		r.recentLatencies = append([]float64(nil), r.recentLatencies[len(r.recentLatencies)-2048:]...)
	}
	r.mu.Unlock()
}

func (r *Recorder) RecordCacheHit() {
	r.cacheHits.Inc()
}

func (r *Recorder) RecordCacheMiss() {
	r.cacheMisses.Inc()
}

func (r *Recorder) RecordSchedulerRun(name string, duration time.Duration, err error) {
	r.schedulerRuns.WithLabelValues(name).Inc()
	r.schedulerDurations.WithLabelValues(name).Observe(float64(duration.Milliseconds()))
	r.schedulerTotal.Add(1)
	if err != nil {
		r.schedulerFailures.WithLabelValues(name).Inc()
		r.schedulerFailed.Add(1)
	}
}

func (r *Recorder) Snapshot(ctx context.Context) Snapshot {
	if r == nil {
		return Snapshot{CapturedAt: time.Now().UTC()}
	}
	latencies := r.copyLatencies()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	result := Snapshot{
		CapturedAt:         time.Now().UTC(),
		RequestCount:       r.httpTotal.Load(),
		ClientErrorCount:   r.http4xx.Load(),
		ServerErrorCount:   r.http5xx.Load(),
		P50LatencyMs:       percentile(latencies, 0.50),
		P95LatencyMs:       percentile(latencies, 0.95),
		P99LatencyMs:       percentile(latencies, 0.99),
		SchedulerRuns:      r.schedulerTotal.Load(),
		SchedulerFailures:  r.schedulerFailed.Load(),
		HeapUsedBytes:      float64(mem.HeapAlloc),
		HeapCommittedBytes: float64(mem.HeapSys),
		HeapMaxBytes:       float64(mem.Sys),
		GCPauseCount:       int64(mem.NumGC),
		GCPauseTimeMs:      float64(mem.PauseTotalNs) / 1_000_000,
		Goroutines:         int64(runtime.NumGoroutine()),
	}
	if r.datasource != nil {
		result.DatasourceUp = r.datasource.PingContext(ctx) == nil
	}
	return result
}

func (r *Recorder) copyLatencies() []float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]float64, len(r.recentLatencies))
	copy(result, r.recentLatencies)
	return result
}

func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	if p <= 0 {
		return values[0]
	}
	if p >= 1 {
		return values[len(values)-1]
	}
	idx := int(float64(len(values)-1) * p)
	return values[idx]
}

func prometheusLabelInt(value int) string {
	return strconv.Itoa(value)
}
