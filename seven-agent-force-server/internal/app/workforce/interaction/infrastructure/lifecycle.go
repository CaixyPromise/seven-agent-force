package infrastructure

import (
	"context"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ColdQuestion(ctx context.Context, tx pgx.Tx, tenant, task, turn, response int64) (*domain.Interaction, error) {
	i, e := decode(tx.QueryRow(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=$2 AND kind='QUESTION' AND body_json->>'resumeTaskTurnId'=$3::bigint::text AND body_json->>'responseId'=$4::bigint::text FOR UPDATE`, tenant, task, turn, response))
	if errors.Is(e, domain.ErrNotFound) {
		return nil, nil
	}
	return &i, e
}

func (s *Store) SavedSourceMessages(ctx context.Context, tx pgx.Tx, tenant, task, turn, attempt int64) ([]domain.Interaction, error) {
	rows, e := tx.Query(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=$2 AND task_turn_id=$3 AND (attempt_id=$4 OR attempt_id IS NULL) AND kind='USER_MESSAGE' AND state='RESOLVED' AND delivery_state='SAVED' ORDER BY id FOR UPDATE`, tenant, task, turn, attempt)
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

// AttemptInteractions loads lifecycle candidates; the application decides which
// transitions are legal and preserves saved/delivered facts.
func (s *Store) AttemptInteractions(ctx context.Context, tx pgx.Tx, tenant, task, attempt int64) ([]domain.Interaction, error) {
	rows, err := tx.Query(ctx, `SELECT body_json,tenant_id,actor_principal_id,request_digest FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=$2 AND attempt_id=$3 AND state IN ('OPEN','RESOLVED') ORDER BY id FOR UPDATE`, tenant, task, attempt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Interaction{}
	for rows.Next() {
		i, err := decode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}
