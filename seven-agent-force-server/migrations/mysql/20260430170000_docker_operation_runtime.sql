-- +goose Up
-- Docker operation runtime schema and permissions are defined in
-- 20260430090000_docker_starter.sql for fresh installs. This migration is kept
-- as a no-op compatibility marker to avoid duplicate table definitions and
-- non-portable ALTER TABLE IF NOT EXISTS syntax on existing MySQL variants.

-- +goose Down
-- No-op compatibility marker.
