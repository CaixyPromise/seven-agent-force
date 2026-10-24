package securitycontext

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

const loginPunishmentAuditContextKey = "__seven_login_punishment_audit__"

type LoginPunishmentAudit struct {
	LoginTransactionID  string
	AccountFingerprint  string
	Outcome             string
	Code                int
	CaptchaRequired     bool
	TotpRequired        bool
	Locked              bool
	LockExpiresAt       *int64
	ChallengeIdentifier string
}

func SetLoginPunishmentAudit(reqCtx *app.RequestContext, audit LoginPunishmentAudit) {
	if reqCtx == nil {
		return
	}
	reqCtx.Set(loginPunishmentAuditContextKey, normalizeLoginPunishmentAudit(audit))
}

func GetLoginPunishmentAudit(reqCtx *app.RequestContext) (LoginPunishmentAudit, bool) {
	if reqCtx == nil {
		return LoginPunishmentAudit{}, false
	}
	value, ok := reqCtx.Get(loginPunishmentAuditContextKey)
	if !ok || value == nil {
		return LoginPunishmentAudit{}, false
	}
	audit, ok := value.(LoginPunishmentAudit)
	if !ok {
		return LoginPunishmentAudit{}, false
	}
	if strings.TrimSpace(audit.LoginTransactionID) == "" && strings.TrimSpace(audit.AccountFingerprint) == "" && strings.TrimSpace(audit.Outcome) == "" {
		return LoginPunishmentAudit{}, false
	}
	return audit, true
}

func LoginPunishmentAuditMetadata(reqCtx *app.RequestContext) (map[string]any, bool) {
	audit, ok := GetLoginPunishmentAudit(reqCtx)
	if !ok {
		return nil, false
	}
	metadata := map[string]any{}
	if audit.LoginTransactionID != "" {
		metadata["loginTransactionId"] = audit.LoginTransactionID
	}
	if audit.AccountFingerprint != "" {
		metadata["accountFingerprint"] = audit.AccountFingerprint
	}
	if audit.Outcome != "" {
		metadata["outcome"] = audit.Outcome
	}
	if audit.Code != 0 {
		metadata["code"] = audit.Code
	}
	if audit.CaptchaRequired {
		metadata["captchaRequired"] = true
	}
	if audit.TotpRequired {
		metadata["totpRequired"] = true
	}
	if audit.Locked {
		metadata["locked"] = true
	}
	if audit.LockExpiresAt != nil {
		metadata["lockExpiresAt"] = *audit.LockExpiresAt
	}
	if audit.ChallengeIdentifier != "" {
		metadata["challengeIdentifier"] = audit.ChallengeIdentifier
	}
	if len(metadata) == 0 {
		return nil, false
	}
	return metadata, true
}

func LoginAccountFingerprint(userAccount string) string {
	normalized := strings.TrimSpace(userAccount)
	if normalized == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(normalized))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeLoginPunishmentAudit(audit LoginPunishmentAudit) LoginPunishmentAudit {
	return LoginPunishmentAudit{
		LoginTransactionID:  strings.TrimSpace(audit.LoginTransactionID),
		AccountFingerprint:  strings.TrimSpace(audit.AccountFingerprint),
		Outcome:             strings.TrimSpace(audit.Outcome),
		Code:                audit.Code,
		CaptchaRequired:     audit.CaptchaRequired,
		TotpRequired:        audit.TotpRequired,
		Locked:              audit.Locked,
		LockExpiresAt:       audit.LockExpiresAt,
		ChallengeIdentifier: strings.TrimSpace(audit.ChallengeIdentifier),
	}
}
