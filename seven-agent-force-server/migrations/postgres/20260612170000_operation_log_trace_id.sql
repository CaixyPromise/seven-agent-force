-- +goose Up
-- Persist request trace IDs in operation logs so audit records can be joined
-- with HTTP responses and structured runtime logs.
-- +goose StatementBegin
DO $operation_log_trace_id$
BEGIN
  IF to_regclass('public.sys_operation_log') IS NOT NULL THEN
    EXECUTE 'ALTER TABLE sys_operation_log ADD COLUMN IF NOT EXISTS "traceId" VARCHAR(64)';
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_operation_log_trace_id ON sys_operation_log ("traceId")';
  END IF;
END
$operation_log_trace_id$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $operation_log_trace_id$
BEGIN
  IF to_regclass('public.sys_operation_log') IS NOT NULL THEN
    EXECUTE 'DROP INDEX IF EXISTS idx_operation_log_trace_id';
    EXECUTE 'ALTER TABLE sys_operation_log DROP COLUMN IF EXISTS "traceId"';
  END IF;
END
$operation_log_trace_id$;
-- +goose StatementEnd
