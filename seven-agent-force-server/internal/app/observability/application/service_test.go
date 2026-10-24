package application

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/observability/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestGetOverviewBuildsSSOAndLogPanels(t *testing.T) {
	now := time.Now().UTC()
	store := &fakeSnapshotStore{items: []domain.RuntimeSnapshot{{CapturedAt: now.Add(-time.Minute), RequestCount: 10}, {CapturedAt: now, RequestCount: 20, ServerErrorCount: 1}}}
	service := NewService(
		config.ObservabilityConfig{Logs: config.ObservabilityLogsConfig{Enabled: true, RecentLimit: 2, ErrorLimit: 2, HotLoggerLimit: 2, TrendBucketSeconds: 300}},
		store,
		nil,
		fakeRuntimeSnapshotProvider{},
		fakeAuditFacade{events: []ssofacade.AuditEventRecord{
			{ID: 1, EventType: "login_success", ClientID: "console", Result: "SUCCESS", CreatedAt: ptrTime(now.Add(-10 * time.Minute))},
			{ID: 2, EventType: "login_failure", ClientID: "console", Result: "FAILURE", ReasonCode: "bad_password", CreatedAt: ptrTime(now.Add(-5 * time.Minute))},
			{ID: 3, EventType: "token_issued", ClientID: "console", Result: "SUCCESS", CreatedAt: ptrTime(now.Add(-3 * time.Minute))},
			{ID: 4, EventType: "TOKEN_EXCHANGED", ClientID: "console", Result: "FAILURE", ReasonCode: "invalid_code", CreatedAt: ptrTime(now.Add(-2 * time.Minute))},
			{ID: 5, EventType: "TOKEN_REFRESH_REUSE_DETECTED", ClientID: "console", Result: "FAILURE", ReasonCode: "reuse_detected", CreatedAt: ptrTime(now.Add(-1 * time.Minute))},
			{ID: 6, EventType: "TOKEN_EXCHANGED", ClientID: "console", Result: "SUCCESS", ReasonCode: "exchanged", CreatedAt: ptrTime(now.Add(-30 * time.Second))},
		}},
		fakeClientFacade{items: []ssofacade.ClientRecord{{ClientID: "console", ClientName: "Console"}}},
		fakeSessionFacade{count: 3},
		fakeRuntimeLogFacade{lines: []adminfacade.RuntimeLogLineDTO{
			{LineID: "1", LogTime: ptrTime(now.Add(-2 * time.Minute)), Level: "ERROR", LoggerName: "auth", Message: "failed password=***"},
			{LineID: "2", LogTime: ptrTime(now.Add(-1 * time.Minute)), Level: "INFO", LoggerName: "auth", Message: "ok"},
		}},
		nil,
	)

	overview, err := service.GetOverview(context.Background(), "", "1h")
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.SelectedPlatformKey != "sso" || overview.RangeKey != "1h" {
		t.Fatalf("unexpected overview keys: %+v", overview)
	}
	if len(overview.Platforms) != 1 || len(overview.HeadlineMetrics) == 0 {
		t.Fatalf("expected platform metrics: %+v", overview)
	}
	if overview.LogSummary == nil || overview.LogSummary.Error != 1 || len(overview.RecentErrors) != 1 {
		t.Fatalf("expected log summary/errors: %+v", overview.LogSummary)
	}
	if !overview.LogStreamEnabled {
		t.Fatal("expected log stream enabled")
	}
	if store.appendCount != 0 {
		t.Fatalf("overview should not mutate snapshot store, appendCount=%d", store.appendCount)
	}
	alerts := overview.Platforms[0].Alerts
	if len(alerts) != 3 {
		t.Fatalf("expected three security alerts, got %+v", alerts)
	}
	alertByID := map[int64]ssofacade.AuditEventRecord{}
	severityByID := map[int64]string{}
	titleByID := map[int64]string{}
	for _, alert := range alerts {
		alertByID[alert.ID] = ssofacade.AuditEventRecord{ID: alert.ID, EventType: alert.EventType, Result: alert.Summary}
		severityByID[alert.ID] = alert.Severity
		titleByID[alert.ID] = alert.Title
	}
	if _, ok := alertByID[6]; ok {
		t.Fatalf("successful token exchange must not become a security alert: %+v", alerts)
	}
	if severityByID[4] != "MEDIUM" || titleByID[4] != "登录令牌换取" {
		t.Fatalf("expected token exchange failure alert to be localized MEDIUM, got severity=%q title=%q", severityByID[4], titleByID[4])
	}
	if severityByID[5] != "HIGH" || titleByID[5] != "刷新令牌复用" {
		t.Fatalf("expected refresh reuse alert to be localized HIGH, got severity=%q title=%q", severityByID[5], titleByID[5])
	}
}

func TestGetOverviewDisablesLogPanelWhenConfigured(t *testing.T) {
	service := NewService(
		config.ObservabilityConfig{Logs: config.ObservabilityLogsConfig{Enabled: false}},
		&fakeSnapshotStore{},
		nil,
		fakeRuntimeSnapshotProvider{},
		fakeAuditFacade{},
		fakeClientFacade{},
		fakeSessionFacade{},
		fakeRuntimeLogFacade{},
		nil,
	)
	overview, err := service.GetOverview(context.Background(), "unknown", "bad")
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.RangeKey != "24h" || overview.LogStreamEnabled {
		t.Fatalf("unexpected fallback/log status: %+v", overview)
	}
	if overview.LogSummary != nil || len(overview.RecentLogs) != 0 {
		t.Fatalf("expected empty log panel: %+v", overview)
	}
}

func TestRefreshSnapshotsAppendsRuntimeSnapshot(t *testing.T) {
	store := &fakeSnapshotStore{}
	service := NewService(
		config.ObservabilityConfig{},
		store,
		nil,
		fakeRuntimeSnapshotProvider{},
		fakeAuditFacade{},
		fakeClientFacade{},
		fakeSessionFacade{},
		nil,
		nil,
	)

	if err := service.RefreshSnapshots(context.Background()); err != nil {
		t.Fatalf("RefreshSnapshots() error = %v", err)
	}
	if store.appendCount != 1 {
		t.Fatalf("expected one snapshot append, got %d", store.appendCount)
	}
}

func TestStreamLogsReturnsErrorWhenRuntimeLogUnavailable(t *testing.T) {
	service := NewService(
		config.ObservabilityConfig{},
		&fakeSnapshotStore{},
		nil,
		fakeRuntimeSnapshotProvider{},
		fakeAuditFacade{},
		fakeClientFacade{},
		fakeSessionFacade{},
		nil,
		nil,
	)

	if _, err := service.StreamLogs(context.Background(), adminfacade.RuntimeLogStreamRequestDTO{}, 1); err == nil {
		t.Fatal("expected error when runtime log facade is unavailable")
	}
}

type fakeSnapshotStore struct {
	items       []domain.RuntimeSnapshot
	appendCount int
}

type fakeRuntimeSnapshotProvider struct{}

func (fakeRuntimeSnapshotProvider) Snapshot(context.Context) domain.RuntimeSnapshot {
	return domain.RuntimeSnapshot{CapturedAt: time.Now().UTC(), DatasourceUp: true}
}

func (f *fakeSnapshotStore) Append(context.Context, string, domain.RuntimeSnapshot) error {
	f.appendCount++
	return nil
}
func (f *fakeSnapshotStore) ListBetween(context.Context, string, time.Time, time.Time) ([]domain.RuntimeSnapshot, error) {
	return f.items, nil
}
func (f *fakeSnapshotStore) Latest(context.Context, string) (*domain.RuntimeSnapshot, error) {
	if len(f.items) == 0 {
		return nil, nil
	}
	return &f.items[len(f.items)-1], nil
}

type fakeAuditFacade struct{ events []ssofacade.AuditEventRecord }

func (f fakeAuditFacade) ListEventsSince(context.Context, time.Time) ([]ssofacade.AuditEventRecord, error) {
	return f.events, nil
}

type fakeClientFacade struct{ items []ssofacade.ClientRecord }

func (f fakeClientFacade) ListEnabledClients(context.Context) ([]ssofacade.ClientRecord, error) {
	return f.items, nil
}

type fakeSessionFacade struct{ count int64 }

func (f fakeSessionFacade) ListSessionsByUserID(context.Context, int64) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}
func (f fakeSessionFacade) ListActiveSessions(context.Context) ([]ssofacade.SessionRecord, error) {
	return nil, nil
}
func (f fakeSessionFacade) CountActiveSessions(context.Context) (int64, error)  { return f.count, nil }
func (f fakeSessionFacade) RevokeSession(context.Context, string) (bool, error) { return true, nil }
func (f fakeSessionFacade) RevokeSessionsByUserID(context.Context, int64) (int64, error) {
	return 0, nil
}
func (f fakeSessionFacade) RevokeSessionsByPlatformCode(context.Context, string) (int64, error) {
	return 0, nil
}
func (f fakeSessionFacade) RevokeSessionsByPlatformLoginMethod(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (f fakeSessionFacade) RevokeSessionsByExternalProvider(context.Context, string) (int64, error) {
	return 0, nil
}
func (f fakeSessionFacade) RevokeSessionsByExternalIdentity(context.Context, int64) (int64, error) {
	return 0, nil
}
func (f fakeSessionFacade) ResolveActiveSessionRecord(context.Context, string) (*ssofacade.SessionRecord, error) {
	return nil, nil
}

type fakeRuntimeLogFacade struct {
	lines []adminfacade.RuntimeLogLineDTO
}

func (f fakeRuntimeLogFacade) Page(_ context.Context, request adminfacade.RuntimeLogQueryDTO) (*adminfacade.PageResult[adminfacade.RuntimeLogLineDTO], error) {
	return &adminfacade.PageResult[adminfacade.RuntimeLogLineDTO]{Current: request.Current, Size: request.Size, Total: int64(len(f.lines)), Records: f.lines}, nil
}
func (f fakeRuntimeLogFacade) Stream(context.Context, adminfacade.RuntimeLogStreamRequestDTO, int64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("event: heartbeat\ndata: {}\n\n")), nil
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
