-- +goose Up
SET @sql := (
    SELECT IF(COUNT(*) = 0,
        'ALTER TABLE sysPlatform ADD COLUMN allowFormRegister TINYINT NOT NULL DEFAULT 0 COMMENT ''是否允许表单注册'' AFTER allowAutoRegister',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysPlatform' AND column_name = 'allowFormRegister'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- +goose Down
SET @sql := (
    SELECT IF(COUNT(*) > 0,
        'ALTER TABLE sysPlatform DROP COLUMN allowFormRegister',
        'SELECT 1')
    FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'sysPlatform' AND column_name = 'allowFormRegister'
);
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;
