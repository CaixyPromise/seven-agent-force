-- +goose Up
-- B3 renames notification physical tables in place. Apply it before starting
-- application code that uses the lower snake_case names; no legacy-name
-- process may remain live afterward.
RENAME TABLE
    sysNotification TO sys_notification,
    sysNotificationChannel TO sys_notification_channel,
    sysNotificationDelivery TO sys_notification_delivery,
    sysNotificationDeliveryAttempt TO sys_notification_delivery_attempt,
    sysNotificationDeliveryDiagnosticAudit TO sys_notification_delivery_diagnostic_audit,
    sysNotificationDeliveryEphemeralContent TO sys_notification_delivery_ephemeral_content,
    sysNotificationExternalTarget TO sys_notification_external_target,
    sysNotificationHTTPDeliverySnapshot TO sys_notification_http_delivery_snapshot,
    sysNotificationMailbox TO sys_notification_mailbox,
    sysNotificationMaterializationTask TO sys_notification_materialization_task,
    sysNotificationRecipient TO sys_notification_recipient,
    sysNotificationSceneBinding TO sys_notification_scene_binding,
    sysNotificationSceneDefinition TO sys_notification_scene_definition,
    sysNotificationSceneRevision TO sys_notification_scene_revision,
    sysNotificationSceneRevisionAudit TO sys_notification_scene_revision_audit,
    sysNotificationSceneSnapshot TO sys_notification_scene_snapshot,
    sysNotificationTemplate TO sys_notification_template,
    sysNotificationTemplateDefinition TO sys_notification_template_definition,
    sysNotificationTemplateRevision TO sys_notification_template_revision,
    sysNotificationTemplateRevisionAudit TO sys_notification_template_revision_audit;

CREATE TABLE sys_notification_group_route (
    id BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    provider_type VARCHAR(32) NOT NULL,
    transport_type VARCHAR(32) NOT NULL,
    delivery_channel_id BIGINT NOT NULL,
    required_permission_code VARCHAR(191) NOT NULL DEFAULT 'workforce:task:create',
    target_ciphertext LONGTEXT NOT NULL,
    target_edek LONGTEXT NOT NULL,
    target_wrap_key_ref VARCHAR(191) NOT NULL,
    target_digest VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'DRAFT',
    route_version BIGINT NOT NULL DEFAULT 1,
    channel_version BIGINT NOT NULL DEFAULT 1,
    connection_version BIGINT NULL,
    config_digest VARCHAR(128) NOT NULL,
    capabilities_json JSON NOT NULL,
    last_probe_status VARCHAR(32) NOT NULL DEFAULT 'NOT_PROBED',
    last_probe_reason_code VARCHAR(128) NOT NULL DEFAULT '',
    last_probe_at DATETIME(3) NULL,
    creator_id BIGINT NOT NULL,
    updater_id BIGINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY sys_notification_group_route_name_uidx (organization_id,display_name),
    KEY sys_notification_group_route_available_idx (organization_id,status,provider_type,display_name)
);

CREATE TABLE sys_notification_group_route_candidate (
    id BIGINT PRIMARY KEY,
    token_hash VARCHAR(128) NOT NULL,
    organization_id BIGINT NOT NULL,
    connection_id BIGINT NOT NULL,
    connection_version BIGINT NOT NULL,
    connection_config_digest VARCHAR(128) NOT NULL,
    display_name VARCHAR(255) NOT NULL,
    description VARCHAR(500) NOT NULL,
    target_ciphertext LONGTEXT NOT NULL,
    target_edek LONGTEXT NOT NULL,
    target_wrap_key_ref VARCHAR(191) NOT NULL,
    target_digest VARCHAR(128) NOT NULL,
    expires_at DATETIME(3) NOT NULL,
    consumed_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY sys_notification_group_route_candidate_token_uidx (token_hash),
    KEY sys_notification_group_route_candidate_expiry_idx (organization_id,expires_at)
);

CREATE TABLE sys_notification_group_route_snapshot (
    id BIGINT PRIMARY KEY,
    notification_id VARCHAR(191) NOT NULL,
    delivery_id VARCHAR(191) NOT NULL,
    organization_id BIGINT NOT NULL,
    route_id BIGINT NOT NULL,
    route_version BIGINT NOT NULL,
    provider_type VARCHAR(32) NOT NULL,
    transport_type VARCHAR(32) NOT NULL,
    route_config_digest VARCHAR(128) NOT NULL,
    delivery_channel_id BIGINT NOT NULL,
    channel_version BIGINT NOT NULL,
    enterprise_connection_version BIGINT NULL,
    target_ciphertext LONGTEXT NOT NULL,
    target_edek LONGTEXT NOT NULL,
    target_wrap_key_ref VARCHAR(191) NOT NULL,
    target_digest VARCHAR(128) NOT NULL,
    presentation VARCHAR(32) NOT NULL,
    action_url_snapshot TEXT NOT NULL,
    mention_requested BOOLEAN NOT NULL DEFAULT FALSE,
    mention_user_id BIGINT NULL,
    mention_identity_id BIGINT NULL,
    mention_identity_version BIGINT NULL,
    mention_authority_digest VARCHAR(128) NOT NULL,
    warning_code VARCHAR(128) NOT NULL,
    rendered_payload_digest VARCHAR(128) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY sys_notification_group_route_snapshot_delivery_uidx (delivery_id),
    KEY sys_notification_group_route_snapshot_route_idx (organization_id,route_id,created_at)
);

CREATE TABLE sys_notification_user_preferences (
    id BIGINT PRIMARY KEY,
    organization_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    event_family VARCHAR(64) NOT NULL,
    channel_kind VARCHAR(32) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version BIGINT NOT NULL DEFAULT 1,
    creator_id BIGINT NOT NULL,
    updater_id BIGINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    UNIQUE KEY sys_notification_user_preferences_uidx (organization_id,user_id,event_family,channel_kind),
    KEY sys_notification_user_preferences_user_idx (organization_id,user_id,event_family)
);

INSERT IGNORE INTO sys_permission (id,code,name,resourceType,method,path,status,description,createTime,updateTime,isDeleted) VALUES
 (1900301070,'system:notification-group-route:list','通知群列表','API','GET','/notification/group-routes',0,'查看组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301071,'system:notification-group-route:query','查看通知群','API','GET','/notification/group-routes/:routeId',0,'查看组织通知群详情',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301072,'system:notification-group-route:create','创建通知群','API','POST','/notification/group-routes',0,'创建组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301073,'system:notification-group-route:update','更新通知群','API','PUT','/notification/group-routes/:routeId',0,'更新组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301074,'system:notification-group-route:probe','测试通知群','API','POST','/notification/group-routes/:routeId/probe',0,'测试组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301075,'system:notification-group-route:enable','启用通知群','API','POST','/notification/group-routes/:routeId/enable',0,'启用组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301076,'system:notification-group-route:disable','停用通知群','API','POST','/notification/group-routes/:routeId/disable',0,'停用组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301077,'system:notification-group-route:retire','退役通知群','API','POST','/notification/group-routes/:routeId/retire',0,'退役组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE),
 (1900301078,'system:notification-group-route:delete','删除通知群','API','DELETE','/notification/group-routes/:routeId',0,'删除未使用的组织通知群',CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3),FALSE);

INSERT IGNORE INTO sys_role_permission (id,roleId,permissionId,creatorId,createTime,updateTime)
SELECT 1900302000+ROW_NUMBER() OVER (ORDER BY r.id,p.id),r.id,p.id,NULL,CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3)
FROM sys_role r CROSS JOIN sys_permission p
WHERE r.code IN ('SUPER_ADMIN','SYSTEM_ADMIN') AND r.isDeleted=FALSE AND p.code LIKE 'system:notification-group-route:%' AND p.isDeleted=FALSE;

-- +goose Down
-- B3 is an in-place, forward-only rename. Repair a failed rollout against the
-- current schema and deploy forward; do not restart an older binary.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'B3 notification table rename is forward-only';
