package domain

import (
	"context"
	"time"
)

type Repository interface {
	FindTypeByID(ctx context.Context, id int64) (*DictType, error)
	FindTypeByCode(ctx context.Context, dictCode string) (*DictType, error)
	CountTypeByCode(ctx context.Context, dictCode string, excludeID int64) (int64, error)
	InsertType(ctx context.Context, item *DictType) (int64, error)
	UpdateType(ctx context.Context, item *DictType) error
	QueryTypes(ctx context.Context, query DictTypePageQuery) (*DictTypePage, error)
	CountItemsByTypeID(ctx context.Context, typeID int64) (int64, error)
	CountItemsByTypeIDs(ctx context.Context, typeIDs []int64) (map[int64]int64, error)
	SoftDeleteItemsByTypeID(ctx context.Context, typeID, actorID int64, updatedAt time.Time) error
	ShiftTypeSort(ctx context.Context, targetID int64, oldOrder, newOrder int) error
	FindReadableTypesByCodes(ctx context.Context, dictCodes []string) ([]DictType, error)

	FindItemByID(ctx context.Context, id int64) (*DictItem, error)
	CountItemByValue(ctx context.Context, typeID int64, itemValue string, excludeID int64) (int64, error)
	InsertItem(ctx context.Context, item *DictItem) (int64, error)
	UpdateItem(ctx context.Context, item *DictItem) error
	QueryItems(ctx context.Context, query DictItemListQuery) ([]DictItem, error)
	ListItemsByIDs(ctx context.Context, ids []int64) ([]DictItem, error)
	ListReadableItemsByTypeIDs(ctx context.Context, typeIDs []int64) ([]DictItem, error)
	ShiftItemSort(ctx context.Context, typeID, targetID int64, oldOrder, newOrder int) error
}
