package application

import (
	"context"
	"fmt"
	"io"
	"math"
	"runtime"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/observability/domain"
	obsfacade "github.com/CaixyPromise/seven-agent/force/internal/app/observability/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type RuntimeLogFacade interface {
	Page(ctx context.Context, request adminfacade.RuntimeLogQueryDTO) (*adminfacade.PageResult[adminfacade.RuntimeLogLineDTO], error)
	Stream(ctx context.Context, request adminfacade.RuntimeLogStreamRequestDTO, userID int64) (io.ReadCloser, error)
}

type DockerMetricsProvider interface {
	MetricsSnapshot(ctx context.Context) (*DockerMetricsSnapshot, error)
}

type DockerMetricsSnapshot struct {
	Enabled               bool
	DaemonHealthy         bool
	RegistryHealthy       bool
	ContainerCountByState map[string]int64
	ImageCount            int64
	ImageSizeBytes        int64
	OperationTotal        int64
	OperationSucceeded    int64
	OperationFailed       int64
	PolicyViolationTotal  int64
}

type Service struct {
	cfg        config.ObservabilityConfig
	cache      domain.SnapshotStore
	depsHealth domain.DependencyHealthProvider
	runtime    domain.RuntimeSnapshotProvider
	audits     ssofacade.AuditEventQueryFacade
	clients    ssofacade.ClientQueryFacade
	sessions   ssofacade.SessionFacade
	runtimeLog RuntimeLogFacade
	docker     DockerMetricsProvider
	startedAt  time.Time
}

func NewService(
	cfg config.ObservabilityConfig,
	cache domain.SnapshotStore,
	depsHealth domain.DependencyHealthProvider,
	runtime domain.RuntimeSnapshotProvider,
	audits ssofacade.AuditEventQueryFacade,
	clients ssofacade.ClientQueryFacade,
	sessions ssofacade.SessionFacade,
	runtimeLog RuntimeLogFacade,
	docker DockerMetricsProvider,
) *Service {
	return &Service{
		cfg:        cfg,
		cache:      cache,
		depsHealth: depsHealth,
		runtime:    runtime,
		audits:     audits,
		clients:    clients,
		sessions:   sessions,
		runtimeLog: runtimeLog,
		docker:     docker,
		startedAt:  time.Now().UTC(),
	}
}

func (s *Service) GetOverview(ctx context.Context, platformKey, rangeKey string) (*obsfacade.OverviewVO, error) {
	query := buildQuery(platformKey, rangeKey)
	platform, err := s.buildSSOPlatform(ctx, query)
	if err != nil {
		return nil, err
	}
	logSummary, logTrends, recentErrors, hotLoggers, recentLogs := s.buildLogPanels(ctx, query)
	overview := &obsfacade.OverviewVO{
		GeneratedAt:         query.EndTime,
		TimeRangeLabel:      rangeLabel(query.RangeKey),
		SelectedPlatformKey: platform.PlatformKey,
		RangeKey:            query.RangeKey,
		HeadlineMetrics:     platform.Metrics,
		Platforms:           []obsfacade.PlatformVO{platform},
		LogSummary:          logSummary,
		LogTrends:           logTrends,
		RecentErrors:        recentErrors,
		HotLoggers:          hotLoggers,
		RecentLogs:          recentLogs,
		LogStreamEnabled:    s.cfg.Logs.Enabled && s.runtimeLog != nil,
	}
	if dockerPanel := s.buildDockerPanel(ctx); dockerPanel != nil {
		overview.Extra = map[string]any{"docker": dockerPanel}
		overview.HeadlineMetrics = append(overview.HeadlineMetrics, obsfacade.MetricVO{
			Key:        "dockerOperations",
			Label:      "Docker 操作",
			Value:      fmt.Sprintf("%v", dockerPanel["operationTotal"]),
			TrendLabel: "累计",
			Tone:       "cyan",
		})
	}
	return overview, nil
}

func (s *Service) PageLogs(ctx context.Context, request adminfacade.RuntimeLogQueryDTO) (*adminfacade.PageResult[adminfacade.RuntimeLogLineDTO], error) {
	if s.runtimeLog == nil {
		return &adminfacade.PageResult[adminfacade.RuntimeLogLineDTO]{Current: request.Current, Size: request.Size, Records: []adminfacade.RuntimeLogLineDTO{}}, nil
	}
	return s.runtimeLog.Page(ctx, request)
}

func (s *Service) StreamLogs(ctx context.Context, request adminfacade.RuntimeLogStreamRequestDTO, userID int64) (io.ReadCloser, error) {
	if s.runtimeLog == nil {
		return nil, apperrors.Operation("运行日志能力未配置")
	}
	return s.runtimeLog.Stream(ctx, request, userID)
}

func (s *Service) RefreshSnapshots(ctx context.Context) error {
	if s == nil || s.cache == nil || s.runtime == nil {
		return nil
	}
	return s.cache.Append(ctx, "sso", s.runtime.Snapshot(ctx))
}

func (s *Service) buildSSOPlatform(ctx context.Context, query domain.Query) (obsfacade.PlatformVO, error) {
	events, err := s.audits.ListEventsSince(ctx, query.StartTime)
	if err != nil {
		return obsfacade.PlatformVO{}, err
	}
	clients, err := s.clients.ListEnabledClients(ctx)
	if err != nil {
		return obsfacade.PlatformVO{}, err
	}
	activeSessions, err := s.sessions.CountActiveSessions(ctx)
	if err != nil {
		return obsfacade.PlatformVO{}, err
	}
	snapshots, _ := s.cache.ListBetween(ctx, "sso", query.StartTime.Add(-query.BucketSize), query.EndTime.Add(query.BucketSize))
	current := s.currentRuntimeSnapshot(ctx)
	loginSuccess := countEvents(events, "login_success")
	loginFailure := countEvents(events, "login_failure")
	tokenIssued := countTokenEvents(events)
	challengeRequired := countEvents(events, "challenge_required")
	riskEvents := countRiskEvents(events)
	failureRate := rate(loginFailure, loginSuccess+loginFailure)
	status := resolvePlatformStatus(failureRate, riskEvents)
	return obsfacade.PlatformVO{
		PlatformKey:   "sso",
		PlatformName:  "统一登录中心",
		Description:   "聚合当前平台的访问链路、风险信号、运行状态和应用日志。",
		Status:        status,
		StatusLabel:   statusLabel(status),
		LastUpdated:   query.EndTime,
		HealthSummary: s.buildHealthSummary(ctx),
		Metrics: []obsfacade.MetricVO{
			metric("activeSessions", "活跃会话", fmt.Sprintf("%d", activeSessions), "当前在线水位", "sky"),
			metric("loginSuccess", "登录成功", fmt.Sprintf("%d", loginSuccess), "当前观察窗口", "emerald"),
			metric("tokenIssued", "令牌签发", fmt.Sprintf("%d", tokenIssued), "含刷新链路", "violet"),
			metric("requestTotal", "请求量", fmt.Sprintf("%d", current.RequestCount), "HTTP 总体请求数", "indigo"),
			metric("failureRate", "失败率", formatPercent(failureRate), "密码登录失败率", "rose"),
			metric("enabledClients", "启用接入", fmt.Sprintf("%d", len(clients)), "当前开放客户端", "slate"),
		},
		TrafficTrends:    buildTrafficTrends(query, snapshots),
		ResourceTrends:   buildResourceTrends(query, snapshots),
		JvmSnapshot:      buildRuntimeSnapshot(current),
		DependencyMatrix: s.buildDependencyMatrix(ctx, current),
		MiddlewarePanels: s.buildMiddlewarePanels(ctx, current),
		Timeline:         buildTimeline(query, events),
		EndpointInsights: []obsfacade.EndpointInsightVO{},
		EventShares:      buildEventShares(events),
		TopClients:       buildTopClients(events, clients),
		Alerts:           buildAlerts(events, clients),
		ExtensionPanels: []obsfacade.ExtensionPanelVO{
			{PanelKey: "ssoSignals", Title: "平台信号", Description: "聚合协议、风险和会话信号。", Status: status, Metrics: []obsfacade.MetricVO{
				metric("challengeRequired", "二次校验", fmt.Sprintf("%d", challengeRequired), "当前窗口触发量", "amber"),
				metric("sessionActive", "会话水位", fmt.Sprintf("%d", activeSessions), "当前浏览器会话", "sky"),
				metric("riskEvents", "风险事件", fmt.Sprintf("%d", riskEvents), "当前窗口", "rose"),
			}},
		},
	}, nil
}

func (s *Service) currentRuntimeSnapshot(ctx context.Context) domain.RuntimeSnapshot {
	if s.cache != nil {
		if latest, err := s.cache.Latest(ctx, "sso"); err == nil && latest != nil {
			return *latest
		}
	}
	if s.runtime != nil {
		return s.runtime.Snapshot(ctx)
	}
	return domain.RuntimeSnapshot{CapturedAt: time.Now().UTC()}
}

func (s *Service) buildHealthSummary(ctx context.Context) obsfacade.HealthSummaryVO {
	status := "UP"
	if s.runtime == nil {
		status = "UNKNOWN"
	}
	return obsfacade.HealthSummaryVO{
		OverallStatus:      status,
		OverallStatusLabel: resolveHealthLabel(status),
		LivenessStatus:     status,
		ReadinessStatus:    status,
		UptimeLabel:        uptimeLabel(s.startedAt, time.Now().UTC()),
	}
}

func (s *Service) buildDependencyMatrix(ctx context.Context, snapshot domain.RuntimeSnapshot) []obsfacade.DependencyHealthVO {
	redisStatus := "UNKNOWN"
	redisDetail := "cache manager unavailable"
	if s.depsHealth != nil {
		health := s.depsHealth.CacheHealth(ctx)
		redisStatus = boolStatus(health.RedisEnabled && health.RedisConfigured && health.RedisPing)
		redisDetail = fmt.Sprintf("mode=%s configured=%t ping=%t", health.RedisMode, health.RedisConfigured, health.RedisPing)
		if !health.RedisEnabled {
			redisStatus = "UNKNOWN"
			redisDetail = "redis cache layer disabled"
		}
	}
	return []obsfacade.DependencyHealthVO{
		{DependencyKey: "mysql", DependencyName: "MySQL", Status: boolStatus(snapshot.DatasourceUp), StatusLabel: resolveHealthLabel(boolStatus(snapshot.DatasourceUp)), Detail: "datasource ping sampled by observability recorder"},
		{DependencyKey: "redis", DependencyName: "Redis", Status: redisStatus, StatusLabel: resolveHealthLabel(redisStatus), Detail: redisDetail},
		{DependencyKey: "storage", DependencyName: "Storage", Status: "UP", StatusLabel: "正常", Detail: "runtime log file readable when configured"},
	}
}

func (s *Service) buildMiddlewarePanels(ctx context.Context, snapshot domain.RuntimeSnapshot) []obsfacade.MiddlewarePanelVO {
	return []obsfacade.MiddlewarePanelVO{
		{PanelKey: "http", Title: "HTTP", Description: "HTTP 请求与错误水位", Status: "healthy", StatusLabel: "正常", Metrics: []obsfacade.MetricVO{
			metric("requestTotal", "请求量", fmt.Sprintf("%d", snapshot.RequestCount), "进程累计", "indigo"),
			metric("4xx", "4xx", fmt.Sprintf("%d", snapshot.ClientErrorCount), "客户端错误", "amber"),
			metric("5xx", "5xx", fmt.Sprintf("%d", snapshot.ServerErrorCount), "服务端错误", "rose"),
		}},
		{PanelKey: "scheduler", Title: "调度器", Description: "后台任务执行水位", Status: "healthy", StatusLabel: "正常", Metrics: []obsfacade.MetricVO{
			metric("runs", "执行次数", fmt.Sprintf("%d", snapshot.SchedulerRuns), "进程累计", "slate"),
			metric("failures", "失败次数", fmt.Sprintf("%d", snapshot.SchedulerFailures), "进程累计", "rose"),
		}},
	}
}

func (s *Service) buildLogPanels(ctx context.Context, query domain.Query) (*obsfacade.LogSummaryVO, []obsfacade.LogTrendPointVO, []obsfacade.RuntimeLogLineDTO, []obsfacade.LoggerMetricVO, []obsfacade.RuntimeLogLineDTO) {
	if !s.cfg.Logs.Enabled || s.runtimeLog == nil {
		return nil, []obsfacade.LogTrendPointVO{}, []obsfacade.RuntimeLogLineDTO{}, []obsfacade.LoggerMetricVO{}, []obsfacade.RuntimeLogLineDTO{}
	}
	start := query.StartTime.Format(time.RFC3339)
	end := query.EndTime.Format(time.RFC3339)
	limit := int64(maxInt(s.cfg.Logs.RecentLimit+s.cfg.Logs.ErrorLimit+200, 200))
	page, err := s.runtimeLog.Page(ctx, adminfacade.RuntimeLogQueryDTO{
		Current:   1,
		Size:      limit,
		StartTime: &start,
		EndTime:   &end,
	})
	if err != nil || page == nil {
		return nil, []obsfacade.LogTrendPointVO{}, []obsfacade.RuntimeLogLineDTO{}, []obsfacade.LoggerMetricVO{}, []obsfacade.RuntimeLogLineDTO{}
	}
	lines := make([]obsfacade.RuntimeLogLineDTO, 0, len(page.Records))
	for _, item := range page.Records {
		lines = append(lines, convertRuntimeLogLine(item))
	}
	return summarizeLogs(lines), buildLogTrends(query, lines), recentErrors(lines, s.cfg.Logs.ErrorLimit), hotLoggers(lines, s.cfg.Logs.HotLoggerLimit), recentLogs(lines, s.cfg.Logs.RecentLimit)
}

func (s *Service) buildDockerPanel(ctx context.Context) map[string]any {
	if s == nil || s.docker == nil {
		return nil
	}
	snapshot, err := s.docker.MetricsSnapshot(ctx)
	if err != nil || snapshot == nil {
		return map[string]any{"enabled": false, "error": fmt.Sprintf("%v", err)}
	}
	return map[string]any{
		"enabled":               snapshot.Enabled,
		"daemonHealthy":         snapshot.DaemonHealthy,
		"registryHealthy":       snapshot.RegistryHealthy,
		"containerCountByState": snapshot.ContainerCountByState,
		"imageCount":            snapshot.ImageCount,
		"imageSizeBytes":        snapshot.ImageSizeBytes,
		"operationTotal":        snapshot.OperationTotal,
		"operationSucceeded":    snapshot.OperationSucceeded,
		"operationFailed":       snapshot.OperationFailed,
		"policyViolationTotal":  snapshot.PolicyViolationTotal,
	}
}

func buildQuery(platformKey, rangeKey string) domain.Query {
	end := time.Now().UTC()
	rk := strings.ToLower(strings.TrimSpace(rangeKey))
	window := 24 * time.Hour
	bucket := 2 * time.Hour
	switch rk {
	case "1h":
		window, bucket = time.Hour, 5*time.Minute
	case "6h":
		window, bucket = 6*time.Hour, 30*time.Minute
	case "7d":
		window, bucket = 7*24*time.Hour, 12*time.Hour
	default:
		rk = "24h"
	}
	pk := strings.ToLower(strings.TrimSpace(platformKey))
	if pk == "" {
		pk = "sso"
	}
	return domain.Query{PlatformKey: pk, RangeKey: rk, StartTime: end.Add(-window), EndTime: end, BucketSize: bucket}
}

func buildTrafficTrends(query domain.Query, snapshots []domain.RuntimeSnapshot) []obsfacade.TrafficTrendPointVO {
	points := make([]obsfacade.TrafficTrendPointVO, 0)
	for _, bucket := range buckets(query) {
		items := filterSnapshots(snapshots, bucket.start, bucket.end)
		point := obsfacade.TrafficTrendPointVO{BucketStart: bucket.start, BucketLabel: bucketLabel(bucket.start, query)}
		if len(items) > 0 {
			first, last := items[0], items[len(items)-1]
			requests := maxInt64(last.RequestCount-first.RequestCount, 0)
			seconds := math.Max(1, bucket.end.Sub(bucket.start).Seconds())
			point.RequestCount = requests
			point.QPS = round(float64(requests) / seconds)
			point.P50LatencyMs = round(last.P50LatencyMs)
			point.P95LatencyMs = round(last.P95LatencyMs)
			point.P99LatencyMs = round(last.P99LatencyMs)
			point.Error4xxRate = rate(maxInt64(last.ClientErrorCount-first.ClientErrorCount, 0), requests)
			point.Error5xxRate = rate(maxInt64(last.ServerErrorCount-first.ServerErrorCount, 0), requests)
		}
		points = append(points, point)
	}
	return points
}

func buildResourceTrends(query domain.Query, snapshots []domain.RuntimeSnapshot) []obsfacade.ResourceTrendPointVO {
	points := make([]obsfacade.ResourceTrendPointVO, 0)
	for _, bucket := range buckets(query) {
		items := filterSnapshots(snapshots, bucket.start, bucket.end)
		point := obsfacade.ResourceTrendPointVO{BucketStart: bucket.start, BucketLabel: bucketLabel(bucket.start, query)}
		if len(items) > 0 {
			latest := items[len(items)-1]
			point.HeapUsedBytes = latest.HeapUsedBytes
			point.HeapCommittedBytes = latest.HeapCommittedBytes
			point.HeapMaxBytes = latest.HeapMaxBytes
			point.HeapUsedMB = round(latest.HeapUsedBytes / 1024 / 1024)
			point.HeapMaxMB = round(latest.HeapMaxBytes / 1024 / 1024)
			point.GCCount = latest.GCPauseCount
			point.GCPauseTimeMs = latest.GCPauseTimeMs
			point.LiveThreadCount = float64(latest.Goroutines)
		}
		points = append(points, point)
	}
	return points
}

func buildRuntimeSnapshot(snapshot domain.RuntimeSnapshot) obsfacade.JvmSnapshotVO {
	if snapshot.HeapUsedBytes > 0 || snapshot.Goroutines > 0 {
		return obsfacade.JvmSnapshotVO{
			HeapUsedBytes:      snapshot.HeapUsedBytes,
			HeapCommittedBytes: snapshot.HeapCommittedBytes,
			HeapMaxBytes:       snapshot.HeapMaxBytes,
			GCPauseCount:       snapshot.GCPauseCount,
			GCPauseTimeMs:      snapshot.GCPauseTimeMs,
			LiveThreadCount:    float64(snapshot.Goroutines),
		}
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return obsfacade.JvmSnapshotVO{
		HeapUsedBytes:      float64(mem.HeapAlloc),
		HeapCommittedBytes: float64(mem.HeapSys),
		HeapMaxBytes:       float64(mem.Sys),
		GCPauseCount:       int64(mem.NumGC),
		GCPauseTimeMs:      float64(mem.PauseTotalNs) / 1_000_000,
		LiveThreadCount:    float64(runtime.NumGoroutine()),
	}
}
