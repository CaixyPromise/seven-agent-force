-- +goose Up
UPDATE sys_menu
SET visible = 1,
    status = 0,
    hierarchy = '/1900300200',
    level = 1,
    updateTime = NOW()
WHERE path = '/system'
  AND parentId = 0
  AND isDeleted = 0;

UPDATE sys_menu
SET isDeleted = 1,
    visible = 0,
    status = 1,
    updateTime = NOW()
WHERE path = '/system/security'
  AND parentId = 1
  AND isDeleted = 0;

-- +goose Down
UPDATE sys_menu
SET isDeleted = 0,
    visible = 1,
    status = 0,
    updateTime = NOW()
WHERE path = '/system/security'
  AND parentId = 1
  AND isDeleted = 1;

UPDATE sys_menu
SET visible = 0,
    updateTime = NOW()
WHERE path = '/system'
  AND parentId = 0
  AND isDeleted = 0;
