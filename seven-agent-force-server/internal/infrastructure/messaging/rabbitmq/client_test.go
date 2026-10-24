package rabbitmq

import (
	"testing"
	"time"
)

func TestQueueArgumentsPreservesBoundedWorkerTopology(t *testing.T) {
	t.Parallel()

	args := queueArguments(QueueOptions{
		DeadLetterExchange:   "seven.workforce.tasks.dlx",
		DeadLetterRoutingKey: "dead.seven.workforce.worker.wrk_test.tasks",
		MessageTTL:           2 * time.Minute,
		MaxLength:            32,
		Overflow:             "reject-publish-dlx",
	})

	want := map[string]any{
		"x-dead-letter-exchange":    "seven.workforce.tasks.dlx",
		"x-dead-letter-routing-key": "dead.seven.workforce.worker.wrk_test.tasks",
		"x-message-ttl":             int64(120000),
		"x-max-length":              int64(32),
		"x-overflow":                "reject-publish-dlx",
	}
	if len(args) != len(want) {
		t.Fatalf("argument count = %d, want %d: %#v", len(args), len(want), args)
	}
	for key, expected := range want {
		if got := args[key]; got != expected {
			t.Fatalf("%s = %#v, want %#v", key, got, expected)
		}
	}
}

func TestQueueArgumentsReturnsNilForDefaults(t *testing.T) {
	t.Parallel()
	if got := queueArguments(QueueOptions{}); got != nil {
		t.Fatalf("default arguments = %#v, want nil", got)
	}
}
