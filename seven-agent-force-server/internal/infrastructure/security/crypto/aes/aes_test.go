package aes

import (
	"encoding/base64"
	"testing"
)

func TestAllModesRoundTrip(t *testing.T) {
	modes := []Mode{ModeECB, ModeCBC, ModeGCM, ModeCFB, ModeOFB, ModeCTR}
	for _, mode := range modes {
		t.Run(string(mode), func(t *testing.T) {
			generated, err := Generate(mode)
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			spec := CipherSpec{
				Mode:   mode,
				KeyB64: generated["key"],
				IVB64:  generated["iv"],
			}
			encrypted, err := Encrypt("hello-seven", spec)
			if err != nil {
				t.Fatalf("encrypt: %v", err)
			}
			decrypted, err := Decrypt(encrypted, spec)
			if err != nil {
				t.Fatalf("decrypt: %v", err)
			}
			if decrypted != "hello-seven" {
				t.Fatalf("unexpected decrypted value: %s", decrypted)
			}
			key, _ := base64.StdEncoding.DecodeString(generated["key"])
			if len(key) != 32 {
				t.Fatalf("unexpected key size: %d", len(key))
			}
			iv, _ := base64.StdEncoding.DecodeString(generated["iv"])
			switch mode {
			case ModeECB:
				if generated["iv"] != "" {
					t.Fatalf("ecb should not generate iv")
				}
			case ModeGCM:
				if len(iv) != 12 {
					t.Fatalf("unexpected gcm iv size: %d", len(iv))
				}
			default:
				if len(iv) != 16 {
					t.Fatalf("unexpected iv size: %d", len(iv))
				}
			}
		})
	}
}

func TestInvalidKeyOrCiphertext(t *testing.T) {
	_, err := NewCipher(ModeCBC, "bad", "", EncryptMode)
	if err == nil {
		t.Fatal("expected invalid key error")
	}
	_, err = Decrypt("bad", CipherSpec{
		Mode:   ModeCBC,
		KeyB64: base64.StdEncoding.EncodeToString(make([]byte, 32)),
		IVB64:  base64.StdEncoding.EncodeToString(make([]byte, 16)),
	})
	if err == nil {
		t.Fatal("expected invalid ciphertext error")
	}
}
