package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	tf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	wi "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	workspacefacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/facade"
	"github.com/jackc/pgx/v5"
)

type preparedStage struct {
	service *Service
	command submitTaskCommand
	source  tf.StageSource
}

func (p *preparedStage) Source() tf.StageSource { return p.source }
func (p *preparedStage) CreateTx(ctx context.Context, tx pgx.Tx, key string) (tf.StageTask, error) {
	if tx == nil {
		return tf.StageTask{}, errors.New("STAGE_TRANSACTION_REQUIRED")
	}
	cmd := p.command
	cmd.Transaction = tx
	cmd.IdempotencyKey = key
	t, _, err := p.service.create(ctx, cmd)
	return tf.StageTask{ID: t.ID, Status: string(t.Status)}, err
}
func (s *Service) PrepareStage(ctx context.Context, r tf.StageRequest) (tf.PreparedStage, error) {
	if r.TenantID < 1 || r.ActorID < 1 || r.ProjectID < 1 || (r.Purpose != "PLAN" && r.Purpose != "IMPLEMENT") {
		return nil, errors.New("WORK_ITEM_STAGE_INVALID")
	}
	if (r.Purpose == "IMPLEMENT" && len(r.ApprovedPlan) == 0) || len(r.ApprovedPlan) > 256*1024 || (len(r.ApprovedPlan) > 0 && !json.Valid(r.ApprovedPlan)) {
		return nil, errors.New("WORK_ITEM_PLAN_INPUT_INVALID")
	}
	additions := r.Selection.Additions()
	additions.Instruction = r.Instruction
	additions.Name = r.Name
	if r.Purpose == "PLAN" {
		additions.DeliveryPreference = projectfacade.DeliverySaveOnly
		additions.ReviewEnabled = false
	}
	var spec projectfacade.FrozenTaskSpec
	var err error
	if r.ProjectVersionID > 0 {
		compiler, ok := s.projects.(projectfacade.VersionTaskCompiler)
		if !ok {
			return nil, errors.New("PROJECT_VERSION_COMPILER_UNAVAILABLE")
		}
		spec, err = compiler.CompileForTaskVersion(ctx, r.TenantID, r.ProjectID, r.ProjectVersionID, additions)
	} else {
		spec, err = s.projects.CompileForTask(ctx, r.TenantID, r.ProjectID, additions)
	}
	if err != nil {
		return nil, err
	}
	if r.SourceRevisionID > 0 {
		spec.WorkspaceSourceRevisionID = r.SourceRevisionID
		spec.GitPlan.SourceRevisionID = r.SourceRevisionID
	}
	if r.Purpose == "PLAN" {
		// Hook classification is not inferred from script prose. Until a compatible
		// purpose contract is declared, fail closed rather than drop project hooks.
		if len(spec.HookReleaseIDs) > 0 {
			return nil, tf.ErrPlanningHooksUnsupported
		}
		resolver, ok := s.processors.(processorfacade.PlanningResolver)
		if !ok {
			return nil, errors.New("PLANNING_PROCESSOR_UNAVAILABLE")
		}
		spec.ResultProcessorReleaseID, err = resolver.PlanningRelease(ctx)
		if err != nil {
			return nil, err
		}
		spec.CompletionCheck = nil
		spec.ReviewEnabled = false
		spec.ResultProcessorParameters = json.RawMessage(`{}`)
		spec.ExecutionRequirements.AllowedPlatformTools = []string{}
	}
	spec.ExecutionRequirements.StagePurpose = r.Purpose
	body, _ := json.Marshal(spec.ExecutionRequirements)
	spec.ExecutionRequirementsDigest = infrastructureDigest(body)
	model, err := s.models.ValidateTaskSelection(ctx, r.TenantID, r.Selection.ModelSelection.CredentialID, r.Selection.ModelSelection.Model)
	if err != nil {
		return nil, err
	}
	if err = s.runtimes.ValidateTaskSelection(ctx, r.TenantID, spec.ProjectVersionID, r.Selection.RuntimeReleaseID, model.Protocol, spec.ExecutionRequirements); err != nil {
		return nil, err
	}
	mode := wi.SourceKindEmptyWorkspace
	if spec.WorkspaceSourceRevisionID > 0 {
		mode = wi.SourceKindSourceRevision
	}
	input := taskInputSelection{Mode: mode, WorkspaceSourceRevisionID: spec.WorkspaceSourceRevisionID, ReferenceAssetIDs: r.Selection.ReferenceAssetIDs}
	sourceInput, err := s.inputs.ResolveExecutionInput(ctx, r.TenantID, workspaceSelection(input, projectGitPlanRequest(spec.GitPlan)))
	if err != nil {
		return nil, err
	}
	if sourceInput.Source.ProjectID != 0 && (sourceInput.Source.ProjectID != r.ProjectID || sourceInput.Source.ProjectVersionID != spec.ProjectVersionID) {
		return nil, errors.New("WORK_ITEM_SOURCE_MISMATCH")
	}
	instruction := spec.Instruction
	notification := &taskNotificationSelection{Enabled: &spec.Notification.Enabled, Events: append([]string(nil), spec.Notification.Events...)}
	if spec.Notification.GroupRouteID > 0 {
		notification.Channels = []taskNotificationChannel{{Kind: "GROUP_ROUTE", RouteID: spec.Notification.GroupRouteID}}
	}
	compiled := compiledTaskRequest{ProjectSpec: &spec, StagePurpose: r.Purpose, FrozenProjectVersion: true, ApprovedPlan: r.ApprovedPlan, CompletionCheck: spec.CompletionCheck, TaskInput: input, Name: spec.Name, Tags: spec.Tags, GitPlan: projectGitPlanRequest(spec.GitPlan), Instruction: instruction, ResultProcessorReleaseID: spec.ResultProcessorReleaseID, ResultProcessorParameters: spec.ResultProcessorParameters, ResultProcessorConfigOverrides: json.RawMessage(`{}`), ReviewEnabled: spec.ReviewEnabled, ReviewPrompt: spec.ReviewPrompt, ModelSelection: model, RuntimeReleaseID: r.Selection.RuntimeReleaseID, Priority: 50, ExecutionTimeoutSeconds: r.Selection.ExecutionTimeoutSeconds, Notification: notification}
	return &preparedStage{service: s, command: submitTaskCommand{TenantID: r.TenantID, CreatorUserID: r.ActorID, PrincipalType: "USER", Channel: "WEB", Request: compiled}, source: tf.StageSource{ProjectID: r.ProjectID, ProjectVersionID: spec.ProjectVersionID, RevisionID: spec.WorkspaceSourceRevisionID, Kind: sourceInput.Snapshot.SourceKind, Commit: sourceInput.Source.Commit, IdentityDigest: sourceInput.Source.IdentityDigest}}, nil
}
func (s *Service) StageTask(ctx context.Context, tenant, id int64) (tf.StageTask, error) {
	t, e := s.store.Get(ctx, id)
	if e != nil {
		return tf.StageTask{}, e
	}
	if t.TenantID != tenant {
		return tf.StageTask{}, errors.New("WORK_ITEM_NOT_FOUND")
	}
	return tf.StageTask{ID: t.ID, Status: string(t.Status)}, nil
}
func (s *Service) CancelStageTask(ctx context.Context, tenant, id, actor int64) error {
	if _, e := s.StageTask(ctx, tenant, id); e != nil {
		return e
	}
	_, e := s.Cancel(ctx, id, actor, "Work item cancelled")
	return e
}

var _ tf.StageExecutor = (*Service)(nil)

func infrastructureDigest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func workspaceSelection(input taskInputSelection, g gitPlanRequest) workspacefacade.TaskInputSelection {
	return workspacefacade.TaskInputSelection{Mode: input.Mode, WorkspaceSourceRevisionID: input.WorkspaceSourceRevisionID, ReferenceAssetIDs: input.ReferenceAssetIDs, GitPlan: wi.GitPlan{DeliveryMode: g.DeliveryMode, DeliveryAuthorization: g.DeliveryAuthorization, ProjectID: g.ProjectID, ProjectVersionID: g.ProjectVersionID, RepositoryConfigDigest: g.RepositoryConfigDigest, ReviewBaseSourceRevisionID: g.ReviewBaseSourceRevisionID, ReviewBaseBranch: g.ReviewBaseBranch, ResultBranch: g.ResultBranch, FullProjectImport: g.FullProjectImport}}
}
