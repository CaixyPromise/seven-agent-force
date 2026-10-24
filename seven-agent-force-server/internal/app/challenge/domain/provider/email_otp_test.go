package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	challengeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/challenge/infrastructure"
	cacheinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache"
	emailinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/email"
	randominfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/random"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestEmailOtpVerifyAcceptsEmailCode(t *testing.T) {
	provider := NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(nil, nil),
		&fakeSubjectStore{},
	)
	provider.email = &challengeinfra.EmailOTPService{}
	step := &domain.ChallengeStep{StepIdentifier: "step-email", StepPurpose: string(domain.ChallengeStepPurposeVerifyOld)}
	session := &domain.ChallengeSession{
		SessionContext: map[string]any{
			"email.otp.code.step-email": "123456",
		},
	}
	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"emailCode": "123456"})
	if err != nil {
		t.Fatalf("verify email otp: %v", err)
	}
	if !ok {
		t.Fatal("expected email otp verification to succeed")
	}
	ok, err = provider.Verify(context.Background(), session, step, map[string]any{"emailCode": "123456"})
	if err != nil {
		t.Fatalf("verify replayed email otp: %v", err)
	}
	if ok {
		t.Fatal("expected consumed email otp code replay to fail")
	}
}

func TestEmailOtpEligibleCachesTargetEmailAndRejectsMissingTarget(t *testing.T) {
	provider := NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(nil, nil),
		&emailSubjectStore{targetEmail: " alice@example.com "},
	)
	session := &domain.ChallengeSession{}
	step := &domain.ChallengeStep{StepIdentifier: "step-email", StepPurpose: string(domain.ChallengeStepPurposeVerifyOld)}

	eligible, err := provider.Eligible(context.Background(), session, step)
	if err != nil {
		t.Fatalf("eligible email otp: %v", err)
	}
	if !eligible {
		t.Fatal("expected subject with target email to be eligible")
	}
	if session.SessionContext["email.target"] != "alice@example.com" {
		t.Fatalf("expected target email to be trimmed and cached, got %#v", session.SessionContext["email.target"])
	}

	provider = NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(nil, nil),
		&emailSubjectStore{},
	)
	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{}, step)
	if err != nil {
		t.Fatalf("eligible missing email otp target: %v", err)
	}
	if eligible {
		t.Fatal("expected missing target email to be ineligible")
	}

	expected := errors.New("target email unavailable")
	provider = NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(nil, nil),
		&emailSubjectStore{err: expected},
	)
	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{}, step)
	if !errors.Is(err, expected) {
		t.Fatalf("expected target email lookup error, got %v", err)
	}
	if eligible {
		t.Fatal("expected errored target email lookup to be ineligible")
	}
}

func TestEmailOtpPrepareSendsCodeAndMasksTargetEmail(t *testing.T) {
	sender := &recordingEmailSender{}
	provider := NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(randominfra.New(config.RandomConfig{CodeLength: 6}), sender),
		&emailSubjectStore{targetEmail: "alice@example.com"},
	)
	session := &domain.ChallengeSession{}
	step := &domain.ChallengeStep{StepIdentifier: "step-email", StepPurpose: string(domain.ChallengeStepPurposeVerifyOld)}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare email otp: %v", err)
	}
	if len(sender.requests) != 1 {
		t.Fatalf("expected one email otp delivery, got %d", len(sender.requests))
	}
	request := sender.requests[0]
	if request.ToEmail != "alice@example.com" {
		t.Fatalf("unexpected email target: %q", request.ToEmail)
	}
	if len(request.Code) != 6 {
		t.Fatalf("expected six digit code, got %q", request.Code)
	}
	if request.Scene != "LOGIN_UNLOCK" {
		t.Fatalf("unexpected email scene: %q", request.Scene)
	}
	if request.TTL != 5*time.Minute {
		t.Fatalf("unexpected email ttl: %s", request.TTL)
	}
	if session.SessionContext[emailOTPContextKey(step)] != request.Code {
		t.Fatalf("expected session to store generated code, got %#v", session.SessionContext[emailOTPContextKey(step)])
	}
	if session.SessionContext["email.target"] != "alice@example.com" {
		t.Fatalf("expected session target email, got %#v", session.SessionContext["email.target"])
	}
	if step.UserInterfaceHints["emailMasked"] != "a***e@example.com" {
		t.Fatalf("expected masked email hint, got %#v", step.UserInterfaceHints["emailMasked"])
	}
	if step.UserInterfaceHints["deliveryState"] != "SENT" || step.UserInterfaceHints["refreshable"] != true {
		t.Fatalf("expected sent refreshable hints, got %#v", step.UserInterfaceHints)
	}
	if mapContainsString(step.UserInterfaceHints, request.Code) {
		t.Fatalf("email otp provider must not expose raw otp in UI hints: %#v", step.UserInterfaceHints)
	}
	if mapContainsString(step.UserInterfaceHints, request.ToEmail) {
		t.Fatalf("email otp provider must not expose raw target email in UI hints: %#v", step.UserInterfaceHints)
	}
}

func TestEmailOtpRefreshKeepsOnlyLatestCodeAndDoesNotExposeRawOtp(t *testing.T) {
	sender := &recordingEmailSender{}
	provider := NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(randominfra.New(config.RandomConfig{CodeLength: 16}), sender),
		&emailSubjectStore{targetEmail: "alice@example.com"},
	)
	session := &domain.ChallengeSession{}
	step := &domain.ChallengeStep{StepIdentifier: "step-email", StepPurpose: string(domain.ChallengeStepPurposeVerifyOld)}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare email otp: %v", err)
	}
	firstCode := sender.requests[0].Code
	if err := provider.Refresh(context.Background(), session, step); err != nil {
		t.Fatalf("refresh email otp: %v", err)
	}
	if len(sender.requests) != 2 {
		t.Fatalf("expected prepare plus refresh to send two codes, got %d", len(sender.requests))
	}
	secondCode := sender.requests[1].Code
	if session.SessionContext[emailOTPContextKey(step)] != secondCode {
		t.Fatalf("expected session to keep latest refreshed code, got %#v", session.SessionContext[emailOTPContextKey(step)])
	}
	if firstCode == secondCode {
		t.Fatalf("expected refresh to generate a new otp code, both deliveries were %q", firstCode)
	}
	if mapContainsString(step.UserInterfaceHints, firstCode) || mapContainsString(step.UserInterfaceHints, secondCode) {
		t.Fatalf("email otp provider must not expose raw otp in UI hints after refresh: %#v", step.UserInterfaceHints)
	}
	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"emailCode": firstCode})
	if err != nil {
		t.Fatalf("verify stale email otp after refresh: %v", err)
	}
	if ok {
		t.Fatal("expected stale email otp code from before refresh to fail")
	}
	ok, err = provider.Verify(context.Background(), session, step, map[string]any{"emailCode": secondCode})
	if err != nil {
		t.Fatalf("verify latest email otp after refresh: %v", err)
	}
	if !ok {
		t.Fatal("expected latest refreshed email otp code to verify")
	}
}

func TestEmailOtpPrepareWithoutTargetDoesNotSendCode(t *testing.T) {
	sender := &recordingEmailSender{}
	provider := NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(randominfra.New(config.RandomConfig{CodeLength: 6}), sender),
		&emailSubjectStore{},
	)
	session := &domain.ChallengeSession{}
	step := &domain.ChallengeStep{StepIdentifier: "step-email", StepPurpose: string(domain.ChallengeStepPurposeVerifyOld)}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare email otp without target: %v", err)
	}
	if len(sender.requests) != 0 {
		t.Fatalf("expected no email delivery without target, got %d", len(sender.requests))
	}
	if _, ok := session.SessionContext[emailOTPContextKey(step)]; ok {
		t.Fatalf("expected no otp code in session without target, got %#v", session.SessionContext)
	}
	if step.UserInterfaceHints["emailMasked"] != "" {
		t.Fatalf("expected empty masked email hint, got %#v", step.UserInterfaceHints["emailMasked"])
	}
}

func TestEmailOtpVerifyAfterCacheRoundTrip(t *testing.T) {
	codec, err := cacheinfra.NewCodec("sonic")
	if err != nil {
		t.Fatalf("new codec: %v", err)
	}
	payload := []byte(`{
		"ChallengeIdentifier":"challenge-1",
		"SessionContext":{"email.otp.code.step-email":"654321"},
		"Steps":[{"StepIdentifier":"step-email","ChallengeType":"EMAIL_ONE_TIME_PASSWORD","StepPurpose":"VERIFY_OLD","StepState":"IN_PROGRESS"}]
	}`)
	var session domain.ChallengeSession
	if err := codec.Unmarshal(payload, &session); err != nil {
		t.Fatalf("unmarshal challenge session: %v", err)
	}
	provider := NewEmailOtpChallengeStepProvider(
		challengeinfra.NewEmailOTPService(nil, nil),
		&fakeSubjectStore{},
	)
	provider.email = &challengeinfra.EmailOTPService{}
	ok, err := provider.Verify(context.Background(), &session, &session.Steps[0], map[string]any{"emailCode": "654321"})
	if err != nil {
		t.Fatalf("verify email otp after cache roundtrip: %v", err)
	}
	if !ok {
		t.Fatal("expected email otp verification to survive cache roundtrip")
	}
}

type emailSubjectStore struct {
	fakeSubjectStore
	targetEmail string
	err         error
}

func (s *emailSubjectStore) ResolveTargetEmail(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	_ = ctx
	_ = session
	if s.err != nil {
		return "", s.err
	}
	return s.targetEmail, nil
}

type recordingEmailSender struct {
	requests []emailinfra.ChallengeOTPRequest
}

func (s *recordingEmailSender) SendChallengeOTP(ctx context.Context, request emailinfra.ChallengeOTPRequest) error {
	_ = ctx
	s.requests = append(s.requests, request)
	return nil
}
