package cache

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/key"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/cache/l1"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	redisclient "github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

const (
	governedGenerationTTL = 24 * time.Hour
	governedEventTTL      = 24 * time.Hour
	// governedWriterDirtyEventCap matches the protocol's bounded relay window.
	// Once a writer commits more unresolved invalidations for one data class,
	// it intentionally stays fail-closed rather than retaining unbounded event
	// IDs or guessing that a duplicate fanout resolved a newer write.
	governedWriterDirtyEventCap = 100
)

var ErrClassifiedCacheRequestInvalid = errors.New("classified cache request is not catalogued")

// ClassifiedLoader retrieves an authoritative business result. A loader may
// return a value with Cacheable=false: callers still receive the value, but no
// L1/L2 entry is written. This is how row-level sensitivity checks avoid
// becoming a negative cache or a broader admission policy.
type ClassifiedLoader func(context.Context) (cachepolicy.CacheableValue, error)

// ClassifiedPreflight checks whether a current source-side authorization or
// catalog fact permits an L1/L2 candidate. It runs only after the governed
// source-adjacent freshness lease has been acquired and before any L1/L2
// access. A false result still invokes the authoritative loader, but never
// admits or returns an existing cache entry.
type ClassifiedPreflight func(context.Context) (bool, error)

// TargetedLoader returns the DG6.2 projection plus its hard expiry. The
// expiry is stored beside the opaque value so every L1/L2 hit can be checked
// without reaching into an SSO domain model.
type TargetedLoader func(context.Context) (cachepolicy.TargetedCacheableValue, error)

// GovernedCache is deliberately separate from Manager. Only reviewed system
// config/dict infrastructure should type-assert it; ordinary cache callers
// cannot accidentally opt a value into DG5.
type GovernedCache interface {
	GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader ClassifiedLoader) (bool, error)
	GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight ClassifiedPreflight, loader ClassifiedLoader) (bool, error)
	MarkLocalDirty(eventID string, classes ...cachepolicy.DataClass)
	EvictLocalAndResolve(eventID string, classes ...cachepolicy.DataClass)
	AdvanceGeneration(ctx context.Context, eventID string, class cachepolicy.DataClass) (bool, error)
	SetFanoutHealthy(healthy bool)
	SetFreshnessGate(gate cachepolicy.FreshnessGate)
	RecordRejectedFanout()
	GovernedStatus() GovernedStatus
}

// TargetedGovernedCache is a distinct DG6.2 surface. It cannot accidentally
// reuse a V1 class-wide key or invalidation operation.
type TargetedGovernedCache interface {
	GetOrLoadTargeted(ctx context.Context, request cachepolicy.TargetedReadRequest, dest any, loader TargetedLoader) (bool, error)
	MarkTargetLocalDirty(eventID string, request cachepolicy.TargetedReadRequest)
	EvictTargetLocalAndResolve(eventID string, request cachepolicy.TargetedReadRequest)
	AdvanceTargetGeneration(ctx context.Context, eventID string, request cachepolicy.TargetedReadRequest) (bool, error)
	SetTargetFreshnessGate(gate cachepolicy.TargetedFreshnessGate)
}

// GlobalRefreshGovernedCache is intentionally not part of Manager or the V1/
// V2 cache interfaces. Only cache-governance composition may advance the
// global V3 epoch or evict its own reviewed namespaces.
type GlobalRefreshGovernedCache interface {
	AdvanceGlobalRefresh(ctx context.Context, eventID string) (bool, error)
	MarkGlobalRefreshDirty(eventID string)
	EvictAllGovernedLocal(eventID string)
}

// GovernedStatus intentionally reports only safe operational state. It never
// includes a cached target, raw cache key, value, scope, or event identifier.
type GovernedStatus struct {
	Enabled              bool
	FanoutHealthy        bool
	RedisHealthy         bool
	FreshnessHealthy     bool
	DirtyClasses         int
	DirtyOverflowClasses int
	TransitioningClasses int
	UnsafeClasses        int
	ReadTrusted          bool
	// RejectedFanoutMessages is an aggregate-only operational signal. It
	// never includes a broker body, event ID, cache key, scope, or value.
	RejectedFanoutMessages uint64
	// GlobalRefreshEvictions is an aggregate-only V3 fanout confirmation. It
	// contains no cache key, target, event identifier, or value.
	GlobalRefreshEvictions uint64
}

type governedLayer struct {
	builder  *key.Builder
	l1       *l1.Store
	provider Provider
	codec    Codec

	generationTTL time.Duration
	eventTTL      time.Duration

	loadGroup           singleflight.Group
	mu                  sync.Mutex
	dirty               map[cachepolicy.DataClass]map[string]struct{}
	dirtyOverflow       map[cachepolicy.DataClass]bool
	generationInFlight  map[cachepolicy.DataClass]uint
	generationUnsafe    map[cachepolicy.DataClass]bool
	globalRefreshDirty  map[string]struct{}
	globalRefreshBusy   uint
	globalRefreshUnsafe bool
	// freshnessGate is installed only by the DG5 composition root. It holds a
	// cross-instance source-adjacent read lease across every classified read,
	// closing the committed-write/pre-relay cache race without making RabbitMQ
	// or Redis the sole freshness authority.
	freshnessGate cachepolicy.FreshnessGate
	// fenceEpoch changes under mu before any state transition that makes a
	// prior local-cache observation untrustworthy. Reads snapshot it under the
	// same short lock, perform Redis/L1/Sonic work without holding the lock,
	// then re-check it before accepting or admitting a cache value.
	fenceEpoch uint64

	fanoutHealthy    atomic.Bool
	redisHealthy     atomic.Bool
	freshnessHealthy atomic.Bool
	rejectedFanout   atomic.Uint64
	globalEvictions  atomic.Uint64
	targeted         *targetedGovernedLayer
}

type governedLoadResult struct {
	found     bool
	payload   []byte
	l1TTL     time.Duration
	cacheable bool
	fromL2    bool
}

// governedL1Read carries a short-lock fence snapshot. The Redis generation
// read, L1 lookup, and Sonic decode happen outside the lock and must all pass
// a final fence re-check before a cached value can be returned.
type governedL1Read struct {
	trusted     bool
	hit         bool
	payloadKey  string
	globalEpoch string
	fence       uint64
}

func NewGovernedLayer(builder *key.Builder, l1Store *l1.Store, provider Provider, codec Codec) *governedLayer {
	layer := &governedLayer{
		builder:            builder,
		l1:                 l1Store,
		provider:           provider,
		codec:              codec,
		generationTTL:      governedGenerationTTL,
		eventTTL:           governedEventTTL,
		dirty:              make(map[cachepolicy.DataClass]map[string]struct{}),
		dirtyOverflow:      make(map[cachepolicy.DataClass]bool),
		generationInFlight: make(map[cachepolicy.DataClass]uint),
		generationUnsafe:   make(map[cachepolicy.DataClass]bool),
		globalRefreshDirty: make(map[string]struct{}),
	}
	layer.targeted = newTargetedGovernedLayer(layer)
	return layer
}

func (l *governedLayer) GetOrLoadTargeted(ctx context.Context, request cachepolicy.TargetedReadRequest, dest any, loader TargetedLoader) (bool, error) {
	if l == nil || l.targeted == nil {
		return false, ErrCacheLayerUnsupported
	}
	return l.targeted.GetOrLoadTargeted(ctx, request, dest, loader)
}
func (l *governedLayer) MarkTargetLocalDirty(eventID string, request cachepolicy.TargetedReadRequest) {
	if l != nil && l.targeted != nil {
		l.targeted.MarkTargetLocalDirty(eventID, request)
	}
}
func (l *governedLayer) EvictTargetLocalAndResolve(eventID string, request cachepolicy.TargetedReadRequest) {
	if l != nil && l.targeted != nil {
		l.targeted.EvictTargetLocalAndResolve(eventID, request)
	}
}
func (l *governedLayer) AdvanceTargetGeneration(ctx context.Context, eventID string, request cachepolicy.TargetedReadRequest) (bool, error) {
	if l == nil || l.targeted == nil {
		return false, ErrCacheLayerUnsupported
	}
	return l.targeted.AdvanceTargetGeneration(ctx, eventID, request)
}
func (l *governedLayer) SetTargetFreshnessGate(gate cachepolicy.TargetedFreshnessGate) {
	if l != nil && l.targeted != nil {
		l.targeted.SetTargetFreshnessGate(gate)
	}
}

// MarkGlobalRefreshDirty is called only after the V3 transaction commits. It
// makes every catalogued candidate source-only on the writer until durable
// generation advancement and fanout converge; it never clears a generic cache
// manager or unrelated Redis state.
func (l *governedLayer) MarkGlobalRefreshDirty(eventID string) {
	if l == nil || strings.TrimSpace(eventID) == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.globalRefreshDirty) >= governedWriterDirtyEventCap {
		l.globalRefreshDirty = make(map[string]struct{})
		l.globalRefreshUnsafe = true
	} else {
		l.globalRefreshDirty[eventID] = struct{}{}
	}
	l.evictAllGovernedL1Locked()
	l.advanceFenceLocked()
}

// AdvanceGlobalRefresh advances one Redis global epoch idempotently. The
// epoch is included in every V1/V2 payload key, so old Redis data becomes
// unreachable without FLUSH*, SCAN, or prefix deletion.
func (l *governedLayer) AdvanceGlobalRefresh(ctx context.Context, eventID string) (bool, error) {
	if l == nil || strings.TrimSpace(eventID) == "" {
		return false, ErrClassifiedCacheRequestInvalid
	}
	l.mu.Lock()
	l.globalRefreshBusy++
	l.evictAllGovernedL1Locked()
	l.advanceFenceLocked()
	l.mu.Unlock()
	client, err := l.client()
	if err != nil {
		l.finishGlobalRefresh(false)
		l.redisHealthy.Store(false)
		return false, err
	}
	seed, err := nextGenerationEpoch()
	if err != nil {
		l.finishGlobalRefresh(false)
		return false, err
	}
	applied, err := governedAdvanceGenerationScript.Run(ctx, client, []string{l.globalRefreshEpochKey(), l.globalRefreshEventKey(eventID)}, "1", l.eventTTL.Milliseconds(), l.generationTTL.Milliseconds(), seed).Int64()
	if err != nil {
		l.redisHealthy.Store(false)
		l.finishGlobalRefresh(false)
		return false, err
	}
	l.redisHealthy.Store(true)
	l.finishGlobalRefresh(true)
	return applied > 0, nil
}

// EvictAllGovernedLocal is called before the V3 fanout delivery ACK. It only
// advances reviewed L1 namespaces (V1 and V2); it cannot touch session,
// locks, rate limits, replay protection, outbox, or arbitrary Manager keys.
func (l *governedLayer) EvictAllGovernedLocal(eventID string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.globalRefreshDirty, strings.TrimSpace(eventID))
	// A successful fanout is the only path that clears a prior overflow-safe
	// marker. The current global epoch has already made stale payloads
	// unreachable, and every governed local namespace is evicted again here.
	l.globalRefreshUnsafe = false
	l.evictAllGovernedL1Locked()
	l.advanceFenceLocked()
	l.globalEvictions.Add(1)
}

func (l *governedLayer) finishGlobalRefresh(success bool) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.globalRefreshBusy > 0 {
		l.globalRefreshBusy--
	}
	if !success {
		l.globalRefreshUnsafe = true
	}
	l.evictAllGovernedL1Locked()
	l.advanceFenceLocked()
}

func (l *governedLayer) evictAllGovernedL1Locked() {
	for _, entry := range cachepolicy.Catalog() {
		if entry.DataClass == cachepolicy.DataClassActiveSessionValidity {
			continue
		}
		l.deleteL1(entry.DataClass)
	}
	if l.targeted != nil {
		l.targeted.evictAllLocal()
	}
}

func (l *governedLayer) Name() string {
	return "governed_two_level"
}

func (l *governedLayer) GetOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, loader ClassifiedLoader) (bool, error) {
	return l.getOrLoadClassified(ctx, request, dest, nil, loader)
}

// GetOrLoadClassifiedWithPreflight is the narrow path for a classified read
// whose cache eligibility depends on current source-side authorization facts.
// It does not expose cache state to application code and cannot widen a
// catalog request: a failed or false preflight is source-only.
func (l *governedLayer) GetOrLoadClassifiedWithPreflight(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight ClassifiedPreflight, loader ClassifiedLoader) (bool, error) {
	return l.getOrLoadClassified(ctx, request, dest, preflight, loader)
}

func (l *governedLayer) getOrLoadClassified(ctx context.Context, request cachepolicy.ReadRequest, dest any, preflight ClassifiedPreflight, loader ClassifiedLoader) (bool, error) {
	if !request.Valid() {
		return false, ErrClassifiedCacheRequestInvalid
	}
	if loader == nil {
		return false, errors.New("classified cache loader is required")
	}
	lease, fresh := l.acquireReadFence(ctx, request.Entry.DataClass)
	if !fresh {
		return l.loadAuthoritative(ctx, dest, loader)
	}
	defer lease.Release()
	if preflight != nil {
		candidateAllowed, preflightErr := preflight(ctx)
		if preflightErr != nil {
			return false, preflightErr
		}
		if !candidateAllowed {
			return l.loadAuthoritative(ctx, dest, loader)
		}
	}
	if l == nil || l.codec == nil || !l.trustedForRead(request.Entry.DataClass) {
		return l.loadAuthoritative(ctx, dest, loader)
	}

	l1Read, err := l.readL1IfTrusted(ctx, request, dest)
	if err != nil {
		return l.loadAuthoritative(ctx, dest, loader)
	}
	if !l1Read.trusted {
		return l.loadAuthoritative(ctx, dest, loader)
	}
	if l1Read.hit {
		return true, nil
	}

	result, err, _ := l.loadGroup.Do(l1Read.payloadKey, func() (any, error) {
		return l.loadOrReadRedis(ctx, request, l1Read.payloadKey, l1Read.fence, loader)
	})
	if err != nil {
		return false, err
	}
	typed, ok := result.(governedLoadResult)
	if !ok || !typed.found || len(typed.payload) == 0 {
		return false, nil
	}
	if typed.fromL2 {
		accepted, decodeErr := l.decodeL2IfCurrentTrusted(ctx, request, l1Read.payloadKey, l1Read.fence, typed.payload, dest)
		if decodeErr != nil {
			return false, decodeErr
		}
		if accepted {
			return true, nil
		}
		return l.loadAuthoritative(ctx, dest, loader)
	}
	if err := l.codec.Unmarshal(typed.payload, dest); err != nil {
		return false, err
	}
	if typed.cacheable {
		l.setL1IfCurrentTrusted(ctx, request, l1Read.payloadKey, l1Read.fence, typed.payload, typed.l1TTL)
	}
	return true, nil
}

// acquireReadFence fail-closes only the cache candidate. When the durable
// outbox is pending, the database fence cannot be acquired, or the shared
// freshness state cannot be checked, callers still receive a fresh
// authoritative load rather than an old L1/L2 value.
func (l *governedLayer) acquireReadFence(ctx context.Context, dataClass cachepolicy.DataClass) (cachepolicy.FreshnessLease, bool) {
	if l == nil {
		return nil, false
	}
	l.mu.Lock()
	gate := l.freshnessGate
	l.mu.Unlock()
	if gate == nil {
		l.freshnessHealthy.Store(false)
		return nil, false
	}
	lease, err := gate.AcquireRead(ctx, dataClass)
	if err != nil || lease == nil || !lease.Trusted() {
		if lease != nil {
			lease.Release()
		}
		l.freshnessHealthy.Store(false)
		return nil, false
	}
	l.freshnessHealthy.Store(true)
	return lease, true
}

func (l *governedLayer) loadOrReadRedis(ctx context.Context, request cachepolicy.ReadRequest, payloadKey string, fence uint64, loader ClassifiedLoader) (governedLoadResult, error) {
	client, err := l.client()
	if err != nil {
		return l.loadForResult(ctx, request, payloadKey, fence, loader, false)
	}
	payload, err := client.Get(ctx, payloadKey).Bytes()
	if err == nil {
		ttl, ttlErr := client.TTL(ctx, payloadKey).Result()
		if ttlErr == nil && ttl > 0 {
			l.redisHealthy.Store(true)
			return governedLoadResult{found: true, payload: payload, l1TTL: minDuration(request.Entry.L1TTL, ttl), cacheable: true, fromL2: true}, nil
		}
		// A persistent/expired TTL cannot satisfy the catalog bound. Remove it
		// best-effort and return to the source of truth.
		_ = client.Del(ctx, payloadKey).Err()
	} else if !errors.Is(err, redisclient.Nil) {
		l.redisHealthy.Store(false)
		return l.loadForResult(ctx, request, payloadKey, fence, loader, false)
	}
	return l.loadForResult(ctx, request, payloadKey, fence, loader, true)
}

func (l *governedLayer) loadForResult(ctx context.Context, request cachepolicy.ReadRequest, payloadKey string, fence uint64, loader ClassifiedLoader, allowWrite bool) (governedLoadResult, error) {
	value, err := loader(ctx)
	if err != nil {
		return governedLoadResult{}, err
	}
	if value.Value == nil {
		return governedLoadResult{}, nil
	}
	payload, err := l.codec.Marshal(value.Value)
	if err != nil {
		return governedLoadResult{}, err
	}
	result := governedLoadResult{found: true, payload: payload, l1TTL: request.Entry.L1TTL, cacheable: value.Cacheable}
	if !value.Cacheable || !allowWrite {
		return result, nil
	}
	l.setL2IfCurrentTrusted(ctx, request, payloadKey, fence, payload)
	return result, nil
}

func (l *governedLayer) loadAuthoritative(ctx context.Context, dest any, loader ClassifiedLoader) (bool, error) {
	value, err := loader(ctx)
	if err != nil {
		return false, err
	}
	if value.Value == nil {
		return false, nil
	}
	payload, err := l.codec.Marshal(value.Value)
	if err != nil {
		return false, err
	}
	if err := l.codec.Unmarshal(payload, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (l *governedLayer) MarkLocalDirty(eventID string, classes ...cachepolicy.DataClass) {
	if l == nil || strings.TrimSpace(eventID) == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := false
	for _, class := range classes {
		if _, ok := cachepolicy.Entry(class); !ok {
			continue
		}
		if !l.dirtyOverflow[class] {
			events := l.dirty[class]
			if events == nil {
				events = make(map[string]struct{})
				l.dirty[class] = events
			}
			if _, alreadyTracked := events[eventID]; !alreadyTracked && len(events) >= governedWriterDirtyEventCap {
				delete(l.dirty, class)
				l.dirtyOverflow[class] = true
			} else {
				events[eventID] = struct{}{}
			}
		}
		l.deleteL1(class)
		changed = true
	}
	if changed {
		l.advanceFenceLocked()
	}
}

// EvictLocalAndResolve runs before the RabbitMQ consumer ACK. It is safe to
// repeat after a redelivery: the class remains evicted and only the matching
// writer-dirty event is cleared.
func (l *governedLayer) EvictLocalAndResolve(eventID string, classes ...cachepolicy.DataClass) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	changed := false
	for _, class := range classes {
		if _, ok := cachepolicy.Entry(class); !ok {
			continue
		}
		l.deleteL1(class)
		if !l.dirtyOverflow[class] {
			if events := l.dirty[class]; events != nil {
				delete(events, eventID)
				if len(events) == 0 {
					delete(l.dirty, class)
				}
			}
		}
		changed = true
	}
	if changed {
		l.advanceFenceLocked()
	}
}

// AdvanceGeneration uses a Redis idempotency marker and atomic INCR. A
// duplicate event can only observe the already advanced generation; it never
// rolls a generation backwards or marks an outbox event complete by itself.
func (l *governedLayer) AdvanceGeneration(ctx context.Context, eventID string, class cachepolicy.DataClass) (bool, error) {
	if l == nil || strings.TrimSpace(eventID) == "" {
		return false, ErrClassifiedCacheRequestInvalid
	}
	if _, ok := cachepolicy.Entry(class); !ok {
		return false, ErrClassifiedCacheRequestInvalid
	}
	// Fence and evict locally before Redis I/O. Readers that already started
	// must re-check this fence before accepting an L1/L2 value, while readers
	// that start afterwards fall through to the authoritative store until this
	// generation transition completes.
	l.beginGenerationAdvance(class)
	client, err := l.client()
	if err != nil {
		l.redisHealthy.Store(false)
		l.failGenerationAdvance(class)
		return false, err
	}
	seed, err := nextGenerationEpoch()
	if err != nil {
		l.failGenerationAdvance(class)
		return false, err
	}
	applied, err := governedAdvanceGenerationScript.Run(ctx, client, []string{l.generationKey(class), l.eventKey(eventID)}, "1", l.eventTTL.Milliseconds(), l.generationTTL.Milliseconds(), seed).Int64()
	if err != nil {
		l.redisHealthy.Store(false)
		l.failGenerationAdvance(class)
		return false, err
	}
	l.redisHealthy.Store(true)
	l.completeGenerationAdvance(class)
	return applied > 0, nil
}

func (l *governedLayer) beginGenerationAdvance(class cachepolicy.DataClass) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.generationInFlight[class]++
	l.deleteL1(class)
	l.advanceFenceLocked()
}

func (l *governedLayer) completeGenerationAdvance(class cachepolicy.DataClass) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.finishGenerationAdvanceLocked(class)
	delete(l.generationUnsafe, class)
	l.deleteL1(class)
	l.advanceFenceLocked()
}

// failGenerationAdvance releases this attempt's in-flight slot but preserves
// an explicit fail-closed marker. A later successful generation advance for
// the same class clears the marker; merely reconnecting Redis cannot make an
// outbox event that failed before the Lua script appear applied.
func (l *governedLayer) failGenerationAdvance(class cachepolicy.DataClass) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.finishGenerationAdvanceLocked(class)
	l.generationUnsafe[class] = true
	l.deleteL1(class)
	l.advanceFenceLocked()
}

func (l *governedLayer) finishGenerationAdvanceLocked(class cachepolicy.DataClass) {
	if l.generationInFlight[class] <= 1 {
		delete(l.generationInFlight, class)
		return
	}
	l.generationInFlight[class]--
}

func (l *governedLayer) SetFanoutHealthy(healthy bool) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fanoutHealthy.Store(healthy)
	l.advanceFenceLocked()
	if healthy {
		return
	}
	for _, entry := range cachepolicy.Catalog() {
		l.deleteL1(entry.DataClass)
	}
}

// SetFreshnessGate changes the source-adjacent global fence only at module
// composition time. Clearing or replacing it invalidates local namespaces so
// a cache candidate cannot survive a topology/state transition.
func (l *governedLayer) SetFreshnessGate(gate cachepolicy.FreshnessGate) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.freshnessGate = gate
	l.freshnessHealthy.Store(false)
	for _, entry := range cachepolicy.Catalog() {
		l.deleteL1(entry.DataClass)
	}
	l.advanceFenceLocked()
}

// RecordRejectedFanout makes malformed or unauthorized broker envelopes
// observable without retaining their potentially hostile body anywhere.
func (l *governedLayer) RecordRejectedFanout() {
	if l != nil {
		l.rejectedFanout.Add(1)
	}
}

func (l *governedLayer) GovernedStatus() GovernedStatus {
	if l == nil {
		return GovernedStatus{}
	}
	l.mu.Lock()
	dirtyClasses := l.dirtyClassCountLocked()
	dirtyOverflowClasses := 0
	for _, overflow := range l.dirtyOverflow {
		if overflow {
			dirtyOverflowClasses++
		}
	}
	transitioningClasses := 0
	for _, count := range l.generationInFlight {
		if count > 0 {
			transitioningClasses++
		}
	}
	unsafeClasses := 0
	for _, unsafe := range l.generationUnsafe {
		if unsafe {
			unsafeClasses++
		}
	}
	enabled := l.provider != nil && l.provider.Configured() && l.codec != nil
	fanoutHealthy := l.fanoutHealthy.Load()
	redisHealthy := l.redisHealthy.Load()
	freshnessHealthy := l.freshnessHealthy.Load()
	freshnessConfigured := l.freshnessGate != nil
	globalDirty := len(l.globalRefreshDirty)
	globalBusy := l.globalRefreshBusy
	globalUnsafe := l.globalRefreshUnsafe
	l.mu.Unlock()
	return GovernedStatus{
		Enabled:                enabled,
		FanoutHealthy:          fanoutHealthy,
		RedisHealthy:           redisHealthy,
		FreshnessHealthy:       freshnessHealthy,
		DirtyClasses:           dirtyClasses,
		DirtyOverflowClasses:   dirtyOverflowClasses,
		TransitioningClasses:   transitioningClasses,
		UnsafeClasses:          unsafeClasses,
		ReadTrusted:            enabled && freshnessConfigured && freshnessHealthy && fanoutHealthy && redisHealthy && dirtyClasses == 0 && transitioningClasses == 0 && unsafeClasses == 0 && globalDirty == 0 && globalBusy == 0 && !globalUnsafe,
		RejectedFanoutMessages: l.rejectedFanout.Load(),
		GlobalRefreshEvictions: l.globalEvictions.Load(),
	}
}

func (l *governedLayer) dirtyClassCountLocked() int {
	count := len(l.dirty)
	for class, overflow := range l.dirtyOverflow {
		if overflow {
			if _, tracked := l.dirty[class]; !tracked {
				count++
			}
		}
	}
	return count
}

func (l *governedLayer) trustedForRead(class cachepolicy.DataClass) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.trustedForReadLocked(class)
}

func (l *governedLayer) trustedForReadLocked(class cachepolicy.DataClass) bool {
	return l.fanoutHealthy.Load() && l.provider != nil && l.provider.Configured() && l.provider.Client() != nil && l.codec != nil && len(l.dirty[class]) == 0 && !l.dirtyOverflow[class] && l.generationInFlight[class] == 0 && !l.generationUnsafe[class] && len(l.globalRefreshDirty) == 0 && l.globalRefreshBusy == 0 && !l.globalRefreshUnsafe
}

// currentGeneration intentionally does Redis I/O without l.mu. If it has to
// create a missing generation, it advances the local fence before returning,
// so any read that began under the old epoch cannot accept a cached payload.
func (l *governedLayer) currentGeneration(ctx context.Context, class cachepolicy.DataClass) (generation string, reset bool, err error) {
	client, err := l.client()
	if err != nil {
		return "", false, err
	}
	key := l.generationKey(class)
	generation, err = client.Get(ctx, key).Result()
	if err == nil && strings.TrimSpace(generation) != "" {
		l.redisHealthy.Store(true)
		return generation, false, nil
	}
	if !errors.Is(err, redisclient.Nil) {
		l.redisHealthy.Store(false)
		return "", false, err
	}
	seed, seedErr := nextGenerationEpoch()
	if seedErr != nil {
		l.redisHealthy.Store(false)
		return "", false, seedErr
	}
	created, createErr := client.SetNX(ctx, key, seed, l.generationTTL).Result()
	if createErr != nil {
		l.redisHealthy.Store(false)
		return "", false, createErr
	}
	if created {
		l.redisHealthy.Store(true)
		l.mu.Lock()
		l.deleteL1(class)
		l.advanceFenceLocked()
		l.mu.Unlock()
		return seed, true, nil
	}
	generation, err = client.Get(ctx, key).Result()
	if err != nil || strings.TrimSpace(generation) == "" {
		l.redisHealthy.Store(false)
		if err == nil {
			err = ErrRedisUnavailable
		}
		return "", false, err
	}
	l.redisHealthy.Store(true)
	return generation, false, nil
}

func (l *governedLayer) beginTrustedRead(class cachepolicy.DataClass) (uint64, bool) {
	if l == nil {
		return 0, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fenceEpoch, l.trustedForReadLocked(class)
}

func (l *governedLayer) fenceCurrentAndTrusted(class cachepolicy.DataClass, fence uint64) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fenceEpoch == fence && l.trustedForReadLocked(class)
}

func (l *governedLayer) advanceFenceLocked() {
	l.fenceEpoch++
	if l.fenceEpoch == 0 {
		// Preserve zero as an impossible post-construction fence value in the
		// astronomically unlikely uint64 wraparound case.
		l.fenceEpoch = 1
	}
}

// nextGenerationEpoch returns a positive signed 64-bit decimal so Redis INCR
// can advance it. A fresh random epoch is essential when the generation key
// disappears: reusing "1" could make an old surviving L2 key look current.
func nextGenerationEpoch() (string, error) {
	var raw [8]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate cache generation epoch: %w", err)
	}
	value := binary.BigEndian.Uint64(raw[:]) & math.MaxInt64
	if value == 0 || value == math.MaxInt64 {
		value = 1
	}
	return strconv.FormatUint(value, 10), nil
}

func (l *governedLayer) client() (redisclient.UniversalClient, error) {
	if l == nil || l.provider == nil || !l.provider.Configured() || l.provider.Client() == nil {
		return nil, ErrRedisUnavailable
	}
	return l.provider.Client(), nil
}

// currentGlobalRefreshEpoch reads the V3 epoch before every candidate. A
// missing/reset epoch first invalidates local governed namespaces and makes
// that read source-only; a later candidate can safely populate the freshly
// seeded epoch. Redis uncertainty never permits an old L1/L2 hit.
func (l *governedLayer) currentGlobalRefreshEpoch(ctx context.Context) (epoch string, reset bool, err error) {
	client, err := l.client()
	if err != nil {
		return "", false, err
	}
	epoch, err = client.Get(ctx, l.globalRefreshEpochKey()).Result()
	if err == nil && strings.TrimSpace(epoch) != "" {
		l.redisHealthy.Store(true)
		return epoch, false, nil
	}
	if !errors.Is(err, redisclient.Nil) {
		l.redisHealthy.Store(false)
		return "", false, err
	}
	seed, err := nextGenerationEpoch()
	if err != nil {
		l.redisHealthy.Store(false)
		return "", false, err
	}
	created, err := client.SetNX(ctx, l.globalRefreshEpochKey(), seed, l.generationTTL).Result()
	if err != nil {
		l.redisHealthy.Store(false)
		return "", false, err
	}
	if created {
		l.redisHealthy.Store(true)
		l.mu.Lock()
		l.evictAllGovernedL1Locked()
		l.advanceFenceLocked()
		l.mu.Unlock()
		return seed, true, nil
	}
	epoch, err = client.Get(ctx, l.globalRefreshEpochKey()).Result()
	if err != nil || strings.TrimSpace(epoch) == "" {
		l.redisHealthy.Store(false)
		if err == nil {
			err = ErrRedisUnavailable
		}
		return "", false, err
	}
	l.redisHealthy.Store(true)
	return epoch, false, nil
}

// currentClassAndGlobalEpoch is the normal V1 candidate-read path: one
// bounded Redis MGET obtains both epochs before L1/L2 can be considered. A
// missing/reset value deliberately falls into the source-only recovery path
// below, where each epoch may be safely seeded without ever accepting old data.
func (l *governedLayer) currentClassAndGlobalEpoch(ctx context.Context, class cachepolicy.DataClass) (globalEpoch, generation string, reset bool, err error) {
	client, err := l.client()
	if err != nil {
		return "", "", false, err
	}
	values, err := client.MGet(ctx, l.globalRefreshEpochKey(), l.generationKey(class)).Result()
	if err != nil {
		l.redisHealthy.Store(false)
		return "", "", false, err
	}
	if len(values) == 2 {
		globalEpoch, _ = values[0].(string)
		generation, _ = values[1].(string)
		if strings.TrimSpace(globalEpoch) != "" && strings.TrimSpace(generation) != "" {
			l.redisHealthy.Store(true)
			return globalEpoch, generation, false, nil
		}
	}
	globalEpoch, globalReset, err := l.currentGlobalRefreshEpoch(ctx)
	if err != nil {
		return "", "", false, err
	}
	generation, generationReset, err := l.currentGeneration(ctx, class)
	if err != nil {
		return "", "", false, err
	}
	return globalEpoch, generation, globalReset || generationReset, nil
}

func (l *governedLayer) payloadKey(request cachepolicy.ReadRequest, generation, globalEpoch string) string {
	return l.builder.Build("dg5", "payload", request.KeyMaterial(), strings.TrimSpace(generation), strings.TrimSpace(globalEpoch))
}

func (l *governedLayer) generationKey(class cachepolicy.DataClass) string {
	return l.builder.Build("dg5", "generation", cachepolicy.ClassTargetDigest(class))
}

func (l *governedLayer) eventKey(eventID string) string {
	return l.builder.Build("dg5", "event", cachepolicy.EventDigest(eventID))
}

func (l *governedLayer) globalRefreshEpochKey() string {
	return l.builder.Build("dg6", "cache-refresh-v3", "epoch")
}

func (l *governedLayer) globalRefreshEventKey(eventID string) string {
	return l.builder.Build("dg6", "cache-refresh-v3", "event", cachepolicy.EventDigest(eventID))
}

func (l *governedLayer) namespace(class cachepolicy.DataClass) string {
	return "dg5:" + string(class)
}

// readL1IfTrusted snapshots the local fence under a short lock, then performs
// Redis, Ristretto, and Sonic work without holding it. Each cached result is
// accepted only after a second short-lock check. This keeps a slow Redis or
// decode from delaying a fail-closed fanout transition.
func (l *governedLayer) readL1IfTrusted(ctx context.Context, request cachepolicy.ReadRequest, dest any) (governedL1Read, error) {
	if l == nil {
		return governedL1Read{}, ErrClassifiedCacheRequestInvalid
	}
	fence, trusted := l.beginTrustedRead(request.Entry.DataClass)
	if !trusted {
		return governedL1Read{}, nil
	}
	globalEpoch, generation, reset, err := l.currentClassAndGlobalEpoch(ctx, request.Entry.DataClass)
	if err != nil {
		return governedL1Read{}, err
	}
	if reset {
		// The old L1 namespace was invalidated by currentGeneration. Start a
		// fresh fence observation so this first post-reset source read can
		// safely repopulate the new epoch instead of needlessly bypassing it.
		fence, trusted = l.beginTrustedRead(request.Entry.DataClass)
		if !trusted {
			return governedL1Read{}, nil
		}
	}
	payloadKey := l.payloadKey(request, generation, globalEpoch)
	result := governedL1Read{trusted: true, payloadKey: payloadKey, globalEpoch: globalEpoch, fence: fence}
	if !l.fenceCurrentAndTrusted(request.Entry.DataClass, fence) {
		return governedL1Read{}, nil
	}
	payload, ok := l.getL1(request.Entry.DataClass, payloadKey)
	if !ok {
		if !l.fenceCurrentAndTrusted(request.Entry.DataClass, fence) {
			return governedL1Read{}, nil
		}
		return result, nil
	}
	if err := l.codec.Unmarshal(payload, dest); err != nil {
		// A corrupted L1 value cannot be selectively deleted without exposing
		// the opaque physical key. Advancing only this class namespace is
		// bounded and makes every prior entry unreachable.
		l.mu.Lock()
		if l.fenceEpoch == fence {
			l.deleteL1(request.Entry.DataClass)
			l.advanceFenceLocked()
		}
		l.mu.Unlock()
		return governedL1Read{}, nil
	}
	if !l.fenceCurrentAndTrusted(request.Entry.DataClass, fence) {
		return governedL1Read{}, nil
	}
	result.hit = true
	return result, nil
}

// decodeL2IfCurrentTrusted rejects an L2 value when a fanout/dirty/generation
// transition happened after the lookup began. A concurrent transition that
// happens after this critical section instead linearizes after this read.
func (l *governedLayer) decodeL2IfCurrentTrusted(ctx context.Context, request cachepolicy.ReadRequest, payloadKey string, fence uint64, payload []byte, dest any) (bool, error) {
	if l == nil {
		return false, nil
	}
	if !l.fenceCurrentAndTrusted(request.Entry.DataClass, fence) {
		return false, nil
	}
	globalEpoch, generation, reset, err := l.currentClassAndGlobalEpoch(ctx, request.Entry.DataClass)
	if err != nil || reset || l.payloadKey(request, generation, globalEpoch) != payloadKey {
		return false, nil
	}
	if err := l.codec.Unmarshal(payload, dest); err != nil {
		return false, nil
	}
	return l.fenceCurrentAndTrusted(request.Entry.DataClass, fence), nil
}

func (l *governedLayer) setL1IfCurrentTrusted(ctx context.Context, request cachepolicy.ReadRequest, payloadKey string, fence uint64, payload []byte, ttl time.Duration) {
	if l == nil || ttl <= 0 {
		return
	}
	if !l.fenceCurrentAndTrusted(request.Entry.DataClass, fence) {
		return
	}
	globalEpoch, generation, reset, err := l.currentClassAndGlobalEpoch(ctx, request.Entry.DataClass)
	if err != nil || reset || l.payloadKey(request, generation, globalEpoch) != payloadKey {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.fenceEpoch != fence || !l.trustedForReadLocked(request.Entry.DataClass) {
		return
	}
	l.setL1(request.Entry.DataClass, payloadKey, payload, ttl)
}

// setL2IfCurrentTrusted gives source-loaded cache entries the same admission
// fence as L1. If a transaction's invalidation has changed the generation or
// fanout becomes untrusted while the source loader is running, that result is
// returned only to the caller and is not reintroduced into Redis.
func (l *governedLayer) setL2IfCurrentTrusted(ctx context.Context, request cachepolicy.ReadRequest, payloadKey string, fence uint64, payload []byte) {
	if l == nil {
		return
	}
	if !l.fenceCurrentAndTrusted(request.Entry.DataClass, fence) {
		return
	}
	globalEpoch, generation, reset, err := l.currentClassAndGlobalEpoch(ctx, request.Entry.DataClass)
	if err != nil || reset || l.payloadKey(request, generation, globalEpoch) != payloadKey {
		return
	}
	client, err := l.client()
	if err != nil {
		l.redisHealthy.Store(false)
		return
	}
	if !l.fenceCurrentAndTrusted(request.Entry.DataClass, fence) {
		return
	}
	if err := client.Set(ctx, payloadKey, payload, request.Entry.L2TTL).Err(); err != nil {
		l.redisHealthy.Store(false)
		return
	}
	l.redisHealthy.Store(true)
}

func (l *governedLayer) getL1(class cachepolicy.DataClass, cacheKey string) ([]byte, bool) {
	if l == nil || l.l1 == nil {
		return nil, false
	}
	// DG5 L1 entries are always looked up through the current class epoch.
	// A class invalidation therefore invalidates every prior per-scope key
	// without retaining an unbounded side map of those keys.
	return l.l1.GetInNamespace(l.namespace(class), cacheKey)
}

func (l *governedLayer) setL1(class cachepolicy.DataClass, cacheKey string, payload []byte, ttl time.Duration) {
	if l == nil || l.l1 == nil || ttl <= 0 {
		return
	}
	l.l1.SetInNamespace(l.namespace(class), cacheKey, payload, ttl)
}

func (l *governedLayer) deleteL1(class cachepolicy.DataClass) {
	if l == nil || l.l1 == nil {
		return
	}
	l.l1.DeleteNamespace(l.namespace(class))
}

func minDuration(left, right time.Duration) time.Duration {
	if left <= 0 || right <= 0 {
		return 0
	}
	if left < right {
		return left
	}
	return right
}

var governedAdvanceGenerationScript = redisclient.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then
  redis.call("SET", KEYS[1], ARGV[4], "PX", ARGV[3])
end
if redis.call("SET", KEYS[2], ARGV[1], "NX", "PX", ARGV[2]) then
  redis.call("INCR", KEYS[1])
  redis.call("PEXPIRE", KEYS[1], ARGV[3])
  return 1
end
return 0
`)
