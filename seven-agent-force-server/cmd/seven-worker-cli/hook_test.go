package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeHookConfigurationFixture(t *testing.T, event, decision string) hookSpec {
	t.Helper()

	root := t.TempDir()
	packageRoot := filepath.Join(root, "packages", "000-guard")
	if err := os.MkdirAll(filepath.Join(packageRoot, "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s' '{\"schemaVersion\":1,\"decision\":\"" + decision + "\",\"reasonCode\":\"FIXTURE_" + decision + "\",\"operatorMessage\":\"fixture\",\"evidence\":[]}'\n"
	if err := os.WriteFile(filepath.Join(packageRoot, "hooks", "guard.sh"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}

	var manifest hookManifest
	manifest.SchemaVersion = "seven.hook-package/v1"
	manifest.Runtime.Kind = "shell"
	manifest.Entrypoints = []hookEntrypoint{{
		Event:       event,
		Path:        "hooks/guard.sh",
		TimeoutMS:   1000,
		FailureMode: "BLOCK",
	}}
	configuration := hookConfigurationSpec{
		SchemaVersion:       1,
		ConfigurationDigest: "digest-test",
		Hooks: []hookPackageSpec{{
			ReleaseID:   910000000000000011,
			PackageID:   910000000000000012,
			Name:        "guard",
			Version:     "1.0.0",
			Digest:      "package-digest",
			RuntimeKind: "shell",
			Root:        packageRoot,
			Manifest:    manifest,
		}},
	}
	body, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hooks.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return hookSpec{
		ConfigurationDigest: "digest-test",
		Root:                root,
		Spec:                filepath.Join(root, "hooks.json"),
	}
}

func writeCustomHookFixture(t *testing.T, runtimeKind, script string, timeoutMS int, failureMode string) (hookSpec, hookPackageSpec, hookEntrypoint) {
	t.Helper()
	root := t.TempDir()
	packageRoot := filepath.Join(root, "packages", "000-custom")
	if err := os.MkdirAll(filepath.Join(packageRoot, "hooks"), 0o700); err != nil {
		t.Fatal(err)
	}
	extension := "sh"
	if runtimeKind == "node" {
		extension = "mjs"
	}
	relative := "hooks/check." + extension
	if err := os.WriteFile(filepath.Join(packageRoot, filepath.FromSlash(relative)), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	entry := hookEntrypoint{Event: "attempt.prepare", Path: relative, TimeoutMS: timeoutMS, FailureMode: failureMode}
	var manifest hookManifest
	manifest.SchemaVersion = "seven.hook-package/v1"
	manifest.Runtime.Kind = runtimeKind
	manifest.Entrypoints = []hookEntrypoint{entry}
	pkg := hookPackageSpec{ReleaseID: 910000000000000013, PackageID: 910000000000000014, Name: "custom", Version: "1.0.0", Digest: "digest", RuntimeKind: runtimeKind, Root: packageRoot, Manifest: manifest}
	configuration := hookConfigurationSpec{SchemaVersion: 1, ConfigurationDigest: "digest-custom", Hooks: []hookPackageSpec{pkg}}
	body, _ := json.Marshal(configuration)
	if err := os.WriteFile(filepath.Join(root, "hooks.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	return hookSpec{ConfigurationDigest: "digest-custom", Root: root, Spec: filepath.Join(root, "hooks.json")}, pkg, entry
}

func TestDispatchHooksAllowAndDeny(t *testing.T) {
	for _, decision := range []string{"ALLOW", "DENY"} {
		t.Run(decision, func(t *testing.T) {
			spec := writeHookConfigurationFixture(t, "attempt.prepare", decision)
			result, err := dispatchHooks(
				context.Background(),
				spec,
				"attempt.prepare",
				[]byte(`{"schemaVersion":1}`),
				func(string, any) error { return nil },
			)
			if err != nil {
				t.Fatal(err)
			}
			if result.Decision != decision {
				t.Fatalf("decision=%s", result.Decision)
			}
		})
	}
}

func TestRunHarnessPrepareDenyDoesNotStartHarnessOrPropose(t *testing.T) {
	spec := attemptSpec{
		SchemaVersion: 1,
		TaskID:        910000000000000015,
		AttemptID:     910000000000000016,
		Generation:    1,
		Instruction:   "fixture",
		Workspace:     t.TempDir(),
		Output:        t.TempDir(),
		State:         t.TempDir(),
		Runtime:       t.TempDir(),
		Temporary:     t.TempDir(),
	}
	hooks := writeHookConfigurationFixture(t, "attempt.prepare", "DENY")
	spec.Hooks = &hooks
	state := &supervisorState{}
	err := runHarness(context.Background(), spec, state, func(int) error {
		t.Fatal("prepare denial reported Harness started")
		return nil
	})
	if err == nil {
		t.Fatal("prepare denial did not fail")
	}
	if _, statErr := os.Stat(filepath.Join(spec.Output, "completion.json")); !os.IsNotExist(statErr) {
		t.Fatal("completion proposal exists after prepare denial")
	}
}

func TestRunHookEntrypointRejectsMalformedAndTrailingJSON(t *testing.T) {
	for name, output := range map[string]string{"malformed": "not-json", "trailing": `{"schemaVersion":1,"decision":"ALLOW","reasonCode":"OK","operatorMessage":"ok","evidence":[]} {}`} {
		t.Run(name, func(t *testing.T) {
			_, pkg, entry := writeCustomHookFixture(t, "shell", "printf '%s' '"+output+"'", 1000, "BLOCK")
			if _, _, err := runHookEntrypoint(context.Background(), filepath.Dir(filepath.Dir(pkg.Root)), pkg, entry, nil); err == nil {
				t.Fatal("invalid output was accepted")
			}
		})
	}
}

func TestRunHookEntrypointTimeoutAndEnvironmentAllowlist(t *testing.T) {
	// The full -race suite runs hundreds of subprocess-heavy packages in
	// parallel on macOS. Keep a wide separation between the allowed
	// termination window and the child lifetime so scheduler contention cannot
	// turn a successful process-group kill into a flaky wall-clock assertion.
	_, timeoutPackage, timeoutEntry := writeCustomHookFixture(t, "shell", "sleep 30", 50, "BLOCK")
	started := time.Now()
	if _, _, err := runHookEntrypoint(context.Background(), filepath.Dir(filepath.Dir(timeoutPackage.Root)), timeoutPackage, timeoutEntry, nil); err == nil {
		t.Fatal("timed out Hook was accepted")
	}
	if time.Since(started) > 15*time.Second {
		t.Fatal("Hook timeout did not terminate promptly")
	}
	t.Setenv("SEVEN_PROVIDER_API_KEY", "must-not-leak")
	allowJSON := `{"schemaVersion":1,"decision":"ALLOW","reasonCode":"ENV_CLEAN","operatorMessage":"clean","evidence":[]}`
	script := `if [ -n "${SEVEN_PROVIDER_API_KEY:-}" ]; then exit 9; fi; printf '%s' '` + allowJSON + `'`
	_, envPackage, envEntry := writeCustomHookFixture(t, "shell", script, 1000, "BLOCK")
	result, _, err := runHookEntrypoint(context.Background(), filepath.Dir(filepath.Dir(envPackage.Root)), envPackage, envEntry, nil)
	if err != nil || result.ReasonCode != "ENV_CLEAN" {
		t.Fatalf("allowlisted environment failed: result=%+v err=%v", result, err)
	}
}

func TestDispatchHooksObserveFailureContinuesAndNodeRuntimeWorks(t *testing.T) {
	failedEvents := []string{}
	spec, _, _ := writeCustomHookFixture(t, "shell", "exit 3", 1000, "OBSERVE")
	result, err := dispatchHooks(context.Background(), spec, "attempt.prepare", []byte(`{}`), func(event string, _ any) error { failedEvents = append(failedEvents, event); return nil })
	if err != nil || result.Decision != "ALLOW" || !strings.Contains(strings.Join(failedEvents, ","), "hook.failed") {
		t.Fatalf("OBSERVE failure did not continue: result=%+v events=%v err=%v", result, failedEvents, err)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node runtime is unavailable")
	}
	nodeScript := `process.stdout.write(JSON.stringify({schemaVersion:1,decision:"DENY",reasonCode:"NODE_DENY",operatorMessage:"node",evidence:[]}));`
	// This case verifies the admitted Node runtime and contract, not timeout
	// enforcement. Keep enough wall-clock headroom for whole-repository test
	// contention; the dedicated timeout case above retains the 50 ms deadline.
	nodeSpec, _, _ := writeCustomHookFixture(t, "node", nodeScript, 5000, "BLOCK")
	nodeResult, err := dispatchHooks(context.Background(), nodeSpec, "attempt.prepare", []byte(`{}`), func(string, any) error { return nil })
	if err != nil || nodeResult.Decision != "DENY" || nodeResult.ReasonCode != "NODE_DENY" {
		t.Fatalf("Node Hook mismatch: result=%+v err=%v", nodeResult, err)
	}
}

func TestDispatchHooksRejectsConfigurationIdentityAndPackageEscape(t *testing.T) {
	spec := writeHookConfigurationFixture(t, "attempt.prepare", "ALLOW")
	spec.ConfigurationDigest = "different"
	if _, err := dispatchHooks(context.Background(), spec, "attempt.prepare", nil, func(string, any) error { return nil }); err == nil {
		t.Fatal("Hook configuration identity mismatch was accepted")
	}
	spec, pkg, entry := writeCustomHookFixture(t, "shell", "exit 0", 1000, "BLOCK")
	pkg.Root = t.TempDir()
	if _, _, err := runHookEntrypoint(context.Background(), spec.Root, pkg, entry, nil); err == nil {
		t.Fatal("package root escape was accepted")
	}
}

func TestNativeHookEventRoundTripsThroughAttemptPrivateUDS(t *testing.T) {
	testNativeHookEventRoundTrip(t, listenControl)
}

func TestNativeHookEventRoundTripsWhenVirtioFSRejectsSocketChmod(t *testing.T) {
	testNativeHookEventRoundTrip(t, func(path string, spec attemptSpec, state *supervisorState) (net.Listener, error) {
		return listenControlWithChmod(path, spec, state, func(string, os.FileMode) error { return syscall.EINVAL })
	})
}

func testNativeHookEventRoundTrip(t *testing.T, listen func(string, attemptSpec, *supervisorState) (net.Listener, error)) {
	t.Helper()
	hooks := writeHookConfigurationFixture(t, "codex.PreToolUse", "DENY")
	shortRoot, err := os.MkdirTemp("/tmp", "seven-uds-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(shortRoot)
	stateDir, runtimeDir := filepath.Join(shortRoot, "state"), filepath.Join(shortRoot, "runtime")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	spec := attemptSpec{TaskID: 2610240832, AttemptID: 3107414342, Generation: 3, State: stateDir, Runtime: runtimeDir, Hooks: &hooks}
	state := &supervisorState{TaskID: spec.TaskID, AttemptID: spec.AttemptID, Generation: spec.Generation, State: "RUNNING"}
	socket := filepath.Join(runtimeDir, "s")
	listener, err := listen(socket, spec, state)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("SEVEN_TASK_ID", strconv.FormatInt(spec.TaskID, 10))
	t.Setenv("SEVEN_ATTEMPT_ID", strconv.FormatInt(spec.AttemptID, 10))
	t.Setenv("SEVEN_ATTEMPT_GENERATION", "3")
	t.Setenv("SEVEN_CONTROL_SOCKET", socket)
	payload := map[string]any{"event": "codex.PreToolUse", "invocationId": "hinv_12345678", "configurationDigest": hooks.ConfigurationDigest, "packageReleaseId": "910000000000000011", "packageVersion": "1.0.0", "packageDigest": "package-digest", "reasonCode": "SENTINEL_COMMAND_DENIED", "durationMs": int64(4)}
	if err := sendNativeHookEvent("hook.deny", payload); err != nil {
		t.Fatal(err)
	}
	journal, err := os.ReadFile(filepath.Join(stateDir, "hook-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(journal), `"SENTINEL_COMMAND_DENIED"`) || !strings.Contains(string(journal), `"packageDigest":"package-digest"`) {
		t.Fatalf("journal=%s", journal)
	}
}
