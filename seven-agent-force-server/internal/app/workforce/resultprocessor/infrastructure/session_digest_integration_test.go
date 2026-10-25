package infrastructure

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Uses a temporary table which shadows the real relation. No business rows are
// inserted or changed, and the whole transaction is rolled back.
func TestPersistSessionDigestPostgresColumnTypes(t *testing.T) {
	dsn := os.Getenv("RESULT_PROCESSOR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires an isolated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, columnType := range []string{"VARCHAR(64)", "TEXT"} {
		t.Run(columnType, func(t *testing.T) {
			db, err := pgx.Connect(ctx, dsn)
			if err != nil {
				t.Fatal("test database connection failed")
			}
			defer db.Close(ctx)
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			if _, err = tx.Exec(ctx, "CREATE TEMP TABLE workforce_task_turns (id BIGINT PRIMARY KEY, session_digest "+columnType+" NOT NULL DEFAULT '') ON COMMIT DROP"); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, "INSERT INTO workforce_task_turns(id) VALUES (1)"); err != nil {
				t.Fatal(err)
			}
			first := strings.Repeat("a", 64)
			for _, incoming := range []string{first, first, strings.Repeat("b", 64)} {
				if _, err = tx.Exec(ctx, persistSessionDigestSQL, int64(1), incoming); err != nil {
					t.Fatal(err)
				}
				var actual string
				if err = tx.QueryRow(ctx, "SELECT session_digest FROM workforce_task_turns WHERE id=1").Scan(&actual); err != nil {
					t.Fatal(err)
				}
				if actual != first {
					t.Fatal("existing session digest was changed")
				}
			}
		})
	}
}
