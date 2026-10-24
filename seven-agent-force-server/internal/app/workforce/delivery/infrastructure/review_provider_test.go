package infrastructure

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
)

func TestReviewAdapterProviderContractsCreateDraftWithFrozenIdentity(t *testing.T) {
	t.Parallel()
	tests := []struct {
		provider, authHeader, authValue string
		branchBody, createBody          string
	}{
		{
			provider: "GITHUB", authHeader: "Authorization", authValue: "Bearer secret",
			branchBody: `{"commit":{"sha":"base-commit"}}`,
			createBody: `{"id":12,"number":7,"html_url":"https://git.example/review/7","draft":true,"head":{"ref":"seven/task-1","sha":"head-commit"},"base":{"ref":"main","sha":"base-commit"}}`,
		},
		{
			provider: "GITEA", authHeader: "Authorization", authValue: "token secret",
			branchBody: `{"commit":{"id":"base-commit"}}`,
			createBody: `{"id":12,"number":7,"html_url":"https://git.example/review/7","draft":true,"head":{"ref":"seven/task-1","sha":"head-commit"},"base":{"ref":"main","sha":"base-commit"}}`,
		},
		{
			provider: "GITLAB", authHeader: "PRIVATE-TOKEN", authValue: "secret",
			branchBody: `{"commit":{"id":"base-commit"}}`,
			createBody: `{"id":12,"iid":7,"web_url":"https://git.example/review/7","draft":true,"source_branch":"seven/task-1","target_branch":"main","sha":"head-commit"}`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.provider, func(t *testing.T) {
			t.Parallel()
			var posts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Header.Get(test.authHeader) != test.authValue {
					t.Errorf("%s authorization missing", test.provider)
					writer.WriteHeader(http.StatusUnauthorized)
					return
				}
				if test.provider == "GITHUB" && (request.Header.Get("X-GitHub-Api-Version") != "2026-03-10" || request.Header.Get("Accept") != "application/vnd.github+json") {
					t.Errorf("GitHub versioned media headers missing: %+v", request.Header)
				}
				if strings.Contains(request.URL.Path, "/branches/") {
					_, _ = writer.Write([]byte(test.branchBody))
					return
				}
				if request.Method == http.MethodGet {
					assertListQuery(t, test.provider, request)
					_, _ = writer.Write([]byte(`[]`))
					return
				}
				posts.Add(1)
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Error(err)
					writer.WriteHeader(http.StatusBadRequest)
					return
				}
				assertCreatePayload(t, test.provider, body)
				writer.Header().Set("Content-Type", "application/json")
				_, _ = writer.Write([]byte(test.createBody))
			}))
			defer server.Close()

			adapter := testReviewAdapter(server.URL)
			got, err := adapter.PublishDraft(context.Background(), reviewWork(test.provider, "marker"), application.DeliveryCredential{Password: "secret"})
			if err != nil || got.ProviderNumber != "7" || posts.Load() != 1 || got.ObservedBase != "base-commit" {
				t.Fatalf("got=%+v posts=%d err=%v", got, posts.Load(), err)
			}
		})
	}
}

func TestReviewAdapterResponseLossReplayFindsOnlyExactFrozenIdentity(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/branches/") {
			_, _ = writer.Write([]byte(`{"commit":{"sha":"base-commit"}}`))
			return
		}
		if request.Method == http.MethodPost {
			posts.Add(1)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = writer.Write([]byte(`[{"id":21,"number":9,"html_url":"https://git.example/review/9","draft":true,"body":"<!-- seven-workforce:stable -->","head":{"ref":"seven/task-1","sha":"head-commit"},"base":{"ref":"main","sha":"base-commit"}}]`))
	}))
	defer server.Close()
	adapter := testReviewAdapter(server.URL)
	got, err := adapter.PublishDraft(context.Background(), reviewWork("GITHUB", "stable"), application.DeliveryCredential{Password: "secret"})
	if err != nil || got.ProviderReviewID != "21" || posts.Load() != 0 {
		t.Fatalf("got=%+v posts=%d err=%v", got, posts.Load(), err)
	}
}

func TestReviewAdapterPostSideEffectResponseLossReconcilesWithoutDuplicate(t *testing.T) {
	t.Parallel()
	var created atomic.Bool
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/branches/") {
			_, _ = writer.Write([]byte(`{"commit":{"sha":"base-commit"}}`))
			return
		}
		if request.Method == http.MethodGet {
			if !created.Load() {
				_, _ = writer.Write([]byte(`[]`))
				return
			}
			_, _ = writer.Write([]byte(`[{"id":31,"number":11,"html_url":"https://git.example/review/11","draft":true,"body":"<!-- seven-workforce:lost -->","head":{"ref":"seven/task-1","sha":"head-commit"},"base":{"ref":"main","sha":"base-commit"}}]`))
			return
		}
		posts.Add(1)
		created.Store(true)
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Error("test server does not support hijacking")
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	adapter := testReviewAdapter(server.URL)
	work := reviewWork("GITHUB", "lost")
	if _, err := adapter.PublishDraft(context.Background(), work, application.DeliveryCredential{Password: "secret"}); err == nil || err.Error() != "REVIEW_RESPONSE_LOST" {
		t.Fatalf("first err=%v", err)
	}
	got, err := adapter.PublishDraft(context.Background(), work, application.DeliveryCredential{Password: "secret"})
	if err != nil || got.ProviderReviewID != "31" || posts.Load() != 1 {
		t.Fatalf("got=%+v posts=%d err=%v", got, posts.Load(), err)
	}
}

func TestReviewAdapterReadbackReconciliationNeverPosts(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts.Add(1)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.Contains(request.URL.Path, "/branches/") {
			_, _ = writer.Write([]byte(`{"commit":{"sha":"base-commit"}}`))
			return
		}
		_, _ = writer.Write([]byte(`[{"id":31,"number":11,"html_url":"https://git.example/review/11","draft":true,"body":"<!-- seven-workforce:readback -->","head":{"ref":"seven/task-1","sha":"head-commit"},"base":{"ref":"main","sha":"base-commit"}}]`))
	}))
	defer server.Close()
	adapter := testReviewAdapter(server.URL)
	publication, found, err := adapter.ReconcileDraft(context.Background(), reviewWork("GITHUB", "readback"), application.DeliveryCredential{Password: "secret"})
	if err != nil || !found || publication.ProviderReviewID != "31" || posts.Load() != 0 {
		t.Fatalf("publication=%+v found=%v posts=%d err=%v", publication, found, posts.Load(), err)
	}
}

func TestReviewAdapterRejectsMarkerReplayWithDifferentHeadCommit(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/branches/") {
			_, _ = writer.Write([]byte(`{"commit":{"sha":"base-commit"}}`))
			return
		}
		_, _ = writer.Write([]byte(`[{"id":21,"number":9,"html_url":"https://git.example/review/9","draft":true,"body":"<!-- seven-workforce:stable -->","head":{"ref":"seven/task-1","sha":"different-commit"},"base":{"ref":"main","sha":"base-commit"}}]`))
	}))
	defer server.Close()
	adapter := testReviewAdapter(server.URL)
	_, err := adapter.PublishDraft(context.Background(), reviewWork("GITHUB", "stable"), application.DeliveryCredential{Password: "secret"})
	if err == nil || err.Error() != "REVIEW_PUBLICATION_IDENTITY_CONFLICT" {
		t.Fatalf("err=%v", err)
	}
}

func TestReviewAdapterRejectsCreateResponseWithDifferentFrozenIdentity(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/branches/") {
			_, _ = writer.Write([]byte(`{"commit":{"sha":"base-commit"}}`))
			return
		}
		if request.Method == http.MethodGet {
			_, _ = writer.Write([]byte(`[]`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":12,"number":7,"html_url":"https://git.example/review/7","draft":true,"head":{"ref":"seven/task-1","sha":"different-commit"},"base":{"ref":"main","sha":"base-commit"}}`))
	}))
	defer server.Close()
	adapter := testReviewAdapter(server.URL)
	_, err := adapter.PublishDraft(context.Background(), reviewWork("GITHUB", "stable"), application.DeliveryCredential{Password: "secret"})
	if err == nil || err.Error() != "REVIEW_PUBLICATION_IDENTITY_CONFLICT" {
		t.Fatalf("err=%v", err)
	}
}

func TestReviewAdapterRejectsProviderResponseThatIsNotDraft(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.Path, "/branches/") {
			_, _ = writer.Write([]byte(`{"commit":{"sha":"base-commit"}}`))
			return
		}
		if request.Method == http.MethodGet {
			_, _ = writer.Write([]byte(`[]`))
			return
		}
		_, _ = writer.Write([]byte(`{"id":12,"number":7,"html_url":"https://git.example/review/7","draft":false,"head":{"ref":"seven/task-1","sha":"head-commit"},"base":{"ref":"main","sha":"base-commit"}}`))
	}))
	defer server.Close()
	adapter := testReviewAdapter(server.URL)
	_, err := adapter.PublishDraft(context.Background(), reviewWork("GITHUB", "stable"), application.DeliveryCredential{Password: "secret"})
	if err == nil || err.Error() != "REVIEW_PUBLICATION_NOT_DRAFT" {
		t.Fatalf("err=%v", err)
	}
}

func TestProviderDraftAcceptsDocumentedGitLabAndGiteaRepresentations(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		provider string
		item     map[string]any
		want     bool
	}{
		{"GITHUB", map[string]any{"draft": true}, true},
		{"GITHUB", map[string]any{"title": "Draft: title"}, false},
		{"GITLAB", map[string]any{"work_in_progress": true}, true},
		{"GITLAB", map[string]any{"title": "Draft: title"}, true},
		{"GITEA", map[string]any{"draft": true}, true},
		{"GITEA", map[string]any{"title": "WIP: title"}, true},
		{"GITEA", map[string]any{"title": "normal pull request"}, false},
	} {
		if got := providerDraft(test.item, test.provider); got != test.want {
			t.Fatalf("provider=%s item=%v got=%t want=%t", test.provider, test.item, got, test.want)
		}
	}
}

func TestReviewAdapterRefusesDriftedFrozenBase(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts.Add(1)
		}
		_, _ = writer.Write([]byte(`{"commit":{"sha":"new-base"}}`))
	}))
	defer server.Close()
	adapter := testReviewAdapter(server.URL)
	_, err := adapter.PublishDraft(context.Background(), reviewWork("GITHUB", "stable"), application.DeliveryCredential{Password: "secret"})
	if err == nil || err.Error() != "REVIEW_BASE_CHANGED" || posts.Load() != 0 {
		t.Fatalf("err=%v posts=%d", err, posts.Load())
	}
}

func testReviewAdapter(base string) *ReviewHTTPAdapter {
	adapter := NewReviewHTTPAdapter()
	adapter.BaseURL = func(application.ReviewWork) (string, error) { return base, nil }
	adapter.ValidateEgress = func(context.Context, application.ReviewWork) error { return nil }
	return adapter
}

func reviewWork(provider, marker string) application.ReviewWork {
	return application.ReviewWork{
		ProviderType: provider, Transport: "HTTPS", Host: "git.acme.test", Port: 443, TaskID: 2610240832, TaskTurnID: 637649102,
		RepositoryPath: "acme/repo.git",
		HeadBranch:     "seven/task-1", BaseBranch: "main", HeadCommit: "head-commit",
		FrozenBaseCommit: "base-commit", MarkerDigest: marker,
	}
}

func TestReviewAPIEndpointSeparatesSSHGitAndHTTPSAPI(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, provider, want string
	}{
		{"GitLab", "GITLAB", "https://git.acme.test/api/v4/projects/acme%2Frepo"},
		{"Gitea", "GITEA", "https://git.acme.test/api/v1/repos/acme/repo"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			work := reviewWork(test.provider, "marker")
			work.Transport = "SSH"
			work.Host = "ssh.git.acme.test"
			work.Port = 22
			work.WebRepositoryURL = "https://git.acme.test/acme/repo"
			got, err := providerAPIBaseURL(work)
			if err != nil || got != test.want {
				t.Fatalf("got=%s want=%s err=%v", got, test.want, err)
			}
		})
	}
}

func TestReviewAPIEndpointRejectsMismatchedRepositoryIdentity(t *testing.T) {
	t.Parallel()
	work := reviewWork("GITLAB", "marker")
	work.Transport = "SSH"
	work.Port = 22
	work.WebRepositoryURL = "https://git.acme.test/acme/other"
	_, err := providerAPIBaseURL(work)
	if err == nil || err.Error() != "REVIEW_WEB_URL_IDENTITY_MISMATCH" {
		t.Fatalf("err=%v", err)
	}
}

func TestReviewAPIEndpointKeepsVerifiedHTTPSProjectOrigin(t *testing.T) {
	t.Parallel()
	work := reviewWork("GITLAB", "marker")
	work.Host = "127.0.0.1"
	work.Port = 3444
	work.WebRepositoryURL = "https://gitlab.localhost:3444/acme/repo"
	got, err := providerAPIBaseURL(work)
	if err != nil || got != "https://127.0.0.1:3444/api/v4/projects/acme%2Frepo" {
		t.Fatalf("got=%s err=%v", got, err)
	}
}

func assertCreatePayload(t *testing.T, provider string, body map[string]any) {
	t.Helper()
	if provider == "GITLAB" {
		if body["source_branch"] != "seven/task-1" || body["target_branch"] != "main" || !strings.HasPrefix(body["title"].(string), "Draft:") || !strings.Contains(body["description"].(string), "seven-workforce:marker") {
			t.Fatalf("GitLab body=%+v", body)
		}
		return
	}
	if provider == "GITEA" {
		if _, exists := body["draft"]; exists || !strings.HasPrefix(body["title"].(string), "WIP:") || body["head"] != "seven/task-1" || body["base"] != "main" || !strings.Contains(body["body"].(string), "seven-workforce:marker") {
			t.Fatalf("Gitea body=%+v", body)
		}
		return
	}
	if body["draft"] != true || body["head"] != "seven/task-1" || body["base"] != "main" || !strings.Contains(body["body"].(string), "seven-workforce:marker") {
		t.Fatalf("%s body=%+v", provider, body)
	}
}

func assertListQuery(t *testing.T, provider string, request *http.Request) {
	t.Helper()
	query := request.URL.Query()
	switch provider {
	case "GITLAB":
		if query.Get("source_branch") != "seven/task-1" || query.Get("target_branch") != "main" || query.Get("scope") != "all" {
			t.Fatalf("GitLab query=%s", request.URL.RawQuery)
		}
	case "GITEA":
		if query.Get("base_branch") != "main" || query.Get("state") != "all" || query.Has("head") || query.Has("base") {
			t.Fatalf("Gitea query=%s", request.URL.RawQuery)
		}
	default:
		if query.Get("head") != "acme:seven/task-1" || query.Get("base") != "main" || query.Get("state") != "all" {
			t.Fatalf("GitHub query=%s", request.URL.RawQuery)
		}
	}
}
