package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	gitdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type IDGenerator interface{ NextID() int64 }
type Store struct {
	db    *pgxpool.Pool
	idGen IDGenerator
}

func New(db *pgxpool.Pool, idGen IDGenerator) *Store { return &Store{db: db, idGen: idGen} }

func (s *Store) GetProjectRepository(ctx context.Context, tenant, projectID, versionID int64) (gitdomain.RepositoryTarget, error) {
	item, err := s.GetVersion(ctx, tenant, projectID, versionID)
	if err != nil || item.Kind != "CODEBASE" || item.RepositoryConfigDigest == "" {
		return gitdomain.RepositoryTarget{}, domain.ErrInvalid
	}
	transport := gitdomain.TransportHTTPS
	if item.RepositoryScheme == "http" {
		transport = gitdomain.TransportHTTP
	}
	return gitdomain.RepositoryTarget{ID: item.ID, TenantID: item.TenantID, Name: item.RepositoryDisplayURL, ProviderType: item.RepositoryProviderType, Transport: transport, Host: item.RepositoryHost, Port: item.RepositoryPort, RepositoryPath: item.RepositoryPath, WebRepositoryURL: item.RepositoryWebURL, Status: gitdomain.RepositoryEnabled, ConfigDigest: item.RepositoryConfigDigest, Read: gitdomain.AccessBinding{Capability: gitdomain.CapabilityReadSource, Status: gitdomain.CapabilityVerified}, Push: gitdomain.AccessBinding{Capability: gitdomain.CapabilityPushBranch, Status: capabilityState(item.RepositoryCapabilities.Push)}, Review: gitdomain.AccessBinding{Capability: gitdomain.CapabilityCreateReview, Status: capabilityState(item.RepositoryCapabilities.DraftReview)}, Egress: gitdomain.EgressPolicy{AllowPrivateNetwork: true, AllowedHosts: []string{item.RepositoryHost}, AllowedPorts: []int{item.RepositoryPort}, AllowedCIDRs: []string{"0.0.0.0/0", "::/0"}}}, nil
}
func capabilityState(ok bool) string {
	if ok {
		return gitdomain.CapabilityVerified
	}
	return gitdomain.CapabilityFailed
}

func (s *Store) Create(ctx context.Context, tenant, actor int64, request projectapp.UpsertRequest) (domain.Project, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	defer tx.Rollback(ctx)
	principal, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return domain.Project{}, err
	}
	projectID, versionID := s.idGen.NextID(), s.idGen.NextID()
	repo := repository(request)
	var storedVersion int64
	var storedDigest string
	err = tx.QueryRow(ctx, `INSERT INTO workforce_projects(id,tenant_id,project_key,status,current_version_id,current_source_state,current_source_revision_id,created_by_principal_id,updated_by_principal_id,create_idempotency_key,create_request_digest) VALUES($1,$2,$3,'ACTIVE',$4,NULLIF($5,''),NULLIF($6::BIGINT,0),$7,$7,$8,$9) ON CONFLICT(tenant_id,created_by_principal_id,create_idempotency_key) DO UPDATE SET updated_at=workforce_projects.updated_at RETURNING id,current_version_id,create_request_digest`, projectID, tenant, request.ProjectKey, versionID, repo.SourceState, repo.SourceRevisionID, principal, request.IdempotencyKey, request.CreateRequestDigest).Scan(&projectID, &storedVersion, &storedDigest)
	if err != nil {
		return domain.Project{}, err
	}
	if storedDigest != request.CreateRequestDigest {
		return domain.Project{}, domain.ErrIdempotencyConflict
	}
	if storedVersion == versionID {
		if err = s.insertVersion(ctx, tx, tenant, projectID, versionID, principal, request); err != nil {
			return domain.Project{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Project{}, err
	}
	return s.Get(ctx, tenant, projectID)
}

func (s *Store) Update(ctx context.Context, tenant, actor, projectID int64, request projectapp.UpsertRequest) (domain.Project, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	defer tx.Rollback(ctx)
	principal, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return domain.Project{}, err
	}
	var currentVersion int64
	var status string
	err = tx.QueryRow(ctx, `SELECT current_version_id,status FROM workforce_projects WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, projectID).Scan(&currentVersion, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Project{}, err
	}
	if status != "ACTIVE" {
		return domain.Project{}, domain.ErrArchived
	}
	if currentVersion != request.ExpectedVersionID {
		return domain.Project{}, domain.ErrConflict
	}
	versionID := s.idGen.NextID()
	if err = s.insertVersion(ctx, tx, tenant, projectID, versionID, principal, request); err != nil {
		return domain.Project{}, err
	}
	repo := repository(request)
	result, err := tx.Exec(ctx, `UPDATE workforce_projects SET current_version_id=$3,current_source_state=NULLIF($4,''),current_source_revision_id=NULLIF($5::BIGINT,0),updated_by_principal_id=$6,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND id=$2 AND current_version_id=$7`, tenant, projectID, versionID, repo.SourceState, repo.SourceRevisionID, principal, request.ExpectedVersionID)
	if err != nil {
		return domain.Project{}, err
	}
	if result.RowsAffected() != 1 {
		return domain.Project{}, domain.ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Project{}, err
	}
	return s.Get(ctx, tenant, projectID)
}

func (s *Store) insertVersion(ctx context.Context, tx pgx.Tx, tenant, projectID, versionID, principal int64, request projectapp.UpsertRequest) error {
	check, digest := completion(request)
	repo := repository(request)
	caps, _ := json.Marshal(repo.Capabilities)
	_, err := tx.Exec(ctx, `INSERT INTO workforce_project_versions(id,tenant_id,project_id,name,description,department_id,kind,repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_display_url,repository_web_url,repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_credential_version,repository_config_digest,repository_capabilities_json,repository_last_check_status,repository_last_check_reason_code,repository_last_checked_at,default_ref_name,tech_stack_json,agent_guidance,completion_check_json,completion_check_digest,result_processor_release_id,result_processor_parameters_json,review_prompt,delivery_mode,delivery_authorization,delivery_authorized_by_principal_id,result_branch_pattern,notification_enabled,notification_events_json,notification_group_route_id,hook_release_ids,created_by_principal_id) VALUES($1,$2,$3,$4,$5,NULLIF($6::BIGINT,0),$7::VARCHAR,CASE WHEN $7::VARCHAR='CODEBASE' THEN $8 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $9 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $10 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $11::INTEGER ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $12 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $13 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $14 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN decode(NULLIF($15,''),'base64') ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN decode(NULLIF($16,''),'base64') ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN NULLIF($17,'') ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN NULLIF($18::INTEGER,0) ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $19 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $20::JSONB ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $21 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $22 ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN CURRENT_TIMESTAMP ELSE NULL END,CASE WHEN $7::VARCHAR='CODEBASE' THEN $23 ELSE NULL END,$24::JSONB,$25,$26::JSONB,$27,$28,$29::JSONB,$30,$31,$32::VARCHAR,CASE WHEN $32::VARCHAR='AUTO_AFTER_GATES' THEN $38::BIGINT ELSE NULL END,$33,$34,$35::JSONB,NULLIF($36::BIGINT,0),COALESCE($37::BIGINT[],'{}'::BIGINT[]),$38)`, versionID, tenant, projectID, request.Name, request.Description, request.DepartmentID, request.Kind, repo.ProviderType, repo.Scheme, repo.Host, repo.Port, repo.Path, repo.DisplayURL, repo.WebURL, repo.CredentialCiphertext, repo.CredentialEDEK, repo.CredentialWrapKeyRef, repo.CredentialVersion, repo.ConfigDigest, caps, repo.LastCheckStatus, repo.LastCheckReasonCode, repo.DefaultRefName, marshalStringList(request.TechStack), request.AgentGuidance, check, digest, request.ResolvedProcessorReleaseID, json.RawMessage(`{}`), request.ReviewPrompt, request.Delivery.Mode, request.Delivery.Authorization, request.Delivery.BranchPattern, request.Notification.Enabled, marshalStringList(request.Notification.Events), request.Notification.GroupRouteID, request.HookReleaseIDs, principal)
	return err
}

func (s *Store) Get(ctx context.Context, tenant, id int64) (domain.Project, error) {
	p, err := scan(s.db.QueryRow(ctx, projectSelect+` WHERE p.tenant_id=$1 AND p.id=$2`, tenant, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, domain.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	items := []domain.Project{p}
	if err = s.enrichProjects(ctx, tenant, items); err != nil {
		return domain.Project{}, err
	}
	return items[0], nil
}
func (s *Store) List(ctx context.Context, tenant int64, request projectapp.ListRequest) (projectapp.ProjectPage, error) {
	rows, err := s.db.Query(ctx, projectSelect+` WHERE p.tenant_id=$1 AND ($2='' OR p.status=$2) AND ($3='' OR strpos(lower(v.name),$3)>0 OR strpos(lower(v.description),$3)>0 OR strpos(lower(p.project_key),$3)>0 OR strpos(lower(COALESCE(v.repository_display_url,'')),$3)>0) AND ($4::BIGINT=0 OR p.id<$4) ORDER BY p.id DESC LIMIT $5`, tenant, request.Status, request.Query, request.Cursor, request.Limit+1)
	if err != nil {
		return projectapp.ProjectPage{}, err
	}
	defer rows.Close()
	items := []domain.Project{}
	for rows.Next() {
		item, e := scan(rows)
		if e != nil {
			return projectapp.ProjectPage{}, e
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return projectapp.ProjectPage{}, err
	}
	page := projectapp.ProjectPage{}
	if len(items) > request.Limit {
		items = items[:request.Limit]
		page.NextCursor = items[len(items)-1].ID
	}
	if err = s.enrichProjects(ctx, tenant, items); err != nil {
		return projectapp.ProjectPage{}, err
	}
	page.Items = items
	return page, nil
}
func (s *Store) Archive(ctx context.Context, tenant, actor, id, expected int64) (domain.Project, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return domain.Project{}, err
	}
	defer tx.Rollback(ctx)
	principal, err := principalinfra.EnsureUser(ctx, tx, s.idGen, actor)
	if err != nil {
		return domain.Project{}, err
	}
	result, err := tx.Exec(ctx, `UPDATE workforce_projects SET status='ARCHIVED',archived_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP,updated_by_principal_id=$5 WHERE tenant_id=$1 AND id=$2 AND status='ACTIVE' AND current_version_id=$3 AND $4::BIGINT>0`, tenant, id, expected, actor, principal)
	if err != nil {
		return domain.Project{}, err
	}
	if result.RowsAffected() != 1 {
		return domain.Project{}, domain.ErrConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Project{}, err
	}
	return s.Get(ctx, tenant, id)
}
func (s *Store) LatestAvailableSource(ctx context.Context, tenant, projectID, versionID int64) (int64, error) {
	var id int64
	err := s.db.QueryRow(ctx, `SELECT id FROM workforce_workspace_source_revisions WHERE tenant_id=$1 AND project_id=$2 AND project_version_id=$3 AND status='AVAILABLE' ORDER BY created_at DESC,id DESC LIMIT 1`, tenant, projectID, versionID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrSourceUnavailable
	}
	return id, err
}
func (s *Store) ListPreparingSources(ctx context.Context, limit int) ([]projectapp.PreparingSource, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.Query(ctx, `SELECT p.tenant_id,wp.user_id,p.id,p.current_version_id FROM workforce_projects p JOIN workforce_principals wp ON wp.id=p.updated_by_principal_id AND wp.principal_type='USER' WHERE p.status='ACTIVE' AND p.current_source_state='PREPARING' ORDER BY p.updated_at,p.id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []projectapp.PreparingSource
	for rows.Next() {
		var item projectapp.PreparingSource
		if err = rows.Scan(&item.TenantID, &item.ActorID, &item.ProjectID, &item.VersionID); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Store) MarkSourcePreparationFailed(ctx context.Context, tenant, projectID, versionID int64) error {
	_, err := s.db.Exec(ctx, `UPDATE workforce_projects SET current_source_state='FAILED',updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND id=$2 AND current_version_id=$3 AND current_source_state='PREPARING'`, tenant, projectID, versionID)
	return err
}

func (s *Store) ResolveProcessorRelease(ctx context.Context, tenant, releaseID int64) (int64, error) {
	if releaseID > 0 {
		var exists bool
		err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_result_processor_releases WHERE id=$1 AND ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id=$2)))`, releaseID, tenant).Scan(&exists)
		if err != nil {
			return 0, err
		}
		if !exists {
			return 0, domain.ErrInvalid
		}
		return releaseID, nil
	}
	err := s.db.QueryRow(ctx, `SELECT b.processor_release_id FROM workforce_result_processor_bindings b JOIN workforce_result_processor_releases r ON r.id=b.processor_release_id WHERE b.tenant_id=$1 AND b.binding_name='default' LIMIT 1`, tenant).Scan(&releaseID)
	if err == nil {
		return releaseID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	err = s.db.QueryRow(ctx, `SELECT r.id FROM workforce_result_processor_releases r JOIN workforce_result_processors p ON p.id=r.processor_id WHERE r.scope_type='SYSTEM' AND r.tenant_id IS NULL AND p.scope_type='SYSTEM' AND p.tenant_id IS NULL AND p.name='标准代码任务处理器' ORDER BY r.published_at DESC,r.id DESC LIMIT 1`).Scan(&releaseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, domain.ErrInvalid
	}
	return releaseID, err
}
func (s *Store) ValidateProjectReferences(ctx context.Context, tenant int64, request projectapp.UpsertRequest) error {
	if len(request.HookReleaseIDs) > 0 {
		var count int
		if err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM workforce_hook_package_releases r WHERE r.id=ANY($1::BIGINT[]) AND ((r.scope_type='SYSTEM' AND r.tenant_id IS NULL) OR (r.scope_type='TENANT' AND r.tenant_id=$2))`, request.HookReleaseIDs, tenant).Scan(&count); err != nil {
			return err
		}
		if count != len(request.HookReleaseIDs) {
			return domain.ErrInvalid
		}
	}
	if request.Notification.Enabled && request.Notification.GroupRouteID > 0 {
		var ok bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_notification_group_route WHERE organization_id=$1 AND id=$2 AND status='ENABLED')`, tenant, request.Notification.GroupRouteID).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return domain.ErrInvalid
		}
	}
	if request.DepartmentID > 0 {
		var ok bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM sys_dept WHERE id=$1 AND "orgId"=$2 AND "isDeleted"=false AND status=0)`, request.DepartmentID, tenant).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return domain.ErrInvalid
		}
	}
	return nil
}

const projectSelectBase = `SELECT p.id,p.tenant_id,p.project_key,v.name,v.description,COALESCE(v.department_id,0),v.kind,p.status,v.id,COALESCE(v.repository_provider_type,''),COALESCE(v.repository_scheme,''),COALESCE(v.repository_host,''),COALESCE(v.repository_port,0),COALESCE(v.repository_path,''),COALESCE(v.repository_display_url,''),COALESCE(v.repository_web_url,''),COALESCE(encode(v.repository_credential_ciphertext,'base64'),''),COALESCE(encode(v.repository_credential_edek,'base64'),''),COALESCE(v.repository_credential_wrap_key_ref,''),COALESCE(v.repository_credential_version,0),COALESCE(v.repository_config_digest,''),COALESCE(p.current_source_state,''),COALESCE(v.repository_capabilities_json,'{}'::jsonb),COALESCE(v.repository_last_check_status,''),COALESCE(v.repository_last_check_reason_code,''),COALESCE(to_char(v.repository_last_checked_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),''),COALESCE(p.current_source_revision_id,0),COALESCE(v.default_ref_name,''),v.tech_stack_json,v.agent_guidance,v.completion_check_json,v.completion_check_digest,COALESCE(v.result_processor_release_id,0),v.result_processor_parameters_json,v.review_prompt,v.delivery_mode,v.delivery_authorization,COALESCE(v.delivery_authorized_by_principal_id,0),v.result_branch_pattern,v.notification_enabled,v.notification_events_json,COALESCE(v.notification_group_route_id,0),v.hook_release_ids,to_char(p.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),to_char(p.updated_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(p.archived_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_projects p JOIN workforce_project_versions v ON v.tenant_id=p.tenant_id AND v.project_id=p.id`

const projectSelect = projectSelectBase + ` AND v.id=p.current_version_id`
const historicalProjectSelect = projectSelectBase + ` AND v.id=$3`

type rowScanner interface{ Scan(...any) error }

func scan(row rowScanner) (domain.Project, error) {
	var p domain.Project
	var tech, events, caps []byte
	err := row.Scan(&p.ID, &p.TenantID, &p.ProjectKey, &p.Name, &p.Description, &p.DepartmentID, &p.Kind, &p.Status, &p.VersionID, &p.RepositoryProviderType, &p.RepositoryScheme, &p.RepositoryHost, &p.RepositoryPort, &p.RepositoryPath, &p.RepositoryDisplayURL, &p.RepositoryWebURL, &p.RepositoryCredentialCiphertext, &p.RepositoryCredentialEDEK, &p.RepositoryCredentialWrapKeyRef, &p.RepositoryCredentialVersion, &p.RepositoryConfigDigest, &p.RepositorySourceState, &caps, &p.RepositoryLastCheckStatus, &p.RepositoryLastCheckReasonCode, &p.RepositoryLastCheckedAt, &p.RepositorySourceRevisionID, &p.DefaultRefName, &tech, &p.AgentGuidance, &p.CompletionCheck, &p.CompletionCheckDigest, &p.ResultProcessorReleaseID, &p.ResultProcessorParameters, &p.ReviewPrompt, &p.DeliveryMode, &p.DeliveryAuthorization, &p.DeliveryAuthorizedByPrincipalID, &p.ResultBranchPattern, &p.NotificationEnabled, &events, &p.NotificationGroupRouteID, &p.HookReleaseIDs, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt)
	if err == nil {
		_ = json.Unmarshal(tech, &p.TechStack)
		_ = json.Unmarshal(events, &p.NotificationEvents)
		_ = json.Unmarshal(caps, &p.RepositoryCapabilities)
	}
	return p, err
}
func (s *Store) enrichProjects(ctx context.Context, tenant int64, items []domain.Project) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	index := map[int64]int{}
	departments, routes, hookReleases := []int64{}, []int64{}, []int64{}
	for i := range items {
		ids = append(ids, items[i].ID)
		index[items[i].ID] = i
		if items[i].DepartmentID > 0 {
			departments = append(departments, items[i].DepartmentID)
		}
		if items[i].NotificationGroupRouteID > 0 {
			routes = append(routes, items[i].NotificationGroupRouteID)
		}
		hookReleases = append(hookReleases, items[i].HookReleaseIDs...)
	}
	if err := queryNames(ctx, s.db, `SELECT id,name FROM sys_dept WHERE "orgId"=$1 AND id=ANY($2::bigint[]) AND "isDeleted"=false`, tenant, departments, func(id int64, name string) {
		for i := range items {
			if items[i].DepartmentID == id {
				items[i].DepartmentName = name
			}
		}
	}); err != nil {
		return err
	}
	if err := queryNames(ctx, s.db, `SELECT id,display_name FROM sys_notification_group_route WHERE organization_id=$1 AND id=ANY($2::bigint[])`, tenant, routes, func(id int64, name string) {
		for i := range items {
			if items[i].NotificationGroupRouteID == id {
				items[i].NotificationGroupRouteName = name
			}
		}
	}); err != nil {
		return err
	}
	if len(hookReleases) > 0 {
		rows, hookErr := s.db.Query(ctx, `SELECT r.id,r.package_id,p.name,r.version,r.manifest_json FROM workforce_hook_package_releases r JOIN workforce_hook_packages p ON p.id=r.package_id WHERE r.id=ANY($1::BIGINT[]) AND ((r.scope_type='SYSTEM' AND r.tenant_id IS NULL) OR (r.scope_type='TENANT' AND r.tenant_id=$2))`, hookReleases, tenant)
		if hookErr != nil {
			return hookErr
		}
		resolved := make(map[int64]domain.HookRelease, len(hookReleases))
		for rows.Next() {
			var view domain.HookRelease
			var manifest struct {
				Entrypoints []struct {
					Event       string `json:"event"`
					FailureMode string `json:"failureMode"`
				} `json:"entrypoints"`
			}
			var raw []byte
			if hookErr = rows.Scan(&view.ReleaseID, &view.HookID, &view.Name, &view.Version, &raw); hookErr != nil {
				rows.Close()
				return hookErr
			}
			if json.Unmarshal(raw, &manifest) != nil {
				rows.Close()
				return domain.ErrInvalid
			}
			for _, entry := range manifest.Entrypoints {
				view.Events = append(view.Events, entry.Event)
				if entry.FailureMode == "BLOCK" {
					view.FailureMode = "BLOCK"
				} else if view.FailureMode == "" {
					view.FailureMode = entry.FailureMode
				}
			}
			resolved[view.ReleaseID] = view
		}
		if hookErr = rows.Err(); hookErr != nil {
			rows.Close()
			return hookErr
		}
		rows.Close()
		for i := range items {
			items[i].Hooks = make([]domain.HookRelease, 0, len(items[i].HookReleaseIDs))
			for _, releaseID := range items[i].HookReleaseIDs {
				view, ok := resolved[releaseID]
				if !ok {
					return domain.ErrInvalid
				}
				items[i].Hooks = append(items[i].Hooks, view)
			}
		}
	}
	rows, err := s.db.Query(ctx, `SELECT project_id,COUNT(*),COUNT(*) FILTER(WHERE status IN('QUEUED','PREPARING','RUNNING','VALIDATING','PACKAGING','UPLOADING','RETRY_WAIT','REVIEWING','MANUAL_REVIEW','CANCELLING')),COALESCE(to_char(MAX(created_at) AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_tasks WHERE tenant_id=$1 AND project_id=ANY($2::bigint[]) GROUP BY project_id`, tenant, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var count, active int
		var latest string
		if err = rows.Scan(&id, &count, &active, &latest); err != nil {
			return err
		}
		if i, ok := index[id]; ok {
			items[i].TaskCount, items[i].ActiveTaskCount, items[i].LatestTaskAt = count, active, latest
		}
	}
	return rows.Err()
}
func queryNames(ctx context.Context, db *pgxpool.Pool, q string, tenant int64, ids []int64, apply func(int64, string)) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := db.Query(ctx, q, tenant, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			return err
		}
		apply(id, name)
	}
	return rows.Err()
}
func completion(request projectapp.UpsertRequest) (json.RawMessage, string) {
	if request.CompletionCheck == nil {
		body := json.RawMessage(`{"mode":"NONE"}`)
		return body, sha256Sum(body)
	}
	body, _ := json.Marshal(request.CompletionCheck)
	return body, sha256Sum(body)
}
func sha256Sum(body []byte) string {
	const chars = "0123456789abcdef"
	sum := sha256.Sum256(body)
	out := make([]byte, 64)
	for i, b := range sum {
		out[i*2] = chars[b>>4]
		out[i*2+1] = chars[b&15]
	}
	return string(out)
}
func marshalStringList(values []string) json.RawMessage {
	if values == nil {
		values = []string{}
	}
	body, _ := json.Marshal(values)
	return body
}
func repository(request projectapp.UpsertRequest) repositoryValues {
	if request.ResolvedRepository == nil {
		return repositoryValues{Capabilities: domain.RepositoryCapabilities{}}
	}
	v := request.ResolvedRepository
	return repositoryValues{ProviderType: v.ProviderType, Scheme: v.Scheme, Host: v.Host, Port: v.Port, Path: v.Path, DisplayURL: v.DisplayURL, WebURL: v.WebURL, CredentialCiphertext: v.CredentialCiphertext, CredentialEDEK: v.CredentialEDEK, CredentialWrapKeyRef: v.CredentialWrapKeyRef, CredentialVersion: v.CredentialVersion, SourceRevisionID: v.SourceRevisionID, ConfigDigest: v.ConfigDigest, SourceState: v.SourceState, Capabilities: v.Capabilities, LastCheckStatus: v.LastCheckStatus, LastCheckReasonCode: v.LastCheckReasonCode, DefaultRefName: v.DefaultRefName}
}

type repositoryValues struct {
	ProviderType, Scheme, Host, Path, DisplayURL, WebURL       string
	Port                                                       int
	CredentialCiphertext, CredentialEDEK, CredentialWrapKeyRef string
	CredentialVersion, SourceRevisionID                        int64
	ConfigDigest, SourceState                                  string
	Capabilities                                               domain.RepositoryCapabilities
	LastCheckStatus, LastCheckReasonCode, DefaultRefName       string
}

func (s *Store) GetVersion(ctx context.Context, tenant, project, version int64) (domain.Project, error) {
	p, err := scan(s.db.QueryRow(ctx, historicalProjectSelect+` WHERE p.tenant_id=$1 AND p.id=$2`, tenant, project, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, domain.ErrNotFound
	}
	// A historical version's prepared source is resolved by version, never from
	// the mutable current_source_revision_id projection.
	p.RepositorySourceState = domain.RepositorySourceReady
	return p, err
}
