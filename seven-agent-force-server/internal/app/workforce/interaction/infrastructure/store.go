package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/jackc/pgx/v5"
)

type Store struct{}

func New() *Store { return &Store{} }

var _ application.Repository = (*Store)(nil)

func decode(row pgx.Row) (domain.Interaction, error) {
	var i domain.Interaction
	var raw []byte
	err := row.Scan(&raw, &i.TenantID, &i.PrincipalID, &i.Digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return i, domain.ErrNotFound
	}
	if err != nil {
		return i, err
	}
	err = json.Unmarshal(raw, &i)
	return i, err
}
func (s *Store) FindOccurrence(ctx context.Context, tx pgx.Tx, attempt, generation int64, key string) (*domain.Interaction, error) {
	i, e := decode(tx.QueryRow(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE attempt_id=$1 AND adapter_session_generation=$2 AND local_request_key=$3`, attempt, generation, key))
	if errors.Is(e, domain.ErrNotFound) {
		return nil, nil
	}
	return &i, e
}
func (s *Store) FindMessage(ctx context.Context, tx pgx.Tx, tenant, task, actor int64, key string) (*domain.Interaction, error) {
	i, e := decode(tx.QueryRow(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=$2 AND actor_principal_id=$3 AND local_request_key=$4 AND kind='USER_MESSAGE'`, tenant, task, actor, key))
	if errors.Is(e, domain.ErrNotFound) {
		return nil, nil
	}
	return &i, e
}
func (s *Store) Get(ctx context.Context, tx pgx.Tx, tenant, task, id int64) (domain.Interaction, error) {
	return decode(tx.QueryRow(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=$2 AND id=$3 FOR UPDATE`, tenant, task, id))
}
func (s *Store) Put(ctx context.Context, tx pgx.Tx, i domain.Interaction, insert bool) error {
	raw, e := json.Marshal(i)
	if e != nil {
		return e
	}
	if insert {
		_, e = tx.Exec(ctx, `INSERT INTO workforce_agent_interactions(id,tenant_id,task_id,task_turn_id,attempt_id,adapter_session_generation,local_request_key,actor_principal_id,kind,state,version,request_digest,body_json,created_at,expires_at,delivery_state,is_blocking) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`, i.ID, i.TenantID, i.TaskID, nullable(i.TaskTurnID), nullable(i.AttemptID), nullable(i.AdapterSessionGeneration), i.LocalRequestKey, i.PrincipalID, i.Kind, i.State, i.Version, i.Digest, raw, i.CreatedAt, i.ExpiresAt, i.DeliveryState, i.IsBlocking)
	} else {
		result, err := tx.Exec(ctx, `UPDATE workforce_agent_interactions SET state=$4,version=$5,body_json=$6,delivery_state=$7,task_turn_id=$8,attempt_id=$9,adapter_session_generation=$10 WHERE tenant_id=$1 AND task_id=$2 AND id=$3 AND version=$5-1`, i.TenantID, i.TaskID, i.ID, i.State, i.Version, raw, i.DeliveryState, nullable(i.TaskTurnID), nullable(i.AttemptID), nullable(i.AdapterSessionGeneration))
		if err != nil {
			return err
		}
		if result.RowsAffected() != 1 {
			return domain.ErrStale
		}
	}
	return e
}
func scanResponse(row pgx.Row) (domain.Response, error) {
	var r domain.Response
	var raw []byte
	e := row.Scan(&r.ID, &r.TenantID, &r.TaskID, &r.InteractionID, &r.PrincipalID, &r.IdempotencyKey, &r.Digest, &raw, &r.CreatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return r, domain.ErrNotFound
	}
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(raw, &r.Answers)
	return r, e
}

const responseCols = `id,tenant_id,task_id,interaction_id,actor_principal_id,idempotency_key,request_digest,answers_json,created_at`

func (s *Store) FindResponse(ctx context.Context, tx pgx.Tx, tenant, task, principal int64, key string) (*domain.Response, error) {
	r, e := scanResponse(tx.QueryRow(ctx, `SELECT `+responseCols+` FROM workforce_agent_interaction_responses WHERE tenant_id=$1 AND task_id=$2 AND actor_principal_id=$3 AND idempotency_key=$4`, tenant, task, principal, key))
	if errors.Is(e, domain.ErrNotFound) {
		return nil, nil
	}
	return &r, e
}
func (s *Store) Response(ctx context.Context, tx pgx.Tx, tenant, id int64) (domain.Response, error) {
	return scanResponse(tx.QueryRow(ctx, `SELECT `+responseCols+` FROM workforce_agent_interaction_responses WHERE tenant_id=$1 AND id=$2`, tenant, id))
}
func (s *Store) InsertResponse(ctx context.Context, tx pgx.Tx, r domain.Response) error {
	raw, e := json.Marshal(r.Answers)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO workforce_agent_interaction_responses(`+responseCols+`) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, r.ID, r.TenantID, r.TaskID, r.InteractionID, r.PrincipalID, r.IdempotencyKey, r.Digest, raw, r.CreatedAt)
	return e
}
func (s *Store) Pending(ctx context.Context, tx pgx.Tx, tenant, task, turn, attempt, generation int64, limit int) ([]domain.Interaction, error) {
	rows, e := tx.Query(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=$2 AND state='RESOLVED' AND delivery_state='SAVED' AND (((task_turn_id=$3 OR task_turn_id IS NULL) AND ((attempt_id=$4 AND adapter_session_generation=$5) OR (kind='USER_MESSAGE' AND attempt_id IS NULL)) AND COALESCE(body_json->>'resumeTaskTurnId','0')='0') OR (kind='QUESTION' AND body_json->>'resumeTaskTurnId'=$3::text)) ORDER BY id LIMIT $6`, tenant, task, turn, attempt, generation, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Interaction{}
	for rows.Next() {
		i, e := decode(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func nullable(n int64) any {
	if n == 0 {
		return nil
	}
	return n
}
func (s *Store) History(ctx context.Context, tx pgx.Tx, tenant, task, before int64, limit int) ([]domain.Item, error) {
	rows, e := tx.Query(ctx, `SELECT id,version,type,body_json,created_at,change_sequence FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=$2 AND ($3::bigint=0 OR id<$3) ORDER BY id DESC LIMIT $4`, tenant, task, before, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Item{}
	for rows.Next() {
		var i domain.Item
		if e = rows.Scan(&i.ID, &i.Version, &i.Type, &i.Body, &i.CreatedAt, &i.Sequence); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) Item(ctx context.Context, tx pgx.Tx, tenant, task, id int64) (domain.Item, error) {
	var i domain.Item
	e := tx.QueryRow(ctx, `SELECT id,version,type,body_json,created_at,change_sequence FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=$2 AND id=$3`, tenant, task, id).Scan(&i.ID, &i.Version, &i.Type, &i.Body, &i.CreatedAt, &i.Sequence)
	if errors.Is(e, pgx.ErrNoRows) {
		e = domain.ErrNotFound
	}
	return i, e
}
func (s *Store) PutItem(ctx context.Context, tx pgx.Tx, i domain.Item) error {
	_, e := tx.Exec(ctx, `INSERT INTO workforce_task_conversation_items(id,tenant_id,task_id,interaction_id,version,type,body_json,created_at,change_sequence,source_kind,source_key,request_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(id) DO UPDATE SET version=EXCLUDED.version,body_json=EXCLUDED.body_json,change_sequence=EXCLUDED.change_sequence`, i.ID, i.TenantID, i.TaskID, nullable(i.InteractionID), i.Version, i.Type, []byte(i.Body), i.CreatedAt, i.Sequence, i.SourceKind, i.SourceKey, i.Digest)
	return e
}
func (s *Store) FindItem(ctx context.Context, tx pgx.Tx, tenant, task int64, kind, key string) (*domain.Item, error) {
	var i domain.Item
	e := tx.QueryRow(ctx, `SELECT id,version,type,body_json,created_at,change_sequence,request_digest FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=$2 AND source_kind=$3 AND source_key=$4`, tenant, task, kind, key).Scan(&i.ID, &i.Version, &i.Type, &i.Body, &i.CreatedAt, &i.Sequence, &i.Digest)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &i, e
}
func (s *Store) OpenQuestions(ctx context.Context, tx pgx.Tx, tenant, task int64) ([]domain.Interaction, error) {
	rows, e := tx.Query(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=$2 AND kind='QUESTION' AND state='OPEN' ORDER BY id FOR UPDATE`, tenant, task)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Interaction{}
	for rows.Next() {
		i, e := decode(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) Items(ctx context.Context, tx pgx.Tx, tenant, task, after int64, limit int) ([]domain.Item, error) {
	rows, e := tx.Query(ctx, `SELECT id,version,type,body_json,created_at,change_sequence FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=$2 AND change_sequence>$3 ORDER BY change_sequence LIMIT $4`, tenant, task, after, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []domain.Item{}
	for rows.Next() {
		var i domain.Item
		if e = rows.Scan(&i.ID, &i.Version, &i.Type, &i.Body, &i.CreatedAt, &i.Sequence); e != nil {
			return nil, e
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
func (s *Store) Watermark(ctx context.Context, tx pgx.Tx, task int64) (int64, error) {
	var n int64
	e := tx.QueryRow(ctx, `SELECT COALESCE(MAX(change_sequence),0) FROM workforce_task_conversation_items WHERE task_id=$1`, task).Scan(&n)
	return n, e
}
func (s *Store) ReadState(ctx context.Context, tx pgx.Tx, tenant, task, actor int64) (int64, error) {
	var n int64
	e := tx.QueryRow(ctx, `SELECT watermark FROM workforce_task_conversation_read_states WHERE tenant_id=$1 AND task_id=$2 AND actor_principal_id=$3`, tenant, task, actor).Scan(&n)
	if errors.Is(e, pgx.ErrNoRows) {
		return 0, nil
	}
	return n, e
}
func (s *Store) PutReadState(ctx context.Context, tx pgx.Tx, id, tenant, task, actor, watermark int64) error {
	_, e := tx.Exec(ctx, `INSERT INTO workforce_task_conversation_read_states(id,tenant_id,task_id,actor_principal_id,watermark,updated_at) VALUES($1,$2,$3,$4,$5,CURRENT_TIMESTAMP) ON CONFLICT(tenant_id,task_id,actor_principal_id) DO UPDATE SET watermark=EXCLUDED.watermark,updated_at=EXCLUDED.updated_at`, id, tenant, task, actor, watermark)
	return e
}
