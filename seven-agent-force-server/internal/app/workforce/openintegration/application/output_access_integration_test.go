package application

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	resultfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
)

type outputAccessIntegrationStub struct {
	listCalls int
	openCalls int
	openErr   error
}

func (s *outputAccessIntegrationStub) ListAvailableOutputs(context.Context, int64, int64) ([]resultfacade.AvailableOutput, error) {
	s.listCalls++
	return nil, nil
}

func (s *outputAccessIntegrationStub) OpenAvailableOutput(context.Context, int64, int64, int64) (resultfacade.OutputDownload, error) {
	s.openCalls++
	return resultfacade.OutputDownload{}, s.openErr
}

func TestOpenOutputOwnershipFailsClosedBeforeResultAccess(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	tenantID := int64(8_600_000_000_000_010_001)
	ownerApplicationID := int64(8_600_000_000_000_010_002)
	otherApplicationID := int64(8_600_000_000_000_010_003)
	taskID := int64(8_600_000_000_000_010_004)
	turnID := int64(8_600_000_000_000_010_005)
	for _, application := range []struct {
		id      int64
		locator string
	}{
		{ownerApplicationID, "credential-output-owner"},
		{otherApplicationID, "credential-output-other"},
	} {
		if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_applications
(id,tenant_id,name,status,webhook_url,created_by)
VALUES ($1,$2,'output isolation','ENABLED','https://example.com/hook',$3)`, application.id, tenantID, int64(101)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_api_credentials
(id,tenant_id,integration_application_id,credential_locator,credential_digest,credential_hint,version,status,created_by)
VALUES ($1,$2,$3,$4,'digest','hint',1,'CURRENT',$5)`, application.id+100, tenantID, application.id, application.locator, int64(101)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_signing_credentials
(id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,created_by)
VALUES ($1,$2,$3,'ciphertext','edek','key','hint',1,'CURRENT',$4)`, application.id+200, tenantID, application.id, int64(101)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,idempotency_key,request_hash,created_by,repository_url,repository_ref,task_instruction,status,submission_channel,integration_application_id,scenario_key,scenario_version,scenario_digest,external_correlation_id,current_task_turn_id,result_processor_release_id,project_id,project_version_id,review_enabled,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','test','SUCCEEDED','OPEN_REST',$6,'project-task',1,$7,'output-isolation',$8,1,$1,$1,TRUE,'{}'::jsonb,$4)`, taskID, tenantID, "idem-output-isolation", strings.Repeat("a", 64), int64(101), ownerApplicationID, strings.Repeat("b", 64), turnID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE id=$1`, taskID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id IN ($1,$2)`, ownerApplicationID, otherApplicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id IN ($1,$2)`, ownerApplicationID, otherApplicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id IN ($1,$2)`, ownerApplicationID, otherApplicationID)
	}()

	outputs := &outputAccessIntegrationStub{openErr: resultfacade.ErrOutputUnavailable}
	service := NewService(openinfra.NewStore(db, nil), nil, outputs, nil, nil, "")
	owner := domain.Identity{TenantID: tenantID, ApplicationID: ownerApplicationID}
	other := domain.Identity{TenantID: tenantID, ApplicationID: otherApplicationID}

	view, err := service.ListOpenTaskOutputs(ctx, owner, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Outputs == nil || len(view.Outputs) != 0 || outputs.listCalls != 1 {
		t.Fatalf("owned empty outputs = %#v, calls=%d", view.Outputs, outputs.listCalls)
	}
	if _, err := service.ListOpenTaskOutputs(ctx, other, taskID); !errors.Is(err, resultfacade.ErrOutputNotFound) {
		t.Fatalf("cross-application list error=%v", err)
	}
	if outputs.listCalls != 1 {
		t.Fatalf("cross-application list reached result access: calls=%d", outputs.listCalls)
	}
	if _, err := service.OpenTaskOutput(ctx, other, taskID, 777); !errors.Is(err, resultfacade.ErrOutputNotFound) {
		t.Fatalf("cross-application download error=%v", err)
	}
	if outputs.openCalls != 0 {
		t.Fatalf("cross-application download reached object access: calls=%d", outputs.openCalls)
	}
	if _, err := service.OpenTaskOutput(ctx, owner, taskID, 777); !errors.Is(err, resultfacade.ErrOutputUnavailable) {
		t.Fatalf("owned object failure error=%v", err)
	}
}
