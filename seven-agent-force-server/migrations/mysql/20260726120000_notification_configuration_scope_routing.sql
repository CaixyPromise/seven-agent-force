-- +goose Up
-- Notification configuration is owned by the same stable installation/Hub/Node
-- scope as Outbox work. Existing NULL rows remain local-only compatibility
-- records until a local configuration update explicitly attributes them.
ALTER TABLE sysNotificationTemplate
  ADD COLUMN scopeId VARCHAR(128) DEFAULT NULL COMMENT '模板所属范围' AFTER templateCode;

ALTER TABLE sysNotificationSceneBinding
  ADD COLUMN scopeId VARCHAR(128) DEFAULT NULL COMMENT '场景绑定所属范围' AFTER sceneCode;

CREATE INDEX idx_notification_template_scope_scene_status
  ON sysNotificationTemplate (scopeId, sceneCode, channelType, locale, status, isDeleted);

CREATE INDEX idx_notification_scene_binding_scope_scene_enabled
  ON sysNotificationSceneBinding (scopeId, sceneCode, enabled, priority, isDeleted);

-- +goose Down
DROP INDEX idx_notification_scene_binding_scope_scene_enabled ON sysNotificationSceneBinding;
DROP INDEX idx_notification_template_scope_scene_status ON sysNotificationTemplate;

ALTER TABLE sysNotificationSceneBinding
  DROP COLUMN scopeId;

ALTER TABLE sysNotificationTemplate
  DROP COLUMN scopeId;
