package main

// Exact-image Result Processor Fixture execution. Fixtures are authoring-time
// evidence: they use a short-lived, hardened container and an isolated
// workspace, never an Attempt workspace, Provider credential, Docker socket,
// or a secret materialization.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	processorinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
)

const (
	fixtureRootInContainer    = "/workspace"
	fixtureRuntimeInContainer = "/run/seven-fixture"
)

type fixtureWorkspaceFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w *worker) runProcessorFixture(ctx context.Context, message processorinfra.FixtureMessage) error {
	claim, err := w.control.ClaimProcessorFixture(ctx, message.RunID, message.Generation)
	if err != nil {
		return err
	}
	if claim.Generation != message.Generation || claim.RunID != message.RunID || claim.HarnessImageDigest == "" || len(claim.RevisionDigest) != 64 {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_IDENTITY_INVALID", Diagnostics: "fixture execution contract does not match its queue message"})
	}
	if err = w.assertFixtureImage(ctx, claim.HarnessImageDigest); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_IMAGE_UNAVAILABLE", Diagnostics: fixtureDiagnostic(err.Error())})
	}

	root := filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "fixtures", fixturePathSegment(message.RunID))
	if err = os.MkdirAll(root, 0o700); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_SETUP_FAILED", Diagnostics: fixtureDiagnostic(err.Error())})
	}
	defer os.RemoveAll(root)
	processorRoot := filepath.Join(root, "processor")
	workspaceRoot := filepath.Join(root, "workspace")
	runtimeRoot := filepath.Join(root, "runtime")
	if err = w.writeFixtureProcessor(processorRoot, claim); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_SOURCE_INVALID", Diagnostics: fixtureDiagnostic(err.Error())})
	}
	if err = writeFixtureWorkspace(workspaceRoot, claim.WorkspaceFiles); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_WORKSPACE_INVALID", Diagnostics: fixtureDiagnostic(err.Error())})
	}
	if err = os.MkdirAll(runtimeRoot, 0o777); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_SETUP_FAILED", Diagnostics: fixtureDiagnostic(err.Error())})
	}
	// The image is non-root uid 10001. The fixture directories contain only
	// caller-provided non-secret source/context and live only for this one
	// container, so this permission bridges the host/container uid boundary
	// without granting it access to any Worker-owned runtime directory.
	if err = os.Chmod(workspaceRoot, 0o777); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_SETUP_FAILED", Diagnostics: fixtureDiagnostic(err.Error())})
	}
	if err = os.Chmod(runtimeRoot, 0o777); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_SETUP_FAILED", Diagnostics: fixtureDiagnostic(err.Error())})
	}
	fixtureSpec := map[string]any{
		"schemaVersion":     1,
		"runId":             strconv.FormatInt(claim.RunID, 10),
		"revisionDigest":    claim.RevisionDigest,
		"processorRoot":     filepath.Join("/opt/seven-worker/result-processors", claim.RevisionDigest),
		"workspaceRoot":     fixtureRootInContainer,
		"runtimeRoot":       fixtureRuntimeInContainer,
		"resultPath":        filepath.Join(fixtureRuntimeInContainer, "result.json"),
		"evaluationContext": json.RawMessage(claim.EvaluationContext),
		"configOverrides":   json.RawMessage(claim.ConfigOverrides),
		"runDeliver":        fixtureDeclaresDelivery(claim.Manifest),
	}
	specBody, err := json.Marshal(fixtureSpec)
	if err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_SETUP_FAILED", Diagnostics: fixtureDiagnostic(err.Error())})
	}
	if err = os.WriteFile(filepath.Join(runtimeRoot, "spec.json"), specBody, 0o666); err != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_SETUP_FAILED", Diagnostics: fixtureDiagnostic(err.Error())})
	}

	output, runErr := runFixtureContainer(ctx, fixtureContainerName(message.RunID), claim.HarnessImageDigest, claim.RevisionDigest, processorRoot, workspaceRoot, runtimeRoot)
	if runErr != nil {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_EXECUTION_FAILED", Diagnostics: fixtureDiagnostic(string(output))})
	}
	contract, readErr := os.ReadFile(filepath.Join(runtimeRoot, "result.json"))
	if readErr != nil || len(contract) == 0 || len(contract) > 1<<20 || !json.Valid(contract) {
		return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{ReasonCode: "RESULT_PROCESSOR_FIXTURE_CONTRACT_MISSING", Diagnostics: fixtureDiagnostic(errorText(readErr, "fixture Result Contract is unavailable"))})
	}
	sum := sha256.Sum256(contract)
	return w.completeProcessorFixture(ctx, message, processorfacade.FixtureResult{Contract: contract, ContractDigest: hex.EncodeToString(sum[:]), Diagnostics: fixtureDiagnostic(string(output))})
}

func (w *worker) completeProcessorFixture(ctx context.Context, message processorinfra.FixtureMessage, result processorfacade.FixtureResult) error {
	return w.control.CompleteProcessorFixture(ctx, message.RunID, message.Generation, result)
}

func fixtureDeclaresDelivery(manifest processordomain.Manifest) bool {
	if manifest.DeliveryPolicy == "DECLARED" {
		return true
	}
	for _, capability := range manifest.Capabilities {
		if capability == "PROCESSOR_DELIVER" {
			return true
		}
	}
	return false
}

func (w *worker) assertFixtureImage(ctx context.Context, imageDigest string) error {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(checkCtx, "docker", "image", "inspect", "--format", "{{.Id}}", imageDigest).Output()
	if err != nil || strings.TrimSpace(string(output)) != imageDigest {
		return errors.New("exact fixture Harness image digest is not present")
	}
	return nil
}

func (w *worker) writeFixtureProcessor(root string, claim processorfacade.FixtureExecution) error {
	validation := processordomain.Validate(claim.Manifest, claim.Files)
	if !validation.Valid {
		return errors.New("fixture Processor Revision failed local validation")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for _, file := range claim.Files {
		path, err := fixturePathInside(root, file.Path)
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err = os.WriteFile(path, []byte(file.Content), 0o444); err != nil {
			return err
		}
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("fixture Processor source contains symlink")
		}
		return nil
	})
}

func writeFixtureWorkspace(root string, body json.RawMessage) error {
	var files []fixtureWorkspaceFile
	if err := json.Unmarshal(body, &files); err != nil || len(files) > 256 {
		return errors.New("fixture workspace files are invalid")
	}
	if err := os.MkdirAll(root, 0o777); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(files))
	total := 0
	for _, file := range files {
		path, err := fixturePathInside(root, file.Path)
		if err != nil || len(file.Content) > 2<<20 {
			return errors.New("fixture workspace file is invalid")
		}
		rel, _ := filepath.Rel(root, path)
		folded := strings.ToLower(filepath.ToSlash(rel))
		if _, exists := seen[folded]; exists {
			return errors.New("fixture workspace file path collides")
		}
		seen[folded] = struct{}{}
		total += len(file.Content)
		if total > 8<<20 {
			return errors.New("fixture workspace exceeds bounded size")
		}
		if err = os.MkdirAll(filepath.Dir(path), 0o777); err != nil {
			return err
		}
		if err = os.WriteFile(path, []byte(file.Content), 0o666); err != nil {
			return err
		}
	}
	return nil
}

func fixturePathInside(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" || strings.ContainsRune(relative, 0) {
		return "", errors.New("fixture path is invalid")
	}
	path := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return "", errors.New("fixture path escapes workspace")
	}
	return path, nil
}

func runFixtureContainer(ctx context.Context, name, image, revisionDigest, processorRoot, workspaceRoot, runtimeRoot string) ([]byte, error) {
	fixtureCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	// docker run --rm only removes a container after the Docker client can
	// observe its normal exit.  A cancelled client, daemon hiccup or abrupt
	// Worker shutdown can leave the named fixture behind; clean that exact,
	// deterministic name without touching any Attempt container.
	defer func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	}()
	args := fixtureDockerArgs(name, image, revisionDigest, processorRoot, workspaceRoot, runtimeRoot)
	output, err := exec.CommandContext(fixtureCtx, "docker", args...).CombinedOutput()
	if fixtureCtx.Err() != nil {
		return output, fixtureCtx.Err()
	}
	return output, err
}

// fixtureDockerArgs is kept deterministic and side-effect free so the
// hardened Fixture contract can be verified without requiring a Docker
// daemon. Its only writable mounts are the isolated authoring workspace and
// fixture runtime directory; neither is an Attempt directory or credential
// location.
func fixtureDockerArgs(name, image, revisionDigest, processorRoot, workspaceRoot, runtimeRoot string) []string {
	return []string{
		"run", "--rm", "--name", name,
		"--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "128", "--memory", "512m", "--cpus", "1.0", "--user", "10001:10001",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/opt/seven-worker/result-processors/%s,readonly", processorRoot, revisionDigest),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s", workspaceRoot, fixtureRootInContainer),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=%s", runtimeRoot, fixtureRuntimeInContainer),
		"--entrypoint", "/usr/local/bin/seven-worker-cli", image,
		"result-fixture", "--spec", filepath.Join(fixtureRuntimeInContainer, "spec.json"),
	}
}

func fixtureContainerName(runID int64) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(runID, 10)))
	return "seven-processor-fixture-" + hex.EncodeToString(sum[:12])
}

func fixturePathSegment(runID int64) string {
	sum := sha256.Sum256([]byte(strconv.FormatInt(runID, 10)))
	return hex.EncodeToString(sum[:16])
}

func fixtureDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4096 {
		return value[:4096]
	}
	return value
}

func errorText(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}
