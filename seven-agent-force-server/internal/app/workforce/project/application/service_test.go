package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/domain"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
)

type listCaptureRepository struct {
	request       ListRequest
	project       domain.Project
	updated       UpsertRequest
	processorSeen int64
}

type repositoryVerifierStub struct{ verification RepositoryVerification }

func (v repositoryVerifierStub) Verify(context.Context, domain.ParsedRepositoryURL, string) (RepositoryVerification, error) {
	return v.verification, nil
}

type credentialCipherStub struct{ encrypted int }

func (c *credentialCipherStub) EncryptRepositoryCredential(context.Context, domain.RepositoryCredential) (CredentialEnvelope, error) {
	c.encrypted++
	return CredentialEnvelope{Ciphertext: "cipher", EDEK: "edek", WrapKeyRef: "key"}, nil
}
func (c *credentialCipherStub) DecryptRepositoryCredential(context.Context, string, string, string) (domain.RepositoryCredential, error) {
	return domain.RepositoryCredential{Username: "user", Token: "token"}, nil
}

func (r *listCaptureRepository) Create(context.Context, int64, int64, UpsertRequest) (domain.Project, error) {
	return domain.Project{}, nil
}
func (r *listCaptureRepository) Update(_ context.Context, _ int64, _ int64, _ int64, request UpsertRequest) (domain.Project, error) {
	r.updated = request
	return r.project, nil
}
func (r *listCaptureRepository) Get(context.Context, int64, int64) (domain.Project, error) {
	return r.project, nil
}
func (r *listCaptureRepository) List(_ context.Context, _ int64, request ListRequest) (ProjectPage, error) {
	r.request = request
	return ProjectPage{Items: []domain.Project{{ID: 9}}, NextCursor: 8}, nil
}
func (r *listCaptureRepository) Archive(context.Context, int64, int64, int64, int64) (domain.Project, error) {
	return domain.Project{}, nil
}
func (r *listCaptureRepository) LatestAvailableSource(context.Context, int64, int64, int64) (int64, error) {
	return 0, nil
}
func (r *listCaptureRepository) ResolveProcessorRelease(_ context.Context, _ int64, releaseID int64) (int64, error) {
	r.processorSeen = releaseID
	if releaseID == 0 {
		return 101, nil
	}
	return releaseID, nil
}
func (r *listCaptureRepository) ListPreparingSources(context.Context, int) ([]PreparingSource, error) {
	return nil, nil
}
func (r *listCaptureRepository) MarkSourcePreparationFailed(context.Context, int64, int64, int64) error {
	return nil
}

func TestNormalizeProjectKindsAndDeliveryBoundary(t *testing.T) {
	codebase := UpsertRequest{
		ProjectKey: " Seven-Workforce ", Name: " Seven Agent Force ", DepartmentID: 7, Kind: "codebase",
		Repository:                &RepositorySelection{CloneURL: "https://seven:token@git.example.com/group/repo.git", DefaultRefName: " main "},
		Delivery:                  DeliveryPolicy{Mode: "new_branch", Authorization: "auto_after_gates", BranchPattern: "codex/{task-id}"},
		DeliveryApproveAuthorized: true,
	}
	normalized, err := normalize(codebase, false)
	if err != nil {
		t.Fatalf("normalize CODEBASE project: %v", err)
	}
	if normalized.ProjectKey != "seven-workforce" || normalized.Repository.DefaultRefName != "main" || normalized.Delivery.Authorization != "AUTO_AFTER_GATES" {
		t.Fatalf("unexpected CODEBASE normalization: %#v", normalized)
	}
	codebase.DeliveryApproveAuthorized = false
	if _, err = normalize(codebase, false); !errors.Is(err, domain.ErrDeliveryApprovalRequired) {
		t.Fatalf("AUTO project accepted without delivery approval permission: %v", err)
	}
	codebase.DeliveryApproveAuthorized = true
	codebase.Delivery.BranchPattern = "codex/shared"
	if _, err = normalize(codebase, false); err == nil {
		t.Fatal("CODEBASE project accepted a non-unique result branch pattern")
	}

	general := UpsertRequest{ProjectKey: "general", Name: "General", DepartmentID: 7, Kind: projectfacade.KindGeneral, Delivery: DeliveryPolicy{Mode: "NONE"}}
	normalizedGeneral, err := normalize(general, false)
	if err != nil {
		t.Fatalf("normalize GENERAL project: %v", err)
	}
	if normalizedGeneral.Delivery.Authorization != "NONE" {
		t.Fatalf("NONE delivery must use the canonical project authorization: %#v", normalizedGeneral.Delivery)
	}
	general.DepartmentID = 0
	if _, err = normalize(general, false); err != nil {
		t.Fatalf("optional department blocked Project creation: %v", err)
	}
	general.Repository = &RepositorySelection{CloneURL: "https://seven:token@git.example.com/group/repo.git", DefaultRefName: "main"}
	if _, err = normalize(general, false); err == nil {
		t.Fatal("GENERAL project accepted a repository")
	}
	general.Repository = nil
	general.Delivery = DeliveryPolicy{Mode: "NEW_BRANCH", Authorization: "ASK_AFTER_GATES", BranchPattern: "codex/{task-id}"}
	if _, err = normalize(general, false); err == nil {
		t.Fatal("GENERAL project accepted Git delivery")
	}
}

func TestNormalizeProjectUpdateRequiresExpectedVersionID(t *testing.T) {
	request := UpsertRequest{ProjectKey: "general", Name: "General", DepartmentID: 7, Kind: projectfacade.KindGeneral, Delivery: DeliveryPolicy{Mode: "NONE"}}
	if _, err := normalize(request, true); err == nil {
		t.Fatal("project update accepted a missing expectedVersionId")
	}
	request.ExpectedVersionID = 3
	if _, err := normalize(request, true); err != nil {
		t.Fatalf("project update rejected expectedVersionId: %v", err)
	}
}

func TestNormalizeProjectKeepsOrderedHookReleasesAndRejectsInvalidSelection(t *testing.T) {
	request := UpsertRequest{
		ProjectKey: "general", Name: "General", Kind: projectfacade.KindGeneral,
		HookReleaseIDs: sharedtransport.Int64Slice{30, 10, 20}, Delivery: DeliveryPolicy{Mode: "NONE"},
	}
	normalized, err := normalize(request, false)
	if err != nil {
		t.Fatalf("normalize ordered Hook releases: %v", err)
	}
	if got := normalized.HookReleaseIDs; len(got) != 3 || got[0] != 30 || got[1] != 10 || got[2] != 20 {
		t.Fatalf("Hook execution order changed: %#v", got)
	}

	for name, values := range map[string]sharedtransport.Int64Slice{
		"duplicate":    {10, 10},
		"non-positive": {0},
		"too-many": func() sharedtransport.Int64Slice {
			result := make(sharedtransport.Int64Slice, 65)
			for index := range result {
				result[index] = int64(index + 1)
			}
			return result
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			request.HookReleaseIDs = values
			if _, err := normalize(request, false); !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("invalid Hook selection accepted: %v", err)
			}
		})
	}
}

func TestResolveRepositoryKeepsPublicRepositoryCredentialFree(t *testing.T) {
	cipher := &credentialCipherStub{}
	service := New(&listCaptureRepository{})
	service.BindCredentialCipher(cipher)
	service.BindRepositoryVerifier(repositoryVerifierStub{verification: RepositoryVerification{ProviderType: domain.RepositoryProviderGeneric, DefaultRefName: "main", Capabilities: domain.RepositoryCapabilities{Read: true}}})
	request := UpsertRequest{Kind: projectfacade.KindCodebase, Repository: &RepositorySelection{CloneURL: "https://git.example.test/group/repo.git"}, Delivery: DeliveryPolicy{Mode: "NONE"}}
	if err := service.resolveRepository(context.Background(), &request, nil); err != nil {
		t.Fatalf("resolve public repository: %v", err)
	}
	if cipher.encrypted != 0 || request.ResolvedRepository.CredentialVersion != 0 || request.ResolvedRepository.CredentialCiphertext != "" {
		t.Fatalf("public repository acquired a fake credential: %#v", request.ResolvedRepository)
	}
}

func TestResolveRepositoryRevalidatesDeliveryWhenURLIsPreserved(t *testing.T) {
	service := New(&listCaptureRepository{})
	current := domain.Project{RepositoryDisplayURL: "https://git.example.test/group/repo.git", RepositoryScheme: "https", RepositoryHost: "git.example.test", RepositoryPort: 443, RepositoryPath: "group/repo.git", DefaultRefName: "main", RepositoryProviderType: domain.RepositoryProviderGeneric, RepositoryConfigDigest: strings.Repeat("a", 64), RepositorySourceState: domain.RepositorySourceReady, RepositoryCapabilities: domain.RepositoryCapabilities{Read: true}}
	request := UpsertRequest{Kind: projectfacade.KindCodebase, Repository: &RepositorySelection{}, Delivery: DeliveryPolicy{Mode: "NEW_BRANCH", Authorization: "ASK_AFTER_GATES", BranchPattern: "codex/{task-id}"}}
	if err := service.resolveRepository(context.Background(), &request, &current); !errors.Is(err, domain.ErrRepositoryCredentialRequired) {
		t.Fatalf("delivery was enabled on a preserved read-only repository: %v", err)
	}
}

func TestProjectUpdatePreservesBoundProcessorReleaseAndReviewPrompt(t *testing.T) {
	repository := &listCaptureRepository{project: domain.Project{
		ID: 9, VersionID: 4, Status: projectfacade.StatusActive, ResultProcessorReleaseID: 301,
		ReviewPrompt: "project review guidance",
	}}
	service := New(repository)
	request := UpsertRequest{
		ExpectedVersionID: 4, ProjectKey: "general", Name: "Renamed", DepartmentID: 7,
		Kind: projectfacade.KindGeneral, ReviewPrompt: "updated review guidance",
		Delivery: DeliveryPolicy{Mode: "NONE"},
	}
	if _, err := service.Update(context.Background(), 1, 2, 9, request); err != nil {
		t.Fatalf("update project: %v", err)
	}
	if repository.processorSeen != 301 {
		t.Fatalf("bound processor was not preserved: processor=%d", repository.processorSeen)
	}
	if repository.updated.ResolvedProcessorReleaseID != 301 || repository.updated.ReviewPrompt != "updated review guidance" {
		t.Fatalf("update silently rebound automation: %#v", repository.updated)
	}
}

func TestMergeCompletionCheckOnlyStrengthensProjectCheck(t *testing.T) {
	base, err := json.Marshal(completioncheck.Input{Mode: "COMMAND", Command: "make server-check"})
	if err != nil {
		t.Fatal(err)
	}
	extra := completioncheck.Input{Mode: "COMMAND", Command: "make web-check"}
	merged, err := mergeCompletionCheck(base, &extra)
	if err != nil {
		t.Fatalf("merge completion check: %v", err)
	}
	if merged.Mode != "SCRIPT" || !strings.Contains(merged.Script, "make server-check") || !strings.Contains(merged.Script, "make web-check") {
		t.Fatalf("base and task checks were not preserved: %#v", merged)
	}
	weaker := completioncheck.Input{Mode: "NONE"}
	if _, err = mergeCompletionCheck(base, &weaker); err == nil {
		t.Fatal("task completion check was allowed to disable the project check")
	}
}

func TestMergeCompletionCheckCanonicalizesAnEmptyProjectCheck(t *testing.T) {
	withoutAddition, err := mergeCompletionCheck(json.RawMessage(`{"mode":"NONE"}`), nil)
	if err != nil {
		t.Fatalf("merge canonical empty project check: %v", err)
	}
	if withoutAddition == nil || withoutAddition.Mode != "NONE" {
		t.Fatalf("empty project check was not preserved canonically: %#v", withoutAddition)
	}

	addition := completioncheck.Input{Mode: "COMMAND", Command: "go test ./..."}
	withAddition, err := mergeCompletionCheck(json.RawMessage(`{"mode":"NONE"}`), &addition)
	if err != nil {
		t.Fatalf("add a task check to an empty project check: %v", err)
	}
	if withAddition == nil || withAddition.Mode != "COMMAND" || withAddition.Command != "go test ./..." {
		t.Fatalf("task check did not replace the canonical empty project check: %#v", withAddition)
	}
}

func TestCompileRequirementsContainsOnlyBusinessEnabledCapabilities(t *testing.T) {
	project := domain.Project{}
	withoutDelivery := compileRequirements(project, projectfacade.FrozenGitPlan{DeliveryMode: "NONE"})
	if containsValue(withoutDelivery.AllowedPlatformTools, "request_git_delivery") {
		t.Fatal("Git delivery tool was enabled without a frozen delivery plan")
	}
	if !containsValue(withoutDelivery.AllowedPlatformTools, "run_command_with_network_access") || !containsValue(withoutDelivery.AdapterCapabilities, "execution.suspend") {
		t.Fatalf("required platform capabilities missing: %#v", withoutDelivery)
	}
	withDelivery := compileRequirements(project, projectfacade.FrozenGitPlan{DeliveryMode: "NEW_BRANCH"})
	if !containsValue(withDelivery.AllowedPlatformTools, "request_git_delivery") {
		t.Fatal("Git delivery tool was not enabled for the frozen delivery plan")
	}
	if len(withDelivery.EnvironmentCapabilities) != 0 {
		t.Fatalf("project must not hide runtime requirements: %#v", withDelivery.EnvironmentCapabilities)
	}
}

func TestListNormalizesServerSideFilterAndCursor(t *testing.T) {
	repository := &listCaptureRepository{}
	service := New(repository)
	page, err := service.List(context.Background(), 7, ListRequest{
		Query: "  Seven  ", Status: "active", Cursor: 99, Limit: 20,
	})
	if err != nil {
		t.Fatalf("list Projects: %v", err)
	}
	if repository.request.Query != "seven" || repository.request.Status != "ACTIVE" || repository.request.Cursor != 99 || repository.request.Limit != 20 {
		t.Fatalf("list request was not normalized for server-side filtering: %#v", repository.request)
	}
	if len(page.Items) != 1 || page.NextCursor != 8 {
		t.Fatalf("list page changed: %#v", page)
	}

	for _, invalid := range []ListRequest{{Status: "DELETED"}, {Cursor: -1}, {Query: strings.Repeat("x", 201)}} {
		if _, err = service.List(context.Background(), 7, invalid); !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("invalid list request %#v returned %v", invalid, err)
		}
	}
}

func containsValue(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
