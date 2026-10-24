package application

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	configfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/config/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
)

type Actor struct {
	UserID        int64
	Username      string
	Nickname      string
	IsAdmin       bool
	Authenticated bool
	AccountID     int64
	ScopeID       string
	AuthzVersion  int64
	RoleIDs       []int64
	Permissions   []string
	StepUpProof   stepup.ProofMetadata
}

func (a Actor) HasPermission(permission string) bool {
	if a.IsAdmin {
		return true
	}
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return true
	}
	for _, item := range a.Permissions {
		candidate := strings.TrimSpace(item)
		if candidate == permission || candidate == "*" {
			return true
		}
	}
	return false
}

type cacheStore interface {
	GetConfigByKey(ctx context.Context, configKey string) (*domain.Config, bool, error)
	SetConfigByKey(ctx context.Context, configKey string, item *domain.Config) error
	GetGroupByCode(ctx context.Context, groupCode string) (*domain.ConfigGroup, bool, error)
	SetGroupByCode(ctx context.Context, groupCode string, item *domain.ConfigGroup) error
	GetListByGroup(ctx context.Context, groupID int64) ([]domain.Config, bool, error)
	SetListByGroup(ctx context.Context, groupID int64, items []domain.Config) error
	GetBatch(ctx context.Context, cacheKey string) (map[string]domain.Config, bool, error)
	SetBatch(ctx context.Context, cacheKey string, value map[string]domain.Config) error
	CurrentBatchVersion(ctx context.Context) (int64, error)
	BumpBatchVersion(ctx context.Context) error
	InvalidateConfig(ctx context.Context, configKey string) error
	InvalidateGroup(ctx context.Context, groupCode string) error
	InvalidateGroupList(ctx context.Context, groupID int64) error
	InvalidateConfigBatch(ctx context.Context, items []domain.Config) error
}

type classifiedCacheStore interface {
	ClassifiedEnabled() bool
	GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader func(context.Context) (cachepolicy.CacheableValue, error)) (bool, error)
}

type transactor interface {
	Enabled() bool
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type readSnapshotter interface {
	Enabled() bool
	WithinReadOnlySnapshot(ctx context.Context, fn func(ctx context.Context) error) error
}

type consistentTransactor interface {
	Enabled() bool
	WithinConsistentTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

type revealCipher interface {
	EncryptForClient(obfuscatedClientPublicKey string, plain string) (string, error)
}

type Service struct {
	transactor    transactor
	repo          domain.Repository
	cache         cacheStore
	domain        *domain.Service
	secrets       domain.SecretCipher
	reveal        revealCipher
	users         domain.UserLookup
	invalidations cachegovernancefacade.InvalidationRegistrar
	assets        filefacade.ConfigAssetFacade
	roles         interface {
		GetRole(ctx context.Context, roleID int64) (*authorizationfacade.RoleVO, error)
		AdvanceRoleGrantRevision(ctx context.Context, roleID int64, operatorID int64) error
	}
	consumersMu             sync.RWMutex
	internalConsumers       map[string]domain.ConsumerRegistration
	consumerRegistryVersion int64
}

const (
	stepUpActionConfigSensitiveReveal = "CONFIG_SENSITIVE_REVEAL"
	stepUpActionConfigApplyPending    = "CONFIG_APPLY_PENDING"
	stepUpActionConfigRollback        = "CONFIG_ROLLBACK"
	stepUpActionConfigScopeAssign     = "CONFIG_SCOPE_ASSIGN"
	pendingConfigApplyChunkSize       = 50
	roleConfigScopeGrantMax           = 100
)

func NewService(
	transactor transactor,
	repo domain.Repository,
	cache cacheStore,
	domainService *domain.Service,
	secrets domain.SecretCipher,
	reveal revealCipher,
	users domain.UserLookup,
	invalidations ...cachegovernancefacade.InvalidationRegistrar,
) *Service {
	var registrar cachegovernancefacade.InvalidationRegistrar
	if len(invalidations) > 0 {
		registrar = invalidations[0]
	}
	return &Service{
		transactor:        transactor,
		repo:              repo,
		cache:             cache,
		domain:            domainService,
		secrets:           secrets,
		reveal:            reveal,
		users:             users,
		invalidations:     registrar,
		internalConsumers: map[string]domain.ConsumerRegistration{},
	}
}

func (s *Service) BindConsumerRegistry(registrations []domain.ConsumerRegistration) {
	if s == nil {
		return
	}
	s.consumersMu.Lock()
	defer s.consumersMu.Unlock()
	s.internalConsumers = make(map[string]domain.ConsumerRegistration, len(registrations))
	s.consumerRegistryVersion++
	for _, registration := range registrations {
		key := strings.TrimSpace(registration.ConsumerID) + "\x00" + strings.TrimSpace(registration.FullyQualifiedKey)
		s.internalConsumers[key] = registration
	}
}

func (s *Service) BindConfigConsumers(registrations []configfacade.ConfigConsumerRegistration) {
	s.BindConsumerRegistry(convertConfigConsumers(registrations))
}

// RegisterConfigConsumers is additive. ConfigFacade is shared by all
// modules, so a late Workforce binding must never wipe registrations that
// another module installed at startup.
func (s *Service) RegisterConfigConsumers(registrations []configfacade.ConfigConsumerRegistration) {
	converted := convertConfigConsumers(registrations)
	if s == nil || len(converted) == 0 {
		return
	}
	s.consumersMu.Lock()
	defer s.consumersMu.Unlock()
	if s.internalConsumers == nil {
		s.internalConsumers = map[string]domain.ConsumerRegistration{}
	}
	for _, registration := range converted {
		key := registration.ConsumerID + "\x00" + registration.FullyQualifiedKey
		s.internalConsumers[key] = registration
	}
	s.consumerRegistryVersion++
}

func convertConfigConsumers(registrations []configfacade.ConfigConsumerRegistration) []domain.ConsumerRegistration {
	converted := make([]domain.ConsumerRegistration, 0, len(registrations))
	for _, registration := range registrations {
		allowed, err := domain.NormalizeSensitivity(registration.AllowedSensitivity, 0)
		if err != nil {
			continue
		}
		converted = append(converted, domain.ConsumerRegistration{
			ConsumerID:        strings.TrimSpace(registration.ConsumerID),
			FullyQualifiedKey: strings.TrimSpace(registration.FullyQualifiedKey),
			ScopeID:           strings.TrimSpace(registration.ServerScope),
			Purpose:           strings.TrimSpace(registration.Purpose),
			AllowedSecret:     allowed,
			Source:            strings.TrimSpace(registration.Source),
			ActualConsumer:    strings.TrimSpace(registration.ActualConsumer),
			Activation:        strings.TrimSpace(registration.Activation),
			CacheRule:         strings.TrimSpace(registration.CacheRule),
		})
	}
	return converted
}

func (s *Service) BindRoleSecurity(roles interface {
	GetRole(ctx context.Context, roleID int64) (*authorizationfacade.RoleVO, error)
	AdvanceRoleGrantRevision(ctx context.Context, roleID int64, operatorID int64) error
}) {
	if s != nil {
		s.roles = roles
	}
}

// BindConfigAssets wires the dedicated file facade after both modules are
// installed. It remains optional for scalar-only deployments, while IMAGE and
// FILE requests fail closed until the binding is present.
func (s *Service) BindConfigAssets(assets filefacade.ConfigAssetFacade) {
	if s != nil {
		s.assets = assets
	}
}

func (s *Service) AddConfigGroup(ctx context.Context, actor Actor, request configfacade.ConfigGroupAddRequest) (int64, error) {
	if actor.UserID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	if !actor.IsAdmin {
		return 0, apperrors.Forbidden("无权限创建配置分组")
	}
	groupCode := s.domain.NormalizeGroupCode(request.GroupCode)
	if groupCode == "" {
		return 0, apperrors.Params("配置分组编码不能为空")
	}
	status := defaultInt(request.Status, 1)
	if err := s.domain.ValidateStatus(status); err != nil {
		return 0, err
	}
	count, err := s.repo.CountGroupByCode(ctx, groupCode, 0)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, apperrors.Params("配置分组编码已存在：" + groupCode)
	}
	now := time.Now().UTC()
	item := &domain.ConfigGroup{
		GroupCode:      groupCode,
		GroupName:      strings.TrimSpace(request.GroupName),
		Module:         strings.TrimSpace(request.Module),
		PermissionCode: strings.TrimSpace(request.PermissionCode),
		SortOrder:      defaultInt(request.SortOrder, 0),
		Status:         status,
		CreateTime:     &now,
		UpdateTime:     &now,
		IsDeleted:      0,
	}
	var id int64
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		var createErr error
		id, createErr = s.repo.InsertGroup(txCtx, item)
		return createErr
	}); err != nil {
		return 0, err
	}
	item.ID = id
	return id, nil
}

func (s *Service) UpdateConfigGroup(ctx context.Context, actor Actor, request configfacade.ConfigGroupUpdateRequest) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindGroupByID(ctx, request.ID)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("配置分组不存在")
	}
	if err := s.requireConfigWriteAccess(ctx, actor, item.GroupCode, ""); err != nil {
		return err
	}
	oldGroupCode := item.GroupCode
	if request.GroupCode != nil {
		next := s.domain.NormalizeGroupCode(*request.GroupCode)
		if next == "" {
			return apperrors.Params("配置分组编码不能为空")
		}
		if next != oldGroupCode {
			count, countErr := s.repo.CountGroupByCode(ctx, next, item.ID)
			if countErr != nil {
				return countErr
			}
			if count > 0 {
				return apperrors.Params("配置分组编码已存在：" + next)
			}
			item.GroupCode = next
		}
	}
	if request.GroupName != nil {
		item.GroupName = strings.TrimSpace(*request.GroupName)
	}
	if request.Module != nil {
		item.Module = strings.TrimSpace(*request.Module)
	}
	if request.PermissionCode != nil {
		item.PermissionCode = strings.TrimSpace(*request.PermissionCode)
	}
	if request.SortOrder != nil {
		item.SortOrder = *request.SortOrder
	}
	if request.Status != nil {
		if err := s.domain.ValidateStatus(*request.Status); err != nil {
			return err
		}
		item.Status = *request.Status
	}
	now := time.Now().UTC()
	item.UpdateTime = &now
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateGroup(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteConfigGroup(ctx context.Context, actor Actor, id int64) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindGroupByID(ctx, id)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("配置分组不存在")
	}
	if err := s.requireConfigDeleteAccess(ctx, actor, item.GroupCode, ""); err != nil {
		return err
	}
	count, err := s.repo.CountConfigsByGroupID(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperrors.Operation("配置分组下存在配置项，无法删除")
	}
	now := time.Now().UTC()
	item.IsDeleted = 1
	item.UpdateTime = &now
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateGroup(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetConfigGroupPage(ctx context.Context, actor Actor, request configfacade.ConfigGroupQueryRequest) (*configfacade.PageResult[configfacade.ConfigGroupVO], error) {
	current, size := s.domain.NormalizePage(firstPositiveInt64(request.Current, request.PageNum), request.PageSize)
	var result *configfacade.PageResult[configfacade.ConfigGroupVO]
	if err := s.withReadOnlySnapshot(ctx, func(snapshotCtx context.Context) error {
		page, err := s.repo.QueryGroups(snapshotCtx, domain.ConfigGroupPageQuery{
			Current:   current,
			PageSize:  size,
			GroupCode: s.domain.NormalizeGroupCode(request.GroupCode),
			GroupName: strings.TrimSpace(request.GroupName),
			Module:    strings.TrimSpace(request.Module),
			Status:    request.Status,
		})
		if err != nil {
			return err
		}
		accessGrants, err := s.loadConfigAccessGrants(snapshotCtx, actor)
		if err != nil {
			return err
		}
		records := make([]configfacade.ConfigGroupVO, 0, len(page.Records))
		for _, item := range page.Records {
			copyItem := item
			access := s.resolveConfigAccessFromGrants(actor, accessGrants, copyItem.GroupCode, "")
			if !actor.IsAdmin && !access.CanRead {
				continue
			}
			records = append(records, *toConfigGroupVO(&copyItem, access))
		}
		result = &configfacade.PageResult[configfacade.ConfigGroupVO]{
			Current: page.Current,
			Size:    page.Size,
			Total:   filteredTotal(page.Total, len(page.Records), len(records), actor),
			Records: records,
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) GetConfigGroupByID(ctx context.Context, actor Actor, id int64) (*configfacade.ConfigGroupVO, error) {
	item, err := s.loadGroupByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.IsDeleted == 1 {
		return nil, apperrors.NotFound("配置分组不存在")
	}
	count, err := s.repo.CountConfigsByGroupID(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	item.ConfigCount = count
	access, err := s.resolveConfigAccess(ctx, actor, item.GroupCode, "")
	if err != nil {
		return nil, err
	}
	if !actor.IsAdmin && !access.CanRead {
		return nil, apperrors.DataScopeDenied("配置范围不足")
	}
	return toConfigGroupVO(item, access), nil
}

func (s *Service) MoveConfigGroup(ctx context.Context, actor Actor, id int64, beforeID, afterID *int64) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	target, err := s.repo.FindGroupByID(ctx, id)
	if err != nil {
		return err
	}
	if target == nil || target.IsDeleted == 1 {
		return apperrors.NotFound("配置分组不存在")
	}
	if err := s.requireConfigWriteAccess(ctx, actor, target.GroupCode, ""); err != nil {
		return err
	}
	oldOrder := target.SortOrder
	newOrder, err := s.resolveGroupMoveOrder(ctx, id, beforeID, afterID)
	if err != nil {
		return err
	}
	if oldOrder == newOrder {
		return nil
	}
	now := time.Now().UTC()
	target.SortOrder = newOrder
	target.UpdateTime = &now
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.ShiftGroupSort(txCtx, id, oldOrder, newOrder); err != nil {
			return err
		}
		return s.repo.UpdateGroup(txCtx, target)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) AddConfig(ctx context.Context, actor Actor, request configfacade.ConfigAddRequest) (int64, error) {
	if actor.UserID <= 0 {
		return 0, apperrors.Unauthorized("未登录或登录信息失效")
	}
	group, err := s.repo.FindGroupByID(ctx, request.GroupID)
	if err != nil {
		return 0, err
	}
	if group == nil || group.IsDeleted == 1 {
		return 0, apperrors.NotFound("配置分组不存在")
	}
	if err := s.requireConfigWriteAccess(ctx, actor, group.GroupCode, ""); err != nil {
		return 0, err
	}
	configKey := s.domain.NormalizeConfigKey(request.ConfigKey)
	if configKey == "" {
		return 0, apperrors.Params("配置键不能为空")
	}
	valueType, err := domain.NormalizeValueType(request.ValueType)
	if err != nil {
		return 0, apperrors.Params(err.Error())
	}
	widget, err := domain.NormalizeWidget(request.UIWidget, valueType)
	if err != nil {
		return 0, apperrors.Params(err.Error())
	}
	exposure, err := domain.NormalizeExposure(request.Exposure)
	if err != nil {
		return 0, apperrors.Params(err.Error())
	}
	sensitivity, err := domain.NormalizeSensitivity(request.Sensitivity, defaultInt(request.IsSensitive, 0))
	if err != nil {
		return 0, apperrors.Params(err.Error())
	}
	if sensitivity == domain.ConfigSensitivitySecret && !strings.EqualFold(strings.TrimSpace(request.Sensitivity), string(domain.ConfigSensitivitySecret)) {
		return 0, apperrors.Params("SECRET 仅允许通过新契约显式创建")
	}
	schemaVersion := defaultInt(request.SchemaVersion, domain.CurrentScalarSchemaVersion)
	validation := toDomainValidation(request.Validation)
	if validation == nil && request.ExtJSON != nil && len(request.ExtJSON.Enums) > 0 {
		validation = &domain.ScalarValidation{Options: append([]string(nil), request.ExtJSON.Enums...)}
	}
	if err := domain.ValidateScalarMetadata(valueType, widget, validation, schemaVersion); err != nil {
		return 0, apperrors.Params(err.Error())
	}
	if sensitivity != domain.ConfigSensitivityNormal && s.domain.NormalizeEffectType(request.EffectType) == string(domain.ConfigEffectRestart) {
		return 0, apperrors.Params("敏感配置首版仅允许实时生效，避免待应用日志承载明文")
	}
	assetType, isAsset := configAssetType(valueType)
	if isAsset {
		if sensitivity != domain.ConfigSensitivityNormal {
			return 0, apperrors.Params("IMAGE/FILE 配置不支持敏感或 SECRET 级别")
		}
		if request.AssetFileID == nil || *request.AssetFileID <= 0 {
			return 0, apperrors.Params("IMAGE/FILE 配置创建时必须提供已上传的 assetFileId")
		}
		if strings.TrimSpace(request.ConfigValue) != "" {
			return 0, apperrors.Params("IMAGE/FILE 配置不接受自定义 URL 或配置值")
		}
	} else if request.AssetFileID != nil {
		return 0, apperrors.Params("assetFileId 仅支持 IMAGE/FILE 配置")
	}
	valueForValidation := request.ConfigValue
	if isAsset {
		// configId is allocated by the repository. Validate the durable path
		// contract now with a non-authoritative placeholder, then replace it
		// inside the same transaction after the real ID is known.
		valueForValidation = filefacade.ConfigAssetStablePath(1)
	}
	canonicalValue, _, err := domain.CanonicalizeScalarValue(valueForValidation, valueType, validation)
	if err != nil {
		return 0, apperrors.Params(err.Error())
	}
	extJSON, err := s.domain.NormalizeExtJSON(toDomainExtJSON(request.ExtJSON), strings.ToLower(string(valueType)))
	if err != nil {
		return 0, err
	}
	count, err := s.repo.CountConfigByGroupAndKey(ctx, group.ID, configKey, 0)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, apperrors.Params("该分组下配置键已存在：" + configKey)
	}
	item := &domain.Config{
		GroupID:        group.ID,
		ConfigKey:      configKey,
		ValueType:      string(valueType),
		ConfigDesc:     strings.TrimSpace(request.ConfigDesc),
		IsSensitive:    boolInt(sensitivity != domain.ConfigSensitivityNormal),
		IsSystemConfig: defaultInt(request.IsSystemConfig, 0),
		RequiredLogin:  defaultInt(request.RequiredLogin, 0),
		UIWidget:       string(widget),
		Validation:     validation,
		Exposure:       string(exposure),
		Sensitivity:    string(sensitivity),
		SchemaVersion:  schemaVersion,
		Version:        1,
		ExtJSON:        extJSON,
		IsReadonly:     defaultInt(request.IsReadonly, 0),
		IsEnabled:      defaultInt(request.IsEnabled, 1),
		EffectType:     s.domain.NormalizeEffectType(request.EffectType),
		CreatedBy:      actor.UserID,
		UpdatedBy:      actor.UserID,
		IsDeleted:      0,
		GroupCode:      group.GroupCode,
		GroupName:      group.GroupName,
	}
	if err := s.guardReadonlyMutation(actor, 0, item.IsReadonly); err != nil {
		return 0, err
	}
	if err := s.guardSensitiveMutation(actor, 0, item.IsSensitive); err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	item.CreateTime = &now
	item.UpdateTime = &now
	if err := s.prepareConfigValueStorage(ctx, item, canonicalValue); err != nil {
		return 0, err
	}
	var id int64
	var createLogID int64
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		var createErr error
		id, createErr = s.repo.InsertConfig(txCtx, item)
		if createErr != nil {
			return createErr
		}
		item.ID = id
		if isAsset {
			if s.assets == nil {
				return apperrors.System("配置资产服务未配置")
			}
			if err := s.assets.BindConfigAsset(txCtx, filefacade.BindConfigAssetCommand{
				FileID:    *request.AssetFileID,
				ConfigID:  item.ID,
				AssetType: assetType,
				Exposure:  configAssetExposure(exposure),
			}); err != nil {
				return err
			}
			canonicalValue = filefacade.ConfigAssetStablePath(item.ID)
			if err := s.prepareConfigValueStorage(txCtx, item, canonicalValue); err != nil {
				return err
			}
			if err := s.repo.UpdateConfig(txCtx, item); err != nil {
				return err
			}
		}
		createLog := s.domain.BuildChangeLog(domain.CreateChangeLogInput{
			ConfigID:        item.ID,
			ConfigKey:       item.ConfigKey,
			OperationType:   domain.ConfigOperationCreate,
			OldValue:        "",
			NewValue:        protectedAuditValue(item, canonicalValue),
			EffectType:      item.EffectType,
			OperatorID:      actor.UserID,
			OperationReason: "新增配置",
			Now:             now,
		})
		createLog.OperatorName = actorName(actor)
		createLogID, createErr = s.repo.InsertChangeLog(txCtx, createLog)
		if createErr != nil {
			return createErr
		}
		_ = createLogID
		return nil
	}); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Service) UpdateConfig(ctx context.Context, actor Actor, request configfacade.ConfigUpdateRequest) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindConfigByID(ctx, request.ID)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("配置项不存在")
	}
	group, err := s.repo.FindGroupByID(ctx, item.GroupID)
	if err != nil {
		return err
	}
	if group == nil || group.IsDeleted == 1 {
		return apperrors.NotFound("配置分组不存在")
	}
	if err := s.requireConfigWriteAccess(ctx, actor, item.GroupCode, item.ConfigKey); err != nil {
		return err
	}

	oldRuntimeValue, err := s.resolveRuntimeValue(ctx, item)
	if err != nil {
		return err
	}
	// Keep the persisted policy before mutating item below. The asset facade
	// needs this comparison to notice an exposure-only edit and derive the
	// corresponding sys_file_reference policy in the same transaction.
	previousExposure := strings.TrimSpace(item.Exposure)
	oldReadonly := item.IsReadonly
	oldSensitive := item.IsSensitive
	if request.Version != nil && *request.Version != item.Version {
		return apperrors.Params("配置版本已变化，请刷新后重试")
	}

	if request.IsReadonly != nil {
		if err := s.guardReadonlyMutation(actor, oldReadonly, *request.IsReadonly); err != nil {
			return err
		}
	} else if oldReadonly == 1 && !s.canEditReadonly(actor) {
		return apperrors.Forbidden("该配置为只读配置，您无权修改")
	}

	targetValueType, err := domain.NormalizeValueType(item.ValueType)
	if err != nil {
		return apperrors.Operation("现有配置值类型无效")
	}
	if request.ValueType != nil && strings.TrimSpace(*request.ValueType) != "" {
		targetValueType, err = domain.NormalizeValueType(*request.ValueType)
		if err != nil {
			return apperrors.Params(err.Error())
		}
	}
	targetWidget, err := domain.NormalizeWidget(item.UIWidget, targetValueType)
	if err != nil {
		targetWidget = domain.DefaultWidget(targetValueType)
	}
	if request.UIWidget != nil {
		targetWidget, err = domain.NormalizeWidget(*request.UIWidget, targetValueType)
		if err != nil {
			return apperrors.Params(err.Error())
		}
	}
	targetExposure, err := domain.NormalizeExposure(item.Exposure)
	if err != nil {
		return apperrors.Operation("现有配置暴露级别无效")
	}
	if request.Exposure != nil {
		targetExposure, err = domain.NormalizeExposure(*request.Exposure)
		if err != nil {
			return apperrors.Params(err.Error())
		}
	}
	targetSensitivity, err := domain.NormalizeSensitivity(item.Sensitivity, item.IsSensitive)
	if err != nil {
		return apperrors.Operation("现有配置敏感级别无效")
	}
	if request.Sensitivity != nil {
		targetSensitivity, err = domain.NormalizeSensitivity(*request.Sensitivity, item.IsSensitive)
		if err != nil {
			return apperrors.Params(err.Error())
		}
	} else if request.IsSensitive != nil {
		targetSensitivity, err = domain.NormalizeSensitivity("", *request.IsSensitive)
		if err != nil {
			return apperrors.Params(err.Error())
		}
	}
	targetSensitive := boolInt(targetSensitivity != domain.ConfigSensitivityNormal)
	if err := s.guardSensitiveMutation(actor, oldSensitive, targetSensitive); err != nil {
		return err
	}
	oldValueType, oldValueTypeErr := domain.NormalizeValueType(item.ValueType)
	if oldValueTypeErr != nil {
		return apperrors.Operation("现有配置值类型无效")
	}
	oldAssetType, oldIsAsset := configAssetType(oldValueType)
	oldExposure, oldExposureErr := domain.NormalizeExposure(previousExposure)
	if oldExposureErr != nil {
		return apperrors.Operation("现有配置暴露级别无效")
	}
	targetAssetType, targetIsAsset := configAssetType(targetValueType)
	assetFileRequested := request.AssetFileID != nil
	clearAssetRequested := request.ClearAsset != nil && *request.ClearAsset
	if request.AssetFileID != nil && *request.AssetFileID <= 0 {
		return apperrors.Params("assetFileId 必须是正整数")
	}
	if assetFileRequested && clearAssetRequested {
		return apperrors.Params("assetFileId 与 clearAsset 不能同时提交")
	}
	if !targetIsAsset && (assetFileRequested || clearAssetRequested) {
		return apperrors.Params("assetFileId/clearAsset 仅支持 IMAGE/FILE 配置")
	}
	if targetIsAsset {
		if targetSensitivity != domain.ConfigSensitivityNormal {
			return apperrors.Params("IMAGE/FILE 配置不支持敏感或 SECRET 级别")
		}
		if request.ConfigValue != nil && strings.TrimSpace(*request.ConfigValue) != "" {
			return apperrors.Params("IMAGE/FILE 配置不接受自定义 URL 或配置值")
		}
		if !oldIsAsset && !assetFileRequested {
			return apperrors.Params("切换到 IMAGE/FILE 时必须提供已上传的 assetFileId")
		}
		if oldIsAsset && oldValueType != targetValueType && !assetFileRequested {
			return apperrors.Params("切换 IMAGE 与 FILE 时必须提供已上传的 assetFileId")
		}
	}
	if oldIsAsset && !targetIsAsset && !clearAssetRequested {
		return apperrors.Params("切换 IMAGE/FILE 为标量时必须显式 clearAsset")
	}
	if oldIsAsset && !targetIsAsset && (request.ConfigValue == nil || strings.TrimSpace(*request.ConfigValue) == "") {
		return apperrors.Params("切换 IMAGE/FILE 为标量时必须提供新的标量配置值")
	}
	targetValidation := item.Validation
	if request.Validation != nil {
		targetValidation = toDomainValidation(request.Validation)
	}
	targetSchemaVersion := item.SchemaVersion
	if targetSchemaVersion == 0 {
		targetSchemaVersion = domain.CurrentScalarSchemaVersion
	}
	if request.SchemaVersion != nil {
		targetSchemaVersion = *request.SchemaVersion
	}
	if err := domain.ValidateScalarMetadata(targetValueType, targetWidget, targetValidation, targetSchemaVersion); err != nil {
		return apperrors.Params(err.Error())
	}
	targetEffectType := item.EffectType
	if request.EffectType != nil && strings.TrimSpace(*request.EffectType) != "" {
		targetEffectType = s.domain.NormalizeEffectType(*request.EffectType)
	}
	if targetEffectType == "" {
		targetEffectType = string(domain.ConfigEffectRealtime)
	}
	if (oldIsAsset || targetIsAsset) && targetEffectType != string(domain.ConfigEffectRealtime) {
		return apperrors.Params("IMAGE/FILE 配置首版仅支持即时生效")
	}
	if targetSensitivity != domain.ConfigSensitivityNormal && targetEffectType == string(domain.ConfigEffectRestart) {
		return apperrors.Params("敏感配置首版仅允许实时生效，避免待应用日志承载明文")
	}
	mergedExtJSON := s.domain.MergeExtJSON(item.ExtJSON, toDomainExtJSON(request.ExtJSON))
	mergedExtJSON, err = s.domain.NormalizeExtJSON(mergedExtJSON, strings.ToLower(string(targetValueType)))
	if err != nil {
		return err
	}

	configValueChanged := false
	newRuntimeValue := oldRuntimeValue
	if targetIsAsset {
		switch {
		case assetFileRequested:
			newRuntimeValue = filefacade.ConfigAssetStablePath(item.ID)
			configValueChanged = newRuntimeValue != oldRuntimeValue
		case clearAssetRequested:
			newRuntimeValue = ""
			configValueChanged = newRuntimeValue != oldRuntimeValue
		}
	} else if request.ConfigValue != nil && strings.TrimSpace(*request.ConfigValue) != "" {
		requestValue := *request.ConfigValue
		if targetSensitive == 1 && s.domain.IsMaskedSensitivePlaceholder(requestValue) {
			request.ConfigValue = nil
		} else {
			newRuntimeValue = requestValue
			configValueChanged = newRuntimeValue != oldRuntimeValue
		}
	}
	canonicalValue, _, err := domain.CanonicalizeScalarValue(newRuntimeValue, targetValueType, targetValidation)
	if err != nil {
		return apperrors.Params(err.Error())
	}
	newRuntimeValue = canonicalValue

	if request.ConfigKey != nil && strings.TrimSpace(*request.ConfigKey) != "" {
		nextKey := s.domain.NormalizeConfigKey(*request.ConfigKey)
		if nextKey == "" {
			return apperrors.Params("配置键不能为空")
		}
		if nextKey != item.ConfigKey {
			count, countErr := s.repo.CountConfigByGroupAndKey(ctx, item.GroupID, nextKey, item.ID)
			if countErr != nil {
				return countErr
			}
			if count > 0 {
				return apperrors.Params("该分组下配置键已存在：" + nextKey)
			}
			item.ConfigKey = nextKey
		}
	}
	if request.ConfigDesc != nil {
		item.ConfigDesc = strings.TrimSpace(*request.ConfigDesc)
	}
	if request.IsEnabled != nil {
		if err := s.domain.ValidateStatus(*request.IsEnabled); err != nil {
			return err
		}
		item.IsEnabled = *request.IsEnabled
	}
	if request.EffectType != nil && strings.TrimSpace(*request.EffectType) != "" {
		item.EffectType = targetEffectType
	}
	if request.ValueType != nil && strings.TrimSpace(*request.ValueType) != "" {
		item.ValueType = string(targetValueType)
	}
	if request.IsSensitive != nil {
		item.IsSensitive = targetSensitive
	}
	item.ValueType = string(targetValueType)
	item.IsSensitive = targetSensitive
	item.UIWidget = string(targetWidget)
	item.Validation = targetValidation
	item.Exposure = string(targetExposure)
	item.Sensitivity = string(targetSensitivity)
	item.SchemaVersion = targetSchemaVersion
	if request.IsSystemConfig != nil {
		item.IsSystemConfig = *request.IsSystemConfig
	}
	if request.RequiredLogin != nil {
		item.RequiredLogin = *request.RequiredLogin
	}
	if request.ExtJSON != nil {
		item.ExtJSON = mergedExtJSON
	}
	if request.IsReadonly != nil {
		item.IsReadonly = *request.IsReadonly
	}

	metadataChangeAffectStorage := request.ValueType != nil || request.IsSensitive != nil || request.Sensitivity != nil || request.ExtJSON != nil
	switch {
	case configValueChanged && targetEffectType == string(domain.ConfigEffectRealtime):
		item.ValueType = string(targetValueType)
		item.IsSensitive = targetSensitive
		item.ExtJSON = mergedExtJSON
		if err := s.prepareConfigValueStorage(ctx, item, newRuntimeValue); err != nil {
			return err
		}
	case metadataChangeAffectStorage:
		item.ValueType = string(targetValueType)
		item.IsSensitive = targetSensitive
		item.ExtJSON = mergedExtJSON
		if err := s.prepareConfigValueStorage(ctx, item, oldRuntimeValue); err != nil {
			return err
		}
	}

	now := time.Now().UTC()
	item.UpdatedBy = actor.UserID
	item.UpdateTime = &now
	if item.GroupCode == "" {
		item.GroupCode = group.GroupCode
	}
	if item.GroupName == "" {
		item.GroupName = group.GroupName
	}

	assetPolicyChanged := targetIsAsset && oldIsAsset && !assetFileRequested && !clearAssetRequested &&
		(previousExposure != string(targetExposure) || oldValueType != targetValueType)
	assetLifecycleChanged := (targetIsAsset && (assetFileRequested || clearAssetRequested)) || (oldIsAsset && !targetIsAsset)
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		// Conditional config update goes first for asset mutations. A stale
		// version therefore fails before it can bind/replace a reference.
		if err := s.repo.UpdateConfig(txCtx, item); err != nil {
			return err
		}
		var oldAssetSnapshot *domain.ConfigAssetBindingSnapshot
		var newAssetSnapshot *domain.ConfigAssetBindingSnapshot
		if assetLifecycleChanged || assetPolicyChanged {
			if s.assets == nil {
				return apperrors.System("配置资产服务未配置")
			}
			captureType := targetAssetType
			captureExposure := configAssetExposure(targetExposure)
			if oldIsAsset {
				captureType = oldAssetType
				captureExposure = configAssetExposure(oldExposure)
			}
			captured, captureErr := s.assets.CaptureConfigAssetBinding(txCtx, filefacade.CaptureConfigAssetBindingCommand{
				ConfigID: item.ID, AssetType: captureType, Exposure: captureExposure,
			})
			if captureErr != nil {
				return captureErr
			}
			if oldIsAsset {
				if err := validateConfigAssetSnapshotRuntimeValue(captured, item.ID, oldRuntimeValue); err != nil {
					return err
				}
			}
			oldAssetSnapshot = configAssetAuditSnapshot(captured)
			switch {
			case targetIsAsset && assetFileRequested:
				if err := s.assets.BindConfigAsset(txCtx, filefacade.BindConfigAssetCommand{
					FileID:    *request.AssetFileID,
					ConfigID:  item.ID,
					AssetType: targetAssetType,
					Exposure:  configAssetExposure(targetExposure),
				}); err != nil {
					return err
				}
			case targetIsAsset && clearAssetRequested:
				if err := s.assets.ClearConfigAsset(txCtx, item.ID); err != nil {
					return err
				}
			case oldIsAsset && !targetIsAsset:
				if err := s.assets.ClearConfigAsset(txCtx, item.ID); err != nil {
					return err
				}
			case assetPolicyChanged:
				if err := s.assets.UpdateConfigAssetPolicy(txCtx, filefacade.UpdateConfigAssetPolicyCommand{
					ConfigID:  item.ID,
					AssetType: targetAssetType,
					Exposure:  configAssetExposure(targetExposure),
				}); err != nil {
					return err
				}
			}
			captureType = targetAssetType
			captureExposure = configAssetExposure(targetExposure)
			if !targetIsAsset {
				// A type transition away from IMAGE/FILE is deliberately recorded
				// as an empty state using the former asset policy. The rollback
				// path later refuses to reconstruct metadata it cannot prove.
				captureType = oldAssetType
				captureExposure = configAssetExposure(oldExposure)
			}
			captured, captureErr = s.assets.CaptureConfigAssetBinding(txCtx, filefacade.CaptureConfigAssetBindingCommand{
				ConfigID: item.ID, AssetType: captureType, Exposure: captureExposure,
			})
			if captureErr != nil {
				return captureErr
			}
			if targetIsAsset {
				if err := validateConfigAssetSnapshotRuntimeValue(captured, item.ID, newRuntimeValue); err != nil {
					return err
				}
			}
			newAssetSnapshot = configAssetAuditSnapshot(captured)
		}
		if configValueChanged || assetLifecycleChanged || assetPolicyChanged {
			reason := "配置更新"
			switch {
			case targetIsAsset && assetFileRequested:
				reason = "配置资产替换"
			case targetIsAsset && clearAssetRequested:
				reason = "配置资产清除"
			case oldIsAsset && !targetIsAsset:
				reason = "配置资产清除并切换标量"
			case assetPolicyChanged:
				reason = "配置资产读取策略更新"
			}
			changeLog := s.domain.BuildChangeLog(domain.CreateChangeLogInput{
				ConfigID:         item.ID,
				ConfigKey:        item.ConfigKey,
				OperationType:    domain.ConfigOperationUpdate,
				OldValue:         protectedAuditValue(item, oldRuntimeValue),
				NewValue:         protectedAuditValue(item, newRuntimeValue),
				EffectType:       targetEffectType,
				OperatorID:       actor.UserID,
				OperationReason:  reason,
				OldAssetSnapshot: oldAssetSnapshot,
				NewAssetSnapshot: newAssetSnapshot,
				Now:              now,
			})
			changeLog.OperatorName = actorName(actor)
			if _, err := s.repo.InsertChangeLog(txCtx, changeLog); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) DeleteConfig(ctx context.Context, actor Actor, id int64) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	item, err := s.repo.FindConfigByID(ctx, id)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("配置项不存在")
	}
	if err := s.requireConfigDeleteAccess(ctx, actor, item.GroupCode, item.ConfigKey); err != nil {
		return err
	}
	oldRuntimeValue, err := s.resolveRuntimeValue(ctx, item)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	item.IsDeleted = 1
	item.UpdatedBy = actor.UserID
	item.UpdateTime = &now
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.UpdateConfig(txCtx, item); err != nil {
			return err
		}
		if _, isAsset := configAssetTypeMust(item.ValueType); isAsset {
			if s.assets == nil {
				return apperrors.System("配置资产服务未配置")
			}
			// Deleting a configuration must retire its one active CONFIG_ASSET
			// reference in the same outer transaction. The object itself stays
			// with the DC1 lifecycle for later eligible cleanup.
			if err := s.assets.ClearConfigAsset(txCtx, item.ID); err != nil {
				return err
			}
		}
		deleteLog := s.domain.BuildChangeLog(domain.CreateChangeLogInput{
			ConfigID:      item.ID,
			ConfigKey:     item.ConfigKey,
			OperationType: domain.ConfigOperationDelete,
			OldValue:      protectedAuditValue(item, oldRuntimeValue),
			NewValue:      "",
			EffectType:    item.EffectType,
			OperatorID:    actor.UserID,
			Now:           now,
		})
		deleteLog.OperatorName = actorName(actor)
		_, err := s.repo.InsertChangeLog(txCtx, deleteLog)
		return err
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetConfigByID(ctx context.Context, actor Actor, id int64) (*configfacade.ConfigVO, error) {
	item, err := s.repo.FindConfigByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.IsDeleted == 1 {
		return nil, apperrors.NotFound("配置项不存在")
	}
	access, err := s.resolveConfigAccess(ctx, actor, item.GroupCode, item.ConfigKey)
	if err != nil {
		return nil, err
	}
	if !actor.IsAdmin && !access.CanRead {
		return nil, apperrors.DataScopeDenied("配置范围不足")
	}
	return s.toConfigVO(ctx, item, access), nil
}

// OpenConfigAsset authorizes and opens the stable same-origin representation
// of an IMAGE/FILE configuration. The generic file download paths deliberately
// reject CONFIG_ASSET; this method is the sole place that maps a config ID to
// its hidden file reference and derived delivery policy.
func (s *Service) OpenConfigAsset(ctx context.Context, actor Actor, id int64) (*filefacade.ConfigAssetOpenResult, error) {
	if id <= 0 {
		return nil, apperrors.Params("配置ID不能为空")
	}
	item, err := s.repo.FindConfigByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.IsDeleted == 1 || item.IsEnabled != 1 {
		return nil, apperrors.NotFound("配置资产不存在")
	}
	group, err := s.repo.FindGroupByID(ctx, item.GroupID)
	if err != nil {
		return nil, err
	}
	if group == nil || group.IsDeleted == 1 || group.Status != 1 {
		return nil, apperrors.NotFound("配置资产不存在")
	}
	item.GroupCode = group.GroupCode
	item.GroupName = group.GroupName
	assetType, isAsset := configAssetTypeMust(item.ValueType)
	if !isAsset || strings.TrimSpace(item.ConfigValue) != filefacade.ConfigAssetStablePath(item.ID) {
		return nil, apperrors.NotFound("配置资产不存在")
	}
	exposure, err := domain.NormalizeExposure(item.Exposure)
	if err != nil {
		return nil, apperrors.Operation("配置资产暴露级别无效")
	}
	sensitivity, err := domain.NormalizeSensitivity(item.Sensitivity, item.IsSensitive)
	if err != nil || sensitivity != domain.ConfigSensitivityNormal {
		return nil, apperrors.Forbidden("配置资产读取策略不允许当前身份访问")
	}
	if !s.canReadConfigForClientList(actor, item, group) {
		// INTERNAL assets intentionally do not gain a generic authenticated
		// read path. A logged-in administrator or a configuration-scope reader
		// may inspect them; all other callers receive the same policy result as
		// the typed config read API.
		access, accessErr := s.resolveConfigAccess(ctx, actor, item.GroupCode, item.ConfigKey)
		if accessErr != nil {
			return nil, accessErr
		}
		if !actor.IsAdmin && !access.CanRead {
			if exposure == domain.ConfigExposureAuthenticated && !actor.Authenticated {
				return nil, apperrors.Unauthorized("该配置需要登录后访问")
			}
			return nil, apperrors.Forbidden("配置资产读取策略不允许当前身份访问")
		}
	}
	if s.assets == nil {
		return nil, apperrors.System("配置资产服务未配置")
	}
	result, err := s.assets.OpenConfigAsset(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	if result == nil || result.Reader == nil || result.AssetType != assetType || result.AccessScope != configAssetExposure(exposure) {
		if result != nil && result.Reader != nil {
			_ = result.Reader.Close()
		}
		return nil, apperrors.Operation("配置资产绑定状态无效")
	}
	return result, nil
}

func (s *Service) GetConfigPage(ctx context.Context, actor Actor, request configfacade.ConfigQueryRequest) (*configfacade.PageResult[configfacade.ConfigVO], error) {
	current, size := s.domain.NormalizePage(firstPositiveInt64(request.Current, request.PageNum), request.PageSize)
	var result *configfacade.PageResult[configfacade.ConfigVO]
	if err := s.withReadOnlySnapshot(ctx, func(snapshotCtx context.Context) error {
		page, err := s.repo.QueryConfigs(snapshotCtx, domain.ConfigPageQuery{
			Current:    current,
			PageSize:   size,
			GroupID:    request.GroupID,
			Keyword:    strings.TrimSpace(request.Keyword),
			SearchText: strings.TrimSpace(request.SearchText),
			SearchType: strings.TrimSpace(request.SearchType),
			IsEnabled:  request.IsEnabled,
		})
		if err != nil {
			return err
		}
		accessGrants, err := s.loadConfigAccessGrants(snapshotCtx, actor)
		if err != nil {
			return err
		}
		records := make([]configfacade.ConfigVO, 0, len(page.Records))
		for idx := range page.Records {
			item := &page.Records[idx]
			access := s.resolveConfigAccessFromGrants(actor, accessGrants, item.GroupCode, item.ConfigKey)
			if !actor.IsAdmin && !access.CanRead {
				continue
			}
			records = append(records, *s.toConfigVO(snapshotCtx, item, access))
		}
		result = &configfacade.PageResult[configfacade.ConfigVO]{
			Current: page.Current,
			Size:    page.Size,
			Total:   filteredTotal(page.Total, len(page.Records), len(records), actor),
			Records: records,
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) ChangeEnabled(ctx context.Context, actor Actor, id int64, request configfacade.ConfigEnabledRequest) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	if request.IsEnabled == nil {
		return apperrors.Params("启用状态不能为空")
	}
	if err := s.domain.ValidateStatus(*request.IsEnabled); err != nil {
		return err
	}
	item, err := s.repo.FindConfigByID(ctx, id)
	if err != nil {
		return err
	}
	if item == nil || item.IsDeleted == 1 {
		return apperrors.NotFound("配置项不存在")
	}
	if err := s.requireConfigWriteAccess(ctx, actor, item.GroupCode, item.ConfigKey); err != nil {
		return err
	}
	now := time.Now().UTC()
	item.IsEnabled = *request.IsEnabled
	item.UpdatedBy = actor.UserID
	item.UpdateTime = &now
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateConfig(txCtx, item)
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) RevealSensitiveValue(ctx context.Context, actor Actor, id int64, request configfacade.ConfigSensitiveRevealRequest) (*configfacade.ConfigSensitiveRevealResponse, error) {
	if id <= 0 {
		return nil, apperrors.Params("配置ID不能为空")
	}
	if err := stepup.Require(actor.StepUpProof, stepUpActionConfigSensitiveReveal, sensitiveRevealBinding(id)); err != nil {
		return nil, err
	}
	item, err := s.repo.FindConfigByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.IsDeleted == 1 {
		return nil, apperrors.NotFound("配置项不存在")
	}
	if err := s.requireConfigReadAccess(ctx, actor, item.GroupCode, item.ConfigKey); err != nil {
		return nil, err
	}
	sensitivity, err := domain.NormalizeSensitivity(item.Sensitivity, item.IsSensitive)
	if err != nil {
		return nil, apperrors.Operation("配置敏感级别无效")
	}
	if sensitivity == domain.ConfigSensitivitySecret {
		return nil, apperrors.Forbidden("SECRET 配置为管理 HTTP write-only")
	}
	if sensitivity != domain.ConfigSensitivitySensitive {
		return nil, apperrors.Params("该配置不是敏感配置")
	}
	runtimeValue, err := s.resolveRuntimeValue(ctx, item)
	if err != nil {
		return nil, err
	}
	if s.reveal == nil {
		return nil, apperrors.Operation("敏感配置回显失败")
	}
	encryptedValue, err := s.reveal.EncryptForClient(request.ObfuscatedClientPublicKey, runtimeValue)
	if err != nil {
		return nil, apperrors.Operation("敏感配置回显失败")
	}
	return &configfacade.ConfigSensitiveRevealResponse{EncryptedValue: encryptedValue}, nil
}

func (s *Service) GetConfigByKey(ctx context.Context, configKey string) (*configfacade.ConfigValueDTO, error) {
	return nil, apperrors.Forbidden("内部配置读取必须提供 consumerId、fully-qualified key、server scope、purpose 与 allowed sensitivity")
}

func (s *Service) GetConfigByKeyForClient(ctx context.Context, actor Actor, configKey string) (*configfacade.ConfigValueDTO, error) {
	configKey = strings.TrimSpace(configKey)
	if classified, ok := s.cache.(classifiedCacheStore); ok && classified.ClassifiedEnabled() {
		if request, eligible := cachepolicy.ConfigReadRequest(configKey, cacheRequestScope(actor), cacheBusinessIdentity(actor)); eligible {
			// The catalog only admits data that is public independently of the
			// caller. Resolve the parent directly before accepting an L1/L2
			// record: a PUBLIC row inside a permission-gated group remains
			// permission-derived and must take the normal authorization path.
			parentEligible, parentErr := s.classifiedPublicParentEligible(ctx, configKey)
			if parentErr != nil {
				return nil, parentErr
			}
			if !parentEligible {
				goto authoritative
			}
			var cached configValueCacheRecord
			found, err := classified.GetOrLoadClassified(ctx, request, &cached, func(loadCtx context.Context) (cachepolicy.CacheableValue, error) {
				item, loadErr := s.getConfigByKeyInternal(loadCtx, configKey, false, actor)
				if loadErr != nil || item == nil {
					return cachepolicy.CacheableValue{}, loadErr
				}
				value, valueErr := s.toConfigValueCacheRecord(loadCtx, item)
				if valueErr != nil {
					return cachepolicy.CacheableValue{}, valueErr
				}
				parent, parentLoadErr := s.loadClassifiedPublicParent(loadCtx, item.GroupCode)
				if parentLoadErr != nil {
					return cachepolicy.CacheableValue{}, parentLoadErr
				}
				sensitivity, sensitivityErr := domain.NormalizeSensitivity(item.Sensitivity, item.IsSensitive)
				if sensitivityErr != nil {
					return cachepolicy.CacheableValue{Value: value, Cacheable: false}, nil
				}
				value.CataloguedPublic = classifiedPublicConfigEligible(item, parent)
				return cachepolicy.CacheableValue{
					Value: value,
					Cacheable: value.CataloguedPublic && cachepolicy.ValidateLoaded(request, item.Exposure, string(sensitivity), item.SchemaVersion,
						item.IsEnabled == 1 && item.IsDeleted == 0),
				}, nil
			})
			if err != nil {
				return nil, err
			}
			if found && cached.CataloguedPublic {
				value, valueErr := cached.toDTO()
				if valueErr != nil {
					return nil, apperrors.Operation("配置缓存值不符合声明的标量契约")
				}
				return value, nil
			}
		}
	}

authoritative:
	item, err := s.getConfigByKeyInternal(ctx, configKey, false, actor)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("配置不存在或未启用：" + configKey)
	}
	return s.toConfigValueDTO(ctx, item)
}

// classifiedPublicParentEligible deliberately bypasses the legacy group cache
// because it is an authorization fact, not a cache value. The governed layer
// acquires its source-adjacent freshness fence before any candidate payload is
// used; this direct parent check keeps a permission-gated group out of that
// candidate path even if an old record exists in L1/L2.
func (s *Service) classifiedPublicParentEligible(ctx context.Context, fullyQualifiedKey string) (bool, error) {
	parts := strings.SplitN(strings.TrimSpace(fullyQualifiedKey), ".", 2)
	if len(parts) != 2 || s.domain.NormalizeGroupCode(parts[0]) == "" || s.domain.NormalizeConfigKey(parts[1]) == "" {
		return false, nil
	}
	parent, err := s.loadClassifiedPublicParent(ctx, parts[0])
	if err != nil {
		return false, err
	}
	return parent != nil && parent.Status == 1 && parent.IsDeleted == 0 && strings.TrimSpace(parent.PermissionCode) == "", nil
}

func (s *Service) loadClassifiedPublicParent(ctx context.Context, groupCode string) (*domain.ConfigGroup, error) {
	if s == nil || s.repo == nil {
		return nil, apperrors.System("配置分类缓存权限校验不可用")
	}
	groupCode = s.domain.NormalizeGroupCode(groupCode)
	if groupCode == "" {
		return nil, nil
	}
	return s.repo.FindGroupByCode(ctx, groupCode)
}

func classifiedPublicConfigEligible(item *domain.Config, parent *domain.ConfigGroup) bool {
	return item != nil && parent != nil && parent.Status == 1 && parent.IsDeleted == 0 &&
		strings.TrimSpace(parent.PermissionCode) == "" && item.RequiredLogin == 0 && item.IsSystemConfig == 0
}

func cacheRequestScope(actor Actor) string {
	if scope := strings.TrimSpace(actor.ScopeID); scope != "" {
		return "client:" + scope
	}
	return "public:global"
}

func cacheBusinessIdentity(actor Actor) string {
	if !actor.Authenticated {
		return "anonymous"
	}
	return "account:" + strconv.FormatInt(actor.AccountID, 10) + ":authz:" + strconv.FormatInt(actor.AuthzVersion, 10)
}

func (s *Service) ListConfigsForClient(ctx context.Context, actor Actor, request configfacade.ConfigClientListRequest) (map[string]configfacade.ConfigValueDTO, error) {
	groupCode := s.domain.NormalizeGroupCode(request.GroupCode)
	if groupCode == "" {
		return nil, apperrors.Params("groupCode不能为空")
	}
	group, err := s.loadGroupByCode(ctx, groupCode)
	if err != nil {
		return nil, err
	}
	if group == nil || group.Status != 1 || group.IsDeleted == 1 {
		return map[string]configfacade.ConfigValueDTO{}, nil
	}
	items, hit, err := s.cache.GetListByGroup(ctx, group.ID)
	if err != nil {
		return nil, err
	}
	if !hit {
		items, err = s.loadEnabledConfigsByGroup(ctx, group.ID)
		if err != nil {
			return nil, err
		}
		if err := s.cache.SetListByGroup(ctx, group.ID, items); err != nil {
			return nil, err
		}
	}
	result := make(map[string]configfacade.ConfigValueDTO)
	for idx := range items {
		item := &items[idx]
		item.GroupCode = group.GroupCode
		item.GroupName = group.GroupName
		if !s.canReadConfigForClientList(actor, item, group) {
			continue
		}
		dto, err := s.toConfigValueDTO(ctx, item)
		if err != nil {
			return nil, err
		}
		result[item.FullyQualifiedKey(group)] = *dto
	}
	return result, nil
}

func (s *Service) GetConfigBatch(ctx context.Context, request configfacade.ConfigBatchRequest) (map[string]configfacade.ConfigValueDTO, error) {
	return nil, apperrors.Forbidden("内部批量配置读取不得使用无身份接口")
}

func (s *Service) GetConfigForConsumer(ctx context.Context, request configfacade.ConfigInternalReadRequest) (*configfacade.ConfigValueDTO, error) {
	result, err := s.GetConfigBatchForConsumer(ctx, configfacade.ConfigInternalBatchReadRequest{
		ConsumerID:         request.ConsumerID,
		FullyQualifiedKeys: []string{request.FullyQualifiedKey},
		ServerScope:        request.ServerScope,
		Purpose:            request.Purpose,
		AllowedSensitivity: request.AllowedSensitivity,
	})
	if err != nil {
		return nil, err
	}
	item, ok := result[strings.TrimSpace(request.FullyQualifiedKey)]
	if !ok {
		return nil, apperrors.NotFound("配置不存在或未启用")
	}
	return &item, nil
}

func (s *Service) GetConfigBatchForConsumer(ctx context.Context, request configfacade.ConfigInternalBatchReadRequest) (map[string]configfacade.ConfigValueDTO, error) {
	consumerID, serverScope, purpose, allowed, err := normalizeInternalReadIdentity(
		request.ConsumerID,
		request.ServerScope,
		request.Purpose,
		request.AllowedSensitivity,
	)
	if err != nil {
		return nil, err
	}
	keys := canonicalConfigKeys(request.FullyQualifiedKeys)
	if len(keys) == 0 || len(keys) > 50 {
		return nil, apperrors.Params("内部批量读取必须包含1到50个 fully-qualified key")
	}
	for _, key := range keys {
		if !strings.Contains(key, ".") {
			return nil, apperrors.Params("配置读取必须使用 fully-qualified key")
		}
	}
	cacheKey, err := s.internalConsumerCacheKey(ctx, consumerID, serverScope, purpose, allowed, keys)
	if err != nil {
		return nil, err
	}
	if cached, hit, cacheErr := s.cache.GetBatch(ctx, cacheKey); cacheErr == nil && hit {
		return s.toConfigValueMap(ctx, cached)
	} else if cacheErr != nil {
		return nil, cacheErr
	}
	registrations := make(map[string]domain.ConsumerRegistration, len(keys))
	s.consumersMu.RLock()
	for _, key := range keys {
		registration, ok := s.internalConsumers[consumerID+"\x00"+key]
		if !ok {
			s.consumersMu.RUnlock()
			return nil, apperrors.Forbidden("配置消费者未注册")
		}
		registrations[key] = registration
	}
	s.consumersMu.RUnlock()
	var resolved map[string]domain.Config
	if err := s.withReadOnlySnapshot(ctx, func(snapshotCtx context.Context) error {
		var groupsByID map[int64]domain.ConfigGroup
		var resolveErr error
		resolved, groupsByID, resolveErr = s.resolveFullyQualifiedConfigsBatch(snapshotCtx, keys)
		if resolveErr != nil {
			return resolveErr
		}
		for _, key := range keys {
			item, ok := resolved[key]
			if !ok {
				return apperrors.NotFound("配置不存在或未启用")
			}
			registration := registrations[key]
			group := groupsByID[item.GroupID]
			if !domain.CanReadConfig(&item, &group, domain.ConfigReadContext{
				Identity: domain.ConfigReadInternal, ConsumerID: consumerID, ScopeID: serverScope,
				Purpose: purpose, AllowedSecret: allowed,
			}, &registration) {
				return apperrors.Forbidden("配置消费者声明与注册策略不匹配")
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := s.cache.SetBatch(ctx, cacheKey, resolved); err != nil {
		return nil, err
	}
	return s.toConfigValueMap(ctx, resolved)
}

func (s *Service) ListConfigsForConsumer(ctx context.Context, request configfacade.ConfigInternalListRequest) (map[string]configfacade.ConfigValueDTO, error) {
	consumerID, serverScope, purpose, allowed, err := normalizeInternalReadIdentity(
		request.ConsumerID,
		request.ServerScope,
		request.Purpose,
		request.AllowedSensitivity,
	)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0)
	s.consumersMu.RLock()
	for _, registration := range s.internalConsumers {
		if registration.ConsumerID == consumerID &&
			registration.ScopeID == serverScope &&
			registration.Purpose == purpose &&
			sensitivityWithin(registration.AllowedSecret, allowed) {
			keys = append(keys, registration.FullyQualifiedKey)
		}
	}
	s.consumersMu.RUnlock()
	sort.Strings(keys)
	if len(keys) == 0 {
		return nil, apperrors.Forbidden("配置消费者未注册")
	}
	return s.GetConfigBatchForConsumer(ctx, configfacade.ConfigInternalBatchReadRequest{
		ConsumerID:         consumerID,
		FullyQualifiedKeys: keys,
		ServerScope:        serverScope,
		Purpose:            purpose,
		AllowedSensitivity: string(allowed),
	})
}

func (s *Service) ListConfigConsumers(context.Context) []configfacade.ConfigConsumerVO {
	return []configfacade.ConfigConsumerVO{
		{ConsumerID: "frontend.login-brand-text", FullyQualifiedKey: "sysPlatform.brandJson", ServerScope: "browser", Purpose: "render login brand text only", AllowedSensitivity: "NORMAL", Source: "sysPlatform.brandJson", ActualConsumer: "seven-framework-web login page", Activation: "realtime", CacheRule: "platform store lifecycle", Connected: true},
		{ConsumerID: "frontend.login-logo", FullyQualifiedKey: "SEVEN_FRONTEND_METADATA.loginLogo", ServerScope: "browser", Purpose: "render bound CONFIG_ASSET login logo", AllowedSensitivity: "NORMAL", Source: "sys_config", ActualConsumer: "seven-framework-web login page", Activation: "realtime", CacheRule: "account/scope/authz generation; stable config-asset path", Connected: true},
		{ConsumerID: "frontend.favicon", FullyQualifiedKey: "SEVEN_FRONTEND_METADATA.favicon", ServerScope: "browser", Purpose: "set bound CONFIG_ASSET browser favicon", AllowedSensitivity: "NORMAL", Source: "sys_config", ActualConsumer: "seven-framework-web RuntimeBrandAssets", Activation: "realtime", CacheRule: "account/scope/authz generation; stable config-asset path", Connected: true},
		{ConsumerID: "frontend.shell-title", FullyQualifiedKey: "SEVEN_FRONTEND_METADATA.title", ServerScope: "browser", Purpose: "render shell title", AllowedSensitivity: "NORMAL", Source: "sys_config", ActualConsumer: "seven-framework-web GlobalLayoutShell", Activation: "realtime", CacheRule: "account/scope/authz generation", Connected: true},
		{ConsumerID: "backend.login-security", FullyQualifiedKey: "application.login.*", ServerScope: "server:local", Purpose: "login risk policy", AllowedSensitivity: "NORMAL", Source: "application.yml", ActualConsumer: "authorization runtime", Activation: "restart", CacheRule: "startup configuration", Connected: true},
		{ConsumerID: "frontend.shell-short-title", FullyQualifiedKey: "SEVEN_FRONTEND_METADATA.shortTitle", ServerScope: "browser", Purpose: "render compact shell title", AllowedSensitivity: "NORMAL", Source: "sys_config", ActualConsumer: "seven-framework-web GlobalLayoutShell collapsed title", Activation: "realtime", CacheRule: "account/scope/authz generation", Connected: true},
		{ConsumerID: "frontend.theme-primary", FullyQualifiedKey: "SEVEN_FRONTEND_METADATA.themePrimaryColor", ServerScope: "browser", Purpose: "select finite theme token", AllowedSensitivity: "NORMAL", Source: "sys_config", ActualConsumer: "seven-framework-web GlobalLayoutShell nested ConfigProvider", Activation: "realtime", CacheRule: "account/scope/authz generation", Connected: true},
	}
}

func (s *Service) GetConfigBatchForClient(ctx context.Context, actor Actor, request configfacade.ConfigBatchRequest) (map[string]configfacade.ConfigValueDTO, error) {
	return s.getConfigBatchInternal(ctx, request, false, actor)
}

func (s *Service) ApplyPendingConfigs(ctx context.Context, actor Actor, isStartup bool) (int, error) {
	if !isStartup {
		if actor.UserID <= 0 {
			return 0, apperrors.Unauthorized("未登录或登录信息失效")
		}
		if err := stepup.Require(actor.StepUpProof, stepUpActionConfigApplyPending, "config:apply-pending"); err != nil {
			return 0, err
		}
	}
	pendingLogs, err := s.repo.ListPendingLogs(ctx)
	if err != nil {
		return 0, err
	}
	if len(pendingLogs) == 0 {
		return 0, nil
	}
	if len(pendingLogs) > 500 {
		return 0, apperrors.ObjectState("待应用配置数量超过单次处理上限")
	}
	latest := make(map[int64]domain.ConfigChangeLog, len(pendingLogs))
	for _, item := range pendingLogs {
		existing, ok := latest[item.ConfigID]
		if !ok || changeLogLater(item, existing) {
			latest[item.ConfigID] = item
		}
	}
	configIDs := make([]int64, 0, len(latest))
	for configID := range latest {
		configIDs = append(configIDs, configID)
	}
	sort.Slice(configIDs, func(i, j int) bool { return configIDs[i] < configIDs[j] })
	configs, err := s.repo.ListConfigsByIDs(ctx, configIDs)
	if err != nil {
		return 0, err
	}
	configByID := make(map[int64]domain.Config, len(configs))
	for _, item := range configs {
		configByID[item.ID] = item
	}
	var writeGrants []domain.ConfigScopeGrant
	if !isStartup && !actor.IsAdmin {
		roleIDs := dedupePositiveIDs(actor.RoleIDs)
		if len(roleIDs) > 0 {
			writeGrants, err = s.repo.ListConfigScopeGrantsByRoleIDs(ctx, roleIDs)
			if err != nil {
				return 0, err
			}
		}
	}
	operatorID := actor.UserID
	if isStartup || operatorID <= 0 {
		operatorID = 0
	}
	operatorName := actorName(actor)
	if isStartup {
		operatorName = "系统"
	}
	prepared := make([]domain.PendingConfigApply, 0, len(configIDs))
	for _, configID := range configIDs {
		pendingLog := latest[configID]
		configValue, exists := configByID[configID]
		if !exists || configValue.IsDeleted == 1 {
			continue
		}
		if !isStartup && !actor.IsAdmin && !s.configWriteAllowed(writeGrants, configValue.GroupCode, configValue.ConfigKey) {
			continue
		}
		now := time.Now().UTC()
		configItem := configValue
		currentValue, err := s.resolveRuntimeValue(ctx, &configItem)
		if err != nil {
			return 0, err
		}
		pendingValue := pendingLog.NewValue
		if err := s.prepareConfigValueStorage(ctx, &configItem, pendingValue); err != nil {
			return 0, err
		}
		configItem.UpdatedBy = operatorID
		configItem.UpdateTime = &now
		parentID := pendingLog.ID
		applyLog := s.domain.BuildChangeLog(domain.CreateChangeLogInput{
			ConfigID:        configItem.ID,
			ConfigKey:       configItem.ConfigKey,
			OperationType:   domain.ConfigOperationApply,
			OldValue:        protectedAuditValue(&configItem, currentValue),
			NewValue:        protectedAuditValue(&configItem, pendingValue),
			EffectType:      pendingLog.EffectType,
			ParentLogID:     &parentID,
			OperatorID:      operatorID,
			OperationReason: ternary(isStartup, "系统启动自动应用", "手动触发应用"),
			IsStartup:       isStartup,
			Now:             now,
		})
		applyLog.OperatorName = operatorName
		appliedBy := operatorID
		applyLog.AppliedBy = &appliedBy
		applyLog.AppliedTime = &now
		prepared = append(prepared, domain.PendingConfigApply{
			PendingLogID: pendingLog.ID,
			Config:       configItem,
			ApplyLog:     *applyLog,
		})
	}
	appliedCount := 0
	for start := 0; start < len(prepared); start += pendingConfigApplyChunkSize {
		end := start + pendingConfigApplyChunkSize
		if end > len(prepared) {
			end = len(prepared)
		}
		chunk := prepared[start:end]
		var claimedIDs []int64
		if err := s.withConfigInvalidationTxIf(ctx, func(txCtx context.Context) (bool, error) {
			var batchErr error
			claimedIDs, batchErr = s.repo.ApplyPendingConfigBatch(txCtx, chunk)
			return len(claimedIDs) > 0, batchErr
		}); err != nil {
			return appliedCount, err
		}
		if len(claimedIDs) == 0 {
			continue
		}
		claimedSet := make(map[int64]struct{}, len(claimedIDs))
		for _, id := range claimedIDs {
			claimedSet[id] = struct{}{}
		}
		appliedConfigs := make([]domain.Config, 0, len(claimedIDs))
		for _, item := range chunk {
			if _, ok := claimedSet[item.PendingLogID]; ok {
				appliedConfigs = append(appliedConfigs, item.Config)
			}
		}
		appliedCount += len(appliedConfigs)
	}
	return appliedCount, nil
}

func (s *Service) configWriteAllowed(grants []domain.ConfigScopeGrant, groupCode, configKey string) bool {
	groupCode = s.domain.NormalizeGroupCode(groupCode)
	configKey = s.domain.NormalizeConfigKey(configKey)
	for _, grant := range grants {
		if grant.CanWrite == 0 || s.domain.NormalizeGroupCode(grant.GroupCode) != groupCode {
			continue
		}
		grantKey := s.domain.NormalizeConfigKey(grant.ConfigKey)
		if grantKey == "" || grantKey == configKey {
			return true
		}
	}
	return false
}

func (s *Service) GetPendingConfigs(ctx context.Context, actor Actor) ([]configfacade.PendingConfigVO, error) {
	pendingLogs, err := s.repo.ListPendingLogs(ctx)
	if err != nil {
		return nil, err
	}
	if len(pendingLogs) == 0 {
		return []configfacade.PendingConfigVO{}, nil
	}
	if len(pendingLogs) > 500 {
		return nil, apperrors.ObjectState("待应用配置数量超过单次读取上限")
	}
	latest := make(map[int64]domain.ConfigChangeLog, len(pendingLogs))
	for _, item := range pendingLogs {
		existing, ok := latest[item.ConfigID]
		if !ok || changeLogLater(item, existing) {
			latest[item.ConfigID] = item
		}
	}
	configIDs := make([]int64, 0, len(latest))
	for configID := range latest {
		configIDs = append(configIDs, configID)
	}
	configs, err := s.repo.ListConfigsByIDs(ctx, configIDs)
	if err != nil {
		return nil, err
	}
	configMap := make(map[int64]domain.Config, len(configs))
	for _, item := range configs {
		configMap[item.ID] = item
	}
	userIDs := make([]int64, 0, len(latest))
	for _, logItem := range latest {
		if logItem.OperatorID > 0 {
			userIDs = append(userIDs, logItem.OperatorID)
		}
	}
	nameMap, _ := s.findNicknames(ctx, userIDs)
	maskAllValues := !actor.HasPermission("system:config:sensitive")
	result := make([]configfacade.PendingConfigVO, 0, len(latest))
	for configID, logItem := range latest {
		configItem, ok := configMap[configID]
		if !ok {
			continue
		}
		access, accessErr := s.resolveConfigAccess(ctx, actor, configItem.GroupCode, configItem.ConfigKey)
		if accessErr != nil {
			return nil, accessErr
		}
		if !access.CanRead {
			continue
		}
		currentValue, _ := s.resolveRuntimeValue(ctx, &configItem)
		currentValue = s.maskLogValue(currentValue, configItem.IsSensitive, maskAllValues)
		pendingValue := s.maskLogValue(logItem.NewValue, configItem.IsSensitive, maskAllValues)
		result = append(result, configfacade.PendingConfigVO{
			LogID:         logItem.ID,
			ConfigID:      configID,
			ConfigKey:     logItem.ConfigKey,
			ConfigDesc:    configItem.ConfigDesc,
			CurrentValue:  currentValue,
			PendingValue:  pendingValue,
			CreatedBy:     logItem.OperatorID,
			CreatedByName: lookupOperatorName(nameMap, logItem.OperatorID),
			CreateTime:    logItem.OperationTime,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return timePtrBefore(result[i].CreateTime, result[j].CreateTime)
	})
	return result, nil
}

func (s *Service) GetConfigChangeHistory(ctx context.Context, actor Actor, configID int64, limit int) ([]configfacade.ConfigChangeLogVO, error) {
	if configID <= 0 {
		return nil, apperrors.Params("配置ID不能为空")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	configItem, err := s.repo.FindConfigByID(ctx, configID)
	if err != nil {
		return nil, err
	}
	if configItem == nil || configItem.IsDeleted == 1 {
		return nil, apperrors.NotFound("配置项不存在")
	}
	if err := s.requireConfigReadAccess(ctx, actor, configItem.GroupCode, configItem.ConfigKey); err != nil {
		return nil, err
	}
	logs, err := s.repo.ListHistoryByConfigID(ctx, configID, limit)
	if err != nil {
		return nil, err
	}
	return s.toChangeLogVOs(ctx, logs, !actor.HasPermission("system:config:sensitive"))
}

func (s *Service) RollbackConfigChange(ctx context.Context, actor Actor, logID int64, reason string) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	if logID <= 0 {
		return apperrors.Params("日志ID不能为空")
	}
	if err := stepup.Require(actor.StepUpProof, stepUpActionConfigRollback, configRollbackBinding(logID)); err != nil {
		return err
	}
	changeLog, err := s.repo.FindChangeLogByID(ctx, logID)
	if err != nil {
		return err
	}
	if changeLog == nil {
		return apperrors.NotFound("变更日志不存在")
	}
	if changeLog.Status != string(domain.ConfigStatusApplied) {
		return apperrors.Params("只能回滚已应用的配置变更，当前状态：" + changeLog.Status)
	}
	configItem, err := s.repo.FindConfigByID(ctx, changeLog.ConfigID)
	if err != nil {
		return err
	}
	if configItem == nil || configItem.IsDeleted == 1 {
		return apperrors.NotFound("配置项不存在或已删除")
	}
	if err := s.requireConfigWriteAccess(ctx, actor, configItem.GroupCode, configItem.ConfigKey); err != nil {
		return err
	}
	sensitivity, sensitivityErr := domain.NormalizeSensitivity(configItem.Sensitivity, configItem.IsSensitive)
	if sensitivityErr != nil {
		return apperrors.Operation("配置敏感级别无效，无法回滚")
	}
	if sensitivity != domain.ConfigSensitivityNormal || changeLog.OldValueProtected || changeLog.NewValueProtected {
		return apperrors.Operation("受保护配置不支持基于审计历史回滚，请提交新的配置值")
	}
	currentValue, err := s.resolveRuntimeValue(ctx, configItem)
	if err != nil {
		return err
	}
	if currentValue != changeLog.NewValue {
		return apperrors.Params("配置值已被修改，无法回滚")
	}
	currentAssetType, currentIsAsset := configAssetTypeMust(configItem.ValueType)
	oldAssetSnapshot, newAssetSnapshot, snapshotErr := changeLog.PrivateAssetSnapshots()
	if snapshotErr != nil {
		return apperrors.Operation("配置资产历史快照无效，不能回滚")
	}
	assetRollback := oldAssetSnapshot != nil || newAssetSnapshot != nil
	var restoreCommand *filefacade.RestoreConfigAssetBindingCommand
	if currentIsAsset {
		// Stable config paths intentionally do not distinguish A from B. A
		// typed IMAGE/FILE rollback therefore requires the paired private
		// binding snapshot rather than trying to infer the former file from a
		// value, a reference timestamp, or a request-provided fileId.
		if !assetRollback || oldAssetSnapshot == nil || newAssetSnapshot == nil {
			return apperrors.Operation("配置资产历史记录缺少可恢复私有快照，不能回滚")
		}
		currentExposure, exposureErr := domain.NormalizeExposure(configItem.Exposure)
		if exposureErr != nil {
			return apperrors.Operation("配置资产暴露级别无效，不能回滚")
		}
		expectedState, expectedErr := configAssetBindingStateFromAuditSnapshot(newAssetSnapshot)
		if expectedErr != nil {
			return expectedErr
		}
		restoreState, restoreErr := configAssetBindingStateFromAuditSnapshot(oldAssetSnapshot)
		if restoreErr != nil {
			return restoreErr
		}
		currentExposureValue := configAssetExposure(currentExposure)
		if expectedState.ConfigID != configItem.ID || restoreState.ConfigID != configItem.ID ||
			expectedState.AssetType != currentAssetType || restoreState.AssetType != currentAssetType ||
			expectedState.Exposure != currentExposureValue {
			return apperrors.Operation("配置资产类型或读取策略已变化，不能安全回滚历史记录")
		}
		if err := validateConfigAssetSnapshotRuntimeValue(expectedState, configItem.ID, changeLog.NewValue); err != nil {
			return err
		}
		if err := validateConfigAssetSnapshotRuntimeValue(restoreState, configItem.ID, changeLog.OldValue); err != nil {
			return err
		}
		if s.assets == nil {
			return apperrors.System("配置资产服务未配置")
		}
		restoreCommand = &filefacade.RestoreConfigAssetBindingCommand{
			ConfigID: configItem.ID, AssetType: currentAssetType, Exposure: currentExposureValue,
			Expected: expectedState, Restore: restoreState,
		}
	} else if assetRollback {
		// A value-type transition cannot be reconstructed from a binding
		// snapshot alone. Refuse safely rather than recreating an IMAGE/FILE
		// reference below a scalar config or guessing metadata.
		return apperrors.Operation("配置资产当前类型已变化，不能安全回滚历史记录")
	}
	rollbackValue := changeLog.OldValue
	now := time.Now().UTC()
	if err := s.withConfigInvalidationTx(ctx, func(txCtx context.Context) error {
		if restoreCommand != nil {
			// The restore snapshot is the only trusted source for a historic
			// CONFIG_ASSET exposure. This restores a PUBLIC/AUTHENTICATED/INTERNAL
			// policy change together with sys_file_reference rather than leaving
			// the stable route and its persisted policy out of sync.
			configItem.Exposure = string(restoreCommand.Restore.Exposure)
		}
		if err := s.prepareConfigValueStorage(txCtx, configItem, rollbackValue); err != nil {
			return err
		}
		configItem.UpdatedBy = actor.UserID
		configItem.UpdateTime = &now
		if err := s.repo.UpdateConfig(txCtx, configItem); err != nil {
			return err
		}
		if restoreCommand != nil {
			// This is not a normal upload bind. The command is derived solely
			// from the private audit pair after current write access and a
			// log-bound step-up proof have both been checked above.
			if err := s.assets.RestoreConfigAssetBinding(txCtx, *restoreCommand); err != nil {
				return err
			}
		}
		changeLog.Status = string(domain.ConfigStatusRolledBack)
		if err := s.repo.UpdateChangeLog(txCtx, changeLog); err != nil {
			return err
		}
		parentID := changeLog.ID
		rollbackLog := s.domain.BuildChangeLog(domain.CreateChangeLogInput{
			ConfigID:         configItem.ID,
			ConfigKey:        configItem.ConfigKey,
			OperationType:    domain.ConfigOperationRollback,
			OldValue:         protectedAuditValue(configItem, currentValue),
			NewValue:         protectedAuditValue(configItem, rollbackValue),
			EffectType:       changeLog.EffectType,
			ParentLogID:      &parentID,
			RelatedLogID:     changeLog.ParentLogID,
			OperatorID:       actor.UserID,
			OperationReason:  strings.TrimSpace(reason),
			OldAssetSnapshot: configAssetAuditSnapshotOrNil(restoreCommand, true),
			NewAssetSnapshot: configAssetAuditSnapshotOrNil(restoreCommand, false),
			Now:              now,
		})
		rollbackLog.Status = string(domain.ConfigStatusApplied)
		rollbackLog.OperatorName = actorName(actor)
		appliedBy := actor.UserID
		rollbackLog.AppliedBy = &appliedBy
		rollbackLog.AppliedTime = &now
		if _, err := s.repo.InsertChangeLog(txCtx, rollbackLog); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) GetOperationChain(ctx context.Context, actor Actor, logID int64) ([]configfacade.ConfigChangeLogVO, error) {
	if logID <= 0 {
		return nil, apperrors.Params("日志ID不能为空")
	}
	logs, err := s.buildOperationChain(ctx, logID)
	if err != nil {
		return nil, err
	}
	logs, err = s.filterReadableChangeLogs(ctx, actor, logs)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 && !actor.IsAdmin {
		return nil, apperrors.DataScopeDenied("配置范围不足")
	}
	return s.toChangeLogVOs(ctx, logs, !actor.HasPermission("system:config:sensitive"))
}

func (s *Service) GetAuditLogs(ctx context.Context, actor Actor, request configfacade.AuditLogQueryRequest) ([]configfacade.ConfigChangeLogVO, error) {
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	logs, err := s.repo.ListAuditLogs(ctx, domain.AuditLogQuery{
		ConfigID:      request.ConfigID,
		OperationType: strings.TrimSpace(request.OperationType),
		Status:        strings.TrimSpace(request.Status),
		StartTime:     request.StartTime,
		EndTime:       request.EndTime,
		Limit:         limit,
	})
	if err != nil {
		return nil, err
	}
	logs, err = s.filterReadableChangeLogs(ctx, actor, logs)
	if err != nil {
		return nil, err
	}
	return s.toChangeLogVOs(ctx, logs, !actor.HasPermission("system:config:sensitive"))
}

func (s *Service) GetRoleConfigScopes(ctx context.Context, actor Actor, roleID int64) ([]configfacade.ConfigScopeGrantVO, error) {
	if actor.UserID <= 0 {
		return nil, apperrors.Unauthorized("未登录或登录信息失效")
	}
	if roleID <= 0 {
		return nil, apperrors.Params("roleId不能为空")
	}
	grants, err := s.repo.ListConfigScopeGrantsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	result := make([]configfacade.ConfigScopeGrantVO, 0, len(grants))
	for _, item := range grants {
		result = append(result, configfacade.ConfigScopeGrantVO{
			GroupCode: item.GroupCode,
			ConfigKey: item.ConfigKey,
			CanRead:   normalizeAccessFlag(item.CanRead),
			CanWrite:  normalizeAccessFlag(item.CanWrite),
			CanDelete: normalizeAccessFlag(item.CanDelete),
		})
	}
	return result, nil
}

// ListRoleConfigScopes exposes config grants to the authorization-owned atomic role grant policy.
func (s *Service) ListRoleConfigScopes(ctx context.Context, roleID int64) ([]authorizationfacade.RoleConfigScopeGrantVO, error) {
	if roleID <= 0 {
		return nil, apperrors.Params("roleId不能为空")
	}
	grants, err := s.repo.ListConfigScopeGrantsByRoleID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	result := make([]authorizationfacade.RoleConfigScopeGrantVO, 0, len(grants))
	for _, item := range grants {
		result = append(result, authorizationfacade.RoleConfigScopeGrantVO{
			GroupCode: item.GroupCode, ConfigKey: item.ConfigKey,
			CanRead: normalizeAccessFlag(item.CanRead), CanWrite: normalizeAccessFlag(item.CanWrite), CanDelete: normalizeAccessFlag(item.CanDelete),
		})
	}
	return result, nil
}

// NormalizeRoleConfigScopes validates and normalizes grants without mutating state.
func (s *Service) NormalizeRoleConfigScopes(ctx context.Context, values []authorizationfacade.RoleConfigScopeGrantVO) ([]authorizationfacade.RoleConfigScopeGrantVO, error) {
	grants := make([]configfacade.ConfigScopeGrantVO, 0, len(values))
	for _, item := range values {
		grants = append(grants, configfacade.ConfigScopeGrantVO{
			GroupCode: item.GroupCode, ConfigKey: item.ConfigKey,
			CanRead: item.CanRead, CanWrite: item.CanWrite, CanDelete: item.CanDelete,
		})
	}
	normalized, err := s.normalizeRoleConfigScopes(ctx, grants)
	if err != nil {
		return nil, err
	}
	result := make([]authorizationfacade.RoleConfigScopeGrantVO, 0, len(normalized))
	for _, item := range normalized {
		result = append(result, authorizationfacade.RoleConfigScopeGrantVO{
			GroupCode: item.GroupCode, ConfigKey: item.ConfigKey,
			CanRead: item.CanRead, CanWrite: item.CanWrite, CanDelete: item.CanDelete,
		})
	}
	return result, nil
}

// ReplaceRoleConfigScopes persists already policy-authorized grants in the caller's transaction.
func (s *Service) ReplaceRoleConfigScopes(ctx context.Context, roleID int64, values []authorizationfacade.RoleConfigScopeGrantVO, operatorID int64) error {
	if roleID <= 0 {
		return apperrors.Params("roleId不能为空")
	}
	normalized, err := s.NormalizeRoleConfigScopes(ctx, values)
	if err != nil {
		return err
	}
	grants := make([]domain.ConfigScopeGrant, 0, len(normalized))
	for _, item := range normalized {
		grants = append(grants, domain.ConfigScopeGrant{
			RoleID: roleID, GroupCode: item.GroupCode, ConfigKey: item.ConfigKey,
			CanRead: item.CanRead, CanWrite: item.CanWrite, CanDelete: item.CanDelete,
		})
	}
	return s.repo.ReplaceRoleConfigScopes(ctx, roleID, grants, operatorID, func() int64 { return time.Now().UTC().UnixNano() })
}

func (s *Service) AssignRoleConfigScopes(ctx context.Context, actor Actor, roleID int64, request configfacade.AssignRoleConfigScopesRequest) error {
	if actor.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	if roleID <= 0 {
		return apperrors.Params("roleId不能为空")
	}
	if err := stepup.Require(actor.StepUpProof, stepUpActionConfigScopeAssign, configScopeAssignmentBinding(roleID, request.Grants)); err != nil {
		return err
	}
	if s.roles == nil {
		return apperrors.System("配置范围角色安全能力未配置")
	}
	return s.withConfigInvalidationConsistentTx(ctx, func(txCtx context.Context) error {
		role, err := s.roles.GetRole(txCtx, roleID)
		if err != nil {
			return err
		}
		if role == nil {
			return apperrors.NotFound("角色不存在")
		}
		if role.AuthorizationRoot {
			return apperrors.Operation("授权安全根的配置范围由系统管理")
		}
		grants, err := s.normalizeRoleConfigScopes(txCtx, request.Grants)
		if err != nil {
			return err
		}
		for index := range grants {
			grants[index].RoleID = roleID
		}
		if err := s.repo.ReplaceRoleConfigScopes(txCtx, roleID, grants, actor.UserID, func() int64 { return time.Now().UTC().UnixNano() }); err != nil {
			return err
		}
		return s.roles.AdvanceRoleGrantRevision(txCtx, roleID, actor.UserID)
	})
}

func (s *Service) normalizeRoleConfigScopes(ctx context.Context, values []configfacade.ConfigScopeGrantVO) ([]domain.ConfigScopeGrant, error) {
	if len(values) > roleConfigScopeGrantMax {
		return nil, apperrors.Params("配置范围数量超过单次上限")
	}
	grants := make([]domain.ConfigScopeGrant, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		groupCode := s.domain.NormalizeGroupCode(item.GroupCode)
		if groupCode == "" {
			return nil, apperrors.Params("配置分组编码不能为空")
		}
		configKey := s.domain.NormalizeConfigKey(item.ConfigKey)
		canRead := normalizeAccessFlag(item.CanRead)
		canWrite := normalizeAccessFlag(item.CanWrite)
		canDelete := normalizeAccessFlag(item.CanDelete)
		if canWrite != 0 || canDelete != 0 {
			canRead = 1
		}
		key := groupCode + "\x00" + configKey
		if _, exists := seen[key]; exists {
			return nil, apperrors.Params("配置范围存在重复项：" + groupCode + "." + configKey)
		}
		seen[key] = struct{}{}
		grants = append(grants, domain.ConfigScopeGrant{
			RoleID:    0,
			GroupCode: groupCode,
			ConfigKey: configKey,
			CanRead:   canRead,
			CanWrite:  canWrite,
			CanDelete: canDelete,
		})
	}
	groupCodes := make([]string, 0, len(grants))
	groupCodeSet := make(map[string]struct{}, len(grants))
	for _, grant := range grants {
		if _, exists := groupCodeSet[grant.GroupCode]; !exists {
			groupCodeSet[grant.GroupCode] = struct{}{}
			groupCodes = append(groupCodes, grant.GroupCode)
		}
	}
	sort.Strings(groupCodes)
	groups, err := s.repo.ListGroupsByCodes(ctx, groupCodes)
	if err != nil {
		return nil, err
	}
	groupByCode := make(map[string]domain.ConfigGroup, len(groups))
	for _, group := range groups {
		groupByCode[s.domain.NormalizeGroupCode(group.GroupCode)] = group
	}
	configRefs := make([]domain.ConfigKeyRef, 0, len(grants))
	for _, grant := range grants {
		group, exists := groupByCode[grant.GroupCode]
		if !exists || group.IsDeleted == 1 {
			return nil, apperrors.Params("配置分组不存在：" + grant.GroupCode)
		}
		if grant.ConfigKey != "" {
			configRefs = append(configRefs, domain.ConfigKeyRef{GroupID: group.ID, ConfigKey: grant.ConfigKey})
		}
	}
	configs, err := s.repo.ListConfigsByGroupAndKeys(ctx, configRefs)
	if err != nil {
		return nil, err
	}
	configSet := make(map[string]struct{}, len(configs))
	for _, item := range configs {
		if item.IsDeleted == 0 {
			configSet[strconv.FormatInt(item.GroupID, 10)+"\x00"+s.domain.NormalizeConfigKey(item.ConfigKey)] = struct{}{}
		}
	}
	for _, ref := range configRefs {
		key := strconv.FormatInt(ref.GroupID, 10) + "\x00" + ref.ConfigKey
		if _, exists := configSet[key]; !exists {
			groupCode := ""
			for code, group := range groupByCode {
				if group.ID == ref.GroupID {
					groupCode = code
					break
				}
			}
			return nil, apperrors.Params("配置项不存在：" + groupCode + "." + ref.ConfigKey)
		}
	}
	sort.Slice(grants, func(i, j int) bool {
		left, right := grants[i].GroupCode+"\x00"+grants[i].ConfigKey, grants[j].GroupCode+"\x00"+grants[j].ConfigKey
		return left < right
	})
	return grants, nil
}

func (s *Service) getConfigByKeyInternal(ctx context.Context, configKey string, internalAccess bool, actor Actor) (*domain.Config, error) {
	if strings.TrimSpace(configKey) == "" {
		return nil, apperrors.Params("配置键不能为空")
	}
	if !strings.Contains(configKey, ".") {
		return nil, apperrors.Params("配置读取必须使用 fully-qualified key")
	}
	cacheKey, err := s.readCacheKey(ctx, configKey, internalAccess, actor)
	if err != nil {
		return nil, err
	}
	if cached, hit, err := s.cache.GetConfigByKey(ctx, cacheKey); err == nil && hit {
		return cached, nil
	} else if err != nil {
		return nil, err
	}
	item, err := s.resolveConfigByKey(ctx, configKey, internalAccess, actor)
	if err != nil || item == nil {
		return item, err
	}
	if err := s.cache.SetConfigByKey(ctx, cacheKey, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Service) getConfigBatchInternal(ctx context.Context, request configfacade.ConfigBatchRequest, internalAccess bool, actor Actor) (map[string]configfacade.ConfigValueDTO, error) {
	configKeys := canonicalConfigKeys(request.ConfigKeys)
	if len(configKeys) == 0 {
		return map[string]configfacade.ConfigValueDTO{}, nil
	}
	if len(configKeys) > 50 {
		return nil, apperrors.Params("批量读取配置数量超过50")
	}
	for _, configKey := range configKeys {
		if !strings.Contains(configKey, ".") {
			return nil, apperrors.Params("配置读取必须使用 fully-qualified key")
		}
	}
	cacheKey, err := s.policyBatchCacheKey(ctx, configKeys, internalAccess, actor)
	if err != nil {
		return nil, err
	}
	if cached, hit, err := s.cache.GetBatch(ctx, cacheKey); err == nil && hit {
		return s.toConfigValueMap(ctx, cached)
	} else if err != nil {
		return nil, err
	}
	var resolved map[string]domain.Config
	result := make(map[string]configfacade.ConfigValueDTO, len(configKeys))
	if err := s.withReadOnlySnapshot(ctx, func(snapshotCtx context.Context) error {
		var groupsByID map[int64]domain.ConfigGroup
		var resolveErr error
		resolved, groupsByID, resolveErr = s.resolveFullyQualifiedConfigsBatch(snapshotCtx, configKeys)
		if resolveErr != nil {
			return resolveErr
		}
		for _, inputKey := range configKeys {
			item, ok := resolved[inputKey]
			if !ok {
				continue
			}
			group := groupsByID[item.GroupID]
			if accessErr := s.enforceClientReadAccess(actor, &item, &group, internalAccess); accessErr != nil {
				return accessErr
			}
			copyItem := item
			resolved[inputKey] = copyItem
			dto, dtoErr := s.toConfigValueDTO(snapshotCtx, &copyItem)
			if dtoErr != nil {
				return dtoErr
			}
			result[inputKey] = *dto
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err := s.cache.SetBatch(ctx, cacheKey, resolved); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) resolveFullyQualifiedConfigsBatch(ctx context.Context, keys []string) (map[string]domain.Config, map[int64]domain.ConfigGroup, error) {
	type parsedKey struct{ groupCode, configKey string }
	parsed := make(map[string]parsedKey, len(keys))
	groupCodes := make([]string, 0, len(keys))
	seenGroups := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		parts := strings.SplitN(strings.TrimSpace(key), ".", 2)
		if len(parts) != 2 {
			return nil, nil, apperrors.Params("配置读取必须使用 fully-qualified key")
		}
		groupCode := s.domain.NormalizeGroupCode(parts[0])
		configKey := s.domain.NormalizeConfigKey(parts[1])
		if groupCode == "" || configKey == "" {
			return nil, nil, apperrors.Params("配置读取必须使用 fully-qualified key")
		}
		parsed[key] = parsedKey{groupCode: groupCode, configKey: configKey}
		if _, exists := seenGroups[groupCode]; !exists {
			seenGroups[groupCode] = struct{}{}
			groupCodes = append(groupCodes, groupCode)
		}
	}
	groups, err := s.repo.ListGroupsByCodes(ctx, groupCodes)
	if err != nil {
		return nil, nil, err
	}
	groupByCode := make(map[string]domain.ConfigGroup, len(groups))
	groupByID := make(map[int64]domain.ConfigGroup, len(groups))
	for _, group := range groups {
		if group.IsDeleted != 0 {
			continue
		}
		groupByCode[s.domain.NormalizeGroupCode(group.GroupCode)] = group
		groupByID[group.ID] = group
	}
	refs := make([]domain.ConfigKeyRef, 0, len(parsed))
	for _, item := range parsed {
		if group, ok := groupByCode[item.groupCode]; ok {
			refs = append(refs, domain.ConfigKeyRef{GroupID: group.ID, ConfigKey: item.configKey})
		}
	}
	configs, err := s.repo.ListConfigsByGroupAndKeys(ctx, refs)
	if err != nil {
		return nil, nil, err
	}
	configByRef := make(map[string]domain.Config, len(configs))
	for _, item := range configs {
		configByRef[strconv.FormatInt(item.GroupID, 10)+"\x00"+s.domain.NormalizeConfigKey(item.ConfigKey)] = item
	}
	resolved := make(map[string]domain.Config, len(keys))
	for _, key := range keys {
		item := parsed[key]
		group, ok := groupByCode[item.groupCode]
		if !ok {
			continue
		}
		config, ok := configByRef[strconv.FormatInt(group.ID, 10)+"\x00"+item.configKey]
		if !ok {
			continue
		}
		config.GroupCode, config.GroupName = group.GroupCode, group.GroupName
		resolved[key] = config
	}
	return resolved, groupByID, nil
}

func (s *Service) resolveConfigByKey(ctx context.Context, configKey string, internalAccess bool, actor Actor) (*domain.Config, error) {
	if !strings.Contains(configKey, ".") {
		return nil, apperrors.Params("配置读取必须使用 fully-qualified key")
	}
	if strings.Contains(configKey, ".") {
		parts := strings.SplitN(configKey, ".", 2)
		if len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != "" {
			group, err := s.loadGroupByCode(ctx, parts[0])
			if err != nil {
				return nil, err
			}
			if group != nil {
				item, err := s.repo.FindConfigByGroupAndKey(ctx, group.ID, strings.TrimSpace(parts[1]), false)
				if err != nil {
					return nil, err
				}
				if item != nil {
					item.GroupCode = group.GroupCode
					item.GroupName = group.GroupName
					if err := s.enforceClientReadAccess(actor, item, group, internalAccess); err != nil {
						return nil, err
					}
					return item, nil
				}
			}
		}
	}
	items, err := s.repo.FindConfigsByRawKey(ctx, configKey, false)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	item := items[0]
	var group *domain.ConfigGroup
	if item.GroupID > 0 {
		group, err = s.loadGroupByID(ctx, item.GroupID)
		if err != nil {
			return nil, err
		}
		if group != nil {
			item.GroupCode = group.GroupCode
			item.GroupName = group.GroupName
		}
	}
	if err := s.enforceClientReadAccess(actor, &item, group, internalAccess); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Service) loadEnabledConfigsByGroup(ctx context.Context, groupID int64) ([]domain.Config, error) {
	const pageSize int64 = 200
	enabled := 1
	result := make([]domain.Config, 0)
	for current := int64(1); ; current++ {
		page, err := s.repo.QueryConfigs(ctx, domain.ConfigPageQuery{
			Current:   current,
			PageSize:  pageSize,
			GroupID:   &groupID,
			IsEnabled: &enabled,
		})
		if err != nil {
			return nil, err
		}
		result = append(result, page.Records...)
		if int64(len(result)) >= page.Total || len(page.Records) == 0 {
			return result, nil
		}
	}
}

func (s *Service) canReadConfigForClientList(actor Actor, item *domain.Config, group *domain.ConfigGroup) bool {
	identity := domain.ConfigReadAnonymous
	if actor.Authenticated {
		identity = domain.ConfigReadAuthenticated
	}
	if !domain.CanReadConfig(item, group, domain.ConfigReadContext{
		Identity:     identity,
		AccountID:    actor.AccountID,
		ScopeID:      actor.ScopeID,
		AuthzVersion: actor.AuthzVersion,
	}, nil) {
		return false
	}
	if strings.TrimSpace(group.PermissionCode) == "" {
		return true
	}
	for _, permission := range strings.Split(group.PermissionCode, ";") {
		if actor.HasPermission(strings.TrimSpace(permission)) {
			return true
		}
	}
	return false
}

func (s *Service) resolveFullyQualifiedConfig(ctx context.Context, fullyQualifiedKey string) (*domain.Config, *domain.ConfigGroup, error) {
	parts := strings.SplitN(strings.TrimSpace(fullyQualifiedKey), ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return nil, nil, apperrors.Params("配置读取必须使用 fully-qualified key")
	}
	group, err := s.loadGroupByCode(ctx, parts[0])
	if err != nil || group == nil {
		return nil, group, err
	}
	item, err := s.repo.FindConfigByGroupAndKey(ctx, group.ID, strings.TrimSpace(parts[1]), false)
	if err != nil || item == nil {
		return item, group, err
	}
	item.GroupCode = group.GroupCode
	item.GroupName = group.GroupName
	return item, group, nil
}

func (s *Service) resolveConfigAccess(ctx context.Context, actor Actor, groupCode string, configKey string) (domain.ConfigAccess, error) {
	grants, err := s.loadConfigAccessGrants(ctx, actor)
	if err != nil {
		return domain.ConfigAccess{}, err
	}
	return s.resolveConfigAccessFromGrants(actor, grants, groupCode, configKey), nil
}

func (s *Service) loadConfigAccessGrants(ctx context.Context, actor Actor) ([]domain.ConfigScopeGrant, error) {
	if actor.IsAdmin {
		return nil, nil
	}
	roleIDs := dedupePositiveIDs(actor.RoleIDs)
	if len(roleIDs) == 0 {
		return []domain.ConfigScopeGrant{}, nil
	}
	return s.repo.ListConfigScopeGrantsByRoleIDs(ctx, roleIDs)
}

func (s *Service) resolveConfigAccessFromGrants(actor Actor, grants []domain.ConfigScopeGrant, groupCode string, configKey string) domain.ConfigAccess {
	if actor.IsAdmin {
		return domain.ConfigAccess{CanRead: true, CanWrite: true, CanDelete: true, AccessSource: "admin"}
	}
	groupCode = s.domain.NormalizeGroupCode(groupCode)
	configKey = s.domain.NormalizeConfigKey(configKey)
	if groupCode == "" {
		return domain.ConfigAccess{}
	}
	access := domain.ConfigAccess{AccessSource: "none"}
	for _, grant := range grants {
		if s.domain.NormalizeGroupCode(grant.GroupCode) != groupCode {
			continue
		}
		grantKey := s.domain.NormalizeConfigKey(grant.ConfigKey)
		source := "group"
		if grantKey != "" {
			if configKey == "" {
				if grant.CanRead != 0 || grant.CanWrite != 0 || grant.CanDelete != 0 {
					access.CanRead = true
					if access.AccessSource == "" || access.AccessSource == "none" {
						access.AccessSource = "key"
					}
				}
				continue
			}
			if grantKey != configKey {
				continue
			}
			source = "key"
		}
		if grant.CanRead != 0 {
			access.CanRead = true
		}
		if grant.CanWrite != 0 {
			access.CanWrite = true
			access.CanRead = true
		}
		if grant.CanDelete != 0 {
			access.CanDelete = true
			access.CanRead = true
		}
		if access.AccessSource == "" || access.AccessSource == "none" || source == "key" {
			access.AccessSource = source
		}
	}
	return access
}

func (s *Service) requireConfigReadAccess(ctx context.Context, actor Actor, groupCode string, configKey string) error {
	access, err := s.resolveConfigAccess(ctx, actor, groupCode, configKey)
	if err != nil {
		return err
	}
	if actor.IsAdmin || access.CanRead {
		return nil
	}
	return apperrors.DataScopeDenied("配置范围不足")
}

func (s *Service) requireConfigWriteAccess(ctx context.Context, actor Actor, groupCode string, configKey string) error {
	access, err := s.resolveConfigAccess(ctx, actor, groupCode, configKey)
	if err != nil {
		return err
	}
	if actor.IsAdmin || access.CanWrite {
		return nil
	}
	return apperrors.Forbidden("无权限修改该配置范围")
}

func (s *Service) requireConfigDeleteAccess(ctx context.Context, actor Actor, groupCode string, configKey string) error {
	access, err := s.resolveConfigAccess(ctx, actor, groupCode, configKey)
	if err != nil {
		return err
	}
	if actor.IsAdmin || access.CanDelete {
		return nil
	}
	return apperrors.Forbidden("无权限删除该配置范围")
}

func (s *Service) filterReadableChangeLogs(ctx context.Context, actor Actor, logs []domain.ConfigChangeLog) ([]domain.ConfigChangeLog, error) {
	if actor.IsAdmin || len(logs) == 0 {
		return logs, nil
	}
	configIDs := make([]int64, 0, len(logs))
	for _, item := range logs {
		if item.ConfigID > 0 {
			configIDs = append(configIDs, item.ConfigID)
		}
	}
	configs, err := s.repo.ListConfigsByIDs(ctx, dedupePositiveIDs(configIDs))
	if err != nil {
		return nil, err
	}
	configMap := make(map[int64]domain.Config, len(configs))
	for _, item := range configs {
		configMap[item.ID] = item
	}
	result := make([]domain.ConfigChangeLog, 0, len(logs))
	for _, item := range logs {
		configItem, ok := configMap[item.ConfigID]
		if !ok || configItem.IsDeleted == 1 {
			continue
		}
		access, accessErr := s.resolveConfigAccess(ctx, actor, configItem.GroupCode, configItem.ConfigKey)
		if accessErr != nil {
			return nil, accessErr
		}
		if access.CanRead {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *Service) enforceClientReadAccess(actor Actor, item *domain.Config, group *domain.ConfigGroup, internalAccess bool) error {
	if internalAccess || item == nil {
		return nil
	}
	identity := domain.ConfigReadAnonymous
	if actor.Authenticated {
		identity = domain.ConfigReadAuthenticated
	}
	if !domain.CanReadConfig(item, group, domain.ConfigReadContext{
		Identity:     identity,
		AccountID:    actor.AccountID,
		ScopeID:      actor.ScopeID,
		AuthzVersion: actor.AuthzVersion,
	}, nil) {
		if item.RequiredLogin == 1 && !actor.Authenticated {
			return apperrors.Unauthorized("该配置需要登录后访问")
		}
		return apperrors.Forbidden("配置读取策略不允许当前身份访问")
	}
	if group == nil || strings.TrimSpace(group.PermissionCode) == "" {
		return nil
	}
	for _, permission := range strings.Split(group.PermissionCode, ";") {
		if actor.HasPermission(strings.TrimSpace(permission)) {
			return nil
		}
	}
	return apperrors.Forbidden("您无权限读取该配置")
}

func (s *Service) resolveRuntimeValue(ctx context.Context, item *domain.Config) (string, error) {
	if item == nil {
		return "", nil
	}
	if strings.TrimSpace(item.ConfigValue) != "" {
		return item.ConfigValue, nil
	}
	if item.ExtJSON == nil || item.ExtJSON.Secret == nil {
		return item.ConfigValue, nil
	}
	if strings.TrimSpace(item.ExtJSON.Secret.Plain) != "" {
		return item.ExtJSON.Secret.Plain, nil
	}
	if s.secrets == nil || strings.TrimSpace(item.ExtJSON.Secret.CiphertextB64) == "" || strings.TrimSpace(item.ExtJSON.Secret.EDEKB64) == "" {
		return item.ConfigValue, nil
	}
	value, err := s.secrets.DecryptString(ctx, *item.ExtJSON.Secret)
	if err != nil {
		return "", err
	}
	item.ExtJSON.Secret.Plain = value
	return value, nil
}

func (s *Service) prepareConfigValueStorage(ctx context.Context, item *domain.Config, runtimeValue string) error {
	if item == nil {
		return nil
	}
	if item.ExtJSON == nil {
		item.ExtJSON = &domain.ConfigExtJSON{}
	}
	if item.IsSensitive == 1 {
		if s.secrets == nil {
			return apperrors.System("敏感配置加密服务未配置")
		}
		secretValue, err := s.secrets.EncryptString(ctx, runtimeValue)
		if err != nil {
			return err
		}
		item.ExtJSON.Secret = &secretValue
		item.ConfigValue = ""
		return nil
	}
	item.ConfigValue = runtimeValue
	item.ExtJSON.Secret = nil
	return nil
}

func (s *Service) loadGroupByID(ctx context.Context, groupID int64) (*domain.ConfigGroup, error) {
	if groupID <= 0 {
		return nil, nil
	}
	return s.repo.FindGroupByID(ctx, groupID)
}

func (s *Service) loadGroupByCode(ctx context.Context, groupCode string) (*domain.ConfigGroup, error) {
	normalized := s.domain.NormalizeGroupCode(groupCode)
	if normalized == "" {
		return nil, nil
	}
	if cached, hit, err := s.cache.GetGroupByCode(ctx, normalized); err == nil && hit {
		return cached, nil
	} else if err != nil {
		return nil, err
	}
	item, err := s.repo.FindGroupByCode(ctx, normalized)
	if err != nil || item == nil {
		return item, err
	}
	_ = s.cache.SetGroupByCode(ctx, normalized, item)
	return item, nil
}

func (s *Service) resolveGroupMoveOrder(ctx context.Context, targetID int64, beforeID, afterID *int64) (int, error) {
	if beforeID == nil && afterID == nil {
		return 1, nil
	}
	if beforeID == nil && afterID != nil {
		afterItem, err := s.repo.FindGroupByID(ctx, *afterID)
		if err != nil {
			return 0, err
		}
		if afterItem == nil || afterItem.IsDeleted == 1 {
			return 0, apperrors.Params("afterId 对应的配置分组不存在")
		}
		if afterItem.ID == targetID {
			return 0, apperrors.Params("afterId 不能是自己")
		}
		return afterItem.SortOrder, nil
	}
	if beforeID != nil && afterID == nil {
		beforeItem, err := s.repo.FindGroupByID(ctx, *beforeID)
		if err != nil {
			return 0, err
		}
		if beforeItem == nil || beforeItem.IsDeleted == 1 {
			return 0, apperrors.Params("beforeId 对应的配置分组不存在")
		}
		if beforeItem.ID == targetID {
			return 0, apperrors.Params("beforeId 不能是自己")
		}
		return beforeItem.SortOrder, nil
	}
	beforeItem, err := s.repo.FindGroupByID(ctx, *beforeID)
	if err != nil {
		return 0, err
	}
	afterItem, err := s.repo.FindGroupByID(ctx, *afterID)
	if err != nil {
		return 0, err
	}
	if beforeItem == nil || beforeItem.IsDeleted == 1 {
		return 0, apperrors.Params("beforeId 对应的配置分组不存在")
	}
	if afterItem == nil || afterItem.IsDeleted == 1 {
		return 0, apperrors.Params("afterId 对应的配置分组不存在")
	}
	if beforeItem.ID == targetID || afterItem.ID == targetID {
		return 0, apperrors.Params("beforeId/afterId 不能是自己")
	}
	return afterItem.SortOrder, nil
}

func (s *Service) buildOperationChain(ctx context.Context, logID int64) ([]domain.ConfigChangeLog, error) {
	logMap := map[int64]domain.ConfigChangeLog{}
	processed := map[int64]struct{}{}
	toProcess := map[int64]struct{}{logID: {}}
	for iteration := 0; len(toProcess) > 0 && iteration < 100; iteration++ {
		currentIDs := make([]int64, 0, len(toProcess))
		for id := range toProcess {
			currentIDs = append(currentIDs, id)
		}
		toProcess = map[int64]struct{}{}

		currentLogs, err := s.repo.ListChangeLogsByIDs(ctx, currentIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range currentLogs {
			logMap[item.ID] = item
		}
		referencing, err := s.repo.ListChangeLogsReferencing(ctx, currentIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range referencing {
			logMap[item.ID] = item
		}
		found := make([]int64, 0)
		for _, id := range currentIDs {
			if _, ok := processed[id]; ok {
				continue
			}
			processed[id] = struct{}{}
			item, ok := logMap[id]
			if !ok {
				continue
			}
			if item.ParentLogID != nil {
				found = append(found, *item.ParentLogID)
			}
			if item.RelatedLogID != nil {
				found = append(found, *item.RelatedLogID)
			}
		}
		for _, item := range referencing {
			found = append(found, item.ID)
		}
		for _, id := range found {
			if id <= 0 {
				continue
			}
			if _, ok := processed[id]; ok {
				continue
			}
			toProcess[id] = struct{}{}
		}
	}
	result := make([]domain.ConfigChangeLog, 0, len(logMap))
	for _, item := range logMap {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		return timePtrBefore(result[i].OperationTime, result[j].OperationTime)
	})
	return result, nil
}

func (s *Service) toConfigVO(ctx context.Context, item *domain.Config, access domain.ConfigAccess) *configfacade.ConfigVO {
	if item == nil {
		return nil
	}
	runtimeValue, _ := s.resolveRuntimeValue(ctx, item)
	sensitivity, _ := domain.NormalizeSensitivity(item.Sensitivity, item.IsSensitive)
	ext := item.ExtJSON
	if ext != nil {
		ext = s.domain.SanitizeExtJSON(ext)
	}
	return &configfacade.ConfigVO{
		ID:             item.ID,
		GroupID:        item.GroupID,
		GroupName:      item.GroupName,
		ConfigKey:      item.ConfigKey,
		ConfigValue:    ternary(sensitivity == domain.ConfigSensitivityNormal, runtimeValue, ""),
		ValueType:      item.ValueType,
		ConfigDesc:     item.ConfigDesc,
		IsSensitive:    item.IsSensitive,
		IsReadonly:     item.IsReadonly,
		IsEnabled:      item.IsEnabled,
		EffectType:     item.EffectType,
		IsSystemConfig: item.IsSystemConfig,
		RequiredLogin:  item.RequiredLogin,
		ExtJSON:        toFacadeExtJSON(ext),
		UIWidget:       item.UIWidget,
		Validation:     toFacadeValidation(item.Validation),
		Exposure:       item.Exposure,
		Sensitivity:    item.Sensitivity,
		SchemaVersion:  item.SchemaVersion,
		Version:        item.Version,
		ValuePresent:   strings.TrimSpace(runtimeValue) != "",
		Connected:      configConnected(item),
		ConsumerStatus: ternary(configConnected(item), "CONNECTED", "UNCONNECTED"),
		CreatedBy:      item.CreatedBy,
		CreateTime:     item.CreateTime,
		UpdatedBy:      item.UpdatedBy,
		UpdateTime:     item.UpdateTime,
		Access:         toConfigAccessVO(access),
	}
}

// configValueCacheRecord keeps a validated scalar in its canonical string
// representation. ConfigValueDTO.Value is intentionally any for the public
// API, but putting that field directly through Sonic would widen INTEGER to
// float64 on an L1/L2 hit. Rehydrating through the declared scalar contract
// keeps cache and database reads observationally identical.
type configValueCacheRecord struct {
	Key           string                   `json:"key"`
	Type          string                   `json:"type"`
	RawValue      string                   `json:"rawValue"`
	Validation    *domain.ScalarValidation `json:"validation,omitempty"`
	GroupCode     string                   `json:"groupCode,omitempty"`
	GroupName     string                   `json:"groupName,omitempty"`
	SchemaVersion int                      `json:"schemaVersion"`
	Version       int64                    `json:"version"`
	// CataloguedPublic is written only after the direct parent-group and row
	// checks prove this value is public without a login or permission grant.
	// Missing legacy Sonic fields decode to false and therefore fall back to
	// the authoritative authorization path.
	CataloguedPublic bool `json:"cataloguedPublic"`
}

func (s *Service) toConfigValueCacheRecord(ctx context.Context, item *domain.Config) (*configValueCacheRecord, error) {
	if item == nil {
		return nil, nil
	}
	runtimeValue, err := s.resolveRuntimeValue(ctx, item)
	if err != nil {
		return nil, err
	}
	valueType, err := domain.NormalizeValueType(item.ValueType)
	if err != nil {
		return nil, apperrors.Operation("配置值不符合声明的标量契约")
	}
	canonicalValue, _, err := domain.CanonicalizeScalarValue(runtimeValue, valueType, item.Validation)
	if err != nil {
		return nil, apperrors.Operation("配置值不符合声明的标量契约")
	}
	return &configValueCacheRecord{
		Key:           item.ConfigKey,
		Type:          item.ValueType,
		RawValue:      canonicalValue,
		Validation:    item.Validation,
		GroupCode:     item.GroupCode,
		GroupName:     item.GroupName,
		SchemaVersion: item.SchemaVersion,
		Version:       item.Version,
	}, nil
}

func (r configValueCacheRecord) toDTO() (*configfacade.ConfigValueDTO, error) {
	typedValue, err := domain.DecodeScalarValue(r.RawValue, r.Type, r.Validation)
	if err != nil {
		return nil, err
	}
	return &configfacade.ConfigValueDTO{
		Key:           r.Key,
		Type:          r.Type,
		Value:         typedValue,
		GroupCode:     r.GroupCode,
		GroupName:     r.GroupName,
		SchemaVersion: r.SchemaVersion,
		Version:       r.Version,
	}, nil
}

func (s *Service) toConfigValueDTO(ctx context.Context, item *domain.Config) (*configfacade.ConfigValueDTO, error) {
	record, err := s.toConfigValueCacheRecord(ctx, item)
	if err != nil || record == nil {
		return nil, err
	}
	value, err := record.toDTO()
	if err != nil {
		return nil, apperrors.Operation("配置值不符合声明的标量契约")
	}
	return value, nil
}

func (s *Service) toConfigValueMap(ctx context.Context, items map[string]domain.Config) (map[string]configfacade.ConfigValueDTO, error) {
	result := make(map[string]configfacade.ConfigValueDTO, len(items))
	for key, item := range items {
		dto, err := s.toConfigValueDTO(ctx, &item)
		if err != nil {
			return nil, err
		}
		result[key] = *dto
	}
	return result, nil
}

func configConnected(item *domain.Config) bool {
	if item == nil {
		return false
	}
	key := item.FullyQualifiedKey(nil)
	switch key {
	case "SEVEN_FRONTEND_METADATA.title",
		"SEVEN_FRONTEND_METADATA.shortTitle",
		"SEVEN_FRONTEND_METADATA.themePrimaryColor",
		"SEVEN_FRONTEND_METADATA.loginLogo",
		"SEVEN_FRONTEND_METADATA.favicon":
		return true
	default:
		return false
	}
}

func toConfigGroupVO(item *domain.ConfigGroup, access domain.ConfigAccess) *configfacade.ConfigGroupVO {
	if item == nil {
		return nil
	}
	return &configfacade.ConfigGroupVO{
		ID:             item.ID,
		GroupCode:      item.GroupCode,
		GroupName:      item.GroupName,
		Module:         item.Module,
		PermissionCode: item.PermissionCode,
		SortOrder:      item.SortOrder,
		Status:         item.Status,
		ConfigCount:    item.ConfigCount,
		CreateTime:     item.CreateTime,
		UpdateTime:     item.UpdateTime,
		Access:         toConfigAccessVO(access),
	}
}

func toConfigAccessVO(access domain.ConfigAccess) configfacade.ConfigAccessVO {
	return configfacade.ConfigAccessVO{
		CanRead:      access.CanRead,
		CanWrite:     access.CanWrite,
		CanDelete:    access.CanDelete,
		AccessSource: access.AccessSource,
	}
}

func (s *Service) toChangeLogVOs(ctx context.Context, logs []domain.ConfigChangeLog, maskAllValues bool) ([]configfacade.ConfigChangeLogVO, error) {
	if len(logs) == 0 {
		return []configfacade.ConfigChangeLogVO{}, nil
	}
	configIDs := make([]int64, 0, len(logs))
	userIDs := make([]int64, 0, len(logs)*2)
	for _, item := range logs {
		if item.ConfigID > 0 {
			configIDs = append(configIDs, item.ConfigID)
		}
		if item.OperatorID > 0 {
			userIDs = append(userIDs, item.OperatorID)
		}
		if item.AppliedBy != nil && *item.AppliedBy > 0 {
			userIDs = append(userIDs, *item.AppliedBy)
		}
	}
	configMap := make(map[int64]domain.Config)
	configs, err := s.repo.ListConfigsByIDs(ctx, dedupePositiveIDs(configIDs))
	if err != nil {
		return nil, err
	}
	for _, item := range configs {
		configMap[item.ID] = item
	}
	nameMap, _ := s.findNicknames(ctx, userIDs)
	result := make([]configfacade.ConfigChangeLogVO, 0, len(logs))
	for _, item := range logs {
		configItem, hasConfig := configMap[item.ConfigID]
		oldValue := item.OldValue
		newValue := item.NewValue
		if hasConfig {
			oldValue = s.maskLogValue(oldValue, configItem.IsSensitive, maskAllValues)
			newValue = s.maskLogValue(newValue, configItem.IsSensitive, maskAllValues)
		} else if maskAllValues {
			oldValue = s.domain.MaskSensitive(oldValue, 1)
			newValue = s.domain.MaskSensitive(newValue, 1)
		}
		vo := configfacade.ConfigChangeLogVO{
			ID:              item.ID,
			ConfigID:        item.ConfigID,
			ConfigKey:       item.ConfigKey,
			OldValue:        oldValue,
			NewValue:        newValue,
			EffectType:      item.EffectType,
			Status:          item.Status,
			OperationType:   item.OperationType,
			OperatorID:      item.OperatorID,
			OperatorName:    lookupOperatorName(nameMap, item.OperatorID),
			OperationTime:   item.OperationTime,
			OperationReason: item.OperationReason,
			AppliedTime:     item.AppliedTime,
			ParentLogID:     item.ParentLogID,
			RelatedLogID:    item.RelatedLogID,
		}
		if item.AppliedBy != nil {
			vo.AppliedBy = *item.AppliedBy
			vo.AppliedByName = lookupOperatorName(nameMap, *item.AppliedBy)
		}
		if item.OperationType == string(domain.ConfigOperationRollback) {
			vo.RollbackReason = item.OperationReason
		}
		result = append(result, vo)
	}
	return result, nil
}

func (s *Service) maskLogValue(value string, isSensitive int, forceMask bool) string {
	if forceMask {
		return s.domain.MaskSensitive(value, 1)
	}
	return s.domain.MaskSensitive(value, isSensitive)
}

func (s *Service) findNicknames(ctx context.Context, userIDs []int64) (map[int64]string, error) {
	result := map[int64]string{0: "系统"}
	unique := dedupePositiveIDs(userIDs)
	if len(unique) == 0 || s.users == nil {
		return result, nil
	}
	found, err := s.users.FindNicknames(ctx, unique)
	if err != nil {
		return result, err
	}
	for key, value := range found {
		result[key] = value
	}
	return result, nil
}

func (s *Service) guardReadonlyMutation(actor Actor, before, after int) error {
	if before == after {
		return nil
	}
	if s.canEditReadonly(actor) {
		return nil
	}
	return apperrors.Forbidden("您无权修改只读保护标志")
}

func (s *Service) canEditReadonly(actor Actor) bool {
	return actor.IsAdmin || actor.HasPermission("system:config:edit:readonly")
}

func (s *Service) guardSensitiveMutation(actor Actor, before, after int) error {
	if before != 1 && after != 1 {
		return nil
	}
	if actor.HasPermission("system:config:sensitive") {
		return nil
	}
	return apperrors.Forbidden("该配置为敏感配置，您无权修改")
}

func (s *Service) batchCacheKey(ctx context.Context, configKeys []string) (string, error) {
	version, err := s.cache.CurrentBatchVersion(ctx)
	if err != nil {
		return "", err
	}
	return strings.Join(configKeys, ",") + "|v=" + strconv.FormatInt(version, 10), nil
}

func (s *Service) readCacheKey(ctx context.Context, fullyQualifiedKey string, internalAccess bool, actor Actor) (string, error) {
	version, err := s.cache.CurrentBatchVersion(ctx)
	if err != nil {
		return "", err
	}
	identity := "anonymous"
	accountID := int64(0)
	scopeID := "server:local"
	authzVersion := int64(0)
	if internalAccess {
		identity = "internal-legacy-denied"
	} else if actor.Authenticated {
		identity = "authenticated"
		accountID = actor.AccountID
		scopeID = strings.TrimSpace(actor.ScopeID)
		authzVersion = actor.AuthzVersion
	}
	return strings.Join([]string{
		"policy", identity,
		"account=" + strconv.FormatInt(accountID, 10),
		"scope=" + scopeID,
		"authz=" + strconv.FormatInt(authzVersion, 10),
		"key=" + fullyQualifiedKey,
		"v=" + strconv.FormatInt(version, 10),
	}, "|"), nil
}

func normalizeInternalReadIdentity(consumerID, serverScope, purpose, allowedSensitivity string) (string, string, string, domain.ConfigSensitivity, error) {
	consumerID = strings.TrimSpace(consumerID)
	serverScope = strings.TrimSpace(serverScope)
	purpose = strings.TrimSpace(purpose)
	if consumerID == "" || serverScope == "" || purpose == "" {
		return "", "", "", "", apperrors.Params("内部读取身份字段不完整")
	}
	allowed, err := domain.NormalizeSensitivity(allowedSensitivity, 0)
	if err != nil {
		return "", "", "", "", apperrors.Params(err.Error())
	}
	return consumerID, serverScope, purpose, allowed, nil
}

func (s *Service) resolveRegisteredConsumerConfig(
	ctx context.Context,
	consumerID string,
	fullyQualifiedKey string,
	serverScope string,
	purpose string,
	allowed domain.ConfigSensitivity,
) (*domain.Config, error) {
	s.consumersMu.RLock()
	registration, ok := s.internalConsumers[consumerID+"\x00"+fullyQualifiedKey]
	s.consumersMu.RUnlock()
	if !ok {
		return nil, apperrors.Forbidden("配置消费者未注册")
	}
	item, group, err := s.resolveFullyQualifiedConfig(ctx, fullyQualifiedKey)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, apperrors.NotFound("配置不存在或未启用")
	}
	if !domain.CanReadConfig(item, group, domain.ConfigReadContext{
		Identity:      domain.ConfigReadInternal,
		ConsumerID:    consumerID,
		ScopeID:       serverScope,
		Purpose:       purpose,
		AllowedSecret: allowed,
	}, &registration) {
		return nil, apperrors.Forbidden("配置消费者声明与注册策略不匹配")
	}
	return item, nil
}

func (s *Service) internalConsumerCacheKey(
	ctx context.Context,
	consumerID string,
	serverScope string,
	purpose string,
	allowed domain.ConfigSensitivity,
	keys []string,
) (string, error) {
	version, err := s.cache.CurrentBatchVersion(ctx)
	if err != nil {
		return "", err
	}
	s.consumersMu.RLock()
	registryVersion := s.consumerRegistryVersion
	s.consumersMu.RUnlock()
	return strings.Join([]string{
		"policy", "internal",
		"consumer=" + consumerID,
		"scope=" + serverScope,
		"purpose=" + purpose,
		"allowed=" + string(allowed),
		"registry=" + strconv.FormatInt(registryVersion, 10),
		"keys=" + strings.Join(keys, ","),
		"v=" + strconv.FormatInt(version, 10),
	}, "|"), nil
}

func sensitivityWithin(value domain.ConfigSensitivity, allowed domain.ConfigSensitivity) bool {
	rank := func(sensitivity domain.ConfigSensitivity) int {
		switch sensitivity {
		case domain.ConfigSensitivitySecret:
			return 2
		case domain.ConfigSensitivitySensitive:
			return 1
		default:
			return 0
		}
	}
	return rank(value) <= rank(allowed)
}

func (s *Service) policyBatchCacheKey(ctx context.Context, configKeys []string, internalAccess bool, actor Actor) (string, error) {
	identityKey, err := s.readCacheKey(ctx, "batch", internalAccess, actor)
	if err != nil {
		return "", err
	}
	return identityKey + "|keys=" + strings.Join(configKeys, ","), nil
}

func (s *Service) withTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.transactor == nil || !s.transactor.Enabled() {
		return fn(ctx)
	}
	return s.transactor.WithinTransaction(ctx, fn)
}

// withConfigInvalidationTx makes every configuration mutation append its DG5
// invalidation to the same transaction as the source-of-truth write. The
// writer cache becomes dirty only after that transaction committed.
func (s *Service) withConfigInvalidationTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return cachegovernancefacade.RunInvalidatedMutation(ctx, s.withTx, s.transactor, s.invalidations, cachepolicy.DataClassConfigPublicScalar, func(txCtx context.Context) (bool, error) {
		return true, fn(txCtx)
	})
}

func (s *Service) withConfigInvalidationTxIf(ctx context.Context, fn func(ctx context.Context) (bool, error)) error {
	return cachegovernancefacade.RunInvalidatedMutation(ctx, s.withTx, s.transactor, s.invalidations, cachepolicy.DataClassConfigPublicScalar, fn)
}

func (s *Service) withReadOnlySnapshot(ctx context.Context, fn func(ctx context.Context) error) error {
	snapshotter, ok := s.transactor.(readSnapshotter)
	if !ok || snapshotter == nil || !snapshotter.Enabled() {
		return apperrors.System("配置组合读取需要一致性只读快照")
	}
	return snapshotter.WithinReadOnlySnapshot(ctx, fn)
}

func (s *Service) withConsistentTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	consistent, ok := s.transactor.(consistentTransactor)
	if !ok || consistent == nil || !consistent.Enabled() {
		return apperrors.System("配置范围变更需要一致性事务")
	}
	return consistent.WithinConsistentTransaction(ctx, fn)
}

func (s *Service) withConfigInvalidationConsistentTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return cachegovernancefacade.RunInvalidatedMutation(ctx, s.withConsistentTransaction, s.transactor, s.invalidations, cachepolicy.DataClassConfigPublicScalar, func(txCtx context.Context) (bool, error) {
		return true, fn(txCtx)
	})
}

func toDomainExtJSON(ext *configfacade.ConfigExtJSON) *domain.ConfigExtJSON {
	if ext == nil {
		return nil
	}
	result := &domain.ConfigExtJSON{}
	if len(ext.Enums) > 0 {
		result.Enums = append([]string(nil), ext.Enums...)
	}
	if ext.Secret != nil {
		result.Secret = &domain.ConfigSecretValue{
			Plain:         ext.Secret.Plain,
			CiphertextB64: ext.Secret.CiphertextB64,
			EDEKB64:       ext.Secret.EDEKB64,
			WrapKeyRef:    ext.Secret.WrapKeyRef,
		}
	}
	return result
}

func toDomainValidation(value *configfacade.ScalarValidation) *domain.ScalarValidation {
	if value == nil {
		return nil
	}
	return &domain.ScalarValidation{
		Required:  value.Required,
		MinLength: value.MinLength,
		MaxLength: value.MaxLength,
		MinValue:  value.MinValue,
		MaxValue:  value.MaxValue,
		Options:   append([]string(nil), value.Options...),
		MaxItems:  value.MaxItems,
	}
}

func toFacadeValidation(value *domain.ScalarValidation) *configfacade.ScalarValidation {
	if value == nil {
		return nil
	}
	return &configfacade.ScalarValidation{
		Required:  value.Required,
		MinLength: value.MinLength,
		MaxLength: value.MaxLength,
		MinValue:  value.MinValue,
		MaxValue:  value.MaxValue,
		Options:   append([]string(nil), value.Options...),
		MaxItems:  value.MaxItems,
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func protectedAuditValue(item *domain.Config, value string) string {
	if item == nil {
		return value
	}
	sensitivity, err := domain.NormalizeSensitivity(item.Sensitivity, item.IsSensitive)
	if err != nil || sensitivity != domain.ConfigSensitivityNormal {
		return "[PROTECTED]"
	}
	return value
}

// configAssetType maps the typed configuration contract to the restricted
// file facade. IMAGE and FILE are intentionally absent from every generic
// file-binding command; only this mapping can create CONFIG_ASSET references.
func configAssetType(value domain.ConfigValueType) (filefacade.ConfigAssetType, bool) {
	switch value {
	case domain.ConfigValueImage:
		return filefacade.ConfigAssetImage, true
	case domain.ConfigValueFile:
		return filefacade.ConfigAssetFile, true
	default:
		return "", false
	}
}

// configAssetTypeMust is a defensive variant for persisted values. A row that
// somehow contains an unsupported value type remains a non-asset here; its
// normal typed validation path reports the invalid type before mutation.
func configAssetTypeMust(raw string) (filefacade.ConfigAssetType, bool) {
	value, err := domain.NormalizeValueType(raw)
	if err != nil {
		return "", false
	}
	return configAssetType(value)
}

func configAssetExposure(value domain.ConfigExposure) filefacade.ConfigAssetExposure {
	switch value {
	case domain.ConfigExposurePublic:
		return filefacade.ConfigAssetPublic
	case domain.ConfigExposureAuthenticated:
		return filefacade.ConfigAssetAuthenticated
	default:
		return filefacade.ConfigAssetInternal
	}
}

func configAssetAuditSnapshot(state filefacade.ConfigAssetBindingState) *domain.ConfigAssetBindingSnapshot {
	returnPtr := domain.NewConfigAssetBindingSnapshot(
		state.ConfigID,
		string(state.State),
		state.FileID,
		state.ScopeID,
		string(state.AssetType),
		string(state.Exposure),
	)
	return &returnPtr
}

func configAssetBindingStateFromAuditSnapshot(snapshot *domain.ConfigAssetBindingSnapshot) (filefacade.ConfigAssetBindingState, error) {
	if snapshot == nil {
		return filefacade.ConfigAssetBindingState{}, apperrors.Operation("配置资产历史记录缺少私有快照，不能回滚")
	}
	state := filefacade.ConfigAssetBindingState{
		ConfigID:  snapshot.ConfigID,
		State:     filefacade.ConfigAssetBindingKind(snapshot.State),
		FileID:    snapshot.FileID,
		ScopeID:   snapshot.ScopeID,
		AssetType: filefacade.ConfigAssetType(snapshot.AssetType),
		Exposure:  filefacade.ConfigAssetExposure(snapshot.Exposure),
	}
	return state, nil
}

func configAssetAuditSnapshotOrNil(command *filefacade.RestoreConfigAssetBindingCommand, expected bool) *domain.ConfigAssetBindingSnapshot {
	if command == nil {
		return nil
	}
	state := command.Restore
	if expected {
		state = command.Expected
	}
	return configAssetAuditSnapshot(state)
}

func validateConfigAssetSnapshotRuntimeValue(state filefacade.ConfigAssetBindingState, configID int64, runtimeValue string) error {
	stablePath := filefacade.ConfigAssetStablePath(configID)
	switch state.State {
	case filefacade.ConfigAssetBindingBound:
		if runtimeValue != stablePath {
			return apperrors.Operation("配置资产历史快照与稳定配置值不一致，不能安全回滚")
		}
	case filefacade.ConfigAssetBindingEmpty:
		if runtimeValue != "" {
			return apperrors.Operation("配置资产历史空快照与配置值不一致，不能安全回滚")
		}
	default:
		return apperrors.Operation("配置资产历史快照状态无效，不能安全回滚")
	}
	return nil
}

func toFacadeExtJSON(ext *domain.ConfigExtJSON) *configfacade.ConfigExtJSON {
	if ext == nil {
		return nil
	}
	result := &configfacade.ConfigExtJSON{}
	if len(ext.Enums) > 0 {
		result.Enums = append([]string(nil), ext.Enums...)
	}
	if ext.Secret != nil {
		result.Secret = &configfacade.ConfigSecretValue{
			Plain:         ext.Secret.Plain,
			CiphertextB64: ext.Secret.CiphertextB64,
			EDEKB64:       ext.Secret.EDEKB64,
			WrapKeyRef:    ext.Secret.WrapKeyRef,
		}
	}
	return result
}

func defaultInt(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeAccessFlag(value int) int {
	if value != 0 {
		return 1
	}
	return 0
}

func filteredTotal(original int64, originalRecords int, filteredRecords int, actor Actor) int64 {
	if actor.IsAdmin || originalRecords == filteredRecords {
		return original
	}
	return int64(filteredRecords)
}

func actorName(actor Actor) string {
	if strings.TrimSpace(actor.Nickname) != "" {
		return strings.TrimSpace(actor.Nickname)
	}
	if strings.TrimSpace(actor.Username) != "" {
		return strings.TrimSpace(actor.Username)
	}
	if actor.UserID == 0 {
		return "系统"
	}
	return ""
}

func lookupOperatorName(values map[int64]string, userID int64) string {
	if value, ok := values[userID]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	if userID == 0 {
		return "系统"
	}
	return "未知"
}

func dedupePositiveIDs(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, item := range values {
		if item <= 0 {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func canonicalConfigKeys(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func timePtrAfter(left, right *time.Time) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	return left.After(*right)
}

func changeLogLater(left, right domain.ConfigChangeLog) bool {
	if timePtrAfter(left.OperationTime, right.OperationTime) {
		return true
	}
	if timePtrAfter(right.OperationTime, left.OperationTime) {
		return false
	}
	return left.ID > right.ID
}

func timePtrBefore(left, right *time.Time) bool {
	if left == nil && right == nil {
		return false
	}
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	return left.Before(*right)
}

func ternary[T any](condition bool, whenTrue, whenFalse T) T {
	if condition {
		return whenTrue
	}
	return whenFalse
}

func sensitiveRevealBinding(configID int64) string {
	return "config:" + strconv.FormatInt(configID, 10) + "|reveal"
}

func configRollbackBinding(logID int64) string {
	return "config:rollback:" + strconv.FormatInt(logID, 10)
}

func configScopeAssignmentBinding(roleID int64, grants []configfacade.ConfigScopeGrantVO) string {
	items := make([]string, 0, len(grants))
	for _, grant := range grants {
		groupCode := strings.ToLower(strings.TrimSpace(grant.GroupCode))
		if groupCode == "" {
			continue
		}
		configKey := strings.ToLower(strings.TrimSpace(grant.ConfigKey))
		scope := groupCode
		if configKey != "" {
			scope += "." + configKey
		}
		scope += ":r" + strconv.Itoa(normalizeAccessFlag(grant.CanRead)) +
			"w" + strconv.Itoa(normalizeAccessFlag(grant.CanWrite)) +
			"d" + strconv.Itoa(normalizeAccessFlag(grant.CanDelete))
		items = append(items, scope)
	}
	sort.Strings(items)
	return "config-scope:role:" + strconv.FormatInt(roleID, 10) + "|scopes:" + strings.Join(items, ",")
}
