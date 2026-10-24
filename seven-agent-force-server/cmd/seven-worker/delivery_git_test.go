package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

func TestCreateDeliveryCommitAndLocalBarePush(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	bare := filepath.Join(root, ".seven", "local-git", "target.git")
	if err := os.MkdirAll(filepath.Dir(bare), 0o700); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		command := exec.Command(args[0], args[1:]...)
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
		body, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, body)
		}
		return string(body)
	}
	run("git", "init", "-q", repo)
	run("git", "-C", repo, "config", "user.name", "Fixture")
	run("git", "-C", repo, "config", "user.email", "fixture@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "result.txt"), []byte("ok\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("git", "-C", repo, "add", "result.txt")
	run("git", "-C", repo, "commit", "-qm", "base")
	base, _ := controlledWorkerGit(context.Background(), repo, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(repo, "result.txt"), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("git", "-C", repo, "add", "result.txt")
	tree, _ := controlledWorkerGit(context.Background(), repo, "write-tree")
	when := time.Unix(1770000000, 0).UTC()
	message := "Seven Agent Force result\n\nTask: 2610240832\nTurn: 637649102\nResult: 1111111111111111111111111111111111111111\n"
	expected, err := deliverydomain.ComputeDeliveryCommit(tree, base, "Seven Bot", "bot@example.invalid", when, message)
	if err != nil {
		t.Fatal(err)
	}
	claim := deliveryapp.DeliveryClaim{TaskID: 2610240832, TaskTurnID: 637649102, ResultCommit: "1111111111111111111111111111111111111111", ResultTree: tree, ExpectedBaseCommit: base, CommitTimestamp: when.Format(time.RFC3339), Target: deliverydomain.Target{CommitAuthorName: "Seven Bot", CommitAuthorEmail: "bot@example.invalid"}}
	commit, err := createDeliveryCommit(context.Background(), repo, claim)
	if err != nil || commit != expected {
		t.Fatalf("commit mismatch: %s %s %v", commit, expected, err)
	}
	run("git", "init", "--bare", "-q", bare)
	run("git", "-C", repo, "push", "-q", bare, base+":refs/heads/main")
	localRef := "refs/seven/delivery/test"
	run("git", "-C", repo, "update-ref", localRef, commit)
	env := controlledDeliveryEnv(t.TempDir())
	if err = runDeliveryPush(context.Background(), repo, bare, localRef, "refs/heads/seven/test", env, true); err != nil {
		t.Fatal(err)
	}
	remote, readable := readDeliveryRemoteRef(context.Background(), bare, "refs/heads/seven/test", env, true)
	if !readable || remote != commit {
		t.Fatalf("remote ref mismatch: %s %v", remote, readable)
	}
}

func TestDeliverySSHCommandBridgesPublicDestinationThroughPinnedDockerRelay(t *testing.T) {
	t.Parallel()
	policy, err := json.Marshal(deliverydomain.EgressPolicy{AllowedHosts: []string{"ssh.github.com"}, AllowedCIDRs: []string{"0.0.0.0/0"}, AllowedPorts: []int{443}})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	command, err := deliverySSHCommand(root, deliverydomain.Target{Host: "ssh.github.com", Port: 443, Egress: policy}, filepath.Join(root, "identity"), filepath.Join(root, "known_hosts"), []net.IP{net.ParseIP("140.82.112.36")})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(command)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ProxyCommand="+filepath.Join(root, "docker-ssh-egress")) || strings.Contains(string(body), "ProxyCommand=nc") {
		t.Fatalf("unexpected public SSH wrapper: %s", body)
	}
	proxy, err := os.ReadFile(filepath.Join(root, "docker-ssh-egress"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(proxy), dockerSSHEgressBridgeImage) || !strings.Contains(string(proxy), "140.82.112.36") {
		t.Fatalf("unexpected public SSH relay: %s", proxy)
	}
}

func TestDeliveryEgressPinsApprovedLoopbackSSHDestination(t *testing.T) {
	policy, err := json.Marshal(deliverydomain.EgressPolicy{
		AllowPrivateNetwork: true,
		AllowedHosts:        []string{"gitlab.localhost"},
		AllowedCIDRs:        []string{"127.0.0.0/8"},
		AllowedPorts:        []int{2224},
	})
	if err != nil {
		t.Fatal(err)
	}
	target := deliverydomain.Target{Host: "gitlab.localhost", Port: 2224, Egress: policy}
	addresses, err := resolveDeliveryEgress(context.Background(), target, func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	})
	if err != nil || len(addresses) != 1 || !addresses[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("resolveDeliveryEgress() = %v, %v", addresses, err)
	}
	command, err := deliverySSHCommand(t.TempDir(), target, filepath.Join(t.TempDir(), "identity"), filepath.Join(t.TempDir(), "known_hosts"), addresses)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(strings.TrimPrefix(command, "sh "))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{"ProxyCommand=nc -w 10 127.0.0.1 2224", "HostKeyAlias=[gitlab.localhost]:2224", "StrictHostKeyChecking=yes"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("pinned SSH command did not contain %q: %s", expected, text)
		}
	}
	forbidden := deliverydomain.Target{Host: "gitlab.localhost", Port: 2224, Egress: json.RawMessage(`{"allowPrivateNetwork":false,"allowedHosts":["gitlab.localhost"],"allowedCidrs":["127.0.0.0/8"],"allowedPorts":[2224]}`)}
	if _, err = resolveDeliveryEgress(context.Background(), forbidden, func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }); err == nil {
		t.Fatal("expected loopback target without an explicit private-network policy to be rejected")
	}
	trailing := target
	trailing.Egress = append(append([]byte(nil), policy...), []byte(` {}`)...)
	if _, err = resolveDeliveryEgress(context.Background(), trailing, func(context.Context, string) ([]net.IP, error) { return []net.IP{net.ParseIP("127.0.0.1")}, nil }); err == nil {
		t.Fatal("expected a policy with trailing JSON to be rejected")
	}
}

func TestResolveDeliveryEgressPrefersIPv4ForDualStackLocalTarget(t *testing.T) {
	policy, err := json.Marshal(deliverydomain.EgressPolicy{
		AllowPrivateNetwork: true,
		AllowedHosts:        []string{"gitea.localhost"},
		AllowedCIDRs:        []string{"127.0.0.1/32", "::1/128"},
		AllowedPorts:        []int{2223},
	})
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := resolveDeliveryEgress(context.Background(), deliverydomain.Target{Host: "gitea.localhost", Port: 2223, Egress: policy}, func(context.Context, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("::1"), net.ParseIP("127.0.0.1")}, nil
	})
	if err != nil || len(addresses) != 2 || !addresses[0].Equal(net.ParseIP("127.0.0.1")) || !addresses[1].Equal(net.ParseIP("::1")) {
		t.Fatalf("resolveDeliveryEgress() = %v, %v", addresses, err)
	}
}
