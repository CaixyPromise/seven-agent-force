package totp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	DefaultDigits = otp.DigitsSix
	DefaultPeriod = 30
	DefaultSkew   = 1
)

type Enrollment struct {
	Secret string
}

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) GenerateSecret(ctx context.Context, issuer, accountName string) (Enrollment, error) {
	_ = ctx
	issuer = strings.TrimSpace(issuer)
	accountName = strings.TrimSpace(accountName)
	if issuer == "" {
		return Enrollment{}, fmt.Errorf("issuer must not be empty")
	}
	if accountName == "" {
		return Enrollment{}, fmt.Errorf("account name must not be empty")
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		Period:      DefaultPeriod,
		Digits:      DefaultDigits,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return Enrollment{}, fmt.Errorf("generate totp secret: %w", err)
	}
	return Enrollment{Secret: key.Secret()}, nil
}

func (s *Service) BuildKeyURI(secret, issuer, accountName string) (string, error) {
	secret = strings.TrimSpace(secret)
	issuer = strings.TrimSpace(issuer)
	accountName = strings.TrimSpace(accountName)
	if secret == "" {
		return "", fmt.Errorf("secret must not be empty")
	}
	if issuer == "" {
		return "", fmt.Errorf("issuer must not be empty")
	}
	if accountName == "" {
		return "", fmt.Errorf("account name must not be empty")
	}
	safeIssuer := urlEncode(issuer)
	safeAccountName := urlEncode(accountName)
	uri := fmt.Sprintf(
		"otpauth://totp/%s:%s?secret=%s&issuer=%s&algorithm=SHA1&digits=%d&period=%d",
		safeIssuer,
		safeAccountName,
		secret,
		safeIssuer,
		DefaultDigits.Length(),
		DefaultPeriod,
	)
	key, err := otp.NewKeyFromURL(uri)
	if err != nil {
		return "", fmt.Errorf("build totp key uri: %w", err)
	}
	return key.URL(), nil
}

func urlEncode(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	return strings.ReplaceAll(url.QueryEscape(raw), "+", "%20")
}

func (s *Service) VerifyCode(secret, code string, now time.Time, skew int) bool {
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	if skew < 0 {
		skew = DefaultSkew
	}
	valid, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{
		Period:    DefaultPeriod,
		Skew:      uint(skew),
		Digits:    DefaultDigits,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && valid
}
