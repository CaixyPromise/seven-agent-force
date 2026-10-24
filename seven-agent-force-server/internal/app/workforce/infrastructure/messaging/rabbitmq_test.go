package messaging

import (
	"strings"
	"testing"

	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

func TestOpenRequiresRegistrationQueue(t *testing.T) {
	t.Parallel()
	_, err := Open(config.RabbitMQConfig{Enabled: true})
	if err == nil || !strings.Contains(err.Error(), "registered worker queue") {
		t.Fatalf("Open error = %v, want registered queue error", err)
	}
}

func TestOpenRequiresRabbitMQ(t *testing.T) {
	t.Parallel()
	_, err := Open(config.RabbitMQConfig{})
	if err == nil || !strings.Contains(err.Error(), "rabbitmq is required") {
		t.Fatalf("Open error = %v, want required error", err)
	}
}
