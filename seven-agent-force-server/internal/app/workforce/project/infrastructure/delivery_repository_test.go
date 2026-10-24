package infrastructure

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestDeliveryRepositoryBindingTenantAndImmutableVersion(t *testing.T) {
	dsn := os.Getenv("DELIVERY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set DELIVERY_TEST_POSTGRES_DSN for isolated ProjectVersion binding verification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("delivery_binding_%d", time.Now().UnixNano())
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
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Definitions only: no existing rows, identities or encrypted credentials.
	for _, table := range []string{"workforce_projects", "workforce_project_versions"} {
		if _, err = db.Exec(ctx, "CREATE TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_projects(id,tenant_id,project_key,current_version_id,created_by_principal_id,updated_by_principal_id,create_idempotency_key,create_request_digest) VALUES(24,1,'binding-fixture',101,7,7,'fixture-key',repeat('a',64));
INSERT INTO workforce_project_versions(id,tenant_id,project_id,name,kind,created_by_principal_id,repository_provider_type,repository_scheme,repository_host,repository_port,repository_path,repository_web_url,repository_config_digest,repository_credential_version,repository_credential_ciphertext,repository_credential_edek,repository_credential_wrap_key_ref,repository_capabilities_json,repository_last_check_status,completion_check_digest,result_processor_release_id,repository_display_url,default_ref_name)
VALUES(101,1,24,'binding-fixture','CODEBASE',7,'GITLAB','https','127.0.0.1',3444,'team/fixture.git','https://127.0.0.1:3444/team/fixture',repeat('c',64),2,'fixture-ciphertext'::bytea,'fixture-edek'::bytea,'fixture-key','{"read":true,"push":true,"draftReview":true}','SUCCEEDED',repeat('a',64),1,'https://127.0.0.1:3444/team/fixture.git','main')`); err != nil {
		t.Fatal(err)
	}
	s := New(db, &postgresProjectIDs{})
	b, err := s.DeliveryRepositoryBinding(ctx, 1, 24, 101)
	if err != nil || !b.Current || b.CredentialVersion != 2 || !b.Push || !b.DraftReview {
		t.Fatalf("verified version not resolved: %v", err)
	}
	for _, scope := range [][3]int64{{2, 24, 101}, {1, 25, 101}, {1, 24, 102}} {
		if _, err := s.DeliveryRepositoryBinding(ctx, scope[0], scope[1], scope[2]); err == nil {
			t.Fatal("wrong tenant/project/version accepted")
		}
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_projects SET current_version_id=102 WHERE id=24`); err != nil {
		t.Fatal(err)
	}
	b, err = s.DeliveryRepositoryBinding(ctx, 1, 24, 101)
	if err != nil || b.Current || b.CredentialVersion != 2 {
		t.Fatalf("explicit historical reference advanced: %v", err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_projects SET status='ARCHIVED',archived_at=CURRENT_TIMESTAMP WHERE id=24`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeliveryRepositoryBinding(ctx, 1, 24, 101); err == nil {
		t.Fatal("archived project accepted")
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_projects SET status='ACTIVE',archived_at=NULL WHERE id=24; UPDATE workforce_project_versions SET repository_last_check_status='FAILED' WHERE id=101`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeliveryRepositoryBinding(ctx, 1, 24, 101); err == nil {
		t.Fatal("unverified repository accepted")
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_project_versions SET repository_last_check_status='SUCCEEDED',repository_capabilities_json='{"read":false,"push":true,"draftReview":true}' WHERE id=101`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeliveryRepositoryBinding(ctx, 1, 24, 101); err == nil {
		t.Fatal("credential without verified read accepted")
	}
}
