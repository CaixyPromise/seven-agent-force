-- +goose Up
-- Goal-6.1 adds a versioned authoring workspace beside the legacy
-- sysNotificationTemplate runtime configuration. These tables are not read by
-- the existing dispatch path and therefore cannot alter current sends.

CREATE TABLE IF NOT EXISTS sysNotificationTemplateDefinition (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    templateCode VARCHAR(96) NOT NULL COMMENT '模板稳定编码',
    templateName VARCHAR(128) NOT NULL COMMENT '模板名称',
    locale VARCHAR(32) NOT NULL DEFAULT 'zh-CN' COMMENT '语言标识',
    currentDraftRevisionId BIGINT NULL COMMENT '当前可编辑草稿修订',
    currentPublishedRevisionId BIGINT NULL COMMENT '当前已发布修订',
    version INT NOT NULL DEFAULT 1 COMMENT '定义指针乐观锁版本',
    creatorId BIGINT NULL COMMENT '创建人',
    updaterId BIGINT NULL COMMENT '更新人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    UNIQUE KEY ukNotificationTemplateDefinitionScopeCode (scopeId, templateCode, isDeleted),
    KEY idxNotificationTemplateDefinitionScopeUpdate (scopeId, updateTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知模板版本化定义';

CREATE TABLE IF NOT EXISTS sysNotificationTemplateRevision (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    templateDefinitionId BIGINT NOT NULL COMMENT '模板定义主键',
    revisionNo INT NOT NULL COMMENT '定义内递增修订号',
    state VARCHAR(16) NOT NULL COMMENT 'DRAFT、PUBLISHED或SUPERSEDED',
    revisionVersion INT NOT NULL DEFAULT 1 COMMENT '草稿乐观锁版本',
    subjectTemplate VARCHAR(512) NULL COMMENT '标题模板',
    textTemplate TEXT NULL COMMENT '文本正文模板',
    htmlTemplate TEXT NULL COMMENT 'HTML正文模板',
    markdownTemplate TEXT NULL COMMENT 'Markdown正文模板',
    primaryActionLabelTemplate VARCHAR(128) NOT NULL DEFAULT '' COMMENT '平台主操作文案模板',
    variableSchemaJson JSON NOT NULL COMMENT '结构化变量定义内部表示',
    contentDigest VARCHAR(128) NOT NULL COMMENT '已校验内容摘要',
    publishedAt DATETIME NULL COMMENT '发布时间',
    publishedBy BIGINT NULL COMMENT '发布人',
    creatorId BIGINT NULL COMMENT '创建人',
    updaterId BIGINT NULL COMMENT '更新人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationTemplateRevisionDefinitionNo (templateDefinitionId, revisionNo),
    KEY idxNotificationTemplateRevisionDefinitionState (templateDefinitionId, state, revisionNo)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知模板不可变发布修订';

-- This audit table stores metadata only. It intentionally has no template
-- body, variable value, preview, recipient, credential, route or provider data.
CREATE TABLE IF NOT EXISTS sysNotificationTemplateRevisionAudit (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    templateDefinitionId BIGINT NOT NULL COMMENT '模板定义主键',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    action VARCHAR(48) NOT NULL COMMENT '草稿、复制或发布动作',
    fromRevisionNo INT NULL COMMENT '来源修订号',
    toRevisionNo INT NULL COMMENT '目标修订号',
    actorId BIGINT NULL COMMENT '操作者',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    KEY idxNotificationTemplateRevisionAuditDefinition (templateDefinitionId, createTime),
    KEY idxNotificationTemplateRevisionAuditScope (scopeId, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知模板修订元数据审计';

-- +goose Down
-- Forward-only: published-template history and its audit evidence must not be
-- removed automatically.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification template revision migration cannot be rolled back automatically';
