package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
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

func TestDictRepositoryActualCRUD(t *testing.T) {
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
	dictType := &domain.DictType{
		DictCode: uniqueCode, DictName: "DC23 Acceptance", Module: "test", Status: 1,
		ValueType: "STRING", UIWidget: "SELECT", Exposure: "INTERNAL", Sensitivity: "NORMAL",
		SchemaVersion: 1, Version: 1, SortOrder: 1, CreatedBy: 1, UpdatedBy: 1, CreateTime: &now, UpdateTime: &now,
	}
	typeID, err := repo.InsertType(ctx, dictType)
	if err != nil {
		t.Fatalf("insert dict type: %v", err)
	}
	dictType.ID = typeID
	foundType, err := repo.FindTypeByID(ctx, typeID)
	if err != nil || foundType == nil || foundType.DictCode != dictType.DictCode {
		t.Fatalf("read dict type: item=%#v err=%v", foundType, err)
	}
	staleType := *foundType
	foundType.DictName = "Updated Acceptance"
	if err := repo.UpdateType(ctx, foundType); err != nil {
		t.Fatalf("update dict type: %v", err)
	}
	if foundType.Version != 2 {
		t.Fatalf("expected type version 2, got %d", foundType.Version)
	}
	if err := repo.UpdateType(ctx, &staleType); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected stale type conflict, got %v", err)
	}
	neighborType := &domain.DictType{
		DictCode: uniqueCode + "_neighbor", DictName: "DC23 Neighbor", Module: "test", Status: 1,
		ValueType: "STRING", UIWidget: "SELECT", Exposure: "INTERNAL", Sensitivity: "NORMAL",
		SchemaVersion: 1, Version: 1, SortOrder: 2, CreatedBy: 1, UpdatedBy: 1, CreateTime: &now, UpdateTime: &now,
	}
	neighborTypeID, err := repo.InsertType(ctx, neighborType)
	if err != nil {
		t.Fatalf("insert neighboring dict type: %v", err)
	}
	if err := repo.ShiftTypeSort(ctx, typeID, 1, 2); err != nil {
		t.Fatalf("shift neighboring dict type: %v", err)
	}
	shiftedNeighborType, err := repo.FindTypeByID(ctx, neighborTypeID)
	if err != nil || shiftedNeighborType == nil || shiftedNeighborType.Version != 2 {
		t.Fatalf("shift must advance neighboring type version: item=%#v err=%v", shiftedNeighborType, err)
	}
	item := &domain.DictItem{
		DictTypeID: typeID, ItemValue: "safe", ItemLabel: "Safe", Status: 1,
		PresentationVersion: 1, Version: 1, CreatedBy: 1, UpdatedBy: 1, CreateTime: &now, UpdateTime: &now,
	}
	itemID, err := repo.InsertItem(ctx, item)
	if err != nil {
		t.Fatalf("insert dict item: %v", err)
	}
	item.ID = itemID
	foundItem, err := repo.FindItemByID(ctx, itemID)
	if err != nil || foundItem == nil || foundItem.ItemLabel != "Safe" {
		t.Fatalf("read dict item: item=%#v err=%v", foundItem, err)
	}
	staleItem := *foundItem
	foundItem.ItemLabel = "Safer"
	if err := repo.UpdateItem(ctx, foundItem); err != nil {
		t.Fatalf("update dict item: %v", err)
	}
	if foundItem.Version != 2 {
		t.Fatalf("expected item version 2, got %d", foundItem.Version)
	}
	if err := repo.UpdateItem(ctx, &staleItem); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected stale item conflict, got %v", err)
	}
	neighbor := &domain.DictItem{
		DictTypeID: typeID, ItemValue: "fast", ItemLabel: "Fast", Status: 1, SortOrder: 2,
		PresentationVersion: 1, Version: 1, CreatedBy: 1, UpdatedBy: 1, CreateTime: &now, UpdateTime: &now,
	}
	neighborID, err := repo.InsertItem(ctx, neighbor)
	if err != nil {
		t.Fatalf("insert neighboring dict item: %v", err)
	}
	neighbor.ID = neighborID
	foundItem.IsDeleted = 1
	if err := repo.UpdateItem(ctx, foundItem); err != nil {
		t.Fatalf("soft delete dict item: %v", err)
	}
	resurrectDeleted := *foundItem
	resurrectDeleted.IsDeleted = 0
	if err := repo.UpdateItem(ctx, &resurrectDeleted); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected deleted item resurrection conflict, got %v", err)
	}
	deleted, err := repo.FindItemByID(ctx, itemID)
	if err != nil || deleted != nil {
		t.Fatalf("deleted dict item must be absent: item=%#v err=%v", deleted, err)
	}
	if err := repo.ShiftItemSort(ctx, typeID, itemID, 1, 2); err != nil {
		t.Fatalf("shift neighboring dict item: %v", err)
	}
	shiftedNeighbor, err := repo.FindItemByID(ctx, neighborID)
	if err != nil || shiftedNeighbor == nil || shiftedNeighbor.Version != 2 {
		t.Fatalf("shift must advance neighboring item version: item=%#v err=%v", shiftedNeighbor, err)
	}
	if err := repo.SoftDeleteItemsByTypeID(ctx, typeID, 1, now); err != nil {
		t.Fatalf("cascade soft delete items: %v", err)
	}
	if err := repo.UpdateItem(ctx, shiftedNeighbor); apperrors.From(err).Code() != apperrors.CodeObjectStateInvalid {
		t.Fatalf("expected cascade-deleted item stale conflict, got %v", err)
	}
}
