package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

const (
	operationOutputLimit      = 8 << 20
	operationAgentOutputLimit = 64 << 10
	operationDefaultTimeout   = 30 * time.Minute
)

type networkOperationTarget struct {
	Host     string `json:"host"`
	Protocol string `json:"protocol"`
	Port     int    `json:"port"`
}

// networkOperationClaim is intentionally a Worker-owned transport shape. The
// Server machine client can map its authoritative claim into this structure
// without allowing a container to choose image, workspace or deadline.
type networkOperationClaim struct {
	OperationID        int64
	OperationAttemptID int64
	TaskID             int64
	TaskTurnID         int64
	AttemptID          int64
	OwnerEpoch         int64
	DescriptorDigest   string
	Purpose            string
	Command            string
	CWD                string
	Targets            []networkOperationTarget
	HarnessImage       string
	RepositoryRoot     string
	Deadline           time.Time
	RuntimeContainerID string
	RelayIdentity      string
}

type preparedNetworkOperation struct {
	claim  networkOperationClaim
	root   string
	runner string
}

type networkOperationResult struct {
	OperationID  int64  `json:"operationId,string"`
	Status       string `json:"status"`
	ExitCode     int    `json:"exitCode,omitempty"`
	ReasonCode   string `json:"reasonCode,omitempty"`
	Output       string `json:"output,omitempty"`
	OutputDigest string `json:"outputDigest,omitempty"`
	ResultDigest string `json:"resultDigest"`
	StartedAt    string `json:"startedAt,omitempty"`
	FinishedAt   string `json:"finishedAt"`
}

type operationResolver func(context.Context, string) ([]net.IP, error)

type networkOperationRuntime struct {
	workspaceRoot string
	journal       *resourcejournal.Journal
	run           resourcejournal.Command
	resolve       operationResolver
	now           func() time.Time
}

func newNetworkOperationRuntime(workspaceRoot string, journal *resourcejournal.Journal, dnsOverHTTPSURL ...string) *networkOperationRuntime {
	dohURL := ""
	if len(dnsOverHTTPSURL) > 0 {
		dohURL = strings.TrimSpace(dnsOverHTTPSURL[0])
	}
	return &networkOperationRuntime{
		workspaceRoot: workspaceRoot,
		journal:       journal,
		run:           boundedDockerCommand,
		resolve: func(ctx context.Context, host string) ([]net.IP, error) {
			return resolveOperationAddresses(ctx, host, dohURL)
		},
		now: time.Now,
	}
}

const operationDNSResponseLimit = 64 << 10

// resolveOperationAddresses keeps synthetic DNS ranges fail closed. Some
// desktop VPN/TUN resolvers intentionally return RFC 2544 fake IPs for every
// hostname; when the operator configured a trusted HTTPS resolver, use its
// authenticated DNS answer and apply the same public-address filter below.
// The Task and Agent can neither select nor override this resolver.
func resolveOperationAddresses(ctx context.Context, host, dohURL string) ([]net.IP, error) {
	addresses, systemErr := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if hasPublicOperationIP(addresses) || strings.TrimSpace(dohURL) == "" {
		return addresses, systemErr
	}
	dohAddresses, dohErr := resolveOperationDoH(ctx, http.DefaultClient, dohURL, host)
	if dohErr == nil && hasPublicOperationIP(dohAddresses) {
		return dohAddresses, nil
	}
	if systemErr != nil {
		return nil, systemErr
	}
	if dohErr != nil {
		return nil, dohErr
	}
	return addresses, nil
}

func hasPublicOperationIP(addresses []net.IP) bool {
	for _, address := range addresses {
		if publicOperationIP(address) {
			return true
		}
	}
	return false
}

func resolveOperationDoH(ctx context.Context, client *http.Client, endpoint, host string) ([]net.IP, error) {
	if client == nil {
		return nil, errors.New("OPERATION_DNS_UNAVAILABLE")
	}
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("OPERATION_DNS_UNAVAILABLE")
	}
	var addresses []net.IP
	for _, recordType := range []string{"A", "AAAA"} {
		requestURL := *parsed
		query := requestURL.Query()
		query.Set("name", host)
		query.Set("type", recordType)
		requestURL.RawQuery = query.Encode()
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if requestErr != nil {
			return nil, errors.New("OPERATION_DNS_UNAVAILABLE")
		}
		request.Header.Set("Accept", "application/dns-json")
		response, requestErr := client.Do(request)
		if requestErr != nil {
			return nil, errors.New("OPERATION_DNS_UNAVAILABLE")
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, operationDNSResponseLimit+1))
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || len(body) > operationDNSResponseLimit {
			return nil, errors.New("OPERATION_DNS_UNAVAILABLE")
		}
		var wire struct {
			Status int `json:"Status"`
			Answer []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			} `json:"Answer"`
		}
		if json.Unmarshal(body, &wire) != nil || wire.Status != 0 {
			return nil, errors.New("OPERATION_DNS_UNAVAILABLE")
		}
		for _, answer := range wire.Answer {
			if answer.Type != 1 && answer.Type != 28 {
				continue
			}
			if address := net.ParseIP(strings.TrimSpace(answer.Data)); address != nil {
				addresses = append(addresses, address)
			}
		}
	}
	if len(addresses) == 0 {
		return nil, errors.New("OPERATION_DNS_UNAVAILABLE")
	}
	return addresses, nil
}

func boundedDockerCommand(ctx context.Context, name string, args ...string) (string, error) {
	if name != "docker" {
		return "", errors.New("OPERATION_COMMAND_FORBIDDEN")
	}
	var output boundedOperationBuffer
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	if output.exceeded {
		return output.String(), errors.New("OPERATION_OUTPUT_LIMIT")
	}
	return output.String(), err
}

type boundedOperationBuffer struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (b *boundedOperationBuffer) Write(value []byte) (int, error) {
	original := len(value)
	remaining := operationOutputLimit - b.buffer.Len()
	if remaining <= 0 {
		b.exceeded = true
		return original, nil
	}
	if len(value) > remaining {
		value = value[:remaining]
		b.exceeded = true
	}
	_, _ = b.buffer.Write(value)
	return original, nil
}

func (b *boundedOperationBuffer) String() string { return b.buffer.String() }

func (r *networkOperationRuntime) Prepare(ctx context.Context, claim networkOperationClaim) (networkOperationClaim, error) {
	if r == nil || r.journal == nil || r.run == nil || r.resolve == nil || r.now == nil {
		return networkOperationClaim{}, errors.New("OPERATION_RUNTIME_UNAVAILABLE")
	}
	validated, err := validateNetworkOperationClaim(claim, r.workspaceRoot)
	if err != nil {
		return networkOperationClaim{}, err
	}
	root := filepath.Join(r.workspaceRoot, ".seven", "execution-operations", strconv.FormatInt(claim.OperationID, 10))
	if prepared, ok, readErr := readPreparedNetworkOperation(root, claim); readErr != nil {
		return networkOperationClaim{}, readErr
	} else if ok {
		claim.RuntimeContainerID, claim.RelayIdentity = prepared.RuntimeContainerID, prepared.RelayIdentity
		return claim, nil
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		return networkOperationClaim{}, errors.New("OPERATION_RESULT_DIRECTORY_UNAVAILABLE")
	}
	labels := operationLabels(claim)
	if _, err = r.journal.TrackDirectory(root, labels); err != nil {
		return networkOperationClaim{}, err
	}
	// Once the first resource is reserved, every pre-start failure must turn
	// the exact Operation group into retryable cleanup work. After started.json
	// is durable, resources and readback are retained so recovery can return a
	// saved result or OUTCOME_UNKNOWN instead of rerunning the command.
	prepared := false
	defer func() {
		if !prepared {
			_ = r.cleanupMatching(claim.OperationID)
		}
	}()

	deadline := claim.Deadline
	if deadline.IsZero() || deadline.After(r.now().Add(operationDefaultTimeout)) {
		deadline = r.now().Add(operationDefaultTimeout)
	}
	if !deadline.After(r.now()) {
		return networkOperationClaim{}, errors.New("OPERATION_AUTHORITY_EXPIRED")
	}
	operationCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	base := "seven-op-" + strconv.FormatInt(claim.OperationID, 10)
	internalNetwork, outboundNetwork := base+"-int", base+"-out"
	internalID, err := r.createNetwork(operationCtx, internalNetwork, true, labels)
	if err != nil {
		return networkOperationClaim{}, err
	}
	outboundID, err := r.createNetwork(operationCtx, outboundNetwork, false, labels)
	if err != nil {
		return networkOperationClaim{}, err
	}
	var gatewayIDs []string
	hostMappings := make(map[string]string, len(validated.targets))
	for index, target := range validated.targets {
		ip, resolveErr := r.resolveTarget(operationCtx, target.Host)
		if resolveErr != nil {
			return networkOperationClaim{}, resolveErr
		}
		gateway := fmt.Sprintf("%s-gw-%d", base, index+1)
		gatewayID, internalIP, createErr := r.createGateway(operationCtx, gateway, internalNetwork, outboundNetwork, claim.HarnessImage, target, ip, deadline, labels)
		if createErr != nil {
			return networkOperationClaim{}, createErr
		}
		gatewayIDs = append(gatewayIDs, gatewayID)
		hostMappings[target.Host] = internalIP
	}

	runner := base + "-runner"
	runnerID, err := r.createRunner(operationCtx, runner, internalNetwork, claim, validated.containerCWD, hostMappings, labels)
	if err != nil {
		return networkOperationClaim{}, err
	}
	if err = r.verifyInternalNetworkMembers(operationCtx, internalNetwork, runner, len(gatewayIDs)); err != nil {
		return networkOperationClaim{}, err
	}
	sort.Strings(gatewayIDs)
	relayBody, _ := json.Marshal(struct {
		InternalNetwork string   `json:"internalNetwork"`
		OutboundNetwork string   `json:"outboundNetwork"`
		Gateways        []string `json:"gateways"`
	}{internalID, outboundID, gatewayIDs})
	relayDigest := sha256.Sum256(relayBody)
	claim.RuntimeContainerID = strings.TrimSpace(runnerID)
	claim.RelayIdentity = hex.EncodeToString(relayDigest[:])
	if err = writeOperationJSON(filepath.Join(root, "prepared.json"), struct {
		OperationID        int64  `json:"operationId,string"`
		DescriptorDigest   string `json:"descriptorDigest"`
		RuntimeContainerID string `json:"runtimeContainerId"`
		RelayIdentity      string `json:"relayIdentity"`
	}{claim.OperationID, claim.DescriptorDigest, claim.RuntimeContainerID, claim.RelayIdentity}); err != nil {
		return networkOperationClaim{}, err
	}
	prepared = true
	return claim, nil
}

func (r *networkOperationRuntime) Execute(ctx context.Context, claim networkOperationClaim) (networkOperationResult, error) {
	if claim.RuntimeContainerID == "" || claim.RelayIdentity == "" {
		return networkOperationResult{}, errors.New("OPERATION_RUNTIME_IDENTITY_INVALID")
	}
	root := filepath.Join(r.workspaceRoot, ".seven", "execution-operations", strconv.FormatInt(claim.OperationID, 10))
	if result, ok, readErr := readNetworkOperationResult(root, claim); readErr != nil {
		return networkOperationResult{}, readErr
	} else if ok {
		return result, nil
	}
	if _, statErr := os.Stat(filepath.Join(root, "started.json")); statErr == nil {
		return r.persistOutcomeUnknown(root, claim, "OPERATION_RESULT_NOT_RECOVERABLE")
	} else if !os.IsNotExist(statErr) {
		return networkOperationResult{}, errors.New("OPERATION_READBACK_UNAVAILABLE")
	}
	runner := "seven-op-" + strconv.FormatInt(claim.OperationID, 10) + "-runner"
	startedAt := r.now().UTC()
	if err := writeOperationJSON(filepath.Join(root, "started.json"), map[string]string{
		"operationId": strconv.FormatInt(claim.OperationID, 10), "descriptorDigest": claim.DescriptorDigest,
		"runnerId": claim.RuntimeContainerID, "startedAt": startedAt.Format(time.RFC3339Nano),
	}); err != nil {
		return networkOperationResult{}, err
	}
	deadline := claim.Deadline
	if deadline.IsZero() || deadline.After(r.now().Add(operationDefaultTimeout)) {
		deadline = r.now().Add(operationDefaultTimeout)
	}
	operationCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	output, runErr := r.journal.Execute(operationCtx, r.run, "docker", "start", "-a", runner)
	finishedAt := r.now().UTC()
	result := networkOperationResult{OperationID: claim.OperationID, StartedAt: startedAt.Format(time.RFC3339Nano), FinishedAt: finishedAt.Format(time.RFC3339Nano)}
	result.Output, result.OutputDigest = safeOperationOutput(output)
	if runErr != nil {
		if operationCtx.Err() != nil {
			result.Status, result.ReasonCode = "FAILED", "OPERATION_TIMEOUT"
		} else if strings.Contains(runErr.Error(), "OPERATION_OUTPUT_LIMIT") {
			result.Status, result.ReasonCode = "FAILED", "OPERATION_OUTPUT_LIMIT"
		} else {
			result.Status, result.ReasonCode = "FAILED", "OPERATION_COMMAND_FAILED"
		}
		result.ExitCode = dockerExitCode(runErr)
	} else {
		result.Status = "SUCCEEDED"
	}
	return persistNetworkOperationResult(root, claim, result)
}

type validatedNetworkOperation struct {
	containerCWD string
	targets      []networkOperationTarget
}

var operationHostPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)

func validateNetworkOperationClaim(claim networkOperationClaim, workspaceRoot string) (validatedNetworkOperation, error) {
	if claim.OperationID <= 0 || claim.TaskID <= 0 || claim.AttemptID <= 0 || claim.OwnerEpoch <= 0 || len(claim.DescriptorDigest) != 64 {
		return validatedNetworkOperation{}, errors.New("OPERATION_IDENTITY_INVALID")
	}
	if _, err := hex.DecodeString(claim.DescriptorDigest); err != nil || len(claim.Purpose) > 2<<10 || strings.TrimSpace(claim.Command) == "" || len(claim.Command) > 16<<10 || len(claim.Targets) == 0 || len(claim.Targets) > 8 || strings.TrimSpace(claim.HarnessImage) == "" {
		return validatedNetworkOperation{}, errors.New("OPERATION_DESCRIPTOR_INVALID")
	}
	root, err := filepath.EvalSymlinks(claim.RepositoryRoot)
	if err != nil || !filepath.IsAbs(root) {
		return validatedNetworkOperation{}, errors.New("OPERATION_WORKSPACE_INVALID")
	}
	cwd := filepath.Clean(strings.TrimSpace(claim.CWD))
	if cwd == "" {
		cwd = "."
	}
	if filepath.IsAbs(cwd) || cwd == ".." || strings.HasPrefix(cwd, ".."+string(filepath.Separator)) {
		return validatedNetworkOperation{}, errors.New("OPERATION_CWD_INVALID")
	}
	hostCWD, err := filepath.EvalSymlinks(filepath.Join(root, cwd))
	if err != nil || hostCWD != root && !strings.HasPrefix(hostCWD, root+string(filepath.Separator)) {
		return validatedNetworkOperation{}, errors.New("OPERATION_CWD_INVALID")
	}
	if workspaceRoot == "" || !filepath.IsAbs(workspaceRoot) {
		return validatedNetworkOperation{}, errors.New("OPERATION_RUNTIME_ROOT_INVALID")
	}
	targets := make([]networkOperationTarget, 0, len(claim.Targets))
	seen := map[string]bool{}
	seenHost := map[string]bool{}
	for _, target := range claim.Targets {
		target.Host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(target.Host), "."))
		target.Protocol = strings.ToLower(strings.TrimSpace(target.Protocol))
		if !operationHostPattern.MatchString(target.Host) || net.ParseIP(target.Host) != nil || strings.Contains(target.Host, "..") || (target.Protocol != "http" && target.Protocol != "https") || target.Port < 1 || target.Port > 65535 {
			return validatedNetworkOperation{}, errors.New("OPERATION_TARGET_INVALID")
		}
		if seenHost[target.Host] {
			return validatedNetworkOperation{}, errors.New("OPERATION_TARGET_INVALID")
		}
		seenHost[target.Host] = true
		key := target.Protocol + "://" + target.Host + ":" + strconv.Itoa(target.Port)
		if !seen[key] {
			seen[key] = true
			targets = append(targets, target)
		}
	}
	return validatedNetworkOperation{containerCWD: filepath.ToSlash(filepath.Join("/workspace", cwd)), targets: targets}, nil
}

func (r *networkOperationRuntime) resolveTarget(ctx context.Context, host string) (net.IP, error) {
	addresses, err := r.resolve(ctx, host)
	if err != nil {
		return nil, errors.New("OPERATION_TARGET_UNAVAILABLE")
	}
	var allowed []net.IP
	for _, address := range addresses {
		if publicOperationIP(address) {
			allowed = append(allowed, address)
		}
	}
	if len(allowed) == 0 {
		return nil, errors.New("OPERATION_TARGET_DENIED")
	}
	sort.Slice(allowed, func(i, j int) bool { return bytes.Compare(allowed[i].To16(), allowed[j].To16()) < 0 })
	return allowed[0], nil
}

func publicOperationIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		blocked := netip.MustParsePrefix(prefix)
		if blocked.Contains(address) {
			return false
		}
	}
	return true
}

func operationLabels(claim networkOperationClaim) map[string]string {
	return map[string]string{
		"seven.managed": "true", "seven.execution.operation": "true",
		"seven.operation.id": strconv.FormatInt(claim.OperationID, 10),
		"seven.task.id":      strconv.FormatInt(claim.TaskID, 10),
		"seven.attempt.id":   strconv.FormatInt(claim.AttemptID, 10),
		"seven.owner.epoch":  strconv.FormatInt(claim.OwnerEpoch, 10),
	}
}

func (r *networkOperationRuntime) createNetwork(ctx context.Context, name string, internal bool, labels map[string]string) (string, error) {
	args := []string{"network", "create", "--driver", "bridge"}
	if internal {
		args = append(args, "--internal")
	}
	for key, value := range labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args, name)
	output, err := r.journal.Execute(ctx, r.run, "docker", args...)
	if err != nil || len(output) > 4<<10 {
		return "", errors.New("OPERATION_NETWORK_UNAVAILABLE")
	}
	return strings.TrimSpace(output), nil
}

func (r *networkOperationRuntime) createGateway(ctx context.Context, name, internalNetwork, outboundNetwork, image string, target networkOperationTarget, ip net.IP, deadline time.Time, labels map[string]string) (string, string, error) {
	args := []string{"container", "create", "--name", name, "--network", outboundNetwork}
	for key, value := range labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args, "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "64", "--memory", "128m", "--cpus", "0.25", "--user", "10001:10001", "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m", "--entrypoint", "/usr/local/bin/seven-worker-cli", image,
		"target-egress-gateway", "--listen-port", strconv.Itoa(target.Port), "--target-ip", ip.String(), "--target-port", strconv.Itoa(target.Port), "--deadline", deadline.UTC().Format(time.RFC3339))
	output, err := r.journal.Execute(ctx, r.run, "docker", args...)
	if err != nil || strings.TrimSpace(output) == "" || len(output) > 4<<10 {
		return "", "", errors.New("OPERATION_GATEWAY_UNAVAILABLE")
	}
	containerID := strings.TrimSpace(output)
	if _, err = r.journal.Execute(ctx, r.run, "docker", "start", name); err != nil {
		return "", "", errors.New("OPERATION_GATEWAY_UNAVAILABLE")
	}
	if _, err = r.journal.Execute(ctx, r.run, "docker", "network", "connect", "--alias", target.Host, internalNetwork, name); err != nil {
		return "", "", errors.New("OPERATION_GATEWAY_UNAVAILABLE")
	}
	state, err := r.journal.Execute(ctx, r.run, "docker", "inspect", "--format", "{{.State.Running}}", name)
	if err != nil || strings.TrimSpace(state) != "true" {
		return "", "", errors.New("OPERATION_GATEWAY_UNAVAILABLE")
	}
	internalIP, err := r.journal.Execute(ctx, r.run, "docker", "inspect", "--format", "{{with index .NetworkSettings.Networks \""+internalNetwork+"\"}}{{.IPAddress}}{{end}}", name)
	internalIP = strings.TrimSpace(internalIP)
	if err != nil || net.ParseIP(internalIP) == nil {
		return "", "", errors.New("OPERATION_GATEWAY_UNAVAILABLE")
	}
	return containerID, internalIP, nil
}

func (r *networkOperationRuntime) createRunner(ctx context.Context, name, network string, claim networkOperationClaim, cwd string, hostMappings map[string]string, labels map[string]string) (string, error) {
	args := []string{"container", "create", "--name", name, "--network", network}
	for key, value := range labels {
		args = append(args, "--label", key+"="+value)
	}
	args = append(args,
		"--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "256", "--memory", "2g", "--cpus", "2.0", "--user", "10001:10001",
		"--tmpfs", "/tmp:rw,nosuid,size=256m",
		"--tmpfs", "/home/seven:rw,exec,nosuid,size=512m,uid=10001,gid=10001,mode=0700",
		"--workdir", cwd,
		"--mount", "type=bind,src="+claim.RepositoryRoot+",dst=/workspace",
		"--dns", "127.0.0.1")
	hosts := make([]string, 0, len(hostMappings))
	for host := range hostMappings {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	for _, host := range hosts {
		args = append(args, "--add-host", host+":"+hostMappings[host])
	}
	args = append(args, "--entrypoint", "/bin/sh", claim.HarnessImage, "-lc", claim.Command)
	output, err := r.journal.Execute(ctx, r.run, "docker", args...)
	if err != nil || strings.TrimSpace(output) == "" || len(output) > 4<<10 {
		return "", errors.New("OPERATION_RUNNER_UNAVAILABLE")
	}
	return strings.TrimSpace(output), nil
}

func (r *networkOperationRuntime) verifyInternalNetworkMembers(ctx context.Context, network, runner string, gatewayCount int) error {
	output, err := r.journal.Execute(ctx, r.run, "docker", "network", "inspect", "--format", "{{range .Containers}}{{.Name}}\n{{end}}", network)
	if err != nil {
		return fmt.Errorf("OPERATION_NETWORK_UNAVAILABLE: inspect members")
	}
	lines := strings.Fields(output)
	if len(lines) != gatewayCount && len(lines) != gatewayCount+1 {
		return fmt.Errorf("OPERATION_NETWORK_MEMBERSHIP_INVALID: member count %d", len(lines))
	}
	seenRunner := false
	for _, name := range lines {
		if name == runner {
			seenRunner = true
			continue
		}
		if !strings.HasPrefix(name, "seven-op-") || !strings.Contains(name, "-gw-") {
			return fmt.Errorf("OPERATION_NETWORK_MEMBERSHIP_INVALID: unexpected member")
		}
	}
	runnerNetwork, inspectErr := r.journal.Execute(ctx, r.run, "docker", "inspect", "--format", "{{.HostConfig.NetworkMode}}", runner)
	if inspectErr != nil || strings.TrimSpace(runnerNetwork) != network {
		return fmt.Errorf("OPERATION_NETWORK_MEMBERSHIP_INVALID: runner network")
	}
	_ = seenRunner // stopped containers may not appear in Network.Containers until start.
	return nil
}

func readPreparedNetworkOperation(root string, claim networkOperationClaim) (networkOperationClaim, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, "prepared.json"))
	if os.IsNotExist(err) {
		return networkOperationClaim{}, false, nil
	}
	if err != nil || len(body) > 4096 {
		return networkOperationClaim{}, false, errors.New("OPERATION_READBACK_INVALID")
	}
	var record struct {
		OperationID                                         int64 `json:"operationId,string"`
		DescriptorDigest, RuntimeContainerID, RelayIdentity string
	}
	if json.Unmarshal(body, &record) != nil || record.OperationID != claim.OperationID || record.DescriptorDigest != claim.DescriptorDigest || record.RuntimeContainerID == "" || len(record.RelayIdentity) != 64 {
		return networkOperationClaim{}, false, errors.New("OPERATION_READBACK_INVALID")
	}
	claim.RuntimeContainerID, claim.RelayIdentity = record.RuntimeContainerID, record.RelayIdentity
	return claim, true, nil
}

func safeOperationOutput(output string) (string, string) {
	digest := sha256.Sum256([]byte(output))
	output = strings.ToValidUTF8(output, "�")
	output = strings.Map(func(value rune) rune {
		if value == '\n' || value == '\t' || value >= 0x20 {
			return value
		}
		return -1
	}, output)
	secret := regexp.MustCompile(`(?i)(authorization|password|secret|token|api[_-]?key)(\s*[:=]\s*)\S+`)
	output = secret.ReplaceAllString(output, "$1$2[REDACTED]")
	if len(output) > operationAgentOutputLimit {
		output = output[:operationAgentOutputLimit]
		output = strings.ToValidUTF8(output, "")
	}
	return output, hex.EncodeToString(digest[:])
}

func dockerExitCode(err error) int {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return -1
}

func persistNetworkOperationResult(root string, claim networkOperationClaim, result networkOperationResult) (networkOperationResult, error) {
	body, err := networkOperationResultDigestInput(claim.DescriptorDigest, result)
	if err != nil {
		return networkOperationResult{}, err
	}
	digest := sha256.Sum256(body)
	result.ResultDigest = hex.EncodeToString(digest[:])
	if err = writeOperationJSON(filepath.Join(root, "result.json"), struct {
		DescriptorDigest string                 `json:"descriptorDigest"`
		Result           networkOperationResult `json:"result"`
	}{claim.DescriptorDigest, result}); err != nil {
		return networkOperationResult{}, err
	}
	return result, nil
}

func networkOperationResultDigestInput(descriptorDigest string, result networkOperationResult) ([]byte, error) {
	result.ResultDigest = ""
	return json.Marshal(struct {
		DescriptorDigest string                 `json:"descriptorDigest"`
		Result           networkOperationResult `json:"result"`
	}{descriptorDigest, result})
}

func readNetworkOperationResult(root string, claim networkOperationClaim) (networkOperationResult, bool, error) {
	body, err := os.ReadFile(filepath.Join(root, "result.json"))
	if os.IsNotExist(err) {
		return networkOperationResult{}, false, nil
	}
	if err != nil || len(body) > operationAgentOutputLimit+4096 {
		return networkOperationResult{}, false, errors.New("OPERATION_READBACK_INVALID")
	}
	var record struct {
		DescriptorDigest string                 `json:"descriptorDigest"`
		Result           networkOperationResult `json:"result"`
	}
	if json.Unmarshal(body, &record) != nil || record.DescriptorDigest != claim.DescriptorDigest || record.Result.OperationID != claim.OperationID || len(record.Result.ResultDigest) != 64 {
		return networkOperationResult{}, false, errors.New("OPERATION_READBACK_INVALID")
	}
	digestInput, err := networkOperationResultDigestInput(record.DescriptorDigest, record.Result)
	if err != nil {
		return networkOperationResult{}, false, errors.New("OPERATION_READBACK_INVALID")
	}
	digest := sha256.Sum256(digestInput)
	if hex.EncodeToString(digest[:]) != record.Result.ResultDigest {
		return networkOperationResult{}, false, errors.New("OPERATION_READBACK_INVALID")
	}
	return record.Result, true, nil
}

func (r *networkOperationRuntime) persistOutcomeUnknown(root string, claim networkOperationClaim, reason string) (networkOperationResult, error) {
	result := networkOperationResult{OperationID: claim.OperationID, Status: "OUTCOME_UNKNOWN", ReasonCode: reason, FinishedAt: r.now().UTC().Format(time.RFC3339Nano)}
	return persistNetworkOperationResult(root, claim, result)
}

func writeOperationJSON(path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".operation-")
	if err != nil {
		return errors.New("OPERATION_READBACK_UNAVAILABLE")
	}
	defer os.Remove(temporary.Name())
	if err = temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(body)
	}
	if err == nil {
		err = temporary.Sync()
	}
	err = errors.Join(err, temporary.Close())
	if err == nil {
		err = os.Rename(temporary.Name(), path)
	}
	if err != nil {
		return errors.New("OPERATION_READBACK_UNAVAILABLE")
	}
	return nil
}

func (r *networkOperationRuntime) Cleanup(operationID int64) error {
	if operationID <= 0 {
		return errors.New("OPERATION_IDENTITY_INVALID")
	}
	return r.cleanupMatching(operationID)
}

func (r *networkOperationRuntime) cleanupMatching(operationID int64) error {
	return r.journal.CleanupMatching(map[string]string{"seven.operation.id": strconv.FormatInt(operationID, 10)}, func(ctx context.Context, resource resourcejournal.Resource) error {
		return resourcejournal.Remove(ctx, r.run, resource)
	})
}
