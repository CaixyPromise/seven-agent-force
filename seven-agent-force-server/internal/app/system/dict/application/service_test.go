package application

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	dictfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
)

func TestAddDictTypeRejectsSystemTypeForNonAdmin(t *testing.T) {
	repo := &fakeRepository{typeByCodeCount: map[string]int64{}}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService())
	one := 1

	_, err := service.AddDictType(context.Background(), Actor{UserID: 1001, IsAdmin: false}, dictfacade.DictTypeAddRequest{
		DictCode: "system_demo",
		DictName: "System Demo",
		IsSystem: &one,
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected forbidden app error, got %v", err)
	}
}

func TestAddDictTypeCanonicalizesCodeAndChecksCaseInsensitiveUniqueness(t *testing.T) {
	repo := &fakeRepository{typeByCodeCount: map[string]int64{"mixed_code": 1}}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService())

	_, err := service.AddDictType(context.Background(), Actor{UserID: 1001, IsAdmin: true}, dictfacade.DictTypeAddRequest{
		DictCode: " Mixed_Code ",
		DictName: "Mixed Code",
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected params app error for duplicate dictCode, got %v", err)
	}
	if repo.lastCountTypeByCode != "mixed_code" {
		t.Fatalf("expected canonical lowercase dictCode lookup, got %q", repo.lastCountTypeByCode)
	}
}

func TestDictTypePaginationAcceptsLegacyPageNum(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService())

	if _, err := service.GetDictTypePage(context.Background(), dictfacade.DictTypeQueryRequest{
		PageNum:  5,
		PageSize: 30,
	}); err != nil {
		t.Fatalf("get dict type page: %v", err)
	}
	if repo.lastTypeQuery.Current != 5 || repo.lastTypeQuery.PageSize != 30 {
		t.Fatalf("expected legacy pageNum to drive dict pagination, got %#v", repo.lastTypeQuery)
	}
}

func TestAddDictItemDuplicateErrorDoesNotEchoSensitiveValue(t *testing.T) {
	const canaryValue = "secret_item_value_canary"
	repo := &fakeRepository{
		typesByID: map[int64]*domain.DictType{
			11: {ID: 11, DictCode: "secret_dict", DictName: "Secret Dict", Status: 1},
		},
		itemByValueCount: 1,
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService())
	status := 1

	_, err := service.AddDictItem(context.Background(), Actor{UserID: 1001}, 11, dictfacade.DictItemAddRequest{
		ItemValue: canaryValue,
		ItemLabel: "Secret Label",
		Status:    &status,
	})
	appErr := apperrors.From(err)
	if appErr.Code() != apperrors.CodeParamsError {
		t.Fatalf("expected duplicate value params error, got %v", err)
	}
	if strings.Contains(appErr.Message(), canaryValue) {
		t.Fatalf("duplicate error echoed sensitive item value: %q", appErr.Message())
	}
}

func TestDeleteDictTypeForceSoftDeletesItemsWithoutLegacyDirectCacheInvalidation(t *testing.T) {
	repo := &fakeRepository{
		typesByID: map[int64]*domain.DictType{
			11: {
				ID:        11,
				DictCode:  "user_status",
				DictName:  "User Status",
				Status:    1,
				SortOrder: 1,
			},
		},
		itemCountByTypeID: map[int64]int64{11: 2},
	}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService())

	err := service.DeleteDictType(context.Background(), Actor{UserID: 1001, IsAdmin: true}, 11, true)
	if err != nil {
		t.Fatalf("delete dict type force: %v", err)
	}
	if repo.softDeletedTypeID != 11 {
		t.Fatalf("expected soft delete for type 11, got %d", repo.softDeletedTypeID)
	}
	if repo.updatedType == nil || repo.updatedType.IsDeleted != 1 {
		t.Fatalf("expected type marked deleted, got %#v", repo.updatedType)
	}
	if cache.bumpCount != 0 {
		t.Fatalf("legacy batch cache version must not be bumped, got %d", cache.bumpCount)
	}
	if cache.invalidatedTypeID != 0 || cache.invalidatedTypeCode != "" {
		t.Fatalf("legacy direct cache invalidation unexpectedly ran: id=%d code=%q", cache.invalidatedTypeID, cache.invalidatedTypeCode)
	}
}

func TestBatchGetDictForClientRequiresLoginForProtectedType(t *testing.T) {
	repo := &fakeRepository{
		readableTypes: []domain.DictType{
			{ID: 21, DictCode: "secure_dict", DictName: "Secure Dict", Status: 1, RequiredLogin: 1, Exposure: "AUTHENTICATED", Sensitivity: "NORMAL"},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService())

	_, err := service.BatchGetDictForClient(context.Background(), Actor{Authenticated: false}, dictfacade.DictBatchRequest{
		DictCodes: []string{"secure_dict"},
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeNotLogin {
		t.Fatalf("expected unauthorized app error, got %v", err)
	}
}

func TestGetDictByCodePreservesOriginalRequestedKey(t *testing.T) {
	repo := &fakeRepository{
		readableTypes: []domain.DictType{
			{ID: 31, DictCode: "USER_STATUS", DictName: "User Status", Status: 1, Exposure: "PUBLIC", Sensitivity: "NORMAL"},
		},
		readableItemsByTypeID: map[int64][]domain.DictItem{
			31: {
				{ID: 301, DictTypeID: 31, ItemValue: "ENABLE", ItemLabel: "启用", Status: 1, SortOrder: 1},
			},
		},
	}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService())

	result, err := service.GetDictByCodeForClient(context.Background(), Actor{}, "User_Status")
	if err != nil {
		t.Fatalf("get dict by code: %v", err)
	}
	if result == nil || len(result.Record) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	items, ok := result.Record["User_Status"]
	if !ok {
		t.Fatalf("expected original request key preserved, got %#v", result.Record)
	}
	if len(items) != 1 || items[0].ItemValue != "ENABLE" {
		t.Fatalf("unexpected items: %#v", items)
	}
	for _, required := range []string{
		"account=0",
		"scope=server:local",
		"authz=0",
		"dict=user_status",
	} {
		if !strings.Contains(cache.lastItemsByCodeKey, required) {
			t.Fatalf("expected identity-bound items-by-code cache key to contain %q, got %q", required, cache.lastItemsByCodeKey)
		}
	}
}

func TestCataloguedGenderCacheMissUsesOneTypeAndOneItemQuery(t *testing.T) {
	repo := &fakeRepository{
		readableTypes: []domain.DictType{{
			ID:       71,
			DictCode: "gender",
			DictName: "Gender",
			Status:   1,
			// The real public gender catalog is system-managed. IsSystem
			// protects mutation authority, not this PUBLIC client read.
			IsSystem:      1,
			Exposure:      "PUBLIC",
			Sensitivity:   "NORMAL",
			SchemaVersion: cachepolicy.SchemaVersionV1,
		}},
		readableItemsByTypeID: map[int64][]domain.DictItem{
			71: {{ID: 701, DictTypeID: 71, ItemValue: "M", ItemLabel: "Male", Status: 1, SortOrder: 1}},
		},
	}
	cache := &fakeCacheStore{classifiedEnabled: true}
	service := NewService(nil, repo, cache, domain.NewService())

	result, err := service.GetDictByCodeForClient(context.Background(), Actor{}, "gender")
	if err != nil {
		t.Fatalf("get catalogued gender: %v", err)
	}
	if result == nil || len(result.Record["gender"]) != 1 {
		t.Fatalf("unexpected catalogued gender response: %#v", result)
	}
	if repo.readableTypeCalls != 1 || repo.readableItemCalls != 1 {
		t.Fatalf("catalogued cache miss must use one type and one item query, got type=%d item=%d", repo.readableTypeCalls, repo.readableItemCalls)
	}
	if cache.classifiedCalls != 1 || len(cache.classifiedRequests) != 1 || cache.classifiedRequests[0].Entry.DataClass != cachepolicy.DataClassDictPublicItems {
		t.Fatalf("expected exactly one classified dictionary request, got calls=%d requests=%#v", cache.classifiedCalls, cache.classifiedRequests)
	}
}

func TestClassifiedDictCacheDoesNotBypassRequiredLogin(t *testing.T) {
	repo := &fakeRepository{
		readableTypes: []domain.DictType{{
			ID:            72,
			DictCode:      "gender",
			DictName:      "Gender",
			Status:        1,
			RequiredLogin: 1,
			Exposure:      "PUBLIC",
			Sensitivity:   "NORMAL",
			SchemaVersion: cachepolicy.SchemaVersionV1,
		}},
	}
	cache := &fakeCacheStore{
		classifiedEnabled: true,
		classifiedCachedResponse: &dictfacade.DictBatchResponse{Record: map[string][]dictfacade.DictItemVO{
			"gender": {{ItemValue: "M", ItemLabel: "must-not-leak"}},
		}},
	}
	service := NewService(nil, repo, cache, domain.NewService())

	_, err := service.GetDictByCodeForClient(context.Background(), Actor{}, "gender")
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeNotLogin {
		t.Fatalf("required-login dictionary escaped classified cache to anonymous caller: %v", err)
	}
}

func TestGetDictItemListUsesCacheWhenEligible(t *testing.T) {
	cache := &fakeCacheStore{
		itemsByTypeHit: map[int64][]domain.DictItemView{
			41: {
				{DictItem: domain.DictItem{ID: 401, DictTypeID: 41, ItemValue: "A", ItemLabel: "Alpha", Status: 1, SortOrder: 1}},
			},
		},
	}
	repo := &fakeRepository{}
	service := NewService(nil, repo, cache, domain.NewService())

	items, err := service.GetDictItemList(context.Background(), dictfacade.DictItemQueryRequest{DictTypeID: 41})
	if err != nil {
		t.Fatalf("get dict item list from cache: %v", err)
	}
	if len(items) != 1 || items[0].ItemValue != "A" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if repo.queryItemsCalls != 0 {
		t.Fatalf("expected repo query skipped on cache hit, got %d", repo.queryItemsCalls)
	}
}

func TestInternalConsumerBatchListAndCacheUseRegisteredIdentity(t *testing.T) {
	repo := &fakeRepository{
		readableTypes: []domain.DictType{
			{ID: 51, DictCode: "GENDER", DictName: "Gender", Status: 1, Exposure: "INTERNAL", Sensitivity: "NORMAL"},
		},
		readableItemsByTypeID: map[int64][]domain.DictItem{
			51: {
				{ID: 501, DictTypeID: 51, ItemValue: "M", ItemLabel: "Male", Status: 1, SortOrder: 1},
			},
		},
	}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService())
	service.BindDictConsumers([]dictfacade.DictConsumerRegistration{{
		ConsumerID:         "system.profile",
		DictCode:           "gender",
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "NORMAL",
	}})

	request := dictfacade.DictInternalBatchReadRequest{
		ConsumerID:         "system.profile",
		DictCodes:          []string{"gender"},
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "NORMAL",
	}
	result, err := service.BatchGetDictForConsumer(context.Background(), request)
	if err != nil {
		t.Fatalf("batch get registered dict: %v", err)
	}
	if len(result.Record["gender"]) != 1 {
		t.Fatalf("unexpected registered dict result: %#v", result)
	}
	for _, required := range []string{
		"internal",
		"scope=server:local",
		"consumer=system.profile",
		"purpose=render profile",
		"allowed=NORMAL",
	} {
		if !strings.Contains(cache.lastBatchKey, required) {
			t.Fatalf("expected cache key to contain %q, got %q", required, cache.lastBatchKey)
		}
	}

	listed, err := service.ListDictsForConsumer(context.Background(), dictfacade.DictInternalListRequest{
		ConsumerID:         request.ConsumerID,
		ServerScope:        request.ServerScope,
		Purpose:            request.Purpose,
		AllowedSensitivity: request.AllowedSensitivity,
	})
	if err != nil {
		t.Fatalf("list registered dicts: %v", err)
	}
	if len(listed.Record["gender"]) != 1 {
		t.Fatalf("unexpected registered dict list: %#v", listed)
	}

	request.Purpose = "export profile"
	if _, err := service.BatchGetDictForConsumer(context.Background(), request); apperrors.From(err) == nil ||
		apperrors.From(err).Code() != apperrors.CodeForbidden {
		t.Fatalf("expected mismatched purpose to be forbidden, got %v", err)
	}
}

func TestInternalConsumerCannotEscalateBeyondRegisteredSensitivity(t *testing.T) {
	repo := &fakeRepository{
		readableTypes: []domain.DictType{
			{ID: 61, DictCode: "credential_state", DictName: "Credential State", Status: 1, Exposure: "INTERNAL", Sensitivity: "SECRET"},
		},
	}
	service := NewService(nil, repo, &fakeCacheStore{}, domain.NewService())
	service.BindDictConsumers([]dictfacade.DictConsumerRegistration{{
		ConsumerID:         "system.profile",
		DictCode:           "credential_state",
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "NORMAL",
	}})

	_, err := service.GetDictForConsumer(context.Background(), dictfacade.DictInternalReadRequest{
		ConsumerID:         "system.profile",
		DictCode:           "credential_state",
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "SECRET",
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected registered sensitivity ceiling to forbid secret dict, got %v", err)
	}
}

func TestInternalConsumerRejectsUnknownStoredSensitivityBeforeCacheRead(t *testing.T) {
	repo := &fakeRepository{
		readableTypes: []domain.DictType{
			{ID: 62, DictCode: "credential_state", DictName: "Credential State", Status: 1, Exposure: "INTERNAL", Sensitivity: "CUSTOM_UNKNOWN"},
		},
	}
	cache := &fakeCacheStore{}
	service := NewService(nil, repo, cache, domain.NewService())
	service.BindDictConsumers([]dictfacade.DictConsumerRegistration{{
		ConsumerID:         "system.profile",
		DictCode:           "credential_state",
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "NORMAL",
	}})

	_, err := service.GetDictForConsumer(context.Background(), dictfacade.DictInternalReadRequest{
		ConsumerID:         "system.profile",
		DictCode:           "credential_state",
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "NORMAL",
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected unknown persisted sensitivity to fail closed, got %v", err)
	}
	if cache.getBatchCalls != 0 {
		t.Fatalf("unknown persisted sensitivity must fail before cache access, got %d cache reads", cache.getBatchCalls)
	}
}

func TestBindDictConsumersRejectsUnknownSensitivity(t *testing.T) {
	service := NewService(nil, &fakeRepository{}, &fakeCacheStore{}, domain.NewService())
	service.BindDictConsumers([]dictfacade.DictConsumerRegistration{{
		ConsumerID:         "system.profile",
		DictCode:           "gender",
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "CUSTOM",
	}})

	_, err := service.GetDictForConsumer(context.Background(), dictfacade.DictInternalReadRequest{
		ConsumerID:         "system.profile",
		DictCode:           "gender",
		ServerScope:        "server:local",
		Purpose:            "render profile",
		AllowedSensitivity: "NORMAL",
	})
	appErr := apperrors.From(err)
	if appErr == nil || appErr.Code() != apperrors.CodeForbidden {
		t.Fatalf("expected invalid registration to be skipped, got %v", err)
	}
}

func TestDictConsumerRegistryRecordsActualGenderConsumer(t *testing.T) {
	service := NewService(nil, &fakeRepository{}, &fakeCacheStore{}, domain.NewService())
	consumers := service.ListDictConsumers(context.Background())
	if len(consumers) != 1 {
		t.Fatalf("expected one reviewed dictionary consumer, got %#v", consumers)
	}
	consumer := consumers[0]
	if !consumer.Connected ||
		consumer.DictCode != "gender" ||
		consumer.Source != "sys_dict_type/sys_dict_item" ||
		consumer.ActualConsumer == "" ||
		consumer.Activation == "" ||
		consumer.CacheRule == "" {
		t.Fatalf("incomplete gender consumer registry row: %#v", consumer)
	}
}

type fakeRepository struct {
	typesByID             map[int64]*domain.DictType
	itemsByID             map[int64]*domain.DictItem
	typeByCodeCount       map[string]int64
	lastCountTypeByCode   string
	itemByValueCount      int64
	itemCountByTypeID     map[int64]int64
	readableTypes         []domain.DictType
	readableItemsByTypeID map[int64][]domain.DictItem
	updatedType           *domain.DictType
	updatedItem           *domain.DictItem
	softDeletedTypeID     int64
	queryItemsCalls       int
	readableTypeCalls     int
	readableItemCalls     int
	lastTypeQuery         domain.DictTypePageQuery
}

func (f *fakeRepository) FindTypeByID(_ context.Context, id int64) (*domain.DictType, error) {
	if f.typesByID == nil {
		return nil, nil
	}
	item := f.typesByID[id]
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (f *fakeRepository) FindTypeByCode(context.Context, string) (*domain.DictType, error) {
	return nil, nil
}

func (f *fakeRepository) CountTypeByCode(_ context.Context, dictCode string, _ int64) (int64, error) {
	f.lastCountTypeByCode = dictCode
	if f.typeByCodeCount == nil {
		return 0, nil
	}
	return f.typeByCodeCount[dictCode], nil
}

func (f *fakeRepository) InsertType(_ context.Context, item *domain.DictType) (int64, error) {
	if f.typesByID == nil {
		f.typesByID = map[int64]*domain.DictType{}
	}
	id := int64(len(f.typesByID) + 100)
	copyItem := *item
	copyItem.ID = id
	f.typesByID[id] = &copyItem
	return id, nil
}

func (f *fakeRepository) UpdateType(_ context.Context, item *domain.DictType) error {
	copyItem := *item
	f.updatedType = &copyItem
	if f.typesByID == nil {
		f.typesByID = map[int64]*domain.DictType{}
	}
	f.typesByID[item.ID] = &copyItem
	return nil
}

func (f *fakeRepository) QueryTypes(_ context.Context, query domain.DictTypePageQuery) (*domain.DictTypePage, error) {
	f.lastTypeQuery = query
	return &domain.DictTypePage{Current: query.Current, Size: query.PageSize, Total: 0, Records: []domain.DictType{}}, nil
}

func (f *fakeRepository) CountItemsByTypeID(_ context.Context, typeID int64) (int64, error) {
	if f.itemCountByTypeID == nil {
		return 0, nil
	}
	return f.itemCountByTypeID[typeID], nil
}

func (f *fakeRepository) CountItemsByTypeIDs(_ context.Context, typeIDs []int64) (map[int64]int64, error) {
	result := make(map[int64]int64, len(typeIDs))
	for _, typeID := range typeIDs {
		result[typeID] = f.itemCountByTypeID[typeID]
	}
	return result, nil
}

func (f *fakeRepository) SoftDeleteItemsByTypeID(_ context.Context, typeID, _ int64, _ time.Time) error {
	f.softDeletedTypeID = typeID
	return nil
}

func (f *fakeRepository) ShiftTypeSort(context.Context, int64, int, int) error {
	return nil
}

func (f *fakeRepository) FindReadableTypesByCodes(_ context.Context, dictCodes []string) ([]domain.DictType, error) {
	f.readableTypeCalls++
	if len(dictCodes) == 0 {
		return []domain.DictType{}, nil
	}
	return append([]domain.DictType(nil), f.readableTypes...), nil
}

func (f *fakeRepository) FindItemByID(_ context.Context, id int64) (*domain.DictItem, error) {
	if f.itemsByID == nil {
		return nil, nil
	}
	item := f.itemsByID[id]
	if item == nil {
		return nil, nil
	}
	copyItem := *item
	return &copyItem, nil
}

func (f *fakeRepository) CountItemByValue(context.Context, int64, string, int64) (int64, error) {
	return f.itemByValueCount, nil
}

func (f *fakeRepository) InsertItem(context.Context, *domain.DictItem) (int64, error) {
	return 501, nil
}

func (f *fakeRepository) UpdateItem(_ context.Context, item *domain.DictItem) error {
	copyItem := *item
	f.updatedItem = &copyItem
	return nil
}

func (f *fakeRepository) QueryItems(_ context.Context, query domain.DictItemListQuery) ([]domain.DictItem, error) {
	f.queryItemsCalls++
	return append([]domain.DictItem(nil), f.readableItemsByTypeID[query.DictTypeID]...), nil
}

func (f *fakeRepository) ListItemsByIDs(context.Context, []int64) ([]domain.DictItem, error) {
	return nil, nil
}

func (f *fakeRepository) ListReadableItemsByTypeIDs(_ context.Context, typeIDs []int64) ([]domain.DictItem, error) {
	f.readableItemCalls++
	result := make([]domain.DictItem, 0)
	for _, typeID := range typeIDs {
		result = append(result, f.readableItemsByTypeID[typeID]...)
	}
	return result, nil
}

func (f *fakeRepository) ShiftItemSort(context.Context, int64, int64, int, int) error {
	return nil
}

type fakeCacheStore struct {
	itemsByTypeHit           map[int64][]domain.DictItemView
	lastItemsByCodeKey       string
	lastBatchKey             string
	getBatchCalls            int
	invalidatedTypeID        int64
	invalidatedTypeCode      string
	bumpCount                int
	classifiedEnabled        bool
	classifiedCalls          int
	classifiedRequests       []cachepolicy.ReadRequest
	classifiedCachedResponse *dictfacade.DictBatchResponse
}

func (f *fakeCacheStore) GetTypeByID(context.Context, int64) (*domain.DictType, bool, error) {
	return nil, false, nil
}

func (f *fakeCacheStore) SetTypeByID(context.Context, *domain.DictType) error {
	return nil
}

func (f *fakeCacheStore) GetTypeByCode(context.Context, string) (*domain.DictType, bool, error) {
	return nil, false, nil
}

func (f *fakeCacheStore) SetTypeByCode(context.Context, string, *domain.DictType) error {
	return nil
}

func (f *fakeCacheStore) GetItemsByType(_ context.Context, typeID int64) ([]domain.DictItemView, bool, error) {
	items, ok := f.itemsByTypeHit[typeID]
	return append([]domain.DictItemView(nil), items...), ok, nil
}

func (f *fakeCacheStore) SetItemsByType(context.Context, int64, []domain.DictItemView) error {
	return nil
}

func (f *fakeCacheStore) GetItemsByCode(context.Context, string) ([]domain.DictItemView, bool, error) {
	return nil, false, nil
}

func (f *fakeCacheStore) SetItemsByCode(_ context.Context, dictCode string, _ []domain.DictItemView) error {
	f.lastItemsByCodeKey = dictCode
	return nil
}

func (f *fakeCacheStore) GetBatch(_ context.Context, cacheKey string) (*domain.BatchResult, bool, error) {
	f.getBatchCalls++
	f.lastBatchKey = cacheKey
	return nil, false, nil
}

func (f *fakeCacheStore) SetBatch(_ context.Context, cacheKey string, _ *domain.BatchResult) error {
	f.lastBatchKey = cacheKey
	return nil
}

func (f *fakeCacheStore) ClassifiedEnabled() bool {
	return f.classifiedEnabled
}

func (f *fakeCacheStore) GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error) {
	f.classifiedCalls++
	f.classifiedRequests = append(f.classifiedRequests, request)
	if f.classifiedCachedResponse != nil {
		target, ok := dest.(*dictfacade.DictBatchResponse)
		if !ok {
			return false, nil
		}
		*target = *f.classifiedCachedResponse
		return true, nil
	}
	value, err := loader(ctx)
	if err != nil || value.Value == nil {
		return false, err
	}
	result, ok := value.Value.(*dictfacade.DictBatchResponse)
	if !ok {
		return false, nil
	}
	target, ok := dest.(*dictfacade.DictBatchResponse)
	if !ok {
		return false, nil
	}
	*target = *result
	return true, nil
}

func (f *fakeCacheStore) GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight func(context.Context) (bool, error), loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error) {
	f.classifiedCalls++
	f.classifiedRequests = append(f.classifiedRequests, request)
	allowed, err := preflight(ctx)
	if err != nil {
		return false, err
	}
	if allowed && f.classifiedCachedResponse != nil {
		target, ok := dest.(*classifiedDictCacheRecord)
		if !ok {
			return false, nil
		}
		*target = classifiedDictCacheRecord{Response: *f.classifiedCachedResponse, CataloguedPublic: true}
		return true, nil
	}
	value, err := loader(ctx)
	if err != nil || value.Value == nil {
		return false, err
	}
	record, ok := value.Value.(classifiedDictCacheRecord)
	if !ok {
		return false, nil
	}
	target, ok := dest.(*classifiedDictCacheRecord)
	if !ok {
		return false, nil
	}
	*target = record
	return true, nil
}

func (f *fakeCacheStore) CurrentBatchVersion(context.Context) (int64, error) {
	return 0, nil
}

func (f *fakeCacheStore) BumpBatchVersion(context.Context) error {
	f.bumpCount++
	return nil
}

func (f *fakeCacheStore) InvalidateType(_ context.Context, typeID int64, dictCode string) error {
	f.invalidatedTypeID = typeID
	f.invalidatedTypeCode = dictCode
	return nil
}

func (f *fakeCacheStore) InvalidateItems(context.Context, int64, string) error {
	return nil
}
