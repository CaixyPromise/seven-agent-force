import { request } from '@/api/request';

export interface ResultEnvelope<T> {
  code: number;
  message?: string;
  data: T;
}

export interface ObservabilityMetric {
  key: string;
  label: string;
  value: string;
  unit?: string;
  trendLabel?: string;
  tone?: string;
}

export interface ObservabilityHealthSummary {
  overallStatus: string;
  overallStatusLabel: string;
  livenessStatus: string;
  readinessStatus: string;
  uptimeLabel: string;
  version: string;
  gitCommit: string;
}

export interface ObservabilityTrafficTrendPoint {
  bucketLabel: string;
  bucketStart?: string;
  requestCount: number;
  qps: number;
  p50LatencyMs: number;
  p95LatencyMs: number;
  p99LatencyMs: number;
  error4xxRate: number;
  error5xxRate: number;
}

export interface ObservabilityResourceTrendPoint {
  bucketLabel: string;
  bucketStart?: string;
  cpuUsagePercent: number;
  processCpuUsagePercent: number;
  heapUsedMb: number;
  heapUsedBytes: number;
  heapCommittedBytes: number;
  heapMaxMb: number;
  heapMaxBytes: number;
  nonHeapUsedBytes: number;
  metaspaceUsedBytes: number;
  gcCount: number;
  gcPauseTimeMs: number;
  gcPauseMaxMs: number;
  liveThreadCount: number;
  daemonThreadCount: number;
  diskUsagePercent: number;
  diskTotalBytes: number;
  diskFreeBytes: number;
  networkReceiveBytesPerSecond: number;
  networkMetricsAvailable: boolean;
  networkTransmitBytesPerSecond: number;
  diskReadBytesPerSecond: number;
  diskIoMetricsAvailable: boolean;
  diskWriteBytesPerSecond: number;
}

export interface ObservabilityDependencyHealth {
  dependencyKey: string;
  dependencyName: string;
  status: string;
  statusLabel: string;
  detail?: string;
}

export interface ObservabilityTimelinePoint {
  bucketLabel: string;
  bucketStart?: string;
  loginSuccessCount: number;
  loginFailureCount: number;
  tokenIssuedCount: number;
  riskEventCount: number;
}

export interface ObservabilityEventShare {
  eventKey: string;
  eventName: string;
  count: number;
}

export interface ObservabilityClientActivity {
  clientId: string;
  clientName: string;
  eventCount: number;
  failureCount: number;
  lastActivityAt?: string;
}

export interface ObservabilityAlert {
  id?: API.Int64;
  severity: string;
  eventType: string;
  title: string;
  summary: string;
  reasonCode?: string;
  clientId?: string;
  clientName?: string;
  createdAt?: string;
}

export interface ObservabilityExtensionPanel {
  panelKey: string;
  title: string;
  description: string;
  status: string;
  metrics: ObservabilityMetric[];
}

export interface ObservabilityEndpointInsight {
  insightKey: string;
  method: string;
  uri: string;
  requestCount: number;
  averageLatencyMs: number;
  maxLatencyMs: number;
  errorRate: number;
  severity: string;
  severityLabel: string;
  summary: string;
}

export interface ObservabilityJvmSnapshot {
  systemCpuUsagePercent: number;
  processCpuUsagePercent: number;
  heapUsedBytes: number;
  heapCommittedBytes: number;
  heapMaxBytes: number;
  nonHeapUsedBytes: number;
  metaspaceUsedBytes: number;
  codeCacheUsedBytes: number;
  liveThreadCount: number;
  daemonThreadCount: number;
  peakThreadCount: number;
  gcPauseCount: number;
  youngGcCount: number;
  fullGcCount: number;
  gcPauseTimeMs: number;
  youngGcPauseTimeMs: number;
  fullGcPauseTimeMs: number;
  gcPauseMaxMs: number;
  fullGcPauseMaxMs: number;
  openFileDescriptorCount: number;
  maxFileDescriptorCount: number;
  diskTotalBytes: number;
  diskFreeBytes: number;
}

export interface ObservabilityMiddlewarePanel {
  panelKey: string;
  title: string;
  description: string;
  status: string;
  statusLabel: string;
  metrics: ObservabilityMetric[];
  detailLines: string[];
}

export interface ObservabilityPlatform {
  platformKey: string;
  platformName: string;
  description: string;
  status: string;
  statusLabel: string;
  lastUpdated?: string;
  metrics: ObservabilityMetric[];
  healthSummary: ObservabilityHealthSummary;
  trafficTrends: ObservabilityTrafficTrendPoint[];
  resourceTrends: ObservabilityResourceTrendPoint[];
  jvmSnapshot: ObservabilityJvmSnapshot;
  dependencyMatrix: ObservabilityDependencyHealth[];
  middlewarePanels: ObservabilityMiddlewarePanel[];
  timeline: ObservabilityTimelinePoint[];
  endpointInsights: ObservabilityEndpointInsight[];
  eventShares: ObservabilityEventShare[];
  topClients: ObservabilityClientActivity[];
  alerts: ObservabilityAlert[];
  extensionPanels: ObservabilityExtensionPanel[];
}

export interface ObservabilityOverview {
  generatedAt?: string;
  selectedPlatformKey: string;
  rangeKey: string;
  timeRangeLabel: string;
  headlineMetrics: ObservabilityMetric[];
  platforms: ObservabilityPlatform[];
  extra?: {
    docker?: {
      enabled?: boolean;
      daemonHealthy?: boolean;
      registryHealthy?: boolean;
      containerCountByState?: Record<string, number>;
      imageCount?: number;
      imageSizeBytes?: number;
      operationTotal?: number;
      operationSucceeded?: number;
      operationFailed?: number;
      policyViolationTotal?: number;
    };
  };
}

export async function getObservabilityOverview(params?: { platform?: string; range?: string }) {
  const search = new URLSearchParams();
  if (params?.platform) {
    search.set('platform', params.platform);
  }
  if (params?.range) {
    search.set('range', params.range);
  }
  const suffix = search.toString() ? `?${search.toString()}` : '';
  return request<ResultEnvelope<ObservabilityOverview>>(`/api/observability/overview${suffix}`, {
    method: 'GET',
  });
}
