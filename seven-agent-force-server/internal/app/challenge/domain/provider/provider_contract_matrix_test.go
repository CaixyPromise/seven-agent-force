package provider

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
)

type providerContractMatrixEntry struct {
	challengeType domain.ChallengeType
	provider      ChallengeStepProvider
	eligible      bool
	refreshable   bool
	contracts     map[string][]string
	notes         []string
}

var requiredProviderContractDimensions = []string{"start", "respond", "eligibility", "replay", "noLeak"}

func TestChallengeProviderContractMatrixCoversEveryChallengeType(t *testing.T) {
	matrix := challengeProviderContractMatrix()
	declared := domain.AllChallengeTypes()
	if len(matrix) != len(declared) {
		t.Fatalf("provider contract matrix size = %d, declared challenge types = %d", len(matrix), len(declared))
	}

	providers := make([]ChallengeStepProvider, 0, len(matrix))
	seen := make(map[domain.ChallengeType]struct{}, len(matrix))
	for _, entry := range matrix {
		if entry.provider == nil {
			t.Fatalf("provider for %s is nil", entry.challengeType)
		}
		if entry.provider.Type() != entry.challengeType {
			t.Fatalf("provider type drift: expected %s, got %s", entry.challengeType, entry.provider.Type())
		}
		if _, duplicate := seen[entry.challengeType]; duplicate {
			t.Fatalf("duplicate provider contract matrix row: %s", entry.challengeType)
		}
		seen[entry.challengeType] = struct{}{}
		if _, ok := entry.provider.(ChallengeStepEligibilityProvider); ok != entry.eligible {
			t.Fatalf("%s eligibility capability drift: expected %v, got %v", entry.challengeType, entry.eligible, ok)
		}
		if _, ok := entry.provider.(RefreshableChallengeStepProvider); ok != entry.refreshable {
			t.Fatalf("%s refresh capability drift: expected %v, got %v", entry.challengeType, entry.refreshable, ok)
		}
		if len(entry.contracts) == 0 {
			t.Fatalf("%s provider contract matrix row must name contract-dimension regression tests", entry.challengeType)
		}
		for _, dimension := range requiredProviderContractDimensions {
			tests, ok := entry.contracts[dimension]
			if !ok || len(tests) == 0 {
				t.Fatalf("%s provider contract matrix is missing %s coverage", entry.challengeType, dimension)
			}
		}
		providers = append(providers, entry.provider)
	}
	for _, challengeType := range declared {
		if _, ok := seen[challengeType]; !ok {
			t.Fatalf("declared challenge type %s is missing from provider contract matrix", challengeType)
		}
	}

	registry := NewRegistry(providers...)
	for _, challengeType := range declared {
		item, ok := registry.Provider(challengeType)
		if !ok {
			t.Fatalf("registry missing provider for %s", challengeType)
		}
		if item.Type() != challengeType {
			t.Fatalf("registry provider mismatch for %s: got %s", challengeType, item.Type())
		}
	}
}

func TestChallengeProviderContractMatrixBaselineTestsExist(t *testing.T) {
	source := providerPackageTestSource(t)
	for _, entry := range challengeProviderContractMatrix() {
		for dimension, tests := range entry.contracts {
			for _, testName := range tests {
				if strings.HasPrefix(testName, "NOTE:") {
					continue
				}
				if !strings.Contains(source, "func "+testName+"(") {
					t.Fatalf("%s %s baseline test %s is missing from provider package tests", entry.challengeType, dimension, testName)
				}
			}
		}
	}
}

func challengeProviderContractMatrix() []providerContractMatrixEntry {
	return []providerContractMatrixEntry{
		{
			challengeType: domain.ChallengeTypeImageCaptcha,
			provider:      NewImageCaptchaChallengeStepProvider(nil),
			refreshable:   true,
			contracts: map[string][]string{
				"start":       {"TestImageCaptchaPrepareGeneratesRefreshableImageAndSessionCode"},
				"respond":     {"TestImageCaptchaVerifyConsumesCodeOnce", "TestImageCaptchaVerifyRejectsMissingExpectedCode"},
				"eligibility": {"NOTE:no-eligibility-provider"},
				"replay":      {"TestImageCaptchaVerifyConsumesCodeOnce", "TestImageCaptchaRefreshOverwritesSessionCodeAndKeepsPlainCodeOutOfHints"},
				"noLeak":      {"TestImageCaptchaRefreshOverwritesSessionCodeAndKeepsPlainCodeOutOfHints"},
			},
		},
		{
			challengeType: domain.ChallengeTypePasswordVerification,
			provider:      NewPasswordChallengeStepProvider(nil, nil),
			contracts: map[string][]string{
				"start":       {"TestPasswordPrepareMarksStepRequired"},
				"respond":     {"TestPasswordVerifyAcceptsMatchingCredentialAndRejectsWrongPassword", "TestPasswordVerifyRejectsBlankOrMissingCredential", "TestPasswordVerifyPropagatesCredentialLookupErrors"},
				"eligibility": {"NOTE:no-eligibility-provider"},
				"replay":      {"NOTE:reusable-subject-credential-not-consumed-by-provider"},
				"noLeak":      {"TestPasswordProviderDoesNotPersistSubmittedPasswordOrCredentialHash"},
			},
		},
		{
			challengeType: domain.ChallengeTypeEmailOneTimePassword,
			provider:      NewEmailOtpChallengeStepProvider(nil, nil),
			eligible:      true,
			refreshable:   true,
			contracts: map[string][]string{
				"start":       {"TestEmailOtpPrepareSendsCodeAndMasksTargetEmail", "TestEmailOtpPrepareWithoutTargetDoesNotSendCode"},
				"respond":     {"TestEmailOtpVerifyAcceptsEmailCode", "TestEmailOtpVerifyAfterCacheRoundTrip"},
				"eligibility": {"TestEmailOtpEligibleCachesTargetEmailAndRejectsMissingTarget"},
				"replay":      {"TestEmailOtpVerifyAcceptsEmailCode", "TestEmailOtpRefreshKeepsOnlyLatestCodeAndDoesNotExposeRawOtp"},
				"noLeak":      {"TestEmailOtpPrepareSendsCodeAndMasksTargetEmail", "TestEmailOtpRefreshKeepsOnlyLatestCodeAndDoesNotExposeRawOtp"},
			},
		},
		{
			challengeType: domain.ChallengeTypeTimeBasedOneTimePassword,
			provider:      NewTimeBasedOtpChallengeStepProvider(nil, nil, TimeBasedOtpSettings{}),
			eligible:      true,
			contracts: map[string][]string{
				"start":       {"TestTimeBasedOtpPrepareUsesDynamicIssuerAndAccountName", "TestTimeBasedOtpPrepareVerifyOldDoesNotExposeStoredSecret", "TestTimeBasedOtpPrepareHandlesNilInputs"},
				"respond":     {"TestTimeBasedOtpVerifyAcceptsCurrentWindowOTP", "TestTimeBasedOtpVerifyRejectsMalformedOTP", "TestTimeBasedOtpVerifyReturnsFalseWhenBindingIsMissing", "TestTimeBasedOtpVerifyNewUsesPendingSecret"},
				"eligibility": {"TestTimeBasedOtpEligibleRequiresSecretExceptRegistration"},
				"replay":      {"NOTE:time-window-credential-provider-does-not-consume-totp-codes"},
				"noLeak":      {"TestTimeBasedOtpPrepareVerifyOldDoesNotExposeStoredSecret", "TestTimeBasedOtpProviderDoesNotPersistSubmittedCode"},
			},
			notes: []string{"Replay throttling and failed-attempt punishment are enforced by challenge/login service layers, not the TOTP provider."},
		},
		{
			challengeType: domain.ChallengeTypeRecoveryCodeVerification,
			provider:      NewRecoveryCodeChallengeStepProvider(nil),
			eligible:      true,
			contracts: map[string][]string{
				"start":       {"TestRecoveryCodePrepareExposesSingleUseHints"},
				"respond":     {"TestRecoveryCodeVerifyConsumesOnce", "TestRecoveryCodeVerifyRejectsBlankCode", "TestRecoveryCodeVerifyHandlesNilSession"},
				"eligibility": {"TestRecoveryCodeEligibleRequiresAvailableCodes", "TestRecoveryCodeEligibleRejectsMissingCodes"},
				"replay":      {"TestRecoveryCodeVerifyConsumesOnce"},
				"noLeak":      {"TestRecoveryCodeProviderDoesNotExposeSubmittedCode"},
			},
		},
		{
			challengeType: domain.ChallengeTypeWebAuthnPasskeyAssertion,
			provider:      NewWebAuthnPasskeyAssertionStepProvider(nil, nil, "", nil, 0),
			eligible:      true,
			contracts: map[string][]string{
				"start":       {"TestWebAuthnAssertionPrepareStoresChallengeAndOnlyPublicHints"},
				"respond":     {"TestWebAuthnAssertionVerifiesSignedAuthenticatorData", "TestWebAuthnAssertionRejectsForgedOrInvalidAuthenticatorData", "TestWebAuthnAssertionRejectsMismatchedUserHandle", "TestWebAuthnAssertionRejectsWhenAllowedOriginsAreMissing", "TestWebAuthnAssertionAcceptsFullUint32SignCount", "TestWebAuthnAssertionAllowsLegacyPasskeyWithoutStoredUserHandle"},
				"eligibility": {"TestWebAuthnAssertionEligibleRequiresAllowedOriginsAndCredentials"},
				"replay":      {"TestWebAuthnAssertionRejectsForgedOrInvalidAuthenticatorData"},
				"noLeak":      {"TestWebAuthnAssertionPrepareStoresChallengeAndOnlyPublicHints"},
			},
			notes: []string{"Assertion replay is represented by sign-counter regression and forged/stale authenticator data rejection at provider level."},
		},
		{
			challengeType: domain.ChallengeTypeWebAuthnPasskeyRegistration,
			provider:      NewWebAuthnPasskeyRegistrationStepProvider(nil, nil, "", "", 0),
			refreshable:   true,
			contracts: map[string][]string{
				"start":       {"TestWebAuthnRegistrationStoresUserHandle", "TestWebAuthnRegistrationPrepareDoesNotExposeExistingCredentialSecrets"},
				"respond":     {"TestWebAuthnRegistrationStoresUserHandle", "TestWebAuthnRegistrationRejectsMissingOrMismatchedUserHandle", "TestWebAuthnRegistrationRejectsCredentialIDMismatchFromAttestation", "TestWebAuthnRegistrationRejectsWrongOriginOrRpIDHash", "TestWebAuthnRegistrationRejectsWhenAllowedOriginsAreMissing", "TestWebAuthnRegistrationRejectsMalformedPublicKeyCose"},
				"eligibility": {"NOTE:no-eligibility-provider"},
				"replay":      {"TestWebAuthnRegistrationRefreshReplacesChallengeAndRejectsStaleClientData", "TestWebAuthnRegistrationRejectsExistingCredentialReplay"},
				"noLeak":      {"TestWebAuthnRegistrationPrepareDoesNotExposeExistingCredentialSecrets"},
			},
		},
	}
}

func providerPackageTestSource(t *testing.T) string {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve current test file")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(currentFile), "*_test.go"))
	if err != nil {
		t.Fatalf("list provider package tests: %v", err)
	}
	var builder strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read provider test source %s: %v", file, err)
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}
	return builder.String()
}
