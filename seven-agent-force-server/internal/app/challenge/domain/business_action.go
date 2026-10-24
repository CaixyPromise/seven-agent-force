package domain

type BusinessAction string

const (
	BusinessActionLogin                              BusinessAction = "LOGIN"
	BusinessActionRegisterAccount                    BusinessAction = "REGISTER_ACCOUNT"
	BusinessActionChangeMFA                          BusinessAction = "CHANGE_MFA"
	BusinessActionResetPassword                      BusinessAction = "RESET_PASSWORD"
	BusinessActionAdminResetPassword                 BusinessAction = "ADMIN_RESET_PASSWORD"
	BusinessActionCurrentUserPasswordChange          BusinessAction = "CURRENT_USER_PASSWORD_CHANGE"
	BusinessActionAdminForceLogout                   BusinessAction = "ADMIN_FORCE_LOGOUT"
	BusinessActionAdminDeleteUser                    BusinessAction = "ADMIN_DELETE_USER"
	BusinessActionAdminChangeUserStatus              BusinessAction = "ADMIN_CHANGE_USER_STATUS"
	BusinessActionProfilePhoneUpdate                 BusinessAction = "PROFILE_PHONE_UPDATE"
	BusinessActionProfileEmailUpdate                 BusinessAction = "PROFILE_EMAIL_UPDATE"
	BusinessActionMFAOTPBind                         BusinessAction = "MFA_OTP_BIND"
	BusinessActionMFAOTPSwitch                       BusinessAction = "MFA_OTP_SWITCH"
	BusinessActionMFAPasskeyBind                     BusinessAction = "MFA_PASSKEY_BIND"
	BusinessActionMFAPasskeySwitch                   BusinessAction = "MFA_PASSKEY_SWITCH"
	BusinessActionMFARecoveryVerify                  BusinessAction = "MFA_RECOVERY_VERIFY"
	BusinessActionMFARecoveryCodesRegenerate         BusinessAction = "MFA_RECOVERY_CODES_REGENERATE"
	BusinessActionMFAOTPDelete                       BusinessAction = "MFA_OTP_DELETE"
	BusinessActionMFAPasskeyDelete                   BusinessAction = "MFA_PASSKEY_DELETE"
	BusinessActionRBACAssignUserRoles                BusinessAction = "RBAC_ASSIGN_USER_ROLES"
	BusinessActionRBACAssignRolePermissions          BusinessAction = "RBAC_ASSIGN_ROLE_PERMISSIONS"
	BusinessActionRBACAssignRoleMenus                BusinessAction = "RBAC_ASSIGN_ROLE_MENUS"
	BusinessActionRBACAssignRoleDepts                BusinessAction = "RBAC_ASSIGN_ROLE_DEPTS"
	BusinessActionRBACAssignMenuPermissions          BusinessAction = "RBAC_ASSIGN_MENU_PERMISSIONS"
	BusinessActionRBACAssignPostRoles                BusinessAction = "RBAC_ASSIGN_POST_ROLES"
	BusinessActionRBACCommitRoleGrants               BusinessAction = "RBAC_COMMIT_ROLE_GRANTS"
	BusinessActionRBACGrantTempPermission            BusinessAction = "RBAC_GRANT_TEMP_PERMISSION"
	BusinessActionRBACRevokeTempPermission           BusinessAction = "RBAC_REVOKE_TEMP_PERMISSION"
	BusinessActionRBACExtendTempPermission           BusinessAction = "RBAC_EXTEND_TEMP_PERMISSION"
	BusinessActionConfigSensitiveReveal              BusinessAction = "CONFIG_SENSITIVE_REVEAL"
	BusinessActionConfigApplyPending                 BusinessAction = "CONFIG_APPLY_PENDING"
	BusinessActionConfigRollback                     BusinessAction = "CONFIG_ROLLBACK"
	BusinessActionConfigScopeAssign                  BusinessAction = "CONFIG_SCOPE_ASSIGN"
	BusinessActionNotificationDeliveryContentView    BusinessAction = "NOTIFICATION_DELIVERY_CONTENT_VIEW"
	BusinessActionNotificationGroupRouteSave         BusinessAction = "NOTIFICATION_GROUP_ROUTE_SAVE"
	BusinessActionNotificationGroupRouteEnable       BusinessAction = "NOTIFICATION_GROUP_ROUTE_ENABLE"
	BusinessActionNotificationGroupRouteDisable      BusinessAction = "NOTIFICATION_GROUP_ROUTE_DISABLE"
	BusinessActionNotificationGroupRouteRetire       BusinessAction = "NOTIFICATION_GROUP_ROUTE_RETIRE"
	BusinessActionNotificationGroupRouteDelete       BusinessAction = "NOTIFICATION_GROUP_ROUTE_DELETE"
	BusinessActionSSOClientCreate                    BusinessAction = "SSO_CLIENT_CREATE"
	BusinessActionSSOClientUpdate                    BusinessAction = "SSO_CLIENT_UPDATE"
	BusinessActionSSOClientStatusChange              BusinessAction = "SSO_CLIENT_STATUS_CHANGE"
	BusinessActionSSOClientRedirectEdit              BusinessAction = "SSO_CLIENT_REDIRECT_EDIT"
	BusinessActionSSOClientSecretGenerate            BusinessAction = "SSO_CLIENT_SECRET_GENERATE"
	BusinessActionSSOClientSecretDisable             BusinessAction = "SSO_CLIENT_SECRET_DISABLE"
	BusinessActionExternalLoginProviderCreate        BusinessAction = "EXTERNAL_LOGIN_PROVIDER_CREATE"
	BusinessActionExternalLoginProviderUpdate        BusinessAction = "EXTERNAL_LOGIN_PROVIDER_UPDATE"
	BusinessActionExternalLoginProviderStatusChange  BusinessAction = "EXTERNAL_LOGIN_PROVIDER_STATUS_CHANGE"
	BusinessActionExternalLoginProviderSecretRotate  BusinessAction = "EXTERNAL_LOGIN_PROVIDER_SECRET_ROTATE"
	BusinessActionExternalLoginIdentityStatusChange  BusinessAction = "EXTERNAL_LOGIN_IDENTITY_STATUS_CHANGE"
	BusinessActionExternalOAuthTokenRevoke           BusinessAction = "EXTERNAL_OAUTH_TOKEN_REVOKE"
	BusinessActionEnterpriseConnectionSave           BusinessAction = "ENTERPRISE_CONNECTION_SAVE"
	BusinessActionEnterpriseConnectionStatus         BusinessAction = "ENTERPRISE_CONNECTION_STATUS"
	BusinessActionEnterpriseIdentityLoginAccess      BusinessAction = "ENTERPRISE_IDENTITY_LOGIN_ACCESS"
	BusinessActionEnterpriseIdentityUnbind           BusinessAction = "ENTERPRISE_IDENTITY_UNBIND"
	BusinessActionPlatformCreate                     BusinessAction = "PLATFORM_CREATE"
	BusinessActionPlatformUpdate                     BusinessAction = "PLATFORM_UPDATE"
	BusinessActionPlatformStatusChange               BusinessAction = "PLATFORM_STATUS_CHANGE"
	BusinessActionPlatformLoginMethodsReplace        BusinessAction = "PLATFORM_LOGIN_METHODS_REPLACE"
	BusinessActionPlatformSourceRulesReplace         BusinessAction = "PLATFORM_SOURCE_RULES_REPLACE"
	BusinessActionPlatformDefaultRolesReplace        BusinessAction = "PLATFORM_DEFAULT_ROLES_REPLACE"
	BusinessActionWorkforceIndependentReviewDecision BusinessAction = "WORKFORCE_INDEPENDENT_REVIEW_DECISION"
)

var allBusinessActions = []BusinessAction{
	BusinessActionLogin,
	BusinessActionRegisterAccount,
	BusinessActionChangeMFA,
	BusinessActionResetPassword,
	BusinessActionAdminResetPassword,
	BusinessActionCurrentUserPasswordChange,
	BusinessActionAdminForceLogout,
	BusinessActionAdminDeleteUser,
	BusinessActionAdminChangeUserStatus,
	BusinessActionProfilePhoneUpdate,
	BusinessActionProfileEmailUpdate,
	BusinessActionMFAOTPBind,
	BusinessActionMFAOTPSwitch,
	BusinessActionMFAPasskeyBind,
	BusinessActionMFAPasskeySwitch,
	BusinessActionMFARecoveryVerify,
	BusinessActionMFARecoveryCodesRegenerate,
	BusinessActionMFAOTPDelete,
	BusinessActionMFAPasskeyDelete,
	BusinessActionRBACAssignUserRoles,
	BusinessActionRBACAssignRolePermissions,
	BusinessActionRBACAssignRoleMenus,
	BusinessActionRBACAssignRoleDepts,
	BusinessActionRBACAssignMenuPermissions,
	BusinessActionRBACAssignPostRoles,
	BusinessActionRBACCommitRoleGrants,
	BusinessActionRBACGrantTempPermission,
	BusinessActionRBACRevokeTempPermission,
	BusinessActionRBACExtendTempPermission,
	BusinessActionConfigSensitiveReveal,
	BusinessActionConfigApplyPending,
	BusinessActionConfigRollback,
	BusinessActionConfigScopeAssign,
	BusinessActionNotificationDeliveryContentView,
	BusinessActionNotificationGroupRouteSave,
	BusinessActionNotificationGroupRouteEnable,
	BusinessActionNotificationGroupRouteDisable,
	BusinessActionNotificationGroupRouteRetire,
	BusinessActionNotificationGroupRouteDelete,
	BusinessActionSSOClientCreate,
	BusinessActionSSOClientUpdate,
	BusinessActionSSOClientStatusChange,
	BusinessActionSSOClientRedirectEdit,
	BusinessActionSSOClientSecretGenerate,
	BusinessActionSSOClientSecretDisable,
	BusinessActionExternalLoginProviderCreate,
	BusinessActionExternalLoginProviderUpdate,
	BusinessActionExternalLoginProviderStatusChange,
	BusinessActionExternalLoginProviderSecretRotate,
	BusinessActionExternalLoginIdentityStatusChange,
	BusinessActionExternalOAuthTokenRevoke,
	BusinessActionEnterpriseConnectionSave,
	BusinessActionEnterpriseConnectionStatus,
	BusinessActionEnterpriseIdentityLoginAccess,
	BusinessActionEnterpriseIdentityUnbind,
	BusinessActionPlatformCreate,
	BusinessActionPlatformUpdate,
	BusinessActionPlatformStatusChange,
	BusinessActionPlatformLoginMethodsReplace,
	BusinessActionPlatformSourceRulesReplace,
	BusinessActionPlatformDefaultRolesReplace,
	BusinessActionWorkforceIndependentReviewDecision,
}

func AllBusinessActions() []BusinessAction {
	return append([]BusinessAction(nil), allBusinessActions...)
}
