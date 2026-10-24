package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
)

type repositoryResolverStub struct{ address string }

func (r repositoryResolverStub) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP(r.address)}}, nil
}

func TestRepositoryPolicyAllowsEnterprisePrivateButRejectsMetadata(t *testing.T) {
	target := domain.ParsedRepositoryURL{Host: "git.example.test", Port: 443, DisplayURL: "https://git.example.test/group/repo.git"}
	privateGuard := outboundurl.NewOutboundURLGuard(outboundurl.Options{Resolver: repositoryResolverStub{address: "10.24.0.20"}, InterfaceAddrs: func() ([]net.Addr, error) { return nil, nil }})
	validated, err := privateGuard.Validate(context.Background(), target.DisplayURL, repositoryPolicy(target, false))
	if err != nil || validated.DialIP != "10.24.0.20" {
		t.Fatalf("enterprise private Git target was not pinned: target=%#v err=%v", validated, err)
	}
	metadataGuard := outboundurl.NewOutboundURLGuard(outboundurl.Options{Resolver: repositoryResolverStub{address: "169.254.169.254"}, InterfaceAddrs: func() ([]net.Addr, error) { return nil, nil }})
	if _, err = metadataGuard.Validate(context.Background(), target.DisplayURL, repositoryPolicy(target, false)); err == nil {
		t.Fatal("metadata address was accepted as a Project repository")
	}
}

func TestCurlResolvePreservesHostnameAndPinsAddress(t *testing.T) {
	if got := curlResolve("git.example.test", 443, "203.0.113.7"); got != "git.example.test:443:203.0.113.7" {
		t.Fatalf("unexpected curl resolve directive: %q", got)
	}
}

func TestLocalGitLabLabPinsIPv4Loopback(t *testing.T) {
	target := domain.ParsedRepositoryURL{Host: "127.0.0.1", Port: 3444, DisplayURL: "https://127.0.0.1:3444/root/repo.git"}
	guard := outboundurl.NewOutboundURLGuard(outboundurl.Options{InterfaceAddrs: func() ([]net.Addr, error) { return nil, nil }})
	validated, err := guard.Validate(context.Background(), target.DisplayURL, repositoryPolicy(target, true))
	if err != nil || validated.DialIP != "127.0.0.1" {
		t.Fatalf("local GitLab fixture was not accepted and pinned: target=%#v err=%v", validated, err)
	}
}

func TestDetectGitLabProjectTokenUsesEffectiveMembership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PRIVATE-TOKEN") != "project-token" {
			t.Fatal("GitLab request omitted project token")
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v4/projects/group/repo":
			_ = json.NewEncoder(w).Encode(map[string]any{"web_url": "https://gitlab.example/group/repo", "permissions": map[string]any{}})
		case r.URL.Path == "/api/v4/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": 42})
		case strings.HasSuffix(r.URL.Path, "/members/all/42"):
			_ = json.NewEncoder(w).Encode(map[string]any{"access_level": 40})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	endpoint, _ := url.Parse(server.URL)
	port := endpoint.Port()
	target := domain.ParsedRepositoryURL{
		Scheme: "http", Host: endpoint.Hostname(), RepositoryPath: "group/repo.git",
		Credential: domain.RepositoryCredential{Username: "seven", Token: "project-token"},
	}
	_, _ = fmt.Sscan(port, &target.Port)
	verifier := RepositoryVerifier{Client: server.Client()}
	provider, _, capabilities := verifier.detectProvider(context.Background(), nil, target, outboundurl.Policy{})
	if provider != domain.RepositoryProviderGitLab || !capabilities.Read || !capabilities.Push || !capabilities.DraftReview {
		t.Fatalf("project token membership was not mapped to GitLab capabilities: provider=%s capabilities=%+v", provider, capabilities)
	}
}
