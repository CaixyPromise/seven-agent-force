package application

import (
	"context"
	"strings"
	"testing"
	"time"

	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	logindomain "github.com/CaixyPromise/seven-agent/force/internal/app/login/domain"
	loginfacade "github.com/CaixyPromise/seven-agent/force/internal/app/login/facade"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	limiterinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/limiter"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestSubmitPasswordAuthenticatesWithoutSecondFactor(t *testing.T) {
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	hash, err := passwordService.Hash(context.Background(), "secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	service := NewService(
		passwordService,
		&fakeCredentialFacade{password: &credentialfacade.PasswordCredential{UserID: 1001, PasswordHash: hash}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		nil,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	result, err := service.SubmitPassword(context.Background(), loginfacade.PasswordSubmitRequest{
		LoginTransactionID: "txn-1",
		UserAccount:        "admin",
		Password:           "secret123",
	})
	if err != nil {
		t.Fatalf("submit password: %v", err)
	}
	if !result.Authenticated || result.TotpRequired {
		t.Fatalf("expected authenticated=true totpRequired=false, got %#v", result)
	}
}

func TestSubmitPasswordRejectsPendingReviewAccountWithExplicitMessage(t *testing.T) {
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	hash, err := passwordService.Hash(context.Background(), "secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	failures := newFakeLoginFailureFacade()
	service := NewService(
		passwordService,
		&fakeCredentialFacade{password: &credentialfacade.PasswordCredential{UserID: 1001, PasswordHash: hash}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"pending": {UserID: 1001, AccountName: "pending", Status: userfacade.UserStatusPendingReview, Enabled: false}}},
		nil,
		nil,
		nil,
		failures,
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	_, err = service.SubmitPassword(context.Background(), loginfacade.PasswordSubmitRequest{
		LoginTransactionID: "txn-pending",
		UserAccount:        "pending",
		Password:           "secret123",
	})
	if err == nil || apperrors.From(err).Message() != "账号正在审核中，请等待管理员审核通过" {
		t.Fatalf("expected pending review login rejection, got %v", err)
	}
	if failures.failures["pending"] != 0 {
		t.Fatalf("correct password for pending account must not record login failure, got %d", failures.failures["pending"])
	}
}

func TestSubmitPasswordPersistsPlatformProvenance(t *testing.T) {
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	hash, err := passwordService.Hash(context.Background(), "secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	authSessions := &fakeAuthorizationSessions{
		snapshot: &ssofacade.AuthorizationSessionSnapshot{LoginTransactionID: "txn-platform", ClientID: "client-a"},
	}
	completion := &fakeAuthenticationCompletion{}
	platform := &fakePlatformFacade{platformCode: "seven-admin"}
	service := NewService(
		passwordService,
		&fakeCredentialFacade{password: &credentialfacade.PasswordCredential{UserID: 1001, PasswordHash: hash}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		nil,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	service.BindPlatform(platform)
	service.BindSSO(authSessions, completion, nil)

	result, err := service.SubmitPassword(context.Background(), loginfacade.PasswordSubmitRequest{
		LoginTransactionID: "txn-platform",
		LoginContextID:     "plctx-1",
		UserAccount:        "admin",
		Password:           "secret123",
		RequestContext:     &loginfacade.RequestContext{Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("submit password: %v", err)
	}
	if result == nil || !result.Authenticated {
		t.Fatalf("expected authenticated result, got %#v", result)
	}
	if platform.requiredMethod != "PASSWORD" {
		t.Fatalf("expected PASSWORD method gate, got %q", platform.requiredMethod)
	}
	if completion.command.PlatformCode != "seven-admin" {
		t.Fatalf("expected SSO completion platform seven-admin, got %#v", completion.command)
	}
}

func TestGetRegisterStateRejectsWhenFormRegisterDisabled(t *testing.T) {
	challenges := &fakeChallengeInternalFacade{response: registerCaptchaChallenge()}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{},
		challenges,
		&fakeChallengeClientFacade{},
		nil,
		newFakeLoginFailureFacade(),
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	service.BindPlatform(&fakePlatformFacade{
		platformCode: "seven-admin",
		formPolicy:   &platformfacade.ProvisioningPolicy{PlatformCode: "seven-admin", AllowFormRegister: false},
	})

	_, err := service.GetRegisterState(context.Background(), loginfacade.RegisterStateRequest{
		LoginTransactionID: "txn-register-disabled",
		LoginContextID:     "plctx-disabled",
		UserAccount:        "newuser",
		RequestContext:     &loginfacade.RequestContext{Host: "127.0.0.1:5291"},
	})
	if err == nil || apperrors.From(err).Kind() != apperrors.KindForbidden {
		t.Fatalf("expected forbidden register state error, got %v", err)
	}
	if challenges.startCalls != 0 {
		t.Fatalf("disabled form register must not start captcha challenge, got %d", challenges.startCalls)
	}
}

func TestSendRegisterEmailCodeRejectsWrongCaptchaWithoutSendingEmail(t *testing.T) {
	captchaChallenge := registerCaptchaChallenge()
	subjects := &fakeSubjectLookupFacade{}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		subjects,
		&fakeChallengeInternalFacade{response: captchaChallenge},
		&fakeChallengeClientFacade{
			getResponse:     captchaChallenge,
			respondResponse: &challengefacade.RespondChallengeResponse{ChallengeState: "FAILED"},
		},
		nil,
		newFakeLoginFailureFacade(),
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	service.BindPlatform(&fakePlatformFacade{platformCode: "seven-admin"})

	result, err := service.SendRegisterEmailCode(context.Background(), loginfacade.RegisterEmailCodeRequest{
		LoginTransactionID: "txn-register-captcha",
		LoginContextID:     "plctx-register",
		UserAccount:        "newuser",
		UserEmail:          "new@example.com",
		CaptchaCode:        "wrong",
		RequestContext:     &loginfacade.RequestContext{LoginIP: "203.0.113.10", Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("send register email code wrong captcha: %v", err)
	}
	if result == nil || result.Sent || result.Captcha == nil {
		t.Fatalf("expected rejected email code result with refreshed captcha, got %#v", result)
	}
	if subjects.createFormCalls != 0 {
		t.Fatalf("wrong captcha must not create subject, got %d calls", subjects.createFormCalls)
	}
}

func TestSubmitRegisterCreatesSubjectWithDefaultPolicy(t *testing.T) {
	emailChallenge := registerEmailChallenge()
	subjects := &fakeSubjectLookupFacade{}
	interactions := newFakeInteractionStore()
	interactions.snapshots["txn-register-ok"] = &logindomain.InteractionSnapshot{
		LoginTransactionID:      "txn-register-ok",
		LoginContextID:          "plctx-register-ok",
		PlatformCode:            "seven-admin",
		RegisterAccount:         "newuser",
		RegisterEmail:           "new@example.com",
		RegisterEmailIdentifier: "register-email-challenge",
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		subjects,
		&fakeChallengeInternalFacade{response: emailChallenge},
		&fakeChallengeClientFacade{
			getResponse:     emailChallenge,
			respondResponse: &challengefacade.RespondChallengeResponse{ChallengeState: "PASSED"},
		},
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	service.BindPlatform(&fakePlatformFacade{
		platformCode: "seven-admin",
		formPolicy: &platformfacade.ProvisioningPolicy{
			PlatformCode:      "seven-admin",
			AllowFormRegister: true,
			DefaultOrgID:      int64Pointer(11),
			DefaultDeptID:     int64Pointer(22),
			DefaultPostIDs:    []int64{33},
			DefaultRoleIDs:    []int64{44, 45},
		},
	})
	service.BindLimiter(&fakeRegisterLimiter{decision: limiterinfra.Decision{Allowed: true}})

	result, err := service.SubmitRegister(context.Background(), loginfacade.RegisterSubmitRequest{
		LoginTransactionID: "txn-register-ok",
		LoginContextID:     "plctx-register-ok",
		UserAccount:        "newuser",
		UserName:           "New User",
		UserEmail:          "new@example.com",
		Password:           "Secret123",
		ConfirmPassword:    "Secret123",
		EmailCode:          "123456",
		RequestContext:     &loginfacade.RequestContext{LoginIP: "203.0.113.11", DeviceID: "device-1", Host: "127.0.0.1:5291"},
	})
	if err != nil {
		t.Fatalf("submit register: %v", err)
	}
	if result == nil || !result.Registered || result.UserAccount != "newuser" {
		t.Fatalf("expected registered result, got %#v", result)
	}
	if subjects.createFormCalls != 1 {
		t.Fatalf("expected one form subject create, got %d", subjects.createFormCalls)
	}
	command := subjects.createdFormCommand
	if command.RegisterPlatformCode != "seven-admin" || int64Value(command.DefaultOrgID) != 11 || int64Value(command.DefaultDeptID) != 22 {
		t.Fatalf("default policy not forwarded: %#v", command)
	}
	if len(command.DefaultRoleIDs) != 2 || command.DefaultRoleIDs[0] != 44 || command.DefaultPostIDs[0] != 33 {
		t.Fatalf("default role/post policy not forwarded: %#v", command)
	}
}

func TestSubmitRegisterRateLimitedDoesNotCreateSubject(t *testing.T) {
	emailChallenge := registerEmailChallenge()
	subjects := &fakeSubjectLookupFacade{}
	interactions := newFakeInteractionStore()
	interactions.snapshots["txn-register-limited"] = &logindomain.InteractionSnapshot{
		LoginTransactionID:      "txn-register-limited",
		LoginContextID:          "plctx-register-limited",
		PlatformCode:            "seven-admin",
		RegisterAccount:         "newuser",
		RegisterEmail:           "new@example.com",
		RegisterEmailIdentifier: "register-email-challenge",
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		subjects,
		&fakeChallengeInternalFacade{response: emailChallenge},
		&fakeChallengeClientFacade{
			getResponse:     emailChallenge,
			respondResponse: &challengefacade.RespondChallengeResponse{ChallengeState: "PASSED"},
		},
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	service.BindPlatform(&fakePlatformFacade{platformCode: "seven-admin"})
	service.BindLimiter(&fakeRegisterLimiter{decision: limiterinfra.Decision{
		Allowed:    false,
		RetryAfter: 2 * time.Minute,
	}})

	_, err := service.SubmitRegister(context.Background(), loginfacade.RegisterSubmitRequest{
		LoginTransactionID: "txn-register-limited",
		LoginContextID:     "plctx-register-limited",
		UserAccount:        "newuser",
		UserName:           "New User",
		UserEmail:          "new@example.com",
		Password:           "Secret123",
		ConfirmPassword:    "Secret123",
		EmailCode:          "123456",
		RequestContext:     &loginfacade.RequestContext{LoginIP: "203.0.113.12", Host: "127.0.0.1:5291"},
	})
	if err == nil || apperrors.From(err).Kind() != apperrors.KindRateLimited {
		t.Fatalf("expected rate limited error, got %v", err)
	}
	if subjects.createFormCalls != 0 {
		t.Fatalf("rate-limited register must not create subject, got %d calls", subjects.createFormCalls)
	}
}

func TestSubmitPasswordReturnsTotpRequiredWhenRiskThresholdMet(t *testing.T) {
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	hash, err := passwordService.Hash(context.Background(), "secret123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	interactions := newFakeInteractionStore()
	interactions.failuresFacade.failures["admin"] = 5
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-2"] = &logindomain.InteractionSnapshot{
		LoginTransactionID:        "txn-2",
		UserAccount:               "admin",
		UserID:                    1001,
		PrimaryChallengeSatisfied: true,
		CreatedAt:                 &now,
		ExpiresAt:                 &expiresAt,
	}
	service := NewService(
		passwordService,
		&fakeCredentialFacade{
			password: &credentialfacade.PasswordCredential{UserID: 1001, PasswordHash: hash},
			totp:     &credentialfacade.TotpCredential{UserID: 1001, CredentialKey: "PRIMARY"},
		},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		nil,
		nil,
		nil,
		interactions.failuresFacade,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	result, err := service.SubmitPassword(context.Background(), loginfacade.PasswordSubmitRequest{
		LoginTransactionID: "txn-2",
		UserAccount:        "admin",
		Password:           "secret123",
	})
	if err != nil {
		t.Fatalf("submit password: %v", err)
	}
	if result == nil || !result.TotpRequired || result.Authenticated {
		t.Fatalf("expected totp required result, got %#v", result)
	}
}

func TestVerifyTotpAuthenticatesAfterChallengePassed(t *testing.T) {
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-3"] = &logindomain.InteractionSnapshot{
		LoginTransactionID: "txn-3",
		UserAccount:        "admin",
		UserID:             1001,
		FlowNonce:          "flow-1",
		CreatedAt:          &now,
		ExpiresAt:          &expiresAt,
	}
	challengeResponse := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-1",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-totp",
			ChallengeType:  "TIME_BASED_ONE_TIME_PASSWORD",
		}},
	}
	service := NewService(
		passwordService,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		&fakeChallengeInternalFacade{response: challengeResponse},
		&fakeChallengeClientFacade{
			getResponse:     challengeResponse,
			respondResponse: &challengefacade.RespondChallengeResponse{ChallengeState: "PASSED", ProofToken: "proof-1"},
		},
		&fakeProofVerifier{},
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	result, err := service.VerifyTotp(context.Background(), loginfacade.TotpVerifyRequest{
		LoginTransactionID: "txn-3",
		UserAccount:        "admin",
		OTPCode:            "123456",
	})
	if err != nil {
		t.Fatalf("verify totp: %v", err)
	}
	if !result.Authenticated {
		t.Fatalf("expected authenticated result, got %#v", result)
	}
}

func TestVerifyTotpRecordsFailureAndLocksAccount(t *testing.T) {
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-totp-failure"] = &logindomain.InteractionSnapshot{
		LoginTransactionID: "txn-totp-failure",
		UserAccount:        "admin",
		UserID:             1001,
		FlowNonce:          "flow-totp",
		CreatedAt:          &now,
		ExpiresAt:          &expiresAt,
	}
	failures := newFakeLoginFailureFacade()
	failures.failures["admin"] = 9
	challengeResponse := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-totp",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-totp",
			ChallengeType:  "TIME_BASED_ONE_TIME_PASSWORD",
		}},
	}
	service := NewService(
		passwordService,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		&fakeChallengeInternalFacade{response: challengeResponse},
		&fakeChallengeClientFacade{
			getResponse: challengeResponse,
			respondResponse: &challengefacade.RespondChallengeResponse{
				ChallengeState:        "FAILED",
				RemainingAttemptCount: 1,
				FailureReason:         "STEP_VERIFY_FAILED",
			},
		},
		&fakeProofVerifier{},
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyTotp(context.Background(), loginfacade.TotpVerifyRequest{
		LoginTransactionID: "txn-totp-failure",
		UserAccount:        "admin",
		OTPCode:            "123456",
		RequestContext:     &loginfacade.RequestContext{LoginIP: "203.0.113.46", DeviceID: "device-totp"},
	})
	if err != nil {
		t.Fatalf("verify totp failure: %v", err)
	}
	if result == nil || result.Authenticated || !result.Locked || result.LockExpiresAt == nil {
		t.Fatalf("expected failed totp verify to lock after threshold, got %#v", result)
	}
	assertTotpFailureDoesNotReturnCredentialMaterial(t, result)
	if failures.failures["admin"] != 10 {
		t.Fatalf("expected totp verify failure to record login failure, got %d", failures.failures["admin"])
	}
	if failures.lastFailureIP != "203.0.113.46" || failures.lastFailureDeviceID != "device-totp" {
		t.Fatalf("expected failure risk context to be recorded, ip=%q device=%q", failures.lastFailureIP, failures.lastFailureDeviceID)
	}
}

func TestVerifyTotpMapsChallengeThrottleToLockedResult(t *testing.T) {
	passwordService, err := passwordinfra.New(config.PasswordConfig{Algorithm: "bcrypt", Bcrypt: config.BcryptPasswordConfig{Cost: 4}})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-totp-throttled"] = &logindomain.InteractionSnapshot{
		LoginTransactionID: "txn-totp-throttled",
		UserAccount:        "admin",
		UserID:             1001,
		FlowNonce:          "flow-totp",
		CreatedAt:          &now,
		ExpiresAt:          &expiresAt,
	}
	failures := newFakeLoginFailureFacade()
	challengeResponse := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-totp",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-totp",
			ChallengeType:  "TIME_BASED_ONE_TIME_PASSWORD",
		}},
	}
	service := NewService(
		passwordService,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		&fakeChallengeInternalFacade{response: challengeResponse},
		&fakeChallengeClientFacade{
			getResponse: challengeResponse,
			respondResponse: &challengefacade.RespondChallengeResponse{
				ChallengeState:  "PENDING",
				CooldownSeconds: 30,
				FailureReason:   "CHALLENGE_THROTTLED",
			},
		},
		&fakeProofVerifier{},
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyTotp(context.Background(), loginfacade.TotpVerifyRequest{
		LoginTransactionID: "txn-totp-throttled",
		UserAccount:        "admin",
		OTPCode:            "123456",
	})
	if err != nil {
		t.Fatalf("verify totp throttled: %v", err)
	}
	if result == nil || result.Authenticated || !result.Locked || result.LockExpiresAt == nil {
		t.Fatalf("expected challenge throttle to map to locked totp result, got %#v", result)
	}
	assertTotpFailureDoesNotReturnCredentialMaterial(t, result)
	if *result.LockExpiresAt <= time.Now().UTC().UnixMilli() {
		t.Fatalf("expected challenge throttle lock expiry in the future, got %d", *result.LockExpiresAt)
	}
	if failures.failures["admin"] != 1 {
		t.Fatalf("expected throttled totp verify to retain failure signal, got %d", failures.failures["admin"])
	}
}

func assertTotpFailureDoesNotReturnCredentialMaterial(t *testing.T, result *loginfacade.TotpVerifyResult) {
	t.Helper()
	if result == nil {
		t.Fatal("missing TOTP verify result")
	}
	if result.AccessToken != "" ||
		result.TokenType != "" ||
		result.AccessTTLSeconds != 0 ||
		result.SessionCookieHeaderValue != "" ||
		result.RefreshCookieHeaderValue != "" {
		t.Fatalf("failed TOTP verify returned credential material: %#v", result)
	}
}

func TestSubmitPasswordRecordsCaptchaFailureWhenPrimaryCaptchaRejected(t *testing.T) {
	interactions := newFakeInteractionStore()
	failures := interactions.failuresFacade
	failures.failures["admin"] = 3
	captchaChallenge := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "captcha-challenge-1",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-captcha",
			ChallengeType:  "IMAGE_CAPTCHA",
			UserInterfaceHints: map[string]any{
				"codeImage": "base64-image",
			},
		}},
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		&fakeChallengeInternalFacade{response: captchaChallenge},
		&fakeChallengeClientFacade{
			getResponse:     captchaChallenge,
			respondResponse: &challengefacade.RespondChallengeResponse{ChallengeState: "FAILED"},
		},
		nil,
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.SubmitPassword(context.Background(), loginfacade.PasswordSubmitRequest{
		LoginTransactionID: "txn-captcha-fail",
		UserAccount:        "admin",
		Password:           "secret123",
		CaptchaCode:        "wrong",
		RequestContext:     &loginfacade.RequestContext{LoginIP: "203.0.113.10"},
	})
	if err != nil {
		t.Fatalf("submit password with wrong captcha: %v", err)
	}
	if result == nil || !result.CaptchaRequired || result.Captcha == nil {
		t.Fatalf("expected refreshed captcha-required result, got %#v", result)
	}
	if !result.CaptchaRejected {
		t.Fatalf("expected wrong captcha result to carry internal CaptchaRejected marker, got %#v", result)
	}
	if got := failures.captchaFailures["admin"]; got != 1 {
		t.Fatalf("expected one captcha failure to be recorded, got %d", got)
	}
	if got := failures.lastCaptchaFailureIP; got != "203.0.113.10" {
		t.Fatalf("expected captcha failure IP to be forwarded, got %q", got)
	}
}

func TestSubmitPasswordForwardsDeviceIDToFailureFacade(t *testing.T) {
	interactions := newFakeInteractionStore()
	failures := interactions.failuresFacade
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		nil,
		nil,
		nil,
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	_, err := service.SubmitPassword(context.Background(), loginfacade.PasswordSubmitRequest{
		LoginTransactionID: "txn-device-failure",
		UserAccount:        "admin",
		Password:           "wrong-password",
		RequestContext: &loginfacade.RequestContext{
			LoginIP:  "203.0.113.10",
			DeviceID: "device-browser-1",
		},
	})
	if err == nil {
		t.Fatalf("expected wrong password to be rejected")
	}
	if failures.lastFailureIP != "203.0.113.10" {
		t.Fatalf("expected failure IP to be forwarded, got %q", failures.lastFailureIP)
	}
	if failures.lastFailureDeviceID != "device-browser-1" {
		t.Fatalf("expected failure device ID to be forwarded, got %q", failures.lastFailureDeviceID)
	}
}

func TestSubmitPasswordRequiresCaptchaWhenContextRiskThresholdMet(t *testing.T) {
	interactions := newFakeInteractionStore()
	failures := interactions.failuresFacade
	failures.riskFailures["ip:203.0.113.10"] = 3
	captchaChallenge := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "captcha-challenge-context",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-captcha-context",
			ChallengeType:  "IMAGE_CAPTCHA",
			UserInterfaceHints: map[string]any{
				"codeImage": "base64-image",
			},
		}},
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		&fakeChallengeInternalFacade{response: captchaChallenge},
		&fakeChallengeClientFacade{getResponse: captchaChallenge},
		nil,
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.SubmitPassword(context.Background(), loginfacade.PasswordSubmitRequest{
		LoginTransactionID: "txn-context-captcha",
		UserAccount:        "admin",
		Password:           "secret123",
		RequestContext:     &loginfacade.RequestContext{LoginIP: "203.0.113.10"},
	})
	if err != nil {
		t.Fatalf("submit password with hot context risk: %v", err)
	}
	if result == nil || !result.CaptchaRequired || result.Captcha == nil {
		t.Fatalf("expected context risk to require captcha before password verification, got %#v", result)
	}
	if result.Captcha.ChallengeIdentifier != "captcha-challenge-context" {
		t.Fatalf("unexpected captcha challenge: %#v", result.Captcha)
	}
}

func TestGetPasswordStateRefreshesExistingCaptchaWhenRequested(t *testing.T) {
	interactions := newFakeInteractionStore()
	interactions.snapshots["txn-refresh-captcha"] = &logindomain.InteractionSnapshot{
		LoginTransactionID:         "txn-refresh-captcha",
		UserAccount:                "admin",
		PrimaryChallengeIdentifier: "captcha-challenge-existing",
	}
	failures := interactions.failuresFacade
	failures.failures["admin"] = 3
	existingChallenge := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "captcha-challenge-existing",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-captcha-old",
			ChallengeType:  "IMAGE_CAPTCHA",
			UserInterfaceHints: map[string]any{
				"codeImage": "old-image",
			},
		}},
	}
	refreshedChallenge := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "captcha-challenge-existing",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-captcha-new",
			ChallengeType:  "IMAGE_CAPTCHA",
			UserInterfaceHints: map[string]any{
				"codeImage": "new-image",
			},
		}},
	}
	challengeClient := &fakeChallengeClientFacade{
		getResponse:     existingChallenge,
		refreshResponse: refreshedChallenge,
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{"admin": {UserID: 1001, AccountName: "admin", Enabled: true}}},
		&fakeChallengeInternalFacade{},
		challengeClient,
		nil,
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.GetPasswordState(context.Background(), loginfacade.PasswordStateRequest{
		LoginTransactionID: "txn-refresh-captcha",
		UserAccount:        "admin",
		RefreshCaptcha:     true,
		RequestContext:     &loginfacade.RequestContext{LoginIP: "203.0.113.10"},
	})
	if err != nil {
		t.Fatalf("get password state with captcha refresh: %v", err)
	}
	if result == nil || result.Captcha == nil {
		t.Fatalf("expected refreshed captcha, got %#v", result)
	}
	if result.Captcha.StepIdentifier != "step-captcha-new" || result.Captcha.ImageBase64 != "new-image" {
		t.Fatalf("expected refreshed captcha material, got %#v", result.Captcha)
	}
	if challengeClient.refreshCalls != 1 {
		t.Fatalf("expected one challenge refresh call, got %d", challengeClient.refreshCalls)
	}
}

func TestPrepareInteractionRejectsCompletedTransactionReplay(t *testing.T) {
	interactions := newFakeInteractionStore()
	interactions.completed["txn-replayed"] = true
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{},
		nil,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)
	if _, _, err := service.prepareInteraction(context.Background(), "txn-replayed", "", "admin", nil, "", ""); err == nil {
		t.Fatalf("prepareInteraction() expected replayed login transaction to be rejected")
	}
}

func TestPrepareInteractionRejectsAccountSwitchOnExistingTransaction(t *testing.T) {
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-switch"] = &logindomain.InteractionSnapshot{
		LoginTransactionID: "txn-switch",
		UserAccount:        "admin",
		UserID:             1001,
		CreatedAt:          &now,
		ExpiresAt:          &expiresAt,
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{},
		nil,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	if _, _, err := service.prepareInteraction(context.Background(), "txn-switch", "", "bob", nil, "", ""); err == nil {
		t.Fatalf("prepareInteraction() expected account switch on existing login transaction to be rejected")
	}
	if got := interactions.snapshots["txn-switch"].UserAccount; got != "admin" {
		t.Fatalf("existing login transaction account was mutated to %q", got)
	}
}

func TestPrepareInteractionAllowsSameAccountOnExistingTransaction(t *testing.T) {
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-same"] = &logindomain.InteractionSnapshot{
		LoginTransactionID: "txn-same",
		UserAccount:        "admin",
		UserID:             1001,
		CreatedAt:          &now,
		ExpiresAt:          &expiresAt,
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{},
		nil,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	userAccount, snapshot, err := service.prepareInteraction(context.Background(), "txn-same", "", "admin", nil, "", "")
	if err != nil {
		t.Fatalf("prepareInteraction() same account returned error: %v", err)
	}
	if userAccount != "admin" || snapshot.UserAccount != "admin" {
		t.Fatalf("expected same account to continue, got userAccount=%q snapshot=%q", userAccount, snapshot.UserAccount)
	}
}

func TestPrepareInteractionBindsLegacyBlankAccountSnapshot(t *testing.T) {
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-blank"] = &logindomain.InteractionSnapshot{
		LoginTransactionID: "txn-blank",
		UserID:             1001,
		CreatedAt:          &now,
		ExpiresAt:          &expiresAt,
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{},
		nil,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	userAccount, snapshot, err := service.prepareInteraction(context.Background(), "txn-blank", "", "admin", nil, "", "")
	if err != nil {
		t.Fatalf("prepareInteraction() blank account snapshot returned error: %v", err)
	}
	if userAccount != "admin" || snapshot.UserAccount != "admin" {
		t.Fatalf("expected blank account snapshot to bind admin, got userAccount=%q snapshot=%q", userAccount, snapshot.UserAccount)
	}
}

func TestStartPasskeyReturnsPublicChallengeForUnknownAccount(t *testing.T) {
	interactions := newFakeInteractionStore()
	challenges := &fakeChallengeInternalFacade{}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{}},
		challenges,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.StartPasskey(context.Background(), loginfacade.PasskeyStartRequest{
		LoginTransactionID: "txn-passkey-unknown",
		UserAccount:        "missing",
	})
	if err != nil {
		t.Fatalf("StartPasskey() unknown account must return public challenge, got error: %v", err)
	}
	assertPublicPasskeyStartResult(t, result)
	if challenges.startCalls != 0 {
		t.Fatalf("unknown account must not start a real passkey challenge, got %d calls", challenges.startCalls)
	}
}

func TestStartPasskeyReturnsPublicChallengeForAccountWithoutPasskey(t *testing.T) {
	interactions := newFakeInteractionStore()
	challenges := &fakeChallengeInternalFacade{}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		challenges,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.StartPasskey(context.Background(), loginfacade.PasskeyStartRequest{
		LoginTransactionID: "txn-passkey-empty",
		UserAccount:        "admin",
	})
	if err != nil {
		t.Fatalf("StartPasskey() no-passkey account must return public challenge, got error: %v", err)
	}
	assertPublicPasskeyStartResult(t, result)
	if challenges.startCalls != 0 {
		t.Fatalf("no-passkey account must not start a real passkey challenge, got %d calls", challenges.startCalls)
	}
}

func TestStartPasskeyReturnsPublicChallengeForDisabledAccount(t *testing.T) {
	interactions := newFakeInteractionStore()
	challenges := &fakeChallengeInternalFacade{}
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: false},
		}},
		challenges,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.StartPasskey(context.Background(), loginfacade.PasskeyStartRequest{
		LoginTransactionID: "txn-passkey-disabled",
		UserAccount:        "admin",
	})
	if err != nil {
		t.Fatalf("StartPasskey() disabled account must return public challenge, got error: %v", err)
	}
	assertPublicPasskeyStartResult(t, result)
	if challenges.startCalls != 0 {
		t.Fatalf("disabled account must not start a real passkey challenge, got %d calls", challenges.startCalls)
	}
}

func TestStartPasskeyReturnsPublicChallengeWhenRealChallengeUnavailable(t *testing.T) {
	interactions := newFakeInteractionStore()
	challenges := &fakeChallengeInternalFacade{response: &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-without-passkey-step",
		ChallengeState:      "PENDING",
	}}
	service := NewService(
		nil,
		&fakeCredentialFacade{passkeys: []credentialfacade.PasskeyCredential{{
			UserID:        1001,
			CredentialKey: "credential-1",
		}}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		challenges,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.StartPasskey(context.Background(), loginfacade.PasskeyStartRequest{
		LoginTransactionID: "txn-passkey-unavailable",
		UserAccount:        "admin",
	})
	if err != nil {
		t.Fatalf("StartPasskey() unavailable real challenge must return public challenge, got error: %v", err)
	}
	assertPublicPasskeyStartResult(t, result)
	if challenges.startCalls != 1 {
		t.Fatalf("passkey account should attempt one real challenge before public fallback, got %d calls", challenges.startCalls)
	}
}

func TestStartPasskeyKeepsRealChallengeForPasskeyAccount(t *testing.T) {
	interactions := newFakeInteractionStore()
	challenges := &fakeChallengeInternalFacade{response: &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-passkey",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-passkey",
			ChallengeType:  "WEBAUTHN_PASSKEY_ASSERTION",
			UserInterfaceHints: map[string]any{
				"challenge":          "real-challenge",
				"rpId":               "localhost",
				"timeoutSeconds":     300,
				"allowCredentialIds": []string{"credential-1"},
			},
		}},
	}}
	service := NewService(
		nil,
		&fakeCredentialFacade{passkeys: []credentialfacade.PasskeyCredential{{
			UserID:        1001,
			CredentialKey: "credential-1",
		}}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		challenges,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.StartPasskey(context.Background(), loginfacade.PasskeyStartRequest{
		LoginTransactionID: "txn-passkey-real",
		UserAccount:        "admin",
	})
	if err != nil {
		t.Fatalf("StartPasskey() passkey account returned error: %v", err)
	}
	if result == nil || result.ChallengeIdentifier != "challenge-passkey" || result.StepIdentifier != "step-passkey" {
		t.Fatalf("expected real challenge result, got %#v", result)
	}
	if challenges.startCalls != 1 {
		t.Fatalf("passkey account must start exactly one real challenge, got %d calls", challenges.startCalls)
	}
	assertEmptyAllowCredentialIds(t, result.UserInterfaceHints)
	assertEmptyAllowCredentialIds(t, result.Challenge.Steps[0].UserInterfaceHints)
}

func TestStartPasskeyHandlesRealChallengeWithoutHints(t *testing.T) {
	interactions := newFakeInteractionStore()
	challenges := &fakeChallengeInternalFacade{response: &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-passkey",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-passkey",
			ChallengeType:  "WEBAUTHN_PASSKEY_ASSERTION",
		}},
	}}
	service := NewService(
		nil,
		&fakeCredentialFacade{passkeys: []credentialfacade.PasskeyCredential{{
			UserID:        1001,
			CredentialKey: "credential-1",
		}}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		challenges,
		nil,
		nil,
		newFakeLoginFailureFacade(),
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.StartPasskey(context.Background(), loginfacade.PasskeyStartRequest{
		LoginTransactionID: "txn-passkey-real-no-hints",
		UserAccount:        "admin",
	})
	if err != nil {
		t.Fatalf("StartPasskey() real challenge without hints returned error: %v", err)
	}
	if result == nil || result.ChallengeIdentifier != "challenge-passkey" || result.StepIdentifier != "step-passkey" {
		t.Fatalf("expected real challenge result, got %#v", result)
	}
	assertEmptyAllowCredentialIds(t, result.UserInterfaceHints)
	assertEmptyAllowCredentialIds(t, result.Challenge.Steps[0].UserInterfaceHints)
}

func assertPublicPasskeyStartResult(t *testing.T, result *loginfacade.PasskeyStartResult) {
	t.Helper()
	if result == nil {
		t.Fatal("expected public passkey start result")
	}
	if strings.TrimSpace(result.ChallengeIdentifier) == "" {
		t.Fatalf("expected non-empty public challenge identifier: %#v", result)
	}
	if strings.TrimSpace(result.StepIdentifier) == "" {
		t.Fatalf("expected non-empty public step identifier: %#v", result)
	}
	hints := result.UserInterfaceHints
	if strings.TrimSpace(stringValue(hints["challenge"])) == "" {
		t.Fatalf("expected synthetic WebAuthn challenge hint: %#v", hints)
	}
	if strings.TrimSpace(stringValue(hints["rpId"])) == "" {
		t.Fatalf("expected synthetic WebAuthn rpId hint: %#v", hints)
	}
	assertEmptyAllowCredentialIds(t, hints)
	if result.Challenge == nil || len(result.Challenge.Steps) != 1 {
		t.Fatalf("expected one-step public challenge envelope: %#v", result.Challenge)
	}
}

func assertEmptyAllowCredentialIds(t *testing.T, hints map[string]any) {
	t.Helper()
	allowCredentials, ok := hints["allowCredentialIds"].([]string)
	if !ok || len(allowCredentials) != 0 {
		t.Fatalf("expected empty allowCredentialIds hint: %#v", hints["allowCredentialIds"])
	}
}

func TestVerifyPasskeyReturnsUnauthenticatedForUnknownAccount(t *testing.T) {
	failures := newFakeLoginFailureFacade()
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{}},
		&fakeChallengeInternalFacade{},
		&fakeChallengeClientFacade{},
		nil,
		failures,
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyPasskey(context.Background(), loginfacade.PasskeyVerifyRequest{
		LoginTransactionID:   "txn-passkey-verify-unknown",
		UserAccount:          "missing",
		CredentialIdentifier: "credential-1",
		ClientDataJSON:       "client-data",
		AuthenticatorData:    "authenticator-data",
		Signature:            "signature",
	})
	if err != nil {
		t.Fatalf("VerifyPasskey() unknown account must fail closed without public error, got: %v", err)
	}
	if result == nil || result.Authenticated {
		t.Fatalf("expected unauthenticated result, got %#v", result)
	}
	if failures.failures["missing"] != 0 {
		t.Fatalf("unknown account must not record passkey failure, got %d", failures.failures["missing"])
	}
}

func TestVerifyPasskeyReturnsUnauthenticatedForAccountWithoutPasskey(t *testing.T) {
	failures := newFakeLoginFailureFacade()
	service := NewService(
		nil,
		&fakeCredentialFacade{},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		&fakeChallengeInternalFacade{},
		&fakeChallengeClientFacade{},
		nil,
		failures,
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyPasskey(context.Background(), loginfacade.PasskeyVerifyRequest{
		LoginTransactionID:   "txn-passkey-verify-empty",
		UserAccount:          "admin",
		CredentialIdentifier: "credential-1",
		ClientDataJSON:       "client-data",
		AuthenticatorData:    "authenticator-data",
		Signature:            "signature",
	})
	if err != nil {
		t.Fatalf("VerifyPasskey() no-passkey account must fail closed without public error, got: %v", err)
	}
	if result == nil || result.Authenticated {
		t.Fatalf("expected unauthenticated result, got %#v", result)
	}
	if failures.failures["admin"] != 0 {
		t.Fatalf("no-passkey account must not record passkey failure, got %d", failures.failures["admin"])
	}
}

func TestVerifyPasskeyReturnsUnauthenticatedForDisabledAccountWithoutRecordingFailure(t *testing.T) {
	failures := newFakeLoginFailureFacade()
	service := NewService(
		nil,
		&fakeCredentialFacade{passkeys: []credentialfacade.PasskeyCredential{{
			UserID:        1001,
			CredentialKey: "credential-1",
		}}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: false},
		}},
		&fakeChallengeInternalFacade{},
		&fakeChallengeClientFacade{},
		nil,
		failures,
		newFakeInteractionStore(),
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyPasskey(context.Background(), loginfacade.PasskeyVerifyRequest{
		LoginTransactionID:   "txn-passkey-verify-disabled",
		UserAccount:          "admin",
		CredentialIdentifier: "credential-1",
		ClientDataJSON:       "client-data",
		AuthenticatorData:    "authenticator-data",
		Signature:            "signature",
	})
	if err != nil {
		t.Fatalf("VerifyPasskey() disabled account must fail closed without public error, got: %v", err)
	}
	if result == nil || result.Authenticated {
		t.Fatalf("expected unauthenticated result, got %#v", result)
	}
	if failures.failures["admin"] != 0 {
		t.Fatalf("disabled account must not record passkey failure, got %d", failures.failures["admin"])
	}
}

func TestVerifyPasskeyRecordsFailureAndLocksPasskeyAccount(t *testing.T) {
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-passkey-verify-failure"] = &logindomain.InteractionSnapshot{
		LoginTransactionID:  "txn-passkey-verify-failure",
		UserAccount:         "admin",
		UserID:              1001,
		ChallengeIdentifier: "challenge-passkey",
		FlowNonce:           "flow-passkey",
		CreatedAt:           &now,
		ExpiresAt:           &expiresAt,
	}
	failures := newFakeLoginFailureFacade()
	failures.failures["admin"] = 9
	challenge := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-passkey",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-passkey",
			ChallengeType:  "WEBAUTHN_PASSKEY_ASSERTION",
			StepState:      "PENDING",
		}},
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{passkeys: []credentialfacade.PasskeyCredential{{
			UserID:        1001,
			CredentialKey: "credential-1",
		}}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		&fakeChallengeInternalFacade{response: challenge},
		&fakeChallengeClientFacade{
			getResponse: challenge,
			respondResponse: &challengefacade.RespondChallengeResponse{
				ChallengeState:        "FAILED",
				RemainingAttemptCount: 1,
			},
		},
		nil,
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyPasskey(context.Background(), loginfacade.PasskeyVerifyRequest{
		LoginTransactionID:   "txn-passkey-verify-failure",
		UserAccount:          "admin",
		CredentialIdentifier: "credential-1",
		ClientDataJSON:       "bad-client-data",
		AuthenticatorData:    "bad-authenticator-data",
		Signature:            "bad-signature",
		RequestContext:       &loginfacade.RequestContext{LoginIP: "203.0.113.45", DeviceID: "device-passkey"},
	})
	if err != nil {
		t.Fatalf("VerifyPasskey() failed: %v", err)
	}
	if result == nil || result.Authenticated || !result.Locked || result.LockExpiresAt == nil {
		t.Fatalf("expected failed passkey verify to lock after threshold, got %#v", result)
	}
	if failures.failures["admin"] != 10 {
		t.Fatalf("expected passkey verify failure to record login failure, got %d", failures.failures["admin"])
	}
	if failures.lastFailureIP != "203.0.113.45" || failures.lastFailureDeviceID != "device-passkey" {
		t.Fatalf("expected failure risk context to be recorded, ip=%q device=%q", failures.lastFailureIP, failures.lastFailureDeviceID)
	}
}

func TestVerifyPasskeyMapsChallengeThrottleToLockedResult(t *testing.T) {
	interactions := newFakeInteractionStore()
	now := time.Now().UTC()
	expiresAt := now.Add(5 * time.Minute)
	interactions.snapshots["txn-passkey-verify-throttled"] = &logindomain.InteractionSnapshot{
		LoginTransactionID:  "txn-passkey-verify-throttled",
		UserAccount:         "admin",
		UserID:              1001,
		ChallengeIdentifier: "challenge-passkey",
		FlowNonce:           "flow-passkey",
		CreatedAt:           &now,
		ExpiresAt:           &expiresAt,
	}
	failures := newFakeLoginFailureFacade()
	challenge := &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "challenge-passkey",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "step-passkey",
			ChallengeType:  "WEBAUTHN_PASSKEY_ASSERTION",
			StepState:      "PENDING",
		}},
	}
	service := NewService(
		nil,
		&fakeCredentialFacade{passkeys: []credentialfacade.PasskeyCredential{{
			UserID:        1001,
			CredentialKey: "credential-1",
		}}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		&fakeChallengeInternalFacade{response: challenge},
		&fakeChallengeClientFacade{
			getResponse: challenge,
			respondResponse: &challengefacade.RespondChallengeResponse{
				ChallengeState:  "PENDING",
				CooldownSeconds: 45,
				FailureReason:   "CHALLENGE_THROTTLED",
			},
		},
		nil,
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyPasskey(context.Background(), loginfacade.PasskeyVerifyRequest{
		LoginTransactionID:   "txn-passkey-verify-throttled",
		UserAccount:          "admin",
		CredentialIdentifier: "credential-1",
		ClientDataJSON:       "bad-client-data",
		AuthenticatorData:    "bad-authenticator-data",
		Signature:            "bad-signature",
	})
	if err != nil {
		t.Fatalf("VerifyPasskey() throttled failed: %v", err)
	}
	if result == nil || result.Authenticated || !result.Locked || result.LockExpiresAt == nil {
		t.Fatalf("expected challenge throttle to map to locked passkey result, got %#v", result)
	}
	if *result.LockExpiresAt <= time.Now().UTC().UnixMilli() {
		t.Fatalf("expected challenge throttle lock expiry in the future, got %d", *result.LockExpiresAt)
	}
	if failures.failures["admin"] != 1 {
		t.Fatalf("expected throttled passkey verify to retain failure signal, got %d", failures.failures["admin"])
	}
}

func TestVerifyPasskeyRecordsFailureWhenPasskeyStepUnavailable(t *testing.T) {
	interactions := newFakeInteractionStore()
	failures := newFakeLoginFailureFacade()
	failures.failures["admin"] = 9
	service := NewService(
		nil,
		&fakeCredentialFacade{passkeys: []credentialfacade.PasskeyCredential{{
			UserID:        1001,
			CredentialKey: "credential-1",
		}}},
		&fakeSubjectLookupFacade{byAccount: map[string]*userfacade.SubjectRecord{
			"admin": {UserID: 1001, AccountName: "admin", Enabled: true},
		}},
		&fakeChallengeInternalFacade{response: &challengefacade.StartChallengeResponse{
			ChallengeIdentifier: "challenge-without-passkey-step",
			ChallengeState:      "PENDING",
			Steps: []challengefacade.ChallengeStepVO{{
				StepIdentifier: "step-totp",
				ChallengeType:  "TIME_BASED_ONE_TIME_PASSWORD",
				StepState:      "PENDING",
			}},
		}},
		&fakeChallengeClientFacade{},
		nil,
		failures,
		interactions,
		logindomain.NewRiskPolicy(config.LoginConfig{CaptchaThreshold: 3, TOTPThreshold: 5, LockThreshold: 10, LockDurationHours: 24}),
		5*time.Minute,
	)

	result, err := service.VerifyPasskey(context.Background(), loginfacade.PasskeyVerifyRequest{
		LoginTransactionID:   "txn-passkey-step-unavailable",
		UserAccount:          "admin",
		CredentialIdentifier: "credential-1",
		ClientDataJSON:       "bad-client-data",
		AuthenticatorData:    "bad-authenticator-data",
		Signature:            "bad-signature",
	})
	if err != nil {
		t.Fatalf("VerifyPasskey() unavailable step failed: %v", err)
	}
	if result == nil || result.Authenticated || !result.Locked || result.LockExpiresAt == nil {
		t.Fatalf("expected unavailable passkey step to retain punishment semantics, got %#v", result)
	}
	if failures.failures["admin"] != 10 {
		t.Fatalf("expected unavailable passkey step to record failure, got %d", failures.failures["admin"])
	}
}

type fakeInteractionStore struct {
	snapshots      map[string]*logindomain.InteractionSnapshot
	completed      map[string]bool
	failuresFacade *fakeLoginFailureFacade
}

func newFakeInteractionStore() *fakeInteractionStore {
	return &fakeInteractionStore{
		snapshots:      make(map[string]*logindomain.InteractionSnapshot),
		completed:      make(map[string]bool),
		failuresFacade: newFakeLoginFailureFacade(),
	}
}

func (f *fakeInteractionStore) GetInteraction(_ context.Context, loginTransactionID string) (*logindomain.InteractionSnapshot, error) {
	return f.snapshots[loginTransactionID], nil
}
func (f *fakeInteractionStore) SaveInteraction(_ context.Context, snapshot *logindomain.InteractionSnapshot, _ time.Duration) error {
	f.snapshots[snapshot.LoginTransactionID] = snapshot
	return nil
}
func (f *fakeInteractionStore) RemoveInteraction(_ context.Context, loginTransactionID string) error {
	delete(f.snapshots, loginTransactionID)
	return nil
}
func (f *fakeInteractionStore) IsCompleted(_ context.Context, loginTransactionID string) (bool, error) {
	return f.completed[loginTransactionID], nil
}
func (f *fakeInteractionStore) MarkCompleted(_ context.Context, loginTransactionID string, _ time.Duration) error {
	f.completed[loginTransactionID] = true
	return nil
}
func (f *fakeInteractionStore) AcquirePrimaryAuthenticationLock(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (f *fakeInteractionStore) ReleasePrimaryAuthenticationLock(_ context.Context, _ string) error {
	return nil
}
func (f *fakeInteractionStore) AcquireChallengeDispatchLock(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (f *fakeInteractionStore) ReleaseChallengeDispatchLock(_ context.Context, _ string) error {
	return nil
}

type fakeLoginFailureFacade struct {
	failures             map[string]int
	riskFailures         map[string]int
	locks                map[string]int64
	captchaFailures      map[string]int
	lastFailureIP        string
	lastFailureDeviceID  string
	lastCaptchaFailureIP string
}

func newFakeLoginFailureFacade() *fakeLoginFailureFacade {
	return &fakeLoginFailureFacade{
		failures:        make(map[string]int),
		riskFailures:    make(map[string]int),
		locks:           make(map[string]int64),
		captchaFailures: make(map[string]int),
	}
}

func (f *fakeLoginFailureFacade) RecordFailure(_ context.Context, userAccount, clientIP, deviceID string) error {
	f.failures[userAccount]++
	f.lastFailureIP = clientIP
	f.lastFailureDeviceID = deviceID
	if f.failures[userAccount] >= 10 {
		f.locks[userAccount] = time.Now().UTC().Add(24 * time.Hour).UnixMilli()
	}
	return nil
}

func (f *fakeLoginFailureFacade) ClearFailure(_ context.Context, userAccount string) error {
	delete(f.failures, userAccount)
	delete(f.locks, userAccount)
	delete(f.captchaFailures, userAccount)
	return nil
}

func (f *fakeLoginFailureFacade) NeedCaptcha(_ context.Context, userAccount string) (bool, error) {
	return f.failures[userAccount] >= 3, nil
}

func (f *fakeLoginFailureFacade) IsAccountLocked(_ context.Context, userAccount string) (bool, error) {
	_, ok := f.locks[userAccount]
	return ok, nil
}

func (f *fakeLoginFailureFacade) GetUnlockTime(_ context.Context, userAccount string) (*int64, error) {
	value, ok := f.locks[userAccount]
	if !ok {
		return nil, nil
	}
	return &value, nil
}

func (f *fakeLoginFailureFacade) GetFailureCount(_ context.Context, userAccount string) (int, error) {
	return f.failures[userAccount], nil
}

func (f *fakeLoginFailureFacade) GetRiskFailureCount(_ context.Context, userAccount, clientIP, deviceID string) (int, error) {
	count := f.failures[userAccount]
	for _, key := range []string{"ip:" + clientIP, "device:" + deviceID, "ip_device:" + clientIP + "|" + deviceID} {
		if f.riskFailures[key] > count {
			count = f.riskFailures[key]
		}
	}
	return count, nil
}

func (f *fakeLoginFailureFacade) UnlockAccount(context.Context, string) error    { return nil }
func (f *fakeLoginFailureFacade) LockAccount(context.Context, string, int) error { return nil }
func (f *fakeLoginFailureFacade) RecordCaptchaFailure(_ context.Context, userAccount, clientIP string) error {
	f.captchaFailures[userAccount]++
	f.lastCaptchaFailureIP = clientIP
	return nil
}
func (f *fakeLoginFailureFacade) ClearCaptchaFailure(_ context.Context, userAccount string) error {
	delete(f.captchaFailures, userAccount)
	return nil
}
func (f *fakeLoginFailureFacade) GetCaptchaFailureCount(_ context.Context, userAccount string) (int, error) {
	return f.captchaFailures[userAccount], nil
}

type fakeCredentialFacade struct {
	password *credentialfacade.PasswordCredential
	totp     *credentialfacade.TotpCredential
	passkeys []credentialfacade.PasskeyCredential
}

func (f *fakeCredentialFacade) FindActivePasswordByUserID(context.Context, int64) (*credentialfacade.PasswordCredential, error) {
	return f.password, nil
}
func (f *fakeCredentialFacade) UpsertPasswordCredential(context.Context, credentialfacade.UpsertPasswordCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) MarkPasswordUsed(context.Context, int64, time.Time) error { return nil }
func (f *fakeCredentialFacade) FindActiveTotpByUserID(context.Context, int64) (*credentialfacade.TotpCredential, error) {
	return f.totp, nil
}
func (f *fakeCredentialFacade) FindActiveTotpSecretByUserID(context.Context, int64) (*credentialfacade.TotpSecret, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) UpsertTotpCredential(context.Context, credentialfacade.UpsertTotpCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) CompleteTotpBinding(context.Context, credentialfacade.CompleteTotpBindingCommand) error {
	return nil
}
func (f *fakeCredentialFacade) DisableTotpCredential(context.Context, int64) (bool, error) {
	return true, nil
}
func (f *fakeCredentialFacade) MarkTotpUsed(context.Context, int64, time.Time) error { return nil }
func (f *fakeCredentialFacade) ListActivePasskeys(context.Context, int64) ([]credentialfacade.PasskeyCredential, error) {
	return f.passkeys, nil
}
func (f *fakeCredentialFacade) FindActivePasskeyByCredentialKey(context.Context, string) (*credentialfacade.PasskeyCredential, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) SavePasskeyCredential(context.Context, credentialfacade.SavePasskeyCredentialCommand) error {
	return nil
}
func (f *fakeCredentialFacade) CompletePasskeyBinding(context.Context, credentialfacade.CompletePasskeyBindingCommand) error {
	return nil
}
func (f *fakeCredentialFacade) DisablePasskeyCredential(context.Context, int64, string) (bool, error) {
	return true, nil
}
func (f *fakeCredentialFacade) UpdatePasskeyUsage(context.Context, string, int64, time.Time) error {
	return nil
}
func (f *fakeCredentialFacade) CountAvailableRecoveryCodes(context.Context, int64) (int, error) {
	return 0, nil
}
func (f *fakeCredentialFacade) RegenerateRecoveryCodes(context.Context, int64, int) (*credentialfacade.RegeneratedRecoveryCodes, error) {
	return nil, nil
}
func (f *fakeCredentialFacade) ConsumeRecoveryCode(context.Context, int64, string, time.Time) (bool, error) {
	return false, nil
}

type fakeSubjectLookupFacade struct {
	byAccount          map[string]*userfacade.SubjectRecord
	createFormCalls    int
	createdFormCommand userfacade.CreateFormSubjectCommand
}

func (f *fakeSubjectLookupFacade) FindSubjectByID(context.Context, int64) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
func (f *fakeSubjectLookupFacade) ExistsByID(context.Context, int64) (bool, error) {
	return false, nil
}
func (f *fakeSubjectLookupFacade) BuildPrincipalSeed(context.Context, int64) (*userfacade.UserPrincipalSeed, error) {
	return nil, nil
}
func (f *fakeSubjectLookupFacade) FindSubjectByAccount(_ context.Context, account string) (*userfacade.SubjectRecord, error) {
	return f.byAccount[account], nil
}
func (f *fakeSubjectLookupFacade) FindSubjectByEmail(context.Context, string) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
func (f *fakeSubjectLookupFacade) CreateExternalSubject(context.Context, userfacade.CreateExternalSubjectCommand) (*userfacade.SubjectRecord, error) {
	return nil, nil
}
func (f *fakeSubjectLookupFacade) CreateFormSubject(_ context.Context, command userfacade.CreateFormSubjectCommand) (*userfacade.SubjectRecord, error) {
	f.createFormCalls++
	f.createdFormCommand = command
	return &userfacade.SubjectRecord{UserID: 2001, AccountName: command.AccountName, Enabled: true}, nil
}

type fakePlatformFacade struct {
	platformCode   string
	requiredMethod string
	formPolicy     *platformfacade.ProvisioningPolicy
}

func (f *fakePlatformFacade) ResolveLoginOptions(context.Context, platformfacade.ResolvePlatformRequest) (*platformfacade.LoginOptionResult, error) {
	return nil, nil
}
func (f *fakePlatformFacade) ResolvePlatformCode(context.Context, platformfacade.ResolvePlatformRequest) (string, error) {
	return f.platformCode, nil
}
func (f *fakePlatformFacade) ValidateLoginContext(_ context.Context, loginContextID string, _ platformfacade.ResolvePlatformRequest) (*platformfacade.LoginContextValidation, error) {
	return &platformfacade.LoginContextValidation{LoginContextID: loginContextID, PlatformCode: f.platformCode, Authority: "PRESENTATION"}, nil
}
func (f *fakePlatformFacade) IssueProvisioningAuthority(_ context.Context, loginContextID string, _ platformfacade.ResolvePlatformRequest) (*platformfacade.ProvisioningAuthority, error) {
	return &platformfacade.ProvisioningAuthority{AuthorityID: "plprov_test", LoginContextID: loginContextID, PlatformCode: f.platformCode, Authority: platformfacade.AuthorityProvisioning}, nil
}
func (f *fakePlatformFacade) GetProvisioningPolicy(context.Context, platformfacade.ProvisioningAuthority) (*platformfacade.ProvisioningPolicy, error) {
	return nil, nil
}
func (f *fakePlatformFacade) GetFormRegistrationPolicy(context.Context, string) (*platformfacade.ProvisioningPolicy, error) {
	if f.formPolicy != nil {
		return f.formPolicy, nil
	}
	return &platformfacade.ProvisioningPolicy{PlatformCode: f.platformCode, AllowFormRegister: true}, nil
}
func (f *fakePlatformFacade) RequireLoginMethod(_ context.Context, _ string, methodType string, _ string) error {
	f.requiredMethod = methodType
	return nil
}

type fakeAuthorizationSessions struct {
	snapshot *ssofacade.AuthorizationSessionSnapshot
}

func (f *fakeAuthorizationSessions) CreateAuthorizationSession(context.Context, ssofacade.CreateAuthorizationSessionRequest) (*ssofacade.AuthorizationSessionSnapshot, error) {
	return nil, nil
}
func (f *fakeAuthorizationSessions) GetAuthorizationSession(context.Context, string) (*ssofacade.AuthorizationSessionSnapshot, error) {
	return f.snapshot, nil
}
func (f *fakeAuthorizationSessions) RemoveAuthorizationSession(context.Context, string) error {
	return nil
}
func (f *fakeAuthorizationSessions) AcquireCompletionLock(context.Context, string) (bool, error) {
	return true, nil
}
func (f *fakeAuthorizationSessions) ReleaseCompletionLock(context.Context, string) error {
	return nil
}
func (f *fakeAuthorizationSessions) MarkSessionFinalized(context.Context, string) (bool, error) {
	return true, nil
}
func (f *fakeAuthorizationSessions) ReleaseSessionFinalized(context.Context, string) error {
	return nil
}

type fakeAuthenticationCompletion struct {
	command ssofacade.CompleteInteractiveAuthenticationCommand
}

func (f *fakeAuthenticationCompletion) CompleteInteractiveAuthentication(_ context.Context, command ssofacade.CompleteInteractiveAuthenticationCommand) (*ssofacade.AuthenticationCompletionResult, error) {
	f.command = command
	return &ssofacade.AuthenticationCompletionResult{Authenticated: true, RedirectURL: "/done"}, nil
}

type fakeChallengeInternalFacade struct {
	response   *challengefacade.StartChallengeResponse
	startCalls int
}

func (f *fakeChallengeInternalFacade) StartChallenge(context.Context, challengefacade.StartChallengeRequest) (*challengefacade.StartChallengeResponse, error) {
	f.startCalls++
	return f.response, nil
}

type fakeChallengeClientFacade struct {
	getResponse     *challengefacade.StartChallengeResponse
	refreshResponse *challengefacade.StartChallengeResponse
	respondResponse *challengefacade.RespondChallengeResponse
	refreshCalls    int
}

func (f *fakeChallengeClientFacade) GetChallenge(context.Context, string) (*challengefacade.StartChallengeResponse, error) {
	return f.getResponse, nil
}
func (f *fakeChallengeClientFacade) Respond(context.Context, string, challengefacade.RespondChallengeRequest) (*challengefacade.RespondChallengeResponse, error) {
	return f.respondResponse, nil
}
func (f *fakeChallengeClientFacade) Refresh(context.Context, string, challengefacade.RefreshChallengeRequest) (*challengefacade.StartChallengeResponse, error) {
	f.refreshCalls++
	if f.refreshResponse != nil {
		return f.refreshResponse, nil
	}
	return f.getResponse, nil
}

type fakeRegisterLimiter struct {
	decision limiterinfra.Decision
	err      error
	calls    int
}

func (f *fakeRegisterLimiter) Allow(context.Context, string, int64, time.Duration) (limiterinfra.Decision, error) {
	f.calls++
	return f.decision, f.err
}

func (f *fakeRegisterLimiter) AllowDefault(context.Context, string) (limiterinfra.Decision, error) {
	f.calls++
	return f.decision, f.err
}

func (f *fakeRegisterLimiter) AllowWithFailOpen(context.Context, string, int64, time.Duration, bool) (limiterinfra.Decision, error) {
	f.calls++
	return f.decision, f.err
}

func registerCaptchaChallenge() *challengefacade.StartChallengeResponse {
	return &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "register-captcha-challenge",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "register-captcha-step",
			ChallengeType:  "IMAGE_CAPTCHA",
			UserInterfaceHints: map[string]any{
				"codeImage": "register-base64-image",
			},
		}},
	}
}

func registerEmailChallenge() *challengefacade.StartChallengeResponse {
	return &challengefacade.StartChallengeResponse{
		ChallengeIdentifier: "register-email-challenge",
		ChallengeState:      "PENDING",
		Steps: []challengefacade.ChallengeStepVO{{
			StepIdentifier: "register-email-step",
			ChallengeType:  "EMAIL_ONE_TIME_PASSWORD",
			UserInterfaceHints: map[string]any{
				"emailMasked": "n***@example.com",
			},
		}},
	}
}

type fakeProofVerifier struct{}

func (f *fakeProofVerifier) VerifyProofToken(context.Context, challengefacade.ProofTokenVerifyRequest) (*challengefacade.ProofTokenClaims, error) {
	return &challengefacade.ProofTokenClaims{
		SubjectIdentifier:         "user:1001",
		BusinessAction:            "LOGIN",
		FlowNonce:                 "flow-1",
		AuthenticationMethodNames: []string{"TIME_BASED_ONE_TIME_PASSWORD"},
		TokenUniqueIdentifier:     "token-1",
		ExpiresAt:                 timePointer(time.Now().UTC().Add(5 * time.Minute)),
	}, nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func int64Pointer(value int64) *int64 {
	return &value
}

func int64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
