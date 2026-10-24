-- +goose Up
-- Goal-6.2 adds versioned notification scenes beside the V1 scene-binding
-- table. A row fixes exactly one receiver kind, one published template and
-- one sending connection. Dynamic people, groups, URLs and credentials are
-- intentionally absent from every scene table.

CREATE TABLE IF NOT EXISTS sysNotificationSceneDefinition (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    sceneCode VARCHAR(96) NOT NULL COMMENT '业务场景稳定编码',
    sceneName VARCHAR(128) NOT NULL COMMENT '场景显示名称',
    receiverKind VARCHAR(32) NOT NULL COMMENT '接收对象类别',
    currentDraftRevisionId BIGINT NULL COMMENT '当前可编辑草稿修订',
    currentPublishedRevisionId BIGINT NULL COMMENT '当前已发布修订',
    version INT NOT NULL DEFAULT 1 COMMENT '定义指针乐观锁版本',
    creatorId BIGINT NULL COMMENT '创建人',
    updaterId BIGINT NULL COMMENT '更新人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    isDeleted TINYINT NOT NULL DEFAULT 0 COMMENT '逻辑删除标记',
    UNIQUE KEY ukNotificationSceneDefinitionScopeCodeKind (scopeId, sceneCode, receiverKind, isDeleted),
    KEY idxNotificationSceneDefinitionScopeUpdate (scopeId, updateTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知场景版本化定义';

CREATE TABLE IF NOT EXISTS sysNotificationSceneRevision (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    sceneDefinitionId BIGINT NOT NULL COMMENT '场景定义主键',
    revisionNo INT NOT NULL COMMENT '定义内递增修订号',
    state VARCHAR(16) NOT NULL COMMENT 'DRAFT、PUBLISHED或SUPERSEDED',
    revisionVersion INT NOT NULL DEFAULT 1 COMMENT '草稿乐观锁版本',
    enabled TINYINT NOT NULL DEFAULT 1 COMMENT '已发布版本是否启用',
    templateRevisionId BIGINT NOT NULL COMMENT '已发布模板修订主键',
    connectionRef VARCHAR(64) NULL COMMENT '唯一发送连接引用，站内信为空',
    connectionDigest VARCHAR(128) NULL COMMENT '连接身份摘要，不含密钥',
    publishedAt DATETIME NULL COMMENT '发布时间',
    publishedBy BIGINT NULL COMMENT '发布人',
    creatorId BIGINT NULL COMMENT '创建人',
    updaterId BIGINT NULL COMMENT '更新人',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationSceneRevisionDefinitionNo (sceneDefinitionId, revisionNo),
    KEY idxNotificationSceneRevisionDefinitionState (sceneDefinitionId, state, revisionNo),
    KEY idxNotificationSceneRevisionTemplate (templateRevisionId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知场景不可变发布修订';

CREATE TABLE IF NOT EXISTS sysNotificationSceneRevisionAudit (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    sceneDefinitionId BIGINT NOT NULL COMMENT '场景定义主键',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    action VARCHAR(48) NOT NULL COMMENT '草稿、复制、发布或停用动作',
    fromRevisionNo INT NULL COMMENT '来源修订号',
    toRevisionNo INT NULL COMMENT '目标修订号',
    errorCode VARCHAR(64) NULL COMMENT '安全错误码，不含原始参数',
    actorId BIGINT NULL COMMENT '操作者',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    KEY idxNotificationSceneRevisionAuditDefinition (sceneDefinitionId, createTime),
    KEY idxNotificationSceneRevisionAuditScope (scopeId, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知场景修订元数据审计';

CREATE TABLE IF NOT EXISTS sysNotificationSceneSnapshot (
    id BIGINT NOT NULL PRIMARY KEY COMMENT '内部主键',
    notificationId BIGINT NOT NULL COMMENT '接受时逻辑通知主键',
    scopeId VARCHAR(128) NOT NULL COMMENT '部署、Hub或Node作用域',
    sceneCode VARCHAR(96) NOT NULL COMMENT '业务场景编码',
    receiverKind VARCHAR(32) NOT NULL COMMENT '接收对象类别',
    sceneDefinitionId BIGINT NOT NULL COMMENT '场景定义主键',
    sceneRevisionId BIGINT NOT NULL COMMENT '场景已发布修订主键',
    templateDefinitionId BIGINT NOT NULL COMMENT '模板定义主键',
    templateRevisionId BIGINT NOT NULL COMMENT '模板已发布修订主键',
    connectionRef VARCHAR(64) NULL COMMENT '保存连接引用，不含URL或密钥',
    connectionDigest VARCHAR(128) NULL COMMENT '连接身份摘要',
    templateContentDigest VARCHAR(128) NOT NULL COMMENT '模板内容摘要',
    renderedDigest VARCHAR(128) NOT NULL COMMENT '渲染结果摘要',
    variableDigest VARCHAR(128) NOT NULL COMMENT '变量摘要',
    resolution VARCHAR(32) NOT NULL COMMENT 'ACCEPTED或SCENE_DISABLED',
    createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY ukNotificationSceneSnapshotNotificationKind (notificationId, receiverKind),
    KEY idxNotificationSceneSnapshotSceneRevision (sceneRevisionId),
    KEY idxNotificationSceneSnapshotScope (scopeId, createTime)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='通知场景接受时快照';

-- MySQL releases differ on support for ADD COLUMN IF NOT EXISTS. Keep the
-- forward-only migration replayable after goose down/up without relying on
-- that newer syntax or deleting accepted snapshots.
DROP PROCEDURE IF EXISTS ensureNotificationSceneDeliverySnapshotColumn;
-- +goose StatementBegin
CREATE PROCEDURE ensureNotificationSceneDeliverySnapshotColumn()
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = 'sysNotificationDelivery'
          AND column_name = 'sceneSnapshotId'
    ) THEN
        ALTER TABLE sysNotificationDelivery
            ADD COLUMN sceneSnapshotId BIGINT NULL COMMENT '场景接受时快照主键' AFTER externalTargetId;
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = 'sysNotificationDelivery'
          AND index_name = 'idxNotificationDeliverySceneSnapshot'
    ) THEN
        ALTER TABLE sysNotificationDelivery
            ADD KEY idxNotificationDeliverySceneSnapshot (sceneSnapshotId);
    END IF;
END
-- +goose StatementEnd
CALL ensureNotificationSceneDeliverySnapshotColumn();
DROP PROCEDURE ensureNotificationSceneDeliverySnapshotColumn;

-- +goose Down
-- Forward-only: accepted scene snapshots and immutable revision history must
-- remain available for delivery audit and idempotency evidence.
SIGNAL SQLSTATE '45000'
    SET MESSAGE_TEXT = 'forward-only notification scene revision migration cannot be rolled back automatically';
