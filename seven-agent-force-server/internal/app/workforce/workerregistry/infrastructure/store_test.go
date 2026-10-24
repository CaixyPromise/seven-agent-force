package infrastructure

import (
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
)

func TestRuntimeIdentityQueriesUseAttemptResolvedSnapshot(t *testing.T) {
	for name, query := range map[string]string{
		"token rotation": activeProviderRuntimeCountSQL,
		"recovery":       recoverableRuntimesSQL,
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(query, "a.resolved_execution_snapshot_json->>'modelCredentialId'") || !strings.Contains(query, "::BIGINT,0)>0") {
				t.Fatal("provider identity is not read from the immutable Attempt snapshot")
			}
			for _, removed := range []string{"t.execution_snapshot_json", "t.provider_alias", "t.provider_model", "t.harness_timeout_seconds"} {
				if strings.Contains(query, removed) {
					t.Fatalf("query still reads removed Task field %s", removed)
				}
			}
		})
	}
}

func TestWorkerImagePersistenceIncludesSchedulingIdentity(t *testing.T) {
	for _, field := range []string{"scheduling_scope", "scheduling_priority", "environment_capabilities_json", "execution_kinds_json", "scheduling_digest"} {
		if !strings.Contains(publishInventoryImageSQL, field) {
			t.Fatalf("inventory upsert omits %s", field)
		}
		if !strings.Contains(workerImagesSelectSQL, field) {
			t.Fatalf("inventory readback omits %s", field)
		}
	}
}

func TestMetricQueriesPreserveWeightedFleetAndPeakSlots(t *testing.T) {
	for _, token := range []string{"SUM(cpu_usage_percent*logical_cpu_cores)", "SUM(memory_used_bytes)", "SUM(memory_total_bytes)", "SUM(workspace_disk_used_bytes)", "MAX(active_attempts)", "MAX(max_active_attempts)"} {
		if !strings.Contains(fleetMetricSeriesSQL, token) {
			t.Fatalf("fleet metric query omits %s", token)
		}
	}
	if !strings.Contains(singleWorkerMetricSeriesSQL, "AVG(cpu_usage_percent)") || !strings.Contains(singleWorkerMetricSeriesSQL, "MAX(active_attempts)") {
		t.Fatal("single Worker metric query lost average resource or peak slot aggregation")
	}
}

func TestSameStringArrayJSONIgnoresJSONBWhitespaceButNotOrder(t *testing.T) {
	if !sameStringArrayJSON([]byte(`["language.go","shell.posix"]`), []byte(`["language.go", "shell.posix"]`)) {
		t.Fatal("equivalent JSONB array rejected")
	}
	if sameStringArrayJSON([]byte(`["language.go","shell.posix"]`), []byte(`["shell.posix","language.go"]`)) {
		t.Fatal("non-canonical scheduling order accepted")
	}
}

func TestDeriveStateDistinguishesDrainingAndDrained(t *testing.T) {
	worker := domain.Worker{DesiredState: domain.DesiredDraining, CurrentInstanceID: "wki_1", ConsumerState: "STOPPED", MaxActiveAttempts: 2}
	worker.RunningAttempts = 1
	if got := deriveState(worker, false, true, true); got != domain.StateDraining {
		t.Fatalf("active Worker must remain DRAINING, got %s", got)
	}
	worker.RunningAttempts = 0
	if got := deriveState(worker, false, true, true); got != domain.StateDrained {
		t.Fatalf("empty stopped Worker must become DRAINED, got %s", got)
	}
}

func TestDeriveStateMakesDecommissionTerminal(t *testing.T) {
	worker := domain.Worker{DesiredState: domain.DesiredDisabled, DecommissionedAt: "2026-08-26T00:00:00Z"}
	if got := deriveState(worker, true, false, false); got != domain.StateDecommissioned {
		t.Fatalf("decommission must dominate all transient state, got %s", got)
	}
}

func TestWorkerCanDisableOnlyWhenDrainedOrOffline(t *testing.T) {
	tests := []struct {
		name           string
		desired        string
		instance       string
		consumer       string
		heartbeatFresh bool
		active         int
		want           bool
	}{
		{name: "drained", desired: domain.DesiredDraining, instance: "wki_1", consumer: "STOPPED", heartbeatFresh: true, want: true},
		{name: "offline", desired: domain.DesiredEnabled, instance: "wki_1", consumer: "CONSUMING", heartbeatFresh: false, want: true},
		{name: "never registered", desired: domain.DesiredEnabled, want: true},
		{name: "online", desired: domain.DesiredEnabled, instance: "wki_1", consumer: "CONSUMING", heartbeatFresh: true, want: false},
		{name: "drain not acknowledged", desired: domain.DesiredDraining, instance: "wki_1", consumer: "CONSUMING", heartbeatFresh: true, want: false},
		{name: "still active", desired: domain.DesiredDraining, instance: "wki_1", consumer: "STOPPED", heartbeatFresh: true, active: 1, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := workerCanDisable(test.desired, test.instance, test.consumer, test.heartbeatFresh, test.active); got != test.want {
				t.Fatalf("workerCanDisable() = %t, want %t", got, test.want)
			}
		})
	}
}
