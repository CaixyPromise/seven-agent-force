package application

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	challengedomain "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeprovider "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain/provider"
	challengefacade "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	emailinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/email"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	randominfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
	totpinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/totp"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/alicebob/miniredis/v2"
)

func TestChallengeServiceStartIsIdempotentAndRespondIssuesProofToken(t *testing.T) {
	service, repo := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                3,
		ImageCooldownSeconds:            1,
		PasswordMaxAttempts:             3,
		PasswordCooldownSeconds:         1,
		EmailMaxAttempts:                3,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  3,
		OTPCooldownSeconds:              1,
		RecoveryMaxAttempts:             3,
		RecoveryCooldownSeconds:         1,
		RecoveryBatchSize:               8,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionLogin),
		SubjectIdentifier:    "login:alice",
		FlowNonce:            "flow-1",
		IdempotencyKey:       "idem-1",
	})
	if err != nil {
		t.Fatalf("start challenge: %v", err)
	}
	if start.ChallengeIdentifier == "" || len(start.Steps) != 1 {
		t.Fatalf("unexpected start response: %+v", start)
	}

	startAgain, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionLogin),
		SubjectIdentifier:    "login:alice",
		FlowNonce:            "flow-1",
		IdempotencyKey:       "idem-1",
	})
	if err != nil {
		t.Fatalf("start challenge second time: %v", err)
	}
	if startAgain.ChallengeIdentifier != start.ChallengeIdentifier {
		t.Fatalf("expected idempotent challenge identifier reuse, got %s vs %s", startAgain.ChallengeIdentifier, start.ChallengeIdentifier)
	}

	stored := repo.mustSession(t, start.ChallengeIdentifier)
	step := stored.CurrentStep(time.Now().UTC())
	if step == nil {
		t.Fatal("expected current step")
	}
	codeKey := "captcha.code." + step.StepIdentifier
	code, _ := stored.SessionContext[codeKey].(string)
	if code == "" {
		t.Fatalf("expected captcha code in session context, got %+v", stored.SessionContext)
	}

	refreshed, err := service.Refresh(context.Background(), start.ChallengeIdentifier, challengefacade.RefreshChallengeRequest{
		StepIdentifier: step.StepIdentifier,
	})
	if err != nil {
		t.Fatalf("refresh challenge: %v", err)
	}
	if refreshed.ChallengeIdentifier != start.ChallengeIdentifier {
		t.Fatalf("unexpected refresh response: %+v", refreshed)
	}

	stored = repo.mustSession(t, start.ChallengeIdentifier)
	step = stored.CurrentStep(time.Now().UTC())
	code, _ = stored.SessionContext["captcha.code."+step.StepIdentifier].(string)
	if code == "" {
		t.Fatal("expected refreshed captcha code")
	}

	responded, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: step.StepIdentifier,
		Payload: map[string]any{
			"captchaCode": code,
		},
	})
	if err != nil {
		t.Fatalf("respond challenge: %v", err)
	}
	if responded.ChallengeState != string(challengedomain.ChallengeStatePassed) || responded.ProofToken == "" {
		t.Fatalf("unexpected respond result: %+v", responded)
	}

	claims, err := service.VerifyProofToken(context.Background(), challengefacade.ProofTokenVerifyRequest{
		ProofToken:          responded.ProofToken,
		AudienceServiceName: "system-admin",
		BusinessAction:      string(challengedomain.BusinessActionLogin),
		FlowNonce:           "flow-1",
		SubjectIdentifier:   "login:alice",
		ConsumeOnce:         true,
	})
	if err != nil {
		t.Fatalf("verify proof token: %v", err)
	}
	if claims.ChallengeIdentifier != start.ChallengeIdentifier {
		t.Fatalf("unexpected claims: %+v", claims)
	}

	if _, err := service.VerifyProofToken(context.Background(), challengefacade.ProofTokenVerifyRequest{
		ProofToken:          responded.ProofToken,
		AudienceServiceName: "system-admin",
		BusinessAction:      string(challengedomain.BusinessActionLogin),
		FlowNonce:           "flow-1",
		SubjectIdentifier:   "login:alice",
		ConsumeOnce:         true,
	}); err == nil {
		t.Fatal("expected proof token replay to be rejected")
	}

	replayed, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: step.StepIdentifier,
		Payload: map[string]any{
			"captchaCode": code,
		},
	})
	if err != nil {
		t.Fatalf("respond challenge after passed: %v", err)
	}
	if replayed.ProofToken != responded.ProofToken {
		t.Fatalf("expected passed-session response to reuse cached proof token, got %s vs %s", replayed.ProofToken, responded.ProofToken)
	}
}

func TestChallengeServiceFailedStateAfterCaptchaAttemptsExhausted(t *testing.T) {
	service, repo := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                1,
		ImageCooldownSeconds:            1,
		PasswordMaxAttempts:             3,
		PasswordCooldownSeconds:         1,
		EmailMaxAttempts:                3,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  3,
		OTPCooldownSeconds:              1,
		RecoveryMaxAttempts:             3,
		RecoveryCooldownSeconds:         1,
		RecoveryBatchSize:               8,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionLogin),
		SubjectIdentifier:    "login:alice",
		FlowNonce:            "flow-2",
		IdempotencyKey:       "idem-2",
	})
	if err != nil {
		t.Fatalf("start challenge: %v", err)
	}

	session := repo.mustSession(t, start.ChallengeIdentifier)
	step := session.CurrentStep(time.Now().UTC())
	if step == nil {
		t.Fatal("expected current step")
	}

	response, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: step.StepIdentifier,
		Payload: map[string]any{
			"captchaCode": "0000",
		},
	})
	if err != nil {
		t.Fatalf("respond challenge with wrong code: %v", err)
	}
	if response.ChallengeState != string(challengedomain.ChallengeStateFailed) || response.FailureReason != "STEP_LOCKED" {
		t.Fatalf("unexpected failed response: %+v", response)
	}
}

func TestChallengeServiceThrottlesRepeatedFailuresAcrossSessionsBySubjectAndFactor(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                2,
		ImageCooldownSeconds:            1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	failLoginCaptcha(t, service, "alice", "idem-subject-1", "10.0.0.1", "device-a")
	failLoginCaptcha(t, service, "alice", "idem-subject-2", "10.0.0.2", "device-b")

	result := failLoginCaptcha(t, service, "alice", "idem-subject-3", "10.0.0.3", "device-c")
	if result.FailureReason != "CHALLENGE_THROTTLED" {
		t.Fatalf("expected cross-session subject/factor throttle, got %+v", result)
	}
}

func TestChallengeServiceThrottlesRepeatedFailuresAcrossSessionsByRiskContext(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                2,
		ImageCooldownSeconds:            1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	failLoginCaptcha(t, service, "alice", "idem-risk-1", "10.9.0.1", "device-shared")
	failLoginCaptcha(t, service, "bob", "idem-risk-2", "10.9.0.1", "device-shared")

	result := failLoginCaptcha(t, service, "carol", "idem-risk-3", "10.9.0.1", "device-shared")
	if result.FailureReason != "CHALLENGE_THROTTLED" {
		t.Fatalf("expected cross-session ip/device throttle, got %+v", result)
	}
}

func TestChallengeServiceThrottlesEmailTargetAcrossSessions(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		EmailMaxAttempts:                2,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  5,
		OTPCooldownSeconds:              1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	failPrivilegedEmailOTP(t, service, "user:1001", "idem-email-1", "config:1|reveal")
	failPrivilegedEmailOTP(t, service, "user:1002", "idem-email-2", "config:2|reveal")

	result := failPrivilegedEmailOTP(t, service, "user:1003", "idem-email-3", "config:3|reveal")
	if result.FailureReason != "CHALLENGE_THROTTLED" {
		t.Fatalf("expected cross-session email-target throttle, got %+v", result)
	}
}

func TestChallengeServiceThrottlesRepeatedChallengeStartBySubject(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                2,
		ImageCooldownSeconds:            1,
		TriggerMaxAttempts:              2,
		ThrottleWindowSeconds:           300,
		ThrottleLockSeconds:             900,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	startLoginCaptcha(t, service, "alice", "idem-start-1", "10.0.1.1", "device-a")
	startLoginCaptcha(t, service, "alice", "idem-start-2", "10.0.1.2", "device-b")

	_, err := service.StartChallenge(context.Background(), loginCaptchaStartRequest("alice", "idem-start-3", "10.0.1.3", "device-c"))
	assertChallengeThrottledError(t, err)
}

func TestChallengeServiceThrottlesEmailOtpStartBeforeThirdSend(t *testing.T) {
	var sent int32
	service, _ := newTestChallengeServiceWithStoreAndEmailSender(
		t,
		config.ChallengeConfig{
			SessionTTLSeconds:               300,
			ProofTokenTTLMinSeconds:         60,
			ProofTokenTTLMaxSeconds:         300,
			EmailMaxAttempts:                2,
			EmailCooldownSeconds:            1,
			TriggerMaxAttempts:              2,
			ThrottleWindowSeconds:           300,
			ThrottleLockSeconds:             900,
			WebAuthnChallengeTimeoutSeconds: 60,
		},
		&testCompletionStore{},
		countingEmailSender{count: &sent},
	)

	startPrivilegedEmailChallenge(t, service, "user:1001", "idem-email-start-1", "config:1|reveal")
	startPrivilegedEmailChallenge(t, service, "user:1002", "idem-email-start-2", "config:2|reveal")

	_, err := service.StartChallenge(context.Background(), privilegedEmailStartRequest("user:1003", "idem-email-start-3", "config:3|reveal"))
	assertChallengeThrottledError(t, err)
	if got := atomic.LoadInt32(&sent); got != 2 {
		t.Fatalf("expected third email challenge start to be blocked before send, sent=%d", got)
	}
}

func TestChallengeServiceThrottlesEmailOtpRefreshBeforeThirdSend(t *testing.T) {
	var sent int32
	service, repo := newTestChallengeServiceWithStoreAndEmailSender(
		t,
		config.ChallengeConfig{
			SessionTTLSeconds:               300,
			ProofTokenTTLMinSeconds:         60,
			ProofTokenTTLMaxSeconds:         300,
			EmailMaxAttempts:                2,
			EmailCooldownSeconds:            1,
			TriggerMaxAttempts:              2,
			ThrottleWindowSeconds:           300,
			ThrottleLockSeconds:             900,
			WebAuthnChallengeTimeoutSeconds: 60,
		},
		&testCompletionStore{},
		countingEmailSender{count: &sent},
	)

	start := startPrivilegedEmailChallenge(t, service, "user:1001", "idem-email-refresh-1", "config:1|reveal")
	step := firstStepOfType(t, start, challengedomain.ChallengeTypeEmailOneTimePassword)
	initialStep := start.Steps[0]
	if cooldown, ok := initialStep.UserInterfaceHints["resendCooldownSeconds"].(int); !ok || cooldown <= 0 {
		t.Fatalf("initial email send must expose resend cooldown, got %+v", initialStep.UserInterfaceHints)
	}
	stored := repo.mustSession(t, start.ChallengeIdentifier)
	stored.SessionContext[emailOTPTriggerContextKey(&stored.Steps[0])] = time.Now().UTC().Add(-2 * time.Second).Format(time.RFC3339Nano)
	if _, err := service.Refresh(context.Background(), start.ChallengeIdentifier, challengefacade.RefreshChallengeRequest{
		StepIdentifier: step.StepIdentifier,
	}); err != nil {
		t.Fatalf("first email refresh should be allowed: %v", err)
	}

	_, err := service.Refresh(context.Background(), start.ChallengeIdentifier, challengefacade.RefreshChallengeRequest{
		StepIdentifier: step.StepIdentifier,
	})
	assertChallengeThrottledError(t, err)
	if got := atomic.LoadInt32(&sent); got != 2 {
		t.Fatalf("expected throttled refresh to be blocked before send, sent=%d", got)
	}
}

func TestChallengeServiceBlocksImmediateEmailOtpResendAndFreshCodeClearsVerificationCooldown(t *testing.T) {
	var sent int32
	service, repo := newTestChallengeServiceWithStoreAndEmailSender(
		t,
		config.ChallengeConfig{
			SessionTTLSeconds:               300,
			ProofTokenTTLMinSeconds:         60,
			ProofTokenTTLMaxSeconds:         300,
			EmailMaxAttempts:                5,
			EmailCooldownSeconds:            60,
			TriggerMaxAttempts:              5,
			ThrottleWindowSeconds:           300,
			ThrottleLockSeconds:             900,
			WebAuthnChallengeTimeoutSeconds: 60,
		},
		&testCompletionStore{},
		countingEmailSender{count: &sent},
	)

	start := startPrivilegedEmailChallenge(t, service, "user:1001", "idem-email-resend-cooldown", "config:1|reveal")
	step := firstStepOfType(t, start, challengedomain.ChallengeTypeEmailOneTimePassword)
	if _, err := service.Refresh(context.Background(), start.ChallengeIdentifier, challengefacade.RefreshChallengeRequest{
		StepIdentifier: step.StepIdentifier,
	}); err == nil {
		t.Fatal("immediate email resend must be rejected")
	} else {
		assertChallengeThrottledError(t, err)
	}
	if got := atomic.LoadInt32(&sent); got != 1 {
		t.Fatalf("immediate resend created a duplicate email: sent=%d", got)
	}

	stored := repo.mustSession(t, start.ChallengeIdentifier)
	storedStep := &stored.Steps[0]
	storedStep.ActivateCooldown(time.Now().UTC())
	stored.SessionContext[emailOTPTriggerContextKey(storedStep)] = time.Now().UTC().Add(-61 * time.Second).Format(time.RFC3339Nano)
	refreshed, err := service.Refresh(context.Background(), start.ChallengeIdentifier, challengefacade.RefreshChallengeRequest{
		StepIdentifier: step.StepIdentifier,
	})
	if err != nil {
		t.Fatalf("email resend after cooldown failed: %v", err)
	}
	if refreshed.Steps[0].CooldownSeconds != 0 {
		t.Fatalf("fresh email code retained old verification cooldown: %+v", refreshed.Steps[0])
	}
	if got := atomic.LoadInt32(&sent); got != 2 {
		t.Fatalf("expected exactly one fresh email after cooldown: sent=%d", got)
	}
	refreshedSession := repo.mustSession(t, start.ChallengeIdentifier)
	refreshedStep := &refreshedSession.Steps[0]
	freshCode, _ := refreshedSession.SessionContext["email.otp.code."+refreshedStep.StepIdentifier].(string)
	if freshCode == "" {
		t.Fatal("fresh email code was not persisted for verification")
	}
	result, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: refreshedStep.StepIdentifier,
		Payload:        map[string]any{"oneTimePassword": freshCode},
	})
	if err != nil {
		t.Fatalf("fresh email code was not immediately verifiable: %v", err)
	}
	if result.ChallengeState != string(challengedomain.ChallengeStatePassed) {
		t.Fatalf("fresh email code did not pass challenge: %+v", result)
	}
}

func TestChallengeServiceTriggerThrottleDoesNotConsumeRespondThrottle(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                5,
		ImageCooldownSeconds:            1,
		TriggerMaxAttempts:              2,
		ThrottleMaxFailures:             2,
		ThrottleWindowSeconds:           300,
		ThrottleLockSeconds:             900,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	start := startLoginCaptcha(t, service, "alice", "idem-trigger-separate-1", "10.0.2.1", "device-a")
	startLoginCaptcha(t, service, "alice", "idem-trigger-separate-2", "10.0.2.2", "device-b")
	_, err := service.StartChallenge(context.Background(), loginCaptchaStartRequest("alice", "idem-trigger-separate-3", "10.0.2.3", "device-c"))
	assertChallengeThrottledError(t, err)

	step := firstStepOfType(t, start, challengedomain.ChallengeTypeImageCaptcha)
	result, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: step.StepIdentifier,
		Payload:        map[string]any{"captchaCode": "wrong"},
	})
	if err != nil {
		t.Fatalf("respond after trigger lock: %v", err)
	}
	if result.FailureReason == "CHALLENGE_THROTTLED" {
		t.Fatalf("trigger throttle should not consume respond throttle counters: %+v", result)
	}
}

func TestChallengeServiceRespondThrottleDoesNotBlockNewChallengeStart(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                5,
		ImageCooldownSeconds:            1,
		TriggerMaxAttempts:              10,
		ThrottleMaxFailures:             2,
		ThrottleWindowSeconds:           300,
		ThrottleLockSeconds:             900,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	failLoginCaptcha(t, service, "alice", "idem-respond-separate-1", "10.0.3.1", "device-a")
	result := failLoginCaptcha(t, service, "alice", "idem-respond-separate-2", "10.0.3.2", "device-b")
	if result.FailureReason != "CHALLENGE_THROTTLED" {
		t.Fatalf("expected respond throttle lock, got %+v", result)
	}

	startLoginCaptcha(t, service, "alice", "idem-respond-separate-3", "10.0.3.3", "device-c")
}

func TestChallengeServiceEmailOtpRefreshRestoresMissingEmailTargetBeforeThrottle(t *testing.T) {
	var sent int32
	service, repo := newTestChallengeServiceWithStoreAndEmailSender(
		t,
		config.ChallengeConfig{
			SessionTTLSeconds:               300,
			ProofTokenTTLMinSeconds:         60,
			ProofTokenTTLMaxSeconds:         300,
			EmailMaxAttempts:                3,
			EmailCooldownSeconds:            1,
			TriggerMaxAttempts:              3,
			ThrottleWindowSeconds:           300,
			ThrottleLockSeconds:             900,
			WebAuthnChallengeTimeoutSeconds: 60,
		},
		&testCompletionStore{},
		countingEmailSender{count: &sent},
	)

	start := startPrivilegedEmailChallenge(t, service, "user:1001", "idem-email-missing-target", "config:1|reveal")
	step := firstStepOfType(t, start, challengedomain.ChallengeTypeEmailOneTimePassword)
	storedBeforeRefresh := repo.mustSession(t, start.ChallengeIdentifier)
	delete(storedBeforeRefresh.SessionContext, "email.target")
	storedBeforeRefresh.SessionContext[emailOTPTriggerContextKey(&storedBeforeRefresh.Steps[0])] = time.Now().UTC().Add(-2 * time.Second).Format(time.RFC3339Nano)

	if _, err := service.Refresh(context.Background(), start.ChallengeIdentifier, challengefacade.RefreshChallengeRequest{
		StepIdentifier: step.StepIdentifier,
	}); err != nil {
		t.Fatalf("refresh should restore email target before throttle: %v", err)
	}
	stored := repo.mustSession(t, start.ChallengeIdentifier)
	if target := stored.SessionContext["email.target"]; target != "alice@example.com" {
		t.Fatalf("expected email target restored, got %+v", target)
	}
	if got := atomic.LoadInt32(&sent); got != 2 {
		t.Fatalf("expected refresh to send second email OTP after restoring target, sent=%d", got)
	}
}

func TestChallengeServiceProfileEmailUpdateRespondAfterCacheRoundTripIssuesProofToken(t *testing.T) {
	mini := miniredis.RunT(t)
	cacheCfg := config.CacheConfig{
		Enabled: true,
		Prefix:  "seven-test",
		Codec:   "sonic",
		L1: config.CacheL1Config{
			Enabled: false,
		},
		Redis: config.RedisCacheConfig{
			Enabled:   true,
			Mode:      config.RedisCacheModeSingle,
			KeyPrefix: "seven-test",
			Single: config.RedisSingleConfig{
				Addr: mini.Addr(),
			},
		},
	}
	cacheProvider := cacheinfra.NewProvider(cacheCfg)
	cacheManager, err := cacheinfra.NewDefaultManager(cacheCfg, cacheProvider)
	if err != nil {
		t.Fatalf("new default cache manager: %v", err)
	}
	defer func() {
		_ = cacheProvider.Close()
	}()

	randomService := randominfra.New(config.RandomConfig{TokenLength: 16, NonceLength: 16, CodeLength: 6})
	store := &profileEmailTestStore{targetEmail: "alice@example.com"}
	stepService := NewStepService(challengeprovider.NewRegistry(
		challengeprovider.NewWebAuthnPasskeyAssertionStepProvider(
			challengeinfra.NewWebAuthnService(randomService),
			store,
			"localhost",
			[]string{"http://127.0.0.1:5177"},
			300,
		),
		challengeprovider.NewTimeBasedOtpChallengeStepProvider(
			totpinfra.New(),
			store,
			challengeprovider.TimeBasedOtpSettings{
				IssuerName:          "SevenFramework",
				AllowedDriftWindows: 1,
			},
		),
		challengeprovider.NewRecoveryCodeChallengeStepProvider(store),
		challengeprovider.NewEmailOtpChallengeStepProvider(challengeinfra.NewEmailOTPService(randomService, fakeEmailSender{}), store),
	))
	sessionRepo := challengeinfra.NewSessionRepository(cacheManager)
	jwtService := newTestJWTService(t)
	proofTokens := challengeinfra.NewProofTokenService(jwtService, sessionRepo, time.Minute, 5*time.Minute)
	service := NewChallengeService(
		config.ChallengeConfig{
			SessionTTLSeconds:               300,
			ProofTokenTTLMinSeconds:         60,
			ProofTokenTTLMaxSeconds:         300,
			EmailMaxAttempts:                3,
			EmailCooldownSeconds:            1,
			RecoveryBatchSize:               8,
			WebAuthnChallengeTimeoutSeconds: 60,
		},
		sessionRepo,
		challengeinfra.NewThrottleRepository(cacheManager),
		stepService,
		NewCompletionHandler(store, 8),
		proofTokens,
	)

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "authorization-app",
		AudienceServiceNames: []string{"authorization-app"},
		BusinessAction:       string(challengedomain.BusinessActionProfileEmailUpdate),
		SubjectIdentifier:    "user:1",
		FlowNonce:            "flow-profile-email",
		IdempotencyKey:       "idem-profile-email",
		ExtensionContext: map[string]any{
			"operationBinding": "email:alice-new@example.com",
			"targetEmail":      "alice-new@example.com",
		},
	})
	if err != nil {
		t.Fatalf("start challenge: %v", err)
	}

	var emailStep challengefacade.ChallengeStepVO
	found := false
	for _, step := range start.Steps {
		if step.ChallengeType == string(challengedomain.ChallengeTypeEmailOneTimePassword) {
			emailStep = step
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected email otp step in challenge response: %+v", start.Steps)
	}

	stored, err := sessionRepo.GetSession(context.Background(), start.ChallengeIdentifier)
	if err != nil {
		t.Fatalf("get stored session: %v", err)
	}
	if stored == nil {
		t.Fatal("expected stored session")
	}
	code, _ := stored.SessionContext["email.otp.code."+emailStep.StepIdentifier].(string)
	if code == "" {
		t.Fatalf("expected cached email otp code in session context, got %+v", stored.SessionContext)
	}

	responded, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: emailStep.StepIdentifier,
		Payload: map[string]any{
			"emailCode": code,
		},
	})
	if err != nil {
		t.Fatalf("respond profile email challenge: %v", err)
	}
	if responded.ChallengeState != string(challengedomain.ChallengeStatePending) || responded.NextStepIdentifier == "" {
		t.Fatalf("expected verify-new email step after current-account proof, got %+v", responded)
	}
	stored, err = sessionRepo.GetSession(context.Background(), start.ChallengeIdentifier)
	if err != nil || stored == nil {
		t.Fatalf("reload verify-new session: session=%+v err=%v", stored, err)
	}
	newCode, _ := stored.SessionContext["email.otp.code."+responded.NextStepIdentifier].(string)
	if newCode == "" {
		t.Fatalf("expected verify-new OTP in session context, got %+v", stored.SessionContext)
	}
	responded, err = service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: responded.NextStepIdentifier,
		Payload:        map[string]any{"emailCode": newCode},
	})
	if err != nil {
		t.Fatalf("respond verify-new email challenge: %v", err)
	}
	if responded.ChallengeState != string(challengedomain.ChallengeStatePassed) || responded.ProofToken == "" {
		t.Fatalf("expected proof only after new address verification, got %+v", responded)
	}
}

func TestChallengeServiceMfaDeleteFallbackDoesNotDowngradePrimaryMethod(t *testing.T) {
	service := &ChallengeService{config: config.ChallengeConfig{}}
	tests := []struct {
		name      string
		action    challengedomain.BusinessAction
		wantFirst challengedomain.ChallengeType
		forbidden []challengedomain.ChallengeType
	}{
		{
			name:      "otp delete requires existing otp",
			action:    challengedomain.BusinessActionMFAOTPDelete,
			wantFirst: challengedomain.ChallengeTypeTimeBasedOneTimePassword,
			forbidden: []challengedomain.ChallengeType{
				challengedomain.ChallengeTypePasswordVerification,
				challengedomain.ChallengeTypeEmailOneTimePassword,
				challengedomain.ChallengeTypeImageCaptcha,
			},
		},
		{
			name:      "passkey delete requires existing passkey",
			action:    challengedomain.BusinessActionMFAPasskeyDelete,
			wantFirst: challengedomain.ChallengeTypeWebAuthnPasskeyAssertion,
			forbidden: []challengedomain.ChallengeType{
				challengedomain.ChallengeTypePasswordVerification,
				challengedomain.ChallengeTypeEmailOneTimePassword,
				challengedomain.ChallengeTypeImageCaptcha,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := service.buildSteps(challengefacade.StartChallengeRequest{
				BusinessAction: string(tt.action),
				ExtensionContext: map[string]any{
					"fallback": true,
				},
			})
			if len(steps) == 0 {
				t.Fatal("expected challenge steps")
			}
			if got := steps[0].ChallengeType; got != tt.wantFirst {
				t.Fatalf("expected first challenge type %s, got %s in steps %+v", tt.wantFirst, got, steps)
			}
			for _, step := range steps {
				for _, forbidden := range tt.forbidden {
					if step.ChallengeType == forbidden {
						t.Fatalf("unexpected downgrade challenge type %s in steps %+v", forbidden, steps)
					}
				}
			}
		})
	}
}

func TestChallengeServiceRejectsUnknownBusinessAction(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                3,
		ImageCooldownSeconds:            1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	if _, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       "UNKNOWN_ACTION",
		SubjectIdentifier:    "user:1001",
		FlowNonce:            "flow-unknown",
		IdempotencyKey:       "idem-unknown",
	}); err == nil {
		t.Fatal("StartChallenge() accepted unknown business action")
	}
}

func TestChallengeServiceMfaFallbackDoesNotDowngradeSwitchOrRecoveryActions(t *testing.T) {
	service := &ChallengeService{config: config.ChallengeConfig{}}
	tests := []struct {
		name      string
		action    challengedomain.BusinessAction
		wantFirst challengedomain.ChallengeType
		forbidden challengedomain.ChallengeType
	}{
		{
			name:      "otp switch requires existing otp first",
			action:    challengedomain.BusinessActionMFAOTPSwitch,
			wantFirst: challengedomain.ChallengeTypeTimeBasedOneTimePassword,
			forbidden: challengedomain.ChallengeTypeEmailOneTimePassword,
		},
		{
			name:      "passkey switch requires existing passkey first",
			action:    challengedomain.BusinessActionMFAPasskeySwitch,
			wantFirst: challengedomain.ChallengeTypeWebAuthnPasskeyAssertion,
			forbidden: challengedomain.ChallengeTypeEmailOneTimePassword,
		},
		{
			name:      "recovery regenerate requires recovery proof",
			action:    challengedomain.BusinessActionMFARecoveryCodesRegenerate,
			wantFirst: challengedomain.ChallengeTypePasswordVerification,
			forbidden: challengedomain.ChallengeTypeEmailOneTimePassword,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := service.buildSteps(challengefacade.StartChallengeRequest{
				BusinessAction: string(tt.action),
				ExtensionContext: map[string]any{
					"fallback": true,
				},
			})
			if len(steps) == 0 {
				t.Fatal("expected challenge steps")
			}
			if got := steps[0].ChallengeType; got != tt.wantFirst {
				t.Fatalf("expected first challenge type %s, got %s in steps %+v", tt.wantFirst, got, steps)
			}
			for _, step := range steps {
				if step.ChallengeType == tt.forbidden {
					t.Fatalf("unexpected downgrade challenge type %s in steps %+v", tt.forbidden, steps)
				}
			}
		})
	}
}

func TestChallengeServiceRejectsUnknownBusinessActionProofToken(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                3,
		ImageCooldownSeconds:            1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})
	now := time.Now().UTC()
	session := &challengedomain.ChallengeSession{
		ChallengeIdentifier:       "challenge-unknown-proof",
		IssuingServiceName:        "challenge-app",
		AudienceServiceNames:      []string{"challenge-app"},
		SubjectIdentifier:         "user:1001",
		FlowNonce:                 "flow-unknown-proof",
		BusinessAction:            "UNKNOWN_ACTION",
		ChallengeState:            challengedomain.ChallengeStatePassed,
		AuthenticationMethodNames: []string{string(challengedomain.ChallengeTypeImageCaptcha)},
		CreatedAt:                 &now,
		ExpiresAt:                 timePointer(now.Add(time.Minute)),
	}
	_, proofToken, err := service.proofTokens.Issue(context.Background(), session)
	if err != nil {
		t.Fatalf("issue proof token: %v", err)
	}

	if _, err := service.VerifyProofToken(context.Background(), challengefacade.ProofTokenVerifyRequest{
		ProofToken:          proofToken,
		AudienceServiceName: "challenge-app",
		BusinessAction:      "UNKNOWN_ACTION",
		FlowNonce:           "flow-unknown-proof",
		SubjectIdentifier:   "user:1001",
	}); err == nil {
		t.Fatal("VerifyProofToken() accepted unknown business action")
	}
}

func TestChallengeServiceRejectsLowAssuranceProofForHighRiskActions(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                3,
		ImageCooldownSeconds:            1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})
	now := time.Now().UTC()
	session := &challengedomain.ChallengeSession{
		ChallengeIdentifier:       "challenge-low-proof",
		IssuingServiceName:        "challenge-app",
		AudienceServiceNames:      []string{"challenge-app"},
		SubjectIdentifier:         "user:1001",
		FlowNonce:                 "flow-low-proof",
		BusinessAction:            string(challengedomain.BusinessActionMFAPasskeyDelete),
		ChallengeState:            challengedomain.ChallengeStatePassed,
		AuthenticationMethodNames: []string{string(challengedomain.ChallengeTypePasswordVerification)},
		SessionContext: map[string]any{
			"operationBinding": "passkey:credential-1",
		},
		CreatedAt: &now,
		ExpiresAt: timePointer(now.Add(time.Minute)),
	}
	_, proofToken, err := service.proofTokens.Issue(context.Background(), session)
	if err != nil {
		t.Fatalf("issue proof token: %v", err)
	}

	if _, err := service.VerifyProofToken(context.Background(), challengefacade.ProofTokenVerifyRequest{
		ProofToken:          proofToken,
		AudienceServiceName: "challenge-app",
		BusinessAction:      string(challengedomain.BusinessActionMFAPasskeyDelete),
		FlowNonce:           "flow-low-proof",
		SubjectIdentifier:   "user:1001",
		OperationBinding:    "passkey:credential-1",
	}); err == nil {
		t.Fatal("VerifyProofToken() accepted password-only proof for passkey delete")
	}
}

func TestChallengeServiceMfaSwitchAndRecoveryProofTokenRejectsPasswordDowngrade(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		WebAuthnChallengeTimeoutSeconds: 60,
	})
	tests := []struct {
		name           string
		action         challengedomain.BusinessAction
		requiredMethod challengedomain.ChallengeType
	}{
		{
			name:           "otp_switch_requires_totp",
			action:         challengedomain.BusinessActionMFAOTPSwitch,
			requiredMethod: challengedomain.ChallengeTypeTimeBasedOneTimePassword,
		},
		{
			name:           "passkey_switch_requires_passkey",
			action:         challengedomain.BusinessActionMFAPasskeySwitch,
			requiredMethod: challengedomain.ChallengeTypeWebAuthnPasskeyAssertion,
		},
		{
			name:           "recovery_codes_regenerate_requires_recovery_code",
			action:         challengedomain.BusinessActionMFARecoveryCodesRegenerate,
			requiredMethod: challengedomain.ChallengeTypeRecoveryCodeVerification,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			passwordProof := issueProofTokenForTest(t, service, "challenge-low-"+tt.name, tt.action, []challengedomain.ChallengeType{
				challengedomain.ChallengeTypePasswordVerification,
			}, "")
			request := challengefacade.ProofTokenVerifyRequest{
				ProofToken:          passwordProof,
				AudienceServiceName: "challenge-app",
				BusinessAction:      string(tt.action),
				FlowNonce:           "flow-challenge-low-" + tt.name,
				SubjectIdentifier:   "user:1001",
			}
			if _, err := service.VerifyProofToken(context.Background(), request); err == nil {
				t.Fatalf("VerifyProofToken() accepted password-only proof for %s", tt.action)
			}

			requiredProof := issueProofTokenForTest(t, service, "challenge-required-"+tt.name, tt.action, []challengedomain.ChallengeType{
				tt.requiredMethod,
			}, "")
			request.ProofToken = requiredProof
			request.FlowNonce = "flow-challenge-required-" + tt.name
			if _, err := service.VerifyProofToken(context.Background(), request); err != nil {
				t.Fatalf("VerifyProofToken() rejected required %s proof for %s: %v", tt.requiredMethod, tt.action, err)
			}
		})
	}
}

func TestChallengeServiceRejectsMissingOperationBindingForBoundActions(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                3,
		ImageCooldownSeconds:            1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})
	now := time.Now().UTC()
	session := &challengedomain.ChallengeSession{
		ChallengeIdentifier:       "challenge-missing-binding",
		IssuingServiceName:        "challenge-app",
		AudienceServiceNames:      []string{"challenge-app"},
		SubjectIdentifier:         "user:1001",
		FlowNonce:                 "flow-missing-binding",
		BusinessAction:            string(challengedomain.BusinessActionMFAPasskeyDelete),
		ChallengeState:            challengedomain.ChallengeStatePassed,
		AuthenticationMethodNames: []string{string(challengedomain.ChallengeTypeWebAuthnPasskeyAssertion)},
		CreatedAt:                 &now,
		ExpiresAt:                 timePointer(now.Add(time.Minute)),
	}
	_, proofToken, err := service.proofTokens.Issue(context.Background(), session)
	if err != nil {
		t.Fatalf("issue proof token: %v", err)
	}

	if _, err := service.VerifyProofToken(context.Background(), challengefacade.ProofTokenVerifyRequest{
		ProofToken:          proofToken,
		AudienceServiceName: "challenge-app",
		BusinessAction:      string(challengedomain.BusinessActionMFAPasskeyDelete),
		FlowNonce:           "flow-missing-binding",
		SubjectIdentifier:   "user:1001",
	}); err == nil {
		t.Fatal("VerifyProofToken() accepted missing operation binding for passkey delete")
	}
}

func TestChallengeServiceStartRejectsMissingOperationBindingForProfileUpdate(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		ImageMaxAttempts:                3,
		ImageCooldownSeconds:            1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	if _, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionProfileEmailUpdate),
		SubjectIdentifier:    "user:1001",
		FlowNonce:            "flow-profile-missing-binding",
		IdempotencyKey:       "idem-profile-missing-binding",
	}); err == nil {
		t.Fatal("StartChallenge() accepted profile update without operation binding")
	}
}

func TestChallengeServicePrivilegedMutationRequiresBindingAndStrongChallengeMethods(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		EmailMaxAttempts:                3,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  3,
		OTPCooldownSeconds:              1,
		WebAuthnChallengeTimeoutSeconds: 60,
	})

	if _, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionRBACAssignUserRoles),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-rbac-missing-binding",
		IdempotencyKey:       "idem-rbac-missing-binding",
	}); err == nil {
		t.Fatal("StartChallenge() accepted privileged RBAC action without operation binding")
	}

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionRBACAssignUserRoles),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-rbac-strong",
		IdempotencyKey:       "idem-rbac-strong",
		ExtensionContext: map[string]any{
			"operationBinding": "user:1001|roles:1,2",
		},
	})
	if err != nil {
		t.Fatalf("StartChallenge() rejected privileged RBAC action with binding: %v", err)
	}
	if start.RequiredAssuranceLevel != "AAL2" || start.ResolvedAssuranceLevel != "AAL2" {
		t.Fatalf("expected AAL2 assurance metadata, got %+v", start)
	}
	if len(start.Steps) == 0 {
		t.Fatalf("expected strong challenge methods, got %+v", start)
	}
	allowed := map[string]bool{
		string(challengedomain.ChallengeTypeWebAuthnPasskeyAssertion): true,
		string(challengedomain.ChallengeTypeTimeBasedOneTimePassword): true,
		string(challengedomain.ChallengeTypeEmailOneTimePassword):     true,
	}
	for _, step := range start.Steps {
		if !allowed[step.ChallengeType] {
			t.Fatalf("unexpected privileged challenge method %s in %+v", step.ChallengeType, start.Steps)
		}
	}
}

func TestChallengeServicePrivilegedMutationBuildsStrongSteps(t *testing.T) {
	service := &ChallengeService{config: config.ChallengeConfig{}}
	tests := []challengedomain.BusinessAction{
		challengedomain.BusinessActionAdminResetPassword,
		challengedomain.BusinessActionCurrentUserPasswordChange,
		challengedomain.BusinessActionAdminForceLogout,
		challengedomain.BusinessActionAdminDeleteUser,
		challengedomain.BusinessActionAdminChangeUserStatus,
		challengedomain.BusinessActionRBACCommitRoleGrants,
		challengedomain.BusinessActionRBACGrantTempPermission,
		challengedomain.BusinessActionRBACRevokeTempPermission,
		challengedomain.BusinessActionRBACExtendTempPermission,
		challengedomain.BusinessActionSSOClientCreate,
		challengedomain.BusinessActionSSOClientUpdate,
		challengedomain.BusinessActionSSOClientStatusChange,
		challengedomain.BusinessActionSSOClientRedirectEdit,
		challengedomain.BusinessActionSSOClientSecretGenerate,
		challengedomain.BusinessActionSSOClientSecretDisable,
		challengedomain.BusinessActionExternalLoginProviderCreate,
		challengedomain.BusinessActionExternalLoginProviderUpdate,
		challengedomain.BusinessActionExternalLoginProviderStatusChange,
		challengedomain.BusinessActionExternalLoginProviderSecretRotate,
		challengedomain.BusinessActionExternalLoginIdentityStatusChange,
		challengedomain.BusinessActionExternalOAuthTokenRevoke,
		challengedomain.BusinessActionEnterpriseConnectionSave,
		challengedomain.BusinessActionEnterpriseConnectionStatus,
		challengedomain.BusinessActionEnterpriseIdentityLoginAccess,
		challengedomain.BusinessActionEnterpriseIdentityUnbind,
		challengedomain.BusinessActionPlatformCreate,
		challengedomain.BusinessActionPlatformUpdate,
		challengedomain.BusinessActionPlatformStatusChange,
		challengedomain.BusinessActionPlatformLoginMethodsReplace,
		challengedomain.BusinessActionPlatformSourceRulesReplace,
		challengedomain.BusinessActionPlatformDefaultRolesReplace,
		challengedomain.BusinessActionNotificationGroupRouteSave,
		challengedomain.BusinessActionNotificationGroupRouteEnable,
		challengedomain.BusinessActionNotificationGroupRouteDisable,
		challengedomain.BusinessActionNotificationGroupRouteRetire,
		challengedomain.BusinessActionNotificationGroupRouteDelete,
		challengedomain.BusinessActionNotificationDeliveryContentView,
	}
	allowed := map[challengedomain.ChallengeType]bool{
		challengedomain.ChallengeTypeWebAuthnPasskeyAssertion: true,
		challengedomain.ChallengeTypeTimeBasedOneTimePassword: true,
		challengedomain.ChallengeTypeEmailOneTimePassword:     true,
	}

	for _, action := range tests {
		t.Run(string(action), func(t *testing.T) {
			steps := service.buildSteps(challengefacade.StartChallengeRequest{
				BusinessAction: string(action),
			})
			if len(steps) == 0 {
				t.Fatal("expected privileged strong challenge steps")
			}
			for _, step := range steps {
				if !allowed[step.ChallengeType] {
					t.Fatalf("unexpected challenge type %s in steps %+v", step.ChallengeType, steps)
				}
				if step.ChallengeType == challengedomain.ChallengeTypePasswordVerification || step.ChallengeType == challengedomain.ChallengeTypeImageCaptcha {
					t.Fatalf("privileged mutation must not downgrade to %s in steps %+v", step.ChallengeType, steps)
				}
			}
		})
	}
}

func TestChallengeServiceEveryAAL2ActionBuildsVerificationSteps(t *testing.T) {
	service := &ChallengeService{config: config.ChallengeConfig{}}
	for _, action := range challengedomain.AllBusinessActions() {
		policy, ok := challengedomain.LookupChallengeActionPolicy(string(action))
		if !ok || policy.MinimumAssuranceLevel != "AAL2" {
			continue
		}
		t.Run(string(action), func(t *testing.T) {
			steps := service.buildSteps(challengefacade.StartChallengeRequest{BusinessAction: string(action)})
			if len(steps) == 0 {
				t.Fatalf("AAL2 action %s has a policy but no verification steps", action)
			}
		})
	}
}

func TestChallengeServicePrivilegedMutationFiltersUnavailableStrongMethods(t *testing.T) {
	service, _ := newTestChallengeServiceWithStore(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		EmailMaxAttempts:                3,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  3,
		OTPCooldownSeconds:              1,
		WebAuthnChallengeTimeoutSeconds: 60,
	}, &profileEmailTestStore{targetEmail: "alice@example.com"})

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionConfigSensitiveReveal),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-config-email-only",
		IdempotencyKey:       "idem-config-email-only",
		ExtensionContext: map[string]any{
			"operationBinding": "config:204|reveal",
		},
	})
	if err != nil {
		t.Fatalf("StartChallenge() rejected email-eligible privileged action: %v", err)
	}
	if len(start.Steps) != 1 || start.Steps[0].ChallengeType != string(challengedomain.ChallengeTypeEmailOneTimePassword) {
		t.Fatalf("expected only eligible email OTP step, got %+v", start.Steps)
	}
	for _, actual := range start.ActualChallengeTypeNames {
		if actual == string(challengedomain.ChallengeTypeWebAuthnPasskeyAssertion) || actual == string(challengedomain.ChallengeTypeTimeBasedOneTimePassword) {
			t.Fatalf("unavailable strong method leaked into challenge types: %+v", start.ActualChallengeTypeNames)
		}
	}
}

func TestChallengeServicePrivilegedMutationFallsBackWhenTotpSecretIsCorrupt(t *testing.T) {
	service, _ := newTestChallengeServiceWithStore(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		EmailMaxAttempts:                3,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  3,
		OTPCooldownSeconds:              1,
		WebAuthnChallengeTimeoutSeconds: 60,
	}, &profileEmailTestStore{
		targetEmail:  "alice@example.com",
		otpSecretErr: apperrors.ObjectState("TOTP凭证不可用，请重新绑定"),
	})

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionAdminForceLogout),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-force-logout-corrupt-totp",
		IdempotencyKey:       "idem-force-logout-corrupt-totp",
		ExtensionContext: map[string]any{
			"operationBinding": "users:1002|force-logout",
		},
	})
	if err != nil {
		t.Fatalf("corrupt TOTP material should not block alternate strong challenge methods: %v", err)
	}
	if len(start.Steps) != 1 || start.Steps[0].ChallengeType != string(challengedomain.ChallengeTypeEmailOneTimePassword) {
		t.Fatalf("expected email OTP fallback when TOTP material is corrupt, got %+v", start.Steps)
	}
}

func TestChallengeServicePrivilegedMutationRejectsWhenNoStrongMethodEligible(t *testing.T) {
	service, _ := newTestChallengeServiceWithStore(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		EmailMaxAttempts:                3,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  3,
		OTPCooldownSeconds:              1,
		WebAuthnChallengeTimeoutSeconds: 60,
	}, &profileEmailTestStore{})

	if _, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionConfigSensitiveReveal),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-config-no-method",
		IdempotencyKey:       "idem-config-no-method",
		ExtensionContext: map[string]any{
			"operationBinding": "config:204|reveal",
		},
	}); err == nil {
		t.Fatal("StartChallenge() exposed privileged action without any eligible strong method")
	}
}

func TestChallengeServiceRecoveryCodeRequiredActionRejectsWhenNoRecoveryCodeEligible(t *testing.T) {
	service, _ := newTestChallengeServiceWithStore(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		PasswordMaxAttempts:             3,
		PasswordCooldownSeconds:         1,
		RecoveryMaxAttempts:             3,
		RecoveryCooldownSeconds:         1,
		WebAuthnChallengeTimeoutSeconds: 60,
	}, &testCompletionStore{})

	if _, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionMFARecoveryCodesRegenerate),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-recovery-code-empty",
		IdempotencyKey:       "idem-recovery-code-empty",
	}); err == nil {
		t.Fatal("StartChallenge() exposed a recovery-code-required action without available recovery codes")
	}
}

func TestChallengeServiceRecoveryCodeRequiredActionKeepsEligibleRecoveryStep(t *testing.T) {
	service, _ := newTestChallengeServiceWithStore(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		PasswordMaxAttempts:             3,
		PasswordCooldownSeconds:         1,
		RecoveryMaxAttempts:             3,
		RecoveryCooldownSeconds:         1,
		WebAuthnChallengeTimeoutSeconds: 60,
	}, &testCompletionStore{recoveryAvailable: 1})

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionMFARecoveryCodesRegenerate),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-recovery-code-present",
		IdempotencyKey:       "idem-recovery-code-present",
	})
	if err != nil {
		t.Fatalf("StartChallenge() rejected eligible recovery code action: %v", err)
	}
	firstStepOfType(t, start, challengedomain.ChallengeTypePasswordVerification)
	firstStepOfType(t, start, challengedomain.ChallengeTypeRecoveryCodeVerification)
}

func TestChallengeServiceRejectsPasswordOnlyProofForPrivilegedMutation(t *testing.T) {
	service, _ := newTestChallengeService(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		WebAuthnChallengeTimeoutSeconds: 60,
	})
	tests := []struct {
		name             string
		action           challengedomain.BusinessAction
		operationBinding string
	}{
		{
			name:             "rbac_assign_user_roles",
			action:           challengedomain.BusinessActionRBACAssignUserRoles,
			operationBinding: "user:1001|roles:1,2",
		},
		{
			name:             "rbac_assign_menu_permissions",
			action:           challengedomain.BusinessActionRBACAssignMenuPermissions,
			operationBinding: "menu:9|permissions:10,12",
		},
		{
			name:             "config_sensitive_reveal",
			action:           challengedomain.BusinessActionConfigSensitiveReveal,
			operationBinding: "config:204|reveal",
		},
		{
			name:             "notification_delivery_content_view",
			action:           challengedomain.BusinessActionNotificationDeliveryContentView,
			operationBinding: "delivery:delivery-1|reason:INCIDENT|ticket:INC-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now().UTC()
			session := &challengedomain.ChallengeSession{
				ChallengeIdentifier:       "challenge-" + tt.name,
				IssuingServiceName:        "challenge-app",
				AudienceServiceNames:      []string{"challenge-app"},
				SubjectIdentifier:         "user:9001",
				FlowNonce:                 "flow-" + tt.name,
				BusinessAction:            string(tt.action),
				ChallengeState:            challengedomain.ChallengeStatePassed,
				AuthenticationMethodNames: []string{string(challengedomain.ChallengeTypePasswordVerification)},
				SessionContext: map[string]any{
					"operationBinding": tt.operationBinding,
				},
				CreatedAt: &now,
				ExpiresAt: timePointer(now.Add(time.Minute)),
			}
			_, proofToken, err := service.proofTokens.Issue(context.Background(), session)
			if err != nil {
				t.Fatalf("issue proof token: %v", err)
			}

			if _, err := service.VerifyProofToken(context.Background(), challengefacade.ProofTokenVerifyRequest{
				ProofToken:          proofToken,
				AudienceServiceName: "challenge-app",
				BusinessAction:      string(tt.action),
				FlowNonce:           "flow-" + tt.name,
				SubjectIdentifier:   "user:9001",
				OperationBinding:    tt.operationBinding,
			}); err == nil {
				t.Fatalf("VerifyProofToken() accepted password-only proof for %s", tt.action)
			}
		})
	}
}

func TestChallengeServiceNotificationDeliveryContentViewStartsAndVerifiesBoundAAL2Proof(t *testing.T) {
	service, _ := newTestChallengeServiceWithStore(t, config.ChallengeConfig{
		SessionTTLSeconds:               300,
		ProofTokenTTLMinSeconds:         60,
		ProofTokenTTLMaxSeconds:         300,
		EmailMaxAttempts:                3,
		EmailCooldownSeconds:            1,
		OTPMaxAttempts:                  3,
		OTPCooldownSeconds:              1,
		WebAuthnChallengeTimeoutSeconds: 60,
	}, &profileEmailTestStore{otpSecret: "JBSWY3DPEHPK3PXP"})
	const binding = "delivery:delivery-1|reason:INCIDENT|ticket:INC-1"
	if _, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "authorization-app",
		AudienceServiceNames: []string{"authorization-app"},
		BusinessAction:       string(challengedomain.BusinessActionNotificationDeliveryContentView),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-notification-diagnostic-missing-binding",
		IdempotencyKey:       "idem-notification-diagnostic-missing-binding",
	}); err == nil {
		t.Fatal("StartChallenge() accepted notification diagnostic view without an operation binding")
	}

	start, err := service.StartChallenge(context.Background(), challengefacade.StartChallengeRequest{
		IssuingServiceName:   "authorization-app",
		AudienceServiceNames: []string{"authorization-app"},
		BusinessAction:       string(challengedomain.BusinessActionNotificationDeliveryContentView),
		SubjectIdentifier:    "user:9001",
		FlowNonce:            "flow-notification-diagnostic",
		IdempotencyKey:       "idem-notification-diagnostic",
		ExtensionContext:     map[string]any{"operationBinding": binding},
	})
	if err != nil {
		t.Fatalf("StartChallenge() rejected notification diagnostic view: %v", err)
	}
	if start.RequiredAssuranceLevel != "AAL2" || start.ResolvedAssuranceLevel != "AAL2" {
		t.Fatalf("expected AAL2 notification diagnostic challenge, got %+v", start)
	}
	if len(start.Steps) == 0 {
		t.Fatalf("expected a strong notification diagnostic challenge step, got %+v", start)
	}
	for _, step := range start.Steps {
		if step.ChallengeType == string(challengedomain.ChallengeTypePasswordVerification) ||
			step.ChallengeType == string(challengedomain.ChallengeTypeImageCaptcha) ||
			step.ChallengeType == string(challengedomain.ChallengeTypeEmailOneTimePassword) {
			t.Fatalf("notification diagnostic challenge downgraded to %s", step.ChallengeType)
		}
	}

	proofToken := issueProofTokenForTest(t, service, "notification-diagnostic-proof", challengedomain.BusinessActionNotificationDeliveryContentView,
		[]challengedomain.ChallengeType{challengedomain.ChallengeTypeTimeBasedOneTimePassword}, binding)
	claims, err := service.VerifyProofToken(context.Background(), challengefacade.ProofTokenVerifyRequest{
		ProofToken:          proofToken,
		AudienceServiceName: "challenge-app",
		BusinessAction:      string(challengedomain.BusinessActionNotificationDeliveryContentView),
		FlowNonce:           "flow-notification-diagnostic-proof",
		SubjectIdentifier:   "user:1001",
		OperationBinding:    binding,
		ConsumeOnce:         true,
	})
	if err != nil {
		t.Fatalf("VerifyProofToken() rejected the bound AAL2 notification diagnostic proof: %v", err)
	}
	if claims == nil || claims.BusinessAction != string(challengedomain.BusinessActionNotificationDeliveryContentView) || claims.OperationBinding != binding {
		t.Fatalf("unexpected notification diagnostic proof claims: %+v", claims)
	}
}

func issueProofTokenForTest(t *testing.T, service *ChallengeService, challengeIdentifier string, action challengedomain.BusinessAction, methods []challengedomain.ChallengeType, operationBinding string) string {
	t.Helper()
	now := time.Now().UTC()
	methodNames := make([]string, 0, len(methods))
	for _, method := range methods {
		methodNames = append(methodNames, string(method))
	}
	session := &challengedomain.ChallengeSession{
		ChallengeIdentifier:       challengeIdentifier,
		IssuingServiceName:        "challenge-app",
		AudienceServiceNames:      []string{"challenge-app"},
		SubjectIdentifier:         "user:1001",
		FlowNonce:                 "flow-" + challengeIdentifier,
		BusinessAction:            string(action),
		ChallengeState:            challengedomain.ChallengeStatePassed,
		AuthenticationMethodNames: methodNames,
		CreatedAt:                 &now,
		ExpiresAt:                 timePointer(now.Add(time.Minute)),
	}
	if operationBinding != "" {
		session.SessionContext = map[string]any{"operationBinding": operationBinding}
	}
	_, proofToken, err := service.proofTokens.Issue(context.Background(), session)
	if err != nil {
		t.Fatalf("issue proof token: %v", err)
	}
	return proofToken
}

type testSessionRepository struct {
	sessions      map[string]*challengedomain.ChallengeSession
	idempotency   map[string]string
	submitLocks   map[string]string
	proofConsumed map[string]bool
	throttleCount map[string]int
	throttleLocks map[string]bool
}

func newTestSessionRepository() *testSessionRepository {
	return &testSessionRepository{
		sessions:      make(map[string]*challengedomain.ChallengeSession),
		idempotency:   make(map[string]string),
		submitLocks:   make(map[string]string),
		proofConsumed: make(map[string]bool),
		throttleCount: make(map[string]int),
		throttleLocks: make(map[string]bool),
	}
}

func (r *testSessionRepository) SaveSession(_ context.Context, session *challengedomain.ChallengeSession) error {
	copy := *session
	copy.Steps = append([]challengedomain.ChallengeStep(nil), session.Steps...)
	copy.AudienceServiceNames = append([]string(nil), session.AudienceServiceNames...)
	copy.AuthenticationMethodNames = append([]string(nil), session.AuthenticationMethodNames...)
	if session.SessionContext != nil {
		copy.SessionContext = make(map[string]any, len(session.SessionContext))
		for key, value := range session.SessionContext {
			copy.SessionContext[key] = value
		}
	}
	r.sessions[session.ChallengeIdentifier] = &copy
	return nil
}

func (r *testSessionRepository) GetSession(_ context.Context, challengeIdentifier string) (*challengedomain.ChallengeSession, error) {
	if session, ok := r.sessions[challengeIdentifier]; ok {
		copy := *session
		copy.Steps = append([]challengedomain.ChallengeStep(nil), session.Steps...)
		copy.AudienceServiceNames = append([]string(nil), session.AudienceServiceNames...)
		copy.AuthenticationMethodNames = append([]string(nil), session.AuthenticationMethodNames...)
		if session.SessionContext != nil {
			copy.SessionContext = make(map[string]any, len(session.SessionContext))
			for key, value := range session.SessionContext {
				copy.SessionContext[key] = value
			}
		}
		return &copy, nil
	}
	return nil, nil
}

func (r *testSessionRepository) BindIdempotencyKey(_ context.Context, idempotencyKey, challengeIdentifier string, _ time.Duration) error {
	r.idempotency[idempotencyKey] = challengeIdentifier
	return nil
}

func (r *testSessionRepository) GetSessionByIdempotencyKey(_ context.Context, idempotencyKey string) (string, bool, error) {
	value, ok := r.idempotency[idempotencyKey]
	return value, ok, nil
}

func (r *testSessionRepository) AcquireSubmitLock(_ context.Context, challengeIdentifier string, _ time.Duration) (string, bool, error) {
	if _, exists := r.submitLocks[challengeIdentifier]; exists {
		return "", false, nil
	}
	token := "lock-" + challengeIdentifier
	r.submitLocks[challengeIdentifier] = token
	return token, true, nil
}

func (r *testSessionRepository) ReleaseSubmitLock(_ context.Context, challengeIdentifier, token string) error {
	if r.submitLocks[challengeIdentifier] == token {
		delete(r.submitLocks, challengeIdentifier)
	}
	return nil
}

func (r *testSessionRepository) MarkProofConsumed(_ context.Context, tokenUniqueIdentifier, audience string, _ time.Duration) (bool, error) {
	key := tokenUniqueIdentifier + "|" + audience
	if r.proofConsumed[key] {
		return false, nil
	}
	r.proofConsumed[key] = true
	return true, nil
}

func (r *testSessionRepository) CheckLocked(_ context.Context, keys []challengedomain.ChallengeThrottleKey) (*challengedomain.ChallengeThrottleDecision, error) {
	for _, key := range keys {
		cacheKey := testThrottleKey(key)
		if r.throttleLocks[cacheKey] {
			return &challengedomain.ChallengeThrottleDecision{
				Locked:           true,
				Dimension:        key.Dimension,
				RemainingSeconds: 900,
			}, nil
		}
	}
	return nil, nil
}

func (r *testSessionRepository) RecordFailure(_ context.Context, keys []challengedomain.ChallengeThrottleKey, maxFailures int, _, _ time.Duration) (*challengedomain.ChallengeThrottleDecision, error) {
	var locked *challengedomain.ChallengeThrottleDecision
	for _, key := range keys {
		cacheKey := testThrottleKey(key)
		r.throttleCount[cacheKey]++
		if maxFailures > 0 && r.throttleCount[cacheKey] >= maxFailures {
			r.throttleLocks[cacheKey] = true
			if locked == nil {
				locked = &challengedomain.ChallengeThrottleDecision{
					Locked:           true,
					Dimension:        key.Dimension,
					FailureCount:     r.throttleCount[cacheKey],
					RemainingSeconds: 900,
				}
			}
		}
	}
	return locked, nil
}

func (r *testSessionRepository) ClearFailures(_ context.Context, keys []challengedomain.ChallengeThrottleKey) error {
	for _, key := range keys {
		cacheKey := testThrottleKey(key)
		delete(r.throttleCount, cacheKey)
		delete(r.throttleLocks, cacheKey)
	}
	return nil
}

func testThrottleKey(key challengedomain.ChallengeThrottleKey) string {
	return key.Dimension + "|" + key.Value
}

func (r *testSessionRepository) mustSession(t *testing.T, challengeIdentifier string) *challengedomain.ChallengeSession {
	t.Helper()
	session, ok := r.sessions[challengeIdentifier]
	if !ok || session == nil {
		t.Fatalf("expected session %s", challengeIdentifier)
	}
	return session
}

func failLoginCaptcha(t *testing.T, service *ChallengeService, subject, idempotencyKey, ipAddress, deviceIdentifier string) *challengefacade.RespondChallengeResponse {
	t.Helper()
	start := startLoginCaptcha(t, service, subject, idempotencyKey, ipAddress, deviceIdentifier)
	step := firstStepOfType(t, start, challengedomain.ChallengeTypeImageCaptcha)
	result, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: step.StepIdentifier,
		Payload: map[string]any{
			"captchaCode": "wrong",
		},
	})
	if err != nil {
		t.Fatalf("respond login challenge: %v", err)
	}
	return result
}

func startLoginCaptcha(t *testing.T, service *ChallengeService, subject, idempotencyKey, ipAddress, deviceIdentifier string) *challengefacade.StartChallengeResponse {
	t.Helper()
	start, err := service.StartChallenge(context.Background(), loginCaptchaStartRequest(subject, idempotencyKey, ipAddress, deviceIdentifier))
	if err != nil {
		t.Fatalf("start login challenge: %v", err)
	}
	return start
}

func loginCaptchaStartRequest(subject, idempotencyKey, ipAddress, deviceIdentifier string) challengefacade.StartChallengeRequest {
	return challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionLogin),
		SubjectIdentifier:    "login:" + subject,
		FlowNonce:            "flow-" + idempotencyKey,
		IdempotencyKey:       idempotencyKey,
		RiskContext: &challengefacade.RiskContext{
			IPAddress:        ipAddress,
			DeviceIdentifier: deviceIdentifier,
		},
		ExpectedChallengeTypes: []string{string(challengedomain.ChallengeTypeImageCaptcha)},
	}
}

func failPrivilegedEmailOTP(t *testing.T, service *ChallengeService, subjectIdentifier, idempotencyKey, operationBinding string) *challengefacade.RespondChallengeResponse {
	t.Helper()
	start := startPrivilegedEmailChallenge(t, service, subjectIdentifier, idempotencyKey, operationBinding)
	step := firstStepOfType(t, start, challengedomain.ChallengeTypeEmailOneTimePassword)
	result, err := service.Respond(context.Background(), start.ChallengeIdentifier, challengefacade.RespondChallengeRequest{
		StepIdentifier: step.StepIdentifier,
		Payload: map[string]any{
			"oneTimePassword": "000000",
		},
	})
	if err != nil {
		t.Fatalf("respond privileged challenge: %v", err)
	}
	return result
}

func startPrivilegedEmailChallenge(t *testing.T, service *ChallengeService, subjectIdentifier, idempotencyKey, operationBinding string) *challengefacade.StartChallengeResponse {
	t.Helper()
	start, err := service.StartChallenge(context.Background(), privilegedEmailStartRequest(subjectIdentifier, idempotencyKey, operationBinding))
	if err != nil {
		t.Fatalf("start privileged challenge: %v", err)
	}
	return start
}

func privilegedEmailStartRequest(subjectIdentifier, idempotencyKey, operationBinding string) challengefacade.StartChallengeRequest {
	return challengefacade.StartChallengeRequest{
		IssuingServiceName:   "system-admin",
		AudienceServiceNames: []string{"system-admin"},
		BusinessAction:       string(challengedomain.BusinessActionConfigSensitiveReveal),
		SubjectIdentifier:    subjectIdentifier,
		FlowNonce:            "flow-" + idempotencyKey,
		IdempotencyKey:       idempotencyKey,
		RiskContext: &challengefacade.RiskContext{
			IPAddress:        "10.8.0." + idempotencyKey[len(idempotencyKey)-1:],
			DeviceIdentifier: "device-" + idempotencyKey,
		},
		ExtensionContext: map[string]any{
			"operationBinding": operationBinding,
		},
	}
}

func assertChallengeThrottledError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected challenge throttle error")
	}
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeRateLimited {
		t.Fatalf("expected rate limited error for challenge throttle, got %T %v", err, err)
	}
	details, ok := appErr.Details().(map[string]any)
	if !ok {
		t.Fatalf("expected challenge throttle details, got %+v", appErr.Details())
	}
	if _, exists := details["errorCode"]; exists {
		t.Fatalf("challenge throttle details must omit errorCode, got %+v", details)
	}
	if details["cooldownSeconds"] == nil {
		t.Fatalf("expected cooldownSeconds details, got %+v", details)
	}
}

func firstStepOfType(t *testing.T, response *challengefacade.StartChallengeResponse, challengeType challengedomain.ChallengeType) challengefacade.ChallengeStepVO {
	t.Helper()
	for _, step := range response.Steps {
		if step.ChallengeType == string(challengeType) {
			return step
		}
	}
	t.Fatalf("expected step %s in %+v", challengeType, response.Steps)
	return challengefacade.ChallengeStepVO{}
}

func newTestChallengeService(t *testing.T, cfg config.ChallengeConfig) (*ChallengeService, *testSessionRepository) {
	t.Helper()
	return newTestChallengeServiceWithStore(t, cfg, &testCompletionStore{})
}

func newTestChallengeServiceWithStore(t *testing.T, cfg config.ChallengeConfig, store challengeprovider.SubjectCredentialStore) (*ChallengeService, *testSessionRepository) {
	t.Helper()
	return newTestChallengeServiceWithStoreAndEmailSender(t, cfg, store, fakeEmailSender{})
}

func newTestChallengeServiceWithStoreAndEmailSender(t *testing.T, cfg config.ChallengeConfig, store challengeprovider.SubjectCredentialStore, sender emailinfra.Sender) (*ChallengeService, *testSessionRepository) {
	t.Helper()
	randomService := randominfra.New(config.RandomConfig{TokenLength: 16, NonceLength: 16, CodeLength: 6})
	passwordService, err := passwordinfra.New(config.PasswordConfig{
		Algorithm: "bcrypt",
		Bcrypt:    config.BcryptPasswordConfig{Cost: 4},
	})
	if err != nil {
		t.Fatalf("new password service: %v", err)
	}
	stepService := NewStepService(challengeprovider.NewRegistry(
		challengeprovider.NewImageCaptchaChallengeStepProvider(challengeinfra.NewCaptchaService(randomService)),
		challengeprovider.NewPasswordChallengeStepProvider(passwordService, store),
		challengeprovider.NewWebAuthnPasskeyAssertionStepProvider(
			challengeinfra.NewWebAuthnService(randomService),
			store,
			"localhost",
			[]string{"http://127.0.0.1:5177"},
			300,
		),
		challengeprovider.NewTimeBasedOtpChallengeStepProvider(
			totpinfra.New(),
			store,
			challengeprovider.TimeBasedOtpSettings{
				IssuerName:          "SevenFramework",
				AllowedDriftWindows: 1,
			},
		),
		challengeprovider.NewEmailOtpChallengeStepProvider(challengeinfra.NewEmailOTPService(randomService, sender), store),
		challengeprovider.NewRecoveryCodeChallengeStepProvider(store),
	))
	repo := newTestSessionRepository()
	jwtService := newTestJWTService(t)
	proofTokens := challengeinfra.NewProofTokenService(
		jwtService,
		repo,
		time.Duration(cfg.ProofTokenTTLMinSeconds)*time.Second,
		time.Duration(cfg.ProofTokenTTLMaxSeconds)*time.Second,
	)
	completion := NewCompletionHandler(store, max(1, cfg.RecoveryBatchSize))
	return NewChallengeService(cfg, repo, repo, stepService, completion, proofTokens), repo
}

func newTestJWTService(t *testing.T) *jwtinfra.Service {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "jwt-private.pem")
	publicPath := filepath.Join(dir, "jwt-public.pem")
	writePEMFile(t, privatePath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey))
	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	writePEMFile(t, publicPath, "PUBLIC KEY", publicDER)
	keys, err := keyring.NewLocalProvider(config.KeysConfig{
		Provider: "local",
		JWT: config.JWTKeysConfig{
			Algorithm: "RS256",
			Active: config.JWTKeySourceConfig{
				KID:              "kid-active",
				PrivateKeySource: "file:" + privatePath,
				PublicKeySource:  "file:" + publicPath,
			},
		},
	})
	if err != nil {
		t.Fatalf("new local key provider: %v", err)
	}
	service, err := jwtinfra.New(keys, "RS256")
	if err != nil {
		t.Fatalf("new jwt service: %v", err)
	}
	return service
}

func writePEMFile(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	block := &pem.Block{Type: blockType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write pem file: %v", err)
	}
}

type testCompletionStore struct {
	recoveryAvailable int
}

func (s *testCompletionStore) FindEnabledOtpBinding(ctx context.Context, session *challengedomain.ChallengeSession) (*challengedomain.OtpBindingRecord, error) {
	return nil, nil
}
func (s *testCompletionStore) FindEnabledOtpSecret(ctx context.Context, session *challengedomain.ChallengeSession) (string, error) {
	return "", nil
}
func (s *testCompletionStore) FindPasswordCredential(ctx context.Context, session *challengedomain.ChallengeSession) (string, error) {
	return "", nil
}
func (s *testCompletionStore) ListPasskeys(ctx context.Context, session *challengedomain.ChallengeSession) ([]challengedomain.PasskeyRegistration, error) {
	return nil, nil
}
func (s *testCompletionStore) FindPasskey(ctx context.Context, credentialKey string) (*challengedomain.PasskeyRegistration, error) {
	return nil, nil
}
func (s *testCompletionStore) UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error {
	return nil
}
func (s *testCompletionStore) ConsumeRecoveryCode(ctx context.Context, session *challengedomain.ChallengeSession, recoveryCode string, usedAt time.Time) (bool, error) {
	return false, nil
}
func (s *testCompletionStore) CountAvailableRecoveryCodes(ctx context.Context, session *challengedomain.ChallengeSession) (int, error) {
	return s.recoveryAvailable, nil
}
func (s *testCompletionStore) CompleteTotpBinding(ctx context.Context, session *challengedomain.ChallengeSession, plainSecret string, verifiedAt time.Time, recoveryBatchSize int) error {
	return nil
}
func (s *testCompletionStore) CompletePasskeyBinding(ctx context.Context, session *challengedomain.ChallengeSession, registration challengedomain.PasskeyRegistration, disableExisting bool, verifiedAt time.Time, recoveryBatchSize int) error {
	return nil
}
func (s *testCompletionStore) ResolveAccountName(ctx context.Context, session *challengedomain.ChallengeSession) (string, error) {
	return "alice", nil
}
func (s *testCompletionStore) ResolveTargetEmail(ctx context.Context, session *challengedomain.ChallengeSession) (string, error) {
	return "alice@example.com", nil
}

type profileEmailTestStore struct {
	targetEmail  string
	otpSecret    string
	otpSecretErr error
}

func (s *profileEmailTestStore) FindEnabledOtpBinding(context.Context, *challengedomain.ChallengeSession) (*challengedomain.OtpBindingRecord, error) {
	if s.otpSecret != "" {
		return &challengedomain.OtpBindingRecord{UserID: 9001, SecretEncrypted: "test-fixture"}, nil
	}
	return nil, nil
}

func (s *profileEmailTestStore) FindEnabledOtpSecret(context.Context, *challengedomain.ChallengeSession) (string, error) {
	if s.otpSecretErr != nil {
		return "", s.otpSecretErr
	}
	return s.otpSecret, nil
}

func (s *profileEmailTestStore) FindPasswordCredential(context.Context, *challengedomain.ChallengeSession) (string, error) {
	return "", nil
}

func (s *profileEmailTestStore) ListPasskeys(context.Context, *challengedomain.ChallengeSession) ([]challengedomain.PasskeyRegistration, error) {
	return nil, nil
}

func (s *profileEmailTestStore) FindPasskey(context.Context, string) (*challengedomain.PasskeyRegistration, error) {
	return nil, nil
}

func (s *profileEmailTestStore) UpdatePasskeyUsage(context.Context, string, int64, time.Time) error {
	return nil
}

func (s *profileEmailTestStore) ConsumeRecoveryCode(context.Context, *challengedomain.ChallengeSession, string, time.Time) (bool, error) {
	return false, nil
}

func (s *profileEmailTestStore) CompleteTotpBinding(context.Context, *challengedomain.ChallengeSession, string, time.Time, int) error {
	return nil
}

func (s *profileEmailTestStore) CompletePasskeyBinding(context.Context, *challengedomain.ChallengeSession, challengedomain.PasskeyRegistration, bool, time.Time, int) error {
	return nil
}

func (s *profileEmailTestStore) ResolveAccountName(context.Context, *challengedomain.ChallengeSession) (string, error) {
	return "alice", nil
}

func (s *profileEmailTestStore) ResolveTargetEmail(context.Context, *challengedomain.ChallengeSession) (string, error) {
	return s.targetEmail, nil
}

type fakeEmailSender struct{}

func (fakeEmailSender) SendChallengeOTP(context.Context, emailinfra.ChallengeOTPRequest) error {
	return nil
}

type countingEmailSender struct {
	count *int32
}

func (s countingEmailSender) SendChallengeOTP(context.Context, emailinfra.ChallengeOTPRequest) error {
	if s.count != nil {
		atomic.AddInt32(s.count, 1)
	}
	return nil
}
