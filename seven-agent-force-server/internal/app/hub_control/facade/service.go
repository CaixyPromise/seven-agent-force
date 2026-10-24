package facade

import (
	"context"

	nodefacade "github.com/CaixyPromise/seven-agent/force/internal/app/node_management/facade"
)

// NodeAdminFacade is the complete Hub Node control-plane contract.
type NodeAdminFacade interface {
	PageNodes(context.Context, NodePageQuery) (*NodePage, error)
	GetNode(context.Context, string) (*NodeDetail, error)
	SaveNode(context.Context, SaveNodeCommand) (*NodeDetail, error)
	CopyNode(context.Context, string, CopyNodeCommand) (*NodeDetail, error)
	SetNodeStatus(context.Context, SetNodeStatusCommand) error
	TestConnection(context.Context, string) (*NodeHealth, error)
	ListNodeUsers(context.Context, string, nodefacade.UserPageQuery) (*nodefacade.UserPage, error)
	GetNodeUser(context.Context, string, string) (*nodefacade.UserDetail, error)
	SetNodeUserStatus(context.Context, NodeUserStatusCommand) error
	ListNodeUserSessions(context.Context, string, string, nodefacade.SessionPageQuery) (*nodefacade.SessionPage, error)
	RevokeNodeUserSessions(context.Context, RevokeNodeSessionsCommand) error
	GetNodeLoginPolicy(context.Context, string) (*nodefacade.ManagedLoginPolicy, error)
	ApplyNodeLoginPolicy(context.Context, string, nodefacade.ApplyLoginPolicyCommand) error
	GetFederationStatus(context.Context, string) (*FederationStatus, error)
	ProvisionNodeConnection(context.Context, ProvisionConnectionCommand) error
}

// ManagedSSOClientFacade is the only cross-module SSO dependency used by Hub control.
type ManagedSSOClientFacade interface {
	UpsertManagedClient(context.Context, ManagedSSOClientCommand) (*ManagedSSOClientResult, error)
	SetManagedClientStatus(context.Context, ManagedSSOClientStatusCommand) error
}
