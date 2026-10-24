package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	providerproxy "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/proxy"
)

const (
	configuredRelayMaxRequestBytes = 128 << 20
	configuredRelayUID             = "10001:10001"
	configuredRelaySocketPath      = "/provider/review-gateway.sock"
	configuredRelaySecretRoot      = "/run/seven/secrets"
	configuredRelaySpecPath        = "/run/seven/provider.json"
	configuredRelayLeasePath       = configuredRelaySecretRoot + "/review.lease"
	configuredRelayTokenPath       = configuredRelaySecretRoot + "/worker.token"
	configuredRelayControlSocket   = "/tmp/seven-review-provider-control.sock"
)

// startConfiguredProviderRelay runs the exact, admitted Provider Proxy image
// beside a Reviewer. The Reviewer sees only the socket volume; the proxy sees
// a separate private secret volume containing the Worker token and the
// ReviewRun lease. Neither secret is present in the proxy specification or in
// Docker labels.
func (d *Docker) startConfiguredProviderRelay(ctx context.Context, work Work) (*providerRelayContainer, error) {
	if d == nil || !validConfiguredRelayWork(work) {
		return nil, ErrResultIdentityInvalid
	}
	if !validExactDockerDigest(d.providerProxy.ImageDigest) || !strings.Contains(d.providerProxy.Image, "@sha256:") || strings.TrimSpace(d.providerProxy.Version) == "" {
		return nil, errors.New("REVIEW_PROVIDER_PROXY_IDENTITY_INVALID")
	}
	if !validExactDockerDigest(work.ProviderProxyImageDigest) || work.ProviderProxyImageDigest != d.providerProxy.ImageDigest {
		return nil, errors.New("REVIEW_PROVIDER_PROXY_IDENTITY_INVALID")
	}
	if !privateRegularFile(d.cfg.TokenFile) || !validAuthorityToken(work.LeaseToken) {
		return nil, errors.New("REVIEW_PROVIDER_AUTHORITY_INVALID")
	}
	resolved, err := d.exec(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", d.providerProxy.Image)
	if err != nil || strings.TrimSpace(string(resolved)) != d.providerProxy.ImageDigest {
		return nil, errors.New("REVIEW_PROVIDER_PROXY_DIGEST_MISMATCH")
	}

	deadline := work.ExecutionDeadline
	if deadline.IsZero() {
		hardTimeout := work.HardTimeout
		if hardTimeout <= 0 {
			hardTimeout = 60 * time.Minute
		}
		deadline = time.Now().UTC().Add(hardTimeout)
	}
	if !deadline.After(time.Now().UTC()) {
		return nil, errors.New("REVIEW_PROVIDER_DEADLINE_EXPIRED")
	}
	leaseExpiresAt := work.LeaseExpiresAt.UTC()
	if leaseExpiresAt.IsZero() || !leaseExpiresAt.After(time.Now().UTC()) {
		return nil, errors.New("REVIEW_PROVIDER_LEASE_EXPIRED")
	}
	gatewayBaseURL, err := reviewGatewayURLForContainer(d.cfg.ServerBaseURL)
	if err != nil {
		return nil, err
	}

	relayName := providerRelayName(work)
	socketVolume := relayName + "-volume"
	secretVolume := relayName + "-secret"
	relayRoot := filepath.Join(work.ReviewRoot, "provider-relay")
	if err = ensurePrivateDirectory(relayRoot); err != nil {
		return nil, err
	}
	var cleanupDirectory func()
	if d.cleanupJournal != nil {
		record, err := d.cleanupJournal.TrackDirectory(relayRoot, map[string]string{"seven.review.run.id": strconv.FormatInt(work.ReviewRunID, 10), "seven.review.owner.epoch": strconv.FormatInt(work.OwnerEpoch, 10)})
		if err != nil {
			return nil, err
		}
		cleanupDirectory = func() { _ = d.cleanupJournal.Cleanup(record.Key, nil) }
	}
	leaseFile := filepath.Join(relayRoot, "review.lease")
	specFile := filepath.Join(relayRoot, "provider.json")
	if err = writeRelayFile(leaseFile, []byte(work.LeaseToken), 0o600); err != nil {
		if cleanupDirectory != nil {
			cleanupDirectory()
		}
		return nil, errors.New("REVIEW_PROVIDER_LEASE_FILE_UNAVAILABLE")
	}
	cleanupFiles := func() {
		if cleanupDirectory != nil {
			cleanupDirectory()
			return
		}
		_ = os.Remove(leaseFile)
		_ = os.Remove(specFile)
	}

	requestTimeout := time.Until(deadline).Milliseconds()
	if requestTimeout < 1 {
		requestTimeout = 1
	}
	proxySpec := providerproxy.Config{
		ExecutionKind:                    "REVIEW_RUN",
		GatewayBaseURL:                   gatewayBaseURL,
		WorkerTokenFile:                  configuredRelayTokenPath,
		WorkerID:                         work.WorkerID,
		WorkerInstanceID:                 work.WorkerInstanceID,
		HostBootID:                       work.HostBootID,
		TaskID:                           work.TaskID,
		OwnerEpoch:                       work.OwnerEpoch,
		ReviewRunID:                      work.ReviewRunID,
		ReviewRunAttemptID:               work.ReviewRunAttemptID,
		ReviewLeaseFile:                  configuredRelayLeasePath,
		ModelCredentialID:                work.Provider.CredentialID,
		CredentialVersion:                work.Provider.CredentialVersion,
		ModelProtocol:                    work.Provider.Protocol,
		ProviderStructuredOutputContract: work.Provider.StructuredContract,
		PrimaryModel:                     work.Provider.PrimaryModel,
		ExecutionDeadline:                deadline.UTC().Format(time.RFC3339Nano),
		LeaseExpiresAt:                   leaseExpiresAt.Format(time.RFC3339Nano),
		ControlSocket:                    configuredRelayControlSocket,
		MaxRequestBytes:                  configuredRelayMaxRequestBytes,
		RequestTimeoutMS:                 requestTimeout,
	}
	specBody, err := json.Marshal(proxySpec)
	if err != nil {
		cleanupFiles()
		return nil, errors.New("REVIEW_PROVIDER_SPEC_UNAVAILABLE")
	}
	if err = writeRelayFile(specFile, specBody, 0o444); err != nil {
		cleanupFiles()
		return nil, errors.New("REVIEW_PROVIDER_SPEC_UNAVAILABLE")
	}

	relay := &providerRelayContainer{
		name:         relayName,
		volume:       socketVolume,
		secretVolume: secretVolume,
		specFile:     specFile,
		leaseFile:    leaseFile,
		imageDigest:  d.providerProxy.ImageDigest,
		configured:   true,
	}
	ready := false
	defer func() {
		if !ready {
			d.stopProviderRelay(relay)
			cleanupFiles()
		}
	}()
	labelArgs := []string{"--label", "seven.review.relay=true", "--label", "seven.review.run.id=" + strconv.FormatInt(work.ReviewRunID, 10), "--label", "seven.review.owner.epoch=" + strconv.FormatInt(work.OwnerEpoch, 10)}
	if _, err = d.exec(ctx, "docker", append([]string{"volume", "create"}, append(labelArgs, socketVolume)...)...); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_SOCKET_VOLUME_UNAVAILABLE")
	}
	secretLabelArgs := []string{"--label", "seven.review.relay.secret=true", "--label", "seven.review.run.id=" + strconv.FormatInt(work.ReviewRunID, 10), "--label", "seven.review.owner.epoch=" + strconv.FormatInt(work.OwnerEpoch, 10)}
	if _, err = d.exec(ctx, "docker", append([]string{"volume", "create"}, append(secretLabelArgs, secretVolume)...)...); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_SECRET_VOLUME_UNAVAILABLE")
	}
	// Docker Desktop's local volume can retain a root-owned directory without
	// granting the short-lived initializer DAC access. Keep the capability
	// confined to this network-none, read-only, one-shot container so the
	// long-lived relay and Reviewer still run with no capabilities.
	socketInitArgs := []string{"run", "--rm", "--user", "0:0", "--network", "none", "--read-only", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "64m", "--cpus", "0.10", "--tmpfs", "/tmp:rw,noexec,nosuid,size=8m", "--mount", "type=volume,src=" + socketVolume + ",dst=/provider", "--entrypoint", "/bin/sh", work.ReviewerImage, "-ec", "umask 077; head -c 32 /dev/urandom | sha256sum | cut -d ' ' -f1 > /provider/review-authority; chown 10001:10001 /provider /provider/review-authority"}
	if _, err = d.exec(ctx, "docker", socketInitArgs...); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_SOCKET_INIT_FAILED")
	}
	// Tighten modes while the files are still owned by the root initializer,
	// then transfer ownership to the non-root Proxy. Docker Desktop's local
	// volume driver requires DAC_OVERRIDE for chown after a 0400 mode; this
	// capability exists only in the short-lived initializer. The long-lived
	// Proxy below still drops every capability.
	secretInitArgs := []string{"run", "--rm", "--user", "0:0", "--network", "none", "--read-only", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--cap-add", "DAC_OVERRIDE", "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "64m", "--cpus", "0.10", "--tmpfs", "/tmp:rw,noexec,nosuid,size=8m", "--mount", "type=bind,src=" + d.cfg.TokenFile + ",dst=/input/worker.token,readonly", "--mount", "type=bind,src=" + leaseFile + ",dst=/input/review.lease,readonly", "--mount", "type=volume,src=" + secretVolume + ",dst=/secrets", "--entrypoint", "/bin/sh", work.ReviewerImage, "-ec", "umask 077; cp /input/worker.token /secrets/worker.token; cp /input/review.lease /secrets/review.lease; chmod 0700 /secrets; chmod 0400 /secrets/worker.token /secrets/review.lease; chown 10001:10001 /secrets /secrets/worker.token /secrets/review.lease"}
	if _, err = d.exec(ctx, "docker", secretInitArgs...); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_SECRET_INIT_FAILED")
	}

	containerArgs := []string{"create", "--name", relayName, "--network", "bridge", "--add-host", "host.docker.internal:host-gateway", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "64", "--memory", "128m", "--cpus", "0.5", "--user", configuredRelayUID, "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m", "--label", "seven.review.relay=true", "--label", "seven.review.relay.kind=configured", "--label", "seven.review.run.id=" + strconv.FormatInt(work.ReviewRunID, 10), "--label", "seven.review.owner.epoch=" + strconv.FormatInt(work.OwnerEpoch, 10), "--label", "seven.review.image.digest=" + d.providerProxy.ImageDigest, "--label", "seven.review.provider-proxy.version=" + d.providerProxy.Version, "--mount", "type=volume,src=" + socketVolume + ",dst=/provider", "--mount", "type=volume,src=" + secretVolume + ",dst=/run/seven/secrets,readonly", "--mount", "type=bind,src=" + specFile + ",dst=" + configuredRelaySpecPath + ",readonly", d.providerProxy.Image, "--spec", configuredRelaySpecPath, "--listen", "unix://" + configuredRelaySocketPath}
	created, err := d.exec(ctx, "docker", containerArgs...)
	if err != nil {
		return nil, errors.New("REVIEW_PROVIDER_PROXY_CREATE_FAILED")
	}
	relay.id = strings.TrimSpace(string(created))
	if len(relay.id) < 12 {
		return nil, ErrResultIdentityInvalid
	}
	if _, err = d.exec(ctx, "docker", "start", relay.id); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_PROXY_START_FAILED")
	}
	if err = d.waitConfiguredProviderRelay(ctx, work, socketVolume); err != nil {
		return nil, err
	}
	ready = true
	return relay, nil
}

func (d *Docker) recoverConfiguredProviderRelay(ctx context.Context, work Work) (*providerRelayContainer, error) {
	if d == nil || !validConfiguredRelayWork(work) || !validExactDockerDigest(d.providerProxy.ImageDigest) || !validExactDockerDigest(work.ProviderProxyImageDigest) || work.ProviderProxyImageDigest != d.providerProxy.ImageDigest || strings.TrimSpace(d.providerProxy.Image) == "" || strings.TrimSpace(d.providerProxy.Version) == "" {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	name := providerRelayName(work)
	body, err := d.exec(ctx, "docker", "inspect", name)
	if err != nil {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	var inspected []struct {
		ID    string `json:"Id"`
		Image string `json:"Image"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal(body, &inspected) != nil || len(inspected) != 1 || !inspected[0].State.Running {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	labels := inspected[0].Config.Labels
	if labels["seven.review.relay"] != "true" || labels["seven.review.relay.kind"] != "configured" || labels["seven.review.run.id"] != strconv.FormatInt(work.ReviewRunID, 10) || labels["seven.review.owner.epoch"] != strconv.FormatInt(work.OwnerEpoch, 10) || labels["seven.review.image.digest"] != d.providerProxy.ImageDigest {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	if inspected[0].Image != "" && inspected[0].Image != d.providerProxy.ImageDigest {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	if !privateRegularFile(filepath.Join(work.ReviewRoot, "provider-relay", "review.lease")) || !safeReadOnlyRegularFile(filepath.Join(work.ReviewRoot, "provider-relay", "provider.json")) {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	socketVolume := name + "-volume"
	secretVolume := name + "-secret"
	volumeBody, volumeErr := d.exec(ctx, "docker", "volume", "inspect", socketVolume)
	secretBody, secretErr := d.exec(ctx, "docker", "volume", "inspect", secretVolume)
	if volumeErr != nil || secretErr != nil || !relayVolumeIdentityMatches(volumeBody, work, socketVolume) || !relaySecretVolumeIdentityMatches(secretBody, work, secretVolume) {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	// The relay process survived the Worker restart, but its in-memory
	// authority still contains the previous lease token. Rebind it through the
	// proxy's private control UDS before allowing the recovered Reviewer to make
	// a Provider request. The token is sent over docker exec stdin and is never
	// placed in command arguments, labels, the spec, or logs.
	if err = d.renewConfiguredProviderRelayAuthority(ctx, inspected[0].ID, work); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	if err = d.waitConfiguredProviderRelay(ctx, work, socketVolume); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	return &providerRelayContainer{id: inspected[0].ID, name: name, volume: socketVolume, secretVolume: secretVolume, specFile: filepath.Join(work.ReviewRoot, "provider-relay", "provider.json"), leaseFile: filepath.Join(work.ReviewRoot, "provider-relay", "review.lease"), imageDigest: d.providerProxy.ImageDigest, configured: true}, nil
}

func (d *Docker) waitConfiguredProviderRelay(ctx context.Context, work Work, socketVolume string) error {
	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		_, err := d.exec(readyCtx, "docker", "run", "--rm", "--user", configuredRelayUID, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "16", "--memory", "32m", "--cpus", "0.05", "--mount", "type=volume,src="+socketVolume+",dst=/provider,readonly", "--entrypoint", "/bin/sh", work.ReviewerImage, "-ec", "test -S "+configuredRelaySocketPath)
		if err == nil {
			return nil
		}
		select {
		case <-readyCtx.Done():
			return errors.New("REVIEW_PROVIDER_RELAY_UNAVAILABLE")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func validConfiguredRelayWork(work Work) bool {
	return work.ReviewRunID > 0 && work.ReviewRunAttemptID > 0 && work.WorkerID > 0 && strings.TrimSpace(work.WorkerInstanceID) != "" && strings.TrimSpace(work.HostBootID) != "" && work.TaskID > 0 && work.OwnerEpoch > 0 && filepath.IsAbs(work.ReviewRoot) && filepath.Clean(work.ReviewRoot) != string(filepath.Separator) && work.Provider.Enabled()
}

type configuredRelayControlResponse struct {
	OK               bool   `json:"ok"`
	OwnerEpoch       int64  `json:"ownerEpoch,string"`
	WorkerInstanceID string `json:"workerInstanceId"`
	LeaseExpiresAt   string `json:"leaseExpiresAt"`
	ControlState     string `json:"controlState"`
}

func (d *Docker) renewConfiguredProviderRelayAuthority(ctx context.Context, containerID string, work Work) error {
	if d == nil || d.execInput == nil || strings.TrimSpace(containerID) == "" || !validAuthorityToken(work.LeaseToken) || work.LeaseExpiresAt.IsZero() {
		return errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	requestBody, err := json.Marshal(map[string]any{
		"schemaVersion":  1,
		"command":        "renew-authority",
		"ownerEpoch":     strconv.FormatInt(work.OwnerEpoch, 10),
		"leaseToken":     work.LeaseToken,
		"leaseExpiresAt": work.LeaseExpiresAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	output, err := d.execInput(ctx, requestBody, "docker", "exec", "-i", containerID, "/usr/local/bin/seven-provider-proxy", "control", "--socket", configuredRelayControlSocket)
	if err != nil {
		return errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	var response configuredRelayControlResponse
	if json.Unmarshal(output, &response) != nil || !response.OK || response.OwnerEpoch != work.OwnerEpoch || response.WorkerInstanceID != work.WorkerInstanceID || response.ControlState != "READY" {
		return errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	leaseExpires, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(response.LeaseExpiresAt))
	if err != nil || !leaseExpires.After(time.Now().UTC()) {
		return errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	return nil
}

func validExactDockerDigest(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) == len("sha256:")+64 && strings.HasPrefix(value, "sha256:") && validHexDigest(strings.TrimPrefix(value, "sha256:"))
}

func validHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'f') && !(character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func validAuthorityToken(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= 32 && !strings.ContainsAny(value, "\r\n\x00")
}

func privateRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o077 == 0
}

func safeReadOnlyRegularFile(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o022 == 0
}

func ensurePrivateDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) == string(filepath.Separator) {
		return ErrResultIdentityInvalid
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return ErrResultIdentityInvalid
	}
	return nil
}

func writeRelayFile(path string, body []byte, mode os.FileMode) error {
	if err := ensurePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return ErrResultIdentityInvalid
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func reviewGatewayURLForContainer(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(value), "/"))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("review Gateway base URL is invalid")
	}
	hostname := parsed.Hostname()
	if hostname == "127.0.0.1" || hostname == "localhost" || hostname == "::1" {
		if port := parsed.Port(); port != "" {
			parsed.Host = net.JoinHostPort("host.docker.internal", port)
		} else {
			parsed.Host = "host.docker.internal"
		}
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func relayVolumeIdentityMatches(body []byte, work Work, volume string) bool {
	name, ok := relayVolumeIdentity(body, volume)
	return ok && name == providerRelayName(work)
}

func relaySecretVolumeIdentityMatches(body []byte, work Work, volume string) bool {
	name, ok := relaySecretVolumeIdentity(body, volume)
	return ok && name == providerRelayName(work)
}
