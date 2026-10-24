package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

func TestDecodeProviderProxyControlResponseAcceptsAuthorityLease(t *testing.T) {
	body := []byte(`{"ok":true,"ownerEpoch":"2","workerInstanceId":"wki_new","leaseExpiresAt":"2026-08-27T00:30:00Z","reattachPermitId":"4101","reattachPermitDigest":"digest","controlState":"READY"}`)
	var response providerProxyControlResponse
	if err := decodeProviderProxyControlResponse(body, &response); err != nil {
		t.Fatalf("decode Provider Proxy control response: %v", err)
	}
	if response.OwnerEpoch != 2 || response.WorkerInstanceID != "wki_new" || response.LeaseExpiresAt != "2026-08-27T00:30:00Z" || response.ControlState != "READY" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

func TestDecodeProviderProxyControlResponseRejectsUnknownFieldsAndTrailingData(t *testing.T) {
	for _, body := range []string{
		`{"ok":true,"controlState":"READY","secret":"must-not-be-accepted"}`,
		`{"ok":true,"controlState":"READY"}{}`,
	} {
		var response providerProxyControlResponse
		if err := decodeProviderProxyControlResponse([]byte(body), &response); err == nil {
			t.Fatalf("expected strict decode failure for %s", strings.TrimSpace(body))
		}
	}
}

func TestStartProviderProxyUsesGatewayIdentityWithoutProviderSecret(t *testing.T) {
	temporary := t.TempDir()
	tokenFile := filepath.Join(temporary, "worker.token")
	if err := os.WriteFile(tokenFile, []byte(strings.Repeat("t", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	controlRoot := filepath.Join(temporary, "control")
	runtime := NewDocker(config.WorkerConfig{ID: 1688799477, ServerBaseURL: "http://127.0.0.1:9277", TokenFile: tokenFile}, config.ProviderProxyConfig{Image: "provider-proxy@sha256:abc", ImageDigest: "sha256:abc", Version: "0.2.0", ControlRoot: controlRoot}, config.ProviderSSRFConfig{MaxRequestBytes: 128 << 20})
	runtime.waitProviderProxy = func(context.Context, string) error { return nil }
	commands := make([]string, 0, 8)
	runtime.commandText = func(_ context.Context, name string, args ...string) (string, error) {
		command := strings.TrimSpace(name + " " + strings.Join(args, " "))
		commands = append(commands, command)
		switch {
		case strings.Contains(command, "docker image inspect"):
			return "sha256:abc", nil
		case strings.Contains(command, "docker network create"):
			return "network-id", nil
		case strings.Contains(command, "docker create --name"):
			return "proxy-id", nil
		default:
			return "ok", nil
		}
	}
	proxyRuntime, err := runtime.startProviderProxy(context.Background(), domain.WorkItem{TaskID: 2610240832, HarnessTimeoutSeconds: 60}, domain.Attempt{ID: 3107414342, Generation: 1, OwnerEpoch: 1, LeaseToken: strings.Repeat("l", 64), WorkerInstance: "wki_1", HostBootID: "boot_1"}, ProviderRuntimeIdentity{
		ModelCredentialID: 3260443846, CredentialVersion: 3, Protocol: "OPENAI", PrimaryModel: "gpt-test",
		ExpectedProxyDigest: "sha256:abc", ExpectedProxyVersion: "0.2.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if proxyRuntime.NetworkName != "seven-provider-3107414342" || proxyRuntime.ContainerID != "proxy-id" || len(proxyRuntime.ExecutionIdentityDigest) != 64 {
		t.Fatalf("unexpected Provider Proxy runtime: %+v", proxyRuntime)
	}
	commandLog := strings.Join(commands, "\n")
	if !strings.Contains(commandLog, "network create --internal") || !strings.Contains(commandLog, "network connect --alias seven-provider-proxy") {
		t.Fatalf("private network commands missing:\n%s", commandLog)
	}
	if strings.Contains(commandLog, " -p ") || strings.Contains(commandLog, "--publish") {
		t.Fatalf("proxy published a host port:\n%s", commandLog)
	}
	if strings.Contains(commandLog, "provider-calls.jsonl") || strings.Contains(commandLog, "upstreamApiKey") || strings.Contains(commandLog, "grantToken") {
		t.Fatalf("legacy Provider secret/audit path remains:\n%s", commandLog)
	}
	if !strings.Contains(commandLog, "dst=/run/seven/worker.token,readonly") || !strings.Contains(commandLog, "dst=/run/seven/control") {
		t.Fatalf("Gateway authority mounts missing:\n%s", commandLog)
	}
	specPath := filepath.Join(providerProxyControlDirectory(controlRoot, 2610240832, 3107414342), "provider.json")
	info, err := os.Stat(specPath)
	if err != nil || info.Mode().Perm() != 0o444 {
		t.Fatalf("provider spec info=%v err=%v", info, err)
	}
	var spec map[string]any
	body, _ := os.ReadFile(specPath)
	if err := json.Unmarshal(body, &spec); err != nil {
		t.Fatal(err)
	}
	if spec["gatewayBaseUrl"] != "http://host.docker.internal:9277" || spec["modelProtocol"] != "OPENAI" {
		t.Fatalf("unexpected Gateway spec: %+v", spec)
	}
	for _, forbidden := range []string{"upstreamBaseUrl", "upstreamApiKey", "grantToken", "grantId", "auditPath"} {
		if _, exists := spec[forbidden]; exists {
			t.Fatalf("legacy secret field %s remains", forbidden)
		}
	}
	proxyRuntime.cleanup()
	if _, err := os.Stat(filepath.Dir(specPath)); !os.IsNotExist(err) {
		t.Fatalf("control directory was not removed: %v", err)
	}
}

func TestProviderProxyControlSocketUsesShortDeterministicLocator(t *testing.T) {
	root := "/tmp/seven-workforce/provider-proxy"
	taskID := int64(2610240832)
	attemptID := int64(3107414342)

	first := providerProxyControlSocket(root, taskID, attemptID)
	second := providerProxyControlSocket(root, taskID, attemptID)
	other := providerProxyControlSocket(root, taskID, 3107414343)
	if first != second {
		t.Fatalf("locator is not deterministic: %q != %q", first, second)
	}
	if first == other {
		t.Fatalf("different Attempt identities share locator %q", first)
	}
	if len(first) >= 104 {
		t.Fatalf("socket path exceeds macOS AF_UNIX boundary: len=%d path=%q", len(first), first)
	}
	if strings.Contains(first, strconv.FormatInt(taskID, 10)) || strings.Contains(first, strconv.FormatInt(attemptID, 10)) {
		t.Fatalf("socket path retained unbounded business identity: %q", first)
	}
	if !strings.HasSuffix(first, "/socket/provider.sock") {
		t.Fatalf("unexpected socket path: %q", first)
	}
}

func TestGatewayURLForContainerKeepsNonLoopbackAndRewritesLoopback(t *testing.T) {
	loopback, err := gatewayURLForContainer("http://127.0.0.1:9277")
	if err != nil || loopback != "http://host.docker.internal:9277" {
		t.Fatalf("loopback=%q err=%v", loopback, err)
	}
	remote, err := gatewayURLForContainer("https://framework.internal")
	if err != nil || remote != "https://framework.internal" {
		t.Fatalf("remote=%q err=%v", remote, err)
	}
}

func TestProviderProxyControlUsesContainerExecInsteadOfHostSocket(t *testing.T) {
	runtime := NewDocker(config.WorkerConfig{}, config.ProviderProxyConfig{ControlRoot: "/tmp/unused-host-socket"})
	var container string
	var requestBody []byte
	runtime.providerProxyControlExec = func(_ context.Context, gotContainer string, body []byte) ([]byte, error) {
		container = gotContainer
		requestBody = append([]byte(nil), body...)
		return []byte(`{"ok":true,"ownerEpoch":"2","workerInstanceId":"wki_2","controlState":"READY"}`), nil
	}
	response, err := runtime.callProviderProxyControl(context.Background(), 2610240832, 3107414342, 1, map[string]any{
		"schemaVersion": 1, "command": "state", "attemptId": "3107414342", "generation": "1", "ownerEpoch": "2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !response.OK || response.OwnerEpoch != 2 || response.ControlState != "READY" {
		t.Fatalf("unexpected response: %+v", response)
	}
	if container != "seven-provider-proxy-3107414342" {
		t.Fatalf("container-local control target=%q", container)
	}
	if strings.Contains(string(requestBody), "/tmp/unused-host-socket") || strings.Contains(string(requestBody), "provider.sock") {
		t.Fatalf("host UDS leaked into control request: %s", requestBody)
	}
}
