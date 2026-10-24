package checkpoint

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"

	processordomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"golang.org/x/text/unicode/norm"
)

const SelectionSchemaVersion = 1

var platformRegenerable = map[string]struct{}{
	"node_modules": {}, "target": {}, "__pycache__": {}, ".pytest_cache": {}, ".mypy_cache": {}, ".ruff_cache": {},
	".venv": {}, "venv": {}, "dist": {}, "build": {}, ".next": {}, ".nuxt": {}, "coverage": {}, ".gradle": {},
	"bin": {}, "obj": {}, "tmp": {}, "temp": {},
}

var platformProtectedRoots = map[string]struct{}{
	".seven": {}, ".codex": {},
}

type SelectionInput struct {
	RepoRoot             string
	BaseCommit           string
	Outputs              []processordomain.Output
	ContinuationIncludes []string
}

type SelectionEntry struct {
	Path       string `json:"path"`
	Type       string `json:"type"`
	GitMode    string `json:"gitMode"`
	SizeBytes  int64  `json:"sizeBytes"`
	SHA256     string `json:"sha256,omitempty"`
	Source     string `json:"source"`
	ReasonCode string `json:"reasonCode"`
}

type SelectionIndex []SelectionEntry

func (i SelectionIndex) Contains(path string) bool {
	for _, item := range i {
		if item.Path == path {
			return true
		}
	}
	return false
}
func (i SelectionIndex) Reason(path string) string {
	for _, item := range i {
		if item.Path == path {
			return item.ReasonCode
		}
	}
	return ""
}

type ResultFileSelection struct {
	SchemaVersion      int                       `json:"schemaVersion"`
	ResultTreeIndex    SelectionIndex            `json:"resultTreeIndex"`
	OverlayIndex       SelectionIndex            `json:"overlayIndex"`
	OutputIndexes      map[string]SelectionIndex `json:"outputIndexes"`
	ExcludedIndex      SelectionIndex            `json:"excludedIndex"`
	SecurityViolations []string                  `json:"securityViolations"`
	GitignoreDigest    string                    `json:"gitignoreDigest"`
	PolicyDigest       string                    `json:"platformIgnorePolicyDigest"`
	SelectionDigest    string                    `json:"selectionDigest"`
}

func SelectResult(ctx context.Context, input SelectionInput) (ResultFileSelection, error) {
	root, err := filepath.Abs(input.RepoRoot)
	if err != nil {
		return ResultFileSelection{}, err
	}
	if input.BaseCommit == "" {
		return ResultFileSelection{}, errors.New("RESULT_SELECTION_BASE_REQUIRED")
	}
	controlled, cleanup, err := newControlledGit(ctx, root, input.BaseCommit)
	if err != nil {
		return ResultFileSelection{}, err
	}
	defer cleanup()
	tracked, err := controlled.tracked(ctx)
	if err != nil {
		return ResultFileSelection{}, err
	}
	result := ResultFileSelection{SchemaVersion: SelectionSchemaVersion, OutputIndexes: map[string]SelectionIndex{}, PolicyDigest: platformPolicyDigest()}
	paths, violations, err := collectWorkspaceFiles(root, tracked)
	if err != nil {
		return result, err
	}
	result.SecurityViolations = append(result.SecurityViolations, violations...)
	for path, mode := range tracked {
		if mode == "160000" {
			result.SecurityViolations = append(result.SecurityViolations, "RESULT_SUBMODULE_FORBIDDEN:"+path)
			continue
		}
		if mode == "120000" {
			result.SecurityViolations = append(result.SecurityViolations, "RESULT_SYMLINK_FORBIDDEN:"+path)
			continue
		}
		lfs, attrErr := controlled.usesLFS(ctx, path)
		if attrErr != nil {
			return result, attrErr
		}
		if lfs {
			result.SecurityViolations = append(result.SecurityViolations, "RESULT_GIT_LFS_FORBIDDEN:"+path)
			continue
		}
		entry, exists := paths[path]
		if !exists {
			result.ResultTreeIndex = append(result.ResultTreeIndex, SelectionEntry{Path: path, Type: "DELETED", GitMode: mode, Source: "TRACKED", ReasonCode: "TRACKED_DELETED"})
			continue
		}
		entry.GitMode, entry.Source, entry.ReasonCode = mode, "TRACKED", "TRACKED"
		result.ResultTreeIndex = append(result.ResultTreeIndex, entry)
		delete(paths, path)
	}
	outputMatchers, err := compileOutputMatchers(input.Outputs)
	if err != nil {
		return result, err
	}
	continuationMatchers, err := compilePatterns(input.ContinuationIncludes)
	if err != nil {
		return result, err
	}
	remaining := sortedEntryKeys(paths)
	for _, path := range remaining {
		entry := paths[path]
		ignored, checkErr := controlled.ignored(ctx, path)
		if checkErr != nil {
			return result, checkErr
		}
		regenerable := isRegenerable(path)
		matchedOutput := false
		for name, matchers := range outputMatchers {
			if matchesAny(path, matchers.includes) && !matchesAny(path, matchers.excludes) {
				copy := entry
				copy.Source, copy.ReasonCode = "DECLARED_OUTPUT", "DECLARED_OUTPUT"
				result.OutputIndexes[name] = append(result.OutputIndexes[name], copy)
				matchedOutput = true
			}
		}
		matchedContinuation := matchesAny(path, continuationMatchers)
		if ignored || regenerable {
			if matchedOutput || matchedContinuation {
				copy := entry
				copy.Source, copy.ReasonCode = "OVERLAY", chooseSelectionReason(matchedContinuation, "CONTINUATION_INCLUDE", "DECLARED_OUTPUT")
				result.OverlayIndex = append(result.OverlayIndex, copy)
			} else {
				entry.Source = "UNTRACKED"
				entry.ReasonCode = chooseSelectionReason(regenerable, "PLATFORM_REGENERABLE", "GITIGNORED")
				result.ExcludedIndex = append(result.ExcludedIndex, entry)
			}
			continue
		}
		entry.Source, entry.ReasonCode = "UNTRACKED", "UNTRACKED_INCLUDED"
		result.ResultTreeIndex = append(result.ResultTreeIndex, entry)
	}
	result.GitignoreDigest, err = repositoryGitignoreDigest(root)
	if err != nil {
		return result, err
	}
	sortSelection(&result)
	if len(result.SecurityViolations) > 0 {
		return result, errors.New(strings.Join(result.SecurityViolations, ";"))
	}
	canonical := result
	canonical.SelectionDigest = ""
	body, _ := json.Marshal(canonical)
	result.SelectionDigest = sha256String(body)
	return result, nil
}

func (g *controlledGit) usesLFS(ctx context.Context, path string) (bool, error) {
	body, err := g.output(ctx, "check-attr", "filter", "--", path)
	if err != nil {
		return false, err
	}
	parts := strings.Split(strings.TrimSpace(body), ":")
	return len(parts) >= 3 && strings.TrimSpace(parts[len(parts)-1]) == "lfs", nil
}

type controlledGit struct {
	root, dir, index string
	env              []string
}

func newControlledGit(ctx context.Context, root, base string) (*controlledGit, func(), error) {
	resolvedBase, err := gitOutputControlled(ctx, root, nil, "rev-parse", "--verify", base+"^{commit}")
	if err != nil {
		return nil, func() {}, err
	}
	dir, err := os.MkdirTemp("", "seven-selection-git-")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	gitDir := filepath.Join(dir, "git")
	if output, runErr := exec.CommandContext(ctx, "git", "init", "--bare", "-q", gitDir).CombinedOutput(); runErr != nil {
		cleanup()
		return nil, func() {}, fmt.Errorf("RESULT_SELECTION_GIT_INIT_FAILED: %w: %s", runErr, output)
	}
	objectDir, err := gitOutputControlled(ctx, root, nil, "rev-parse", "--path-format=absolute", "--git-path", "objects")
	if err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if err = os.MkdirAll(filepath.Join(gitDir, "objects", "info"), 0o700); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	if err = os.WriteFile(filepath.Join(gitDir, "objects", "info", "alternates"), []byte(objectDir+"\n"), 0o600); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	index := filepath.Join(dir, "index")
	env := append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "global.gitconfig"), "HOME="+dir, "XDG_CONFIG_HOME="+dir, "GIT_DIR="+gitDir, "GIT_WORK_TREE="+root, "GIT_INDEX_FILE="+index, "GIT_ATTR_NOSYSTEM=1")
	controlled := &controlledGit{root: root, dir: gitDir, index: index, env: env}
	if _, err = controlled.output(ctx, "read-tree", resolvedBase); err != nil {
		cleanup()
		return nil, func() {}, err
	}
	return controlled, cleanup, nil
}

func (g *controlledGit) tracked(ctx context.Context) (map[string]string, error) {
	body, err := g.output(ctx, "ls-files", "-s", "-z")
	if err != nil {
		return nil, err
	}
	result := map[string]string{}
	for _, record := range bytes.Split([]byte(body), []byte{0}) {
		if len(record) == 0 {
			continue
		}
		fields := bytes.Fields(record)
		if len(fields) < 4 {
			return nil, errors.New("RESULT_SELECTION_INDEX_INVALID")
		}
		path := string(bytes.Join(fields[3:], []byte(" ")))
		result[path] = string(fields[0])
	}
	return result, nil
}

func (g *controlledGit) ignored(ctx context.Context, path string) (bool, error) {
	command := exec.CommandContext(ctx, "git", "check-ignore", "--no-index", "-q", "--", path)
	command.Env, command.Dir = g.env, g.root
	err := command.Run()
	if err == nil {
		return true, nil
	}
	if exit := new(exec.ExitError); errors.As(err, &exit) && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("RESULT_SELECTION_GITIGNORE_FAILED: %w", err)
}

func (g *controlledGit) output(ctx context.Context, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", args...)
	command.Env, command.Dir = g.env, g.root
	body, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("RESULT_SELECTION_GIT_FAILED: git %s: %w: %s", strings.Join(args, " "), err, body)
	}
	return string(body), nil
}

func collectWorkspaceFiles(root string, tracked map[string]string) (map[string]SelectionEntry, []string, error) {
	result := map[string]SelectionEntry{}
	violations := []string{}
	normalized := map[string]string{}
	folded := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if isProtectedResultPath(rel) {
			if entry.IsDir() {
				violations = append(violations, "RESULT_PROTECTED_PATH_FORBIDDEN:"+rel)
				return filepath.SkipDir
			}
			violations = append(violations, "RESULT_PROTECTED_PATH_FORBIDDEN:"+rel)
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if _, trackedPath := tracked[rel]; !trackedPath && isRegenerable(rel) {
				return nil
			}
			violations = append(violations, "RESULT_SYMLINK_FORBIDDEN:"+rel)
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode()&(os.ModeSetuid|os.ModeSetgid) != 0 {
			violations = append(violations, "RESULT_FILE_TYPE_FORBIDDEN:"+rel)
			return nil
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Nlink > 1 {
			violations = append(violations, "RESULT_HARDLINK_FORBIDDEN:"+rel)
			return nil
		}
		nfc, fold := norm.NFC.String(rel), strings.ToLower(norm.NFC.String(rel))
		if prior, ok := normalized[nfc]; ok && prior != rel {
			violations = append(violations, "RESULT_UNICODE_PATH_CONFLICT:"+prior+":"+rel)
		}
		if prior, ok := folded[fold]; ok && prior != rel {
			violations = append(violations, "RESULT_CASE_PATH_CONFLICT:"+prior+":"+rel)
		}
		normalized[nfc], folded[fold] = rel, rel
		sha, err := fileDigest(path)
		if err != nil {
			return err
		}
		mode := "100644"
		if info.Mode()&0o111 != 0 {
			mode = "100755"
		}
		result[rel] = SelectionEntry{Path: rel, Type: "FILE", GitMode: mode, SizeBytes: info.Size(), SHA256: sha}
		return nil
	})
	return result, violations, err
}

type matchSet struct{ includes, excludes []*regexp.Regexp }

func compileOutputMatchers(outputs []processordomain.Output) (map[string]matchSet, error) {
	result := map[string]matchSet{}
	for _, output := range outputs {
		patterns := []string{output.Path}
		excludes := []string{}
		if output.Kind == "ARCHIVE" {
			patterns, excludes = output.Selection.Includes, output.Selection.Excludes
		}
		included, err := compilePatterns(patterns)
		if err != nil {
			return nil, err
		}
		excluded, err := compilePatterns(excludes)
		if err != nil {
			return nil, err
		}
		result[output.Name] = matchSet{included, excluded}
	}
	return result, nil
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	result := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		quoted := regexp.QuoteMeta(filepath.ToSlash(pattern))
		quoted = strings.ReplaceAll(quoted, `\*\*`, `.*`)
		quoted = strings.ReplaceAll(quoted, `\*`, `[^/]*`)
		quoted = strings.ReplaceAll(quoted, `\?`, `[^/]`)
		compiled, err := regexp.Compile("^" + quoted + "$")
		if err != nil {
			return nil, err
		}
		result = append(result, compiled)
	}
	return result, nil
}
func matchesAny(path string, matchers []*regexp.Regexp) bool {
	for _, matcher := range matchers {
		if matcher.MatchString(path) {
			return true
		}
	}
	return false
}
func isRegenerable(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if _, ok := platformRegenerable[part]; ok {
			return true
		}
	}
	return false
}
func isProtectedResultPath(path string) bool {
	first := strings.Split(filepath.ToSlash(path), "/")[0]
	_, ok := platformProtectedRoots[first]
	return ok
}
func sortedEntryKeys(values map[string]SelectionEntry) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
func chooseSelectionReason(condition bool, yes, no string) string {
	if condition {
		return yes
	}
	return no
}
func sortSelection(result *ResultFileSelection) {
	sort.Slice(result.ResultTreeIndex, func(i, j int) bool { return result.ResultTreeIndex[i].Path < result.ResultTreeIndex[j].Path })
	sort.Slice(result.OverlayIndex, func(i, j int) bool { return result.OverlayIndex[i].Path < result.OverlayIndex[j].Path })
	sort.Slice(result.ExcludedIndex, func(i, j int) bool { return result.ExcludedIndex[i].Path < result.ExcludedIndex[j].Path })
	sort.Strings(result.SecurityViolations)
	for name, index := range result.OutputIndexes {
		sort.Slice(index, func(i, j int) bool { return index[i].Path < index[j].Path })
		result.OutputIndexes[name] = index
	}
}
func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func sha256String(body []byte) string { sum := sha256.Sum256(body); return hex.EncodeToString(sum[:]) }
func platformPolicyDigest() string {
	keys := make([]string, 0, len(platformRegenerable))
	for key := range platformRegenerable {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return sha256String([]byte(strings.Join(keys, "\n")))
}
func repositoryGitignoreDigest(root string) (string, error) {
	paths := []string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() && entry.Name() == ".gitignore" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		rel, _ := filepath.Rel(root, path)
		body, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(hash, "%s\x00%d\x00", filepath.ToSlash(rel), len(body))
		hash.Write(body)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func gitOutputControlled(ctx context.Context, repo string, env []string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	if env != nil {
		command.Env = env
	}
	body, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, body)
	}
	return strings.TrimSpace(string(body)), nil
}
