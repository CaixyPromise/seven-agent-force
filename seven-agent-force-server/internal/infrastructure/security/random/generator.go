package random

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
)

type TokenGenerator interface {
	Token(ctx context.Context) (string, error)
}

type NonceGenerator interface {
	Nonce(ctx context.Context) (string, error)
}

type CodeGenerator interface {
	Code(ctx context.Context) (string, error)
}

type Service struct {
	tokenLength int
	nonceLength int
	codeLength  int
}

func New(cfg config.RandomConfig) *Service {
	return &Service{
		tokenLength: cfg.TokenLength,
		nonceLength: cfg.NonceLength,
		codeLength:  cfg.CodeLength,
	}
}

func (s *Service) Token(ctx context.Context) (string, error) {
	_ = ctx
	return generateURLSafe(s.tokenLength)
}

func (s *Service) Nonce(ctx context.Context) (string, error) {
	_ = ctx
	return generateURLSafe(s.nonceLength)
}

func (s *Service) Code(ctx context.Context) (string, error) {
	_ = ctx
	length := s.codeLength
	if length <= 0 {
		length = 6
	}
	var builder strings.Builder
	builder.Grow(length)
	for range length {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		builder.WriteByte(byte('0' + n.Int64()))
	}
	return builder.String(), nil
}

func generateURLSafe(length int) (string, error) {
	if length <= 0 {
		length = 32
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
