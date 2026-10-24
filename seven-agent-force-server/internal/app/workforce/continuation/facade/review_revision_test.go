package facade

import "testing"

func TestPublicInstructionOmitsManagedCorrectionContract(t *testing.T) {
	const original = "Fix the pagination boundary."
	managed := original + ManagedCorrectionMarker + "Use " + ReviewReportPathPlaceholder + " with an internal digest."
	if got := PublicInstruction(managed); got != original {
		t.Fatalf("public instruction = %q", got)
	}
	if got := PublicInstruction("  " + original + "  "); got != original {
		t.Fatalf("plain public instruction = %q", got)
	}
}
