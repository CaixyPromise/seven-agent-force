package envelope

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
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

func TestEncryptDecryptAesGCM(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	ciphertext, err := EncryptAesGCM(key, []byte("hello-envelope"))
	if err != nil {
		t.Fatalf("encrypt gcm: %v", err)
	}
	payload, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload) <= gcmIVSize+gcmTagSize {
		t.Fatalf("unexpected payload size: %d", len(payload))
	}
	plain, err := DecryptAesGCM(key, ciphertext)
	if err != nil {
		t.Fatalf("decrypt gcm: %v", err)
	}
	if string(plain) != "hello-envelope" {
		t.Fatalf("unexpected plain: %s", plain)
	}
}

func TestEnvelopeService(t *testing.T) {
	masterKey := &keyring.MasterKey{
		KID:      "SEVEN_MASTER_KEY_V1",
		Material: bytes.Repeat([]byte{1}, 32),
	}
	service := NewService(masterProvider{
		current: masterKey,
		keys: map[string]*keyring.MasterKey{
			masterKey.KID: masterKey,
		},
	})
	secret, err := service.EncryptString(context.Background(), "otp-secret")
	if err != nil {
		t.Fatalf("encrypt string: %v", err)
	}
	if secret.KID != "SEVEN_MASTER_KEY_V1" || secret.EDEKB64 == "" || secret.CiphertextB64 == "" {
		t.Fatalf("unexpected secret: %+v", secret)
	}
	plain, err := service.DecryptString(context.Background(), secret)
	if err != nil {
		t.Fatalf("decrypt string: %v", err)
	}
	if plain != "otp-secret" {
		t.Fatalf("unexpected plain: %s", plain)
	}
}

func TestEnvelopeServiceFallbackCurrentKey(t *testing.T) {
	masterKey := &keyring.MasterKey{
		KID:      "SEVEN_MASTER_KEY_V2",
		Material: bytes.Repeat([]byte{2}, 32),
	}
	service := NewService(masterProvider{
		current: masterKey,
		keys:    map[string]*keyring.MasterKey{},
	})
	secret, err := service.EncryptString(context.Background(), "fallback")
	if err != nil {
		t.Fatalf("encrypt string: %v", err)
	}
	secret.KID = "UNKNOWN"
	plain, err := service.DecryptString(context.Background(), secret)
	if err != nil {
		t.Fatalf("decrypt fallback: %v", err)
	}
	if !strings.EqualFold(plain, "fallback") {
		t.Fatalf("unexpected fallback plain: %s", plain)
	}
}
