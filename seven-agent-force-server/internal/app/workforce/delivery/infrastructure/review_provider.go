package infrastructure

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/securehttp"
)

type ReviewHTTPAdapter struct {
	Client         *http.Client
	BaseURL        func(application.ReviewWork) (string, error)
	ValidateEgress func(context.Context, application.ReviewWork) error
}

func NewReviewHTTPAdapter() *ReviewHTTPAdapter {
	return &ReviewHTTPAdapter{Client: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("REVIEW_REDIRECT_FORBIDDEN") }}, BaseURL: providerAPIBaseURL, ValidateEgress: validateReviewEgress}
}

func (a *ReviewHTTPAdapter) PublishDraft(ctx context.Context, work application.ReviewWork, credential application.DeliveryCredential) (application.ReviewPublication, error) {
	if work.ProviderType == "LOCAL_TEST" {
		marker := work.MarkerDigest
		if len(marker) > 16 {
			marker = marker[:16]
		}
		return application.ReviewPublication{ProviderReviewID: "local-" + marker, ProviderNumber: "1", WebURL: fmt.Sprintf("local-review://%d/%s", work.ProjectID, work.HeadBranch), ObservedBase: work.FrozenBaseCommit}, nil
	}
	if a == nil || a.Client == nil || a.BaseURL == nil || credential.Password == "" {
		return application.ReviewPublication{}, errors.New("REVIEW_CREDENTIAL_INVALID")
	}
	if a.ValidateEgress == nil {
		return application.ReviewPublication{}, errors.New("REVIEW_EGRESS_POLICY_INVALID")
	}
	if err := a.ValidateEgress(ctx, work); err != nil {
		return application.ReviewPublication{}, err
	}
	base, err := a.BaseURL(work)
	if err != nil {
		return application.ReviewPublication{}, err
	}
	client, err := reviewHTTPClient(ctx, a, work)
	if err != nil {
		return application.ReviewPublication{}, err
	}
	observedBase, err := a.readBaseCommit(ctx, client, base, work, credential)
	if err != nil {
		return application.ReviewPublication{}, err
	}
	if observedBase != work.FrozenBaseCommit {
		return application.ReviewPublication{}, errors.New("REVIEW_BASE_CHANGED")
	}
	if found, ok, findErr := a.findExisting(ctx, client, base, work, credential); findErr == nil && ok {
		found.ObservedBase = observedBase
		return found, nil
	} else if findErr != nil {
		return application.ReviewPublication{}, findErr
	}
	path, payload := reviewCreateRequest(work)
	body, _ := json.Marshal(payload)
	response, err := a.do(ctx, client, http.MethodPost, base+path, body, work.ProviderType, credential)
	if err != nil {
		// The POST may have crossed the side-effect boundary. A following
		// reconciliation always searches by the immutable marker before create.
		return application.ReviewPublication{}, errors.New("REVIEW_RESPONSE_LOST")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return application.ReviewPublication{}, errors.New("REVIEW_RATE_LIMITED")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return application.ReviewPublication{}, reviewStatusError(response.StatusCode)
	}
	publication, err := decodeReviewPublication(response.Body, work)
	publication.ObservedBase = observedBase
	return publication, err
}

// ReconcileDraft is deliberately read-only. It re-reads the frozen base and
// searches for the immutable review marker, but never creates or mutates a
// review. This is the only safe operator action after an unknown outcome.
func (a *ReviewHTTPAdapter) ReconcileDraft(ctx context.Context, work application.ReviewWork, credential application.DeliveryCredential) (application.ReviewPublication, bool, error) {
	if work.ProviderType == "LOCAL_TEST" {
		return application.ReviewPublication{}, false, nil
	}
	if a == nil || a.Client == nil || a.BaseURL == nil || credential.Password == "" {
		return application.ReviewPublication{}, false, errors.New("REVIEW_CREDENTIAL_INVALID")
	}
	if a.ValidateEgress == nil {
		return application.ReviewPublication{}, false, errors.New("REVIEW_EGRESS_POLICY_INVALID")
	}
	if err := a.ValidateEgress(ctx, work); err != nil {
		return application.ReviewPublication{}, false, err
	}
	base, err := a.BaseURL(work)
	if err != nil {
		return application.ReviewPublication{}, false, err
	}
	client, err := reviewHTTPClient(ctx, a, work)
	if err != nil {
		return application.ReviewPublication{}, false, err
	}
	observedBase, err := a.readBaseCommit(ctx, client, base, work, credential)
	if err != nil {
		return application.ReviewPublication{}, false, err
	}
	publication, found, err := a.findExisting(ctx, client, base, work, credential)
	if err != nil || !found {
		return application.ReviewPublication{}, found, err
	}
	publication.ObservedBase = observedBase
	return publication, true, nil
}

func (a *ReviewHTTPAdapter) readBaseCommit(ctx context.Context, client *http.Client, base string, work application.ReviewWork, credential application.DeliveryCredential) (string, error) {
	branch := url.PathEscape(work.BaseBranch)
	path := "/branches/" + branch
	if work.ProviderType == "GITLAB" {
		path = "/repository/branches/" + branch
	}
	response, err := a.do(ctx, client, http.MethodGet, base+path, nil, work.ProviderType, credential)
	if err != nil {
		return "", errors.New("REVIEW_REMOTE_UNAVAILABLE")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return "", errors.New("REVIEW_RATE_LIMITED")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", reviewStatusError(response.StatusCode)
	}
	var item map[string]any
	if err = json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&item); err != nil {
		return "", errors.New("REVIEW_RESPONSE_INVALID")
	}
	commit, _ := item["commit"].(map[string]any)
	key := "sha"
	if work.ProviderType == "GITLAB" || work.ProviderType == "GITEA" {
		key = "id"
	}
	value := stringValue(commit, key)
	if value == "" {
		return "", errors.New("REVIEW_RESPONSE_INVALID")
	}
	return value, nil
}

func (a *ReviewHTTPAdapter) findExisting(ctx context.Context, client *http.Client, base string, work application.ReviewWork, credential application.DeliveryCredential) (application.ReviewPublication, bool, error) {
	path := reviewListPath(work)
	response, err := a.do(ctx, client, http.MethodGet, base+path, nil, work.ProviderType, credential)
	if err != nil {
		return application.ReviewPublication{}, false, errors.New("REVIEW_REMOTE_UNAVAILABLE")
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusTooManyRequests {
		return application.ReviewPublication{}, false, errors.New("REVIEW_RATE_LIMITED")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return application.ReviewPublication{}, false, reviewStatusError(response.StatusCode)
	}
	var items []map[string]any
	if err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&items); err != nil {
		return application.ReviewPublication{}, false, errors.New("REVIEW_RESPONSE_INVALID")
	}
	marker := reviewMarker(work.MarkerDigest)
	for _, item := range items {
		body := stringValue(item, "body")
		if work.ProviderType == "GITLAB" {
			body = stringValue(item, "description")
		}
		if !strings.Contains(body, marker) {
			continue
		}
		if !reviewIdentityMatches(item, work) {
			return application.ReviewPublication{}, false, errors.New("REVIEW_PUBLICATION_IDENTITY_CONFLICT")
		}
		if !providerDraft(item, work.ProviderType) {
			return application.ReviewPublication{}, false, errors.New("REVIEW_PUBLICATION_NOT_DRAFT")
		}
		publication, err := publicationFromMap(item, work.ProviderType)
		return publication, err == nil, err
	}
	return application.ReviewPublication{}, false, nil
}

func (a *ReviewHTTPAdapter) do(ctx context.Context, client *http.Client, method, target string, body []byte, provider string, credential application.DeliveryCredential) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	switch provider {
	case "GITLAB":
		request.Header.Set("PRIVATE-TOKEN", credential.Password)
	case "GITEA":
		request.Header.Set("Authorization", "token "+credential.Password)
	default:
		request.Header.Set("Authorization", "Bearer "+credential.Password)
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	}
	return client.Do(request)
}

func reviewHTTPClient(ctx context.Context, adapter *ReviewHTTPAdapter, work application.ReviewWork) (*http.Client, error) {
	// Tests and explicit in-memory adapters can supply their own validation
	// seam. Those adapters own their transport as well; the production default
	// below is the only path that may dial a real Repository endpoint.
	if adapter.ValidateEgress == nil || reflect.ValueOf(adapter.ValidateEgress).Pointer() != reflect.ValueOf(validateReviewEgress).Pointer() {
		return adapter.Client, nil
	}
	host, port, _, err := reviewAPIEndpoint(work)
	if err != nil {
		return nil, err
	}
	addresses, err := resolveReviewEgress(ctx, work)
	if err != nil {
		return nil, err
	}
	return securehttp.NewPinnedClient(work.TLSPolicy, host, port, addresses, 30*time.Second)
}

func providerAPIBaseURL(work application.ReviewWork) (string, error) {
	host, port, repository, err := reviewAPIEndpoint(work)
	if err != nil {
		return "", err
	}
	urlHost := host
	if port != 443 {
		urlHost = net.JoinHostPort(host, strconv.Itoa(port))
	}
	switch work.ProviderType {
	case "GITHUB":
		if host == "github.com" || host == "api.github.com" {
			return "https://api.github.com/repos/" + repository, nil
		}
		return "https://" + urlHost + "/api/v3/repos/" + repository, nil
	case "GITLAB":
		return "https://" + urlHost + "/api/v4/projects/" + url.PathEscape(repository), nil
	case "GITEA":
		return "https://" + urlHost + "/api/v1/repos/" + repository, nil
	default:
		return "", errors.New("REVIEW_PROVIDER_UNSUPPORTED")
	}
}

func reviewAPIEndpoint(work application.ReviewWork) (string, int, string, error) {
	host, port := strings.ToLower(strings.TrimSpace(work.Host)), work.Port
	repository := strings.Trim(strings.TrimSuffix(work.RepositoryPath, ".git"), "/")
	if strings.TrimSpace(work.WebRepositoryURL) != "" {
		parsed, err := url.Parse(strings.TrimSpace(work.WebRepositoryURL))
		if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return "", 0, "", errors.New("REVIEW_WEB_URL_INVALID")
		}
		webHost, webPort := strings.ToLower(parsed.Hostname()), 443
		if parsed.Port() != "" {
			parsedPort, parseErr := strconv.Atoi(parsed.Port())
			if parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
				return "", 0, "", errors.New("REVIEW_WEB_URL_INVALID")
			}
			webPort = parsedPort
		}
		webPath, decodeErr := url.PathUnescape(strings.Trim(strings.TrimSuffix(parsed.EscapedPath(), ".git"), "/"))
		if decodeErr != nil || webPath == "" {
			return "", 0, "", errors.New("REVIEW_WEB_URL_INVALID")
		}
		if webPath != repository {
			return "", 0, "", errors.New("REVIEW_WEB_URL_IDENTITY_MISMATCH")
		}
		// SSH remotes require the Provider's HTTPS web endpoint. Direct HTTP(S)
		// Project repositories already carry the exact verified API origin in
		// Host/Port; a Provider-reported canonical web URL must not silently
		// redirect credentials or widen the frozen egress target.
		if work.Transport == "SSH" {
			host, port = webHost, webPort
		}
	} else if work.Transport == "SSH" {
		port = 443
	}
	if work.ProviderType == "GITHUB" && host == "github.com" {
		host, port = "api.github.com", 443
	}
	if host == "" || port < 1 || port > 65535 || repository == "" {
		return "", 0, "", errors.New("REVIEW_WEB_URL_INVALID")
	}
	return host, port, repository, nil
}

type reviewEgressPolicy struct {
	AllowPrivateNetwork bool     `json:"allowPrivateNetwork"`
	AllowedHosts        []string `json:"allowedHosts"`
	AllowedCIDRs        []string `json:"allowedCidrs"`
	AllowedPorts        []int    `json:"allowedPorts"`
}

func validateReviewEgress(ctx context.Context, work application.ReviewWork) error {
	_, err := resolveReviewEgress(ctx, work)
	return err
}

func resolveReviewEgress(ctx context.Context, work application.ReviewWork) ([]net.IP, error) {
	var policy reviewEgressPolicy
	if len(work.EgressPolicy) == 0 || json.Unmarshal(work.EgressPolicy, &policy) != nil {
		return nil, errors.New("REVIEW_EGRESS_POLICY_INVALID")
	}
	hostAllowed, portAllowed := false, false
	apiHost, apiPort, _, err := reviewAPIEndpoint(work)
	if err != nil {
		return nil, err
	}
	for _, host := range policy.AllowedHosts {
		if strings.EqualFold(strings.TrimSuffix(host, "."), strings.TrimSuffix(apiHost, ".")) {
			hostAllowed = true
		}
	}
	for _, port := range policy.AllowedPorts {
		if port == apiPort {
			portAllowed = true
		}
	}
	if !hostAllowed || !portAllowed {
		return nil, errors.New("REVIEW_EGRESS_DENIED")
	}
	addresses, err := net.DefaultResolver.LookupIP(ctx, "ip", apiHost)
	if err != nil || len(addresses) == 0 {
		return nil, errors.New("REVIEW_DNS_UNAVAILABLE")
	}
	networks := make([]*net.IPNet, 0, len(policy.AllowedCIDRs))
	for _, value := range policy.AllowedCIDRs {
		_, network, parseErr := net.ParseCIDR(value)
		if parseErr != nil {
			return nil, errors.New("REVIEW_EGRESS_POLICY_INVALID")
		}
		networks = append(networks, network)
	}
	for _, address := range addresses {
		if address.IsUnspecified() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() || address.IsMulticast() || ((address.IsLoopback() || address.IsPrivate()) && !policy.AllowPrivateNetwork) {
			return nil, errors.New("REVIEW_EGRESS_DENIED")
		}
		// Private and loopback addresses must be explicitly pinned to a policy
		// CIDR. This admits the local GitLab/Gitea lab only when the Connection
		// records 127.0.0.1/32 and/or ::1/128; allowPrivateNetwork alone never
		// widens the target.
		if len(networks) > 0 || address.IsLoopback() || address.IsPrivate() {
			matched := false
			for _, network := range networks {
				matched = matched || network.Contains(address)
			}
			if !matched {
				return nil, errors.New("REVIEW_EGRESS_DENIED")
			}
		}
	}
	return addresses, nil
}

func reviewCreateRequest(work application.ReviewWork) (string, map[string]any) {
	title := fmt.Sprintf("Draft: Seven Agent Force %d / %d", work.TaskID, work.TaskTurnID)
	body := "Automated draft review for a verified Seven Agent Force result.\n\n" + reviewMarker(work.MarkerDigest)
	if work.ProviderType == "GITLAB" {
		return "/merge_requests", map[string]any{"source_branch": work.HeadBranch, "target_branch": work.BaseBranch, "title": title, "description": body, "remove_source_branch": false}
	}
	if work.ProviderType == "GITEA" {
		// Gitea's CreatePullRequestOption does not define a draft request
		// field. Its documented WIP title prefixes mark a new PR as draft;
		// the response is still verified below before the side effect is
		// accepted as a successful draft publication.
		return "/pulls", map[string]any{"head": work.HeadBranch, "base": work.BaseBranch, "title": fmt.Sprintf("WIP: Seven Agent Force %d / %d", work.TaskID, work.TaskTurnID), "body": body}
	}
	return "/pulls", map[string]any{"head": work.HeadBranch, "base": work.BaseBranch, "title": title, "body": body, "draft": true}
}

func reviewListPath(work application.ReviewWork) string {
	query := url.Values{}
	if work.ProviderType == "GITLAB" {
		query.Set("scope", "all")
		query.Set("source_branch", work.HeadBranch)
		query.Set("target_branch", work.BaseBranch)
		return "/merge_requests?" + query.Encode()
	}
	if work.ProviderType == "GITEA" {
		query.Set("state", "all")
		query.Set("base_branch", work.BaseBranch)
		return "/pulls?" + query.Encode()
	}
	query.Set("state", "all")
	query.Set("head", githubHeadFilter(work.RepositoryPath, work.HeadBranch))
	query.Set("base", work.BaseBranch)
	return "/pulls?" + query.Encode()
}

func githubHeadFilter(repositoryPath, branch string) string {
	repositoryPath = strings.Trim(strings.TrimSuffix(repositoryPath, ".git"), "/")
	owner, _, ok := strings.Cut(repositoryPath, "/")
	if !ok || owner == "" {
		return branch
	}
	return owner + ":" + branch
}

func reviewMarker(digest string) string { return "<!-- seven-workforce:" + digest + " -->" }

func decodeReviewPublication(reader io.Reader, work application.ReviewWork) (application.ReviewPublication, error) {
	var item map[string]any
	if err := json.NewDecoder(io.LimitReader(reader, 2<<20)).Decode(&item); err != nil {
		return application.ReviewPublication{}, errors.New("REVIEW_RESPONSE_INVALID")
	}
	if !reviewIdentityMatches(item, work) {
		return application.ReviewPublication{}, errors.New("REVIEW_PUBLICATION_IDENTITY_CONFLICT")
	}
	if !providerDraft(item, work.ProviderType) {
		return application.ReviewPublication{}, errors.New("REVIEW_PUBLICATION_NOT_DRAFT")
	}
	return publicationFromMap(item, work.ProviderType)
}

// providerDraft tolerates the documented provider representations while
// keeping normal (non-draft) reviews fail-closed. GitHub uses draft=true;
// GitLab can expose draft or work_in_progress and recognises Draft/WIP titles;
// Gitea versions may expose draft or use the documented WIP/Draft title form.
func providerDraft(item map[string]any, provider string) bool {
	if draft, _ := item["draft"].(bool); draft {
		return true
	}
	title := strings.TrimSpace(stringValue(item, "title"))
	switch provider {
	case "GITLAB":
		if workInProgress, _ := item["work_in_progress"].(bool); workInProgress {
			return true
		}
		return strings.HasPrefix(strings.ToLower(title), "draft:") || strings.HasPrefix(strings.ToLower(title), "wip:")
	case "GITEA":
		return strings.HasPrefix(strings.ToLower(title), "draft:") || strings.HasPrefix(strings.ToLower(title), "wip:")
	default:
		return false
	}
}

// reviewIdentityMatches treats the immutable marker as an idempotency hint,
// not as sufficient proof of identity. A provider response is trusted only
// when its head branch, head commit and base branch all match the frozen
// Delivery/Review work. This prevents a stale or malicious review containing
// the same marker from being accepted during response-loss reconciliation.
func reviewIdentityMatches(item map[string]any, work application.ReviewWork) bool {
	if work.ProviderType == "GITLAB" {
		return stringValue(item, "source_branch") == work.HeadBranch &&
			stringValue(item, "target_branch") == work.BaseBranch &&
			stringValue(item, "sha") == work.HeadCommit
	}
	head, _ := item["head"].(map[string]any)
	base, _ := item["base"].(map[string]any)
	return stringValue(head, "ref") == work.HeadBranch &&
		stringValue(head, "sha") == work.HeadCommit &&
		stringValue(base, "ref") == work.BaseBranch
}

func publicationFromMap(item map[string]any, provider string) (application.ReviewPublication, error) {
	id := anyID(item["id"])
	number := anyID(item["number"])
	web := stringValue(item, "html_url")
	if provider == "GITLAB" {
		number = anyID(item["iid"])
		web = stringValue(item, "web_url")
	}
	if id == "" || number == "" || web == "" {
		return application.ReviewPublication{}, errors.New("REVIEW_RESPONSE_INVALID")
	}
	return application.ReviewPublication{ProviderReviewID: id, ProviderNumber: number, WebURL: web}, nil
}

func anyID(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

func stringValue(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
}

func reviewStatusError(status int) error {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return errors.New("REVIEW_PERMISSION_DENIED")
	}
	if status == http.StatusConflict || status == http.StatusUnprocessableEntity {
		return errors.New("REVIEW_PUBLICATION_CONFLICT")
	}
	if status >= 500 {
		return errors.New("REVIEW_REMOTE_UNAVAILABLE")
	}
	return errors.New("REVIEW_PUBLICATION_FAILED")
}

var _ application.ReviewProvider = (*ReviewHTTPAdapter)(nil)
