package infrastructure

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProjectVersionWriteUsesMigratedColumnTypes(t *testing.T) {
	dsn := os.Getenv("PROJECT_REPOSITORY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set PROJECT_REPOSITORY_TEST_POSTGRES_DSN for isolated ProjectVersion writes")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("project_version_write_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Copy definitions, including VARCHAR and CHECK constraints, but no rows or
	// credentials. Every write below stays inside this disposable test schema.
	for _, table := range []string{"sys_user", "sys_dept", "sys_notification_group_route", "workforce_principals", "workforce_projects", "workforce_project_versions", "workforce_tasks"} {
		if _, err = db.Exec(ctx, "CREATE TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(ctx, `INSERT INTO sys_user(id,"userAccount","nickName") VALUES(8,'project-write-fixture','Project write fixture')`); err != nil {
		t.Fatal(err)
	}
	store := New(db, &postgresProjectIDs{next: 1000})
	general, err := store.Create(ctx, 7, 8, projectapp.UpsertRequest{
		ProjectKey: "general", Name: "General", Kind: "GENERAL", ResolvedProcessorReleaseID: 1,
		Delivery:       projectapp.DeliveryPolicy{Mode: "NONE", Authorization: "NONE"},
		IdempotencyKey: "general", CreateRequestDigest: strings.Repeat("a", 64),
	})
	if err != nil {
		t.Fatalf("GENERAL version insert against migrated types: %v", err)
	}
	if general.DepartmentID != 0 || general.RepositoryDisplayURL != "" {
		t.Fatal("optional department or GENERAL repository changed")
	}
	repo := &projectapp.ResolvedRepository{
		ProviderType: "GITLAB", Scheme: "https", Host: "git.example.test", Port: 443,
		Path: "group/project.git", DisplayURL: "https://git.example.test/group/project.git", WebURL: "https://git.example.test/group/project", DefaultRefName: "main",
		CredentialCiphertext: base64.StdEncoding.EncodeToString([]byte("fixture-cipher-v1")),
		CredentialEDEK:       base64.StdEncoding.EncodeToString([]byte("fixture-edek")), CredentialWrapKeyRef: "fixture-key", CredentialVersion: 1,
		ConfigDigest: strings.Repeat("b", 64), SourceState: "PREPARING", LastCheckStatus: "SUCCEEDED",
		Capabilities: domain.RepositoryCapabilities{Read: true, Push: true, DraftReview: true},
	}
	request := projectapp.UpsertRequest{
		ProjectKey: "codebase", Name: "Codebase", Kind: "CODEBASE", ResolvedProcessorReleaseID: 1, ResolvedRepository: repo,
		Delivery:       projectapp.DeliveryPolicy{Mode: "NEW_BRANCH_AND_DRAFT_REVIEW", Authorization: "AUTO_AFTER_GATES", BranchPattern: "codex/{task-id}"},
		IdempotencyKey: "codebase", CreateRequestDigest: strings.Repeat("c", 64),
	}
	created, err := store.Create(ctx, 7, 8, request)
	if err != nil {
		t.Fatalf("CODEBASE version insert with automatic delivery: %v", err)
	}
	request.ExpectedVersionID = created.VersionID
	request.Name = "Updated"
	repo.CredentialVersion = 2
	repo.CredentialCiphertext = base64.StdEncoding.EncodeToString([]byte("fixture-cipher-v2"))
	repo.ConfigDigest = strings.Repeat("d", 64)
	updated, err := store.Update(ctx, 7, 8, created.ID, request)
	if err != nil {
		t.Fatalf("credential binding update against migrated types: %v", err)
	}
	if updated.VersionID == created.VersionID || updated.RepositoryCredentialVersion != 2 || updated.DeliveryMode != request.Delivery.Mode || updated.DepartmentID != 0 {
		t.Fatal("new version did not preserve optional department and delivery configuration")
	}
	var oldName string
	var oldCredentialVersion int
	var departmentNull bool
	if err = db.QueryRow(ctx, `SELECT name,repository_credential_version,department_id IS NULL FROM workforce_project_versions WHERE id=$1`, created.VersionID).Scan(&oldName, &oldCredentialVersion, &departmentNull); err != nil {
		t.Fatal(err)
	}
	if oldName != "Codebase" || oldCredentialVersion != 1 || !departmentNull {
		t.Fatal("update modified the historical ProjectVersion")
	}
	if _, err = store.Update(ctx, 7, 8, created.ID, request); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("stale version update must conflict: %v", err)
	}
	if _, err = store.Update(ctx, 9, 8, created.ID, request); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other tenant must not update the project: %v", err)
	}
}
