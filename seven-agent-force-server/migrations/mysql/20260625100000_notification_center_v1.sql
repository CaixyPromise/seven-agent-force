-- +goose Up
CREATE TABLE IF NOT EXISTS sysNotificationChannel (
    id BIGINT NOT NULL PRIMARY KEY,
    channelCode VARCHAR(64) NOT NULL COMMENT '渠道编码',
    channelName VARCHAR(128) NOT NULL COMMENT '渠道名称',
    channelType VARCHAR(32) NOT NULL COMMENT '渠道类型',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0启用 1停用',
    priority INT NOT NULL DEFAULT 100 COMMENT '优先级',
    configJson JSON NULL COMMENT '非敏感配置',
    secretCiphertext TEXT NULL COMMENT '敏感配置密文',
    secretEdek TEXT NULL COMMENT '敏感配置EDEK',
    secretWrapKeyRef VARCHAR(128) NULL COMMENT '敏感配置包装密钥引用',
    rateLimitJson JSON NULL COMMENT '限流配置',
    metadataJson JSON NULL COMMENT '扩展元数据',
    creatorId BIGINT NULL COMMENT '创建人',
    updaterId BIGINT NULL COMMENT '更新人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '是否删除',
    UNIQUE KEY ukNotificationChannelCode (channelCode, isDeleted),
    KEY idxNotificationChannelTypeStatus (channelType, status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知渠道';

CREATE TABLE IF NOT EXISTS sysNotificationTemplate (
    id BIGINT NOT NULL PRIMARY KEY,
    templateCode VARCHAR(96) NOT NULL COMMENT '模板编码',
    templateName VARCHAR(128) NOT NULL COMMENT '模板名称',
    sceneCode VARCHAR(64) NOT NULL COMMENT '场景编码',
    channelType VARCHAR(32) NOT NULL COMMENT '渠道类型',
    locale VARCHAR(32) NOT NULL DEFAULT 'zh-CN' COMMENT '语言区域',
    subjectTemplate VARCHAR(512) NULL COMMENT '标题模板',
    textTemplate TEXT NULL COMMENT '文本模板',
    htmlTemplate TEXT NULL COMMENT 'HTML模板',
    markdownTemplate TEXT NULL COMMENT 'Markdown模板',
    jsonTemplate JSON NULL COMMENT 'JSON模板',
    variablesJson JSON NULL COMMENT '变量定义',
    status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0启用 1停用',
    version INT NOT NULL DEFAULT 1 COMMENT '版本',
    creatorId BIGINT NULL COMMENT '创建人',
    updaterId BIGINT NULL COMMENT '更新人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '是否删除',
    UNIQUE KEY ukNotificationTemplateCode (templateCode, isDeleted),
    KEY idxNotificationTemplateScene (sceneCode, channelType, locale, status, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知模板';

CREATE TABLE IF NOT EXISTS sysNotificationSceneBinding (
    id BIGINT NOT NULL PRIMARY KEY,
    sceneCode VARCHAR(64) NOT NULL COMMENT '场景编码',
    sceneName VARCHAR(128) NOT NULL COMMENT '场景名称',
    channelCode VARCHAR(64) NOT NULL COMMENT '渠道编码',
    templateCode VARCHAR(96) NOT NULL COMMENT '模板编码',
    enabled TINYINT NOT NULL DEFAULT 1 COMMENT '是否启用',
    priority INT NOT NULL DEFAULT 100 COMMENT '优先级',
    maxRetry INT NOT NULL DEFAULT 3 COMMENT '最大重试次数',
    retryIntervalSeconds INT NOT NULL DEFAULT 60 COMMENT '重试间隔秒',
    metadataJson JSON NULL COMMENT '扩展元数据',
    creatorId BIGINT NULL COMMENT '创建人',
    updaterId BIGINT NULL COMMENT '更新人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '是否删除',
    KEY idxNotificationSceneBindingScene (sceneCode, enabled, priority, isDeleted),
    KEY idxNotificationSceneBindingChannel (channelCode, isDeleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知场景绑定';

CREATE TABLE IF NOT EXISTS sysNotificationDelivery (
    id BIGINT NOT NULL PRIMARY KEY,
    deliveryId VARCHAR(96) NOT NULL COMMENT '投递ID',
    requestDigest VARCHAR(128) NOT NULL COMMENT '请求摘要',
    sceneCode VARCHAR(64) NOT NULL COMMENT '场景编码',
    channelCode VARCHAR(64) NOT NULL COMMENT '渠道编码',
    channelType VARCHAR(32) NOT NULL COMMENT '渠道类型',
    templateCode VARCHAR(96) NOT NULL COMMENT '模板编码',
    target VARCHAR(512) NULL COMMENT '目标地址',
    targetMasked VARCHAR(512) NULL COMMENT '脱敏目标地址',
    payloadJson JSON NULL COMMENT '变量负载',
    renderedSubject VARCHAR(512) NULL COMMENT '渲染标题',
    renderedText TEXT NULL COMMENT '渲染文本',
    renderedHtml TEXT NULL COMMENT '渲染HTML',
    renderedMarkdown TEXT NULL COMMENT '渲染Markdown',
    status VARCHAR(24) NOT NULL DEFAULT 'PENDING' COMMENT '状态',
    retryCount INT NOT NULL DEFAULT 0 COMMENT '重试次数',
    maxRetry INT NOT NULL DEFAULT 3 COMMENT '最大重试次数',
    nextRetryAt DATETIME NULL COMMENT '下次重试时间',
    lastError TEXT NULL COMMENT '最近错误',
    traceId VARCHAR(64) NULL COMMENT '链路ID',
    sentAt DATETIME NULL COMMENT '发送时间',
    creatorId BIGINT NULL COMMENT '创建人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '是否删除',
    UNIQUE KEY ukNotificationDeliveryId (deliveryId),
    UNIQUE KEY ukNotificationDeliveryDigest (requestDigest, isDeleted),
    KEY idxNotificationDeliveryStatus (status, nextRetryAt, isDeleted),
    KEY idxNotificationDeliveryScene (sceneCode, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知投递';

SET @operatorId := 0;
SET @baseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000000000) FROM sysNotificationChannel);

INSERT INTO sysNotificationChannel (id, channelCode, channelName, channelType, status, priority, configJson, metadataJson, creatorId, updaterId)
SELECT @baseId + 1, 'mock-default', '默认 Mock 通知渠道', 'MOCK', 0, 10,
       JSON_OBJECT('capturePrefix', 'notification:mock:capture'),
       JSON_OBJECT('seed', 'notification-center-v1'), @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysNotificationChannel WHERE channelCode = 'mock-default' AND isDeleted = 0);

SET @templateBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000000100) FROM sysNotificationTemplate);

INSERT INTO sysNotificationTemplate (
    id, templateCode, templateName, sceneCode, channelType, locale,
    subjectTemplate, textTemplate, htmlTemplate, variablesJson, creatorId, updaterId
)
SELECT @templateBaseId + 1, 'challenge_otp_mock_zh_cn', '安全验证码 Mock 模板', 'CHALLENGE_OTP', 'MOCK', 'zh-CN',
       '【{{.AppName}}】-{{.SceneName}}',
       '您的验证码是 {{.Code}}，{{.TTLMinutes}} 分钟内有效。',
       '<p>您的验证码是 <strong>{{.Code}}</strong>，{{.TTLMinutes}} 分钟内有效。</p>',
       JSON_ARRAY('AppName', 'SceneName', 'Code', 'TTLMinutes', 'ToEmail'),
       @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysNotificationTemplate WHERE templateCode = 'challenge_otp_mock_zh_cn' AND isDeleted = 0);

INSERT INTO sysNotificationTemplate (
    id, templateCode, templateName, sceneCode, channelType, locale,
    subjectTemplate, textTemplate, htmlTemplate, variablesJson, creatorId, updaterId
)
SELECT @templateBaseId + 2, 'challenge_otp_email_zh_cn', '安全验证码 Email 模板', 'CHALLENGE_OTP', 'EMAIL', 'zh-CN',
       '【{{.AppName}}】-{{.SceneName}}',
       '您的验证码是 {{.Code}}，{{.TTLMinutes}} 分钟内有效。',
       '<p>您的验证码是 <strong>{{.Code}}</strong>，{{.TTLMinutes}} 分钟内有效。</p>',
       JSON_ARRAY('AppName', 'SceneName', 'Code', 'TTLMinutes', 'ToEmail'),
       @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysNotificationTemplate WHERE templateCode = 'challenge_otp_email_zh_cn' AND isDeleted = 0);

SET @bindingBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000000200) FROM sysNotificationSceneBinding);

INSERT INTO sysNotificationSceneBinding (id, sceneCode, sceneName, channelCode, templateCode, enabled, priority, maxRetry, retryIntervalSeconds, creatorId, updaterId)
SELECT @bindingBaseId + 1, 'CHALLENGE_OTP', '安全验证码', 'mock-default', 'challenge_otp_mock_zh_cn', 1, 10, 3, 60, @operatorId, @operatorId
WHERE NOT EXISTS (SELECT 1 FROM sysNotificationSceneBinding WHERE sceneCode = 'CHALLENGE_OTP' AND channelCode = 'mock-default' AND isDeleted = 0);

SET @permissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000001000) FROM sys_permission);

UPDATE sys_permission existing
JOIN (
  SELECT 1 AS sortOrder, 'system:notification:channel:list' AS code, '通知渠道列表' AS name, 'GET' AS method, '/notification/channels' AS path, '查询通知渠道' AS description
  UNION ALL SELECT 2, 'system:notification:channel:edit', '编辑通知渠道', 'POST', '/notification/channels', '新增或编辑通知渠道'
  UNION ALL SELECT 3, 'system:notification:template:list', '通知模板列表', 'GET', '/notification/templates', '查询通知模板'
  UNION ALL SELECT 4, 'system:notification:template:edit', '编辑通知模板', 'POST', '/notification/templates', '新增或编辑通知模板'
  UNION ALL SELECT 5, 'system:notification:scene:list', '通知场景列表', 'GET', '/notification/scene-bindings', '查询通知场景'
  UNION ALL SELECT 6, 'system:notification:scene:edit', '编辑通知场景', 'POST', '/notification/scene-bindings', '新增或编辑通知场景'
  UNION ALL SELECT 7, 'system:notification:delivery:list', '通知投递日志', 'GET', '/notification/deliveries', '查询通知投递日志'
  UNION ALL SELECT 8, 'system:notification:test', '测试通知发送', 'POST', '/notification/test-send', '测试通知发送'
) item ON existing.code = item.code
SET existing.name = item.name,
    existing.resourceType = 'API',
    existing.method = item.method,
    existing.path = item.path,
    existing.status = 0,
    existing.description = item.description,
    existing.updateTime = NOW(),
    existing.isDeleted = 0;

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT @permissionBaseId + ROW_NUMBER() OVER (ORDER BY item.sortOrder), item.code, item.name, 'API', item.method, item.path, 0, item.description, NOW(), NOW(), 0
FROM (
  SELECT 1 AS sortOrder, 'system:notification:channel:list' AS code, '通知渠道列表' AS name, 'GET' AS method, '/notification/channels' AS path, '查询通知渠道' AS description
  UNION ALL SELECT 2, 'system:notification:channel:edit', '编辑通知渠道', 'POST', '/notification/channels', '新增或编辑通知渠道'
  UNION ALL SELECT 3, 'system:notification:template:list', '通知模板列表', 'GET', '/notification/templates', '查询通知模板'
  UNION ALL SELECT 4, 'system:notification:template:edit', '编辑通知模板', 'POST', '/notification/templates', '新增或编辑通知模板'
  UNION ALL SELECT 5, 'system:notification:scene:list', '通知场景列表', 'GET', '/notification/scene-bindings', '查询通知场景'
  UNION ALL SELECT 6, 'system:notification:scene:edit', '编辑通知场景', 'POST', '/notification/scene-bindings', '新增或编辑通知场景'
  UNION ALL SELECT 7, 'system:notification:delivery:list', '通知投递日志', 'GET', '/notification/deliveries', '查询通知投递日志'
  UNION ALL SELECT 8, 'system:notification:test', '测试通知发送', 'POST', '/notification/test-send', '测试通知发送'
) item
WHERE NOT EXISTS (SELECT 1 FROM sys_permission existing WHERE existing.code = item.code);

SET @rootMenuId := (SELECT id FROM sys_menu WHERE path = '/system' AND isDeleted = 0 ORDER BY visible DESC, sortOrder ASC, id LIMIT 1);
SET @accessMenuId := (SELECT id FROM sys_menu WHERE path = '/system/access' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @menuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000002000) FROM sys_menu);

INSERT INTO sys_menu (id, name, parentId, sortOrder, path, component, icon, type, permission, isFrame, isCache, visible, hierarchy, level, status, creatorId, createTime, updaterId, updateTime, isDeleted, remark)
SELECT @menuBaseId + 1, '通知中心', @accessMenuId, 70, '/system/notification', 'system/notification/index', 'NotificationOutlined', 'C', 'system:notification:channel:list', 1, 1, 1, CONCAT('/1/', @rootMenuId, '/', @accessMenuId, '/', @menuBaseId + 1), 3, 0, @operatorId, NOW(), @operatorId, NOW(), 0, '通知渠道、模板、场景与投递日志'
WHERE @accessMenuId IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM sys_menu existing WHERE existing.path = '/system/notification' AND existing.isDeleted = 0);

SET @notificationMenuId := (SELECT id FROM sys_menu WHERE path = '/system/notification' AND isDeleted = 0 ORDER BY id LIMIT 1);
SET @menuPermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000003000) FROM sys_menu_permission);

INSERT INTO sys_menu_permission (id, menuId, permissionId, creatorId, createTime)
SELECT @menuPermissionBaseId + ROW_NUMBER() OVER (ORDER BY p.id), @notificationMenuId, p.id, @operatorId, NOW()
FROM sys_permission p
WHERE p.code LIKE 'system:notification:%'
AND @notificationMenuId IS NOT NULL
AND NOT EXISTS (
  SELECT 1 FROM sys_menu_permission existing WHERE existing.menuId = @notificationMenuId AND existing.permissionId = p.id
);

SET @roleMenuBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000004000) FROM sys_role_menu);

INSERT INTO sys_role_menu (id, roleId, menuId, createTime, updateTime)
SELECT @roleMenuBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.menuId), candidate.roleId, candidate.menuId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, m.id AS menuId
  FROM sys_role r
  JOIN sys_menu m ON m.path IN ('/system/access', '/system/notification') AND m.isDeleted = 0
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_menu existing WHERE existing.roleId = r.id AND existing.menuId = m.id)
) candidate;

SET @rolePermissionBaseId := (SELECT GREATEST(COALESCE(MAX(id), 0), 2026062510000005000) FROM sys_role_permission);

INSERT INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT @rolePermissionBaseId + ROW_NUMBER() OVER (ORDER BY candidate.roleId, candidate.permissionId), candidate.roleId, candidate.permissionId, @operatorId, NOW(), NOW()
FROM (
  SELECT r.id AS roleId, p.id AS permissionId
  FROM sys_role r
  JOIN sys_permission p ON p.isDeleted = 0 AND p.code LIKE 'system:notification:%'
  WHERE r.code = 'SUPER_ADMIN' AND r.isDeleted = 0
    AND NOT EXISTS (SELECT 1 FROM sys_role_permission existing WHERE existing.roleId = r.id AND existing.permissionId = p.id)
) candidate;

-- +goose Down
-- Notification center seed and tables are retained on rollback to avoid losing
-- operator-maintained channel secrets, templates, and delivery audit records.
SELECT 1;
