package imageadmission

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	runtimeconfig "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

var harnessAliasPart = regexp.MustCompile(`[^a-z0-9]+`)

// AdoptReportedHarness promotes exact Worker-local admission evidence into an
// immutable Harness release. The Server never inspects Docker itself: only a
// fresh current Worker session may supply evidence for the configured image.
func AdoptReportedHarness(ctx context.Context, db *pgxpool.Pool, cfg runtimeconfig.HarnessConfig, idGen IDGenerator) (bool, error) {
	return adoptReportedHarnessRef(ctx, db, cfg, cfg.Image, cfg.Alias, idGen)
}

// AdoptReportedHarnessFleet exposes every currently admitted protocol image as
// an immutable, digest-addressed Harness channel. The configured channel keeps
// its operator-friendly alias; additional images get a deterministic alias so
// Tasks can freeze and route an exact heterogeneous-fleet requirement.
func AdoptReportedHarnessFleet(ctx context.Context, db *pgxpool.Pool, cfg runtimeconfig.HarnessConfig, idGen IDGenerator) (int, error) {
	rows, err := db.Query(ctx, `SELECT DISTINCT ON (wi.image_digest) wi.image_ref,wi.image_digest,wi.manifest_json::text
		FROM workforce_worker_images wi
		JOIN workforce_workers w ON w.id=wi.worker_id AND w.current_instance_id=wi.instance_id
		JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
		WHERE wi.role='attempt' AND wi.contract_version=$1
		  AND wi.admission_status='ADMITTED' AND wi.presence_status='PRESENT'
		  AND s.ended_at IS NULL AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds'
		ORDER BY wi.image_digest,wi.last_seen_at DESC`, cfg.Contract)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type candidate struct{ imageRef, digest, manifest string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.imageRef, &item.digest, &item.manifest); err != nil {
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	adopted := 0
	for _, item := range candidates {
		alias := cfg.Alias
		if item.imageRef != cfg.Image {
			alias, err = fleetHarnessAlias(item.manifest, item.digest)
			if err != nil {
				return adopted, err
			}
		}
		ok, err := adoptReportedHarnessRef(ctx, db, cfg, item.imageRef, alias, idGen)
		if err != nil {
			return adopted, err
		}
		if ok {
			adopted++
		}
	}
	return adopted, nil
}

func adoptReportedHarnessRef(ctx context.Context, db *pgxpool.Pool, cfg runtimeconfig.HarnessConfig, imageRef, alias string, idGen IDGenerator) (bool, error) {
	if idGen == nil {
		return false, errors.New("workforce ID generator is not configured")
	}
	var workerID int64
	var digest, contract, manifestBody, evidenceBody string
	err := db.QueryRow(ctx, `SELECT wi.worker_id,wi.image_digest,wi.contract_version,wi.manifest_json::text,wi.admission_evidence_json::text
		FROM workforce_worker_images wi
		JOIN workforce_workers w ON w.id=wi.worker_id AND w.current_instance_id=wi.instance_id
		JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
		WHERE wi.image_ref=$1 AND wi.role='attempt' AND wi.admission_status='ADMITTED' AND wi.presence_status='PRESENT'
		  AND s.ended_at IS NULL AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds'
		ORDER BY wi.last_seen_at DESC LIMIT 1`, imageRef).Scan(&workerID, &digest, &contract, &manifestBody, &evidenceBody)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var manifest struct {
		HarnessType       string         `json:"harnessType"`
		HarnessVersion    string         `json:"harnessVersion"`
		SupervisorVersion string         `json:"supervisorVersion"`
		Probe             map[string]any `json:"probe"`
		Scheduling        struct {
			Scope                   string   `json:"scope"`
			Priority                int      `json:"priority"`
			EnvironmentCapabilities []string `json:"environmentCapabilities"`
			ProviderProtocols       []string `json:"providerProtocols"`
			HookEvents              []string `json:"hookEvents"`
		} `json:"scheduling"`
		ExecutionKinds   []string `json:"executionKinds"`
		SchedulingDigest string   `json:"schedulingDigest"`
	}
	if err := json.Unmarshal([]byte(manifestBody), &manifest); err != nil || manifest.HarnessType == "" || manifest.HarnessVersion == "" || manifest.SupervisorVersion == "" || contract != cfg.Contract {
		return false, errors.New("reported Harness admission evidence is invalid")
	}
	var workerEvidence any
	if json.Unmarshal([]byte(evidenceBody), &workerEvidence) != nil {
		return false, errors.New("reported Harness admission evidence is malformed")
	}
	evidence, _ := json.Marshal(map[string]any{
		"source": "WORKER_LOCAL_ADMISSION", "workerId": workerID,
		"adoptedAt": time.Now().UTC().Format(time.RFC3339Nano), "probe": manifest.Probe,
		"workerEvidence": workerEvidence, "localOnly": cfg.AllowLocalImage,
	})
	declared, err := probeCapabilities(manifest.Probe)
	if err != nil {
		return false, err
	}
	scheduling, err := normalizeScheduling(manifest.Scheduling.Scope, manifest.Scheduling.Priority, manifest.Scheduling.ProviderProtocols, manifest.Scheduling.EnvironmentCapabilities, manifest.Scheduling.HookEvents, manifest.ExecutionKinds)
	if err != nil || scheduling.Digest != manifest.SchedulingDigest {
		return false, errors.New("reported Harness scheduling contract is invalid")
	}
	capabilities, _ := json.Marshal(declared)
	providerProtocols, _ := json.Marshal(scheduling.ProviderProtocols)
	environmentCapabilities, _ := json.Marshal(scheduling.EnvironmentCapabilities)
	hookEvents, _ := json.Marshal(scheduling.HookEvents)
	executionKinds, _ := json.Marshal(scheduling.ExecutionKinds)
	tx, err := db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	releaseID := idGen.NextID()
	if err = tx.QueryRow(ctx, `INSERT INTO workforce_harness_releases (id,alias,image_ref,image_digest,contract_version,supervisor_version,harness_type,harness_version,capabilities_json,scheduling_scope,scheduling_priority,provider_protocols_json,environment_capabilities_json,hook_events_json,execution_kinds_json,scheduling_digest,manifest_json,admission_evidence_json,status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,'ACTIVE')
		ON CONFLICT (image_digest) DO UPDATE SET image_ref=EXCLUDED.image_ref,contract_version=EXCLUDED.contract_version,supervisor_version=EXCLUDED.supervisor_version,harness_type=EXCLUDED.harness_type,harness_version=EXCLUDED.harness_version,capabilities_json=EXCLUDED.capabilities_json,scheduling_scope=EXCLUDED.scheduling_scope,scheduling_priority=EXCLUDED.scheduling_priority,provider_protocols_json=EXCLUDED.provider_protocols_json,environment_capabilities_json=EXCLUDED.environment_capabilities_json,hook_events_json=EXCLUDED.hook_events_json,execution_kinds_json=EXCLUDED.execution_kinds_json,scheduling_digest=EXCLUDED.scheduling_digest,manifest_json=EXCLUDED.manifest_json,admission_evidence_json=EXCLUDED.admission_evidence_json,status='ACTIVE',updated_at=CURRENT_TIMESTAMP
		RETURNING id`, releaseID, alias, imageRef, digest, contract, manifest.SupervisorVersion, manifest.HarnessType, manifest.HarnessVersion, capabilities, scheduling.Scope, scheduling.Priority, providerProtocols, environmentCapabilities, hookEvents, executionKinds, scheduling.Digest, json.RawMessage(manifestBody), evidence).Scan(&releaseID); err != nil {
		return false, err
	}
	principalID, err := principalinfra.EnsureWorker(ctx, tx, idGen, workerID)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_harness_channels (alias,harness_release_id,updated_by_principal_id) VALUES ($1,$2,$3)
		ON CONFLICT(alias) DO UPDATE SET harness_release_id=EXCLUDED.harness_release_id,updated_by_principal_id=EXCLUDED.updated_by_principal_id,updated_at=CURRENT_TIMESTAMP`, alias, releaseID, principalID); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func fleetHarnessAlias(manifestBody, digest string) (string, error) {
	var manifest struct {
		HarnessType    string `json:"harnessType"`
		HarnessVersion string `json:"harnessVersion"`
	}
	if err := json.Unmarshal([]byte(manifestBody), &manifest); err != nil || manifest.HarnessType == "" || manifest.HarnessVersion == "" {
		return "", errors.New("reported Harness manifest cannot produce a fleet alias")
	}
	digest = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(digest)), "sha256:")
	if len(digest) < 12 {
		return "", fmt.Errorf("reported Harness digest is too short")
	}
	part := func(value string) string {
		return strings.Trim(harnessAliasPart.ReplaceAllString(strings.ToLower(value), "-"), "-")
	}
	return fmt.Sprintf("fleet-%s-%s-%s", part(manifest.HarnessType), part(manifest.HarnessVersion), digest[:12]), nil
}

func RunWorkerEvidenceAdopter(ctx context.Context, db *pgxpool.Pool, cfg runtimeconfig.HarnessConfig, idGen IDGenerator) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_, _ = AdoptReportedHarnessFleet(ctx, db, cfg, idGen)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
