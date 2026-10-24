// Package facade defines the single cross-module contract for resolving the
// infrastructure used by an execution node. Tasks describe requirements;
// assignments freeze the concrete runtime, Provider and Worker.
package facade

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
)

var (
	ErrRuntimeSelectionRequired     = errors.New("TASK_RUNTIME_SELECTION_REQUIRED")
	ErrRuntimeSelectionUnavailable  = errors.New("TASK_RUNTIME_SELECTION_UNAVAILABLE")
	ErrRuntimeSelectionIncompatible = errors.New("TASK_RUNTIME_SELECTION_INCOMPATIBLE")
)

const (
	WaitAgentRuntime    = "WAITING_AGENT_RUNTIME"
	WaitSelectedRuntime = "WAITING_SELECTED_RUNTIME"
	WaitModelCredential = "WAITING_MODEL_CREDENTIAL"
	WaitWorkerCapacity  = "WAITING_WORKER_CAPACITY"
	WaitWorkerDisk      = "WORKER_DISK_PRESSURE"
	WaitProjectInvalid  = "PROJECT_CONFIGURATION_INVALID"
)

type RuntimeWorkerOption struct {
	WorkerID          int64  `json:"workerId,string"`
	Name              string `json:"name"`
	State             string `json:"state"`
	ReservedAttempts  int    `json:"reservedAttempts"`
	StartingAttempts  int    `json:"startingAttempts"`
	RunningAttempts   int    `json:"runningAttempts"`
	AvailableAttempts int    `json:"availableAttempts"`
}

type RuntimeOption struct {
	RuntimeReleaseID          int64                 `json:"runtimeReleaseId,string"`
	Name                      string                `json:"name"`
	Description               string                `json:"description"`
	HarnessType               string                `json:"harnessType"`
	HarnessVersion            string                `json:"harnessVersion"`
	AdapterVersion            string                `json:"adapterVersion"`
	ImageRef                  string                `json:"imageRef"`
	ImageDigest               string                `json:"imageDigest"`
	ProviderProtocols         []string              `json:"providerProtocols"`
	EnvironmentCapabilities   []string              `json:"environmentCapabilities"`
	ExecutionKinds            []string              `json:"executionKinds"`
	HookEvents                []string              `json:"hookEvents"`
	SupportsIndependentReview bool                  `json:"supportsIndependentReview"`
	WaitingTaskCount          int                   `json:"waitingTaskCount"`
	AvailableSlots            int                   `json:"availableSlots"`
	Recommended               bool                  `json:"recommended"`
	Workers                   []RuntimeWorkerOption `json:"workers"`
	SchedulingPriority        int                   `json:"-"`
}

type RuntimeOptions struct {
	ProjectVersionID int64           `json:"projectVersionId,string"`
	Items            []RuntimeOption `json:"items"`
}

type RuntimeCatalog interface {
	ValidateTaskSelection(context.Context, int64, int64, int64, string, projectfacade.ExecutionRequirements) error
	ValidateTaskSelectionInTransaction(context.Context, pgx.Tx, int64, int64, int64, string, projectfacade.ExecutionRequirements) error
	ListTaskOptions(context.Context, int64, int64, int64) (RuntimeOptions, error)
}

type NodeIdentity struct {
	TenantID   int64
	TaskID     int64
	TaskTurnID int64
	NodeID     int64
	Generation int64
}

type PlatformIdentity struct {
	ProviderProxyImageDigest string
	ProviderProxyVersion     string
	LocalFacadeVersion       string
	LocalFacadeDigest        string
	GatewayIdentity          string
	GatewayContractDigest    string
	ProviderAdapterIdentity  string
}

type ResolvedAssignment struct {
	WorkerID                int64
	WorkerInstanceID        string
	RuntimeReleaseID        int64
	RuntimeImageRef         string
	RuntimeImageDigest      string
	AdapterID               string
	AdapterVersion          string
	AdapterProtocol         string
	AdapterCapabilities     []string
	HookConfigurationDigest string
	ModelCredentialID       int64
	CredentialVersion       int64
	ModelCredentialName     string
	ModelProtocol           string
	PrimaryModel            string
	WorkerImageContract     string
	IsolationProfile        string
	Snapshot                taskdomain.ExecutionSnapshot
	SnapshotJSON            json.RawMessage
	SelectionDigest         string
}

type AssignmentResolver interface {
	ResolveAssignment(context.Context, pgx.Tx, NodeIdentity, projectfacade.ExecutionRequirements, taskdomain.ExecutionSnapshot) (ResolvedAssignment, string, error)
}

type ReviewRequirements struct {
	TenantID                    int64
	ReviewRunID                 int64
	PreferredRuntimeReleaseID   int64
	RequiredAdapterCapabilities []string
	ModelCredentialID           int64
	CredentialVersion           int64
	ModelProtocol               string
	PrimaryModel                string
}

type ResolvedReviewAssignment struct {
	RuntimeReleaseID    int64
	RuntimeImageDigest  string
	AdapterID           string
	AdapterVersion      string
	AdapterProtocol     string
	AdapterCapabilities json.RawMessage
	ModelCredentialID   int64
	CredentialVersion   int64
	ModelProtocol       string
	PrimaryModel        string
	ProviderProxyImage  string
	WorkerID            int64
	WorkerInstanceID    string
	SelectionDigest     string
}

type ReviewAssignmentResolver interface {
	ResolveReviewAssignment(context.Context, pgx.Tx, ReviewRequirements) (ResolvedReviewAssignment, string, error)
}
