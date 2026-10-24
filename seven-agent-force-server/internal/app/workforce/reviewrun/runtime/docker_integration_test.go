package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
)

// TestDockerRunIsolatesFrozenResult is opt-in because it launches a real
// local Docker container. CI/local verification supplies a precise image
// digest through REVIEW_RUNTIME_DOCKER_IMAGE; tags are deliberately rejected.
func TestDockerRunIsolatesFrozenResult(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_DOCKER_IMAGE"))
	if image == "" {
		t.Skip("set REVIEW_RUNTIME_DOCKER_IMAGE to an exact local Fake Reviewer digest")
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		t.Fatalf("REVIEW_RUNTIME_DOCKER_IMAGE must be an exact sha256 digest, got %q", image)
	}
	if _, err := commandOutput(context.Background(), "docker", "image", "inspect", image); err != nil {
		t.Skipf("exact Fake Reviewer image is unavailable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	workspaceRoot := filepath.Join(t.TempDir(), "worker-workspaces")
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	workspace, err := attemptruntime.OpenPersistentWorkspace(workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	const taskID, turnID, finalizationID, reviewRunID, workerID, ownerEpoch int64 = 9101, 9102, 9103, 9104, 9105, 1
	commit, tree, resultRoot := installFrozenDockerResult(t, ctx, workspace, taskID, turnID, finalizationID)
	beforeCommit, err := commandOutput(ctx, "git", "--git-dir="+resultRoot.RepositoryRoot, "rev-parse", commit+"^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	beforeTree, err := commandOutput(ctx, "git", "--git-dir="+resultRoot.RepositoryRoot, "rev-parse", commit+"^{tree}")
	if err != nil {
		t.Fatal(err)
	}

	runtime := NewDocker(config.WorkerConfig{})
	runtime.BindPersistentWorkspace(workspace)
	work := Work{ReviewRunID: reviewRunID, WorkerID: workerID, TaskID: taskID, TaskTurnID: turnID, FinalizationRunID: finalizationID, ResultCommit: commit, ResultTree: tree, ReviewerImage: image, OwnerEpoch: ownerEpoch, HardTimeout: 20 * time.Second, ReviewContract: []byte(`{"version":"independent-review/v1"}`), Requirements: []byte(`{"instruction":"inspect the frozen result"}`)}
	var inspected dockerIsolationInspect
	result, err := runtime.Run(ctx, work, Gate{ContainerCreated: func(container Container) error {
		body, inspectErr := commandOutput(ctx, "docker", "inspect", container.ID)
		if inspectErr != nil {
			return inspectErr
		}
		if err = json.Unmarshal(body, &inspected); err != nil || len(inspected) != 1 {
			return ErrResultIdentityInvalid
		}
		return nil
	}})
	if err != nil {
		t.Fatalf("run real isolated Fake Reviewer: %v", err)
	}
	if result.Container.ID == "" || len(result.Report) == 0 || !strings.Contains(string(result.Report), `"verdict":"PASS"`) {
		t.Fatalf("unexpected Fake Reviewer result: container=%+v report=%s", result.Container, result.Report)
	}
	assertDockerIsolation(t, inspected, resultRoot, workspace, taskID, reviewRunID, ownerEpoch)

	afterCommit, err := commandOutput(ctx, "git", "--git-dir="+resultRoot.RepositoryRoot, "rev-parse", commit+"^{commit}")
	if err != nil || strings.TrimSpace(string(afterCommit)) != strings.TrimSpace(string(beforeCommit)) {
		t.Fatalf("Reviewer changed ResultCommit: got=%q err=%v", afterCommit, err)
	}
	afterTree, err := commandOutput(ctx, "git", "--git-dir="+resultRoot.RepositoryRoot, "rev-parse", commit+"^{tree}")
	if err != nil || strings.TrimSpace(string(afterTree)) != strings.TrimSpace(string(beforeTree)) {
		t.Fatalf("Reviewer changed ResultTree: got=%q err=%v", afterTree, err)
	}
	review, err := workspace.ReviewAttempt(taskID, reviewRunID, ownerEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(review.Root, "scratch", "repo", ".git")); err != nil {
		t.Fatalf("isolated COW scratch was not writable: %v", err)
	}
	if _, err = os.Stat(filepath.Join(review.Root, "output", "review-report.json")); !os.IsNotExist(err) {
		t.Fatalf("untrusted candidate report escaped the ephemeral report volume: %v", err)
	}
	if volumes, volumeErr := commandOutput(ctx, "docker", "volume", "ls", "-q", "--filter", "label=seven.review.report=true", "--filter", "label=seven.review.run.id="+strconv.FormatInt(reviewRunID, 10)); volumeErr != nil || len(strings.Fields(string(volumes))) != 0 {
		t.Fatalf("review candidate report volume survived completion: volumes=%q err=%v", volumes, volumeErr)
	}
}

// TestDockerRunUsesOnlyRunScopedProviderRelay launches the immutable
// PROVIDER_PASS image. Its only provider call is HTTP over the mounted Unix
// socket; Docker still has --network none and the fixture cannot receive a
// Worker token or an upstream Provider credential.
func TestDockerRunUsesOnlyRunScopedProviderRelay(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_PROVIDER_DOCKER_IMAGE"))
	if image == "" {
		t.Skip("set REVIEW_RUNTIME_PROVIDER_DOCKER_IMAGE to the exact PROVIDER_PASS fixture digest")
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		t.Fatalf("REVIEW_RUNTIME_PROVIDER_DOCKER_IMAGE must be an exact sha256 digest, got %q", image)
	}
	if _, err := commandOutput(context.Background(), "docker", "image", "inspect", image); err != nil {
		t.Skipf("exact PROVIDER_PASS image is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	workspace, err := attemptruntime.OpenPersistentWorkspace(filepath.Join(t.TempDir(), "worker-workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	const taskID, turnID, finalizationID, reviewRunID, workerID, ownerEpoch int64 = 9201, 9202, 9203, 9204, 9205, 1
	commit, tree, resultRoot := installFrozenDockerResultWithIDs(t, ctx, workspace, taskID, turnID, finalizationID)
	runtime := NewDocker(config.WorkerConfig{})
	runtime.BindPersistentWorkspace(workspace)
	var inspected dockerIsolationInspect
	var relayInspected dockerIsolationInspect
	result, err := runtime.Run(ctx, Work{ReviewRunID: reviewRunID, WorkerID: workerID, TaskID: taskID, TaskTurnID: turnID, FinalizationRunID: finalizationID, ResultCommit: commit, ResultTree: tree, ReviewerImage: image, OwnerEpoch: ownerEpoch, HardTimeout: 20 * time.Second, ReviewContract: []byte(`{"version":"independent-review/v1"}`), Requirements: []byte(`{"instruction":"provider boundary fixture"}`), Provider: ProviderIdentity{CredentialID: 99, CredentialVersion: 1, Protocol: "OPENAI", PrimaryModel: "fake-review-model", StructuredContract: "independent-reviewer/structured-output/v1"}}, Gate{ContainerCreated: func(container Container) error {
		body, inspectErr := commandOutput(ctx, "docker", "inspect", container.ID)
		if inspectErr != nil {
			return inspectErr
		}
		if err = json.Unmarshal(body, &inspected); err != nil || len(inspected) != 1 {
			return ErrResultIdentityInvalid
		}
		relayIDs, relayErr := commandOutput(ctx, "docker", "ps", "-q", "--filter", "label=seven.review.relay=true", "--filter", "label=seven.review.run.id=9204")
		if relayErr != nil || len(strings.Fields(string(relayIDs))) != 1 {
			return ErrResultIdentityInvalid
		}
		relayBody, relayInspectErr := commandOutput(ctx, "docker", "inspect", strings.TrimSpace(string(relayIDs)))
		if relayInspectErr != nil || json.Unmarshal(relayBody, &relayInspected) != nil || len(relayInspected) != 1 {
			return ErrResultIdentityInvalid
		}
		return nil
	}})
	if err != nil {
		t.Fatalf("run isolated Provider fixture: %v", err)
	}
	if result.Usage.Requests != 1 || !strings.Contains(string(result.Report), `"verdict":"PASS"`) {
		t.Fatalf("provider relay evidence is incomplete: usage=%+v report=%s", result.Usage, result.Report)
	}
	if inspected[0].HostConfig.NetworkMode != "none" {
		t.Fatalf("Provider fixture obtained a network plane: %+v", inspected[0].HostConfig)
	}
	if relayInspected[0].Config.User != "10001:10001" || relayInspected[0].HostConfig.NetworkMode != "none" || !relayInspected[0].HostConfig.ReadonlyRootfs || !contains(relayInspected[0].HostConfig.CapDrop, "ALL") || !contains(relayInspected[0].HostConfig.SecurityOpt, "no-new-privileges") {
		t.Fatalf("unsafe Provider relay sidecar configuration: %+v", relayInspected[0])
	}
	for key, value := range relayInspected[0].Config.Labels {
		lower := strings.ToLower(key + "=" + value)
		for _, forbidden := range []string{"token", "permit", "lease", "credential", "provider_url", "provider_secret"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("relay label exposed a control or secret fact: %q=%q", key, value)
			}
		}
	}
	foundRelay := false
	for _, mount := range inspected[0].Mounts {
		if mount.Destination == "/provider" {
			foundRelay = !mount.RW
		}
	}
	if !foundRelay {
		t.Fatalf("Reviewer did not receive only a read-only run-scoped Provider relay mount: %+v", inspected[0].Mounts)
	}
	afterCommit, err := commandOutput(ctx, "git", "--git-dir="+resultRoot.RepositoryRoot, "rev-parse", commit+"^{commit}")
	if err != nil || strings.TrimSpace(string(afterCommit)) != commit {
		t.Fatalf("Provider fixture changed frozen ResultCommit: got=%q err=%v", afterCommit, err)
	}
}

// TestDockerRunTimeoutFixtureSurvivesUntilDeadline proves that the fixed,
// immutable TIMEOUT fixture stays alive long enough for the Worker to own
// deadline enforcement. A fixture that exits by itself would turn a restart
// test into an ordinary process-failure test and never exercise reattachment.
func TestDockerRunTimeoutFixtureSurvivesUntilDeadline(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_TIMEOUT_DOCKER_IMAGE"))
	if image == "" {
		t.Skip("set REVIEW_RUNTIME_TIMEOUT_DOCKER_IMAGE to the exact TIMEOUT fixture digest")
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		t.Fatalf("REVIEW_RUNTIME_TIMEOUT_DOCKER_IMAGE must be an exact sha256 digest, got %q", image)
	}
	if _, err := commandOutput(context.Background(), "docker", "image", "inspect", image); err != nil {
		t.Skipf("exact Fake Reviewer image is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	workspace, err := attemptruntime.OpenPersistentWorkspace(filepath.Join(t.TempDir(), "worker-workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	const taskID, turnID, finalizationID, reviewRunID, workerID, ownerEpoch int64 = 9301, 9302, 9303, 9304, 9305, 1
	commit, tree, _ := installFrozenDockerResultWithIDs(t, ctx, workspace, taskID, turnID, finalizationID)
	runtime := NewDocker(config.WorkerConfig{})
	runtime.BindPersistentWorkspace(workspace)
	created := false
	_, err = runtime.Run(ctx, Work{ReviewRunID: reviewRunID, WorkerID: workerID, TaskID: taskID, TaskTurnID: turnID, FinalizationRunID: finalizationID, ResultCommit: commit, ResultTree: tree, ReviewerImage: image, OwnerEpoch: ownerEpoch, HardTimeout: time.Second, ReviewContract: []byte(`{"version":"independent-review/v1"}`), Requirements: []byte(`{"instruction":"timeout fixture"}`)}, Gate{ContainerCreated: func(container Container) error {
		body, inspectErr := commandOutput(ctx, "docker", "inspect", container.ID)
		if inspectErr != nil || len(body) == 0 {
			return ErrResultIdentityInvalid
		}
		created = true
		return nil
	}})
	if !created {
		t.Fatalf("TIMEOUT fixture never reached an isolated Docker container: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("TIMEOUT fixture error=%v, want worker-enforced context deadline", err)
	}
}

// TestDockerRunUsesFreshScratchForReplacementOwner proves that a transient
// ReviewRun retry cannot inherit the previous Reviewer's mutable clone. The
// same logical Run is intentionally executed twice with distinct fenced owner
// epochs; both deterministic containers must be able to clone into their own
// COW scratch roots.
func TestDockerRunUsesFreshScratchForReplacementOwner(t *testing.T) {
	image := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_DOCKER_IMAGE"))
	if image == "" {
		t.Skip("set REVIEW_RUNTIME_DOCKER_IMAGE to an exact local Fake Reviewer digest")
	}
	if !strings.HasPrefix(image, "sha256:") || len(image) != 71 {
		t.Fatalf("REVIEW_RUNTIME_DOCKER_IMAGE must be an exact sha256 digest, got %q", image)
	}
	if _, err := commandOutput(context.Background(), "docker", "image", "inspect", image); err != nil {
		t.Skipf("exact Fake Reviewer image is unavailable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	workspace, err := attemptruntime.OpenPersistentWorkspace(filepath.Join(t.TempDir(), "worker-workspaces"))
	if err != nil {
		t.Fatal(err)
	}
	const taskID, turnID, finalizationID, reviewRunID, workerID int64 = 9401, 9402, 9403, 9404, 9405
	commit, tree, _ := installFrozenDockerResultWithIDs(t, ctx, workspace, taskID, turnID, finalizationID)
	runtime := NewDocker(config.WorkerConfig{})
	runtime.BindPersistentWorkspace(workspace)
	for _, ownerEpoch := range []int64{1, 3} {
		result, runErr := runtime.Run(ctx, Work{ReviewRunID: reviewRunID, WorkerID: workerID, TaskID: taskID, TaskTurnID: turnID, FinalizationRunID: finalizationID, ResultCommit: commit, ResultTree: tree, ReviewerImage: image, OwnerEpoch: ownerEpoch, HardTimeout: 20 * time.Second, ReviewContract: []byte(`{"version":"independent-review/v1"}`), Requirements: []byte(`{"instruction":"replacement scratch fixture"}`)}, Gate{})
		if runErr != nil || !strings.Contains(string(result.Report), `"verdict":"PASS"`) {
			t.Fatalf("owner epoch %d isolated run: result=%+v err=%v", ownerEpoch, result, runErr)
		}
		attempt := assertReviewAttemptAudit(t, workspace, taskID, reviewRunID, ownerEpoch)
		if _, statErr := os.Stat(filepath.Join(attempt.Root, "scratch", "repo", ".git")); statErr != nil {
			t.Fatalf("owner epoch %d did not receive its own writable COW scratch: %v", ownerEpoch, statErr)
		}
	}
	first, err := workspace.ReviewAttempt(taskID, reviewRunID, 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := workspace.ReviewAttempt(taskID, reviewRunID, 3)
	if err != nil || first.Root == second.Root || first.InputPath == second.InputPath || first.JournalRoot == second.JournalRoot {
		t.Fatalf("replacement owner reused mutable review state: first=%+v second=%+v err=%v", first, second, err)
	}
	assertNoRunLevelMutableReviewState(t, workspace, taskID, reviewRunID)
}

// TestDockerRunFakeFixtureMatrix is the opt-in Slice 7 container matrix. The
// map is intentionally supplied as scenario -> exact image ID rather than a
// tag: a local tag is only a build convenience and must never become the
// Review Profile identity. This test exercises the runtime boundary for every
// fixed fixture; database state transitions remain covered by the ReviewRun
// integration suite, where the Server is the verdict authority.
func TestDockerRunFakeFixtureMatrix(t *testing.T) {
	fixturesJSON := strings.TrimSpace(os.Getenv("REVIEW_RUNTIME_FIXTURE_IMAGES"))
	if fixturesJSON == "" {
		t.Skip("set REVIEW_RUNTIME_FIXTURE_IMAGES to a JSON object of scenario -> exact sha256 image IDs")
	}
	var fixtures map[string]string
	if err := json.Unmarshal([]byte(fixturesJSON), &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatal("REVIEW_RUNTIME_FIXTURE_IMAGES must be a non-empty JSON object")
	}
	ordered := make([]string, 0, len(fixtures))
	for scenario, image := range fixtures {
		if !fixtureScenarioAllowed(scenario) {
			t.Fatalf("fixture scenario %q is not in the closed Slice 7 allowlist", scenario)
		}
		if !strings.HasPrefix(image, "sha256:") || len(image) != len("sha256:")+64 || !validFixtureHex(strings.TrimPrefix(image, "sha256:")) {
			t.Fatalf("fixture %q must use an exact sha256 image ID", scenario)
		}
		if _, err := commandOutput(context.Background(), "docker", "image", "inspect", image); err != nil {
			t.Fatalf("fixture %q image %q is unavailable: %v", scenario, image, err)
		}
		ordered = append(ordered, scenario)
	}
	sort.Strings(ordered)
	for index, scenario := range ordered {
		scenario, image := scenario, fixtures[scenario]
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), fixtureRuntimeTimeout(scenario))
			defer cancel()
			workspace, err := attemptruntime.OpenPersistentWorkspace(filepath.Join(t.TempDir(), "worker-workspaces"))
			if err != nil {
				t.Fatal(err)
			}
			base := int64(9600 + index*10)
			commit, tree, _ := installFrozenDockerResultWithIDs(t, ctx, workspace, base+1, base+2, base+3)
			runtime := NewDocker(config.WorkerConfig{})
			runtime.BindPersistentWorkspace(workspace)
			work := Work{
				ReviewRunID: base + 4, ReviewRunAttemptID: base + 5, WorkerID: base + 6,
				WorkerInstanceID: "slice7-matrix-worker", HostBootID: "slice7-matrix-boot",
				TaskID: base + 1, TaskTurnID: base + 2, FinalizationRunID: base + 3,
				ResultCommit: commit, ResultTree: tree, ReviewerImage: image, OwnerEpoch: 1,
				HardTimeout: fixtureRuntimeTimeout(scenario), ReviewContract: []byte(`{"version":"independent-review/v1"}`),
				Requirements: []byte(`{"instruction":"inspect the frozen matrix fixture"}`),
			}
			if scenario == "PROVIDER_PASS" {
				work.Provider = ProviderIdentity{CredentialID: 9901, CredentialVersion: 1, Protocol: "OPENAI", PrimaryModel: "slice7-fake-review", StructuredContract: "independent-reviewer/structured-output/v1"}
			}
			result, runErr := runtime.Run(ctx, work, Gate{})
			if wantDeadlineError(scenario) {
				if !errors.Is(runErr, context.DeadlineExceeded) {
					t.Fatalf("scenario=%s error=%v, want worker deadline", scenario, runErr)
				}
				t.Logf("scenario=%s outcome=worker-deadline", scenario)
				return
			}
			if wantContainerFailure(scenario) {
				if runErr == nil {
					t.Fatalf("scenario=%s unexpectedly succeeded", scenario)
				}
				t.Logf("scenario=%s outcome=transient-container-failure", scenario)
				return
			}
			if wantReportMissing(scenario) {
				if !errors.Is(runErr, ErrReportMissing) {
					t.Fatalf("scenario=%s error=%v, want ErrReportMissing", scenario, runErr)
				}
				t.Logf("scenario=%s outcome=report-missing", scenario)
				return
			}
			if runErr != nil {
				t.Fatalf("scenario=%s runtime error=%v", scenario, runErr)
			}
			if len(result.Report) == 0 {
				t.Fatalf("scenario=%s produced no candidate report", scenario)
			}
			if scenario == "PASS" || scenario == "PROVIDER_PASS" {
				if !bytes.Contains(result.Report, []byte(`"verdict":"PASS"`)) {
					t.Fatalf("scenario=%s did not produce expected PASS candidate", scenario)
				}
			}
			if scenario == "REVISE" || scenario == "CORRECTION_CYCLE" || scenario == "FINDING_IMPROVEMENT" || scenario == "TEST_IMPROVEMENT" || scenario == "NO_PROGRESS" || scenario == "FORMATTING_ONLY_CHANGE" {
				if !bytes.Contains(result.Report, []byte(`"verdict":"REVISE"`)) {
					t.Fatalf("scenario=%s did not produce expected REVISE candidate", scenario)
				}
			}
			if scenario == "REJECT" && !bytes.Contains(result.Report, []byte(`"verdict":"REJECT"`)) {
				t.Fatalf("scenario=%s did not produce expected REJECT candidate", scenario)
			}
			t.Logf("scenario=%s outcome=candidate-report bytes=%d", scenario, len(result.Report))
		})
	}
}

func fixtureScenarioAllowed(scenario string) bool {
	switch scenario {
	case "PASS", "REVISE", "REJECT", "INVALID", "MALFORMED_JSON", "INVALID_ENUM", "OVERSIZED_REPORT", "PASS_WITH_BLOCKING_FINDING", "PATH_TRAVERSAL", "SYMLINK_ESCAPE", "INVALID_LINE_RANGE", "UNABLE_TO_REPRODUCE_PASS", "TIMEOUT", "CONTAINER_TIMEOUT", "RESPONSE_LOSS", "PROVIDER_TIMEOUT", "PROVIDER_RESPONSE_LOSS", "CONTAINER_EXIT", "PROVIDER_PASS", "CORRECTION_CYCLE", "FINDING_IMPROVEMENT", "TEST_IMPROVEMENT", "NO_PROGRESS", "FORMATTING_ONLY_CHANGE":
		return true
	default:
		return false
	}
}

func validFixtureHex(value string) bool {
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return len(value) == 64
}

func fixtureRuntimeTimeout(scenario string) time.Duration {
	if wantDeadlineError(scenario) {
		return 2 * time.Second
	}
	return 30 * time.Second
}

func wantDeadlineError(scenario string) bool {
	return scenario == "TIMEOUT" || scenario == "CONTAINER_TIMEOUT" || scenario == "PROVIDER_TIMEOUT"
}

func wantReportMissing(scenario string) bool {
	return scenario == "OVERSIZED_REPORT" || scenario == "RESPONSE_LOSS" || scenario == "PROVIDER_RESPONSE_LOSS"
}

func wantContainerFailure(scenario string) bool {
	return scenario == "CONTAINER_EXIT"
}

type dockerIsolationInspect []struct {
	Config struct {
		User   string            `json:"User"`
		Env    []string          `json:"Env"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		NetworkMode    string   `json:"NetworkMode"`
		ReadonlyRootfs bool     `json:"ReadonlyRootfs"`
		CapDrop        []string `json:"CapDrop"`
		SecurityOpt    []string `json:"SecurityOpt"`
		PidsLimit      int64    `json:"PidsLimit"`
		Memory         int64    `json:"Memory"`
		NanoCPUs       int64    `json:"NanoCpus"`
	} `json:"HostConfig"`
	Mounts []struct {
		Source      string `json:"Source"`
		Destination string `json:"Destination"`
		RW          bool   `json:"RW"`
	} `json:"Mounts"`
}

func assertDockerIsolation(t *testing.T, inspected dockerIsolationInspect, result attemptruntime.ResultWorkspace, workspace *attemptruntime.PersistentWorkspace, taskID, reviewRunID, ownerEpoch int64) {
	t.Helper()
	if len(inspected) != 1 {
		t.Fatalf("unexpected Docker inspect shape: %+v", inspected)
	}
	config := inspected[0]
	if config.Config.User != "10001:10001" || config.HostConfig.NetworkMode != "none" || !config.HostConfig.ReadonlyRootfs || config.HostConfig.PidsLimit != 128 || config.HostConfig.Memory != 512<<20 || config.HostConfig.NanoCPUs != 1_000_000_000 {
		t.Fatalf("unsafe Docker isolation config: %+v", config)
	}
	if !contains(config.HostConfig.CapDrop, "ALL") || !contains(config.HostConfig.SecurityOpt, "no-new-privileges") {
		t.Fatalf("Reviewer lacks capability/no-new-privileges isolation: %+v", config.HostConfig)
	}
	for _, env := range config.Config.Env {
		lower := strings.ToLower(env)
		for _, forbidden := range []string{"worker_token", "provider", "git_credential", "coding_session", "docker_host"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("Reviewer received forbidden environment fact %q", env)
			}
		}
	}
	review := assertReviewAttemptAudit(t, workspace, taskID, reviewRunID, ownerEpoch)
	assertNoRunLevelMutableReviewState(t, workspace, taskID, reviewRunID)
	expected := map[string]bool{
		"/result/repository.git": false,
		"/scratch":               true,
		"/review/input.json":     false,
		"/report":                true,
	}
	if len(config.Mounts) != len(expected) {
		t.Fatalf("unexpected Reviewer mount count: %+v", config.Mounts)
	}
	for _, mount := range config.Mounts {
		wantRW, ok := expected[mount.Destination]
		if !ok || mount.RW != wantRW {
			t.Fatalf("unexpected Reviewer mount: %+v", mount)
		}
		if strings.Contains(mount.Source, "/var/run/docker.sock") || strings.Contains(mount.Source, "/input/review/") {
			t.Fatalf("Reviewer mount exposed prohibited host scope: %+v", mount)
		}
		switch mount.Destination {
		case "/result/repository.git":
			if mount.Source != result.RepositoryRoot {
				t.Fatalf("Reviewer result mount escaped the frozen repository: %+v", mount)
			}
		case "/scratch":
			if mount.Source != filepath.Join(review.Root, "scratch") {
				t.Fatalf("Reviewer scratch did not use the fenced owner-attempt root: %+v", mount)
			}
		case "/review/input.json":
			if mount.Source != review.InputPath {
				t.Fatalf("Reviewer input did not use the fenced owner-attempt contract: %+v", mount)
			}
		case "/report":
			if strings.HasPrefix(mount.Source, workspace.Root()+string(os.PathSeparator)) {
				t.Fatalf("ephemeral report output used the persistent Task workspace: %+v", mount)
			}
		}
	}
	if result.RepositoryRoot == "" || review.Root == "" { // keep exact host paths used above meaningful to the test.
		t.Fatal("test fixture did not materialize result/review roots")
	}
}

func assertReviewAttemptAudit(t *testing.T, workspace *attemptruntime.PersistentWorkspace, taskID, reviewRunID, ownerEpoch int64) attemptruntime.ReviewWorkspace {
	t.Helper()
	attempt, err := workspace.ReviewAttempt(taskID, reviewRunID, ownerEpoch)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claimed.json", "container-created.json"} {
		journal, readErr := os.ReadFile(filepath.Join(attempt.JournalRoot, name))
		lower := strings.ToLower(string(journal))
		if readErr != nil || !strings.Contains(string(journal), strconv.FormatInt(reviewRunID, 10)) {
			t.Fatalf("missing owner-attempt review journal %s: err=%v body=%q", name, readErr, journal)
		}
		for _, forbidden := range []string{"lease", "permit", "worker_token", "provider_secret"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("owner-attempt review journal %s exposed %q: body=%q", name, forbidden, journal)
			}
		}
	}
	return attempt
}

func assertNoRunLevelMutableReviewState(t *testing.T, workspace *attemptruntime.PersistentWorkspace, taskID, reviewRunID int64) {
	t.Helper()
	review, err := workspace.Review(taskID, reviewRunID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(review.Root, "scratch"),
		filepath.Join(review.JournalRoot, "claimed.json"),
		filepath.Join(review.JournalRoot, "container-created.json"),
	} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("run-level mutable review state exists at %q: %v", path, statErr)
		}
	}
}

func installFrozenDockerResult(t *testing.T, ctx context.Context, workspace *attemptruntime.PersistentWorkspace, taskID, turnID, finalizationID int64) (string, string, attemptruntime.ResultWorkspace) {
	return installFrozenDockerResultWithIDs(t, ctx, workspace, taskID, turnID, finalizationID)
}

func installFrozenDockerResultWithIDs(t *testing.T, ctx context.Context, workspace *attemptruntime.PersistentWorkspace, taskID, turnID, finalizationID int64) (string, string, attemptruntime.ResultWorkspace) {
	t.Helper()
	result, err := workspace.Result(taskID, turnID, finalizationID)
	if err != nil {
		t.Fatal(err)
	}
	if err = workspace.PrepareResultParent(result); err != nil {
		t.Fatal(err)
	}
	checkout := filepath.Join(t.TempDir(), "source")
	for _, args := range [][]string{{"init", checkout}, {"-C", checkout, "config", "user.email", "reviewer@example.invalid"}, {"-C", checkout, "config", "user.name", "Reviewer"}} {
		if _, err = commandOutput(ctx, "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range map[string]string{
		"README.md":              "isolated review fixture\n",
		"SEVEN_WORKER_RESULT.md": "frozen result evidence\n",
	} {
		if err = os.WriteFile(filepath.Join(checkout, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Symlink("outside-result", filepath.Join(checkout, "SEVEN_WORKER_RESULT_LINK.md")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"-C", checkout, "add", "README.md", "SEVEN_WORKER_RESULT.md", "SEVEN_WORKER_RESULT_LINK.md"}, {"-C", checkout, "commit", "-m", "frozen result"}, {"clone", "--bare", checkout, result.RepositoryRoot}} {
		if _, err = commandOutput(ctx, "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	commitRaw, err := commandOutput(ctx, "git", "--git-dir="+result.RepositoryRoot, "rev-parse", "HEAD^{commit}")
	if err != nil {
		t.Fatal(err)
	}
	treeRaw, err := commandOutput(ctx, "git", "--git-dir="+result.RepositoryRoot, "rev-parse", "HEAD^{tree}")
	if err != nil {
		t.Fatal(err)
	}
	commit, tree := strings.TrimSpace(string(commitRaw)), strings.TrimSpace(string(treeRaw))
	manifest, err := json.Marshal(map[string]any{"schemaVersion": 1, "taskId": strconv.FormatInt(taskID, 10), "taskTurnId": strconv.FormatInt(turnID, 10), "finalizationRunId": strconv.FormatInt(finalizationID, 10), "resultCommit": commit, "resultTree": tree})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(result.ManifestPath, manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	ready, err := json.Marshal(map[string]any{"schemaVersion": 1, "manifest": "result-manifest.json", "manifestDigest": digest(manifest)})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(result.ReadyPath, ready, 0o600); err != nil {
		t.Fatal(err)
	}
	return commit, tree, result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
