package application

import (
	"context"
	"encoding/json"
	"errors"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	projectdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
	"strings"
	"testing"
)

// Unexpected source preparation or mutation fails through the nil embedding.
type selectionProjectRepository struct {
	projectapp.Repository
	project  projectdomain.Project
	tenantID int64
}

func (r selectionProjectRepository) Get(_ context.Context, tenantID, projectID int64) (projectdomain.Project, error) {
	if tenantID != r.tenantID || projectID != r.project.ID {
		return projectdomain.Project{}, projectdomain.ErrNotFound
	}
	return r.project, nil
}

type selectionModels struct {
	calls    int
	tenantID int64
	items    []providerfacade.ModelOption
}

func (m *selectionModels) ListTaskModelOptions(_ context.Context, tenantID int64) ([]providerfacade.ModelOption, error) {
	m.calls++
	m.tenantID = tenantID
	return m.items, nil
}

type selectionRuntimes struct {
	routingfacade.RuntimeCatalog
	calls                             int
	tenantID, projectID, credentialID int64
	err                               error
}

func (r *selectionRuntimes) ListTaskOptions(_ context.Context, tenantID, projectID, credentialID int64) (routingfacade.RuntimeOptions, error) {
	r.calls++
	r.tenantID, r.projectID, r.credentialID = tenantID, projectID, credentialID
	return routingfacade.RuntimeOptions{ProjectVersionID: 88, Items: []routingfacade.RuntimeOption{{RuntimeReleaseID: 99, Name: "runtime", Recommended: true, ImageRef: "private-image", ImageDigest: "private-digest", AvailableSlots: 4, Workers: []routingfacade.RuntimeWorkerOption{{}}}}}, r.err
}
func selectionService(status string) (*Service, *selectionModels, *selectionRuntimes) {
	models := &selectionModels{items: []providerfacade.ModelOption{{CredentialID: 9007199254740993, Model: "configured", Protocol: "OPENAI", Label: "configured"}}}
	runtimes := &selectionRuntimes{}
	service := NewService(nil, nil, nil, nil, nil, "")
	service.SetSelectionCatalogs(models, projectapp.New(selectionProjectRepository{tenantID: 10, project: projectdomain.Project{ID: 77, VersionID: 88, Status: status, Kind: projectfacade.KindGeneral}}), runtimes)
	return service, models, runtimes
}
func TestSelectionUsesRealProjectCompilerBeforeCatalogReads(t *testing.T) {
	identity := domain.Identity{ApplicationID: 1, TenantID: 10, CreatedByUserID: 20}
	for _, tc := range []struct {
		name, status        string
		tenantID, projectID int64
		want                error
	}{
		{"other tenant", projectfacade.StatusActive, 11, 77, projectdomain.ErrNotFound},
		{"missing project", projectfacade.StatusActive, 10, 78, projectdomain.ErrNotFound},
		{"archived project", projectfacade.StatusArchived, 10, 77, projectdomain.ErrArchived},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, models, runtimes := selectionService(tc.status)
			caller := identity
			caller.TenantID = tc.tenantID
			_, err := service.ListModelOptions(context.Background(), caller, tc.projectID)
			if !errors.Is(err, tc.want) {
				t.Fatalf("model error=%v", err)
			}
			_, err = service.ListRuntimeOptions(context.Background(), caller, tc.projectID, 9007199254740993)
			if !errors.Is(err, tc.want) {
				t.Fatalf("runtime error=%v", err)
			}
			if models.calls != 0 || runtimes.calls != 0 {
				t.Fatal("selection data queried before project authorization")
			}
		})
	}
	service, models, runtimes := selectionService(projectfacade.StatusActive)
	modelView, err := service.ListModelOptions(context.Background(), identity, 77)
	if err != nil {
		t.Fatal(err)
	}
	runtimeView, err := service.ListRuntimeOptions(context.Background(), identity, 77, 9007199254740993)
	if err != nil {
		t.Fatal(err)
	}
	if models.tenantID != 10 || runtimes.tenantID != 10 || runtimes.projectID != 77 || runtimes.credentialID != 9007199254740993 {
		t.Fatal("query scope lost")
	}
	raw, _ := json.Marshal([]any{modelView, runtimeView})
	for _, private := range []string{"private-image", "private-digest", "workers", "availableSlots", "adapter", "secret"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private data leaked: %s", raw)
		}
	}
	if len(runtimeView.Items) != 1 || runtimeView.Items[0].RuntimeReleaseID != 99 || !runtimeView.Items[0].Recommended || len(modelView.Items) != 1 || modelView.Items[0].CredentialID != 9007199254740993 {
		t.Fatalf("safe selection fields lost: %s", raw)
	}
	capability := service.GetOpenCapabilities(context.Background(), identity)
	if !capability.TaskSubmission.SupportsSelectionDiscovery {
		t.Fatal("discovery capability missing")
	}
}
func TestSelectionRejectsInvalidIdentityAndCredentialAndPreservesUnavailable(t *testing.T) {
	service, models, runtimes := selectionService(projectfacade.StatusActive)
	identity := domain.Identity{ApplicationID: 1, TenantID: 10, CreatedByUserID: 20}
	invalid := identity
	invalid.ApplicationID = 0
	if _, err := service.ListModelOptions(context.Background(), invalid, 77); !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("identity error=%v", err)
	}
	if _, err := service.ListRuntimeOptions(context.Background(), identity, 77, 0); !errors.Is(err, domain.ErrInvalidRequest) {
		t.Fatalf("credential error=%v", err)
	}
	if models.calls != 0 || runtimes.calls != 0 {
		t.Fatal("invalid input reached catalog")
	}
	runtimes.err = routingfacade.ErrRuntimeSelectionUnavailable
	_, err := service.ListRuntimeOptions(context.Background(), identity, 77, 9007199254740993)
	status, code, _ := StableError(err)
	if status != 409 || code != "TASK_RUNTIME_SELECTION_UNAVAILABLE" {
		t.Fatalf("unavailable mapping=%d/%s/%v", status, code, err)
	}
}
