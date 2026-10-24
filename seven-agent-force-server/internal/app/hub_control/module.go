package hub_control

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/domain"
	hubfacade "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/facade"
	hubhandler "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/handler"
	hubinfra "github.com/CaixyPromise/seven-agent/force/internal/app/hub_control/infrastructure"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	"github.com/CaixyPromise/seven-agent/force/internal/common/response"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/microservice/consul"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/securitycontext"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xcontext"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/route"
)

type Dependencies struct{ ManagedSSO ssofacade.ManagedClientFacade }
type Module struct {
	handler              *hubhandler.Handler
	oplog                adminfacade.OperationLogger
	closeIdleConnections func()
	shutdownOnce         sync.Once
}

func Install(deps bootstrapruntime.ModuleDeps, refs Dependencies) (*Module, hubfacade.NodeAdminFacade, error) {
	if deps.Infra.Datasource == nil {
		return nil, nil, fmt.Errorf("hub control requires datasource provider")
	}
	if deps.Security.SecretValue == nil {
		return nil, nil, fmt.Errorf("hub control requires secret value service")
	}
	if deps.IDGen == nil {
		return nil, nil, fmt.Errorf("hub control requires id generator")
	}
	if refs.ManagedSSO == nil {
		return nil, nil, fmt.Errorf("hub control requires managed SSO client facade")
	}
	repository, err := hubinfra.NewRepository(deps.Infra.Datasource)
	if err != nil {
		return nil, nil, err
	}
	consulClient, err := consul.NewClient(consul.ClientOptions{Address: deps.Config.Microservice.Registry.Address, Token: deps.Config.Microservice.Registry.Token, Timeout: deps.Config.Microservice.Discovery.ResolveTimeout})
	if err != nil {
		return nil, nil, err
	}
	resolver := consul.NewResolver(consulClient, consul.ResolverOptions{Datacenter: deps.Config.Microservice.Discovery.Datacenter, Tags: deps.Config.Microservice.Discovery.Tags})
	cached := microservice.NewCachedResolverWithOptions(resolver, microservice.CachedResolverOptions{TTL: deps.Config.Microservice.Discovery.CacheTTL, EmptyTTL: deps.Config.Microservice.Discovery.EmptyResultTTL, ResolveTimeout: deps.Config.Microservice.Discovery.ResolveTimeout})
	outboundPolicy, err := microservice.NewOutboundTrustPolicy(microservice.OutboundTrustConfig{TrustedHosts: deps.Config.Microservice.Outbound.TrustedHosts, TrustedCIDRs: deps.Config.Microservice.Outbound.TrustedCIDRs, RegistryTrustedHosts: deps.Config.Microservice.Outbound.RegistryTrustedHosts, RegistryTrustedCIDRs: deps.Config.Microservice.Outbound.RegistryTrustedCIDRs}, nil)
	if err != nil {
		return nil, nil, err
	}
	options := microservice.HTTPClientOptions{ConnectTimeout: deps.Config.Microservice.Client.ConnectTimeout, RequestTimeout: deps.Config.Microservice.Client.RequestTimeout, MaxIdleConns: deps.Config.Microservice.Client.MaxIdleConns, MaxIdleConnsPerHost: deps.Config.Microservice.Client.MaxIdleConnsPerHost, IdleConnTimeout: deps.Config.Microservice.Client.IdleConnTimeout, MaxRequestBytes: deps.Config.Microservice.Client.MaxRequestBytes, MaxResponseBytes: deps.Config.Microservice.Client.MaxResponseBytes, OutboundPolicy: outboundPolicy}
	remote := hubinfra.NewNodeClient(nil, nil, deps.Security.SecretValue, deps.Logger)
	remote.ConfigureRuntime(cached, options)
	serviceOptions := []application.Option{}
	if !isProduction(deps.Config.Profile, deps.Config.Seven.Env) {
		serviceOptions = append(serviceOptions, application.WithDevelopmentHTTPIssuer())
	}
	service := application.NewService(repositoryAdapter{repository}, nodeClientAdapter{remote}, managedSSOAdapter{facade: refs.ManagedSSO}, secretAdapter{deps.Security.SecretValue}, deps.IDGen.NextID, serviceOptions...)
	service.BindTransactor(deps.Infra.Transactor)
	handler := hubhandler.New(service)
	module := &Module{handler: handler, closeIdleConnections: remote.CloseIdleConnections}
	handler.BindRouteWrapper(module.wrapRoute)
	return module, service, nil
}

func isProduction(profile, environment string) bool {
	for _, value := range []string{profile, environment} {
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "prod", "production":
			return true
		}
	}
	return false
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "hub-control", Prefix: "/system/hub/nodes"}
}
func (m *Module) Mount(route.IRouter) {}
func (m *Module) MountHub(router route.IRouter) {
	if m != nil && m.handler != nil && router != nil {
		m.handler.Mount(router)
	}
}
func (m *Module) BindOperationLogger(oplog adminfacade.OperationLogger) {
	if m != nil {
		m.oplog = oplog
	}
}
func (m *Module) Shutdown(context.Context) error {
	if m != nil {
		m.shutdownOnce.Do(func() {
			if m.closeIdleConnections != nil {
				m.closeIdleConnections()
			}
		})
	}
	return nil
}
func (m *Module) wrapRoute(permission, description string, handler app.HandlerFunc) app.HandlerFunc {
	wrapped := handler
	if m != nil && m.oplog != nil {
		wrapped = m.oplog.Wrap(hubOperationLogSpec(permission, description), wrapped)
	}
	return func(ctx context.Context, c *app.RequestContext) {
		if !securitycontext.IsLogin(c) {
			response.Error(c, apperrors.Unauthorized("未登录"))
			return
		}
		if !securitycontext.HasPermission(c, permission) {
			response.Error(c, apperrors.PermissionDenied(permission))
			return
		}
		wrapped(ctx, c)
	}
}

func hubOperationLogSpec(permission, description string) adminfacade.OperationLogSpec {
	spec := adminfacade.OperationLogSpec{Operation: adminfacade.OperationTypeOther, Description: description, IncludeParams: true, IncludeResult: true}
	remoteRead := map[string]bool{
		"system:hub-node:user:list": true, "system:hub-node:user:query": true,
		"system:hub-node:session:list": true, "system:hub-node:policy:query": true,
	}
	remoteMutation := map[string]bool{
		"system:hub-node:user:status": true, "system:hub-node:session:revoke": true,
		"system:hub-node:policy:apply": true, "system:hub-node:federation:apply": true,
	}
	if remoteRead[permission] || remoteMutation[permission] {
		spec.IncludeResult = false
		spec.OmitQuery = true
		spec.CompletionEnrichers = []adminfacade.OperationLogEnricher{hubAuditCompletionEnricher{}}
	}
	if remoteRead[permission] || permission == "system:hub-node:policy:apply" {
		spec.IncludeParams = false
	}
	return spec
}

type hubAuditCompletionEnricher struct{}

func (hubAuditCompletionEnricher) Enrich(_ context.Context, reqCtx *app.RequestContext, entry *adminfacade.OperationLogEntry) {
	if reqCtx == nil || entry == nil {
		return
	}
	summary := map[string]any{}
	entry.MethodName = hubAuditRouteTemplate(string(reqCtx.Path()))
	entry.RequestURL = entry.MethodName
	if nodeCode := safeNodeCodeFromPath(string(reqCtx.Path())); nodeCode != "" {
		summary["nodeCode"] = nodeCode
	}
	body := reqCtx.Response.Body()
	if len(body) > 0 && len(body) <= 64<<10 {
		var envelope struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		if json.Unmarshal(body, &envelope) == nil {
			summary["code"] = envelope.Code
			if traceID := xcontext.TraceID(reqCtx); xcontext.IsCanonicalTraceID(traceID) {
				summary["traceId"] = traceID
			}
			var counts struct {
				Total        json.RawMessage   `json:"total"`
				ChangedCount json.RawMessage   `json:"changedCount"`
				Records      []json.RawMessage `json:"records"`
			}
			if len(envelope.Data) > 0 && json.Unmarshal(envelope.Data, &counts) == nil {
				if value := boundedAuditNumber(counts.Total); value != "" {
					summary["total"] = value
				}
				if value := boundedAuditNumber(counts.ChangedCount); value != "" {
					summary["changedCount"] = value
				}
				if counts.Records != nil {
					summary["recordCount"] = len(counts.Records)
				}
			}
		}
	} else if len(body) > 0 {
		summary["responseOmitted"] = true
	}
	if encoded, err := json.Marshal(summary); err == nil {
		entry.ResponseResult = string(encoded)
	}
}

func hubAuditRouteTemplate(path string) string {
	segments := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	if len(segments) < 4 || strings.Join(segments[:3], "/") != "system/hub/nodes" {
		return "/system/hub/nodes"
	}
	result := []string{"system", "hub", "nodes", ":nodeCode"}
	for index, segment := range segments[4:] {
		if index > 0 && segments[4] == "users" && index == 1 {
			result = append(result, ":userId")
			continue
		}
		result = append(result, segment)
	}
	return "/" + strings.Join(result, "/")
}

func safeNodeCodeFromPath(path string) string {
	const prefix = "/system/hub/nodes/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	code := strings.Split(strings.TrimPrefix(path, prefix), "/")[0]
	if len(code) < 2 || len(code) > 64 {
		return ""
	}
	for _, value := range code {
		if !((value >= 'a' && value <= 'z') || (value >= '0' && value <= '9') || value == '.' || value == '_' || value == '-') {
			return ""
		}
	}
	return code
}

func boundedAuditNumber(raw json.RawMessage) string {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if value == "" || len(value) > 20 {
		return ""
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return ""
		}
	}
	return value
}

type managedSSOAdapter struct{ facade ssofacade.ManagedClientFacade }

func (a managedSSOAdapter) UpsertManagedClient(ctx context.Context, command hubfacade.ManagedSSOClientCommand) (*hubfacade.ManagedSSOClientResult, error) {
	result, err := a.facade.UpsertManagedClient(ctx, ssofacade.ManagedClientCommand{ClientID: command.ClientID, ClientName: command.ClientName, RedirectURI: command.RedirectURI, RotateSecret: command.RotateSecret, OwnerNodeCode: command.OwnerNodeCode})
	if err != nil {
		return nil, err
	}
	return &hubfacade.ManagedSSOClientResult{ClientID: result.ClientID, ClientSecret: result.ClientSecret}, nil
}
func (a managedSSOAdapter) SetManagedClientStatus(ctx context.Context, command hubfacade.ManagedSSOClientStatusCommand) error {
	return a.facade.SetManagedClientStatus(ctx, ssofacade.ManagedClientStatusCommand{ClientID: command.ClientID, OwnerNodeCode: command.OwnerNodeCode, Status: command.Status})
}

type secretAdapter struct{ service secretvalueinfra.Service }

func (a secretAdapter) Encrypt(ctx context.Context, plaintext string) (domain.EncryptedSecret, error) {
	value, err := a.service.EncryptString(ctx, plaintext)
	if err != nil {
		return domain.EncryptedSecret{}, err
	}
	return domain.EncryptedSecret{Ciphertext: value.CiphertextB64, EDEK: value.EDEKB64, WrapKeyRef: value.WrapKeyRef}, nil
}
func (a secretAdapter) Decrypt(ctx context.Context, value domain.EncryptedSecret) (string, error) {
	return a.service.DecryptString(ctx, secretvalueinfra.SecretValue{CiphertextB64: value.Ciphertext, EDEKB64: value.EDEK, WrapKeyRef: value.WrapKeyRef})
}

type repositoryAdapter struct{ repository *hubinfra.Repository }

func (a repositoryAdapter) Page(ctx context.Context, query domain.NodePageQuery) ([]domain.Node, int64, error) {
	records, total, err := a.repository.Page(ctx, hubinfra.NodePageQuery{Current: query.Current, Size: query.Size, Keyword: query.Keyword, Status: query.Status})
	if err != nil {
		return nil, 0, err
	}
	items := make([]domain.Node, 0, len(records))
	for _, record := range records {
		items = append(items, nodeFromRecord(record))
	}
	return items, total, nil
}
func (a repositoryAdapter) Find(ctx context.Context, nodeCode string) (*domain.Node, error) {
	record, err := a.repository.Find(ctx, nodeCode)
	if err != nil || record == nil {
		return nil, err
	}
	node := nodeFromRecord(*record)
	return &node, nil
}
func (a repositoryAdapter) FindForUpdate(ctx context.Context, nodeCode string) (*domain.Node, error) {
	record, err := a.repository.FindForUpdate(ctx, nodeCode)
	if err != nil || record == nil {
		return nil, err
	}
	node := nodeFromRecord(*record)
	return &node, nil
}
func (a repositoryAdapter) Insert(ctx context.Context, node *domain.Node) error {
	record := recordFromNode(*node)
	return a.repository.Insert(ctx, &record)
}
func (a repositoryAdapter) UpdateMetadata(ctx context.Context, node *domain.Node) error {
	record := recordFromNode(*node)
	return a.repository.UpdateMetadata(ctx, &record)
}
func (a repositoryAdapter) ReplaceManagementBearer(ctx context.Context, node *domain.Node) error {
	record := recordFromNode(*node)
	return a.repository.ReplaceManagementBearer(ctx, &record)
}
func (a repositoryAdapter) UpdateStatus(ctx context.Context, node *domain.Node) error {
	record := recordFromNode(*node)
	return a.repository.UpdateStatus(ctx, &record)
}
func (a repositoryAdapter) UpdateTargetState(ctx context.Context, node *domain.Node) error {
	record := recordFromNode(*node)
	return a.repository.UpdateTargetState(ctx, &record)
}
func (a repositoryAdapter) UpdateHealth(ctx context.Context, node *domain.Node) error {
	record := recordFromNode(*node)
	return a.repository.UpdateHealth(ctx, &record)
}
func (a repositoryAdapter) UpdateConnection(ctx context.Context, node *domain.Node) error {
	record := recordFromNode(*node)
	return a.repository.UpdateConnection(ctx, &record)
}
func (a repositoryAdapter) FindConnectionCommandForUpdate(ctx context.Context, nodeCode, version string) (*domain.ConnectionCommand, error) {
	record, err := a.repository.FindConnectionCommandForUpdate(ctx, nodeCode, version)
	if err != nil || record == nil {
		return nil, err
	}
	return &domain.ConnectionCommand{NodeCode: record.NodeCode, ConnectionVersion: record.ConnectionVersion, RequestHash: record.RequestHash, TargetRevision: record.TargetRevision, State: record.State, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}, nil
}
func (a repositoryAdapter) SaveConnectionCommand(ctx context.Context, command *domain.ConnectionCommand) error {
	return a.repository.SaveConnectionCommand(ctx, &hubinfra.ConnectionCommandRecord{NodeCode: command.NodeCode, ConnectionVersion: command.ConnectionVersion, RequestHash: command.RequestHash, TargetRevision: command.TargetRevision, State: command.State, CreatedAt: command.CreatedAt, UpdatedAt: command.UpdatedAt})
}

func nodeFromRecord(record hubinfra.NodeRecord) domain.Node {
	return domain.Node{ID: record.ID, NodeCode: record.NodeCode, NodeName: record.NodeName, Status: record.Status, DiscoveryType: record.DiscoveryType, ServiceName: record.ServiceName, ManagementBaseURL: record.ManagementBaseURL, HubIssuer: record.HubIssuer, OIDCClientID: record.OIDCClientID, OIDCClientSecret: domain.EncryptedSecret{Ciphertext: record.OIDCClientSecret.Ciphertext, EDEK: record.OIDCClientSecret.EDEK, WrapKeyRef: record.OIDCClientSecret.WrapKeyRef}, ManagementBearer: domain.EncryptedSecret{Ciphertext: record.ManagementBearer.Ciphertext, EDEK: record.ManagementBearer.EDEK, WrapKeyRef: record.ManagementBearer.WrapKeyRef}, CapabilitiesJSON: record.CapabilitiesJSON, ConnectionStatus: record.ConnectionStatus, ConnectionVersion: record.ConnectionVersion, ConnectionRequestHash: record.ConnectionRequestHash, TargetRevision: record.TargetRevision, IssuerLockedAt: record.IssuerLockedAt, LastConnectionError: record.LastConnectionError, LastConnectionTraceID: record.LastConnectionTraceID, LastHealthyAt: record.LastHealthyAt, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
}
func recordFromNode(node domain.Node) hubinfra.NodeRecord {
	return hubinfra.NodeRecord{ID: node.ID, NodeCode: node.NodeCode, NodeName: node.NodeName, Status: node.Status, DiscoveryType: node.DiscoveryType, ServiceName: node.ServiceName, ManagementBaseURL: node.ManagementBaseURL, HubIssuer: node.HubIssuer, OIDCClientID: node.OIDCClientID, OIDCClientSecret: hubinfra.EncryptedValue{Ciphertext: node.OIDCClientSecret.Ciphertext, EDEK: node.OIDCClientSecret.EDEK, WrapKeyRef: node.OIDCClientSecret.WrapKeyRef}, ManagementBearer: hubinfra.EncryptedValue{Ciphertext: node.ManagementBearer.Ciphertext, EDEK: node.ManagementBearer.EDEK, WrapKeyRef: node.ManagementBearer.WrapKeyRef}, CapabilitiesJSON: node.CapabilitiesJSON, ConnectionStatus: node.ConnectionStatus, ConnectionVersion: node.ConnectionVersion, ConnectionRequestHash: node.ConnectionRequestHash, TargetRevision: node.TargetRevision, IssuerLockedAt: node.IssuerLockedAt, LastConnectionError: node.LastConnectionError, LastConnectionTraceID: node.LastConnectionTraceID, LastHealthyAt: node.LastHealthyAt, CreatedAt: node.CreatedAt, UpdatedAt: node.UpdatedAt}
}

type nodeClientAdapter struct{ client *hubinfra.NodeClient }

type userPageWire struct {
	Current hubinfra.DecimalInt64    `json:"current"`
	Size    hubinfra.DecimalInt64    `json:"size"`
	Total   hubinfra.DecimalInt64    `json:"total"`
	Records []nodefacade.UserSummary `json:"records"`
}
type sessionPageWire struct {
	Current hubinfra.DecimalInt64       `json:"current"`
	Size    hubinfra.DecimalInt64       `json:"size"`
	Total   hubinfra.DecimalInt64       `json:"total"`
	Records []nodefacade.SessionSummary `json:"records"`
}

func targetFromNode(node domain.Node) hubinfra.NodeTarget {
	return hubinfra.NodeTarget{NodeCode: node.NodeCode, DiscoveryType: node.DiscoveryType, ServiceName: node.ServiceName, ManagementBaseURL: node.ManagementBaseURL, ManagementBearer: hubinfra.EncryptedValue{Ciphertext: node.ManagementBearer.Ciphertext, EDEK: node.ManagementBearer.EDEK, WrapKeyRef: node.ManagementBearer.WrapKeyRef}}
}
func (a nodeClientAdapter) Describe(ctx context.Context, node domain.Node) (*nodefacade.NodeDescriptor, error) {
	var out nodefacade.NodeDescriptor
	return &out, a.client.Do(ctx, targetFromNode(node), http.MethodGet, "/internal/node/v1/descriptor", nil, nil, "", &out)
}
func (a nodeClientAdapter) ListUsers(ctx context.Context, node domain.Node, query nodefacade.UserPageQuery) (*nodefacade.UserPage, error) {
	values := url.Values{"current": {strconv.FormatInt(query.Current, 10)}, "size": {strconv.FormatInt(query.Size, 10)}}
	if query.Keyword != "" {
		values.Set("keyword", query.Keyword)
	}
	if query.Status != nil {
		values.Set("status", strconv.Itoa(*query.Status))
	}
	var wire userPageWire
	if err := a.client.Do(ctx, targetFromNode(node), http.MethodGet, "/internal/node/v1/users", values, nil, "", &wire); err != nil {
		return nil, err
	}
	return &nodefacade.UserPage{Current: int64(wire.Current), Size: int64(wire.Size), Total: int64(wire.Total), Records: wire.Records}, nil
}
func (a nodeClientAdapter) GetUser(ctx context.Context, node domain.Node, userID string) (*nodefacade.UserDetail, error) {
	var out nodefacade.UserDetail
	return &out, a.client.Do(ctx, targetFromNode(node), http.MethodGet, "/internal/node/v1/users/"+url.PathEscape(userID), nil, nil, "", &out)
}
func (a nodeClientAdapter) SetUserStatus(ctx context.Context, node domain.Node, command nodefacade.SetUserStatusCommand) error {
	body := struct {
		Status int    `json:"status"`
		Reason string `json:"reason"`
	}{command.Status, command.Reason}
	return a.client.Do(ctx, targetFromNode(node), http.MethodPut, "/internal/node/v1/users/"+url.PathEscape(command.UserID)+"/status", nil, body, command.IdempotencyKey, nil)
}
func (a nodeClientAdapter) ListUserSessions(ctx context.Context, node domain.Node, userID string, query nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error) {
	values := url.Values{"current": {strconv.FormatInt(query.Current, 10)}, "size": {strconv.FormatInt(query.Size, 10)}}
	var wire sessionPageWire
	if err := a.client.Do(ctx, targetFromNode(node), http.MethodGet, "/internal/node/v1/users/"+url.PathEscape(userID)+"/sessions", values, nil, "", &wire); err != nil {
		return nil, err
	}
	return &nodefacade.SessionPage{Current: int64(wire.Current), Size: int64(wire.Size), Total: int64(wire.Total), Records: wire.Records}, nil
}
func (a nodeClientAdapter) RevokeUserSessions(ctx context.Context, node domain.Node, command nodefacade.RevokeUserSessionsCommand) error {
	body := struct {
		All         bool     `json:"all"`
		SessionRefs []string `json:"sessionRefs,omitempty"`
		Reason      string   `json:"reason"`
	}{command.All, command.SessionRefs, command.Reason}
	return a.client.Do(ctx, targetFromNode(node), http.MethodPost, "/internal/node/v1/users/"+url.PathEscape(command.UserID)+"/sessions/revoke", nil, body, command.IdempotencyKey, nil)
}
func (a nodeClientAdapter) GetLoginPolicy(ctx context.Context, node domain.Node) (*nodefacade.ManagedLoginPolicy, error) {
	var out nodefacade.ManagedLoginPolicy
	return &out, a.client.Do(ctx, targetFromNode(node), http.MethodGet, "/internal/node/v1/login-policy", nil, nil, "", &out)
}
func (a nodeClientAdapter) ApplyLoginPolicy(ctx context.Context, node domain.Node, command nodefacade.ApplyLoginPolicyCommand) error {
	return a.client.Do(ctx, targetFromNode(node), http.MethodPost, "/internal/node/v1/login-policy/apply", nil, command, command.IdempotencyKey, nil)
}
func (a nodeClientAdapter) ApplyHubConnection(ctx context.Context, node domain.Node, command nodefacade.ApplyHubConnectionCommand) error {
	return a.client.Do(ctx, targetFromNode(node), http.MethodPut, "/internal/node/v1/hub-connection", nil, command, command.IdempotencyKey, nil)
}
