package credential

import (
	"fmt"

	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	credentialinfra "github.com/CaixyPromise/seven-agent/force/internal/app/credential/infrastructure"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	facade facade.UserCredentialFacade
}

func Install(deps bootstrapruntime.ModuleDeps) (*Module, facade.UserCredentialFacade, error) {
	if deps.Infra.Datasource == nil {
		return nil, nil, fmt.Errorf("credential module requires datasource provider")
	}
	if deps.IDGen == nil {
		return nil, nil, fmt.Errorf("credential module requires id generator")
	}
	if deps.Security.Recovery == nil {
		return nil, nil, fmt.Errorf("credential module requires recovery code service")
	}
	if deps.Security.Envelope == nil || deps.Security.CredentialCodec == nil {
		return nil, nil, fmt.Errorf("credential module requires envelope service and credential codec")
	}

	repository, err := credentialinfra.NewUserCredentialRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, nil, err
	}
	payloadCodec := credentialinfra.NewCredentialPayloadCodec()
	domainService := domain.NewService(repository, deps.IDGen, deps.Security.Recovery, payloadCodec)
	appService := application.NewService(deps.Infra.Transactor, domainService, deps.Security.Envelope, deps.Security.CredentialCodec)
	module := &Module{
		facade: appService,
	}
	return module, module.facade, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{
		Name:   "credential",
		Prefix: "/credential",
	}
}

func (m *Module) Mount(engine route.IRouter) {}

func (m *Module) Facade() facade.UserCredentialFacade {
	return m.facade
}
