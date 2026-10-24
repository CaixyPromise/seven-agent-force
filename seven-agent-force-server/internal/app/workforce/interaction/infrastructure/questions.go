package infrastructure

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) QuestionHistory(ctx context.Context, tx pgx.Tx, tenant, task, before int64, limit int) ([]domain.Item, error) {
	rows, e := tx.Query(ctx, `SELECT id,version,type,body_json,created_at,change_sequence FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=$2 AND type='QUESTION' AND ($3::bigint=0 OR id<$3) ORDER BY id DESC LIMIT $4`, tenant, task, before, limit)
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
