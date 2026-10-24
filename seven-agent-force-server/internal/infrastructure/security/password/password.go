package password

import (
	"context"
	"fmt"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"golang.org/x/crypto/bcrypt"
)

type Hasher interface {
	Hash(ctx context.Context, raw string) (string, error)
}

type Verifier interface {
	Verify(ctx context.Context, raw, encoded string) error
}

type Service struct {
	cost int
}

func New(cfg config.PasswordConfig) (*Service, error) {
	if cfg.Algorithm != "bcrypt" {
		return nil, fmt.Errorf("unsupported password algorithm: %s", cfg.Algorithm)
	}
	cost := cfg.Bcrypt.Cost
	if cost <= 0 {
		cost = 10
	}
	return &Service{cost: cost}, nil
}

func (s *Service) Hash(ctx context.Context, raw string) (string, error) {
	_ = ctx
	hashed, err := bcrypt.GenerateFromPassword([]byte(raw), s.cost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func (s *Service) Verify(ctx context.Context, raw, encoded string) error {
	_ = ctx
	return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(raw))
}
