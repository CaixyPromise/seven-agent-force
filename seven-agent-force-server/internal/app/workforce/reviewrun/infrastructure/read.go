package infrastructure

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
)

// runViewProjection contains only the business-facing ReviewRun projection.
// APPLIED human decisions are loaded in one bounded batch so history reads use
// two database round trips regardless of page size and every logical read
// remains inside the repository's three-table DG2 width limit.
const runViewProjection = `SELECT r.id,r.task_id,r.task_turn_id,r.status,r.report_verdict,t.quality_outcome,t.status,r.result_tree,
 r.canonical_report_json,r.canonical_report_digest,r.automatic_revision_number,r.automatic_revision_limit,r.no_progress_streak,COALESCE(NULLIF(r.closure_reason_code,''),''),
 COALESCE(tr.review_revision_kind,''),COALESCE(r.successor_task_turn_id,0),r.completion_check_digest,
 to_char(r.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(r.finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'')
FROM workforce_review_runs r
JOIN workforce_tasks t ON t.id=r.task_id AND t.tenant_id=r.tenant_id
JOIN workforce_task_turns tr ON tr.id=r.task_turn_id AND tr.task_id=r.task_id AND tr.tenant_id=r.tenant_id`

func (s *Store) ListRunsForTask(ctx context.Context, tenantID, taskID, cursor int64, limit int) (domain.RunPage, error) {
	rows, err := s.db.Query(ctx, runViewProjection+`
WHERE r.tenant_id=$1 AND r.task_id=$2 AND ($3::BIGINT=0 OR r.id<$3)
ORDER BY r.id DESC LIMIT $4`, tenantID, taskID, cursor, limit+1)
	if err != nil {
		return domain.RunPage{}, err
	}
	defer rows.Close()
	items := make([]domain.RunView, 0, limit+1)
	for rows.Next() {
		item, scanErr := scanRunView(rows)
		if scanErr != nil {
			return domain.RunPage{}, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return domain.RunPage{}, err
	}
	page := domain.RunPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	if err = s.attachHumanDecisions(ctx, tenantID, page.Items); err != nil {
		return domain.RunPage{}, err
	}
	return page, nil
}

func (s *Store) RunDetailForTask(ctx context.Context, tenantID, runID int64) (domain.RunView, error) {
	item, err := scanRunView(s.db.QueryRow(ctx, runViewProjection+` WHERE r.tenant_id=$1 AND r.id=$2`, tenantID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.RunView{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.RunView{}, err
	}
	items := []domain.RunView{item}
	if err = s.attachHumanDecisions(ctx, tenantID, items); err != nil {
		return domain.RunView{}, err
	}
	return items[0], nil
}

type runViewScanner interface {
	Scan(dest ...any) error
}

func scanRunView(row runViewScanner) (domain.RunView, error) {
	var item domain.RunView
	var taskStatus string
	var reportJSON []byte
	var digest string
	var automaticRevisionLimit int
	if err := row.Scan(&item.ID, &item.TaskID, &item.TaskTurnID, &item.Status, &item.Verdict, &item.QualityOutcome, &taskStatus, &item.ResultTree,
		&reportJSON, &digest, &item.AutomaticRevisionNumber, &automaticRevisionLimit, &item.NoProgressStreak, &item.StopReason, &item.RevisionKind,
		&item.SuccessorTaskTurnID, &item.CompletionCheckDigest, &item.CreatedAt, &item.FinishedAt); err != nil {
		return domain.RunView{}, err
	}
	if !domain.ValidCompletionCheckDigest(item.CompletionCheckDigest) {
		return domain.RunView{}, domain.ErrInvalidReviewInput
	}
	if len(reportJSON) > 0 {
		report, recalculated, err := domain.CanonicalizeReport(reportJSON)
		if err != nil || digest == "" || !secureEqual(digest, recalculated) {
			return domain.RunView{}, domain.ErrInvalidReport
		}
		item.Report = &domain.TaskReport{
			Required: true, Status: item.Status, Verdict: report.Verdict, Score: report.Score, Summary: report.Summary,
			Findings: report.Findings, ReviewTests: report.ReviewTests, Limitations: report.Limitations, ReportDigest: digest,
			QualityOutcome: item.QualityOutcome, AutomaticRevisionCount: item.AutomaticRevisionNumber,
			AutomaticRevisionLimit: automaticRevisionLimit, NoProgressStreak: item.NoProgressStreak, StopReason: item.StopReason,
			AvailableActions: reviewActions(taskStatus, report.Verdict, digest), RevisionGuidanceRequired: taskStatus == "MANUAL_REVIEW" && digest == "",
		}
	}
	return item, nil
}

func reviewActions(taskStatus, verdict, reportDigest string) []string {
	if taskStatus != "MANUAL_REVIEW" {
		return []string{}
	}
	actions := make([]string, 0, 3)
	if reportDigest != "" && (verdict == "REVISE" || verdict == "REJECT") {
		actions = append(actions, domain.HumanActionAcceptRisk)
	}
	actions = append(actions, domain.HumanActionAuthorizeRevision, domain.HumanActionCancelTask)
	return actions
}

func (s *Store) attachHumanDecisions(ctx context.Context, tenantID int64, items []domain.RunView) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, len(items))
	positions := make(map[int64]int, len(items))
	for i := range items {
		ids[i] = items[i].ID
		positions[items[i].ID] = i
	}
	rows, err := s.db.Query(ctx, `SELECT review_run_id,jsonb_build_object(
 'id',id::text,'action',action,'reason',reason,'revisionGuidance',COALESCE(revision_guidance,''),'aiVerdict',ai_verdict,
 'reportDigest',canonical_report_digest,'qualityOutcome',quality_outcome,
 'resultingTaskTurnId',COALESCE(resulting_task_turn_id,0)::text,
 'decidedAt',to_char(decided_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'))
FROM workforce_human_review_decisions
WHERE tenant_id=$1 AND review_run_id=ANY($2::BIGINT[]) AND status='APPLIED'`, tenantID, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var runID int64
		var decisionJSON []byte
		if err = rows.Scan(&runID, &decisionJSON); err != nil {
			return err
		}
		position, ok := positions[runID]
		if !ok {
			return domain.ErrInvalidReviewInput
		}
		var decision domain.HumanDecisionSummary
		if err = json.Unmarshal(decisionJSON, &decision); err != nil {
			return domain.ErrInvalidReviewInput
		}
		items[position].HumanDecision = &decision
	}
	return rows.Err()
}
