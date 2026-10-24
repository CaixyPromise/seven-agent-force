package application

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/domain"
	dictfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/dict/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/bytedance/sonic"
)

type Actor struct {
	UserID        int64
	IsAdmin       bool
	Authenticated bool
	AccountID     int64
	ScopeID       string
	AuthzVersion  int64
	ConsumerID    string
	Purpose       string
	AllowedSecret string
}

type cacheStore interface {
	GetTypeByID(ctx context.Context, typeID int64) (*domain.DictType, bool, error)
	SetTypeByID(ctx context.Context, item *domain.DictType) error
	GetTypeByCode(ctx context.Context, dictCode string) (*domain.DictType, bool, error)
	SetTypeByCode(ctx context.Context, dictCode string, item *domain.DictType) error
	GetItemsByType(ctx context.Context, typeID int64) ([]domain.DictItemView, bool, error)
	SetItemsByType(ctx context.Context, typeID int64, items []domain.DictItemView) error
	GetItemsByCode(ctx context.Context, dictCode string) ([]domain.DictItemView, bool, error)
	SetItemsByCode(ctx context.Context, dictCode string, items []domain.DictItemView) error
	GetBatch(ctx context.Context, cacheKey string) (*domain.BatchResult, bool, error)
	SetBatch(ctx context.Context, cacheKey string, result *domain.BatchResult) error
	CurrentBatchVersion(ctx context.Context) (int64, error)
	BumpBatchVersion(ctx context.Context) error
	InvalidateType(ctx context.Context, typeID int64, dictCode string) error
	InvalidateItems(ctx context.Context, typeID int64, dictCode string) error
}

type classifiedCacheStore interface {
	ClassifiedEnabled() bool
	GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error)
}

// classifiedPreflightCacheStore is intentionally narrower than a generic
// cache API. It lets an application retain source-side authorization and
// catalog checks before a governed candidate can be returned, without giving
// cache infrastructure any domain model or policy dependency.
type classifiedPreflightCacheStore interface {
	classifiedCacheStore
	GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight func(context.Context) (bool, error), loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error)
}

// classifiedDictCacheRecord prevents a legacy or non-catalogued DTO payload
// from gaining DG5 admission after an upgrade. The value carries only the
// already-authorized client DTO; CataloguedPublic is set exclusively from the
// current source row under the governed freshness lease.
type classifiedDictCacheRecord struct {
	Response         dictfacade.DictBatchResponse `json:"response"`
	CataloguedPublic bool                         `json:"cataloguedPublic"`
}

type transactor interface {
	Enabled() bool
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type dictItemSortBatchRepository interface {
	UpdateItemSorts(ctx context.Context, items []domain.DictItem) error
}

type Service struct {
	transactor              transactor
	repo                    domain.Repository
	cache                   cacheStore
	domain                  *domain.Service
	invalidations           cachegovernancefacade.InvalidationRegistrar
	consumers               map[string]dictfacade.DictConsumerRegistration
	consumerRegistryVersion int64
}

func NewService(transactor transactor, repo domain.Repository, cache cacheStore, domainService *domain.Service, invalidations ...cachegovernancefacade.InvalidationRegistrar) *Service {
	var registrar cachegovernancefacade.InvalidationRegistrar
	if len(invalidations) > 0 {
		registrar = invalidations[0]
	}
	return &Service{
		transactor:    transactor,
		repo:          repo,
		cache:         cache,
		domain:        domainService,
		invalidations: registrar,
		consumers:     map[string]dictfacade.DictConsumerRegistration{},
	}
}

func (s *Service) BindDictConsumers(registrations []dictfacade.DictConsumerRegistration) {
	if s == nil {
		return
	}
	s.consumers = make(map[string]dictfacade.DictConsumerRegistration, len(registrations))
	s.consumerRegistryVersion++
	for _, registration := range registrations {
		registration.ConsumerID = strings.TrimSpace(registration.ConsumerID)
		registration.DictCode = s.domain.NormalizeDictCode(registration.DictCode)
		registration.ServerScope = strings.TrimSpace(registration.ServerScope)
		registration.Purpose = strings.TrimSpace(registration.Purpose)
		registration.AllowedSensitivity = normalizeDictSensitivity(registration.AllowedSensitivity)
		if registration.ConsumerID == "" ||
			registration.DictCode == "" ||
			registration.ServerScope == "" ||
			registration.Purpose == "" ||
			!isSupportedDictSensitivity(registration.AllowedSensitivity) {
			continue
		}
		s.consumers[registration.ConsumerID+"\x00"+registration.DictCode] = registration
	}
}

func (s *Service) AddDictType(ctx context.Context, actor Actor, request dictfacade.DictTypeAddRequest) (int64, error) {
	if actor.UserID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	validationJSON, err := marshalValidation(request.Validation)
	if err != nil {
		return 0, err
	}
	item, err := s.domain.NewDictType(actor.UserID, domain.CreateDictTypeInput{
		DictCode:       request.DictCode,
		DictName:       request.DictName,
		DictDesc:       request.DictDesc,
		Module:         request.Module,
		Status:         request.Status,
		IsSystem:       request.IsSystem,
		RequiredLogin:  request.RequiredLogin,
		ValueType:      request.ValueType,
		UIWidget:       request.UIWidget,
		ValidationJSON: validationJSON,
		Exposure:       request.Exposure,
		Sensitivity:    request.Sensitivity,
		SchemaVersion:  request.SchemaVersion,
	}, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	if err := s.domain.EnsureSystemTypeAllowed(item.IsSystem, actor.IsAdmin); err != nil {
		return 0, err
	}
	count, err := s.repo.CountTypeByCode(ctx, item.DictCode, 0)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, apperrors.Params("字典类型编码已存在：" + item.DictCode)
	}
	var id int64
	err = s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		var createErr error
		id, createErr = s.repo.InsertType(txCtx, item)
		return createErr
	})
	if err != nil {
		return 0, err
	}
	item.ID = id
	return id, nil
}

func (s *Service) UpdateDictType(ctx context.Context, actor Actor, request dictfacade.DictTypeUpdateRequest) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindTypeByID(ctx, request.ID)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("字典类型不存在")
	}
	if request.Version != nil && *request.Version != item.Version {
		return apperrors.Params("字典版本已变化，请刷新后重试")
	}
	var validationJSON *string
	if request.Validation != nil {
		raw, marshalErr := marshalValidation(request.Validation)
		if marshalErr != nil {
			return marshalErr
		}
		validationJSON = &raw
	}
	if err := s.domain.ApplyDictTypeUpdate(item, actor.UserID, domain.UpdateDictTypeInput{
		DictName:       request.DictName,
		DictDesc:       request.DictDesc,
		Module:         request.Module,
		Status:         request.Status,
		SortOrder:      request.SortOrder,
		RequiredLogin:  request.RequiredLogin,
		ValueType:      request.ValueType,
		UIWidget:       request.UIWidget,
		ValidationJSON: validationJSON,
		Exposure:       request.Exposure,
		Sensitivity:    request.Sensitivity,
		SchemaVersion:  request.SchemaVersion,
	}, time.Now().UTC()); err != nil {
		return err
	}
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateType(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteDictType(ctx context.Context, actor Actor, id int64, force bool) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindTypeByID(ctx, id)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("字典类型不存在")
	}
	itemCount, err := s.repo.CountItemsByTypeID(ctx, id)
	if err != nil {
		return err
	}
	if itemCount > 0 && !force {
		return apperrors.Operation("字典类型下存在 " + strconv.FormatInt(itemCount, 10) + " 个字典项，无法删除。如需强制删除，请使用 force=true 参数")
	}
	now := time.Now().UTC()
	if err := s.domain.MarkDictTypeDeleted(item, actor.UserID, now); err != nil {
		return err
	}
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		if force && itemCount > 0 {
			if err := s.repo.SoftDeleteItemsByTypeID(txCtx, id, actor.UserID, now); err != nil {
				return err
			}
		}
		return s.repo.UpdateType(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetDictTypeByID(ctx context.Context, id int64) (*dictfacade.DictTypeVO, error) {
	if id <= 0 {
		return nil, apperrors.Params("id不能为空")
	}
	item, err := s.loadTypeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.IsDeleted == 1 {
		return nil, apperrors.NotFound("字典类型不存在")
	}
	itemCount, err := s.repo.CountItemsByTypeID(ctx, id)
	if err != nil {
		return nil, err
	}
	item.ItemCount = itemCount
	_ = s.cache.SetTypeByID(ctx, item)
	return toDictTypeVO(item), nil
}

func (s *Service) GetDictTypePage(ctx context.Context, request dictfacade.DictTypeQueryRequest) (*dictfacade.PageResult[dictfacade.DictTypeVO], error) {
	current, size := s.domain.NormalizePage(firstPositiveInt64(request.Current, request.PageNum), request.PageSize)
	page, err := s.repo.QueryTypes(ctx, domain.DictTypePageQuery{
		Current:  current,
		PageSize: size,
		Keyword:  strings.TrimSpace(request.Keyword),
		Module:   strings.TrimSpace(request.Module),
		Status:   request.Status,
	})
	if err != nil {
		return nil, err
	}
	records := make([]dictfacade.DictTypeVO, 0, len(page.Records))
	for _, item := range page.Records {
		copyItem := item
		records = append(records, *toDictTypeVO(&copyItem))
	}
	return &dictfacade.PageResult[dictfacade.DictTypeVO]{
		Current: page.Current,
		Size:    page.Size,
		Total:   page.Total,
		Records: records,
	}, nil
}

func (s *Service) ChangeDictTypeStatus(ctx context.Context, actor Actor, id int64, status int) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindTypeByID(ctx, id)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("字典类型不存在")
	}
	if err := s.domain.ChangeDictTypeStatus(item, actor.UserID, status, time.Now().UTC()); err != nil {
		return err
	}
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateType(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) MoveDictType(ctx context.Context, actor Actor, id int64, beforeID, afterID *int64) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	target, err := s.repo.FindTypeByID(ctx, id)
	if err != nil {
		return err
	}
	if target == nil || target.IsDeleted == 1 {
		return apperrors.NotFound("字典类型不存在")
	}
	oldOrder := target.SortOrder
	newOrder, err := s.resolveTypeMoveOrder(ctx, id, beforeID, afterID)
	if err != nil {
		return err
	}
	if oldOrder == newOrder {
		return nil
	}
	now := time.Now().UTC()
	target.SortOrder = newOrder
	target.UpdatedBy = actor.UserID
	target.UpdateTime = &now
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.ShiftTypeSort(txCtx, id, oldOrder, newOrder); err != nil {
			return err
		}
		return s.repo.UpdateType(txCtx, target)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) AddDictItem(ctx context.Context, actor Actor, typeID int64, request dictfacade.DictItemAddRequest) (int64, error) {
	if actor.UserID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	dictType, err := s.repo.FindTypeByID(ctx, typeID)
	if err != nil {
		return 0, err
	}
	if dictType == nil || dictType.IsDeleted == 1 {
		return 0, apperrors.NotFound("字典类型不存在")
	}
	if dictType.Status != 1 {
		return 0, apperrors.Operation("字典类型已禁用，无法添加字典项")
	}
	item, err := s.domain.NewDictItem(actor.UserID, typeID, domain.CreateDictItemInput{
		ItemValue:  request.ItemValue,
		ItemLabel:  request.ItemLabel,
		ItemDesc:   request.ItemDesc,
		SortOrder:  request.SortOrder,
		Status:     request.Status,
		ExtJSON:    request.ExtJSON,
		ColorToken: request.ColorToken,
		IconToken:  request.IconToken,
	}, time.Now().UTC())
	if err != nil {
		return 0, err
	}
	count, err := s.repo.CountItemByValue(ctx, typeID, item.ItemValue, 0)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, apperrors.Params("字典值已存在")
	}
	var id int64
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		var createErr error
		id, createErr = s.repo.InsertItem(txCtx, item)
		return createErr
	}); err != nil {
		return 0, err
	}
	item.ID = id
	return id, nil
}

func (s *Service) UpdateDictItem(ctx context.Context, actor Actor, request dictfacade.DictItemUpdateRequest) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindItemByID(ctx, request.ID)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("字典项不存在")
	}
	if request.Version != nil && *request.Version != item.Version {
		return apperrors.Params("字典项版本已变化，请刷新后重试")
	}
	if err := s.domain.ApplyDictItemUpdate(item, actor.UserID, domain.UpdateDictItemInput{
		ItemLabel:  request.ItemLabel,
		ItemDesc:   request.ItemDesc,
		SortOrder:  request.SortOrder,
		Status:     request.Status,
		ExtJSON:    request.ExtJSON,
		ColorToken: request.ColorToken,
		IconToken:  request.IconToken,
	}, time.Now().UTC()); err != nil {
		return err
	}
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateItem(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteDictItem(ctx context.Context, actor Actor, id int64) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindItemByID(ctx, id)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("字典项不存在")
	}
	if err := s.domain.MarkDictItemDeleted(item, actor.UserID, time.Now().UTC()); err != nil {
		return err
	}
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateItem(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) ChangeDictItemStatus(ctx context.Context, actor Actor, id int64, status int) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindItemByID(ctx, id)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("字典项不存在")
	}
	if err := s.domain.ChangeDictItemStatus(item, actor.UserID, status, time.Now().UTC()); err != nil {
		return err
	}
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateItem(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetDictItemList(ctx context.Context, request dictfacade.DictItemQueryRequest) ([]dictfacade.DictItemVO, error) {
	if request.DictTypeID <= 0 {
		return nil, apperrors.Params("dictTypeId不能为空")
	}
	if !request.Force && request.Status == nil && strings.TrimSpace(request.Keyword) == "" {
		if cached, hit, err := s.cache.GetItemsByType(ctx, request.DictTypeID); err == nil && hit {
			return toDictItemVOList(cached), nil
		} else if err != nil {
			return nil, err
		}
	}
	items, err := s.repo.QueryItems(ctx, domain.DictItemListQuery{
		DictTypeID: request.DictTypeID,
		Status:     request.Status,
		Keyword:    strings.TrimSpace(request.Keyword),
	})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []dictfacade.DictItemVO{}, nil
	}
	dictType, err := s.repo.FindTypeByID(ctx, request.DictTypeID)
	if err != nil {
		return nil, err
	}
	views := buildItemViews(items, dictType)
	if !request.Force && request.Status == nil && strings.TrimSpace(request.Keyword) == "" {
		_ = s.cache.SetItemsByType(ctx, request.DictTypeID, views)
	}
	return toDictItemVOList(views), nil
}

func (s *Service) BatchUpdateSort(ctx context.Context, actor Actor, typeID int64, request dictfacade.DictItemSortRequest) (int, error) {
	if actor.UserID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	if len(request.Items) == 0 {
		return 0, apperrors.Params("字典项排序列表不能为空")
	}
	if len(request.Items) > 100 {
		return 0, apperrors.Params("字典项排序数量超过单次批量上限")
	}
	dictType, err := s.repo.FindTypeByID(ctx, typeID)
	if err != nil {
		return 0, err
	}
	if dictType == nil || dictType.IsDeleted == 1 {
		return 0, apperrors.NotFound("字典类型不存在")
	}
	ids := make([]int64, 0, len(request.Items))
	sortOrders := make(map[int64]int, len(request.Items))
	for _, item := range request.Items {
		ids = append(ids, item.ID)
		sortOrders[item.ID] = item.SortOrder
	}
	items, err := s.repo.ListItemsByIDs(ctx, ids)
	if err != nil {
		return 0, err
	}
	if len(items) != len(ids) {
		return 0, apperrors.Params("部分字典项不存在")
	}
	now := time.Now().UTC()
	for idx := range items {
		item := &items[idx]
		if item.IsDeleted == 1 || item.DictTypeID != typeID {
			return 0, apperrors.Params("部分字典项不属于指定类型或已删除")
		}
		order, ok := sortOrders[item.ID]
		if !ok {
			continue
		}
		item.SortOrder = order
		item.UpdatedBy = actor.UserID
		item.UpdateTime = &now
	}
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		batchRepo, ok := s.repo.(dictItemSortBatchRepository)
		if !ok {
			return apperrors.System("字典项批量排序仓储能力未配置")
		}
		return batchRepo.UpdateItemSorts(txCtx, items)
	}); err != nil {
		return 0, err
	}
	return len(items), nil
}

func (s *Service) MoveDictItem(ctx context.Context, actor Actor, typeID, itemID int64, beforeID, afterID *int64) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	dictType, err := s.repo.FindTypeByID(ctx, typeID)
	if err != nil {
		return err
	}
	if dictType == nil || dictType.IsDeleted == 1 {
		return apperrors.NotFound("字典类型不存在")
	}
	target, err := s.repo.FindItemByID(ctx, itemID)
	if err != nil {
		return err
	}
	if target == nil || target.IsDeleted == 1 {
		return apperrors.NotFound("字典项不存在")
	}
	if target.DictTypeID != typeID {
		return apperrors.Params("字典项不属于该字典类型")
	}
	oldOrder := target.SortOrder
	newOrder, err := s.resolveItemMoveOrder(ctx, typeID, itemID, beforeID, afterID)
	if err != nil {
		return err
	}
	if oldOrder == newOrder {
		return nil
	}
	now := time.Now().UTC()
	target.SortOrder = newOrder
	target.UpdatedBy = actor.UserID
	target.UpdateTime = &now
	if err := s.withDictInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.ShiftItemSort(txCtx, typeID, itemID, oldOrder, newOrder); err != nil {
			return err
		}
		return s.repo.UpdateItem(txCtx, target)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) BatchGetDict(ctx context.Context, request dictfacade.DictBatchRequest) (*dictfacade.DictBatchResponse, error) {
	return nil, apperrors.Forbidden("内部字典读取必须通过显式消费者注册")
}

func (s *Service) GetDictByCode(ctx context.Context, dictCode string) (*dictfacade.DictBatchResponse, error) {
	return nil, apperrors.Forbidden("内部字典读取必须通过显式消费者注册")
}

func (s *Service) GetDictForConsumer(ctx context.Context, request dictfacade.DictInternalReadRequest) (*dictfacade.DictBatchResponse, error) {
	return s.BatchGetDictForConsumer(ctx, dictfacade.DictInternalBatchReadRequest{
		ConsumerID:         request.ConsumerID,
		DictCodes:          []string{request.DictCode},
		ServerScope:        request.ServerScope,
		Purpose:            request.Purpose,
		AllowedSensitivity: request.AllowedSensitivity,
	})
}

func (s *Service) BatchGetDictForConsumer(ctx context.Context, request dictfacade.DictInternalBatchReadRequest) (*dictfacade.DictBatchResponse, error) {
	consumerID, serverScope, purpose, allowed, err := normalizeDictInternalIdentity(
		request.ConsumerID,
		request.ServerScope,
		request.Purpose,
		request.AllowedSensitivity,
	)
	if err != nil {
		return nil, err
	}
	codes := s.domain.CanonicalBatchCodes(request.DictCodes)
	if len(codes) == 0 || len(codes) > 30 {
		return nil, apperrors.Params("内部字典批量读取必须包含1到30个字典编码")
	}
	registrations := make(map[string]dictfacade.DictConsumerRegistration, len(codes))
	for _, code := range codes {
		canonicalCode := s.domain.NormalizeDictCode(code)
		registration, ok := s.consumers[consumerID+"\x00"+canonicalCode]
		if !ok ||
			registration.ServerScope != serverScope ||
			registration.Purpose != purpose ||
			dictSensitivityRank(registration.AllowedSensitivity) > dictSensitivityRank(allowed) {
			return nil, apperrors.Forbidden("字典消费者声明与注册策略不匹配")
		}
		registrations[canonicalCode] = registration
	}
	typeByCode := make(map[string]domain.DictType)
	variantSet := make(map[string]struct{}, len(codes)*3)
	variants := make([]string, 0, len(codes)*3)
	for _, code := range codes {
		for _, variant := range s.domain.BuildDictCodeVariants(code) {
			if _, ok := variantSet[variant]; ok {
				continue
			}
			variantSet[variant] = struct{}{}
			variants = append(variants, variant)
		}
	}
	types, err := s.repo.FindReadableTypesByCodes(ctx, variants)
	if err != nil {
		return nil, err
	}
	for _, dictType := range types {
		typeByCode[s.domain.NormalizeDictCode(dictType.DictCode)] = dictType
	}
	for _, code := range codes {
		dictType, ok := typeByCode[s.domain.NormalizeDictCode(code)]
		if !ok {
			continue
		}
		actualSensitivity := normalizeDictSensitivity(dictType.Sensitivity)
		if !isSupportedDictSensitivity(actualSensitivity) {
			return nil, apperrors.Forbidden("字典敏感级别无效")
		}
		registration := registrations[s.domain.NormalizeDictCode(code)]
		if dictSensitivityRank(actualSensitivity) > dictSensitivityRank(allowed) ||
			dictSensitivityRank(actualSensitivity) > dictSensitivityRank(registration.AllowedSensitivity) {
			return nil, apperrors.Forbidden("字典敏感级别超过消费者允许范围")
		}
	}
	result, err := s.batchGetDictInternal(ctx, dictfacade.DictBatchRequest{DictCodes: codes}, true, Actor{
		ScopeID:       serverScope,
		AuthzVersion:  s.consumerRegistryVersion,
		ConsumerID:    consumerID,
		Purpose:       purpose,
		AllowedSecret: allowed,
	})
	if err != nil {
		return nil, err
	}
	return toBatchResponse(result), nil
}

func (s *Service) ListDictsForConsumer(ctx context.Context, request dictfacade.DictInternalListRequest) (*dictfacade.DictBatchResponse, error) {
	consumerID, serverScope, purpose, allowed, err := normalizeDictInternalIdentity(
		request.ConsumerID,
		request.ServerScope,
		request.Purpose,
		request.AllowedSensitivity,
	)
	if err != nil {
		return nil, err
	}
	codes := make([]string, 0)
	for _, registration := range s.consumers {
		if registration.ConsumerID == consumerID &&
			registration.ServerScope == serverScope &&
			registration.Purpose == purpose &&
			dictSensitivityRank(registration.AllowedSensitivity) <= dictSensitivityRank(allowed) {
			codes = append(codes, registration.DictCode)
		}
	}
	sort.Strings(codes)
	if len(codes) == 0 {
		return nil, apperrors.Forbidden("字典消费者未注册")
	}
	return s.BatchGetDictForConsumer(ctx, dictfacade.DictInternalBatchReadRequest{
		ConsumerID:         consumerID,
		DictCodes:          codes,
		ServerScope:        serverScope,
		Purpose:            purpose,
		AllowedSensitivity: allowed,
	})
}

func (s *Service) ListDictConsumers(context.Context) []dictfacade.DictConsumerVO {
	return []dictfacade.DictConsumerVO{{
		DictConsumerRegistration: dictfacade.DictConsumerRegistration{
			ConsumerID:         "frontend.user-gender",
			DictCode:           "gender",
			ServerScope:        "browser",
			Purpose:            "render and edit user gender",
			AllowedSensitivity: "NORMAL",
			Source:             "sys_dict_type/sys_dict_item",
			ActualConsumer:     "UserSexBadge and user create/edit/detail pages",
			Activation:         "refresh",
			CacheRule:          "account/scope/authz generation",
		},
		Connected: true,
	}}
}

func (s *Service) BatchGetDictForClient(ctx context.Context, actor Actor, request dictfacade.DictBatchRequest) (*dictfacade.DictBatchResponse, error) {
	result, err := s.batchGetDictInternal(ctx, request, false, actor)
	if err != nil {
		return nil, err
	}
	return toBatchResponse(result), nil
}

func (s *Service) GetDictByCodeForClient(ctx context.Context, actor Actor, dictCode string) (*dictfacade.DictBatchResponse, error) {
	dictCode = strings.TrimSpace(dictCode)
	if classified, ok := s.cache.(classifiedPreflightCacheStore); ok && classified.ClassifiedEnabled() && dictCode == s.domain.NormalizeDictCode(dictCode) {
		if request, eligible := cachepolicy.DictReadRequest(dictCode, dictCacheRequestScope(actor), dictCacheBusinessIdentity(actor)); eligible {
			// Do not cache domain.BatchResult directly: DictItemView embeds
			// DictItem, and a cache codec must not depend on embedding-specific
			// decode behavior. The facade DTO has an explicit, stable public
			// JSON shape and is the exact value this read is authorised to return.
			var cached classifiedDictCacheRecord
			var resolvedType *domain.DictType
			found, err := classified.GetOrLoadClassifiedWithPreflight(ctx, request, &cached, func(preflightCtx context.Context) (bool, error) {
				typeItem, loadErr := s.loadCataloguedDictTypeForClient(preflightCtx, actor, dictCode)
				if loadErr != nil {
					return false, loadErr
				}
				resolvedType = typeItem
				return classifiedPublicDictEligible(typeItem), nil
			}, func(loadCtx context.Context) (cachepolicy.CacheableValue, error) {
				typeItem := resolvedType
				if typeItem == nil {
					var loadErr error
					typeItem, loadErr = s.loadCataloguedDictTypeForClient(loadCtx, actor, dictCode)
					if loadErr != nil {
						return cachepolicy.CacheableValue{}, loadErr
					}
				}
				result, loadErr := s.loadCataloguedDictItemsByType(loadCtx, dictCode, typeItem)
				if loadErr != nil || result == nil {
					return cachepolicy.CacheableValue{}, loadErr
				}
				record := classifiedDictCacheRecord{
					Response:         *toBatchResponse(result),
					CataloguedPublic: classifiedPublicDictEligible(typeItem),
				}
				cacheable := record.CataloguedPublic && typeItem != nil && cachepolicy.ValidateLoaded(request, typeItem.Exposure, typeItem.Sensitivity, typeItem.SchemaVersion,
					typeItem.Status == 1 && typeItem.IsDeleted == 0)
				return cachepolicy.CacheableValue{Value: record, Cacheable: cacheable}, nil
			})
			if err != nil {
				return nil, err
			}
			if found && cached.CataloguedPublic {
				response := cached.Response
				return &response, nil
			}
		}
	}
	result, err := s.getDictByCodeInternal(ctx, dictCode, false, actor)
	if err != nil {
		return nil, err
	}
	return toBatchResponse(result), nil
}

// loadCataloguedDictByCode is the bounded authoritative single-code path for
// the DG5 allowlist. It deliberately uses exactly one type query and one
// batched item query, so a classified cache miss cannot add a second lookup or
// reintroduce N+1 behavior. Its client-access gate matches batch reads.
func (s *Service) loadCataloguedDictByCode(ctx context.Context, actor Actor, dictCode string) (*domain.BatchResult, *domain.DictType, error) {
	typeItem, err := s.loadCataloguedDictTypeForClient(ctx, actor, dictCode)
	if err != nil {
		return nil, nil, err
	}
	result, err := s.loadCataloguedDictItemsByType(ctx, dictCode, typeItem)
	if err != nil {
		return nil, typeItem, err
	}
	return result, typeItem, nil
}

func (s *Service) loadCataloguedDictTypeForClient(ctx context.Context, actor Actor, dictCode string) (*domain.DictType, error) {
	normalized := s.domain.NormalizeDictCode(dictCode)
	if normalized == "" {
		return nil, nil
	}
	types, err := s.repo.FindReadableTypesByCodes(ctx, s.domain.BuildDictCodeVariants(normalized))
	if err != nil {
		return nil, err
	}
	if err := s.enforceClientReadableDictTypes(actor, types); err != nil {
		return nil, err
	}
	for _, item := range types {
		if s.domain.NormalizeDictCode(item.DictCode) == normalized {
			copyItem := item
			return &copyItem, nil
		}
	}
	return nil, nil
}

func (s *Service) loadCataloguedDictItemsByType(ctx context.Context, dictCode string, typeItem *domain.DictType) (*domain.BatchResult, error) {
	if typeItem == nil {
		return &domain.BatchResult{Record: map[string][]domain.DictItemView{}, Missing: []string{dictCode}}, nil
	}
	items, err := s.repo.ListReadableItemsByTypeIDs(ctx, []int64{typeItem.ID})
	if err != nil {
		return nil, err
	}
	views := buildItemViews(items, typeItem)
	return &domain.BatchResult{
		Record:  map[string][]domain.DictItemView{dictCode: views},
		Missing: []string{},
	}, nil
}

func classifiedPublicDictEligible(item *domain.DictType) bool {
	// IsSystem constrains management mutation authority, not the established
	// client-read exposure. The seeded public `gender` dictionary is system
	// managed, so public cache eligibility follows the actual read policy:
	// active, non-deleted, no required login, plus ValidateLoaded's PUBLIC /
	// NORMAL / schema checks at admission.
	return item != nil && item.Status == 1 && item.IsDeleted == 0 && item.RequiredLogin == 0
}

func dictCacheRequestScope(actor Actor) string {
	if scope := strings.TrimSpace(actor.ScopeID); scope != "" {
		return "client:" + scope
	}
	return "public:global"
}

func dictCacheBusinessIdentity(actor Actor) string {
	if !actor.Authenticated {
		return "anonymous"
	}
	return "account:" + strconv.FormatInt(actor.AccountID, 10) + ":authz:" + strconv.FormatInt(actor.AuthzVersion, 10)
}

func (s *Service) batchGetDictInternal(ctx context.Context, request dictfacade.DictBatchRequest, internalAccess bool, actor Actor) (*domain.BatchResult, error) {
	dictCodes := s.domain.CanonicalBatchCodes(request.DictCodes)
	if len(dictCodes) == 0 {
		return &domain.BatchResult{Record: map[string][]domain.DictItemView{}, Missing: []string{}}, nil
	}

	if !request.Force {
		batchKey, err := s.policyBatchCacheKey(ctx, dictCodes, internalAccess, actor)
		if err != nil {
			return nil, err
		}
		if cached, hit, err := s.cache.GetBatch(ctx, batchKey); err == nil && hit {
			return cached, nil
		} else if err != nil {
			return nil, err
		}
	}

	variantSet := make(map[string]struct{}, len(dictCodes)*3)
	variants := make([]string, 0, len(dictCodes)*3)
	for _, code := range dictCodes {
		for _, item := range s.domain.BuildDictCodeVariants(code) {
			if _, ok := variantSet[item]; ok {
				continue
			}
			variantSet[item] = struct{}{}
			variants = append(variants, item)
		}
	}

	types, err := s.repo.FindReadableTypesByCodes(ctx, variants)
	if err != nil {
		return nil, err
	}
	if !internalAccess {
		if err := s.enforceClientReadableDictTypes(actor, types); err != nil {
			return nil, err
		}
	}

	typeMap := make(map[string]domain.DictType, len(types))
	for _, item := range types {
		typeMap[s.domain.NormalizeDictCode(item.DictCode)] = item
	}

	requestedMap := make(map[string]domain.DictType, len(dictCodes))
	missing := make([]string, 0)
	for _, code := range dictCodes {
		matched, ok := typeMap[s.domain.NormalizeDictCode(code)]
		if !ok {
			missing = append(missing, code)
			continue
		}
		requestedMap[code] = matched
	}
	if len(requestedMap) == 0 {
		return &domain.BatchResult{Record: map[string][]domain.DictItemView{}, Missing: missing}, nil
	}

	typeIDs := make([]int64, 0, len(requestedMap))
	seenTypeID := map[int64]struct{}{}
	for _, dictType := range requestedMap {
		if _, ok := seenTypeID[dictType.ID]; ok {
			continue
		}
		seenTypeID[dictType.ID] = struct{}{}
		typeIDs = append(typeIDs, dictType.ID)
	}
	items, err := s.repo.ListReadableItemsByTypeIDs(ctx, typeIDs)
	if err != nil {
		return nil, err
	}
	itemMap := make(map[int64][]domain.DictItem, len(typeIDs))
	for _, item := range items {
		itemMap[item.DictTypeID] = append(itemMap[item.DictTypeID], item)
	}

	record := make(map[string][]domain.DictItemView, len(requestedMap))
	for requestedCode, dictType := range requestedMap {
		views := buildItemViews(itemMap[dictType.ID], &dictType)
		record[requestedCode] = views
	}
	result := &domain.BatchResult{
		Record:  record,
		Missing: missing,
	}
	if !request.Force && len(record) > 0 {
		batchKey, err := s.policyBatchCacheKey(ctx, dictCodes, internalAccess, actor)
		if err != nil {
			return nil, err
		}
		if err := s.cache.SetBatch(ctx, batchKey, result); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Service) enforceClientReadableDictTypes(actor Actor, types []domain.DictType) error {
	for _, dictType := range types {
		exposure := strings.ToUpper(strings.TrimSpace(dictType.Exposure))
		if exposure == "" {
			exposure = "INTERNAL"
		}
		sensitivity := strings.ToUpper(strings.TrimSpace(dictType.Sensitivity))
		if sensitivity == "" {
			sensitivity = "NORMAL"
		}
		if sensitivity != "NORMAL" {
			return apperrors.Forbidden("敏感字典不允许外部读取：" + dictType.DictCode)
		}
		if exposure == "INTERNAL" {
			return apperrors.Forbidden("内部字典不允许外部读取：" + dictType.DictCode)
		}
		if dictType.RequiredLogin != 0 && !actor.Authenticated {
			return apperrors.Unauthorized("字典需要登录后访问：" + dictType.DictCode)
		}
		if exposure == "AUTHENTICATED" && (!actor.Authenticated || actor.AccountID <= 0 || strings.TrimSpace(actor.ScopeID) == "") {
			return apperrors.Unauthorized("字典需要登录后访问：" + dictType.DictCode)
		}
	}
	return nil
}

func (s *Service) getDictByCodeInternal(ctx context.Context, dictCode string, internalAccess bool, actor Actor) (*domain.BatchResult, error) {
	if strings.TrimSpace(dictCode) == "" {
		return &domain.BatchResult{
			Record:  map[string][]domain.DictItemView{},
			Missing: []string{dictCode},
		}, nil
	}
	{
		cacheKey, err := s.policyDictCacheKey(ctx, s.domain.NormalizeDictCode(dictCode), internalAccess, actor)
		if err != nil {
			return nil, err
		}
		if cached, hit, err := s.cache.GetItemsByCode(ctx, cacheKey); err == nil && hit {
			return &domain.BatchResult{
				Record:  map[string][]domain.DictItemView{dictCode: cached},
				Missing: []string{},
			}, nil
		} else if err != nil {
			return nil, err
		}
	}
	result, err := s.batchGetDictInternal(ctx, dictfacade.DictBatchRequest{
		DictCodes: []string{dictCode},
		Force:     false,
	}, internalAccess, actor)
	if err != nil {
		return nil, err
	}
	if items, ok := result.Record[dictCode]; ok {
		cacheKey, keyErr := s.policyDictCacheKey(ctx, s.domain.NormalizeDictCode(dictCode), internalAccess, actor)
		if keyErr != nil {
			return nil, keyErr
		}
		if err := s.cache.SetItemsByCode(ctx, cacheKey, items); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Service) resolveTypeMoveOrder(ctx context.Context, targetID int64, beforeID, afterID *int64) (int, error) {
	if beforeID == nil && afterID == nil {
		return 1, nil
	}
	if beforeID == nil && afterID != nil {
		afterType, err := s.repo.FindTypeByID(ctx, *afterID)
		if err != nil {
			return 0, err
		}
		if afterType == nil || afterType.IsDeleted == 1 {
			return 0, apperrors.Params("afterId 对应的字典类型不存在")
		}
		if afterType.ID == targetID {
			return 0, apperrors.Params("afterId 不能是自己")
		}
		return afterType.SortOrder, nil
	}
	if beforeID != nil && afterID == nil {
		beforeType, err := s.repo.FindTypeByID(ctx, *beforeID)
		if err != nil {
			return 0, err
		}
		if beforeType == nil || beforeType.IsDeleted == 1 {
			return 0, apperrors.Params("beforeId 对应的字典类型不存在")
		}
		if beforeType.ID == targetID {
			return 0, apperrors.Params("beforeId 不能是自己")
		}
		return beforeType.SortOrder, nil
	}
	beforeType, err := s.repo.FindTypeByID(ctx, *beforeID)
	if err != nil {
		return 0, err
	}
	afterType, err := s.repo.FindTypeByID(ctx, *afterID)
	if err != nil {
		return 0, err
	}
	if beforeType == nil || beforeType.IsDeleted == 1 {
		return 0, apperrors.Params("beforeId 对应的字典类型不存在")
	}
	if afterType == nil || afterType.IsDeleted == 1 {
		return 0, apperrors.Params("afterId 对应的字典类型不存在")
	}
	if beforeType.ID == targetID || afterType.ID == targetID {
		return 0, apperrors.Params("beforeId/afterId 不能是自己")
	}
	return afterType.SortOrder, nil
}

func (s *Service) resolveItemMoveOrder(ctx context.Context, typeID, targetID int64, beforeID, afterID *int64) (int, error) {
	if beforeID == nil && afterID == nil {
		return 1, nil
	}
	if beforeID == nil && afterID != nil {
		afterItem, err := s.repo.FindItemByID(ctx, *afterID)
		if err != nil {
			return 0, err
		}
		if afterItem == nil || afterItem.IsDeleted == 1 || afterItem.DictTypeID != typeID {
			return 0, apperrors.Params("afterId 对应的字典项不存在或不属于该类型")
		}
		if afterItem.ID == targetID {
			return 0, apperrors.Params("afterId 不能是自己")
		}
		return afterItem.SortOrder, nil
	}
	if beforeID != nil && afterID == nil {
		beforeItem, err := s.repo.FindItemByID(ctx, *beforeID)
		if err != nil {
			return 0, err
		}
		if beforeItem == nil || beforeItem.IsDeleted == 1 || beforeItem.DictTypeID != typeID {
			return 0, apperrors.Params("beforeId 对应的字典项不存在或不属于该类型")
		}
		if beforeItem.ID == targetID {
			return 0, apperrors.Params("beforeId 不能是自己")
		}
		return beforeItem.SortOrder, nil
	}
	beforeItem, err := s.repo.FindItemByID(ctx, *beforeID)
	if err != nil {
		return 0, err
	}
	afterItem, err := s.repo.FindItemByID(ctx, *afterID)
	if err != nil {
		return 0, err
	}
	if beforeItem == nil || beforeItem.IsDeleted == 1 || beforeItem.DictTypeID != typeID {
		return 0, apperrors.Params("beforeId 对应的字典项不存在或不属于该类型")
	}
	if afterItem == nil || afterItem.IsDeleted == 1 || afterItem.DictTypeID != typeID {
		return 0, apperrors.Params("afterId 对应的字典项不存在或不属于该类型")
	}
	if beforeItem.ID == targetID || afterItem.ID == targetID {
		return 0, apperrors.Params("beforeId/afterId 不能是自己")
	}
	return afterItem.SortOrder, nil
}

func (s *Service) loadTypeByID(ctx context.Context, typeID int64) (*domain.DictType, error) {
	if cached, hit, err := s.cache.GetTypeByID(ctx, typeID); err == nil && hit {
		return cached, nil
	} else if err != nil {
		return nil, err
	}
	item, err := s.repo.FindTypeByID(ctx, typeID)
	if err != nil || item == nil {
		return item, err
	}
	_ = s.cache.SetTypeByID(ctx, item)
	return item, nil
}

func (s *Service) batchCacheKey(ctx context.Context, dictCodes []string) (string, error) {
	version, err := s.cache.CurrentBatchVersion(ctx)
	if err != nil {
		return "", err
	}
	return strings.Join(dictCodes, ",") + "|v=" + strconv.FormatInt(version, 10), nil
}

func (s *Service) policyDictCacheKey(ctx context.Context, dictCode string, internalAccess bool, actor Actor) (string, error) {
	version, err := s.cache.CurrentBatchVersion(ctx)
	if err != nil {
		return "", err
	}
	identity := "anonymous"
	accountID := int64(0)
	scopeID := "server:local"
	authzVersion := int64(0)
	if internalAccess {
		identity = "internal"
		scopeID = strings.TrimSpace(actor.ScopeID)
		authzVersion = actor.AuthzVersion
	} else if actor.Authenticated {
		identity = "authenticated"
		accountID = actor.AccountID
		scopeID = strings.TrimSpace(actor.ScopeID)
		authzVersion = actor.AuthzVersion
	}
	return strings.Join([]string{
		identity,
		"account=" + strconv.FormatInt(accountID, 10),
		"scope=" + scopeID,
		"authz=" + strconv.FormatInt(authzVersion, 10),
		"consumer=" + strings.TrimSpace(actor.ConsumerID),
		"purpose=" + strings.TrimSpace(actor.Purpose),
		"allowed=" + normalizeDictSensitivity(actor.AllowedSecret),
		"dict=" + dictCode,
		"v=" + strconv.FormatInt(version, 10),
	}, "|"), nil
}

func normalizeDictInternalIdentity(consumerID, serverScope, purpose, allowedSensitivity string) (string, string, string, string, error) {
	consumerID = strings.TrimSpace(consumerID)
	serverScope = strings.TrimSpace(serverScope)
	purpose = strings.TrimSpace(purpose)
	if consumerID == "" || serverScope == "" || purpose == "" {
		return "", "", "", "", apperrors.Params("内部字典读取身份字段不完整")
	}
	allowed := normalizeDictSensitivity(allowedSensitivity)
	if allowed != "NORMAL" && allowed != "SENSITIVE" && allowed != "SECRET" {
		return "", "", "", "", apperrors.Params("字典 allowedSensitivity 无效")
	}
	return consumerID, serverScope, purpose, allowed, nil
}

func normalizeDictSensitivity(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return "NORMAL"
	}
	return value
}

func dictSensitivityRank(value string) int {
	switch normalizeDictSensitivity(value) {
	case "SECRET":
		return 2
	case "SENSITIVE":
		return 1
	default:
		return 0
	}
}

func isSupportedDictSensitivity(value string) bool {
	switch normalizeDictSensitivity(value) {
	case "NORMAL", "SENSITIVE", "SECRET":
		return true
	default:
		return false
	}
}

func (s *Service) policyBatchCacheKey(ctx context.Context, dictCodes []string, internalAccess bool, actor Actor) (string, error) {
	key, err := s.policyDictCacheKey(ctx, "batch", internalAccess, actor)
	if err != nil {
		return "", err
	}
	return key + "|codes=" + strings.Join(dictCodes, ","), nil
}

func (s *Service) withTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.transactor == nil || !s.transactor.Enabled() {
		return fn(ctx)
	}
	return s.transactor.WithinTransaction(ctx, fn)
}

// withDictInvalidationTx keeps dictionary writes and the classified-cache
// Outbox event inside one transaction. No post-commit direct Redis deletion
// is used; AfterCommit only makes the writer's own L1 untrusted.
func (s *Service) withDictInvalidationTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return cachegovernancefacade.RunInvalidatedMutation(ctx, s.withTx, s.transactor, s.invalidations, cachepolicy.DataClassDictPublicItems, func(txCtx context.Context) (bool, error) {
		return true, fn(txCtx)
	})
}

func marshalValidation(value map[string]any) (string, error) {
	if value == nil {
		return "", nil
	}
	payload, err := sonic.Marshal(value)
	if err != nil {
		return "", apperrors.Params("字典 validation 无法序列化")
	}
	return string(payload), nil
}

func unmarshalValidation(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var value map[string]any
	if err := sonic.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return value
}

func buildItemViews(items []domain.DictItem, dictType *domain.DictType) []domain.DictItemView {
	result := make([]domain.DictItemView, 0, len(items))
	dictCode := ""
	dictName := ""
	if dictType != nil {
		dictCode = dictType.DictCode
		dictName = dictType.DictName
	}
	for _, item := range items {
		result = append(result, domain.DictItemView{
			DictItem: item,
			DictCode: dictCode,
			DictName: dictName,
		})
	}
	return result
}

func toDictTypeVO(item *domain.DictType) *dictfacade.DictTypeVO {
	if item == nil {
		return nil
	}
	return &dictfacade.DictTypeVO{
		ID:            item.ID,
		DictCode:      item.DictCode,
		DictName:      item.DictName,
		DictDesc:      item.DictDesc,
		Module:        item.Module,
		Status:        item.Status,
		RequiredLogin: item.RequiredLogin,
		ValueType:     item.ValueType,
		UIWidget:      item.UIWidget,
		Validation:    unmarshalValidation(item.ValidationJSON),
		Exposure:      item.Exposure,
		Sensitivity:   item.Sensitivity,
		SchemaVersion: item.SchemaVersion,
		Version:       item.Version,
		IsSystem:      item.IsSystem,
		CreatedBy:     item.CreatedBy,
		CreateTime:    item.CreateTime,
		UpdatedBy:     item.UpdatedBy,
		UpdateTime:    item.UpdateTime,
		ItemCount:     item.ItemCount,
		SortOrder:     item.SortOrder,
	}
}

func toDictItemVOList(items []domain.DictItemView) []dictfacade.DictItemVO {
	result := make([]dictfacade.DictItemVO, 0, len(items))
	for _, item := range items {
		copyItem := item
		result = append(result, dictfacade.DictItemVO{
			ID:                  copyItem.ID,
			DictTypeID:          copyItem.DictTypeID,
			DictCode:            copyItem.DictCode,
			DictName:            copyItem.DictName,
			ItemValue:           copyItem.ItemValue,
			ItemLabel:           copyItem.ItemLabel,
			ItemDesc:            copyItem.ItemDesc,
			SortOrder:           copyItem.SortOrder,
			Status:              copyItem.Status,
			ColorToken:          copyItem.ColorToken,
			IconToken:           copyItem.IconToken,
			PresentationVersion: copyItem.PresentationVersion,
			Version:             copyItem.Version,
			CreatedBy:           copyItem.CreatedBy,
			CreateTime:          copyItem.CreateTime,
			UpdatedBy:           copyItem.UpdatedBy,
			UpdateTime:          copyItem.UpdateTime,
		})
	}
	return result
}

func toBatchResponse(result *domain.BatchResult) *dictfacade.DictBatchResponse {
	if result == nil {
		return &dictfacade.DictBatchResponse{Record: map[string][]dictfacade.DictItemVO{}}
	}
	record := make(map[string][]dictfacade.DictItemVO, len(result.Record))
	for key, items := range result.Record {
		record[key] = toDictItemVOList(items)
	}
	return &dictfacade.DictBatchResponse{
		Record:  record,
		Missing: append([]string(nil), result.Missing...),
	}
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func dictCodeOrEmpty(item *domain.DictType) string {
	if item == nil {
		return ""
	}
	return item.DictCode
}
