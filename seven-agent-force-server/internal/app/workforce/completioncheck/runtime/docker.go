package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

type Request struct {
	Input                                                     domain.Input
	Digest, Phase, Repository, BaseCommit, Image, PrivateRoot string
	Timeout                                                   time.Duration
	Labels                                                    map[string]string
	InputDigest                                               string
	References                                                []facade.Reference
}
type Runner struct {
	Command resourcejournal.Command
	Journal *resourcejournal.Journal
}

func (r *Runner) BindResourceJournal(j *resourcejournal.Journal) { r.Journal = j }

var imageRE = regexp.MustCompile(`^(?:sha256:|[^\s]+@sha256:)[a-f0-9]{64}$`)
var commitRE = regexp.MustCompile(`^[a-f0-9]{40}(?:[a-f0-9]{24})?$`)

// Run must finish BASELINE before any Harness start. CURRENT runs only after
// the Harness stops. Caller persists the returned observation with fenced
// attempt/cycle authority before continuing evaluation.
func (r *Runner) Run(ctx context.Context, q Request) (out domain.Observation, err error) {
	out = domain.Observation{CheckDigest: q.Digest, Status: domain.Unavailable}
	defer func() {
		if err != nil && out.ReasonCode == "" {
			out.ReasonCode = "CHECK_RUNTIME_UNAVAILABLE"
		}
	}()
	input, e := domain.Normalize(q.Input)
	if e != nil {
		return out, errors.New("CHECK_INPUT_INVALID")
	}
	digest, _ := domain.Digest(input)
	inputDigest := q.InputDigest
	if inputDigest == "" && len(input.ReferenceAssetIDs) == 0 {
		inputDigest = q.Digest
	}
	frozenDigest, freezeErr := facade.FrozenDigest(input, q.References)
	if digest != inputDigest || freezeErr != nil || frozenDigest != q.Digest {
		return out, errors.New("CHECK_DIGEST_MISMATCH")
	}
	if input.Mode == "NONE" {
		out.Status = domain.NotConfigured
		return out, nil
	}
	if strings.ContainsAny(q.Repository+q.PrivateRoot, ",\n\r") {
		return out, errors.New("CHECK_MOUNT_PATH_INVALID")
	}
	if r.Journal == nil || !imageRE.MatchString(q.Image) || !filepath.IsAbs(q.Repository) || !filepath.IsAbs(q.PrivateRoot) || (q.Phase != "BASELINE" && q.Phase != "CURRENT") {
		return out, errors.New("CHECK_RUNTIME_INVALID")
	}
	if q.Timeout <= 0 || q.Timeout > 10*time.Minute {
		q.Timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, q.Timeout+10*time.Second)
	defer cancel()
	root, e := os.MkdirTemp(q.PrivateRoot, "completion-check-")
	if e != nil {
		return out, errors.New("CHECK_PRIVATE_ROOT_UNAVAILABLE")
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return out, errors.New("CHECK_PRIVATE_ROOT_INVALID")
	}
	resourceLabels := map[string]string{"seven.completion.check": "true"}
	for k, v := range q.Labels {
		resourceLabels[k] = v
	}
	resource, e := r.Journal.TrackDirectory(root, resourceLabels)
	if e != nil {
		return out, errors.New("CHECK_DIRECTORY_JOURNAL_FAILED")
	}
	defer func() { _ = r.Journal.Cleanup(resource.Key, nil) }()
	repository := q.Repository
	if q.Phase == "BASELINE" {
		if !commitRE.MatchString(q.BaseCommit) {
			return out, errors.New("CHECK_BASE_COMMIT_INVALID")
		}
		repository = filepath.Join(root, "repository")
		for _, args := range [][]string{{"-c", "core.hooksPath=/dev/null", "clone", "--no-local", "--no-checkout", "--", q.Repository, repository}, {"-C", repository, "-c", "core.hooksPath=/dev/null", "checkout", "--detach", q.BaseCommit}} {
			cmd := exec.CommandContext(ctx, "git", args...)
			cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0"}
			if cmd.Run() != nil {
				return out, errors.New("CHECK_BASELINE_UNAVAILABLE")
			}
		}
	}
	inputPath := filepath.Join(root, "input.json")
	sourcePath := filepath.Join(root, "source")
	if e := Snapshot(ctx, repository, sourcePath); e != nil {
		return out, e
	}
	if e := os.Chmod(sourcePath, 0755); e != nil {
		return out, errors.New("CHECK_SOURCE_PERMISSIONS_FAILED")
	}
	workspaceDigest, e := SnapshotDigest(ctx, sourcePath)
	if e != nil {
		return out, e
	}
	out.WorkspaceDigest = workspaceDigest
	definitionDigest, e := checkDefinitionDigest(sourcePath, input)
	if e != nil {
		return out, e
	}
	out.DefinitionDigest = definitionDigest
	body, _ := json.Marshal(input)
	if os.WriteFile(inputPath, body, 0444) != nil {
		return out, errors.New("CHECK_INPUT_WRITE_FAILED")
	}
	command := r.Command
	if command == nil {
		command = safeCommand
	}
	run := func(c context.Context, args ...string) (string, error) {
		return r.Journal.Execute(c, command, "docker", args...)
	}
	name := filepath.Base(root)
	args := []string{"create", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--cpus", "1", "--user", "65534:65534", "--tmpfs", "/tmp:rw,nosuid,nodev,size=134217728", "--tmpfs", "/workspace:rw,nosuid,nodev,size=536870912,uid=65534,gid=65534,mode=0700", "--mount", "type=bind,src=" + sourcePath + ",dst=/check/source,readonly", "--mount", "type=bind,src=" + inputPath + ",dst=/check/input.json,readonly"}
	checkArgs := []string{"-i", "PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/tmp", "/usr/local/bin/seven-worker-cli", "check", "--input", "/check/input.json", "--source", "/check/source", "--repository", "/workspace"}
	if q.Phase == "CURRENT" {
		dependencies := filepath.Join(repository, "node_modules")
		if info, statErr := os.Lstat(dependencies); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			args = append(args, "--mount", "type=bind,src="+dependencies+",dst=/check/dependencies/node_modules,readonly")
			checkArgs = append(checkArgs, "--dependencies", "/check/dependencies/node_modules")
		}
	}
	args = append(args, "--workdir", "/workspace", "--entrypoint", "/usr/bin/env", q.Image)
	args = append(args, checkArgs...)
	args = append(args, "--timeout", q.Timeout.String())
	keys := make([]string, 0, len(q.Labels))
	for key := range q.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	labels := []string{}
	labels = append(labels, "--label", "seven.completion.check=true")
	for _, key := range keys {
		labels = append(labels, "--label", key+"="+q.Labels[key])
	}
	args = append(append([]string{"create"}, labels...), args[1:]...)
	id, e := run(ctx, args...)
	if e != nil {
		return out, errors.New("CHECK_CONTAINER_CREATE_FAILED")
	}
	id = strings.TrimSpace(id)
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = run(c, "rm", "-f", id)
		// Failed cleanup remains in the journal/capacity fence. Preserve the
		// completed observation and mounted files for exact-resource retry.
	}()
	if _, e = run(ctx, "start", id); e != nil {
		return out, errors.New("CHECK_CONTAINER_START_FAILED")
	}
	exit, e := run(ctx, "wait", id)
	if e != nil {
		if ctx.Err() != nil {
			out.Status = domain.TimedOut
		}
		return out, errors.New("CHECK_CONTAINER_WAIT_FAILED")
	}
	if strings.TrimSpace(exit) != "0" {
		return out, errors.New("CHECK_EXECUTOR_FAILED")
	}
	raw, e := run(ctx, "logs", id)
	if e != nil || len(raw) > 4096 {
		return out, errors.New("CHECK_RESULT_UNAVAILABLE")
	}
	var result domain.Observation
	if json.Unmarshal([]byte(raw), &result) != nil || result.CheckDigest != digest || len(result.Failures) != 0 {
		return out, errors.New("CHECK_RESULT_INVALID")
	}
	if result.TestCount < 0 || (!result.TestCountKnown && result.TestCount != 0) || (result.Status == domain.Passed && result.TestCountKnown && result.TestCount == 0) {
		return out, errors.New("CHECK_TEST_EVIDENCE_INVALID")
	}
	if result.DiagnosticKnown && (result.DiagnosticTruncated || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(result.DiagnosticDigest) || result.ExitCode == nil) {
		return out, errors.New("CHECK_DIAGNOSTIC_INVALID")
	}
	if !result.DiagnosticKnown && result.DiagnosticDigest != "" {
		return out, errors.New("CHECK_DIAGNOSTIC_INVALID")
	}
	switch result.Status {
	case domain.Passed, domain.Failed, domain.Unavailable, domain.NoTests, domain.NotConfigured, domain.TimedOut:
	default:
		return out, errors.New("CHECK_RESULT_INVALID")
	}
	// The shell proves execution of the input definition, while the trusted
	// Worker binds the result to the Server-frozen reference identities. This
	// binding does not assert that reference bytes were read by the command.
	result.CheckDigest = frozenDigest
	result.DefinitionDigest = definitionDigest
	result.WorkspaceDigest = workspaceDigest
	return result, nil
}
func safeCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	capture := &boundedOutput{}
	cmd.Stdout = capture
	cmd.Stderr = capture
	err := cmd.Run()
	if capture.truncated {
		return "", errors.New("CHECK_COMMAND_OUTPUT_LIMIT")
	}
	return capture.body(), err
}
