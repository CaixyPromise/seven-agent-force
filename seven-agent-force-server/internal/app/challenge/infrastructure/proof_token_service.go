package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/challenge/facade"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	jwtinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/jwt"
	"github.com/google/uuid"
)

type ProofTokenService struct {
	jwt      *jwtinfra.Service
	sessions domain.ChallengeSessionRepository
	minTTL   time.Duration
	maxTTL   time.Duration
}

func NewProofTokenService(jwt *jwtinfra.Service, sessions domain.ChallengeSessionRepository, minTTL, maxTTL time.Duration) *ProofTokenService {
	return &ProofTokenService{
		jwt:      jwt,
		sessions: sessions,
		minTTL:   minTTL,
		maxTTL:   maxTTL,
	}
}

func (s *ProofTokenService) Issue(ctx context.Context, session *domain.ChallengeSession) (*facade.ProofTokenClaims, string, error) {
	if s == nil || s.jwt == nil || session == nil {
		return nil, "", fmt.Errorf("challenge proof token service is not configured")
	}
	now := time.Now().UTC()
	expiresAt := now.Add(s.minTTL)
	if s.maxTTL > 0 && expiresAt.After(now.Add(s.maxTTL)) {
		expiresAt = now.Add(s.maxTTL)
	}
	if session.ExpiresAt != nil && session.ExpiresAt.UTC().Before(expiresAt) {
		expiresAt = session.ExpiresAt.UTC()
	}
	tokenID := uuid.NewString()
	claims := &facade.ProofTokenClaims{
		IssuerServiceName:         strings.TrimSpace(session.IssuingServiceName),
		AudienceServiceNames:      append([]string(nil), session.AudienceServiceNames...),
		SubjectIdentifier:         strings.TrimSpace(session.SubjectIdentifier),
		BusinessAction:            strings.TrimSpace(session.BusinessAction),
		ChallengeIdentifier:       strings.TrimSpace(session.ChallengeIdentifier),
		FlowNonce:                 strings.TrimSpace(session.FlowNonce),
		OperationBinding:          resolveOperationBinding(session),
		AuthenticationMethodNames: append([]string(nil), session.AuthenticationMethodNames...),
		TokenUniqueIdentifier:     tokenID,
		IssuedAt:                  &now,
		ExpiresAt:                 &expiresAt,
	}
	raw, err := s.jwt.Sign(ctx, map[string]any{
		"iss":                       claims.IssuerServiceName,
		"sub":                       claims.SubjectIdentifier,
		"aud":                       claims.AudienceServiceNames,
		"jti":                       claims.TokenUniqueIdentifier,
		"iat":                       now.Unix(),
		"exp":                       expiresAt.Unix(),
		"issuerServiceName":         claims.IssuerServiceName,
		"audienceServiceNames":      claims.AudienceServiceNames,
		"subjectIdentifier":         claims.SubjectIdentifier,
		"businessAction":            claims.BusinessAction,
		"challengeIdentifier":       claims.ChallengeIdentifier,
		"flowNonce":                 claims.FlowNonce,
		"operationBinding":          claims.OperationBinding,
		"authenticationMethodNames": claims.AuthenticationMethodNames,
		"tokenUniqueIdentifier":     claims.TokenUniqueIdentifier,
		"issuedAtUnix":              now.Unix(),
		"expiresAtUnix":             expiresAt.Unix(),
	})
	if err != nil {
		return nil, "", err
	}
	return claims, raw, nil
}

func (s *ProofTokenService) Verify(ctx context.Context, request facade.ProofTokenVerifyRequest) (*facade.ProofTokenClaims, error) {
	if s == nil || s.jwt == nil {
		return nil, fmt.Errorf("challenge proof token service is not configured")
	}
	payload, err := s.jwt.Verify(ctx, strings.TrimSpace(request.ProofToken))
	if err != nil {
		return nil, invalidProofTokenError()
	}
	claims, err := parseProofClaims(payload)
	if err != nil {
		return nil, invalidProofTokenError()
	}
	if claims.BusinessAction != strings.TrimSpace(request.BusinessAction) {
		return nil, invalidProofTokenError()
	}
	if claims.FlowNonce != strings.TrimSpace(request.FlowNonce) {
		return nil, invalidProofTokenError()
	}
	if value := strings.TrimSpace(request.SubjectIdentifier); value != "" && claims.SubjectIdentifier != value {
		return nil, invalidProofTokenError()
	}
	if value := strings.TrimSpace(request.OperationBinding); value != "" {
		if claims.OperationBinding != value {
			return nil, invalidProofTokenError()
		}
	} else if strings.TrimSpace(claims.OperationBinding) != "" {
		return nil, invalidProofTokenError()
	}
	audience := strings.TrimSpace(request.AudienceServiceName)
	if !containsString(claims.AudienceServiceNames, audience) {
		return nil, invalidProofTokenError()
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Before(time.Now().UTC()) {
		return nil, invalidProofTokenError()
	}
	if request.ConsumeOnce {
		if s.sessions == nil {
			return nil, fmt.Errorf("challenge session repository is not configured")
		}
		ok, err := s.sessions.MarkProofConsumed(ctx, claims.TokenUniqueIdentifier, audience, time.Until(claims.ExpiresAt.UTC()))
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, invalidProofTokenError()
		}
	}
	return claims, nil
}

func invalidProofTokenError() error {
	return apperrors.Forbidden("step-up proof无效或已过期")
}

func resolveOperationBinding(session *domain.ChallengeSession) string {
	if session == nil || session.SessionContext == nil {
		return ""
	}
	if value := strings.TrimSpace(stringValue(session.SessionContext["operationBinding"])); value != "" {
		return value
	}
	extension, _ := session.SessionContext["extensionContext"].(map[string]any)
	return strings.TrimSpace(stringValue(extension["operationBinding"]))
}

func parseProofClaims(payload map[string]any) (*facade.ProofTokenClaims, error) {
	issuedAt, err := resolveTime(payload["issuedAtUnix"], payload["iat"])
	if err != nil {
		return nil, err
	}
	expiresAt, err := resolveTime(payload["expiresAtUnix"], payload["exp"])
	if err != nil {
		return nil, err
	}
	return &facade.ProofTokenClaims{
		IssuerServiceName:         firstNonBlank(payload["issuerServiceName"], payload["iss"]),
		AudienceServiceNames:      stringSlice(payload["audienceServiceNames"]),
		SubjectIdentifier:         firstNonBlank(payload["subjectIdentifier"], payload["sub"]),
		BusinessAction:            strings.TrimSpace(stringValue(payload["businessAction"])),
		ChallengeIdentifier:       strings.TrimSpace(stringValue(payload["challengeIdentifier"])),
		FlowNonce:                 strings.TrimSpace(stringValue(payload["flowNonce"])),
		OperationBinding:          strings.TrimSpace(stringValue(payload["operationBinding"])),
		AuthenticationMethodNames: stringSlice(payload["authenticationMethodNames"]),
		TokenUniqueIdentifier:     firstNonBlank(payload["tokenUniqueIdentifier"], payload["jti"]),
		IssuedAt:                  issuedAt,
		ExpiresAt:                 expiresAt,
	}, nil
}

func unixTime(value any) (*time.Time, error) {
	switch typed := value.(type) {
	case time.Time:
		ts := typed.UTC()
		return &ts, nil
	case *time.Time:
		if typed == nil {
			break
		}
		ts := typed.UTC()
		return &ts, nil
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			ts := time.Unix(parsed, 0).UTC()
			return &ts, nil
		}
	}
	raw := int64Value(value)
	if raw <= 0 {
		return nil, apperrors.Operation("proof token时间字段缺失")
	}
	ts := time.Unix(raw, 0).UTC()
	return &ts, nil
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if value := strings.TrimSpace(stringValue(item)); value != "" {
				result = append(result, value)
			}
		}
		return result
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	default:
		if trimmed := strings.TrimSpace(stringValue(value)); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	}
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int32:
		return int64(typed)
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case float32:
		return int64(typed)
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
			return parsed
		}
	}
	if text := strings.TrimSpace(stringValue(value)); text != "" {
		if parsed, err := strconv.ParseInt(text, 10, 64); err == nil {
			return parsed
		}
	}
	return 0
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if strings.TrimSpace(item) == target {
			return true
		}
	}
	return false
}

func firstNonBlank(values ...any) string {
	for _, value := range values {
		if text := strings.TrimSpace(stringValue(value)); text != "" {
			return text
		}
	}
	return ""
}

func resolveTime(values ...any) (*time.Time, error) {
	for _, value := range values {
		if value == nil {
			continue
		}
		if resolved, err := unixTime(value); err == nil && resolved != nil {
			return resolved, nil
		}
	}
	return nil, apperrors.Operation("proof token时间字段缺失")
}
