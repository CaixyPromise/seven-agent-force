package stepup

import (
	"strings"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

const requiredPrivilegedAssuranceLevel = "AAL2"

var acceptedPrivilegedProofMethods = map[string]struct{}{
	"WEBAUTHN_PASSKEY_ASSERTION":   {},
	"PASSKEY":                      {},
	"TIME_BASED_ONE_TIME_PASSWORD": {},
	"TOTP":                         {},
	"EMAIL_ONE_TIME_PASSWORD":      {},
	"EMAIL_OTP":                    {},
}

type ProofMetadata struct {
	BusinessAction        string
	OperationBinding      string
	ProofIdentifier       string
	ChallengeIdentifier   string
	AssuranceLevel        string
	AuthenticationMethods []string
}

func (p ProofMetadata) Normalized() ProofMetadata {
	methods := make([]string, 0, len(p.AuthenticationMethods))
	for _, item := range p.AuthenticationMethods {
		if value := strings.TrimSpace(item); value != "" {
			methods = append(methods, strings.ToUpper(value))
		}
	}
	return ProofMetadata{
		BusinessAction:        strings.TrimSpace(p.BusinessAction),
		OperationBinding:      strings.TrimSpace(p.OperationBinding),
		ProofIdentifier:       strings.TrimSpace(p.ProofIdentifier),
		ChallengeIdentifier:   strings.TrimSpace(p.ChallengeIdentifier),
		AssuranceLevel:        strings.ToUpper(strings.TrimSpace(p.AssuranceLevel)),
		AuthenticationMethods: methods,
	}
}

func Require(p ProofMetadata, businessAction, operationBinding string) error {
	normalized := p.Normalized()
	expectedAction := strings.TrimSpace(businessAction)
	expectedBinding := strings.TrimSpace(operationBinding)
	if expectedAction == "" || expectedBinding == "" {
		return apperrors.System("step-up proof contract未配置")
	}
	if normalized.ProofIdentifier == "" {
		return apperrors.Forbidden("缺少step-up proof元数据")
	}
	if normalized.ChallengeIdentifier == "" {
		return apperrors.Forbidden("缺少step-up challenge元数据")
	}
	if normalized.AssuranceLevel != requiredPrivilegedAssuranceLevel {
		return apperrors.Forbidden("step-up proof保证等级不足")
	}
	if !hasAcceptedPrivilegedProofMethod(normalized.AuthenticationMethods) {
		return apperrors.Forbidden("step-up proof认证方式不足")
	}
	if normalized.BusinessAction != expectedAction {
		return apperrors.Forbidden("step-up proof业务动作不匹配")
	}
	if normalized.OperationBinding != expectedBinding {
		return apperrors.Forbidden("step-up proof操作绑定不匹配")
	}
	return nil
}

func hasAcceptedPrivilegedProofMethod(methods []string) bool {
	for _, method := range methods {
		if _, ok := acceptedPrivilegedProofMethods[strings.ToUpper(strings.TrimSpace(method))]; ok {
			return true
		}
	}
	return false
}
