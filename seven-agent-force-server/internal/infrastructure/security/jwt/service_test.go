package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

func TestRS256SignVerifyAndJWKS(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa: %v", err)
	}
	publicKey := &privateKey.PublicKey
	dir := t.TempDir()
	privatePath := filepath.Join(dir, "jwt-private.pem")
	publicPath := filepath.Join(dir, "jwt-public.pem")
	writePEM(t, privatePath, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(privateKey))
	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		t.Fatalf("marshal public: %v", err)
	}
	writePEM(t, publicPath, "PUBLIC KEY", publicDER)

	keys, err := keyring.NewLocalProvider(config.KeysConfig{
		Provider: "local",
		JWT: config.JWTKeysConfig{
			Algorithm: "RS256",
			Active: config.JWTKeySourceConfig{
				KID:              "kid-active",
				PrivateKeySource: "file:" + privatePath,
				PublicKeySource:  "file:" + publicPath,
			},
			Retired: []config.JWTKeySourceConfig{
				{
					KID:             "kid-retired",
					PublicKeySource: "file:" + publicPath,
					VerifyUntil:     time.Minute,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("new local provider: %v", err)
	}
	service, err := New(keys, "RS256")
	if err != nil {
		t.Fatalf("new jwt service: %v", err)
	}
	raw, err := service.Sign(context.Background(), map[string]any{
		"sub": "user-1",
		"iss": "seven",
		"exp": time.Now().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := service.Verify(context.Background(), raw)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims["sub"] != "user-1" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	jwks, err := service.JWKS(context.Background())
	if err != nil {
		t.Fatalf("jwks: %v", err)
	}
	if len(jwks) == 0 {
		t.Fatal("expected jwks payload")
	}
	snapshot, err := service.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if snapshot.ActiveKID != "kid-active" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
}

func writePEM(t *testing.T, path, blockType string, der []byte) {
	t.Helper()
	block := &pem.Block{Type: blockType, Bytes: der}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatalf("write pem: %v", err)
	}
}
