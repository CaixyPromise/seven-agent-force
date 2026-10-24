package credentialcrypto

import (
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/envelope"
)

func TestCodecRoundTrip(t *testing.T) {
	c := NewCodec()
	raw, err := c.Encode(envelope.Secret{
		KID:           "SEVEN_MASTER_KEY_V1",
		EDEKB64:       "wrapped",
		CiphertextB64: "cipher",
	})
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	if raw != `{"kid":"SEVEN_MASTER_KEY_V1","edekB64":"wrapped","ciphertextB64":"cipher"}` {
		t.Fatalf("unexpected raw payload: %s", raw)
	}
	decoded, err := c.Decode(raw)
	if err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if decoded.KID != "SEVEN_MASTER_KEY_V1" || decoded.EDEKB64 != "wrapped" || decoded.CiphertextB64 != "cipher" {
		t.Fatalf("unexpected decoded secret: %+v", decoded)
	}
}

func TestCodecDecodeEmpty(t *testing.T) {
	c := NewCodec()
	if _, err := c.Decode("   "); err == nil {
		t.Fatalf("expected decode error for empty payload")
	}
}
