package main

import (
	"encoding/json"
	"strings"
	"testing"

	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
)

func TestFixturePathInsideRejectsTraversalAndAbsolutePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	for _, value := range []string{"../escape.py", "nested/../../escape.py", "/tmp/escape.py", "", "\x00bad.py"} {
		if _, err := fixturePathInside(root, value); err == nil {
			t.Fatalf("fixturePathInside(%q) accepted an unsafe path", value)
		}
	}
	path, err := fixturePathInside(root, "checks/quality.py")
	if err != nil || !strings.HasSuffix(path, "checks/quality.py") {
		t.Fatalf("fixturePathInside safe path = %q, %v", path, err)
	}
}

func TestWriteFixtureWorkspaceRejectsCaseCollisionAndTraversal(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cases := []json.RawMessage{
		json.RawMessage(`[{"path":"../escape","content":"x"}]`),
		json.RawMessage(`[{"path":"Readme.md","content":"x"},{"path":"README.md","content":"y"}]`),
	}
	for _, body := range cases {
		if err := writeFixtureWorkspace(root, body); err == nil {
			t.Fatalf("writeFixtureWorkspace(%s) accepted unsafe files", body)
		}
	}
}

func TestFixtureDockerArgsKeepFixtureIsolated(t *testing.T) {
	t.Parallel()
	args := fixtureDockerArgs(
		"seven-processor-fixture-0123",
		"sha256:"+strings.Repeat("a", 64),
		strings.Repeat("b", 64),
		"/worker/fixture/processor",
		"/worker/fixture/workspace",
		"/worker/fixture/runtime",
	)
	joined := strings.Join(args, "\n")
	for _, required := range []string{
		"--network\nnone", "--read-only", "--cap-drop\nALL", "no-new-privileges",
		"--user\n10001:10001", "--entrypoint\n/usr/local/bin/seven-worker-cli",
		"dst=/opt/seven-worker/result-processors/" + strings.Repeat("b", 64) + ",readonly",
		"dst=/workspace", "dst=/run/seven-fixture",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("Fixture Docker args miss %q:\n%s", required, joined)
		}
	}
	for _, forbidden := range []string{"--publish", "/var/run/docker.sock", "worker.token", "provider", "minio"} {
		if strings.Contains(strings.ToLower(joined), strings.ToLower(forbidden)) {
			t.Fatalf("Fixture Docker args expose forbidden value %q:\n%s", forbidden, joined)
		}
	}
}

func TestFixtureDeclaresOptionalDeliveryOnlyFromManifest(t *testing.T) {
	t.Parallel()
	if fixtureDeclaresDelivery(processordomain.Manifest{}) {
		t.Fatal("empty manifest unexpectedly declared delivery")
	}
	if !fixtureDeclaresDelivery(processordomain.Manifest{Capabilities: []string{"PROCESSOR_DELIVER"}}) {
		t.Fatal("delivery capability was not recognized")
	}
	if !fixtureDeclaresDelivery(processordomain.Manifest{DeliveryPolicy: "DECLARED"}) {
		t.Fatal("declared delivery policy was not recognized")
	}
}
