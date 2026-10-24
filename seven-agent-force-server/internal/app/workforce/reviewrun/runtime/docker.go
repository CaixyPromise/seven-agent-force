// Package runtime owns the mechanical, isolated Reviewer container boundary.
// It deliberately has no database, RabbitMQ or Task transition dependency.
package runtime

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	attemptruntime "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	reviewdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	config "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/worker/resourcejournal"
)

const maxRuntimeReportBytes = 256 << 10

var (
	ErrResultIdentityInvalid = errors.New("REVIEW_RESULT_IDENTITY_INVALID")
	ErrReportMissing         = errors.New("REVIEW_REPORT_MISSING")
	ErrReviewInputInvalid    = errors.New("REVIEW_INPUT_INVALID")
)

type Docker struct {
	cleanupJournal *resourcejournal.Journal
	cleanupCommand resourcejournal.Command
	cfg            config.WorkerConfig
	providerProxy  config.ProviderProxyConfig
	workspace      *attemptruntime.PersistentWorkspace
	exec           func(context.Context, string, ...string) ([]byte, error)
	execInput      func(context.Context, []byte, string, ...string) ([]byte, error)
}

func NewDocker(cfg config.WorkerConfig, providerProxy ...config.ProviderProxyConfig) *Docker {
	var proxy config.ProviderProxyConfig
	if len(providerProxy) > 0 {
		proxy = providerProxy[0]
	}
	return &Docker{cfg: cfg, providerProxy: proxy, exec: commandOutput, execInput: commandInputOutput}
}

func (d *Docker) BindPersistentWorkspace(workspace *attemptruntime.PersistentWorkspace) {
	if d != nil {
		d.workspace = workspace
	}
}

type Work struct {
	ReviewRunID        int64
	ReviewRunAttemptID int64
	WorkerID           int64
	WorkerInstanceID   string
	HostBootID         string
	TaskID             int64
	TaskTurnID         int64
	FinalizationRunID  int64
	ResultCommit       string
	ResultTree         string
	ReviewerImage      string
	OwnerEpoch         int64
	HardTimeout        time.Duration
	// ExecutionDeadline is issued by the Server once per fenced ReviewRun
	// Attempt. A same-owner restart reuses it; a replacement owner receives a
	// fresh deadline only after the previous Attempt has become retryable.
	ExecutionDeadline time.Time
	// LeaseExpiresAt is the current Server-issued ReviewRun lease. It is kept
	// separate from ExecutionDeadline because a Worker restart must rebind the
	// existing relay to a fresh lease without changing the execution deadline.
	LeaseExpiresAt time.Time
	LeaseToken     string
	ReviewRoot     string
	ReviewContract []byte
	Requirements   []byte
	Provider       ProviderIdentity
	// ProviderProxyImageDigest is part of the frozen task execution snapshot.
	// A configured ReviewRun relay must use this exact admitted image; the
	// Worker-local configuration is not allowed to silently replace it.
	ProviderProxyImageDigest string
}

type Container struct {
	ID                      string
	Name                    string
	ExecutionIdentityDigest string
}

type Result struct {
	Container Container
	Report    []byte
	Usage     reviewdomain.UsageFacts
}

// ManagedContainer is the label-only evidence discovered on a local Docker
// daemon during Worker restart. It intentionally has no lease, permit,
// provider authority, socket path, or Task workspace location.
type ManagedContainer struct {
	Container
	ReviewRunID int64
	WorkerID    int64
	OwnerEpoch  int64
	// ReportVolume is the one opaque, Docker-internal output volume mounted at
	// /report. It is recovered only after checking both the Reviewer labels and
	// the volume labels; it is never derived from a predictable Run name.
	ReportVolume string
	Running      bool
	State        string
	ImageDigest  string
}

type Gate struct {
	ContainerCreated func(Container) error
}

func (d *Docker) Run(ctx context.Context, work Work, gate Gate) (Result, error) {
	if d == nil || d.workspace == nil || !validWork(work) {
		return Result{}, ErrResultIdentityInvalid
	}
	if err := validateReviewInputDocuments(work.ReviewContract, work.Requirements); err != nil {
		return Result{}, err
	}
	if work.HardTimeout <= 0 {
		work.HardTimeout = 60 * time.Minute
	}
	resultRoot, err := d.workspace.Result(work.TaskID, work.TaskTurnID, work.FinalizationRunID)
	if err != nil {
		return Result{}, err
	}
	if err = verifyFrozenResult(ctx, resultRoot, work); err != nil {
		return Result{}, err
	}
	review, err := d.workspace.PrepareReviewAttempt(work.TaskID, work.ReviewRunID, work.OwnerEpoch)
	if err != nil {
		return Result{}, err
	}
	referenceRoot := filepath.Join(review.Root, "references")
	hasReferences, err := materializeReviewReferences(review.TaskRoot, referenceRoot, work.Requirements)
	if err != nil {
		return Result{}, err
	}
	// The configured review relay keeps its short-lived lease/spec files next
	// to this exact owner-attempt envelope. Never derive it from the run-level
	// directory: a replacement owner must receive a new writable authority
	// boundary.
	work.ReviewRoot = review.Root
	scratch := filepath.Join(review.Root, "scratch")
	if err = os.MkdirAll(scratch, 0o777); err != nil {
		return Result{}, err
	}
	inputDocument := map[string]any{
		"schemaVersion":  1,
		"reviewRunId":    strconv.FormatInt(work.ReviewRunID, 10),
		"taskId":         strconv.FormatInt(work.TaskID, 10),
		"resultCommit":   work.ResultCommit,
		"resultTree":     work.ResultTree,
		"reviewContract": json.RawMessage(work.ReviewContract),
		"requirements":   json.RawMessage(work.Requirements),
	}
	if work.Provider.Enabled() {
		inputDocument["provider"] = map[string]any{
			"credentialId":       strconv.FormatInt(work.Provider.CredentialID, 10),
			"credentialVersion":  strconv.FormatInt(work.Provider.CredentialVersion, 10),
			"protocol":           work.Provider.Protocol,
			"model":              work.Provider.PrimaryModel,
			"structuredContract": work.Provider.StructuredContract,
		}
	}
	input, err := json.Marshal(inputDocument)
	if err != nil {
		return Result{}, ErrReviewInputInvalid
	}
	if err = d.workspace.WriteAtomic(review.InputPath, input, 0o600); err != nil {
		return Result{}, err
	}
	identity := executionIdentity(work)
	if err = d.writeJournal(review, "claimed.json", map[string]any{
		"schemaVersion": 1, "reviewRunId": strconv.FormatInt(work.ReviewRunID, 10), "workerId": strconv.FormatInt(work.WorkerID, 10),
		"ownerEpoch": strconv.FormatInt(work.OwnerEpoch, 10), "resultCommit": work.ResultCommit, "resultTree": work.ResultTree,
		"reviewerImageDigest": work.ReviewerImage, "executionIdentityDigest": identity,
	}); err != nil {
		return Result{}, err
	}
	reportOutput, err := d.startReportOutput(ctx, work)
	if err != nil {
		return Result{}, err
	}
	reportOutputOwned := true
	defer func() {
		if reportOutputOwned {
			_ = d.stopReportOutput(reportOutput)
		}
	}()
	var relay *providerRelayContainer
	if work.Provider.Enabled() {
		relay, err = d.startProviderRelay(ctx, work)
		if err != nil {
			return Result{}, err
		}
		defer d.stopProviderRelay(relay)
	}
	name := "seven-review-" + strconv.FormatInt(work.ReviewRunID, 10) + "-" + strconv.FormatInt(work.OwnerEpoch, 10)
	args := []string{"create", "--name", name, "--network", "none", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--pids-limit", "128", "--memory", "512m", "--cpus", "1.0", "--user", "10001:10001", "--tmpfs", "/tmp:rw,noexec,nosuid,size=64m",
		"--label", "seven.managed=true", "--label", "seven.review.run.id=" + strconv.FormatInt(work.ReviewRunID, 10), "--label", "seven.review.worker.id=" + strconv.FormatInt(work.WorkerID, 10), "--label", "seven.review.owner.epoch=" + strconv.FormatInt(work.OwnerEpoch, 10), "--label", "seven.review.image.digest=" + work.ReviewerImage, "--label", "seven.review.execution.identity=" + identity,
		"--mount", "type=bind,src=" + resultRoot.RepositoryRoot + ",dst=/result/repository.git,readonly",
		"--mount", "type=bind,src=" + scratch + ",dst=/scratch",
		"--mount", "type=bind,src=" + review.InputPath + ",dst=/review/input.json,readonly",
		"--mount", "type=volume,src=" + reportOutput.name + ",dst=/report",
	}
	if relay != nil {
		args = append(args, "--mount", "type=volume,src="+relay.volume+",dst=/provider,readonly")
	}
	if hasReferences {
		args = append(args, "--mount", "type=bind,src="+referenceRoot+",dst=/review/references,readonly")
	}
	args = append(args, work.ReviewerImage, "review")
	created, err := d.exec(ctx, "docker", args...)
	if err != nil {
		return Result{}, fmt.Errorf("create isolated Reviewer: %w", err)
	}
	container := Container{ID: strings.TrimSpace(string(created)), Name: name, ExecutionIdentityDigest: identity}
	if len(container.ID) < 12 {
		return Result{}, errors.New("REVIEW_CONTAINER_ID_INVALID")
	}
	containerOwned := true
	defer func() {
		if containerOwned {
			_ = d.StopAndRemove(context.Background(), container.ID)
		}
	}()
	if err = d.writeJournal(review, "container-created.json", map[string]any{
		"schemaVersion": 1, "reviewRunId": strconv.FormatInt(work.ReviewRunID, 10), "containerId": container.ID,
		"containerName": container.Name, "reviewerImageDigest": work.ReviewerImage, "executionIdentityDigest": identity,
	}); err != nil {
		return Result{}, err
	}
	if gate.ContainerCreated != nil {
		if err = gate.ContainerCreated(container); err != nil {
			return Result{}, err
		}
	}
	runCtx, cancel := executionContext(ctx, work)
	defer cancel()
	started, startErr := d.exec(runCtx, "docker", "start", "-a", container.ID)
	if startErr != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return Result{}, context.DeadlineExceeded
		}
		return Result{}, wrapContainerStartFailure("run isolated Reviewer", started, startErr)
	}
	if err = verifyFrozenResult(ctx, resultRoot, work); err != nil {
		return Result{}, err
	}
	report, err := d.readContainerReport(ctx, container.ID)
	if err != nil {
		return Result{}, ErrReportMissing
	}
	// The container is untrusted.  A Worker reopens each evidence path from the
	// frozen bare repository and checks the concrete line boundary before the
	// Server receives a candidate report.  The Server canonicalizes it again.
	// On a containment/line mismatch we intentionally submit only a minimal
	// invalid candidate; the raw Reviewer document is not persisted or logged
	// and the Server converts the occurrence to MANUAL_REVIEW.
	if validateReportEvidence(ctx, resultRoot.RepositoryRoot, work.ResultCommit, report) != nil {
		report = []byte(`{"schemaVersion":"review-report/v1"}`)
	}
	result := Result{Container: container, Report: report, Usage: d.providerUsage(ctx, relay)}
	// Cleanup is a local resource fact and must not erase the generated report.
	_ = d.StopAndRemove(ctx, container.ID)
	containerOwned = false
	reportOutputOwned = false
	return result, nil
}

func validWork(work Work) bool {
	return work.ReviewRunID > 0 && work.WorkerID > 0 && work.TaskID > 0 && work.TaskTurnID > 0 && work.FinalizationRunID > 0 && work.OwnerEpoch > 0 && strings.TrimSpace(work.ResultCommit) != "" && strings.TrimSpace(work.ResultTree) != "" && validExactDockerDigest(work.ReviewerImage)
}

func executionContext(ctx context.Context, work Work) (context.Context, context.CancelFunc) {
	deadline := work.ExecutionDeadline
	if deadline.IsZero() {
		// Production claims carry a durable Server deadline. This fallback keeps
		// local unit fixtures compatible without changing the production rule.
		deadline = time.Now().UTC().Add(work.HardTimeout)
	}
	return context.WithDeadline(ctx, deadline)
}

// ManagedContainers scans only explicitly labelled review containers and
// parses their labels defensively. A malformed label is returned as a local
// evidence record that the Worker will stop; it is never used to construct a
// Server ReviewRun identity.
func (d *Docker) ManagedContainers(ctx context.Context) ([]ManagedContainer, error) {
	ids, err := d.exec(ctx, "docker", "ps", "-aq", "--filter", "label=seven.managed=true", "--filter", "label=seven.review.run.id")
	if err != nil {
		return nil, err
	}
	result := make([]ManagedContainer, 0)
	for _, id := range strings.Fields(string(ids)) {
		body, inspectErr := d.exec(ctx, "docker", "inspect", id)
		if inspectErr != nil {
			return nil, inspectErr
		}
		var inspected []struct {
			ID     string `json:"Id"`
			Name   string `json:"Name"`
			Config struct {
				Labels map[string]string `json:"Labels"`
			} `json:"Config"`
			State struct {
				Running bool   `json:"Running"`
				Status  string `json:"Status"`
			} `json:"State"`
			Mounts []struct {
				Type        string `json:"Type"`
				Name        string `json:"Name"`
				Destination string `json:"Destination"`
			} `json:"Mounts"`
		}
		if json.Unmarshal(body, &inspected) != nil || len(inspected) != 1 {
			return nil, ErrResultIdentityInvalid
		}
		labels := inspected[0].Config.Labels
		runID, runErr := strconv.ParseInt(labels["seven.review.run.id"], 10, 64)
		workerID, workerErr := strconv.ParseInt(labels["seven.review.worker.id"], 10, 64)
		epoch, epochErr := strconv.ParseInt(labels["seven.review.owner.epoch"], 10, 64)
		identity := labels["seven.review.execution.identity"]
		image := labels["seven.review.image.digest"]
		if runErr != nil || workerErr != nil || epochErr != nil || runID < 1 || workerID < 1 || epoch < 1 || len(identity) != 64 || !validExactDockerDigest(image) {
			return nil, ErrResultIdentityInvalid
		}
		work := Work{ReviewRunID: runID, OwnerEpoch: epoch}
		reportVolume := ""
		for _, mount := range inspected[0].Mounts {
			if mount.Destination != "/report" {
				continue
			}
			if mount.Type != "volume" || mount.Name == "" || reportVolume != "" {
				return nil, ErrResultIdentityInvalid
			}
			volume, volumeErr := d.exec(ctx, "docker", "volume", "inspect", mount.Name)
			if volumeErr != nil || !reportVolumeMatches(volume, work, mount.Name) {
				return nil, ErrResultIdentityInvalid
			}
			reportVolume = mount.Name
		}
		if reportVolume == "" {
			return nil, ErrResultIdentityInvalid
		}
		result = append(result, ManagedContainer{Container: Container{ID: inspected[0].ID, Name: strings.TrimPrefix(inspected[0].Name, "/"), ExecutionIdentityDigest: identity}, ReviewRunID: runID, WorkerID: workerID, OwnerEpoch: epoch, ReportVolume: reportVolume, Running: inspected[0].State.Running, State: inspected[0].State.Status, ImageDigest: image})
	}
	return result, nil
}

// Reattach waits for a previously registered review container and rereads the
// candidate report. The exact frozen result identity is verified both before
// and after the container finishes, just as in a new launch.
func (d *Docker) Reattach(ctx context.Context, work Work, container ManagedContainer) (Result, error) {
	if d == nil || d.workspace == nil || !validWork(work) || container.ReviewRunID != work.ReviewRunID || container.WorkerID != work.WorkerID || container.OwnerEpoch != work.OwnerEpoch || container.ImageDigest != work.ReviewerImage {
		return Result{}, ErrResultIdentityInvalid
	}
	// A discovered container is untrusted until every frozen input document and
	// identity check succeeds. Install the cleanup fence before validating the
	// documents so malformed persisted input cannot leave a live Reviewer
	// behind after a Worker restart.
	containerOwned := true
	defer func() {
		if containerOwned {
			_ = d.StopAndRemove(context.Background(), container.ID)
		}
	}()
	if err := validateReviewInputDocuments(work.ReviewContract, work.Requirements); err != nil {
		return Result{}, err
	}
	if work.HardTimeout <= 0 {
		work.HardTimeout = 60 * time.Minute
	}
	resultRoot, err := d.workspace.Result(work.TaskID, work.TaskTurnID, work.FinalizationRunID)
	if err != nil || verifyFrozenResult(ctx, resultRoot, work) != nil {
		return Result{}, ErrResultIdentityInvalid
	}
	review, err := d.workspace.ReviewAttempt(work.TaskID, work.ReviewRunID, work.OwnerEpoch)
	if err != nil {
		return Result{}, ErrResultIdentityInvalid
	}
	referenceRoot := filepath.Join(review.Root, "references")
	hasReferences, err := materializeReviewReferences(review.TaskRoot, referenceRoot, work.Requirements)
	if err != nil {
		return Result{}, err
	}
	if err = d.verifyReferenceMount(ctx, container.ID, referenceRoot, hasReferences); err != nil {
		return Result{}, err
	}
	work.ReviewRoot = review.Root
	// From this point forward a recovery failure is itself authority to stop
	// the locally discovered container. In particular, a missing report volume
	// or relay must not leave a live Reviewer behind until the next restart.
	reportOutput, err := d.recoverReportOutput(ctx, work, container.ReportVolume)
	if err != nil {
		return Result{}, err
	}
	reportOutputOwned := true
	defer func() {
		if reportOutputOwned {
			_ = d.stopReportOutput(reportOutput)
		}
	}()
	var relay *providerRelayContainer
	if work.Provider.Enabled() {
		relay, err = d.recoverProviderRelay(ctx, work)
		if err != nil {
			return Result{}, err
		}
		defer d.stopProviderRelay(relay)
	}
	if container.Running {
		runCtx, cancel := executionContext(ctx, work)
		defer cancel()
		if _, err = d.exec(runCtx, "docker", "wait", container.ID); err != nil {
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				return Result{}, context.DeadlineExceeded
			}
			return Result{}, fmt.Errorf("reattach isolated Reviewer: %w", err)
		}
	} else if container.State == "created" {
		runCtx, cancel := executionContext(ctx, work)
		defer cancel()
		started, startErr := d.exec(runCtx, "docker", "start", "-a", container.ID)
		if startErr != nil {
			if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
				return Result{}, context.DeadlineExceeded
			}
			return Result{}, wrapContainerStartFailure("resume created isolated Reviewer", started, startErr)
		}
	} else if container.State != "exited" {
		return Result{}, ErrResultIdentityInvalid
	}
	if err = verifyFrozenResult(ctx, resultRoot, work); err != nil {
		return Result{}, err
	}
	report, err := d.readContainerReport(ctx, container.ID)
	if err != nil {
		return Result{}, ErrReportMissing
	}
	if validateReportEvidence(ctx, resultRoot.RepositoryRoot, work.ResultCommit, report) != nil {
		report = []byte(`{"schemaVersion":"review-report/v1"}`)
	}
	result := Result{Container: container.Container, Report: report, Usage: d.providerUsage(ctx, relay)}
	_ = d.StopAndRemove(ctx, container.ID)
	containerOwned = false
	reportOutputOwned = false
	return result, nil
}

// wrapContainerStartFailure preserves only an allowlisted machine reason from
// a Reviewer container's bounded combined output. Docker output is untrusted
// and may contain a Provider response, shell output, or other sensitive data;
// none of that is allowed to cross the runtime boundary or enter ReviewRun
// error facts.
func wrapContainerStartFailure(prefix string, output []byte, cause error) error {
	if code := safeRuntimeReasonCode(output); code != "" {
		return fmt.Errorf("%s: review provider relay rejected: %s", prefix, code)
	}
	return fmt.Errorf("%s: %w", prefix, cause)
}

func safeRuntimeReasonCode(output []byte) string {
	const marker = "review provider relay rejected:"
	message := string(output)
	index := strings.Index(message, marker)
	if index < 0 {
		return ""
	}
	value := strings.TrimSpace(message[index+len(marker):])
	if value == "" || len(value) > 128 {
		return ""
	}
	for _, character := range value {
		if !(character == '_' || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9')) {
			return ""
		}
	}
	if strings.HasPrefix(value, "PROVIDER_") || strings.HasPrefix(value, "REVIEW_") || strings.HasPrefix(value, "INVOCATION_") {
		return value
	}
	return ""
}

func (d *Docker) writeJournal(review attemptruntime.ReviewWorkspace, name string, value any) error {
	if d == nil || d.workspace == nil || name == "" {
		return ErrResultIdentityInvalid
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return d.workspace.WriteAtomic(filepath.Join(review.JournalRoot, name), body, 0o600)
}

func (d *Docker) StopAndRemove(ctx context.Context, containerID string) error {
	if d == nil || strings.TrimSpace(containerID) == "" {
		return ErrResultIdentityInvalid
	}
	ctx, cancel := context.WithTimeout(context.Background(), resourcejournal.Timeout)
	defer cancel()
	if d.cleanupJournal != nil {
		return d.cleanupReviewer(ctx, containerID)
	}
	// A cancellation or stale-owner fence may be the only process that gets to
	// tear down a stopped Reviewer. Best-effort cleanup of its labelled report
	// volume prevents a raw candidate from surviving without its container.
	volume := ""
	volumeWork := Work{}
	if body, inspectErr := d.exec(ctx, "docker", "inspect", containerID); inspectErr == nil {
		if candidate, work := reportVolumeFromInspect(body); candidate != "" {
			volume, volumeWork = candidate, work
		}
	}
	_, err := d.exec(ctx, "docker", "rm", "-f", containerID)
	if volume != "" {
		if body, inspectErr := d.exec(ctx, "docker", "volume", "inspect", volume); inspectErr == nil && reportVolumeMatches(body, volumeWork, volume) {
			if cleanupErr := d.stopReportOutput(&reportOutputVolume{name: volume}); err == nil {
				err = cleanupErr
			}
		}
	}
	return err
}

func verifyFrozenResult(ctx context.Context, root attemptruntime.ResultWorkspace, work Work) error {
	ready, err := os.ReadFile(root.ReadyPath)
	if err != nil || len(ready) == 0 {
		return ErrResultIdentityInvalid
	}
	manifest, err := os.ReadFile(root.ManifestPath)
	if err != nil {
		return ErrResultIdentityInvalid
	}
	var readyDoc struct {
		Manifest       string `json:"manifest"`
		ManifestDigest string `json:"manifestDigest"`
	}
	if json.Unmarshal(ready, &readyDoc) != nil || readyDoc.Manifest != "result-manifest.json" || digest(manifest) != readyDoc.ManifestDigest {
		return ErrResultIdentityInvalid
	}
	var manifestDoc struct {
		TaskID            int64  `json:"taskId,string"`
		TaskTurnID        int64  `json:"taskTurnId,string"`
		FinalizationRunID int64  `json:"finalizationRunId,string"`
		ResultCommit      string `json:"resultCommit"`
		ResultTree        string `json:"resultTree"`
	}
	if json.Unmarshal(manifest, &manifestDoc) != nil || manifestDoc.TaskID != work.TaskID || manifestDoc.TaskTurnID != work.TaskTurnID || manifestDoc.FinalizationRunID != work.FinalizationRunID || manifestDoc.ResultCommit != work.ResultCommit || manifestDoc.ResultTree != work.ResultTree {
		return ErrResultIdentityInvalid
	}
	commit, err := commandOutput(ctx, "git", "--git-dir="+root.RepositoryRoot, "rev-parse", work.ResultCommit+"^{commit}")
	if err != nil || strings.TrimSpace(string(commit)) != work.ResultCommit {
		return ErrResultIdentityInvalid
	}
	tree, err := commandOutput(ctx, "git", "--git-dir="+root.RepositoryRoot, "rev-parse", work.ResultCommit+"^{tree}")
	if err != nil || strings.TrimSpace(string(tree)) != work.ResultTree {
		return ErrResultIdentityInvalid
	}
	return nil
}

func commandOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}

func commandInputOutput(ctx context.Context, input []byte, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdin = bytes.NewReader(input)
	return command.CombinedOutput()
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := io.LimitReader(file, limit+1)
	value, err := io.ReadAll(reader)
	if err != nil || int64(len(value)) > limit {
		return nil, errors.New("REVIEW_REPORT_TOO_LARGE")
	}
	return value, nil
}

// readContainerReport copies an untrusted candidate only after the isolated
// container has stopped. The file lives in the container's per-Run tmpfs, then
// in a fresh OS temp directory for the duration of this call, and is removed
// before returning. It is never written to the seven-day Worker workspace,
// journals, logs or database; SubmitReport is the sole canonical persistence
// boundary.
func (d *Docker) readContainerReport(ctx context.Context, containerID string) ([]byte, error) {
	if d == nil || strings.TrimSpace(containerID) == "" {
		return nil, ErrReportMissing
	}
	temporary, err := os.MkdirTemp("", "seven-review-candidate-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(temporary)
	reportPath := filepath.Join(temporary, "review-report.json")
	if _, err = d.exec(ctx, "docker", "cp", containerID+":/report/review-report.json", reportPath); err != nil {
		return nil, ErrReportMissing
	}
	return readBounded(reportPath, maxRuntimeReportBytes)
}

// reportOutputVolume is Docker-internal and per ReviewRun owner epoch. It is
// intentionally not a bind mount into the seven-day Worker workspace or a
// host temporary path. A one-shot root init grants the non-root Reviewer write
// access; the long-lived Reviewer itself keeps all capabilities dropped.
type reportOutputVolume struct{ name string }

func (d *Docker) startReportOutput(ctx context.Context, work Work) (*reportOutputVolume, error) {
	name, err := newReportOutputVolumeName(work)
	if err != nil {
		return nil, err
	}
	if _, err := d.exec(ctx, "docker", "volume", "create", "--label", "seven.review.report=true", "--label", "seven.review.run.id="+strconv.FormatInt(work.ReviewRunID, 10), "--label", "seven.review.owner.epoch="+strconv.FormatInt(work.OwnerEpoch, 10), name); err != nil {
		return nil, fmt.Errorf("create review report volume: %w", err)
	}
	body, err := d.exec(ctx, "docker", "volume", "inspect", name)
	if err != nil || !reportVolumeMatches(body, work, name) {
		_, _ = d.exec(context.Background(), "docker", "volume", "rm", "-f", name)
		return nil, ErrResultIdentityInvalid
	}
	initArgs := []string{"run", "--rm", "--user", "0:0", "--network", "none", "--read-only", "--cap-drop", "ALL", "--cap-add", "CHOWN", "--security-opt", "no-new-privileges", "--pids-limit", "32", "--memory", "64m", "--cpus", "0.10", "--tmpfs", "/tmp:rw,noexec,nosuid,size=8m",
		"--mount", "type=volume,src=" + name + ",dst=/report", "--entrypoint", "/bin/sh", work.ReviewerImage, "-ec", "chown 10001:10001 /report"}
	if _, err := d.exec(ctx, "docker", initArgs...); err != nil {
		_, _ = d.exec(context.Background(), "docker", "volume", "rm", "-f", name)
		return nil, fmt.Errorf("initialize review report volume: %w", err)
	}
	return &reportOutputVolume{name: name}, nil
}

func (d *Docker) recoverReportOutput(ctx context.Context, work Work, name string) (*reportOutputVolume, error) {
	if name == "" {
		return nil, ErrResultIdentityInvalid
	}
	body, err := d.exec(ctx, "docker", "volume", "inspect", name)
	if err != nil || !reportVolumeMatches(body, work, name) {
		return nil, ErrResultIdentityInvalid
	}
	return &reportOutputVolume{name: name}, nil
}

func (d *Docker) stopReportOutput(output *reportOutputVolume) error {
	if d == nil || output == nil || strings.TrimSpace(output.name) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), resourcejournal.Timeout)
	defer cancel()
	_, err := d.exec(ctx, "docker", "volume", "rm", output.name)
	return err
}

// CleanupOrphanReportVolumes is called once at Worker recovery after the
// authoritative local Reviewer-container scan. It removes only volumes with
// the expected label/name identity that no longer belong to a managed
// container; malformed volumes are retained fail-closed for operator cleanup.
func (d *Docker) CleanupOrphanReportVolumes(ctx context.Context, containers []ManagedContainer) error {
	if d == nil {
		return ErrResultIdentityInvalid
	}
	active := make(map[string]struct{}, len(containers))
	for _, container := range containers {
		if container.ReviewRunID > 0 && container.OwnerEpoch > 0 && container.ReportVolume != "" {
			active[container.ReportVolume] = struct{}{}
		}
	}
	volumes, err := d.exec(ctx, "docker", "volume", "ls", "-q", "--filter", "label=seven.review.report=true")
	if err != nil {
		return err
	}
	for _, name := range strings.Fields(string(volumes)) {
		body, inspectErr := d.exec(ctx, "docker", "volume", "inspect", name)
		if inspectErr != nil || !reportVolumeNamed(body, name) {
			continue
		}
		if _, kept := active[name]; kept {
			continue
		}
		if _, removeErr := d.exec(ctx, "docker", "volume", "rm", "-f", name); removeErr != nil {
			return removeErr
		}
	}
	return nil
}

func reportOutputVolumePrefix(work Work) string {
	return "seven-review-report-" + strconv.FormatInt(work.ReviewRunID, 10) + "-" + strconv.FormatInt(work.OwnerEpoch, 10) + "-"
}

func newReportOutputVolumeName(work Work) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return reportOutputVolumePrefix(work) + hex.EncodeToString(nonce), nil
}

func reportVolumeMatches(body []byte, work Work, name string) bool {
	var volumes []struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	}
	return json.Unmarshal(body, &volumes) == nil && len(volumes) == 1 && volumes[0].Name == name && volumes[0].Labels["seven.review.report"] == "true" && volumes[0].Labels["seven.review.run.id"] == strconv.FormatInt(work.ReviewRunID, 10) && volumes[0].Labels["seven.review.owner.epoch"] == strconv.FormatInt(work.OwnerEpoch, 10) && validReportVolumeName(name, work)
}

func reportVolumeNamed(body []byte, name string) bool {
	var volumes []struct {
		Name   string            `json:"Name"`
		Labels map[string]string `json:"Labels"`
	}
	if json.Unmarshal(body, &volumes) != nil || len(volumes) != 1 || volumes[0].Name != name || volumes[0].Labels["seven.review.report"] != "true" {
		return false
	}
	runID, runErr := strconv.ParseInt(volumes[0].Labels["seven.review.run.id"], 10, 64)
	epoch, epochErr := strconv.ParseInt(volumes[0].Labels["seven.review.owner.epoch"], 10, 64)
	return runErr == nil && epochErr == nil && runID > 0 && epoch > 0 && validReportVolumeName(name, Work{ReviewRunID: runID, OwnerEpoch: epoch})
}

func validReportVolumeName(name string, work Work) bool {
	prefix := reportOutputVolumePrefix(work)
	// TrimPrefix alone would accept a bare 32-hex-character volume name. The
	// nonce is useful only after the Run/owner prefix has been bound to the
	// Docker volume labels, so require both parts of the identity explicitly.
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	nonce := strings.TrimPrefix(name, prefix)
	if len(nonce) != 32 {
		return false
	}
	_, err := hex.DecodeString(nonce)
	return err == nil
}

func reportVolumeFromInspect(body []byte) (string, Work) {
	var inspected []struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		Mounts []struct {
			Type        string `json:"Type"`
			Name        string `json:"Name"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}
	if json.Unmarshal(body, &inspected) != nil || len(inspected) != 1 {
		return "", Work{}
	}
	labels := inspected[0].Config.Labels
	runID, runErr := strconv.ParseInt(labels["seven.review.run.id"], 10, 64)
	epoch, epochErr := strconv.ParseInt(labels["seven.review.owner.epoch"], 10, 64)
	if runErr != nil || epochErr != nil || runID < 1 || epoch < 1 {
		return "", Work{}
	}
	for _, mount := range inspected[0].Mounts {
		if mount.Destination == "/report" && mount.Type == "volume" && validReportVolumeName(mount.Name, Work{ReviewRunID: runID, OwnerEpoch: epoch}) {
			return mount.Name, Work{ReviewRunID: runID, OwnerEpoch: epoch}
		}
	}
	return "", Work{}
}

func validateReportEvidence(ctx context.Context, repositoryRoot, commit string, raw []byte) error {
	report, _, err := reviewdomain.CanonicalizeReport(raw)
	if err != nil {
		// The Server is the authority for protocol validation; it will receive
		// and safely reject the original candidate.
		return nil
	}
	for _, finding := range report.Findings {
		object, mode, lookupErr := frozenEvidenceEntry(ctx, repositoryRoot, commit, finding.Evidence.Path)
		if lookupErr != nil || mode != "100644" && mode != "100755" {
			// A Git symlink is a tree entry whose mode is 120000. It must never
			// become evidence because a reviewer could use it to point outside the
			// immutable ResultCommit view. Only regular blobs are admissible.
			return ErrResultIdentityInvalid
		}
		if _, err = commandOutput(ctx, "git", "--git-dir="+repositoryRoot, "cat-file", "-e", object); err != nil {
			return ErrResultIdentityInvalid
		}
		kind, kindErr := commandOutput(ctx, "git", "--git-dir="+repositoryRoot, "cat-file", "-t", object)
		if kindErr != nil || strings.TrimSpace(string(kind)) != "blob" {
			return ErrResultIdentityInvalid
		}
		blob, showErr := commandOutput(ctx, "git", "--git-dir="+repositoryRoot, "show", object)
		if showErr != nil || finding.Evidence.Line > lineCount(blob) {
			return ErrResultIdentityInvalid
		}
	}
	return nil
}

func frozenEvidenceEntry(ctx context.Context, repositoryRoot, commit, repositoryPath string) (string, string, error) {
	entries, err := commandOutput(ctx, "git", "--git-dir="+repositoryRoot, "ls-tree", "-z", commit, "--", repositoryPath)
	if err != nil {
		return "", "", ErrResultIdentityInvalid
	}
	var object, mode string
	for _, entry := range strings.Split(string(entries), "\x00") {
		if entry == "" {
			continue
		}
		metadata, entryPath, found := strings.Cut(entry, "\t")
		if !found || entryPath != repositoryPath {
			continue
		}
		parts := strings.Fields(metadata)
		if len(parts) != 3 || parts[1] != "blob" {
			return "", "", ErrResultIdentityInvalid
		}
		mode, object = parts[0], commit+":"+repositoryPath
		break
	}
	if mode == "" {
		return "", "", ErrResultIdentityInvalid
	}
	return object, mode, nil
}

func lineCount(value []byte) int {
	if len(value) == 0 {
		return 0
	}
	count := strings.Count(string(value), "\n")
	if value[len(value)-1] != '\n' {
		count++
	}
	return count
}

func executionIdentity(work Work) string {
	return digest([]byte(strings.Join([]string{"seven.review-runtime/v1", strconv.FormatInt(work.ReviewRunID, 10), strconv.FormatInt(work.OwnerEpoch, 10), work.ResultCommit, work.ResultTree, work.ReviewerImage}, "\x00")))
}

func digest(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }

func validateReviewInputDocuments(reviewContract, requirements []byte) error {
	for _, value := range [][]byte{reviewContract, requirements} {
		if len(value) == 0 || !json.Valid(value) {
			return ErrReviewInputInvalid
		}
		var document map[string]json.RawMessage
		if err := json.Unmarshal(value, &document); err != nil || document == nil {
			return ErrReviewInputInvalid
		}
	}
	return nil
}
