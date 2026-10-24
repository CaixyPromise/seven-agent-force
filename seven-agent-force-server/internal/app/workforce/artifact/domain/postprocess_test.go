package domain

import "testing"

func TestFinalizationRunTransitions(t *testing.T) {
	run := FinalizationRun{Status: FinalizationPending, RunNumber: 1, MaxRuns: 3}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	if err := run.Retry("FINALIZATION_TIMEOUT"); err != nil {
		t.Fatal(err)
	}
	if run.Status != FinalizationRetryWait || run.RunNumber != 2 {
		t.Fatalf("run=%+v", run)
	}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	if err := run.Freeze(digest('a')); err != nil {
		t.Fatal(err)
	}
	if err := run.Retry("SHOULD_NOT_REPACKAGE"); err == nil {
		t.Fatal("frozen finalization must not be repackaged")
	}
}

func TestArtifactUploadAttemptHasIndependentBudget(t *testing.T) {
	upload := ArtifactUploadAttempt{Status: UploadPending, AttemptNumber: 1, MaxAttempts: 5}
	for n := 2; n <= 5; n++ {
		if err := upload.Start(); err != nil {
			t.Fatal(err)
		}
		if err := upload.Retry("MINIO_TRANSIENT"); err != nil {
			t.Fatal(err)
		}
		if upload.AttemptNumber != n {
			t.Fatalf("attempt=%d want=%d", upload.AttemptNumber, n)
		}
	}
	if err := upload.Start(); err != nil {
		t.Fatal(err)
	}
	if err := upload.Retry("MINIO_TRANSIENT"); err == nil {
		t.Fatal("sixth upload attempt must be rejected")
	}
}

func TestFrozenSourceLossRequiresManualReview(t *testing.T) {
	if got := ResolveSourceLoss(true); got != SourceLossManualReview {
		t.Fatalf("frozen source loss=%s", got)
	}
	if got := ResolveSourceLoss(false); got != SourceLossAgentRetry {
		t.Fatalf("unfrozen source loss=%s", got)
	}
}

func digest(ch byte) string {
	b := make([]byte, 64)
	for i := range b {
		b[i] = ch
	}
	return string(b)
}
