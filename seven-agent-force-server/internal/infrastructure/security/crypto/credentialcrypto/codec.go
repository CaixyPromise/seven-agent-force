package credentialcrypto

import (
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/envelope"
	"github.com/bytedance/sonic"
)

type Codec interface {
	Encode(secret envelope.Secret) (string, error)
	Decode(payload string) (envelope.Secret, error)
}

type codec struct{}

func NewCodec() Codec {
	return codec{}
}

func (codec) Encode(secret envelope.Secret) (string, error) {
	payload, err := sonic.Marshal(secret)
	if err != nil {
		return "", fmt.Errorf("marshal credential envelope: %w", err)
	}
	return string(payload), nil
}

func (codec) Decode(payload string) (envelope.Secret, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return envelope.Secret{}, fmt.Errorf("credential envelope payload is empty")
	}
	var secret envelope.Secret
	if err := sonic.UnmarshalString(payload, &secret); err != nil {
		return envelope.Secret{}, fmt.Errorf("unmarshal credential envelope: %w", err)
	}
	return secret, nil
}
