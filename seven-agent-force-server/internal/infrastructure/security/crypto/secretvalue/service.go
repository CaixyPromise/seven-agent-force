package secretvalue

import (
	"context"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/envelope"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
)

type SecretValue struct {
	Plain         string `json:"plain,omitempty"`
	CiphertextB64 string `json:"ciphertextB64,omitempty"`
	EDEKB64       string `json:"edekB64,omitempty"`
	WrapKeyRef    string `json:"wrapKeyRef,omitempty"`
	TempDEK       []byte `json:"-"`
}

type Service interface {
	EncryptString(ctx context.Context, plain string) (SecretValue, error)
	DecryptString(ctx context.Context, value SecretValue) (string, error)
	EncryptBytes(ctx context.Context, plain []byte) (SecretValue, error)
	DecryptBytes(ctx context.Context, value SecretValue) ([]byte, error)
}

type service struct {
	keys keyring.MasterKeyProvider
}

func NewService(keys keyring.MasterKeyProvider) Service {
	return &service{keys: keys}
}

func (s *service) EncryptString(ctx context.Context, plain string) (SecretValue, error) {
	return s.EncryptBytes(ctx, []byte(plain))
}

func (s *service) DecryptString(ctx context.Context, value SecretValue) (string, error) {
	plain, err := s.DecryptBytes(ctx, value)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (s *service) EncryptBytes(ctx context.Context, plain []byte) (SecretValue, error) {
	if s == nil || s.keys == nil {
		return SecretValue{}, fmt.Errorf("master key provider is not configured")
	}
	masterKey, err := s.keys.Current(ctx)
	if err != nil {
		return SecretValue{}, err
	}
	if masterKey == nil || len(masterKey.Material) == 0 {
		return SecretValue{}, fmt.Errorf("active master key is not configured")
	}
	dek := envelope.NewDEK()
	ciphertextB64, err := envelope.EncryptAesGCM(dek, plain)
	if err != nil {
		return SecretValue{}, err
	}
	edekB64, err := envelope.EncryptAesGCM(masterKey.Material, dek)
	if err != nil {
		return SecretValue{}, err
	}
	return SecretValue{
		CiphertextB64: ciphertextB64,
		EDEKB64:       edekB64,
		WrapKeyRef:    masterKey.KID,
	}, nil
}

func (s *service) DecryptBytes(ctx context.Context, value SecretValue) ([]byte, error) {
	if s == nil || s.keys == nil {
		return nil, fmt.Errorf("master key provider is not configured")
	}
	if strings.TrimSpace(value.CiphertextB64) == "" || strings.TrimSpace(value.EDEKB64) == "" {
		return nil, fmt.Errorf("secret value ciphertext or edek is empty")
	}
	wrapKeyRef := strings.TrimSpace(value.WrapKeyRef)
	masterKey, err := s.keys.ByKID(ctx, wrapKeyRef)
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
		return nil, fmt.Errorf("master key for wrap key ref %s is not available", wrapKeyRef)
	}
	dek, err := envelope.DecryptAesGCM(masterKey.Material, value.EDEKB64)
	if err != nil {
		return nil, err
	}
	return envelope.DecryptAesGCM(dek, value.CiphertextB64)
}
