package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	gitdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/gittransport"
	workspaceinputapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/application"
)

func TestGitSourceFetchProducesRestorableBundleForMultiCommitRepository(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	work := filepath.Join(root, "work")
	ingested := filepath.Join(root, "ingested")
	if _, err := controlledWorkerGit(ctx, root, "init", "--bare", "-q", remote); err != nil {
		t.Fatal(err)
	}
	if _, err := controlledWorkerGit(ctx, root, "init", "-q", work); err != nil {
		t.Fatal(err)
	}
	for index, body := range []string{"first\n", "second\n"} {
		if err := os.WriteFile(filepath.Join(work, "value.txt"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := controlledWorkerGit(ctx, work, "add", "value.txt"); err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, "git", "-C", work, "-c", "user.name=fixture", "-c", "user.email=fixture@example.test", "commit", "-q", "-m", fmt.Sprintf("commit-%d", index+1))
		command.Env = controlledSourceGitEnv(root)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("commit: %v: %s", err, output)
		}
	}
	commit, err := controlledWorkerGit(ctx, work, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := controlledWorkerGit(ctx, work, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controlledWorkerGit(ctx, work, "push", "-q", remote, "HEAD:refs/heads/main"); err != nil {
		t.Fatal(err)
	}
	if _, err = controlledWorkerGit(ctx, root, "init", "-q", ingested); err != nil {
		t.Fatal(err)
	}
	args := []string{"-C", ingested, "-c", "protocol.file.allow=always", "fetch", "--no-tags", remote, commit}
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = controlledSourceGitEnv(root)
	if output, fetchErr := command.CombinedOutput(); fetchErr != nil {
		t.Fatalf("fetch: %v: %s", fetchErr, output)
	}
	if _, err = controlledWorkerGit(ctx, ingested, "checkout", "--detach", "-q", commit); err != nil {
		t.Fatal(err)
	}
	ref := "refs/seven/sources/test"
	if _, err = controlledWorkerGit(ctx, ingested, "update-ref", ref, commit); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "source.bundle")
	command = exec.CommandContext(ctx, "git", "-C", ingested, "bundle", "create", bundle, ref)
	command.Env = controlledSourceGitEnv(root)
	if output, bundleErr := command.CombinedOutput(); bundleErr != nil {
		t.Fatalf("bundle: %v: %s", bundleErr, output)
	}
	if err = verifySourceBundle(ctx, bundle, commit, tree, root); err != nil {
		t.Fatalf("verify independently restorable bundle: %v", err)
	}
}

func TestSafeSourcePathRejectsTraversalGitAndUnicodeCollision(t *testing.T) {
	for _, value := range []string{"../secret", "/absolute", ".git/config", "src/.git/index"} {
		if _, err := safeSourcePath(value, map[string]struct{}{}); err == nil {
			t.Fatalf("safeSourcePath(%q) unexpectedly succeeded", value)
		}
	}
	seen := map[string]struct{}{}
	if _, err := safeSourcePath("Cafe\u0301.txt", seen); err != nil {
		t.Fatal(err)
	}
	if _, err := safeSourcePath("Caf\u00e9.txt", seen); err == nil || !strings.Contains(err.Error(), "COLLISION") {
		t.Fatalf("unicode collision error=%v", err)
	}
}

func TestExtractSourceZIPRejectsSymlinkAndKeepsRegularFiles(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "source.zip")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	regular, _ := writer.Create("src/main.go")
	_, _ = regular.Write([]byte("package main\n"))
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	opened, _ := os.Open(archive)
	if err = extractSourceZIP(context.Background(), opened, filepath.Join(root, "out")); err != nil {
		t.Fatal(err)
	}
	_ = opened.Close()
	if body, readErr := os.ReadFile(filepath.Join(root, "out", "src", "main.go")); readErr != nil || string(body) != "package main\n" {
		t.Fatalf("body=%q err=%v", body, readErr)
	}

	unsafe := filepath.Join(root, "unsafe.zip")
	file, _ = os.Create(unsafe)
	writer = zip.NewWriter(file)
	header := &zip.FileHeader{Name: "link"}
	header.SetMode(os.ModeSymlink | 0o777)
	entry, _ := writer.CreateHeader(header)
	_, _ = entry.Write([]byte("target"))
	_ = writer.Close()
	_ = file.Close()
	opened, _ = os.Open(unsafe)
	defer opened.Close()
	if err = extractSourceZIP(context.Background(), opened, filepath.Join(root, "unsafe-out")); err == nil {
		t.Fatal("symlink archive accepted")
	}
}

func TestExtractSourceTarRejectsLinks(t *testing.T) {
	var raw bytes.Buffer
	w := tar.NewWriter(&raw)
	_ = w.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "target", Mode: 0o777})
	_ = w.Close()
	if err := extractSourceTar(context.Background(), tar.NewReader(bytes.NewReader(raw.Bytes())), t.TempDir()); err == nil {
		t.Fatal("tar symlink accepted")
	}
}

func TestFilterUploadedProjectRemovesRegenerableAndRejectsSecrets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules", "pkg"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "pkg", "index.js"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filterUploadedProject(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "node_modules")); !os.IsNotExist(err) {
		t.Fatalf("node_modules remained: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filterUploadedProject(root); err == nil || !strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("secret error=%v", err)
	}
}

func TestSourceSSHCommandPinsResolvedDestinationAndKeepsHostKeyAlias(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	command, err := sourceSSHCommand(root, "/tmp/key", "/tmp/known_hosts", "gitea.localhost", 2223, []string{"127.0.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if command != filepath.Join(root, "pinned-ssh") {
		t.Fatalf("command=%q", command)
	}
	body, err := os.ReadFile(command)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ProxyCommand=nc -w 10 127.0.0.1 2223", "HostKeyAlias=[gitea.localhost]:2223", "StrictHostKeyChecking=yes"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
}

func TestOrderResolvedIPsPrefersIPv4ForPinnedLocalDockerEndpoint(t *testing.T) {
	actual := orderResolvedIPs([]string{"::1", "127.0.0.1", "2001:db8::1", "192.0.2.12"})
	want := []string{"127.0.0.1", "192.0.2.12", "2001:db8::1", "::1"}
	if len(actual) != len(want) {
		t.Fatalf("addresses=%v", actual)
	}
	for index := range want {
		if actual[index] != want[index] {
			t.Fatalf("addresses=%v want=%v", actual, want)
		}
	}
}

func TestSourceHTTPAuthenticationPinsValidatedAddress(t *testing.T) {
	root := t.TempDir()
	connection := gitdomain.RepositoryTarget{Transport: gitdomain.TransportHTTPS, Host: "git.example.test", Port: 443, RepositoryPath: "group/repo.git"}
	_, environment, _, cleanup, err := prepareSourceAuthenticationPinned(context.Background(), root, connection, workspaceinputapp.SourceCredential{Username: "user", Password: "token"}, []string{"203.0.113.9"})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	joined := strings.Join(environment, "\n")
	if !strings.Contains(joined, "GIT_CONFIG_KEY_0=http.curloptResolve") || !strings.Contains(joined, "GIT_CONFIG_VALUE_0=git.example.test:443:203.0.113.9") {
		t.Fatalf("Git HTTP destination is not pinned: %s", joined)
	}
}

func TestSourceSSHCommandBridgesPublicDestinationThroughPinnedDockerRelay(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	command, err := sourceSSHCommand(root, "/tmp/key", "/tmp/known_hosts", "ssh.github.com", 443, []string{"140.82.112.36"})
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
	for _, want := range []string{"--pull=never", "--read-only", "--cap-drop ALL", dockerSSHEgressBridgeImage, "140.82.112.36", "443"} {
		if !strings.Contains(string(proxy), want) {
			t.Fatalf("public SSH relay missing %q: %s", want, proxy)
		}
	}
}

func TestAppendDockerBridgeEnvironmentPreservesOnlyValidatedConfiguredEndpoint(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///tmp/seven-workforce-docker.sock")
	environment, err := appendDockerBridgeEnvironment(context.Background(), []string{"PATH=/usr/bin"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, value := range environment {
		if value == "DOCKER_HOST=unix:///tmp/seven-workforce-docker.sock" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Docker endpoint was not propagated: %v", environment)
	}
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	if _, err = appendDockerBridgeEnvironment(context.Background(), []string{"PATH=/usr/bin"}); err == nil || !strings.Contains(err.Error(), "DOCKER_CONTEXT_INVALID") {
		t.Fatalf("unsafe Docker endpoint error=%v", err)
	}
}
