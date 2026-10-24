package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	sharedtransport "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/transport"
)

const (
	SourcePending                   = "PENDING"
	SourceAvailable                 = "AVAILABLE"
	ReferenceAvailable              = "AVAILABLE"
	DeliveryNone                    = "NONE"
	DeliveryNewBranch               = "NEW_BRANCH"
	DeliveryNewBranchAndDraftReview = "NEW_BRANCH_AND_DRAFT_REVIEW"
	DeliveryAuthorizationAuto       = "AUTO_AFTER_GATES"
	DeliveryAuthorizationAsk        = "ASK_AFTER_GATES"
	SourceIngestionMessageKind      = "seven.source-ingestion-run"
)

var (
	ErrTaskInputInvalid      = errors.New("TASK_INPUT_INVALID")
	ErrSourceNotAvailable    = errors.New("WORKSPACE_SOURCE_REVISION_NOT_AVAILABLE")
	ErrReferenceNotAvailable = errors.New("REFERENCE_ASSET_NOT_AVAILABLE")
	ErrGitPlanInvalid        = errors.New("TASK_GIT_PLAN_INVALID")
)

type SourceRevisionView struct {
	ID                   int64
	TenantID             int64
	Status               string
	SourceCommit         string
	SourceTree           string
	SourceIdentityDigest string
}

type ReferenceAssetView struct {
	ID            int64
	TenantID      int64
	Status        string
	ContentSHA256 string
	SizeBytes     int64
}

type GitPlan struct {
	DeliveryMode               string `json:"deliveryMode"`
	DeliveryAuthorization      string `json:"deliveryAuthorization"`
	TargetConfigDigest         string `json:"targetConfigDigest,omitempty"`
	PushAuthorizationDigest    string `json:"pushAuthorizationDigest,omitempty"`
	ReviewAuthorizationDigest  string `json:"reviewAuthorizationDigest,omitempty"`
	ProjectID                  int64  `json:"projectId,string,omitempty"`
	ProjectVersionID           int64  `json:"projectVersionId,string,omitempty"`
	RepositoryConfigDigest     string `json:"repositoryConfigDigest,omitempty"`
	ReviewBaseSourceRevisionID int64  `json:"reviewBaseSourceRevisionId,string,omitempty"`
	ReviewBaseBranch           string `json:"reviewBaseBranch,omitempty"`
	ReviewBaseCommit           string `json:"reviewBaseCommit,omitempty"`
	ResultBranch               string `json:"resultBranch,omitempty"`
	FullProjectImport          bool   `json:"fullProjectImport"`
}

type RepositoryPolicy struct {
	ReviewSupported bool
}

type TaskInputSnapshot struct {
	SourceKind                string                     `json:"sourceKind"`
	WorkspaceSourceRevisionID int64                      `json:"workspaceSourceRevisionId,string"`
	SourceCommit              string                     `json:"sourceCommit"`
	SourceTree                string                     `json:"sourceTree"`
	SourceIdentityDigest      string                     `json:"sourceIdentityDigest"`
	ReferenceAssetIDs         sharedtransport.Int64Slice `json:"referenceAssetIds"`
	ReferenceIdentityDigests  []string                   `json:"referenceIdentityDigests"`
	GitPlan                   GitPlan                    `json:"gitPlan"`
	Digest                    string                     `json:"digest"`
}

type SourceIngestionMessage struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"`
	MessageID     int64  `json:"messageId,string"`
	WorkerID      int64  `json:"workerId,string"`
	ResourceID    int64  `json:"resourceId,string"`
	Generation    int64  `json:"generation,string"`
}

func FreezeEmptyTaskInput(tenantID int64, references []ReferenceAssetView, plan GitPlan) (TaskInputSnapshot, error) {
	if tenantID <= 0 || plan.DeliveryMode != DeliveryNone {
		return TaskInputSnapshot{}, ErrTaskInputInvalid
	}
	identity := CanonicalEmptyWorkspaceIdentity()
	return freezeTaskInputIdentity(tenantID, SourceKindEmptyWorkspace, SourceRevisionView{
		TenantID: tenantID, Status: SourceAvailable, SourceCommit: identity.Commit,
		SourceTree: identity.Tree, SourceIdentityDigest: identity.IdentityDigest,
	}, references, plan)
}

func FreezeTaskInput(tenantID int64, source SourceRevisionView, references []ReferenceAssetView, plan GitPlan) (TaskInputSnapshot, error) {
	return freezeTaskInputIdentity(tenantID, SourceKindSourceRevision, source, references, plan)
}

func freezeTaskInputIdentity(tenantID int64, sourceKind string, source SourceRevisionView, references []ReferenceAssetView, plan GitPlan) (TaskInputSnapshot, error) {
	if tenantID <= 0 || source.TenantID != tenantID || source.Status != SourceAvailable || !hexLen(source.SourceCommit, 40) || !hexLen(source.SourceTree, 40) || !hexLen(source.SourceIdentityDigest, 64) {
		return TaskInputSnapshot{}, ErrSourceNotAvailable
	}
	ids, digests := make([]int64, 0, len(references)), make([]string, 0, len(references))
	seen := map[int64]struct{}{}
	for _, item := range references {
		if item.TenantID != tenantID || item.Status != ReferenceAvailable || item.ID <= 0 || !hexLen(item.ContentSHA256, 64) || item.SizeBytes < 0 {
			return TaskInputSnapshot{}, ErrReferenceNotAvailable
		}
		if _, ok := seen[item.ID]; ok {
			return TaskInputSnapshot{}, ErrTaskInputInvalid
		}
		seen[item.ID] = struct{}{}
		ids = append(ids, item.ID)
		digests = append(digests, item.ContentSHA256)
	}
	pairs := make([]struct {
		id     int64
		digest string
	}, len(ids))
	for i := range ids {
		pairs[i] = struct {
			id     int64
			digest string
		}{ids[i], digests[i]}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].id < pairs[j].id })
	for i := range pairs {
		ids[i], digests[i] = pairs[i].id, pairs[i].digest
	}
	item := TaskInputSnapshot{SourceKind: sourceKind, WorkspaceSourceRevisionID: source.ID, SourceCommit: source.SourceCommit, SourceTree: source.SourceTree, SourceIdentityDigest: source.SourceIdentityDigest, ReferenceAssetIDs: sharedtransport.Int64Slice(ids), ReferenceIdentityDigests: digests, GitPlan: plan}
	body, _ := json.Marshal(item)
	sum := sha256.Sum256(body)
	item.Digest = hex.EncodeToString(sum[:])
	return item, nil
}

func ValidateGitPlan(plan GitPlan, policy RepositoryPolicy) error {
	if plan.DeliveryMode != DeliveryNone && plan.DeliveryMode != DeliveryNewBranch && plan.DeliveryMode != DeliveryNewBranchAndDraftReview {
		return ErrGitPlanInvalid
	}
	if plan.DeliveryMode == DeliveryNone {
		if plan.DeliveryAuthorization != "" || plan.TargetConfigDigest != "" || plan.PushAuthorizationDigest != "" || plan.ReviewAuthorizationDigest != "" || plan.ProjectID != 0 || plan.ProjectVersionID != 0 || plan.RepositoryConfigDigest != "" || plan.ReviewBaseSourceRevisionID != 0 || plan.ReviewBaseBranch != "" || plan.ReviewBaseCommit != "" || plan.ResultBranch != "" {
			return ErrGitPlanInvalid
		}
		return nil
	}
	if plan.DeliveryAuthorization != DeliveryAuthorizationAuto && plan.DeliveryAuthorization != DeliveryAuthorizationAsk {
		return ErrGitPlanInvalid
	}
	if plan.ProjectID <= 0 || plan.ProjectVersionID <= 0 || !hexLen(plan.RepositoryConfigDigest, 64) || !validBranch(plan.ResultBranch) || !validBranch(plan.ReviewBaseBranch) || !hexLen(plan.ReviewBaseCommit, 40) || plan.ResultBranch == plan.ReviewBaseBranch {
		return ErrGitPlanInvalid
	}
	if !hexLen(plan.TargetConfigDigest, 64) || !hexLen(plan.PushAuthorizationDigest, 64) || (plan.DeliveryMode == DeliveryNewBranchAndDraftReview && !hexLen(plan.ReviewAuthorizationDigest, 64)) || (plan.DeliveryMode == DeliveryNewBranch && plan.ReviewAuthorizationDigest != "") {
		return ErrGitPlanInvalid
	}
	if plan.DeliveryMode == DeliveryNewBranchAndDraftReview && !policy.ReviewSupported {
		return ErrGitPlanInvalid
	}
	return nil
}

func validBranch(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= 255 && !strings.HasPrefix(value, "refs/") && !strings.HasPrefix(value, "-") && !strings.Contains(value, "..") && !strings.Contains(value, "//") && !strings.ContainsAny(value, " ~^:?*[\\\x00\r\n") && !strings.HasSuffix(value, ".") && !strings.HasSuffix(value, "/")
}
func hexLen(value string, n int) bool {
	if len(value) != n {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
