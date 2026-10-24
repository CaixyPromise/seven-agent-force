package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewcontract"
	reviewdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

// ProviderIdentity is the non-secret, frozen provider contract a Reviewer may
// request through its run-scoped relay. It deliberately contains no bearer
// token, upstream URL, credential, or mutable provider selection.
type ProviderIdentity struct {
	CredentialID       int64
	CredentialVersion  int64
	Protocol           string
	PrimaryModel       string
	StructuredContract string
}

func (p ProviderIdentity) Enabled() bool {
	return p.CredentialID > 0 && p.CredentialVersion > 0 && (p.Protocol == "OPENAI" || p.Protocol == "ANTHROPIC") && strings.TrimSpace(p.PrimaryModel) != "" && p.StructuredContract == reviewcontract.StructuredOutputContractV1
}

// providerRelayContainer is the Docker Desktop-compatible implementation of
// the review-scoped Unix relay. Docker Desktop cannot connect to a UDS made on
// the host through an osxfs bind mount, so the Worker creates a small,
// network-none relay sidecar and an anonymous per-run Docker volume. Neither
// container has an upstream URL, provider credential, Worker token, or Docker
// socket. The init process creates a short-lived random authority inside that
// one volume; the sidecar requires it alongside the frozen Provider identity.
// When a Provider Proxy image is configured, the same socket volume is backed
// by that exact digest and the proxy talks to the Server's ReviewRun machine
// endpoint. With no configured proxy, the deterministic local relay remains
// available for Slice 4/6 Fake fixtures.
type providerRelayContainer struct {
	id           string
	name         string
	volume       string
	secretVolume string
	specFile     string
	imageDigest  string
	leaseFile    string
	networkName  string
	configured   bool
}

func (d *Docker) startProviderRelay(ctx context.Context, work Work) (*providerRelayContainer, error) {
	if d == nil || !work.Provider.Enabled() {
		return nil, ErrResultIdentityInvalid
	}
	// The deterministic in-process relay is only a test fixture when the
	// Worker itself has no configured Provider Proxy. Once a Worker is
	// configured for the review-scoped Provider plane, a missing frozen digest
	// is a hard configuration failure; silently switching to Fake would make a
	// live ReviewRun unverifiable.
	if strings.TrimSpace(d.providerProxy.Image) == "" {
		return d.startFakeProviderRelay(ctx, work)
	}
	if !validExactDockerDigest(work.ProviderProxyImageDigest) {
		return nil, errors.New("REVIEW_PROVIDER_PROXY_IDENTITY_MISSING")
	}
	return d.startConfiguredProviderRelay(ctx, work)
}

func (d *Docker) startFakeProviderRelay(ctx context.Context, work Work) (*providerRelayContainer, error) {
	name := providerRelayName(work)
	volume := name + "-volume"
	if _, err := d.exec(ctx, "docker", "volume", "create", "--label", "seven.review.relay=true", "--label", "seven.review.run.id="+fmt.Sprintf("%d", work.ReviewRunID), "--label", "seven.review.owner.epoch="+fmt.Sprintf("%d", work.OwnerEpoch), volume); err != nil {
		return nil, fmt.Errorf("create review provider relay volume: %w", err)
	}
	// A Docker named volume starts root-owned. A one-shot, network-none init
	// process changes ownership of only this freshly-created per-Run volume;
	// the long-lived relay and Reviewer both remain non-root. No Task content,
	// credential, provider endpoint, or Worker authority enters the init step.
	initArgs := []string{"run", "--rm", "--user", "0:0", "--network", "none", "--read-only", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "64m", "--cpus", "0.10", "--tmpfs", "/tmp:rw,noexec,nosuid,size=8m",
		"--mount", "type=volume,src=" + volume + ",dst=/provider", "--entrypoint", "/bin/sh", work.ReviewerImage, "-ec", "umask 077; head -c 32 /dev/urandom | sha256sum | awk '{print $1}' > /provider/review-authority; chown 10001:10001 /provider /provider/review-authority"}
	if _, err := d.exec(ctx, "docker", initArgs...); err != nil {
		_, _ = d.exec(context.Background(), "docker", "volume", "rm", "-f", volume)
		return nil, fmt.Errorf("initialize review provider relay volume: %w", err)
	}
	args := []string{"create", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "64", "--memory", "128m", "--cpus", "0.25", "--user", "10001:10001",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=16m",
		"--label", "seven.review.relay=true", "--label", "seven.review.run.id=" + fmt.Sprintf("%d", work.ReviewRunID), "--label", "seven.review.owner.epoch=" + fmt.Sprintf("%d", work.OwnerEpoch), "--label", "seven.review.image.digest=" + work.ReviewerImage,
		"--mount", "type=volume,src=" + volume + ",dst=/provider",
		work.ReviewerImage, "review-provider-relay", "--socket", "/provider/review-gateway.sock", "--credential-id", fmt.Sprintf("%d", work.Provider.CredentialID), "--credential-version", fmt.Sprintf("%d", work.Provider.CredentialVersion), "--protocol", work.Provider.Protocol, "--model", work.Provider.PrimaryModel, "--contract", work.Provider.StructuredContract}
	created, err := d.exec(ctx, "docker", args...)
	if err != nil {
		_, _ = d.exec(context.Background(), "docker", "volume", "rm", "-f", volume)
		return nil, fmt.Errorf("create review provider relay: %w", err)
	}
	relay := &providerRelayContainer{id: strings.TrimSpace(string(created)), name: name, volume: volume, imageDigest: work.ReviewerImage}
	if len(relay.id) < 12 {
		d.stopProviderRelay(relay)
		return nil, ErrResultIdentityInvalid
	}
	if _, err = d.exec(ctx, "docker", "start", relay.id); err != nil {
		d.stopProviderRelay(relay)
		return nil, fmt.Errorf("start review provider relay: %w", err)
	}
	readyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		if _, err = d.exec(readyCtx, "docker", "exec", relay.id, "test", "-S", "/provider/review-gateway.sock"); err == nil {
			return relay, nil
		}
		if readyCtx.Err() != nil {
			d.stopProviderRelay(relay)
			return nil, errors.New("REVIEW_PROVIDER_RELAY_UNAVAILABLE")
		}
		select {
		case <-readyCtx.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// recoverProviderRelay accepts only the deterministic sidecar identity from
// the same Run/owner epoch. A missing or replaced sidecar fails the current
// execution transiently; it never recreates a relay for a stale container.
func (d *Docker) recoverProviderRelay(ctx context.Context, work Work) (*providerRelayContainer, error) {
	if d == nil {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	if strings.TrimSpace(d.providerProxy.Image) == "" {
		return d.recoverFakeProviderRelay(ctx, work)
	}
	if !validExactDockerDigest(work.ProviderProxyImageDigest) {
		return nil, errors.New("REVIEW_PROVIDER_PROXY_IDENTITY_MISSING")
	}
	return d.recoverConfiguredProviderRelay(ctx, work)
}

func (d *Docker) recoverFakeProviderRelay(ctx context.Context, work Work) (*providerRelayContainer, error) {
	name := providerRelayName(work)
	body, err := d.exec(ctx, "docker", "inspect", name)
	if err != nil {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	var inspected []struct {
		ID    string `json:"Id"`
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal(body, &inspected) != nil || len(inspected) != 1 || !inspected[0].State.Running || inspected[0].Config.Labels["seven.review.relay"] != "true" || inspected[0].Config.Labels["seven.review.run.id"] != fmt.Sprintf("%d", work.ReviewRunID) || inspected[0].Config.Labels["seven.review.owner.epoch"] != fmt.Sprintf("%d", work.OwnerEpoch) || inspected[0].Config.Labels["seven.review.image.digest"] != work.ReviewerImage {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	relay := &providerRelayContainer{id: inspected[0].ID, name: name, volume: name + "-volume", imageDigest: work.ReviewerImage}
	if _, err = d.exec(ctx, "docker", "exec", relay.id, "test", "-S", "/provider/review-gateway.sock"); err != nil {
		return nil, errors.New("REVIEW_PROVIDER_RELAY_RECOVERY_REQUIRED")
	}
	return relay, nil
}

func (d *Docker) stopProviderRelay(relay *providerRelayContainer) {
	if d == nil || relay == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), resourcejournal.Timeout)
	defer cancel()
	var cleanupErr error
	if strings.TrimSpace(relay.id) != "" {
		_, err := d.exec(ctx, "docker", "rm", "-f", relay.id)
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if strings.TrimSpace(relay.networkName) != "" {
		_, err := d.exec(ctx, "docker", "network", "rm", relay.networkName)
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if strings.TrimSpace(relay.volume) != "" {
		_, err := d.exec(ctx, "docker", "volume", "rm", relay.volume)
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if strings.TrimSpace(relay.secretVolume) != "" {
		_, err := d.exec(ctx, "docker", "volume", "rm", relay.secretVolume)
		cleanupErr = errors.Join(cleanupErr, err)
	}
	if d.cleanupJournal != nil && relay.specFile != "" {
		if record, ok := d.cleanupJournal.Find("directory", filepath.Dir(relay.specFile)); ok {
			_ = d.cleanupJournal.Cleanup(record.Key, nil)
		}
		return
	}
	if cleanupErr != nil {
		return
	}
	if strings.TrimSpace(relay.leaseFile) != "" {
		_ = os.Remove(relay.leaseFile)
	}
	if strings.TrimSpace(relay.specFile) != "" {
		_ = os.Remove(relay.specFile)
	}
}

// CleanupOrphanProviderRelays closes the crash window between creating the
// run-scoped relay/volume and persisting the Reviewer container. A relay is
// retained only while a validated managed Reviewer has the same Run/epoch;
// labels alone never earn indefinite retention of an authority volume.
func (d *Docker) CleanupOrphanProviderRelays(ctx context.Context, reviewers []ManagedContainer) error {
	if d == nil {
		return ErrResultIdentityInvalid
	}
	active := make(map[string]struct{}, len(reviewers))
	for _, reviewer := range reviewers {
		if reviewer.ReviewRunID > 0 && reviewer.OwnerEpoch > 0 {
			active[providerRelayName(Work{ReviewRunID: reviewer.ReviewRunID, OwnerEpoch: reviewer.OwnerEpoch})] = struct{}{}
		}
	}
	ids, err := d.exec(ctx, "docker", "ps", "-aq", "--filter", "label=seven.review.relay=true")
	if err != nil {
		return err
	}
	for _, id := range strings.Fields(string(ids)) {
		body, inspectErr := d.exec(ctx, "docker", "inspect", id)
		name, valid := relayIdentityFromInspect(body)
		if inspectErr != nil || !valid {
			continue // malformed labels fail closed for operator inspection.
		}
		if _, keep := active[name]; !keep {
			if _, removeErr := d.exec(ctx, "docker", "rm", "-f", id); removeErr != nil {
				return removeErr
			}
		}
	}
	volumes, err := d.exec(ctx, "docker", "volume", "ls", "-q", "--filter", "label=seven.review.relay=true")
	if err != nil {
		return err
	}
	for _, volume := range strings.Fields(string(volumes)) {
		body, inspectErr := d.exec(ctx, "docker", "volume", "inspect", volume)
		name, valid := relayVolumeIdentity(body, volume)
		if inspectErr != nil || !valid {
			continue
		}
		if _, keep := active[name]; !keep {
			if _, removeErr := d.exec(ctx, "docker", "volume", "rm", "-f", volume); removeErr != nil {
				return removeErr
			}
		}
	}
	secretVolumes, err := d.exec(ctx, "docker", "volume", "ls", "-q", "--filter", "label=seven.review.relay.secret=true")
	if err != nil {
		return err
	}
	for _, volume := range strings.Fields(string(secretVolumes)) {
		body, inspectErr := d.exec(ctx, "docker", "volume", "inspect", volume)
		name, valid := relaySecretVolumeIdentity(body, volume)
		if inspectErr != nil || !valid {
			continue
		}
		if _, keep := active[name]; !keep {
			if _, removeErr := d.exec(ctx, "docker", "volume", "rm", "-f", volume); removeErr != nil {
				return removeErr
			}
		}
	}
	return nil
}

func relayIdentityFromInspect(body []byte) (string, bool) {
	var inspected []struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if json.Unmarshal(body, &inspected) != nil || len(inspected) != 1 {
		return "", false
	}
	labels := inspected[0].Config.Labels
	if labels["seven.review.relay"] != "true" {
		return "", false
	}
	runID, runErr := strconv.ParseInt(labels["seven.review.run.id"], 10, 64)
	epoch, epochErr := strconv.ParseInt(labels["seven.review.owner.epoch"], 10, 64)
	if runErr != nil || epochErr != nil || runID < 1 || epoch < 1 {
		return "", false
	}
	return providerRelayName(Work{ReviewRunID: runID, OwnerEpoch: epoch}), true
}

func relayVolumeIdentity(body []byte, volume string) (string, bool) {
	var volumes []struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	}
	if json.Unmarshal(body, &volumes) != nil || len(volumes) != 1 || volumes[0].Name != volume || volumes[0].Labels["seven.review.relay"] != "true" {
		return "", false
	}
	runID, runErr := strconv.ParseInt(volumes[0].Labels["seven.review.run.id"], 10, 64)
	epoch, epochErr := strconv.ParseInt(volumes[0].Labels["seven.review.owner.epoch"], 10, 64)
	if runErr != nil || epochErr != nil || runID < 1 || epoch < 1 {
		return "", false
	}
	name := providerRelayName(Work{ReviewRunID: runID, OwnerEpoch: epoch})
	return name, volume == name+"-volume"
}

func relaySecretVolumeIdentity(body []byte, volume string) (string, bool) {
	var volumes []struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	}
	if json.Unmarshal(body, &volumes) != nil || len(volumes) != 1 || volumes[0].Name != volume || volumes[0].Labels["seven.review.relay.secret"] != "true" {
		return "", false
	}
	runID, runErr := strconv.ParseInt(volumes[0].Labels["seven.review.run.id"], 10, 64)
	epoch, epochErr := strconv.ParseInt(volumes[0].Labels["seven.review.owner.epoch"], 10, 64)
	if runErr != nil || epochErr != nil || runID < 1 || epoch < 1 {
		return "", false
	}
	name := providerRelayName(Work{ReviewRunID: runID, OwnerEpoch: epoch})
	return name, volume == name+"-secret"
}

func (d *Docker) providerUsage(ctx context.Context, relay *providerRelayContainer) reviewdomain.UsageFacts {
	if d == nil || relay == nil || relay.id == "" {
		return reviewdomain.UsageFacts{}
	}
	// A configured Provider Proxy records usage in the Server-side
	// ProviderInvocation aggregate. The proxy image is intentionally
	// distroless and has no shell or usage file to inspect from Docker.
	if relay.configured {
		return reviewdomain.UsageFacts{}
	}
	body, err := d.exec(ctx, "docker", "exec", relay.id, "cat", "/provider/usage.json")
	if err != nil || len(body) > 1024 {
		return reviewdomain.UsageFacts{}
	}
	var usage reviewdomain.UsageFacts
	if json.Unmarshal(body, &usage) != nil || usage.Requests < 0 || usage.Requests > 1 || usage.InputTokens != 0 || usage.OutputTokens != 0 || usage.DurationMilli != 0 {
		return reviewdomain.UsageFacts{}
	}
	return usage
}

func providerRelayName(work Work) string {
	return "seven-review-relay-" + fmt.Sprintf("%d", work.ReviewRunID) + "-" + fmt.Sprintf("%d", work.OwnerEpoch)
}
