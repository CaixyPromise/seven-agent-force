package domain

import "github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"

const (
	OutboxOwner = cachepolicy.CacheGovernanceOutboxOwner
	EventType   = cachepolicy.CacheInvalidationEventType
	ScopeID     = cachepolicy.StorageScopeSystemGlobal

	SchemaVersion = cachepolicy.SchemaVersionV1
)

var (
	ErrInvalidationEvent = cachepolicy.ErrInvalidationEnvelope
	ErrFanoutUnavailable = cachepolicy.ErrFanoutUnavailable
)

// The domain keeps the business intent to invalidate a classified read model.
// The concrete wire/adapter shapes are aliases of the shared protocol so
// infrastructure can implement them without a reverse domain dependency.
type InvalidationEvent = cachepolicy.InvalidationEnvelope
type OutboxEvent = cachepolicy.OutboxEvent
type Lease = cachepolicy.Lease
type OutboxPort = cachepolicy.OutboxPort
type GenerationPort = cachepolicy.GenerationPort
type FanoutPort = cachepolicy.FanoutPort

func NewInvalidationEvent(eventID string, dataClass cachepolicy.DataClass) (InvalidationEvent, error) {
	return cachepolicy.NewInvalidationEnvelope(eventID, dataClass)
}

func DecodeInvalidationEvent(payload []byte) (InvalidationEvent, error) {
	return cachepolicy.DecodeInvalidationEnvelope(payload)
}
