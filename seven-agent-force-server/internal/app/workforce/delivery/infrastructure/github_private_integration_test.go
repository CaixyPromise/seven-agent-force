package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

// TestGitHubPrivateCollaborationAcceptance is an opt-in contract test. It uses
// the same controlled credential files, Git arguments and ReviewHTTPAdapter as
// production code, while keeping the token out of URLs, argv and test output.
//
// Required environment:
//
//	SEVEN_GITHUB_ACCEPTANCE_REPOSITORY=owner/private-repository
//	SEVEN_GITHUB_ACCEPTANCE_TOKEN=<token>
//	SEVEN_GITHUB_ACCEPTANCE_BRANCH=seven/acceptance-unique-id
//
// The test intentionally leaves the pushed branch and draft PR in the private
// acceptance repository so the remote facts can be audited after the run.
func TestGitHubPrivateCollaborationAcceptance(t *testing.T) {
	repositoryName := strings.Trim(strings.TrimSpace(os.Getenv("SEVEN_GITHUB_ACCEPTANCE_REPOSITORY")), "/")
	token := strings.TrimSpace(os.Getenv("SEVEN_GITHUB_ACCEPTANCE_TOKEN"))
	branch := strings.TrimSpace(os.Getenv("SEVEN_GITHUB_ACCEPTANCE_BRANCH"))
	if repositoryName == "" || token == "" || branch == "" {
		t.Skip("real GitHub acceptance environment is not configured")
	}
	if strings.Count(repositoryName, "/") != 1 || !validAcceptanceBranch(branch) {
		t.Fatal("invalid GitHub acceptance repository or branch identity")
	}

	root := t.TempDir()
	target := domain.Target{
		Transport: domain.TransportHTTPS, Host: "github.com", Port: 443,
		RepositoryPath: repositoryName + ".git",
	}
	remote, gitEnvironment, err := prepareGitAuthentication(root, target, application.DeliveryCredential{Username: "x-access-token", Password: token})
	if err != nil {
		t.Fatal(err)
	}

	checkout := filepath.Join(root, "checkout")
	if output, cloneErr := runAcceptanceGit(root, gitEnvironment, append(controlledGitArgs(), "clone", "--quiet", "--no-tags", "--branch", "main", "--", remote, checkout)...); cloneErr != nil {
		zeroOutput(output)
		t.Fatal("private repository clone failed")
	}
	baseCommit := acceptanceGitOutput(t, checkout, gitEnvironment, "rev-parse", "HEAD")
	if output, checkoutErr := runAcceptanceGit(root, gitEnvironment, "-C", checkout, "checkout", "-q", "-b", branch); checkoutErr != nil {
		zeroOutput(output)
		t.Fatal("acceptance branch creation failed")
	}
	content := []byte("Seven Agent Force private collaboration acceptance\nbranch=" + branch + "\n")
	if err = os.WriteFile(filepath.Join(checkout, "WORKFORCE_ACCEPTANCE.md"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, addErr := runAcceptanceGit(root, gitEnvironment, "-C", checkout, "add", "--", "WORKFORCE_ACCEPTANCE.md"); addErr != nil {
		zeroOutput(output)
		t.Fatal("acceptance content staging failed")
	}
	commitEnvironment := append(append([]string(nil), gitEnvironment...),
		"GIT_AUTHOR_NAME=Seven Agent Force Acceptance", "GIT_AUTHOR_EMAIL=seven-workforce-acceptance@localhost",
		"GIT_COMMITTER_NAME=Seven Agent Force Acceptance", "GIT_COMMITTER_EMAIL=seven-workforce-acceptance@localhost",
	)
	if output, commitErr := runAcceptanceGit(root, commitEnvironment, "-C", checkout, "commit", "-q", "-m", "Verify private Git collaboration contract"); commitErr != nil {
		zeroOutput(output)
		t.Fatal("acceptance commit creation failed")
	}
	headCommit := acceptanceGitOutput(t, checkout, gitEnvironment, "rev-parse", "HEAD")
	if output, pushErr := runAcceptanceGit(root, gitEnvironment, append(controlledGitArgs(), "-C", checkout, "push", "--porcelain", "--", remote, "HEAD:refs/heads/"+branch)...); pushErr != nil {
		zeroOutput(output)
		t.Fatal("private repository push failed")
	}

	policy, _ := json.Marshal(map[string]any{
		"allowPrivateNetwork": false,
		"allowedHosts":        []string{"api.github.com"},
		"allowedPorts":        []int{443},
		"allowedCidrs":        []string{},
	})
	markerSource := sha256.Sum256([]byte(repositoryName + "\x00" + branch + "\x00" + headCommit))
	work := application.ReviewWork{
		ProjectID: 4270437003, ProviderType: "GITHUB",
		Transport: "HTTPS", Host: "github.com", Port: 443, RepositoryPath: repositoryName, WebRepositoryURL: "https://github.com/" + repositoryName, EgressPolicy: policy,
		TaskID: 3723850349, TaskTurnID: 2249902353,
		HeadBranch: branch, BaseBranch: "main", HeadCommit: headCommit,
		FrozenBaseCommit: baseCommit, MarkerDigest: hex.EncodeToString(markerSource[:]),
	}
	adapter := NewReviewHTTPAdapter()
	credential := application.DeliveryCredential{Password: token}
	first, err := adapter.PublishDraft(context.Background(), work, credential)
	if err != nil {
		t.Fatalf("draft PR publication failed: %v", err)
	}
	second, err := adapter.PublishDraft(context.Background(), work, credential)
	if err != nil || second.ProviderReviewID != first.ProviderReviewID || second.ProviderNumber != first.ProviderNumber {
		t.Fatalf("draft PR idempotency failed: first=%+v second=%+v err=%v", first, second, err)
	}
	if first.WebURL == "" || first.ObservedBase != baseCommit {
		t.Fatalf("incomplete remote review evidence: %+v", first)
	}
}

func runAcceptanceGit(root string, environment []string, args ...string) ([]byte, error) {
	command := exec.Command("git", args...)
	command.Dir = root
	command.Env = environment
	return command.CombinedOutput()
}

func acceptanceGitOutput(t *testing.T, repository string, environment []string, args ...string) string {
	t.Helper()
	output, err := runAcceptanceGit(repository, environment, append([]string{"-C", repository}, args...)...)
	if err != nil {
		zeroOutput(output)
		t.Fatal("Git identity read failed")
	}
	return strings.TrimSpace(string(output))
}

func validAcceptanceBranch(value string) bool {
	return strings.HasPrefix(value, "seven/acceptance-") && len(value) <= 120 && !strings.ContainsAny(value, " ~^:?*[\\\x00\r\n") && !strings.Contains(value, "..")
}

func zeroOutput(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
