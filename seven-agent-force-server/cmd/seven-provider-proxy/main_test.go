package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestControlJSONRelaysStrictRequestOverContainerPrivateSocket(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "pc-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "provider.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	done := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			done <- acceptErr
			return
		}
		defer connection.Close()
		var request controlRequest
		if decodeErr := json.NewDecoder(connection).Decode(&request); decodeErr != nil {
			done <- decodeErr
			return
		}
		if request.Command != "state" || request.AttemptID != 910000000000000041 {
			done <- errors.New("unexpected relayed request")
			return
		}
		done <- json.NewEncoder(connection).Encode(map[string]any{"ok": true, "controlState": "READY"})
	}()
	previousStdin, previousStdout := os.Stdin, os.Stdout
	requestFile, _ := os.CreateTemp(directory, "request-")
	responseFile, _ := os.CreateTemp(directory, "response-")
	t.Cleanup(func() { os.Stdin, os.Stdout = previousStdin, previousStdout })
	_, _ = requestFile.WriteString(`{"schemaVersion":1,"command":"state","attemptId":"910000000000000041","generation":"1","ownerEpoch":"1"}`)
	_, _ = requestFile.Seek(0, 0)
	os.Stdin, os.Stdout = requestFile, responseFile
	if err = controlJSON(path); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	_, _ = responseFile.Seek(0, 0)
	response := new(bytes.Buffer)
	_, _ = response.ReadFrom(responseFile)
	if !strings.Contains(response.String(), `"controlState":"READY"`) {
		t.Fatalf("unexpected response %q", response.String())
	}
}

func TestControlSocketPathAcceptsExplicitPrivateSocketOverride(t *testing.T) {
	t.Setenv("SEVEN_PROVIDER_PROXY_CONTROL_SOCKET", "/run/seven/default.sock")
	path, err := controlSocketPath([]string{"control", "--socket", "/tmp/review-provider.sock"})
	if err != nil || path != "/tmp/review-provider.sock" {
		t.Fatalf("explicit control socket path=%q err=%v", path, err)
	}
	if _, err = controlSocketPath([]string{"control", "--socket"}); err == nil {
		t.Fatal("missing control socket value was accepted")
	}
	if _, err = controlSocketPath([]string{"control", "--unknown", "value"}); err == nil {
		t.Fatal("unknown control option was accepted")
	}
}

func TestEnsurePrivateSocketAcceptsPrivateParentWhenMountedSocketChmodIsUnsupported(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "puds-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "provider.sock")
	previous := syscall.Umask(0o077)
	listener, err := net.Listen("unix", path)
	syscall.Umask(previous)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateSocketWithChmod(path, func(string, os.FileMode) error { return syscall.EINVAL }); err != nil {
		t.Fatalf("socket under owner-only directory should be accepted: %v", err)
	}
}

func TestEnsurePrivateSocketRejectsPublicSocketWhenMountedChmodIsUnsupported(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "puds-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	path := filepath.Join(directory, "provider.sock")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := os.Chmod(directory, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateSocketWithChmod(path, func(string, os.FileMode) error { return syscall.EINVAL }); !errors.Is(err, errControlSocketNotPrivate) {
		t.Fatalf("public socket should be rejected, got %v", err)
	}
}

func TestLoadConfigRejectsUnknownFieldsAndOversizedSpecs(t *testing.T) {
	directory := t.TempDir()
	unknown := filepath.Join(directory, "unknown.json")
	if err := os.WriteFile(unknown, []byte(`{"upstreamBaseUrl":"https://api.openai.com/v1","unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("loadConfig unknown field error=%v", err)
	}
	overlarge := filepath.Join(directory, "overlarge.json")
	if err := os.WriteFile(overlarge, []byte(strings.Repeat("x", maxSpecBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfig(overlarge); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("loadConfig overlarge error=%v", err)
	}
}

func TestLoadConfigDoesNotRequireSecretsInEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.json")
	body := `{"gatewayBaseUrl":"https://framework.internal","workerTokenFile":"/run/seven/worker.token","workerId":"910000000000000042","workerInstanceId":"wki_1","hostBootId":"boot_1","taskId":"910000000000000043","attemptId":"910000000000000044","generation":"1","ownerEpoch":"1","attemptLeaseToken":"lease","modelCredentialId":"910000000000000045","credentialVersion":"2","modelProtocol":"OPENAI","primaryModel":"gpt-5.6-luna","controlSocket":"/run/seven/control/provider.sock"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.GatewayBaseURL != "https://framework.internal" || config.ModelCredentialID != 910000000000000045 || config.ModelProtocol != "OPENAI" {
		t.Fatal("configuration did not load the specification values")
	}
}
