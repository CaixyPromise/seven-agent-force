package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	gitdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
)

type IDGenerator interface{ NextID() int64 }

type Store struct {
	db    *pgxpool.Pool
	idGen IDGenerator
}

func New(db *pgxpool.Pool, idGen IDGenerator) *Store { return &Store{db: db, idGen: idGen} }

type ArtifactAdapter struct{ Store *artifactstore.Store }

func (a ArtifactAdapter) Put(ctx context.Context, key, media string, reader io.Reader, size int64) error {
	return a.Store.Put(ctx, key, media, reader, size)
}
func (a ArtifactAdapter) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	return a.Store.Get(ctx, key)
}

func (s *Store) CreateGitSource(ctx context.Context, tenant int64, actor int64, request application.GitSourceRequest, connection gitdomain.RepositoryTarget) (application.SourceRevision, application.IngestionRun, bool, error) {
	digest := requestDigest(request)
	if existing, replay, err := s.replayRun(ctx, tenant, request.IdempotencyKey, digest); err != nil || replay {
		return existingRevision(ctx, s.db, tenant, existing.SourceRevisionID), existing, replay, err
	}
	return s.createSourceRun(ctx, tenant, actor, "GIT_REPOSITORY", request.ProjectID, request.ProjectVersionID, connection.ConfigDigest, request.RefType, request.RefName, "", "", "", "", 0, request.IdempotencyKey, digest, connection.Name)
}

func (s *Store) RetryGitSource(ctx context.Context, tenant, runID int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var revisionID, projectID, configVersion int64
	var repositoryDigest string
	if err = tx.QueryRow(ctx, `SELECT workspace_source_revision_id,project_id,project_version_id,repository_config_digest FROM workforce_source_ingestion_runs WHERE id=$1 AND tenant_id=$2 AND kind='GIT_REPOSITORY' AND status IN ('FAILED','WAITING_WORKER') FOR UPDATE`, runID, tenant).Scan(&revisionID, &projectID, &configVersion, &repositoryDigest); err != nil {
		return err
	}
	var worker int64
	_ = tx.QueryRow(ctx, `SELECT w.id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL AND s.docker_ready=true AND s.consumer_state='CONSUMING' ORDER BY COALESCE(s.last_heartbeat_at,s.registered_at) DESC,w.id LIMIT 1`).Scan(&worker)
	status := "WAITING_WORKER"
	if worker != 0 {
		status = "PENDING"
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_source_ingestion_runs SET worker_id=NULLIF($2::BIGINT,0),worker_instance_id='',host_boot_id='',lease_sha256='',lease_expires_at=NULL,status=$3,reason_code='',attempt_number=0,next_retry_at=NULL,finished_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=$1`, runID, worker, status); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_workspace_source_revisions SET status='PENDING',reason_code='',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, revisionID); err != nil {
		return err
	}
	if tag, updateErr := tx.Exec(ctx, `UPDATE workforce_projects p SET current_source_state='PREPARING',updated_at=CURRENT_TIMESTAMP FROM workforce_project_versions v WHERE p.id=$1 AND p.tenant_id=$2 AND p.current_version_id=$3 AND v.id=p.current_version_id AND v.repository_config_digest=$4`, projectID, tenant, configVersion, repositoryDigest); updateErr != nil || tag.RowsAffected() != 1 {
		if updateErr != nil {
			return updateErr
		}
		return errors.New("PROJECT_REPOSITORY_VERSION_CONFLICT")
	}
	if worker != 0 {
		messageID := s.idGen.NextID()
		message := domain.SourceIngestionMessage{SchemaVersion: 1, Kind: domain.SourceIngestionMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: 1}
		body, _ := json.Marshal(message)
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'source_ingestion',$2,'source.ingestion.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, runID, body, "postprocess."+strconv.FormatInt(worker, 10)); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) CreateUploadedSource(ctx context.Context, tenant int64, actor int64, request application.UploadSourceRequest, claim filefacade.WorkforceInputClaim, key, contentDigest, objectDigest string) (application.SourceRevision, application.IngestionRun, bool, error) {
	digest := requestDigest(request)
	if existing, replay, err := s.replayRun(ctx, tenant, request.IdempotencyKey, digest); err != nil || replay {
		return existingRevision(ctx, s.db, tenant, existing.SourceRevisionID), existing, replay, err
	}
	kind := "UPLOADED_ARCHIVE"
	if request.SourceKind == "DIRECTORY_ARCHIVE" {
		kind = "UPLOADED_DIRECTORY"
	}
	return s.createSourceRun(ctx, tenant, actor, kind, 0, 0, "", "", "", claim.Handle, key, contentDigest, objectDigest, claim.Size, request.IdempotencyKey, digest, claim.FileName)
}
func (s *Store) createSourceRun(ctx context.Context, tenant int64, actor int64, kind string, projectID, projectVersionId int64, repositoryDigest, refType string, refName string, handle string, inputKey string, inputContentDigest string, inputObjectDigest string, inputSize int64, idempotency, digest, display string) (application.SourceRevision, application.IngestionRun, bool, error) {
	sourceID, revisionID, runID := s.idGen.NextID(), s.idGen.NextID(), s.idGen.NextID()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.SourceRevision{}, application.IngestionRun{}, false, err
	}
	defer tx.Rollback(ctx)
	actorPrincipalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return application.SourceRevision{}, application.IngestionRun{}, false, err
	}
	if projectID != 0 {
		var available bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_projects p JOIN workforce_project_versions v ON v.tenant_id=p.tenant_id AND v.project_id=p.id AND v.id=p.current_version_id WHERE p.tenant_id=$1 AND p.id=$2 AND v.id=$3 AND v.repository_config_digest=$4 AND p.status='ACTIVE')`, tenant, projectID, projectVersionId, repositoryDigest).Scan(&available); err != nil || !available {
			return application.SourceRevision{}, application.IngestionRun{}, false, errors.New("PROJECT_REPOSITORY_VERSION_CONFLICT")
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_workspace_sources (id,tenant_id,kind,project_id,project_version_id,repository_config_digest,display_name,created_by_principal_id) VALUES ($1,$2,$3,NULLIF($4::BIGINT,0),NULLIF($5::BIGINT,0),$6,$7,$8)`, sourceID, tenant, kind, projectID, projectVersionId, repositoryDigest, display, actorPrincipalID); err != nil {
		return application.SourceRevision{}, application.IngestionRun{}, false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_workspace_source_revisions (id,tenant_id,workspace_source_id,version,kind,project_id,project_version_id,repository_config_digest,ref_type,ref_name,status) VALUES ($1,$2,$3,1,$4,NULLIF($5::BIGINT,0),NULLIF($6::BIGINT,0),$7,$8,$9,'PENDING')`, revisionID, tenant, sourceID, kind, projectID, projectVersionId, repositoryDigest, refType, refName); err != nil {
		return application.SourceRevision{}, application.IngestionRun{}, false, err
	}
	var worker int64
	_ = tx.QueryRow(ctx, `SELECT w.id FROM workforce_workers w JOIN workforce_worker_sessions s ON s.instance_id=w.current_instance_id WHERE w.desired_state='ENABLED' AND w.decommissioned_at IS NULL AND s.ended_at IS NULL AND s.docker_ready=true AND s.consumer_state='CONSUMING' ORDER BY COALESCE(s.last_heartbeat_at,s.registered_at) DESC,w.id LIMIT 1`).Scan(&worker)
	status := "WAITING_WORKER"
	if worker != 0 {
		status = "PENDING"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_source_ingestion_runs (id,tenant_id,workspace_source_revision_id,kind,file_asset_handle,input_object_key,input_content_sha256,input_object_sha256,input_size_bytes,project_id,project_version_id,repository_config_digest,requested_ref_type,requested_ref_name,worker_id,status,idempotency_key,request_digest,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULLIF($10::BIGINT,0),NULLIF($11::BIGINT,0),$12,$13,$14,NULLIF($15::BIGINT,0),$16,$17,$18,$19)`, runID, tenant, revisionID, kind, handle, inputKey, inputContentDigest, inputObjectDigest, inputSize, projectID, projectVersionId, repositoryDigest, refType, refName, worker, status, idempotency, digest, actorPrincipalID); err != nil {
		return application.SourceRevision{}, application.IngestionRun{}, false, err
	}
	if worker != 0 {
		messageID := s.idGen.NextID()
		message := domain.SourceIngestionMessage{SchemaVersion: 1, Kind: domain.SourceIngestionMessageKind, MessageID: messageID, WorkerID: worker, ResourceID: runID, Generation: 1}
		body, _ := json.Marshal(message)
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'source_ingestion',$2,'source.ingestion.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, messageID, runID, body, "postprocess."+strconv.FormatInt(worker, 10)); err != nil {
			return application.SourceRevision{}, application.IngestionRun{}, false, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return application.SourceRevision{}, application.IngestionRun{}, false, err
	}
	revision := application.SourceRevision{ID: revisionID, SourceID: sourceID, TenantID: tenant, Kind: kind, ProjectID: projectID, ProjectVersionID: projectVersionId, RepositoryConfigDigest: repositoryDigest, RefType: refType, RefName: refName, Status: "PENDING"}
	run := application.IngestionRun{ID: runID, TenantID: tenant, SourceRevisionID: revisionID, Kind: kind, Status: status, MaxAttempts: 3}
	return revision, run, false, nil
}
func (s *Store) CreateReference(ctx context.Context, tenant int64, actor int64, request application.ReferenceRequest, claim filefacade.WorkforceInputClaim, key, objectDigest string) (application.ReferenceAsset, bool, error) {
	digest := requestDigest(request)
	var existing application.ReferenceAsset
	var existingDigest string
	err := s.db.QueryRow(ctx, `SELECT id,tenant_id,display_name,content_type,content_sha256,size_bytes,status,request_digest FROM workforce_reference_assets WHERE tenant_id=$1 AND idempotency_key=$2`, tenant, request.IdempotencyKey).Scan(&existing.ID, &existing.TenantID, &existing.DisplayName, &existing.ContentType, &existing.ContentSHA256, &existing.SizeBytes, &existing.Status, &existingDigest)
	if err == nil {
		if existingDigest != digest {
			return application.ReferenceAsset{}, false, errors.New("IDEMPOTENCY_CONFLICT")
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return application.ReferenceAsset{}, false, err
	}
	item := application.ReferenceAsset{ID: s.idGen.NextID(), TenantID: tenant, DisplayName: claim.FileName, ContentType: claim.ContentType, ContentSHA256: claim.ContentSHA256, SizeBytes: claim.Size, Status: "AVAILABLE"}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return application.ReferenceAsset{}, false, err
	}
	defer tx.Rollback(ctx)
	actorPrincipalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return application.ReferenceAsset{}, false, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO workforce_reference_assets (id,tenant_id,file_asset_handle,display_name,content_type,content_sha256,size_bytes,object_key,object_sha256,status,idempotency_key,request_digest,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'AVAILABLE',$10,$11,$12) ON CONFLICT (tenant_id,content_sha256,display_name) DO NOTHING`, item.ID, tenant, claim.Handle, item.DisplayName, item.ContentType, item.ContentSHA256, item.SizeBytes, key, objectDigest, request.IdempotencyKey, digest, actorPrincipalID)
	if err != nil {
		return application.ReferenceAsset{}, false, err
	}
	if tag.RowsAffected() == 1 {
		if err = tx.Commit(ctx); err != nil {
			return application.ReferenceAsset{}, false, err
		}
		return item, false, nil
	}
	// Immutable reference bytes are tenant-scoped and content addressed. Reusing
	// the same named attachment in another Task must return the original asset
	// instead of exposing a storage uniqueness failure to the user.
	err = tx.QueryRow(ctx, `SELECT id,tenant_id,display_name,content_type,content_sha256,size_bytes,status FROM workforce_reference_assets WHERE tenant_id=$1 AND content_sha256=$2 AND display_name=$3`, tenant, item.ContentSHA256, item.DisplayName).Scan(&existing.ID, &existing.TenantID, &existing.DisplayName, &existing.ContentType, &existing.ContentSHA256, &existing.SizeBytes, &existing.Status)
	if err != nil {
		return application.ReferenceAsset{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return application.ReferenceAsset{}, false, err
	}
	return existing, true, nil
}
func (s *Store) GetRevision(ctx context.Context, tenant int64, id int64) (application.SourceRevision, error) {
	item := existingRevision(ctx, s.db, tenant, id)
	if item.ID == 0 {
		return item, application.ErrNotFound
	}
	return item, nil
}
func (s *Store) GetRun(ctx context.Context, tenant int64, id int64) (application.IngestionRun, error) {
	var item application.IngestionRun
	err := s.db.QueryRow(ctx, `SELECT id,tenant_id,workspace_source_revision_id,kind,status,reason_code,attempt_number,max_attempts FROM workforce_source_ingestion_runs WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&item.ID, &item.TenantID, &item.SourceRevisionID, &item.Kind, &item.Status, &item.ReasonCode, &item.AttemptNumber, &item.MaxAttempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, application.ErrNotFound
	}
	return item, err
}
func (s *Store) GetReference(ctx context.Context, tenant int64, id int64) (application.ReferenceAsset, error) {
	var item application.ReferenceAsset
	err := s.db.QueryRow(ctx, `SELECT id,tenant_id,display_name,content_type,content_sha256,size_bytes,status,object_key,object_sha256 FROM workforce_reference_assets WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&item.ID, &item.TenantID, &item.DisplayName, &item.ContentType, &item.ContentSHA256, &item.SizeBytes, &item.Status, &item.ObjectKey, &item.ObjectSHA256)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, application.ErrNotFound
	}
	return item, err
}
func (s *Store) GetTaskInput(ctx context.Context, tenant int64, taskID int64) (application.TaskInputView, error) {
	var view application.TaskInputView
	var sourceID int64
	err := s.db.QueryRow(ctx, `SELECT i.task_id,i.task_turn_id,i.source_kind,COALESCE(i.workspace_source_revision_id,0),i.input_snapshot_digest,i.repository_bundle_size_bytes FROM workforce_task_input_snapshots i JOIN workforce_tasks t ON t.id=i.task_id AND t.current_task_turn_id=i.task_turn_id WHERE i.task_id=$1 AND i.tenant_id=$2`, taskID, tenant).Scan(&view.TaskID, &view.TaskTurnID, &view.SourceKind, &sourceID, &view.InputSnapshotDigest, &view.RepositoryBundleSize)
	if errors.Is(err, pgx.ErrNoRows) {
		return view, application.ErrNotFound
	}
	if err != nil {
		return view, err
	}
	if view.SourceKind == domain.SourceKindEmptyWorkspace {
		identity := domain.CanonicalEmptyWorkspaceIdentity()
		view.WorkspaceSource = application.SourceRevision{TenantID: tenant, Kind: domain.SourceKindEmptyWorkspace, Status: domain.SourceAvailable, ResolvedCommit: identity.Commit, ResolvedTree: identity.Tree, SourceIdentityDigest: identity.IdentityDigest}
	} else {
		view.WorkspaceSource, err = s.GetRevision(ctx, tenant, sourceID)
		if err != nil {
			return view, err
		}
	}
	rows, err := s.db.Query(ctx, `SELECT r.id,r.tenant_id,r.display_name,r.content_type,r.content_sha256,r.size_bytes,r.status FROM workforce_task_input_references x JOIN workforce_reference_assets r ON r.id=x.reference_asset_id WHERE x.task_input_snapshot_id=(SELECT id FROM workforce_task_input_snapshots WHERE task_id=$1 AND task_turn_id=$2) ORDER BY x.display_order,r.id`, taskID, view.TaskTurnID)
	if err != nil {
		return view, err
	}
	defer rows.Close()
	for rows.Next() {
		var item application.ReferenceAsset
		if err = rows.Scan(&item.ID, &item.TenantID, &item.DisplayName, &item.ContentType, &item.ContentSHA256, &item.SizeBytes, &item.Status); err != nil {
			return view, err
		}
		view.References = append(view.References, item)
	}
	return view, rows.Err()
}
func (s *Store) GetGitPlan(ctx context.Context, tenant int64, taskID int64) (application.GitPlanView, error) {
	var plan application.GitPlanView
	err := s.db.QueryRow(ctx, `SELECT COALESCE(p.project_id,0),COALESCE(p.project_id,0),COALESCE(p.review_base_source_revision_id,0),p.source_ref_type,p.source_ref_name,p.source_commit,p.review_base_branch,p.review_base_commit,p.result_branch,p.delivery_mode,p.delivery_authorization,p.full_project_import,p.previous_delivery_commit FROM workforce_task_git_plans p JOIN workforce_tasks t ON t.id=p.task_id AND t.current_task_turn_id=p.task_turn_id WHERE p.task_id=$1 AND p.tenant_id=$2`, taskID, tenant).Scan(&plan.SourceProjectID, &plan.ProjectID, &plan.ReviewBaseSourceRevisionID, &plan.SourceRefType, &plan.SourceRefName, &plan.SourceCommit, &plan.ReviewBaseBranch, &plan.ReviewBaseCommit, &plan.ResultBranch, &plan.DeliveryMode, &plan.DeliveryAuthorization, &plan.FullProjectImport, &plan.PreviousDeliveryCommit)
	if errors.Is(err, pgx.ErrNoRows) {
		return plan, application.ErrNotFound
	}
	return plan, err
}
func (s *Store) ResolveTaskInput(ctx context.Context, tenant int64, sourceID int64, referenceIDs []int64) (domain.SourceRevisionView, []domain.ReferenceAssetView, error) {
	revision, err := s.GetRevision(ctx, tenant, sourceID)
	if err != nil {
		return domain.SourceRevisionView{}, nil, err
	}
	source := domain.SourceRevisionView{ID: revision.ID, TenantID: revision.TenantID, Status: revision.Status, SourceCommit: revision.ResolvedCommit, SourceTree: revision.ResolvedTree, SourceIdentityDigest: revision.SourceIdentityDigest}
	refs := make([]domain.ReferenceAssetView, 0, len(referenceIDs))
	for _, id := range referenceIDs {
		item, refErr := s.GetReference(ctx, tenant, id)
		if refErr != nil {
			return source, nil, refErr
		}
		refs = append(refs, domain.ReferenceAssetView{ID: item.ID, TenantID: item.TenantID, Status: item.Status, ContentSHA256: item.ContentSHA256, SizeBytes: item.SizeBytes})
	}
	return source, refs, nil
}
func (s *Store) replayRun(ctx context.Context, tenant int64, key string, digest string) (application.IngestionRun, bool, error) {
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return application.IngestionRun{}, false, domain.ErrTaskInputInvalid
	}
	var item application.IngestionRun
	var stored string
	err := s.db.QueryRow(ctx, `SELECT id,tenant_id,workspace_source_revision_id,kind,status,reason_code,attempt_number,max_attempts,request_digest FROM workforce_source_ingestion_runs WHERE tenant_id=$1 AND idempotency_key=$2`, tenant, key).Scan(&item.ID, &item.TenantID, &item.SourceRevisionID, &item.Kind, &item.Status, &item.ReasonCode, &item.AttemptNumber, &item.MaxAttempts, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return item, false, nil
	}
	if err != nil {
		return item, false, err
	}
	if stored != digest {
		return item, false, errors.New("IDEMPOTENCY_CONFLICT")
	}
	return item, true, nil
}
func existingRevision(ctx context.Context, db *pgxpool.Pool, tenant int64, id int64) application.SourceRevision {
	var item application.SourceRevision
	_ = db.QueryRow(ctx, `SELECT id,workspace_source_id,tenant_id,kind,COALESCE(project_id,0),COALESCE(project_version_id,0),repository_config_digest,ref_type,ref_name,resolved_commit,resolved_tree,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,repository_bundle_object_key,source_identity_digest,status,reason_code FROM workforce_workspace_source_revisions WHERE tenant_id=$1 AND id=$2`, tenant, id).Scan(&item.ID, &item.SourceID, &item.TenantID, &item.Kind, &item.ProjectID, &item.ProjectVersionID, &item.RepositoryConfigDigest, &item.RefType, &item.RefName, &item.ResolvedCommit, &item.ResolvedTree, &item.RepositoryBundleContentSHA256, &item.RepositoryBundleObjectSHA256, &item.RepositoryBundleSizeBytes, &item.RepositoryBundleObjectKey, &item.SourceIdentityDigest, &item.Status, &item.ReasonCode)
	return item
}
func requestDigest(value any) string {
	body, _ := json.Marshal(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func nullID(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
