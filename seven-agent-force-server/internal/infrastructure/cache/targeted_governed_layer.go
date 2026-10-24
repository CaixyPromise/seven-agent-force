package cache

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/bytedance/sonic"
	redisclient "github.com/redis/go-redis/v9"
)

// targetedGovernedLayer is intentionally separate from the V1 class-wide
// layer. Its maps are bounded by unresolved outbox events; completed fanout
// removes the exact target and no target enumeration is exposed.
type targetedGovernedLayer struct {
	parent *governedLayer
	mu     sync.Mutex
	dirty  map[string]map[string]struct{}
	// dirtyCount is globally bounded across targets. A bulk revocation can
	// legitimately affect many sessions; retaining every event ID would turn a
	// security fence into an unbounded memory sink. Overflow clears the IDs and
	// leaves the entire v2 namespace source-only until process restart.
	dirtyCount int
	overflow   bool
	unsafe     map[string]bool
	gate       cachepolicy.TargetedFreshnessGate
}

const targetedWriterDirtyEventCap = 100

// targetedWireValue uses RawMessage to preserve the existing Sonic boundary
// while keeping a hard expiry outside the private projection itself.
type targetedWireValue struct {
	ExpiresAt  time.Time `json:"expiresAt"`
	Generation string    `json:"generation"`
	Value      []byte    `json:"value"`
}

func newTargetedGovernedLayer(parent *governedLayer) *targetedGovernedLayer {
	return &targetedGovernedLayer{parent: parent, dirty: make(map[string]map[string]struct{}), unsafe: make(map[string]bool)}
}

func (t *targetedGovernedLayer) GetOrLoadTargeted(ctx context.Context, request cachepolicy.TargetedReadRequest, dest any, loader TargetedLoader) (bool, error) {
	if !request.Valid() || loader == nil {
		return false, ErrClassifiedCacheRequestInvalid
	}
	if t == nil || t.parent == nil || t.parent.codec == nil {
		return loadTargetedAuthoritative(ctx, dest, loader)
	}
	lease, trusted := t.acquire(ctx, request, false)
	if !trusted {
		return t.load(ctx, dest, loader)
	}
	defer lease.Release()
	if !t.trusted(request) {
		return t.load(ctx, dest, loader)
	}
	globalEpoch, generation, _, err := t.currentEpochs(ctx, request)
	if err != nil {
		return t.load(ctx, dest, loader)
	}
	payloadKey := t.payloadKey(request, generation, globalEpoch)
	l1Key := t.l1Key(request, generation, globalEpoch)
	if t.parent.l1 != nil {
		if payload, ok := t.parent.l1.GetInNamespace(t.l1Namespace(), l1Key); ok {
			if found, valid := t.decode(payload, generation, dest); found && valid && t.trusted(request) && t.globalEpochCurrent(ctx, globalEpoch) {
				return true, nil
			}
			t.parent.l1.DeleteNamespace(t.l1Namespace())
		}
	}
	client, err := t.parent.client()
	if err != nil {
		return t.load(ctx, dest, loader)
	}
	payload, err := client.Get(ctx, payloadKey).Bytes()
	if err == nil {
		if found, valid := t.decode(payload, generation, dest); found && valid && t.trusted(request) && t.globalEpochCurrent(ctx, globalEpoch) {
			ttl, ttlErr := client.TTL(ctx, payloadKey).Result()
			if ttlErr == nil && ttl > 0 && t.parent.l1 != nil {
				t.parent.l1.SetInNamespace(t.l1Namespace(), l1Key, payload, minDuration(request.Entry.L1TTL, ttl))
			}
			return true, nil
		}
		_ = client.Del(ctx, payloadKey).Err()
	} else if !errors.Is(err, redisclient.Nil) {
		return t.load(ctx, dest, loader)
	}
	return t.loadAndStore(ctx, request, dest, payloadKey, l1Key, generation, globalEpoch, loader)
}

// loadTargetedAuthoritative is intentionally receiver-independent. A missing
// cache layer is source-only rather than a nil dereference or stale fallback.
func loadTargetedAuthoritative(ctx context.Context, dest any, loader TargetedLoader) (bool, error) {
	value, err := loader(ctx)
	if err != nil || value.Value == nil {
		return false, err
	}
	payload, err := sonic.Marshal(value.Value)
	if err != nil {
		return false, err
	}
	if err := sonic.Unmarshal(payload, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (t *targetedGovernedLayer) load(ctx context.Context, dest any, loader TargetedLoader) (bool, error) {
	if t == nil || t.parent == nil || t.parent.codec == nil {
		return loadTargetedAuthoritative(ctx, dest, loader)
	}
	value, err := loader(ctx)
	if err != nil {
		return false, err
	}
	if value.Value == nil {
		return false, nil
	}
	payload, err := t.parent.codec.Marshal(value.Value)
	if err != nil {
		return false, err
	}
	if err := t.parent.codec.Unmarshal(payload, dest); err != nil {
		return false, err
	}
	return true, nil
}

func (t *targetedGovernedLayer) loadAndStore(ctx context.Context, request cachepolicy.TargetedReadRequest, dest any, payloadKey, l1Key, generation, globalEpoch string, loader TargetedLoader) (bool, error) {
	value, err := loader(ctx)
	if err != nil {
		return false, err
	}
	if value.Value == nil {
		return false, nil
	}
	payload, err := t.parent.codec.Marshal(value.Value)
	if err != nil {
		return false, err
	}
	if err := t.parent.codec.Unmarshal(payload, dest); err != nil {
		return false, err
	}
	remaining := time.Until(value.ExpiresAt.UTC())
	ttl := minDuration(request.Entry.L2TTL, remaining)
	if !value.Cacheable || value.ExpiresAt.IsZero() || remaining <= 0 || ttl <= 0 || !t.trusted(request) || !t.globalEpochCurrent(ctx, globalEpoch) {
		return true, nil
	}
	wire, err := sonic.Marshal(targetedWireValue{ExpiresAt: value.ExpiresAt.UTC(), Generation: generation, Value: payload})
	if err != nil {
		return true, nil
	}
	client, err := t.parent.client()
	if err != nil {
		return true, nil
	}
	if err := client.Set(ctx, payloadKey, wire, ttl).Err(); err != nil {
		return true, nil
	}
	if t.parent.l1 != nil {
		t.parent.l1.SetInNamespace(t.l1Namespace(), l1Key, wire, minDuration(request.Entry.L1TTL, ttl))
	}
	return true, nil
}

func (t *targetedGovernedLayer) decode(payload []byte, generation string, dest any) (bool, bool) {
	var wire targetedWireValue
	if len(payload) == 0 || sonic.Unmarshal(payload, &wire) != nil || strings.TrimSpace(wire.Generation) != strings.TrimSpace(generation) || wire.ExpiresAt.IsZero() || !wire.ExpiresAt.After(time.Now().UTC()) || len(wire.Value) == 0 {
		return false, false
	}
	if t.parent.codec.Unmarshal(wire.Value, dest) != nil {
		return false, false
	}
	return true, true
}

func (t *targetedGovernedLayer) key(request cachepolicy.TargetedReadRequest) string {
	return request.TargetKind + ":" + request.TargetDigest
}
func (t *targetedGovernedLayer) payloadKey(request cachepolicy.TargetedReadRequest, generation, globalEpoch string) string {
	return t.parent.builder.Build("dg6", "session-v2", "payload", request.KeyMaterial(), generation, globalEpoch)
}
func (t *targetedGovernedLayer) l1Key(request cachepolicy.TargetedReadRequest, generation, globalEpoch string) string {
	return t.parent.builder.Build("dg6", "session-v2", "l1", request.KeyMaterial(), generation, globalEpoch)
}
func (t *targetedGovernedLayer) l1Namespace() string { return "dg6:session-v2" }
func (t *targetedGovernedLayer) generationKey(request cachepolicy.TargetedReadRequest) string {
	return t.parent.builder.Build("dg6", "session-v2", "generation", request.TargetDigest)
}
func (t *targetedGovernedLayer) eventKey(eventID string) string {
	return t.parent.builder.Build("dg6", "session-v2", "event", cachepolicy.EventDigest(eventID))
}

func (t *targetedGovernedLayer) currentGeneration(ctx context.Context, request cachepolicy.TargetedReadRequest) (string, error) {
	client, err := t.parent.client()
	if err != nil {
		return "", err
	}
	key := t.generationKey(request)
	generation, err := client.Get(ctx, key).Result()
	if err == nil && strings.TrimSpace(generation) != "" {
		return generation, nil
	}
	if !errors.Is(err, redisclient.Nil) {
		return "", err
	}
	seed, err := nextGenerationEpoch()
	if err != nil {
		return "", err
	}
	if _, err := client.SetNX(ctx, key, seed, governedGenerationTTL).Result(); err != nil {
		return "", err
	}
	return client.Get(ctx, key).Result()
}

// currentEpochs is the normal DG6.2 candidate path: one bounded MGET reads
// the V3 global epoch and exact target generation together. Missing/reset
// recovery remains source-safe and never turns an old targeted payload valid.
func (t *targetedGovernedLayer) currentEpochs(ctx context.Context, request cachepolicy.TargetedReadRequest) (globalEpoch, generation string, reset bool, err error) {
	client, err := t.parent.client()
	if err != nil {
		return "", "", false, err
	}
	values, err := client.MGet(ctx, t.parent.globalRefreshEpochKey(), t.generationKey(request)).Result()
	if err != nil {
		t.parent.redisHealthy.Store(false)
		return "", "", false, err
	}
	if len(values) == 2 {
		globalEpoch, _ = values[0].(string)
		generation, _ = values[1].(string)
		if strings.TrimSpace(globalEpoch) != "" && strings.TrimSpace(generation) != "" {
			t.parent.redisHealthy.Store(true)
			return globalEpoch, generation, false, nil
		}
	}
	globalEpoch, globalReset, err := t.parent.currentGlobalRefreshEpoch(ctx)
	if err != nil {
		return "", "", false, err
	}
	generation, err = t.currentGeneration(ctx, request)
	if err != nil {
		return "", "", false, err
	}
	return globalEpoch, generation, globalReset, nil
}

func (t *targetedGovernedLayer) acquire(ctx context.Context, request cachepolicy.TargetedReadRequest, mutation bool) (cachepolicy.FreshnessLease, bool) {
	t.mu.Lock()
	gate := t.gate
	t.mu.Unlock()
	if gate == nil {
		return nil, false
	}
	var lease cachepolicy.FreshnessLease
	var err error
	if mutation {
		lease, err = gate.AcquireTargetedMutation(ctx, request.Entry.DataClass, request.TargetKind, request.TargetDigest)
	} else {
		lease, err = gate.AcquireTargetedRead(ctx, request.Entry.DataClass, request.TargetKind, request.TargetDigest)
	}
	if err != nil || lease == nil || !lease.Trusted() {
		if lease != nil {
			lease.Release()
		}
		return nil, false
	}
	return lease, true
}

func (t *targetedGovernedLayer) trusted(request cachepolicy.TargetedReadRequest) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.overflow && t.parent.fanoutHealthy.Load() && t.parent.provider != nil && t.parent.provider.Configured() && !t.unsafe[t.key(request)] && len(t.dirty[t.key(request)]) == 0
}

func (t *targetedGovernedLayer) MarkTargetLocalDirty(eventID string, request cachepolicy.TargetedReadRequest) {
	if t == nil || !request.Valid() || strings.TrimSpace(eventID) == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.overflow {
		return
	}
	key := t.key(request)
	if t.dirty[key] == nil {
		t.dirty[key] = map[string]struct{}{}
	}
	if _, exists := t.dirty[key][eventID]; exists {
		return
	}
	if t.dirtyCount >= targetedWriterDirtyEventCap {
		t.dirty = make(map[string]map[string]struct{})
		t.dirtyCount = 0
		t.overflow = true
		return
	}
	t.dirty[key][eventID] = struct{}{}
	t.dirtyCount++
}
func (t *targetedGovernedLayer) EvictTargetLocalAndResolve(eventID string, request cachepolicy.TargetedReadRequest) {
	if t == nil || !request.Valid() {
		return
	}
	t.mu.Lock()
	key := t.key(request)
	if events := t.dirty[key]; events != nil {
		if _, existed := events[eventID]; existed {
			delete(events, eventID)
			if t.dirtyCount > 0 {
				t.dirtyCount--
			}
		}
		if len(events) == 0 {
			delete(t.dirty, key)
		}
	}
	t.mu.Unlock()
	// The target generation changes before fanout. Its old L1 key is therefore
	// unreachable; no per-session key registry or broad deletion is retained.
}
func (t *targetedGovernedLayer) AdvanceTargetGeneration(ctx context.Context, eventID string, request cachepolicy.TargetedReadRequest) (bool, error) {
	if t == nil || !request.Valid() {
		return false, ErrClassifiedCacheRequestInvalid
	}
	client, err := t.parent.client()
	if err != nil {
		return false, err
	}
	seed, err := nextGenerationEpoch()
	if err != nil {
		return false, err
	}
	applied, err := governedAdvanceGenerationScript.Run(ctx, client, []string{t.generationKey(request), t.eventKey(eventID)}, "1", governedEventTTL.Milliseconds(), governedGenerationTTL.Milliseconds(), seed).Int64()
	if err != nil {
		t.mu.Lock()
		t.unsafe[t.key(request)] = true
		t.mu.Unlock()
		return false, err
	}
	return applied > 0, nil
}
func (t *targetedGovernedLayer) SetTargetFreshnessGate(gate cachepolicy.TargetedFreshnessGate) {
	if t != nil {
		t.mu.Lock()
		t.gate = gate
		t.mu.Unlock()
	}
}

func (t *targetedGovernedLayer) evictAllLocal() {
	if t != nil && t.parent != nil && t.parent.l1 != nil {
		t.parent.l1.DeleteNamespace(t.l1Namespace())
	}
}

func (t *targetedGovernedLayer) globalEpochCurrent(ctx context.Context, expected string) bool {
	if t == nil || t.parent == nil {
		return false
	}
	// This is a final admission recheck after the normal MGET path already
	// obtained both epochs. A separate global read here can only make the
	// operation more conservative if Redis changed between decode and return.
	epoch, reset, err := t.parent.currentGlobalRefreshEpoch(ctx)
	return err == nil && !reset && strings.TrimSpace(epoch) == strings.TrimSpace(expected) && t.parent.trustedForRead(cachepolicy.DataClassActiveSessionValidity)
}
