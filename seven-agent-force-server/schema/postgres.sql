-- PostgreSQL public schema; structure only, no application rows or migration ledger.
SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET search_path = '';
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;


CREATE SCHEMA IF NOT EXISTS "public";



CREATE FUNCTION "public"."workforce_assert_project_hard_cut_ready"() RETURNS "void"
    LANGUAGE "plpgsql"
    SET "search_path" TO 'pg_catalog', 'public'
    AS $$
DECLARE
  relation_name TEXT;
  has_business_rows BOOLEAN;
BEGIN
  FOREACH relation_name IN ARRAY ARRAY[
    'workforce_repository_connections',
    'workforce_repository_access_bindings',
    'workforce_repository_credential_events',
    'workforce_repository_lifecycle_events',
    'workforce_hook_bundles',
    'workforce_hook_bundle_releases',
    'workforce_hook_bindings',
    'workforce_hook_execution_profiles',
    'workforce_golden_verification_runs',
    'workforce_integrated_verification_runs'
  ] LOOP
    IF pg_catalog.to_regclass(pg_catalog.format('public.%I',relation_name)) IS NULL THEN
      CONTINUE;
    END IF;
    EXECUTE pg_catalog.format(
      'SELECT EXISTS (SELECT 1 FROM public.%I LIMIT 1)',
      relation_name
    ) INTO has_business_rows;
    IF has_business_rows THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'PROJECT_REPOSITORY_HARD_CUT_RESET_REQUIRED';
    END IF;
  END LOOP;

  FOREACH relation_name IN ARRAY ARRAY[
    'workforce_tasks',
    'workforce_task_attempts',
    'workforce_harness_sessions',
    'workforce_completion_proposals',
    'workforce_task_events',
    'workforce_artifacts',
    'workforce_check_runs',
    'workforce_model_invocations',
    'workforce_task_execution_snapshots',
    'workforce_review_runs',
    'workforce_delivery_requests'
  ] LOOP
    IF pg_catalog.to_regclass(pg_catalog.format('public.%I',relation_name)) IS NULL THEN
      CONTINUE;
    END IF;
    EXECUTE pg_catalog.format(
      'SELECT EXISTS (SELECT 1 FROM public.%I LIMIT 1)',
      relation_name
    ) INTO has_business_rows;
    IF has_business_rows THEN
      RAISE EXCEPTION USING
        ERRCODE = 'P0001',
        MESSAGE = 'PROJECT_HARD_CUT_RESET_REQUIRED';
    END IF;
  END LOOP;
END
$$;



CREATE FUNCTION "public"."workforce_project_version_immutable"() RETURNS "trigger"
    LANGUAGE "plpgsql"
    AS $$
BEGIN
	RAISE EXCEPTION 'PROJECT_VERSION_IMMUTABLE';
END;
$$;


SET default_table_access_method = "heap";


CREATE TABLE "public"."docker_compose_project" (
    "id" bigint NOT NULL,
    "projectId" character varying(128) NOT NULL,
    "projectName" character varying(128) NOT NULL,
    "workingDir" character varying(1024),
    "configFilesJson" "text",
    "composeYaml" "text",
    "composeFilePath" character varying(1024),
    "fileManifestJson" "text",
    "description" character varying(512),
    "status" character varying(32) DEFAULT 'unknown'::character varying NOT NULL,
    "lastPreviewJson" "text",
    "lastValidationJson" "text",
    "lastOperationId" bigint,
    "source" character varying(32) DEFAULT 'MANAGED'::character varying NOT NULL,
    "createdBy" bigint,
    "deleted" boolean DEFAULT false NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."docker_operation" (
    "id" bigint NOT NULL,
    "operationType" character varying(64) NOT NULL,
    "targetType" character varying(64) NOT NULL,
    "targetId" character varying(128),
    "targetName" character varying(512),
    "status" character varying(32) NOT NULL,
    "progressPercent" integer DEFAULT 0 NOT NULL,
    "currentStage" character varying(128),
    "errorSummary" character varying(1024),
    "resultJson" "text",
    "requestPayloadPreview" "text",
    "requestPayloadCiphertext" "text",
    "requestPayloadEdek" "text",
    "requestPayloadWrapKeyRef" character varying(255),
    "actorUserId" bigint,
    "actorUsername" character varying(128),
    "retryOf" bigint,
    "cancelRequested" boolean DEFAULT false NOT NULL,
    "timeoutAt" timestamp with time zone,
    "startedAt" timestamp with time zone,
    "finishedAt" timestamp with time zone,
    "heartbeatAt" timestamp with time zone,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."docker_operation_event" (
    "id" bigint NOT NULL,
    "operationId" bigint NOT NULL,
    "sequence" bigint NOT NULL,
    "eventType" character varying(32) NOT NULL,
    "stage" character varying(128),
    "percent" integer,
    "message" character varying(2048),
    "payloadJson" "text",
    "occurredAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "integrityStatus" character varying(16) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "diagnosticId" character varying(191),
    "integrityVersion" bigint DEFAULT 0 NOT NULL,
    "diagnosedAt" timestamp with time zone,
    "integrityScope" character varying(64),
    "integrityRelationshipType" character varying(128),
    "integrityReason" character varying(128),
    CONSTRAINT "chk_docker_operation_event_diagnostic_metadata" CHECK ((((("integrityStatus")::"text" = 'ACTIVE'::"text") AND ("diagnosticId" IS NULL) AND ("integrityScope" IS NULL) AND ("integrityRelationshipType" IS NULL) AND ("integrityReason" IS NULL)) OR ((("integrityStatus")::"text" = 'QUARANTINED'::"text") AND ("diagnosticId" IS NOT NULL) AND ("integrityScope" IS NOT NULL) AND ("integrityRelationshipType" IS NOT NULL) AND ("integrityReason" IS NOT NULL)))),
    CONSTRAINT "chk_docker_operation_event_integrity_status" CHECK ((("integrityStatus")::"text" = ANY ((ARRAY['ACTIVE'::character varying, 'QUARANTINED'::character varying])::"text"[])))
);



CREATE TABLE "public"."docker_operation_event_orphan_audit" (
    "id" bigint NOT NULL,
    "diagnosticId" character varying(191) NOT NULL,
    "eventId" bigint NOT NULL,
    "operationId" bigint NOT NULL,
    "sequence" bigint NOT NULL,
    "expectedIntegrityVersion" bigint NOT NULL,
    "action" character varying(32) NOT NULL,
    "result" character varying(32) NOT NULL,
    "actorUserId" bigint NOT NULL,
    "actorUsername" character varying(128) NOT NULL,
    "reason" character varying(512) NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "chk_docker_operation_orphan_audit_action" CHECK ((("action")::"text" = 'DELETE'::"text")),
    CONSTRAINT "chk_docker_operation_orphan_audit_result" CHECK ((("result")::"text" = ANY ((ARRAY['DELETED'::character varying, 'ALREADY_DONE'::character varying, 'PARENT_PRESENT'::character varying])::"text"[])))
);



CREATE TABLE "public"."docker_operation_integrity_guard" (
    "operationId" bigint NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."docker_remote_registry" (
    "id" bigint NOT NULL,
    "name" character varying(128) NOT NULL,
    "code" character varying(64) NOT NULL,
    "registryType" character varying(32) DEFAULT 'REGISTRY'::character varying NOT NULL,
    "endpoint" character varying(512) NOT NULL,
    "apiBaseUrl" character varying(512),
    "authType" character varying(32) DEFAULT 'ANONYMOUS'::character varying NOT NULL,
    "username" character varying(256),
    "tokenRealm" character varying(512),
    "tokenService" character varying(256),
    "credentialId" bigint,
    "namespaceWhitelistJson" "text",
    "tlsEnabled" boolean DEFAULT true NOT NULL,
    "insecureSkipVerify" boolean DEFAULT false NOT NULL,
    "defaultRegistry" boolean DEFAULT false NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "description" character varying(512),
    "sort" integer DEFAULT 0 NOT NULL,
    "secretCiphertext" "text",
    "secretEdek" "text",
    "wrapKeyRef" character varying(255),
    "deleted" boolean DEFAULT false NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_external_login_provider" (
    "id" bigint NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "providerName" character varying(128) NOT NULL,
    "protocolType" character varying(32) NOT NULL,
    "issuer" character varying(512),
    "authorizationEndpoint" character varying(1024) NOT NULL,
    "tokenEndpoint" character varying(1024) NOT NULL,
    "userinfoEndpoint" character varying(1024),
    "jwksUri" character varying(1024),
    "clientId" character varying(255) NOT NULL,
    "clientSecretCiphertext" "text",
    "clientSecretEdek" "text",
    "clientSecretWrapKeyRef" character varying(128),
    "scopesJson" "json" NOT NULL,
    "redirectUri" character varying(1024) NOT NULL,
    "displayName" character varying(128) NOT NULL,
    "icon" character varying(128),
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "displayEnabled" smallint DEFAULT '0'::smallint NOT NULL,
    "loginEnabled" smallint DEFAULT '0'::smallint NOT NULL,
    "bindEnabled" smallint DEFAULT '1'::smallint NOT NULL,
    "emailAutoBindEnabled" smallint DEFAULT '0'::smallint NOT NULL,
    "accountAutoCreateEnabled" smallint DEFAULT '0'::smallint NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysExternalLoginProvider_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysExternalLoginProvider_id_seq" OWNED BY "public"."sys_external_login_provider"."id";



CREATE TABLE "public"."sys_external_oauth_login_state" (
    "id" bigint NOT NULL,
    "stateId" character varying(128) NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "platformCode" character varying(64),
    "provisioningAuthorityId" character varying(96),
    "loginTransactionId" character varying(128),
    "redirectAfterLogin" character varying(1024),
    "bindUserId" bigint,
    "stateHash" character varying(255) NOT NULL,
    "nonceHash" character varying(255),
    "codeVerifierCiphertext" "text",
    "codeVerifierEdek" "text",
    "codeVerifierWrapKeyRef" character varying(128),
    "issuer" character varying(512),
    "providerConfigDigest" character(64),
    "redirectUri" character varying(1024) NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "consumedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "loginIp" character varying(64),
    "userAgent" character varying(1024),
    "traceId" character varying(128),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysExternalOAuthLoginState_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysExternalOAuthLoginState_id_seq" OWNED BY "public"."sys_external_oauth_login_state"."id";



CREATE TABLE "public"."sys_external_oauth_token" (
    "id" bigint NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "identityId" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "tokenPurpose" character varying(32) NOT NULL,
    "scopeJson" "json",
    "scopeHash" character varying(128) NOT NULL,
    "tokenSetCiphertext" "text" NOT NULL,
    "tokenSetEdek" "text" NOT NULL,
    "tokenSetWrapKeyRef" character varying(128) NOT NULL,
    "accessExpiresAt" timestamp with time zone,
    "refreshExpiresAt" timestamp with time zone,
    "lastRefreshAt" timestamp with time zone,
    "revokedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "version" integer DEFAULT 0 NOT NULL,
    "metadataJson" "json",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysExternalOAuthToken_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysExternalOAuthToken_id_seq" OWNED BY "public"."sys_external_oauth_token"."id";



CREATE TABLE "public"."sys_external_provider_method" (
    "id" bigint NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "methodKey" character varying(128) NOT NULL,
    "capabilityCode" character varying(64) NOT NULL,
    "requiredScopesJson" "json",
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysExternalProviderMethod_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysExternalProviderMethod_id_seq" OWNED BY "public"."sys_external_provider_method"."id";



CREATE TABLE "public"."sys_external_user_identity" (
    "id" bigint NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "externalIssuer" character varying(512),
    "externalIssuerDigest" "bytea",
    "externalIdentityDigest" "bytea",
    "externalSubject" character varying(255),
    "providerSubjectDigest" "bytea",
    "userId" bigint NOT NULL,
    "externalLogin" character varying(255),
    "externalEmail" character varying(255),
    "emailVerified" smallint DEFAULT '0'::smallint NOT NULL,
    "displayName" character varying(255),
    "avatarUrl" character varying(1024),
    "profileJson" "json",
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "firstLinkedAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "lastLoginAt" timestamp with time zone,
    "lastVerifiedAt" timestamp with time zone,
    "metadataJson" "json",
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL,
    "scope_type" character varying(32) DEFAULT 'SYSTEM'::character varying NOT NULL,
    "organization_id" bigint,
    "enterprise_connection_id" bigint,
    "subject_kind" character varying(64),
    "subject_ciphertext" "text",
    "subject_edek" "text",
    "subject_wrap_key_ref" character varying(128),
    "subject_digest" character varying(128),
    "subject_digest_key_ref" character varying(128),
    "display_hint" character varying(128),
    "identity_version" bigint DEFAULT 1 NOT NULL,
    "verified_at" timestamp with time zone,
    "revoked_at" timestamp with time zone,
    CONSTRAINT "chk_external_identity_scope" CHECK ((((("scope_type")::"text" = 'SYSTEM'::"text") AND ("organization_id" IS NULL) AND ("enterprise_connection_id" IS NULL)) OR ((("scope_type")::"text" = 'ORGANIZATION'::"text") AND ("organization_id" IS NOT NULL) AND ("enterprise_connection_id" IS NOT NULL))))
);



CREATE SEQUENCE "public"."sysExternalUserIdentity_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysExternalUserIdentity_id_seq" OWNED BY "public"."sys_external_user_identity"."id";



CREATE TABLE "public"."sys_notification_mailbox" (
    "id" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "userId" bigint NOT NULL,
    "mailboxKey" character varying(96) NOT NULL,
    "changeSequence" bigint DEFAULT 0 NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



ALTER TABLE "public"."sys_notification_mailbox" ALTER COLUMN "id" ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME "public"."sysNotificationMailbox_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);



CREATE TABLE "public"."sys_platform_default_role" (
    "id" bigint NOT NULL,
    "platformCode" character varying(64) NOT NULL,
    "roleId" bigint NOT NULL,
    "autoAssignEnabled" smallint DEFAULT '0'::smallint NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysPlatformDefaultRole_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysPlatformDefaultRole_id_seq" OWNED BY "public"."sys_platform_default_role"."id";



CREATE TABLE "public"."sys_platform_login_method" (
    "id" bigint NOT NULL,
    "platformCode" character varying(64) NOT NULL,
    "methodType" character varying(32) NOT NULL,
    "providerCode" character varying(64) DEFAULT ''::character varying NOT NULL,
    "displayName" character varying(128) NOT NULL,
    "icon" character varying(128),
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "displayEnabled" smallint DEFAULT '1'::smallint NOT NULL,
    "loginEnabled" smallint DEFAULT '1'::smallint NOT NULL,
    "metadataJson" "json",
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysPlatformLoginMethod_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysPlatformLoginMethod_id_seq" OWNED BY "public"."sys_platform_login_method"."id";



CREATE TABLE "public"."sys_platform_source_rule" (
    "id" bigint NOT NULL,
    "platformCode" character varying(64) NOT NULL,
    "matchType" character varying(32) NOT NULL,
    "matchValue" character varying(1024) NOT NULL,
    "priority" integer DEFAULT 0 NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysPlatformSourceRule_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysPlatformSourceRule_id_seq" OWNED BY "public"."sys_platform_source_rule"."id";



CREATE TABLE "public"."sys_platform_sso_client" (
    "id" bigint NOT NULL,
    "platformCode" character varying(64) NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysPlatformSsoClient_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysPlatformSsoClient_id_seq" OWNED BY "public"."sys_platform_sso_client"."id";



CREATE TABLE "public"."sys_platform" (
    "id" bigint NOT NULL,
    "platformCode" character varying(64) NOT NULL,
    "platformName" character varying(128) NOT NULL,
    "platformType" character varying(32) DEFAULT 'ADMIN'::character varying NOT NULL,
    "description" character varying(512),
    "defaultRedirectUrl" character varying(1024),
    "allowAutoRegister" smallint DEFAULT '0'::smallint NOT NULL,
    "allowFormRegister" smallint DEFAULT '0'::smallint NOT NULL,
    "isDefault" smallint DEFAULT '0'::smallint NOT NULL,
    "defaultDeptId" bigint,
    "brandJson" "json",
    "settingsJson" "json",
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysPlatform_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysPlatform_id_seq" OWNED BY "public"."sys_platform"."id";



CREATE TABLE "public"."sys_sso_audit_log" (
    "id" bigint NOT NULL,
    "eventType" character varying(64) NOT NULL,
    "clientId" character varying(128),
    "userId" bigint,
    "sessionId" character varying(128),
    "deviceId" character varying(128),
    "tenantId" character varying(128),
    "loginIp" character varying(64),
    "userAgent" character varying(1024),
    "result" character varying(32) NOT NULL,
    "reasonCode" character varying(64),
    "detailJson" "json",
    "traceId" character varying(128),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoAuditLog_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoAuditLog_id_seq" OWNED BY "public"."sys_sso_audit_log"."id";



CREATE TABLE "public"."sys_sso_authorization_code" (
    "id" bigint NOT NULL,
    "code" character varying(255) NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "userId" bigint NOT NULL,
    "sessionId" character varying(128) NOT NULL,
    "redirectUri" character varying(512) NOT NULL,
    "scopesJson" "json" NOT NULL,
    "codeChallenge" character varying(255),
    "codeChallengeMethod" character varying(32),
    "nonce" character varying(255),
    "acr" character varying(64),
    "amrJson" "json",
    "expiresAt" timestamp with time zone NOT NULL,
    "consumedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoAuthorizationCode_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoAuthorizationCode_id_seq" OWNED BY "public"."sys_sso_authorization_code"."id";



CREATE TABLE "public"."sys_sso_client_redirect_uri" (
    "id" bigint NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "redirectUri" character varying(512) NOT NULL,
    "postLogoutRedirectUri" character varying(512),
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoClientRedirectUri_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoClientRedirectUri_id_seq" OWNED BY "public"."sys_sso_client_redirect_uri"."id";



CREATE TABLE "public"."sys_sso_client_secret" (
    "id" bigint NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "secretHash" character varying(255) NOT NULL,
    "secretHint" character varying(128),
    "expiresAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoClientSecret_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoClientSecret_id_seq" OWNED BY "public"."sys_sso_client_secret"."id";



CREATE TABLE "public"."sys_sso_client" (
    "id" bigint NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "clientName" character varying(128) NOT NULL,
    "clientType" character varying(32) NOT NULL,
    "clientAuthMethod" character varying(32) DEFAULT 'none'::character varying NOT NULL,
    "grantTypesJson" "json" NOT NULL,
    "scopesJson" "json" NOT NULL,
    "requirePkce" smallint DEFAULT '1'::smallint NOT NULL,
    "requireConsent" smallint DEFAULT '0'::smallint NOT NULL,
    "trustedFirstParty" smallint DEFAULT '0'::smallint NOT NULL,
    "accessTokenTtlSec" integer DEFAULT 1800 NOT NULL,
    "refreshTokenTtlSec" integer DEFAULT 2592000 NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoClient_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoClient_id_seq" OWNED BY "public"."sys_sso_client"."id";



CREATE TABLE "public"."sys_sso_consent_grant" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "scopesJson" "json" NOT NULL,
    "grantedAt" timestamp with time zone NOT NULL,
    "revokedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoConsentGrant_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoConsentGrant_id_seq" OWNED BY "public"."sys_sso_consent_grant"."id";



CREATE TABLE "public"."sys_sso_issuer_key" (
    "id" bigint NOT NULL,
    "kid" character varying(128) NOT NULL,
    "algorithm" character varying(32) NOT NULL,
    "publicKeyPem" "text" NOT NULL,
    "privateKeyCiphertext" "text",
    "keyStatus" character varying(32) NOT NULL,
    "activateAt" timestamp with time zone,
    "retireAt" timestamp with time zone,
    "metadataJson" "json",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoIssuerKey_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoIssuerKey_id_seq" OWNED BY "public"."sys_sso_issuer_key"."id";



CREATE TABLE "public"."sys_sso_refresh_token_family" (
    "id" bigint NOT NULL,
    "familyId" character varying(128) NOT NULL,
    "sessionId" character varying(128) NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "userId" bigint NOT NULL,
    "currentTokenHash" character varying(255) NOT NULL,
    "previousTokenHash" character varying(255),
    "reuseDetected" smallint DEFAULT '0'::smallint NOT NULL,
    "rotatedAt" timestamp with time zone,
    "expiresAt" timestamp with time zone NOT NULL,
    "revokedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoRefreshTokenFamily_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoRefreshTokenFamily_id_seq" OWNED BY "public"."sys_sso_refresh_token_family"."id";



CREATE TABLE "public"."sys_sso_session" (
    "id" bigint NOT NULL,
    "sessionId" character varying(128) NOT NULL,
    "userId" bigint NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "platformCode" character varying(64),
    "deviceId" character varying(128),
    "loginMethod" character varying(64) DEFAULT 'LOCAL'::character varying NOT NULL,
    "externalProviderCode" character varying(64),
    "externalIdentityId" bigint,
    "loginIp" character varying(64),
    "userAgent" character varying(1024),
    "acr" character varying(64),
    "amrJson" "json",
    "loginAt" timestamp with time zone NOT NULL,
    "lastAccessAt" timestamp with time zone,
    "expiresAt" timestamp with time zone NOT NULL,
    "revokedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" "json",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);



CREATE SEQUENCE "public"."sysSsoSession_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sysSsoSession_id_seq" OWNED BY "public"."sys_sso_session"."id";



CREATE TABLE "public"."sys_config" (
    "id" bigint NOT NULL,
    "groupId" bigint NOT NULL,
    "configKey" character varying(128) NOT NULL,
    "configValue" "text" NOT NULL,
    "valueType" character varying(16) NOT NULL,
    "configDesc" character varying(255),
    "isSensitive" smallint DEFAULT '0'::smallint NOT NULL,
    "isSystemConfig" boolean DEFAULT false NOT NULL,
    "requiredLogin" boolean DEFAULT false NOT NULL,
    "isReadonly" smallint DEFAULT '0'::smallint NOT NULL,
    "isEnabled" smallint DEFAULT '1'::smallint NOT NULL,
    "effectType" character varying(32),
    "extJson" "text",
    "createdBy" bigint DEFAULT '0'::bigint NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedBy" bigint DEFAULT '0'::bigint NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "uiWidget" character varying(32) DEFAULT 'INPUT'::character varying NOT NULL,
    "validationJson" "text",
    "exposure" character varying(20) DEFAULT 'INTERNAL'::character varying NOT NULL,
    "sensitivity" character varying(20) DEFAULT 'NORMAL'::character varying NOT NULL,
    "schemaVersion" integer DEFAULT 1 NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL
);



CREATE TABLE "public"."sys_config_change_log" (
    "id" bigint NOT NULL,
    "configId" bigint NOT NULL,
    "configKey" character varying(255) NOT NULL,
    "operationType" character varying(20) NOT NULL,
    "oldValue" "text",
    "newValue" "text",
    "effectType" character varying(20) NOT NULL,
    "status" character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    "parentLogId" bigint,
    "relatedLogId" bigint,
    "operatorId" bigint,
    "operatorName" character varying(100),
    "operationTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "operationReason" character varying(500),
    "appliedBy" bigint,
    "appliedTime" timestamp with time zone,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "oldValueProtected" boolean DEFAULT false NOT NULL,
    "newValueProtected" boolean DEFAULT false NOT NULL,
    "oldAssetSnapshot" "text",
    "newAssetSnapshot" "text"
);



CREATE SEQUENCE "public"."sys_config_change_log_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_config_change_log_id_seq" OWNED BY "public"."sys_config_change_log"."id";



CREATE TABLE "public"."sys_config_group" (
    "id" bigint NOT NULL,
    "groupCode" character varying(64) NOT NULL,
    "groupName" character varying(128) NOT NULL,
    "module" character varying(64),
    "permissionCode" character varying(1024),
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "status" smallint DEFAULT '1'::smallint NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE SEQUENCE "public"."sys_config_group_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_config_group_id_seq" OWNED BY "public"."sys_config_group"."id";



CREATE SEQUENCE "public"."sys_config_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_config_id_seq" OWNED BY "public"."sys_config"."id";



CREATE TABLE "public"."sys_dept" (
    "id" bigint NOT NULL,
    "name" character varying(128) NOT NULL,
    "code" character varying(64) NOT NULL,
    "orgId" bigint NOT NULL,
    "parentId" bigint DEFAULT '0'::bigint NOT NULL,
    "leaderUserId" bigint,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "hierarchy" character varying(512),
    "level" integer DEFAULT 1 NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_dict_item" (
    "id" bigint NOT NULL,
    "dictTypeId" bigint NOT NULL,
    "itemValue" character varying(64) NOT NULL,
    "itemLabel" character varying(128) NOT NULL,
    "itemDesc" character varying(255),
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "status" smallint DEFAULT '1'::smallint NOT NULL,
    "extJson" "json",
    "createdBy" bigint NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedBy" bigint NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "colorToken" character varying(32),
    "iconToken" character varying(64),
    "presentationVersion" integer DEFAULT 1 NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL
);



CREATE SEQUENCE "public"."sys_dict_item_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_dict_item_id_seq" OWNED BY "public"."sys_dict_item"."id";



CREATE TABLE "public"."sys_dict_type" (
    "id" bigint NOT NULL,
    "dictCode" character varying(64) NOT NULL,
    "dictName" character varying(128) NOT NULL,
    "dictDesc" character varying(255),
    "module" character varying(64),
    "status" smallint DEFAULT '1'::smallint NOT NULL,
    "requiredLogin" boolean DEFAULT false NOT NULL,
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "isSystem" smallint DEFAULT '0'::smallint NOT NULL,
    "createdBy" bigint NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedBy" bigint NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "valueType" character varying(20) DEFAULT 'STRING'::character varying NOT NULL,
    "uiWidget" character varying(32) DEFAULT 'SELECT'::character varying NOT NULL,
    "validationJson" "text",
    "exposure" character varying(20) DEFAULT 'INTERNAL'::character varying NOT NULL,
    "sensitivity" character varying(20) DEFAULT 'NORMAL'::character varying NOT NULL,
    "schemaVersion" integer DEFAULT 1 NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL
);



CREATE SEQUENCE "public"."sys_dict_type_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_dict_type_id_seq" OWNED BY "public"."sys_dict_type"."id";



CREATE TABLE "public"."sys_enterprise_identity_authorization" (
    "id" bigint NOT NULL,
    "authorization_identity" character varying(128) NOT NULL,
    "purpose" character varying(32) NOT NULL,
    "organization_id" bigint NOT NULL,
    "connection_id" bigint NOT NULL,
    "connection_version" bigint NOT NULL,
    "connection_config_digest" character varying(128) NOT NULL,
    "bind_user_id" bigint,
    "requested_grants_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "requested_grants_digest" character varying(128) NOT NULL,
    "state_hash" character varying(128) NOT NULL,
    "nonce_hash" character varying(128) DEFAULT ''::character varying NOT NULL,
    "code_verifier_ciphertext" "text",
    "code_verifier_edek" "text",
    "code_verifier_wrap_key_ref" character varying(128),
    "redirect_after" character varying(1024) DEFAULT ''::character varying NOT NULL,
    "expires_at" timestamp with time zone NOT NULL,
    "status" character varying(32) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "result_identity_id" bigint,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "consumed_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_enterprise_identity_connection" (
    "id" bigint NOT NULL,
    "organization_id" bigint NOT NULL,
    "provider_type" character varying(32) NOT NULL,
    "connection_role" character varying(32) DEFAULT 'PRIMARY'::character varying NOT NULL,
    "display_name" character varying(128) NOT NULL,
    "app_id" character varying(255),
    "corp_id" character varying(255),
    "agent_id" character varying(255),
    "endpoint_profile" character varying(32) DEFAULT 'OFFICIAL_CN'::character varying NOT NULL,
    "client_secret_ciphertext" "text" NOT NULL,
    "client_secret_edek" "text" NOT NULL,
    "client_secret_wrap_key_ref" character varying(128) NOT NULL,
    "credential_version" bigint DEFAULT 1 NOT NULL,
    "binding_enabled" boolean DEFAULT true NOT NULL,
    "notification_direct_enabled" boolean DEFAULT true NOT NULL,
    "notification_mention_enabled" boolean DEFAULT true NOT NULL,
    "login_enabled" boolean DEFAULT false NOT NULL,
    "protocol_capabilities_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "config_digest" character varying(128) NOT NULL,
    "status" character varying(32) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "last_probe_status" character varying(32) DEFAULT 'NOT_PROBED'::character varying NOT NULL,
    "last_probe_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "last_probe_at" timestamp with time zone,
    "version" bigint DEFAULT 1 NOT NULL,
    "creator_id" bigint NOT NULL,
    "updater_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_enterprise_identity_grant" (
    "id" bigint NOT NULL,
    "organization_id" bigint NOT NULL,
    "external_identity_id" bigint NOT NULL,
    "grant_type" character varying(64) NOT NULL,
    "status" character varying(32) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "granted_by" bigint NOT NULL,
    "granted_at" timestamp with time zone NOT NULL,
    "revoked_by" bigint,
    "revoked_at" timestamp with time zone,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_external_managed_provider_command" (
    "providerCode" character varying(64) NOT NULL,
    "connectionVersion" character varying(128) NOT NULL,
    "requestHash" character(64) NOT NULL,
    "createdAt" timestamp with time zone NOT NULL
);



CREATE TABLE "public"."sys_federated_node" (
    "id" bigint NOT NULL,
    "nodeCode" character varying(64) NOT NULL,
    "nodeName" character varying(128) NOT NULL,
    "status" smallint DEFAULT '1'::smallint NOT NULL,
    "discoveryType" character varying(16) NOT NULL,
    "serviceName" character varying(128),
    "managementBaseUrl" character varying(2048),
    "hubIssuer" character varying(512) NOT NULL,
    "oidcClientId" character varying(128),
    "oidcClientSecretCiphertext" "text",
    "oidcClientSecretEdek" "text",
    "oidcClientSecretWrapKeyRef" character varying(128),
    "managementBearerCiphertext" "text",
    "managementBearerEdek" "text",
    "managementBearerWrapKeyRef" character varying(128),
    "capabilitiesJson" "text",
    "connectionStatus" character varying(16) DEFAULT 'PENDING'::character varying NOT NULL,
    "connectionVersion" character varying(128),
    "connectionRequestHash" character(64),
    "targetRevision" bigint DEFAULT '1'::bigint NOT NULL,
    "issuerLockedAt" timestamp with time zone,
    "lastConnectionError" character varying(512),
    "lastConnectionTraceId" character varying(128),
    "lastHealthyAt" timestamp with time zone,
    "createdAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL,
    "activeKey" smallint
);



CREATE TABLE "public"."sys_federated_node_connection_command" (
    "nodeCode" character varying(64) NOT NULL,
    "connectionVersion" character varying(128) NOT NULL,
    "requestHash" character(64) NOT NULL,
    "targetRevision" bigint NOT NULL,
    "terminalState" character varying(16) NOT NULL,
    "createdAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_file_binding_task" (
    "id" bigint NOT NULL,
    "fileId" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "bizType" integer NOT NULL,
    "bizId" bigint,
    "bindingToken" character varying(128) NOT NULL,
    "channel" character varying(16) NOT NULL,
    "status" character varying(32) NOT NULL,
    "attemptCount" integer DEFAULT 0 NOT NULL,
    "nextRetryTime" timestamp with time zone,
    "lastError" character varying(512),
    "fileName" character varying(255),
    "displayName" character varying(255),
    "visitStrategy" character varying(32),
    "accessScope" character varying(32),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_file_chunk_upload" (
    "id" bigint NOT NULL,
    "uploadId" character varying(64) NOT NULL,
    "userId" bigint NOT NULL,
    "fileName" character varying(255) NOT NULL,
    "contentType" character varying(128),
    "fileSize" bigint NOT NULL,
    "chunkSize" integer NOT NULL,
    "totalChunks" integer NOT NULL,
    "uploadedChunks" "text",
    "chunkSha256Map" "text",
    "fileSha256" character(64),
    "expectedCrc32c" character varying(16),
    "serverCrc32c" character varying(16),
    "storageStrategyId" bigint NOT NULL,
    "tempStoragePath" character varying(512),
    "cloudUploadId" character varying(128),
    "partETagsMap" "text",
    "bizType" character varying(50),
    "bizId" bigint,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "expireTime" timestamp with time zone NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "scopeId" character varying(128),
    "uploadTaskId" character varying(64)
);



CREATE TABLE "public"."sys_file_info" (
    "id" bigint NOT NULL,
    "fileInnerName" character varying(255) NOT NULL,
    "fileSize" bigint NOT NULL,
    "fileSha256" character(64),
    "fileCrc32c" character varying(16),
    "hashAlgorithm" character varying(32),
    "contentType" character varying(64) NOT NULL,
    "fileMetadata" "text",
    "thumbnailData" "text",
    "storageStrategyId" bigint,
    "storagePath" character varying(255) DEFAULT ''::character varying NOT NULL,
    "status" character varying(32),
    "scanStatus" character varying(32),
    "integrityStatus" character varying(32),
    "integrityCheckedAt" timestamp with time zone,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "deletedTime" timestamp with time zone
);



CREATE TABLE "public"."sys_file_integrity_audit" (
    "id" bigint NOT NULL,
    "fileId" bigint NOT NULL,
    "storageStrategyId" bigint,
    "expectedSha256" character(64),
    "actualSha256" character(64),
    "status" character varying(32) NOT NULL,
    "errorMsg" character varying(512),
    "auditTime" timestamp with time zone NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_file_process_run" (
    "id" bigint NOT NULL,
    "taskId" bigint NOT NULL,
    "fileId" bigint NOT NULL,
    "taskType" character varying(32) NOT NULL,
    "status" smallint NOT NULL,
    "attempt" integer DEFAULT 0 NOT NULL,
    "errorMsg" "text",
    "resultData" "text",
    "startedAt" timestamp with time zone NOT NULL,
    "finishedAt" timestamp with time zone,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_file_process_task" (
    "id" bigint NOT NULL,
    "fileId" bigint NOT NULL,
    "taskType" character varying(32) NOT NULL,
    "taskParams" "text",
    "pipelineId" character varying(64),
    "nodeId" character varying(64),
    "idempotencyKey" character varying(128),
    "dedupKey" character varying(128),
    "replayToken" character varying(128),
    "dependsOn" character varying(512),
    "attempt" integer DEFAULT 0 NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "retryCount" integer DEFAULT 0 NOT NULL,
    "maxRetry" integer DEFAULT 3 NOT NULL,
    "errorMsg" "text",
    "resultData" "text",
    "priority" integer DEFAULT 0 NOT NULL,
    "mqMessageId" character varying(128),
    "nextRetryTime" timestamp with time zone,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "startTime" timestamp with time zone,
    "finishTime" timestamp with time zone
);



CREATE TABLE "public"."sys_file_reference" (
    "id" bigint NOT NULL,
    "fileId" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "displayName" character varying(128) NOT NULL,
    "bizType" character varying(50) NOT NULL,
    "bizId" bigint NOT NULL,
    "visitUrl" character varying(255),
    "accessLevel" smallint DEFAULT '0'::smallint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "visitStrategy" character varying(32) DEFAULT 'PRIVATE_PREVIEW'::character varying NOT NULL,
    "accessScope" character varying(32) DEFAULT 'OWNER_ONLY'::character varying NOT NULL,
    "scopeId" character varying(128)
);



CREATE TABLE "public"."sys_menu" (
    "id" bigint NOT NULL,
    "name" character varying(64) NOT NULL,
    "parentId" bigint DEFAULT '0'::bigint NOT NULL,
    "sortOrder" integer DEFAULT 0,
    "path" character varying(200) DEFAULT ''::character varying,
    "component" character varying(255),
    "icon" character varying(100),
    "type" character(1) NOT NULL,
    "permission" character varying(100),
    "featureCode" character varying(64),
    "isFrame" boolean DEFAULT false,
    "isCache" boolean DEFAULT false,
    "visible" boolean DEFAULT true,
    "hierarchy" character varying(500),
    "level" integer DEFAULT 0,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "remark" character varying(255),
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_menu_permission" (
    "id" bigint NOT NULL,
    "menuId" bigint NOT NULL,
    "permissionId" bigint NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_message_consume_log" (
    "id" bigint NOT NULL,
    "messageId" character varying(128) NOT NULL,
    "consumer" character varying(64) NOT NULL,
    "status" character varying(32) NOT NULL,
    "detail" character varying(512),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "leaseOwner" character varying(128),
    "leaseToken" character varying(64),
    "leaseUntil" timestamp with time zone,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification" (
    "id" bigint NOT NULL,
    "notificationId" character varying(96) NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "eventKey" character varying(128) NOT NULL,
    "idempotencyKey" character varying(191) NOT NULL,
    "requestFingerprint" character(64) NOT NULL,
    "audienceJson" "jsonb" NOT NULL,
    "category" character varying(64) NOT NULL,
    "priority" character varying(32) NOT NULL,
    "mandatory" boolean DEFAULT false NOT NULL,
    "title" character varying(512) NOT NULL,
    "content" "text" NOT NULL,
    "deepLink" character varying(512),
    "actionType" character varying(32) DEFAULT ''::character varying NOT NULL,
    "actionLabelSnapshot" character varying(128) DEFAULT ''::character varying NOT NULL,
    "actionStyle" character varying(32) DEFAULT ''::character varying NOT NULL,
    "scheduleAt" timestamp with time zone,
    "expiresAt" timestamp with time zone,
    "traceId" character varying(128),
    "status" character varying(32) NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "organizationId" bigint
);



CREATE TABLE "public"."sys_notification_channel" (
    "id" bigint NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "channelName" character varying(128) NOT NULL,
    "channelType" character varying(32) NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "priority" integer DEFAULT 100 NOT NULL,
    "configJson" "json",
    "secretCiphertext" "text",
    "secretEdek" "text",
    "secretWrapKeyRef" character varying(128),
    "rateLimitJson" "json",
    "metadataJson" "json",
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL,
    "scopeId" character varying(128),
    "scopeType" character varying(32) DEFAULT 'SYSTEM'::character varying NOT NULL,
    "organizationId" bigint,
    "enterpriseConnectionId" bigint,
    "enterpriseConnectionVersion" bigint DEFAULT 0 NOT NULL,
    "enterpriseConnectionDigest" character(64) DEFAULT ''::"bpchar" NOT NULL
);



CREATE TABLE "public"."sys_notification_delivery" (
    "id" bigint NOT NULL,
    "deliveryId" character varying(96) NOT NULL,
    "requestDigest" character varying(128) NOT NULL,
    "sceneCode" character varying(64) NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "channelType" character varying(32) NOT NULL,
    "templateCode" character varying(96) NOT NULL,
    "target" character varying(512),
    "targetMasked" character varying(512),
    "payloadJson" "json",
    "renderedSubject" character varying(512),
    "renderedText" "text",
    "renderedHtml" "text",
    "renderedMarkdown" "text",
    "status" character varying(24) DEFAULT 'PENDING'::character varying NOT NULL,
    "retryCount" integer DEFAULT 0 NOT NULL,
    "maxRetry" integer DEFAULT 3 NOT NULL,
    "nextRetryAt" timestamp with time zone,
    "lastError" "text",
    "traceId" character varying(64),
    "sentAt" timestamp with time zone,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL,
    "organizationId" bigint,
    "notificationId" bigint,
    "externalTargetId" bigint,
    "providerReference" character varying(191),
    "presentationType" character varying(32) DEFAULT 'TEXT'::character varying NOT NULL,
    "actionLabelSnapshot" character varying(128) DEFAULT ''::character varying NOT NULL,
    "actionUrlSnapshot" character varying(1024) DEFAULT ''::character varying NOT NULL,
    "renderedPayloadDigest" character(64) DEFAULT ''::"bpchar" NOT NULL,
    "sceneSnapshotId" bigint,
    "contentTier" character varying(32) DEFAULT 'SENSITIVE'::character varying NOT NULL
);



CREATE TABLE "public"."sys_notification_delivery_attempt" (
    "id" bigint NOT NULL,
    "attemptId" character varying(96) NOT NULL,
    "deliveryId" character varying(96) NOT NULL,
    "attemptNo" integer NOT NULL,
    "status" character varying(32) NOT NULL,
    "failureClass" character varying(64),
    "providerReference" character varying(191),
    "diagnostic" character varying(128),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_delivery_diagnostic_audit" (
    "id" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "deliveryId" character varying(128) NOT NULL,
    "actorId" bigint NOT NULL,
    "contentTier" character varying(32) NOT NULL,
    "reasonCode" character varying(32) NOT NULL,
    "ticketReference" character varying(128),
    "resultCode" character varying(48) NOT NULL,
    "traceId" character varying(128),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_delivery_ephemeral_content" (
    "id" bigint NOT NULL,
    "deliveryId" character varying(128) NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "ciphertext" "text" NOT NULL,
    "edek" "text" NOT NULL,
    "wrapKeyRef" character varying(128) NOT NULL,
    "expiresAt" timestamp with time zone NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_external_target" (
    "id" bigint NOT NULL,
    "externalTargetId" character varying(96) NOT NULL,
    "notificationId" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "organizationId" bigint,
    "userId" bigint,
    "externalIdentityId" bigint,
    "externalIdentityVersion" bigint DEFAULT 0 NOT NULL,
    "enterpriseConnectionId" bigint,
    "enterpriseConnectionVersion" bigint DEFAULT 0 NOT NULL,
    "authorityDigest" character varying(128) DEFAULT ''::character varying NOT NULL,
    "grantType" character varying(32) DEFAULT ''::character varying NOT NULL,
    "connectionRef" character varying(64) NOT NULL,
    "providerCode" character varying(32) NOT NULL,
    "identityKind" character varying(32) NOT NULL,
    "subjectCiphertext" "text" NOT NULL,
    "subjectEdek" "text" NOT NULL,
    "subjectWrapKeyRef" character varying(191) NOT NULL,
    "subjectDigest" character(64) NOT NULL,
    "subjectDigestKeyRef" character varying(191) NOT NULL,
    "providerParamsJson" "jsonb",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_group_route" (
    "id" bigint NOT NULL,
    "organization_id" bigint NOT NULL,
    "display_name" "text" NOT NULL,
    "provider_type" "text" NOT NULL,
    "transport_type" "text" NOT NULL,
    "delivery_channel_id" bigint NOT NULL,
    "required_permission_code" "text" DEFAULT 'workforce:task:create'::"text" NOT NULL,
    "target_ciphertext" "text" DEFAULT ''::"text" NOT NULL,
    "target_edek" "text" DEFAULT ''::"text" NOT NULL,
    "target_wrap_key_ref" "text" DEFAULT ''::"text" NOT NULL,
    "target_digest" "text" DEFAULT ''::"text" NOT NULL,
    "status" "text" DEFAULT 'DRAFT'::"text" NOT NULL,
    "route_version" bigint DEFAULT 1 NOT NULL,
    "channel_version" bigint DEFAULT 1 NOT NULL,
    "connection_version" bigint,
    "config_digest" "text" NOT NULL,
    "capabilities_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "last_probe_status" "text" DEFAULT 'NOT_PROBED'::"text" NOT NULL,
    "last_probe_reason_code" "text" DEFAULT ''::"text" NOT NULL,
    "last_probe_at" timestamp with time zone,
    "creator_id" bigint NOT NULL,
    "updater_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "sys_notification_group_route_capabilities_json_check" CHECK (("jsonb_typeof"("capabilities_json") = 'array'::"text")),
    CONSTRAINT "sys_notification_group_route_channel_version_check" CHECK (("channel_version" > 0)),
    CONSTRAINT "sys_notification_group_route_check" CHECK (((("transport_type" = 'FEISHU_APP_CHAT'::"text") AND ("target_ciphertext" <> ''::"text") AND ("target_edek" <> ''::"text") AND ("target_wrap_key_ref" <> ''::"text") AND ("target_digest" <> ''::"text") AND ("connection_version" IS NOT NULL)) OR (("transport_type" = ANY (ARRAY['FEISHU_WEBHOOK'::"text", 'WECOM_WEBHOOK'::"text"])) AND ("target_ciphertext" = ''::"text") AND ("target_edek" = ''::"text") AND ("target_wrap_key_ref" = ''::"text") AND ("target_digest" = ''::"text") AND ("connection_version" IS NULL)))),
    CONSTRAINT "sys_notification_group_route_connection_version_check" CHECK (("connection_version" > 0)),
    CONSTRAINT "sys_notification_group_route_provider_type_check" CHECK (("provider_type" = ANY (ARRAY['FEISHU'::"text", 'WECOM'::"text"]))),
    CONSTRAINT "sys_notification_group_route_route_version_check" CHECK (("route_version" > 0)),
    CONSTRAINT "sys_notification_group_route_status_check" CHECK (("status" = ANY (ARRAY['DRAFT'::"text", 'VERIFIED'::"text", 'ENABLED'::"text", 'DISABLED'::"text", 'RETIRED'::"text"]))),
    CONSTRAINT "sys_notification_group_route_transport_type_check" CHECK (("transport_type" = ANY (ARRAY['FEISHU_APP_CHAT'::"text", 'FEISHU_WEBHOOK'::"text", 'WECOM_WEBHOOK'::"text"])))
);



CREATE TABLE "public"."sys_notification_group_route_candidate" (
    "id" bigint NOT NULL,
    "token_hash" "text" NOT NULL,
    "organization_id" bigint NOT NULL,
    "connection_id" bigint NOT NULL,
    "connection_version" bigint NOT NULL,
    "connection_config_digest" "text" NOT NULL,
    "display_name" "text" NOT NULL,
    "description" "text" DEFAULT ''::"text" NOT NULL,
    "target_ciphertext" "text" NOT NULL,
    "target_edek" "text" NOT NULL,
    "target_wrap_key_ref" "text" NOT NULL,
    "target_digest" "text" NOT NULL,
    "expires_at" timestamp with time zone NOT NULL,
    "consumed_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_group_route_snapshot" (
    "id" bigint NOT NULL,
    "notification_id" "text" NOT NULL,
    "delivery_id" "text" NOT NULL,
    "organization_id" bigint NOT NULL,
    "route_id" bigint NOT NULL,
    "route_version" bigint NOT NULL,
    "provider_type" "text" NOT NULL,
    "transport_type" "text" NOT NULL,
    "route_config_digest" "text" NOT NULL,
    "delivery_channel_id" bigint NOT NULL,
    "channel_version" bigint NOT NULL,
    "enterprise_connection_version" bigint,
    "target_ciphertext" "text" DEFAULT ''::"text" NOT NULL,
    "target_edek" "text" DEFAULT ''::"text" NOT NULL,
    "target_wrap_key_ref" "text" DEFAULT ''::"text" NOT NULL,
    "target_digest" "text" DEFAULT ''::"text" NOT NULL,
    "presentation" "text" NOT NULL,
    "action_url_snapshot" "text" DEFAULT ''::"text" NOT NULL,
    "mention_requested" boolean DEFAULT false NOT NULL,
    "mention_user_id" bigint,
    "mention_identity_id" bigint,
    "mention_identity_version" bigint,
    "mention_authority_digest" "text" DEFAULT ''::"text" NOT NULL,
    "warning_code" "text" DEFAULT ''::"text" NOT NULL,
    "rendered_payload_digest" "text" DEFAULT ''::"text" NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_http_delivery_snapshot" (
    "id" bigint NOT NULL,
    "deliveryId" character varying(96) NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "channelType" character varying(32) NOT NULL,
    "channelPriority" integer DEFAULT 100 NOT NULL,
    "configJson" "jsonb" NOT NULL,
    "secretCiphertext" "text",
    "secretEdek" "text",
    "secretWrapKeyRef" character varying(191),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_materialization_task" (
    "id" bigint NOT NULL,
    "taskId" character varying(96) NOT NULL,
    "notificationId" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "audienceJson" "jsonb" NOT NULL,
    "materializationCursor" character varying(512) NOT NULL,
    "status" character varying(32) NOT NULL,
    "materializedCount" bigint DEFAULT 0 NOT NULL,
    "retryCount" integer DEFAULT 0 NOT NULL,
    "nextRunAt" timestamp with time zone NOT NULL,
    "leaseOwner" character varying(128),
    "leaseToken" character varying(64),
    "leaseUntil" timestamp with time zone,
    "lastError" "text",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_recipient" (
    "id" bigint NOT NULL,
    "recipientId" character varying(96) NOT NULL,
    "notificationId" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "userId" bigint NOT NULL,
    "eventKey" character varying(128) NOT NULL,
    "category" character varying(64) NOT NULL,
    "priority" character varying(32) NOT NULL,
    "mandatory" boolean DEFAULT false NOT NULL,
    "title" character varying(512) NOT NULL,
    "content" "text" NOT NULL,
    "deepLink" character varying(512),
    "actionType" character varying(32) DEFAULT ''::character varying NOT NULL,
    "actionLabelSnapshot" character varying(128) DEFAULT ''::character varying NOT NULL,
    "actionStyle" character varying(32) DEFAULT ''::character varying NOT NULL,
    "expiresAt" timestamp with time zone,
    "firstSeenAt" timestamp with time zone,
    "readAt" timestamp with time zone,
    "archivedAt" timestamp with time zone,
    "mailboxVersion" bigint NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "expiredAt" timestamp with time zone
);



CREATE TABLE "public"."sys_notification_scene_binding" (
    "id" bigint NOT NULL,
    "sceneCode" character varying(64) NOT NULL,
    "sceneName" character varying(128) NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "templateCode" character varying(96) NOT NULL,
    "enabled" smallint DEFAULT '1'::smallint NOT NULL,
    "priority" integer DEFAULT 100 NOT NULL,
    "maxRetry" integer DEFAULT 3 NOT NULL,
    "retryIntervalSeconds" integer DEFAULT 60 NOT NULL,
    "metadataJson" "json",
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL,
    "scopeId" character varying(128)
);



CREATE TABLE "public"."sys_notification_scene_definition" (
    "id" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "sceneCode" character varying(96) NOT NULL,
    "sceneName" character varying(128) NOT NULL,
    "receiverKind" character varying(32) NOT NULL,
    "currentDraftRevisionId" bigint,
    "currentPublishedRevisionId" bigint,
    "version" integer DEFAULT 1 NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT 0 NOT NULL
);



CREATE TABLE "public"."sys_notification_scene_revision" (
    "id" bigint NOT NULL,
    "sceneDefinitionId" bigint NOT NULL,
    "revisionNo" integer NOT NULL,
    "state" character varying(16) NOT NULL,
    "revisionVersion" integer DEFAULT 1 NOT NULL,
    "enabled" boolean DEFAULT true NOT NULL,
    "templateRevisionId" bigint NOT NULL,
    "connectionRef" character varying(64),
    "connectionDigest" character varying(128),
    "publishedAt" timestamp with time zone,
    "publishedBy" bigint,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_scene_revision_audit" (
    "id" bigint NOT NULL,
    "sceneDefinitionId" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "action" character varying(48) NOT NULL,
    "fromRevisionNo" integer,
    "toRevisionNo" integer,
    "errorCode" character varying(64),
    "actorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_scene_snapshot" (
    "id" bigint NOT NULL,
    "notificationId" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "sceneCode" character varying(96) NOT NULL,
    "receiverKind" character varying(32) NOT NULL,
    "sceneDefinitionId" bigint NOT NULL,
    "sceneRevisionId" bigint NOT NULL,
    "templateDefinitionId" bigint NOT NULL,
    "templateRevisionId" bigint NOT NULL,
    "connectionRef" character varying(64),
    "connectionDigest" character varying(128),
    "templateContentDigest" character varying(128) NOT NULL,
    "renderedDigest" character varying(128) NOT NULL,
    "variableDigest" character varying(128) NOT NULL,
    "resolution" character varying(32) NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_template" (
    "id" bigint NOT NULL,
    "templateCode" character varying(96) NOT NULL,
    "templateName" character varying(128) NOT NULL,
    "sceneCode" character varying(64) NOT NULL,
    "channelType" character varying(32) NOT NULL,
    "locale" character varying(32) DEFAULT 'zh-CN'::character varying NOT NULL,
    "subjectTemplate" character varying(512),
    "textTemplate" "text",
    "htmlTemplate" "text",
    "markdownTemplate" "text",
    "jsonTemplate" "json",
    "variablesJson" "json",
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "version" integer DEFAULT 1 NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL,
    "scopeId" character varying(128)
);



CREATE TABLE "public"."sys_notification_template_definition" (
    "id" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "templateCode" character varying(96) NOT NULL,
    "templateName" character varying(128) NOT NULL,
    "locale" character varying(32) DEFAULT 'zh-CN'::character varying NOT NULL,
    "currentDraftRevisionId" bigint,
    "currentPublishedRevisionId" bigint,
    "version" integer DEFAULT 1 NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT 0 NOT NULL
);



CREATE TABLE "public"."sys_notification_template_revision" (
    "id" bigint NOT NULL,
    "templateDefinitionId" bigint NOT NULL,
    "revisionNo" integer NOT NULL,
    "state" character varying(16) NOT NULL,
    "revisionVersion" integer DEFAULT 1 NOT NULL,
    "subjectTemplate" character varying(512),
    "textTemplate" "text",
    "htmlTemplate" "text",
    "markdownTemplate" "text",
    "primaryActionLabelTemplate" character varying(128) DEFAULT ''::character varying NOT NULL,
    "variableSchemaJson" "jsonb" NOT NULL,
    "contentDigest" character varying(128) NOT NULL,
    "publishedAt" timestamp with time zone,
    "publishedBy" bigint,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_template_revision_audit" (
    "id" bigint NOT NULL,
    "templateDefinitionId" bigint NOT NULL,
    "scopeId" character varying(128) NOT NULL,
    "action" character varying(48) NOT NULL,
    "fromRevisionNo" integer,
    "toRevisionNo" integer,
    "actorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_notification_user_preferences" (
    "id" bigint NOT NULL,
    "organization_id" bigint NOT NULL,
    "user_id" bigint NOT NULL,
    "event_family" "text" NOT NULL,
    "channel_kind" "text" NOT NULL,
    "enabled" boolean DEFAULT true NOT NULL,
    "version" bigint DEFAULT 1 NOT NULL,
    "creator_id" bigint NOT NULL,
    "updater_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "sys_notification_user_preferences_channel_kind_check" CHECK (("channel_kind" = ANY (ARRAY['IN_APP'::"text", 'EMAIL_USER'::"text", 'FEISHU_DIRECT'::"text", 'WECOM_DIRECT'::"text"]))),
    CONSTRAINT "sys_notification_user_preferences_event_family_check" CHECK (("event_family" = ANY (ARRAY['TASK_ACTION_REQUIRED'::"text", 'TASK_INTERRUPTED'::"text", 'TASK_COMPLETED'::"text"]))),
    CONSTRAINT "sys_notification_user_preferences_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."sys_operation_log" (
    "id" bigint NOT NULL,
    "userId" bigint,
    "userName" character varying(64),
    "nickName" character varying(64),
    "operationType" character varying(64) NOT NULL,
    "operationDesc" character varying(255),
    "methodName" character varying(255),
    "requestMethod" character varying(16),
    "requestUrl" character varying(512),
    "traceId" character varying(64),
    "requestParams" "text",
    "responseResult" "text",
    "requestIp" character varying(64),
    "requestLocation" character varying(128),
    "userAgent" character varying(512),
    "browser" character varying(64),
    "os" character varying(64),
    "operationTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "executionTime" bigint,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "errorMsg" "text",
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE SEQUENCE "public"."sys_operation_log_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_operation_log_id_seq" OWNED BY "public"."sys_operation_log"."id";



CREATE TABLE "public"."sys_org" (
    "id" bigint NOT NULL,
    "code" character varying(64) NOT NULL,
    "name" character varying(128) NOT NULL,
    "parentId" bigint DEFAULT '0'::bigint NOT NULL,
    "hierarchy" character varying(500),
    "level" integer DEFAULT 0,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "leaderUserId" bigint,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_outbox_event" (
    "id" bigint NOT NULL,
    "eventId" character varying(64) NOT NULL,
    "eventType" character varying(64) NOT NULL,
    "aggregateType" character varying(64) NOT NULL,
    "aggregateId" character varying(64) NOT NULL,
    "payload" "text" NOT NULL,
    "status" character varying(16) DEFAULT 'PENDING'::character varying NOT NULL,
    "retryCount" integer DEFAULT 0 NOT NULL,
    "nextRetryAt" timestamp with time zone,
    "errorMsg" character varying(512),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "eventOwner" character varying(64) DEFAULT 'unassigned'::character varying NOT NULL,
    "leaseOwner" character varying(128),
    "leaseToken" character varying(64),
    "leaseUntil" timestamp with time zone,
    "scopeId" character varying(128)
);



CREATE TABLE "public"."sys_permission" (
    "id" bigint NOT NULL,
    "code" character varying(100) NOT NULL,
    "featureCode" character varying(64),
    "name" character varying(100) NOT NULL,
    "resourceType" character varying(30) DEFAULT 'API'::character varying NOT NULL,
    "method" character varying(10),
    "path" character varying(255),
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "description" character varying(255),
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_post" (
    "id" bigint NOT NULL,
    "code" character varying(64) NOT NULL,
    "name" character varying(128) NOT NULL,
    "deptId" bigint DEFAULT '0'::bigint NOT NULL,
    "orgId" bigint DEFAULT '0'::bigint NOT NULL,
    "hierarchy" character varying(500),
    "level" integer DEFAULT 0,
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "remark" character varying(512),
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_post_role" (
    "id" bigint NOT NULL,
    "postId" bigint NOT NULL,
    "roleId" bigint NOT NULL
);



CREATE SEQUENCE "public"."sys_post_role_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_post_role_id_seq" OWNED BY "public"."sys_post_role"."id";



CREATE TABLE "public"."sys_role" (
    "id" bigint NOT NULL,
    "name" character varying(64) NOT NULL,
    "code" character varying(100) NOT NULL,
    "systemKey" character varying(64),
    "dataScope" smallint DEFAULT '1'::smallint NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "type" smallint DEFAULT '0'::smallint NOT NULL,
    "hierarchy" character varying(500),
    "level" integer DEFAULT 0,
    "sortOrder" integer DEFAULT 0,
    "remark" character varying(500),
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "grantRevision" bigint DEFAULT 0 NOT NULL
);



CREATE TABLE "public"."sys_role_config_scope" (
    "id" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "groupCode" character varying(64) NOT NULL,
    "configKey" character varying(128) DEFAULT ''::character varying NOT NULL,
    "canRead" boolean DEFAULT true NOT NULL,
    "canWrite" boolean DEFAULT false NOT NULL,
    "canDelete" boolean DEFAULT false NOT NULL,
    "createdBy" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedBy" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_role_dept" (
    "id" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "deptId" bigint NOT NULL
);



CREATE TABLE "public"."sys_role_grant_request" (
    "roleId" bigint NOT NULL,
    "idempotencyKey" character varying(128) NOT NULL,
    "requestHash" character(64) NOT NULL,
    "resultRevision" bigint NOT NULL,
    "impactedUserCount" integer DEFAULT 0 NOT NULL,
    "changed" smallint DEFAULT 0 NOT NULL,
    "operatorId" bigint,
    "createTime" timestamp without time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_role_menu" (
    "id" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "menuId" bigint NOT NULL,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_role_permission" (
    "id" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "permissionId" bigint NOT NULL,
    "source" character varying(16) DEFAULT 'DIRECT'::character varying NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_security_bootstrap" (
    "bootstrapKey" character varying(64) NOT NULL,
    "rootRoleId" bigint NOT NULL,
    "rootRoleCode" character varying(50) NOT NULL,
    "initializedAt" timestamp with time zone NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."sys_storage_alert_log" (
    "id" bigint NOT NULL,
    "strategyId" bigint NOT NULL,
    "alertType" character varying(32) NOT NULL,
    "alertLevel" character varying(16) NOT NULL,
    "message" character varying(512) NOT NULL,
    "status" character varying(16) DEFAULT 'OPEN'::character varying NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);



CREATE TABLE "public"."sys_storage_strategy" (
    "id" bigint NOT NULL,
    "strategyName" character varying(64) NOT NULL,
    "providerType" character varying(32) NOT NULL,
    "isDefault" boolean DEFAULT false NOT NULL,
    "isEnabled" boolean DEFAULT true NOT NULL,
    "runState" character varying(16) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "priority" integer DEFAULT 0 NOT NULL,
    "configCiphertext" "text" NOT NULL,
    "configEdek" character varying(512) NOT NULL,
    "wrapKeyRef" character varying(64) NOT NULL,
    "healthCheckUrl" character varying(255),
    "healthStatus" smallint DEFAULT '1'::smallint,
    "lastHealthCheck" timestamp with time zone,
    "failureCount" integer DEFAULT 0 NOT NULL,
    "totalRequests" integer DEFAULT 0 NOT NULL,
    "failureRateThreshold" numeric(5,2) DEFAULT 10.00 NOT NULL,
    "windowStartTime" timestamp with time zone,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_upload_task" (
    "id" character varying(64) NOT NULL,
    "userId" bigint NOT NULL,
    "bizType" integer,
    "bizId" bigint,
    "fileName" character varying(255),
    "contentType" character varying(128),
    "storageStrategyId" bigint,
    "objectKeyStaging" character varying(512) NOT NULL,
    "objectKeyClean" character varying(512) NOT NULL,
    "status" character varying(32) NOT NULL,
    "uploadMode" character varying(16),
    "multipartUploadId" character varying(128),
    "partSize" integer,
    "totalParts" integer,
    "expectedSize" bigint,
    "expectedSha256" character(64),
    "expectedCrc32c" character varying(16),
    "actualSize" bigint,
    "etag" character varying(128),
    "serverCrc32c" character varying(16),
    "failureCategory" character varying(32),
    "failureReason" character varying(512),
    "fileId" bigint,
    "bindingToken" character varying(128),
    "bindingChannel" character varying(64),
    "expireAt" timestamp with time zone,
    "userIp" character varying(64),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "scopeId" character varying(128),
    "credentialId" character varying(64),
    "credentialVersion" integer DEFAULT 0 NOT NULL,
    "protectedUntil" timestamp with time zone,
    "credentialExpireAt" timestamp with time zone,
    "revokedAt" timestamp with time zone
);



CREATE TABLE "public"."sys_user" (
    "id" bigint NOT NULL,
    "userAccount" character varying(30) NOT NULL,
    "nickName" character varying(30) NOT NULL,
    "password" character varying(100) DEFAULT ''::character varying,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "statusVersion" bigint DEFAULT '0'::bigint NOT NULL,
    "statusCommandHash" character(64),
    "creatorId" bigint,
    "updaterId" bigint,
    "userPhone" character varying(11),
    "userEmail" character varying(254) DEFAULT ''::character varying NOT NULL,
    "userGender" boolean DEFAULT false NOT NULL,
    "userAvatar" character varying(1024),
    "userProfile" character varying(1024),
    "unsealTime" timestamp with time zone,
    "deletionTime" timestamp with time zone,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "registerPlatformCode" character varying(64),
    "registerProviderCode" character varying(64),
    "email_verified_at" timestamp with time zone,
    "email_version" bigint DEFAULT 1 NOT NULL,
    "email_verification_source" character varying(64) DEFAULT ''::character varying NOT NULL,
    CONSTRAINT "chk_sys_user_email_version_positive" CHECK (("email_version" > 0))
);



CREATE TABLE "public"."sys_user_credential" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "credentialType" character varying(32) NOT NULL,
    "credentialKey" character varying(255) NOT NULL,
    "secretHash" character varying(255),
    "secretCiphertext" "text",
    "credentialPayloadJson" "text",
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "verifiedAt" timestamp with time zone,
    "lastUsedAt" timestamp with time zone,
    "invalidatedAt" timestamp with time zone,
    "metadataJson" "text",
    "mustChangePassword" boolean DEFAULT false NOT NULL,
    "passwordChangedAt" timestamp with time zone,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE TABLE "public"."sys_user_dept" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "deptId" bigint NOT NULL,
    "isPrimary" boolean DEFAULT false NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE SEQUENCE "public"."sys_user_dept_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_user_dept_id_seq" OWNED BY "public"."sys_user_dept"."id";



CREATE SEQUENCE "public"."sys_user_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_user_id_seq" OWNED BY "public"."sys_user"."id";



CREATE TABLE "public"."sys_user_org" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "orgId" bigint NOT NULL,
    "isPrimary" boolean DEFAULT false NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE SEQUENCE "public"."sys_user_org_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_user_org_id_seq" OWNED BY "public"."sys_user_org"."id";



CREATE TABLE "public"."sys_user_permission" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "permissionId" bigint NOT NULL,
    "type" boolean DEFAULT true NOT NULL,
    "expireTime" timestamp with time zone,
    "source" character varying(100),
    "grantedBy" bigint,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL,
    "reason" character varying(500)
);



CREATE SEQUENCE "public"."sys_user_permission_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_user_permission_id_seq" OWNED BY "public"."sys_user_permission"."id";



CREATE TABLE "public"."sys_user_position" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "postId" bigint NOT NULL,
    "isPrimary" boolean DEFAULT false NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE SEQUENCE "public"."sys_user_position_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_user_position_id_seq" OWNED BY "public"."sys_user_position"."id";



CREATE TABLE "public"."sys_user_role" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    "isDeleted" boolean DEFAULT false NOT NULL
);



CREATE SEQUENCE "public"."sys_user_role_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."sys_user_role_id_seq" OWNED BY "public"."sys_user_role"."id";



CREATE TABLE "public"."workforce_agent_interaction_responses" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "interaction_id" bigint NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "idempotency_key" character varying(128) NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "answers_json" "jsonb" NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    CONSTRAINT "workforce_agent_interaction_responses_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_agent_interaction_responses_answers_json_check" CHECK ((("jsonb_typeof"("answers_json") = 'array'::"text") AND ("octet_length"(("answers_json")::"text") <= 18432))),
    CONSTRAINT "workforce_agent_interaction_responses_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_agent_interaction_responses_idempotency_key_check" CHECK ((("octet_length"(("idempotency_key")::"text") >= 1) AND ("octet_length"(("idempotency_key")::"text") <= 128))),
    CONSTRAINT "workforce_agent_interaction_responses_request_digest_check" CHECK ((("request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_agent_interaction_responses_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_agent_interaction_responses_tenant_id_check" CHECK (("tenant_id" > 0))
);



CREATE TABLE "public"."workforce_agent_interactions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint,
    "attempt_id" bigint,
    "adapter_session_generation" integer,
    "local_request_key" character varying(128) NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "kind" character varying(32) NOT NULL,
    "is_blocking" boolean DEFAULT true NOT NULL,
    "state" character varying(16) NOT NULL,
    "delivery_state" character varying(16) NOT NULL,
    "version" integer NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "body_json" "jsonb" NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "expires_at" timestamp with time zone DEFAULT (CURRENT_TIMESTAMP + '24:00:00'::interval) NOT NULL,
    CONSTRAINT "workforce_agent_interactions_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_agent_interactions_adapter_session_generation_check" CHECK (("adapter_session_generation" > 0)),
    CONSTRAINT "workforce_agent_interactions_attempt_id_check" CHECK (("attempt_id" > 0)),
    CONSTRAINT "workforce_agent_interactions_body_json_check" CHECK ((("jsonb_typeof"("body_json") = 'object'::"text") AND ("octet_length"(("body_json")::"text") <= 65536))),
    CONSTRAINT "workforce_agent_interactions_check" CHECK (("expires_at" > "created_at")),
    CONSTRAINT "workforce_agent_interactions_check1" CHECK (((("kind")::"text" <> 'QUESTION'::"text") OR (("task_turn_id" IS NOT NULL) AND ("attempt_id" IS NOT NULL) AND ("adapter_session_generation" IS NOT NULL)))),
    CONSTRAINT "workforce_agent_interactions_delivery_state_check" CHECK ((("delivery_state")::"text" = ANY ((ARRAY['NONE'::character varying, 'SAVED'::character varying, 'DELIVERED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_agent_interactions_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_agent_interactions_kind_check" CHECK ((("kind")::"text" = ANY ((ARRAY['QUESTION'::character varying, 'USER_MESSAGE'::character varying])::"text"[]))),
    CONSTRAINT "workforce_agent_interactions_local_request_key_check" CHECK ((("octet_length"(("local_request_key")::"text") >= 1) AND ("octet_length"(("local_request_key")::"text") <= 128))),
    CONSTRAINT "workforce_agent_interactions_request_digest_check" CHECK ((("request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_agent_interactions_state_check" CHECK ((("state")::"text" = ANY ((ARRAY['OPEN'::character varying, 'RESOLVED'::character varying, 'EXPIRED'::character varying, 'CANCELLED'::character varying, 'SUPERSEDED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_agent_interactions_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_agent_interactions_task_turn_id_check" CHECK (("task_turn_id" > 0)),
    CONSTRAINT "workforce_agent_interactions_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_agent_interactions_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_artifact_upload_attempts" (
    "id" bigint NOT NULL,
    "finalization_run_id" bigint,
    "materialization_run_id" bigint,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "artifact_type" character varying(64) NOT NULL,
    "artifact_name" "text" NOT NULL,
    "object_key" "text" NOT NULL,
    "sha256" character varying(64) NOT NULL,
    "size_bytes" bigint NOT NULL,
    "media_type" character varying(255) NOT NULL,
    "attempt_number" integer DEFAULT 1 NOT NULL,
    "max_attempts" integer DEFAULT 5 NOT NULL,
    "worker_id" bigint NOT NULL,
    "host_boot_id" "text" NOT NULL,
    "status" character varying(32) NOT NULL,
    "lease_sha256" character varying(64),
    "lease_expires_at" timestamp with time zone,
    "next_retry_at" timestamp with time zone,
    "error_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "started_at" timestamp with time zone,
    "uploaded_at" timestamp with time zone,
    "verified_at" timestamp with time zone,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_artifacts" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "artifact_type" character varying(64) NOT NULL,
    "name" "text" NOT NULL,
    "object_key" character varying(1024) NOT NULL,
    "sha256" character varying(64) NOT NULL,
    "size_bytes" bigint NOT NULL,
    "media_type" character varying(255) NOT NULL,
    "status" character varying(32) NOT NULL,
    "metadata_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_attempt_facts" (
    "attempt_id" bigint NOT NULL,
    "fact_id" "text" NOT NULL,
    "producer_source" "text" DEFAULT 'WORKER'::"text" NOT NULL,
    "producer_sequence" integer NOT NULL,
    "event_type" character varying(128) NOT NULL,
    "payload_sha256" character varying(64) NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_attempt_facts_producer_source_check" CHECK (("producer_source" = ANY (ARRAY['WORKER'::"text", 'SUPERVISOR_JOURNAL'::"text"])))
);



CREATE TABLE "public"."workforce_attempt_runtimes" (
    "attempt_id" bigint NOT NULL,
    "execution_node_id" bigint NOT NULL,
    "assignment_id" bigint NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" "text" NOT NULL,
    "host_boot_id" "text" NOT NULL,
    "generation" integer NOT NULL,
    "owner_epoch" integer DEFAULT 1 NOT NULL,
    "execution_identity_digest" character varying(64) NOT NULL,
    "expected_container_name" "text" NOT NULL,
    "container_id" "text",
    "provider_proxy_container_id" "text",
    "provider_proxy_container_name" "text",
    "provider_proxy_image_digest" "text" DEFAULT ''::"text" NOT NULL,
    "provider_proxy_execution_identity_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "provider_proxy_owner_epoch" integer,
    "provider_proxy_control_state" character varying(32) DEFAULT ''::character varying NOT NULL,
    "provider_proxy_ready_at" timestamp with time zone,
    "harness_image_digest" "text" NOT NULL,
    "hook_configuration_digest" "text" DEFAULT ''::"text" NOT NULL,
    "status" character varying(32) NOT NULL,
    "launch_permit_id" bigint,
    "launch_permit_sha256" character varying(64),
    "launch_permit_expires_at" timestamp with time zone,
    "container_created_at" timestamp with time zone,
    "supervisor_ready_at" timestamp with time zone,
    "harness_started_at" timestamp with time zone,
    "harness_pid" integer,
    "run_permit_id" bigint,
    "run_permit_sha256" character varying(64),
    "run_permit_expires_at" timestamp with time zone,
    "run_permit_issued_at" timestamp with time zone,
    "run_nonce_sha256" character varying(64),
    "run_nonce_accepted_at" timestamp with time zone,
    "execution_lease_sha256" character varying(64),
    "execution_lease_expires_at" timestamp with time zone,
    "execution_lease_renewed_at" timestamp with time zone,
    "startup_deadline" timestamp with time zone,
    "supervisor_protocol_version" integer DEFAULT 2 NOT NULL,
    "recovery_started_at" timestamp with time zone,
    "recovery_previous_status" character varying(32),
    "reattach_deadline" timestamp with time zone,
    "reattach_permit_id" bigint,
    "reattach_permit_sha256" character varying(64),
    "reattach_permit_expires_at" timestamp with time zone,
    "pending_worker_instance_id" "text",
    "pending_owner_epoch" integer,
    "reattached_at" timestamp with time zone,
    "authority_receipt_sha256" character varying(64),
    "watchdog_deadline" timestamp with time zone,
    "terminating_at" timestamp with time zone,
    "lost_at" timestamp with time zone,
    "fenced_at" timestamp with time zone,
    "terminal_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "adapter_session_generation" integer,
    CONSTRAINT "workforce_attempt_runtimes_adapter_session_generation_check" CHECK (("adapter_session_generation" > 0))
);



CREATE TABLE "public"."workforce_check_runs" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "ordinal" integer NOT NULL,
    "name" "text" NOT NULL,
    "command_digest" character varying(64) NOT NULL,
    "status" character varying(32) NOT NULL,
    "exit_code" integer,
    "timeout_seconds" integer NOT NULL,
    "output" "text",
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_completion_check_cycle_authorizations" (
    "cycle_id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "cycle_number" integer NOT NULL,
    CONSTRAINT "workforce_completion_check_cycle_authorizati_cycle_number_check" CHECK (("cycle_number" > 0)),
    CONSTRAINT "workforce_completion_check_cycle_authorizati_task_turn_id_check" CHECK (("task_turn_id" > 0)),
    CONSTRAINT "workforce_completion_check_cycle_authorization_attempt_id_check" CHECK (("attempt_id" > 0)),
    CONSTRAINT "workforce_completion_check_cycle_authorizations_cycle_id_check" CHECK (("cycle_id" > 0)),
    CONSTRAINT "workforce_completion_check_cycle_authorizations_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_completion_check_cycle_authorizations_tenant_id_check" CHECK (("tenant_id" > 0))
);



CREATE TABLE "public"."workforce_completion_check_decisions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "source_task_turn_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "checkpoint_id" bigint NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "action" character varying(32) NOT NULL,
    "result_id" bigint NOT NULL,
    "baseline_result_id" bigint NOT NULL,
    "reason" character varying(2000) NOT NULL,
    "repair_scope" character varying(20000) DEFAULT ''::character varying NOT NULL,
    "idempotency_key" character varying(128) NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_completion_check_decisions_action_check" CHECK ((("action")::"text" = ANY ((ARRAY['CONTINUE_WITH_LIMITATIONS'::character varying, 'AUTHORIZE_REPAIR'::character varying, 'ADD_VERIFICATION'::character varying])::"text"[]))),
    CONSTRAINT "workforce_completion_check_decisions_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_completion_check_decisions_baseline_result_id_check" CHECK (("baseline_result_id" > 0)),
    CONSTRAINT "workforce_completion_check_decisions_checkpoint_id_check" CHECK (("checkpoint_id" > 0)),
    CONSTRAINT "workforce_completion_check_decisions_result_id_check" CHECK (("result_id" > 0)),
    CONSTRAINT "workforce_completion_check_decisions_source_task_turn_id_check" CHECK (("source_task_turn_id" > 0)),
    CONSTRAINT "workforce_completion_check_decisions_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_completion_check_decisions_task_turn_id_check" CHECK (("task_turn_id" > 0)),
    CONSTRAINT "workforce_completion_check_decisions_tenant_id_check" CHECK (("tenant_id" > 0))
);



CREATE TABLE "public"."workforce_completion_check_results" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "evaluation_cycle_id" bigint,
    "phase" character varying(16) NOT NULL,
    "check_digest" character varying(64) NOT NULL,
    "status" character varying(32) NOT NULL,
    "summary" character varying(4000) DEFAULT ''::character varying NOT NULL,
    "observation_json" "jsonb" NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "source_commit" character varying(64) DEFAULT ''::character varying NOT NULL,
    "source_identity_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    CONSTRAINT "workforce_completion_check_results_attempt_id_check" CHECK (("attempt_id" > 0)),
    CONSTRAINT "workforce_completion_check_results_check" CHECK ((((("phase")::"text" = 'BASELINE'::"text") AND ("evaluation_cycle_id" IS NULL)) OR ((("phase")::"text" = 'CURRENT'::"text") AND ("evaluation_cycle_id" > 0)))),
    CONSTRAINT "workforce_completion_check_results_check_digest_check" CHECK ((("check_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_results_phase_check" CHECK ((("phase")::"text" = ANY ((ARRAY['BASELINE'::character varying, 'CURRENT'::character varying])::"text"[]))),
    CONSTRAINT "workforce_completion_check_results_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['PASSED'::character varying, 'FAILED'::character varying, 'UNAVAILABLE'::character varying, 'NO_TESTS'::character varying, 'NOT_CONFIGURED'::character varying, 'TIMED_OUT'::character varying])::"text"[]))),
    CONSTRAINT "workforce_completion_check_results_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_completion_check_results_task_turn_id_check" CHECK (("task_turn_id" > 0)),
    CONSTRAINT "workforce_completion_check_results_tenant_id_check" CHECK (("tenant_id" > 0))
);



CREATE TABLE "public"."workforce_completion_check_revisions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "source_task_turn_id" bigint NOT NULL,
    "source_attempt_id" bigint NOT NULL,
    "source_version" integer NOT NULL,
    "revision_number" integer NOT NULL,
    "version" integer NOT NULL,
    "status" character varying(16) NOT NULL,
    "creator_user_id" bigint NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "idempotency_key" character varying(128) NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "base_input_json" "jsonb" NOT NULL,
    "base_digest" character varying(64) NOT NULL,
    "input_json" "jsonb" NOT NULL,
    "input_digest" character varying(64) NOT NULL,
    "check_digest" character varying(64) NOT NULL,
    "references_json" "jsonb" NOT NULL,
    "source_baseline_result_id" bigint,
    "source_commit" character varying(64) NOT NULL,
    "source_identity_digest" character varying(64) NOT NULL,
    "execution_snapshot_digest" character varying(64) NOT NULL,
    "processor_release_id" bigint NOT NULL,
    "processor_digest" character varying(64) NOT NULL,
    "applied_turn_id" bigint,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "applied_at" timestamp with time zone,
    CONSTRAINT "workforce_completion_check_revi_execution_snapshot_digest_check" CHECK ((("execution_snapshot_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_revi_source_baseline_result_id_check" CHECK (("source_baseline_result_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisio_source_identity_digest_check" CHECK ((("source_identity_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_revisions_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_applied_turn_id_check" CHECK (("applied_turn_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_base_digest_check" CHECK ((("base_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_revisions_base_input_json_check" CHECK ((("jsonb_typeof"("base_input_json") = 'object'::"text") AND ("octet_length"(("base_input_json")::"text") <= 131072))),
    CONSTRAINT "workforce_completion_check_revisions_check" CHECK (((("status")::"text" = 'APPLIED'::"text") = (("applied_turn_id" IS NOT NULL) AND ("applied_at" IS NOT NULL)))),
    CONSTRAINT "workforce_completion_check_revisions_check_digest_check" CHECK ((("check_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_revisions_creator_user_id_check" CHECK (("creator_user_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_idempotency_key_check" CHECK ((("octet_length"(("idempotency_key")::"text") >= 1) AND ("octet_length"(("idempotency_key")::"text") <= 128))),
    CONSTRAINT "workforce_completion_check_revisions_input_digest_check" CHECK ((("input_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_revisions_input_json_check" CHECK ((("jsonb_typeof"("input_json") = 'object'::"text") AND ("octet_length"(("input_json")::"text") <= 131072))),
    CONSTRAINT "workforce_completion_check_revisions_processor_digest_check" CHECK ((("processor_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_revisions_processor_release_id_check" CHECK (("processor_release_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_references_json_check" CHECK ((("jsonb_typeof"("references_json") = 'array'::"text") AND ("octet_length"(("references_json")::"text") <= 65536))),
    CONSTRAINT "workforce_completion_check_revisions_request_digest_check" CHECK ((("request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_completion_check_revisions_revision_number_check" CHECK (("revision_number" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_source_attempt_id_check" CHECK (("source_attempt_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_source_task_turn_id_check" CHECK (("source_task_turn_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_source_version_check" CHECK (("source_version" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'APPLIED'::character varying, 'CANCELLED'::character varying, 'SUPERSEDED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_completion_check_revisions_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_completion_check_revisions_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_completion_proposals" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "harness_session_id" bigint NOT NULL,
    "schema_version" integer NOT NULL,
    "raw_json" "jsonb" NOT NULL,
    "validation_status" character varying(32) NOT NULL,
    "submitted_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_continuation_checkpoints" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "evaluation_cycle_id" bigint,
    "decision" character varying(32) NOT NULL,
    "checkpoint_identity_sha256" character varying(64) NOT NULL,
    "format_version" integer DEFAULT 2 NOT NULL,
    "repository_bundle_object_key" "text" DEFAULT ''::"text" NOT NULL,
    "repository_bundle_content_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "repository_bundle_object_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "repository_bundle_size_bytes" bigint DEFAULT 0 NOT NULL,
    "workspace_overlay_object_key" "text" DEFAULT ''::"text" NOT NULL,
    "workspace_overlay_content_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "workspace_overlay_object_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "workspace_overlay_size_bytes" bigint DEFAULT 0 NOT NULL,
    "selection_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "selection_policy_digest" "text" DEFAULT ''::"text" NOT NULL,
    "gitignore_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "session_object_key" "text" NOT NULL,
    "session_sha256" character varying(64) NOT NULL,
    "identity_manifest_object_key" "text" NOT NULL,
    "manifest_sha256" character varying(64) NOT NULL,
    "task_execution_snapshot_digest" "text" DEFAULT ''::"text" NOT NULL,
    "base_commit" "text" DEFAULT ''::"text" NOT NULL,
    "result_commit" "text" DEFAULT ''::"text" NOT NULL,
    "result_tree" "text" DEFAULT ''::"text" NOT NULL,
    "session_lineage_digest" "text" DEFAULT ''::"text" NOT NULL,
    "session_content_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "session_object_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "session_size_bytes" bigint DEFAULT 0 NOT NULL,
    "manifest_content_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "manifest_object_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "manifest_size_bytes" bigint DEFAULT 0 NOT NULL,
    "encryption_algorithm" "text" DEFAULT ''::"text" NOT NULL,
    "encryption_key_id" "text" DEFAULT ''::"text" NOT NULL,
    "wrapped_data_key" "text" DEFAULT ''::"text" NOT NULL,
    "approved_by_principal_id" bigint,
    "approved_at" timestamp with time zone,
    "version" integer DEFAULT 1 NOT NULL,
    "approval_reason" "text" DEFAULT ''::"text" NOT NULL,
    "consumed_by_turn_id" bigint,
    "consumed_at" timestamp with time zone,
    "status" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    "expires_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "available_at" timestamp with time zone,
    "expired_at" timestamp with time zone,
    "purpose" character varying(32) DEFAULT 'RESULT'::character varying NOT NULL,
    "interaction_id" bigint,
    "operation_id" bigint,
    "interaction_phase" character varying(16),
    "adapter_session_generation" integer,
    "automatic_revision_count" integer,
    "interaction_round" bigint,
    "suspend_control_key_sha256" character varying(64),
    "interaction_manifest_json" "jsonb",
    CONSTRAINT "workforce_checkpoint_interaction_manifest_ck" CHECK ((("interaction_manifest_json" IS NULL) OR (("jsonb_typeof"("interaction_manifest_json") = 'object'::"text") AND ("octet_length"(("interaction_manifest_json")::"text") <= 1048576) AND ("interaction_manifest_json" ?& ARRAY['schemaVersion'::"text", 'format'::"text", 'purpose'::"text", 'decision'::"text", 'checkpointIdentity'::"text", 'taskId'::"text", 'taskTurnId'::"text", 'attemptId'::"text", 'generation'::"text", 'interactionId'::"text", 'operationId'::"text", 'adapterSessionGeneration'::"text", 'automaticRevisionCount'::"text", 'sessionDigest'::"text", 'taskExecutionSnapshotDigest'::"text", 'baseCommit'::"text", 'resultCommit'::"text", 'resultTree'::"text", 'selectionDigest'::"text", 'repositoryBundle'::"text", 'workspaceOverlay'::"text", 'session'::"text", 'executionBudget'::"text", 'currentRound'::"text", 'suspendControlKeySha256'::"text"]) AND (("interaction_manifest_json" - ARRAY['schemaVersion'::"text", 'format'::"text", 'purpose'::"text", 'decision'::"text", 'checkpointIdentity'::"text", 'taskId'::"text", 'taskTurnId'::"text", 'attemptId'::"text", 'generation'::"text", 'interactionId'::"text", 'operationId'::"text", 'adapterSessionGeneration'::"text", 'automaticRevisionCount'::"text", 'sessionDigest'::"text", 'taskExecutionSnapshotDigest'::"text", 'baseCommit'::"text", 'resultCommit'::"text", 'resultTree'::"text", 'selectionDigest'::"text", 'repositoryBundle'::"text", 'workspaceOverlay'::"text", 'session'::"text", 'executionBudget'::"text", 'currentRound'::"text", 'suspendControlKeySha256'::"text"]) = '{}'::"jsonb") AND (("interaction_manifest_json" ->> 'currentRound'::"text") = ("interaction_round")::"text") AND (("interaction_manifest_json" ->> 'suspendControlKeySha256'::"text") = ("suspend_control_key_sha256")::"text") AND ("jsonb_typeof"(("interaction_manifest_json" -> 'executionBudget'::"text")) = 'object'::"text") AND (("interaction_manifest_json" -> 'executionBudget'::"text") ?& ARRAY['version'::"text", 'taskId'::"text", 'attemptId'::"text", 'generation'::"text", 'ownerEpoch'::"text", 'activeLimitMillis'::"text", 'activeElapsedMillis'::"text", 'interactionWaitingMillis'::"text", 'absoluteDeadlineUnixMillis'::"text"]) AND ((("interaction_manifest_json" -> 'executionBudget'::"text") - ARRAY['version'::"text", 'taskId'::"text", 'attemptId'::"text", 'generation'::"text", 'ownerEpoch'::"text", 'activeLimitMillis'::"text", 'activeElapsedMillis'::"text", 'interactionWaitingMillis'::"text", 'absoluteDeadlineUnixMillis'::"text"]) = '{}'::"jsonb") AND ((("interaction_manifest_json" -> 'executionBudget'::"text") ->> 'version'::"text") = '1'::"text") AND ((("interaction_manifest_json" -> 'executionBudget'::"text") ->> 'taskId'::"text") = ("task_id")::"text") AND ((("interaction_manifest_json" -> 'executionBudget'::"text") ->> 'attemptId'::"text") = ("attempt_id")::"text") AND ("jsonb_typeof"(("interaction_manifest_json" -> 'repositoryBundle'::"text")) = 'object'::"text") AND ("jsonb_typeof"(("interaction_manifest_json" -> 'workspaceOverlay'::"text")) = 'object'::"text") AND ("jsonb_typeof"(("interaction_manifest_json" -> 'session'::"text")) = 'object'::"text") AND (("interaction_manifest_json" -> 'repositoryBundle'::"text") ?& ARRAY['sha256'::"text", 'sizeBytes'::"text"]) AND (("interaction_manifest_json" -> 'workspaceOverlay'::"text") ?& ARRAY['sha256'::"text", 'sizeBytes'::"text"]) AND (("interaction_manifest_json" -> 'session'::"text") ?& ARRAY['sha256'::"text", 'sizeBytes'::"text"]) AND ("jsonb_typeof"((("interaction_manifest_json" -> 'repositoryBundle'::"text") -> 'sha256'::"text")) = 'string'::"text") AND ("jsonb_typeof"((("interaction_manifest_json" -> 'workspaceOverlay'::"text") -> 'sha256'::"text")) = 'string'::"text") AND ("jsonb_typeof"((("interaction_manifest_json" -> 'session'::"text") -> 'sha256'::"text")) = 'string'::"text") AND ("jsonb_typeof"((("interaction_manifest_json" -> 'repositoryBundle'::"text") -> 'sizeBytes'::"text")) = 'number'::"text") AND ("jsonb_typeof"((("interaction_manifest_json" -> 'workspaceOverlay'::"text") -> 'sizeBytes'::"text")) = 'number'::"text") AND ("jsonb_typeof"((("interaction_manifest_json" -> 'session'::"text") -> 'sizeBytes'::"text")) = 'number'::"text") AND (("interaction_manifest_json" ->> 'purpose'::"text") = 'INTERACTION_SUSPEND'::"text") AND (("interaction_manifest_json" ->> 'decision'::"text") = ''::"text") AND (("interaction_manifest_json" ->> 'checkpointIdentity'::"text") = ("checkpoint_identity_sha256")::"text") AND (("interaction_manifest_json" ->> 'taskId'::"text") = ("task_id")::"text") AND (("interaction_manifest_json" ->> 'taskTurnId'::"text") = ("task_turn_id")::"text") AND (("interaction_manifest_json" ->> 'attemptId'::"text") = ("attempt_id")::"text") AND (("interaction_manifest_json" ->> 'interactionId'::"text") = (COALESCE("interaction_id", (0)::bigint))::"text") AND (("interaction_manifest_json" ->> 'operationId'::"text") = (COALESCE("operation_id", (0)::bigint))::"text") AND (("interaction_manifest_json" ->> 'adapterSessionGeneration'::"text") = ("adapter_session_generation")::"text") AND (("interaction_manifest_json" ->> 'automaticRevisionCount'::"text") = ("automatic_revision_count")::"text") AND ((("interaction_manifest_json" -> 'repositoryBundle'::"text") - ARRAY['sha256'::"text", 'sizeBytes'::"text"]) = '{}'::"jsonb") AND ((("interaction_manifest_json" -> 'workspaceOverlay'::"text") - ARRAY['sha256'::"text", 'sizeBytes'::"text"]) = '{}'::"jsonb") AND ((("interaction_manifest_json" -> 'session'::"text") - ARRAY['sha256'::"text", 'sizeBytes'::"text"]) = '{}'::"jsonb")))),
    CONSTRAINT "workforce_checkpoint_purpose_ck" CHECK ((((("purpose")::"text" = 'RESULT'::"text") AND ("interaction_id" IS NULL) AND ("operation_id" IS NULL) AND ("interaction_phase" IS NULL) AND ("evaluation_cycle_id" IS NOT NULL) AND ("adapter_session_generation" IS NULL) AND ("automatic_revision_count" IS NULL) AND ("interaction_manifest_json" IS NULL) AND ("interaction_round" IS NULL) AND ("suspend_control_key_sha256" IS NULL)) OR ((("purpose")::"text" = 'INTERACTION_SUSPEND'::"text") AND ((("interaction_id" > 0) AND ("operation_id" IS NULL)) OR (("operation_id" > 0) AND ("interaction_id" IS NULL))) AND ("evaluation_cycle_id" IS NULL) AND (("decision")::"text" = ''::"text") AND ("format_version" = 2) AND ("interaction_phase" IS NOT NULL) AND (("interaction_phase")::"text" = ANY ((ARRAY['CAPTURING'::character varying, 'SUSPENDED'::character varying, 'RESUMED'::character varying])::"text"[])) AND ("adapter_session_generation" > 0) AND ("adapter_session_generation" IS NOT NULL) AND ("automatic_revision_count" >= 0) AND ("automatic_revision_count" IS NOT NULL) AND ("interaction_round" > 0) AND ("interaction_round" IS NOT NULL) AND ("suspend_control_key_sha256" IS NOT NULL) AND (("suspend_control_key_sha256")::"text" ~ '^[a-f0-9]{64}$'::"text") AND ("approved_by_principal_id" IS NULL) AND ("approved_at" IS NULL) AND (((("interaction_phase")::"text" = 'CAPTURING'::"text") AND (("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'FAILED'::character varying, 'INVALIDATED'::character varying, 'EXPIRED'::character varying])::"text"[])) AND ("consumed_by_turn_id" IS NULL) AND ("consumed_at" IS NULL) AND ("interaction_manifest_json" IS NULL)) OR ((("interaction_phase")::"text" = 'SUSPENDED'::"text") AND (("status")::"text" = ANY ((ARRAY['AVAILABLE'::character varying, 'FAILED'::character varying, 'INVALIDATED'::character varying, 'EXPIRED'::character varying])::"text"[])) AND ("available_at" IS NOT NULL) AND ("consumed_by_turn_id" IS NULL) AND ("consumed_at" IS NULL) AND ("interaction_manifest_json" IS NOT NULL)) OR ((("interaction_phase")::"text" = 'RESUMED'::"text") AND (("status")::"text" = ANY ((ARRAY['AVAILABLE'::character varying, 'FAILED'::character varying, 'INVALIDATED'::character varying, 'EXPIRED'::character varying])::"text"[])) AND ("consumed_by_turn_id" IS NOT NULL) AND ("consumed_at" IS NOT NULL) AND ("interaction_manifest_json" IS NOT NULL))))))
);



CREATE TABLE "public"."workforce_continuation_restore_runs" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "source_turn_id" bigint NOT NULL,
    "target_turn_id" bigint NOT NULL,
    "execution_node_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "checkpoint_id" bigint NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" "text" NOT NULL,
    "host_boot_id" "text" NOT NULL,
    "generation" integer NOT NULL,
    "owner_epoch" integer NOT NULL,
    "expected_identity_digest" character varying(64) NOT NULL,
    "restored_identity_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "status" character varying(32) NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "started_at" timestamp with time zone,
    "restored_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "interaction_execution_budget_json" "jsonb",
    CONSTRAINT "workforce_restore_interaction_budget_ck" CHECK ((("interaction_execution_budget_json" IS NULL) OR (("jsonb_typeof"("interaction_execution_budget_json") = 'object'::"text") AND ("octet_length"(("interaction_execution_budget_json")::"text") <= 4096) AND ("interaction_execution_budget_json" ?& ARRAY['version'::"text", 'taskId'::"text", 'attemptId'::"text", 'generation'::"text", 'ownerEpoch'::"text", 'activeLimitMillis'::"text", 'activeElapsedMillis'::"text", 'interactionWaitingMillis'::"text", 'absoluteDeadlineUnixMillis'::"text"]) AND (("interaction_execution_budget_json" - ARRAY['version'::"text", 'taskId'::"text", 'attemptId'::"text", 'generation'::"text", 'ownerEpoch'::"text", 'activeLimitMillis'::"text", 'activeElapsedMillis'::"text", 'interactionWaitingMillis'::"text", 'absoluteDeadlineUnixMillis'::"text"]) = '{}'::"jsonb") AND (("interaction_execution_budget_json" ->> 'version'::"text") = '1'::"text") AND (("interaction_execution_budget_json" ->> 'taskId'::"text") = ("task_id")::"text") AND (("interaction_execution_budget_json" ->> 'attemptId'::"text") = ("attempt_id")::"text"))))
);



CREATE TABLE "public"."workforce_delivery_attempts" (
    "id" bigint NOT NULL,
    "request_id" bigint NOT NULL,
    "attempt_number" integer NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" "text" NOT NULL,
    "host_boot_id" "text" NOT NULL,
    "generation" integer DEFAULT 1 NOT NULL,
    "lease_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "lease_expires_at" timestamp with time zone,
    "push_authorization_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "push_authorization_expires_at" timestamp with time zone,
    "phase" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    "status" character varying(32) NOT NULL,
    "remote_before" "text" DEFAULT ''::"text" NOT NULL,
    "remote_after" "text" DEFAULT ''::"text" NOT NULL,
    "push_started_at" timestamp with time zone,
    "verified_at" timestamp with time zone,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "reconciliation_only" boolean DEFAULT false NOT NULL,
    "next_retry_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "finished_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_delivery_decisions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "delivery_request_id" bigint NOT NULL,
    "action" character varying(32) NOT NULL,
    "expected_version" integer NOT NULL,
    "principal_id" bigint NOT NULL,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "authorization_digest" character varying(64) NOT NULL,
    "reason" character varying(2048) DEFAULT ''::character varying NOT NULL,
    "applied_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_delivery_decisions_action_ck" CHECK ((("action")::"text" = ANY ((ARRAY['AUTO_FROM_TASK_PLAN'::character varying, 'DELIVER'::character varying, 'SKIP_THIS_RESULT'::character varying, 'RESTART_DELIVERY'::character varying])::"text"[]))),
    CONSTRAINT "workforce_delivery_decisions_digest_ck" CHECK (((("request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND (("authorization_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_delivery_decisions_idempotency_ck" CHECK ((("octet_length"("idempotency_key") >= 1) AND ("octet_length"("idempotency_key") <= 128))),
    CONSTRAINT "workforce_delivery_decisions_identity_ck" CHECK ((("id" > 0) AND ("tenant_id" > 0) AND ("task_id" > 0) AND ("delivery_request_id" > 0) AND ("expected_version" > 0) AND ("principal_id" > 0))),
    CONSTRAINT "workforce_delivery_decisions_reason_ck" CHECK (("octet_length"(("reason")::"text") <= 2048))
);



CREATE TABLE "public"."workforce_delivery_intents" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "adapter_session_generation" integer NOT NULL,
    "local_tool_call_key" "text" NOT NULL,
    "native_tool_call_id" "text" NOT NULL,
    "round_key" "text" NOT NULL,
    "summary" character varying(2048) NOT NULL,
    "summary_digest" character varying(64) NOT NULL,
    "git_plan_digest" character varying(64) NOT NULL,
    "delivery_mode" character varying(32) NOT NULL,
    "delivery_authorization" character varying(32) NOT NULL,
    "delivery_request_id" bigint,
    "status" character varying(32) NOT NULL,
    "version" integer DEFAULT 1 NOT NULL,
    "adapter_received_at" timestamp with time zone,
    "closed_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_delivery_intents_authorization_ck" CHECK ((("delivery_authorization")::"text" = ANY ((ARRAY['AUTO_AFTER_GATES'::character varying, 'ASK_AFTER_GATES'::character varying])::"text"[]))),
    CONSTRAINT "workforce_delivery_intents_digest_ck" CHECK (((("summary_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND (("git_plan_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_delivery_intents_identity_ck" CHECK ((("id" > 0) AND ("tenant_id" > 0) AND ("task_id" > 0) AND ("task_turn_id" > 0) AND ("attempt_id" > 0) AND ("adapter_session_generation" > 0) AND (("delivery_request_id" IS NULL) OR ("delivery_request_id" > 0)) AND ("version" > 0))),
    CONSTRAINT "workforce_delivery_intents_mode_ck" CHECK ((("delivery_mode")::"text" = ANY ((ARRAY['NEW_BRANCH'::character varying, 'NEW_BRANCH_AND_DRAFT_REVIEW'::character varying])::"text"[]))),
    CONSTRAINT "workforce_delivery_intents_status_ck" CHECK ((("status")::"text" = ANY ((ARRAY['SCHEDULED'::character varying, 'WAITING_FINALIZATION'::character varying, 'WAITING_CHECKS'::character varying, 'WAITING_REVIEW'::character varying, 'WAITING_USER_DECISION'::character varying, 'DELIVERY_RUNNING'::character varying, 'DELIVERED'::character varying, 'DELIVERY_FAILED'::character varying, 'OUTCOME_UNKNOWN'::character varying, 'SKIPPED'::character varying, 'CANCELLED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_delivery_intents_summary_ck" CHECK (((("summary")::"text" = "btrim"(("summary")::"text")) AND (("octet_length"(("summary")::"text") >= 1) AND ("octet_length"(("summary")::"text") <= 2048)))),
    CONSTRAINT "workforce_delivery_intents_tool_identity_ck" CHECK (((("octet_length"("local_tool_call_key") >= 1) AND ("octet_length"("local_tool_call_key") <= 128)) AND (("octet_length"("native_tool_call_id") >= 1) AND ("octet_length"("native_tool_call_id") <= 512)) AND (("octet_length"("round_key") >= 1) AND ("octet_length"("round_key") <= 128))))
);



CREATE TABLE "public"."workforce_delivery_operations" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "delivery_request_id" bigint NOT NULL,
    "operation_kind" character varying(32) NOT NULL,
    "status" character varying(32) NOT NULL,
    "attempt_count" integer DEFAULT 0 NOT NULL,
    "max_attempts" integer DEFAULT 3 NOT NULL,
    "authorization_version" "text" DEFAULT ''::"text" NOT NULL,
    "authorization_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "reauthorized_by_principal_id" bigint,
    "reauthorized_at" timestamp with time zone,
    "reauthorization_reason" "text" DEFAULT ''::"text" NOT NULL,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "next_retry_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "finished_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_delivery_requests" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "checkpoint_id" bigint NOT NULL,
    "action_type" character varying(64) NOT NULL,
    "action_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "action_digest" "text" NOT NULL,
    "processor_release_id" bigint NOT NULL,
    "processor_digest" character varying(64) NOT NULL,
    "result_commit" "text" NOT NULL,
    "result_tree" "text" NOT NULL,
    "selection_digest" character varying(64) NOT NULL,
    "project_id" bigint NOT NULL,
    "target_config_digest" character varying(64) NOT NULL,
    "target_identity_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "policy_digest" "text" NOT NULL,
    "credential_version" "text" DEFAULT ''::"text" NOT NULL,
    "authorization_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "reauthorized_by_principal_id" bigint,
    "reauthorized_at" timestamp with time zone,
    "reauthorized_project_version_id" bigint,
    "reauthorization_reason" "text" DEFAULT ''::"text" NOT NULL,
    "base_branch" "text" NOT NULL,
    "destination_branch" "text" NOT NULL,
    "expected_base_commit" "text" NOT NULL,
    "delivery_commit" "text" NOT NULL,
    "commit_timestamp" timestamp with time zone NOT NULL,
    "approval_mode" character varying(32) NOT NULL,
    "approved_by_principal_id" bigint,
    "approved_at" timestamp with time zone,
    "status" character varying(32) NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "version" integer DEFAULT 1 NOT NULL,
    "delivery_completed_notified_at" timestamp with time zone,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "finished_at" timestamp with time zone,
    CONSTRAINT "workforce_delivery_requests_version_ck" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_execution_nodes" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "node_key" "text" NOT NULL,
    "status" character varying(32) NOT NULL,
    "generation" integer DEFAULT 1 NOT NULL,
    "execution_requirement_digest" "text" NOT NULL,
    "wait_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "max_attempts" integer DEFAULT 3 NOT NULL,
    "retry_count" integer DEFAULT 0 NOT NULL,
    "next_retry_at" timestamp with time zone,
    "last_retry_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_execution_operation_attempts" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "operation_id" bigint NOT NULL,
    "attempt_number" integer NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" character varying(128) NOT NULL,
    "host_boot_id" character varying(128) NOT NULL,
    "owner_epoch" integer NOT NULL,
    "lease_digest" character varying(64) NOT NULL,
    "lease_expires_at" timestamp with time zone NOT NULL,
    "status" character varying(32) NOT NULL,
    "container_id" "text" DEFAULT ''::"text" NOT NULL,
    "relay_identity" "text" DEFAULT ''::"text" NOT NULL,
    "result_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "exit_code" integer,
    "created_at" timestamp with time zone NOT NULL,
    "updated_at" timestamp with time zone NOT NULL,
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    CONSTRAINT "workforce_execution_operation_attempts_attempt_number_check" CHECK (("attempt_number" > 0)),
    CONSTRAINT "workforce_execution_operation_attempts_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_execution_operation_attempts_lease_digest_check" CHECK ((("lease_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_execution_operation_attempts_operation_id_check" CHECK (("operation_id" > 0)),
    CONSTRAINT "workforce_execution_operation_attempts_owner_epoch_check" CHECK (("owner_epoch" > 0)),
    CONSTRAINT "workforce_execution_operation_attempts_result_digest_check" CHECK (((("result_digest")::"text" = ''::"text") OR (("result_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_execution_operation_attempts_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['DISPATCHED'::character varying, 'RUNNING'::character varying, 'SUCCEEDED'::character varying, 'FAILED'::character varying, 'OUTCOME_UNKNOWN'::character varying])::"text"[]))),
    CONSTRAINT "workforce_execution_operation_attempts_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_execution_operation_attempts_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_execution_operation_attempts_worker_id_check" CHECK (("worker_id" > 0))
);



CREATE TABLE "public"."workforce_execution_operation_decisions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "operation_id" bigint NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "action" character varying(32) NOT NULL,
    "idempotency_key" character varying(128) NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "reason" character varying(2000) DEFAULT ''::character varying NOT NULL,
    "status" character varying(16) NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    CONSTRAINT "workforce_execution_operation_decision_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_execution_operation_decisions_action_check" CHECK ((("action")::"text" = ANY ((ARRAY['ALLOW_ONCE'::character varying, 'ALLOW_FOR_TASK'::character varying, 'DENY'::character varying])::"text"[]))),
    CONSTRAINT "workforce_execution_operation_decisions_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_execution_operation_decisions_idempotency_key_check" CHECK ((("octet_length"(("idempotency_key")::"text") >= 1) AND ("octet_length"(("idempotency_key")::"text") <= 128))),
    CONSTRAINT "workforce_execution_operation_decisions_operation_id_check" CHECK (("operation_id" > 0)),
    CONSTRAINT "workforce_execution_operation_decisions_reason_check" CHECK (("octet_length"(("reason")::"text") <= 2000)),
    CONSTRAINT "workforce_execution_operation_decisions_request_digest_check" CHECK ((("request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_execution_operation_decisions_status_check" CHECK ((("status")::"text" = 'APPLIED'::"text")),
    CONSTRAINT "workforce_execution_operation_decisions_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_execution_operation_decisions_tenant_id_check" CHECK (("tenant_id" > 0))
);



CREATE TABLE "public"."workforce_execution_operations" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "adapter_session_generation" integer NOT NULL,
    "authorization_grant_id" bigint,
    "local_tool_call_key" character varying(128) NOT NULL,
    "native_tool_call_id" "text" NOT NULL,
    "round_key" character varying(128) NOT NULL,
    "operation_kind" character varying(32) NOT NULL,
    "descriptor_ciphertext" "bytea" NOT NULL,
    "descriptor_edek" "bytea" NOT NULL,
    "descriptor_key_ref" character varying(256) NOT NULL,
    "descriptor_digest" character varying(64) NOT NULL,
    "purpose" character varying(2048) NOT NULL,
    "targets_json" "jsonb" NOT NULL,
    "status" character varying(32) NOT NULL,
    "version" integer NOT NULL,
    "result_delivery_status" character varying(32) DEFAULT ''::character varying NOT NULL,
    "agent_continued" boolean DEFAULT false NOT NULL,
    "result_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "result_summary" "text" DEFAULT ''::"text" NOT NULL,
    "exit_code" integer,
    "result_output_ciphertext" "bytea",
    "result_output_edek" "bytea",
    "result_output_key_ref" character varying(256),
    "expires_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    "updated_at" timestamp with time zone NOT NULL,
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    CONSTRAINT "workforce_execution_operations_adapter_session_generation_check" CHECK (("adapter_session_generation" > 0)),
    CONSTRAINT "workforce_execution_operations_attempt_id_check" CHECK (("attempt_id" > 0)),
    CONSTRAINT "workforce_execution_operations_authorization_grant_id_check" CHECK (("authorization_grant_id" > 0)),
    CONSTRAINT "workforce_execution_operations_check" CHECK (("expires_at" > "created_at")),
    CONSTRAINT "workforce_execution_operations_check1" CHECK (((("result_output_ciphertext" IS NULL) AND ("result_output_edek" IS NULL) AND ("result_output_key_ref" IS NULL)) OR (("result_output_ciphertext" IS NOT NULL) AND ("result_output_edek" IS NOT NULL) AND ("result_output_key_ref" IS NOT NULL)))),
    CONSTRAINT "workforce_execution_operations_check2" CHECK (((("status")::"text" <> ALL ((ARRAY['SUCCEEDED'::character varying, 'FAILED'::character varying, 'OUTCOME_UNKNOWN'::character varying, 'DENIED'::character varying])::"text"[])) OR ("finished_at" IS NOT NULL))),
    CONSTRAINT "workforce_execution_operations_descriptor_digest_check" CHECK ((("descriptor_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_execution_operations_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_execution_operations_local_tool_call_key_check" CHECK ((("octet_length"(("local_tool_call_key")::"text") >= 1) AND ("octet_length"(("local_tool_call_key")::"text") <= 128))),
    CONSTRAINT "workforce_execution_operations_native_tool_call_id_ck" CHECK (((("octet_length"("native_tool_call_id") >= 1) AND ("octet_length"("native_tool_call_id") <= 512)) AND ("native_tool_call_id" ~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$'::"text"))),
    CONSTRAINT "workforce_execution_operations_operation_kind_check" CHECK ((("operation_kind")::"text" = 'NETWORK_COMMAND'::"text")),
    CONSTRAINT "workforce_execution_operations_purpose_check" CHECK ((("octet_length"(("purpose")::"text") >= 1) AND ("octet_length"(("purpose")::"text") <= 2048))),
    CONSTRAINT "workforce_execution_operations_result_delivery_status_check" CHECK ((("result_delivery_status")::"text" = ANY ((ARRAY[''::character varying, 'SAVED'::character varying, 'DELIVERED_TO_ADAPTER'::character varying, 'NATIVE_CALL_RESOLVED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_execution_operations_result_digest_check" CHECK (((("result_digest")::"text" = ''::"text") OR (("result_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_execution_operations_result_summary_check" CHECK (("octet_length"("result_summary") <= 2048)),
    CONSTRAINT "workforce_execution_operations_round_key_check" CHECK ((("octet_length"(("round_key")::"text") >= 1) AND ("octet_length"(("round_key")::"text") <= 128))),
    CONSTRAINT "workforce_execution_operations_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['WAITING_APPROVAL'::character varying, 'READY'::character varying, 'DISPATCHED'::character varying, 'RUNNING'::character varying, 'SUCCEEDED'::character varying, 'FAILED'::character varying, 'OUTCOME_UNKNOWN'::character varying, 'DENIED'::character varying, 'EXPIRED'::character varying, 'CANCELLED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_execution_operations_targets_json_check" CHECK ((("jsonb_typeof"("targets_json") = 'array'::"text") AND (("jsonb_array_length"("targets_json") >= 1) AND ("jsonb_array_length"("targets_json") <= 8)))),
    CONSTRAINT "workforce_execution_operations_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_execution_operations_task_turn_id_check" CHECK (("task_turn_id" > 0)),
    CONSTRAINT "workforce_execution_operations_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_execution_operations_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_finalization_runs" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "execution_node_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "generation" integer NOT NULL,
    "input_identity_digest" character varying(64) NOT NULL,
    "completion_proposal_id" bigint NOT NULL,
    "result_commit" "text" DEFAULT ''::"text" NOT NULL,
    "result_tree" "text" DEFAULT ''::"text" NOT NULL,
    "status" character varying(32) NOT NULL,
    "run_number" integer DEFAULT 1 NOT NULL,
    "max_runs" integer DEFAULT 3 NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" "text" NOT NULL,
    "host_boot_id" "text" NOT NULL,
    "lease_sha256" character varying(64),
    "lease_expires_at" timestamp with time zone,
    "manifest_digest" character varying(64),
    "manifest_json" "jsonb",
    "spool_identity" "text" DEFAULT ''::"text" NOT NULL,
    "next_retry_at" timestamp with time zone,
    "error_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "started_at" timestamp with time zone,
    "frozen_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_harness_channels" (
    "alias" "text" NOT NULL,
    "harness_release_id" bigint NOT NULL,
    "updated_by_principal_id" bigint,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_harness_releases" (
    "id" bigint NOT NULL,
    "alias" "text" NOT NULL,
    "image_ref" "text" NOT NULL,
    "image_digest" character varying(71) NOT NULL,
    "contract_version" "text" NOT NULL,
    "supervisor_version" "text" NOT NULL,
    "harness_type" "text" NOT NULL,
    "harness_version" "text" NOT NULL,
    "capabilities_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "scheduling_scope" character varying(16) DEFAULT 'PRODUCTION'::character varying NOT NULL,
    "scheduling_priority" integer DEFAULT 100 NOT NULL,
    "provider_protocols_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "environment_capabilities_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "hook_events_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "execution_kinds_json" "jsonb" DEFAULT '["AGENT_ATTEMPT", "REVIEW_RUN"]'::"jsonb" NOT NULL,
    "scheduling_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "manifest_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "admission_evidence_json" "jsonb" NOT NULL,
    "status" character varying(32) NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_harness_releases_environment_capabilities_ck" CHECK (("jsonb_typeof"("environment_capabilities_json") = 'array'::"text")),
    CONSTRAINT "workforce_harness_releases_execution_kinds_ck" CHECK ((("jsonb_typeof"("execution_kinds_json") = 'array'::"text") AND ("jsonb_array_length"("execution_kinds_json") > 0) AND ("execution_kinds_json" <@ '["AGENT_ATTEMPT", "REVIEW_RUN"]'::"jsonb"))),
    CONSTRAINT "workforce_harness_releases_hook_events_ck" CHECK (("jsonb_typeof"("hook_events_json") = 'array'::"text")),
    CONSTRAINT "workforce_harness_releases_manifest_ck" CHECK (("jsonb_typeof"("manifest_json") = 'object'::"text")),
    CONSTRAINT "workforce_harness_releases_provider_protocols_ck" CHECK (("jsonb_typeof"("provider_protocols_json") = 'array'::"text")),
    CONSTRAINT "workforce_harness_releases_scheduling_digest_ck" CHECK (((("scheduling_digest")::"text" = ''::"text") OR (("scheduling_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_harness_releases_scheduling_priority_ck" CHECK ((("scheduling_priority" >= '-100000'::integer) AND ("scheduling_priority" <= 100000))),
    CONSTRAINT "workforce_harness_releases_scheduling_scope_ck" CHECK ((("scheduling_scope")::"text" = ANY ((ARRAY['PRODUCTION'::character varying, 'TEST_ONLY'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_harness_sessions" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "provider" "text" NOT NULL,
    "provider_session_id" "text",
    "status" character varying(32) NOT NULL,
    "resume_count" integer DEFAULT 0 NOT NULL,
    "cli_version" "text" NOT NULL,
    "adapter_version" "text" NOT NULL,
    "started_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "stopped_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_hook_package_releases" (
    "id" bigint NOT NULL,
    "package_id" bigint NOT NULL,
    "revision_id" bigint NOT NULL,
    "scope_type" character varying(16) NOT NULL,
    "tenant_id" bigint,
    "version" "text" NOT NULL,
    "runtime_kind" character varying(32) NOT NULL,
    "manifest_json" "jsonb" NOT NULL,
    "files_json" "jsonb" NOT NULL,
    "archive_object_key" character varying(1024) NOT NULL,
    "archive_sha256" character varying(64) NOT NULL,
    "archive_size_bytes" bigint NOT NULL,
    "published_by_principal_id" bigint NOT NULL,
    "published_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_hook_package_releases_check" CHECK ((((("scope_type")::"text" = 'SYSTEM'::"text") AND ("tenant_id" IS NULL)) OR ((("scope_type")::"text" = 'TENANT'::"text") AND ("tenant_id" IS NOT NULL)))),
    CONSTRAINT "workforce_hook_package_releases_scope_type_check" CHECK ((("scope_type")::"text" = ANY ((ARRAY['SYSTEM'::character varying, 'TENANT'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_hook_package_revisions" (
    "id" bigint NOT NULL,
    "package_id" bigint NOT NULL,
    "scope_type" character varying(16) NOT NULL,
    "tenant_id" bigint,
    "revision" bigint DEFAULT 1 NOT NULL,
    "manifest_json" "jsonb" NOT NULL,
    "files_json" "jsonb" NOT NULL,
    "validation_json" "jsonb",
    "status" character varying(32) DEFAULT 'DRAFT'::character varying NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_hook_package_revisions_check" CHECK ((((("scope_type")::"text" = 'SYSTEM'::"text") AND ("tenant_id" IS NULL)) OR ((("scope_type")::"text" = 'TENANT'::"text") AND ("tenant_id" IS NOT NULL)))),
    CONSTRAINT "workforce_hook_package_revisions_scope_type_check" CHECK ((("scope_type")::"text" = ANY ((ARRAY['SYSTEM'::character varying, 'TENANT'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_hook_packages" (
    "id" bigint NOT NULL,
    "scope_type" character varying(16) NOT NULL,
    "tenant_id" bigint,
    "name" character varying(255) NOT NULL,
    "category" character varying(64) NOT NULL,
    "description" "text" DEFAULT ''::"text" NOT NULL,
    "trust_class" character varying(32) DEFAULT 'PLATFORM_TRUSTED'::character varying NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_hook_packages_check" CHECK ((((("scope_type")::"text" = 'SYSTEM'::"text") AND ("tenant_id" IS NULL)) OR ((("scope_type")::"text" = 'TENANT'::"text") AND ("tenant_id" IS NOT NULL)))),
    CONSTRAINT "workforce_hook_packages_scope_type_check" CHECK ((("scope_type")::"text" = ANY ((ARRAY['SYSTEM'::character varying, 'TENANT'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_hook_test_runs" (
    "id" bigint NOT NULL,
    "revision_id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "event_name" character varying(128) NOT NULL,
    "fixture_json" "jsonb" NOT NULL,
    "decision" character varying(32) NOT NULL,
    "reason_code" character varying(128) NOT NULL,
    "diagnostics" "text" DEFAULT ''::"text" NOT NULL,
    "duration_ms" integer DEFAULT 0 NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_human_review_decisions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "review_run_id" bigint NOT NULL,
    "action" character varying(32) NOT NULL,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "reason" "text" NOT NULL,
    "revision_guidance" character varying(4000),
    "proof_identifier_sha256" character varying(64) NOT NULL,
    "challenge_identifier_sha256" character varying(64) NOT NULL,
    "assurance_level" character varying(16) NOT NULL,
    "authentication_methods_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "ai_verdict" character varying(32) DEFAULT ''::character varying NOT NULL,
    "canonical_report_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "status" character varying(32) DEFAULT 'APPLIED'::character varying NOT NULL,
    "resulting_task_turn_id" bigint,
    "quality_outcome" character varying(32) DEFAULT ''::character varying NOT NULL,
    "decided_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_human_review_decisi_authentication_methods_json_check" CHECK (("jsonb_typeof"("authentication_methods_json") = 'array'::"text")),
    CONSTRAINT "workforce_human_review_decisi_challenge_identifier_sha256_check" CHECK ((("challenge_identifier_sha256")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_human_review_decisions_action_check" CHECK ((("action")::"text" = ANY ((ARRAY['ACCEPT_RISK'::character varying, 'AUTHORIZE_ONE_REVISION'::character varying, 'CANCEL_TASK'::character varying])::"text"[]))),
    CONSTRAINT "workforce_human_review_decisions_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_human_review_decisions_ai_verdict_check" CHECK ((("ai_verdict")::"text" = ANY ((ARRAY[''::character varying, 'REVISE'::character varying, 'REJECT'::character varying])::"text"[]))),
    CONSTRAINT "workforce_human_review_decisions_assurance_level_check" CHECK ((("assurance_level")::"text" = 'AAL2'::"text")),
    CONSTRAINT "workforce_human_review_decisions_canonical_report_digest_check" CHECK (((("canonical_report_digest")::"text" = ''::"text") OR (("canonical_report_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_human_review_decisions_guidance_ck" CHECK (((("revision_guidance" IS NULL) OR (("octet_length"(("revision_guidance")::"text") >= 1) AND ("octet_length"(("revision_guidance")::"text") <= 4000))) AND ((("action")::"text" = 'AUTHORIZE_ONE_REVISION'::"text") OR ("revision_guidance" IS NULL)) AND ((("action")::"text" <> 'AUTHORIZE_ONE_REVISION'::"text") OR (("canonical_report_digest")::"text" <> ''::"text") OR ("revision_guidance" IS NOT NULL)))),
    CONSTRAINT "workforce_human_review_decisions_idempotency_key_check" CHECK (("idempotency_key" <> ''::"text")),
    CONSTRAINT "workforce_human_review_decisions_proof_identifier_sha256_check" CHECK ((("proof_identifier_sha256")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_human_review_decisions_quality_outcome_check" CHECK ((("quality_outcome")::"text" = ANY ((ARRAY[''::character varying, 'HUMAN_ACCEPTED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_human_review_decisions_reason_check" CHECK ((("octet_length"("reason") >= 1) AND ("octet_length"("reason") <= 2000))),
    CONSTRAINT "workforce_human_review_decisions_request_digest_check" CHECK ((("request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_human_review_decisions_review_run_id_check" CHECK (("review_run_id" > 0)),
    CONSTRAINT "workforce_human_review_decisions_status_check" CHECK ((("status")::"text" = 'APPLIED'::"text")),
    CONSTRAINT "workforce_human_review_decisions_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_human_review_decisions_task_turn_id_check" CHECK (("task_turn_id" > 0)),
    CONSTRAINT "workforce_human_review_decisions_tenant_id_check" CHECK (("tenant_id" > 0))
);



CREATE TABLE "public"."workforce_image_admission_runs" (
    "id" bigint NOT NULL,
    "candidate_id" bigint NOT NULL,
    "worker_id" bigint NOT NULL,
    "image_digest" character varying(71) NOT NULL,
    "contract_version" "text",
    "supervisor_version" "text",
    "harness_type" "text",
    "harness_version" "text",
    "declared_manifest_json" "jsonb",
    "observed_probe_json" "jsonb",
    "checks_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "verdict" character varying(32) NOT NULL,
    "error_code" character varying(128),
    "started_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "finished_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_image_candidates" (
    "id" bigint NOT NULL,
    "requested_ref" "text" NOT NULL,
    "resolved_digest" character varying(71),
    "requested_harness_type" "text" NOT NULL,
    "status" character varying(32) NOT NULL,
    "rejection_code" character varying(128),
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_integration_api_credentials" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "integration_application_id" bigint NOT NULL,
    "credential_locator" character varying(256) NOT NULL,
    "credential_digest" character varying(64) NOT NULL,
    "credential_hint" character varying(64) NOT NULL,
    "version" integer NOT NULL,
    "status" character varying(32) NOT NULL,
    "valid_until" timestamp with time zone,
    "revoked_at" timestamp with time zone,
    "created_by" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "last_used_at" timestamp with time zone,
    CONSTRAINT "workforce_integration_api_credentials_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['CURRENT'::character varying, 'GRACE'::character varying, 'REVOKED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_integration_api_credentials_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_integration_applications" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "name" character varying(255) NOT NULL,
    "status" character varying(32) DEFAULT 'ENABLED'::character varying NOT NULL,
    "webhook_url" character varying(2048) DEFAULT ''::character varying NOT NULL,
    "created_by" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "last_used_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_integration_signing_credentials" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "integration_application_id" bigint NOT NULL,
    "signing_secret_ciphertext" "bytea" NOT NULL,
    "signing_secret_edek" "bytea" NOT NULL,
    "signing_secret_wrap_key_ref" character varying(256) NOT NULL,
    "signing_secret_hint" character varying(64) NOT NULL,
    "version" integer NOT NULL,
    "status" character varying(32) NOT NULL,
    "valid_until" timestamp with time zone,
    "revoked_at" timestamp with time zone,
    "created_by" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "last_used_at" timestamp with time zone,
    CONSTRAINT "workforce_integration_signing_credentials_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['CURRENT'::character varying, 'GRACE'::character varying, 'REVOKED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_integration_signing_credentials_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_legacy_identity_map" (
    "entity_type" character varying(64) NOT NULL,
    "legacy_identity" "text" NOT NULL,
    "entity_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_machine_idempotency_records" (
    "worker_id" bigint NOT NULL,
    "http_method" character varying(16) NOT NULL,
    "normalized_route" "text" NOT NULL,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "status" character varying(32) NOT NULL,
    "response_status" integer,
    "safe_response_json" "jsonb",
    "encrypted_response_blob" "bytea",
    "resource_type" character varying(64) DEFAULT ''::character varying NOT NULL,
    "resource_id" bigint,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "completed_at" timestamp with time zone,
    "expires_at" timestamp with time zone NOT NULL
);



CREATE TABLE "public"."workforce_model_credentials" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "name" character varying(100) NOT NULL,
    "protocol" character varying(16) NOT NULL,
    "base_url" character varying(2048) NOT NULL,
    "secret_ciphertext" "bytea",
    "secret_edek" "bytea",
    "secret_wrap_key_ref" character varying(256),
    "secret_hint" character varying(16) DEFAULT ''::character varying NOT NULL,
    "secret_version" integer DEFAULT 1 NOT NULL,
    "models_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "status" character varying(16) NOT NULL,
    "version" integer DEFAULT 1 NOT NULL,
    "last_discovery_status" character varying(32) DEFAULT ''::character varying NOT NULL,
    "last_discovery_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "last_discovered_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_model_credentials_check" CHECK ((((("status")::"text" = 'DELETED'::"text") AND ("secret_ciphertext" IS NULL) AND ("secret_edek" IS NULL) AND ("secret_wrap_key_ref" IS NULL) AND (("secret_hint")::"text" = ''::"text")) OR ((("status")::"text" = ANY ((ARRAY['ACTIVE'::character varying, 'DISABLED'::character varying])::"text"[])) AND ("secret_ciphertext" IS NOT NULL) AND ("secret_edek" IS NOT NULL) AND ("secret_wrap_key_ref" IS NOT NULL) AND (("octet_length"("secret_ciphertext") >= 1) AND ("octet_length"("secret_ciphertext") <= 16384)) AND (("octet_length"("secret_edek") >= 1) AND ("octet_length"("secret_edek") <= 4096)) AND ("btrim"(("secret_wrap_key_ref")::"text") <> ''::"text")))),
    CONSTRAINT "workforce_model_credentials_models_json_check" CHECK (("jsonb_typeof"("models_json") = 'array'::"text")),
    CONSTRAINT "workforce_model_credentials_models_json_check1" CHECK (("jsonb_array_length"("models_json") <= 5000)),
    CONSTRAINT "workforce_model_credentials_protocol_check" CHECK ((("protocol")::"text" = ANY ((ARRAY['OPENAI'::character varying, 'ANTHROPIC'::character varying])::"text"[]))),
    CONSTRAINT "workforce_model_credentials_secret_version_check" CHECK (("secret_version" > 0)),
    CONSTRAINT "workforce_model_credentials_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['ACTIVE'::character varying, 'DISABLED'::character varying, 'DELETED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_model_credentials_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_model_invocations" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "protocol_invocation_key" character varying(64) NOT NULL,
    "execution_kind" character varying(32) DEFAULT 'AGENT_ATTEMPT'::character varying NOT NULL,
    "attempt_id" bigint,
    "review_run_id" bigint,
    "review_run_attempt_id" bigint,
    "generation" integer NOT NULL,
    "owner_epoch" integer NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" character varying(128) NOT NULL,
    "host_boot_id" character varying(128) NOT NULL,
    "model_credential_id" bigint NOT NULL,
    "credential_version" integer NOT NULL,
    "protocol" character varying(16) NOT NULL,
    "primary_model" character varying(256) NOT NULL,
    "requested_model" character varying(256) NOT NULL,
    "response_model" character varying(256),
    "operation" character varying(32) NOT NULL,
    "is_primary_model" boolean NOT NULL,
    "request_body_sha256" "bytea" NOT NULL,
    "nonce_sha256" "bytea" NOT NULL,
    "status" character varying(32) NOT NULL,
    "upstream_status_code" smallint,
    "upstream_request_id" character varying(256),
    "headers_at" timestamp with time zone,
    "first_byte_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "duration_milliseconds" integer,
    "input_tokens" integer,
    "cached_input_tokens" integer,
    "cache_creation_input_tokens" integer,
    "output_tokens" integer,
    "reasoning_output_tokens" integer,
    "total_tokens" integer,
    "usage_json" "jsonb",
    "error_code" character varying(128),
    "started_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_model_invocations_cache_creation_input_tokens_check" CHECK ((("cache_creation_input_tokens" IS NULL) OR ("cache_creation_input_tokens" >= 0))),
    CONSTRAINT "workforce_model_invocations_cached_input_tokens_check" CHECK ((("cached_input_tokens" IS NULL) OR ("cached_input_tokens" >= 0))),
    CONSTRAINT "workforce_model_invocations_credential_version_check" CHECK (("credential_version" > 0)),
    CONSTRAINT "workforce_model_invocations_duration_milliseconds_check" CHECK ((("duration_milliseconds" IS NULL) OR ("duration_milliseconds" >= 0))),
    CONSTRAINT "workforce_model_invocations_execution_identity_ck" CHECK ((((("execution_kind")::"text" = 'AGENT_ATTEMPT'::"text") AND ("attempt_id" IS NOT NULL) AND ("review_run_id" IS NULL) AND ("review_run_attempt_id" IS NULL) AND ("generation" > 0)) OR ((("execution_kind")::"text" = 'REVIEW_RUN'::"text") AND ("attempt_id" IS NULL) AND ("review_run_id" IS NOT NULL) AND ("review_run_attempt_id" IS NOT NULL) AND ("generation" = 0)))),
    CONSTRAINT "workforce_model_invocations_execution_kind_ck" CHECK ((("execution_kind")::"text" = ANY ((ARRAY['AGENT_ATTEMPT'::character varying, 'REVIEW_RUN'::character varying])::"text"[]))),
    CONSTRAINT "workforce_model_invocations_input_tokens_check" CHECK ((("input_tokens" IS NULL) OR ("input_tokens" >= 0))),
    CONSTRAINT "workforce_model_invocations_nonce_sha256_check" CHECK (("octet_length"("nonce_sha256") = 32)),
    CONSTRAINT "workforce_model_invocations_operation_check" CHECK ((("operation")::"text" = ANY ((ARRAY['responses.create'::character varying, 'responses.compact'::character varying, 'messages.create'::character varying, 'messages.count_tokens'::character varying])::"text"[]))),
    CONSTRAINT "workforce_model_invocations_output_tokens_check" CHECK ((("output_tokens" IS NULL) OR ("output_tokens" >= 0))),
    CONSTRAINT "workforce_model_invocations_owner_epoch_ck" CHECK (("owner_epoch" > 0)),
    CONSTRAINT "workforce_model_invocations_protocol_check" CHECK ((("protocol")::"text" = ANY ((ARRAY['OPENAI'::character varying, 'ANTHROPIC'::character varying])::"text"[]))),
    CONSTRAINT "workforce_model_invocations_reasoning_output_tokens_check" CHECK ((("reasoning_output_tokens" IS NULL) OR ("reasoning_output_tokens" >= 0))),
    CONSTRAINT "workforce_model_invocations_request_body_sha256_check" CHECK (("octet_length"("request_body_sha256") = 32)),
    CONSTRAINT "workforce_model_invocations_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['AUTHORIZED'::character varying, 'STREAMING'::character varying, 'SUCCEEDED'::character varying, 'FAILED'::character varying, 'CANCELLED'::character varying, 'OUTCOME_UNKNOWN'::character varying])::"text"[]))),
    CONSTRAINT "workforce_model_invocations_total_tokens_check" CHECK ((("total_tokens" IS NULL) OR ("total_tokens" >= 0))),
    CONSTRAINT "workforce_model_invocations_upstream_status_code_check" CHECK ((("upstream_status_code" IS NULL) OR (("upstream_status_code" >= 100) AND ("upstream_status_code" <= 599))))
);



CREATE TABLE "public"."workforce_outbox_events" (
    "id" bigint NOT NULL,
    "aggregate_type" character varying(64) NOT NULL,
    "aggregate_id" bigint NOT NULL,
    "event_type" character varying(128) NOT NULL,
    "payload_json" "jsonb" NOT NULL,
    "status" character varying(32) NOT NULL,
    "attempts" integer DEFAULT 0 NOT NULL,
    "next_retry_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "published_at" timestamp with time zone,
    "last_error" "text",
    "exchange_name" "text",
    "routing_key" "text",
    "message_expiration_ms" bigint
);



CREATE TABLE "public"."workforce_principals" (
    "id" bigint NOT NULL,
    "principal_type" character varying(16) NOT NULL,
    "user_id" bigint,
    "worker_id" bigint,
    "service_code" character varying(128),
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_principals_shape_ck" CHECK ((((("principal_type")::"text" = 'USER'::"text") AND ("user_id" IS NOT NULL) AND ("worker_id" IS NULL) AND ("service_code" IS NULL)) OR ((("principal_type")::"text" = 'WORKER'::"text") AND ("user_id" IS NULL) AND ("worker_id" IS NOT NULL) AND ("service_code" IS NULL)) OR ((("principal_type")::"text" = 'SERVICE'::"text") AND ("user_id" IS NULL) AND ("worker_id" IS NULL) AND ("service_code" IS NOT NULL) AND (("service_code")::"text" <> ''::"text")))),
    CONSTRAINT "workforce_principals_type_ck" CHECK ((("principal_type")::"text" = ANY ((ARRAY['USER'::character varying, 'WORKER'::character varying, 'SERVICE'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_project_versions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "project_id" bigint NOT NULL,
    "name" "text" NOT NULL,
    "description" "text" DEFAULT ''::"text" NOT NULL,
    "department_id" bigint,
    "kind" character varying(32) NOT NULL,
    "repository_provider_type" character varying(32),
    "repository_scheme" character varying(32),
    "repository_host" "text",
    "repository_port" integer,
    "repository_path" "text",
    "repository_display_url" "text",
    "repository_web_url" "text",
    "repository_credential_ciphertext" "bytea",
    "repository_credential_edek" "bytea",
    "repository_credential_wrap_key_ref" "text",
    "repository_credential_version" integer,
    "repository_config_digest" character varying(64),
    "repository_capabilities_json" "jsonb",
    "repository_last_check_status" character varying(32),
    "repository_last_check_reason_code" character varying(128),
    "repository_last_checked_at" timestamp with time zone,
    "default_ref_name" "text",
    "tech_stack_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "agent_guidance" "text" DEFAULT ''::"text" NOT NULL,
    "completion_check_json" "jsonb" DEFAULT '{"mode": "NONE"}'::"jsonb" NOT NULL,
    "completion_check_digest" character varying(64) NOT NULL,
    "result_processor_release_id" bigint NOT NULL,
    "result_processor_parameters_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "review_prompt" "text" DEFAULT ''::"text" NOT NULL,
    "hook_release_ids" bigint[] DEFAULT '{}'::bigint[] NOT NULL,
    "delivery_mode" character varying(32) DEFAULT 'NONE'::character varying NOT NULL,
    "delivery_authorization" character varying(32) DEFAULT 'NONE'::character varying NOT NULL,
    "delivery_authorized_by_principal_id" bigint,
    "result_branch_pattern" "text" DEFAULT ''::"text" NOT NULL,
    "notification_enabled" boolean DEFAULT false NOT NULL,
    "notification_events_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "notification_group_route_id" bigint,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_project_versions_completion_check_ck" CHECK (("jsonb_typeof"("completion_check_json") = 'object'::"text")),
    CONSTRAINT "workforce_project_versions_completion_check_digest_ck" CHECK ((("completion_check_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_project_versions_created_by_principal_id_ck" CHECK (("created_by_principal_id" > 0)),
    CONSTRAINT "workforce_project_versions_delivery_authorization_ck" CHECK ((("delivery_authorization")::"text" = ANY ((ARRAY['NONE'::character varying, 'AUTO_AFTER_GATES'::character varying, 'ASK_AFTER_GATES'::character varying])::"text"[]))),
    CONSTRAINT "workforce_project_versions_delivery_mode_ck" CHECK ((("delivery_mode")::"text" = ANY ((ARRAY['NONE'::character varying, 'NEW_BRANCH'::character varying, 'NEW_BRANCH_AND_DRAFT_REVIEW'::character varying])::"text"[]))),
    CONSTRAINT "workforce_project_versions_delivery_shape_ck" CHECK ((((("delivery_mode")::"text" = 'NONE'::"text") AND (("delivery_authorization")::"text" = 'NONE'::"text") AND ("delivery_authorized_by_principal_id" IS NULL) AND ("result_branch_pattern" = ''::"text")) OR ((("kind")::"text" = 'CODEBASE'::"text") AND (("delivery_mode")::"text" <> 'NONE'::"text") AND ("btrim"("result_branch_pattern") <> ''::"text") AND (((("delivery_authorization")::"text" = 'AUTO_AFTER_GATES'::"text") AND ("delivery_authorized_by_principal_id" > 0)) OR ((("delivery_authorization")::"text" = 'ASK_AFTER_GATES'::"text") AND ("delivery_authorized_by_principal_id" IS NULL)))))),
    CONSTRAINT "workforce_project_versions_department_id_ck" CHECK ((("department_id" IS NULL) OR ("department_id" > 0))),
    CONSTRAINT "workforce_project_versions_id_ck" CHECK (("id" > 0)),
    CONSTRAINT "workforce_project_versions_kind_ck" CHECK ((("kind")::"text" = ANY ((ARRAY['CODEBASE'::character varying, 'GENERAL'::character varying])::"text"[]))),
    CONSTRAINT "workforce_project_versions_name_ck" CHECK (("btrim"("name") <> ''::"text")),
    CONSTRAINT "workforce_project_versions_notification_events_ck" CHECK (("jsonb_typeof"("notification_events_json") = 'array'::"text")),
    CONSTRAINT "workforce_project_versions_project_id_ck" CHECK (("project_id" > 0)),
    CONSTRAINT "workforce_project_versions_repository_shape_ck" CHECK ((((("kind")::"text" = 'CODEBASE'::"text") AND (("repository_scheme")::"text" = ANY ((ARRAY['http'::character varying, 'https'::character varying])::"text"[])) AND ("btrim"("repository_host") <> ''::"text") AND (("repository_port" >= 1) AND ("repository_port" <= 65535)) AND ("btrim"("repository_path") <> ''::"text") AND ("btrim"("repository_display_url") <> ''::"text") AND ((("repository_credential_ciphertext" IS NULL) AND ("repository_credential_edek" IS NULL) AND ("repository_credential_wrap_key_ref" IS NULL) AND ("repository_credential_version" IS NULL)) OR (("octet_length"("repository_credential_ciphertext") > 0) AND ("octet_length"("repository_credential_edek") > 0) AND ("btrim"("repository_credential_wrap_key_ref") <> ''::"text") AND ("repository_credential_version" > 0))) AND (("repository_config_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND ("btrim"("default_ref_name") <> ''::"text")) OR ((("kind")::"text" = 'GENERAL'::"text") AND ("repository_provider_type" IS NULL) AND ("repository_scheme" IS NULL) AND ("repository_host" IS NULL) AND ("repository_port" IS NULL) AND ("repository_path" IS NULL) AND ("repository_display_url" IS NULL) AND ("repository_web_url" IS NULL) AND ("repository_credential_ciphertext" IS NULL) AND ("repository_credential_edek" IS NULL) AND ("repository_credential_wrap_key_ref" IS NULL) AND ("repository_credential_version" IS NULL) AND ("repository_config_digest" IS NULL) AND ("repository_capabilities_json" IS NULL) AND ("repository_last_check_status" IS NULL) AND ("repository_last_check_reason_code" IS NULL) AND ("repository_last_checked_at" IS NULL) AND ("default_ref_name" IS NULL)))),
    CONSTRAINT "workforce_project_versions_result_processor_parameters_ck" CHECK (("jsonb_typeof"("result_processor_parameters_json") = 'object'::"text")),
    CONSTRAINT "workforce_project_versions_result_processor_release_id_ck" CHECK (("result_processor_release_id" > 0)),
    CONSTRAINT "workforce_project_versions_tech_stack_ck" CHECK (("jsonb_typeof"("tech_stack_json") = 'array'::"text")),
    CONSTRAINT "workforce_project_versions_tenant_id_ck" CHECK (("tenant_id" > 0))
);



CREATE TABLE "public"."workforce_projects" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "project_key" "text" NOT NULL,
    "status" character varying(32) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "current_version_id" bigint NOT NULL,
    "current_source_state" character varying(32),
    "current_source_revision_id" bigint,
    "created_by_principal_id" bigint NOT NULL,
    "updated_by_principal_id" bigint NOT NULL,
    "create_idempotency_key" "text" NOT NULL,
    "create_request_digest" character varying(64) NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "archived_at" timestamp with time zone,
    CONSTRAINT "workforce_projects_archive_shape_ck" CHECK ((((("status")::"text" = 'ACTIVE'::"text") AND ("archived_at" IS NULL)) OR ((("status")::"text" = 'ARCHIVED'::"text") AND ("archived_at" IS NOT NULL)))),
    CONSTRAINT "workforce_projects_create_idempotency_key_ck" CHECK (("btrim"("create_idempotency_key") <> ''::"text")),
    CONSTRAINT "workforce_projects_create_request_digest_ck" CHECK ((("create_request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_projects_created_by_principal_id_ck" CHECK (("created_by_principal_id" > 0)),
    CONSTRAINT "workforce_projects_current_version_id_ck" CHECK (("current_version_id" > 0)),
    CONSTRAINT "workforce_projects_id_ck" CHECK (("id" > 0)),
    CONSTRAINT "workforce_projects_project_key_ck" CHECK (("btrim"("project_key") <> ''::"text")),
    CONSTRAINT "workforce_projects_source_revision_id_ck" CHECK ((("current_source_revision_id" IS NULL) OR ("current_source_revision_id" > 0))),
    CONSTRAINT "workforce_projects_source_state_ck" CHECK ((("current_source_state" IS NULL) OR (("current_source_state")::"text" = ANY ((ARRAY['PREPARING'::character varying, 'READY'::character varying, 'FAILED'::character varying])::"text"[])))),
    CONSTRAINT "workforce_projects_status_ck" CHECK ((("status")::"text" = ANY ((ARRAY['ACTIVE'::character varying, 'ARCHIVED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_projects_tenant_id_ck" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_projects_updated_by_principal_id_ck" CHECK (("updated_by_principal_id" > 0))
);



CREATE TABLE "public"."workforce_public_events" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "integration_application_id" bigint NOT NULL,
    "source_task_event_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "event_type" character varying(128) NOT NULL,
    "sequence" integer NOT NULL,
    "correlation_id" character varying(256) NOT NULL,
    "payload_json" "jsonb" NOT NULL,
    "occurred_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_pull_request_publications" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "delivery_request_id" bigint NOT NULL,
    "delivery_operation_id" bigint NOT NULL,
    "project_id" bigint NOT NULL,
    "provider_type" character varying(32) NOT NULL,
    "provider_review_id" "text" DEFAULT ''::"text" NOT NULL,
    "provider_number" "text" DEFAULT ''::"text" NOT NULL,
    "status" character varying(32) NOT NULL,
    "head_branch" "text" NOT NULL,
    "base_branch" "text" NOT NULL,
    "head_commit" "text" NOT NULL,
    "frozen_base_commit" "text" NOT NULL,
    "observed_base_commit" "text" DEFAULT ''::"text" NOT NULL,
    "web_url" "text" DEFAULT ''::"text" NOT NULL,
    "marker_digest" "text" NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_reference_assets" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "file_asset_handle" "text" NOT NULL,
    "display_name" "text" NOT NULL,
    "content_type" character varying(255) DEFAULT 'application/octet-stream'::character varying NOT NULL,
    "content_sha256" character varying(64) NOT NULL,
    "size_bytes" bigint NOT NULL,
    "object_key" "text" NOT NULL,
    "object_sha256" character varying(64) NOT NULL,
    "status" character varying(32) NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_result_evaluation_cycles" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "execution_node_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "cycle_number" integer NOT NULL,
    "session_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "candidate_tree_digest" "text" DEFAULT ''::"text" NOT NULL,
    "findings_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "contract_digest" character varying(64) NOT NULL,
    "decision" character varying(32) NOT NULL,
    "reason_code" character varying(128) NOT NULL,
    "message" "text" DEFAULT ''::"text" NOT NULL,
    "contract_json" "jsonb" NOT NULL,
    "status" character varying(32) DEFAULT 'PERSISTED'::character varying NOT NULL,
    "resume_nonce_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "started_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "persisted_at" timestamp with time zone,
    "completed_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_result_materialization_runs" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "source_checkpoint_id" bigint NOT NULL,
    "output_id" bigint,
    "kind" character varying(32) NOT NULL,
    "input_identity_digest" character varying(64) NOT NULL,
    "worker_id" bigint,
    "worker_instance_id" "text",
    "host_boot_id" "text",
    "attempt_id" bigint NOT NULL,
    "generation" integer NOT NULL,
    "lease_sha256" character varying(64),
    "lease_expires_at" timestamp with time zone,
    "status" character varying(32) NOT NULL,
    "run_number" integer DEFAULT 1 NOT NULL,
    "max_runs" integer DEFAULT 3 NOT NULL,
    "artifact_id" bigint,
    "artifact_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "artifact_size_bytes" bigint DEFAULT 0 NOT NULL,
    "artifact_name" "text" DEFAULT ''::"text" NOT NULL,
    "artifact_media_type" character varying(255) DEFAULT ''::character varying NOT NULL,
    "spool_identity" "text" DEFAULT ''::"text" NOT NULL,
    "manifest_json" "jsonb",
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "next_retry_at" timestamp with time zone,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_result_outputs" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "evaluation_cycle_id" bigint NOT NULL,
    "name" "text" NOT NULL,
    "source_path" "text" NOT NULL,
    "output_kind" character varying(32) DEFAULT 'PATH'::character varying NOT NULL,
    "archive_format" character varying(32) DEFAULT ''::character varying NOT NULL,
    "selection_spec_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "selection_manifest_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "selection_manifest_object_key" "text" DEFAULT ''::"text" NOT NULL,
    "selection_manifest_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "selected_file_count" integer DEFAULT 0 NOT NULL,
    "storage" character varying(32) NOT NULL,
    "artifact_id" bigint,
    "sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "size_bytes" bigint DEFAULT 0 NOT NULL,
    "status" character varying(32) DEFAULT 'DECLARED'::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_result_processor_bindings" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "binding_name" "text" NOT NULL,
    "processor_release_id" bigint NOT NULL,
    "version" integer DEFAULT 1 NOT NULL,
    "changed_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_result_processor_environment_bindings" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "processor_release_id" bigint NOT NULL,
    "name" "text" NOT NULL,
    "type" character varying(32) NOT NULL,
    "phase" character varying(32) DEFAULT 'EVALUATE'::character varying NOT NULL,
    "config_json" "jsonb",
    "secret_ref" "text" DEFAULT ''::"text" NOT NULL,
    "secret_version" "text" DEFAULT ''::"text" NOT NULL,
    "task_override_allowed" boolean DEFAULT false NOT NULL,
    "network_policy" character varying(32) DEFAULT 'NONE'::character varying NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_result_processor_fixture_runs" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "revision_id" bigint NOT NULL,
    "revision_manifest_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "revision_files_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "revision_digest" "text" DEFAULT ''::"text" NOT NULL,
    "harness_release_id" bigint NOT NULL,
    "harness_image_digest" "text" NOT NULL,
    "evaluation_context_json" "jsonb" NOT NULL,
    "workspace_files_json" "jsonb" NOT NULL,
    "config_overrides_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "expected_decision" character varying(32) DEFAULT ''::character varying NOT NULL,
    "status" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    "worker_id" bigint,
    "worker_instance_id" "text",
    "generation" integer DEFAULT 1 NOT NULL,
    "result_contract_json" "jsonb",
    "result_contract_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "diagnostics" "text" DEFAULT ''::"text" NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_result_processor_releases" (
    "id" bigint NOT NULL,
    "processor_id" bigint NOT NULL,
    "scope_type" character varying(32) NOT NULL,
    "tenant_id" bigint,
    "revision_id" bigint NOT NULL,
    "version" "text" NOT NULL,
    "manifest_json" "jsonb" NOT NULL,
    "files_json" "jsonb" NOT NULL,
    "archive_object_key" "text" NOT NULL,
    "archive_sha256" character varying(64) NOT NULL,
    "archive_size_bytes" bigint NOT NULL,
    "published_by_principal_id" bigint NOT NULL,
    "published_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_result_processor_releases_check" CHECK ((((("scope_type")::"text" = 'SYSTEM'::"text") AND ("tenant_id" IS NULL)) OR ((("scope_type")::"text" = 'TENANT'::"text") AND ("tenant_id" IS NOT NULL)))),
    CONSTRAINT "workforce_result_processor_releases_scope_type_check" CHECK ((("scope_type")::"text" = ANY ((ARRAY['SYSTEM'::character varying, 'TENANT'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_result_processor_revisions" (
    "id" bigint NOT NULL,
    "processor_id" bigint NOT NULL,
    "scope_type" character varying(32) NOT NULL,
    "tenant_id" bigint,
    "revision_number" integer NOT NULL,
    "draft_version" integer DEFAULT 1 NOT NULL,
    "status" character varying(32) DEFAULT 'DRAFT'::character varying NOT NULL,
    "manifest_json" "jsonb" NOT NULL,
    "files_json" "jsonb" NOT NULL,
    "validation_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "validation_status" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_result_processor_revisions_check" CHECK ((((("scope_type")::"text" = 'SYSTEM'::"text") AND ("tenant_id" IS NULL)) OR ((("scope_type")::"text" = 'TENANT'::"text") AND ("tenant_id" IS NOT NULL)))),
    CONSTRAINT "workforce_result_processor_revisions_scope_type_check" CHECK ((("scope_type")::"text" = ANY ((ARRAY['SYSTEM'::character varying, 'TENANT'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_result_processors" (
    "id" bigint NOT NULL,
    "scope_type" character varying(32) NOT NULL,
    "tenant_id" bigint,
    "name" "text" NOT NULL,
    "category" character varying(64) DEFAULT ''::character varying NOT NULL,
    "description" "text" DEFAULT ''::"text" NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_result_processors_check" CHECK ((((("scope_type")::"text" = 'SYSTEM'::"text") AND ("tenant_id" IS NULL)) OR ((("scope_type")::"text" = 'TENANT'::"text") AND ("tenant_id" IS NOT NULL)))),
    CONSTRAINT "workforce_result_processors_scope_type_check" CHECK ((("scope_type")::"text" = ANY ((ARRAY['SYSTEM'::character varying, 'TENANT'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_review_run_attempts" (
    "id" bigint NOT NULL,
    "review_run_id" bigint NOT NULL,
    "attempt_number" integer NOT NULL,
    "runtime_release_id" bigint NOT NULL,
    "reviewer_image_digest" "text" NOT NULL,
    "adapter_identity" "text" NOT NULL,
    "adapter_version" "text" NOT NULL,
    "adapter_protocol" character varying(64) NOT NULL,
    "adapter_capabilities_json" "jsonb" NOT NULL,
    "model_credential_id" bigint NOT NULL,
    "credential_version" integer NOT NULL,
    "model_protocol" character varying(16) NOT NULL,
    "primary_model" character varying(256) NOT NULL,
    "provider_proxy_image_digest" "text" NOT NULL,
    "selection_digest" character varying(64) NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" "text" NOT NULL,
    "host_boot_id" "text" DEFAULT ''::"text" NOT NULL,
    "owner_epoch" integer NOT NULL,
    "status" character varying(32) NOT NULL,
    "lease_token_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "lease_expires_at" timestamp with time zone,
    "launch_permit_id" bigint,
    "launch_permit_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "launch_permit_expires_at" timestamp with time zone,
    "container_id" "text" DEFAULT ''::"text" NOT NULL,
    "container_name" "text" DEFAULT ''::"text" NOT NULL,
    "image_digest" "text" DEFAULT ''::"text" NOT NULL,
    "execution_identity_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "error_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "assigned_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "claimed_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_review_run_attempts_adapter_capabilities_json_check" CHECK (("jsonb_typeof"("adapter_capabilities_json") = 'array'::"text")),
    CONSTRAINT "workforce_review_run_attempts_adapter_identity_check" CHECK (("adapter_identity" <> ''::"text")),
    CONSTRAINT "workforce_review_run_attempts_adapter_protocol_check" CHECK ((("adapter_protocol")::"text" <> ''::"text")),
    CONSTRAINT "workforce_review_run_attempts_adapter_version_check" CHECK (("adapter_version" <> ''::"text")),
    CONSTRAINT "workforce_review_run_attempts_attempt_number_check" CHECK (("attempt_number" > 0)),
    CONSTRAINT "workforce_review_run_attempts_credential_version_check" CHECK (("credential_version" > 0)),
    CONSTRAINT "workforce_review_run_attempts_execution_identity_digest_check" CHECK (((("execution_identity_digest")::"text" = ''::"text") OR (("execution_identity_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_run_attempts_launch_permit_sha256_check" CHECK (((("launch_permit_sha256")::"text" = ''::"text") OR (("launch_permit_sha256")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_run_attempts_lease_token_sha256_check" CHECK (((("lease_token_sha256")::"text" = ''::"text") OR (("lease_token_sha256")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_run_attempts_model_credential_id_check" CHECK (("model_credential_id" > 0)),
    CONSTRAINT "workforce_review_run_attempts_model_protocol_check" CHECK ((("model_protocol")::"text" = ANY ((ARRAY['OPENAI'::character varying, 'ANTHROPIC'::character varying])::"text"[]))),
    CONSTRAINT "workforce_review_run_attempts_owner_epoch_check" CHECK (("owner_epoch" > 0)),
    CONSTRAINT "workforce_review_run_attempts_primary_model_check" CHECK ((("primary_model")::"text" <> ''::"text")),
    CONSTRAINT "workforce_review_run_attempts_provider_proxy_image_digest_check" CHECK (("provider_proxy_image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_review_run_attempts_review_run_id_check" CHECK (("review_run_id" > 0)),
    CONSTRAINT "workforce_review_run_attempts_reviewer_image_digest_check" CHECK (("reviewer_image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_review_run_attempts_runtime_release_id_check" CHECK (("runtime_release_id" > 0)),
    CONSTRAINT "workforce_review_run_attempts_selection_digest_check" CHECK ((("selection_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_review_run_attempts_slice4_runtime_ck" CHECK (((("container_id" = ''::"text") AND ("container_name" = ''::"text") AND ("image_digest" = ''::"text") AND (("execution_identity_digest")::"text" = ''::"text")) OR (("container_id" <> ''::"text") AND ("container_name" <> ''::"text") AND ("image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text") AND (("execution_identity_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")))),
    CONSTRAINT "workforce_review_run_attempts_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['ASSIGNED'::character varying, 'RUNNING'::character varying, 'VALIDATING_REPORT'::character varying, 'COMPLETED'::character varying, 'REJECTED'::character varying, 'FAILED'::character varying, 'EXPIRED'::character varying, 'CANCELLED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_review_run_attempts_worker_id_check" CHECK (("worker_id" > 0))
);



CREATE TABLE "public"."workforce_review_runs" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "finalization_run_id" bigint NOT NULL,
    "result_commit" "text" NOT NULL,
    "result_tree" "text" NOT NULL,
    "runtime_release_id" bigint,
    "reviewer_image_digest" "text" DEFAULT ''::"text" NOT NULL,
    "adapter_identity" "text" DEFAULT ''::"text" NOT NULL,
    "adapter_version" "text" DEFAULT ''::"text" NOT NULL,
    "adapter_protocol" character varying(64) DEFAULT ''::character varying NOT NULL,
    "adapter_capabilities_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "model_credential_id" bigint,
    "credential_version" integer,
    "model_protocol" character varying(16) DEFAULT ''::character varying NOT NULL,
    "primary_model" character varying(256) DEFAULT ''::character varying NOT NULL,
    "provider_proxy_image_digest" "text" DEFAULT ''::"text" NOT NULL,
    "selection_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "hard_timeout_seconds" integer NOT NULL,
    "transient_retry_limit" integer NOT NULL,
    "automatic_revision_limit" integer NOT NULL,
    "identity_digest" character varying(64) NOT NULL,
    "status" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    "worker_id" bigint,
    "worker_instance_id" "text" DEFAULT ''::"text" NOT NULL,
    "host_boot_id" "text" DEFAULT ''::"text" NOT NULL,
    "owner_epoch" integer DEFAULT 0 NOT NULL,
    "assignment_generation" integer DEFAULT 0 NOT NULL,
    "attempt_count" integer DEFAULT 0 NOT NULL,
    "lease_token_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "lease_expires_at" timestamp with time zone,
    "execution_deadline_at" timestamp with time zone,
    "launch_permit_id" bigint,
    "launch_permit_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "launch_permit_expires_at" timestamp with time zone,
    "runtime_container_id" "text" DEFAULT ''::"text" NOT NULL,
    "runtime_container_name" "text" DEFAULT ''::"text" NOT NULL,
    "runtime_execution_identity_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "canonical_report_json" "jsonb",
    "canonical_report_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "report_verdict" character varying(32) DEFAULT ''::character varying NOT NULL,
    "provider_usage_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "automatic_revision_number" integer DEFAULT 0 NOT NULL,
    "blocking_finding_count" integer DEFAULT 0 NOT NULL,
    "actionable_finding_digest" "text" DEFAULT ''::"text" NOT NULL,
    "deterministic_test_summary_digest" "text" DEFAULT ''::"text" NOT NULL,
    "deterministic_failure_count" integer DEFAULT 0 NOT NULL,
    "progress_signature" "text" DEFAULT ''::"text" NOT NULL,
    "no_progress_streak" integer DEFAULT 0 NOT NULL,
    "successor_task_turn_id" bigint,
    "closure_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "next_retry_at" timestamp with time zone,
    "wait_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "error_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "cancelled_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "completion_check_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    CONSTRAINT "workforce_review_runs_adapter_capabilities_json_check" CHECK (("jsonb_typeof"("adapter_capabilities_json") = 'array'::"text")),
    CONSTRAINT "workforce_review_runs_assignment_ck" CHECK ((((("runtime_release_id" IS NULL) AND ("reviewer_image_digest" = ''::"text") AND ("adapter_identity" = ''::"text") AND ("adapter_version" = ''::"text") AND (("adapter_protocol")::"text" = ''::"text") AND ("model_credential_id" IS NULL) AND ("credential_version" IS NULL) AND (("model_protocol")::"text" = ''::"text") AND (("primary_model")::"text" = ''::"text") AND ("provider_proxy_image_digest" = ''::"text") AND (("selection_digest")::"text" = ''::"text") AND ("worker_id" IS NULL) AND ("worker_instance_id" = ''::"text")) OR (("runtime_release_id" > 0) AND ("reviewer_image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text") AND ("adapter_identity" <> ''::"text") AND ("adapter_version" <> ''::"text") AND (("adapter_protocol")::"text" <> ''::"text") AND ("model_credential_id" > 0) AND ("credential_version" > 0) AND (("model_protocol")::"text" = ANY ((ARRAY['OPENAI'::character varying, 'ANTHROPIC'::character varying])::"text"[])) AND (("primary_model")::"text" <> ''::"text") AND ("provider_proxy_image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text") AND (("selection_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND ("worker_id" > 0) AND ("worker_instance_id" <> ''::"text"))) AND ((("status")::"text" <> ALL ((ARRAY['ASSIGNED'::character varying, 'RUNNING'::character varying, 'VALIDATING_REPORT'::character varying])::"text"[])) OR (("selection_digest")::"text" <> ''::"text")))),
    CONSTRAINT "workforce_review_runs_assignment_generation_check" CHECK (("assignment_generation" >= 0)),
    CONSTRAINT "workforce_review_runs_attempt_count_check" CHECK (("attempt_count" >= 0)),
    CONSTRAINT "workforce_review_runs_attempt_id_check" CHECK (("attempt_id" > 0)),
    CONSTRAINT "workforce_review_runs_automatic_revision_limit_check" CHECK ((("automatic_revision_limit" >= 0) AND ("automatic_revision_limit" <= 10))),
    CONSTRAINT "workforce_review_runs_canonical_report_digest_check" CHECK (((("canonical_report_digest")::"text" = ''::"text") OR (("canonical_report_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_completion_check_digest_ck" CHECK (((("completion_check_digest")::"text" = ''::"text") OR (("completion_check_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_finalization_run_id_check" CHECK (("finalization_run_id" > 0)),
    CONSTRAINT "workforce_review_runs_hard_timeout_seconds_check" CHECK ((("hard_timeout_seconds" >= 60) AND ("hard_timeout_seconds" <= 14400))),
    CONSTRAINT "workforce_review_runs_identity_digest_check" CHECK ((("identity_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_review_runs_launch_permit_sha256_check" CHECK (((("launch_permit_sha256")::"text" = ''::"text") OR (("launch_permit_sha256")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_lease_token_sha256_check" CHECK (((("lease_token_sha256")::"text" = ''::"text") OR (("lease_token_sha256")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_owner_epoch_check" CHECK (("owner_epoch" >= 0)),
    CONSTRAINT "workforce_review_runs_provider_proxy_image_digest_check" CHECK ((("provider_proxy_image_digest" = ''::"text") OR ("provider_proxy_image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_provider_usage_json_check" CHECK (("jsonb_typeof"("provider_usage_json") = 'object'::"text")),
    CONSTRAINT "workforce_review_runs_report_verdict_check" CHECK ((("report_verdict")::"text" = ANY ((ARRAY[''::character varying, 'PASS'::character varying, 'REVISE'::character varying, 'REJECT'::character varying])::"text"[]))),
    CONSTRAINT "workforce_review_runs_result_commit_check" CHECK (("result_commit" <> ''::"text")),
    CONSTRAINT "workforce_review_runs_result_tree_check" CHECK (("result_tree" <> ''::"text")),
    CONSTRAINT "workforce_review_runs_reviewer_image_digest_check" CHECK ((("reviewer_image_digest" = ''::"text") OR ("reviewer_image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_runtime_execution_identity_digest_check" CHECK (((("runtime_execution_identity_digest")::"text" = ''::"text") OR (("runtime_execution_identity_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_selection_digest_check" CHECK (((("selection_digest")::"text" = ''::"text") OR (("selection_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_review_runs_slice4_report_ck" CHECK (((("canonical_report_json" IS NULL) AND (("canonical_report_digest")::"text" = ''::"text") AND (("report_verdict")::"text" = ''::"text")) OR (("jsonb_typeof"("canonical_report_json") = 'object'::"text") AND (("canonical_report_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND (("report_verdict")::"text" = ANY ((ARRAY['PASS'::character varying, 'REVISE'::character varying, 'REJECT'::character varying])::"text"[]))))),
    CONSTRAINT "workforce_review_runs_slice4_runtime_ck" CHECK (((("runtime_container_id" = ''::"text") AND ("runtime_container_name" = ''::"text") AND (("runtime_execution_identity_digest")::"text" = ''::"text")) OR (("runtime_container_id" <> ''::"text") AND ("runtime_container_name" <> ''::"text") AND (("runtime_execution_identity_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")))),
    CONSTRAINT "workforce_review_runs_slice5_progress_ck" CHECK ((("automatic_revision_number" >= 0) AND ("blocking_finding_count" >= 0) AND ("deterministic_failure_count" >= 0) AND ("no_progress_streak" >= 0) AND (("actionable_finding_digest" = ''::"text") OR ("actionable_finding_digest" ~ '^[0-9a-f]{64}$'::"text")) AND (("deterministic_test_summary_digest" = ''::"text") OR ("deterministic_test_summary_digest" ~ '^[0-9a-f]{64}$'::"text")) AND (("progress_signature" = ''::"text") OR ("progress_signature" ~ '^[0-9a-f]{64}$'::"text")))),
    CONSTRAINT "workforce_review_runs_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'ASSIGNED'::character varying, 'RUNNING'::character varying, 'VALIDATING_REPORT'::character varying, 'PASSED'::character varying, 'REVISION_REQUIRED'::character varying, 'REJECTED'::character varying, 'RETRY_WAIT'::character varying, 'MANUAL_REVIEW'::character varying, 'CANCELLED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_review_runs_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_review_runs_task_turn_id_check" CHECK (("task_turn_id" > 0)),
    CONSTRAINT "workforce_review_runs_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_review_runs_transient_retry_limit_check" CHECK ((("transient_retry_limit" >= 0) AND ("transient_retry_limit" <= 10)))
);



CREATE TABLE "public"."workforce_runtime_reconciliation_runs" (
    "id" bigint NOT NULL,
    "attempt_id" bigint NOT NULL,
    "execution_node_id" bigint NOT NULL,
    "worker_id" bigint NOT NULL,
    "old_instance_id" "text" NOT NULL,
    "new_instance_id" "text" NOT NULL,
    "host_boot_id" "text" NOT NULL,
    "old_owner_epoch" integer NOT NULL,
    "new_owner_epoch" integer NOT NULL,
    "status" character varying(32) NOT NULL,
    "runtime_evidence_sha256" character varying(64) NOT NULL,
    "supervisor_ack_sha256" character varying(64),
    "provider_proxy_ack_sha256" character varying(64),
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "deadline" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "completed_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_source_ingestion_runs" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "workspace_source_revision_id" bigint,
    "kind" character varying(32) NOT NULL,
    "file_asset_handle" "text" DEFAULT ''::"text" NOT NULL,
    "input_object_key" "text" DEFAULT ''::"text" NOT NULL,
    "input_content_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "input_object_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "input_size_bytes" bigint DEFAULT 0 NOT NULL,
    "project_id" bigint,
    "project_version_id" bigint,
    "repository_config_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "requested_ref_type" character varying(32) DEFAULT ''::character varying NOT NULL,
    "requested_ref_name" "text" DEFAULT ''::"text" NOT NULL,
    "frozen_commit" "text" DEFAULT ''::"text" NOT NULL,
    "attempt_number" integer DEFAULT 0 NOT NULL,
    "max_attempts" integer DEFAULT 3 NOT NULL,
    "worker_id" bigint,
    "worker_instance_id" "text" DEFAULT ''::"text" NOT NULL,
    "host_boot_id" "text" DEFAULT ''::"text" NOT NULL,
    "lease_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "lease_expires_at" timestamp with time zone,
    "status" character varying(32) NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "next_retry_at" timestamp with time zone,
    "idempotency_key" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "finished_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_task_assignments" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "execution_node_id" bigint NOT NULL,
    "worker_id" bigint NOT NULL,
    "worker_instance_id" "text" NOT NULL,
    "generation" integer NOT NULL,
    "execution_requirement_digest" "text" NOT NULL,
    "agent_runtime_release_id" bigint NOT NULL,
    "runtime_image_ref" "text" NOT NULL,
    "runtime_image_digest" "text" NOT NULL,
    "adapter_identity" "text" NOT NULL,
    "adapter_version" "text" NOT NULL,
    "adapter_protocol" character varying(64) NOT NULL,
    "adapter_capabilities_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "hook_configuration_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "model_credential_id" bigint NOT NULL,
    "credential_version" integer NOT NULL,
    "model_protocol" character varying(16) NOT NULL,
    "primary_model" character varying(256) NOT NULL,
    "provider_proxy_identity" "text" NOT NULL,
    "worker_image_contract" "text" NOT NULL,
    "isolation_profile" character varying(64) NOT NULL,
    "resolved_execution_snapshot_json" "jsonb" NOT NULL,
    "selection_digest" character varying(64) NOT NULL,
    "lease_token_sha256" character varying(64) NOT NULL,
    "lease_expires_at" timestamp with time zone NOT NULL,
    "status" character varying(32) NOT NULL,
    "message_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "published_at" timestamp with time zone,
    "claimed_at" timestamp with time zone,
    "completed_at" timestamp with time zone,
    "released_at" timestamp with time zone,
    "release_reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "delivery_disposition" character varying(32) DEFAULT 'UNSETTLED'::character varying NOT NULL,
    "delivery_acked_at" timestamp with time zone,
    "dead_lettered_at" timestamp with time zone,
    CONSTRAINT "workforce_task_assignments_adapter_capabilities_json_check" CHECK (("jsonb_typeof"("adapter_capabilities_json") = 'array'::"text")),
    CONSTRAINT "workforce_task_assignments_adapter_identity_check" CHECK (("btrim"("adapter_identity") <> ''::"text")),
    CONSTRAINT "workforce_task_assignments_adapter_protocol_check" CHECK (("btrim"(("adapter_protocol")::"text") <> ''::"text")),
    CONSTRAINT "workforce_task_assignments_adapter_version_check" CHECK (("btrim"("adapter_version") <> ''::"text")),
    CONSTRAINT "workforce_task_assignments_agent_runtime_release_id_check" CHECK (("agent_runtime_release_id" > 0)),
    CONSTRAINT "workforce_task_assignments_credential_version_check" CHECK (("credential_version" > 0)),
    CONSTRAINT "workforce_task_assignments_hook_configuration_digest_check" CHECK (((("hook_configuration_digest")::"text" = ''::"text") OR (("hook_configuration_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))),
    CONSTRAINT "workforce_task_assignments_isolation_profile_check" CHECK (("btrim"(("isolation_profile")::"text") <> ''::"text")),
    CONSTRAINT "workforce_task_assignments_model_credential_id_check" CHECK (("model_credential_id" > 0)),
    CONSTRAINT "workforce_task_assignments_model_protocol_check" CHECK ((("model_protocol")::"text" = ANY ((ARRAY['OPENAI'::character varying, 'ANTHROPIC'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_assignments_primary_model_check" CHECK (("btrim"(("primary_model")::"text") <> ''::"text")),
    CONSTRAINT "workforce_task_assignments_provider_proxy_identity_check" CHECK (("btrim"("provider_proxy_identity") <> ''::"text")),
    CONSTRAINT "workforce_task_assignments_resolved_execution_snapshot_js_check" CHECK (("jsonb_typeof"("resolved_execution_snapshot_json") = 'object'::"text")),
    CONSTRAINT "workforce_task_assignments_runtime_image_digest_check" CHECK (("runtime_image_digest" ~ '^sha256:[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_task_assignments_runtime_image_ref_check" CHECK (("btrim"("runtime_image_ref") <> ''::"text")),
    CONSTRAINT "workforce_task_assignments_selection_digest_check" CHECK ((("selection_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_task_assignments_worker_image_contract_check" CHECK (("btrim"("worker_image_contract") <> ''::"text"))
);



CREATE TABLE "public"."workforce_task_attempts" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "attempt_no" integer NOT NULL,
    "generation" integer DEFAULT 1 NOT NULL,
    "worker_id" bigint NOT NULL,
    "container_id" character varying(128),
    "workspace_path" "text" NOT NULL,
    "status" character varying(32) NOT NULL,
    "lease_token" character varying(256) NOT NULL,
    "lease_expires_at" timestamp with time zone NOT NULL,
    "last_heartbeat_at" timestamp with time zone,
    "last_progress_at" timestamp with time zone,
    "runtime_driver" "text" NOT NULL,
    "runtime_image_digest" character varying(71),
    "image_contract_version" "text",
    "supervisor_protocol_version" integer,
    "base_commit" character varying(64),
    "result_commit" character varying(64),
    "result_tree" character varying(64),
    "started_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "finished_at" timestamp with time zone,
    "exit_code" integer,
    "error_code" character varying(128),
    "error_message" "text",
    "runtime_metrics_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "assignment_id" bigint,
    "execution_node_id" bigint,
    "worker_instance_id" "text",
    "host_boot_id" "text",
    "owner_epoch" integer DEFAULT 1 NOT NULL,
    "retry_class" character varying(32) DEFAULT ''::character varying NOT NULL,
    "retry_of_attempt_id" bigint,
    "execution_completed_at" timestamp with time zone,
    "lost_at" timestamp with time zone,
    "timeout_stage" character varying(32) DEFAULT ''::character varying NOT NULL,
    "timed_out_at" timestamp with time zone,
    "assignment_selection_digest" character varying(64) NOT NULL,
    "resolved_execution_snapshot_json" "jsonb" NOT NULL,
    "task_turn_id" bigint,
    CONSTRAINT "workforce_task_attempts_assignment_selection_digest_ck" CHECK ((("assignment_selection_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_task_attempts_resolved_execution_snapshot_ck" CHECK (("jsonb_typeof"("resolved_execution_snapshot_json") = 'object'::"text"))
);



CREATE TABLE "public"."workforce_task_conversation_items" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "interaction_id" bigint,
    "version" integer NOT NULL,
    "type" character varying(32) NOT NULL,
    "source_kind" character varying(32) NOT NULL,
    "source_key" character varying(256) NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "body_json" "jsonb" NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    "change_sequence" integer NOT NULL,
    CONSTRAINT "workforce_task_conversation_items_body_json_check" CHECK ((("jsonb_typeof"("body_json") = 'object'::"text") AND ("octet_length"(("body_json")::"text") <= 65536))),
    CONSTRAINT "workforce_task_conversation_items_change_sequence_check" CHECK (("change_sequence" > 0)),
    CONSTRAINT "workforce_task_conversation_items_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_task_conversation_items_interaction_id_check" CHECK (("interaction_id" > 0)),
    CONSTRAINT "workforce_task_conversation_items_request_digest_check" CHECK ((("request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_task_conversation_items_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_task_conversation_items_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_task_conversation_items_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_task_conversation_read_states" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "watermark" integer NOT NULL,
    "updated_at" timestamp with time zone NOT NULL,
    CONSTRAINT "workforce_task_conversation_read_state_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_task_conversation_read_states_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_task_conversation_read_states_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_task_conversation_read_states_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_task_conversation_read_states_watermark_check" CHECK (("watermark" >= 0))
);



CREATE TABLE "public"."workforce_task_events" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "attempt_id" bigint,
    "event_type" character varying(128) NOT NULL,
    "sequence" integer NOT NULL,
    "payload_json" "jsonb" NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "open_integration_projection_status" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    CONSTRAINT "workforce_task_events_open_projection_status_ck" CHECK ((("open_integration_projection_status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'PROJECTED'::character varying, 'SKIPPED'::character varying])::"text"[])))
);



CREATE SEQUENCE "public"."workforce_task_events_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;



ALTER SEQUENCE "public"."workforce_task_events_id_seq" OWNED BY "public"."workforce_task_events"."id";



CREATE TABLE "public"."workforce_task_git_plans" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "project_id" bigint,
    "project_version_id" bigint,
    "repository_provider_type" character varying(32) DEFAULT ''::character varying NOT NULL,
    "repository_scheme" character varying(32) DEFAULT ''::character varying NOT NULL,
    "repository_host" "text" DEFAULT ''::"text" NOT NULL,
    "repository_port" integer DEFAULT 0 NOT NULL,
    "repository_path" "text" DEFAULT ''::"text" NOT NULL,
    "repository_display_url" "text" DEFAULT ''::"text" NOT NULL,
    "repository_web_url" "text" DEFAULT ''::"text" NOT NULL,
    "repository_config_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "repository_credential_ciphertext" "bytea" DEFAULT '\x'::"bytea" NOT NULL,
    "repository_credential_edek" "bytea" DEFAULT '\x'::"bytea" NOT NULL,
    "repository_credential_wrap_key_ref" "text" DEFAULT ''::"text" NOT NULL,
    "repository_credential_version" integer DEFAULT 0 NOT NULL,
    "review_base_source_revision_id" bigint,
    "source_ref_type" character varying(32) DEFAULT ''::character varying NOT NULL,
    "source_ref_name" "text" DEFAULT ''::"text" NOT NULL,
    "source_commit" "text" NOT NULL,
    "review_base_branch" "text" DEFAULT ''::"text" NOT NULL,
    "review_base_commit" "text" DEFAULT ''::"text" NOT NULL,
    "result_branch" "text" DEFAULT ''::"text" NOT NULL,
    "delivery_mode" character varying(32) NOT NULL,
    "delivery_authorization" character varying(32) DEFAULT ''::character varying NOT NULL,
    "delivery_authorized_by_principal_id" bigint,
    "delivery_authorized_at" timestamp with time zone,
    "target_config_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "push_authorization_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "review_authorization_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "full_project_import" boolean DEFAULT false NOT NULL,
    "previous_delivery_commit" "text" DEFAULT ''::"text" NOT NULL,
    "plan_digest" "text" NOT NULL,
    "status" character varying(32) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_task_git_plans_delivery_authorization_ck" CHECK ((((("delivery_mode")::"text" = 'NONE'::"text") AND (("delivery_authorization")::"text" = ''::"text") AND ("delivery_authorized_by_principal_id" IS NULL) AND ("delivery_authorized_at" IS NULL) AND (("target_config_digest")::"text" = ''::"text") AND (("push_authorization_digest")::"text" = ''::"text") AND (("review_authorization_digest")::"text" = ''::"text")) OR ((("delivery_mode")::"text" = ANY ((ARRAY['NEW_BRANCH'::character varying, 'NEW_BRANCH_AND_DRAFT_REVIEW'::character varying])::"text"[])) AND (((("delivery_authorization")::"text" = 'ASK_AFTER_GATES'::"text") AND ("delivery_authorized_by_principal_id" IS NULL) AND ("delivery_authorized_at" IS NULL) AND ((("target_config_digest")::"text" = ''::"text") OR (("target_config_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")) AND ((("push_authorization_digest")::"text" = ''::"text") OR (("push_authorization_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")) AND ((("review_authorization_digest")::"text" = ''::"text") OR (("review_authorization_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))) OR ((("delivery_authorization")::"text" = 'AUTO_AFTER_GATES'::"text") AND ("delivery_authorized_by_principal_id" > 0) AND ("delivery_authorized_at" IS NOT NULL) AND (("target_config_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND (("push_authorization_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND (((("delivery_mode")::"text" = 'NEW_BRANCH'::"text") AND (("review_authorization_digest")::"text" = ''::"text")) OR ((("delivery_mode")::"text" = 'NEW_BRANCH_AND_DRAFT_REVIEW'::"text") AND (("review_authorization_digest")::"text" ~ '^[0-9a-f]{64}$'::"text"))))))))
);



CREATE TABLE "public"."workforce_task_input_references" (
    "task_input_snapshot_id" bigint NOT NULL,
    "reference_asset_id" bigint NOT NULL,
    "reference_content_sha256" character varying(64) NOT NULL,
    "display_order" integer DEFAULT 0 NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_task_input_snapshots" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "source_kind" character varying(32) NOT NULL,
    "workspace_source_revision_id" bigint,
    "source_commit" "text" NOT NULL,
    "source_tree" "text" NOT NULL,
    "source_identity_digest" character varying(64) NOT NULL,
    "repository_bundle_object_key" "text" NOT NULL,
    "repository_bundle_content_sha256" character varying(64) NOT NULL,
    "repository_bundle_object_sha256" character varying(64) NOT NULL,
    "repository_bundle_size_bytes" bigint NOT NULL,
    "input_snapshot_digest" "text" NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_task_network_grants" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "source_decision_id" bigint NOT NULL,
    "actor_principal_id" bigint NOT NULL,
    "targets_json" "jsonb" NOT NULL,
    "targets_digest" character varying(64) NOT NULL,
    "status" character varying(16) NOT NULL,
    "version" integer NOT NULL,
    "revoke_idempotency_key" character varying(128),
    "revoke_request_digest" character varying(64),
    "expires_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone NOT NULL,
    "updated_at" timestamp with time zone NOT NULL,
    "revoked_at" timestamp with time zone,
    CONSTRAINT "workforce_task_network_grants_actor_principal_id_check" CHECK (("actor_principal_id" > 0)),
    CONSTRAINT "workforce_task_network_grants_check" CHECK (("expires_at" > "created_at")),
    CONSTRAINT "workforce_task_network_grants_check1" CHECK (((("revoke_idempotency_key" IS NULL) AND ("revoke_request_digest" IS NULL)) OR ((("octet_length"(("revoke_idempotency_key")::"text") >= 1) AND ("octet_length"(("revoke_idempotency_key")::"text") <= 128)) AND (("revoke_request_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")))),
    CONSTRAINT "workforce_task_network_grants_id_check" CHECK (("id" > 0)),
    CONSTRAINT "workforce_task_network_grants_source_decision_id_check" CHECK (("source_decision_id" > 0)),
    CONSTRAINT "workforce_task_network_grants_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['ACTIVE'::character varying, 'REVOKED'::character varying, 'EXPIRED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_network_grants_targets_digest_check" CHECK ((("targets_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_task_network_grants_targets_json_check" CHECK ((("jsonb_typeof"("targets_json") = 'array'::"text") AND (("jsonb_array_length"("targets_json") >= 1) AND ("jsonb_array_length"("targets_json") <= 8)))),
    CONSTRAINT "workforce_task_network_grants_task_id_check" CHECK (("task_id" > 0)),
    CONSTRAINT "workforce_task_network_grants_tenant_id_check" CHECK (("tenant_id" > 0)),
    CONSTRAINT "workforce_task_network_grants_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_task_notification_record_channels" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "record_id" bigint NOT NULL,
    "channel_kind" character varying(32) NOT NULL,
    "group_route_id" bigint,
    "mention_user_id" bigint,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_task_notification_record_channels_channel_kind_check" CHECK ((("channel_kind")::"text" = ANY ((ARRAY['IN_APP'::character varying, 'EMAIL_USER'::character varying, 'FEISHU_DIRECT'::character varying, 'WECOM_DIRECT'::character varying, 'GROUP_ROUTE'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_notification_record_channels_check" CHECK ((((("channel_kind")::"text" = 'GROUP_ROUTE'::"text") AND ("group_route_id" IS NOT NULL)) OR ((("channel_kind")::"text" <> 'GROUP_ROUTE'::"text") AND ("group_route_id" IS NULL))))
);



CREATE TABLE "public"."workforce_task_notification_record_recipients" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "record_id" bigint NOT NULL,
    "user_id" bigint NOT NULL,
    "recipient_role" character varying(32) NOT NULL,
    "personal_channel_policy" character varying(32) NOT NULL,
    "in_app_required" boolean DEFAULT false NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_task_notification_recor_personal_channel_policy_check" CHECK ((("personal_channel_policy")::"text" = ANY ((ARRAY['TASK_SUBSCRIPTION'::character varying, 'USER_PREFERENCE'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_notification_record_recipie_recipient_role_check" CHECK ((("recipient_role")::"text" = ANY ((ARRAY['SUBSCRIBER'::character varying, 'ATTENTION'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_task_notification_records" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "task_turn_id" bigint NOT NULL,
    "source_type" character varying(64) NOT NULL,
    "source_id" bigint NOT NULL,
    "source_occurrence" bigint DEFAULT 0 NOT NULL,
    "fact_identity_version" integer DEFAULT 1 NOT NULL,
    "fact_type" character varying(64) NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "focus" character varying(32) NOT NULL,
    "subscription_id" bigint,
    "subscriber_user_id" bigint NOT NULL,
    "subscription_version" integer,
    "task_name_snapshot" "text" NOT NULL,
    "turn_number" integer NOT NULL,
    "summary_snapshot" "text" DEFAULT ''::"text" NOT NULL,
    "title_snapshot" "text" NOT NULL,
    "content_snapshot" "text" NOT NULL,
    "publication_contract_version" integer DEFAULT 1 NOT NULL,
    "scene_code" "text" DEFAULT ''::"text" NOT NULL,
    "template_variables_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "deep_link" "text" NOT NULL,
    "idempotency_identity" "text" NOT NULL,
    "request_digest" character varying(64) NOT NULL,
    "status" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    "publish_attempts" integer DEFAULT 0 NOT NULL,
    "next_retry_at" timestamp with time zone,
    "lease_token" "text" DEFAULT ''::"text" NOT NULL,
    "lease_until" timestamp with time zone,
    "notification_receipt_identity" "text" DEFAULT ''::"text" NOT NULL,
    "warning_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "last_error_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "occurred_at" timestamp with time zone NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "published_at" timestamp with time zone,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_task_notification__publication_contract_version_check" CHECK (("publication_contract_version" = ANY (ARRAY[1, 2]))),
    CONSTRAINT "workforce_task_notification_records_check" CHECK (((("status")::"text" = 'SUPPRESSED'::"text") OR (("subscription_id" IS NOT NULL) AND ("subscription_version" IS NOT NULL)))),
    CONSTRAINT "workforce_task_notification_records_fact_identity_version_check" CHECK (("fact_identity_version" = ANY (ARRAY[1, 2]))),
    CONSTRAINT "workforce_task_notification_records_fact_type_check" CHECK ((("fact_type")::"text" = ANY ((ARRAY['TASK_ACTION_REQUIRED'::character varying, 'TASK_INTERRUPTED'::character varying, 'TASK_COMPLETED'::character varying, 'DELIVERY_COMPLETED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_notification_records_focus_check" CHECK ((("focus")::"text" = ANY ((ARRAY['evaluation'::character varying, 'result'::character varying, 'continuation'::character varying, 'delivery'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_notification_records_publish_attempts_check" CHECK (("publish_attempts" >= 0)),
    CONSTRAINT "workforce_task_notification_records_source_occurrence_check" CHECK (("source_occurrence" >= 0)),
    CONSTRAINT "workforce_task_notification_records_source_type_check" CHECK ((("source_type")::"text" = ANY ((ARRAY['CONTINUATION_CHECKPOINT'::character varying, 'FINALIZATION_RUN'::character varying, 'ARTIFACT_UPLOAD'::character varying, 'RESULT_EVALUATION'::character varying, 'TASK_ATTEMPT'::character varying, 'DELIVERY_REQUEST'::character varying, 'REVIEW_OPERATION'::character varying, 'INDEPENDENT_REVIEW_RUN'::character varying, 'RUNTIME_RECOVERY'::character varying, 'TASK_TIMEOUT'::character varying, 'TASK_CANCEL'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_notification_records_status_check" CHECK ((("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'PROCESSING'::character varying, 'RETRY_WAIT'::character varying, 'PUBLISHED'::character varying, 'SUPPRESSED'::character varying, 'FAILED'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_notification_records_subscription_version_check" CHECK (("subscription_version" > 0)),
    CONSTRAINT "workforce_task_notification_records_turn_number_check" CHECK (("turn_number" > 0))
);



CREATE TABLE "public"."workforce_task_notification_subscription_channels" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "subscription_id" bigint NOT NULL,
    "channel_kind" character varying(32) NOT NULL,
    "group_route_id" bigint,
    "created_by" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_task_notification_subscription_cha_channel_kind_check" CHECK ((("channel_kind")::"text" = ANY ((ARRAY['IN_APP'::character varying, 'EMAIL_USER'::character varying, 'FEISHU_DIRECT'::character varying, 'WECOM_DIRECT'::character varying, 'GROUP_ROUTE'::character varying])::"text"[]))),
    CONSTRAINT "workforce_task_notification_subscription_channels_check" CHECK ((((("channel_kind")::"text" = 'GROUP_ROUTE'::"text") AND ("group_route_id" IS NOT NULL)) OR ((("channel_kind")::"text" <> 'GROUP_ROUTE'::"text") AND ("group_route_id" IS NULL))))
);



CREATE TABLE "public"."workforce_task_notification_subscriptions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "subscriber_user_id" bigint NOT NULL,
    "events_json" "jsonb" NOT NULL,
    "enabled" boolean DEFAULT true NOT NULL,
    "version" integer DEFAULT 1 NOT NULL,
    "created_by" bigint NOT NULL,
    "updated_by" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_task_notification_subscriptions_events_json_check" CHECK (("jsonb_typeof"("events_json") = 'array'::"text")),
    CONSTRAINT "workforce_task_notification_subscriptions_version_check" CHECK (("version" > 0))
);



CREATE TABLE "public"."workforce_task_turns" (
    "id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "turn_number" integer NOT NULL,
    "instruction" "text" NOT NULL,
    "source_turn_id" bigint,
    "source_checkpoint_id" bigint,
    "execution_snapshot_digest" "text" NOT NULL,
    "continuation_lineage_id" bigint NOT NULL,
    "created_by" bigint NOT NULL,
    "result_commit" "text" DEFAULT ''::"text" NOT NULL,
    "result_tree" "text" DEFAULT ''::"text" NOT NULL,
    "result_summary" "text" DEFAULT ''::"text" NOT NULL,
    "resume_nonce_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "idempotency_key" "text",
    "request_digest" character varying(64),
    "review_revision_kind" character varying(32),
    "source_review_run_id" bigint,
    "review_report_digest" character varying(64),
    "revision_trigger_principal_id" bigint,
    "processor_release_id" bigint NOT NULL,
    "processor_digest" character varying(64) NOT NULL,
    "environment_config_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "environment_snapshot_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "secret_version_identities" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "parameters_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "session_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "status" character varying(32) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "completion_check_json" "jsonb",
    "completion_check_input_digest" character varying(64),
    "completion_check_baseline_result_id" bigint,
    "completion_check_digest" character varying(64),
    "completion_check_references_json" "jsonb",
    "source_interaction_response_id" bigint,
    "interaction_response_json" "jsonb",
    "source_execution_operation_id" bigint,
    CONSTRAINT "workforce_task_turns_review_revision_ck" CHECK (((("review_revision_kind" IS NULL) AND ("source_review_run_id" IS NULL) AND ("review_report_digest" IS NULL) AND ("revision_trigger_principal_id" IS NULL)) OR ((("review_revision_kind")::"text" = 'AUTOMATIC'::"text") AND ("source_review_run_id" IS NOT NULL) AND (("review_report_digest")::"text" ~ '^[0-9a-f]{64}$'::"text") AND ("revision_trigger_principal_id" IS NOT NULL)) OR ((("review_revision_kind")::"text" = 'HUMAN_AUTHORIZED'::"text") AND ("source_review_run_id" IS NOT NULL) AND (("review_report_digest" IS NULL) OR (("review_report_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")) AND ("revision_trigger_principal_id" IS NOT NULL)))),
    CONSTRAINT "workforce_task_turns_source_execution_operation_ck" CHECK ((("source_execution_operation_id" IS NULL) OR (("source_execution_operation_id" > 0) AND ("source_checkpoint_id" IS NOT NULL)))),
    CONSTRAINT "workforce_turn_interaction_response_ck" CHECK (((("source_interaction_response_id" IS NULL) AND ("interaction_response_json" IS NULL)) OR (("source_interaction_response_id" IS NOT NULL) AND ("source_interaction_response_id" > 0) AND ("source_checkpoint_id" IS NOT NULL) AND ("interaction_response_json" IS NOT NULL) AND ("jsonb_typeof"("interaction_response_json") = 'object'::"text") AND ("octet_length"(("interaction_response_json")::"text") <= 16384) AND ("interaction_response_json" ?& ARRAY['interactionKey'::"text", 'responseId'::"text", 'answers'::"text"]) AND (("interaction_response_json" - ARRAY['interactionKey'::"text", 'responseId'::"text", 'answers'::"text"]) = '{}'::"jsonb") AND (("interaction_response_json" ->> 'responseId'::"text") = ("source_interaction_response_id")::"text") AND ("jsonb_typeof"(("interaction_response_json" -> 'interactionKey'::"text")) = 'string'::"text") AND ("jsonb_typeof"(("interaction_response_json" -> 'answers'::"text")) = 'array'::"text"))))
);



CREATE TABLE "public"."workforce_tasks" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "created_by" bigint NOT NULL,
    "idempotency_key" character varying(128) NOT NULL,
    "request_hash" character varying(64) NOT NULL,
    "repository_url" "text" NOT NULL,
    "repository_ref" "text" NOT NULL,
    "resolved_base_commit" character varying(64),
    "task_instruction" "text" NOT NULL,
    "validation_commands_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "status" character varying(32) NOT NULL,
    "priority" integer DEFAULT 0 NOT NULL,
    "max_attempts" integer DEFAULT 1 NOT NULL,
    "attempt_count" integer DEFAULT 0 NOT NULL,
    "result_summary" "text",
    "latest_attempt_id" bigint,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "queued_at" timestamp with time zone,
    "started_at" timestamp with time zone,
    "finished_at" timestamp with time zone,
    "verified_at" timestamp with time zone,
    "version" integer DEFAULT 0 NOT NULL,
    "cancel_request_id" bigint,
    "cancel_generation" integer DEFAULT 0 NOT NULL,
    "cancel_requested_by_principal_id" bigint,
    "cancel_requested_at" timestamp with time zone,
    "cancel_reason" "text" DEFAULT ''::"text" NOT NULL,
    "quality_outcome" character varying(64) DEFAULT ''::"text" NOT NULL,
    "result_processor_release_id" bigint NOT NULL,
    "result_processor_parameters_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "project_id" bigint NOT NULL,
    "project_version_id" bigint NOT NULL,
    "review_enabled" boolean DEFAULT true NOT NULL,
    "model_credential_id" bigint,
    "primary_model" character varying(256),
    "selected_runtime_release_id" bigint,
    "execution_timeout_seconds" integer,
    "execution_active_milliseconds" bigint DEFAULT 0 NOT NULL,
    "execution_requirements_json" "jsonb" NOT NULL,
    "execution_requirements_digest" character varying(64) NOT NULL,
    "current_task_turn_id" bigint,
    "current_turn_number" integer DEFAULT 1 NOT NULL,
    "name" "text" DEFAULT ''::"text" NOT NULL,
    "tags_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "metadata_version" integer DEFAULT 1 NOT NULL,
    "submission_channel" character varying(32) DEFAULT 'WEB'::character varying NOT NULL,
    "integration_application_id" bigint,
    "scenario_key" "text",
    "scenario_version" integer,
    "scenario_digest" character varying(64),
    "external_correlation_id" character varying(256),
    CONSTRAINT "workforce_tasks_execution_active_milliseconds_ck" CHECK (("execution_active_milliseconds" >= 0)),
    CONSTRAINT "workforce_tasks_execution_requirements_ck" CHECK (("jsonb_typeof"("execution_requirements_json") = 'object'::"text")),
    CONSTRAINT "workforce_tasks_execution_requirements_digest_ck" CHECK ((("execution_requirements_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_tasks_execution_timeout_seconds_ck" CHECK ((("execution_timeout_seconds" IS NULL) OR ("execution_timeout_seconds" > 0))),
    CONSTRAINT "workforce_tasks_model_selection_ck" CHECK (((("model_credential_id" IS NULL) AND ("primary_model" IS NULL)) OR (("model_credential_id" > 0) AND ("btrim"(("primary_model")::"text") <> ''::"text")))),
    CONSTRAINT "workforce_tasks_project_id_ck" CHECK (("project_id" > 0)),
    CONSTRAINT "workforce_tasks_project_version_id_ck" CHECK (("project_version_id" > 0)),
    CONSTRAINT "workforce_tasks_quality_outcome_ck" CHECK ((("quality_outcome")::"text" = ANY (ARRAY[''::"text", 'INDEPENDENT_REVIEW_PASSED'::"text", 'HUMAN_ACCEPTED'::"text"]))),
    CONSTRAINT "workforce_tasks_result_processor_parameters_ck" CHECK (("jsonb_typeof"("result_processor_parameters_json") = 'object'::"text")),
    CONSTRAINT "workforce_tasks_result_processor_release_id_ck" CHECK (("result_processor_release_id" > 0)),
    CONSTRAINT "workforce_tasks_selected_runtime_release_id_ck" CHECK ((("selected_runtime_release_id" IS NULL) OR ("selected_runtime_release_id" > 0)))
);



CREATE TABLE "public"."workforce_webhook_deliveries" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "integration_application_id" bigint NOT NULL,
    "public_event_id" bigint NOT NULL,
    "task_id" bigint NOT NULL,
    "event_type" character varying(128) NOT NULL,
    "status" character varying(32) DEFAULT 'PENDING'::character varying NOT NULL,
    "attempt_count" integer DEFAULT 0 NOT NULL,
    "next_attempt_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "lease_owner" character varying(256),
    "lease_expires_at" timestamp with time zone,
    "last_http_status" smallint,
    "last_failure_class" character varying(128) DEFAULT ''::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "delivered_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_webhook_delivery_attempts" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "integration_application_id" bigint NOT NULL,
    "delivery_id" bigint NOT NULL,
    "signing_credential_id" bigint NOT NULL,
    "grace_signing_credential_id" bigint,
    "attempt_number" integer NOT NULL,
    "status" character varying(32) NOT NULL,
    "http_status" smallint,
    "failure_class" character varying(128) DEFAULT ''::character varying NOT NULL,
    "duration_millis" integer DEFAULT 0 NOT NULL,
    "attempted_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_worker_images" (
    "worker_id" bigint NOT NULL,
    "instance_id" "text" NOT NULL,
    "image_digest" "text" NOT NULL,
    "image_ref" "text" DEFAULT ''::"text" NOT NULL,
    "role" character varying(32) NOT NULL,
    "contract_version" "text" NOT NULL,
    "manifest_digest" character varying(64) NOT NULL,
    "manifest_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "capabilities_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "scheduling_scope" character varying(32) DEFAULT 'PRODUCTION'::character varying NOT NULL,
    "scheduling_priority" integer DEFAULT 100 NOT NULL,
    "environment_capabilities_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "execution_kinds_json" "jsonb" DEFAULT '["AGENT_ATTEMPT", "REVIEW_RUN"]'::"jsonb" NOT NULL,
    "scheduling_digest" character varying(64) NOT NULL,
    "admission_status" character varying(32) NOT NULL,
    "admission_evidence_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "presence_status" character varying(32) DEFAULT 'PRESENT'::character varying NOT NULL,
    "missing_since" timestamp with time zone,
    "drift_reason" "text" DEFAULT ''::"text" NOT NULL,
    "inventory_version" integer NOT NULL,
    "inventory_digest" "text" NOT NULL,
    "last_seen_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_worker_images_environment_capabilities_ck" CHECK (("jsonb_typeof"("environment_capabilities_json") = 'array'::"text")),
    CONSTRAINT "workforce_worker_images_execution_kinds_ck" CHECK ((("jsonb_typeof"("execution_kinds_json") = 'array'::"text") AND (((("role")::"text" = 'attempt'::"text") AND ("jsonb_array_length"("execution_kinds_json") > 0)) OR ((("role")::"text" <> 'attempt'::"text") AND ("jsonb_array_length"("execution_kinds_json") = 0))) AND ("execution_kinds_json" <@ '["AGENT_ATTEMPT", "REVIEW_RUN"]'::"jsonb"))),
    CONSTRAINT "workforce_worker_images_scheduling_digest_ck" CHECK ((("scheduling_digest")::"text" ~ '^[0-9a-f]{64}$'::"text")),
    CONSTRAINT "workforce_worker_images_scheduling_priority_ck" CHECK ((("scheduling_priority" >= '-100000'::integer) AND ("scheduling_priority" <= 100000))),
    CONSTRAINT "workforce_worker_images_scheduling_scope_ck" CHECK ((("scheduling_scope")::"text" = ANY ((ARRAY['PRODUCTION'::character varying, 'TEST_ONLY'::character varying])::"text"[])))
);



CREATE TABLE "public"."workforce_worker_metric_samples" (
    "worker_id" bigint NOT NULL,
    "sampled_at" timestamp with time zone NOT NULL,
    "logical_cpu_cores" smallint NOT NULL,
    "cpu_usage_percent" real,
    "load1" real,
    "memory_used_bytes" bigint NOT NULL,
    "memory_total_bytes" bigint NOT NULL,
    "memory_usage_percent" real NOT NULL,
    "workspace_disk_used_bytes" bigint NOT NULL,
    "workspace_disk_total_bytes" bigint NOT NULL,
    "workspace_disk_usage_percent" real NOT NULL,
    "active_attempts" smallint NOT NULL,
    "max_active_attempts" smallint NOT NULL,
    "docker_ready" boolean NOT NULL,
    CONSTRAINT "workforce_worker_metric_samp_workspace_disk_usage_percent_check" CHECK ((("workspace_disk_usage_percent" >= (0)::double precision) AND ("workspace_disk_usage_percent" <= (100)::double precision))),
    CONSTRAINT "workforce_worker_metric_samples_active_attempts_check" CHECK (("active_attempts" >= 0)),
    CONSTRAINT "workforce_worker_metric_samples_check" CHECK (("memory_total_bytes" >= "memory_used_bytes")),
    CONSTRAINT "workforce_worker_metric_samples_check1" CHECK (("workspace_disk_total_bytes" >= "workspace_disk_used_bytes")),
    CONSTRAINT "workforce_worker_metric_samples_check2" CHECK ((("max_active_attempts" > 0) AND ("active_attempts" <= "max_active_attempts"))),
    CONSTRAINT "workforce_worker_metric_samples_cpu_usage_percent_check" CHECK ((("cpu_usage_percent" >= (0)::double precision) AND ("cpu_usage_percent" <= (100)::double precision))),
    CONSTRAINT "workforce_worker_metric_samples_load1_check" CHECK (("load1" >= (0)::double precision)),
    CONSTRAINT "workforce_worker_metric_samples_logical_cpu_cores_check" CHECK (("logical_cpu_cores" > 0)),
    CONSTRAINT "workforce_worker_metric_samples_memory_usage_percent_check" CHECK ((("memory_usage_percent" >= (0)::double precision) AND ("memory_usage_percent" <= (100)::double precision))),
    CONSTRAINT "workforce_worker_metric_samples_memory_used_bytes_check" CHECK (("memory_used_bytes" >= 0)),
    CONSTRAINT "workforce_worker_metric_samples_workspace_disk_used_bytes_check" CHECK (("workspace_disk_used_bytes" >= 0))
);



CREATE TABLE "public"."workforce_worker_sessions" (
    "instance_id" "text" NOT NULL,
    "worker_id" bigint NOT NULL,
    "host_boot_id" "text" NOT NULL,
    "started_at" timestamp with time zone NOT NULL,
    "registered_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "ended_at" timestamp with time zone,
    "end_reason" character varying(128),
    "last_heartbeat_at" timestamp with time zone,
    "heartbeat_sequence" integer DEFAULT 0 NOT NULL,
    "consumer_state" character varying(32) DEFAULT 'STARTING'::character varying NOT NULL,
    "docker_ready" boolean DEFAULT false NOT NULL,
    "active_attempts" integer DEFAULT 0 NOT NULL,
    "max_active_attempts" integer DEFAULT 1 NOT NULL,
    "worker_version" "text" DEFAULT ''::"text" NOT NULL,
    "worker_revision" "text" DEFAULT ''::"text" NOT NULL,
    "worker_build_date" "text" DEFAULT ''::"text" NOT NULL,
    "observed_remote_ip" "text" DEFAULT ''::"text" NOT NULL,
    "reported_ip" "text" DEFAULT ''::"text" NOT NULL,
    "hostname" "text" DEFAULT ''::"text" NOT NULL,
    "primary_interface" "text" DEFAULT ''::"text" NOT NULL,
    "primary_mac" "text" DEFAULT ''::"text" NOT NULL,
    "interfaces_json" "jsonb" DEFAULT '[]'::"jsonb" NOT NULL,
    "resource_snapshot_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "capabilities_json" "jsonb" DEFAULT '{}'::"jsonb" NOT NULL,
    "session_state" character varying(32) DEFAULT 'REGISTERED'::character varying NOT NULL,
    "reconciliation_deadline" timestamp with time zone
);



CREATE TABLE "public"."workforce_worker_tokens" (
    "id" bigint NOT NULL,
    "worker_id" bigint NOT NULL,
    "token_sha256" character varying(64) NOT NULL,
    "token_hint" "text" NOT NULL,
    "status" character varying(32) DEFAULT 'ACTIVE'::character varying NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "revoked_at" timestamp with time zone
);



CREATE TABLE "public"."workforce_workers" (
    "id" bigint NOT NULL,
    "name" "text" NOT NULL,
    "description" "text" DEFAULT ''::"text" NOT NULL,
    "desired_state" character varying(32) DEFAULT 'ENABLED'::character varying NOT NULL,
    "max_active_attempts" integer DEFAULT 1 NOT NULL,
    "max_active_reviews" integer DEFAULT 1 NOT NULL,
    "max_total_containers" integer DEFAULT 2 NOT NULL,
    "current_instance_id" "text",
    "token_hint" "text" NOT NULL,
    "inventory_refresh_generation" integer DEFAULT 1 NOT NULL,
    "decommissioned_at" timestamp with time zone,
    "version" integer DEFAULT 1 NOT NULL,
    "created_by_principal_id" bigint,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    CONSTRAINT "workforce_workers_max_active_reviews_check" CHECK ((("max_active_reviews" > 0) AND ("max_active_reviews" <= 64))),
    CONSTRAINT "workforce_workers_max_total_containers_check" CHECK ((("max_total_containers" > 0) AND ("max_total_containers" <= 128)))
);



CREATE TABLE "public"."workforce_workspace_source_revisions" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "workspace_source_id" bigint NOT NULL,
    "version" integer NOT NULL,
    "kind" character varying(32) NOT NULL,
    "project_id" bigint,
    "project_version_id" bigint,
    "repository_config_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "ref_type" character varying(32) DEFAULT ''::character varying NOT NULL,
    "ref_name" "text" DEFAULT ''::"text" NOT NULL,
    "resolved_commit" "text" DEFAULT ''::"text" NOT NULL,
    "resolved_tree" "text" DEFAULT ''::"text" NOT NULL,
    "repository_bundle_object_key" "text" DEFAULT ''::"text" NOT NULL,
    "repository_bundle_content_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "repository_bundle_object_sha256" character varying(64) DEFAULT ''::character varying NOT NULL,
    "repository_bundle_size_bytes" bigint DEFAULT 0 NOT NULL,
    "source_identity_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "status" character varying(32) NOT NULL,
    "reason_code" character varying(128) DEFAULT ''::character varying NOT NULL,
    "resolved_at" timestamp with time zone,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updated_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



CREATE TABLE "public"."workforce_workspace_sources" (
    "id" bigint NOT NULL,
    "tenant_id" bigint NOT NULL,
    "kind" character varying(32) NOT NULL,
    "project_id" bigint,
    "project_version_id" bigint,
    "repository_config_digest" character varying(64) DEFAULT ''::character varying NOT NULL,
    "display_name" "text" NOT NULL,
    "created_by_principal_id" bigint NOT NULL,
    "created_at" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);



ALTER TABLE ONLY "public"."sys_config" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_config_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_config_change_log" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_config_change_log_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_config_group" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_config_group_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_dict_item" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_dict_item_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_dict_type" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_dict_type_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_external_login_provider" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalLoginProvider_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_external_oauth_login_state" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalOAuthLoginState_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_external_oauth_token" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalOAuthToken_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_external_provider_method" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalProviderMethod_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_external_user_identity" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalUserIdentity_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_operation_log" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_operation_log_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_platform" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatform_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_platform_default_role" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformDefaultRole_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_platform_login_method" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformLoginMethod_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_platform_source_rule" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformSourceRule_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_platform_sso_client" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformSsoClient_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_post_role" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_post_role_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_audit_log" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoAuditLog_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_authorization_code" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoAuthorizationCode_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_client" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoClient_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_client_redirect_uri" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoClientRedirectUri_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_client_secret" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoClientSecret_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_consent_grant" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoConsentGrant_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_issuer_key" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoIssuerKey_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_refresh_token_family" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoRefreshTokenFamily_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_sso_session" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoSession_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_user" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_user_dept" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_dept_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_user_org" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_org_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_user_permission" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_permission_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_user_position" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_position_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."sys_user_role" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_role_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."workforce_task_events" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."workforce_task_events_id_seq"'::"regclass");



ALTER TABLE ONLY "public"."docker_compose_project"
    ADD CONSTRAINT "idx_3095026_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."docker_operation"
    ADD CONSTRAINT "idx_3095044_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."docker_operation_event"
    ADD CONSTRAINT "idx_3095061_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."docker_remote_registry"
    ADD CONSTRAINT "idx_3095072_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_config"
    ADD CONSTRAINT "idx_3095111_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_config_change_log"
    ADD CONSTRAINT "idx_3095142_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_config_group"
    ADD CONSTRAINT "idx_3095159_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_dept"
    ADD CONSTRAINT "idx_3095178_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_dict_item"
    ADD CONSTRAINT "idx_3095201_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_dict_type"
    ADD CONSTRAINT "idx_3095224_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_file_binding_task"
    ADD CONSTRAINT "idx_3095249_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_file_chunk_upload"
    ADD CONSTRAINT "idx_3095267_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_file_info"
    ADD CONSTRAINT "idx_3095286_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_file_integrity_audit"
    ADD CONSTRAINT "idx_3095302_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_file_process_run"
    ADD CONSTRAINT "idx_3095313_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_file_process_task"
    ADD CONSTRAINT "idx_3095328_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_file_reference"
    ADD CONSTRAINT "idx_3095349_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_menu"
    ADD CONSTRAINT "idx_3095370_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_menu_permission"
    ADD CONSTRAINT "idx_3095392_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_message_consume_log"
    ADD CONSTRAINT "idx_3095400_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_operation_log"
    ADD CONSTRAINT "idx_3095412_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_org"
    ADD CONSTRAINT "idx_3095430_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_outbox_event"
    ADD CONSTRAINT "idx_3095450_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_permission"
    ADD CONSTRAINT "idx_3095468_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_post"
    ADD CONSTRAINT "idx_3095486_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_post_role"
    ADD CONSTRAINT "idx_3095509_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_role"
    ADD CONSTRAINT "idx_3095516_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_role_config_scope"
    ADD CONSTRAINT "idx_3095538_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_role_dept"
    ADD CONSTRAINT "idx_3095558_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_role_menu"
    ADD CONSTRAINT "idx_3095564_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_role_permission"
    ADD CONSTRAINT "idx_3095574_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_security_bootstrap"
    ADD CONSTRAINT "idx_3095586_PRIMARY" PRIMARY KEY ("bootstrapKey");



ALTER TABLE ONLY "public"."sys_storage_alert_log"
    ADD CONSTRAINT "idx_3095597_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_storage_strategy"
    ADD CONSTRAINT "idx_3095612_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_upload_task"
    ADD CONSTRAINT "idx_3095643_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_user"
    ADD CONSTRAINT "idx_3095657_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_user_credential"
    ADD CONSTRAINT "idx_3095680_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_user_dept"
    ADD CONSTRAINT "idx_3095700_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_user_org"
    ADD CONSTRAINT "idx_3095712_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_user_permission"
    ADD CONSTRAINT "idx_3095727_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_user_position"
    ADD CONSTRAINT "idx_3095743_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_user_role"
    ADD CONSTRAINT "idx_3095758_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_external_login_provider"
    ADD CONSTRAINT "idx_3095771_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_external_managed_provider_command"
    ADD CONSTRAINT "idx_3095807_PRIMARY" PRIMARY KEY ("providerCode", "connectionVersion");



ALTER TABLE ONLY "public"."sys_external_oauth_login_state"
    ADD CONSTRAINT "idx_3095815_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_external_oauth_token"
    ADD CONSTRAINT "idx_3095836_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_external_provider_method"
    ADD CONSTRAINT "idx_3095862_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_external_user_identity"
    ADD CONSTRAINT "idx_3095881_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_federated_node"
    ADD CONSTRAINT "idx_3095903_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_federated_node_connection_command"
    ADD CONSTRAINT "idx_3095925_PRIMARY" PRIMARY KEY ("nodeCode", "connectionVersion");



ALTER TABLE ONLY "public"."sys_notification_channel"
    ADD CONSTRAINT "idx_3095937_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_delivery"
    ADD CONSTRAINT "idx_3095956_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_scene_binding"
    ADD CONSTRAINT "idx_3095980_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_template"
    ADD CONSTRAINT "idx_3096004_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_platform"
    ADD CONSTRAINT "idx_3096027_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_platform_default_role"
    ADD CONSTRAINT "idx_3096053_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_platform_login_method"
    ADD CONSTRAINT "idx_3096071_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_platform_source_rule"
    ADD CONSTRAINT "idx_3096096_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_platform_sso_client"
    ADD CONSTRAINT "idx_3096117_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_audit_log"
    ADD CONSTRAINT "idx_3096133_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_authorization_code"
    ADD CONSTRAINT "idx_3096147_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_client"
    ADD CONSTRAINT "idx_3096170_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_client_redirect_uri"
    ADD CONSTRAINT "idx_3096203_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_client_secret"
    ADD CONSTRAINT "idx_3096221_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_consent_grant"
    ADD CONSTRAINT "idx_3096239_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_issuer_key"
    ADD CONSTRAINT "idx_3096259_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_refresh_token_family"
    ADD CONSTRAINT "idx_3096277_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_sso_session"
    ADD CONSTRAINT "idx_3096301_PRIMARY" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."docker_operation_event_orphan_audit"
    ADD CONSTRAINT "pk_docker_operation_event_orphan_audit" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."docker_operation_integrity_guard"
    ADD CONSTRAINT "pk_docker_operation_integrity_guard" PRIMARY KEY ("operationId");



ALTER TABLE ONLY "public"."sys_notification_delivery_attempt"
    ADD CONSTRAINT "sysNotificationDeliveryAttempt_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_delivery_diagnostic_audit"
    ADD CONSTRAINT "sysNotificationDeliveryDiagnosticAudit_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_delivery_ephemeral_content"
    ADD CONSTRAINT "sysNotificationDeliveryEphemeralContent_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_external_target"
    ADD CONSTRAINT "sysNotificationExternalTarget_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_http_delivery_snapshot"
    ADD CONSTRAINT "sysNotificationHTTPDeliverySnapshot_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_mailbox"
    ADD CONSTRAINT "sysNotificationMailbox_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_materialization_task"
    ADD CONSTRAINT "sysNotificationMaterializationTask_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_recipient"
    ADD CONSTRAINT "sysNotificationRecipient_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_scene_definition"
    ADD CONSTRAINT "sysNotificationSceneDefinition_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_scene_revision_audit"
    ADD CONSTRAINT "sysNotificationSceneRevisionAudit_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_scene_revision"
    ADD CONSTRAINT "sysNotificationSceneRevision_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_scene_snapshot"
    ADD CONSTRAINT "sysNotificationSceneSnapshot_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_template_definition"
    ADD CONSTRAINT "sysNotificationTemplateDefinition_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_template_revision_audit"
    ADD CONSTRAINT "sysNotificationTemplateRevisionAudit_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_template_revision"
    ADD CONSTRAINT "sysNotificationTemplateRevision_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification"
    ADD CONSTRAINT "sysNotification_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_enterprise_identity_authorization"
    ADD CONSTRAINT "sys_enterprise_identity_authorization_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_enterprise_identity_connection"
    ADD CONSTRAINT "sys_enterprise_identity_connection_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_enterprise_identity_grant"
    ADD CONSTRAINT "sys_enterprise_identity_grant_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_group_route_candidate"
    ADD CONSTRAINT "sys_notification_group_route_candidate_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_group_route_candidate"
    ADD CONSTRAINT "sys_notification_group_route_candidate_token_hash_key" UNIQUE ("token_hash");



ALTER TABLE ONLY "public"."sys_notification_group_route"
    ADD CONSTRAINT "sys_notification_group_route_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_group_route_snapshot"
    ADD CONSTRAINT "sys_notification_group_route_snapshot_delivery_id_key" UNIQUE ("delivery_id");



ALTER TABLE ONLY "public"."sys_notification_group_route_snapshot"
    ADD CONSTRAINT "sys_notification_group_route_snapshot_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_notification_user_preferences"
    ADD CONSTRAINT "sys_notification_user_prefere_organization_id_user_id_event_key" UNIQUE ("organization_id", "user_id", "event_family", "channel_kind");



ALTER TABLE ONLY "public"."sys_notification_user_preferences"
    ADD CONSTRAINT "sys_notification_user_preferences_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."sys_role_grant_request"
    ADD CONSTRAINT "sys_role_grant_request_pkey" PRIMARY KEY ("roleId", "idempotencyKey");



ALTER TABLE ONLY "public"."sys_notification_delivery_ephemeral_content"
    ADD CONSTRAINT "ukNotificationDeliveryEphemeralContentDelivery" UNIQUE ("deliveryId");



ALTER TABLE ONLY "public"."sys_notification_scene_revision"
    ADD CONSTRAINT "ukNotificationSceneRevisionDefinitionNo" UNIQUE ("sceneDefinitionId", "revisionNo");



ALTER TABLE ONLY "public"."sys_notification_scene_snapshot"
    ADD CONSTRAINT "ukNotificationSceneSnapshotNotificationKind" UNIQUE ("notificationId", "receiverKind");



ALTER TABLE ONLY "public"."sys_notification_template_revision"
    ADD CONSTRAINT "ukNotificationTemplateRevisionDefinitionNo" UNIQUE ("templateDefinitionId", "revisionNo");



ALTER TABLE ONLY "public"."workforce_agent_interaction_responses"
    ADD CONSTRAINT "workforce_agent_interaction_r_tenant_id_task_id_actor_princ_key" UNIQUE ("tenant_id", "task_id", "actor_principal_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_agent_interaction_responses"
    ADD CONSTRAINT "workforce_agent_interaction_responses_interaction_id_key" UNIQUE ("interaction_id");



ALTER TABLE ONLY "public"."workforce_agent_interaction_responses"
    ADD CONSTRAINT "workforce_agent_interaction_responses_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_agent_interactions"
    ADD CONSTRAINT "workforce_agent_interactions_attempt_id_adapter_session_gen_key" UNIQUE ("attempt_id", "adapter_session_generation", "local_request_key");



ALTER TABLE ONLY "public"."workforce_agent_interactions"
    ADD CONSTRAINT "workforce_agent_interactions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_artifact_upload_attempts"
    ADD CONSTRAINT "workforce_artifact_upload_attempts_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_artifacts"
    ADD CONSTRAINT "workforce_artifacts_object_key_key" UNIQUE ("object_key");



ALTER TABLE ONLY "public"."workforce_artifacts"
    ADD CONSTRAINT "workforce_artifacts_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_attempt_facts"
    ADD CONSTRAINT "workforce_attempt_facts_pkey" PRIMARY KEY ("attempt_id", "fact_id");



ALTER TABLE ONLY "public"."workforce_attempt_runtimes"
    ADD CONSTRAINT "workforce_attempt_runtimes_assignment_id_key" UNIQUE ("assignment_id");



ALTER TABLE ONLY "public"."workforce_attempt_runtimes"
    ADD CONSTRAINT "workforce_attempt_runtimes_pkey" PRIMARY KEY ("attempt_id");



ALTER TABLE ONLY "public"."workforce_check_runs"
    ADD CONSTRAINT "workforce_check_runs_attempt_id_ordinal_key" UNIQUE ("attempt_id", "ordinal");



ALTER TABLE ONLY "public"."workforce_check_runs"
    ADD CONSTRAINT "workforce_check_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_completion_check_cycle_authorizations"
    ADD CONSTRAINT "workforce_completion_check_cycle_au_attempt_id_cycle_number_key" UNIQUE ("attempt_id", "cycle_number");



ALTER TABLE ONLY "public"."workforce_completion_check_cycle_authorizations"
    ADD CONSTRAINT "workforce_completion_check_cycle_authorizations_pkey" PRIMARY KEY ("cycle_id");



ALTER TABLE ONLY "public"."workforce_completion_check_decisions"
    ADD CONSTRAINT "workforce_completion_check_de_tenant_id_task_id_idempotency_key" UNIQUE ("tenant_id", "task_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_completion_check_decisions"
    ADD CONSTRAINT "workforce_completion_check_decision_tenant_id_checkpoint_id_key" UNIQUE ("tenant_id", "checkpoint_id");



ALTER TABLE ONLY "public"."workforce_completion_check_decisions"
    ADD CONSTRAINT "workforce_completion_check_decisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_completion_check_revisions"
    ADD CONSTRAINT "workforce_completion_check_re_tenant_id_task_id_creator_use_key" UNIQUE ("tenant_id", "task_id", "creator_user_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_completion_check_revisions"
    ADD CONSTRAINT "workforce_completion_check_re_tenant_id_task_id_revision_nu_key" UNIQUE ("tenant_id", "task_id", "revision_number");



ALTER TABLE ONLY "public"."workforce_completion_check_results"
    ADD CONSTRAINT "workforce_completion_check_results_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_completion_check_revisions"
    ADD CONSTRAINT "workforce_completion_check_revisions_applied_turn_id_key" UNIQUE ("applied_turn_id");



ALTER TABLE ONLY "public"."workforce_completion_check_revisions"
    ADD CONSTRAINT "workforce_completion_check_revisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_completion_proposals"
    ADD CONSTRAINT "workforce_completion_proposals_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_continuation_checkpoints"
    ADD CONSTRAINT "workforce_continuation_checkp_attempt_id_checkpoint_identit_key" UNIQUE ("attempt_id", "checkpoint_identity_sha256");



ALTER TABLE ONLY "public"."workforce_continuation_checkpoints"
    ADD CONSTRAINT "workforce_continuation_checkpoints_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_continuation_restore_runs"
    ADD CONSTRAINT "workforce_continuation_restore_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_delivery_attempts"
    ADD CONSTRAINT "workforce_delivery_attempts_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_delivery_attempts"
    ADD CONSTRAINT "workforce_delivery_attempts_request_id_attempt_number_key" UNIQUE ("request_id", "attempt_number");



ALTER TABLE ONLY "public"."workforce_delivery_decisions"
    ADD CONSTRAINT "workforce_delivery_decisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_delivery_decisions"
    ADD CONSTRAINT "workforce_delivery_decisions_tenant_id_principal_id_idempot_key" UNIQUE ("tenant_id", "principal_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_delivery_intents"
    ADD CONSTRAINT "workforce_delivery_intents_attempt_id_adapter_session_gener_key" UNIQUE ("attempt_id", "adapter_session_generation", "local_tool_call_key");



ALTER TABLE ONLY "public"."workforce_delivery_intents"
    ADD CONSTRAINT "workforce_delivery_intents_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_delivery_intents"
    ADD CONSTRAINT "workforce_delivery_intents_task_turn_id_key" UNIQUE ("task_turn_id");



ALTER TABLE ONLY "public"."workforce_delivery_operations"
    ADD CONSTRAINT "workforce_delivery_operations_delivery_request_id_operation_key" UNIQUE ("delivery_request_id", "operation_kind");



ALTER TABLE ONLY "public"."workforce_delivery_operations"
    ADD CONSTRAINT "workforce_delivery_operations_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_delivery_operations"
    ADD CONSTRAINT "workforce_delivery_operations_tenant_id_idempotency_key_key" UNIQUE ("tenant_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_delivery_requests"
    ADD CONSTRAINT "workforce_delivery_requests_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_delivery_requests"
    ADD CONSTRAINT "workforce_delivery_requests_task_turn_id_action_digest_key" UNIQUE ("task_turn_id", "action_digest");



ALTER TABLE ONLY "public"."workforce_delivery_requests"
    ADD CONSTRAINT "workforce_delivery_requests_tenant_id_idempotency_key_key" UNIQUE ("tenant_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_execution_nodes"
    ADD CONSTRAINT "workforce_execution_nodes_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_execution_nodes"
    ADD CONSTRAINT "workforce_execution_nodes_task_turn_id_node_key_key" UNIQUE ("task_turn_id", "node_key");



ALTER TABLE ONLY "public"."workforce_execution_operation_attempts"
    ADD CONSTRAINT "workforce_execution_operation_a_operation_id_attempt_number_key" UNIQUE ("operation_id", "attempt_number");



ALTER TABLE ONLY "public"."workforce_execution_operations"
    ADD CONSTRAINT "workforce_execution_operation_attempt_id_adapter_session_ge_key" UNIQUE ("attempt_id", "adapter_session_generation", "local_tool_call_key");



ALTER TABLE ONLY "public"."workforce_execution_operation_attempts"
    ADD CONSTRAINT "workforce_execution_operation_attempts_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_execution_operation_decisions"
    ADD CONSTRAINT "workforce_execution_operation_decisions_operation_id_key" UNIQUE ("operation_id");



ALTER TABLE ONLY "public"."workforce_execution_operation_decisions"
    ADD CONSTRAINT "workforce_execution_operation_decisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_execution_operation_decisions"
    ADD CONSTRAINT "workforce_execution_operation_tenant_id_task_id_actor_princ_key" UNIQUE ("tenant_id", "task_id", "actor_principal_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_execution_operations"
    ADD CONSTRAINT "workforce_execution_operations_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_finalization_runs"
    ADD CONSTRAINT "workforce_finalization_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_harness_channels"
    ADD CONSTRAINT "workforce_harness_channels_pkey" PRIMARY KEY ("alias");



ALTER TABLE ONLY "public"."workforce_harness_releases"
    ADD CONSTRAINT "workforce_harness_releases_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_harness_sessions"
    ADD CONSTRAINT "workforce_harness_sessions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_hook_package_releases"
    ADD CONSTRAINT "workforce_hook_package_releases_archive_object_key_key" UNIQUE ("archive_object_key");



ALTER TABLE ONLY "public"."workforce_hook_package_releases"
    ADD CONSTRAINT "workforce_hook_package_releases_package_id_version_key" UNIQUE ("package_id", "version");



ALTER TABLE ONLY "public"."workforce_hook_package_releases"
    ADD CONSTRAINT "workforce_hook_package_releases_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_hook_package_revisions"
    ADD CONSTRAINT "workforce_hook_package_revisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_hook_packages"
    ADD CONSTRAINT "workforce_hook_packages_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_hook_test_runs"
    ADD CONSTRAINT "workforce_hook_test_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_human_review_decisions"
    ADD CONSTRAINT "workforce_human_review_decisi_tenant_id_review_run_id_idemp_key" UNIQUE ("tenant_id", "review_run_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_human_review_decisions"
    ADD CONSTRAINT "workforce_human_review_decisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_image_admission_runs"
    ADD CONSTRAINT "workforce_image_admission_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_image_candidates"
    ADD CONSTRAINT "workforce_image_candidates_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_integration_api_credentials"
    ADD CONSTRAINT "workforce_integration_api_cre_integration_application_id_ve_key" UNIQUE ("integration_application_id", "version");



ALTER TABLE ONLY "public"."workforce_integration_api_credentials"
    ADD CONSTRAINT "workforce_integration_api_credentials_credential_locator_key" UNIQUE ("credential_locator");



ALTER TABLE ONLY "public"."workforce_integration_api_credentials"
    ADD CONSTRAINT "workforce_integration_api_credentials_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_integration_applications"
    ADD CONSTRAINT "workforce_integration_applications_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_integration_signing_credentials"
    ADD CONSTRAINT "workforce_integration_signing_credentials_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_integration_signing_credentials"
    ADD CONSTRAINT "workforce_integration_signing_integration_application_id_ve_key" UNIQUE ("integration_application_id", "version");



ALTER TABLE ONLY "public"."workforce_legacy_identity_map"
    ADD CONSTRAINT "workforce_legacy_identity_map_entity_type_entity_id_key" UNIQUE ("entity_type", "entity_id");



ALTER TABLE ONLY "public"."workforce_legacy_identity_map"
    ADD CONSTRAINT "workforce_legacy_identity_map_pkey" PRIMARY KEY ("entity_type", "legacy_identity");



ALTER TABLE ONLY "public"."workforce_machine_idempotency_records"
    ADD CONSTRAINT "workforce_machine_idempotency_records_pkey" PRIMARY KEY ("worker_id", "http_method", "normalized_route", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_model_credentials"
    ADD CONSTRAINT "workforce_model_credentials_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_model_invocations"
    ADD CONSTRAINT "workforce_model_invocations_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_model_invocations"
    ADD CONSTRAINT "workforce_model_invocations_protocol_key_uk" UNIQUE ("tenant_id", "protocol_invocation_key");



ALTER TABLE ONLY "public"."workforce_model_invocations"
    ADD CONSTRAINT "workforce_model_invocations_worker_id_nonce_sha256_key" UNIQUE ("worker_id", "nonce_sha256");



ALTER TABLE ONLY "public"."workforce_outbox_events"
    ADD CONSTRAINT "workforce_outbox_events_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_principals"
    ADD CONSTRAINT "workforce_principals_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_project_versions"
    ADD CONSTRAINT "workforce_project_versions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_project_versions"
    ADD CONSTRAINT "workforce_project_versions_tenant_id_project_id_id_key" UNIQUE ("tenant_id", "project_id", "id");



ALTER TABLE ONLY "public"."workforce_projects"
    ADD CONSTRAINT "workforce_projects_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_projects"
    ADD CONSTRAINT "workforce_projects_tenant_id_created_by_principal_id_create_key" UNIQUE ("tenant_id", "created_by_principal_id", "create_idempotency_key");



ALTER TABLE ONLY "public"."workforce_projects"
    ADD CONSTRAINT "workforce_projects_tenant_id_project_key_key" UNIQUE ("tenant_id", "project_key");



ALTER TABLE ONLY "public"."workforce_public_events"
    ADD CONSTRAINT "workforce_public_events_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_public_events"
    ADD CONSTRAINT "workforce_public_events_source_task_event_id_key" UNIQUE ("source_task_event_id");



ALTER TABLE ONLY "public"."workforce_pull_request_publications"
    ADD CONSTRAINT "workforce_pull_request_public_project_id_head_branch_base_b_key" UNIQUE ("project_id", "head_branch", "base_branch");



ALTER TABLE ONLY "public"."workforce_pull_request_publications"
    ADD CONSTRAINT "workforce_pull_request_publications_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_reference_assets"
    ADD CONSTRAINT "workforce_reference_assets_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_reference_assets"
    ADD CONSTRAINT "workforce_reference_assets_tenant_id_content_sha256_display_key" UNIQUE ("tenant_id", "content_sha256", "display_name");



ALTER TABLE ONLY "public"."workforce_reference_assets"
    ADD CONSTRAINT "workforce_reference_assets_tenant_id_idempotency_key_key" UNIQUE ("tenant_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_result_evaluation_cycles"
    ADD CONSTRAINT "workforce_result_evaluation_cycles_attempt_id_cycle_number_key" UNIQUE ("attempt_id", "cycle_number");



ALTER TABLE ONLY "public"."workforce_result_evaluation_cycles"
    ADD CONSTRAINT "workforce_result_evaluation_cycles_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_materialization_runs"
    ADD CONSTRAINT "workforce_result_materialization_ru_task_id_idempotency_key_key" UNIQUE ("task_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_result_materialization_runs"
    ADD CONSTRAINT "workforce_result_materialization_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_outputs"
    ADD CONSTRAINT "workforce_result_outputs_evaluation_cycle_id_name_key" UNIQUE ("evaluation_cycle_id", "name");



ALTER TABLE ONLY "public"."workforce_result_outputs"
    ADD CONSTRAINT "workforce_result_outputs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_processor_bindings"
    ADD CONSTRAINT "workforce_result_processor_bindings_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_processor_bindings"
    ADD CONSTRAINT "workforce_result_processor_bindings_tenant_id_binding_name_key" UNIQUE ("tenant_id", "binding_name");



ALTER TABLE ONLY "public"."workforce_result_processor_environment_bindings"
    ADD CONSTRAINT "workforce_result_processor_en_tenant_id_processor_release_i_key" UNIQUE ("tenant_id", "processor_release_id", "name");



ALTER TABLE ONLY "public"."workforce_result_processor_environment_bindings"
    ADD CONSTRAINT "workforce_result_processor_environment_bindings_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_processor_fixture_runs"
    ADD CONSTRAINT "workforce_result_processor_fixture_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_processor_releases"
    ADD CONSTRAINT "workforce_result_processor_releases_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_processor_revisions"
    ADD CONSTRAINT "workforce_result_processor_rev_processor_id_revision_number_key" UNIQUE ("processor_id", "revision_number");



ALTER TABLE ONLY "public"."workforce_result_processor_revisions"
    ADD CONSTRAINT "workforce_result_processor_revisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_result_processors"
    ADD CONSTRAINT "workforce_result_processors_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_review_run_attempts"
    ADD CONSTRAINT "workforce_review_run_attempts_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_review_run_attempts"
    ADD CONSTRAINT "workforce_review_run_attempts_review_run_id_attempt_number_key" UNIQUE ("review_run_id", "attempt_number");



ALTER TABLE ONLY "public"."workforce_review_runs"
    ADD CONSTRAINT "workforce_review_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_review_runs"
    ADD CONSTRAINT "workforce_review_runs_tenant_id_task_id_task_turn_id_final_key1" UNIQUE ("tenant_id", "task_id", "task_turn_id", "finalization_run_id", "result_commit", "result_tree");



ALTER TABLE ONLY "public"."workforce_review_runs"
    ADD CONSTRAINT "workforce_review_runs_tenant_id_task_id_task_turn_id_finali_key" UNIQUE ("tenant_id", "task_id", "task_turn_id", "finalization_run_id");



ALTER TABLE ONLY "public"."workforce_runtime_reconciliation_runs"
    ADD CONSTRAINT "workforce_runtime_reconciliation_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_source_ingestion_runs"
    ADD CONSTRAINT "workforce_source_ingestion_runs_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_source_ingestion_runs"
    ADD CONSTRAINT "workforce_source_ingestion_runs_tenant_id_idempotency_key_key" UNIQUE ("tenant_id", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_task_assignments"
    ADD CONSTRAINT "workforce_task_assignments_message_id_key" UNIQUE ("message_id");



ALTER TABLE ONLY "public"."workforce_task_assignments"
    ADD CONSTRAINT "workforce_task_assignments_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_attempts"
    ADD CONSTRAINT "workforce_task_attempts_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_attempts"
    ADD CONSTRAINT "workforce_task_attempts_task_id_attempt_no_key" UNIQUE ("task_id", "attempt_no");



ALTER TABLE ONLY "public"."workforce_task_conversation_items"
    ADD CONSTRAINT "workforce_task_conversation_i_tenant_id_task_id_source_kind_key" UNIQUE ("tenant_id", "task_id", "source_kind", "source_key");



ALTER TABLE ONLY "public"."workforce_task_conversation_items"
    ADD CONSTRAINT "workforce_task_conversation_items_interaction_id_key" UNIQUE ("interaction_id");



ALTER TABLE ONLY "public"."workforce_task_conversation_items"
    ADD CONSTRAINT "workforce_task_conversation_items_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_conversation_items"
    ADD CONSTRAINT "workforce_task_conversation_items_task_id_change_sequence_key" UNIQUE ("task_id", "change_sequence");



ALTER TABLE ONLY "public"."workforce_task_conversation_read_states"
    ADD CONSTRAINT "workforce_task_conversation_r_tenant_id_task_id_actor_princ_key" UNIQUE ("tenant_id", "task_id", "actor_principal_id");



ALTER TABLE ONLY "public"."workforce_task_conversation_read_states"
    ADD CONSTRAINT "workforce_task_conversation_read_states_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_events"
    ADD CONSTRAINT "workforce_task_events_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_events"
    ADD CONSTRAINT "workforce_task_events_task_id_attempt_id_sequence_key" UNIQUE NULLS NOT DISTINCT ("task_id", "attempt_id", "sequence");



ALTER TABLE ONLY "public"."workforce_task_git_plans"
    ADD CONSTRAINT "workforce_task_git_plans_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_git_plans"
    ADD CONSTRAINT "workforce_task_git_plans_task_id_task_turn_id_key" UNIQUE ("task_id", "task_turn_id");



ALTER TABLE ONLY "public"."workforce_task_input_references"
    ADD CONSTRAINT "workforce_task_input_references_pkey" PRIMARY KEY ("task_input_snapshot_id", "reference_asset_id");



ALTER TABLE ONLY "public"."workforce_task_input_snapshots"
    ADD CONSTRAINT "workforce_task_input_snapshots_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_input_snapshots"
    ADD CONSTRAINT "workforce_task_input_snapshots_task_id_task_turn_id_key" UNIQUE ("task_id", "task_turn_id");



ALTER TABLE ONLY "public"."workforce_task_network_grants"
    ADD CONSTRAINT "workforce_task_network_grants_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_network_grants"
    ADD CONSTRAINT "workforce_task_network_grants_source_decision_id_key" UNIQUE ("source_decision_id");



ALTER TABLE ONLY "public"."workforce_task_notification_record_channels"
    ADD CONSTRAINT "workforce_task_notification_r_record_id_channel_kind_group__key" UNIQUE ("record_id", "channel_kind", "group_route_id");



ALTER TABLE ONLY "public"."workforce_task_notification_record_channels"
    ADD CONSTRAINT "workforce_task_notification_record_channels_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_notification_record_recipients"
    ADD CONSTRAINT "workforce_task_notification_record_recipi_record_id_user_id_key" UNIQUE ("record_id", "user_id");



ALTER TABLE ONLY "public"."workforce_task_notification_record_recipients"
    ADD CONSTRAINT "workforce_task_notification_record_recipients_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_notification_records"
    ADD CONSTRAINT "workforce_task_notification_records_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_notification_subscription_channels"
    ADD CONSTRAINT "workforce_task_notification_s_subscription_id_channel_kind__key" UNIQUE ("subscription_id", "channel_kind", "group_route_id");



ALTER TABLE ONLY "public"."workforce_task_notification_subscriptions"
    ADD CONSTRAINT "workforce_task_notification_s_tenant_id_task_id_subscriber__key" UNIQUE ("tenant_id", "task_id", "subscriber_user_id");



ALTER TABLE ONLY "public"."workforce_task_notification_subscription_channels"
    ADD CONSTRAINT "workforce_task_notification_subscription_channels_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_notification_subscriptions"
    ADD CONSTRAINT "workforce_task_notification_subscriptions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_turns"
    ADD CONSTRAINT "workforce_task_turns_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_task_turns"
    ADD CONSTRAINT "workforce_task_turns_task_id_turn_number_key" UNIQUE ("task_id", "turn_number");



ALTER TABLE ONLY "public"."workforce_tasks"
    ADD CONSTRAINT "workforce_tasks_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_tasks"
    ADD CONSTRAINT "workforce_tasks_tenant_id_created_by_idempotency_key_key" UNIQUE ("tenant_id", "created_by", "idempotency_key");



ALTER TABLE ONLY "public"."workforce_webhook_deliveries"
    ADD CONSTRAINT "workforce_webhook_deliveries_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_webhook_deliveries"
    ADD CONSTRAINT "workforce_webhook_deliveries_public_event_id_key" UNIQUE ("public_event_id");



ALTER TABLE ONLY "public"."workforce_webhook_delivery_attempts"
    ADD CONSTRAINT "workforce_webhook_delivery_attem_delivery_id_attempt_number_key" UNIQUE ("delivery_id", "attempt_number");



ALTER TABLE ONLY "public"."workforce_webhook_delivery_attempts"
    ADD CONSTRAINT "workforce_webhook_delivery_attempts_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_worker_images"
    ADD CONSTRAINT "workforce_worker_images_pkey" PRIMARY KEY ("worker_id", "image_digest", "role");



ALTER TABLE ONLY "public"."workforce_worker_metric_samples"
    ADD CONSTRAINT "workforce_worker_metric_samples_pkey" PRIMARY KEY ("worker_id", "sampled_at");



ALTER TABLE ONLY "public"."workforce_worker_sessions"
    ADD CONSTRAINT "workforce_worker_sessions_pkey" PRIMARY KEY ("instance_id");



ALTER TABLE ONLY "public"."workforce_worker_tokens"
    ADD CONSTRAINT "workforce_worker_tokens_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_worker_tokens"
    ADD CONSTRAINT "workforce_worker_tokens_token_sha256_key" UNIQUE ("token_sha256");



ALTER TABLE ONLY "public"."workforce_workers"
    ADD CONSTRAINT "workforce_workers_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_workspace_source_revisions"
    ADD CONSTRAINT "workforce_workspace_source_revi_workspace_source_id_version_key" UNIQUE ("workspace_source_id", "version");



ALTER TABLE ONLY "public"."workforce_workspace_source_revisions"
    ADD CONSTRAINT "workforce_workspace_source_revisions_pkey" PRIMARY KEY ("id");



ALTER TABLE ONLY "public"."workforce_workspace_sources"
    ADD CONSTRAINT "workforce_workspace_sources_pkey" PRIMARY KEY ("id");



CREATE INDEX "idxNotificationChannelOrganization" ON "public"."sys_notification_channel" USING "btree" ("organizationId", "channelType", "status") WHERE ("isDeleted" = 0);



CREATE INDEX "idxNotificationDeliveryAttemptDelivery" ON "public"."sys_notification_delivery_attempt" USING "btree" ("deliveryId", "createTime");



CREATE INDEX "idxNotificationDeliveryDiagnosticAuditActorTime" ON "public"."sys_notification_delivery_diagnostic_audit" USING "btree" ("actorId", "createTime");



CREATE INDEX "idxNotificationDeliveryDiagnosticAuditDeliveryTime" ON "public"."sys_notification_delivery_diagnostic_audit" USING "btree" ("deliveryId", "createTime");



CREATE INDEX "idxNotificationDeliveryDiagnosticAuditScopeTime" ON "public"."sys_notification_delivery_diagnostic_audit" USING "btree" ("scopeId", "createTime");



CREATE INDEX "idxNotificationDeliveryEphemeralContentScopeExpiry" ON "public"."sys_notification_delivery_ephemeral_content" USING "btree" ("scopeId", "expiresAt");



CREATE INDEX "idxNotificationDeliveryExternalTarget" ON "public"."sys_notification_delivery" USING "btree" ("externalTargetId", "createTime");



CREATE INDEX "idxNotificationDeliveryNotification" ON "public"."sys_notification_delivery" USING "btree" ("notificationId", "createTime");



CREATE INDEX "idxNotificationDeliverySceneSnapshot" ON "public"."sys_notification_delivery" USING "btree" ("sceneSnapshotId");



CREATE INDEX "idxNotificationExternalTargetAuthority" ON "public"."sys_notification_external_target" USING "btree" ("organizationId", "externalIdentityId", "enterpriseConnectionId", "createTime");



CREATE INDEX "idxNotificationExternalTargetNotification" ON "public"."sys_notification_external_target" USING "btree" ("notificationId", "createTime");



CREATE INDEX "idxNotificationExternalTargetScopeConnection" ON "public"."sys_notification_external_target" USING "btree" ("scopeId", "connectionRef", "createTime");



CREATE INDEX "idxNotificationHTTPDeliverySnapshotScopeChannel" ON "public"."sys_notification_http_delivery_snapshot" USING "btree" ("scopeId", "channelCode", "createTime");



CREATE INDEX "idxNotificationMaterializationReady" ON "public"."sys_notification_materialization_task" USING "btree" ("status", "nextRunAt", "leaseUntil", "createTime");



CREATE INDEX "idxNotificationMaterializationScopeReady" ON "public"."sys_notification_materialization_task" USING "btree" ("scopeId", "status", "nextRunAt", "leaseUntil", "createTime");



CREATE INDEX "idxNotificationRecipientExpiry" ON "public"."sys_notification_recipient" USING "btree" ("scopeId", "expiredAt", "expiresAt", "id");



CREATE INDEX "idxNotificationRecipientInboxPage" ON "public"."sys_notification_recipient" USING "btree" ("scopeId", "userId", "archivedAt", "createTime", "id");



CREATE INDEX "idxNotificationRecipientMailboxVersion" ON "public"."sys_notification_recipient" USING "btree" ("scopeId", "userId", "mailboxVersion");



CREATE INDEX "idxNotificationRecipientUnread" ON "public"."sys_notification_recipient" USING "btree" ("scopeId", "userId", "archivedAt", "readAt", "expiresAt");



CREATE INDEX "idxNotificationSceneDefinitionScopeUpdate" ON "public"."sys_notification_scene_definition" USING "btree" ("scopeId", "updateTime");



CREATE INDEX "idxNotificationSceneRevisionAuditDefinition" ON "public"."sys_notification_scene_revision_audit" USING "btree" ("sceneDefinitionId", "createTime");



CREATE INDEX "idxNotificationSceneRevisionAuditScope" ON "public"."sys_notification_scene_revision_audit" USING "btree" ("scopeId", "createTime");



CREATE INDEX "idxNotificationSceneRevisionDefinitionState" ON "public"."sys_notification_scene_revision" USING "btree" ("sceneDefinitionId", "state", "revisionNo");



CREATE INDEX "idxNotificationSceneRevisionTemplate" ON "public"."sys_notification_scene_revision" USING "btree" ("templateRevisionId");



CREATE INDEX "idxNotificationSceneSnapshotSceneRevision" ON "public"."sys_notification_scene_snapshot" USING "btree" ("sceneRevisionId");



CREATE INDEX "idxNotificationSceneSnapshotScope" ON "public"."sys_notification_scene_snapshot" USING "btree" ("scopeId", "createTime");



CREATE INDEX "idxNotificationScopeStatusTime" ON "public"."sys_notification" USING "btree" ("scopeId", "status", "createTime");



CREATE INDEX "idxNotificationTemplateDefinitionScopeUpdate" ON "public"."sys_notification_template_definition" USING "btree" ("scopeId", "updateTime");



CREATE INDEX "idxNotificationTemplateRevisionAuditDefinition" ON "public"."sys_notification_template_revision_audit" USING "btree" ("templateDefinitionId", "createTime");



CREATE INDEX "idxNotificationTemplateRevisionAuditScope" ON "public"."sys_notification_template_revision_audit" USING "btree" ("scopeId", "createTime");



CREATE INDEX "idxNotificationTemplateRevisionDefinitionState" ON "public"."sys_notification_template_revision" USING "btree" ("templateDefinitionId", "state", "revisionNo");



CREATE INDEX "idxUserVerifiedEmail" ON "public"."sys_user" USING "btree" ("lower"(("userEmail")::"text"), "email_verified_at") WHERE (("isDeleted" = false) AND ("status" = 0) AND ("email_verified_at" IS NOT NULL));



CREATE INDEX "idx_3095026_idx_docker_compose_project_operation" ON "public"."docker_compose_project" USING "btree" ("lastOperationId");



CREATE INDEX "idx_3095026_idx_docker_compose_project_status" ON "public"."docker_compose_project" USING "btree" ("status", "deleted");



CREATE UNIQUE INDEX "idx_3095026_uk_docker_compose_project_id_deleted" ON "public"."docker_compose_project" USING "btree" ("projectId", "deleted");



CREATE UNIQUE INDEX "idx_3095026_uk_docker_compose_project_name_deleted" ON "public"."docker_compose_project" USING "btree" ("projectName", "deleted");



CREATE INDEX "idx_3095044_idx_docker_operation_actor" ON "public"."docker_operation" USING "btree" ("actorUserId", "updateTime");



CREATE INDEX "idx_3095044_idx_docker_operation_retry" ON "public"."docker_operation" USING "btree" ("retryOf");



CREATE INDEX "idx_3095044_idx_docker_operation_status" ON "public"."docker_operation" USING "btree" ("status", "updateTime");



CREATE INDEX "idx_3095044_idx_docker_operation_target" ON "public"."docker_operation" USING "btree" ("targetType", "targetId", "targetName", "updateTime");



CREATE INDEX "idx_3095044_idx_docker_operation_type" ON "public"."docker_operation" USING "btree" ("operationType", "updateTime");



CREATE INDEX "idx_3095061_idx_docker_operation_event_operation" ON "public"."docker_operation_event" USING "btree" ("operationId", "occurredAt");



CREATE UNIQUE INDEX "idx_3095061_uk_docker_operation_event_sequence" ON "public"."docker_operation_event" USING "btree" ("operationId", "sequence");



CREATE INDEX "idx_3095072_idx_docker_registry_default" ON "public"."docker_remote_registry" USING "btree" ("defaultRegistry", "deleted");



CREATE INDEX "idx_3095072_idx_docker_registry_status" ON "public"."docker_remote_registry" USING "btree" ("status", "deleted");



CREATE UNIQUE INDEX "idx_3095072_uk_docker_registry_code_deleted" ON "public"."docker_remote_registry" USING "btree" ("code", "deleted");



CREATE INDEX "idx_3095111_idx_configKey" ON "public"."sys_config" USING "btree" ("configKey");



CREATE INDEX "idx_3095111_idx_enabled" ON "public"."sys_config" USING "btree" ("isEnabled", "isDeleted");



CREATE INDEX "idx_3095111_idx_groupId" ON "public"."sys_config" USING "btree" ("groupId");



CREATE INDEX "idx_3095111_idx_sensitive" ON "public"."sys_config" USING "btree" ("isSensitive");



CREATE UNIQUE INDEX "idx_3095111_uk_configKey_groupId" ON "public"."sys_config" USING "btree" ("configKey", "groupId");



CREATE INDEX "idx_3095142_idx_config_id" ON "public"."sys_config_change_log" USING "btree" ("configId");



CREATE INDEX "idx_3095142_idx_config_key" ON "public"."sys_config_change_log" USING "btree" ("configKey");



CREATE INDEX "idx_3095142_idx_operation_time" ON "public"."sys_config_change_log" USING "btree" ("operationTime");



CREATE INDEX "idx_3095142_idx_operation_type" ON "public"."sys_config_change_log" USING "btree" ("operationType");



CREATE INDEX "idx_3095142_idx_operator_id" ON "public"."sys_config_change_log" USING "btree" ("operatorId");



CREATE INDEX "idx_3095142_idx_parent_log_id" ON "public"."sys_config_change_log" USING "btree" ("parentLogId");



CREATE INDEX "idx_3095142_idx_related_log_id" ON "public"."sys_config_change_log" USING "btree" ("relatedLogId");



CREATE INDEX "idx_3095142_idx_status" ON "public"."sys_config_change_log" USING "btree" ("status");



CREATE INDEX "idx_3095159_idx_module" ON "public"."sys_config_group" USING "btree" ("module");



CREATE INDEX "idx_3095159_idx_status" ON "public"."sys_config_group" USING "btree" ("status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095159_uk_groupCode" ON "public"."sys_config_group" USING "btree" ("groupCode");



CREATE INDEX "idx_3095178_idx_dept_org" ON "public"."sys_dept" USING "btree" ("orgId", "isDeleted");



CREATE INDEX "idx_3095178_idx_dept_parent" ON "public"."sys_dept" USING "btree" ("parentId", "isDeleted");



CREATE INDEX "idx_3095178_idx_dept_status" ON "public"."sys_dept" USING "btree" ("status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095178_uk_dept_code_deleted" ON "public"."sys_dept" USING "btree" ("code", "isDeleted");



CREATE INDEX "idx_3095201_idx_type_status_sort" ON "public"."sys_dict_item" USING "btree" ("dictTypeId", "status", "sortOrder");



CREATE UNIQUE INDEX "idx_3095201_uk_type_value" ON "public"."sys_dict_item" USING "btree" ("dictTypeId", "itemValue");



CREATE INDEX "idx_3095224_idx_module_status" ON "public"."sys_dict_type" USING "btree" ("module", "status");



CREATE UNIQUE INDEX "idx_3095224_uk_dictCode" ON "public"."sys_dict_type" USING "btree" ("dictCode");



CREATE INDEX "idx_3095249_idx_file_binding_status_retry" ON "public"."sys_file_binding_task" USING "btree" ("status", "nextRetryTime");



CREATE INDEX "idx_3095249_idx_file_binding_user" ON "public"."sys_file_binding_task" USING "btree" ("userId");



CREATE UNIQUE INDEX "idx_3095249_uk_file_binding_file_token" ON "public"."sys_file_binding_task" USING "btree" ("fileId", "bindingToken");



CREATE INDEX "idx_3095267_idx_chunk_expire" ON "public"."sys_file_chunk_upload" USING "btree" ("expireTime");



CREATE INDEX "idx_3095267_idx_chunk_user_status" ON "public"."sys_file_chunk_upload" USING "btree" ("userId", "status", "expireTime");



CREATE UNIQUE INDEX "idx_3095267_uk_chunk_upload_id" ON "public"."sys_file_chunk_upload" USING "btree" ("uploadId");



CREATE INDEX "idx_3095286_idx_createTime" ON "public"."sys_file_info" USING "btree" ("createTime");



CREATE INDEX "idx_3095286_idx_fileInnerName" ON "public"."sys_file_info" USING "btree" ("fileInnerName");



CREATE INDEX "idx_3095286_idx_sha256" ON "public"."sys_file_info" USING "btree" ("fileSha256");



CREATE INDEX "idx_3095286_idx_storage_strategy" ON "public"."sys_file_info" USING "btree" ("storageStrategyId");



CREATE UNIQUE INDEX "idx_3095286_uk_sha256_size" ON "public"."sys_file_info" USING "btree" ("fileSha256", "fileSize");



CREATE INDEX "idx_3095302_idx_integrity_file" ON "public"."sys_file_integrity_audit" USING "btree" ("fileId");



CREATE INDEX "idx_3095302_idx_integrity_status" ON "public"."sys_file_integrity_audit" USING "btree" ("status", "auditTime");



CREATE INDEX "idx_3095313_idx_run_file" ON "public"."sys_file_process_run" USING "btree" ("fileId");



CREATE INDEX "idx_3095313_idx_run_status" ON "public"."sys_file_process_run" USING "btree" ("status");



CREATE INDEX "idx_3095313_idx_run_task" ON "public"."sys_file_process_run" USING "btree" ("taskId");



CREATE INDEX "idx_3095328_idx_fileId" ON "public"."sys_file_process_task" USING "btree" ("fileId");



CREATE INDEX "idx_3095328_idx_nextRetryTime" ON "public"."sys_file_process_task" USING "btree" ("nextRetryTime");



CREATE INDEX "idx_3095328_idx_status_priority" ON "public"."sys_file_process_task" USING "btree" ("status", "priority");



CREATE INDEX "idx_3095328_idx_taskType" ON "public"."sys_file_process_task" USING "btree" ("taskType");



CREATE UNIQUE INDEX "idx_3095328_uk_process_task_idempotency" ON "public"."sys_file_process_task" USING "btree" ("idempotencyKey");



CREATE INDEX "idx_3095349_idx_fileId" ON "public"."sys_file_reference" USING "btree" ("fileId");



CREATE INDEX "idx_3095349_idx_user_business" ON "public"."sys_file_reference" USING "btree" ("userId", "bizType", "bizId");



CREATE INDEX "idx_3095370_idx_menu_parent" ON "public"."sys_menu" USING "btree" ("parentId");



CREATE INDEX "idx_3095370_idx_menu_status" ON "public"."sys_menu" USING "btree" ("status");



CREATE INDEX "idx_3095370_idx_menu_type" ON "public"."sys_menu" USING "btree" ("type");



CREATE UNIQUE INDEX "idx_3095392_uk_menu_permission" ON "public"."sys_menu_permission" USING "btree" ("menuId", "permissionId");



CREATE INDEX "idx_3095400_idx_message_consume_time" ON "public"."sys_message_consume_log" USING "btree" ("createTime");



CREATE UNIQUE INDEX "idx_3095400_uk_message_consume" ON "public"."sys_message_consume_log" USING "btree" ("messageId", "consumer");



CREATE INDEX "idx_3095412_idx_operation_log_deleted_time" ON "public"."sys_operation_log" USING "btree" ("isDeleted", "operationTime");



CREATE INDEX "idx_3095412_idx_operation_log_method" ON "public"."sys_operation_log" USING "btree" ("requestMethod");



CREATE INDEX "idx_3095412_idx_operation_log_trace_id" ON "public"."sys_operation_log" USING "btree" ("traceId");



CREATE INDEX "idx_3095412_idx_operation_log_type_time" ON "public"."sys_operation_log" USING "btree" ("operationType", "operationTime");



CREATE INDEX "idx_3095412_idx_operation_log_url" ON "public"."sys_operation_log" USING "btree" ("requestUrl");



CREATE INDEX "idx_3095412_idx_operation_log_user_time" ON "public"."sys_operation_log" USING "btree" ("userId", "operationTime");



CREATE INDEX "idx_3095430_idx_org_parent" ON "public"."sys_org" USING "btree" ("parentId", "isDeleted");



CREATE INDEX "idx_3095430_idx_org_status" ON "public"."sys_org" USING "btree" ("status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095430_uk_org_code" ON "public"."sys_org" USING "btree" ("code");



CREATE INDEX "idx_3095450_idx_outbox_aggregate" ON "public"."sys_outbox_event" USING "btree" ("aggregateType", "aggregateId");



CREATE INDEX "idx_3095450_idx_outbox_status_retry" ON "public"."sys_outbox_event" USING "btree" ("status", "nextRetryAt");



CREATE UNIQUE INDEX "idx_3095450_uk_outbox_event_id" ON "public"."sys_outbox_event" USING "btree" ("eventId");



CREATE INDEX "idx_3095468_idx_permission_status" ON "public"."sys_permission" USING "btree" ("status");



CREATE UNIQUE INDEX "idx_3095468_uk_permission_code" ON "public"."sys_permission" USING "btree" ("code");



CREATE INDEX "idx_3095486_idx_post_dept" ON "public"."sys_post" USING "btree" ("deptId", "isDeleted");



CREATE INDEX "idx_3095486_idx_post_org" ON "public"."sys_post" USING "btree" ("orgId", "isDeleted");



CREATE INDEX "idx_3095486_idx_post_status" ON "public"."sys_post" USING "btree" ("status", "isDeleted");



CREATE INDEX "idx_3095509_idx_post_role_post" ON "public"."sys_post_role" USING "btree" ("postId");



CREATE INDEX "idx_3095509_idx_post_role_role" ON "public"."sys_post_role" USING "btree" ("roleId");



CREATE UNIQUE INDEX "idx_3095509_uk_post_role" ON "public"."sys_post_role" USING "btree" ("postId", "roleId");



CREATE INDEX "idx_3095516_idx_role_code" ON "public"."sys_role" USING "btree" ("code");



CREATE INDEX "idx_3095516_idx_role_status" ON "public"."sys_role" USING "btree" ("status");



CREATE UNIQUE INDEX "idx_3095516_uk_sys_role_system_key" ON "public"."sys_role" USING "btree" ("systemKey");



CREATE INDEX "idx_3095538_idx_role_config_scope_group" ON "public"."sys_role_config_scope" USING "btree" ("groupCode", "configKey", "isDeleted");



CREATE INDEX "idx_3095538_idx_role_config_scope_role" ON "public"."sys_role_config_scope" USING "btree" ("roleId", "isDeleted");



CREATE UNIQUE INDEX "idx_3095538_uk_role_config_scope" ON "public"."sys_role_config_scope" USING "btree" ("roleId", "groupCode", "configKey");



CREATE UNIQUE INDEX "idx_3095558_uk_role_dept" ON "public"."sys_role_dept" USING "btree" ("roleId", "deptId");



CREATE INDEX "idx_3095564_idx_role_menu_menu" ON "public"."sys_role_menu" USING "btree" ("menuId");



CREATE INDEX "idx_3095564_idx_role_menu_role" ON "public"."sys_role_menu" USING "btree" ("roleId");



CREATE UNIQUE INDEX "idx_3095564_uk_role_menu" ON "public"."sys_role_menu" USING "btree" ("roleId", "menuId");



CREATE INDEX "idx_3095574_idx_role_permission_permission" ON "public"."sys_role_permission" USING "btree" ("permissionId");



CREATE INDEX "idx_3095574_idx_role_permission_role" ON "public"."sys_role_permission" USING "btree" ("roleId");



CREATE UNIQUE INDEX "idx_3095574_uk_role_permission" ON "public"."sys_role_permission" USING "btree" ("roleId", "permissionId");



CREATE UNIQUE INDEX "idx_3095586_uk_sys_security_bootstrap_root_role" ON "public"."sys_security_bootstrap" USING "btree" ("rootRoleId");



CREATE INDEX "idx_3095597_idx_storage_alert_strategy" ON "public"."sys_storage_alert_log" USING "btree" ("strategyId", "status");



CREATE INDEX "idx_3095597_idx_storage_alert_type" ON "public"."sys_storage_alert_log" USING "btree" ("alertType", "createTime");



CREATE INDEX "idx_3095612_idx_healthStatus" ON "public"."sys_storage_strategy" USING "btree" ("healthStatus", "priority");



CREATE INDEX "idx_3095612_idx_isDefault" ON "public"."sys_storage_strategy" USING "btree" ("isDefault");



CREATE INDEX "idx_3095612_idx_providerType" ON "public"."sys_storage_strategy" USING "btree" ("providerType");



CREATE INDEX "idx_3095612_idx_runState" ON "public"."sys_storage_strategy" USING "btree" ("runState");



CREATE UNIQUE INDEX "idx_3095612_uk_strategyName" ON "public"."sys_storage_strategy" USING "btree" ("strategyName");



CREATE INDEX "idx_3095643_idx_expire" ON "public"."sys_upload_task" USING "btree" ("expireAt");



CREATE INDEX "idx_3095643_idx_file" ON "public"."sys_upload_task" USING "btree" ("fileId");



CREATE INDEX "idx_3095643_idx_status" ON "public"."sys_upload_task" USING "btree" ("status");



CREATE INDEX "idx_3095643_idx_user" ON "public"."sys_upload_task" USING "btree" ("userId");



CREATE INDEX "idx_3095657_idx_user_email" ON "public"."sys_user" USING "btree" ("userEmail");



CREATE INDEX "idx_3095657_idx_user_phone" ON "public"."sys_user" USING "btree" ("userPhone");



CREATE INDEX "idx_3095657_idx_user_status" ON "public"."sys_user" USING "btree" ("status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095657_uk_user_account_deleted" ON "public"."sys_user" USING "btree" ("userAccount", "isDeleted");



CREATE INDEX "idx_3095680_idx_user_credential_status" ON "public"."sys_user_credential" USING "btree" ("status", "isDeleted");



CREATE INDEX "idx_3095680_idx_user_credential_user_type_status" ON "public"."sys_user_credential" USING "btree" ("userId", "credentialType", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095680_uk_credential_type_key" ON "public"."sys_user_credential" USING "btree" ("credentialType", "credentialKey", "isDeleted");



CREATE UNIQUE INDEX "idx_3095680_uk_user_credential_scope" ON "public"."sys_user_credential" USING "btree" ("userId", "credentialType", "credentialKey", "isDeleted");



CREATE INDEX "idx_3095700_idx_user_dept_dept" ON "public"."sys_user_dept" USING "btree" ("deptId");



CREATE INDEX "idx_3095700_idx_user_dept_user" ON "public"."sys_user_dept" USING "btree" ("userId");



CREATE UNIQUE INDEX "idx_3095700_uk_user_dept" ON "public"."sys_user_dept" USING "btree" ("userId", "deptId");



CREATE INDEX "idx_3095712_idx_user_org_org" ON "public"."sys_user_org" USING "btree" ("orgId", "isDeleted");



CREATE INDEX "idx_3095712_idx_user_org_user" ON "public"."sys_user_org" USING "btree" ("userId", "isDeleted");



CREATE UNIQUE INDEX "idx_3095712_uk_user_org" ON "public"."sys_user_org" USING "btree" ("userId", "orgId");



CREATE UNIQUE INDEX "idx_3095727_uk_user_permission" ON "public"."sys_user_permission" USING "btree" ("userId", "permissionId");



CREATE INDEX "idx_3095743_idx_user_position_post" ON "public"."sys_user_position" USING "btree" ("postId", "isDeleted");



CREATE INDEX "idx_3095743_idx_user_position_user" ON "public"."sys_user_position" USING "btree" ("userId", "isDeleted");



CREATE UNIQUE INDEX "idx_3095743_uk_user_post" ON "public"."sys_user_position" USING "btree" ("userId", "postId");



CREATE INDEX "idx_3095758_idx_user_role_role" ON "public"."sys_user_role" USING "btree" ("roleId", "isDeleted");



CREATE INDEX "idx_3095758_idx_user_role_user" ON "public"."sys_user_role" USING "btree" ("userId", "isDeleted");



CREATE UNIQUE INDEX "idx_3095758_uk_user_role" ON "public"."sys_user_role" USING "btree" ("userId", "roleId");



CREATE INDEX "idx_3095771_idx_sysExternalLoginProvider_display_login_status_d" ON "public"."sys_external_login_provider" USING "btree" ("displayEnabled", "loginEnabled", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095771_uk_sysExternalLoginProvider_code_deleted" ON "public"."sys_external_login_provider" USING "btree" ("providerCode", "isDeleted");



CREATE INDEX "idx_3095815_idxSysExternalOAuthLoginStateProvisioningAuthorityI" ON "public"."sys_external_oauth_login_state" USING "btree" ("provisioningAuthorityId");



CREATE INDEX "idx_3095815_idx_sysExternalOAuthLoginState_platform_status_dele" ON "public"."sys_external_oauth_login_state" USING "btree" ("platformCode", "status", "isDeleted");



CREATE INDEX "idx_3095815_idx_sysExternalOAuthLoginState_provider_status_expi" ON "public"."sys_external_oauth_login_state" USING "btree" ("providerCode", "status", "expiresAt", "isDeleted");



CREATE INDEX "idx_3095815_idx_sysExternalOAuthLoginState_stateHash_status_del" ON "public"."sys_external_oauth_login_state" USING "btree" ("stateHash", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095815_uk_sysExternalOAuthLoginState_stateId_deleted" ON "public"."sys_external_oauth_login_state" USING "btree" ("stateId", "isDeleted");



CREATE INDEX "idx_3095836_idx_sysExternalOAuthToken_provider_status_deleted" ON "public"."sys_external_oauth_token" USING "btree" ("providerCode", "status", "isDeleted");



CREATE INDEX "idx_3095836_idx_sysExternalOAuthToken_user_status_deleted" ON "public"."sys_external_oauth_token" USING "btree" ("userId", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095836_uk_sysExternalOAuthToken_identity_purpose_scope_del" ON "public"."sys_external_oauth_token" USING "btree" ("identityId", "tokenPurpose", "scopeHash", "isDeleted");



CREATE INDEX "idx_3095862_idx_sysExternalProviderMethod_capability_status_del" ON "public"."sys_external_provider_method" USING "btree" ("capabilityCode", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095862_uk_sysExternalProviderMethod_provider_method_delete" ON "public"."sys_external_provider_method" USING "btree" ("providerCode", "methodKey", "isDeleted");



CREATE INDEX "idx_3095881_idx_sysExternalUserIdentity_provider_status_deleted" ON "public"."sys_external_user_identity" USING "btree" ("providerCode", "status", "isDeleted");



CREATE INDEX "idx_3095881_idx_sysExternalUserIdentity_user_provider_deleted" ON "public"."sys_external_user_identity" USING "btree" ("userId", "providerCode", "isDeleted");



CREATE UNIQUE INDEX "idx_3095881_uk_sysExternalUserIdentity_issuer_subject_deleted" ON "public"."sys_external_user_identity" USING "btree" ("externalIdentityDigest", "isDeleted");



CREATE UNIQUE INDEX "idx_3095881_uk_sysExternalUserIdentity_provider_subject_digest_" ON "public"."sys_external_user_identity" USING "btree" ("providerSubjectDigest", "isDeleted");



CREATE INDEX "idx_3095903_idx_sysFederatedNode_connectionStatus" ON "public"."sys_federated_node" USING "btree" ("connectionStatus");



CREATE INDEX "idx_3095903_idx_sysFederatedNode_status_updatedAt" ON "public"."sys_federated_node" USING "btree" ("status", "updatedAt");



CREATE UNIQUE INDEX "idx_3095903_uk_sysFederatedNode_nodeCode_active" ON "public"."sys_federated_node" USING "btree" ("nodeCode", "activeKey");



CREATE INDEX "idx_3095925_idx_sysFederatedNodeConnectionCommand_state_updated" ON "public"."sys_federated_node_connection_command" USING "btree" ("terminalState", "updatedAt");



CREATE INDEX "idx_3095937_idxNotificationChannelTypeStatus" ON "public"."sys_notification_channel" USING "btree" ("channelType", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3095937_ukNotificationChannelCode" ON "public"."sys_notification_channel" USING "btree" ("channelCode", "isDeleted");



CREATE INDEX "idx_3095956_idxNotificationDeliveryScene" ON "public"."sys_notification_delivery" USING "btree" ("sceneCode", "createTime");



CREATE INDEX "idx_3095956_idxNotificationDeliveryStatus" ON "public"."sys_notification_delivery" USING "btree" ("status", "nextRetryAt", "isDeleted");



CREATE UNIQUE INDEX "idx_3095956_ukNotificationDeliveryDigest" ON "public"."sys_notification_delivery" USING "btree" ("requestDigest", "isDeleted");



CREATE UNIQUE INDEX "idx_3095956_ukNotificationDeliveryId" ON "public"."sys_notification_delivery" USING "btree" ("deliveryId");



CREATE INDEX "idx_3095980_idxNotificationSceneBindingChannel" ON "public"."sys_notification_scene_binding" USING "btree" ("channelCode", "isDeleted");



CREATE INDEX "idx_3095980_idxNotificationSceneBindingScene" ON "public"."sys_notification_scene_binding" USING "btree" ("sceneCode", "enabled", "priority", "isDeleted");



CREATE INDEX "idx_3096004_idxNotificationTemplateScene" ON "public"."sys_notification_template" USING "btree" ("sceneCode", "channelType", "locale", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096004_ukNotificationTemplateCode" ON "public"."sys_notification_template" USING "btree" ("templateCode", "isDeleted");



CREATE INDEX "idx_3096027_idx_sysPlatform_default_status_deleted" ON "public"."sys_platform" USING "btree" ("isDefault", "status", "isDeleted");



CREATE INDEX "idx_3096027_idx_sysPlatform_status_deleted" ON "public"."sys_platform" USING "btree" ("status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096027_uk_sysPlatform_code_deleted" ON "public"."sys_platform" USING "btree" ("platformCode", "isDeleted");



CREATE INDEX "idx_3096053_idx_sysPlatformDefaultRole_platform_status_deleted" ON "public"."sys_platform_default_role" USING "btree" ("platformCode", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096053_uk_sysPlatformDefaultRole_platform_role_deleted" ON "public"."sys_platform_default_role" USING "btree" ("platformCode", "roleId", "isDeleted");



CREATE INDEX "idx_3096071_idx_sysPlatformLoginMethod_platform_display_login_d" ON "public"."sys_platform_login_method" USING "btree" ("platformCode", "displayEnabled", "loginEnabled", "isDeleted");



CREATE UNIQUE INDEX "idx_3096071_uk_sysPlatformLoginMethod_platform_method_provider_" ON "public"."sys_platform_login_method" USING "btree" ("platformCode", "methodType", "providerCode", "isDeleted");



CREATE INDEX "idx_3096096_idx_sysPlatformSourceRule_type_status_priority_dele" ON "public"."sys_platform_source_rule" USING "btree" ("matchType", "status", "priority", "isDeleted");



CREATE UNIQUE INDEX "idx_3096096_uk_sysPlatformSourceRule_platform_type_value_delete" ON "public"."sys_platform_source_rule" USING "btree" ("platformCode", "matchType", "matchValue", "isDeleted");



CREATE INDEX "idx_3096117_idx_sysPlatformSsoClient_client_status_deleted" ON "public"."sys_platform_sso_client" USING "btree" ("clientId", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096117_uk_sysPlatformSsoClient_platform_client_deleted" ON "public"."sys_platform_sso_client" USING "btree" ("platformCode", "clientId", "isDeleted");



CREATE INDEX "idx_3096133_idx_sysSsoAuditLog_trace_deleted" ON "public"."sys_sso_audit_log" USING "btree" ("traceId", "isDeleted");



CREATE INDEX "idx_3096147_idx_sysSsoAuthorizationCode_client_user_status" ON "public"."sys_sso_authorization_code" USING "btree" ("clientId", "userId", "status");



CREATE UNIQUE INDEX "idx_3096147_uk_sysSsoAuthorizationCode_code_deleted" ON "public"."sys_sso_authorization_code" USING "btree" ("code", "isDeleted");



CREATE INDEX "idx_3096170_idx_sysSsoClient_type_status_deleted" ON "public"."sys_sso_client" USING "btree" ("clientType", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096170_uk_sysSsoClient_clientId_deleted" ON "public"."sys_sso_client" USING "btree" ("clientId", "isDeleted");



CREATE INDEX "idx_3096203_idx_sysSsoClientRedirectUri_client_status_deleted" ON "public"."sys_sso_client_redirect_uri" USING "btree" ("clientId", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096203_uk_sysSsoClientRedirectUri_client_uri_deleted" ON "public"."sys_sso_client_redirect_uri" USING "btree" ("clientId", "redirectUri", "isDeleted");



CREATE INDEX "idx_3096221_idx_sysSsoClientSecret_client_status_deleted" ON "public"."sys_sso_client_secret" USING "btree" ("clientId", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096239_uk_sysSsoConsentGrant_user_client_deleted" ON "public"."sys_sso_consent_grant" USING "btree" ("userId", "clientId", "isDeleted");



CREATE UNIQUE INDEX "idx_3096259_uk_sysSsoIssuerKey_kid_deleted" ON "public"."sys_sso_issuer_key" USING "btree" ("kid", "isDeleted");



CREATE INDEX "idx_3096277_idx_sysSsoRefreshTokenFamily_currentTokenHash_delet" ON "public"."sys_sso_refresh_token_family" USING "btree" ("currentTokenHash", "isDeleted");



CREATE INDEX "idx_3096277_idx_sysSsoRefreshTokenFamily_user_status_deleted_cr" ON "public"."sys_sso_refresh_token_family" USING "btree" ("userId", "status", "isDeleted", "createTime");



CREATE UNIQUE INDEX "idx_3096277_uk_sysSsoRefreshTokenFamily_family_deleted" ON "public"."sys_sso_refresh_token_family" USING "btree" ("familyId", "isDeleted");



CREATE INDEX "idx_3096301_idx_sysSsoSession_external_identity_status_deleted" ON "public"."sys_sso_session" USING "btree" ("externalIdentityId", "status", "isDeleted");



CREATE INDEX "idx_3096301_idx_sysSsoSession_external_provider_status_deleted" ON "public"."sys_sso_session" USING "btree" ("externalProviderCode", "status", "isDeleted");



CREATE INDEX "idx_3096301_idx_sysSsoSession_platformCode_status_deleted" ON "public"."sys_sso_session" USING "btree" ("platformCode", "status", "isDeleted");



CREATE INDEX "idx_3096301_idx_sysSsoSession_user_status_deleted" ON "public"."sys_sso_session" USING "btree" ("userId", "status", "isDeleted");



CREATE UNIQUE INDEX "idx_3096301_uk_sysSsoSession_sessionId_deleted" ON "public"."sys_sso_session" USING "btree" ("sessionId", "isDeleted");



CREATE INDEX "idx_chunk_scope_user_status" ON "public"."sys_file_chunk_upload" USING "btree" ("scopeId", "userId", "status", "expireTime");



CREATE INDEX "idx_chunk_upload_task" ON "public"."sys_file_chunk_upload" USING "btree" ("uploadTaskId");



CREATE INDEX "idx_enterprise_authorization_active" ON "public"."sys_enterprise_identity_authorization" USING "btree" ("organization_id", "status", "expires_at");



CREATE INDEX "idx_enterprise_connection_status" ON "public"."sys_enterprise_identity_connection" USING "btree" ("organization_id", "status", "provider_type");



CREATE INDEX "idx_enterprise_identity_grant_active" ON "public"."sys_enterprise_identity_grant" USING "btree" ("organization_id", "grant_type", "status", "external_identity_id");



CREATE INDEX "idx_file_reference_config_asset_lookup" ON "public"."sys_file_reference" USING "btree" ("bizType", "bizId", "scopeId") WHERE ("isDeleted" = false);



CREATE INDEX "idx_file_reference_scope_file" ON "public"."sys_file_reference" USING "btree" ("scopeId", "fileId", "isDeleted");



CREATE INDEX "idx_message_consume_status_lease" ON "public"."sys_message_consume_log" USING "btree" ("status", "leaseUntil");



CREATE INDEX "idx_notification_scene_binding_scope_scene_enabled" ON "public"."sys_notification_scene_binding" USING "btree" ("scopeId", "sceneCode", "enabled", "priority", "isDeleted");



CREATE INDEX "idx_notification_template_scope_scene_status" ON "public"."sys_notification_template" USING "btree" ("scopeId", "sceneCode", "channelType", "locale", "status", "isDeleted");



CREATE INDEX "idx_outbox_owner_scope_type_status_retry" ON "public"."sys_outbox_event" USING "btree" ("eventOwner", "scopeId", "eventType", "status", "nextRetryAt", "leaseUntil", "createTime");



CREATE INDEX "idx_outbox_owner_type_status_retry" ON "public"."sys_outbox_event" USING "btree" ("eventOwner", "eventType", "status", "nextRetryAt", "leaseUntil", "createTime");



CREATE INDEX "idx_sys_role_grant_request_create_time" ON "public"."sys_role_grant_request" USING "btree" ("createTime");



CREATE INDEX "idx_upload_credential_authority" ON "public"."sys_upload_task" USING "btree" ("userId", "scopeId", "fileId", "status", "credentialVersion", "credentialExpireAt");



CREATE INDEX "idx_upload_credential_protection" ON "public"."sys_upload_task" USING "btree" ("fileId", "status", "protectedUntil", "revokedAt");



CREATE INDEX "idx_workforce_integration_api_credentials_application" ON "public"."workforce_integration_api_credentials" USING "btree" ("tenant_id", "integration_application_id", "status", "version" DESC);



CREATE UNIQUE INDEX "idx_workforce_integration_api_credentials_current" ON "public"."workforce_integration_api_credentials" USING "btree" ("integration_application_id") WHERE (("status")::"text" = 'CURRENT'::"text");



CREATE INDEX "idx_workforce_integration_applications_tenant" ON "public"."workforce_integration_applications" USING "btree" ("tenant_id", "created_at" DESC);



CREATE INDEX "idx_workforce_integration_signing_credentials_application" ON "public"."workforce_integration_signing_credentials" USING "btree" ("tenant_id", "integration_application_id", "status", "version" DESC);



CREATE UNIQUE INDEX "idx_workforce_integration_signing_credentials_current" ON "public"."workforce_integration_signing_credentials" USING "btree" ("integration_application_id") WHERE (("status")::"text" = 'CURRENT'::"text");



CREATE INDEX "idx_workforce_public_events_application" ON "public"."workforce_public_events" USING "btree" ("tenant_id", "integration_application_id", "created_at" DESC);



CREATE INDEX "idx_workforce_task_events_open_projection_pending" ON "public"."workforce_task_events" USING "btree" ("id") WHERE ((("open_integration_projection_status")::"text" = 'PENDING'::"text") AND (("event_type")::"text" = ANY ((ARRAY['task.succeeded'::character varying, 'task.failed'::character varying, 'task.cancelled'::character varying, 'task.timed_out'::character varying])::"text"[])));



CREATE INDEX "idx_workforce_tasks_open_origin" ON "public"."workforce_tasks" USING "btree" ("tenant_id", "integration_application_id", "created_at" DESC) WHERE ("integration_application_id" IS NOT NULL);



CREATE INDEX "idx_workforce_webhook_deliveries_application" ON "public"."workforce_webhook_deliveries" USING "btree" ("tenant_id", "integration_application_id", "updated_at" DESC);



CREATE INDEX "idx_workforce_webhook_deliveries_claim" ON "public"."workforce_webhook_deliveries" USING "btree" ("status", "next_attempt_at", "lease_expires_at");



CREATE INDEX "idx_workforce_webhook_delivery_attempts_delivery" ON "public"."workforce_webhook_delivery_attempts" USING "btree" ("delivery_id", "attempt_number" DESC);



CREATE INDEX "sys_notification_group_route_available_idx" ON "public"."sys_notification_group_route" USING "btree" ("organization_id", "status", "provider_type", "display_name");



CREATE INDEX "sys_notification_group_route_candidate_expiry_idx" ON "public"."sys_notification_group_route_candidate" USING "btree" ("organization_id", "expires_at");



CREATE UNIQUE INDEX "sys_notification_group_route_name_uidx" ON "public"."sys_notification_group_route" USING "btree" ("organization_id", "lower"("display_name")) WHERE ("status" <> 'RETIRED'::"text");



CREATE INDEX "sys_notification_group_route_snapshot_route_idx" ON "public"."sys_notification_group_route_snapshot" USING "btree" ("organization_id", "route_id", "created_at" DESC);



CREATE UNIQUE INDEX "sys_notification_group_route_target_uidx" ON "public"."sys_notification_group_route" USING "btree" ("organization_id", "provider_type", "transport_type", "target_digest") WHERE (("target_digest" <> ''::"text") AND ("status" <> 'RETIRED'::"text"));



CREATE INDEX "sys_notification_user_preferences_user_idx" ON "public"."sys_notification_user_preferences" USING "btree" ("organization_id", "user_id", "event_family");



CREATE INDEX "sys_user_verified_email_idx" ON "public"."sys_user" USING "btree" ("lower"(("userEmail")::"text"), "email_verified_at") WHERE (("isDeleted" = false) AND ("status" = 0) AND ("email_verified_at" IS NOT NULL));



CREATE UNIQUE INDEX "ukNotificationDeliveryAttemptId" ON "public"."sys_notification_delivery_attempt" USING "btree" ("attemptId");



CREATE UNIQUE INDEX "ukNotificationDeliveryAttemptNumber" ON "public"."sys_notification_delivery_attempt" USING "btree" ("deliveryId", "attemptNo");



CREATE UNIQUE INDEX "ukNotificationExternalId" ON "public"."sys_notification" USING "btree" ("notificationId");



CREATE UNIQUE INDEX "ukNotificationExternalTargetId" ON "public"."sys_notification_external_target" USING "btree" ("externalTargetId");



CREATE UNIQUE INDEX "ukNotificationExternalTargetSemantic" ON "public"."sys_notification_external_target" USING "btree" ("notificationId", "connectionRef", "identityKind", "subjectDigest");



CREATE UNIQUE INDEX "ukNotificationHTTPDeliverySnapshotDelivery" ON "public"."sys_notification_http_delivery_snapshot" USING "btree" ("deliveryId");



CREATE UNIQUE INDEX "ukNotificationIdempotency" ON "public"."sys_notification" USING "btree" ("scopeId", "eventKey", "idempotencyKey");



CREATE UNIQUE INDEX "ukNotificationMailboxKey" ON "public"."sys_notification_mailbox" USING "btree" ("mailboxKey");



CREATE UNIQUE INDEX "ukNotificationMailboxOwner" ON "public"."sys_notification_mailbox" USING "btree" ("scopeId", "userId");



CREATE UNIQUE INDEX "ukNotificationMaterializationNotification" ON "public"."sys_notification_materialization_task" USING "btree" ("notificationId");



CREATE UNIQUE INDEX "ukNotificationMaterializationTaskId" ON "public"."sys_notification_materialization_task" USING "btree" ("taskId");



CREATE UNIQUE INDEX "ukNotificationRecipientId" ON "public"."sys_notification_recipient" USING "btree" ("recipientId");



CREATE UNIQUE INDEX "ukNotificationRecipientUser" ON "public"."sys_notification_recipient" USING "btree" ("notificationId", "userId");



CREATE UNIQUE INDEX "ukNotificationSceneDefinitionScopeCodeKind" ON "public"."sys_notification_scene_definition" USING "btree" ("scopeId", "sceneCode", "receiverKind", "isDeleted");



CREATE UNIQUE INDEX "ukNotificationTemplateDefinitionScopeCode" ON "public"."sys_notification_template_definition" USING "btree" ("scopeId", "templateCode", "isDeleted");



CREATE UNIQUE INDEX "uk_docker_operation_event_diagnostic" ON "public"."docker_operation_event" USING "btree" ("diagnosticId") WHERE ("diagnosticId" IS NOT NULL);



CREATE UNIQUE INDEX "uk_enterprise_authorization_identity" ON "public"."sys_enterprise_identity_authorization" USING "btree" ("authorization_identity");



CREATE UNIQUE INDEX "uk_enterprise_authorization_state" ON "public"."sys_enterprise_identity_authorization" USING "btree" ("state_hash");



CREATE UNIQUE INDEX "uk_enterprise_connection_primary" ON "public"."sys_enterprise_identity_connection" USING "btree" ("organization_id", "provider_type") WHERE (("connection_role")::"text" = 'PRIMARY'::"text");



CREATE UNIQUE INDEX "uk_enterprise_identity_grant" ON "public"."sys_enterprise_identity_grant" USING "btree" ("organization_id", "external_identity_id", "grant_type");



CREATE UNIQUE INDEX "uk_external_identity_enterprise_subject_active" ON "public"."sys_external_user_identity" USING "btree" ("organization_id", "enterprise_connection_id", "subject_digest") WHERE ((("scope_type")::"text" = 'ORGANIZATION'::"text") AND ("status" = 0) AND ("isDeleted" = 0));



CREATE UNIQUE INDEX "uk_external_identity_enterprise_user_active" ON "public"."sys_external_user_identity" USING "btree" ("organization_id", "enterprise_connection_id", "userId") WHERE ((("scope_type")::"text" = 'ORGANIZATION'::"text") AND ("status" = 0) AND ("isDeleted" = 0));



CREATE UNIQUE INDEX "uk_external_identity_system_subject_active" ON "public"."sys_external_user_identity" USING "btree" ("providerCode", COALESCE("externalIssuer", ''::character varying), "subject_digest") WHERE ((("scope_type")::"text" = 'SYSTEM'::"text") AND ("status" = 0) AND ("isDeleted" = 0));



CREATE UNIQUE INDEX "uk_file_reference_active_config_asset" ON "public"."sys_file_reference" USING "btree" ("bizId") WHERE (("isDeleted" = false) AND (("bizType")::"text" = 'CONFIG_ASSET'::"text"));



CREATE UNIQUE INDEX "uk_file_reference_active_slot" ON "public"."sys_file_reference" USING "btree" ("userId", "scopeId", "bizType", "bizId") WHERE ("isDeleted" = false);



CREATE UNIQUE INDEX "uk_sys_dict_item_type_value" ON "public"."sys_dict_item" USING "btree" ("dictTypeId", "itemValue");



CREATE UNIQUE INDEX "uk_sys_dict_type_dictCode" ON "public"."sys_dict_type" USING "btree" ("dictCode");



CREATE UNIQUE INDEX "uk_upload_credential_id" ON "public"."sys_upload_task" USING "btree" ("credentialId") WHERE ("credentialId" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_active_assignment_node_idx" ON "public"."workforce_task_assignments" USING "btree" ("execution_node_id") WHERE (("status")::"text" = ANY ((ARRAY['RESERVED'::character varying, 'PUBLISHED'::character varying, 'CLAIMED'::character varying])::"text"[]));



CREATE UNIQUE INDEX "workforce_active_finalization_attempt_idx" ON "public"."workforce_finalization_runs" USING "btree" ("attempt_id") WHERE (("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'RUNNING'::character varying, 'RETRY_WAIT'::character varying, 'FROZEN'::character varying])::"text"[]));



CREATE UNIQUE INDEX "workforce_active_reconciliation_attempt_idx" ON "public"."workforce_runtime_reconciliation_runs" USING "btree" ("attempt_id") WHERE (("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'ADOPTING'::character varying])::"text"[]));



CREATE UNIQUE INDEX "workforce_active_runtime_node_idx" ON "public"."workforce_attempt_runtimes" USING "btree" ("execution_node_id") WHERE (("status")::"text" = ANY ((ARRAY['AUTHORIZED'::character varying, 'CONTAINER_CREATED'::character varying, 'SUPERVISOR_READY'::character varying, 'STARTING'::character varying, 'RUNNING'::character varying, 'RECOVERING'::character varying, 'REATTACHING'::character varying, 'TERMINATING'::character varying, 'RECONCILIATION_REQUIRED'::character varying])::"text"[]));



CREATE INDEX "workforce_artifacts_task_created_idx" ON "public"."workforce_artifacts" USING "btree" ("task_id", "created_at");



CREATE INDEX "workforce_assignments_capacity_idx" ON "public"."workforce_task_assignments" USING "btree" ("worker_id", "status", "claimed_at");



CREATE INDEX "workforce_assignments_selection_idx" ON "public"."workforce_task_assignments" USING "btree" ("selection_digest", "execution_node_id", "generation");



CREATE INDEX "workforce_assignments_worker_status_idx" ON "public"."workforce_task_assignments" USING "btree" ("worker_id", "status", "created_at");



CREATE UNIQUE INDEX "workforce_attempt_facts_source_sequence_uidx" ON "public"."workforce_attempt_facts" USING "btree" ("attempt_id", "producer_source", "producer_sequence");



CREATE UNIQUE INDEX "workforce_checkpoint_consumed_once_idx" ON "public"."workforce_continuation_checkpoints" USING "btree" ("id") WHERE ("consumed_by_turn_id" IS NOT NULL);



CREATE INDEX "workforce_checkpoint_expiry_idx" ON "public"."workforce_continuation_checkpoints" USING "btree" ("status", "expires_at");



CREATE UNIQUE INDEX "workforce_checkpoint_interaction_once_idx" ON "public"."workforce_continuation_checkpoints" USING "btree" ("interaction_id") WHERE ("interaction_id" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_checkpoint_operation_once_idx" ON "public"."workforce_continuation_checkpoints" USING "btree" ("operation_id") WHERE ("operation_id" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_completion_attempt_once_idx" ON "public"."workforce_completion_proposals" USING "btree" ("attempt_id");



CREATE UNIQUE INDEX "workforce_completion_check_result_occurrence" ON "public"."workforce_completion_check_results" USING "btree" ("tenant_id", "attempt_id", "phase", COALESCE("evaluation_cycle_id", (0)::bigint), "check_digest");



CREATE INDEX "workforce_completion_check_results_tenant_task_created_idx" ON "public"."workforce_completion_check_results" USING "btree" ("tenant_id", "task_id", "created_at" DESC, "id");



CREATE UNIQUE INDEX "workforce_completion_revision_pending_uq" ON "public"."workforce_completion_check_revisions" USING "btree" ("tenant_id", "task_id") WHERE (("status")::"text" = 'PENDING'::"text");



CREATE INDEX "workforce_conversation_changes_idx" ON "public"."workforce_task_conversation_items" USING "btree" ("tenant_id", "task_id", "change_sequence");



CREATE UNIQUE INDEX "workforce_delivery_active_destination_idx" ON "public"."workforce_delivery_requests" USING "btree" ("project_id", "destination_branch") WHERE (("status")::"text" = ANY ((ARRAY['READY'::character varying, 'DISPATCHED'::character varying, 'RUNNING'::character varying, 'CONFIRMING'::character varying, 'OUTCOME_UNKNOWN'::character varying])::"text"[]));



CREATE INDEX "workforce_delivery_attempt_retry_idx" ON "public"."workforce_delivery_attempts" USING "btree" ("status", "next_retry_at", "worker_id");



CREATE INDEX "workforce_delivery_decisions_request_idx" ON "public"."workforce_delivery_decisions" USING "btree" ("delivery_request_id", "id" DESC);



CREATE INDEX "workforce_delivery_intents_request_idx" ON "public"."workforce_delivery_intents" USING "btree" ("delivery_request_id") WHERE ("delivery_request_id" IS NOT NULL);



CREATE INDEX "workforce_delivery_intents_task_idx" ON "public"."workforce_delivery_intents" USING "btree" ("tenant_id", "task_id", "status", "id" DESC);



CREATE INDEX "workforce_delivery_operations_dispatch_idx" ON "public"."workforce_delivery_operations" USING "btree" ("status", "next_retry_at", "created_at");



CREATE INDEX "workforce_delivery_requests_dispatch_idx" ON "public"."workforce_delivery_requests" USING "btree" ("status", "created_at");



CREATE UNIQUE INDEX "workforce_delivery_requests_id_version_uidx" ON "public"."workforce_delivery_requests" USING "btree" ("id", "version");



CREATE INDEX "workforce_delivery_requests_task_idx" ON "public"."workforce_delivery_requests" USING "btree" ("task_id", "created_at" DESC);



CREATE INDEX "workforce_delivery_requests_tenant_project_created_idx" ON "public"."workforce_delivery_requests" USING "btree" ("tenant_id", "project_id", "created_at" DESC, "id");



CREATE UNIQUE INDEX "workforce_delivery_succeeded_attempt_idx" ON "public"."workforce_delivery_attempts" USING "btree" ("request_id") WHERE (("status")::"text" = 'SUCCEEDED'::"text");



CREATE INDEX "workforce_evaluation_cycles_attempt_idx" ON "public"."workforce_result_evaluation_cycles" USING "btree" ("attempt_id", "cycle_number");



CREATE INDEX "workforce_execution_nodes_dispatch_idx" ON "public"."workforce_execution_nodes" USING "btree" ("status", "created_at");



CREATE UNIQUE INDEX "workforce_execution_operation_one_active_attempt_idx" ON "public"."workforce_execution_operation_attempts" USING "btree" ("operation_id") WHERE (("status")::"text" = ANY ((ARRAY['DISPATCHED'::character varying, 'RUNNING'::character varying])::"text"[]));



CREATE INDEX "workforce_execution_operation_result_idx" ON "public"."workforce_execution_operations" USING "btree" ("attempt_id", "adapter_session_generation", "result_delivery_status", "id");



CREATE INDEX "workforce_execution_operation_task_idx" ON "public"."workforce_execution_operations" USING "btree" ("tenant_id", "task_id", "id" DESC);



CREATE UNIQUE INDEX "workforce_finalization_identity_idx" ON "public"."workforce_finalization_runs" USING "btree" ("attempt_id", "input_identity_digest");



CREATE INDEX "workforce_finalization_retry_idx" ON "public"."workforce_finalization_runs" USING "btree" ("status", "next_retry_at", "worker_id");



CREATE UNIQUE INDEX "workforce_harness_releases_digest_uidx" ON "public"."workforce_harness_releases" USING "btree" ("image_digest");



CREATE INDEX "workforce_hook_package_releases_tenant_published_idx" ON "public"."workforce_hook_package_releases" USING "btree" ("tenant_id", "published_at" DESC);



CREATE UNIQUE INDEX "workforce_hook_packages_system_name_uidx" ON "public"."workforce_hook_packages" USING "btree" ("name") WHERE (("scope_type")::"text" = 'SYSTEM'::"text");



CREATE UNIQUE INDEX "workforce_hook_packages_tenant_name_uidx" ON "public"."workforce_hook_packages" USING "btree" ("tenant_id", "name") WHERE (("scope_type")::"text" = 'TENANT'::"text");



CREATE INDEX "workforce_hook_packages_tenant_updated_idx" ON "public"."workforce_hook_packages" USING "btree" ("tenant_id", "updated_at" DESC);



CREATE INDEX "workforce_hook_test_runs_revision_idx" ON "public"."workforce_hook_test_runs" USING "btree" ("revision_id", "created_at" DESC);



CREATE UNIQUE INDEX "workforce_human_review_decisions_applied_run_uidx" ON "public"."workforce_human_review_decisions" USING "btree" ("review_run_id") WHERE (("status")::"text" = 'APPLIED'::"text");



CREATE INDEX "workforce_human_review_decisions_task_read_idx" ON "public"."workforce_human_review_decisions" USING "btree" ("tenant_id", "task_id", "id" DESC);



CREATE INDEX "workforce_images_presence_idx" ON "public"."workforce_worker_images" USING "btree" ("worker_id", "presence_status", "admission_status", "image_digest");



CREATE UNIQUE INDEX "workforce_interaction_message_key_idx" ON "public"."workforce_agent_interactions" USING "btree" ("tenant_id", "task_id", "actor_principal_id", "local_request_key") WHERE (("kind")::"text" = 'USER_MESSAGE'::"text");



CREATE INDEX "workforce_interaction_task_idx" ON "public"."workforce_agent_interactions" USING "btree" ("tenant_id", "task_id", "id");



CREATE INDEX "workforce_machine_idempotency_expiry_idx" ON "public"."workforce_machine_idempotency_records" USING "btree" ("expires_at");



CREATE UNIQUE INDEX "workforce_materialization_identity_output_idx" ON "public"."workforce_result_materialization_runs" USING "btree" ("task_turn_id", "kind", "input_identity_digest", "output_id") WHERE ("output_id" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_materialization_identity_platform_idx" ON "public"."workforce_result_materialization_runs" USING "btree" ("task_turn_id", "kind", "input_identity_digest") WHERE ("output_id" IS NULL);



CREATE UNIQUE INDEX "workforce_model_credentials_tenant_name_uidx" ON "public"."workforce_model_credentials" USING "btree" ("tenant_id", "name") WHERE (("status")::"text" <> 'DELETED'::"text");



CREATE INDEX "workforce_model_credentials_tenant_status_idx" ON "public"."workforce_model_credentials" USING "btree" ("tenant_id", "status", "name");



CREATE INDEX "workforce_model_invocations_active_idx" ON "public"."workforce_model_invocations" USING "btree" ("tenant_id", "attempt_id", "status") WHERE (("status")::"text" = ANY ((ARRAY['AUTHORIZED'::character varying, 'STREAMING'::character varying])::"text"[]));



CREATE INDEX "workforce_model_invocations_credential_idx" ON "public"."workforce_model_invocations" USING "btree" ("tenant_id", "model_credential_id", "started_at" DESC);



CREATE INDEX "workforce_model_invocations_review_run_active_idx" ON "public"."workforce_model_invocations" USING "btree" ("tenant_id", "review_run_id", "review_run_attempt_id", "status") WHERE ((("execution_kind")::"text" = 'REVIEW_RUN'::"text") AND (("status")::"text" = ANY ((ARRAY['AUTHORIZED'::character varying, 'STREAMING'::character varying])::"text"[])));



CREATE INDEX "workforce_model_invocations_task_idx" ON "public"."workforce_model_invocations" USING "btree" ("tenant_id", "task_id", "started_at" DESC);



CREATE INDEX "workforce_outbox_events_publish_idx" ON "public"."workforce_outbox_events" USING "btree" ("status", "next_retry_at", "created_at");



CREATE UNIQUE INDEX "workforce_principals_service_uidx" ON "public"."workforce_principals" USING "btree" ("service_code") WHERE (("principal_type")::"text" = 'SERVICE'::"text");



CREATE UNIQUE INDEX "workforce_principals_user_uidx" ON "public"."workforce_principals" USING "btree" ("user_id") WHERE (("principal_type")::"text" = 'USER'::"text");



CREATE UNIQUE INDEX "workforce_principals_worker_uidx" ON "public"."workforce_principals" USING "btree" ("worker_id") WHERE (("principal_type")::"text" = 'WORKER'::"text");



CREATE INDEX "workforce_project_versions_department_idx" ON "public"."workforce_project_versions" USING "btree" ("tenant_id", "department_id", "id" DESC);



CREATE INDEX "workforce_project_versions_project_idx" ON "public"."workforce_project_versions" USING "btree" ("tenant_id", "project_id", "id" DESC);



CREATE INDEX "workforce_project_versions_repository_idx" ON "public"."workforce_project_versions" USING "btree" ("tenant_id", "repository_host", "repository_path") WHERE (("kind")::"text" = 'CODEBASE'::"text");



CREATE INDEX "workforce_projects_tenant_status_idx" ON "public"."workforce_projects" USING "btree" ("tenant_id", "status", "id");



CREATE INDEX "workforce_pull_request_publications_task_idx" ON "public"."workforce_pull_request_publications" USING "btree" ("task_id", "created_at" DESC);



CREATE INDEX "workforce_reconciliation_deadline_idx" ON "public"."workforce_attempt_runtimes" USING "btree" ("status", "reattach_deadline");



CREATE INDEX "workforce_reference_assets_tenant_idx" ON "public"."workforce_reference_assets" USING "btree" ("tenant_id", "status", "created_at" DESC);



CREATE UNIQUE INDEX "workforce_restore_active_turn_idx" ON "public"."workforce_continuation_restore_runs" USING "btree" ("target_turn_id") WHERE (("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'RUNNING'::character varying])::"text"[]));



CREATE INDEX "workforce_result_outputs_task_idx" ON "public"."workforce_result_outputs" USING "btree" ("task_id", "created_at");



CREATE INDEX "workforce_result_processor_fixture_dispatch_idx" ON "public"."workforce_result_processor_fixture_runs" USING "btree" ("status", "harness_image_digest", "created_at");



CREATE UNIQUE INDEX "workforce_result_processor_releases_digest_uidx" ON "public"."workforce_result_processor_releases" USING "btree" ("processor_id", "archive_sha256");



CREATE INDEX "workforce_result_processor_releases_lookup_idx" ON "public"."workforce_result_processor_releases" USING "btree" ("tenant_id", "id");



CREATE UNIQUE INDEX "workforce_result_processor_releases_version_uidx" ON "public"."workforce_result_processor_releases" USING "btree" ("processor_id", "version");



CREATE UNIQUE INDEX "workforce_result_processors_system_name_uidx" ON "public"."workforce_result_processors" USING "btree" ("name") WHERE (("scope_type")::"text" = 'SYSTEM'::"text");



CREATE UNIQUE INDEX "workforce_result_processors_tenant_name_uidx" ON "public"."workforce_result_processors" USING "btree" ("tenant_id", "name") WHERE (("scope_type")::"text" = 'TENANT'::"text");



CREATE INDEX "workforce_review_run_attempts_run_owner_idx" ON "public"."workforce_review_run_attempts" USING "btree" ("review_run_id", "owner_epoch", "status");



CREATE INDEX "workforce_review_runs_scheduler_idx" ON "public"."workforce_review_runs" USING "btree" ("status", "next_retry_at", "created_at");



CREATE INDEX "workforce_review_runs_task_read_idx" ON "public"."workforce_review_runs" USING "btree" ("tenant_id", "task_id", "id" DESC);



CREATE INDEX "workforce_review_runs_task_retention_idx" ON "public"."workforce_review_runs" USING "btree" ("task_id", "status", "finalization_run_id");



CREATE INDEX "workforce_review_runs_worker_capacity_idx" ON "public"."workforce_review_runs" USING "btree" ("worker_id", "status", "lease_expires_at");



CREATE UNIQUE INDEX "workforce_running_evaluation_attempt_idx" ON "public"."workforce_result_evaluation_cycles" USING "btree" ("attempt_id") WHERE (("status")::"text" = 'RUNNING'::"text");



CREATE INDEX "workforce_runtime_capacity_idx" ON "public"."workforce_attempt_runtimes" USING "btree" ("worker_id", "status", "updated_at");



CREATE INDEX "workforce_source_ingestion_dispatch_idx" ON "public"."workforce_source_ingestion_runs" USING "btree" ("status", "next_retry_at", "created_at");



CREATE INDEX "workforce_task_attempts_lease_idx" ON "public"."workforce_task_attempts" USING "btree" ("status", "lease_expires_at");



CREATE INDEX "workforce_task_events_task_cursor_idx" ON "public"."workforce_task_events" USING "btree" ("task_id", "id");



CREATE INDEX "workforce_task_git_plans_task_idx" ON "public"."workforce_task_git_plans" USING "btree" ("task_id", "task_turn_id");



CREATE INDEX "workforce_task_input_snapshots_task_idx" ON "public"."workforce_task_input_snapshots" USING "btree" ("task_id", "task_turn_id");



CREATE INDEX "workforce_task_network_grant_task_idx" ON "public"."workforce_task_network_grants" USING "btree" ("tenant_id", "task_id", "status", "id" DESC);



CREATE UNIQUE INDEX "workforce_task_notification_record_channel_direct_uidx" ON "public"."workforce_task_notification_record_channels" USING "btree" ("record_id", "channel_kind") WHERE ("group_route_id" IS NULL);



CREATE INDEX "workforce_task_notification_record_channels_route_idx" ON "public"."workforce_task_notification_record_channels" USING "btree" ("tenant_id", "group_route_id", "record_id") WHERE ("group_route_id" IS NOT NULL);



CREATE INDEX "workforce_task_notification_record_recipients_user_idx" ON "public"."workforce_task_notification_record_recipients" USING "btree" ("tenant_id", "user_id", "created_at" DESC);



CREATE INDEX "workforce_task_notification_records_dispatch_idx" ON "public"."workforce_task_notification_records" USING "btree" ("status", "next_retry_at", "lease_until", "occurred_at") WHERE (("status")::"text" = ANY ((ARRAY['PENDING'::character varying, 'PROCESSING'::character varying, 'RETRY_WAIT'::character varying])::"text"[]));



CREATE UNIQUE INDEX "workforce_task_notification_records_fact_v2_uidx" ON "public"."workforce_task_notification_records" USING "btree" ("tenant_id", "task_id", "task_turn_id", "fact_type", "source_type", "source_id", "source_occurrence", "subscriber_user_id");



CREATE INDEX "workforce_task_notification_records_task_idx" ON "public"."workforce_task_notification_records" USING "btree" ("tenant_id", "task_id", "occurred_at" DESC, "id" DESC);



CREATE UNIQUE INDEX "workforce_task_notification_subscription_channel_direct_uidx" ON "public"."workforce_task_notification_subscription_channels" USING "btree" ("subscription_id", "channel_kind") WHERE ("group_route_id" IS NULL);



CREATE INDEX "workforce_task_notification_subscription_channels_route_idx" ON "public"."workforce_task_notification_subscription_channels" USING "btree" ("tenant_id", "group_route_id", "subscription_id") WHERE ("group_route_id" IS NOT NULL);



CREATE INDEX "workforce_task_notification_subscriptions_lookup_idx" ON "public"."workforce_task_notification_subscriptions" USING "btree" ("tenant_id", "task_id", "enabled");



CREATE UNIQUE INDEX "workforce_task_turns_active_idx" ON "public"."workforce_task_turns" USING "btree" ("task_id") WHERE (("status")::"text" = 'ACTIVE'::"text");



CREATE UNIQUE INDEX "workforce_task_turns_idempotency_idx" ON "public"."workforce_task_turns" USING "btree" ("task_id", "idempotency_key") WHERE ("idempotency_key" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_task_turns_source_review_run_uidx" ON "public"."workforce_task_turns" USING "btree" ("source_review_run_id") WHERE ("source_review_run_id" IS NOT NULL);



CREATE INDEX "workforce_task_turns_task_idx" ON "public"."workforce_task_turns" USING "btree" ("task_id", "turn_number");



CREATE INDEX "workforce_tasks_creator_created_idx" ON "public"."workforce_tasks" USING "btree" ("tenant_id", "created_by", "created_at" DESC);



CREATE INDEX "workforce_tasks_fair_dispatch_idx" ON "public"."workforce_tasks" USING "btree" ("tenant_id", "status", "priority" DESC, "created_at");



CREATE INDEX "workforce_tasks_project_created_idx" ON "public"."workforce_tasks" USING "btree" ("tenant_id", "project_id", "created_at" DESC, "id");



CREATE INDEX "workforce_tasks_status_created_idx" ON "public"."workforce_tasks" USING "btree" ("status", "priority" DESC, "created_at");



CREATE INDEX "workforce_tasks_tags_gin_idx" ON "public"."workforce_tasks" USING "gin" ("tags_json");



CREATE INDEX "workforce_tasks_tenant_created_idx" ON "public"."workforce_tasks" USING "btree" ("tenant_id", "created_at" DESC);



CREATE INDEX "workforce_tasks_tenant_project_finished_idx" ON "public"."workforce_tasks" USING "btree" ("tenant_id", "project_id", "finished_at" DESC, "id") WHERE ("finished_at" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_turn_execution_operation_once_idx" ON "public"."workforce_task_turns" USING "btree" ("source_execution_operation_id") WHERE ("source_execution_operation_id" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_turn_interaction_response_once_idx" ON "public"."workforce_task_turns" USING "btree" ("source_interaction_response_id") WHERE ("source_interaction_response_id" IS NOT NULL);



CREATE UNIQUE INDEX "workforce_turn_source_checkpoint_once_idx" ON "public"."workforce_task_turns" USING "btree" ("source_checkpoint_id") WHERE ("source_checkpoint_id" IS NOT NULL);



CREATE INDEX "workforce_upload_retry_idx" ON "public"."workforce_artifact_upload_attempts" USING "btree" ("status", "next_retry_at", "worker_id");



CREATE UNIQUE INDEX "workforce_verified_materialization_upload_idx" ON "public"."workforce_artifact_upload_attempts" USING "btree" ("materialization_run_id", "sha256") WHERE (("materialization_run_id" IS NOT NULL) AND (("status")::"text" = 'VERIFIED'::"text"));



CREATE UNIQUE INDEX "workforce_verified_upload_identity_idx" ON "public"."workforce_artifact_upload_attempts" USING "btree" ("finalization_run_id", "artifact_type", "sha256") WHERE (("status")::"text" = 'VERIFIED'::"text");



CREATE INDEX "workforce_worker_images_environment_capabilities_gin_idx" ON "public"."workforce_worker_images" USING "gin" ("environment_capabilities_json");



CREATE INDEX "workforce_worker_images_match_idx" ON "public"."workforce_worker_images" USING "btree" ("image_digest", "role", "admission_status", "worker_id");



CREATE INDEX "workforce_worker_images_routing_idx" ON "public"."workforce_worker_images" USING "btree" ("scheduling_scope", "role", "admission_status", "presence_status", "scheduling_priority", "worker_id");



CREATE INDEX "workforce_worker_metric_samples_sampled_at_brin" ON "public"."workforce_worker_metric_samples" USING "brin" ("sampled_at");



CREATE INDEX "workforce_worker_sessions_fresh_idx" ON "public"."workforce_worker_sessions" USING "btree" ("worker_id", "last_heartbeat_at" DESC);



CREATE UNIQUE INDEX "workforce_workspace_source_revisions_identity_uidx" ON "public"."workforce_workspace_source_revisions" USING "btree" ("tenant_id", "source_identity_digest") WHERE (("source_identity_digest")::"text" <> ''::"text");



CREATE INDEX "workforce_workspace_source_revisions_lookup_idx" ON "public"."workforce_workspace_source_revisions" USING "btree" ("tenant_id", "status", "created_at" DESC);



CREATE TRIGGER "workforce_project_versions_immutable_trg" BEFORE DELETE OR UPDATE ON "public"."workforce_project_versions" FOR EACH ROW EXECUTE FUNCTION "public"."workforce_project_version_immutable"();
