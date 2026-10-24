package application

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	setupdomain "github.com/CaixyPromise/seven-agent/force/internal/app/setup/domain"
	setupfacade "github.com/CaixyPromise/seven-agent/force/internal/app/setup/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/google/uuid"
)

type StateStore interface {
	ConsumeNonce(ctx context.Context, nonce string, ttl time.Duration) (bool, error)
	AcquireBootstrapLock(ctx context.Context, token string, ttl time.Duration) (bool, error)
	ReleaseBootstrapLock(ctx context.Context, token string) error
}

type Settings struct {
	Setup              config.SetupConfig
	LoginEnabled       bool
	SSOFrontendPrimary bool
	AppVersion         string
	AppCommit          string
	StartTime          time.Time
}

type Service struct {
	settings            Settings
	tokens              *setupdomain.TokenService
	state               StateStore
	transactor          store.Transactor
	users               userfacade.ProvisioningFacade
	relations           userfacade.UserRelationFacade
	roles               authorizationfacade.RoleFacade
	permissions         authorizationfacade.PermissionFacade
	ssoBootstrap        ssofacade.BootstrapSessionFacade
	now                 func() time.Time
	mismatchWarningOnce sync.Once
}

func NewService(
	settings Settings,
	tokens *setupdomain.TokenService,
	state StateStore,
	transactor store.Transactor,
	users userfacade.ProvisioningFacade,
	relations userfacade.UserRelationFacade,
	roles authorizationfacade.RoleFacade,
	permissions authorizationfacade.PermissionFacade,
	ssoBootstrap ssofacade.BootstrapSessionFacade,
) *Service {
	return &Service{
		settings:     settings,
		tokens:       tokens,
		state:        state,
		transactor:   transactor,
		users:        users,
		relations:    relations,
		roles:        roles,
		permissions:  permissions,
		ssoBootstrap: ssoBootstrap,
		now:          func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) GetSetupStatus(ctx context.Context) (*setupfacade.SetupStatusDTO, error) {
	if s == nil {
		return nil, apperrors.System("setup service未配置")
	}
	if !s.settings.Setup.Enabled {
		return &setupfacade.SetupStatusDTO{
			Initialized:   true,
			OwnerRequired: false,
			LoginEnabled:  true,
			AppVersion:    metadataValue(s.settings.AppVersion),
			AppCommit:     metadataValue(s.settings.AppCommit),
			StartTime:     s.settings.StartTime.UTC().Format(time.RFC3339Nano),
			SetupToken:    nil,
		}, nil
	}
	initialized, err := s.hasSuperAdminAccount(ctx)
	if err != nil {
		return nil, err
	}
	var setupToken *string
	if !initialized {
		token, err := s.tokens.Generate(s.now())
		if err != nil {
			return nil, err
		}
		setupToken = &token
	}
	return &setupfacade.SetupStatusDTO{
		Initialized:   initialized,
		OwnerRequired: !initialized,
		LoginEnabled:  initialized && s.settings.LoginEnabled && s.settings.SSOFrontendPrimary,
		AppVersion:    metadataValue(s.settings.AppVersion),
		AppCommit:     metadataValue(s.settings.AppCommit),
		StartTime:     s.settings.StartTime.UTC().Format(time.RFC3339Nano),
		SetupToken:    setupToken,
	}, nil
}

func (s *Service) CreateOwner(ctx context.Context, request setupfacade.SetupOwnerRequestDTO, setupToken string, requestContext *ssofacade.RequestContext) (*setupfacade.OwnerBootstrapResult, error) {
	if s == nil {
		return nil, apperrors.System("setup service未配置")
	}
	if !s.settings.Setup.Enabled {
		return nil, setupCompleted("系统已完成初始化")
	}
	if err := validateOwnerRequest(request); err != nil {
		return nil, err
	}
	payload, err := s.tokens.Validate(setupToken, s.now())
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(maxInt64(payload.Exp-s.now().UTC().Unix(), 1)) * time.Second
	consumed, err := s.state.ConsumeNonce(ctx, payload.Nonce, ttl)
	if err != nil {
		return nil, err
	}
	if !consumed {
		return nil, invalidSetupToken("初始化校验无效，请刷新页面后重试")
	}
	lockToken := uuid.NewString()
	locked, err := s.state.AcquireBootstrapLock(ctx, lockToken, time.Duration(maxInt64(s.settings.Setup.OwnerBootstrapLockSeconds, 5))*time.Second)
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, setupCompleted("系统初始化正在执行，请稍后重试")
	}
	defer func() { _ = s.state.ReleaseBootstrapLock(ctx, lockToken) }()

	var owner *userfacade.ProvisionedUser
	var session *ssofacade.BootstrapSessionResult
	if err := s.withTransaction(ctx, func(txCtx context.Context) error {
		if initialized, err := s.hasSuperAdminAccount(txCtx); err != nil {
			return err
		} else if initialized {
			return setupCompleted("系统已完成初始化")
		}
		account := strings.TrimSpace(request.Username)
		if existing, err := s.users.FindUserByAccount(txCtx, account); err != nil {
			return err
		} else if existing != nil {
			return apperrors.Params("用户名已存在")
		}
		root, err := s.roles.BootstrapAuthorizationRoot(txCtx, authorizationfacade.BootstrapAuthorizationRootCommand{
			Code:          s.settings.Setup.Bootstrap.SuperAdminRoleCode,
			Name:          s.settings.Setup.Bootstrap.SuperAdminRoleName,
			InitializedAt: s.now(),
		})
		if err != nil {
			return err
		}
		if root.AlreadyInitialized {
			return setupCompleted("系统已完成初始化")
		}
		roleID := root.Role.RoleID
		nickname := strings.TrimSpace(request.Nickname)
		if nickname == "" {
			nickname = account
		}
		created, err := s.users.CreateOwnerUser(txCtx, userfacade.CreateOwnerUserCommand{
			AccountName: account,
			NickName:    nickname,
			RawPassword: request.Password,
		})
		if err != nil {
			return err
		}
		if err := s.roles.BootstrapOwnerRoles(txCtx, authorizationfacade.BootstrapOwnerRolesCommand{
			UserID:     int64(created.UserID),
			RoleIDs:    []int64{roleID},
			OperatorID: created.UserID,
		}); err != nil {
			return err
		}
		bootstrapped, err := s.ssoBootstrap.BootstrapFirstPartySession(txCtx, ssofacade.BootstrapSessionCommand{
			UserID:         created.UserID,
			ClientID:       strings.TrimSpace(s.settings.Setup.BootstrapClientID),
			RequestContext: requestContext,
		})
		if err != nil {
			return err
		}
		owner = created
		session = bootstrapped
		return nil
	}); err != nil {
		return nil, err
	}
	if owner == nil || session == nil {
		return nil, apperrors.System("owner初始化结果为空")
	}
	if err := s.permissions.RefreshUserPermissionCache(ctx, owner.UserID); err != nil {
		return nil, err
	}
	permissions, err := s.permissions.GetUserPermissions(ctx, owner.UserID)
	if err != nil {
		return nil, err
	}
	roleCodes, err := s.permissions.GetUserRoles(ctx, owner.UserID)
	if err != nil {
		return nil, err
	}
	return &setupfacade.OwnerBootstrapResult{
		Owner: &setupfacade.SetupOwnerResultDTO{
			ID:           owner.UserID,
			Username:     owner.AccountName,
			Nickname:     owner.NickName,
			UserAvatar:   owner.Avatar,
			Permissions:  sortedStrings(permissions),
			RoleCodes:    sortedStrings(roleCodes),
			AccessToken:  session.AccessToken,
			TokenType:    session.TokenType,
			AccessTTLSec: session.AccessTTLSeconds,
		},
		SessionCookieHeaderValue: session.SessionCookieHeaderValue,
		RefreshCookieHeaderValue: session.RefreshCookieHeaderValue,
	}, nil
}

func (s *Service) withTransaction(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("setup owner bootstrap requires datasource transaction")
	}
	consistent, ok := s.transactor.(store.ConsistentTransactor)
	if !ok || !consistent.Enabled() {
		return apperrors.System("setup owner bootstrap requires consistent datasource transaction")
	}
	return consistent.WithinConsistentTransaction(ctx, fn)
}

func (s *Service) hasSuperAdminAccount(ctx context.Context) (bool, error) {
	roleID, err := s.findAuthorizationRootRoleID(ctx)
	if err != nil || roleID <= 0 {
		return false, err
	}
	if s.relations == nil {
		return false, apperrors.System("system user relation facade未配置")
	}
	audience, ok := s.relations.(userfacade.NotificationAudienceFacade)
	if !ok {
		return false, apperrors.System("system user bounded audience facade未配置")
	}
	userIDs, err := audience.ListActiveUserIDsByRoleIDPage(ctx, roleID, 0, 1)
	if err != nil {
		return false, err
	}
	return len(userIDs) > 0, nil
}

func (s *Service) findAuthorizationRootRoleID(ctx context.Context) (int64, error) {
	if s.roles == nil {
		return 0, apperrors.System("authorization role facade未配置")
	}
	items, err := s.roles.GetRoleList(ctx)
	if err != nil {
		return 0, err
	}
	var result int64
	for _, item := range items {
		if item.AuthorizationRoot && item.Status == 0 && item.RoleID > 0 {
			configuredCode := strings.TrimSpace(s.settings.Setup.Bootstrap.SuperAdminRoleCode)
			if configuredCode != "" && !strings.EqualFold(configuredCode, strings.TrimSpace(item.Code)) {
				s.mismatchWarningOnce.Do(func() {
					log.Printf("event=authorization_root_config_mismatch configuredCode=%q persistedCode=%q rootRoleId=%d", configuredCode, item.Code, item.RoleID)
				})
			}
			if result == 0 || item.RoleID < result {
				result = item.RoleID
			}
		}
	}
	return result, nil
}

func validateOwnerRequest(request setupfacade.SetupOwnerRequestDTO) error {
	if strings.TrimSpace(request.Username) == "" {
		return apperrors.Params("用户名不能为空")
	}
	if strings.TrimSpace(request.Password) == "" {
		return apperrors.Params("密码不能为空")
	}
	if strings.TrimSpace(request.ConfirmPassword) == "" {
		return apperrors.Params("确认密码不能为空")
	}
	if request.Password != request.ConfirmPassword {
		return apperrors.Params("两次输入的密码不一致")
	}
	return nil
}

func invalidSetupToken(message string) error {
	if strings.TrimSpace(message) == "" {
		message = "初始化校验无效，请刷新页面后重试"
	}
	return apperrors.New(apperrors.CodeNoAuth, apperrors.KindForbidden, message)
}

func setupCompleted(message string) error {
	if strings.TrimSpace(message) == "" {
		message = "系统已完成初始化"
	}
	return apperrors.Operation(message)
}

func sortedStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func metadataValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "dev"
	}
	return strings.TrimSpace(value)
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}
