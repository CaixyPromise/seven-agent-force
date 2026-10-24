package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
)

type postgresProjectIDs struct{ next int64 }

func (g *postgresProjectIDs) NextID() int64 {
	g.next++
	return g.next
}

func TestProjectStorePersistsDirectRepositoryShape(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PROJECT_REPOSITORY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set PROJECT_REPOSITORY_TEST_POSTGRES_DSN to the migrated isolated PostgreSQL database")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = pool.Ping(ctx); err != nil {
		t.Fatal(err)
	}

	baseID := time.Now().UTC().UnixNano()
	tenantID, deptID, userID := baseID, baseID+1, baseID+2
	ids := &postgresProjectIDs{next: baseID + 1_000}
	store := New(pool, ids)
	if err = store.ValidateProjectReferences(ctx, tenantID, projectapp.UpsertRequest{}); err != nil {
		t.Fatalf("optional department must not block Project creation: %v", err)
	}
	cleanup := func(cleanupCtx context.Context) {
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM workforce_workspace_source_revisions WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM workforce_workspace_sources WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM workforce_projects WHERE tenant_id=$1`, tenantID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM workforce_principals WHERE user_id=$1`, userID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM sys_user WHERE id=$1`, userID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM sys_dept WHERE id=$1`, deptID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM sys_org WHERE id=$1`, tenantID)
	}
	cleanup(ctx)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		cleanup(cleanupCtx)
	})

	identitySuffix := fmt.Sprintf("%d", baseID)
	if _, err = pool.Exec(ctx, `INSERT INTO sys_org (id,code,name) VALUES ($1,$2,'Project Repository Test')`, tenantID, "project-repository-test-"+identitySuffix); err != nil {
		t.Fatalf("seed Framework identities: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO sys_dept (id,name,code,"orgId") VALUES ($1,'Project Repository Test',$2,$3)`, deptID, "project-repository-test-"+identitySuffix, tenantID); err != nil {
		t.Fatalf("seed Framework department: %v", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO sys_user (id,"userAccount","nickName") VALUES ($1,$2,'Project Repository Test')`, userID, "prj-"+identitySuffix); err != nil {
		t.Fatalf("seed Framework user: %v", err)
	}
	general, err := store.Create(ctx, tenantID, userID, projectapp.UpsertRequest{
		ProjectKey: "general-store-test", Name: "General Store Test", DepartmentID: deptID, Kind: "GENERAL",
		ResolvedProcessorReleaseID: 1, Delivery: projectapp.DeliveryPolicy{Mode: "NONE", Authorization: "NONE"},
		IdempotencyKey: "general-store-test", CreateRequestDigest: strings.Repeat("1", 64),
	})
	if err != nil {
		t.Fatalf("create GENERAL Project: %v", err)
	}
	var generalRepositoryFields int
	if err = pool.QueryRow(ctx, `SELECT num_nonnulls(
		repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_display_url,repository_web_url,
		repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_credential_version,
		repository_config_digest,repository_capabilities_json,repository_last_check_status,
		repository_last_check_reason_code,repository_last_checked_at,default_ref_name)
		FROM workforce_project_versions WHERE id=$1`, general.VersionID).Scan(&generalRepositoryFields); err != nil {
		t.Fatal(err)
	}
	if generalRepositoryFields != 0 {
		t.Fatalf("GENERAL Project persisted %d repository fields; want all NULL", generalRepositoryFields)
	}
	replayed, err := store.Create(ctx, tenantID, userID, projectapp.UpsertRequest{
		ProjectKey: "ignored-on-replay", Name: "General Store Test", DepartmentID: deptID, Kind: "GENERAL",
		ResolvedProcessorReleaseID: 1, Delivery: projectapp.DeliveryPolicy{Mode: "NONE", Authorization: "NONE"},
		IdempotencyKey: "general-store-test", CreateRequestDigest: strings.Repeat("1", 64),
	})
	if err != nil || replayed.ID != general.ID {
		t.Fatalf("idempotent Project create did not replay: id=%d err=%v", replayed.ID, err)
	}
	_, err = store.Create(ctx, tenantID, userID, projectapp.UpsertRequest{
		ProjectKey: "conflicting-replay", Name: "Changed", DepartmentID: deptID, Kind: "GENERAL",
		ResolvedProcessorReleaseID: 1, Delivery: projectapp.DeliveryPolicy{Mode: "NONE", Authorization: "NONE"},
		IdempotencyKey: "general-store-test", CreateRequestDigest: strings.Repeat("9", 64),
	})
	if !errors.Is(err, domain.ErrIdempotencyConflict) {
		t.Fatalf("Project idempotency digest conflict returned %v", err)
	}

	resolved := &projectapp.ResolvedRepository{
		ProviderType: "GITLAB", Scheme: "https", Host: "gitlab.example.test", Port: 443,
		Path: "group/repository.git", DisplayURL: "https://gitlab.example.test/group/repository.git",
		WebURL: "https://gitlab.example.test/group/repository", DefaultRefName: "main",
		CredentialCiphertext: "ciphertext", CredentialEDEK: "edek", CredentialWrapKeyRef: "test-wrap-key",
		CredentialVersion: 1, ConfigDigest: strings.Repeat("a", 64), SourceState: "PREPARING",
		Capabilities: domain.RepositoryCapabilities{Read: true, Push: true, DraftReview: true}, LastCheckStatus: "VERIFIED",
	}
	codebaseRequest := projectapp.UpsertRequest{
		ProjectKey: "codebase-store-test", Name: "Codebase Store Test", DepartmentID: deptID, Kind: "CODEBASE",
		ResolvedProcessorReleaseID: 1, Delivery: projectapp.DeliveryPolicy{Mode: "NONE", Authorization: "NONE"},
		ResolvedRepository: resolved,
		IdempotencyKey:     "codebase-store-test", CreateRequestDigest: strings.Repeat("2", 64),
	}
	codebase, err := store.Create(ctx, tenantID, userID, codebaseRequest)
	if err != nil {
		t.Fatalf("create CODEBASE Project: %v", err)
	}
	if codebase.RepositoryDisplayURL != resolved.DisplayURL || codebase.RepositoryCredentialCiphertext != "ciphertext" || codebase.RepositorySourceState != "PREPARING" {
		t.Fatalf("CODEBASE repository did not round-trip: %#v", codebase)
	}

	codebaseRequest.ExpectedVersionID = codebase.VersionID
	codebaseRequest.Name = "Updated Codebase Store Test"
	codebaseRequest.ResolvedRepository.SourceState = "READY"
	updated, err := store.Update(ctx, tenantID, userID, codebase.ID, codebaseRequest)
	if err != nil {
		t.Fatalf("update CODEBASE Project: %v", err)
	}
	if updated.VersionID == codebase.VersionID || updated.RepositorySourceState != "READY" || updated.Name != codebaseRequest.Name {
		t.Fatalf("unexpected updated CODEBASE Project: %#v", updated)
	}
	var oldName string
	if err = pool.QueryRow(ctx, `SELECT name FROM workforce_project_versions WHERE id=$1`, codebase.VersionID).Scan(&oldName); err != nil {
		t.Fatal(err)
	}
	if oldName != "Codebase Store Test" {
		t.Fatalf("old ProjectVersion changed to %q", oldName)
	}
	if _, err = pool.Exec(ctx, `UPDATE workforce_project_versions SET name='forged' WHERE id=$1`, codebase.VersionID); err == nil || !strings.Contains(err.Error(), "PROJECT_VERSION_IMMUTABLE") {
		t.Fatalf("ProjectVersion update was not rejected: %v", err)
	}
	if _, err = pool.Exec(ctx, `DELETE FROM workforce_project_versions WHERE id=$1`, codebase.VersionID); err == nil || !strings.Contains(err.Error(), "PROJECT_VERSION_IMMUTABLE") {
		t.Fatalf("ProjectVersion deletion was not rejected: %v", err)
	}
	var creatorPrincipalID int64
	if err = pool.QueryRow(ctx, `SELECT created_by_principal_id FROM workforce_projects WHERE id=$1`, codebase.ID).Scan(&creatorPrincipalID); err != nil {
		t.Fatal(err)
	}
	sourceID, revisionID := ids.NextID(), ids.NextID()
	if _, err = pool.Exec(ctx, `INSERT INTO workforce_workspace_sources (id,tenant_id,kind,project_id,project_version_id,repository_config_digest,display_name,created_by_principal_id) VALUES ($1,$2,'GIT_REPOSITORY',$3,$4,$5,'repository',$6)`, sourceID, tenantID, codebase.ID, updated.VersionID, resolved.ConfigDigest, creatorPrincipalID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO workforce_workspace_source_revisions (id,tenant_id,workspace_source_id,version,kind,project_id,project_version_id,repository_config_digest,status) VALUES ($1,$2,$3,1,'GIT_REPOSITORY',$4,$5,$6,'AVAILABLE')`, revisionID, tenantID, sourceID, codebase.ID, updated.VersionID, resolved.ConfigDigest); err != nil {
		t.Fatal(err)
	}
	if availableID, lookupErr := store.LatestAvailableSource(ctx, tenantID, codebase.ID, updated.VersionID); lookupErr != nil || availableID != revisionID {
		t.Fatalf("metadata-only Project update did not reuse prepared source: id=%d err=%v", availableID, lookupErr)
	}
}
