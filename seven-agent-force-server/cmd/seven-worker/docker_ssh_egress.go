package main

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// dockerSSHEgressBridgeImage is a pinned, single-purpose TCP relay used only
// by the trusted Worker when its host network cannot reach an approved public
// Git SSH endpoint. It receives neither Git credentials nor a repository
// path: OpenSSH continues to authenticate locally and invokes this process
// only as a byte stream to one DNS-pinned address.
//
// Local GitLab/Gitea addresses remain host-local. Docker Desktop containers
// cannot use 127.0.0.1 to reach a host-published service, so using the bridge
// for loopback would turn an otherwise precise local connection into a broken
// one. Public endpoints never fall back to a direct, unpinned connection.
const dockerSSHEgressBridgeImage = "alpine@sha256:48b0309ca019d89d40f670aa1bc06e426dc0931948452e8491e3d65087abc07d"

func sshEgressProxyCommand(root string, ip net.IP, port int) (string, error) {
	if strings.TrimSpace(root) == "" || ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || port < 1 || port > 65535 {
		return "", errors.New("SSH_EGRESS_PROXY_INVALID")
	}
	if ip.IsLoopback() {
		return "nc -w 10 " + ip.String() + " " + strconv.Itoa(port), nil
	}
	proxy := filepath.Join(root, "docker-ssh-egress")
	body := "#!/bin/sh\nexec docker run --rm -i --pull=never --network bridge --read-only --user 65534:65534 --cap-drop ALL --security-opt no-new-privileges --pids-limit 16 --memory 32m " +
		shellQuote(dockerSSHEgressBridgeImage) + " nc -w 15 " + shellQuote(ip.String()) + " " + shellQuote(strconv.Itoa(port)) + "\n"
	if err := os.WriteFile(proxy, []byte(body), 0o700); err != nil {
		return "", errors.New("SSH_EGRESS_PROXY_UNAVAILABLE")
	}
	return proxy, nil
}

// appendDockerBridgeEnvironment carries only the selected Docker daemon
// endpoint into the tightly-controlled Git/SSH process. Docker Desktop keeps
// that endpoint in its user context rather than /var/run/docker.sock; without
// this one non-secret value a subprocess would silently use the wrong daemon.
// The Worker reads its own context first, then exposes no Docker configuration,
// credentials, or socket path to an Attempt container.
func appendDockerBridgeEnvironment(ctx context.Context, environment []string) ([]string, error) {
	for _, value := range environment {
		if strings.HasPrefix(value, "DOCKER_HOST=") {
			return environment, nil
		}
	}
	if configured := strings.TrimSpace(os.Getenv("DOCKER_HOST")); configured != "" {
		if !validDockerBridgeHost(configured) {
			return nil, errors.New("SSH_EGRESS_DOCKER_CONTEXT_INVALID")
		}
		return append(environment, "DOCKER_HOST="+configured), nil
	}
	output, err := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{ .Endpoints.docker.Host }}").Output()
	if err != nil {
		return nil, errors.New("SSH_EGRESS_DOCKER_CONTEXT_UNAVAILABLE")
	}
	host := strings.TrimSpace(string(output))
	if !validDockerBridgeHost(host) {
		return nil, errors.New("SSH_EGRESS_DOCKER_CONTEXT_INVALID")
	}
	return append(environment, "DOCKER_HOST="+host), nil
}

func validDockerBridgeHost(host string) bool {
	return strings.HasPrefix(host, "unix:///") && len(host) <= 512 && !strings.ContainsAny(host, "\r\n\x00")
}
