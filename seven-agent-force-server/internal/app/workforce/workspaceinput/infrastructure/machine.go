package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	gitdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

func (s *Store) ClaimIngestion(ctx context.Context, a application.MachineAuthority, runID int64) (application.SourceIngestionClaim, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.SourceIngestionClaim{}, err
	}
	defer tx.Rollback(ctx)
	var claim application.SourceIngestionClaim
	var status, instance, boot string
	var worker, projectID, projectVersionId int64
	err = tx.QueryRow(ctx, `SELECT id,workspace_source_revision_id,kind,attempt_number,max_attempts,worker_id,worker_instance_id,host_boot_id,COALESCE(project_id,0),COALESCE(project_version_id,0),requested_ref_type,requested_ref_name,frozen_commit,input_object_key,input_content_sha256,input_object_sha256,input_size_bytes,status FROM workforce_source_ingestion_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&claim.RunID, &claim.SourceRevisionID, &claim.Kind, &claim.AttemptNumber, new(int), &worker, &instance, &boot, &projectID, &projectVersionId, &claim.RequestedRefType, &claim.RequestedRefName, &claim.FrozenCommit, &claim.InputObjectKey, &claim.InputContentSHA256, &claim.InputObjectSHA256, &claim.InputSizeBytes, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return claim, application.ErrNotFound
	}
	if err != nil {
		return claim, err
	}
	if status != "PENDING" && status != "RETRY_WAIT" || worker != a.WorkerID || a.Generation != 1 {
		return claim, domain.ErrTaskInputInvalid
	}
	var current string
	if err = tx.QueryRow(ctx, `SELECT current_instance_id FROM workforce_workers WHERE id=$1 AND decommissioned_at IS NULL`, worker).Scan(&current); err != nil || current != a.InstanceID {
		return claim, domain.ErrTaskInputInvalid
	}
	claim.AttemptNumber++
	claim.Generation = 1
	if projectID != 0 {
		claim.Repository, err = connectionForMachine(ctx, tx, projectID, projectVersionId)
		if err != nil || gitdomain.RequireCapability(claim.Repository, gitdomain.CapabilityReadSource) != nil {
			return claim, domain.ErrTaskInputInvalid
		}
	}
	lease := randomMachineToken()
	expires := time.Now().UTC().Add(5 * time.Minute)
	command, err := tx.Exec(ctx, `UPDATE workforce_source_ingestion_runs SET attempt_number=$2,worker_instance_id=$3,host_boot_id=$4,lease_sha256=$5,lease_expires_at=$6,status='RUNNING',reason_code='',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, claim.AttemptNumber, a.InstanceID, a.HostBootID, machineSecretHash(lease), expires)
	if err != nil || command.RowsAffected() != 1 {
		return claim, err
	}
	if err = tx.Commit(ctx); err != nil {
		return claim, err
	}
	claim.LeaseToken = lease
	claim.LeaseExpiresAt = expires.Format(time.RFC3339Nano)
	claim.BundleObjectKey = fmt.Sprintf("workspace-sources/%d/repository.bundle", claim.SourceRevisionID)
	return claim, nil
}

func (s *Store) FreezeSourceCommit(ctx context.Context, a application.MachineAuthority, runID int64, lease string, commit string) error {
	if !hexIdentity(commit, 40) {
		return domain.ErrTaskInputInvalid
	}
	command, err := s.db.Exec(ctx, `UPDATE workforce_source_ingestion_runs SET frozen_commit=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND worker_id=$2 AND worker_instance_id=$4 AND host_boot_id=$5 AND status='RUNNING' AND lease_sha256=$6 AND lease_expires_at>CURRENT_TIMESTAMP AND (frozen_commit='' OR frozen_commit=$3)`, runID, a.WorkerID, commit, a.InstanceID, a.HostBootID, machineSecretHash(lease))
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return domain.ErrTaskInputInvalid
	}
	return nil
}

func (s *Store) ResolveCredentialScope(ctx context.Context, a application.MachineAuthority, runID int64, lease string) (application.EncryptedRepositoryCredential, error) {
	var value application.EncryptedRepositoryCredential
	err := s.db.QueryRow(ctx, `SELECT COALESCE(encode(v.repository_credential_ciphertext,'base64'),''),COALESCE(encode(v.repository_credential_edek,'base64'),''),COALESCE(v.repository_credential_wrap_key_ref,''),COALESCE(v.repository_credential_version,0)
		FROM workforce_source_ingestion_runs r JOIN workforce_project_versions v ON v.project_id=r.project_id AND v.tenant_id=r.tenant_id AND v.id=r.project_version_id AND v.repository_config_digest=r.repository_config_digest
		WHERE r.id=$1 AND r.worker_id=$2 AND r.worker_instance_id=$3 AND r.host_boot_id=$4 AND r.status='RUNNING' AND r.lease_sha256=$5 AND r.lease_expires_at>CURRENT_TIMESTAMP`, runID, a.WorkerID, a.InstanceID, a.HostBootID, machineSecretHash(lease)).Scan(&value.Ciphertext, &value.EDEK, &value.WrapKeyRef, &value.Version)
	return value, err
}

func (s *Store) CompleteIngestion(ctx context.Context, a application.MachineAuthority, runID int64, lease string, result application.SourceIngestionResult) (application.SourceRevision, error) {
	if !hexIdentity(result.ResolvedCommit, 40) || !hexIdentity(result.ResolvedTree, 40) || !hexIdentity(result.RepositoryBundleContentSHA256, 64) || !hexIdentity(result.RepositoryBundleObjectSHA256, 64) || result.RepositoryBundleSizeBytes < 1 {
		return application.SourceRevision{}, domain.ErrTaskInputInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.SourceRevision{}, err
	}
	defer tx.Rollback(ctx)
	var sourceID, revisionID, tenant, projectID, projectVersionId, worker int64
	var kind, refType, refName, frozen, instance, boot, storedLease, status string
	err = tx.QueryRow(ctx, `SELECT v.workspace_source_id,r.workspace_source_revision_id,r.tenant_id,r.kind,COALESCE(r.project_id,0),COALESCE(r.project_version_id,0),r.requested_ref_type,r.requested_ref_name,r.frozen_commit,r.worker_id,r.worker_instance_id,r.host_boot_id,r.lease_sha256,r.status FROM workforce_source_ingestion_runs r JOIN workforce_workspace_source_revisions v ON v.id=r.workspace_source_revision_id WHERE r.id=$1 FOR UPDATE OF r,v`, runID).Scan(&sourceID, &revisionID, &tenant, &kind, &projectID, &projectVersionId, &refType, &refName, &frozen, &worker, &instance, &boot, &storedLease, &status)
	if err != nil {
		return application.SourceRevision{}, err
	}
	if status != "RUNNING" || worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID || storedLease != machineSecretHash(lease) || kind == gitdomain.SourceGitRepository && frozen != result.ResolvedCommit {
		return application.SourceRevision{}, domain.ErrTaskInputInvalid
	}
	objectKey := fmt.Sprintf("workspace-sources/%d/repository.bundle", revisionID)
	frozenRevision, err := gitdomain.FreezeSourceRevision(gitdomain.FreezeSourceRevisionInput{ID: revisionID, SourceID: sourceID, TenantID: tenant, Kind: kind, ProjectID: projectID, RefType: refType, RefName: refName, ResolvedCommit: result.ResolvedCommit, ResolvedTree: result.ResolvedTree, RepositoryBundleContentSHA256: result.RepositoryBundleContentSHA256, RepositoryBundleObjectSHA256: result.RepositoryBundleObjectSHA256, RepositoryBundleObjectKey: objectKey, ResolvedAt: time.Now().UTC()})
	if err != nil {
		return application.SourceRevision{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_workspace_source_revisions SET resolved_commit=$2,resolved_tree=$3,repository_bundle_object_key=$4,repository_bundle_content_sha256=$5,repository_bundle_object_sha256=$6,repository_bundle_size_bytes=$7,source_identity_digest=$8,status='AVAILABLE',reason_code='',resolved_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, revisionID, result.ResolvedCommit, result.ResolvedTree, objectKey, result.RepositoryBundleContentSHA256, result.RepositoryBundleObjectSHA256, result.RepositoryBundleSizeBytes, frozenRevision.IdentityDigest); err != nil {
		return application.SourceRevision{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_source_ingestion_runs SET status='SUCCEEDED',reason_code='',finished_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID); err != nil {
		return application.SourceRevision{}, err
	}
	if projectID > 0 {
		if _, err = tx.Exec(ctx, `UPDATE workforce_projects SET current_source_state='READY',current_source_revision_id=$3,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND id=$2 AND current_version_id=$4`, tenant, projectID, revisionID, projectVersionId); err != nil {
			return application.SourceRevision{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.SourceRevision{}, err
	}
	return application.SourceRevision{ID: revisionID, SourceID: sourceID, TenantID: tenant, Kind: kind, ProjectID: projectID, ProjectVersionID: projectVersionId, RefType: refType, RefName: refName, ResolvedCommit: result.ResolvedCommit, ResolvedTree: result.ResolvedTree, RepositoryBundleContentSHA256: result.RepositoryBundleContentSHA256, RepositoryBundleObjectSHA256: result.RepositoryBundleObjectSHA256, RepositoryBundleSizeBytes: result.RepositoryBundleSizeBytes, SourceIdentityDigest: frozenRevision.IdentityDigest, Status: "AVAILABLE"}, nil
}

func (s *Store) FailIngestion(ctx context.Context, a application.MachineAuthority, runID int64, lease string, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 128 {
		return domain.ErrTaskInputInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var revisionID, worker int64
	var instance, boot, stored string
	var attempt, max int
	if err = tx.QueryRow(ctx, `SELECT workspace_source_revision_id,worker_id,worker_instance_id,host_boot_id,lease_sha256,attempt_number,max_attempts FROM workforce_source_ingestion_runs WHERE id=$1 AND status='RUNNING' FOR UPDATE`, runID).Scan(&revisionID, &worker, &instance, &boot, &stored, &attempt, &max); err != nil {
		return err
	}
	if worker != a.WorkerID || instance != a.InstanceID || boot != a.HostBootID || stored != machineSecretHash(lease) {
		return domain.ErrTaskInputInvalid
	}
	retry := sourceFailureRetryable(reason) && attempt < max
	status := "FAILED"
	if retry {
		status = "RETRY_WAIT"
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_source_ingestion_runs SET status=$2,reason_code=$3,next_retry_at=CASE WHEN $2='RETRY_WAIT' THEN CURRENT_TIMESTAMP+INTERVAL '1 second' ELSE NULL END,finished_at=CASE WHEN $2='FAILED' THEN CURRENT_TIMESTAMP ELSE NULL END,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, status, reason); err != nil {
		return err
	}
	if retry {
		messageID := s.idGen.NextID()
		message := domain.SourceIngestionMessage{SchemaVersion: 1, Kind: domain.SourceIngestionMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: 1}
		body, _ := json.Marshal(message)
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms,available_at) VALUES ($1,'source_ingestion',$2,'source.ingestion.retry',$3,'PENDING','seven.workforce.postprocess',$4,600000,CURRENT_TIMESTAMP+INTERVAL '1 second')`, messageID, runID, body, "postprocess."+strconv.FormatInt(worker, 10)); err != nil {
			return err
		}
	} else {
		_, err = tx.Exec(ctx, `UPDATE workforce_workspace_source_revisions SET status='FAILED',reason_code=$2,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, revisionID, reason)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE workforce_projects p SET current_source_state='FAILED',updated_at=CURRENT_TIMESTAMP FROM workforce_workspace_source_revisions r WHERE r.id=$1 AND p.id=r.project_id AND p.current_version_id=r.project_version_id`, revisionID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func connectionForMachine(ctx context.Context, tx pgx.Tx, id, versionID int64) (gitdomain.RepositoryTarget, error) {
	var item gitdomain.RepositoryTarget
	var scheme string
	err := tx.QueryRow(ctx, `SELECT p.id,p.tenant_id,v.repository_display_url,v.repository_provider_type,v.repository_scheme,v.repository_host,v.repository_port,v.repository_path,v.repository_web_url,v.repository_config_digest FROM workforce_projects p JOIN workforce_project_versions v ON v.tenant_id=p.tenant_id AND v.project_id=p.id WHERE p.id=$1 AND v.id=$2 AND v.kind='CODEBASE' AND p.status='ACTIVE'`, id, versionID).Scan(&item.ID, &item.TenantID, &item.Name, &item.ProviderType, &scheme, &item.Host, &item.Port, &item.RepositoryPath, &item.WebRepositoryURL, &item.ConfigDigest)
	if err != nil {
		return item, err
	}
	item.Transport = gitdomain.TransportHTTPS
	if scheme == "http" {
		item.Transport = gitdomain.TransportHTTP
	}
	item.Status = gitdomain.RepositoryEnabled
	item.Read.Capability = gitdomain.CapabilityReadSource
	item.Read.Status = gitdomain.CapabilityVerified
	item.Push.Capability = gitdomain.CapabilityPushBranch
	item.Push.Status = gitdomain.CapabilityUnverified
	item.Review.Capability = gitdomain.CapabilityCreateReview
	item.Review.Status = gitdomain.CapabilityUnverified
	item.Egress = gitdomain.EgressPolicy{AllowPrivateNetwork: true, AllowedHosts: []string{item.Host}, AllowedPorts: []int{item.Port}, AllowedCIDRs: []string{"0.0.0.0/0", "::/0"}}
	return item, err
}

func randomMachineToken() string {
	value := make([]byte, 32)
	_, _ = rand.Read(value)
	return hex.EncodeToString(value)
}
func machineSecretHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func hexIdentity(value string, n int) bool {
	if len(value) != n {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func sourceFailureRetryable(reason string) bool {
	switch reason {
	case "SOURCE_REMOTE_UNREACHABLE", "SOURCE_OBJECT_UNAVAILABLE", "SOURCE_UPLOAD_FAILED", "SOURCE_READBACK_FAILED", "SOURCE_WORKER_INTERRUPTED":
		return true
	default:
		return false
	}
}
