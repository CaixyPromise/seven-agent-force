package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	dbgovernance "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type integrationProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *integrationProvider) Driver() string               { return p.driver }
func (p *integrationProvider) Dialect() string              { return p.dialect }
func (p *integrationProvider) DB() *sql.DB                  { return p.db }
func (p *integrationProvider) SQLX() *sqlx.DB               { return p.sqlxDB }
func (p *integrationProvider) Transactor() store.Transactor { return nil }
func (p *integrationProvider) Configured() bool             { return true }
func (p *integrationProvider) Close() error                 { return p.db.Close() }

func TestConfigRepositoryActualCRUD(t *testing.T) {
	dialect := os.Getenv("DC23_TEST_DIALECT")
	dsn := os.Getenv("DC23_TEST_DSN")
	if dialect == "" || dsn == "" {
		t.Skip("set DC23_TEST_DIALECT and DC23_TEST_DSN for isolated database acceptance")
	}
	driver := "mysql"
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open %s: %v", dialect, err)
	}
	if err := dbgovernance.AssertConnectedDatabase(context.Background(), db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	provider := &integrationProvider{
		driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver),
	}
	t.Cleanup(func() { _ = provider.Close() })
	repo, err := NewRepository(provider)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	uniqueCode := fmt.Sprintf("dc23_acceptance_%d", now.UnixNano())
	group := &domain.ConfigGroup{
		GroupCode: uniqueCode, GroupName: "DC23 Acceptance", Module: "test",
		Status: 1, CreateTime: &now, UpdateTime: &now,
	}
	groupID, err := repo.InsertGroup(ctx, group)
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}
	group.ID = groupID
	foundGroup, err := repo.FindGroupByID(ctx, groupID)
	if err != nil || foundGroup == nil || foundGroup.GroupCode != group.GroupCode {
		t.Fatalf("read group: item=%#v err=%v", foundGroup, err)
	}
	item := &domain.Config{
		GroupID: groupID, ConfigKey: "runtime.title", ConfigValue: "First", ValueType: "STRING",
		ConfigDesc: "Acceptance title", UIWidget: "INPUT", Exposure: "INTERNAL", Sensitivity: "NORMAL",
		SchemaVersion: 1, Version: 1, IsEnabled: 1, EffectType: "restart",
		CreatedBy: 1, UpdatedBy: 1, CreateTime: &now, UpdateTime: &now,
	}
	id, err := repo.InsertConfig(ctx, item)
	if err != nil {
		t.Fatalf("insert config: %v", err)
	}
	item.ID = id
	found, err := repo.FindConfigByID(ctx, id)
	if err != nil || found == nil || found.ConfigValue != "First" {
		t.Fatalf("read config: item=%#v err=%v", found, err)
	}
	changeLogID, err := repo.InsertChangeLog(ctx, &domain.ConfigChangeLog{
		ConfigID:      id,
		ConfigKey:     item.ConfigKey,
		OperationType: "UPDATE",
		OldValue:      "First",
		NewValue:      "Second",
		EffectType:    "restart",
		Status:        "applied",
		OperatorID:    1,
		OperationTime: &now,
	})
	if err != nil || changeLogID <= 0 {
		t.Fatalf("insert config change log: id=%d err=%v", changeLogID, err)
	}
	t.Run("pending batch apply is atomic and idempotent", func(t *testing.T) {
		makeConfig := func(key, value string) domain.Config {
			return domain.Config{
				GroupID: groupID, ConfigKey: key, ConfigValue: value, ValueType: "STRING",
				UIWidget: "INPUT", Exposure: "INTERNAL", Sensitivity: "NORMAL",
				SchemaVersion: 1, Version: 1, IsEnabled: 1, EffectType: "restart",
				CreatedBy: 1, UpdatedBy: 1, CreateTime: &now, UpdateTime: &now,
			}
		}
		insertPending := func(t *testing.T, configID int64, key, oldValue, newValue string) int64 {
			t.Helper()
			id, insertErr := repo.InsertChangeLog(ctx, &domain.ConfigChangeLog{
				ConfigID: configID, ConfigKey: key, OperationType: "UPDATE",
				OldValue: oldValue, NewValue: newValue, EffectType: "restart",
				Status: "pending", OperatorID: 1, OperationTime: &now,
			})
			if insertErr != nil {
				t.Fatalf("insert pending log %s: %v", key, insertErr)
			}
			return id
		}
		buildApply := func(config domain.Config, pendingID int64, oldValue, newValue string) domain.PendingConfigApply {
			config.ConfigValue = newValue
			config.UpdatedBy = 1
			config.UpdateTime = &now
			parentID := pendingID
			appliedBy := int64(1)
			return domain.PendingConfigApply{
				PendingLogID: pendingID,
				Config:       config,
				ApplyLog: domain.ConfigChangeLog{
					ConfigID: config.ID, ConfigKey: config.ConfigKey, OperationType: "APPLY",
					OldValue: oldValue, NewValue: newValue, EffectType: "restart", Status: "applied",
					ParentLogID: &parentID, OperatorID: 1, OperatorName: "integration",
					OperationTime: &now, AppliedBy: &appliedBy, AppliedTime: &now,
				},
			}
		}
		insertConfig := func(t *testing.T, config *domain.Config) {
			t.Helper()
			configID, insertErr := repo.InsertConfig(ctx, config)
			if insertErr != nil {
				t.Fatalf("insert batch config %s: %v", config.ConfigKey, insertErr)
			}
			config.ID = configID
		}
		runBatch := func(items []domain.PendingConfigApply) ([]int64, error) {
			var claimed []int64
			txErr := store.NewSQLXTransactor(provider.sqlxDB).WithinTransaction(ctx, func(txCtx context.Context) error {
				var applyErr error
				claimed, applyErr = repo.ApplyPendingConfigBatch(txCtx, items)
				return applyErr
			})
			return claimed, txErr
		}

		first := makeConfig("batch.first", "old-1")
		second := makeConfig("batch.second", "old-2")
		insertConfig(t, &first)
		insertConfig(t, &second)
		firstLogID := insertPending(t, first.ID, first.ConfigKey, "old-1", "new-1")
		secondLogID := insertPending(t, second.ID, second.ConfigKey, "old-2", "new-2")
		successBatch := []domain.PendingConfigApply{
			buildApply(first, firstLogID, "old-1", "new-1"),
			buildApply(second, secondLogID, "old-2", "new-2"),
		}
		claimed, applyErr := runBatch(successBatch)
		if applyErr != nil || len(claimed) != 2 {
			t.Fatalf("apply two pending configs: claimed=%#v err=%v", claimed, applyErr)
		}
		for _, expected := range []struct {
			configID int64
			logID    int64
			value    string
		}{
			{first.ID, firstLogID, "new-1"},
			{second.ID, secondLogID, "new-2"},
		} {
			foundConfig, findErr := repo.FindConfigByID(ctx, expected.configID)
			if findErr != nil || foundConfig == nil || foundConfig.ConfigValue != expected.value || foundConfig.Version != 2 {
				t.Fatalf("verify applied config %d: item=%#v err=%v", expected.configID, foundConfig, findErr)
			}
			foundLog, findErr := repo.FindChangeLogByID(ctx, expected.logID)
			if findErr != nil || foundLog == nil || foundLog.Status != "applied" {
				t.Fatalf("verify claimed log %d: item=%#v err=%v", expected.logID, foundLog, findErr)
			}
		}
		retryClaimed, retryErr := runBatch(successBatch)
		if retryErr != nil || len(retryClaimed) != 0 {
			t.Fatalf("repeat apply must be idempotent: claimed=%#v err=%v", retryClaimed, retryErr)
		}
		audits, auditErr := repo.ListChangeLogsReferencing(ctx, []int64{firstLogID, secondLogID})
		if auditErr != nil {
			t.Fatalf("list apply audits: %v", auditErr)
		}
		applyAuditCount := 0
		for _, audit := range audits {
			if audit.OperationType == "APPLY" {
				applyAuditCount++
			}
		}
		if applyAuditCount != 2 {
			t.Fatalf("apply audit count=%d, want 2", applyAuditCount)
		}

		conflict := makeConfig("batch.conflict", "old-conflict")
		peer := makeConfig("batch.peer", "old-peer")
		insertConfig(t, &conflict)
		insertConfig(t, &peer)
		conflictLogID := insertPending(t, conflict.ID, conflict.ConfigKey, "old-conflict", "new-conflict")
		peerLogID := insertPending(t, peer.ID, peer.ConfigKey, "old-peer", "new-peer")
		staleConflict := conflict
		conflict.ConfigValue = "concurrent"
		if updateErr := repo.UpdateConfig(ctx, &conflict); updateErr != nil {
			t.Fatalf("create version conflict: %v", updateErr)
		}
		_, conflictErr := runBatch([]domain.PendingConfigApply{
			buildApply(staleConflict, conflictLogID, "old-conflict", "new-conflict"),
			buildApply(peer, peerLogID, "old-peer", "new-peer"),
		})
		if apperrors.From(conflictErr).Code() != apperrors.CodeObjectStateInvalid {
			t.Fatalf("expected whole-batch version conflict, got %v", conflictErr)
		}
		foundPeer, findErr := repo.FindConfigByID(ctx, peer.ID)
		if findErr != nil || foundPeer == nil || foundPeer.ConfigValue != "old-peer" || foundPeer.Version != 1 {
			t.Fatalf("peer update was not rolled back: item=%#v err=%v", foundPeer, findErr)
		}
		for _, pendingID := range []int64{conflictLogID, peerLogID} {
			foundLog, findErr := repo.FindChangeLogByID(ctx, pendingID)
			if findErr != nil || foundLog == nil || foundLog.Status != "pending" {
				t.Fatalf("claim was not rolled back for %d: item=%#v err=%v", pendingID, foundLog, findErr)
			}
		}
		conflictAudits, auditErr := repo.ListChangeLogsReferencing(ctx, []int64{conflictLogID, peerLogID})
		if auditErr != nil {
			t.Fatalf("list conflict audits: %v", auditErr)
		}
		for _, audit := range conflictAudits {
			if audit.OperationType == "APPLY" {
				t.Fatalf("rolled-back batch emitted apply audit: %#v", audit)
			}
		}
	})
	stale := *found
	found.ConfigValue = "Second"
	if err := repo.UpdateConfig(ctx, found); err != nil {
		t.Fatalf("update config: %v", err)
	}
	if found.Version != 2 {
		t.Fatalf("expected version 2, got %d", found.Version)
	}
	stale.ConfigValue = "Stale"
	if err := repo.UpdateConfig(ctx, &stale); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected stale update conflict, got %v", err)
	}
	found.IsDeleted = 1
	if err := repo.UpdateConfig(ctx, found); err != nil {
		t.Fatalf("soft delete config: %v", err)
	}
	deleted, err := repo.FindConfigByID(ctx, id)
	if err != nil || deleted != nil {
		t.Fatalf("deleted config must be absent: item=%#v err=%v", deleted, err)
	}
}
