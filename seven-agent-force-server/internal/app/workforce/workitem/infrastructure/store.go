package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	principal "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/facade"
	a "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/application"
	d "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"strconv"
)

type Store struct {
	db         *pgxpool.Pool
	ids        a.IDs
	principals principal.Resolver
}

func New(db *pgxpool.Pool, ids a.IDs, p principal.Resolver) *Store { return &Store{db, ids, p} }

func (s *Store) IsWorkItemTask(ctx context.Context, tenant, task int64) (bool, error) {
	var found bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_work_item_stage_runs WHERE tenant_id=$1 AND task_id=$2)`, tenant, task).Scan(&found)
	return found, err
}

type transaction struct {
	pgx.Tx
	store *Store
}

func (t *transaction) Native() pgx.Tx { return t.Tx }
func (s *Store) InTx(ctx context.Context, fn func(a.Tx) error) error {
	tx, e := s.db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = fn(&transaction{tx, s}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type query interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func get(ctx context.Context, q query, tenant, id int64, lock bool) (d.Item, error) {
	var row pgx.Row
	if lock {
		row = q.QueryRow(ctx, `SELECT data_json,request_json FROM workforce_work_items WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenant, id)
	} else {
		row = q.QueryRow(ctx, `SELECT data_json,request_json FROM workforce_work_items WHERE tenant_id=$1 AND id=$2`, tenant, id)
	}
	var body, request []byte
	if e := row.Scan(&body, &request); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			e = d.ErrNotFound
		}
		return d.Item{}, e
	}
	var item d.Item
	if e := json.Unmarshal(body, &item); e != nil {
		return item, e
	}
	if e := json.Unmarshal(request, &item.Request); e != nil {
		return item, e
	}
	item.TenantID = tenant
	item.Actions()
	return item, nil
}
func (s *Store) Get(ctx context.Context, tenant, id int64) (d.Item, error) {
	return get(ctx, s.db, tenant, id, false)
}
func (t *transaction) Get(ctx context.Context, tenant, id int64) (d.Item, error) {
	return get(ctx, t.Tx, tenant, id, true)
}
func (s *Store) List(ctx context.Context, tenant, project int64, limit int) ([]d.Item, error) {
	return s.list(ctx, `SELECT data_json FROM workforce_work_items WHERE tenant_id=$1 AND project_id=$2 ORDER BY id DESC LIMIT $3`, tenant, project, limit)
}
func (s *Store) Active(ctx context.Context, limit int) ([]d.Item, error) {
	rows, e := s.db.Query(ctx, `SELECT tenant_id,id FROM workforce_work_items WHERE status IN ('PLANNING','IMPLEMENTING','CANCELLING','AWAITING_PLAN_APPROVAL') ORDER BY updated_at,id LIMIT $1`, limit)
	if e != nil {
		return nil, e
	}
	var refs [][2]int64
	for rows.Next() {
		var r [2]int64
		if e = rows.Scan(&r[0], &r[1]); e != nil {
			rows.Close()
			return nil, e
		}
		refs = append(refs, r)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out := []d.Item{}
	for _, r := range refs {
		i, e := s.Get(ctx, r[0], r[1])
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, nil
}
func (s *Store) list(ctx context.Context, sql string, args ...any) ([]d.Item, error) {
	rows, e := s.db.Query(ctx, sql, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []d.Item{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var item d.Item
		if e = json.Unmarshal(b, &item); e != nil {
			return nil, e
		}
		item.Actions()
		items = append(items, item)
	}
	return items, rows.Err()
}
func (t *transaction) Replay(ctx context.Context, tenant, actor, scope int64, key, hash string) (*d.Item, error) {
	p, e := t.store.principals.EnsureUser(ctx, t.Tx, t.store.ids, actor)
	if e != nil {
		return nil, e
	}
	// Serialize identical create/decision keys before any Task can be created.
	namespace := strconv.FormatInt(tenant, 10) + ":" + strconv.FormatInt(p, 10) + ":" + strconv.FormatInt(scope, 10) + ":" + key
	if _, e = t.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, namespace); e != nil {
		return nil, e
	}
	var old string
	var b []byte
	e = t.QueryRow(ctx, `SELECT request_digest,response_json FROM workforce_work_item_decisions WHERE tenant_id=$1 AND actor_principal_id=$2 AND scope_key=$3 AND request_key=$4`, tenant, p, strconv.FormatInt(scope, 10), key).Scan(&old, &b)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	if old != hash {
		return nil, d.ErrConflict
	}
	var item d.Item
	e = json.Unmarshal(b, &item)
	item.TenantID = tenant
	item.Actions()
	return &item, e
}
func (t *transaction) Insert(ctx context.Context, item d.Item) error {
	body, _ := json.Marshal(item)
	request, _ := json.Marshal(item.Request)
	p, e := t.store.principals.EnsureUser(ctx, t.Tx, t.store.ids, item.Creator)
	if e != nil {
		return e
	}
	_, e = t.Exec(ctx, `INSERT INTO workforce_work_items(id,tenant_id,project_id,project_version_id,created_by_principal_id,status,version,data_json,request_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, item.ID, item.TenantID, item.ProjectID, item.ProjectVersionID, p, item.Status, item.Version, body, request)
	if e != nil {
		return e
	}
	for n, key := range []string{"PLAN", "IMPLEMENT"} {
		if _, e = t.Exec(ctx, `INSERT INTO workforce_work_item_stages(id,tenant_id,work_item_id,stage_key,position,status) VALUES($1,$2,$3,$4,$5,'PENDING')`, t.store.ids.NextID(), item.TenantID, item.ID, key, n); e != nil {
			return e
		}
	}
	return t.Save(ctx, item)
}
func (t *transaction) Save(ctx context.Context, item d.Item) error {
	item.Actions()
	body, _ := json.Marshal(item)
	var oldStatus string
	if e := t.QueryRow(ctx, `SELECT status FROM workforce_work_items WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, item.TenantID, item.ID).Scan(&oldStatus); e != nil {
		return e
	}
	tag, e := t.Exec(ctx, `UPDATE workforce_work_items SET status=$3,version=$4,data_json=$5,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND id=$2`, item.TenantID, item.ID, item.Status, item.Version, body)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return d.ErrNotFound
	}
	if oldStatus != item.Status {
		event, _ := json.Marshal(map[string]any{"from": oldStatus, "to": item.Status, "version": strconv.FormatInt(item.Version, 10), "reasonCode": item.ErrorCode})
		if _, e = t.Exec(ctx, `INSERT INTO workforce_work_item_events(id,tenant_id,work_item_id,kind,data_json) VALUES($1,$2,$3,'STATUS_CHANGED',$4)`, t.store.ids.NextID(), item.TenantID, item.ID, event); e != nil {
			return e
		}
	}
	for _, r := range item.Runs {
		tag, e := t.Exec(ctx, `INSERT INTO workforce_work_item_stage_runs(id,tenant_id,work_item_id,stage_key,run_number,task_id,status) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO UPDATE SET status=EXCLUDED.status WHERE workforce_work_item_stage_runs.task_id=EXCLUDED.task_id`, r.ID, item.TenantID, item.ID, r.Stage, r.Number, r.TaskID, r.Status)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return d.ErrConflict
		}
		_, e = t.Exec(ctx, `UPDATE workforce_work_item_stages SET status=$4,current_run_id=$5 WHERE tenant_id=$1 AND work_item_id=$2 AND stage_key=$3`, item.TenantID, item.ID, r.Stage, r.Status, r.ID)
		if e != nil {
			return e
		}
	}
	if item.CurrentPlan != nil {
		r := item.CurrentPlan
		b, _ := json.Marshal(r)
		_, e = t.Exec(ctx, `INSERT INTO workforce_work_item_plan_revisions(id,tenant_id,work_item_id,revision,source_task_id,source_output_id,content_digest,data_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(id) DO NOTHING`, r.ID, item.TenantID, item.ID, r.Number, r.TaskID, r.OutputID, r.Digest, b)
		if e != nil {
			return e
		}
	}
	_, e = t.Exec(ctx, `INSERT INTO workforce_work_item_usage_snapshots(work_item_id,tenant_id,usage_json) VALUES($1,$2,$3) ON CONFLICT(work_item_id) DO UPDATE SET usage_json=EXCLUDED.usage_json,observed_at=CURRENT_TIMESTAMP`, item.ID, item.TenantID, item.Usage)
	return e
}
func (t *transaction) Decision(ctx context.Context, tenant, actor, scope int64, key, hash string, item d.Item) error {
	p, e := t.store.principals.EnsureUser(ctx, t.Tx, t.store.ids, actor)
	if e != nil {
		return e
	}
	b, _ := json.Marshal(item)
	kind := "CREATE"
	if scope != 0 {
		switch item.Status {
		case "IMPLEMENTING":
			kind = "APPROVE_PLAN"
		case "PLANNING":
			kind = "REVISE_PLAN"
		default:
			kind = "EDIT_PLAN"
		}
	}
	var revisionID any
	var digest any
	if item.CurrentPlan != nil {
		revisionID = item.CurrentPlan.ID
		digest = item.CurrentPlan.Digest
	}
	_, e = t.Exec(ctx, `INSERT INTO workforce_work_item_decisions(id,tenant_id,work_item_id,actor_principal_id,decision_kind,plan_revision_id,plan_content_digest,scope_key,request_key,request_digest,response_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, t.store.ids.NextID(), tenant, item.ID, p, kind, revisionID, digest, strconv.FormatInt(scope, 10), key, hash, b)
	if e != nil {
		return e
	}
	event, _ := json.Marshal(map[string]any{"decision": kind, "version": strconv.FormatInt(item.Version, 10), "planRevisionId": revisionID, "contentDigest": digest})
	_, e = t.Exec(ctx, `INSERT INTO workforce_work_item_events(id,tenant_id,work_item_id,kind,data_json) VALUES($1,$2,$3,$4,$5)`, t.store.ids.NextID(), tenant, item.ID, kind, event)
	return e
}
