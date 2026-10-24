package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

func TestConfiguredProviderRelayUsesExactImageAndPrivateAuthorityBoundary(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	tokenPath := filepath.Join(root, "worker.token")
	if err := os.WriteFile(tokenPath, []byte(strings.Repeat("t", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	proxyDigest := "sha256:" + strings.Repeat("a", 64)
	proxyImage := "seven-workforce/provider-proxy:review-slice7@" + proxyDigest
	reviewerImage := "sha256:" + strings.Repeat("c", 64)
	proxy := config.ProviderProxyConfig{
		Image:       proxyImage,
		ImageDigest: proxyDigest,
		Version:     "0.2.4-review-slice7",
		ControlRoot: filepath.Join(root, "control"),
	}
	d := NewDocker(config.WorkerConfig{
		ID:            1001,
		ServerBaseURL: "http://127.0.0.1:9278/api",
		TokenFile:     tokenPath,
		WorkspaceRoot: filepath.Join(root, "workspaces"),
	}, proxy)
	var mu sync.Mutex
	var commands [][]string
	d.exec = func(_ context.Context, name string, args ...string) ([]byte, error) {
		mu.Lock()
		commands = append(commands, append([]string{name}, args...))
		mu.Unlock()
		if name == "docker" && len(args) >= 3 && args[0] == "image" && args[1] == "inspect" {
			return []byte(proxyDigest), nil
		}
		if name == "docker" && len(args) >= 1 && args[0] == "create" {
			return []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"), nil
		}
		return nil, nil
	}
	work := Work{
		ReviewRunID:              2001,
		ReviewRunAttemptID:       2002,
		WorkerID:                 1001,
		WorkerInstanceID:         "worker-instance",
		HostBootID:               "host-boot",
		TaskID:                   3001,
		OwnerEpoch:               4,
		ReviewerImage:            reviewerImage,
		ReviewRoot:               filepath.Join(root, "reviews", "3001", "2001", "4"),
		LeaseToken:               strings.Repeat("l", 64),
		LeaseExpiresAt:           time.Now().UTC().Add(4 * time.Minute),
		ExecutionDeadline:        time.Now().UTC().Add(time.Minute),
		ProviderProxyImageDigest: proxyDigest,
		Provider: ProviderIdentity{
			CredentialID:       4001,
			CredentialVersion:  1,
			Protocol:           "OPENAI",
			PrimaryModel:       "review-model",
			StructuredContract: "independent-reviewer/structured-output/v1",
		},
	}
	relay, err := d.startConfiguredProviderRelay(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	providerSpecPath := filepath.Join(work.ReviewRoot, "provider-relay", "provider.json")
	specBody, err := os.ReadFile(providerSpecPath)
	if err != nil {
		t.Fatal(err)
	}
	specInfo, err := os.Stat(providerSpecPath)
	if err != nil {
		t.Fatalf("configured provider spec cannot be inspected: %v", err)
	}
	if specInfo.Mode().Perm() != 0o444 {
		t.Fatalf("configured provider spec is not read-only: mode=%v", specInfo.Mode())
	}
	if strings.Contains(string(specBody), work.LeaseToken) || strings.Contains(string(specBody), strings.Repeat("t", 64)) {
		t.Fatal("configured provider spec persisted raw authority material")
	}
	var persisted map[string]any
	if err := json.Unmarshal(specBody, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted["workerTokenFile"] != configuredRelayTokenPath || persisted["reviewLeaseFile"] != configuredRelayLeasePath {
		t.Fatalf("provider spec did not retain private in-container paths: %v", persisted)
	}
	if persisted["leaseExpiresAt"] != work.LeaseExpiresAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("provider spec did not freeze the current review lease expiry: %v", persisted["leaseExpiresAt"])
	}
	d.stopProviderRelay(relay)

	if _, err := os.Stat(providerSpecPath); !os.IsNotExist(err) {
		t.Fatalf("configured relay cleanup left provider spec behind: %v", err)
	}
	leasePath := filepath.Join(work.ReviewRoot, "provider-relay", "review.lease")
	if _, err := os.Stat(leasePath); !os.IsNotExist(err) {
		t.Fatalf("configured relay cleanup left lease behind: %v", err)
	}

	mu.Lock()
	deferredCommands := append([][]string(nil), commands...)
	mu.Unlock()
	var create []string
	for _, command := range deferredCommands {
		if len(command) > 1 && command[0] == "docker" && command[1] == "create" {
			create = command
			break
		}
	}
	if len(create) == 0 {
		t.Fatal("configured relay did not create a container")
	}
	joined := strings.Join(create, "\n")
	for _, required := range []string{
		"--network\nbridge",
		"--read-only",
		"--cap-drop\nALL",
		"--user\n10001:10001",
		"type=volume,src=seven-review-relay-2001-4-volume",
		"dst=/run/seven/secrets,readonly",
		"dst=/run/seven/provider.json,readonly",
		proxyImage,
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("configured relay create command misses %q: %s", required, joined)
		}
	}
	for _, forbidden := range []string{work.LeaseToken, strings.Repeat("t", 64), "X-Seven-Review-Run-Lease"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("configured relay create command contains secret/control material %q", forbidden)
		}
	}

	var specCommand []string
	for _, command := range deferredCommands {
		if len(command) > 2 && command[0] == "docker" && command[1] == "run" && strings.Contains(strings.Join(command, "\n"), "--mount\ntype=bind,src="+filepath.Join(work.ReviewRoot, "provider-relay", "review.lease")) {
			specCommand = command
			break
		}
	}
	if len(specCommand) == 0 {
		t.Fatal("configured relay did not stage a private lease volume")
	}
}

func TestConfiguredProviderRelayRecoveryRebindsFreshLeaseOverPrivateControlUDS(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	reviewerImage := "sha256:" + strings.Repeat("c", 64)
	proxyDigest := "sha256:" + strings.Repeat("a", 64)
	proxyName := "seven-workforce/provider-proxy:review-slice7@" + proxyDigest
	work := Work{
		ReviewRunID:              2001,
		ReviewRunAttemptID:       2002,
		WorkerID:                 1001,
		WorkerInstanceID:         "replacement-worker",
		HostBootID:               "replacement-boot",
		TaskID:                   3001,
		OwnerEpoch:               4,
		ReviewerImage:            reviewerImage,
		ReviewRoot:               filepath.Join(root, "reviews", "3001", "2001", "4"),
		LeaseToken:               strings.Repeat("n", 64),
		LeaseExpiresAt:           time.Now().UTC().Add(5 * time.Minute),
		ProviderProxyImageDigest: proxyDigest,
		Provider: ProviderIdentity{
			CredentialID:       4001,
			CredentialVersion:  1,
			Protocol:           "OPENAI",
			PrimaryModel:       "review-model",
			StructuredContract: "independent-reviewer/structured-output/v1",
		},
	}
	providerRoot := filepath.Join(work.ReviewRoot, "provider-relay")
	if err := os.MkdirAll(providerRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerRoot, "review.lease"), []byte(strings.Repeat("o", 64)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(providerRoot, "provider.json"), []byte(`{"executionKind":"REVIEW_RUN"}`), 0o444); err != nil {
		t.Fatal(err)
	}
	name := providerRelayName(work)
	containerID := strings.Repeat("1", 64)
	inspectBody := []byte(`[{"Id":"` + containerID + `","Image":"` + proxyDigest + `","State":{"Running":true},"Config":{"Labels":{"seven.review.relay":"true","seven.review.relay.kind":"configured","seven.review.run.id":"2001","seven.review.owner.epoch":"4","seven.review.image.digest":"` + proxyDigest + `"}}}]`)
	volumeBody := []byte(`[{"Name":"` + name + `-volume","Labels":{"seven.review.relay":"true","seven.review.run.id":"2001","seven.review.owner.epoch":"4"}}]`)
	secretVolumeBody := []byte(`[{"Name":"` + name + `-secret","Labels":{"seven.review.relay.secret":"true","seven.review.run.id":"2001","seven.review.owner.epoch":"4"}}]`)
	d := NewDocker(config.WorkerConfig{}, config.ProviderProxyConfig{Image: proxyName, ImageDigest: proxyDigest, Version: "0.2.4-review-slice7"})
	var input []byte
	var inputArgs []string
	d.exec = func(_ context.Context, command string, args ...string) ([]byte, error) {
		if command != "docker" || len(args) == 0 {
			return nil, nil
		}
		switch {
		case args[0] == "inspect" && len(args) > 1 && args[1] == name:
			return inspectBody, nil
		case args[0] == "volume" && len(args) > 2 && args[1] == "inspect" && args[2] == name+"-volume":
			return volumeBody, nil
		case args[0] == "volume" && len(args) > 2 && args[1] == "inspect" && args[2] == name+"-secret":
			return secretVolumeBody, nil
		case args[0] == "run":
			return nil, nil
		default:
			return nil, errors.New("unexpected Docker command")
		}
	}
	d.execInput = func(_ context.Context, body []byte, command string, args ...string) ([]byte, error) {
		input = append([]byte(nil), body...)
		inputArgs = append([]string{command}, args...)
		return []byte(`{"ok":true,"ownerEpoch":"4","workerInstanceId":"replacement-worker","leaseExpiresAt":"2099-01-01T00:00:00Z","controlState":"READY"}`), nil
	}
	relay, err := d.recoverConfiguredProviderRelay(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	if relay == nil || relay.id != containerID {
		t.Fatalf("recovered relay=%+v", relay)
	}
	if strings.Contains(strings.Join(inputArgs, "\x00"), work.LeaseToken) || strings.Contains(strings.Join(inputArgs, "\x00"), "review.lease") {
		t.Fatalf("fresh lease or lease path leaked into docker command arguments: %v", inputArgs)
	}
	var request map[string]any
	if err := json.Unmarshal(input, &request); err != nil {
		t.Fatal(err)
	}
	if request["command"] != "renew-authority" || request["ownerEpoch"] != "4" || request["leaseToken"] != work.LeaseToken {
		t.Fatalf("unexpected private rebind request: %#v", request)
	}
}

func TestConfiguredWorkerRejectsReviewWithoutFrozenProxyDigest(t *testing.T) {
	d := NewDocker(config.WorkerConfig{}, config.ProviderProxyConfig{
		Image:       "seven-workforce/provider-proxy:review-slice7@sha256:" + strings.Repeat("a", 64),
		ImageDigest: "sha256:" + strings.Repeat("a", 64),
		Version:     "acceptance",
	})
	work := Work{
		ReviewRunID: 1,
		WorkerID:    2,
		TaskID:      3,
		Provider: ProviderIdentity{
			CredentialID:       4,
			CredentialVersion:  1,
			Protocol:           "OPENAI",
			PrimaryModel:       "review-model",
			StructuredContract: "independent-reviewer/structured-output/v1",
		},
	}
	if _, err := d.startProviderRelay(context.Background(), work); err == nil || !strings.Contains(err.Error(), "REVIEW_PROVIDER_PROXY_IDENTITY_MISSING") {
		t.Fatalf("configured Worker accepted a review without frozen proxy identity: %v", err)
	}
}

// TestConfiguredProviderRelayDockerSmoke is an opt-in acceptance probe for
// the exact local proxy/reviewer images. The normal package suite keeps this
// out of the default run because it needs Docker and a private Worker token;
// Slice 7 runs it in the isolated acceptance environment.
func TestConfiguredProviderRelayDockerSmoke(t *testing.T) {
	proxyImage := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_PROVIDER_DOCKER_IMAGE"))
	proxyDigest := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_PROVIDER_DOCKER_DIGEST"))
	reviewerImage := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_DOCKER_IMAGE"))
	tokenPath := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_WORKER_TOKEN_FILE"))
	if proxyImage == "" || proxyDigest == "" || reviewerImage == "" || tokenPath == "" {
		t.Skip("REVIEW_RUNTIME_* acceptance image and token variables are required")
	}
	if !privateRegularFile(tokenPath) {
		t.Fatal("acceptance Worker token must be a private regular file")
	}
	root := t.TempDir()
	d := NewDocker(config.WorkerConfig{
		ID:            1001,
		ServerBaseURL: "http://127.0.0.1:9278/api",
		TokenFile:     tokenPath,
		WorkspaceRoot: filepath.Join(root, "workspaces"),
	}, config.ProviderProxyConfig{Image: proxyImage, ImageDigest: proxyDigest, Version: "acceptance", ControlRoot: filepath.Join(root, "control")})
	d.exec = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		output, err := commandOutput(ctx, name, args...)
		if err != nil {
			t.Logf("acceptance command failed: %s %v: %s", name, args, strings.TrimSpace(string(output)))
		}
		return output, err
	}
	work := Work{
		ReviewRunID:              7001,
		ReviewRunAttemptID:       7002,
		WorkerID:                 1001,
		WorkerInstanceID:         "acceptance-worker",
		HostBootID:               "acceptance-boot",
		TaskID:                   7003,
		OwnerEpoch:               1,
		ReviewerImage:            reviewerImage,
		ReviewRoot:               filepath.Join(root, "reviews", "7001", "1"),
		LeaseToken:               strings.Repeat("l", 64),
		LeaseExpiresAt:           time.Now().UTC().Add(4 * time.Minute),
		ExecutionDeadline:        time.Now().UTC().Add(30 * time.Second),
		ProviderProxyImageDigest: proxyDigest,
		Provider: ProviderIdentity{
			CredentialID:       7004,
			CredentialVersion:  1,
			Protocol:           "OPENAI",
			PrimaryModel:       "acceptance-model",
			StructuredContract: "independent-reviewer/structured-output/v1",
		},
	}
	relay, err := d.startConfiguredProviderRelay(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	if relay == nil || relay.id == "" {
		t.Fatal("configured provider relay did not start")
	}
	d.stopProviderRelay(relay)
}
