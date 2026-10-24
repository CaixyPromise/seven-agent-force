package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
)

// Embedding an uninitialized Repository makes any unexpected call fail. The
// requirements compiler may only load the Project, even before source readiness.
type requirementsOnlyRepository struct {
	Repository
	project             domain.Project
	err                 error
	tenantID, projectID int64
}

func (r *requirementsOnlyRepository) Get(_ context.Context, tenantID, projectID int64) (domain.Project, error) {
	r.tenantID, r.projectID = tenantID, projectID
	return r.project, r.err
}

func TestCompileExecutionRequirementsReadsOnlyProjectAndReusesCompiler(t *testing.T) {
	for _, kind := range []string{projectfacade.KindGeneral, projectfacade.KindCodebase} {
		for _, delivery := range []string{"NONE", "NEW_BRANCH", "NEW_BRANCH_AND_DRAFT_REVIEW"} {
			t.Run(kind+"/"+delivery, func(t *testing.T) {
				repo := &requirementsOnlyRepository{project: domain.Project{ID: 7, VersionID: 8, Status: projectfacade.StatusActive, Kind: kind, DeliveryMode: delivery, RepositorySourceState: domain.RepositorySourcePreparing}}
				compiled, err := New(repo).CompileExecutionRequirements(context.Background(), 100, 7)
				if err != nil {
					t.Fatal(err)
				}
				want := compileRequirements(repo.project, projectfacade.FrozenGitPlan{DeliveryMode: delivery})
				if compiled.ProjectVersionID != 8 || !reflect.DeepEqual(compiled.Requirements, want) || repo.tenantID != 100 || repo.projectID != 7 {
					t.Fatalf("compiled=%#v want=%#v", compiled, want)
				}
			})
		}
	}
}

func TestCompileExecutionRequirementsRejectsArchivedAndPropagatesReadFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		repo *requirementsOnlyRepository
		want error
	}{
		{"archived", &requirementsOnlyRepository{project: domain.Project{Status: projectfacade.StatusArchived}}, domain.ErrArchived},
		{"missing", &requirementsOnlyRepository{err: domain.ErrNotFound}, domain.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			compiled, err := New(tc.repo).CompileExecutionRequirements(context.Background(), 100, 7)
			if !errors.Is(err, tc.want) || compiled.ProjectVersionID != 0 {
				t.Fatalf("compiled=%#v err=%v want=%v", compiled, err, tc.want)
			}
		})
	}
}
