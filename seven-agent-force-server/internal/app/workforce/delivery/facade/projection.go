package facade

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Projection is the allowlisted Delivery card read model. It contains no
// credential, remote URL, Provider body, Worker identity, lease, or permit.
type Projection struct {
	ID, IntentID, RequestID, Version, IntentVersion, RequestVersion, TenantID, TaskID, TaskTurnID, AttemptID int64
	Status, Summary, DeliveryMode, Authorization, RepositoryName                                             string
	DestinationBranch, ResultCommit, DeliveryCommit, PullRequestURL                                          string
	ReasonCode                                                                                               string
	RemoteReadbackVerified, AttentionRequired                                                                bool
	AllowedActions                                                                                           []string
	OccurredAt                                                                                               time.Time
}

type ConversationProjector interface {
	ProjectDeliveryTx(context.Context, pgx.Tx, Projection) error
}
