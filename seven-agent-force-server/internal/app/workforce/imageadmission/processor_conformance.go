package imageadmission

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// This check executes only Worker-owned Processor source, not the image's
// Coding Agent. Adapter handshake and native runtime acceptance are separate
// evidence; admission never substitutes a Fake result for a real Agent.
func runProcessorBoundaryConformance(ctx context.Context, image string) (returnErr error) {
	root, err := os.MkdirTemp("", "seven-processor-admission-")
	if err != nil {
		return err
	}
	name := filepath.Base(root)
	created := false
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if created {
			_, removeErr := exec.CommandContext(cleanup, "docker", "rm", "-f", name).CombinedOutput()
			inspection, inspectErr := exec.CommandContext(cleanup, "docker", "container", "inspect", name).CombinedOutput()
			absent := inspectErr != nil && (strings.Contains(string(inspection), "No such container") || strings.Contains(string(inspection), "No such object"))
			if !absent {
				_ = removeErr
				returnErr = errors.Join(returnErr, errors.New("IMAGE_CONFORMANCE_CLEANUP_PENDING"))
				return
			}
		}
		if err := os.RemoveAll(root); err != nil {
			returnErr = errors.Join(returnErr, errors.New("IMAGE_CONFORMANCE_TEMP_CLEANUP_FAILED"))
		}
	}()
	for _, dir := range []string{root, filepath.Join(root, "processor"), filepath.Join(root, "repo"), filepath.Join(root, "output")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o777); err != nil {
			return err
		}
	}
	// Admission must work with immutable pre-rename images as well as Force
	// images. The Force SDK retains this protocol-compatible legacy entry point.
	source := "from seven_workforce_runtime.processor import pass_result\ndef process(context):\n    result = context.process.run(['sh', '-c', 'printf ready > admission.txt'])\n    return pass_result(reason_code='ADMISSION_OK', message='Processor boundary checked')\n"
	if err := os.WriteFile(filepath.Join(root, "processor", "processor.py"), []byte(source), 0o444); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]any{"taskId": "9000001", "attemptId": "9000002", "cycleId": "9000003", "cycleNumber": 1, "workspaceRepo": "/admission/repo"})
	if err := os.WriteFile(filepath.Join(root, "context.json"), body, 0o444); err != nil {
		return err
	}
	run, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	created = true
	command := exec.CommandContext(run, "docker", "run", "--name", name, "--rm", "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "64",
		"--mount", "type=bind,src="+filepath.Join(root, "processor")+",dst=/admission/processor,readonly",
		"--mount", "type=bind,src="+filepath.Join(root, "context.json")+",dst=/admission/context.json,readonly",
		"--mount", "type=bind,src="+filepath.Join(root, "repo")+",dst=/admission/repo",
		"--mount", "type=bind,src="+filepath.Join(root, "output")+",dst=/admission/output",
		"--entrypoint", "/usr/bin/python3", image, "-I", "/opt/seven-worker/result-runtime/bootstrap.py",
		"--processor-root", "/admission/processor", "--context", "/admission/context.json", "--result", "/admission/output/result.json")
	if err := command.Run(); err != nil {
		return errors.New("IMAGE_PROCESSOR_CONFORMANCE_FAILED")
	}
	output, err := os.ReadFile(filepath.Join(root, "output", "result.json"))
	if err != nil {
		return errors.New("IMAGE_PROCESSOR_CONFORMANCE_FAILED")
	}
	var result struct {
		Decision string `json:"decision"`
		Reason   string `json:"reasonCode"`
	}
	if json.Unmarshal(output, &result) != nil || result.Decision != "PASS" || result.Reason != "ADMISSION_OK" {
		return errors.New("IMAGE_PROCESSOR_CONFORMANCE_FAILED")
	}
	proof, err := os.ReadFile(filepath.Join(root, "repo", "admission.txt"))
	if err != nil || string(proof) != "ready" {
		return errors.New("IMAGE_PROCESSOR_CONFORMANCE_FAILED")
	}
	return nil
}
