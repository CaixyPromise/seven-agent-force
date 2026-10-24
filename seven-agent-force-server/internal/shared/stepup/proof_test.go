package stepup

import (
	"testing"

	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
)

func TestRequireRejectsWeakAssuranceOrProofMethods(t *testing.T) {
	tests := []struct {
		name  string
		proof ProofMetadata
	}{
		{name: "low assurance", proof: validProof("AAL1", []string{"TIME_BASED_ONE_TIME_PASSWORD"})},
		{name: "missing assurance", proof: validProof("", []string{"TIME_BASED_ONE_TIME_PASSWORD"})},
		{name: "missing method", proof: validProof("AAL2", nil)},
		{name: "password method", proof: validProof("AAL2", []string{"PASSWORD_VERIFICATION"})},
		{name: "captcha method", proof: validProof("AAL2", []string{"IMAGE_CAPTCHA"})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Require(tt.proof, "RBAC_ASSIGN_USER_ROLES", "user:1001|roles:1,2")
			if appErr := apperrors.From(err); appErr == nil || appErr.Code() != apperrors.CodeForbidden {
				t.Fatalf("expected weak proof rejection, got %v", err)
			}
		})
	}
}

func TestRequireAcceptsStrongProofMethods(t *testing.T) {
	for _, methods := range [][]string{
		{"TIME_BASED_ONE_TIME_PASSWORD"},
		{"TOTP"},
		{"WEBAUTHN_PASSKEY_ASSERTION"},
		{"PASSKEY"},
		{"EMAIL_ONE_TIME_PASSWORD"},
		{"EMAIL_OTP"},
	} {
		t.Run(methods[0], func(t *testing.T) {
			err := Require(validProof("AAL2", methods), "RBAC_ASSIGN_USER_ROLES", "user:1001|roles:1,2")
			if err != nil {
				t.Fatalf("expected strong proof method %v to pass: %v", methods, err)
			}
		})
	}
}

func validProof(assurance string, methods []string) ProofMetadata {
	return ProofMetadata{
		BusinessAction:        "RBAC_ASSIGN_USER_ROLES",
		OperationBinding:      "user:1001|roles:1,2",
		ProofIdentifier:       "proof-jti",
		ChallengeIdentifier:   "challenge-id",
		AssuranceLevel:        assurance,
		AuthenticationMethods: methods,
	}
}
