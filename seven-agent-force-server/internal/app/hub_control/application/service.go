package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/domain"
	hubfacade "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/facade"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/federation"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
)

var ErrAmbiguousTransport = errors.New("ambiguous remote transport failure")

type NodeClient interface {
	Describe(context.Context, domain.Node) (*nodefacade.NodeDescriptor, error)
	ListUsers(context.Context, domain.Node, nodefacade.UserPageQuery) (*nodefacade.UserPage, error)
	GetUser(context.Context, domain.Node, string) (*nodefacade.UserDetail, error)
	SetUserStatus(context.Context, domain.Node, nodefacade.SetUserStatusCommand) error
	ListUserSessions(context.Context, domain.Node, string, nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error)
	RevokeUserSessions(context.Context, domain.Node, nodefacade.RevokeUserSessionsCommand) error
	GetLoginPolicy(context.Context, domain.Node) (*nodefacade.ManagedLoginPolicy, error)
	ApplyLoginPolicy(context.Context, domain.Node, nodefacade.ApplyLoginPolicyCommand) error
	ApplyHubConnection(context.Context, domain.Node, nodefacade.ApplyHubConnectionCommand) error
}

type Service struct {
	repository      domain.Repository
	remote          NodeClient
	managedSSO      hubfacade.ManagedSSOClientFacade
	secrets         domain.SecretService
	transactor      domain.Transactor
	nextID          func() int64
	allowHTTPIssuer bool
}

type Option func(*Service)

// WithDevelopmentHTTPIssuer permits loopback/test HTTP issuers outside production.
func WithDevelopmentHTTPIssuer() Option {
	return func(service *Service) { service.allowHTTPIssuer = true }
}

// BindTransactor enables one local transaction for Hub metadata and managed SSO handoff.
func (s *Service) BindTransactor(transactor domain.Transactor) {
	if s != nil {
		s.transactor = transactor
	}
}

func NewService(repository domain.Repository, remote NodeClient, managedSSO hubfacade.ManagedSSOClientFacade, secrets domain.SecretService, nextID func() int64, options ...Option) *Service {
	service := &Service{repository: repository, remote: remote, managedSSO: managedSSO, secrets: secrets, nextID: nextID}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) PageNodes(ctx context.Context, query hubfacade.NodePageQuery) (*hubfacade.NodePage, error) {
	current, size, err := normalizePage(query.Current, query.Size)
	if err != nil {
		return nil, err
	}
	keyword := strings.TrimSpace(query.Keyword)
	if len(keyword) > 256 {
		return nil, apperrors.Params("keyword不能超过256字符")
	}
	items, total, err := s.repository.Page(ctx, domain.NodePageQuery{Current: current, Size: size, Keyword: keyword, Status: query.Status})
	if err != nil {
		return nil, err
	}
	result := &hubfacade.NodePage{Current: current, Size: size, Total: total, Records: make([]hubfacade.NodeDetail, 0, len(items))}
	for _, item := range items {
		result.Records = append(result.Records, toDetail(item))
	}
	return result, nil
}

func (s *Service) GetNode(ctx context.Context, nodeCode string) (*hubfacade.NodeDetail, error) {
	node, err := s.requireNode(ctx, nodeCode, false)
	if err != nil {
		return nil, err
	}
	detail := toDetail(*node)
	return &detail, nil
}

func (s *Service) SaveNode(ctx context.Context, command hubfacade.SaveNodeCommand) (*hubfacade.NodeDetail, error) {
	normalized, err := s.normalizeSave(command)
	if err != nil {
		return nil, err
	}
	if s.transactor == nil {
		return nil, apperrors.System("Hub节点事务未配置")
	}
	lookupCode := normalized.OriginalNodeCode
	if lookupCode == "" {
		lookupCode = normalized.NodeCode
	}
	var replacement domain.EncryptedSecret
	if strings.TrimSpace(normalized.ManagementBearer) != "" {
		replacement, err = s.encrypt(ctx, normalized.ManagementBearer)
		if err != nil {
			return nil, err
		}
	}
	var saved *domain.Node
	err = s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		existing, findErr := s.repository.FindForUpdate(txCtx, lookupCode)
		if findErr != nil {
			return findErr
		}
		now := time.Now().UTC()
		if existing == nil {
			if normalized.OriginalNodeCode != "" {
				return apperrors.NotFound("Node不存在")
			}
			if !replacement.Present() {
				return apperrors.Params("managementBearer不能为空")
			}
			node := &domain.Node{ID: s.nextID(), NodeCode: normalized.NodeCode, NodeName: normalized.NodeName, Status: normalized.Status, DiscoveryType: normalized.DiscoveryType, ServiceName: normalized.ServiceName, ManagementBaseURL: normalized.ManagementBaseURL, HubIssuer: normalized.HubIssuer, ManagementBearer: replacement, CapabilitiesJSON: normalized.CapabilitiesJSON, ConnectionStatus: domain.ConnectionPending, TargetRevision: 1, CreatedAt: now, UpdatedAt: now}
			if insertErr := s.repository.Insert(txCtx, node); insertErr != nil {
				return insertErr
			}
			saved = node
			return nil
		}
		previousVersion, previousHash, previousRevision := existing.ConnectionVersion, existing.ConnectionRequestHash, existing.EffectiveTargetRevision()
		if editErr := existing.EditMetadata(nodeMetadataFromSave(normalized), now); editErr != nil {
			return mapDomainMutationError(editErr)
		}
		statusChanged := existing.Status != normalized.Status
		existing.SetStatus(normalized.Status, now)
		if updateErr := s.repository.UpdateMetadata(txCtx, existing); updateErr != nil {
			return updateErr
		}
		if statusChanged {
			if updateErr := s.repository.UpdateStatus(txCtx, existing); updateErr != nil {
				return updateErr
			}
		}
		if replacement.Present() {
			existing.ReplaceManagementBearer(replacement, now)
			if updateErr := s.repository.ReplaceManagementBearer(txCtx, existing); updateErr != nil {
				return updateErr
			}
		}
		if statusChanged && existing.OIDCClientID != "" {
			if statusErr := s.managedSSO.SetManagedClientStatus(txCtx, hubfacade.ManagedSSOClientStatusCommand{ClientID: existing.OIDCClientID, OwnerNodeCode: existing.NodeCode, Status: existing.Status}); statusErr != nil {
				return statusErr
			}
		}
		if existing.TargetRevision != previousRevision {
			if targetErr := s.repository.UpdateTargetState(txCtx, existing); targetErr != nil {
				return targetErr
			}
			if supersedeErr := s.supersedeConnectionCommand(txCtx, existing.NodeCode, previousVersion, previousHash, previousRevision, now); supersedeErr != nil {
				return supersedeErr
			}
		}
		saved = existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	if normalized.Status == domain.NodeStatusDisabled {
		if err := s.disableManagedFederation(ctx, saved); err != nil {
			return nil, err
		}
	}
	detail := toDetail(*saved)
	return &detail, nil
}

func (s *Service) CopyNode(ctx context.Context, sourceCode string, command hubfacade.CopyNodeCommand) (*hubfacade.NodeDetail, error) {
	source, err := s.requireNode(ctx, sourceCode, false)
	if err != nil {
		return nil, err
	}
	if err := validateNodeCode(command.NodeCode); err != nil {
		return nil, err
	}
	if strings.TrimSpace(command.NodeName) == "" || len([]rune(command.NodeName)) > 128 {
		return nil, apperrors.Params("nodeName不能为空且不能超过128字符")
	}
	if len(command.ManagementBearer) > 8192 {
		return nil, apperrors.Params("managementBearer不能超过8192字符")
	}
	if duplicate, findErr := s.repository.Find(ctx, command.NodeCode); findErr != nil {
		return nil, findErr
	} else if duplicate != nil {
		return nil, apperrors.ObjectState("nodeCode已存在")
	}
	bearer := source.ManagementBearer
	if strings.TrimSpace(command.ManagementBearer) != "" {
		bearer, err = s.encrypt(ctx, command.ManagementBearer)
		if err != nil {
			return nil, err
		}
	}
	now := time.Now().UTC()
	copyValue := source.Copy(s.nextID(), command.NodeCode, strings.TrimSpace(command.NodeName), bearer, now)
	copyNode := &copyValue
	if err := s.repository.Insert(ctx, copyNode); err != nil {
		return nil, err
	}
	detail := toDetail(*copyNode)
	return &detail, nil
}

func (s *Service) SetNodeStatus(ctx context.Context, command hubfacade.SetNodeStatusCommand) error {
	if command.Status != domain.NodeStatusEnabled && command.Status != domain.NodeStatusDisabled {
		return apperrors.Params("status无效")
	}
	if s.transactor == nil {
		return apperrors.System("Hub节点事务未配置")
	}
	var updated *domain.Node
	err := s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		node, err := s.repository.FindForUpdate(txCtx, strings.TrimSpace(command.NodeCode))
		if err != nil {
			return err
		}
		if node == nil {
			return apperrors.NotFound("Node不存在")
		}
		now := time.Now().UTC()
		previousVersion, previousHash, previousRevision := node.ConnectionVersion, node.ConnectionRequestHash, node.EffectiveTargetRevision()
		node.SetStatus(command.Status, now)
		if node.OIDCClientID != "" {
			if statusErr := s.managedSSO.SetManagedClientStatus(txCtx, hubfacade.ManagedSSOClientStatusCommand{ClientID: node.OIDCClientID, OwnerNodeCode: node.NodeCode, Status: node.Status}); statusErr != nil {
				return statusErr
			}
		}
		if err := s.repository.UpdateStatus(txCtx, node); err != nil {
			return err
		}
		if node.TargetRevision != previousRevision {
			if err := s.repository.UpdateTargetState(txCtx, node); err != nil {
				return err
			}
			if err := s.supersedeConnectionCommand(txCtx, node.NodeCode, previousVersion, previousHash, previousRevision, now); err != nil {
				return err
			}
		}
		updated = node
		return nil
	})
	if err != nil {
		return err
	}
	if command.Status == domain.NodeStatusDisabled {
		return s.disableManagedFederation(ctx, updated)
	}
	return nil
}

func (s *Service) disableManagedFederation(ctx context.Context, node *domain.Node) error {
	if node == nil || strings.TrimSpace(node.OIDCClientID) == "" || strings.TrimSpace(node.ConnectionVersion) == "" {
		return nil
	}
	identity := fmt.Sprintf("%s\x00%s\x00%d\x00disabled", node.NodeCode, node.ConnectionVersion, node.EffectiveTargetRevision())
	sum := sha256.Sum256([]byte(identity))
	commandID := hex.EncodeToString(sum[:])
	return s.remote.ApplyHubConnection(ctx, *node, nodefacade.ApplyHubConnectionCommand{
		ConnectionVersion: "disable-" + commandID,
		TargetRevision:    node.EffectiveTargetRevision(),
		Enabled:           false,
		DisplayName:       node.NodeName,
		Issuer:            node.HubIssuer,
		ClientID:          node.OIDCClientID,
		RedirectURI:       node.HubIssuer,
		Reason:            "Hub Node disabled",
		IdempotencyKey:    "hub-disable-" + commandID,
	})
}

func (s *Service) TestConnection(ctx context.Context, nodeCode string) (*hubfacade.NodeHealth, error) {
	node, err := s.requireNode(ctx, nodeCode, true)
	if err != nil {
		return nil, err
	}
	descriptor, err := s.remote.Describe(ctx, *node)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	node.RecordHealthy(now)
	if err := s.repository.UpdateHealth(ctx, node); err != nil {
		return nil, err
	}
	return &hubfacade.NodeHealth{NodeCode: descriptor.NodeCode, Version: descriptor.Version, Capabilities: descriptor.Capabilities, Health: descriptor.Health}, nil
}

func (s *Service) ListNodeUsers(ctx context.Context, code string, query nodefacade.UserPageQuery) (*nodefacade.UserPage, error) {
	node, err := s.requireNode(ctx, code, true)
	if err != nil {
		return nil, err
	}
	if _, _, err = normalizePage64(query.Current, query.Size); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(query.Keyword)) > 256 {
		return nil, apperrors.Params("keyword不能超过256字符")
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	return s.remote.ListUsers(ctx, *node, query)
}
func (s *Service) GetNodeUser(ctx context.Context, code, userID string) (*nodefacade.UserDetail, error) {
	node, err := s.requireNode(ctx, code, true)
	if err != nil {
		return nil, err
	}
	if err = validateBounded("userId", userID, 128); err != nil {
		return nil, err
	}
	return s.remote.GetUser(ctx, *node, userID)
}
func (s *Service) SetNodeUserStatus(ctx context.Context, command hubfacade.NodeUserStatusCommand) error {
	node, err := s.requireNode(ctx, command.NodeCode, true)
	if err != nil {
		return err
	}
	remoteCommand := nodefacade.SetUserStatusCommand{UserID: command.UserID, Status: command.Status, Reason: command.Reason, IdempotencyKey: command.IdempotencyKey}
	if err = validateBounded("userId", remoteCommand.UserID, 128); err != nil {
		return err
	}
	if remoteCommand.Status != nodefacade.UserStatusNormal && remoteCommand.Status != nodefacade.UserStatusDisabled && remoteCommand.Status != nodefacade.UserStatusPendingReview {
		return apperrors.Params("status无效")
	}
	if err = validateWrite(remoteCommand.IdempotencyKey, remoteCommand.Reason); err != nil {
		return err
	}
	return s.remote.SetUserStatus(ctx, *node, remoteCommand)
}
func (s *Service) ListNodeUserSessions(ctx context.Context, code, userID string, query nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error) {
	node, err := s.requireNode(ctx, code, true)
	if err != nil {
		return nil, err
	}
	if _, _, err = normalizePage64(query.Current, query.Size); err != nil {
		return nil, err
	}
	if err = validateBounded("userId", userID, 128); err != nil {
		return nil, err
	}
	return s.remote.ListUserSessions(ctx, *node, userID, query)
}
func (s *Service) RevokeNodeUserSessions(ctx context.Context, command hubfacade.RevokeNodeSessionsCommand) error {
	node, err := s.requireNode(ctx, command.NodeCode, true)
	if err != nil {
		return err
	}
	remote := nodefacade.RevokeUserSessionsCommand{UserID: command.UserID, All: command.All, SessionRefs: append([]string(nil), command.SessionRefs...), Reason: command.Reason, IdempotencyKey: command.IdempotencyKey}
	if err = validateBounded("userId", remote.UserID, 128); err != nil {
		return err
	}
	if len(remote.SessionRefs) > nodefacade.MaxSessionReferencesPerCommand {
		return apperrors.Params("sessionRefs数量过多")
	}
	for _, ref := range remote.SessionRefs {
		if err = validateBounded("sessionRef", ref, 512); err != nil {
			return err
		}
	}
	if err = validateWrite(remote.IdempotencyKey, remote.Reason); err != nil {
		return err
	}
	return s.remote.RevokeUserSessions(ctx, *node, remote)
}
func (s *Service) GetNodeLoginPolicy(ctx context.Context, code string) (*nodefacade.ManagedLoginPolicy, error) {
	node, err := s.requireNode(ctx, code, true)
	if err != nil {
		return nil, err
	}
	return s.remote.GetLoginPolicy(ctx, *node)
}
func (s *Service) ApplyNodeLoginPolicy(ctx context.Context, code string, command nodefacade.ApplyLoginPolicyCommand) error {
	node, err := s.requireNode(ctx, code, true)
	if err != nil {
		return err
	}
	if err = validateWrite(command.IdempotencyKey, command.Reason); err != nil {
		return err
	}
	return s.remote.ApplyLoginPolicy(ctx, *node, command)
}

func (s *Service) GetFederationStatus(ctx context.Context, code string) (*hubfacade.FederationStatus, error) {
	node, err := s.requireNode(ctx, code, false)
	if err != nil {
		return nil, err
	}
	return &hubfacade.FederationStatus{NodeCode: node.NodeCode, OIDCClientID: node.OIDCClientID, ConnectionStatus: node.ConnectionStatus, ConnectionVersion: node.ConnectionVersion, LastConnectionError: node.LastConnectionError, LastConnectionTraceID: node.LastConnectionTraceID}, nil
}

func (s *Service) ProvisionNodeConnection(ctx context.Context, command hubfacade.ProvisionConnectionCommand) error {
	ctx, _ = xcontext.EnsureContextTraceID(ctx)
	if err := s.validateProvision(command); err != nil {
		return err
	}
	requestHash, err := provisionRequestHash(command)
	if err != nil {
		return err
	}
	if s.transactor == nil {
		return apperrors.System("Hub连接编排事务未配置")
	}
	var node *domain.Node
	alreadyActive := false
	err = s.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		locked, findErr := s.repository.FindForUpdate(txCtx, strings.TrimSpace(command.NodeCode))
		if findErr != nil {
			return findErr
		}
		if locked == nil {
			return apperrors.NotFound("Node不存在")
		}
		if locked.Status != domain.NodeStatusEnabled {
			return apperrors.ObjectState("Node已禁用")
		}
		locked.EffectiveTargetRevision()
		commandRecord, commandErr := s.repository.FindConnectionCommandForUpdate(txCtx, locked.NodeCode, command.ConnectionVersion)
		if commandErr != nil {
			return commandErr
		}
		if commandRecord != nil {
			if commandRecord.RequestHash != requestHash {
				return mapDomainMutationError(domain.ErrProvisionReplay)
			}
			if commandRecord.State == domain.CommandSuperseded || locked.ConnectionVersion != command.ConnectionVersion || commandRecord.TargetRevision != locked.TargetRevision {
				return mapDomainMutationError(domain.ErrProvisionSuperseded)
			}
		} else {
			if locked.ConnectionVersion == command.ConnectionVersion && locked.ConnectionRequestHash != "" && locked.ConnectionRequestHash != requestHash {
				return mapDomainMutationError(domain.ErrProvisionReplay)
			}
			if locked.ConnectionVersion != "" && locked.ConnectionVersion != command.ConnectionVersion {
				if supersedeErr := s.supersedeConnectionCommand(txCtx, locked.NodeCode, locked.ConnectionVersion, locked.ConnectionRequestHash, locked.TargetRevision, time.Now().UTC()); supersedeErr != nil {
					return supersedeErr
				}
			}
			now := time.Now().UTC()
			commandRecord = &domain.ConnectionCommand{NodeCode: locked.NodeCode, ConnectionVersion: command.ConnectionVersion, RequestHash: requestHash, TargetRevision: locked.TargetRevision, State: domain.CommandPending, CreatedAt: now, UpdatedAt: now}
		}
		decision, startErr := locked.StartProvision(command.ConnectionVersion, requestHash, command.RotateSecret, time.Now().UTC())
		if startErr != nil {
			return mapDomainMutationError(startErr)
		}
		if decision.AlreadyActive {
			commandRecord.State = domain.CommandActive
			commandRecord.UpdatedAt = time.Now().UTC()
			if saveErr := s.repository.SaveConnectionCommand(txCtx, commandRecord); saveErr != nil {
				return saveErr
			}
			node, alreadyActive = locked, true
			return nil
		}
		clientID := locked.OIDCClientID
		if clientID == "" {
			clientID = managedClientID(locked.NodeCode)
		}
		if decision.NeedsManagedClient {
			managed, managedErr := s.managedSSO.UpsertManagedClient(txCtx, hubfacade.ManagedSSOClientCommand{ClientID: clientID, ClientName: "Hub Node " + locked.NodeName, RedirectURI: command.RedirectURI, RotateSecret: decision.RotateSecret, OwnerNodeCode: locked.NodeCode})
			if managedErr != nil {
				return managedErr
			}
			managedClientID := clientID
			if managed != nil && managed.ClientID != "" {
				managedClientID = managed.ClientID
			}
			var managedSecret domain.EncryptedSecret
			if managed != nil && managed.ClientSecret != "" && (!locked.OIDCClientSecret.Present() || decision.RotateSecret) {
				managedSecret, managedErr = s.encrypt(txCtx, managed.ClientSecret)
				if managedErr != nil {
					return managedErr
				}
			}
			locked.AcceptManagedClient(managedClientID, managedSecret, time.Now().UTC())
		}
		if !locked.OIDCClientSecret.Present() {
			return errors.New("managed SSO client secret unavailable")
		}
		if updateErr := s.repository.UpdateConnection(txCtx, locked); updateErr != nil {
			return updateErr
		}
		commandRecord.TargetRevision = decision.TargetRevision
		commandRecord.State = domain.CommandPending
		commandRecord.UpdatedAt = time.Now().UTC()
		if saveErr := s.repository.SaveConnectionCommand(txCtx, commandRecord); saveErr != nil {
			return saveErr
		}
		node = locked
		return nil
	})
	if err != nil {
		return err
	}
	if alreadyActive {
		return nil
	}
	secret, err := s.decrypt(ctx, node.OIDCClientSecret)
	if err != nil {
		return s.failSaga(ctx, node, err)
	}
	remoteCommand := nodefacade.ApplyHubConnectionCommand{ConnectionVersion: command.ConnectionVersion, TargetRevision: node.EffectiveTargetRevision(), Enabled: true, DisplayName: command.DisplayName, Issuer: node.HubIssuer, ClientID: node.OIDCClientID, ClientSecret: secret, RedirectURI: command.RedirectURI, Reason: command.Reason, IdempotencyKey: command.IdempotencyKey}
	if err = s.remote.ApplyHubConnection(ctx, *node, remoteCommand); err != nil {
		return s.failSaga(ctx, node, err)
	}
	return s.completeSaga(ctx, node)
}

func (s *Service) failSaga(ctx context.Context, node *domain.Node, cause error) error {
	if node == nil || s.transactor == nil {
		return cause
	}
	terminalCtx, cancel := terminalPersistenceContext(ctx)
	defer cancel()
	err := s.transactor.WithinTransaction(terminalCtx, func(txCtx context.Context) error {
		current, findErr := s.repository.FindForUpdate(txCtx, node.NodeCode)
		if findErr != nil {
			return findErr
		}
		if current == nil {
			return apperrors.NotFound("Node不存在")
		}
		_, traceID := xcontext.EnsureContextTraceID(txCtx)
		now := time.Now().UTC()
		if !current.FailProvisionForTarget(node.ConnectionVersion, node.ConnectionRequestHash, node.TargetRevision, "节点连接编排失败", traceID, now) {
			return nil
		}
		if err := s.repository.UpdateConnection(txCtx, current); err != nil {
			return err
		}
		return s.saveConnectionCommandState(txCtx, node, domain.CommandError, now)
	})
	if err != nil {
		return fmt.Errorf("persist connection error after %v: %w", cause, err)
	}
	return cause
}

func (s *Service) completeSaga(ctx context.Context, node *domain.Node) error {
	if node == nil || s.transactor == nil {
		return apperrors.System("Hub连接编排事务未配置")
	}
	terminalCtx, cancel := terminalPersistenceContext(ctx)
	defer cancel()
	return s.transactor.WithinTransaction(terminalCtx, func(txCtx context.Context) error {
		current, err := s.repository.FindForUpdate(txCtx, node.NodeCode)
		if err != nil {
			return err
		}
		if current == nil {
			return apperrors.NotFound("Node不存在")
		}
		now := time.Now().UTC()
		if completeErr := current.CompleteProvisionForTarget(node.ConnectionVersion, node.ConnectionRequestHash, node.TargetRevision, now); completeErr != nil {
			return mapDomainMutationError(completeErr)
		}
		if err := s.repository.UpdateConnection(txCtx, current); err != nil {
			return err
		}
		return s.saveConnectionCommandState(txCtx, node, domain.CommandActive, now)
	})
}

func (s *Service) supersedeConnectionCommand(ctx context.Context, nodeCode, version, requestHash string, targetRevision int64, now time.Time) error {
	if strings.TrimSpace(version) == "" {
		return nil
	}
	command, err := s.repository.FindConnectionCommandForUpdate(ctx, nodeCode, version)
	if err != nil {
		return err
	}
	if command == nil {
		command = &domain.ConnectionCommand{NodeCode: nodeCode, ConnectionVersion: version, RequestHash: durableRequestHash(requestHash), TargetRevision: targetRevision, CreatedAt: now}
	}
	command.State = domain.CommandSuperseded
	command.UpdatedAt = now
	return s.repository.SaveConnectionCommand(ctx, command)
}

func (s *Service) saveConnectionCommandState(ctx context.Context, node *domain.Node, state string, now time.Time) error {
	command, err := s.repository.FindConnectionCommandForUpdate(ctx, node.NodeCode, node.ConnectionVersion)
	if err != nil {
		return err
	}
	if command == nil || command.RequestHash != node.ConnectionRequestHash || command.TargetRevision != node.TargetRevision {
		return mapDomainMutationError(domain.ErrStaleProvisionResult)
	}
	command.State = state
	command.UpdatedAt = now
	return s.repository.SaveConnectionCommand(ctx, command)
}

func durableRequestHash(value string) string {
	if len(value) == 64 {
		return value
	}
	return strings.Repeat("0", 64)
}

func terminalPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
}
func (s *Service) requireNode(ctx context.Context, code string, requireEnabled bool) (*domain.Node, error) {
	if err := validateNodeCode(code); err != nil {
		return nil, err
	}
	node, err := s.repository.Find(ctx, strings.TrimSpace(code))
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, apperrors.NotFound("Node不存在")
	}
	if requireEnabled && node.Status != domain.NodeStatusEnabled {
		return nil, apperrors.ObjectState("Node已禁用")
	}
	return node, nil
}
func (s *Service) encrypt(ctx context.Context, plain string) (domain.EncryptedSecret, error) {
	return s.secrets.Encrypt(ctx, plain)
}
func (s *Service) decrypt(ctx context.Context, value domain.EncryptedSecret) (string, error) {
	return s.secrets.Decrypt(ctx, value)
}

func (s *Service) normalizeSave(command hubfacade.SaveNodeCommand) (hubfacade.SaveNodeCommand, error) {
	command.OriginalNodeCode = strings.TrimSpace(command.OriginalNodeCode)
	command.NodeCode = strings.TrimSpace(command.NodeCode)
	command.NodeName = strings.TrimSpace(command.NodeName)
	command.DiscoveryType = strings.ToUpper(strings.TrimSpace(command.DiscoveryType))
	command.ServiceName = strings.TrimSpace(command.ServiceName)
	command.ManagementBaseURL = strings.TrimSpace(command.ManagementBaseURL)
	command.HubIssuer = strings.TrimSpace(command.HubIssuer)
	if err := validateNodeCode(command.NodeCode); err != nil {
		return command, err
	}
	if command.NodeName == "" || len([]rune(command.NodeName)) > 128 {
		return command, apperrors.Params("nodeName不能为空且不能超过128字符")
	}
	if command.Status != domain.NodeStatusEnabled && command.Status != domain.NodeStatusDisabled {
		return command, apperrors.Params("status无效")
	}
	if err := validateDiscovery(command.DiscoveryType, command.ServiceName, command.ManagementBaseURL); err != nil {
		return command, err
	}
	if err := s.validateIssuer(command.HubIssuer); err != nil {
		return command, err
	}
	if len(command.ManagementBaseURL) > 2048 || len(command.ManagementBearer) > 8192 || len(command.CapabilitiesJSON) > 16384 {
		return command, apperrors.Params("请求字段过长")
	}
	return command, nil
}

func normalizeSave(command hubfacade.SaveNodeCommand) (hubfacade.SaveNodeCommand, error) {
	return (&Service{}).normalizeSave(command)
}

func validateDiscovery(kind, serviceName, baseURL string) error {
	switch kind {
	case domain.DiscoveryStatic:
		if serviceName != "" || baseURL == "" {
			return apperrors.Params("STATIC只能配置managementBaseUrl")
		}
		if err := domain.ValidateStaticManagementURL(baseURL); err != nil {
			return apperrors.Params("managementBaseUrl无效")
		}
	case domain.DiscoveryConsul:
		if baseURL != "" || serviceName == "" || len(serviceName) > 128 {
			return apperrors.Params("CONSUL只能配置serviceName")
		}
	default:
		return apperrors.Params("discoveryType无效")
	}
	return nil
}
func (s *Service) validateIssuer(raw string) error {
	if _, err := federation.CanonicalOIDCIssuer(raw, s.allowHTTPIssuer); err != nil {
		return apperrors.Params("hubIssuer无效")
	}
	return nil
}

func provisionRequestHash(command hubfacade.ProvisionConnectionCommand) (string, error) {
	payload, err := json.Marshal(struct {
		NodeCode, ConnectionVersion, DisplayName, RedirectURI, Reason, IdempotencyKey string
		RotateSecret                                                                  bool
	}{command.NodeCode, command.ConnectionVersion, command.DisplayName, command.RedirectURI, command.Reason, command.IdempotencyKey, command.RotateSecret})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
func validateNodeCode(value string) error {
	if _, err := federation.CanonicalManagedOwner(value); err != nil {
		return apperrors.Params("nodeCode格式无效")
	}
	return nil
}
func validateBounded(name, value string, max int) error {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > max {
		return apperrors.Params(name + "不能为空或过长")
	}
	return nil
}
func validateWrite(key, reason string) error {
	if err := validateBounded("Idempotency-Key", key, 256); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" || len([]rune(reason)) > 512 {
		return apperrors.Params("reason不能为空且不能超过512字符")
	}
	return nil
}
func (s *Service) validateProvision(command hubfacade.ProvisionConnectionCommand) error {
	if err := validateNodeCode(command.NodeCode); err != nil {
		return err
	}
	if err := validateBounded("connectionVersion", command.ConnectionVersion, 128); err != nil {
		return err
	}
	if err := validateWrite(command.IdempotencyKey, command.Reason); err != nil {
		return err
	}
	parsed, err := url.Parse(strings.TrimSpace(command.RedirectURI))
	allowedScheme := parsed.Scheme == "https" || (s.allowHTTPIssuer && parsed.Scheme == "http")
	if err != nil || !allowedScheme || parsed.Host == "" || parsed.Fragment != "" || len(command.RedirectURI) > 2048 {
		return apperrors.Params("redirectUri无效")
	}
	if len([]rune(command.DisplayName)) > 128 {
		return apperrors.Params("displayName过长")
	}
	return nil
}

func validateProvision(command hubfacade.ProvisionConnectionCommand) error {
	return (&Service{}).validateProvision(command)
}
func normalizePage(current, size int) (int, int, error) {
	if current == 0 {
		current = 1
	}
	if size == 0 {
		size = 20
	}
	if current < 1 || current > 1000000 || size < 1 || size > 200 {
		return 0, 0, apperrors.Params("分页参数无效")
	}
	return current, size, nil
}
func normalizePage64(current, size int64) (int64, int64, error) {
	if current == 0 {
		current = 1
	}
	if size == 0 {
		size = 20
	}
	if current < 1 || current > 1000000 || size < 1 || size > 200 {
		return 0, 0, apperrors.Params("分页参数无效")
	}
	return current, size, nil
}
func managedClientID(nodeCode string) string { return "hub-node-" + nodeCode }
func nodeMetadataFromSave(command hubfacade.SaveNodeCommand) domain.NodeMetadata {
	return domain.NodeMetadata{
		NodeCode: command.NodeCode, NodeName: command.NodeName, DiscoveryType: command.DiscoveryType,
		ServiceName: command.ServiceName, ManagementBaseURL: command.ManagementBaseURL,
		HubIssuer: command.HubIssuer, CapabilitiesJSON: command.CapabilitiesJSON,
	}
}
func mapDomainMutationError(err error) error {
	switch {
	case errors.Is(err, domain.ErrNodeCodeImmutable):
		return apperrors.ObjectState("nodeCode不可修改，请使用复制创建新节点")
	case errors.Is(err, domain.ErrIssuerLocked):
		return apperrors.ObjectState("连接激活后hubIssuer不可修改")
	case errors.Is(err, domain.ErrProvisionReplay):
		return apperrors.ObjectState("同一connectionVersion的重放请求不一致")
	case errors.Is(err, domain.ErrProvisionSuperseded):
		return apperrors.ObjectState("connectionVersion已被后续版本替代")
	case errors.Is(err, domain.ErrStaleProvisionResult):
		return apperrors.ObjectState("连接编排结果已过期")
	default:
		return err
	}
}
func isAmbiguous(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrAmbiguousTransport) {
		return true
	}
	marker, ok := err.(interface{ Ambiguous() bool })
	return ok && marker.Ambiguous()
}
func toDetail(node domain.Node) hubfacade.NodeDetail {
	return hubfacade.NodeDetail{NodeCode: node.NodeCode, NodeName: node.NodeName, Status: node.Status, DiscoveryType: node.DiscoveryType, ServiceName: node.ServiceName, ManagementBaseURL: node.ManagementBaseURL, HubIssuer: node.HubIssuer, OIDCClientID: node.OIDCClientID, CapabilitiesJSON: node.CapabilitiesJSON, ConnectionStatus: node.ConnectionStatus, ConnectionVersion: node.ConnectionVersion, IssuerLockedAt: node.IssuerLockedAt, LastConnectionError: node.LastConnectionError, LastConnectionTraceID: node.LastConnectionTraceID, LastHealthyAt: node.LastHealthyAt, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt}
}
