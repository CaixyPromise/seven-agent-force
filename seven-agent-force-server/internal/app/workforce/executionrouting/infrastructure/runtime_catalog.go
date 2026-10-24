package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
)

type RuntimeCatalog struct {
	db       *pgxpool.Pool
	projects projectfacade.RequirementsCompiler
}

func NewRuntimeCatalog(db *pgxpool.Pool, projects projectfacade.RequirementsCompiler) *RuntimeCatalog {
	return &RuntimeCatalog{db: db, projects: projects}
}

type catalogRelease struct {
	id, projectVersionID                                           int64
	name, description, harnessType, harnessVersion, adapterVersion string
	imageRef, imageDigest                                          string
	priority                                                       int
	protocols, environment, hookEvents, executionKinds             []string
	status, scope, contract                                        string
	manifest                                                       runtimeManifest
}

type runtimeWorkerInventory struct {
	id, maxAttempts             int64
	name, digest                string
	reserved, starting, running int
}

func (c *RuntimeCatalog) ValidateTaskSelection(ctx context.Context, tenantID, projectVersionID, releaseID int64, protocol string, requirements projectfacade.ExecutionRequirements) error {
	if c == nil || c.db == nil || releaseID < 1 {
		return routingfacade.ErrRuntimeSelectionRequired
	}
	return c.validateTaskSelection(ctx, c.db, tenantID, projectVersionID, releaseID, protocol, requirements)
}

func (c *RuntimeCatalog) ValidateTaskSelectionInTransaction(ctx context.Context, tx pgx.Tx, tenantID, projectVersionID, releaseID int64, protocol string, requirements projectfacade.ExecutionRequirements) error {
	if c == nil || tx == nil || releaseID < 1 {
		return routingfacade.ErrRuntimeSelectionRequired
	}
	return c.validateTaskSelection(ctx, tx, tenantID, projectVersionID, releaseID, protocol, requirements)
}

type catalogQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func (c *RuntimeCatalog) validateTaskSelection(ctx context.Context, db catalogQuerier, tenantID, projectVersionID, releaseID int64, protocol string, requirements projectfacade.ExecutionRequirements) error {
	release, err := loadCatalogRelease(ctx, db, releaseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return routingfacade.ErrRuntimeSelectionUnavailable
	}
	if err != nil {
		return err
	}
	required, err := projectHookEvents(ctx, db, tenantID, projectVersionID)
	if err != nil {
		return err
	}
	if !compatibleTaskRuntime(release, requirements, required, protocol) {
		return routingfacade.ErrRuntimeSelectionIncompatible
	}
	return nil
}

func (c *RuntimeCatalog) ListTaskOptions(ctx context.Context, tenantID, projectID, modelCredentialID int64) (routingfacade.RuntimeOptions, error) {
	var result routingfacade.RuntimeOptions
	var protocol string
	if c == nil || c.db == nil || c.projects == nil || tenantID < 1 || projectID < 1 || modelCredentialID < 1 {
		return result, routingfacade.ErrRuntimeSelectionUnavailable
	}
	if err := c.db.QueryRow(ctx, `SELECT p.current_version_id,c.protocol
FROM workforce_projects p
JOIN workforce_model_credentials c ON c.tenant_id=p.tenant_id AND c.id=$3 AND c.status='ACTIVE'
WHERE p.tenant_id=$1 AND p.id=$2 AND p.status='ACTIVE'`, tenantID, projectID, modelCredentialID).Scan(&result.ProjectVersionID, &protocol); errors.Is(err, pgx.ErrNoRows) {
		return result, routingfacade.ErrRuntimeSelectionUnavailable
	} else if err != nil {
		return result, err
	}
	compiled, err := c.projects.CompileExecutionRequirements(ctx, tenantID, projectID)
	if err != nil {
		return result, err
	}
	if compiled.ProjectVersionID != result.ProjectVersionID {
		return result, routingfacade.ErrRuntimeSelectionUnavailable
	}
	requiredHooks, err := projectHookEvents(ctx, c.db, tenantID, result.ProjectVersionID)
	if err != nil {
		return result, err
	}

	rows, err := c.db.Query(ctx, `SELECT id,alias,harness_type,harness_version,image_ref,image_digest,scheduling_priority,
	provider_protocols_json,environment_capabilities_json,hook_events_json,execution_kinds_json,manifest_json,status,scheduling_scope,contract_version
FROM workforce_harness_releases
WHERE status='ACTIVE' AND scheduling_scope='PRODUCTION' AND execution_kinds_json ? 'AGENT_ATTEMPT'
	AND image_digest ~ '^sha256:[0-9a-f]{64}$' AND jsonb_typeof(manifest_json)='object'
ORDER BY scheduling_priority,id`)
	if err != nil {
		return result, err
	}
	byID := map[int64]*routingfacade.RuntimeOption{}
	byDigest := map[string][]*routingfacade.RuntimeOption{}
	for rows.Next() {
		var release catalogRelease
		var protocolsRaw, environmentRaw, hooksRaw, kindsRaw, manifestRaw json.RawMessage
		if err = rows.Scan(&release.id, &release.name, &release.harnessType, &release.harnessVersion, &release.imageRef, &release.imageDigest, &release.priority,
			&protocolsRaw, &environmentRaw, &hooksRaw, &kindsRaw, &manifestRaw, &release.status, &release.scope, &release.contract); err != nil {
			rows.Close()
			return result, err
		}
		if json.Unmarshal(protocolsRaw, &release.protocols) != nil || json.Unmarshal(environmentRaw, &release.environment) != nil || json.Unmarshal(hooksRaw, &release.hookEvents) != nil || json.Unmarshal(kindsRaw, &release.executionKinds) != nil {
			continue
		}
		if json.Unmarshal(manifestRaw, &release.manifest) != nil || !compatibleTaskRuntime(release, compiled.Requirements, requiredHooks, protocol) {
			continue
		}
		manifest := release.manifest
		name := strings.TrimSpace(manifest.Metadata.Name)
		if name == "" {
			name = release.name
		}
		option := &routingfacade.RuntimeOption{RuntimeReleaseID: release.id, Name: name, Description: strings.TrimSpace(manifest.Metadata.Description), HarnessType: release.harnessType, HarnessVersion: release.harnessVersion, AdapterVersion: manifest.Adapter.Version, ImageRef: release.imageRef, ImageDigest: release.imageDigest, ProviderProtocols: release.protocols, EnvironmentCapabilities: release.environment, HookEvents: release.hookEvents, ExecutionKinds: release.executionKinds, SupportsIndependentReview: containsString(release.executionKinds, "REVIEW_RUN"), SchedulingPriority: release.priority, Workers: []routingfacade.RuntimeWorkerOption{}}
		byID[release.id] = option
		byDigest[release.imageDigest] = append(byDigest[release.imageDigest], option)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	waitingRows, err := c.db.Query(ctx, `SELECT t.selected_runtime_release_id,COUNT(DISTINCT t.id)::INTEGER
FROM workforce_tasks t
JOIN workforce_execution_nodes n ON n.task_id=t.id AND n.status='QUEUED'
WHERE t.selected_runtime_release_id IS NOT NULL
GROUP BY t.selected_runtime_release_id`)
	if err != nil {
		return result, err
	}
	for waitingRows.Next() {
		var releaseID int64
		var waiting int
		if err = waitingRows.Scan(&releaseID, &waiting); err != nil {
			waitingRows.Close()
			return result, err
		}
		if option := byID[releaseID]; option != nil {
			option.WaitingTaskCount = waiting
		}
	}
	if err = waitingRows.Err(); err != nil {
		waitingRows.Close()
		return result, err
	}
	waitingRows.Close()

	workers := make([]*runtimeWorkerInventory, 0)
	selectedWorkerIDs := map[int64]bool{}
	inventoryRows, err := c.db.Query(ctx, `SELECT wi.image_digest,w.id,w.name,LEAST(w.max_active_attempts,s.max_active_attempts)
FROM workforce_worker_images wi
JOIN workforce_workers w ON w.id=wi.worker_id AND w.current_instance_id=wi.instance_id
	AND w.desired_state='ENABLED' AND w.decommissioned_at IS NULL
JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id AND s.ended_at IS NULL
	AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds' AND s.consumer_state='CONSUMING' AND s.docker_ready
WHERE wi.role='attempt' AND wi.admission_status='ADMITTED' AND wi.presence_status='PRESENT'
	AND wi.scheduling_scope='PRODUCTION'
	AND wi.contract_version='seven.worker-image/v1' AND wi.execution_kinds_json ? 'AGENT_ATTEMPT'
ORDER BY w.id`)
	if err != nil {
		return result, err
	}
	for inventoryRows.Next() {
		worker := &runtimeWorkerInventory{}
		if err = inventoryRows.Scan(&worker.digest, &worker.id, &worker.name, &worker.maxAttempts); err != nil {
			inventoryRows.Close()
			return result, err
		}
		if len(byDigest[worker.digest]) > 0 {
			workers = append(workers, worker)
			selectedWorkerIDs[worker.id] = true
		}
	}
	if err = inventoryRows.Err(); err != nil {
		inventoryRows.Close()
		return result, err
	}
	inventoryRows.Close()

	reservedRows, err := c.db.Query(ctx, `SELECT a.worker_id,COUNT(*)::INTEGER
FROM workforce_task_assignments a
WHERE a.status IN ('RESERVED','PUBLISHED') OR (
	a.status='CLAIMED' AND NOT EXISTS (
		SELECT 1 FROM workforce_attempt_runtimes r
		WHERE r.assignment_id=a.id
		  AND r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','RECONCILIATION_REQUIRED','TERMINATING')
	)
)
GROUP BY a.worker_id`)
	if err != nil {
		return result, err
	}
	for reservedRows.Next() {
		var workerID int64
		var reserved int
		if err = reservedRows.Scan(&workerID, &reserved); err != nil {
			reservedRows.Close()
			return result, err
		}
		if selectedWorkerIDs[workerID] {
			for _, worker := range workers {
				if worker.id == workerID {
					worker.reserved = reserved
				}
			}
		}
	}
	if err = reservedRows.Err(); err != nil {
		reservedRows.Close()
		return result, err
	}
	reservedRows.Close()

	runtimeRows, err := c.db.Query(ctx, `SELECT a.worker_id,
	COUNT(*) FILTER (WHERE r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING'))::INTEGER,
	COUNT(*) FILTER (WHERE r.status IN ('RUNNING','RECOVERING','REATTACHING','RECONCILIATION_REQUIRED','TERMINATING'))::INTEGER
FROM workforce_task_assignments a
JOIN workforce_task_attempts ta ON ta.assignment_id=a.id
JOIN workforce_attempt_runtimes r ON r.attempt_id=ta.id
GROUP BY a.worker_id`)
	if err != nil {
		return result, err
	}
	for runtimeRows.Next() {
		var workerID int64
		var starting, running int
		if err = runtimeRows.Scan(&workerID, &starting, &running); err != nil {
			runtimeRows.Close()
			return result, err
		}
		if selectedWorkerIDs[workerID] {
			for _, worker := range workers {
				if worker.id == workerID {
					worker.starting = starting
					worker.running = running
				}
			}
		}
	}
	if err = runtimeRows.Err(); err != nil {
		runtimeRows.Close()
		return result, err
	}
	runtimeRows.Close()

	for _, worker := range workers {
		attachWorkerInventory(byDigest, worker)
	}
	for _, option := range byID {
		if len(option.Workers) > 0 {
			sort.SliceStable(option.Workers, func(i, j int) bool { return option.Workers[i].WorkerID < option.Workers[j].WorkerID })
			result.Items = append(result.Items, *option)
		}
	}
	sortRuntimeOptions(result.Items)
	if len(result.Items) > 0 {
		result.Items[0].Recommended = true
	}
	return result, nil
}

func attachWorkerInventory(byDigest map[string][]*routingfacade.RuntimeOption, worker *runtimeWorkerInventory) {
	if worker == nil {
		return
	}
	available := int(worker.maxAttempts) - worker.reserved - worker.starting - worker.running
	if available < 0 {
		available = 0
	}
	for _, option := range byDigest[worker.digest] {
		option.AvailableSlots += available
		option.Workers = append(option.Workers, routingfacade.RuntimeWorkerOption{WorkerID: worker.id, Name: worker.name, State: "ONLINE", ReservedAttempts: worker.reserved, StartingAttempts: worker.starting, RunningAttempts: worker.running, AvailableAttempts: available})
	}
}

func sortRuntimeOptions(items []routingfacade.RuntimeOption) {
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.SchedulingPriority != right.SchedulingPriority {
			return left.SchedulingPriority < right.SchedulingPriority
		}
		if left.AvailableSlots != right.AvailableSlots {
			return left.AvailableSlots > right.AvailableSlots
		}
		if left.WaitingTaskCount != right.WaitingTaskCount {
			return left.WaitingTaskCount < right.WaitingTaskCount
		}
		// Equally ranked and loaded releases should prefer the newer admitted
		// release, so publishing a fixed runtime does not leave its predecessor
		// as the default forever. This only orders new-task options; frozen task
		// selections are validated and dispatched by their exact release ID.
		return left.RuntimeReleaseID > right.RuntimeReleaseID
	})
}

func loadCatalogRelease(ctx context.Context, db catalogQuerier, releaseID int64) (catalogRelease, error) {
	var result catalogRelease
	var protocolsRaw, environmentRaw, hooksRaw, kindsRaw, manifestRaw json.RawMessage
	err := db.QueryRow(ctx, `SELECT id,alias,harness_type,harness_version,image_ref,image_digest,scheduling_priority,provider_protocols_json,environment_capabilities_json,hook_events_json,execution_kinds_json,manifest_json,status,scheduling_scope,contract_version
FROM workforce_harness_releases
WHERE id=$1 AND status='ACTIVE' AND scheduling_scope='PRODUCTION' AND execution_kinds_json ? 'AGENT_ATTEMPT'
`, releaseID).Scan(&result.id, &result.name, &result.harnessType, &result.harnessVersion, &result.imageRef, &result.imageDigest, &result.priority, &protocolsRaw, &environmentRaw, &hooksRaw, &kindsRaw, &manifestRaw, &result.status, &result.scope, &result.contract)
	if err != nil {
		return result, err
	}
	if json.Unmarshal(protocolsRaw, &result.protocols) != nil || json.Unmarshal(environmentRaw, &result.environment) != nil || json.Unmarshal(hooksRaw, &result.hookEvents) != nil || json.Unmarshal(kindsRaw, &result.executionKinds) != nil || json.Unmarshal(manifestRaw, &result.manifest) != nil {
		return catalogRelease{}, routingfacade.ErrRuntimeSelectionIncompatible
	}
	return result, nil
}

func projectHookEvents(ctx context.Context, db catalogQuerier, tenantID, projectVersionID int64) ([]string, error) {
	var releaseIDs []int64
	if err := db.QueryRow(ctx, `SELECT hook_release_ids FROM workforce_project_versions WHERE tenant_id=$1 AND id=$2`, tenantID, projectVersionID).Scan(&releaseIDs); err != nil {
		return nil, err
	}
	if len(releaseIDs) == 0 {
		return []string{}, nil
	}
	rows, err := db.Query(ctx, `SELECT manifest_json FROM workforce_hook_package_releases WHERE id=ANY($1::BIGINT[]) AND ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id=$2))`, releaseIDs, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	count := 0
	for rows.Next() {
		var raw json.RawMessage
		var manifest struct {
			Entrypoints []struct {
				Event string `json:"event"`
			} `json:"entrypoints"`
		}
		if rows.Scan(&raw) != nil || json.Unmarshal(raw, &manifest) != nil {
			return nil, routingfacade.ErrRuntimeSelectionIncompatible
		}
		count++
		for _, entry := range manifest.Entrypoints {
			if entry.Event != "" && !containsString(result, entry.Event) {
				result = append(result, entry.Event)
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if count != len(releaseIDs) {
		return nil, routingfacade.ErrRuntimeSelectionIncompatible
	}
	return result, nil
}

func containsString(values []string, expected string) bool {
	expected = strings.TrimSpace(expected)
	for _, value := range values {
		if strings.TrimSpace(value) == expected {
			return true
		}
	}
	return false
}
