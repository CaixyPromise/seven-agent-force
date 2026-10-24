-- +goose Up
CREATE TABLE IF NOT EXISTS docker_remote_registry (
  id BIGINT NOT NULL COMMENT '注册中心ID',
  name VARCHAR(128) NOT NULL COMMENT '注册中心名称',
  code VARCHAR(64) NOT NULL COMMENT '注册中心编码',
  registryType VARCHAR(32) NOT NULL DEFAULT 'REGISTRY' COMMENT '注册中心类型',
  endpoint VARCHAR(512) NOT NULL COMMENT 'Registry endpoint',
  apiBaseUrl VARCHAR(512) DEFAULT NULL COMMENT 'Registry HTTP API base URL',
  authType VARCHAR(32) NOT NULL DEFAULT 'ANONYMOUS' COMMENT '认证类型',
  username VARCHAR(256) DEFAULT NULL COMMENT '用户名',
  tokenRealm VARCHAR(512) DEFAULT NULL COMMENT 'Bearer token realm',
  tokenService VARCHAR(256) DEFAULT NULL COMMENT 'Bearer token service',
  credentialId BIGINT DEFAULT NULL COMMENT '外部凭证ID',
  namespaceWhitelistJson TEXT DEFAULT NULL COMMENT '命名空间白名单JSON',
  tlsEnabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用TLS',
  insecureSkipVerify TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否跳过TLS校验',
  defaultRegistry TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否默认注册中心',
  status TINYINT NOT NULL DEFAULT 0 COMMENT '状态：0启用 1停用',
  description VARCHAR(512) DEFAULT NULL COMMENT '描述',
  sort INT NOT NULL DEFAULT 0 COMMENT '排序',
  secretCiphertext TEXT DEFAULT NULL COMMENT '加密后的密码密文',
  secretEdek TEXT DEFAULT NULL COMMENT '加密后的数据密钥',
  wrapKeyRef VARCHAR(255) DEFAULT NULL COMMENT '包装密钥引用',
  deleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_docker_registry_code_deleted (code, deleted),
  KEY idx_docker_registry_default (defaultRegistry, deleted),
  KEY idx_docker_registry_status (status, deleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Docker远程注册中心配置';

CREATE TABLE IF NOT EXISTS docker_operation (
  id BIGINT NOT NULL COMMENT 'Docker操作ID',
  operationType VARCHAR(64) NOT NULL COMMENT '操作类型',
  targetType VARCHAR(64) NOT NULL COMMENT '目标类型',
  targetName VARCHAR(512) DEFAULT NULL COMMENT '目标名称',
  status VARCHAR(32) NOT NULL COMMENT '状态',
  progressPercent INT NOT NULL DEFAULT 0 COMMENT '进度百分比',
  currentStage VARCHAR(128) DEFAULT NULL COMMENT '当前阶段',
  errorSummary VARCHAR(1024) DEFAULT NULL COMMENT '错误摘要',
  resultJson MEDIUMTEXT DEFAULT NULL COMMENT '结果JSON',
  requestPayloadPreview MEDIUMTEXT DEFAULT NULL COMMENT '脱敏请求摘要',
  requestPayloadCiphertext MEDIUMTEXT DEFAULT NULL COMMENT '加密请求载荷',
  requestPayloadEdek MEDIUMTEXT DEFAULT NULL COMMENT '加密数据密钥',
  requestPayloadWrapKeyRef VARCHAR(255) DEFAULT NULL COMMENT '包装密钥引用',
  actorUserId BIGINT DEFAULT NULL COMMENT '操作者用户ID',
  actorUsername VARCHAR(128) DEFAULT NULL COMMENT '操作者账号',
  retryOf BIGINT DEFAULT NULL COMMENT '重试来源操作ID',
  cancelRequested TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否请求取消',
  timeoutAt DATETIME DEFAULT NULL COMMENT '超时时间',
  startedAt DATETIME DEFAULT NULL COMMENT '开始时间',
  finishedAt DATETIME DEFAULT NULL COMMENT '结束时间',
  heartbeatAt DATETIME DEFAULT NULL COMMENT '心跳时间',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  PRIMARY KEY (id),
  KEY idx_docker_operation_status (status, updateTime),
  KEY idx_docker_operation_type (operationType, updateTime),
  KEY idx_docker_operation_actor (actorUserId, updateTime),
  KEY idx_docker_operation_retry (retryOf)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Docker异步操作';

CREATE TABLE IF NOT EXISTS docker_operation_event (
  id BIGINT NOT NULL COMMENT 'Docker操作事件ID',
  operationId BIGINT NOT NULL COMMENT 'Docker操作ID',
  sequence BIGINT NOT NULL COMMENT '事件序号',
  eventType VARCHAR(32) NOT NULL COMMENT '事件类型',
  stage VARCHAR(128) DEFAULT NULL COMMENT '阶段',
  percent INT DEFAULT NULL COMMENT '百分比',
  message VARCHAR(2048) DEFAULT NULL COMMENT '消息',
  payloadJson MEDIUMTEXT DEFAULT NULL COMMENT '脱敏载荷',
  occurredAt DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '发生时间',
  PRIMARY KEY (id),
  UNIQUE KEY uk_docker_operation_event_sequence (operationId, sequence),
  KEY idx_docker_operation_event_operation (operationId, occurredAt),
  CONSTRAINT fk_docker_operation_event_operation FOREIGN KEY (operationId) REFERENCES docker_operation(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='Docker异步操作事件';

INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100001, 'admin:docker:container:list', 'Docker容器列表', 'API', 'GET', '/admin/docker/containers', 0, '查询Docker容器列表', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:list');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100002, 'admin:docker:container:query', 'Docker容器详情', 'API', 'GET', '/admin/docker/containers/{id}', 0, '查询Docker容器详情', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:query');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100003, 'admin:docker:container:logs', 'Docker容器日志', 'API', 'GET', '/admin/docker/containers/{id}/logs', 0, '读取Docker容器日志', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:logs');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100004, 'admin:docker:container:start', '启动Docker容器', 'API', 'POST', '/admin/docker/containers/{id}/start', 0, '启动Docker容器', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:start');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100005, 'admin:docker:container:stop', '停止Docker容器', 'API', 'POST', '/admin/docker/containers/{id}/stop', 0, '停止Docker容器', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:stop');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100006, 'admin:docker:container:restart', '重启Docker容器', 'API', 'POST', '/admin/docker/containers/{id}/restart', 0, '重启Docker容器', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:restart');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100007, 'admin:docker:container:delete', '删除Docker容器', 'API', 'DELETE', '/admin/docker/containers/{id}', 0, '删除Docker容器', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:delete');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100008, 'admin:docker:container:create', '创建Docker容器', 'API', 'POST', '/admin/docker/containers/create-from-image', 0, '从镜像创建Docker容器', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:container:create');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100011, 'admin:docker:image:list', 'Docker镜像列表', 'API', 'GET', '/admin/docker/images/local', 0, '查询Docker本地镜像列表', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:list');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100012, 'admin:docker:image:query', 'Docker镜像详情', 'API', 'GET', '/admin/docker/images/local/{id}', 0, '查询Docker镜像详情', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:query');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100013, 'admin:docker:image:containers', 'Docker镜像关联容器', 'API', 'GET', '/admin/docker/images/local/{id}/containers', 0, '查询Docker镜像关联容器', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:containers');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100014, 'admin:docker:image:pull', '拉取Docker镜像', 'API', 'POST', '/admin/docker/images/pull', 0, '拉取Docker镜像', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:pull');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100015, 'admin:docker:image:tag', '标记Docker镜像', 'API', 'POST', '/admin/docker/images/tag', 0, '标记Docker镜像', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:tag');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100016, 'admin:docker:image:push', '推送Docker镜像', 'API', 'POST', '/admin/docker/images/push', 0, '推送Docker镜像', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:push');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100017, 'admin:docker:image:delete', '删除Docker镜像', 'API', 'DELETE', '/admin/docker/images/local/{id}', 0, '删除Docker镜像', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:delete');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100018, 'admin:docker:image:startup-preview', 'Docker镜像启动预览', 'API', 'POST', '/admin/docker/images/local/{id}/startup-preview', 0, '预览Docker镜像启动参数', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:image:startup-preview');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100021, 'admin:docker:registry:list', 'Docker Registry列表', 'API', 'GET', '/admin/docker/registries', 0, '查询Docker Registry列表', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:registry:list');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100022, 'admin:docker:registry:create', '创建Docker Registry', 'API', 'POST', '/admin/docker/registries', 0, '创建Docker Registry', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:registry:create');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100023, 'admin:docker:registry:update', '更新Docker Registry', 'API', 'PUT', '/admin/docker/registries/{id}', 0, '更新Docker Registry', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:registry:update');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100024, 'admin:docker:registry:test', '测试Docker Registry', 'API', 'POST', '/admin/docker/registries/{id}/test', 0, '测试Docker Registry连接', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:registry:test');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100031, 'admin:docker:compose:validate', '校验Docker Compose', 'API', 'POST', '/admin/docker/compose/validate', 0, '校验Docker Compose YAML', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:validate');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100032, 'admin:docker:compose:up', '执行Docker Compose', 'API', 'POST', '/admin/docker/compose/up', 0, '执行Docker Compose Up', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:compose:up');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100041, 'admin:docker:operation:list', 'Docker操作列表', 'API', 'GET', '/admin/docker/operations', 0, '查询Docker异步操作列表', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:operation:list');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100042, 'admin:docker:operation:query', 'Docker操作详情', 'API', 'GET', '/admin/docker/operations/{id}', 0, '查询Docker异步操作详情', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:operation:query');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100043, 'admin:docker:operation:stream', 'Docker操作事件流', 'API', 'GET', '/admin/docker/operations/{id}/stream', 0, '订阅Docker异步操作事件', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:operation:stream');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100044, 'admin:docker:operation:cancel', '取消Docker操作', 'API', 'POST', '/admin/docker/operations/{id}/cancel', 0, '取消Docker异步操作', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:operation:cancel');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100045, 'admin:docker:operation:retry', '重试Docker操作', 'API', 'POST', '/admin/docker/operations/{id}/retry', 0, '重试Docker异步操作', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:operation:retry');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100046, 'admin:docker:dangerous', 'Docker高危操作', 'API', '*', '/admin/docker/**', 0, '允许执行Docker高危操作', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:dangerous');
INSERT INTO sys_permission (id, code, name, resourceType, method, path, status, description, createTime, updateTime, isDeleted)
SELECT 1900100047, 'admin:docker:policy:override', 'Docker策略覆盖', 'API', '*', '/admin/docker/**', 0, '允许覆盖Docker locked-down策略', NOW(), NOW(), 0
WHERE NOT EXISTS (SELECT 1 FROM sys_permission WHERE code = 'admin:docker:policy:override');

INSERT IGNORE INTO sys_role_permission (id, roleId, permissionId, creatorId, createTime, updateTime)
SELECT 190020000000 + r.id + p.id, r.id, p.id, 0, NOW(), NOW()
FROM sys_role r
JOIN sys_permission p ON p.code LIKE 'admin:docker:%' AND p.isDeleted = 0
WHERE r.isDeleted = 0 AND r.code IN ('SUPER_ADMIN', 'SYSTEM_ADMIN');

-- +goose Down
DELETE FROM sys_role_permission WHERE permissionId IN (SELECT id FROM sys_permission WHERE code LIKE 'admin:docker:%');
DELETE FROM sys_permission WHERE code LIKE 'admin:docker:%';
DROP TABLE IF EXISTS docker_operation_event;
DROP TABLE IF EXISTS docker_operation;
DROP TABLE IF EXISTS docker_remote_registry;
