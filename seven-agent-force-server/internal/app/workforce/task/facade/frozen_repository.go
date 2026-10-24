package facade

import "context"

// FrozenRepositoryPlan is the immutable Task-owned repository identity used
// by Delivery. Delivery receives this narrow projection and never reads the
// Task module's private persistence tables.
type FrozenRepositoryPlan struct {
	ProjectID, ProjectVersionID    int64
	ProviderType, Scheme, Host     string
	Port                           int
	RepositoryPath, DisplayURL     string
	WebURL, RepositoryConfigDigest string
	CredentialCiphertext           string
	CredentialEDEK                 string
	CredentialWrapKeyRef           string
	CredentialVersion              int64
	BaseBranch, BaseCommit         string
	ResultBranch, DeliveryMode     string
	DeliveryAuthorization          string
	AuthorizedPrincipalID          int64
	PreviousDeliveryCommit         string
	PlanDigest, TargetConfigDigest string
	PushAuthorizationDigest        string
	ReviewAuthorizationDigest      string
}

type FrozenRepositoryPlanReader interface {
	FrozenRepositoryPlan(context.Context, int64, int64, int64) (FrozenRepositoryPlan, error)
}
