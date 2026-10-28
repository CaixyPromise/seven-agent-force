package infrastructure

import (
	"context"
	"encoding/json"
	"strconv"
)

func (s *Store) UsageForTasks(ctx context.Context, tenant int64, ids []int64) (json.RawMessage, error) {
	var calls, known, unknown, inflight, tokens int64
	e := s.db.QueryRow(ctx, `SELECT count(*),count(total_tokens),count(*) FILTER(WHERE total_tokens IS NULL),count(*) FILTER(WHERE status IN('AUTHORIZED','STREAMING')),COALESCE(sum(total_tokens),0) FROM workforce_model_invocations WHERE tenant_id=$1 AND task_id=ANY($2::BIGINT[])`, tenant, ids).Scan(&calls, &known, &unknown, &inflight, &tokens)
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(map[string]any{"mode": "OBSERVE", "calls": strconv.FormatInt(calls, 10), "knownUsageInvocationCount": strconv.FormatInt(known, 10), "unknownUsageInvocationCount": strconv.FormatInt(unknown, 10), "inFlightCount": strconv.FormatInt(inflight, 10), "knownTotalTokens": strconv.FormatInt(tokens, 10), "estimatedCost": nil})
	return b, e
}
