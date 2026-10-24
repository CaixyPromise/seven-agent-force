package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
	workspaceinputapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
)

// TestGitHubPrivateSourceIngestionSSHAcceptance exercises the production SSH
// code path with the separate read deploy key held by the collaboration live
// lab.  On Docker Desktop the Worker pins the GitHub address and uses its
// narrowly-scoped Docker TCP bridge; the host never receives a HTTPS Git
// fallback or a copied review API token.
func TestGitHubPrivateSourceIngestionSSHAcceptance(t *testing.T) {
	secretsRoot := strings.TrimSpace(os.Getenv("SEVEN_COLLAB_LIVE_LAB_SECRETS"))
	repositoryName := strings.Trim(strings.TrimSpace(os.Getenv("SEVEN_GITHUB_ACCEPTANCE_REPOSITORY")), "/")
	if secretsRoot == "" || repositoryName == "" {
		t.Skip("GitHub SSH collaboration live-lab environment is not configured")
	}
	if strings.Count(repositoryName, "/") != 1 {
		t.Fatal("invalid GitHub acceptance repository identity")
	}
	privateKey, err := os.ReadFile(filepath.Join(secretsRoot, "github", "read-v2"))
	if err != nil {
		t.Fatalf("read deploy key is unavailable: %v", err)
	}
	knownHosts, err := os.ReadFile(filepath.Join(secretsRoot, "github", "known_hosts"))
	if err != nil {
		t.Fatalf("known_hosts is unavailable: %v", err)
	}
	fingerprintOutput, err := exec.Command("ssh-keygen", "-lf", filepath.Join(secretsRoot, "github", "known_hosts")).Output()
	if err != nil {
		t.Fatal("known_hosts fingerprint is unavailable")
	}
	fingerprint := ""
	for _, field := range strings.Fields(string(fingerprintOutput)) {
		if strings.HasPrefix(field, "SHA256:") {
			fingerprint = field
			break
		}
	}
	if fingerprint == "" {
		t.Fatal("known_hosts fingerprint is invalid")
	}

	ctx := context.Background()
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", "ssh.github.com")
	if err != nil || len(addresses) == 0 {
		t.Fatal("GitHub SSH DNS is unavailable")
	}
	resolved := make([]string, 0, len(addresses))
	for _, address := range addresses {
		resolved = append(resolved, address.String())
	}
	root := t.TempDir()
	connection := gitdomain.RepositoryTarget{
		ProviderType: gitdomain.ProviderGitHub, Transport: gitdomain.TransportSSH,
		Host: "ssh.github.com", Port: 443, RepositoryPath: repositoryName + ".git",
		WebRepositoryURL: "https://github.com/" + repositoryName, SSHKnownHostFingerprint: fingerprint,
	}
	remote, environment, allowFile, cleanup, err := prepareSourceAuthenticationPinned(ctx, root, connection, workspaceinputapp.SourceCredential{PrivateKey: string(privateKey), KnownHosts: string(knownHosts)}, resolved)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if allowFile || strings.Contains(remote, "http") || strings.Contains(strings.Join(environment, "\n"), string(privateKey)) {
		t.Fatal("SSH source authentication leaked credential or enabled an unsafe transport")
	}
	refCommand := exec.CommandContext(ctx, "git", "-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "submodule.recurse=false", "ls-remote", "--refs", remote, "refs/heads/main")
	refCommand.Env = environment
	refOutput, refErr := refCommand.CombinedOutput()
	if refErr != nil {
		diagnostic := strings.ReplaceAll(string(refOutput), string(privateKey), "[redacted-private-key]")
		diagnostic = strings.ReplaceAll(diagnostic, string(knownHosts), "[redacted-known-hosts]")
		if len(diagnostic) > 1024 {
			diagnostic = diagnostic[:1024]
		}
		t.Fatalf("private GitHub main ref is unreadable through the pinned SSH bridge: %s", strings.TrimSpace(diagnostic))
	}
	fields := strings.Fields(string(refOutput))
	if len(fields) != 2 || len(fields[0]) != 40 {
		t.Fatal("private GitHub main ref response is malformed")
	}
	commit := fields[0]
	repository := filepath.Join(root, "repository")
	if err = os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err = controlledWorkerGit(ctx, repository, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "git", "-C", repository, "-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "submodule.recurse=false", "fetch", "--no-tags", "--depth=1", remote, commit)
	command.Env = environment
	if output, fetchErr := command.CombinedOutput(); fetchErr != nil {
		zeroSourceAcceptanceOutput(output)
		t.Fatal("private GitHub exact commit SSH fetch failed")
	}
	if _, err = controlledWorkerGit(ctx, repository, "checkout", "--detach", "-q", commit); err != nil {
		t.Fatal(err)
	}
	if err = rejectUnsupportedRepository(repository); err != nil {
		t.Fatal(err)
	}
}

// TestGitHubPrivateSourceIngestionAcceptance is opt-in and verifies the actual
// Worker source credential materialization, exact-ref fetch and standalone
// repository bundle path against a private GitHub repository.
func TestGitHubPrivateSourceIngestionAcceptance(t *testing.T) {
	repositoryName := strings.Trim(strings.TrimSpace(os.Getenv("SEVEN_GITHUB_ACCEPTANCE_REPOSITORY")), "/")
	token := strings.TrimSpace(os.Getenv("SEVEN_GITHUB_ACCEPTANCE_TOKEN"))
	if repositoryName == "" || token == "" {
		t.Skip("real GitHub source-ingestion acceptance environment is not configured")
	}
	if strings.Count(repositoryName, "/") != 1 {
		t.Fatal("invalid GitHub acceptance repository identity")
	}

	ctx := context.Background()
	root := t.TempDir()
	connection := gitdomain.RepositoryTarget{
		ProviderType: gitdomain.ProviderGitHub, Transport: gitdomain.TransportHTTPS,
		Host: "github.com", Port: 443, RepositoryPath: repositoryName + ".git",
	}
	remote, environment, allowFile, cleanup, err := prepareSourceAuthentication(ctx, root, connection, workspaceinputapp.SourceCredential{Username: "x-access-token", Password: token})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if allowFile || strings.Contains(remote, token) {
		t.Fatal("private source remote leaked credential or enabled file protocol")
	}
	for _, name := range []string{"username", "password"} {
		info, statErr := os.Stat(filepath.Join(root, "credentials", name))
		if statErr != nil {
			t.Fatalf("credential %s unavailable: %v", name, statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("credential %s permissions=%v", name, info.Mode().Perm())
		}
	}

	commit, readable := readDeliveryRemoteRef(ctx, remote, "refs/heads/main", environment, false)
	if !readable || len(commit) != 40 {
		t.Fatal("private source main ref is unreadable")
	}
	repository := filepath.Join(root, "repository")
	if err = os.MkdirAll(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err = controlledWorkerGit(ctx, repository, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(ctx, "git", "-C", repository, "-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "submodule.recurse=false", "fetch", "--no-tags", "--depth=1", remote, commit)
	command.Env = environment
	if output, fetchErr := command.CombinedOutput(); fetchErr != nil {
		zeroSourceAcceptanceOutput(output)
		t.Fatal("private source exact commit fetch failed")
	}
	if _, err = controlledWorkerGit(ctx, repository, "checkout", "--detach", "-q", commit); err != nil {
		t.Fatal(err)
	}
	if err = rejectUnsupportedRepository(repository); err != nil {
		t.Fatal(err)
	}
	tree, err := controlledWorkerGit(ctx, repository, "rev-parse", "HEAD^{tree}")
	if err != nil || len(tree) != 40 {
		t.Fatalf("source tree invalid: %q err=%v", tree, err)
	}
	ref := "refs/seven/sources/github-private-acceptance"
	if _, err = controlledWorkerGit(ctx, repository, "update-ref", ref, commit); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "repository.bundle")
	bundleCommand := exec.CommandContext(ctx, "git", "-C", repository, "bundle", "create", bundle, ref)
	bundleCommand.Env = controlledSourceGitEnv(root)
	if output, bundleErr := bundleCommand.CombinedOutput(); bundleErr != nil {
		zeroSourceAcceptanceOutput(output)
		t.Fatal("private source bundle creation failed")
	}
	if err = verifySourceBundle(ctx, bundle, commit, tree, root); err != nil {
		t.Fatalf("private source standalone bundle verification failed: %v", err)
	}
}

func zeroSourceAcceptanceOutput(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
