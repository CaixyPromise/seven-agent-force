-- +goose Up
-- B3 renames notification physical tables in place. Apply it before starting
-- application code that uses the lower snake_case names; no legacy-name
-- process may remain live afterward.
ALTER TABLE "sysNotification" RENAME TO sys_notification;
ALTER TABLE "sysNotificationChannel" RENAME TO sys_notification_channel;
ALTER TABLE "sysNotificationDelivery" RENAME TO sys_notification_delivery;
ALTER TABLE "sysNotificationDeliveryAttempt" RENAME TO sys_notification_delivery_attempt;
ALTER TABLE "sysNotificationDeliveryDiagnosticAudit" RENAME TO sys_notification_delivery_diagnostic_audit;
ALTER TABLE "sysNotificationDeliveryEphemeralContent" RENAME TO sys_notification_delivery_ephemeral_content;
ALTER TABLE "sysNotificationExternalTarget" RENAME TO sys_notification_external_target;
ALTER TABLE "sysNotificationHTTPDeliverySnapshot" RENAME TO sys_notification_http_delivery_snapshot;
ALTER TABLE "sysNotificationMailbox" RENAME TO sys_notification_mailbox;
ALTER TABLE "sysNotificationMaterializationTask" RENAME TO sys_notification_materialization_task;
ALTER TABLE "sysNotificationRecipient" RENAME TO sys_notification_recipient;
ALTER TABLE "sysNotificationSceneBinding" RENAME TO sys_notification_scene_binding;
ALTER TABLE "sysNotificationSceneDefinition" RENAME TO sys_notification_scene_definition;
ALTER TABLE "sysNotificationSceneRevision" RENAME TO sys_notification_scene_revision;
ALTER TABLE "sysNotificationSceneRevisionAudit" RENAME TO sys_notification_scene_revision_audit;
ALTER TABLE "sysNotificationSceneSnapshot" RENAME TO sys_notification_scene_snapshot;
ALTER TABLE "sysNotificationTemplate" RENAME TO sys_notification_template;
ALTER TABLE "sysNotificationTemplateDefinition" RENAME TO sys_notification_template_definition;
ALTER TABLE "sysNotificationTemplateRevision" RENAME TO sys_notification_template_revision;
ALTER TABLE "sysNotificationTemplateRevisionAudit" RENAME TO sys_notification_template_revision_audit;

CREATE TABLE sys_notification_group_route (
    id BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    display_name TEXT NOT NULL,
    provider_type TEXT NOT NULL CHECK (provider_type IN ('FEISHU','WECOM')),
    transport_type TEXT NOT NULL CHECK (transport_type IN ('FEISHU_APP_CHAT','FEISHU_WEBHOOK','WECOM_WEBHOOK')),
    delivery_channel_id BIGINT NOT NULL,
    required_permission_code TEXT NOT NULL DEFAULT 'workforce:task:create',
    target_ciphertext TEXT NOT NULL DEFAULT '',
    target_edek TEXT NOT NULL DEFAULT '',
    target_wrap_key_ref TEXT NOT NULL DEFAULT '',
    target_digest TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'DRAFT' CHECK (status IN ('DRAFT','VERIFIED','ENABLED','DISABLED','RETIRED')),
    route_version BIGINT NOT NULL DEFAULT 1 CHECK (route_version>0),
    channel_version BIGINT NOT NULL DEFAULT 1 CHECK (channel_version>0),
    connection_version BIGINT CHECK (connection_version>0),
    config_digest TEXT NOT NULL,
    capabilities_json JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(capabilities_json)='array'),
    last_probe_status TEXT NOT NULL DEFAULT 'NOT_PROBED',
    last_probe_reason_code TEXT NOT NULL DEFAULT '',
    last_probe_at TIMESTAMPTZ,
    creator_id BIGINT NOT NULL,
    updater_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((transport_type='FEISHU_APP_CHAT' AND target_ciphertext<>'' AND target_edek<>'' AND target_wrap_key_ref<>'' AND target_digest<>'' AND connection_version IS NOT NULL)
        OR (transport_type IN ('FEISHU_WEBHOOK','WECOM_WEBHOOK') AND target_ciphertext='' AND target_edek='' AND target_wrap_key_ref='' AND target_digest='' AND connection_version IS NULL))
);
CREATE UNIQUE INDEX sys_notification_group_route_name_uidx ON sys_notification_group_route (organization_id,lower(display_name)) WHERE status<>'RETIRED';
CREATE UNIQUE INDEX sys_notification_group_route_target_uidx ON sys_notification_group_route (organization_id,provider_type,transport_type,target_digest) WHERE target_digest<>'' AND status<>'RETIRED';
CREATE INDEX sys_notification_group_route_available_idx ON sys_notification_group_route (organization_id,status,provider_type,display_name);

CREATE TABLE sys_notification_group_route_candidate (
    id BIGINT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    organization_id BIGINT NOT NULL,
    connection_id BIGINT NOT NULL,
    connection_version BIGINT NOT NULL,
    connection_config_digest TEXT NOT NULL,
    display_name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    target_ciphertext TEXT NOT NULL,
    target_edek TEXT NOT NULL,
    target_wrap_key_ref TEXT NOT NULL,
    target_digest TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX sys_notification_group_route_candidate_expiry_idx ON sys_notification_group_route_candidate (organization_id,expires_at);

CREATE TABLE sys_notification_group_route_snapshot (
    id BIGINT PRIMARY KEY,
    notification_id TEXT NOT NULL,
    delivery_id TEXT NOT NULL,
    organization_id BIGINT NOT NULL,
    route_id BIGINT NOT NULL,
    route_version BIGINT NOT NULL,
    provider_type TEXT NOT NULL,
    transport_type TEXT NOT NULL,
    route_config_digest TEXT NOT NULL,
    delivery_channel_id BIGINT NOT NULL,
    channel_version BIGINT NOT NULL,
    enterprise_connection_version BIGINT,
    target_ciphertext TEXT NOT NULL DEFAULT '',
    target_edek TEXT NOT NULL DEFAULT '',
    target_wrap_key_ref TEXT NOT NULL DEFAULT '',
    target_digest TEXT NOT NULL DEFAULT '',
    presentation TEXT NOT NULL,
    action_url_snapshot TEXT NOT NULL DEFAULT '',
    mention_requested BOOLEAN NOT NULL DEFAULT FALSE,
    mention_user_id BIGINT,
    mention_identity_id BIGINT,
    mention_identity_version BIGINT,
    mention_authority_digest TEXT NOT NULL DEFAULT '',
    warning_code TEXT NOT NULL DEFAULT '',
    rendered_payload_digest TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (delivery_id)
);
CREATE INDEX sys_notification_group_route_snapshot_route_idx ON sys_notification_group_route_snapshot (organization_id,route_id,created_at DESC);

CREATE TABLE sys_notification_user_preferences (
    id BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    event_family TEXT NOT NULL CHECK (event_family IN ('TASK_ACTION_REQUIRED','TASK_INTERRUPTED','TASK_COMPLETED')),
    channel_kind TEXT NOT NULL CHECK (channel_kind IN ('IN_APP','EMAIL_USER','FEISHU_DIRECT','WECOM_DIRECT')),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version BIGINT NOT NULL DEFAULT 1 CHECK (version>0),
    creator_id BIGINT NOT NULL,
    updater_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (organization_id,user_id,event_family,channel_kind)
);
CREATE INDEX sys_notification_user_preferences_user_idx ON sys_notification_user_preferences (organization_id,user_id,event_family);

INSERT INTO sys_permission (id,code,name,"resourceType",method,path,status,description,"createTime","updateTime","isDeleted") VALUES
 (1900301070,'system:notification-group-route:list','通知群列表','API','GET','/notification/group-routes',0,'查看组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301071,'system:notification-group-route:query','查看通知群','API','GET','/notification/group-routes/:routeId',0,'查看组织通知群详情',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301072,'system:notification-group-route:create','创建通知群','API','POST','/notification/group-routes',0,'创建组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301073,'system:notification-group-route:update','更新通知群','API','PUT','/notification/group-routes/:routeId',0,'更新组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301074,'system:notification-group-route:probe','测试通知群','API','POST','/notification/group-routes/:routeId/probe',0,'测试组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301075,'system:notification-group-route:enable','启用通知群','API','POST','/notification/group-routes/:routeId/enable',0,'启用组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301076,'system:notification-group-route:disable','停用通知群','API','POST','/notification/group-routes/:routeId/disable',0,'停用组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301077,'system:notification-group-route:retire','退役通知群','API','POST','/notification/group-routes/:routeId/retire',0,'退役组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false),
 (1900301078,'system:notification-group-route:delete','删除通知群','API','DELETE','/notification/group-routes/:routeId',0,'删除未使用的组织通知群',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,false)
ON CONFLICT (code) DO NOTHING;

INSERT INTO sys_role_permission (id,"roleId","permissionId","creatorId","createTime","updateTime")
SELECT 1900302000+ROW_NUMBER() OVER (ORDER BY r.id,p.id),r.id,p.id,NULL,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP
FROM sys_role r CROSS JOIN sys_permission p
WHERE r.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND r."isDeleted"=false AND p.code LIKE 'system:notification-group-route:%' AND p."isDeleted"=false
AND NOT EXISTS (SELECT 1 FROM sys_role_permission rp WHERE rp."roleId"=r.id AND rp."permissionId"=p.id);

-- +goose Down
-- B3 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
-- +goose StatementBegin
DO $b3$
BEGIN
    RAISE EXCEPTION 'B3 notification table rename is forward-only';
END
$b3$;
-- +goose StatementEnd
