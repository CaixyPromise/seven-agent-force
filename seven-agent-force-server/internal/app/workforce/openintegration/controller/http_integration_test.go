package controller

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	openapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/application"
	openinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/infrastructure"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route/param"
	"github.com/jackc/pgx/v5/pgxpool"
)

type contractIDGen struct{ next int64 }

func (g *contractIDGen) NextID() int64 { g.next++; return g.next }

// Authenticate with a real isolated Application and query its stored Task. This
// catches permissive path parsing that otherwise aliases noncanonical IDs to a
// valid Task, while also exercising the Application/tenant ownership boundary.
func TestAuthenticatedTaskQueryUsesCanonicalIDsAndApplicationOwnership(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	seed := time.Now().UnixNano()
	service := openapp.NewService(openinfra.NewStore(db, &contractIDGen{next: seed}), nil, nil, nil, nil, "")
	owner, err := service.CreateApplication(ctx, seed, 101, openapp.CreateApplicationRequest{Name: "query contract owner"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateApplication(ctx, seed, 101, openapp.CreateApplicationRequest{Name: "query contract other"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := service.CreateApplication(ctx, seed+1, 101, openapp.CreateApplicationRequest{Name: "query contract foreign tenant"})
	if err != nil {
		t.Fatal(err)
	}
	taskID := seed + 100
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE id=$1 AND integration_application_id=$2`, taskID, owner.Application.ID)
		for _, id := range []int64{owner.Application.ID, other.Application.ID, foreign.Application.ID} {
			_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, id)
			_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, id)
		}
	})
	_, err = db.Exec(ctx, `INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest,current_task_turn_id,integration_application_id,external_correlation_id)
 VALUES($1,$2,101,'query-contract',$3,'','','fixture','QUEUED',$1,$1,$1,'{}',$3,$1,$4,'query-contract')`, taskID, seed, strings.Repeat("a", 64), owner.Application.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := New(service)
	raw := strconv.FormatInt(taskID, 10)
	cases := []struct {
		name, id, key string
		status        int
	}{
		{"canonical", raw, owner.APIKey, 200},
		{"leading zero", "0" + raw, owner.APIKey, 404},
		{"leading plus", "+" + raw, owner.APIKey, 404},
		{"whitespace", " " + raw + " ", owner.APIKey, 404},
		{"overflow", "9223372036854775808", owner.APIKey, 404},
		{"other application", raw, other.APIKey, 404},
		{"other tenant", raw, foreign.APIKey, 404},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &app.RequestContext{}
			c.Params = param.Params{{Key: "id", Value: tc.id}}
			c.Request.Header.Set("Authorization", "Bearer "+tc.key)
			h.getOpenTask(ctx, c)
			if c.Response.StatusCode() != tc.status {
				t.Fatalf("query status=%d want=%d", c.Response.StatusCode(), tc.status)
			}
			if tc.status == 404 && !strings.Contains(string(c.Response.Body()), "TASK_NOT_FOUND") {
				t.Fatal("query did not preserve the safe not-found classification")
			}
			if tc.status == 200 && !strings.Contains(string(c.Response.Body()), `"taskId":"`+raw+`"`) {
				t.Fatal("query lost the decimal-string task identity")
			}
		})
	}
}
