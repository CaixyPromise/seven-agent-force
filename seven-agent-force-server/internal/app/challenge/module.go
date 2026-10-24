package challenge

import (
	"context"
	"fmt"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain/provider"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	challengehandler "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/handler"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	emailinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/email"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Module struct {
	mfaFacade        facade.MfaCredentialFacade
	managementFacade facade.MfaManagementFacade
	stepFacade       facade.ChallengeStepFacade
	internalFacade   facade.ChallengeInternalFacade
	clientFacade     facade.ChallengeClientFacade
	proofVerifier    facade.ProofTokenVerifier
	internalCtrl     *challengehandler.InternalHandler
	clientCtrl       *challengehandler.ClientHandler
	managementCtrl   *challengehandler.MfaManagementHandler
	oplog            adminfacade.OperationLogger
}

type Dependencies struct {
	UserCredentials credentialfacade.UserCredentialFacade
	Subjects        userfacade.SubjectFacade
	Notifications   notificationfacade.NotificationFacade
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, error) {
	if refs.UserCredentials == nil {
		return nil, fmt.Errorf("challenge module requires credential facade")
	}
	if deps.Security.Totp == nil {
		return nil, fmt.Errorf("challenge module requires totp service")
	}
	subjectResolver := challengeinfra.NewSubjectResolver(refs.Subjects)
	repository := challengeinfra.NewMfaCredentialRepository(refs.UserCredentials)
	store := challengeinfra.NewMfaCredentialStore(refs.UserCredentials, subjectResolver)
	domainService := domain.NewService(repository)
	appService := application.NewService(domainService)
	captchaService := challengeinfra.NewCaptchaService(deps.Security.Random)
	emailSender := emailinfra.Sender(deps.Infra.Email)
	if refs.Notifications != nil {
		emailSender = notificationEmailSender{facade: refs.Notifications}
	}
	emailOTPService := challengeinfra.NewEmailOTPService(deps.Security.Random, emailSender)
	webauthnService := challengeinfra.NewWebAuthnService(deps.Security.Random)
	providers := provider.NewRegistry(
		provider.NewImageCaptchaChallengeStepProvider(captchaService),
		provider.NewPasswordChallengeStepProvider(deps.Security.Password, store),
		provider.NewEmailOtpChallengeStepProvider(emailOTPService, store),
		provider.NewTimeBasedOtpChallengeStepProvider(
			deps.Security.Totp,
			store,
			provider.TimeBasedOtpSettings{
				IssuerName:          challengeIssuerName(deps.Config),
				AllowedDriftWindows: challengeAllowedDriftWindows(deps.Config),
			},
		),
		provider.NewRecoveryCodeChallengeStepProvider(store),
		provider.NewWebAuthnPasskeyAssertionStepProvider(
			webauthnService,
			store,
			deps.Config.Challenge.WebAuthnRPID,
			deps.Config.Challenge.WebAuthnAllowedOrigins,
			deps.Config.Challenge.WebAuthnChallengeTimeoutSeconds,
		),
		provider.NewWebAuthnPasskeyRegistrationStepProvider(
			webauthnService,
			store,
			deps.Config.Challenge.WebAuthnRPID,
			deps.Config.Challenge.WebAuthnRPName,
			deps.Config.Challenge.WebAuthnChallengeTimeoutSeconds,
			deps.Config.Challenge.WebAuthnAllowedOrigins,
		),
	)
	stepService := application.NewStepService(providers)
	completionHandler := application.NewCompletionHandler(store, challengeRecoveryBatchSize(deps.Config))
	sessionRepository := challengeinfra.NewSessionRepository(deps.Infra.CacheMgr)
	throttleRepository := challengeinfra.NewThrottleRepository(deps.Infra.CacheMgr)
	proofTokens := challengeinfra.NewProofTokenService(
		deps.Security.JWT,
		sessionRepository,
		time.Duration(challengeProofTokenTTLMinSeconds(deps.Config))*time.Second,
		time.Duration(challengeProofTokenTTLMaxSeconds(deps.Config))*time.Second,
	)
	challengeService := application.NewChallengeService(
		deps.Config.Challenge,
		sessionRepository,
		throttleRepository,
		stepService,
		completionHandler,
		proofTokens,
	)
	managementService := application.NewMfaManagementService(
		refs.UserCredentials,
		refs.Subjects,
		challengeService,
		challengeService,
		challengeRecoveryBatchSize(deps.Config),
		deps.Config.Challenge.SessionTTLSeconds,
	)
	module := &Module{
		mfaFacade:        appService,
		managementFacade: managementService,
		stepFacade:       application.NewFlowService(stepService, completionHandler),
		internalFacade:   challengeService,
		clientFacade:     challengeService,
		proofVerifier:    challengeService,
		internalCtrl:     challengehandler.NewInternalHandler(challengeService),
		clientCtrl:       challengehandler.NewClientHandler(challengeService),
		managementCtrl:   challengehandler.NewMfaManagementHandler(managementService, nil),
	}
	return module, nil
}

func (m *Module) BindAuthorization(auth authorizationfacade.AuthFacade) {
	if m == nil || m.managementCtrl == nil {
		return
	}
	m.managementCtrl.BindAuthorization(auth)
}

func (m *Module) BindSessions(sessions ssofacade.SessionFacade) {
	_ = sessions
}

func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m == nil {
		return
	}
	m.oplog = oplog
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{
		Name:   "challenge",
		Prefix: "/challenge",
	}
}

func (m *Module) Mount(engine route.IRouter) {
	if engine == nil {
		return
	}
	if m.internalCtrl != nil {
		engine.POST("/internal/challenges/start", m.internalCtrl.Start)
	}
	if m.clientCtrl != nil {
		engine.GET("/v1/challenges/:challengeIdentifier", m.clientCtrl.Get)
		engine.POST("/v1/challenges/:challengeIdentifier/respond", m.clientCtrl.Respond)
		engine.POST("/v1/challenges/:challengeIdentifier/refresh", m.clientCtrl.Refresh)
	}
	if m.managementCtrl != nil {
		engine.POST("/internal/mfa/status", m.managementCtrl.QueryStatusInternal)
		engine.POST("/internal/mfa/recovery-codes/regenerate", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "内部重新生成 MFA 恢复码", IncludeParams: true}, m.managementCtrl.RegenerateRecoveryCodesInternal))
		engine.GET("/v1/mfa/status", m.managementCtrl.QueryStatusForCurrentUser)
		engine.POST("/v1/mfa/recovery-codes/regenerate", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "重新生成当前用户 MFA 恢复码", IncludeParams: true}, m.managementCtrl.RegenerateRecoveryCodesForCurrentUser))
		engine.DELETE("/v1/mfa/otp-binding", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "解绑当前用户 OTP", IncludeParams: true}, m.managementCtrl.DeleteCurrentUserOtpBinding))
		engine.GET("/v1/mfa/passkeys", m.managementCtrl.ListCurrentUserPasskeys)
		engine.DELETE("/v1/mfa/passkeys/:credentialIdentifier", m.wrapOperation(adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: "删除当前用户 Passkey", IncludeParams: true}, m.managementCtrl.DeleteCurrentUserPasskey))
		engine.POST("/v1/mfa/challenges/start", m.managementCtrl.StartMfaChallenge)
	}
}

func (m *Module) wrapOperation(spec adminfacade.OperationLogSpec, handler app.HandlerFunc) app.HandlerFunc {
	if m == nil || m.oplog == nil {
		return handler
	}
	return m.oplog.Wrap(spec, handler)
}

func (m *Module) Facade() facade.MfaCredentialFacade {
	return m.mfaFacade
}

func (m *Module) Management() facade.MfaManagementFacade {
	return m.managementFacade
}

func (m *Module) Steps() facade.ChallengeStepFacade {
	return m.stepFacade
}

func (m *Module) InternalFacade() facade.ChallengeInternalFacade {
	return m.internalFacade
}

func (m *Module) ClientFacade() facade.ChallengeClientFacade {
	return m.clientFacade
}

func (m *Module) ProofTokenVerifier() facade.ProofTokenVerifier {
	return m.proofVerifier
}

func challengeIssuerName(cfg config.Config) string {
	if value := cfg.Challenge.OTPIssuerName; value != "" {
		return value
	}
	return cfg.Seven.Name
}

func challengeAllowedDriftWindows(cfg config.Config) int {
	if cfg.Challenge.OTPAllowedDriftWindows < 0 {
		return 0
	}
	return cfg.Challenge.OTPAllowedDriftWindows
}

func challengeRecoveryBatchSize(cfg config.Config) int {
	if cfg.Challenge.RecoveryBatchSize <= 0 {
		return 10
	}
	return cfg.Challenge.RecoveryBatchSize
}

func challengeProofTokenTTLMinSeconds(cfg config.Config) int {
	if cfg.Challenge.ProofTokenTTLMinSeconds <= 0 {
		return 60
	}
	return cfg.Challenge.ProofTokenTTLMinSeconds
}

func challengeProofTokenTTLMaxSeconds(cfg config.Config) int {
	if cfg.Challenge.ProofTokenTTLMaxSeconds <= 0 {
		return 300
	}
	return cfg.Challenge.ProofTokenTTLMaxSeconds
}

type notificationEmailSender struct {
	facade notificationfacade.NotificationFacade
}

func (s notificationEmailSender) SendChallengeOTP(ctx context.Context, request emailinfra.ChallengeOTPRequest) error {
	if s.facade == nil {
		return nil
	}
	return s.facade.EnqueueChallengeOTP(ctx, notificationfacade.ChallengeOTPRequest{
		ToEmail:   request.ToEmail,
		Code:      request.Code,
		Scene:     request.Scene,
		SceneName: request.SceneName,
		TTL:       request.TTL,
		Metadata:  request.Metadata,
	})
}
