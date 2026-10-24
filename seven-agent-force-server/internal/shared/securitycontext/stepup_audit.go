package securitycontext

import (
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

const stepUpProofAuditContextKey = "__seven_step_up_proof_audit__"

type StepUpProofAudit struct {
	BusinessAction        string
	OperationBinding      string
	ProofIdentifier       string
	ChallengeIdentifier   string
	AssuranceLevel        string
	AuthenticationMethods []string
	ProofToken            string
}

func SetStepUpProofAudit(reqCtx *app.RequestContext, audit StepUpProofAudit) {
	if reqCtx == nil {
		return
	}
	reqCtx.Set(stepUpProofAuditContextKey, normalizeStepUpProofAudit(audit))
}

func GetStepUpProofAudit(reqCtx *app.RequestContext) (StepUpProofAudit, bool) {
	if reqCtx == nil {
		return StepUpProofAudit{}, false
	}
	value, ok := reqCtx.Get(stepUpProofAuditContextKey)
	if !ok || value == nil {
		return StepUpProofAudit{}, false
	}
	audit, ok := value.(StepUpProofAudit)
	if !ok {
		return StepUpProofAudit{}, false
	}
	if strings.TrimSpace(audit.BusinessAction) == "" && strings.TrimSpace(audit.OperationBinding) == "" && strings.TrimSpace(audit.ProofIdentifier) == "" {
		return StepUpProofAudit{}, false
	}
	return audit, true
}

func StepUpProofAuditMetadata(reqCtx *app.RequestContext) (map[string]any, bool) {
	audit, ok := GetStepUpProofAudit(reqCtx)
	if !ok {
		return nil, false
	}
	metadata := map[string]any{}
	if audit.BusinessAction != "" {
		metadata["businessAction"] = audit.BusinessAction
	}
	if audit.OperationBinding != "" {
		metadata["operationBinding"] = audit.OperationBinding
	}
	if audit.ProofIdentifier != "" {
		metadata["proofIdentifier"] = audit.ProofIdentifier
	}
	if audit.ChallengeIdentifier != "" {
		metadata["challengeIdentifier"] = audit.ChallengeIdentifier
	}
	if audit.AssuranceLevel != "" {
		metadata["assuranceLevel"] = audit.AssuranceLevel
	}
	if len(audit.AuthenticationMethods) > 0 {
		metadata["authenticationMethods"] = append([]string(nil), audit.AuthenticationMethods...)
	}
	if len(metadata) == 0 {
		return nil, false
	}
	return metadata, true
}

func normalizeStepUpProofAudit(audit StepUpProofAudit) StepUpProofAudit {
	methods := make([]string, 0, len(audit.AuthenticationMethods))
	for _, item := range audit.AuthenticationMethods {
		if value := strings.TrimSpace(item); value != "" {
			methods = append(methods, value)
		}
	}
	return StepUpProofAudit{
		BusinessAction:        strings.TrimSpace(audit.BusinessAction),
		OperationBinding:      strings.TrimSpace(audit.OperationBinding),
		ProofIdentifier:       strings.TrimSpace(audit.ProofIdentifier),
		ChallengeIdentifier:   strings.TrimSpace(audit.ChallengeIdentifier),
		AssuranceLevel:        strings.TrimSpace(audit.AssuranceLevel),
		AuthenticationMethods: methods,
	}
}
