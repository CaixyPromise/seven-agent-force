package domain

import (
	"encoding/json"
	"errors"
	"strings"

	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
)

var (
	ErrNotFound                 = errors.New("PROJECT_NOT_FOUND")
	ErrArchived                 = errors.New("PROJECT_ARCHIVED")
	ErrConflict                 = errors.New("PROJECT_VERSION_CONFLICT")
	ErrInvalid                  = errors.New("PROJECT_CONFIGURATION_INVALID")
	ErrDeliveryApprovalRequired = errors.New("PROJECT_DELIVERY_APPROVAL_PERMISSION_REQUIRED")
	ErrSourceUnavailable        = errors.New("PROJECT_SOURCE_NOT_READY")
	ErrIdempotencyConflict      = errors.New("PROJECT_IDEMPOTENCY_CONFLICT")
)

type Project struct {
	ID                              int64                      `json:"id,string"`
	TenantID                        int64                      `json:"tenantId,string"`
	ProjectKey                      string                     `json:"projectKey"`
	Name                            string                     `json:"name"`
	Description                     string                     `json:"description,omitempty"`
	DepartmentID                    int64                      `json:"departmentId,string,omitempty"`
	Kind                            string                     `json:"kind"`
	Status                          string                     `json:"status"`
	VersionID                       int64                      `json:"versionId,string"`
	RepositoryProviderType          string                     `json:"repositoryProvider,omitempty"`
	RepositoryScheme                string                     `json:"-"`
	RepositoryHost                  string                     `json:"-"`
	RepositoryPort                  int                        `json:"-"`
	RepositoryPath                  string                     `json:"-"`
	RepositoryDisplayURL            string                     `json:"repositoryDisplayUrl,omitempty"`
	RepositoryWebURL                string                     `json:"repositoryWebUrl,omitempty"`
	RepositoryCredentialCiphertext  string                     `json:"-"`
	RepositoryCredentialEDEK        string                     `json:"-"`
	RepositoryCredentialWrapKeyRef  string                     `json:"-"`
	RepositoryCredentialVersion     int64                      `json:"-"`
	RepositoryConfigDigest          string                     `json:"-"`
	RepositorySourceState           string                     `json:"repositorySourceState,omitempty"`
	RepositoryCapabilities          RepositoryCapabilities     `json:"repositoryCapabilities,omitempty"`
	RepositoryLastCheckStatus       string                     `json:"-"`
	RepositoryLastCheckReasonCode   string                     `json:"repositoryReasonCode,omitempty"`
	RepositoryLastCheckedAt         string                     `json:"repositoryLastCheckedAt,omitempty"`
	RepositorySourceRevisionID      int64                      `json:"-"`
	DefaultRefName                  string                     `json:"defaultRefName,omitempty"`
	DepartmentName                  string                     `json:"departmentName,omitempty"`
	TechStack                       []string                   `json:"techStack"`
	AgentGuidance                   string                     `json:"agentGuidance,omitempty"`
	CompletionCheck                 json.RawMessage            `json:"completionCheck"`
	CompletionCheckDigest           string                     `json:"completionCheckDigest,omitempty"`
	ResultProcessorReleaseID        int64                      `json:"resultProcessorReleaseId,string,omitempty"`
	ResultProcessorParameters       json.RawMessage            `json:"resultProcessorParameters"`
	ReviewPrompt                    string                     `json:"reviewPrompt,omitempty"`
	HookReleaseIDs                  sharedtransport.Int64Slice `json:"hookReleaseIds,omitempty"`
	Hooks                           []HookRelease              `json:"hooks,omitempty"`
	DeliveryMode                    string                     `json:"deliveryMode"`
	DeliveryAuthorization           string                     `json:"deliveryAuthorization,omitempty"`
	DeliveryAuthorizedByPrincipalID int64                      `json:"-"`
	ResultBranchPattern             string                     `json:"resultBranchPattern,omitempty"`
	NotificationEnabled             bool                       `json:"notificationEnabled"`
	NotificationEvents              []string                   `json:"notificationEvents"`
	NotificationGroupRouteID        int64                      `json:"notificationGroupRouteId,string,omitempty"`
	NotificationGroupRouteName      string                     `json:"notificationGroupRouteName,omitempty"`
	TaskCount                       int                        `json:"taskCount"`
	ActiveTaskCount                 int                        `json:"activeTaskCount"`
	LatestTaskAt                    string                     `json:"latestTaskAt,omitempty"`
	CreatedAt                       string                     `json:"createdAt"`
	UpdatedAt                       string                     `json:"updatedAt"`
	ArchivedAt                      string                     `json:"archivedAt,omitempty"`
}

type HookRelease struct {
	ReleaseID   int64    `json:"releaseId,string"`
	HookID      int64    `json:"hookId,string"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Events      []string `json:"events"`
	FailureMode string   `json:"failureMode"`
}

func NormalizeStringList(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 128 {
			return nil, ErrInvalid
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}
