package facade

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
)

// Stable aliases keep other Workforce modules on this facade boundary rather
// than importing tasknotification's private domain package directly.
type TaskNotificationTransition = domain.TaskNotificationTransition
type TransitionDisposition = domain.TransitionDisposition
type SubscriptionCommand = domain.SubscriptionCommand
type FactType = domain.FactType
type SourceType = domain.SourceType
type Focus = domain.Focus
type ChannelType = domain.ChannelType
type ChannelSelection = domain.ChannelSelection

const (
	FactActionRequired            = domain.FactActionRequired
	FactInterrupted               = domain.FactInterrupted
	FactCompleted                 = domain.FactCompleted
	FactDeliveryCompleted         = domain.FactDeliveryCompleted
	SourceContinuationCheckpoint  = domain.SourceContinuationCheckpoint
	SourceFinalizationRun         = domain.SourceFinalizationRun
	SourceArtifactUpload          = domain.SourceArtifactUpload
	SourceResultEvaluation        = domain.SourceResultEvaluation
	SourceTaskAttempt             = domain.SourceTaskAttempt
	SourceRuntimeRecovery         = domain.SourceRuntimeRecovery
	SourceTaskTimeout             = domain.SourceTaskTimeout
	SourceTaskCancel              = domain.SourceTaskCancel
	SourceDeliveryRequest         = domain.SourceDeliveryRequest
	SourceReviewOperation         = domain.SourceReviewOperation
	SourceIndependentReviewRun    = domain.SourceIndependentReviewRun
	FocusEvaluation               = domain.FocusEvaluation
	FocusResult                   = domain.FocusResult
	RecoverabilityAutoRecovering  = domain.RecoverabilityAutoRecovering
	RecoverabilityUserRecoverable = domain.RecoverabilityUserRecoverable
	RecoverabilityTerminal        = domain.RecoverabilityTerminal
	ChannelInApp                  = domain.ChannelInApp
	ChannelEmailUser              = domain.ChannelEmailUser
	ChannelFeishuDirect           = domain.ChannelFeishuDirect
	ChannelWeComDirect            = domain.ChannelWeComDirect
	ChannelGroupRoute             = domain.ChannelGroupRoute
)

func DefaultSubscriptionCommand() SubscriptionCommand {
	return domain.DefaultSubscriptionCommand()
}

// TransactionalRecorder persists a Task notification fact and its Workforce
// outbox intent inside the caller's authoritative state-transition transaction.
type TransactionalRecorder interface {
	RecordTransition(context.Context, pgx.Tx, domain.TaskNotificationTransition) (domain.TransitionDisposition, error)
}

// SubscriptionWriter creates the creator-owned subscription in the same
// transaction as a newly-created Task.
type SubscriptionWriter interface {
	CreateSubscription(context.Context, pgx.Tx, int64, int64, int64, domain.SubscriptionCommand) (domain.Subscription, bool, error)
}

// PublicWebURLResolver is frozen for later external-card delivery. Gate 1
// in-app delivery does not require or invoke it.
type PublicWebURLResolver interface {
	ResolveInternalLink(deepLink string) (absoluteURL string, available bool, reasonCode string)
}
