package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrIntentInvalid      = errors.New("DELIVERY_INTENT_INVALID")
	ErrIntentNotAvailable = errors.New("DELIVERY_INTENT_NOT_AVAILABLE")
	ErrRequestStale       = errors.New("DELIVERY_REQUEST_STALE")
	ErrDecisionConflict   = errors.New("DELIVERY_DECISION_IDEMPOTENCY_CONFLICT")
	ErrDecisionNotAllowed = errors.New("DELIVERY_DECISION_ACTION_NOT_ALLOWED")
	ErrResultNotCurrent   = errors.New("DELIVERY_RESULT_NOT_CURRENT")
	ErrReauthorization    = errors.New("DELIVERY_REAUTHORIZATION_REQUIRED")
)

var adapterKeyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,511}$`)

func NormalizeDeliverySummary(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || len([]byte(value)) > 2048 || strings.ContainsRune(value, '\x00') {
		return "", ErrIntentInvalid
	}
	return value, nil
}

func ValidAdapterKey(value string) bool { return adapterKeyPattern.MatchString(value) }

func DigestText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
