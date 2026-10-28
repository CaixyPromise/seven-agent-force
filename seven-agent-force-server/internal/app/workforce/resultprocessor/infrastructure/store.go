package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

var (
	ErrNotFound = errors.New("result processor resource not found")
	ErrConflict = errors.New("result processor resource conflict")
)

const FixtureMessageKind = "seven.result-processor-fixture-run"

// Store owns the PostgreSQL persistence boundary of Result Processor
// management. Other Workforce modules interact through the facade package.
type Store struct {
	completionEvidence   completioncheck.EvidenceReader
	db                   *pgxpool.Pool
	idGen                IDGenerator
	environmentResolver  EnvironmentValueResolver
	notificationRecorder tasknotification.TransactionalRecorder
}

type IDGenerator interface{ NextID() int64 }

func New(db *pgxpool.Pool, idGen IDGenerator) *Store {
	return &Store{db: db, idGen: idGen}
}

func (s *Store) SetTaskNotificationRecorder(recorder tasknotification.TransactionalRecorder) {
	if s != nil {
		s.notificationRecorder = recorder
	}
}

type Revision struct {
	ID             int64                  `json:"id,string"`
	ProcessorID    int64                  `json:"processorId,string"`
	ScopeType      string                 `json:"scopeType"`
	TenantID       int64                  `json:"tenantId,string"`
	RevisionNumber int64                  `json:"revisionNumber,string"`
	DraftVersion   int64                  `json:"draftVersion,string"`
	Status         string                 `json:"status"`
	Manifest       processor.Manifest     `json:"manifest"`
	Files          []processor.SourceFile `json:"files"`
	Validation     processor.Validation   `json:"validation"`
	CreatedAt      string                 `json:"createdAt"`
	UpdatedAt      string                 `json:"updatedAt"`
}

type Release struct {
	ID                  int64                  `json:"id,string"`
	ProcessorID         int64                  `json:"processorId,string"`
	ScopeType           string                 `json:"scopeType"`
	TenantID            int64                  `json:"tenantId,string"`
	RevisionID          int64                  `json:"revisionId,string"`
	Version             string                 `json:"version"`
	Manifest            processor.Manifest     `json:"manifest"`
	Files               []processor.SourceFile `json:"files"`
	ArchiveObjectKey    string                 `json:"archiveObjectKey"`
	ArchiveSHA256       string                 `json:"archiveSha256"`
	ArchiveSizeBytes    int64                  `json:"archiveSizeBytes"`
	PublishedAt         string                 `json:"publishedAt"`
	EnvironmentBindings []EnvironmentBinding   `json:"environmentBindings,omitempty"`
}

type Processor struct {
	ID            int64     `json:"id,string"`
	ScopeType     string    `json:"scopeType"`
	TenantID      int64     `json:"tenantId,string"`
	Name          string    `json:"name"`
	Category      string    `json:"category"`
	Description   string    `json:"description"`
	DefaultBound  bool      `json:"defaultBound"`
	LatestRelease *Release  `json:"latestRelease,omitempty"`
	CurrentDraft  *Revision `json:"currentDraft,omitempty"`
	Releases      []Release `json:"releases,omitempty"`
	CreatedAt     string    `json:"createdAt"`
	UpdatedAt     string    `json:"updatedAt"`
}

type EnvironmentBinding struct {
	ID                  int64           `json:"id,string"`
	TenantID            int64           `json:"tenantId,string"`
	ProcessorReleaseID  int64           `json:"processorReleaseId,string"`
	Name                string          `json:"name"`
	Type                string          `json:"type"`
	Phase               string          `json:"phase"`
	Config              json.RawMessage `json:"config,omitempty"`
	SecretRef           string          `json:"secretRef,omitempty"`
	SecretVersion       string          `json:"secretVersion,omitempty"`
	TaskOverrideAllowed bool            `json:"taskOverrideAllowed"`
	NetworkPolicy       string          `json:"networkPolicy"`
}

type FixtureRun struct {
	ID                   int64           `json:"id,string"`
	TenantID             int64           `json:"tenantId,string"`
	RevisionID           int64           `json:"revisionId,string"`
	HarnessReleaseID     int64           `json:"harnessReleaseId,string"`
	HarnessImageDigest   string          `json:"harnessImageDigest"`
	EvaluationContext    json.RawMessage `json:"evaluationContext"`
	WorkspaceFiles       json.RawMessage `json:"workspaceFiles"`
	ConfigOverrides      json.RawMessage `json:"configOverrides"`
	ExpectedDecision     string          `json:"expectedDecision,omitempty"`
	Status               string          `json:"status"`
	WorkerID             int64           `json:"workerId,string,omitempty"`
	WorkerInstanceID     string          `json:"workerInstanceId,omitempty"`
	Generation           int64           `json:"generation,string"`
	ResultContract       json.RawMessage `json:"resultContract,omitempty"`
	ResultContractSHA256 string          `json:"resultContractSha256,omitempty"`
	ReasonCode           string          `json:"reasonCode,omitempty"`
	Diagnostics          string          `json:"diagnostics,omitempty"`
	CreatedAt            string          `json:"createdAt"`
	StartedAt            string          `json:"startedAt,omitempty"`
	FinishedAt           string          `json:"finishedAt,omitempty"`
}

type FixtureMessage struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	MessageID     int64  `json:"messageId,string"`
	WorkerID      int64  `json:"workerId,string"`
	RunID         int64  `json:"runId,string"`
	Generation    int64  `json:"generation,string"`
}

type EvaluationCycle struct {
	ID                  int64                    `json:"id,string"`
	TaskID              int64                    `json:"taskId,string"`
	TaskTurnID          int64                    `json:"taskTurnId,string"`
	ExecutionNodeID     int64                    `json:"executionNodeId,string"`
	AttemptID           int64                    `json:"attemptId,string"`
	CycleNumber         int                      `json:"cycleNumber"`
	SessionDigest       string                   `json:"sessionDigest,omitempty"`
	CandidateTreeDigest string                   `json:"candidateTreeDigest,omitempty"`
	FindingsDigest      string                   `json:"findingsDigest,omitempty"`
	ContractDigest      string                   `json:"contractDigest"`
	Decision            string                   `json:"decision"`
	ReasonCode          string                   `json:"reasonCode"`
	Message             string                   `json:"message"`
	Contract            processor.ResultContract `json:"contract"`
	Status              string                   `json:"status"`
	ResumeNonceSHA256   string                   `json:"resumeNonceSha256,omitempty"`
	StartedAt           string                   `json:"startedAt"`
	PersistedAt         string                   `json:"persistedAt,omitempty"`
	CompletedAt         string                   `json:"completedAt,omitempty"`
}

func (s *Store) CreateProcessor(ctx context.Context, tenantID int64, actor int64, name string, category string, description string, manifest processor.Manifest, files []processor.SourceFile) (Processor, error) {
	return s.createProcessor(ctx, "TENANT", tenantID, actor, "", name, category, description, manifest, files)
}

func (s *Store) CreateSystemProcessor(ctx context.Context, serviceCode string, name string, category string, description string, manifest processor.Manifest, files []processor.SourceFile) (Processor, error) {
	return s.createProcessor(ctx, "SYSTEM", 0, 0, serviceCode, name, category, description, manifest, files)
}

func (s *Store) createProcessor(ctx context.Context, scopeType string, tenantID int64, actor int64, serviceCode string, name string, category string, description string, manifest processor.Manifest, files []processor.SourceFile) (Processor, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Processor{}, err
	}
	defer tx.Rollback(ctx)
	processorID := s.idGen.NextID()
	revisionID := s.idGen.NextID()
	var principalID int64
	if scopeType == "SYSTEM" {
		principalID, err = principalinfra.EnsureService(ctx, tx, s.idGen, serviceCode)
	} else {
		principalID, err = principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	}
	if err != nil {
		return Processor{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_processors (id,scope_type,tenant_id,name,category,description,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7)`, processorID, scopeType, nullScopeTenant(scopeType, tenantID), name, category, description, principalID); err != nil {
		return Processor{}, classifyConflict(err)
	}
	manifestJSON, filesJSON := mustJSON(manifest), mustJSON(files)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_processor_revisions (id,processor_id,scope_type,tenant_id,revision_number,draft_version,status,manifest_json,files_json,created_by_principal_id) VALUES ($1,$2,$3,$4,1,1,'DRAFT',$5,$6,$7)`, revisionID, processorID, scopeType, nullScopeTenant(scopeType, tenantID), manifestJSON, filesJSON, principalID); err != nil {
		return Processor{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Processor{}, err
	}
	if scopeType == "SYSTEM" {
		return s.GetSystemProcessor(ctx, processorID)
	}
	return s.GetProcessor(ctx, tenantID, processorID)
}

func (s *Store) ListProcessors(ctx context.Context, tenantID int64) ([]Processor, error) {
	// Resolve the list in three bounded reads instead of one four-source
	// statement. This keeps the query-width contract explicit without creating
	// an N+1 query: processor rows, one default binding, then one release scan.
	rows, err := s.db.Query(ctx, `SELECT id,scope_type,COALESCE(tenant_id,0),name,category,description,
       COALESCE(to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),
       COALESCE(to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'')
	FROM workforce_result_processors WHERE scope_type='SYSTEM' OR (scope_type='TENANT' AND tenant_id=$1)
	 ORDER BY CASE WHEN scope_type='SYSTEM' THEN 1 ELSE 0 END,updated_at DESC,id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Processor, 0)
	processorIDs := make([]int64, 0)
	for rows.Next() {
		var item Processor
		if err := rows.Scan(&item.ID, &item.ScopeType, &item.TenantID, &item.Name, &item.Category, &item.Description, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
		processorIDs = append(processorIDs, item.ID)
	}
	if err = rows.Err(); err != nil || len(items) == 0 {
		return items, err
	}
	var defaultProcessorID int64
	_ = s.db.QueryRow(ctx, `SELECT r.processor_id
  FROM workforce_result_processor_bindings b
  JOIN workforce_result_processor_releases r ON r.id=b.processor_release_id
 WHERE b.tenant_id=$1 AND b.binding_name='default'`, tenantID).Scan(&defaultProcessorID)
	latestRows, err := s.db.Query(ctx, `SELECT DISTINCT ON (processor_id) id,processor_id,version,archive_sha256
  FROM workforce_result_processor_releases
	 WHERE (scope_type='SYSTEM' OR (scope_type='TENANT' AND tenant_id=$1)) AND processor_id=ANY($2)
 ORDER BY processor_id,published_at DESC,id DESC`, tenantID, processorIDs)
	if err != nil {
		return nil, err
	}
	defer latestRows.Close()
	latest := make(map[int64]Release, len(items))
	for latestRows.Next() {
		var release Release
		if err = latestRows.Scan(&release.ID, &release.ProcessorID, &release.Version, &release.ArchiveSHA256); err != nil {
			return nil, err
		}
		latest[release.ProcessorID] = release
	}
	if err = latestRows.Err(); err != nil {
		return nil, err
	}
	for index := range items {
		items[index].DefaultBound = items[index].ID == defaultProcessorID
		if release, exists := latest[items[index].ID]; exists {
			items[index].LatestRelease = &release
		}
	}
	return items, nil
}

func (s *Store) ListSystemProcessors(ctx context.Context) ([]Processor, error) {
	items, err := s.ListProcessors(ctx, 0)
	if err != nil {
		return nil, err
	}
	result := make([]Processor, 0, len(items))
	for _, item := range items {
		if item.ScopeType == "SYSTEM" {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *Store) GetProcessor(ctx context.Context, tenantID int64, processorID int64) (Processor, error) {
	return s.getProcessor(ctx, tenantID, processorID, false)
}
func (s *Store) GetSystemProcessor(ctx context.Context, processorID int64) (Processor, error) {
	return s.getProcessor(ctx, 0, processorID, true)
}
func (s *Store) getProcessor(ctx context.Context, tenantID int64, processorID int64, systemOnly bool) (Processor, error) {
	var item Processor
	err := s.db.QueryRow(ctx, `SELECT id,scope_type,COALESCE(tenant_id,0),name,category,description,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processors WHERE id=$2 AND (($3 AND scope_type='SYSTEM') OR (NOT $3 AND (scope_type='SYSTEM' OR (scope_type='TENANT' AND tenant_id=$1))))`, tenantID, processorID, systemOnly).Scan(&item.ID, &item.ScopeType, &item.TenantID, &item.Name, &item.Category, &item.Description, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Processor{}, ErrNotFound
	}
	if err != nil {
		return Processor{}, err
	}
	revision, err := s.latestRevision(ctx, tenantID, processorID)
	if err == nil {
		item.CurrentDraft = &revision
	} else if !errors.Is(err, ErrNotFound) {
		return Processor{}, err
	}
	releases, err := s.ListReleases(ctx, tenantID, processorID)
	if err != nil {
		return Processor{}, err
	}
	item.Releases = releases
	if len(releases) > 0 {
		item.LatestRelease = &releases[0]
	}
	return item, nil
}

func (s *Store) CreateRevision(ctx context.Context, tenantID int64, processorID int64, actor int64) (Revision, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Revision{}, err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return Revision{}, err
	}
	var lockedProcessorID int64
	if err = tx.QueryRow(ctx, `SELECT id FROM workforce_result_processors WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, processorID).Scan(&lockedProcessorID); err != nil {
		return Revision{}, classifyNotFound(err)
	}
	var source Revision
	if err = scanRevision(tx.QueryRow(ctx, `SELECT id,processor_id,scope_type,COALESCE(tenant_id,0),revision_number,draft_version,status,manifest_json,files_json,validation_json,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processor_revisions WHERE tenant_id=$1 AND processor_id=$2 ORDER BY revision_number DESC LIMIT 1 FOR UPDATE`, tenantID, processorID), &source); err != nil {
		return Revision{}, classifyNotFound(err)
	}
	result := source
	result.ID = s.idGen.NextID()
	result.RevisionNumber++
	result.DraftVersion = 1
	result.Status = "DRAFT"
	result.Validation = processor.Validation{}
	manifestJSON, filesJSON := mustJSON(result.Manifest), mustJSON(result.Files)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_processor_revisions (id,processor_id,scope_type,tenant_id,revision_number,draft_version,status,manifest_json,files_json,created_by_principal_id) VALUES ($1,$2,'TENANT',$3,$4,1,'DRAFT',$5,$6,$7)`, result.ID, processorID, tenantID, result.RevisionNumber, manifestJSON, filesJSON, principalID); err != nil {
		return Revision{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Revision{}, err
	}
	return s.Revision(ctx, tenantID, result.ID)
}

func (s *Store) Revision(ctx context.Context, tenantID int64, revisionID int64) (Revision, error) {
	var item Revision
	err := scanRevision(s.db.QueryRow(ctx, `SELECT id,processor_id,scope_type,COALESCE(tenant_id,0),revision_number,draft_version,status,manifest_json,files_json,validation_json,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processor_revisions WHERE id=$2 AND (scope_type='SYSTEM' OR (scope_type='TENANT' AND tenant_id=$1))`, tenantID, revisionID), &item)
	return item, classifyNotFound(err)
}

func (s *Store) latestRevision(ctx context.Context, tenantID int64, processorID int64) (Revision, error) {
	var item Revision
	err := scanRevision(s.db.QueryRow(ctx, `SELECT id,processor_id,scope_type,COALESCE(tenant_id,0),revision_number,draft_version,status,manifest_json,files_json,validation_json,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processor_revisions WHERE processor_id=$2 AND (scope_type='SYSTEM' OR (scope_type='TENANT' AND tenant_id=$1)) ORDER BY revision_number DESC LIMIT 1`, tenantID, processorID), &item)
	return item, classifyNotFound(err)
}

func (s *Store) UpdateRevision(ctx context.Context, tenantID int64, revisionID int64, expectedVersion int64, manifest processor.Manifest, files []processor.SourceFile) (Revision, error) {
	if expectedVersion < 1 {
		return Revision{}, fmt.Errorf("expectedRevision must be positive")
	}
	manifestJSON, filesJSON := mustJSON(manifest), mustJSON(files)
	tag, err := s.db.Exec(ctx, `UPDATE workforce_result_processor_revisions SET manifest_json=$4,files_json=$5,draft_version=draft_version+1,validation_json='{}'::jsonb,validation_status='PENDING',updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND id=$2 AND draft_version=$3 AND status='DRAFT'`, tenantID, revisionID, expectedVersion, manifestJSON, filesJSON)
	if err != nil {
		return Revision{}, err
	}
	if tag.RowsAffected() != 1 {
		return Revision{}, ErrConflict
	}
	return s.Revision(ctx, tenantID, revisionID)
}

func (s *Store) SetValidation(ctx context.Context, tenantID int64, revisionID int64, validation processor.Validation) error {
	tag, err := s.db.Exec(ctx, `UPDATE workforce_result_processor_revisions SET validation_json=$3,validation_status=$4,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND id=$2 AND status='DRAFT'`, tenantID, revisionID, mustJSON(validation), ternary(validation.Valid, "VALID", "INVALID"))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetSystemValidation(ctx context.Context, revisionID int64, validation processor.Validation) error {
	tag, err := s.db.Exec(ctx, `UPDATE workforce_result_processor_revisions SET validation_json=$2,validation_status=$3,updated_at=CURRENT_TIMESTAMP WHERE scope_type='SYSTEM' AND id=$1 AND status='DRAFT'`, revisionID, mustJSON(validation), ternary(validation.Valid, "VALID", "INVALID"))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Publish(ctx context.Context, tenantID int64, revisionID int64, actor int64, version string, objectKey string, digest string, sizeBytes int64) (Release, error) {
	return s.publish(ctx, "TENANT", tenantID, revisionID, actor, "", version, objectKey, digest, sizeBytes)
}

func (s *Store) PublishSystem(ctx context.Context, revisionID int64, serviceCode string, version string, objectKey string, digest string, sizeBytes int64) (Release, error) {
	return s.publish(ctx, "SYSTEM", 0, revisionID, 0, serviceCode, version, objectKey, digest, sizeBytes)
}

func (s *Store) publish(ctx context.Context, scopeType string, tenantID int64, revisionID int64, actor int64, serviceCode string, version string, objectKey string, digest string, sizeBytes int64) (Release, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Release{}, err
	}
	defer tx.Rollback(ctx)
	var revision Revision
	if err = scanRevision(tx.QueryRow(ctx, `SELECT id,processor_id,scope_type,COALESCE(tenant_id,0),revision_number,draft_version,status,manifest_json,files_json,validation_json,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),to_char(updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processor_revisions WHERE id=$2 AND scope_type=$3 AND (($3='SYSTEM' AND tenant_id IS NULL) OR ($3='TENANT' AND tenant_id=$1)) FOR UPDATE`, tenantID, revisionID, scopeType), &revision); err != nil {
		return Release{}, classifyNotFound(err)
	}
	if !revision.Validation.Valid {
		return Release{}, fmt.Errorf("RESULT_PROCESSOR_VALIDATION_REQUIRED")
	}
	manifest := revision.Manifest
	manifest.Version = version
	manifestJSON, filesJSON := mustJSON(manifest), mustJSON(revision.Files)
	newID := s.idGen.NextID()
	var principalID int64
	if scopeType == "SYSTEM" {
		principalID, err = principalinfra.EnsureService(ctx, tx, s.idGen, serviceCode)
	} else {
		principalID, err = principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	}
	if err != nil {
		return Release{}, err
	}
	row := tx.QueryRow(ctx, `INSERT INTO workforce_result_processor_releases (id,processor_id,scope_type,tenant_id,revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,published_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (processor_id,version) DO NOTHING RETURNING id,processor_id,scope_type,COALESCE(tenant_id,0),revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,to_char(published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`, newID, revision.ProcessorID, scopeType, nullScopeTenant(scopeType, tenantID), revisionID, version, manifestJSON, filesJSON, objectKey, digest, sizeBytes, principalID)
	var release Release
	err = scanRelease(row, &release)
	if errors.Is(err, pgx.ErrNoRows) {
		err = scanRelease(tx.QueryRow(ctx, `SELECT id,processor_id,scope_type,COALESCE(tenant_id,0),revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,to_char(published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processor_releases WHERE scope_type=$4 AND processor_id=$2 AND version=$3 AND (($4='SYSTEM' AND tenant_id IS NULL) OR ($4='TENANT' AND tenant_id=$1))`, tenantID, revision.ProcessorID, version, scopeType), &release)
		if err != nil {
			return Release{}, err
		}
		if release.ArchiveSHA256 != digest {
			return Release{}, ErrConflict
		}
		return release, tx.Commit(ctx)
	}
	if err != nil {
		return Release{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_processor_revisions SET status='PUBLISHED',updated_at=CURRENT_TIMESTAMP WHERE id=$1`, revisionID); err != nil {
		return Release{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_result_processors SET updated_at=CURRENT_TIMESTAMP WHERE id=$1`, revision.ProcessorID); err != nil {
		return Release{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Release{}, err
	}
	return release, nil
}

func (s *Store) ListReleases(ctx context.Context, tenantID int64, processorID int64) ([]Release, error) {
	rows, err := s.db.Query(ctx, `SELECT id,processor_id,scope_type,COALESCE(tenant_id,0),revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,to_char(published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processor_releases WHERE processor_id=$2 AND (scope_type='SYSTEM' OR (scope_type='TENANT' AND tenant_id=$1)) ORDER BY published_at DESC,id DESC`, tenantID, processorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Release, 0)
	for rows.Next() {
		var item Release
		if err := scanRelease(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.withReleaseEnvironmentBindings(ctx, tenantID, items)
}

func (s *Store) Release(ctx context.Context, tenantID int64, releaseID int64) (Release, error) {
	var item Release
	err := scanRelease(s.db.QueryRow(ctx, `SELECT id,processor_id,scope_type,COALESCE(tenant_id,0),revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,to_char(published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_result_processor_releases WHERE id=$1 AND (scope_type='SYSTEM' OR (scope_type='TENANT' AND tenant_id=$2))`, releaseID, tenantID), &item)
	if err != nil {
		return item, classifyNotFound(err)
	}
	releases, err := s.withReleaseEnvironmentBindings(ctx, tenantID, []Release{item})
	if err != nil {
		return Release{}, err
	}
	if len(releases) == 1 {
		return releases[0], nil
	}
	return Release{}, ErrNotFound
}

// withReleaseEnvironmentBindings enriches immutable Release metadata with
// the caller tenant's configuration bindings. Bindings deliberately remain
// tenant-owned even when the Release itself is a visible SYSTEM asset; the
// batch read keeps Release listing bounded and avoids an authoring-time N+1.
func (s *Store) withReleaseEnvironmentBindings(ctx context.Context, tenantID int64, releases []Release) ([]Release, error) {
	if len(releases) == 0 {
		return releases, nil
	}
	ids := make([]int64, 0, len(releases))
	index := make(map[int64]int, len(releases))
	for i := range releases {
		ids = append(ids, releases[i].ID)
		index[releases[i].ID] = i
	}
	rows, err := s.db.Query(ctx, `SELECT id,tenant_id,processor_release_id,name,type,phase,COALESCE(config_json,'null'::jsonb),secret_ref,secret_version,task_override_allowed,network_policy
	  FROM workforce_result_processor_environment_bindings
	 WHERE tenant_id=$1 AND processor_release_id=ANY($2)
	 ORDER BY processor_release_id,name`, tenantID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var binding EnvironmentBinding
		if err := rows.Scan(&binding.ID, &binding.TenantID, &binding.ProcessorReleaseID, &binding.Name, &binding.Type, &binding.Phase, &binding.Config, &binding.SecretRef, &binding.SecretVersion, &binding.TaskOverrideAllowed, &binding.NetworkPolicy); err != nil {
			return nil, err
		}
		if releaseIndex, exists := index[binding.ProcessorReleaseID]; exists {
			releases[releaseIndex].EnvironmentBindings = append(releases[releaseIndex].EnvironmentBindings, binding)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return releases, nil
}

func (s *Store) SetDefaultBinding(ctx context.Context, tenantID int64, actor int64, releaseID int64) error {
	if _, err := s.Release(ctx, tenantID, releaseID); err != nil {
		return err
	}
	principalID, err := s.userPrincipal(ctx, actor)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO workforce_result_processor_bindings (id,tenant_id,binding_name,processor_release_id,changed_by_principal_id) VALUES ($1,$2,'default',$3,$4) ON CONFLICT (tenant_id,binding_name) DO UPDATE SET processor_release_id=EXCLUDED.processor_release_id,version=workforce_result_processor_bindings.version+1,changed_by_principal_id=EXCLUDED.changed_by_principal_id,updated_at=CURRENT_TIMESTAMP`, s.idGen.NextID(), tenantID, releaseID, principalID)
	return err
}

func (s *Store) SetEnvironmentBinding(ctx context.Context, tenantID int64, actor int64, binding EnvironmentBinding) (EnvironmentBinding, error) {
	if _, err := s.Release(ctx, tenantID, binding.ProcessorReleaseID); err != nil {
		return EnvironmentBinding{}, err
	}
	if binding.Phase == "" {
		binding.Phase = "EVALUATE"
	}
	if binding.NetworkPolicy == "" {
		binding.NetworkPolicy = "NONE"
	}
	if binding.Type != "CONFIG" && binding.Type != "SECRET" || binding.Phase != "EVALUATE" || (binding.Type == "SECRET" && (strings.TrimSpace(binding.SecretRef) == "" || strings.TrimSpace(binding.SecretVersion) == "")) {
		return EnvironmentBinding{}, fmt.Errorf("invalid Result Processor environment binding")
	}
	if binding.Type == "SECRET" && len(binding.Config) != 0 {
		return EnvironmentBinding{}, fmt.Errorf("SECRET environment binding must not contain a config value")
	}
	if binding.Type == "SECRET" {
		registrar, registered := s.environmentResolver.(EnvironmentSecretRegistrar)
		verifier, verified := s.environmentResolver.(EnvironmentSecretBindingVerifier)
		if s.environmentResolver == nil || !registered || !verified {
			return EnvironmentBinding{}, fmt.Errorf("RESULT_PROCESSOR_SECRET_RESOLVER_UNAVAILABLE")
		}
		if err := registrar.RegisterResultProcessorSecretRef(ctx, binding.SecretRef); err != nil {
			return EnvironmentBinding{}, fmt.Errorf("RESULT_PROCESSOR_SECRET_REGISTER_FAILED: %w", err)
		}
		if err := verifier.VerifyResultProcessorSecretRef(ctx, binding.SecretRef, binding.SecretVersion, binding.Phase); err != nil {
			return EnvironmentBinding{}, fmt.Errorf("RESULT_PROCESSOR_SECRET_VERSION_INVALID: %w", err)
		}
	}
	binding.ID = s.idGen.NextID()
	principalID, err := s.userPrincipal(ctx, actor)
	if err != nil {
		return EnvironmentBinding{}, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO workforce_result_processor_environment_bindings (id,tenant_id,processor_release_id,name,type,phase,config_json,secret_ref,secret_version,task_override_allowed,network_policy,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (tenant_id,processor_release_id,name) DO UPDATE SET type=EXCLUDED.type,phase=EXCLUDED.phase,config_json=EXCLUDED.config_json,secret_ref=EXCLUDED.secret_ref,secret_version=EXCLUDED.secret_version,task_override_allowed=EXCLUDED.task_override_allowed,network_policy=EXCLUDED.network_policy,created_by_principal_id=EXCLUDED.created_by_principal_id,updated_at=CURRENT_TIMESTAMP`, binding.ID, tenantID, binding.ProcessorReleaseID, binding.Name, binding.Type, binding.Phase, nullableJSON(binding.Config), binding.SecretRef, binding.SecretVersion, binding.TaskOverrideAllowed, binding.NetworkPolicy, principalID)
	if err != nil {
		return EnvironmentBinding{}, err
	}
	return s.EnvironmentBinding(ctx, tenantID, binding.ProcessorReleaseID, binding.Name)
}

func (s *Store) EnvironmentBinding(ctx context.Context, tenantID int64, releaseID int64, name string) (EnvironmentBinding, error) {
	var item EnvironmentBinding
	err := s.db.QueryRow(ctx, `SELECT id,tenant_id,processor_release_id,name,type,phase,COALESCE(config_json,'null'::jsonb),secret_ref,secret_version,task_override_allowed,network_policy FROM workforce_result_processor_environment_bindings WHERE tenant_id=$1 AND processor_release_id=$2 AND name=$3`, tenantID, releaseID, name).Scan(&item.ID, &item.TenantID, &item.ProcessorReleaseID, &item.Name, &item.Type, &item.Phase, &item.Config, &item.SecretRef, &item.SecretVersion, &item.TaskOverrideAllowed, &item.NetworkPolicy)
	return item, classifyNotFound(err)
}

func (s *Store) ResolveExecution(ctx context.Context, tenantID int64, mode string, releaseID int64, parameters, configOverrides json.RawMessage) (facade.Execution, error) {
	mode = strings.ToUpper(strings.TrimSpace(mode))
	if mode == "" {
		mode = "DEFAULT"
	}
	if mode != "DEFAULT" && mode != "RELEASE" {
		return facade.Execution{}, fmt.Errorf("resultProcessorSelection.mode must be DEFAULT or RELEASE")
	}
	if mode == "DEFAULT" {
		err := s.db.QueryRow(ctx, `SELECT processor_release_id FROM workforce_result_processor_bindings WHERE tenant_id=$1 AND binding_name='default'`, tenantID).Scan(&releaseID)
		if errors.Is(err, pgx.ErrNoRows) {
			return facade.Execution{}, fmt.Errorf("RESULT_PROCESSOR_DEFAULT_NOT_CONFIGURED")
		}
		if err != nil {
			return facade.Execution{}, err
		}
	} else if releaseID <= 0 {
		return facade.Execution{}, fmt.Errorf("resultProcessorSelection.releaseId is required")
	}
	release, err := s.Release(ctx, tenantID, releaseID)
	if err != nil {
		return facade.Execution{}, err
	}
	if len(parameters) == 0 {
		parameters = json.RawMessage(`{}`)
	}
	if len(configOverrides) == 0 {
		configOverrides = json.RawMessage(`{}`)
	}
	if err := processor.ValidateParameters(release.Manifest.ParametersSchema, parameters); err != nil {
		return facade.Execution{}, err
	}
	if !validJSONObject(configOverrides) {
		return facade.Execution{}, fmt.Errorf("resultProcessorConfigOverrides must be a JSON object")
	}
	bindings, err := s.environmentBindings(ctx, tenantID, release.ID)
	if err != nil {
		return facade.Execution{}, err
	}
	environment, secretVersions, err := canonicalEnvironment(bindings, parameters, configOverrides)
	if err != nil {
		return facade.Execution{}, err
	}
	environmentJSON := mustJSON(environment)
	manifestJSON := mustJSON(release.Manifest)
	return facade.Execution{
		ReleaseID: release.ID, Digest: release.ArchiveSHA256, ArchiveObjectKey: release.ArchiveObjectKey, Manifest: manifestJSON,
		EnvironmentConfigDigest: shaJSON(environment), Environment: environmentJSON, SecretVersionIdentities: secretVersions, Parameters: append(json.RawMessage(nil), parameters...),
		MaxEvaluationCycles: release.Manifest.MaxEvaluationCycles, ContinuationPolicy: release.Manifest.ContinuationPolicy, DeliveryPolicy: release.Manifest.DeliveryPolicy,
	}, nil
}

func (s *Store) CreateFixtureRun(ctx context.Context, tenantID int64, revisionID int64, harnessReleaseID int64, expectedDecision string, actor int64, evaluationContext, workspaceFiles, configOverrides json.RawMessage) (FixtureRun, FixtureMessage, error) {
	if !validJSONObject(evaluationContext) || !validJSONObject(configOverrides) || !validWorkspaceFiles(workspaceFiles) {
		return FixtureRun{}, FixtureMessage{}, fmt.Errorf("fixture input is invalid or exceeds its bounded file contract")
	}
	var validation processor.Validation
	var manifest processor.Manifest
	var files []processor.SourceFile
	if err := s.db.QueryRow(ctx, `SELECT validation_json,manifest_json,files_json FROM workforce_result_processor_revisions WHERE tenant_id=$1 AND id=$2`, tenantID, revisionID).Scan(&validation, &manifest, &files); errors.Is(err, pgx.ErrNoRows) {
		return FixtureRun{}, FixtureMessage{}, ErrNotFound
	} else if err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	if !validation.Valid {
		return FixtureRun{}, FixtureMessage{}, fmt.Errorf("RESULT_PROCESSOR_VALIDATION_REQUIRED")
	}
	_, revisionDigest, err := processor.CanonicalArchive(manifest, files)
	if err != nil {
		return FixtureRun{}, FixtureMessage{}, fmt.Errorf("RESULT_PROCESSOR_FIXTURE_REVISION_INVALID: %w", err)
	}
	var imageDigest string
	var admissionEvidence json.RawMessage
	if err := s.db.QueryRow(ctx, `SELECT image_digest,admission_evidence_json FROM workforce_harness_releases WHERE id=$1 AND status='ACTIVE'`, harnessReleaseID).Scan(&imageDigest, &admissionEvidence); errors.Is(err, pgx.ErrNoRows) {
		return FixtureRun{}, FixtureMessage{}, fmt.Errorf("fixture Harness Release is not active")
	} else if err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	if err := requireAdmittedProcessorRuntime(admissionEvidence); err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	// Exact image availability is the hard filter. A fixture is placed on a
	// Worker-owned postprocess queue only after a current admitted image match.
	var workerID int64
	var instanceID string
	err = s.db.QueryRow(ctx, `SELECT i.worker_id,i.instance_id FROM workforce_worker_images i JOIN workforce_workers w ON w.id=i.worker_id JOIN workforce_worker_sessions s ON s.worker_id=w.id AND s.instance_id=w.current_instance_id WHERE i.image_digest=$1 AND i.role='attempt' AND i.admission_status='ADMITTED' AND i.presence_status='PRESENT' AND w.desired_state='ENABLED' AND s.consumer_state='CONSUMING' AND s.docker_ready=true AND s.last_heartbeat_at>=CURRENT_TIMESTAMP-INTERVAL '35 seconds' ORDER BY s.last_heartbeat_at DESC,i.worker_id LIMIT 1`, imageDigest).Scan(&workerID, &instanceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return FixtureRun{}, FixtureMessage{}, fmt.Errorf("RESULT_PROCESSOR_FIXTURE_WORKER_UNAVAILABLE")
	}
	if err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	run := FixtureRun{ID: s.idGen.NextID(), TenantID: tenantID, RevisionID: revisionID, HarnessReleaseID: harnessReleaseID, HarnessImageDigest: imageDigest, EvaluationContext: evaluationContext, WorkspaceFiles: workspaceFiles, ConfigOverrides: configOverrides, ExpectedDecision: expectedDecision, Status: "PENDING", WorkerID: workerID, WorkerInstanceID: instanceID, Generation: 1}
	message := FixtureMessage{SchemaVersion: 1, Kind: FixtureMessageKind, MessageID: s.idGen.NextID(), WorkerID: workerID, RunID: run.ID, Generation: 1}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	defer tx.Rollback(ctx)
	principalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_result_processor_fixture_runs (id,tenant_id,revision_id,revision_manifest_json,revision_files_json,revision_digest,harness_release_id,harness_image_digest,evaluation_context_json,workspace_files_json,config_overrides_json,expected_decision,status,worker_id,worker_instance_id,generation,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,'PENDING',$13,$14,$15,$16)`, run.ID, tenantID, revisionID, mustJSON(manifest), mustJSON(files), revisionDigest, harnessReleaseID, imageDigest, evaluationContext, workspaceFiles, configOverrides, expectedDecision, workerID, instanceID, run.Generation, principalID); err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	payload := mustJSON(message)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status,exchange_name,routing_key,message_expiration_ms) VALUES ($1,'result_processor_fixture',$2,'result_processor.fixture.requested',$3,'PENDING','seven.workforce.postprocess',$4,600000)`, message.MessageID, run.ID, payload, "postprocess."+strconv.FormatInt(workerID, 10)); err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return FixtureRun{}, FixtureMessage{}, err
	}
	return run, message, nil
}

func (s *Store) FixtureRun(ctx context.Context, tenantID int64, runID int64) (FixtureRun, error) {
	var item FixtureRun
	err := s.db.QueryRow(ctx, `SELECT id,tenant_id,revision_id,harness_release_id,harness_image_digest,evaluation_context_json,workspace_files_json,config_overrides_json,expected_decision,status,COALESCE(worker_id,0),COALESCE(worker_instance_id,''),generation,COALESCE(result_contract_json,'null'::jsonb),result_contract_sha256,reason_code,diagnostics,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_result_processor_fixture_runs WHERE tenant_id=$1 AND id=$2`, tenantID, runID).Scan(&item.ID, &item.TenantID, &item.RevisionID, &item.HarnessReleaseID, &item.HarnessImageDigest, &item.EvaluationContext, &item.WorkspaceFiles, &item.ConfigOverrides, &item.ExpectedDecision, &item.Status, &item.WorkerID, &item.WorkerInstanceID, &item.Generation, &item.ResultContract, &item.ResultContractSHA256, &item.ReasonCode, &item.Diagnostics, &item.CreatedAt, &item.StartedAt, &item.FinishedAt)
	return item, classifyNotFound(err)
}

func (s *Store) environmentBindings(ctx context.Context, tenantID int64, releaseID int64) ([]EnvironmentBinding, error) {
	rows, err := s.db.Query(ctx, `SELECT id,tenant_id,processor_release_id,name,type,phase,COALESCE(config_json,'null'::jsonb),secret_ref,secret_version,task_override_allowed,network_policy FROM workforce_result_processor_environment_bindings WHERE tenant_id=$1 AND processor_release_id=$2 ORDER BY name`, tenantID, releaseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []EnvironmentBinding{}
	for rows.Next() {
		var item EnvironmentBinding
		if err := rows.Scan(&item.ID, &item.TenantID, &item.ProcessorReleaseID, &item.Name, &item.Type, &item.Phase, &item.Config, &item.SecretRef, &item.SecretVersion, &item.TaskOverrideAllowed, &item.NetworkPolicy); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// RegisterPersistedSecretReferences rebuilds the ConfigFacade consumer
// registry after a Server restart. The registrations contain only declared
// config references, never values or ciphertext; the exact value/version is
// still checked at each EvaluationCycle materialization.
func (s *Store) RegisterPersistedSecretReferences(ctx context.Context) error {
	registrar, ok := s.environmentResolver.(EnvironmentSecretRegistrar)
	if !ok || registrar == nil {
		return fmt.Errorf("RESULT_PROCESSOR_SECRET_RESOLVER_UNAVAILABLE")
	}
	rows, err := s.db.Query(ctx, `SELECT DISTINCT secret_ref FROM workforce_result_processor_environment_bindings WHERE type='SECRET' AND secret_ref<>'' ORDER BY secret_ref`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return err
		}
		if err := registrar.RegisterResultProcessorSecretRef(ctx, ref); err != nil {
			return err
		}
	}
	return rows.Err()
}

func scanRevision(row pgx.Row, target *Revision) error {
	var manifest, files, validation json.RawMessage
	err := row.Scan(&target.ID, &target.ProcessorID, &target.ScopeType, &target.TenantID, &target.RevisionNumber, &target.DraftVersion, &target.Status, &manifest, &files, &validation, &target.CreatedAt, &target.UpdatedAt)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(manifest, &target.Manifest); err != nil {
		return err
	}
	if err = json.Unmarshal(files, &target.Files); err != nil {
		return err
	}
	if len(validation) > 0 && string(validation) != "null" {
		_ = json.Unmarshal(validation, &target.Validation)
	}
	return nil
}
func scanRelease(row pgx.Row, target *Release) error {
	var manifest, files json.RawMessage
	err := row.Scan(&target.ID, &target.ProcessorID, &target.ScopeType, &target.TenantID, &target.RevisionID, &target.Version, &manifest, &files, &target.ArchiveObjectKey, &target.ArchiveSHA256, &target.ArchiveSizeBytes, &target.PublishedAt)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(manifest, &target.Manifest); err != nil {
		return err
	}
	return json.Unmarshal(files, &target.Files)
}
func mustJSON(value any) json.RawMessage { body, _ := json.Marshal(value); return body }
func classifyNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func classifyConflict(err error) error {
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		return ErrConflict
	}
	return err
}
func ternary(test bool, yes, no string) string {
	if test {
		return yes
	}
	return no
}
func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return value
}
func nullScopeTenant(scopeType string, tenantID int64) any {
	if scopeType == "SYSTEM" {
		return nil
	}
	return tenantID
}
func validJSONObject(value json.RawMessage) bool {
	var out map[string]any
	return json.Unmarshal(value, &out) == nil
}

type frozenEnvironment struct {
	Parameters json.RawMessage          `json:"parameters"`
	Bindings   []frozenEnvironmentEntry `json:"bindings"`
}

type frozenEnvironmentEntry struct {
	Name                string          `json:"name"`
	Type                string          `json:"type"`
	Phase               string          `json:"phase"`
	TaskOverrideAllowed bool            `json:"taskOverrideAllowed"`
	NetworkPolicy       string          `json:"networkPolicy"`
	Config              json.RawMessage `json:"config,omitempty"`
	SecretRef           string          `json:"secretRef,omitempty"`
	SecretVersion       string          `json:"secretVersion,omitempty"`
}

func canonicalEnvironment(bindings []EnvironmentBinding, parameters, overrides json.RawMessage) (frozenEnvironment, json.RawMessage, error) {
	var overrideValues map[string]json.RawMessage
	if err := json.Unmarshal(overrides, &overrideValues); err != nil || overrideValues == nil {
		return frozenEnvironment{}, nil, fmt.Errorf("resultProcessorConfigOverrides must be a JSON object")
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Name < bindings[j].Name })
	config := frozenEnvironment{Parameters: append(json.RawMessage(nil), parameters...), Bindings: make([]frozenEnvironmentEntry, 0, len(bindings))}
	secrets := make([]map[string]string, 0)
	for _, binding := range bindings {
		item := frozenEnvironmentEntry{Name: binding.Name, Type: binding.Type, Phase: binding.Phase, TaskOverrideAllowed: binding.TaskOverrideAllowed, NetworkPolicy: binding.NetworkPolicy}
		if binding.Type == "SECRET" {
			if _, overridden := overrideValues[binding.Name]; overridden {
				return frozenEnvironment{}, nil, fmt.Errorf("resultProcessorConfigOverrides.%s cannot override SECRET", binding.Name)
			}
			item.SecretRef = binding.SecretRef
			item.SecretVersion = binding.SecretVersion
			secrets = append(secrets, map[string]string{"name": binding.Name, "ref": binding.SecretRef, "version": binding.SecretVersion})
		} else {
			item.Config = append(json.RawMessage(nil), binding.Config...)
			if override, overridden := overrideValues[binding.Name]; overridden {
				if !binding.TaskOverrideAllowed {
					return frozenEnvironment{}, nil, fmt.Errorf("resultProcessorConfigOverrides.%s is not allowed", binding.Name)
				}
				if !json.Valid(override) {
					return frozenEnvironment{}, nil, fmt.Errorf("resultProcessorConfigOverrides.%s is invalid JSON", binding.Name)
				}
				item.Config = append(json.RawMessage(nil), override...)
			}
		}
		config.Bindings = append(config.Bindings, item)
		delete(overrideValues, binding.Name)
	}
	if len(overrideValues) != 0 {
		keys := make([]string, 0, len(overrideValues))
		for key := range overrideValues {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		return frozenEnvironment{}, nil, fmt.Errorf("resultProcessorConfigOverrides.%s is not declared", keys[0])
	}
	body := mustJSON(secrets)
	return config, body, nil
}
func shaJSON(value any) string {
	body := mustJSON(value)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func validWorkspaceFiles(value json.RawMessage) bool {
	var files []struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if json.Unmarshal(value, &files) != nil || len(files) > 256 {
		return false
	}
	var total int
	for _, file := range files {
		if strings.TrimSpace(file.Path) == "" || len(file.Content) > 2<<20 {
			return false
		}
		total += len(file.Content)
		if total > 8<<20 {
			return false
		}
	}
	return true
}

func (s *Store) userPrincipal(ctx context.Context, actor int64) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	id, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) PlanningRelease(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `SELECT r.id FROM workforce_result_processors p JOIN workforce_result_processor_releases r ON r.processor_id=p.id WHERE p.scope_type='SYSTEM' AND p.name='开发方案处理器' AND r.version='1.0.0' ORDER BY r.id DESC LIMIT 1`).Scan(&id)
	return id, err
}
