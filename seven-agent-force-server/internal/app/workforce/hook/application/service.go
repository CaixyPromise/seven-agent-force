package application

import (
	"bytes"
	"context"
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
	"syscall"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/hook/infrastructure"
	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
)

type Service struct {
	store     *infrastructure.Store
	artifacts *artifactstore.Store
}

func NewService(store *infrastructure.Store, artifacts *artifactstore.Store) *Service {
	return &Service{store: store, artifacts: artifacts}
}

type CreatePackageCommand struct {
	Name        string              `json:"name"`
	Category    string              `json:"category"`
	Description string              `json:"description"`
	Manifest    domain.Manifest     `json:"manifest"`
	Files       []domain.SourceFile `json:"files"`
}

func (s *Service) CreatePackage(ctx context.Context, tenant int64, actor int64, command CreatePackageCommand) (infrastructure.Package, error) {
	command.Name = strings.TrimSpace(command.Name)
	command.Category = strings.TrimSpace(command.Category)
	if command.Name == "" || len(command.Name) > 128 {
		return infrastructure.Package{}, fmt.Errorf("name is required and must be at most 128 characters")
	}
	if command.Category == "" {
		command.Category = "其他"
	}
	if command.Manifest.SchemaVersion == "" {
		command.Manifest = DefaultManifest(command.Name, "node")
	}
	return s.store.CreatePackage(ctx, tenant, actor, command.Name, command.Category, strings.TrimSpace(command.Description), command.Manifest, command.Files)
}
func DefaultManifest(name, runtime string) domain.Manifest {
	entryPath := "hooks/pre-tool-use.mjs"
	requirement := ">=22 <23"
	if runtime == "shell" {
		entryPath = "hooks/attempt-prepare.sh"
		requirement = ""
	}
	return domain.Manifest{SchemaVersion: domain.PackageSchema, Name: name, TrustClass: "PLATFORM_TRUSTED", Runtime: domain.Runtime{Kind: runtime, Requirement: requirement}, Entrypoints: []domain.Entrypoint{{Event: "codex.PreToolUse", Path: entryPath, TimeoutMS: domain.DefaultTimeoutMS, FailureMode: "BLOCK"}}, Limits: domain.Limits{StdinBytes: domain.MaxInputBytes, StdoutBytes: domain.MaxOutputBytes, StderrBytes: domain.MaxErrorBytes}}
}
func (s *Service) ListPackages(ctx context.Context, tenant int64) ([]infrastructure.Package, error) {
	return s.store.ListPackages(ctx, tenant)
}
func (s *Service) GetPackage(ctx context.Context, tenant int64, id int64) (infrastructure.Package, error) {
	return s.store.GetPackage(ctx, tenant, id)
}
func (s *Service) CreateRevision(ctx context.Context, tenant int64, packageID int64, actor int64) (infrastructure.Package, error) {
	return s.store.CreateRevision(ctx, tenant, packageID, actor)
}
func (s *Service) UpdateRevision(ctx context.Context, tenant int64, id int64, expected int64, manifest domain.Manifest, files []domain.SourceFile) (infrastructure.Package, error) {
	if expected < 1 {
		return infrastructure.Package{}, fmt.Errorf("expectedRevision must be positive")
	}
	return s.store.UpdateRevision(ctx, tenant, id, expected, manifest, files)
}
func (s *Service) ImportArchive(ctx context.Context, tenant int64, id int64, expected int64, body []byte) (infrastructure.Package, error) {
	manifest, files, err := domain.ImportArchive(body)
	if err != nil {
		return infrastructure.Package{}, err
	}
	return s.store.UpdateRevision(ctx, tenant, id, expected, manifest, files)
}
func (s *Service) Validate(ctx context.Context, tenant int64, id int64) (domain.Validation, error) {
	revision, err := s.store.Revision(ctx, tenant, id)
	if err != nil {
		return domain.Validation{}, err
	}
	result := domain.Validate(revision.Manifest, revision.Files)
	if err := s.store.SetValidation(ctx, tenant, id, result); err != nil {
		return domain.Validation{}, err
	}
	return result, nil
}

type TestCommand struct {
	Event   string         `json:"event"`
	Fixture map[string]any `json:"fixture"`
}
type TestResult struct {
	ID              int64            `json:"id,string"`
	SchemaVersion   int              `json:"schemaVersion"`
	Decision        string           `json:"decision"`
	ReasonCode      string           `json:"reasonCode"`
	OperatorMessage string           `json:"operatorMessage"`
	Evidence        []map[string]any `json:"evidence"`
	Diagnostics     string           `json:"diagnostics"`
	DurationMS      int64            `json:"durationMs"`
}

func (s *Service) Test(ctx context.Context, tenant int64, id int64, actor int64, command TestCommand) (TestResult, error) {
	revision, err := s.store.Revision(ctx, tenant, id)
	if err != nil {
		return TestResult{}, err
	}
	validation := domain.Validate(revision.Manifest, revision.Files)
	if !validation.Valid {
		return TestResult{}, fmt.Errorf("revision is invalid: %s", strings.Join(validation.Errors, "; "))
	}
	entry, ok := entrypointFor(revision.Manifest, command.Event)
	if !ok {
		return TestResult{}, fmt.Errorf("event is not declared by this package")
	}
	body, err := jsonMarshal(command.Fixture)
	if err != nil {
		return TestResult{}, err
	}
	started := time.Now()
	result, diagnostics, err := executeFixture(ctx, revision.Manifest.Runtime.Kind, entry, revision.Files, body)
	duration := time.Since(started).Milliseconds()
	if err != nil {
		return TestResult{}, err
	}
	runID, err := s.store.CreateTestRun(ctx, tenant, id, actor, command.Event, command.Fixture, result, diagnostics, duration)
	if err != nil {
		return TestResult{}, err
	}
	return TestResult{
		ID: runID, SchemaVersion: result.SchemaVersion, Decision: result.Decision,
		ReasonCode: result.ReasonCode, OperatorMessage: result.OperatorMessage,
		Evidence: result.Evidence, Diagnostics: diagnostics, DurationMS: duration,
	}, nil
}

func (s *Service) Publish(ctx context.Context, tenant int64, id int64, actor int64, version string) (infrastructure.Release, error) {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > 64 {
		return infrastructure.Release{}, fmt.Errorf("version is required and must be at most 64 characters")
	}
	revision, err := s.store.Revision(ctx, tenant, id)
	if err != nil {
		return infrastructure.Release{}, err
	}
	manifest := revision.Manifest
	manifest.Version = version
	archive, digest, err := domain.CanonicalArchive(manifest, revision.Files)
	if err != nil {
		return infrastructure.Release{}, err
	}
	key := fmt.Sprintf("managed-hooks/packages/%s/%d/%s/%s.tar.gz", sanitize(strconv.FormatInt(tenant, 10)), revision.ID, sanitize(version), digest)
	if err := s.artifacts.Put(ctx, key, "application/gzip", bytes.NewReader(archive), int64(len(archive))); err != nil {
		return infrastructure.Release{}, err
	}
	object, err := s.artifacts.Get(ctx, key)
	if err != nil {
		return infrastructure.Release{}, err
	}
	defer object.Close()
	actual := sha256.New()
	size, err := io.Copy(actual, io.LimitReader(object, domain.MaxArchiveBytes+1))
	if err != nil || size != int64(len(archive)) || hex.EncodeToString(actual.Sum(nil)) != digest {
		return infrastructure.Release{}, errors.New("published hook archive failed byte-for-byte verification")
	}
	return s.store.Publish(ctx, tenant, id, actor, version, key, digest, int64(len(archive)))
}
func (s *Service) ListReleases(ctx context.Context, tenant int64) ([]infrastructure.Release, error) {
	return s.store.ListReleases(ctx, tenant)
}
func (s *Service) ListAudit(ctx context.Context, tenant int64) ([]infrastructure.AuditEvent, error) {
	return s.store.ListAudit(ctx, tenant)
}

func (s *Service) ListHarnessChannels(ctx context.Context) ([]infrastructure.HarnessChannel, error) {
	return s.store.ListHarnessChannels(ctx)
}

func entrypointFor(manifest domain.Manifest, event string) (domain.Entrypoint, bool) {
	for _, entry := range manifest.Entrypoints {
		if entry.Event == event {
			return entry, true
		}
	}
	return domain.Entrypoint{}, false
}
func executeFixture(parent context.Context, runtime string, entry domain.Entrypoint, files []domain.SourceFile, input []byte) (domain.HookResult, string, error) {
	root, err := os.MkdirTemp("", "seven-hook-test-")
	if err != nil {
		return domain.HookResult{}, "", err
	}
	defer os.RemoveAll(root)
	for _, file := range files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return domain.HookResult{}, "", err
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			return domain.HookResult{}, "", err
		}
	}
	timeout := time.Duration(entry.TimeoutMS) * time.Millisecond
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	var command *exec.Cmd
	if runtime == "node" {
		command = exec.CommandContext(ctx, "node", filepath.Join(root, filepath.FromSlash(entry.Path)))
	} else {
		command = exec.CommandContext(ctx, "/bin/sh", filepath.Join(root, filepath.FromSlash(entry.Path)))
	}
	command.Dir = root
	command.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "HOME=/nonexistent"}
	command.Stdin = strings.NewReader(string(input))
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr boundedBuffer
	stdout.max = domain.MaxOutputBytes
	stderr.max = domain.MaxErrorBytes
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
		return domain.HookResult{}, stderr.String(), fmt.Errorf("hook fixture failed: %w", err)
	}
	result, err := domain.ValidateResult(stdout.Bytes())
	if err != nil {
		return domain.HookResult{}, stderr.String(), err
	}
	return result, stderr.String(), nil
}

type boundedBuffer struct {
	data []byte
	max  int64
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if int64(len(b.data)+len(p)) > b.max {
		return 0, errors.New("hook output exceeded protocol limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *boundedBuffer) Bytes() []byte  { return b.data }
func (b *boundedBuffer) String() string { return string(b.data) }
func sanitize(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '-'
	}, value)
}

// indirection keeps large fixture maps bounded and testable without exposing JSON implementation details.
func jsonMarshal(value any) ([]byte, error) { return json.Marshal(value) }
