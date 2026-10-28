package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

type Resolver struct {
	platform routingfacade.PlatformIdentity
	now      func() time.Time
}

func NewResolver(platform routingfacade.PlatformIdentity) *Resolver {
	return &Resolver{platform: platform, now: time.Now}
}

type modelCredential struct {
	id, version, runtimeReleaseID int64
	name, protocol                string
	primaryModel                  string
}

type runtimeCandidate struct {
	workerID, maxAttempts, active   int64
	instance, imageRef, imageDigest string
	releaseID                       int64
	contract                        string
	priority                        int
	diskAvailable, diskTotal        int64
	lastAssigned                    time.Time
	manifest                        runtimeManifest
}

type reviewRuntimeCandidate struct {
	workerID, maxReviews, maxTotal, activeReviews, activeContainers int64
	instance, imageDigest, contract, providerProxyImage             string
	releaseID, priority, diskAvailable, diskTotal                   int64
	lastAssigned                                                    time.Time
	manifest                                                        runtimeManifest
}

type runtimeManifest struct {
	Metadata struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"metadata"`
	Adapter struct {
		ID           string   `json:"id"`
		Version      string   `json:"version"`
		Capabilities []string `json:"capabilities"`
		Protocol     struct {
			Name string `json:"name"`
			Min  int    `json:"min"`
			Max  int    `json:"max"`
		} `json:"protocol"`
	} `json:"adapter"`
	Probe struct {
		Capabilities []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"capabilities"`
	} `json:"probe"`
	ResultProcessor struct {
		PythonRuntimeIdentity string `json:"pythonRuntimeIdentity"`
		BootstrapDigest       string `json:"bootstrapSha256"`
		SDKDigest             string `json:"sdkSha256"`
		ModuleIndexDigest     string `json:"moduleIndexSha256"`
	} `json:"resultProcessor"`
	Scheduling struct {
		HookEvents        []string `json:"hookEvents"`
		ProviderProtocols []string `json:"providerProtocols"`
	} `json:"scheduling"`
}

func (r *Resolver) ResolveAssignment(ctx context.Context, tx pgx.Tx, node routingfacade.NodeIdentity, requirements projectfacade.ExecutionRequirements, base taskdomain.ExecutionSnapshot) (routingfacade.ResolvedAssignment, string, error) {
	if tx == nil || node.TenantID < 1 || node.TaskID < 1 || node.NodeID < 1 || requirements.ExecutionKind != "AGENT_ATTEMPT" {
		return routingfacade.ResolvedAssignment{}, routingfacade.WaitProjectInvalid, nil
	}
	if preserved, continuation, wait, err := r.resolveContinuationAssignment(ctx, tx, node); err != nil {
		return routingfacade.ResolvedAssignment{}, "", err
	} else if continuation {
		return preserved, wait, nil
	}
	credential, found, err := r.resolveTaskModelCredential(ctx, tx, node.TenantID, node.TaskID)
	if err != nil {
		return routingfacade.ResolvedAssignment{}, "", err
	}
	if !found {
		return routingfacade.ResolvedAssignment{}, routingfacade.WaitModelCredential, nil
	}
	hookReleases, hookDigest, requiredHookEvents, err := r.resolveManagedHooks(ctx, tx, node.TenantID, node.TaskID)
	if err != nil {
		return routingfacade.ResolvedAssignment{}, routingfacade.WaitProjectInvalid, nil
	}
	runtime, wait, err := r.selectRuntime(ctx, tx, credential.runtimeReleaseID, requirements, requiredHookEvents, credential.protocol)
	if err != nil || wait != "" {
		return routingfacade.ResolvedAssignment{}, wait, err
	}

	snapshot := base
	snapshot.HarnessReleaseID = runtime.releaseID
	snapshot.HarnessImageDigest = runtime.imageDigest
	snapshot.ModelCredentialID = credential.id
	snapshot.CredentialVersion = credential.version
	snapshot.ModelProtocol = credential.protocol
	snapshot.PrimaryModel = credential.primaryModel
	snapshot.ProviderProxyImageDigest = r.platform.ProviderProxyImageDigest
	snapshot.ProviderProxyVersion = r.platform.ProviderProxyVersion
	snapshot.LocalProviderFacadeVersion = r.platform.LocalFacadeVersion
	snapshot.LocalProviderFacadeDigest = r.platform.LocalFacadeDigest
	snapshot.ProviderGatewayIdentity = r.platform.GatewayIdentity
	snapshot.ProviderGatewayContract = r.platform.GatewayContractDigest
	snapshot.ProviderAdapterIdentity = r.platform.ProviderAdapterIdentity
	snapshot.PythonRuntimeIdentity = runtime.manifest.ResultProcessor.PythonRuntimeIdentity
	snapshot.BootstrapDigest = runtime.manifest.ResultProcessor.BootstrapDigest
	snapshot.SDKDigest = runtime.manifest.ResultProcessor.SDKDigest
	snapshot.ModuleIndexDigest = runtime.manifest.ResultProcessor.ModuleIndexDigest
	snapshot.IsolationProfile = "seven.worker-image/v1"
	snapshot.HookConfigurationDigest = hookDigest
	snapshot.PackageReleases = hookReleases
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return routingfacade.ResolvedAssignment{}, "", err
	}
	selection := struct {
		TaskID, NodeID, Generation, WorkerID, RuntimeReleaseID, ModelCredentialID, CredentialVersion int64
		WorkerInstanceID, RuntimeImageDigest, AdapterID, AdapterVersion, ModelProtocol               string
		PrimaryModel, RequirementsDigest                                                             string
		Snapshot                                                                                     json.RawMessage
	}{node.TaskID, node.NodeID, node.Generation, runtime.workerID, runtime.releaseID, credential.id, credential.version, runtime.instance, runtime.imageDigest, runtime.manifest.Adapter.ID, runtime.manifest.Adapter.Version, credential.protocol, credential.primaryModel, digestJSON(requirements), snapshotJSON}
	selectionJSON, _ := json.Marshal(selection)
	result := routingfacade.ResolvedAssignment{
		WorkerID: runtime.workerID, WorkerInstanceID: runtime.instance,
		RuntimeReleaseID: runtime.releaseID, RuntimeImageRef: runtime.imageRef, RuntimeImageDigest: runtime.imageDigest,
		AdapterID: runtime.manifest.Adapter.ID, AdapterVersion: runtime.manifest.Adapter.Version,
		AdapterProtocol: runtime.manifest.Adapter.Protocol.Name + "/v1", AdapterCapabilities: append([]string(nil), runtime.manifest.Adapter.Capabilities...),
		HookConfigurationDigest: snapshot.HookConfigurationDigest,
		ModelCredentialID:       credential.id, CredentialVersion: credential.version, ModelCredentialName: credential.name,
		ModelProtocol: credential.protocol, PrimaryModel: credential.primaryModel, WorkerImageContract: runtime.contract,
		IsolationProfile: snapshot.IsolationProfile, Snapshot: snapshot, SnapshotJSON: snapshotJSON, SelectionDigest: digestBytes(selectionJSON),
	}
	return result, "", nil
}

func (r *Resolver) resolveTaskModelCredential(ctx context.Context, tx pgx.Tx, tenantID, taskID int64) (modelCredential, bool, error) {
	var credential modelCredential
	err := tx.QueryRow(ctx, `SELECT c.id,c.secret_version,COALESCE(t.selected_runtime_release_id,0),c.name,c.protocol,t.primary_model
FROM workforce_tasks t
JOIN workforce_model_credentials c ON c.id=t.model_credential_id AND c.tenant_id=t.tenant_id
WHERE t.tenant_id=$1 AND t.id=$2 AND c.status='ACTIVE'`, tenantID, taskID).Scan(
		&credential.id, &credential.version, &credential.runtimeReleaseID, &credential.name, &credential.protocol, &credential.primaryModel)
	if errors.Is(err, pgx.ErrNoRows) {
		return modelCredential{}, false, nil
	}
	if err != nil {
		return modelCredential{}, false, err
	}
	return credential, credential.id > 0 && credential.version > 0 && credential.runtimeReleaseID > 0 && credential.primaryModel != "" && (credential.protocol == "OPENAI" || credential.protocol == "ANTHROPIC"), nil
}

// resolveContinuationAssignment preserves the runtime, Adapter and model credential
// identity captured by a continuation checkpoint. The portable checkpoint may
// move to another Worker that has the same admitted image; it must never become
// a fresh execution routed to a different model or Adapter.
func (r *Resolver) resolveContinuationAssignment(ctx context.Context, tx pgx.Tx, node routingfacade.NodeIdentity) (routingfacade.ResolvedAssignment, bool, string, error) {
	var sourceAttemptID int64
	err := tx.QueryRow(ctx, `SELECT c.attempt_id FROM workforce_task_turns tr
JOIN workforce_continuation_checkpoints c ON c.id=tr.source_checkpoint_id AND c.task_id=tr.task_id
WHERE tr.id=$1 AND tr.task_id=$2 AND c.status='AVAILABLE' AND c.consumed_by_turn_id=tr.id`, node.TaskTurnID, node.TaskID).Scan(&sourceAttemptID)
	if errors.Is(err, pgx.ErrNoRows) {
		return routingfacade.ResolvedAssignment{}, false, "", nil
	}
	if err != nil {
		return routingfacade.ResolvedAssignment{}, true, "", err
	}
	var result routingfacade.ResolvedAssignment
	var adapterRaw, snapshotRaw json.RawMessage
	err = tx.QueryRow(ctx, `SELECT a.worker_id,a.worker_instance_id,a.agent_runtime_release_id,a.runtime_image_ref,a.runtime_image_digest,
a.adapter_identity,a.adapter_version,a.adapter_protocol,a.adapter_capabilities_json,a.hook_configuration_digest,
a.model_credential_id,a.credential_version,a.model_protocol,a.primary_model,a.worker_image_contract,a.isolation_profile,a.resolved_execution_snapshot_json
FROM workforce_task_attempts source
JOIN workforce_task_assignments a ON a.id=source.assignment_id AND a.task_id=source.task_id
WHERE source.id=$1 AND source.task_id=$2`, sourceAttemptID, node.TaskID).Scan(
		&result.WorkerID, &result.WorkerInstanceID, &result.RuntimeReleaseID, &result.RuntimeImageRef, &result.RuntimeImageDigest,
		&result.AdapterID, &result.AdapterVersion, &result.AdapterProtocol, &adapterRaw, &result.HookConfigurationDigest,
		&result.ModelCredentialID, &result.CredentialVersion, &result.ModelProtocol, &result.PrimaryModel,
		&result.WorkerImageContract, &result.IsolationProfile, &snapshotRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return routingfacade.ResolvedAssignment{}, true, routingfacade.WaitAgentRuntime, nil
	}
	if err != nil {
		return routingfacade.ResolvedAssignment{}, true, "", err
	}
	sourceWorkerID, sourceWorkerInstance := result.WorkerID, result.WorkerInstanceID
	if json.Unmarshal(adapterRaw, &result.AdapterCapabilities) != nil || json.Unmarshal(snapshotRaw, &result.Snapshot) != nil {
		return routingfacade.ResolvedAssignment{}, true, routingfacade.WaitProjectInvalid, nil
	}
	result.SnapshotJSON = append(json.RawMessage(nil), snapshotRaw...)
	// Key rotation must not invalidate a running native session or its
	// continuation. The Assignment keeps the originally selected identity for
	// audit, while every invocation resolves the credential's current secret
	// version. A protocol change is fenced separately while work is active.
	if err = tx.QueryRow(ctx, `SELECT name FROM workforce_model_credentials
WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE' AND protocol=$3`, node.TenantID, result.ModelCredentialID, result.ModelProtocol).Scan(&result.ModelCredentialName); errors.Is(err, pgx.ErrNoRows) {
		return routingfacade.ResolvedAssignment{}, true, routingfacade.WaitModelCredential, nil
	} else if err != nil {
		return routingfacade.ResolvedAssignment{}, true, "", err
	}
	worker, wait, err := r.selectContinuationWorker(ctx, tx, result.RuntimeReleaseID, result.RuntimeImageDigest, result.WorkerImageContract, result.Snapshot.ProviderProxyImageDigest, sourceWorkerID, sourceWorkerInstance, result.Snapshot.StagePurpose == "PLAN")
	if err != nil || wait != "" {
		return routingfacade.ResolvedAssignment{}, true, wait, err
	}
	result.WorkerID, result.WorkerInstanceID = worker.workerID, worker.instance
	digestInput := struct {
		TaskID, NodeID, Generation                                        int64
		RuntimeReleaseID, ModelCredentialID, CredentialVersion, WorkerID  int64
		WorkerInstanceID, RuntimeImageDigest, ModelProtocol, PrimaryModel string
		Snapshot                                                          json.RawMessage
	}{node.TaskID, node.NodeID, node.Generation, result.RuntimeReleaseID, result.ModelCredentialID, result.CredentialVersion, result.WorkerID,
		result.WorkerInstanceID, result.RuntimeImageDigest, result.ModelProtocol, result.PrimaryModel, result.SnapshotJSON}
	raw, _ := json.Marshal(digestInput)
	result.SelectionDigest = digestBytes(raw)
	return result, true, "", nil
}

func (r *Resolver) selectContinuationWorker(ctx context.Context, tx pgx.Tx, runtimeReleaseID int64, imageDigest, imageContract, providerProxyImage string, sourceWorkerID int64, sourceWorkerInstance string, planning bool) (runtimeCandidate, string, error) {
	var admittedImageDigest string
	err := tx.QueryRow(ctx, `SELECT image_digest FROM workforce_harness_releases
WHERE id=$1 AND status='ACTIVE' AND scheduling_scope='PRODUCTION' AND execution_kinds_json ? 'AGENT_ATTEMPT'`, runtimeReleaseID).Scan(&admittedImageDigest)
	if errors.Is(err, pgx.ErrNoRows) {
		return runtimeCandidate{}, routingfacade.WaitAgentRuntime, nil
	}
	if err != nil {
		return runtimeCandidate{}, "", err
	}
	if admittedImageDigest != imageDigest {
		return runtimeCandidate{}, routingfacade.WaitAgentRuntime, nil
	}
	rows, err := tx.Query(ctx, `SELECT w.id,w.current_instance_id,LEAST(w.max_active_attempts,s.max_active_attempts),
COALESCE(NULLIF(s.resource_snapshot_json->>'workspaceDiskAvailableBytes','')::bigint,0),
COALESCE(NULLIF(s.resource_snapshot_json->>'workspaceDiskTotalBytes','')::bigint,0),wi.scheduling_priority
FROM workforce_workers w
JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
JOIN workforce_worker_images wi ON wi.worker_id=w.id AND wi.instance_id=w.current_instance_id AND wi.role='attempt'
WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL
  AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds' AND s.consumer_state='CONSUMING' AND s.docker_ready
  AND ($3::boolean=false OR s.capabilities_json @> '{"planningSource":"v1"}'::jsonb)
  AND wi.image_digest=$1 AND wi.contract_version=$2 AND wi.admission_status='ADMITTED' AND wi.presence_status='PRESENT'
  AND wi.scheduling_scope='PRODUCTION' AND wi.execution_kinds_json ? 'AGENT_ATTEMPT'
ORDER BY wi.scheduling_priority,w.id
FOR UPDATE OF w SKIP LOCKED`, imageDigest, imageContract, planning)
	if err != nil {
		return runtimeCandidate{}, "", err
	}
	var discovered []runtimeCandidate
	for rows.Next() {
		candidate := runtimeCandidate{releaseID: runtimeReleaseID, imageDigest: imageDigest, contract: imageContract}
		if err = rows.Scan(&candidate.workerID, &candidate.instance, &candidate.maxAttempts, &candidate.diskAvailable, &candidate.diskTotal, &candidate.priority); err != nil {
			rows.Close()
			return runtimeCandidate{}, "", err
		}
		discovered = append(discovered, candidate)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return runtimeCandidate{}, "", err
	}
	rows.Close()

	compatible, capacityBlocked, diskBlocked := 0, 0, 0
	available := make([]runtimeCandidate, 0, len(discovered))
	for _, candidate := range discovered {
		if err = tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(MAX(created_at),to_timestamp(0)) FROM workforce_task_assignments
WHERE worker_id=$1 AND status IN ('RESERVED','PUBLISHED','CLAIMED')`, candidate.workerID).Scan(&candidate.active, &candidate.lastAssigned); err != nil {
			return runtimeCandidate{}, "", err
		}
		if providerProxyImage != "" {
			var proxyPresent bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_worker_images
WHERE worker_id=$1 AND instance_id=$2 AND role='provider-proxy' AND image_digest=$3
  AND admission_status='ADMITTED' AND presence_status='PRESENT')`, candidate.workerID, candidate.instance, providerProxyImage).Scan(&proxyPresent); err != nil {
				return runtimeCandidate{}, "", err
			}
			if !proxyPresent {
				continue
			}
		}
		compatible++
		if candidate.diskTotal > 0 && (candidate.diskAvailable < 5*1024*1024*1024 || candidate.diskAvailable*100 < candidate.diskTotal*5) {
			diskBlocked++
			continue
		}
		if candidate.maxAttempts < 1 || candidate.active >= candidate.maxAttempts {
			capacityBlocked++
			continue
		}
		available = append(available, candidate)
	}
	if len(available) == 0 {
		switch {
		case compatible == 0:
			return runtimeCandidate{}, routingfacade.WaitAgentRuntime, nil
		case diskBlocked == compatible:
			return runtimeCandidate{}, routingfacade.WaitWorkerDisk, nil
		case capacityBlocked > 0:
			return runtimeCandidate{}, routingfacade.WaitWorkerCapacity, nil
		default:
			return runtimeCandidate{}, routingfacade.WaitWorkerCapacity, nil
		}
	}
	sortContinuationWorkers(available, sourceWorkerID, sourceWorkerInstance)
	return available[0], "", nil
}

func sortContinuationWorkers(available []runtimeCandidate, sourceWorkerID int64, sourceWorkerInstance string) {
	sort.SliceStable(available, func(i, j int) bool {
		left, right := available[i], available[j]
		leftIsSource := left.workerID == sourceWorkerID && left.instance == sourceWorkerInstance
		rightIsSource := right.workerID == sourceWorkerID && right.instance == sourceWorkerInstance
		if leftIsSource != rightIsSource {
			return leftIsSource
		}
		if left.priority != right.priority {
			return left.priority < right.priority
		}
		leftUse, rightUse := float64(left.active)/float64(left.maxAttempts), float64(right.active)/float64(right.maxAttempts)
		if leftUse != rightUse {
			return leftUse < rightUse
		}
		if left.diskAvailable != right.diskAvailable {
			return left.diskAvailable > right.diskAvailable
		}
		if !left.lastAssigned.Equal(right.lastAssigned) {
			return left.lastAssigned.Before(right.lastAssigned)
		}
		return left.workerID < right.workerID
	})
}

func (r *Resolver) ResolveReviewAssignment(ctx context.Context, tx pgx.Tx, requirements routingfacade.ReviewRequirements) (routingfacade.ResolvedReviewAssignment, string, error) {
	if tx == nil || requirements.TenantID < 1 || requirements.ReviewRunID < 1 {
		return routingfacade.ResolvedReviewAssignment{}, routingfacade.WaitProjectInvalid, nil
	}
	if requirements.ModelCredentialID < 1 || requirements.CredentialVersion < 1 || requirements.PrimaryModel == "" || (requirements.ModelProtocol != "OPENAI" && requirements.ModelProtocol != "ANTHROPIC") {
		return routingfacade.ResolvedReviewAssignment{}, routingfacade.WaitModelCredential, nil
	}
	var credentialActive bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_model_credentials WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE' AND secret_version=$3 AND protocol=$4)`, requirements.TenantID, requirements.ModelCredentialID, requirements.CredentialVersion, requirements.ModelProtocol).Scan(&credentialActive); err != nil {
		return routingfacade.ResolvedReviewAssignment{}, "", err
	}
	if !credentialActive {
		return routingfacade.ResolvedReviewAssignment{}, routingfacade.WaitModelCredential, nil
	}
	runtime, wait, err := r.selectReviewRuntime(ctx, tx, requirements.RequiredAdapterCapabilities, requirements.PreferredRuntimeReleaseID, requirements.ModelProtocol)
	if err != nil || wait != "" {
		return routingfacade.ResolvedReviewAssignment{}, wait, err
	}
	adapterCapabilities, _ := json.Marshal(runtime.manifest.Adapter.Capabilities)
	result := routingfacade.ResolvedReviewAssignment{
		RuntimeReleaseID: runtime.releaseID, RuntimeImageDigest: runtime.imageDigest,
		AdapterID: runtime.manifest.Adapter.ID, AdapterVersion: runtime.manifest.Adapter.Version,
		AdapterProtocol: runtime.manifest.Adapter.Protocol.Name + "/v1", AdapterCapabilities: adapterCapabilities,
		ModelCredentialID: requirements.ModelCredentialID, CredentialVersion: requirements.CredentialVersion,
		ModelProtocol: requirements.ModelProtocol, PrimaryModel: requirements.PrimaryModel,
		ProviderProxyImage: runtime.providerProxyImage, WorkerID: runtime.workerID, WorkerInstanceID: runtime.instance,
	}
	digestInput := struct {
		ReviewRunID, RuntimeReleaseID, ModelCredentialID, CredentialVersion, WorkerID int64
		RuntimeImage, AdapterID, AdapterVersion, AdapterProtocol                      string
		ModelProtocol, PrimaryModel, ProviderProxy, WorkerSession                     string
		RequiredAdapter                                                               []string
	}{requirements.ReviewRunID, result.RuntimeReleaseID, result.ModelCredentialID, result.CredentialVersion, result.WorkerID,
		result.RuntimeImageDigest, result.AdapterID, result.AdapterVersion, result.AdapterProtocol,
		result.ModelProtocol, result.PrimaryModel, result.ProviderProxyImage, result.WorkerInstanceID,
		append([]string(nil), requirements.RequiredAdapterCapabilities...)}
	raw, _ := json.Marshal(digestInput)
	result.SelectionDigest = digestBytes(raw)
	return result, "", nil
}

func (r *Resolver) selectReviewRuntime(ctx context.Context, tx pgx.Tx, required []string, preferredReleaseID int64, protocol string) (reviewRuntimeCandidate, string, error) {
	rows, err := tx.Query(ctx, `SELECT w.id,w.current_instance_id,w.max_active_reviews,w.max_total_containers,
COALESCE(NULLIF(s.resource_snapshot_json->>'workspaceDiskAvailableBytes','')::bigint,0),
COALESCE(NULLIF(s.resource_snapshot_json->>'workspaceDiskTotalBytes','')::bigint,0),
wi.image_digest,wi.contract_version,wi.scheduling_priority,wi.manifest_json
FROM workforce_workers w
JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
JOIN workforce_worker_images wi ON wi.worker_id=w.id AND wi.instance_id=w.current_instance_id AND wi.role='attempt'
WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL
  AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds' AND s.consumer_state='CONSUMING' AND s.docker_ready
  AND s.capabilities_json @> '{"independentReviewConsumer":"v1"}'::jsonb
  AND wi.contract_version='seven.worker-image/v1' AND wi.admission_status='ADMITTED' AND wi.presence_status='PRESENT'
  AND wi.scheduling_scope='PRODUCTION' AND wi.execution_kinds_json ? 'REVIEW_RUN'
ORDER BY wi.scheduling_priority,wi.image_digest,w.id
FOR UPDATE OF w SKIP LOCKED`)
	if err != nil {
		return reviewRuntimeCandidate{}, "", err
	}
	var discovered []reviewRuntimeCandidate
	for rows.Next() {
		var candidate reviewRuntimeCandidate
		var raw json.RawMessage
		if err := rows.Scan(&candidate.workerID, &candidate.instance, &candidate.maxReviews, &candidate.maxTotal,
			&candidate.diskAvailable, &candidate.diskTotal, &candidate.imageDigest, &candidate.contract, &candidate.priority, &raw); err != nil {
			return reviewRuntimeCandidate{}, "", err
		}
		if json.Unmarshal(raw, &candidate.manifest) != nil || !validRuntimeManifest(candidate.manifest) || !supportsRuntimeCapabilities(candidate.manifest, required) || !slices.Contains(candidate.manifest.Scheduling.ProviderProtocols, protocol) {
			continue
		}
		discovered = append(discovered, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return reviewRuntimeCandidate{}, "", err
	}
	rows.Close()

	var candidates []reviewRuntimeCandidate
	compatible, capacityBlocked, diskBlocked := 0, 0, 0
	for _, candidate := range discovered {
		var releasePriority int64
		if err := tx.QueryRow(ctx, `SELECT id,scheduling_priority FROM workforce_harness_releases
WHERE image_digest=$1 AND status='ACTIVE' AND scheduling_scope='PRODUCTION' AND execution_kinds_json ? 'REVIEW_RUN'
ORDER BY scheduling_priority,id LIMIT 1`, candidate.imageDigest).Scan(&candidate.releaseID, &releasePriority); errors.Is(err, pgx.ErrNoRows) {
			continue
		} else if err != nil {
			return reviewRuntimeCandidate{}, "", err
		}
		if releasePriority < candidate.priority {
			candidate.priority = releasePriority
		}
		if err := tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(MAX(updated_at),to_timestamp(0)) FROM workforce_review_runs
WHERE worker_id=$1 AND status IN ('ASSIGNED','RUNNING','VALIDATING_REPORT')`, candidate.workerID).Scan(&candidate.activeReviews, &candidate.lastAssigned); err != nil {
			return reviewRuntimeCandidate{}, "", err
		}
		var activeAssignments int64
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_task_assignments WHERE worker_id=$1 AND status IN ('RESERVED','PUBLISHED','CLAIMED')`, candidate.workerID).Scan(&activeAssignments); err != nil {
			return reviewRuntimeCandidate{}, "", err
		}
		candidate.activeContainers = candidate.activeReviews*2 + activeAssignments*2
		if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT image_digest FROM workforce_worker_images
WHERE worker_id=$1 AND instance_id=$2 AND role='provider-proxy' AND contract_version='seven.worker-image/v1'
  AND admission_status='ADMITTED' AND presence_status='PRESENT' ORDER BY image_digest LIMIT 1),'')`, candidate.workerID, candidate.instance).Scan(&candidate.providerProxyImage); err != nil {
			return reviewRuntimeCandidate{}, "", err
		}
		if candidate.providerProxyImage == "" {
			continue
		}
		compatible++
		if candidate.diskTotal > 0 && (candidate.diskAvailable < 5*1024*1024*1024 || candidate.diskAvailable*100 < candidate.diskTotal*5) {
			diskBlocked++
			continue
		}
		if candidate.activeReviews >= candidate.maxReviews || candidate.activeContainers+2 > candidate.maxTotal {
			capacityBlocked++
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		switch {
		case compatible == 0:
			return reviewRuntimeCandidate{}, "WAITING_REVIEW_RUNTIME", nil
		case diskBlocked == compatible:
			return reviewRuntimeCandidate{}, routingfacade.WaitWorkerDisk, nil
		case capacityBlocked > 0:
			return reviewRuntimeCandidate{}, "WAITING_REVIEWER_CAPACITY", nil
		default:
			return reviewRuntimeCandidate{}, "WAITING_REVIEWER_CAPACITY", nil
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		leftPreferred, rightPreferred := left.releaseID == preferredReleaseID, right.releaseID == preferredReleaseID
		if leftPreferred != rightPreferred {
			return leftPreferred
		}
		if left.priority != right.priority {
			return left.priority < right.priority
		}
		leftUse, rightUse := float64(left.activeReviews)/float64(left.maxReviews), float64(right.activeReviews)/float64(right.maxReviews)
		if leftUse != rightUse {
			return leftUse < rightUse
		}
		if left.diskAvailable != right.diskAvailable {
			return left.diskAvailable > right.diskAvailable
		}
		if !left.lastAssigned.Equal(right.lastAssigned) {
			return left.lastAssigned.Before(right.lastAssigned)
		}
		if left.releaseID != right.releaseID {
			return left.releaseID < right.releaseID
		}
		return left.workerID < right.workerID
	})
	return candidates[0], "", nil
}

func sortRuntimeCandidates(candidates []runtimeCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		if left.priority != right.priority {
			return left.priority < right.priority
		}
		leftUse, rightUse := float64(left.active)/float64(left.maxAttempts), float64(right.active)/float64(right.maxAttempts)
		if leftUse != rightUse {
			return leftUse < rightUse
		}
		if left.diskAvailable != right.diskAvailable {
			return left.diskAvailable > right.diskAvailable
		}
		if !left.lastAssigned.Equal(right.lastAssigned) {
			return left.lastAssigned.Before(right.lastAssigned)
		}
		if left.releaseID != right.releaseID {
			return left.releaseID < right.releaseID
		}
		return left.workerID < right.workerID
	})
}

func (r *Resolver) selectRuntime(ctx context.Context, tx pgx.Tx, selectedReleaseID int64, requirements projectfacade.ExecutionRequirements, requiredHookEvents []string, protocol string) (runtimeCandidate, string, error) {
	release, err := loadCatalogRelease(ctx, tx, selectedReleaseID)
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, routingfacade.ErrRuntimeSelectionIncompatible) {
		return runtimeCandidate{}, routingfacade.WaitSelectedRuntime, nil
	}
	if err != nil {
		return runtimeCandidate{}, "", err
	}
	if !compatibleTaskRuntime(release, requirements, requiredHookEvents, protocol) {
		return runtimeCandidate{}, routingfacade.WaitSelectedRuntime, nil
	}
	selected := runtimeCandidate{releaseID: release.id, imageRef: release.imageRef, imageDigest: release.imageDigest, contract: release.contract, priority: release.priority, manifest: release.manifest}
	rows, err := tx.Query(ctx, `SELECT w.id,w.current_instance_id,LEAST(w.max_active_attempts,s.max_active_attempts),
COALESCE(NULLIF(s.resource_snapshot_json->>'workspaceDiskAvailableBytes','')::bigint,0),
COALESCE(NULLIF(s.resource_snapshot_json->>'workspaceDiskTotalBytes','')::bigint,0),
wi.scheduling_priority
FROM workforce_workers w
JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id
JOIN workforce_worker_images wi ON wi.worker_id=w.id AND wi.instance_id=w.current_instance_id AND wi.role='attempt'
WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL
  AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds' AND s.consumer_state='CONSUMING' AND s.docker_ready
  AND ($3::boolean=false OR s.capabilities_json @> '{"planningSource":"v1"}'::jsonb)
  AND wi.image_digest=$1 AND wi.contract_version=$2
  AND wi.admission_status='ADMITTED' AND wi.presence_status='PRESENT' AND wi.scheduling_scope='PRODUCTION'
  AND wi.execution_kinds_json ? 'AGENT_ATTEMPT'
ORDER BY wi.scheduling_priority,w.id
FOR UPDATE OF w SKIP LOCKED`, selected.imageDigest, selected.contract, requirements.StagePurpose == "PLAN")
	if err != nil {
		return runtimeCandidate{}, "", err
	}
	var discovered []runtimeCandidate
	for rows.Next() {
		candidate := selected
		if err := rows.Scan(&candidate.workerID, &candidate.instance, &candidate.maxAttempts, &candidate.diskAvailable, &candidate.diskTotal, &candidate.priority); err != nil {
			return runtimeCandidate{}, "", err
		}
		discovered = append(discovered, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return runtimeCandidate{}, "", err
	}
	rows.Close()
	var candidates []runtimeCandidate
	compatible, capacityBlocked, diskBlocked := 0, 0, 0
	for _, candidate := range discovered {
		if err := tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(MAX(created_at),to_timestamp(0)) FROM workforce_task_assignments WHERE worker_id=$1 AND status IN ('RESERVED','PUBLISHED','CLAIMED')`, candidate.workerID).Scan(&candidate.active, &candidate.lastAssigned); err != nil {
			return runtimeCandidate{}, "", err
		}
		if r.platform.ProviderProxyImageDigest != "" {
			var proxy bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_worker_images WHERE worker_id=$1 AND instance_id=$2 AND role='provider-proxy' AND image_digest=$3 AND admission_status='ADMITTED' AND presence_status='PRESENT')`, candidate.workerID, candidate.instance, r.platform.ProviderProxyImageDigest).Scan(&proxy); err != nil {
				return runtimeCandidate{}, "", err
			}
			if !proxy {
				continue
			}
		}
		compatible++
		if candidate.diskTotal > 0 && (candidate.diskAvailable < 5*1024*1024*1024 || candidate.diskAvailable*100 < candidate.diskTotal*5) {
			diskBlocked++
			continue
		}
		if candidate.active >= candidate.maxAttempts {
			capacityBlocked++
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		switch {
		case compatible == 0:
			return runtimeCandidate{}, routingfacade.WaitSelectedRuntime, nil
		case diskBlocked == compatible:
			return runtimeCandidate{}, routingfacade.WaitWorkerDisk, nil
		case capacityBlocked > 0:
			return runtimeCandidate{}, routingfacade.WaitWorkerCapacity, nil
		default:
			return runtimeCandidate{}, routingfacade.WaitWorkerCapacity, nil
		}
	}
	sortRuntimeCandidates(candidates)
	return candidates[0], "", nil
}

func (r *Resolver) resolveManagedHooks(ctx context.Context, tx pgx.Tx, tenantID, taskID int64) ([]taskdomain.HookPackageReleaseSnapshot, string, []string, error) {
	var releaseIDs []int64
	if err := tx.QueryRow(ctx, `SELECT v.hook_release_ids FROM workforce_tasks t JOIN workforce_project_versions v ON v.tenant_id=t.tenant_id AND v.project_id=t.project_id AND v.id=t.project_version_id WHERE t.tenant_id=$1 AND t.id=$2`, tenantID, taskID).Scan(&releaseIDs); err != nil {
		return nil, "", nil, err
	}
	if len(releaseIDs) == 0 {
		return nil, "", nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT r.id,r.package_id,p.name,r.version,r.archive_sha256,r.runtime_kind,r.archive_object_key,r.manifest_json FROM workforce_hook_package_releases r JOIN workforce_hook_packages p ON p.id=r.package_id WHERE r.id=ANY($1::BIGINT[]) AND ((r.scope_type='SYSTEM' AND r.tenant_id IS NULL) OR (r.scope_type='TENANT' AND r.tenant_id=$2))`, releaseIDs, tenantID)
	if err != nil {
		return nil, "", nil, err
	}
	defer rows.Close()
	type resolved struct {
		snapshot taskdomain.HookPackageReleaseSnapshot
		events   []string
	}
	byID := make(map[int64]resolved, len(releaseIDs))
	for rows.Next() {
		var item resolved
		var manifest struct {
			Entrypoints []struct {
				Event string `json:"event"`
			} `json:"entrypoints"`
		}
		var raw []byte
		if err = rows.Scan(&item.snapshot.ID, &item.snapshot.PackageID, &item.snapshot.Name, &item.snapshot.Version, &item.snapshot.Digest, &item.snapshot.RuntimeKind, &item.snapshot.ArchiveObjectKey, &raw); err != nil {
			return nil, "", nil, err
		}
		if json.Unmarshal(raw, &manifest) != nil {
			return nil, "", nil, errors.New("PROJECT_HOOK_RELEASE_INVALID")
		}
		for _, entry := range manifest.Entrypoints {
			item.events = append(item.events, entry.Event)
		}
		byID[item.snapshot.ID] = item
	}
	if err = rows.Err(); err != nil {
		return nil, "", nil, err
	}
	result := make([]taskdomain.HookPackageReleaseSnapshot, 0, len(releaseIDs))
	events := []string{}
	for _, releaseID := range releaseIDs {
		item, ok := byID[releaseID]
		if !ok {
			return nil, "", nil, errors.New("PROJECT_HOOK_RELEASE_NOT_FOUND")
		}
		result = append(result, item.snapshot)
		for _, event := range item.events {
			if !slices.Contains(events, event) {
				events = append(events, event)
			}
		}
	}
	body, _ := json.Marshal(result)
	return result, digestBytes(body), events, nil
}

func validRuntimeManifest(manifest runtimeManifest) bool {
	return strings.TrimSpace(manifest.Adapter.ID) != "" && strings.TrimSpace(manifest.Adapter.Version) != "" && manifest.Adapter.Protocol.Name == "seven.agent-adapter" && manifest.Adapter.Protocol.Min <= 1 && manifest.Adapter.Protocol.Max >= 1 && validHexDigest(manifest.ResultProcessor.PythonRuntimeIdentity) && validHexDigest(manifest.ResultProcessor.BootstrapDigest) && validHexDigest(manifest.ResultProcessor.SDKDigest) && validHexDigest(manifest.ResultProcessor.ModuleIndexDigest)
}

func containsAll(actual, required []string) bool {
	set := make(map[string]struct{}, len(actual))
	for _, item := range actual {
		set[strings.TrimSpace(item)] = struct{}{}
	}
	for _, item := range required {
		if _, ok := set[strings.TrimSpace(item)]; !ok {
			return false
		}
	}
	return true
}

func supportsRuntimeCapabilities(manifest runtimeManifest, required []string) bool {
	actual := make([]string, 0, len(manifest.Probe.Capabilities))
	for _, capability := range manifest.Probe.Capabilities {
		if strings.EqualFold(strings.TrimSpace(capability.Status), "supported") {
			actual = append(actual, strings.TrimSpace(capability.Name))
		}
	}
	return containsAll(actual, required)
}

func digestJSON(value any) string     { raw, _ := json.Marshal(value); return digestBytes(raw) }
func digestBytes(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }

var _ routingfacade.AssignmentResolver = (*Resolver)(nil)
