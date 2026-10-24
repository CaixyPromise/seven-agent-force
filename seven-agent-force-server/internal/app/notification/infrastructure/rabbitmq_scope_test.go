package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
)

func TestNotificationRabbitTopologyKeepsLegacyLocalAndSeparatesOtherScopes(t *testing.T) {
	local := notificationRabbitTopology("local")
	if local.queue != NotificationQueue || local.routingKey != NotificationRoutingKey || local.dlq != NotificationDLQ {
		t.Fatalf("local topology=%#v, want legacy notification names", local)
	}
	first := notificationRabbitTopology("node:gray-a")
	second := notificationRabbitTopology("node:gray-b")
	if first.exchange != NotificationExchange || first.dlx != NotificationDLX {
		t.Fatalf("non-local topology must keep the shared exchanges: %#v", first)
	}
	if first.queue == NotificationQueue || first.routingKey == NotificationRoutingKey || first.dlq == NotificationDLQ {
		t.Fatalf("non-local topology reused legacy local names: %#v", first)
	}
	if first.queue == second.queue || first.routingKey == second.routingKey || first.dlq == second.dlq {
		t.Fatalf("different scopes share a topology: first=%#v second=%#v", first, second)
	}
	for _, value := range []string{first.queue, first.routingKey, first.dlq, first.dlk} {
		if strings.Contains(value, "gray-a") || strings.Contains(value, "node:") {
			t.Fatalf("scope routing name exposes raw scope: %q", value)
		}
	}
}

func TestScopedRabbitMQRejectsForeignScopeBeforePublish(t *testing.T) {
	adapter, err := NewScopedRabbitMQ(nil, false, "node:gray-a")
	if err != nil {
		t.Fatalf("create disabled scoped adapter: %v", err)
	}
	err = adapter.PublishDispatch(context.Background(), domain.DeliveryMessage{
		MessageID:  "notification:test",
		DeliveryID: "test",
		ScopeID:    "node:gray-b",
	})
	if !errors.Is(err, ErrRabbitMQScopeMismatch) {
		t.Fatalf("PublishDispatch() error=%v, want scope mismatch before disabled publish", err)
	}
}
