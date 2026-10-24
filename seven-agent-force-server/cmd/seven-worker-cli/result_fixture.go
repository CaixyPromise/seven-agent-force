package main

// The Result Processor Fixture command is deliberately much smaller than the
// Attempt supervisor.  A Fixture runs one immutable, unpublished Revision in
// an exact admitted Harness image.  It has no UDS, no Provider Facade and no
// access to an Attempt workspace or credentials; the Worker gives it only an
// isolated temporary workspace and a writable private result directory.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	fixtureProcessorRoot = "/opt/seven-worker/result-processors"
	fixtureRuntimeRoot   = "/run/seven-fixture"
	fixtureWorkspaceRoot = "/workspace"
	fixtureBootstrapPath = "/opt/seven-worker/result-runtime/bootstrap.py"
)

type resultFixtureSpec struct {
	SchemaVersion     int             `json:"schemaVersion"`
	RunID             int64           `json:"runId,string"`
	RevisionDigest    string          `json:"revisionDigest"`
	ProcessorRoot     string          `json:"processorRoot"`
	WorkspaceRoot     string          `json:"workspaceRoot"`
	RuntimeRoot       string          `json:"runtimeRoot"`
	ResultPath        string          `json:"resultPath"`
	EvaluationContext json.RawMessage `json:"evaluationContext"`
	ConfigOverrides   json.RawMessage `json:"configOverrides"`
	RunDeliver        bool            `json:"runDeliver,omitempty"`
}

func resultFixture(args []string) error {
	flags := flag.NewFlagSet("result-fixture", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	specPath := flags.String("spec", "", "private fixture spec")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*specPath) == "" {
		return errors.New("usage: seven-worker-cli result-fixture --spec <path>")
	}
	body, err := os.ReadFile(*specPath)
	if err != nil {
		return fmt.Errorf("read fixture spec: %w", err)
	}
	if len(body) == 0 || len(body) > 1<<20 {
		return errors.New("fixture spec is empty or oversized")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var spec resultFixtureSpec
	if err = decoder.Decode(&spec); err != nil {
		return fmt.Errorf("decode fixture spec: %w", err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("fixture spec must contain exactly one object")
	}
	if err = validateResultFixtureSpec(spec); err != nil {
		return err
	}
	// A fixture has the same protected Python bootstrap/module contract as a
	// real Attempt.  It may use an unpublished revision, but never a mutable
	// or host-provided runtime.
	if _, err = currentResultProcessorImageIdentity(); err != nil {
		return fmt.Errorf("verify protected Result Processor runtime: %w", err)
	}
	return executeResultFixture(spec)
}

func validateResultFixtureSpec(spec resultFixtureSpec) error {
	if spec.SchemaVersion != 1 || spec.RunID <= 0 || len(spec.RevisionDigest) != 64 || !isLowerHex(spec.RevisionDigest) {
		return errors.New("fixture identity is invalid")
	}
	expectedRoot := filepath.Join(fixtureProcessorRoot, spec.RevisionDigest)
	if filepath.Clean(spec.ProcessorRoot) != expectedRoot || filepath.Clean(spec.WorkspaceRoot) != fixtureWorkspaceRoot || filepath.Clean(spec.RuntimeRoot) != fixtureRuntimeRoot || filepath.Clean(spec.ResultPath) != filepath.Join(fixtureRuntimeRoot, "result.json") {
		return errors.New("fixture path contract is invalid")
	}
	if !jsonObject(spec.EvaluationContext) || !jsonObject(spec.ConfigOverrides) {
		return errors.New("fixture evaluation context or config overrides are invalid")
	}
	return nil
}

func executeResultFixture(spec resultFixtureSpec) error {
	if info, err := os.Stat(spec.ProcessorRoot); err != nil || !info.IsDir() {
		return errors.New("fixture Processor release root is unavailable")
	}
	if info, err := os.Stat(spec.WorkspaceRoot); err != nil || !info.IsDir() {
		return errors.New("fixture workspace is unavailable")
	}
	if err := os.MkdirAll(spec.RuntimeRoot, 0o700); err != nil {
		return err
	}
	contextPath := filepath.Join(spec.RuntimeRoot, "context.json")
	materializationPath := filepath.Join(spec.RuntimeRoot, "materialization.json")
	if !insideRuntimeDirectory(spec.RuntimeRoot, contextPath) || !insideRuntimeDirectory(spec.RuntimeRoot, materializationPath) || !insideRuntimeDirectory(spec.RuntimeRoot, spec.ResultPath) {
		return errors.New("fixture private path is invalid")
	}
	contextBody, err := fixtureEvaluationContext(spec)
	if err != nil {
		return err
	}
	materializationBody, err := json.Marshal(map[string]any{"variables": json.RawMessage(spec.ConfigOverrides)})
	if err != nil {
		return err
	}
	if err = atomicPrivateWrite(contextPath, contextBody); err != nil {
		return err
	}
	if err = atomicPrivateWrite(materializationPath, materializationBody); err != nil {
		return err
	}
	defer os.Remove(contextPath)
	defer os.Remove(materializationPath)

	fixtureCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err = runFixtureBootstrap(fixtureCtx, spec, contextPath, spec.ResultPath, materializationPath, "process"); err != nil {
		return err
	}
	candidate, err := os.ReadFile(spec.ResultPath)
	if err != nil || len(candidate) == 0 || len(candidate) > 1<<20 || !json.Valid(candidate) {
		return errors.New("fixture Result Contract is unavailable")
	}
	if spec.RunDeliver {
		decision, decisionErr := fixtureContractDecision(candidate)
		if decisionErr != nil {
			return decisionErr
		}
		if decision == "PASS" {
			actionsPath := filepath.Join(spec.RuntimeRoot, "delivery-actions.json")
			deliveryMaterializationPath := filepath.Join(spec.RuntimeRoot, "delivery-materialization.json")
			if err = atomicPrivateWrite(deliveryMaterializationPath, []byte(`{"variables":{}}`)); err != nil {
				return err
			}
			defer os.Remove(deliveryMaterializationPath)
			if err = runFixtureBootstrap(fixtureCtx, spec, contextPath, actionsPath, deliveryMaterializationPath, "deliver"); err != nil {
				return err
			}
			actions, readErr := os.ReadFile(actionsPath)
			if readErr != nil || len(actions) == 0 || len(actions) > 1<<20 || !json.Valid(actions) {
				return errors.New("fixture deliver actions are unavailable")
			}
			candidate, err = mergeFixtureDeliveryActions(candidate, actions)
			if err != nil {
				return err
			}
			if err = atomicPrivateWrite(spec.ResultPath, candidate); err != nil {
				return err
			}
		}
	}
	sum := sha256.Sum256(candidate)
	// The candidate stays in the mounted private result directory for the
	// Worker to forward. It is intentionally not written to stdout.
	if err = atomicPrivateWrite(filepath.Join(spec.RuntimeRoot, "result.sha256"), []byte(hex.EncodeToString(sum[:]))); err != nil {
		return err
	}
	return nil
}

func runFixtureBootstrap(ctx context.Context, spec resultFixtureSpec, contextPath, resultPath, materializationPath, entry string) error {
	command := exec.CommandContext(ctx, "python3", fixtureBootstrapPath,
		"--processor-root", spec.ProcessorRoot,
		"--context", contextPath,
		"--result", resultPath,
		"--materialization", materializationPath,
		"--entry", entry,
	)
	command.Dir = spec.RuntimeRoot
	command.Env = []string{
		"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8", "LC_ALL=C.UTF-8", "PYTHONNOUSERSITE=1",
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	diagnostic, commandErr := boundedCommandOutput(ctx, command, 64<<10)
	if commandErr == nil {
		return nil
	}
	if len(diagnostic) != 0 {
		return fmt.Errorf("result processor fixture %s bootstrap: %s", entry, boundedDiagnostic(diagnostic, 4096))
	}
	return fmt.Errorf("result processor fixture %s bootstrap: %w", entry, commandErr)
}

func fixtureContractDecision(candidate []byte) (string, error) {
	var contract struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(candidate, &contract); err != nil || contract.Decision == "" {
		return "", errors.New("fixture Result Contract decision is invalid")
	}
	return contract.Decision, nil
}

func mergeFixtureDeliveryActions(contractBody, actionsBody []byte) ([]byte, error) {
	var contract map[string]json.RawMessage
	if err := json.Unmarshal(contractBody, &contract); err != nil {
		return nil, errors.New("fixture Result Contract is invalid")
	}
	var delivery struct {
		Actions []json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(actionsBody, &delivery); err != nil || delivery.Actions == nil {
		return nil, errors.New("fixture deliver action envelope is invalid")
	}
	var existing []json.RawMessage
	if current, ok := contract["actions"]; ok && len(current) != 0 && string(current) != "null" {
		if err := json.Unmarshal(current, &existing); err != nil {
			return nil, errors.New("fixture Result Contract actions are invalid")
		}
	}
	if len(existing)+len(delivery.Actions) > 64 {
		return nil, errors.New("fixture Result Contract actions exceed limit")
	}
	merged, err := json.Marshal(append(existing, delivery.Actions...))
	if err != nil {
		return nil, err
	}
	contract["actions"] = merged
	result, err := json.Marshal(contract)
	if err != nil || len(result) > 1<<20 {
		return nil, errors.New("fixture Result Contract actions are oversized")
	}
	return result, nil
}

func fixtureEvaluationContext(spec resultFixtureSpec) ([]byte, error) {
	var supplied map[string]json.RawMessage
	if err := json.Unmarshal(spec.EvaluationContext, &supplied); err != nil {
		return nil, errors.New("fixture evaluation context is invalid")
	}
	parameters := json.RawMessage(`{}`)
	if candidate, ok := supplied["parameters"]; ok {
		if !jsonObject(candidate) {
			return nil, errors.New("fixture parameters must be an object")
		}
		parameters = candidate
	}
	var parameterObject map[string]any
	if err := json.Unmarshal(parameters, &parameterObject); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{
		"taskId":        strconv.FormatInt(spec.RunID, 10),
		"attemptId":     strconv.FormatInt(spec.RunID, 10),
		"cycleId":       "fixture",
		"cycleNumber":   1,
		"workspaceRepo": fixtureWorkspaceRoot,
		"parameters":    parameterObject,
	})
}

func jsonObject(value []byte) bool {
	var object map[string]any
	return len(value) != 0 && json.Unmarshal(value, &object) == nil
}

func isLowerHex(value string) bool {
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
