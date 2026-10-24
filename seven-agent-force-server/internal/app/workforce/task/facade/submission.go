// Package facade exposes stable cross-module Task contracts. Callers submit a
// fully compiled Task request; only the Task application owns execution asset
// resolution, snapshot freezing, idempotency and graph creation.
package facade

import (
	"errors"
	"strings"
	"unicode/utf8"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
)

var ErrIdempotencyConflict = errors.New("task submission idempotency conflict")

// ErrInvalidTaskSubmission marks Task-owned input validation failures across
// module boundaries. It does not classify arbitrary dependency errors.
var ErrInvalidTaskSubmission = errors.New("invalid task submission")

const MaxInstructionCharacters = 20000

// OpenProjectTaskCommand is filled by the authenticated Open boundary, not
// decoded from caller JSON. IdempotencyKey is application-namespaced there.
// It grants no operator permission; the existing tenant-level submission scope
// and transaction-owned Application guard remain authoritative.
type OpenProjectTaskCommand struct {
	TenantID, ApplicationID, CreatorUserID, ProjectID int64
	IdempotencyKey, ExternalCorrelationID             string
	Request                                           ProjectTaskRequest
}

// ProjectTaskRequest is the Project-scoped Task payload shared by entry points.
// Project rules are compiled; model and Runtime selections are validated and
// frozen at creation. The dispatcher selects the Worker.
type ProjectTaskRequest struct {
	Instruction               string                     `json:"instruction"`
	Name                      string                     `json:"name,omitempty"`
	Tags                      []string                   `json:"tags,omitempty"`
	ReferenceAssetIDs         sharedtransport.Int64Slice `json:"referenceAssetIds,omitempty"`
	AdditionalCompletionCheck *completioncheck.Input     `json:"additionalCompletionCheck,omitempty"`
	ReviewEnabled             *bool                      `json:"reviewEnabled,omitempty"`
	DeliveryPreference        string                     `json:"deliveryPreference,omitempty"`
	ModelSelection            ModelSelection             `json:"modelSelection"`
	RuntimeReleaseID          int64                      `json:"runtimeReleaseId,string"`
	ExecutionTimeoutSeconds   int                        `json:"executionTimeoutSeconds,omitempty"`
}

type ModelSelection struct {
	CredentialID int64  `json:"credentialId,string"`
	Model        string `json:"model"`
}

func (r ProjectTaskRequest) Additions() projectfacade.TaskAdditions {
	reviewEnabled := true
	if r.ReviewEnabled != nil {
		reviewEnabled = *r.ReviewEnabled
	}
	return projectfacade.TaskAdditions{Instruction: r.Instruction, Name: r.Name, Tags: append([]string(nil), r.Tags...), ReferenceAssetIDs: append(sharedtransport.Int64Slice(nil), r.ReferenceAssetIDs...), AdditionalCompletionCheck: r.AdditionalCompletionCheck, ReviewEnabled: reviewEnabled, DeliveryPreference: r.DeliveryPreference}
}

func ValidInstruction(value string) bool {
	value = strings.TrimSpace(value)
	count := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && count >= 1 && count <= MaxInstructionCharacters
}
