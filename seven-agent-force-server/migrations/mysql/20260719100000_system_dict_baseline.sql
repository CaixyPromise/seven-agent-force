-- +goose Up
CREATE TABLE IF NOT EXISTS sys_dict_type (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  dictCode VARCHAR(64) COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典类型编码',
  dictName VARCHAR(128) COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典类型名称',
  dictDesc VARCHAR(255) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '字典类型描述',
  module VARCHAR(64) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '所属模块',
  status TINYINT NOT NULL DEFAULT 1 COMMENT '状态：1-启用，0-禁用',
  requiredLogin TINYINT(1) NOT NULL DEFAULT 0 COMMENT '读取是否要求登录',
  sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序规则',
  isSystem TINYINT NOT NULL DEFAULT 0 COMMENT '是否系统内置',
  createdBy BIGINT NOT NULL COMMENT '创建人ID',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updatedBy BIGINT NOT NULL COMMENT '更新人ID',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  isDeleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  PRIMARY KEY (id),
  UNIQUE KEY uk_dictCode (dictCode),
  KEY idx_module_status (module, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='字典类型表';

CREATE TABLE IF NOT EXISTS sys_dict_item (
  id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  dictTypeId BIGINT NOT NULL COMMENT '字典类型ID',
  itemValue VARCHAR(64) COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典值',
  itemLabel VARCHAR(128) COLLATE utf8mb4_general_ci NOT NULL COMMENT '字典显示文本',
  itemDesc VARCHAR(255) COLLATE utf8mb4_general_ci DEFAULT NULL COMMENT '字典项描述',
  sortOrder INT NOT NULL DEFAULT 0 COMMENT '排序号',
  status TINYINT NOT NULL DEFAULT 1 COMMENT '状态：1-启用，0-禁用',
  extJson JSON DEFAULT NULL COMMENT '扩展字段',
  createdBy BIGINT NOT NULL COMMENT '创建人ID',
  createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  updatedBy BIGINT NOT NULL COMMENT '更新人ID',
  updateTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
  isDeleted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否删除',
  PRIMARY KEY (id),
  UNIQUE KEY uk_type_value (dictTypeId, itemValue),
  KEY idx_type_status_sort (dictTypeId, status, sortOrder)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci COMMENT='字典项表';

INSERT INTO sys_dict_type (
  id, dictCode, dictName, dictDesc, module, status, requiredLogin, sortOrder,
  isSystem, createdBy, updatedBy, isDeleted
)
SELECT 2026042501001, 'gender', '性别', '用户性别字典', 'user', 1, 0, 0, 1, 1, 1, 0
WHERE NOT EXISTS (
  SELECT 1 FROM sys_dict_type WHERE dictCode = 'gender'
);

UPDATE sys_dict_type
SET status = 1,
    isDeleted = 0,
    updateTime = NOW()
WHERE dictCode = 'gender';

INSERT INTO sys_dict_item (
  dictTypeId, itemValue, itemLabel, itemDesc, sortOrder, status, extJson,
  createdBy, updatedBy, isDeleted
)
SELECT type.id, seed.itemValue, seed.itemLabel, seed.itemDesc, seed.sortOrder, 1,
       JSON_OBJECT('color', seed.color, 'icon', seed.icon), 1, 1, 0
FROM sys_dict_type type
JOIN (
  SELECT '0' AS itemValue, '未知' AS itemLabel, '性别-未知' AS itemDesc, 0 AS sortOrder, 'gray' AS color, 'unknown' AS icon
  UNION ALL SELECT '1', '男', '性别-男', 1, 'blue', 'male'
  UNION ALL SELECT '2', '女', '性别-女', 2, 'pink', 'female'
) seed
WHERE type.dictCode = 'gender'
  AND NOT EXISTS (
    SELECT 1
    FROM sys_dict_item item
    WHERE item.dictTypeId = type.id
      AND item.itemValue = seed.itemValue
  );

UPDATE sys_dict_item item
JOIN sys_dict_type type ON type.id = item.dictTypeId
SET item.status = 1,
    item.isDeleted = 0,
    item.updateTime = NOW()
WHERE type.dictCode = 'gender'
  AND item.itemValue IN ('0', '1', '2');

-- +goose Down
-- This repair may adopt tables created by an earlier runtime baseline. Dropping them
-- would destroy pre-existing dictionary data, so rollback is intentionally non-destructive.
SELECT 1;
