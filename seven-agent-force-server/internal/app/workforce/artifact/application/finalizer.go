package application

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	processorcheckpoint "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/buildinfo"
)

const (
	maxResultFiles     = 20_000
	maxResultFileBytes = 64 << 20
	maxResultTreeBytes = 256 << 20
)

type Finalizer struct {
	store EvidenceRecorder
}

type EvidenceRecorder interface {
	CheckEvidence(context.Context, int64, int64) ([]taskinfra.CheckEvidence, error)
	Events(context.Context, int64, int64, int) ([]domain.Event, error)
	RecordArtifact(context.Context, domain.Artifact) error
}

func NewFinalizer(store EvidenceRecorder, artifacts *artifactstore.Store) *Finalizer {
	return &Finalizer{store: store}
}

type Result struct {
	Commit          string
	Tree            string
	SelectionDigest string
}

type FrozenArtifact struct {
	Type string `json:"type"`
	Name string `json:"name"`
	// SourcePath is populated only for an IMMEDIATE Result Processor output.
	// It is a workspace-relative declaration used to make result-output audit
	// records explainable; it is never an object-store key or a trusted path
	// outside the checked repository root.
	SourcePath string `json:"sourcePath,omitempty"`
	MediaType  string `json:"mediaType"`
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	SizeBytes  int64  `json:"sizeBytes"`
}

type FrozenResult struct {
	Result
	SpoolRoot      string           `json:"spoolRoot"`
	SpoolIdentity  string           `json:"spoolIdentity"`
	ManifestDigest string           `json:"manifestDigest"`
	Artifacts      []FrozenArtifact `json:"artifacts"`
	// ReviewEvidence is a bounded, immutable index derived directly from the
	// trusted ResultCommit. It lets the Server validate a Reviewer's
	// repository-relative evidence path and line bound without accepting an
	// assertion from the Review Worker or mounting its local workspace.
	ReviewEvidence []ReviewEvidence `json:"reviewEvidence,omitempty"`
}

type ReviewEvidence struct {
	Path      string `json:"path"`
	LineCount int    `json:"lineCount"`
}

// ResultPlan is the immutable, Server-persisted PASS contract consumed by the
// trusted Result Coordinator.  It is deliberately much smaller than the
// Processor contract: actions merely request safe built-in materialization;
// the Processor never supplies bytes, hashes, object keys, or a Task outcome.
//
// Legacy is retained only for read-only historical tasks which predate P0.9.
// New Tasks always pass an explicit ResultPlan derived from their final PASS
// EvaluationCycle.
type ResultPlan struct {
	Outputs              []processordomain.Output `json:"outputs,omitempty"`
	Actions              []processordomain.Action `json:"actions,omitempty"`
	ContinuationIncludes []string                 `json:"continuationIncludes,omitempty"`
	Legacy               bool                     `json:"-"`
}

type artifactCandidate struct {
	typeName   string
	name       string
	sourcePath string
	mediaType  string
	body       []byte
}

func actionRequested(actions []processordomain.Action, expected string) bool {
	for _, action := range actions {
		if strings.EqualFold(strings.TrimSpace(action.Type), expected) {
			return true
		}
	}
	return false
}

// declaredOutputCandidates re-reads every requested file from the repository
// at finalization time.  Result Processor paths are declarations only: no
// Processor-provided bytes, MIME types, hashes, object keys or directories are
// trusted.  The function intentionally accepts only regular files below the
// checked repository root.
func declaredOutputCandidates(repo string, outputs []processordomain.Output) ([]artifactCandidate, error) {
	seenNames := make(map[string]struct{}, len(outputs))
	seenPaths := make(map[string]struct{}, len(outputs))
	result := make([]artifactCandidate, 0, len(outputs))
	root, err := filepath.Abs(repo)
	if err != nil {
		return nil, err
	}
	for _, output := range outputs {
		name := strings.TrimSpace(output.Name)
		path := filepath.ToSlash(strings.TrimSpace(output.Path))
		// ARCHIVE output is always produced by the independent, streaming
		// MaterializationRun after the format-2 Checkpoint is AVAILABLE.  This
		// keeps large archives out of Finalizer memory and gives IMMEDIATE and
		// ON_DEMAND the same deterministic archive implementation.
		if output.Kind == "ARCHIVE" {
			continue
		}
		// ON_DEMAND is a frozen declaration, not an eager Finalization input.
		// Its bytes are materialized later from the verified Continuation
		// Checkpoint by a dedicated MaterializationRun.
		if output.Storage == "ON_DEMAND" {
			continue
		}
		if name == "" || len(name) > 256 || output.Storage != "IMMEDIATE" || filepath.Base(name) != name {
			return nil, errors.New("RESULT_OUTPUT_DECLARATION_INVALID")
		}
		if _, exists := seenNames[name]; exists {
			return nil, errors.New("RESULT_OUTPUT_NAME_DUPLICATE")
		}
		if _, exists := seenPaths[path]; exists {
			return nil, errors.New("RESULT_OUTPUT_PATH_DUPLICATE")
		}
		seenNames[name], seenPaths[path] = struct{}{}, struct{}{}
		candidatePath := filepath.Clean(filepath.Join(root, filepath.FromSlash(path)))
		if candidatePath == root || !strings.HasPrefix(candidatePath, root+string(os.PathSeparator)) {
			return nil, errors.New("RESULT_OUTPUT_PATH_INVALID")
		}
		info, statErr := os.Lstat(candidatePath)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxResultFileBytes {
			return nil, errors.New("RESULT_OUTPUT_SOURCE_INVALID")
		}
		body, readErr := os.ReadFile(candidatePath)
		if readErr != nil || int64(len(body)) != info.Size() {
			return nil, errors.New("RESULT_OUTPUT_READ_FAILED")
		}
		result = append(result, artifactCandidate{
			typeName:   "RESULT_OUTPUT",
			name:       name,
			sourcePath: path,
			mediaType:  mediaTypeForOutput(name, candidatePath),
			body:       body,
		})
	}
	return result, nil
}

func mediaTypeForOutput(name, path string) string {
	extension := strings.ToLower(filepath.Ext(name))
	if extension == "" {
		extension = strings.ToLower(filepath.Ext(path))
	}
	switch extension {
	case ".json":
		return "application/json"
	case ".md":
		return "text/markdown"
	case ".txt", ".log":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".csv":
		return "text/csv"
	case ".xml":
		return "application/xml"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".patch", ".diff":
		return "text/x-diff"
	default:
		return "application/octet-stream"
	}
}

func (f *Finalizer) Freeze(ctx context.Context, item domain.WorkItem, attempt domain.Attempt, repo string, completion []byte, plan ResultPlan, spoolParent string, finalizationRunID int64) (FrozenResult, error) {
	if finalizationRunID <= 0 || strings.TrimSpace(spoolParent) == "" {
		return FrozenResult{}, errors.New("Finalization spool identity is required")
	}
	finalRoot := filepath.Join(spoolParent, fmt.Sprintf("%d", finalizationRunID))
	if existing, ok, err := loadFrozenResult(finalRoot, finalizationRunID, item.TaskID, item.TaskTurnID, attempt.ID); err != nil {
		return FrozenResult{}, err
	} else if ok {
		return existing, nil
	}
	identity, err := ComputeResultIdentityForPlan(ctx, repo, item.ResolvedBaseCommit, plan.Outputs, plan.ContinuationIncludes)
	if err != nil {
		return FrozenResult{}, err
	}
	commit, tree := identity.Commit, identity.Tree
	reviewEvidence, err := reviewEvidenceIndex(ctx, repo, commit)
	if err != nil {
		return FrozenResult{}, err
	}
	changedFiles, err := changedFileList(ctx, repo, item.ResolvedBaseCommit, commit)
	if err != nil {
		return FrozenResult{}, err
	}
	diffStat, err := gitOutput(ctx, repo, "diff", "--stat", "--summary", item.ResolvedBaseCommit, commit)
	if err != nil {
		return FrozenResult{}, err
	}

	// P0.9 does not unconditionally publish a patch, bundle, validation log or
	// runtime event archive.  Those latter two remain operations evidence; the
	// former two are materialized only when the immutable Processor explicitly
	// asks for them.  The trusted ResultCommit/ResultTree is still always
	// recomputed below.
	requestedPatch := plan.Legacy || actionRequested(plan.Actions, "PATCH")
	requestedBundle := plan.Legacy || actionRequested(plan.Actions, "BUNDLE")
	var patch, bundle []byte
	if requestedPatch {
		patch, err = gitBytes(ctx, repo, "diff", "--binary", "--full-index", item.ResolvedBaseCommit, commit)
		if err != nil {
			return FrozenResult{}, fmt.Errorf("create binary full-index patch: %w", err)
		}
		if err := verifyPatch(ctx, repo, item.ResolvedBaseCommit, patch); err != nil {
			return FrozenResult{}, err
		}
	}
	if requestedBundle {
		bundlePath := filepath.Join(filepath.Dir(repo), "output", "result.bundle")
		ref := fmt.Sprintf("refs/seven-worker/results/%d", attempt.ID)
		if err := runGit(ctx, repo, "update-ref", ref, commit); err != nil {
			return FrozenResult{}, err
		}
		defer runGit(context.Background(), repo, "update-ref", "-d", ref)
		if output, err := exec.CommandContext(ctx, "git", "-C", repo, "bundle", "create", bundlePath, ref, "^"+item.ResolvedBaseCommit).CombinedOutput(); err != nil {
			return FrozenResult{}, fmt.Errorf("create git bundle: %w: %s", err, bounded(output))
		}
		if output, err := exec.CommandContext(ctx, "git", "-C", repo, "bundle", "verify", bundlePath).CombinedOutput(); err != nil {
			return FrozenResult{}, fmt.Errorf("verify git bundle: %w: %s", err, bounded(output))
		}
		bundle, err = os.ReadFile(bundlePath)
		if err != nil {
			return FrozenResult{}, err
		}
	}
	executionBody, _ := json.MarshalIndent(map[string]any{
		"schemaVersion": 1, "taskId": strconv.FormatInt(item.TaskID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10),
		"harnessReleaseId": strconv.FormatInt(item.HarnessReleaseID, 10), "imageDigest": item.HarnessImageDigest,
		"baseCommit": item.ResolvedBaseCommit, "resultCommit": commit, "resultTree": tree,
		"changedFiles": changedFiles, "producer": buildinfo.String(),
	}, "", "  ")
	candidates := make([]artifactCandidate, 0, len(plan.Outputs)+4)
	if plan.Legacy {
		checks, checkErr := f.store.CheckEvidence(ctx, item.TaskID, attempt.ID)
		if checkErr != nil {
			return FrozenResult{}, checkErr
		}
		events, eventErr := f.store.Events(ctx, item.TaskID, 0, 1000)
		if eventErr != nil {
			return FrozenResult{}, eventErr
		}
		validationBody, _ := json.MarshalIndent(map[string]any{"schemaVersion": 1, "taskId": strconv.FormatInt(item.TaskID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10), "checks": checks}, "", "  ")
		eventsBody, _ := json.MarshalIndent(map[string]any{"schemaVersion": 1, "taskId": strconv.FormatInt(item.TaskID, 10), "events": events}, "", "  ")
		candidates = append(candidates,
			artifactCandidate{typeName: "PATCH", name: "changes.patch", mediaType: "text/x-diff", body: patch},
			artifactCandidate{typeName: "BUNDLE", name: "result.bundle", mediaType: "application/x-git-bundle", body: bundle},
			artifactCandidate{typeName: "COMPLETION", name: "completion.json", mediaType: "application/json", body: completion},
			artifactCandidate{typeName: "EXECUTION_SUMMARY", name: "execution-summary.json", mediaType: "application/json", body: executionBody},
			artifactCandidate{typeName: "VALIDATION_LOG", name: "validation-log.json", mediaType: "application/json", body: validationBody},
			artifactCandidate{typeName: "EVENT_ARCHIVE", name: "events.json", mediaType: "application/json", body: eventsBody},
		)
	} else {
		outputs, outputErr := declaredOutputCandidates(repo, plan.Outputs)
		if outputErr != nil {
			return FrozenResult{}, outputErr
		}
		candidates = append(candidates, outputs...)
		if requestedPatch {
			candidates = append(candidates, artifactCandidate{typeName: "PATCH", name: "changes.patch", mediaType: "text/x-diff", body: patch})
		}
		if requestedBundle {
			candidates = append(candidates, artifactCandidate{typeName: "BUNDLE", name: "result.bundle", mediaType: "application/x-git-bundle", body: bundle})
		}
	}
	artifactIndex := make([]map[string]any, 0, len(candidates))
	for _, candidate := range candidates {
		artifactIndex = append(artifactIndex, map[string]any{"name": candidate.name, "type": candidate.typeName, "sha256": digest(candidate.body), "sizeBytes": len(candidate.body)})
	}
	manifest := map[string]any{
		"schemaVersion": 1, "taskId": strconv.FormatInt(item.TaskID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10),
		"baseCommit": item.ResolvedBaseCommit, "resultCommit": commit, "resultTree": tree,
		"changedFiles": changedFiles, "diffStat": diffStat, "outputs": artifactIndex,
		"producer":  map[string]any{"name": "seven-worker", "version": buildinfo.Version, "commit": buildinfo.Commit},
		"createdAt": time.Now().UTC().Format(time.RFC3339Nano),
	}
	manifestBody, _ := json.MarshalIndent(manifest, "", "  ")
	// The manifest is an internal trusted result record. It is uploaded so a
	// later Continuation restore can verify identity, but ordinary user output
	// views only surface RESULT_OUTPUT and explicitly requested patch/bundle.
	candidates = append(candidates, artifactCandidate{typeName: "RESULT_MANIFEST", name: "result-manifest.json", mediaType: "application/json", body: manifestBody})
	staging, err := os.MkdirTemp(spoolParent, fmt.Sprintf(".%d-staging-", finalizationRunID))
	if err != nil {
		return FrozenResult{}, err
	}
	defer os.RemoveAll(staging)
	if err = os.Chmod(staging, 0o700); err != nil {
		return FrozenResult{}, err
	}
	frozen := make([]FrozenArtifact, 0, len(candidates))
	for _, candidate := range candidates {
		digestValue := digest(candidate.body)
		name := digestValue + "-" + candidate.name
		path := filepath.Join(staging, name)
		if err = writeSynced(path, candidate.body, 0o600); err != nil {
			return FrozenResult{}, err
		}
		frozen = append(frozen, FrozenArtifact{Type: candidate.typeName, Name: candidate.name, SourcePath: candidate.sourcePath, MediaType: candidate.mediaType, Path: name, SHA256: digestValue, SizeBytes: int64(len(candidate.body))})
	}
	// The Reviewer never receives the mutable Agent checkout.  Freeze a new,
	// detached bare repository containing the exact ResultCommit under the
	// durable Result directory before publishing READY.  Its only purpose is
	// read-only ResultCommit verification and COW scratch creation; it is not a
	// user artifact and it does not retain a remote back to the Agent workspace.
	if err = materializeReviewRepository(ctx, repo, filepath.Join(staging, "repository.git"), commit, tree); err != nil {
		return FrozenResult{}, err
	}
	// The outer manifest is the durable Result directory contract. It is
	// distinct from the RESULT_MANIFEST artifact above: this one proves the
	// local frozen payloads and their ResultCommit/ResultTree identities before
	// a later Reviewer is ever allowed to consume them.
	spoolManifest, _ := json.Marshal(map[string]any{"schemaVersion": 1, "finalizationRunId": strconv.FormatInt(finalizationRunID, 10), "taskId": strconv.FormatInt(item.TaskID, 10), "taskTurnId": strconv.FormatInt(item.TaskTurnID, 10), "attemptId": strconv.FormatInt(attempt.ID, 10), "resultCommit": commit, "resultTree": tree, "selectionDigest": identity.SelectionDigest, "artifacts": frozen, "reviewEvidence": reviewEvidence})
	manifestDigest := digest(spoolManifest)
	if err = writeSynced(filepath.Join(staging, "result-manifest.json"), spoolManifest, 0o600); err != nil {
		return FrozenResult{}, err
	}
	if err = syncDirectory(staging); err != nil {
		return FrozenResult{}, err
	}
	if err = os.Rename(staging, finalRoot); err != nil {
		return FrozenResult{}, err
	}
	// READY is intentionally the final write. An interrupted materialization
	// leaves either a sibling staging directory or a final directory without a
	// valid READY marker; startup recovery must treat both as non-recoverable.
	readyBody, _ := json.Marshal(struct {
		SchemaVersion  int    `json:"schemaVersion"`
		Manifest       string `json:"manifest"`
		ManifestDigest string `json:"manifestDigest"`
	}{SchemaVersion: 1, Manifest: "result-manifest.json", ManifestDigest: manifestDigest})
	if err = writeSynced(filepath.Join(finalRoot, "READY"), readyBody, 0o600); err != nil {
		return FrozenResult{}, err
	}
	if err = syncDirectory(finalRoot); err != nil {
		return FrozenResult{}, err
	}
	if err = syncDirectory(spoolParent); err != nil {
		return FrozenResult{}, err
	}
	verified, ok, verifyErr := loadFrozenResult(finalRoot, finalizationRunID, item.TaskID, item.TaskTurnID, attempt.ID)
	if verifyErr != nil || !ok {
		if verifyErr == nil {
			verifyErr = errors.New("frozen Finalization result did not become ready")
		}
		return FrozenResult{}, verifyErr
	}
	return verified, nil
}

// ComputeResultIdentity derives the trusted result commit/tree without
// mutating the Agent repository. It is shared by the continuation freeze and
// the independent FinalizationRun; both calls must produce the same identity.
func ComputeResultIdentity(ctx context.Context, repo, baseCommit string) (Result, error) {
	return ComputeResultIdentityForPlan(ctx, repo, baseCommit, nil, nil)
}

// ComputeResultIdentityForPlan freezes a changed result for Finalization.
// Output/continuation declarations influence the Selection digest and Overlay
// only; they never enlarge ResultTree.
func ComputeResultIdentityForPlan(ctx context.Context, repo, baseCommit string, outputs []processordomain.Output, continuationIncludes []string) (Result, error) {
	return computeResultIdentityForPlan(ctx, repo, baseCommit, outputs, continuationIncludes, false)
}

// ComputeCheckpointIdentityForPlan also permits an unchanged repository while
// waiting for user input or manual review. This preserves a resumable base;
// it does not authorize successful Finalization or Git delivery of a no-op.
func ComputeCheckpointIdentityForPlan(ctx context.Context, repo, baseCommit, decision string, outputs []processordomain.Output, continuationIncludes []string) (Result, error) {
	switch decision {
	case "PASS":
		return ComputeResultIdentityForPlan(ctx, repo, baseCommit, outputs, continuationIncludes)
	case "WAIT_USER", "MANUAL_REVIEW":
		return computeResultIdentityForPlan(ctx, repo, baseCommit, outputs, continuationIncludes, true)
	default:
		return Result{}, errors.New("CHECKPOINT_DECISION_INVALID")
	}
}

func computeResultIdentityForPlan(ctx context.Context, repo, baseCommit string, outputs []processordomain.Output, continuationIncludes []string, allowUnchanged bool) (Result, error) {
	indexPath, err := temporaryIndexPath(repo)
	if err != nil {
		return Result{}, err
	}
	defer os.Remove(indexPath)
	gitEnv := []string{
		"GIT_INDEX_FILE=" + indexPath,
		"GIT_AUTHOR_NAME=Seven Worker Finalizer", "GIT_AUTHOR_EMAIL=seven-worker@localhost",
		"GIT_COMMITTER_NAME=Seven Worker Finalizer", "GIT_COMMITTER_EMAIL=seven-worker@localhost",
		"GIT_AUTHOR_DATE=2000-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2000-01-01T00:00:00Z",
	}
	if err = runGitEnv(ctx, repo, gitEnv, "read-tree", baseCommit); err != nil {
		return Result{}, err
	}
	selection, err := processorcheckpoint.SelectResult(ctx, processorcheckpoint.SelectionInput{RepoRoot: repo, BaseCommit: baseCommit, Outputs: outputs, ContinuationIncludes: continuationIncludes})
	if err != nil {
		return Result{}, err
	}
	if err = validateResultSelection(repo, selection.ResultTreeIndex); err != nil {
		return Result{}, err
	}
	for _, entry := range selection.ResultTreeIndex {
		if entry.Type == "DELETED" {
			if err = runGitEnv(ctx, repo, gitEnv, "update-index", "--remove", "--", entry.Path); err != nil {
				return Result{}, err
			}
			continue
		}
		blob, hashErr := gitOutputEnv(ctx, repo, gitEnv, "hash-object", "-w", "--", entry.Path)
		if hashErr != nil {
			return Result{}, hashErr
		}
		if err = runGitEnv(ctx, repo, gitEnv, "update-index", "--add", "--cacheinfo", entry.GitMode+","+blob+","+entry.Path); err != nil {
			return Result{}, err
		}
	}
	tree, err := gitOutputEnv(ctx, repo, gitEnv, "write-tree")
	if err != nil {
		return Result{}, err
	}
	baseTree, err := gitOutput(ctx, repo, "rev-parse", baseCommit+"^{tree}")
	if err != nil {
		return Result{}, err
	}
	if tree == baseTree {
		if allowUnchanged {
			commit, err := gitOutput(ctx, repo, "rev-parse", baseCommit+"^{commit}")
			if err != nil {
				return Result{}, err
			}
			return Result{Commit: commit, Tree: tree, SelectionDigest: selection.SelectionDigest}, nil
		}
		return Result{}, errors.New("Agent produced no repository changes")
	}
	commit, err := gitCommitTree(ctx, repo, gitEnv, tree, baseCommit)
	if err != nil {
		return Result{}, err
	}
	return Result{Commit: commit, Tree: tree, SelectionDigest: selection.SelectionDigest}, nil
}

func loadFrozenResult(root string, runID int64, taskID int64, taskTurnID int64, attemptID int64) (FrozenResult, bool, error) {
	rootInfo, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return FrozenResult{}, false, nil
	}
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return FrozenResult{}, false, errors.New("frozen Finalization directory is invalid")
	}
	readyBody, err := readFrozenRegularFile(filepath.Join(root, "READY"), 64<<10)
	if errors.Is(err, os.ErrNotExist) {
		return FrozenResult{}, false, errors.New("frozen Finalization result is incomplete")
	}
	if err != nil {
		return FrozenResult{}, false, err
	}
	var ready struct {
		SchemaVersion  int    `json:"schemaVersion"`
		Manifest       string `json:"manifest"`
		ManifestDigest string `json:"manifestDigest"`
	}
	if err = json.Unmarshal(readyBody, &ready); err != nil || ready.SchemaVersion != 1 || ready.Manifest != "result-manifest.json" || len(ready.ManifestDigest) != 64 {
		return FrozenResult{}, false, errors.New("frozen Finalization ready marker is invalid")
	}
	body, err := readFrozenRegularFile(filepath.Join(root, ready.Manifest), 2<<20)
	if err != nil || digest(body) != ready.ManifestDigest {
		return FrozenResult{}, false, errors.New("frozen Finalization manifest digest mismatch")
	}
	var manifest struct {
		SchemaVersion     int              `json:"schemaVersion"`
		FinalizationRunID int64            `json:"finalizationRunId,string"`
		TaskID            int64            `json:"taskId,string"`
		TaskTurnID        int64            `json:"taskTurnId,string"`
		AttemptID         int64            `json:"attemptId,string"`
		ResultCommit      string           `json:"resultCommit"`
		ResultTree        string           `json:"resultTree"`
		SelectionDigest   string           `json:"selectionDigest"`
		Artifacts         []FrozenArtifact `json:"artifacts"`
		ReviewEvidence    []ReviewEvidence `json:"reviewEvidence"`
	}
	if err = json.Unmarshal(body, &manifest); err != nil || manifest.SchemaVersion != 1 || manifest.FinalizationRunID != runID || manifest.TaskID != taskID || manifest.TaskTurnID != taskTurnID || manifest.TaskTurnID < 1 || manifest.AttemptID != attemptID || !validGitObjectID(manifest.ResultCommit) || !validGitObjectID(manifest.ResultTree) || !validHexDigest(manifest.SelectionDigest) || len(manifest.Artifacts) == 0 {
		return FrozenResult{}, false, errors.New("frozen Finalization manifest identity is invalid")
	}
	for _, artifact := range manifest.Artifacts {
		if filepath.Base(artifact.Path) != artifact.Path || len(artifact.SHA256) != 64 {
			return FrozenResult{}, false, errors.New("frozen Artifact path is invalid")
		}
		payload, readErr := readFrozenRegularFile(filepath.Join(root, artifact.Path), artifact.SizeBytes)
		if readErr != nil || int64(len(payload)) != artifact.SizeBytes || digest(payload) != artifact.SHA256 {
			return FrozenResult{}, false, errors.New("frozen Artifact digest mismatch")
		}
	}
	identity := digest(body)
	if err = validateReviewEvidenceIndex(manifest.ReviewEvidence); err != nil {
		return FrozenResult{}, false, err
	}
	return FrozenResult{Result: Result{Commit: manifest.ResultCommit, Tree: manifest.ResultTree, SelectionDigest: manifest.SelectionDigest}, SpoolRoot: root, SpoolIdentity: identity, ManifestDigest: identity, Artifacts: manifest.Artifacts, ReviewEvidence: manifest.ReviewEvidence}, true, nil
}

func reviewEvidenceIndex(ctx context.Context, repo, commit string) ([]ReviewEvidence, error) {
	body, err := gitBytes(ctx, repo, "ls-tree", "-rz", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	entries := bytes.Split(body, []byte{0})
	result := make([]ReviewEvidence, 0, len(entries))
	for _, entry := range entries {
		if len(entry) == 0 {
			continue
		}
		tab := bytes.IndexByte(entry, '\t')
		if tab <= 0 || tab == len(entry)-1 {
			return nil, errors.New("review evidence tree entry is invalid")
		}
		// The metadata before the tab is exactly "<mode> <type> <object>".
		// The path comes after the tab so paths with whitespace remain separate
		// from the trusted object identity.
		fields := strings.Fields(string(entry[:tab]))
		if len(fields) != 3 || fields[1] != "blob" {
			continue
		}
		filePath := string(entry[tab+1:])
		if !validReviewEvidencePath(filePath) {
			// Non-UTF8 or special Git paths can remain in the trusted result
			// tree, but can never become user-visible Reviewer evidence.
			continue
		}
		blob, blobErr := gitBytes(ctx, repo, "cat-file", "blob", fields[2])
		if blobErr != nil {
			return nil, blobErr
		}
		result = append(result, ReviewEvidence{Path: filePath, LineCount: reviewLineCount(blob)})
		if len(result) > maxResultFiles {
			return nil, errors.New("review evidence file limit exceeded")
		}
	}
	if len(result) == 0 {
		return nil, errors.New("review evidence index is empty")
	}
	return result, nil
}

func validateReviewEvidenceIndex(entries []ReviewEvidence) error {
	if len(entries) > maxResultFiles {
		return errors.New("review evidence index exceeds file limit")
	}
	previous := ""
	for _, entry := range entries {
		if !validReviewEvidencePath(entry.Path) || entry.LineCount < 0 || (previous != "" && previous >= entry.Path) {
			return errors.New("review evidence index is invalid")
		}
		previous = entry.Path
	}
	return nil
}

func validReviewEvidencePath(value string) bool {
	return value != "" && utf8.ValidString(value) && len(value) <= 1024 && !strings.HasPrefix(value, "/") && !strings.Contains(value, "\\") && path.Clean(value) == value && value != "." && !strings.HasPrefix(value, "../")
}

func reviewLineCount(value []byte) int {
	if len(value) == 0 {
		return 0
	}
	count := bytes.Count(value, []byte{'\n'})
	if value[len(value)-1] != '\n' {
		count++
	}
	return count
}

func validGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// materializeReviewRepository builds a minimal, detached bare repository for
// the trusted ResultCommit.  Unlike a filesystem clone it records no remote
// URL pointing at the mutable Agent workspace, and it is sealed read-only
// before the Result directory can become READY.
func materializeReviewRepository(ctx context.Context, sourceRepo, destination, commit, tree string) error {
	if !validGitObjectID(commit) || !validGitObjectID(tree) || strings.TrimSpace(sourceRepo) == "" || filepath.Base(destination) != "repository.git" {
		return errors.New("review repository identity is invalid")
	}
	if output, err := exec.CommandContext(ctx, "git", "init", "--bare", "--", destination).CombinedOutput(); err != nil {
		return fmt.Errorf("initialize frozen review repository: %w: %s", err, bounded(output))
	}
	// Fetch only the trusted ResultCommit into a fixed local ref.  Git's local
	// fetch protocol writes objects to the new bare repository and does not
	// add a remote section to its config.
	if output, err := exec.CommandContext(ctx, "git", "--git-dir="+destination, "fetch", "--no-tags", "--no-write-fetch-head", "--", sourceRepo, commit+":refs/seven-workforce/review/result").CombinedOutput(); err != nil {
		return fmt.Errorf("materialize frozen review commit: %w: %s", err, bounded(output))
	}
	// Be defensive for Git versions that still create FETCH_HEAD despite the
	// no-write flag: it can contain a local source path, which is neither input
	// nor evidence the isolated Reviewer is allowed to observe.
	if err := os.Remove(filepath.Join(destination, "FETCH_HEAD")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove frozen review fetch head: %w", err)
	}
	resolvedCommit, err := gitBareOutput(ctx, destination, "rev-parse", commit+"^{commit}")
	if err != nil || resolvedCommit != commit {
		return errors.New("frozen review repository commit mismatch")
	}
	resolvedTree, err := gitBareOutput(ctx, destination, "rev-parse", commit+"^{tree}")
	if err != nil || resolvedTree != tree {
		return errors.New("frozen review repository tree mismatch")
	}
	if remotes, remoteErr := gitBareOutput(ctx, destination, "remote"); remoteErr != nil || remotes != "" {
		return errors.New("frozen review repository unexpectedly retains a remote")
	}
	if err := sealReadOnlyTree(destination); err != nil {
		return fmt.Errorf("seal frozen review repository: %w", err)
	}
	return nil
}

func gitBareOutput(ctx context.Context, repository string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"--git-dir=" + repository}, args...)...)
	command.Env = safeGitEnvironment()
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git --git-dir %s: %w: %s", strings.Join(args, " "), err, bounded(output))
	}
	return strings.TrimSpace(string(output)), nil
}

// sealReadOnlyTree rejects non-regular entries and removes every write bit
// from the narrow bare repository that is bind-mounted into the Reviewer.
func sealReadOnlyTree(root string) error {
	return filepath.WalkDir(root, func(entryPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("frozen review repository contains unsafe entry")
		}
		if entry.IsDir() {
			return os.Chmod(entryPath, 0o555)
		}
		if !info.Mode().IsRegular() {
			return errors.New("frozen review repository contains non-regular entry")
		}
		return os.Chmod(entryPath, 0o444)
	})
}

func readFrozenRegularFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, errors.New("frozen Finalization file is invalid")
	}
	body, err := os.ReadFile(path)
	if err != nil || int64(len(body)) != info.Size() {
		return nil, errors.New("frozen Finalization file is unavailable")
	}
	return body, nil
}

func writeSynced(path string, body []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func validateResultSelection(repo string, entries processorcheckpoint.SelectionIndex) error {
	if len(entries) > maxResultFiles {
		return errors.New("result tree exceeds file-count limit")
	}
	var total int64
	secretNames := map[string]bool{".env": true, ".npmrc": true, ".pypirc": true, "auth.json": true, "credentials": true, "credentials.json": true, "id_rsa": true, "id_ed25519": true}
	for _, entry := range entries {
		if entry.Type == "DELETED" {
			continue
		}
		if entry.Type != "FILE" || (entry.GitMode != "100644" && entry.GitMode != "100755") {
			return fmt.Errorf("result tree contains forbidden file type at %q", entry.Path)
		}
		name := strings.ToLower(filepath.Base(filepath.FromSlash(entry.Path)))
		if secretNames[name] || strings.HasPrefix(name, ".env.") {
			return fmt.Errorf("result tree contains denied secret filename %q", entry.Path)
		}
		path := filepath.Join(repo, filepath.FromSlash(entry.Path))
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("result tree contains forbidden file type at %q", entry.Path)
		}
		if info.Size() != entry.SizeBytes || info.Size() > maxResultFileBytes {
			return fmt.Errorf("result file %q changed or exceeds size limit", entry.Path)
		}
		body, err := os.ReadFile(path)
		if err != nil || digest(body) != entry.SHA256 {
			return fmt.Errorf("result file %q changed during selection", entry.Path)
		}
		total += info.Size()
		if total > maxResultTreeBytes {
			return errors.New("result tree exceeds total-size limit")
		}
	}
	return nil
}

func temporaryIndexPath(repo string) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(repo), ".seven-index-*")
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func gitCommitTree(ctx context.Context, repo string, env []string, tree, parent string) (string, error) {
	command := exec.CommandContext(ctx, "git", "-C", repo, "commit-tree", tree, "-p", parent)
	command.Env = safeGitEnvironment(env...)
	command.Stdin = strings.NewReader("seven-worker: apply task result\n")
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("create result commit: %w: %s", err, bounded(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func verifyPatch(ctx context.Context, repo, base string, patch []byte) error {
	root, err := os.MkdirTemp("", "seven-patch-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if output, err := exec.CommandContext(ctx, "git", "clone", "--quiet", "--no-checkout", "--", repo, root).CombinedOutput(); err != nil {
		return fmt.Errorf("prepare patch verification: %w: %s", err, bounded(output))
	}
	if err := runGit(ctx, root, "checkout", "--quiet", "--detach", base); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, "git", "-C", root, "apply", "--check", "--binary", "-")
	command.Stdin = bytes.NewReader(patch)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("verify binary patch: %w: %s", err, bounded(output))
	}
	return nil
}

func changedFileList(ctx context.Context, repo, base, commit string) ([]string, error) {
	body, err := gitBytes(ctx, repo, "diff", "--name-only", "-z", base, commit)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(body), "\x00")
	result := make([]string, 0, len(parts))
	for _, item := range parts {
		if item != "" {
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result, nil
}

func runGit(ctx context.Context, repo string, args ...string) error {
	return runGitEnv(ctx, repo, nil, args...)
}

func runGitEnv(ctx context.Context, repo string, env []string, args ...string) error {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	command.Env = safeGitEnvironment(env...)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, bounded(output))
	}
	return nil
}

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	return gitOutputEnv(ctx, repo, nil, args...)
}

func gitOutputEnv(ctx context.Context, repo string, env []string, args ...string) (string, error) {
	body, err := gitBytesEnv(ctx, repo, env, args...)
	return strings.TrimSpace(string(body)), err
}

func gitBytes(ctx context.Context, repo string, args ...string) ([]byte, error) {
	return gitBytesEnv(ctx, repo, nil, args...)
}

func gitBytesEnv(ctx context.Context, repo string, env []string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	command.Env = safeGitEnvironment(env...)
	output, err := command.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, bounded(output))
	}
	return output, nil
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func safeGitEnvironment(extra ...string) []string {
	environment := make([]string, 0, len(os.Environ())+len(extra)+5)
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") && !strings.HasPrefix(value, "SSH_ASKPASS=") {
			environment = append(environment, value)
		}
	}
	environment = append(environment,
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=/bin/false", "SSH_ASKPASS=/bin/false",
	)
	return append(environment, extra...)
}

func bounded(value []byte) string {
	if len(value) > 8192 {
		value = value[:8192]
	}
	return strings.TrimSpace(string(value))
}
