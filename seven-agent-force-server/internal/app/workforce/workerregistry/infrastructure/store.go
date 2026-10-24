package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workerregistry/domain"
)

type IDGenerator interface{ NextID() int64 }
type Store struct {
	db    *pgxpool.Pool
	idGen IDGenerator
}

func New(db *pgxpool.Pool, idGen ...IDGenerator) *Store {
	store := &Store{db: db}
	if len(idGen) > 0 {
		store.idGen = idGen[0]
	}
	return store
}

func (s *Store) Create(ctx context.Context, w domain.Worker, tokenID int64, tokenHash string, actor int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_workers (id,name,description,desired_state,max_active_attempts,max_active_reviews,max_total_containers,token_hint,version,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,1,$6,$7,1,$8)`, w.ID, w.Name, w.Description, w.DesiredState, w.MaxActiveAttempts, w.MaxActiveAttempts+1, w.TokenHint, principalID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_worker_tokens (id,worker_id,token_sha256,token_hint,status) VALUES ($1,$2,$3,$4,'ACTIVE')`, tokenID, w.ID, tokenHash, w.TokenHint); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RotateToken(ctx context.Context, workerID int64, tokenID int64, hash string, hint string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var activeProviderRuntimes int
	if err = tx.QueryRow(ctx, activeProviderRuntimeCountSQL, workerID).Scan(&activeProviderRuntimes); err != nil {
		return err
	}
	if activeProviderRuntimes > 0 {
		return application.ErrProviderRuntimeActive
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_worker_tokens SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP WHERE worker_id=$1 AND status='ACTIVE'`, workerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return application.ErrNotFound
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_worker_tokens (id,worker_id,token_sha256,token_hint,status) VALUES ($1,$2,$3,$4,'ACTIVE')`, tokenID, workerID, hash, hint); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_workers SET token_hint=$2,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, workerID, hint); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const activeProviderRuntimeCountSQL = `SELECT COUNT(*) FROM workforce_attempt_runtimes r
JOIN workforce_task_attempts a ON a.id=r.attempt_id
WHERE r.worker_id=$1
  AND COALESCE(NULLIF(a.resolved_execution_snapshot_json->>'modelCredentialId','')::BIGINT,0)>0
  AND r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','RECONCILIATION_REQUIRED','TERMINATING')`

func (s *Store) Authenticate(ctx context.Context, workerID int64) (string, string, error) {
	var hash, state string
	err := s.db.QueryRow(ctx, `SELECT t.token_sha256,w.desired_state FROM workforce_workers w JOIN workforce_worker_tokens t ON t.worker_id=w.id AND t.status='ACTIVE' WHERE w.id=$1`, workerID).Scan(&hash, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", application.ErrNotFound
	}
	return hash, state, err
}

func (s *Store) Register(ctx context.Context, c application.RegisterCommand) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var desired string
	if err = tx.QueryRow(ctx, `SELECT desired_state FROM workforce_workers WHERE id=$1 FOR UPDATE`, c.WorkerID).Scan(&desired); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	} else if err != nil {
		return err
	}
	if desired == domain.DesiredDisabled {
		return application.ErrUnauthorized
	}
	_, _ = tx.Exec(ctx, `UPDATE workforce_worker_sessions SET ended_at=CURRENT_TIMESTAMP,end_reason='SUPERSEDED' WHERE worker_id=$1 AND instance_id<>$2 AND ended_at IS NULL`, c.WorkerID, c.InstanceID)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_worker_sessions (instance_id,worker_id,host_boot_id,started_at,worker_version,worker_revision,worker_build_date,observed_remote_ip,max_active_attempts,capabilities_json,session_state) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,'REGISTERED') ON CONFLICT (instance_id) DO UPDATE SET observed_remote_ip=EXCLUDED.observed_remote_ip,capabilities_json=EXCLUDED.capabilities_json`, c.InstanceID, c.WorkerID, c.HostBootID, c.StartedAt, c.Version, c.Revision, c.BuildDate, c.ObservedRemoteIP, c.MaxActiveAttempts, c.Capabilities)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_workers SET current_instance_id=$2,max_active_reviews=$3,max_total_containers=$4,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, c.WorkerID, c.InstanceID, c.MaxActiveReviews, c.MaxTotalContainers)
	if err != nil {
		return err
	}
	var active int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_attempt_runtimes WHERE worker_id=$1 AND status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','RECONCILIATION_REQUIRED')`, c.WorkerID).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		deadline := time.Now().UTC().Add(2 * time.Minute)
		if _, err = tx.Exec(ctx, `UPDATE workforce_attempt_runtimes SET recovery_previous_status=CASE WHEN status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING') THEN status ELSE recovery_previous_status END,status='RECOVERING',recovery_started_at=COALESCE(recovery_started_at,CURRENT_TIMESTAMP),reattach_deadline=$2,watchdog_deadline=$2,updated_at=CURRENT_TIMESTAMP WHERE worker_id=$1 AND status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','RECONCILIATION_REQUIRED')`, c.WorkerID, deadline); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_worker_sessions SET session_state='RECONCILING',reconciliation_deadline=$2 WHERE instance_id=$1`, c.InstanceID, deadline); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) RecoverableRuntimes(ctx context.Context, workerID int64, instanceID string) ([]domain.RecoverableRuntime, error) {
	rows, err := s.db.Query(ctx, recoverableRuntimesSQL, workerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.RecoverableRuntime, 0, 8)
	for rows.Next() {
		var item domain.RecoverableRuntime
		if err := rows.Scan(&item.CancellationRequested, &item.TaskID, &item.AttemptID, &item.ExecutionNodeID, &item.AssignmentID, &item.Generation, &item.OwnerEpoch, &item.HostBootID, &item.ExpectedContainerName, &item.ContainerID, &item.RuntimeStatus, &item.ExecutionIdentityDigest, &item.HarnessImageDigest, &item.HookConfigurationDigest, &item.ProviderBacked, &item.ProviderProxyContainerID, &item.ProviderProxyContainerName, &item.ProviderProxyImageDigest, &item.ProviderProxyIdentityDigest, &item.ReattachDeadline); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		_, _ = s.db.Exec(ctx, `UPDATE workforce_worker_sessions SET session_state='REGISTERED',reconciliation_deadline=NULL WHERE instance_id=$1`, instanceID)
	}
	return items, nil
}

const recoverableRuntimesSQL = `SELECT (t.status IN ('CANCELLING','CANCELLED')),
       a.task_id,r.attempt_id,r.execution_node_id,r.assignment_id,r.generation,r.owner_epoch,
       r.host_boot_id,r.expected_container_name,COALESCE(r.container_id,''),
       COALESCE(r.recovery_previous_status,r.status),r.execution_identity_digest,
       r.harness_image_digest,r.hook_configuration_digest,
       (COALESCE(NULLIF(a.resolved_execution_snapshot_json->>'modelCredentialId','')::BIGINT,0)>0),
       COALESCE(r.provider_proxy_container_id,''),COALESCE(r.provider_proxy_container_name,''),
       r.provider_proxy_image_digest,r.provider_proxy_execution_identity_digest,
       COALESCE(to_char(r.reattach_deadline AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'')
FROM workforce_attempt_runtimes r
JOIN workforce_task_attempts a ON a.id=r.attempt_id
JOIN workforce_tasks t ON t.id=a.task_id
WHERE r.worker_id=$1
  AND r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','RECONCILIATION_REQUIRED')
ORDER BY r.created_at`

func (s *Store) Heartbeat(ctx context.Context, c application.HeartbeatCommand) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current string
	if err = tx.QueryRow(ctx, `SELECT COALESCE(current_instance_id,'') FROM workforce_workers WHERE id=$1 FOR UPDATE`, c.WorkerID).Scan(&current); err != nil {
		return err
	}
	if current != c.InstanceID {
		return application.ErrStaleInstance
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_worker_sessions SET last_heartbeat_at=CURRENT_TIMESTAMP,heartbeat_sequence=$4,consumer_state=$5,docker_ready=$6,active_attempts=$7,max_active_attempts=$8,hostname=$9,reported_ip=$10,primary_interface=$11,primary_mac=$12,interfaces_json=$13,resource_snapshot_json=$14 WHERE worker_id=$1 AND instance_id=$2 AND host_boot_id=$3 AND ended_at IS NULL AND heartbeat_sequence<$4`, c.WorkerID, c.InstanceID, c.HostBootID, c.Sequence, c.ConsumerState, c.DockerReady, c.ActiveAttempts, c.MaxActiveAttempts, c.Hostname, c.ReportedIP, c.PrimaryInterface, c.PrimaryMAC, c.Interfaces, c.ResourceSnapshot)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return application.ErrConflict
	}
	return tx.Commit(ctx)
}

func (s *Store) RecordMetricSample(ctx context.Context, sample application.MetricSample) error {
	_, err := s.db.Exec(ctx, `INSERT INTO workforce_worker_metric_samples (
		worker_id,sampled_at,logical_cpu_cores,cpu_usage_percent,load1,
		memory_used_bytes,memory_total_bytes,memory_usage_percent,
		workspace_disk_used_bytes,workspace_disk_total_bytes,workspace_disk_usage_percent,
		active_attempts,max_active_attempts,docker_ready
	) VALUES ($1,date_trunc('minute',CURRENT_TIMESTAMP),$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	ON CONFLICT (worker_id,sampled_at) DO NOTHING`, sample.WorkerID, sample.LogicalCPUCores, sample.CPUUsagePercent, sample.Load1, sample.MemoryUsedBytes, sample.MemoryTotalBytes, sample.MemoryUsagePercent, sample.WorkspaceDiskUsedBytes, sample.WorkspaceDiskTotalBytes, sample.WorkspaceDiskUsagePercent, sample.ActiveAttempts, sample.MaxActiveAttempts, sample.DockerReady)
	return err
}

func (s *Store) QueryMetricSeries(ctx context.Context, workerID *int64, from, to time.Time, stepSeconds int) ([]domain.MetricPoint, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if workerID != nil {
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_workers WHERE id=$1)`, *workerID).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, application.ErrNotFound
		}
		rows, err = s.db.Query(ctx, singleWorkerMetricSeriesSQL, from, to, stepSeconds, *workerID)
	} else {
		rows, err = s.db.Query(ctx, fleetMetricSeriesSQL, from, to, stepSeconds)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	points := make([]domain.MetricPoint, 0)
	for rows.Next() {
		var point domain.MetricPoint
		if err = rows.Scan(&point.At, &point.CPUUsagePercent, &point.MemoryUsagePercent, &point.WorkspaceDiskUsagePercent, &point.ActiveAttempts, &point.MaxActiveAttempts); err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

const singleWorkerMetricSeriesSQL = `SELECT
	to_timestamp(floor(extract(epoch FROM sampled_at)/$3)*$3) AS bucket,
	AVG(cpu_usage_percent)::DOUBLE PRECISION,
	AVG(memory_usage_percent)::DOUBLE PRECISION,
	AVG(workspace_disk_usage_percent)::DOUBLE PRECISION,
	MAX(active_attempts)::INTEGER,
	MAX(max_active_attempts)::INTEGER
FROM workforce_worker_metric_samples
WHERE sampled_at >= $1 AND sampled_at < $2 AND worker_id=$4
GROUP BY bucket ORDER BY bucket`

const fleetMetricSeriesSQL = `WITH minute_fleet AS (
	SELECT sampled_at,
		SUM(cpu_usage_percent*logical_cpu_cores) FILTER (WHERE cpu_usage_percent IS NOT NULL) / NULLIF(SUM(logical_cpu_cores) FILTER (WHERE cpu_usage_percent IS NOT NULL),0) AS cpu_usage_percent,
		100.0*SUM(memory_used_bytes)/NULLIF(SUM(memory_total_bytes),0) AS memory_usage_percent,
		100.0*SUM(workspace_disk_used_bytes)/NULLIF(SUM(workspace_disk_total_bytes),0) AS workspace_disk_usage_percent,
		SUM(active_attempts) AS active_attempts,
		SUM(max_active_attempts) AS max_active_attempts
	FROM workforce_worker_metric_samples
	WHERE sampled_at >= $1 AND sampled_at < $2
	GROUP BY sampled_at
)
SELECT to_timestamp(floor(extract(epoch FROM sampled_at)/$3)*$3) AS bucket,
	AVG(cpu_usage_percent)::DOUBLE PRECISION,
	AVG(memory_usage_percent)::DOUBLE PRECISION,
	AVG(workspace_disk_usage_percent)::DOUBLE PRECISION,
	MAX(active_attempts)::INTEGER,
	MAX(max_active_attempts)::INTEGER
FROM minute_fleet GROUP BY bucket ORDER BY bucket`

func (s *Store) DeleteMetricSamplesBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
	tag, err := s.db.Exec(ctx, `WITH expired AS (
		SELECT worker_id,sampled_at FROM workforce_worker_metric_samples WHERE sampled_at<$1 ORDER BY sampled_at LIMIT $2
	) DELETE FROM workforce_worker_metric_samples samples USING expired
	WHERE samples.worker_id=expired.worker_id AND samples.sampled_at=expired.sampled_at`, before, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) PublishInventory(ctx context.Context, c application.InventoryCommand) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var current, boot string
	if err = tx.QueryRow(ctx, `SELECT w.current_instance_id,s.host_boot_id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.id=$1 FOR UPDATE`, c.WorkerID).Scan(&current, &boot); err != nil {
		return err
	}
	if current != c.InstanceID || boot != c.HostBootID {
		return application.ErrStaleInstance
	}
	var version int64
	var digest string
	err = tx.QueryRow(ctx, `SELECT inventory_version,inventory_digest FROM workforce_worker_images WHERE worker_id=$1 ORDER BY inventory_version DESC,last_seen_at DESC LIMIT 1`, c.WorkerID).Scan(&version, &digest)
	if errors.Is(err, pgx.ErrNoRows) {
		version, digest, err = 0, "", nil
	}
	if err != nil {
		return err
	}
	if c.InventoryVersion < version || (c.InventoryVersion == version && digest != "" && digest != c.InventoryDigest) {
		return application.ErrConflict
	}
	if c.InventoryVersion == version && digest == c.InventoryDigest {
		return nil
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_worker_images SET presence_status='MISSING',missing_since=COALESCE(missing_since,CURRENT_TIMESTAMP),last_seen_at=last_seen_at WHERE worker_id=$1 AND presence_status NOT IN ('MISSING','DRIFTED')`, c.WorkerID); err != nil {
		return err
	}
	for _, image := range c.Images {
		// A ref which now resolves to another digest is drift, not merely absence.
		// The old row is intentionally retained as execution-history evidence.
		_, err = tx.Exec(ctx, `UPDATE workforce_worker_images SET presence_status='DRIFTED',missing_since=COALESCE(missing_since,CURRENT_TIMESTAMP),drift_reason='IMAGE_REF_RESOLVED_TO_NEW_DIGEST' WHERE worker_id=$1 AND image_ref=$2 AND role=$3 AND image_digest<>$4`, c.WorkerID, image.ImageRef, image.Role, image.ImageDigest)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, publishInventoryImageSQL, c.WorkerID, c.InstanceID, image.ImageDigest, image.ImageRef, image.Role, image.ContractVersion, image.ManifestDigest, image.Manifest, image.Capabilities, image.SchedulingScope, image.SchedulingPriority, image.EnvironmentCapabilities, image.ExecutionKinds, image.SchedulingDigest, image.AdmissionStatus, image.AdmissionEvidence, c.InventoryVersion, c.InventoryDigest)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

const publishInventoryImageSQL = `INSERT INTO workforce_worker_images (worker_id,instance_id,image_digest,image_ref,role,contract_version,manifest_digest,manifest_json,capabilities_json,scheduling_scope,scheduling_priority,environment_capabilities_json,execution_kinds_json,scheduling_digest,admission_status,admission_evidence_json,presence_status,missing_since,drift_reason,inventory_version,inventory_digest,last_seen_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,'PRESENT',NULL,'',$17,$18,CURRENT_TIMESTAMP)
ON CONFLICT (worker_id,image_digest,role) DO UPDATE SET
instance_id=EXCLUDED.instance_id,image_ref=EXCLUDED.image_ref,contract_version=EXCLUDED.contract_version,
manifest_digest=EXCLUDED.manifest_digest,manifest_json=EXCLUDED.manifest_json,capabilities_json=EXCLUDED.capabilities_json,
scheduling_scope=EXCLUDED.scheduling_scope,scheduling_priority=EXCLUDED.scheduling_priority,
environment_capabilities_json=EXCLUDED.environment_capabilities_json,execution_kinds_json=EXCLUDED.execution_kinds_json,
scheduling_digest=EXCLUDED.scheduling_digest,admission_status=EXCLUDED.admission_status,
admission_evidence_json=EXCLUDED.admission_evidence_json,presence_status='PRESENT',missing_since=NULL,drift_reason='',
inventory_version=EXCLUDED.inventory_version,inventory_digest=EXCLUDED.inventory_digest,last_seen_at=CURRENT_TIMESTAMP`

func (s *Store) Deregister(ctx context.Context, workerID int64, instanceID string, reason string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE workforce_worker_sessions SET ended_at=CURRENT_TIMESTAMP,end_reason=$3,consumer_state='STOPPED' WHERE worker_id=$1 AND instance_id=$2 AND ended_at IS NULL`, workerID, instanceID, truncate(reason, 200))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return application.ErrStaleInstance
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_workers SET current_instance_id=NULL,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND current_instance_id=$2`, workerID, instanceID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) SetDesiredState(ctx context.Context, id int64, state string, version int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentState, instance string
	var currentVersion int64
	if err = tx.QueryRow(ctx, `SELECT desired_state,COALESCE(current_instance_id,''),version FROM workforce_workers WHERE id=$1 FOR UPDATE`, id).Scan(&currentState, &instance, &currentVersion); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	} else if err != nil {
		return err
	}
	if currentVersion != version {
		return application.ErrConflict
	}
	if state == domain.DesiredDisabled {
		var active int
		if err = tx.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM workforce_task_assignments WHERE worker_id=$1 AND status IN ('RESERVED','PUBLISHED','CLAIMED')) + (SELECT COUNT(*) FROM workforce_attempt_runtimes WHERE worker_id=$1 AND status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECONCILIATION_REQUIRED'))`, id).Scan(&active); err != nil {
			return err
		}
		consumer := "STOPPED"
		heartbeatFresh := false
		if instance != "" {
			if err = tx.QueryRow(ctx, `SELECT consumer_state,last_heartbeat_at >= CURRENT_TIMESTAMP-INTERVAL '35 seconds' FROM workforce_worker_sessions WHERE instance_id=$1`, instance).Scan(&consumer, &heartbeatFresh); err != nil {
				return err
			}
		}
		if !workerCanDisable(currentState, instance, consumer, heartbeatFresh, active) {
			return application.ErrNotDrained
		}
		if instance != "" {
			_, _ = tx.Exec(ctx, `UPDATE workforce_worker_sessions SET ended_at=CURRENT_TIMESTAMP,end_reason='DISABLED' WHERE instance_id=$1 AND ended_at IS NULL`, instance)
		}
	}
	if state == domain.DesiredEnabled && currentState == domain.DesiredDisabled {
		return application.ErrConflict
	}
	currentInstance := instance
	if state == domain.DesiredDisabled {
		currentInstance = ""
	}
	_, err = tx.Exec(ctx, `UPDATE workforce_workers SET desired_state=$2,current_instance_id=NULLIF($3,''),version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id, state, currentInstance)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) UpdateCapacity(ctx context.Context, id int64, maximum int, version int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentVersion int64
	var currentInstance string
	if err = tx.QueryRow(ctx, `SELECT version,COALESCE(current_instance_id,'') FROM workforce_workers WHERE id=$1 AND decommissioned_at IS NULL FOR UPDATE`, id).Scan(&currentVersion, &currentInstance); errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	} else if err != nil {
		return err
	}
	if currentVersion != version {
		return application.ErrConflict
	}
	if currentInstance != "" {
		var reported int
		if err = tx.QueryRow(ctx, `SELECT max_active_attempts FROM workforce_worker_sessions WHERE instance_id=$1`, currentInstance).Scan(&reported); err != nil {
			return err
		}
		if maximum > reported {
			return application.ErrCapacityExceedsWorker
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE workforce_workers SET max_active_attempts=$2,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND version=$3`, id, maximum, version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return application.ErrConflict
	}
	return tx.Commit(ctx)
}

func workerCanDisable(currentDesired, instance, consumer string, heartbeatFresh bool, active int) bool {
	if active != 0 {
		return false
	}
	if instance == "" || !heartbeatFresh {
		return true
	}
	return currentDesired == domain.DesiredDraining && consumer == "STOPPED"
}

func (s *Store) RequestInventoryRefresh(ctx context.Context, id int64) (int64, error) {
	var generation int64
	err := s.db.QueryRow(ctx, `UPDATE workforce_workers SET inventory_refresh_generation=inventory_refresh_generation+1,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND decommissioned_at IS NULL RETURNING inventory_refresh_generation`, id).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, application.ErrNotFound
	}
	return generation, err
}

func (s *Store) Decommission(ctx context.Context, id int64, version int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var desired, instance string
	var currentVersion int64
	if err = tx.QueryRow(ctx, `SELECT desired_state,COALESCE(current_instance_id,''),version FROM workforce_workers WHERE id=$1 FOR UPDATE`, id).Scan(&desired, &instance, &currentVersion); err != nil {
		return err
	}
	if desired != domain.DesiredDisabled || instance != "" || currentVersion != version {
		return application.ErrConflict
	}
	var assignments, runtimes, finalizations, uploads int
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_task_assignments WHERE worker_id=$1 AND status IN ('RESERVED','PUBLISHED','CLAIMED')`, id).Scan(&assignments); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_attempt_runtimes WHERE worker_id=$1 AND status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECOVERING','REATTACHING','TERMINATING','RECONCILIATION_REQUIRED')`, id).Scan(&runtimes); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_finalization_runs f WHERE f.worker_id=$1 AND (f.status IN ('PENDING','RUNNING','RETRY_WAIT') OR (f.status='FROZEN' AND EXISTS (SELECT 1 FROM workforce_artifact_upload_attempts u WHERE u.finalization_run_id=f.id AND u.status<>'VERIFIED')))`, id).Scan(&finalizations); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_artifact_upload_attempts WHERE worker_id=$1 AND status IN ('PENDING','UPLOADING','VERIFYING','RETRY_WAIT')`, id).Scan(&uploads); err != nil {
		return err
	}
	if assignments+runtimes+finalizations+uploads != 0 {
		return application.ErrConflict
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_worker_tokens SET status='REVOKED',revoked_at=CURRENT_TIMESTAMP WHERE worker_id=$1 AND status='ACTIVE'`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_workers SET decommissioned_at=CURRENT_TIMESTAMP,version=version+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) List(ctx context.Context, limit int) ([]domain.Worker, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, workerSelect+` ORDER BY w.created_at DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]workerRow, 0)
	for rows.Next() {
		v, err := scanWorkerRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	imageStats := map[int64]struct {
		attempt   int
		inventory int64
	}{}
	imageRows, err := s.db.Query(ctx, `SELECT worker_id,COUNT(*) FILTER (WHERE role='attempt' AND admission_status='ADMITTED' AND presence_status='PRESENT'),COALESCE(MAX(inventory_version),0) FROM workforce_worker_images GROUP BY worker_id`)
	if err != nil {
		return nil, err
	}
	for imageRows.Next() {
		var workerID int64
		var attempt int
		var inventory int64
		if err = imageRows.Scan(&workerID, &attempt, &inventory); err != nil {
			imageRows.Close()
			return nil, err
		}
		imageStats[workerID] = struct {
			attempt   int
			inventory int64
		}{attempt: attempt, inventory: inventory}
	}
	if err = imageRows.Err(); err != nil {
		imageRows.Close()
		return nil, err
	}
	imageRows.Close()
	capacityByWorker, err := s.loadCapacities(ctx)
	if err != nil {
		return nil, err
	}
	postprocessByWorker, err := s.loadPostprocessCounts(ctx)
	if err != nil {
		return nil, err
	}
	workers := make([]domain.Worker, 0, len(result))
	for _, row := range result {
		stats := imageStats[row.ID]
		row.ImageCount = stats.attempt
		row.InventoryVersion = stats.inventory
		row.PendingFinalizations = postprocessByWorker[row.ID].finalizations
		row.PendingArtifactUploads = postprocessByWorker[row.ID].uploads
		applyCapacity(&row.Worker, capacityByWorker[row.ID])
		row.State = deriveState(row.Worker, row.sessionEnded, row.heartbeatFresh, stats.attempt > 0)
		workers = append(workers, row.Worker)
	}
	return workers, nil
}

type postprocessCounts struct {
	finalizations int
	uploads       int
}

func (s *Store) loadPostprocessCounts(ctx context.Context) (map[int64]postprocessCounts, error) {
	result := make(map[int64]postprocessCounts)
	rows, err := s.db.Query(ctx, `SELECT worker_id,COUNT(*) FROM workforce_finalization_runs WHERE status IN ('PENDING','RUNNING','RETRY_WAIT') GROUP BY worker_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var workerID int64
		var count int
		if err = rows.Scan(&workerID, &count); err != nil {
			rows.Close()
			return nil, err
		}
		value := result[workerID]
		value.finalizations = count
		result[workerID] = value
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	rows, err = s.db.Query(ctx, `SELECT worker_id,COUNT(*) FROM workforce_artifact_upload_attempts WHERE status IN ('PENDING','UPLOADING','VERIFYING','RETRY_WAIT') GROUP BY worker_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var workerID int64
		var count int
		if err = rows.Scan(&workerID, &count); err != nil {
			return nil, err
		}
		value := result[workerID]
		value.uploads = count
		result[workerID] = value
	}
	return result, rows.Err()
}

func (s *Store) loadPostprocessCountsForWorker(ctx context.Context, workerID int64) (postprocessCounts, error) {
	var result postprocessCounts
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_finalization_runs WHERE worker_id=$1 AND status IN ('PENDING','RUNNING','RETRY_WAIT')`, workerID).Scan(&result.finalizations); err != nil {
		return result, err
	}
	if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_artifact_upload_attempts WHERE worker_id=$1 AND status IN ('PENDING','UPLOADING','VERIFYING','RETRY_WAIT')`, workerID).Scan(&result.uploads); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) Get(ctx context.Context, id int64) (domain.Worker, []domain.Image, error) {
	row, err := scanWorkerRow(s.db.QueryRow(ctx, workerSelect+` WHERE w.id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Worker{}, nil, application.ErrNotFound
	}
	if err != nil {
		return domain.Worker{}, nil, err
	}
	rows, err := s.db.Query(ctx, workerImagesSelectSQL, id)
	if err != nil {
		return domain.Worker{}, nil, err
	}
	defer rows.Close()
	images := []domain.Image{}
	admittedAttempts := 0
	var inventoryVersion int64
	for rows.Next() {
		var i domain.Image
		if err := rows.Scan(&i.ImageDigest, &i.ImageRef, &i.Role, &i.ContractVersion, &i.ManifestDigest, &i.Manifest, &i.Capabilities, &i.SchedulingScope, &i.SchedulingPriority, &i.EnvironmentCapabilities, &i.ExecutionKinds, &i.SchedulingDigest, &i.AdmissionStatus, &i.AdmissionEvidence, &i.InventoryVersion, &i.PresenceStatus, &i.MissingSince, &i.DriftReason); err != nil {
			return domain.Worker{}, nil, err
		}
		// Only an admitted Attempt image owns a trusted executable manifest.
		// Rejected discovery rows intentionally retain empty/untrusted manifest
		// evidence and are fenced from routing by admission_status.
		if i.Role == "attempt" && i.AdmissionStatus == "ADMITTED" {
			resolved, schedulingErr := application.ParseManifestScheduling(i.Manifest)
			if schedulingErr != nil || resolved.Digest != i.SchedulingDigest || resolved.Scope != i.SchedulingScope || resolved.Priority != i.SchedulingPriority || !sameStringArrayJSON(resolved.EnvironmentCapabilitiesJSON, i.EnvironmentCapabilities) || !sameStringArrayJSON(resolved.ExecutionKindsJSON, i.ExecutionKinds) {
				return domain.Worker{}, nil, application.ErrInventorySchedulingDrift
			}
		}
		if i.Role == "attempt" && i.AdmissionStatus == "ADMITTED" && i.PresenceStatus == "PRESENT" {
			admittedAttempts++
		}
		if i.InventoryVersion > inventoryVersion {
			inventoryVersion = i.InventoryVersion
		}
		images = append(images, i)
	}
	if err = rows.Err(); err != nil {
		return domain.Worker{}, nil, err
	}
	var latestInventoryVersion int64
	if err = s.db.QueryRow(ctx, `SELECT COALESCE(MAX(inventory_version),0) FROM workforce_worker_images WHERE worker_id=$1`, id).Scan(&latestInventoryVersion); err != nil {
		return domain.Worker{}, nil, err
	}
	if latestInventoryVersion > inventoryVersion {
		inventoryVersion = latestInventoryVersion
	}
	row.ImageCount = admittedAttempts
	row.InventoryVersion = inventoryVersion
	counts, err := s.loadPostprocessCountsForWorker(ctx, id)
	if err != nil {
		return domain.Worker{}, nil, err
	}
	row.PendingFinalizations, row.PendingArtifactUploads = counts.finalizations, counts.uploads
	if err = s.populateCapacity(ctx, &row.Worker); err != nil {
		return domain.Worker{}, nil, err
	}
	row.State = deriveState(row.Worker, row.sessionEnded, row.heartbeatFresh, admittedAttempts > 0)
	return row.Worker, images, nil
}

const workerImagesSelectSQL = `SELECT image_digest,image_ref,role,contract_version,manifest_digest,manifest_json,capabilities_json,
scheduling_scope,scheduling_priority,environment_capabilities_json,execution_kinds_json,scheduling_digest,
admission_status,admission_evidence_json,inventory_version,presence_status,
COALESCE(to_char(missing_since AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),drift_reason
FROM workforce_worker_images
WHERE worker_id=$1
  AND instance_id=(SELECT current_instance_id FROM workforce_workers WHERE id=$1)
ORDER BY role,image_ref`

func sameStringArrayJSON(left, right []byte) bool {
	var leftValues, rightValues []string
	if json.Unmarshal(left, &leftValues) != nil || json.Unmarshal(right, &rightValues) != nil || len(leftValues) != len(rightValues) {
		return false
	}
	for index := range leftValues {
		if leftValues[index] != rightValues[index] {
			return false
		}
	}
	return true
}

func (s *Store) ListAssignments(ctx context.Context, workerID int64, limit int) ([]domain.Assignment, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	rows, err := s.db.Query(ctx, `SELECT a.id,a.task_id,a.execution_node_id,a.worker_instance_id,a.generation,a.execution_requirement_digest,a.status,a.message_id,COALESCE(rt.attempt_id,0),to_char(a.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(a.claimed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(a.completed_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_task_assignments a LEFT JOIN workforce_attempt_runtimes rt ON rt.assignment_id=a.id WHERE a.worker_id=$1 ORDER BY a.created_at DESC LIMIT $2`, workerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Assignment, 0)
	for rows.Next() {
		var item domain.Assignment
		if err := rows.Scan(&item.ID, &item.TaskID, &item.ExecutionNodeID, &item.WorkerInstanceID, &item.Generation, &item.ExecutionRequirementDigest, &item.Status, &item.MessageID, &item.AttemptID, &item.CreatedAt, &item.ClaimedAt, &item.CompletedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

const workerSelect = `SELECT w.id,w.name,w.description,w.desired_state,w.max_active_attempts,COALESCE(s.max_active_attempts,0),COALESCE(w.current_instance_id,''),COALESCE(s.host_boot_id,''),COALESCE(s.worker_version,''),COALESCE(s.worker_revision,''),COALESCE(s.worker_build_date,''),w.token_hint,w.inventory_refresh_generation,0::bigint,COALESCE(to_char(w.decommissioned_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),w.version,to_char(w.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(s.observed_remote_ip,''),COALESCE(s.reported_ip,''),COALESCE(s.hostname,''),COALESCE(s.primary_interface,''),COALESCE(s.primary_mac,''),COALESCE(s.consumer_state,''),COALESCE(s.session_state,''),COALESCE(to_char(s.reconciliation_deadline AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(s.docker_ready,false),COALESCE(s.active_attempts,0),COALESCE(to_char(s.last_heartbeat_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(s.resource_snapshot_json,'{}'),COALESCE(s.interfaces_json,'[]'),0::integer,0::integer,COALESCE(s.ended_at IS NOT NULL,false),COALESCE(s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds',false) FROM workforce_workers w LEFT JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id`

type scanner interface{ Scan(...any) error }

type workerRow struct {
	domain.Worker
	sessionEnded   bool
	heartbeatFresh bool
}

func scanWorkerRow(r scanner) (workerRow, error) {
	var row workerRow
	w := &row.Worker
	err := r.Scan(&w.ID, &w.Name, &w.Description, &w.DesiredState, &w.MaxActiveAttempts, &w.ReportedMaxActiveAttempts, &w.CurrentInstanceID, &w.HostBootID, &w.WorkerVersion, &w.WorkerRevision, &w.WorkerBuildDate, &w.TokenHint, &w.InventoryRefreshGeneration, &w.InventoryVersion, &w.DecommissionedAt, &w.Version, &w.CreatedAt, &w.ObservedRemoteIP, &w.ReportedIP, &w.Hostname, &w.PrimaryInterface, &w.PrimaryMAC, &w.ConsumerState, &w.SessionState, &w.ReconciliationDeadline, &w.DockerReady, &w.ActiveAttempts, &w.LastHeartbeatAt, &w.ResourceSnapshot, &w.Interfaces, &w.PendingFinalizations, &w.PendingArtifactUploads, &row.sessionEnded, &row.heartbeatFresh)
	return row, err
}

func deriveState(w domain.Worker, sessionEnded, heartbeatFresh, hasAdmittedAttempt bool) string {
	switch {
	case w.DecommissionedAt != "":
		return domain.StateDecommissioned
	case w.DesiredState == domain.DesiredDisabled:
		return domain.StateDisabled
	case w.DesiredState == domain.DesiredDraining && w.ReservedAttempts == 0 && w.StartingAttempts == 0 && w.RunningAttempts == 0 && w.ConsumerState == "STOPPED":
		return domain.StateDrained
	case w.DesiredState == domain.DesiredDraining:
		return domain.StateDraining
	case w.CurrentInstanceID == "":
		return domain.StateProvisioned
	case sessionEnded:
		return domain.StateOffline
	case w.SessionState == "RECONCILING":
		return domain.StateReconciling
	case !heartbeatFresh:
		return domain.StateStale
	case w.ConsumerState == "CONSUMING" && w.DockerReady && hasAdmittedAttempt:
		return domain.StateOnline
	default:
		return domain.StateOffline
	}
}

func (s *Store) populateCapacity(ctx context.Context, w *domain.Worker) error {
	err := s.db.QueryRow(ctx, `SELECT
	COUNT(*) FILTER (WHERE a.status IN ('RESERVED','PUBLISHED') OR (a.status='CLAIMED' AND r.attempt_id IS NULL)),
	COUNT(*) FILTER (WHERE r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING')),
	COUNT(*) FILTER (WHERE r.status='RUNNING')
	FROM workforce_workers base
	LEFT JOIN workforce_task_assignments a ON a.worker_id=base.id AND a.status IN ('RESERVED','PUBLISHED','CLAIMED')
	LEFT JOIN workforce_attempt_runtimes r ON r.assignment_id=a.id AND r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECONCILIATION_REQUIRED')
	WHERE base.id=$1 GROUP BY base.id`, w.ID).Scan(&w.ReservedAttempts, &w.StartingAttempts, &w.RunningAttempts)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	used := w.ReservedAttempts + w.StartingAttempts + w.RunningAttempts
	w.AvailableAttempts = effectiveWorkerCapacity(*w) - used
	if w.AvailableAttempts < 0 {
		w.AvailableAttempts = 0
	}
	return err
}

type workerCapacity struct {
	reserved int
	starting int
	running  int
}

func (s *Store) loadCapacities(ctx context.Context) (map[int64]workerCapacity, error) {
	rows, err := s.db.Query(ctx, `SELECT base.id,
	COUNT(*) FILTER (WHERE a.status IN ('RESERVED','PUBLISHED') OR (a.status='CLAIMED' AND r.attempt_id IS NULL)),
	COUNT(*) FILTER (WHERE r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING')),
	COUNT(*) FILTER (WHERE r.status='RUNNING')
	FROM workforce_workers base
	LEFT JOIN workforce_task_assignments a ON a.worker_id=base.id AND a.status IN ('RESERVED','PUBLISHED','CLAIMED')
	LEFT JOIN workforce_attempt_runtimes r ON r.assignment_id=a.id AND r.status IN ('AUTHORIZED','CONTAINER_CREATED','SUPERVISOR_READY','STARTING','RUNNING','RECONCILIATION_REQUIRED')
	GROUP BY base.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[int64]workerCapacity)
	for rows.Next() {
		var workerID int64
		var capacity workerCapacity
		if err := rows.Scan(&workerID, &capacity.reserved, &capacity.starting, &capacity.running); err != nil {
			return nil, err
		}
		result[workerID] = capacity
	}
	return result, rows.Err()
}

func applyCapacity(w *domain.Worker, capacity workerCapacity) {
	w.ReservedAttempts = capacity.reserved
	w.StartingAttempts = capacity.starting
	w.RunningAttempts = capacity.running
	w.AvailableAttempts = effectiveWorkerCapacity(*w) - capacity.reserved - capacity.starting - capacity.running
	if w.AvailableAttempts < 0 {
		w.AvailableAttempts = 0
	}
}

func effectiveWorkerCapacity(w domain.Worker) int {
	if w.ReportedMaxActiveAttempts > 0 && w.ReportedMaxActiveAttempts < w.MaxActiveAttempts {
		return w.ReportedMaxActiveAttempts
	}
	return w.MaxActiveAttempts
}
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n]
	}
	return s
}

var _ = json.Valid
var _ = time.Second
