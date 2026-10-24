package workspaceinput

import (
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	inputapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
	inputcontroller "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/controller"
	inputfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/facade"
	inputinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/infrastructure"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	Service *inputapp.Service
	Handler *inputcontroller.Handler
	Facade  inputfacade.Resolver
}

func New(db *pgxpool.Pool, projects inputapp.ProjectRepositoryReader, artifacts *artifactstore.Store, idGen inputinfra.IDGenerator) *Module {
	service := inputapp.New(inputinfra.New(db, idGen), projects, inputinfra.ArtifactAdapter{Store: artifacts})
	return &Module{Service: service, Handler: inputcontroller.New(service), Facade: inputfacade.Adapter{Service: service}}
}
func (m *Module) BindFiles(files filefacade.WorkforceInputAssetFacade) {
	if m != nil && m.Service != nil {
		m.Service.BindFiles(files)
	}
}
func (m *Module) BindCredentials(credentials inputapp.CredentialResolver) {
	if m != nil && m.Service != nil {
		m.Service.BindCredentials(credentials)
	}
}
