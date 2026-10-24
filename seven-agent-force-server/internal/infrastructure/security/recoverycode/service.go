package recoverycode

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	HashAlgorithm         = "PBKDF2WithHmacSHA256"
	DerivedKeyBits        = 256
	DefaultIterationCount = 210_000
	randomCodeBytes       = 16
	randomSaltBytes       = 16
	groupSize             = 4
)

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) GenerateCodes(batchSize int) ([]string, error) {
	if batchSize < 0 {
		batchSize = 0
	}
	codes := make([]string, 0, batchSize)
	for range batchSize {
		buf := make([]byte, randomCodeBytes)
		if _, err := rand.Read(buf); err != nil {
			return nil, fmt.Errorf("generate recovery code bytes: %w", err)
		}
		raw := strings.ToUpper(base64.RawURLEncoding.EncodeToString(buf))
		codes = append(codes, formatCode(raw))
	}
	return codes, nil
}

func (s *Service) GenerateSalt() (string, error) {
	buf := make([]byte, randomSaltBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate recovery salt: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func (s *Service) HashCode(code, saltB64 string, iterationCount int) (string, error) {
	salt, err := base64.StdEncoding.DecodeString(strings.TrimSpace(saltB64))
	if err != nil {
		return "", fmt.Errorf("decode recovery salt: %w", err)
	}
	if iterationCount <= 0 {
		iterationCount = DefaultIterationCount
	}
	derived := pbkdf2.Key([]byte(normalize(code)), salt, iterationCount, DerivedKeyBits/8, sha256.New)
	return base64.StdEncoding.EncodeToString(derived), nil
}

func (s *Service) VerifyCode(code, saltB64 string, iterationCount int, expectedHashB64 string) bool {
	actual, err := s.HashCode(code, saltB64, iterationCount)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(actual), []byte(strings.TrimSpace(expectedHashB64))) == 1
}

func (s *Service) HashAlgorithm() string {
	return HashAlgorithm
}

func normalize(value string) string {
	return strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(value, "-", "")))
}

func formatCode(raw string) string {
	var builder strings.Builder
	builder.Grow(len(raw) + len(raw)/groupSize)
	for i := range raw {
		if i > 0 && i%groupSize == 0 {
			builder.WriteByte('-')
		}
		builder.WriteByte(raw[i])
	}
	return builder.String()
}
