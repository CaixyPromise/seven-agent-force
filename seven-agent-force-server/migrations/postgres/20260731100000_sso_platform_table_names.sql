-- +goose Up
-- B1 renames the SSO and platform physical tables in place. Apply it before
-- starting application code that uses the lower snake_case names; no
-- legacy-name process may remain live afterward.
ALTER TABLE "sysSsoAuditLog" RENAME TO sys_sso_audit_log;
ALTER TABLE "sysSsoAuthorizationCode" RENAME TO sys_sso_authorization_code;
ALTER TABLE "sysSsoClient" RENAME TO sys_sso_client;
ALTER TABLE "sysSsoClientRedirectUri" RENAME TO sys_sso_client_redirect_uri;
ALTER TABLE "sysSsoClientSecret" RENAME TO sys_sso_client_secret;
ALTER TABLE "sysSsoConsentGrant" RENAME TO sys_sso_consent_grant;
ALTER TABLE "sysSsoIssuerKey" RENAME TO sys_sso_issuer_key;
ALTER TABLE "sysSsoRefreshTokenFamily" RENAME TO sys_sso_refresh_token_family;
ALTER TABLE "sysSsoSession" RENAME TO sys_sso_session;
ALTER TABLE "sysPlatform" RENAME TO sys_platform;
ALTER TABLE "sysPlatformDefaultRole" RENAME TO sys_platform_default_role;
ALTER TABLE "sysPlatformLoginMethod" RENAME TO sys_platform_login_method;
ALTER TABLE "sysPlatformSourceRule" RENAME TO sys_platform_source_rule;
ALTER TABLE "sysPlatformSsoClient" RENAME TO sys_platform_sso_client;

-- +goose Down
-- B1 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
-- +goose StatementBegin
DO $b1$
BEGIN
    RAISE EXCEPTION 'B1 SSO and platform table rename is forward-only';
END
$b1$;
-- +goose StatementEnd
