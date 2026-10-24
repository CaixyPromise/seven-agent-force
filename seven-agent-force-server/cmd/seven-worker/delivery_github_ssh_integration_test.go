package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

// TestGitHubPrivateDeliverySSHAcceptance proves the Worker-side, non-force
// SSH transport used by Delivery.  It deliberately uses the distinct push-v2
// deploy key: the read key and the GitHub review token cannot be substituted
// for it. The branch is retained as private acceptance evidence.
func TestGitHubPrivateDeliverySSHAcceptance(t *testing.T) {
	secretsRoot := strings.TrimSpace(os.Getenv("SEVEN_COLLAB_LIVE_LAB_SECRETS"))
	repositoryName := strings.Trim(strings.TrimSpace(os.Getenv("SEVEN_GITHUB_ACCEPTANCE_REPOSITORY")), "/")
	if secretsRoot == "" || repositoryName == "" {
		t.Skip("GitHub SSH collaboration live-lab environment is not configured")
	}
	key, err := os.ReadFile(filepath.Join(secretsRoot, "github", "push-v2"))
	if err != nil {
		t.Fatalf("push deploy key is unavailable: %v", err)
	}
	known, err := os.ReadFile(filepath.Join(secretsRoot, "github", "known_hosts"))
	if err != nil {
		t.Fatalf("known_hosts is unavailable: %v", err)
	}
	fingerprints, err := exec.Command("ssh-keygen", "-lf", filepath.Join(secretsRoot, "github", "known_hosts")).Output()
	if err != nil {
		t.Fatal("known_hosts fingerprint is unavailable")
	}
	fingerprint := ""
	for _, value := range strings.Fields(string(fingerprints)) {
		if strings.HasPrefix(value, "SHA256:") {
			fingerprint = value
			break
		}
	}
	if fingerprint == "" {
		t.Fatal("known_hosts fingerprint is invalid")
	}
	policy, err := json.Marshal(deliverydomain.EgressPolicy{AllowedHosts: []string{"ssh.github.com"}, AllowedCIDRs: []string{"0.0.0.0/0", "::/0"}, AllowedPorts: []int{443}})
	if err != nil {
		t.Fatal(err)
	}
	target := deliverydomain.Target{Transport: deliverydomain.TransportSSH, Host: "ssh.github.com", Port: 443, RepositoryPath: repositoryName + ".git", SSHKnownHostFingerprint: fingerprint, Egress: policy}
	ctx := context.Background()
	addresses, err := resolveDeliveryEgress(ctx, target, func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	})
	if err != nil || len(addresses) == 0 {
		t.Fatalf("GitHub SSH egress resolution failed: %v", err)
	}
	root := t.TempDir()
	identity, knownPath := filepath.Join(root, "identity"), filepath.Join(root, "known_hosts")
	if err = os.WriteFile(identity, key, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(knownPath, known, 0o600); err != nil {
		t.Fatal(err)
	}
	sshCommand, err := deliverySSHCommand(root, target, identity, knownPath, addresses)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := appendDockerBridgeEnvironment(ctx, controlledDeliveryEnv(root))
	if err != nil {
		t.Fatal(err)
	}
	environment = append(environment, "GIT_SSH_COMMAND="+sshCommand)
	remote := deliverySSHRemote(target)
	checkout := filepath.Join(root, "checkout")
	clone := exec.CommandContext(ctx, "git", "clone", "--quiet", "--no-tags", "--branch", "main", "--", remote, checkout)
	clone.Env = environment
	if output, cloneErr := clone.CombinedOutput(); cloneErr != nil {
		zeroBytes(output)
		t.Fatal("private GitHub SSH delivery clone failed")
	}
	branchHash := sha256.Sum256([]byte(t.Name() + time.Now().UTC().Format(time.RFC3339Nano)))
	branch := "seven/ssh-delivery-" + hex.EncodeToString(branchHash[:6])
	if _, err = controlledWorkerGit(ctx, checkout, "checkout", "-q", "-b", branch); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(checkout, "WORKFORCE_SSH_DELIVERY_ACCEPTANCE.md"), []byte("SSH Delivery acceptance\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = controlledWorkerGit(ctx, checkout, "add", "--", "WORKFORCE_SSH_DELIVERY_ACCEPTANCE.md"); err != nil {
		t.Fatal(err)
	}
	commit := exec.CommandContext(ctx, "git", "-C", checkout, "-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "Verify SSH delivery path")
	commit.Env = append(append([]string(nil), environment...), "GIT_AUTHOR_NAME=Seven Agent Force Acceptance", "GIT_AUTHOR_EMAIL=seven-workforce-acceptance@localhost", "GIT_COMMITTER_NAME=Seven Agent Force Acceptance", "GIT_COMMITTER_EMAIL=seven-workforce-acceptance@localhost")
	if output, commitErr := commit.CombinedOutput(); commitErr != nil {
		zeroBytes(output)
		t.Fatal("private GitHub SSH delivery commit failed")
	}
	head, err := controlledWorkerGit(ctx, checkout, "rev-parse", "HEAD")
	if err != nil || len(head) != 40 {
		t.Fatalf("delivery commit identity is unavailable: %v", err)
	}
	if err = runDeliveryPush(ctx, checkout, remote, "HEAD", "refs/heads/"+branch, environment, false); err != nil {
		t.Fatal("private GitHub SSH delivery push failed")
	}
	remoteHead, readable := readDeliveryRemoteRef(ctx, remote, "refs/heads/"+branch, environment, false)
	if !readable || remoteHead != head {
		t.Fatalf("SSH delivery remote ref did not converge: remote=%q readable=%t", remoteHead, readable)
	}
}
