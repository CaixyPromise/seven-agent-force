package application

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/domain"
	cachefacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/cache_governance/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/bytedance/sonic"
)

// DG5 adapters may depend on shared protocol contracts only. Keeping an
// application/domain import in an infrastructure package reverses the DDD
// dependency direction even when the implementation happens to compile.
func TestDG5InfrastructureDependsOnlyOnSharedProtocolContracts(t *testing.T) {
	root := cacheGovernanceRepositoryRoot(t)
	infrastructureDir := filepath.Join(root, "internal", "app", "system", "cache_governance", "infrastructure")
	entries, err := os.ReadDir(infrastructureDir)
	if err != nil {
		t.Fatalf("read DG5 infrastructure directory: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		name := entry.Name()
		path := filepath.Join(infrastructureDir, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			if spec.Path == nil {
				continue
			}
			dependency := strings.Trim(spec.Path.Value, "\"")
			if strings.Contains(dependency, "/internal/app/system/cache_governance/domain") ||
				strings.Contains(dependency, "/internal/app/system/cache_governance/application") ||
				strings.Contains(dependency, "/internal/app/system/cache_governance/facade") {
				t.Fatalf("%s reverses DDD dependency direction through %s", name, dependency)
			}
		}
	}
	legacyAcceptance := filepath.Join(root, "internal", "infrastructure", "datasource", "governance", "dg5_cache_acceptance_integration_test.go")
	if _, err := os.Stat(legacyAcceptance); err == nil {
		t.Fatalf("DG5 application acceptance must not live under infrastructure datasource governance: %s", legacyAcceptance)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat legacy DG5 acceptance path: %v", err)
	}
}

func cacheGovernanceRepositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func TestRegisterAndAfterCommitKeepInvalidationContentFree(t *testing.T) {
	outbox := &fakeOutbox{}
	generation := &fakeGeneration{}
	service := NewService(outbox, generation, &fakeFanout{enabled: true}, trustedFakeFreshnessGate{}, "worker-a")

	registration, err := service.Register(context.Background(), cachepolicy.DataClassConfigPublicScalar)
	if err != nil {
		t.Fatalf("register invalidation: %v", err)
	}
	if len(outbox.appended) != 1 {
		t.Fatalf("outbox append count=%d, want 1", len(outbox.appended))
	}
	event := outbox.appended[0]
	if err := event.Validate(); err != nil {
		t.Fatalf("invalid durable event: %v", err)
	}
	if strings.Contains(event.TargetDigest, "title") || strings.Contains(event.TargetDigest, "secret") {
		t.Fatalf("durable event target digest leaked source material: %q", event.TargetDigest)
	}
	service.AfterCommit(context.Background(), registration)
	if len(generation.dirty) != 1 || generation.dirty[0].eventID != registration.EventID {
		t.Fatalf("writer cache was not dirtied after commit: %+v", generation.dirty)
	}
}

func TestRelayDoesNotMarkCompleteUntilPublisherConfirms(t *testing.T) {
	event, err := domain.NewInvalidationEvent("event-confirm", cachepolicy.DataClassConfigPublicScalar)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	row := domain.OutboxEvent{ID: 1, EventID: event.EventID, EventOwner: domain.OutboxOwner, ScopeID: domain.ScopeID, EventType: domain.EventType, AggregateType: cachepolicy.CacheInvalidationAggregate, AggregateID: event.TargetDigest, Payload: string(payload)}

	outbox := &fakeOutbox{ready: []domain.OutboxEvent{row}}
	generation := &fakeGeneration{}
	brokerDown := &fakeFanout{enabled: true, publishErr: errors.New("confirm unknown")}
	service := NewService(outbox, generation, brokerDown, trustedFakeFreshnessGate{}, "worker-a")
	service.now = func() time.Time { return time.Unix(100, 0).UTC() }
	if err := service.RelayOutbox(context.Background(), 1); err != nil {
		t.Fatalf("relay publish failure should persist retry state: %v", err)
	}
	if len(outbox.marks) != 1 || outbox.marks[0].status != "FAILED" {
		t.Fatalf("publisher failure was completed or ignored: %+v", outbox.marks)
	}
	if len(generation.advanced) != 1 || brokerDown.published != 1 {
		t.Fatalf("expected generation then one publish attempt: generation=%+v published=%d", generation.advanced, brokerDown.published)
	}

	outbox = &fakeOutbox{ready: []domain.OutboxEvent{row}}
	generation = &fakeGeneration{}
	brokerUp := &fakeFanout{enabled: true}
	service = NewService(outbox, generation, brokerUp, trustedFakeFreshnessGate{}, "worker-a")
	if err := service.RelayOutbox(context.Background(), 1); err != nil {
		t.Fatalf("relay confirmed publish: %v", err)
	}
	if len(outbox.marks) != 1 || outbox.marks[0].status != "DONE" || brokerUp.published != 1 {
		t.Fatalf("confirmed publish did not complete exact outbox event: marks=%+v published=%d", outbox.marks, brokerUp.published)
	}
}

func TestRelayDeadLettersMalformedAndUnknownEvents(t *testing.T) {
	malformed := domain.OutboxEvent{ID: 2, EventID: "bad", EventOwner: domain.OutboxOwner, ScopeID: domain.ScopeID, EventType: domain.EventType, AggregateType: cachepolicy.CacheInvalidationAggregate, AggregateID: cachepolicy.ClassTargetDigest(cachepolicy.DataClassConfigPublicScalar), Payload: "{not-json"}
	unknown := domain.OutboxEvent{ID: 3, EventID: "unknown", EventOwner: domain.OutboxOwner, ScopeID: domain.ScopeID, EventType: "CACHE_UNKNOWN"}
	outbox := &fakeOutbox{ready: []domain.OutboxEvent{malformed}, unknown: []domain.OutboxEvent{unknown}}
	generation := &fakeGeneration{}
	broker := &fakeFanout{enabled: true}
	service := NewService(outbox, generation, broker, trustedFakeFreshnessGate{}, "worker-a")
	if err := service.RelayOutbox(context.Background(), 10); err != nil {
		t.Fatalf("relay malformed/unknown: %v", err)
	}
	if len(outbox.marks) != 2 || outbox.marks[0].status != "DEAD" || outbox.marks[1].status != "DEAD" {
		t.Fatalf("malformed or unknown cache event was not fail-closed: %+v", outbox.marks)
	}
	if len(generation.advanced) != 0 || broker.published != 0 {
		t.Fatalf("invalid payload reached cache or broker: advanced=%+v published=%d", generation.advanced, broker.published)
	}
}

// A syntactically valid envelope with an unrecognised field is hostile input,
// not a forward-compatible cache eviction. This test is intentionally more
// strict than Event.Validate: the Sonic decode boundary must reject it before
// generation advance or broker publication.
func TestRelayDeadLettersUnknownInvalidationJSONField(t *testing.T) {
	event, err := domain.NewInvalidationEvent("event-unexpected-json-field", cachepolicy.DataClassConfigPublicScalar)
	if err != nil {
		t.Fatalf("new invalidation event: %v", err)
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("marshal invalidation event: %v", err)
	}
	if len(payload) < 2 {
		t.Fatal("unexpected empty invalidation payload")
	}
	hostile := string(payload[:len(payload)-1]) + `,"unexpected":"not-allowed"}`
	outbox := &fakeOutbox{ready: []domain.OutboxEvent{{
		ID:            4,
		EventID:       event.EventID,
		EventOwner:    domain.OutboxOwner,
		ScopeID:       domain.ScopeID,
		EventType:     domain.EventType,
		AggregateType: cachepolicy.CacheInvalidationAggregate,
		AggregateID:   event.TargetDigest,
		Payload:       hostile,
	}}}
	generation := &fakeGeneration{}
	broker := &fakeFanout{enabled: true}
	service := NewService(outbox, generation, broker, trustedFakeFreshnessGate{}, "worker-a")
	if err := service.RelayOutbox(context.Background(), 10); err != nil {
		t.Fatalf("relay hostile event: %v", err)
	}
	if len(outbox.marks) != 1 || outbox.marks[0].status != "DEAD" {
		t.Fatalf("unknown JSON field was not fail-closed: %+v", outbox.marks)
	}
	if len(generation.advanced) != 0 || broker.published != 0 {
		t.Fatalf("unknown JSON field reached generation or broker: advanced=%+v published=%d", generation.advanced, broker.published)
	}
}

// An oversized but otherwise syntactically and semantically valid envelope
// must never reach Sonic-driven cache generation or RabbitMQ publication.
// This protects the durable sys_outbox_event path as well as the broker path.
func TestRelayDeadLettersOversizedInvalidationPayloadBeforeGenerationOrPublish(t *testing.T) {
	event := domain.InvalidationEvent{
		SchemaVersion: cachepolicy.SchemaVersionV1,
		EventID:       strings.Repeat("e", 1024),
		ScopeID:       domain.ScopeID,
		DataClass:     cachepolicy.DataClassConfigPublicScalar,
		TargetDigest:  cachepolicy.ClassTargetDigest(cachepolicy.DataClassConfigPublicScalar),
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("marshal oversized invalidation event: %v", err)
	}
	if len(payload) <= 1024 {
		t.Fatalf("oversized invalidation fixture is only %d bytes", len(payload))
	}
	outbox := &fakeOutbox{ready: []domain.OutboxEvent{{
		ID:            45,
		EventID:       event.EventID,
		EventOwner:    domain.OutboxOwner,
		ScopeID:       domain.ScopeID,
		EventType:     domain.EventType,
		AggregateType: cachepolicy.CacheInvalidationAggregate,
		AggregateID:   event.TargetDigest,
		Payload:       string(payload),
	}}}
	generation := &fakeGeneration{}
	broker := &fakeFanout{enabled: true}
	service := NewService(outbox, generation, broker, trustedFakeFreshnessGate{}, "worker-a")
	if err := service.RelayOutbox(context.Background(), 10); err != nil {
		t.Fatalf("relay oversized event: %v", err)
	}
	if len(outbox.marks) != 1 || outbox.marks[0].status != "DEAD" || outbox.marks[0].reason != "invalid cache invalidation payload" {
		t.Fatalf("oversized cache event was not terminally rejected: %+v", outbox.marks)
	}
	if len(generation.advanced) != 0 || broker.published != 0 {
		t.Fatalf("oversized payload reached generation or broker: advanced=%+v published=%d", generation.advanced, broker.published)
	}
}

func TestRelayDeadLettersBoundedOutboxPayloadWithoutDecodingIt(t *testing.T) {
	event, err := domain.NewInvalidationEvent("bounded-outbox-event", cachepolicy.DataClassDictPublicItems)
	if err != nil {
		t.Fatalf("new bounded outbox event: %v", err)
	}
	outbox := &fakeOutbox{ready: []domain.OutboxEvent{{
		ID:               46,
		EventID:          event.EventID,
		EventOwner:       domain.OutboxOwner,
		ScopeID:          domain.ScopeID,
		EventType:        domain.EventType,
		AggregateType:    cachepolicy.CacheInvalidationAggregate,
		AggregateID:      event.TargetDigest,
		PayloadOversized: true,
	}}}
	generation := &fakeGeneration{}
	broker := &fakeFanout{enabled: true}
	service := NewService(outbox, generation, broker, trustedFakeFreshnessGate{}, "worker-a")
	if err := service.RelayOutbox(context.Background(), 10); err != nil {
		t.Fatalf("relay bounded outbox event: %v", err)
	}
	if len(outbox.marks) != 1 || outbox.marks[0].status != "DEAD" || outbox.marks[0].reason != "cache invalidation payload exceeds protocol limit" {
		t.Fatalf("bounded oversized row was not terminally rejected: %+v", outbox.marks)
	}
	if len(generation.advanced) != 0 || broker.published != 0 {
		t.Fatalf("bounded oversized row reached generation or broker: advanced=%+v published=%d", generation.advanced, broker.published)
	}
}

func TestRelayRejectsForeignOwnerOrScopeEvenIfAnAdapterReturnsIt(t *testing.T) {
	event, err := domain.NewInvalidationEvent("event-foreign-boundary", cachepolicy.DataClassConfigPublicScalar)
	if err != nil {
		t.Fatalf("new event: %v", err)
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, row := range []domain.OutboxEvent{
		{ID: 41, EventID: event.EventID, EventOwner: "other-owner", ScopeID: domain.ScopeID, EventType: domain.EventType, AggregateType: cachepolicy.CacheInvalidationAggregate, AggregateID: event.TargetDigest, Payload: string(payload)},
		{ID: 42, EventID: event.EventID, EventOwner: domain.OutboxOwner, ScopeID: "local", EventType: domain.EventType, AggregateType: cachepolicy.CacheInvalidationAggregate, AggregateID: event.TargetDigest, Payload: string(payload)},
	} {
		outbox := &fakeOutbox{ready: []domain.OutboxEvent{row}}
		generation := &fakeGeneration{}
		fanout := &fakeFanout{enabled: true}
		service := NewService(outbox, generation, fanout, trustedFakeFreshnessGate{}, "worker-a")
		if err := service.RelayOutbox(context.Background(), 10); err != nil {
			t.Fatalf("relay foreign boundary row: %v", err)
		}
		if len(outbox.marks) != 0 || len(generation.advanced) != 0 || fanout.published != 0 {
			t.Fatalf("foreign owner/scope row escaped DG5 boundary: marks=%+v advanced=%+v published=%d", outbox.marks, generation.advanced, fanout.published)
		}
	}
}

func TestRelayDeadLettersWrongAggregateBeforeGenerationOrPublish(t *testing.T) {
	event, err := domain.NewInvalidationEvent("event-wrong-aggregate", cachepolicy.DataClassConfigPublicScalar)
	if err != nil {
		t.Fatalf("new invalidation event: %v", err)
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outbox := &fakeOutbox{ready: []domain.OutboxEvent{{
		ID:            43,
		EventID:       event.EventID,
		EventOwner:    domain.OutboxOwner,
		ScopeID:       domain.ScopeID,
		EventType:     domain.EventType,
		AggregateType: "other-aggregate",
		AggregateID:   event.TargetDigest,
		Payload:       string(payload),
	}}}
	generation := &fakeGeneration{}
	fanout := &fakeFanout{enabled: true}
	service := NewService(outbox, generation, fanout, trustedFakeFreshnessGate{}, "worker-a")
	if err := service.RelayOutbox(context.Background(), 10); err != nil {
		t.Fatalf("relay wrong aggregate: %v", err)
	}
	if len(outbox.marks) != 1 || outbox.marks[0].status != "DEAD" {
		t.Fatalf("wrong aggregate was not terminally rejected: %+v", outbox.marks)
	}
	if len(generation.advanced) != 0 || fanout.published != 0 {
		t.Fatalf("wrong aggregate reached cache or broker: advanced=%+v published=%d", generation.advanced, fanout.published)
	}
}

func TestRelayDeadLettersAggregateTargetMismatchBeforeGenerationOrPublish(t *testing.T) {
	event, err := domain.NewInvalidationEvent("event-wrong-aggregate-target", cachepolicy.DataClassDictPublicItems)
	if err != nil {
		t.Fatalf("new invalidation event: %v", err)
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outbox := &fakeOutbox{ready: []domain.OutboxEvent{{
		ID:            44,
		EventID:       event.EventID,
		EventOwner:    domain.OutboxOwner,
		ScopeID:       domain.ScopeID,
		EventType:     domain.EventType,
		AggregateType: cachepolicy.CacheInvalidationAggregate,
		AggregateID:   cachepolicy.ClassTargetDigest(cachepolicy.DataClassConfigPublicScalar),
		Payload:       string(payload),
	}}}
	generation := &fakeGeneration{}
	fanout := &fakeFanout{enabled: true}
	service := NewService(outbox, generation, fanout, trustedFakeFreshnessGate{}, "worker-a")
	if err := service.RelayOutbox(context.Background(), 10); err != nil {
		t.Fatalf("relay aggregate target mismatch: %v", err)
	}
	if len(outbox.marks) != 1 || outbox.marks[0].status != "DEAD" {
		t.Fatalf("aggregate target mismatch was not terminally rejected: %+v", outbox.marks)
	}
	if len(generation.advanced) != 0 || fanout.published != 0 {
		t.Fatalf("aggregate target mismatch reached cache or broker: advanced=%+v published=%d", generation.advanced, fanout.published)
	}
}

func TestHandleFanoutEvictsBeforeSuccessfulReturn(t *testing.T) {
	generation := &fakeGeneration{}
	service := NewService(&fakeOutbox{}, generation, &fakeFanout{enabled: true}, trustedFakeFreshnessGate{}, "worker-a")
	bad := domain.InvalidationEvent{EventID: "bad"}
	if err := service.HandleFanout(context.Background(), bad); !errors.Is(err, domain.ErrInvalidationEvent) {
		t.Fatalf("malformed fanout accepted: %v", err)
	}
	if len(generation.evicted) != 0 {
		t.Fatalf("malformed fanout evicted cache: %+v", generation.evicted)
	}
	good, _ := domain.NewInvalidationEvent("event-delivery", cachepolicy.DataClassDictPublicItems)
	if err := service.HandleFanout(context.Background(), good); err != nil {
		t.Fatalf("handle valid fanout: %v", err)
	}
	if len(generation.evicted) != 1 || generation.evicted[0].eventID != good.EventID {
		t.Fatalf("local L1 was not evicted before ACK path: %+v", generation.evicted)
	}
}

type fakeOutbox struct {
	appended []domain.InvalidationEvent
	ready    []domain.OutboxEvent
	unknown  []domain.OutboxEvent
	marks    []fakeMark
}

func (f *fakeOutbox) Append(_ context.Context, event domain.InvalidationEvent) error {
	f.appended = append(f.appended, event)
	return nil
}
func (f *fakeOutbox) ListReady(context.Context, int) ([]domain.OutboxEvent, error) {
	return append([]domain.OutboxEvent(nil), f.ready...), nil
}
func (f *fakeOutbox) ListUnknown(context.Context, int) ([]domain.OutboxEvent, error) {
	return append([]domain.OutboxEvent(nil), f.unknown...), nil
}
func (*fakeOutbox) Claim(_ context.Context, _ int64, _ string, _ string) (*domain.Lease, bool, error) {
	return &domain.Lease{Token: "fence"}, true, nil
}
func (f *fakeOutbox) Mark(_ context.Context, id int64, eventType, _ string, status, reason string, retryCount int, nextRetryAt *time.Time) (bool, error) {
	f.marks = append(f.marks, fakeMark{id: id, eventType: eventType, status: status, reason: reason, retryCount: retryCount, next: nextRetryAt})
	return true, nil
}

type fakeMark struct {
	id         int64
	eventType  string
	status     string
	reason     string
	retryCount int
	next       *time.Time
}

type fakeGeneration struct {
	advanced []generationCall
	dirty    []generationCall
	evicted  []generationCall
	healthy  bool
	rejected int
}

type generationCall struct {
	eventID string
	class   cachepolicy.DataClass
}

func (f *fakeGeneration) Advance(_ context.Context, eventID string, dataClass cachepolicy.DataClass) (bool, error) {
	f.advanced = append(f.advanced, generationCall{eventID: eventID, class: dataClass})
	return true, nil
}
func (f *fakeGeneration) MarkWriterDirty(eventID string, dataClass cachepolicy.DataClass) {
	f.dirty = append(f.dirty, generationCall{eventID: eventID, class: dataClass})
}
func (f *fakeGeneration) EvictAndResolve(eventID string, dataClass cachepolicy.DataClass) {
	f.evicted = append(f.evicted, generationCall{eventID: eventID, class: dataClass})
}
func (f *fakeGeneration) SetFanoutHealthy(healthy bool) { f.healthy = healthy }
func (f *fakeGeneration) RecordRejectedFanout()         { f.rejected++ }

type fakeFanout struct {
	enabled    bool
	publishErr error
	published  int
}

func (f *fakeFanout) Enabled() bool { return f.enabled }
func (f *fakeFanout) Publish(_ context.Context, _ domain.InvalidationEvent) error {
	f.published++
	return f.publishErr
}

type trustedFakeFreshnessGate struct{}

func (trustedFakeFreshnessGate) AcquireRead(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return trustedFakeFreshnessLease{}, nil
}

func (trustedFakeFreshnessGate) AcquireMutation(context.Context, cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return trustedFakeFreshnessLease{}, nil
}

type trustedFakeFreshnessLease struct{}

func (trustedFakeFreshnessLease) Trusted() bool { return true }
func (trustedFakeFreshnessLease) Release()      {}

var _ cachefacade.InvalidationRegistrar = (*Service)(nil)
