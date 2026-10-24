package acceptance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

const dg5ExplainEnv = "DG5_CACHE_GOVERNANCE_EXPLAIN"

// TestDG5FreshnessFenceExplain records real dual-dialect cost evidence for
// the exact fail-closed predicate used by the source-adjacent DG5 fence. It
// writes only de-identified, self-cleaning history into an already-guarded
// governance database; it never proposes or applies an index migration.
func TestDG5FreshnessFenceExplain(t *testing.T) {
	if strings.ToLower(strings.TrimSpace(os.Getenv(dg5ExplainEnv))) != dg5AcceptanceApply {
		t.Skip("set DG5_CACHE_GOVERNANCE_EXPLAIN=apply after the isolated migration path")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("dg5-explain-%s-%d-", target.dialect, time.Now().UTC().UnixNano())
	if err := dg5SeedFreshnessExplainRows(ctx, target.db, target.dialect, prefix); err != nil {
		t.Fatalf("seed isolated DG5 EXPLAIN fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := AssertConnectedDatabase(context.Background(), target.db, target.dialect); err == nil {
			_, _ = target.db.ExecContext(context.Background(), dg5ExplainCleanupSQL(target.dialect), prefix+"%")
		}
	})
	if _, err := target.db.ExecContext(ctx, dg5ExplainAnalyzeSQL(target.dialect)); err != nil {
		t.Fatalf("analyze isolated DG5 freshness fixture: %v", err)
	}
	plan, err := dg5FreshnessExplain(ctx, target.db, target.dialect)
	if err != nil {
		t.Fatalf("EXPLAIN DG5 freshness fence: %v", err)
	}
	if strings.TrimSpace(plan) == "" {
		t.Fatal("DG5 freshness EXPLAIN returned no plan")
	}
	t.Logf("DG5 %s freshness fence EXPLAIN (2048 DONE + 1 PENDING isolated rows): %s", target.dialect, plan)
}

func dg5SeedFreshnessExplainRows(ctx context.Context, db *sql.DB, dialect, prefix string) error {
	if err := AssertConnectedDatabase(ctx, db, dialect); err != nil {
		return err
	}
	const total = 2049
	const batchSize = 128
	baseID := time.Now().UTC().UnixNano()
	for start := 0; start < total; start += batchSize {
		end := start + batchSize
		if end > total {
			end = total
		}
		columns := dg5ExplainColumns(dialect)
		values := make([]string, 0, end-start)
		args := make([]any, 0, (end-start)*8)
		placeholder := 1
		for index := start; index < end; index++ {
			status := "DONE"
			if index == total-1 {
				status = "PENDING"
			}
			values = append(values, fmt.Sprintf("(%s, %s, %s, %s, %s, %s, %s, %s, %s, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
				dg5ExplainPlaceholder(dialect, placeholder), dg5ExplainPlaceholder(dialect, placeholder+1), dg5ExplainPlaceholder(dialect, placeholder+2), dg5ExplainPlaceholder(dialect, placeholder+3),
				dg5ExplainPlaceholder(dialect, placeholder+4), dg5ExplainPlaceholder(dialect, placeholder+5), dg5ExplainPlaceholder(dialect, placeholder+6), dg5ExplainPlaceholder(dialect, placeholder+7), dg5ExplainPlaceholder(dialect, placeholder+8)))
			placeholder += 9
			args = append(args,
				baseID+int64(index),
				fmt.Sprintf("%s%d", prefix, index),
				cachepolicy.CacheGovernanceOutboxOwner,
				cachepolicy.StorageScopeSystemGlobal,
				cachepolicy.CacheInvalidationEventType,
				cachepolicy.CacheInvalidationAggregate,
				cachepolicy.ClassTargetDigest(cachepolicy.DataClassConfigPublicScalar),
				`{"fixture":"dg5-explain"}`,
				status,
			)
		}
		query := "INSERT INTO sys_outbox_event " + columns + " VALUES " + strings.Join(values, ",")
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func dg5FreshnessExplain(ctx context.Context, db *sql.DB, dialect string) (string, error) {
	if err := AssertConnectedDatabase(ctx, db, dialect); err != nil {
		return "", err
	}
	query := `EXPLAIN FORMAT=JSON SELECT EXISTS(SELECT 1 FROM sys_outbox_event
WHERE eventOwner=? AND scopeId=? AND eventType=? AND aggregateType=? AND aggregateId=? AND status <> 'DONE')`
	args := []any{
		cachepolicy.CacheGovernanceOutboxOwner,
		cachepolicy.StorageScopeSystemGlobal,
		cachepolicy.CacheInvalidationEventType,
		cachepolicy.CacheInvalidationAggregate,
		cachepolicy.ClassTargetDigest(cachepolicy.DataClassConfigPublicScalar),
	}
	if dialect == "postgres" {
		query = `EXPLAIN (COSTS TRUE, FORMAT TEXT) SELECT EXISTS(SELECT 1 FROM sys_outbox_event
WHERE "eventOwner"=$1 AND "scopeId"=$2 AND "eventType"=$3 AND "aggregateType"=$4 AND "aggregateId"=$5 AND status <> 'DONE')`
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	lines := make([]string, 0, 8)
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return "", err
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(lines, " | "), nil
}

func dg5ExplainColumns(dialect string) string {
	if dialect == "postgres" {
		return `("id", "eventId", "eventOwner", "scopeId", "eventType", "aggregateType", "aggregateId", payload, status, "createTime", "updateTime")`
	}
	return `(id, eventId, eventOwner, scopeId, eventType, aggregateType, aggregateId, payload, status, createTime, updateTime)`
}

func dg5ExplainPlaceholder(dialect string, index int) string {
	if dialect == "postgres" {
		return fmt.Sprintf("$%d", index)
	}
	return "?"
}

func dg5ExplainAnalyzeSQL(dialect string) string {
	if dialect == "postgres" {
		return `ANALYZE "sys_outbox_event"`
	}
	return `ANALYZE TABLE sys_outbox_event`
}

func dg5ExplainCleanupSQL(dialect string) string {
	if dialect == "postgres" {
		return `DELETE FROM sys_outbox_event WHERE "eventId" LIKE $1`
	}
	return `DELETE FROM sys_outbox_event WHERE eventId LIKE ?`
}
