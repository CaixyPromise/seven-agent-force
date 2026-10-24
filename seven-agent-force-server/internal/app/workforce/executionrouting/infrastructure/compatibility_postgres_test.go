package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	"github.com/jackc/pgx/v5/pgxpool"
)

type requirementsCompilerStub struct {
	requirements projectfacade.ExecutionRequirements
}

func (s requirementsCompilerStub) CompileExecutionRequirements(context.Context, int64, int64) (projectfacade.CompiledExecutionRequirements, error) {
	return projectfacade.CompiledExecutionRequirements{ProjectVersionID: 2, Requirements: s.requirements}, nil
}

func TestPostgresRuntimeCompatibilityEquivalence(t *testing.T) {
	dsn := os.Getenv("RUNTIME_COMPATIBILITY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set RUNTIME_COMPATIBILITY_TEST_POSTGRES_DSN to isolated test database")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "seven_workforce_consistency_test_20260929" {
		t.Fatal("requires authorized isolated consistency database")
	}
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("runtime_compatibility_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, err = pool.Exec(ctx, `
 CREATE TABLE workforce_projects (id bigint, tenant_id bigint, current_version_id bigint, status text);
 CREATE TABLE workforce_model_credentials (id bigint, tenant_id bigint, protocol text, status text);
 CREATE TABLE workforce_project_versions (id bigint, tenant_id bigint, hook_release_ids bigint[]);
 CREATE TABLE workforce_hook_package_releases (id bigint, tenant_id bigint, scope_type text, manifest_json jsonb);
 CREATE TABLE workforce_harness_releases (id bigint, alias text, harness_type text, harness_version text, image_ref text, image_digest text, contract_version text, scheduling_priority integer, provider_protocols_json jsonb, environment_capabilities_json jsonb, hook_events_json jsonb, execution_kinds_json jsonb, manifest_json jsonb, status text, scheduling_scope text);
 CREATE TABLE workforce_workers (id bigint, name text, current_instance_id text, max_active_attempts bigint, desired_state text, decommissioned_at timestamptz);
 CREATE TABLE workforce_worker_sessions (instance_id text, max_active_attempts bigint, ended_at timestamptz, last_heartbeat_at timestamptz, consumer_state text, docker_ready boolean, resource_snapshot_json jsonb);
 CREATE TABLE workforce_worker_images (worker_id bigint, instance_id text, image_digest text, contract_version text, scheduling_priority integer, role text, admission_status text, presence_status text, scheduling_scope text, execution_kinds_json jsonb);
 CREATE TABLE workforce_tasks (id bigint, selected_runtime_release_id bigint);
 CREATE TABLE workforce_execution_nodes (task_id bigint, status text);
 CREATE TABLE workforce_task_assignments (id bigint, worker_id bigint, status text, created_at timestamptz);
 CREATE TABLE workforce_task_attempts (id bigint, assignment_id bigint);
 CREATE TABLE workforce_attempt_runtimes (attempt_id bigint, assignment_id bigint, status text);
 INSERT INTO workforce_projects VALUES (1,100,2,'ACTIVE');
 INSERT INTO workforce_model_credentials VALUES (3,100,'OPENAI','ACTIVE');
 INSERT INTO workforce_project_versions VALUES (2,100,ARRAY[4]::bigint[]);
 INSERT INTO workforce_hook_package_releases VALUES (4,100,'TENANT','{"entrypoints":[{"event":"before_run"}]}');
 INSERT INTO workforce_workers VALUES (5,'test','session',4,'ENABLED',NULL);
 INSERT INTO workforce_worker_sessions VALUES ('session',4,NULL,now(),'CONSUMING',true,'{}');`)
	if err != nil {
		t.Fatal(err)
	}
	image := "sha256:" + strings.Repeat("a", 64)
	_, err = pool.Exec(ctx, `INSERT INTO workforce_harness_releases VALUES (6,'test','test','1',$1,$1,'seven.worker-image/v1',1,'["OPENAI"]','[]','["before_run"]','["AGENT_ATTEMPT"]','{}','ACTIVE','PRODUCTION')`, image)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO workforce_worker_images VALUES (5,'session',$1,'seven.worker-image/v1',1,'attempt','ADMITTED','PRESENT','PRODUCTION','["AGENT_ATTEMPT"]')`, image)
	if err != nil {
		t.Fatal(err)
	}
	requirements := compiledProjectRequirements(t)
	catalog := NewRuntimeCatalog(pool, requirementsCompilerStub{requirements})
	resolver := NewResolver(routingfacade.PlatformIdentity{})
	t.Run("newer_recommendation_preserves_frozen_selection", func(t *testing.T) {
		raw, _ := json.Marshal(compatibleManifest())
		if _, err := pool.Exec(ctx, `UPDATE workforce_harness_releases SET manifest_json=$1`, raw); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO workforce_harness_releases SELECT 7,alias,harness_type,harness_version,image_ref,image_digest,contract_version,scheduling_priority,provider_protocols_json,environment_capabilities_json,hook_events_json,execution_kinds_json,manifest_json,status,scheduling_scope FROM workforce_harness_releases WHERE id=6`); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(ctx, `DELETE FROM workforce_harness_releases WHERE id=7`)
		options, err := catalog.ListTaskOptions(ctx, 100, 1, 3)
		if err != nil || len(options.Items) != 2 || options.Items[0].RuntimeReleaseID != 7 || !options.Items[0].Recommended || options.Items[1].RuntimeReleaseID != 6 || options.Items[1].Recommended {
			t.Fatalf("recommendation=%#v err=%v", options, err)
		}
		for _, id := range []int64{6, 7} {
			if err := catalog.ValidateTaskSelection(ctx, 100, 2, id, "OPENAI", requirements); err != nil {
				t.Fatalf("explicit release %d rejected: %v", id, err)
			}
		}
	})
	for _, tc := range []struct {
		name   string
		mutate func(*runtimeManifest)
		sql    string
		want   bool
	}{
		{name: "compatible", want: true},
		{name: "missing_adapter_capability", mutate: func(m *runtimeManifest) { m.Adapter.Capabilities = nil }},
		{name: "missing_interaction_request", mutate: func(m *runtimeManifest) {
			m.Adapter.Capabilities = slices.DeleteFunc(m.Adapter.Capabilities, func(value string) bool { return value == "interaction.request" })
		}},
		{name: "missing_interaction_respond", mutate: func(m *runtimeManifest) {
			m.Adapter.Capabilities = slices.DeleteFunc(m.Adapter.Capabilities, func(value string) bool { return value == "interaction.respond" })
		}},
		{name: "missing_tool_request", mutate: func(m *runtimeManifest) {
			m.Adapter.Capabilities = slices.DeleteFunc(m.Adapter.Capabilities, func(value string) bool { return value == "tool.request" })
		}},
		{name: "missing_tool_result", mutate: func(m *runtimeManifest) {
			m.Adapter.Capabilities = slices.DeleteFunc(m.Adapter.Capabilities, func(value string) bool { return value == "tool.result" })
		}},
		{name: "invalid_manifest", mutate: func(m *runtimeManifest) { m.Adapter.ID = "" }},
		{name: "nonhex_manifest_digest", mutate: func(m *runtimeManifest) { m.ResultProcessor.SDKDigest = strings.Repeat("z", 64) }},
		{name: "invalid_image_digest", sql: `UPDATE workforce_harness_releases SET image_digest='sha256:invalid'`},
		{name: "wrong_protocol", sql: `UPDATE workforce_harness_releases SET provider_protocols_json='["ANTHROPIC"]'`},
		{name: "missing_hook", sql: `UPDATE workforce_harness_releases SET hook_events_json='[]'`},
		{name: "inactive", sql: `UPDATE workforce_harness_releases SET status='DISABLED'`},
		{name: "nonproduction", sql: `UPDATE workforce_harness_releases SET scheduling_scope='TEST'`},
		{name: "wrong_kind", sql: `UPDATE workforce_harness_releases SET execution_kinds_json='["REVIEW_RUN"]'`},
		{name: "wrong_contract", sql: `UPDATE workforce_harness_releases SET contract_version='seven.worker-image/v2'`},
		{name: "malformed_manifest_shape", sql: `UPDATE workforce_harness_releases SET manifest_json='[]'`},
		{name: "malformed_protocols", sql: `UPDATE workforce_harness_releases SET provider_protocols_json='{}'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := compatibleManifest()
			if tc.mutate != nil {
				tc.mutate(&m)
			}
			raw, _ := json.Marshal(m)
			if _, err := pool.Exec(ctx, `UPDATE workforce_harness_releases SET image_digest=$1,status='ACTIVE',scheduling_scope='PRODUCTION',execution_kinds_json='["AGENT_ATTEMPT"]',provider_protocols_json='["OPENAI"]',hook_events_json='["before_run"]',contract_version='seven.worker-image/v1',manifest_json=$2`, image, raw); err != nil {
				t.Fatal(err)
			}
			if tc.sql != "" {
				if _, err := pool.Exec(ctx, tc.sql); err != nil {
					t.Fatal(err)
				}
			}
			options, err := catalog.ListTaskOptions(ctx, 100, 1, 3)
			if err != nil {
				t.Fatal(err)
			}
			validation := catalog.ValidateTaskSelection(ctx, 100, 2, 6, "OPENAI", requirements)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			transactionalValidation := catalog.ValidateTaskSelectionInTransaction(ctx, tx, 100, 2, 6, "OPENAI", requirements)
			_, wait, err := resolver.selectRuntime(ctx, tx, 6, requirements, []string{"before_run"}, "OPENAI")
			if err != nil {
				t.Fatal(err)
			}
			if listed, accepted, txAccepted, resolved := len(options.Items) == 1, validation == nil, transactionalValidation == nil, wait == ""; listed != tc.want || accepted != tc.want || txAccepted != tc.want || resolved != tc.want {
				t.Fatalf("listed=%v createAccepted=%v txAccepted=%v resolved=%v want=%v (validation=%v wait=%s)", listed, accepted, txAccepted, resolved, tc.want, validation, wait)
			}
		})
	}

	t.Run("source_codex_claude_equivalence", func(t *testing.T) {
		for _, tc := range []struct{ name, protocol string }{{"codex", "OPENAI"}, {"claude", "ANTHROPIC"}} {
			t.Run(tc.name, func(t *testing.T) {
				release := sourceCatalogRelease(t, tc.name)
				manifest, _ := json.Marshal(release.manifest)
				protocols, _ := json.Marshal(release.protocols)
				hooks, _ := json.Marshal(release.hookEvents)
				environment, _ := json.Marshal(release.environment)
				kinds, _ := json.Marshal(release.executionKinds)
				if _, err := pool.Exec(ctx, `UPDATE workforce_harness_releases SET provider_protocols_json=$1,hook_events_json=$2,environment_capabilities_json=$3,execution_kinds_json=$4,manifest_json=$5`, protocols, hooks, environment, kinds, manifest); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, `UPDATE workforce_model_credentials SET protocol=$1`, tc.protocol); err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, `UPDATE workforce_hook_package_releases SET manifest_json='{"entrypoints":[{"event":"attempt.prepare"}]}'`); err != nil {
					t.Fatal(err)
				}
				options, err := catalog.ListTaskOptions(ctx, 100, 1, 3)
				if err != nil || len(options.Items) != 1 || options.Items[0].RuntimeReleaseID != 6 {
					t.Fatalf("source candidates=%#v err=%v", options, err)
				}
				if err := catalog.ValidateTaskSelection(ctx, 100, 2, 6, tc.protocol, requirements); err != nil {
					t.Fatal(err)
				}
				tx, err := pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
				if err := catalog.ValidateTaskSelectionInTransaction(ctx, tx, 100, 2, 6, tc.protocol, requirements); err != nil {
					t.Fatal(err)
				}
				selected, wait, err := resolver.selectRuntime(ctx, tx, 6, requirements, []string{"attempt.prepare"}, tc.protocol)
				if err != nil || wait != "" || selected.releaseID != 6 || selected.manifest.Adapter.ID != release.manifest.Adapter.ID {
					t.Fatalf("source selected=%#v wait=%s err=%v", selected, wait, err)
				}
			})
		}
	})
	// Restore the shared fixture after source-manifest coverage.
	if _, err := pool.Exec(ctx, `UPDATE workforce_harness_releases SET provider_protocols_json='["OPENAI"]',hook_events_json='["before_run"]'; UPDATE workforce_model_credentials SET protocol='OPENAI'; UPDATE workforce_hook_package_releases SET manifest_json='{"entrypoints":[{"event":"before_run"}]}'`); err != nil {
		t.Fatal(err)
	}

	t.Run("full_selected_runtime_never_uses_idle_different_image", func(t *testing.T) {
		manifest, _ := json.Marshal(compatibleManifest())
		if _, err := pool.Exec(ctx, `UPDATE workforce_harness_releases SET manifest_json=$1`, manifest); err != nil {
			t.Fatal(err)
		}
		otherImage := "sha256:" + strings.Repeat("b", 64)
		// The alternate release has better priority and four free slots, but it
		// must not replace the user's selected release when that release is full.
		if _, err := pool.Exec(ctx, `INSERT INTO workforce_harness_releases SELECT 8,'idle-other',harness_type,harness_version,$1,$1,contract_version,0,provider_protocols_json,environment_capabilities_json,hook_events_json,execution_kinds_json,manifest_json,status,scheduling_scope FROM workforce_harness_releases WHERE id=6`, otherImage); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO workforce_workers VALUES (9,'idle-other','other-session',4,'ENABLED',NULL); INSERT INTO workforce_worker_sessions VALUES ('other-session',4,NULL,now(),'CONSUMING',true,'{}'); INSERT INTO workforce_task_assignments SELECT 100+n,5,'RESERVED',now() FROM generate_series(1,4) n`); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO workforce_worker_images VALUES (9,'other-session',$1,'seven.worker-image/v1',0,'attempt','ADMITTED','PRESENT','PRODUCTION','["AGENT_ATTEMPT"]')`, otherImage); err != nil {
			t.Fatal(err)
		}
		options, err := catalog.ListTaskOptions(ctx, 100, 1, 3)
		if err != nil {
			t.Fatal(err)
		}
		slots := map[int64]int{}
		for _, option := range options.Items {
			slots[option.RuntimeReleaseID] = option.AvailableSlots
		}
		if len(slots) != 2 || slots[6] != 0 || slots[8] != 4 {
			t.Fatalf("candidate slots=%v want selected=0 alternate=4", slots)
		}
		if err := catalog.ValidateTaskSelection(ctx, 100, 2, 6, "OPENAI", requirements); err != nil {
			t.Fatalf("full runtime must remain compatible: %v", err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if err := catalog.ValidateTaskSelectionInTransaction(ctx, tx, 100, 2, 6, "OPENAI", requirements); err != nil {
			t.Fatal(err)
		}
		selected, wait, err := resolver.selectRuntime(ctx, tx, 6, requirements, []string{"before_run"}, "OPENAI")
		if err != nil || wait != routingfacade.WaitWorkerCapacity || selected.releaseID != 0 {
			t.Fatalf("full selected release=%d wait=%s err=%v", selected.releaseID, wait, err)
		}
		alternate, wait, err := resolver.selectRuntime(ctx, tx, 8, requirements, []string{"before_run"}, "OPENAI")
		if err != nil || wait != "" || alternate.releaseID != 8 || alternate.workerID != 9 || alternate.imageDigest != otherImage {
			t.Fatalf("idle alternate=%#v wait=%s err=%v", alternate, wait, err)
		}
		// With no current selected-image inventory, the idle different image also
		// cannot replace it; the wait reason changes from capacity to availability.
		if _, err := tx.Exec(ctx, `DELETE FROM workforce_worker_images WHERE worker_id=5`); err != nil {
			t.Fatal(err)
		}
		selected, wait, err = resolver.selectRuntime(ctx, tx, 6, requirements, []string{"before_run"}, "OPENAI")
		if err != nil || wait != routingfacade.WaitSelectedRuntime || selected.releaseID != 0 {
			t.Fatalf("missing selected image release=%d wait=%s err=%v", selected.releaseID, wait, err)
		}
	})

	t.Run("transaction_reads_its_release_and_hook_changes", func(t *testing.T) {
		raw, _ := json.Marshal(compatibleManifest())
		if _, err := pool.Exec(ctx, `UPDATE workforce_harness_releases SET provider_protocols_json='["OPENAI"]',manifest_json=$1`, raw); err != nil {
			t.Fatal(err)
		}
		for _, sql := range []string{
			`UPDATE workforce_harness_releases SET manifest_json=jsonb_set(manifest_json,'{adapter,capabilities}','[]')`,
			`UPDATE workforce_project_versions SET hook_release_ids=ARRAY[99]::bigint[]`,
		} {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			func() {
				defer tx.Rollback(ctx)
				if _, err := tx.Exec(ctx, sql); err != nil {
					t.Fatal(err)
				}
				// No pool or compiler is available here: all final checks must use tx.
				if err := (&RuntimeCatalog{}).ValidateTaskSelectionInTransaction(ctx, tx, 100, 2, 6, "OPENAI", requirements); err != routingfacade.ErrRuntimeSelectionIncompatible {
					t.Fatalf("transaction ignored its own changes: %v", err)
				}
				if err := catalog.ValidateTaskSelection(ctx, 100, 2, 6, "OPENAI", requirements); err != nil {
					t.Fatalf("outside quick path must still see committed compatible data: %v", err)
				}
			}()
		}
	})
}
