// Package facade exposes the business configuration frozen from a Workforce
// Project into a Task. Infrastructure routing is deliberately absent.
package facade

import (
	"context"
	"encoding/json"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
)

const (
	KindCodebase = "CODEBASE"
	KindGeneral  = "GENERAL"

	StatusActive   = "ACTIVE"
	StatusArchived = "ARCHIVED"

	DeliveryProjectDefault = "PROJECT_DEFAULT"
	DeliveryAskAfterGates  = "ASK_AFTER_GATES"
	DeliverySaveOnly       = "SAVE_ONLY"
)

type ExecutionRequirements struct {
	StagePurpose             string   `json:"stagePurpose,omitempty"`
	ExecutionKind            string   `json:"executionKind"`
	AdapterCapabilities      []string `json:"adapterCapabilities"`
	EnvironmentCapabilities  []string `json:"environmentCapabilities"`
	AllowedPlatformTools     []string `json:"allowedPlatformTools"`
	ResourceClass            string   `json:"resourceClass"`
	RequiresNativeResume     bool     `json:"requiresNativeResume"`
	RequiresStructuredOutput bool     `json:"requiresStructuredOutput"`
}

type TaskAdditions struct {
	Instruction               string                     `json:"instruction"`
	Name                      string                     `json:"name,omitempty"`
	Tags                      []string                   `json:"tags,omitempty"`
	ReferenceAssetIDs         sharedtransport.Int64Slice `json:"referenceAssetIds,omitempty"`
	AdditionalCompletionCheck *completioncheck.Input     `json:"additionalCompletionCheck,omitempty"`
	ReviewEnabled             bool                       `json:"reviewEnabled"`
	DeliveryPreference        string                     `json:"deliveryPreference,omitempty"`
}

type FrozenGitPlan struct {
	ProjectID                       int64  `json:"projectId,string,omitempty"`
	ProjectVersionID                int64  `json:"projectVersionId,string,omitempty"`
	RepositoryConfigDigest          string `json:"repositoryConfigDigest,omitempty"`
	RepositoryCredentialVersion     int64  `json:"repositoryCredentialVersion,string,omitempty"`
	RepositoryDisplayURL            string `json:"repositoryDisplayUrl,omitempty"`
	RepositoryProviderType          string `json:"repositoryProviderType,omitempty"`
	RepositoryScheme                string `json:"-"`
	RepositoryHost                  string `json:"-"`
	RepositoryPort                  int    `json:"-"`
	RepositoryPath                  string `json:"-"`
	RepositoryWebURL                string `json:"repositoryWebUrl,omitempty"`
	RepositoryCredentialCiphertext  string `json:"-"`
	RepositoryCredentialEDEK        string `json:"-"`
	RepositoryCredentialWrapKeyRef  string `json:"-"`
	SourceRevisionID                int64  `json:"sourceRevisionId,string,omitempty"`
	DefaultRefName                  string `json:"defaultRefName,omitempty"`
	ReviewBaseBranch                string `json:"reviewBaseBranch,omitempty"`
	ResultBranchPattern             string `json:"resultBranchPattern,omitempty"`
	DeliveryMode                    string `json:"deliveryMode"`
	DeliveryAuthorization           string `json:"deliveryAuthorization,omitempty"`
	DeliveryAuthorizedByPrincipalID int64  `json:"-"`
}

type NotificationPolicy struct {
	Enabled      bool            `json:"enabled"`
	Events       []string        `json:"events,omitempty"`
	GroupRouteID int64           `json:"groupRouteId,string,omitempty"`
	Raw          json.RawMessage `json:"-"`
}

type FrozenTaskSpec struct {
	HookReleaseIDs              []int64                    `json:"-"`
	ProjectID                   int64                      `json:"projectId,string"`
	ProjectVersionID            int64                      `json:"projectVersionId,string"`
	ExecutionRequirements       ExecutionRequirements      `json:"executionRequirements"`
	ExecutionRequirementsDigest string                     `json:"executionRequirementsDigest"`
	ProjectName                 string                     `json:"projectName"`
	ProjectKind                 string                     `json:"projectKind"`
	Instruction                 string                     `json:"instruction"`
	Name                        string                     `json:"name"`
	Tags                        []string                   `json:"tags"`
	ReferenceAssetIDs           sharedtransport.Int64Slice `json:"referenceAssetIds"`
	WorkspaceSourceRevisionID   int64                      `json:"workspaceSourceRevisionId,string,omitempty"`
	CompletionCheck             *completioncheck.Input     `json:"completionCheck,omitempty"`
	ResultProcessorReleaseID    int64                      `json:"resultProcessorReleaseId,string,omitempty"`
	ResultProcessorParameters   json.RawMessage            `json:"resultProcessorParameters"`
	ReviewEnabled               bool                       `json:"reviewEnabled"`
	ReviewPrompt                string                     `json:"reviewPrompt,omitempty"`
	GitPlan                     FrozenGitPlan              `json:"gitPlan"`
	Notification                NotificationPolicy         `json:"notification"`
}

type TaskCompiler interface {
	CompileForTask(context.Context, int64, int64, TaskAdditions) (FrozenTaskSpec, error)
}

// RequirementsCompiler reads the current Project version without compiling a
// Task instruction, preparing sources, or invoking external services.
type RequirementsCompiler interface {
	CompileExecutionRequirements(context.Context, int64, int64) (CompiledExecutionRequirements, error)
}

type CompiledExecutionRequirements struct {
	ProjectVersionID int64
	Requirements     ExecutionRequirements
}

// VersionTaskCompiler is reserved for server-owned stage orchestration. Callers
// cannot override repository credentials, completion rules or delivery targets.
type VersionTaskCompiler interface {
	CompileForTaskVersion(context.Context, int64, int64, int64, TaskAdditions) (FrozenTaskSpec, error)
}
