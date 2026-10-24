package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	processorcheckpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
)

// runCustomDelivery executes the optional deliver(context) hook in a
// short-lived, networkless container. P0.9 accepts its output only as a
// candidate: without an independently configured target verifier the Request
// settles in MANUAL_REVIEW, never in a fabricated success state.
func (w *worker) runCustomDelivery(ctx context.Context, message deliverydomain.Message, claim deliveryapp.DeliveryClaim, key []byte, root string) error {
	fail := func(reason string) error {
		if reportErr := w.control.FailDelivery(context.Background(), message, claim.LeaseToken, reason); reportErr != nil {
			return reportErr
		}
		return nil
	}
	if claim.ProcessorReleaseID == 0 || len(claim.ProcessorDigest) != 64 || claim.ProcessorArchiveObjectKey == "" || claim.HarnessImageDigest == "" {
		return fail("CUSTOM_DELIVERY_IDENTITY_INVALID")
	}
	mount, err := w.processors.Materialize(ctx, claim.ProcessorReleaseID, claim.ProcessorDigest, claim.ProcessorArchiveObjectKey)
	if err != nil {
		return fail("CUSTOM_DELIVERY_PROCESSOR_UNAVAILABLE")
	}
	bundle, err := w.downloadCheckpointPlain(ctx, claim.RepositoryBundleObjectKey, claim.RepositoryBundleObjectSHA256, claim.RepositoryBundleContentSHA256, claim.RepositoryBundleSizeBytes, key, claim.CheckpointIdentity, "repository-bundle", root, processorcheckpoint.MaxArchiveBytes)
	if err != nil {
		return fail("DELIVERY_CHECKPOINT_TAMPERED")
	}
	defer removeClosed(bundle)
	repository := filepath.Join(root, "repository")
	runtimeRoot := filepath.Join(root, "runtime")
	if err = os.MkdirAll(repository, 0o700); err != nil {
		return fail("CUSTOM_DELIVERY_SETUP_FAILED")
	}
	if _, err = controlledWorkerGit(ctx, repository, "init", "-q"); err != nil {
		return fail("DELIVERY_BUNDLE_INVALID")
	}
	ref := "refs/seven/checkpoints/" + claim.CheckpointIdentity
	if _, err = controlledWorkerGit(ctx, repository, "-c", "protocol.file.allow=always", "fetch", "-q", bundle.Name(), ref+":"+ref); err != nil {
		return fail("DELIVERY_BUNDLE_INVALID")
	}
	if _, err = controlledWorkerGit(ctx, repository, "checkout", "--detach", "-q", claim.ResultCommit); err != nil {
		return fail("DELIVERY_RESULT_IDENTITY_MISMATCH")
	}
	tree, err := controlledWorkerGit(ctx, repository, "rev-parse", "HEAD^{tree}")
	if err != nil || tree != claim.ResultTree {
		return fail("DELIVERY_RESULT_IDENTITY_MISMATCH")
	}
	if err = os.MkdirAll(runtimeRoot, 0o777); err != nil {
		return fail("CUSTOM_DELIVERY_SETUP_FAILED")
	}
	if err = os.Chmod(repository, 0o755); err != nil {
		return fail("CUSTOM_DELIVERY_SETUP_FAILED")
	}
	deliveryParameters := map[string]any{
		"deliveryAttemptId": strconv.FormatInt(claim.AttemptID, 10), "requestId": strconv.FormatInt(claim.RequestID, 10), "taskTurnId": strconv.FormatInt(claim.TaskTurnID, 10),
		"operation": claim.Action.Operation, "payload": claim.Action.Payload,
		"result": map[string]string{"commit": claim.ResultCommit, "tree": claim.ResultTree, "selectionDigest": claim.SelectionDigest},
		"target": map[string]any{"id": strconv.FormatInt(claim.Target.ID, 10), "name": claim.Target.Name, "transport": claim.Target.Transport},
	}
	// The protected bootstrap uses one context shape for process() and
	// deliver(). Delivery receives a synthetic cycle identity and the approved
	// DeliveryContext as parameters; it never receives the Coding Session.
	contextBody, _ := json.Marshal(map[string]any{
		"taskId": strconv.FormatInt(claim.TaskID, 10), "attemptId": strconv.FormatInt(claim.AttemptID, 10),
		"cycleId": "delivery:" + strconv.FormatInt(claim.AttemptID, 10), "cycleNumber": claim.AttemptNumber,
		"workspaceRepo": "/workspace", "parameters": deliveryParameters,
	})
	spec := map[string]any{
		"schemaVersion": 1, "deliveryAttemptId": strconv.FormatInt(claim.AttemptID, 10), "processorDigest": claim.ProcessorDigest,
		"processorRoot": filepath.Join("/opt/seven-worker/result-processors", claim.ProcessorDigest), "resultRoot": "/workspace",
		"runtimeRoot": "/run/seven-delivery", "resultPath": "/run/seven-delivery/result.json",
		"deliveryContext": json.RawMessage(contextBody), "materializationPath": "/run/seven-delivery/materialization.json",
	}
	specBody, _ := json.Marshal(spec)
	if err = os.WriteFile(filepath.Join(runtimeRoot, "spec.json"), specBody, 0o666); err != nil {
		return fail("CUSTOM_DELIVERY_SETUP_FAILED")
	}
	if err = os.WriteFile(filepath.Join(runtimeRoot, "materialization.json"), []byte(`{"variables":{}}`), 0o666); err != nil {
		return fail("CUSTOM_DELIVERY_SETUP_FAILED")
	}
	containerName := customDeliveryContainerName(claim.AttemptID, claim.AttemptNumber)
	egress, err := w.startCustomDeliveryEgress(ctx, claim)
	if err != nil {
		return fail(customDeliveryEgressReason(err))
	}
	defer egress.cleanup()
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	defer exec.Command("docker", "rm", "-f", containerName).Run()
	args := []string{
		"run", "--rm", "--name", containerName, "--network", egress.runnerNetwork, "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--pids-limit", "128", "--memory", "512m", "--cpus", "1.0", "--user", "10001:10001", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/opt/seven-worker/result-processors/%s,readonly", mount.Source, claim.ProcessorDigest),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/workspace,readonly", repository),
		"--mount", fmt.Sprintf("type=bind,src=%s,dst=/run/seven-delivery", runtimeRoot),
		"--entrypoint", "/usr/local/bin/seven-worker-cli", claim.HarnessImageDigest, "result-deliver", "--spec", "/run/seven-delivery/spec.json",
	}
	output, runErr := exec.CommandContext(runCtx, "docker", args...).CombinedOutput()
	if len(output) > 64<<10 {
		return fail("CUSTOM_DELIVERY_OUTPUT_LIMIT")
	}
	if runErr != nil {
		if runCtx.Err() != nil {
			return fail("CUSTOM_DELIVERY_TIMEOUT")
		}
		return fail("CUSTOM_DELIVERY_EXECUTION_FAILED")
	}
	candidate, err := os.ReadFile(filepath.Join(runtimeRoot, "result.json"))
	if err != nil || len(candidate) == 0 || len(candidate) > 1<<20 || !json.Valid(candidate) || strings.Contains(strings.ToLower(string(candidate)), "secret") {
		return fail("CUSTOM_DELIVERY_CONTRACT_INVALID")
	}
	// The default networkless runner intentionally has no target verifier. The
	// candidate remains available in audit diagnostics only by digest; a future
	// admitted verifier may promote it without re-running the Agent.
	return fail("CUSTOM_DELIVERY_REQUIRES_VERIFIER")
}

// customDeliveryEgress is a per-DeliveryAttempt pair of Docker networks. The
// Runner joins only the internal network; the gateway joins that network and a
// private outbound network. Docker's network alias maps the target hostname to
// the gateway, preserving TLS SNI / SSH host identity while preventing direct
// DNS or IP connections from the Runner. The relay receives a single frozen
// IP:port, so it has no route-selection authority of its own.
type customDeliveryEgress struct {
	runnerNetwork  string
	gatewayNetwork string
	gatewayName    string
}

func (r customDeliveryEgress) cleanup() {
	if r.gatewayName != "" {
		_ = exec.Command("docker", "rm", "-f", r.gatewayName).Run()
	}
	if r.runnerNetwork != "" {
		_ = exec.Command("docker", "network", "rm", r.runnerNetwork).Run()
	}
	if r.gatewayNetwork != "" {
		_ = exec.Command("docker", "network", "rm", r.gatewayNetwork).Run()
	}
}

func (w *worker) startCustomDeliveryEgress(ctx context.Context, claim deliveryapp.DeliveryClaim) (customDeliveryEgress, error) {
	if claim.Target.Transport == deliverydomain.TransportLocalBare || strings.TrimSpace(claim.Target.Host) == "" {
		return customDeliveryEgress{}, errors.New("NETWORKLESS_TARGET_UNSUPPORTED")
	}
	port := claim.Target.Port
	if port == 0 {
		if claim.Target.Transport == deliverydomain.TransportSSH {
			port = 22
		} else {
			port = 443
		}
	}
	target := claim.Target
	target.Port = port
	addresses, err := resolveDeliveryEgress(ctx, target, func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	})
	if err != nil || len(addresses) != 1 || addresses[0] == nil {
		return customDeliveryEgress{}, errors.New("DELIVERY_EGRESS_TARGET_UNAVAILABLE")
	}
	base := customDeliveryContainerName(claim.AttemptID, claim.AttemptNumber)
	base = strings.TrimPrefix(base, "seven-delivery-")
	runtime := customDeliveryEgress{
		runnerNetwork:  "seven-delivery-int-" + base,
		gatewayNetwork: "seven-delivery-out-" + base,
		gatewayName:    "seven-delivery-egress-" + base,
	}
	// Always clear a previously interrupted attempt's exact resources. Names
	// are scoped by immutable DeliveryAttempt and cannot target unrelated work.
	runtime.cleanup()
	if output, commandErr := exec.CommandContext(ctx, "docker", "network", "create", "--driver", "bridge", "--internal", "--label", "io.github.caixypromise.seven-worker.delivery-egress=internal", runtime.runnerNetwork).CombinedOutput(); commandErr != nil || len(output) > 4<<10 {
		return customDeliveryEgress{}, errors.New("DELIVERY_EGRESS_NETWORK_UNAVAILABLE")
	}
	if output, commandErr := exec.CommandContext(ctx, "docker", "network", "create", "--driver", "bridge", "--label", "io.github.caixypromise.seven-worker.delivery-egress=outbound", runtime.gatewayNetwork).CombinedOutput(); commandErr != nil || len(output) > 4<<10 {
		runtime.cleanup()
		return customDeliveryEgress{}, errors.New("DELIVERY_EGRESS_NETWORK_UNAVAILABLE")
	}
	deadline := time.Now().UTC().Add(130 * time.Second).Format(time.RFC3339)
	args := []string{
		"run", "-d", "--name", runtime.gatewayName, "--network", runtime.gatewayNetwork,
		"--read-only", "--cap-drop", "ALL", "--cap-add", "NET_BIND_SERVICE", "--security-opt", "no-new-privileges",
		"--pids-limit", "64", "--memory", "128m", "--cpus", "0.25", "--user", "10001:10001", "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m",
		"--entrypoint", "/usr/local/bin/seven-worker-cli", claim.HarnessImageDigest,
		"delivery-egress-gateway", "--listen-port", strconv.Itoa(port), "--target-ip", addresses[0].String(), "--target-port", strconv.Itoa(port), "--deadline", deadline,
	}
	if output, commandErr := exec.CommandContext(ctx, "docker", args...).CombinedOutput(); commandErr != nil || len(output) > 4<<10 {
		runtime.cleanup()
		return customDeliveryEgress{}, errors.New("DELIVERY_EGRESS_GATEWAY_UNAVAILABLE")
	}
	if output, commandErr := exec.CommandContext(ctx, "docker", "network", "connect", "--alias", target.Host, runtime.runnerNetwork, runtime.gatewayName).CombinedOutput(); commandErr != nil || len(output) > 4<<10 {
		runtime.cleanup()
		return customDeliveryEgress{}, errors.New("DELIVERY_EGRESS_GATEWAY_UNAVAILABLE")
	}
	// A short inspect after connecting turns an immediate bind/entrypoint error
	// into a fail-closed setup error before the Processor ever receives a
	// networked execution context.
	time.Sleep(150 * time.Millisecond)
	status, statusErr := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Running}}", runtime.gatewayName).Output()
	if statusErr != nil || strings.TrimSpace(string(status)) != "true" {
		runtime.cleanup()
		return customDeliveryEgress{}, errors.New("DELIVERY_EGRESS_GATEWAY_UNAVAILABLE")
	}
	return runtime, nil
}

func customDeliveryEgressReason(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "DELIVERY_EGRESS_TARGET_UNAVAILABLE", "DELIVERY_EGRESS_NETWORK_UNAVAILABLE", "DELIVERY_EGRESS_GATEWAY_UNAVAILABLE":
		return err.Error()
	default:
		return "DELIVERY_EGRESS_CAPABILITY_UNAVAILABLE"
	}
}

func customDeliveryContainerName(attemptID int64, number int) string {
	value := strconv.FormatInt(attemptID, 10)
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, value)
	if len(value) > 40 {
		value = value[:40]
	}
	if value == "" {
		value = "delivery"
	}
	return fmt.Sprintf("seven-delivery-%s-n%d", value, number)
}

var _ = errors.New
