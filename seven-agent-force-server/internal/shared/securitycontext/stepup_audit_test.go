package securitycontext

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
)

func TestStepUpProofAuditMetadataRoundTripExcludesProofToken(t *testing.T) {
	reqCtx := &app.RequestContext{}
	SetStepUpProofAudit(reqCtx, StepUpProofAudit{
		BusinessAction:        "RBAC_ASSIGN_USER_ROLES",
		OperationBinding:      "user:1001|roles:1,2",
		ProofIdentifier:       "proof-jti-1",
		ChallengeIdentifier:   "challenge-1",
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
		ProofToken:            "raw-proof-token",
	})

	metadata, ok := StepUpProofAuditMetadata(reqCtx)
	if !ok {
		t.Fatal("expected step-up proof audit metadata")
	}
	if metadata["businessAction"] != "RBAC_ASSIGN_USER_ROLES" {
		t.Fatalf("unexpected action metadata: %#v", metadata)
	}
	if metadata["operationBinding"] != "user:1001|roles:1,2" {
		t.Fatalf("unexpected binding metadata: %#v", metadata)
	}
	if metadata["proofIdentifier"] != "proof-jti-1" {
		t.Fatalf("unexpected proof identifier: %#v", metadata)
	}
	if metadata["challengeIdentifier"] != "challenge-1" {
		t.Fatalf("unexpected challenge identifier: %#v", metadata)
	}
	if metadata["proofToken"] != nil {
		t.Fatalf("proof token must not be exposed in audit metadata: %#v", metadata)
	}
}

func TestStepUpProofAuditMetadataDoesNotFallbackProofIdentifierToChallenge(t *testing.T) {
	reqCtx := &app.RequestContext{}
	SetStepUpProofAudit(reqCtx, StepUpProofAudit{
		BusinessAction:      "CONFIG_REVEAL_SENSITIVE",
		OperationBinding:    "config:10|reveal",
		ChallengeIdentifier: "challenge-1",
	})

	metadata, ok := StepUpProofAuditMetadata(reqCtx)
	if !ok {
		t.Fatal("expected step-up proof audit metadata")
	}
	if metadata["proofIdentifier"] != nil {
		t.Fatalf("proofIdentifier must not fallback to challengeIdentifier: %#v", metadata)
	}
	if metadata["challengeIdentifier"] != "challenge-1" {
		t.Fatalf("unexpected challengeIdentifier: %#v", metadata)
	}
}
