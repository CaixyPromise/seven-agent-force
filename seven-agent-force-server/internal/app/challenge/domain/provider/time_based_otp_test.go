package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	totpinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/totp"
	"github.com/pquerna/otp"
	libtotp "github.com/pquerna/otp/totp"
)

type fakeSubjectStore struct {
	otpBinding      *domain.OtpBindingRecord
	otpSecret       string
	otpSecretErr    error
	consumeResults  []bool
	accountName     string
	lastSessionID   string
	completedSecret string
	recoveryBatch   int
	recoveryCount   int
}

func (f *fakeSubjectStore) FindEnabledOtpBinding(ctx context.Context, session *domain.ChallengeSession) (*domain.OtpBindingRecord, error) {
	if session != nil {
		f.lastSessionID = session.SubjectIdentifier
	}
	return f.otpBinding, nil
}

func (f *fakeSubjectStore) FindEnabledOtpSecret(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	if session != nil {
		f.lastSessionID = session.SubjectIdentifier
	}
	if f.otpSecretErr != nil {
		return "", f.otpSecretErr
	}
	return f.otpSecret, nil
}

func (f *fakeSubjectStore) ConsumeRecoveryCode(ctx context.Context, session *domain.ChallengeSession, recoveryCode string, usedAt time.Time) (bool, error) {
	if len(f.consumeResults) == 0 {
		return false, nil
	}
	result := f.consumeResults[0]
	f.consumeResults = f.consumeResults[1:]
	return result, nil
}

func (f *fakeSubjectStore) CountAvailableRecoveryCodes(ctx context.Context, session *domain.ChallengeSession) (int, error) {
	if session != nil {
		f.lastSessionID = session.SubjectIdentifier
	}
	return f.recoveryCount, nil
}

func (f *fakeSubjectStore) CompleteTotpBinding(ctx context.Context, session *domain.ChallengeSession, plainSecret string, verifiedAt time.Time, recoveryBatchSize int) error {
	f.completedSecret = plainSecret
	f.recoveryBatch = recoveryBatchSize
	return nil
}

func (f *fakeSubjectStore) CompletePasskeyBinding(ctx context.Context, session *domain.ChallengeSession, registration domain.PasskeyRegistration, disableExisting bool, verifiedAt time.Time, recoveryBatchSize int) error {
	f.recoveryBatch = recoveryBatchSize
	return nil
}

func (f *fakeSubjectStore) ListPasskeys(ctx context.Context, session *domain.ChallengeSession) ([]domain.PasskeyRegistration, error) {
	return nil, nil
}

func (f *fakeSubjectStore) FindPasskey(ctx context.Context, credentialKey string) (*domain.PasskeyRegistration, error) {
	return nil, nil
}

func (f *fakeSubjectStore) FindPasswordCredential(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	return "", nil
}

func (f *fakeSubjectStore) UpdatePasskeyUsage(ctx context.Context, credentialKey string, signCount int64, usedAt time.Time) error {
	return nil
}

func (f *fakeSubjectStore) ResolveAccountName(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	if f.accountName != "" {
		return f.accountName, nil
	}
	return "user", nil
}
func (f *fakeSubjectStore) ResolveTargetEmail(ctx context.Context, session *domain.ChallengeSession) (string, error) {
	return "", nil
}
func TestTimeBasedOtpPrepareUsesDynamicIssuerAndAccountName(t *testing.T) {
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{accountName: "alice@example.com"},
		TimeBasedOtpSettings{IssuerName: "Seven Security", AllowedDriftWindows: 1},
	)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice@example.com",
	}
	step := &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeRegisterNew),
	}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare otp step: %v", err)
	}

	if step.UserInterfaceHints["issuer"] != "Seven Security" {
		t.Fatalf("unexpected issuer: %+v", step.UserInterfaceHints["issuer"])
	}
	if step.UserInterfaceHints["accountName"] != "alice@example.com" {
		t.Fatalf("unexpected account name: %+v", step.UserInterfaceHints["accountName"])
	}
	otpauthURL, _ := step.UserInterfaceHints["otpauthUrl"].(string)
	if !strings.HasPrefix(otpauthURL, "otpauth://totp/") {
		t.Fatalf("unexpected otpauth url: %s", otpauthURL)
	}
	if !strings.Contains(otpauthURL, "Seven%20Security:alice%40example.com") {
		t.Fatalf("expected encoded label, got %s", otpauthURL)
	}
	if !strings.Contains(otpauthURL, "issuer=Seven%20Security") {
		t.Fatalf("expected encoded issuer query, got %s", otpauthURL)
	}
	if _, ok := session.SessionContext["otp.pendingSecretPlain"]; !ok {
		t.Fatalf("expected pending secret in session context")
	}
}

func TestTimeBasedOtpVerifyAcceptsCurrentWindowOTP(t *testing.T) {
	totpService := totpinfra.New()
	store := &fakeSubjectStore{
		otpSecret: "JBSWY3DPEHPK3PXP",
	}
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpService,
		store,
		TimeBasedOtpSettings{AllowedDriftWindows: 1},
	)

	now := time.Now()
	code, err := libtotp.GenerateCodeCustom("JBSWY3DPEHPK3PXP", now, libtotp.ValidateOpts{
		Period:    totpinfra.DefaultPeriod,
		Skew:      0,
		Digits:    totpinfra.DefaultDigits,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	ok, err := provider.Verify(context.Background(), &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
	}, &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
	}, map[string]any{"oneTimePassword": code})
	if err != nil {
		t.Fatalf("verify otp code: %v", err)
	}
	if !ok {
		t.Fatalf("expected otp verification to succeed")
	}
	if store.lastSessionID != "login:alice" {
		t.Fatalf("expected provider to load binding via subject store, got %s", store.lastSessionID)
	}
}

func TestTimeBasedOtpProviderDoesNotPersistSubmittedCode(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	code, err := libtotp.GenerateCodeCustom(secret, time.Now(), libtotp.ValidateOpts{
		Period:    totpinfra.DefaultPeriod,
		Skew:      0,
		Digits:    totpinfra.DefaultDigits,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{otpSecret: secret},
		TimeBasedOtpSettings{AllowedDriftWindows: 1},
	)
	session := &domain.ChallengeSession{
		SubjectIdentifier: "login:alice",
		SessionContext:    map[string]any{},
	}
	step := &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeVerifyOld),
	}
	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare verify-old otp step: %v", err)
	}
	ok, err := provider.Verify(context.Background(), session, step, map[string]any{"oneTimePassword": code})
	if err != nil {
		t.Fatalf("verify otp code: %v", err)
	}
	if !ok {
		t.Fatal("expected otp verification to succeed")
	}
	if mapContainsString(session.SessionContext, code) {
		t.Fatalf("TOTP provider must not persist submitted code in session context: %#v", session.SessionContext)
	}
	if mapContainsString(step.UserInterfaceHints, code) {
		t.Fatalf("TOTP provider must not expose submitted code in hints: %#v", step.UserInterfaceHints)
	}
}

func TestTimeBasedOtpPrepareVerifyOldDoesNotExposeStoredSecret(t *testing.T) {
	const storedSecret = "JBSWY3DPEHPK3PXP"
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{otpSecret: storedSecret},
		TimeBasedOtpSettings{AllowedDriftWindows: 1},
	)
	session := &domain.ChallengeSession{SubjectIdentifier: "login:alice"}
	step := &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeVerifyOld),
	}

	if err := provider.Prepare(context.Background(), session, step); err != nil {
		t.Fatalf("prepare verify-old otp step: %v", err)
	}
	if mapContainsString(step.UserInterfaceHints, storedSecret) {
		t.Fatalf("verify-old TOTP hints must not expose stored secret: %#v", step.UserInterfaceHints)
	}
	if mapContainsString(session.SessionContext, storedSecret) {
		t.Fatalf("verify-old TOTP prepare must not persist stored secret in session context: %#v", session.SessionContext)
	}
}

func TestTimeBasedOtpEligibleRequiresSecretExceptRegistration(t *testing.T) {
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{otpSecret: "JBSWY3DPEHPK3PXP"},
		TimeBasedOtpSettings{},
	)

	eligible, err := provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:alice"}, &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeVerifyOld),
	})
	if err != nil {
		t.Fatalf("check otp eligibility: %v", err)
	}
	if !eligible {
		t.Fatal("expected TOTP verification to be eligible when enabled secret exists")
	}

	provider = NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{},
		TimeBasedOtpSettings{},
	)
	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:missing"}, &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeVerifyOld),
	})
	if err != nil {
		t.Fatalf("check missing otp eligibility: %v", err)
	}
	if eligible {
		t.Fatal("expected TOTP verification to be ineligible when enabled secret is missing")
	}

	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:new"}, &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeRegisterNew),
	})
	if err != nil {
		t.Fatalf("check registration otp eligibility: %v", err)
	}
	if !eligible {
		t.Fatal("expected TOTP registration to stay eligible without an existing secret")
	}

	expected := errors.New("otp store unavailable")
	provider = NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{otpSecretErr: expected},
		TimeBasedOtpSettings{},
	)
	eligible, err = provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "login:error"}, &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeVerifyOld),
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected otp store error, got %v", err)
	}
	if eligible {
		t.Fatal("expected errored TOTP secret lookup to be ineligible")
	}
}

func TestTimeBasedOtpEligibleTreatsCorruptSecretAsUnavailable(t *testing.T) {
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{otpSecretErr: apperrors.ObjectState("TOTP凭证不可用，请重新绑定")},
		TimeBasedOtpSettings{},
	)

	eligible, err := provider.Eligible(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
		StepPurpose:   string(domain.ChallengeStepPurposeVerifyOld),
	})

	if err != nil {
		t.Fatalf("corrupt TOTP material should not fail challenge eligibility: %v", err)
	}
	if eligible {
		t.Fatal("corrupt TOTP material must be treated as an unavailable factor")
	}
}

func TestTimeBasedOtpVerifyRejectsMalformedOTP(t *testing.T) {
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{},
		TimeBasedOtpSettings{AllowedDriftWindows: 1},
	)
	ok, err := provider.Verify(context.Background(), &domain.ChallengeSession{SubjectIdentifier: "user:1001"}, &domain.ChallengeStep{}, map[string]any{"oneTimePassword": "12ab56"})
	if err != nil {
		t.Fatalf("verify malformed otp: %v", err)
	}
	if ok {
		t.Fatalf("expected malformed otp to fail")
	}
}

func TestTimeBasedOtpVerifyReturnsFalseWhenBindingIsMissing(t *testing.T) {
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{},
		TimeBasedOtpSettings{AllowedDriftWindows: 1},
	)

	ok, err := provider.Verify(context.Background(), &domain.ChallengeSession{
		SubjectIdentifier: "login:missing",
	}, &domain.ChallengeStep{
		ChallengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
	}, map[string]any{"oneTimePassword": "123456"})
	if err != nil {
		t.Fatalf("verify otp without binding: %v", err)
	}
	if ok {
		t.Fatalf("expected missing binding to fail")
	}
}

func TestTimeBasedOtpPrepareHandlesNilInputs(t *testing.T) {
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{},
		TimeBasedOtpSettings{},
	)
	if err := provider.Prepare(context.Background(), nil, nil); err == nil {
		t.Fatalf("expected prepare with nil inputs to fail")
	}
}

func TestTimeBasedOtpVerifyNewUsesPendingSecret(t *testing.T) {
	provider := NewTimeBasedOtpChallengeStepProvider(
		totpinfra.New(),
		&fakeSubjectStore{},
		TimeBasedOtpSettings{AllowedDriftWindows: 1},
	)
	now := time.Now()
	code, err := libtotp.GenerateCodeCustom("JBSWY3DPEHPK3PXP", now, libtotp.ValidateOpts{
		Period:    totpinfra.DefaultPeriod,
		Skew:      0,
		Digits:    totpinfra.DefaultDigits,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	ok, err := provider.Verify(context.Background(), &domain.ChallengeSession{
		SessionContext: map[string]any{"otp.pendingSecretPlain": "JBSWY3DPEHPK3PXP"},
	}, &domain.ChallengeStep{
		StepPurpose: string(domain.ChallengeStepPurposeVerifyNew),
	}, map[string]any{"otpCode": code})
	if err != nil {
		t.Fatalf("verify new otp: %v", err)
	}
	if !ok {
		t.Fatal("expected verify_new to use pending secret")
	}
}
