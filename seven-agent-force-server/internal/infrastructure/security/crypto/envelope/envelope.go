package envelope

import (
	"context"
	stdaes "crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
)

const (
	gcmIVSize  = 12
	gcmTagSize = 16
)

type Secret struct {
	KID           string `json:"kid"`
	EDEKB64       string `json:"edekB64"`
	CiphertextB64 string `json:"ciphertextB64"`
}

type Service interface {
	EncryptString(ctx context.Context, plain string) (Secret, error)
	DecryptString(ctx context.Context, secret Secret) (string, error)
	EncryptBytes(ctx context.Context, plain []byte) (Secret, error)
	DecryptBytes(ctx context.Context, secret Secret) ([]byte, error)
}

type service struct {
	keys keyring.MasterKeyProvider
}

func NewService(keys keyring.MasterKeyProvider) Service {
	return &service{keys: keys}
}

func NewDEK() []byte {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		panic(fmt.Sprintf("generate envelope dek: %v", err))
	}
	return dek
}

func EncryptAesGCM(key, plain []byte) (string, error) {
	block, err := stdaes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("new aes cipher: %w", err)
	}
	iv := make([]byte, gcmIVSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate gcm iv: %w", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return "", fmt.Errorf("new gcm: %w", err)
	}
	combined := gcm.Seal(nil, iv, plain, nil)
	ciphertext := combined[:len(combined)-gcmTagSize]
	tag := combined[len(combined)-gcmTagSize:]
	payload := append(append(append(make([]byte, 0, len(iv)+len(tag)+len(ciphertext)), iv...), tag...), ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func DecryptAesGCM(key []byte, ciphertextB64 string) ([]byte, error) {
	payload, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decode envelope ciphertext: %w", err)
	}
	if len(payload) < gcmIVSize+gcmTagSize {
		return nil, fmt.Errorf("invalid envelope payload length")
	}
	iv := payload[:gcmIVSize]
	tag := payload[gcmIVSize : gcmIVSize+gcmTagSize]
	ciphertext := payload[gcmIVSize+gcmTagSize:]
	block, err := stdaes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("new aes cipher: %w", err)
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, len(iv))
	if err != nil {
		return nil, fmt.Errorf("new gcm: %w", err)
	}
	combined := append(append(make([]byte, 0, len(ciphertext)+len(tag)), ciphertext...), tag...)
	plain, err := gcm.Open(nil, iv, combined, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt envelope payload: %w", err)
	}
	return plain, nil
}

func (s *service) EncryptString(ctx context.Context, plain string) (Secret, error) {
	return s.EncryptBytes(ctx, []byte(plain))
}

func (s *service) DecryptString(ctx context.Context, secret Secret) (string, error) {
	plain, err := s.DecryptBytes(ctx, secret)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *service) EncryptBytes(ctx context.Context, plain []byte) (Secret, error) {
	if s == nil || s.keys == nil {
		return Secret{}, fmt.Errorf("master key provider is not configured")
	}
	masterKey, err := s.keys.Current(ctx)
	if err != nil {
		return Secret{}, err
	}
	if masterKey == nil || len(masterKey.Material) == 0 {
		return Secret{}, fmt.Errorf("active master key is not configured")
	}
	dek := NewDEK()
	ciphertextB64, err := EncryptAesGCM(dek, plain)
	if err != nil {
		return Secret{}, err
	}
	edekB64, err := EncryptAesGCM(masterKey.Material, dek)
	if err != nil {
		return Secret{}, err
	}
	return Secret{
		KID:           masterKey.KID,
		EDEKB64:       edekB64,
		CiphertextB64: ciphertextB64,
	}, nil
}

func (s *service) DecryptBytes(ctx context.Context, secret Secret) ([]byte, error) {
	if s == nil || s.keys == nil {
		return nil, fmt.Errorf("master key provider is not configured")
	}
	masterKey, err := s.keys.ByKID(ctx, strings.TrimSpace(secret.KID))
	if err != nil {
		return nil, err
	}
	if masterKey == nil || len(masterKey.Material) == 0 {
		masterKey, err = s.keys.Current(ctx)
		if err != nil {
			return nil, err
		}
	}
	if masterKey == nil || len(masterKey.Material) == 0 {
		return nil, fmt.Errorf("master key for kid %s is not available", secret.KID)
	}
	dek, err := DecryptAesGCM(masterKey.Material, secret.EDEKB64)
	if err != nil {
		return nil, err
	}
	return DecryptAesGCM(dek, secret.CiphertextB64)
}
