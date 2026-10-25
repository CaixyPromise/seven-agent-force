-- +goose Up

--
-- PostgreSQL database dump
--


-- Dumped from database version 18.0 (Homebrew)
-- Dumped by pg_dump version 18.0 (Homebrew)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

-- Register the historical PostgreSQL upgrade chain represented by this
-- snapshot. Goose creates this table before executing the migration, and the
-- surrounding migration transaction rolls these rows back if the snapshot
-- fails at any later statement.
INSERT INTO "public"."goose_db_version" ("version_id", "is_applied", "tstamp")
VALUES
    (20260528100000, true, CURRENT_TIMESTAMP),
    (20260528101000, true, CURRENT_TIMESTAMP),
    (20260528102000, true, CURRENT_TIMESTAMP),
    (20260528102500, true, CURRENT_TIMESTAMP),
    (20260528103000, true, CURRENT_TIMESTAMP),
    (20260528104000, true, CURRENT_TIMESTAMP),
    (20260528105000, true, CURRENT_TIMESTAMP),
    (20260528106000, true, CURRENT_TIMESTAMP),
    (20260611110000, true, CURRENT_TIMESTAMP),
    (20260611111000, true, CURRENT_TIMESTAMP),
    (20260611112000, true, CURRENT_TIMESTAMP),
    (20260612010000, true, CURRENT_TIMESTAMP),
    (20260612093000, true, CURRENT_TIMESTAMP),
    (20260612133000, true, CURRENT_TIMESTAMP),
    (20260612160000, true, CURRENT_TIMESTAMP),
    (20260612170000, true, CURRENT_TIMESTAMP),
    (20260612190000, true, CURRENT_TIMESTAMP),
    (20260618100000, true, CURRENT_TIMESTAMP),
    (20260618101000, true, CURRENT_TIMESTAMP),
    (20260619090000, true, CURRENT_TIMESTAMP),
    (20260718140000, true, CURRENT_TIMESTAMP),
    (20260718150000, true, CURRENT_TIMESTAMP);

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--



SET default_tablespace = '';

SET default_table_access_method = "heap";

--
-- Name: docker_compose_project; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "docker_compose_project"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."docker_compose_project" IS 'Docker Compose项目';


--
-- Name: COLUMN "docker_compose_project"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."id" IS 'Compose项目内部ID';


--
-- Name: COLUMN "docker_compose_project"."projectId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."projectId" IS 'Compose项目稳定ID';


--
-- Name: COLUMN "docker_compose_project"."projectName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."projectName" IS 'Docker Compose项目名';


--
-- Name: COLUMN "docker_compose_project"."workingDir"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."workingDir" IS '工作目录';


--
-- Name: COLUMN "docker_compose_project"."configFilesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."configFilesJson" IS '配置文件列表JSON';


--
-- Name: COLUMN "docker_compose_project"."composeYaml"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."composeYaml" IS 'Compose YAML';


--
-- Name: COLUMN "docker_compose_project"."composeFilePath"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."composeFilePath" IS '实际Compose文件路径';


--
-- Name: COLUMN "docker_compose_project"."fileManifestJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."fileManifestJson" IS '项目文件清单JSON';


--
-- Name: COLUMN "docker_compose_project"."description"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."description" IS '描述';


--
-- Name: COLUMN "docker_compose_project"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."status" IS '项目状态';


--
-- Name: COLUMN "docker_compose_project"."lastPreviewJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."lastPreviewJson" IS '最近一次策略预览JSON';


--
-- Name: COLUMN "docker_compose_project"."lastValidationJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."lastValidationJson" IS '最近一次校验结果JSON';


--
-- Name: COLUMN "docker_compose_project"."lastOperationId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."lastOperationId" IS '最近一次操作ID';


--
-- Name: COLUMN "docker_compose_project"."source"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."source" IS '来源：MANAGED/DISCOVERED';


--
-- Name: COLUMN "docker_compose_project"."createdBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."createdBy" IS '创建人';


--
-- Name: COLUMN "docker_compose_project"."deleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."deleted" IS '是否删除';


--
-- Name: COLUMN "docker_compose_project"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."createTime" IS '创建时间';


--
-- Name: COLUMN "docker_compose_project"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_compose_project"."updateTime" IS '更新时间';


--
-- Name: docker_operation; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "docker_operation"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."docker_operation" IS 'Docker异步操作';


--
-- Name: COLUMN "docker_operation"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."id" IS 'Docker操作ID';


--
-- Name: COLUMN "docker_operation"."operationType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."operationType" IS '操作类型';


--
-- Name: COLUMN "docker_operation"."targetType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."targetType" IS '目标类型';


--
-- Name: COLUMN "docker_operation"."targetId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."targetId" IS '目标稳定ID';


--
-- Name: COLUMN "docker_operation"."targetName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."targetName" IS '目标名称';


--
-- Name: COLUMN "docker_operation"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."status" IS '状态';


--
-- Name: COLUMN "docker_operation"."progressPercent"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."progressPercent" IS '进度百分比';


--
-- Name: COLUMN "docker_operation"."currentStage"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."currentStage" IS '当前阶段';


--
-- Name: COLUMN "docker_operation"."errorSummary"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."errorSummary" IS '错误摘要';


--
-- Name: COLUMN "docker_operation"."resultJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."resultJson" IS '结果JSON';


--
-- Name: COLUMN "docker_operation"."requestPayloadPreview"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."requestPayloadPreview" IS '脱敏请求摘要';


--
-- Name: COLUMN "docker_operation"."requestPayloadCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."requestPayloadCiphertext" IS '加密请求载荷';


--
-- Name: COLUMN "docker_operation"."requestPayloadEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."requestPayloadEdek" IS '加密数据密钥';


--
-- Name: COLUMN "docker_operation"."requestPayloadWrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."requestPayloadWrapKeyRef" IS '包装密钥引用';


--
-- Name: COLUMN "docker_operation"."actorUserId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."actorUserId" IS '操作者用户ID';


--
-- Name: COLUMN "docker_operation"."actorUsername"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."actorUsername" IS '操作者账号';


--
-- Name: COLUMN "docker_operation"."retryOf"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."retryOf" IS '重试来源操作ID';


--
-- Name: COLUMN "docker_operation"."cancelRequested"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."cancelRequested" IS '是否请求取消';


--
-- Name: COLUMN "docker_operation"."timeoutAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."timeoutAt" IS '超时时间';


--
-- Name: COLUMN "docker_operation"."startedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."startedAt" IS '开始时间';


--
-- Name: COLUMN "docker_operation"."finishedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."finishedAt" IS '结束时间';


--
-- Name: COLUMN "docker_operation"."heartbeatAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."heartbeatAt" IS '心跳时间';


--
-- Name: COLUMN "docker_operation"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."createTime" IS '创建时间';


--
-- Name: COLUMN "docker_operation"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation"."updateTime" IS '更新时间';


--
-- Name: docker_operation_event; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."docker_operation_event" (
    "id" bigint NOT NULL,
    "operationId" bigint NOT NULL,
    "sequence" bigint NOT NULL,
    "eventType" character varying(32) NOT NULL,
    "stage" character varying(128),
    "percent" integer,
    "message" character varying(2048),
    "payloadJson" "text",
    "occurredAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "docker_operation_event"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."docker_operation_event" IS 'Docker异步操作事件';


--
-- Name: COLUMN "docker_operation_event"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."id" IS 'Docker操作事件ID';


--
-- Name: COLUMN "docker_operation_event"."operationId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."operationId" IS 'Docker操作ID';


--
-- Name: COLUMN "docker_operation_event"."sequence"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."sequence" IS '事件序号';


--
-- Name: COLUMN "docker_operation_event"."eventType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."eventType" IS '事件类型';


--
-- Name: COLUMN "docker_operation_event"."stage"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."stage" IS '阶段';


--
-- Name: COLUMN "docker_operation_event"."percent"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."percent" IS '百分比';


--
-- Name: COLUMN "docker_operation_event"."message"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."message" IS '消息';


--
-- Name: COLUMN "docker_operation_event"."payloadJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."payloadJson" IS '脱敏载荷';


--
-- Name: COLUMN "docker_operation_event"."occurredAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_operation_event"."occurredAt" IS '发生时间';


--
-- Name: docker_remote_registry; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "docker_remote_registry"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."docker_remote_registry" IS 'Docker远程注册中心配置';


--
-- Name: COLUMN "docker_remote_registry"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."id" IS '注册中心ID';


--
-- Name: COLUMN "docker_remote_registry"."name"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."name" IS '注册中心名称';


--
-- Name: COLUMN "docker_remote_registry"."code"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."code" IS '注册中心编码';


--
-- Name: COLUMN "docker_remote_registry"."registryType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."registryType" IS '注册中心类型';


--
-- Name: COLUMN "docker_remote_registry"."endpoint"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."endpoint" IS 'Registry endpoint';


--
-- Name: COLUMN "docker_remote_registry"."apiBaseUrl"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."apiBaseUrl" IS 'Registry HTTP API base URL';


--
-- Name: COLUMN "docker_remote_registry"."authType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."authType" IS '认证类型';


--
-- Name: COLUMN "docker_remote_registry"."username"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."username" IS '用户名';


--
-- Name: COLUMN "docker_remote_registry"."tokenRealm"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."tokenRealm" IS 'Bearer token realm';


--
-- Name: COLUMN "docker_remote_registry"."tokenService"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."tokenService" IS 'Bearer token service';


--
-- Name: COLUMN "docker_remote_registry"."credentialId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."credentialId" IS '外部凭证ID';


--
-- Name: COLUMN "docker_remote_registry"."namespaceWhitelistJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."namespaceWhitelistJson" IS '命名空间白名单JSON';


--
-- Name: COLUMN "docker_remote_registry"."tlsEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."tlsEnabled" IS '是否启用TLS';


--
-- Name: COLUMN "docker_remote_registry"."insecureSkipVerify"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."insecureSkipVerify" IS '是否跳过TLS校验';


--
-- Name: COLUMN "docker_remote_registry"."defaultRegistry"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."defaultRegistry" IS '是否默认注册中心';


--
-- Name: COLUMN "docker_remote_registry"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."status" IS '状态：0启用 1停用';


--
-- Name: COLUMN "docker_remote_registry"."description"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."description" IS '描述';


--
-- Name: COLUMN "docker_remote_registry"."sort"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."sort" IS '排序';


--
-- Name: COLUMN "docker_remote_registry"."secretCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."secretCiphertext" IS '加密后的密码密文';


--
-- Name: COLUMN "docker_remote_registry"."secretEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."secretEdek" IS '加密后的数据密钥';


--
-- Name: COLUMN "docker_remote_registry"."wrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."wrapKeyRef" IS '包装密钥引用';


--
-- Name: COLUMN "docker_remote_registry"."deleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."deleted" IS '是否删除';


--
-- Name: COLUMN "docker_remote_registry"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."createTime" IS '创建时间';


--
-- Name: COLUMN "docker_remote_registry"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."docker_remote_registry"."updateTime" IS '更新时间';


--
-- Name: sysExternalLoginProvider; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysExternalLoginProvider" (
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
    "scopesJson" json NOT NULL,
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
    "metadataJson" json,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysExternalLoginProvider"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysExternalLoginProvider" IS '外部登录提供方配置表';


--
-- Name: COLUMN "sysExternalLoginProvider"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."id" IS '主键标识';


--
-- Name: COLUMN "sysExternalLoginProvider"."providerCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."providerCode" IS '提供方编码';


--
-- Name: COLUMN "sysExternalLoginProvider"."providerName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."providerName" IS '提供方名称';


--
-- Name: COLUMN "sysExternalLoginProvider"."protocolType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."protocolType" IS '协议类型：OIDC/OAUTH2';


--
-- Name: COLUMN "sysExternalLoginProvider"."issuer"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."issuer" IS 'OIDC issuer';


--
-- Name: COLUMN "sysExternalLoginProvider"."authorizationEndpoint"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."authorizationEndpoint" IS '授权端点';


--
-- Name: COLUMN "sysExternalLoginProvider"."tokenEndpoint"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."tokenEndpoint" IS '令牌端点';


--
-- Name: COLUMN "sysExternalLoginProvider"."userinfoEndpoint"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."userinfoEndpoint" IS '用户信息端点';


--
-- Name: COLUMN "sysExternalLoginProvider"."jwksUri"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."jwksUri" IS 'JWKS 地址';


--
-- Name: COLUMN "sysExternalLoginProvider"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."clientId" IS '外部平台客户端 ID';


--
-- Name: COLUMN "sysExternalLoginProvider"."clientSecretCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."clientSecretCiphertext" IS '客户端密钥密文';


--
-- Name: COLUMN "sysExternalLoginProvider"."clientSecretEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."clientSecretEdek" IS '客户端密钥 EDEK';


--
-- Name: COLUMN "sysExternalLoginProvider"."clientSecretWrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."clientSecretWrapKeyRef" IS '客户端密钥包装密钥引用';


--
-- Name: COLUMN "sysExternalLoginProvider"."scopesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."scopesJson" IS '授权范围 JSON';


--
-- Name: COLUMN "sysExternalLoginProvider"."redirectUri"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."redirectUri" IS '回调地址';


--
-- Name: COLUMN "sysExternalLoginProvider"."displayName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."displayName" IS '登录页展示名称';


--
-- Name: COLUMN "sysExternalLoginProvider"."icon"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."icon" IS '图标编码';


--
-- Name: COLUMN "sysExternalLoginProvider"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."sortOrder" IS '排序';


--
-- Name: COLUMN "sysExternalLoginProvider"."displayEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."displayEnabled" IS '是否展示在登录页';


--
-- Name: COLUMN "sysExternalLoginProvider"."loginEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."loginEnabled" IS '是否允许登录';


--
-- Name: COLUMN "sysExternalLoginProvider"."bindEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."bindEnabled" IS '是否允许绑定身份';


--
-- Name: COLUMN "sysExternalLoginProvider"."emailAutoBindEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."emailAutoBindEnabled" IS '是否允许 verified email 自动绑定';


--
-- Name: COLUMN "sysExternalLoginProvider"."accountAutoCreateEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."accountAutoCreateEnabled" IS '是否允许 verified email 自动创建本地用户';


--
-- Name: COLUMN "sysExternalLoginProvider"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysExternalLoginProvider"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysExternalLoginProvider"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."creatorId" IS '创建人 ID';


--
-- Name: COLUMN "sysExternalLoginProvider"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."updaterId" IS '更新人 ID';


--
-- Name: COLUMN "sysExternalLoginProvider"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysExternalLoginProvider"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysExternalLoginProvider"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalLoginProvider"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysExternalLoginProvider_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysExternalLoginProvider_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysExternalLoginProvider_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysExternalLoginProvider_id_seq" OWNED BY "public"."sysExternalLoginProvider"."id";


--
-- Name: sysExternalManagedProviderCommand; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysExternalManagedProviderCommand" (
    "providerCode" character varying(64) NOT NULL,
    "connectionVersion" character varying(128) NOT NULL,
    "requestHash" character(64) NOT NULL,
    "createdAt" timestamp with time zone NOT NULL
);


--
-- Name: TABLE "sysExternalManagedProviderCommand"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysExternalManagedProviderCommand" IS 'Node系统托管OIDC Provider命令账本';


--
-- Name: COLUMN "sysExternalManagedProviderCommand"."providerCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalManagedProviderCommand"."providerCode" IS '系统托管Provider编码';


--
-- Name: COLUMN "sysExternalManagedProviderCommand"."connectionVersion"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalManagedProviderCommand"."connectionVersion" IS '连接版本';


--
-- Name: COLUMN "sysExternalManagedProviderCommand"."requestHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalManagedProviderCommand"."requestHash" IS '完整配置请求摘要';


--
-- Name: COLUMN "sysExternalManagedProviderCommand"."createdAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalManagedProviderCommand"."createdAt" IS '首次应用时间';


--
-- Name: sysExternalOAuthLoginState; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysExternalOAuthLoginState" (
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


--
-- Name: TABLE "sysExternalOAuthLoginState"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysExternalOAuthLoginState" IS '外部 OAuth 登录状态表';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."id" IS '主键标识';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."stateId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."stateId" IS '状态 ID';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."providerCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."providerCode" IS '提供方编码';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."platformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."platformCode" IS '平台编码';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."provisioningAuthorityId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."provisioningAuthorityId" IS '平台注册授权ID';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."loginTransactionId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."loginTransactionId" IS 'SSO 登录交易 ID';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."redirectAfterLogin"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."redirectAfterLogin" IS '登录后跳转地址';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."bindUserId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."bindUserId" IS '主动绑定当前用户ID';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."stateHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."stateHash" IS 'state 哈希';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."nonceHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."nonceHash" IS 'nonce 哈希';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."codeVerifierCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."codeVerifierCiphertext" IS 'PKCE verifier 密文';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."codeVerifierEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."codeVerifierEdek" IS 'PKCE verifier EDEK';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."codeVerifierWrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."codeVerifierWrapKeyRef" IS 'PKCE verifier 包装密钥引用';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."issuer"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."issuer" IS '绑定 issuer';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."providerConfigDigest"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."providerConfigDigest" IS '托管Provider启动配置摘要';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."redirectUri"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."redirectUri" IS '绑定回调地址';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."expiresAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."expiresAt" IS '过期时间';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."consumedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."consumedAt" IS '消费时间';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."status" IS '状态：0 ACTIVE，1 CONSUMED，2 EXPIRED';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."loginIp"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."loginIp" IS '登录 IP';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."userAgent"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."userAgent" IS '用户代理';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."traceId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."traceId" IS '追踪 ID';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysExternalOAuthLoginState"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthLoginState"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysExternalOAuthLoginState_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysExternalOAuthLoginState_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysExternalOAuthLoginState_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysExternalOAuthLoginState_id_seq" OWNED BY "public"."sysExternalOAuthLoginState"."id";


--
-- Name: sysExternalOAuthToken; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysExternalOAuthToken" (
    "id" bigint NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "identityId" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "tokenPurpose" character varying(32) NOT NULL,
    "scopeJson" json,
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
    "metadataJson" json,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysExternalOAuthToken"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysExternalOAuthToken" IS '外部 OAuth 令牌保险库表';


--
-- Name: COLUMN "sysExternalOAuthToken"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."id" IS '主键标识';


--
-- Name: COLUMN "sysExternalOAuthToken"."providerCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."providerCode" IS '提供方编码';


--
-- Name: COLUMN "sysExternalOAuthToken"."identityId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."identityId" IS '外部身份绑定 ID';


--
-- Name: COLUMN "sysExternalOAuthToken"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."userId" IS '本地用户 ID';


--
-- Name: COLUMN "sysExternalOAuthToken"."tokenPurpose"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."tokenPurpose" IS '令牌用途：LOGIN/API';


--
-- Name: COLUMN "sysExternalOAuthToken"."scopeJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."scopeJson" IS '授权范围 JSON';


--
-- Name: COLUMN "sysExternalOAuthToken"."scopeHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."scopeHash" IS '授权范围哈希';


--
-- Name: COLUMN "sysExternalOAuthToken"."tokenSetCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."tokenSetCiphertext" IS '令牌集密文';


--
-- Name: COLUMN "sysExternalOAuthToken"."tokenSetEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."tokenSetEdek" IS '令牌集 EDEK';


--
-- Name: COLUMN "sysExternalOAuthToken"."tokenSetWrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."tokenSetWrapKeyRef" IS '令牌集包装密钥引用';


--
-- Name: COLUMN "sysExternalOAuthToken"."accessExpiresAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."accessExpiresAt" IS 'access token 过期时间';


--
-- Name: COLUMN "sysExternalOAuthToken"."refreshExpiresAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."refreshExpiresAt" IS 'refresh token 过期时间';


--
-- Name: COLUMN "sysExternalOAuthToken"."lastRefreshAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."lastRefreshAt" IS '最近刷新时间';


--
-- Name: COLUMN "sysExternalOAuthToken"."revokedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."revokedAt" IS '撤销时间';


--
-- Name: COLUMN "sysExternalOAuthToken"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."status" IS '状态：0 ACTIVE，1 REVOKED，2 EXPIRED，3 REFRESH_FAILED';


--
-- Name: COLUMN "sysExternalOAuthToken"."version"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."version" IS '乐观锁版本';


--
-- Name: COLUMN "sysExternalOAuthToken"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysExternalOAuthToken"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysExternalOAuthToken"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysExternalOAuthToken"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalOAuthToken"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysExternalOAuthToken_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysExternalOAuthToken_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysExternalOAuthToken_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysExternalOAuthToken_id_seq" OWNED BY "public"."sysExternalOAuthToken"."id";


--
-- Name: sysExternalProviderMethod; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysExternalProviderMethod" (
    "id" bigint NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "methodKey" character varying(128) NOT NULL,
    "capabilityCode" character varying(64) NOT NULL,
    "requiredScopesJson" json,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" json,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysExternalProviderMethod"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysExternalProviderMethod" IS '外部提供方能力方法索引表';


--
-- Name: COLUMN "sysExternalProviderMethod"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."id" IS '主键标识';


--
-- Name: COLUMN "sysExternalProviderMethod"."providerCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."providerCode" IS '提供方编码';


--
-- Name: COLUMN "sysExternalProviderMethod"."methodKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."methodKey" IS '能力方法键';


--
-- Name: COLUMN "sysExternalProviderMethod"."capabilityCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."capabilityCode" IS '能力编码';


--
-- Name: COLUMN "sysExternalProviderMethod"."requiredScopesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."requiredScopesJson" IS '所需 scope JSON';


--
-- Name: COLUMN "sysExternalProviderMethod"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysExternalProviderMethod"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysExternalProviderMethod"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysExternalProviderMethod"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysExternalProviderMethod"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalProviderMethod"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysExternalProviderMethod_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysExternalProviderMethod_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysExternalProviderMethod_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysExternalProviderMethod_id_seq" OWNED BY "public"."sysExternalProviderMethod"."id";


--
-- Name: sysExternalUserIdentity; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysExternalUserIdentity" (
    "id" bigint NOT NULL,
    "providerCode" character varying(64) NOT NULL,
    "externalIssuer" character varying(512),
    "externalIssuerDigest" "bytea",
    "externalIdentityDigest" "bytea",
    "externalSubject" character varying(255) NOT NULL,
    "providerSubjectDigest" "bytea",
    "userId" bigint NOT NULL,
    "externalLogin" character varying(255),
    "externalEmail" character varying(255),
    "emailVerified" smallint DEFAULT '0'::smallint NOT NULL,
    "displayName" character varying(255),
    "avatarUrl" character varying(1024),
    "profileJson" json,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "firstLinkedAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "lastLoginAt" timestamp with time zone,
    "lastVerifiedAt" timestamp with time zone,
    "metadataJson" json,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysExternalUserIdentity"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysExternalUserIdentity" IS '外部用户身份绑定表';


--
-- Name: COLUMN "sysExternalUserIdentity"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."id" IS '主键标识';


--
-- Name: COLUMN "sysExternalUserIdentity"."providerCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."providerCode" IS '提供方编码';


--
-- Name: COLUMN "sysExternalUserIdentity"."externalIssuer"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."externalIssuer" IS '经验证的OIDC issuer';


--
-- Name: COLUMN "sysExternalUserIdentity"."externalSubject"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."externalSubject" IS '外部稳定主体 ID';


--
-- Name: COLUMN "sysExternalUserIdentity"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."userId" IS '本地用户 ID';


--
-- Name: COLUMN "sysExternalUserIdentity"."externalLogin"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."externalLogin" IS '外部登录名';


--
-- Name: COLUMN "sysExternalUserIdentity"."externalEmail"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."externalEmail" IS '外部邮箱';


--
-- Name: COLUMN "sysExternalUserIdentity"."emailVerified"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."emailVerified" IS '外部邮箱是否已验证';


--
-- Name: COLUMN "sysExternalUserIdentity"."displayName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."displayName" IS '外部展示名称';


--
-- Name: COLUMN "sysExternalUserIdentity"."avatarUrl"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."avatarUrl" IS '外部头像 URL';


--
-- Name: COLUMN "sysExternalUserIdentity"."profileJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."profileJson" IS '外部资料 JSON';


--
-- Name: COLUMN "sysExternalUserIdentity"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."status" IS '状态：0 ACTIVE，1 DISABLED，2 UNLINKED';


--
-- Name: COLUMN "sysExternalUserIdentity"."firstLinkedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."firstLinkedAt" IS '首次绑定时间';


--
-- Name: COLUMN "sysExternalUserIdentity"."lastLoginAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."lastLoginAt" IS '最近登录时间';


--
-- Name: COLUMN "sysExternalUserIdentity"."lastVerifiedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."lastVerifiedAt" IS '最近验证时间';


--
-- Name: COLUMN "sysExternalUserIdentity"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysExternalUserIdentity"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."creatorId" IS '创建人 ID';


--
-- Name: COLUMN "sysExternalUserIdentity"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."updaterId" IS '更新人 ID';


--
-- Name: COLUMN "sysExternalUserIdentity"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysExternalUserIdentity"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysExternalUserIdentity"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysExternalUserIdentity"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysExternalUserIdentity_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysExternalUserIdentity_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysExternalUserIdentity_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysExternalUserIdentity_id_seq" OWNED BY "public"."sysExternalUserIdentity"."id";


--
-- Name: sysFederatedNode; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysFederatedNode" (
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


--
-- Name: TABLE "sysFederatedNode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysFederatedNode" IS 'Hub联邦节点注册表';


--
-- Name: COLUMN "sysFederatedNode"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."id" IS '主键';


--
-- Name: COLUMN "sysFederatedNode"."nodeCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."nodeCode" IS '稳定节点编码';


--
-- Name: COLUMN "sysFederatedNode"."nodeName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."nodeName" IS '节点名称';


--
-- Name: COLUMN "sysFederatedNode"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."status" IS '状态:0启用,1禁用';


--
-- Name: COLUMN "sysFederatedNode"."discoveryType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."discoveryType" IS '发现类型:STATIC,CONSUL';


--
-- Name: COLUMN "sysFederatedNode"."serviceName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."serviceName" IS 'Consul服务名称';


--
-- Name: COLUMN "sysFederatedNode"."managementBaseUrl"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."managementBaseUrl" IS '静态管理地址';


--
-- Name: COLUMN "sysFederatedNode"."hubIssuer"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."hubIssuer" IS 'Hub公开OIDC issuer';


--
-- Name: COLUMN "sysFederatedNode"."oidcClientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."oidcClientId" IS '系统托管OIDC客户端ID';


--
-- Name: COLUMN "sysFederatedNode"."oidcClientSecretCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."oidcClientSecretCiphertext" IS 'OIDC客户端密钥密文';


--
-- Name: COLUMN "sysFederatedNode"."oidcClientSecretEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."oidcClientSecretEdek" IS 'OIDC客户端密钥封装DEK';


--
-- Name: COLUMN "sysFederatedNode"."oidcClientSecretWrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."oidcClientSecretWrapKeyRef" IS 'OIDC客户端密钥包装密钥引用';


--
-- Name: COLUMN "sysFederatedNode"."managementBearerCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."managementBearerCiphertext" IS '管理Bearer密文';


--
-- Name: COLUMN "sysFederatedNode"."managementBearerEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."managementBearerEdek" IS '管理Bearer封装DEK';


--
-- Name: COLUMN "sysFederatedNode"."managementBearerWrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."managementBearerWrapKeyRef" IS '管理Bearer包装密钥引用';


--
-- Name: COLUMN "sysFederatedNode"."capabilitiesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."capabilitiesJson" IS '安全能力快照JSON';


--
-- Name: COLUMN "sysFederatedNode"."connectionStatus"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."connectionStatus" IS '连接状态:PENDING,ACTIVE,ERROR';


--
-- Name: COLUMN "sysFederatedNode"."connectionVersion"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."connectionVersion" IS '稳定连接版本';


--
-- Name: COLUMN "sysFederatedNode"."connectionRequestHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."connectionRequestHash" IS '连接重放请求哈希';


--
-- Name: COLUMN "sysFederatedNode"."targetRevision"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."targetRevision" IS '路由与管理凭证单调修订号';


--
-- Name: COLUMN "sysFederatedNode"."issuerLockedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."issuerLockedAt" IS '首次激活时间及issuer永久锁标记';


--
-- Name: COLUMN "sysFederatedNode"."lastConnectionError"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."lastConnectionError" IS '脱敏连接错误';


--
-- Name: COLUMN "sysFederatedNode"."lastConnectionTraceId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."lastConnectionTraceId" IS '远端追踪ID';


--
-- Name: COLUMN "sysFederatedNode"."lastHealthyAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."lastHealthyAt" IS '最近健康时间';


--
-- Name: COLUMN "sysFederatedNode"."createdAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."createdAt" IS '创建时间';


--
-- Name: COLUMN "sysFederatedNode"."updatedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."updatedAt" IS '更新时间';


--
-- Name: COLUMN "sysFederatedNode"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNode"."isDeleted" IS '软删除标记';


--
-- Name: sysFederatedNodeConnectionCommand; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysFederatedNodeConnectionCommand" (
    "nodeCode" character varying(64) NOT NULL,
    "connectionVersion" character varying(128) NOT NULL,
    "requestHash" character(64) NOT NULL,
    "targetRevision" bigint NOT NULL,
    "terminalState" character varying(16) NOT NULL,
    "createdAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedAt" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sysFederatedNodeConnectionCommand"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysFederatedNodeConnectionCommand" IS 'Hub节点连接命令元数据账本';


--
-- Name: COLUMN "sysFederatedNodeConnectionCommand"."nodeCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNodeConnectionCommand"."nodeCode" IS '稳定节点编码';


--
-- Name: COLUMN "sysFederatedNodeConnectionCommand"."connectionVersion"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNodeConnectionCommand"."connectionVersion" IS '客户端提供的稳定连接版本';


--
-- Name: COLUMN "sysFederatedNodeConnectionCommand"."requestHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNodeConnectionCommand"."requestHash" IS '连接请求哈希';


--
-- Name: COLUMN "sysFederatedNodeConnectionCommand"."targetRevision"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNodeConnectionCommand"."targetRevision" IS '接受命令时的目标修订号';


--
-- Name: COLUMN "sysFederatedNodeConnectionCommand"."terminalState"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNodeConnectionCommand"."terminalState" IS '命令状态:PENDING,ACTIVE,ERROR,SUPERSEDED';


--
-- Name: COLUMN "sysFederatedNodeConnectionCommand"."createdAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNodeConnectionCommand"."createdAt" IS '创建时间';


--
-- Name: COLUMN "sysFederatedNodeConnectionCommand"."updatedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysFederatedNodeConnectionCommand"."updatedAt" IS '更新时间';


--
-- Name: sysNotificationChannel; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysNotificationChannel" (
    "id" bigint NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "channelName" character varying(128) NOT NULL,
    "channelType" character varying(32) NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "priority" integer DEFAULT 100 NOT NULL,
    "configJson" json,
    "secretCiphertext" "text",
    "secretEdek" "text",
    "secretWrapKeyRef" character varying(128),
    "rateLimitJson" json,
    "metadataJson" json,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysNotificationChannel"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysNotificationChannel" IS '通知渠道';


--
-- Name: COLUMN "sysNotificationChannel"."channelCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."channelCode" IS '渠道编码';


--
-- Name: COLUMN "sysNotificationChannel"."channelName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."channelName" IS '渠道名称';


--
-- Name: COLUMN "sysNotificationChannel"."channelType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."channelType" IS '渠道类型';


--
-- Name: COLUMN "sysNotificationChannel"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."status" IS '状态：0启用 1停用';


--
-- Name: COLUMN "sysNotificationChannel"."priority"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."priority" IS '优先级';


--
-- Name: COLUMN "sysNotificationChannel"."configJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."configJson" IS '非敏感配置';


--
-- Name: COLUMN "sysNotificationChannel"."secretCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."secretCiphertext" IS '敏感配置密文';


--
-- Name: COLUMN "sysNotificationChannel"."secretEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."secretEdek" IS '敏感配置EDEK';


--
-- Name: COLUMN "sysNotificationChannel"."secretWrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."secretWrapKeyRef" IS '敏感配置包装密钥引用';


--
-- Name: COLUMN "sysNotificationChannel"."rateLimitJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."rateLimitJson" IS '限流配置';


--
-- Name: COLUMN "sysNotificationChannel"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."metadataJson" IS '扩展元数据';


--
-- Name: COLUMN "sysNotificationChannel"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."creatorId" IS '创建人';


--
-- Name: COLUMN "sysNotificationChannel"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."updaterId" IS '更新人';


--
-- Name: COLUMN "sysNotificationChannel"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysNotificationChannel"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysNotificationChannel"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationChannel"."isDeleted" IS '是否删除';


--
-- Name: sysNotificationDelivery; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysNotificationDelivery" (
    "id" bigint NOT NULL,
    "deliveryId" character varying(96) NOT NULL,
    "requestDigest" character varying(128) NOT NULL,
    "sceneCode" character varying(64) NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "channelType" character varying(32) NOT NULL,
    "templateCode" character varying(96) NOT NULL,
    "target" character varying(512),
    "targetMasked" character varying(512),
    "payloadJson" json,
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
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysNotificationDelivery"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysNotificationDelivery" IS '通知投递';


--
-- Name: COLUMN "sysNotificationDelivery"."deliveryId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."deliveryId" IS '投递ID';


--
-- Name: COLUMN "sysNotificationDelivery"."requestDigest"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."requestDigest" IS '请求摘要';


--
-- Name: COLUMN "sysNotificationDelivery"."sceneCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."sceneCode" IS '场景编码';


--
-- Name: COLUMN "sysNotificationDelivery"."channelCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."channelCode" IS '渠道编码';


--
-- Name: COLUMN "sysNotificationDelivery"."channelType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."channelType" IS '渠道类型';


--
-- Name: COLUMN "sysNotificationDelivery"."templateCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."templateCode" IS '模板编码';


--
-- Name: COLUMN "sysNotificationDelivery"."target"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."target" IS '目标地址';


--
-- Name: COLUMN "sysNotificationDelivery"."targetMasked"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."targetMasked" IS '脱敏目标地址';


--
-- Name: COLUMN "sysNotificationDelivery"."payloadJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."payloadJson" IS '变量负载';


--
-- Name: COLUMN "sysNotificationDelivery"."renderedSubject"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."renderedSubject" IS '渲染标题';


--
-- Name: COLUMN "sysNotificationDelivery"."renderedText"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."renderedText" IS '渲染文本';


--
-- Name: COLUMN "sysNotificationDelivery"."renderedHtml"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."renderedHtml" IS '渲染HTML';


--
-- Name: COLUMN "sysNotificationDelivery"."renderedMarkdown"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."renderedMarkdown" IS '渲染Markdown';


--
-- Name: COLUMN "sysNotificationDelivery"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."status" IS '状态';


--
-- Name: COLUMN "sysNotificationDelivery"."retryCount"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."retryCount" IS '重试次数';


--
-- Name: COLUMN "sysNotificationDelivery"."maxRetry"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."maxRetry" IS '最大重试次数';


--
-- Name: COLUMN "sysNotificationDelivery"."nextRetryAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."nextRetryAt" IS '下次重试时间';


--
-- Name: COLUMN "sysNotificationDelivery"."lastError"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."lastError" IS '最近错误';


--
-- Name: COLUMN "sysNotificationDelivery"."traceId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."traceId" IS '链路ID';


--
-- Name: COLUMN "sysNotificationDelivery"."sentAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."sentAt" IS '发送时间';


--
-- Name: COLUMN "sysNotificationDelivery"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."creatorId" IS '创建人';


--
-- Name: COLUMN "sysNotificationDelivery"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysNotificationDelivery"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysNotificationDelivery"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationDelivery"."isDeleted" IS '是否删除';


--
-- Name: sysNotificationSceneBinding; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysNotificationSceneBinding" (
    "id" bigint NOT NULL,
    "sceneCode" character varying(64) NOT NULL,
    "sceneName" character varying(128) NOT NULL,
    "channelCode" character varying(64) NOT NULL,
    "templateCode" character varying(96) NOT NULL,
    "enabled" smallint DEFAULT '1'::smallint NOT NULL,
    "priority" integer DEFAULT 100 NOT NULL,
    "maxRetry" integer DEFAULT 3 NOT NULL,
    "retryIntervalSeconds" integer DEFAULT 60 NOT NULL,
    "metadataJson" json,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysNotificationSceneBinding"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysNotificationSceneBinding" IS '通知场景绑定';


--
-- Name: COLUMN "sysNotificationSceneBinding"."sceneCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."sceneCode" IS '场景编码';


--
-- Name: COLUMN "sysNotificationSceneBinding"."sceneName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."sceneName" IS '场景名称';


--
-- Name: COLUMN "sysNotificationSceneBinding"."channelCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."channelCode" IS '渠道编码';


--
-- Name: COLUMN "sysNotificationSceneBinding"."templateCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."templateCode" IS '模板编码';


--
-- Name: COLUMN "sysNotificationSceneBinding"."enabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."enabled" IS '是否启用';


--
-- Name: COLUMN "sysNotificationSceneBinding"."priority"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."priority" IS '优先级';


--
-- Name: COLUMN "sysNotificationSceneBinding"."maxRetry"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."maxRetry" IS '最大重试次数';


--
-- Name: COLUMN "sysNotificationSceneBinding"."retryIntervalSeconds"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."retryIntervalSeconds" IS '重试间隔秒';


--
-- Name: COLUMN "sysNotificationSceneBinding"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."metadataJson" IS '扩展元数据';


--
-- Name: COLUMN "sysNotificationSceneBinding"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."creatorId" IS '创建人';


--
-- Name: COLUMN "sysNotificationSceneBinding"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."updaterId" IS '更新人';


--
-- Name: COLUMN "sysNotificationSceneBinding"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysNotificationSceneBinding"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysNotificationSceneBinding"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationSceneBinding"."isDeleted" IS '是否删除';


--
-- Name: sysNotificationTemplate; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysNotificationTemplate" (
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
    "jsonTemplate" json,
    "variablesJson" json,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "version" integer DEFAULT 1 NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysNotificationTemplate"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysNotificationTemplate" IS '通知模板';


--
-- Name: COLUMN "sysNotificationTemplate"."templateCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."templateCode" IS '模板编码';


--
-- Name: COLUMN "sysNotificationTemplate"."templateName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."templateName" IS '模板名称';


--
-- Name: COLUMN "sysNotificationTemplate"."sceneCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."sceneCode" IS '场景编码';


--
-- Name: COLUMN "sysNotificationTemplate"."channelType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."channelType" IS '渠道类型';


--
-- Name: COLUMN "sysNotificationTemplate"."locale"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."locale" IS '语言区域';


--
-- Name: COLUMN "sysNotificationTemplate"."subjectTemplate"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."subjectTemplate" IS '标题模板';


--
-- Name: COLUMN "sysNotificationTemplate"."textTemplate"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."textTemplate" IS '文本模板';


--
-- Name: COLUMN "sysNotificationTemplate"."htmlTemplate"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."htmlTemplate" IS 'HTML模板';


--
-- Name: COLUMN "sysNotificationTemplate"."markdownTemplate"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."markdownTemplate" IS 'Markdown模板';


--
-- Name: COLUMN "sysNotificationTemplate"."jsonTemplate"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."jsonTemplate" IS 'JSON模板';


--
-- Name: COLUMN "sysNotificationTemplate"."variablesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."variablesJson" IS '变量定义';


--
-- Name: COLUMN "sysNotificationTemplate"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."status" IS '状态：0启用 1停用';


--
-- Name: COLUMN "sysNotificationTemplate"."version"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."version" IS '版本';


--
-- Name: COLUMN "sysNotificationTemplate"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."creatorId" IS '创建人';


--
-- Name: COLUMN "sysNotificationTemplate"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."updaterId" IS '更新人';


--
-- Name: COLUMN "sysNotificationTemplate"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysNotificationTemplate"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysNotificationTemplate"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysNotificationTemplate"."isDeleted" IS '是否删除';


--
-- Name: sysPlatform; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysPlatform" (
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
    "brandJson" json,
    "settingsJson" json,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysPlatform"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysPlatform" IS '平台配置表';


--
-- Name: COLUMN "sysPlatform"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."id" IS '主键标识';


--
-- Name: COLUMN "sysPlatform"."platformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."platformCode" IS '平台编码';


--
-- Name: COLUMN "sysPlatform"."platformName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."platformName" IS '平台名称';


--
-- Name: COLUMN "sysPlatform"."platformType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."platformType" IS '平台类型：ADMIN/PORTAL/API';


--
-- Name: COLUMN "sysPlatform"."description"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."description" IS '平台说明';


--
-- Name: COLUMN "sysPlatform"."defaultRedirectUrl"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."defaultRedirectUrl" IS '默认登录后跳转地址';


--
-- Name: COLUMN "sysPlatform"."allowAutoRegister"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."allowAutoRegister" IS '是否允许自动创建用户';


--
-- Name: COLUMN "sysPlatform"."allowFormRegister"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."allowFormRegister" IS '是否允许表单注册';


--
-- Name: COLUMN "sysPlatform"."isDefault"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."isDefault" IS '是否默认平台';


--
-- Name: COLUMN "sysPlatform"."defaultDeptId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."defaultDeptId" IS '默认部门 ID';


--
-- Name: COLUMN "sysPlatform"."brandJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."brandJson" IS '登录页品牌配置 JSON';


--
-- Name: COLUMN "sysPlatform"."settingsJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."settingsJson" IS '平台扩展设置 JSON';


--
-- Name: COLUMN "sysPlatform"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysPlatform"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."creatorId" IS '创建人 ID';


--
-- Name: COLUMN "sysPlatform"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."updaterId" IS '更新人 ID';


--
-- Name: COLUMN "sysPlatform"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysPlatform"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysPlatform"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatform"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysPlatformDefaultRole; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysPlatformDefaultRole" (
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


--
-- Name: TABLE "sysPlatformDefaultRole"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysPlatformDefaultRole" IS '平台默认角色配置表';


--
-- Name: COLUMN "sysPlatformDefaultRole"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."id" IS '主键标识';


--
-- Name: COLUMN "sysPlatformDefaultRole"."platformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."platformCode" IS '平台编码';


--
-- Name: COLUMN "sysPlatformDefaultRole"."roleId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."roleId" IS '默认角色 ID';


--
-- Name: COLUMN "sysPlatformDefaultRole"."autoAssignEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."autoAssignEnabled" IS '是否允许自动注册分配';


--
-- Name: COLUMN "sysPlatformDefaultRole"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysPlatformDefaultRole"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."creatorId" IS '创建人 ID';


--
-- Name: COLUMN "sysPlatformDefaultRole"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."updaterId" IS '更新人 ID';


--
-- Name: COLUMN "sysPlatformDefaultRole"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysPlatformDefaultRole"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysPlatformDefaultRole"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformDefaultRole"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysPlatformDefaultRole_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysPlatformDefaultRole_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysPlatformDefaultRole_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysPlatformDefaultRole_id_seq" OWNED BY "public"."sysPlatformDefaultRole"."id";


--
-- Name: sysPlatformLoginMethod; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysPlatformLoginMethod" (
    "id" bigint NOT NULL,
    "platformCode" character varying(64) NOT NULL,
    "methodType" character varying(32) NOT NULL,
    "providerCode" character varying(64) DEFAULT ''::character varying NOT NULL,
    "displayName" character varying(128) NOT NULL,
    "icon" character varying(128),
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "displayEnabled" smallint DEFAULT '1'::smallint NOT NULL,
    "loginEnabled" smallint DEFAULT '1'::smallint NOT NULL,
    "metadataJson" json,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysPlatformLoginMethod"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysPlatformLoginMethod" IS '平台登录方式配置表';


--
-- Name: COLUMN "sysPlatformLoginMethod"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."id" IS '主键标识';


--
-- Name: COLUMN "sysPlatformLoginMethod"."platformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."platformCode" IS '平台编码';


--
-- Name: COLUMN "sysPlatformLoginMethod"."methodType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."methodType" IS '登录方式：PASSWORD/PASSKEY/EXTERNAL_OAUTH';


--
-- Name: COLUMN "sysPlatformLoginMethod"."providerCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."providerCode" IS '外部登录提供方编码，非外部登录为空字符串';


--
-- Name: COLUMN "sysPlatformLoginMethod"."displayName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."displayName" IS '登录页展示名称';


--
-- Name: COLUMN "sysPlatformLoginMethod"."icon"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."icon" IS '图标编码';


--
-- Name: COLUMN "sysPlatformLoginMethod"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."sortOrder" IS '排序';


--
-- Name: COLUMN "sysPlatformLoginMethod"."displayEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."displayEnabled" IS '是否在登录页展示';


--
-- Name: COLUMN "sysPlatformLoginMethod"."loginEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."loginEnabled" IS '是否允许登录';


--
-- Name: COLUMN "sysPlatformLoginMethod"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysPlatformLoginMethod"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."creatorId" IS '创建人 ID';


--
-- Name: COLUMN "sysPlatformLoginMethod"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."updaterId" IS '更新人 ID';


--
-- Name: COLUMN "sysPlatformLoginMethod"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysPlatformLoginMethod"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysPlatformLoginMethod"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformLoginMethod"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysPlatformLoginMethod_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysPlatformLoginMethod_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysPlatformLoginMethod_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysPlatformLoginMethod_id_seq" OWNED BY "public"."sysPlatformLoginMethod"."id";


--
-- Name: sysPlatformSourceRule; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysPlatformSourceRule" (
    "id" bigint NOT NULL,
    "platformCode" character varying(64) NOT NULL,
    "matchType" character varying(32) NOT NULL,
    "matchValue" character varying(1024) NOT NULL,
    "priority" integer DEFAULT 0 NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" json,
    "creatorId" bigint,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysPlatformSourceRule"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysPlatformSourceRule" IS '平台来源匹配规则表';


--
-- Name: COLUMN "sysPlatformSourceRule"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."id" IS '主键标识';


--
-- Name: COLUMN "sysPlatformSourceRule"."platformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."platformCode" IS '平台编码';


--
-- Name: COLUMN "sysPlatformSourceRule"."matchType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."matchType" IS '匹配类型：CLIENT_ID/REDIRECT_HOST/REDIRECT_PREFIX/HOST/ORIGIN/REFERER_HOST';


--
-- Name: COLUMN "sysPlatformSourceRule"."matchValue"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."matchValue" IS '匹配值';


--
-- Name: COLUMN "sysPlatformSourceRule"."priority"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."priority" IS '优先级，数值越大越优先';


--
-- Name: COLUMN "sysPlatformSourceRule"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysPlatformSourceRule"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysPlatformSourceRule"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."creatorId" IS '创建人 ID';


--
-- Name: COLUMN "sysPlatformSourceRule"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."updaterId" IS '更新人 ID';


--
-- Name: COLUMN "sysPlatformSourceRule"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysPlatformSourceRule"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysPlatformSourceRule"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSourceRule"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysPlatformSourceRule_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysPlatformSourceRule_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysPlatformSourceRule_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysPlatformSourceRule_id_seq" OWNED BY "public"."sysPlatformSourceRule"."id";


--
-- Name: sysPlatformSsoClient; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysPlatformSsoClient" (
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


--
-- Name: TABLE "sysPlatformSsoClient"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysPlatformSsoClient" IS '平台与 SSO 客户端关联表';


--
-- Name: COLUMN "sysPlatformSsoClient"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."id" IS '主键标识';


--
-- Name: COLUMN "sysPlatformSsoClient"."platformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."platformCode" IS '平台编码';


--
-- Name: COLUMN "sysPlatformSsoClient"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."clientId" IS 'SSO 客户端标识';


--
-- Name: COLUMN "sysPlatformSsoClient"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysPlatformSsoClient"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."creatorId" IS '创建人 ID';


--
-- Name: COLUMN "sysPlatformSsoClient"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."updaterId" IS '更新人 ID';


--
-- Name: COLUMN "sysPlatformSsoClient"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysPlatformSsoClient"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysPlatformSsoClient"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysPlatformSsoClient"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysPlatformSsoClient_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysPlatformSsoClient_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysPlatformSsoClient_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysPlatformSsoClient_id_seq" OWNED BY "public"."sysPlatformSsoClient"."id";


--
-- Name: sysPlatform_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysPlatform_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysPlatform_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysPlatform_id_seq" OWNED BY "public"."sysPlatform"."id";


--
-- Name: sysSsoAuditLog; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoAuditLog" (
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
    "detailJson" json,
    "traceId" character varying(128),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysSsoAuditLog"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoAuditLog" IS 'SSO 审计日志表';


--
-- Name: COLUMN "sysSsoAuditLog"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoAuditLog"."eventType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."eventType" IS '事件类型';


--
-- Name: COLUMN "sysSsoAuditLog"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoAuditLog"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."userId" IS '用户 ID';


--
-- Name: COLUMN "sysSsoAuditLog"."sessionId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."sessionId" IS '会话 ID';


--
-- Name: COLUMN "sysSsoAuditLog"."deviceId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."deviceId" IS '设备 ID';


--
-- Name: COLUMN "sysSsoAuditLog"."tenantId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."tenantId" IS '租户 ID';


--
-- Name: COLUMN "sysSsoAuditLog"."loginIp"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."loginIp" IS '登录 IP';


--
-- Name: COLUMN "sysSsoAuditLog"."userAgent"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."userAgent" IS '用户代理';


--
-- Name: COLUMN "sysSsoAuditLog"."result"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."result" IS '事件结果';


--
-- Name: COLUMN "sysSsoAuditLog"."reasonCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."reasonCode" IS '原因编码';


--
-- Name: COLUMN "sysSsoAuditLog"."detailJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."detailJson" IS '事件详情 JSON';


--
-- Name: COLUMN "sysSsoAuditLog"."traceId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."traceId" IS '追踪 ID';


--
-- Name: COLUMN "sysSsoAuditLog"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoAuditLog"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuditLog"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoAuditLog_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoAuditLog_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoAuditLog_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoAuditLog_id_seq" OWNED BY "public"."sysSsoAuditLog"."id";


--
-- Name: sysSsoAuthorizationCode; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoAuthorizationCode" (
    "id" bigint NOT NULL,
    "code" character varying(255) NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "userId" bigint NOT NULL,
    "sessionId" character varying(128) NOT NULL,
    "redirectUri" character varying(512) NOT NULL,
    "scopesJson" json NOT NULL,
    "codeChallenge" character varying(255),
    "codeChallengeMethod" character varying(32),
    "nonce" character varying(255),
    "acr" character varying(64),
    "amrJson" json,
    "expiresAt" timestamp with time zone NOT NULL,
    "consumedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" json,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysSsoAuthorizationCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoAuthorizationCode" IS 'SSO 授权码表';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."code"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."code" IS '授权码';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."userId" IS '用户 ID';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."sessionId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."sessionId" IS '会话 ID';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."redirectUri"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."redirectUri" IS '回调地址';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."scopesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."scopesJson" IS '授权 scope JSON';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."codeChallenge"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."codeChallenge" IS 'PKCE code challenge';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."codeChallengeMethod"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."codeChallengeMethod" IS 'PKCE challenge 方法';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."nonce"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."nonce" IS 'OIDC nonce';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."acr"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."acr" IS '认证上下文级别';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."amrJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."amrJson" IS '认证方式 JSON';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."expiresAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."expiresAt" IS '过期时间';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."consumedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."consumedAt" IS '消费时间';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."status" IS '状态：0 ACTIVE，1 CONSUMED，2 EXPIRED';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoAuthorizationCode"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoAuthorizationCode"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoAuthorizationCode_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoAuthorizationCode_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoAuthorizationCode_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoAuthorizationCode_id_seq" OWNED BY "public"."sysSsoAuthorizationCode"."id";


--
-- Name: sysSsoClient; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoClient" (
    "id" bigint NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "clientName" character varying(128) NOT NULL,
    "clientType" character varying(32) NOT NULL,
    "clientAuthMethod" character varying(32) DEFAULT 'none'::character varying NOT NULL,
    "grantTypesJson" json NOT NULL,
    "scopesJson" json NOT NULL,
    "requirePkce" smallint DEFAULT '1'::smallint NOT NULL,
    "requireConsent" smallint DEFAULT '0'::smallint NOT NULL,
    "trustedFirstParty" smallint DEFAULT '0'::smallint NOT NULL,
    "accessTokenTtlSec" integer DEFAULT 1800 NOT NULL,
    "refreshTokenTtlSec" integer DEFAULT 2592000 NOT NULL,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" json,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updaterId" bigint,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysSsoClient"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoClient" IS 'SSO 客户端表';


--
-- Name: COLUMN "sysSsoClient"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoClient"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoClient"."clientName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."clientName" IS '客户端名称';


--
-- Name: COLUMN "sysSsoClient"."clientType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."clientType" IS '客户端类型';


--
-- Name: COLUMN "sysSsoClient"."clientAuthMethod"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."clientAuthMethod" IS '客户端认证方式';


--
-- Name: COLUMN "sysSsoClient"."grantTypesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."grantTypesJson" IS '授权模式 JSON';


--
-- Name: COLUMN "sysSsoClient"."scopesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."scopesJson" IS '允许的 scope JSON';


--
-- Name: COLUMN "sysSsoClient"."requirePkce"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."requirePkce" IS '是否强制 PKCE';


--
-- Name: COLUMN "sysSsoClient"."requireConsent"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."requireConsent" IS '是否要求 consent';


--
-- Name: COLUMN "sysSsoClient"."trustedFirstParty"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."trustedFirstParty" IS '是否为可信首方客户端';


--
-- Name: COLUMN "sysSsoClient"."accessTokenTtlSec"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."accessTokenTtlSec" IS 'Access Token 有效期秒数';


--
-- Name: COLUMN "sysSsoClient"."refreshTokenTtlSec"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."refreshTokenTtlSec" IS 'Refresh Token 有效期秒数';


--
-- Name: COLUMN "sysSsoClient"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysSsoClient"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."metadataJson" IS '客户端元数据 JSON';


--
-- Name: COLUMN "sysSsoClient"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."creatorId" IS '创建人';


--
-- Name: COLUMN "sysSsoClient"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoClient"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."updaterId" IS '更新人';


--
-- Name: COLUMN "sysSsoClient"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoClient"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClient"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoClientRedirectUri; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoClientRedirectUri" (
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


--
-- Name: TABLE "sysSsoClientRedirectUri"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoClientRedirectUri" IS 'SSO 客户端回调地址表';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."redirectUri"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."redirectUri" IS '登录回调地址';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."postLogoutRedirectUri"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."postLogoutRedirectUri" IS '登出回调地址';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."creatorId" IS '创建人';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."updaterId" IS '更新人';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoClientRedirectUri"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientRedirectUri"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoClientRedirectUri_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoClientRedirectUri_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoClientRedirectUri_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoClientRedirectUri_id_seq" OWNED BY "public"."sysSsoClientRedirectUri"."id";


--
-- Name: sysSsoClientSecret; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoClientSecret" (
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


--
-- Name: TABLE "sysSsoClientSecret"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoClientSecret" IS 'SSO 客户端密钥表';


--
-- Name: COLUMN "sysSsoClientSecret"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoClientSecret"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoClientSecret"."secretHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."secretHash" IS '客户端密钥哈希';


--
-- Name: COLUMN "sysSsoClientSecret"."secretHint"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."secretHint" IS '密钥提示';


--
-- Name: COLUMN "sysSsoClientSecret"."expiresAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."expiresAt" IS '过期时间';


--
-- Name: COLUMN "sysSsoClientSecret"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."status" IS '状态：0 ACTIVE，1 DISABLED';


--
-- Name: COLUMN "sysSsoClientSecret"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."creatorId" IS '创建人';


--
-- Name: COLUMN "sysSsoClientSecret"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoClientSecret"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."updaterId" IS '更新人';


--
-- Name: COLUMN "sysSsoClientSecret"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoClientSecret"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoClientSecret"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoClientSecret_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoClientSecret_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoClientSecret_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoClientSecret_id_seq" OWNED BY "public"."sysSsoClientSecret"."id";


--
-- Name: sysSsoClient_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoClient_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoClient_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoClient_id_seq" OWNED BY "public"."sysSsoClient"."id";


--
-- Name: sysSsoConsentGrant; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoConsentGrant" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "clientId" character varying(128) NOT NULL,
    "scopesJson" json NOT NULL,
    "grantedAt" timestamp with time zone NOT NULL,
    "revokedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" json,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysSsoConsentGrant"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoConsentGrant" IS 'SSO consent 授权表';


--
-- Name: COLUMN "sysSsoConsentGrant"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoConsentGrant"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."userId" IS '用户 ID';


--
-- Name: COLUMN "sysSsoConsentGrant"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoConsentGrant"."scopesJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."scopesJson" IS '授权 scope JSON';


--
-- Name: COLUMN "sysSsoConsentGrant"."grantedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."grantedAt" IS '授权时间';


--
-- Name: COLUMN "sysSsoConsentGrant"."revokedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."revokedAt" IS '撤销时间';


--
-- Name: COLUMN "sysSsoConsentGrant"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."status" IS '状态：0 ACTIVE，1 REVOKED';


--
-- Name: COLUMN "sysSsoConsentGrant"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysSsoConsentGrant"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoConsentGrant"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoConsentGrant"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoConsentGrant"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoConsentGrant_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoConsentGrant_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoConsentGrant_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoConsentGrant_id_seq" OWNED BY "public"."sysSsoConsentGrant"."id";


--
-- Name: sysSsoIssuerKey; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoIssuerKey" (
    "id" bigint NOT NULL,
    "kid" character varying(128) NOT NULL,
    "algorithm" character varying(32) NOT NULL,
    "publicKeyPem" "text" NOT NULL,
    "privateKeyCiphertext" "text",
    "keyStatus" character varying(32) NOT NULL,
    "activateAt" timestamp with time zone,
    "retireAt" timestamp with time zone,
    "metadataJson" json,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysSsoIssuerKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoIssuerKey" IS 'SSO issuer 密钥表';


--
-- Name: COLUMN "sysSsoIssuerKey"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoIssuerKey"."kid"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."kid" IS 'kid 标识';


--
-- Name: COLUMN "sysSsoIssuerKey"."algorithm"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."algorithm" IS '签名算法';


--
-- Name: COLUMN "sysSsoIssuerKey"."publicKeyPem"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."publicKeyPem" IS '公钥内容';


--
-- Name: COLUMN "sysSsoIssuerKey"."privateKeyCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."privateKeyCiphertext" IS '私钥密文';


--
-- Name: COLUMN "sysSsoIssuerKey"."keyStatus"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."keyStatus" IS '密钥状态：ACTIVE/NEXT/RETIRED';


--
-- Name: COLUMN "sysSsoIssuerKey"."activateAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."activateAt" IS '启用时间';


--
-- Name: COLUMN "sysSsoIssuerKey"."retireAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."retireAt" IS '退役时间';


--
-- Name: COLUMN "sysSsoIssuerKey"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysSsoIssuerKey"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoIssuerKey"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoIssuerKey"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoIssuerKey"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoIssuerKey_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoIssuerKey_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoIssuerKey_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoIssuerKey_id_seq" OWNED BY "public"."sysSsoIssuerKey"."id";


--
-- Name: sysSsoRefreshTokenFamily; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoRefreshTokenFamily" (
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
    "metadataJson" json,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysSsoRefreshTokenFamily"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoRefreshTokenFamily" IS 'SSO 刷新令牌族表';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."familyId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."familyId" IS '刷新令牌族 ID';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."sessionId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."sessionId" IS '会话 ID';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."userId" IS '用户 ID';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."currentTokenHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."currentTokenHash" IS '当前刷新令牌哈希';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."previousTokenHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."previousTokenHash" IS '上一枚刷新令牌哈希';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."reuseDetected"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."reuseDetected" IS '是否检测到 reuse';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."rotatedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."rotatedAt" IS '最近轮转时间';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."expiresAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."expiresAt" IS '过期时间';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."revokedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."revokedAt" IS '撤销时间';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."status" IS '状态：0 ACTIVE，1 REVOKED，2 EXPIRED';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoRefreshTokenFamily"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoRefreshTokenFamily"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoRefreshTokenFamily_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoRefreshTokenFamily_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoRefreshTokenFamily_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoRefreshTokenFamily_id_seq" OWNED BY "public"."sysSsoRefreshTokenFamily"."id";


--
-- Name: sysSsoSession; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sysSsoSession" (
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
    "amrJson" json,
    "loginAt" timestamp with time zone NOT NULL,
    "lastAccessAt" timestamp with time zone,
    "expiresAt" timestamp with time zone NOT NULL,
    "revokedAt" timestamp with time zone,
    "status" smallint DEFAULT '0'::smallint NOT NULL,
    "metadataJson" json,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" smallint DEFAULT '0'::smallint NOT NULL
);


--
-- Name: TABLE "sysSsoSession"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sysSsoSession" IS 'SSO 会话表';


--
-- Name: COLUMN "sysSsoSession"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."id" IS '主键标识';


--
-- Name: COLUMN "sysSsoSession"."sessionId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."sessionId" IS '会话 ID';


--
-- Name: COLUMN "sysSsoSession"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."userId" IS '用户 ID';


--
-- Name: COLUMN "sysSsoSession"."clientId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."clientId" IS '客户端标识';


--
-- Name: COLUMN "sysSsoSession"."platformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."platformCode" IS '平台编码';


--
-- Name: COLUMN "sysSsoSession"."deviceId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."deviceId" IS '设备 ID';


--
-- Name: COLUMN "sysSsoSession"."loginMethod"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."loginMethod" IS '登录方式：LOCAL/PASSWORD/PASSKEY/TOTP/EXTERNAL_OAUTH';


--
-- Name: COLUMN "sysSsoSession"."externalProviderCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."externalProviderCode" IS '外部登录提供方编码';


--
-- Name: COLUMN "sysSsoSession"."externalIdentityId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."externalIdentityId" IS '外部身份绑定 ID';


--
-- Name: COLUMN "sysSsoSession"."loginIp"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."loginIp" IS '登录 IP';


--
-- Name: COLUMN "sysSsoSession"."userAgent"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."userAgent" IS '用户代理';


--
-- Name: COLUMN "sysSsoSession"."acr"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."acr" IS '认证上下文级别';


--
-- Name: COLUMN "sysSsoSession"."amrJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."amrJson" IS '认证方式 JSON';


--
-- Name: COLUMN "sysSsoSession"."loginAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."loginAt" IS '登录时间';


--
-- Name: COLUMN "sysSsoSession"."lastAccessAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."lastAccessAt" IS '最近访问时间';


--
-- Name: COLUMN "sysSsoSession"."expiresAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."expiresAt" IS '过期时间';


--
-- Name: COLUMN "sysSsoSession"."revokedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."revokedAt" IS '撤销时间';


--
-- Name: COLUMN "sysSsoSession"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."status" IS '状态：0 ACTIVE，1 REVOKED，2 EXPIRED';


--
-- Name: COLUMN "sysSsoSession"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."metadataJson" IS '扩展元数据 JSON';


--
-- Name: COLUMN "sysSsoSession"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."createTime" IS '创建时间';


--
-- Name: COLUMN "sysSsoSession"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sysSsoSession"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sysSsoSession"."isDeleted" IS '逻辑删除标记';


--
-- Name: sysSsoSession_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sysSsoSession_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sysSsoSession_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sysSsoSession_id_seq" OWNED BY "public"."sysSsoSession"."id";


--
-- Name: sys_config; Type: TABLE; Schema: public; Owner: -
--

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
    "isDeleted" boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE "sys_config"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_config" IS '系统配置表';


--
-- Name: COLUMN "sys_config"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."id" IS '主键ID';


--
-- Name: COLUMN "sys_config"."groupId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."groupId" IS '配置分组ID';


--
-- Name: COLUMN "sys_config"."configKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."configKey" IS '配置键';


--
-- Name: COLUMN "sys_config"."configValue"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."configValue" IS '配置值';


--
-- Name: COLUMN "sys_config"."valueType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."valueType" IS '值类型';


--
-- Name: COLUMN "sys_config"."configDesc"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."configDesc" IS '配置描述';


--
-- Name: COLUMN "sys_config"."isSensitive"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."isSensitive" IS '是否敏感';


--
-- Name: COLUMN "sys_config"."isSystemConfig"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."isSystemConfig" IS '系统内部配置';


--
-- Name: COLUMN "sys_config"."requiredLogin"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."requiredLogin" IS '读取是否要求登录';


--
-- Name: COLUMN "sys_config"."isReadonly"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."isReadonly" IS '是否只读';


--
-- Name: COLUMN "sys_config"."isEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."isEnabled" IS '是否启用';


--
-- Name: COLUMN "sys_config"."effectType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."effectType" IS '生效方式';


--
-- Name: COLUMN "sys_config"."extJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."extJson" IS '扩展信息';


--
-- Name: COLUMN "sys_config"."createdBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."createdBy" IS '创建人ID';


--
-- Name: COLUMN "sys_config"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_config"."updatedBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."updatedBy" IS '更新人ID';


--
-- Name: COLUMN "sys_config"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sys_config"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config"."isDeleted" IS '是否删除';


--
-- Name: sys_config_change_log; Type: TABLE; Schema: public; Owner: -
--

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
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sys_config_change_log"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_config_change_log" IS '配置变更审计日志表';


--
-- Name: COLUMN "sys_config_change_log"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."id" IS '主键ID';


--
-- Name: COLUMN "sys_config_change_log"."configId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."configId" IS '配置ID';


--
-- Name: COLUMN "sys_config_change_log"."configKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."configKey" IS '配置键';


--
-- Name: COLUMN "sys_config_change_log"."operationType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."operationType" IS '操作类型';


--
-- Name: COLUMN "sys_config_change_log"."oldValue"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."oldValue" IS '旧值';


--
-- Name: COLUMN "sys_config_change_log"."newValue"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."newValue" IS '新值';


--
-- Name: COLUMN "sys_config_change_log"."effectType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."effectType" IS '生效方式';


--
-- Name: COLUMN "sys_config_change_log"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."status" IS '状态';


--
-- Name: COLUMN "sys_config_change_log"."parentLogId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."parentLogId" IS '父级日志ID';


--
-- Name: COLUMN "sys_config_change_log"."relatedLogId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."relatedLogId" IS '关联日志ID';


--
-- Name: COLUMN "sys_config_change_log"."operatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."operatorId" IS '操作人ID';


--
-- Name: COLUMN "sys_config_change_log"."operatorName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."operatorName" IS '操作人姓名';


--
-- Name: COLUMN "sys_config_change_log"."operationTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."operationTime" IS '操作时间';


--
-- Name: COLUMN "sys_config_change_log"."operationReason"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."operationReason" IS '操作原因';


--
-- Name: COLUMN "sys_config_change_log"."appliedBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."appliedBy" IS '应用人ID';


--
-- Name: COLUMN "sys_config_change_log"."appliedTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."appliedTime" IS '应用时间';


--
-- Name: COLUMN "sys_config_change_log"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_change_log"."createTime" IS '创建时间';


--
-- Name: sys_config_group; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_config_group"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_config_group" IS '系统配置分组表';


--
-- Name: COLUMN "sys_config_group"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."id" IS '主键ID';


--
-- Name: COLUMN "sys_config_group"."groupCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."groupCode" IS '配置分组编码';


--
-- Name: COLUMN "sys_config_group"."groupName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."groupName" IS '配置分组名称';


--
-- Name: COLUMN "sys_config_group"."module"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."module" IS '所属模块';


--
-- Name: COLUMN "sys_config_group"."permissionCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."permissionCode" IS '读取权限编码';


--
-- Name: COLUMN "sys_config_group"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."sortOrder" IS '排序顺序';


--
-- Name: COLUMN "sys_config_group"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."status" IS '状态';


--
-- Name: COLUMN "sys_config_group"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_config_group"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sys_config_group"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_config_group"."isDeleted" IS '是否删除';


--
-- Name: sys_config_group_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_config_group_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_config_group_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_config_group_id_seq" OWNED BY "public"."sys_config_group"."id";


--
-- Name: sys_config_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_config_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_config_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_config_id_seq" OWNED BY "public"."sys_config"."id";


--
-- Name: sys_dept; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_dept"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_dept" IS '系统部门表';


--
-- Name: COLUMN "sys_dept"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."id" IS '部门ID';


--
-- Name: COLUMN "sys_dept"."name"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."name" IS '部门名称';


--
-- Name: COLUMN "sys_dept"."code"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."code" IS '部门编码';


--
-- Name: COLUMN "sys_dept"."orgId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."orgId" IS '组织ID';


--
-- Name: COLUMN "sys_dept"."parentId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."parentId" IS '父部门ID';


--
-- Name: COLUMN "sys_dept"."leaderUserId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."leaderUserId" IS '负责人';


--
-- Name: COLUMN "sys_dept"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."status" IS '状态：0正常 1停用';


--
-- Name: COLUMN "sys_dept"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."sortOrder" IS '排序';


--
-- Name: COLUMN "sys_dept"."hierarchy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."hierarchy" IS '层级路径';


--
-- Name: COLUMN "sys_dept"."level"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dept"."level" IS '层级';


--
-- Name: sys_dict_item; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_dict_item" (
    "id" bigint NOT NULL,
    "dictTypeId" bigint NOT NULL,
    "itemValue" character varying(64) NOT NULL,
    "itemLabel" character varying(128) NOT NULL,
    "itemDesc" character varying(255),
    "sortOrder" integer DEFAULT 0 NOT NULL,
    "status" smallint DEFAULT '1'::smallint NOT NULL,
    "extJson" json,
    "createdBy" bigint NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updatedBy" bigint NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "isDeleted" boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE "sys_dict_item"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_dict_item" IS '字典项表';


--
-- Name: COLUMN "sys_dict_item"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."id" IS '主键ID';


--
-- Name: COLUMN "sys_dict_item"."dictTypeId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."dictTypeId" IS '字典类型ID';


--
-- Name: COLUMN "sys_dict_item"."itemValue"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."itemValue" IS '字典值';


--
-- Name: COLUMN "sys_dict_item"."itemLabel"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."itemLabel" IS '字典显示文本';


--
-- Name: COLUMN "sys_dict_item"."itemDesc"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."itemDesc" IS '字典项描述';


--
-- Name: COLUMN "sys_dict_item"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."sortOrder" IS '排序号';


--
-- Name: COLUMN "sys_dict_item"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."status" IS '状态：1-启用，0-禁用';


--
-- Name: COLUMN "sys_dict_item"."extJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."extJson" IS '扩展字段';


--
-- Name: COLUMN "sys_dict_item"."createdBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."createdBy" IS '创建人ID';


--
-- Name: COLUMN "sys_dict_item"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_dict_item"."updatedBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."updatedBy" IS '更新人ID';


--
-- Name: COLUMN "sys_dict_item"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sys_dict_item"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_item"."isDeleted" IS '是否删除';


--
-- Name: sys_dict_item_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_dict_item_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_dict_item_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_dict_item_id_seq" OWNED BY "public"."sys_dict_item"."id";


--
-- Name: sys_dict_type; Type: TABLE; Schema: public; Owner: -
--

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
    "isDeleted" boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE "sys_dict_type"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_dict_type" IS '字典类型表';


--
-- Name: COLUMN "sys_dict_type"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."id" IS '主键ID';


--
-- Name: COLUMN "sys_dict_type"."dictCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."dictCode" IS '字典类型编码';


--
-- Name: COLUMN "sys_dict_type"."dictName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."dictName" IS '字典类型名称';


--
-- Name: COLUMN "sys_dict_type"."dictDesc"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."dictDesc" IS '字典类型描述';


--
-- Name: COLUMN "sys_dict_type"."module"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."module" IS '所属模块';


--
-- Name: COLUMN "sys_dict_type"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."status" IS '状态：1-启用，0-禁用';


--
-- Name: COLUMN "sys_dict_type"."requiredLogin"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."requiredLogin" IS '读取是否要求登录';


--
-- Name: COLUMN "sys_dict_type"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."sortOrder" IS '排序规则';


--
-- Name: COLUMN "sys_dict_type"."isSystem"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."isSystem" IS '是否系统内置';


--
-- Name: COLUMN "sys_dict_type"."createdBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."createdBy" IS '创建人ID';


--
-- Name: COLUMN "sys_dict_type"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_dict_type"."updatedBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."updatedBy" IS '更新人ID';


--
-- Name: COLUMN "sys_dict_type"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sys_dict_type"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_dict_type"."isDeleted" IS '是否删除';


--
-- Name: sys_dict_type_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_dict_type_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_dict_type_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_dict_type_id_seq" OWNED BY "public"."sys_dict_type"."id";


--
-- Name: sys_file_binding_task; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_file_binding_task"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_file_binding_task" IS '文件业务绑定任务表';


--
-- Name: sys_file_chunk_upload; Type: TABLE; Schema: public; Owner: -
--

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
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: TABLE "sys_file_chunk_upload"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_file_chunk_upload" IS '文件分块上传表';


--
-- Name: COLUMN "sys_file_chunk_upload"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."id" IS '分块上传ID';


--
-- Name: COLUMN "sys_file_chunk_upload"."uploadId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."uploadId" IS '上传事务ID';


--
-- Name: COLUMN "sys_file_chunk_upload"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."userId" IS '用户ID';


--
-- Name: COLUMN "sys_file_chunk_upload"."fileName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."fileName" IS '文件名';


--
-- Name: COLUMN "sys_file_chunk_upload"."contentType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."contentType" IS 'MIME';


--
-- Name: COLUMN "sys_file_chunk_upload"."fileSize"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."fileSize" IS '文件大小';


--
-- Name: COLUMN "sys_file_chunk_upload"."chunkSize"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."chunkSize" IS '分块大小';


--
-- Name: COLUMN "sys_file_chunk_upload"."totalChunks"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."totalChunks" IS '总分块数';


--
-- Name: COLUMN "sys_file_chunk_upload"."uploadedChunks"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."uploadedChunks" IS '已上传分块JSON';


--
-- Name: COLUMN "sys_file_chunk_upload"."chunkSha256Map"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."chunkSha256Map" IS '分块SHA256映射';


--
-- Name: COLUMN "sys_file_chunk_upload"."fileSha256"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_chunk_upload"."fileSha256" IS '文件SHA256';


--
-- Name: sys_file_info; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_file_info"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_file_info" IS '文件信息表';


--
-- Name: COLUMN "sys_file_info"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_info"."id" IS '文件ID';


--
-- Name: COLUMN "sys_file_info"."fileInnerName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_info"."fileInnerName" IS '文件内部名称';


--
-- Name: COLUMN "sys_file_info"."fileSize"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_info"."fileSize" IS '文件大小';


--
-- Name: COLUMN "sys_file_info"."fileSha256"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_info"."fileSha256" IS 'SHA256';


--
-- Name: COLUMN "sys_file_info"."contentType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_info"."contentType" IS 'MIME';


--
-- Name: sys_file_integrity_audit; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_file_integrity_audit"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_file_integrity_audit" IS '文件完整性审计表';


--
-- Name: sys_file_process_run; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_file_process_run"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_file_process_run" IS '文件处理运行记录表';


--
-- Name: sys_file_process_task; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_file_process_task"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_file_process_task" IS '文件处理任务表';


--
-- Name: sys_file_reference; Type: TABLE; Schema: public; Owner: -
--

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
    "accessScope" character varying(32) DEFAULT 'OWNER_ONLY'::character varying NOT NULL
);


--
-- Name: TABLE "sys_file_reference"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_file_reference" IS '文件引用表';


--
-- Name: COLUMN "sys_file_reference"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_reference"."id" IS '主键ID';


--
-- Name: COLUMN "sys_file_reference"."fileId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_reference"."fileId" IS '文件ID';


--
-- Name: COLUMN "sys_file_reference"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_reference"."userId" IS '用户ID';


--
-- Name: COLUMN "sys_file_reference"."displayName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_reference"."displayName" IS '展示名称';


--
-- Name: COLUMN "sys_file_reference"."bizType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_reference"."bizType" IS '业务类型';


--
-- Name: COLUMN "sys_file_reference"."bizId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_file_reference"."bizId" IS '业务ID';


--
-- Name: sys_menu; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_menu"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_menu" IS '菜单权限表';


--
-- Name: COLUMN "sys_menu"."name"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_menu"."name" IS '菜单名称';


--
-- Name: COLUMN "sys_menu"."featureCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_menu"."featureCode" IS '菜单所属功能能力编码';


--
-- Name: sys_menu_permission; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_menu_permission" (
    "id" bigint NOT NULL,
    "menuId" bigint NOT NULL,
    "permissionId" bigint NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sys_menu_permission"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_menu_permission" IS '菜单权限映射表';


--
-- Name: sys_message_consume_log; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_message_consume_log" (
    "id" bigint NOT NULL,
    "messageId" character varying(128) NOT NULL,
    "consumer" character varying(64) NOT NULL,
    "status" character varying(32) NOT NULL,
    "detail" character varying(512),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sys_message_consume_log"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_message_consume_log" IS '消息消费幂等日志';


--
-- Name: sys_operation_log; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_operation_log"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_operation_log" IS '操作日志表';


--
-- Name: COLUMN "sys_operation_log"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."id" IS '操作日志ID';


--
-- Name: COLUMN "sys_operation_log"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."userId" IS '用户ID';


--
-- Name: COLUMN "sys_operation_log"."userName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."userName" IS '用户名';


--
-- Name: COLUMN "sys_operation_log"."nickName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."nickName" IS '用户昵称';


--
-- Name: COLUMN "sys_operation_log"."operationType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."operationType" IS '操作类型';


--
-- Name: COLUMN "sys_operation_log"."operationDesc"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."operationDesc" IS '操作描述';


--
-- Name: COLUMN "sys_operation_log"."methodName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."methodName" IS '方法名称';


--
-- Name: COLUMN "sys_operation_log"."requestMethod"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."requestMethod" IS '请求方法';


--
-- Name: COLUMN "sys_operation_log"."requestUrl"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."requestUrl" IS '请求地址';


--
-- Name: COLUMN "sys_operation_log"."traceId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."traceId" IS '请求链路追踪ID';


--
-- Name: COLUMN "sys_operation_log"."requestParams"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."requestParams" IS '请求参数';


--
-- Name: COLUMN "sys_operation_log"."responseResult"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."responseResult" IS '响应结果';


--
-- Name: COLUMN "sys_operation_log"."requestIp"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."requestIp" IS '请求IP';


--
-- Name: COLUMN "sys_operation_log"."requestLocation"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."requestLocation" IS '请求位置';


--
-- Name: COLUMN "sys_operation_log"."userAgent"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."userAgent" IS '用户代理';


--
-- Name: COLUMN "sys_operation_log"."browser"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."browser" IS '浏览器';


--
-- Name: COLUMN "sys_operation_log"."os"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."os" IS '操作系统';


--
-- Name: COLUMN "sys_operation_log"."operationTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."operationTime" IS '操作时间';


--
-- Name: COLUMN "sys_operation_log"."executionTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."executionTime" IS '执行耗时毫秒';


--
-- Name: COLUMN "sys_operation_log"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."status" IS '状态：1成功 0失败';


--
-- Name: COLUMN "sys_operation_log"."errorMsg"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."errorMsg" IS '错误信息';


--
-- Name: COLUMN "sys_operation_log"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_operation_log"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sys_operation_log"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_operation_log"."isDeleted" IS '是否删除';


--
-- Name: sys_operation_log_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_operation_log_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_operation_log_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_operation_log_id_seq" OWNED BY "public"."sys_operation_log"."id";


--
-- Name: sys_org; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_org"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_org" IS '系统组织表';


--
-- Name: COLUMN "sys_org"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."id" IS '组织ID';


--
-- Name: COLUMN "sys_org"."code"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."code" IS '组织编码';


--
-- Name: COLUMN "sys_org"."name"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."name" IS '组织名称';


--
-- Name: COLUMN "sys_org"."parentId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."parentId" IS '父组织ID';


--
-- Name: COLUMN "sys_org"."hierarchy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."hierarchy" IS '组织层级路径';


--
-- Name: COLUMN "sys_org"."level"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."level" IS '组织层级';


--
-- Name: COLUMN "sys_org"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."status" IS '状态：0正常 1停用';


--
-- Name: COLUMN "sys_org"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."sortOrder" IS '排序';


--
-- Name: COLUMN "sys_org"."leaderUserId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_org"."leaderUserId" IS '负责人';


--
-- Name: sys_outbox_event; Type: TABLE; Schema: public; Owner: -
--

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
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: TABLE "sys_outbox_event"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_outbox_event" IS 'Outbox事务消息';


--
-- Name: sys_permission; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_permission"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_permission" IS '权限资源表';


--
-- Name: COLUMN "sys_permission"."code"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_permission"."code" IS '权限编码';


--
-- Name: COLUMN "sys_permission"."featureCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_permission"."featureCode" IS '权限所属功能能力编码';


--
-- Name: COLUMN "sys_permission"."name"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_permission"."name" IS '权限名称';


--
-- Name: COLUMN "sys_permission"."resourceType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_permission"."resourceType" IS '资源类型';


--
-- Name: sys_post; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_post"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_post" IS '系统岗位表';


--
-- Name: COLUMN "sys_post"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."id" IS '岗位ID';


--
-- Name: COLUMN "sys_post"."code"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."code" IS '岗位编码';


--
-- Name: COLUMN "sys_post"."name"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."name" IS '岗位名称';


--
-- Name: COLUMN "sys_post"."deptId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."deptId" IS '部门ID';


--
-- Name: COLUMN "sys_post"."orgId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."orgId" IS '组织ID';


--
-- Name: COLUMN "sys_post"."hierarchy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."hierarchy" IS '岗位层级路径';


--
-- Name: COLUMN "sys_post"."level"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."level" IS '岗位层级';


--
-- Name: COLUMN "sys_post"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."sortOrder" IS '排序';


--
-- Name: COLUMN "sys_post"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."status" IS '状态：0正常 1停用';


--
-- Name: COLUMN "sys_post"."remark"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_post"."remark" IS '备注';


--
-- Name: sys_post_role; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_post_role" (
    "id" bigint NOT NULL,
    "postId" bigint NOT NULL,
    "roleId" bigint NOT NULL
);


--
-- Name: TABLE "sys_post_role"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_post_role" IS '岗位角色关系表';


--
-- Name: sys_post_role_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_post_role_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_post_role_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_post_role_id_seq" OWNED BY "public"."sys_post_role"."id";


--
-- Name: sys_role; Type: TABLE; Schema: public; Owner: -
--

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
    "isDeleted" boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE "sys_role"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_role" IS '角色信息表';


--
-- Name: COLUMN "sys_role"."name"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."name" IS '角色名称';


--
-- Name: COLUMN "sys_role"."code"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."code" IS '角色编码';


--
-- Name: COLUMN "sys_role"."systemKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."systemKey" IS '稳定系统角色标识';


--
-- Name: COLUMN "sys_role"."dataScope"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."dataScope" IS '数据范围';


--
-- Name: COLUMN "sys_role"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."status" IS '状态';


--
-- Name: COLUMN "sys_role"."type"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."type" IS '角色类型';


--
-- Name: COLUMN "sys_role"."hierarchy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."hierarchy" IS '角色层级路径';


--
-- Name: COLUMN "sys_role"."level"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."level" IS '角色层级';


--
-- Name: COLUMN "sys_role"."sortOrder"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."sortOrder" IS '排序';


--
-- Name: COLUMN "sys_role"."remark"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role"."remark" IS '备注';


--
-- Name: sys_role_config_scope; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_role_config_scope"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_role_config_scope" IS '角色配置访问范围表';


--
-- Name: COLUMN "sys_role_config_scope"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."id" IS '主键ID';


--
-- Name: COLUMN "sys_role_config_scope"."roleId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."roleId" IS '角色ID';


--
-- Name: COLUMN "sys_role_config_scope"."groupCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."groupCode" IS '配置分组编码';


--
-- Name: COLUMN "sys_role_config_scope"."configKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."configKey" IS '配置键，空表示整个分组';


--
-- Name: COLUMN "sys_role_config_scope"."canRead"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."canRead" IS '可读';


--
-- Name: COLUMN "sys_role_config_scope"."canWrite"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."canWrite" IS '可写';


--
-- Name: COLUMN "sys_role_config_scope"."canDelete"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."canDelete" IS '可删除';


--
-- Name: COLUMN "sys_role_config_scope"."createdBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."createdBy" IS '创建人ID';


--
-- Name: COLUMN "sys_role_config_scope"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_role_config_scope"."updatedBy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."updatedBy" IS '更新人ID';


--
-- Name: COLUMN "sys_role_config_scope"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sys_role_config_scope"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_config_scope"."isDeleted" IS '是否删除';


--
-- Name: sys_role_dept; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_role_dept" (
    "id" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "deptId" bigint NOT NULL
);


--
-- Name: TABLE "sys_role_dept"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_role_dept" IS '角色部门数据范围关系表';


--
-- Name: sys_role_menu; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_role_menu" (
    "id" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "menuId" bigint NOT NULL,
    "updaterId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sys_role_menu"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_role_menu" IS '角色菜单关系表';


--
-- Name: sys_role_permission; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_role_permission" (
    "id" bigint NOT NULL,
    "roleId" bigint NOT NULL,
    "permissionId" bigint NOT NULL,
    "source" character varying(16) DEFAULT 'DIRECT'::character varying NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sys_role_permission"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_role_permission" IS '角色权限关系表';


--
-- Name: COLUMN "sys_role_permission"."source"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_role_permission"."source" IS '授权来源：DIRECT/MENU/BOTH';


--
-- Name: sys_security_bootstrap; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_security_bootstrap" (
    "bootstrapKey" character varying(64) NOT NULL,
    "rootRoleId" bigint NOT NULL,
    "rootRoleCode" character varying(50) NOT NULL,
    "initializedAt" timestamp with time zone NOT NULL,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sys_security_bootstrap"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_security_bootstrap" IS '安全根初始化记录';


--
-- Name: COLUMN "sys_security_bootstrap"."bootstrapKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_security_bootstrap"."bootstrapKey" IS '初始化记录键';


--
-- Name: COLUMN "sys_security_bootstrap"."rootRoleId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_security_bootstrap"."rootRoleId" IS '授权安全根角色ID';


--
-- Name: COLUMN "sys_security_bootstrap"."rootRoleCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_security_bootstrap"."rootRoleCode" IS '初始化时固化的外部角色编码';


--
-- Name: COLUMN "sys_security_bootstrap"."initializedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_security_bootstrap"."initializedAt" IS '初始化时间';


--
-- Name: COLUMN "sys_security_bootstrap"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_security_bootstrap"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_security_bootstrap"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_security_bootstrap"."updateTime" IS '更新时间';


--
-- Name: sys_storage_alert_log; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_storage_alert_log"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_storage_alert_log" IS '存储告警日志表';


--
-- Name: sys_storage_strategy; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_storage_strategy"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_storage_strategy" IS '存储策略配置表';


--
-- Name: COLUMN "sys_storage_strategy"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."id" IS '策略ID';


--
-- Name: COLUMN "sys_storage_strategy"."strategyName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."strategyName" IS '策略名称';


--
-- Name: COLUMN "sys_storage_strategy"."providerType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."providerType" IS '存储提供商类型';


--
-- Name: COLUMN "sys_storage_strategy"."isDefault"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."isDefault" IS '是否默认';


--
-- Name: COLUMN "sys_storage_strategy"."isEnabled"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."isEnabled" IS '是否启用';


--
-- Name: COLUMN "sys_storage_strategy"."runState"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."runState" IS '运行态';


--
-- Name: COLUMN "sys_storage_strategy"."priority"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."priority" IS '优先级';


--
-- Name: COLUMN "sys_storage_strategy"."configCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."configCiphertext" IS '配置密文';


--
-- Name: COLUMN "sys_storage_strategy"."configEdek"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."configEdek" IS '加密DEK';


--
-- Name: COLUMN "sys_storage_strategy"."wrapKeyRef"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_storage_strategy"."wrapKeyRef" IS '主密钥引用';


--
-- Name: sys_upload_task; Type: TABLE; Schema: public; Owner: -
--

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
    "bindingChannel" character varying(16),
    "expireAt" timestamp with time zone,
    "userIp" character varying(64),
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    "updateTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: TABLE "sys_upload_task"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_upload_task" IS '上传任务表';


--
-- Name: sys_user; Type: TABLE; Schema: public; Owner: -
--

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
    "registerProviderCode" character varying(64)
);


--
-- Name: TABLE "sys_user"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_user" IS '系统用户表';


--
-- Name: COLUMN "sys_user"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."id" IS '用户ID';


--
-- Name: COLUMN "sys_user"."userAccount"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."userAccount" IS '账号';


--
-- Name: COLUMN "sys_user"."nickName"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."nickName" IS '昵称';


--
-- Name: COLUMN "sys_user"."password"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."password" IS '兼容 Java 用户表密码字段，Go 密码凭证写 credential 模块';


--
-- Name: COLUMN "sys_user"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."status" IS '状态：0正常 1停用/锁定';


--
-- Name: COLUMN "sys_user"."statusVersion"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."statusVersion" IS '用户状态单调版本';


--
-- Name: COLUMN "sys_user"."statusCommandHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."statusCommandHash" IS '节点状态命令哈希';


--
-- Name: COLUMN "sys_user"."registerPlatformCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."registerPlatformCode" IS '注册来源平台编码';


--
-- Name: COLUMN "sys_user"."registerProviderCode"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user"."registerProviderCode" IS '注册来源外部登录提供方编码';


--
-- Name: sys_user_credential; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_user_credential"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_user_credential" IS '用户凭证表';


--
-- Name: COLUMN "sys_user_credential"."id"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."id" IS '凭证ID';


--
-- Name: COLUMN "sys_user_credential"."userId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."userId" IS '用户ID';


--
-- Name: COLUMN "sys_user_credential"."credentialType"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."credentialType" IS '凭证类型：PASSWORD/TOTP/PASSKEY/RECOVERY_CODE';


--
-- Name: COLUMN "sys_user_credential"."credentialKey"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."credentialKey" IS '凭证键，同一类型下用于区分具体凭证';


--
-- Name: COLUMN "sys_user_credential"."secretHash"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."secretHash" IS '密钥哈希，密码和恢复码使用';


--
-- Name: COLUMN "sys_user_credential"."secretCiphertext"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."secretCiphertext" IS '密钥密文，TOTP等加密材料使用';


--
-- Name: COLUMN "sys_user_credential"."credentialPayloadJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."credentialPayloadJson" IS '凭证扩展载荷JSON';


--
-- Name: COLUMN "sys_user_credential"."status"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."status" IS '状态：0启用 1禁用 2已消费 3已失效';


--
-- Name: COLUMN "sys_user_credential"."verifiedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."verifiedAt" IS '验证时间';


--
-- Name: COLUMN "sys_user_credential"."lastUsedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."lastUsedAt" IS '最后使用时间';


--
-- Name: COLUMN "sys_user_credential"."invalidatedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."invalidatedAt" IS '失效时间';


--
-- Name: COLUMN "sys_user_credential"."metadataJson"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."metadataJson" IS '扩展元数据JSON';


--
-- Name: COLUMN "sys_user_credential"."mustChangePassword"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."mustChangePassword" IS '是否必须修改密码';


--
-- Name: COLUMN "sys_user_credential"."passwordChangedAt"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."passwordChangedAt" IS '密码修改时间';


--
-- Name: COLUMN "sys_user_credential"."creatorId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."creatorId" IS '创建人ID';


--
-- Name: COLUMN "sys_user_credential"."createTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."createTime" IS '创建时间';


--
-- Name: COLUMN "sys_user_credential"."updaterId"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."updaterId" IS '更新人ID';


--
-- Name: COLUMN "sys_user_credential"."updateTime"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."updateTime" IS '更新时间';


--
-- Name: COLUMN "sys_user_credential"."isDeleted"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN "public"."sys_user_credential"."isDeleted" IS '是否删除';


--
-- Name: sys_user_dept; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE "public"."sys_user_dept" (
    "id" bigint NOT NULL,
    "userId" bigint NOT NULL,
    "deptId" bigint NOT NULL,
    "isPrimary" boolean DEFAULT false NOT NULL,
    "creatorId" bigint,
    "createTime" timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE "sys_user_dept"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_user_dept" IS '用户部门关系表';


--
-- Name: sys_user_dept_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_user_dept_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_user_dept_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_user_dept_id_seq" OWNED BY "public"."sys_user_dept"."id";


--
-- Name: sys_user_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_user_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_user_id_seq" OWNED BY "public"."sys_user"."id";


--
-- Name: sys_user_org; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_user_org"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_user_org" IS '用户组织关系表';


--
-- Name: sys_user_org_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_user_org_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_user_org_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_user_org_id_seq" OWNED BY "public"."sys_user_org"."id";


--
-- Name: sys_user_permission; Type: TABLE; Schema: public; Owner: -
--

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
    "isDeleted" boolean DEFAULT false NOT NULL
);


--
-- Name: TABLE "sys_user_permission"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_user_permission" IS '用户临时权限表';


--
-- Name: sys_user_permission_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_user_permission_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_user_permission_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_user_permission_id_seq" OWNED BY "public"."sys_user_permission"."id";


--
-- Name: sys_user_position; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_user_position"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_user_position" IS '用户岗位关系表';


--
-- Name: sys_user_position_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_user_position_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_user_position_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_user_position_id_seq" OWNED BY "public"."sys_user_position"."id";


--
-- Name: sys_user_role; Type: TABLE; Schema: public; Owner: -
--

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


--
-- Name: TABLE "sys_user_role"; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE "public"."sys_user_role" IS '用户角色关系表';


--
-- Name: sys_user_role_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE "public"."sys_user_role_id_seq"
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: sys_user_role_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE "public"."sys_user_role_id_seq" OWNED BY "public"."sys_user_role"."id";


--
-- Name: sysExternalLoginProvider id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalLoginProvider" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalLoginProvider_id_seq"'::"regclass");


--
-- Name: sysExternalOAuthLoginState id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalOAuthLoginState" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalOAuthLoginState_id_seq"'::"regclass");


--
-- Name: sysExternalOAuthToken id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalOAuthToken" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalOAuthToken_id_seq"'::"regclass");


--
-- Name: sysExternalProviderMethod id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalProviderMethod" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalProviderMethod_id_seq"'::"regclass");


--
-- Name: sysExternalUserIdentity id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalUserIdentity" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysExternalUserIdentity_id_seq"'::"regclass");


--
-- Name: sysPlatform id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatform" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatform_id_seq"'::"regclass");


--
-- Name: sysPlatformDefaultRole id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformDefaultRole" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformDefaultRole_id_seq"'::"regclass");


--
-- Name: sysPlatformLoginMethod id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformLoginMethod" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformLoginMethod_id_seq"'::"regclass");


--
-- Name: sysPlatformSourceRule id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformSourceRule" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformSourceRule_id_seq"'::"regclass");


--
-- Name: sysPlatformSsoClient id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformSsoClient" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysPlatformSsoClient_id_seq"'::"regclass");


--
-- Name: sysSsoAuditLog id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoAuditLog" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoAuditLog_id_seq"'::"regclass");


--
-- Name: sysSsoAuthorizationCode id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoAuthorizationCode" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoAuthorizationCode_id_seq"'::"regclass");


--
-- Name: sysSsoClient id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoClient" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoClient_id_seq"'::"regclass");


--
-- Name: sysSsoClientRedirectUri id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoClientRedirectUri" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoClientRedirectUri_id_seq"'::"regclass");


--
-- Name: sysSsoClientSecret id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoClientSecret" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoClientSecret_id_seq"'::"regclass");


--
-- Name: sysSsoConsentGrant id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoConsentGrant" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoConsentGrant_id_seq"'::"regclass");


--
-- Name: sysSsoIssuerKey id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoIssuerKey" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoIssuerKey_id_seq"'::"regclass");


--
-- Name: sysSsoRefreshTokenFamily id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoRefreshTokenFamily" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoRefreshTokenFamily_id_seq"'::"regclass");


--
-- Name: sysSsoSession id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoSession" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sysSsoSession_id_seq"'::"regclass");


--
-- Name: sys_config id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_config" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_config_id_seq"'::"regclass");


--
-- Name: sys_config_group id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_config_group" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_config_group_id_seq"'::"regclass");


--
-- Name: sys_dict_item id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_dict_item" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_dict_item_id_seq"'::"regclass");


--
-- Name: sys_dict_type id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_dict_type" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_dict_type_id_seq"'::"regclass");


--
-- Name: sys_operation_log id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_operation_log" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_operation_log_id_seq"'::"regclass");


--
-- Name: sys_post_role id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_post_role" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_post_role_id_seq"'::"regclass");


--
-- Name: sys_user id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_id_seq"'::"regclass");


--
-- Name: sys_user_dept id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_dept" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_dept_id_seq"'::"regclass");


--
-- Name: sys_user_org id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_org" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_org_id_seq"'::"regclass");


--
-- Name: sys_user_permission id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_permission" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_permission_id_seq"'::"regclass");


--
-- Name: sys_user_position id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_position" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_position_id_seq"'::"regclass");


--
-- Name: sys_user_role id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_role" ALTER COLUMN "id" SET DEFAULT "nextval"('"public"."sys_user_role_id_seq"'::"regclass");


--
-- Data for Name: docker_compose_project; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: docker_operation; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: docker_operation_event; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: docker_remote_registry; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysExternalLoginProvider; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysExternalManagedProviderCommand; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysExternalOAuthLoginState; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysExternalOAuthToken; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysExternalProviderMethod; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysExternalUserIdentity; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysFederatedNode; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysFederatedNodeConnectionCommand; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysNotificationChannel; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysNotificationChannel" VALUES (2026062510000000001, 'mock-default', '默认 Mock 通知渠道', 'MOCK', 0, 10, '{"capturePrefix": "notification:mock:capture"}', NULL, NULL, NULL, NULL, '{"seed": "notification-center-v1"}', 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysNotificationDelivery; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysNotificationSceneBinding; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysNotificationSceneBinding" VALUES (2026062510000000201, 'CHALLENGE_OTP', '安全验证码', 'mock-default', 'challenge_otp_mock_zh_cn', 1, 10, 3, 60, NULL, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysNotificationTemplate; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysNotificationTemplate" VALUES (2026062510000000101, 'challenge_otp_mock_zh_cn', '安全验证码 Mock 模板', 'CHALLENGE_OTP', 'MOCK', 'zh-CN', '【{{.AppName}}】-{{.SceneName}}', '您的验证码是 {{.Code}}，{{.TTLMinutes}} 分钟内有效。', '<p>您的验证码是 <strong>{{.Code}}</strong>，{{.TTLMinutes}} 分钟内有效。</p>', NULL, NULL, '["AppName", "SceneName", "Code", "TTLMinutes", "ToEmail"]', 0, 1, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);
INSERT INTO "public"."sysNotificationTemplate" VALUES (2026062510000000102, 'challenge_otp_email_zh_cn', '安全验证码 Email 模板', 'CHALLENGE_OTP', 'EMAIL', 'zh-CN', '【{{.AppName}}】-{{.SceneName}}', '您的验证码是 {{.Code}}，{{.TTLMinutes}} 分钟内有效。', '<p>您的验证码是 <strong>{{.Code}}</strong>，{{.TTLMinutes}} 分钟内有效。</p>', NULL, NULL, '["AppName", "SceneName", "Code", "TTLMinutes", "ToEmail"]', 0, 1, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysPlatform; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysPlatform" VALUES (1, 'seven-admin', 'Seven 管理后台', 'ADMIN', 'Seven 默认管理后台平台', 'http://127.0.0.1:5291/', 0, 0, 1, NULL, '{"theme": "blue-cyan", "title": "Seven", "subtitle": "统一身份认证系统"}', '{"seed": "platform-management-v1"}', 0, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysPlatformDefaultRole; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysPlatformLoginMethod; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysPlatformLoginMethod" VALUES (1, 'seven-admin', 'PASSWORD', '', '账号密码登录', 'LockOutlined', 10, 1, 1, NULL, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);
INSERT INTO "public"."sysPlatformLoginMethod" VALUES (2, 'seven-admin', 'PASSKEY', '', '通行密钥', 'KeyOutlined', 20, 1, 1, NULL, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);
INSERT INTO "public"."sysPlatformLoginMethod" VALUES (3, 'seven-admin', 'EXTERNAL_OAUTH', 'github', 'GitHub', 'GithubOutlined', 30, 1, 1, NULL, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);
INSERT INTO "public"."sysPlatformLoginMethod" VALUES (4, 'seven-admin', 'EXTERNAL_OAUTH', 'google', 'Google', 'GoogleOutlined', 40, 1, 1, NULL, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysPlatformSourceRule; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysPlatformSourceRule" VALUES (1, 'seven-admin', 'CLIENT_ID', 'authorization-console', 100, 0, NULL, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);
INSERT INTO "public"."sysPlatformSourceRule" VALUES (2, 'seven-admin', 'HOST', '127.0.0.1:5291', 50, 0, NULL, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysPlatformSsoClient; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysPlatformSsoClient" VALUES (1, 'seven-admin', 'authorization-console', 0, 0, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysSsoAuditLog; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysSsoAuthorizationCode; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysSsoClient; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysSsoClient" VALUES (1, 'authorization-console', 'Authorization Console', 'PUBLIC', 'none', '["authorization_code", "refresh_token"]', '["openid", "profile", "email", "offline_access"]', 1, 0, 1, 1800, 2592000, 0, '{"seed": "sso-provider-v1"}', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysSsoClientRedirectUri; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sysSsoClientRedirectUri" VALUES (1, 'authorization-console', 'http://127.0.0.1:5291/oidc/callback/authorization-console', NULL, 0, 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', 0);


--
-- Data for Name: sysSsoClientSecret; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysSsoConsentGrant; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysSsoIssuerKey; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysSsoRefreshTokenFamily; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sysSsoSession; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_config; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_config_change_log; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_config_group; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_dept; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_dict_item; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_dict_item" VALUES (1, 2026042501001, '0', '未知', '性别-未知', 0, 1, '{"icon": "unknown", "color": "gray"}', 1, '2026-07-19 00:39:00+08', 1, '2026-07-19 00:39:32+08', false);
INSERT INTO "public"."sys_dict_item" VALUES (2, 2026042501001, '1', '男', '性别-男', 1, 1, '{"icon": "male", "color": "blue"}', 1, '2026-07-19 00:39:00+08', 1, '2026-07-19 00:39:32+08', false);
INSERT INTO "public"."sys_dict_item" VALUES (3, 2026042501001, '2', '女', '性别-女', 2, 1, '{"icon": "female", "color": "pink"}', 1, '2026-07-19 00:39:00+08', 1, '2026-07-19 00:39:32+08', false);


--
-- Data for Name: sys_dict_type; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_dict_type" VALUES (2026042501001, 'gender', '性别', '用户性别字典', 'user', 1, false, 0, 1, 1, '2026-07-19 00:39:00+08', 1, '2026-07-19 00:39:32+08', false);


--
-- Data for Name: sys_file_binding_task; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_file_chunk_upload; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_file_info; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_file_integrity_audit; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_file_process_run; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_file_process_task; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_file_reference; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_menu; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_menu" VALUES (1900300200, '系统管理', 0, 100, '/system', NULL, 'SettingOutlined', 'M', NULL, NULL, false, false, true, '/1900300200', 1, 0, 'RBAC admin seed parent menu', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (1900300201, '角色管理', 2012232900000000002, 10, '/system/role', '/system/role', 'TeamOutlined', 'C', 'system:role:list', NULL, false, false, true, '/1/1900300200/2012232900000000002/1900300201', 3, 0, '角色管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (1900300202, '菜单管理', 2012232900000000002, 20, '/system/menu', '/system/menu', 'MenuOutlined', 'C', 'system:menu:list', NULL, false, false, true, '/1/1900300200/2012232900000000002/1900300202', 3, 0, '菜单管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (1900300203, '权限资源', 2012232900000000002, 30, '/system/permission', '/system/permission', 'SafetyOutlined', 'C', 'system:permission:list', NULL, false, false, true, '/1/1900300200/2012232900000000002/1900300203', 3, 0, '权限资源管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232069056864258, '字典管理', 2012232900000000004, 20, '/system/dict', 'system/dict/index', 'SettingOutlined', 'C', 'system:dict:query', NULL, false, false, true, '/1/1900300200/2012232900000000004/2012232069056864258', 3, 0, '字典管理', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232069056864259, '在线用户', 2012232900000000003, 30, '/system/online-user', 'system/online-user/index', 'GlobalOutlined', 'C', 'admin:online:view', NULL, false, false, true, '/1/1900300200/2012232900000000003/2012232069056864259', 3, 0, '在线用户', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232069056864260, '应用运行日志', 2012232900000000003, 40, '/system/runtime-log', 'system/runtime-log/index', 'FileTextOutlined', 'C', 'admin:runtime-log:view', NULL, false, false, true, '/1/1900300200/2012232900000000003/2012232069056864260', 3, 0, '应用运行日志', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232069056864261, '文件列表', 2012232900000000004, 30, '/system/files', 'system/files/index', 'FileTextOutlined', 'C', 'system:file:list', NULL, false, false, true, '/1/1900300200/2012232900000000004/2012232069056864261', 3, 0, '文件列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232069056864262, '文件任务', 2012232900000000004, 40, '/system/file-tasks', 'system/file-tasks/index', 'FileTextOutlined', 'C', 'system:file-task:list', NULL, false, false, true, '/1/1900300200/2012232900000000004/2012232069056864262', 3, 0, '文件任务', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232069056864263, '存储策略', 2012232900000000004, 50, '/system/storage', 'system/storage/index', 'FileTextOutlined', 'C', 'system:storage:list', NULL, false, false, true, '/1/1900300200/2012232900000000004/2012232069056864263', 3, 0, '存储策略', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232407482671109, 'Docker 运维', 2012232900000000003, 10, '/system/docker', 'system/docker/index', 'ClusterOutlined', 'C', 'admin:docker:container:list', 'docker.admin', false, false, true, '/1/1900300200/2012232900000000003/2012232407482671109', 3, 0, 'Docker运维工作台', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232800000000001, '安全中心', 1, 115, '/system/security', 'Layout', 'SafetyOutlined', 'M', NULL, NULL, true, true, false, '/1/2012232800000000001', 2, 1, '系统安全管理分组', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', true);
INSERT INTO "public"."sys_menu" VALUES (2012232800000000002, 'OAuth 客户端', 2012232900000000002, 40, '/system/sso-client', 'system/sso-client/index', 'SafetyOutlined', 'C', 'system:sso-client:list', NULL, true, true, true, '/1/1900300200/2012232900000000002/2012232800000000002', 3, 0, 'OIDC客户端管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232900000000001, '身份与组织', 1900300200, 10, '/system/identity', 'Layout', 'TeamOutlined', 'M', NULL, NULL, true, true, true, '/1/1900300200/2012232900000000001', 2, 0, '用户、组织、部门、岗位管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232900000000002, '权限与认证', 1900300200, 20, '/system/access', 'Layout', 'SafetyOutlined', 'M', NULL, NULL, true, true, true, '/1/1900300200/2012232900000000002', 2, 0, '角色、权限、菜单与 OAuth 客户端管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232900000000003, '平台运维', 1900300200, 30, '/system/ops', 'Layout', 'ClusterOutlined', 'M', NULL, NULL, true, true, true, '/1/1900300200/2012232900000000003', 2, 0, 'Docker、可观测性、在线会话与运行日志', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232900000000004, '配置与内容', 1900300200, 40, '/system/settings', 'Layout', 'SettingOutlined', 'M', NULL, NULL, true, true, true, '/1/1900300200/2012232900000000004', 2, 0, '系统配置、字典、文件与存储策略', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232900000000005, '审计与日志', 1900300200, 50, '/system/audit', 'Layout', 'FileTextOutlined', 'M', NULL, NULL, true, true, true, '/1/1900300200/2012232900000000005', 2, 0, '操作审计入口', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012232900000000101, '组织架构', 2012232900000000001, 20, '/system/organization-management', 'system/organization-management/index', 'ApartmentOutlined', 'C', 'system:org:list', NULL, true, true, true, '/1/1900300200/2012232900000000001/2012232900000000101', 3, 0, '组织、部门、岗位聚合管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233000000000001, '外部登录', 2012232900000000002, 50, '/system/external-login-provider', 'system/external-login-provider/index', 'SafetyOutlined', 'C', 'system:external-login-provider:list', NULL, true, true, true, '/1/1900300200/2012232900000000002/2012233000000000001', 3, 0, '外部登录提供方管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233100000000001, '配置管理', 2012232900000000004, 10, '/system/config', 'system/config/index', 'SettingOutlined', 'C', 'system:config:query', NULL, true, true, true, '/1/1900300200/2012232900000000004/2012233100000000001', 3, 0, '配置管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233100000000002, '部门管理', 2012232900000000001, 40, '/system/department', 'system/department/index', 'BankOutlined', 'C', 'system:dept:list', NULL, true, true, true, '/1/1900300200/2012232900000000001/2012233100000000002', 3, 0, '部门管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233100000000003, '可观测性中心', 2012232900000000003, 20, '/system/observability', 'system/observability/index', 'RadarChartOutlined', 'C', 'admin:observability:view', NULL, true, true, true, '/1/1900300200/2012232900000000003/2012233100000000003', 3, 0, '可观测性中心', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233100000000004, '操作审计', 2012232900000000005, 10, '/system/operation-log', 'system/operation-log/index', 'FileTextOutlined', 'C', 'admin:log:view', NULL, true, true, true, '/1/1900300200/2012232900000000005/2012233100000000004', 3, 0, '操作审计', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233100000000005, '组织管理', 2012232900000000001, 30, '/system/organization', 'system/organization/index', 'ApartmentOutlined', 'C', 'system:org:list', NULL, true, true, true, '/1/1900300200/2012232900000000001/2012233100000000005', 3, 0, '组织管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233100000000006, '岗位管理', 2012232900000000001, 50, '/system/post', 'system/post/index', 'IdcardOutlined', 'C', 'system:post:list', NULL, true, true, true, '/1/1900300200/2012232900000000001/2012233100000000006', 3, 0, '岗位管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233100000000007, '用户管理', 2012232900000000001, 10, '/system/user', 'system/user/index', 'UserOutlined', 'C', 'system:user:list', NULL, true, true, true, '/1/1900300200/2012232900000000001/2012233100000000007', 3, 0, '用户管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2012233200000000001, '平台管理', 2012232900000000002, 60, '/system/platform', 'system/platform/index', 'AppstoreOutlined', 'C', 'system:platform:list', 'platform.control', true, true, true, '/1/1900300200/2012232900000000002/2012233200000000001', 3, 0, '平台入口、登录策略与默认权限管理', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2026062510000002001, '通知中心', 2012232900000000002, 70, '/system/notification', 'system/notification/index', 'NotificationOutlined', 'C', 'system:notification:channel:list', NULL, true, true, true, '/1/1900300200/2012232900000000002/2026062510000002001', 3, 0, '通知渠道、模板、场景与投递日志', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_menu" VALUES (2026062510000002002, 'Hub节点管理', 2012232900000000002, 70, '/system/hub-node', 'system/hub-node/index', 'DeploymentUnitOutlined', 'C', 'system:hub-node:list', 'federation.hub', true, true, true, '/1/1900300200/2012232900000000002/2026062510000002002', 3, 0, '管理联邦Node连接、用户状态、会话和登录策略', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);


--
-- Data for Name: sys_menu_permission; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_menu_permission" VALUES (193830630302, 1900300201, 1900300101, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630303, 1900300201, 1900300102, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630304, 1900300201, 1900300103, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630305, 1900300201, 1900300104, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630306, 1900300201, 1900300105, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630307, 1900300201, 1900300106, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630313, 1900300202, 1900300111, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630314, 1900300202, 1900300112, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630315, 1900300202, 1900300113, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630316, 1900300202, 1900300114, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630317, 1900300202, 1900300115, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630323, 1900300202, 1900300121, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630324, 1900300203, 1900300121, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630325, 1900300202, 1900300123, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630326, 1900300203, 1900300123, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630327, 1900300203, 1900300124, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630328, 1900300203, 1900300125, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630333, 1900300202, 1900300131, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (193830630334, 1900300202, 1900300132, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000101, 2012232407482671109, 1900100001, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000102, 2012232407482671109, 1900100002, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000103, 2012232407482671109, 1900100003, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000104, 2012232407482671109, 1900100004, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000105, 2012232407482671109, 1900100005, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000106, 2012232407482671109, 1900100006, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000107, 2012232407482671109, 1900100007, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000108, 2012232407482671109, 1900100008, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000109, 2012232407482671109, 1900100011, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000110, 2012232407482671109, 1900100012, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000111, 2012232407482671109, 1900100013, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000112, 2012232407482671109, 1900100014, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000113, 2012232407482671109, 1900100015, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000114, 2012232407482671109, 1900100016, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000115, 2012232407482671109, 1900100017, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000116, 2012232407482671109, 1900100018, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000117, 2012232407482671109, 1900100021, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000118, 2012232407482671109, 1900100022, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000119, 2012232407482671109, 1900100023, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000120, 2012232407482671109, 1900100024, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000121, 2012232407482671109, 1900100031, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000122, 2012232407482671109, 1900100032, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000123, 2012232407482671109, 1900100033, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000124, 2012232407482671109, 1900100034, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000125, 2012232407482671109, 1900100035, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000126, 2012232407482671109, 1900100036, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000127, 2012232407482671109, 1900100037, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000128, 2012232407482671109, 1900100038, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000129, 2012232407482671109, 1900100039, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000130, 2012232407482671109, 1900100040, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000131, 2012232407482671109, 1900100041, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000132, 2012232407482671109, 1900100042, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000133, 2012232407482671109, 1900100043, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000134, 2012232407482671109, 1900100044, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000135, 2012232407482671109, 1900100045, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000136, 2012232407482671109, 1900100046, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000137, 2012232407482671109, 1900100047, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000138, 2012232407482671109, 1900100501, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000139, 2012232407482671109, 1900100502, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000140, 2012232407482671109, 1900100503, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000141, 2012232407482671109, 1900100504, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000142, 2012232407482671109, 1900100505, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000143, 2012232407482671109, 1900100506, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000144, 2012232407482671109, 1900100507, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000145, 2012232407482671109, 1900100508, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000146, 2012232407482671109, 1900100509, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000147, 2012232407482671109, 1900100510, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000148, 2012232407482671109, 1900100511, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000149, 2012232407482671109, 1900100512, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000150, 2012232407482671109, 1900100513, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000151, 2012232407482671109, 1900100514, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000152, 2012232407482671109, 1900100515, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000153, 2012232407482671109, 1900100516, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000154, 2012232407482671109, 1900100517, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000155, 2012232407482671109, 1900100518, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232600000000156, 2012232407482671109, 1900100519, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000101, 2012232800000000002, 1900301101, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000102, 2012232800000000002, 1900301102, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000103, 2012232800000000002, 1900301103, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000104, 2012232800000000002, 1900301104, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000105, 2012232800000000002, 1900301105, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000106, 2012232800000000002, 1900301106, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000107, 2012232800000000002, 1900301107, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000108, 2012232800000000002, 1900301108, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000109, 2012232800000000002, 1900301109, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012232800000000110, 2012232800000000002, 1900301110, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000101, 2012233000000000001, 1900301201, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000102, 2012233000000000001, 1900301202, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000103, 2012233000000000001, 1900301203, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000104, 2012233000000000001, 1900301204, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000105, 2012233000000000001, 1900301205, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000106, 2012233000000000001, 1900301206, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000107, 2012233000000000001, 1900301207, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000108, 2012233000000000001, 1900301208, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000109, 2012233000000000001, 1900301209, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233000000000110, 2012233000000000001, 1900301210, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000101, 2012233200000000001, 1900301301, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000102, 2012233200000000001, 1900301302, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000103, 2012233200000000001, 1900301303, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000104, 2012233200000000001, 1900301304, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000105, 2012233200000000001, 1900301305, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000106, 2012233200000000001, 1900301306, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000107, 2012233200000000001, 1900301307, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2012233200000000108, 2012233200000000001, 1900301308, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003001, 2026062510000002001, 2026062510000001001, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003002, 2026062510000002001, 2026062510000001002, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003003, 2026062510000002001, 2026062510000001003, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003004, 2026062510000002001, 2026062510000001004, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003005, 2026062510000002001, 2026062510000001005, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003006, 2026062510000002001, 2026062510000001006, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003007, 2026062510000002001, 2026062510000001007, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003008, 2026062510000002001, 2026062510000001008, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003009, 2026062510000002002, 2026062510000001009, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003010, 2026062510000002002, 2026062510000001010, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003011, 2026062510000002002, 2026062510000001011, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003012, 2026062510000002002, 2026062510000001012, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003013, 2026062510000002002, 2026062510000001013, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003014, 2026062510000002002, 2026062510000001014, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003015, 2026062510000002002, 2026062510000001015, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003016, 2026062510000002002, 2026062510000001016, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003017, 2026062510000002002, 2026062510000001017, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003018, 2026062510000002002, 2026062510000001018, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003019, 2026062510000002002, 2026062510000001019, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003020, 2026062510000002002, 2026062510000001020, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003021, 2026062510000002002, 2026062510000001021, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003022, 2026062510000002002, 2026062510000001022, 0, '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_menu_permission" VALUES (2026062510000003023, 2026062510000002002, 2026062510000001023, 0, '2026-07-19 00:38:59+08');


--
-- Data for Name: sys_message_consume_log; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_operation_log; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_org; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_outbox_event; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_permission; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_permission" VALUES (1900100001, 'admin:docker:container:list', 'docker.admin', 'Docker容器列表', 'API', 'GET', '/admin/docker/containers', 0, '查询Docker容器列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100002, 'admin:docker:container:query', 'docker.admin', 'Docker容器详情', 'API', 'GET', '/admin/docker/containers/{id}', 0, '查询Docker容器详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100003, 'admin:docker:container:logs', 'docker.admin', 'Docker容器日志', 'API', 'GET', '/admin/docker/containers/{id}/logs', 0, '读取Docker容器日志', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100004, 'admin:docker:container:start', 'docker.admin', '启动Docker容器', 'API', 'POST', '/admin/docker/containers/{id}/start', 0, '启动Docker容器', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100005, 'admin:docker:container:stop', 'docker.admin', '停止Docker容器', 'API', 'POST', '/admin/docker/containers/{id}/stop', 0, '停止Docker容器', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100006, 'admin:docker:container:restart', 'docker.admin', '重启Docker容器', 'API', 'POST', '/admin/docker/containers/{id}/restart', 0, '重启Docker容器', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100007, 'admin:docker:container:delete', 'docker.admin', '删除Docker容器', 'API', 'DELETE', '/admin/docker/containers/{id}', 0, '删除Docker容器', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100008, 'admin:docker:container:create', 'docker.admin', '创建Docker容器', 'API', 'POST', '/admin/docker/containers/create-from-image', 0, '从镜像创建Docker容器', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100011, 'admin:docker:image:list', 'docker.admin', 'Docker镜像列表', 'API', 'GET', '/admin/docker/images/local', 0, '查询Docker本地镜像列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100012, 'admin:docker:image:query', 'docker.admin', 'Docker镜像详情', 'API', 'GET', '/admin/docker/images/local/{id}', 0, '查询Docker镜像详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100013, 'admin:docker:image:containers', 'docker.admin', 'Docker镜像关联容器', 'API', 'GET', '/admin/docker/images/local/{id}/containers', 0, '查询Docker镜像关联容器', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100014, 'admin:docker:image:pull', 'docker.admin', '拉取Docker镜像', 'API', 'POST', '/admin/docker/images/pull', 0, '拉取Docker镜像', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100015, 'admin:docker:image:tag', 'docker.admin', '标记Docker镜像', 'API', 'POST', '/admin/docker/images/tag', 0, '标记Docker镜像', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100016, 'admin:docker:image:push', 'docker.admin', '推送Docker镜像', 'API', 'POST', '/admin/docker/images/push', 0, '推送Docker镜像', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100017, 'admin:docker:image:delete', 'docker.admin', '删除Docker镜像', 'API', 'DELETE', '/admin/docker/images/local/{id}', 0, '删除Docker镜像', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100018, 'admin:docker:image:startup-preview', 'docker.admin', 'Docker镜像启动预览', 'API', 'POST', '/admin/docker/images/local/{id}/startup-preview', 0, '预览Docker镜像启动参数', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100021, 'admin:docker:registry:list', 'docker.admin', 'Docker Registry列表', 'API', 'GET', '/admin/docker/registries', 0, '查询Docker Registry列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100022, 'admin:docker:registry:create', 'docker.admin', '创建Docker Registry', 'API', 'POST', '/admin/docker/registries', 0, '创建Docker Registry', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100023, 'admin:docker:registry:update', 'docker.admin', '更新Docker Registry', 'API', 'PUT', '/admin/docker/registries/{id}', 0, '更新Docker Registry', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100024, 'admin:docker:registry:test', 'docker.admin', '测试Docker Registry', 'API', 'POST', '/admin/docker/registries/{id}/test', 0, '测试Docker Registry连接', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100031, 'admin:docker:compose:validate', 'docker.admin', '校验Docker Compose', 'API', 'POST', '/admin/docker/compose/validate', 0, '校验Docker Compose YAML', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100032, 'admin:docker:compose:up', 'docker.admin', '执行Docker Compose', 'API', 'POST', '/admin/docker/compose/up', 0, '执行Docker Compose Up', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100033, 'admin:docker:compose:project:list', 'docker.admin', 'Docker Compose项目列表', 'API', 'GET', '/admin/docker/compose/projects', 0, '查询Docker Compose项目列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100034, 'admin:docker:compose:project:query', 'docker.admin', 'Docker Compose项目详情', 'API', 'GET', '/admin/docker/compose/projects/{id}', 0, '查询Docker Compose项目详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100035, 'admin:docker:compose:project:save', 'docker.admin', '保存Docker Compose项目', 'API', 'POST', '/admin/docker/compose/projects', 0, '创建或更新Docker Compose项目', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100036, 'admin:docker:compose:workspace:check', 'docker.admin', '检查Docker Compose工作目录', 'API', 'POST', '/admin/docker/compose/workspace/check', 0, '检查Docker Compose工作目录', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100037, 'admin:docker:compose:yaml:validate', 'docker.admin', '校验Docker Compose YAML', 'API', 'POST', '/admin/docker/compose/yaml/validate', 0, '校验Docker Compose YAML并返回解析结果', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100038, 'admin:docker:compose:dockerfile:preview', 'docker.admin', '预览Dockerfile构建', 'API', 'POST', '/admin/docker/compose/dockerfile/preview', 0, '预览Dockerfile构建配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100039, 'admin:docker:compose:project:create', 'docker.admin', '创建Docker Compose项目', 'API', 'POST', '/admin/docker/compose/projects', 0, '创建Docker Compose项目', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100040, 'admin:docker:compose:project:update', 'docker.admin', '更新Docker Compose项目', 'API', 'PUT', '/admin/docker/compose/projects/{id}/compose', 0, '更新Docker Compose项目配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100041, 'admin:docker:operation:list', 'docker.admin', 'Docker操作列表', 'API', 'GET', '/admin/docker/operations', 0, '查询Docker异步操作列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100042, 'admin:docker:operation:query', 'docker.admin', 'Docker操作详情', 'API', 'GET', '/admin/docker/operations/{id}', 0, '查询Docker异步操作详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100043, 'admin:docker:operation:stream', 'docker.admin', 'Docker操作事件流', 'API', 'GET', '/admin/docker/operations/{id}/stream', 0, '订阅Docker异步操作事件', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100044, 'admin:docker:operation:cancel', 'docker.admin', '取消Docker操作', 'API', 'POST', '/admin/docker/operations/{id}/cancel', 0, '取消Docker异步操作', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100045, 'admin:docker:operation:retry', 'docker.admin', '重试Docker操作', 'API', 'POST', '/admin/docker/operations/{id}/retry', 0, '重试Docker异步操作', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100046, 'admin:docker:dangerous', 'docker.admin', 'Docker高危操作', 'API', '*', '/admin/docker/**', 0, '允许执行Docker高危操作', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100047, 'admin:docker:policy:override', 'docker.admin', 'Docker策略覆盖', 'API', '*', '/admin/docker/**', 0, '允许覆盖Docker locked-down策略', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100501, 'admin:docker:container:terminal', 'docker.admin', 'Docker容器终端', 'API', 'GET', '/admin/docker/containers/{id}/terminal', 0, '打开Docker容器终端', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100502, 'admin:docker:volume:list', 'docker.admin', 'Docker Volume列表', 'API', 'GET', '/admin/docker/volumes', 0, '查询Docker Volume列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100503, 'admin:docker:volume:create', 'docker.admin', '创建Docker Volume', 'API', 'POST', '/admin/docker/volumes', 0, '创建Docker Volume', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100504, 'admin:docker:volume:prune', 'docker.admin', '清理Docker Volume', 'API', 'POST', '/admin/docker/volumes/prune/*', 0, '清理未使用Docker Volume', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100505, 'admin:docker:volume:query', 'docker.admin', 'Docker Volume详情', 'API', 'GET', '/admin/docker/volumes/{name}', 0, '查询Docker Volume详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100506, 'admin:docker:volume:delete', 'docker.admin', '删除Docker Volume', 'API', 'DELETE', '/admin/docker/volumes/{name}', 0, '删除Docker Volume', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100507, 'admin:docker:network:list', 'docker.admin', 'Docker Network列表', 'API', 'GET', '/admin/docker/networks', 0, '查询Docker Network列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100508, 'admin:docker:network:create', 'docker.admin', '创建Docker Network', 'API', 'POST', '/admin/docker/networks', 0, '创建Docker Network', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100509, 'admin:docker:network:prune', 'docker.admin', '清理Docker Network', 'API', 'POST', '/admin/docker/networks/prune/*', 0, '清理未使用Docker Network', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100510, 'admin:docker:network:query', 'docker.admin', 'Docker Network详情', 'API', 'GET', '/admin/docker/networks/{id}', 0, '查询Docker Network详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100511, 'admin:docker:network:delete', 'docker.admin', '删除Docker Network', 'API', 'DELETE', '/admin/docker/networks/{id}', 0, '删除Docker Network', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100512, 'admin:docker:network:connect', 'docker.admin', '连接Docker Network', 'API', 'POST', '/admin/docker/networks/{id}/connect', 0, '连接容器到Docker Network', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100513, 'admin:docker:network:disconnect', 'docker.admin', '断开Docker Network', 'API', 'POST', '/admin/docker/networks/{id}/disconnect', 0, '断开容器与Docker Network', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100514, 'admin:docker:config:query', 'docker.admin', 'Docker配置查询', 'API', 'GET', '/admin/docker/daemon/config', 0, '查询Docker daemon配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100515, 'admin:docker:config:validate', 'docker.admin', 'Docker配置校验', 'API', 'POST', '/admin/docker/daemon/config/validate', 0, '校验Docker daemon配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100516, 'admin:docker:config:update', 'docker.admin', 'Docker配置更新', 'API', 'PUT', '/admin/docker/daemon/config', 0, '保存Docker daemon配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100517, 'admin:docker:config:restart', 'docker.admin', 'Docker daemon重启', 'API', 'POST', '/admin/docker/daemon/restart', 0, '重启Docker daemon', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100518, 'admin:docker:registry:sync', 'docker.admin', '同步Docker Registry', 'API', 'POST', '/admin/docker/registries/{id}/sync', 0, '同步Docker Registry', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900100519, 'admin:docker:registry:delete', 'docker.admin', '删除Docker Registry', 'API', 'DELETE', '/admin/docker/registries/{id}', 0, '删除本地Docker Registry配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300100, '*', NULL, '全部权限', 'API', '*', '*', 0, '超级管理员全部权限', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300101, 'system:role:list', NULL, '角色列表', 'API', 'GET', '/system/role/page', 0, '分页查询角色', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300102, 'system:role:query', NULL, '角色详情', 'API', 'GET', '/system/role/{id}', 0, '查询角色详情与授权', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300103, 'system:role:add', NULL, '新增角色', 'API', 'POST', '/system/role', 0, '新增角色', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300104, 'system:role:edit', NULL, '编辑角色', 'API', 'PUT', '/system/role', 0, '编辑角色与菜单授权', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300105, 'system:role:remove', NULL, '删除角色', 'API', 'DELETE', '/system/role/{id}', 0, '删除角色', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300106, 'system:role:grant', NULL, '角色权限分配', 'API', 'POST', '/system/role/permissions/assign', 0, '分配角色权限资源', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300111, 'system:menu:list', NULL, '菜单列表', 'API', 'GET', '/system/menu/tree', 0, '查询菜单树', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300112, 'system:menu:query', NULL, '菜单详情', 'API', 'GET', '/system/menu/{id}', 0, '查询菜单详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300113, 'system:menu:add', NULL, '新增菜单', 'API', 'POST', '/system/menu', 0, '新增菜单', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300114, 'system:menu:edit', NULL, '编辑菜单', 'API', 'PUT', '/system/menu', 0, '编辑菜单', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300115, 'system:menu:remove', NULL, '删除菜单', 'API', 'DELETE', '/system/menu/{id}', 0, '删除菜单', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300121, 'system:permission:list', NULL, '权限资源列表', 'API', 'GET', '/system/menu/permissions', 0, '查询权限资源列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300122, 'system:permission:query', NULL, '权限资源详情', 'API', 'GET', '/system/menu/permissions/{permissionId}', 0, '查询权限资源详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300123, 'system:permission:add', NULL, '新增权限资源', 'API', 'POST', '/system/menu/permissions', 0, '新增权限资源', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300124, 'system:permission:edit', NULL, '编辑权限资源', 'API', 'PUT', '/system/menu/permissions/{permissionId}', 0, '编辑权限资源', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300125, 'system:permission:remove', NULL, '删除权限资源', 'API', 'DELETE', '/system/menu/permissions/{permissionId}', 0, '删除权限资源', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300131, 'system:menu:permission:list', NULL, '菜单权限绑定列表', 'API', 'GET', '/system/menu/{menuId}/permissions', 0, '查询菜单权限绑定', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300132, 'system:menu:permission:assign', NULL, '菜单权限绑定', 'API', 'POST', '/system/menu/{menuId}/permissions', 0, '绑定菜单权限', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300133, 'system:user-role:assign', NULL, '用户角色分配', 'API', 'POST', '/system/role/user-roles/assign', 0, '分配用户角色', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300901, 'admin:sso:session:list', NULL, 'SSO会话列表', 'API', 'GET', '/sso/admin/users/{userId}/sessions', 0, '查询用户SSO会话', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300902, 'admin:sso:session:kick', NULL, 'SSO会话踢出', 'API', 'POST', '/sso/admin/users/{userId}/sessions/{sessionId}/kick', 0, '踢出用户SSO会话', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300903, 'admin:sso:device:list', NULL, 'SSO设备列表', 'API', 'GET', '/sso/admin/users/{userId}/devices', 0, '查询用户SSO设备', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300904, 'admin:sso:device:kick', NULL, 'SSO设备踢出', 'API', 'POST', '/sso/admin/users/{userId}/devices/{deviceId}/kick', 0, '踢出用户SSO设备', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900300905, 'admin:ops:module:list', NULL, '模块运行列表', 'API', 'GET', '/ops/modules', 0, '查询模块运行列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301001, 'admin:log:clean', NULL, 'admin log clean', 'API', 'POST', '/admin/logs/operation/clean', 0, 'admin log clean', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301002, 'admin:log:delete', NULL, 'admin log delete', 'API', 'POST', '/admin/logs/operation/deleteByTimeRange', 0, 'admin log delete', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301003, 'admin:log:export', NULL, 'admin log export', 'API', 'GET', '/admin/logs/operation/export', 0, 'admin log export', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301004, 'admin:log:view', NULL, 'admin log view', 'API', 'GET', '/admin/logs/operation', 0, 'admin log view', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301005, 'admin:online:kick', NULL, 'admin online kick', 'API', 'POST', '/admin/kick/:userId', 0, 'admin online kick', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301006, 'admin:temp-permission:cleanup', NULL, 'admin temp-permission cleanup', 'API', 'POST', '/admin/temp-permission/cleanup', 0, 'admin temp-permission cleanup', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301007, 'admin:temp-permission:extend', NULL, 'admin temp-permission extend', 'API', 'PUT', '/admin/temp-permission/extend', 0, 'admin temp-permission extend', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301008, 'admin:temp-permission:grant', NULL, 'admin temp-permission grant', 'API', 'POST', '/admin/temp-permission/grant', 0, 'admin temp-permission grant', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301009, 'admin:temp-permission:query', NULL, 'admin temp-permission query', 'API', 'GET', '/admin/temp-permission/list', 0, 'admin temp-permission query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301010, 'admin:temp-permission:revoke', NULL, 'admin temp-permission revoke', 'API', 'DELETE', '/admin/temp-permission/revoke', 0, 'admin temp-permission revoke', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301011, 'admin:temp-permission:stats', NULL, 'admin temp-permission stats', 'API', 'GET', '/admin/temp-permission/statistics', 0, 'admin temp-permission stats', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301012, 'auth:user:info', NULL, 'auth user info', 'API', 'GET', '/admin/logs/operation/my', 0, 'auth user info', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301013, 'system:config:add', NULL, 'system config add', 'API', 'POST', '/config', 0, 'system config add', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301014, 'system:config:apply', NULL, 'system config apply', 'API', 'POST', '/config/apply-pending', 0, 'system config apply', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301015, 'system:config:delete', NULL, 'system config delete', 'API', 'POST', '/config/delete', 0, 'system config delete', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301016, 'system:config:edit', NULL, 'system config edit', 'API', 'POST', '/config/update', 0, 'system config edit', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301017, 'system:config:group:add', NULL, 'system config group add', 'API', 'POST', '/config-groups', 0, 'system config group add', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301018, 'system:config:group:delete', NULL, 'system config group delete', 'API', 'POST', '/config-groups/delete', 0, 'system config group delete', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301019, 'system:config:group:edit', NULL, 'system config group edit', 'API', 'POST', '/config-groups/update', 0, 'system config group edit', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301020, 'system:config:group:query', NULL, 'system config group query', 'API', 'GET', '/config-groups/page', 0, 'system config group query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301021, 'system:config:query', NULL, 'system config query', 'API', 'GET', '/config/:id', 0, 'system config query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301022, 'system:config:rollback', NULL, 'system config rollback', 'API', 'POST', '/config/rollback', 0, 'system config rollback', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301023, 'system:config:sensitive', NULL, 'system config sensitive', 'API', 'POST', '/config/:id/sensitive/reveal', 0, 'system config sensitive', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301024, 'system:dept:add', NULL, 'system dept add', 'API', 'POST', '/system/dept', 0, 'system dept add', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301025, 'system:dept:edit', NULL, 'system dept edit', 'API', 'PUT', '/system/dept', 0, 'system dept edit', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301026, 'system:dept:query', NULL, 'system dept query', 'API', 'GET', '/system/dept/:deptId/children', 0, 'system dept query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301027, 'system:dept:remove', NULL, 'system dept remove', 'API', 'DELETE', '/system/dept/:id', 0, 'system dept remove', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301028, 'system:dept:view', NULL, 'system dept view', 'API', 'GET', '/system/dept/tree/enabled', 0, 'system dept view', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301029, 'system:dict:add', NULL, 'system dict add', 'API', 'POST', '/dict-type/add', 0, 'system dict add', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301030, 'system:dict:delete', NULL, 'system dict delete', 'API', 'POST', '/dict-type/delete', 0, 'system dict delete', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301031, 'system:dict:edit', NULL, 'system dict edit', 'API', 'POST', '/dict-type/update', 0, 'system dict edit', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301032, 'system:dict:query', NULL, 'system dict query', 'API', 'GET', '/dict-type/:id', 0, 'system dict query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301033, 'system:file-task:list', NULL, 'system file-task list', 'API', 'GET', '/file-process-task', 0, 'system file-task list', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301034, 'system:file-task:retry', NULL, 'system file-task retry', 'API', 'POST', '/file-process-task/:id/retry', 0, 'system file-task retry', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301035, 'system:file:delete', NULL, 'system file delete', 'API', 'POST', '/file-manage/:id/delete', 0, 'system file delete', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301036, 'system:file:edit', NULL, 'system file edit', 'API', 'POST', '/file-manage/references/:id/access-level', 0, 'system file edit', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301037, 'system:file:list', NULL, 'system file list', 'API', 'GET', '/file-manage/list', 0, 'system file list', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301038, 'system:file:query', NULL, 'system file query', 'API', 'GET', '/file-manage/:id', 0, 'system file query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301039, 'system:post:role', NULL, 'system post role', 'API', 'GET', '/system/post/:postId/roles', 0, 'system post role', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301040, 'system:storage:add', NULL, 'system storage add', 'API', 'POST', '/storage-strategy', 0, 'system storage add', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301041, 'system:storage:delete', NULL, 'system storage delete', 'API', 'POST', '/storage-strategy/:id/delete', 0, 'system storage delete', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301042, 'system:storage:edit', NULL, 'system storage edit', 'API', 'POST', '/storage-strategy/update', 0, 'system storage edit', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301043, 'system:storage:list', NULL, 'system storage list', 'API', 'GET', '/storage-strategy', 0, 'system storage list', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301044, 'system:user:query', NULL, 'system user query', 'API', 'GET', '/user/get/:id', 0, 'system user query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301045, 'system:user:status', NULL, 'system user status', 'API', 'POST', '/user/status/:id', 0, 'system user status', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301060, 'system:config:scope:query', NULL, 'system config scope query', 'API', 'GET', '/config-scopes/roles/:roleId', 0, 'system config scope query', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301061, 'system:config:scope:assign', NULL, 'system config scope assign', 'API', 'POST', '/config-scopes/roles/:roleId', 0, 'system config scope assign', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301062, 'system:user:access:query', NULL, '查询用户有效权限', 'API', 'GET', '/system/user/:id/effective-access', 0, '查询目标用户的有效角色、数据范围和权限来源', 0, '2026-07-19 00:39:00+08', 0, '2026-07-19 00:39:00+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301063, 'system:user:access:explain', NULL, '解释用户权限', 'API', 'GET', '/system/user/:id/access-explain', 0, '解释目标用户指定权限的允许或拒绝原因', 0, '2026-07-19 00:39:00+08', 0, '2026-07-19 00:39:00+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301101, 'system:sso-client:list', NULL, 'SSO客户端列表', 'API', 'GET', '/sso/admin/clients', 0, '分页查询SSO客户端列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301102, 'system:sso-client:query', NULL, 'SSO客户端详情', 'API', 'GET', '/sso/admin/clients/{clientId}', 0, '查询SSO客户端详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301103, 'system:sso-client:add', NULL, '创建SSO客户端', 'API', 'POST', '/sso/admin/clients', 0, '创建SSO客户端', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301104, 'system:sso-client:edit', NULL, '编辑SSO客户端', 'API', 'PUT', '/sso/admin/clients/{clientId}', 0, '编辑SSO客户端', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301105, 'system:sso-client:status', NULL, '启停SSO客户端', 'API', 'PUT', '/sso/admin/clients/{clientId}/status', 0, '启用或停用SSO客户端', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301106, 'system:sso-client:redirect:list', NULL, '查询SSO回调地址', 'API', 'GET', '/sso/admin/clients/{clientId}/redirect-uris', 0, '查询SSO客户端回调地址', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301107, 'system:sso-client:redirect:edit', NULL, '编辑SSO回调地址', 'API', 'PUT', '/sso/admin/clients/{clientId}/redirect-uris', 0, '替换SSO客户端回调地址', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301108, 'system:sso-client:secret:list', NULL, '查询SSO密钥', 'API', 'GET', '/sso/admin/clients/{clientId}/secrets', 0, '查询SSO客户端密钥摘要', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301109, 'system:sso-client:secret:generate', NULL, '生成SSO密钥', 'API', 'POST', '/sso/admin/clients/{clientId}/secrets', 0, '生成SSO客户端密钥', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301110, 'system:sso-client:secret:disable', NULL, '停用SSO密钥', 'API', 'PUT', '/sso/admin/clients/{clientId}/secrets/{secretId}/status', 0, '停用SSO客户端密钥', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301201, 'system:external-login-provider:list', NULL, '外部登录提供方列表', 'API', 'GET', '/external-login/admin/providers', 0, '分页查询外部登录提供方列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301202, 'system:external-login-provider:query', NULL, '外部登录提供方详情', 'API', 'GET', '/external-login/admin/providers/:providerCode', 0, '查询外部登录提供方详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301203, 'system:external-login-provider:add', NULL, '创建外部登录提供方', 'API', 'POST', '/external-login/admin/providers', 0, '创建外部登录提供方', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301204, 'system:external-login-provider:edit', NULL, '编辑外部登录提供方', 'API', 'PUT', '/external-login/admin/providers/:providerCode', 0, '编辑外部登录提供方', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301205, 'system:external-login-provider:status', NULL, '启停外部登录提供方', 'API', 'PUT', '/external-login/admin/providers/:providerCode/status', 0, '启用或停用外部登录提供方', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301206, 'system:external-login-provider:secret:rotate', NULL, '轮换外部登录密钥', 'API', 'POST', '/external-login/admin/providers/:providerCode/client-secret/rotate', 0, '轮换外部登录提供方 client secret', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301207, 'system:external-login-identity:list', NULL, '外部身份绑定列表', 'API', 'GET', '/external-login/admin/identities', 0, '分页查询外部身份绑定列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301208, 'system:external-login-identity:status', NULL, '变更外部身份状态', 'API', 'PUT', '/external-login/admin/identities/:identityId/status', 0, '启用、停用或解绑外部身份', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301209, 'system:external-oauth-token:list', NULL, '外部 OAuth Token 列表', 'API', 'GET', '/external-login/admin/tokens', 0, '分页查询外部 OAuth token 列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301210, 'system:external-oauth-token:revoke', NULL, '撤销外部 OAuth Token', 'API', 'PUT', '/external-login/admin/tokens/:tokenId/revoke', 0, '撤销外部 OAuth token', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301301, 'system:platform:list', 'platform.control', '平台列表', 'API', 'GET', '/platform/admin/platforms', 0, '分页查询平台列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301302, 'system:platform:query', 'platform.control', '平台详情', 'API', 'GET', '/platform/admin/platforms/:platformCode', 0, '查询平台详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301303, 'system:platform:add', 'platform.control', '创建平台', 'API', 'POST', '/platform/admin/platforms', 0, '创建平台配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301304, 'system:platform:edit', 'platform.control', '编辑平台', 'API', 'PUT', '/platform/admin/platforms/:platformCode', 0, '编辑平台配置', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301305, 'system:platform:status', 'platform.control', '启停平台', 'API', 'PUT', '/platform/admin/platforms/:platformCode/status', 0, '启用或停用平台', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301306, 'system:platform:login-method:edit', 'platform.control', '编辑平台登录方式', 'API', 'PUT', '/platform/admin/platforms/:platformCode/login-methods', 0, '替换平台登录方式', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301307, 'system:platform:source-rule:edit', 'platform.control', '编辑平台来源规则', 'API', 'PUT', '/platform/admin/platforms/:platformCode/source-rules', 0, '替换平台来源规则', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (1900301308, 'system:platform:default-role:edit', 'platform.control', '编辑平台默认角色', 'API', 'PUT', '/platform/admin/platforms/:platformCode/default-roles', 0, '替换平台默认角色', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001001, 'system:notification:channel:list', NULL, '通知渠道列表', 'API', 'GET', '/notification/channels', 0, '查询通知渠道', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001002, 'system:notification:channel:edit', NULL, '编辑通知渠道', 'API', 'POST', '/notification/channels', 0, '新增或编辑通知渠道', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001003, 'system:notification:template:list', NULL, '通知模板列表', 'API', 'GET', '/notification/templates', 0, '查询通知模板', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001004, 'system:notification:template:edit', NULL, '编辑通知模板', 'API', 'POST', '/notification/templates', 0, '新增或编辑通知模板', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001005, 'system:notification:scene:list', NULL, '通知场景列表', 'API', 'GET', '/notification/scene-bindings', 0, '查询通知场景', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001006, 'system:notification:scene:edit', NULL, '编辑通知场景', 'API', 'POST', '/notification/scene-bindings', 0, '新增或编辑通知场景', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001007, 'system:notification:delivery:list', NULL, '通知投递日志', 'API', 'GET', '/notification/deliveries', 0, '查询通知投递日志', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001008, 'system:notification:test', NULL, '测试通知发送', 'API', 'POST', '/notification/test-send', 0, '测试通知发送', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001009, 'system:hub-node:list', 'federation.hub', 'Hub节点列表', 'API', 'GET', '/system/hub/nodes', 0, '查询Hub节点列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001010, 'system:hub-node:add', 'federation.hub', '创建Hub节点', 'API', 'POST', '/system/hub/nodes', 0, '创建或复制Hub节点', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001011, 'system:hub-node:query', 'federation.hub', 'Hub节点详情', 'API', 'GET', '/system/hub/nodes/:nodeCode', 0, '查询Hub节点详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001012, 'system:hub-node:edit', 'federation.hub', '编辑Hub节点', 'API', 'PUT', '/system/hub/nodes/:nodeCode', 0, '编辑Hub节点', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001013, 'system:hub-node:status', 'federation.hub', '启停Hub节点', 'API', 'PUT', '/system/hub/nodes/:nodeCode/status', 0, '启用或停用Hub节点', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001014, 'system:hub-node:test', 'federation.hub', '测试Hub节点连接', 'API', 'POST', '/system/hub/nodes/:nodeCode/connection-test', 0, '测试Hub节点连接', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001015, 'system:hub-node:user:list', 'federation.hub', 'Node用户列表', 'API', 'GET', '/system/hub/nodes/:nodeCode/users', 0, '查询Node用户列表', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001016, 'system:hub-node:user:query', 'federation.hub', 'Node用户详情', 'API', 'GET', '/system/hub/nodes/:nodeCode/users/:userId', 0, '查询Node用户详情', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001017, 'system:hub-node:user:status', 'federation.hub', '修改Node用户状态', 'API', 'PUT', '/system/hub/nodes/:nodeCode/users/:userId/status', 0, '修改Node用户状态', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001018, 'system:hub-node:session:list', 'federation.hub', 'Node用户会话', 'API', 'GET', '/system/hub/nodes/:nodeCode/users/:userId/sessions', 0, '查询Node用户会话', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001019, 'system:hub-node:session:revoke', 'federation.hub', '撤销Node用户会话', 'API', 'POST', '/system/hub/nodes/:nodeCode/users/:userId/sessions/revoke', 0, '撤销Node用户会话', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001020, 'system:hub-node:policy:query', 'federation.hub', 'Node登录策略', 'API', 'GET', '/system/hub/nodes/:nodeCode/login-policy', 0, '查询Node登录策略', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001021, 'system:hub-node:policy:apply', 'federation.hub', '应用Node登录策略', 'API', 'POST', '/system/hub/nodes/:nodeCode/login-policy/apply', 0, '应用Node登录策略', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001022, 'system:hub-node:federation:query', 'federation.hub', 'Node联邦连接', 'API', 'GET', '/system/hub/nodes/:nodeCode/federation', 0, '查询Node联邦连接', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_permission" VALUES (2026062510000001023, 'system:hub-node:federation:apply', 'federation.hub', '编排Node联邦连接', 'API', 'POST', '/system/hub/nodes/:nodeCode/federation/provision', 0, '编排Node联邦连接', NULL, '2026-07-19 00:38:59+08', NULL, '2026-07-19 00:38:59+08', false);


--
-- Data for Name: sys_post; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_post_role; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_role; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_role" VALUES (1900300001, '超级管理员', 'SUPER_ADMIN', 'AUTHORIZATION_ROOT', 1, 0, 1, NULL, 0, 0, 'RBAC admin seed role', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:39:00+08', false);
INSERT INTO "public"."sys_role" VALUES (2012232600000000001, '运维实习生', 'OPS_INTERN', NULL, 5, 0, 3, NULL, 0, 60, 'Docker operations read-only role', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_role" VALUES (2012232600000000002, '运维工程师', 'OPS_ENGINEER', NULL, 5, 0, 3, NULL, 0, 61, 'Docker operations controlled-action role', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);
INSERT INTO "public"."sys_role" VALUES (2012232600000000003, '运维管理员', 'OPS_ADMIN', NULL, 5, 0, 3, NULL, 0, 62, 'Docker operations administrator role', 0, '2026-07-19 00:38:59+08', 0, '2026-07-19 00:38:59+08', false);


--
-- Data for Name: sys_role_config_scope; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_role_dept; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_role_menu; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_role_menu" VALUES (193830640201, 1900300001, 1900300200, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (193830640202, 1900300001, 1900300201, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (193830640203, 1900300001, 1900300202, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (193830640204, 1900300001, 1900300203, 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232069056870001, 1900300001, 2012232069056864258, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232069056870002, 1900300001, 2012232069056864259, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232069056870003, 1900300001, 2012232069056864260, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232069056870004, 1900300001, 2012232069056864261, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232069056870005, 1900300001, 2012232069056864262, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232069056870006, 1900300001, 2012232069056864263, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232407482671201, 1900300001, 2012232407482671109, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232600000000201, 2012232600000000001, 2012232407482671109, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232600000000202, 2012232600000000002, 2012232407482671109, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232600000000203, 2012232600000000003, 2012232407482671109, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232800000000201, 1900300001, 2012232800000000001, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232800000000202, 1900300001, 2012232800000000002, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000201, 1900300001, 2012232900000000001, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000202, 1900300001, 2012232900000000002, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000203, 1900300001, 2012232900000000003, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000204, 1900300001, 2012232900000000004, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000205, 1900300001, 2012232900000000005, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000206, 2012232600000000001, 2012232900000000003, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000207, 2012232600000000002, 2012232900000000003, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012232900000000208, 2012232600000000003, 2012232900000000003, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233000000000201, 1900300001, 2012233000000000001, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000201, 1900300001, 2012232900000000101, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000202, 1900300001, 2012233100000000001, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000203, 1900300001, 2012233100000000002, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000204, 1900300001, 2012233100000000003, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000205, 1900300001, 2012233100000000004, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000206, 1900300001, 2012233100000000005, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000207, 1900300001, 2012233100000000006, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233100000000208, 1900300001, 2012233100000000007, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2012233200000000201, 1900300001, 2012233200000000001, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2026062510000004001, 1900300001, 2026062510000002001, NULL, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_menu" VALUES (2026062510000004002, 1900300001, 2026062510000002002, NULL, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');


--
-- Data for Name: sys_role_permission; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_role_permission" VALUES (193830650101, 1900300001, 1900300100, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650102, 1900300001, 1900300101, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650103, 1900300001, 1900300102, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650104, 1900300001, 1900300103, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650105, 1900300001, 1900300104, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650106, 1900300001, 1900300105, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650107, 1900300001, 1900300106, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650112, 1900300001, 1900300111, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650113, 1900300001, 1900300112, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650114, 1900300001, 1900300113, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650115, 1900300001, 1900300114, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650116, 1900300001, 1900300115, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650122, 1900300001, 1900300121, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650123, 1900300001, 1900300122, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650124, 1900300001, 1900300123, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650125, 1900300001, 1900300124, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650126, 1900300001, 1900300125, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650132, 1900300001, 1900300131, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650133, 1900300001, 1900300132, 'MENU', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830650134, 1900300001, 1900300133, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830690902, 1900300001, 1900300901, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830690903, 1900300001, 1900300902, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830690904, 1900300001, 1900300903, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830690905, 1900300001, 1900300904, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830690906, 1900300001, 1900300905, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701002, 1900300001, 1900301001, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701003, 1900300001, 1900301002, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701004, 1900300001, 1900301003, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701005, 1900300001, 1900301004, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701006, 1900300001, 1900301005, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701007, 1900300001, 1900301006, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701008, 1900300001, 1900301007, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701009, 1900300001, 1900301008, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701010, 1900300001, 1900301009, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701011, 1900300001, 1900301010, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701012, 1900300001, 1900301011, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701013, 1900300001, 1900301012, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701014, 1900300001, 1900301013, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701015, 1900300001, 1900301014, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701016, 1900300001, 1900301015, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701017, 1900300001, 1900301016, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701018, 1900300001, 1900301017, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701019, 1900300001, 1900301018, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701020, 1900300001, 1900301019, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701021, 1900300001, 1900301020, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701022, 1900300001, 1900301021, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701023, 1900300001, 1900301022, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701024, 1900300001, 1900301023, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701025, 1900300001, 1900301024, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701026, 1900300001, 1900301025, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701027, 1900300001, 1900301026, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701028, 1900300001, 1900301027, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701029, 1900300001, 1900301028, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701030, 1900300001, 1900301029, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701031, 1900300001, 1900301030, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701032, 1900300001, 1900301031, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701033, 1900300001, 1900301032, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701034, 1900300001, 1900301033, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701035, 1900300001, 1900301034, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701036, 1900300001, 1900301035, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701037, 1900300001, 1900301036, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701038, 1900300001, 1900301037, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701039, 1900300001, 1900301038, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701040, 1900300001, 1900301039, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701041, 1900300001, 1900301040, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701042, 1900300001, 1900301041, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701043, 1900300001, 1900301042, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701044, 1900300001, 1900301043, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701045, 1900300001, 1900301044, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830701046, 1900300001, 1900301045, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830707061, 1900300001, 1900301060, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830707062, 1900300001, 1900301061, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830707263, 1900300001, 1900301062, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (193830707264, 1900300001, 1900301063, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000301, 1900300001, 1900100001, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000302, 1900300001, 1900100002, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000303, 1900300001, 1900100003, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000304, 1900300001, 1900100004, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000305, 1900300001, 1900100005, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000306, 1900300001, 1900100006, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000307, 1900300001, 1900100007, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000308, 1900300001, 1900100008, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000309, 1900300001, 1900100011, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000310, 1900300001, 1900100012, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000311, 1900300001, 1900100013, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000312, 1900300001, 1900100014, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000313, 1900300001, 1900100015, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000314, 1900300001, 1900100016, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000315, 1900300001, 1900100017, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000316, 1900300001, 1900100018, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000317, 1900300001, 1900100021, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000318, 1900300001, 1900100022, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000319, 1900300001, 1900100023, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000320, 1900300001, 1900100024, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000321, 1900300001, 1900100031, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000322, 1900300001, 1900100032, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000323, 1900300001, 1900100033, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000324, 1900300001, 1900100034, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000325, 1900300001, 1900100035, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000326, 1900300001, 1900100036, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000327, 1900300001, 1900100037, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000328, 1900300001, 1900100038, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000329, 1900300001, 1900100039, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000330, 1900300001, 1900100040, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000331, 1900300001, 1900100041, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000332, 1900300001, 1900100042, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000333, 1900300001, 1900100043, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000334, 1900300001, 1900100044, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000335, 1900300001, 1900100045, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000336, 1900300001, 1900100046, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000337, 1900300001, 1900100047, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000338, 1900300001, 1900100501, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000339, 1900300001, 1900100502, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000340, 1900300001, 1900100503, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000341, 1900300001, 1900100504, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000342, 1900300001, 1900100505, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000343, 1900300001, 1900100506, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000344, 1900300001, 1900100507, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000345, 1900300001, 1900100508, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000346, 1900300001, 1900100509, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000347, 1900300001, 1900100510, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000348, 1900300001, 1900100511, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000349, 1900300001, 1900100512, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000350, 1900300001, 1900100513, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000351, 1900300001, 1900100514, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000352, 1900300001, 1900100515, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000353, 1900300001, 1900100516, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000354, 1900300001, 1900100517, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000355, 1900300001, 1900100518, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000356, 1900300001, 1900100519, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000357, 2012232600000000001, 1900100001, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000358, 2012232600000000001, 1900100002, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000359, 2012232600000000001, 1900100003, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000360, 2012232600000000001, 1900100011, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000361, 2012232600000000001, 1900100012, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000362, 2012232600000000001, 1900100013, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000363, 2012232600000000001, 1900100021, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000364, 2012232600000000001, 1900100031, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000365, 2012232600000000001, 1900100033, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000366, 2012232600000000001, 1900100034, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000367, 2012232600000000001, 1900100041, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000368, 2012232600000000001, 1900100042, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000369, 2012232600000000001, 1900100043, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000370, 2012232600000000001, 1900100502, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000371, 2012232600000000001, 1900100505, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000372, 2012232600000000001, 1900100507, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000373, 2012232600000000001, 1900100510, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000374, 2012232600000000001, 1900100514, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000375, 2012232600000000002, 1900100001, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000376, 2012232600000000002, 1900100002, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000377, 2012232600000000002, 1900100003, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000378, 2012232600000000002, 1900100004, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000379, 2012232600000000002, 1900100005, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000380, 2012232600000000002, 1900100006, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000381, 2012232600000000002, 1900100008, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000382, 2012232600000000002, 1900100011, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000383, 2012232600000000002, 1900100012, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000384, 2012232600000000002, 1900100013, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000385, 2012232600000000002, 1900100014, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000386, 2012232600000000002, 1900100015, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000387, 2012232600000000002, 1900100018, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000388, 2012232600000000002, 1900100021, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000389, 2012232600000000002, 1900100024, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000390, 2012232600000000002, 1900100031, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000391, 2012232600000000002, 1900100032, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000392, 2012232600000000002, 1900100033, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000393, 2012232600000000002, 1900100034, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000394, 2012232600000000002, 1900100036, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000395, 2012232600000000002, 1900100037, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000396, 2012232600000000002, 1900100038, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000397, 2012232600000000002, 1900100039, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000398, 2012232600000000002, 1900100040, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000399, 2012232600000000002, 1900100041, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000400, 2012232600000000002, 1900100042, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000401, 2012232600000000002, 1900100043, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000402, 2012232600000000002, 1900100044, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000403, 2012232600000000002, 1900100502, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000404, 2012232600000000002, 1900100503, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000405, 2012232600000000002, 1900100505, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000406, 2012232600000000002, 1900100507, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000407, 2012232600000000002, 1900100508, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000408, 2012232600000000002, 1900100510, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000409, 2012232600000000002, 1900100512, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000410, 2012232600000000002, 1900100513, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000411, 2012232600000000002, 1900100514, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000412, 2012232600000000002, 1900100515, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000413, 2012232600000000003, 1900100001, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000414, 2012232600000000003, 1900100002, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000415, 2012232600000000003, 1900100003, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000416, 2012232600000000003, 1900100004, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000417, 2012232600000000003, 1900100005, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000418, 2012232600000000003, 1900100006, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000419, 2012232600000000003, 1900100007, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000420, 2012232600000000003, 1900100008, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000421, 2012232600000000003, 1900100011, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000422, 2012232600000000003, 1900100012, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000423, 2012232600000000003, 1900100013, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000424, 2012232600000000003, 1900100014, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000425, 2012232600000000003, 1900100015, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000426, 2012232600000000003, 1900100016, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000427, 2012232600000000003, 1900100017, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000428, 2012232600000000003, 1900100018, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000429, 2012232600000000003, 1900100021, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000430, 2012232600000000003, 1900100022, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000431, 2012232600000000003, 1900100023, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000432, 2012232600000000003, 1900100024, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000433, 2012232600000000003, 1900100031, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000434, 2012232600000000003, 1900100032, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000435, 2012232600000000003, 1900100033, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000436, 2012232600000000003, 1900100034, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000437, 2012232600000000003, 1900100035, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000438, 2012232600000000003, 1900100036, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000439, 2012232600000000003, 1900100037, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000440, 2012232600000000003, 1900100038, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000441, 2012232600000000003, 1900100039, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000442, 2012232600000000003, 1900100040, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000443, 2012232600000000003, 1900100041, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000444, 2012232600000000003, 1900100042, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000445, 2012232600000000003, 1900100043, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000446, 2012232600000000003, 1900100044, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000447, 2012232600000000003, 1900100045, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000448, 2012232600000000003, 1900100046, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000449, 2012232600000000003, 1900100047, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000450, 2012232600000000003, 1900100501, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000451, 2012232600000000003, 1900100502, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000452, 2012232600000000003, 1900100503, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000453, 2012232600000000003, 1900100504, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000454, 2012232600000000003, 1900100505, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000455, 2012232600000000003, 1900100506, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000456, 2012232600000000003, 1900100507, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000457, 2012232600000000003, 1900100508, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000458, 2012232600000000003, 1900100509, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000459, 2012232600000000003, 1900100510, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000460, 2012232600000000003, 1900100511, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000461, 2012232600000000003, 1900100512, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000462, 2012232600000000003, 1900100513, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000463, 2012232600000000003, 1900100514, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000464, 2012232600000000003, 1900100515, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000465, 2012232600000000003, 1900100516, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000466, 2012232600000000003, 1900100517, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000467, 2012232600000000003, 1900100518, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232600000000468, 2012232600000000003, 1900100519, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000301, 1900300001, 1900301101, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000302, 1900300001, 1900301102, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000303, 1900300001, 1900301103, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000304, 1900300001, 1900301104, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000305, 1900300001, 1900301105, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000306, 1900300001, 1900301106, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000307, 1900300001, 1900301107, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000308, 1900300001, 1900301108, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000309, 1900300001, 1900301109, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012232800000000310, 1900300001, 1900301110, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000301, 1900300001, 1900301201, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000302, 1900300001, 1900301202, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000303, 1900300001, 1900301203, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000304, 1900300001, 1900301204, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000305, 1900300001, 1900301205, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000306, 1900300001, 1900301206, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000307, 1900300001, 1900301207, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000308, 1900300001, 1900301208, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000309, 1900300001, 1900301209, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233000000000310, 1900300001, 1900301210, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000301, 1900300001, 1900301301, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000302, 1900300001, 1900301302, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000303, 1900300001, 1900301303, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000304, 1900300001, 1900301304, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000305, 1900300001, 1900301305, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000306, 1900300001, 1900301306, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000307, 1900300001, 1900301307, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2012233200000000308, 1900300001, 1900301308, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005001, 1900300001, 2026062510000001001, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005002, 1900300001, 2026062510000001002, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005003, 1900300001, 2026062510000001003, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005004, 1900300001, 2026062510000001004, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005005, 1900300001, 2026062510000001005, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005006, 1900300001, 2026062510000001006, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005007, 1900300001, 2026062510000001007, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005008, 1900300001, 2026062510000001008, 'DIRECT', 0, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005009, 1900300001, 2026062510000001009, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005010, 1900300001, 2026062510000001010, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005011, 1900300001, 2026062510000001011, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005012, 1900300001, 2026062510000001012, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005013, 1900300001, 2026062510000001013, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005014, 1900300001, 2026062510000001014, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005015, 1900300001, 2026062510000001015, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005016, 1900300001, 2026062510000001016, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005017, 1900300001, 2026062510000001017, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005018, 1900300001, 2026062510000001018, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005019, 1900300001, 2026062510000001019, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005020, 1900300001, 2026062510000001020, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005021, 1900300001, 2026062510000001021, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005022, 1900300001, 2026062510000001022, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');
INSERT INTO "public"."sys_role_permission" VALUES (2026062510000005023, 1900300001, 2026062510000001023, 'DIRECT', 0, '2026-07-19 00:39:00+08', '2026-07-19 00:39:00+08');


--
-- Data for Name: sys_security_bootstrap; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_storage_alert_log; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_storage_strategy; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO "public"."sys_storage_strategy" VALUES (2026041901001, 'local-default', 'LOCAL', true, true, 'ACTIVE', 100, '', '', '', NULL, 1, NULL, 0, 0, 10.00, '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', '2026-07-19 00:38:59+08', false);


--
-- Data for Name: sys_upload_task; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user_credential; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user_dept; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user_org; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user_permission; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user_position; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: sys_user_role; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Name: sysExternalLoginProvider_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysExternalLoginProvider_id_seq"', 1, true);


--
-- Name: sysExternalOAuthLoginState_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysExternalOAuthLoginState_id_seq"', 1, true);


--
-- Name: sysExternalOAuthToken_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysExternalOAuthToken_id_seq"', 1, true);


--
-- Name: sysExternalProviderMethod_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysExternalProviderMethod_id_seq"', 1, true);


--
-- Name: sysExternalUserIdentity_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysExternalUserIdentity_id_seq"', 1, true);


--
-- Name: sysPlatformDefaultRole_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysPlatformDefaultRole_id_seq"', 1, true);


--
-- Name: sysPlatformLoginMethod_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysPlatformLoginMethod_id_seq"', 4, true);


--
-- Name: sysPlatformSourceRule_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysPlatformSourceRule_id_seq"', 2, true);


--
-- Name: sysPlatformSsoClient_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysPlatformSsoClient_id_seq"', 1, true);


--
-- Name: sysPlatform_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysPlatform_id_seq"', 1, true);


--
-- Name: sysSsoAuditLog_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoAuditLog_id_seq"', 1, true);


--
-- Name: sysSsoAuthorizationCode_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoAuthorizationCode_id_seq"', 1, true);


--
-- Name: sysSsoClientRedirectUri_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoClientRedirectUri_id_seq"', 1, true);


--
-- Name: sysSsoClientSecret_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoClientSecret_id_seq"', 1, true);


--
-- Name: sysSsoClient_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoClient_id_seq"', 1, true);


--
-- Name: sysSsoConsentGrant_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoConsentGrant_id_seq"', 1, true);


--
-- Name: sysSsoIssuerKey_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoIssuerKey_id_seq"', 1, true);


--
-- Name: sysSsoRefreshTokenFamily_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoRefreshTokenFamily_id_seq"', 1, true);


--
-- Name: sysSsoSession_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sysSsoSession_id_seq"', 1, true);


--
-- Name: sys_config_group_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_config_group_id_seq"', 1, true);


--
-- Name: sys_config_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_config_id_seq"', 1, true);


--
-- Name: sys_dict_item_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_dict_item_id_seq"', 3, true);


--
-- Name: sys_dict_type_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_dict_type_id_seq"', 2026042501001, true);


--
-- Name: sys_operation_log_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_operation_log_id_seq"', 1, true);


--
-- Name: sys_post_role_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_post_role_id_seq"', 1, true);


--
-- Name: sys_user_dept_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_user_dept_id_seq"', 1, true);


--
-- Name: sys_user_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_user_id_seq"', 1, true);


--
-- Name: sys_user_org_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_user_org_id_seq"', 1, true);


--
-- Name: sys_user_permission_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_user_permission_id_seq"', 1, true);


--
-- Name: sys_user_position_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_user_position_id_seq"', 1, true);


--
-- Name: sys_user_role_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('"public"."sys_user_role_id_seq"', 1, true);


--
-- Name: docker_compose_project idx_3095026_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."docker_compose_project"
    ADD CONSTRAINT "idx_3095026_PRIMARY" PRIMARY KEY ("id");


--
-- Name: docker_operation idx_3095044_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."docker_operation"
    ADD CONSTRAINT "idx_3095044_PRIMARY" PRIMARY KEY ("id");


--
-- Name: docker_operation_event idx_3095061_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."docker_operation_event"
    ADD CONSTRAINT "idx_3095061_PRIMARY" PRIMARY KEY ("id");


--
-- Name: docker_remote_registry idx_3095072_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."docker_remote_registry"
    ADD CONSTRAINT "idx_3095072_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_config idx_3095111_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_config"
    ADD CONSTRAINT "idx_3095111_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_config_change_log idx_3095142_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_config_change_log"
    ADD CONSTRAINT "idx_3095142_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_config_group idx_3095159_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_config_group"
    ADD CONSTRAINT "idx_3095159_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_dept idx_3095178_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_dept"
    ADD CONSTRAINT "idx_3095178_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_dict_item idx_3095201_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_dict_item"
    ADD CONSTRAINT "idx_3095201_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_dict_type idx_3095224_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_dict_type"
    ADD CONSTRAINT "idx_3095224_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_file_binding_task idx_3095249_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_file_binding_task"
    ADD CONSTRAINT "idx_3095249_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_file_chunk_upload idx_3095267_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_file_chunk_upload"
    ADD CONSTRAINT "idx_3095267_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_file_info idx_3095286_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_file_info"
    ADD CONSTRAINT "idx_3095286_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_file_integrity_audit idx_3095302_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_file_integrity_audit"
    ADD CONSTRAINT "idx_3095302_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_file_process_run idx_3095313_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_file_process_run"
    ADD CONSTRAINT "idx_3095313_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_file_process_task idx_3095328_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_file_process_task"
    ADD CONSTRAINT "idx_3095328_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_file_reference idx_3095349_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_file_reference"
    ADD CONSTRAINT "idx_3095349_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_menu idx_3095370_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_menu"
    ADD CONSTRAINT "idx_3095370_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_menu_permission idx_3095392_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_menu_permission"
    ADD CONSTRAINT "idx_3095392_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_message_consume_log idx_3095400_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_message_consume_log"
    ADD CONSTRAINT "idx_3095400_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_operation_log idx_3095412_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_operation_log"
    ADD CONSTRAINT "idx_3095412_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_org idx_3095430_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_org"
    ADD CONSTRAINT "idx_3095430_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_outbox_event idx_3095450_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_outbox_event"
    ADD CONSTRAINT "idx_3095450_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_permission idx_3095468_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_permission"
    ADD CONSTRAINT "idx_3095468_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_post idx_3095486_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_post"
    ADD CONSTRAINT "idx_3095486_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_post_role idx_3095509_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_post_role"
    ADD CONSTRAINT "idx_3095509_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_role idx_3095516_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_role"
    ADD CONSTRAINT "idx_3095516_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_role_config_scope idx_3095538_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_role_config_scope"
    ADD CONSTRAINT "idx_3095538_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_role_dept idx_3095558_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_role_dept"
    ADD CONSTRAINT "idx_3095558_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_role_menu idx_3095564_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_role_menu"
    ADD CONSTRAINT "idx_3095564_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_role_permission idx_3095574_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_role_permission"
    ADD CONSTRAINT "idx_3095574_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_security_bootstrap idx_3095586_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_security_bootstrap"
    ADD CONSTRAINT "idx_3095586_PRIMARY" PRIMARY KEY ("bootstrapKey");


--
-- Name: sys_storage_alert_log idx_3095597_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_storage_alert_log"
    ADD CONSTRAINT "idx_3095597_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_storage_strategy idx_3095612_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_storage_strategy"
    ADD CONSTRAINT "idx_3095612_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_upload_task idx_3095643_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_upload_task"
    ADD CONSTRAINT "idx_3095643_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_user idx_3095657_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user"
    ADD CONSTRAINT "idx_3095657_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_user_credential idx_3095680_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_credential"
    ADD CONSTRAINT "idx_3095680_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_user_dept idx_3095700_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_dept"
    ADD CONSTRAINT "idx_3095700_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_user_org idx_3095712_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_org"
    ADD CONSTRAINT "idx_3095712_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_user_permission idx_3095727_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_permission"
    ADD CONSTRAINT "idx_3095727_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_user_position idx_3095743_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_position"
    ADD CONSTRAINT "idx_3095743_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sys_user_role idx_3095758_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sys_user_role"
    ADD CONSTRAINT "idx_3095758_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysExternalLoginProvider idx_3095771_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalLoginProvider"
    ADD CONSTRAINT "idx_3095771_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysExternalManagedProviderCommand idx_3095807_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalManagedProviderCommand"
    ADD CONSTRAINT "idx_3095807_PRIMARY" PRIMARY KEY ("providerCode", "connectionVersion");


--
-- Name: sysExternalOAuthLoginState idx_3095815_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalOAuthLoginState"
    ADD CONSTRAINT "idx_3095815_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysExternalOAuthToken idx_3095836_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalOAuthToken"
    ADD CONSTRAINT "idx_3095836_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysExternalProviderMethod idx_3095862_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalProviderMethod"
    ADD CONSTRAINT "idx_3095862_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysExternalUserIdentity idx_3095881_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysExternalUserIdentity"
    ADD CONSTRAINT "idx_3095881_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysFederatedNode idx_3095903_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysFederatedNode"
    ADD CONSTRAINT "idx_3095903_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysFederatedNodeConnectionCommand idx_3095925_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysFederatedNodeConnectionCommand"
    ADD CONSTRAINT "idx_3095925_PRIMARY" PRIMARY KEY ("nodeCode", "connectionVersion");


--
-- Name: sysNotificationChannel idx_3095937_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysNotificationChannel"
    ADD CONSTRAINT "idx_3095937_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysNotificationDelivery idx_3095956_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysNotificationDelivery"
    ADD CONSTRAINT "idx_3095956_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysNotificationSceneBinding idx_3095980_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysNotificationSceneBinding"
    ADD CONSTRAINT "idx_3095980_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysNotificationTemplate idx_3096004_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysNotificationTemplate"
    ADD CONSTRAINT "idx_3096004_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysPlatform idx_3096027_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatform"
    ADD CONSTRAINT "idx_3096027_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysPlatformDefaultRole idx_3096053_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformDefaultRole"
    ADD CONSTRAINT "idx_3096053_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysPlatformLoginMethod idx_3096071_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformLoginMethod"
    ADD CONSTRAINT "idx_3096071_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysPlatformSourceRule idx_3096096_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformSourceRule"
    ADD CONSTRAINT "idx_3096096_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysPlatformSsoClient idx_3096117_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysPlatformSsoClient"
    ADD CONSTRAINT "idx_3096117_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoAuditLog idx_3096133_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoAuditLog"
    ADD CONSTRAINT "idx_3096133_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoAuthorizationCode idx_3096147_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoAuthorizationCode"
    ADD CONSTRAINT "idx_3096147_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoClient idx_3096170_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoClient"
    ADD CONSTRAINT "idx_3096170_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoClientRedirectUri idx_3096203_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoClientRedirectUri"
    ADD CONSTRAINT "idx_3096203_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoClientSecret idx_3096221_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoClientSecret"
    ADD CONSTRAINT "idx_3096221_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoConsentGrant idx_3096239_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoConsentGrant"
    ADD CONSTRAINT "idx_3096239_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoIssuerKey idx_3096259_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoIssuerKey"
    ADD CONSTRAINT "idx_3096259_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoRefreshTokenFamily idx_3096277_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoRefreshTokenFamily"
    ADD CONSTRAINT "idx_3096277_PRIMARY" PRIMARY KEY ("id");


--
-- Name: sysSsoSession idx_3096301_PRIMARY; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."sysSsoSession"
    ADD CONSTRAINT "idx_3096301_PRIMARY" PRIMARY KEY ("id");


--
-- Name: idx_3095026_idx_docker_compose_project_operation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095026_idx_docker_compose_project_operation" ON "public"."docker_compose_project" USING "btree" ("lastOperationId");


--
-- Name: idx_3095026_idx_docker_compose_project_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095026_idx_docker_compose_project_status" ON "public"."docker_compose_project" USING "btree" ("status", "deleted");


--
-- Name: idx_3095026_uk_docker_compose_project_id_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095026_uk_docker_compose_project_id_deleted" ON "public"."docker_compose_project" USING "btree" ("projectId", "deleted");


--
-- Name: idx_3095026_uk_docker_compose_project_name_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095026_uk_docker_compose_project_name_deleted" ON "public"."docker_compose_project" USING "btree" ("projectName", "deleted");


--
-- Name: idx_3095044_idx_docker_operation_actor; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095044_idx_docker_operation_actor" ON "public"."docker_operation" USING "btree" ("actorUserId", "updateTime");


--
-- Name: idx_3095044_idx_docker_operation_retry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095044_idx_docker_operation_retry" ON "public"."docker_operation" USING "btree" ("retryOf");


--
-- Name: idx_3095044_idx_docker_operation_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095044_idx_docker_operation_status" ON "public"."docker_operation" USING "btree" ("status", "updateTime");


--
-- Name: idx_3095044_idx_docker_operation_target; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095044_idx_docker_operation_target" ON "public"."docker_operation" USING "btree" ("targetType", "targetId", "targetName", "updateTime");


--
-- Name: idx_3095044_idx_docker_operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095044_idx_docker_operation_type" ON "public"."docker_operation" USING "btree" ("operationType", "updateTime");


--
-- Name: idx_3095061_idx_docker_operation_event_operation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095061_idx_docker_operation_event_operation" ON "public"."docker_operation_event" USING "btree" ("operationId", "occurredAt");


--
-- Name: idx_3095061_uk_docker_operation_event_sequence; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095061_uk_docker_operation_event_sequence" ON "public"."docker_operation_event" USING "btree" ("operationId", "sequence");


--
-- Name: idx_3095072_idx_docker_registry_default; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095072_idx_docker_registry_default" ON "public"."docker_remote_registry" USING "btree" ("defaultRegistry", "deleted");


--
-- Name: idx_3095072_idx_docker_registry_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095072_idx_docker_registry_status" ON "public"."docker_remote_registry" USING "btree" ("status", "deleted");


--
-- Name: idx_3095072_uk_docker_registry_code_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095072_uk_docker_registry_code_deleted" ON "public"."docker_remote_registry" USING "btree" ("code", "deleted");


--
-- Name: idx_3095111_idx_configKey; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095111_idx_configKey" ON "public"."sys_config" USING "btree" ("configKey");


--
-- Name: idx_3095111_idx_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095111_idx_enabled" ON "public"."sys_config" USING "btree" ("isEnabled", "isDeleted");


--
-- Name: idx_3095111_idx_groupId; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095111_idx_groupId" ON "public"."sys_config" USING "btree" ("groupId");


--
-- Name: idx_3095111_idx_sensitive; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095111_idx_sensitive" ON "public"."sys_config" USING "btree" ("isSensitive");


--
-- Name: idx_3095111_uk_configKey_groupId; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095111_uk_configKey_groupId" ON "public"."sys_config" USING "btree" ("configKey", "groupId");


--
-- Name: idx_3095142_idx_config_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_config_id" ON "public"."sys_config_change_log" USING "btree" ("configId");


--
-- Name: idx_3095142_idx_config_key; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_config_key" ON "public"."sys_config_change_log" USING "btree" ("configKey");


--
-- Name: idx_3095142_idx_operation_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_operation_time" ON "public"."sys_config_change_log" USING "btree" ("operationTime");


--
-- Name: idx_3095142_idx_operation_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_operation_type" ON "public"."sys_config_change_log" USING "btree" ("operationType");


--
-- Name: idx_3095142_idx_operator_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_operator_id" ON "public"."sys_config_change_log" USING "btree" ("operatorId");


--
-- Name: idx_3095142_idx_parent_log_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_parent_log_id" ON "public"."sys_config_change_log" USING "btree" ("parentLogId");


--
-- Name: idx_3095142_idx_related_log_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_related_log_id" ON "public"."sys_config_change_log" USING "btree" ("relatedLogId");


--
-- Name: idx_3095142_idx_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095142_idx_status" ON "public"."sys_config_change_log" USING "btree" ("status");


--
-- Name: idx_3095159_idx_module; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095159_idx_module" ON "public"."sys_config_group" USING "btree" ("module");


--
-- Name: idx_3095159_idx_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095159_idx_status" ON "public"."sys_config_group" USING "btree" ("status", "isDeleted");


--
-- Name: idx_3095159_uk_groupCode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095159_uk_groupCode" ON "public"."sys_config_group" USING "btree" ("groupCode");


--
-- Name: idx_3095178_idx_dept_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095178_idx_dept_org" ON "public"."sys_dept" USING "btree" ("orgId", "isDeleted");


--
-- Name: idx_3095178_idx_dept_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095178_idx_dept_parent" ON "public"."sys_dept" USING "btree" ("parentId", "isDeleted");


--
-- Name: idx_3095178_idx_dept_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095178_idx_dept_status" ON "public"."sys_dept" USING "btree" ("status", "isDeleted");


--
-- Name: idx_3095178_uk_dept_code_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095178_uk_dept_code_deleted" ON "public"."sys_dept" USING "btree" ("code", "isDeleted");


--
-- Name: idx_3095201_idx_type_status_sort; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095201_idx_type_status_sort" ON "public"."sys_dict_item" USING "btree" ("dictTypeId", "status", "sortOrder");


--
-- Name: idx_3095201_uk_type_value; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095201_uk_type_value" ON "public"."sys_dict_item" USING "btree" ("dictTypeId", "itemValue");


--
-- Name: idx_3095224_idx_module_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095224_idx_module_status" ON "public"."sys_dict_type" USING "btree" ("module", "status");


--
-- Name: idx_3095224_uk_dictCode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095224_uk_dictCode" ON "public"."sys_dict_type" USING "btree" ("dictCode");


--
-- Name: idx_3095249_idx_file_binding_status_retry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095249_idx_file_binding_status_retry" ON "public"."sys_file_binding_task" USING "btree" ("status", "nextRetryTime");


--
-- Name: idx_3095249_idx_file_binding_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095249_idx_file_binding_user" ON "public"."sys_file_binding_task" USING "btree" ("userId");


--
-- Name: idx_3095249_uk_file_binding_file_token; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095249_uk_file_binding_file_token" ON "public"."sys_file_binding_task" USING "btree" ("fileId", "bindingToken");


--
-- Name: idx_3095267_idx_chunk_expire; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095267_idx_chunk_expire" ON "public"."sys_file_chunk_upload" USING "btree" ("expireTime");


--
-- Name: idx_3095267_idx_chunk_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095267_idx_chunk_user_status" ON "public"."sys_file_chunk_upload" USING "btree" ("userId", "status", "expireTime");


--
-- Name: idx_3095267_uk_chunk_upload_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095267_uk_chunk_upload_id" ON "public"."sys_file_chunk_upload" USING "btree" ("uploadId");


--
-- Name: idx_3095286_idx_createTime; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095286_idx_createTime" ON "public"."sys_file_info" USING "btree" ("createTime");


--
-- Name: idx_3095286_idx_fileInnerName; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095286_idx_fileInnerName" ON "public"."sys_file_info" USING "btree" ("fileInnerName");


--
-- Name: idx_3095286_idx_sha256; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095286_idx_sha256" ON "public"."sys_file_info" USING "btree" ("fileSha256");


--
-- Name: idx_3095286_idx_storage_strategy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095286_idx_storage_strategy" ON "public"."sys_file_info" USING "btree" ("storageStrategyId");


--
-- Name: idx_3095286_uk_sha256_size; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095286_uk_sha256_size" ON "public"."sys_file_info" USING "btree" ("fileSha256", "fileSize");


--
-- Name: idx_3095302_idx_integrity_file; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095302_idx_integrity_file" ON "public"."sys_file_integrity_audit" USING "btree" ("fileId");


--
-- Name: idx_3095302_idx_integrity_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095302_idx_integrity_status" ON "public"."sys_file_integrity_audit" USING "btree" ("status", "auditTime");


--
-- Name: idx_3095313_idx_run_file; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095313_idx_run_file" ON "public"."sys_file_process_run" USING "btree" ("fileId");


--
-- Name: idx_3095313_idx_run_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095313_idx_run_status" ON "public"."sys_file_process_run" USING "btree" ("status");


--
-- Name: idx_3095313_idx_run_task; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095313_idx_run_task" ON "public"."sys_file_process_run" USING "btree" ("taskId");


--
-- Name: idx_3095328_idx_fileId; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095328_idx_fileId" ON "public"."sys_file_process_task" USING "btree" ("fileId");


--
-- Name: idx_3095328_idx_nextRetryTime; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095328_idx_nextRetryTime" ON "public"."sys_file_process_task" USING "btree" ("nextRetryTime");


--
-- Name: idx_3095328_idx_status_priority; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095328_idx_status_priority" ON "public"."sys_file_process_task" USING "btree" ("status", "priority");


--
-- Name: idx_3095328_idx_taskType; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095328_idx_taskType" ON "public"."sys_file_process_task" USING "btree" ("taskType");


--
-- Name: idx_3095328_uk_process_task_idempotency; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095328_uk_process_task_idempotency" ON "public"."sys_file_process_task" USING "btree" ("idempotencyKey");


--
-- Name: idx_3095349_idx_fileId; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095349_idx_fileId" ON "public"."sys_file_reference" USING "btree" ("fileId");


--
-- Name: idx_3095349_idx_user_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095349_idx_user_business" ON "public"."sys_file_reference" USING "btree" ("userId", "bizType", "bizId");


--
-- Name: idx_3095349_uk_user_biz_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095349_uk_user_biz_active" ON "public"."sys_file_reference" USING "btree" ("userId", "bizType", "bizId", "isDeleted");


--
-- Name: idx_3095370_idx_menu_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095370_idx_menu_parent" ON "public"."sys_menu" USING "btree" ("parentId");


--
-- Name: idx_3095370_idx_menu_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095370_idx_menu_status" ON "public"."sys_menu" USING "btree" ("status");


--
-- Name: idx_3095370_idx_menu_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095370_idx_menu_type" ON "public"."sys_menu" USING "btree" ("type");


--
-- Name: idx_3095392_uk_menu_permission; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095392_uk_menu_permission" ON "public"."sys_menu_permission" USING "btree" ("menuId", "permissionId");


--
-- Name: idx_3095400_idx_message_consume_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095400_idx_message_consume_time" ON "public"."sys_message_consume_log" USING "btree" ("createTime");


--
-- Name: idx_3095400_uk_message_consume; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095400_uk_message_consume" ON "public"."sys_message_consume_log" USING "btree" ("messageId", "consumer");


--
-- Name: idx_3095412_idx_operation_log_deleted_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095412_idx_operation_log_deleted_time" ON "public"."sys_operation_log" USING "btree" ("isDeleted", "operationTime");


--
-- Name: idx_3095412_idx_operation_log_method; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095412_idx_operation_log_method" ON "public"."sys_operation_log" USING "btree" ("requestMethod");


--
-- Name: idx_3095412_idx_operation_log_trace_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095412_idx_operation_log_trace_id" ON "public"."sys_operation_log" USING "btree" ("traceId");


--
-- Name: idx_3095412_idx_operation_log_type_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095412_idx_operation_log_type_time" ON "public"."sys_operation_log" USING "btree" ("operationType", "operationTime");


--
-- Name: idx_3095412_idx_operation_log_url; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095412_idx_operation_log_url" ON "public"."sys_operation_log" USING "btree" ("requestUrl");


--
-- Name: idx_3095412_idx_operation_log_user_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095412_idx_operation_log_user_time" ON "public"."sys_operation_log" USING "btree" ("userId", "operationTime");


--
-- Name: idx_3095430_idx_org_parent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095430_idx_org_parent" ON "public"."sys_org" USING "btree" ("parentId", "isDeleted");


--
-- Name: idx_3095430_idx_org_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095430_idx_org_status" ON "public"."sys_org" USING "btree" ("status", "isDeleted");


--
-- Name: idx_3095430_uk_org_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095430_uk_org_code" ON "public"."sys_org" USING "btree" ("code");


--
-- Name: idx_3095450_idx_outbox_aggregate; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095450_idx_outbox_aggregate" ON "public"."sys_outbox_event" USING "btree" ("aggregateType", "aggregateId");


--
-- Name: idx_3095450_idx_outbox_status_retry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095450_idx_outbox_status_retry" ON "public"."sys_outbox_event" USING "btree" ("status", "nextRetryAt");


--
-- Name: idx_3095450_uk_outbox_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095450_uk_outbox_event_id" ON "public"."sys_outbox_event" USING "btree" ("eventId");


--
-- Name: idx_3095468_idx_permission_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095468_idx_permission_status" ON "public"."sys_permission" USING "btree" ("status");


--
-- Name: idx_3095468_uk_permission_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095468_uk_permission_code" ON "public"."sys_permission" USING "btree" ("code");


--
-- Name: idx_3095486_idx_post_dept; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095486_idx_post_dept" ON "public"."sys_post" USING "btree" ("deptId", "isDeleted");


--
-- Name: idx_3095486_idx_post_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095486_idx_post_org" ON "public"."sys_post" USING "btree" ("orgId", "isDeleted");


--
-- Name: idx_3095486_idx_post_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095486_idx_post_status" ON "public"."sys_post" USING "btree" ("status", "isDeleted");


--
-- Name: idx_3095509_idx_post_role_post; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095509_idx_post_role_post" ON "public"."sys_post_role" USING "btree" ("postId");


--
-- Name: idx_3095509_idx_post_role_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095509_idx_post_role_role" ON "public"."sys_post_role" USING "btree" ("roleId");


--
-- Name: idx_3095509_uk_post_role; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095509_uk_post_role" ON "public"."sys_post_role" USING "btree" ("postId", "roleId");


--
-- Name: idx_3095516_idx_role_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095516_idx_role_code" ON "public"."sys_role" USING "btree" ("code");


--
-- Name: idx_3095516_idx_role_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095516_idx_role_status" ON "public"."sys_role" USING "btree" ("status");


--
-- Name: idx_3095516_uk_sys_role_system_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095516_uk_sys_role_system_key" ON "public"."sys_role" USING "btree" ("systemKey");


--
-- Name: idx_3095538_idx_role_config_scope_group; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095538_idx_role_config_scope_group" ON "public"."sys_role_config_scope" USING "btree" ("groupCode", "configKey", "isDeleted");


--
-- Name: idx_3095538_idx_role_config_scope_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095538_idx_role_config_scope_role" ON "public"."sys_role_config_scope" USING "btree" ("roleId", "isDeleted");


--
-- Name: idx_3095538_uk_role_config_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095538_uk_role_config_scope" ON "public"."sys_role_config_scope" USING "btree" ("roleId", "groupCode", "configKey");


--
-- Name: idx_3095558_uk_role_dept; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095558_uk_role_dept" ON "public"."sys_role_dept" USING "btree" ("roleId", "deptId");


--
-- Name: idx_3095564_idx_role_menu_menu; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095564_idx_role_menu_menu" ON "public"."sys_role_menu" USING "btree" ("menuId");


--
-- Name: idx_3095564_idx_role_menu_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095564_idx_role_menu_role" ON "public"."sys_role_menu" USING "btree" ("roleId");


--
-- Name: idx_3095564_uk_role_menu; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095564_uk_role_menu" ON "public"."sys_role_menu" USING "btree" ("roleId", "menuId");


--
-- Name: idx_3095574_idx_role_permission_permission; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095574_idx_role_permission_permission" ON "public"."sys_role_permission" USING "btree" ("permissionId");


--
-- Name: idx_3095574_idx_role_permission_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095574_idx_role_permission_role" ON "public"."sys_role_permission" USING "btree" ("roleId");


--
-- Name: idx_3095574_uk_role_permission; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095574_uk_role_permission" ON "public"."sys_role_permission" USING "btree" ("roleId", "permissionId");


--
-- Name: idx_3095586_uk_sys_security_bootstrap_root_role; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095586_uk_sys_security_bootstrap_root_role" ON "public"."sys_security_bootstrap" USING "btree" ("rootRoleId");


--
-- Name: idx_3095597_idx_storage_alert_strategy; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095597_idx_storage_alert_strategy" ON "public"."sys_storage_alert_log" USING "btree" ("strategyId", "status");


--
-- Name: idx_3095597_idx_storage_alert_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095597_idx_storage_alert_type" ON "public"."sys_storage_alert_log" USING "btree" ("alertType", "createTime");


--
-- Name: idx_3095612_idx_healthStatus; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095612_idx_healthStatus" ON "public"."sys_storage_strategy" USING "btree" ("healthStatus", "priority");


--
-- Name: idx_3095612_idx_isDefault; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095612_idx_isDefault" ON "public"."sys_storage_strategy" USING "btree" ("isDefault");


--
-- Name: idx_3095612_idx_providerType; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095612_idx_providerType" ON "public"."sys_storage_strategy" USING "btree" ("providerType");


--
-- Name: idx_3095612_idx_runState; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095612_idx_runState" ON "public"."sys_storage_strategy" USING "btree" ("runState");


--
-- Name: idx_3095612_uk_strategyName; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095612_uk_strategyName" ON "public"."sys_storage_strategy" USING "btree" ("strategyName");


--
-- Name: idx_3095643_idx_expire; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095643_idx_expire" ON "public"."sys_upload_task" USING "btree" ("expireAt");


--
-- Name: idx_3095643_idx_file; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095643_idx_file" ON "public"."sys_upload_task" USING "btree" ("fileId");


--
-- Name: idx_3095643_idx_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095643_idx_status" ON "public"."sys_upload_task" USING "btree" ("status");


--
-- Name: idx_3095643_idx_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095643_idx_user" ON "public"."sys_upload_task" USING "btree" ("userId");


--
-- Name: idx_3095657_idx_user_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095657_idx_user_email" ON "public"."sys_user" USING "btree" ("userEmail");


--
-- Name: idx_3095657_idx_user_phone; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095657_idx_user_phone" ON "public"."sys_user" USING "btree" ("userPhone");


--
-- Name: idx_3095657_idx_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095657_idx_user_status" ON "public"."sys_user" USING "btree" ("status", "isDeleted");


--
-- Name: idx_3095657_uk_user_account_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095657_uk_user_account_deleted" ON "public"."sys_user" USING "btree" ("userAccount", "isDeleted");


--
-- Name: idx_3095680_idx_user_credential_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095680_idx_user_credential_status" ON "public"."sys_user_credential" USING "btree" ("status", "isDeleted");


--
-- Name: idx_3095680_idx_user_credential_user_type_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095680_idx_user_credential_user_type_status" ON "public"."sys_user_credential" USING "btree" ("userId", "credentialType", "status", "isDeleted");


--
-- Name: idx_3095680_uk_credential_type_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095680_uk_credential_type_key" ON "public"."sys_user_credential" USING "btree" ("credentialType", "credentialKey", "isDeleted");


--
-- Name: idx_3095680_uk_user_credential_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095680_uk_user_credential_scope" ON "public"."sys_user_credential" USING "btree" ("userId", "credentialType", "credentialKey", "isDeleted");


--
-- Name: idx_3095700_idx_user_dept_dept; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095700_idx_user_dept_dept" ON "public"."sys_user_dept" USING "btree" ("deptId");


--
-- Name: idx_3095700_idx_user_dept_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095700_idx_user_dept_user" ON "public"."sys_user_dept" USING "btree" ("userId");


--
-- Name: idx_3095700_uk_user_dept; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095700_uk_user_dept" ON "public"."sys_user_dept" USING "btree" ("userId", "deptId");


--
-- Name: idx_3095712_idx_user_org_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095712_idx_user_org_org" ON "public"."sys_user_org" USING "btree" ("orgId", "isDeleted");


--
-- Name: idx_3095712_idx_user_org_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095712_idx_user_org_user" ON "public"."sys_user_org" USING "btree" ("userId", "isDeleted");


--
-- Name: idx_3095712_uk_user_org; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095712_uk_user_org" ON "public"."sys_user_org" USING "btree" ("userId", "orgId");


--
-- Name: idx_3095727_uk_user_permission; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095727_uk_user_permission" ON "public"."sys_user_permission" USING "btree" ("userId", "permissionId");


--
-- Name: idx_3095743_idx_user_position_post; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095743_idx_user_position_post" ON "public"."sys_user_position" USING "btree" ("postId", "isDeleted");


--
-- Name: idx_3095743_idx_user_position_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095743_idx_user_position_user" ON "public"."sys_user_position" USING "btree" ("userId", "isDeleted");


--
-- Name: idx_3095743_uk_user_post; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095743_uk_user_post" ON "public"."sys_user_position" USING "btree" ("userId", "postId");


--
-- Name: idx_3095758_idx_user_role_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095758_idx_user_role_role" ON "public"."sys_user_role" USING "btree" ("roleId", "isDeleted");


--
-- Name: idx_3095758_idx_user_role_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095758_idx_user_role_user" ON "public"."sys_user_role" USING "btree" ("userId", "isDeleted");


--
-- Name: idx_3095758_uk_user_role; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095758_uk_user_role" ON "public"."sys_user_role" USING "btree" ("userId", "roleId");


--
-- Name: idx_3095771_idx_sysExternalLoginProvider_display_login_status_d; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095771_idx_sysExternalLoginProvider_display_login_status_d" ON "public"."sysExternalLoginProvider" USING "btree" ("displayEnabled", "loginEnabled", "status", "isDeleted");


--
-- Name: idx_3095771_uk_sysExternalLoginProvider_code_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095771_uk_sysExternalLoginProvider_code_deleted" ON "public"."sysExternalLoginProvider" USING "btree" ("providerCode", "isDeleted");


--
-- Name: idx_3095815_idxSysExternalOAuthLoginStateProvisioningAuthorityI; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095815_idxSysExternalOAuthLoginStateProvisioningAuthorityI" ON "public"."sysExternalOAuthLoginState" USING "btree" ("provisioningAuthorityId");


--
-- Name: idx_3095815_idx_sysExternalOAuthLoginState_platform_status_dele; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095815_idx_sysExternalOAuthLoginState_platform_status_dele" ON "public"."sysExternalOAuthLoginState" USING "btree" ("platformCode", "status", "isDeleted");


--
-- Name: idx_3095815_idx_sysExternalOAuthLoginState_provider_status_expi; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095815_idx_sysExternalOAuthLoginState_provider_status_expi" ON "public"."sysExternalOAuthLoginState" USING "btree" ("providerCode", "status", "expiresAt", "isDeleted");


--
-- Name: idx_3095815_idx_sysExternalOAuthLoginState_stateHash_status_del; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095815_idx_sysExternalOAuthLoginState_stateHash_status_del" ON "public"."sysExternalOAuthLoginState" USING "btree" ("stateHash", "status", "isDeleted");


--
-- Name: idx_3095815_uk_sysExternalOAuthLoginState_stateId_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095815_uk_sysExternalOAuthLoginState_stateId_deleted" ON "public"."sysExternalOAuthLoginState" USING "btree" ("stateId", "isDeleted");


--
-- Name: idx_3095836_idx_sysExternalOAuthToken_provider_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095836_idx_sysExternalOAuthToken_provider_status_deleted" ON "public"."sysExternalOAuthToken" USING "btree" ("providerCode", "status", "isDeleted");


--
-- Name: idx_3095836_idx_sysExternalOAuthToken_user_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095836_idx_sysExternalOAuthToken_user_status_deleted" ON "public"."sysExternalOAuthToken" USING "btree" ("userId", "status", "isDeleted");


--
-- Name: idx_3095836_uk_sysExternalOAuthToken_identity_purpose_scope_del; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095836_uk_sysExternalOAuthToken_identity_purpose_scope_del" ON "public"."sysExternalOAuthToken" USING "btree" ("identityId", "tokenPurpose", "scopeHash", "isDeleted");


--
-- Name: idx_3095862_idx_sysExternalProviderMethod_capability_status_del; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095862_idx_sysExternalProviderMethod_capability_status_del" ON "public"."sysExternalProviderMethod" USING "btree" ("capabilityCode", "status", "isDeleted");


--
-- Name: idx_3095862_uk_sysExternalProviderMethod_provider_method_delete; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095862_uk_sysExternalProviderMethod_provider_method_delete" ON "public"."sysExternalProviderMethod" USING "btree" ("providerCode", "methodKey", "isDeleted");


--
-- Name: idx_3095881_idx_sysExternalUserIdentity_provider_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095881_idx_sysExternalUserIdentity_provider_status_deleted" ON "public"."sysExternalUserIdentity" USING "btree" ("providerCode", "status", "isDeleted");


--
-- Name: idx_3095881_idx_sysExternalUserIdentity_user_provider_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095881_idx_sysExternalUserIdentity_user_provider_deleted" ON "public"."sysExternalUserIdentity" USING "btree" ("userId", "providerCode", "isDeleted");


--
-- Name: idx_3095881_uk_sysExternalUserIdentity_issuer_subject_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095881_uk_sysExternalUserIdentity_issuer_subject_deleted" ON "public"."sysExternalUserIdentity" USING "btree" ("externalIdentityDigest", "isDeleted");


--
-- Name: idx_3095881_uk_sysExternalUserIdentity_provider_subject_digest_; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095881_uk_sysExternalUserIdentity_provider_subject_digest_" ON "public"."sysExternalUserIdentity" USING "btree" ("providerSubjectDigest", "isDeleted");


--
-- Name: idx_3095903_idx_sysFederatedNode_connectionStatus; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095903_idx_sysFederatedNode_connectionStatus" ON "public"."sysFederatedNode" USING "btree" ("connectionStatus");


--
-- Name: idx_3095903_idx_sysFederatedNode_status_updatedAt; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095903_idx_sysFederatedNode_status_updatedAt" ON "public"."sysFederatedNode" USING "btree" ("status", "updatedAt");


--
-- Name: idx_3095903_uk_sysFederatedNode_nodeCode_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095903_uk_sysFederatedNode_nodeCode_active" ON "public"."sysFederatedNode" USING "btree" ("nodeCode", "activeKey");


--
-- Name: idx_3095925_idx_sysFederatedNodeConnectionCommand_state_updated; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095925_idx_sysFederatedNodeConnectionCommand_state_updated" ON "public"."sysFederatedNodeConnectionCommand" USING "btree" ("terminalState", "updatedAt");


--
-- Name: idx_3095937_idxNotificationChannelTypeStatus; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095937_idxNotificationChannelTypeStatus" ON "public"."sysNotificationChannel" USING "btree" ("channelType", "status", "isDeleted");


--
-- Name: idx_3095937_ukNotificationChannelCode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095937_ukNotificationChannelCode" ON "public"."sysNotificationChannel" USING "btree" ("channelCode", "isDeleted");


--
-- Name: idx_3095956_idxNotificationDeliveryScene; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095956_idxNotificationDeliveryScene" ON "public"."sysNotificationDelivery" USING "btree" ("sceneCode", "createTime");


--
-- Name: idx_3095956_idxNotificationDeliveryStatus; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095956_idxNotificationDeliveryStatus" ON "public"."sysNotificationDelivery" USING "btree" ("status", "nextRetryAt", "isDeleted");


--
-- Name: idx_3095956_ukNotificationDeliveryDigest; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095956_ukNotificationDeliveryDigest" ON "public"."sysNotificationDelivery" USING "btree" ("requestDigest", "isDeleted");


--
-- Name: idx_3095956_ukNotificationDeliveryId; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3095956_ukNotificationDeliveryId" ON "public"."sysNotificationDelivery" USING "btree" ("deliveryId");


--
-- Name: idx_3095980_idxNotificationSceneBindingChannel; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095980_idxNotificationSceneBindingChannel" ON "public"."sysNotificationSceneBinding" USING "btree" ("channelCode", "isDeleted");


--
-- Name: idx_3095980_idxNotificationSceneBindingScene; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3095980_idxNotificationSceneBindingScene" ON "public"."sysNotificationSceneBinding" USING "btree" ("sceneCode", "enabled", "priority", "isDeleted");


--
-- Name: idx_3096004_idxNotificationTemplateScene; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096004_idxNotificationTemplateScene" ON "public"."sysNotificationTemplate" USING "btree" ("sceneCode", "channelType", "locale", "status", "isDeleted");


--
-- Name: idx_3096004_ukNotificationTemplateCode; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096004_ukNotificationTemplateCode" ON "public"."sysNotificationTemplate" USING "btree" ("templateCode", "isDeleted");


--
-- Name: idx_3096027_idx_sysPlatform_default_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096027_idx_sysPlatform_default_status_deleted" ON "public"."sysPlatform" USING "btree" ("isDefault", "status", "isDeleted");


--
-- Name: idx_3096027_idx_sysPlatform_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096027_idx_sysPlatform_status_deleted" ON "public"."sysPlatform" USING "btree" ("status", "isDeleted");


--
-- Name: idx_3096027_uk_sysPlatform_code_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096027_uk_sysPlatform_code_deleted" ON "public"."sysPlatform" USING "btree" ("platformCode", "isDeleted");


--
-- Name: idx_3096053_idx_sysPlatformDefaultRole_platform_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096053_idx_sysPlatformDefaultRole_platform_status_deleted" ON "public"."sysPlatformDefaultRole" USING "btree" ("platformCode", "status", "isDeleted");


--
-- Name: idx_3096053_uk_sysPlatformDefaultRole_platform_role_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096053_uk_sysPlatformDefaultRole_platform_role_deleted" ON "public"."sysPlatformDefaultRole" USING "btree" ("platformCode", "roleId", "isDeleted");


--
-- Name: idx_3096071_idx_sysPlatformLoginMethod_platform_display_login_d; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096071_idx_sysPlatformLoginMethod_platform_display_login_d" ON "public"."sysPlatformLoginMethod" USING "btree" ("platformCode", "displayEnabled", "loginEnabled", "isDeleted");


--
-- Name: idx_3096071_uk_sysPlatformLoginMethod_platform_method_provider_; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096071_uk_sysPlatformLoginMethod_platform_method_provider_" ON "public"."sysPlatformLoginMethod" USING "btree" ("platformCode", "methodType", "providerCode", "isDeleted");


--
-- Name: idx_3096096_idx_sysPlatformSourceRule_type_status_priority_dele; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096096_idx_sysPlatformSourceRule_type_status_priority_dele" ON "public"."sysPlatformSourceRule" USING "btree" ("matchType", "status", "priority", "isDeleted");


--
-- Name: idx_3096096_uk_sysPlatformSourceRule_platform_type_value_delete; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096096_uk_sysPlatformSourceRule_platform_type_value_delete" ON "public"."sysPlatformSourceRule" USING "btree" ("platformCode", "matchType", "matchValue", "isDeleted");


--
-- Name: idx_3096117_idx_sysPlatformSsoClient_client_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096117_idx_sysPlatformSsoClient_client_status_deleted" ON "public"."sysPlatformSsoClient" USING "btree" ("clientId", "status", "isDeleted");


--
-- Name: idx_3096117_uk_sysPlatformSsoClient_platform_client_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096117_uk_sysPlatformSsoClient_platform_client_deleted" ON "public"."sysPlatformSsoClient" USING "btree" ("platformCode", "clientId", "isDeleted");


--
-- Name: idx_3096133_idx_sysSsoAuditLog_trace_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096133_idx_sysSsoAuditLog_trace_deleted" ON "public"."sysSsoAuditLog" USING "btree" ("traceId", "isDeleted");


--
-- Name: idx_3096147_idx_sysSsoAuthorizationCode_client_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096147_idx_sysSsoAuthorizationCode_client_user_status" ON "public"."sysSsoAuthorizationCode" USING "btree" ("clientId", "userId", "status");


--
-- Name: idx_3096147_uk_sysSsoAuthorizationCode_code_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096147_uk_sysSsoAuthorizationCode_code_deleted" ON "public"."sysSsoAuthorizationCode" USING "btree" ("code", "isDeleted");


--
-- Name: idx_3096170_idx_sysSsoClient_type_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096170_idx_sysSsoClient_type_status_deleted" ON "public"."sysSsoClient" USING "btree" ("clientType", "status", "isDeleted");


--
-- Name: idx_3096170_uk_sysSsoClient_clientId_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096170_uk_sysSsoClient_clientId_deleted" ON "public"."sysSsoClient" USING "btree" ("clientId", "isDeleted");


--
-- Name: idx_3096203_idx_sysSsoClientRedirectUri_client_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096203_idx_sysSsoClientRedirectUri_client_status_deleted" ON "public"."sysSsoClientRedirectUri" USING "btree" ("clientId", "status", "isDeleted");


--
-- Name: idx_3096203_uk_sysSsoClientRedirectUri_client_uri_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096203_uk_sysSsoClientRedirectUri_client_uri_deleted" ON "public"."sysSsoClientRedirectUri" USING "btree" ("clientId", "redirectUri", "isDeleted");


--
-- Name: idx_3096221_idx_sysSsoClientSecret_client_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096221_idx_sysSsoClientSecret_client_status_deleted" ON "public"."sysSsoClientSecret" USING "btree" ("clientId", "status", "isDeleted");


--
-- Name: idx_3096239_uk_sysSsoConsentGrant_user_client_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096239_uk_sysSsoConsentGrant_user_client_deleted" ON "public"."sysSsoConsentGrant" USING "btree" ("userId", "clientId", "isDeleted");


--
-- Name: idx_3096259_uk_sysSsoIssuerKey_kid_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096259_uk_sysSsoIssuerKey_kid_deleted" ON "public"."sysSsoIssuerKey" USING "btree" ("kid", "isDeleted");


--
-- Name: idx_3096277_idx_sysSsoRefreshTokenFamily_currentTokenHash_delet; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096277_idx_sysSsoRefreshTokenFamily_currentTokenHash_delet" ON "public"."sysSsoRefreshTokenFamily" USING "btree" ("currentTokenHash", "isDeleted");


--
-- Name: idx_3096277_idx_sysSsoRefreshTokenFamily_user_status_deleted_cr; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096277_idx_sysSsoRefreshTokenFamily_user_status_deleted_cr" ON "public"."sysSsoRefreshTokenFamily" USING "btree" ("userId", "status", "isDeleted", "createTime");


--
-- Name: idx_3096277_uk_sysSsoRefreshTokenFamily_family_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096277_uk_sysSsoRefreshTokenFamily_family_deleted" ON "public"."sysSsoRefreshTokenFamily" USING "btree" ("familyId", "isDeleted");


--
-- Name: idx_3096301_idx_sysSsoSession_external_identity_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096301_idx_sysSsoSession_external_identity_status_deleted" ON "public"."sysSsoSession" USING "btree" ("externalIdentityId", "status", "isDeleted");


--
-- Name: idx_3096301_idx_sysSsoSession_external_provider_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096301_idx_sysSsoSession_external_provider_status_deleted" ON "public"."sysSsoSession" USING "btree" ("externalProviderCode", "status", "isDeleted");


--
-- Name: idx_3096301_idx_sysSsoSession_platformCode_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096301_idx_sysSsoSession_platformCode_status_deleted" ON "public"."sysSsoSession" USING "btree" ("platformCode", "status", "isDeleted");


--
-- Name: idx_3096301_idx_sysSsoSession_user_status_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX "idx_3096301_idx_sysSsoSession_user_status_deleted" ON "public"."sysSsoSession" USING "btree" ("userId", "status", "isDeleted");


--
-- Name: idx_3096301_uk_sysSsoSession_sessionId_deleted; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX "idx_3096301_uk_sysSsoSession_sessionId_deleted" ON "public"."sysSsoSession" USING "btree" ("sessionId", "isDeleted");


--
-- Name: docker_operation_event fk_docker_operation_event_operation; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY "public"."docker_operation_event"
    ADD CONSTRAINT "fk_docker_operation_event_operation" FOREIGN KEY ("operationId") REFERENCES "public"."docker_operation"("id") ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

-- Platform enterprise identity tables are part of the unpublished clean
-- install schema. Their relationship integrity is enforced by application and
-- governance checks; deliberately no database foreign keys are introduced.
CREATE TABLE IF NOT EXISTS sys_enterprise_identity_connection (
 id BIGINT PRIMARY KEY, organization_id BIGINT NOT NULL, provider_type VARCHAR(32) NOT NULL,
 connection_role VARCHAR(32) NOT NULL DEFAULT 'PRIMARY', display_name VARCHAR(128) NOT NULL,
 app_id VARCHAR(255), corp_id VARCHAR(255), agent_id VARCHAR(255), endpoint_profile VARCHAR(32) NOT NULL DEFAULT 'OFFICIAL_CN',
 client_secret_ciphertext TEXT NOT NULL, client_secret_edek TEXT NOT NULL, client_secret_wrap_key_ref VARCHAR(128) NOT NULL,
 credential_version BIGINT NOT NULL DEFAULT 1, binding_enabled BOOLEAN NOT NULL DEFAULT TRUE,
 notification_direct_enabled BOOLEAN NOT NULL DEFAULT TRUE, notification_mention_enabled BOOLEAN NOT NULL DEFAULT TRUE,
 login_enabled BOOLEAN NOT NULL DEFAULT FALSE, protocol_capabilities_json JSONB NOT NULL DEFAULT '{}'::jsonb,
 config_digest VARCHAR(128) NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE', last_probe_status VARCHAR(32) NOT NULL DEFAULT 'NOT_PROBED',
 last_probe_reason_code VARCHAR(128) NOT NULL DEFAULT '', last_probe_at TIMESTAMPTZ, version BIGINT NOT NULL DEFAULT 1,
 creator_id BIGINT NOT NULL, updater_id BIGINT NOT NULL, created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sys_enterprise_identity_grant (
 id BIGINT PRIMARY KEY, organization_id BIGINT NOT NULL, external_identity_id BIGINT NOT NULL, grant_type VARCHAR(64) NOT NULL,
 status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE', version BIGINT NOT NULL DEFAULT 1, granted_by BIGINT NOT NULL, granted_at TIMESTAMPTZ NOT NULL,
 revoked_by BIGINT, revoked_at TIMESTAMPTZ, reason_code VARCHAR(128) NOT NULL DEFAULT '', created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE IF NOT EXISTS sys_enterprise_identity_authorization (
 id BIGINT PRIMARY KEY, authorization_identity VARCHAR(128) NOT NULL, purpose VARCHAR(32) NOT NULL, organization_id BIGINT NOT NULL,
 connection_id BIGINT NOT NULL, connection_version BIGINT NOT NULL, connection_config_digest VARCHAR(128) NOT NULL, bind_user_id BIGINT,
 requested_grants_json JSONB NOT NULL DEFAULT '[]'::jsonb, requested_grants_digest VARCHAR(128) NOT NULL, state_hash VARCHAR(128) NOT NULL,
 nonce_hash VARCHAR(128) NOT NULL DEFAULT '', code_verifier_ciphertext TEXT, code_verifier_edek TEXT, code_verifier_wrap_key_ref VARCHAR(128),
 redirect_after VARCHAR(1024) NOT NULL DEFAULT '', expires_at TIMESTAMPTZ NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'ACTIVE',
 result_identity_id BIGINT, reason_code VARCHAR(128) NOT NULL DEFAULT '', consumed_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO sys_permission (id, code, name, "resourceType", method, path, status, description, "createTime", "updateTime", "isDeleted") VALUES
 (2012233100000000001, 'system:enterprise-connection:list', '企业连接列表', 'API', 'GET', '/system/enterprise-connections', 0, '查询本组织企业连接', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false),
 (2012233100000000002, 'system:enterprise-connection:edit', '编辑企业连接', 'API', 'PUT', '/system/enterprise-connections', 0, '保存并检查本组织企业连接', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false),
 (2012233100000000003, 'system:enterprise-connection:status', '启停企业连接', 'API', 'PUT', '/system/enterprise-connections/:connectionId/status', 0, '启用或停用企业连接', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false),
 (2012233100000000004, 'system:enterprise-identity:list', '企业账号成员列表', 'API', 'GET', '/system/enterprise-identities', 0, '查询本组织已绑定成员', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false),
 (2012233100000000005, 'system:enterprise-identity:login-access', '企业账号登录授权', 'API', 'PUT', '/system/enterprise-identities/:identityId/login-access', 0, '授予或撤销企业登录权限', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, false)
ON CONFLICT (code) DO NOTHING;

INSERT INTO sys_menu (id, name, "parentId", "sortOrder", path, component, icon, type, permission, "isFrame", "isCache", visible, hierarchy, level, status, "creatorId", "createTime", "updaterId", "updateTime", "isDeleted", remark)
SELECT 2012233100000000101, '企业连接', parent.id, 51, '/system/enterprise-connections', 'system/enterprise-connections/page', 'LinkOutlined', 'C', 'system:enterprise-connection:list', true, true, true, '/1/1900300200/' || parent.id || '/2012233100000000101', 3, 0, 0, CURRENT_TIMESTAMP, 0, CURRENT_TIMESTAMP, false, '组织企业应用和成员绑定管理'
FROM sys_menu parent WHERE parent.path='/system/access' AND parent."isDeleted"=false
AND NOT EXISTS (SELECT 1 FROM sys_menu WHERE path='/system/enterprise-connections' AND "isDeleted"=false);

INSERT INTO sys_menu_permission (id, "menuId", "permissionId", "creatorId", "createTime")
SELECT 2012233100000000200 + ROW_NUMBER() OVER (ORDER BY p.id), m.id, p.id, 0, CURRENT_TIMESTAMP
FROM sys_menu m JOIN sys_permission p ON p.code LIKE 'system:enterprise-%'
WHERE m.path='/system/enterprise-connections' AND m."isDeleted"=false
AND NOT EXISTS (SELECT 1 FROM sys_menu_permission x WHERE x."menuId"=m.id AND x."permissionId"=p.id);

INSERT INTO sys_role_menu (id, "roleId", "menuId", "createTime", "updateTime")
SELECT 2012233100000000300 + ROW_NUMBER() OVER (ORDER BY r.id), r.id, m.id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM sys_role r JOIN sys_menu m ON m.path='/system/enterprise-connections' AND m."isDeleted"=false
WHERE r.code='SUPER_ADMIN' AND r."isDeleted"=false
AND NOT EXISTS (SELECT 1 FROM sys_role_menu x WHERE x."roleId"=r.id AND x."menuId"=m.id);

INSERT INTO sys_role_permission (id, "roleId", "permissionId", "creatorId", "createTime", "updateTime")
SELECT 2012233100000000400 + ROW_NUMBER() OVER (ORDER BY r.id,p.id), r.id, p.id, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM sys_role r JOIN sys_permission p ON p.code LIKE 'system:enterprise-%' AND p."isDeleted"=false
WHERE r.code='SUPER_ADMIN' AND r."isDeleted"=false
AND NOT EXISTS (SELECT 1 FROM sys_role_permission x WHERE x."roleId"=r.id AND x."permissionId"=p.id);

-- +goose Down

-- +goose StatementBegin
DO $governance$
BEGIN
    RAISE EXCEPTION 'forward-only PostgreSQL clean-install baseline: repair the forward precondition and resume; Down is forbidden';
END
$governance$;
-- +goose StatementEnd
