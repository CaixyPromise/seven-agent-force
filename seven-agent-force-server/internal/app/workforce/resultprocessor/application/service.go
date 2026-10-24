// Package application coordinates Result Processor authoring, immutable
// publication, exact-image fixtures and Task-time resolution. Runtime
// execution remains outside this module: the Supervisor owns process(context)
// and the Result Coordinator owns trusted result materialization.
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
	"strconv"
	"strings"

	artifactstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/infrastructure/artifact"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
)

const (
	maxProcessorArchiveReadback = 8 << 20
)

// Service is the single application boundary for Result Processor management.
// Its Store implements facade.Resolver, so Task creation depends on the small
// immutable execution contract rather than this authoring implementation.
type Service struct {
	store         *infrastructure.Store
	artifacts     *artifactstore.Store
	outputStore   availableOutputRepository
	outputObjects outputObjectReader
}

func NewService(store *infrastructure.Store, artifacts *artifactstore.Store) *Service {
	return &Service{
		store:         store,
		artifacts:     artifacts,
		outputStore:   store,
		outputObjects: artifactOutputReader{store: artifacts},
	}
}

func (s *Service) Resolver() facade.Resolver { return s.store }

// SetEnvironmentValueResolver wires the Framework-owned CONFIG/SECRET
// boundary without letting Result Processor persistence learn about ciphertext
// or a second secret store.
func (s *Service) SetEnvironmentValueResolver(resolver infrastructure.EnvironmentValueResolver) {
	if s != nil && s.store != nil {
		s.store.SetEnvironmentValueResolver(resolver)
	}
}

// RegisterPersistedSecretReferences is called once after the Framework
// ConfigFacade has been composed. This restores registrations for immutable
// releases created before a Server restart without loading a secret value.
func (s *Service) RegisterPersistedSecretReferences(ctx context.Context) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("result processor store is unavailable")
	}
	return s.store.RegisterPersistedSecretReferences(ctx)
}

// MaterializeEvaluationEnvironment returns one cycle-scoped environment to an
// authenticated Worker. The caller must keep it out of logs and Docker env.
func (s *Service) MaterializeEvaluationEnvironment(ctx context.Context, identity facade.MachineIdentity, attemptID int64, leaseToken string, cycleID int64, cycleNumber int) (facade.Materialization, error) {
	return s.store.MaterializeEvaluationEnvironment(ctx, identity, attemptID, leaseToken, cycleID, cycleNumber)
}

// PersistEvaluation commits the Processor decision before the Supervisor may
// resume the Session or write a CompletionProposal.
func (s *Service) PersistEvaluation(ctx context.Context, identity facade.MachineIdentity, attemptID int64, leaseToken string, candidate facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
	return s.store.PersistEvaluation(ctx, identity, attemptID, leaseToken, candidate)
}

// RecordContinuationCheckpoint accepts a Worker-side, byte-for-byte verified
// checkpoint after an EvaluationCycle.  The Worker owns object I/O; this
// application boundary owns the execution fencing and durable identity.
func (s *Service) RecordContinuationCheckpoint(ctx context.Context, identity facade.MachineIdentity, attemptID int64, leaseToken string, candidate facade.ContinuationCheckpointCandidate) (facade.ContinuationCheckpointView, error) {
	return s.store.RecordContinuationCheckpoint(ctx, identity, attemptID, leaseToken, candidate)
}

func (s *Service) ListEvaluations(ctx context.Context, tenantID int64, taskID int64) ([]facade.EvaluationView, error) {
	return s.store.ListEvaluations(ctx, tenantID, taskID)
}

func (s *Service) ListContinuationCheckpoints(ctx context.Context, tenantID int64, taskID int64) ([]facade.ContinuationCheckpointView, error) {
	return s.store.ListContinuationCheckpoints(ctx, tenantID, taskID)
}

func (s *Service) ListResultOutputs(ctx context.Context, tenantID int64, taskID int64) ([]facade.ResultOutputView, error) {
	return s.store.ListResultOutputs(ctx, tenantID, taskID)
}

type CreateDraftCommand struct {
	Name        string                 `json:"name"`
	Category    string                 `json:"category"`
	Description string                 `json:"description"`
	Manifest    processor.Manifest     `json:"manifest"`
	Files       []processor.SourceFile `json:"files"`
}

func (s *Service) CreateDraft(ctx context.Context, tenantID int64, actor int64, command CreateDraftCommand) (infrastructure.Processor, error) {
	command.Name = strings.TrimSpace(command.Name)
	command.Category = strings.TrimSpace(command.Category)
	if command.Name == "" || len(command.Name) > 128 {
		return infrastructure.Processor{}, fmt.Errorf("name is required and must be at most 128 characters")
	}
	if command.Category == "" {
		command.Category = "通用"
	}
	if command.Manifest.SchemaVersion == "" {
		command.Manifest = processor.DefaultManifest(command.Name)
	}
	return s.store.CreateProcessor(ctx, tenantID, actor, command.Name, command.Category, strings.TrimSpace(command.Description), command.Manifest, command.Files)
}

func (s *Service) ListProcessors(ctx context.Context, tenantID int64) ([]infrastructure.Processor, error) {
	return s.store.ListProcessors(ctx, tenantID)
}

func (s *Service) GetProcessor(ctx context.Context, tenantID int64, processorID int64) (infrastructure.Processor, error) {
	return s.store.GetProcessor(ctx, tenantID, processorID)
}

func (s *Service) CreateRevision(ctx context.Context, tenantID int64, processorID int64, actor int64) (infrastructure.Revision, error) {
	return s.store.CreateRevision(ctx, tenantID, processorID, actor)
}

func (s *Service) Revision(ctx context.Context, tenantID int64, revisionID int64) (infrastructure.Revision, error) {
	return s.store.Revision(ctx, tenantID, revisionID)
}

func (s *Service) UpdateDraft(ctx context.Context, tenantID int64, revisionID int64, expectedRevision int64, manifest processor.Manifest, files []processor.SourceFile) (infrastructure.Revision, error) {
	if expectedRevision < 1 {
		return infrastructure.Revision{}, fmt.Errorf("expectedRevision must be positive")
	}
	return s.store.UpdateRevision(ctx, tenantID, revisionID, expectedRevision, manifest, files)
}

func (s *Service) ImportArchive(ctx context.Context, tenantID int64, revisionID int64, expectedRevision int64, archive []byte) (infrastructure.Revision, error) {
	manifest, files, err := processor.ImportArchive(archive)
	if err != nil {
		return infrastructure.Revision{}, err
	}
	return s.UpdateDraft(ctx, tenantID, revisionID, expectedRevision, manifest, files)
}

func (s *Service) ValidateDraft(ctx context.Context, tenantID int64, revisionID int64) (processor.Validation, error) {
	revision, err := s.store.Revision(ctx, tenantID, revisionID)
	if err != nil {
		return processor.Validation{}, err
	}
	validation := processor.Validate(revision.Manifest, revision.Files)
	if err := s.store.SetValidation(ctx, tenantID, revision.ID, validation); err != nil {
		return processor.Validation{}, err
	}
	return validation, nil
}

type FixtureCommand struct {
	HarnessReleaseID  int64           `json:"harnessReleaseId,string"`
	EvaluationContext json.RawMessage `json:"evaluationContext"`
	WorkspaceFiles    json.RawMessage `json:"workspaceFiles"`
	ConfigOverrides   json.RawMessage `json:"configOverrides"`
	ExpectedDecision  string          `json:"expectedDecision,omitempty"`
}

func (s *Service) RequestFixtureRun(ctx context.Context, tenantID int64, revisionID int64, actor int64, command FixtureCommand) (infrastructure.FixtureRun, infrastructure.FixtureMessage, error) {
	command.ExpectedDecision = strings.ToUpper(strings.TrimSpace(command.ExpectedDecision))
	if command.HarnessReleaseID == 0 {
		return infrastructure.FixtureRun{}, infrastructure.FixtureMessage{}, fmt.Errorf("harnessReleaseId is required")
	}
	if command.ExpectedDecision != "" && !processor.IsDecision(command.ExpectedDecision) {
		return infrastructure.FixtureRun{}, infrastructure.FixtureMessage{}, fmt.Errorf("expectedDecision is not supported")
	}
	return s.store.CreateFixtureRun(ctx, tenantID, revisionID, command.HarnessReleaseID, command.ExpectedDecision, actor, defaultJSONObject(command.EvaluationContext), defaultJSONArray(command.WorkspaceFiles), defaultJSONObject(command.ConfigOverrides))
}

func (s *Service) FixtureRun(ctx context.Context, tenantID int64, runID int64) (infrastructure.FixtureRun, error) {
	return s.store.FixtureRun(ctx, tenantID, runID)
}

// ClaimFixtureRun and RecordFixtureResult are Machine-facing only. They keep
// authoring APIs out of the Worker execution path while preserving a small,
// exact-image contract for an unpublished Revision fixture.
func (s *Service) ClaimFixtureRun(ctx context.Context, identity facade.MachineIdentity, runID int64) (facade.FixtureExecution, error) {
	return s.store.ClaimFixtureRun(ctx, identity, runID)
}

func (s *Service) RecordFixtureResult(ctx context.Context, identity facade.MachineIdentity, runID int64, result facade.FixtureResult) (infrastructure.FixtureRun, error) {
	return s.store.RecordFixtureResult(ctx, identity, runID, result)
}

func (s *Service) PublishRelease(ctx context.Context, tenantID int64, revisionID int64, actor int64, version string) (infrastructure.Release, error) {
	version = strings.TrimSpace(version)
	if version == "" || len(version) > 64 {
		return infrastructure.Release{}, fmt.Errorf("version is required and must be at most 64 characters")
	}
	revision, err := s.store.Revision(ctx, tenantID, revisionID)
	if err != nil {
		return infrastructure.Release{}, err
	}
	validation := processor.Validate(revision.Manifest, revision.Files)
	if !validation.Valid {
		_ = s.store.SetValidation(ctx, tenantID, revision.ID, validation)
		return infrastructure.Release{}, fmt.Errorf("RESULT_PROCESSOR_VALIDATION_REQUIRED")
	}
	manifest := revision.Manifest
	manifest.Version = version
	archive, digest, err := processor.CanonicalArchive(manifest, revision.Files)
	if err != nil {
		return infrastructure.Release{}, err
	}
	key := fmt.Sprintf("result-processors/%s/%d/%s/%s.tar.gz", objectSegment(strconv.FormatInt(tenantID, 10)), revision.ProcessorID, objectSegment(version), digest)
	if err := s.putAndReadback(ctx, key, archive, digest); err != nil {
		return infrastructure.Release{}, err
	}
	return s.store.Publish(ctx, tenantID, revision.ID, actor, version, key, digest, int64(len(archive)))
}

func (s *Service) SetDefaultBinding(ctx context.Context, tenantID int64, actor int64, releaseID int64) error {
	return s.store.SetDefaultBinding(ctx, tenantID, actor, releaseID)
}

func (s *Service) SetEnvironmentBinding(ctx context.Context, tenantID int64, actor int64, binding infrastructure.EnvironmentBinding) (infrastructure.EnvironmentBinding, error) {
	binding.Name = strings.TrimSpace(binding.Name)
	binding.SecretRef = strings.TrimSpace(binding.SecretRef)
	binding.SecretVersion = strings.TrimSpace(binding.SecretVersion)
	if binding.Name == "" || len(binding.Name) > 128 {
		return infrastructure.EnvironmentBinding{}, fmt.Errorf("environment binding name is required")
	}
	return s.store.SetEnvironmentBinding(ctx, tenantID, actor, binding)
}

// BootstrapBuiltins creates the small platform-owned Processor catalog using
// the same Draft and immutable Release records as tenant-authored Processors.
// It is idempotent and intentionally does not install a tenant default binding.
func (s *Service) BootstrapBuiltins(ctx context.Context) error {
	for _, builtin := range systemProcessorAssets() {
		if err := s.bootstrapSystemProcessor(ctx, builtin); err != nil {
			return fmt.Errorf("bootstrap system Result Processor %q: %w", builtin.Name, err)
		}
	}
	return nil
}

func (s *Service) bootstrapSystemProcessor(ctx context.Context, builtin builtinAsset) error {
	archive, digest, err := processor.CanonicalArchive(builtin.Manifest, builtin.Files)
	if err != nil {
		return err
	}
	items, err := s.store.ListSystemProcessors(ctx)
	if err != nil {
		return err
	}
	var item *infrastructure.Processor
	for i := range items {
		if items[i].Name == builtin.Name {
			item = &items[i]
			break
		}
	}
	if item != nil {
		detailed, detailErr := s.store.GetSystemProcessor(ctx, item.ID)
		if detailErr != nil {
			return detailErr
		}
		item = &detailed
		if item.LatestRelease != nil && item.LatestRelease.Version == builtin.Version {
			if item.LatestRelease.ArchiveSHA256 != digest {
				return infrastructure.ErrConflict
			}
			return nil
		}
	}
	if item == nil {
		created, createErr := s.store.CreateSystemProcessor(ctx, "resultprocessor.bootstrap", builtin.Name, builtin.Category, builtin.Description, builtin.Manifest, builtin.Files)
		if createErr != nil && !errors.Is(createErr, infrastructure.ErrConflict) {
			return createErr
		}
		if createErr == nil {
			item = &created
		} else {
			items, err = s.store.ListSystemProcessors(ctx)
			if err != nil {
				return err
			}
			for i := range items {
				if items[i].Name == builtin.Name {
					detailed, detailErr := s.store.GetSystemProcessor(ctx, items[i].ID)
					if detailErr != nil {
						return detailErr
					}
					item = &detailed
					break
				}
			}
		}
	}
	if item == nil || item.CurrentDraft == nil {
		return fmt.Errorf("resolve system Result Processor draft")
	}
	validation := processor.Validate(item.CurrentDraft.Manifest, item.CurrentDraft.Files)
	if !validation.Valid {
		return fmt.Errorf("system Result Processor source invalid: %s", strings.Join(validation.Errors, "; "))
	}
	if err := s.store.SetSystemValidation(ctx, item.CurrentDraft.ID, validation); err != nil {
		return err
	}
	draftManifest := item.CurrentDraft.Manifest
	draftManifest.Version = builtin.Version
	draftArchive, draftDigest, err := processor.CanonicalArchive(draftManifest, item.CurrentDraft.Files)
	if err != nil {
		return err
	}
	if draftDigest != digest || !bytes.Equal(draftArchive, archive) {
		return infrastructure.ErrConflict
	}
	key := fmt.Sprintf("result-processors/system/%d/%s/%s.tar.gz", item.ID, objectSegment(builtin.Version), digest)
	if err = s.putAndReadback(ctx, key, archive, digest); err != nil {
		return err
	}
	_, err = s.store.PublishSystem(ctx, item.CurrentDraft.ID, "resultprocessor.bootstrap", builtin.Version, key, digest, int64(len(archive)))
	if errors.Is(err, infrastructure.ErrConflict) {
		current, getErr := s.store.GetSystemProcessor(ctx, item.ID)
		if getErr != nil {
			return getErr
		}
		for _, release := range current.Releases {
			if release.Version == builtin.Version && release.ArchiveSHA256 == digest {
				return nil
			}
		}
		return infrastructure.ErrConflict
	}
	return err
}

func (s *Service) putAndReadback(ctx context.Context, key string, archive []byte, expectedDigest string) error {
	if s.artifacts == nil {
		return errors.New("artifact store is unavailable")
	}
	if err := s.artifacts.Put(ctx, key, "application/gzip", bytes.NewReader(archive), int64(len(archive))); err != nil {
		return err
	}
	object, err := s.artifacts.Get(ctx, key)
	if err != nil {
		return err
	}
	defer object.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(object, maxProcessorArchiveReadback+1))
	if err != nil {
		return err
	}
	if size != int64(len(archive)) || hex.EncodeToString(hash.Sum(nil)) != expectedDigest {
		return errors.New("Result Processor archive failed byte-for-byte readback")
	}
	return nil
}

func defaultJSONObject(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func defaultJSONArray(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`[]`)
	}
	return value
}

func objectSegment(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, value)
	if value == "" {
		return "unknown"
	}
	return value
}
