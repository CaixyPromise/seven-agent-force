-- +goose Up
-- B1 renames the SSO and platform physical tables in place. Apply it before
-- starting application code that uses the lower snake_case names; no
-- legacy-name process may remain live afterward.
RENAME TABLE
    sysSsoAuditLog TO sys_sso_audit_log,
    sysSsoAuthorizationCode TO sys_sso_authorization_code,
    sysSsoClient TO sys_sso_client,
    sysSsoClientRedirectUri TO sys_sso_client_redirect_uri,
    sysSsoClientSecret TO sys_sso_client_secret,
    sysSsoConsentGrant TO sys_sso_consent_grant,
    sysSsoIssuerKey TO sys_sso_issuer_key,
    sysSsoRefreshTokenFamily TO sys_sso_refresh_token_family,
    sysSsoSession TO sys_sso_session,
    sysPlatform TO sys_platform,
    sysPlatformDefaultRole TO sys_platform_default_role,
    sysPlatformLoginMethod TO sys_platform_login_method,
    sysPlatformSourceRule TO sys_platform_source_rule,
    sysPlatformSsoClient TO sys_platform_sso_client;

-- +goose Down
-- B1 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'B1 SSO and platform table rename is forward-only';
