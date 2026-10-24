-- +goose Up
ALTER TABLE sys_config
  ADD COLUMN uiWidget VARCHAR(32) NOT NULL DEFAULT 'INPUT' COMMENT '受控表单控件',
  ADD COLUMN validationJson TEXT DEFAULT NULL COMMENT '标量校验规则JSON',
  ADD COLUMN exposure VARCHAR(20) NOT NULL DEFAULT 'INTERNAL' COMMENT '读取暴露级别',
  ADD COLUMN sensitivity VARCHAR(20) NOT NULL DEFAULT 'NORMAL' COMMENT '敏感级别',
  ADD COLUMN schemaVersion INT NOT NULL DEFAULT 1 COMMENT '标量契约版本',
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1 COMMENT '配置版本';

UPDATE sys_config
SET valueType = CASE UPPER(valueType)
  WHEN 'INT' THEN 'INTEGER'
  WHEN 'BOOL' THEN 'BOOLEAN'
  WHEN 'ARRAY' THEN 'MULTI_ENUM'
  ELSE UPPER(valueType)
END,
uiWidget = CASE UPPER(valueType)
  WHEN 'INT' THEN 'INPUT_NUMBER'
  WHEN 'INTEGER' THEN 'INPUT_NUMBER'
  WHEN 'BOOL' THEN 'SWITCH'
  WHEN 'BOOLEAN' THEN 'SWITCH'
  WHEN 'ENUM' THEN 'SELECT'
  WHEN 'ARRAY' THEN 'MULTI_SELECT'
  WHEN 'MULTI_ENUM' THEN 'MULTI_SELECT'
  WHEN 'JSON' THEN 'CONTROLLED_JSON'
  ELSE 'INPUT'
END,
exposure = 'INTERNAL',
sensitivity = CASE WHEN isSensitive = 1 THEN 'SENSITIVE' ELSE 'NORMAL' END,
schemaVersion = 1,
version = 1;

-- This scalar key already has a reviewed authenticated runtime consumer.
UPDATE sys_config c
JOIN sys_config_group g ON g.id = c.groupId
SET c.exposure = 'AUTHENTICATED'
WHERE g.groupCode = 'SEVEN_FRONTEND_METADATA' AND c.configKey = 'title';

ALTER TABLE sys_config_change_log
  MODIFY COLUMN id BIGINT NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  ADD COLUMN oldValueProtected TINYINT(1) NOT NULL DEFAULT 0 COMMENT '旧值是否受保护',
  ADD COLUMN newValueProtected TINYINT(1) NOT NULL DEFAULT 0 COMMENT '新值是否受保护';

UPDATE sys_config_change_log l
JOIN sys_config c ON c.id = l.configId
SET l.oldValue = CASE WHEN c.isSensitive = 1 THEN '[PROTECTED]' ELSE l.oldValue END,
    l.newValue = CASE WHEN c.isSensitive = 1 THEN '[PROTECTED]' ELSE l.newValue END,
    l.oldValueProtected = CASE WHEN c.isSensitive = 1 THEN 1 ELSE 0 END,
    l.newValueProtected = CASE WHEN c.isSensitive = 1 THEN 1 ELSE 0 END;

UPDATE sys_config_change_log l
JOIN sys_config c ON c.id = l.configId
SET l.status = 'rolled_back',
    l.operationReason = 'scalar migration: sensitive pending value retired'
WHERE c.isSensitive = 1 AND l.status = 'pending';

ALTER TABLE sys_dict_type
  ADD COLUMN valueType VARCHAR(20) NOT NULL DEFAULT 'STRING' COMMENT '字典值标量类型',
  ADD COLUMN uiWidget VARCHAR(32) NOT NULL DEFAULT 'SELECT' COMMENT '受控表单控件',
  ADD COLUMN validationJson TEXT DEFAULT NULL COMMENT '标量校验规则JSON',
  ADD COLUMN exposure VARCHAR(20) NOT NULL DEFAULT 'INTERNAL' COMMENT '读取暴露级别',
  ADD COLUMN sensitivity VARCHAR(20) NOT NULL DEFAULT 'NORMAL' COMMENT '敏感级别',
  ADD COLUMN schemaVersion INT NOT NULL DEFAULT 1 COMMENT '标量契约版本',
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1 COMMENT '字典版本';

ALTER TABLE sys_dict_item
  ADD COLUMN colorToken VARCHAR(32) DEFAULT NULL COMMENT '受控颜色令牌',
  ADD COLUMN iconToken VARCHAR(64) DEFAULT NULL COMMENT '受控图标令牌',
  ADD COLUMN presentationVersion INT NOT NULL DEFAULT 1 COMMENT '展示契约版本',
  ADD COLUMN version BIGINT NOT NULL DEFAULT 1 COMMENT '字典项版本';

UPDATE sys_dict_type SET exposure = 'INTERNAL', sensitivity = 'NORMAL', schemaVersion = 1, version = 1;
UPDATE sys_dict_type SET exposure = 'PUBLIC' WHERE dictCode = 'gender';
UPDATE sys_dict_item
SET colorToken = CASE
      WHEN JSON_UNQUOTE(JSON_EXTRACT(extJson, '$.color')) IN ('gray','blue','pink','green','orange','red','purple')
        THEN JSON_UNQUOTE(JSON_EXTRACT(extJson, '$.color'))
      ELSE NULL
    END,
    iconToken = CASE
      WHEN JSON_UNQUOTE(JSON_EXTRACT(extJson, '$.icon')) IN ('unknown','male','female','check','close','info')
        THEN JSON_UNQUOTE(JSON_EXTRACT(extJson, '$.icon'))
      ELSE NULL
    END
WHERE extJson IS NOT NULL AND TRIM(extJson) <> '' AND JSON_VALID(extJson);

-- +goose Down
ALTER TABLE sys_dict_item
  DROP COLUMN version,
  DROP COLUMN presentationVersion,
  DROP COLUMN iconToken,
  DROP COLUMN colorToken;

ALTER TABLE sys_dict_type
  DROP COLUMN version,
  DROP COLUMN schemaVersion,
  DROP COLUMN sensitivity,
  DROP COLUMN exposure,
  DROP COLUMN validationJson,
  DROP COLUMN uiWidget,
  DROP COLUMN valueType;

ALTER TABLE sys_config_change_log
  DROP COLUMN newValueProtected,
  DROP COLUMN oldValueProtected,
  MODIFY COLUMN id BIGINT NOT NULL COMMENT '主键ID';

ALTER TABLE sys_config
  DROP COLUMN version,
  DROP COLUMN schemaVersion,
  DROP COLUMN sensitivity,
  DROP COLUMN exposure,
  DROP COLUMN validationJson,
  DROP COLUMN uiWidget;
