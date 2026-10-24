package infrastructure

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/config/domain"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

const obfuscationSalt = byte(0x7A)

func DeobfuscateKey(obfuscated string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(obfuscated))
	if err != nil {
		return "", fmt.Errorf("decode obfuscated key: %w", err)
	}
	decoded := make([]byte, len(payload))
	for idx, item := range payload {
		decoded[idx] = item ^ obfuscationSalt
	}
	for left, right := 0, len(decoded)-1; left < right; left, right = left+1, right-1 {
		decoded[left], decoded[right] = decoded[right], decoded[left]
	}
	return string(decoded), nil
}

func EncryptWithClientPublicKey(publicKeyBase64 string, plainText string) (string, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKeyBase64))
	if err != nil {
		return "", fmt.Errorf("decode client public key: %w", err)
	}
	publicKeyRaw, err := x509.ParsePKIXPublicKey(keyBytes)
	if err != nil {
		return "", fmt.Errorf("parse client public key: %w", err)
	}
	publicKey, ok := publicKeyRaw.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("client public key is not rsa")
	}
	encrypted, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, []byte(plainText), nil)
	if err != nil {
		return "", fmt.Errorf("rsa oaep encrypt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

type SecretCipherAdapter struct {
	service secretvalueinfra.Service
}

type SensitiveRevealCipher struct{}

func NewSecretCipher(service secretvalueinfra.Service) *SecretCipherAdapter {
	return &SecretCipherAdapter{service: service}
}

func NewSensitiveRevealCipher() *SensitiveRevealCipher {
	return &SensitiveRevealCipher{}
}

func (a *SecretCipherAdapter) EncryptString(ctx context.Context, plain string) (domain.ConfigSecretValue, error) {
	if a == nil || a.service == nil {
		return domain.ConfigSecretValue{}, fmt.Errorf("secret cipher service is not configured")
	}
	value, err := a.service.EncryptString(ctx, plain)
	if err != nil {
		return domain.ConfigSecretValue{}, err
	}
	return domain.ConfigSecretValue{
		Plain:         plain,
		CiphertextB64: value.CiphertextB64,
		EDEKB64:       value.EDEKB64,
		WrapKeyRef:    value.WrapKeyRef,
	}, nil
}

func (a *SecretCipherAdapter) DecryptString(ctx context.Context, value domain.ConfigSecretValue) (string, error) {
	if strings.TrimSpace(value.Plain) != "" {
		return value.Plain, nil
	}
	if a == nil || a.service == nil {
		return "", fmt.Errorf("secret cipher service is not configured")
	}
	return a.service.DecryptString(ctx, secretvalueinfra.SecretValue{
		CiphertextB64: value.CiphertextB64,
		EDEKB64:       value.EDEKB64,
		WrapKeyRef:    value.WrapKeyRef,
	})
}

func (c *SensitiveRevealCipher) EncryptForClient(obfuscatedClientPublicKey string, plain string) (string, error) {
	clientPublicKey, err := DeobfuscateKey(obfuscatedClientPublicKey)
	if err != nil {
		return "", err
	}
	return EncryptWithClientPublicKey(clientPublicKey, plain)
}
