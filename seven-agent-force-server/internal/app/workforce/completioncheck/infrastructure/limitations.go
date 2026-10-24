package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/jackc/pgx/v5"
	"sort"
)

// AcceptedLimitations is authority for one exact observed failure set. It does
// not authorize missing evidence, new failures or changing the frozen check.
func (s *Store) AcceptedLimitations(ctx context.Context, tx pgx.Tx, tenant, task, turn int64, digest string, current facade.Observation) (bool, error) {
	if current.CheckDigest != digest || current.Status != "FAILED" || !hexDigest.MatchString(current.WorkspaceDigest) {
		return false, nil
	}
	// Read only this tenant's exact Task graph, then validate the complete
	// ancestor chain in Go. Missing edges and cycles fail closed; unrelated
	// branches never enter the set of eligible decision Turn identities.
	rows, err := tx.Query(ctx, `SELECT id,COALESCE(source_turn_id,0),COALESCE(completion_check_baseline_result_id,0) FROM workforce_task_turns WHERE tenant_id=$1 AND task_id=$2`, tenant, task)
	if err != nil {
		return false, err
	}
	type edge struct{ parent, baseline int64 }
	edges := map[int64]edge{}
	for rows.Next() {
		var id int64
		var e edge
		if err = rows.Scan(&id, &e.parent, &e.baseline); err != nil {
			rows.Close()
			return false, err
		}
		edges[id] = e
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	root, exists := edges[turn]
	if !exists || root.baseline <= 0 {
		return false, nil
	}
	ancestors := make([]int64, 0, len(edges))
	seen := map[int64]bool{}
	for id := turn; id != 0; {
		e, exists := edges[id]
		if !exists || seen[id] {
			return false, nil
		}
		seen[id] = true
		ancestors = append(ancestors, id)
		id = e.parent
	}
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT r.observation_json FROM workforce_completion_check_decisions d JOIN workforce_completion_check_results r ON r.id=d.result_id AND r.tenant_id=$1 AND r.task_id=$2 WHERE d.tenant_id=$1 AND d.task_id=$2 AND d.task_turn_id=ANY($3::BIGINT[]) AND d.action='CONTINUE_WITH_LIMITATIONS' AND r.check_digest=$4 AND d.baseline_result_id=$5 ORDER BY d.id DESC LIMIT 1`, tenant, task, ancestors, digest, root.baseline).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var approved facade.Observation
	if err = json.Unmarshal(raw, &approved); err != nil {
		return false, err
	}
	if approved.Status != "FAILED" || approved.CheckDigest != digest || approved.WorkspaceDigest != current.WorkspaceDigest || len(approved.Failures) != len(current.Failures) {
		return false, nil
	}
	if len(current.Failures) == 0 && (!current.DiagnosticKnown || !approved.DiagnosticKnown || current.DiagnosticTruncated || approved.DiagnosticTruncated || !hexDigest.MatchString(current.DiagnosticDigest) || approved.DiagnosticDigest != current.DiagnosticDigest) {
		return false, nil
	}
	signatures := func(o facade.Observation) []string {
		out := make([]string, 0, len(o.Failures))
		for _, f := range o.Failures {
			out = append(out, f.Key+"\x00"+f.Path+"\x00"+f.Fingerprint)
		}
		sort.Strings(out)
		return out
	}
	a, b := signatures(approved), signatures(current)
	for i := range a {
		if a[i] != b[i] {
			return false, nil
		}
	}
	return true, nil
}
