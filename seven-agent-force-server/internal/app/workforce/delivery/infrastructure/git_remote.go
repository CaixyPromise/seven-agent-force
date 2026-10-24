package infrastructure

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	deliveryapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/domain"
)

func probeRemoteTarget(ctx context.Context, target domain.Target, credential deliveryapp.DeliveryCredential) error {
	if target.Transport != domain.TransportLocalBare {
		if err := ensurePublicRemoteResolution(ctx, target.Host); err != nil {
			return err
		}
	}
	root, err := os.MkdirTemp("", "seven-delivery-probe-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	remote, env, err := prepareGitAuthentication(root, target, credential)
	if err != nil {
		return err
	}
	args := append(controlledGitArgs(), "ls-remote", remote, "HEAD")
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = env
	output, err := command.CombinedOutput()
	for index := range output {
		output[index] = 0
	}
	if err != nil {
		return fmt.Errorf("DELIVERY_TARGET_PROBE_FAILED")
	}
	return nil
}

func ensurePublicRemoteResolution(ctx context.Context, host string) error {
	if !safeRemoteHost(host) {
		return fmt.Errorf("DELIVERY_TARGET_HOST_FORBIDDEN")
	}
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return nil
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil || len(addresses) == 0 {
		return fmt.Errorf("DELIVERY_TARGET_DNS_UNAVAILABLE")
	}
	for _, address := range addresses {
		if address.IsLoopback() || address.IsPrivate() || address.IsUnspecified() || address.IsMulticast() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() {
			return fmt.Errorf("DELIVERY_TARGET_DNS_FORBIDDEN")
		}
	}
	return nil
}

func prepareGitAuthentication(root string, target domain.Target, credential deliveryapp.DeliveryCredential) (string, []string, error) {
	if !safeRemoteHost(target.Host) {
		return "", nil, fmt.Errorf("DELIVERY_TARGET_HOST_FORBIDDEN")
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C"}
	switch target.Transport {
	case domain.TransportHTTP, domain.TransportHTTPS:
		if credential.Username == "" || credential.Password == "" {
			return "", nil, fmt.Errorf("DELIVERY_CREDENTIAL_INVALID")
		}
		username := filepath.Join(root, "username")
		password := filepath.Join(root, "password")
		askpass := filepath.Join(root, "askpass")
		if err := os.WriteFile(username, []byte(credential.Username), 0o600); err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(password, []byte(credential.Password), 0o600); err != nil {
			return "", nil, err
		}
		script := "#!/bin/sh\ncase \"$1\" in *sername*) exec /bin/cat \"$SEVEN_GIT_USERNAME_FILE\";; *) exec /bin/cat \"$SEVEN_GIT_PASSWORD_FILE\";; esac\n"
		if err := os.WriteFile(askpass, []byte(script), 0o700); err != nil {
			return "", nil, err
		}
		env = append(env, "GIT_ASKPASS="+askpass, "SEVEN_GIT_USERNAME_FILE="+username, "SEVEN_GIT_PASSWORD_FILE="+password)
		return httpRemote(target), env, nil
	case domain.TransportSSH:
		if credential.PrivateKey == "" || credential.KnownHosts == "" || target.SSHKnownHostFingerprint == "" {
			return "", nil, fmt.Errorf("DELIVERY_CREDENTIAL_INVALID")
		}
		key := filepath.Join(root, "identity")
		known := filepath.Join(root, "known_hosts")
		if err := os.WriteFile(key, []byte(credential.PrivateKey), 0o600); err != nil {
			return "", nil, err
		}
		if err := os.WriteFile(known, []byte(credential.KnownHosts), 0o600); err != nil {
			return "", nil, err
		}
		if err := verifyKnownHostsFingerprint(known, target.SSHKnownHostFingerprint); err != nil {
			return "", nil, err
		}
		sshCommand := "ssh -F /dev/null -i " + key + " -o UserKnownHostsFile=" + known + " -o StrictHostKeyChecking=yes -o IdentitiesOnly=yes -o BatchMode=yes"
		env = append(env, "GIT_SSH_COMMAND="+sshCommand)
		return sshRemote(target), env, nil
	default:
		return "", nil, fmt.Errorf("DELIVERY_TARGET_TRANSPORT_UNSUPPORTED")
	}
}

func controlledGitArgs() []string {
	return []string{"-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "submodule.recurse=false"}
}

func httpRemote(target domain.Target) string {
	host := target.Host
	defaultPort := 443
	scheme := "https"
	if target.Transport == domain.TransportHTTP {
		defaultPort, scheme = 80, "http"
	}
	if target.Port > 0 && target.Port != defaultPort {
		host = net.JoinHostPort(host, strconv.Itoa(target.Port))
	}
	return (&url.URL{Scheme: scheme, Host: host, Path: "/" + strings.TrimPrefix(target.RepositoryPath, "/")}).String()
}

func httpsRemote(target domain.Target) string { return httpRemote(target) }

func sshRemote(target domain.Target) string {
	host := target.Host
	if target.Port > 0 && target.Port != 22 {
		host = net.JoinHostPort(host, strconv.Itoa(target.Port))
	}
	return (&url.URL{Scheme: "ssh", User: url.User("git"), Host: host, Path: "/" + strings.TrimPrefix(target.RepositoryPath, "/")}).String()
}

func safeRemoteHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".local") || strings.ContainsAny(host, "/\\@?#%") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsMulticast() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast()
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

func verifyKnownHostsFingerprint(path, expected string) error {
	output, err := exec.Command("ssh-keygen", "-lf", path).Output()
	if err != nil || !strings.Contains(string(output), strings.TrimSpace(expected)) {
		return fmt.Errorf("DELIVERY_SSH_HOST_FINGERPRINT_MISMATCH")
	}
	return nil
}
