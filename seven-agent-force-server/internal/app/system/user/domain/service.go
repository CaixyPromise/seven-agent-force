package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"time"
)

var (
	emailPattern   = regexp.MustCompile(`[\w!#$%&'*+/=?^_` + "`" + `{|}~-]+(?:\.[\w!#$%&'*+/=?^_` + "`" + `{|}~-]+)*@(?:[\w](?:[\w-]*[\w])?\.)+[\w](?:[\w-]*[\w])?`)
	phonePattern   = regexp.MustCompile(`^1[3-9]\d{9}$`)
	accountPattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{4,15}$`)
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) ValidateEmail(value string) bool {
	return emailPattern.MatchString(strings.TrimSpace(value))
}

func (s *Service) ValidatePhone(value string) bool {
	return phonePattern.MatchString(strings.TrimSpace(value))
}

func (s *Service) ValidatePassword(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 8 || len(value) > 20 {
		return false
	}
	var hasUpper, hasLower, hasDigit bool
	for _, char := range value {
		switch {
		case char >= 'A' && char <= 'Z':
			hasUpper = true
		case char >= 'a' && char <= 'z':
			hasLower = true
		case char >= '0' && char <= '9':
			hasDigit = true
		}
	}
	return hasUpper && hasLower && hasDigit
}

func (s *Service) ValidateAccount(value string) bool {
	return accountPattern.MatchString(strings.TrimSpace(value))
}

func (s *Service) BuildOperationBinding(field, value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(field) + "|" + strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Enabled(status int) bool {
	return status == UserStatusNormal
}

func (s *Service) Locked(status int, unsealAt *time.Time, now time.Time) bool {
	if status == UserStatusNormal || unsealAt == nil {
		return false
	}
	return unsealAt.After(now.UTC())
}

func (s *Service) ValidUserStatus(status int) bool {
	return status == UserStatusNormal || status == UserStatusDisabled || status == UserStatusPendingReview
}
