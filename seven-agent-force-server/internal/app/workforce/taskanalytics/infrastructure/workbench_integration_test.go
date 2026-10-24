package infrastructure

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	analytics "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskanalytics/application"
)

func TestWorkbenchAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("WORKFORCE_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WORKFORCE_POSTGRES_DSN is not configured")
	}
	db, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tenantID int64
	if err = db.QueryRow(context.Background(), `SELECT tenant_id FROM workforce_tasks ORDER BY created_at LIMIT 1`).Scan(&tenantID); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
	result, err := New(db).Workbench(context.Background(), tenantID, analytics.Window{From: now.AddDate(0, 0, -14), To: now, Days: 14}, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ActivityTrend) != 14 {
		t.Fatalf("activity points=%d, want 14", len(result.ActivityTrend))
	}
}
