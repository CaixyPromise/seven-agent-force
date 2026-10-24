package domain

import "strings"

type ChallengeActionPolicy struct {
	Action                   BusinessAction
	MinimumAssuranceLevel    string
	RequiredProofMethods     []ChallengeType
	AcceptedProofMethods     []ChallengeType
	RequiredOperationBinding bool
	AllowEmptyBinding        bool
	PublicStartAllowed       bool
}

func LookupChallengeActionPolicy(action string) (ChallengeActionPolicy, bool) {
	key := BusinessAction(strings.TrimSpace(action))
	policy, ok := challengeActionPolicies[key]
	return policy, ok
}

func (p ChallengeActionPolicy) RequiresOperationBinding() bool {
	return p.RequiredOperationBinding && !p.AllowEmptyBinding
}

func (p ChallengeActionPolicy) ProofMethodsSatisfied(methodNames []string) bool {
	seen := make(map[string]struct{}, len(methodNames))
	for _, item := range methodNames {
		value := strings.TrimSpace(item)
		if value != "" {
			seen[value] = struct{}{}
		}
	}
	for _, required := range p.RequiredProofMethods {
		if _, ok := seen[string(required)]; !ok {
			return false
		}
	}
	if len(p.AcceptedProofMethods) == 0 {
		return true
	}
	for _, accepted := range p.AcceptedProofMethods {
		if _, ok := seen[string(accepted)]; ok {
			return true
		}
	}
	return false
}

var strongPrivilegedMutationMethods = []ChallengeType{
	ChallengeTypeWebAuthnPasskeyAssertion,
	ChallengeTypeTimeBasedOneTimePassword,
	ChallengeTypeEmailOneTimePassword,
}

// diagnosticContentRevealMethods deliberately excludes email OTP. Revealing
// sensitive or short-lived secret delivery content is a read operation with
// equivalent impact to a privileged secret reveal, so it requires a device
// bound passkey assertion or a locally-held TOTP factor.
var diagnosticContentRevealMethods = []ChallengeType{
	ChallengeTypeWebAuthnPasskeyAssertion,
	ChallengeTypeTimeBasedOneTimePassword,
}

func privilegedMutationPolicy(action BusinessAction) ChallengeActionPolicy {
	return ChallengeActionPolicy{
		Action:                   action,
		PublicStartAllowed:       true,
		RequiredOperationBinding: true,
		AcceptedProofMethods:     strongPrivilegedMutationMethods,
		MinimumAssuranceLevel:    "AAL2",
	}
}

func diagnosticContentRevealPolicy(action BusinessAction) ChallengeActionPolicy {
	return ChallengeActionPolicy{
		Action:                   action,
		PublicStartAllowed:       true,
		RequiredOperationBinding: true,
		AcceptedProofMethods:     diagnosticContentRevealMethods,
		MinimumAssuranceLevel:    "AAL2",
	}
}

var challengeActionPolicies = map[BusinessAction]ChallengeActionPolicy{
	BusinessActionWorkforceIndependentReviewDecision: privilegedMutationPolicy(BusinessActionWorkforceIndependentReviewDecision),
	BusinessActionLogin: {
		Action:             BusinessActionLogin,
		PublicStartAllowed: true,
		AllowEmptyBinding:  true,
	},
	BusinessActionRegisterAccount: {
		Action:                   BusinessActionRegisterAccount,
		PublicStartAllowed:       true,
		RequiredOperationBinding: true,
		AcceptedProofMethods:     []ChallengeType{ChallengeTypeImageCaptcha},
	},
	BusinessActionChangeMFA: {
		Action:             BusinessActionChangeMFA,
		PublicStartAllowed: true,
		AllowEmptyBinding:  true,
	},
	BusinessActionResetPassword: {
		Action:             BusinessActionResetPassword,
		PublicStartAllowed: true,
		AllowEmptyBinding:  true,
	},
	BusinessActionProfilePhoneUpdate: {
		Action:                   BusinessActionProfilePhoneUpdate,
		PublicStartAllowed:       true,
		RequiredOperationBinding: true,
	},
	BusinessActionProfileEmailUpdate: {
		Action:                   BusinessActionProfileEmailUpdate,
		PublicStartAllowed:       true,
		RequiredOperationBinding: true,
	},
	BusinessActionMFAOTPBind: {
		Action:             BusinessActionMFAOTPBind,
		PublicStartAllowed: true,
		AllowEmptyBinding:  true,
	},
	BusinessActionMFAOTPSwitch: {
		Action:                BusinessActionMFAOTPSwitch,
		PublicStartAllowed:    true,
		AllowEmptyBinding:     true,
		RequiredProofMethods:  []ChallengeType{ChallengeTypeTimeBasedOneTimePassword},
		MinimumAssuranceLevel: "AAL2",
	},
	BusinessActionMFAPasskeyBind: {
		Action:             BusinessActionMFAPasskeyBind,
		PublicStartAllowed: true,
		AllowEmptyBinding:  true,
	},
	BusinessActionMFAPasskeySwitch: {
		Action:                BusinessActionMFAPasskeySwitch,
		PublicStartAllowed:    true,
		AllowEmptyBinding:     true,
		RequiredProofMethods:  []ChallengeType{ChallengeTypeWebAuthnPasskeyAssertion},
		MinimumAssuranceLevel: "AAL2",
	},
	BusinessActionMFARecoveryVerify: {
		Action:                BusinessActionMFARecoveryVerify,
		PublicStartAllowed:    true,
		AllowEmptyBinding:     true,
		RequiredProofMethods:  []ChallengeType{ChallengeTypeRecoveryCodeVerification},
		MinimumAssuranceLevel: "AAL2",
	},
	BusinessActionMFARecoveryCodesRegenerate: {
		Action:                BusinessActionMFARecoveryCodesRegenerate,
		PublicStartAllowed:    true,
		AllowEmptyBinding:     true,
		RequiredProofMethods:  []ChallengeType{ChallengeTypeRecoveryCodeVerification},
		MinimumAssuranceLevel: "AAL2",
	},
	BusinessActionMFAOTPDelete: {
		Action:                BusinessActionMFAOTPDelete,
		PublicStartAllowed:    true,
		AllowEmptyBinding:     true,
		RequiredProofMethods:  []ChallengeType{ChallengeTypeTimeBasedOneTimePassword},
		MinimumAssuranceLevel: "AAL2",
	},
	BusinessActionMFAPasskeyDelete: {
		Action:                   BusinessActionMFAPasskeyDelete,
		PublicStartAllowed:       true,
		RequiredOperationBinding: true,
		RequiredProofMethods:     []ChallengeType{ChallengeTypeWebAuthnPasskeyAssertion},
		MinimumAssuranceLevel:    "AAL2",
	},
	BusinessActionRBACAssignUserRoles:               privilegedMutationPolicy(BusinessActionRBACAssignUserRoles),
	BusinessActionRBACAssignRolePermissions:         privilegedMutationPolicy(BusinessActionRBACAssignRolePermissions),
	BusinessActionRBACAssignRoleMenus:               privilegedMutationPolicy(BusinessActionRBACAssignRoleMenus),
	BusinessActionRBACAssignRoleDepts:               privilegedMutationPolicy(BusinessActionRBACAssignRoleDepts),
	BusinessActionRBACAssignMenuPermissions:         privilegedMutationPolicy(BusinessActionRBACAssignMenuPermissions),
	BusinessActionRBACAssignPostRoles:               privilegedMutationPolicy(BusinessActionRBACAssignPostRoles),
	BusinessActionRBACCommitRoleGrants:              privilegedMutationPolicy(BusinessActionRBACCommitRoleGrants),
	BusinessActionRBACGrantTempPermission:           privilegedMutationPolicy(BusinessActionRBACGrantTempPermission),
	BusinessActionRBACRevokeTempPermission:          privilegedMutationPolicy(BusinessActionRBACRevokeTempPermission),
	BusinessActionRBACExtendTempPermission:          privilegedMutationPolicy(BusinessActionRBACExtendTempPermission),
	BusinessActionConfigSensitiveReveal:             privilegedMutationPolicy(BusinessActionConfigSensitiveReveal),
	BusinessActionConfigApplyPending:                privilegedMutationPolicy(BusinessActionConfigApplyPending),
	BusinessActionConfigRollback:                    privilegedMutationPolicy(BusinessActionConfigRollback),
	BusinessActionConfigScopeAssign:                 privilegedMutationPolicy(BusinessActionConfigScopeAssign),
	BusinessActionNotificationDeliveryContentView:   diagnosticContentRevealPolicy(BusinessActionNotificationDeliveryContentView),
	BusinessActionNotificationGroupRouteSave:        privilegedMutationPolicy(BusinessActionNotificationGroupRouteSave),
	BusinessActionNotificationGroupRouteEnable:      privilegedMutationPolicy(BusinessActionNotificationGroupRouteEnable),
	BusinessActionNotificationGroupRouteDisable:     privilegedMutationPolicy(BusinessActionNotificationGroupRouteDisable),
	BusinessActionNotificationGroupRouteRetire:      privilegedMutationPolicy(BusinessActionNotificationGroupRouteRetire),
	BusinessActionNotificationGroupRouteDelete:      privilegedMutationPolicy(BusinessActionNotificationGroupRouteDelete),
	BusinessActionAdminResetPassword:                privilegedMutationPolicy(BusinessActionAdminResetPassword),
	BusinessActionCurrentUserPasswordChange:         privilegedMutationPolicy(BusinessActionCurrentUserPasswordChange),
	BusinessActionAdminForceLogout:                  privilegedMutationPolicy(BusinessActionAdminForceLogout),
	BusinessActionAdminDeleteUser:                   privilegedMutationPolicy(BusinessActionAdminDeleteUser),
	BusinessActionAdminChangeUserStatus:             privilegedMutationPolicy(BusinessActionAdminChangeUserStatus),
	BusinessActionSSOClientCreate:                   privilegedMutationPolicy(BusinessActionSSOClientCreate),
	BusinessActionSSOClientUpdate:                   privilegedMutationPolicy(BusinessActionSSOClientUpdate),
	BusinessActionSSOClientStatusChange:             privilegedMutationPolicy(BusinessActionSSOClientStatusChange),
	BusinessActionSSOClientRedirectEdit:             privilegedMutationPolicy(BusinessActionSSOClientRedirectEdit),
	BusinessActionSSOClientSecretGenerate:           privilegedMutationPolicy(BusinessActionSSOClientSecretGenerate),
	BusinessActionSSOClientSecretDisable:            privilegedMutationPolicy(BusinessActionSSOClientSecretDisable),
	BusinessActionExternalLoginProviderCreate:       privilegedMutationPolicy(BusinessActionExternalLoginProviderCreate),
	BusinessActionExternalLoginProviderUpdate:       privilegedMutationPolicy(BusinessActionExternalLoginProviderUpdate),
	BusinessActionExternalLoginProviderStatusChange: privilegedMutationPolicy(BusinessActionExternalLoginProviderStatusChange),
	BusinessActionExternalLoginProviderSecretRotate: privilegedMutationPolicy(BusinessActionExternalLoginProviderSecretRotate),
	BusinessActionExternalLoginIdentityStatusChange: privilegedMutationPolicy(BusinessActionExternalLoginIdentityStatusChange),
	BusinessActionExternalOAuthTokenRevoke:          privilegedMutationPolicy(BusinessActionExternalOAuthTokenRevoke),
	BusinessActionEnterpriseConnectionSave:          privilegedMutationPolicy(BusinessActionEnterpriseConnectionSave),
	BusinessActionEnterpriseConnectionStatus:        privilegedMutationPolicy(BusinessActionEnterpriseConnectionStatus),
	BusinessActionEnterpriseIdentityLoginAccess:     privilegedMutationPolicy(BusinessActionEnterpriseIdentityLoginAccess),
	BusinessActionEnterpriseIdentityUnbind:          privilegedMutationPolicy(BusinessActionEnterpriseIdentityUnbind),
	BusinessActionPlatformCreate:                    privilegedMutationPolicy(BusinessActionPlatformCreate),
	BusinessActionPlatformUpdate:                    privilegedMutationPolicy(BusinessActionPlatformUpdate),
	BusinessActionPlatformStatusChange:              privilegedMutationPolicy(BusinessActionPlatformStatusChange),
	BusinessActionPlatformLoginMethodsReplace:       privilegedMutationPolicy(BusinessActionPlatformLoginMethodsReplace),
	BusinessActionPlatformSourceRulesReplace:        privilegedMutationPolicy(BusinessActionPlatformSourceRulesReplace),
	BusinessActionPlatformDefaultRolesReplace:       privilegedMutationPolicy(BusinessActionPlatformDefaultRolesReplace),
}
