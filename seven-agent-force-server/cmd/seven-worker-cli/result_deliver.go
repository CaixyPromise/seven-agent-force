package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type resultDeliverSpec struct {
	SchemaVersion       int             `json:"schemaVersion"`
	DeliveryAttemptID   int64           `json:"deliveryAttemptId,string"`
	ProcessorDigest     string          `json:"processorDigest"`
	ProcessorRoot       string          `json:"processorRoot"`
	ResultRoot          string          `json:"resultRoot"`
	RuntimeRoot         string          `json:"runtimeRoot"`
	ResultPath          string          `json:"resultPath"`
	DeliveryContext     json.RawMessage `json:"deliveryContext"`
	MaterializationPath string          `json:"materializationPath"`
}

// resultDeliver invokes only the optional, published deliver(context) entry in
// a short-lived delivery runner. The business result is written to a protected
// file and is still only a candidate until the platform verifier accepts it.
func resultDeliver(args []string) error {
	flags := flag.NewFlagSet("result-deliver", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	specPath := flags.String("spec", "", "protected delivery spec")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *specPath == "" {
		return errors.New("usage: seven-worker-cli result-deliver --spec <path>")
	}
	body, err := os.ReadFile(*specPath)
	if err != nil || len(body) == 0 || len(body) > 1<<20 {
		return errors.New("delivery spec is unavailable")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var spec resultDeliverSpec
	if err = decoder.Decode(&spec); err != nil {
		return fmt.Errorf("decode delivery spec: %w", err)
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return errors.New("delivery spec must contain exactly one object")
	}
	if err = validateResultDeliverSpec(spec); err != nil {
		return err
	}
	if _, err = currentResultProcessorImageIdentity(); err != nil {
		return fmt.Errorf("verify protected Result Processor runtime: %w", err)
	}
	contextPath := filepath.Join(spec.RuntimeRoot, "delivery-context.json")
	if err = atomicPrivateWrite(contextPath, spec.DeliveryContext); err != nil {
		return err
	}
	defer os.Remove(contextPath)
	runCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	command := exec.CommandContext(runCtx, "python3", fixtureBootstrapPath,
		"--processor-root", spec.ProcessorRoot, "--context", contextPath,
		"--result", spec.ResultPath, "--materialization", spec.MaterializationPath,
		"--entry", "deliver")
	command.Dir = spec.RuntimeRoot
	command.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "PYTHONNOUSERSITE=1"}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	diagnostic, err := boundedCommandOutput(runCtx, command, 64<<10)
	if err != nil {
		return fmt.Errorf("result delivery failed: %s", boundedDiagnostic(diagnostic, 4096))
	}
	candidate, err := os.ReadFile(spec.ResultPath)
	if err != nil || len(candidate) == 0 || len(candidate) > 1<<20 || !json.Valid(candidate) {
		return errors.New("delivery candidate contract is unavailable")
	}
	return nil
}

func validateResultDeliverSpec(spec resultDeliverSpec) error {
	if spec.SchemaVersion != 1 || spec.DeliveryAttemptID <= 0 || len(spec.ProcessorDigest) != 64 || !isLowerHex(spec.ProcessorDigest) || !jsonObject(spec.DeliveryContext) {
		return errors.New("delivery identity is invalid")
	}
	if filepath.Clean(spec.ProcessorRoot) != filepath.Join(fixtureProcessorRoot, spec.ProcessorDigest) || filepath.Clean(spec.ResultRoot) != "/workspace" || filepath.Clean(spec.RuntimeRoot) != "/run/seven-delivery" || filepath.Clean(spec.ResultPath) != "/run/seven-delivery/result.json" || filepath.Clean(spec.MaterializationPath) != "/run/seven-delivery/materialization.json" {
		return errors.New("delivery path contract is invalid")
	}
	return nil
}
