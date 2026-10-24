package infrastructure

import (
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/domain"
	"github.com/bytedance/sonic"
)

type CredentialPayloadCodec struct{}

func NewCredentialPayloadCodec() *CredentialPayloadCodec {
	return &CredentialPayloadCodec{}
}

func (c *CredentialPayloadCodec) EncodePasskey(payload domain.PasskeyPayload) (string, error) {
	raw, err := sonic.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal passkey payload: %w", err)
	}
	return string(raw), nil
}

func (c *CredentialPayloadCodec) DecodePasskey(payload string) (domain.PasskeyPayload, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return domain.PasskeyPayload{}, nil
	}
	var result domain.PasskeyPayload
	if err := sonic.UnmarshalString(payload, &result); err != nil {
		return domain.PasskeyPayload{}, fmt.Errorf("unmarshal passkey payload: %w", err)
	}
	return result, nil
}

func (c *CredentialPayloadCodec) EncodeRecoveryCode(payload domain.RecoveryCodePayload) (string, error) {
	raw, err := sonic.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal recovery code payload: %w", err)
	}
	return string(raw), nil
}

func (c *CredentialPayloadCodec) DecodeRecoveryCode(payload string) (domain.RecoveryCodePayload, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return domain.RecoveryCodePayload{}, fmt.Errorf("recovery code payload is empty")
	}
	var result domain.RecoveryCodePayload
	if err := sonic.UnmarshalString(payload, &result); err != nil {
		return domain.RecoveryCodePayload{}, fmt.Errorf("unmarshal recovery code payload: %w", err)
	}
	return result, nil
}
