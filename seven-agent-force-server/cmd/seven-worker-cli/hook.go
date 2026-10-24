package main

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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const maxHookIO = 1 << 20

type hookSpec struct {
	ConfigurationDigest string `json:"configurationDigest"`
	Root                string `json:"root"`
	Spec                string `json:"spec"`
}
type hookConfigurationSpec struct {
	SchemaVersion       int               `json:"schemaVersion"`
	ConfigurationDigest string            `json:"configurationDigest"`
	Hooks               []hookPackageSpec `json:"hooks"`
}
type hookPackageSpec struct {
	ReleaseID   int64        `json:"releaseId,string"`
	PackageID   int64        `json:"packageId,string"`
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Digest      string       `json:"digest"`
	RuntimeKind string       `json:"runtimeKind"`
	Root        string       `json:"root"`
	Manifest    hookManifest `json:"manifest"`
}
type hookManifest struct {
	SchemaVersion string `json:"schemaVersion"`
	Runtime       struct {
		Kind string `json:"kind"`
	} `json:"runtime"`
	Entrypoints []hookEntrypoint `json:"entrypoints"`
}
type hookEntrypoint struct {
	Event       string `json:"event"`
	Path        string `json:"path"`
	TimeoutMS   int    `json:"timeoutMs"`
	FailureMode string `json:"failureMode"`
}
type hookDecision struct {
	SchemaVersion   int              `json:"schemaVersion"`
	Decision        string           `json:"decision"`
	ReasonCode      string           `json:"reasonCode"`
	OperatorMessage string           `json:"operatorMessage"`
	Evidence        []map[string]any `json:"evidence"`
}

func hookMain(args []string) error {
	if len(args) < 1 || args[0] != "dispatch" {
		return errors.New("seven-hook supports only: dispatch --event EVENT")
	}
	flags := flag.NewFlagSet("hook dispatch", flag.ContinueOnError)
	event := flags.String("event", "", "known Hook event")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if !knownHookEvent(*event) {
		return errors.New("unknown Hook event")
	}
	input, err := io.ReadAll(io.LimitReader(os.Stdin, maxHookIO+1))
	if err != nil || len(input) > maxHookIO {
		return errors.New("Hook input exceeds 1 MiB")
	}
	root := strings.TrimSpace(os.Getenv("SEVEN_HOOK_CONFIGURATION_ROOT"))
	digest := strings.TrimSpace(os.Getenv("SEVEN_HOOK_CONFIGURATION_DIGEST"))
	if root == "" || digest == "" {
		return errors.New("managed Hook configuration environment is missing")
	}
	spec := hookSpec{Root: root, Spec: filepath.Join(root, "hooks.json"), ConfigurationDigest: digest}
	envelope := nativeEnvelope(*event, input)
	sink := sendNativeHookEvent
	result, err := dispatchHooks(context.Background(), spec, *event, envelope, sink)
	if err != nil {
		if strings.HasPrefix(*event, "codex.") && strings.Contains(err.Error(), "HOOK_AUDIT_UNAVAILABLE") {
			_ = writeHookAuditFailureMarker(err)
			return writeCodexHookResult(*event, hookDecision{SchemaVersion: 1, Decision: "DENY", ReasonCode: "HOOK_AUDIT_UNAVAILABLE", OperatorMessage: "managed Hook audit channel is unavailable", Evidence: []map[string]any{}})
		}
		return err
	}
	if strings.HasPrefix(*event, "codex.") {
		return writeCodexHookResult(*event, result)
	}
	if strings.HasPrefix(*event, "claude.") {
		return writeClaudeHookResult(*event, result)
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func dispatchHooks(ctx context.Context, spec hookSpec, event string, input []byte, sink func(string, any) error) (hookDecision, error) {
	if spec.ConfigurationDigest == "" {
		return allowDecision(), nil
	}
	if filepath.Clean(spec.Root) != filepath.Clean(filepath.Dir(spec.Spec)) {
		return hookDecision{}, errors.New("HOOK_SPEC_PATH_INVALID")
	}
	body, err := os.ReadFile(spec.Spec)
	if err != nil {
		return hookDecision{}, fmt.Errorf("HOOK_SPEC_READ_FAILED: %w", err)
	}
	var configuration hookConfigurationSpec
	if err := json.Unmarshal(body, &configuration); err != nil || configuration.SchemaVersion != 1 || configuration.ConfigurationDigest != spec.ConfigurationDigest {
		return hookDecision{}, errors.New("HOOK_CONFIGURATION_IDENTITY_MISMATCH")
	}
	aggregate := allowDecision()
	for _, pkg := range configuration.Hooks {
		for _, entry := range pkg.Manifest.Entrypoints {
			if entry.Event != event {
				continue
			}
			invocationID := hookInvocationID(spec.ConfigurationDigest, pkg.ReleaseID, event, input)
			started := time.Now()
			if err := sink("hook.started", hookEventPayload(spec, pkg, event, invocationID, "", "", 0)); err != nil {
				return hookDecision{}, fmt.Errorf("HOOK_AUDIT_UNAVAILABLE: %w", err)
			}
			result, diagnostics, runErr := runHookEntrypoint(ctx, spec.Root, pkg, entry, input)
			duration := time.Since(started).Milliseconds()
			if runErr != nil {
				if err := sink("hook.failed", hookEventPayload(spec, pkg, event, invocationID, "HOOK_EXECUTION_FAILED", boundedHookText(diagnostics), duration)); err != nil {
					return hookDecision{}, fmt.Errorf("HOOK_AUDIT_UNAVAILABLE: %w", err)
				}
				if entry.FailureMode == "BLOCK" {
					return hookDecision{SchemaVersion: 1, Decision: "DENY", ReasonCode: "HOOK_EXECUTION_FAILED", OperatorMessage: "managed Hook execution failed closed", Evidence: []map[string]any{{"packageReleaseId": strconv.FormatInt(pkg.ReleaseID, 10)}}}, nil
				}
				continue
			}
			if err := sink("hook."+strings.ToLower(result.Decision), hookEventPayload(spec, pkg, event, invocationID, result.ReasonCode, "", duration)); err != nil {
				return hookDecision{}, fmt.Errorf("HOOK_AUDIT_UNAVAILABLE: %w", err)
			}
			if result.Decision == "DENY" {
				aggregate = result
				return aggregate, nil
			}
		}
	}
	return aggregate, nil
}

func hookEventPayload(spec hookSpec, pkg hookPackageSpec, event, invocationID, reasonCode, diagnostic string, durationMS int64) map[string]any {
	return map[string]any{
		"event": event, "invocationId": invocationID, "configurationDigest": spec.ConfigurationDigest,
		"hookName": pkg.Name, "packageReleaseId": strconv.FormatInt(pkg.ReleaseID, 10), "packageVersion": pkg.Version, "packageDigest": pkg.Digest,
		"reasonCode": reasonCode, "diagnostic": diagnostic, "durationMs": durationMS,
	}
}

func sendNativeHookEvent(eventType string, value any) error {
	payload, ok := value.(map[string]any)
	if !ok {
		return errors.New("invalid Hook audit payload")
	}
	generation, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("SEVEN_ATTEMPT_GENERATION")), 10, 64)
	if err != nil || generation < 1 {
		return errors.New("Attempt generation is unavailable")
	}
	taskID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("SEVEN_TASK_ID")), 10, 64)
	if err != nil || taskID <= 0 {
		return errors.New("Task identity is unavailable")
	}
	attemptID, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("SEVEN_ATTEMPT_ID")), 10, 64)
	if err != nil || attemptID <= 0 {
		return errors.New("Attempt identity is unavailable")
	}
	packageReleaseID := int64Value(payload["packageReleaseId"])
	if packageReleaseID <= 0 {
		return errors.New("Hook Package Release identity is unavailable")
	}
	request := attemptControlRequest{
		SchemaVersion: 1, Command: "hook-event", TaskID: taskID, AttemptID: attemptID, Generation: generation,
		EventType: eventType, HookEvent: stringValue(payload["event"]), InvocationID: stringValue(payload["invocationId"]),
		ConfigurationDigest: stringValue(payload["configurationDigest"]), HookName: stringValue(payload["hookName"]), PackageReleaseID: packageReleaseID,
		PackageVersion: stringValue(payload["packageVersion"]), PackageDigest: stringValue(payload["packageDigest"]),
		ReasonCode: stringValue(payload["reasonCode"]), Diagnostic: boundedHookText(stringValue(payload["diagnostic"])), DurationMS: int64Value(payload["durationMs"]),
	}
	socket := strings.TrimSpace(os.Getenv("SEVEN_CONTROL_SOCKET"))
	if socket == "" {
		return errors.New("SEVEN_CONTROL_SOCKET is required")
	}
	conn, err := net.DialTimeout("unix", socket, 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return err
	}
	var response struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(conn, 16<<10)).Decode(&response); err != nil {
		return err
	}
	if !response.OK {
		return errors.New(response.Error)
	}
	return nil
}

func writeHookAuditFailureMarker(cause error) error {
	state := strings.TrimSpace(os.Getenv("SEVEN_STATE"))
	if state == "" {
		return errors.New("SEVEN_STATE is required")
	}
	temporary, err := os.CreateTemp(state, ".hook-audit-failure-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "reasonCode": "HOOK_AUDIT_UNAVAILABLE", "diagnostic": boundedHookText(cause.Error()), "time": time.Now().UTC().Format(time.RFC3339Nano)})
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(state, "hook-audit-failure.json"))
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

func int64Value(value any) int64 {
	switch number := value.(type) {
	case int64:
		return number
	case int:
		return int64(number)
	case float64:
		return int64(number)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(number), 10, 64)
		if err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == number {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func runHookEntrypoint(parent context.Context, configurationRoot string, pkg hookPackageSpec, entry hookEntrypoint, input []byte) (hookDecision, string, error) {
	packageRoot := filepath.Clean(pkg.Root)
	expectedPrefix := filepath.Clean(configurationRoot) + string(os.PathSeparator)
	if !strings.HasPrefix(packageRoot, expectedPrefix) {
		return hookDecision{}, "", errors.New("package root escapes Hook configuration")
	}
	target := filepath.Join(packageRoot, filepath.FromSlash(entry.Path))
	if !strings.HasPrefix(filepath.Clean(target), packageRoot+string(os.PathSeparator)) {
		return hookDecision{}, "", errors.New("entrypoint escapes package")
	}
	timeout := time.Duration(entry.TimeoutMS) * time.Millisecond
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var command *exec.Cmd
	if pkg.RuntimeKind == "node" {
		command = exec.Command("node", target)
	} else if pkg.RuntimeKind == "shell" {
		command = exec.Command("/bin/sh", target)
	} else {
		return hookDecision{}, "", errors.New("unsupported Hook runtime")
	}
	command.Dir = packageRoot
	command.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "HOME=/nonexistent", "SEVEN_TASK_ID=" + os.Getenv("SEVEN_TASK_ID"), "SEVEN_ATTEMPT_ID=" + os.Getenv("SEVEN_ATTEMPT_ID"), "SEVEN_WORKSPACE=" + os.Getenv("SEVEN_WORKSPACE")}
	command.Stdin = bytes.NewReader(input)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr limitedWriter
	stdout.limit = maxHookIO
	stderr.limit = 512 << 10
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		return hookDecision{}, stderr.String(), err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		err = <-done
		if err == nil {
			err = ctx.Err()
		}
	}
	if err != nil {
		if command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
		return hookDecision{}, stderr.String(), err
	}
	var result hookDecision
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return hookDecision{}, stderr.String(), err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return hookDecision{}, stderr.String(), errors.New("Hook result must contain exactly one JSON value")
	}
	if result.SchemaVersion != 1 || (result.Decision != "ALLOW" && result.Decision != "DENY") || strings.TrimSpace(result.ReasonCode) == "" {
		return hookDecision{}, stderr.String(), errors.New("invalid Hook result")
	}
	return result, stderr.String(), nil
}

func writeCodexHookResult(event string, result hookDecision) error {
	native := strings.TrimPrefix(event, "codex.")
	if result.Decision == "ALLOW" {
		return nil
	}
	if native == "PostToolUse" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"decision":      "block",
			"reason":        result.OperatorMessage,
			"systemMessage": result.ReasonCode,
		})
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": native, "permissionDecision": "deny", "permissionDecisionReason": result.OperatorMessage}, "systemMessage": result.ReasonCode})
}
func writeClaudeHookResult(event string, result hookDecision) error {
	native := strings.TrimPrefix(event, "claude.")
	if result.Decision == "ALLOW" {
		return nil
	}
	if native == "PostToolUse" {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"decision":      "block",
			"reason":        result.OperatorMessage,
			"systemMessage": result.ReasonCode,
		})
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": native, "permissionDecision": "deny",
		"permissionDecisionReason": result.OperatorMessage,
	}, "systemMessage": result.ReasonCode})
}
func nativeEnvelope(event string, input []byte) []byte {
	var payload any
	if json.Unmarshal(input, &payload) != nil {
		payload = map[string]any{"raw": "invalid"}
	}
	body, _ := json.Marshal(map[string]any{"schemaVersion": 1, "event": event, "taskId": os.Getenv("SEVEN_TASK_ID"), "attemptId": os.Getenv("SEVEN_ATTEMPT_ID"), "configurationDigest": os.Getenv("SEVEN_HOOK_CONFIGURATION_DIGEST"), "payload": payload})
	return body
}
func knownHookEvent(event string) bool {
	switch event {
	case "attempt.prepare", "attempt.propose", "attempt.end", "codex.PreToolUse", "codex.PostToolUse", "claude.PreToolUse", "claude.PostToolUse":
		return true
	}
	return false
}
func allowDecision() hookDecision {
	return hookDecision{SchemaVersion: 1, Decision: "ALLOW", ReasonCode: "OK", OperatorMessage: "allowed", Evidence: []map[string]any{}}
}
func hookInvocationID(parts ...any) string {
	body, _ := json.Marshal(parts)
	sum := sha256.Sum256(body)
	return "hinv_" + hex.EncodeToString(sum[:8])
}

type limitedWriter struct {
	body  []byte
	limit int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if len(w.body)+len(p) > w.limit {
		return 0, errors.New("Hook output limit exceeded")
	}
	w.body = append(w.body, p...)
	return len(p), nil
}
func (w *limitedWriter) Bytes() []byte  { return w.body }
func (w *limitedWriter) String() string { return string(w.body) }
func boundedHookText(value string) string {
	if len(value) > 4096 {
		return value[:4096]
	}
	return value
}
