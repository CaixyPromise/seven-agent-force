package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	workspaceRootMode            = 0o700
	workspaceInputMode           = 0o755
	workspaceAttemptMode         = 0o777 // bind-mounted into the non-root Attempt image
	workspaceStateMode           = 0o700 // Adapter native session and Supervisor journal are private
	minimumWorkspaceFreeBytes    = uint64(512 << 20)
	workspaceRetention           = 7 * 24 * time.Hour
	maxWorkspaceManifestBytes    = 2 << 20
	maxWorkspaceArtifactScanSize = int64(256 << 20)
)

// PersistentWorkspace is the Worker-owned, same-host task storage boundary.
// PostgreSQL and encrypted checkpoint/object storage remain the authority for
// task state and cross-host recovery. This directory only makes a Worker
// restart on the same host auditable and resumable where the durable state
// machine permits it.
type PersistentWorkspace struct {
	root string
	now  func() time.Time
}

// OpenPersistentWorkspace validates and opens worker.workspaceRoot without
// introducing a second configuration setting. The returned root is an
// absolute, canonical host path; callers must use it rather than the raw
// configuration string when constructing bind mounts or retention paths.
func OpenPersistentWorkspace(root string) (*PersistentWorkspace, error) {
	canonical, err := validateWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	return &PersistentWorkspace{root: canonical, now: time.Now}, nil
}

func (w *PersistentWorkspace) Root() string {
	if w == nil {
		return ""
	}
	return w.root
}

// ResultWorkspace reserves the frozen ResultCommit directory contract. Slice
// 1 establishes and verifies this durable envelope; it does not start a
// Reviewer or copy a second repository merely for a future Reviewer ceremony.
type ResultWorkspace struct {
	TaskRoot       string
	TurnRoot       string
	Root           string
	RepositoryRoot string
	ManifestPath   string
	EvidenceRoot   string
	ReadyPath      string
}

// ReviewWorkspace owns only durable Worker inputs, runtime identity and
// recovery journals. An untrusted candidate report is deliberately excluded:
// it remains in a Docker-internal per-Run volume until the Worker copies it
// into a short-lived OS temporary file for immediate submission to the Server.
// The volume is removed on every terminal/recovery cleanup path. Only the
// Server's canonical report may become durable business evidence.
type ReviewWorkspace struct {
	TaskRoot    string
	InputRoot   string
	InputReport string
	Root        string
	InputPath   string
	RuntimeRoot string
	JournalRoot string
}

func (w *PersistentWorkspace) Attempt(taskID, attemptID int64) (Workspace, error) {
	taskRoot, err := w.taskRoot(taskID)
	if err != nil {
		return Workspace{}, err
	}
	attemptText, err := decimalID(attemptID)
	if err != nil {
		return Workspace{}, err
	}
	inputRoot := filepath.Join(taskRoot, "input")
	attemptRoot := filepath.Join(taskRoot, "attempts", attemptText)
	if err = w.assertSafeDirectoryPath(attemptRoot); err != nil {
		return Workspace{}, err
	}
	return Workspace{
		TaskRoot: taskRoot, InputRoot: inputRoot, AttemptRoot: attemptRoot,
		Repo: filepath.Join(attemptRoot, "repo"), Output: filepath.Join(attemptRoot, "output"),
		State: filepath.Join(attemptRoot, "state"), Runtime: filepath.Join(attemptRoot, "runtime"),
		Temporary: filepath.Join(attemptRoot, "tmp"), SpecPath: filepath.Join(inputRoot, "attempts", attemptText, "attempt.json"),
	}, nil
}

// PrepareAttempt creates only the current Attempt's writable directories and
// its Worker-owned input directory. No sibling Attempt or whole Task root is
// mounted into a container.
func (w *PersistentWorkspace) PrepareAttempt(taskID, attemptID int64) (Workspace, error) {
	workspace, err := w.Attempt(taskID, attemptID)
	if err != nil {
		return Workspace{}, err
	}
	for _, directory := range []struct {
		path string
		mode os.FileMode
	}{
		{workspace.TaskRoot, workspaceRootMode},
		{workspace.InputRoot, workspaceInputMode},
		{filepath.Join(workspace.InputRoot, "attempts"), workspaceInputMode},
		{filepath.Dir(workspace.SpecPath), workspaceInputMode},
		{filepath.Join(workspace.TaskRoot, "attempts"), workspaceRootMode},
		{workspace.AttemptRoot, workspaceAttemptMode},
		{workspace.Output, workspaceAttemptMode},
		{workspace.State, workspaceStateMode},
		{workspace.Runtime, workspaceAttemptMode},
		{workspace.Temporary, workspaceAttemptMode},
	} {
		if err = w.ensureDirectory(directory.path, directory.mode); err != nil {
			return Workspace{}, err
		}
	}
	// Mkdir/MkdirAll do not tighten an already prepared directory. Enforce the
	// current Attempt's private state contract before exposing it to the Adapter;
	// do not follow a substituted final-component symlink or touch sibling state.
	state, err := os.OpenFile(workspace.State, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return Workspace{}, fmt.Errorf("open private Attempt state: %w", err)
	}
	defer state.Close()
	if err := state.Chmod(workspaceStateMode); err != nil {
		return Workspace{}, fmt.Errorf("secure private Attempt state: %w", err)
	}
	info, err := state.Stat()
	if err != nil || info.Mode().Perm() != workspaceStateMode {
		return Workspace{}, errors.New("WORKSPACE_STATE_NOT_PRIVATE")
	}
	return workspace, nil
}

func (w *PersistentWorkspace) Result(taskID, taskTurnID, finalizationRunID int64) (ResultWorkspace, error) {
	taskRoot, err := w.taskRoot(taskID)
	if err != nil {
		return ResultWorkspace{}, err
	}
	turnText, err := decimalID(taskTurnID)
	if err != nil {
		return ResultWorkspace{}, err
	}
	runText, err := decimalID(finalizationRunID)
	if err != nil {
		return ResultWorkspace{}, err
	}
	turnRoot := filepath.Join(taskRoot, "results", turnText)
	root := filepath.Join(turnRoot, runText)
	if err = w.assertSafeDirectoryPath(root); err != nil {
		return ResultWorkspace{}, err
	}
	return ResultWorkspace{
		TaskRoot: taskRoot, TurnRoot: turnRoot, Root: root,
		RepositoryRoot: filepath.Join(root, "repository.git"),
		ManifestPath:   filepath.Join(root, "result-manifest.json"),
		EvidenceRoot:   filepath.Join(root, "deterministic-evidence"),
		ReadyPath:      filepath.Join(root, "READY"),
	}, nil
}

// PrepareResultParent creates the parent only. The final run directory itself
// must be made by Finalizer through a sibling staging directory, fsync, atomic
// rename and a final READY marker.
func (w *PersistentWorkspace) PrepareResultParent(result ResultWorkspace) error {
	if err := w.assertContained(result.TurnRoot); err != nil {
		return err
	}
	for _, directory := range []struct {
		path string
		mode os.FileMode
	}{
		{result.TaskRoot, workspaceRootMode},
		{filepath.Join(result.TaskRoot, "results"), workspaceRootMode},
		{result.TurnRoot, workspaceRootMode},
	} {
		if err := w.ensureDirectory(directory.path, directory.mode); err != nil {
			return err
		}
	}
	return nil
}

func (w *PersistentWorkspace) Review(taskID, reviewRunID int64) (ReviewWorkspace, error) {
	taskRoot, err := w.taskRoot(taskID)
	if err != nil {
		return ReviewWorkspace{}, err
	}
	reviewText, err := decimalID(reviewRunID)
	if err != nil {
		return ReviewWorkspace{}, err
	}
	inputRoot := filepath.Join(taskRoot, "input", "review", reviewText)
	root := filepath.Join(taskRoot, "reviews", reviewText)
	if err = w.assertSafeDirectoryPath(root); err != nil {
		return ReviewWorkspace{}, err
	}
	return ReviewWorkspace{
		TaskRoot: taskRoot, InputRoot: inputRoot, InputReport: filepath.Join(inputRoot, "review-report.json"),
		Root: root, InputPath: filepath.Join(root, "input.json"),
		RuntimeRoot: filepath.Join(root, "runtime"), JournalRoot: filepath.Join(root, "journal"),
	}, nil
}

// ReviewAttempt resolves one fenced Reviewer execution envelope. Scratch,
// input and journals are scoped to the durable ReviewRun *and* owner epoch:
// a retry must never inherit mutable COW state from a failed Attempt, while a
// same-owner recovery can still reopen the exact original envelope.
func (w *PersistentWorkspace) ReviewAttempt(taskID, reviewRunID, ownerEpoch int64) (ReviewWorkspace, error) {
	base, err := w.Review(taskID, reviewRunID)
	if err != nil {
		return ReviewWorkspace{}, err
	}
	epochText, err := decimalID(ownerEpoch)
	if err != nil {
		return ReviewWorkspace{}, err
	}
	inputRoot := filepath.Join(base.InputRoot, "attempts", epochText)
	root := filepath.Join(base.Root, "attempts", epochText)
	if err = w.assertSafeDirectoryPath(root); err != nil {
		return ReviewWorkspace{}, err
	}
	return ReviewWorkspace{
		TaskRoot: base.TaskRoot, InputRoot: inputRoot, InputReport: filepath.Join(inputRoot, "review-report.json"),
		Root: root, InputPath: filepath.Join(root, "input.json"),
		RuntimeRoot: filepath.Join(root, "runtime"), JournalRoot: filepath.Join(root, "journal"),
	}, nil
}

// PrepareReviewAttempt creates only this fenced ReviewRun Attempt's durable
// input/recovery envelope. The caller still decides which narrow paths are
// mounted into a container; this method never returns the mutable Agent
// workspace, the whole Task root, or a durable candidate-report path.
func (w *PersistentWorkspace) PrepareReviewAttempt(taskID, reviewRunID, ownerEpoch int64) (ReviewWorkspace, error) {
	review, err := w.ReviewAttempt(taskID, reviewRunID, ownerEpoch)
	if err != nil {
		return ReviewWorkspace{}, err
	}
	base, err := w.Review(taskID, reviewRunID)
	if err != nil {
		return ReviewWorkspace{}, err
	}
	for _, directory := range []struct {
		path string
		mode os.FileMode
	}{
		{review.TaskRoot, workspaceRootMode},
		{filepath.Join(review.TaskRoot, "input"), workspaceInputMode},
		{filepath.Join(review.TaskRoot, "input", "review"), workspaceInputMode},
		{base.InputRoot, workspaceInputMode},
		{filepath.Join(base.InputRoot, "attempts"), workspaceInputMode},
		{review.InputRoot, workspaceInputMode},
		{filepath.Join(review.TaskRoot, "reviews"), workspaceRootMode},
		{base.Root, workspaceRootMode},
		{filepath.Join(base.Root, "attempts"), workspaceRootMode},
		{review.Root, workspaceRootMode},
		{review.RuntimeRoot, workspaceAttemptMode},
		{review.JournalRoot, workspaceAttemptMode},
	} {
		if err = w.ensureDirectory(directory.path, directory.mode); err != nil {
			return ReviewWorkspace{}, err
		}
	}
	return review, nil
}

// PrepareReview preserves the historical first-owner helper for callers that
// do not yet possess a machine owner epoch. Runtime execution must always use
// PrepareReviewAttempt so retries receive independent writable state.
func (w *PersistentWorkspace) PrepareReview(taskID, reviewRunID int64) (ReviewWorkspace, error) {
	return w.PrepareReviewAttempt(taskID, reviewRunID, 1)
}

// WriteAtomic writes a Worker-owned file inside workspaceRoot only after the
// parent is proven non-symlinked. It is used for the Attempt spec; result
// materialization uses the stronger directory protocol in Finalizer.
func (w *PersistentWorkspace) WriteAtomic(path string, body []byte, mode os.FileMode) error {
	if err := w.assertContained(path); err != nil {
		return err
	}
	if err := w.assertExistingDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return writeFileAtomically(path, body, mode)
}

func (w *PersistentWorkspace) taskRoot(taskID int64) (string, error) {
	taskText, err := decimalID(taskID)
	if err != nil {
		return "", err
	}
	path := filepath.Join(w.root, taskText)
	if err = w.assertSafeDirectoryPath(path); err != nil {
		return "", err
	}
	return path, nil
}

func (w *PersistentWorkspace) ensureDirectory(path string, mode os.FileMode) error {
	if err := w.assertContained(path); err != nil {
		return err
	}
	relative, err := filepath.Rel(w.root, path)
	if err != nil {
		return err
	}
	current := w.root
	if relative == "." {
		return w.assertExistingDirectory(current)
	}
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		if component == "" || component == "." || component == ".." {
			return errors.New("WORKSPACE_PATH_INVALID")
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			if err = os.Mkdir(current, mode); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("WORKSPACE_PATH_SYMLINK_OR_NOT_DIRECTORY")
		}
	}
	return nil
}

func (w *PersistentWorkspace) assertSafeDirectoryPath(path string) error {
	if err := w.assertContained(path); err != nil {
		return err
	}
	relative, err := filepath.Rel(w.root, path)
	if err != nil {
		return err
	}
	current := w.root
	if relative == "." {
		return w.assertExistingDirectory(current)
	}
	for _, component := range strings.Split(relative, string(os.PathSeparator)) {
		if component == "" || component == "." || component == ".." {
			return errors.New("WORKSPACE_PATH_INVALID")
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if errors.Is(statErr, os.ErrNotExist) {
			return nil
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("WORKSPACE_PATH_SYMLINK_OR_NOT_DIRECTORY")
		}
	}
	return nil
}

func (w *PersistentWorkspace) assertExistingDirectory(path string) error {
	if err := w.assertContained(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("WORKSPACE_PATH_SYMLINK_OR_NOT_DIRECTORY")
	}
	return nil
}

func (w *PersistentWorkspace) assertContained(path string) error {
	if w == nil || w.root == "" {
		return errors.New("WORKSPACE_ROOT_UNAVAILABLE")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(w.root, abs)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) || filepath.IsAbs(relative) {
		return errors.New("WORKSPACE_PATH_ESCAPE")
	}
	return nil
}

func validateWorkspaceRoot(raw string) (string, error) {
	root := strings.TrimSpace(raw)
	if root == "" || !filepath.IsAbs(root) {
		return "", errors.New("worker.workspaceRoot must be an absolute path")
	}
	root = filepath.Clean(root)
	if isFilesystemRoot(root) {
		return "", errors.New("worker.workspaceRoot must not be the filesystem root")
	}
	if err := os.MkdirAll(root, workspaceRootMode); err != nil {
		return "", fmt.Errorf("create worker.workspaceRoot: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil || !filepath.IsAbs(canonical) || isFilesystemRoot(canonical) {
		return "", errors.New("worker.workspaceRoot cannot be resolved safely")
	}
	info, err := os.Stat(canonical)
	if err != nil || !info.IsDir() {
		return "", errors.New("worker.workspaceRoot must be a directory")
	}
	if info.Mode().Perm()&0o022 != 0 {
		return "", errors.New("worker.workspaceRoot must not be group/world writable")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Getuid() {
		return "", errors.New("worker.workspaceRoot must be owned by the Worker user")
	}
	probe, err := os.CreateTemp(canonical, ".seven-workspace-probe-")
	if err != nil {
		return "", errors.New("worker.workspaceRoot is not writable")
	}
	probeName := probe.Name()
	writeErr := func() error {
		if _, err = probe.Write([]byte("ok")); err != nil {
			return err
		}
		return probe.Sync()
	}()
	closeErr := probe.Close()
	removeErr := os.Remove(probeName)
	if writeErr != nil || closeErr != nil || removeErr != nil {
		return "", errors.New("worker.workspaceRoot is not writable")
	}
	var fs syscall.Statfs_t
	if err = syscall.Statfs(canonical, &fs); err != nil {
		return "", fmt.Errorf("inspect worker.workspaceRoot free space: %w", err)
	}
	available := uint64(fs.Bavail) * uint64(fs.Bsize)
	if available < minimumWorkspaceFreeBytes {
		return "", fmt.Errorf("worker.workspaceRoot has insufficient free space: need at least %d bytes", minimumWorkspaceFreeBytes)
	}
	return canonical, nil
}

func isFilesystemRoot(path string) bool {
	volume := filepath.VolumeName(path)
	if volume != "" {
		return filepath.Clean(path) == filepath.Clean(volume+string(os.PathSeparator))
	}
	return filepath.Clean(path) == string(os.PathSeparator)
}

func decimalID(value int64) (string, error) {
	if value < 1 {
		return "", errors.New("WORKSPACE_ID_INVALID")
	}
	return strconv.FormatInt(value, 10), nil
}

func decimal(value int64) string { return strconv.FormatInt(value, 10) }

func writeFileAtomically(path string, body []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	staging, err := os.CreateTemp(directory, "."+filepath.Base(path)+".staging-")
	if err != nil {
		return err
	}
	stagingName := staging.Name()
	defer os.Remove(stagingName)
	if err = staging.Chmod(mode); err != nil {
		_ = staging.Close()
		return err
	}
	if _, err = staging.Write(body); err != nil {
		_ = staging.Close()
		return err
	}
	if err = staging.Sync(); err != nil {
		_ = staging.Close()
		return err
	}
	if err = staging.Close(); err != nil {
		return err
	}
	if err = os.Rename(stagingName, path); err != nil {
		return err
	}
	return syncWorkspaceDirectory(directory)
}

func syncWorkspaceDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

type ScanState string

const (
	ScanReady         ScanState = "READY"
	ScanPartial       ScanState = "PARTIAL"
	ScanCorrupt       ScanState = "CORRUPT"
	ScanUnknownSchema ScanState = "UNKNOWN_SCHEMA"
)

// ScanRecord contains only IDs and safe reason codes. It deliberately never
// serializes workspace contents, provider data, tokens, or absolute paths.
type ScanRecord struct {
	TaskID            int64
	TaskTurnID        int64
	FinalizationRunID int64
	State             ScanState
	ReasonCode        string
}

// Scan recognizes complete Result directories and treats every incomplete or
// corrupt directory as non-recoverable. It never restores process memory or
// treats a directory alone as Task state authority.
func (w *PersistentWorkspace) Scan(ctx context.Context) ([]ScanRecord, error) {
	entries, err := os.ReadDir(w.root)
	if err != nil {
		return nil, err
	}
	records := make([]ScanRecord, 0)
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		taskID, valid := parseDecimalID(entry.Name())
		if !valid {
			continue // Worker caches and lock files are not Task directories.
		}
		taskRoot := filepath.Join(w.root, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			records = append(records, ScanRecord{TaskID: taskID, State: ScanCorrupt, ReasonCode: "TASK_ROOT_INVALID"})
			continue
		}
		resultRoot := filepath.Join(taskRoot, "results")
		info, statErr := os.Lstat(resultRoot)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			records = append(records, ScanRecord{TaskID: taskID, State: ScanCorrupt, ReasonCode: "RESULTS_ROOT_INVALID"})
			continue
		}
		turns, readErr := os.ReadDir(resultRoot)
		if readErr != nil {
			return nil, readErr
		}
		for _, turn := range turns {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			turnID, turnValid := parseDecimalID(turn.Name())
			if !turnValid || turn.Type()&os.ModeSymlink != 0 || !turn.IsDir() {
				records = append(records, ScanRecord{TaskID: taskID, State: ScanUnknownSchema, ReasonCode: "RESULT_TURN_DIRECTORY_INVALID"})
				continue
			}
			runs, runReadErr := os.ReadDir(filepath.Join(resultRoot, turn.Name()))
			if runReadErr != nil {
				return nil, runReadErr
			}
			for _, run := range runs {
				if err = ctx.Err(); err != nil {
					return nil, err
				}
				runID, runValid := parseDecimalID(run.Name())
				record := ScanRecord{TaskID: taskID, TaskTurnID: turnID, FinalizationRunID: runID}
				if !runValid {
					record.State, record.ReasonCode = ScanPartial, "RESULT_STAGING_DIRECTORY"
				} else if run.Type()&os.ModeSymlink != 0 || !run.IsDir() {
					record.State, record.ReasonCode = ScanCorrupt, "RESULT_DIRECTORY_INVALID"
				} else {
					record.State, record.ReasonCode = w.classifyResultDirectory(record, filepath.Join(resultRoot, turn.Name(), run.Name()))
				}
				records = append(records, record)
			}
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].TaskID != records[j].TaskID {
			return records[i].TaskID < records[j].TaskID
		}
		if records[i].TaskTurnID != records[j].TaskTurnID {
			return records[i].TaskTurnID < records[j].TaskTurnID
		}
		return records[i].FinalizationRunID < records[j].FinalizationRunID
	})
	return records, nil
}

func (w *PersistentWorkspace) classifyResultDirectory(record ScanRecord, root string) (ScanState, string) {
	readyBody, err := readBoundedRegularFile(filepath.Join(root, "READY"), maxWorkspaceManifestBytes)
	if errors.Is(err, os.ErrNotExist) {
		return ScanPartial, "READY_MISSING"
	}
	if err != nil {
		return ScanCorrupt, "READY_INVALID"
	}
	var ready struct {
		SchemaVersion  int    `json:"schemaVersion"`
		Manifest       string `json:"manifest"`
		ManifestDigest string `json:"manifestDigest"`
	}
	if json.Unmarshal(readyBody, &ready) != nil {
		return ScanCorrupt, "READY_INVALID"
	}
	if ready.SchemaVersion != 1 {
		return ScanUnknownSchema, "READY_SCHEMA_UNSUPPORTED"
	}
	if ready.Manifest != "result-manifest.json" || !isHexDigest(ready.ManifestDigest) {
		return ScanCorrupt, "READY_IDENTITY_INVALID"
	}
	manifestBody, err := readBoundedRegularFile(filepath.Join(root, ready.Manifest), maxWorkspaceManifestBytes)
	if err != nil || digestBytes(manifestBody) != ready.ManifestDigest {
		return ScanCorrupt, "RESULT_MANIFEST_DIGEST_MISMATCH"
	}
	var manifest struct {
		SchemaVersion     int    `json:"schemaVersion"`
		TaskID            int64  `json:"taskId,string"`
		TaskTurnID        int64  `json:"taskTurnId,string"`
		FinalizationRunID int64  `json:"finalizationRunId,string"`
		AttemptID         int64  `json:"attemptId,string"`
		ResultCommit      string `json:"resultCommit"`
		ResultTree        string `json:"resultTree"`
		SelectionDigest   string `json:"selectionDigest"`
		Artifacts         []struct {
			Path      string `json:"path"`
			SHA256    string `json:"sha256"`
			SizeBytes int64  `json:"sizeBytes"`
		} `json:"artifacts"`
	}
	if json.Unmarshal(manifestBody, &manifest) != nil {
		return ScanCorrupt, "RESULT_MANIFEST_INVALID"
	}
	if manifest.SchemaVersion != 1 {
		return ScanUnknownSchema, "RESULT_MANIFEST_SCHEMA_UNSUPPORTED"
	}
	if manifest.TaskID != record.TaskID || manifest.TaskTurnID != record.TaskTurnID || manifest.FinalizationRunID != record.FinalizationRunID || manifest.AttemptID < 1 || !isGitObjectID(manifest.ResultCommit) || !isGitObjectID(manifest.ResultTree) || !isHexDigest(manifest.SelectionDigest) || len(manifest.Artifacts) == 0 {
		return ScanCorrupt, "RESULT_MANIFEST_IDENTITY_INVALID"
	}
	for _, artifact := range manifest.Artifacts {
		if filepath.Base(artifact.Path) != artifact.Path || artifact.Path == "" || artifact.SizeBytes < 0 || artifact.SizeBytes > maxWorkspaceArtifactScanSize || !isHexDigest(artifact.SHA256) {
			return ScanCorrupt, "RESULT_ARTIFACT_IDENTITY_INVALID"
		}
		payload, readErr := readBoundedRegularFile(filepath.Join(root, artifact.Path), artifact.SizeBytes)
		if readErr != nil || int64(len(payload)) != artifact.SizeBytes || digestBytes(payload) != artifact.SHA256 {
			return ScanCorrupt, "RESULT_ARTIFACT_DIGEST_MISMATCH"
		}
	}
	return ScanReady, "RESULT_READY"
}

func readBoundedRegularFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > limit {
		return nil, errors.New("WORKSPACE_FILE_INVALID")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil || int64(len(body)) != info.Size() || int64(len(body)) > limit {
		return nil, errors.New("WORKSPACE_FILE_INVALID")
	}
	return body, nil
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func isHexDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func isGitObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func parseDecimalID(value string) (int64, bool) {
	if value == "" || strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") || len(value) > 1 && value[0] == '0' {
		return 0, false
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	return parsed, err == nil && parsed > 0
}

// GCEligibility is a Server-derived decision. The filesystem adapter never
// infers task terminal state or active references from directory contents.
type GCEligibility struct {
	TaskID                 int64
	Terminal               bool
	TerminalAt             time.Time
	ActiveReferences       bool
	PortableStateAvailable bool
	ReasonCode             string
}

type GCAuthority interface {
	Eligibility(context.Context, int64) (GCEligibility, error)
}

type GCAudit struct {
	TaskID      int64
	Disposition string
	ReasonCode  string
}

type GCReport struct {
	Deleted int
	Skipped int
}

// GarbageCollect removes only a task's direct workspaceRoot child after the
// Server has proved it is terminal, older than seven days, has no active
// references, and retains its portable state. Query failures, symlinks and
// concurrent filesystem surprises fail closed and are emitted as safe audit
// facts. Repeating a completed collection is a no-op.
func (w *PersistentWorkspace) GarbageCollect(ctx context.Context, authority GCAuthority, audit func(GCAudit)) (GCReport, error) {
	if authority == nil {
		return GCReport{}, errors.New("WORKSPACE_GC_AUTHORITY_REQUIRED")
	}
	entries, err := os.ReadDir(w.root)
	if err != nil {
		return GCReport{}, err
	}
	report := GCReport{}
	cutoff := w.now().UTC().Add(-workspaceRetention)
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return report, err
		}
		taskID, valid := parseDecimalID(entry.Name())
		if !valid {
			continue
		}
		taskRoot := filepath.Join(w.root, entry.Name())
		skip := func(reason string) {
			report.Skipped++
			if audit != nil {
				audit(GCAudit{TaskID: taskID, Disposition: "SKIPPED", ReasonCode: reason})
			}
		}
		info, statErr := os.Lstat(taskRoot)
		if statErr != nil {
			if !errors.Is(statErr, os.ErrNotExist) {
				skip("TASK_ROOT_UNAVAILABLE")
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || w.assertContained(taskRoot) != nil {
			skip("TASK_ROOT_ESCAPE")
			continue
		}
		eligibility, eligibilityErr := authority.Eligibility(ctx, taskID)
		if eligibilityErr != nil || eligibility.TaskID != taskID {
			skip("AUTHORITY_UNAVAILABLE")
			continue
		}
		if !eligibility.Terminal {
			skip(firstNonEmpty(eligibility.ReasonCode, "TASK_NOT_TERMINAL"))
			continue
		}
		if eligibility.TerminalAt.IsZero() || eligibility.TerminalAt.After(cutoff) {
			skip(firstNonEmpty(eligibility.ReasonCode, "RETENTION_NOT_EXPIRED"))
			continue
		}
		if eligibility.ActiveReferences {
			skip(firstNonEmpty(eligibility.ReasonCode, "ACTIVE_REFERENCE"))
			continue
		}
		if !eligibility.PortableStateAvailable {
			skip(firstNonEmpty(eligibility.ReasonCode, "PORTABLE_STATE_UNAVAILABLE"))
			continue
		}
		if err = rejectSymlinkTree(taskRoot); err != nil {
			skip("TASK_ROOT_SYMLINK")
			continue
		}
		// Recheck the direct child immediately before the atomic quarantine
		// rename. This makes concurrent collectors safe: exactly one can move the
		// numeric Task directory, and RemoveAll only sees the private quarantine
		// path. RemoveAll never follows a symlink.
		info, statErr = os.Lstat(taskRoot)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			skip("TASK_ROOT_CHANGED")
			continue
		}
		quarantine, quarantineErr := os.MkdirTemp(w.root, ".gc-"+entry.Name()+"-")
		if quarantineErr != nil {
			skip("TASK_ROOT_QUARANTINE_UNAVAILABLE")
			continue
		}
		if quarantineErr = os.Remove(quarantine); quarantineErr != nil {
			skip("TASK_ROOT_QUARANTINE_UNAVAILABLE")
			continue
		}
		if err = os.Rename(taskRoot, quarantine); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				skip("TASK_ROOT_CHANGED")
			}
			continue
		}
		if err = syncWorkspaceDirectory(w.root); err != nil {
			skip("TASK_ROOT_QUARANTINE_SYNC_FAILED")
			continue
		}
		if err = os.RemoveAll(quarantine); err != nil {
			skip("TASK_ROOT_QUARANTINE_REMOVE_FAILED")
			continue
		}
		if err = syncWorkspaceDirectory(w.root); err != nil {
			skip("TASK_ROOT_REMOVE_SYNC_FAILED")
			continue
		}
		report.Deleted++
		if audit != nil {
			audit(GCAudit{TaskID: taskID, Disposition: "DELETED", ReasonCode: "RETENTION_EXPIRED"})
		}
	}
	return report, nil
}

func rejectSymlinkTree(root string) error {
	return filepath.WalkDir(root, func(_ string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("WORKSPACE_SYMLINK_PRESENT")
		}
		return nil
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "UNKNOWN"
}
