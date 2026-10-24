package node_management

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	externalfacade "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	nodeapp "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/application"
	nodedomain "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/domain"
	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
	nodehandler "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/handler"
	nodeinfra "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/infrastructure"
	platformfacade "github.com/CaixyPromise/seven-agent/force/internal/app/platform/facade"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	adminfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/admin/facade"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	bootstrapruntime "github.com/CaixyPromise/seven-agent/force/internal/bootstrap/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/core"
	"github.com/cloudwego/hertz/pkg/route"
)

// Dependencies contains existing cross-module facade contracts.
type Dependencies struct {
	Users         userfacade.AdminUserFacade
	ManagedUsers  userfacade.ManagedUserStatusFacade
	Sessions      ssofacade.ManagedSessionFacade
	Policies      platformfacade.ManagedLoginPolicyFacade
	Audit         adminfacade.OperationLogFacade
	HubConnection nodefacade.HubConnectionPort
}

// Module is the node-only internal route mounter.
type Module struct {
	service  *nodeapp.Service
	handler  *nodehandler.Handler
	nodeCode string
}

// Install builds the real Node management service and fails closed on gaps.
func Install(deps bootstrapruntime.ModuleDeps, references Dependencies) (*Module, error) {
	if strings.TrimSpace(deps.Config.Platform.Node.Code) == "" {
		return nil, fmt.Errorf("node management requires platform.node.code")
	}
	if strings.TrimSpace(deps.Config.Platform.Node.ManagementBearer) == "" {
		return nil, fmt.Errorf("node management requires platform.node.managementBearer")
	}
	if deps.Infra.CacheMgr == nil || references.Users == nil || references.ManagedUsers == nil || references.Sessions == nil || references.Policies == nil || references.Audit == nil {
		return nil, fmt.Errorf("node management requires cache, user, session, policy, and audit facades")
	}
	sessionRefs, err := nodeapp.NewSessionReferenceCodec(deps.Config.Platform.Node.Code, deps.Config.Platform.Node.ManagementBearer)
	if err != nil {
		return nil, fmt.Errorf("build node session reference codec: %w", err)
	}
	version := strings.TrimSpace(deps.Config.Microservice.Service.Metadata["version"])
	if version == "" {
		version = "1.0.0"
	}
	callerHash := sha256.Sum256([]byte(deps.Config.Platform.Node.ManagementBearer))
	service := nodeapp.NewService(nodeapp.Config{NodeCode: deps.Config.Platform.Node.Code, Version: version, CallerIDHash: hex.EncodeToString(callerHash[:])}, nodeapp.Dependencies{
		Users: references.Users, ManagedUsers: references.ManagedUsers, Sessions: references.Sessions, Policies: references.Policies,
		HubConnection: references.HubConnection, Replay: nodedomain.NewCommandCoordinator(nodeinfra.NewCommandStore(deps.Infra.CacheMgr)), Audit: references.Audit, SessionRefs: sessionRefs,
	})
	return &Module{service: service, handler: nodehandler.New(service, service, deps.Config.Platform.Node.ManagementBearer), nodeCode: deps.Config.Platform.Node.Code}, nil
}

func (m *Module) BindManagedOIDCProvider(provider externalfacade.ManagedOIDCProviderFacade) {
	if m == nil || provider == nil {
		return
	}
	m.BindHubConnection(newManagedOIDCPort(m.nodeCode, provider))
}

type managedOIDCPort struct {
	ownerNodeCode string
	providers     externalfacade.ManagedOIDCProviderFacade
}

func newManagedOIDCPort(ownerNodeCode string, providers externalfacade.ManagedOIDCProviderFacade) nodefacade.HubConnectionPort {
	return &managedOIDCPort{ownerNodeCode: strings.TrimSpace(ownerNodeCode), providers: providers}
}

func (p *managedOIDCPort) ApplyHubConnection(ctx context.Context, command nodefacade.ManagedHubConnectionCommand) error {
	if !command.Enabled {
		return p.providers.DisableManagedOIDCProvider(ctx, p.ownerNodeCode, command.ConnectionVersion, command.TargetRevision)
	}
	return p.providers.ApplyManagedOIDCProvider(ctx, externalfacade.ManagedOIDCProviderCommand{
		OwnerNodeCode: p.ownerNodeCode, ConnectionVersion: command.ConnectionVersion, TargetRevision: command.TargetRevision, Enabled: true,
		DisplayName: command.DisplayName, Issuer: command.Issuer, ClientID: command.ClientID,
		ClientSecret: command.ClientSecret, RedirectURI: command.RedirectURI,
	})
}

func (m *Module) Descriptor() core.ModuleDescriptor {
	return core.ModuleDescriptor{Name: "node-management", Prefix: "/internal/node/v1"}
}
func (m *Module) Mount(route.IRouter) {}
func (m *Module) MountInternal(router route.IRouter) {
	if m != nil && m.handler != nil {
		m.handler.Mount(router)
	}
}

// BindHubConnection binds the Task 7 managed OIDC port after installation.
func (m *Module) BindHubConnection(port nodefacade.HubConnectionPort) {
	if m != nil && m.service != nil {
		m.service.BindHubConnection(port)
	}
}

var _ bootstrapruntime.InternalRouteMounter = (*Module)(nil)
