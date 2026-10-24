package mcpserver

import (
	"context"
	"errors"
	"io"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
)

const (
	ProtocolVersion = "2026-07-28"
	DefaultEndpoint = "/mcp"
	DefaultMaxBody  = int64(256 << 10)
	DefaultMaxText  = int64(256 << 10)
)

type Capabilities struct {
	Schema         string                   `json:"schema"`
	ApplicationID  int64                    `json:"applicationId,string"`
	TaskSubmission TaskSubmissionCapability `json:"taskSubmission"`
}

type TaskSubmissionCapability struct {
	SupportsSelectionDiscovery        bool `json:"supportsSelectionDiscovery"`
	RequiresModelSelection            bool `json:"requiresModelSelection"`
	RequiresRuntimeReleaseID          bool `json:"requiresRuntimeReleaseId"`
	RequiresProjectID                 bool `json:"requiresProjectId"`
	SupportsReferenceAssets           bool `json:"supportsReferenceAssets"`
	SupportsAdditionalCompletionCheck bool `json:"supportsAdditionalCompletionCheck"`
	SupportsReviewToggle              bool `json:"supportsReviewToggle"`
	SupportsDeliveryPreference        bool `json:"supportsDeliveryPreference"`
	SupportsOutputs                   bool `json:"supportsOutputs"`
}

type CreateTaskRequest struct {
	Tags                      []string                   `json:"tags,omitempty"`
	ReferenceAssetIDs         sharedtransport.Int64Slice `json:"referenceAssetIds,omitempty"`
	AdditionalCompletionCheck *completioncheck.Input     `json:"additionalCompletionCheck,omitempty"`
	ReviewEnabled             *bool                      `json:"reviewEnabled,omitempty"`
	DeliveryPreference        string                     `json:"deliveryPreference,omitempty"`
	ModelSelection            taskfacade.ModelSelection  `json:"modelSelection"`
	RuntimeReleaseID          int64                      `json:"runtimeReleaseId,string"`
	ExecutionTimeoutSeconds   int                        `json:"executionTimeoutSeconds,omitempty"`

	IdempotencyKey        string `json:"-"`
	ProjectID             int64  `json:"-"`
	Name                  string `json:"name"`
	Instruction           string `json:"instruction"`
	ExternalCorrelationID string `json:"externalCorrelationId"`
}

type Task struct {
	Schema                string    `json:"schema,omitempty"`
	TaskID                int64     `json:"taskId,string"`
	TaskTurnID            int64     `json:"taskTurnId,string,omitempty"`
	ProjectID             int64     `json:"projectId,string,omitempty"`
	Status                string    `json:"status"`
	Replayed              *bool     `json:"replayed,omitempty"`
	ExternalCorrelationID string    `json:"externalCorrelationId,omitempty"`
	ReasonCode            string    `json:"reasonCode,omitempty"`
	ResultSummary         string    `json:"resultSummary,omitempty"`
	CreatedAt             string    `json:"createdAt,omitempty"`
	StartedAt             string    `json:"startedAt,omitempty"`
	FinishedAt            string    `json:"finishedAt,omitempty"`
	Links                 TaskLinks `json:"links,omitempty"`
}

type TaskLinks struct {
	Operator string `json:"operator,omitempty"`
	Self     string `json:"self,omitempty"`
}

type Output struct {
	OutputID   int64       `json:"outputId,string"`
	Name       string      `json:"name"`
	OutputKind string      `json:"outputKind"`
	Status     string      `json:"status"`
	MediaType  string      `json:"mediaType"`
	SHA256     string      `json:"sha256"`
	SizeBytes  int64       `json:"sizeBytes,string"`
	Links      OutputLinks `json:"links,omitempty"`
}

type OutputLinks struct {
	Download string `json:"download,omitempty"`
}

type OutputStream struct {
	Content io.ReadCloser
}

type WorkforceGateway interface {
	ListModelOptions(context.Context, string, int64) (ModelOptions, error)
	ListRuntimeOptions(context.Context, string, int64, int64) (RuntimeOptions, error)

	GetCapabilities(ctx context.Context, authorization string) (Capabilities, error)
	CreateTask(ctx context.Context, authorization string, request CreateTaskRequest) (Task, error)
	GetTask(ctx context.Context, authorization string, taskID int64) (Task, error)
	ListOutputs(ctx context.Context, authorization string, taskID int64) ([]Output, error)
	OpenOutput(ctx context.Context, authorization string, taskID, outputID int64) (OutputStream, error)
}

type GatewayError struct {
	Code       string
	Message    string
	Retryable  bool
	HTTPStatus int
}

func (e *GatewayError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

func ErrorCode(err error) string {
	var target *GatewayError
	if errors.As(err, &target) && target.Code != "" {
		return target.Code
	}
	return "INTERNAL_ERROR"
}

type ModelOption struct {
	CredentialID int64  `json:"credentialId,string"`
	Model        string `json:"model"`
	Protocol     string `json:"protocol"`
	Label        string `json:"label"`
}
type ModelOptions struct {
	Schema    string        `json:"schema"`
	ProjectID int64         `json:"projectId,string"`
	Items     []ModelOption `json:"items"`
}
type RuntimeOption struct {
	RuntimeReleaseID int64  `json:"runtimeReleaseId,string"`
	Name             string `json:"name"`
	Recommended      bool   `json:"recommended"`
}
type RuntimeOptions struct {
	Schema           string          `json:"schema"`
	ProjectVersionID int64           `json:"projectVersionId,string"`
	Items            []RuntimeOption `json:"items"`
}
