package infrastructure

import (
	"strings"
	"testing"
	"time"
)

func TestFanoutQueueNameVersionsImmutableTopologyArguments(t *testing.T) {
	digest := strings.Repeat("a", 64)
	legacy := "seven.cache-governance.dg5." + digest[:24]
	current := fanoutQueueName(digest)
	if current == legacy || !strings.Contains(current, "."+FanoutTopologyQueueVersion+".") {
		t.Fatalf("DG5 queue name does not version immutable topology arguments: current=%q legacy=%q", current, legacy)
	}
}

func TestFanoutRejectsOversizedEnvelopeBeforeDecode(t *testing.T) {
	payload := make([]byte, MaxFanoutEnvelopeBytes+1)
	if err := validateFanoutEnvelopePayload(payload); err == nil {
		t.Fatalf("DG5 fanout accepted %d-byte envelope beyond %d-byte limit", len(payload), MaxFanoutEnvelopeBytes)
	}
}

// Both queues are fleet-scoped. A rejected envelope retains its content-free
// diagnostic long enough for a short restart or investigation, but neither
// queue may retain broker resources forever after its instance is gone.
func TestFanoutTopologyBoundsDeadLetterDiagnosticRetention(t *testing.T) {
	adapter := &FanoutAdapter{
		queue:                "seven.cache-governance.dg5.test",
		deadLetterQueue:      "seven.cache-governance.dg5.test.dlq",
		deadLetterRoutingKey: "seven.cache-governance.dg5.test.dead",
	}
	topology := adapter.topology()
	var sourceExpires, sourceCarriesRawToDLQ bool
	var sourceRetention, deadLetterRetention time.Duration
	for _, queue := range topology.Queues {
		switch queue.Name {
		case adapter.queue:
			sourceRetention = queue.Options.Expires
			sourceExpires = sourceRetention > 0
			sourceCarriesRawToDLQ = queue.Options.DeadLetterExchange != "" || queue.Options.MessageTTL > 0
		case adapter.deadLetterQueue:
			deadLetterRetention = queue.Options.Expires
		}
	}
	if !sourceExpires {
		t.Fatal("DG5 instance source queue must remain controlled and expiring")
	}
	if sourceCarriesRawToDLQ {
		t.Fatal("DG5 source queue must not broker-dead-letter an untrusted raw body")
	}
	if deadLetterRetention != sourceRetention || deadLetterRetention != queueExpires {
		t.Fatalf("DG5 terminal DLQ retention=%s, want source-controlled %s", deadLetterRetention, queueExpires)
	}
}
