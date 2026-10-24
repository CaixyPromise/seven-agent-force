package jwt

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/keyring"
	"github.com/lestrrat-go/jwx/v3/jwa"
	jwkjwx "github.com/lestrrat-go/jwx/v3/jwk"
	jwsjwx "github.com/lestrrat-go/jwx/v3/jws"
	jwtjwx "github.com/lestrrat-go/jwx/v3/jwt"
)

type Signer interface {
	Sign(ctx context.Context, claims map[string]any) (string, error)
}

type Verifier interface {
	Verify(ctx context.Context, raw string) (map[string]any, error)
}

type JWKSProvider interface {
	JWKS(ctx context.Context) ([]byte, error)
}

type KeyRotationSnapshot struct {
	Algorithm  string   `json:"algorithm"`
	ActiveKID  string   `json:"activeKid"`
	NextKID    string   `json:"nextKid"`
	VerifyKIDs []string `json:"verifyKids"`
}

type Service struct {
	keys      keyring.SigningKeyProvider
	algorithm jwa.SignatureAlgorithm
}

func New(keys keyring.SigningKeyProvider, algorithm string) (*Service, error) {
	if keys == nil {
		return nil, fmt.Errorf("signing key provider is required")
	}
	if algorithm == "" {
		algorithm = "RS256"
	}
	if algorithm != "RS256" {
		return nil, fmt.Errorf("unsupported jwt algorithm: %s", algorithm)
	}
	return &Service{
		keys:      keys,
		algorithm: jwa.RS256(),
	}, nil
}

func (s *Service) Sign(ctx context.Context, claims map[string]any) (string, error) {
	key, err := s.keys.Active(ctx)
	if err != nil {
		return "", err
	}
	if key == nil || key.PrivateKey == nil {
		return "", fmt.Errorf("active signing key is not configured")
	}
	token := jwtjwx.New()
	for k, v := range claims {
		if err := token.Set(k, v); err != nil {
			return "", err
		}
	}
	headers := jwsjwx.NewHeaders()
	if err := headers.Set(jwsjwx.KeyIDKey, key.KID); err != nil {
		return "", err
	}
	signed, err := jwtjwx.Sign(token, jwtjwx.WithKey(s.algorithm, key.PrivateKey, jwsjwx.WithProtectedHeaders(headers)))
	if err != nil {
		return "", err
	}
	return string(signed), nil
}

func (s *Service) Verify(ctx context.Context, raw string) (map[string]any, error) {
	verifySet, err := s.buildVerifyKeySet(ctx)
	if err != nil {
		return nil, err
	}
	token, err := jwtjwx.ParseString(raw, jwtjwx.WithKeySet(verifySet))
	if err != nil {
		return nil, err
	}
	result := make(map[string]any)
	for _, key := range token.Keys() {
		var value any
		if err := token.Get(key, &value); err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func (s *Service) JWKS(ctx context.Context) ([]byte, error) {
	set, err := s.buildVerifyKeySet(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(set)
}

func (s *Service) Snapshot(ctx context.Context) (KeyRotationSnapshot, error) {
	active, err := s.keys.Active(ctx)
	if err != nil {
		return KeyRotationSnapshot{}, err
	}
	next, err := s.keys.Next(ctx)
	if err != nil {
		return KeyRotationSnapshot{}, err
	}
	verifyKeys, err := s.keys.VerifyKeys(ctx, time.Now())
	if err != nil {
		return KeyRotationSnapshot{}, err
	}
	snapshot := KeyRotationSnapshot{
		Algorithm: fmt.Sprint(s.algorithm),
	}
	if active != nil {
		snapshot.ActiveKID = active.KID
	}
	if next != nil {
		snapshot.NextKID = next.KID
	}
	for _, item := range verifyKeys {
		snapshot.VerifyKIDs = append(snapshot.VerifyKIDs, item.KID)
	}
	return snapshot, nil
}

func (s *Service) buildVerifyKeySet(ctx context.Context) (jwkjwx.Set, error) {
	keys, err := s.keys.VerifyKeys(ctx, time.Now())
	if err != nil {
		return nil, err
	}
	set := jwkjwx.NewSet()
	for _, signingKey := range keys {
		if signingKey.PublicKey == nil {
			continue
		}
		key, err := jwkjwx.Import(signingKey.PublicKey)
		if err != nil {
			return nil, err
		}
		if err := key.Set(jwkjwx.KeyIDKey, signingKey.KID); err != nil {
			return nil, err
		}
		if err := key.Set(jwkjwx.AlgorithmKey, s.algorithm.String()); err != nil {
			return nil, err
		}
		set.AddKey(key)
	}
	return set, nil
}
