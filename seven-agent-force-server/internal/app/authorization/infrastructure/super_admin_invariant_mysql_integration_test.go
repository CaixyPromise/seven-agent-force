package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
)

func TestMySQLConcurrentSuperAdminRemovalsAreSerialized(t *testing.T) {
	dsn := os.Getenv("RBAC_MYSQL_DSN")
	if dsn == "" || os.Getenv("RBAC_MYSQL_ALLOW_MUTATION") != "1" {
		t.Skip("RBAC_MYSQL_DSN and RBAC_MYSQL_ALLOW_MUTATION=1 are required")
	}
	dsn = prepareRBACMySQLDatabase(t, dsn, os.Getenv("RBAC_MYSQL_CREATE_DATABASE"))
	provider, err := mysql.NewProvider(config.MySQLConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2,
	}, nil)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })

	ctx := context.Background()
	migrationsDir, err := filepath.Abs("../../../../migrations/mysql")
	if err != nil {
		t.Fatalf("resolve migrations: %v", err)
	}
	goose.SetTableName("goose_db_version")
	if err := goose.SetDialect("mysql"); err != nil {
		t.Fatalf("set goose dialect: %v", err)
	}
	if err := goose.UpContext(ctx, provider.DB(), migrationsDir); err != nil {
		t.Fatalf("migrate MySQL: %v", err)
	}

	const (
		userOneID    int64 = 9190718001
		userTwoID    int64 = 9190718002
		normalRoleID int64 = 9190718099
	)
	cleanup := func() {
		_, _ = provider.DB().ExecContext(context.Background(), `DELETE FROM sys_user_role WHERE userId IN (?, ?)`, userOneID, userTwoID)
		_, _ = provider.DB().ExecContext(context.Background(), `DELETE FROM sys_user WHERE id IN (?, ?)`, userOneID, userTwoID)
		_, _ = provider.DB().ExecContext(context.Background(), `DELETE FROM sys_role WHERE id = ?`, normalRoleID)
	}
	cleanup()
	t.Cleanup(cleanup)

	var superAdminRoleID int64
	if err := provider.DB().QueryRowContext(ctx, `SELECT id FROM sys_role WHERE systemKey = ? AND status = 0 AND isDeleted = 0 ORDER BY id LIMIT 1`, domain.AuthorizationRootSystemKey).Scan(&superAdminRoleID); err != nil {
		t.Fatalf("find SUPER_ADMIN role: %v", err)
	}
	var existingActiveSuperAdmins int
	if err := provider.DB().QueryRowContext(ctx, `
SELECT COUNT(DISTINCT su.id)
FROM sys_user su
JOIN sys_user_role sur ON sur.userId = su.id AND sur.isDeleted = 0
JOIN sys_role sr ON sr.id = sur.roleId
WHERE su.status = 0 AND su.isDeleted = 0
  AND sr.systemKey = ? AND sr.status = 0 AND sr.isDeleted = 0`, domain.AuthorizationRootSystemKey).Scan(&existingActiveSuperAdmins); err != nil {
		t.Fatalf("count existing SUPER_ADMIN users: %v", err)
	}
	if _, err := provider.DB().ExecContext(ctx, `INSERT INTO sys_role (id, name, code, type, status, dataScope, sortOrder, remark, isDeleted) VALUES (?, 'RBAC并发测试角色', 'RBAC_CONCURRENCY_TEST', 3, 0, 5, 0, 'integration test', 0)`, normalRoleID); err != nil {
		t.Fatalf("seed normal role: %v", err)
	}
	if _, err := provider.DB().ExecContext(ctx, `INSERT INTO sys_user (id, userAccount, nickName, status, userEmail, userGender, userAvatar, userProfile, isDeleted) VALUES (?, ?, ?, 0, ?, 0, '', '', 0), (?, ?, ?, 0, ?, 0, '', '', 0)`,
		userOneID, fmt.Sprintf("rbac%d", userOneID), "RBAC并发用户1", fmt.Sprintf("rbac%d@example.test", userOneID),
		userTwoID, fmt.Sprintf("rbac%d", userTwoID), "RBAC并发用户2", fmt.Sprintf("rbac%d@example.test", userTwoID)); err != nil {
		t.Fatalf("seed users: %v", err)
	}
	if _, err := provider.DB().ExecContext(ctx, `INSERT INTO sys_user_role (userId, roleId, isDeleted) VALUES (?, ?, 0), (?, ?, 0)`, userOneID, superAdminRoleID, userTwoID, superAdminRoleID); err != nil {
		t.Fatalf("seed SUPER_ADMIN relations: %v", err)
	}

	repository := &Repository{db: provider.SQLX()}
	transactor := provider.Transactor()
	var relationID atomic.Int64
	relationID.Store(9190718200)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, targetUserID := range []int64{userOneID, userTwoID} {
		wg.Add(1)
		go func(userID int64) {
			defer wg.Done()
			<-start
			results <- transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				snapshot, err := repository.LockSuperAdminInvariant(txCtx, userID)
				if err != nil {
					return err
				}
				if snapshot.WouldRemoveLastUser(false) {
					return apperrors.Operation("必须保留至少一个有效超级管理员")
				}
				return repository.ReplaceUserRoles(txCtx, userID, []int64{normalRoleID}, 0, func() int64 {
					return relationID.Add(1)
				})
			})
		}(targetUserID)
	}
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	blocked := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if appErr := apperrors.From(err); appErr != nil && appErr.Code() == apperrors.CodeOperateError {
			blocked++
			continue
		}
		t.Fatalf("unexpected concurrent mutation error: %v", err)
	}
	if existingActiveSuperAdmins == 0 && (successes != 1 || blocked != 1) {
		t.Fatalf("unexpected concurrent outcomes: successes=%d blocked=%d", successes, blocked)
	}

	var remaining int
	if err := provider.DB().QueryRowContext(ctx, `
SELECT COUNT(DISTINCT su.id)
FROM sys_user su
JOIN sys_user_role sur ON sur.userId = su.id AND sur.isDeleted = 0
JOIN sys_role sr ON sr.id = sur.roleId
WHERE su.id IN (?, ?) AND su.status = 0 AND su.isDeleted = 0
  AND sr.systemKey = ? AND sr.status = 0 AND sr.isDeleted = 0`, userOneID, userTwoID, domain.AuthorizationRootSystemKey).Scan(&remaining); err != nil {
		t.Fatalf("count remaining SUPER_ADMIN users: %v", err)
	}
	if existingActiveSuperAdmins == 0 && remaining != 1 {
		t.Fatalf("remaining active SUPER_ADMIN users=%d, want 1", remaining)
	}
	if existingActiveSuperAdmins > 0 {
		var globalRemaining int
		if err := provider.DB().QueryRowContext(ctx, `
SELECT COUNT(DISTINCT su.id)
FROM sys_user su
JOIN sys_user_role sur ON sur.userId = su.id AND sur.isDeleted = 0
JOIN sys_role sr ON sr.id = sur.roleId
WHERE su.status = 0 AND su.isDeleted = 0
  AND sr.systemKey = ? AND sr.status = 0 AND sr.isDeleted = 0`, domain.AuthorizationRootSystemKey).Scan(&globalRemaining); err != nil {
			t.Fatalf("count global remaining SUPER_ADMIN users: %v", err)
		}
		if globalRemaining < 1 {
			t.Fatalf("upgrade database lost all SUPER_ADMIN users")
		}
		t.Logf("upgrade database retained %d pre-existing active SUPER_ADMIN users; test mutations succeeded=%d blocked=%d", existingActiveSuperAdmins, successes, blocked)
	}
}

func TestMySQLAuthorizationRootBootstrapIsAtomicAndConfigurationIsOneShot(t *testing.T) {
	dsn := os.Getenv("RBAC_MYSQL_DSN")
	if dsn == "" || os.Getenv("RBAC_MYSQL_ALLOW_MUTATION") != "1" {
		t.Skip("RBAC_MYSQL_DSN and RBAC_MYSQL_ALLOW_MUTATION=1 are required")
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MySQL DSN: %v", err)
	}
	if !strings.Contains(parsed.DBName, "rbac_a1_gate") {
		t.Fatalf("bootstrap integration test requires an isolated rbac_a1_gate database, got %q", parsed.DBName)
	}
	provider, err := mysql.NewProvider(config.MySQLConfig{Enabled: true, DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2}, nil)
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	defer provider.Close()
	ctx := context.Background()
	if _, err := provider.DB().ExecContext(ctx, `DELETE FROM sys_user_role`); err != nil {
		t.Fatalf("clear isolated user roles: %v", err)
	}
	if _, err := provider.DB().ExecContext(ctx, `DELETE FROM sys_user`); err != nil {
		t.Fatalf("clear isolated users: %v", err)
	}
	if _, err := provider.DB().ExecContext(ctx, `DELETE FROM sys_security_bootstrap WHERE bootstrapKey = ?`, domain.AuthorizationRootSystemKey); err != nil {
		t.Fatalf("clear isolated bootstrap marker: %v", err)
	}
	if _, err := provider.DB().ExecContext(ctx, `UPDATE sys_role SET code = 'SUPER_ADMIN', name = '超级管理员' WHERE systemKey = ?`, domain.AuthorizationRootSystemKey); err != nil {
		t.Fatalf("reset isolated root role: %v", err)
	}

	repository := &Repository{db: provider.SQLX()}
	transactor := provider.Transactor()
	const conflictRoleID int64 = 9190718300
	_, _ = provider.DB().ExecContext(ctx, `DELETE FROM sys_role WHERE id = ?`, conflictRoleID)
	if _, err := provider.DB().ExecContext(ctx, `INSERT INTO sys_role (id, name, code, type, status, dataScope, sortOrder, isDeleted) VALUES (?, '冲突角色', 'CONFLICTING_ROOT', 3, 0, 5, 0, 0)`, conflictRoleID); err != nil {
		t.Fatalf("seed conflicting role: %v", err)
	}
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := repository.BootstrapAuthorizationRoot(txCtx, "CONFLICTING_ROOT", "冲突 Owner", time.Now().UTC())
		return err
	})
	if err == nil {
		t.Fatal("expected conflicting bootstrap code to fail")
	}
	var markerCount int
	if err := provider.DB().QueryRowContext(ctx, `SELECT COUNT(1) FROM sys_security_bootstrap WHERE bootstrapKey = ?`, domain.AuthorizationRootSystemKey).Scan(&markerCount); err != nil || markerCount != 0 {
		t.Fatalf("conflicting bootstrap must not write marker, count=%d err=%v", markerCount, err)
	}
	if _, err := provider.DB().ExecContext(ctx, `DELETE FROM sys_role WHERE id = ?`, conflictRoleID); err != nil {
		t.Fatalf("remove conflicting role: %v", err)
	}

	type bootstrapOutcome struct {
		result *domain.AuthorizationRootBootstrapResult
		err    error
	}
	start := make(chan struct{})
	outcomes := make(chan bootstrapOutcome, 2)
	var wg sync.WaitGroup
	for _, code := range []string{"OWNER_ALPHA", "OWNER_BETA"} {
		wg.Add(1)
		go func(candidate string) {
			defer wg.Done()
			<-start
			var result *domain.AuthorizationRootBootstrapResult
			err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				var innerErr error
				result, innerErr = repository.BootstrapAuthorizationRoot(txCtx, candidate, candidate, time.Now().UTC())
				return innerErr
			})
			outcomes <- bootstrapOutcome{result: result, err: err}
		}(code)
	}
	close(start)
	wg.Wait()
	close(outcomes)
	initialized := 0
	alreadyInitialized := 0
	for outcome := range outcomes {
		if outcome.err != nil || outcome.result == nil {
			t.Fatalf("concurrent bootstrap failed: result=%#v err=%v", outcome.result, outcome.err)
		}
		if outcome.result.AlreadyInitialized {
			alreadyInitialized++
		} else {
			initialized++
		}
	}
	if initialized != 1 || alreadyInitialized != 1 {
		t.Fatalf("unexpected concurrent bootstrap outcomes: initialized=%d already=%d", initialized, alreadyInitialized)
	}
	var persistedCode string
	if err := provider.DB().QueryRowContext(ctx, `SELECT code FROM sys_role WHERE systemKey = ?`, domain.AuthorizationRootSystemKey).Scan(&persistedCode); err != nil {
		t.Fatalf("read persisted root code: %v", err)
	}
	if persistedCode != "OWNER_ALPHA" && persistedCode != "OWNER_BETA" {
		t.Fatalf("unexpected persisted root code: %s", persistedCode)
	}
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		result, err := repository.BootstrapAuthorizationRoot(txCtx, "OWNER_CHANGED", "Changed", time.Now().UTC())
		if err != nil {
			return err
		}
		if !result.AlreadyInitialized || result.Role.Code != persistedCode {
			return fmt.Errorf("configuration changed persisted root: %#v", result)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify one-shot bootstrap configuration: %v", err)
	}
}

func prepareRBACMySQLDatabase(t *testing.T, baseDSN, databaseName string) string {
	t.Helper()
	if databaseName == "" {
		return baseDSN
	}
	for _, char := range databaseName {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
			t.Fatalf("unsafe RBAC_MYSQL_CREATE_DATABASE value %q", databaseName)
		}
	}
	baseConfig, err := mysqldriver.ParseDSN(baseDSN)
	if err != nil {
		t.Fatalf("parse base MySQL DSN: %v", err)
	}
	baseConfig.DBName = ""
	adminDB, err := sql.Open("mysql", baseConfig.FormatDSN())
	if err != nil {
		t.Fatalf("open MySQL admin connection: %v", err)
	}
	defer adminDB.Close()
	if _, err := adminDB.ExecContext(context.Background(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); err != nil {
		t.Fatalf("create clean MySQL database %s: %v", databaseName, err)
	}
	t.Cleanup(func() {
		cleanupDB, err := sql.Open("mysql", baseConfig.FormatDSN())
		if err != nil {
			return
		}
		defer cleanupDB.Close()
		_, _ = cleanupDB.ExecContext(context.Background(), "DROP DATABASE `"+databaseName+"`")
	})
	targetConfig := *baseConfig
	targetConfig.DBName = databaseName
	return targetConfig.FormatDSN()
}
