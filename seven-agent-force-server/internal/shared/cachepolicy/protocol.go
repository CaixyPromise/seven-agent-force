package cachepolicy

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/bytedance/sonic"
)

const (
	// CacheGovernanceOutboxOwner and CacheInvalidationEventType are transport
	// allowlists for the shared sys_outbox_event store. They intentionally live
	// beside the envelope so adapters need not import a DG5 domain package.
	CacheGovernanceOutboxOwner = "cache-governance"
	CacheInvalidationEventType = "CACHE_INVALIDATE_V1"
	// TargetedCacheInvalidationEventType is an intentionally separate strict
	// DG6.2 protocol. V1 accepts only class digests and must stay unchanged.
	TargetedCacheInvalidationEventType = "CACHE_INVALIDATE_V2"
	// CacheRefreshEventType is the only global application-owned governed-cache
	// refresh message. It carries no class, target, cache key, identity, or
	// business value, so neither V1 nor V2 can reinterpret it.
	CacheRefreshEventType      = "CACHE_REFRESH_V3"
	CacheInvalidationAggregate = "cache-invalidation"
	CacheRefreshAggregate      = "cache-refresh"
	CacheRefreshAggregateID    = "system:global"
	CacheRefreshOperation      = "SYSTEM_CACHE_REFRESH"

	// Fanout rejection categories are deliberately fixed strings. A diagnostic
	// must never preserve a decoder error, raw broker body, cache key, target,
	// identity, or configuration value.
	FanoutRejectionInvalidEnvelope = "invalid-envelope"
	FanoutRejectionInvalidDelivery = "invalid-delivery"

	// MaxInvalidationEnvelopeBytes is the shared durable-and-broker boundary
	// for a DG5 v1 invalidation envelope. It is checked before Sonic decodes
	// an untrusted payload, so a malformed outbox row cannot turn a bounded
	// cache protocol into an unbounded allocation or decode path.
	MaxInvalidationEnvelopeBytes = 1024

	maxInvalidationEventIDBytes = 128
)

var (
	ErrInvalidationEnvelope = errors.New("cache invalidation envelope is invalid")
	ErrFanoutUnavailable    = errors.New("cache invalidation fanout is unavailable")
)

// strictInvalidationJSON is intentionally separate from the ordinary
// RabbitMQ JSON client. The DG5 durable/fanout envelope is an allowlist
// protocol and must reject unknown or trailing data before it gains cache
// eviction authority. Sonic is deliberately confined to this DG5 boundary.
var strictInvalidationJSON = sonic.Config{
	CaseSensitive:         true,
	CopyString:            true,
	DisallowUnknownFields: true,
	ValidateString:        true,
}.Froze()

// InvalidationEnvelope is a content-free DG5 transport contract. It never
// carries cached values, configuration values, raw keys, identities, or
// authorization data.
type InvalidationEnvelope struct {
	SchemaVersion int       `json:"schemaVersion"`
	EventID       string    `json:"eventId"`
	ScopeID       string    `json:"scopeId"`
	DataClass     DataClass `json:"dataClass"`
	TargetDigest  string    `json:"targetDigest"`
}

// TargetedInvalidationEnvelope carries no session ID or cached value. Its
// opaque target digest is meaningful only with the fixed data class and kind.
type TargetedInvalidationEnvelope struct {
	SchemaVersion int       `json:"schemaVersion"`
	EventID       string    `json:"eventId"`
	ScopeID       string    `json:"scopeId"`
	DataClass     DataClass `json:"dataClass"`
	TargetKind    string    `json:"targetKind"`
	TargetDigest  string    `json:"targetDigest"`
}

// CacheRefreshEnvelope is the strict DG6.3 global refresh protocol. Its
// fixed operation is intentionally the entire payload contract: a global
// request must never smuggle a physical key, target, scope variant, cached
// value, identity, or other application data into sys_outbox_event or AMQP.
type CacheRefreshEnvelope struct {
	SchemaVersion int    `json:"schemaVersion"`
	EventID       string `json:"eventId"`
	ScopeID       string `json:"scopeId"`
	Operation     string `json:"operation"`
}

func NewCacheRefreshEnvelope(eventID string) (CacheRefreshEnvelope, error) {
	event := CacheRefreshEnvelope{SchemaVersion: SchemaVersionV3, EventID: strings.TrimSpace(eventID), ScopeID: StorageScopeSystemGlobal, Operation: CacheRefreshOperation}
	if err := event.Validate(); err != nil {
		return CacheRefreshEnvelope{}, err
	}
	return event, nil
}

func (e CacheRefreshEnvelope) Validate() error {
	if e.SchemaVersion != SchemaVersionV3 || strings.TrimSpace(e.EventID) == "" || len(strings.TrimSpace(e.EventID)) > maxInvalidationEventIDBytes || strings.TrimSpace(e.ScopeID) != StorageScopeSystemGlobal || strings.TrimSpace(e.Operation) != CacheRefreshOperation {
		return ErrInvalidationEnvelope
	}
	return nil
}

// DecodeCacheRefreshEnvelope is intentionally separate from V1/V2 decoders.
// strictInvalidationJSON rejects their different fields and any future
// extension until it has its own reviewed decoder.
func DecodeCacheRefreshEnvelope(payload []byte) (CacheRefreshEnvelope, error) {
	if len(payload) == 0 || len(payload) > MaxInvalidationEnvelopeBytes {
		return CacheRefreshEnvelope{}, ErrInvalidationEnvelope
	}
	decoder := strictInvalidationJSON.NewDecoder(bytes.NewReader(payload))
	var event CacheRefreshEnvelope
	if err := decoder.Decode(&event); err != nil {
		return CacheRefreshEnvelope{}, ErrInvalidationEnvelope
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return CacheRefreshEnvelope{}, ErrInvalidationEnvelope
	}
	if err := event.Validate(); err != nil {
		return CacheRefreshEnvelope{}, ErrInvalidationEnvelope
	}
	return event, nil
}

func NewTargetedInvalidationEnvelope(eventID, targetDigest string) (TargetedInvalidationEnvelope, error) {
	event := TargetedInvalidationEnvelope{SchemaVersion: SchemaVersionV2, EventID: strings.TrimSpace(eventID), ScopeID: StorageScopeSystemGlobal, DataClass: DataClassActiveSessionValidity, TargetKind: "active-session", TargetDigest: strings.TrimSpace(targetDigest)}
	if err := event.Validate(); err != nil {
		return TargetedInvalidationEnvelope{}, err
	}
	return event, nil
}

func (e TargetedInvalidationEnvelope) Validate() error {
	if e.SchemaVersion != SchemaVersionV2 || strings.TrimSpace(e.EventID) == "" || len(strings.TrimSpace(e.EventID)) > maxInvalidationEventIDBytes || strings.TrimSpace(e.ScopeID) != StorageScopeSystemGlobal || e.DataClass != DataClassActiveSessionValidity || strings.TrimSpace(e.TargetKind) != "active-session" || !isDigest(e.TargetDigest) {
		return ErrInvalidationEnvelope
	}
	return nil
}

func DecodeTargetedInvalidationEnvelope(payload []byte) (TargetedInvalidationEnvelope, error) {
	if len(payload) == 0 || len(payload) > MaxInvalidationEnvelopeBytes {
		return TargetedInvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	decoder := strictInvalidationJSON.NewDecoder(bytes.NewReader(payload))
	var event TargetedInvalidationEnvelope
	if err := decoder.Decode(&event); err != nil {
		return TargetedInvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return TargetedInvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	if err := event.Validate(); err != nil {
		return TargetedInvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	return event, nil
}

func NewInvalidationEnvelope(eventID string, dataClass DataClass) (InvalidationEnvelope, error) {
	event := InvalidationEnvelope{
		SchemaVersion: SchemaVersionV1,
		EventID:       strings.TrimSpace(eventID),
		ScopeID:       StorageScopeSystemGlobal,
		DataClass:     dataClass,
		TargetDigest:  ClassTargetDigest(dataClass),
	}
	if err := event.Validate(); err != nil {
		return InvalidationEnvelope{}, err
	}
	return event, nil
}

func (e InvalidationEnvelope) Validate() error {
	eventID := strings.TrimSpace(e.EventID)
	if e.SchemaVersion != SchemaVersionV1 || eventID == "" || len(eventID) > maxInvalidationEventIDBytes || strings.TrimSpace(e.ScopeID) != StorageScopeSystemGlobal || strings.TrimSpace(e.TargetDigest) == "" {
		return ErrInvalidationEnvelope
	}
	if !isV1InvalidationClass(e.DataClass) {
		return ErrInvalidationEnvelope
	}
	if e.TargetDigest != ClassTargetDigest(e.DataClass) {
		return ErrInvalidationEnvelope
	}
	return nil
}

func isV1InvalidationClass(class DataClass) bool {
	switch class {
	case DataClassConfigPublicScalar, DataClassDictPublicItems, DataClassAuthorizationContext, DataClassAuthorizationMenus:
		return true
	default:
		return false
	}
}

// DecodeInvalidationEnvelope accepts one exact Sonic-encoded DG5 envelope.
// It returns only a sentinel error so a hostile payload cannot leak to logs.
func DecodeInvalidationEnvelope(payload []byte) (InvalidationEnvelope, error) {
	if len(payload) == 0 || len(payload) > MaxInvalidationEnvelopeBytes {
		return InvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	decoder := strictInvalidationJSON.NewDecoder(bytes.NewReader(payload))
	var event InvalidationEnvelope
	if err := decoder.Decode(&event); err != nil {
		return InvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return InvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	if err := event.Validate(); err != nil {
		return InvalidationEnvelope{}, ErrInvalidationEnvelope
	}
	return event, nil
}

// FanoutRejectionDiagnostic is the only message that may enter DG5's
// terminal diagnostic queue. It is constructed locally after a strict
// rejection and intentionally contains no original AMQP body or identifier.
type FanoutRejectionDiagnostic struct {
	SchemaVersion  int    `json:"schemaVersion"`
	EventID        string `json:"eventId"`
	ScopeID        string `json:"scopeId"`
	InstanceDigest string `json:"instanceDigest"`
	Category       string `json:"category"`
}

func NewFanoutRejectionDiagnostic(eventID, instanceIdentity, category string) (FanoutRejectionDiagnostic, error) {
	diagnostic := FanoutRejectionDiagnostic{
		SchemaVersion:  SchemaVersionV1,
		EventID:        strings.TrimSpace(eventID),
		ScopeID:        StorageScopeSystemGlobal,
		InstanceDigest: EventDigest(instanceIdentity),
		Category:       strings.TrimSpace(category),
	}
	if err := diagnostic.Validate(); err != nil {
		return FanoutRejectionDiagnostic{}, err
	}
	return diagnostic, nil
}

func (d FanoutRejectionDiagnostic) Validate() error {
	if d.SchemaVersion != SchemaVersionV1 || strings.TrimSpace(d.EventID) == "" || len(strings.TrimSpace(d.EventID)) > maxInvalidationEventIDBytes ||
		strings.TrimSpace(d.ScopeID) != StorageScopeSystemGlobal || strings.TrimSpace(d.InstanceDigest) == "" {
		return ErrInvalidationEnvelope
	}
	switch d.Category {
	case FanoutRejectionInvalidEnvelope, FanoutRejectionInvalidDelivery:
		return nil
	default:
		return ErrInvalidationEnvelope
	}
}

// OutboxEvent and Lease are shared adapter-facing records. Application rules
// decide when they are created; the infrastructure owns their persistence.
type OutboxEvent struct {
	ID            int64
	EventID       string
	EventOwner    string
	ScopeID       string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       string
	// PayloadOversized is a content-free signal from the bounded shared outbox
	// listing path. A relay must mark it terminally invalid without attempting
	// to decode the omitted original body.
	PayloadOversized bool
	RetryCount       int
}

type Lease struct {
	Token string
	Until time.Time
}

// OutboxPort is the narrow durable protocol used by DG5 application code.
type OutboxPort interface {
	Append(ctx context.Context, event InvalidationEnvelope) error
	ListReady(ctx context.Context, limit int) ([]OutboxEvent, error)
	ListUnknown(ctx context.Context, limit int) ([]OutboxEvent, error)
	Claim(ctx context.Context, id int64, eventType, worker string) (*Lease, bool, error)
	Mark(ctx context.Context, id int64, eventType, leaseToken, status, reason string, retryCount int, nextRetryAt *time.Time) (bool, error)
}

// GenerationPort protects cache-key reuse and local writer state without
// exposing a business value to the relay or broker adapter.
type GenerationPort interface {
	Advance(ctx context.Context, eventID string, dataClass DataClass) (bool, error)
	MarkWriterDirty(eventID string, dataClass DataClass)
	EvictAndResolve(eventID string, dataClass DataClass)
	SetFanoutHealthy(healthy bool)
	RecordRejectedFanout()
}

// FanoutPort publishes only strict DG5 envelopes after a known confirmation.
type FanoutPort interface {
	Enabled() bool
	Publish(ctx context.Context, event InvalidationEnvelope) error
}

// FreshnessLease serializes a classified cache candidate with a mutation of
// the same data class. Trusted is false when a durable invalidation has not
// completed; callers must then bypass L1/L2 and use the authority.
type FreshnessLease interface {
	Trusted() bool
	Release()
}

// FreshnessGate is a cross-instance, source-adjacent fence. It is distinct
// from Redis generation: the read lease prevents a post-commit cache race
// before the asynchronous relay can advance Redis or publish fanout.
type FreshnessGate interface {
	AcquireRead(ctx context.Context, dataClass DataClass) (FreshnessLease, error)
	AcquireMutation(ctx context.Context, dataClass DataClass) (FreshnessLease, error)
}

// Targeted* ports deliberately do not widen V1. They preserve V1's
// class-wide compatibility while giving sessions target-specific freshness.
type TargetedOutboxPort interface {
	AppendTargeted(ctx context.Context, event TargetedInvalidationEnvelope) error
	ListTargetedReady(ctx context.Context, limit int) ([]OutboxEvent, error)
	ListTargetedUnknown(ctx context.Context, limit int) ([]OutboxEvent, error)
	Claim(ctx context.Context, id int64, eventType, worker string) (*Lease, bool, error)
	Mark(ctx context.Context, id int64, eventType, leaseToken, status, reason string, retryCount int, nextRetryAt *time.Time) (bool, error)
}

type TargetedGenerationPort interface {
	AdvanceTarget(ctx context.Context, eventID string, request TargetedReadRequest) (bool, error)
	MarkTargetDirty(eventID string, request TargetedReadRequest)
	EvictTarget(eventID string, request TargetedReadRequest)
	SetTargetFanoutHealthy(healthy bool)
}

type TargetedFanoutPort interface {
	Enabled() bool
	PublishTargeted(ctx context.Context, event TargetedInvalidationEnvelope) error
}

type TargetedFreshnessGate interface {
	AcquireTargetedRead(ctx context.Context, dataClass DataClass, targetKind, targetDigest string) (FreshnessLease, error)
	AcquireTargetedMutation(ctx context.Context, dataClass DataClass, targetKind, targetDigest string) (FreshnessLease, error)
}

// TargetedMutationFence is a transaction-scoped, streaming collection of
// target locks. It deliberately uses one physical fence connection for every
// target registered by a bulk mutation, while each target remains locked until
// the business transaction completes. It is not a read lease.
type TargetedMutationFence interface {
	AcquireTargetedMutation(ctx context.Context, dataClass DataClass, targetKind, targetDigest string) error
	Release()
}

// TargetedMutationFenceFactory is intentionally separate from the read gate:
// older/read-only gates stay source-safe, while DG6.2 writers require this
// bounded-connection transaction protocol before appending target events.
type TargetedMutationFenceFactory interface {
	BeginTargetedMutationFence(ctx context.Context) (TargetedMutationFence, error)
}

// RefreshOperation is intentionally content-free. It lets the application
// coalesce a live global refresh and enforce a short cooldown without exposing
// a cache key, a payload, or an outbox implementation to an HTTP handler.
type RefreshOperation struct {
	EventID     string
	CompletedAt time.Time
}

// RefreshOutboxPort is a separate V3 port so existing V1/V2 relays cannot
// accidentally claim or decode a CACHE_REFRESH_V3 message.
type RefreshOutboxPort interface {
	AppendRefresh(ctx context.Context, event CacheRefreshEnvelope) error
	ListRefreshReady(ctx context.Context, limit int) ([]OutboxEvent, error)
	ListRefreshUnknown(ctx context.Context, limit int) ([]OutboxEvent, error)
	FindActiveRefresh(ctx context.Context) (*RefreshOperation, error)
	FindLatestCompletedRefresh(ctx context.Context) (*RefreshOperation, error)
	Claim(ctx context.Context, id int64, eventType, worker string) (*Lease, bool, error)
	Mark(ctx context.Context, id int64, eventType, leaseToken, status, reason string, retryCount int, nextRetryAt *time.Time) (bool, error)
}

type RefreshGenerationPort interface {
	AdvanceGlobalRefresh(ctx context.Context, eventID string) (bool, error)
	MarkGlobalRefreshDirty(eventID string)
	EvictAllGovernedLocal(eventID string)
	SetGlobalRefreshFanoutHealthy(healthy bool)
}

type RefreshFanoutPort interface {
	Enabled() bool
	PublishRefresh(ctx context.Context, event CacheRefreshEnvelope) error
}

// RefreshFreshnessGate serializes creation of the singleton global operation.
// Candidate reads stay on their V1/V2 gates, whose implementations must also
// fail closed while any V3 operation is non-terminal.
type RefreshFreshnessGate interface {
	AcquireRefreshMutation(ctx context.Context) (FreshnessLease, error)
}
