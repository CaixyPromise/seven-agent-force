package infrastructure

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/credential/domain"
)

func TestCredentialPayloadCodecRoundTrip(t *testing.T) {
	codec := NewCredentialPayloadCodec()
	payload := domain.RecoveryCodePayload{
		Salt:            "salt",
		HashAlgorithm:   "PBKDF2WithHmacSHA256",
		IterationCount:  210000,
		BatchIdentifier: "batch_1",
	}
	raw, err := codec.EncodeRecoveryCode(payload)
	if err != nil {
		t.Fatalf("encode recovery code payload: %v", err)
	}
	decoded, err := codec.DecodeRecoveryCode(raw)
	if err != nil {
		t.Fatalf("decode recovery code payload: %v", err)
	}
	if decoded != payload {
		t.Fatalf("decoded payload mismatch: %#v", decoded)
	}
}
