package domain

import (
	"encoding/json"
	"time"
)

const (
	ApplicationEnabled  = "ENABLED"
	ApplicationDisabled = "DISABLED"
	CredentialCurrent   = "CURRENT"
	CredentialGrace     = "GRACE"
	CredentialRevoked   = "REVOKED"
)

type CredentialSummary struct {
	ID      int64  `json:"credentialId,string"`
	Version int    `json:"version"`
	Hint    string `json:"hint"`
}

type CredentialGraceSummary struct {
	ID         int64     `json:"credentialId,string"`
	Version    int       `json:"version"`
	ValidUntil time.Time `json:"validUntil"`
}

type Application struct {
	ID                   int64                   `json:"id,string"`
	TenantID             int64                   `json:"-"`
	CreatedByUserID      int64                   `json:"-"`
	Name                 string                  `json:"name"`
	WebhookURL           string                  `json:"webhookUrl"`
	Status               string                  `json:"status"`
	APICredential        CredentialSummary       `json:"apiCredential"`
	SigningCredential    *CredentialSummary      `json:"signingCredential,omitempty"`
	APIKeyGrace          *CredentialGraceSummary `json:"apiKeyGrace,omitempty"`
	SigningSecretGrace   *CredentialGraceSummary `json:"signingSecretGrace,omitempty"`
	LatestDeliveryStatus string                  `json:"latestDeliveryStatus,omitempty"`
	CreatedAt            time.Time               `json:"createdAt"`
	UpdatedAt            time.Time               `json:"updatedAt"`
	LastUsedAt           *time.Time              `json:"lastUsedAt,omitempty"`
}

type Identity struct {
	ApplicationID int64
	TenantID      int64
	Name          string
	// CreatedByUserID is the immutable Framework user that owns the
	// Integration Application. Open Task records use it for their numeric
	// Task/TaskTurn ownership while ApplicationID records the actual caller.
	CreatedByUserID int64
}

type OpenCapabilitiesView struct {
	Schema         string                       `json:"schema"`
	ApplicationID  int64                        `json:"applicationId,string"`
	TaskSubmission OpenTaskSubmissionCapability `json:"taskSubmission"`
}

type OpenTaskSubmissionCapability struct {
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

type OpenTaskView struct {
	Schema                string    `json:"schema"`
	TaskID                int64     `json:"taskId,string"`
	TaskTurnID            int64     `json:"taskTurnId,string"`
	ProjectID             int64     `json:"projectId,string"`
	Status                string    `json:"status"`
	Replayed              *bool     `json:"replayed,omitempty"`
	ExternalCorrelationID string    `json:"externalCorrelationId"`
	ReasonCode            string    `json:"reasonCode,omitempty"`
	ResultSummary         string    `json:"resultSummary,omitempty"`
	CreatedAt             string    `json:"createdAt,omitempty"`
	StartedAt             string    `json:"startedAt,omitempty"`
	FinishedAt            string    `json:"finishedAt,omitempty"`
	Links                 TaskLinks `json:"links"`
}

type TaskLinks struct {
	Operator string `json:"operator,omitempty"`
	Self     string `json:"self"`
}

type OpenTaskOutputsView struct {
	Schema  string           `json:"schema"`
	TaskID  int64            `json:"taskId,string"`
	Outputs []OpenTaskOutput `json:"outputs"`
}

type OpenTaskOutput struct {
	OutputID   int64       `json:"outputId,string"`
	Name       string      `json:"name"`
	OutputKind string      `json:"outputKind"`
	Status     string      `json:"status"`
	MediaType  string      `json:"mediaType"`
	SHA256     string      `json:"sha256"`
	SizeBytes  int64       `json:"sizeBytes,string"`
	Links      OutputLinks `json:"links"`
}

type OutputLinks struct {
	Download string `json:"download"`
}

type PublicEventEnvelope struct {
	Schema        string       `json:"schema"`
	EventID       int64        `json:"eventId,string"`
	DeliveryID    int64        `json:"deliveryId,string"`
	EventType     string       `json:"eventType"`
	OccurredAt    string       `json:"occurredAt"`
	Sequence      int64        `json:"sequence,string"`
	TenantID      int64        `json:"tenantId,string"`
	ApplicationID int64        `json:"applicationId,string"`
	CorrelationID string       `json:"correlationId"`
	Subject       EventSubject `json:"subject"`
	Data          EventData    `json:"data"`
	Links         EventLinks   `json:"links"`
}

type EventLinks struct {
	Task    string `json:"task"`
	Outputs string `json:"outputs"`
}

type EventSubject struct {
	Type string `json:"type"`
	ID   int64  `json:"id,string"`
}

type EventData struct {
	TaskID        int64  `json:"taskId,string"`
	TaskTurnID    int64  `json:"taskTurnId,string"`
	Status        string `json:"status"`
	ReasonCode    string `json:"reasonCode"`
	ResultSummary string `json:"resultSummary"`
}

type Delivery struct {
	ID                       int64           `json:"deliveryId,string"`
	EventID                  int64           `json:"eventId,string"`
	TaskID                   int64           `json:"taskId,string"`
	EventType                string          `json:"eventType"`
	Status                   string          `json:"status"`
	AttemptCount             int             `json:"attemptCount"`
	LastHTTPStatus           *int            `json:"lastHttpStatus,omitempty"`
	NextAttemptAt            *time.Time      `json:"nextAttemptAt,omitempty"`
	UpdatedAt                time.Time       `json:"updatedAt"`
	Payload                  json.RawMessage `json:"-"`
	WebhookURL               string          `json:"-"`
	SigningSecret            string          `json:"-"`
	TenantID                 int64           `json:"-"`
	ApplicationID            int64           `json:"-"`
	SigningCredentialID      int64           `json:"-"`
	GraceSigningCredentialID *int64          `json:"-"`
}

func PublicStatus(status string) (publicStatus, reasonCode, summary string) {
	switch status {
	case "CREATED", "QUEUED", "RETRY_WAIT":
		return "QUEUED", "", ""
	case "PREPARING", "RUNNING", "VALIDATING", "PACKAGING", "UPLOADING", "CANCELLING", "REVIEWING":
		return "RUNNING", "", ""
	case "SUCCEEDED":
		return "SUCCEEDED", "TASK_SUCCEEDED", "Task completed successfully."
	case "CANCELLED":
		return "CANCELLED", "TASK_CANCELLED", "Task was cancelled."
	case "TIMED_OUT":
		return "TIMED_OUT", "TASK_TIMED_OUT", "Task exceeded its execution time limit."
	case "INTERACTION_SUSPENDED":
		return "ACTION_REQUIRED", "TASK_INTERACTION_REQUIRED", "Task is waiting for an interaction answer."
	case "MANUAL_REVIEW":
		return "ACTION_REQUIRED", "TASK_ACTION_REQUIRED", "Task requires operator action."
	default:
		return "FAILED", "TASK_FAILED", "Task could not be completed."
	}
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
