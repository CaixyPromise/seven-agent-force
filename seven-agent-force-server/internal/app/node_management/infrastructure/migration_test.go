package infrastructure

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const nodeCommandOrderingMigration = "../../../../migrations/mysql/20260711223000_node_command_ordering.sql"
const ssoCutoffPrecisionMigration = "../../../../migrations/mysql/20260712100000_sso_cutoff_precision.sql"
const nodeStatusCommandHashMigration = "../../../../migrations/mysql/20260712110000_node_status_command_hash.sql"

func TestNodeCommandOrderingMigrationHasExactSchemaContracts(t *testing.T) {
	payload, err := os.ReadFile(nodeCommandOrderingMigration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(payload)), " ")
	assertMigrationPattern(t, normalized, `-- \+goose Up .*ALTER TABLE sys_user ADD COLUMN statusVersion BIGINT UNSIGNED NOT NULL DEFAULT 0`)
	assertMigrationPattern(t, normalized, `ADD (?:KEY|INDEX) idx_sysSsoRefreshTokenFamily_user_status_deleted_createTime \(userId, status, isDeleted, createTime\)`)
	assertMigrationPattern(t, normalized, `-- \+goose Down .*DROP (?:KEY|INDEX) idx_sysSsoRefreshTokenFamily_user_status_deleted_createTime`)
	assertMigrationPattern(t, normalized, `ALTER TABLE sys_user DROP COLUMN statusVersion`)
}

func TestNodeStatusCommandHashMigrationHasExactUpAndDownContracts(t *testing.T) {
	payload, err := os.ReadFile(nodeStatusCommandHashMigration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(payload)), " ")
	assertMigrationPattern(t, normalized, `-- \+goose Up .*ALTER TABLE sys_user ADD COLUMN statusCommandHash CHAR\(64\) NULL COMMENT '节点状态命令哈希' AFTER statusVersion`)
	assertMigrationPattern(t, normalized, `-- \+goose Down .*ALTER TABLE sys_user DROP COLUMN statusCommandHash`)
}

func TestSSOCutoffPrecisionMigrationHasExactUpAndDownContracts(t *testing.T) {
	payload, err := os.ReadFile(ssoCutoffPrecisionMigration)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(payload)), " ")
	assertMigrationPattern(t, normalized, `-- \+goose Up .*ALTER TABLE sysSsoSession MODIFY COLUMN createTime DATETIME\(6\) NOT NULL DEFAULT CURRENT_TIMESTAMP\(6\) COMMENT '创建时间'`)
	assertMigrationPattern(t, normalized, `ALTER TABLE sysSsoRefreshTokenFamily MODIFY COLUMN createTime DATETIME\(6\) NOT NULL DEFAULT CURRENT_TIMESTAMP\(6\) COMMENT '创建时间'`)
	assertMigrationPattern(t, normalized, `-- \+goose Down .*ALTER TABLE sysSsoSession MODIFY COLUMN createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间'`)
	assertMigrationPattern(t, normalized, `ALTER TABLE sysSsoRefreshTokenFamily MODIFY COLUMN createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间'`)
}

func assertMigrationPattern(t *testing.T, value, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(value) {
		t.Fatalf("migration missing exact contract %q\n%s", pattern, value)
	}
}
