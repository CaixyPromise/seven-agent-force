// Package runtime executes frozen checks without inheriting Agent authority.
package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
)

// Shell is the image-side executor. Production callers must put it in the
// isolated check container; a clean environment alone is not a sandbox.
// Generic exit zero proves command success, without claiming tests ran.
func Shell(ctx context.Context, input domain.Input, repository string, timeout time.Duration) domain.Observation {
	input, err := domain.Normalize(input)
	if err != nil {
		return domain.Observation{Status: domain.Unavailable, ReasonCode: "CHECK_INPUT_INVALID"}
	}
	digest, _ := domain.Digest(input)
	out := domain.Observation{CheckDigest: digest, Status: domain.Unavailable}
	if input.Mode == "NONE" {
		out.Status = domain.NotConfigured
		out.ReasonCode = "CHECK_NOT_CONFIGURED"
		return out
	}
	if timeout <= 0 || timeout > 10*time.Minute {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	script := input.Command
	if input.Mode == "SCRIPT" {
		script = input.Script
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = time.Second
	cmd.Dir = repository
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/tmp", "TMPDIR=/tmp", "LANG=C", "LC_ALL=C"}
	capture := &boundedOutput{}
	stderrCapture := &boundedOutput{}
	stdoutDiagnostic, stderrDiagnostic := newDiagnosticOutput(), newDiagnosticOutput()
	cmd.Stdout = io.MultiWriter(capture, stdoutDiagnostic)
	cmd.Stderr = io.MultiWriter(stderrCapture, stderrDiagnostic)
	err = cmd.Run()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
		code := cmd.ProcessState.ExitCode()
		out.ExitCode = &code
	}
	out.DiagnosticTruncated = stdoutDiagnostic.truncated || stderrDiagnostic.truncated
	if out.ExitCode != nil && !out.DiagnosticTruncated && ctx.Err() == nil {
		raw, _ := json.Marshal(struct {
			Exit                     int
			StdoutHash, StderrHash   string
			StdoutBytes, StderrBytes int64
		}{*out.ExitCode, stdoutDiagnostic.digest(), stderrDiagnostic.digest(), stdoutDiagnostic.bytes, stderrDiagnostic.bytes})
		sum := sha256.Sum256(raw)
		out.DiagnosticDigest = hex.EncodeToString(sum[:])
		out.DiagnosticKnown = true
	}
	if ctx.Err() != nil {
		out.Status = domain.TimedOut
		out.ReasonCode = "CHECK_TIMED_OUT"
		return out
	}
	evidenceStatus, count, known := tapEvidence(capture.body(), capture.truncated)
	out.TestCount, out.TestCountKnown = count, known
	if err != nil {
		out.ReasonCode = "CHECK_COMMAND_UNAVAILABLE"
		diagnostic := strings.ToLower(capture.body() + "\n" + stderrCapture.body())
		for _, marker := range []string{"err_pnpm_no_script", "missing script:", "cannot find module", "modulenotfounderror", "econnrefused", "could not connect to server"} {
			if strings.Contains(diagnostic, marker) {
				// A recognized missing prerequisite is a decision, not an
				// instruction to repeatedly rewrite unrelated project code.
				return out
			}
		}
		if strings.Contains(diagnostic, "no tests found") || strings.Contains(diagnostic, "no test files found") {
			out.Status, out.ReasonCode = domain.NoTests, "CHECK_NO_TESTS"
			return out
		}
		if evidenceStatus == domain.NoTests {
			out.Status = domain.NoTests
			out.ReasonCode = "CHECK_NO_TESTS"
			return out
		}
		if e, ok := err.(*exec.ExitError); ok && e.ExitCode() != 126 && e.ExitCode() != 127 {
			out.Status = domain.Failed
			out.ReasonCode = "CHECK_COMMAND_FAILED"
		}
		return out
	}
	out.Status = evidenceStatus
	switch out.Status {
	case domain.Passed:
		out.ReasonCode = "CHECK_COMMAND_SUCCEEDED"
	case domain.NoTests:
		out.ReasonCode = "CHECK_NO_TESTS"
	case domain.Failed:
		out.ReasonCode = "CHECK_TEST_FAILED"
	default:
		out.ReasonCode = "CHECK_TEST_EVIDENCE_INCOMPLETE"
	}
	return out
}

type boundedOutput struct {
	mu        sync.Mutex
	value     []byte
	truncated bool
}

// Only hashes and bounded byte counts are retained for stderr. Stream labels
// are hashed separately so a stdout/stderr swap cannot reuse old evidence.
type diagnosticOutput struct {
	mu        sync.Mutex
	sum       hash.Hash
	bytes     int64
	truncated bool
}

func newDiagnosticOutput() *diagnosticOutput { return &diagnosticOutput{sum: sha256.New()} }
func (d *diagnosticOutput) Write(p []byte) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	n := len(p)
	remaining := int64(64<<10) - d.bytes
	if int64(n) > remaining {
		d.truncated = true
		p = p[:remaining]
	}
	_, _ = d.sum.Write(p)
	d.bytes += int64(len(p))
	return n, nil
}
func (d *diagnosticOutput) digest() string { return hex.EncodeToString(d.sum.Sum(nil)) }

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	remaining := (64 << 10) - len(b.value)
	if n > remaining {
		b.truncated = true
		p = p[:remaining]
	}
	b.value = append(b.value, p...)
	return n, nil
}
func (b *boundedOutput) body() string { b.mu.Lock(); defer b.mu.Unlock(); return string(b.value) }

var planRE = regexp.MustCompile(`^1\.\.([0-9]+)(?:\s+#.*)?$`)
var testRE = regexp.MustCompile(`^(not ok|ok)\s+([0-9]+)(?:\s|$)`)
var zeroTestsRE = regexp.MustCompile(`(?m)^(?:no tests ran in [0-9.]+s|No tests found(?:, exiting with code [01])?)\s*$`)

func tapEvidence(body string, truncated bool) (domain.Status, int, bool) {
	if truncated {
		// A captured failure remains evidence even when later output exceeds
		// the bound. An apparently successful prefix cannot establish the
		// omitted suffix, so truncation never produces a passing observation.
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Bail out!") {
				return domain.Failed, 0, false
			}
			if m := testRE.FindStringSubmatch(line); m != nil && m[1] == "not ok" {
				upper := strings.ToUpper(line)
				if !strings.Contains(upper, "# SKIP") && !strings.Contains(upper, "# TODO") {
					return domain.Failed, 0, false
				}
			}
		}
		return domain.Unavailable, 0, false
	}
	plan := -1
	seen := map[int]bool{}
	failed := false
	executed := 0
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Bail out!") {
			return domain.Failed, 0, false
		}
		if m := planRE.FindStringSubmatch(line); m != nil {
			if plan >= 0 {
				return domain.Unavailable, 0, false
			}
			plan, _ = strconv.Atoi(m[1])
			continue
		}
		if m := testRE.FindStringSubmatch(line); m != nil {
			id, _ := strconv.Atoi(m[2])
			if id <= 0 || seen[id] {
				return domain.Unavailable, 0, false
			}
			seen[id] = true
			upper := strings.ToUpper(line)
			if strings.Contains(upper, "# SKIP") || strings.Contains(upper, "# TODO") {
				continue
			}
			executed++
			failed = failed || m[1] == "not ok"
		}
	}
	if plan < 0 {
		if zeroTestsRE.MatchString(body) {
			return domain.NoTests, 0, true
		}
		lines := strings.Split(strings.TrimSpace(body), "\n")
		goNoTests := strings.TrimSpace(body) != ""
		for _, line := range lines {
			goNoTests = goNoTests && strings.HasPrefix(strings.TrimSpace(line), "?") && strings.HasSuffix(strings.TrimSpace(line), "[no test files]")
		}
		if goNoTests {
			return domain.NoTests, 0, true
		}
		if failed {
			return domain.Failed, 0, false
		}
		return domain.Passed, 0, false
	}
	if len(seen) != plan {
		return domain.Unavailable, 0, false
	}
	for id := range seen {
		if id > plan {
			return domain.Unavailable, 0, false
		}
	}
	if failed {
		return domain.Failed, executed, true
	}
	if executed == 0 {
		return domain.NoTests, 0, true
	}
	return domain.Passed, executed, true
}
