-- +goose Up
UPDATE sys_menu
SET visible = 0, updateTime = NOW()
WHERE id IN (1900300200, 1900300201, 1900300202, 1900300203)
  AND isDeleted = 0
  AND visible <> 0;

-- +goose Down
UPDATE sys_menu
SET visible = 1, updateTime = NOW()
WHERE id IN (1900300200, 1900300201, 1900300202, 1900300203)
  AND isDeleted = 0;
