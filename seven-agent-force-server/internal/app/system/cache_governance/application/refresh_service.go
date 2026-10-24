package application

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"time"

	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/google/uuid"
)

const refreshCooldown = time.Minute

// TransactionRunner is the minimal application transaction boundary needed
// by the protected refresh operation. The service neither imports datasource
// infrastructure nor accepts a raw database/cache/broker handle.
type TransactionRunner interface {
	Enabled() bool
	WithinTransaction(ctx context.Context, fn func(context.Context) error) error
}

// RefreshService owns DG6.3's singleton global refresh operation. It writes
// a strict V3 event in one transaction and marks the writer local cache dirty
// only after that transaction commits. A disabled or failed transaction never
// falls back to a local-only clear.
type RefreshService struct {
	tx             TransactionRunner
	outbox         cachepolicy.RefreshOutboxPort
	generation     cachepolicy.RefreshGenerationPort
	fanout         cachepolicy.RefreshFanoutPort
	freshness      cachepolicy.RefreshFreshnessGate
	now            func() time.Time
	requestEnabled atomic.Bool
}

func NewRefreshService(tx TransactionRunner, outbox cachepolicy.RefreshOutboxPort, generation cachepolicy.RefreshGenerationPort, fanout cachepolicy.RefreshFanoutPort, freshness cachepolicy.RefreshFreshnessGate) *RefreshService {
	service := &RefreshService{tx: tx, outbox: outbox, generation: generation, fanout: fanout, freshness: freshness, now: time.Now}
	// Unit and integration composition deliberately start active. The module
	// turns request creation off from the explicit rollout configuration, while
	// every current binary keeps V3 consume/recovery support available.
	service.requestEnabled.Store(true)
	return service
}

func (s *RefreshService) Enabled() bool {
	return s != nil && s.tx != nil && s.tx.Enabled() && s.outbox != nil && s.generation != nil && s.fanout != nil && s.freshness != nil
}

// SetRequestEnabled controls only new global-refresh requests. It deliberately
// does not disable V3 relay or fanout handling: a new binary must understand
// an already durable V3 event during rollout even before operators enable the
// user-facing operation after the old fleet has been drained.
func (s *RefreshService) SetRequestEnabled(enabled bool) {
	if s != nil {
		s.requestEnabled.Store(enabled)
	}
}

func (s *RefreshService) requestCreationEnabled() bool {
	return s != nil && s.requestEnabled.Load()
}

func (s *RefreshService) Refresh(ctx context.Context) (cachefacade.RefreshResult, error) {
	if !s.Enabled() {
		return cachefacade.RefreshResult{}, errors.New("cache governance refresh is not enabled")
	}
	if !s.requestCreationEnabled() {
		return cachefacade.RefreshResult{State: "DISABLED"}, nil
	}
	lease, err := s.freshness.AcquireRefreshMutation(ctx)
	if err != nil || lease == nil {
		if lease != nil {
			lease.Release()
		}
		if err == nil {
			err = errors.New("cache refresh fence is unavailable")
		}
		return cachefacade.RefreshResult{}, err
	}
	defer lease.Release()

	result := cachefacade.RefreshResult{}
	var committedEvent string
	err = s.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		active, findErr := s.outbox.FindActiveRefresh(txCtx)
		if findErr != nil {
			return findErr
		}
		if active != nil && strings.TrimSpace(active.EventID) != "" {
			result.State = "PENDING"
			return nil
		}
		latest, findErr := s.outbox.FindLatestCompletedRefresh(txCtx)
		if findErr != nil {
			return findErr
		}
		if latest != nil && !latest.CompletedAt.IsZero() && s.clock().Sub(latest.CompletedAt.UTC()) < refreshCooldown {
			result.State = "COOLDOWN"
			return nil
		}
		event, eventErr := cachepolicy.NewCacheRefreshEnvelope(uuid.NewString())
		if eventErr != nil {
			return eventErr
		}
		if appendErr := s.outbox.AppendRefresh(txCtx, event); appendErr != nil {
			return appendErr
		}
		committedEvent = event.EventID
		result.State = "PENDING"
		return nil
	})
	if err != nil {
		return cachefacade.RefreshResult{}, err
	}
	if committedEvent != "" {
		s.generation.MarkGlobalRefreshDirty(committedEvent)
	}
	return result, nil
}

func (s *RefreshService) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

// RelayOutbox follows the existing claim/fence -> Redis epoch -> confirmed
// fanout -> DONE sequence. A crash, unknown confirm, or malformed row remains
// reclaimable or is terminally diagnosed without giving a V1/V2 relay authority
// over V3.
func (s *RefreshService) RelayOutbox(ctx context.Context, limit int) error {
	if !s.Enabled() {
		return errors.New("cache governance refresh is not enabled")
	}
	if limit <= 0 || limit > maxRelayLimit {
		limit = maxRelayLimit
	}
	unknown, err := s.outbox.ListRefreshUnknown(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range unknown {
		if err := s.markDead(ctx, row, "unsupported cache refresh event type"); err != nil {
			return err
		}
	}
	rows, err := s.outbox.ListRefreshReady(ctx, limit)
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

func (s *RefreshService) HandleFanout(_ context.Context, event cachepolicy.CacheRefreshEnvelope) error {
	if !s.Enabled() {
		return cachepolicy.ErrFanoutUnavailable
	}
	if err := event.Validate(); err != nil {
		return err
	}
	s.generation.EvictAllGovernedLocal(event.EventID)
	return nil
}

func (s *RefreshService) markDead(ctx context.Context, row cachepolicy.OutboxEvent, reason string) error {
	lease, claimed, err := s.outbox.Claim(ctx, row.ID, row.EventType, "cache-governance-refresh-v3")
	if err != nil || !claimed {
		return err
	}
	_, err = s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DEAD", reason, row.RetryCount+1, nil)
	return err
}

func (s *RefreshService) relay(ctx context.Context, row cachepolicy.OutboxEvent) error {
	if strings.TrimSpace(row.EventOwner) != cachepolicy.CacheGovernanceOutboxOwner || strings.TrimSpace(row.ScopeID) != cachepolicy.StorageScopeSystemGlobal || strings.TrimSpace(row.EventType) != cachepolicy.CacheRefreshEventType {
		return nil
	}
	lease, claimed, err := s.outbox.Claim(ctx, row.ID, row.EventType, "cache-governance-refresh-v3")
	if err != nil || !claimed {
		return err
	}
	if strings.TrimSpace(row.AggregateType) != cachepolicy.CacheRefreshAggregate || strings.TrimSpace(row.AggregateID) != cachepolicy.CacheRefreshAggregateID || row.PayloadOversized {
		return s.dead(ctx, row, lease.Token, "invalid cache refresh envelope")
	}
	event, err := cachepolicy.DecodeCacheRefreshEnvelope([]byte(row.Payload))
	if err != nil || strings.TrimSpace(event.EventID) != strings.TrimSpace(row.EventID) {
		return s.dead(ctx, row, lease.Token, "invalid cache refresh envelope")
	}
	if _, err := s.generation.AdvanceGlobalRefresh(ctx, event.EventID); err != nil {
		return s.retry(ctx, row, lease.Token, "cache refresh epoch unavailable")
	}
	if !s.fanout.Enabled() {
		return s.retry(ctx, row, lease.Token, "cache refresh fanout unavailable")
	}
	if err := s.fanout.PublishRefresh(ctx, event); err != nil {
		return s.retry(ctx, row, lease.Token, "cache refresh fanout publish confirmation unavailable")
	}
	_, err = s.outbox.Mark(ctx, row.ID, row.EventType, lease.Token, "DONE", "", row.RetryCount, nil)
	return err
}

func (s *RefreshService) dead(ctx context.Context, row cachepolicy.OutboxEvent, token, reason string) error {
	_, err := s.outbox.Mark(ctx, row.ID, row.EventType, token, "DEAD", reason, row.RetryCount+1, nil)
	return err
}

func (s *RefreshService) retry(ctx context.Context, row cachepolicy.OutboxEvent, token, reason string) error {
	next := s.clock().Add(relayBackoff(row.RetryCount + 1))
	_, err := s.outbox.Mark(ctx, row.ID, row.EventType, token, "FAILED", reason, row.RetryCount+1, &next)
	return err
}

var _ cachefacade.RefreshFacade = (*RefreshService)(nil)
