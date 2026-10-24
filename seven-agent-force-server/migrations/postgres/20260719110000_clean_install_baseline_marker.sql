-- +goose Up

-- This marker aligns existing upgrade databases with the clean-install
-- snapshot cutoff. Existing databases have already applied the historical
-- migrations and require no schema mutation here.
SELECT 1;

-- +goose Down

SELECT 1;
