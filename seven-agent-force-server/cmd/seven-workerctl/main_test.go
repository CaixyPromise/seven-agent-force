package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHealth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/healthz"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	got := run([]string{"health", "--server", server.URL}, &stdout, &stderr, server.Client())
	if got != 0 {
		t.Fatalf("run() = %d, want 0; stderr=%q", got, stderr.String())
	}
	if gotBody, want := strings.TrimSpace(stdout.String()), `{"status":"ok"}`; gotBody != want {
		t.Fatalf("stdout = %q, want %q", gotBody, want)
	}
}

func TestRunHealthRejectsServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	got := run([]string{"health", "--server", server.URL}, &stdout, &stderr, server.Client())
	if got != 1 {
		t.Fatalf("run() = %d, want 1", got)
	}
	if !strings.Contains(stderr.String(), "HTTP 503") {
		t.Fatalf("stderr = %q, want HTTP status", stderr.String())
	}
}

func TestRunTaskRequiresSubcommand(t *testing.T) {
	t.Parallel()

	var stderr bytes.Buffer
	got := run([]string{"task"}, ioDiscard{}, &stderr, nil)
	if got != 2 {
		t.Fatalf("run() = %d, want 2", got)
	}
}

func TestTaskUsageNeverPrintsEnvironmentAccessToken(t *testing.T) {
	const secret = "secret-access-token-that-must-not-appear"
	t.Setenv("SEVEN_ACCESS_TOKEN", secret)

	var stderr bytes.Buffer
	got := run([]string{"task", "get", "--not-a-real-flag"}, ioDiscard{}, &stderr, nil)
	if got != 2 {
		t.Fatalf("run() = %d, want 2", got)
	}
	if strings.Contains(stderr.String(), secret) {
		t.Fatal("usage output disclosed SEVEN_ACCESS_TOKEN")
	}
}

func TestTaskUsesEnvironmentAccessTokenWithoutPuttingItInFlagDefaults(t *testing.T) {
	const secret = "environment-access-token"
	t.Setenv("SEVEN_ACCESS_TOKEN", secret)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer "+secret; got != want {
			t.Errorf("Authorization = %q, want bearer token from environment", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if got := run([]string{"task", "list", "--server", server.URL}, &stdout, &stderr, server.Client()); got != 0 {
		t.Fatalf("run() = %d, want 0; stderr=%q", got, stderr.String())
	}
}

func TestRunTaskCreateUsesProjectScopedContract(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.URL.Path, "/api/workforce/projects/9007199254740993/tasks"; got != want {
			t.Errorf("request path = %q, want %q", got, want)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["instruction"] != "Update README" || body["name"] != "README fix" || body["deliveryPreference"] != "SAVE_ONLY" {
			t.Errorf("project task additions = %#v", body)
		}
		if enabled, ok := body["reviewEnabled"].(bool); !ok || enabled {
			t.Errorf("review toggle = %#v", body["reviewEnabled"])
		}
		selection, _ := body["modelSelection"].(map[string]any)
		if selection["credentialId"] != "9007199254740997" || selection["model"] != "gpt-seven" {
			t.Errorf("model selection = %#v", selection)
		}
		if body["runtimeReleaseId"] != "9007199254740999" {
			t.Errorf("runtime release = %#v", body["runtimeReleaseId"])
		}
		for _, forbidden := range []string{"providerAlias", "providerModel", "harnessAlias", "hookSelection", "resultProcessorSelection", "repositoryUrl", "repositoryRef", "priority", "harnessTimeoutSeconds"} {
			if _, exists := body[forbidden]; exists {
				t.Errorf("project Task payload exposed infrastructure field %q: %#v", forbidden, body)
			}
		}
		check, _ := body["additionalCompletionCheck"].(map[string]any)
		if check["mode"] != "COMMAND" || check["command"] != "make test" {
			t.Errorf("completion check = %#v", check)
		}
		assets, _ := body["referenceAssetIds"].([]any)
		if len(assets) != 1 || assets[0] != "9007199254740995" {
			t.Errorf("reference assets = %#v", assets)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":"tsk_1"}}`))
	}))
	defer server.Close()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	got := run([]string{
		"task", "create", "--server", server.URL,
		"--project", "9007199254740993", "--instruction", "Update README", "--name", "README fix",
		"--model-credential", "9007199254740997", "--model", "gpt-seven",
		"--runtime-release", "9007199254740999",
		"--tag", "docs", "--reference-asset", "9007199254740995", "--completion-check-command", "make test",
		"--review-enabled=false", "--delivery-preference", "save_only",
	}, &stdout, &stderr, server.Client())
	if got != 0 {
		t.Fatalf("run() = %d, want 0; stderr=%q", got, stderr.String())
	}
}

func TestRunTaskCreateRejectsLegacyInfrastructureFlags(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	got := run([]string{"task", "create", "--project", "701", "--instruction", "work", "--provider", "legacy"}, ioDiscard{}, &stderr, nil)
	if got != 2 || !strings.Contains(stderr.String(), "flag provided but not defined") {
		t.Fatalf("legacy infrastructure flag was not rejected: code=%d stderr=%q", got, stderr.String())
	}
}

func TestRunTaskCreateRequiresPositiveProjectID(t *testing.T) {
	t.Parallel()
	var stderr bytes.Buffer
	got := run([]string{"task", "create", "--project", "0", "--model-credential", "701", "--model", "gpt-seven", "--instruction", "work"}, ioDiscard{}, &stderr, nil)
	if got != 2 || !strings.Contains(stderr.String(), "positive decimal") {
		t.Fatalf("invalid Project ID was not rejected: code=%d stderr=%q", got, stderr.String())
	}
}

func TestArtifactDownloadIsBinaryExact(t *testing.T) {
	t.Parallel()

	want := []byte{0x00, 0x01, 0x02, 0xfe, 0xff}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, expected := r.URL.Path, "/api/workforce/tasks/task-1/artifacts/artifact-1/download"; got != expected {
			t.Errorf("request path = %q, want %q", got, expected)
		}
		_, _ = w.Write(want)
	}))
	defer server.Close()

	output := filepath.Join(t.TempDir(), "artifact.bin")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	got := run([]string{"artifact", "download", "--server", server.URL, "--output", output, "task-1", "artifact-1"}, &stdout, &stderr, server.Client())
	if got != 0 {
		t.Fatalf("run() = %d, want 0; stderr=%q", got, stderr.String())
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("downloaded bytes = %x, want %x", body, want)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
