package application

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/observability/domain"
	obsfacade "github.com/CaixyPromise/seven-agent/force/internal/app/observability/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
)

type bucketWindow struct{ start, end time.Time }

func metric(key, label, value, trendLabel, tone string) obsfacade.MetricVO {
	return obsfacade.MetricVO{Key: key, Label: label, Value: value, TrendLabel: trendLabel, Tone: tone}
}

func rangeLabel(rangeKey string) string {
	switch rangeKey {
	case "1h":
		return "最近 1 小时"
	case "6h":
		return "最近 6 小时"
	case "7d":
		return "最近 7 天"
	default:
		return "最近 24 小时"
	}
}

func uptimeLabel(startedAt, now time.Time) string {
	if startedAt.IsZero() || now.Before(startedAt) {
		return "未知"
	}
	d := now.Sub(startedAt)
	if d >= 24*time.Hour {
		return fmt.Sprintf("%d 天 %d 小时", int(d.Hours()/24), int(d.Hours())%24)
	}
	if d >= time.Hour {
		return fmt.Sprintf("%d 小时 %d 分钟", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("%d 分钟", maxInt(int(d.Minutes()), 1))
}

func countEvents(events []ssofacade.AuditEventRecord, eventType string) int64 {
	var count int64
	for _, item := range events {
		if normalizeEventType(item.EventType) == normalizeEventType(eventType) {
			count++
		}
	}
	return count
}

func countTokenEvents(events []ssofacade.AuditEventRecord) int64 {
	var count int64
	for _, item := range events {
		if isTokenEvent(item.EventType) {
			count++
		}
	}
	return count
}

func countRiskEvents(events []ssofacade.AuditEventRecord) int64 {
	var count int64
	for _, item := range events {
		if isRiskEvent(item) {
			count++
		}
	}
	return count
}

func buildTimeline(query domain.Query, events []ssofacade.AuditEventRecord) []obsfacade.TimelinePointVO {
	points := make([]obsfacade.TimelinePointVO, 0)
	for _, bucket := range buckets(query) {
		point := obsfacade.TimelinePointVO{BucketStart: bucket.start, BucketLabel: bucketLabel(bucket.start, query)}
		for _, event := range events {
			if event.CreatedAt == nil || event.CreatedAt.Before(bucket.start) || !event.CreatedAt.Before(bucket.end) {
				continue
			}
			switch normalizeEventType(event.EventType) {
			case "login_success":
				point.LoginSuccessCount++
			case "login_failure":
				point.LoginFailureCount++
			}
			if isTokenEvent(event.EventType) {
				point.TokenIssuedCount++
			}
			if isRiskEvent(event) {
				point.RiskEventCount++
			}
		}
		points = append(points, point)
	}
	return points
}

func buildEventShares(events []ssofacade.AuditEventRecord) []obsfacade.EventShareVO {
	counts := map[string]int64{}
	for _, item := range events {
		key := strings.TrimSpace(item.EventType)
		if key != "" {
			counts[key]++
		}
	}
	keys := sortedCountKeys(counts)
	result := make([]obsfacade.EventShareVO, 0, minInt(len(keys), 6))
	for _, key := range keys {
		if len(result) >= 6 {
			break
		}
		result = append(result, obsfacade.EventShareVO{EventKey: key, EventName: eventName(key), Count: counts[key]})
	}
	return result
}

func buildTopClients(events []ssofacade.AuditEventRecord, clients []ssofacade.ClientRecord) []obsfacade.ClientActivityVO {
	names := map[string]string{}
	for _, client := range clients {
		names[client.ClientID] = firstNonEmpty(client.ClientName, client.ClientID)
	}
	type agg struct {
		count, failures int64
		last            *time.Time
	}
	aggs := map[string]*agg{}
	for _, event := range events {
		clientID := firstNonEmpty(strings.TrimSpace(event.ClientID), "unknown")
		item := aggs[clientID]
		if item == nil {
			item = &agg{}
			aggs[clientID] = item
		}
		item.count++
		if strings.EqualFold(event.Result, "FAILURE") {
			item.failures++
		}
		if event.CreatedAt != nil && (item.last == nil || event.CreatedAt.After(*item.last)) {
			value := *event.CreatedAt
			item.last = &value
		}
	}
	keys := make([]string, 0, len(aggs))
	for key := range aggs {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool { return aggs[keys[i]].count > aggs[keys[j]].count })
	result := make([]obsfacade.ClientActivityVO, 0, minInt(len(keys), 5))
	for _, key := range keys {
		if len(result) >= 5 {
			break
		}
		item := aggs[key]
		result = append(result, obsfacade.ClientActivityVO{ClientID: key, ClientName: firstNonEmpty(names[key], key), EventCount: item.count, FailureCount: item.failures, LastActivityAt: item.last})
	}
	return result
}

func buildAlerts(events []ssofacade.AuditEventRecord, clients []ssofacade.ClientRecord) []obsfacade.AlertVO {
	names := map[string]string{}
	for _, client := range clients {
		names[client.ClientID] = firstNonEmpty(client.ClientName, client.ClientID)
	}
	result := make([]obsfacade.AlertVO, 0, 10)
	for _, event := range events {
		if !isRiskEvent(event) {
			continue
		}
		result = append(result, obsfacade.AlertVO{
			ID:         event.ID,
			Severity:   alertSeverity(event.EventType),
			EventType:  event.EventType,
			Title:      eventName(event.EventType),
			Summary:    firstNonEmpty(event.ReasonCode, event.EventType),
			ReasonCode: event.ReasonCode,
			ClientID:   event.ClientID,
			ClientName: firstNonEmpty(names[event.ClientID], event.ClientID),
			CreatedAt:  event.CreatedAt,
		})
		if len(result) >= 10 {
			break
		}
	}
	return result
}

func summarizeLogs(lines []obsfacade.RuntimeLogLineDTO) *obsfacade.LogSummaryVO {
	summary := &obsfacade.LogSummaryVO{Total: int64(len(lines))}
	for idx, line := range lines {
		if idx == 0 {
			summary.LatestLevel = line.Level
		}
		switch strings.ToUpper(strings.TrimSpace(line.Level)) {
		case "DEBUG":
			summary.Debug++
		case "WARN", "WARNING":
			summary.Warn++
		case "ERROR", "FATAL", "PANIC":
			summary.Error++
		default:
			summary.Info++
		}
	}
	return summary
}

func buildLogTrends(query domain.Query, lines []obsfacade.RuntimeLogLineDTO) []obsfacade.LogTrendPointVO {
	result := make([]obsfacade.LogTrendPointVO, 0)
	for _, bucket := range buckets(query) {
		point := obsfacade.LogTrendPointVO{BucketStart: bucket.start, BucketLabel: bucketLabel(bucket.start, query)}
		for _, line := range lines {
			if line.LogTime == nil || line.LogTime.Before(bucket.start) || !line.LogTime.Before(bucket.end) {
				continue
			}
			switch strings.ToUpper(strings.TrimSpace(line.Level)) {
			case "DEBUG":
				point.Debug++
			case "WARN", "WARNING":
				point.Warn++
			case "ERROR", "FATAL", "PANIC":
				point.Error++
			default:
				point.Info++
			}
		}
		result = append(result, point)
	}
	return result
}

func recentErrors(lines []obsfacade.RuntimeLogLineDTO, limit int) []obsfacade.RuntimeLogLineDTO {
	result := make([]obsfacade.RuntimeLogLineDTO, 0, limit)
	for _, line := range lines {
		level := strings.ToUpper(strings.TrimSpace(line.Level))
		if level == "ERROR" || level == "FATAL" || level == "PANIC" {
			result = append(result, line)
			if len(result) >= limit {
				break
			}
		}
	}
	return result
}

func hotLoggers(lines []obsfacade.RuntimeLogLineDTO, limit int) []obsfacade.LoggerMetricVO {
	type agg struct{ count, errors int64 }
	aggs := map[string]*agg{}
	for _, line := range lines {
		key := firstNonEmpty(strings.TrimSpace(line.LoggerName), "unknown")
		item := aggs[key]
		if item == nil {
			item = &agg{}
			aggs[key] = item
		}
		item.count++
		level := strings.ToUpper(strings.TrimSpace(line.Level))
		if level == "ERROR" || level == "FATAL" || level == "PANIC" {
			item.errors++
		}
	}
	keys := make([]string, 0, len(aggs))
	for key := range aggs {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool { return aggs[keys[i]].count > aggs[keys[j]].count })
	result := make([]obsfacade.LoggerMetricVO, 0, minInt(limit, len(keys)))
	for _, key := range keys {
		if len(result) >= limit {
			break
		}
		result = append(result, obsfacade.LoggerMetricVO{LoggerName: key, Count: aggs[key].count, ErrorCount: aggs[key].errors})
	}
	return result
}

func recentLogs(lines []obsfacade.RuntimeLogLineDTO, limit int) []obsfacade.RuntimeLogLineDTO {
	if limit <= 0 || len(lines) <= limit {
		return append([]obsfacade.RuntimeLogLineDTO(nil), lines...)
	}
	return append([]obsfacade.RuntimeLogLineDTO(nil), lines[:limit]...)
}

func convertRuntimeLogLine(item adminfacade.RuntimeLogLineDTO) obsfacade.RuntimeLogLineDTO {
	return obsfacade.RuntimeLogLineDTO(item)
}

func buckets(query domain.Query) []bucketWindow {
	var result []bucketWindow
	for cursor := query.StartTime; !cursor.After(query.EndTime); cursor = cursor.Add(query.BucketSize) {
		result = append(result, bucketWindow{start: cursor, end: cursor.Add(query.BucketSize)})
	}
	return result
}

func filterSnapshots(items []domain.RuntimeSnapshot, start, end time.Time) []domain.RuntimeSnapshot {
	result := make([]domain.RuntimeSnapshot, 0)
	for _, item := range items {
		if !item.CapturedAt.Before(start) && item.CapturedAt.Before(end) {
			result = append(result, item)
		}
	}
	return result
}

func bucketLabel(value time.Time, query domain.Query) string {
	if query.RangeKey == "7d" {
		return value.Format("01-02 15:04")
	}
	return value.Format("15:04")
}

func eventName(eventType string) string {
	switch normalizeEventType(eventType) {
	case "login_success":
		return "登录成功"
	case "login_failure":
		return "登录失败"
	case "token_issued":
		return "令牌签发"
	case "token_refreshed":
		return "令牌刷新"
	case "challenge_required":
		return "需要二次校验"
	case "refresh_reuse_detected", "token_refresh_reuse_detected":
		return "刷新令牌复用"
	case "token_validation_failed":
		return "令牌校验失败"
	case "challenge_failed":
		return "二次校验失败"
	case "client_auth_failed":
		return "客户端认证失败"
	case "login_locked":
		return "登录临时锁定"
	case "token_exchanged":
		return "登录令牌换取"
	case "token_revoked":
		return "令牌注销"
	case "token_introspected":
		return "令牌状态检查"
	case "userinfo_accessed":
		return "用户信息访问"
	case "interactive_login_completed":
		return "统一登录完成"
	case "session_revoked":
		return "会话注销"
	case "user_sessions_revoked":
		return "用户会话注销"
	default:
		return eventType
	}
}

func resolvePlatformStatus(failureRate float64, riskEvents int64) string {
	if riskEvents > 0 || failureRate >= 0.2 {
		return "warning"
	}
	return "healthy"
}

func statusLabel(status string) string {
	if status == "warning" {
		return "需关注"
	}
	return "健康"
}

func resolveHealthLabel(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "UP", "HEALTHY":
		return "正常"
	case "UNKNOWN":
		return "未知"
	default:
		return "异常"
	}
}

func boolStatus(ok bool) string {
	if ok {
		return "UP"
	}
	return "DOWN"
}

func alertSeverity(eventType string) string {
	switch normalizeEventType(eventType) {
	case "refresh_reuse_detected", "token_refresh_reuse_detected", "login_locked":
		return "HIGH"
	case "token_exchanged", "token_refreshed", "token_revoked", "token_introspected",
		"token_validation_failed", "client_auth_failed", "challenge_failed":
		return "MEDIUM"
	default:
		return "LOW"
	}
}

func normalizeEventType(eventType string) string {
	return strings.ToLower(strings.TrimSpace(eventType))
}

func isTokenEvent(eventType string) bool {
	switch normalizeEventType(eventType) {
	case "token_issued", "token_refreshed", "token_exchanged", "token_revoked", "token_introspected":
		return true
	default:
		return false
	}
}

func isRiskEvent(event ssofacade.AuditEventRecord) bool {
	switch normalizeEventType(event.EventType) {
	case "login_failure", "challenge_failed", "refresh_reuse_detected", "token_refresh_reuse_detected",
		"client_auth_failed", "token_validation_failed", "login_locked":
		return true
	case "token_exchanged", "token_refreshed", "token_revoked", "token_introspected", "userinfo_accessed":
		return strings.EqualFold(strings.TrimSpace(event.Result), "FAILURE")
	default:
		return false
	}
}

func sortedCountKeys(counts map[string]int64) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.SliceStable(keys, func(i, j int) bool { return counts[keys[i]] > counts[keys[j]] })
	return keys
}

func rate(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return round(float64(part) / float64(total))
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.2f%%", value*100)
}

func round(value float64) float64 {
	return math.Round(value*100) / 100
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func maxInt(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt64(value, fallback int64) int64 {
	if value > fallback {
		return value
	}
	return fallback
}
