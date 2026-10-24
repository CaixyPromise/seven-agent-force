package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestEveryBusinessActionHasRegisteredChallengePolicy(t *testing.T) {
	actions := AllBusinessActions()
	if len(actions) == 0 {
		t.Fatal("business action matrix is empty")
	}
	declared := declaredBusinessActions(t)
	if len(actions) != len(declared) {
		t.Fatalf("business action matrix size = %d, declared constants = %d", len(actions), len(declared))
	}
	seen := map[BusinessAction]bool{}
	for _, action := range actions {
		if seen[action] {
			t.Fatalf("duplicate business action in matrix: %s", action)
		}
		seen[action] = true
		if !declared[action] {
			t.Fatalf("business action matrix contains undeclared action: %s", action)
		}
		policy, ok := LookupChallengeActionPolicy(string(action))
		if !ok {
			t.Fatalf("business action %s has no challenge action policy", action)
		}
		if policy.Action != action {
			t.Fatalf("policy for %s reports action %s", action, policy.Action)
		}
	}
	for action := range declared {
		if !seen[action] {
			t.Fatalf("declared business action %s is missing from AllBusinessActions", action)
		}
	}
}

func TestHighRiskActionPolicyMatrixRejectsDowngradeMethods(t *testing.T) {
	for _, action := range AllBusinessActions() {
		t.Run(string(action), func(t *testing.T) {
			policy, ok := LookupChallengeActionPolicy(string(action))
			if !ok {
				t.Fatalf("%s policy is not registered", action)
			}
			if policy.MinimumAssuranceLevel != "AAL2" {
				t.Skip("not a high-risk AAL2 action")
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
				t.Fatalf("%s accepted password-only proof", action)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypeImageCaptcha)}) {
				t.Fatalf("%s accepted image-captcha proof", action)
			}
			if policy.ProofMethodsSatisfied(nil) {
				t.Fatalf("%s accepted empty proof methods", action)
			}
		})
	}
}

func declaredBusinessActions(t *testing.T) map[BusinessAction]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "business_action.go", nil, 0)
	if err != nil {
		t.Fatalf("parse business_action.go: %v", err)
	}
	declared := map[BusinessAction]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Values) != len(value.Names) {
				continue
			}
			for i, name := range value.Names {
				if !strings.HasPrefix(name.Name, "BusinessAction") {
					continue
				}
				literal, ok := value.Values[i].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				declared[BusinessAction(strings.Trim(literal.Value, `"`))] = true
			}
		}
	}
	return declared
}

func TestAdminResetPasswordPolicyRequiresStrongBoundProof(t *testing.T) {
	policy, ok := LookupChallengeActionPolicy("ADMIN_RESET_PASSWORD")
	if !ok {
		t.Fatal("ADMIN_RESET_PASSWORD policy is not registered")
	}
	if !policy.RequiresOperationBinding() {
		t.Fatal("ADMIN_RESET_PASSWORD must require operation binding")
	}
	if policy.MinimumAssuranceLevel != "AAL2" {
		t.Fatalf("unexpected assurance level: %s", policy.MinimumAssuranceLevel)
	}
	if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
		t.Fatal("ADMIN_RESET_PASSWORD accepted password-only proof")
	}
	if policy.ProofMethodsSatisfied([]string{string(ChallengeTypeImageCaptcha)}) {
		t.Fatal("ADMIN_RESET_PASSWORD accepted image captcha proof")
	}
	if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
		t.Fatal("ADMIN_RESET_PASSWORD rejected TOTP proof")
	}
	if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeWebAuthnPasskeyAssertion)}) {
		t.Fatal("ADMIN_RESET_PASSWORD rejected passkey proof")
	}
	if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
		t.Fatal("ADMIN_RESET_PASSWORD rejected email OTP proof")
	}
}

func TestCurrentUserPasswordChangePolicyRequiresStrongBoundProof(t *testing.T) {
	policy, ok := LookupChallengeActionPolicy("CURRENT_USER_PASSWORD_CHANGE")
	if !ok {
		t.Fatal("CURRENT_USER_PASSWORD_CHANGE policy is not registered")
	}
	if !policy.RequiresOperationBinding() {
		t.Fatal("CURRENT_USER_PASSWORD_CHANGE must require operation binding")
	}
	if policy.MinimumAssuranceLevel != "AAL2" {
		t.Fatalf("unexpected assurance level: %s", policy.MinimumAssuranceLevel)
	}
	if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
		t.Fatal("CURRENT_USER_PASSWORD_CHANGE accepted password-only proof")
	}
	if policy.ProofMethodsSatisfied([]string{string(ChallengeTypeImageCaptcha)}) {
		t.Fatal("CURRENT_USER_PASSWORD_CHANGE accepted image captcha proof")
	}
	if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
		t.Fatal("CURRENT_USER_PASSWORD_CHANGE rejected TOTP proof")
	}
	if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeWebAuthnPasskeyAssertion)}) {
		t.Fatal("CURRENT_USER_PASSWORD_CHANGE rejected passkey proof")
	}
	if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
		t.Fatal("CURRENT_USER_PASSWORD_CHANGE rejected email OTP proof")
	}
}

func TestNotificationDeliveryContentViewPolicyRequiresStrongBoundProof(t *testing.T) {
	policy, ok := LookupChallengeActionPolicy(string(BusinessActionNotificationDeliveryContentView))
	if !ok {
		t.Fatal("NOTIFICATION_DELIVERY_CONTENT_VIEW policy is not registered")
	}
	if !policy.RequiresOperationBinding() {
		t.Fatal("NOTIFICATION_DELIVERY_CONTENT_VIEW must bind each diagnostic read")
	}
	if policy.MinimumAssuranceLevel != "AAL2" {
		t.Fatalf("unexpected assurance level: %s", policy.MinimumAssuranceLevel)
	}
	if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
		t.Fatal("NOTIFICATION_DELIVERY_CONTENT_VIEW accepted password-only proof")
	}
	if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
		t.Fatal("NOTIFICATION_DELIVERY_CONTENT_VIEW rejected TOTP proof")
	}
	if policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
		t.Fatal("NOTIFICATION_DELIVERY_CONTENT_VIEW accepted email-only proof")
	}
}

func TestNotificationGroupRouteActionsRequirePrivilegedProof(t *testing.T) {
	actions := []BusinessAction{
		BusinessActionNotificationGroupRouteSave,
		BusinessActionNotificationGroupRouteEnable,
		BusinessActionNotificationGroupRouteDisable,
		BusinessActionNotificationGroupRouteRetire,
		BusinessActionNotificationGroupRouteDelete,
	}
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			policy, ok := LookupChallengeActionPolicy(string(action))
			if !ok {
				t.Fatalf("%s policy is not registered", action)
			}
			if !policy.RequiresOperationBinding() {
				t.Fatalf("%s must require operation binding", action)
			}
			if policy.MinimumAssuranceLevel != "AAL2" {
				t.Fatalf("%s assurance level=%s, want AAL2", action, policy.MinimumAssuranceLevel)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
				t.Fatalf("%s accepted password-only proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
				t.Fatalf("%s rejected TOTP proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeWebAuthnPasskeyAssertion)}) {
				t.Fatalf("%s rejected passkey proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
				t.Fatalf("%s rejected email OTP proof", action)
			}
		})
	}
}

func TestAdminUserLifecyclePoliciesRequireStrongBoundProof(t *testing.T) {
	for _, action := range []BusinessAction{BusinessActionAdminDeleteUser, BusinessActionAdminChangeUserStatus} {
		t.Run(string(action), func(t *testing.T) {
			policy, ok := LookupChallengeActionPolicy(string(action))
			if !ok {
				t.Fatalf("%s policy is not registered", action)
			}
			if !policy.RequiresOperationBinding() {
				t.Fatalf("%s must require operation binding", action)
			}
			if policy.MinimumAssuranceLevel != "AAL2" {
				t.Fatalf("%s assurance level=%s, want AAL2", action, policy.MinimumAssuranceLevel)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
				t.Fatalf("%s accepted password-only proof", action)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypeImageCaptcha)}) {
				t.Fatalf("%s accepted image captcha proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
				t.Fatalf("%s rejected TOTP proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeWebAuthnPasskeyAssertion)}) {
				t.Fatalf("%s rejected passkey proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
				t.Fatalf("%s rejected email OTP proof", action)
			}
		})
	}
}

func TestSSOClientActionsRequirePrivilegedProof(t *testing.T) {
	actions := []BusinessAction{
		BusinessActionSSOClientCreate,
		BusinessActionSSOClientUpdate,
		BusinessActionSSOClientStatusChange,
		BusinessActionSSOClientRedirectEdit,
		BusinessActionSSOClientSecretGenerate,
		BusinessActionSSOClientSecretDisable,
	}
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			policy, ok := LookupChallengeActionPolicy(string(action))
			if !ok {
				t.Fatalf("%s policy is not registered", action)
			}
			if !policy.RequiresOperationBinding() {
				t.Fatalf("%s must require operation binding", action)
			}
			if policy.MinimumAssuranceLevel != "AAL2" {
				t.Fatalf("%s assurance level=%s, want AAL2", action, policy.MinimumAssuranceLevel)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
				t.Fatalf("%s accepted password-only proof", action)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypeImageCaptcha)}) {
				t.Fatalf("%s accepted image captcha proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
				t.Fatalf("%s rejected TOTP proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeWebAuthnPasskeyAssertion)}) {
				t.Fatalf("%s rejected passkey proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
				t.Fatalf("%s rejected email OTP proof", action)
			}
		})
	}
}

func TestExternalLoginActionsRequirePrivilegedProof(t *testing.T) {
	actions := []BusinessAction{
		BusinessActionExternalLoginProviderCreate,
		BusinessActionExternalLoginProviderUpdate,
		BusinessActionExternalLoginProviderStatusChange,
		BusinessActionExternalLoginProviderSecretRotate,
		BusinessActionExternalLoginIdentityStatusChange,
		BusinessActionExternalOAuthTokenRevoke,
	}
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			policy, ok := LookupChallengeActionPolicy(string(action))
			if !ok {
				t.Fatalf("%s policy is not registered", action)
			}
			if !policy.RequiresOperationBinding() {
				t.Fatalf("%s must require operation binding", action)
			}
			if policy.MinimumAssuranceLevel != "AAL2" {
				t.Fatalf("%s assurance level=%s, want AAL2", action, policy.MinimumAssuranceLevel)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
				t.Fatalf("%s accepted password-only proof", action)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypeImageCaptcha)}) {
				t.Fatalf("%s accepted image captcha proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
				t.Fatalf("%s rejected TOTP proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeWebAuthnPasskeyAssertion)}) {
				t.Fatalf("%s rejected passkey proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
				t.Fatalf("%s rejected email OTP proof", action)
			}
		})
	}
}

func TestPlatformActionsRequirePrivilegedProof(t *testing.T) {
	actions := []BusinessAction{
		BusinessActionPlatformCreate,
		BusinessActionPlatformUpdate,
		BusinessActionPlatformStatusChange,
		BusinessActionPlatformLoginMethodsReplace,
		BusinessActionPlatformSourceRulesReplace,
		BusinessActionPlatformDefaultRolesReplace,
	}
	for _, action := range actions {
		t.Run(string(action), func(t *testing.T) {
			policy, ok := LookupChallengeActionPolicy(string(action))
			if !ok {
				t.Fatalf("%s policy is not registered", action)
			}
			if !policy.RequiresOperationBinding() {
				t.Fatalf("%s must require operation binding", action)
			}
			if policy.MinimumAssuranceLevel != "AAL2" {
				t.Fatalf("%s assurance level=%s, want AAL2", action, policy.MinimumAssuranceLevel)
			}
			if policy.ProofMethodsSatisfied([]string{string(ChallengeTypePasswordVerification)}) {
				t.Fatalf("%s accepted password-only proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeTimeBasedOneTimePassword)}) {
				t.Fatalf("%s rejected TOTP proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeWebAuthnPasskeyAssertion)}) {
				t.Fatalf("%s rejected passkey proof", action)
			}
			if !policy.ProofMethodsSatisfied([]string{string(ChallengeTypeEmailOneTimePassword)}) {
				t.Fatalf("%s rejected email OTP proof", action)
			}
		})
	}
}
