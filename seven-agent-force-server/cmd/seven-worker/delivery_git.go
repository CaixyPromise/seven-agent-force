package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	deliverydomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
	processorcheckpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
)

func (w *worker) runDelivery(ctx context.Context, message deliverydomain.Message) error {
	claim, err := w.control.ClaimDelivery(ctx, message)
	if err != nil {
		return err
	}
	fail := func(reason string) error {
		if reportErr := w.control.FailDelivery(context.Background(), message, claim.LeaseToken, reason); reportErr != nil {
			return reportErr
		}
		return nil
	}
	root := filepath.Join(w.cfg.Worker.WorkspaceRoot, ".seven", "delivery", strconv.FormatInt(claim.AttemptID, 10))
	if err = os.RemoveAll(root); err != nil {
		return fail("DELIVERY_SPOOL_UNAVAILABLE")
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		return fail("DELIVERY_SPOOL_UNAVAILABLE")
	}
	defer os.RemoveAll(root)
	if claim.ReconciliationOnly {
		return w.runDeliveryReconciliation(ctx, message, claim, root)
	}
	key, err := base64.StdEncoding.DecodeString(claim.PlainCheckpointKeyBase64)
	if err != nil || len(key) != 32 {
		return fail("DELIVERY_CHECKPOINT_KEY_INVALID")
	}
	defer zeroBytes(key)
	if claim.ActionType == deliverydomain.ActionCustomDelivery {
		return w.runCustomDelivery(ctx, message, claim, key, root)
	}
	if claim.ActionType != deliverydomain.ActionGitPush {
		return fail("DELIVERY_ACTION_UNSUPPORTED")
	}
	bundle, err := w.downloadCheckpointPlain(ctx, claim.RepositoryBundleObjectKey, claim.RepositoryBundleObjectSHA256, claim.RepositoryBundleContentSHA256, claim.RepositoryBundleSizeBytes, key, claim.CheckpointIdentity, "repository-bundle", root, processorcheckpoint.MaxArchiveBytes)
	if err != nil {
		return fail("DELIVERY_CHECKPOINT_TAMPERED")
	}
	defer removeClosed(bundle)
	repository := filepath.Join(root, "repository")
	if err = os.MkdirAll(repository, 0o700); err != nil {
		return fail("DELIVERY_SPOOL_UNAVAILABLE")
	}
	if _, err = controlledWorkerGit(ctx, repository, "init", "-q"); err != nil {
		return fail("DELIVERY_BUNDLE_INVALID")
	}
	checkpointRef := "refs/seven/checkpoints/" + claim.CheckpointIdentity
	if _, err = controlledWorkerGit(ctx, repository, "-c", "protocol.file.allow=always", "fetch", "-q", bundle.Name(), checkpointRef+":"+checkpointRef); err != nil {
		return fail("DELIVERY_BUNDLE_INVALID")
	}
	if _, err = controlledWorkerGit(ctx, repository, "checkout", "--detach", "-q", claim.ResultCommit); err != nil {
		return fail("DELIVERY_RESULT_IDENTITY_MISMATCH")
	}
	tree, err := controlledWorkerGit(ctx, repository, "rev-parse", "HEAD^{tree}")
	if err != nil || tree != claim.ResultTree {
		return fail("DELIVERY_RESULT_IDENTITY_MISMATCH")
	}
	created, err := createDeliveryCommit(ctx, repository, claim)
	if err != nil || created != claim.DeliveryCommit {
		return fail("DELIVERY_COMMIT_IDENTITY_MISMATCH")
	}
	prepare, err := w.control.PrepareDelivery(ctx, message, claim.LeaseToken, created)
	if err != nil {
		return err
	}
	remote, remoteEnv, allowFile, cleanup, err := w.prepareDeliveryRemote(ctx, message, claim, root)
	if err != nil {
		return fail(deliveryReason(err))
	}
	defer cleanup()
	baseRef := "refs/heads/" + claim.BaseBranch
	destinationRef := "refs/heads/" + claim.DestinationBranch
	remoteBeforeBase, baseReadable := readDeliveryRemoteRef(ctx, remote, baseRef, remoteEnv, allowFile)
	remoteBefore, destinationReadable := readDeliveryRemoteRef(ctx, remote, destinationRef, remoteEnv, allowFile)
	if !baseReadable || !destinationReadable {
		return w.control.CompleteDelivery(ctx, message, claim.LeaseToken, prepare.PushAuthorization, deliveryapp.DeliveryResult{RemoteBefore: remoteBefore, Readable: false})
	}
	decision, preflightErr := deliverydomain.Preflight(claim.Target, remoteBeforeBase, remoteBefore, claim.ExpectedBaseCommit, claim.DeliveryCommit)
	if preflightErr != nil {
		return fail(preflightErr.Error())
	}
	if decision.Status == "SUCCEEDED" {
		return w.control.CompleteDelivery(ctx, message, claim.LeaseToken, prepare.PushAuthorization, deliveryapp.DeliveryResult{RemoteBefore: remoteBefore, RemoteAfter: remoteBefore, Readable: true})
	}
	localRef := "refs/seven/delivery/" + strconv.FormatInt(claim.AttemptID, 10)
	if _, err = controlledWorkerGit(ctx, repository, "update-ref", localRef, claim.DeliveryCommit); err != nil {
		return fail("DELIVERY_COMMIT_PREPARE_FAILED")
	}
	authorizationExpiry, expiryErr := time.Parse(time.RFC3339Nano, prepare.ExpiresAt)
	if expiryErr != nil || !authorizationExpiry.After(time.Now().UTC()) {
		return fail("DELIVERY_PUSH_AUTHORIZATION_EXPIRED")
	}
	pushStarted := true
	pushErr := runDeliveryPush(ctx, repository, remote, localRef, destinationRef, remoteEnv, allowFile)
	remoteAfter, readable := readDeliveryRemoteRef(ctx, remote, destinationRef, remoteEnv, allowFile)
	if pushErr != nil && !readable {
		return w.control.CompleteDelivery(ctx, message, claim.LeaseToken, prepare.PushAuthorization, deliveryapp.DeliveryResult{RemoteBefore: remoteBefore, Readable: false, PushStarted: pushStarted})
	}
	return w.control.CompleteDelivery(ctx, message, claim.LeaseToken, prepare.PushAuthorization, deliveryapp.DeliveryResult{RemoteBefore: remoteBefore, RemoteAfter: remoteAfter, Readable: readable, PushStarted: pushStarted})
}

func (w *worker) runDeliveryReconciliation(ctx context.Context, message deliverydomain.Message, claim deliveryapp.DeliveryClaim, root string) error {
	remote, remoteEnv, allowFile, cleanup, err := w.prepareDeliveryRemote(ctx, message, claim, root)
	if err != nil {
		return w.control.ReconcileDelivery(ctx, message, claim.LeaseToken, deliveryapp.DeliveryResult{Readable: false})
	}
	defer cleanup()
	remoteAfter, readable := readDeliveryRemoteRef(ctx, remote, "refs/heads/"+claim.DestinationBranch, remoteEnv, allowFile)
	return w.control.ReconcileDelivery(ctx, message, claim.LeaseToken, deliveryapp.DeliveryResult{RemoteAfter: remoteAfter, Readable: readable})
}

func (w *worker) prepareDeliveryRemote(ctx context.Context, message deliverydomain.Message, claim deliveryapp.DeliveryClaim, root string) (string, []string, bool, func(), error) {
	clean := func() {}
	if claim.Target.Transport == deliverydomain.TransportLocalBare {
		remote, err := validatedLocalBarePath(w.cfg.Worker.WorkspaceRoot, claim.Target.RepositoryPath)
		return remote, controlledDeliveryEnv(root), true, clean, err
	}
	addresses, err := resolveDeliveryEgress(ctx, claim.Target, func(ctx context.Context, host string) ([]net.IP, error) {
		return net.DefaultResolver.LookupIP(ctx, "ip", host)
	})
	if err != nil {
		return "", nil, false, clean, err
	}
	credential, err := w.control.MaterializeDeliveryCredential(ctx, message, claim.LeaseToken)
	if err != nil {
		return "", nil, false, clean, fmt.Errorf("DELIVERY_CREDENTIAL_MATERIALIZATION_FAILED")
	}
	credentialRoot := filepath.Join(root, "credentials")
	if err = os.MkdirAll(credentialRoot, 0o700); err != nil {
		return "", nil, false, clean, err
	}
	clean = func() { _ = os.RemoveAll(credentialRoot) }
	env := controlledDeliveryEnv(root)
	switch claim.Target.Transport {
	case deliverydomain.TransportHTTPS:
		if credential.Username == "" || credential.Password == "" {
			return "", nil, false, clean, fmt.Errorf("DELIVERY_CREDENTIAL_INVALID")
		}
		username, password := filepath.Join(credentialRoot, "username"), filepath.Join(credentialRoot, "password")
		askpass := filepath.Join(credentialRoot, "askpass")
		if err = os.WriteFile(username, []byte(credential.Username), 0o600); err != nil {
			return "", nil, false, clean, err
		}
		if err = os.WriteFile(password, []byte(credential.Password), 0o600); err != nil {
			return "", nil, false, clean, err
		}
		script := "#!/bin/sh\ncase \"$1\" in *sername*) exec /bin/cat \"$SEVEN_GIT_USERNAME_FILE\";; *) exec /bin/cat \"$SEVEN_GIT_PASSWORD_FILE\";; esac\n"
		if err = os.WriteFile(askpass, []byte(script), 0o700); err != nil {
			return "", nil, false, clean, err
		}
		env = append(env, "GIT_ASKPASS="+askpass, "SEVEN_GIT_USERNAME_FILE="+username, "SEVEN_GIT_PASSWORD_FILE="+password)
		return deliveryHTTPSRemote(claim.Target), env, false, clean, nil
	case deliverydomain.TransportSSH:
		if credential.PrivateKey == "" || credential.KnownHosts == "" || claim.Target.SSHKnownHostFingerprint == "" {
			return "", nil, false, clean, fmt.Errorf("DELIVERY_CREDENTIAL_INVALID")
		}
		key, known := filepath.Join(credentialRoot, "identity"), filepath.Join(credentialRoot, "known_hosts")
		if err = os.WriteFile(key, []byte(credential.PrivateKey), 0o600); err != nil {
			return "", nil, false, clean, err
		}
		if err = os.WriteFile(known, []byte(credential.KnownHosts), 0o600); err != nil {
			return "", nil, false, clean, err
		}
		fingerprint, fingerprintErr := exec.CommandContext(ctx, "ssh-keygen", "-lf", known).Output()
		if fingerprintErr != nil || !strings.Contains(string(fingerprint), strings.TrimSpace(claim.Target.SSHKnownHostFingerprint)) {
			return "", nil, false, clean, fmt.Errorf("DELIVERY_SSH_HOST_FINGERPRINT_MISMATCH")
		}
		command, commandErr := deliverySSHCommand(credentialRoot, claim.Target, key, known, addresses)
		if commandErr != nil {
			return "", nil, false, clean, commandErr
		}
		bridgeEnv, bridgeErr := appendDockerBridgeEnvironment(ctx, env)
		if bridgeErr != nil {
			return "", nil, false, clean, bridgeErr
		}
		env = append(bridgeEnv, "GIT_SSH_COMMAND="+command)
		return deliverySSHRemote(claim.Target), env, false, clean, nil
	default:
		return "", nil, false, clean, fmt.Errorf("DELIVERY_TARGET_TRANSPORT_UNSUPPORTED")
	}
}

// resolveDeliveryEgress locks a Delivery attempt to the precise DNS answers
// validated by the frozen Repository Connection policy. It deliberately does
// not infer "public Internet" safety: local GitLab/Gitea are valid only when
// their Connection explicitly allows the loopback CIDR and port.
func resolveDeliveryEgress(ctx context.Context, target deliverydomain.Target, lookup func(context.Context, string) ([]net.IP, error)) ([]net.IP, error) {
	if !validDeliveryHost(target.Host) || target.Port < 1 || target.Port > 65535 {
		return nil, errors.New("DELIVERY_TARGET_HOST_FORBIDDEN")
	}
	if lookup == nil {
		return nil, errors.New("DELIVERY_TARGET_DNS_UNAVAILABLE")
	}
	addresses, err := lookup(ctx, target.Host)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("DELIVERY_TARGET_DNS_UNAVAILABLE")
	}
	decoder := json.NewDecoder(bytes.NewReader(target.Egress))
	decoder.DisallowUnknownFields()
	var policy deliverydomain.EgressPolicy
	if len(target.Egress) == 0 || decoder.Decode(&policy) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return nil, errors.New("DELIVERY_EGRESS_POLICY_INVALID")
	}
	resolved := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address == nil {
			return nil, errors.New("DELIVERY_TARGET_DNS_FORBIDDEN")
		}
		resolved = append(resolved, address.String())
	}
	if policy.Authorize(target.Host, target.Port, resolved) != nil {
		return nil, errors.New("DELIVERY_TARGET_DNS_FORBIDDEN")
	}
	return orderDeliveryResolvedIPs(addresses), nil
}

// orderDeliveryResolvedIPs fixes the exact transport address before SSH builds
// its pinned ProxyCommand.  Local Docker acceptance services deliberately bind
// their SSH port on 127.0.0.1, while macOS resolves *.localhost as both ::1
// and 127.0.0.1.  Resolver order is not a security property, so select a
// deterministic, authorised IPv4 address before IPv6 when both are present.
// Every returned address was already validated against the frozen egress
// policy above.
func orderDeliveryResolvedIPs(addresses []net.IP) []net.IP {
	ordered := append([]net.IP(nil), addresses...)
	sort.SliceStable(ordered, func(left, right int) bool {
		leftIPv4 := ordered[left] != nil && ordered[left].To4() != nil
		rightIPv4 := ordered[right] != nil && ordered[right].To4() != nil
		if leftIPv4 != rightIPv4 {
			return leftIPv4
		}
		return ordered[left].String() < ordered[right].String()
	})
	return ordered
}

// deliverySSHCommand keeps OpenSSH's host-key validation on the original host
// while forcing its single TCP connection through an exact pinned address.
// Git receives only this small immutable command and a single remote/refspec.
func deliverySSHCommand(root string, target deliverydomain.Target, key, known string, addresses []net.IP) (string, error) {
	if len(addresses) == 0 || addresses[0] == nil || target.Port < 1 || target.Port > 65535 || !validDeliveryHost(target.Host) {
		return "", errors.New("DELIVERY_EGRESS_PIN_INVALID")
	}
	alias := target.Host
	if target.Port != 22 {
		alias = "[" + target.Host + "]:" + strconv.Itoa(target.Port)
	}
	proxy, proxyErr := sshEgressProxyCommand(root, addresses[0], target.Port)
	if proxyErr != nil {
		return "", errors.New("DELIVERY_EGRESS_PIN_UNAVAILABLE")
	}
	script := filepath.Join(root, "pinned-ssh")
	body := "#!/bin/sh\nexec ssh -F /dev/null -i " + shellQuote(key) +
		" -o " + shellQuote("UserKnownHostsFile="+known) +
		" -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes" +
		" -o " + shellQuote("HostKeyAlias="+alias) +
		" -o " + shellQuote("ProxyCommand="+proxy) + " \"$@\"\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		return "", errors.New("DELIVERY_EGRESS_PIN_UNAVAILABLE")
	}
	return script, nil
}

func controlledDeliveryEnv(root string) []string {
	return appendRuntimeGitCA([]string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "XDG_CONFIG_HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"})
}

func deliveryReason(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if strings.HasPrefix(value, "DELIVERY_") && len(value) <= 128 {
		return value
	}
	return "DELIVERY_REMOTE_UNREACHABLE"
}

func validDeliveryHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" || strings.ContainsAny(host, "/\\@?#%") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
	}
	for _, part := range strings.Split(host, ".") {
		if part == "" || len(part) > 63 || strings.HasPrefix(part, "-") || strings.HasSuffix(part, "-") {
			return false
		}
		for _, value := range part {
			if !(value == '-' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9') {
				return false
			}
		}
	}
	return true
}

func deliveryHTTPSRemote(target deliverydomain.Target) string {
	host := target.Host
	if target.Port > 0 && target.Port != 443 {
		host = net.JoinHostPort(host, strconv.Itoa(target.Port))
	}
	return (&url.URL{Scheme: "https", Host: host, Path: "/" + strings.TrimPrefix(target.RepositoryPath, "/")}).String()
}

func deliverySSHRemote(target deliverydomain.Target) string {
	host := target.Host
	if target.Port > 0 && target.Port != 22 {
		host = net.JoinHostPort(host, strconv.Itoa(target.Port))
	}
	return (&url.URL{Scheme: "ssh", User: url.User("git"), Host: host, Path: "/" + strings.TrimPrefix(target.RepositoryPath, "/")}).String()
}

func createDeliveryCommit(ctx context.Context, repository string, claim deliveryapp.DeliveryClaim) (string, error) {
	timestamp, err := time.Parse(time.RFC3339, claim.CommitTimestamp)
	if err != nil {
		return "", err
	}
	message := fmt.Sprintf("Seven Agent Force result\n\nTask: %d\nTurn: %d\nResult: %s\n", claim.TaskID, claim.TaskTurnID, claim.ResultCommit)
	command := exec.CommandContext(ctx, "git", "-C", repository, "commit-tree", claim.ResultTree, "-p", claim.ExpectedBaseCommit)
	command.Stdin = strings.NewReader(message)
	home, err := os.MkdirTemp(filepath.Dir(repository), ".delivery-home-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(home)
	date := fmt.Sprintf("%d +0000", timestamp.UTC().Unix())
	command.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(home, "config"), "HOME="+home, "XDG_CONFIG_HOME="+home,
		"GIT_AUTHOR_NAME="+claim.Target.CommitAuthorName, "GIT_AUTHOR_EMAIL="+claim.Target.CommitAuthorEmail, "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME="+claim.Target.CommitAuthorName, "GIT_COMMITTER_EMAIL="+claim.Target.CommitAuthorEmail, "GIT_COMMITTER_DATE="+date,
		"GIT_TERMINAL_PROMPT=0",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("DELIVERY_COMMIT_CREATE_FAILED: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func validatedLocalBarePath(workspaceRoot, target string) (string, error) {
	allowed, err := filepath.Abs(filepath.Join(workspaceRoot, ".seven", "local-git"))
	if err != nil {
		return "", err
	}
	target, err = filepath.Abs(target)
	tempRoot, _ := filepath.Abs(os.TempDir())
	tempRelative, tempErr := filepath.Rel(tempRoot, target)
	tempAllowed := tempErr == nil && tempRelative != "." && !strings.HasPrefix(tempRelative, ".."+string(os.PathSeparator)) && strings.HasPrefix(filepath.Base(strings.Split(filepath.ToSlash(tempRelative), "/")[0]), "seven-workforce-delivery-")
	if err != nil || ((target != allowed && !strings.HasPrefix(target, allowed+string(os.PathSeparator))) && !tempAllowed) {
		return "", errors.New("outside controlled local Git root")
	}
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return "", errors.New("local bare repository unavailable")
	}
	bare, err := exec.Command("git", "-C", target, "rev-parse", "--is-bare-repository").Output()
	if err != nil || strings.TrimSpace(string(bare)) != "true" {
		return "", errors.New("target is not bare")
	}
	return target, nil
}

func readDeliveryRemoteRef(ctx context.Context, remote, ref string, env []string, allowFile bool) (string, bool) {
	protocol := "never"
	if allowFile {
		protocol = "always"
	}
	command := exec.CommandContext(ctx, "git", "-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow="+protocol, "-c", "protocol.ext.allow=never", "-c", "submodule.recurse=false", "ls-remote", "--refs", remote, ref)
	command.Env = env
	output, err := command.Output()
	if err != nil {
		return "", false
	}
	fields := bytes.Fields(output)
	if len(fields) == 0 {
		return "", true
	}
	if len(fields) != 2 {
		return "", false
	}
	return string(fields[0]), true
}

func runDeliveryPush(ctx context.Context, repository, remote, localRef, destinationRef string, env []string, allowFile bool) error {
	protocol := "never"
	if allowFile {
		protocol = "always"
	}
	command := exec.CommandContext(ctx, "git", "-C", repository, "-c", "protocol.file.allow="+protocol, "-c", "protocol.ext.allow=never", "-c", "submodule.recurse=false", "-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "push", "--porcelain", "--", remote, localRef+":"+destinationRef)
	command.Env = env
	output, err := command.CombinedOutput()
	if len(output) > 64<<10 {
		return errors.New("DELIVERY_GIT_OUTPUT_LIMIT")
	}
	return err
}

func zeroBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}
