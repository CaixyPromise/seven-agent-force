package application

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/login/domain"
	loginfacade "github.com/CaixyPromise/seven-agent/force/internal/app/login/facade"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	limiterinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/limiter"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/google/uuid"
)

var otpPattern = regexp.MustCompile(`^\d{6}$`)

const (
	registerIPLimit                int64 = 10
	registerPlatformLimit          int64 = 5
	registerSubjectLimit           int64 = 3
	registerDeviceLimit            int64 = 5
	registerEmailSendIPLimit       int64 = 10
	registerEmailSendPlatformLimit int64 = 5
	registerEmailSendSubjectLimit  int64 = 3
	registerEmailSendDeviceLimit   int64 = 5

	registerIPWindow       = time.Hour
	registerPlatformWindow = 10 * time.Minute
	registerSubjectWindow  = time.Hour
	registerDeviceWindow   = time.Hour
	registerEmailCodeTTL   = 5 * time.Minute
	registerEmailCooldown  = 60
)

type Service struct {
	password        *passwordinfra.Service
	credentials     credentialfacade.UserCredentialFacade
	subjects        userfacade.SubjectFacade
	challengeInt    challengefacade.ChallengeInternalFacade
	challengeClient challengefacade.ChallengeClientFacade
	proofVerifier   challengefacade.ProofTokenVerifier
	authSessions    ssofacade.AuthorizationSessionFacade
	authCompletion  ssofacade.AuthenticationCompletionFacade
	bootstrap       ssofacade.BootstrapSessionFacade
	platform        platformfacade.PublicFacade
	loginFailures   adminfacade.LoginFailureFacade
	interactions    interactionStore
	riskPolicy      *domain.RiskPolicy
	interactionTTL  time.Duration
	passkeyPublic   PasskeyPublicStartOptions
	limiter         limiterinfra.Limiter
}

type PasskeyPublicStartOptions struct {
	RPID           string
	TimeoutSeconds int
}

type interactionStore interface {
	GetInteraction(ctx context.Context, loginTransactionID string) (*domain.InteractionSnapshot, error)
	SaveInteraction(ctx context.Context, snapshot *domain.InteractionSnapshot, ttl time.Duration) error
	RemoveInteraction(ctx context.Context, loginTransactionID string) error
	IsCompleted(ctx context.Context, loginTransactionID string) (bool, error)
	MarkCompleted(ctx context.Context, loginTransactionID string, ttl time.Duration) error
	AcquirePrimaryAuthenticationLock(ctx context.Context, loginTransactionID string) (bool, error)
	ReleasePrimaryAuthenticationLock(ctx context.Context, loginTransactionID string) error
	AcquireChallengeDispatchLock(ctx context.Context, loginTransactionID string) (bool, error)
	ReleaseChallengeDispatchLock(ctx context.Context, loginTransactionID string) error
}

func NewService(
	password *passwordinfra.Service,
	credentials credentialfacade.UserCredentialFacade,
	subjects userfacade.SubjectFacade,
	challengeInt challengefacade.ChallengeInternalFacade,
	challengeClient challengefacade.ChallengeClientFacade,
	proofVerifier challengefacade.ProofTokenVerifier,
	loginFailures adminfacade.LoginFailureFacade,
	interactions interactionStore,
	riskPolicy *domain.RiskPolicy,
	interactionTTL time.Duration,
	passkeyPublicOptions ...PasskeyPublicStartOptions,
) *Service {
	passkeyPublic := PasskeyPublicStartOptions{RPID: "localhost", TimeoutSeconds: 60}
	if len(passkeyPublicOptions) > 0 {
		passkeyPublic = passkeyPublicOptions[0]
	}
	passkeyPublic.RPID = strings.TrimSpace(passkeyPublic.RPID)
	if passkeyPublic.RPID == "" {
		passkeyPublic.RPID = "localhost"
	}
	if passkeyPublic.TimeoutSeconds <= 0 {
		passkeyPublic.TimeoutSeconds = 60
	}
	return &Service{
		password:        password,
		credentials:     credentials,
		subjects:        subjects,
		challengeInt:    challengeInt,
		challengeClient: challengeClient,
		proofVerifier:   proofVerifier,
		loginFailures:   loginFailures,
		interactions:    interactions,
		riskPolicy:      riskPolicy,
		interactionTTL:  interactionTTL,
		passkeyPublic:   passkeyPublic,
	}
}

func (s *Service) BindSSO(authSessions ssofacade.AuthorizationSessionFacade, completion ssofacade.AuthenticationCompletionFacade, bootstrap ssofacade.BootstrapSessionFacade) {
	s.authSessions = authSessions
	s.authCompletion = completion
	s.bootstrap = bootstrap
}

func (s *Service) BindPlatform(platform platformfacade.PublicFacade) {
	s.platform = platform
}

func (s *Service) BindLimiter(limiter limiterinfra.Limiter) {
	s.limiter = limiter
}

func (s *Service) getFailureCount(ctx context.Context, userAccount string) (int, error) {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return 0, nil
	}
	return s.loginFailures.GetFailureCount(ctx, strings.TrimSpace(userAccount))
}

func (s *Service) getRiskFailureCount(ctx context.Context, userAccount string, request *loginfacade.RequestContext) (int, error) {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return 0, nil
	}
	clientIP, deviceID := loginFailureContext(request)
	return s.loginFailures.GetRiskFailureCount(ctx, strings.TrimSpace(userAccount), clientIP, deviceID)
}

func (s *Service) isLocked(ctx context.Context, userAccount string) (bool, *int64, error) {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return false, nil, nil
	}
	locked, err := s.loginFailures.IsAccountLocked(ctx, strings.TrimSpace(userAccount))
	if err != nil || !locked {
		return locked, nil, err
	}
	unlockTime, err := s.loginFailures.GetUnlockTime(ctx, strings.TrimSpace(userAccount))
	if err != nil {
		return false, nil, err
	}
	return true, unlockTime, nil
}

func (s *Service) clearFailureState(ctx context.Context, userAccount string) error {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	return s.loginFailures.ClearFailure(ctx, strings.TrimSpace(userAccount))
}

func (s *Service) clearCaptchaFailureState(ctx context.Context, userAccount string) error {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	return s.loginFailures.ClearCaptchaFailure(ctx, strings.TrimSpace(userAccount))
}

func (s *Service) recordFailure(ctx context.Context, userAccount string, request *loginfacade.RequestContext) (int, bool, *int64, error) {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return 0, false, nil, nil
	}
	clientIP := ""
	deviceID := ""
	if request != nil {
		clientIP = strings.TrimSpace(request.LoginIP)
		deviceID = strings.TrimSpace(request.DeviceID)
	}
	userAccount = strings.TrimSpace(userAccount)
	if err := s.loginFailures.RecordFailure(ctx, userAccount, clientIP, deviceID); err != nil {
		return 0, false, nil, err
	}
	count, err := s.loginFailures.GetRiskFailureCount(ctx, userAccount, clientIP, deviceID)
	if err != nil {
		return 0, false, nil, err
	}
	locked, err := s.loginFailures.IsAccountLocked(ctx, userAccount)
	if err != nil || !locked {
		return count, locked, nil, err
	}
	unlockTime, err := s.loginFailures.GetUnlockTime(ctx, userAccount)
	if err != nil {
		return count, false, nil, err
	}
	return count, true, unlockTime, nil
}

func (s *Service) recordCaptchaFailure(ctx context.Context, userAccount string, request *loginfacade.RequestContext) error {
	if s.loginFailures == nil || strings.TrimSpace(userAccount) == "" {
		return nil
	}
	clientIP := ""
	if request != nil {
		clientIP = strings.TrimSpace(request.LoginIP)
	}
	return s.loginFailures.RecordCaptchaFailure(ctx, strings.TrimSpace(userAccount), clientIP)
}

func loginFailureContext(request *loginfacade.RequestContext) (string, string) {
	if request == nil {
		return "", ""
	}
	return strings.TrimSpace(request.LoginIP), strings.TrimSpace(request.DeviceID)
}

func (s *Service) GetPasswordState(ctx context.Context, request loginfacade.PasswordStateRequest) (*loginfacade.PasswordState, error) {
	userAccount, snapshot, err := s.prepareInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext, "PASSWORD", "")
	if err != nil {
		return nil, err
	}
	if locked, expiresAt, err := s.isLocked(ctx, userAccount); err != nil {
		return nil, err
	} else if locked {
		return &loginfacade.PasswordState{
			CanPasswordLogin: true,
			Locked:           true,
			LockExpiresAt:    expiresAt,
		}, nil
	}
	failureCount, err := s.getRiskFailureCount(ctx, userAccount, request.RequestContext)
	if err != nil {
		return nil, err
	}
	response := &loginfacade.PasswordState{
		CanPasswordLogin: true,
		CaptchaRequired:  s.riskPolicy.RequiresCaptcha(failureCount),
	}
	subject, _ := s.findSubjectByAccount(ctx, userAccount)
	if subject != nil {
		snapshot.UserID = subject.UserID
	}
	if subject != nil && subject.UserID > 0 && s.riskPolicy.RequiresTotp(failureCount) {
		if record, err := s.credentials.FindActiveTotpByUserID(ctx, subject.UserID); err != nil {
			return nil, err
		} else if record != nil {
			response.TotpRequired = true
		}
	}
	if response.CaptchaRequired {
		var captcha *loginfacade.Captcha
		if request.RefreshCaptcha {
			captcha, err = s.refreshPrimaryCaptcha(ctx, snapshot, userAccount, request.RequestContext)
		} else {
			captcha, err = s.resolvePrimaryCaptcha(ctx, snapshot, userAccount, request.RequestContext)
		}
		if err != nil {
			return nil, err
		}
		response.Captcha = captcha
	}
	if err := s.saveInteraction(ctx, snapshot); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) SubmitPassword(ctx context.Context, request loginfacade.PasswordSubmitRequest) (*loginfacade.PasswordSubmitResult, error) {
	if strings.TrimSpace(request.Password) == "" {
		return nil, apperrors.Params("账号和密码不能为空")
	}
	locked, err := s.interactions.AcquirePrimaryAuthenticationLock(ctx, request.LoginTransactionID)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, apperrors.Operation("登录主认证正在被并发处理")
	}
	defer func() {
		_ = s.interactions.ReleasePrimaryAuthenticationLock(ctx, request.LoginTransactionID)
	}()

	userAccount, snapshot, err := s.prepareInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext, "PASSWORD", "")
	if err != nil {
		return nil, err
	}
	if locked, expiresAt, err := s.isLocked(ctx, userAccount); err != nil {
		return nil, err
	} else if locked {
		return &loginfacade.PasswordSubmitResult{
			CanPasswordLogin: true,
			Locked:           true,
			LockExpiresAt:    expiresAt,
		}, nil
	}
	failureCount, err := s.getRiskFailureCount(ctx, userAccount, request.RequestContext)
	if err != nil {
		return nil, err
	}
	if s.riskPolicy.RequiresCaptcha(failureCount) && !snapshot.PrimaryChallengeSatisfied {
		if strings.TrimSpace(request.CaptchaCode) == "" {
			captcha, err := s.resolvePrimaryCaptcha(ctx, snapshot, userAccount, request.RequestContext)
			if err != nil {
				return nil, err
			}
			if err := s.saveInteraction(ctx, snapshot); err != nil {
				return nil, err
			}
			return &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				CaptchaRequired:  true,
				Captcha:          captcha,
			}, nil
		}
		ok, captcha, err := s.verifyPrimaryCaptcha(ctx, snapshot, userAccount, request.CaptchaCode, request.RequestContext)
		if err != nil {
			return nil, err
		}
		if !ok {
			if err := s.saveInteraction(ctx, snapshot); err != nil {
				return nil, err
			}
			return &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				CaptchaRequired:  true,
				CaptchaRejected:  true,
				Captcha:          captcha,
			}, nil
		}
	}
	subject, err := s.findSubjectByAccount(ctx, userAccount)
	if err != nil {
		return nil, err
	}
	passwordCredential := (*credentialfacade.PasswordCredential)(nil)
	if subject != nil && (subject.Enabled || subject.Status == userfacade.UserStatusPendingReview) {
		passwordCredential, err = s.credentials.FindActivePasswordByUserID(ctx, subject.UserID)
		if err != nil {
			return nil, err
		}
	}
	passwordMatched := passwordCredential != nil && s.password != nil && s.password.Verify(ctx, request.Password, passwordCredential.PasswordHash) == nil
	if subject != nil && subject.Status == userfacade.UserStatusPendingReview && passwordMatched {
		snapshot.PrimaryChallengeSatisfied = false
		if err := s.saveInteraction(ctx, snapshot); err != nil {
			return nil, err
		}
		return nil, apperrors.Forbidden("账号正在审核中，请等待管理员审核通过")
	}
	if subject == nil || !subject.Enabled || !passwordMatched {
		count, lockedNow, expiresAt, err := s.recordFailure(ctx, userAccount, request.RequestContext)
		if err != nil {
			return nil, err
		}
		snapshot.PrimaryChallengeSatisfied = false
		if lockedNow {
			if err := s.saveInteraction(ctx, snapshot); err != nil {
				return nil, err
			}
			return &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				Locked:           true,
				LockExpiresAt:    expiresAt,
			}, nil
		}
		if s.riskPolicy.RequiresCaptcha(count) {
			captcha, err := s.resolvePrimaryCaptcha(ctx, snapshot, userAccount, request.RequestContext)
			if err != nil {
				return nil, err
			}
			if err := s.saveInteraction(ctx, snapshot); err != nil {
				return nil, err
			}
			return &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				CaptchaRequired:  true,
				Captcha:          captcha,
			}, nil
		}
		if err := s.saveInteraction(ctx, snapshot); err != nil {
			return nil, err
		}
		return nil, apperrors.Unauthorized("账号或密码错误")
	}

	now := time.Now().UTC()
	if err := s.credentials.MarkPasswordUsed(ctx, subject.UserID, now); err != nil {
		return nil, err
	}
	snapshot.UserID = subject.UserID
	snapshot.UserAccount = subject.AccountName
	snapshot.PrimaryChallengeSatisfied = false
	snapshot.PrimaryChallengeIdentifier = ""
	snapshot.PrimaryChallengeFlowNonce = ""
	if strings.TrimSpace(snapshot.FlowNonce) == "" {
		snapshot.FlowNonce = uuid.NewString()
	}
	if err := s.saveInteraction(ctx, snapshot); err != nil {
		return nil, err
	}
	if s.riskPolicy.RequiresTotp(failureCount) {
		if totpCredential, err := s.credentials.FindActiveTotpByUserID(ctx, subject.UserID); err != nil {
			return nil, err
		} else if totpCredential != nil {
			return &loginfacade.PasswordSubmitResult{
				CanPasswordLogin: true,
				TotpRequired:     true,
			}, nil
		}
	}
	if err := s.clearFailureState(ctx, userAccount); err != nil {
		return nil, err
	}
	success, err := s.completeLogin(ctx, snapshot, request.RequestContext, []string{"pwd"})
	if err != nil {
		return nil, err
	}
	if err := s.interactions.MarkCompleted(ctx, request.LoginTransactionID, s.interactionTTL); err != nil {
		return nil, err
	}
	_ = s.interactions.RemoveInteraction(ctx, request.LoginTransactionID)
	return &loginfacade.PasswordSubmitResult{
		Authenticated:            true,
		RedirectURL:              success.RedirectURL,
		AccessToken:              success.AccessToken,
		TokenType:                success.TokenType,
		AccessTTLSeconds:         success.AccessTTLSeconds,
		SessionCookieHeaderValue: success.SessionCookieHeaderValue,
		RefreshCookieHeaderValue: success.RefreshCookieHeaderValue,
		CanPasswordLogin:         true,
	}, nil
}

func (s *Service) VerifyTotp(ctx context.Context, request loginfacade.TotpVerifyRequest) (*loginfacade.TotpVerifyResult, error) {
	if !otpPattern.MatchString(strings.TrimSpace(request.OTPCode)) {
		return nil, apperrors.Params("TOTP 格式非法")
	}
	userAccount, snapshot, err := s.prepareInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext, "PASSWORD", "")
	if err != nil {
		return nil, err
	}
	if snapshot.UserID <= 0 {
		return nil, apperrors.Params("密码阶段尚未完成")
	}
	if locked, expiresAt, err := s.isLocked(ctx, userAccount); err != nil {
		return nil, err
	} else if locked {
		return &loginfacade.TotpVerifyResult{Locked: true, LockExpiresAt: expiresAt}, nil
	}
	challenge, err := s.startOrReuseTotpChallenge(ctx, snapshot, request.RequestContext)
	if err != nil {
		return nil, err
	}
	stepIdentifier := findStepIdentifier(challenge, "TIME_BASED_ONE_TIME_PASSWORD")
	if stepIdentifier == "" {
		return nil, apperrors.Operation("TOTP 挑战步骤不可用")
	}
	response, err := s.challengeClient.Respond(ctx, challenge.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: stepIdentifier,
		Payload: map[string]any{
			"oneTimePassword": strings.TrimSpace(request.OTPCode),
		},
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.ChallengeState) == "PASSED" {
		if _, err := s.proofVerifier.VerifyProofToken(ctx, challengefacade.ProofTokenVerifyRequest{
			ProofToken:          response.ProofToken,
			AudienceServiceName: "login-app",
			BusinessAction:      "LOGIN",
			FlowNonce:           snapshot.FlowNonce,
			SubjectIdentifier:   "user:" + int64ToString(snapshot.UserID),
			ConsumeOnce:         true,
		}); err != nil {
			return nil, err
		}
		if err := s.clearFailureState(ctx, userAccount); err != nil {
			return nil, err
		}
		success, err := s.completeLogin(ctx, snapshot, request.RequestContext, []string{"pwd", "otp"})
		if err != nil {
			return nil, err
		}
		if err := s.interactions.MarkCompleted(ctx, request.LoginTransactionID, s.interactionTTL); err != nil {
			return nil, err
		}
		_ = s.interactions.RemoveInteraction(ctx, request.LoginTransactionID)
		return &loginfacade.TotpVerifyResult{
			Authenticated:            true,
			RedirectURL:              success.RedirectURL,
			AccessToken:              success.AccessToken,
			TokenType:                success.TokenType,
			AccessTTLSeconds:         success.AccessTTLSeconds,
			SessionCookieHeaderValue: success.SessionCookieHeaderValue,
			RefreshCookieHeaderValue: success.RefreshCookieHeaderValue,
		}, nil
	}
	return s.recordTotpVerifyFailure(ctx, userAccount, request.RequestContext, response)
}

func (s *Service) GetRegisterState(ctx context.Context, request loginfacade.RegisterStateRequest) (*loginfacade.RegisterState, error) {
	userAccount, snapshot, err := s.prepareRegisterInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireFormRegistrationPolicy(ctx, snapshot, request.RequestContext); err != nil {
		return nil, err
	}
	captcha, err := s.resolveRegisterCaptcha(ctx, snapshot, userAccount, request.RequestContext)
	if err != nil {
		return nil, err
	}
	if err := s.saveInteraction(ctx, snapshot); err != nil {
		return nil, err
	}
	return &loginfacade.RegisterState{CanRegister: true, Captcha: captcha}, nil
}

func (s *Service) SendRegisterEmailCode(ctx context.Context, request loginfacade.RegisterEmailCodeRequest) (*loginfacade.RegisterEmailCodeResult, error) {
	userAccount, snapshot, err := s.prepareRegisterInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireFormRegistrationPolicy(ctx, snapshot, request.RequestContext); err != nil {
		return nil, err
	}
	userEmail, err := normalizeRegisterEmail(request.UserEmail)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(request.CaptchaCode) == "" {
		captcha, captchaErr := s.resolveRegisterCaptcha(ctx, snapshot, userAccount, request.RequestContext)
		if captchaErr != nil {
			return nil, captchaErr
		}
		if saveErr := s.saveInteraction(ctx, snapshot); saveErr != nil {
			return nil, saveErr
		}
		return &loginfacade.RegisterEmailCodeResult{
			Sent:             false,
			CooldownSeconds:  registerEmailCooldown,
			ExpiresInSeconds: int(registerEmailCodeTTL / time.Second),
			Message:          "请输入图形验证码",
			Captcha:          captcha,
		}, nil
	}
	ok, captcha, err := s.verifyRegisterCaptcha(ctx, snapshot, userAccount, request.CaptchaCode, request.RequestContext)
	if err != nil {
		return nil, err
	}
	if !ok {
		if saveErr := s.saveInteraction(ctx, snapshot); saveErr != nil {
			return nil, saveErr
		}
		return &loginfacade.RegisterEmailCodeResult{
			Sent:             false,
			CooldownSeconds:  registerEmailCooldown,
			ExpiresInSeconds: int(registerEmailCodeTTL / time.Second),
			Message:          "图形验证码错误，请重新输入",
			Captcha:          captcha,
		}, nil
	}
	if err := s.enforceRegisterEmailCodeRateLimit(ctx, snapshot.PlatformCode, userAccount, userEmail, request.RequestContext); err != nil {
		return nil, err
	}
	challenge, err := s.startOrRefreshRegisterEmailChallenge(ctx, snapshot, userAccount, userEmail, request.RequestContext)
	if err != nil {
		return nil, err
	}
	snapshot.RegisterEmail = userEmail
	snapshot.RegisterEmailSatisfied = false
	if err := s.saveInteraction(ctx, snapshot); err != nil {
		return nil, err
	}
	return &loginfacade.RegisterEmailCodeResult{
		Sent:             true,
		EmailMasked:      registerEmailMasked(challenge),
		CooldownSeconds:  registerEmailCooldown,
		ExpiresInSeconds: int(registerEmailCodeTTL / time.Second),
		Message:          "验证码已发送，请查收邮箱",
	}, nil
}

func (s *Service) SubmitRegister(ctx context.Context, request loginfacade.RegisterSubmitRequest) (*loginfacade.RegisterSubmitResult, error) {
	if strings.TrimSpace(request.Password) == "" || request.Password != request.ConfirmPassword {
		return nil, apperrors.Params("两次输入的密码不一致")
	}
	userAccount, snapshot, err := s.prepareRegisterInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext)
	if err != nil {
		return nil, err
	}
	policy, err := s.requireFormRegistrationPolicy(ctx, snapshot, request.RequestContext)
	if err != nil {
		return nil, err
	}
	userEmail, err := normalizeRegisterEmail(request.UserEmail)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(snapshot.RegisterEmail) != userEmail || strings.TrimSpace(snapshot.RegisterEmailIdentifier) == "" {
		return &loginfacade.RegisterSubmitResult{
			Registered:  false,
			UserAccount: userAccount,
			Message:     "请先获取邮箱验证码",
		}, nil
	}
	if strings.TrimSpace(request.EmailCode) == "" {
		return &loginfacade.RegisterSubmitResult{
			Registered:  false,
			UserAccount: userAccount,
			Message:     "请输入邮箱验证码",
		}, nil
	}
	ok, err := s.verifyRegisterEmailCode(ctx, snapshot, userAccount, userEmail, request.EmailCode)
	if err != nil {
		return nil, err
	}
	if !ok {
		if saveErr := s.saveInteraction(ctx, snapshot); saveErr != nil {
			return nil, saveErr
		}
		return &loginfacade.RegisterSubmitResult{
			Registered:  false,
			UserAccount: userAccount,
			Message:     "邮箱验证码错误或已过期",
		}, nil
	}
	if err := s.enforceRegisterRateLimit(ctx, snapshot.PlatformCode, userAccount, userEmail, request.RequestContext); err != nil {
		return nil, err
	}
	subject, err := s.subjects.CreateFormSubject(ctx, userfacade.CreateFormSubjectCommand{
		AccountName:          userAccount,
		NickName:             strings.TrimSpace(request.UserName),
		UserEmail:            userEmail,
		RawPassword:          request.Password,
		RegisterPlatformCode: snapshot.PlatformCode,
		DefaultOrgID:         policy.DefaultOrgID,
		DefaultDeptID:        policy.DefaultDeptID,
		DefaultPostIDs:       policy.DefaultPostIDs,
		DefaultRoleIDs:       policy.DefaultRoleIDs,
	})
	if err != nil {
		return nil, err
	}
	snapshot.RegisterEmailSatisfied = true
	if saveErr := s.saveInteraction(ctx, snapshot); saveErr != nil {
		return nil, saveErr
	}
	var userID int64
	if subject != nil {
		userID = subject.UserID
	}
	return &loginfacade.RegisterSubmitResult{
		Registered:  true,
		UserID:      userID,
		UserAccount: userAccount,
		Message:     "注册成功，请使用新账号登录",
	}, nil
}

func (s *Service) StartPasskey(ctx context.Context, request loginfacade.PasskeyStartRequest) (*loginfacade.PasskeyStartResult, error) {
	_, snapshot, err := s.prepareInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext, "PASSKEY", "")
	if err != nil {
		return nil, err
	}
	subject, err := s.findSubjectByAccount(ctx, snapshot.UserAccount)
	if err != nil {
		return nil, err
	}
	if subject == nil || !subject.Enabled {
		if err := s.saveInteraction(ctx, snapshot); err != nil {
			return nil, err
		}
		return s.publicPasskeyStartResult()
	}
	snapshot.UserID = subject.UserID
	snapshot.UserAccount = subject.AccountName
	if err := s.saveInteraction(ctx, snapshot); err != nil {
		return nil, err
	}
	items, err := s.credentials.ListActivePasskeys(ctx, subject.UserID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return s.publicPasskeyStartResult()
	}
	challenge, err := s.startOrReusePasskeyChallenge(ctx, snapshot, request.RequestContext)
	if err != nil {
		return s.publicPasskeyStartResult()
	}
	step := findStep(challenge, "WEBAUTHN_PASSKEY_ASSERTION")
	if step == nil {
		return s.publicPasskeyStartResult()
	}
	publicChallenge := sanitizePasskeyStartChallenge(challenge, step.StepIdentifier)
	publicStep := findStep(publicChallenge, "WEBAUTHN_PASSKEY_ASSERTION")
	if publicStep == nil {
		return s.publicPasskeyStartResult()
	}
	return &loginfacade.PasskeyStartResult{
		ChallengeIdentifier: publicChallenge.ChallengeIdentifier,
		StepIdentifier:      publicStep.StepIdentifier,
		UserInterfaceHints:  publicStep.UserInterfaceHints,
		Challenge:           publicChallenge,
	}, nil
}

func (s *Service) publicPasskeyStartResult() (*loginfacade.PasskeyStartResult, error) {
	challengeValue, err := randomBase64URL(32)
	if err != nil {
		return nil, err
	}
	challengeIdentifier := uuid.NewString()
	stepIdentifier := uuid.NewString()
	hints := map[string]any{
		"challenge":          challengeValue,
		"rpId":               s.passkeyPublic.RPID,
		"timeoutSeconds":     s.passkeyPublic.TimeoutSeconds,
		"allowCredentialIds": []string{},
	}
	expiresAt := time.Now().UTC().Add(s.interactionTTL)
	challenge := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier:        challengeIdentifier,
		ExpiresAt:                  &expiresAt,
		ChallengeState:             "PENDING",
		EffectiveTimeToLiveSeconds: int(s.interactionTTL / time.Second),
		RecommendedStepIdentifier:  stepIdentifier,
		ActualChallengeTypeNames:   []string{"WEBAUTHN_PASSKEY_ASSERTION"},
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier:     stepIdentifier,
			ChallengeType:      "WEBAUTHN_PASSKEY_ASSERTION",
			StepState:          "PENDING",
			UserInterfaceHints: hints,
		}},
	}
	return &loginfacade.PasskeyStartResult{
		ChallengeIdentifier: challengeIdentifier,
		StepIdentifier:      stepIdentifier,
		UserInterfaceHints:  hints,
		Challenge:           challenge,
	}, nil
}

func sanitizePasskeyStartChallenge(challenge *challengefacade.StartChallengeResponse, stepIdentifier string) *challengefacade.StartChallengeResponse {
	if challenge == nil {
		return nil
	}
	clone := *challenge
	clone.Steps = make([]challengefacade.ChallengeStepVO, 0, len(challenge.Steps))
	for _, step := range challenge.Steps {
		copied := step
		if copied.StepIdentifier == stepIdentifier || copied.ChallengeType == "WEBAUTHN_PASSKEY_ASSERTION" {
			copied.UserInterfaceHints = sanitizePasskeyStartHints(step.UserInterfaceHints)
		} else if step.UserInterfaceHints != nil {
			copied.UserInterfaceHints = copyHints(step.UserInterfaceHints)
		}
		clone.Steps = append(clone.Steps, copied)
	}
	return &clone
}

func sanitizePasskeyStartHints(hints map[string]any) map[string]any {
	copied := copyHints(hints)
	if copied == nil {
		copied = make(map[string]any, 1)
	}
	copied["allowCredentialIds"] = []string{}
	return copied
}

func copyHints(hints map[string]any) map[string]any {
	if hints == nil {
		return nil
	}
	copied := make(map[string]any, len(hints))
	for key, value := range hints {
		copied[key] = value
	}
	return copied
}

func randomBase64URL(length int) (string, error) {
	if length <= 0 {
		length = 32
	}
	buf := make([]byte, length)
	if _, err := cryptorand.Read(buf); err != nil {
		return "", fmt.Errorf("generate public passkey challenge: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (s *Service) VerifyPasskey(ctx context.Context, request loginfacade.PasskeyVerifyRequest) (*loginfacade.PasskeyVerifyResult, error) {
	userAccount, snapshot, err := s.prepareInteraction(ctx, request.LoginTransactionID, request.LoginContextID, request.UserAccount, request.RequestContext, "PASSKEY", "")
	if err != nil {
		return nil, err
	}
	subject, err := s.findSubjectByAccount(ctx, userAccount)
	if err != nil {
		return nil, err
	}
	if subject == nil || !subject.Enabled {
		return &loginfacade.PasskeyVerifyResult{Authenticated: false}, nil
	}
	snapshot.UserID = subject.UserID
	snapshot.UserAccount = subject.AccountName
	items, err := s.credentials.ListActivePasskeys(ctx, subject.UserID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return &loginfacade.PasskeyVerifyResult{Authenticated: false}, nil
	}
	if locked, expiresAt, err := s.isLocked(ctx, userAccount); err != nil {
		return nil, err
	} else if locked {
		return &loginfacade.PasskeyVerifyResult{Locked: true, LockExpiresAt: expiresAt}, nil
	}
	challenge, err := s.startOrReusePasskeyChallenge(ctx, snapshot, request.RequestContext)
	if err != nil {
		return &loginfacade.PasskeyVerifyResult{Authenticated: false}, nil
	}
	stepIdentifier := findStepIdentifier(challenge, "WEBAUTHN_PASSKEY_ASSERTION")
	if stepIdentifier == "" {
		return s.recordPasskeyVerifyFailure(ctx, userAccount, request.RequestContext, nil)
	}
	response, err := s.challengeClient.Respond(ctx, challenge.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: stepIdentifier,
		Payload: map[string]any{
			"credentialIdentifier": request.CredentialIdentifier,
			"clientDataJSON":       request.ClientDataJSON,
			"authenticatorData":    request.AuthenticatorData,
			"signature":            request.Signature,
		},
	})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.ChallengeState) == "PASSED" {
		if _, err := s.proofVerifier.VerifyProofToken(ctx, challengefacade.ProofTokenVerifyRequest{
			ProofToken:          response.ProofToken,
			AudienceServiceName: "login-app",
			BusinessAction:      "LOGIN",
			FlowNonce:           snapshot.FlowNonce,
			SubjectIdentifier:   "user:" + int64ToString(snapshot.UserID),
			ConsumeOnce:         true,
		}); err != nil {
			return nil, err
		}
		if err := s.clearFailureState(ctx, userAccount); err != nil {
			return nil, err
		}
		success, err := s.completeLogin(ctx, snapshot, request.RequestContext, []string{"hwk"})
		if err != nil {
			return nil, err
		}
		if err := s.interactions.MarkCompleted(ctx, request.LoginTransactionID, s.interactionTTL); err != nil {
			return nil, err
		}
		_ = s.interactions.RemoveInteraction(ctx, request.LoginTransactionID)
		return &loginfacade.PasskeyVerifyResult{
			Authenticated:            true,
			RedirectURL:              success.RedirectURL,
			AccessToken:              success.AccessToken,
			TokenType:                success.TokenType,
			AccessTTLSeconds:         success.AccessTTLSeconds,
			SessionCookieHeaderValue: success.SessionCookieHeaderValue,
			RefreshCookieHeaderValue: success.RefreshCookieHeaderValue,
		}, nil
	}
	if isPasskeyFailureResponse(response) {
		return s.recordPasskeyVerifyFailure(ctx, userAccount, request.RequestContext, response)
	}
	return &loginfacade.PasskeyVerifyResult{Authenticated: false}, nil
}

func (s *Service) recordPasskeyVerifyFailure(ctx context.Context, userAccount string, requestContext *loginfacade.RequestContext, response *challengefacade.RespondChallengeResponse) (*loginfacade.PasskeyVerifyResult, error) {
	_, lockedNow, expiresAt, err := s.recordFailure(ctx, userAccount, requestContext)
	if err != nil {
		return nil, err
	}
	if lockedNow {
		return &loginfacade.PasskeyVerifyResult{Locked: true, LockExpiresAt: expiresAt}, nil
	}
	if isPasskeyChallengePunishment(response) {
		return &loginfacade.PasskeyVerifyResult{Locked: true, LockExpiresAt: passkeyChallengeLockExpiresAt(response)}, nil
	}
	return &loginfacade.PasskeyVerifyResult{Authenticated: false}, nil
}

func (s *Service) recordTotpVerifyFailure(ctx context.Context, userAccount string, requestContext *loginfacade.RequestContext, response *challengefacade.RespondChallengeResponse) (*loginfacade.TotpVerifyResult, error) {
	_, lockedNow, expiresAt, err := s.recordFailure(ctx, userAccount, requestContext)
	if err != nil {
		return nil, err
	}
	if lockedNow {
		return &loginfacade.TotpVerifyResult{Locked: true, LockExpiresAt: expiresAt}, nil
	}
	if isChallengePunishment(response) {
		return &loginfacade.TotpVerifyResult{Locked: true, LockExpiresAt: challengeLockExpiresAt(response)}, nil
	}
	return &loginfacade.TotpVerifyResult{Authenticated: false}, nil
}

func isPasskeyFailureResponse(response *challengefacade.RespondChallengeResponse) bool {
	if response == nil {
		return false
	}
	if strings.TrimSpace(response.ChallengeState) != "PASSED" {
		return true
	}
	switch strings.TrimSpace(response.FailureReason) {
	case "STEP_VERIFY_FAILED", "STEP_LOCKED", "CHALLENGE_THROTTLED":
		return true
	default:
		return false
	}
}

func isPasskeyChallengePunishment(response *challengefacade.RespondChallengeResponse) bool {
	return isChallengePunishment(response)
}

func isChallengePunishment(response *challengefacade.RespondChallengeResponse) bool {
	if response == nil {
		return false
	}
	switch strings.TrimSpace(response.FailureReason) {
	case "STEP_LOCKED", "CHALLENGE_THROTTLED":
		return true
	default:
		return false
	}
}

func passkeyChallengeLockExpiresAt(response *challengefacade.RespondChallengeResponse) *int64 {
	return challengeLockExpiresAt(response)
}

func challengeLockExpiresAt(response *challengefacade.RespondChallengeResponse) *int64 {
	if response == nil || response.CooldownSeconds <= 0 {
		return nil
	}
	value := time.Now().UTC().Add(time.Duration(response.CooldownSeconds) * time.Second).UnixMilli()
	return &value
}

func (s *Service) resolvePrimaryCaptcha(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount string, requestContext *loginfacade.RequestContext) (*loginfacade.Captcha, error) {
	challenge, err := s.startOrReusePrimaryChallenge(ctx, snapshot, userAccount, requestContext)
	if err != nil {
		return nil, err
	}
	step := findStep(challenge, "IMAGE_CAPTCHA")
	if step == nil {
		return nil, apperrors.Operation("图形验证码步骤不可用")
	}
	return &loginfacade.Captcha{
		ChallengeIdentifier: challenge.ChallengeIdentifier,
		StepIdentifier:      step.StepIdentifier,
		ImageBase64:         stringValue(step.UserInterfaceHints["codeImage"]),
	}, nil
}

func (s *Service) resolveRegisterCaptcha(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount string, requestContext *loginfacade.RequestContext) (*loginfacade.Captcha, error) {
	challenge, err := s.refreshOrStartRegisterCaptchaChallenge(ctx, snapshot, userAccount, requestContext)
	if err != nil {
		return nil, err
	}
	step := findStep(challenge, "IMAGE_CAPTCHA")
	if step == nil {
		return nil, apperrors.Operation("图形验证码步骤不可用")
	}
	return &loginfacade.Captcha{
		ChallengeIdentifier: challenge.ChallengeIdentifier,
		StepIdentifier:      step.StepIdentifier,
		ImageBase64:         stringValue(step.UserInterfaceHints["codeImage"]),
	}, nil
}

func (s *Service) verifyPrimaryCaptcha(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount, captchaCode string, requestContext *loginfacade.RequestContext) (bool, *loginfacade.Captcha, error) {
	challenge, err := s.startOrReusePrimaryChallenge(ctx, snapshot, userAccount, requestContext)
	if err != nil {
		return false, nil, err
	}
	stepIdentifier := findStepIdentifier(challenge, "IMAGE_CAPTCHA")
	if stepIdentifier == "" {
		return false, nil, apperrors.Operation("图形验证码步骤不可用")
	}
	response, err := s.challengeClient.Respond(ctx, challenge.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: stepIdentifier,
		Payload: map[string]any{
			"captchaCode": strings.TrimSpace(captchaCode),
		},
	})
	if err != nil {
		return false, nil, err
	}
	if strings.TrimSpace(response.ChallengeState) == "PASSED" {
		if err := s.clearCaptchaFailureState(ctx, userAccount); err != nil {
			return false, nil, err
		}
		snapshot.PrimaryChallengeSatisfied = true
		return true, nil, nil
	}
	snapshot.PrimaryChallengeSatisfied = false
	if err := s.recordCaptchaFailure(ctx, userAccount, requestContext); err != nil {
		return false, nil, err
	}
	captcha, err := s.refreshPrimaryCaptcha(ctx, snapshot, userAccount, requestContext)
	return false, captcha, err
}

func (s *Service) verifyRegisterCaptcha(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount, captchaCode string, requestContext *loginfacade.RequestContext) (bool, *loginfacade.Captcha, error) {
	challenge, err := s.startOrReuseRegisterCaptchaChallenge(ctx, snapshot, userAccount, requestContext)
	if err != nil {
		return false, nil, err
	}
	stepIdentifier := findStepIdentifier(challenge, "IMAGE_CAPTCHA")
	if stepIdentifier == "" {
		return false, nil, apperrors.Operation("图形验证码步骤不可用")
	}
	response, err := s.challengeClient.Respond(ctx, challenge.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: stepIdentifier,
		Payload: map[string]any{
			"captchaCode": strings.TrimSpace(captchaCode),
		},
	})
	if err != nil {
		return false, nil, err
	}
	if strings.TrimSpace(response.ChallengeState) == "PASSED" {
		snapshot.RegisterCaptchaSatisfied = true
		return true, nil, nil
	}
	snapshot.RegisterCaptchaSatisfied = false
	captcha, err := s.resolveRegisterCaptcha(ctx, snapshot, userAccount, requestContext)
	return false, captcha, err
}

func (s *Service) startOrReusePrimaryChallenge(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount string, requestContext *loginfacade.RequestContext) (*challengefacade.StartChallengeResponse, error) {
	if strings.TrimSpace(snapshot.PrimaryChallengeIdentifier) != "" {
		return s.challengeClient.GetChallenge(ctx, snapshot.PrimaryChallengeIdentifier)
	}
	if strings.TrimSpace(snapshot.PrimaryChallengeFlowNonce) == "" {
		snapshot.PrimaryChallengeFlowNonce = uuid.NewString()
	}
	response, err := s.challengeInt.StartChallenge(ctx, challengefacade.StartChallengeRequest{
		IssuingServiceName:         "login-app",
		AudienceServiceNames:       []string{"login-app"},
		BusinessAction:             "LOGIN",
		SubjectIdentifier:          "login:" + userAccount,
		SubjectHint:                &challengefacade.SubjectHint{SubjectType: "ANONYMOUS", SubjectValue: "login:" + userAccount},
		FlowNonce:                  snapshot.PrimaryChallengeFlowNonce,
		RequestedTimeToLiveSeconds: int(s.interactionTTL / time.Second),
		IdempotencyKey:             "LOGIN_PRIMARY|" + snapshot.LoginTransactionID + "|" + snapshot.PrimaryChallengeFlowNonce,
		RiskContext:                toRiskContext(requestContext),
		ExpectedChallengeTypes:     []string{"IMAGE_CAPTCHA"},
		ExtensionContext: map[string]any{
			"operationBinding": "register:" + snapshot.LoginTransactionID,
		},
	})
	if err != nil {
		return nil, err
	}
	snapshot.PrimaryChallengeIdentifier = response.ChallengeIdentifier
	return response, nil
}

func (s *Service) refreshPrimaryCaptcha(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount string, requestContext *loginfacade.RequestContext) (*loginfacade.Captcha, error) {
	challenge, err := s.refreshOrStartPrimaryChallenge(ctx, snapshot, userAccount, requestContext)
	if err != nil {
		return nil, err
	}
	step := findStep(challenge, "IMAGE_CAPTCHA")
	if step == nil {
		return nil, apperrors.Operation("图形验证码步骤不可用")
	}
	return &loginfacade.Captcha{
		ChallengeIdentifier: challenge.ChallengeIdentifier,
		StepIdentifier:      step.StepIdentifier,
		ImageBase64:         stringValue(step.UserInterfaceHints["codeImage"]),
	}, nil
}

func (s *Service) refreshOrStartPrimaryChallenge(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount string, requestContext *loginfacade.RequestContext) (*challengefacade.StartChallengeResponse, error) {
	if strings.TrimSpace(snapshot.PrimaryChallengeIdentifier) != "" {
		challenge, err := s.challengeClient.GetChallenge(ctx, snapshot.PrimaryChallengeIdentifier)
		if err == nil {
			stepIdentifier := findStepIdentifier(challenge, "IMAGE_CAPTCHA")
			if stepIdentifier != "" {
				refreshed, refreshErr := s.challengeClient.Refresh(ctx, snapshot.PrimaryChallengeIdentifier, challengefacade.RefreshChallengeRequest{StepIdentifier: stepIdentifier})
				if refreshErr == nil {
					snapshot.PrimaryChallengeSatisfied = false
					return refreshed, nil
				}
			}
		}
		snapshot.PrimaryChallengeIdentifier = ""
		snapshot.PrimaryChallengeSatisfied = false
	}
	return s.startOrReusePrimaryChallenge(ctx, snapshot, userAccount, requestContext)
}

func (s *Service) startOrReuseRegisterCaptchaChallenge(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount string, requestContext *loginfacade.RequestContext) (*challengefacade.StartChallengeResponse, error) {
	if strings.TrimSpace(snapshot.RegisterCaptchaIdentifier) != "" {
		return s.challengeClient.GetChallenge(ctx, snapshot.RegisterCaptchaIdentifier)
	}
	if strings.TrimSpace(snapshot.RegisterCaptchaFlowNonce) == "" {
		snapshot.RegisterCaptchaFlowNonce = uuid.NewString()
	}
	response, err := s.challengeInt.StartChallenge(ctx, challengefacade.StartChallengeRequest{
		IssuingServiceName:         "login-app",
		AudienceServiceNames:       []string{"login-app"},
		BusinessAction:             "REGISTER_ACCOUNT",
		SubjectIdentifier:          "register:" + userAccount,
		SubjectHint:                &challengefacade.SubjectHint{SubjectType: "ANONYMOUS", SubjectValue: "register:" + userAccount},
		FlowNonce:                  snapshot.RegisterCaptchaFlowNonce,
		RequestedTimeToLiveSeconds: int(s.interactionTTL / time.Second),
		IdempotencyKey:             "REGISTER_FORM_CAPTCHA|" + snapshot.LoginTransactionID + "|" + snapshot.RegisterCaptchaFlowNonce,
		RiskContext:                toRiskContext(requestContext),
		ExpectedChallengeTypes:     []string{"IMAGE_CAPTCHA"},
		ExtensionContext: map[string]any{
			"operationBinding": "register:" + snapshot.LoginTransactionID,
		},
	})
	if err != nil {
		return nil, err
	}
	snapshot.RegisterCaptchaIdentifier = response.ChallengeIdentifier
	return response, nil
}

func (s *Service) refreshOrStartRegisterCaptchaChallenge(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount string, requestContext *loginfacade.RequestContext) (*challengefacade.StartChallengeResponse, error) {
	if strings.TrimSpace(snapshot.RegisterCaptchaIdentifier) != "" {
		challenge, err := s.challengeClient.GetChallenge(ctx, snapshot.RegisterCaptchaIdentifier)
		if err == nil {
			stepIdentifier := findStepIdentifier(challenge, "IMAGE_CAPTCHA")
			if stepIdentifier != "" {
				refreshed, refreshErr := s.challengeClient.Refresh(ctx, snapshot.RegisterCaptchaIdentifier, challengefacade.RefreshChallengeRequest{StepIdentifier: stepIdentifier})
				if refreshErr == nil {
					snapshot.RegisterCaptchaSatisfied = false
					return refreshed, nil
				}
			}
		}
		snapshot.RegisterCaptchaIdentifier = ""
		snapshot.RegisterCaptchaSatisfied = false
	}
	return s.startOrReuseRegisterCaptchaChallenge(ctx, snapshot, userAccount, requestContext)
}

func (s *Service) startOrRefreshRegisterEmailChallenge(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount, userEmail string, requestContext *loginfacade.RequestContext) (*challengefacade.StartChallengeResponse, error) {
	if strings.TrimSpace(snapshot.RegisterEmailFlowNonce) == "" || strings.TrimSpace(snapshot.RegisterEmail) != userEmail {
		snapshot.RegisterEmailFlowNonce = uuid.NewString()
		snapshot.RegisterEmailIdentifier = ""
		snapshot.RegisterEmailSatisfied = false
	}
	if strings.TrimSpace(snapshot.RegisterEmailIdentifier) != "" {
		challenge, err := s.challengeClient.GetChallenge(ctx, snapshot.RegisterEmailIdentifier)
		if err == nil {
			stepIdentifier := findStepIdentifier(challenge, "EMAIL_ONE_TIME_PASSWORD")
			if stepIdentifier != "" {
				return s.challengeClient.Refresh(ctx, snapshot.RegisterEmailIdentifier, challengefacade.RefreshChallengeRequest{StepIdentifier: stepIdentifier})
			}
		}
		snapshot.RegisterEmailIdentifier = ""
	}
	response, err := s.challengeInt.StartChallenge(ctx, challengefacade.StartChallengeRequest{
		IssuingServiceName:         "login-app",
		AudienceServiceNames:       []string{"login-app"},
		BusinessAction:             "REGISTER_ACCOUNT",
		SubjectIdentifier:          "register:" + userAccount,
		SubjectHint:                &challengefacade.SubjectHint{SubjectType: "ANONYMOUS", SubjectValue: "register:" + userAccount},
		FlowNonce:                  snapshot.RegisterEmailFlowNonce,
		RequestedTimeToLiveSeconds: int(registerEmailCodeTTL / time.Second),
		IdempotencyKey:             "REGISTER_FORM_EMAIL|" + snapshot.LoginTransactionID + "|" + snapshot.RegisterEmailFlowNonce,
		RiskContext:                toRiskContext(requestContext),
		ExpectedChallengeTypes:     []string{"EMAIL_ONE_TIME_PASSWORD"},
		ExtensionContext: map[string]any{
			"operationBinding": "register:" + snapshot.LoginTransactionID,
			"targetEmail":      userEmail,
		},
	})
	if err != nil {
		return nil, err
	}
	snapshot.RegisterEmailIdentifier = response.ChallengeIdentifier
	return response, nil
}

func (s *Service) verifyRegisterEmailCode(ctx context.Context, snapshot *domain.InteractionSnapshot, userAccount, userEmail, emailCode string) (bool, error) {
	_ = userAccount
	if !otpPattern.MatchString(strings.TrimSpace(emailCode)) {
		return false, nil
	}
	challengeIdentifier := strings.TrimSpace(snapshot.RegisterEmailIdentifier)
	if challengeIdentifier == "" || strings.TrimSpace(snapshot.RegisterEmail) != userEmail {
		return false, nil
	}
	challenge, err := s.challengeClient.GetChallenge(ctx, challengeIdentifier)
	if err != nil {
		return false, err
	}
	stepIdentifier := findStepIdentifier(challenge, "EMAIL_ONE_TIME_PASSWORD")
	if stepIdentifier == "" {
		return false, apperrors.Operation("邮箱验证码步骤不可用")
	}
	response, err := s.challengeClient.Respond(ctx, challengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: stepIdentifier,
		Payload: map[string]any{
			"emailCode": strings.TrimSpace(emailCode),
		},
	})
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(response.ChallengeState) == "PASSED" {
		snapshot.RegisterEmailSatisfied = true
		return true, nil
	}
	snapshot.RegisterEmailSatisfied = false
	return false, nil
}

func (s *Service) startOrReuseTotpChallenge(ctx context.Context, snapshot *domain.InteractionSnapshot, requestContext *loginfacade.RequestContext) (*challengefacade.StartChallengeResponse, error) {
	if strings.TrimSpace(snapshot.ChallengeIdentifier) != "" {
		if existing, err := s.challengeClient.GetChallenge(ctx, snapshot.ChallengeIdentifier); err == nil && findStep(existing, "TIME_BASED_ONE_TIME_PASSWORD") != nil {
			return existing, nil
		}
		snapshot.ChallengeIdentifier = ""
	}
	if strings.TrimSpace(snapshot.FlowNonce) == "" {
		snapshot.FlowNonce = uuid.NewString()
	}
	response, err := s.challengeInt.StartChallenge(ctx, challengefacade.StartChallengeRequest{
		IssuingServiceName:         "login-app",
		AudienceServiceNames:       []string{"login-app"},
		BusinessAction:             "LOGIN",
		SubjectIdentifier:          "user:" + int64ToString(snapshot.UserID),
		SubjectHint:                &challengefacade.SubjectHint{SubjectType: "USER", SubjectValue: "user:" + int64ToString(snapshot.UserID)},
		FlowNonce:                  snapshot.FlowNonce,
		RequestedTimeToLiveSeconds: int(s.interactionTTL / time.Second),
		IdempotencyKey:             "LOGIN_TOTP|" + snapshot.LoginTransactionID + "|" + snapshot.FlowNonce,
		RiskContext:                toRiskContext(requestContext),
		ExpectedChallengeTypes:     []string{"TIME_BASED_ONE_TIME_PASSWORD"},
	})
	if err != nil {
		return nil, err
	}
	snapshot.ChallengeIdentifier = response.ChallengeIdentifier
	if err := s.saveInteraction(ctx, snapshot); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) startOrReusePasskeyChallenge(ctx context.Context, snapshot *domain.InteractionSnapshot, requestContext *loginfacade.RequestContext) (*challengefacade.StartChallengeResponse, error) {
	if strings.TrimSpace(snapshot.ChallengeIdentifier) != "" {
		if existing, err := s.challengeClient.GetChallenge(ctx, snapshot.ChallengeIdentifier); err == nil && findStep(existing, "WEBAUTHN_PASSKEY_ASSERTION") != nil {
			return existing, nil
		}
		snapshot.ChallengeIdentifier = ""
	}
	if strings.TrimSpace(snapshot.FlowNonce) == "" {
		snapshot.FlowNonce = uuid.NewString()
	}
	response, err := s.challengeInt.StartChallenge(ctx, challengefacade.StartChallengeRequest{
		IssuingServiceName:         "login-app",
		AudienceServiceNames:       []string{"login-app"},
		BusinessAction:             "LOGIN",
		SubjectIdentifier:          "user:" + int64ToString(snapshot.UserID),
		SubjectHint:                &challengefacade.SubjectHint{SubjectType: "USER", SubjectValue: "user:" + int64ToString(snapshot.UserID)},
		FlowNonce:                  snapshot.FlowNonce,
		RequestedTimeToLiveSeconds: int(s.interactionTTL / time.Second),
		IdempotencyKey:             "LOGIN_PASSKEY|" + snapshot.LoginTransactionID + "|" + snapshot.FlowNonce,
		RiskContext:                toRiskContext(requestContext),
		ExpectedChallengeTypes:     []string{"WEBAUTHN_PASSKEY_ASSERTION"},
	})
	if err != nil {
		return nil, err
	}
	snapshot.ChallengeIdentifier = response.ChallengeIdentifier
	if err := s.saveInteraction(ctx, snapshot); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) prepareInteraction(ctx context.Context, loginTransactionID, loginContextID, userAccount string, requestContext *loginfacade.RequestContext, methodType, providerCode string) (string, *domain.InteractionSnapshot, error) {
	loginTransactionID = strings.TrimSpace(loginTransactionID)
	loginContextID = strings.TrimSpace(loginContextID)
	userAccount = strings.TrimSpace(userAccount)
	if loginTransactionID == "" || userAccount == "" {
		return "", nil, apperrors.Params("缺少 loginTransactionId 或 userAccount")
	}
	snapshot, err := s.interactions.GetInteraction(ctx, loginTransactionID)
	if err != nil {
		return "", nil, err
	}
	if snapshot == nil {
		completed, completedErr := s.interactions.IsCompleted(ctx, loginTransactionID)
		if completedErr != nil {
			return "", nil, completedErr
		}
		if completed {
			return "", nil, apperrors.Operation("登录事务已完成或已失效")
		}
		now := time.Now().UTC()
		expiresAt := now.Add(s.interactionTTL)
		snapshot = &domain.InteractionSnapshot{
			LoginTransactionID: loginTransactionID,
			LoginContextID:     loginContextID,
			UserAccount:        userAccount,
			CreatedAt:          &now,
			ExpiresAt:          &expiresAt,
		}
	} else {
		if existingAccount := strings.TrimSpace(snapshot.UserAccount); existingAccount != "" && existingAccount != userAccount {
			return "", nil, apperrors.Operation("登录事务已绑定其他账号")
		}
		snapshot.UserAccount = userAccount
	}
	if err := s.attachPlatformContext(ctx, snapshot, loginContextID, requestContext, methodType, providerCode); err != nil {
		return "", nil, err
	}
	return userAccount, snapshot, nil
}

func (s *Service) prepareRegisterInteraction(ctx context.Context, loginTransactionID, loginContextID, userAccount string, requestContext *loginfacade.RequestContext) (string, *domain.InteractionSnapshot, error) {
	loginTransactionID = strings.TrimSpace(loginTransactionID)
	loginContextID = strings.TrimSpace(loginContextID)
	userAccount = strings.TrimSpace(userAccount)
	if loginTransactionID == "" || loginContextID == "" || userAccount == "" {
		return "", nil, apperrors.Params("缺少 loginTransactionId、loginContextId 或 userAccount")
	}
	snapshot, err := s.interactions.GetInteraction(ctx, loginTransactionID)
	if err != nil {
		return "", nil, err
	}
	if snapshot == nil {
		completed, completedErr := s.interactions.IsCompleted(ctx, loginTransactionID)
		if completedErr != nil {
			return "", nil, completedErr
		}
		if completed {
			return "", nil, apperrors.Operation("登录事务已完成或已失效")
		}
		now := time.Now().UTC()
		expiresAt := now.Add(s.interactionTTL)
		snapshot = &domain.InteractionSnapshot{
			LoginTransactionID: loginTransactionID,
			LoginContextID:     loginContextID,
			CreatedAt:          &now,
			ExpiresAt:          &expiresAt,
		}
	}
	if existingAccount := strings.TrimSpace(snapshot.RegisterAccount); existingAccount != "" && existingAccount != userAccount {
		snapshot.RegisterCaptchaIdentifier = ""
		snapshot.RegisterCaptchaFlowNonce = ""
		snapshot.RegisterCaptchaSatisfied = false
		snapshot.RegisterEmail = ""
		snapshot.RegisterEmailIdentifier = ""
		snapshot.RegisterEmailFlowNonce = ""
		snapshot.RegisterEmailSatisfied = false
	}
	snapshot.RegisterAccount = userAccount
	if err := s.attachPlatformContext(ctx, snapshot, loginContextID, requestContext, "", ""); err != nil {
		return "", nil, err
	}
	return userAccount, snapshot, nil
}

func (s *Service) requireFormRegistrationPolicy(ctx context.Context, snapshot *domain.InteractionSnapshot, requestContext *loginfacade.RequestContext) (*platformfacade.ProvisioningPolicy, error) {
	if s.platform == nil || snapshot == nil {
		return nil, apperrors.Forbidden("当前平台不允许注册")
	}
	validation, err := s.platform.ValidateLoginContext(ctx, snapshot.LoginContextID, platformfacade.ResolvePlatformRequest{
		LoginTransactionID: snapshot.LoginTransactionID,
		LoginContextID:     snapshot.LoginContextID,
		TrustedSource:      toPlatformTrustedSource(requestContext),
	})
	if err != nil {
		return nil, err
	}
	if validation == nil || strings.TrimSpace(validation.PlatformCode) == "" {
		return nil, apperrors.Forbidden("登录上下文无效，请重新登录")
	}
	policy, err := s.platform.GetFormRegistrationPolicy(ctx, validation.PlatformCode)
	if err != nil {
		return nil, err
	}
	if policy == nil || !policy.AllowFormRegister {
		return nil, apperrors.Forbidden("当前平台未开放注册")
	}
	snapshot.LoginContextID = validation.LoginContextID
	snapshot.PlatformCode = validation.PlatformCode
	return policy, nil
}

func (s *Service) enforceRegisterRateLimit(ctx context.Context, platformCode, account, email string, requestContext *loginfacade.RequestContext) error {
	if s.limiter == nil {
		return apperrors.RateLimited("注册服务繁忙，请稍后再试")
	}
	keys := []struct {
		key    string
		limit  int64
		window time.Duration
	}{
		{key: "login:register:ip:" + digestRateLimitValue(requestIP(requestContext)), limit: registerIPLimit, window: registerIPWindow},
		{key: "login:register:platform-ip:" + strings.ToLower(strings.TrimSpace(platformCode)) + ":" + digestRateLimitValue(requestIP(requestContext)), limit: registerPlatformLimit, window: registerPlatformWindow},
		{key: "login:register:account:" + digestRateLimitValue(account), limit: registerSubjectLimit, window: registerSubjectWindow},
		{key: "login:register:email:" + digestRateLimitValue(strings.ToLower(strings.TrimSpace(email))), limit: registerSubjectLimit, window: registerSubjectWindow},
	}
	if requestContext != nil && strings.TrimSpace(requestContext.DeviceID) != "" {
		keys = append(keys, struct {
			key    string
			limit  int64
			window time.Duration
		}{key: "login:register:device:" + digestRateLimitValue(requestContext.DeviceID), limit: registerDeviceLimit, window: registerDeviceWindow})
	}
	for _, item := range keys {
		decision, err := allowRegisterLimit(ctx, s.limiter, item.key, item.limit, item.window)
		if err != nil {
			if errors.Is(err, limiterinfra.ErrRateLimited) || !decision.Allowed {
				return apperrors.RateLimited("注册请求过于频繁，请稍后再试").WithDetails(map[string]any{
					"retryAfterSeconds": int(decision.RetryAfter.Seconds()),
				})
			}
			return apperrors.RateLimited("注册服务繁忙，请稍后再试")
		}
		if !decision.Allowed {
			return apperrors.RateLimited("注册请求过于频繁，请稍后再试").WithDetails(map[string]any{
				"retryAfterSeconds": int(decision.RetryAfter.Seconds()),
			})
		}
	}
	return nil
}

func (s *Service) enforceRegisterEmailCodeRateLimit(ctx context.Context, platformCode, account, email string, requestContext *loginfacade.RequestContext) error {
	if s.limiter == nil {
		return apperrors.RateLimited("注册服务繁忙，请稍后再试")
	}
	keys := []struct {
		key    string
		limit  int64
		window time.Duration
	}{
		{key: "login:register-email:ip:" + digestRateLimitValue(requestIP(requestContext)), limit: registerEmailSendIPLimit, window: registerIPWindow},
		{key: "login:register-email:platform-ip:" + strings.ToLower(strings.TrimSpace(platformCode)) + ":" + digestRateLimitValue(requestIP(requestContext)), limit: registerEmailSendPlatformLimit, window: registerPlatformWindow},
		{key: "login:register-email:account:" + digestRateLimitValue(account), limit: registerEmailSendSubjectLimit, window: registerSubjectWindow},
		{key: "login:register-email:email:" + digestRateLimitValue(strings.ToLower(strings.TrimSpace(email))), limit: registerEmailSendSubjectLimit, window: registerSubjectWindow},
	}
	if requestContext != nil && strings.TrimSpace(requestContext.DeviceID) != "" {
		keys = append(keys, struct {
			key    string
			limit  int64
			window time.Duration
		}{key: "login:register-email:device:" + digestRateLimitValue(requestContext.DeviceID), limit: registerEmailSendDeviceLimit, window: registerDeviceWindow})
	}
	for _, item := range keys {
		decision, err := allowRegisterLimit(ctx, s.limiter, item.key, item.limit, item.window)
		if err != nil {
			if errors.Is(err, limiterinfra.ErrRateLimited) || !decision.Allowed {
				return apperrors.RateLimited("验证码发送过于频繁，请稍后再试").WithDetails(map[string]any{
					"retryAfterSeconds": int(decision.RetryAfter.Seconds()),
				})
			}
			return apperrors.RateLimited("注册服务繁忙，请稍后再试")
		}
		if !decision.Allowed {
			return apperrors.RateLimited("验证码发送过于频繁，请稍后再试").WithDetails(map[string]any{
				"retryAfterSeconds": int(decision.RetryAfter.Seconds()),
			})
		}
	}
	return nil
}

func normalizeRegisterEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", apperrors.Params("请输入邮箱")
	}
	address, err := mail.ParseAddress(normalized)
	if err != nil || strings.TrimSpace(address.Address) != normalized {
		return "", apperrors.Params("请输入有效邮箱")
	}
	return normalized, nil
}

func registerEmailMasked(challenge *challengefacade.StartChallengeResponse) string {
	step := findStep(challenge, "EMAIL_ONE_TIME_PASSWORD")
	if step == nil {
		return ""
	}
	return stringValue(step.UserInterfaceHints["emailMasked"])
}

func allowRegisterLimit(ctx context.Context, limiter limiterinfra.Limiter, key string, limit int64, window time.Duration) (limiterinfra.Decision, error) {
	if override, ok := limiter.(limiterinfra.FailOpenOverrideLimiter); ok {
		return override.AllowWithFailOpen(ctx, key, limit, window, false)
	}
	return limiter.Allow(ctx, key, limit, window)
}

func requestIP(requestContext *loginfacade.RequestContext) string {
	if requestContext == nil {
		return "unknown"
	}
	return strings.TrimSpace(requestContext.LoginIP)
}

func digestRateLimitValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "anonymous"
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) attachPlatformContext(ctx context.Context, snapshot *domain.InteractionSnapshot, loginContextID string, requestContext *loginfacade.RequestContext, methodType, providerCode string) error {
	if s.platform == nil || snapshot == nil {
		return nil
	}
	loginContextID = strings.TrimSpace(loginContextID)
	if loginContextID == "" {
		loginContextID = strings.TrimSpace(snapshot.LoginContextID)
	}
	if loginContextID != "" && loginContextID != strings.TrimSpace(snapshot.LoginContextID) {
		validation, err := s.platform.ValidateLoginContext(ctx, loginContextID, platformfacade.ResolvePlatformRequest{
			LoginTransactionID: snapshot.LoginTransactionID,
			LoginContextID:     loginContextID,
			TrustedSource:      toPlatformTrustedSource(requestContext),
		})
		if err != nil {
			return err
		}
		if validation == nil || strings.TrimSpace(validation.PlatformCode) == "" {
			return apperrors.Forbidden("登录上下文无效，请重新登录")
		}
		if strings.TrimSpace(snapshot.PlatformCode) != "" && !strings.EqualFold(snapshot.PlatformCode, validation.PlatformCode) {
			return apperrors.Forbidden("登录上下文平台不一致")
		}
		snapshot.LoginContextID = validation.LoginContextID
		snapshot.PlatformCode = validation.PlatformCode
	}
	if strings.TrimSpace(snapshot.PlatformCode) == "" {
		if loginContextID == "" {
			return apperrors.Forbidden("登录上下文缺失，请重新登录")
		}
		validation, err := s.platform.ValidateLoginContext(ctx, loginContextID, platformfacade.ResolvePlatformRequest{
			LoginTransactionID: snapshot.LoginTransactionID,
			LoginContextID:     loginContextID,
			TrustedSource:      toPlatformTrustedSource(requestContext),
		})
		if err != nil {
			return err
		}
		if validation == nil || strings.TrimSpace(validation.PlatformCode) == "" {
			return apperrors.Forbidden("登录上下文无效，请重新登录")
		}
		snapshot.LoginContextID = validation.LoginContextID
		snapshot.PlatformCode = validation.PlatformCode
	}
	if strings.TrimSpace(methodType) != "" {
		if err := s.platform.RequireLoginMethod(ctx, snapshot.PlatformCode, methodType, providerCode); err != nil {
			return err
		}
	}
	return nil
}

func toPlatformTrustedSource(input *loginfacade.RequestContext) platformfacade.TrustedSource {
	if input == nil {
		return platformfacade.TrustedSource{}
	}
	return platformfacade.TrustedSource{
		Host:    strings.TrimSpace(input.Host),
		Origin:  strings.TrimSpace(input.Origin),
		Referer: strings.TrimSpace(input.Referer),
	}
}

func (s *Service) saveInteraction(ctx context.Context, snapshot *domain.InteractionSnapshot) error {
	return s.interactions.SaveInteraction(ctx, snapshot, s.interactionTTL)
}

func (s *Service) findSubjectByAccount(ctx context.Context, userAccount string) (*userfacade.SubjectRecord, error) {
	if s.subjects == nil {
		return nil, nil
	}
	return s.subjects.FindSubjectByAccount(ctx, userAccount)
}

func findStep(response *challengefacade.StartChallengeResponse, challengeType string) *challengefacade.ChallengeStepVO {
	if response == nil {
		return nil
	}
	for idx := range response.Steps {
		step := &response.Steps[idx]
		if step.ChallengeType == challengeType {
			return step
		}
	}
	return nil
}

func findStepIdentifier(response *challengefacade.StartChallengeResponse, challengeType string) string {
	step := findStep(response, challengeType)
	if step == nil {
		return ""
	}
	return step.StepIdentifier
}

func toRiskContext(requestContext *loginfacade.RequestContext) *challengefacade.RiskContext {
	if requestContext == nil {
		return nil
	}
	return &challengefacade.RiskContext{
		IPAddress:        requestContext.LoginIP,
		UserAgent:        requestContext.UserAgent,
		DeviceIdentifier: requestContext.DeviceID,
		TenantIdentifier: requestContext.TenantID,
	}
}

func int64ToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

type loginSuccess struct {
	RedirectURL              string
	AccessToken              string
	TokenType                string
	AccessTTLSeconds         int64
	SessionCookieHeaderValue string
	RefreshCookieHeaderValue string
}

func (s *Service) completeLogin(ctx context.Context, snapshot *domain.InteractionSnapshot, requestContext *loginfacade.RequestContext, amr []string) (*loginSuccess, error) {
	if s.authSessions != nil && s.authCompletion != nil {
		if authSession, err := s.authSessions.GetAuthorizationSession(ctx, snapshot.LoginTransactionID); err != nil {
			return nil, err
		} else if authSession != nil {
			acr := "LEVEL_1"
			if len(amr) > 1 || slices.Contains(amr, "otp") || slices.Contains(amr, "hwk") {
				acr = "LEVEL_2"
			}
			result, err := s.authCompletion.CompleteInteractiveAuthentication(ctx, ssofacade.CompleteInteractiveAuthenticationCommand{
				LoginTransactionID: snapshot.LoginTransactionID,
				UserID:             snapshot.UserID,
				PlatformCode:       snapshot.PlatformCode,
				ACR:                acr,
				AMR:                amr,
				AuthTime:           pointerTime(time.Now().UTC()),
				RequestContext:     toSSORequestContext(requestContext),
			})
			if err != nil {
				return nil, err
			}
			return &loginSuccess{
				RedirectURL:              result.RedirectURL,
				SessionCookieHeaderValue: result.SessionCookieHeaderValue,
			}, nil
		}
	}
	if s.bootstrap == nil {
		return &loginSuccess{}, nil
	}
	result, err := s.bootstrap.BootstrapFirstPartySession(ctx, ssofacade.BootstrapSessionCommand{
		UserID:         snapshot.UserID,
		PlatformCode:   snapshot.PlatformCode,
		ACR:            chooseACR(amr),
		AMR:            amr,
		RequestContext: toSSORequestContext(requestContext),
	})
	if err != nil {
		return nil, err
	}
	return &loginSuccess{
		AccessToken:              result.AccessToken,
		TokenType:                result.TokenType,
		AccessTTLSeconds:         result.AccessTTLSeconds,
		SessionCookieHeaderValue: result.SessionCookieHeaderValue,
		RefreshCookieHeaderValue: result.RefreshCookieHeaderValue,
	}, nil
}

func toSSORequestContext(input *loginfacade.RequestContext) *ssofacade.RequestContext {
	if input == nil {
		return nil
	}
	return &ssofacade.RequestContext{
		DeviceID:  strings.TrimSpace(input.DeviceID),
		TenantID:  strings.TrimSpace(input.TenantID),
		LoginIP:   strings.TrimSpace(input.LoginIP),
		UserAgent: strings.TrimSpace(input.UserAgent),
		TraceID:   strings.TrimSpace(input.TraceID),
	}
}

func pointerTime(value time.Time) *time.Time {
	return &value
}

func chooseACR(amr []string) string {
	if len(amr) > 1 || slices.Contains(amr, "otp") || slices.Contains(amr, "hwk") {
		return "LEVEL_2"
	}
	return "LEVEL_1"
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}
