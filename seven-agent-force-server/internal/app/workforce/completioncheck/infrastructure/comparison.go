package infrastructure

import (
	"context"
	"encoding/json"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/jackc/pgx/v5"
)

func (s *Store) LoadComparison(ctx context.Context, tx pgx.Tx, tenant, task, turn, attempt, cycle int64, digest string) (facade.Observation, facade.Observation, error) {
	var before, after facade.Observation
	if tx == nil || tenant <= 0 || task <= 0 || turn <= 0 || attempt <= 0 || cycle <= 0 {
		return before, after, facade.ErrInvalid
	}
	rows, err := tx.Query(ctx, `SELECT phase,observation_json FROM workforce_completion_check_results WHERE tenant_id=$1 AND task_id=$2 AND check_digest=$5 AND ((phase='BASELINE' AND id=(SELECT completion_check_baseline_result_id FROM workforce_task_turns WHERE tenant_id=$1 AND task_id=$2 AND id=$3)) OR (phase='CURRENT' AND task_turn_id=$3 AND attempt_id=$4 AND evaluation_cycle_id=$6))`, tenant, task, turn, attempt, digest, cycle)
	if err != nil {
		return before, after, err
	}
	defer rows.Close()
	for rows.Next() {
		var phase string
		var raw []byte
		if err = rows.Scan(&phase, &raw); err != nil {
			return before, after, err
		}
		target := &after
		if phase == "BASELINE" {
			target = &before
		}
		if err = json.Unmarshal(raw, target); err != nil {
			return before, after, err
		}
		if target.CheckDigest != digest {
			return before, after, facade.ErrConflict
		}
	}
	return before, after, rows.Err()
}
