package schemaalignment

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func ensureWorkerMetricSchema(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS workforce_worker_metric_samples (
	worker_id BIGINT NOT NULL,
	sampled_at TIMESTAMPTZ NOT NULL,
	logical_cpu_cores SMALLINT NOT NULL CHECK (logical_cpu_cores > 0),
	cpu_usage_percent REAL CHECK (cpu_usage_percent BETWEEN 0 AND 100),
	load1 REAL CHECK (load1 >= 0),
	memory_used_bytes BIGINT NOT NULL CHECK (memory_used_bytes >= 0),
	memory_total_bytes BIGINT NOT NULL CHECK (memory_total_bytes >= memory_used_bytes),
	memory_usage_percent REAL NOT NULL CHECK (memory_usage_percent BETWEEN 0 AND 100),
	workspace_disk_used_bytes BIGINT NOT NULL CHECK (workspace_disk_used_bytes >= 0),
	workspace_disk_total_bytes BIGINT NOT NULL CHECK (workspace_disk_total_bytes >= workspace_disk_used_bytes),
	workspace_disk_usage_percent REAL NOT NULL CHECK (workspace_disk_usage_percent BETWEEN 0 AND 100),
	active_attempts SMALLINT NOT NULL CHECK (active_attempts >= 0),
	max_active_attempts SMALLINT NOT NULL CHECK (max_active_attempts > 0 AND active_attempts <= max_active_attempts),
	docker_ready BOOLEAN NOT NULL,
	PRIMARY KEY (worker_id, sampled_at)
);
CREATE INDEX IF NOT EXISTS workforce_worker_metric_samples_sampled_at_brin
	ON workforce_worker_metric_samples USING BRIN (sampled_at);
COMMENT ON TABLE workforce_worker_metric_samples IS 'Worker 每分钟资源与任务负载样本';
COMMENT ON COLUMN workforce_worker_metric_samples.worker_id IS 'Worker 标识';
COMMENT ON COLUMN workforce_worker_metric_samples.sampled_at IS 'Server 记录的采样分钟';
COMMENT ON COLUMN workforce_worker_metric_samples.logical_cpu_cores IS '逻辑 CPU 核数';
COMMENT ON COLUMN workforce_worker_metric_samples.cpu_usage_percent IS 'CPU 使用率，无法采集时为空';
COMMENT ON COLUMN workforce_worker_metric_samples.load1 IS '一分钟系统负载';
COMMENT ON COLUMN workforce_worker_metric_samples.memory_used_bytes IS '已使用内存字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.memory_total_bytes IS '内存总字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.memory_usage_percent IS '内存使用率';
COMMENT ON COLUMN workforce_worker_metric_samples.workspace_disk_used_bytes IS '工作区磁盘已使用字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.workspace_disk_total_bytes IS '工作区磁盘总字节数';
COMMENT ON COLUMN workforce_worker_metric_samples.workspace_disk_usage_percent IS '工作区磁盘使用率';
COMMENT ON COLUMN workforce_worker_metric_samples.active_attempts IS '采样时正在占用的任务槽位';
COMMENT ON COLUMN workforce_worker_metric_samples.max_active_attempts IS '采样时可用的任务槽位总数';
COMMENT ON COLUMN workforce_worker_metric_samples.docker_ready IS '采样时 Docker 是否可用';`)
	if err != nil {
		return fmt.Errorf("ensure Worker metric schema: %w", err)
	}
	return nil
}

func verifyWorkerMetricSchema(ctx context.Context, db DB) error {
	var columns, comments, indexes int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND table_name='workforce_worker_metric_samples'
		AND (column_name,data_type) IN (
			('worker_id','bigint'),('sampled_at','timestamp with time zone'),('logical_cpu_cores','smallint'),
			('cpu_usage_percent','real'),('load1','real'),('memory_used_bytes','bigint'),('memory_total_bytes','bigint'),
			('memory_usage_percent','real'),('workspace_disk_used_bytes','bigint'),('workspace_disk_total_bytes','bigint'),
			('workspace_disk_usage_percent','real'),('active_attempts','smallint'),('max_active_attempts','smallint'),('docker_ready','boolean'))`).Scan(&columns); err != nil {
		return err
	}
	if columns != 14 {
		return fmt.Errorf("Worker metric schema has %d expected columns, want 14", columns)
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_description d
		JOIN pg_class c ON c.oid=d.objoid JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname='public' AND c.relname='workforce_worker_metric_samples' AND d.description<>''`).Scan(&comments); err != nil {
		return err
	}
	if comments < 15 {
		return fmt.Errorf("Worker metric schema comments are incomplete")
	}
	if err := db.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE schemaname='public' AND tablename='workforce_worker_metric_samples'
		AND (indexname='workforce_worker_metric_samples_pkey' OR indexname='workforce_worker_metric_samples_sampled_at_brin')`).Scan(&indexes); err != nil {
		return err
	}
	if indexes != 2 {
		return fmt.Errorf("Worker metric schema indexes are incomplete")
	}
	return nil
}
