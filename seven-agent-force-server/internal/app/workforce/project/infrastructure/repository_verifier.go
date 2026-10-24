package infrastructure

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	projectapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

type RepositoryVerifier struct {
	Client        *http.Client
	Guard         *outboundurl.OutboundURLGuard
	AllowLocalLab bool
}

func (v RepositoryVerifier) Verify(ctx context.Context, target domain.ParsedRepositoryURL, requestedRef string) (projectapp.RepositoryVerification, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	guard := v.Guard
	if guard == nil {
		guard = outboundurl.NewOutboundURLGuard(outboundurl.Options{})
	}
	policy := repositoryPolicy(target, v.AllowLocalLab)
	validated, err := guard.Validate(ctx, target.DisplayURL, policy)
	if err != nil {
		return projectapp.RepositoryVerification{}, domain.ErrRepositoryURLInvalid
	}
	defaultRef, err := verifyGitRemote(ctx, target, requestedRef, validated.DialIP)
	if err != nil {
		return projectapp.RepositoryVerification{}, err
	}
	provider, webURL, capabilities := target.ProviderType, target.DisplayURL, domain.RepositoryCapabilities{Read: true}
	if target.Credential.Token != "" {
		provider, webURL, capabilities = v.detectProvider(ctx, guard, target, policy)
		capabilities.Read = true
	}
	return projectapp.RepositoryVerification{ProviderType: provider, DefaultRefName: defaultRef, WebURL: webURL, Capabilities: capabilities}, nil
}

func repositoryPolicy(target domain.ParsedRepositoryURL, allowLocalLab bool) outboundurl.Policy {
	if allowLocalLab && target.Host == "127.0.0.1" && target.Port == 3444 {
		return outboundurl.Policy{Mode: outboundurl.ModeLocalLoopback, AllowedHostnames: []string{"127.0.0.1"}, AllowedCIDRs: []string{"127.0.0.1/32"}, AllowedPorts: []int{3444}, AllowHTTP: true}
	}
	return outboundurl.Policy{Mode: outboundurl.ModeRestrictedOnly, AllowHTTP: true}
}

func verifyGitRemote(ctx context.Context, target domain.ParsedRepositoryURL, requestedRef, dialIP string) (string, error) {
	root, err := os.MkdirTemp("", "seven-project-repository-")
	if err != nil {
		return "", domain.ErrRepositoryPreparationFailed
	}
	defer os.RemoveAll(root)
	if err = os.Chmod(root, 0o700); err != nil {
		return "", domain.ErrRepositoryPreparationFailed
	}
	usernameFile, tokenFile, askpass := filepath.Join(root, "username"), filepath.Join(root, "token"), filepath.Join(root, "askpass")
	if err = os.WriteFile(usernameFile, []byte(target.Credential.Username), 0o600); err != nil {
		return "", domain.ErrRepositoryPreparationFailed
	}
	if err = os.WriteFile(tokenFile, []byte(target.Credential.Token), 0o600); err != nil {
		return "", domain.ErrRepositoryPreparationFailed
	}
	script := "#!/bin/sh\ncase \"$1\" in *sername*) exec /bin/cat \"$SEVEN_GIT_USERNAME_FILE\";; *) exec /bin/cat \"$SEVEN_GIT_TOKEN_FILE\";; esac\n"
	if err = os.WriteFile(askpass, []byte(script), 0o700); err != nil {
		return "", domain.ErrRepositoryPreparationFailed
	}
	env := append([]string{}, os.Environ()...)
	env = append(env, "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS="+askpass, "SEVEN_GIT_USERNAME_FILE="+usernameFile, "SEVEN_GIT_TOKEN_FILE="+tokenFile,
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=http.curloptResolve", "GIT_CONFIG_VALUE_0="+curlResolve(target.Host, target.Port, dialIP))
	args := []string{"-c", "credential.helper=", "-c", "core.hooksPath=/dev/null", "-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "ls-remote", "--symref", target.DisplayURL, "HEAD"}
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = env
	output, runErr := command.Output()
	if runErr != nil {
		if exitErr := new(exec.ExitError); errors.As(runErr, &exitErr) {
			for i := range exitErr.Stderr {
				exitErr.Stderr[i] = 0
			}
		}
		return "", domain.ErrRepositoryAuthentication
	}
	defer func() {
		for i := range output {
			output[i] = 0
		}
	}()
	defaultRef := ""
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[0] == "ref:" && fields[2] == "HEAD" {
			defaultRef = strings.TrimPrefix(fields[1], "refs/heads/")
			break
		}
	}
	if requestedRef = strings.TrimSpace(requestedRef); requestedRef != "" {
		check := exec.CommandContext(ctx, "git", args[:len(args)-2]...)
		check.Args = append(check.Args, target.DisplayURL, "refs/heads/"+requestedRef)
		check.Env = env
		branchOutput, branchErr := check.Output()
		for i := range branchOutput {
			branchOutput[i] = 0
		}
		if branchErr != nil || len(branchOutput) == 0 {
			return "", domain.ErrRepositoryRefNotFound
		}
		return requestedRef, nil
	}
	if defaultRef == "" {
		return "", domain.ErrRepositoryRefNotFound
	}
	return defaultRef, nil
}

func curlResolve(host string, port int, address string) string {
	if strings.Contains(address, ":") {
		address = "[" + address + "]"
	}
	return fmt.Sprintf("%s:%d:%s", host, port, address)
}

func (v RepositoryVerifier) detectProvider(ctx context.Context, guard *outboundurl.OutboundURLGuard, target domain.ParsedRepositoryURL, policy outboundurl.Policy) (string, string, domain.RepositoryCapabilities) {
	client := v.Client
	if client == nil {
		client = repositoryProviderHTTPClient(guard, policy)
		client.Timeout = 10 * time.Second
	}
	path := strings.TrimSuffix(target.RepositoryPath, ".git")
	if target.Host == "github.com" {
		if v.Client == nil {
			client = repositoryProviderHTTPClient(guard, outboundurl.Policy{Mode: outboundurl.ModePublic, AllowedHostnames: []string{"api.github.com"}, AllowedPorts: []int{443}})
			client.Timeout = 10 * time.Second
		}
		var body struct {
			HTMLURL     string `json:"html_url"`
			Permissions struct {
				Push bool `json:"push"`
			} `json:"permissions"`
		}
		if providerGet(ctx, client, "https://api.github.com/repos/"+path, "Authorization", "Bearer "+target.Credential.Token, &body) == nil {
			return domain.RepositoryProviderGitHub, body.HTMLURL, domain.RepositoryCapabilities{Read: true, Push: body.Permissions.Push, DraftReview: body.Permissions.Push}
		}
		return domain.RepositoryProviderGitHub, target.DisplayURL, domain.RepositoryCapabilities{Read: true}
	}
	base := target.Scheme + "://" + target.Host
	if target.Port != 80 && target.Port != 443 {
		base += fmt.Sprintf(":%d", target.Port)
	}
	var gitlab struct {
		WebURL      string `json:"web_url"`
		Permissions struct {
			ProjectAccess *struct {
				AccessLevel int `json:"access_level"`
			} `json:"project_access"`
			GroupAccess *struct {
				AccessLevel int `json:"access_level"`
			} `json:"group_access"`
		} `json:"permissions"`
	}
	if providerGet(ctx, client, base+"/api/v4/projects/"+url.PathEscape(path), "PRIVATE-TOKEN", target.Credential.Token, &gitlab) == nil {
		level := 0
		if gitlab.Permissions.ProjectAccess != nil {
			level = gitlab.Permissions.ProjectAccess.AccessLevel
		}
		if gitlab.Permissions.GroupAccess != nil && gitlab.Permissions.GroupAccess.AccessLevel > level {
			level = gitlab.Permissions.GroupAccess.AccessLevel
		}
		// GitLab Project Access Tokens authenticate as project-bot users. The
		// project response does not consistently populate permissions for that
		// principal, so read the bot's effective project membership before
		// declaring push or Draft MR support.
		if level == 0 {
			var user struct {
				ID json.Number `json:"id"`
			}
			if providerGet(ctx, client, base+"/api/v4/user", "PRIVATE-TOKEN", target.Credential.Token, &user) == nil {
				if userID, parseErr := user.ID.Int64(); parseErr == nil && userID > 0 {
					var membership struct {
						AccessLevel int `json:"access_level"`
					}
					memberURL := base + "/api/v4/projects/" + url.PathEscape(path) + "/members/all/" + fmt.Sprint(userID)
					if providerGet(ctx, client, memberURL, "PRIVATE-TOKEN", target.Credential.Token, &membership) == nil {
						level = membership.AccessLevel
					}
				}
			}
		}
		return domain.RepositoryProviderGitLab, gitlab.WebURL, domain.RepositoryCapabilities{Read: true, Push: level >= 30, DraftReview: level >= 30}
	}
	var gitea struct {
		HTMLURL     string `json:"html_url"`
		Permissions struct {
			Push bool `json:"push"`
		} `json:"permissions"`
	}
	if providerGet(ctx, client, base+"/api/v1/repos/"+path, "Authorization", "token "+target.Credential.Token, &gitea) == nil {
		return domain.RepositoryProviderGitea, gitea.HTMLURL, domain.RepositoryCapabilities{Read: true, Push: gitea.Permissions.Push, DraftReview: gitea.Permissions.Push}
	}
	return domain.RepositoryProviderGeneric, target.DisplayURL, domain.RepositoryCapabilities{Read: true}
}

func repositoryProviderHTTPClient(guard *outboundurl.OutboundURLGuard, policy outboundurl.Policy) *http.Client {
	var roots *x509.CertPool
	for _, environmentName := range []string{"GIT_SSL_CAINFO", "SSL_CERT_FILE"} {
		path := strings.TrimSpace(os.Getenv(environmentName))
		if path == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() || info.Size() <= 0 || info.Size() > 1<<20 {
			continue
		}
		pem, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if roots == nil {
			roots, _ = x509.SystemCertPool()
			if roots == nil {
				roots = x509.NewCertPool()
			}
		}
		if !roots.AppendCertsFromPEM(pem) {
			for index := range pem {
				pem[index] = 0
			}
			continue
		}
		for index := range pem {
			pem[index] = 0
		}
	}
	return guard.HTTPClientWithOptions(policy, outboundurl.HTTPClientOptions{RootCAs: roots})
}

func providerGet(ctx context.Context, client *http.Client, endpoint, header, value string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set(header, value)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("provider unavailable")
	}
	return json.NewDecoder(response.Body).Decode(output)
}

var _ projectapp.RepositoryVerifier = RepositoryVerifier{}
