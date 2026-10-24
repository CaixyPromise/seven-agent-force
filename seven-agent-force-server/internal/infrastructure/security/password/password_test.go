package password

import (
	"context"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestBcryptHashAndVerify(t *testing.T) {
	service, err := New(config.PasswordConfig{
		Algorithm: "bcrypt",
		Bcrypt:    config.BcryptPasswordConfig{Cost: 10},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	hashed, err := service.Hash(context.Background(), "secret")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(hashed, "$2") {
		t.Fatalf("unexpected hash prefix: %s", hashed)
	}
	if err := service.Verify(context.Background(), "secret", hashed); err != nil {
		t.Fatalf("verify: %v", err)
	}
	javaStyleHash := strings.Replace(hashed, "$2b$", "$2a$", 1)
	if err := service.Verify(context.Background(), "secret", javaStyleHash); err != nil {
		t.Fatalf("verify java-style hash: %v", err)
	}
}
