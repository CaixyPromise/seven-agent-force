package facade

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

var ErrPlanningHooksUnsupported = errors.New("PLANNING_PROJECT_HOOKS_UNSUPPORTED")

// StageRequest is an internal use-case contract, never decoded by ordinary Task
// endpoints. A stage cannot supply credentials, worker placement or raw policy.
type StageRequest struct {
	TenantID, ActorID, ProjectID, ProjectVersionID, SourceRevisionID int64
	Purpose, Instruction, Name, DeliveryPreference                   string
	Selection                                                        ProjectTaskRequest
	ApprovedPlan                                                     json.RawMessage
}
type StageSource struct {
	ProjectID        int64  `json:"projectId,string"`
	ProjectVersionID int64  `json:"projectVersionId,string"`
	RevisionID       int64  `json:"revisionId,string"`
	Kind             string `json:"kind"`
	Commit           string `json:"commit"`
	IdentityDigest   string `json:"identityDigest"`
}
type StageTask struct {
	ID     int64  `json:"id,string"`
	Status string `json:"status"`
}
type PreparedStage interface {
	Source() StageSource
	CreateTx(context.Context, pgx.Tx, string) (StageTask, error)
}
type StageExecutor interface {
	PrepareStage(context.Context, StageRequest) (PreparedStage, error)
	StageTask(context.Context, int64, int64) (StageTask, error)
	CancelStageTask(context.Context, int64, int64, int64) error
}
