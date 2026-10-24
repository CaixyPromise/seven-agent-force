package secretvalue

import (
	"bytes"
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
)

type masterProvider struct {
	current *keyring.MasterKey
	keys    map[string]*keyring.MasterKey
}

func (m masterProvider) Current(ctx context.Context) (*keyring.MasterKey, error) {
	_ = ctx
	if m.current == nil {
		return nil, nil
	}
	copy := *m.current
	copy.Material = append([]byte(nil), m.current.Material...)
	return &copy, nil
}

func (m masterProvider) ByKID(ctx context.Context, kid string) (*keyring.MasterKey, error) {
	_ = ctx
	value := m.keys[kid]
	if value == nil {
		return nil, nil
	}
	copy := *value
	copy.Material = append([]byte(nil), value.Material...)
	return &copy, nil
}

func TestSecretValueServiceRoundTrip(t *testing.T) {
	masterKey := &keyring.MasterKey{
		KID:      "SEVEN_MASTER_KEY_V1",
		Material: bytes.Repeat([]byte{3}, 32),
	}
	service := NewService(masterProvider{
		current: masterKey,
		keys: map[string]*keyring.MasterKey{
			masterKey.KID: masterKey,
		},
	})
	value, err := service.EncryptString(context.Background(), "config-secret")
	if err != nil {
		t.Fatalf("encrypt secret value: %v", err)
	}
	if value.WrapKeyRef != masterKey.KID || value.CiphertextB64 == "" || value.EDEKB64 == "" {
		t.Fatalf("unexpected encrypted secret value: %+v", value)
	}
	if value.Plain != "" || len(value.TempDEK) != 0 {
		t.Fatalf("runtime-only fields should not be populated after encrypt: %+v", value)
	}
	plain, err := service.DecryptString(context.Background(), value)
	if err != nil {
		t.Fatalf("decrypt secret value: %v", err)
	}
	if plain != "config-secret" {
		t.Fatalf("unexpected plain value: %s", plain)
	}
}

func TestSecretValueServiceFallbackActiveKey(t *testing.T) {
	masterKey := &keyring.MasterKey{
		KID:      "SEVEN_MASTER_KEY_V2",
		Material: bytes.Repeat([]byte{4}, 32),
	}
	service := NewService(masterProvider{
		current: masterKey,
		keys:    map[string]*keyring.MasterKey{},
	})
	value, err := service.EncryptString(context.Background(), "fallback")
	if err != nil {
		t.Fatalf("encrypt secret value: %v", err)
	}
	value.WrapKeyRef = "UNKNOWN"
	plain, err := service.DecryptString(context.Background(), value)
	if err != nil {
		t.Fatalf("decrypt with fallback: %v", err)
	}
	if plain != "fallback" {
		t.Fatalf("unexpected fallback plain: %s", plain)
	}
}

func TestSecretValueServiceRejectsIncompletePayload(t *testing.T) {
	service := NewService(masterProvider{})
	if _, err := service.DecryptString(context.Background(), SecretValue{}); err == nil {
		t.Fatalf("expected error for empty secret value")
	}
}
