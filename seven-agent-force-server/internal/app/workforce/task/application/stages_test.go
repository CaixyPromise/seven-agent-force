package application

import (
	"context"
	"encoding/json"
	"errors"
	routing "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	project "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	provider "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	processor "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	inputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	input "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/facade"
	"github.com/jackc/pgx/v5"
	"strings"
	"testing"
)

type stageProject struct {
	hooks []int64
	got   project.TaskAdditions
}

func (p *stageProject) CompileForTask(_ context.Context, tenant, id int64, a project.TaskAdditions) (project.FrozenTaskSpec, error) {
	p.got = a
	return project.FrozenTaskSpec{ProjectID: id, ProjectVersionID: 9, Instruction: a.Instruction, Name: a.Name, HookReleaseIDs: p.hooks, ExecutionRequirements: project.ExecutionRequirements{ExecutionKind: "AGENT_ATTEMPT", AllowedPlatformTools: []string{"network"}}, ExecutionRequirementsDigest: strings.Repeat("a", 64), ResultProcessorReleaseID: 10, ResultProcessorParameters: json.RawMessage(`{}`), ReviewEnabled: a.ReviewEnabled, GitPlan: project.FrozenGitPlan{DeliveryMode: "NONE"}}, nil
}

type stageProcessor struct{}

func (stageProcessor) PlanningRelease(context.Context) (int64, error) { return 42, nil }
func (stageProcessor) ResolveExecution(context.Context, int64, string, int64, json.RawMessage, json.RawMessage) (processor.Execution, error) {
	return processor.Execution{}, nil
}

type stageModel struct{}

func (stageModel) ValidateTaskSelection(_ context.Context, tenant, id int64, model string) (provider.RuntimeSelection, error) {
	return provider.RuntimeSelection{CredentialID: id, Protocol: "OPENAI", PrimaryModel: model}, nil
}
func (m stageModel) ValidateTaskSelectionInTransaction(ctx context.Context, _ pgx.Tx, tenant, id int64, model string) (provider.RuntimeSelection, error) {
	return m.ValidateTaskSelection(ctx, tenant, id, model)
}

type stageRuntime struct{}

func (stageRuntime) ValidateTaskSelection(context.Context, int64, int64, int64, string, project.ExecutionRequirements) error {
	return nil
}
func (stageRuntime) ValidateTaskSelectionInTransaction(context.Context, pgx.Tx, int64, int64, int64, string, project.ExecutionRequirements) error {
	return nil
}
func (stageRuntime) ListTaskOptions(context.Context, int64, int64, int64) (routing.RuntimeOptions, error) {
	return routing.RuntimeOptions{}, nil
}

type stageInputs struct{}

func (stageInputs) ResolveExecutionInput(_ context.Context, _ int64, in input.TaskInputSelection) (input.FrozenTaskInput, error) {
	return input.FrozenTaskInput{Snapshot: inputdomain.TaskInputSnapshot{SourceKind: in.Mode}, Source: input.SourceBundle{Commit: "base", IdentityDigest: "identity"}}, nil
}
func TestPreparePlanningStageUsesIsolatedPolicy(t *testing.T) {
	compiler := &stageProject{}
	s := &Service{projects: compiler, processors: stageProcessor{}, models: stageModel{}, runtimes: stageRuntime{}, inputs: stageInputs{}}
	r := task.StageRequest{TenantID: 1, ActorID: 2, ProjectID: 3, Purpose: "PLAN", Name: "plan", Instruction: "build", Selection: task.ProjectTaskRequest{ModelSelection: task.ModelSelection{CredentialID: 4, Model: "m"}, RuntimeReleaseID: 5}}
	result, err := s.PrepareStage(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	p := result.(*preparedStage)
	if compiler.got.ReviewEnabled || compiler.got.DeliveryPreference != project.DeliverySaveOnly || p.command.Request.ReviewEnabled || p.command.Request.ResultProcessorReleaseID != 42 || p.command.Request.ProjectSpec.ExecutionRequirements.StagePurpose != "PLAN" || len(p.command.Request.ProjectSpec.ExecutionRequirements.AllowedPlatformTools) != 0 {
		t.Fatal("planning stage did not isolate rules")
	}
	if p.source.ProjectVersionID != 9 || p.source.Commit != "base" {
		t.Fatal("source binding missing")
	}
	compiler.hooks = []int64{99}
	if _, err = s.PrepareStage(context.Background(), r); !errors.Is(err, task.ErrPlanningHooksUnsupported) {
		t.Fatal("unsafe hooks accepted", err)
	}
	r.Purpose = "IMPLEMENT"
	compiler.hooks = nil
	if _, err = s.PrepareStage(context.Background(), r); err == nil {
		t.Fatal("implementation without approved plan accepted")
	}
}
