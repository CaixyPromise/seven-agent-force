package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	credentialfacade "github.com/CaixyPromise/seven-agent/force/internal/app/credential/facade"
	filefacade "github.com/CaixyPromise/seven-agent/force/internal/app/file/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	cachegovernancefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	passwordinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/password"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

const (
	stepUpActionRBACAssignUserRoles   = "RBAC_ASSIGN_USER_ROLES"
	stepUpActionRBACAssignPostRoles   = "RBAC_ASSIGN_POST_ROLES"
	stepUpActionAdminDeleteUser       = "ADMIN_DELETE_USER"
	stepUpActionAdminChangeUserStatus = "ADMIN_CHANGE_USER_STATUS"
	adminMutationBatchMax             = 100
)

type postBatchRepository interface {
	ListReferencedPostIDs(ctx context.Context, postIDs []int64) ([]int64, error)
	DeletePosts(ctx context.Context, postIDs []int64) error
}

type Service struct {
	repo            domain.Repository
	domain          *domain.Service
	password        *passwordinfra.Service
	credentials     credentialfacade.UserCredentialFacade
	files           filefacade.FileAssetBindingFacade
	permissions     authorizationfacade.PermissionFacade
	roleAssignments authorizationfacade.UserRoleAssignmentFacade
	sessions        ssofacade.SessionFacade
	managedSessions ssofacade.ManagedSessionFacade
	transactor      store.Transactor
	idGen           *xid.Generator
	invalidations   cachegovernancefacade.InvalidationRegistrar
}

type Option func(*Service)

func WithTransactor(transactor store.Transactor) Option {
	return func(s *Service) {
		s.transactor = transactor
	}
}

func WithIDGenerator(idGen *xid.Generator) Option {
	return func(s *Service) {
		s.idGen = idGen
	}
}

func NewService(
	repo domain.Repository,
	domainService *domain.Service,
	password *passwordinfra.Service,
	credentials credentialfacade.UserCredentialFacade,
	options ...Option,
) *Service {
	service := &Service{
		repo:        repo,
		domain:      domainService,
		password:    password,
		credentials: credentials,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) BindCredentials(credentials credentialfacade.UserCredentialFacade) {
	s.credentials = credentials
}

// BindFileAssets installs the cross-module file asset Facade used by user
// application transactions.
func (s *Service) BindFileAssets(files filefacade.FileAssetBindingFacade) {
	s.files = files
}

func (s *Service) BindPermissions(permissions authorizationfacade.PermissionFacade) {
	s.permissions = permissions
}

func (s *Service) BindRoleAssignments(assignments authorizationfacade.UserRoleAssignmentFacade) {
	s.roleAssignments = assignments
}

func (s *Service) BindSessions(sessions ssofacade.SessionFacade) {
	s.sessions = sessions
}

// BindManagedSessions binds cutoff-based session revocation for trusted Node commands.
func (s *Service) BindManagedSessions(sessions ssofacade.ManagedSessionFacade) {
	s.managedSessions = sessions
}

// BindCacheInvalidations installs the shared cache-governance Facade after
// module composition. User writes use its durable transaction protocol rather
// than an authorization module implementation or local cache deletion.
func (s *Service) BindCacheInvalidations(registrar cachegovernancefacade.InvalidationRegistrar) {
	if s != nil {
		s.invalidations = registrar
	}
}

func (s *Service) FindSubjectByID(ctx context.Context, userID int64) (*userfacade.SubjectRecord, error) {
	if userID <= 0 {
		return nil, apperrors.Params("userId不能为空")
	}
	record, err := s.repo.FindSubjectByID(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	return s.toSubjectRecord(record), nil
}

func (s *Service) FindSubjectByAccount(ctx context.Context, account string) (*userfacade.SubjectRecord, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, apperrors.Params("account不能为空")
	}
	record, err := s.repo.FindSubjectByAccount(ctx, account)
	if err != nil || record == nil {
		return nil, err
	}
	return s.toSubjectRecord(record), nil
}

func (s *Service) FindSubjectByEmail(ctx context.Context, email string) (*userfacade.SubjectRecord, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, nil
	}
	record, err := s.repo.FindSubjectByEmail(ctx, email)
	if err != nil || record == nil {
		return nil, err
	}
	subject := s.toSubjectRecord(record)
	if subject == nil || !subject.Enabled || subject.LockStatus {
		return nil, nil
	}
	return subject, nil
}

func (s *Service) CreateExternalSubject(ctx context.Context, command userfacade.CreateExternalSubjectCommand) (*userfacade.SubjectRecord, error) {
	email := strings.ToLower(strings.TrimSpace(command.UserEmail))
	if email == "" || !s.domain.ValidateEmail(email) {
		return nil, apperrors.Params("外部登录邮箱无效")
	}
	if s.idGen == nil {
		return nil, apperrors.System("user id generator未配置")
	}
	if !command.DisableEmailMerge {
		if existing, err := s.FindSubjectByEmail(ctx, email); err != nil || existing != nil {
			return existing, err
		}
	}
	account := strings.TrimSpace(command.AccountName)
	if !s.domain.ValidateAccount(account) {
		account = externalAccountName(email)
	}
	uniqueEmail := email
	if command.DisableEmailMerge {
		uniqueEmail = ""
	}
	if err := s.ensureUniqueUserFields(ctx, 0, account, uniqueEmail, ""); err != nil {
		return nil, err
	}
	nickname := strings.TrimSpace(command.NickName)
	if nickname == "" {
		nickname = email
	}
	userID := s.idGen.NextID()
	roleIDs := normalizeIDs(command.DefaultRoleIDs)
	postIDs := normalizeIDs(command.DefaultPostIDs)
	var orgIDs []int64
	var primaryOrgID int64
	if command.DefaultOrgID != nil && *command.DefaultOrgID > 0 {
		primaryOrgID = *command.DefaultOrgID
		orgIDs = []int64{primaryOrgID}
	}
	var deptIDs []int64
	var primaryDeptID int64
	if command.DefaultDeptID != nil && *command.DefaultDeptID > 0 {
		primaryDeptID = *command.DefaultDeptID
		deptIDs = []int64{primaryDeptID}
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateExternalSubject(txCtx, domain.ExternalSubjectCreateRecord{
			ID:                   userID,
			AccountName:          account,
			NickName:             nickname,
			Email:                email,
			Avatar:               strings.TrimSpace(command.UserAvatar),
			RegisterPlatformCode: strings.TrimSpace(command.RegisterPlatformCode),
			RegisterProviderCode: strings.TrimSpace(command.RegisterProviderCode),
			Status:               0,
		}); err != nil {
			return err
		}
		if len(roleIDs) > 0 {
			if s.roleAssignments == nil {
				return apperrors.System("用户角色策略服务未配置")
			}
			if err := s.roleAssignments.AssignProvisionedUserRoles(txCtx, authorizationfacade.AssignProvisionedUserRolesCommand{
				UserID: int64(userID), RoleIDs: []int64(roleIDs),
			}); err != nil {
				return err
			}
		}
		if len(orgIDs) > 0 {
			if err := s.repo.ReplaceUserOrgs(txCtx, userID, orgIDs, primaryOrgID, 0); err != nil {
				return err
			}
		}
		if len(deptIDs) > 0 {
			if err := s.repo.ReplaceUserDepts(txCtx, userID, deptIDs, primaryDeptID, 0); err != nil {
				return err
			}
		}
		if len(postIDs) > 0 {
			return s.repo.ReplaceUserPosts(txCtx, userID, postIDs, 0, 0)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	_ = s.refreshUser(ctx, userID)
	return s.FindSubjectByID(ctx, userID)
}

func (s *Service) CreateFormSubject(ctx context.Context, command userfacade.CreateFormSubjectCommand) (*userfacade.SubjectRecord, error) {
	if s == nil || s.repo == nil {
		return nil, apperrors.System("user registration未配置")
	}
	if s.credentials == nil || s.password == nil {
		return nil, apperrors.System("user registration password credential未配置")
	}
	if s.idGen == nil {
		return nil, apperrors.System("user registration id generator未配置")
	}
	account := strings.TrimSpace(command.AccountName)
	if account == "" {
		return nil, apperrors.Params("用户名不能为空")
	}
	if !s.domain.ValidateAccount(account) {
		return nil, apperrors.Params("用户名格式错误")
	}
	email := strings.ToLower(strings.TrimSpace(command.UserEmail))
	if email == "" || !s.domain.ValidateEmail(email) {
		return nil, apperrors.Params("邮箱格式错误")
	}
	if !s.domain.ValidatePassword(command.RawPassword) {
		return nil, apperrors.Params("密码不符合要求")
	}
	if err := s.ensureUniqueUserFields(ctx, 0, account, email, ""); err != nil {
		return nil, apperrors.Operation("注册信息不可用或已被占用")
	}
	nickname := strings.TrimSpace(command.NickName)
	if nickname == "" {
		nickname = account
	}
	if len([]rune(nickname)) > 30 {
		return nil, apperrors.Params("用户昵称不能超过 30 个字符")
	}
	userID := s.idGen.NextID()
	now := time.Now().UTC()
	hash, err := s.password.Hash(ctx, command.RawPassword)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeSystemError, apperrors.KindSystem, "密码加密失败", err)
	}
	roleIDs := normalizeIDs(command.DefaultRoleIDs)
	postIDs := normalizeIDs(command.DefaultPostIDs)
	var orgIDs []int64
	var primaryOrgID int64
	if command.DefaultOrgID != nil && *command.DefaultOrgID > 0 {
		primaryOrgID = *command.DefaultOrgID
		orgIDs = []int64{primaryOrgID}
	}
	var deptIDs []int64
	var primaryDeptID int64
	if command.DefaultDeptID != nil && *command.DefaultDeptID > 0 {
		primaryDeptID = *command.DefaultDeptID
		deptIDs = []int64{primaryDeptID}
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateFormSubject(txCtx, domain.FormSubjectCreateRecord{
			ID:                   userID,
			AccountName:          account,
			NickName:             nickname,
			Email:                email,
			RegisterPlatformCode: strings.TrimSpace(command.RegisterPlatformCode),
			RegisterProviderCode: "form",
			Status:               0,
		}); err != nil {
			return err
		}
		if err := s.credentials.UpsertPasswordCredential(txCtx, credentialfacade.UpsertPasswordCredentialCommand{
			UserID:             userID,
			PasswordHash:       hash,
			MustChangePassword: false,
			PasswordChangedAt:  &now,
		}); err != nil {
			return err
		}
		if len(roleIDs) > 0 {
			if s.roleAssignments == nil {
				return apperrors.System("用户角色策略服务未配置")
			}
			if err := s.roleAssignments.AssignProvisionedUserRoles(txCtx, authorizationfacade.AssignProvisionedUserRolesCommand{
				UserID: int64(userID), RoleIDs: []int64(roleIDs),
			}); err != nil {
				return err
			}
		}
		if len(orgIDs) > 0 {
			if err := s.repo.ReplaceUserOrgs(txCtx, userID, orgIDs, primaryOrgID, 0); err != nil {
				return err
			}
		}
		if len(deptIDs) > 0 {
			if err := s.repo.ReplaceUserDepts(txCtx, userID, deptIDs, primaryDeptID, 0); err != nil {
				return err
			}
		}
		if len(postIDs) > 0 {
			return s.repo.ReplaceUserPosts(txCtx, userID, postIDs, 0, 0)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	_ = s.refreshUser(ctx, userID)
	return s.FindSubjectByID(ctx, userID)
}

func externalAccountName(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return "u" + hex.EncodeToString(sum[:])[:15]
}

func (s *Service) ExistsByID(ctx context.Context, userID int64) (bool, error) {
	if userID <= 0 {
		return false, apperrors.Params("userId不能为空")
	}
	return s.repo.ExistsByID(ctx, userID)
}

func (s *Service) BuildPrincipalSeed(ctx context.Context, userID int64) (*userfacade.UserPrincipalSeed, error) {
	record, err := s.repo.FindSubjectByID(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	return &userfacade.UserPrincipalSeed{
		UserID:      record.UserID,
		AccountName: record.AccountName,
		NickName:    record.NickName,
		Email:       record.Email,
		Phone:       record.Phone,
		Enabled:     s.domain.Enabled(record.Status),
	}, nil
}

func (s *Service) GetProfileByUserID(ctx context.Context, userID int64) (*userfacade.UserProfile, error) {
	if userID <= 0 {
		return nil, apperrors.Params("userId不能为空")
	}
	record, err := s.repo.FindSubjectByID(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	var passwordChangedAt *time.Time
	if s.credentials != nil {
		credential, credentialErr := s.credentials.FindActivePasswordByUserID(ctx, userID)
		if credentialErr != nil {
			return nil, credentialErr
		}
		if credential != nil {
			passwordChangedAt = credential.PasswordChangedAt
		}
	}
	return &userfacade.UserProfile{
		UserID:            record.UserID,
		AccountName:       record.AccountName,
		NickName:          record.NickName,
		Email:             record.Email,
		Phone:             record.Phone,
		Avatar:            record.Avatar,
		Profile:           record.Profile,
		Enabled:           s.domain.Enabled(record.Status),
		PasswordChangedAt: passwordChangedAt,
		EmailVerified:     record.EmailVerifiedAt != nil,
		EmailVerifiedAt:   record.EmailVerifiedAt,
		EmailVersion:      normalizedEmailVersion(record.EmailVersion),
	}, nil
}

func (s *Service) UpdateSelfProfile(ctx context.Context, command userfacade.UpdateSelfProfileCommand) error {
	if command.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	record, err := s.repo.FindSubjectByID(ctx, command.UserID)
	if err != nil {
		return err
	}
	if record == nil {
		return apperrors.NotFound("当前用户不存在")
	}

	var phoneToStore *string
	if command.UserPhone != nil {
		trimmed := strings.TrimSpace(*command.UserPhone)
		if trimmed != "" && trimmed != record.Phone {
			if !s.domain.ValidatePhone(trimmed) {
				return apperrors.Params("手机号格式错误")
			}
			count, err := s.repo.CountByPhoneExcludingUserID(ctx, command.UserID, trimmed)
			if err != nil {
				return err
			}
			if count > 0 {
				return apperrors.Params("手机号已存在")
			}
			phoneToStore = &trimmed
		}
	}
	return s.repo.UpdateProfile(ctx, command.UserID, sanitizeOptional(command.NickName), phoneToStore, sanitizeOptional(command.UserAvatar), command.UserProfile)
}

func (s *Service) UpdateSelfEmail(ctx context.Context, command userfacade.UpdateSelfEmailCommand) error {
	if command.UserID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	record, err := s.repo.FindSubjectByID(ctx, command.UserID)
	if err != nil {
		return err
	}
	if record == nil {
		return apperrors.NotFound("当前用户不存在")
	}
	if command.ExpectedEmailVersion <= 0 {
		return apperrors.Params("expectedEmailVersion不能为空")
	}
	if command.ExpectedEmailVersion != normalizedEmailVersion(record.EmailVersion) {
		return emailVersionConflict()
	}
	requestedEmail := strings.TrimSpace(command.UserEmail)
	if requestedEmail == "" {
		return apperrors.Params("新邮箱不能为空")
	}
	if !s.domain.ValidateEmail(requestedEmail) {
		return apperrors.Params("邮箱格式错误")
	}
	if requestedEmail == strings.TrimSpace(record.Email) {
		return apperrors.Params("新旧邮箱不能一致")
	}
	count, err := s.repo.CountByEmailExcludingUserID(ctx, command.UserID, requestedEmail)
	if err != nil {
		return err
	}
	if count > 0 {
		return apperrors.Params("邮箱已存在")
	}
	updated, err := s.repo.CompareAndSetVerifiedEmail(ctx, command.UserID, requestedEmail, command.ExpectedEmailVersion, "CHALLENGE_VERIFY_NEW", time.Now().UTC())
	if err != nil {
		return err
	}
	if !updated {
		return emailVersionConflict()
	}
	return nil
}

func (s *Service) ResolveVerifiedNotificationEmail(ctx context.Context, userID int64) (string, bool, error) {
	if userID <= 0 {
		return "", false, nil
	}
	record, err := s.repo.FindSubjectByID(ctx, userID)
	if err != nil || record == nil {
		return "", false, err
	}
	email := strings.ToLower(strings.TrimSpace(record.Email))
	if record.Status != domain.UserStatusNormal || record.EmailVerifiedAt == nil || email == "" || !s.domain.ValidateEmail(email) {
		return "", false, nil
	}
	return email, true, nil
}

func normalizedEmailVersion(value int64) int64 {
	if value <= 0 {
		return 1
	}
	return value
}

func emailVersionConflict() error {
	return apperrors.ObjectState("邮箱设置已在其他位置更新，请刷新后重试").WithDetails(map[string]string{"reasonCode": "PROFILE_EMAIL_VERSION_CONFLICT"})
}

// CommitCurrentUserAvatar authorizes the user target and atomically replaces
// the file reference and persisted avatar value in one shared transaction.
func (s *Service) CommitCurrentUserAvatar(ctx context.Context, userID, fileID int64) (string, error) {
	if userID <= 0 || fileID <= 0 {
		return "", apperrors.Params("用户ID和文件ID不能为空")
	}
	currentUser := securitycontext.FromContext(ctx)
	if currentUser == nil || currentUser.IsAnonymous || currentUser.UserID != userID {
		return "", apperrors.Forbidden("只能修改当前用户头像")
	}
	if _, err := securitycontext.ResolveOrganizationScope(currentUser); err != nil {
		return "", apperrors.Forbidden("当前认证上下文缺少明确组织范围")
	}
	if s.files == nil {
		return "", apperrors.System("file asset facade未配置")
	}
	record, err := s.repo.FindSubjectByID(ctx, userID)
	if err != nil {
		return "", err
	}
	if record == nil || record.Status != 0 {
		return "", apperrors.NotFound("当前用户不存在或不可用")
	}
	var avatar string
	if err := s.withTx(ctx, func(txCtx context.Context) error {
		ref, err := s.files.BindUploadedFile(txCtx, filefacade.BindUploadedFileCommand{
			FileID: fileID,
			Slot:   filefacade.FileAssetSlotUserAvatar,
		})
		if err != nil {
			return err
		}
		if ref == nil || strings.TrimSpace(ref.VisitURL) == "" ||
			ref.AccessScope != string(filefacade.AccessPublic) ||
			ref.VisitStrategy != string(filefacade.VisitPublicStatic) {
			return apperrors.Forbidden("头像文件未完成安全绑定")
		}
		avatar = strings.TrimSpace(ref.VisitURL)
		return s.repo.UpdateProfile(txCtx, userID, nil, nil, &avatar, nil)
	}); err != nil {
		return "", err
	}
	return avatar, nil
}

func (s *Service) SyncExternalProfile(ctx context.Context, command userfacade.SyncExternalProfileCommand) error {
	if command.UserID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	record, err := s.repo.FindSubjectByID(ctx, command.UserID)
	if err != nil {
		return err
	}
	if record == nil {
		return apperrors.NotFound("当前用户不存在")
	}

	nickName := strings.TrimSpace(command.NickName)
	if nickName == "" {
		nickName = strings.TrimSpace(command.ExternalLogin)
	}
	var nickNameToStore *string
	if nickName != "" && nickName != strings.TrimSpace(record.NickName) {
		nickNameToStore = &nickName
	}
	avatar := strings.TrimSpace(command.UserAvatar)
	var avatarToStore *string
	if avatar != "" && avatar != strings.TrimSpace(record.Avatar) {
		avatarToStore = &avatar
	}
	if err := s.repo.UpdateProfile(ctx, command.UserID, nickNameToStore, nil, avatarToStore, nil); err != nil {
		return err
	}

	email := strings.ToLower(strings.TrimSpace(command.UserEmail))
	if !command.EmailVerified || email == "" {
		return nil
	}
	if !s.domain.ValidateEmail(email) {
		return nil
	}
	count, err := s.repo.CountByEmailExcludingUserID(ctx, command.UserID, email)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	updated, err := s.repo.CompareAndSetVerifiedEmail(ctx, command.UserID, email, normalizedEmailVersion(record.EmailVersion), "OAUTH:"+strings.ToUpper(strings.TrimSpace(command.ProviderCode)), time.Now().UTC())
	if err != nil {
		return err
	}
	if !updated {
		return emailVersionConflict()
	}
	return nil
}

func (s *Service) VerifyPassword(ctx context.Context, userID int64, rawPassword string) (bool, error) {
	if userID <= 0 || strings.TrimSpace(rawPassword) == "" {
		return false, nil
	}
	if s.credentials == nil || s.password == nil {
		return false, apperrors.System("password verification未配置")
	}
	credential, err := s.credentials.FindActivePasswordByUserID(ctx, userID)
	if err != nil || credential == nil {
		return false, err
	}
	return s.password.Verify(ctx, rawPassword, credential.PasswordHash) == nil, nil
}

func (s *Service) UpdatePassword(ctx context.Context, command userfacade.UpdatePasswordCommand) error {
	if command.UserID <= 0 {
		return apperrors.Params("用户不存在")
	}
	if s.credentials == nil || s.password == nil {
		return apperrors.System("password update未配置")
	}
	if !s.domain.ValidatePassword(command.RawPassword) {
		return apperrors.Params("密码不符合要求")
	}
	if s.sessions == nil {
		return apperrors.Operation("SSO会话回收能力未配置")
	}
	hash, err := s.password.Hash(ctx, command.RawPassword)
	if err != nil {
		return apperrors.Wrap(apperrors.CodeSystemError, apperrors.KindSystem, "密码加密失败", err)
	}
	now := time.Now().UTC()
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.credentials.UpsertPasswordCredential(txCtx, credentialfacade.UpsertPasswordCredentialCommand{
			UserID:             command.UserID,
			PasswordHash:       hash,
			MustChangePassword: false,
			PasswordChangedAt:  &now,
			CreatorID:          pointerInt64(command.OperatorID),
			UpdaterID:          pointerInt64(command.OperatorID),
		}); err != nil {
			return err
		}
		_, err := s.sessions.RevokeSessionsByUserID(txCtx, command.UserID)
		return err
	})
}

func (s *Service) UpdateLockState(ctx context.Context, command userfacade.UpdateLockStateCommand) error {
	if command.UserID <= 0 {
		return apperrors.Params("userId不能为空")
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if command.Status != domain.UserStatusNormal {
			if err := s.guardUserDeactivation(txCtx, command.UserID); err != nil {
				return err
			}
			if s.sessions == nil {
				return apperrors.Operation("SSO会话回收能力未配置")
			}
		}
		if err := s.repo.UpdateLockState(txCtx, command.UserID, command.Status, command.UnsealTime); err != nil {
			return err
		}
		if command.Status != domain.UserStatusNormal {
			_, err := s.sessions.RevokeSessionsByUserID(txCtx, command.UserID)
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) FindUserByAccount(ctx context.Context, account string) (*userfacade.SubjectRecord, error) {
	return s.FindSubjectByAccount(ctx, account)
}

func (s *Service) CreateOwnerUser(ctx context.Context, command userfacade.CreateOwnerUserCommand) (*userfacade.ProvisionedUser, error) {
	if s == nil || s.repo == nil {
		return nil, apperrors.System("user provisioning未配置")
	}
	if s.credentials == nil || s.password == nil {
		return nil, apperrors.System("user provisioning password credential未配置")
	}
	if s.idGen == nil {
		return nil, apperrors.System("user provisioning id generator未配置")
	}
	account := strings.TrimSpace(command.AccountName)
	if account == "" {
		return nil, apperrors.Params("用户名不能为空")
	}
	if !s.domain.ValidateAccount(account) {
		return nil, apperrors.Params("用户名格式错误")
	}
	if !s.domain.ValidatePassword(command.RawPassword) {
		return nil, apperrors.Params("密码不符合要求")
	}
	nickname := strings.TrimSpace(command.NickName)
	if nickname == "" {
		nickname = account
	}
	existing, err := s.repo.FindSubjectByAccount(ctx, account)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, apperrors.Params("用户名已存在")
	}
	userID := s.idGen.NextID()
	now := time.Now().UTC()
	hash, err := s.password.Hash(ctx, command.RawPassword)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.CodeSystemError, apperrors.KindSystem, "密码加密失败", err)
	}
	create := func(txCtx context.Context) error {
		if err := s.repo.CreateOwnerUser(txCtx, &domain.OwnerUserRecord{
			UserID:      userID,
			AccountName: account,
			NickName:    nickname,
			Email:       "",
			Avatar:      "",
			Profile:     "",
			Status:      0,
			Gender:      0,
		}); err != nil {
			return err
		}
		return s.credentials.UpsertPasswordCredential(txCtx, credentialfacade.UpsertPasswordCredentialCommand{
			UserID:             userID,
			PasswordHash:       hash,
			MustChangePassword: false,
			PasswordChangedAt:  &now,
			CreatorID:          pointerInt64(userID),
			UpdaterID:          pointerInt64(userID),
		})
	}
	if s.transactor != nil && s.transactor.Enabled() {
		if err := s.transactor.WithinTransaction(ctx, create); err != nil {
			return nil, err
		}
	} else if err := create(ctx); err != nil {
		return nil, err
	}
	return &userfacade.ProvisionedUser{
		UserID:      userID,
		AccountName: account,
		NickName:    nickname,
		Avatar:      "",
	}, nil
}

func (s *Service) QueryUsers(ctx context.Context, query userfacade.AdminUserQuery) (*userfacade.PageResult[userfacade.AdminUserVO], error) {
	domainQuery := domain.AdminUserQuery{
		Current: query.Current, Size: query.Size, Account: query.Username, Nickname: query.Nickname,
		Status: query.Status, OrgID: query.OrgID, DeptID: query.DeptID, PostID: query.PostID, Scope: toDomainScope(query.Scope),
	}
	var records []domain.AdminUserRecord
	var total int64
	load := func(readCtx context.Context) error {
		var err error
		records, total, err = s.repo.QueryAdminUsers(readCtx, domainQuery)
		return err
	}
	snapshotter, ok := s.transactor.(store.Snapshotter)
	if !ok || !snapshotter.Enabled() {
		return nil, apperrors.System("用户查询一致性快照能力未配置")
	}
	err := snapshotter.WithinReadOnlySnapshot(ctx, load)
	if err != nil {
		return nil, err
	}
	result := &userfacade.PageResult[userfacade.AdminUserVO]{Current: defaultPage(query.Current), Size: defaultSize(query.Size), Total: total, Records: make([]userfacade.AdminUserVO, 0, len(records))}
	for _, record := range records {
		vo := s.toAdminUserVO(record)
		result.Records = append(result.Records, vo)
	}
	return result, nil
}

func (s *Service) GetAdminUser(ctx context.Context, userID int64) (*userfacade.AdminUserVO, error) {
	if userID <= 0 {
		return nil, apperrors.Params("用户ID不能为空")
	}
	snapshotter, ok := s.transactor.(store.Snapshotter)
	if !ok || snapshotter == nil || !snapshotter.Enabled() {
		return nil, apperrors.System("用户详情一致性快照能力未配置")
	}
	var result *userfacade.AdminUserVO
	if err := snapshotter.WithinReadOnlySnapshot(ctx, func(readCtx context.Context) error {
		record, err := s.repo.FindAdminUserByID(readCtx, userID)
		if err != nil {
			return err
		}
		if record == nil {
			return apperrors.NotFound("用户不存在")
		}
		vo := s.toAdminUserVO(*record)
		if vo.RoleIDs, err = s.repo.ListUserRoleIDs(readCtx, userID); err != nil {
			return err
		}
		if vo.OrgIDs, err = s.repo.ListUserOrgIDs(readCtx, userID); err != nil {
			return err
		}
		if vo.DeptIDs, err = s.repo.ListUserDeptIDs(readCtx, userID); err != nil {
			return err
		}
		if vo.PostIDs, err = s.repo.ListUserPostIDs(readCtx, userID); err != nil {
			return err
		}
		result = &vo
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) CreateAdminUser(ctx context.Context, command userfacade.AdminUserCreateCommand) (int64, error) {
	account := strings.TrimSpace(command.Username)
	if account == "" {
		return 0, apperrors.Params("用户名不能为空")
	}
	if !s.domain.ValidateAccount(account) {
		return 0, apperrors.Params("用户名格式错误")
	}
	if !s.domain.ValidatePassword(command.Password) {
		return 0, apperrors.Params("密码不符合要求")
	}
	roleIDs := command.RoleIDs
	if len(roleIDs) > 0 {
		if s.roleAssignments == nil {
			return 0, apperrors.System("用户角色策略服务未配置")
		}
		if err := s.roleAssignments.ValidateCreatedUserRoles(ctx, authorizationfacade.AssignCreatedUserRolesCommand{
			Username: account, RoleIDs: []int64(roleIDs),
			OperatorID: command.OperatorID, StepUpProof: command.StepUpProof,
		}); err != nil {
			return 0, err
		}
	}
	if s.idGen == nil {
		return 0, apperrors.System("user admin id generator未配置")
	}
	if err := s.ensureUniqueUserFields(ctx, 0, account, command.Email, command.UserPhone); err != nil {
		return 0, err
	}
	userID := s.idGen.NextID()
	status := intValue(command.Status, 0)
	if !s.domain.ValidUserStatus(status) {
		return 0, apperrors.Params("用户状态错误")
	}
	nickname := strings.TrimSpace(command.Nickname)
	if nickname == "" {
		nickname = account
	}
	hash, err := s.password.Hash(ctx, command.Password)
	if err != nil {
		return 0, apperrors.Wrap(apperrors.CodeSystemError, apperrors.KindSystem, "密码加密失败", err)
	}
	now := time.Now().UTC()
	err = s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateAdminUser(txCtx, domain.AdminUserCreateRecord{
			ID: userID, AccountName: account, NickName: nickname, Email: command.Email, Phone: command.UserPhone,
			Gender: command.UserGender, Profile: command.Remark, Status: status, OperatorID: command.OperatorID,
		}); err != nil {
			return err
		}
		if s.credentials == nil {
			return apperrors.System("password credential未配置")
		}
		if err := s.credentials.UpsertPasswordCredential(txCtx, credentialfacade.UpsertPasswordCredentialCommand{
			UserID: userID, PasswordHash: hash, PasswordChangedAt: &now, CreatorID: pointerInt64(command.OperatorID), UpdaterID: pointerInt64(command.OperatorID),
		}); err != nil {
			return err
		}
		if len(roleIDs) > 0 {
			if s.roleAssignments == nil {
				return apperrors.System("用户角色策略服务未配置")
			}
			if err := s.roleAssignments.AssignCreatedUserRoles(txCtx, authorizationfacade.AssignCreatedUserRolesCommand{
				UserID: int64(userID), Username: account,
				RoleIDs: []int64(roleIDs), OperatorID: command.OperatorID, StepUpProof: command.StepUpProof,
			}); err != nil {
				return err
			}
		}
		if err := s.repo.ReplaceUserOrgs(txCtx, userID, command.OrgIDs, 0, command.OperatorID); err != nil {
			return err
		}
		if err := s.repo.ReplaceUserDepts(txCtx, userID, command.DeptIDs, 0, command.OperatorID); err != nil {
			return err
		}
		return s.repo.ReplaceUserPosts(txCtx, userID, command.PostIDs, 0, command.OperatorID)
	})
	if err != nil {
		return 0, err
	}
	_ = s.refreshUser(ctx, userID)
	return userID, nil
}

func (s *Service) UpdateAdminUser(ctx context.Context, command userfacade.AdminUserUpdateCommand) error {
	userID := command.ID
	if userID <= 0 {
		return apperrors.Params("用户ID不能为空")
	}
	existing, err := s.repo.FindAdminUserByID(ctx, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return apperrors.NotFound("用户不存在")
	}
	if strings.TrimSpace(command.Username) != "" {
		if strings.TrimSpace(command.Username) != existing.AccountName {
			return apperrors.Params("账号创建后不可修改")
		}
	}
	nickName := strings.TrimSpace(command.Nickname)
	if nickName == "" {
		nickName = existing.NickName
	}
	email := strings.TrimSpace(command.Email)
	if email == "" {
		email = existing.Email
	}
	phone := strings.TrimSpace(command.UserPhone)
	if phone == "" {
		phone = existing.Phone
	}
	if err := s.ensureUniqueUserFields(ctx, userID, "", email, phone); err != nil {
		return err
	}
	status := existing.Status
	if command.Status != nil {
		status = *command.Status
	}
	if !s.domain.ValidUserStatus(status) {
		return apperrors.Params("用户状态错误")
	}
	var statusUpdate *int
	if command.Status != nil && status != existing.Status {
		statusUpdate = &status
	}
	err = s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if statusUpdate != nil && *statusUpdate != domain.UserStatusNormal {
			if err := s.guardUserDeactivation(txCtx, userID); err != nil {
				return err
			}
			if s.sessions == nil {
				return apperrors.Operation("SSO会话回收能力未配置")
			}
		}
		if err := s.repo.UpdateAdminUser(txCtx, domain.AdminUserUpdateRecord{
			ID: userID, NickName: nickName, Email: email,
			Phone: phone, Gender: command.UserGender, Profile: command.Remark, Status: statusUpdate, OperatorID: command.OperatorID,
		}); err != nil {
			return err
		}
		if statusUpdate != nil && *statusUpdate != domain.UserStatusNormal {
			_, err := s.sessions.RevokeSessionsByUserID(txCtx, userID)
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.refreshUser(ctx, userID)
}

func (s *Service) DeleteAdminUser(ctx context.Context, command userfacade.AdminUserDeleteCommand) error {
	userID := command.UserID
	if userID <= 0 {
		return apperrors.Params("用户ID不能为空")
	}
	if userID == command.OperatorID {
		return apperrors.Operation("不能删除当前用户")
	}
	if err := stepup.Require(command.StepUpProof, stepUpActionAdminDeleteUser, adminDeleteUserBinding(userID)); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.guardUserDeactivation(txCtx, userID); err != nil {
			return err
		}
		if s.sessions == nil {
			return apperrors.Operation("SSO会话回收能力未配置")
		}
		if err := s.repo.SoftDeleteUser(txCtx, userID, command.OperatorID); err != nil {
			return err
		}
		_, err := s.sessions.RevokeSessionsByUserID(txCtx, userID)
		return err
	}); err != nil {
		return err
	}
	return s.refreshUser(ctx, userID)
}

func (s *Service) UpdateAdminUserStatus(ctx context.Context, command userfacade.AdminUserStatusCommand) error {
	userID := command.UserID
	status := command.Status
	if userID <= 0 {
		return apperrors.Params("用户ID不能为空")
	}
	if userID == command.OperatorID {
		return apperrors.Operation("不能修改当前用户状态")
	}
	if !s.domain.ValidUserStatus(status) {
		return apperrors.Params("用户状态错误")
	}
	if err := stepup.Require(command.StepUpProof, stepUpActionAdminChangeUserStatus, adminChangeUserStatusBinding(userID, status)); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if status != domain.UserStatusNormal {
			if err := s.guardUserDeactivation(txCtx, userID); err != nil {
				return err
			}
			if s.sessions == nil {
				return apperrors.Operation("SSO会话回收能力未配置")
			}
		}
		if err := s.repo.UpdateLockState(txCtx, userID, status, nil); err != nil {
			return err
		}
		if status != domain.UserStatusNormal {
			_, err := s.sessions.RevokeSessionsByUserID(txCtx, userID)
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if err := s.refreshUser(ctx, userID); err != nil {
		return err
	}
	return nil
}

// GetManagedUserStatusSnapshot returns the current durable status revision for command preparation.
func (s *Service) GetManagedUserStatusSnapshot(ctx context.Context, userID int64) (*userfacade.ManagedUserStatusSnapshot, error) {
	if userID <= 0 {
		return nil, apperrors.Params("用户ID不能为空")
	}
	existing, err := s.repo.FindAdminUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperrors.NotFound("用户不存在")
	}
	return &userfacade.ManagedUserStatusSnapshot{Status: existing.Status, Version: existing.StatusVersion}, nil
}

// SetManagedUserStatus persists an absolute status and reports whether business state changed.
func (s *Service) SetManagedUserStatus(ctx context.Context, command userfacade.SetManagedUserStatusCommand) (int64, error) {
	if command.UserID <= 0 {
		return 0, apperrors.Params("用户ID不能为空")
	}
	if !s.domain.ValidUserStatus(command.Status) {
		return 0, apperrors.Params("用户状态错误")
	}
	if !s.domain.ValidUserStatus(command.ExpectedStatus) {
		return 0, apperrors.Params("用户状态前置条件错误")
	}
	if !validStatusCommandHash(command.StatusCommandHash) {
		return 0, apperrors.Params("用户状态命令标识无效")
	}
	if command.Status != domain.UserStatusNormal && command.Cutoff.IsZero() {
		return 0, apperrors.Params("用户会话撤销截止条件无效")
	}
	intentChanged := command.ExpectedStatus != command.Status
	statusApplied := false
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if command.Status != domain.UserStatusNormal {
			if err := s.guardUserDeactivation(txCtx, command.UserID); err != nil {
				return err
			}
			if s.managedSessions == nil {
				return apperrors.ServiceUnavailable("会话撤销能力不可用")
			}
		}
		var err error
		statusApplied, err = s.repo.CompareAndSetManagedUserStatus(txCtx, command.UserID, command.ExpectedStatus, command.ExpectedVersion, command.Status, nil, command.StatusCommandHash)
		if err != nil {
			return err
		}
		if statusApplied {
			if command.Status == domain.UserStatusNormal {
				return nil
			}
			_, err := s.managedSessions.RevokeSessionsByUserIDAtOrBefore(txCtx, command.UserID, command.Cutoff.UTC())
			return err
		}
		current, err := s.repo.FindAdminUserByID(txCtx, command.UserID)
		if err != nil {
			return err
		}
		if current == nil {
			return apperrors.NotFound("用户不存在")
		}
		acceptedReplay := command.ExpectedVersion < ^uint64(0) && current.Status == command.Status && current.StatusVersion == command.ExpectedVersion+1 && current.StatusCommandHash == command.StatusCommandHash
		if !acceptedReplay {
			return apperrors.ObjectState("用户状态已被更新，请重新提交命令")
		}
		if command.Status == domain.UserStatusNormal {
			return nil
		}
		_, err = s.managedSessions.RevokeSessionsByUserIDAtOrBefore(txCtx, command.UserID, command.Cutoff.UTC())
		return err
	}); err != nil {
		return 0, err
	}
	if err := s.refreshUser(ctx, command.UserID); err != nil {
		return 0, err
	}
	if command.Status == domain.UserStatusNormal {
		if intentChanged {
			return 1, nil
		}
		return 0, nil
	}
	// The managed-command result describes whether the requested durable status
	// changed. A same-status command may still update its idempotency metadata
	// and repeat the mandatory cutoff revoke, but neither is a new status
	// transition for the Node coordinator.
	if intentChanged {
		return 1, nil
	}
	return 0, nil
}

func validStatusCommandHash(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func (s *Service) ResetAdminUserPassword(ctx context.Context, command userfacade.AdminPasswordResetCommand) error {
	if strings.TrimSpace(command.RawPassword) == "" {
		return apperrors.Params("新密码不能为空")
	}
	return s.UpdatePassword(ctx, userfacade.UpdatePasswordCommand{UserID: command.UserID, RawPassword: command.RawPassword, OperatorID: command.OperatorID})
}

func (s *Service) ListUserRoleIDs(ctx context.Context, userID int64) ([]int64, error) {
	return s.repo.ListUserRoleIDs(ctx, userID)
}
func (s *Service) ListUserOrgIDs(ctx context.Context, userID int64) ([]int64, error) {
	return s.repo.ListUserOrgIDs(ctx, userID)
}
func (s *Service) ListUserDeptIDs(ctx context.Context, userID int64) ([]int64, error) {
	return s.repo.ListUserDeptIDs(ctx, userID)
}
func (s *Service) ListUserPostIDs(ctx context.Context, userID int64) ([]int64, error) {
	return s.repo.ListUserPostIDs(ctx, userID)
}

func (s *Service) AssignUserRoles(ctx context.Context, command userfacade.RelationAssignCommand) error {
	if s.roleAssignments == nil {
		return apperrors.System("用户角色策略服务未配置")
	}
	return s.roleAssignments.AssignUserRoles(ctx, authorizationfacade.AssignUserRolesCommand{
		UserID: int64(command.UserID), RoleIDs: []int64(command.IDs),
		OperatorID: command.OperatorID, StepUpProof: command.StepUpProof,
	})
}
func (s *Service) AssignUserOrgs(ctx context.Context, command userfacade.RelationAssignCommand) error {
	if err := s.ensureRelationOperator(ctx, command.OperatorID); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.ReplaceUserOrgs(txCtx, command.UserID, command.IDs, command.PrimaryID, command.OperatorID)
	}); err != nil {
		return err
	}
	return s.refreshUser(ctx, command.UserID)
}
func (s *Service) AssignUserDepts(ctx context.Context, command userfacade.RelationAssignCommand) error {
	if err := s.ensureRelationOperator(ctx, command.OperatorID); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.ReplaceUserDepts(txCtx, command.UserID, command.IDs, command.PrimaryID, command.OperatorID)
	}); err != nil {
		return err
	}
	return s.refreshUser(ctx, command.UserID)
}
func (s *Service) AssignUserPosts(ctx context.Context, command userfacade.RelationAssignCommand) error {
	if err := s.ensureRelationOperator(ctx, command.OperatorID); err != nil {
		return err
	}
	if err := s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.ReplaceUserPosts(txCtx, command.UserID, command.IDs, command.PrimaryID, command.OperatorID)
	}); err != nil {
		return err
	}
	return s.refreshUser(ctx, command.UserID)
}

func (s *Service) ListActiveUserIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	if roleID <= 0 {
		return []int64{}, nil
	}
	return s.repo.ListActiveUserIDsByRoleID(ctx, roleID)
}

// ListActiveUserIDsByRoleIDPage returns one bounded, stable page of enabled
// role members for notification audience materialization.
func (s *Service) ListActiveUserIDsByRoleIDPage(ctx context.Context, roleID, afterUserID int64, limit int) ([]int64, error) {
	if roleID <= 0 {
		return []int64{}, nil
	}
	return s.repo.ListActiveUserIDsByRoleIDPage(ctx, roleID, afterUserID, limit)
}

func (s *Service) ensureRelationOperator(ctx context.Context, operatorID int64) error {
	if operatorID <= 0 {
		return apperrors.Unauthorized("未登录或登录信息失效")
	}
	if s.roleAssignments == nil {
		return nil
	}
	ok, err := s.roleAssignments.IsAuthorizationRootUser(ctx, operatorID)
	if err != nil {
		return err
	}
	if !ok {
		return apperrors.Forbidden("仅超级管理员可维护用户组织关系")
	}
	return nil
}

func (s *Service) GetAuthorizationUserAggregate(ctx context.Context, userID int64) (*userfacade.AuthorizationUserAggregate, error) {
	record, err := s.repo.FindSubjectByID(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	orgIDs, _ := s.ListUserOrgIDs(ctx, userID)
	deptIDs, _ := s.ListUserDeptIDs(ctx, userID)
	postIDs, _ := s.ListUserPostIDs(ctx, userID)
	return &userfacade.AuthorizationUserAggregate{
		UserID:        record.UserID,
		Username:      record.AccountName,
		Nickname:      record.NickName,
		Avatar:        record.Avatar,
		Email:         record.Email,
		Phone:         record.Phone,
		Enabled:       s.domain.Enabled(record.Status),
		Locked:        s.domain.Locked(record.Status, record.UnsealAt, time.Now().UTC()),
		PrimaryOrgID:  firstID(orgIDs),
		PrimaryDeptID: firstID(deptIDs),
		PrimaryPostID: firstID(postIDs),
	}, nil
}

func (s *Service) ListAuthorizationOrganizations(ctx context.Context, userID int64) ([]userfacade.AuthorizationOrgRecord, error) {
	ids, err := s.ListUserOrgIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindOrgsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]domain.OrgRecord, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := make([]userfacade.AuthorizationOrgRecord, 0, len(ids))
	for index, id := range ids {
		item, ok := byID[id]
		if !ok {
			continue
		}
		result = append(result, userfacade.AuthorizationOrgRecord{OrgID: item.ID, Code: item.Code, Name: item.Name, IsPrimary: index == 0})
	}
	return result, nil
}

func (s *Service) ListAuthorizationDepartments(ctx context.Context, userID int64) ([]userfacade.AuthorizationDeptRecord, error) {
	ids, err := s.ListUserDeptIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindDeptsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]domain.DeptRecord, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := make([]userfacade.AuthorizationDeptRecord, 0, len(ids))
	for index, id := range ids {
		item, ok := byID[id]
		if !ok {
			continue
		}
		result = append(result, userfacade.AuthorizationDeptRecord{DeptID: item.ID, OrgID: item.OrgID, Code: item.Code, Name: item.Name, Hierarchy: item.Hierarchy, IsPrimary: index == 0})
	}
	return result, nil
}

func (s *Service) ListAuthorizationPosts(ctx context.Context, userID int64) ([]userfacade.AuthorizationPostRecord, error) {
	ids, err := s.ListUserPostIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.FindPostsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]domain.PostRecord, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	result := make([]userfacade.AuthorizationPostRecord, 0, len(ids))
	for index, id := range ids {
		item, ok := byID[id]
		if !ok {
			continue
		}
		result = append(result, userfacade.AuthorizationPostRecord{PostID: item.ID, OrgID: item.OrgID, DeptID: item.DeptID, Code: item.Code, Name: item.Name, IsPrimary: index == 0})
	}
	return result, nil
}

func (s *Service) ListDeptHierarchyMap(ctx context.Context, deptIDs []int64) (map[int64]string, error) {
	result := make(map[int64]string, len(deptIDs))
	items, err := s.repo.FindDeptsByIDs(ctx, deptIDs)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		result[item.ID] = item.Hierarchy
	}
	return result, nil
}

func (s *Service) ListDeptIDsByHierarchies(ctx context.Context, hierarchies []string) (map[string][]int64, error) {
	result := make(map[string][]int64, len(hierarchies))
	depts, err := s.repo.ListDepts(ctx, false, "", 0, nil, 0)
	if err != nil {
		return nil, err
	}
	for _, hierarchy := range hierarchies {
		prefix := strings.TrimSpace(hierarchy)
		if prefix == "" {
			continue
		}
		for _, dept := range depts {
			if dept.Hierarchy == prefix || strings.HasPrefix(dept.Hierarchy, prefix+"/") {
				result[prefix] = append(result[prefix], dept.ID)
			}
		}
	}
	return result, nil
}

func (s *Service) CreateOrg(ctx context.Context, command userfacade.OrgCommand) error {
	record, err := s.buildOrgRecord(ctx, command, true)
	if err != nil {
		return err
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.CreateOrg(txCtx, record, command.OperatorID); err != nil {
			return err
		}
		if command.OperatorID <= 0 {
			return nil
		}
		orgIDs, err := s.repo.ListUserOrgIDs(txCtx, command.OperatorID)
		if err != nil {
			return err
		}
		if len(orgIDs) > 0 {
			return nil
		}
		return s.repo.ReplaceUserOrgs(txCtx, command.OperatorID, []int64{record.ID}, record.ID, command.OperatorID)
	})
}
func (s *Service) UpdateOrg(ctx context.Context, command userfacade.OrgCommand) error {
	record, err := s.buildOrgRecord(ctx, command, false)
	if err != nil {
		return err
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateOrg(txCtx, record, command.OperatorID)
	})
}
func (s *Service) DeleteOrg(ctx context.Context, orgID int64) error {
	if count, err := s.repo.CountOrgChildren(ctx, orgID); err != nil || count > 0 {
		if err != nil {
			return err
		}
		return apperrors.Operation("存在子组织，不能删除")
	}
	if count, err := s.repo.CountDeptByOrgID(ctx, orgID); err != nil || count > 0 {
		if err != nil {
			return err
		}
		return apperrors.Operation("组织下存在部门，不能删除")
	}
	if count, err := s.repo.CountUserOrgByOrgID(ctx, orgID); err != nil || count > 0 {
		if err != nil {
			return err
		}
		return apperrors.Operation("组织下存在用户，不能删除")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.DeleteOrg(txCtx, orgID)
	})
}
func (s *Service) GetOrgByID(ctx context.Context, orgID int64) (*userfacade.OrgVO, error) {
	record, err := s.repo.FindOrgByID(ctx, orgID)
	if err != nil || record == nil {
		return nil, err
	}
	vo := toOrgVO(*record)
	return &vo, nil
}
func (s *Service) GetOrgByCode(ctx context.Context, code string) (*userfacade.OrgVO, error) {
	record, err := s.repo.FindOrgByCode(ctx, code)
	if err != nil || record == nil {
		return nil, err
	}
	vo := toOrgVO(*record)
	return &vo, nil
}
func (s *Service) GetOrgByUserID(ctx context.Context, userID int64) (*userfacade.OrgVO, error) {
	record, err := s.repo.FindOrgByUserID(ctx, userID)
	if err != nil || record == nil {
		return nil, err
	}
	vo := toOrgVO(*record)
	return &vo, nil
}
func (s *Service) GetOrgTree(ctx context.Context) ([]userfacade.OrgVO, error) {
	records, err := s.repo.ListOrgs(ctx, false)
	if err != nil {
		return nil, err
	}
	return buildOrgTree(records), nil
}
func (s *Service) ListActiveOrgs(ctx context.Context) ([]userfacade.OrgVO, error) {
	records, err := s.repo.ListOrgs(ctx, true)
	if err != nil {
		return nil, err
	}
	return toOrgVOs(records), nil
}
func (s *Service) ListOrgChildren(ctx context.Context, parentID int64) ([]userfacade.OrgVO, error) {
	records, err := s.repo.ListOrgChildren(ctx, parentID)
	if err != nil {
		return nil, err
	}
	return toOrgVOs(records), nil
}
func (s *Service) CheckOrgCode(ctx context.Context, code string, excludeID int64) (bool, error) {
	count, err := s.repo.CountOrgCodeExcludingID(ctx, excludeID, code)
	return count == 0, err
}
func (s *Service) ChangeOrgStatus(ctx context.Context, orgID int64, status int, operatorID int64) error {
	if status != 0 && status != 1 {
		return apperrors.Params("组织状态错误")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateOrgStatus(txCtx, orgID, status, operatorID)
	})
}
func (s *Service) MoveOrg(ctx context.Context, orgID, newParentID int64, operatorID int64) error {
	if orgID == newParentID {
		return apperrors.Params("父组织不能是自身")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateOrgParent(txCtx, orgID, newParentID, operatorID)
	})
}

func (s *Service) GetDeptTree(ctx context.Context, enabledOnly bool) ([]userfacade.DeptVO, error) {
	records, err := s.repo.ListDepts(ctx, enabledOnly, "", 0, nil, 0)
	if err != nil {
		return nil, err
	}
	return buildDeptTree(records), nil
}
func (s *Service) SearchDepts(ctx context.Context, keyword string, orgID int64, status *int, limit int) ([]userfacade.DeptVO, error) {
	records, err := s.repo.ListDepts(ctx, false, keyword, orgID, status, limit)
	if err != nil {
		return nil, err
	}
	return toDeptVOs(records), nil
}
func (s *Service) GetDeptByID(ctx context.Context, deptID int64) (*userfacade.DeptVO, error) {
	record, err := s.repo.FindDeptByID(ctx, deptID)
	if err != nil || record == nil {
		return nil, err
	}
	vo := toDeptVO(*record)
	return &vo, nil
}
func (s *Service) CreateDept(ctx context.Context, command userfacade.DeptCommand) error {
	record, err := s.buildDeptRecord(ctx, command)
	if err != nil {
		return err
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.CreateDept(txCtx, record, command.OperatorID)
	})
}
func (s *Service) UpdateDept(ctx context.Context, command userfacade.DeptCommand) error {
	record, err := s.buildDeptRecord(ctx, command)
	if err != nil {
		return err
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdateDept(txCtx, record, command.OperatorID)
	})
}
func (s *Service) DeleteDept(ctx context.Context, deptID int64) error {
	if count, err := s.repo.CountDeptChildren(ctx, deptID); err != nil || count > 0 {
		if err != nil {
			return err
		}
		return apperrors.Operation("存在子部门，不能删除")
	}
	if count, err := s.repo.CountUserDeptByDeptID(ctx, deptID); err != nil || count > 0 {
		if err != nil {
			return err
		}
		return apperrors.Operation("部门下存在用户，不能删除")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.DeleteDept(txCtx, deptID)
	})
}
func (s *Service) GetChildDeptIDs(ctx context.Context, deptID int64) ([]int64, error) {
	return s.repo.ListChildDeptIDs(ctx, deptID)
}

func (s *Service) QueryPosts(ctx context.Context, query userfacade.PostQuery) (*userfacade.PageResult[userfacade.PostVO], error) {
	records, total, err := s.repo.QueryPosts(ctx, domain.PostQuery{Current: query.Current, Size: query.Size, Name: query.Name, Code: query.Code, Status: query.Status, Scope: toDomainScope(query.Scope)})
	if err != nil {
		return nil, err
	}
	return &userfacade.PageResult[userfacade.PostVO]{Current: defaultPage(query.Current), Size: defaultSize(query.Size), Total: total, Records: toPostVOs(records)}, nil
}
func (s *Service) ListEnabledPosts(ctx context.Context) ([]userfacade.PostVO, error) {
	records, err := s.repo.ListEnabledPosts(ctx)
	if err != nil {
		return nil, err
	}
	return toPostVOs(records), nil
}
func (s *Service) GetPostByID(ctx context.Context, postID int64) (*userfacade.PostVO, error) {
	record, err := s.repo.FindPostByID(ctx, postID)
	if err != nil || record == nil {
		return nil, err
	}
	vo := toPostVO(*record)
	return &vo, nil
}
func (s *Service) ListPostsByIDs(ctx context.Context, postIDs []int64) ([]userfacade.PostVO, error) {
	ids := normalizeIDs(postIDs)
	if len(ids) == 0 {
		return []userfacade.PostVO{}, nil
	}
	if len(ids) > adminMutationBatchMax {
		return nil, apperrors.Params("岗位数量超过单次批量上限")
	}
	records, err := s.repo.FindPostsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return toPostVOs(records), nil
}
func (s *Service) CreatePost(ctx context.Context, command userfacade.PostCommand) error {
	record, err := s.buildPostRecord(ctx, command)
	if err != nil {
		return err
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.CreatePost(txCtx, record, command.OperatorID)
	})
}
func (s *Service) UpdatePost(ctx context.Context, command userfacade.PostCommand) error {
	record, err := s.buildPostRecord(ctx, command)
	if err != nil {
		return err
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdatePost(txCtx, record, command.OperatorID)
	})
}
func (s *Service) DeletePost(ctx context.Context, postID int64) error {
	if s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("岗位删除事务能力未配置")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		if count, err := s.repo.CountUserPostByPostID(txCtx, postID); err != nil || count > 0 {
			if err != nil {
				return err
			}
			return apperrors.Operation("岗位下存在用户，不能删除")
		}
		return s.repo.DeletePost(txCtx, postID)
	})
}
func (s *Service) BatchDeletePosts(ctx context.Context, postIDs []int64) error {
	ids := normalizeIDs(postIDs)
	if len(ids) == 0 {
		return apperrors.Params("岗位ID不能为空")
	}
	if len(ids) > adminMutationBatchMax {
		return apperrors.Params("岗位数量超过单次批量上限")
	}
	batchRepo, ok := s.repo.(postBatchRepository)
	if !ok {
		return apperrors.System("岗位批量仓储能力未配置")
	}
	if s.transactor == nil || !s.transactor.Enabled() {
		return apperrors.System("岗位批量删除事务能力未配置")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		referenced, err := batchRepo.ListReferencedPostIDs(txCtx, ids)
		if err != nil {
			return err
		}
		if len(referenced) > 0 {
			return apperrors.Operation("岗位下存在用户，不能删除")
		}
		return batchRepo.DeletePosts(txCtx, ids)
	})
}
func (s *Service) ChangePostStatus(ctx context.Context, postID int64, status int, operatorID int64) error {
	if status != 0 && status != 1 {
		return apperrors.Params("岗位状态错误")
	}
	return s.withAuthorizationInvalidationTx(ctx, func(txCtx context.Context) error {
		return s.repo.UpdatePostStatus(txCtx, postID, status, operatorID)
	})
}
func (s *Service) ListPostRoleIDs(ctx context.Context, postID int64) ([]int64, error) {
	return s.repo.ListPostRoleIDs(ctx, postID)
}
func (s *Service) AssignPostRoles(ctx context.Context, command userfacade.PostRoleAssignCommand) error {
	roleIDs := normalizeIDs(command.RoleIDs)
	if len(roleIDs) > adminMutationBatchMax {
		return apperrors.Params("角色数量超过单次批量上限")
	}
	if err := stepup.Require(command.StepUpProof, stepUpActionRBACAssignPostRoles, postRoleAssignmentBinding(command.PostID, roleIDs)); err != nil {
		return err
	}
	consistent, ok := s.transactor.(store.ConsistentTransactor)
	if !ok || consistent == nil || !consistent.Enabled() {
		return apperrors.System("岗位角色分配一致性事务能力未配置")
	}
	if err := s.withAuthorizationInvalidationBoundary(ctx, consistent.WithinConsistentTransaction, func(txCtx context.Context) error {
		if len(roleIDs) > 0 {
			guard, ok := s.permissions.(authorizationfacade.PostRoleAssignmentGuardFacade)
			if !ok {
				return apperrors.System("岗位角色父记录锁定校验能力未配置")
			}
			allowed, err := guard.LockAndValidatePostRoleAssignments(txCtx, command.OperatorID, command.PostID, roleIDs)
			if err != nil {
				return err
			}
			if !allowed {
				return apperrors.Forbidden("无权分配该岗位角色")
			}
		}
		if err := s.repo.ReplacePostRoles(txCtx, command.PostID, roleIDs, command.OperatorID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}
func (s *Service) ListPostIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	return s.repo.ListPostIDsByRoleID(ctx, roleID)
}

func (s *Service) withTx(ctx context.Context, fn func(context.Context) error) error {
	if s.transactor != nil && s.transactor.Enabled() {
		return s.transactor.WithinTransaction(ctx, fn)
	}
	return fn(ctx)
}

// withAuthorizationInvalidationTx registers both private authorization
// projections with the same user mutation. Generation invalidation is
// class-wide and bounded: it never enumerates every user affected by an
// organization, department, post, or status change to be correct.
func (s *Service) withAuthorizationInvalidationTx(ctx context.Context, fn func(context.Context) error) error {
	return s.withAuthorizationInvalidationBoundary(ctx, s.withTx, fn)
}

func (s *Service) withAuthorizationInvalidationBoundary(ctx context.Context, boundary cachegovernancefacade.TransactionBoundary, fn func(context.Context) error) error {
	return cachegovernancefacade.RunInvalidatedMutationClasses(ctx, boundary, s.transactor, s.invalidations, []cachepolicy.DataClass{
		cachepolicy.DataClassAuthorizationContext,
		cachepolicy.DataClassAuthorizationMenus,
	}, func(txCtx context.Context) (bool, error) {
		if err := fn(txCtx); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (s *Service) guardUserDeactivation(ctx context.Context, userID int64) error {
	if s.roleAssignments == nil {
		return apperrors.System("用户角色策略服务未配置")
	}
	return s.roleAssignments.GuardUserDeactivation(ctx, userID)
}

func (s *Service) refreshUser(ctx context.Context, userID int64) error {
	if s.permissions == nil || userID <= 0 {
		return nil
	}
	return s.permissions.RefreshUserPermissionCache(ctx, userID)
}

func (s *Service) revokeUserSessions(ctx context.Context, userID int64) error {
	if s.sessions == nil || userID <= 0 {
		return nil
	}
	_, err := s.sessions.RevokeSessionsByUserID(ctx, userID)
	return err
}

func (s *Service) ensureUniqueUserFields(ctx context.Context, userID int64, account, email, phone string) error {
	account = strings.TrimSpace(account)
	if account != "" {
		count, err := s.repo.CountByAccountExcludingUserID(ctx, userID, account)
		if err != nil {
			return err
		}
		if count > 0 {
			return apperrors.Params("用户名已存在")
		}
	}
	email = strings.TrimSpace(email)
	if email != "" {
		if !s.domain.ValidateEmail(email) {
			return apperrors.Params("邮箱格式错误")
		}
		count, err := s.repo.CountByEmailExcludingUserID(ctx, userID, email)
		if err != nil {
			return err
		}
		if count > 0 {
			return apperrors.Params("邮箱已存在")
		}
	}
	phone = strings.TrimSpace(phone)
	if phone != "" {
		if !s.domain.ValidatePhone(phone) {
			return apperrors.Params("手机号格式错误")
		}
		count, err := s.repo.CountByPhoneExcludingUserID(ctx, userID, phone)
		if err != nil {
			return err
		}
		if count > 0 {
			return apperrors.Params("手机号已存在")
		}
	}
	return nil
}

func (s *Service) buildOrgRecord(ctx context.Context, command userfacade.OrgCommand, creating bool) (domain.OrgRecord, error) {
	if !creating && command.ID <= 0 {
		return domain.OrgRecord{}, apperrors.Params("组织ID不能为空")
	}
	code := strings.TrimSpace(command.Code)
	if code == "" {
		return domain.OrgRecord{}, apperrors.Params("组织编码不能为空")
	}
	if strings.TrimSpace(command.Name) == "" {
		return domain.OrgRecord{}, apperrors.Params("组织名称不能为空")
	}
	count, err := s.repo.CountOrgCodeExcludingID(ctx, command.ID, code)
	if err != nil {
		return domain.OrgRecord{}, err
	}
	if count > 0 {
		return domain.OrgRecord{}, apperrors.Params("组织编码已存在")
	}
	if command.ParentID > 0 {
		if command.ParentID == command.ID {
			return domain.OrgRecord{}, apperrors.Params("父组织不能是自身")
		}
		parent, err := s.repo.FindOrgByID(ctx, command.ParentID)
		if err != nil {
			return domain.OrgRecord{}, err
		}
		if parent == nil || parent.Status != 0 {
			return domain.OrgRecord{}, apperrors.Params("父组织不存在或已禁用")
		}
	}
	id := command.ID
	if creating {
		id = s.nextID()
	}
	return domain.OrgRecord{ID: id, Code: code, Name: strings.TrimSpace(command.Name), ParentID: command.ParentID, Status: intValue(command.Status, 0), SortOrder: intValue(command.SortOrder, 0), LeaderUserID: command.LeaderUserID}, nil
}

func (s *Service) buildDeptRecord(ctx context.Context, command userfacade.DeptCommand) (domain.DeptRecord, error) {
	if strings.TrimSpace(command.Name) == "" {
		return domain.DeptRecord{}, apperrors.Params("部门名称不能为空")
	}
	if command.OrgID <= 0 {
		return domain.DeptRecord{}, apperrors.Params("组织不能为空")
	}
	id := command.ID
	if id <= 0 {
		id = s.nextID()
	}
	org, err := s.repo.FindOrgByID(ctx, command.OrgID)
	if err != nil {
		return domain.DeptRecord{}, err
	}
	if org == nil {
		return domain.DeptRecord{}, apperrors.Params("组织不存在")
	}
	if command.ParentID == id && id > 0 {
		return domain.DeptRecord{}, apperrors.Params("父部门不能是自身")
	}
	level := 1
	hierarchy := "/" + int64String(id)
	if command.ParentID > 0 {
		parent, err := s.repo.FindDeptByID(ctx, command.ParentID)
		if err != nil {
			return domain.DeptRecord{}, err
		}
		if parent == nil {
			return domain.DeptRecord{}, apperrors.Params("父部门不存在")
		}
		if parent.OrgID != command.OrgID {
			return domain.DeptRecord{}, apperrors.Params("父部门不属于指定组织")
		}
		level = parent.Level + 1
		hierarchy = strings.TrimRight(parent.Hierarchy, "/") + "/" + int64String(id)
	}
	count, err := s.repo.CountDeptNameUnderParent(ctx, command.ID, command.ParentID, command.Name)
	if err != nil {
		return domain.DeptRecord{}, err
	}
	if count > 0 {
		return domain.DeptRecord{}, apperrors.Params("同级部门名称已存在")
	}
	code := strings.TrimSpace(command.Code)
	if code == "" {
		code = int64String(id)
	}
	return domain.DeptRecord{ID: id, Code: code, Name: strings.TrimSpace(command.Name), OrgID: command.OrgID, ParentID: command.ParentID, LeaderUserID: command.LeaderUserID, Status: intValue(command.Status, 0), SortOrder: intValue(command.SortOrder, 0), Hierarchy: hierarchy, Level: level}, nil
}

func (s *Service) buildPostRecord(ctx context.Context, command userfacade.PostCommand) (domain.PostRecord, error) {
	if strings.TrimSpace(command.Code) == "" || strings.TrimSpace(command.Name) == "" {
		return domain.PostRecord{}, apperrors.Params("岗位编码和名称不能为空")
	}
	postID := command.ID
	deptID := command.DeptID
	orgID := command.OrgID
	if deptID <= 0 {
		return domain.PostRecord{}, apperrors.Params("请选择所属部门")
	}
	if count, err := s.repo.CountPostCodeExcludingID(ctx, postID, command.Code); err != nil || count > 0 {
		if err != nil {
			return domain.PostRecord{}, err
		}
		return domain.PostRecord{}, apperrors.Params("岗位编码已存在")
	}
	if count, err := s.repo.CountPostNameExcludingID(ctx, postID, command.Name); err != nil || count > 0 {
		if err != nil {
			return domain.PostRecord{}, err
		}
		return domain.PostRecord{}, apperrors.Params("岗位名称已存在")
	}
	if deptID > 0 {
		dept, err := s.repo.FindDeptByID(ctx, deptID)
		if err != nil {
			return domain.PostRecord{}, err
		}
		if dept == nil {
			return domain.PostRecord{}, apperrors.Params("部门不存在")
		}
		if orgID > 0 && dept.OrgID != orgID {
			return domain.PostRecord{}, apperrors.Params("岗位部门不属于指定组织")
		}
		if orgID <= 0 {
			orgID = dept.OrgID
		}
	}
	id := postID
	if id <= 0 {
		id = s.nextID()
	}
	return domain.PostRecord{ID: id, Code: strings.TrimSpace(command.Code), Name: strings.TrimSpace(command.Name), DeptID: deptID, OrgID: orgID, SortOrder: intValue(command.SortOrder, 0), Status: intValue(command.Status, 0), Remark: strings.TrimSpace(command.Remark)}, nil
}

func (s *Service) toSubjectRecord(record *domain.SubjectRecord) *userfacade.SubjectRecord {
	if record == nil {
		return nil
	}
	now := time.Now().UTC()
	return &userfacade.SubjectRecord{
		UserID:      record.UserID,
		AccountName: record.AccountName,
		Email:       record.Email,
		Phone:       record.Phone,
		Status:      record.Status,
		Enabled:     s.domain.Enabled(record.Status),
		LockStatus:  s.domain.Locked(record.Status, record.UnsealAt, now),
		UnsealAt:    record.UnsealAt,
	}
}

func (s *Service) toAdminUserVO(record domain.AdminUserRecord) userfacade.AdminUserVO {
	return userfacade.AdminUserVO{
		ID: record.ID, Username: record.AccountName, Nickname: record.NickName, Avatar: record.Avatar,
		Email: record.Email, UserPhone: record.Phone, UserGender: record.Gender, Status: record.Status,
		UserProfile: record.Profile, CreateTime: record.CreateTime, UpdateTime: record.UpdateTime,
	}
}

func toOrgVO(record domain.OrgRecord) userfacade.OrgVO {
	return userfacade.OrgVO{ID: record.ID, Code: record.Code, Name: record.Name, ParentID: record.ParentID, Status: record.Status, SortOrder: record.SortOrder, LeaderUserID: record.LeaderUserID}
}

func toOrgVOs(records []domain.OrgRecord) []userfacade.OrgVO {
	result := make([]userfacade.OrgVO, 0, len(records))
	for _, record := range records {
		result = append(result, toOrgVO(record))
	}
	return result
}

func buildOrgTree(records []domain.OrgRecord) []userfacade.OrgVO {
	nodes := make(map[int64]*userfacade.OrgVO, len(records))
	order := make([]int64, 0, len(records))
	for _, record := range records {
		vo := toOrgVO(record)
		nodes[vo.ID] = &vo
		order = append(order, vo.ID)
	}
	result := make([]userfacade.OrgVO, 0)
	for _, id := range order {
		node := nodes[id]
		if parent := nodes[node.ParentID]; parent != nil && node.ID != node.ParentID {
			parent.Children = append(parent.Children, *node)
		} else {
			result = append(result, *node)
		}
	}
	return result
}

func toDeptVO(record domain.DeptRecord) userfacade.DeptVO {
	return userfacade.DeptVO{ID: record.ID, Code: record.Code, Name: record.Name, OrgID: record.OrgID, ParentID: record.ParentID, LeaderUserID: record.LeaderUserID, Status: record.Status, SortOrder: record.SortOrder, Hierarchy: record.Hierarchy, Level: record.Level}
}

func toDeptVOs(records []domain.DeptRecord) []userfacade.DeptVO {
	result := make([]userfacade.DeptVO, 0, len(records))
	for _, record := range records {
		result = append(result, toDeptVO(record))
	}
	return result
}

func buildDeptTree(records []domain.DeptRecord) []userfacade.DeptVO {
	nodes := make(map[int64]*userfacade.DeptVO, len(records))
	order := make([]int64, 0, len(records))
	for _, record := range records {
		vo := toDeptVO(record)
		nodes[vo.ID] = &vo
		order = append(order, vo.ID)
	}
	result := make([]userfacade.DeptVO, 0)
	for _, id := range order {
		node := nodes[id]
		if parent := nodes[node.ParentID]; parent != nil && node.ID != node.ParentID {
			parent.Children = append(parent.Children, *node)
		} else {
			result = append(result, *node)
		}
	}
	return result
}

func toPostVO(record domain.PostRecord) userfacade.PostVO {
	return userfacade.PostVO{ID: record.ID, Code: record.Code, Name: record.Name, DeptID: record.DeptID, OrgID: record.OrgID, SortOrder: record.SortOrder, Status: record.Status, Remark: record.Remark}
}

func toDomainScope(scope userfacade.DataScopeFilter) domain.DataScopeFilter {
	return domain.DataScopeFilter{
		Enabled:    scope.Enabled,
		None:       scope.None,
		ScopeType:  scope.ScopeType,
		SelfUserID: scope.SelfUserID,
		DeptIDs:    append([]int64{}, scope.DeptIDs...),
		OrgIDs:     append([]int64{}, scope.OrgIDs...),
	}
}

func toPostVOs(records []domain.PostRecord) []userfacade.PostVO {
	result := make([]userfacade.PostVO, 0, len(records))
	for _, record := range records {
		result = append(result, toPostVO(record))
	}
	return result
}

func defaultPage(value int64) int64 {
	if value <= 0 {
		return 1
	}
	return value
}

func defaultSize(value int64) int64 {
	if value <= 0 {
		return 10
	}
	if value > 200 {
		return 200
	}
	return value
}

func intValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func postRoleAssignmentBinding(postID int64, roleIDs []int64) string {
	return "post:" + strconv.FormatInt(postID, 10) + "|roles:" + joinSortedIDs(roleIDs)
}

func adminDeleteUserBinding(userID int64) string {
	return "user:" + strconv.FormatInt(userID, 10) + "|delete"
}

func adminChangeUserStatusBinding(userID int64, status int) string {
	return "user:" + strconv.FormatInt(userID, 10) + "|status:" + strconv.Itoa(status)
}

func joinSortedIDs(ids []int64) string {
	normalized := normalizeIDs(ids)
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	parts := make([]string, 0, len(normalized))
	for _, id := range normalized {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

func firstID(ids []int64) int64 {
	for _, id := range ids {
		if id > 0 {
			return id
		}
	}
	return 0
}

func (s *Service) nextID() int64 {
	if s.idGen != nil {
		return s.idGen.NextID()
	}
	return time.Now().UTC().UnixNano()
}

func int64String(value int64) string {
	return strconv.FormatInt(value, 10)
}

func pointerInt64(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	copy := value
	return &copy
}

func sanitizeOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}
