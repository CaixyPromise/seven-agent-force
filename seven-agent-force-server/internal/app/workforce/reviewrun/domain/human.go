package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	HumanActionAcceptRisk          = "ACCEPT_RISK"
	HumanActionAuthorizeRevision   = "AUTHORIZE_ONE_REVISION"
	HumanActionCancelTask          = "CANCEL_TASK"
	QualityIndependentReviewPassed = "INDEPENDENT_REVIEW_PASSED"
	QualityHumanAccepted           = "HUMAN_ACCEPTED"
	BusinessActionHumanDecision    = "WORKFORCE_INDEPENDENT_REVIEW_DECISION"
)

var (
	ErrHumanActionNotAllowed         = errors.New("INDEPENDENT_REVIEW_ACTION_NOT_ALLOWED")
	ErrHumanDecisionStale            = errors.New("INDEPENDENT_REVIEW_STALE")
	ErrHumanIdempotency              = errors.New("INDEPENDENT_REVIEW_IDEMPOTENCY_CONFLICT")
	ErrHumanReasonInvalid            = errors.New("INDEPENDENT_REVIEW_REASON_INVALID")
	ErrHumanGuidanceInvalid          = errors.New("INDEPENDENT_REVIEW_GUIDANCE_INVALID")
	ErrHumanGuidanceRequired         = errors.New("INDEPENDENT_REVIEW_GUIDANCE_REQUIRED")
	ErrDeterministicAuthorityInvalid = errors.New("INDEPENDENT_REVIEW_DETERMINISTIC_AUTHORITY_INVALID")
	credentialLikeReason             = regexp.MustCompile(`(?i)(authorization\s*[:=]|bearer\s+[a-z0-9._-]+|(?:api[_-]?key|token|password|secret)\s*[:=]|\b(?:sk|ghp|github_pat|xox[baprs])[-_a-z0-9]{16,})`)
)

func ValidateHumanAction(value string) bool {
	switch value {
	case HumanActionAcceptRisk, HumanActionAuthorizeRevision, HumanActionCancelTask:
		return true
	default:
		return false
	}
}

func NormalizeHumanReason(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 2000 || !utf8.ValidString(value) || credentialLikeReason.MatchString(value) {
		return "", ErrHumanReasonInvalid
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", ErrHumanReasonInvalid
		}
	}
	return value, nil
}

func NormalizeRevisionGuidance(action, value string) (string, error) {
	value = strings.TrimSpace(value)
	if action != HumanActionAuthorizeRevision {
		if value != "" {
			return "", ErrHumanGuidanceInvalid
		}
		return "", nil
	}
	if value == "" {
		return "", nil
	}
	if len(value) > 4000 || !utf8.ValidString(value) || credentialLikeReason.MatchString(value) {
		return "", ErrHumanGuidanceInvalid
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", ErrHumanGuidanceInvalid
		}
	}
	return value, nil
}

func HumanDecisionOperationBinding(runID int64, action, reason, guidance, idempotencyKey string) string {
	guidanceIdentity := "none"
	if guidance != "" {
		guidanceIdentity = sha256Hex(guidance)
	}
	return fmt.Sprintf("workforce:review-run:%d|action:%s|reason:%s|guidance:%s|idempotency:%s", runID, action, sha256Hex(reason), guidanceIdentity, sha256Hex(idempotencyKey))
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
