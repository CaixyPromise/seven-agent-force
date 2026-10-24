package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/domain"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
)

var ErrNotFound = errors.New("hook resource not found")
var ErrConflict = errors.New("hook resource conflict")

type Store struct {
	db    *pgxpool.Pool
	idGen IDGenerator
}
type IDGenerator interface{ NextID() int64 }

func NewStore(db *pgxpool.Pool, idGen IDGenerator) *Store {
	return &Store{db: db, idGen: idGen}
}

type Package struct {
	ID             int64               `json:"id,string"`
	TenantID       int64               `json:"tenantId,string"`
	Name           string              `json:"name"`
	Category       string              `json:"category"`
	Description    string              `json:"description"`
	TrustClass     string              `json:"trustClass"`
	RevisionID     int64               `json:"revisionId,string"`
	Revision       int64               `json:"revision"`
	RevisionStatus string              `json:"revisionStatus"`
	Manifest       domain.Manifest     `json:"manifest"`
	Files          []domain.SourceFile `json:"files"`
	Validation     json.RawMessage     `json:"validation,omitempty"`
	LatestVersion  string              `json:"latestVersion,omitempty"`
	UpdatedAt      string              `json:"updatedAt"`
}

type Release struct {
	ID               int64               `json:"id,string"`
	PackageID        int64               `json:"packageId,string"`
	Version          string              `json:"version"`
	RuntimeKind      string              `json:"runtimeKind"`
	Manifest         domain.Manifest     `json:"manifest"`
	Files            []domain.SourceFile `json:"files"`
	ArchiveSHA256    string              `json:"archiveSha256"`
	ArchiveSizeBytes int64               `json:"archiveSizeBytes"`
	PublishedAt      string              `json:"publishedAt"`
}

type AuditEvent struct {
	EventType  string          `json:"eventType"`
	ResourceID int64           `json:"resourceId,string"`
	Status     string          `json:"status"`
	Actor      int64           `json:"actor,string"`
	Details    json.RawMessage `json:"details"`
	CreatedAt  string          `json:"createdAt"`
}

// HarnessChannel is exposed only to the Result Processor fixture runner. It
// is not a Hook binding or a Project configuration surface.
type HarnessChannel struct {
	Alias             string `json:"alias"`
	HarnessReleaseID  int64  `json:"harnessReleaseId,string"`
	HarnessType       string `json:"harnessType"`
	HarnessVersion    string `json:"harnessVersion"`
	ImageDigest       string `json:"imageDigest"`
	SupervisorVersion string `json:"supervisorVersion"`
}

func (s *Store) ListHarnessChannels(ctx context.Context) ([]HarnessChannel, error) {
	rows, err := s.db.Query(ctx, `SELECT c.alias,r.id,r.harness_type,r.harness_version,r.image_digest,r.supervisor_version FROM workforce_harness_channels c JOIN workforce_harness_releases r ON r.id=c.harness_release_id WHERE r.status='ACTIVE' ORDER BY c.alias`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []HarnessChannel{}
	for rows.Next() {
		var item HarnessChannel
		if err := rows.Scan(&item.Alias, &item.HarnessReleaseID, &item.HarnessType, &item.HarnessVersion, &item.ImageDigest, &item.SupervisorVersion); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreatePackage(ctx context.Context, tenant int64, actor int64, name string, category string, description string, manifest domain.Manifest, files []domain.SourceFile) (Package, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Package{}, err
	}
	defer tx.Rollback(ctx)
	actorPrincipalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return Package{}, err
	}
	packageID, revisionID := s.idGen.NextID(), s.idGen.NextID()
	manifestJSON, _ := json.Marshal(manifest)
	filesJSON, _ := json.Marshal(files)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_hook_packages (id,tenant_id,name,category,description,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6)`, packageID, tenant, name, category, description, actorPrincipalID); err != nil {
		if strings.Contains(err.Error(), "unique") {
			return Package{}, ErrConflict
		}
		return Package{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_hook_package_revisions (id,package_id,tenant_id,manifest_json,files_json,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6)`, revisionID, packageID, tenant, manifestJSON, filesJSON, actorPrincipalID); err != nil {
		return Package{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Package{}, err
	}
	return s.GetPackage(ctx, tenant, packageID)
}

func (s *Store) ListPackages(ctx context.Context, tenant int64) ([]Package, error) {
	rows, err := s.db.Query(ctx, `
SELECT p.id,p.tenant_id,p.name,p.category,p.description,p.trust_class,r.id,r.revision,r.status,r.manifest_json,r.files_json,COALESCE(r.validation_json,'null'::jsonb),
'',to_char(p.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM workforce_hook_packages p JOIN LATERAL (SELECT * FROM workforce_hook_package_revisions x WHERE x.package_id=p.id ORDER BY x.created_at DESC LIMIT 1) r ON true
WHERE p.tenant_id=$1 ORDER BY p.updated_at DESC LIMIT 200`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Package{}
	for rows.Next() {
		item, err := scanPackage(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	latestRows, err := s.db.Query(ctx, `SELECT DISTINCT ON (package_id) package_id,version FROM workforce_hook_package_releases WHERE tenant_id=$1 ORDER BY package_id,published_at DESC`, tenant)
	if err != nil {
		return nil, err
	}
	defer latestRows.Close()
	latest := map[int64]string{}
	for latestRows.Next() {
		var packageID int64
		var version string
		if err := latestRows.Scan(&packageID, &version); err != nil {
			return nil, err
		}
		latest[packageID] = version
	}
	for index := range result {
		result[index].LatestVersion = latest[result[index].ID]
	}
	return result, latestRows.Err()
}

func (s *Store) GetPackage(ctx context.Context, tenant int64, id int64) (Package, error) {
	row := s.db.QueryRow(ctx, `
SELECT p.id,p.tenant_id,p.name,p.category,p.description,p.trust_class,r.id,r.revision,r.status,r.manifest_json,r.files_json,COALESCE(r.validation_json,'null'::jsonb),
'',to_char(p.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
FROM workforce_hook_packages p JOIN LATERAL (SELECT * FROM workforce_hook_package_revisions x WHERE x.package_id=p.id ORDER BY x.created_at DESC LIMIT 1) r ON true
WHERE p.tenant_id=$1 AND p.id=$2`, tenant, id)
	item, err := scanPackage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Package{}, ErrNotFound
	}
	if err == nil {
		_ = s.db.QueryRow(ctx, `SELECT version FROM workforce_hook_package_releases WHERE tenant_id=$1 AND package_id=$2 ORDER BY published_at DESC LIMIT 1`, tenant, id).Scan(&item.LatestVersion)
	}
	return item, err
}

type scanner interface{ Scan(...any) error }

func scanPackage(row scanner) (Package, error) {
	var item Package
	var manifest, files []byte
	err := row.Scan(&item.ID, &item.TenantID, &item.Name, &item.Category, &item.Description, &item.TrustClass, &item.RevisionID, &item.Revision, &item.RevisionStatus, &manifest, &files, &item.Validation, &item.LatestVersion, &item.UpdatedAt)
	if err != nil {
		return Package{}, err
	}
	_ = json.Unmarshal(manifest, &item.Manifest)
	_ = json.Unmarshal(files, &item.Files)
	return item, nil
}

func (s *Store) CreateRevision(ctx context.Context, tenant int64, packageID int64, actor int64) (Package, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Package{}, err
	}
	defer tx.Rollback(ctx)
	actorPrincipalID, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return Package{}, err
	}
	var manifestJSON, filesJSON []byte
	var currentStatus string
	err = tx.QueryRow(ctx, `SELECT manifest_json,files_json,status FROM workforce_hook_package_revisions WHERE tenant_id=$1 AND package_id=$2 ORDER BY created_at DESC LIMIT 1 FOR UPDATE`, tenant, packageID).Scan(&manifestJSON, &filesJSON, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return Package{}, ErrNotFound
	}
	if err != nil {
		return Package{}, err
	}
	if currentStatus == "DRAFT" {
		return Package{}, ErrConflict
	}
	revisionID := s.idGen.NextID()
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_hook_package_revisions (id,package_id,tenant_id,manifest_json,files_json,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6)`, revisionID, packageID, tenant, manifestJSON, filesJSON, actorPrincipalID); err != nil {
		return Package{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_hook_packages SET updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND tenant_id=$2`, packageID, tenant); err != nil {
		return Package{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Package{}, err
	}
	return s.GetPackage(ctx, tenant, packageID)
}

func (s *Store) UpdateRevision(ctx context.Context, tenant int64, revisionID int64, expected int64, manifest domain.Manifest, files []domain.SourceFile) (Package, error) {
	manifestJSON, _ := json.Marshal(manifest)
	filesJSON, _ := json.Marshal(files)
	command, err := s.db.Exec(ctx, `UPDATE workforce_hook_package_revisions SET manifest_json=$4,files_json=$5,validation_json=NULL,revision=revision+1,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND tenant_id=$2 AND revision=$3 AND status='DRAFT'`, revisionID, tenant, expected, manifestJSON, filesJSON)
	if err != nil {
		return Package{}, err
	}
	if command.RowsAffected() != 1 {
		return Package{}, ErrConflict
	}
	var packageID int64
	if err := s.db.QueryRow(ctx, `SELECT package_id FROM workforce_hook_package_revisions WHERE id=$1 AND tenant_id=$2`, revisionID, tenant).Scan(&packageID); err != nil {
		return Package{}, ErrNotFound
	}
	_, _ = s.db.Exec(ctx, `UPDATE workforce_hook_packages SET updated_at=CURRENT_TIMESTAMP WHERE id=$1`, packageID)
	return s.GetPackage(ctx, tenant, packageID)
}

func (s *Store) SetValidation(ctx context.Context, tenant int64, revisionID int64, validation domain.Validation) error {
	body, _ := json.Marshal(validation)
	command, err := s.db.Exec(ctx, `UPDATE workforce_hook_package_revisions SET validation_json=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND tenant_id=$2`, revisionID, tenant, body)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Revision(ctx context.Context, tenant int64, revisionID int64) (Package, error) {
	var packageID int64
	if err := s.db.QueryRow(ctx, `SELECT package_id FROM workforce_hook_package_revisions WHERE id=$1 AND tenant_id=$2`, revisionID, tenant).Scan(&packageID); errors.Is(err, pgx.ErrNoRows) {
		return Package{}, ErrNotFound
	} else if err != nil {
		return Package{}, err
	}
	return s.GetPackage(ctx, tenant, packageID)
}

func (s *Store) CreateTestRun(ctx context.Context, tenant int64, revisionID int64, actor int64, event string, fixture any, result domain.HookResult, diagnostics string, durationMS int64) (int64, error) {
	id := s.idGen.NextID()
	fixtureJSON, _ := json.Marshal(fixture)
	principalID, err := s.userPrincipal(ctx, actor)
	if err != nil {
		return 0, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO workforce_hook_test_runs (id,revision_id,tenant_id,event_name,fixture_json,decision,reason_code,diagnostics,duration_ms,created_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, revisionID, tenant, event, fixtureJSON, result.Decision, result.ReasonCode, diagnostics, durationMS, principalID)
	return id, err
}

func (s *Store) Publish(ctx context.Context, tenant int64, revisionID int64, actor int64, version string, objectKey string, digest string, size int64) (Release, error) {
	revision, err := s.Revision(ctx, tenant, revisionID)
	if err != nil {
		return Release{}, err
	}
	manifest := revision.Manifest
	manifest.Version = version
	manifestJSON, _ := json.Marshal(manifest)
	filesJSON, _ := json.Marshal(revision.Files)
	releaseID := s.idGen.NextID()
	principalID, err := s.userPrincipal(ctx, actor)
	if err != nil {
		return Release{}, err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO workforce_hook_package_releases (id,package_id,revision_id,tenant_id,version,runtime_kind,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,published_by_principal_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, releaseID, revision.ID, revisionID, tenant, version, manifest.Runtime.Kind, manifestJSON, filesJSON, objectKey, digest, size, principalID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			var existing Release
			var existingManifest, existingFiles []byte
			lookupErr := s.db.QueryRow(ctx, `SELECT id,package_id,version,runtime_kind,manifest_json,files_json,archive_sha256,archive_size_bytes,to_char(published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_hook_package_releases WHERE tenant_id=$1 AND package_id=$2 AND version=$3`, tenant, revision.ID, version).Scan(&existing.ID, &existing.PackageID, &existing.Version, &existing.RuntimeKind, &existingManifest, &existingFiles, &existing.ArchiveSHA256, &existing.ArchiveSizeBytes, &existing.PublishedAt)
			if lookupErr == nil && existing.ArchiveSHA256 == digest {
				_ = json.Unmarshal(existingManifest, &existing.Manifest)
				_ = json.Unmarshal(existingFiles, &existing.Files)
				return existing, nil
			}
			return Release{}, ErrConflict
		}
		return Release{}, err
	}
	_, _ = s.db.Exec(ctx, `UPDATE workforce_hook_package_revisions SET status='PUBLISHED' WHERE id=$1`, revisionID)
	return Release{ID: releaseID, PackageID: revision.ID, Version: version, RuntimeKind: manifest.Runtime.Kind, Manifest: manifest, Files: revision.Files, ArchiveSHA256: digest, ArchiveSizeBytes: size}, nil
}

func (s *Store) ListReleases(ctx context.Context, tenant int64) ([]Release, error) {
	rows, err := s.db.Query(ctx, `SELECT id,package_id,version,runtime_kind,manifest_json,files_json,archive_sha256,archive_size_bytes,to_char(published_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM workforce_hook_package_releases WHERE tenant_id=$1 ORDER BY published_at DESC LIMIT 500`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Release{}
	for rows.Next() {
		var item Release
		var m, f []byte
		if err := rows.Scan(&item.ID, &item.PackageID, &item.Version, &item.RuntimeKind, &m, &f, &item.ArchiveSHA256, &item.ArchiveSizeBytes, &item.PublishedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(m, &item.Manifest)
		_ = json.Unmarshal(f, &item.Files)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) ListAudit(ctx context.Context, tenant int64) ([]AuditEvent, error) {
	rows, err := s.db.Query(ctx, `
SELECT event_type,resource_id,status,actor,details,to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"') FROM (
  SELECT 'hook.published'::text event_type,id resource_id,'PUBLISHED'::text status,published_by_principal_id actor,
         jsonb_build_object('hookId',package_id,'version',version,'digest',archive_sha256) details,published_at created_at
    FROM workforce_hook_package_releases WHERE tenant_id=$1
  UNION ALL
  SELECT 'hook.test.completed',id,decision,created_by_principal_id,
         jsonb_build_object('revisionId',revision_id,'event',event_name,'reasonCode',reason_code,'durationMs',duration_ms),created_at
    FROM workforce_hook_test_runs WHERE tenant_id=$1
) audit ORDER BY created_at DESC LIMIT 500`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AuditEvent{}
	for rows.Next() {
		var item AuditEvent
		if err := rows.Scan(&item.EventType, &item.ResourceID, &item.Status, &item.Actor, &item.Details, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
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
