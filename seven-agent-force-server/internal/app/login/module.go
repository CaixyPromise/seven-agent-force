package login

import (
	"fmt"
	"time"

	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/login/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/login/domain"
	loginfacade "github.com/CaixyPromise/seven-agent/force/internal/app/login/facade"
	loginhandler "github.com/CaixyPromise/seven-agent/force/internal/app/login/handler"
	logininfra "github.com/CaixyPromise/seven-agent/force/internal/app/login/infrastructure"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	facade  loginfacade.PasswordFlowFacade
	handler *loginhandler.Handler
	oplog   adminfacade.OperationLogger
}

type Dependencies struct {
	UserCredentials          credentialfacade.UserCredentialFacade
	Subjects                 userfacade.SubjectFacade
	ChallengeInternal        challengefacade.ChallengeInternalFacade
	ChallengeClient          challengefacade.ChallengeClientFacade
	ProofVerifier            challengefacade.ProofTokenVerifier
	LoginFailures            adminfacade.LoginFailureFacade
	AuthorizationSessions    ssofacade.AuthorizationSessionFacade
	AuthenticationCompletion ssofacade.AuthenticationCompletionFacade
	BootstrapSession         ssofacade.BootstrapSessionFacade
	Platform                 platformfacade.PublicFacade
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, loginfacade.PasswordFlowFacade, error) {
	if !deps.Config.Login.Enabled {
		return nil, nil, nil
	}
	if deps.Security.Password == nil {
		return nil, nil, fmt.Errorf("login module requires password service")
	}
	if deps.Infra.CacheMgr == nil {
		return nil, nil, fmt.Errorf("login module requires cache manager")
	}
	if refs.UserCredentials == nil || refs.Subjects == nil || refs.ChallengeInternal == nil || refs.ChallengeClient == nil || refs.ProofVerifier == nil {
		return nil, nil, fmt.Errorf("login module requires credential, user, and challenge facades")
	}
	interactions := logininfra.NewInteractionCacheService(deps.Infra.CacheMgr)
	riskPolicy := domain.NewRiskPolicy(deps.Config.Login)
	service := application.NewService(
		deps.Security.Password,
		refs.UserCredentials,
		refs.Subjects,
		refs.ChallengeInternal,
		refs.ChallengeClient,
		refs.ProofVerifier,
		refs.LoginFailures,
		interactions,
		riskPolicy,
		time.Duration(deps.Config.Login.InteractionTTLSeconds)*time.Second,
		application.PasskeyPublicStartOptions{
			RPID:           deps.Config.Challenge.WebAuthnRPID,
			TimeoutSeconds: deps.Config.Challenge.WebAuthnChallengeTimeoutSeconds,
		},
	)
	service.BindSSO(refs.AuthorizationSessions, refs.AuthenticationCompletion, refs.BootstrapSession)
	service.BindPlatform(refs.Platform)
	service.BindLimiter(deps.Infra.Limiter)
	module := &Module{
		facade:  service,
		handler: loginhandler.NewHandler(service),
	}
	return module, service, nil
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{
		Name:   "login",
		Prefix: "/login",
	}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil || m.handler == nil {
		return
	}
	engine.POST("/login/password/state", m.handler.PasswordState)
	engine.POST("/login/password", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogin,
		Description:   "密码登录",
		IncludeParams: true,
	}, m.handler.Password))
	engine.POST("/login/register/state", m.handler.RegisterState)
	engine.POST("/login/register/email-code", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserCreate,
		Description:   "发送注册邮箱验证码",
		IncludeParams: true,
	}, m.handler.RegisterEmailCode))
	engine.POST("/login/register", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserCreate,
		Description:   "表单注册",
		IncludeParams: true,
	}, m.handler.Register))
	engine.POST("/login/passkey/start", m.handler.StartPasskey)
	engine.POST("/login/passkey/verify", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogin,
		Description:   "Passkey 登录",
		IncludeParams: true,
	}, m.handler.VerifyPasskey))
	engine.POST("/login/totp/verify", m.wrapOperation(adminfacade.OperationLogSpec{
		Operation:     adminfacade.OperationTypeUserLogin,
		Description:   "TOTP 登录",
		IncludeParams: true,
	}, m.handler.VerifyTotp))
}

func (m *Module) Facade() loginfacade.PasswordFlowFacade {
	return m.facade
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m == nil {
		return
	}
	m.oplog = oplog
}

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}
