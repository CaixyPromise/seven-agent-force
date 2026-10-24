package application

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	nodedomain "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/domain"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
)

const maxReasonLength = 512

// UserQueryFacade is the narrow safe subset consumed from AdminUserFacade.
type UserQueryFacade interface {
	QueryUsers(ctx context.Context, query userfacade.AdminUserQuery) (*userfacade.PageResult[userfacade.AdminUserVO], error)
	GetAdminUser(ctx context.Context, userID int64) (*userfacade.AdminUserVO, error)
}

// SessionFacade is the narrow session read/revoke subset used by this module.
type SessionFacade interface {
	ListSessionsByUserIDPage(ctx context.Context, userID int64, offset, limit int) ([]ssofacade.SessionRecord, error)
	CountSessionsByUserID(ctx context.Context, userID int64) (int64, error)
	CaptureManagedSessionCutoff(ctx context.Context) (time.Time, error)
	RevokeManagedSession(ctx context.Context, sessionID string) (bool, error)
	RevokeSessionsByUserIDAtOrBefore(ctx context.Context, userID int64, cutoff time.Time) (int64, error)
}

// AuditWriter persists command audits synchronously.
type AuditWriter interface {
	SaveLog(ctx context.Context, entry adminfacade.OperationLogEntry) error
}

// Config contains non-secret runtime identity and the pre-hashed caller ID.
type Config struct {
	NodeCode     string
	Version      string
	CallerIDHash string
}

// Dependencies contains facade ports used by Node management use cases.
type Dependencies struct {
	Users         UserQueryFacade
	ManagedUsers  userfacade.ManagedUserStatusFacade
	Sessions      SessionFacade
	Policies      platformfacade.ManagedLoginPolicyFacade
	HubConnection nodefacade.HubConnectionPort
	Replay        nodedomain.CommandCoordinator
	Audit         AuditWriter
	SessionRefs   nodefacade.SessionReferenceCodec
}

// Service composes safe queries and value-idempotent commands.
type Service struct {
	config        Config
	users         UserQueryFacade
	managedUsers  userfacade.ManagedUserStatusFacade
	sessions      SessionFacade
	policies      platformfacade.ManagedLoginPolicyFacade
	hubConnection nodefacade.HubConnectionPort
	replay        nodedomain.CommandCoordinator
	audit         AuditWriter
	sessionRefs   nodefacade.SessionReferenceCodec
	now           func() time.Time
}

// NewService creates the Node management application service.
func NewService(config Config, dependencies Dependencies) *Service {
	return &Service{
		config: config, users: dependencies.Users, managedUsers: dependencies.ManagedUsers,
		sessions: dependencies.Sessions, policies: dependencies.Policies, hubConnection: dependencies.HubConnection,
		replay: dependencies.Replay, audit: dependencies.Audit, sessionRefs: dependencies.SessionRefs,
		now: time.Now,
	}
}

// BindHubConnection binds the Task 7 managed OIDC persistence port.
func (s *Service) BindHubConnection(port nodefacade.HubConnectionPort) {
	if s != nil {
		s.hubConnection = port
	}
}

// Describe returns the safe Node identity and capability descriptor.
func (s *Service) Describe(context.Context) (*nodefacade.NodeDescriptor, error) {
	return &nodefacade.NodeDescriptor{NodeCode: s.config.NodeCode, Version: s.config.Version, Capabilities: []string{"users", "sessions", "login-policy", "hub-connection"}, Health: "UP"}, nil
}

// ListUsers returns a contact-masked page of local users.
func (s *Service) ListUsers(ctx context.Context, query nodefacade.UserPageQuery) (*nodefacade.UserPage, error) {
	if s == nil || s.users == nil {
		return nil, apperrors.ServiceUnavailable("用户查询能力不可用")
	}
	current, size := normalizePage(query.Current, query.Size)
	if _, err := pageOffset(current, size, int64(^uint64(0)>>1)); err != nil {
		return nil, err
	}
	if len([]rune(strings.TrimSpace(query.Keyword))) > 128 {
		return nil, apperrors.Params("keyword不能超过128字符")
	}
	page, err := s.users.QueryUsers(ctx, userfacade.AdminUserQuery{Current: current, Size: size, Username: strings.TrimSpace(query.Keyword), Status: query.Status})
	if err != nil {
		return nil, err
	}
	result := &nodefacade.UserPage{Current: page.Current, Size: page.Size, Total: page.Total, Records: make([]nodefacade.UserSummary, 0, len(page.Records))}
	for _, record := range page.Records {
		result.Records = append(result.Records, mapUser(record))
	}
	return result, nil
}

// GetUser returns one contact-masked local user.
func (s *Service) GetUser(ctx context.Context, userID int64) (*nodefacade.UserDetail, error) {
	if userID <= 0 {
		return nil, apperrors.Params("userId格式错误")
	}
	if s == nil || s.users == nil {
		return nil, apperrors.ServiceUnavailable("用户查询能力不可用")
	}
	record, err := s.users.GetAdminUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, apperrors.NotFound("用户不存在")
	}
	result := nodefacade.UserDetail(mapUser(*record))
	return &result, nil
}

// SetUserStatus sets an absolute status through the trusted managed facade.
func (s *Service) SetUserStatus(ctx context.Context, command nodefacade.SetUserStatusCommand) (*nodefacade.CommandResult, error) {
	userID, err := parseID(command.UserID)
	if err != nil {
		return nil, err
	}
	if command.Status < nodefacade.UserStatusNormal || command.Status > nodefacade.UserStatusPendingReview {
		return nil, apperrors.Params("status格式错误")
	}
	if err := validateCommand(command.IdempotencyKey, command.Reason); err != nil {
		return nil, err
	}
	if s.managedUsers == nil {
		return nil, apperrors.ServiceUnavailable("用户状态管理能力不可用")
	}
	path := "/internal/node/v1/users/" + command.UserID + "/status"
	metadata, err := commandMetadata(s, httpMethodPut, path, command.IdempotencyKey, command)
	if err != nil {
		return nil, err
	}
	type acceptedStatusCommand struct {
		Status        int       `json:"status"`
		StatusVersion uint64    `json:"statusVersion"`
		Cutoff        time.Time `json:"cutoff,omitempty"`
	}
	prepared, err := s.replay.Prepare(ctx, metadata, func(ctx context.Context) ([]byte, error) {
		snapshot, err := s.managedUsers.GetManagedUserStatusSnapshot(ctx, userID)
		if err != nil {
			return nil, err
		}
		accepted := acceptedStatusCommand{Status: snapshot.Status, StatusVersion: snapshot.Version}
		if command.Status != nodefacade.UserStatusNormal {
			if s.sessions == nil {
				return nil, apperrors.ServiceUnavailable("会话撤销能力不可用")
			}
			cutoff, err := s.sessions.CaptureManagedSessionCutoff(ctx)
			if err != nil {
				return nil, err
			}
			accepted.Cutoff = cutoff.UTC()
		}
		return json.Marshal(accepted)
	})
	if err != nil {
		return nil, err
	}
	var accepted acceptedStatusCommand
	if len(prepared) > 0 {
		if err := json.Unmarshal(prepared, &accepted); err != nil || accepted.Status < nodefacade.UserStatusNormal || accepted.Status > nodefacade.UserStatusPendingReview || (command.Status != nodefacade.UserStatusNormal && accepted.Cutoff.IsZero()) {
			return nil, apperrors.ServiceUnavailable("命令准备状态无法读取")
		}
		accepted.Cutoff = accepted.Cutoff.UTC()
	}
	return executeCommand(s, ctx, metadata, "NODE_USER_STATUS", command.UserID, func(ctx context.Context) (int64, error) {
		if len(prepared) == 0 {
			return 0, apperrors.ServiceUnavailable("命令准备状态无法读取")
		}
		if command.Status != nodefacade.UserStatusNormal && accepted.Cutoff.IsZero() {
			return 0, apperrors.ServiceUnavailable("命令准备状态无法读取")
		}
		return s.managedUsers.SetManagedUserStatus(ctx, userfacade.SetManagedUserStatusCommand{UserID: userID, Status: command.Status, ExpectedStatus: accepted.Status, ExpectedVersion: accepted.StatusVersion, Cutoff: accepted.Cutoff, StatusCommandHash: nodedomain.CommandScopeHash(metadata)})
	})
}

// ListUserSessions returns safe session projections with opaque references.
func (s *Service) ListUserSessions(ctx context.Context, userID int64, query nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error) {
	if userID <= 0 {
		return nil, apperrors.Params("userId格式错误")
	}
	if s == nil || s.sessions == nil || s.sessionRefs == nil {
		return nil, apperrors.ServiceUnavailable("会话查询能力不可用")
	}
	current, size := normalizePage(query.Current, query.Size)
	offset, err := pageOffset(current, size, int64(^uint(0)>>1))
	if err != nil {
		return nil, err
	}
	total, err := s.sessions.CountSessionsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	records, err := s.sessions.ListSessionsByUserIDPage(ctx, userID, int(offset), int(size))
	if err != nil {
		return nil, err
	}
	result := &nodefacade.SessionPage{Current: current, Size: size, Total: total, Records: make([]nodefacade.SessionSummary, 0, len(records))}
	for _, record := range records {
		if record.UserID != userID {
			return nil, apperrors.Forbidden("会话归属校验失败")
		}
		reference, err := s.sessionRefs.Encode(ctx, record)
		if err != nil {
			return nil, err
		}
		result.Records = append(result.Records, nodefacade.SessionSummary{SessionRef: reference, ClientID: record.ClientID, LoginMethod: record.LoginMethod, LoginAt: record.LoginAt, LastAccess: record.LastAccessAt, ExpiresAt: record.ExpiresAt, Status: effectiveSessionStatus(record, s.now())})
	}
	return result, nil
}

// RevokeUserSessions revokes all or an ownership-checked session set.
func (s *Service) RevokeUserSessions(ctx context.Context, command nodefacade.RevokeUserSessionsCommand) (*nodefacade.RevokeResult, error) {
	userID, err := parseID(command.UserID)
	if err != nil {
		return nil, err
	}
	if err := validateCommand(command.IdempotencyKey, command.Reason); err != nil {
		return nil, err
	}
	if command.All == (len(command.SessionRefs) > 0) {
		return nil, apperrors.Params("all与sessionRefs必须且只能指定一个")
	}
	if len(command.SessionRefs) > nodefacade.MaxSessionReferencesPerCommand {
		return nil, apperrors.Params("sessionRefs不能超过100个")
	}
	if s.sessions == nil || s.sessionRefs == nil {
		return nil, apperrors.ServiceUnavailable("会话撤销能力不可用")
	}
	path := "/internal/node/v1/users/" + command.UserID + "/sessions/revoke"
	metadata, err := commandMetadata(s, httpMethodPost, path, command.IdempotencyKey, command)
	if err != nil {
		return nil, err
	}
	var cutoff *time.Time
	if command.All {
		prepared, err := s.replay.Prepare(ctx, metadata, func(ctx context.Context) ([]byte, error) {
			cutoff, err := s.sessions.CaptureManagedSessionCutoff(ctx)
			if err != nil {
				return nil, err
			}
			accepted := struct {
				Cutoff time.Time `json:"cutoff"`
			}{Cutoff: cutoff.UTC()}
			return json.Marshal(accepted)
		})
		if err != nil {
			return nil, err
		}
		if len(prepared) > 0 {
			var accepted struct {
				Cutoff time.Time `json:"cutoff"`
			}
			if err := json.Unmarshal(prepared, &accepted); err != nil || accepted.Cutoff.IsZero() {
				return nil, apperrors.ServiceUnavailable("命令准备状态无法读取")
			}
			cutoff = &accepted.Cutoff
		}
	}
	var sessionIDs []string
	if !command.All {
		sessionIDs = make([]string, 0, len(command.SessionRefs))
		for _, reference := range command.SessionRefs {
			decoded, err := s.sessionRefs.Decode(ctx, reference)
			if err != nil {
				return nil, err
			}
			if decoded.UserID != userID {
				return nil, apperrors.Forbidden("会话不属于目标用户")
			}
			sessionIDs = append(sessionIDs, decoded.SessionID)
		}
	}
	return executeCommand(s, ctx, metadata, "NODE_SESSION_REVOKE", command.UserID, func(ctx context.Context) (int64, error) {
		if command.All {
			if cutoff == nil {
				return 0, apperrors.ServiceUnavailable("命令准备状态无法读取")
			}
			return s.sessions.RevokeSessionsByUserIDAtOrBefore(ctx, userID, cutoff.UTC())
		}
		var changed int64
		for _, sessionID := range sessionIDs {
			revoked, err := s.sessions.RevokeManagedSession(ctx, sessionID)
			if err != nil {
				return changed, err
			}
			if revoked {
				changed++
			}
		}
		return changed, nil
	})
}

// GetLoginPolicy returns only the remotely manageable policy subset.
func (s *Service) GetLoginPolicy(ctx context.Context) (*nodefacade.ManagedLoginPolicy, error) {
	if s == nil || s.policies == nil {
		return nil, apperrors.ServiceUnavailable("登录策略管理能力不可用")
	}
	policy, err := s.policies.GetManagedLoginPolicy(ctx)
	if err != nil {
		return nil, err
	}
	return mapPolicy(policy), nil
}

// ApplyLoginPolicy applies a complete safe policy snapshot.
func (s *Service) ApplyLoginPolicy(ctx context.Context, command nodefacade.ApplyLoginPolicyCommand) (*nodefacade.CommandResult, error) {
	if err := validateCommand(command.IdempotencyKey, command.Reason); err != nil {
		return nil, err
	}
	if s == nil || s.policies == nil {
		return nil, apperrors.ServiceUnavailable("登录策略管理能力不可用")
	}
	if len(command.LoginMethods) > 32 || len(command.SourceRules) > 256 {
		return nil, apperrors.Params("登录策略条目数量超过限制")
	}
	managed := mapManagedPolicy(command.ManagedLoginPolicy)
	return execute(s, ctx, httpMethodPost, "/internal/node/v1/login-policy/apply", command.IdempotencyKey, command, "NODE_LOGIN_POLICY_APPLY", command.PlatformCode, func(ctx context.Context) (int64, error) {
		return s.policies.ApplyManagedLoginPolicy(ctx, platformfacade.ApplyManagedLoginPolicyCommand{ManagedLoginPolicy: managed})
	})
}

// ApplyHubConnection applies a complete connection version through its port.
func (s *Service) ApplyHubConnection(ctx context.Context, command nodefacade.ApplyHubConnectionCommand) (*nodefacade.CommandResult, error) {
	if err := validateCommand(command.IdempotencyKey, command.Reason); err != nil {
		return nil, err
	}
	if s == nil || s.hubConnection == nil {
		return nil, apperrors.ServiceUnavailable("Hub连接管理能力尚未绑定")
	}
	connectionVersion := strings.TrimSpace(command.ConnectionVersion)
	clientID := strings.TrimSpace(command.ClientID)
	redirectURI := strings.TrimSpace(command.RedirectURI)
	if connectionVersion == "" || clientID == "" || redirectURI == "" {
		return nil, apperrors.Params("Hub连接参数不完整")
	}
	if command.TargetRevision < 0 {
		return nil, apperrors.Params("targetRevision格式无效")
	}
	if len(connectionVersion) > 128 || len(clientID) > 255 || len(command.ClientSecret) > 4096 || len(command.Issuer) > 2048 || len(redirectURI) > 2048 || len([]rune(command.DisplayName)) > 128 {
		return nil, apperrors.Params("Hub连接参数长度超过限制")
	}
	if command.Enabled && strings.TrimSpace(command.ClientSecret) == "" {
		return nil, apperrors.Params("启用Hub连接时clientSecret不能为空")
	}
	issuer, err := url.Parse(strings.TrimSpace(command.Issuer))
	if err != nil || issuer.Host == "" || (issuer.Scheme != "https" && issuer.Scheme != "http") {
		return nil, apperrors.Params("issuer格式错误")
	}
	redirect, redirectErr := url.Parse(redirectURI)
	if redirectErr != nil || redirect.Host == "" || (redirect.Scheme != "https" && redirect.Scheme != "http") {
		return nil, apperrors.Params("redirectUri格式错误")
	}
	managed := nodefacade.ManagedHubConnectionCommand{ConnectionVersion: connectionVersion, TargetRevision: command.TargetRevision, Enabled: command.Enabled, DisplayName: strings.TrimSpace(command.DisplayName), Issuer: issuer.String(), ClientID: clientID, ClientSecret: command.ClientSecret, RedirectURI: redirect.String()}
	return execute(s, ctx, httpMethodPut, "/internal/node/v1/hub-connection", command.IdempotencyKey, command, "NODE_HUB_CONNECTION_APPLY", command.ConnectionVersion, func(ctx context.Context) (int64, error) {
		return 1, s.hubConnection.ApplyHubConnection(ctx, managed)
	})
}

const (
	httpMethodPut  = "PUT"
	httpMethodPost = "POST"
)

func execute(s *Service, ctx context.Context, method, path, idempotencyKey string, digestValue any, commandName, target string, operation func(context.Context) (int64, error)) (*nodefacade.CommandResult, error) {
	metadata, err := commandMetadata(s, method, path, idempotencyKey, digestValue)
	if err != nil {
		return nil, err
	}
	return executeCommand(s, ctx, metadata, commandName, target, operation)
}

func commandMetadata(s *Service, method, path, idempotencyKey string, digestValue any) (nodedomain.CommandMetadata, error) {
	if s == nil || s.replay == nil {
		return nodedomain.CommandMetadata{}, apperrors.ServiceUnavailable("命令幂等能力不可用")
	}
	requestDigest, err := digest(digestValue)
	if err != nil {
		return nodedomain.CommandMetadata{}, err
	}
	return nodedomain.CommandMetadata{NodeCode: s.config.NodeCode, Method: method, Path: path, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest}, nil
}

func executeCommand(s *Service, ctx context.Context, metadata nodedomain.CommandMetadata, commandName, target string, operation func(context.Context) (int64, error)) (*nodefacade.CommandResult, error) {
	payload, replayed, err := s.replay.Execute(ctx, metadata, func(ctx context.Context) ([]byte, error) {
		changed, err := operation(ctx)
		if err != nil {
			return nil, err
		}
		result := struct {
			ChangedCount int64 `json:"changedCount"`
		}{ChangedCount: changed}
		return json.Marshal(result)
	})
	if err != nil {
		return nil, err
	}
	var result nodefacade.CommandResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, apperrors.ServiceUnavailable("命令结果无法读取")
	}
	result.Replayed = replayed
	if err := s.writeAudit(ctx, commandName, metadata.Method, metadata.Path, target, metadata.IdempotencyKey, result.ChangedCount); err != nil {
		return nil, err
	}
	return &result, nil
}

func (s *Service) writeAudit(ctx context.Context, commandName, method, path, target, idempotencyKey string, changed int64) error {
	if s.audit == nil {
		return apperrors.ServiceUnavailable("同步操作审计能力不可用")
	}
	params, _ := json.Marshal(map[string]any{"command": commandName, "targetHash": hashText(target), "idempotencyKeyHash": hashText(idempotencyKey), "changedCount": changed})
	callerID := strings.TrimSpace(s.config.CallerIDHash)
	if len(callerID) > 48 {
		callerID = callerID[:48]
	}
	return s.audit.SaveLog(ctx, adminfacade.OperationLogEntry{UserName: "node-management:" + callerID, OperationType: adminfacade.OperationTypeOther, OperationDesc: commandName, MethodName: commandName, RequestMethod: method, RequestURL: safeAuditPath(commandName, path), TraceID: xcontext.TraceIDFromContext(ctx), RequestParams: string(params), OperationTime: s.now().UTC(), Status: 1})
}

func safeAuditPath(commandName, fallback string) string {
	switch commandName {
	case "NODE_USER_STATUS":
		return "/internal/node/v1/users/:userId/status"
	case "NODE_SESSION_REVOKE":
		return "/internal/node/v1/users/:userId/sessions/revoke"
	default:
		return fallback
	}
}

func digest(value any) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", apperrors.Params("命令请求无法编码")
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func validateCommand(key, reason string) error {
	if strings.TrimSpace(key) == "" || len(key) > 256 {
		return apperrors.Params("Idempotency-Key不能为空且不能超过256字符")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > maxReasonLength {
		return apperrors.Params("reason不能为空且不能超过512字符")
	}
	return nil
}

func parseID(value string) (int64, error) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed <= 0 {
		return 0, apperrors.Params("userId格式错误")
	}
	return parsed, nil
}

func normalizePage(current, size int64) (int64, int64) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	return current, size
}

func pageOffset(current, size, maxOffset int64) (int64, error) {
	if current <= 0 || size <= 0 || maxOffset < 0 {
		return 0, apperrors.Params("分页参数错误")
	}
	pageIndex := current - 1
	if pageIndex > maxOffset/size {
		return 0, apperrors.Params("分页偏移量超出范围")
	}
	return pageIndex * size, nil
}

func mapUser(record userfacade.AdminUserVO) nodefacade.UserSummary {
	return nodefacade.UserSummary{UserID: nodefacade.FormatID(record.ID), Username: record.Username, Nickname: record.Nickname, EmailMasked: maskEmail(record.Email), PhoneMasked: maskPhone(record.UserPhone), Status: record.Status, CreatedAt: record.CreateTime, UpdatedAt: record.UpdateTime}
}

func maskEmail(value string) string {
	value = strings.TrimSpace(value)
	local, domain, ok := strings.Cut(value, "@")
	if !ok || local == "" || domain == "" {
		return ""
	}
	runes := []rune(local)
	return string(runes[:1]) + "***@" + domain
}

func maskPhone(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 7 {
		return ""
	}
	return value[:3] + "****" + value[len(value)-4:]
}

func effectiveSessionStatus(record ssofacade.SessionRecord, now time.Time) string {
	if record.RevokedAt != nil || strings.EqualFold(record.Status, nodefacade.SessionStatusRevoked) {
		return nodefacade.SessionStatusRevoked
	}
	if record.ExpiresAt != nil && !record.ExpiresAt.After(now) {
		return nodefacade.SessionStatusExpired
	}
	return nodefacade.SessionStatusActive
}

func mapPolicy(policy *platformfacade.ManagedLoginPolicy) *nodefacade.ManagedLoginPolicy {
	if policy == nil {
		return nil
	}
	result := &nodefacade.ManagedLoginPolicy{PlatformCode: policy.PlatformCode, Status: policy.Status, AllowAutoRegister: policy.AllowAutoRegister, AllowFormRegister: policy.AllowFormRegister, LoginMethods: make([]nodefacade.LoginMethod, 0, len(policy.LoginMethods)), SourceRules: make([]nodefacade.SourceRule, 0, len(policy.SourceRules))}
	for _, method := range policy.LoginMethods {
		result.LoginMethods = append(result.LoginMethods, nodefacade.LoginMethod{MethodType: method.MethodType, ProviderCode: method.ProviderCode, DisplayName: method.DisplayName, Icon: method.Icon, SortOrder: method.SortOrder, DisplayEnabled: method.DisplayEnabled, LoginEnabled: method.LoginEnabled})
	}
	for _, rule := range policy.SourceRules {
		result.SourceRules = append(result.SourceRules, nodefacade.SourceRule{MatchType: rule.MatchType, MatchValue: rule.MatchValue, Priority: rule.Priority, Status: rule.Status})
	}
	return result
}

func mapManagedPolicy(policy nodefacade.ManagedLoginPolicy) platformfacade.ManagedLoginPolicy {
	result := platformfacade.ManagedLoginPolicy{PlatformCode: policy.PlatformCode, Status: policy.Status, AllowAutoRegister: policy.AllowAutoRegister, AllowFormRegister: policy.AllowFormRegister, LoginMethods: make([]platformfacade.ManagedLoginMethod, 0, len(policy.LoginMethods)), SourceRules: make([]platformfacade.ManagedSourceRule, 0, len(policy.SourceRules))}
	for _, method := range policy.LoginMethods {
		result.LoginMethods = append(result.LoginMethods, platformfacade.ManagedLoginMethod{MethodType: method.MethodType, ProviderCode: method.ProviderCode, DisplayName: method.DisplayName, Icon: method.Icon, SortOrder: method.SortOrder, DisplayEnabled: method.DisplayEnabled, LoginEnabled: method.LoginEnabled})
	}
	for _, rule := range policy.SourceRules {
		result.SourceRules = append(result.SourceRules, platformfacade.ManagedSourceRule{MatchType: rule.MatchType, MatchValue: rule.MatchValue, Priority: rule.Priority, Status: rule.Status})
	}
	return result
}

type sessionReferencePayload struct {
	UserID    int64  `json:"u"`
	SessionID string `json:"s"`
}
type sessionReferenceCodec struct{ aead cipher.AEAD }

// NewSessionReferenceCodec builds an authenticated Node-bound opaque codec.
func NewSessionReferenceCodec(nodeCode, secret string) (nodefacade.SessionReferenceCodec, error) {
	key := sha256.Sum256([]byte(strings.TrimSpace(nodeCode) + "\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &sessionReferenceCodec{aead: aead}, nil
}

func (c *sessionReferenceCodec) Encode(_ context.Context, record ssofacade.SessionRecord) (string, error) {
	if c == nil || c.aead == nil || record.UserID <= 0 || strings.TrimSpace(record.SessionID) == "" {
		return "", apperrors.Params("会话引用无法生成")
	}
	payload, _ := json.Marshal(sessionReferencePayload{UserID: record.UserID, SessionID: record.SessionID})
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate session reference nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, payload, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (c *sessionReferenceCodec) Decode(_ context.Context, reference string) (nodefacade.SessionReference, error) {
	if c == nil || c.aead == nil {
		return nodefacade.SessionReference{}, apperrors.ServiceUnavailable("会话引用能力不可用")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(reference))
	if err != nil || len(sealed) <= c.aead.NonceSize() {
		return nodefacade.SessionReference{}, apperrors.Params("sessionRef格式错误")
	}
	nonce, ciphertext := sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():]
	payload, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nodefacade.SessionReference{}, apperrors.Forbidden("sessionRef校验失败")
	}
	var decoded sessionReferencePayload
	if json.Unmarshal(payload, &decoded) != nil || decoded.UserID <= 0 || strings.TrimSpace(decoded.SessionID) == "" {
		return nodefacade.SessionReference{}, apperrors.Params("sessionRef格式错误")
	}
	return nodefacade.SessionReference{UserID: decoded.UserID, SessionID: decoded.SessionID}, nil
}

var _ nodefacade.NodeManagementFacade = (*Service)(nil)
