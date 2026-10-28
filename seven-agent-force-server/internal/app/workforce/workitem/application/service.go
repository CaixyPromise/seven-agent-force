package application

import (
	"context"
	"encoding/json"
	"errors"
	pf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	tf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	d "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/domain"
	"github.com/jackc/pgx/v5"
	"io"
	"strconv"
	"time"
)

type IDs interface{ NextID() int64 }
type Tx interface {
	Native() pgx.Tx
	Get(context.Context, int64, int64) (d.Item, error)
	Replay(context.Context, int64, int64, int64, string, string) (*d.Item, error)
	Insert(context.Context, d.Item) error
	Save(context.Context, d.Item) error
	Decision(context.Context, int64, int64, int64, string, string, d.Item) error
}
type Repository interface {
	InTx(context.Context, func(Tx) error) error
	Get(context.Context, int64, int64) (d.Item, error)
	List(context.Context, int64, int64, int) ([]d.Item, error)
	Active(context.Context, int) ([]d.Item, error)
}
type UsageReader interface {
	UsageForTasks(context.Context, int64, []int64) (json.RawMessage, error)
}
type Service struct {
	repo    Repository
	tasks   tf.StageExecutor
	outputs pf.ResultOutputAccess
	ids     IDs
	usage   UsageReader
}

func New(r Repository, t tf.StageExecutor, p pf.ResultOutputAccess, ids IDs, u UsageReader) *Service {
	return &Service{r, t, p, ids, u}
}
func stageRequest(i d.Item, purpose string) tf.StageRequest {
	selection := i.Request.PlanSelection
	name := "方案：" + i.Name
	instruction := i.Instruction
	var approved json.RawMessage
	if purpose == "IMPLEMENT" {
		selection = i.Request.ImplementationSelection
		name = "实现：" + i.Name
		approved, _ = json.Marshal(i.CurrentPlan)
	}
	req := selection.TaskRequest()
	req.Instruction = instruction
	req.Name = name
	req.ReferenceAssetIDs = i.Request.ReferenceAssetIDs
	req.DeliveryPreference = i.Request.DeliveryPreference
	return tf.StageRequest{TenantID: i.TenantID, ActorID: i.Creator, ProjectID: i.ProjectID, ProjectVersionID: i.ProjectVersionID, SourceRevisionID: i.Source.RevisionID, Purpose: purpose, Name: name, Instruction: instruction, Selection: req, ApprovedPlan: approved}
}
func (s *Service) Create(ctx context.Context, tenant, actor, project int64, key string, r d.Create) (d.Item, error) {
	if tenant < 1 || actor < 1 || project < 1 || len(key) < 1 || len(key) > 128 {
		return d.Item{}, d.ErrInvalid
	}
	if e := r.Validate(); e != nil {
		return d.Item{}, e
	}
	b, _ := json.Marshal(struct {
		Project int64
		Request d.Create
	}{project, r})
	hash := d.Hash(b)
	var item d.Item
	var replay bool
	err := s.repo.InTx(ctx, func(tx Tx) error {
		x, e := tx.Replay(ctx, tenant, actor, 0, key, hash)
		if e != nil {
			return e
		}
		if x != nil {
			item = *x
			replay = true
		}
		return nil
	})
	if err != nil || replay {
		return item, err
	}
	item = d.Item{ID: s.ids.NextID(), TenantID: tenant, ProjectID: project, Creator: actor, Name: r.Name, Instruction: r.Instruction, Status: "PLANNING", Version: 1, Request: r, Usage: json.RawMessage(`{"mode":"OBSERVE"}`), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	prepared, e := s.tasks.PrepareStage(ctx, stageRequest(item, "PLAN"))
	if e != nil {
		return d.Item{}, e
	}
	item.Source = prepared.Source()
	item.ProjectVersionID = item.Source.ProjectVersionID
	err = s.repo.InTx(ctx, func(tx Tx) error {
		x, e := tx.Replay(ctx, tenant, actor, 0, key, hash)
		if e != nil {
			return e
		}
		if x != nil {
			item = *x
			return nil
		}
		task, e := prepared.CreateTx(ctx, tx.Native(), "workitem:"+decimal(item.ID)+":plan:1")
		if e != nil {
			return e
		}
		item.Runs = []d.Run{{ID: s.ids.NextID(), Stage: "PLAN", Number: 1, TaskID: task.ID, Status: task.Status}}
		if e = tx.Insert(ctx, item); e != nil {
			return e
		}
		return tx.Decision(ctx, tenant, actor, 0, key, hash, item)
	})
	item.Actions()
	return item, err
}

type Approve struct {
	ExpectedVersion int64  `json:"expectedVersion,string"`
	PlanRevisionID  int64  `json:"planRevisionId,string"`
	ContentDigest   string `json:"contentDigest"`
	Decision        string `json:"decision"`
}

type EditPlan struct {
	ExpectedVersion int64           `json:"expectedVersion,string"`
	PlanRevisionID  int64           `json:"planRevisionId,string"`
	ContentDigest   string          `json:"contentDigest"`
	Plan            json.RawMessage `json:"plan"`
}

// Edit creates an immutable user-authored revision, including resolutions to
// blocking questions, without invoking a model.
func (s *Service) Edit(ctx context.Context, tenant, actor, id int64, key string, r EditPlan) (d.Item, error) {
	if tenant < 1 || actor < 1 || id < 1 || len(key) < 1 || len(key) > 128 {
		return d.Item{}, d.ErrInvalid
	}
	plan, err := d.ParsePlan(r.Plan)
	if err != nil {
		return d.Item{}, err
	}
	body, _ := json.Marshal(r)
	hash := d.Hash(body)
	var item d.Item
	err = s.repo.InTx(ctx, func(tx Tx) error {
		old, err := tx.Replay(ctx, tenant, actor, id, key, hash)
		if err != nil {
			return err
		}
		if old != nil {
			item = *old
			return nil
		}
		item, err = tx.Get(ctx, tenant, id)
		if err != nil {
			return err
		}
		if item.Status != "AWAITING_PLAN_APPROVAL" || item.Version != r.ExpectedVersion || item.CurrentPlan == nil || item.CurrentPlan.ID != r.PlanRevisionID || item.CurrentPlan.Digest != r.ContentDigest {
			return d.ErrConflict
		}
		previous := item.CurrentPlan
		rev := d.Revision{ID: s.ids.NextID(), Number: previous.Number + 1, ParentRevisionID: previous.ID, TaskID: previous.TaskID, OutputID: previous.OutputID, ArtifactSHA256: previous.ArtifactSHA256, Plan: plan, Source: item.Source, Origin: "USER_AUTHORED"}
		_, rev.Digest = rev.DigestBody()
		item.CurrentPlan = &rev
		item.Version++
		if err = tx.Save(ctx, item); err != nil {
			return err
		}
		return tx.Decision(ctx, tenant, actor, id, key, hash, item)
	})
	item.Actions()
	return item, err
}

type RevisePlan struct {
	ExpectedVersion int64  `json:"expectedVersion,string"`
	PlanRevisionID  int64  `json:"planRevisionId,string"`
	ContentDigest   string `json:"contentDigest"`
	Feedback        string `json:"feedback"`
}

// Revise creates a new planner Task only on an explicit user request.
func (s *Service) Revise(ctx context.Context, tenant, actor, id int64, key string, r RevisePlan) (d.Item, error) {
	if tenant < 1 || actor < 1 || id < 1 || len(key) < 1 || len(key) > 128 || !tf.ValidInstruction(r.Feedback) {
		return d.Item{}, d.ErrInvalid
	}
	body, _ := json.Marshal(r)
	hash := d.Hash(body)
	var item d.Item
	var replay bool
	err := s.repo.InTx(ctx, func(tx Tx) error {
		old, e := tx.Replay(ctx, tenant, actor, id, key, hash)
		if old != nil {
			item = *old
			replay = true
		}
		return e
	})
	if err != nil || replay {
		return item, err
	}
	item, err = s.repo.Get(ctx, tenant, id)
	if err != nil {
		return item, err
	}
	valid := func(i d.Item) bool {
		if i.Version != r.ExpectedVersion || (i.Status != "AWAITING_PLAN_APPROVAL" && !(i.Status == "NEEDS_ATTENTION" && len(i.Runs) > 0 && i.Runs[len(i.Runs)-1].Stage == "PLAN")) {
			return false
		}
		if i.CurrentPlan == nil {
			return r.PlanRevisionID == 0 && r.ContentDigest == ""
		}
		return i.CurrentPlan.ID == r.PlanRevisionID && i.CurrentPlan.Digest == r.ContentDigest
	}
	if !valid(item) {
		return item, d.ErrConflict
	}
	req := stageRequest(item, "PLAN")
	req.ActorID = actor
	req.Instruction = item.Instruction + "\n\nRevise the previous plan using this user feedback:\n" + r.Feedback
	if !tf.ValidInstruction(req.Instruction) {
		return item, d.ErrInvalid
	}
	if item.CurrentPlan != nil {
		req.ApprovedPlan, _ = json.Marshal(item.CurrentPlan)
	}
	prepared, err := s.tasks.PrepareStage(ctx, req)
	if err != nil {
		return item, err
	}
	if prepared.Source() != item.Source {
		return item, errors.New("WORK_ITEM_SOURCE_MISMATCH")
	}
	err = s.repo.InTx(ctx, func(tx Tx) error {
		old, e := tx.Replay(ctx, tenant, actor, id, key, hash)
		if e != nil {
			return e
		}
		if old != nil {
			item = *old
			return nil
		}
		current, e := tx.Get(ctx, tenant, id)
		if e != nil {
			return e
		}
		if !valid(current) {
			return d.ErrConflict
		}
		number := 1
		for _, run := range current.Runs {
			if run.Stage == "PLAN" && run.Number >= number {
				number = run.Number + 1
			}
		}
		task, e := prepared.CreateTx(ctx, tx.Native(), "workitem:"+decimal(id)+":plan:"+strconv.Itoa(number))
		if e != nil {
			return e
		}
		current.Runs = append(current.Runs, d.Run{ID: s.ids.NextID(), Stage: "PLAN", Number: number, TaskID: task.ID, Status: task.Status})
		current.Status = "PLANNING"
		current.ErrorCode = ""
		current.Version++
		item = current
		if e = tx.Save(ctx, item); e != nil {
			return e
		}
		return tx.Decision(ctx, tenant, actor, id, key, hash, item)
	})
	item.Actions()
	return item, err
}

func (s *Service) Approve(ctx context.Context, tenant, actor, id int64, key string, r Approve) (d.Item, error) {
	if actor < 1 || len(key) < 1 || len(key) > 128 || r.Decision != "APPROVE" {
		return d.Item{}, d.ErrInvalid
	}
	b, _ := json.Marshal(r)
	hash := d.Hash(b)
	var item d.Item
	var replay bool
	e := s.repo.InTx(ctx, func(tx Tx) error {
		x, e := tx.Replay(ctx, tenant, actor, id, key, hash)
		if e != nil {
			return e
		}
		if x != nil {
			item = *x
			replay = true
		}
		return nil
	})
	if e != nil || replay {
		return item, e
	}
	item, e = s.repo.Get(ctx, tenant, id)
	if e != nil {
		return item, e
	}
	if e = item.CanApprove(r.ExpectedVersion, r.PlanRevisionID, r.ContentDigest); e != nil {
		return item, e
	}
	request := stageRequest(item, "IMPLEMENT")
	request.ActorID = actor
	prepared, e := s.tasks.PrepareStage(ctx, request)
	if e != nil {
		return item, e
	}
	if prepared.Source() != item.Source {
		return item, errors.New("WORK_ITEM_SOURCE_MISMATCH")
	}
	e = s.repo.InTx(ctx, func(tx Tx) error {
		x, e := tx.Replay(ctx, tenant, actor, id, key, hash)
		if e != nil {
			return e
		}
		if x != nil {
			item = *x
			return nil
		}
		current, e := tx.Get(ctx, tenant, id)
		if e != nil {
			return e
		}
		if e = current.CanApprove(r.ExpectedVersion, r.PlanRevisionID, r.ContentDigest); e != nil {
			return e
		}
		task, e := prepared.CreateTx(ctx, tx.Native(), "workitem:"+decimal(id)+":implement:"+decimal(current.CurrentPlan.ID))
		if e != nil {
			return e
		}
		current.Runs = append(current.Runs, d.Run{ID: s.ids.NextID(), Stage: "IMPLEMENT", Number: 1, TaskID: task.ID, Status: task.Status})
		current.Status = "IMPLEMENTING"
		current.Version++
		item = current
		if e = tx.Save(ctx, item); e != nil {
			return e
		}
		return tx.Decision(ctx, tenant, actor, id, key, hash, item)
	})
	item.Actions()
	return item, e
}
func (s *Service) Get(ctx context.Context, tenant, id int64) (d.Item, error) {
	i, e := s.repo.Get(ctx, tenant, id)
	if e != nil {
		return i, e
	}
	if s.usage != nil {
		ids := make([]int64, 0, len(i.Runs))
		for _, r := range i.Runs {
			ids = append(ids, r.TaskID)
		}
		if usage, err := s.usage.UsageForTasks(ctx, tenant, ids); err == nil {
			i.Usage = usage
		}
	}
	i.Actions()
	return i, nil
}
func (s *Service) List(ctx context.Context, tenant, project int64) ([]d.Item, error) {
	return s.repo.List(ctx, tenant, project, 100)
}
func (s *Service) Reconcile(ctx context.Context) error {
	items, e := s.repo.Active(ctx, 32)
	if e != nil {
		return e
	}
	var joined error
	for _, item := range items {
		joined = errors.Join(joined, s.reconcile(ctx, item))
	}
	return joined
}
func (s *Service) reconcile(ctx context.Context, item d.Item) error {
	if len(item.Runs) == 0 {
		return nil
	}
	run := item.Runs[len(item.Runs)-1]
	task, e := s.tasks.StageTask(ctx, item.TenantID, run.TaskID)
	if e != nil {
		return e
	}
	if item.Status == "CANCELLING" && !terminal(task.Status) && item.CancelRequestedBy > 0 {
		if err := s.tasks.CancelStageTask(ctx, item.TenantID, run.TaskID, item.CancelRequestedBy); err != nil {
			return err
		}
	}
	var revision *d.Revision
	reason := ""
	if item.Status == "PLANNING" && task.Status == "SUCCEEDED" {
		outputs, e := s.outputs.ListAvailableOutputs(ctx, item.TenantID, task.ID)
		if e != nil {
			return e
		}
		for _, o := range outputs {
			if o.Name != "development-plan" {
				continue
			}
			if o.SizeBytes > d.MaxPlanBytes {
				reason = "WORK_ITEM_PLAN_INVALID"
				break
			}
			stream, e := s.outputs.OpenAvailableOutput(ctx, item.TenantID, task.ID, o.ID)
			if e != nil {
				return e
			}
			body, e := io.ReadAll(io.LimitReader(stream.Content, d.MaxPlanBytes+1))
			stream.Content.Close()
			if e != nil {
				return e
			}
			if d.Hash(body) != o.SHA256 {
				reason = "WORK_ITEM_PLAN_DIGEST_MISMATCH"
				break
			}
			plan, e := d.ParsePlan(body)
			if e != nil {
				reason = "WORK_ITEM_PLAN_INVALID"
				break
			}
			rev := d.Revision{ID: s.ids.NextID(), Number: 1, TaskID: task.ID, OutputID: o.ID, ArtifactSHA256: o.SHA256, Plan: plan, Source: item.Source, Origin: "AGENT"}
			if item.CurrentPlan != nil {
				rev.Number = item.CurrentPlan.Number + 1
				rev.ParentRevisionID = item.CurrentPlan.ID
			}
			_, rev.Digest = rev.DigestBody()
			revision = &rev
			break
		}
		if revision == nil && reason == "" {
			return nil
		} // finalization/output may still be reconciling
	}
	var usage json.RawMessage
	if s.usage != nil {
		var ids []int64
		for _, r := range item.Runs {
			ids = append(ids, r.TaskID)
		}
		usage, e = s.usage.UsageForTasks(ctx, item.TenantID, ids)
		if e != nil {
			return e
		}
	}
	return s.repo.InTx(ctx, func(tx Tx) error {
		current, e := tx.Get(ctx, item.TenantID, item.ID)
		if e != nil {
			return e
		}
		if current.Version != item.Version || current.Status != item.Status {
			return nil
		}
		if len(usage) > 0 {
			current.Usage = usage
		}
		current.Runs[len(current.Runs)-1].Status = task.Status
		if current.Status == "CANCELLING" {
			if terminal(task.Status) {
				current.Status = "CANCELLED"
			}
		} else if reason != "" {
			current.Status = "NEEDS_ATTENTION"
			current.ErrorCode = reason
		} else if revision != nil {
			current.CurrentPlan = revision
			current.Status = "AWAITING_PLAN_APPROVAL"
		} else if task.Status == "SUCCEEDED" && run.Stage == "IMPLEMENT" {
			current.Status = "SUCCEEDED"
		} else if task.Status == "FAILED" || task.Status == "CANCELLED" || task.Status == "TIMED_OUT" {
			current.Status = "NEEDS_ATTENTION"
			current.ErrorCode = "EXECUTION_" + task.Status
		}
		// Ordinary progress updates do not invalidate a human approval version.
		if current.Status != item.Status {
			current.Version++
		}
		return tx.Save(ctx, current)
	})
}
func terminal(status string) bool {
	return status == "SUCCEEDED" || status == "FAILED" || status == "CANCELLED" || status == "TIMED_OUT"
}
func (s *Service) Cancel(ctx context.Context, tenant, actor, id, version int64) (d.Item, error) {
	var item d.Item
	e := s.repo.InTx(ctx, func(tx Tx) error {
		i, e := tx.Get(ctx, tenant, id)
		if e != nil {
			return e
		}
		if i.Status == "CANCELLED" {
			item = i
			return nil
		}
		// A cancellation issued against the approval screen remains valid if the
		// sole intervening transition approved that same plan and created the
		// implementation. The row lock ensures we see and cancel the new Task.
		approvalWonRace := i.Status == "IMPLEMENTING" && i.Version == version+1 && i.CurrentPlan != nil
		if (i.Version != version && !approvalWonRace) || i.Status == "SUCCEEDED" {
			return d.ErrConflict
		}
		i.Status = "CANCELLING"
		i.CancelRequestedBy = actor
		i.Version++
		item = i
		return tx.Save(ctx, i)
	})
	if e != nil {
		return item, e
	}
	for _, r := range item.Runs {
		if !terminal(r.Status) {
			if e = s.tasks.CancelStageTask(ctx, tenant, r.TaskID, actor); e != nil {
				return item, e
			}
		}
	}
	return item, nil
}

func decimal(id int64) string { return strconv.FormatInt(id, 10) }
