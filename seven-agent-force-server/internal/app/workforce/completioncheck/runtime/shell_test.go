package runtime

import (
	"context"
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTruncatedStdoutCannotPromoteCheckToPassed(t *testing.T) {
	for _, tc := range []struct {
		name, prefix string
		quiet, noisy domain.Status
	}{
		{"failure", "1..1\\nnot ok 1 - regression\\n", domain.Failed, domain.Failed},
		{"bailout", "Bail out! regression\\n", domain.Failed, domain.Failed},
		{"positive-prefix", "1..1\\nok 1\\n", domain.Passed, domain.Unavailable},
		{"custom-output", "custom command output\\n", domain.Passed, domain.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := domain.Input{Mode: "SCRIPT", Script: "printf '" + tc.prefix + "'; if test -f noisy; then head -c 70000 /dev/zero; fi; exit 0"}
			repository := t.TempDir()
			quiet := Shell(context.Background(), input, repository, time.Second)
			if err := os.WriteFile(filepath.Join(repository, "noisy"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			noisy := Shell(context.Background(), input, repository, time.Second)
			if quiet.Status != tc.quiet || noisy.Status != tc.noisy {
				t.Fatalf("quiet=%s noisy=%s; want %s/%s", quiet.Status, noisy.Status, tc.quiet, tc.noisy)
			}
			if quiet.CheckDigest != noisy.CheckDigest {
				t.Fatal("regression must use identical frozen command")
			}
			if noisy.ExitCode == nil || *noisy.ExitCode != 0 {
				t.Fatal("regression requires exit zero")
			}
			if !noisy.DiagnosticTruncated || noisy.DiagnosticKnown || noisy.DiagnosticDigest != "" || noisy.TestCountKnown {
				t.Fatal("truncated evidence asserted complete")
			}
			if noisy.Status == domain.Failed && noisy.ReasonCode != "CHECK_TEST_FAILED" {
				t.Fatal("captured failure lost its reason")
			}
		})
	}
}

func TestActualShellEvidence(t *testing.T) {
	t.Setenv("PRIVATE_PROVIDER_KEY", "never-expose-this")
	for _, tc := range []struct {
		name, script string
		status       domain.Status
	}{
		{"tap", "printf '1..1\\nok 1 - actual\\n'", domain.Passed},
		{"custom-check", "true", domain.Passed},
		{"zero", "printf '1..0\\n'", domain.NoTests},
		{"pytest-zero", "printf 'no tests ran in 0.01s\\n'; exit 5", domain.NoTests},
		{"go-zero", "printf '? example/module [no test files]\\n'", domain.NoTests},
		{"skip", "printf '1..1\\nok 1 # SKIP unavailable\\n'", domain.NoTests},
		{"failure", "printf '1..1\\nnot ok 1\\n'; exit 1", domain.Failed},
		{"missing", "exec /definitely/missing/check", domain.Unavailable},
		{"partial", "printf '1..2\\nok 1\\n'", domain.Unavailable},
		{"no-secret", "test -z \"$PRIVATE_PROVIDER_KEY\" && printf '1..1\\nok 1\\n'", domain.Passed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := Shell(context.Background(), domain.Input{Mode: "SCRIPT", Script: tc.script}, t.TempDir(), time.Second)
			if result.Status != tc.status {
				t.Fatalf("got %s want %s", result.Status, tc.status)
			}
			if tc.name == "custom-check" && (result.TestCountKnown || result.TestCount != 0) {
				t.Fatal("invented test count")
			}
			if tc.name == "tap" && (!result.TestCountKnown || result.TestCount != 1) {
				t.Fatal("lost positive test evidence")
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "never-expose") {
				t.Fatal("raw secret leaked")
			}
		})
	}
}
func TestActualShellTimeoutAndReferences(t *testing.T) {
	result := Shell(context.Background(), domain.Input{Mode: "COMMAND", Command: "sleep 30 & wait"}, t.TempDir(), 20*time.Millisecond)
	if result.Status != domain.TimedOut {
		t.Fatal(result)
	}
	result = Shell(context.Background(), domain.Input{Mode: "COMMAND", Command: "printf '1..1\\nok 1\\n'", ReferenceAssetIDs: []int64{1}}, t.TempDir(), time.Second)
	if result.Status != domain.Passed {
		t.Fatal("optional reference metadata blocked independent command")
	}
}

func TestFailureDiagnosticBindsBothStreamsAndTruncation(t *testing.T) {
	run := func(script string) domain.Observation {
		return Shell(context.Background(), domain.Input{Mode: "SCRIPT", Script: script}, t.TempDir(), time.Second)
	}
	first := run("printf public; printf private-failure >&2; exit 1")
	same := run("printf public; printf private-failure >&2; exit 1")
	stderrChanged := run("printf public; printf different-private-failure >&2; exit 1")
	stdoutChanged := run("printf changed; printf private-failure >&2; exit 1")
	if !first.DiagnosticKnown || first.DiagnosticDigest == "" || first.DiagnosticDigest != same.DiagnosticDigest || first.DiagnosticDigest == stderrChanged.DiagnosticDigest || first.DiagnosticDigest == stdoutChanged.DiagnosticDigest {
		t.Fatal("diagnostic identity does not bind output")
	}
	raw, _ := json.Marshal(first)
	if strings.Contains(string(raw), "private-failure") || strings.Contains(string(raw), "public") {
		t.Fatal("raw diagnostic persisted")
	}
	large := run("head -c 70000 /dev/zero >&2; exit 1")
	if !large.DiagnosticTruncated || large.DiagnosticKnown || large.DiagnosticDigest != "" {
		t.Fatal("truncated diagnostic remained reusable")
	}
	timeout := run("sleep 5")
	if timeout.DiagnosticKnown || timeout.DiagnosticDigest != "" {
		t.Fatal("timeout diagnostic remained reusable")
	}
}
