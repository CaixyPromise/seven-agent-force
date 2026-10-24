package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

// TestNetworkOperationDockerEgressAndCleanup is intentionally opt-in because
// it exercises the real Docker daemon and public npm registry. The image must
// be supplied by immutable image ID so the gateway binary and runner are the
// same admitted runtime identity.
func TestNetworkOperationDockerEgressAndCleanup(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("EXECUTION_OPERATION_DOCKER_IMAGE"))
	if image == "" {
		t.Skip("EXECUTION_OPERATION_DOCKER_IMAGE requires an exact local image ID")
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		t.Fatal("EXECUTION_OPERATION_DOCKER_IMAGE must be an exact sha256 image ID")
	}

	// A cold Corepack cache may need several minutes to fetch the pinned pnpm
	// binary on developer machines. Keep this opt-in integration test below the
	// production Operation limit while leaving enough time for the subsequent
	// egress-denial probes to run.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Join(root, "repo")
	if err = os.Mkdir(repository, 0o700); err != nil {
		t.Fatal(err)
	}
	probe := `const net=require("net");
function fail(x){console.error(x);process.exit(1)}
function blocked(host,port){return new Promise((resolve,reject)=>{const s=net.connect({host,port});let done=false;const finish=ok=>{if(done)return;done=true;s.destroy();ok?resolve():reject(new Error(host+":"+port+" reachable"))};s.setTimeout(900,()=>finish(true));s.on("error",()=>finish(true));s.on("connect",()=>finish(false))})}
Promise.all([
  blocked("registry.npmjs.org",80),
  blocked("example.com",443),
  blocked("host.docker.internal",80),
  blocked("1.1.1.1",443),
  blocked("10.0.0.1",443),
  blocked("169.254.169.254",80)
]).then(()=>{console.log("approved=reachable forbidden-targets=blocked private-worker-root=absent");process.exit(0)}).catch(e=>fail(e.message));`
	if err = os.WriteFile(filepath.Join(repository, "probe.js"), []byte(probe), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(repository, "package.json"), []byte(`{"name":"network-operation-fixture","version":"1.0.0","private":true,"dependencies":{"is-number":"7.0.0"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	journal, err := resourcejournal.Open(filepath.Join(root, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	journal.SetContainerLimit(4)
	operationID := time.Now().UnixNano() & 0x1fffffffffffffff
	runtime := newNetworkOperationRuntime(root, journal, "https://dns.google/resolve")
	claim := networkOperationClaim{
		OperationID: operationID, OperationAttemptID: operationID + 1,
		TaskID: operationID + 2, TaskTurnID: operationID + 3,
		AttemptID: operationID + 4, OwnerEpoch: 1,
		DescriptorDigest: strings.Repeat("a", 64), Purpose: "verify fixed npm egress",
		Command: "test ! -e /run/seven-operation && pnpm install && test -d node_modules/is-number && node probe.js", CWD: ".",
		Targets:      []networkOperationTarget{{Host: "registry.npmjs.org", Protocol: "https", Port: 443}},
		HarnessImage: image, RepositoryRoot: repository, Deadline: time.Now().Add(4 * time.Minute),
	}
	base := fmt.Sprintf("seven-op-%d", operationID)
	t.Cleanup(func() {
		_ = runtime.Cleanup(operationID)
		// Exact-name fallback is test-owned and is only reached if the journal
		// itself is what the test is diagnosing.
		for _, name := range []string{base + "-runner", base + "-gw-1"} {
			_, _ = exec.Command("docker", "container", "rm", "-f", name).CombinedOutput()
		}
		for _, name := range []string{base + "-int", base + "-out"} {
			_, _ = exec.Command("docker", "network", "rm", name).CombinedOutput()
		}
	})

	claim, err = runtime.Prepare(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("prepared runner=%s relay=%s", claim.RuntimeContainerID, claim.RelayIdentity[:12])
	result, err := runtime.Execute(ctx, claim)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "SUCCEEDED" || result.ExitCode != 0 || !strings.Contains(result.Output, "approved=reachable forbidden-targets=blocked private-worker-root=absent") {
		t.Fatalf("unexpected operation result: status=%s exit=%d output=%q", result.Status, result.ExitCode, result.Output)
	}
	if err = runtime.Cleanup(operationID); err != nil {
		t.Fatal(err)
	}
	if journal.Pending() != 0 {
		t.Fatalf("operation cleanup remains pending: %d", journal.Pending())
	}
	for _, target := range [][]string{{"container", "inspect", base + "-runner"}, {"container", "inspect", base + "-gw-1"}, {"network", "inspect", base + "-int"}, {"network", "inspect", base + "-out"}} {
		if output, inspectErr := exec.CommandContext(ctx, "docker", target...).CombinedOutput(); inspectErr == nil || !missingDockerResource(string(output), inspectErr) {
			t.Fatalf("resource remains or absence is unproven: docker %s: %s (%v)", strings.Join(target, " "), output, inspectErr)
		}
	}
	if _, err = os.Stat(filepath.Join(root, ".seven", "execution-operations", fmt.Sprint(operationID))); !os.IsNotExist(err) {
		t.Fatalf("operation result directory remains: %v", err)
	}
	t.Log("approved hostname reached through fixed relay; unapproved DNS, wrong port, host alias, direct/public/private/metadata IPs and Worker-private result root were blocked; resources removed")
}

func missingDockerResource(output string, err error) bool {
	if err == nil {
		return false
	}
	value := strings.ToLower(output + " " + err.Error())
	return strings.Contains(value, "no such container") || strings.Contains(value, "no such network") || strings.Contains(value, "not found")
}
