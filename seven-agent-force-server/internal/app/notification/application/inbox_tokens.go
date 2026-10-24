package application

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

const inboxTokenTTL = 24 * time.Hour

type inboxTokenPayload struct {
	Version        int    `json:"v"`
	Kind           string `json:"k"`
	ScopeID        string `json:"s"`
	UserID         int64  `json:"u"`
	Archived       bool   `json:"a,omitempty"`
	CreateTime     string `json:"c,omitempty"`
	RecipientID    string `json:"r,omitempty"`
	RecipientRowID int64  `json:"i,omitempty"`
	ChangeSequence int64  `json:"q,omitempty"`
	ExpiresAt      int64  `json:"e"`
}

type inboxTokenEnvelope struct {
	CiphertextB64 string `json:"c"`
	EDEKB64       string `json:"e"`
	WrapKeyRef    string `json:"k"`
}

func (s *Service) encodeInboxToken(ctx context.Context, payload inboxTokenPayload) (string, error) {
	if s == nil || s.secrets == nil {
		return "", fmt.Errorf("notification inbox token codec is not configured")
	}
	payload.Version = 1
	if payload.ExpiresAt <= 0 {
		payload.ExpiresAt = s.now().Add(inboxTokenTTL).UTC().Unix()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	value, err := s.secrets.EncryptString(ctx, string(raw))
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value.CiphertextB64) == "" || strings.TrimSpace(value.EDEKB64) == "" || strings.TrimSpace(value.WrapKeyRef) == "" {
		return "", fmt.Errorf("notification inbox token encryption returned an incomplete envelope")
	}
	envelope, err := json.Marshal(inboxTokenEnvelope{
		CiphertextB64: value.CiphertextB64,
		EDEKB64:       value.EDEKB64,
		WrapKeyRef:    value.WrapKeyRef,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(envelope), nil
}

func (s *Service) decodeInboxToken(ctx context.Context, raw, expectedKind, scopeID string, userID int64) (inboxTokenPayload, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return inboxTokenPayload{}, fmt.Errorf("notification inbox token is empty")
	}
	if s == nil || s.secrets == nil {
		return inboxTokenPayload{}, fmt.Errorf("notification inbox token codec is not configured")
	}
	encoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return inboxTokenPayload{}, err
	}
	var envelope inboxTokenEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return inboxTokenPayload{}, err
	}
	plain, err := s.secrets.DecryptString(ctx, secretvalueinfra.SecretValue{
		CiphertextB64: envelope.CiphertextB64,
		EDEKB64:       envelope.EDEKB64,
		WrapKeyRef:    envelope.WrapKeyRef,
	})
	if err != nil {
		return inboxTokenPayload{}, err
	}
	var payload inboxTokenPayload
	if err := json.Unmarshal([]byte(plain), &payload); err != nil {
		return inboxTokenPayload{}, err
	}
	if payload.Version != 1 || payload.Kind != expectedKind || payload.ScopeID != scopeID || payload.UserID != userID || payload.ExpiresAt <= s.now().UTC().Unix() {
		return inboxTokenPayload{}, fmt.Errorf("notification inbox token is invalid or expired")
	}
	return payload, nil
}

func (s *Service) encodeInboxPageCursor(ctx context.Context, scopeID string, userID int64, archived bool, createTime time.Time, recipientID string, rowID int64) (string, error) {
	return s.encodeInboxToken(ctx, inboxTokenPayload{
		Kind:           "page",
		ScopeID:        scopeID,
		UserID:         userID,
		Archived:       archived,
		CreateTime:     createTime.UTC().Format(time.RFC3339Nano),
		RecipientID:    strings.TrimSpace(recipientID),
		RecipientRowID: rowID,
	})
}

func (s *Service) decodeInboxPageCursor(ctx context.Context, raw, scopeID string, userID int64, archived bool) (time.Time, int64, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, 0, nil
	}
	payload, err := s.decodeInboxToken(ctx, raw, "page", scopeID, userID)
	if err != nil || payload.Archived != archived || payload.RecipientRowID <= 0 || strings.TrimSpace(payload.RecipientID) == "" {
		return time.Time{}, 0, invalidInboxPageCursor()
	}
	createTime, err := time.Parse(time.RFC3339Nano, payload.CreateTime)
	if err != nil {
		return time.Time{}, 0, invalidInboxPageCursor()
	}
	return createTime.UTC(), payload.RecipientRowID, nil
}

func (s *Service) encodeInboxChangeToken(ctx context.Context, scopeID string, userID, changeSequence int64) (string, error) {
	if changeSequence < 0 {
		return "", fmt.Errorf("notification inbox change sequence is invalid")
	}
	return s.encodeInboxToken(ctx, inboxTokenPayload{
		Kind:           "change",
		ScopeID:        scopeID,
		UserID:         userID,
		ChangeSequence: changeSequence,
	})
}

func (s *Service) decodeInboxChangeToken(ctx context.Context, raw, scopeID string, userID int64) (int64, error) {
	payload, err := s.decodeInboxToken(ctx, raw, "change", scopeID, userID)
	if err != nil || payload.ChangeSequence < 0 {
		return 0, fmt.Errorf("notification inbox change token is invalid")
	}
	return payload.ChangeSequence, nil
}

func invalidInboxPageCursor() error {
	return apperrors.ParamsWithDetails("分页游标无效", map[string]string{"reasonCode": "INVALID_PAGE_CURSOR"})
}
