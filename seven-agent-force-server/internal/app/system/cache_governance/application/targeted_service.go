package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/google/uuid"
)

// TargetedService owns DG6.2 only. It intentionally does not share V1's
// class-wide Register or relay semantics.
type TargetedService struct {
	outbox     cachepolicy.TargetedOutboxPort
	generation cachepolicy.TargetedGenerationPort
	fanout     cachepolicy.TargetedFanoutPort
	freshness  cachepolicy.TargetedFreshnessGate
	worker     string
	now        func() time.Time
	mu         sync.Mutex
}

func NewTargetedService(outbox cachepolicy.TargetedOutboxPort, generation cachepolicy.TargetedGenerationPort, fanout cachepolicy.TargetedFanoutPort, freshness cachepolicy.TargetedFreshnessGate, worker string) *TargetedService {
	return &TargetedService{outbox: outbox, generation: generation, fanout: fanout, freshness: freshness, worker: strings.TrimSpace(worker), now: time.Now}
}
func (s *TargetedService) Enabled() bool {
	return s != nil && s.outbox != nil && s.generation != nil && s.fanout != nil && s.freshness != nil && s.worker != ""
}
func (s *TargetedService) RegisterTarget(ctx context.Context, class cachepolicy.DataClass, kind, digest string) (cachefacade.TargetedRegistration, error) {
	if !s.Enabled() {
		return cachefacade.TargetedRegistration{}, errors.New("targeted cache governance is not enabled")
	}
	if class != cachepolicy.DataClassActiveSessionValidity || strings.TrimSpace(kind) != "active-session" || !cachepolicy.IsDigest(digest) {
		return cachefacade.TargetedRegistration{}, cachepolicy.ErrInvalidationEnvelope
	}
	event, err := cachepolicy.NewTargetedInvalidationEnvelope(uuid.NewString(), digest)
	if err != nil {
		return cachefacade.TargetedRegistration{}, err
	}
	if err = s.outbox.AppendTargeted(ctx, event); err != nil {
		return cachefacade.TargetedRegistration{}, err
	}
	return cachefacade.TargetedRegistration{EventID: event.EventID, DataClass: event.DataClass, TargetKind: event.TargetKind, TargetDigest: event.TargetDigest}, nil
}
func (s *TargetedService) AfterTargetCommit(_ context.Context, registrations ...cachefacade.TargetedRegistration) {
	if !s.Enabled() {
		return
	}
	for _, item := range registrations {
		request, ok := cachepolicy.ActiveSessionValidityReadRequestForDigest(item.TargetDigest)
		if ok && item.EventID != "" {
			s.generation.MarkTargetDirty(item.EventID, request)
		}
	}
}
func (s *TargetedService) AcquireTargetMutationFence(ctx context.Context, class cachepolicy.DataClass, kind, digest string) (cachepolicy.FreshnessLease, error) {
	if !s.Enabled() {
		return nil, errors.New("targeted cache governance is not enabled")
	}
	return s.freshness.AcquireTargetedMutation(ctx, class, kind, digest)
}

// BeginTargetMutationFence returns a transaction-scoped fence that can stream
// arbitrary pages of distinct session targets without consuming one database
// connection per target. Writers still acquire each exact target before its
// outbox event is appended and retain every acquired lock until completion.
func (s *TargetedService) BeginTargetMutationFence(ctx context.Context) (cachepolicy.TargetedMutationFence, error) {
	if !s.Enabled() {
		return nil, errors.New("targeted cache governance is not enabled")
	}
	factory, ok := s.freshness.(cachepolicy.TargetedMutationFenceFactory)
	if !ok || factory == nil {
		return nil, errors.New("targeted cache mutation fence factory is not configured")
	}
	return factory.BeginTargetedMutationFence(ctx)
}

func (s *TargetedService) RelayOutbox(ctx context.Context, limit int) error {
	if !s.Enabled() || !s.mu.TryLock() {
		return nil
	}
	defer s.mu.Unlock()
	if limit <= 0 || limit > maxRelayLimit {
		limit = maxRelayLimit
	}
	unknown, err := s.outbox.ListTargetedUnknown(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range unknown {
		if err := s.markDead(ctx, row, "unsupported targeted cache invalidation event type"); err != nil {
			return err
		}
	}
	rows, err := s.outbox.ListTargetedReady(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.relay(ctx, row); err != nil {
			return err
		}
	}
	return nil
}
func (s *TargetedService) HandleFanout(_ context.Context, event cachepolicy.TargetedInvalidationEnvelope) error {
	if !s.Enabled() {
		return cachepolicy.ErrFanoutUnavailable
	}
	if err := event.Validate(); err != nil {
		return err
	}
	request, ok := cachepolicy.ActiveSessionValidityReadRequestForDigest(event.TargetDigest)
	if !ok {
		return cachepolicy.ErrInvalidationEnvelope
	}
	s.generation.EvictTarget(event.EventID, request)
	return nil
}
func (s *TargetedService) markDead(ctx context.Context, row cachepolicy.OutboxEvent, reason string) error {
	lease, claimed, err := s.outbox.Claim(ctx, row.ID, row.EventType, s.worker)
	if err != nil || !claimed {
		return err
	}
	_, err = s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DEAD", reason, row.RetryCount+1, nil)
	return err
}
func (s *TargetedService) relay(ctx context.Context, row cachepolicy.OutboxEvent) error {
	if row.EventOwner != cachepolicy.CacheGovernanceOutboxOwner || row.ScopeID != cachepolicy.StorageScopeSystemGlobal || row.EventType != cachepolicy.TargetedCacheInvalidationEventType || row.AggregateType != cachepolicy.CacheInvalidationAggregate {
		return nil
	}
	lease, claimed, err := s.outbox.Claim(ctx, row.ID, row.EventType, s.worker)
	if err != nil || !claimed {
		return err
	}
	if row.PayloadOversized {
		return s.markDeadWithLease(ctx, row, lease.Token, "targeted cache invalidation payload exceeds protocol limit")
	}
	event, err := cachepolicy.DecodeTargetedInvalidationEnvelope([]byte(row.Payload))
	if err != nil || event.EventID != row.EventID || event.TargetDigest != row.AggregateID {
		return s.markDeadWithLease(ctx, row, lease.Token, "invalid targeted cache invalidation payload")
	}
	request, ok := cachepolicy.ActiveSessionValidityReadRequestForDigest(event.TargetDigest)
	if !ok {
		return s.markDeadWithLease(ctx, row, lease.Token, "invalid targeted cache invalidation target")
	}
	if _, err = s.generation.AdvanceTarget(ctx, event.EventID, request); err != nil {
		return s.retry(ctx, row, lease.Token, "targeted cache generation advance unavailable")
	}
	if !s.fanout.Enabled() {
		return s.retry(ctx, row, lease.Token, "targeted cache fanout unavailable")
	}
	if err = s.fanout.PublishTargeted(ctx, event); err != nil {
		return s.retry(ctx, row, lease.Token, "targeted cache fanout publish confirmation unavailable")
	}
	_, err = s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DONE", "", row.RetryCount, nil)
	return err
}
func (s *TargetedService) markDeadWithLease(ctx context.Context, row cachepolicy.OutboxEvent, leaseToken, reason string) error {
	_, err := s.outbox.Mark(ctx, row.ID, row.EventType, leaseToken, "DEAD", reason, row.RetryCount+1, nil)
	return err
}
func (s *TargetedService) retry(ctx context.Context, row cachepolicy.OutboxEvent, token, reason string) error {
	next := s.now().UTC().Add(relayBackoff(row.RetryCount + 1))
	_, err := s.outbox.Mark(ctx, row.ID, row.EventType, token, "FAILED", reason, row.RetryCount+1, &next)
	return err
}

var _ cachefacade.TargetedInvalidationRegistrar = (*TargetedService)(nil)
