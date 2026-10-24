package schemaalignment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerMetricMigrationUsesCompactNaturalKeySchema(t *testing.T) {
	body := readProjectFirstMigration(t, projectFirstWorkerMigration)
	up, down := migrationUpAndDown(t, body)
	for _, token := range []string{
		"CREATE TABLE WORKFORCE_WORKER_METRIC_SAMPLES",
		"WORKER_ID BIGINT NOT NULL",
		"SAMPLED_AT TIMESTAMPTZ NOT NULL",
		"LOGICAL_CPU_CORES SMALLINT NOT NULL",
		"CPU_USAGE_PERCENT REAL",
		"ACTIVE_ATTEMPTS SMALLINT NOT NULL",
		"PRIMARY KEY (WORKER_ID, SAMPLED_AT)",
		"USING BRIN (SAMPLED_AT)",
		"COMMENT ON TABLE WORKFORCE_WORKER_METRIC_SAMPLES",
	} {
		if !strings.Contains(up, token) {
			t.Errorf("Worker metric migration is missing %q", token)
		}
	}
	if strings.Contains(up, "WORKFORCE_WORKER_METRIC_SAMPLES (\n    ID ") {
		t.Fatal("Worker metric samples must not have a synthetic ID")
	}
	if !strings.Contains(down, "DROP TABLE IF EXISTS WORKFORCE_WORKER_METRIC_SAMPLES") {
		t.Fatal("Worker metric Down contract is missing")
	}
}

func TestWorkerMetricAlignmentIsIdempotent(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("worker_metrics.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToUpper(string(body))
	for _, token := range []string{"CREATE TABLE IF NOT EXISTS WORKFORCE_WORKER_METRIC_SAMPLES", "CREATE INDEX IF NOT EXISTS WORKFORCE_WORKER_METRIC_SAMPLES_SAMPLED_AT_BRIN", "VERIFYWORKERMETRICSCHEMA"} {
		if !strings.Contains(text, token) {
			t.Errorf("Worker metric alignment is missing %q", token)
		}
	}
}
