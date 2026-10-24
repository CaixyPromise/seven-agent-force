package infrastructure

import (
	"context"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/alicebob/miniredis/v2"
	redisclient "github.com/redis/go-redis/v9"
)

func TestInboxRealtimeHubDeliversOnlyNewerHintsToTheirOwner(t *testing.T) {
	hub := newInboxRealtimeHub()
	ownerEvents, stopOwner := hub.Subscribe(42)
	defer stopOwner()
	otherEvents, stopOther := hub.Subscribe(84)
	defer stopOther()

	hub.Publish(domain.InboxChangedIntent{
		ScopeID:        "local",
		UserID:         42,
		ChangeSequence: 5,
		NewUnread:      true,
	})
	first := receiveInboxRealtimeEvent(t, ownerEvents)
	if first.UserID != 42 || first.ChangeSequence != 5 || !first.NewUnread {
		t.Fatalf("first owner event = %#v", first)
	}

	// Pub/Sub may redeliver while a client reconnects. A duplicate or an older
	// hint must not create a second browser-visible event.
	hub.Publish(first)
	hub.Publish(domain.InboxChangedIntent{
		ScopeID:        "local",
		UserID:         42,
		ChangeSequence: 4,
	})
	assertNoInboxRealtimeEvent(t, ownerEvents)
	assertNoInboxRealtimeEvent(t, otherEvents)

	hub.Publish(domain.InboxChangedIntent{
		ScopeID:        "local",
		UserID:         42,
		ChangeSequence: 6,
	})
	second := receiveInboxRealtimeEvent(t, ownerEvents)
	if second.ChangeSequence != 6 || second.NewUnread {
		t.Fatalf("second owner event = %#v", second)
	}
	assertNoInboxRealtimeEvent(t, otherEvents)
}

func TestInboxRealtimeHubCoalescesSlowSubscriberToLatestSequence(t *testing.T) {
	hub := newInboxRealtimeHub()
	events, stop := hub.Subscribe(42)
	defer stop()

	// The SSE writer has a one-event buffer. When it is slow, keep only the
	// newest position while preserving the fact that at least one unread item
	// arrived. REST remains the source of truth for the actual mailbox data.
	hub.Publish(domain.InboxChangedIntent{
		ScopeID:        "local",
		UserID:         42,
		ChangeSequence: 10,
		NewUnread:      true,
	})
	hub.Publish(domain.InboxChangedIntent{
		ScopeID:        "local",
		UserID:         42,
		ChangeSequence: 11,
		NewUnread:      false,
	})

	got := receiveInboxRealtimeEvent(t, events)
	if got.ChangeSequence != 11 || !got.NewUnread {
		t.Fatalf("coalesced event = %#v", got)
	}
	assertNoInboxRealtimeEvent(t, events)
}

func TestInboxRealtimeBusRelaysContentFreeHintAcrossInstances(t *testing.T) {
	mini := miniredis.RunT(t)
	firstClient := redisclient.NewClient(&redisclient.Options{Addr: mini.Addr()})
	secondClient := redisclient.NewClient(&redisclient.Options{Addr: mini.Addr()})
	defer firstClient.Close()
	defer secondClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := NewInboxRealtimeBus(firstClient, "seven-test", "local", nil)
	second := NewInboxRealtimeBus(secondClient, "seven-test", "local", nil)
	first.Start(ctx)
	second.Start(ctx)
	waitInboxRealtimeBusReady(t, first)
	waitInboxRealtimeBusReady(t, second)

	events, stop := second.SubscribeInboxChanges(42)
	defer stop()
	if err := first.PublishInboxChanged(context.Background(), domain.InboxChangedIntent{
		ScopeID:        "local",
		UserID:         42,
		ChangeSequence: 8,
		NewUnread:      true,
	}); err != nil {
		t.Fatalf("publish cross-instance hint: %v", err)
	}
	got := receiveInboxRealtimeEvent(t, events)
	if got.ScopeID != "local" || got.UserID != 42 || got.ChangeSequence != 8 || !got.NewUnread {
		t.Fatalf("cross-instance event = %#v", got)
	}
}

func waitInboxRealtimeBusReady(t *testing.T, bus *InboxRealtimeBus) {
	t.Helper()
	select {
	case <-bus.ready:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Redis Pub/Sub subscription")
	}
}

func receiveInboxRealtimeEvent(t *testing.T, events <-chan domain.InboxChangedIntent) domain.InboxChangedIntent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for inbox realtime event")
		return domain.InboxChangedIntent{}
	}
}

func assertNoInboxRealtimeEvent(t *testing.T, events <-chan domain.InboxChangedIntent) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected inbox realtime event: %#v", event)
	case <-time.After(40 * time.Millisecond):
	}
}
