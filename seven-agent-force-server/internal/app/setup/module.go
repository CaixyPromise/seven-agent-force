package setup

import (
	"fmt"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	setupapp "github.com/CaixyPromise/seven-agent/force/internal/app/setup/application"
	setupdomain "github.com/CaixyPromise/seven-agent/force/internal/app/setup/domain"
	setuphandler "github.com/CaixyPromise/seven-agent/force/internal/app/setup/handler"
	setupinfra "github.com/CaixyPromise/seven-agent/force/internal/app/setup/infrastructure"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/cloudwego/hertz/pkg/route"
)

type Dependencies struct {
	Users        userfacade.ProvisioningFacade
	Relations    userfacade.UserRelationFacade
	Roles        authorizationfacade.RoleFacade
	Permissions  authorizationfacade.PermissionFacade
	SsoBootstrap ssofacade.BootstrapSessionFacade
}

type Module struct {
	handler *setuphandler.Handler
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, error) {
	if deps.Infra.Datasource == nil {
		return nil, fmt.Errorf("setup module requires datasource provider")
	}
	if deps.Infra.CacheMgr == nil {
		return nil, fmt.Errorf("setup module requires cache manager")
	}
	if deps.Config.Setup.Enabled && (deps.Infra.Transactor == nil || !deps.Infra.Transactor.Enabled()) {
		return nil, fmt.Errorf("setup module requires datasource transactor")
	}
	if refs.Users == nil || refs.Relations == nil || refs.Roles == nil || refs.Permissions == nil || refs.SsoBootstrap == nil {
		return nil, fmt.Errorf("setup module requires user, authorization, and sso facades")
	}
	startTime := time.Now().UTC()
	startupEpoch := startTime.UnixMilli()
	tokenService, err := setupdomain.NewTokenService(deps.Config.Setup.TokenSecret, deps.Config.Setup.TokenTTLSeconds, startupEpoch)
	if err != nil {
		return nil, err
	}
	service := setupapp.NewService(
		setupapp.Settings{
			Setup:              deps.Config.Setup,
			LoginEnabled:       deps.Config.Login.Enabled,
			SSOFrontendPrimary: deps.Config.SSO.FrontendPrimaryEnabled,
			AppVersion:         "dev",
			AppCommit:          "dev",
			StartTime:          startTime,
		},
		tokenService,
		setupinfra.NewStateStore(deps.Infra.CacheMgr),
		deps.Infra.Transactor,
		refs.Users,
		refs.Relations,
		refs.Roles,
		refs.Permissions,
		refs.SsoBootstrap,
	)
	return &Module{handler: setuphandler.NewHandler(service, deps.Config.Setup)}, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "setup", Prefix: "/setup"}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m == nil || m.handler == nil {
		return
	}
	engine.GET("/setup/status", m.handler.Status)
	engine.POST("/setup/owner", m.handler.CreateOwner)
}
