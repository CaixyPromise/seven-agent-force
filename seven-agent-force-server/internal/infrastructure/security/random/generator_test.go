package random

import (
	"context"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestGenerators(t *testing.T) {
	service := New(config.RandomConfig{
		TokenLength: 16,
		NonceLength: 12,
		CodeLength:  6,
	})
	token, err := service.Token(context.Background())
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if strings.Contains(token, "=") {
		t.Fatalf("token should be raw url-safe base64: %s", token)
	}
	nonce, err := service.Nonce(context.Background())
	if err != nil {
		t.Fatalf("nonce: %v", err)
	}
	if strings.Contains(nonce, "=") {
		t.Fatalf("nonce should be raw url-safe base64: %s", nonce)
	}
	code, err := service.Code(context.Background())
	if err != nil {
		t.Fatalf("code: %v", err)
	}
	if len(code) != 6 {
		t.Fatalf("unexpected code length: %d", len(code))
	}
}
