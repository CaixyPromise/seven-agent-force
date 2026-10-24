package facade

import "time"

type OverviewVO struct {
	GeneratedAt         time.Time           `json:"generatedAt"`
	TimeRangeLabel      string              `json:"timeRangeLabel"`
	SelectedPlatformKey string              `json:"selectedPlatformKey"`
	RangeKey            string              `json:"rangeKey"`
	HeadlineMetrics     []MetricVO          `json:"headlineMetrics"`
	Platforms           []PlatformVO        `json:"platforms"`
	LogSummary          *LogSummaryVO       `json:"logSummary,omitempty"`
	LogTrends           []LogTrendPointVO   `json:"logTrends,omitempty"`
	RecentErrors        []RuntimeLogLineDTO `json:"recentErrors,omitempty"`
	HotLoggers          []LoggerMetricVO    `json:"hotLoggers,omitempty"`
	RecentLogs          []RuntimeLogLineDTO `json:"recentLogs,omitempty"`
	LogStreamEnabled    bool                `json:"logStreamEnabled,omitempty"`
	Extra               map[string]any      `json:"extra,omitempty"`
}

type PlatformVO struct {
	PlatformKey      string                 `json:"platformKey"`
	PlatformName     string                 `json:"platformName"`
	Description      string                 `json:"description"`
	Status           string                 `json:"status"`
	StatusLabel      string                 `json:"statusLabel"`
	LastUpdated      time.Time              `json:"lastUpdated"`
	Metrics          []MetricVO             `json:"metrics"`
	HealthSummary    HealthSummaryVO        `json:"healthSummary"`
	TrafficTrends    []TrafficTrendPointVO  `json:"trafficTrends"`
	ResourceTrends   []ResourceTrendPointVO `json:"resourceTrends"`
	JvmSnapshot      JvmSnapshotVO          `json:"jvmSnapshot"`
	DependencyMatrix []DependencyHealthVO   `json:"dependencyMatrix"`
	MiddlewarePanels []MiddlewarePanelVO    `json:"middlewarePanels"`
	Timeline         []TimelinePointVO      `json:"timeline"`
	EndpointInsights []EndpointInsightVO    `json:"endpointInsights"`
	EventShares      []EventShareVO         `json:"eventShares"`
	TopClients       []ClientActivityVO     `json:"topClients"`
	Alerts           []AlertVO              `json:"alerts"`
	ExtensionPanels  []ExtensionPanelVO     `json:"extensionPanels"`
}

type MetricVO struct {
	Key        string `json:"key"`
	Label      string `json:"label"`
	Value      string `json:"value"`
	Unit       string `json:"unit,omitempty"`
	TrendLabel string `json:"trendLabel,omitempty"`
	Tone       string `json:"tone,omitempty"`
}

type HealthSummaryVO struct {
	OverallStatus      string `json:"overallStatus"`
	OverallStatusLabel string `json:"overallStatusLabel"`
	LivenessStatus     string `json:"livenessStatus"`
	ReadinessStatus    string `json:"readinessStatus"`
	UptimeLabel        string `json:"uptimeLabel"`
	Version            string `json:"version,omitempty"`
	GitCommit          string `json:"gitCommit,omitempty"`
}

type TrafficTrendPointVO struct {
	BucketLabel  string    `json:"bucketLabel"`
	BucketStart  time.Time `json:"bucketStart"`
	RequestCount int64     `json:"requestCount"`
	QPS          float64   `json:"qps"`
	P50LatencyMs float64   `json:"p50LatencyMs"`
	P95LatencyMs float64   `json:"p95LatencyMs"`
	P99LatencyMs float64   `json:"p99LatencyMs"`
	Error4xxRate float64   `json:"error4xxRate"`
	Error5xxRate float64   `json:"error5xxRate"`
}

type ResourceTrendPointVO struct {
	BucketLabel                   string    `json:"bucketLabel"`
	BucketStart                   time.Time `json:"bucketStart"`
	CPUUsagePercent               float64   `json:"cpuUsagePercent"`
	ProcessCPUUsagePercent        float64   `json:"processCpuUsagePercent"`
	HeapUsedMB                    float64   `json:"heapUsedMb"`
	HeapUsedBytes                 float64   `json:"heapUsedBytes"`
	HeapCommittedBytes            float64   `json:"heapCommittedBytes"`
	HeapMaxMB                     float64   `json:"heapMaxMb"`
	HeapMaxBytes                  float64   `json:"heapMaxBytes"`
	NonHeapUsedBytes              float64   `json:"nonHeapUsedBytes"`
	MetaspaceUsedBytes            float64   `json:"metaspaceUsedBytes"`
	GCCount                       int64     `json:"gcCount"`
	GCPauseTimeMs                 float64   `json:"gcPauseTimeMs"`
	GCPauseMaxMs                  float64   `json:"gcPauseMaxMs"`
	LiveThreadCount               float64   `json:"liveThreadCount"`
	DaemonThreadCount             float64   `json:"daemonThreadCount"`
	DiskUsagePercent              float64   `json:"diskUsagePercent"`
	DiskTotalBytes                float64   `json:"diskTotalBytes"`
	DiskFreeBytes                 float64   `json:"diskFreeBytes"`
	NetworkReceiveBytesPerSecond  float64   `json:"networkReceiveBytesPerSecond"`
	NetworkMetricsAvailable       bool      `json:"networkMetricsAvailable"`
	NetworkTransmitBytesPerSecond float64   `json:"networkTransmitBytesPerSecond"`
	DiskReadBytesPerSecond        float64   `json:"diskReadBytesPerSecond"`
	DiskIOMetricsAvailable        bool      `json:"diskIoMetricsAvailable"`
	DiskWriteBytesPerSecond       float64   `json:"diskWriteBytesPerSecond"`
}

type JvmSnapshotVO struct {
	SystemCPUUsagePercent   float64 `json:"systemCpuUsagePercent"`
	ProcessCPUUsagePercent  float64 `json:"processCpuUsagePercent"`
	HeapUsedBytes           float64 `json:"heapUsedBytes"`
	HeapCommittedBytes      float64 `json:"heapCommittedBytes"`
	HeapMaxBytes            float64 `json:"heapMaxBytes"`
	NonHeapUsedBytes        float64 `json:"nonHeapUsedBytes"`
	MetaspaceUsedBytes      float64 `json:"metaspaceUsedBytes"`
	CodeCacheUsedBytes      float64 `json:"codeCacheUsedBytes"`
	LiveThreadCount         float64 `json:"liveThreadCount"`
	DaemonThreadCount       float64 `json:"daemonThreadCount"`
	PeakThreadCount         float64 `json:"peakThreadCount"`
	GCPauseCount            int64   `json:"gcPauseCount"`
	YoungGCCount            int64   `json:"youngGcCount"`
	FullGCCount             int64   `json:"fullGcCount"`
	GCPauseTimeMs           float64 `json:"gcPauseTimeMs"`
	YoungGCPauseTimeMs      float64 `json:"youngGcPauseTimeMs"`
	FullGCPauseTimeMs       float64 `json:"fullGcPauseTimeMs"`
	GCPauseMaxMs            float64 `json:"gcPauseMaxMs"`
	FullGCPauseMaxMs        float64 `json:"fullGcPauseMaxMs"`
	OpenFileDescriptorCount float64 `json:"openFileDescriptorCount"`
	MaxFileDescriptorCount  float64 `json:"maxFileDescriptorCount"`
	DiskTotalBytes          float64 `json:"diskTotalBytes"`
	DiskFreeBytes           float64 `json:"diskFreeBytes"`
}

type DependencyHealthVO struct {
	DependencyKey  string `json:"dependencyKey"`
	DependencyName string `json:"dependencyName"`
	Status         string `json:"status"`
	StatusLabel    string `json:"statusLabel"`
	Detail         string `json:"detail,omitempty"`
}

type MiddlewarePanelVO struct {
	PanelKey    string     `json:"panelKey"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	StatusLabel string     `json:"statusLabel,omitempty"`
	Metrics     []MetricVO `json:"metrics"`
	DetailLines []string   `json:"detailLines,omitempty"`
}

type TimelinePointVO struct {
	BucketLabel       string    `json:"bucketLabel"`
	BucketStart       time.Time `json:"bucketStart"`
	LoginSuccessCount int64     `json:"loginSuccessCount"`
	LoginFailureCount int64     `json:"loginFailureCount"`
	TokenIssuedCount  int64     `json:"tokenIssuedCount"`
	RiskEventCount    int64     `json:"riskEventCount"`
}

type EndpointInsightVO struct {
	InsightKey       string  `json:"insightKey"`
	Method           string  `json:"method"`
	URI              string  `json:"uri"`
	RequestCount     int64   `json:"requestCount"`
	AverageLatencyMs float64 `json:"averageLatencyMs"`
	MaxLatencyMs     float64 `json:"maxLatencyMs"`
	ErrorRate        float64 `json:"errorRate"`
	Severity         string  `json:"severity"`
	SeverityLabel    string  `json:"severityLabel"`
	Summary          string  `json:"summary"`
}

type EventShareVO struct {
	EventKey  string `json:"eventKey"`
	EventName string `json:"eventName"`
	Count     int64  `json:"count"`
}

type ClientActivityVO struct {
	ClientID       string     `json:"clientId"`
	ClientName     string     `json:"clientName"`
	EventCount     int64      `json:"eventCount"`
	FailureCount   int64      `json:"failureCount"`
	LastActivityAt *time.Time `json:"lastActivityAt,omitempty"`
}

type AlertVO struct {
	ID         int64      `json:"id"`
	Severity   string     `json:"severity"`
	EventType  string     `json:"eventType"`
	Title      string     `json:"title"`
	Summary    string     `json:"summary"`
	ReasonCode string     `json:"reasonCode,omitempty"`
	ClientID   string     `json:"clientId,omitempty"`
	ClientName string     `json:"clientName,omitempty"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
}

type ExtensionPanelVO struct {
	PanelKey    string     `json:"panelKey"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Status      string     `json:"status"`
	Metrics     []MetricVO `json:"metrics"`
}

type RuntimeLogLineDTO struct {
	LineID     string         `json:"lineId"`
	LogTime    *time.Time     `json:"logTime,omitempty"`
	Level      string         `json:"level,omitempty"`
	ThreadName string         `json:"threadName,omitempty"`
	LoggerName string         `json:"loggerName,omitempty"`
	TraceID    string         `json:"traceId,omitempty"`
	Message    string         `json:"message,omitempty"`
	Source     map[string]any `json:"source,omitempty"`
	FileName   string         `json:"fileName,omitempty"`
	LineNumber int            `json:"lineNumber,omitempty"`
}

type LogSummaryVO struct {
	Total       int64  `json:"total"`
	Debug       int64  `json:"debug"`
	Info        int64  `json:"info"`
	Warn        int64  `json:"warn"`
	Error       int64  `json:"error"`
	LatestLevel string `json:"latestLevel,omitempty"`
}

type LogTrendPointVO struct {
	BucketLabel string    `json:"bucketLabel"`
	BucketStart time.Time `json:"bucketStart"`
	Debug       int64     `json:"debug"`
	Info        int64     `json:"info"`
	Warn        int64     `json:"warn"`
	Error       int64     `json:"error"`
}

type LoggerMetricVO struct {
	LoggerName string `json:"loggerName"`
	Count      int64  `json:"count"`
	ErrorCount int64  `json:"errorCount"`
}
