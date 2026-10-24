package facade

import "context"

// QueryFacade exposes safe Node management reads.
type QueryFacade interface {
	Describe(ctx context.Context) (*NodeDescriptor, error)
	ListUsers(ctx context.Context, query UserPageQuery) (*UserPage, error)
	GetUser(ctx context.Context, userID int64) (*UserDetail, error)
	ListUserSessions(ctx context.Context, userID int64, query SessionPageQuery) (*SessionPage, error)
	GetLoginPolicy(ctx context.Context) (*ManagedLoginPolicy, error)
}

// CommandFacade exposes value-idempotent Node management writes.
type CommandFacade interface {
	SetUserStatus(ctx context.Context, command SetUserStatusCommand) (*CommandResult, error)
	RevokeUserSessions(ctx context.Context, command RevokeUserSessionsCommand) (*RevokeResult, error)
	ApplyLoginPolicy(ctx context.Context, command ApplyLoginPolicyCommand) (*CommandResult, error)
	ApplyHubConnection(ctx context.Context, command ApplyHubConnectionCommand) (*CommandResult, error)
}

// NodeManagementFacade is the complete versioned Node management contract.
type NodeManagementFacade interface {
	QueryFacade
	CommandFacade
}
