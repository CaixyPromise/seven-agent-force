package infrastructure

import (
	"strings"
	"testing"

	continuationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
)

func TestCorrectionInstructionKeepsOnlyCurrentReviewAuthority(t *testing.T) {
	t.Parallel()
	firstDigest := strings.Repeat("a", 64)
	secondDigest := strings.Repeat("b", 64)
	first := correctionInstruction("Implement the original requirement.", continuationfacade.ReviewRevisionTurnRequest{ReportDigest: firstDigest})
	second := correctionInstruction(first, continuationfacade.ReviewRevisionTurnRequest{ReportDigest: secondDigest})
	if strings.Count(second, "[System-managed independent review correction]") != 1 {
		t.Fatalf("derived instruction accumulated correction blocks: %q", second)
	}
	if strings.Contains(second, firstDigest) || !strings.Contains(second, secondDigest) {
		t.Fatalf("derived instruction retained stale report authority: %q", second)
	}
	if strings.Count(second, continuationfacade.ReviewReportPathPlaceholder) != 1 {
		t.Fatalf("derived instruction path placeholder count is not one: %q", second)
	}
}

func TestCorrectionInstructionSeparatesAuditReasonFromRevisionGuidance(t *testing.T) {
	t.Parallel()
	digest := strings.Repeat("c", 64)
	instruction := correctionInstruction("Implement the original requirement.", continuationfacade.ReviewRevisionTurnRequest{
		RevisionKind:     continuationfacade.RevisionHumanAuthorized,
		ReportDigest:     digest,
		RevisionGuidance: "Keep the API compatible and fix only the pagination boundary.",
	})
	if !strings.Contains(instruction, "Keep the API compatible") {
		t.Fatalf("revision guidance missing from correction instruction: %q", instruction)
	}
	if strings.Contains(instruction, "approval reason") {
		t.Fatalf("audit reason leaked into correction instruction: %q", instruction)
	}

	withoutReport := correctionInstruction("Implement the original requirement.", continuationfacade.ReviewRevisionTurnRequest{
		RevisionKind:     continuationfacade.RevisionHumanAuthorized,
		SafeReasonCode:   "INDEPENDENT_REVIEW_REPORT_INVALID",
		RevisionGuidance: "Reproduce the pagination failure and preserve existing response fields.",
	})
	if !strings.Contains(withoutReport, "INDEPENDENT_REVIEW_REPORT_INVALID") || !strings.Contains(withoutReport, "Reproduce the pagination failure") {
		t.Fatalf("safe fallback correction is incomplete: %q", withoutReport)
	}
}
