package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/domain"
	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/google/uuid"
)

const maxRelayLimit = 100

// Service orchestrates DG5's durable invalidation protocol. Business modules
// use its facade-only registrar; the relay is driven independently after the
// transaction commits.
type Service struct {
	outbox     cachepolicy.OutboxPort
	generation cachepolicy.GenerationPort
	fanout     cachepolicy.FanoutPort
	freshness  cachepolicy.FreshnessGate
	worker     string
	now        func() time.Time
	relayMu    sync.Mutex
	targeted   *TargetedService
	refresh    *RefreshService
}

// BindTargeted composes the independent DG6.2 protocol without widening the
// V1 registration surface. It is called only by the module composition root.
func (s *Service) BindTargeted(targeted *TargetedService) {
	if s != nil {
		s.targeted = targeted
	}
}

// BindRefresh composes the independent DG6.3 V3 protocol at the module root.
// It does not widen either V1 or V2 registration/decoder semantics.
func (s *Service) BindRefresh(refresh *RefreshService) {
	if s != nil {
		s.refresh = refresh
	}
}

func NewService(outbox cachepolicy.OutboxPort, generation cachepolicy.GenerationPort, fanout cachepolicy.FanoutPort, freshness cachepolicy.FreshnessGate, worker string) *Service {
	return &Service{
		outbox:     outbox,
		generation: generation,
		fanout:     fanout,
		freshness:  freshness,
		worker:     strings.TrimSpace(worker),
		now:        time.Now,
	}
}

func (s *Service) Enabled() bool {
	return s != nil && s.outbox != nil && s.generation != nil && s.fanout != nil && s.freshness != nil && strings.TrimSpace(s.worker) != ""
}

// Register appends a content-free invalidation in the current application
// transaction. The infrastructure adapter rejects an unbound transaction so
// DG5 cannot silently turn into a post-commit best-effort write.
func (s *Service) Register(ctx context.Context, dataClass cachepolicy.DataClass) (cachefacade.Registration, error) {
	if !s.Enabled() {
		return cachefacade.Registration{}, errors.New("cache governance is not enabled")
	}
	event, err := domain.NewInvalidationEvent(uuid.NewString(), dataClass)
	if err != nil {
		return cachefacade.Registration{}, err
	}
	if err := s.outbox.Append(ctx, event); err != nil {
		return cachefacade.Registration{}, err
	}
	return cachefacade.Registration{EventID: event.EventID, DataClass: event.DataClass}, nil
}

// AfterCommit immediately makes the writer's local cache untrusted. It must
// only be called once the encompassing business transaction returned success.
func (s *Service) AfterCommit(_ context.Context, registrations ...cachefacade.Registration) {
	if !s.Enabled() {
		return
	}
	for _, registration := range registrations {
		if strings.TrimSpace(registration.EventID) == "" {
			continue
		}
		if _, ok := cachepolicy.Entry(registration.DataClass); !ok {
			continue
		}
		s.generation.MarkWriterDirty(registration.EventID, registration.DataClass)
	}
}

// AcquireMutationFence serializes a classified mutation with all candidate
// cache reads of the same class. The shared application helper releases it on
// the real transaction completion path, never merely after a nested call.
func (s *Service) AcquireMutationFence(ctx context.Context, dataClass cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	if !s.Enabled() {
		return nil, errors.New("cache governance is not enabled")
	}
	return s.freshness.AcquireMutation(ctx, dataClass)
}

// RelayOutbox is bounded and serial per process. Lease/fence protection is
// still handled by the shared Outbox Store so separate instances may relay
// safely. A publish outcome is never marked complete before confirmation.
func (s *Service) RelayOutbox(ctx context.Context, limit int) error {
	if !s.Enabled() || !s.relayMu.TryLock() {
		return nil
	}
	defer s.relayMu.Unlock()
	if limit <= 0 || limit > maxRelayLimit {
		limit = maxRelayLimit
	}
	if s.targeted != nil {
		if err := s.targeted.RelayOutbox(ctx, limit); err != nil {
			return err
		}
	}
	if s.refresh != nil {
		if err := s.refresh.RelayOutbox(ctx, limit); err != nil {
			return err
		}
	}

	unknown, err := s.outbox.ListUnknown(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range unknown {
		if err := s.deadLetterUnknown(ctx, event); err != nil {
			return err
		}
	}

	events, err := s.outbox.ListReady(ctx, limit)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := s.relayEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) HandleTargetedFanout(ctx context.Context, event cachepolicy.TargetedInvalidationEnvelope) error {
	if s == nil || s.targeted == nil {
		return cachepolicy.ErrFanoutUnavailable
	}
	return s.targeted.HandleFanout(ctx, event)
}

func (s *Service) HandleRefreshFanout(ctx context.Context, event cachepolicy.CacheRefreshEnvelope) error {
	if s == nil || s.refresh == nil {
		return cachepolicy.ErrFanoutUnavailable
	}
	return s.refresh.HandleFanout(ctx, event)
}

// HandleFanout is called before the RabbitMQ delivery is ACKed. Invalid or
// hostile payloads are rejected by the listener rather than treated as a
// cache eviction request.
func (s *Service) HandleFanout(_ context.Context, event domain.InvalidationEvent) error {
	if !s.Enabled() {
		return domain.ErrFanoutUnavailable
	}
	if err := event.Validate(); err != nil {
		return err
	}
	s.generation.EvictAndResolve(event.EventID, event.DataClass)
	return nil
}

func (s *Service) deadLetterUnknown(ctx context.Context, event domain.OutboxEvent) error {
	if strings.TrimSpace(event.EventOwner) != domain.OutboxOwner || strings.TrimSpace(event.ScopeID) != domain.ScopeID || strings.TrimSpace(event.EventType) == "" {
		return nil
	}
	lease, claimed, err := s.outbox.Claim(ctx, event.ID, event.EventType, s.worker)
	if err != nil || !claimed {
		return err
	}
	_, err = s.outbox.Mark(ctx, event.ID, event.EventType, lease.Token, "DEAD", "unsupported cache invalidation event type", event.RetryCount+1, nil)
	return err
}

func (s *Service) relayEvent(ctx context.Context, row domain.OutboxEvent) error {
	if strings.TrimSpace(row.EventOwner) != domain.OutboxOwner || strings.TrimSpace(row.ScopeID) != domain.ScopeID || strings.TrimSpace(row.EventType) != domain.EventType {
		return nil
	}
	lease, claimed, err := s.outbox.Claim(ctx, row.ID, row.EventType, s.worker)
	if err != nil || !claimed {
		return err
	}
	if strings.TrimSpace(row.AggregateType) != cachepolicy.CacheInvalidationAggregate {
		_, markErr := s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DEAD", "unsupported cache invalidation aggregate", row.RetryCount+1, nil)
		return markErr
	}
	if row.PayloadOversized {
		// The bounded Store query deliberately did not select the raw body.
		// Claim and terminally record only this fixed reason; never decode,
		// publish, log, or retain attacker-controlled durable payload bytes.
		_, markErr := s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DEAD", "cache invalidation payload exceeds protocol limit", row.RetryCount+1, nil)
		return markErr
	}

	event, decodeErr := domain.DecodeInvalidationEvent([]byte(row.Payload))
	if decodeErr != nil || strings.TrimSpace(event.EventID) != strings.TrimSpace(row.EventID) {
		_, markErr := s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DEAD", "invalid cache invalidation payload", row.RetryCount+1, nil)
		return markErr
	}
	if strings.TrimSpace(row.AggregateID) != event.TargetDigest {
		_, markErr := s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DEAD", "cache invalidation aggregate target mismatch", row.RetryCount+1, nil)
		return markErr
	}
	if _, err := s.generation.Advance(ctx, event.EventID, event.DataClass); err != nil {
		return s.retry(ctx, row, lease.Token, "cache generation advance unavailable")
	}
	if !s.fanout.Enabled() {
		return s.retry(ctx, row, lease.Token, "cache fanout unavailable")
	}
	if err := s.fanout.Publish(ctx, event); err != nil {
		return s.retry(ctx, row, lease.Token, "cache fanout publish confirmation unavailable")
	}
	_, err = s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DONE", "", row.RetryCount, nil)
	return err
}

func (s *Service) retry(ctx context.Context, row domain.OutboxEvent, leaseToken, reason string) error {
	next := s.clock().Add(relayBackoff(row.RetryCount + 1))
	_, err := s.outbox.Mark(ctx, row.ID, row.EventType, leaseToken, "FAILED", reason, row.RetryCount+1, &next)
	return err
}

func (s *Service) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func relayBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return time.Second
	}
	if attempt >= 6 {
		return time.Minute
	}
	return time.Second << (attempt - 1)
}
