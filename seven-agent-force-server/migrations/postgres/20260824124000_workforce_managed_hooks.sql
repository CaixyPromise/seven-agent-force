-- +goose Up

ALTER TABLE workforce_harness_releases DROP CONSTRAINT IF EXISTS workforce_harness_releases_alias_key;
CREATE UNIQUE INDEX IF NOT EXISTS workforce_harness_releases_digest_uidx ON workforce_harness_releases (image_digest);

CREATE TABLE workforce_harness_channels (
    alias VARCHAR(128) PRIMARY KEY,
    harness_release_id BIGINT NOT NULL,
    updated_by_principal_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO workforce_harness_channels (alias, harness_release_id)
SELECT DISTINCT ON (alias) alias, id
FROM workforce_harness_releases
WHERE status = 'ACTIVE'
ORDER BY alias, updated_at DESC
ON CONFLICT (alias) DO NOTHING;

CREATE TABLE workforce_hook_packages (
    id BIGINT PRIMARY KEY,
    scope_type VARCHAR(16) NOT NULL CHECK (scope_type IN ('SYSTEM','TENANT')),
    tenant_id BIGINT,
    name VARCHAR(255) NOT NULL,
    category VARCHAR(64) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    trust_class VARCHAR(32) NOT NULL DEFAULT 'PLATFORM_TRUSTED',
    created_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id IS NOT NULL))
);
CREATE UNIQUE INDEX workforce_hook_packages_system_name_uidx ON workforce_hook_packages (name) WHERE scope_type='SYSTEM';
CREATE UNIQUE INDEX workforce_hook_packages_tenant_name_uidx ON workforce_hook_packages (tenant_id,name) WHERE scope_type='TENANT';

CREATE TABLE workforce_hook_package_revisions (
    id BIGINT PRIMARY KEY,
    package_id BIGINT NOT NULL,
    scope_type VARCHAR(16) NOT NULL CHECK (scope_type IN ('SYSTEM','TENANT')),
    tenant_id BIGINT,
    revision INTEGER NOT NULL DEFAULT 1,
    manifest_json JSONB NOT NULL,
    files_json JSONB NOT NULL,
    validation_json JSONB,
    status VARCHAR(32) NOT NULL DEFAULT 'DRAFT',
    created_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id IS NOT NULL))
);

CREATE TABLE workforce_hook_package_releases (
    id BIGINT PRIMARY KEY,
    package_id BIGINT NOT NULL,
    revision_id BIGINT NOT NULL,
    scope_type VARCHAR(16) NOT NULL CHECK (scope_type IN ('SYSTEM','TENANT')),
    tenant_id BIGINT,
    version VARCHAR(64) NOT NULL,
    runtime_kind VARCHAR(32) NOT NULL,
    manifest_json JSONB NOT NULL,
    files_json JSONB NOT NULL,
    archive_object_key VARCHAR(1024) NOT NULL UNIQUE,
    archive_sha256 VARCHAR(64) NOT NULL,
    archive_size_bytes BIGINT NOT NULL,
    published_by_principal_id BIGINT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK ((scope_type='SYSTEM' AND tenant_id IS NULL) OR (scope_type='TENANT' AND tenant_id IS NOT NULL)),
    UNIQUE (package_id, version)
);

CREATE TABLE workforce_hook_test_runs (
    id BIGINT PRIMARY KEY,
    revision_id BIGINT NOT NULL,
    tenant_id BIGINT NOT NULL,
    event_name VARCHAR(128) NOT NULL,
    fixture_json JSONB NOT NULL,
    decision VARCHAR(32) NOT NULL,
    reason_code VARCHAR(128) NOT NULL,
    diagnostics TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_by_principal_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX workforce_hook_packages_tenant_updated_idx ON workforce_hook_packages (tenant_id, updated_at DESC);
CREATE INDEX workforce_hook_package_releases_tenant_published_idx ON workforce_hook_package_releases (tenant_id, published_at DESC);
CREATE INDEX workforce_hook_test_runs_revision_idx ON workforce_hook_test_runs (revision_id, created_at DESC);

COMMENT ON TABLE workforce_harness_channels IS '运行环境发布通道';
COMMENT ON TABLE workforce_hook_packages IS 'Hook 定义';
COMMENT ON TABLE workforce_hook_package_revisions IS 'Hook 草稿版本';
COMMENT ON TABLE workforce_hook_package_releases IS 'Hook 发布版本';
COMMENT ON TABLE workforce_hook_test_runs IS 'Hook 测试记录';
COMMENT ON COLUMN workforce_hook_package_revisions.revision IS '草稿版本号';
COMMENT ON COLUMN workforce_hook_test_runs.duration_ms IS '测试耗时，单位毫秒';

-- +goose Down

DROP TABLE IF EXISTS workforce_hook_test_runs;
DROP TABLE IF EXISTS workforce_hook_package_releases;
DROP TABLE IF EXISTS workforce_hook_package_revisions;
DROP TABLE IF EXISTS workforce_hook_packages;
DROP TABLE IF EXISTS workforce_harness_channels;
DROP INDEX IF EXISTS workforce_harness_releases_digest_uidx;
ALTER TABLE workforce_harness_releases ADD CONSTRAINT workforce_harness_releases_alias_key UNIQUE (alias);
