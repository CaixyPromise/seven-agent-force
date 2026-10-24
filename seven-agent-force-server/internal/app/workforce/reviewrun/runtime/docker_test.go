package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
)

func TestValidateReportEvidenceReopensFrozenGitBlobAndLineBounds(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	checkout := filepath.Join(root, "checkout")
	bare := filepath.Join(root, "result.git")
	for _, args := range [][]string{{"init", checkout}, {"-C", checkout, "config", "user.email", "reviewer@example.invalid"}, {"-C", checkout, "config", "user.name", "reviewer"}} {
		if _, err := commandOutput(context.Background(), "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(checkout, "main.go"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", checkout, "add", "main.go"}, {"-C", checkout, "commit", "-m", "fixture"}, {"clone", "--bare", checkout, bare}} {
		if _, err := commandOutput(context.Background(), "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	commitRaw, err := commandOutput(context.Background(), "git", "--git-dir="+bare, "rev-parse", "HEAD^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(commitRaw))
	valid := []byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"needs evidence","findings":[{"id":"f1","severity":"WARNING","category":"CORRECTNESS","title":"issue","message":"issue","evidence":{"path":"main.go","line":3}}]}`)
	if err = validateReportEvidence(context.Background(), bare, commit, valid); err != nil {
		t.Fatalf("valid frozen evidence: %v", err)
	}
	for _, candidate := range [][]byte{
		[]byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"bad path","findings":[{"id":"f1","severity":"WARNING","category":"CORRECTNESS","title":"issue","message":"issue","evidence":{"path":"missing.go","line":1}}]}`),
		[]byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"bad line","findings":[{"id":"f1","severity":"WARNING","category":"CORRECTNESS","title":"issue","message":"issue","evidence":{"path":"main.go","line":5}}]}`),
	} {
		if err = validateReportEvidence(context.Background(), bare, commit, candidate); err == nil {
			t.Fatalf("invalid frozen evidence accepted: %s", candidate)
		}
	}
}

func TestValidateReportEvidenceRejectsGitSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	checkout := filepath.Join(root, "checkout")
	bare := filepath.Join(root, "result.git")
	for _, args := range [][]string{{"init", checkout}, {"-C", checkout, "config", "user.email", "reviewer@example.invalid"}, {"-C", checkout, "config", "user.name", "reviewer"}} {
		if _, err := commandOutput(context.Background(), "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(checkout, "outside.txt"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("outside.txt", filepath.Join(checkout, "link.txt")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", checkout, "add", "outside.txt", "link.txt"}, {"-C", checkout, "commit", "-m", "symlink fixture"}, {"clone", "--bare", checkout, bare}} {
		if _, err := commandOutput(context.Background(), "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	commitRaw, err := commandOutput(context.Background(), "git", "--git-dir="+bare, "rev-parse", "HEAD^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	commit := strings.TrimSpace(string(commitRaw))
	report := []byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"symlink evidence","findings":[{"id":"f1","severity":"WARNING","category":"SECURITY","title":"link","message":"link","evidence":{"path":"link.txt","line":1}}]}`)
	if err = validateReportEvidence(context.Background(), bare, commit, report); err == nil {
		t.Fatal("accepted symlink evidence")
	}
}

func TestLineCount(t *testing.T) {
	if got := lineCount(nil); got != 0 {
		t.Fatalf("empty lines=%d", got)
	}
	if got := lineCount([]byte("one\ntwo\n")); got != 2 {
		t.Fatalf("trailing newline lines=%d", got)
	}
}

func TestReviewRuntimeRejectsInvalidFrozenInputsBeforeDockerLaunch(t *testing.T) {
	t.Parallel()
	workspace, err := attemptruntime.OpenPersistentWorkspace(filepath.Join(t.TempDir(), "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	base := Work{
		ReviewRunID:       101,
		WorkerID:          102,
		TaskID:            103,
		TaskTurnID:        104,
		FinalizationRunID: 105,
		ResultCommit:      strings.Repeat("a", 40),
		ResultTree:        strings.Repeat("b", 64),
		ReviewerImage:     "sha256:" + strings.Repeat("c", 64),
		OwnerEpoch:        1,
		ReviewContract:    []byte(`{"version":"independent-review/v1"}`),
		Requirements:      []byte(`{"instruction":"inspect the frozen result"}`),
	}
	for _, test := range []struct {
		name string
		edit func(*Work)
	}{
		{name: "malformed review contract", edit: func(work *Work) { work.ReviewContract = []byte(`{"version":`) }},
		{name: "empty requirements", edit: func(work *Work) { work.Requirements = nil }},
		{name: "non-object requirements", edit: func(work *Work) { work.Requirements = []byte(`[]`) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			work := base
			test.edit(&work)
			dockerCalled := false
			runtime := &Docker{workspace: workspace, exec: func(context.Context, string, ...string) ([]byte, error) {
				dockerCalled = true
				return nil, nil
			}}
			if _, err := runtime.Run(context.Background(), work, Gate{}); !errors.Is(err, ErrReviewInputInvalid) {
				t.Fatalf("invalid frozen input error=%v, want %v", err, ErrReviewInputInvalid)
			}
			if dockerCalled {
				t.Fatal("invalid frozen input reached Docker before validation")
			}
		})
	}
}

func TestReviewRuntimeReattachCleansDiscoveredContainerBeforeRejectingInvalidInput(t *testing.T) {
	t.Parallel()
	workspace, err := attemptruntime.OpenPersistentWorkspace(filepath.Join(t.TempDir(), "workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	cleaned := false
	runtime := &Docker{
		workspace: workspace,
		exec: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "docker" && len(args) >= 2 && args[0] == "rm" && args[1] == "-f" {
				cleaned = true
			}
			return nil, nil
		},
	}
	work := Work{
		ReviewRunID:       201,
		WorkerID:          202,
		TaskID:            203,
		TaskTurnID:        204,
		FinalizationRunID: 205,
		ResultCommit:      strings.Repeat("a", 40),
		ResultTree:        strings.Repeat("b", 64),
		ReviewerImage:     "sha256:" + strings.Repeat("c", 64),
		OwnerEpoch:        1,
		ReviewContract:    []byte(`{"version":`),
		Requirements:      []byte(`{"instruction":"inspect"}`),
	}
	container := ManagedContainer{
		Container:   Container{ID: "container-id-1234", ExecutionIdentityDigest: strings.Repeat("d", 64)},
		ReviewRunID: work.ReviewRunID,
		WorkerID:    work.WorkerID,
		OwnerEpoch:  work.OwnerEpoch,
		ImageDigest: work.ReviewerImage,
		State:       "running",
		Running:     true,
	}
	if _, err := runtime.Reattach(context.Background(), work, container); !errors.Is(err, ErrReviewInputInvalid) {
		t.Fatalf("invalid recovered input error=%v, want %v", err, ErrReviewInputInvalid)
	}
	if !cleaned {
		t.Fatal("discovered Reviewer container was not cleaned after invalid input")
	}
}

func TestReportVolumeIdentityRequiresMatchingLabelsAndName(t *testing.T) {
	t.Parallel()
	const name = "seven-review-report-101-2-0123456789abcdef0123456789abcdef"
	valid := []byte(`[{"Name":"` + name + `","Labels":{"seven.review.report":"true","seven.review.run.id":"101","seven.review.owner.epoch":"2"}}]`)
	if !reportVolumeNamed(valid, name) || !reportVolumeMatches(valid, Work{ReviewRunID: 101, OwnerEpoch: 2}, name) {
		t.Fatal("valid report volume identity rejected")
	}
	for _, invalid := range [][]byte{
		[]byte(`[{"Name":"` + name + `","Labels":{"seven.review.report":"true","seven.review.run.id":"101","seven.review.owner.epoch":"3"}}]`),
		[]byte(`[{"Name":"` + name + `","Labels":{"seven.review.report":"true","seven.review.run.id":"nope","seven.review.owner.epoch":"2"}}]`),
		[]byte(`[{"Name":"` + name + `","Labels":{"seven.review.report":"true","seven.review.run.id":"101"}}]`),
		[]byte(`[{"Name":"seven-review-report-101-2-0123456789abcdef0123456789abcdeg","Labels":{"seven.review.report":"true","seven.review.run.id":"101","seven.review.owner.epoch":"2"}}]`),
	} {
		if reportVolumeNamed(invalid, name) {
			t.Fatalf("malformed report volume identity accepted: %s", invalid)
		}
	}
	bareNonce := "0123456789abcdef0123456789abcdef"
	bareNonceVolume := []byte(`[{"Name":"` + bareNonce + `","Labels":{"seven.review.report":"true","seven.review.run.id":"101","seven.review.owner.epoch":"2"}}]`)
	if reportVolumeNamed(bareNonceVolume, bareNonce) || reportVolumeMatches(bareNonceVolume, Work{ReviewRunID: 101, OwnerEpoch: 2}, bareNonce) {
		t.Fatal("report volume without the Run/owner prefix was accepted")
	}
}

func TestReviewRuntimeRequiresExactReviewerImageDigest(t *testing.T) {
	t.Parallel()
	base := Work{
		ReviewRunID:       101,
		WorkerID:          102,
		TaskID:            103,
		TaskTurnID:        104,
		FinalizationRunID: 105,
		ResultCommit:      strings.Repeat("a", 40),
		ResultTree:        strings.Repeat("b", 64),
		ReviewerImage:     "sha256:" + strings.Repeat("c", 64),
		OwnerEpoch:        1,
	}
	if !validWork(base) {
		t.Fatal("valid exact digest rejected")
	}
	for _, digest := range []string{"sha256:abc", "sha256:" + strings.Repeat("g", 64), "sha256:" + strings.Repeat("c", 63), "sha256:" + strings.Repeat("c", 65)} {
		candidate := base
		candidate.ReviewerImage = digest
		if validWork(candidate) {
			t.Fatalf("non-exact reviewer digest accepted: %q", digest)
		}
	}
}
