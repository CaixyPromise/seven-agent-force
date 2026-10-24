package schemaalignment

import (
	dispatchinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/infrastructure"
	"os"
	"strings"
	"testing"
)

func TestFactProducerMigrationAlignmentContract(t *testing.T) {
	migration, err := os.ReadFile("../../../../migrations/postgres/20260825130000_workforce_worker_fleet_mvp.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{string(migration), dispatchinfra.FactProducerSchemaSQL} {
		for _, fragment := range []string{"producer_source", "'WORKER'", "'SUPERVISOR_JOURNAL'", "workforce_attempt_facts_source_sequence_uidx", "attempt_id,producer_source,producer_sequence"} {
			if !strings.Contains(source, fragment) {
				t.Fatalf("missing producer contract %s", fragment)
			}
		}
	}
	if !strings.Contains(dispatchinfra.FactProducerSchemaSQL, "DROP CONSTRAINT IF EXISTS workforce_attempt_facts_attempt_id_producer_sequence_key") {
		t.Fatal("legacy sequence collision not aligned")
	}
	registry, err := os.ReadFile("../../../../migrations/governance/future_table_registry.csv")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(registry), "producer_source is fixed by authenticated Machine ingress") {
		t.Fatal("producer governance missing")
	}
}
